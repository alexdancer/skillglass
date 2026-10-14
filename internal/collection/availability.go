package collection

import (
	"encoding/json"
	"fmt"
)

type EvidenceKind string

const (
	EvidenceObserved EvidenceKind = "observed"
	EvidenceInferred EvidenceKind = "inferred"
	EvidenceUnknown  EvidenceKind = "unknown"
)

type AvailabilityState string

const (
	AvailabilityAvailable    AvailabilityState = "available"
	AvailabilityDisabled     AvailabilityState = "disabled"
	AvailabilityExplicitOnly AvailabilityState = "explicit-only"
	AvailabilityAsk          AvailabilityState = "ask"
	AvailabilityShadowed     AvailabilityState = "shadowed"
	AvailabilityUnavailable  AvailabilityState = "unavailable"
	AvailabilityUnknown      AvailabilityState = "unknown"
)

func AvailabilityStates() []AvailabilityState {
	return []AvailabilityState{AvailabilityAvailable, AvailabilityDisabled, AvailabilityExplicitOnly, AvailabilityAsk, AvailabilityShadowed, AvailabilityUnavailable, AvailabilityUnknown}
}
func ParseAvailabilityState(raw string) (AvailabilityState, error) {
	if raw == "" || raw == "all" {
		return "", nil
	}
	for _, state := range AvailabilityStates() {
		if raw == string(state) {
			return state, nil
		}
	}
	return "", fmt.Errorf("unknown availability %q; use all, available, disabled, explicit-only, ask, shadowed, unavailable, or unknown", raw)
}

type PermissionState string

const (
	PermissionAllowed PermissionState = "allowed"
	PermissionAsk     PermissionState = "ask"
	PermissionDenied  PermissionState = "denied"
	PermissionUnknown PermissionState = "unknown"
)

type ParticipationState string

const (
	ParticipationEligible   ParticipationState = "eligible"
	ParticipationExcluded   ParticipationState = "excluded"
	ParticipationInspection ParticipationState = "inspection-only"
	ParticipationUnknown    ParticipationState = "unknown"
)

type PrecedenceState string

const (
	PrecedenceUnique     PrecedenceState = "unique"
	PrecedenceCoexisting PrecedenceState = "coexisting"
	PrecedenceUnknown    PrecedenceState = "unknown"
)

