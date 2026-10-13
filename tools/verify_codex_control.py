#!/usr/bin/env python3
"""Prove product global Codex writes with disposable offline native inspection."""
import argparse, hashlib, json, os, pathlib, shlex, shutil, subprocess, tempfile
from codex_probe import _Server
from types import SimpleNamespace
parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--codex', default=shutil.which('codex'))
parser.add_argument('--product', type=pathlib.Path, required=True)
parser.add_argument('--out', type=pathlib.Path, required=True)
options = parser.parse_args()
if not options.codex:
    parser.error('Codex binary unavailable; supply --codex')
OUT = options.out.resolve()
OUT.mkdir(parents=True, exist_ok=True)
PRODUCT = options.product.resolve()
ORIGINAL_HOME = pathlib.Path(os.environ['HOME']).resolve()
BASE = pathlib.Path(tempfile.mkdtemp(prefix='ale-27-native-', dir='/private/tmp')).resolve()
FIXTURE_HOME = BASE / 'home'
CODEX_DIR = FIXTURE_HOME / '.codex'
REPO = BASE / 'repo'
CWD = REPO / 'nested'
for p in (CODEX_DIR, CWD):
    p.mkdir(parents=True)
FIXTURE_ENV = {'HOME': str(FIXTURE_HOME), 'CODEX_HOME': str(CODEX_DIR), 'PATH': '/usr/bin:/bin:/usr/sbin:/sbin', 'TMPDIR': str(BASE), 'XDG_CONFIG_HOME': str(FIXTURE_HOME / '.config'), 'RUST_LOG': 'error'}
NATIVE = pathlib.Path(options.codex).resolve()

def q(p):
    return json.dumps(str(p))
PROFILE = BASE / 'native.sb'
PROFILE.write_text('(version 1)\n(allow default)\n(deny network*)\n(deny file-write* (require-not (subpath ' + q(BASE) + ')))\n(deny file-read-data (require-all (subpath ' + q(ORIGINAL_HOME) + ') (require-not (require-any (literal ' + q(NATIVE) + ') (literal ' + q(PRODUCT) + ')))))\n(deny file-read-data (subpath "/etc/codex"))\n')
WRAPPER = BASE / 'codex-isolated'
WRAPPER.write_text('#!/bin/sh\nexec /usr/bin/sandbox-exec -f ' + shlex.quote(str(PROFILE)) + ' ' + shlex.quote(str(NATIVE)) + ' "$@"\n')
WRAPPER.chmod(448)
subprocess.run(['/usr/bin/git', 'init', '-q', str(REPO)], env=FIXTURE_ENV, check=True)
RESULTS = {'root': str(BASE), 'native': str(NATIVE), 'environment': FIXTURE_ENV, 'profile': PROFILE.read_text(), 'cases': []}
VERSION = subprocess.check_output([str(WRAPPER), '--version'], env=FIXTURE_ENV, text=True).strip()
RESULTS['version'] = VERSION
assert VERSION == 'codex-cli 0.155.1', VERSION
PROBE = SimpleNamespace(base=BASE, cwd=CWD, env=FIXTURE_ENV, binary=str(WRAPPER), results=RESULTS)

def skill(root, folder, name):
    d = root / folder
    d.mkdir(parents=True)
    m = d / 'SKILL.md'
    m.write_text('---\nname: ' + name + '\ndescription: Offline control proof.\n---\nDo not execute.\n')
    return m
RESULTS['product'] = str(PRODUCT)
RESULTS['productSHA256'] = hashlib.sha256(PRODUCT.read_bytes()).hexdigest()
RESULTS['nativeSHA256'] = hashlib.sha256(NATIVE.read_bytes()).hexdigest()
RESULTS['productCommands'] = []
CONFIG = CODEX_DIR / 'config.toml'
HEAD = '# untouched native product proof\nmodel = "offline-fixture"\n[analytics]\nenabled = false\n[feedback]\nenabled = false\n'

