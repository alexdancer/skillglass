package collection

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

type RootID string
type ReferenceID string
type SourceID string
type ProblemID string

type ReferenceContext string

const (
	ContextCustom  ReferenceContext = "custom"
	ContextProject ReferenceContext = "project"
	ContextGlobal  ReferenceContext = "global"
	ContextSystem  ReferenceContext = "system"
	ContextManaged ReferenceContext = "managed"
)

type ContextFilter string

const (
	FilterAll     ContextFilter = "all"
	FilterCustom  ContextFilter = "custom"
	FilterProject ContextFilter = "project"
	FilterGlobal  ContextFilter = "global"
	FilterSystem  ContextFilter = "system"
	FilterManaged ContextFilter = "managed"
)

type QueryTarget string

const (
	QuerySources  QueryTarget = "sources"
	QueryProblems QueryTarget = "problems"
)

type Completeness string

const (
	Complete   Completeness = "complete"
	Incomplete Completeness = "incomplete"
)

type RootState string

const (
	RootSearched   RootState = "searched"
	RootEmpty      RootState = "empty"
	RootMissing    RootState = "missing"
	RootUnreadable RootState = "unreadable"
	RootIncomplete RootState = "incomplete"
)

type ProjectContext struct {
	Selected bool   `json:"selected"`
	Path     string `json:"path,omitempty"`
}

type DiscoveryRoot struct {
	ID                  RootID           `json:"id"`
	Path                string           `json:"path"`
	Context             ReferenceContext `json:"context"`
	Origin              string           `json:"origin,omitempty"`
	Optional            bool             `json:"-"`
	piMode              string
	piFile              bool
	childrenOnly        bool
	inspectRootManifest bool
}

type Request struct {
	Project       ProjectContext
	Roots         []DiscoveryRoot
	Agent         string
	home          string
	codexHome     string
	codeafProfile string
	claudeHome    string
	hermesHome    string
	// Private read-only prospective evaluation; never retained in a persisted change plan.
	opencodePlanning  bool
	opencodeOverlay   map[string][]byte
	piPlanning        bool
	piSettingsOverlay map[string][]byte
}

func NewRequest(rawRoots, rawProjectRoots []string, rawProject *string) (Request, error) {
	return NewRequestWithAgent(rawRoots, rawProjectRoots, rawProject, "")
}

func NewRequestWithAgent(rawRoots, rawProjectRoots []string, rawProject *string, agent string) (Request, error) {
	if agent != "" && agent != "codex" && agent != "claude-code" && agent != "pi" && agent != "opencode" && agent != "codeaf" && agent != "gemini" && agent != "hermes" {
		return Request{}, fmt.Errorf("unknown agent %q; use codex, claude-code, pi, opencode, codeaf, gemini, or hermes", agent)
	}
	request := Request{Roots: make([]DiscoveryRoot, 0, len(rawRoots)+len(rawProjectRoots))}
	if rawProject != nil {
		if *rawProject == "" {
			return Request{}, errors.New("--project cannot be empty")
		}
		path, err := filepath.Abs(*rawProject)
		if err != nil {
			return Request{}, fmt.Errorf("normalize project: %w", err)
		}
		request.Project = ProjectContext{Selected: true, Path: filepath.Clean(path)}
	}
	if len(rawProjectRoots) > 0 && !request.Project.Selected {
		return Request{}, errors.New("--project-root requires --project")
	}
	if len(rawRoots)+len(rawProjectRoots) == 0 && agent == "" {
		return Request{}, errors.New("at least one --root or --project-root is required")
	}
	appendRoots := func(rawValues []string, context ReferenceContext, flag string) error {
		for index, raw := range rawValues {
			if raw == "" {
				return fmt.Errorf("--%s %d is empty", flag, index+1)
			}
			path, err := filepath.Abs(raw)
			if err != nil {
				return fmt.Errorf("normalize %s %d: %w", flag, index+1, err)
			}
			request.Roots = append(request.Roots, DiscoveryRoot{
				ID:      RootID(fmt.Sprintf("root-%s-%d", context, index+1)),
				Path:    filepath.Clean(path),
				Context: context,
			})
		}
		return nil
	}
	if err := appendRoots(rawRoots, ContextCustom, "root"); err != nil {
		return Request{}, err
	}
	if err := appendRoots(rawProjectRoots, ContextProject, "project-root"); err != nil {
		return Request{}, err
	}
	request.Agent = agent
	if agent != "" && agent != "codeaf" {
		var err error
		if agent == "gemini" {
			request.home, err = geminiHome()
		} else {
			request.home, err = nativeHome()
		}
		if err != nil {
			return Request{}, err
		}
		if agent == "codex" {
			request.codexHome = codexHome(request.home)
		}
		if agent == "hermes" {
			request.hermesHome = hermesBaseHome(request.home)
		}
		if agent == "claude-code" {
			request.claudeHome = claudeHome(request.home)
		}
	}
	if agent == "codeaf" {
		var err error
		request.home, request.codeafProfile, err = codeafEnvironment()
		if err != nil {
			return Request{}, err
		}
	}
	return request, nil
}

