package acceptance_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/alexdancer/skillglass/internal/companion"
	"github.com/alexdancer/skillglass/internal/setup"
)

func ale48Fixture(t *testing.T) fixture {
	fx := newFixture(t)
	fx.xdgConfig = filepath.Join(fx.home, ".config")
	if err := os.MkdirAll(fx.xdgConfig, 0700); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(fx.home, ".codex/config.toml"), []byte("# unrelated setting\n"), 0600)
	writeFile(t, filepath.Join(fx.home, ".claude/settings.json"), []byte("{\"keep\":true}"), 0600)
	bin := filepath.Join(fx.home, "bin")
	if err := os.MkdirAll(bin, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(skillglassBinary, filepath.Join(bin, "skillglass")); err != nil {
		t.Fatal(err)
	}
	fx.seal(t)
	return fx
}
func ale48Run(t *testing.T, fx fixture, policy, path string, args ...string) commandResult {
	return ale48RunEnv(t, fx, policy, path, nil, args...)
}
func ale48RunEnv(t *testing.T, fx fixture, policy, path string, extraEnv []string, args ...string) commandResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	argv := fx.launchArgs(t, skillglassBinary, true, policy, args...)
	command := exec.CommandContext(ctx, argv[0], argv[1:]...)
	command.Dir = fx.project
	command.Env = append(append(fx.environment(), "PATH="+path), extraEnv...)
	result, err := executeFinalizedCommand(t, command)
	if err != nil {
		if _, ok := err.(*exec.ExitError); !ok {
			t.Fatal(err)
		}
	}
	recordEvidenceCommand(t, "setup-public-contained", result)
	if ctx.Err() != nil {
		t.Fatal("setup timed out")
	}
	return result
}
func ale48Setup(t *testing.T, fx fixture, install bool, want int, apps ...string) setup.SetupReport {
	t.Helper()
	args := []string{"setup", "--json"}
	for _, app := range apps {
		if app == "codeaf" {
			args = append(args, "--allow-shared-app", app)
		} else {
			args = append(args, "--app", app)
		}
	}
	if install {
		args = append(args, "--install")
	}
	r := ale48Run(t, fx, "", filepath.Join(fx.home, "bin"), args...)
	assertExit(t, r, want)
	if len(r.stderr) != 0 {
		t.Fatalf("machine stderr %s", r.stderr)
	}
	var report setup.SetupReport
	decodeJSON(t, r.stdout, &report)
	return report
}
func ale48AssertBundle(t *testing.T, path string) {
	t.Helper()
	for _, f := range companion.Bundled().Files {
		if !bytes.Equal(readBytes(t, filepath.Join(path, f.Path)), []byte(f.Content)) {
			t.Fatalf("bundle differs: %s", f.Path)
		}
	}
}
func TestALE48SetupSelectedAppsIdempotencyAndConflict(t *testing.T) {
	fx := ale48Fixture(t)
	before := treeManifest(t, fx.base)
	p := ale48Setup(t, fx, false, 0, "codex", "opencode", "codeaf")
	if p.State != "preview" || len(p.Results) != 2 {
		t.Fatal(p)
	}
	for _, r := range p.Results {
		if r.Application == "codeaf" {
			continue
		}
		if r.State != "ready" {
			t.Fatalf("not ready: %+v", r)
		}
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	out := ale48Setup(t, fx, true, 0, "codex", "opencode", "codeaf")
	ale48Preserved(t, fx, before, out.Results)
	for _, r := range out.Results {
		if r.Application == "codeaf" {
			continue
		}
		if r.State != "installed" {
			t.Fatal(r)
		}
		ale48AssertBundle(t, r.Destination)
	}
	if _, err := os.Stat(filepath.Join(fx.home, ".claude/skills/skillglass-review")); !os.IsNotExist(err) {
		t.Fatal("unselected Claude changed", err)
	}
	if _, err := os.Stat(filepath.Join(fx.home, ".agents/skills/skillglass-review")); !os.IsNotExist(err) {
		t.Fatal("shared root changed", err)
	}
	stable := treeManifest(t, fx.base)
	again := ale48Setup(t, fx, true, 0, "codex", "opencode", "codeaf")
	for _, r := range again.Results {
		if r.Application == "codeaf" {
			continue
		}
		if r.State != "already-present" {
			t.Fatal(r)
		}
	}
	assertTreeUnchanged(t, stable, treeManifest(t, fx.base))
	custom := filepath.Join(out.Results[0].Destination, "SKILL.md")
	writeFile(t, custom, []byte("CUSTOMIZED"), 0600)
	before = treeManifest(t, fx.base)
	conflict := ale48Setup(t, fx, true, 1, "codex", "codeaf")
	if conflict.Results[0].State != "refused" || !strings.Contains(conflict.Results[0].Reason, "existing companion differs") {
		t.Fatal(conflict)
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
}
func TestALE48SharedRootRequiresExplicitAffectedSelection(t *testing.T) {
	fx := ale48Fixture(t)
	before := treeManifest(t, fx.base)
	p := ale48Setup(t, fx, true, 1, "claude-code")
	if p.Results[0].State != "refused" || !bytes.Contains([]byte(p.Results[0].Reason), []byte("unacknowledged shared consumers: codeaf, opencode")) {
		t.Fatal(p)
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	out := ale48Setup(t, fx, true, 0, "claude-code", "opencode", "codeaf")
	for _, r := range out.Results {
		if r.Application == "codeaf" {
			continue
		}
		if r.State != "installed" {
			t.Fatal(r)
		}
		ale48AssertBundle(t, r.Destination)
	}
}
func TestALE48SetupUnsupportedFailureAndExecutableDiscovery(t *testing.T) {
	fx := ale48Fixture(t)
	p := ale48Setup(t, fx, true, 1, "codex", "hermes", "codeaf")
	if p.State != "partial" || p.Results[0].State != "installed" || p.Results[1].State != "unsupported" {
		t.Fatal(p)
	}
	// Product-level filesystem denial, with a positive controller write.
	fx = ale48Fixture(t)
	target := filepath.Join(fx.home, ".codex/skills")
	writeFile(t, filepath.Join(target, "positive"), []byte("allowed"), 0600)
	policy := "(deny file-write* (subpath " + sandboxString(target) + "))\n"
	r := ale48Run(t, fx, policy, filepath.Join(fx.home, "bin"), "setup", "--app", "codex", "--allow-shared-app", "codeaf", "--install", "--json")
	assertExit(t, r, 1)
	var report setup.SetupReport
	decodeJSON(t, r.stdout, &report)
	if report.Results[0].State != "failed" {
		t.Fatal(report)
	}
	for _, app := range []string{"codex", "opencode"} {
		r = ale48Run(t, fx, "", "/usr/bin:/bin", "setup", "--app", app, "--install", "--json")
		assertExit(t, r, 1)
		decodeJSON(t, r.stdout, &report)
		if report.Executable.State != "missing" {
			t.Fatal(report)
		}
	}
	// Present but incompatible fixture executable: exact staged isolation probe
	// is an allowed child but cannot answer the companion contract.
	link := filepath.Join(fx.home, "bin/skillglass")
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(isolationProbeBinary, link); err != nil {
		t.Fatal(err)
	}
	r = ale48Run(t, fx, "", filepath.Join(fx.home, "bin"), "setup", "--check", "--json")
	assertExit(t, r, 1)
	if !bytes.Contains(r.stdout, []byte("failed")) {
		t.Fatalf("bad executable not diagnosed %s", r.stdout)
	}
	// External and symlinked user destinations are explicit unsupported isolation.
	fx = ale48Fixture(t)
	root := filepath.Join(fx.home, ".codex/skills")
	if err := os.RemoveAll(root); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(fx.home, ".claude/skills"), root); err != nil {
		t.Fatal(err)
	}
	before := treeManifest(t, fx.base)
	report = ale48Setup(t, fx, true, 1, "codex", "codeaf")
	if report.Results[0].State != "refused" {
		t.Fatal(report)
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
}

func TestALE48ProspectiveAliasAndCompatibilityRefusals(t *testing.T) {
	fx := ale48Fixture(t)
	root := filepath.Join(fx.home, ".agents/skills")
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	// This alias contains no companion yet. Installing would expose it through
	// Gemini even though no existing companion source/reference can be found.
	if err := os.Symlink(filepath.Join(fx.home, ".codex/skills"), filepath.Join(root, "future-companion")); err != nil {
		t.Fatal(err)
	}
	before := treeManifest(t, fx.base)
	report := ale48Setup(t, fx, true, 1, "codex", "codeaf")
	if !strings.Contains(report.Results[0].Reason, "prospective alias") {
		t.Fatal(report)
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	fx = ale48Fixture(t)
	// A deliberately executable protocol fixture is separate from inspected skill
	// contents; its interpreter inherits the same bounded filesystem/network policy.
	fake := filepath.Join(fx.home, "fake-bin/skillglass")
	writeFile(t, fake, []byte("#!/bin/bash\nprintf '%s\\n' '{\"name\":\"skillglass-review\",\"version\":\"0.0.0\",\"review_contract_version\":0,\"files\":[]}'\n"), 0700)
	policy := "(allow file-read* (literal \"/bin/bash\"))\n(allow process-exec (literal " + sandboxString(fake) + ") (literal \"/bin/bash\"))\n"
	result := ale48Run(t, fx, policy, filepath.Dir(fake), "setup", "--check", "--json")
	assertExit(t, result, 1)
	if !bytes.Contains(result.stdout, []byte("incompatible")) {
		t.Fatalf("contract mismatch not classified: %s", result.stdout)
	}
	raw, err := json.Marshal(companion.Bundled())
	if err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/bash\nif [ \"$1\" = companion ]; then printf '%s\\n' " + shellQuote(string(raw)) + "; else printf '%s\\n' '{\"schema_version\":1,\"help\":\"read-resource review prepare review select\"}'; fi\n"
	writeFile(t, fake, []byte(script), 0700)
	result = ale48Run(t, fx, policy, filepath.Dir(fake), "setup", "--check", "--json")
	assertExit(t, result, 1)
	if !bytes.Contains(result.stdout, []byte("documented setup")) {
		t.Fatalf("matching version/bundle bypassed missing command capability: %s", result.stdout)
	}
	if err := os.Rename(fake, fake+"-moved"); err != nil {
		t.Fatal(err)
	}
	result = ale48Run(t, fx, "", filepath.Dir(fake), "setup", "--check", "--json")
	assertExit(t, result, 1)
	if !bytes.Contains(result.stdout, []byte("missing")) {
		t.Fatalf("moved executable not classified: %s", result.stdout)
	}
}

func ale48Preserved(t *testing.T, fx fixture, before []byte, results []setup.SetupResult) {
	t.Helper()
	after := treeManifest(t, fx.base)
	var old, new []treeEntry
	decodeJSON(t, before, &old)
	decodeJSON(t, after, &new)
	prior := map[string]treeEntry{}
	current := map[string]treeEntry{}
	for _, entry := range old {
		if strings.HasPrefix(entry.Mode, "d") {
			entry.Size = 0
		}
		prior[entry.Path] = entry
	}
	for _, entry := range new {
		if strings.HasPrefix(entry.Mode, "d") {
			entry.Size = 0
		}
		current[entry.Path] = entry
	}
	for path, entry := range prior {
		if current[path] != entry {
			t.Fatalf("unrelated entry changed %s: %+v -> %+v", path, entry, current[path])
		}
	}
	for path := range current {
		if _, ok := prior[path]; ok {
			continue
		}
		allowed := false
		absolute := filepath.Join(fx.base, path)
		for _, r := range results {
			if r.Destination != "" && (absolute == r.Destination || strings.HasPrefix(absolute, r.Destination+"/") || strings.HasPrefix(r.Destination, absolute+"/")) {
				allowed = true
			}
		}
		if !allowed {
			t.Fatalf("unexpected setup write %s", path)
		}
	}
	writeEvidence(t, "setup/"+t.Name()+"/before.json", before)
	writeEvidence(t, "setup/"+t.Name()+"/after.json", after)
}

func TestALE48ConsentUsageAndPartialFilesystemFailure(t *testing.T) {
	fx := ale48Fixture(t)
	for i, args := range [][]string{
		{"--app", "codex", "--app", "codex"}, {"--app", "codex", "--allow-shared-app", "codex"},
		{"--app", "codex", "--allow-shared-app", "codeaf", "--allow-shared-app", "codeaf"},
		{"--app", "codex", "--allow-shared-app", "all"}, {"--allow-shared-app", "codeaf"},
		{"--app", "arbitrary"}, {"--app", "codex", "--project", fx.project}, {"--app", "codex", "--root", fx.cleanRoot},
		{"--check", "--allow-shared-app", "codeaf"},
		{"--app", "codex", "--destination", fx.cleanRoot},
		{"--app", "codex", "--unknown-option", "value"},
	} {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			before := treeManifest(t, fx.base)
			argv := append([]string{"setup"}, args...)
			argv = append(argv, "--json", "--install")
			r := ale48Run(t, fx, "", filepath.Join(fx.home, "bin"), argv...)
			assertExit(t, r, 2)
			var value map[string]any
			decodeJSON(t, r.stdout, &value)
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
	before := treeManifest(t, fx.base)
	denied := ale48Run(t, fx, "", filepath.Join(fx.home, "bin"), "setup", "--app", "codex", "--install", "--json")
	assertExit(t, denied, 1)
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	// Acknowledgement alone never turns the unverified app into a setup target.
	ok := ale48Setup(t, fx, true, 0, "codex", "opencode", "codeaf")
	if len(ok.Results) != 2 || len(ok.AllowedSharedApps) != 1 || ok.AllowedSharedApps[0] != "codeaf" {
		t.Fatal(ok)
	}
	for _, r := range ok.Results {
		for _, e := range r.Audit.ConsumerEvidence {
			if e.Application == "codeaf" && e.State != "known-loader-native-unverified" {
				t.Fatal(e)
			}
		}
	}
	for _, root := range []string{".codeaf/skills", ".claude/skills", ".agents/skills"} {
		if _, err := os.Stat(filepath.Join(fx.home, root, companion.Name)); !os.IsNotExist(err) {
			t.Fatal("consent installed extra target", root, err)
		}
	}
	ale48Preserved(t, fx, before, ok.Results)
	unsupported := ale48Run(t, fx, "", filepath.Join(fx.home, "bin"), "setup", "--app", "codeaf", "--install", "--json")
	assertExit(t, unsupported, 1)
	var report setup.SetupReport
	decodeJSON(t, unsupported.stdout, &report)
	if report.Results[0].State != "unsupported" {
		t.Fatal(report)
	}
	fx = ale48Fixture(t)
	target := filepath.Join(fx.xdgConfig, "opencode/skills")
	if err := os.MkdirAll(target, 0700); err != nil {
		t.Fatal(err)
	}
	before = treeManifest(t, fx.base)
	policy := "(deny file-write* (subpath " + sandboxString(target) + "))\n"
	result := ale48Run(t, fx, policy, filepath.Join(fx.home, "bin"), "setup", "--app", "codex", "--app", "opencode", "--allow-shared-app", "codeaf", "--install", "--json")
	assertExit(t, result, 1)
	decodeJSON(t, result.stdout, &report)
	if report.State != "partial" || report.Results[0].State != "installed" || report.Results[1].State != "failed" {
		t.Fatal(report)
	}
	ale48AssertBundle(t, report.Results[0].Destination)
	ale48Preserved(t, fx, before, report.Results)
}

func TestALE48ExtraContentAndNonregularConflicts(t *testing.T) {
	for _, kind := range []string{"extra-file", "extra-dir", "fifo", "link"} {
		t.Run(kind, func(t *testing.T) {
			fx := ale48Fixture(t)
			out := ale48Setup(t, fx, true, 0, "codex", "codeaf")
			dest := out.Results[0].Destination
			switch kind {
			case "extra-file":
				writeFile(t, filepath.Join(dest, "custom.md"), []byte("PRESERVE_CUSTOM"), 0600)
			case "extra-dir":
				if err := os.Mkdir(filepath.Join(dest, "custom"), 0700); err != nil {
					t.Fatal(err)
				}
			case "fifo", "link":
				manifest := filepath.Join(dest, "SKILL.md")
				if err := os.Remove(manifest); err != nil {
					t.Fatal(err)
				}
				if kind == "fifo" {
					if err := syscall.Mkfifo(manifest, 0600); err != nil {
						t.Fatal(err)
					}
				} else {
					if err := os.Symlink(filepath.Join(fx.home, ".claude/settings.json"), manifest); err != nil {
						t.Fatal(err)
					}
				}
			}
			before := treeManifest(t, fx.base)
			report := ale48Setup(t, fx, true, 1, "codex", "codeaf")
			if report.Results[0].State != "refused" || !strings.Contains(report.Results[0].Reason, "existing companion differs") {
				t.Fatal(report)
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}

func TestALE48UnsupportedDestinationNeverFallsBack(t *testing.T) {
	for _, app := range []string{"codex", "claude-code", "opencode"} {
		t.Run(app, func(t *testing.T) {
			fx := ale48Fixture(t)
			key := map[string]string{"codex": "CODEX_HOME", "claude-code": "CLAUDE_CONFIG_DIR", "opencode": "XDG_CONFIG_HOME"}[app]
			custom := filepath.Join(fx.base, "allowed", "unverified-user-destination")
			if err := os.MkdirAll(custom, 0700); err != nil {
				t.Fatal(err)
			}
			before := treeManifest(t, fx.base)
			result := ale48RunEnv(t, fx, "", filepath.Join(fx.home, "bin"), []string{key + "=" + custom}, "setup", "--app", app, "--allow-shared-app", "codeaf", "--install", "--json")
			assertExit(t, result, 1)
			var report setup.SetupReport
			decodeJSON(t, result.stdout, &report)
			if report.Results[0].State != "unsupported" || !strings.Contains(report.Results[0].Reason, "non-default user destination") {
				t.Fatal(report)
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}

func TestALE48NestedFutureAliasRefusesBeforeExposure(t *testing.T) {
	fx := ale48Fixture(t)
	target := filepath.Join(fx.home, ".codex/skills")
	if err := os.RemoveAll(target); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(target, 0700); err != nil {
		t.Fatal(err)
	}
	external := filepath.Join(fx.home, "empty-shared-container")
	if err := os.MkdirAll(external, 0700); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(fx.home, ".agents/skills")
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, filepath.Join(root, "container")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(external, "future")); err != nil {
		t.Fatal(err)
	}
	before := treeManifest(t, fx.base)
	report := ale48Setup(t, fx, true, 1, "codex", "codeaf")
	if report.Results[0].State != "refused" || !strings.Contains(report.Results[0].Reason, "prospective alias") {
		t.Fatal(report)
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
}
