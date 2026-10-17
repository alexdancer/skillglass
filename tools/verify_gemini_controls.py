#!/usr/bin/env python3
"""Version-specific Gemini disabled-name proof in credential-free contained fixtures."""
import argparse
import hashlib
import importlib.util
import json
import pathlib
import shutil
import subprocess
import tempfile

def main():
    P = pathlib.Path
    parser = argparse.ArgumentParser(description='Validate Gemini CLI 0.63.0 scoped disabled-name persistence without credentials or network.')
    parser.add_argument('package', type=P, help='installed @google/gemini-cli package directory')
    parser.add_argument('evidence', type=P, help='output directory outside the disposable fixture')
    parser.add_argument('--node', type=P, default=None, help='explicit Node executable (defaults to PATH)')
    args = parser.parse_args()
    OUT = args.evidence.resolve()
    OUT.mkdir(parents=True, exist_ok=True)
    helper = P(__file__).resolve().with_name('verify_gemini.py')
    spec = importlib.util.spec_from_file_location('containment', helper)
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    package = args.package.resolve(strict=True)
    node_binary = args.node or shutil.which('node')
    if not node_binary:
        raise RuntimeError('Node executable is required; supply --node or configure PATH')
    node = P(node_binary).resolve(strict=True)
    binary = package / 'bundle/gemini.js'
    if not binary.is_file():
        raise RuntimeError('Gemini bundle entrypoint is required')
    records = []
    with tempfile.TemporaryDirectory(prefix='ale38-native-', dir='/private/tmp') as td:
        b = P(td)
        h = b / 'home'
        p = b / 'project'
        h.mkdir()
        p.mkdir()
        profile = b / 'offline.sb'
        profile.write_text(mod.native_profile(b, node, package))
        (OUT / 'profile.sb').write_text(profile.read_text())
        env = {'HOME': str(h), 'PATH': '/usr/bin:/bin', 'TMPDIR': str(b), 'GEMINI_CLI_HOME': str(h), 'GEMINI_CLI_NO_RELAUNCH': '1', 'GEMINI_CLI_TRUST_WORKSPACE': 'true', 'GEMINI_CLI_SYSTEM_SETTINGS_PATH': str(b / 'system.json'), 'GEMINI_CLI_SYSTEM_DEFAULTS_PATH': str(b / 'defaults.json'), 'GEMINI_CLI_TRUSTED_FOLDERS_PATH': str(b / 'trusted.json'), 'NO_COLOR': '1', **{'XDG_' + k + '_HOME': str(b / k.lower()) for k in ('CONFIG', 'CACHE', 'DATA', 'STATE')}}
        us = h / '.gemini/settings.json'
        ws = p / '.gemini/settings.json'

        def write(path, obj):
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text(json.dumps(obj))

        def snapshot():
            return {str(x.relative_to(b)): x.read_text() for x in (us, ws, b / 'system.json') if x.exists()}

        def run(name, args, need=(), absent=()):
            before = snapshot()
            argv = ['/usr/bin/sandbox-exec', '-f', str(profile), str(node), str(binary), *args]
            r = subprocess.run(argv, cwd=p, env=env, text=True, capture_output=True, timeout=50)
            out = r.stdout + r.stderr
            (OUT / (name + '.txt')).write_text(out)
            rec = {'case': name, 'argv': argv, 'environment': env.copy(), 'cwd': str(p), 'exit': r.returncode, 'wrapper_sha256_at_launch': hashlib.sha256(binary.read_bytes()).hexdigest(), 'before': before, 'after': snapshot(), 'output': name + '.txt'}
            records.append(rec)
            (OUT / 'native.json').write_text(json.dumps({'package': str(package), 'version': '0.63.0', 'runtime': str(node), 'runtime_sha256': hashlib.sha256(node.read_bytes()).hexdigest(), 'helper_sha256': hashlib.sha256(P(__file__).read_bytes()).hexdigest(), 'containment_helper_sha256': hashlib.sha256(helper.read_bytes()).hexdigest(), 'profile_sha256': hashlib.sha256(profile.read_bytes()).hexdigest(), 'credential_free': True, 'network': 'sandbox-exec denies network*', 'write_scope': {'canonical_fixture': str(b), 'nonpersistent_runtime_sinks': ['/dev/null']}, 'bundle_sha256': {x.name: hashlib.sha256(x.read_bytes()).hexdigest() for x in sorted((package / 'bundle').glob('*.js'))}, 'cases': records}, indent=2) + '\n')
            assert r.returncode == 0, (name, out)
            for t in need:
                assert t in out, (name, t, out)
            for t in absent:
                assert t not in out, (name, t, out)
            return out

        def listing(name, status):
            return run(name, ['skills', 'list', '--all'], ['probe [' + status + ']'])

        def skill(path):
            path.mkdir(parents=True, exist_ok=True)
            (path / 'SKILL.md').write_text('---\nname: probe\ndescription: Inert native control proof.\n---\nNo actions.\n')
        skill(h / '.gemini/skills/probe')
        write(us, {'ui': {'theme': 'Default'}, 'skills': {'disabled': ['unrelated']}})
        write(ws, {'skills': {'disabled': []}})
        version = run('version', ['--version'], ['0.63.0']).strip()
        assert version == '0.63.0', ('unsupported native version', version)
        run('help', ['skills', '--help'])
        listing('initial', 'Enabled')
        run('disable-user', ['skills', 'disable', 'probe', '--scope', 'user'])
        listing('user-disabled', 'Disabled')
        assert json.loads(us.read_text())['skills']['disabled'] == ['unrelated', 'probe']
        write(ws, {'skills': {'disabled': []}})
        listing('empty-workspace-keeps-user-disabled', 'Disabled')
        write(ws, {'skills': {'disabled': ['workspace-other', 'probe']}, 'ui': {'theme': 'Default'}})
        run('enable-native-broadens-both', ['skills', 'enable', 'probe'])
        listing('native-both-enabled', 'Enabled')
        assert json.loads(us.read_text())['skills']['disabled'] == ['unrelated']
        assert json.loads(ws.read_text())['skills']['disabled'] == ['workspace-other']
        run('disable-workspace', ['skills', 'disable', 'probe', '--scope', 'workspace'])
        listing('workspace-disabled', 'Disabled')
        assert 'probe' not in json.loads(us.read_text())['skills']['disabled']
        cfg = json.loads(ws.read_text())
        cfg['skills']['disabled'].remove('probe')
        write(ws, cfg)
        listing('remove-override-inherits-enabled', 'Enabled')
        cfg = json.loads(us.read_text())
        cfg['skills']['disabled'].append('PROBE')
        write(us, cfg)
        listing('case-insensitive-native-disable', 'Disabled')
        run('enable-lowercase-does-not-remove-uppercase', ['skills', 'enable', 'probe'])
        listing('native-case-sensitive-removal-still-disabled', 'Disabled')
        cfg['skills']['disabled'].remove('PROBE')
        write(us, cfg)
        listing('remove-global-case-variant-enables', 'Enabled')
        skill(p / '.gemini/skills/independent')
        run('duplicate-winner', ['skills', 'list', '--all'], [str(p / '.gemini/skills/independent/SKILL.md')], [])
        run('disable-duplicate-name', ['skills', 'disable', 'probe', '--scope', 'user'])
        listing('duplicate-selected-disabled', 'Disabled')
        env['GEMINI_CLI_TRUST_WORKSPACE'] = 'false'
        run('hidden-independent-copy-also-disabled', ['skills', 'list', '--all'], ['probe [Disabled]', str(h / '.gemini/skills/probe/SKILL.md')], [str(p / '.gemini/skills/independent/SKILL.md')])
        env['GEMINI_CLI_TRUST_WORKSPACE'] = 'true'
        cfg = json.loads(us.read_text())
        cfg['skills']['disabled'] = ['unrelated']
        cfg['skills']['enabled'] = False
        write(us, cfg)
        write(ws, {'skills': {'enabled': True, 'disabled': []}})
        listing('workspace-scalar-overrides-global-subsystem', 'Enabled')
        write(b / 'system.json', {'skills': {'enabled': False, 'disabled': ['probe']}})
        run('insecure-system-rejected', ['skills', 'list', '--all'], ['Skipping system settings file', 'probe [Enabled]'])
        env['GEMINI_CLI_TRUST_WORKSPACE'] = 'false'
        run('untrusted-project-scalar-ignored', ['skills', 'list', '--all'], [], ['probe [Enabled]'])
        print(json.dumps({'version': version, 'cases': len(records), 'receipt': str(OUT / 'native.json')}))
if __name__ == '__main__':
    main()
