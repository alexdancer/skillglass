package collection

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/alexdancer/skillglass/internal/companion"
)

const ReviewContractVersion = companion.ContractVersion

type ReviewContext struct {
	Applications []string     `json:"applications"`
	Scope        ControlScope `json:"scope"`
	Project      string       `json:"project,omitempty"`
	Goal         string       `json:"goal"`
	KeepEnabled  []SourceID   `json:"keep_enabled"`
}
type ReviewAssessment struct {
	Source          SourceID `json:"source_id"`
	Assessment      string   `json:"assessment"`
	SupportingPaths []string `json:"supporting_paths"`
	Limitations     []string `json:"limitations"`
}
type ReviewRecommendation struct {
	ID          string        `json:"id"`
	Source      SourceID      `json:"source_id"`
	Application string        `json:"application"`
	Action      ControlChoice `json:"action"`
	Reason      string        `json:"reason"`
}
type ReviewUnchanged struct {
	Source      SourceID `json:"source_id"`
	Application string   `json:"application"`
	Reason      string   `json:"reason"`
}
type ReviewDraft struct {
	SchemaVersion int                    `json:"schema_version"`
	Context       ReviewContext          `json:"context"`
	Sources       []ReviewAssessment     `json:"sources"`
	Changes       []ReviewRecommendation `json:"changes"`
	Unchanged     []ReviewUnchanged      `json:"unchanged"`
	Unresolved    []string               `json:"unresolved"`
}
type ReviewDiscovery struct {
	Application string `json:"application"`
	View        View   `json:"view"`
}
type ReviewEvidence struct {
	Path           string               `json:"path"`
	Coverage       Completeness         `json:"coverage"`
	Representation string               `json:"representation"`
	SHA256         string               `json:"sha256,omitempty"`
	Diagnostics    []ResourceDiagnostic `json:"diagnostics"`
}
type ReviewSource struct {
	Source       Source           `json:"source"`
	Applications []string         `json:"applications"`
	Assessment   string           `json:"assessment"`
	Evidence     []ReviewEvidence `json:"evidence"`
	Limitations  []string         `json:"limitations"`
}
type ReviewPair struct {
	Source      SourceID `json:"source_id"`
	Application string   `json:"application"`
}

// ReviewEffect is prospective persisted configuration, not running-session use.
// Baseline remains the original native preview; Changes reflect preceding effects.
type ReviewEffect struct {
	ID              string          `json:"id"`
	Intent          ChangeIntent    `json:"intent"`
	ConfigPath      string          `json:"config_path,omitempty"`
	Changes         []SettingChange `json:"changes"`
	BeforeSHA256    string          `json:"before_sha256"`
	AfterSHA256     string          `json:"after_sha256"`
	BeforeExists    bool            `json:"before_exists"`
	AfterExists     bool            `json:"after_exists"`
	NoWriteNeeded   bool            `json:"no_write_needed"`
	CreateDirectory string          `json:"create_directory,omitempty"`
}
type ReviewChange struct {
	Recommendation ReviewRecommendation `json:"recommendation"`
	Baseline       ChangeView           `json:"baseline"`
	ObservationID  string               `json:"observation_id,omitempty"`
	Consumers      []GroupConsumer      `json:"consumers"`
}
type ReviewCoverage struct {
	Discovery        Completeness `json:"discovery"`
	SelectedEvidence Completeness `json:"selected_evidence"`
	Unreviewed       []ReviewPair `json:"unreviewed"`
}
type ReviewProposal struct {
	SchemaVersion    int               `json:"schema_version"`
	Kind             string            `json:"kind"`
	CompanionVersion string            `json:"companion_version"`
	ProposalID       string            `json:"proposal_id"`
	State            string            `json:"state"`
	Draft            ReviewDraft       `json:"draft"`
	Discovery        []ReviewDiscovery `json:"discovery"`
	Sources          []ReviewSource    `json:"sources"`
	Changes          []ReviewChange    `json:"changes"`
	Effects          []ReviewEffect    `json:"effects"`
	Coverage         ReviewCoverage    `json:"coverage"`
	Limitations      []string          `json:"limitations"`
}
type ReviewSelection struct {
	SchemaVersion int            `json:"schema_version"`
	Kind          string         `json:"kind"`
	SelectionID   string         `json:"selection_id"`
	State         string         `json:"state"`
	Proposal      ReviewProposal `json:"proposal"`
	Chosen        []string       `json:"chosen"`
	Omitted       []string       `json:"omitted"`
	Effects       []ReviewEffect `json:"effects"`
}

