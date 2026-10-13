#!/usr/bin/env python3
"""Credential-free OpenCode debug observations. Never starts a turn or loads a skill."""
import argparse
import fnmatch
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--opencode', required=True)
    parser.add_argument('--out', required=True)
    args = parser.parse_args()
    out = Path(args.out).resolve()
    out.mkdir(parents=True, exist_ok=True)
    binary = str(Path(args.opencode).resolve())
    base = Path(tempfile.mkdtemp(prefix='ale-35-native-', dir='/private/tmp'))
    home, config, project = base/'home', base/'config', base/'project'
    for name in ['home', 'config', 'data', 'cache', 'state', 'tmp', 'managed', 'project', 'extra']:
        (base/name).mkdir()
    env = {'PATH':'/opt/homebrew/bin:/usr/bin:/bin', 'HOME':str(home),
           'XDG_CONFIG_HOME':str(config), 'XDG_DATA_HOME':str(base/'data'),
           'XDG_CACHE_HOME':str(base/'cache'), 'XDG_STATE_HOME':str(base/'state'),
           'TMPDIR':str(base/'tmp'), 'OPENCODE_DISABLE_AUTOUPDATE':'true',
           'OPENCODE_DISABLE_MODELS_FETCH':'true', 'OPENCODE_DISABLE_DEFAULT_PLUGINS':'true',
           'OPENCODE_TEST_MANAGED_CONFIG_DIR':str(base/'managed'), 'OPENCODE_PURE':'true'}
    profile = base/'native.sb'
    # Protect the real home even if a native component disregards environment routing.
    profile.write_text('(version 1)\n(allow default)\n(deny network*)\n'
                       '(deny file-read* file-write* (subpath "'+str(Path.home())+'"))\n')
    receipts, checks = [], []
    def run(label, *arguments, extra=None):
        e = env.copy()
        e.update(extra or {})
        argv = [binary, '--pure', *arguments]
        result = subprocess.run(['/usr/bin/sandbox-exec','-f',str(profile),*argv],
                                cwd=project/'nested', env=e, capture_output=True, timeout=60)
        (out/(label+'.stdout')).write_bytes(result.stdout)
        (out/(label+'.stderr')).write_bytes(result.stderr)
        receipts.append({'label':label,'argv':argv,'cwd':str(project/'nested'),
                         'exit':result.returncode,'stdout':label+'.stdout','stderr':label+'.stderr'})
        if result.returncode != 0:
            raise RuntimeError(label+' failed: '+result.stderr.decode(errors='replace')[-1000:])
        return result.stdout
    def check(label, value):
        checks.append({'check':label,'passed':bool(value)})
        if not value:
            raise AssertionError(label)
    def skill(root, folder, name, description=True):
        path = root/folder/'SKILL.md'
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text('---\nname: '+name+'\n'+('description: Inert native fixture.\n' if description else '')+'---\nNever execute.\n')
        return path
    def write(path, value):
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(json.dumps({'$schema':'https://opencode.ai/config.json',**value}))
    def listed(label, extra=None):
        return [x for x in json.loads(run(label,'debug','skill',extra=extra)) if x['location']!='<built-in>']
    def permission(label, name, extra=None):
        agent = json.loads(run(label,'debug','agent','build',extra=extra))
        matching = [x for x in agent['permission'] if fnmatch.fnmatchcase('skill',x['permission']) and fnmatch.fnmatchcase(name,x['pattern'])]
        return matching[-1]['action'] if matching else 'ask'
    try:
        (project/'nested').mkdir()
        subprocess.run(['/usr/bin/git','init','-q',str(project)],env=env,check=True,capture_output=True)
        version = run('version','--version').decode().strip()
        check('reference version 1.17.15',version=='1.17.15')
        for root,folder,name in [(home/'.agents/skills','shared','agents-global'),
                                 (home/'.claude/skills','shared','claude-global'),
                                 (config/'opencode/skills','global','native-global'),
                                 (home/'.opencode/skill','legacy','home-native'),
                                 (project/'.opencode/skills','project','project-native'),
                                 (project/'nested/.agents/skills','leaf','leaf-external'),
                                 (project/'.agents/skills','repo','repo-external'),
                                 (base/'.agents/skills','outside','outside-repo'),
                                 (base/'extra','deep/nested','configured')]:
            skill(root,folder,name)
        skill(home/'.agents/skills','.hidden/nested','external-hidden')
        skill(config/'opencode/skills','.hidden/nested','native-hidden')
        parent = skill(config/'opencode/skills','parent','parent')
        skill(parent.parent,'nested','nested-bundle')
        desc = skill(config/'opencode/skills','no-description','no-description',False)
        shared = skill(base/'shared','source','shared-target')
        (home/'.agents/skills/alias').symlink_to(shared.parent, target_is_directory=True)
        (project/'.opencode/skills/alias').symlink_to(shared.parent, target_is_directory=True)
        link = config/'opencode/skills/file-link/SKILL.md'
        link.parent.mkdir(parents=True)
        link.symlink_to(shared)
        write(config/'opencode/opencode.json',{'skills':{'paths':[str(base/'extra')]},'permission':{'skill':{'*':'allow','project-*':'ask','agents-*':'deny'}}})
        all_skills = listed('discovery')
        names = {x['name'] for x in all_skills}
        check('global project configured roots and recursive nested bundles',{'agents-global','claude-global','native-global','home-native','project-native','leaf-external','repo-external','configured','parent','nested-bundle','no-description','shared-target'}<=names)
        check('git boundary excludes parent source','outside-repo' not in names)
        check('external hidden directory included, native hidden excluded','external-hidden' in names and 'native-hidden' not in names)
        check('file manifest alias loads raw definition',any(x['location']==str(link) for x in all_skills))
        check('allow native rule',permission('allow','native-global')=='allow')
        check('ask native rule',permission('ask','project-native')=='ask')
        check('deny native rule',permission('deny','agents-global')=='deny')
        write(project/'opencode.json',{'permission':{'skill':{'agents-*':'ask','project-native':'deny'}}})
        write(project/'nested/opencode.jsonc',{'permission':{'skill':{'project-native':'allow'}}})
        resolved = json.loads(run('config-layered','debug','config'))
        check('config deep merge and existing key position',list(resolved['permission']['skill'])==['*','project-*','agents-*','project-native'])
        check('project replacement keeps lower permission pattern',permission('project-rule','agents-global')=='ask')
        write(project/'nested/.opencode/opencode.json',{'permission':{'skill':{'project-native':'ask'}}})
        write(project/'.opencode/opencode.json',{'permission':{'skill':{'project-native':'deny'}}})
        check('native directory root overrides nearer directory',permission('directory-order','project-native')=='deny')
        custom = base/'custom'
        custom.mkdir()
        skill(custom/'skills','custom','custom-directory')
        write(custom/'opencode.json',{'permission':{'skill':{'project-native':'allow'}}})
        check('custom directory loaded last',permission('custom-directory-order','project-native',{'OPENCODE_CONFIG_DIR':str(custom)})=='allow')
        check('custom directory adds skill',any(x['name']=='custom-directory' for x in listed('custom-directory-discovery',{'OPENCODE_CONFIG_DIR':str(custom)})))
        check('inline override after directory',permission('inline-order','project-native',{'OPENCODE_CONFIG_CONTENT':json.dumps({'permission':{'skill':{'project-native':'ask'}}})})=='ask')
        write(base/'managed/opencode.json',{'permission':{'skill':{'project-native':'deny'}}})
        check('managed config overrides inline',permission('managed-order','project-native',{'OPENCODE_CONFIG_CONTENT':json.dumps({'permission':{'skill':{'project-native':'allow'}}})})=='deny')
        (base/'managed/opencode.json').unlink()
        write(project/'opencode.json',{'agent':{'build':{'permission':{'skill':{'agents-*':'allow'}}}}})
        check('agent permission appended after global/project rules',permission('agent-override','agents-global')=='allow')
        check('environment permission merges before agent override',permission('environment-override','agents-global',{'OPENCODE_PERMISSION':'{"skill":{"agents-*":"deny"}}'})=='allow')
        write(project/'opencode.json',{'permission':{'skill':{'agents-*':'allow','*':'deny'}}})
        check('key replacement preserves lower insertion position',permission('ordered-wildcard','agents-global')=='allow')
        off = {x['name'] for x in listed('external-disabled',{'OPENCODE_DISABLE_EXTERNAL_SKILLS':'true'})}
        check('external flag excludes both compatibility roots','agents-global' not in off and 'claude-global' not in off and 'native-global' in off)
        only = {x['name'] for x in listed('claude-disabled',{'OPENCODE_DISABLE_CLAUDE_CODE_SKILLS':'true'})}
        check('claude-only flag preserves agents root','claude-global' not in only and 'agents-global' in only)
        cache = skill(base/'cache/opencode/skills','stale','stale-cache')
        check('cache folder alone is inactive',all(x['name']!='stale-cache' for x in listed('inactive-cache')))
        skill(home/'.agents/skills','collision','collision')
        skill(project/'.opencode/skills','collision','collision')
        winners=[]
        for index in range(5):
            winners.append(next(x['location'] for x in listed('collision-'+str(index)) if x['name']=='collision'))
        check('native equal names collapse to one source',len(winners)==5)
        # A repeated winner does not establish deterministic precedence under the concurrent source implementation.
        (out/'collision-observations.json').write_text(json.dumps({'locations':winners,'winner_rule':'unknown: source loads concurrently'},indent=2))
    finally:
        (out/'receipts.json').write_text(json.dumps({'version':locals().get('version'),'fixture':str(base),'environment_keys':sorted(env),'network':'sandbox denies network*','real_home':'sandbox denies real HOME reads/writes','commands':receipts,'checks':checks,'gaps':['No inference or tool execution.','Debug skill lists raw definitions without permissions.','Concurrent collision winner cannot be generalized.','Native built-in is omitted from local bundle inventory.','Running session, remote policy, plugin hooks and writers remain unverified.']},indent=2)+'\n')
        shutil.rmtree(base)
    print(json.dumps({'checks':len(checks),'passed':all(x['passed'] for x in checks),'out':str(out)}))


if __name__ == '__main__':
    main()
