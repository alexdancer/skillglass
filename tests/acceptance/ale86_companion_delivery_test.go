package acceptance_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/alexdancer/skillglass/internal/companion"
	"github.com/alexdancer/skillglass/internal/setup"
)

func TestALE86NamedCompanionIsExplicitAndSetupKeepsReviewDefault(t *testing.T) {
	fx := ale48Fixture(t)
	named, err := companion.BundledNamed(companion.ConsolidationName)
	if err != nil {
		t.Fatal(err)
	}
	if named.Name != "skillglass-consolidate" || named.ConsolidationContractVersion != 1 || named.ContractVersion != 0 || len(named.Files) == 0 {
		t.Fatalf("unexpected consolidation bundle metadata: %#v", named)
	}

	before := treeManifest(t, fx.base)
	defaultBundleResult := ale48Run(t, fx, "", filepath.Join(fx.home, "bin"), "companion", "--json")
	assertExit(t, defaultBundleResult, 0)
	var defaultBundle companion.Bundle
	decodeJSON(t, defaultBundleResult.stdout, &defaultBundle)
	if !reflect.DeepEqual(defaultBundle, companion.Bundled()) || defaultBundle.Name != companion.Name || defaultBundle.ContractVersion != companion.ContractVersion || defaultBundle.ConsolidationContractVersion != 0 {
		t.Fatalf("default companion changed from the review bundle: %#v", defaultBundle)
	}

	namedBundleResult := ale48Run(t, fx, "", filepath.Join(fx.home, "bin"), "companion", "--name", companion.ConsolidationName, "--json")
	assertExit(t, namedBundleResult, 0)
	var actualNamed companion.Bundle
	decodeJSON(t, namedBundleResult.stdout, &actualNamed)
	if !reflect.DeepEqual(actualNamed, named) {
		t.Fatalf("named companion metadata/files differ from embedded artifact")
	}
	fileResult := ale48Run(t, fx, "", filepath.Join(fx.home, "bin"), "companion", "--name", companion.ConsolidationName, "--file", "SKILL.md", "--json")
	assertExit(t, fileResult, 0)
	var filePayload struct {
		Name                         string         `json:"name"`
		ConsolidationContractVersion int            `json:"consolidation_contract_version"`
		File                         companion.File `json:"file"`
	}
	decodeJSON(t, fileResult.stdout, &filePayload)
	if filePayload.Name != companion.ConsolidationName || filePayload.ConsolidationContractVersion != 1 || filePayload.File.Content != named.Files[0].Content || filePayload.File.SHA256 != named.Files[0].SHA256 {
		t.Fatalf("named file lookup does not expose its exact contract and bytes: %#v", filePayload)
	}

	for _, args := range [][]string{
		{"-h"},
		{"companion", "-h"},
		{"setup", "-h"},
		{"setup", "--companion", companion.ConsolidationName, "-h"},
		{"consolidate", "-h"},
		{"consolidate", "scan", "-h"},
		{"consolidate", "preview", "-h"},
		{"consolidate", "apply", "-h"},
		{"consolidate", "recover", "-h"},
	} {
		result := ale48Run(t, fx, "", filepath.Join(fx.home, "bin"), args...)
		assertExit(t, result, 0)
		if len(result.stderr) != 0 {
			t.Fatalf("help wrote diagnostics for %v: %s", args, result.stderr)
		}
		if !strings.Contains(string(result.stdout), "skillglass-consolidate") {
			t.Fatalf("help for %v does not expose the named workflow: %s", args, result.stdout)
		}
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))

	for _, args := range [][]string{{"setup", "--check", "--json"}, {"setup", "--check", "--companion", companion.ConsolidationName, "--json"}} {
		result := ale48Run(t, fx, "", filepath.Join(fx.home, "bin"), args...)
		assertExit(t, result, 0)
		var report struct {
			State      string                `json:"state"`
			Executable setup.ExecutableCheck `json:"executable"`
		}
		decodeJSON(t, result.stdout, &report)
		if report.State != "compatible" || report.Executable.State != "compatible" {
			t.Fatalf("compatible current executable was not recognized for %v: %#v", args, report)
		}
	}

	preview := ale86Setup(t, fx, companion.ConsolidationName, false, 0, "codex", "codeaf")
	if preview.State != "preview" || len(preview.Results) != 1 || preview.Results[0].State != "ready" || preview.Results[0].Destination != filepath.Join(fx.home, ".codex", "skills", companion.ConsolidationName) || preview.ConsolidationContractVersion != 1 || preview.ContractVersion != 0 {
		t.Fatalf("named setup preview is incorrect or broadened: %#v", preview)
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))

	installed := ale86Setup(t, fx, companion.ConsolidationName, true, 0, "codex", "codeaf")
	if installed.State != "complete" || installed.Results[0].State != "installed" {
		t.Fatalf("named companion installation failed: %#v", installed)
	}
	consolidationPath := filepath.Join(fx.home, ".codex", "skills", companion.ConsolidationName)
	ale86AssertInstalledBundle(t, consolidationPath, named)
	if _, err := os.Lstat(filepath.Join(fx.home, ".codex", "skills", companion.Name)); !os.IsNotExist(err) {
		t.Fatalf("named installation also installed the default review bundle: %v", err)
	}
	stable := treeManifest(t, fx.base)
	unchanged := ale86Setup(t, fx, companion.ConsolidationName, true, 0, "codex", "codeaf")
	if unchanged.Results[0].State != "already-present" {
		t.Fatalf("exact named companion rerun is not idempotent: %#v", unchanged)
	}
	assertTreeUnchanged(t, stable, treeManifest(t, fx.base))

	review := ale86Setup(t, fx, companion.Name, true, 0, "codex", "codeaf")
	if review.Results[0].State != "installed" {
		t.Fatalf("default setup did not install the review bundle: %#v", review)
	}
	ale86AssertInstalledBundle(t, filepath.Join(fx.home, ".codex", "skills", companion.Name), companion.Bundled())
	ale86AssertInstalledBundle(t, consolidationPath, named)

	unsupported := ale86Setup(t, fx, companion.ConsolidationName, true, 1, "pi")
	if unsupported.State != "refused" || unsupported.Results[0].State != "unsupported" {
		t.Fatalf("named setup broadened targets to unvalidated Pi: %#v", unsupported)
	}
}

