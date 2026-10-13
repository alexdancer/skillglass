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


r = FIXTURE_HOME / '.agents/skills'
skill(r, 'valid')
skill(r, 'group/nested')
skill(r, '.hidden/hidden-skill')
missing_name = skill(r, 'missing-name')
missing_name.write_text('---\ndescription: Description only.\n---\nBody.\n')
missing_description = skill(r, 'missing-description')
missing_description.write_text('---\nname: missing-description\n---\nBody.\n')
malformed = skill(r, 'malformed')
malformed.write_text('---\nname: [\ndescription: malformed\n---\nBody.\n')
source = skill(BASE/'external', 'manifest-target', 'linked-manifest')
(r/'manifest-link').mkdir()
(r/'manifest-link/SKILL.md').symlink_to(source)
result = case('manifest-eligibility')
OUT.joinpath('result.json').write_text(json.dumps(RESULTS,indent=2)+'\n')
summary = [{'name':s['name'],'path':s['path'],'enabled':s['enabled']} for d in result.get('result',{}).get('data',[]) for s in d['skills'] if '/.system/' not in s['path']]
errors = [e for d in result.get('result',{}).get('data',[]) for e in d['errors']]
OUT.joinpath('summary.json').write_text(json.dumps({'version':RESULTS['version'],'skills':summary,'errors':errors},indent=2)+'\n')
