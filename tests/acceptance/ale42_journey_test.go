package acceptance_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

type ale42Collection struct {
	fixture              fixture
	sharedManifest       string
	independentManifests [2]string
	managedManifest      string
	projectWinner        string
	shadowedManifest     string
	externalResource     string
	executionMarker      string
	canaries             []string
}

func newALE42Collection(t *testing.T) ale42Collection {
	t.Helper()
	fx, _, target := ale40Fixture(t, false)
	long := filepath.Join(fx.base, "allowed", strings.Repeat("long-delivered-source-", 7), "ale40-target")
	if err := os.MkdirAll(filepath.Dir(long), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Dir(target), long); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(long, filepath.Dir(target)); err != nil {
		t.Fatal(err)
	}
	fx.project = filepath.Join(fx.project, strings.Repeat("long-selected-project-", 5))
	if err := os.MkdirAll(fx.project, 0700); err != nil {
		t.Fatal(err)
	}
	c := ale42Collection{fixture: fx, sharedManifest: filepath.Join(long, "SKILL.md"), executionMarker: filepath.Join(fx.base, "allowed", "executed-marker"), externalResource: filepath.Join(fx.base, "allowed", "external-resource.txt")}
	for i, folder := range []string{"journey-copy-a", "journey-copy-b"} {
		c.independentManifests[i] = codexSkill(t, filepath.Join(fx.home, ".agents/skills"), folder, "journey-copy")
		writeFile(t, c.independentManifests[i], []byte("---\nname: journey-copy\ndescription: independent comparison sources\n---\nComplete offline instructions.\n"), 0600)
		writeFile(t, filepath.Join(filepath.Dir(c.independentManifests[i]), "guide.txt"), []byte("same resource\n"), 0600)
	}
	writeFile(t, filepath.Join(long, "run.sh"), []byte("#!/bin/sh\ntouch "+shellQuote(c.executionMarker)+"\n"), 0700)
	writeFile(t, c.externalResource, []byte("EXTERNAL_RESOURCE_SURVIVES\n"), 0600)
	if err := os.Symlink(c.externalResource, filepath.Join(filepath.Dir(c.independentManifests[1]), "outside-link")); err != nil {
		t.Fatal(err)
	}
	c.managedManifest, _ = codeafPluginFixture(t, fx)
	registry := filepath.Join(fx.home, ".claude/plugins/installed_plugins.json")
	var registration map[string]any
	if err := json.Unmarshal(readBytes(t, registry), &registration); err != nil {
		t.Fatal(err)
	}
	registration["version"] = 2
	raw, err := json.Marshal(registration)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, registry, raw, 0600)

	writeFile(t, filepath.Join(fx.home, ".claude/settings.json"), []byte(`{"keep":true,"enabledPlugins":{"example@local":true}}`), 0600)
	c.projectWinner = codexSkill(t, filepath.Join(fx.project, ".codeaf/skills"), "project-priority", "priority-case")
	c.shadowedManifest = codexSkill(t, filepath.Join(fx.home, ".codeaf/skills"), "global-priority", "priority-case")
	for _, relative := range []string{".claude/projects/activity.jsonl", ".codex/sessions/activity.jsonl", ".hermes/logs/activity.log", ".pi/agent/sessions/activity.jsonl", ".gemini/tmp/activity.jsonl", ".codeaf/logs/activity.log", ".codex/auth.json", ".claude/.credentials.json"} {
		path := filepath.Join(fx.home, relative)
		writeFile(t, path, []byte("ALE42_PRIVATE_LOG_OR_CREDENTIAL_CANARY\n"), 0600)
		c.canaries = append(c.canaries, path)
	}
	c.canaries = append(c.canaries, filepath.Join(fx.xdgState, "opencode/activity.log"))
	writeFile(t, c.canaries[len(c.canaries)-1], []byte("ALE42_PRIVATE_LOG_OR_CREDENTIAL_CANARY\n"), 0600)
	// Base-policy positive reads and stricter-policy negative reads use the same probe,
	// environment, and owned canary. Product launches never receive the probe mode.
	allowed := filepath.Join(fx.base, "allowed", "canary-positive")
	writeFile(t, allowed, []byte("ALLOWED_READ_SENTINEL\n"), 0600)
	for i, path := range c.canaries {
		if !bytes.Contains(readBytes(t, path), []byte("ALE42_PRIVATE")) {
			t.Fatal("missing owned positive control")
		}
		probe := func(f fixture, denied bool) {
			command := f.containedCommand(t, context.Background(), isolationProbeBinary, allowed, path, filepath.Join(f.temporary, "forbidden-write"), "127.0.0.1:9")
			result, err := executeFinalizedCommand(t, command)
			var out struct {
				Passed bool
				Checks map[string]bool
			}
			decodeJSON(t, result.stdout, &out)
			if out.Checks["allowed_read"] != true || out.Checks["excluded_read_denied"] != denied || out.Passed != denied {
				t.Fatalf("canary control: %s %v", result.stdout, err)
			}
			writeEvidence(t, fmt.Sprintf("ale42/%s/canary-%02d-denied-%t.json", t.Name(), i, denied), result.stdout)
		}
		probe(fx, false)
		strict := fx
		strict.deniedReads = []string{path}
		strict.seal(t)
		probe(strict, true)
		fx.seal(t)
	}
	fx.deniedReads = append([]string(nil), c.canaries...)
	fx.seal(t)
	c.fixture = fx
	return c
}

