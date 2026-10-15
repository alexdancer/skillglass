#!/usr/bin/env python3
"""Reproduce the Codex 0.155.1 project-control limitation without inference."""
import argparse
import hashlib
import json
import pathlib
import shutil
import subprocess
import tempfile
from types import SimpleNamespace

from codex_containment import CodexContainment
from codex_probe import _Server, override


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--codex', default=shutil.which('codex'))
    parser.add_argument('--product', type=pathlib.Path, required=True)
    parser.add_argument('--isolation-probe', type=pathlib.Path, required=True)
    parser.add_argument('--out', type=pathlib.Path, required=True)
    args = parser.parse_args()
    if not args.codex:
        parser.error('Codex unavailable; supply --codex')
    out = args.out.resolve()
    out.mkdir(parents=True, exist_ok=True)
    root = pathlib.Path(tempfile.mkdtemp(prefix='ale28-project-', dir='/private/tmp')).resolve()
    home, a, b = root / 'home', root / 'project-a', root / 'project-b'
    ch = home / '.codex'
    for path in (ch, a, b):
        path.mkdir(parents=True)
    native, product = pathlib.Path(args.codex).resolve(), args.product.resolve()
    env = {'HOME': str(home), 'CODEX_HOME': str(ch), 'PATH': '/usr/bin:/bin:/usr/sbin:/sbin',
           'TMPDIR': str(root), 'GOTMPDIR': str(root), 'XDG_CONFIG_HOME': str(home / '.config'),
           'RUST_LOG': 'error', 'GIT_CONFIG_NOSYSTEM': '1', 'GIT_CONFIG_GLOBAL': '/dev/null'}
    boundary = CodexContainment(root, a, env, native, product, args.isolation_probe, out)
    boundary.initialize_git(a)
    # Controller fixture setup, outside payload profiles, as in the global harness.
    git = ['/usr/bin/git', '-c', 'init.templateDir=' + str(root / 'git-template'), 'init', '-q', str(b)]
    subprocess.run(git, cwd=a, env=boundary.environment, check=True)
    results = {'root': str(root), 'native': str(native), 'nativeSHA256': hashlib.sha256(native.read_bytes()).hexdigest(),
               'productSHA256': hashlib.sha256(product.read_bytes()).hexdigest(), 'invocations': boundary.calls,
               'controllerSecondGit': git, 'cases': [], 'productCommands': [], 'checks': {}}
    checks = results['checks']
    probe = SimpleNamespace(base=root, cwd=a, env=boundary.environment, command=boundary.native_command, results=results)
    config, project = ch / 'config.toml', a / '.codex/config.toml'
    project.parent.mkdir()
    target = home / '.agents/skills/target/SKILL.md'
    target.parent.mkdir(parents=True)
    target.write_text('---\nname: project-control\ndescription: Inert native fixture.\n---\nDo not execute.\n')
    other = home / '.agents/skills/other/SKILL.md'
    other.parent.mkdir()
    other.write_text(target.read_text())
    trust = ''.join('\n[projects.' + json.dumps(str(p)) + ']\ntrust_level="trusted"\n' for p in (a, b))
    head = '[analytics]\nenabled=false\n[feedback]\nenabled=false\n'

    def inspect(label, global_rule='', project_rule=''):
        config.write_text(head + trust + global_rule)
        project.write_text(project_rule)
        server = _Server(probe, ())
        try:
            listing = server.call('skills/list', {'cwds': [str(a), str(b)], 'forceReload': True})
            effective = server.call('config/read', {'cwd': str(a), 'includeLayers': True})
            results['cases'].append({'label': label, 'global': config.read_text(), 'project': project.read_text(),
                                     'skills': listing, 'effectiveConfig': effective})
            return {r['cwd']: {s['path']: s['enabled'] for s in r['skills']} for r in listing['result']['data']}
        finally:
            server.close()

    try:
        results['version'] = subprocess.check_output(boundary.command(native, ['--version'], False), cwd=a, env=boundary.environment, text=True).strip()
        assert results['version'] == 'codex-cli 0.155.1', results['version']
        state = inspect('global-manifest-positive-control', override(target))
        checks['global-exact-target-and-independent-copy'] = all(not state[str(p)][str(target)] and state[str(p)][str(other)] for p in (a, b))
        state = inspect('trusted-project-disable', '', override(target))
        checks['trusted-project-disable-in-config-read'] = results['cases'][-1]['effectiveConfig']['result']['config']['skills']['config'] == [{'path': str(target), 'enabled': False}]
        checks['fresh-project-disable-not-observed'] = all(state[str(p)][str(target)] for p in (a, b))
        state = inspect('trusted-project-enable-global-deny', override(target), override(target, True))
        checks['trusted-project-enable-in-config-read'] = results['cases'][-1]['effectiveConfig']['result']['config']['skills']['config'] == [{'path': str(target), 'enabled': True}]
        checks['fresh-project-enable-not-observed'] = all(not state[str(p)][str(target)] for p in (a, b))
        state = inspect('unrelated-project-row-masks-lower-array', override(target), override(other))
        checks['project-array-replaces-global-array-in-config-read'] = results['cases'][-1]['effectiveConfig']['result']['config']['skills']['config'] == [{'path': str(other), 'enabled': False}]
        checks['unrelated-project-row-still-retains-startup-target-deny'] = not state[str(a)][str(target)]
        state = inspect('removed-project-override', override(target))
        checks['removed-array-inherits-global'] = all(not state[str(p)][str(target)] for p in (a, b))
        before = {p: p.read_bytes() for p in (config, project, target, other)}
        listed = subprocess.run(boundary.command(product, ['list', '--agent', 'codex', '--project', str(a), '--json'], False), cwd=a, env=boundary.environment, capture_output=True, text=True)
        assert listed.returncode == 0, listed.stderr
        source = next(s['id'] for s in json.loads(listed.stdout)['sources'] if s['source_path'] == str(target.parent))
        for action in ('enable', 'disable', 'inherit'):
            argv = [action, '--agent', 'codex', '--scope', 'project', '--project', str(a), '--json', source]
            result = subprocess.run(boundary.command(product, argv, False), cwd=a, env=boundary.environment, capture_output=True, text=True)
            results['productCommands'].append({'argv': argv, 'exit': result.returncode, 'stdout': result.stdout, 'stderr': result.stderr})
            receipt = json.loads(result.stdout)
            checks['product-' + action + '-refuses'] = result.returncode == 1 and receipt['state'] == 'unsupported' and not receipt.get('confirmation')
        checks['project-refusals-preserve-inputs'] = all(p.read_bytes() == raw for p, raw in before.items())
        assert all(checks.values()), checks
    except BaseException as error:
        results['failure'] = repr(error)
        raise
    finally:
        results['stderr'] = {p.name: p.read_text() for p in root.glob('stderr-*')}
        (out / 'result.json').write_text(json.dumps(results, indent=2) + '\n')
        print(json.dumps({'version': results.get('version'), 'checks': checks, 'failure': results.get('failure')}, indent=2))


if __name__ == '__main__':
    main()
