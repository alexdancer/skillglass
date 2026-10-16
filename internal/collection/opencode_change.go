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
	"regexp"
	"strings"
)

type opencodeControlRefusal struct{ reason string }

func (e opencodeControlRefusal) Error() string { return e.reason }

type opencodeControlPlan struct {
	ConfigPath, NativeName, Action string
	Scope                          ControlScope
	Choice                         ControlChoice
}

var opencodeControlName = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

func opencodeControlRecipe(s *Snapshot, intent ChangeIntent, facts *opencodeControlFacts) (opencodeControlPlan, error) {
	p := opencodeControlPlan{Scope: intent.Scope, Choice: intent.Choice}
	if intent.Scope != ControlGlobal && intent.Scope != ControlProject {
		return p, errors.New("choose explicit global or project scope")
	}
	if intent.Choice != ControlEnable && intent.Choice != ControlDisable && intent.Choice != ControlInherit {
		return p, errors.New("choose enable, disable or inherit")
	}
	if intent.Choice == ControlInherit && intent.Scope != ControlProject {
		return p, errors.New("inherit removes only a project override")
	}
	in := s.opencodeControls
	if intent.Scope == ControlGlobal {
		if err := facts.validateGlobal(intent.Source); err != nil {
			return p, err
		}
	} else if err := facts.selected.validate(intent.Source); err != nil {
		return p, err
	}
	if intent.Scope == ControlProject && (!s.request.Project.Selected || opencodeBool("OPENCODE_DISABLE_PROJECT_CONFIG")) {
		return p, errors.New("project controls require an explicit project with native project configuration enabled; no global fallback")
	}
	source := s.sources[intent.Source]
	p.NativeName = source.Availability.NativeName
	if len(p.NativeName) > 64 || !opencodeControlName.MatchString(p.NativeName) || p.NativeName == "customize-opencode" {
		return p, errors.New("native name has unverified pattern semantics or collides with embedded customize-opencode")
	}
	globalOccurrence := false
	for _, fact := range source.Availability.References {
		if fact.Participation == ParticipationEligible && s.references[fact.Reference].Context == ContextGlobal {
			globalOccurrence = true
		}
	}
	if intent.Scope == ControlGlobal && !globalOccurrence {
		return p, errors.New("selected source has no native global occurrence; no broader fallback")
	}
	dir := filepath.Join(opencodeConfigHome(s.request.home), "opencode")
	if intent.Scope == ControlProject {
		dir = s.request.Project.Path
	}
	p.ConfigPath = filepath.Join(dir, "opencode.json")
	if _, e := os.Lstat(filepath.Join(dir, "opencode.jsonc")); !errors.Is(e, os.ErrNotExist) {
		p.ConfigPath = filepath.Join(dir, "opencode.jsonc")
	}
	if intent.Choice == ControlInherit {
		for _, input := range in.inputs {
			if input.Scope != "project" && !(input.Scope == "native-directory" && facts.projectInputs[input.Path]) {
				continue
			}
			if opencodeSavedPermission(input.Raw, p.NativeName) == "" {
				continue
			}
			if filepath.Dir(input.Path) != dir {
				return p, errors.New("another project layer retains an exact override; inheritance cannot be restored at the selected scope")
			}
			if p.ConfigPath != input.Path && opencodeInputExact(in, p.ConfigPath, p.NativeName) {
				return p, errors.New("multiple project layers retain exact overrides; inheritance is ambiguous")
			}
			p.ConfigPath = input.Path
		}
	}
	p.Action = "allow"
	if intent.Choice == ControlDisable {
		p.Action = "deny"
	}
	// A meaningful current ask already permits use with approval. Enabling does
	// not weaken it, nor infer an unrecorded historical ask from a current deny.
	if intent.Choice == ControlEnable && source.Availability.State == AvailabilityAsk {
		p.Action = "ask"
	}
	return p, nil
}
func opencodeInputExact(in *opencodeInputs, path, name string) bool {
	for _, input := range in.inputs {
		if input.Path == path && opencodeSavedPermission(input.Raw, name) != "" {
			return true
		}
	}
	return false
}
func opencodeProjectPath(s *Snapshot, path string) bool {
	ancestors, _, err := nativeProjectAncestors(s.request.Project)
	if err != nil {
		return false
	}
	for _, dir := range ancestors {
		within, err := opencodePathWithin(filepath.Join(dir, ".opencode"), path)
		if err != nil || within {
			return true
		}
	}
	return false
}

