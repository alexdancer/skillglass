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


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('package', type=Path)
    parser.add_argument('evidence', type=Path)
    args = parser.parse_args()
    args.evidence.mkdir(parents=True, exist_ok=True)
    binary = args.package.resolve() / 'bundle/gemini.js'
    node = shutil.which('node')
    assert node and binary.is_file()
    with tempfile.TemporaryDirectory(prefix='ale-37-native-', dir='/private/tmp') as temporary:
        base = Path(temporary)
        home, project = base / 'home', base / 'project'
        home.mkdir(); project.mkdir()
        profile = base / 'offline.sb'
        profile.write_text('(version 1)\n(allow default)\n(deny network*)\n')
        env = {'HOME': str(home), 'PATH': '/usr/bin:/bin', 'TMPDIR': str(base),
               'GEMINI_CLI_SYSTEM_SETTINGS_PATH': str(base / 'system.json'),
               'GEMINI_CLI_SYSTEM_DEFAULTS_PATH': str(base / 'defaults.json'),
               'GEMINI_CLI_TRUST_WORKSPACE': 'true', 'NO_COLOR': '1'}
        cases = []

        def write(path, text):
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text(text)

        def skill(path, name, description='Inert native validation fixture.'):
            write(path / 'SKILL.md', f'---\nname: {name}\ndescription: {description}\n---\nDo not execute anything.\n')

        def run(case, expected=(), absent=(), cwd=project, flags=('skills', 'list', '--all')):
            argv = ['/usr/bin/sandbox-exec', '-f', str(profile), node, str(binary), *flags]
            completed = subprocess.run(argv, cwd=cwd, env=env, capture_output=True, text=True, timeout=45)
            text = completed.stdout + completed.stderr
            (args.evidence / f'{case}.txt').write_text(text)
            assert completed.returncode == 0, (case, completed.returncode, text)
            for needle in expected:
                assert needle in text, (case, 'missing', needle, text)
            for needle in absent:
                assert needle not in text, (case, 'unexpected', needle, text)
            cases.append({'case': case, 'argv': argv, 'cwd': str(cwd), 'exit': completed.returncode,
                          'expected': list(expected), 'absent': list(absent), 'artifact': f'{case}.txt'})
            return text

        version = run('version', expected=('0.63.0',), flags=('--version',)).strip()
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
        run('registered-extension-only', expected=('extension-skill [Enabled]',), absent=('stale-skill [Enabled]',))
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
        receipt = {'version': version, 'package_sha256': hashlib.sha256(binary.read_bytes()).hexdigest(),
                   'node_version': subprocess.check_output([node, '--version'], text=True).strip(),
                   'credential_free': True, 'network': 'sandbox-exec denies network* for every native command',
                   'no_execution': 'skills list only; inert extensions have no hooks/MCP servers; no prompts or skill activation',
                   'cases': cases}
        (args.evidence / 'receipt.json').write_text(json.dumps(receipt, indent=2) + '\n')
        print(json.dumps({'version': version, 'cases': len(cases), 'receipt': str(args.evidence / 'receipt.json')}))


if __name__ == '__main__':
    main()
