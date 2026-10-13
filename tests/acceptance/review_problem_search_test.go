package acceptance_test

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func problemSearchFixture(t *testing.T) (fixture, string, string, string) {
	t.Helper()
	fx := newFixture(t)
	custom := filepath.Join(fx.base, "allowed", "roots", "Requested-Custom")
	project := filepath.Join(fx.base, "allowed", "roots", "Requested-Project")
	bundle := filepath.Join(fx.base, "allowed", "outside", "resolved-bundle")
	writeFile(t, filepath.Join(bundle, "SKILL.md"), []byte("---\nname: [\n---\nBODY\n"), 0o644)
	for root, alias := range map[string]string{custom: "Original-Custom-Alias", project: "Original-Project-Alias"} {
		if err := os.MkdirAll(root, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(bundle, filepath.Join(root, alias)); err != nil {
			t.Fatal(err)
		}
	}
	missing := filepath.Join(fx.base, "allowed", "roots", "Requested-Missing")
	return fx, custom, project, missing
}

func TestReviewProblemSearchUsesKnownProvenance(t *testing.T) {
	fx, custom, project, missing := problemSearchFixture(t)
	before := treeManifest(t, fx.base)
	args := []string{"--root", custom, "--root", missing, "--project", fx.project, "--project-root", project}
	query := func(extra ...string) publicList {
		result := fx.run(t, append(append([]string{"problems", "--json"}, args...), extra...)...)
		assertExit(t, result, 1)
		var view publicList
		decodeJSON(t, result.stdout, &view)
		return view
	}
	baseline := query()
	var malformed, rootOnly publicProblem
	for _, problem := range baseline.Problems {
		switch problem.Code {
		case "malformed-manifest":
			malformed = problem
		case "root-missing":
			rootOnly = problem
		}
	}
	if malformed.ID == "" || malformed.Detail == "" || len(malformed.ReferenceIDs) != 2 || rootOnly.ID == "" || len(rootOnly.ReferenceIDs) != 0 {
		t.Fatalf("unexpected fixture problems: %#v", baseline.Problems)
	}
	for _, tc := range []struct {
		name    string
		filters []string
		want    publicProblem
	}{
		{"code", []string{"--search", "MALFORMED-MANIFEST"}, malformed},
		{"summary", []string{"--search", malformed.Summary}, malformed},
		{"detail", []string{"--search", malformed.Detail}, malformed},
		{"resolved-path", []string{"--search", malformed.Path}, malformed},
		{"custom-alias", []string{"--search", "Original-Custom-Alias"}, malformed},
		{"project-alias-case-fold", []string{"--search", "ORIGINAL-PROJECT-ALIAS"}, malformed},
		{"custom-requested-root", []string{"--search", "REQUESTED-CUSTOM"}, malformed},
		{"project-requested-root", []string{"--search", "Requested-Project"}, malformed},
		{"matching-provenance", []string{"--search", "Original-Custom-Alias", "--source-root", custom, "--context", "custom"}, malformed},
		{"other-reference-search", []string{"--search", "Original-Project-Alias", "--source-root", custom, "--context", "custom"}, malformed},
		{"matching-project-provenance", []string{"--search", "Original-Custom-Alias", "--source-root", project, "--context", "project"}, malformed},
		{"mismatched-provenance", []string{"--search", "Original-Custom-Alias", "--source-root", custom, "--context", "project"}, publicProblem{}},
		{"mismatched-project-provenance", []string{"--search", "Original-Project-Alias", "--source-root", project, "--context", "custom"}, publicProblem{}},
		{"matching-code-filter", []string{"--search", "Original-Custom-Alias", "--problem", "malformed-manifest"}, malformed},
		{"mismatched-code-filter", []string{"--search", "Original-Custom-Alias", "--problem", "root-missing"}, publicProblem{}},
		{"root-only", []string{"--search", "REQUESTED-MISSING", "--source-root", missing, "--context", "custom"}, rootOnly},
		{"root-only-negative", []string{"--search", "REQUESTED-MISSING", "--context", "project"}, publicProblem{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			view := query(tc.filters...)
			if tc.want.ID == "" {
				if len(view.Problems) != 0 {
					t.Fatalf("unexpected matching problems: %#v", view.Problems)
				}
			} else if len(view.Problems) != 1 || !reflect.DeepEqual(view.Problems[0], tc.want) {
				t.Fatalf("problems = %#v, want unchanged %#v", view.Problems, tc.want)
			}
		})
	}
	text := fx.run(t, append(append([]string{"problems"}, args...), "--search", "Original-Custom-Alias")...)
	assertExit(t, text, 1)
	if !bytes.Contains(text.stdout, []byte("malformed-manifest")) {
		t.Fatalf("human problems search missed alias: %s", text.stdout)
	}
	list := fx.run(t, append(append([]string{"list", "--json"}, args...), "--search", "Original-Custom-Alias")...)
	assertExit(t, list, 1)
	var sources publicList
	decodeJSON(t, list.stdout, &sources)
	if len(sources.Sources) != 0 {
		t.Fatal("list search unexpectedly matches discovery aliases")
	}
	if !bytes.Equal(before, treeManifest(t, fx.base)) {
		t.Fatal("problem searches mutated fixture")
	}
}

func TestReviewBrowserProblemSearchFindsOriginalAlias(t *testing.T) {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatal("tmux is required for problem search regression")
	}
	fx, custom, project, _ := problemSearchFixture(t)
	before := treeManifest(t, fx.base)
	socket := fmt.Sprintf("skillglass-problem-search-%d", os.Getpid())
	defer runTmuxIgnoringError(tmux, fx, socket, "kill-server")
	commandLine := strings.Join([]string{"exec", fx.productShell(t, false), "browse", "--root", shellQuote(custom), "--project", shellQuote(fx.project), "--project-root", shellQuote(project)}, " ")
	runTmux(t, tmux, fx, socket, "new-session", "-d", "-x", "60", "-y", "30", "-s", "search", commandLine)
	runTmux(t, tmux, fx, socket, "set-option", "-g", "status", "off")
	runTmux(t, tmux, fx, socket, "set-option", "-w", "-t", "search:0", "remain-on-exit", "on")
	runTmux(t, tmux, fx, socket, "resize-window", "-t", "search:0", "-x", "60", "-y", "30")
	_ = waitForPaneAt(t, tmux, fx, socket, "search:0.0", false, "SKILLGLASS | Sources", "Enter read")
	key := func(value string) { runTmux(t, tmux, fx, socket, "send-keys", "-t", "search:0.0", "-l", value) }
	_ = actForPaneAt(t, tmux, fx, socket, "search:0.0", false, func() { key("\t") }, "SKILLGLASS | Problems", "malformed-manifest")
	frame := actForPaneAt(t, tmux, fx, socket, "search:0.0", false, func() { key("/"); key("ORIGINAL-PROJECT-ALIAS") }, "Search: ORIGINAL-PROJECT-ALIAS█", "malformed-manifest", "Problem focus: 1/1")
	assertPaneCells(t, frame, 60, 30)
	if strings.Contains(frame, "No matching problems.") {
		t.Fatal("alias search did not match a problem")
	}
	writeEvidence(t, "tmux/problem-search/alias.txt", []byte(frame))
	_ = actForPaneAt(t, tmux, fx, socket, "search:0.0", false, func() { key("\r") }, "Enter problem", "Problem focus: 1/1")
	_ = actForPaneAt(t, tmux, fx, socket, "search:0.0", false, func() { key("q") }, "Pane is dead")
	status := strings.TrimSpace(runTmux(t, tmux, fx, socket, "display-message", "-p", "-t", "search:0.0", "#{pane_dead}:#{pane_dead_status}"))
	if status != "1:1" {
		t.Fatalf("browser exit = %q, want incomplete status 1", status)
	}
	if !bytes.Equal(before, treeManifest(t, fx.base)) {
		t.Fatal("browser alias search mutated fixture")
	}
}
