"""Verify versioned Hermes control policy inside disposable OS-contained fixtures."""
import argparse, hashlib, json, os, subprocess, tempfile
from pathlib import Path
parser = argparse.ArgumentParser(description='Contained Hermes 0.20.0 control-policy validation, without CLI/session startup')
parser.add_argument('source', type=Path)
parser.add_argument('output', type=Path)
args = parser.parse_args()
src = args.source.resolve(strict=True)
out = args.output.resolve()
out.mkdir(parents=True, exist_ok=False)
probe_source = Path(__file__).with_name('hermes_control_probe.py')
py = src / 'venv/bin/python'
real = py.resolve(strict=True)
f = Path(tempfile.mkdtemp(prefix='ale32-native-', dir='/private/tmp')).resolve()

def sha(p):
    return hashlib.sha256(p.read_bytes()).hexdigest()
head = subprocess.check_output(['/usr/bin/git', '-C', str(src), 'rev-parse', 'HEAD'], text=True).strip()
status = subprocess.check_output(['/usr/bin/git', '-C', str(src), 'status', '--porcelain'], text=True)
assert head == '5a16635f409e0f5e3d754bb6256bd27707bcec4c' and (not status)
for d in ['home/.hermes/profiles/research', 'home/.hermes/profiles/other', 'project/.hermes', 'tmp']:
    (f / d).mkdir(parents=True)
(f / 'home/.hermes/profiles/research/config.yaml').write_text('unrelated_setting: keep\nskills:\n  disabled: [unrelated]\n  platform_disabled:\n    telegram: [platform-denied]\n  external_dirs: []\n')
(f / 'home/.hermes/profiles/other/config.yaml').write_text('skills:\n  disabled: [other-only]\n')
for p in ['project/config.yaml', 'project/.hermes/config.yaml', 'home/.hermes/config.yaml']:
    (f / p).write_text('skills:\n  disabled: [target, root-only]\n')
for name in ['target', 'alias-dir', 'long-dir']:
    d = f / 'home/.hermes/profiles/research/skills' / name
    d.mkdir(parents=True)
    logical = {'target': 'target', 'alias-dir': 'metadata-name', 'long-dir': 'L' * 70}[name]
    (d / 'SKILL.md').write_text('---\nname: ' + logical + '\nmetadata:\n  hermes:\n    config:\n      - key: ' + name + '.probe\n        description: inert native validation\n---\nThis fixture is data only.\n')
excluded = f.parent / (f.name + '-excluded')
excluded.write_text('owned canary')
(f / 'probe.py').write_bytes(probe_source.read_bytes())
files = ['hermes_constants.py', 'agent/skill_utils.py', 'gateway/session_context.py', 'tools/skills_tool.py', 'agent/prompt_builder.py', 'hermes_cli/skills_config.py', 'hermes_cli/__init__.py']
q = lambda p: json.dumps(str(p))
policy = '(version 1)\n(allow default)\n(deny network*)\n(deny process-fork)\n(deny process-exec (require-not (require-any (literal %s) (literal %s))))\n(deny file-write* (require-not (subpath %s)))\n(deny file-read-data (require-not (require-any (subpath %s) (subpath %s) (subpath %s) (subpath "/System") (subpath "/usr/lib") (subpath "/dev") (literal "/") %s)))\n(deny file-read* (subpath "/etc/hermes") (subpath "/private/etc/hermes") (subpath "/etc/codex") (subpath "/private/etc/codex") (with errno ENOENT))\n' % (q(py), q(real), q(f), q(f), q(src / 'venv'), q(src / '.hermes-runtime'), ' '.join(('(literal ' + q(src / n) + ')' for n in files)))
(f / 'policy.sb').write_text(policy)
(out / 'policy.sb').write_text(policy)
env = {'HOME': str(f / 'home'), 'HERMES_HOME': str(f / 'home/.hermes/profiles/research'), 'HERMES_PLATFORM': 'telegram', 'SOURCE': str(src), 'FIXTURE': str(f), 'EXCLUDED': str(excluded), 'OUTSIDE': str(f.parent / (f.name + '-forbidden')), 'PATH': '/usr/bin:/bin', 'TMPDIR': str(f / 'tmp'), 'PYTHONDONTWRITEBYTECODE': '1', 'HERMES_SKIP_NODE_BOOTSTRAP': '1'}
receipt = {'source_head': head, 'source_status': status, 'source_hashes': {n: sha(src / n) for n in files}, 'runtime': str(real), 'runtime_sha256': sha(real), 'helper_sha256': sha(probe_source), 'runner_sha256': sha(Path(__file__)), 'policy_sha256': sha(f / 'policy.sb'), 'fixture': str(f), 'environment': env, 'argv': ['/usr/bin/sandbox-exec', '-f', str(f / 'policy.sb'), str(py), '-S', str(f / 'probe.py')]}
env['PYTHONPATH'] = str(src / 'venv/lib/python3.11/site-packages')
sites = list((src / 'venv/lib').glob('python*/site-packages'))
assert len(sites) == 1
env['PYTHONPATH'] = str(sites[0])
(out / 'prelaunch.json').write_text(json.dumps(receipt, indent=2) + '\n')
r = subprocess.run(receipt['argv'], env=env, cwd=f / 'project', capture_output=True, text=True, timeout=30)
(out / 'stdout.json').write_text(r.stdout)
(out / 'stderr.txt').write_text(r.stderr)
receipt['exit'] = r.returncode
receipt['source_hashes_after'] = {n: sha(src / n) for n in files}
receipt['source_unchanged'] = receipt['source_hashes'] == receipt['source_hashes_after']
receipt['canary_unchanged'] = excluded.read_text() == 'owned canary'
receipt['runtime_sha256_after'] = sha(real)
(out / 'receipt.json').write_text(json.dumps(receipt, indent=2) + '\n')
print(json.dumps({'exit': r.returncode, 'fixture': str(f), 'stderr': r.stderr}))
assert r.returncode == 0
assert receipt['source_unchanged'] and receipt['canary_unchanged'] and (receipt['runtime_sha256'] == receipt['runtime_sha256_after'])