type preparedReview struct {
	proposal ReviewProposal
	plans    map[string]*ChangePreview
}

// PrepareReview observes exact evidence and native controls. It never calls Apply.
func PrepareReview(ctx context.Context, draft ReviewDraft) (ReviewProposal, error) {
	prepared, err := prepareReview(ctx, draft)
	return prepared.proposal, err
}
func prepareReview(ctx context.Context, draft ReviewDraft) (preparedReview, error) {
	result := preparedReview{plans: map[string]*ChangePreview{}}
	fail := func(err error) (preparedReview, error) { return result, err }
	if err := validateReviewDraft(draft); err != nil {
		return fail(err)
	}
	if draft.Context.Scope == ControlProject {
		draft.Context.Project = filepath.Clean(draft.Context.Project)
	}
	p := ReviewProposal{SchemaVersion: ReviewContractVersion, Kind: "skillglass-review-proposal", CompanionVersion: companion.Version,
		State: "prepared", Draft: draft, Discovery: []ReviewDiscovery{}, Sources: []ReviewSource{}, Changes: []ReviewChange{}, Effects: []ReviewEffect{},
		Coverage:    ReviewCoverage{Discovery: Complete, SelectedEvidence: Complete, Unreviewed: []ReviewPair{}},
		Limitations: []string{"Native discovery is bounded by reported roots and gaps, not machine-wide.", "Evidence reads verify observations, not the agent's assessment or actual usage.", "Prospective effects are not persisted; running-session adoption is unobserved.", "Preparation and selection write no source content or settings. Collection execution is not included in this build."}}
	snapshots := map[string]*Snapshot{}
	sources := map[SourceID]Source{}
	sourceApps := map[SourceID][]string{}
	owner := map[SourceID]*Snapshot{}
	pairs := []ReviewPair{}
	for _, app := range draft.Context.Applications {
		var project *string
		if draft.Context.Scope == ControlProject {
			project = &draft.Context.Project
		}
		request, err := NewRequestWithAgent(nil, nil, project, app)
		if err != nil {
			return fail(err)
		}
		snapshot, err := Build(ctx, request)
		if err != nil {
			return fail(fmt.Errorf("discover %s: %w", app, err))
		}
		snapshots[app] = snapshot
		view := snapshot.Query(Filters{Context: FilterAll}, QuerySources)
		p.Discovery = append(p.Discovery, ReviewDiscovery{app, view})
		if !snapshot.Complete() {
			p.Coverage.Discovery = Incomplete
		}
		for _, source := range view.Sources {
			if _, exists := sources[source.ID]; !exists {
				sources[source.ID], owner[source.ID] = source, snapshot
			}
			sourceApps[source.ID] = append(sourceApps[source.ID], app)
			pairs = append(pairs, ReviewPair{source.ID, app})
		}
	}
	for _, id := range draft.Context.KeepEnabled {
		if _, ok := sources[id]; !ok {
			return fail(fmt.Errorf("keep_enabled source %s is absent from selected applications", id))
		}
	}
	assessed := map[SourceID]bool{}
	for _, assessment := range draft.Sources {
		source, ok := sources[assessment.Source]
		if !ok {
			return fail(fmt.Errorf("reviewed source %s is absent from selected applications", assessment.Source))
		}
		assessed[source.ID] = true
		review := ReviewSource{Source: source, Applications: sourceApps[source.ID], Assessment: assessment.Assessment, Evidence: []ReviewEvidence{}, Limitations: append([]string{}, assessment.Limitations...)}
		instructions := ReviewEvidence{Path: source.Manifest.Path, Coverage: Complete, Representation: "utf-8", Diagnostics: []ResourceDiagnostic{}}
		if source.Manifest.Status == ManifestUnreadable {
			instructions.Coverage, instructions.Representation = Incomplete, "none"
		} else if source.Manifest.RawBytesBase64 != "" {
			instructions.Representation = "base64"
		}
		if instructions.Coverage == Complete {
			raw := []byte(source.Manifest.RawText)
			if source.Manifest.RawBytesBase64 != "" {
				raw, _ = base64.StdEncoding.DecodeString(source.Manifest.RawBytesBase64)
			}
			instructions.SHA256 = bytesHash(raw)
		} else {
			review.Limitations = append(review.Limitations, "Selected instructions could not be read.")
		}
		review.Evidence = append(review.Evidence, instructions)
		for _, path := range assessment.SupportingPaths {
			read := owner[source.ID].ReadResource(ctx, source.ID, path)
			evidence := ReviewEvidence{Path: path, Coverage: read.Coverage, Representation: read.Representation, Diagnostics: read.Diagnostics}
			if read.Coverage == Complete {
				raw := []byte{}
				if read.Text != nil {
					raw = []byte(*read.Text)
				} else {
					raw, _ = base64.StdEncoding.DecodeString(read.BytesBase64)
				}
				evidence.SHA256 = bytesHash(raw)
			}
			review.Evidence = append(review.Evidence, evidence)
		}
		for _, evidence := range review.Evidence {
			if evidence.Coverage != Complete {
				p.Coverage.SelectedEvidence = Incomplete
			}
			if evidence.Representation == "base64" {
				review.Limitations = append(review.Limitations, evidence.Path+": binary bytes observed; meaning is not verified.")
			}
		}
		p.Sources = append(p.Sources, review)
	}
	accounted := map[ReviewPair]bool{}
	for _, recommendation := range draft.Changes {
		snapshot := snapshots[recommendation.Application]
		source, ok := snapshot.Source(recommendation.Source)
		if !ok {
			return fail(fmt.Errorf("change %s source is not in selected application %s", recommendation.ID, recommendation.Application))
		}
		if !assessed[source.ID] {
			return fail(fmt.Errorf("change %s requires a source assessment", recommendation.ID))
		}
		intent := ChangeIntent{Source: source.ID, Application: recommendation.Application, Scope: draft.Context.Scope, Choice: recommendation.Action}
		// Group discovery supplies the same bounded shared-consumer context as direct controls.
		group := snapshot.PreviewGroupChange(ctx, GroupIntent{Source: source.ID, Applications: []string{recommendation.Application}, Scope: intent.Scope, Choice: intent.Choice})
		preview := snapshot.PreviewChange(ctx, intent)
		baseline := preview.View()
		baseline.Confirmation = ""
		change := ReviewChange{Recommendation: recommendation, Baseline: baseline, Consumers: group.View().Consumers}
		if preview.plan == nil || baseline.State != "ready" || group.View().State != "ready" || source.Manifest.Status == ManifestUnreadable {
			p.State = "blocked"
		} else {
			change.ObservationID = reviewHash(preview.plan.digest)
			result.plans[recommendation.ID] = preview
		}
		p.Changes = append(p.Changes, change)
		accounted[ReviewPair{source.ID, recommendation.Application}] = true
	}
	for _, unchanged := range draft.Unchanged {
		if _, ok := snapshots[unchanged.Application].Source(unchanged.Source); !ok {
			return fail(fmt.Errorf("unchanged source %s is absent from %s", unchanged.Source, unchanged.Application))
		}
		if !assessed[unchanged.Source] {
			return fail(fmt.Errorf("unchanged source %s requires an assessment", unchanged.Source))
		}
		accounted[ReviewPair{unchanged.Source, unchanged.Application}] = true
	}
	for _, pair := range pairs {
		if !accounted[pair] {
			p.Coverage.Unreviewed = append(p.Coverage.Unreviewed, pair)
		}
	}
	if p.State == "prepared" {
		var err error
		p.Effects, err = reviewEffects(draft.Changes, result.plans)
		if err != nil {
			return fail(err)
		}
	}
	p.ProposalID = proposalID(p)
	result.proposal = p
	return result, nil
}

