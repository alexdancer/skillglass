package acceptance_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBrowserRefreshReconcilesSelectedProblem(t *testing.T) {
	for _, width := range []int{48, 120} {
		for _, throughScope := range []bool{false, true} {
			for _, repair := range []bool{true, false} {
				name := fmt.Sprintf("%d/scope-%t/repair-%t", width, throughScope, repair)
				t.Run(name, func(t *testing.T) {
					fx := newFixture(t)
					root := filepath.Join(fx.base, "allowed", "roots", "refresh")
					targetDirectory := filepath.Join(fx.base, "allowed", "outside", "repaired")
					if err := os.MkdirAll(root, 0o755); err != nil {
						t.Fatal(err)
					}
					if err := os.Symlink(targetDirectory, filepath.Join(root, "broken")); err != nil {
						t.Fatal(err)
					}
					writeSkill(t, filepath.Join(root, "stable"), "stable-source", "unrelated selected source", "STABLE_SOURCE")
					list := fx.run(t, "list", "--root", root, "--json")
					assertExit(t, list, 1)
					var inventory publicList
					decodeJSON(t, list.stdout, &inventory)
					if len(inventory.Problems) != 1 || inventory.Problems[0].Code != "broken-reference" {
						t.Fatalf("expected one broken reference, got %#v", inventory.Problems)
					}
					problem := inventory.Problems[0]
					stable := sourceNamed(t, inventory.Sources, "stable-source")
					tmux, socket, target := startALE43ReviewBrowser(t, fx, width, 30, "--root", root, "--theme", "dark")
					act := func(key string, needles ...string) string {
						return actForPaneAt(t, tmux, fx, socket, target, false, func() {
							runTmux(t, tmux, fx, socket, "send-keys", "-t", target, key)
						}, needles...)
					}
					_ = act("Enter", "Selected: stable-source", "STABLE_SOURCE", "Rendered instructions")
					_ = act("Tab", "Focused: broken-reference", "Enter problem")
					detail := act("Enter", "Selected problem: broken-reference", problem.ID, "Problem lines")
					identity := frameLineWithPrefix(t, detail, "Selected problem: ")
					writeEvidence(t, "ale43-review/problem-refresh/"+name+"/00-selected.txt", []byte(detail))
					if !repair {
						_ = act("c", "context project", identity, "Problem lines")
					}
					if throughScope {
						scope := act("s", "Scope and filters", identity)
						writeEvidence(t, "ale43-review/problem-refresh/"+name+"/01-scope.txt", []byte(scope))
					}
					if repair {
						writeSkill(t, targetDirectory, "repaired", "repaired reference", "REPAIRED_REFERENCE")
					}
					beforeRefresh := treeManifest(t, fx.base)
					coverage := "coverage incomplete"
					if repair {
						coverage = "coverage complete"
					}
					refreshed := act("r", coverage, "Refresh:")
					writeEvidence(t, "ale43-review/problem-refresh/"+name+"/02-refreshed.txt", []byte(refreshed))
					returned := refreshed
					if throughScope {
						frame := actOnPane(t, tmux, fx, socket, target, func() {
							runTmux(t, tmux, fx, socket, "send-keys", "-t", target, "Escape")
						}, func(frame paneSnapshot) bool {
							return !paneContains(frame.cells, "Scope and filters")
						})
						returned = frame.cells
					}
					writeEvidence(t, "ale43-review/problem-refresh/"+name+"/03-returned.txt", []byte(returned))
					styled := captureStyledPane(t, tmux, fx, socket, target)
					writeEvidence(t, "ale43-review/problem-refresh/"+name+"/03-returned.ansi", []byte(styled))
					assertActiveChromeTab(t, styled, "Problems")
					assertPaneCells(t, returned, width, 30)
					if repair {
						for _, frame := range []string{refreshed, returned} {
							if !strings.Contains(frame, "Refresh: selected problem") {
								t.Fatalf("active Problems context lost disappearance feedback:\n%s", frame)
							}
							if strings.Contains(frame, "Selected problem:") || strings.Contains(frame, "unavailable") || strings.Contains(frame, "Problem lines") {
								t.Fatalf("vanished problem kept a stale detail target or identity:\n%s", frame)
							}
						}
						for _, want := range []string{"Problems 0", "Focused: no matching problems", "No matching problems.", "Search / type to filter"} {
							if !paneContains(returned, want) {
								t.Fatalf("refresh did not return to Problems inventory with %q:\n%s", want, returned)
							}
						}
					} else {
						assertChromeIdentity(t, refreshed, identity)
						assertChromeIdentity(t, returned, identity)
						if !paneContains(returned, problem.ID) || !paneContains(returned, "Problem lines") {
							t.Fatalf("retained problem lost its detail view:\n%s", returned)
						}
						_ = act("Escape", "Focused: no matching problems", "No matching problems.", "Search / type to filter")
						_ = act("c", "context custom", "Focused: broken-reference")
						reopened := act("Enter", "Selected problem: broken-reference", problem.ID, "Problem lines")
						assertChromeIdentity(t, reopened, identity)
					}
					_ = act("Tab", "Focused: stable-source", "Search / type to filter")
					hidden := actForPaneAt(t, tmux, fx, socket, target, false, func() {
						keys := "c"
						if !repair {
							keys = "cc"
						}
						runTmux(t, tmux, fx, socket, "send-keys", "-t", target, "-l", keys)
					}, "context project", "Selected:", "hidden by filters")
					hint := strings.Trim(identityHint(frameLineWithPrefix(t, hidden, "Selected: ")), "[]")
					if hint == "" || !strings.HasPrefix(strings.TrimPrefix(stable.ID, "src_"), hint) {
						t.Fatalf("problem refresh did not preserve the selected source identity:\n%s", hidden)
					}
					writeEvidence(t, "ale43-review/problem-refresh/"+name+"/04-retained-source.txt", []byte(hidden))
					if !bytes.Equal(beforeRefresh, treeManifest(t, fx.base)) {
						t.Fatal("refresh/navigation mutated fixture content or settings")
					}
				})
			}
		}
	}
}

