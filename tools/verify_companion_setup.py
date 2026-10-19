#!/usr/bin/env python3
"""Observe installed companion catalogs in disposable macOS native applications.

No prompt, activation, inference, hook or skill workload is requested. All child
settings, homes and projects are disposable; native network access is denied.
"""
import argparse
import hashlib
import json
from pathlib import Path
import shutil
import tempfile

import codex_containment
from codex_probe import _Server
from claude_containment import NativeContainment
from verify_claude import initialize
from native_containment import native_profile
from native_receipts import recorded_run


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--product', required=True, type=Path)
    parser.add_argument('--probe', required=True, type=Path)
    parser.add_argument('--codex', required=True, type=Path)
    parser.add_argument('--claude', required=True, type=Path)
    parser.add_argument('--opencode', required=True, type=Path)
    parser.add_argument('--out', required=True, type=Path)
    args = parser.parse_args()
    out = args.out.resolve()
    out.mkdir()  # Fresh evidence only; never replace an earlier run.
    root = Path(tempfile.mkdtemp(prefix='skillglass-setup-native-', dir='/private/tmp')).resolve()
    home, cwd = root / 'home', root / 'project'
    home.mkdir()
    cwd.mkdir()
    product, native, probe, claude, original_opencode = [p.resolve(strict=True) for p in
        (args.product, args.codex, args.probe, args.claude, args.opencode)]
    if product.name != 'skillglass':
        raise RuntimeError('product must be named skillglass for its documented PATH lookup')
    env = {
        'HOME': str(home), 'CODEX_HOME': str(home / '.codex'),
        'CLAUDE_CONFIG_DIR': str(home / '.claude'),
        'PATH': str(product.parent) + ':/usr/bin:/bin', 'TMPDIR': str(root),
        'XDG_CONFIG_HOME': str(home / '.config'),
        'XDG_DATA_HOME': str(home / '.local/share'),
        'XDG_CACHE_HOME': str(home / '.cache'), 'XDG_STATE_HOME': str(home / '.state'),
        'GIT_CONFIG_NOSYSTEM': '1', 'GIT_CONFIG_GLOBAL': str(home / 'missing-git'),
        'GIT_CONFIG_SYSTEM': str(home / 'missing-git'), 'RUST_LOG': 'error',
    }
    # Pin the supplied installed bytes before running the existing fail-closed
    # controls. The old helper's version claim is not inherited by this probe.
    codex_containment.PINNED_NATIVE_SHA256 = digest(native)
    codex_out = out / 'codex'
    codex_out.mkdir()
    boundary = codex_containment.CodexContainment(root, cwd, env, native, product, probe, codex_out)
    receipts = []

    def run(label, argv, binary, profile):
        result, receipt = recorded_run(out, label, argv, cwd=cwd, env=env,
            binary=binary, profile=profile, timeout=40)
        receipts.append(receipt)
        (out / (label + '.stdout')).write_bytes(result.stdout)
        (out / (label + '.stderr')).write_bytes(result.stderr)
        if receipt['timed_out']:
            raise RuntimeError(label + ' timed out; owned child group stopped')
        return result

    version = run('codex-version', boundary.native_command(['--version']), native,
                  boundary.profiles[True])
    version.check_returncode()
    assert version.stdout.decode().strip() == 'codex-cli 0.162.1'
    install = run('setup', boundary.command(product, [
        'setup', '--app', 'codex', '--app', 'claude-code', '--app', 'opencode',
        '--allow-shared-app', 'codeaf', '--install', '--json']), product,
        boundary.profiles[True])
    report = json.loads(install.stdout)
    assert install.returncode == 0 and [r['state'] for r in report['results']] == ['installed'] * 3, report
    emitted = run('bundle', boundary.command(product, ['companion', '--json']), product,
                  boundary.profiles[False])
    emitted.check_returncode()
    bundle = json.loads(emitted.stdout)
    installed = {r['application']: Path(r['destination']) for r in report['results']}

    def bundle_manifest():
        result = {}
        for app, destination in installed.items():
            for file in bundle['files']:
                path = destination / file['path']
                raw = path.read_bytes()
                assert raw == file['content'].encode(), path
                result[app + '/' + file['path']] = digest(path)
        return result

    before = bundle_manifest()

    class Probe:
        base, binary, results = root, native, {}

        def command(self, arguments):
            return boundary.native_command(arguments)

    probe_context = Probe()
    probe_context.cwd, probe_context.env = cwd, env
    server = _Server(probe_context, [])
    try:
        catalog = server.call('skills/list', {'cwds': [str(cwd)], 'forceReload': True})
    finally:
        server.close()
    assert str(installed['codex'] / 'SKILL.md') in json.dumps(catalog), catalog
    (out / 'codex-catalog.json').write_text(json.dumps({
        'version': version.stdout.decode(), 'catalog': catalog,
        'transcripts': probe_context.results, 'exit_code': server.process.returncode}, indent=2))

    # Independent helpers own the same control-link name; the first child has
    # ended before replacing this disposable control for the second boundary.
    (root / 'escape-link').unlink()
    claude_out = out / 'claude'
    claude_out.mkdir()
    claude_boundary = NativeContainment(root, cwd, env, claude, claude_out)
    claude_version = run('claude-version', claude_boundary.command(['--version']),
                         claude, claude_boundary.profile)
    claude_version.check_returncode()
    assert claude_version.stdout.decode().strip() == '2.1.303 (Claude Code)'
    raw, stderr, commands, exit_code = initialize(claude_boundary.command([
        '--print', '--input-format', 'stream-json', '--output-format', 'stream-json',
        '--verbose']), cwd, env)
    assert any(command.get('name') == 'skillglass-review' for command in commands['commands']), commands
    (out / 'claude-catalog.json').write_text(json.dumps({
        'version': claude_version.stdout.decode(), 'raw': raw, 'stderr': stderr,
        'exit_code': exit_code, 'catalog': commands}, indent=2))

    immutable = Path(tempfile.mkdtemp(prefix='skillglass-setup-opencode-', dir='/private/tmp')).resolve()
    opencode = immutable / 'opencode'
    shutil.copyfile(original_opencode, opencode)
    opencode.chmod(0o500)
    assert digest(opencode) == digest(original_opencode)
    policy = root / 'opencode.sb'
    policy.write_text(native_profile(root, opencode) +
        '\n(deny process-exec (require-not (literal ' + json.dumps(str(opencode)) + ')))\n')
    env.update({
        'OPENCODE_DISABLE_AUTOUPDATE': 'true', 'OPENCODE_DISABLE_MODELS_FETCH': 'true',
        'OPENCODE_DISABLE_DEFAULT_PLUGINS': 'true',
        'OPENCODE_TEST_MANAGED_CONFIG_DIR': str(root / 'managed'), 'OPENCODE_PURE': 'true',
    })
    prefix = ['/usr/bin/sandbox-exec', '-f', str(policy), str(opencode), '--pure']
    opencode_version = run('opencode-version', prefix + ['--version'], opencode, policy)
    opencode_version.check_returncode()
    assert opencode_version.stdout.decode().strip() == '1.17.15'

    def opencode_catalog(label, expected):
        assert digest(opencode) == digest(original_opencode)
        result = run(label, prefix + ['debug', 'skill'], opencode, policy)
        result.check_returncode()
        entries = json.loads(result.stdout)
        assert any(e['name'] == 'skillglass-review' and
                   Path(e['location']) == expected / 'SKILL.md' for e in entries), entries

    # Isolate each same-name route instead of inferring both from one concurrent
    # native winner. These driver-only moves touch disposable exact bundles.
    held_claude, held_opencode = root / 'held-claude', root / 'held-opencode'
    installed['claude-code'].rename(held_claude)
    try:
        opencode_catalog('opencode-native-catalog', installed['opencode'])
    finally:
        held_claude.rename(installed['claude-code'])
    installed['opencode'].rename(held_opencode)
    try:
        opencode_catalog('opencode-shared-catalog', installed['claude-code'])
    finally:
        held_opencode.rename(installed['opencode'])
    after = bundle_manifest()
    assert before == after
    summary = {
        'root': str(root), 'product': str(product), 'product_sha256': digest(product),
        'native_sources': {str(p): digest(p) for p in (native, claude, original_opencode)},
        'codex_version': version.stdout.decode().strip(),
        'claude_version': claude_version.stdout.decode().strip(),
        'opencode_version': opencode_version.stdout.decode().strip(),
        'installed': report, 'bundle_before': before, 'bundle_after': after,
        'native_catalogs_all_observed': True, 'no_skill_workloads': True,
        'limits': ['No inference or running-session adoption was tested.',
                   'CodeAF is an acknowledged possible source-loader consumer; its dirty installed build is unverified.',
                   'Default native user homes only; overrides and project-local setup remain unsupported.',
                   'Native catalog protocols use the existing bounded transports; their documented containment limits remain.'],
        'receipts': receipts,
    }
    (out / 'summary.json').write_text(json.dumps(summary, indent=2) + '\n')
    print(out / 'summary.json')


if __name__ == '__main__':
    main()