func validateReviewDraft(d ReviewDraft) error {
	if d.SchemaVersion != ReviewContractVersion {
		return errors.New("unsupported review schema_version; use companion contract 1")
	}
	c := d.Context
	if len(c.Applications) == 0 || c.KeepEnabled == nil || d.Sources == nil || d.Changes == nil || d.Unchanged == nil || d.Unresolved == nil {
		return errors.New("explicit applications, keep_enabled, sources, changes, unchanged and unresolved arrays are required (empty arrays are allowed)")
	}
	if strings.TrimSpace(c.Goal) == "" {
		return errors.New("state the user's review goal")
	}
	if c.Scope != ControlGlobal && c.Scope != ControlProject {
		return errors.New("select global or one project scope explicitly")
	}
	if c.Scope == ControlGlobal && c.Project != "" || c.Scope == ControlProject && (!filepath.IsAbs(c.Project) || filepath.Clean(c.Project) != c.Project) {
		return errors.New("global scope omits project; project scope requires one normalized absolute project path")
	}
	apps := map[string]bool{}
	for _, app := range c.Applications {
		if _, ok := applicationCapability(app); !ok || apps[app] {
			return fmt.Errorf("unknown or repeated application %q", app)
		}
		apps[app] = true
	}
	keep := map[SourceID]bool{}
	for _, id := range c.KeepEnabled {
		if id == "" || keep[id] {
			return errors.New("keep_enabled requires distinct exact source IDs")
		}
		keep[id] = true
	}
	assessed := map[SourceID]bool{}
	for _, s := range d.Sources {
		if s.Source == "" || assessed[s.Source] || strings.TrimSpace(s.Assessment) == "" || s.SupportingPaths == nil || s.Limitations == nil {
			return errors.New("each assessed source needs a distinct exact source_id, assessment, supporting_paths and limitations arrays")
		}
		assessed[s.Source] = true
		paths := map[string]bool{}
		for _, path := range s.SupportingPaths {
			if path == "" || paths[path] {
				return errors.New("supporting_paths must be nonempty and distinct")
			}
			paths[path] = true
		}
	}
	ids := map[string]bool{}
	pairs := map[ReviewPair]bool{}
	for _, change := range d.Changes {
		pair := ReviewPair{change.Source, change.Application}
		if strings.TrimSpace(change.ID) == "" || ids[change.ID] || change.Source == "" || !apps[change.Application] || strings.TrimSpace(change.Reason) == "" {
			return errors.New("each change needs a unique nonempty id, exact source_id, selected application and reason")
		}
		if pairs[pair] {
			return errors.New("repeated or contradictory source/application changes are ambiguous")
		}
		if change.Action != ControlEnable && change.Action != ControlDisable {
			return errors.New("review actions are enable or disable only")
		}
		if change.Action == ControlDisable && keep[change.Source] {
			return errors.New("cannot disable a keep_enabled source")
		}
		ids[change.ID], pairs[pair] = true, true
	}
	for _, u := range d.Unchanged {
		pair := ReviewPair{u.Source, u.Application}
		if u.Source == "" || !apps[u.Application] || strings.TrimSpace(u.Reason) == "" || pairs[pair] {
			return errors.New("unchanged entries need an exact source, selected application, reason and no contradictory/repeated action")
		}
		pairs[pair] = true
	}
	for _, q := range d.Unresolved {
		if strings.TrimSpace(q) == "" {
			return errors.New("unresolved questions must be nonempty")
		}
	}
	return nil
}

