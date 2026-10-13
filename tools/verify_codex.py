#!/usr/bin/env python3
"""Offline Codex 0.155.1 probes using a fully disposable home and project."""
import json
from codex_probe import Probe, override

probe = Probe(__doc__)
OUT, BASE = probe.out, probe.base
FIXTURE_HOME, CH = probe.home, probe.codex_home
REPO, CWD = probe.repo, probe.cwd
RESULTS = probe.results
skill, config, case, Server = probe.skill, probe.config, probe.case, probe.server

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

# Native user-root routes into managed caches load independently of plugin activation.
version = cachedskill.parents[2]
outside = skill(version / 'other', 'outside-declaration')
orphan = skill(CH / 'plugins/cache/orphan/orphan-plugin/old/skills', 'orphan-alias')
userroot = FIXTURE_HOME / '.agents/skills'
for name, target in [('registered-cache-alias', cachedskill.parent), ('outside-cache-alias', outside.parent), ('orphan-cache-alias', orphan.parent)]:
    (userroot / name).symlink_to(target)
disabled = trust + '[plugins."probe-plugin@probe-market"]\nenabled = false\n'
case('disabled-plugin-with-user-aliases', disabled)
case('disabled-plugin-canonical-manifest-denied', disabled + override(cachedskill))
case('disabled-plugin-alias-manifest-denied', disabled + override(userroot / 'registered-cache-alias/SKILL.md'))
case('enabled-plugin-with-user-aliases', trust + '[plugins."probe-plugin@probe-market"]\nenabled = true\n')

# A local source outside the installed cache gains the same ancestor-manifest namespace.
(userroot / 'local-source-alias').symlink_to(plugskill.parent)
case('disabled-plugin-with-source-alias', disabled)
case('source-alias-cached-manifest-denied', disabled + override(cachedskill))
case('enabled-plugin-with-source-alias', trust + '[plugins."probe-plugin@probe-market"]\nenabled = true\n')

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
    'disabled plugin user alias still eligible': named('disabled-plugin-with-user-aliases', 'probe-plugin:plugin-skill')[0]['enabled'] is True,
    'outside declared root alias uses manifest namespace': named('disabled-plugin-with-user-aliases', 'probe-plugin:outside-declaration')[0]['enabled'] is True,
    'orphan user alias remains plain and eligible': named('disabled-plugin-with-user-aliases', 'orphan-alias')[0]['enabled'] is True,
    'canonical selector disables independent alias route': named('disabled-plugin-canonical-manifest-denied', 'probe-plugin:plugin-skill')[0]['enabled'] is False,
    'alias selector disables independent alias route': named('disabled-plugin-alias-manifest-denied', 'probe-plugin:plugin-skill')[0]['enabled'] is False,
    'outside-cache source alias uses ancestor namespace': any(s['path'] == str(plugskill) and s['enabled'] for s in named('disabled-plugin-with-source-alias', 'probe-plugin:plugin-skill')),
    'independent source and cache aliases coexist natively': len(named('disabled-plugin-with-source-alias', 'probe-plugin:plugin-skill')) == 2,
    'cached manifest denial leaves independent source enabled': {s['path']: s['enabled'] for s in named('source-alias-cached-manifest-denied', 'probe-plugin:plugin-skill')} == {str(plugskill): True, str(cachedskill): False},
}
(OUT / 'native-checks.json').write_text(json.dumps(checks, indent=2) + '\n')
print('Saved', OUT / 'native-output.json')
print(json.dumps(checks, indent=2))
if not all(checks.values()):
    raise SystemExit('Native behavior changed; inspect failed checks and raw output.')