type ProblemCode string

const (
	ProblemRootMissing             ProblemCode = "root-missing"
	ProblemRootUnreadable          ProblemCode = "root-unreadable"
	ProblemBrokenReference         ProblemCode = "broken-reference"
	ProblemSymlinkCycle            ProblemCode = "symlink-cycle"
	ProblemUnreadableManifest      ProblemCode = "unreadable-manifest"
	ProblemMalformedManifest       ProblemCode = "malformed-manifest"
	ProblemMetadataNonFinite       ProblemCode = "metadata-non-finite-float"
	ProblemManifestTooLarge        ProblemCode = "manifest-too-large"
	ProblemManifestInvalidEncoding ProblemCode = "manifest-invalid-encoding"
	ProblemUnreadableResource      ProblemCode = "unreadable-resource"
	ProblemResourceLimit           ProblemCode = "resource-limit"
	ProblemIdentityUncertain       ProblemCode = "identity-uncertain"
	ProblemDuplicateName           ProblemCode = "duplicate-name"
	ProblemSharedManifest          ProblemCode = "shared-manifest"
	ProblemNativeInput             ProblemCode = "native-input"
)

var documentedProblemCodes = []ProblemCode{
	ProblemRootMissing,
	ProblemRootUnreadable,
	ProblemBrokenReference,
	ProblemSymlinkCycle,
	ProblemUnreadableManifest,
	ProblemMalformedManifest,
	ProblemMetadataNonFinite,
	ProblemManifestTooLarge,
	ProblemManifestInvalidEncoding,
	ProblemUnreadableResource,
	ProblemResourceLimit,
	ProblemIdentityUncertain,
	ProblemDuplicateName,
	ProblemSharedManifest,
	ProblemNativeInput,
}

func ProblemCodes() []ProblemCode {
	return append([]ProblemCode(nil), documentedProblemCodes...)
}

func ParseProblemCode(value string) (ProblemCode, error) {
	if value == "" {
		return "", nil
	}
	for _, code := range documentedProblemCodes {
		if string(code) == value {
			return code, nil
		}
	}
	return "", fmt.Errorf("unknown problem code %q", value)
}

type Problem struct {
	nativeLocal    bool
	ID             ProblemID     `json:"id"`
	Code           ProblemCode   `json:"code"`
	Path           string        `json:"path"`
	Summary        string        `json:"summary"`
	Detail         string        `json:"detail,omitempty"`
	LimitsCoverage bool          `json:"limits_coverage"`
	RootIDs        []RootID      `json:"root_ids,omitempty"`
	ReferenceIDs   []ReferenceID `json:"reference_ids,omitempty"`
	SourceIDs      []SourceID    `json:"source_ids,omitempty"`
}

type RootReport struct {
	ID           RootID           `json:"id"`
	Requested    string           `json:"requested_path"`
	Context      ReferenceContext `json:"context"`
	Resolved     string           `json:"resolved_path,omitempty"`
	State        RootState        `json:"state"`
	SourceCount  int              `json:"source_count"`
	ProblemCount int              `json:"problem_count"`
	Origin       string           `json:"origin,omitempty"`
	Evidence     *Evidence        `json:"evidence,omitempty"`
}

type Scope struct {
	Project      ProjectContext `json:"project"`
	Roots        []RootReport   `json:"roots"`
	Completeness Completeness   `json:"completeness"`
	Agent        *AgentScope    `json:"agent,omitempty"`
}