// reviewEffects composes only already validated native recipes, in memory.
// Aliased/overlapping destinations across applications refuse instead of guessing.
func reviewEffects(recommendations []ReviewRecommendation, plans map[string]*ChangePreview) ([]ReviewEffect, error) {
	effects := []ReviewEffect{}
	for i, r := range recommendations {
		for _, other := range recommendations[i+1:] {
			if r.Application != other.Application {
				if reason := groupInterference([]*ChangePreview{plans[r.ID], plans[other.ID]}); reason != "" {
					return nil, errors.New(reason)
				}
			}
		}
	}
	type state struct {
		raw      []byte
		exists   bool
		app      string
		location groupLocation
	}
	states := map[string]state{}
	for _, r := range recommendations {
		preview := plans[r.ID]
		if preview == nil || preview.plan == nil || preview.plan.reviewPatch == nil {
			return nil, fmt.Errorf("change %s has no supported native recipe", r.ID)
		}
		plan := preview.plan
		path := plan.destination.Path
		current, ok := states[path]
		if !ok {
			location, err := groupLocationAt(path)
			if err != nil {
				return nil, fmt.Errorf("cannot establish destination identity: %w", err)
			}
			for otherPath, other := range states {
				if path != otherPath && (location.within(other.location) || other.location.within(location)) || other.app != r.Application && (location.within(other.location) || other.location.within(location)) {
					return nil, errors.New("review destinations alias or overlap; revise the explicit proposal")
				}
			}
			current = state{append([]byte(nil), plan.destination.Config.Bytes...), plan.destination.Config.Exists, r.Application, location}
		} else if current.app != r.Application {
			return nil, errors.New("different applications share a review destination; revise the explicit proposal")
		}
		post, changes, write, err := plan.reviewPatch(current.raw, current.exists)
		if err != nil {
			return nil, fmt.Errorf("ordered effect %s: %w", r.ID, err)
		}
		effect := ReviewEffect{ID: r.ID, Intent: plan.intent, ConfigPath: path, Changes: changes, BeforeSHA256: bytesHash(current.raw), AfterSHA256: bytesHash(post), BeforeExists: current.exists, AfterExists: current.exists || write, NoWriteNeeded: !write}
		if write && !current.exists {
			effect.CreateDirectory = plan.destination.CreateDirectory
		}
		effects = append(effects, effect)
		current.raw, current.exists = post, effect.AfterExists
		states[path] = current
	}
	return effects, nil
}