def rule(path, enabled):
    return '\n[[skills.config]]\npath = ' + q(path) + '\nenabled = ' + str(enabled).lower() + '\n'

def inspect(label):
    s = _Server(PROBE, ())
    try:
        listing = s.call('skills/list', {'cwds': [str(CWD)], 'forceReload': True})
        config = s.call('config/read', {'cwd': str(CWD), 'includeLayers': True})
        RESULTS['cases'].append({'label': label, 'skills': listing, 'effectiveConfig': config, 'persisted': CONFIG.read_text()})
        return {x['path']: x for row in listing['result']['data'] for x in row['skills']}
    finally:
        s.close()

def product(args, want=0):
    argv = ['/usr/bin/sandbox-exec', '-f', str(PROFILE), str(PRODUCT), *args]
    p = subprocess.run(argv, cwd=CWD, env=FIXTURE_ENV, text=True, capture_output=True)
    RESULTS['productCommands'].append({'argv': argv, 'exit': p.returncode, 'stdout': p.stdout, 'stderr': p.stderr})
    assert p.returncode == want, (args, p.returncode, p.stdout, p.stderr)
    return json.loads(p.stdout)

def source(path):
    r = product(['list', '--agent', 'codex', '--project', str(CWD), '--json'])
    return next((s['id'] for s in r['sources'] if s['source_path'] == str(path.parent)))

def change(choice, path, want=0):
    id = source(path)
    args = [choice, '--agent', 'codex', '--scope', 'global', '--project', str(CWD), '--json']
    p = product([*args, id])
    assert p['state'] == 'ready', p
    out = product([*args, '--confirm', p['confirmation'], id], want)
    assert out['state'] == 'applied' and out['persistence'] == 'verified', out
    return out
A = skill(FIXTURE_HOME / '.agents/skills', 'target', 'same-name')
B = skill(FIXTURE_HOME / '.agents/skills', 'independent', 'same-name')
P = skill(REPO / '.agents/skills', 'project-source', 'project-source')
SHARED = skill(BASE / 'shared', 'source', 'shared-source')
ALIAS = FIXTURE_HOME / '.agents/skills/alias'
ALIAS.symlink_to(SHARED.parent)
trust = '\n[projects.' + q(REPO) + ']\ntrust_level = "trusted"\n'
CONFIG.write_text(HEAD + trust)
checks = {}
baseline = inspect('baseline')
hashes = {str(p): hashlib.sha256(p.read_bytes()).hexdigest() for p in (A, B, P, SHARED)}
change('disable', A)
r = inspect('product-disable-global')
checks['product-canonical-disable-source-only'] = r[str(A)]['enabled'] is False and r[str(B)]['enabled'] is True
change('enable', A)
checks['product-canonical-enable'] = inspect('product-enable-global')[str(A)]['enabled'] is True
change('disable', SHARED)
checks['product-shared-alias-disable'] = inspect('product-disable-alias')[str(SHARED)]['enabled'] is False
change('enable', SHARED)
checks['product-shared-alias-enable'] = inspect('product-enable-alias')[str(SHARED)]['enabled'] is True
projectConfig = REPO / '.codex/config.toml'
projectConfig.parent.mkdir(exist_ok=True)
projectConfig.write_text('# retained project settings\n')
projectBytes = projectConfig.read_bytes()
change('disable', P)
checks['product-project-origin-global-disable'] = inspect('product-disable-project-source')[str(P)]['enabled'] is False
change('enable', P)
checks['product-project-origin-global-enable'] = inspect('product-enable-project-source')[str(P)]['enabled'] is True
checks['project-config-unchanged'] = projectConfig.read_bytes() == projectBytes
CONFIG.write_text(HEAD + trust + '\n[skills]\nconfig = [{ path = ' + q(B) + ', enabled = false, note = "keep" }] # retained\n')
change('disable', A)
r = inspect('product-inline-append')
checks['product-inline-scope'] = not r[str(A)]['enabled'] and (not r[str(B)]['enabled'])
checks['product-inline-comment-retained'] = 'note = "keep" }' in CONFIG.read_text() and '] # retained\n' in CONFIG.read_text()
change('enable', A)
r = inspect('product-inline-enable')
checks['product-inline-enable-preserves-independent-deny'] = r[str(A)]['enabled'] and (not r[str(B)]['enabled'])
PLUGIN = REPO / 'plugins/probe'
PS = skill(PLUGIN / 'skills', 'managed', 'managed')
(PLUGIN / '.codex-plugin').mkdir()
(PLUGIN / '.codex-plugin/plugin.json').write_text(json.dumps({'name': 'probe', 'version': '1.0.0', 'skills': './skills/'}))
market = REPO / '.agents/plugins/marketplace.json'
market.parent.mkdir(parents=True)
market.write_text(json.dumps({'name': 'probe-market', 'plugins': [{'name': 'probe', 'source': {'source': 'local', 'path': './plugins/probe'}, 'policy': {'installation': 'AVAILABLE', 'authentication': 'ON_INSTALL'}, 'category': 'Productivity'}]}))
CONFIG.write_text(HEAD + trust)
s = _Server(PROBE, ())
try:
    RESULTS['pluginInstall'] = s.call('plugin/install', {'pluginName': 'probe', 'marketplacePath': str(market)})
    assert 'result' in RESULTS['pluginInstall'], RESULTS['pluginInstall']
