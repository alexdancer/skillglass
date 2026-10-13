#!/usr/bin/env python3
"""Fail-closed, offline Hermes environment/config helpers. Never starts plugins or CLI."""
import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile

PIN = "5a16635f409e0f5e3d754bb6256bd27707bcec4c"
FILES = ("hermes_cli/env_loader.py", "hermes_cli/config.py", "hermes_cli/managed_scope.py", "hermes_cli/plugins.py", "hermes_cli/main.py", "agent/skill_utils.py")
HELPER = r'''
import os,sys,json,socket,subprocess,hashlib
from pathlib import Path
fixture=Path(os.environ["FIXTURE"])
checks={}
def deny(name,operation,absent=False):
    try:operation()
    except (PermissionError,FileNotFoundError) as error:
        checks[name]=isinstance(error,FileNotFoundError) if absent else isinstance(error,PermissionError)
    else:checks[name]=False
positive=fixture/"positive.txt"
positive.write_text("fixture")
checks["fixture_read_write"]=positive.read_text()=="fixture"
positive.unlink()
deny("excluded_owned_read",lambda:Path(os.environ["EXCLUDED"]).read_bytes())
deny("modeled_system_read",lambda:Path(os.environ["MODELED_SYSTEM"]).read_bytes(),True)
deny("modeled_system_stat",lambda:Path(os.environ["MODELED_SYSTEM"]).stat(),True)
deny("outside_write",lambda:(fixture.parent/"forbidden.txt").write_text("blocked"))
deny("child_execution",lambda:subprocess.run(["/usr/bin/true"],check=True))
def network():
    with socket.socket() as sock:sock.bind(("127.0.0.1",0))
deny("network",network)
assert all(checks.values()),checks
# Native imports occur only after the strict containment controls pass.
sys.dont_write_bytecode=True
sys.path.insert(0,os.environ["SOURCE"])
def manifest():
    return {str(p.relative_to(fixture)):hashlib.sha256(p.read_bytes()).hexdigest() for p in fixture.rglob("*") if p.is_file()}
before=manifest()
from hermes_cli import __version__
from hermes_cli.env_loader import load_hermes_dotenv
from hermes_cli.plugins import _get_enabled_plugins,_get_disabled_plugins
from agent.skill_utils import get_disabled_skill_names
loaded=load_hermes_dotenv(hermes_home=Path(os.environ["HERMES_HOME"]))
result={"version":__version__,"controls":checks,"loaded_envs":[str(p) for p in loaded],"platform":os.environ.get("HERMES_PLATFORM"),"session_platform":os.environ.get("HERMES_SESSION_PLATFORM"),"disabled_skills":sorted(get_disabled_skill_names()),"enabled_plugins":sorted(_get_enabled_plugins() or []),"disabled_plugins":sorted(_get_disabled_plugins()),"fixtures_unchanged":before==manifest()}
assert result["fixtures_unchanged"],"fixture bytes changed"
print(json.dumps(result))
'''

def quoted(value):
    return json.dumps(str(value))