func bytesHash(raw []byte) string { sum := sha256.Sum256(raw); return hex.EncodeToString(sum[:]) }
func reviewHash(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return bytesHash(raw)
}
func proposalID(p ReviewProposal) string { p.ProposalID = ""; return "rp1:" + reviewHash(p) }
func selectionID(s ReviewSelection) string {
	s.SelectionID = ""
	s.State = "preview"
	return "rs1:" + reviewHash(s)
}

// ValidateReviewProposal verifies artifact integrity, not live freshness or consent.
func ValidateReviewProposal(p ReviewProposal) error {
	if p.SchemaVersion != ReviewContractVersion || p.Kind != "skillglass-review-proposal" || p.CompanionVersion != companion.Version {
		return errors.New("incompatible review proposal")
	}
	if p.ProposalID == "" || p.ProposalID != proposalID(p) {
		return errors.New("proposal was altered; reprepare it and renew approval")
	}
	return validateReviewDraft(p.Draft)
}

// SelectReview rechecks original observations, then previews only chosen effects.
// Omitted recommendations never acquire approval, even when the subset is empty.
func SelectReview(ctx context.Context, proposal ReviewProposal, chosen []string) (ReviewSelection, error) {
	if err := ValidateReviewProposal(proposal); err != nil {
		return ReviewSelection{}, err
	}
	if proposal.State != "prepared" {
		return ReviewSelection{}, errors.New("proposal has blocked recommendations; explicitly revise and reprepare before selection")
	}
	fresh, err := prepareReview(ctx, proposal.Draft)
	if err != nil {
		return ReviewSelection{}, err
	}
	if fresh.proposal.ProposalID != proposal.ProposalID {
		return ReviewSelection{}, errors.New("proposal is stale; source, evidence, scope, discovery or native effects changed; reprepare and renew approval")
	}
	selected := map[string]bool{}
	for _, id := range chosen {
		if selected[id] {
			return ReviewSelection{}, fmt.Errorf("repeated chosen change %q", id)
		}
		selected[id] = true
	}
	s := ReviewSelection{SchemaVersion: ReviewContractVersion, Kind: "skillglass-review-selection", State: "preview", Proposal: proposal, Chosen: []string{}, Omitted: []string{}, Effects: []ReviewEffect{}}
	recommendations := []ReviewRecommendation{}
	for _, r := range proposal.Draft.Changes {
		if selected[r.ID] {
			s.Chosen = append(s.Chosen, r.ID)
			recommendations = append(recommendations, r)
			delete(selected, r.ID)
		} else {
			s.Omitted = append(s.Omitted, r.ID)
		}
	}
	if len(selected) > 0 {
		return ReviewSelection{}, errors.New("chosen change is absent from this exact proposal")
	}
	s.Effects, err = reviewEffects(recommendations, fresh.plans)
	if err != nil {
		return ReviewSelection{}, err
	}
	s.SelectionID = selectionID(s)
	return s, nil
}