func ale42SourceAt(t *testing.T, listed codexList, manifest string) codexSource {
	t.Helper()
	path := filepath.Dir(canonicalPath(t, manifest))
	for _, s := range listed.Sources {
		if s.SourcePath == path {
			return s
		}
	}
	t.Fatalf("canonical source absent: %s", path)
	return codexSource{}
}

func TestALE42DeliveredCLIJourney(t *testing.T) {
	c := newALE42Collection(t)
	fx := c.fixture
	before := treeManifest(t, fx.base)
	var sharedID string
	for _, app := range []string{"codex", "claude-code", "hermes", "pi", "opencode", "gemini", "codeaf"} {
		result := fx.run(t, "list", "--agent", app, "--project", fx.project, "--json")
		assertExit(t, result, 0)
		var list codexList
		decodeJSON(t, result.stdout, &list)
		source := ale42SourceAt(t, list, c.sharedManifest)
		if list.Scope.Agent.Application != app || list.Scope.Agent.ValidatedVersion == "" || source.Availability.Session.State != "unknown" {
			t.Fatalf("missing scoped support evidence: %+v", list.Scope)
		}
		if sharedID == "" {
			sharedID = source.ID
		}
		if source.ID != sharedID {
			t.Fatal("shared references were split")
		}
		writeEvidence(t, "ale42/cli/01-inventory-"+app+".json", result.stdout)
		for _, operation := range []string{"read", "explain"} {
			result = fx.run(t, operation, "--agent", app, "--project", fx.project, "--json", source.ID)
			assertExit(t, result, 0)
			if operation == "read" && (!bytes.Contains(result.stdout, []byte("Inert instructions.")) || !bytes.Contains(result.stdout, []byte("run.sh"))) {
				t.Fatal("full instructions or resource inventory missing", string(result.stdout))
			}
			if bytes.Contains(result.stdout, []byte("ALE42_PRIVATE_LOG_OR_CREDENTIAL_CANARY")) {
				t.Fatal("private canary ingested")
			}
			writeEvidence(t, "ale42/cli/02-"+operation+"-"+app+".json", result.stdout)
		}
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	native := codexListResult(t, fx, 0)
	a, b := ale42SourceAt(t, native, c.independentManifests[0]), ale42SourceAt(t, native, c.independentManifests[1])
	if a.ID == b.ID || a.ID == sharedID {
		t.Fatal("independent sources merged")
	}
	// Remove the external link briefly to prove complete identical comparison, then
	// restore it and change a resource before a fresh, visibly incomplete comparison.
	link := filepath.Join(filepath.Dir(c.independentManifests[1]), "outside-link")
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	before = treeManifest(t, fx.base)
	result := fx.run(t, "compare", "--agent", "codex", "--project", fx.project, "--json", a.ID, b.ID)
	assertExit(t, result, 0)
	equal := compareReport(t, result)
	if equal.Report.Outcome != "identical" || equal.Report.Relation != "independent-copies" {
		t.Fatal(string(result.stdout))
	}
	writeEvidence(t, "ale42/cli/03-identical.json", result.stdout)
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	writeFile(t, filepath.Join(filepath.Dir(c.independentManifests[1]), "guide.txt"), []byte("changed supporting resource\n"), 0600)
	if err := os.Symlink(c.externalResource, link); err != nil {
		t.Fatal(err)
	}
	before = treeManifest(t, fx.base)
	result = fx.run(t, "compare", "--agent", "codex", "--project", fx.project, "--json", a.ID, b.ID)
	assertExit(t, result, 1)
	different := compareReport(t, result)
	if different.Report.Outcome != "indeterminate" || different.Report.Coverage != "incomplete" || len(different.Report.Changes) == 0 {
		t.Fatal(string(result.stdout))
	}
	writeEvidence(t, "ale42/cli/04-resource-difference.json", result.stdout)
	codeaf := codeafListResult(t, fx, 0)
	winner := ale42SourceAt(t, codeaf, c.projectWinner)
	shadow := ale42SourceAt(t, codeaf, c.shadowedManifest)
	if winner.Availability.Precedence.State != "winning" || shadow.Availability.State != "shadowed" || shadow.Availability.Precedence.Winner != winner.ID {
		t.Fatal(winner, shadow)
	}
	result = fx.run(t, "explain", "--agent", "codeaf", "--project", fx.project, "--json", shadow.ID)
	assertExit(t, result, 0)
	writeEvidence(t, "ale42/cli/05-precedence.json", result.stdout)
	managed := ale42SourceAt(t, codeaf, c.managedManifest)
	result = changeCommand(t, fx, "", "disable", "--agent", "claude-code", "--scope", "global", "--project", fx.project, "--json", managed.ID)
	assertExit(t, result, 1)
	writeEvidence(t, "ale42/cli/06-managed-refusal.json", result.stdout)
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	global := readBytes(t, filepath.Join(fx.home, ".claude/settings.json"))
	for _, choice := range []string{"disable", "inherit"} {
		preview := claudeChange(t, fx, choice, "project", sharedID, "", 0)
		out := claudeChange(t, fx, choice, "project", sharedID, preview.Confirmation, 0)
		assertSavedChange(t, out)
		inspected := claudeAt(t, claudeListResult(t, fx, 0), filepath.Dir(c.sharedManifest))
		if choice == "disable" && inspected.Availability.State != "disabled" {
			t.Fatal(inspected)
		}
		if choice == "inherit" && inspected.Availability.State != "available" {
			t.Fatal(inspected)
		}
		result = fx.run(t, "explain", "--agent", "claude-code", "--project", fx.project, "--json", sharedID)
		assertExit(t, result, 0)
		writeEvidence(t, "ale42/cli/07-project-"+choice+".json", result.stdout)
	}
	if !bytes.Equal(global, readBytes(t, filepath.Join(fx.home, ".claude/settings.json"))) {
		t.Fatal("project override rewrote global settings")
	}
	before = treeManifest(t, fx.base)
	result = changeCommand(t, fx, "", "disable", "--agent", "codex", "--scope", "project", "--project", fx.project, "--json", sharedID)
	assertExit(t, result, 1)
	writeEvidence(t, "ale42/cli/08-unsupported-project.json", result.stdout)
	apps := []string{"codex", "claude-code", "hermes", "pi", "opencode", "gemini", "codeaf"}
	refused := ale40Preview(t, fx, apps, "global", sharedID, 1)
	if refused.Confirmation != "" || len(refused.Consumers) != 7 {
		t.Fatal("mixed aggregate authorized")
	}
	ale40Apply(t, fx, apps, sharedID, "g1:forged", "", 1)
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	apps = apps[:6]
	for _, choice := range []string{"disable", "enable"} {
		result = ale40Command(t, fx, apps, choice, "global", sharedID, "", "", 0)
		var preview struct{ State, Confirmation string }
		decodeJSON(t, result.stdout, &preview)
		if preview.State != "ready" || preview.Confirmation == "" {
			t.Fatal(string(result.stdout))
		}
		writeEvidence(t, "ale42/cli/09-"+choice+"-preview.json", result.stdout)
		result = ale40Command(t, fx, apps, choice, "global", sharedID, preview.Confirmation, "", 0)
		writeEvidence(t, "ale42/cli/10-"+choice+"-applied.json", result.stdout)
		for _, app := range append(append([]string{}, apps...), "codeaf") {
			result = fx.run(t, "list", "--agent", app, "--project", fx.project, "--json")
			assertExit(t, result, 0)
			var list codexList
			decodeJSON(t, result.stdout, &list)
			source := ale42SourceAt(t, list, c.sharedManifest)
			expected := "available"
			if app == "gemini" {
				expected = "ask"
			}
			if choice == "disable" && app != "codeaf" {
				expected = "disabled"
			}
			if source.Availability.State != expected {
				t.Fatalf("%s %s: %+v", choice, app, source)
			}
			writeEvidence(t, "ale42/cli/11-"+choice+"-reinspect-"+app+".json", result.stdout)
		}
	}
	config := filepath.Join(fx.home, ".hermes/config.yaml")
	healthy := readBytes(t, config)
	writeFile(t, config, []byte("[invalid"), 0600)
	before = treeManifest(t, fx.base)
	incomplete := ale40Preview(t, fx, []string{"codex", "hermes"}, "global", sharedID, 1)
	if incomplete.Confirmation != "" {
		t.Fatal("incomplete member authorized")
	}
	subset := ale40Preview(t, fx, []string{"codex"}, "global", sharedID, 0)
	visible := false
	for _, consumer := range subset.Consumers {
		if consumer.Application == "hermes" && consumer.Scope.Completeness == "incomplete" {
			visible = true
		}
	}
	if !visible {
		t.Fatal("unrequested incomplete coverage hidden")
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	writeFile(t, config, healthy, 0600)
	before = treeManifest(t, fx.base)
	deletion := ale41Preview(t, fx, b.ID)
	if deletion.State != "ready" {
		t.Fatal(deletion.Reason)
	}
	ale41Delete(t, fx, b.ID, "cancelled", "", 1)
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	result = ale41Delete(t, fx, b.ID, deletion.Confirmation, "", 0)
	writeEvidence(t, "ale42/cli/12-delete.json", result.stdout)
	if _, err := os.Lstat(filepath.Dir(c.independentManifests[1])); !os.IsNotExist(err) {
		t.Fatal("selected copy survives", err)
	}
	if !bytes.Contains(readBytes(t, c.independentManifests[0]), []byte("Complete offline")) || string(readBytes(t, c.externalResource)) != "EXTERNAL_RESOURCE_SURVIVES\n" {
		t.Fatal("independent or external content changed")
	}
	if _, err := os.Stat(c.executionMarker); !os.IsNotExist(err) {
		t.Fatal("bundled script executed", err)
	}
}

func paneContainsUnwrappedPath(frame, path string) bool {
	var text strings.Builder
	for _, line := range strings.Split(frame, "\n") {
		text.WriteString(strings.TrimRight(line, " "))
	}
	return strings.Contains(text.String(), path)
}

func assertALE42DeleteWarning(t *testing.T, frame string) {
	t.Helper()
	const warning = "Other projects and consumers outside this checked scope are unverified."
	if !paneContains(frame, warning) {
		t.Fatalf("delete warning is missing or unreadable:\n%s", frame)
	}
	for _, row := range strings.Split(frame, "\n") {
		if strings.TrimSpace(row) == "-" {
			t.Fatalf("delete warning leaves a standalone hyphen on the grid:\n%s", frame)
		}
	}
}

func TestALE42DeliveredBrowserJourney(t *testing.T) {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatal(err)
	}
	for _, width := range []int{120, 48} {
		t.Run(strconv.Itoa(width), func(t *testing.T) {
			c := newALE42Collection(t)
			for i, manifest := range c.independentManifests {
				for n := 0; n < 12; n++ {
					writeFile(t, filepath.Join(filepath.Dir(manifest), fmt.Sprintf("z-%02d.txt", n)), []byte(fmt.Sprintf("copy %d supporting resource %02d\n", i, n)), 0600)
				}
			}
			fx := c.fixture
			height := 30
			if width == 48 {
				height = 28
			}
			socket := fmt.Sprintf("ale42-%d-%d", os.Getpid(), width)
			defer runTmuxIgnoringError(tmux, fx, socket, "kill-server")
			argv := fx.launchArgs(t, skillglassBinary, true, "", "browse", "--agent", "claude-code", "--project", fx.project, "--root", filepath.Join(fx.home, ".agents/skills"), "--theme", "none")
			command := exec.Command(argv[0], argv[1:]...)
			command.Dir, command.Env = fx.project, fx.environment()
			runTmuxProduct(t, tmux, fx, socket, command, "new-session", "-d", "-x", strconv.Itoa(width), "-y", strconv.Itoa(height), "-s", "browser")
			runTmux(t, tmux, fx, socket, "set-option", "-g", "status", "off")
			runTmux(t, tmux, fx, socket, "set-option", "-g", "remain-on-exit", "on")
			runTmux(t, tmux, fx, socket, "resize-window", "-t", "browser:0", "-x", strconv.Itoa(width), "-y", strconv.Itoa(height))
			send := func(key string, needles ...string) string {
				return actForPaneAt(t, tmux, fx, socket, "browser:0.0", false, func() { runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", key) }, needles...)
			}
			capture := func(name, text string) {
				assertPaneCells(t, text, width, height)
				if strings.Contains(text, "ALE42_PRIVATE_LOG_OR_CREDENTIAL_CANARY") {
					t.Fatal("private content ingested")
				}
				writeEvidence(t, fmt.Sprintf("tmux/ale42/%d/%s.txt", width, name), []byte(text))
			}
			capture("01-inventory", waitForPane(t, tmux, fx, socket, false, "Skillglass", "ale40-target"))
			selectName := func(name, count string) {
				send("/", "Enter keep")
				runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "C-u")
				runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "-l", name)
				waitForPane(t, tmux, fx, socket, false, "Focus: 1/"+count)
				actOnPane(t, tmux, fx, socket, "browser:0.0", func() { runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "Enter") }, func(frame paneSnapshot) bool {
					return !paneContains(frame.cells, "Enter keep") && paneHas(frame, false, "Focus: 1/"+count, "Search /")
				})
			}
			before := treeManifest(t, fx.base)
			selectName("journey-copy", "2")
			capture("02-mark-independent", send("m", "Compare left:", "Left path:"))
			send("j", "Focus: 2/2")
			capture("03-compare", send("v", "SKILLGLASS | Comparison", "independent-copies"))
			capture("04-comparison-coverage", send("G", "z-11.txt", "copy 1 supporting resource 11"))
			send("Escape", "Search /")
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			selectName("ale40-target", "1")
			capture("05-read", send("Enter", "Read lines", "Selected:"))
			capture("06-details", send("d", "Availability:", "Session: unknown"))
			capture("07-detail-bottom", send("G", "Resources", "run.sh"))
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			send("D", "Select the setting scope explicitly")
			capture("08-project-preview", send("p", "ready", "y Apply project disable"))
			capture("09-project-applied", send("y", "Persistence: verified", "Effective result: achieved"))
			sharedID := changeSourceID(t, fx, c.sharedManifest)
			if claudeAt(t, claudeListResult(t, fx, 0), filepath.Dir(c.sharedManifest)).Availability.State != "disabled" {
				t.Fatal("browser project change not persisted")
			}
			send("Enter", "Read lines")
			send("D", "Select the setting scope explicitly")
			send("i", "ready", "inherit")
			capture("10-inherit", send("y", "Persistence: verified", "Effective result: achieved"))
			if claudeAt(t, claudeListResult(t, fx, 0), filepath.Dir(c.sharedManifest)).Availability.State != "available" {
				t.Fatal("browser inherit not persisted")
			}
			send("Enter", "Read lines")
			before = treeManifest(t, fx.base)
			send("A", "Choose applications explicitly", "[ ] Codex")
			send("Space", "[1] Codex")
			send("Enter", "Select the group setting scope explicitly")
			capture("10b-unsupported-project", send("p", "Whole request unapplied", "unsupported"))
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			send("Escape", "Read lines")
			capture("11-all-seven", send("A", "Choose applications explicitly", "[ ] Codex"))
			for i := 0; i < 7; i++ {
				if i > 0 {
					runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "Down")
				}
				send("Space", fmt.Sprintf("[%d]", i+1))
			}
			send("Enter", "Select the group setting scope explicitly")
			capture("12-mixed-refusal", send("g", "Whole request unapplied", "unsupported"))
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			send("s", "Edit selection", "[7] CodeAF")
			send("Space", "[ ] CodeAF")
			send("Enter", "Select the group setting scope explicitly")
			capture("13-fresh-six", send("g", "ready", "y Apply group"))
			capture("14-six-applied", send("y", "Group result: applied", "1. Codex: succeeded"))
			capture("15-six-results-bottom", send("G", "home/.gemini/skills/ale40-target"))
			for _, app := range []string{"codex", "claude-code", "hermes", "pi", "opencode", "gemini", "codeaf"} {
				result := fx.run(t, "list", "--agent", app, "--project", fx.project, "--json")
				assertExit(t, result, 0)
				var list codexList
				decodeJSON(t, result.stdout, &list)
				state := ale42SourceAt(t, list, c.sharedManifest).Availability.State
				expected := "disabled"
				if app == "codeaf" {
					expected = "available"
				}
				if state != expected {
					t.Fatal(app, state)
				}
				writeEvidence(t, fmt.Sprintf("ale42/browser/%d/reinspect-%s.json", width, app), result.stdout)
			}
			send("Enter", "Read lines")
			// A fresh explicit six-writer enable restores the collection before deletion.
			send("E", "All start unchecked")
			for i := 0; i < 6; i++ {
				if i > 0 {
					runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "Down")
				}
				send("Space", fmt.Sprintf("[%d]", i+1))
			}
			send("Enter", "Select the group setting scope explicitly")
			send("g", "ready")
			send("y", "Group result: applied")
			send("Enter", "Read lines")
			// Managed provenance is inspectable in the same session and refuses deletion.
			send("Escape", "Search /")
			selectName("plugin-skill", "1")
			send("Enter", "Read lines", "Selected: plugin-skill")
			capture("16-managed-details", send("d", "Availability:"))
			capture("17-managed-delete-refusal", send("X", "PERMANENT DELETE", "refused"))
			send("Escape", "Read lines")
			send("Escape", "Search /")
			selectName("journey-copy", "2")
			send("j", "Focus: 2/2")
			send("Enter", "Read lines", "Selected: journey-copy")
			copyDetails := send("d", "Availability:", "Source ID:")
			capture("18-copy-details", copyDetails)
			// Bind the selected browser item to its exact public SourceID and path.
			ids := codexListResult(t, fx, 0)
			first := ale42SourceAt(t, ids, c.independentManifests[0])
			second := ale42SourceAt(t, ids, c.independentManifests[1])
			var selected codexSource
			var other string
			switch {
			case paneContains(copyDetails, first.ID):
				selected, other = first, c.independentManifests[1]
			case paneContains(copyDetails, second.ID):
				selected, other = second, c.independentManifests[0]
			default:
				t.Fatal("browser details do not show either exact independent SourceID", copyDetails)
			}
			before = treeManifest(t, fx.base)
			deletePreview := send("X", "PERMANENT DELETE", "ready")
			capture("19-delete-preview", deletePreview)
			if !paneContainsUnwrappedPath(deletePreview, selected.SourcePath) {
				t.Fatalf("delete preview target does not match selected source path %s:\n%s", selected.SourcePath, deletePreview)
			}
			warningFrame := deletePreview
			if !paneContains(warningFrame, "unverified.") {
				warningFrame = send("G", "unverified.")
			}
			capture("19b-delete-warning-bottom", warningFrame)
			assertALE42DeleteWarning(t, warningFrame)
			send("Escape", "Cancelled. No further content changed.")
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			deletePreview = send("X", "PERMANENT DELETE", "ready")
			if !paneContainsUnwrappedPath(deletePreview, selected.SourcePath) {
				t.Fatalf("second delete preview target changed from selected source path %s:\n%s", selected.SourcePath, deletePreview)
			}
			send("Enter", "Type: confirm delete", "Input:")
			runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "-l", "confirm delete")
			confirmation := waitForPane(t, tmux, fx, socket, false, "Type: confirm delete", "Input: confirm delete")
			capture("20-exact-confirmation", confirmation)
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			capture("21-removed", send("Enter", "Permanent delete result: removed", "Verified removal: true"))
			if _, err := os.Stat(filepath.Join(selected.SourcePath, "SKILL.md")); !os.IsNotExist(err) {
				t.Fatal("wrong source deleted", err)
			}
			readBytes(t, other)
			readBytes(t, c.sharedManifest)
			if string(readBytes(t, c.externalResource)) != "EXTERNAL_RESOURCE_SURVIVES\n" {
				t.Fatal("external target changed")
			}
			if _, err := os.Stat(c.executionMarker); !os.IsNotExist(err) {
				t.Fatal("bundled code executed", err)
			}
			if sharedID == selected.ID {
				t.Fatal("independent selection merged")
			}
			runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "q")
			status, err := waitForTmuxExit(t, tmux, fx, socket, "browser:0.0", 0, 5*time.Second)
			writeEvidence(t, fmt.Sprintf("tmux/ale42/%d/exit.txt", width), []byte(status))
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}
