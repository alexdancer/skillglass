package collection

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path/filepath"
)

// ReviewReceipt retains the native writer result separately from final reinspection.
// A later failure never erases an earlier write or implies rollback.
type ReviewReceipt struct {
	ID               string                   `json:"id"`
	Intent           ChangeIntent             `json:"intent"`
	Project          string                   `json:"project,omitempty"`
	State            string                   `json:"state"`
	Reason           string                   `json:"reason"`
	Outcome          *ChangeOutcome           `json:"outcome,omitempty"`
	Final            *AvailabilityExplanation `json:"final_configured,omitempty"`
	FinalObservation string                   `json:"final_observation"`
}
type ReviewOutcome struct {
	SchemaVersion int               `json:"schema_version"`
	Kind          string            `json:"kind"`
	SelectionID   string            `json:"selection_id"`
	State         string            `json:"state"`
	Reason        string            `json:"reason"`
	Results       []ReviewReceipt   `json:"results"`
	Omitted       []string          `json:"omitted"`
	Reinspection  []ReviewDiscovery `json:"reinspection"`
	Limitations   []string          `json:"limitations"`
}

func (o ReviewOutcome) ExitCode() int {
	if o.State == "applied" {
		return 0
	}
	return 1
}

// ApplyReview consumes contract 1. Approval is a caller attestation, not an
// authenticated identity. The entire selection is revalidated before any write.
func ApplyReview(ctx context.Context, selection ReviewSelection) ReviewOutcome {
	out := ReviewOutcome{SchemaVersion: ReviewContractVersion, Kind: "skillglass-review-outcome", SelectionID: selection.SelectionID, State: "refused", Results: []ReviewReceipt{}, Omitted: selection.Omitted, Reinspection: []ReviewDiscovery{}, Limitations: []string{
		"Completed writes remain in place. No cross-file atomicity or automatic rollback.",
		"Reinspection observes configured availability only; running-session adoption and actual usage are unobserved.",
		"Discovery and evidence retain the approved proposal's bounded coverage and limitations.",
	}}
	recommendations := map[string]ReviewRecommendation{}
	for _, r := range selection.Proposal.Draft.Changes {
		recommendations[r.ID] = r
	}
	for _, id := range selection.Chosen {
		r := recommendations[id]
		out.Results = append(out.Results, ReviewReceipt{ID: id, Intent: ChangeIntent{Source: r.Source, Application: r.Application, Scope: selection.Proposal.Draft.Context.Scope, Choice: r.Action}, Project: selection.Proposal.Draft.Context.Project, State: "not-attempted", Reason: "whole selection unapplied", FinalObservation: "not-observed"})
	}
	if err := ValidateReviewSelection(selection, true); err != nil {
		out.Reason = err.Error()
		return out
	}
	// Reconstruct every original observation, including omitted recommendations,
	// evidence, consumers and coverage before composing the exact subset.
	prepared, err := prepareReview(ctx, selection.Proposal.Draft)
	if err != nil || prepared.proposal.ProposalID != selection.Proposal.ProposalID {
		out.Reason = "whole-selection preflight refused; observations changed; reprepare and renew approval"
		return reinspectReview(ctx, selection, out)
	}
	chosen := []ReviewRecommendation{}
	for _, id := range selection.Chosen {
		chosen = append(chosen, recommendations[id])
	}
	effects, err := reviewEffects(chosen, prepared.plans)
	if err != nil || !sameReviewValue(effects, selection.Effects) {
		out.Reason = "whole-selection preflight refused; selected effects changed; reprepare and renew approval"
		return reinspectReview(ctx, selection, out)
	}
	guard, err := newReviewGuard(ctx, selection, prepared)
	if err != nil {
		out.Reason = "cannot establish whole-selection observations: " + err.Error()
		return reinspectReview(ctx, selection, out)
	}
	// Bind guard capture to the same original observations rather than adopting
	// external edits that happened while the read set was collected.
	check, err := prepareReview(ctx, selection.Proposal.Draft)
	if err != nil || check.proposal.ProposalID != selection.Proposal.ProposalID {
		out.Reason = "observations changed during preflight; renew approval"
		return reinspectReview(ctx, selection, out)
	}
	out.State, out.Reason = "applied", "all selected requests succeeded"
	for i, effect := range selection.Effects {
		receipt := &out.Results[i]
		fail := func(state, reason string) {
			receipt.State, receipt.Reason = state, reason
			out.State, out.Reason = "failed", "stopped at first non-success; earlier writes remain; renew approval before continuing"
			for j := i + 1; j < len(out.Results); j++ {
				out.Results[j].Reason = "not attempted after " + effect.ID + " non-success"
			}
		}
		if err := guard.check(ctx); err != nil {
			fail("refused", err.Error())
			break
		}
		request := prepared.plans[effect.ID].plan.request
		snapshot, err := Build(ctx, request)
		if err != nil {
			fail("failed", "native reinspection failed: "+err.Error())
			break
		}
		native := snapshot.PreviewChange(ctx, effect.Intent)
		if native.plan == nil || native.view.State != "ready" {
			fail(native.view.State, native.view.Reason)
			break
		}
		// A fresh token is necessary but insufficient: authorize only the exact
		// approved effect against the expected prior native postimages.
		effects, err := reviewEffects([]ReviewRecommendation{recommendations[effect.ID]}, map[string]*ChangePreview{effect.ID: native})
		if err != nil || len(effects) != 1 || !sameReviewValue(effects[0], effect) {
			fail("refused", "remaining native effects changed; reprepare and renew approval")
			break
		}
		discovery, err := guard.project(ctx, native.plan)
		if err != nil {
			fail("refused", err.Error())
			break
		}
		native.plan.reviewCheck = guard.check
		native.plan.reviewCheckCreated = guard.checkCreated
		result := native.Apply(ctx, native.plan.digest)
		receipt.Outcome = &result
		receipt.State, receipt.Reason = result.State, result.Reason
		if result.ExitCode() != 0 {
			fail(result.State, result.Reason)
			break
		}
		if effect.NoWriteNeeded {
			receipt.State = "no-op"
		} else {
			receipt.State = "succeeded"
		}
		if err := guard.advance(native.plan, discovery); err != nil {
			fail("failed", err.Error())
			break
		}
	}
	return reinspectReview(ctx, selection, out)
}

