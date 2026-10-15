"""Executable child controls; this test never invokes OpenCode."""
import hashlib
import json
import os
from pathlib import Path
import shutil
import socket
import secrets
import sys
import tempfile
import time
import unittest

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT/'tools'))
from native_containment import native_profile
from native_receipts import recorded_run


def receive_connections(listener, deadline, stop=None, connection_cap=64):
    """Read complete bounded payloads; unrelated host probes may arrive first."""
    received = []
    while time.monotonic() < deadline:
        listener.settimeout(min(.2, max(.001, deadline - time.monotonic())))
        try:
            connection, peer = listener.accept()
        except socket.timeout:
            if stop is None:
                return received
            continue
        with connection:
            payload = b''
            while len(payload) < 65536 and time.monotonic() < deadline:
                connection.settimeout(min(.2, max(.001, deadline - time.monotonic())))
                try:
                    chunk = connection.recv(min(4096, 65536 - len(payload)))
                except (socket.timeout, ConnectionResetError):
                    break
                if not chunk:
                    break
                payload += chunk
        received.append((peer, payload))
        if stop is not None and stop(payload):
            return received
        if len(received) >= connection_cap:
            raise AssertionError('listener connection cap reached; result is inconclusive')
    return received


class NativeContainment(unittest.TestCase):
    def test_public_helper_and_policy(self):
        base = Path(tempfile.mkdtemp(prefix='ale35-containment-', dir='/private/tmp'))
        evidence = Path(os.environ.get('ALE35_CONTAINMENT_OUT', str(base/'evidence')))
        evidence.mkdir(parents=True, exist_ok=True)
        external = base/'excluded'
        external.mkdir()
        protected = external/'live-home'
        protected.mkdir()
        canary = protected/'config'
        canary.write_text('PROTECTED_CANARY')
        outside = external/'outside'
        outside.write_text('POSITIVE_OUTSIDE_WRITE')
        self.assertEqual(outside.read_text(), 'POSITIVE_OUTSIDE_WRITE')
        outside.unlink()
        self.addCleanup(lambda: shutil.rmtree(base, ignore_errors=True))
        listener = socket.socket()
        self.addCleanup(listener.close)
        listener.bind(('127.0.0.1', 0))
        listener.listen()
        address = '%s:%s' % listener.getsockname()
        network_canary = ('ALE35_' + secrets.token_hex(32)).encode()
        positive_scanner = b'inert unrelated positive-control peer'
        with socket.create_connection(listener.getsockname(), timeout=1) as unrelated:
            unrelated.sendall(positive_scanner)
            unrelated.shutdown(socket.SHUT_WR)
        with socket.create_connection(listener.getsockname(), timeout=1) as positive:
            positive.sendall(network_canary)
            positive.shutdown(socket.SHUT_WR)
        positives = receive_connections(listener, time.monotonic() + 3,
                                        stop=lambda payload: payload == network_canary)
        self.assertTrue(any(payload == network_canary for _, payload in positives),
                        'allowed connection must deliver its complete canary')
        self.assertTrue(any(payload == positive_scanner for _, payload in positives),
                        'positive receiver must tolerate the earlier unrelated peer')
        source = base/'probe.go'
        program = r'''package main
import("encoding/json";"net";"os";"os/exec";"path/filepath";"time")
func main(){
 if filepath.Base(os.Args[0])=="git" {
  if len(os.Args)>1 && os.Args[1]=="rev-parse" {cwd,_:=os.Getwd();os.Stdout.WriteString(filepath.Dir(cwd)+"\n")}
  if len(os.Args)>1 && os.Args[1]=="--version" {os.Stdout.WriteString("inert Git stand-in\n")}
  return
 }
 if len(os.Args)>1 && os.Args[1]=="child" {err:=os.WriteFile(OUTSIDE,[]byte("child"),0600);if err!=nil {os.Exit(7)};os.Exit(0)}
 home:=os.Getenv("HOME")
 inside:=filepath.Join(home,"write-positive")
 writeErr:=os.WriteFile(inside,[]byte("fixture-positive"),0600)
 read,readErr:=os.ReadFile(inside)
 _,excludedErr:=os.ReadFile(CANARY)
 outsideErr:=os.WriteFile(OUTSIDE,[]byte("outside"),0600)
 link:=filepath.Join(home,"escape")
 linkErr:=os.Symlink(EXTERNAL,link)
 escapeErr:=os.WriteFile(filepath.Join(link,"escaped"),[]byte("escape"),0600)
 _,statErr:=os.Stat(CANARY)
 c,networkErr:=net.DialTimeout("tcp",ADDRESS,time.Second);if c!=nil{c.Write([]byte(PAYLOAD));c.Close()}
 null, nullErr:=os.OpenFile("/dev/null",os.O_RDWR,0)
 nullDiscard:=false
 if nullErr==nil {_,writeNullErr:=null.Write([]byte("nonpersistent"));buf:=make([]byte,1);n,_:=null.Read(buf);nullDiscard=writeNullErr==nil&&n==0;null.Close()}
 childErr:=exec.Command(os.Args[0],"child").Run()
 json.NewEncoder(os.Stdout).Encode(map[string]bool{"null_rw_discard":nullDiscard,"fixture_write":writeErr==nil,"fixture_read":readErr==nil&&string(read)=="fixture-positive","excluded_read_denied":excludedErr!=nil,"outside_write_denied":outsideErr!=nil,"symlink_created":linkErr==nil,"symlink_escape_denied":escapeErr!=nil,"network_denied":networkErr!=nil,"inherited_child_denied":childErr!=nil,"protected_stat_absent":os.IsNotExist(statErr)})
}
'''
        for key, value in [('OUTSIDE', outside), ('CANARY', canary), ('EXTERNAL', external), ('ADDRESS', address), ('PAYLOAD', network_canary.decode())]:
            program = program.replace(key, json.dumps(str(value)))
        source.write_text(program)
        binary = base/'probe'
        env = {'PATH':'/usr/bin:/bin','HOME':str(base),'TMPDIR':str(base), 'GOTMPDIR':str(base), 'GOPROXY':'off','GOSUMDB':'off', 'GOCACHE':str(base/'go-cache')}
        build, build_record=recorded_run(evidence, 'inert-probe-build', ['/opt/homebrew/bin/go','build','-o',str(binary),str(source)], cwd=base, env=env, source={'source_file':str(source),'source_file_sha256':hashlib.sha256(source.read_bytes()).hexdigest()})
        self.assertEqual(build.returncode, 0, build.stderr.decode(errors='replace'))
        shutil.copyfile(source,evidence/'inert-probe.go')
        shutil.copyfile(binary,evidence/'inert-probe')
        git = base/'git'; shutil.copyfile(binary, git)
        helper = ROOT/'tools/verify_opencode.py'
        argv = [sys.executable,str(helper),'--opencode',str(binary),'--out',str(evidence/'helper'),'--git',str(git)]
        result, helper_launch = recorded_run(evidence, 'helper-launch', argv, cwd=base, env={'PATH':'/usr/bin:/bin','HOME':str(base)}, timeout=30, child_records=evidence/'helper/invocations', source={'source_script':str(helper),'source_script_sha256':hashlib.sha256(helper.read_bytes()).hexdigest()})
        (evidence/'helper.stdout').write_bytes(result.stdout)
        (evidence/'helper.stderr').write_bytes(result.stderr)
        self.assertEqual(result.returncode,1)
        self.assertIn(b'reference version 1.17.15', result.stderr)
        helper_receipts=json.loads((evidence/'helper/receipts.json').read_text())
        self.assertEqual(helper_receipts['write_boundary'],'canonical fixture plus nonpersistent /dev/null discard')
        child = json.loads((evidence/'helper/version.stdout').read_text())
        for key in ['null_rw_discard','fixture_write','fixture_read','excluded_read_denied','outside_write_denied','symlink_created','symlink_escape_denied','network_denied','inherited_child_denied']:
            self.assertTrue(child[key], key)
        self.assertFalse(outside.exists())
        self.assertFalse((external/'escaped').exists())
        # Model live/system metadata with owned files, never actual settings.
        fixture = base/'fixture'; fixture.mkdir(); (fixture/'home').mkdir()
        policy = native_profile(fixture,binary,protected=[protected])
        profile = base/'profile.sb';profile.write_text(policy)
        runenv={'PATH':'/usr/bin:/bin','HOME':str(fixture/'home')}
        wrapped=['/usr/bin/sandbox-exec','-f',str(profile),str(binary)]
        direct, direct_record=recorded_run(evidence, 'valid-policy', wrapped, cwd=base, env=runenv, binary=binary, profile=profile, timeout=10, source={'source_file':str(source),'source_file_sha256':hashlib.sha256(source.read_bytes()).hexdigest()})
        checks=json.loads(direct.stdout)
        self.assertTrue(all(checks.values()),checks)
        (evidence/'controls.stdout').write_bytes(direct.stdout)
        (evidence/'controls.stderr').write_bytes(direct.stderr)
        # An invalid policy must stop before the child executes.
        (fixture/'home/write-positive').unlink()
        profile.write_text('(invalid policy)')
        failed, failed_record=recorded_run(evidence, 'invalid-policy', wrapped, cwd=base, env=runenv, binary=binary, profile=profile, timeout=10, source={'source_file':str(source),'source_file_sha256':hashlib.sha256(source.read_bytes()).hexdigest()})
        self.assertNotEqual(direct_record['profile_sha256'], failed_record['profile_sha256'])
        self.assertNotEqual(failed.returncode,0)
        self.assertFalse((fixture/'home/write-positive').exists())
        (evidence/'invalid-policy.stderr').write_bytes(failed.stderr)
        invalid_helper = base/'invalid-helper.py'
        shutil.copyfile(helper, invalid_helper)
        shutil.copyfile(ROOT/'tools/native_receipts.py', base/'native_receipts.py')
        (base/'native_containment.py').write_text("def native_profile(*args, **kwargs):\n    return '(invalid policy)'\ndef runtime_data_paths():\n    return []\n")
        invalid_argv=[sys.executable,str(invalid_helper),'--opencode',str(binary),'--out',str(evidence/'invalid-helper'),'--git',str(git)]
        invalid_result, invalid_launch=recorded_run(evidence, 'invalid-helper-launch', invalid_argv, cwd=base, env={'PATH':'/usr/bin:/bin','HOME':str(base)}, timeout=30, child_records=evidence/'invalid-helper/invocations', source={'source_script':str(invalid_helper),'source_script_sha256':hashlib.sha256(invalid_helper.read_bytes()).hexdigest()})
        self.assertNotEqual(invalid_result.returncode,0)
        self.assertEqual((evidence/'invalid-helper/git-init.stdout').read_bytes(),b'')
        (evidence/'invalid-helper.stdout').write_bytes(invalid_result.stdout)
        (evidence/'invalid-helper.stderr').write_bytes(invalid_result.stderr)
        # Local service discovery may connect independently of the sandboxed probe.
        # This positive control ensures unrelated traffic cannot mask probe payloads.
        scanner_payload = b'GET / HTTP/1.1\r\nuser-agent: inert-scanner\r\n\r\n'
        with socket.create_connection(listener.getsockname(), timeout=1) as scanner:
            scanner.sendall(scanner_payload)
            scanner.shutdown(socket.SHUT_WR)
        received = receive_connections(listener, time.monotonic() + 3)
        unrelated_connections = []
        for peer, payload in received:
            self.assertNotIn(network_canary, payload, 'sandboxed probe delivered its network canary')
            unrelated_connections.append({'peer': list(peer), 'bytes': len(payload),
                                          'sha256': hashlib.sha256(payload).hexdigest()})
        (evidence/'unrelated-connections.json').write_text(json.dumps(unrelated_connections, indent=2)+'\n')
        self.assertTrue(any(payload == scanner_payload for _, payload in received),
                        'deterministic scanner positive control must reach listener')
        listener.close()
        receipt={'invocation_records':[build_record,helper_launch,direct_record,failed_record,invalid_launch],'native_applications_started':0,'git':'inert compiled stand-in; no native Git startup','checks':checks,'helper_argv':argv,'helper_exit':result.returncode,'policy':policy,'wrapped_argv':wrapped,'cwd':str(base),'environment':runenv,'binary_sha256':hashlib.sha256(binary.read_bytes()).hexdigest(),'fail_closed_exit':failed.returncode,'public_helper_fail_closed_argv':invalid_argv,'public_helper_fail_closed_exit':invalid_result.returncode,'positive_controls':{'outside_read_write':True,'loopback':True},'build_argv':['/opt/homebrew/bin/go','build','-o',str(binary),str(source)],'build_environment':env,'source_sha256':hashlib.sha256(source.read_bytes()).hexdigest(),'owned_processes':'all recorded launches completed and owned children reaped; no native application launched'}
        (evidence/'receipt.json').write_text(json.dumps(receipt,indent=2)+'\n')
        shutil.rmtree(base)


if __name__ == '__main__': unittest.main(verbosity=2)
