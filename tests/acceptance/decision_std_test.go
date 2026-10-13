package acceptance_test

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestReviewALE29NormalFixtureDeniesOutsideSystemModel(t *testing.T) {
	fx := newFixture(t)
	allowed := filepath.Join(fx.base, "allowed", "policy-control")
	outside := filepath.Join(fx.base, "excluded", "modeled-system-settings.json")
	writeFile(t, allowed, []byte("ALLOWED_READ_SENTINEL\n"), 0600)
	writeFile(t, outside, []byte("inert-system-settings-canary\n"), 0600)
	original := skillglassBinary
	skillglassBinary = isolationProbeBinary
	defer func() { skillglassBinary = original }()
	result := fx.run(t, allowed, outside, filepath.Join(fx.temporary, "probe-write"), "127.0.0.1:9")
	var probe struct {
		Checks map[string]bool   `json:"checks"`
		Errors map[string]string `json:"errors"`
	}
	decodeJSON(t, result.stdout, &probe)
	data, _ := json.MarshalIndent(probe, "", "  ")
	t.Logf("Unchanged ordinary CLI harness probe, exit %d: %s", result.code, data)
	if !probe.Checks["allowed_read"] {
		t.Fatal("positive fixture read failed")
	}
	if !probe.Checks["excluded_read_denied"] {
		t.Fatal("normal public CLI fixture route permitted the modeled outside system-settings read")
	}
}
