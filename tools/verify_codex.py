#!/usr/bin/env python3
"""Offline Codex 0.155.1 app-server probes. All writes stay under tempfile root."""
import argparse, json, os, pathlib, queue, shutil, subprocess, tempfile, threading, time

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--codex', default=shutil.which('codex'))
parser.add_argument('--out', type=pathlib.Path, default=pathlib.Path(__file__).parent)
options = parser.parse_args()
if not options.codex:
    parser.error('Codex binary unavailable; supply --codex')
OUT = options.out.resolve()
OUT.mkdir(parents=True, exist_ok=True)
BASE = pathlib.Path(tempfile.mkdtemp(prefix='ale26-codex-', dir='/tmp'))
FIXTURE_HOME = BASE / 'home'
CH = FIXTURE_HOME / '.codex'
REPO = BASE / 'outside' / 'repo'
CWD = REPO / 'nested' / 'leaf'
for d in (CH, CWD, REPO / '.git'):
    d.mkdir(parents=True, exist_ok=True)
ENV = {'HOME': str(FIXTURE_HOME), 'CODEX_HOME': str(CH), 'PATH': '/usr/bin:/bin:/usr/sbin:/sbin', 'TMPDIR': str(BASE), 'XDG_CONFIG_HOME': str(FIXTURE_HOME / '.config'), 'RUST_LOG': 'error'}
BIN = str(pathlib.Path(options.codex).resolve())
subprocess.run(['/usr/bin/git', 'init', '-q', str(REPO)], env=ENV, check=True)
BASE = BASE.resolve()
FIXTURE_HOME = FIXTURE_HOME.resolve()
CH = CH.resolve()
REPO = REPO.resolve()
CWD = CWD.resolve()
RESULTS = {'root': str(BASE), 'version': subprocess.check_output([BIN, '--version'], env=ENV, text=True).strip(), 'cases': []}

def skill(root, folder, name=None, policy=None):
    d = root / folder
    d.mkdir(parents=True, exist_ok=True)
    p = d / 'SKILL.md'
    p.write_text(f'---\nname: {name or folder}\ndescription: Probe {folder}.\n---\nOffline probe instructions.\n')
    if policy is not None:
        (d / 'agents').mkdir(exist_ok=True)
        (d / 'agents' / 'openai.yaml').write_text(f'policy:\n  allow_implicit_invocation: {str(policy).lower()}\n')
    return p

class Server:
    def __init__(self, flags=()):
        self.stderr = open(BASE / f'stderr-{time.time_ns()}.txt', 'w')
        # Runs inside the caller's network-restricted execution sandbox. No inference calls.
        args = [BIN, 'app-server', '--stdio', *flags]
        self.p = subprocess.Popen(args, cwd=CWD, env=ENV, stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=self.stderr, text=True, bufsize=1)
        self.q = queue.Queue()
        self.messages = []
        threading.Thread(target=lambda: [self.q.put(x) for x in self.p.stdout], daemon=True).start()
        self.n = 0
        self.call('initialize', {'clientInfo': {'name': 'ale26_offline_probe', 'version': '1'}, 'capabilities': {'experimentalApi': True}})
        self.p.stdin.write(json.dumps({'method': 'initialized'}) + '\n'); self.p.stdin.flush()
    def call(self, method, params):
        self.n += 1
        msg = {'id': self.n, 'method': method, 'params': params}
        self.messages.append({'request': msg})
        self.p.stdin.write(json.dumps(msg) + '\n'); self.p.stdin.flush()
        deadline = time.monotonic() + 15
        while time.monotonic() < deadline:
            try: response = json.loads(self.q.get(timeout=max(.01, deadline-time.monotonic())))
            except queue.Empty: raise RuntimeError(f'Timeout for {method}; exit={self.p.poll()}')
            self.messages.append({'response': response})
            if response.get('id') == self.n: return response
        raise RuntimeError(method)
    def close(self):
        self.p.terminate()
        try: self.p.wait(timeout=3)
        except subprocess.TimeoutExpired: self.p.kill(); self.p.wait()
        self.stderr.close()
        RESULTS.setdefault('transcripts', []).append(self.messages)

def config(text=''):
    (CH / 'config.toml').write_text('[analytics]\nenabled = false\n[feedback]\nenabled = false\n' + text)