func reinspectReview(ctx context.Context, selection ReviewSelection, out ReviewOutcome) ReviewOutcome {
	// Always try every selected application, including unapplied items. Native
	// write receipts are historical; these final observations cannot rewrite them.
	for _, app := range selection.Proposal.Draft.Context.Applications {
		var project *string
		if selection.Proposal.Draft.Context.Scope == ControlProject {
			project = &selection.Proposal.Draft.Context.Project
		}
		request, err := NewRequestWithAgent(nil, nil, project, app)
		var snapshot *Snapshot
		if err == nil {
			snapshot, err = Build(ctx, request)
		}
		if err != nil {
			for i := range out.Results {
				if out.Results[i].Intent.Application == app {
					out.Results[i].FinalObservation = "failed: " + err.Error()
				}
			}
			if out.State == "applied" {
				out.State, out.Reason = "failed", "final reinspection failed"
			}
			continue
		}
		out.Reinspection = append(out.Reinspection, ReviewDiscovery{Application: app, View: snapshot.Query(Filters{Context: FilterAll}, QuerySources)})
		for i := range out.Results {
			r := &out.Results[i]
			if r.Intent.Application != app {
				continue
			}
			if source, ok := snapshot.Source(r.Intent.Source); ok {
				final := source.Availability
				r.Final, r.FinalObservation = &final, "observed"
				if (r.State == "succeeded" || r.State == "no-op") && !reviewChoiceObserved(r.Intent.Choice, final.State) && out.State == "applied" {
					out.State, out.Reason = "failed", "final configured result no longer meets the approved request"
				}
			} else {
				r.FinalObservation = "source-unreachable"
				if out.State == "applied" {
					out.State, out.Reason = "failed", "source unreachable during final reinspection"
				}
			}
		}
	}
	return out
}
func reviewChoiceObserved(choice ControlChoice, state AvailabilityState) bool {
	return choice == ControlDisable && state == AvailabilityDisabled || choice == ControlEnable && (state == AvailabilityAvailable || state == AvailabilityExplicitOnly || state == AvailabilityAsk)
}

