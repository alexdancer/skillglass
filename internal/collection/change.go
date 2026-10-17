package collection

import (
	"context"
	"sync"
)

type ControlScope string
type ControlChoice string

const (
	ControlGlobal  ControlScope  = "global"
	ControlProject ControlScope  = "project"
	ControlEnable  ControlChoice = "enable"
	ControlDisable ControlChoice = "disable"
	ControlInherit ControlChoice = "inherit"
)

type ChangeIntent struct {
	Source      SourceID      `json:"source_id"`
	Application string        `json:"application"`
	Scope       ControlScope  `json:"scope"`
	Choice      ControlChoice `json:"choice"`
}
type SettingChange struct {
	Setting string `json:"setting"`
	Before  string `json:"before"`
	After   string `json:"after"`
}
type ChangeView struct {
	State             string                  `json:"state"`
	Reason            string                  `json:"reason"`
	Intent            ChangeIntent            `json:"intent"`
	SourcePath        string                  `json:"source_path,omitempty"`
	ManifestPath      string                  `json:"manifest_path,omitempty"`
	ConfigPath        string                  `json:"config_path,omitempty"`
	InspectionProject ProjectContext          `json:"inspection_project"`
	Changes           []SettingChange         `json:"changes,omitempty"`
	KnownReferences   []Reference             `json:"known_references,omitempty"`
	Before            AvailabilityExplanation `json:"before"`
	Confirmation      string                  `json:"confirmation,omitempty"`
	NoWriteNeeded     bool                    `json:"no_write_needed"`
	CreateDirectory   string                  `json:"create_directory,omitempty"`
	Notes             []string                `json:"notes,omitempty"`
}
type ChangeOutcome struct {
	State               string                   `json:"state"`
	Reason              string                   `json:"reason"`
	Intent              ChangeIntent             `json:"intent"`
	WriteState          string                   `json:"write_state"`
	Persistence         string                   `json:"persistence"`
	Achievement         string                   `json:"achievement"`
	PersistedChoice     ControlChoice            `json:"persisted_choice,omitempty"`
	PersistedPermission PermissionState          `json:"persisted_permission,omitempty"`
	Effective           *AvailabilityExplanation `json:"effective,omitempty"`
	Reload              SessionExplanation       `json:"reload"`
	Artifacts           []string                 `json:"artifacts,omitempty"`
	Snapshot            *Snapshot                `json:"-"`
}

func (out ChangeOutcome) ExitCode() int {
	if out.State == "applied" && out.Persistence == "verified" && out.Achievement == "achieved" {
		return 0
	}
	return 1
}

type ChangePreview struct {
	view ChangeView
	plan *changePlan
	once sync.Mutex
	used bool
}

func (p *ChangePreview) View() ChangeView {
	if p == nil {
		return ChangeView{State: "refused", Reason: "no preview"}
	}
	return cloneChangeView(p.view)
}
func cloneChangeView(original ChangeView) ChangeView {
	view := original
	view.Changes = append([]SettingChange(nil), original.Changes...)
	view.KnownReferences = append([]Reference(nil), original.KnownReferences...)
	for index := range view.KnownReferences {
		view.KnownReferences[index] = cloneReference(view.KnownReferences[index])
	}
	view.Before = cloneAvailability(original.Before)
	view.Notes = append([]string(nil), original.Notes...)
	return view
}
func (s *Snapshot) PreviewChange(ctx context.Context, intent ChangeIntent) *ChangePreview {
	if capability, known := applicationCapability(intent.Application); known && capability.preview != nil {
		return capability.preview(ctx, s, intent)
	}
	return rejectedChange(intent, "unsupported", "unknown or unsupported application")
}
func (p *ChangePreview) Apply(ctx context.Context, confirmation string) ChangeOutcome {
	out := ChangeOutcome{State: "refused", Reason: "confirmation does not match this current preview; request a new preview", WriteState: "none", Persistence: "not-written", Achievement: "unmet", Reload: SessionExplanation{State: "unknown", Evidence: Evidence{Kind: EvidenceUnknown, Rule: "session-not-observed", Reason: "existing session adoption was not verified; a fresh launch is a conservative way to load any saved settings"}}}
	if p == nil {
		return out
	}
	p.once.Lock()
	defer p.once.Unlock()
	out.Intent = p.view.Intent
	if p.plan == nil || p.used || confirmation == "" || confirmation != p.plan.digest {
		return out
	}
	p.used = true
	return p.plan.apply(ctx)
}