func previewOpencodeChange(ctx context.Context, s *Snapshot, intent ChangeIntent) *ChangePreview {
	if s == nil || s.request.Agent != "opencode" {
		return rejectedChange(intent, "unsupported", "explicit --agent opencode is required")
	}
	fresh, err := Build(ctx, cloneRequest(s.request))
	if err != nil {
		return rejectedChange(intent, "failed", "could not reinspect original OpenCode context")
	}
	if _, ok := fresh.sources[intent.Source]; !ok {
		return rejectedChange(intent, "refused", "selected source is no longer reachable; select its exact source ID")
	}
	return previewOpencodeSnapshot(ctx, fresh, intent, true, newOpencodeControlFacts(ctx, fresh))
}
func previewOpencodeSnapshot(ctx context.Context, s *Snapshot, intent ChangeIntent, pin bool, facts *opencodeControlFacts) *ChangePreview {
	recipe, err := opencodeControlRecipe(s, intent, facts)
	if err != nil {
		var refusal opencodeControlRefusal
		if errors.As(err, &refusal) {
			return rejectedChange(intent, "refused", err.Error())
		}
		return rejectedChange(intent, "unsupported", err.Error())
	}
	dest, err := facts.destination(recipe)
	if err != nil {
		return rejectedChange(intent, "refused", err.Error())
	}
	// Keep ask as a verified no-op when it already permits this source. A future
	// independent disable has an explicit deny -> allow preview with no history.
	post, changes, write, err := patchOpencodeChoice(dest.Config.Bytes, recipe.NativeName, recipe.Action, intent.Choice == ControlInherit)
	if intent.Choice == ControlEnable && s.sources[intent.Source].Availability.State == AvailabilityAsk {
		post = append([]byte(nil), dest.Config.Bytes...)
		changes = []SettingChange{{Setting: "effective permission.skill." + recipe.NativeName, Before: "ask", After: "ask (preserved; approval required)"}}
		write = false
		err = nil
	}
	if err != nil {
		return rejectedChange(intent, "refused", err.Error())
	}
	if !pin {
		return &ChangePreview{view: ChangeView{State: "ready", Intent: intent}}
	}
	prospective := cloneRequest(s.request)
	prospective.opencodePlanning = true
	prospective.opencodeOverlay = map[string][]byte{recipe.ConfigPath: post}
	// A missing no-op destination is not a new native input.
	if !dest.Config.Exists && !write {
		prospective.opencodeOverlay = nil
	}
	after, err := Build(ctx, prospective)
	if err != nil {
		return rejectedChange(intent, "failed", "prospective OpenCode evaluation failed")
	}
	selected, ok := after.sources[intent.Source]
	if !ok {
		return rejectedChange(intent, "refused", "selected native source would no longer be reachable")
	}
	if !after.Complete() || after.opencodeControls.uncertain || intent.Choice == ControlDisable && selected.Availability.State != AvailabilityDisabled || intent.Choice == ControlEnable && selected.Availability.State != AvailabilityAvailable && selected.Availability.State != AvailabilityExplicitOnly && selected.Availability.State != AvailabilityAsk {
		return rejectedChange(intent, "refused", "higher-priority configuration, agent policy or ordered wildcard prevents the requested result; no broader fallback")
	}
	globalBefore := facts.global.snapshot
	globalInputs := globalBefore.opencodeControls.inputs
	var expectedGlobal *opencodeContextFacts
	if intent.Scope == ControlGlobal {
		globalAfter, err := opencodeGlobalSnapshot(ctx, after)
		if err != nil {
			return rejectedChange(intent, "failed", "prospective global OpenCode inventory could not be inspected")
		}
		expectedGlobal = newOpencodeContextFacts(globalAfter)
		if err := expectedGlobal.validate(intent.Source); err != nil {
			return rejectedChange(intent, "refused", "prospective global native control certainty: "+err.Error())
		}
		permission := globalAfter.opencodeControls.permission(recipe.NativeName)
		if !opencodeChoiceAchieved(intent.Choice, permission.State) {
			return rejectedChange(intent, "refused", "global configuration does not achieve the requested result independently of project overrides; no broader fallback")
		}
		if !opencodePatchOnlySelected(cloneOpencodeObject(globalBefore.opencodeControls.config), cloneOpencodeObject(globalAfter.opencodeControls.config), recipe.NativeName) {
			return rejectedChange(intent, "refused", "conversion would change unrelated global native rules independently of project overrides")
		}
		if !opencodeIndependentEffects(globalBefore, globalAfter, intent.Source) {
			return rejectedChange(intent, "refused", "permission rewrite would affect an independent global source or membership")
		}
	}
	// This check ranges over every ordered rule, including names not discovered
	// today. It catches scalar replacement resurrecting lower-layer exact rules.
	if !opencodePatchOnlySelected(cloneOpencodeObject(s.opencodeControls.config), cloneOpencodeObject(after.opencodeControls.config), recipe.NativeName) {
		return rejectedChange(intent, "refused", "conversion would change unrelated ordered native rules, including scalar replacement or inherited insertion order")
	}
	if !opencodeIndependentEffects(s, after, intent.Source) {
		return rejectedChange(intent, "refused", "permission rewrite would affect an independent source or membership")
	}
	observations, deps, err := opencodeChangeObservations(s, globalBefore, dest)
	if err != nil {
		return rejectedChange(intent, "refused", "relevant OpenCode inputs cannot be pinned: "+err.Error())
	}
	plan := &changePlan{request: cloneRequest(s.request), intent: intent, postimage: post, destination: dest, snapshotFacts: observations, dependencies: deps, opencodeGlobalExpected: expectedGlobal}
	data, _ := json.Marshal(struct {
		Version      string
		Recipe       opencodeControlPlan
		Facts        []byte
		Dependencies []changeFact
		Destination  changeDestination
		Post         []byte
		Inputs       []opencodeConfigInput
		GlobalInputs []opencodeConfigInput
	}{"opencode-1.17.15-controls-v1", recipe, observations, deps, dest, post, s.opencodeControls.inputs, globalInputs})
	sum := sha256.Sum256(data)
	plan.digest = "opencode1:" + hex.EncodeToString(sum[:])
	source := s.sources[intent.Source]
	createDirectory := dest.CreateDirectory
	if !write {
		createDirectory = ""
	}
	return &ChangePreview{plan: plan, view: ChangeView{State: "ready", Reason: "preview only; no settings changed", Intent: intent, SourcePath: source.Directory, ManifestPath: source.Manifest.Path, ConfigPath: recipe.ConfigPath, InspectionProject: s.request.Project, Changes: changes, KnownReferences: s.References(source.ID), Before: source.Availability, NoWriteNeeded: !write, CreateDirectory: createDirectory, Confirmation: plan.digest, Notes: []string{fmt.Sprintf("Prospective configured result: %s (permission %s).", selected.Availability.State, after.opencodeControls.permission(recipe.NativeName).State), "OpenCode 1.17.15 exact name permission controls validated in disposable fixtures; unrelated JSONC bytes and ordered rules are preserved.", "Inherit removes only the exact project rule; remaining native rules may allow, ask or deny. Enable preserves an existing effective ask; a current deny carries no historical approval state.", "Fresh-process configuration reads and debug denial were validated. Running OpenCode may require reload or restart; live session adoption, approval prompting and full skill loading are unverified."}}}
}

