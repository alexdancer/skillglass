#!/usr/bin/env python3
"""Validate startup catalogs without model requests or skill execution."""

import argparse
import hashlib
import json
import pathlib
import select
import subprocess
import tempfile


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
        return raw, stderr, response
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
    root = pathlib.Path(tempfile.mkdtemp(prefix="ale29-native-", dir="/tmp"))
    home, repo = root / "home", root / "repo"
    cwd = repo / "nested"
    home.mkdir()
    cwd.mkdir(parents=True)
    subprocess.run(["/usr/bin/git", "init", "-q", str(repo)], check=True)
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
    }
    sandbox = root / "native-offline.sb"
    sandbox_text = "(version 1)\n(allow default)\n(deny network*)\n"
    write(sandbox, sandbox_text)
    write(output / "native-offline.sb", sandbox_text)
    prefix = ["/usr/bin/sandbox-exec", "-f", str(sandbox)]
    args = [options.claude, "--print", "--input-format", "stream-json",
            "--output-format", "stream-json", "--verbose", "--strict-mcp-config",
            "--mcp-config", '{"mcpServers":{}}', "--no-session-persistence"]
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
    ]
    areas = [home / ".claude/skills", repo / ".claude", cwd / ".claude",
             plugin, configured / "skills", market / "demo", user,
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
        launch = prefix + args
        if label == "plugin-session":
            launch += ["--plugin-dir", str(plugin)]
        raw, stderr, response = initialize(launch, cwd, environment)
        write(output / (label + ".json"), raw)
        write(output / (label + ".stderr"), stderr)
        after = manifest(areas, root)
        write(output / (label + "-before.json"), json.dumps(before, indent=2) + "\n")
        write(output / (label + "-after.json"), json.dumps(after, indent=2) + "\n")
        assert before == after, (label, "native fixture content mutated")
        registry_result = None
        if label in ["plugin-enabled", "plugin-disabled"]:
            completed = subprocess.run(prefix + [options.claude, "plugin", "list", "--json"],
                                       cwd=cwd, env=environment, capture_output=True,
                                       text=True, timeout=25)
            write(output / (label + "-registry.json"), completed.stdout)
            write(output / (label + "-registry.stderr"), completed.stderr)
            registry_result = {"exit_code": completed.returncode,
                               "stdout_path": str(output / (label + "-registry.json"))}
        results.append({
            "case": label, "commands": [command for command in response["commands"]
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
    summary = {
        "version": subprocess.check_output([options.claude, "--version"], text=True).strip(),
        "fixture": str(root), "network_denied": True, "model_prompts": 0,
        "skill_workloads": 0, "environment_keys": sorted(environment), "results": results,
    }
    write(output / "summary.json", json.dumps(summary, indent=2) + "\n")
    print(json.dumps(summary, indent=2))


if __name__ == "__main__":
    main()
