package collection

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path/filepath"
	"sort"
	"strings"
)

type ManagerState string

const (
	ManagerBoundary  ManagerState = "managed-boundary"
	ManagerObserved  ManagerState = "observed"
	ManagerAbsent    ManagerState = "absent"
	ManagerUncertain ManagerState = "uncertain"
)

type ManagerEvidence struct {
	Manager  string       `json:"manager"`
	Boundary string       `json:"boundary,omitempty"`
	State    ManagerState `json:"state"`
	SHA256   string       `json:"input_sha256,omitempty"`
	Evidence Evidence     `json:"evidence"`
}

func managerInputDigest(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func pluginRegistryManagers(path string, raw []byte, err error, version int, evidence Evidence) []ManagerEvidence {
	input := ManagerEvidence{Manager: "plugin-installations", State: ManagerObserved, Evidence: evidence}
	if nativeAbsent(path, err) {
		input.State = ManagerAbsent
		return []ManagerEvidence{input}
	}
	uncertain := func(reason string) ManagerEvidence {
		value := input
		value.State = ManagerUncertain
		value.Evidence.Kind = EvidenceUnknown
		value.Evidence.Reason = reason
		return value
	}
	var document map[string]json.RawMessage
	if err != nil || len(raw) > 1<<20 || json.Unmarshal(raw, &document) != nil || document == nil {
		return []ManagerEvidence{uncertain("installation registry is unreadable, oversized or invalid; external installation boundaries cannot be established")}
	}
	for field := range document {
		if field != "version" && field != "plugins" {
			return []ManagerEvidence{uncertain("installation registry schema is unsupported; external installation boundaries cannot be established")}
		}
	}
	if value, present := document["version"]; present {
		var actual int
		if json.Unmarshal(value, &actual) != nil || (actual != 1 && actual != 2) || (version != 0 && actual != version) {
			return []ManagerEvidence{uncertain("installation registry version is unsupported; external installation boundaries cannot be established")}
		}
	} else if version != 0 {
		return []ManagerEvidence{uncertain("installation registry version is missing; external installation boundaries cannot be established")}
	}
	var plugins map[string]json.RawMessage
	if json.Unmarshal(document["plugins"], &plugins) != nil || plugins == nil {
		return []ManagerEvidence{uncertain("installation registry plugins must be an object; external installation boundaries cannot be established")}
	}
	input.SHA256 = managerInputDigest(raw)
	out := []ManagerEvidence{input}
	ids := make([]string, 0, len(plugins))
	for id := range plugins {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		value := json.RawMessage(strings.TrimSpace(string(plugins[id])))
		var records []json.RawMessage
		if len(value) > 0 && value[0] == '{' {
			records = []json.RawMessage{value}
		} else if json.Unmarshal(value, &records) != nil || records == nil {
			out = append(out, uncertain("installation records cannot be interpreted: "+id))
			continue
		}
		for _, record := range records {
			var fields map[string]json.RawMessage
			var install *string
			if json.Unmarshal(record, &fields) != nil || fields == nil || json.Unmarshal(fields["installPath"], &install) != nil || install == nil || !filepath.IsAbs(strings.TrimSpace(*install)) {
				out = append(out, uncertain("installation path cannot be established: "+id))
				continue
			}
			out = append(out, ManagerEvidence{Manager: id, Boundary: strings.TrimSpace(*install), State: ManagerBoundary, Evidence: evidence})
		}
	}
	return out
}
