package acceptance_test

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func g2Text(name, extra string) string {
	return "---\nname: " + name + "\ndescription: Inert.\n" + extra + "---\nG2_FULL_CONTENT_SENTINEL\n"
}
func g2Plugin(t *testing.T, profile string) string {
	t.Helper()
	root := filepath.Join(profile, "plugins/portable")
	file := codexSkill(t, filepath.Join(root, "skills"), "explicit", "explicit")
	writeFile(t, filepath.Join(root, "plugin.json"), []byte(`{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"portable"}`), 0600)
	writeFile(t, filepath.Join(profile, "config.yaml"), []byte("plugins:\n  enabled: [portable]\n"), 0600)
	return file
}

func g2Seal(t *testing.T, fx fixture) func() {
	t.Helper()
	label := strings.ReplaceAll(t.Name(), "/", "_")
	files := map[string]any{}
	filepath.WalkDir(fx.base, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			t.Fatal(err)
		}
		info, err := d.Info()
		if err != nil {
			t.Fatal(err)
		}
		rel, _ := filepath.Rel(fx.base, p)
		row := map[string]any{"mode": fmt.Sprintf("%o", info.Mode())}
		if info.Mode()&os.ModeSymlink != 0 {
			v, e := os.Readlink(p)
			if e != nil {
				t.Fatal(e)
			}
			row["link"] = v
		} else if info.Mode().IsRegular() {
			v, e := os.ReadFile(p)
			if e != nil {
				t.Fatal(e)
			}
			row["bytes"] = v
		}
		files[rel] = row
		return nil
	})
	raw, _ := json.MarshalIndent(files, "", "  ")
	writeEvidence(t, "fixtures/"+label+".json", raw)
	before := treeManifest(t, fx.base)
	return func() { assertTreeUnchanged(t, before, treeManifest(t, fx.base)) }
}
func g2Inspect(t *testing.T, fx fixture, env []string, list codexList, file, wantState string) {
	t.Helper()
	s := gateSourceAt(t, list, filepath.Dir(file))
	expected := map[string]any{"state": wantState, "source": s, "scope": list.Scope}
	raw, _ := json.MarshalIndent(expected, "", "  ")
	writeEvidence(t, "observations/"+strings.ReplaceAll(t.Name(), "/", "_")+".json", raw)
	read := hermesRun(t, fx, env, "read", "--agent", "hermes", "--project", fx.project, "--json", s.ID)
	assertExit(t, read, listExit(list))
	var detail publicRead
	decodeJSON(t, read.stdout, &detail)
	original, e := os.ReadFile(file)
	if e != nil {
		t.Fatal(e)
	}
	if detail.Source == nil || detail.Source.Manifest.RawText != string(original) {
		t.Errorf("generic full-content read changed")
	}
	explain := hermesRun(t, fx, env, "explain", "--agent", "hermes", "--project", fx.project, s.ID)
	assertExit(t, explain, listExit(list))
	if !strings.Contains(string(explain.stdout), "Session: unknown") {
		t.Errorf("session uncertainty missing")
	}
	filtered := hermesList(t, fx, listExit(list), env, "--availability", wantState)
	found := false
	for _, x := range filtered.Sources {
		found = found || x.ID == s.ID
	}
	if !found {
		t.Errorf("literal %s filter lost selected source", wantState)
	}
	if s.Availability.State != wantState {
		t.Errorf("state=%s; want %s", s.Availability.State, wantState)
	}
}
func g2List(t *testing.T, fx fixture, wantExit int, env []string) codexList {
	t.Helper()
	r := hermesRun(t, fx, env, "list", "--agent", "hermes", "--project", fx.project, "--json")
	if r.code != wantExit {
		t.Errorf("list exit=%d; want %d", r.code, wantExit)
	}
	var list codexList
	decodeJSON(t, r.stdout, &list)
	return list
}
func listExit(list codexList) int {
	if list.Scope.Completeness == "incomplete" {
		return 1
	}
	return 0
}

