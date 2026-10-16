#!/usr/bin/env python3
"""Credential-free OpenCode inert fixture skill-tool observations. Never starts a model turn."""
import argparse
import fnmatch
import hashlib
import json
import os
from pathlib import Path
import shutil
import tempfile

from native_containment import native_profile, runtime_data_paths
from native_receipts import recorded_run


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--opencode', required=True)
    parser.add_argument('--out', required=True)
    parser.add_argument('--git', default='/Library/Developer/CommandLineTools/usr/bin/git')
    args = parser.parse_args()
    out = Path(args.out).resolve()
    out.mkdir(parents=True, exist_ok=True)
    source_binary = Path(args.opencode).resolve(strict=True)
    source_hash = hashlib.sha256(source_binary.read_bytes()).hexdigest()
    source_git = Path(args.git).resolve(strict=True)
    git_hash = hashlib.sha256(source_git.read_bytes()).hexdigest()
    base = Path(tempfile.mkdtemp(prefix='ale-36-controls-', dir='/private/tmp'))
    binary_path = base/'native-executable'
    shutil.copyfile(source_binary, binary_path)
    binary_path.chmod(0o700)
    binary = str(binary_path)
    if hashlib.sha256(binary_path.read_bytes()).hexdigest() != source_hash:
        raise RuntimeError('native executable copy changed bytes')
    (base/'bin').mkdir()
    git_binary = base/'bin/git'
    shutil.copyfile(source_git, git_binary)
    git_binary.chmod(0o700)
    if hashlib.sha256(git_binary.read_bytes()).hexdigest() != git_hash:
        raise RuntimeError('Git executable copy changed bytes')
    home, config, project = base/'home', base/'config', base/'project'
    for name in ['home', 'config', 'data', 'cache', 'state', 'tmp', 'managed', 'project', 'extra', 'templates']:
        (base/name).mkdir()
    env = {'PATH':str(base/'bin')+':/usr/bin:/bin', 'HOME':str(home),
           'XDG_CONFIG_HOME':str(config), 'XDG_DATA_HOME':str(base/'data'),
           'XDG_CACHE_HOME':str(base/'cache'), 'XDG_STATE_HOME':str(base/'state'),
           'TMPDIR':str(base/'tmp'), 'OPENCODE_DISABLE_AUTOUPDATE':'true',
           'OPENCODE_DISABLE_MODELS_FETCH':'true', 'OPENCODE_DISABLE_DEFAULT_PLUGINS':'true',
           'OPENCODE_TEST_MANAGED_CONFIG_DIR':str(base/'managed'), 'OPENCODE_PURE':'true',
           'GIT_CONFIG_NOSYSTEM':'1','GIT_CONFIG_GLOBAL':str(base/'missing-global.gitconfig'),
           'GIT_CONFIG_SYSTEM':str(base/'missing-system.gitconfig'),'GIT_TEMPLATE_DIR':str(base/'templates')}
    profile = base/'native.sb'
    runtime_data = runtime_data_paths()
    profile.write_text(native_profile(base, Path(binary), runtime_data=runtime_data))
    policy = profile.read_text()
    (out/'native.sb').write_text(policy)
    receipts, checks = [], []
    def run(label, *arguments, extra=None, expected=0, executable=None, pure=True):
        e = env.copy()
        e.update(extra or {})
        executed = executable or binary
        original = source_git if executed == str(git_binary) else source_binary
        original_hash = git_hash if executed == str(git_binary) else source_hash
        argv = [executed, *(['--pure'] if pure else []), *arguments]
        wrapped = ['/usr/bin/sandbox-exec','-f',str(profile),*argv]
        result, receipt = recorded_run(out, label, wrapped, cwd=project/'nested', env=e,
                                       binary=executed, profile=profile, runtime_data=runtime_data,
                                       source={'source_binary':str(original),
                                               'source_binary_sha256':original_hash})
        (out/(label+'.stdout')).write_bytes(result.stdout)
        (out/(label+'.stderr')).write_bytes(result.stderr)
        receipts.append(receipt)
        timed_out = receipt['timed_out']
        if timed_out:
            raise RuntimeError(label+' timed out; owned process group stopped')
        if result.returncode != expected:
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
        run('git-init','init','-q',str(project),executable=str(git_binary),pure=False)
        git_version = run('git-version','--version',executable=str(git_binary),pure=False).decode().strip()
        version = run('version','--version').decode().strip()
        check('reference version 1.17.15',version=='1.17.15')
        global_file=config/'opencode/opencode.json'
        project_file=project/'nested/opencode.json'
        names=['ale36-target','ale36-target-extra','literal[ab]','literala','questionx','questionxx','prefix','prefix-tail','UPPER','upper','slash/name','slash\\name']
        for i,name in enumerate(names): skill(config/'opencode/skills','inert-'+str(i),name)
        def tool(label,name,code=0):
            # Native skill calls reach permission first, then the unavailable isolated ripgrep runtime.
            # Both observed outcomes exit 1. Do not label the positive branch a completed load.
            raw=run(label,'debug','agent','build','--tool','skill','--params',json.dumps({'name':name}),expected=1)
            error=(out/(label+'.stderr')).read_text()
            if code==0:
                check(label+' passed permission and reached isolated ripgrep blocker','ripgrep execution failed' in error)
            else:
                check(label+' native deny before ripgrep','ripgrep execution failed' not in error and any(x in error.lower() for x in ['denied','permission','rule']))
        write(global_file,{'permission':{'skill':{'*':'ask','ale36-target':'allow'}}})
        tool('tool-global-allow','ale36-target')
        write(global_file,{'permission':{'skill':{'*':'allow','ale36-target':'deny'}}})
        tool('tool-global-deny','ale36-target',1)
        tool('tool-exact-prefix-untouched','ale36-target-extra')
        write(project_file,{'permission':{'skill':{'ale36-target':'allow'}}})
        tool('tool-project-allow','ale36-target')
        write(project_file,{'permission':{'skill':{'ale36-target':'deny'}}})
        write(global_file,{'permission':{'skill':{'*':'allow','ale36-target':'allow'}}})
        tool('tool-project-deny','ale36-target',1)
        write(project_file,{'permission':{'skill':{}}})
        tool('tool-project-remove-inherit-allow','ale36-target')
        write(global_file,{'permission':{'skill':{'*':'allow','ale36-target':'deny'}}})
        tool('tool-project-remove-inherit-deny','ale36-target',1)
        write(global_file,{'permission':{'skill':{'*':'ask'}}})
        # Debug intentionally treats ask as successful without prompting; inspect emitted rule only.
        agent=json.loads(run('tool-removal-inherit-ask-rules','debug','agent','build'))
        check('removal restores ask rule',agent['permission'][-2]['action']=='ask' or any(r['permission']=='skill' and r['pattern']=='*' and r['action']=='ask' for r in agent['permission']))
        write(global_file,{'permission':{'skill':{'*':'allow','literal[ab]':'deny','question?':'deny','prefix*':'deny','UPPER':'deny','slash\\name':'deny'}}})
        tool('tool-brackets-literal','literal[ab]',1)
        tool('tool-brackets-no-character-class','literala')
        tool('tool-question-one','questionx',1)
        tool('tool-question-two','questionxx')
        tool('tool-star-zero','prefix',1)
        tool('tool-star-many','prefix-tail',1)
        tool('tool-case-uppercase','UPPER',1)
        tool('tool-case-lowercase','upper')
        tool('tool-slash-normalization','slash/name',1)
        tool('tool-backslash-normalization','slash\\name',1)
        write(global_file,{'permission':{'skill':{'*':'allow','ale36-*':'deny','ale36-target':'allow'}}})
        tool('tool-last-exact-wins','ale36-target')
        write(global_file,{'permission':{'skill':{'*':'allow','ale36-target':'allow','ale36-*':'deny'}}})
        tool('tool-last-wildcard-wins','ale36-target',1)
        (out/'fixture-final-files.json').write_text(json.dumps({str(p.relative_to(base)):p.read_text() for p in base.rglob('opencode.json*')},indent=2))

    finally:
        (out/'receipts.json').write_text(json.dumps({'version':locals().get('version'),'git_version':locals().get('git_version'),'git_runtime':{'source':str(source_git),'sha256':git_hash,'fixture_copy':str(git_binary)},'fixture':str(base),'binary':binary,'source_binary':str(source_binary),'source_binary_sha256':source_hash,'environment':env,'profile':policy,'runtime_data':runtime_data,'network':'sandbox denies network*','write_boundary':'canonical fixture plus nonpersistent /dev/null discard','read_boundary':'fixture, exact executable and OS runtime paths; live homes and managed inputs excluded with controlled ENOENT','commands':receipts,'invocation_journal':str(out/'invocations'),'checks':checks,'gaps':['No model inference. Debug executes inert fixture skill tool; allow branch reaches isolated ripgrep blocker; deny branch rejects earlier. Debug ask bypasses prompting.','Debug skill lists raw definitions without permissions.','Concurrent collision winner cannot be generalized.','Native built-in is omitted from local bundle inventory.','Running session refresh and tool enforcement remain unverified; debug emits rules, source supplies evaluator semantics.','Actual system/enterprise and other user inputs excluded; fixture managed override only.','Historical outside-fixture effects remain unknown.']},indent=2)+'\n')
        shutil.rmtree(base)
    print(json.dumps({'checks':len(checks),'passed':all(x['passed'] for x in checks),'out':str(out)}))


if __name__ == '__main__':
    main()
