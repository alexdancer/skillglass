package acceptance_test

import (
	"path/filepath"
	"testing"

	"github.com/alexdancer/skillglass/internal/collection"
)

func TestALE86ScanReportsExactNativeRelocationProfileRegistry(t *testing.T) {
	fx := newFixture(t)
	writeFile(t, filepath.Join(fx.home, ".codex", "skills", "home-canary", "SKILL.md"), []byte("---\nname: acceptance-home-canary\ndescription: bounded fixture canary\n---\nFixture only.\n"), 0o644)
	library := filepath.Join(fx.xdgData, "skillglass", "skills")
	scan := fx.run(t, "consolidate", "scan", "--app", "codex", "--app", "claude-code", "--app", "pi", "--app", "opencode", "--library", library, "--json")
	assertExit(t, scan, 0)
	var inventory collection.ConsolidationInventory
	decodeJSON(t, scan.stdout, &inventory)
	expected := map[string]struct{ id, profile, digest string }{
		"codex":       {"codex-directory-alias-v1", "Codex CLI 0.162.1", "74c6a6d263f40d65f99c27a815275f5b4ed0bcc2baecc396b4cf3255498bdfc0"},
		"claude-code": {"claude-code-directory-alias-v1", "Claude Code 2.1.309", "5eadc0009cdcdbc0d06e58110d7b2baf88ced6088b1c33a60a21217fe410619c"},
		"opencode":    {"opencode-directory-alias-v1", "OpenCode 1.17.15", "7bdefaeaef5cc4f661988eaba00de047f5f65547fd22a3bed5ba7c4d86a275d3"},
		"pi":          {"pi-directory-alias-v1", "Pi 1.4.0", "e79626f2dd6f94aa45d30f3fa63cd84319a6eefcd150b353cfaf274366926774"},
	}
	for _, app := range inventory.Applications {
		want, validated := expected[app.Application]
		if validated {
			profile := app.NativeRelocationProfile
			if app.NativeRelocationStatus != "validated-profile" || profile == nil || profile.ID != want.id || profile.Profile != want.profile || profile.BinarySHA256 != want.digest {
				t.Errorf("%s profile differs from the maintained exact native observation: %#v", app.Application, app)
			}
			continue
		}
		if app.Application == "hermes" {
			profile := app.NativeRelocationProfile
			if app.NativeRelocationStatus != "validated-profile" || profile == nil || profile.ID != "hermes-default-root-index-alias-v1" || profile.BinarySHA256 != "e3938f2a272dafdc33f3dd12b093dfc85354629476cb97a7d7d4a7201b8482dd" || profile.SourceRevision != "5a16635f409e0f5e3d754bb6256bd27707bcec4c" || profile.LoaderSHA256 != "d420eeafd237d7beccb91bf68014f1bc6cb96d3e06ec45cb6c707587d25b909b" {
				t.Errorf("Hermes default-root profile has incorrect scope or source/runtime identities: %#v", app)
			}
		}
		if app.Application == "gemini" {
			if app.NativeRelocationStatus != "unsupported" || app.NativeRelocationProfile != nil {
				t.Errorf("%s must stay visible but movement-unsupported without a recorded profile: %#v", app.Application, app)
			}
		}
	}
}

func TestALE86UnvalidatedNativeSourceRemainsVisibleAndKeepable(t *testing.T) {
	fx := newFixture(t)
	writeFile(t, filepath.Join(fx.home, ".codex", "skills", "home-canary", "SKILL.md"), []byte("---\nname: acceptance-home-canary\ndescription: bounded fixture canary\n---\nFixture only.\n"), 0o644)
	geminiRoot := filepath.Join(fx.home, ".gemini", "skills")
	geminiSource := filepath.Join(geminiRoot, "unvalidated-source")
	writeFile(t, filepath.Join(geminiSource, "SKILL.md"), []byte("---\nname: unvalidated-source\ndescription: gate fixture\n---\nFixture content.\n"), 0o644)
	library := filepath.Join(fx.xdgData, "skillglass", "skills")
	scan := fx.run(t, "consolidate", "scan", "--app", "gemini", "--library", library, "--json")
	assertExit(t, scan, 0)
	var inventory collection.ConsolidationInventory
	decodeJSON(t, scan.stdout, &inventory)
	var gemini collection.ConsolidationApplication
	for _, app := range inventory.Applications {
		if app.Application == "gemini" {
			gemini = app
			break
		}
	}
	if gemini.NativeRelocationStatus != "unsupported" || gemini.NativeRelocationProfile != nil {
		t.Fatalf("unvalidated Gemini movement gate is not visible in scan: %#v", gemini)
	}
	source := sourceAtPath(t, inventory, geminiSource)
	if !source.Eligible {
		t.Fatalf("readable Gemini source should remain inspectable and keepable: %#v", source.BlockedReasons)
	}

	keep := edit1Draft(inventory, nil)
	keepPath := filepath.Join(fx.base, "allowed", "ownership-artifacts", "gemini-keep.json")
	edit1WriteDraft(t, keepPath, keep)
	keepPreview := fx.run(t, "consolidate", "preview", "--draft", keepPath, "--json")
	assertExit(t, keepPreview, 0)

}

