#!/usr/bin/env python3
"""Offline Hermes 0.20.0 native loader probe in a disposable home/project.

Run with the installed Hermes virtualenv Python, passing its source tree and
an output directory. No agent conversation, credentials, or plugin code runs.
"""
import hashlib
import json
import os
from pathlib import Path
import socket
import subprocess
import sys
import tempfile

sys.dont_write_bytecode = True
source = Path(sys.argv[1]).resolve()
output = Path(sys.argv[2]).resolve()
output.mkdir(parents=True, exist_ok=True)
root = Path(tempfile.mkdtemp(prefix="ale31-hermes-native-")).resolve()
home = root / "home"
hermes = home / ".hermes"
profile = hermes / "profiles" / "research"
project = root / "project"
external = profile / "shared"
for path in (profile, project, external):
    path.mkdir(parents=True)
(hermes / "active_profile").write_text("Research\n")
(profile / ".no-bundled-skills").write_text("")
(hermes / ".no-bundled-skills").write_text("")

def skill(base, folder, name, extra=""):
    path = base / folder / "SKILL.md"
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(f"---\nname: {name}\ndescription: Inert native fixture.\n{extra}---\nRead-only fixture instructions.\n")
    return path

skill(profile / "skills", "a/local", "collision")
skill(external, "b/external", "collision")
skill(profile / "skills", "shared", "shared")
(external / "shared-alias").symlink_to(profile / "skills" / "shared", target_is_directory=True)
skill(profile / "skills", "global-denied", "global-denied")
skill(profile / "skills", "platform-denied", "platform-denied")
skill(profile / "skills", "windows-only", "windows-only", "platforms: [windows]\n")
skill(profile / "skills", "manual-flag", "manual-flag", "disable-model-invocation: true\n")
skill(profile / "skills", "container/child", "nested")
skill(profile / "skills", "first/local-conflict", "local-conflict")
skill(profile / "skills", "second/local-conflict", "local-conflict")
skill(profile / "skills", "container/child/references/archived", "support-hidden")
skill(profile / "skills", "_org/old/stale", "org-stale")
skill(profile / "skills", "_org/current/live", "org-live")
(profile / "skills/_org/.active_org").write_text("current\n")
skill(project / ".hermes/skills", "not-automatic", "project-not-auto")
skill(hermes / "skills", "default-inactive", "default-inactive")
plugin = profile / "plugins/category/portable"
plugin.mkdir(parents=True)
(plugin / "plugin.json").write_text(json.dumps({"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"portable","version":"1.0"}))
skill(plugin / "skills", "explicit", "explicit")
(profile / "config.yaml").write_text("skills:\n  external_dirs: [shared]\n  disabled: [global-denied]\n  platform_disabled:\n    telegram: [platform-denied]\nplugins:\n  enabled: [category/portable]\n")

def manifest():
    return {str(path.relative_to(root)): hashlib.sha256(path.read_bytes()).hexdigest()
            for path in root.rglob("*") if path.is_file() and path.name in {"SKILL.md","config.yaml","active_profile","plugin.json",".active_org"}}

before = manifest()
# Use only the selected disposable environment. Native CLI may create logs in it.
os.environ.clear()
os.environ.update(HOME=str(home), HERMES_HOME=str(hermes), HERMES_PLATFORM="telegram", PATH="/usr/bin:/bin", PYTHONDONTWRITEBYTECODE="1", HERMES_SKIP_NODE_BOOTSTRAP="1")
network_attempts = []
def deny_connect(*args, **kwargs):
    network_attempts.append(str(args))
    raise RuntimeError("network denied in native validation")
socket.socket.connect = deny_connect
socket.create_connection = deny_connect
sys.path.insert(0, str(source))
sys.argv = ["hermes", "--version"]
# This is the real CLI's startup profile selection, without launching a session.
from hermes_cli import main, __version__
from hermes_constants import get_hermes_home
from agent.skill_utils import get_external_skills_dirs, get_disabled_skill_names, iter_skill_index_files
from tools.skills_tool import _find_all_skills, skill_view
from agent.prompt_builder import build_skills_system_prompt
from hermes_cli.agent_plugins import load_agent_plugin
from hermes_cli.plugins import _portable_skill_namespace, PluginManager
assert get_hermes_home() == profile
assert get_external_skills_dirs() == [external.resolve()]
assert get_disabled_skill_names() == {"global-denied", "platform-denied"}
listed = _find_all_skills()
names = [entry["name"] for entry in listed]
assert names.count("collision") == 1
for expected in ("shared", "manual-flag", "nested", "org-live"):
    assert expected in names, (expected, names)
for excluded in ("global-denied", "platform-denied", "windows-only", "support-hidden", "org-stale", "project-not-auto", "default-inactive"):
    assert excluded not in names, (excluded, names)
collision = json.loads(skill_view("collision"))
assert not collision["success"] and len(collision["matches"]) == 2
shared = json.loads(skill_view("shared"))
assert shared["success"], shared
prompt = build_skills_system_prompt()
assert prompt.count("- local-conflict:") == 2, prompt
assert "project-not-auto" not in prompt
package = load_agent_plugin(plugin, profile / "plugin-data")
assert [entry.name for entry in package.skills] == ["explicit"]
manager = PluginManager()
# Read-only manifest discovery proves user/project gates without loading modules.
manifests = manager._collect_directory_manifests()
portable_manifest = next(entry for entry in manifests if entry.path == str(plugin))
assert portable_manifest.skill_namespace == _portable_skill_namespace("category/portable")
after = manifest()
assert before == after
assert not network_attempts
receipt = {
 "version":__version__, "source_head": subprocess.check_output(["/usr/bin/git","-C",str(source),"rev-parse","HEAD"],text=True).strip(),
 "fixture":str(root), "active_home":str(get_hermes_home()), "native_index":listed,
 "collision":collision,"shared_load_success":shared["success"],"portable_namespace":portable_manifest.skill_namespace,
 "portable_skills":[entry.name for entry in package.skills],"disabled_union":sorted(get_disabled_skill_names()),
 "primary_conflict_prompt_entries":prompt.count("- local-conflict:"),
 "native_metadata_outputs":[str(path.relative_to(root)) for path in root.rglob(".skills_prompt_snapshot.json")],
 "source_hashes":{path:hashlib.sha256((source/path).read_bytes()).hexdigest() for path in ("agent/skill_utils.py","agent/prompt_builder.py","tools/skills_tool.py","hermes_constants.py","hermes_cli/main.py","hermes_cli/plugins.py","hermes_cli/agent_plugins.py")},
 "network_attempts":network_attempts,"fixtures_unchanged":before==after,
 "not_exercised":["plugin module execution", "running session", "writer", "native full CLI listing, which may migrate config or seed bundled skills"],
}
(output / "native.json").write_text(json.dumps(receipt, indent=2)+"\n")
(output / "fixture-before.json").write_text(json.dumps(before,indent=2)+"\n")
(output / "fixture-after.json").write_text(json.dumps(after,indent=2)+"\n")
print(json.dumps(receipt,indent=2))
