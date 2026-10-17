package collection

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type geminiControlPlan struct {
	Path, Name string
	Scope      ControlScope
	Choice     ControlChoice
}

type geminiControlFacts struct {
	invalid error
	names   map[string]int
}

func newGeminiControlFacts(s *Snapshot) geminiControlFacts {
	f := geminiControlFacts{names: make(map[string]int)}
	in := s.geminiControls
	if in == nil || in.uncertain || !s.Complete() {
		f.invalid = errors.New("native inputs or discovery are incomplete; no broader fallback")
		return f
	}
	for _, layer := range in.scope.ConfigLayers {
		if layer.State != "loaded" {
			continue
		}
		raw, err := nativeBytes(layer.Path)
		if value, ok := in.overlay[layer.Path]; ok {
			raw, err = value, nil
		}
		if err != nil {
			f.invalid = errors.New("native settings cannot be inspected")
			return f
		}
		if _, err = readGeminiDisabledDocument(raw); err != nil {
			f.invalid = fmt.Errorf("ambiguous native settings layer %s: %w", layer.Path, err)
			return f
		}
	}
	for _, id := range s.order {
		source := s.sources[id]
		native := false
		for _, ref := range source.Availability.References {
			if ref.Participation != ParticipationInspection {
				native = true
				break
			}
		}
		if !native {
			continue
		}
		evidence := source.Availability.NativeNameEvidence
		if evidence == nil || evidence.Kind == EvidenceUnknown {
			f.invalid = errors.New("opaque native candidate prevents isolated name control")
			return f
		}
		if evidence.Rule == "native-manifest-rejection" {
			continue
		}
		key, err := geminiComparisonKey(source.Availability.NativeName)
		if err != nil {
			f.invalid = err
			return f
		}
		f.names[key]++
	}
	return f
}

func geminiChoiceAchieved(choice ControlChoice, permission PermissionState) bool {
	return choice == ControlDisable && permission == PermissionDenied || choice == ControlEnable && permission == PermissionAllowed
}

