"""Native lightweight utility probe; run only through verify_hermes_controls.py."""
import hashlib, importlib.util, json, os, socket, subprocess, sys, types
from pathlib import Path
f = Path(os.environ['FIXTURE'])
source = Path(os.environ['SOURCE'])

def sha(p):
    return hashlib.sha256(p.read_bytes()).hexdigest()

def tree():
    return {str(p.relative_to(f)): sha(p) for p in f.rglob('*') if p.is_file()}
checks = {}

def deny(n, fn):
    try:
        fn()
    except PermissionError:
        checks[n] = True
    else:
        checks[n] = False
p = f / 'positive'
p.write_text('yes')
checks['fixture_rw'] = p.read_text() == 'yes'
p.unlink()
deny('excluded_read', lambda: Path(os.environ['EXCLUDED']).read_bytes())
deny('outside_write', lambda: Path(os.environ['OUTSIDE']).write_text('no'))
deny('child', lambda: subprocess.run(['/usr/bin/true']))

def net():
    with socket.socket() as s:
        s.bind(('127.0.0.1', 0))
deny('network', net)
assert all(checks.values()), checks
before = tree()

def load(name, rel):
    spec = importlib.util.spec_from_file_location(name, source / rel)
    m = importlib.util.module_from_spec(spec)
    sys.modules[name] = m
    spec.loader.exec_module(m)
    return m
g = types.ModuleType('gateway')
g.__path__ = []
sys.modules['gateway'] = g
hc = load('hermes_constants', 'hermes_constants.py')
sc = load('gateway.session_context', 'gateway/session_context.py')
u = load('native_skill_utils', 'agent/skill_utils.py')
rows = []

def check(label, want):
    got = sorted(u.get_disabled_skill_names())
    assert got == sorted(want), (label, got, want)
    offered = sorted((x['skill'] for x in u.discover_all_skill_config_vars()))
    assert ('target' in offered) == ('target' not in want and label != 'profile-isolated'), (label, offered)
    rows.append({'config_var_skill_names': offered, 'case': label, 'disabled': got, 'config': str(hc.get_config_path()), 'hashes': tree()})
config = Path(os.environ['HERMES_HOME']) / 'config.yaml'
original = config.read_bytes()
check('baseline-global-plus-telegram', ['unrelated', 'platform-denied'])
changed = original.decode().replace('disabled: [unrelated]', 'disabled: [unrelated, target]')
tmp = config.with_suffix('.tmp')
tmp.write_text(changed)
tmp.replace(config)
check('global-disable', ['unrelated', 'target', 'platform-denied'])
os.environ['HERMES_PLATFORM'] = 'discord'
check('global-disable-discord', ['unrelated', 'target'])
os.environ['HERMES_PLATFORM'] = ''
os.environ['HERMES_SESSION_PLATFORM'] = 'telegram'
check('session-fallback', ['unrelated', 'target', 'platform-denied'])
sc._SESSION_PLATFORM.set('discord')
check('context-platform-overrides-shell', ['unrelated', 'target'])
sc._SESSION_PLATFORM.set('telegram')
check('context-platform-denial', ['unrelated', 'target', 'platform-denied'])
tmp.write_bytes(original)
tmp.replace(config)
check('global-restore', ['unrelated', 'platform-denied'])
changed = original.decode().replace('telegram: [platform-denied]', 'telegram: [platform-denied, target]')
tmp.write_text(changed)
tmp.replace(config)
check('platform-denial-survives-global-enable', ['unrelated', 'platform-denied', 'target'])
tmp.write_bytes(original)
tmp.replace(config)
changed = original.decode().replace('disabled: [unrelated]', 'disabled: [unrelated, alias-dir, ' + 'L' * 64 + ']')
tmp.write_text(changed)
tmp.replace(config)
alias_observed = sorted((x['skill'] for x in u.discover_all_skill_config_vars()))
assert {'metadata-name', 'L' * 70}.issubset(set(alias_observed))
changed = original.decode().replace('disabled: [unrelated]', 'disabled: [unrelated, metadata-name, ' + 'L' * 70 + ']')
tmp.write_text(changed)
tmp.replace(config)
full_observed = sorted((x['skill'] for x in u.discover_all_skill_config_vars()))
assert set(full_observed) == {'target'}
tmp.write_bytes(original)
tmp.replace(config)
token = hc.set_hermes_home_override(f / 'home/.hermes/profiles/other')
check('profile-isolated', ['other-only'])
hc.reset_hermes_home_override(token)
check('profile-restored', ['unrelated', 'platform-denied'])
os.chdir(f / 'project')
check('project-config-not-consumed', ['unrelated', 'platform-denied'])
assert before == tree(), 'final fixture bytes changed'
closure = {name: {'path': str(Path(m.__file__).resolve()), 'sha256': sha(Path(m.__file__).resolve())} for name, m in list(sys.modules.items()) if getattr(m, '__file__', None) and Path(m.__file__).is_file()}
print(json.dumps({'controls': checks, 'selector_control': {'directory_and_truncated_denials': alias_observed, 'full_metadata_denials': full_observed}, 'rows': rows, 'fixture_before': before, 'fixture_after': tree(), 'unchanged': before == tree(), 'closure': closure}, indent=2))