func TestGeneration2Decision1Framing(t *testing.T) {
	for _, tc := range []struct{ name, text, state string }{
		{"eof", "---\nname: explicit\ndescription: Inert.\n---", "unavailable"},
		{"ellipsis", "---\nname: explicit\ndescription: Inert.\n...\nG2_FULL_CONTENT_SENTINEL\n", "unavailable"},
		{"newline", g2Text("explicit", ""), "explicit-only"},
		{"beyond-index-window", g2Text("explicit", "padding: '"+strings.Repeat("x", 4100)+"'\n"), "explicit-only"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx, p := hermesFixture(t)
			file := g2Plugin(t, p)
			writeFile(t, file, []byte(tc.text), 0600)
			defer g2Seal(t, fx)()
			list := g2List(t, fx, 0, nil)
			g2Inspect(t, fx, nil, list, file, tc.state)
		})
	}
	for _, tc := range []struct{ name, extra string }{
		{"profile-invalid-full-environment", "environments: [docker]\n"},
		{"profile-invalid-full-platform", "platforms: [unsupported-os]\n"},
		{"profile-invalid-requires-tools", "metadata: {hermes: {requires_tools: [inert]}}\n"},
		{"profile-invalid-requires-toolsets", "metadata: {hermes: {requires_toolsets: [inert]}}\n"},
		{"profile-invalid-fallback-tools", "metadata: {hermes: {fallback_for_tools: [inert]}}\n"},
		{"profile-invalid-fallback-toolsets", "metadata: {hermes: {fallback_for_toolsets: [inert]}}\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx, p := hermesFixture(t)
			file := codexSkill(t, filepath.Join(p, "skills"), "guarded", "guarded")
			writeFile(t, file, []byte("---\nname: guarded\ndescription: Inert.\n"+tc.extra+"...\nG2_FULL_CONTENT_SENTINEL\n"), 0600)
			defer g2Seal(t, fx)()
			list := g2List(t, fx, 0, nil)
			g2Inspect(t, fx, nil, list, file, "available")
			if tc.name == "profile-invalid-full-platform" {
				source := gateSourceAt(t, list, filepath.Dir(file))
				result := hermesRun(t, fx, nil, "read", "--agent", "hermes", "--project", fx.project, "--json", source.ID)
				assertExit(t, result, 0)
				var detail struct {
					Source struct {
						Availability struct {
							Invocation struct{ Evidence struct{ Reason string } }
						}
					}
				}
				decodeJSON(t, result.stdout, &detail)
				if strings.Contains(detail.Source.Availability.Invocation.Evidence.Reason, "differs") {
					t.Error("unframed full metadata changed invocation evidence")
				}
			}
		})
	}
}
func TestGeneration2Decision1Falsy(t *testing.T) {
	for _, tc := range []struct{ name, origin, extra, state string }{
		{"profile-platform-false", "profile", "platforms: false\n", "available"},
		{"external-platform-false", "external", "platforms: false\n", "available"},
		{"portable-platform-false", "portable", "platforms: false\n", "explicit-only"},
		{"profile-environment-false", "profile", "environments: false\n", "available"},
		{"external-environment-false", "external", "environments: false\n", "available"},
		{"platform-zero", "profile", "platforms: 0\n", "available"},
		{"platform-empty-map", "profile", "platforms: {}\n", "available"},
		{"environment-zero", "profile", "environments: 0\n", "available"},
		{"platform-null", "profile", "platforms: null\n", "available"},
		{"platform-empty-string", "profile", "platforms: ''\n", "available"},
		{"platform-float-zero", "profile", "platforms: 0.0\n", "available"},
		{"environment-null", "profile", "environments: null\n", "available"},
		{"environment-empty-string", "profile", "environments: ''\n", "available"},
		{"environment-empty-map", "profile", "environments: {}\n", "available"},
		{"environment-float-zero", "profile", "environments: 0.0\n", "available"},
		{"portable-environment-false", "portable", "environments: false\n", "explicit-only"},
		{"platform-tagged-string-false-control", "profile", "platforms: !!str false\n", "unavailable"},
		{"environment-string-false-control", "profile", "environments: 'false'\n", "unknown"},
		{"platform-empty-list-control", "profile", "platforms: []\n", "available"},
		{"environment-empty-list-control", "profile", "environments: []\n", "available"},
		{"platform-true-control", "profile", "platforms: true\n", "unavailable"},
		{"platform-string-false-control", "profile", "platforms: 'false'\n", "unavailable"},
		{"environment-docker-control", "profile", "environments: [docker]\n", "unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx, p := hermesFixture(t)
			root := filepath.Join(p, "skills")
			name := "guarded"
			var file string
			if tc.origin == "external" {
				root = filepath.Join(p, "shared")
			}
			if tc.origin == "portable" {
				file = g2Plugin(t, p)
				name = "explicit"
			} else {
				file = codexSkill(t, root, name, name)
			}
			writeFile(t, file, []byte(g2Text(name, tc.extra)), 0600)
			defer g2Seal(t, fx)()
			list := g2List(t, fx, 0, nil)
			g2Inspect(t, fx, nil, list, file, tc.state)
		})
	}
}
func TestGeneration2Decision1Merge(t *testing.T) {
	for _, tc := range []struct{ name, extra, state string }{
		{"overridden-merge-value", "defaults: &d {flag: off}\nmetadata: {<<: *d, flag: 'off'}\n", "explicit-only"},
		{"overridden-merge-sequence", "first: &a {flag: off}\nsecond: &b {flag: 'safe'}\nmetadata: {<<: [*b, *a]}\n", "explicit-only"},
		{"effective-implicit-control", "defaults: &d {flag: off}\nmetadata: {<<: *d}\n", "unknown"},
		{"effective-quoted-control", "defaults: &d {flag: 'off'}\nmetadata: {<<: *d}\n", "explicit-only"},
		{"effective-implicit-key-control", "defaults: &d {on: 'value'}\nmetadata: {<<: *d}\n", "unknown"},
		{"conflated-string-key-control", "defaults: &d {on: 'value'}\nmetadata: {<<: *d, 'on': 'quoted'}\n", "unknown"},
		{"key-alias-control", "key: &k flag\ndefaults: &d {*k: off}\nmetadata: {<<: *d, flag: 'off'}\n", "explicit-only"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx, p := hermesFixture(t)
			file := g2Plugin(t, p)
			writeFile(t, file, []byte(g2Text("explicit", tc.extra)), 0600)
			defer g2Seal(t, fx)()
			list := g2List(t, fx, 0, nil)
			g2Inspect(t, fx, nil, list, file, tc.state)
		})
	}
}