type Reference struct {
	ID             ReferenceID      `json:"id"`
	Root           RootID           `json:"root_id"`
	Context        ReferenceContext `json:"context"`
	DiscoveredPath string           `json:"discovered_path"`
	ResolvedPath   string           `json:"resolved_path,omitempty"`
	Status         string           `json:"status"`
	Source         SourceID         `json:"source_id,omitempty"`
	Problems       []Problem        `json:"problems"`
	MatchesFilters bool             `json:"matches_filters"`
}

type Identity struct {
	Method    string `json:"method"`
	Certainty string `json:"certainty"`
}

type EvidenceState struct {
	State  string `json:"state"`
	Reason string `json:"reason"`
}

type ManifestStatus string

const (
	ManifestParsed     ManifestStatus = "parsed"
	ManifestMalformed  ManifestStatus = "malformed"
	ManifestUnreadable ManifestStatus = "unreadable"
)

type Manifest struct {
	Status           ManifestStatus `json:"status"`
	Path             string         `json:"path"`
	ResolvedPath     string         `json:"resolved_path,omitempty"`
	IsSymlink        bool           `json:"is_symlink"`
	RawSymlinkTarget string         `json:"raw_symlink_target,omitempty"`
	Name             string         `json:"name,omitempty"`
	Description      string         `json:"description,omitempty"`
	Metadata         map[string]any `json:"metadata,omitempty"`
	RawFrontmatter   string         `json:"raw_frontmatter,omitempty"`
	Instructions     string         `json:"instructions,omitempty"`
	RawText          string         `json:"raw_text"`
	RawBytesBase64   string         `json:"raw_bytes_base64,omitempty"`
}

func (manifest Manifest) MarshalJSON() ([]byte, error) {
	type manifestFields Manifest
	output := struct {
		manifestFields
		RawText *string `json:"raw_text,omitempty"`
	}{manifestFields: manifestFields(manifest)}
	if manifest.RawBytesBase64 == "" {
		output.RawText = &manifest.RawText
	}
	var encoded bytes.Buffer
	encoder := json.NewEncoder(&encoded)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(output); err != nil {
		return nil, err
	}
	return encoded.Bytes(), nil
}

type Resource struct {
	Path       string `json:"path"`
	Kind       string `json:"kind"`
	Mode       string `json:"mode"`
	Size       int64  `json:"size,omitempty"`
	Executable bool   `json:"executable,omitempty"`
	LinkTarget string `json:"link_target,omitempty"`
	LinkScope  string `json:"link_scope,omitempty"`
}

type Source struct {
	ID           SourceID                `json:"id"`
	Identity     Identity                `json:"identity"`
	Directory    string                  `json:"source_path"`
	Name         string                  `json:"name,omitempty"`
	Description  string                  `json:"description,omitempty"`
	Manifest     Manifest                `json:"manifest"`
	Resources    []Resource              `json:"resources"`
	References   []ReferenceID           `json:"reference_ids"`
	Problems     []Problem               `json:"problems"`
	Ownership    EvidenceState           `json:"ownership"`
	Availability AvailabilityExplanation `json:"availability"`
}

type View struct {
	Filters    Filters     `json:"filters"`
	Scope      Scope       `json:"scope"`
	Sources    []Source    `json:"sources"`
	References []Reference `json:"references"`
	Problems   []Problem   `json:"problems"`
}

type Filters struct {
	Search       string            `json:"search,omitempty"`
	SourceRoot   string            `json:"source_root,omitempty"`
	Context      ContextFilter     `json:"context"`
	Problem      ProblemCode       `json:"problem,omitempty"`
	Availability AvailabilityState `json:"availability,omitempty"`
}

type SelectionChangeState string

const (
	SelectionChangeNone     SelectionChangeState = "none"
	SelectionChangeRetained SelectionChangeState = "retained"
	SelectionChangeCleared  SelectionChangeState = "cleared"
)

type SelectionChange struct {
	State  SelectionChangeState `json:"state"`
	Reason string               `json:"reason,omitempty"`
}

type Snapshot struct {
	request          Request
	scope            Scope
	sources          map[SourceID]Source
	order            []SourceID
	references       map[ReferenceID]Reference
	referenceOrder   []ReferenceID
	problems         []Problem
	hermesControls   hermesControlFacts
	piControls       *piInputs
	opencodeControls *opencodeInputs
}

