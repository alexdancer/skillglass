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
	Manager             string       `json:"manager"`
	Reader              string       `json:"reader,omitempty"`
	Boundary            string       `json:"boundary,omitempty"`
	UncertainBoundaries []string     `json:"uncertain_boundaries,omitempty"`
	State               ManagerState `json:"state"`
	SHA256              string       `json:"input_sha256,omitempty"`
	Evidence            Evidence     `json:"evidence"`
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

// managerReader identifies an ownership-bearing input, independently of its
// recovered boundaries. A parser starts uncertain and must finalize its result.
type managerReader struct{ Reader, Path, Boundary string }

type managerRead struct {
	result ManagerEvidence
}

func startManagerRead(required *[]managerReader, reader, manager, path, boundary string, evidence Evidence) managerRead {
	evidence.Kind = EvidenceUnknown
	evidence.Reason = "ownership input has not been completely interpreted"
	*required = append(*required, managerReader{reader, path, boundary})
	return managerRead{result: ManagerEvidence{Reader: reader, Manager: manager, Boundary: boundary, State: ManagerUncertain, Evidence: evidence}}
}
func (r *managerRead) absent() {
	r.result.State = ManagerAbsent
	r.result.SHA256 = ""
	r.result.Evidence.Kind = EvidenceObserved
	r.result.Evidence.Reason = "optional ownership input is absent"
}
func (r *managerRead) complete(raw []byte) {
	r.result.State = ManagerObserved
	r.result.SHA256 = managerInputDigest(raw)
	r.result.Evidence.Kind = EvidenceObserved
	r.result.Evidence.Reason = "ownership-bearing records completely interpreted"
}
func (r *managerRead) uncertain(reason string) {
	r.result.State = ManagerUncertain
	r.result.Evidence.Kind = EvidenceUnknown
	r.result.Evidence.Reason = reason
}
func (r *managerRead) finish(facts *[]ManagerEvidence) {
	*facts = append(*facts, r.result)
}
func missingManagerReaders(required []managerReader, facts []ManagerEvidence) []ManagerEvidence {
	var missing []ManagerEvidence
	for _, reader := range required {
		found := false
		for _, fact := range facts {
			if fact.Reader == reader.Reader && fact.Evidence.InputPath == reader.Path && (fact.State == ManagerObserved || fact.State == ManagerAbsent || fact.State == ManagerUncertain) {
				found = true
				break
			}
		}
		if !found {
			missing = append(missing, ManagerEvidence{Reader: reader.Reader, Manager: "ownership-coverage", Boundary: reader.Boundary, State: ManagerUncertain, Evidence: Evidence{Kind: EvidenceUnknown, Rule: "required-manager-reader", InputPath: reader.Path, Reason: "required ownership reader has no terminal coverage result"}})
		}
	}
	return missing
}
func requiredManagerReaders(request Request) []managerReader {
	switch request.Agent {
	case "pi":
		base, err := piAgentDir(request.home)
		if err != nil {
			return []managerReader{{"pi-settings", request.home, ""}}
		}
		result := []managerReader{{"pi-settings", filepath.Join(base, "settings.json"), ""}}
		if request.Project.Selected {
			result = append(result, managerReader{"pi-settings", filepath.Join(request.Project.Path, ".pi", "settings.json"), ""})
		}
		return result
	case "claude-code":
		return []managerReader{{"claude-marketplaces", filepath.Join(request.claudeHome, "plugins", "known_marketplaces.json"), ""}}
	}
	return nil
}