def case(label, text='', flags=(), extras=None):
    config(text)
    s = Server(flags)
    try:
        if extras is not None: s.call('skills/extraRoots/set', {'extraRoots': [str(x) for x in extras]})
        listing = s.call('skills/list', {'cwds': [str(CWD)], 'forceReload': True})
        effective = s.call('config/read', {'cwd': str(CWD), 'includeLayers': True})
        RESULTS['cases'].append({'label': label, 'config': text, 'flags': list(flags), 'skills': listing, 'effectiveConfig': effective})
        print(label, json.dumps(listing.get('result', listing)))
        return listing
    finally: s.close()

def override(path, enabled=False):
    return f'[[skills.config]]\npath = {json.dumps(str(path))}\nenabled = {str(enabled).lower()}\n'

user = skill(FIXTURE_HOME / '.agents/skills', 'user', 'collision')
legacy = skill(CH / 'skills', 'legacy', 'collision')
root = skill(REPO / '.agents/skills', 'root', 'collision')
nested = skill(REPO / 'nested/.agents/skills', 'nested', 'collision')
leaf = skill(CWD / '.agents/skills', 'leaf', 'collision')
skill(REPO / '.codex/skills', 'project-legacy')
skill(REPO / 'nested/.codex/skills', 'nested-legacy')
skill(CWD / '.codex/skills', 'leaf-legacy')
skill(BASE / 'outside/.agents/skills', 'outside')
skill(CWD / '.agents/skills', 'explicit-only', policy=False)
shared = skill(BASE / 'shared', 'source')
aliasdir = FIXTURE_HOME / '.agents/skills/alias'
aliasdir.symlink_to(shared.parent)
alias = aliasdir / 'SKILL.md'
extra = skill(BASE / 'extra', 'configured-extra')

case('baseline')
case('disable-folder', override(user.parent))
case('disable-manifest', override(user))
case('disable-alias-manifest', override(alias))
case('disable-canonical-manifest', override(shared))
case('disable-canonical-folder', override(shared.parent))
case('extraRoots-api', extras=[extra.parent.parent])
case('candidate-config-skills-paths', '[skills]\npaths = [' + json.dumps(str(extra.parent.parent)) + ']\n')

# Trusted and untrusted config with a different path in each array identifies merge semantics.
projectconfig = REPO / '.codex/config.toml'
projectconfig.parent.mkdir(exist_ok=True)
projectconfig.write_text(override(root))
case('project-untrusted', override(user))
trust = f'[projects.{json.dumps(str(REPO))}]\ntrust_level = "trusted"\n'
case('project-trusted', override(user) + trust)
subconfig = CWD / '.codex/config.toml'
subconfig.parent.mkdir(exist_ok=True)
subconfig.write_text(override(leaf))
case('project-closest-layer', override(user) + trust)
case('cli-override', override(user) + trust, ['-c', 'skills.config=' + '[{path=' + json.dumps(str(nested)) + ',enabled=false}]'])
case('same-path-project-enable', override(user) + trust)
projectconfig.write_text(override(user, True))
case('same-path-project-enable-confirmed', override(user) + trust)
projectconfig.unlink(); subconfig.unlink()

# Multiple discovery aliases of one canonical source are deduplicated natively.
(CH / 'skills/second-alias').symlink_to(shared.parent)
case('shared-multiple-aliases', override(alias))