// ApproveReview records the caller's attestation of explicit user consent.
func ApproveReview(s ReviewSelection, approval string) (ReviewSelection, error) {
	if err := ValidateReviewSelection(s, false); err != nil {
		return ReviewSelection{}, err
	}
	if approval == "" || approval != s.SelectionID {
		return ReviewSelection{}, errors.New("approval must match the exact current selection_id shown to the user")
	}
	s.State = "approved"
	return s, nil
}

// ValidateReviewSelection is shared with execution consumers. Live preflight is
// still mandatory before writes; the hashes are not native confirmation tokens.
func ValidateReviewSelection(s ReviewSelection, requireApproval bool) error {
	if s.SchemaVersion != ReviewContractVersion || s.Kind != "skillglass-review-selection" || s.SelectionID == "" || s.SelectionID != selectionID(s) {
		return errors.New("incompatible or altered selection; preview and approve the exact subset again")
	}
	if s.State != "preview" && s.State != "approved" || requireApproval && s.State != "approved" {
		return errors.New("explicit approval of this exact selection is required")
	}
	if err := ValidateReviewProposal(s.Proposal); err != nil {
		return err
	}
	if s.Proposal.State != "prepared" {
		return errors.New("blocked proposal cannot be approved")
	}
	chosen := map[string]bool{}
	for _, id := range s.Chosen {
		if chosen[id] {
			return errors.New("duplicate chosen ID")
		}
		chosen[id] = true
	}
	expectedChosen, expectedOmitted := []string{}, []string{}
	for _, r := range s.Proposal.Draft.Changes {
		if chosen[r.ID] {
			expectedChosen = append(expectedChosen, r.ID)
		} else {
			expectedOmitted = append(expectedOmitted, r.ID)
		}
	}
	if !sameReviewValue(expectedChosen, s.Chosen) || !sameReviewValue(expectedOmitted, s.Omitted) || len(s.Effects) != len(s.Chosen) {
		return errors.New("selection does not preserve the exact proposal subset and order")
	}
	for i, e := range s.Effects {
		if e.ID != s.Chosen[i] {
			return errors.New("selection effects do not match chosen order")
		}
	}
	return nil
}
func sameReviewValue(a, b any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return bytes.Equal(x, y)
}