func opencodeChoiceAchieved(choice ControlChoice, permission PermissionState) bool {
	return choice == ControlDisable && permission == PermissionDenied || choice == ControlEnable && (permission == PermissionAllowed || permission == PermissionAsk)
}

func cloneOpencodeObject(o *opencodeObject) *opencodeObject {
	out := newOpencodeObject()
	for _, key := range o.keys {
		value := o.get(key)
		if child, ok := value.(*opencodeObject); ok {
			value = cloneOpencodeObject(child)
		}
		out.put(key, value, o.paths[key])
	}
	return out
}

// Resolve existing ancestors as well as missing basenames so a not-yet-created
// output through an alias cannot escape its selected native role.
func opencodeOutputPath(path string) (string, error) {
	path = filepath.Clean(path)
	tail := []string{}
	for {
		resolved, err := filepath.EvalSymlinks(path)
		if err == nil {
			for i := len(tail) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, tail[i])
			}
			return resolved, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(path)
		if parent == path {
			return "", err
		}
		tail = append(tail, filepath.Base(path))
		path = parent
	}
}
func opencodeSamePath(left, right string) (bool, error) {
	leftInfo, leftErr := os.Stat(left)
	rightInfo, rightErr := os.Stat(right)
	if leftErr == nil && rightErr == nil {
		return os.SameFile(leftInfo, rightInfo), nil
	}
	if leftErr != nil && !errors.Is(leftErr, os.ErrNotExist) {
		return false, leftErr
	}
	if rightErr != nil && !errors.Is(rightErr, os.ErrNotExist) {
		return false, rightErr
	}
	if leftErr == nil || rightErr == nil || !strings.EqualFold(filepath.Base(left), filepath.Base(right)) {
		return false, nil
	}
	leftParent, rightParent := filepath.Dir(left), filepath.Dir(right)
	if leftParent == left || rightParent == right {
		return false, nil
	}
	return opencodeSamePath(leftParent, rightParent)
}

