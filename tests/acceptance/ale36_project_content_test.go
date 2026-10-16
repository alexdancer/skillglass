package acceptance_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func gateContentInventory(t *testing.T, fx fixture, project bool) codexList {
	t.Helper()
	args := []string{"list", "--agent", "opencode", "--json"}
	if project {
		args = append(args, "--project", fx.project)
	}
	r := fx.run(t, args...)
	assertExit(t, r, 0)
	var out codexList
	decodeJSON(t, r.stdout, &out)
	for _, s := range out.Sources {
		if s.Name == "target" || s.Name == "bundle-owner" {
			t.Logf("inventory project=%t source=%s id=%s path=%s state=%s", project, s.Name, s.ID, s.SourcePath, s.Availability.State)
		}
	}
	return out
}
func gateContentOwnerCount(list codexList) int {
	n := 0
	for _, s := range list.Sources {
		if s.Name == "bundle-owner" {
			n++
		}
	}
	return n
}
func gateContentControl(t *testing.T, fx fixture, choice, id, token string) (commandResult, changeReceipt) {
	t.Helper()
	args := []string{choice, "--agent", "opencode", "--scope", "project", "--project", fx.project, "--json"}
	if token != "" {
		args = append(args, "--confirm", token)
	}
	r := changeCommand(t, fx, "", append(args, id)...)
	var out changeReceipt
	decodeJSON(t, r.stdout, &out)
	t.Logf("project %s confirmation=%t exit=%d state=%s persistence=%s achievement=%s reason=%s", choice, token != "", r.code, out.State, out.Persistence, out.Achievement, out.Reason)
	return r, out
}
func TestALE36GateProjectDestinationMustPreserveHiddenGlobalBundle(t *testing.T) {
	for _, choice := range []string{"disable", "enable", "inherit", "late-disable", "late-enable", "late-inherit"} {
		t.Run(choice, func(t *testing.T) {
			late := strings.HasPrefix(choice, "late-")
			choice = strings.TrimPrefix(choice, "late-")
			fx, _ := codexFixture(t)
			target := codexSkill(t, filepath.Join(fx.xdgConfig, "opencode/skills"), "ordinary", "target")
			bundles := filepath.Join(fx.base, "allowed", "bundles")
			repo := filepath.Join(bundles, "repo")
			if err := os.MkdirAll(repo, 0700); err != nil {
				t.Fatal(err)
			}
			fx.project = repo
			global := filepath.Join(fx.xdgConfig, "opencode/opencode.json")
			globalRaw, _ := json.Marshal(map[string]any{"skills": map[string]any{"paths": []string{bundles}}, "theme": "global-unchanged"})
			writeFile(t, global, globalRaw, 0600)
			config := filepath.Join(repo, "opencode.json")
			raw := []byte(`{"skills":{"paths":[]},"theme":"bundle-resource"}`)
			if choice != "disable" {
				raw = []byte(`{"skills":{"paths":[]},"permission":{"skill":{"target":"deny"}},"theme":"bundle-resource"}`)
			}
			writeFile(t, config, raw, 0640)
			manifest := ""
			if !late {
				manifest = codexSkill(t, bundles, "repo", "bundle-owner")
			}
			selected := gateContentInventory(t, fx, true)
			globalBefore := gateContentInventory(t, fx, false)
			wantOwners := 1
			if late {
				wantOwners = 0
			}
			if gateContentOwnerCount(selected) != 0 || gateContentOwnerCount(globalBefore) != wantOwners {
				t.Fatal("fixture must hide global bundle only in selected context")
			}
			id := codexNamed(t, selected, "target").ID
			before := treeManifest(t, fx.base)
			r, p := gateContentControl(t, fx, choice, id, "")
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			if p.State != "ready" {
				if late {
					t.Fatalf("ordinary initial project preview must be ready before bundle exists: %s", p.Reason)
				}
				assertExit(t, r, 1)
				if p.State != "refused" || !strings.Contains(p.Reason, "content") {
					t.Fatal(p)
				}
				return
			}
			if late {
				manifest = codexSkill(t, bundles, "repo", "bundle-owner")
				if gateContentOwnerCount(gateContentInventory(t, fx, true)) != 0 || gateContentOwnerCount(gateContentInventory(t, fx, false)) != 1 {
					t.Fatal("new bundle must be visible globally and hidden in selected project")
				}
			}
			ownerInfo, err := os.Stat(manifest)
			if err != nil {
				t.Fatal(err)
			}
			targetInfo, err := os.Stat(target)
			if err != nil {
				t.Fatal(err)
			}
			if os.SameFile(ownerInfo, targetInfo) {
				t.Fatal("target and bundle owner must be independent")
			}
			configBefore, err := os.Stat(config)
			if err != nil {
				t.Fatal(err)
			}
			manifestBytes, targetBytes := readBytes(t, manifest), readBytes(t, target)
			applied, out := gateContentControl(t, fx, choice, id, p.Confirmation)
			afterInfo, err := os.Stat(config)
			if err != nil {
				t.Fatal(err)
			}
			ownerAfter, err := os.Stat(manifest)
			if err != nil {
				t.Fatal(err)
			}
			gateContentInventory(t, fx, true)
			gateContentInventory(t, fx, false)
			t.Logf("independent identities=true owner manifest identity preserved=%t; config mode before=%#o after=%#o; bundle resource before=%s after=%s", os.SameFile(ownerInfo, ownerAfter), configBefore.Mode().Perm(), afterInfo.Mode().Perm(), raw, readBytes(t, config))
			if !bytes.Equal(manifestBytes, readBytes(t, manifest)) || !bytes.Equal(targetBytes, readBytes(t, target)) || !bytes.Equal(globalRaw, readBytes(t, global)) {
				t.Fatal("unrelated manifest, target content or global settings changed")
			}
			if configBefore.Mode().Perm() != afterInfo.Mode().Perm() {
				t.Fatal("configuration mode changed")
			}
			if applied.code == 0 {
				t.Fatalf("project %s must refuse destination inside hidden independent bundle%s: got exit=0 state=%s achievement=%s and resource_changed=%t", choice, map[bool]string{true: " added after preview", false: ""}[late], out.State, out.Achievement, !bytes.Equal(raw, readBytes(t, config)))
			}
			if !bytes.Equal(raw, readBytes(t, config)) {
				t.Fatal("refusal/stale result must preserve skill supporting resource bytes")
			}
		})
	}
}