// reviewGuard guards observations, not stale per-source confirmation tokens.
// Native configuration bytes are the inputs to availability; exact postimages
// alone may advance those inputs. Structural discovery is checked independently
// so a new source/reference cannot hide behind unchanged directory metadata.
type reviewGuard struct {
	facts     map[string]changeFact
	failures  map[string]string
	requests  []Request
	discovery []string
	resources []reviewResourceGuard
}

type reviewResourceGuard struct {
	request     Request
	source      SourceID
	path        string
	observation string
}

func newReviewGuard(ctx context.Context, selection ReviewSelection, prepared preparedReview) (*reviewGuard, error) {
	g := &reviewGuard{facts: map[string]changeFact{}, failures: map[string]string{}}
	add := func(path string) error {
		if path == "" {
			return nil
		}
		fact, err := observeChangePath(path)
		if err != nil {
			return err
		}
		g.facts[path] = fact
		return nil
	}
	// Every catalog application participates in the approved shared-consumer
	// context, including unselected applications. Capture its native inputs too.
	for _, capability := range applicationCatalog {
		var project *string
		if selection.Proposal.Draft.Context.Scope == ControlProject {
			project = &selection.Proposal.Draft.Context.Project
		}
		request, err := NewRequestWithAgent(nil, nil, project, capability.Application)
		if err != nil {
			return nil, err
		}
		snapshot, err := Build(ctx, request)
		if err != nil {
			return nil, fmt.Errorf("%s discovery: %w", capability.Application, err)
		}
		for _, path := range changeObservationPaths(snapshot.Query(Filters{Context: FilterAll}, QuerySources), changeDestination{}) {
			fact, err := observeChangePath(path)
			g.facts[path] = fact
			if err != nil {
				g.failures[path] = err.Error()
			}
		}
		g.requests = append(g.requests, request)
		g.discovery = append(g.discovery, reviewDiscoveryShape(snapshot))
	}
	for _, id := range selection.Chosen {
		plan := prepared.plans[id].plan
		for _, fact := range plan.dependencies {
			g.facts[fact.Path] = fact
		}
		if err := add(plan.destination.Parent.Path); err != nil {
			return nil, err
		}
		if err := add(plan.destination.Path); err != nil {
			return nil, err
		}
		if err := add(plan.destination.CreateDirectory); err != nil {
			return nil, err
		}
	}
	for _, source := range selection.Proposal.Sources {
		var owner *Snapshot
		var ownerRequest Request
		var err error
		for _, request := range g.requests {
			if request.Agent == source.Applications[0] {
				ownerRequest = request
				owner, err = Build(ctx, request)
				if err != nil {
					return nil, err
				}
				break
			}
		}
		if owner == nil {
			return nil, errors.New("reviewed source has no discovery owner")
		}
		for _, assessment := range selection.Proposal.Draft.Sources {
			if assessment.Source != source.Source.ID {
				continue
			}
			for _, path := range assessment.SupportingPaths {
				g.resources = append(g.resources, reviewResourceGuard{ownerRequest, source.Source.ID, path, reviewHash(owner.ReadResource(ctx, source.Source.ID, path))})
			}
		}
		for _, evidence := range source.Evidence {
			if evidence.Coverage != Complete {
				continue
			}
			path := evidence.Path
			if !filepath.IsAbs(path) {
				path = filepath.Join(source.Source.Directory, path)
			}
			if err := add(path); err != nil {
				return nil, fmt.Errorf("evidence %s: %w", path, err)
			}
			if evidence.Coverage == Complete {
				resolved, err := filepath.EvalSymlinks(path)
				if err != nil {
					return nil, err
				}
				if err := add(resolved); err != nil {
					return nil, err
				}
			}
		}
	}
	return g, nil
}
func reviewDiscoveryShape(snapshot *Snapshot) string {
	view := snapshot.Query(Filters{Context: FilterAll}, QuerySources)
	// Availability and native diagnostics are derived from guarded configuration
	// inputs. Retain source identity/content/resources/ownership and references,
	// roots, project and coverage.
	for i := range view.Sources {
		view.Sources[i].Availability = AvailabilityExplanation{}
		view.Sources[i].Problems = nil
	}
	view.Problems = nil
	view.Scope.Agent = nil
	return reviewHash(view)
}
func (g *reviewGuard) check(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	for path, expected := range g.facts {
		current, err := observeChangePath(path)
		currentFailure := ""
		if err != nil {
			currentFailure = err.Error()
		}
		if currentFailure != g.failures[path] || !sameChangeFacts(expected, current) {
			return fmt.Errorf("unexpected external change at %s; remaining selection unapplied; renew approval", path)
		}
	}
	for i, request := range g.requests {
		snapshot, err := Build(ctx, request)
		if err != nil || reviewDiscoveryShape(snapshot) != g.discovery[i] {
			return errors.New("discovery, source identity, evidence or shared consumers changed; renew approval")
		}
		// New native input paths also require approval. Unavailable inputs in
		// an unselected consumer retain their exact partial fact and error.
		for _, path := range changeObservationPaths(snapshot.Query(Filters{Context: FilterAll}, QuerySources), changeDestination{}) {
			if _, ok := g.facts[path]; !ok {
				return fmt.Errorf("new native input %s requires renewed approval", path)
			}
		}
	}
	for _, resource := range g.resources {
		snapshot, err := Build(ctx, resource.request)
		if err != nil || reviewHash(snapshot.ReadResource(ctx, resource.source, resource.path)) != resource.observation {
			return errors.New("supporting evidence or its coverage changed; renew approval")
		}
	}
	return nil
}
func (g *reviewGuard) project(ctx context.Context, plan *changePlan) ([]string, error) {
	discovery := append([]string(nil), g.discovery...)
	if plan.intent.Application != "pi" || bytes.Equal(plan.destination.Config.Bytes, plan.postimage) {
		return discovery, nil
	}
	for i, request := range g.requests {
		if request.Agent != "pi" {
			continue
		}
		request = cloneRequest(request)
		request.piPlanning = true
		request.piSettingsOverlay = map[string][]byte{plan.destination.Path: plan.postimage}
		snapshot, err := Build(ctx, request)
		if err != nil {
			return nil, fmt.Errorf("approved Pi discovery projection failed: %w", err)
		}
		discovery[i] = reviewDiscoveryShape(snapshot)
		for _, path := range changeObservationPaths(snapshot.Query(Filters{Context: FilterAll}, QuerySources), changeDestination{}) {
			if _, exists := g.facts[path]; exists {
				continue
			}
			fact, err := observeChangePath(path)
			g.facts[path] = fact
			if err != nil {
				g.failures[path] = err.Error()
			}
		}
	}
	return discovery, g.check(ctx)
}
func (g *reviewGuard) advance(plan *changePlan, discovery []string) error {
	if plan.committedFact == nil {
		if !bytes.Equal(plan.destination.Config.Bytes, plan.postimage) {
			return errors.New("installed object identity could not be verified; remaining selection unapplied")
		}
		// A native no-op leaves the read set unchanged.
		return g.check(context.Background())
	}
	// committedFact is captured by the native writer and bound to the exact
	// staged inode. Observing an arbitrary same-byte replacement is insufficient.
	fact := *plan.committedFact
	g.discovery = discovery
	g.facts[plan.destination.Path] = fact
	if plan.createdFact != nil {
		g.addCreated(*plan.createdFact, plan.destination.Path)
	}
	return g.check(context.Background())
}
func stringsWithin(path, parent string) bool {
	rel, err := filepath.Rel(parent, path)
	return err == nil && rel != ".." && !filepath.IsAbs(rel) && !(len(rel) > 3 && rel[:3] == "../")
}

// During native exclusive directory creation, accept only its held directory
// identity. The native writer separately requires that its temporary file is
// the directory's sole entry and retains its full dependency recheck.
func (g *reviewGuard) checkCreated(ctx context.Context, created changeFact) error {
	projected := &reviewGuard{facts: map[string]changeFact{}, requests: g.requests, discovery: g.discovery, resources: g.resources, failures: g.failures}
	for path, fact := range g.facts {
		projected.facts[path] = fact
	}
	projected.addCreated(created, "")
	return projected.check(ctx)
}
func (g *reviewGuard) addCreated(created changeFact, destination string) {
	g.facts[created.Path] = created
	for path, old := range g.facts {
		if path == created.Path || path == destination {
			continue
		}
		if stringsWithin(path, created.Path) {
			old.Chain = append([]changeIdentity{created.Identity}, old.Chain...)
			g.facts[path] = old
		}
	}
}
