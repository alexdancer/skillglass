#!/usr/bin/env python3
"""Differential Pi 1.1.0 ignore probe. No sessions, extensions, or workloads.
usage: python3 pi-ignore-contained.py INSTALLED_PI_PACKAGE SKILLGLASS EVIDENCE_DIR
"""
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile

if len(sys.argv) != 4 or sys.platform != 'darwin':
    raise SystemExit(__doc__)
pi, binary, out = [Path(p).resolve() for p in sys.argv[1:]]
out.mkdir(parents=True, exist_ok=True)
node = Path(os.environ.get('PI_PROBE_NODE') or shutil.which('node')).resolve()
store = next((p for p in pi.parents if p.name == 'links'), pi)
live_settings = str(Path.home()/'.pi/agent/settings.json')
root = Path(tempfile.mkdtemp(prefix='ale50-native-', dir='/private/tmp'))
def write(path, text):
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(text)
def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()
def run(args, env, cwd):
    return subprocess.run(args, env=env, cwd=cwd, capture_output=True, text=True, timeout=30)
# Native retains one shared matcher during depth-first traversal. Test root,
# nested and per-file ordering with positive and negative matches.
cases = {
    'root-manifest': {'SKILL.md': '---\nname: root-skill\ndescription: Inert root.\n---\nNo workload.\n'},
    'ignored-root-manifest': {'SKILL.md': '---\nname: root-skill\ndescription: Inert root.\n---\nNo workload.\n', '.ignore': 'SKILL.md\n!target/SKILL.md\n!group/target/SKILL.md\n!sibling/SKILL.md\n'},
    'ignored-alias': {'.ignore': 'alias/\n'},
    'ignored-original': {'.ignore': 'target/\n'},
    'empty': {},
    'comments': {'.gitignore': '# comment\n\n'},
    'unrelated': {'.gitignore': 'unrelated/\n'},
    'directory': {'.gitignore': 'target/\n'},
    'anchored': {'.gitignore': '/target/\n'},
    'manifest': {'.gitignore': 'target/SKILL.md\n'},
    'star': {'.gitignore': 'targ*/\n'},
    'star-manifest': {'.gitignore': 'target/*.md\n'},
    'ascii-class': {'.gitignore': '[t]arget/\n'},
    'case-fold': {'.gitignore': 'TARGET/skill.MD\n'},
    'negate-file': {'.gitignore': 'target/*\n!target/SKILL.md\n'},
    'negate-parent': {'.gitignore': 'target/\n!target/SKILL.md\n'},
    'file-order': {'.gitignore': 'target/SKILL.md\n', '.ignore': '!target/SKILL.md\n'},
    'fdignore-order': {'.gitignore': 'target/SKILL.md\n', '.ignore': '!target/SKILL.md\n', '.fdignore': 'target/SKILL.md\n'},
    'nested-rule': {'group/.ignore': 'target/\n'},
    'nested-negation': {'.gitignore': 'group/target/*\n', 'group/.ignore': '!target/SKILL.md\n'},
    'leaf-live': {'target/.gitignore': '__pycache__/\n*.py[cod]\n.DS_Store\n'},
    'crlf': {'.gitignore': 'target/SKILL.md\r\n'},
    'trailing-space': {'.gitignore': 'target/SKILL.md   \n'},
}
receipts = []
for name, ignores in cases.items():
    base = root/name
    home, agent, project = base/'home', base/'home/.pi/agent', base/'project'
    project.mkdir(parents=True); (project/'.git').mkdir()
    skills = agent/'skills'
    for relative, skillname in [('target','target'), ('group/target','nested'), ('sibling','sibling')]:
        write(skills/relative/'SKILL.md', f'---\nname: {skillname}\ndescription: Inert fixture.\n---\nNo workload.\n')
    if name in ['ignored-alias', 'ignored-original']:
        (skills/'alias').symlink_to(skills/'target')
    write(agent/'settings.json', '{}')
    for relative, content in ignores.items():
        write(skills/relative, content)
    script = base/'probe.mjs'
    code = """import fs from 'node:fs';import net from 'node:net';import assert from 'node:assert/strict';import {spawnSync} from 'node:child_process';
const denied=fn=>{try{fn();return false}catch(e){return ['EPERM','EACCES'].includes(e.code)}};
assert(denied(()=>fs.readFileSync(LIVE_SETTINGS)));
assert(denied(()=>fs.writeFileSync(process.argv[2]+'/forbidden','x')));
const child=spawnSync('/usr/bin/true');assert(child.error&&['EPERM','EACCES'].includes(child.error.code));
assert(await new Promise(r=>{const s=net.connect({host:'127.0.0.1',port:9});s.on('error',e=>r(['EPERM','EACCES'].includes(e.code)));s.on('connect',()=>{s.destroy();r(false)})}));
const {SettingsManager}=await import(SETTINGS_URL);
const {DefaultPackageManager}=await import(PACKAGES_URL);
const {loadSkills}=await import(SKILLS_URL);
const agentDir=process.env.PI_CODING_AGENT_DIR,cwd=process.cwd();
const settingsManager=SettingsManager.create(cwd,agentDir);
const resolved=await new DefaultPackageManager({cwd,agentDir,settingsManager}).resolve(()=>{throw Error('install forbidden')});
const loaded=loadSkills({cwd,agentDir,includeDefaults:false,skillPaths:resolved.skills.filter(s=>s.enabled).map(s=>s.path)});
console.log(JSON.stringify({controls:{liveReadDenied:true,writesDenied:true,networkDenied:true,childrenDenied:true},resolved:resolved.skills,loaded}));
"""
    code = code.replace('LIVE_SETTINGS', json.dumps(live_settings))
    for key, file in [('SETTINGS_URL','settings-manager.js'), ('PACKAGES_URL','package-manager.js'), ('SKILLS_URL','skills.js')]:
        code = code.replace(key, json.dumps((pi/'dist/core'/file).as_uri()))
    write(script, code)
    profile = base/'native.sb'
    write(profile, '(version 1) (allow default) (deny network*) (deny file-write*) (allow file-write* (literal "/dev/null")) (deny file-read* (require-not (require-any (subpath '+json.dumps(str(base))+') (subpath '+json.dumps(str(store))+') (literal '+json.dumps(str(node))+') (literal "/") (literal "/usr/bin/sandbox-exec") (subpath "/System") (subpath "/usr/lib") (subpath "/opt/homebrew/opt") (subpath "/dev")))) (allow file-read-metadata) (deny process-exec (require-not (literal '+json.dumps(str(node))+')))')
    env = {'HOME':str(home), 'PI_CODING_AGENT_DIR':str(agent), 'PATH':'/usr/bin:/bin', 'TMPDIR':str(base), 'TERM':'dumb'}
    native = run(['/usr/bin/sandbox-exec','-f',str(profile),str(node),str(script),str(skills)], env, project)
    (out/(name+'-native.stdout')).write_text(native.stdout)
    (out/(name+'-native.stderr')).write_text(native.stderr)
    assert native.returncode == 0, (name,native.returncode,native.stderr)
    expected = sorted(s['name'] for s in json.loads(native.stdout)['loaded']['skills'])
    actual = run([str(binary),'list','--agent','pi','--json'], env, project)
    (out/(name+'-cli.json')).write_text(actual.stdout)
    assert actual.returncode == 0, (name,actual.returncode,actual.stdout[:500])
    observed = sorted(s['name'] for s in json.loads(actual.stdout)['sources'] if s['availability']['state'] == 'available')
    assert expected == observed, (name,expected,observed)
    receipts.append({'case':name,'native_names':expected,'cli_names':observed,'native_exit':native.returncode,'cli_exit':actual.returncode,'native_script_sha256':digest(script),'profile_sha256':digest(profile)})
(out/'receipt.json').write_text(json.dumps({'fixture_root':str(root),'pi_version':json.loads((pi/'package.json').read_text())['version'],'node':str(node),'node_sha256':digest(node),'binary':str(binary),'binary_sha256':digest(binary),'native_source_sha256':{f:digest(pi/'dist/core'/f) for f in ['skills.js','package-manager.js','settings-manager.js']},'results':receipts},indent=2)+'\n')
print(f'PASS {len(receipts)} contained native/CLI differential cases; fixtures retained at {root}')
