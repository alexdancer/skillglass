"""Shared disposable fixture and bounded offline Codex app-server transport."""
import argparse
import json
import pathlib
import queue
import shutil
import subprocess
import tempfile
import threading
import time


class Probe:
    def __init__(self, description):
        parser = argparse.ArgumentParser(description=description)
        parser.add_argument('--codex', default=shutil.which('codex'))
        parser.add_argument('--out', type=pathlib.Path)
        options = parser.parse_args()
        if not options.codex:
            parser.error('Codex binary unavailable; supply --codex')
        self.out = (options.out or pathlib.Path(tempfile.mkdtemp(prefix='codex-probe-evidence-', dir='/tmp'))).resolve()
        self.out.mkdir(parents=True, exist_ok=True)
        self.base = pathlib.Path(tempfile.mkdtemp(prefix='ale26-codex-', dir='/tmp')).resolve()
        self.home = self.base / 'home'
        self.codex_home = self.home / '.codex'
        self.repo = self.base / 'outside' / 'repo'
        self.cwd = self.repo / 'nested' / 'leaf'
        for directory in (self.codex_home, self.cwd):
            directory.mkdir(parents=True, exist_ok=True)
        self.env = {'HOME': str(self.home), 'CODEX_HOME': str(self.codex_home),
                    'PATH': '/usr/bin:/bin:/usr/sbin:/sbin', 'TMPDIR': str(self.base),
                    'XDG_CONFIG_HOME': str(self.home / '.config'), 'RUST_LOG': 'error'}
        self.binary = str(pathlib.Path(options.codex).resolve())
        subprocess.run(['/usr/bin/git', 'init', '-q', str(self.repo)], env=self.env, check=True)
        self.results = {'root': str(self.base), 'version': subprocess.check_output(
            [self.binary, '--version'], env=self.env, text=True).strip(), 'cases': []}

    def skill(self, root, folder, name=None, policy=None):
        directory = root / folder
        directory.mkdir(parents=True, exist_ok=True)
        manifest = directory / 'SKILL.md'
        manifest.write_text(f'---\nname: {name or folder}\ndescription: Probe {folder}.\n---\nOffline probe instructions.\n')
        if policy is not None:
            (directory / 'agents').mkdir(exist_ok=True)
            (directory / 'agents' / 'openai.yaml').write_text(
                f'policy:\n  allow_implicit_invocation: {str(policy).lower()}\n')
        return manifest

    def config(self, text=''):
        (self.codex_home / 'config.toml').write_text(
            '[analytics]\nenabled = false\n[feedback]\nenabled = false\n' + text)

    def command(self, arguments):
        return [self.binary, *arguments]

    def server(self, flags=()):
        return _Server(self, flags)

    def case(self, label, text='', flags=(), extras=None):
        self.config(text)
        server = self.server(flags)
        try:
            if extras is not None:
                server.call('skills/extraRoots/set', {'extraRoots': [str(path) for path in extras]})
            listing = server.call('skills/list', {'cwds': [str(self.cwd)], 'forceReload': True})
            effective = server.call('config/read', {'cwd': str(self.cwd), 'includeLayers': True})
            self.results['cases'].append({'label': label, 'config': text, 'flags': list(flags),
                                          'skills': listing, 'effectiveConfig': effective})
            print(label, json.dumps(listing.get('result', listing)))
            return listing
        finally:
            server.close()


class _Server:
    def __init__(self, probe, flags):
        self.probe = probe
        self.stderr = open(probe.base / f'stderr-{time.time_ns()}.txt', 'w')
        # No inference calls; caller must supply any required OS/network containment.
        self.process = subprocess.Popen(probe.command(['app-server', '--stdio', *flags]),
                                        cwd=probe.cwd, env=probe.env, stdin=subprocess.PIPE,
                                        stdout=subprocess.PIPE, stderr=self.stderr, text=True, bufsize=1)
        self.queue = queue.Queue()
        self.messages = []
        self.sequence = 0
        self.closed = False
        self.reader = threading.Thread(target=self._read, daemon=True)
        self.reader.start()
        try:
            self.call('initialize', {'clientInfo': {'name': 'ale26_offline_probe', 'version': '1'},
                                     'capabilities': {'experimentalApi': True}})
            self.process.stdin.write(json.dumps({'method': 'initialized'}) + '\n')
            self.process.stdin.flush()
        except BaseException:
            self.close()
            raise

    def _read(self):
        try:
            for line in self.process.stdout:
                self.queue.put(line)
        finally:
            self.queue.put(None)

    def call(self, method, params):
        self.sequence += 1
        message = {'id': self.sequence, 'method': method, 'params': params}
        self.messages.append({'request': message})
        self.process.stdin.write(json.dumps(message) + '\n')
        self.process.stdin.flush()
        deadline = time.monotonic() + 15
        while time.monotonic() < deadline:
            try:
                line = self.queue.get(timeout=max(.01, deadline - time.monotonic()))
            except queue.Empty:
                raise RuntimeError(f'Timeout for {method}; exit={self.process.poll()}') from None
            if line is None:
                raise RuntimeError(f'Codex closed output during {method}; exit={self.process.poll()}')
            response = json.loads(line)
            self.messages.append({'response': response})
            if response.get('id') == self.sequence:
                return response
        raise RuntimeError(f'Timeout for {method}')

    def close(self):
        if self.closed:
            return
        self.closed = True
        self.process.terminate()
        try:
            self.process.wait(timeout=3)
        except subprocess.TimeoutExpired:
            self.process.kill()
            self.process.wait()
        self.reader.join(timeout=3)
        self.process.stdin.close()
        self.process.stdout.close()
        self.stderr.close()
        self.probe.results.setdefault('transcripts', []).append(self.messages)


def override(path, enabled=False):
    return f'[[skills.config]]\npath = {json.dumps(str(path))}\nenabled = {str(enabled).lower()}\n'
