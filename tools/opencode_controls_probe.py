#!/usr/bin/env python3
"""Credential-free OpenCode debug observations. Never starts a turn or loads a skill."""
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
        git_root = run('git-boundary','rev-parse','--show-toplevel',executable=str(git_binary),pure=False).decode().strip()
        check('controlled native Git root',git_root==str(project))
        version = run('version','--version').decode().strip()
        check('reference version 1.17.15',version=='1.17.15')
        target='ale36-target'
        sibling='ale36-target-extra'
        skill(config/'opencode/skills',target,target)
        skill(config/'opencode/skills',sibling,sibling)
        global_file=config/'opencode/opencode.json'
        project_file=project/'nested/opencode.json'
        def reset():
            for path in base.rglob('opencode.json*'): path.unlink()
        def state(label,name=target,extra=None):
            result=permission(label,name,extra)
            return result
        write(global_file,{'permission':{'skill':{'*':'ask',target:'allow',sibling:'deny'},'bash':'ask'},'instructions':['inert-unrelated.md']})
        check('global exact allow',state('global-allow')=='allow')
        check('global exact sibling deny',state('global-sibling-deny',sibling)=='deny')
        cfg=json.loads(global_file.read_text());cfg['permission']['skill'][target]='deny';write(global_file,cfg)
        check('global exact deny',state('global-deny')=='deny')
        del cfg['permission']['skill'][target];write(global_file,cfg)
        check('global exact removal preserves inherited ask',state('global-remove-inherit-ask')=='ask')
        check('unrelated setting survives native loading',json.loads(run('global-preserved-config','debug','config'))['permission']['bash']=='ask')
        write(project_file,{'permission':{'skill':{target:'allow'},'edit':'deny'}})
        check('project exact allow overrides inherited ask',state('project-allow')=='allow')
        write(project_file,{'permission':{'skill':{target:'deny'},'edit':'deny'}})
        check('project exact deny overrides inherited ask',state('project-deny')=='deny')
        write(project_file,{'permission':{'skill':{},'edit':'deny'}})
        check('project exact removal restores inherited ask',state('project-remove-inherit-ask')=='ask')
        check('project remove preserves unrelated edit',json.loads(run('project-preserved-config','debug','config'))['permission']['edit']=='deny')
        cfg['permission']['skill'][target]='deny';write(global_file,cfg)
        write(project_file,{'permission':{'skill':{target:'allow'}}})
        check('project allow overrides global deny',state('project-overrides-global')=='allow')
        write(project/'nested/.opencode/opencode.json',{'permission':{'skill':{target:'deny'}}})
        check('native directory overrides project file',state('native-directory-overrides')=='deny')
        check('inline overrides native directory',state('inline-overrides',extra={'OPENCODE_CONFIG_CONTENT':json.dumps({'permission':{'skill':{target:'allow'}}})})=='allow')
        write(base/'managed/opencode.json',{'permission':{'skill':{target:'ask'}}})
        check('controlled managed overrides inline',state('managed-overrides',extra={'OPENCODE_CONFIG_CONTENT':json.dumps({'permission':{'skill':{target:'allow'}}})})=='ask')
        check('environment overrides managed',state('environment-overrides',extra={'OPENCODE_PERMISSION':json.dumps({'skill':{target:'deny'}})})=='deny')
        write(project_file,{'agent':{'build':{'permission':{'skill':{target:'allow'}}}}})
        check('agent overrides environment',state('agent-overrides',extra={'OPENCODE_PERMISSION':json.dumps({'skill':{target:'deny'}})})=='allow')
        reset()
        write(global_file,{'permission':{'skill':{'*':'ask',target:'allow'}}})
        write(config/'opencode/opencode.jsonc',{'permission':{'skill':{target:'deny'}}})
        check('global jsonc overrides json',state('global-jsonc-overrides')=='deny')
        reset()
        write(global_file,{'permission':{'skill':{'*':'ask',target:'deny'}}})
        check('exact key does not target name prefix',state('exact-name-prefix',sibling)=='ask')
        check('exact key case sensitive',state('exact-name-case','ALE36-target')=='ask')
        reset()
        write(global_file,{'permission':{'skill':{'*':'ask','ale36-*':'deny',target:'allow'}}})
        check('exact after wildcard wins',state('exact-after-wildcard')=='allow')
        check('wildcard still affects sibling',state('wildcard-sibling',sibling)=='deny')
        # Replacement preserves insertion order: later wildcard can defeat an exact replacement.
        write(global_file,{'permission':{'skill':{'*':'ask',target:'deny','ale36-*':'ask'}}})
        write(project_file,{'permission':{'skill':{target:'allow'}}})
        check('existing exact replacement stays before later wildcard',state('existing-position')=='ask')
        raw=json.loads(run('ordered-config','debug','config'))['permission']['skill']
        check('native insertion order retained',list(raw)==['*',target,'ale36-*'])
        reset()
        write(global_file,{'permission':{'skill':{'*':'ask',target:'deny'}}})
        check('fresh process sees deny before edit',state('fresh-before')=='deny')
        write(global_file,{'permission':{'skill':{'*':'ask',target:'allow'}}})
        check('fresh process sees allow after edit',state('fresh-after')=='allow')
        skill(home/'.agents/skills','copy-a','ale36-duplicate')
        skill(project/'.opencode/skills','copy-b','ale36-duplicate')
        winners=[]
        for index in range(3):
            found=[x for x in listed('duplicate-'+str(index)) if x['name']=='ale36-duplicate']
            check('duplicate name collapses '+str(index),len(found)==1)
            winners.append(found[0]['location'])
        (out/'duplicate-observations.json').write_text(json.dumps({'locations':winners,'winner_rule':'unverified; do not generalize repeated winner'},indent=2))
        # Capture the actual native pattern representation without claiming debug executes evaluator.
        reset()
        write(global_file,{'permission':{'skill':{'*':'ask','literal[ab]':'deny','question?':'deny','prefix*':'allow','slash\\name':'deny'}}})
        run('special-pattern-native-rules','debug','agent','build')
        check('native accepts literal and wildcard pattern keys',True)
        (out/'fixture-final-files.json').write_text(json.dumps({str(p.relative_to(base)):p.read_text() for p in base.rglob('opencode.json*')},indent=2))

    finally:
        (out/'receipts.json').write_text(json.dumps({'version':locals().get('version'),'git_version':locals().get('git_version'),'git_runtime':{'source':str(source_git),'sha256':git_hash,'fixture_copy':str(git_binary)},'fixture':str(base),'binary':binary,'source_binary':str(source_binary),'source_binary_sha256':source_hash,'environment':env,'profile':policy,'runtime_data':runtime_data,'network':'sandbox denies network*','write_boundary':'canonical fixture plus nonpersistent /dev/null discard','read_boundary':'fixture, exact executable and OS runtime paths; live homes and managed inputs excluded with controlled ENOENT','commands':receipts,'invocation_journal':str(out/'invocations'),'checks':checks,'gaps':['No inference or tool execution.','Debug skill lists raw definitions without permissions.','Concurrent collision winner cannot be generalized.','Native built-in is omitted from local bundle inventory.','Running session refresh and tool enforcement remain unverified; debug emits rules, source supplies evaluator semantics.','Actual system/enterprise and other user inputs excluded; fixture managed override only.','Historical outside-fixture effects remain unknown.']},indent=2)+'\n')
        shutil.rmtree(base)
    print(json.dumps({'checks':len(checks),'passed':all(x['passed'] for x in checks),'out':str(out)}))


if __name__ == '__main__':
    main()