func opencodePathWithin(root, path string) (bool, error) {
	resolved, err := opencodeOutputPath(path)
	if err != nil {
		return false, err
	}
	for {
		same, err := opencodeSamePath(root, resolved)
		if err != nil || same {
			return same, err
		}
		parent := filepath.Dir(resolved)
		if parent == resolved {
			return false, nil
		}
		resolved = parent
	}
}

func opencodeDestinationContent(s *Snapshot, output string) error {
	for _, source := range s.sources {
		same, sameErr := opencodeSamePath(output, source.Manifest.ResolvedPath)
		within, withinErr := opencodePathWithin(source.Directory, output)
		if sameErr != nil || withinErr != nil || same || within {
			return errors.New("configuration output overlaps discovered skill content; shared content is not a native settings destination")
		}
	}
	return nil
}

func opencodeDestinationScope(s *Snapshot, p opencodeControlPlan) error {
	output, err := opencodeOutputPath(p.ConfigPath)
	if err != nil {
		return err
	}
	in := s.opencodeControls
	if err := opencodeDestinationContent(s, output); err != nil {
		return err
	}
	for _, dir := range []string{in.cache, "/Library/Application Support/opencode", os.Getenv("OPENCODE_TEST_MANAGED_CONFIG_DIR")} {
		if dir == "" {
			continue
		}
		within, e := opencodePathWithin(dir, output)
		if e != nil || within {
			return errors.New("configuration output overlaps managed or unverified storage")
		}
	}
	global := filepath.Join(opencodeConfigHome(s.request.home), "opencode")
	project := s.request.Project.Path
	if p.Scope == ControlProject {
		within, e := opencodePathWithin(global, output)
		if e != nil || within {
			return errors.New("project destination aliases global configuration")
		}
	}
	if p.Scope == ControlGlobal && s.request.Project.Selected {
		for _, name := range []string{"opencode.json", "opencode.jsonc"} {
			same, e := opencodeSamePath(output, filepath.Join(project, name))
			if e != nil || same {
				return errors.New("global destination aliases project configuration")
			}
		}
	}
	for _, layer := range in.scope.ConfigLayers {
		if !filepath.IsAbs(layer.Path) {
			continue
		}
		same, e := opencodeSamePath(layer.Path, output)
		if e != nil {
			return errors.New("native config role cannot be separated safely")
		}
		if !same {
			continue
		}
		if layer.Path != p.ConfigPath {
			return errors.New("configuration output aliases another native config input role")
		}
		if p.Scope == ControlProject && layer.Scope == "native-directory" || p.Scope == ControlGlobal && layer.Scope == "native-directory" && opencodeProjectPath(s, layer.Path) {
			return errors.New("configuration output is also a native-directory input outside the verified selected write role")
		}
		if layer.Scope == "managed" || layer.Scope == "custom-config" || p.Scope == ControlProject && layer.Scope == "global" || p.Scope == ControlGlobal && layer.Scope == "project" {
			return errors.New("configuration output participates in a broader or unverified native load role")
		}
	}
	return nil
}
func applyOpencodeCapabilities(s *Snapshot) {
	facts := newOpencodeControlFacts(context.Background(), s)
	for _, id := range s.order {
		source := s.sources[id]
		source.Availability.Capabilities = nil
		for _, entry := range []struct {
			op     string
			scope  ControlScope
			choice ControlChoice
		}{{"enable", ControlGlobal, ControlEnable}, {"disable", ControlGlobal, ControlDisable}, {"project-override", ControlProject, ControlDisable}, {"inherit", ControlProject, ControlInherit}} {
			p := previewOpencodeSnapshot(context.Background(), s, ChangeIntent{Source: id, Application: "opencode", Scope: entry.scope, Choice: entry.choice}, false, facts)
			state, kind, reason := "unsupported", EvidenceUnknown, p.view.Reason
			if p.view.State == "ready" {
				state, kind, reason = "supported", EvidenceInferred, "native writer eligibility checked; explicit preview must validate effective result and independent-source effects"
			}
			source.Availability.Capabilities = append(source.Availability.Capabilities, Capability{Operation: entry.op, Scope: string(entry.scope), State: state, Evidence: opencodeEvidence(kind, "native-permission-control", source.Manifest.Path, "permission.skill", reason)})
		}
		source.Availability.Capabilities = append(source.Availability.Capabilities, Capability{Operation: "delete", Scope: "source", State: "unsupported", Evidence: opencodeEvidence(EvidenceUnknown, "no-delete", source.Directory, "", "deletion is unsupported")})
		s.sources[id] = source
	}
}
