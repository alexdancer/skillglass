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
)

type changeReceipt struct {
	State, Reason, Confirmation string
	NoWriteNeeded               bool   `json:"no_write_needed"`
	CreateDirectory             string `json:"create_directory"`
	WriteState                  string `json:"write_state"`
	Persistence, Achievement    string
	Effective                   codexAvailability
}

func changeCommand(t *testing.T, fx fixture, extraProfile string, args ...string) commandResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	profile := "(version 1)(allow default)(deny network*)" + extraProfile
	argv := append([]string{"-p", profile, skillglassBinary}, args...)
	cmd := exec.CommandContext(ctx, "/usr/bin/sandbox-exec", argv...)
	cmd.Env = fx.environment()
	cmd.Dir = fx.project
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	started := time.Now()
	err := cmd.Run()
	code := 0
	if err != nil {
		exit, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatal(err)
		}
		code = exit.ExitCode()
	}
	if ctx.Err() != nil {
		t.Fatal("change timed out")
	}
	result := commandResult{code: code, stdout: stdout.Bytes(), stderr: stderr.Bytes(), duration: time.Since(started)}
	recordEvidenceCommand(t, "offline-public-mutation", "/usr/bin/sandbox-exec", argv, fx.project, result)
	return result
}
func changeRun(t *testing.T, fx fixture, choice, id, token string, want int, extra ...string) changeReceipt {
	t.Helper()
	args := []string{choice, "--agent", "codex", "--scope", "global", "--project", fx.project, "--json"}
	if token != "" {
		args = append(args, "--confirm", token)
	}
	args = append(args, extra...)
	args = append(args, id)
	result := changeCommand(t, fx, "", args...)
	assertExit(t, result, want)
	var receipt changeReceipt
	decodeJSON(t, result.stdout, &receipt)
	return receipt
}
func changeSourceID(t *testing.T, fx fixture, path string) string {
	t.Helper()
	for _, source := range codexListResult(t, fx, 0).Sources {
		if source.SourcePath == filepath.Dir(path) {
			return source.ID
		}
	}
	t.Fatal("source path absent", path)
	return ""
}
func readBytes(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
func assertSavedChange(t *testing.T, out changeReceipt) {
	t.Helper()
	if out.State != "applied" || out.Persistence != "verified" || out.Achievement != "achieved" {
		t.Fatalf("saved %+v", out)
	}
}
func TestALE27LosslessRoundTrips(t *testing.T) {
	for _, syntax := range []string{"aot", "inline", "empty-array", "absent-entry", "dotted-quoted", "crlf", "misleading-strings", "trailing-comma-comment"} {
		t.Run(syntax, func(t *testing.T) {
			fx, _ := codexFixture(t)
			target := codexSkill(t, filepath.Join(fx.home, ".agents/skills"), "target", "same-name")
			other := codexSkill(t, filepath.Join(fx.home, ".agents/skills"), "other", "same-name")
			id := changeSourceID(t, fx, target)
			config := filepath.Join(fx.home, ".codex/config.toml")
			head := "# untouched\nmodel = 'fixture-secret-not-rendered'\n"
			path, _ := json.Marshal(target)
			otherPath, _ := json.Marshal(other)
			raw := head + selector(target, false) + "note = 'keep target field'\n" + selector(other, false) + "[unrelated]\ntext = 'keep'\n"
			switch syntax {
			case "inline":
				raw = head + "[skills]\nconfig = [{ path = " + string(path) + ", enabled = false, note = 'keep' }, { path = " + string(otherPath) + ", enabled = false }] # trailing\n"
			case "empty-array":
				raw = head + "[skills]\nconfig = [] # untouched suffix\n"
			case "absent-entry":
				raw = head + selector(other, false)
			case "dotted-quoted":
				raw = head + "[\"skills\"]\n\"config\" = [{ \"path\" = " + string(path) + ", \"enabled\" = false }]\n"
			case "trailing-comma-comment":
				raw = head + "[skills]\nconfig = [\n{ path = " + string(otherPath) + ", enabled = false, note = '# literal bracket ]' }, # keep trailing comment\n]\n"
			case "crlf":
				raw = strings.ReplaceAll(raw, "\n", "\r\n")
			case "misleading-strings":
				raw = head + "text = '''[[skills.config]] enabled = false [not an array]'''\n" + selector(target, false)
			}
			writeFile(t, config, []byte(raw), 0640)
			before := treeManifest(t, fx.base)
			preview := changeRun(t, fx, "enable", id, "", 0)
			if preview.State != "ready" || !strings.HasPrefix(preview.Confirmation, "c1:") {
				t.Fatalf("preview %+v", preview)
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			again := changeRun(t, fx, "enable", id, "", 0)
			if again.Confirmation != preview.Confirmation {
				t.Fatal("nondeterministic preview")
			}
			out := changeRun(t, fx, "enable", id, preview.Confirmation, 0)
			assertSavedChange(t, out)
			saved := string(readBytes(t, config))
			if !strings.HasPrefix(saved, head) && syntax != "crlf" {
				t.Fatal("unrelated leading bytes lost")
			}
			if strings.Contains(saved, "fixture-secret-not-rendered") == false {
				t.Fatal("unknown setting lost")
			}
			if syntax == "aot" || syntax == "crlf" || syntax == "misleading-strings" || syntax == "dotted-quoted" {
				expected := strings.Replace(raw, "enabled = false", "enabled = true", 1)
				if syntax == "dotted-quoted" {
					expected = strings.Replace(raw, "\"enabled\" = false", "\"enabled\" = true", 1)
				}
				if syntax != "misleading-strings" && saved != expected {
					t.Fatalf("bytes changed beyond selected token\n%s", saved)
				}
			}
			info, _ := os.Stat(config)
			if info.Mode().Perm() != 0640 {
				t.Fatal("mode lost")
			}
			list := codexListResult(t, fx, 0)
			for _, s := range list.Sources {
				if s.SourcePath == filepath.Dir(other) && (syntax == "aot" || syntax == "inline" || syntax == "absent-entry" || syntax == "crlf") && s.Availability.State != "disabled" {
					t.Fatal("independent selector changed")
				}
			}
			replay := changeRun(t, fx, "enable", id, preview.Confirmation, 1)
			if replay.State != "refused" {
				t.Fatal("writing token replay accepted")
			}
			noop := changeRun(t, fx, "enable", id, "", 0)
			if !noop.NoWriteNeeded {
				t.Fatal("missing no-op")
			}
			before = treeManifest(t, fx.base)
			assertSavedChange(t, changeRun(t, fx, "enable", id, noop.Confirmation, 0))
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			disable := changeRun(t, fx, "disable", id, "", 0)
			assertSavedChange(t, changeRun(t, fx, "disable", id, disable.Confirmation, 0))
			if !bytes.Equal(readBytes(t, target), []byte(fmt.Sprintf("---\nname: same-name\ndescription: Fixture target.\n---\nInert instructions.\n"))) {
				t.Fatal("content changed")
			}
		})
	}
}
func TestALE27StaleAndUnsafeInputs(t *testing.T) {
	for _, kind := range []string{"config-edit", "equal-config-replacement", "manifest-edit", "equal-manifest-replacement", "rename-source", "policy-edit", "add-related-source", "config-symlink", "config-hardlink", "fifo", "directory", "duplicates-equal", "duplicates-alias", "relative", "name", "malformed"} {
		t.Run(kind, func(t *testing.T) {
			fx, _ := codexFixture(t)
			target := codexSkill(t, filepath.Join(fx.home, ".agents/skills"), "target", "target")
			config := filepath.Join(fx.home, ".codex/config.toml")
			writeFile(t, config, []byte("# original\n"), 0600)
			id := changeSourceID(t, fx, target)
			preview := changeRun(t, fx, "disable", id, "", 0)
			unsafe := false
			switch kind {
			case "config-edit":
				writeFile(t, config, []byte("# external edit\nmodel='external'\n"), 0600)
			case "equal-config-replacement":
				other := config + ".new"
				writeFile(t, other, readBytes(t, config), 0600)
				if err := os.Rename(other, config); err != nil {
					t.Fatal(err)
				}
			case "manifest-edit":
				writeFile(t, target, append(readBytes(t, target), []byte("External text\n")...), 0600)
			case "equal-manifest-replacement":
				other := target + ".new"
				writeFile(t, other, readBytes(t, target), 0600)
				if err := os.Rename(other, target); err != nil {
					t.Fatal(err)
				}
			case "rename-source":
				if err := os.Rename(filepath.Dir(target), filepath.Dir(target)+"-moved"); err != nil {
					t.Fatal(err)
				}
			case "policy-edit":
				writeFile(t, filepath.Join(filepath.Dir(target), "agents/openai.yaml"), []byte("policy:\n  allow_implicit_invocation: false\n"), 0600)
			case "add-related-source":
				codexSkill(t, filepath.Join(fx.home, ".agents/skills"), "other", "target")
			case "config-symlink":
				outside := filepath.Join(fx.base, "outside-config")
				writeFile(t, outside, readBytes(t, config), 0600)
				os.Remove(config)
				if err := os.Symlink(outside, config); err != nil {
					t.Fatal(err)
				}
				unsafe = true
			case "config-hardlink":
				if err := os.Link(config, config+".alias"); err != nil {
					t.Fatal(err)
				}
				unsafe = true
			case "fifo":
				os.Remove(config)
				if err := syscall.Mkfifo(config, 0600); err != nil {
					t.Fatal(err)
				}
				unsafe = true
			case "directory":
				os.Remove(config)
				if err := os.Mkdir(config, 0700); err != nil {
					t.Fatal(err)
				}
				unsafe = true
			case "duplicates-equal":
				writeFile(t, config, []byte(selector(target, false)+selector(target, false)), 0600)
				unsafe = true
			case "duplicates-alias":
				alias := filepath.Join(fx.home, ".agents/skills/alias")
				if err := os.Symlink(filepath.Dir(target), alias); err != nil {
					t.Fatal(err)
				}
				writeFile(t, config, []byte(selector(target, false)+selector(filepath.Join(alias, "SKILL.md"), true)), 0600)
				unsafe = true
			case "relative":
				writeFile(t, config, []byte(selector("relative/SKILL.md", false)), 0600)
				unsafe = true
			case "name":
				writeFile(t, config, []byte("[[skills.config]]\nname='target'\nenabled=false\n"), 0600)
				unsafe = true
			case "malformed":
				writeFile(t, config, []byte("[bad"), 0600)
				unsafe = true
			}
			before := treeManifest(t, fx.base)
			out := changeRun(t, fx, "disable", id, preview.Confirmation, 1)
			if out.State != "refused" && out.State != "unsupported" {
				t.Fatalf("stale %+v", out)
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			if unsafe {
				out = changeRun(t, fx, "disable", id, "", 1)
				if out.State != "refused" {
					t.Fatalf("unsafe %+v", out)
				}
				assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			}
		})
	}
}
func TestALE27SourceScopeAndEffectiveBlockers(t *testing.T) {
	fx, root := codexFixture(t)
	target := codexSkill(t, filepath.Join(root, ".agents/skills"), "project-source", "project-source")
	id := changeSourceID(t, fx, target)
	config := filepath.Join(fx.home, ".codex/config.toml")
	projectConfig := filepath.Join(root, ".codex/config.toml")
	writeFile(t, config, []byte(selector(target, false)+trustProject(root)), 0600)
	writeFile(t, projectConfig, []byte(selector(target, false)), 0600)
	preview := changeRun(t, fx, "enable", id, "", 0)
	beforeProject := readBytes(t, projectConfig)
	out := changeRun(t, fx, "enable", id, preview.Confirmation, 1)
	if out.State != "applied" || out.Persistence != "verified" || out.Achievement != "unmet" || out.Effective.State != "disabled" {
		t.Fatalf("blocker %+v", out)
	}
	if !bytes.Equal(beforeProject, readBytes(t, projectConfig)) {
		t.Fatal("project config changed")
	}
	writeFile(t, projectConfig, []byte("[skills]\nconfig=[]\n"), 0600)
	preview = changeRun(t, fx, "disable", id, "", 0)
	out = changeRun(t, fx, "disable", id, preview.Confirmation, 1)
	if out.State != "applied" || out.Persistence != "verified" || out.Effective.State != "available" {
		t.Fatalf("empty array %+v", out)
	}
	before := treeManifest(t, fx.base)
	project := fx.run(t, "disable", "--agent", "codex", "--scope", "project", "--project", fx.project, "--json", id)
	assertExit(t, project, 1)
	var receipt changeReceipt
	decodeJSON(t, project.stdout, &receipt)
	if receipt.State != "unsupported" {
		t.Fatal("project fallback")
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
}
func TestALE27MissingConfigAndSharedAlias(t *testing.T) {
	fx, _ := codexFixture(t)
	target := codexSkill(t, filepath.Join(fx.home, ".agents/skills"), "target", "target")
	alias := filepath.Join(fx.home, ".agents/skills/alias")
	if err := os.Symlink(filepath.Dir(target), alias); err != nil {
		t.Fatal(err)
	}
	// Remove only this fixture's ordinary compatibility skill/root to test precise root creation.
	if err := os.RemoveAll(filepath.Join(fx.home, ".codex")); err != nil {
		t.Fatal(err)
	}
	id := changeSourceID(t, fx, target)
	before := treeManifest(t, fx.base)
	preview := changeRun(t, fx, "disable", id, "", 0)
	if preview.CreateDirectory == "" {
		t.Fatal("directory creation omitted")
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	assertSavedChange(t, changeRun(t, fx, "disable", id, preview.Confirmation, 0))
	dir, _ := os.Stat(filepath.Join(fx.home, ".codex"))
	config, _ := os.Stat(filepath.Join(fx.home, ".codex/config.toml"))
	if dir.Mode().Perm() != 0700 || config.Mode().Perm() != 0600 {
		t.Fatal("new permissions unsafe")
	}
}
func TestALE27WrongConfirmationConcurrencyAndWriteDenial(t *testing.T) {
	fx, _ := codexFixture(t)
	target := codexSkill(t, filepath.Join(fx.home, ".agents/skills"), "target", "target")
	config := filepath.Join(fx.home, ".codex/config.toml")
	writeFile(t, config, nil, 0600)
	id := changeSourceID(t, fx, target)
	preview := changeRun(t, fx, "disable", id, "", 0)
	before := treeManifest(t, fx.base)
	for _, choice := range []string{"enable", "disable"} {
		out := changeRun(t, fx, choice, id, "c1:forged", 1)
		if out.State != "refused" {
			t.Fatal("forged preview accepted")
		}
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	args := []string{"disable", "--agent", "codex", "--scope", "global", "--project", fx.project, "--json", "--confirm", preview.Confirmation, id}
	denied := changeCommand(t, fx, "(deny file-write*)", args...)
	assertExit(t, denied, 1)
	var out changeReceipt
	decodeJSON(t, denied.stdout, &out)
	if out.State != "failed" || out.WriteState != "none" {
		t.Fatalf("denial %+v", out)
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	results := make(chan commandResult, 2)
	for i := 0; i < 2; i++ {
		go func() {
			cmd := exec.Command(skillglassBinary, args...)
			cmd.Env = fx.environment()
			cmd.Dir = fx.project
			raw, err := cmd.Output()
			code := 0
			if err != nil {
				code = err.(*exec.ExitError).ExitCode()
			}
			results <- commandResult{code: code, stdout: raw}
		}()
	}
	applied, refused := 0, 0
	for i := 0; i < 2; i++ {
		r := <-results
		decodeJSON(t, r.stdout, &out)
		if r.code == 0 && out.State == "applied" {
			applied++
		} else if r.code == 1 && out.State == "refused" {
			refused++
		} else {
			t.Fatalf("race %+v %s", r, out.Reason)
		}
	}
	if applied != 1 || refused != 1 {
		t.Fatalf("writers applied=%d refused=%d", applied, refused)
	}
}

func TestALE27DirectoryAliasesMetadataAndExplicitOnly(t *testing.T) {
	fx, _ := codexFixture(t)
	target := codexSkill(t, filepath.Join(fx.home, ".agents/skills"), "target", "target")
	id := changeSourceID(t, fx, target)
	config := filepath.Join(fx.home, ".codex/config.toml")
	writeFile(t, config, []byte("# preserved\n"), 0600)
	xattr := exec.Command("/usr/bin/xattr", "-w", "org.skillglass.fixture", "preserve-exactly", config)
	xattr.Env = fx.environment()
	if raw, err := xattr.CombinedOutput(); err != nil {
		t.Fatalf("fixture metadata: %s %v", raw, err)
	}
	root := filepath.Dir(config)
	canonical := filepath.Join(fx.home, "codex-storage")
	if err := os.Rename(root, canonical); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(canonical, root); err != nil {
		t.Fatal(err)
	}
	preview := changeRun(t, fx, "disable", id, "", 0)
	assertSavedChange(t, changeRun(t, fx, "disable", id, preview.Confirmation, 0))
	value := exec.Command("/usr/bin/xattr", "-p", "org.skillglass.fixture", config)
	value.Env = fx.environment()
	raw, err := value.Output()
	if err != nil || strings.TrimSpace(string(raw)) != "preserve-exactly" {
		t.Fatal("metadata stripped", err, string(raw))
	}
	writeFile(t, filepath.Join(filepath.Dir(target), "agents/openai.yaml"), []byte("policy:\n  allow_implicit_invocation: false\n"), 0600)
	preview = changeRun(t, fx, "enable", id, "", 0)
	out := changeRun(t, fx, "enable", id, preview.Confirmation, 0)
	if out.Effective.State != "explicit-only" {
		t.Fatal("reduced invocation lost")
	}
	preview = changeRun(t, fx, "disable", id, "", 0)
	elsewhere := filepath.Join(fx.home, "new-storage")
	if err := os.Mkdir(elsewhere, 0700); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(elsewhere, "config.toml"), readBytes(t, config), 0600)
	os.Remove(root)
	if err := os.Symlink(elsewhere, root); err != nil {
		t.Fatal(err)
	}
	before := treeManifest(t, fx.base)
	if changeRun(t, fx, "disable", id, preview.Confirmation, 1).State != "refused" {
		t.Fatal("retargeted config root accepted")
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	empty := changeCommand(t, fx, "", "disable", "--agent", "codex", "--scope", "global", "--project", fx.project, "--json", "--confirm", "", id)
	assertExit(t, empty, 1)
}
func TestALE27ACLAndManifestHardlinksRefuse(t *testing.T) {
	fx, _ := codexFixture(t)
	target := codexSkill(t, filepath.Join(fx.home, ".agents/skills"), "target", "target")
	id := changeSourceID(t, fx, target)
	config := filepath.Join(fx.home, ".codex/config.toml")
	writeFile(t, config, []byte("# preserved\n"), 0600)
	chmod := exec.Command("/bin/chmod", "+a", "everyone deny write", config)
	chmod.Env = fx.environment()
	if raw, err := chmod.CombinedOutput(); err != nil {
		t.Fatalf("fixture ACL: %s %v", raw, err)
	}
	before := treeManifest(t, fx.base)
	out := changeRun(t, fx, "disable", id, "", 1)
	if !strings.Contains(out.Reason, "ACL") {
		t.Fatal("ACL refusal unclear", out)
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	removeACL := exec.Command("/bin/chmod", "-N", config)
	removeACL.Env = fx.environment()
	if err := removeACL.Run(); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(target, filepath.Join(fx.base, "shared-manifest")); err != nil {
		t.Fatal(err)
	}
	before = treeManifest(t, fx.base)
	out = changeRun(t, fx, "disable", id, "", 1)
	if !strings.Contains(out.Reason, "hardlinks") {
		t.Fatal("manifest matching topology guessed", out)
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
}

func TestALE27HumanRefusalEscapesUntrustedSelection(t *testing.T) {
	fx, _ := codexFixture(t)
	before := treeManifest(t, fx.base)
	out := fx.run(t, "disable", "--agent", "codex", "--scope", "global", "\x1b[31mforged\nsource")
	assertExit(t, out, 1)
	if bytes.Contains(out.stdout, []byte{0x1b}) {
		t.Fatal("terminal control sequence escaped the renderer")
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
}
