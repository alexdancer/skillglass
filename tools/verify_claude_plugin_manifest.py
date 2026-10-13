#!/usr/bin/env python3
"""Validate bounded plugin homepage controls without prompts or workloads."""
import argparse
import hashlib
import json
import pathlib
import subprocess
import tempfile

from claude_containment import NativeContainment
from verify_claude import initialize, manifest, write


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("output", type=pathlib.Path)
    parser.add_argument("--claude", default="/Users/alex/.local/bin/claude")
    options = parser.parse_args()
    output = options.output.resolve()
    output.mkdir(parents=True, exist_ok=True)
    root = pathlib.Path(tempfile.mkdtemp(prefix="ale29-manifest-", dir="/private/tmp")).resolve()
    home, repo = root / "home", root / "repo"
    home.mkdir()
    repo.mkdir()
    plugin = root / "market/demo"
    write(plugin / "SKILL.md", "---\nname: root-action\ndescription: INERT-HOMEPAGE-CONTROL\n---\nNever execute anything.\n")
    write(root / "market/.claude-plugin/marketplace.json", json.dumps({"name": "market", "owner": {"name": "Fixture"}, "plugins": [{"name": "demo", "source": "./demo"}]}))
    write(home / ".claude/plugins/known_marketplaces.json", json.dumps({"market": {"source": {"source": "directory", "path": str(root / "market")}, "installLocation": str(root / "market")}}))
    write(home / ".claude/plugins/installed_plugins.json", json.dumps({"version": 2, "plugins": {"demo@market": [{"scope": "user", "installPath": str(plugin), "version": "1.0.0"}]}}))
    write(home / ".claude/settings.json", json.dumps({"enabledPlugins": {"demo@market": True}}))
    write(root / "empty-git-config", "")
    environment = {"HOME": str(home), "CLAUDE_CONFIG_DIR": str(home / ".claude"),
                   "PATH": "/usr/bin:/bin", "TMPDIR": str(root),
                   "ANTHROPIC_API_KEY": "fixture-not-a-credential",
                   "DISABLE_TELEMETRY": "1", "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1",
                   "ANTHROPIC_BASE_URL": "http://127.0.0.1:9",
                   "XDG_CONFIG_HOME": str(home / ".config"), "XDG_CACHE_HOME": str(home / ".cache"),
                   "XCRUN_CACHE_PATH": str(root / "xcrun-cache"), "GIT_CONFIG_NOSYSTEM": "1",
                   "GIT_CONFIG_GLOBAL": str(root / "empty-git-config")}
    containment = NativeContainment(root, repo, environment, options.claude, output)
    containment.initialize_git(repo)
    version = subprocess.run(containment.command(["--version"]), cwd=repo, env=environment,
                             capture_output=True, text=True, timeout=25, check=True)
    write(output / "version.stdout", version.stdout)
    write(output / "version.stderr", version.stderr)
    assert version.stdout.strip() == "2.1.282 (Claude Code)", version.stdout
    args = ["--print", "--input-format", "stream-json", "--output-format", "stream-json",
            "--verbose", "--strict-mcp-config", "--mcp-config", '{"mcpServers":{}}', "--no-session-persistence"]
    areas = [plugin, root / "market/.claude-plugin", home / ".claude/settings.json",
             home / ".claude/plugins/known_marketplaces.json", home / ".claude/plugins/installed_plugins.json"]
    results = []
    for label, homepage in [("absent", None), ("valid", "https://example.invalid/docs"), ("invalid", "not-an-absolute-url")]:
        metadata = {"name": "demo", "version": "1.0.0"}
        if homepage is not None:
            metadata["homepage"] = homepage
        write(plugin / ".claude-plugin/plugin.json", json.dumps(metadata))
        before = manifest(areas, root)
        validation = subprocess.run(containment.command(["plugin", "validate", str(plugin)]),
                                    cwd=repo, env=environment, capture_output=True, text=True, timeout=25)
        write(output / (label + "-validate.stdout"), validation.stdout)
        write(output / (label + "-validate.stderr"), validation.stderr)
        raw, stderr, response, code = initialize(containment.command(args), repo, environment)
        write(output / (label + "-catalog.json"), raw)
        write(output / (label + "-catalog.stderr"), stderr)
        after = manifest(areas, root)
        write(output / (label + "-before.json"), json.dumps(before, indent=2) + "\n")
        write(output / (label + "-after.json"), json.dumps(after, indent=2) + "\n")
        assert before == after, label
        commands = [c for c in response["commands"] if not c.get("builtin")]
        listed = any(c["name"] == "demo:root-action" for c in commands)
        assert listed == (label != "invalid"), (label, commands)
        assert validation.returncode == (1 if label == "invalid" else 0), (label, validation.stderr)
        if label == "invalid":
            assert "homepage" in validation.stdout + validation.stderr
        results.append({"case": label, "manifest": metadata, "validate_exit": validation.returncode,
                        "commands": commands, "termination_after_catalog_exit": code,
                        "fixture_unchanged": True,
                        "before_sha256": hashlib.sha256(json.dumps(before, sort_keys=True).encode()).hexdigest(),
                        "after_sha256": hashlib.sha256(json.dumps(after, sort_keys=True).encode()).hexdigest()})
    summary = {"version": version.stdout.strip(), "results": results, "fixture": str(root),
               "model_prompts": 0, "skill_workloads": 0,
               "helper_sha256": hashlib.sha256(pathlib.Path(__file__).read_bytes()).hexdigest(),
               "containment_helper_sha256": hashlib.sha256((pathlib.Path(__file__).parent / "claude_containment.py").read_bytes()).hexdigest(),
               "limits": "Only absent/valid HTTPS/invalid literal homepage controls observed. No full native schema equivalence, model enforcement or enterprise-state claim."}
    write(output / "summary.json", json.dumps(summary, indent=2) + "\n")
    print(json.dumps(summary, indent=2))


if __name__ == "__main__":
    main()