func TestALE86UnknownPortabilityApprovalCannotWaiveNativeDestinationGate(t *testing.T) {
	fx := newFixture(t)
	writeFile(t, filepath.Join(fx.home, ".codex", "skills", "home-canary", "SKILL.md"), []byte("---\nname: acceptance-home-canary\ndescription: bounded fixture canary\n---\nFixture only.\n"), 0o644)
	piSource := filepath.Join(fx.home, ".pi", "agent", "skills", "portable-to-unknown")
	writeFile(t, filepath.Join(piSource, "SKILL.md"), []byte("---\nname: portable-to-unknown\ndescription: gate fixture\n---\nFixture content.\n"), 0o644)
	library := filepath.Join(fx.xdgData, "skillglass", "skills")
	scan := fx.run(t, "consolidate", "scan", "--app", "pi", "--app", "gemini", "--library", library, "--json")
	assertExit(t, scan, 0)
	var inventory collection.ConsolidationInventory
	decodeJSON(t, scan.stdout, &inventory)
	pi := sourceAtPath(t, inventory, piSource)
	draft := edit1Draft(inventory, map[collection.SourceID]string{pi.ID: "share"})
	choiceIndex := -1
	for index := range draft.Choices {
		if draft.Choices[index].SourceID == pi.ID {
			choiceIndex = index
			break
		}
	}
	if choiceIndex < 0 {
		t.Fatal("Pi source is missing from the complete proposal")
	}
	for index := range draft.Choices[choiceIndex].Destinations {
		if draft.Choices[choiceIndex].Destinations[index].Application == "gemini" {
			draft.Choices[choiceIndex].Destinations[index].Portability = "unknown"
			draft.Choices[choiceIndex].Destinations[index].UserApproved = true
			draft.Choices[choiceIndex].Destinations[index].Reason = "the user explicitly approved the stated portability uncertainty for this fixture"
		}
	}
	draftPath := filepath.Join(fx.base, "allowed", "ownership-artifacts", "unknown-gemini-destination.json")
	edit1WriteDraft(t, draftPath, draft)
	before := treeManifest(t, fx.base)
	result := fx.run(t, "consolidate", "preview", "--draft", draftPath, "--json")
	assertExit(t, result, 1)
	var preview collection.ConsolidationPreview
	decodeJSON(t, result.stdout, &preview)
	if preview.Failure == nil || preview.Failure.Code != "unsupported-native-relocation" || preview.State != "refused" {
		t.Fatalf("unknown portability approval bypassed or obscured native destination evidence: %#v", preview)
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
}

func TestALE86HermesDefaultRootProfileAllowsOnlyProvenAliasRelocation(t *testing.T) {
	fx := newFixture(t)
	writeFile(t, filepath.Join(fx.home, ".codex", "skills", "home-canary", "SKILL.md"), []byte("---\nname: acceptance-home-canary\ndescription: bounded fixture canary\n---\nFixture only.\n"), 0o644)
	piSource := filepath.Join(fx.home, ".pi", "agent", "skills", "hermes-compatible")
	writeFile(t, filepath.Join(piSource, "SKILL.md"), []byte("---\nname: hermes-compatible\ndescription: default-root profile fixture\n---\nApplication-neutral instructions.\n"), 0o644)
	library := filepath.Join(fx.xdgData, "skillglass", "skills")
	scan := fx.run(t, "consolidate", "scan", "--app", "pi", "--app", "hermes", "--library", library, "--json")
	assertExit(t, scan, 0)
	var inventory collection.ConsolidationInventory
	decodeJSON(t, scan.stdout, &inventory)
	var hermes collection.ConsolidationApplication
	for _, app := range inventory.Applications {
		if app.Application == "hermes" {
			hermes = app
			break
		}
	}
	if hermes.NativeRelocationProfile == nil || hermes.NativeRelocationProfile.ID != "hermes-default-root-index-alias-v1" {
		t.Fatalf("ordinary default-root Hermes probe was not scoped and exposed: %#v", hermes)
	}
	pi := sourceAtPath(t, inventory, piSource)
	draft := edit1Draft(inventory, map[collection.SourceID]string{pi.ID: "share"})
	draftPath := filepath.Join(fx.base, "allowed", "ownership-artifacts", "hermes-default-root-share.json")
	edit1WriteDraft(t, draftPath, draft)
	before := treeManifest(t, fx.base)
	result := fx.run(t, "consolidate", "preview", "--draft", draftPath, "--json")
	assertExit(t, result, 0)
	var preview collection.ConsolidationPreview
	decodeJSON(t, result.stdout, &preview)
	if preview.State != "ready" {
		t.Fatalf("exact default-root alias profile did not permit preview: %#v", preview)
	}
	var sawHermesProfile bool
	for _, effect := range preview.Effects {
		for _, profile := range effect.RelocationProfiles {
			if profile.ID == "hermes-default-root-index-alias-v1" && profile.BinarySHA256 == "e3938f2a272dafdc33f3dd12b093dfc85354629476cb97a7d7d4a7201b8482dd" {
				sawHermesProfile = true
			}
		}
	}
	if !sawHermesProfile {
		t.Fatalf("preview confirmation effects omitted the exact Hermes runtime/profile evidence: %#v", preview.Effects)
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
}

func TestALE86HermesActiveProfileSelectionWithoutProfileEvidenceStaysKeepOnly(t *testing.T) {
	fx := newFixture(t)
	writeFile(t, filepath.Join(fx.home, ".codex", "skills", "home-canary", "SKILL.md"), []byte("---\nname: acceptance-home-canary\ndescription: bounded fixture canary\n---\nFixture only.\n"), 0o644)
	writeFile(t, filepath.Join(fx.home, ".hermes", "active_profile"), []byte("research\n"), 0o600)
	hermesRoot := filepath.Join(fx.home, ".hermes", "profiles", "research", "skills")
	hermesSource := filepath.Join(hermesRoot, "active-profile-source")
	writeFile(t, filepath.Join(hermesSource, "SKILL.md"), []byte("---\nname: active-profile-source\ndescription: active profile fixture\n---\nFixture content.\n"), 0o644)
	library := filepath.Join(fx.xdgData, "skillglass", "skills")
	scan := fx.run(t, "consolidate", "scan", "--app", "hermes", "--library", library, "--json")
	assertExit(t, scan, 0)
	var inventory collection.ConsolidationInventory
	decodeJSON(t, scan.stdout, &inventory)
	for _, app := range inventory.Applications {
		if app.Application == "hermes" && (app.NativeRelocationStatus != "unsupported" || app.NativeRelocationProfile != nil) {
			t.Fatalf("active_profile selection was incorrectly covered by a direct/default-root profile: %#v", app)
		}
	}
	source := sourceAtPath(t, inventory, hermesSource)
	if !source.Eligible {
		t.Fatalf("readable active-profile source should remain inspectable and keepable: %#v", source.BlockedReasons)
	}
	keep := edit1Draft(inventory, nil)
	keepPath := filepath.Join(fx.base, "allowed", "ownership-artifacts", "hermes-active-profile-keep.json")
	edit1WriteDraft(t, keepPath, keep)
	keepPreview := fx.run(t, "consolidate", "preview", "--draft", keepPath, "--json")
	assertExit(t, keepPreview, 0)

	archive := edit1Draft(inventory, map[collection.SourceID]string{source.ID: "archive"})
	archivePath := filepath.Join(fx.base, "allowed", "ownership-artifacts", "hermes-active-profile-archive.json")
	edit1WriteDraft(t, archivePath, archive)
	refused := fx.run(t, "consolidate", "preview", "--draft", archivePath, "--json")
	assertExit(t, refused, 1)
	var preview collection.ConsolidationPreview
	decodeJSON(t, refused.stdout, &preview)
	if preview.Failure == nil || preview.Failure.Code != "unsupported-native-relocation" {
		t.Fatalf("unsupported active_profile archive did not fail closed: %#v", preview)
	}
}
