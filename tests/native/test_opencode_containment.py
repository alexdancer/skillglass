"""Executable child controls; this test never invokes OpenCode."""
import hashlib
import json
import os
from pathlib import Path
import shutil
import socket
import subprocess
import sys
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT/'tools'))
from native_containment import native_profile


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
        positive = socket.create_connection(listener.getsockname(), timeout=1)
        accepted, _ = listener.accept()
        positive.close()
        accepted.close()
        source = base/'probe.go'
        program = r'''package main
import("encoding/json";"net";"os";"os/exec";"path/filepath";"time")
func main(){
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
 c,networkErr:=net.DialTimeout("tcp",ADDRESS,time.Second);if c!=nil{c.Close()}
 null, nullErr:=os.OpenFile("/dev/null",os.O_RDWR,0)
 nullDiscard:=false
 if nullErr==nil {_,writeNullErr:=null.Write([]byte("nonpersistent"));buf:=make([]byte,1);n,_:=null.Read(buf);nullDiscard=writeNullErr==nil&&n==0;null.Close()}
 childErr:=exec.Command(os.Args[0],"child").Run()
 json.NewEncoder(os.Stdout).Encode(map[string]bool{"null_rw_discard":nullDiscard,"fixture_write":writeErr==nil,"fixture_read":readErr==nil&&string(read)=="fixture-positive","excluded_read_denied":excludedErr!=nil,"outside_write_denied":outsideErr!=nil,"symlink_created":linkErr==nil,"symlink_escape_denied":escapeErr!=nil,"network_denied":networkErr!=nil,"inherited_child_denied":childErr!=nil,"protected_stat_absent":os.IsNotExist(statErr)})
}
'''
        for key, value in [('OUTSIDE', outside), ('CANARY', canary), ('EXTERNAL', external), ('ADDRESS', address)]:
            program = program.replace(key, json.dumps(str(value)))
        source.write_text(program)
        binary = base/'probe'
        env = {'PATH':'/usr/bin:/bin','HOME':str(base),'TMPDIR':str(base), 'GOTMPDIR':str(base), 'GOPROXY':'off','GOSUMDB':'off', 'GOCACHE':str(base/'go-cache')}
        subprocess.run(['/opt/homebrew/bin/go','build','-o',str(binary),str(source)],env=env,check=True,capture_output=True)
        shutil.copyfile(source,evidence/'inert-probe.go')
        shutil.copyfile(binary,evidence/'inert-probe')
        helper = ROOT/'tools/verify_opencode.py'
        argv = [sys.executable,str(helper),'--opencode',str(binary),'--out',str(evidence/'helper')]
        result = subprocess.run(argv,cwd=base,env={'PATH':'/usr/bin:/bin','HOME':str(base)},capture_output=True,timeout=30)
        (evidence/'helper.stdout').write_bytes(result.stdout)
        (evidence/'helper.stderr').write_bytes(result.stderr)
        self.assertEqual(result.returncode,1)
        self.assertIn(b'reference version 1.17.15', result.stderr)
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
        direct=subprocess.run(wrapped,env=runenv,capture_output=True,timeout=10)
        checks=json.loads(direct.stdout)
        self.assertTrue(all(checks.values()),checks)
        (evidence/'controls.stdout').write_bytes(direct.stdout)
        (evidence/'controls.stderr').write_bytes(direct.stderr)
        # An invalid policy must stop before the child executes.
        (fixture/'home/write-positive').unlink()
        profile.write_text('(invalid policy)')
        failed=subprocess.run(wrapped,env=runenv,capture_output=True,timeout=10)
        self.assertNotEqual(failed.returncode,0)
        self.assertFalse((fixture/'home/write-positive').exists())
        (evidence/'invalid-policy.stderr').write_bytes(failed.stderr)
        invalid_helper = base/'invalid-helper.py'
        shutil.copyfile(helper, invalid_helper)
        (base/'native_containment.py').write_text("def native_profile(*args, **kwargs):\n    return '(invalid policy)'\ndef runtime_data_paths():\n    return []\n")
        invalid_argv=[sys.executable,str(invalid_helper),'--opencode',str(binary),'--out',str(evidence/'invalid-helper')]
        invalid_result=subprocess.run(invalid_argv,cwd=base,env={'PATH':'/usr/bin:/bin','HOME':str(base)},capture_output=True,timeout=30)
        self.assertNotEqual(invalid_result.returncode,0)
        self.assertEqual((evidence/'invalid-helper/git-init.stdout').read_bytes(),b'')
        (evidence/'invalid-helper.stdout').write_bytes(invalid_result.stdout)
        (evidence/'invalid-helper.stderr').write_bytes(invalid_result.stderr)
        listener.settimeout(.2)
        with self.assertRaises(socket.timeout): listener.accept()
        listener.close()
        receipt={'checks':checks,'helper_argv':argv,'helper_exit':result.returncode,'policy':policy,'wrapped_argv':wrapped,'cwd':str(base),'environment':runenv,'binary_sha256':hashlib.sha256(binary.read_bytes()).hexdigest(),'fail_closed_exit':failed.returncode,'public_helper_fail_closed_argv':invalid_argv,'public_helper_fail_closed_exit':invalid_result.returncode,'positive_controls':{'outside_read_write':True,'loopback':True},'build_argv':['/opt/homebrew/bin/go','build','-o',str(binary),str(source)],'build_environment':env,'source_sha256':hashlib.sha256(source.read_bytes()).hexdigest(),'owned_processes':'all subprocess.run calls completed; no native application launched'}
        (evidence/'receipt.json').write_text(json.dumps(receipt,indent=2)+'\n')
        shutil.rmtree(base)


if __name__ == '__main__': unittest.main(verbosity=2)
