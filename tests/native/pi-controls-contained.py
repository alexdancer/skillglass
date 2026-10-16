#!/usr/bin/env python3
"""Run the Pi control-contract probe only after OS containment and inherited-child controls pass."""
import hashlib
import json
import os
from pathlib import Path
import shutil
import sys
import tempfile

from invocation import run_recorded

if len(sys.argv) != 3 or sys.platform != "darwin":
    raise SystemExit("usage on macOS: python3 pi-controls-contained.py INSTALLED_PI_PACKAGE EVIDENCE_DIR")
installed = Path(sys.argv[1]).resolve(strict=True)
out = Path(sys.argv[2]).resolve()
out.mkdir(parents=True, exist_ok=True)
node_path = os.environ.get("PI_PROBE_NODE") or shutil.which("node")
if not node_path:
    raise SystemExit("Select Node with PI_PROBE_NODE")
node = Path(node_path).resolve(strict=True)
# pnpm links point to sibling dependency closures in its immutable package store.
package_store = next((p for p in installed.parents if p.name == "links"), installed)
root = Path(tempfile.mkdtemp(prefix="ale33-contained-", dir="/private/tmp"))
allowed = root / "allowed"
allowed.mkdir()
for name in ["home", "agent", "tmp", "project"]:
    (allowed / name).mkdir()
positive = allowed / "positive"
excluded = root / "excluded-system" / "pi" / "settings.json"
positive.write_text("OWNED_ALLOWED_CANARY")
excluded.parent.mkdir(parents=True)
excluded.write_text("OWNED_EXCLUDED_CANARY")
assert excluded.read_text() == "OWNED_EXCLUDED_CANARY"
script = allowed / "pi-controls.mjs"
shutil.copy2(Path(__file__).with_name("pi-controls.mjs"), script)
shutil.copy2(Path(__file__).with_name("invocation.mjs"), allowed / "invocation.mjs")
def quoted(p):
    return json.dumps(str(p))
ancestors = sorted({str(parent) for base in [allowed, package_store, node, Path("/usr/bin/sandbox-exec"), Path("/opt/homebrew/opt")] for parent in base.parents})
metadata_rules = " ".join("(literal " + quoted(parent) + ")" for parent in ancestors)
profile = f'''(version 1)
(allow default)
(deny network*)
(deny file-write*)
(allow file-write* (subpath {quoted(allowed)}) (literal "/dev/null"))
(deny file-read* (require-not (require-any
 (subpath {quoted(allowed)})
 (subpath {quoted(package_store)})
 (literal {quoted(node)})
 (literal "/usr/bin/sandbox-exec")
 (literal "/")
 (subpath "/System")
 (subpath "/usr/lib")
 (subpath "/opt/homebrew/opt")
 (subpath "/dev"))))
(allow file-read-metadata {metadata_rules})
(deny process-exec (require-not (require-any
 (literal {quoted(node)}))))
'''
profile_path = allowed / "native.sb"
profile_path.write_text(profile)
(out / "native.sb").write_text(profile)
env = {"HOME":str(allowed/"home"), "PI_CODING_AGENT_DIR":str(allowed/"agent"),
       "TMPDIR":str(allowed/"tmp"), "PATH":"/usr/bin:/bin", "TERM":"dumb",
       "PI_OFFLINE":"1", "PI_FIXTURE_CONTAINED":"strict"}
records = []
def run(name, args, timeout=90):
    command = ["/usr/bin/sandbox-exec", "-f", str(profile_path), str(node), *args]
    artifacts = [arg for arg in args if arg.endswith((".mjs", ".js"))]
    result, identity = run_recorded(name, command, allowed/"project", env, allowed/"invocations", profile_path, artifacts, timeout=timeout)
    (out/(name+".stdout")).write_bytes(result.stdout)
    (out/(name+".stderr")).write_bytes(result.stderr)
    records.append({**identity, "exit_code":result.returncode,
                    "stdout_sha256":hashlib.sha256(result.stdout).hexdigest(),
                    "stderr_sha256":hashlib.sha256(result.stderr).hexdigest()})
    (out/"commands.json").write_text(json.dumps(records, indent=2)+"\n")
    if result.returncode:
        raise SystemExit(f"{name} failed closed: {result.returncode}; see {out}")
    return result.stdout.decode("utf-8")
controls = allowed/"controls.mjs"
controls.write_text('''import {spawnRecorded} from './invocation.mjs';
import fs from 'node:fs';
import net from 'node:net';
import assert from 'node:assert/strict';
import {spawnSync} from 'node:child_process';
const [positive,excluded,write]=process.argv.slice(2);
assert.equal(fs.readFileSync(positive,'utf8'),'OWNED_ALLOWED_CANARY');
const denied=(fn)=>{try{fn();return false}catch(e){return ['EPERM','EACCES'].includes(e.code)}};
assert(denied(()=>fs.readFileSync(excluded)));
assert(denied(()=>fs.writeFileSync(write,'forbidden')));
const network=await new Promise(resolve=>{const socket=net.connect({host:'127.0.0.1',port:9});socket.on('error',e=>resolve(['EPERM','EACCES'].includes(e.code)));socket.on('connect',()=>{socket.destroy();resolve(false)});});
assert(network);
const child=spawnSync('/usr/bin/true');assert(child.error && ['EPERM','EACCES'].includes(child.error.code));
if (!process.env.INHERITED_CONTROL) {
 const nested=spawnRecorded(process.execPath,process.argv.slice(1),{cwd:process.cwd(),env:{...process.env,INHERITED_CONTROL:'1'}});
 let error='';nested.stderr.on('data',chunk=>{error+=chunk.toString()});
 const status=await new Promise((resolve,reject)=>{nested.on('error',reject);nested.on('close',resolve)});
 assert.equal(status,0,error);
}
console.log(JSON.stringify({allowedRead:true,excludedSystemReadDenied:true,outsideWriteDenied:true,networkDenied:true,unapprovedChildDenied:true,inheritedBoundary:true,nodeVersion:process.version}));
''')
try:
    control_json = run("controls", [str(controls),str(positive),str(excluded),str(root/"forbidden-write")])
    json.loads(control_json)
    run("native-syntax", ["--check",str(script)])
    version = run("pi-version", [str(installed/"dist/bundle/cli.js"),"--version"], timeout=15).strip()
    # No native version/help, imports or RPC can run before the controls pass.
    receipt = json.loads(run("native", [str(script),str(installed)]))
    assert version == receipt["version"], (version, receipt["version"])
    (out/"native.json").write_text(json.dumps(receipt,indent=2)+"\n")
    manifest = {"native_script_sha256":hashlib.sha256(script.read_bytes()).hexdigest(),
                "version":receipt["version"], "package_store_read_allowance":str(package_store),
                "excluded_system_inputs":"All /etc, /Library and live user application settings excluded by default; owned modeled system canary denied. No live system/enterprise contents observed.",
                "controls":json.loads(control_json),"native_children_waited":True}
    (out/"containment.json").write_text(json.dumps(manifest,indent=2)+"\n")
finally:
    if (allowed/"invocations").exists():
        shutil.copytree(allowed/"invocations", out/"invocations", dirs_exist_ok=True)
    shutil.rmtree(root)
