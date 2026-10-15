package terminal

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/alexdancer/skillglass/internal/collection"
)

func TestVerifyComparisonCompletionAcrossHelp(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"left", "right"} {
		bundle := filepath.Join(root, name)
		if err := os.Mkdir(bundle, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(bundle, "SKILL.md"), []byte("---\nname: same\ndescription: fixture\n---\nIdentical content\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	request, err := collection.NewRequest([]string{root}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := collection.Build(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	for _, withHelp := range []bool{false, true} {
		name := "control"
		if withHelp {
			name = "completion-during-help"
		}
		t.Run(name, func(t *testing.T) {
			var model tea.Model = newBrowserModel(snapshot, collection.Filters{Context: collection.FilterAll}, themeNone)
			model, _ = model.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
			key := func(code rune) tea.Cmd {
				next, cmd := model.Update(tea.KeyPressMsg{Code: code, Text: string(code)})
				model = next
				return cmd
			}
			key('m')
			key('j')
			command := key('v')
			if command == nil {
				t.Fatal("comparison key did not return a command")
			}
			if !strings.Contains(model.View().Content, "Reading current bundle contents...") {
				t.Fatalf("comparison did not start: %s", model.View().Content)
			}
			if withHelp {
				key('?')
				if !strings.Contains(model.View().Content, "Keys") {
					t.Fatalf("Help did not open: %s", model.View().Content)
				}
			}
			// Execute the real comparison command and deliver its real result while Help is visible.
			model, _ = model.Update(command())
			if withHelp {
				if !strings.Contains(model.View().Content, "Keys") {
					t.Fatalf("comparison completion closed Help: %s", model.View().Content)
				}
				model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
			}
			got := model.View().Content
			if !strings.Contains(got, "Coverage: complete | identical") {
				t.Fatalf("returned comparison missing literal %q:\n%s", "Coverage: complete | identical", got)
			}
		})
	}
	t.Run("abandoned-generation", func(t *testing.T) {
		var model tea.Model = newBrowserModel(snapshot, collection.Filters{Context: collection.FilterAll}, themeNone)
		model, _ = model.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
		key := func(code rune) tea.Cmd {
			next, cmd := model.Update(tea.KeyPressMsg{Code: code, Text: string(code)})
			model = next
			return cmd
		}
		key('m')
		key('j')
		abandoned := key('v')
		if abandoned == nil {
			t.Fatal("comparison key did not return a command")
		}
		model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
		before := model.View().Content
		stale := abandoned()
		model, _ = model.Update(stale)
		if model.View().Content != before {
			t.Fatal("abandoned completion changed the source inventory")
		}
		current := key('v')
		if current == nil {
			t.Fatal("second comparison key did not return a command")
		}
		key('?')
		model, _ = model.Update(stale)
		if !strings.Contains(model.View().Content, "Keys") {
			t.Fatal("stale completion closed Help")
		}
		model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
		if !strings.Contains(model.View().Content, "Reading current bundle contents...") {
			t.Fatalf("stale completion replaced the current comparison: %s", model.View().Content)
		}
		model, _ = model.Update(current())
		if !strings.Contains(model.View().Content, "Coverage: complete | identical") {
			t.Fatalf("current comparison did not complete: %s", model.View().Content)
		}
	})
}