func Build(ctx context.Context, request Request) (*Snapshot, error) {
	if request.Agent != "" && request.Agent != "codex" && request.Agent != "claude-code" && request.Agent != "pi" && request.Agent != "opencode" && request.Agent != "codeaf" && request.Agent != "gemini" && request.Agent != "hermes" {
		return nil, fmt.Errorf("unknown agent %q", request.Agent)
	}
	if (request.Agent != "" && request.home == "") || (request.Agent == "codex" && request.codexHome == "") {
		return nil, errors.New("native scope requires NewRequestWithAgent")
	}
	if len(request.Roots) == 0 && request.Agent == "" {
		return nil, errors.New("collection request has no roots")
	}
	for _, root := range request.Roots {
		if !filepath.IsAbs(root.Path) {
			return nil, fmt.Errorf("root %s is not absolute", root.ID)
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if request.Agent == "opencode" {
		return buildOpencode(ctx, request)
	}
	if request.Agent == "codex" {
		return buildCodex(ctx, request)
	}
	if request.Agent == "codeaf" {
		return buildCodeAF(ctx, request)
	}
	if request.Agent == "claude-code" {
		return buildClaude(ctx, request)
	}
	if request.Agent == "pi" {
		return buildPi(ctx, request)
	}
	if request.Agent == "gemini" {
		return buildGemini(ctx, request)
	}
	if request.Agent == "hermes" {
		if request.hermesHome == "" {
			return nil, errors.New("native scope requires NewRequestWithAgent")
		}
		return buildHermes(ctx, request)
	}
	return discover(ctx, request)
}

func (snapshot *Snapshot) Scope() Scope {
	return cloneScope(snapshot.scope)
}

func (snapshot *Snapshot) Complete() bool {
	return snapshot.scope.Completeness == Complete
}

func (snapshot *Snapshot) Problems() []Problem {
	return cloneProblems(snapshot.problems)
}

func (snapshot *Snapshot) NewFilters(search, rawSourceRoot, rawContext, rawProblem string) (Filters, error) {
	filters := Filters{Search: search, Context: FilterAll}
	if rawContext != "" {
		filters.Context = ContextFilter(rawContext)
	}
	switch filters.Context {
	case FilterAll, FilterCustom, FilterProject, FilterGlobal, FilterSystem, FilterManaged:
	default:
		return Filters{}, fmt.Errorf("unknown context %q; use all, project, custom, global, system, or managed", rawContext)
	}
	problem, err := ParseProblemCode(rawProblem)
	if err != nil {
		return Filters{}, err
	}
	filters.Problem = problem
	if rawSourceRoot == "" {
		return filters, nil
	}
	path, err := filepath.Abs(rawSourceRoot)
	if err != nil {
		return Filters{}, fmt.Errorf("normalize source root: %w", err)
	}
	filters.SourceRoot = filepath.Clean(path)
	for _, root := range snapshot.scope.Roots {
		if root.Requested == filters.SourceRoot {
			return filters, nil
		}
	}
	return Filters{}, fmt.Errorf("--source-root %q is not one of the selected roots", filters.SourceRoot)
}

func (snapshot *Snapshot) Query(filters Filters, target QueryTarget) View {
	needle := strings.ToLower(strings.TrimSpace(filters.Search))
	matchingReferences := make(map[ReferenceID]bool, len(snapshot.referenceOrder))
	for _, id := range snapshot.referenceOrder {
		matchingReferences[id] = snapshot.referenceMatches(snapshot.references[id], filters)
	}
	sources := make([]Source, 0, len(snapshot.order))
	selected := make(map[SourceID]struct{}, len(snapshot.order))
	for _, id := range snapshot.order {
		source := snapshot.sources[id]
		if filters.Availability != "" && source.Availability.State != filters.Availability {
			continue
		}
		if !sourceHasMatchingReference(source, matchingReferences) {
			continue
		}
		if target == QuerySources && needle != "" &&
			!strings.Contains(strings.ToLower(source.Name), needle) &&
			!strings.Contains(strings.ToLower(source.Description), needle) {
			continue
		}
		if filters.Problem != "" && !snapshot.sourceHasProblem(source, filters, matchingReferences) {
			continue
		}
		sources = append(sources, cloneSource(source))
		selected[id] = struct{}{}
	}
	problems := make([]Problem, 0, len(snapshot.problems))
	problemReferences := make(map[ReferenceID]struct{})
	for _, problem := range snapshot.problems {
		if !snapshot.problemMatches(problem, filters, matchingReferences) {
			continue
		}
		if target == QueryProblems && needle != "" && !snapshot.problemContains(problem, needle) {
			continue
		}
		problems = append(problems, cloneProblem(problem))
		for _, id := range problem.ReferenceIDs {
			problemReferences[id] = struct{}{}
		}
	}
	references := make([]Reference, 0, len(snapshot.referenceOrder))
	for _, id := range snapshot.referenceOrder {
		reference := snapshot.references[id]
		_, selectedSource := selected[reference.Source]
		_, selectedProblem := problemReferences[id]
		if !selectedSource && !selectedProblem {
			continue
		}
		cloned := cloneReference(reference)
		cloned.MatchesFilters = matchingReferences[id]
		references = append(references, cloned)
	}
	return View{
		Filters:    filters,
		Scope:      cloneScope(snapshot.scope),
		Sources:    sources,
		References: references,
		Problems:   problems,
	}
}

func (snapshot *Snapshot) Refresh(ctx context.Context, selected SourceID) (*Snapshot, SelectionChange, error) {
	refreshed, err := Build(ctx, cloneRequest(snapshot.request))
	if err != nil {
		return nil, SelectionChange{}, err
	}
	if selected == "" {
		return refreshed, SelectionChange{State: SelectionChangeNone}, nil
	}
	previous, previousOK := snapshot.sources[selected]
	current, currentOK := refreshed.sources[selected]
	objectKind := "directory"
	if previousOK && previous.Identity.Method == "darwin-file-object" {
		objectKind = "file"
	}
	if previousOK && currentOK &&
		previous.Identity.Certainty == "proven" && current.Identity.Certainty == "proven" {
		return refreshed, SelectionChange{State: SelectionChangeRetained, Reason: "same proven " + objectKind + " object remains reachable"}, nil
	}
	return refreshed, SelectionChange{State: SelectionChangeCleared, Reason: "selected " + objectKind + " object is no longer provably reachable"}, nil
}

func (snapshot *Snapshot) Source(id SourceID) (Source, bool) {
	source, ok := snapshot.sources[id]
	if !ok {
		return Source{}, false
	}
	return cloneSource(source), true
}

func (snapshot *Snapshot) References(id SourceID) []Reference {
	return snapshot.ReferencesMatchingFilters(id, Filters{Context: FilterAll})
}

func (snapshot *Snapshot) ReferencesMatchingFilters(id SourceID, filters Filters) []Reference {
	source, ok := snapshot.sources[id]
	if !ok {
		return nil
	}
	references := make([]Reference, 0, len(source.References))
	for _, referenceID := range source.References {
		reference := cloneReference(snapshot.references[referenceID])
		reference.MatchesFilters = snapshot.referenceMatches(reference, filters)
		references = append(references, reference)
	}
	return references
}

func sortedSourceIDs(sources map[SourceID]Source) []SourceID {
	ids := make([]SourceID, 0, len(sources))
	for id := range sources {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(left, right int) bool {
		leftSource := sources[ids[left]]
		rightSource := sources[ids[right]]
		leftName := strings.ToLower(leftSource.Name)
		rightName := strings.ToLower(rightSource.Name)
		if leftName != rightName {
			return leftName < rightName
		}
		return ids[left] < ids[right]
	})
	return ids
}

func cloneScope(scope Scope) Scope {
	scope.Roots = append([]RootReport(nil), scope.Roots...)
	for i := range scope.Roots {
		if scope.Roots[i].Evidence != nil {
			copy := *scope.Roots[i].Evidence
			scope.Roots[i].Evidence = &copy
		}
	}
	if scope.Agent != nil {
		copy := *scope.Agent
		copy.ConfigLayers = cloneLayers(copy.ConfigLayers)
		copy.Gaps = append([]string(nil), copy.Gaps...)
		scope.Agent = &copy
	}
	return scope
}

func cloneSource(source Source) Source {
	source.Availability = cloneAvailability(source.Availability)
	source.References = append([]ReferenceID(nil), source.References...)
	source.Resources = append([]Resource(nil), source.Resources...)
	source.Problems = cloneProblems(source.Problems)
	source.Manifest.Metadata = cloneMap(source.Manifest.Metadata)
	return source
}

func cloneReference(reference Reference) Reference {
	reference.Problems = cloneProblems(reference.Problems)
	return reference
}

func cloneProblem(problem Problem) Problem {
	problem.RootIDs = append([]RootID(nil), problem.RootIDs...)
	problem.ReferenceIDs = append([]ReferenceID(nil), problem.ReferenceIDs...)
	problem.SourceIDs = append([]SourceID(nil), problem.SourceIDs...)
	return problem
}

func cloneProblems(problems []Problem) []Problem {
	cloned := make([]Problem, len(problems))
	for index, problem := range problems {
		cloned[index] = cloneProblem(problem)
	}
	return cloned
}

func cloneRequest(request Request) Request {
	request.Roots = append([]DiscoveryRoot(nil), request.Roots...)
	if request.opencodeOverlay != nil {
		overlay := make(map[string][]byte, len(request.opencodeOverlay))
		for path, raw := range request.opencodeOverlay {
			overlay[path] = append([]byte(nil), raw...)
		}
		request.opencodeOverlay = overlay
	}
	return request
}

func (snapshot *Snapshot) referenceMatches(reference Reference, filters Filters) bool {
	if filters.Context != FilterAll && string(reference.Context) != string(filters.Context) {
		return false
	}
	if filters.SourceRoot == "" {
		return true
	}
	for _, root := range snapshot.scope.Roots {
		if root.ID == reference.Root {
			return root.Requested == filters.SourceRoot
		}
	}
	return false
}

func sourceHasMatchingReference(source Source, matching map[ReferenceID]bool) bool {
	for _, id := range source.References {
		if matching[id] {
			return true
		}
	}
	return false
}

func (snapshot *Snapshot) sourceHasProblem(source Source, filters Filters, matchingReferences map[ReferenceID]bool) bool {
	for _, problem := range source.Problems {
		if snapshot.problemMatches(problem, filters, matchingReferences) {
			return true
		}
	}
	return false
}

func (snapshot *Snapshot) problemMatches(
	problem Problem,
	filters Filters,
	matchingReferences map[ReferenceID]bool,
) bool {
	if filters.Problem != "" && problem.Code != filters.Problem {
		return false
	}
	if filters.Availability != "" && len(problem.SourceIDs) > 0 {
		matches := false
		for _, id := range problem.SourceIDs {
			source := snapshot.sources[id]
			if source.Availability.State == filters.Availability && sourceHasMatchingReference(source, matchingReferences) {
				matches = true
				break
			}
		}
		if !matches {
			return false
		}
	}
	if filters.Context == FilterAll && filters.SourceRoot == "" {
		return true
	}
	for _, id := range problem.ReferenceIDs {
		if matchingReferences[id] {
			return true
		}
	}
	for _, rootID := range problem.RootIDs {
		for _, root := range snapshot.scope.Roots {
			if root.ID != rootID {
				continue
			}
			if filters.SourceRoot != "" && root.Requested != filters.SourceRoot {
				continue
			}
			if filters.Context != FilterAll && string(root.Context) != string(filters.Context) {
				continue
			}
			return true
		}
	}
	return false
}

func (snapshot *Snapshot) problemContains(problem Problem, needle string) bool {
	if strings.Contains(strings.ToLower(string(problem.Code)), needle) ||
		strings.Contains(strings.ToLower(problem.Path), needle) ||
		strings.Contains(strings.ToLower(problem.Summary), needle) ||
		strings.Contains(strings.ToLower(problem.Detail), needle) {
		return true
	}
	for _, id := range problem.ReferenceIDs {
		if strings.Contains(strings.ToLower(snapshot.references[id].DiscoveredPath), needle) {
			return true
		}
	}
	for _, id := range problem.RootIDs {
		for _, root := range snapshot.scope.Roots {
			if root.ID == id && strings.Contains(strings.ToLower(root.Requested), needle) {
				return true
			}
		}
	}
	return false
}

func cloneMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	output := make(map[string]any, len(input))
	for key, value := range input {
		output[key] = cloneValue(value)
	}
	return output
}

func cloneValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneMap(typed)
	case []any:
		result := make([]any, len(typed))
		for index := range typed {
			result[index] = cloneValue(typed[index])
		}
		return result
	default:
		return value
	}
}