func geminiGlobalSnapshot(ctx context.Context, s *Snapshot) (*Snapshot, error) {
	r := cloneRequest(s.request)
	r.Project = ProjectContext{}
	r.geminiPlanning = true
	return Build(ctx, r)
}
func geminiControlRecipe(s *Snapshot, intent ChangeIntent) (geminiControlPlan, error) {
	p := geminiControlPlan{Scope: intent.Scope, Choice: intent.Choice}
	in := s.geminiControls
	if intent.Scope != ControlGlobal && intent.Scope != ControlProject {
		return p, errors.New("choose explicit global or project scope")
	}
	if intent.Choice != ControlEnable && intent.Choice != ControlDisable && intent.Choice != ControlInherit {
		return p, errors.New("choose enable, disable or inherit")
	}
	if intent.Choice == ControlInherit && intent.Scope != ControlProject {
		return p, errors.New("inherit removes only a project override")
	}
	if in == nil || in.controlFacts.names == nil {
		return p, errors.New("native control facts are unavailable")
	}
	if in.controlFacts.invalid != nil {
		return p, in.controlFacts.invalid
	}
	source, ok := s.sources[intent.Source]
	if !ok || !changeWriterEligible(source) || source.Ownership.State == "managed" {
		return p, errors.New("selected source identity, native participation or ownership does not permit isolated control")
	}
	p.Name = source.Availability.NativeName
	if p.Name == "" {
		return p, errors.New("native name is unverified")
	}
	key, err := geminiComparisonKey(p.Name)
	if err != nil {
		return p, err
	}
	if key == "skill-creator" || key == "antigravity-support" {
		return p, errors.New("native name overlaps a bundled built-in recorded for " + geminiValidatedVersion + "; isolated name control is unsupported")
	}
	if in.controlFacts.names[key] > 1 {
		return p, errors.New("independent native copies share a case-equivalent name; name control would affect additional sources")
	}
	global := false
	for _, ref := range source.Availability.References {
		if ref.Provenance.Kind == "plugin" || ref.Provenance.Kind == "system" || ref.Provenance.Kind == "managed-candidate" {
			return p, errors.New("managed, builtin or uncertain extension native control is unverified")
		}
		if ref.Participation == ParticipationEligible {
			if s.references[ref.Reference].Context == ContextGlobal {
				global = true
			}
		}
	}
	p.Path = filepath.Join(in.home, ".gemini/settings.json")
	if intent.Scope == ControlGlobal && !global {
		return p, errors.New("selected source has no native global occurrence; no broader fallback")
	}
	if intent.Scope == ControlProject {
		if !s.request.Project.Selected || in.project == "" || in.trust != ParticipationEligible {
			return p, errors.New("project controls require an explicitly selected trusted workspace; no global fallback")
		}
		p.Path = filepath.Join(in.project, ".gemini/settings.json")
	}
	return p, nil
}
func geminiDestination(s *Snapshot, p geminiControlPlan) (changeDestination, error) {
	d, err := inspectChangeDestinationPath(p.Path)
	if err != nil {
		return d, err
	}
	if err = opencodeDestinationContent(s, p.Path); err != nil {
		return d, err
	}
	in := s.geminiControls
	for _, dir := range []string{filepath.Join(in.home, ".gemini/extensions")} {
		within, e := opencodePathWithin(dir, p.Path)
		if e != nil || within {
			return d, errors.New("destination overlaps managed storage")
		}
	}
	roles := []string{filepath.Join(in.home, ".gemini/settings.json"), geminiEnvPath("GEMINI_CLI_TRUSTED_FOLDERS_PATH", filepath.Join(in.home, ".gemini/trustedFolders.json")), filepath.Join(in.home, ".gemini/extensions/extension-enablement.json")}
	if in.project != "" {
		roles = append(roles, filepath.Join(in.project, ".gemini/settings.json"))
	}
	for _, layer := range in.scope.ConfigLayers {
		roles = append(roles, layer.Path)
	}
	for _, path := range roles {
		same, e := opencodeSamePath(p.Path, path)
		if e != nil {
			return d, e
		}
		if same {
			if path != p.Path {
				return d, errors.New("settings destination aliases another native configuration role")
			}
		}
	}
	// A single lexical path can serve multiple roles through environment settings.
	for _, layer := range in.scope.ConfigLayers {
		same, e := opencodeSamePath(p.Path, layer.Path)
		if e != nil {
			return d, e
		}
		if same && (p.Scope == ControlGlobal && layer.Scope != "user" || p.Scope == ControlProject && layer.Scope != "project") {
			return d, errors.New("settings destination participates in another native configuration role")
		}
	}
	trust := roles[1]
	if same, e := opencodeSamePath(p.Path, trust); e != nil || same {
		return d, errors.New("settings destination aliases trust input")
	}
	return d, nil
}
func previewGeminiChange(ctx context.Context, s *Snapshot, intent ChangeIntent) *ChangePreview {
	if s == nil || s.request.Agent != "gemini" {
		return rejectedChange(intent, "unsupported", "explicit --agent gemini is required")
	}
	fresh, err := Build(ctx, cloneRequest(s.request))
	if err != nil {
		return rejectedChange(intent, "failed", "could not reinspect Gemini context")
	}
	return previewGeminiSnapshot(ctx, fresh, intent)
}
func previewGeminiSnapshot(ctx context.Context, s *Snapshot, intent ChangeIntent) *ChangePreview {
	p, err := geminiControlRecipe(s, intent)
	if err != nil {
		return rejectedChange(intent, "unsupported", err.Error())
	}
	dest, err := geminiDestination(s, p)
	if err != nil {
		return rejectedChange(intent, "refused", err.Error())
	}
	doc, err := readGeminiDisabledDocument(dest.Config.Bytes)
	if err != nil {
		return rejectedChange(intent, "refused", err.Error())
	}
	post, changes, write, err := doc.patch(p.Name, p.Choice, !dest.Config.Exists)
	if err != nil {
		return rejectedChange(intent, "refused", err.Error())
	}
	r := cloneRequest(s.request)
	r.geminiPlanning = true
	r.geminiOverlay = map[string][]byte{p.Path: post}
	if !dest.Config.Exists && !write {
		r.geminiOverlay = nil
	}
	after, err := Build(ctx, r)
	if err != nil {
		return rejectedChange(intent, "failed", "prospective native evaluation failed")
	}
	permission := after.geminiControls.permission(p.Name)
	if p.Choice != ControlInherit && !geminiChoiceAchieved(p.Choice, permission.State) {
		return rejectedChange(intent, "refused", "another settings layer prevents the requested result: "+permission.Evidence.InputPath+"; no broader fallback")
	}
	for id, before := range s.sources {
		if id == intent.Source {
			continue
		}
		other, ok := after.sources[id]
		if !ok || before.Availability.State != other.Availability.State {
			return rejectedChange(intent, "refused", "prospective control changes an independent source or membership")
		}
	}
	if len(s.sources) != len(after.sources) {
		return rejectedChange(intent, "refused", "prospective native membership changed")
	}
	var global *Snapshot
	if p.Scope == ControlGlobal {
		global, err = geminiGlobalSnapshot(ctx, s)
		if err != nil {
			return rejectedChange(intent, "failed", "global context inspection failed")
		}
		if _, err = geminiControlRecipe(global, intent); err != nil {
			return rejectedChange(intent, "refused", "global context: "+err.Error())
		}
		gafter, e := geminiGlobalSnapshot(ctx, after)
		if e != nil || !geminiChoiceAchieved(p.Choice, gafter.geminiControls.permission(p.Name).State) {
			return rejectedChange(intent, "refused", "global settings layer prevents requested result independently of project settings")
		}
	}
	observations, deps, err := changeObservations(s, dest)
	if err != nil {
		return rejectedChange(intent, "refused", err.Error())
	}
	if global != nil {
		_, gd, e := changeObservations(global, dest)
		if e != nil {
			return rejectedChange(intent, "refused", e.Error())
		}
		deps = append(deps, gd...)
	}
	// Pin absent registry/trust paths too, so creation invalidates consent.
	paths := []string{filepath.Join(s.geminiControls.home, ".gemini/extensions"), filepath.Join(s.geminiControls.home, ".gemini/extensions/extension-enablement.json"), geminiEnvPath("GEMINI_CLI_TRUSTED_FOLDERS_PATH", filepath.Join(s.geminiControls.home, ".gemini/trustedFolders.json"))}
	registry := filepath.Join(s.geminiControls.home, ".gemini/extensions")
	entries, _ := os.ReadDir(registry)
	for _, entry := range entries {
		paths = append(paths, filepath.Join(registry, entry.Name(), ".gemini-extension-install.json"))
	}
	for _, path := range paths {
		f, e := observeChangePath(path)
		if e != nil {
			return rejectedChange(intent, "refused", e.Error())
		}
		deps = append(deps, f)
	}
	plan := &changePlan{request: cloneRequest(s.request), intent: intent, postimage: post, destination: dest, snapshotFacts: observations, dependencies: deps}
	data, _ := json.Marshal(struct {
		Version      string
		Recipe       geminiControlPlan
		Facts        []byte
		Dependencies []changeFact
		Destination  changeDestination
		Post         []byte
	}{"gemini-0.63.0-controls-v1", p, observations, deps, dest, post})
	sum := sha256.Sum256(data)
	plan.digest = "gemini1:" + hex.EncodeToString(sum[:])
	source := s.sources[intent.Source]
	create := dest.CreateDirectory
	if !write {
		create = ""
	}
	return &ChangePreview{plan: plan, view: ChangeView{State: "ready", Reason: "preview only; no settings changed", Intent: intent, SourcePath: source.Directory, ManifestPath: source.Manifest.Path, ConfigPath: p.Path, InspectionProject: s.request.Project, Changes: changes, KnownReferences: s.References(source.ID), Before: source.Availability, NoWriteNeeded: !write, CreateDirectory: create, Confirmation: plan.digest, Notes: []string{fmt.Sprintf("Gemini native name: %s. Prospective permission: %s. Activation consent remains required.", p.Name, permission.State), "Gemini CLI 0.63.0 disabled-name controls validated in disposable native fixtures. Inherit removes only the project entry; unioned layers may still disable it.", "Fresh native launch sees persisted changes. Source-verified /skills reload or /skills refresh, or restart, loads per-name changes; running-session adoption remains unobserved."}}}
}
func applyGeminiCapabilities(s *Snapshot) {
	for _, id := range s.order {
		source := s.sources[id]
		source.Availability.Capabilities = nil
		for _, v := range []struct {
			op     string
			scope  ControlScope
			choice ControlChoice
		}{{"enable", ControlGlobal, ControlEnable}, {"disable", ControlGlobal, ControlDisable}, {"project-override", ControlProject, ControlDisable}, {"inherit", ControlProject, ControlInherit}} {
			_, err := geminiControlRecipe(s, ChangeIntent{Source: id, Application: "gemini", Scope: v.scope, Choice: v.choice})
			state, reason := "supported", "native disabled-name writer eligibility checked; preview verifies effective result"
			kind := EvidenceInferred
			if err != nil {
				state, reason, kind = "unsupported", err.Error(), EvidenceUnknown
			}
			source.Availability.Capabilities = append(source.Availability.Capabilities, Capability{Operation: v.op, Scope: string(v.scope), State: state, Evidence: geminiEvidence(kind, "native-disabled-name-control", source.Manifest.Path, "skills.disabled", reason)})
		}
		source.Availability.Capabilities = append(source.Availability.Capabilities, Capability{Operation: "delete", Scope: "source", State: "unsupported", Evidence: geminiEvidence(EvidenceUnknown, "no-delete", source.Directory, "", "deletion is unsupported")})
		s.sources[id] = source
	}
}