finally:
    s.close()
CONFIG.write_text(HEAD + trust + '\n[plugins."probe@probe-market"]\nenabled = true\n')
CACHE = CODEX_DIR / 'plugins/cache/probe-market/probe/1.0.0/skills/managed/SKILL.md'
pluginBytes = {str(p): p.read_bytes() for p in (PS, CACHE, market, PLUGIN / '.codex-plugin/plugin.json')}
inspect('managed-baseline')
change('disable', CACHE)
r = inspect('product-managed-disable')
checks['product-managed-per-skill-disable'] = not r[str(CACHE)]['enabled']
change('enable', CACHE)
checks['product-managed-per-skill-enable'] = inspect('product-managed-enable')[str(CACHE)]['enabled']
checks['managed-content-registration-unchanged'] = all((p.read_bytes() == raw for key, raw in pluginBytes.items() for p in [pathlib.Path(key)]))
checks['global-sources-unchanged'] = hashes == {str(p): hashlib.sha256(p.read_bytes()).hexdigest() for p in (A, B, P, SHARED)}
checks['unrelated-native-settings-retained'] = CONFIG.read_text().startswith(HEAD + trust + '\n[plugins."probe@probe-market"]\nenabled = true\n')
SYSTEM = next((pathlib.Path(path) for path in baseline if '/skills/.system/openai-docs/' in path))
systemBytes = SYSTEM.read_bytes()
change('disable', SYSTEM)
checks['product-bundled-per-skill-disable'] = not inspect('product-bundled-disable')[str(SYSTEM)]['enabled']
change('enable', SYSTEM)
checks['product-bundled-per-skill-enable'] = inspect('product-bundled-enable')[str(SYSTEM)]['enabled']
checks['bundled-content-unchanged'] = SYSTEM.read_bytes() == systemBytes
storage = BASE / 'codex-storage'
CODEX_DIR.rename(storage)
CODEX_DIR.symlink_to(storage, target_is_directory=True)
change('disable', A)
checks['product-config-root-alias-disable'] = not inspect('product-root-alias-disable')[str(A)]['enabled']
change('enable', A)
checks['product-config-root-alias-enable'] = inspect('product-root-alias-enable')[str(A)]['enabled']
RESULTS['checks'] = checks
RESULTS['stderr'] = {p.name: p.read_text() for p in BASE.glob('stderr-*')}
(OUT / 'native-output.json').write_text(json.dumps(RESULTS, indent=2) + '\n')
(OUT / 'native-checks.json').write_text(json.dumps(checks, indent=2) + '\n')
print(json.dumps({'version': VERSION, 'root': str(BASE), 'checks': checks}, indent=2))
assert all(checks.values()), checks