# Local plugin source, configured inactive cache and orphaned cache.
plugroot = REPO / 'plugins/probe-plugin'
plugskill = skill(plugroot / 'skills', 'plugin-skill')
(plugroot / '.codex-plugin').mkdir()
(plugroot / '.codex-plugin/plugin.json').write_text(json.dumps({'name': 'probe-plugin', 'version': '1.0.0', 'skills': './skills/'}))
marketplace = REPO / '.agents/plugins/marketplace.json'
marketplace.parent.mkdir(parents=True, exist_ok=True)
marketplace.write_text(json.dumps({'name': 'probe-market', 'plugins': [{'name': 'probe-plugin', 'source': {'source': 'local', 'path': './plugins/probe-plugin'}, 'policy': {'installation': 'AVAILABLE', 'authentication': 'ON_INSTALL'}, 'category': 'Productivity'}]}))
skill(CH / 'plugins/cache/orphan/orphan-plugin/old/skills', 'stale-cache')
case('plugin-source-uninstalled', trust)
case('plugin-config-enabled', trust + '[plugins."probe-plugin@probe-market"]\nenabled = true\n')
case('plugin-config-disabled', trust + '[plugins."probe-plugin@probe-market"]\nenabled = false\n')
case('plugin-skill-manifest-disabled', trust + '[plugins."probe-plugin@probe-market"]\nenabled = true\n' + override(plugskill))
config(trust)
s = Server()
try:
    listing = s.call('plugin/list', {'cwds': [str(CWD)], 'marketplaceKinds': ['local'], 'forceRefetch': False})
    installed = s.call('plugin/install', {'pluginName': 'probe-plugin', 'marketplacePath': str(marketplace)})
    observed = s.call('skills/list', {'cwds': [str(CWD)], 'forceReload': True})
    RESULTS['cases'].append({'label': 'plugin-native-install-disposable', 'pluginListing': listing, 'install': installed, 'skills': observed})
finally: s.close()
case('plugin-installed-enabled', trust + '[plugins."probe-plugin@probe-market"]\nenabled = true\n')
case('plugin-installed-disabled', trust + '[plugins."probe-plugin@probe-market"]\nenabled = false\n')
cachedskill = CH / 'plugins/cache/probe-market/probe-plugin/1.0.0/skills/plugin-skill/SKILL.md'
case('plugin-installed-skill-disabled', trust + '[plugins."probe-plugin@probe-market"]\nenabled = true\n' + override(cachedskill))

# A skills/list observation is not evidence that an inference thread adopted changes.
config()
s = Server()
try:
    a = s.call('skills/list', {'cwds': [str(CWD)]})
    config(override(user))
    b = s.call('skills/list', {'cwds': [str(CWD)]})
    c = s.call('skills/list', {'cwds': [str(CWD)], 'forceReload': True})
    RESULTS['cases'].append({'label': 'reload-config-same-server', 'before': a, 'after': b, 'forceReload': c})
finally: s.close()

RESULTS['stderr'] = {p.name: p.read_text() for p in BASE.glob('stderr-*.txt')}
(OUT / 'native-output.json').write_text(json.dumps(RESULTS, indent=2) + '\n')
cases = {c['label']: c for c in RESULTS['cases']}
def found(label):
    return [s for x in cases[label]['skills']['result']['data'] for s in x['skills']]
def named(label, name):
    return [s for s in found(label) if s['name'] == name]
checks = {
    'five independent names coexist': len(named('baseline', 'collision')) == 5,
    'repo ancestor boundary': not named('baseline', 'outside'),
    'folder override ineffective': all(s['enabled'] for s in named('disable-folder', 'collision')),
    'manifest override effective': sum(not s['enabled'] for s in named('disable-manifest', 'collision')) == 1,
    'alias disable canonical source': named('disable-alias-manifest', 'source')[0]['enabled'] is False,
    'canonical disable shared source': named('disable-canonical-manifest', 'source')[0]['enabled'] is False,
    'same canonical aliases deduplicate': len(named('shared-multiple-aliases', 'source')) == 1,
    'configured API extra roots load': bool(named('extraRoots-api', 'configured-extra')),
    'guess skills.paths does not load': not named('candidate-config-skills-paths', 'configured-extra'),
    'source alone cannot activate plugin': not named('plugin-config-enabled', 'probe-plugin:plugin-skill'),
    'native install has plugin provenance': named('plugin-installed-enabled', 'probe-plugin:plugin-skill')[0].get('pluginId') == 'probe-plugin@probe-market',
    'inactive plugin omitted': not named('plugin-installed-disabled', 'probe-plugin:plugin-skill'),
    'stale cache omitted': not named('plugin-installed-enabled', 'stale-cache'),
    'native plugin per-skill path control': named('plugin-installed-skill-disabled', 'probe-plugin:plugin-skill')[0]['enabled'] is False,
}
(OUT / 'native-checks.json').write_text(json.dumps(checks, indent=2) + '\n')
print('Saved', OUT / 'native-output.json')
print(json.dumps(checks, indent=2))
if not all(checks.values()):
    raise SystemExit('Native behavior changed; inspect failed checks and raw output.')
