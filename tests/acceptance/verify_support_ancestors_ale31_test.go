package acceptance_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifySupportAncestorsALE31(t *testing.T) {
	cases := []struct{ name, support, selector, mode, state, kind, count string }{}
	for _, support := range []string{"references", "templates", "assets", "scripts"} {
		for _, selector := range []string{"category/second", "category:second"} {
			cases = append(cases, struct{ name, support, selector, mode, state, kind, count string }{support + "/" + strings.ReplaceAll(selector, "/", "-"), support, selector, "present", "unambiguous-bare-name", "inferred", "1"})
		}
	}
	for _, tc := range []struct{ name, support, selector, mode, state, kind, count string }{
		{"absent-owner-control", "references", "category/second", "absent", "ambiguous-bare-name", "inferred", "2"},
		{"ordinary-directory-control", "ordinary", "category/second", "present", "ambiguous-bare-name", "inferred", "2"},
		{"nested-discovered-owner-control", "references", "owner/references/library/category/second", "discovered", "unambiguous-bare-name", "inferred", "1"},
		{"configured-root-alias", "references", "category/second", "root-alias", "unambiguous-bare-name", "inferred", "1"},
		{"lexical-reference-alias-control", "references", "category/second", "reference-alias", "ambiguous-bare-name", "inferred", "2"},
		{"recursive-name-control", "references", "category/second", "recursive-name", "ambiguous-bare-name", "inferred", "2"},
		{"same-manifest-control", "references", "category/second", "same-manifest", "unambiguous-bare-name", "inferred", "1"},
		{"denied-owner-target", "references", "category/second", "denied", "unknown", "unknown", "1"},
	} {
		cases = append(cases, tc)
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fx, p := hermesFixture(t)
			first := codexSkill(t, filepath.Join(p, "skills"), "first", tc.selector)
			writeFile(t, first, []byte(g2Text(tc.selector, "")), 0600)
			owner := filepath.Join(p, "outside-discovery", "owner")
			if tc.mode == "discovered" {
				owner = filepath.Join(p, "shared", "owner")
			}
			library := filepath.Join(owner, tc.support, "library")
			second := codexSkill(t, library, "category/second", "other")
			if tc.mode == "recursive-name" {
				writeFile(t, second, []byte(g2Text(tc.selector, "")), 0600)
			}
			if tc.mode != "absent" {
				if tc.mode == "denied" {
					target := filepath.Join(fx.base, "excluded", "ancestor-owner", "SKILL.md")
					writeFile(t, target, []byte(g2Text("owner", "")), 0600)
					if err := os.Symlink(target, filepath.Join(owner, "SKILL.md")); err != nil {
						t.Fatal(err)
					}
				} else {
					writeFile(t, filepath.Join(owner, "SKILL.md"), []byte(g2Text("owner", "")), 0600)
				}
			}
			root := library
			switch tc.mode {
			case "discovered":
				root = filepath.Join(p, "shared")
			case "root-alias":
				root = filepath.Join(p, "library-alias")
				if err := os.Symlink(library, root); err != nil {
					t.Fatal(err)
				}
			case "reference-alias":
				root = filepath.Join(p, "shared")
				if err := os.MkdirAll(filepath.Join(root, "category"), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Dir(second), filepath.Join(root, "category/second")); err != nil {
					t.Fatal(err)
				}
			case "same-manifest":
				if err := os.Remove(second); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(first, second); err != nil {
					t.Fatal(err)
				}
			}
			writeFile(t, filepath.Join(p, "config.yaml"), []byte(fmt.Sprintf("skills:\n  external_dirs: [%q]\n", root)), 0600)
			defer g2Seal(t, fx)()
			if tc.mode == "denied" {
				probe := fx.run(t, "list", "--root", owner, "--json")
				assertExit(t, probe, 1)
				if !strings.Contains(string(probe.stdout), "permission denied") && !strings.Contains(string(probe.stdout), "operation not permitted") {
					t.Errorf("owned ancestor target denial not observed: %s", probe.stdout)
				}
			}
			var list codexList
			if tc.mode == "denied" {
				result := hermesRun(t, fx, nil, "list", "--agent", "hermes", "--project", fx.project, "--json")
				if result.code != 0 && result.code != 1 {
					t.Fatalf("list exit=%d: %s", result.code, result.stderr)
				}
				decodeJSON(t, result.stdout, &list)
			} else {
				list = g2List(t, fx, 0, nil)
			}
			if tc.mode != "denied" {
				g2Inspect(t, fx, nil, list, first, "available")
			}
			source := gateSourceAt(t, list, filepath.Dir(first))
			invocation := g2ExplicitInvocation(t, fx, source)
			want := fmt.Sprintf("selector %q: %s (%s known distinct manifests)", tc.selector, tc.state, tc.count)
			valid := invocation.State == tc.state && invocation.Evidence.Kind == tc.kind && strings.Contains(invocation.Evidence.Reason, want)
			allowUnobserved := (tc.mode == "present" && tc.support != "ordinary") || tc.mode == "root-alias" || tc.mode == "absent"
			if allowUnobserved {
				alternative := fmt.Sprintf("selector %q: unknown (1 known distinct manifests)", tc.selector)
				if invocation.State == "unknown" && invocation.Evidence.Kind == "unknown" && strings.Contains(invocation.Evidence.Reason, alternative) {
					valid = true
					want = alternative
				}
			}
			if !valid {
				t.Errorf("want %s; aggregate %s %s (out-of-scope ancestry may instead be unknown with 1 known match: %t); got %+v", want, tc.state, tc.kind, allowUnobserved, invocation)
			}
			explain := hermesRun(t, fx, nil, "explain", "--agent", "hermes", "--project", fx.project, source.ID)
			assertExit(t, explain, listExit(list))
			if !strings.Contains(string(explain.stdout), want) {
				t.Errorf("public explanation omits required native selector fact %s", want)
			}
		})
	}
}
