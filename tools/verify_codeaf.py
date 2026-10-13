#!/usr/bin/env python3
"""Probe a pinned CodeAF source loader offline without launching a conversation."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile

REVISION = "7e30335645c8c792849de1c13b3547d75775baa4"


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--source", required=True)
    parser.add_argument("--binary", required=True)
    parser.add_argument("--output", required=True)
    args = parser.parse_args()
    source = Path(args.source).resolve()
    output = Path(args.output).resolve()
    output.mkdir(parents=True, exist_ok=True)
    go = Path("/opt/homebrew/bin/go")
    with tempfile.TemporaryDirectory(prefix="ale-39-native-", dir="/private/tmp") as case:
        case = Path(case)
        home, project, override = [case / name for name in ("home", "project", "override")]
        for path in (home, project, override, case / "tmp", case / "profile"):
            path.mkdir()
        env = {"HOME": str(home), "CODEAF_HOME": str(override), "CODEAF_PROFILE_DIR": str(case / "profile"),
               "CODEAF_TELEMETRY": "off", "DO_NOT_TRACK": "1", "PATH": "/usr/bin:/bin",
               "XDG_CONFIG_HOME": str(case / "config"), "XDG_CACHE_HOME": str(case / "cache"),
               "XDG_DATA_HOME": str(case / "data"), "XDG_STATE_HOME": str(case / "state"),
               "TMPDIR": str(case / "tmp"), "GOTMPDIR": str(case / "tmp"),
               "GOCACHE": str(case / "go-cache"), "GOMODCACHE": "/Users/alex/go/pkg/mod",
               "GOPROXY": "off", "GOSUMDB": "off", "GOTOOLCHAIN": "local", "LC_ALL": "C"}
        receipts = []

        def run(argv, cwd=project, contained=True):
            argv = [str(arg) for arg in argv]
            command = argv
            if contained:
                profile = '(version 1) (allow default) (deny network*) (deny file-write*) (allow file-write* (literal "/dev/null")) (allow file-write* (subpath "' + str(case) + '"))'
                command = ["/usr/bin/sandbox-exec", "-p", profile, *argv]
            result = subprocess.run(command, cwd=cwd, env=env, text=True, capture_output=True, timeout=120)
            receipts.append({"argv": command, "cwd": str(cwd), "exit": result.returncode,
                             "stdout": result.stdout, "stderr": result.stderr})
            if result.returncode:
                raise RuntimeError(json.dumps(receipts[-1]))
            return result.stdout

        revision = run(["/usr/bin/git", "rev-parse", "HEAD"], source, contained=False)
        assert revision.strip() == REVISION
        metadata = run([go, "version", "-m", args.binary])
        binary_version = run([args.binary, "version"])
        module = case / "probe"
        module.mkdir()
        (module / "go.mod").write_text("module github.com/Agent-Field/codeaf\n\ngo 1.27.1\n\nrequire gopkg.in/yaml.v3 v3.0.1\n")
        shutil.copy(source / "go.sum", module / "go.sum")
        hashes = []
        # These are unmodified native files, compiled without resident/store/model code.
        for package in ("skills", "home", "env"):
            target = module / "internal" / package
            target.mkdir(parents=True)
            for file in sorted((source / "internal" / package).glob("*.go")):
                if file.name.endswith("_test.go"):
                    continue
                content = file.read_bytes()
                relative = file.relative_to(source)
                pinned = subprocess.run(["/usr/bin/git", "show", REVISION + ":" + str(relative)], cwd=source,
                                        env=env, capture_output=True, check=True).stdout
                assert content == pinned, str(relative) + " differs from the pinned revision"
                shutil.copy(file, target / file.name)
                hashes.append({"path": str(relative), "sha256": hashlib.sha256(content).hexdigest(), "matches_revision": True})
        (module / "main.go").write_text('''package main
import("encoding/json";"os";"github.com/Agent-Field/codeaf/internal/skills";"github.com/Agent-Field/codeaf/internal/home")
func main(){login,err:=home.Login();if err!=nil{panic(err)};found,err:=skills.Discover(skills.Options{ProjectDir:os.Args[1],HomeDir:login});if err!=nil{panic(err)};json.NewEncoder(os.Stdout).Encode(struct{Login string;Skills []skills.Skill}{login,found})}
''')
        run([go, "build", "-mod=readonly", "-o", case / "native-probe", "."], module)

        def skill(root, folder, name):
            directory = root / folder
            directory.mkdir(parents=True, exist_ok=True)
            (directory / "SKILL.md").write_text("---\nname: " + name + "\ndescription: probe fixture\n---\nNever execute this fixture.\n")
            return directory

        winner = skill(project / ".codeaf/skills", "local", "collision")
        skill(project / ".agents/skills", "shadow", "collision")
        skill(override / ".codeaf/skills", "global", "collision")
        skill(home / ".codeaf/skills", "ignored", "wrong-home")
        skill(project / ".agents/skills", "container/deep", "too-deep")
        shared = skill(case / "shared", "bundle", "shared-native")
        (override / ".agents/skills").mkdir(parents=True)
        (override / ".agents/skills/link").symlink_to(shared, target_is_directory=True)
        plugin = case / "plugin"
        skill(plugin / "skills", "attached", "plugin-native")
        registry = override / ".claude/plugins/installed_plugins.json"
        registry.parent.mkdir(parents=True)
        registry.write_text(json.dumps({"plugins": {"example@local": [{"scope": "user", "installPath": str(plugin), "version": "1"}]}}))
        settings = override / ".claude/settings.json"
        settings.write_text(json.dumps({"enabledPlugins": {"example@local": True}}))
        before = {str(p): hashlib.sha256(p.read_bytes()).hexdigest() for p in case.rglob("SKILL.md") if p.is_file()}
        initial = json.loads(run([case / "native-probe", project]))
        names = [s["Name"] for s in initial["Skills"]]
        assert initial["Login"] == str(override)
        assert "wrong-home" not in names and "too-deep" not in names
        collisions = [s for s in initial["Skills"] if s["Name"] == "collision"]
        assert [s["Shadowed"] for s in collisions] == [False, True, True]
        assert collisions[0]["Dir"] == str(winner)
        assert "shared-native" in names and "plugin-native" in names
        local = project / ".claude/settings.local.json"
        local.parent.mkdir(parents=True, exist_ok=True)
        local.write_text(json.dumps({"enabledPlugins": {"example@local": False}}))
        disabled = json.loads(run([case / "native-probe", project]))
        assert "plugin-native" not in [s["Name"] for s in disabled["Skills"]]
        # A profile change and Codex per-skill setting cannot disable ordinary imports.
        (case / "profile/config.json").write_text('{"memory.enabled":false}')
        codex = override / ".codex/config.toml"
        codex.parent.mkdir(parents=True, exist_ok=True)
        codex.write_text('[[skills.config]]\npath="' + str(shared / "SKILL.md") + '"\nenabled=false\n')
        unaffected = json.loads(run([case / "native-probe", project]))
        assert "shared-native" in [s["Name"] for s in unaffected["Skills"]]
        after = {str(p): hashlib.sha256(p.read_bytes()).hexdigest() for p in case.rglob("SKILL.md") if p.is_file()}
        assert before == after
        # A full native discovery CLI is unavailable; never start chat to emulate one.
        report = {"source_revision": REVISION, "source_files": hashes, "binary_sha256": hashlib.sha256(Path(args.binary).read_bytes()).hexdigest(),
                  "binary_metadata": metadata, "binary_version": binary_version, "source_probe": initial,
                  "plugin_off_probe": disabled, "unrelated_settings_probe": unaffected,
                  "skill_fixtures_unchanged": True, "environment_keys": sorted(env), "receipts": receipts,
                  "gaps": ["Source probe validates copied unmodified loader/home/env files, not equivalence with installed +dirty binary.",
                           "No conversation, attachment, model, resident or plugin execution was launched. Runtime skill discovery in the installed binary remains unobserved.",
                           "Persistent per-skill disable is unsupported; detachment is not exercised or claimed as a disable."]}
        (output / "native.json").write_text(json.dumps(report, indent=2) + "\n")
    print(output / "native.json")


if __name__ == "__main__":
    main()
