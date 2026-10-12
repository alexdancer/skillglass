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
	ID   RootID `json:"id"`
	Path string `json:"path"`
}

type Request struct {
	Project ProjectContext
	Roots   []DiscoveryRoot
}

func NewRequest(rawRoots []string, rawProject *string) (Request, error) {
	if len(rawRoots) == 0 {
		return Request{}, errors.New("at least one --root is required")
	}

	request := Request{Roots: make([]DiscoveryRoot, 0, len(rawRoots))}
	for index, raw := range rawRoots {
		if raw == "" {
			return Request{}, fmt.Errorf("--root %d is empty", index+1)
		}
		path, err := filepath.Abs(raw)
		if err != nil {
			return Request{}, fmt.Errorf("normalize root %d: %w", index+1, err)
		}
		request.Roots = append(request.Roots, DiscoveryRoot{
			ID:   RootID(fmt.Sprintf("root-%d", index+1)),
			Path: filepath.Clean(path),
		})
	}

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
)

type Problem struct {
	ID             ProblemID   `json:"id"`
	Code           ProblemCode `json:"code"`
	Path           string      `json:"path"`
	Summary        string      `json:"summary"`
	Detail         string      `json:"detail,omitempty"`
	LimitsCoverage bool        `json:"limits_coverage"`
}

type RootReport struct {
	ID           RootID    `json:"id"`
	Requested    string    `json:"requested_path"`
	Resolved     string    `json:"resolved_path,omitempty"`
	State        RootState `json:"state"`
	SourceCount  int       `json:"source_count"`
	ProblemCount int       `json:"problem_count"`
}

type Scope struct {
	Project      ProjectContext `json:"project"`
	Roots        []RootReport   `json:"roots"`
	Completeness Completeness   `json:"completeness"`
}

type Reference struct {
	ID             ReferenceID `json:"id"`
	Root           RootID      `json:"root_id"`
	DiscoveredPath string      `json:"discovered_path"`
	ResolvedPath   string      `json:"resolved_path,omitempty"`
	Status         string      `json:"status"`
	Source         SourceID    `json:"source_id,omitempty"`
	Problems       []Problem   `json:"problems"`
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
	ID           SourceID      `json:"id"`
	Identity     Identity      `json:"identity"`
	Directory    string        `json:"source_path"`
	Name         string        `json:"name,omitempty"`
	Description  string        `json:"description,omitempty"`
	Manifest     Manifest      `json:"manifest"`
	Resources    []Resource    `json:"resources"`
	References   []ReferenceID `json:"reference_ids"`
	Problems     []Problem     `json:"problems"`
	Ownership    EvidenceState `json:"ownership"`
	Availability EvidenceState `json:"availability"`
}

type View struct {
	Scope      Scope       `json:"scope"`
	Sources    []Source    `json:"sources"`
	References []Reference `json:"references"`
	Problems   []Problem   `json:"problems"`
}

type Snapshot struct {
	scope          Scope
	sources        map[SourceID]Source
	order          []SourceID
	references     map[ReferenceID]Reference
	referenceOrder []ReferenceID
	problems       []Problem
}

func Build(ctx context.Context, request Request) (*Snapshot, error) {
	if len(request.Roots) == 0 {
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
	return discover(ctx, request)
}

func (snapshot *Snapshot) Scope() Scope {
	return cloneScope(snapshot.scope)
}

func (snapshot *Snapshot) Complete() bool {
	return snapshot.scope.Completeness == Complete
}

func (snapshot *Snapshot) Problems() []Problem {
	return append([]Problem(nil), snapshot.problems...)
}

func (snapshot *Snapshot) Query(text string) View {
	needle := strings.ToLower(strings.TrimSpace(text))
	sources := make([]Source, 0, len(snapshot.order))
	selected := make(map[SourceID]struct{}, len(snapshot.order))
	for _, id := range snapshot.order {
		source := snapshot.sources[id]
		if needle != "" &&
			!strings.Contains(strings.ToLower(source.Name), needle) &&
			!strings.Contains(strings.ToLower(source.Description), needle) {
			continue
		}
		sources = append(sources, cloneSource(source))
		selected[id] = struct{}{}
	}
	references := make([]Reference, 0, len(snapshot.referenceOrder))
	for _, id := range snapshot.referenceOrder {
		reference := snapshot.references[id]
		if reference.Source != "" {
			if _, ok := selected[reference.Source]; !ok {
				continue
			}
		} else if needle != "" {
			continue
		}
		references = append(references, cloneReference(reference))
	}
	return View{
		Scope:      cloneScope(snapshot.scope),
		Sources:    sources,
		References: references,
		Problems:   append([]Problem(nil), snapshot.problems...),
	}
}

func (snapshot *Snapshot) Source(id SourceID) (Source, bool) {
	source, ok := snapshot.sources[id]
	if !ok {
		return Source{}, false
	}
	return cloneSource(source), true
}

func (snapshot *Snapshot) References(id SourceID) []Reference {
	source, ok := snapshot.sources[id]
	if !ok {
		return nil
	}
	references := make([]Reference, 0, len(source.References))
	for _, referenceID := range source.References {
		references = append(references, cloneReference(snapshot.references[referenceID]))
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
	return scope
}

func cloneSource(source Source) Source {
	source.References = append([]ReferenceID(nil), source.References...)
	source.Resources = append([]Resource(nil), source.Resources...)
	source.Problems = append([]Problem(nil), source.Problems...)
	source.Manifest.Metadata = cloneMap(source.Manifest.Metadata)
	return source
}

func cloneReference(reference Reference) Reference {
	reference.Problems = append([]Problem(nil), reference.Problems...)
	return reference
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