def run(source, output):
    source=source.resolve(); output=output.resolve(); output.mkdir(parents=True,exist_ok=True)
    head=subprocess.check_output(["/usr/bin/git","-C",str(source),"rev-parse","HEAD"],text=True).strip()
    status=subprocess.check_output(["/usr/bin/git","-C",str(source),"status","--porcelain"],text=True)
    if head!=PIN or status:raise RuntimeError("installed source differs from the validated clean revision")
    hashes={name:hashlib.sha256((source/name).read_bytes()).hexdigest() for name in FILES}
    python=source/"venv/bin/python"
    real_python=python.resolve(strict=True)
    sandbox=Path("/usr/bin/sandbox-exec")
    if not sandbox.is_file():raise RuntimeError("OS containment unavailable; no native invocation")
    root=Path(tempfile.mkdtemp(prefix="ale31-policy-native-")).resolve()
    if any((parent/".git").exists() for parent in (root,*root.parents)):raise RuntimeError("native fixtures must be outside checkouts")
    excluded=root/"excluded"; excluded.mkdir(); (excluded/"home-canary").write_text("OWNED_LIVE_HOME_CANARY")
    modeled=root/"modeled-system"; modeled.mkdir(); (modeled/"config.yaml").write_text("OWNED_SYSTEM_POLICY_CANARY")
    cases=[
        ("profile-managed", "HERMES_PLATFORM=telegram\n", "disabled: [portable]", "", ["dotenv-denied"], ["portable"], ["portable"]),
        ("shell-fallback", None, "", "", [], ["portable"], []),
        ("quoted-literal", "export HERMES_PLATFORM='telegram' # inert\n", "disabled: []", "", ["dotenv-denied"], ["portable"], []),
        ("session-fallback", "HERMES_PLATFORM=\nHERMES_SESSION_PLATFORM=telegram\n", "", "", ["dotenv-denied"], ["portable"], []),
        ("managed-leaf-replacement", "HERMES_PLATFORM=telegram\n", "disabled: []", "", ["dotenv-denied"], ["portable"], []),
        ("managed-env-expansion", "HERMES_PLATFORM=discord\n", "disabled: ['${env:HERMES_SESSION_PLATFORM}']", "HERMES_SESSION_PLATFORM=portable\n", [], ["portable"], ["portable"]),
    ]
    results=[]
    for name,dotenv,managed_cfg,managed_env,skills,enabled,disabled in cases:
        fixture=root/name;home=fixture/"home";profile=home/".hermes/profiles/research";managed=fixture/"managed";project=fixture/"project"
        for path in (profile,project,managed,fixture/"tmp",fixture/"xdg"):path.mkdir(parents=True,exist_ok=True)
        for subdir in ("cron","sessions","logs/curator","memories","pairing","hooks","image_cache","audio_cache","skills"):(profile/subdir).mkdir(parents=True,exist_ok=True)
        (profile/"SOUL.md").write_text("Inert fixture identity.\n")
        user_disabled="  disabled: [portable]\n" if name=="managed-leaf-replacement" else ""
        (profile/"config.yaml").write_text("skills:\n  platform_disabled:\n    telegram: [dotenv-denied]\nplugins:\n  enabled: [portable]\n"+user_disabled)
        if dotenv is not None:(profile/".env").write_text(dotenv)
        if managed_cfg:(managed/"config.yaml").write_text("skills:\n  disabled: [managed-must-not-be-raw]\nplugins:\n  "+managed_cfg+"\n")
        if managed_env:(managed/".env").write_text(managed_env)
        helper=fixture/"helper.py";helper.write_text(HELPER)
        policy=f'''(version 1)
(allow default)
(deny network*)
(deny process-fork)
(deny process-exec (require-not (require-any (literal {quoted(python)}) (literal {quoted(real_python)}))))
(deny file-write* (require-not (subpath {quoted(fixture)})))
(deny file-read-data (require-not (require-any
 (subpath {quoted(fixture)}) (subpath {quoted(source)})
 (subpath {quoted(modeled)}) (subpath "/etc/hermes") (subpath "/etc/codex")
 (subpath "/System") (subpath "/usr/lib") (subpath "/dev") (literal "/"))))
(deny file-read* (subpath {quoted(modeled)}) (with errno ENOENT))
(deny file-read* (subpath "/etc/hermes") (with errno ENOENT))
(deny file-read* (subpath "/etc/codex") (with errno ENOENT))
(deny file-read* (literal {quoted(source/".env")}) (with errno ENOENT))
(deny file-read* (literal {quoted(source/".op.env")}) (with errno ENOENT))
(deny file-read* (literal {quoted(source/"config.yaml")}) (with errno ENOENT))
'''
        profile_path=fixture/"native.sb";profile_path.write_text(policy)
        env={"HOME":str(home),"HERMES_HOME":str(profile),"HERMES_MANAGED_DIR":str(managed),"HERMES_PLATFORM":"discord","PATH":"/usr/bin:/bin","TMPDIR":str(fixture/"tmp"),"PYTHONDONTWRITEBYTECODE":"1","HERMES_SKIP_NODE_BOOTSTRAP":"1","FIXTURE":str(fixture),"SOURCE":str(source),"EXCLUDED":str(excluded/"home-canary"),"MODELED_SYSTEM":str(modeled/"config.yaml")}
        env.update({key:str(fixture/"xdg") for key in ("XDG_CONFIG_HOME","XDG_CACHE_HOME","XDG_DATA_HOME","XDG_STATE_HOME")})
        argv=[str(sandbox),"-f",str(profile_path),str(python),str(helper)]
        completed=subprocess.run(argv,cwd=project,env=env,text=True,capture_output=True,timeout=30)
        (output/(name+".stdout")).write_text(completed.stdout);(output/(name+".stderr")).write_text(completed.stderr);(output/(name+".sb")).write_text(policy)
        record={"case":name,"argv":argv,"cwd":str(project),"environment":env,"exit":completed.returncode}
        (output/(name+"-command.json")).write_text(json.dumps(record,indent=2)+"\n")
        if completed.returncode:raise RuntimeError("contained native case failed: "+name)
        result=json.loads(completed.stdout)
        assert result["version"]=="0.20.0",result
        assert (result["disabled_skills"],result["enabled_plugins"],result["disabled_plugins"])==(skills,enabled,disabled),result
        results.append(dict(record,result=result))
    final_hashes={name:hashlib.sha256((source/name).read_bytes()).hexdigest() for name in FILES}
    assert hashes==final_hashes
    assert subprocess.check_output(["/usr/bin/git","-C",str(source),"rev-parse","HEAD"],text=True).strip()==head
    assert subprocess.check_output(["/usr/bin/git","-C",str(source),"status","--porcelain"],text=True)==status
    receipt={"source_head":head,"source_status":status,"source_hashes":hashes,"fixture_root":str(root),"python":str(python),"results":results,"limits":["OS containment verified by owned canaries, not by inspecting live settings.","Only environment/config helpers ran. Plugin initialization/catalog, full CLI startup, inference session, writers and reload remain unobserved.","Unsupported interpolation/provider cases are conservatively unknown in the viewer; native secret providers are never executed.","Machine default /etc/hermes and /etc/codex inputs are modeled absent in this fixture evidence, not observed enterprise policy."]}
    (output/"native.json").write_text(json.dumps(receipt,indent=2)+"\n")
    print(json.dumps({"source_head":head,"cases":len(results),"fixtures_unchanged":True,"native":str(output/"native.json")}))

if __name__=="__main__":
    if len(sys.argv)!=3:raise SystemExit("usage: verify_hermes_policy.py SOURCE_TREE OUTPUT_DIR")
    run(Path(sys.argv[1]),Path(sys.argv[2]))