func TestBrowserRefreshFeedbackUsesActiveSourceContext(t *testing.T) {
	for _, width := range []int{48, 120} {
		for _, reader := range []bool{false, true} {
			for _, throughScope := range []bool{false, true} {
				for _, replace := range []bool{true, false} {
					name := fmt.Sprintf("%d/reader-%t/scope-%t/replace-%t", width, reader, throughScope, replace)
					t.Run(name, func(t *testing.T) {
						fx := newFixture(t)
						root := filepath.Join(fx.base, "allowed", "roots", "feedback")
						sourceDirectory := filepath.Join(root, "source")
						repairDirectory := filepath.Join(fx.base, "allowed", "outside", "repaired")
						writeSkill(t, sourceDirectory, "a-source", "selected before problem visit", "A_SOURCE")
						if err := os.Symlink(repairDirectory, filepath.Join(root, "broken")); err != nil {
							t.Fatal(err)
						}
						initial := fx.run(t, "list", "--root", root, "--json")
						assertExit(t, initial, 1)
						var inventory publicList
						decodeJSON(t, initial.stdout, &inventory)
						source := sourceNamed(t, inventory.Sources, "a-source")
						if len(inventory.Problems) != 1 || inventory.Problems[0].Code != "broken-reference" {
							t.Fatalf("expected one broken reference, got %#v", inventory.Problems)
						}
						tmux, socket, target := startALE43ReviewBrowser(t, fx, width, 30, "--root", root, "--theme", "dark")
						act := func(key string, needles ...string) string {
							return actForPaneAt(t, tmux, fx, socket, target, false, func() {
								runTmux(t, tmux, fx, socket, "send-keys", "-t", target, key)
							}, needles...)
						}
						_ = act("Enter", "Selected: a-source", "Rendered instructions", "A_SOURCE")
						_ = act("d", "Details", "Source ID: "+source.ID)
						_ = act("Tab", "Focused: broken-reference", "Enter problem")
						problem := act("Enter", "Selected problem: broken-reference", inventory.Problems[0].ID, "Problem lines")
						writeEvidence(t, "ale43-review/refresh-feedback/"+name+"/00-problem.txt", []byte(problem))
						returned := act("Tab", "Focused: a-source", "Search / type to filter")
						if reader {
							_ = act("Enter", "Selected: a-source", "Rendered instructions", "A_SOURCE")
							returned = act("d", "Selected: a-source", "Details", "Source ID: "+source.ID)
						}
						writeEvidence(t, "ale43-review/refresh-feedback/"+name+"/01-source-context.txt", []byte(returned))
						if throughScope {
							scope := act("s", "Scope and filters")
							writeEvidence(t, "ale43-review/refresh-feedback/"+name+"/02-scope.txt", []byte(scope))
						}
						writeSkill(t, repairDirectory, "repaired", "external reference repair", "REPAIRED_REFERENCE")
						if replace {
							if err := os.Rename(sourceDirectory, filepath.Join(fx.base, "allowed", "outside", "original-source")); err != nil {
								t.Fatal(err)
							}
							writeSkill(t, sourceDirectory, "a-source", "selected before problem visit", "A_SOURCE")
						}
						beforeRefresh := treeManifest(t, fx.base)
						afterEdit := fx.run(t, "list", "--root", root, "--json")
						assertExit(t, afterEdit, 0)
						var updated publicList
						decodeJSON(t, afterEdit.stdout, &updated)
						current := sourceNamed(t, updated.Sources, "a-source")
						if (current.ID != source.ID) != replace || len(updated.Problems) != 0 {
							t.Fatalf("external edit did not establish replacement=%t and problem disappearance: %#v", replace, updated)
						}
						refreshed := act("r", "coverage complete", "Refresh:")
						writeEvidence(t, "ale43-review/refresh-feedback/"+name+"/03-refreshed.txt", []byte(refreshed))
						returned = refreshed
						if throughScope {
							frame := actOnPane(t, tmux, fx, socket, target, func() {
								runTmux(t, tmux, fx, socket, "send-keys", "-t", target, "Escape")
							}, func(frame paneSnapshot) bool {
								return !paneContains(frame.cells, "Scope and filters")
							})
							returned = frame.cells
						}
						writeEvidence(t, "ale43-review/refresh-feedback/"+name+"/04-returned.txt", []byte(returned))
						styled := captureStyledPane(t, tmux, fx, socket, target)
						writeEvidence(t, "ale43-review/refresh-feedback/"+name+"/04-returned.ansi", []byte(styled))
						assertActiveChromeTab(t, styled, "Sources")
						wantFeedback := "Refresh: selected directory"
						if replace {
							wantFeedback = "Refresh: selection cleared"
						}
						for _, frame := range []string{refreshed, returned} {
							assertPaneCells(t, frame, width, 30)
							if !strings.Contains(frame, wantFeedback) || strings.Contains(frame, "Refresh: selected problem") {
								t.Fatalf("historical problem replaced active source feedback %q:\n%s", wantFeedback, frame)
							}
						}
						if reader && !replace {
							for _, want := range []string{"Selected: a-source", "Details", "Source ID: " + source.ID} {
								if !paneContains(returned, want) {
									t.Fatalf("retained source lost its reader state %q:\n%s", want, returned)
								}
							}
							_ = act("Escape", "Focused: a-source", "Search / type to filter")
						} else if !paneContains(returned, "Search / type to filter") || strings.Contains(returned, "Selected:") {
							t.Fatalf("source refresh did not reconcile inventory/selection:\n%s", returned)
						}
						filtered := act("c", "context project", "No matching sources.")
						writeEvidence(t, "ale43-review/refresh-feedback/"+name+"/05-filtered-selection.txt", []byte(filtered))
						if replace {
							if strings.Contains(filtered, "Selected:") {
								t.Fatalf("replacement inherited the old source selection:\n%s", filtered)
							}
						} else {
							selection := frameLineWithPrefix(t, filtered, "Selected: ")
							hint := strings.Trim(identityHint(selection), "[]")
							if !strings.Contains(selection, "hidden by filters") || hint == "" || !strings.HasPrefix(strings.TrimPrefix(source.ID, "src_"), hint) {
								t.Fatalf("retained source identity was lost:\n%s", filtered)
							}
						}
						if !bytes.Equal(beforeRefresh, treeManifest(t, fx.base)) {
							t.Fatal("refresh/navigation mutated the externally edited fixture or settings")
						}
					})
				}
			}
		}
	}
}
