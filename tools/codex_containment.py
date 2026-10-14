"""Proved launch boundary and sealed receipts for Codex control verification."""
import hashlib
import json
import pathlib
import socket
import subprocess
from types import MappingProxyType


PINNED_NATIVE_SHA256 = '8eaf1ad12fe6bf89b1710330f58900014322c7c5af677e43be116d8ac5fc0a9e'
CONTROL_NAMES = {
    'allowed_read', 'excluded_read_denied', 'network_denied',
    'unapproved_child_exec_denied', 'fixture_write_matches_mode',
    'outside_write_denied', 'owned_system_config_absent',
    'owned_system_skills_absent', 'inherited_system_config_absent',
    'inherited_system_skills_absent',
}


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def seal(path, value):
    with path.open('xb') as output:
        output.write(value)
    path.chmod(0o400)


class CodexContainment:
    def __init__(self, root, cwd, environment, native, product, probe, output):
        self.root = root.resolve(strict=True)
        self.cwd = cwd.resolve(strict=True)
        self.environment = MappingProxyType(dict(environment))
        self.native, self.product, self.probe = [pathlib.Path(p).resolve(strict=True)
                                               for p in (native, product, probe)]
        self.hashes = {p: digest(p) for p in (self.native, self.product, self.probe)}
        if self.hashes[self.native] != PINNED_NATIVE_SHA256:
            raise RuntimeError('Codex binary does not match the pinned 0.155.1 SHA256')
        if len(self.hashes) != 3:
            raise RuntimeError('native, product and isolation probe must be distinct')
        if any(p.is_relative_to(self.root) for p in (self.native, self.product, self.probe)):
            raise RuntimeError('executables must be outside the writable fixture')
        self.output = output.resolve(strict=True)
        if self.output.is_relative_to(self.root) or not self.cwd.is_relative_to(self.root):
            raise RuntimeError('receipts must be outside the writable fixture; cwd must be inside')
        self.calls = []
        self.ready = False
        self.controls = self.output / 'controls'
        self.controls.mkdir()
        system = self.controls / 'system'
        (system / 'skills/control').mkdir(parents=True)
        (system / 'config.toml').write_text('inert system config')
        (system / 'skills/control/SKILL.md').write_text('inert system skill')
        self.profiles = {}
        self.profile_hashes = {}
        for writer in (False, True):
            profile = self.output / ('writer.sb' if writer else 'reader.sb')
            value = self.policy(writer, system).encode()
            seal(profile, value)
            self.profiles[writer] = profile
            self.profile_hashes[writer] = digest(profile)
        self.prove(system)
        self.ready = True

    def policy(self, writer, system):
        q = lambda p: json.dumps(str(p))
        executable = ' '.join('(literal ' + q(p) + ')' for p in (*self.hashes, '/bin/ls'))
        ancestors = ' '.join('(literal ' + q(p) + ')' for p in self.root.parents)
        write = ('(deny file-write* (require-not (subpath ' + q(self.root) + ')))'
                 if writer else '(deny file-write*)')
        return ('(version 1)\n(allow default)\n(deny network*)\n' + write + '\n'
                '(deny file-read-data (require-not (require-any ' + executable + ' ' + ancestors +
                ' (subpath ' + q(self.root) + ') (subpath ' + q(system) + ')'
                ' (subpath "/etc/codex") (subpath "/private/etc/codex")'
                ' (subpath "/System") (subpath "/usr/lib") (subpath "/dev"))))\n'
                '(deny process-exec (require-not (require-any ' + executable + ')))\n' +
                ''.join('(deny file-read* (subpath ' + q(p) + ') (with errno ENOENT))\n'
                        for p in (system, '/etc/codex', '/private/etc/codex')))

    def command(self, executable, arguments, writer=True):
        executable = pathlib.Path(executable).resolve(strict=True)
        if executable not in self.hashes or (not self.ready and executable != self.probe):
            raise RuntimeError('launch refused before containment proof')
        if any(digest(p) != expected for p, expected in self.hashes.items()):
            raise RuntimeError('sealed executable changed; launch refused')
        profile = self.profiles[writer]
        if digest(profile) != self.profile_hashes[writer]:
            raise RuntimeError('sealed profile changed; launch refused')
        argv = ['/usr/bin/sandbox-exec', '-f', str(profile), str(executable), *arguments]
        return self._record(argv, executable, profile)

    def _record(self, argv, executable, profile=None):
        context = {'argv': argv, 'cwd': str(self.cwd), 'environment': dict(self.environment),
                   'binary': str(executable), 'binary_sha256': digest(executable),
                   'profile': str(profile) if profile else None,
                   'profile_sha256': digest(profile) if profile else None}
        raw = (json.dumps(context, sort_keys=True, indent=2) + '\n').encode()
        receipt = self.output / f'invocation-{len(self.calls):04d}.json'
        seal(receipt, raw)
        self.calls.append({'receipt': str(receipt), 'sha256': hashlib.sha256(raw).hexdigest()})
        return argv

    def native_command(self, arguments):
        return self.command(self.native, arguments)

    def initialize_git(self, repo):
        template = self.root / 'git-template'
        template.mkdir()
        git = pathlib.Path('/usr/bin/git').resolve(strict=True)
        argv = self._record([str(git), '-c', 'init.templateDir=' + str(template), 'init', '-q', str(repo)], git)
        subprocess.run(argv, cwd=self.cwd, env=self.environment, check=True)

    def prove(self, system):
        allowed = self.root / 'control-input'
        excluded = self.controls / 'outside-input'
        write = self.root / 'control-write'
        outside_write = self.controls / 'outside-write'
        allowed.write_text('ALLOWED_READ_SENTINEL\n')
        excluded.write_text('inert outside content')
        (self.root / 'escape-link').symlink_to(self.controls, target_is_directory=True)
        positive = {}
        positive['fixture_read'] = allowed.read_text() == 'ALLOWED_READ_SENTINEL\n'
        positive['outside_read'] = excluded.read_text() == 'inert outside content'
        positive['system_read'] = (system / 'config.toml').read_text() == 'inert system config'
        positive['system_list'] = len(list((system / 'skills').iterdir())) == 1
        for label, path in [('fixture_write', write), ('outside_write', outside_write),
                            ('symlink_write', self.root / 'escape-link/symlink-write')]:
            path.write_text('positive')
            positive[label] = path.read_text() == 'positive'
            path.unlink()
        executable = pathlib.Path('/usr/bin/true').resolve(strict=True)
        argv = self._record([str(executable)], executable)
        positive['executable'] = subprocess.run(argv, cwd=self.cwd,
                                               env=self.environment, check=True).returncode == 0
        with socket.socket() as listener:
            listener.bind(('127.0.0.1', 0))
            listener.listen()
            with socket.create_connection(listener.getsockname(), timeout=2):
                connection, _ = listener.accept()
                connection.close()
            positive['network'] = True
        if not all(positive.values()):
            raise RuntimeError('positive containment controls failed')
        results = []
        for writer in (False, True):
            for outside in (outside_write, self.root / 'escape-link/symlink-write'):
                args = ['boundary', str(writer).lower(), str(allowed), str(excluded),
                        str(write), str(outside), str(system / 'config.toml'), str(system / 'skills')]
                argv = self.command(self.probe, args, writer)
                result = subprocess.run(argv, cwd=self.cwd, env=self.environment,
                                        capture_output=True, text=True, timeout=10)
                results.append({'writer': writer, 'argv': argv, 'exit': result.returncode,
                                'stdout': result.stdout, 'stderr': result.stderr})
                if result.returncode != 0:
                    raise RuntimeError('containment controls failed; native launch refused')
                controls = json.loads(result.stdout)
                if (controls['checks'] != dict.fromkeys(CONTROL_NAMES, True)
                        or outside_write.exists() or (self.controls / 'symlink-write').exists()):
                    raise RuntimeError('containment controls failed; native launch refused')
        seal(self.output / 'containment.json',
             (json.dumps({'positive': positive, 'contained': results}, indent=2) + '\n').encode())