func TestALE86NamedExecutableCheckRejectsReviewOnlyBuild(t *testing.T) {
	fx := ale48Fixture(t)
	bundleJSON, err := json.Marshal(companion.Bundled())
	if err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(fx.home, "bin", "skillglass")
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	oldBuild := "#!/bin/bash\nif [ \"$1\" = companion ]; then printf '%s\\n' " + shellQuote(string(bundleJSON)) + "; exit 0; fi\nif [ \"$1\" = help ]; then printf '%s\\n' '{\"schema_version\":1,\"help\":\"read-resource setup --check review prepare review select review apply\"}'; exit 0; fi\nexit 2\n"
	writeFile(t, link, []byte(oldBuild), 0o700)
	policy := "(allow file-read* (literal \"/bin/bash\"))\n(allow process-exec (literal " + sandboxString(link) + ") (literal \"/bin/bash\"))\n"

	before := treeManifest(t, fx.base)
	reviewCheck := ale48Run(t, fx, policy, filepath.Join(fx.home, "bin"), "setup", "--check", "--json")
	assertExit(t, reviewCheck, 0)
	var review struct {
		State string `json:"state"`
	}
	decodeJSON(t, reviewCheck.stdout, &review)
	if review.State != "compatible" {
		t.Fatalf("review-only executable stopped satisfying the default check: %s", reviewCheck.stdout)
	}
	consolidationCheck := ale48Run(t, fx, policy, filepath.Join(fx.home, "bin"), "setup", "--check", "--companion", companion.ConsolidationName, "--json")
	assertExit(t, consolidationCheck, 1)
	var named struct {
		State      string                `json:"state"`
		Executable setup.ExecutableCheck `json:"executable"`
	}
	decodeJSON(t, consolidationCheck.stdout, &named)
	if named.State != "incompatible" || named.Executable.State != "incompatible" {
		t.Fatalf("named check accepted a review-only executable: %s", consolidationCheck.stdout)
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
}

func ale86Setup(t *testing.T, fx fixture, bundleName string, install bool, want int, applications ...string) setup.SetupReport {
	t.Helper()
	args := []string{"setup", "--companion", bundleName, "--json"}
	for _, app := range applications {
		if app == "codeaf" {
			args = append(args, "--allow-shared-app", app)
		} else {
			args = append(args, "--app", app)
		}
	}
	if install {
		args = append(args, "--install")
	}
	result := ale48Run(t, fx, "", filepath.Join(fx.home, "bin"), args...)
	assertExit(t, result, want)
	if len(result.stderr) != 0 {
		t.Fatalf("named setup machine stderr: %s", result.stderr)
	}
	var report setup.SetupReport
	decodeJSON(t, result.stdout, &report)
	return report
}

func ale86AssertInstalledBundle(t *testing.T, path string, bundle companion.Bundle) {
	t.Helper()
	for _, file := range bundle.Files {
		actual := readBytes(t, filepath.Join(path, file.Path))
		if !bytes.Equal(actual, []byte(file.Content)) {
			t.Fatalf("installed companion differs at %s", file.Path)
		}
	}
}
