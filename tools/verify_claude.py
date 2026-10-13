#!/usr/bin/env python3
"""Validate startup catalogs without model requests or skill execution."""

import argparse
import hashlib
import json
import pathlib
import select
import subprocess
import tempfile

from claude_containment import NativeContainment


def write(path, text):
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(text)


def skill(base, folder, name=None, extra="", description=None):
    path = base / ".claude/skills" / folder / "SKILL.md"
    metadata = f"name: {name}\n" if name else ""
    write(path, "---\n" + metadata + f"description: {description or folder}\n"
          + extra + "---\nInert instructions. Never execute anything.\n")
    return path


def manifest(areas, root):
    return {
        str(path.relative_to(root)): hashlib.sha256(path.read_bytes()).hexdigest()
        for area in areas
        for path in ([area] if area.is_file() else area.rglob("*"))
        if path.is_file()
    }


def initialize(command, cwd, environment):
    process = subprocess.Popen(command, cwd=cwd, env=environment,
                               stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                               stderr=subprocess.PIPE, text=True)
    request = {"type": "control_request", "request_id": "init",
               "request": {"subtype": "initialize"}}
    try:
        process.stdin.write(json.dumps(request) + "\n")
        process.stdin.flush()
        if not select.select([process.stdout], [], [], 25)[0]:
            raise RuntimeError("native initialization timed out")
        raw = process.stdout.readline()
        process.terminate()
        _, stderr = process.communicate(timeout=10)
        if not raw:
            raise RuntimeError("native initialization returned no catalog: " + stderr)
        response = json.loads(raw)["response"]["response"]
        return raw, stderr, response, process.returncode
    finally:
        if process.poll() is None:
            process.kill()
            process.communicate()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("output", type=pathlib.Path)
    parser.add_argument("--claude", default="/Users/alex/.local/bin/claude")
    options = parser.parse_args()
    output = options.output.resolve()
    output.mkdir(parents=True, exist_ok=True)
    root = pathlib.Path(tempfile.mkdtemp(prefix="ale29-native-", dir="/private/tmp")).resolve()
    home, repo = root / "home", root / "repo"
    cwd = repo / "nested"
    home.mkdir()
    cwd.mkdir(parents=True)
    fixtures = [
        (repo, "nested-collision", "nested-collision", "", "ROOT"),
        (cwd, "nested-collision", "nested-collision", "", "CHILD"),
        (home, "missing-name", None, "", None),
        (repo, "root-only", "root-only", "", None),
        (cwd, "nested-only", "nested-only", "", None),
        (home, "collision", "collision", "", "PERSONAL"),
        (repo, "collision", "collision", "", "PROJECT"),
        (cwd, "collision", "collision", "", "NESTED"),
        (root, "outside", "outside", "", None),
        (repo / "below", "below", "below", "", None),
        (home, "renamed", "display-name", "", None),
        (home, "explicit", "explicit", "disable-model-invocation: true\n", None),
        (home, "model-only", "model-only", "user-invocable: false\n", None),
        (home, "group/deep", "deep", "", None),
        (home, "synced", "reserved-synced", "", None),
        (home, "manifest-link", "linked", "", None),
    ]
    for base, folder, name, extra, description in fixtures:
        skill(base, folder, name, extra, description)
    aliases = ["true", "TRUE", "yes", "YeS", "on", "ON", "1",
               "false", "FALSE", "no", "No", "off", "OFF", "0"]
    for index, alias in enumerate(aliases):
        for field in ["user-invocable", "disable-model-invocation"]:
            skill(home, f"{field}-{index}", f"{field}-{index}", f"{field}: {alias}\n")
    target = home / ".claude/skills/manifest-link/SKILL.md"
    target.rename(root / "linked.md")
    target.symlink_to(root / "linked.md")
    shared = skill(root / "shared", "source", "shared")
    for base in [home, repo]:
        (base / ".claude/skills/alias").symlink_to(shared.parent)
    plugin = home / ".claude/plugins/cache/market/demo/1"
    write(plugin / ".claude-plugin/plugin.json",
          json.dumps({"name": "demo", "version": "1.0.0"}))
    write(plugin / "skills/action/SKILL.md",
          "---\nname: action\ndescription: PLUGIN\n---\nInert instructions.\n")
    market = root / "market"
    write(market / "demo/.claude-plugin/plugin.json", json.dumps({"name": "demo", "version": "1.0.0"}))
    write(market / "demo/skills/action/SKILL.md", "---\nname: action\ndescription: LOCAL-SOURCE\n---\nInert instructions.\n")
    write(market / ".claude-plugin/marketplace.json", json.dumps({
        "name": "market", "owner": {"name": "Fixture"},
        "plugins": [{"name": "demo", "source": "./demo"}]}))
    write(home / ".claude/plugins/known_marketplaces.json", json.dumps({
        "market": {"source": {"source": "directory", "path": str(market)},
                   "installLocation": str(market),
                   "lastUpdated": "2026-10-13T00:00:00Z"}}))
    write(home / ".claude/plugins/installed_plugins.json", json.dumps({
        "version": 2, "plugins": {"demo@market": [{
            "scope": "user", "installPath": str(plugin), "version": "1.0.0",
            "installedAt": "2026-10-13T00:00:00Z",
            "lastUpdated": "2026-10-13T00:00:00Z"}]}}))
    root_plugin = market / "root-demo"
    write(root_plugin / ".claude-plugin/plugin.json", json.dumps({"name": "root-demo", "version": "1.0.0"}))
    write(root_plugin / "SKILL.md", "---\nname: root-action\ndescription: ROOT-PLUGIN-SKILL\n---\nInert.\n")
    marketplace_path = market / ".claude-plugin/marketplace.json"
    marketplace_data = json.loads(marketplace_path.read_text())
    marketplace_data["plugins"].append({"name": "root-demo", "source": "./root-demo"})
    write(marketplace_path, json.dumps(marketplace_data))
    registry_path = home / ".claude/plugins/installed_plugins.json"
    registry_data = json.loads(registry_path.read_text())
    registry_data["plugins"]["root-demo@market"] = [{"scope": "user", "installPath": str(root_plugin), "version": "1.0.0"}]
    write(registry_path, json.dumps(registry_data))
    user = home / ".claude/settings.json"
    project = cwd / ".claude/settings.json"
    local = cwd / ".claude/settings.local.json"
    configured = root / "configured"
    write(configured / "skills/alternate/SKILL.md",
          "---\nname: alternate\ndescription: CONFIGURED\n---\nInert.\n")
    environment = {
        "ANTHROPIC_API_KEY": "fixture-not-a-credential", "HOME": str(home),
        "CLAUDE_CONFIG_DIR": str(home / ".claude"), "PATH": "/usr/bin:/bin",
        "TMPDIR": str(root), "DISABLE_TELEMETRY": "1",
        "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1",
        "ANTHROPIC_BASE_URL": "http://127.0.0.1:9",
        "XDG_CONFIG_HOME": str(home / ".config"), "XDG_CACHE_HOME": str(home / ".cache"),
        "XCRUN_CACHE_PATH": str(root / "xcrun-cache"),
        "GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": str(root / "empty-git-config"),
    }
    write(root / "empty-git-config", "")
    containment = NativeContainment(root, cwd, environment, options.claude, output)
    containment.initialize_git(repo)
    args = ["--print", "--input-format", "stream-json",
            "--output-format", "stream-json", "--verbose", "--strict-mcp-config",
            "--mcp-config", '{"mcpServers":{}}', "--no-session-persistence"]
    version = subprocess.check_output(containment.command(["--version"]),
                                      cwd=cwd, env=environment, text=True, timeout=25).strip()
    assert version == "2.1.282 (Claude Code)", ("unsupported native version", version)
    cases = [
        ("baseline", {}, {}, {}),
        ("user-off", {"skillOverrides": {"collision": "off", "explicit": "off"}}, {}, {}),
        ("project-on", {"skillOverrides": {"collision": "off"}},
         {"skillOverrides": {"collision": "on"}}, {}),
        ("local-explicit", {"skillOverrides": {"collision": "off"}},
         {"skillOverrides": {"collision": "on"}},
         {"skillOverrides": {"collision": "user-invocable-only"}}),
        ("plugin-enabled", {"enabledPlugins": {"demo@market": True}}, {}, {}),
        ("plugin-project-on", {"enabledPlugins": {"demo@market": False}},
         {"enabledPlugins": {"demo@market": True}}, {}),
        ("plugin-local-off", {"enabledPlugins": {"demo@market": True}},
         {"enabledPlugins": {"demo@market": True}}, {"enabledPlugins": {"demo@market": False}}),
        ("plugin-disabled", {"enabledPlugins": {"demo@market": False}}, {}, {}),
        ("plugin-override", {"enabledPlugins": {"demo@market": True},
                             "skillOverrides": {"demo:action": "off"}}, {}, {}),
        ("plugin-session", {"skillOverrides": {"demo:action": "off"}}, {}, {}),
        ("ancestor-settings-ignored", {}, {}, {}),
        ("configured-root", {}, {}, {}),
        ("visibility-on", {"skillOverrides": {"model-only": "on", "explicit": "on"}}, {}, {}),
        ("visibility-user-only", {"skillOverrides": {
            "model-only": "user-invocable-only", "explicit": "user-invocable-only"}}, {}, {}),
        ("deny", {"permissions": {"deny": ["Skill(collision)"],
                                  "ask": ["Skill(explicit)"]}}, {}, {}),
        ("root-plugin-enabled", {"enabledPlugins": {"root-demo@market": True}}, {}, {}),
        ("root-plugin-disabled", {"enabledPlugins": {"root-demo@market": False}}, {}, {}),
    ]
    areas = [home / ".claude/skills", repo / ".claude", cwd / ".claude",
             plugin, configured / "skills", market / "demo", root_plugin, marketplace_path,
             shared.parent, root / "linked.md", user,
             home / ".claude/plugins/installed_plugins.json",
             home / ".claude/plugins/known_marketplaces.json"]
    results = []
    for label, user_data, project_data, local_data in cases:
        for path, value in [(user, user_data), (project, project_data), (local, local_data)]:
            write(path, json.dumps(value))
        if label == "ancestor-settings-ignored":
            write(repo / ".claude/settings.json",
                  json.dumps({"skillOverrides": {"collision": "off"}}))
        environment["CLAUDE_CONFIG_DIR"] = str(
            configured if label == "configured-root" else home / ".claude")
        before = manifest(areas, root)
        launch_args = list(args)
        if label == "plugin-session":
            launch_args += ["--plugin-dir", str(plugin)]
        launch = containment.command(launch_args)
        raw, stderr, response, exit_code = initialize(launch, cwd, environment)
        write(output / (label + ".json"), raw)
        write(output / (label + ".stderr"), stderr)
        after = manifest(areas, root)
        write(output / (label + "-before.json"), json.dumps(before, indent=2) + "\n")
        write(output / (label + "-after.json"), json.dumps(after, indent=2) + "\n")
        assert before == after, (label, "native fixture content mutated")
        registry_result = None
        if label in ["plugin-enabled", "plugin-disabled"]:
            completed = subprocess.run(containment.command(["plugin", "list", "--json"]),
                                       cwd=cwd, env=environment, capture_output=True,
                                       text=True, timeout=25)
            write(output / (label + "-registry.json"), completed.stdout)
            write(output / (label + "-registry.stderr"), completed.stderr)
            completed.check_returncode()
            registry_result = {"exit_code": completed.returncode,
                               "stdout_path": str(output / (label + "-registry.json"))}
        results.append({
            "case": label, "process_exit_after_catalog_termination": exit_code, "commands": [command for command in response["commands"]
                                         if not command.get("builtin")],
            "fixture_unchanged": True, "account": response["account"],
            "command": launch, "settings": {"user": user_data,
                                              "project": project_data,
                                              "local": local_data},
            "before_sha256": hashlib.sha256(json.dumps(before, sort_keys=True).encode()).hexdigest(),
            "after_sha256": hashlib.sha256(json.dumps(after, sort_keys=True).encode()).hexdigest(),
            "registry_check": registry_result,
        })
    commands = {result["case"]: result["commands"] for result in results}
    assert any(c["name"] == "nested-collision" and "CHILD" in c["description"]
               for c in commands["baseline"])
    assert any(c["name"] == "collision" and "PERSONAL" in c["description"]
               for c in commands["baseline"])
    assert not any(c["name"] == "collision" for c in commands["user-off"])
    assert any(c["name"] == "collision" for c in commands["project-on"])
    assert any(c["name"] == "collision" for c in commands["ancestor-settings-ignored"])
    assert any(c["name"] == "alternate" for c in commands["configured-root"])
    assert any(c["name"] == "demo:action" and "LOCAL-SOURCE" in c["description"] for c in commands["plugin-enabled"])
    assert any(c["name"] == "demo:action" for c in commands["plugin-project-on"])
    assert not any(c["name"] == "demo:action" for c in commands["plugin-local-off"])
    assert any(c["name"] == "demo:action" for c in commands["plugin-override"])
    assert not any(c["name"] == "demo:action" for c in commands["plugin-disabled"])
    assert any(c["name"] == "demo:action" for c in commands["plugin-session"])
    assert not any(c["name"] == "model-only" for c in commands["visibility-user-only"])
    assert not any(c["name"] in ["deep", "outside", "below", "reserved-synced"]
                   for c in commands["baseline"])
    assert any(c["name"] == "root-demo:root-action" and "ROOT-PLUGIN-SKILL" in c["description"]
               for c in commands["root-plugin-enabled"])
    assert not any(c["name"] == "root-demo:root-action" for c in commands["root-plugin-disabled"])
    for index, alias in enumerate(aliases):
        listed = any(c["name"] == f"user-invocable-{index}" for c in commands["baseline"])
        assert listed == (alias.lower() in {"true", "yes", "on", "1"}), (alias, listed)
        # Catalog inclusion observes user invocation only, not model enforcement.
        assert any(c["name"] == f"disable-model-invocation-{index}" for c in commands["baseline"])
    summary = {
        "version": version, "requested_binary": options.claude,
        "helper_sha256": hashlib.sha256(pathlib.Path(__file__).read_bytes()).hexdigest(),
        "containment_helper_sha256": hashlib.sha256((pathlib.Path(__file__).parent / "claude_containment.py").read_bytes()).hexdigest(),
        "fixture": str(root), "network_denied": True, "containment": str(output / "containment.json"), "model_prompts": 0,
        "skill_workloads": 0, "environment_keys": sorted(environment), "results": results,
    }
    write(output / "summary.json", json.dumps(summary, indent=2) + "\n")
    print(json.dumps(summary, indent=2))


if __name__ == "__main__":
    main()
