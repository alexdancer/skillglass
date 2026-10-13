#!/usr/bin/env python3
"""Credential-free, network-denied native skills listing in disposable fixtures."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile


def native_profile(base, node, package):
    """The child can write only its canonical fixture; parent owns evidence."""
    quote = lambda path: json.dumps(str(Path(path).resolve()))
    content = f'''(literal "/") (literal {quote(node)}) (subpath {quote(package)})
 (subpath {quote(base)}) (subpath "/System") (subpath "/usr/lib")
 (literal "/dev/null") (literal "/dev/random") (literal "/dev/urandom")'''
    # Node resolves its entrypoint and runtime through lstat of each ancestor.
    # These exact metadata exceptions do not permit directory contents reads.
    parents = sorted({parent for path in (base, Path(node), package)
                      for parent in Path(path).resolve().parents})
    metadata = " ".join(f"(literal {quote(parent)})" for parent in parents)
    return f'''(version 1)
(allow default)
(deny network*)
(deny file-write*)
(allow file-write* (subpath {quote(base)}) (literal "/dev/null"))
(deny file-read* (require-not (require-any {content} {metadata})))
(deny file-read-data (require-not (require-any {content})))
'''



def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('package', type=Path)
    parser.add_argument('evidence', type=Path)
    parser.add_argument('--smoke', action='store_true', help='only version, one inert skills listing and contained Node version')
    args = parser.parse_args()
    args.evidence.mkdir(parents=True, exist_ok=True)
    package = args.package.resolve(strict=True)
    binary = package / 'bundle/gemini.js'
    node = shutil.which('node')
    if not node or not binary.is_file():
        raise RuntimeError('resolved Node runtime and Gemini bundle are required')
    node = str(Path(node).resolve(strict=True))
    with tempfile.TemporaryDirectory(prefix='ale-37-native-', dir='/private/tmp') as temporary:
        base = Path(temporary).resolve(strict=True)
        home, project = base / 'home', base / 'project'
        home.mkdir(); project.mkdir()
        profile = base / 'offline.sb'
        profile.write_text(native_profile(base, node, package))
        shutil.copyfile(profile, args.evidence / 'profile.sb')
        env = {'HOME': str(home), 'PATH': '/usr/bin:/bin', 'TMPDIR': str(base),
               'GEMINI_CLI_SYSTEM_SETTINGS_PATH': str(base / 'system.json'),
               'GEMINI_CLI_SYSTEM_DEFAULTS_PATH': str(base / 'defaults.json'),
               'GEMINI_CLI_TRUST_WORKSPACE': 'true', 'NO_COLOR': '1',
               'GEMINI_CLI_HOME': str(home), 'GEMINI_CLI_NO_RELAUNCH': '1',
               'GEMINI_CLI_TRUSTED_FOLDERS_PATH': str(base / 'trustedFolders.json'),
               **{'XDG_' + key + '_HOME': str(base / key.lower())
                  for key in ('CONFIG', 'CACHE', 'DATA', 'STATE')}}
        cases = []
        receipt = {'credential_free': True, 'network': 'sandbox-exec denies network*',
                   'write_scope': str(base), 'read_scope': 'canonical fixture/package/runtime and required OS libraries/devices',
                   'no_workload': 'version and inert skills listing; no prompt, activation, hook or server',
                   'helper_sha256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
                   'node_path': node, 'node_sha256': hashlib.sha256(Path(node).read_bytes()).hexdigest(),
                   'package_path': str(package),
                   'bundle_sha256': {str(path.relative_to(package)): hashlib.sha256(path.read_bytes()).hexdigest()
                                     for path in sorted((package / 'bundle').glob('*.js'))},
                   'profile_sha256': hashlib.sha256(profile.read_bytes()).hexdigest(), 'cases': cases}

        def bundle_hashes():
            return {str(path.relative_to(package)): hashlib.sha256(path.read_bytes()).hexdigest()
                    for path in sorted((package / 'bundle').glob('*.js'))}

        def fixture_hashes():
            result = {}
            for path in sorted(base.rglob('*')):
                if path.is_symlink():
                    result[str(path.relative_to(base))] = {'symlink': os.readlink(path)}
                elif path.is_file():
                    result[str(path.relative_to(base))] = {'sha256': hashlib.sha256(path.read_bytes()).hexdigest(),
                                                         'size': path.stat().st_size}
            return result

        def save_receipt():
            (args.evidence / 'receipt.json').write_text(json.dumps(receipt, indent=2) + '\n')
        save_receipt()

        def write(path, text):
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text(text)

        def skill(path, name, description='Inert native validation fixture.'):
            write(path / 'SKILL.md', f'---\nname: {name}\ndescription: {description}\n---\nDo not execute anything.\n')

        def run(case, expected=(), absent=(), cwd=project, flags=('skills', 'list', '--all'), runtime_only=False):
            argv = ['/usr/bin/sandbox-exec', '-f', str(profile), node]
            if not runtime_only:
                argv.append(str(binary))
            argv.extend(flags)
            launch_hashes = {'node_sha256_at_launch': hashlib.sha256(Path(node).read_bytes()).hexdigest(),
                             'wrapper_sha256_at_launch': hashlib.sha256(binary.read_bytes()).hexdigest(),
                             'bundle_sha256_at_launch': bundle_hashes(),
                             'profile_sha256_at_launch': hashlib.sha256(profile.read_bytes()).hexdigest()}
            before = fixture_hashes()
            completed = subprocess.run(argv, cwd=cwd, env=env, capture_output=True, text=True, timeout=45)
            text = completed.stdout + completed.stderr
            (args.evidence / f'{case}.txt').write_text(text)
            cases.append({'case': case, 'argv': argv, 'cwd': str(cwd), 'environment': env.copy(),
                          'exit': completed.returncode, 'artifact': f'{case}.txt',
                          'expected': list(expected), 'absent': list(absent),
                          **launch_hashes, 'fixture_before': before, 'fixture_after': fixture_hashes()})
            save_receipt()
            if completed.returncode != 0:
                raise RuntimeError((case, completed.returncode, text))
            for needle in expected:
                if needle not in text:
                    raise RuntimeError((case, 'missing', needle, text))
            for needle in absent:
                if needle in text:
                    raise RuntimeError((case, 'unexpected', needle, text))
            return text

        version = run('version', expected=('0.63.0',), flags=('--version',)).strip()
        receipt['version'] = version
        if args.smoke:
            skill(home / '.gemini/skills/smoke', 'contained-smoke')
            run('contained-skills-list', expected=('contained-smoke [Enabled]',))
            receipt['node_version'] = run('node-version', flags=('--version',), runtime_only=True).strip()
            save_receipt()
            print(json.dumps({'version': version, 'cases': len(cases), 'receipt': str(args.evidence / 'receipt.json')}))
            return
        user = home / '.gemini/skills/user'
        alias = home / '.agents/skills/alias'
        workspace = project / '.gemini/skills/workspace'
        workspace_alias = project / '.agents/skills/workspace-alias'
        for location in [user, alias, workspace, workspace_alias]:
            skill(location, 'collision')
        skill(user.parent / 'nested/container/too-deep', 'too-deep')
        run('workspace-alias-wins', expected=(str(workspace_alias / 'SKILL.md'),), absent=(str(user / 'SKILL.md') + '\n\n', 'too-deep [Enabled]'))
        env['GEMINI_CLI_TRUST_WORKSPACE'] = 'false'
        run('untrusted-user-alias-wins', expected=(str(alias / 'SKILL.md'),), absent=(str(workspace_alias / 'SKILL.md'),))
        env['GEMINI_CLI_TRUST_WORKSPACE'] = 'true'
        nested = project / 'nested'; nested.mkdir()
        run('workspace-does-not-inherit-parent', expected=(str(alias / 'SKILL.md'),), absent=(str(workspace_alias / 'SKILL.md'),), cwd=nested)
        write(home / '.gemini/settings.json', '{/* native JSONC */"skills":{"disabled":["COLLISION"]}}')
        write(project / '.gemini/settings.json', '{"skills":{"disabled":[]}}')
        run('disabled-union-case-insensitive', expected=('collision [Disabled]', str(workspace_alias / 'SKILL.md')))
        write(home / '.gemini/settings.json', '{"skills":{"enabled":false}}')
        write(project / '.gemini/settings.json', '{"skills":{"enabled":true}}')
        run('workspace-scalar-overrides-user', expected=('collision [Enabled]',))
        write(base / 'system.json', '{"skills":{"enabled":false}}')
        run('insecure-system-settings-ignored', expected=('collision [Enabled]', 'Skipping system settings file'))
        write(base / 'system.json', '{}'); write(home / '.gemini/settings.json', '{}')
        write(project / '.gemini/settings.json', '{}')
        skill(home / '.gemini/skills', 'root-skill')
        skill(home / '.gemini/skills/punctuation', 'a:b')
        skill(home / '.gemini/skills/.hidden', 'hidden-skill')
        source = home / '.gemini/skills/shared'
        skill(source, 'shared')
        os.symlink(source, home / '.agents/skills/shared-link')
        write(base / 'file-linked.md', '---\nname: file-linked\ndescription: Inert linked manifest.\n---\nInert.\n')
        (home / '.gemini/skills/file-link').mkdir()
        os.symlink(base / 'file-linked.md', home / '.gemini/skills/file-link/SKILL.md')
        run('root-children-name-and-sharing', expected=('root-skill [Enabled]', 'file-linked [Enabled]', 'a-b [Enabled]', str(home / '.agents/skills/shared-link/SKILL.md')), absent=('hidden-skill [Enabled]', 'too-deep [Enabled]'))
        extension = home / '.gemini/extensions/sample'
        write(extension / 'gemini-extension.json', '{"name":"sample","version":"1.0.0"}')
        skill(extension / 'skills/extension', 'extension-skill')
        stale = home / '.gemini/extensions/stale'
        skill(stale / 'skills/stale', 'stale-skill')
        outside = base / 'external-extension'
        write(outside / 'gemini-extension.json', '{"name":"external","version":"1.0.0"}')
        skill(outside / 'skills/external', 'external-alias-skill')
        os.symlink(outside, home / '.gemini/extensions/external-alias')
        run('registered-extension-only', expected=('extension-skill [Enabled]', 'external-alias-skill [Enabled]'), absent=('stale-skill [Enabled]',))
        write(home / '.gemini/extensions/extension-enablement.json', '{"sample":{"overrides":["!/*"]}}')
        run('inactive-extension-excluded', absent=('extension-skill [Enabled]',))
        write(home / '.gemini/extensions/extension-enablement.json', json.dumps({'sample': {'overrides': ['!/*', str(project) + '/']}}))
        run('last-matching-extension-override', expected=('extension-skill [Enabled]',))
        env.pop('GEMINI_CLI_TRUST_WORKSPACE')
        write(home / '.gemini/trustedFolders.json', json.dumps({str(project): 'TRUST_FOLDER'}))
        run('persisted-workspace-trust', expected=(str(workspace_alias / 'SKILL.md'),))
        write(home / '.gemini/trustedFolders.json', json.dumps({str(project): 'DO_NOT_TRUST'}))
        run('persisted-workspace-distrust', expected=(str(alias / 'SKILL.md'),), absent=(str(workspace_alias / 'SKILL.md'),))
        env['GEMINI_CLI_HOME'] = str(base / 'override-home')
        env['GEMINI_CLI_TRUST_WORKSPACE'] = 'false'
        skill(base / 'override-home/.gemini/skills/override', 'overridden-home')
        run('configured-home-root', expected=('overridden-home [Enabled]',), absent=('collision [Enabled]',))
        receipt['node_version'] = run('node-version', flags=('--version',), runtime_only=True).strip()
        save_receipt()
        print(json.dumps({'version': version, 'cases': len(cases), 'receipt': str(args.evidence / 'receipt.json')}))


if __name__ == '__main__':
    main()