type Evidence struct {
	Kind         EvidenceKind `json:"kind"`
	Rule         string       `json:"rule"`
	InputPath    string       `json:"input_path,omitempty"`
	Setting      string       `json:"setting,omitempty"`
	AgentVersion string       `json:"agent_version,omitempty"`
	Reason       string       `json:"reason"`
}
type PermissionChoice struct {
	Path    string `json:"path"`
	Enabled bool   `json:"enabled"`
}
type PermissionLayer struct {
	Evidence Evidence           `json:"evidence"`
	Path     string             `json:"path"`
	Scope    string             `json:"scope"`
	State    string             `json:"state"`
	Reason   string             `json:"reason"`
	Choices  []PermissionChoice `json:"choices,omitempty"`
}
type PermissionExplanation struct {
	State    PermissionState   `json:"state"`
	Evidence Evidence          `json:"evidence"`
	Layers   []PermissionLayer `json:"layers,omitempty"`
}
type PluginRecord struct {
	Kind              string          `json:"kind"`
	InputPath         string          `json:"input_path"`
	Path              string          `json:"path"`
	Scope             string          `json:"scope,omitempty"`
	ProjectPath       json.RawMessage `json:"project_path,omitempty"`
	Version           json.RawMessage `json:"version,omitempty"`
	Applicability     string          `json:"applicability,omitempty"`
	RegistrationInput string          `json:"registration_input,omitempty"`
	LastUpdated       json.RawMessage `json:"last_updated,omitempty"`
	DeclaredSource    string          `json:"declared_source,omitempty"`
}
type Provenance struct {
	Kind       string         `json:"kind"`
	PluginID   string         `json:"plugin_id,omitempty"`
	Version    string         `json:"version,omitempty"`
	Evidence   Evidence       `json:"evidence"`
	Records    []PluginRecord `json:"records,omitempty"`
	Setting    *Evidence      `json:"setting,omitempty"`
	Activation *Evidence      `json:"activation,omitempty"`
}
type ReferenceAvailability struct {
	NativeName    string                `json:"native_name,omitempty"`
	Reference     ReferenceID           `json:"reference_id"`
	Participation ParticipationState    `json:"participation"`
	Evidence      Evidence              `json:"evidence"`
	Permission    PermissionExplanation `json:"permission"`
	Provenance    Provenance            `json:"provenance"`
}
type PrecedenceExplanation struct {
	State          PrecedenceState `json:"state"`
	Evidence       Evidence        `json:"evidence"`
	RelatedSources []SourceID      `json:"related_sources,omitempty"`
	Winner         SourceID        `json:"winner,omitempty"`
}
type SessionExplanation struct {
	State    string   `json:"state"`
	Evidence Evidence `json:"evidence"`
}
type Capability struct {
	Operation string   `json:"operation"`
	Scope     string   `json:"scope"`
	State     string   `json:"state"`
	Evidence  Evidence `json:"evidence"`
}
type AvailabilityExplanation struct {
	NativeNameEvidence *Evidence               `json:"native_name_evidence,omitempty"`
	NativeName         string                  `json:"native_name,omitempty"`
	State              AvailabilityState       `json:"state"`
	Reason             string                  `json:"reason"`
	Evidence           *Evidence               `json:"evidence,omitempty"`
	References         []ReferenceAvailability `json:"references,omitempty"`
	Precedence         *PrecedenceExplanation  `json:"precedence,omitempty"`
	Invocation         *SessionExplanation     `json:"invocation,omitempty"`
	Session            *SessionExplanation     `json:"session,omitempty"`
	Reload             *SessionExplanation     `json:"reload,omitempty"`
	Capabilities       []Capability            `json:"capabilities,omitempty"`
}
type AgentScope struct {
	Evidence         Evidence          `json:"evidence"`
	Application      string            `json:"application"`
	ValidatedVersion string            `json:"validated_version"`
	VersionEvidence  string            `json:"version_evidence"`
	ConfigLayers     []PermissionLayer `json:"config_layers"`
	Gaps             []string          `json:"gaps"`
}

func cloneLayers(layers []PermissionLayer) []PermissionLayer {
	result := append([]PermissionLayer(nil), layers...)
	for i := range result {
		result[i].Choices = append([]PermissionChoice(nil), result[i].Choices...)
	}
	return result
}
func cloneAvailability(value AvailabilityExplanation) AvailabilityExplanation {
	if value.NativeNameEvidence != nil {
		copy := *value.NativeNameEvidence
		value.NativeNameEvidence = &copy
	}
	if value.Evidence != nil {
		copy := *value.Evidence
		value.Evidence = &copy
	}
	value.References = append([]ReferenceAvailability(nil), value.References...)
	for i := range value.References {
		value.References[i].Permission.Layers = cloneLayers(value.References[i].Permission.Layers)
		provenance := &value.References[i].Provenance
		provenance.Records = append([]PluginRecord(nil), provenance.Records...)
		for j := range provenance.Records {
			provenance.Records[j].ProjectPath = append(json.RawMessage(nil), provenance.Records[j].ProjectPath...)
			provenance.Records[j].Version = append(json.RawMessage(nil), provenance.Records[j].Version...)
			provenance.Records[j].LastUpdated = append(json.RawMessage(nil), provenance.Records[j].LastUpdated...)
		}
		if provenance.Setting != nil {
			copy := *provenance.Setting
			provenance.Setting = &copy
		}
		if provenance.Activation != nil {
			copy := *provenance.Activation
			provenance.Activation = &copy
		}
	}
	if value.Precedence != nil {
		copy := *value.Precedence
		copy.RelatedSources = append([]SourceID(nil), copy.RelatedSources...)
		value.Precedence = &copy
	}
	if value.Invocation != nil {
		copy := *value.Invocation
		value.Invocation = &copy
	}
	if value.Session != nil {
		copy := *value.Session
		value.Session = &copy
	}
	if value.Reload != nil {
		copy := *value.Reload
		value.Reload = &copy
	}
	value.Capabilities = append([]Capability(nil), value.Capabilities...)
	return value
}
