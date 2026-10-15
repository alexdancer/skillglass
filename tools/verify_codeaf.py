#!/usr/bin/env python3
"""Probe pinned CodeAF source offline after fail-closed disposable containment."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import socket
import subprocess
import sys
import tempfile

REVISION = "7e30335645c8c792849de1c13b3547d75775baa4"
GO = Path("/opt/homebrew/bin/go").resolve()
GIT = Path("/Library/Developer/CommandLineTools/usr/bin/git").resolve()
OS_READS = [Path("/usr/lib"), Path("/System/Library/Frameworks"),
            Path("/System/Volumes/Preboot/Cryptexes/OS/System/Library/dyld"),
            Path("/System/Volumes/Preboot/Cryptexes/OS/usr/lib")]


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def profile(reads, writes, executables, fork=False):
    # Paths are JSON quoted, never interpolated as profile syntax.
    rules = ["(version 1)", "(allow default)", "(deny network*)", "(deny file-read*)",
             "(deny file-write*)", "(deny process-exec*)"]
    if not fork:
        rules.extend(["(deny process-fork)", "(deny mach-lookup)"])
    for path in [*OS_READS, *reads]:
        rules.append("(allow file-read* (subpath " + json.dumps(str(path)) + "))")
        for parent in path.parents:
            rules.append("(allow file-read-metadata (literal " + json.dumps(str(parent)) + "))")
    for path in writes:
        rules.append("(allow file-write* (subpath " + json.dumps(str(path)) + "))")
    for path in executables:
        rules.append("(allow process-exec (literal " + json.dumps(str(path)) + "))")
    rules.append('(allow file-read* file-write* (literal "/dev/null"))')
    rules.append('(allow file-read-data (literal "/"))')
    rules.append('(allow file-read* (literal "/dev/urandom") (literal "/dev/random"))')
    return "\n".join(rules) + "\n"


class Runner:
    def __init__(self, env, receipts):
        self.env, self.receipts = env, receipts

    def run(self, argv, cwd, policy=None, pinned=None):
        argv = [str(arg) for arg in argv]
        if pinned is not None and digest(Path(argv[0])) != pinned:
            raise RuntimeError("executable bytes changed before invocation")
        command = argv
        if policy is not None:
            path, expected = policy
            if digest(path) != expected:
                raise RuntimeError("containment profile changed before invocation")
            command = ["/usr/bin/sandbox-exec", "-f", str(path), *argv]
        result = subprocess.run(command, cwd=cwd, env=self.env, text=True, capture_output=True, timeout=120)
        receipt = {"argv": command, "cwd": str(cwd), "exit": result.returncode,
                   "stdout": result.stdout, "stderr": result.stderr,
                   "profile_sha256": policy[1] if policy else None,
                   "executable_sha256": pinned}
        self.receipts.append(receipt)
        if result.returncode:
            raise RuntimeError(json.dumps(receipt))
        return result.stdout


def save_policy(directory, name, content):
    path = directory / (name + ".sb")
    path.write_text(content)
    path.chmod(0o600)
    return path, digest(path)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--source", required=True)
    parser.add_argument("--binary", required=True)
    parser.add_argument("--output", required=True)
    args = parser.parse_args()
    if sys.platform != "darwin" or not Path("/usr/bin/sandbox-exec").is_file():
        raise RuntimeError("Darwin sandbox required; no uncontained fallback")
    source = Path(args.source).resolve()
    supplied = Path(args.binary).resolve(strict=True)
    output = Path(args.output).resolve()
    output.mkdir(parents=True, exist_ok=True)
    receipts = []
    try:
        with tempfile.TemporaryDirectory(prefix="ale-39-native-", dir="/private/tmp") as case_name:
            case = Path(case_name)
            fixtures, immutable, build = [case / name for name in ("fixtures", "immutable", "build")]
            for path in (fixtures, immutable, build):
                path.mkdir()
            home, project, override = [fixtures / name for name in ("home", "project", "override")]
            for path in (home, project, override, fixtures / "tmp", fixtures / "profile"):
                path.mkdir()
            env = {"HOME": str(home), "CODEAF_HOME": str(override), "CODEAF_PROFILE_DIR": str(fixtures / "profile"),
                   "CODEAF_TELEMETRY": "off", "DO_NOT_TRACK": "1", "PATH": "/usr/bin:/bin",
                   "XDG_CONFIG_HOME": str(fixtures / "config"), "XDG_CACHE_HOME": str(fixtures / "cache"),
                   "XDG_DATA_HOME": str(fixtures / "data"), "XDG_STATE_HOME": str(fixtures / "state"),
                   "TMPDIR": str(fixtures / "tmp"), "GOTMPDIR": str(fixtures / "tmp"),
                   "GOCACHE": str(build / "go-cache"), "GOMODCACHE": "/Users/alex/go/pkg/mod",
                   "GOPROXY": "off", "GOSUMDB": "off", "GOTOOLCHAIN": "local", "GOENV": "off", "CGO_ENABLED": "0", "LC_ALL": "C",
                   "GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_SYSTEM": "/dev/null", "GIT_CONFIG_GLOBAL": "/dev/null",
                   "GIT_CONFIG_COUNT": "0", "GIT_ATTR_NOSYSTEM": "1", "GIT_TERMINAL_PROMPT": "0"}
            runner = Runner(env, receipts)
            # Only data is copied here. Supplied bytes cannot run during bootstrap.
            binary = immutable / "supplied-binary"
            binary.write_bytes(supplied.read_bytes())
            binary.chmod(0o500)
            binary_hash = digest(binary)
            probe = immutable / "boundary-probe"
            child = immutable / "inert-child"
            native = immutable / "native-probe"
            probe_source = build / "boundary.go"
            probe_source.write_bytes((Path(__file__).parent / "native-verification/boundary.go.txt").read_bytes())
            toolchain = GO.parent.parent
            build_policy = save_policy(immutable, "build", profile(
                [build, fixtures, immutable, toolchain,
                 Path(env["GOMODCACHE"]) / "gopkg.in/yaml.v3@v3.0.1",
                 Path(env["GOMODCACHE"]) / "cache/download/gopkg.in/yaml.v3"],
                [build, fixtures / "tmp", immutable], [GO, toolchain / "pkg/tool/darwin_arm64/compile",
                toolchain / "pkg/tool/darwin_arm64/asm", toolchain / "pkg/tool/darwin_arm64/link"], fork=True))
            runner.run([GO, "build", "-o", probe, probe_source], build, build_policy)
            probe.chmod(0o500)
            shutil.copyfile(probe, child)
            child.chmod(0o500)
            probe_hash = digest(probe)
            runtime_policy = save_policy(immutable, "runtime", profile(
                [fixtures, immutable], [fixtures], [binary, probe, child, native]))
            # The positive control's outside directory is disposable and controlled,
            # but excluded from runtime reads, metadata, writes and symlink traversal.
            with tempfile.TemporaryDirectory(prefix="ale-39-excluded-", dir="/private/tmp") as excluded_name:
                excluded = Path(excluded_name)
                canary = excluded / "canary"
                canary.write_text("CONTROLLED_OUTSIDE_CANARY\n")
                allowed = project / "allowed-read"
                allowed.write_text("allowed\n")
                (project / "outside-link").symlink_to(canary)
                with socket.socket() as listener:
                    listener.bind(("127.0.0.1", 0))
                    listener.listen()
                    address = "127.0.0.1:" + str(listener.getsockname()[1])
                    argv = [probe, canary, excluded, child, excluded / "write", allowed,
                            project / "outside-link", project / "inside-write", address, runtime_policy[0]]
                    positive = json.loads(runner.run(argv, project, pinned=probe_hash))
                    if not all(positive.values()):
                        raise RuntimeError("containment positive controls failed: " + json.dumps(positive))
                    (excluded / "write").unlink()
                    negative = json.loads(runner.run(argv, project, runtime_policy, probe_hash))
                    expected = {key: key in ("fixture_read", "inside_write") for key in positive}
                    if negative != expected:
                        raise RuntimeError("containment denial controls failed: " + json.dumps(negative))
            containment = {"passed": True, "positive": positive, "contained": negative,
                           "runtime_profile": runtime_policy[0].read_text(), "runtime_profile_sha256": runtime_policy[1],
                           "build_profile": build_policy[0].read_text(), "build_profile_sha256": build_policy[1],
                           "probe_sha256": probe_hash, "binary_sha256": binary_hash}
            (output / "containment.json").write_text(json.dumps(containment, indent=2) + "\n")
            git_policy = save_policy(immutable, "git", profile([source, GIT], [], [GIT]))
            revision = runner.run([GIT, "--no-optional-locks", "rev-parse", "HEAD"], source, git_policy)
            assert revision.strip() == REVISION
            metadata = runner.run([GO, "version", "-m", binary], build, build_policy)
            binary_version = runner.run([binary, "version"], project, runtime_policy, binary_hash)
            module = build / "probe"
            module.mkdir()
            (module / "go.mod").write_text("module github.com/Agent-Field/codeaf\n\ngo 1.27.1\n\nrequire gopkg.in/yaml.v3 v3.0.1\n")
            def source_bytes(file):
                resolved = file.resolve(strict=True)
                if not resolved.is_relative_to(source) or not resolved.is_file():
                    raise RuntimeError("source file escapes selected source checkout: " + str(file))
                return resolved.read_bytes()

            go_sum = source_bytes(source / "go.sum")
            pinned_sum = runner.run([GIT, "--no-optional-locks", "show", REVISION + ":go.sum"], source, git_policy)
            if go_sum != pinned_sum.encode():
                raise RuntimeError("go.sum differs from the pinned revision")
            (module / "go.sum").write_bytes(go_sum)
            hashes = []
            # These are unmodified native files, compiled without resident/store/model code.
            for package in ("skills", "home", "env"):
                target = module / "internal" / package
                target.mkdir(parents=True)
                for file in sorted((source / "internal" / package).glob("*.go")):
                    if file.name.endswith("_test.go"):
                        continue
                    content = source_bytes(file)
                    relative = file.relative_to(source)
                    pinned = runner.run([GIT, "--no-optional-locks", "show", REVISION + ":" + str(relative)], source, git_policy)
                    assert content == pinned.encode(), str(relative) + " differs from the pinned revision"
                    (target / file.name).write_bytes(content)
                    hashes.append({"path": str(relative), "sha256": hashlib.sha256(content).hexdigest(), "matches_revision": True})
            (module / "main.go").write_text('''package main
import("encoding/json";"os";"github.com/Agent-Field/codeaf/internal/skills";"github.com/Agent-Field/codeaf/internal/home")
func main(){login,err:=home.Login();if err!=nil{panic(err)};found,err:=skills.Discover(skills.Options{ProjectDir:os.Args[1],HomeDir:login});if err!=nil{panic(err)};json.NewEncoder(os.Stdout).Encode(struct{Login string;Skills []skills.Skill}{login,found})}
''')
            runner.run([GO, "build", "-mod=readonly", "-o", native, "."], module, build_policy)
            native.chmod(0o500)
            native_hash = digest(native)
            def run(argv):
                return runner.run(argv, project, runtime_policy, native_hash)

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
            shared = skill(fixtures / "shared", "bundle", "shared-native")
            (override / ".agents/skills").mkdir(parents=True)
            (override / ".agents/skills/link").symlink_to(shared, target_is_directory=True)
            plugin = fixtures / "plugin"
            skill(plugin / "skills", "attached", "plugin-native")
            registry = override / ".claude/plugins/installed_plugins.json"
            registry.parent.mkdir(parents=True)
            registry.write_text(json.dumps({"plugins": {"example@local": [{"scope": "user", "installPath": str(plugin), "version": "1"}]}}))
            settings = override / ".claude/settings.json"
            settings.write_text(json.dumps({"enabledPlugins": {"example@local": True}}))
            before = {str(p): hashlib.sha256(p.read_bytes()).hexdigest() for p in fixtures.rglob("SKILL.md") if p.is_file()}
            initial = json.loads(run([native, project]))
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
            disabled = json.loads(run([native, project]))
            assert "plugin-native" not in [s["Name"] for s in disabled["Skills"]]
            # A profile change and Codex per-skill setting cannot disable ordinary imports.
            (fixtures / "profile/config.json").write_text('{"memory.enabled":false}')
            codex = override / ".codex/config.toml"
            codex.parent.mkdir(parents=True, exist_ok=True)
            codex.write_text('[[skills.config]]\npath="' + str(shared / "SKILL.md") + '"\nenabled=false\n')
            unaffected = json.loads(run([native, project]))
            assert "shared-native" in [s["Name"] for s in unaffected["Skills"]]
            after = {str(p): hashlib.sha256(p.read_bytes()).hexdigest() for p in fixtures.rglob("SKILL.md") if p.is_file()}
            assert before == after
            # A full native discovery CLI is unavailable; never start chat to emulate one.
            report = {"source_revision": REVISION, "source_files": hashes, "binary_sha256": binary_hash, "containment": containment, "native_probe_sha256": native_hash, "source_go_sum_sha256": hashlib.sha256(go_sum).hexdigest(),
                      "binary_metadata": metadata, "binary_version": binary_version, "source_probe": initial,
                      "plugin_off_probe": disabled, "unrelated_settings_probe": unaffected,
                      "skill_fixtures_unchanged": True, "environment_keys": sorted(env), "receipts": receipts,
                      "gaps": ["Source probe validates copied unmodified loader/home/env files, not equivalence with installed +dirty binary.",
                               "No conversation, attachment, model, resident or plugin execution was launched. Runtime skill discovery in the installed binary remains unobserved.",
                               "Persistent per-skill disable is unsupported; detachment is not exercised or claimed as a disable.",
                               "Child creation is denied, including spawning the same executable. In-process replacement by an allowed pinned image is not prevented; the kernel sandbox remains inherited.",
                               "The version command establishes its reported identifier only, never full installed discovery or source equivalence."]}
            (output / "native.json").write_text(json.dumps(report, indent=2) + "\n")
    finally:
        (output / "receipts.json").write_text(json.dumps(receipts, indent=2) + "\n")
    print(output / "native.json")


if __name__ == "__main__":
    main()
