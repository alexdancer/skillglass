package terminal

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/alexdancer/skillglass/internal/collection"
)

type listEnvelope struct {
	Operation  string                 `json:"operation"`
	Filters    collection.Filters     `json:"filters"`
	Scope      collection.Scope       `json:"scope"`
	Sources    []collection.Source    `json:"sources"`
	References []collection.Reference `json:"references"`
	Problems   []collection.Problem   `json:"problems"`
}

type readEnvelope struct {
	Operation  string                 `json:"operation"`
	Scope      collection.Scope       `json:"scope"`
	Source     *collection.Source     `json:"source,omitempty"`
	References []collection.Reference `json:"references"`
	Problems   []collection.Problem   `json:"problems"`
	Error      string                 `json:"error,omitempty"`
}

func encodeListJSON(snapshot *collection.Snapshot, filters collection.Filters) ([]byte, error) {
	view := snapshot.Query(filters, collection.QuerySources)
	return encodeJSON(listEnvelope{
		Operation:  "list",
		Filters:    view.Filters,
		Scope:      view.Scope,
		Sources:    view.Sources,
		References: view.References,
		Problems:   view.Problems,
	})
}

func encodeProblemsJSON(snapshot *collection.Snapshot, filters collection.Filters) ([]byte, error) {
	view := snapshot.Query(filters, collection.QueryProblems)
	return encodeJSON(listEnvelope{
		Operation:  "problems",
		Filters:    view.Filters,
		Scope:      view.Scope,
		Sources:    view.Sources,
		References: view.References,
		Problems:   view.Problems,
	})
}

func encodeReadJSON(snapshot *collection.Snapshot, id collection.SourceID) ([]byte, error) {
	source, ok := snapshot.Source(id)
	envelope := readEnvelope{
		Operation: "read",
		Scope:     snapshot.Scope(),
		Problems:  snapshot.Problems(),
	}
	if !ok {
		envelope.Error = fmt.Sprintf("source %q was not found", id)
		return encodeJSON(envelope)
	}
	envelope.Source = &source
	envelope.References = snapshot.References(id)
	return encodeJSON(envelope)
}

func encodeJSON(value any) ([]byte, error) {
	var output bytes.Buffer
	encoder := json.NewEncoder(&output)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func renderList(snapshot *collection.Snapshot, filters collection.Filters) string {
	view := snapshot.Query(filters, collection.QuerySources)
	var output strings.Builder
	output.WriteString("Skillglass collection\n")
	renderScope(&output, view.Scope)
	renderFilters(&output, view.Filters)
	output.WriteString("\n")
	renderProblems(&output, view.Problems)
	if len(view.Sources) == 0 {
		if filters.Search == "" && filters.SourceRoot == "" && filters.Context == collection.FilterAll && filters.Problem == "" {
			output.WriteString("Sources: none found\n")
		} else {
			output.WriteString("Sources: no matches\n")
		}
		return output.String()
	}

	referencesBySource := make(map[collection.SourceID][]collection.Reference)
	for _, reference := range view.References {
		referencesBySource[reference.Source] = append(referencesBySource[reference.Source], reference)
	}
	fmt.Fprintf(&output, "Sources: %d\n", len(view.Sources))
	for _, source := range view.Sources {
		name := source.Name
		if name == "" {
			name = "<metadata unavailable>"
		}
		fmt.Fprintf(&output, "\n[%s] %s\n", safeInline(string(source.ID)), safeInline(name))
		fmt.Fprintf(&output, "  Description: %s\n", displayOptional(source.Description))
		fmt.Fprintf(&output, "  Source: %s\n", safeInline(source.Directory))
		fmt.Fprintf(&output, "  Manifest: %s\n", source.Manifest.Status)
		fmt.Fprintf(&output, "  Ownership: %s\n", source.Ownership.State)
		fmt.Fprintf(&output, "  Availability: %s\n", source.Availability.State)
		fmt.Fprintf(&output, "  Resources: %d\n", len(source.Resources))
		for _, reference := range referencesBySource[source.ID] {
			matching := ""
			if reference.MatchesFilters {
				matching = " [matches filters]"
			}
			fmt.Fprintf(
				&output,
				"  Discovered (%s): %s%s\n",
				reference.Context,
				safeInline(reference.DiscoveredPath),
				matching,
			)
		}
		if source.Manifest.Status == collection.ManifestParsed {
			fmt.Fprintf(&output, "  Metadata: %s\n", safeInline(compactMetadata(source.Manifest.Metadata)))
		}
	}
	return output.String()
}

func renderProblemList(snapshot *collection.Snapshot, filters collection.Filters) string {
	view := snapshot.Query(filters, collection.QueryProblems)
	var output strings.Builder
	output.WriteString("Skillglass problems\n")
	renderScope(&output, view.Scope)
	renderFilters(&output, view.Filters)
	output.WriteString("\n")
	if len(view.Problems) == 0 {
		output.WriteString("Problems: none found\n")
		return output.String()
	}
	renderProblems(&output, view.Problems)
	return output.String()
}

func renderRead(snapshot *collection.Snapshot, id collection.SourceID) (string, bool) {
	var output strings.Builder
	output.WriteString("Skillglass source\n")
	renderScope(&output, snapshot.Scope())
	output.WriteString("\n")
	renderProblems(&output, snapshot.Problems())

	source, ok := snapshot.Source(id)
	if !ok {
		fmt.Fprintf(&output, "Selection failed: source %q was not found.\n", safeInline(string(id)))
		return output.String(), false
	}

	name := source.Name
	if name == "" {
		name = "<metadata unavailable>"
	}
	fmt.Fprintf(&output, "ID: %s\n", safeInline(string(source.ID)))
	fmt.Fprintf(&output, "Name: %s\n", safeInline(name))
	fmt.Fprintf(&output, "Description: %s\n", displayOptional(source.Description))
	fmt.Fprintf(&output, "Source: %s\n", safeInline(source.Directory))
	for _, reference := range snapshot.References(id) {
		fmt.Fprintf(&output, "Discovered (%s): %s\n", reference.Context, safeInline(reference.DiscoveredPath))
		if reference.ResolvedPath != "" && reference.ResolvedPath != reference.DiscoveredPath {
			fmt.Fprintf(&output, "Resolved reference: %s\n", safeInline(reference.ResolvedPath))
		}
	}
	fmt.Fprintf(&output, "Identity: %s (%s)\n", source.Identity.Method, source.Identity.Certainty)
	fmt.Fprintf(
		&output,
		"Ownership: %s (%s)\n",
		source.Ownership.State,
		safeInline(source.Ownership.Reason),
	)
	fmt.Fprintf(
		&output,
		"Availability: %s (%s)\n",
		source.Availability.State,
		safeInline(source.Availability.Reason),
	)
	fmt.Fprintf(&output, "Manifest: %s\n", source.Manifest.Status)
	if source.Manifest.IsSymlink {
		fmt.Fprintf(
			&output,
			"Manifest link: %s -> %s\n",
			safeInline(source.Manifest.Path),
			safeInline(source.Manifest.RawSymlinkTarget),
		)
	}
	if source.Manifest.Status == collection.ManifestParsed {
		output.WriteString("\nMetadata:\n")
		output.WriteString(indentBlock(source.Manifest.RawFrontmatter, "  "))
		output.WriteString("\n\nInstructions:\n")
		output.WriteString(indentBlock(source.Manifest.Instructions, "  "))
		output.WriteString("\n")
	} else if source.Manifest.RawText != "" {
		output.WriteString("\nRaw manifest:\n")
		output.WriteString(indentBlock(source.Manifest.RawText, "  "))
		output.WriteString("\n")
	}

	output.WriteString("\nResources:\n")
	if len(source.Resources) == 0 {
		output.WriteString("  none\n")
	}
	for _, resource := range source.Resources {
		fmt.Fprintf(
			&output,
			"  %s  %s  mode=%s",
			safeInline(resource.Path),
			resource.Kind,
			resource.Mode,
		)
		if resource.Kind == "file" {
			fmt.Fprintf(&output, "  size=%d", resource.Size)
			if resource.Executable {
				output.WriteString("  executable")
			}
		}
		if resource.Kind == "symlink" {
			fmt.Fprintf(
				&output,
				"  target=%s  scope=%s",
				safeInline(resource.LinkTarget),
				resource.LinkScope,
			)
		}
		output.WriteString("\n")
	}
	return output.String(), true
}

func renderScope(output *strings.Builder, scope collection.Scope) {
	if scope.Project.Selected {
		fmt.Fprintf(output, "Project: %s\n", safeInline(scope.Project.Path))
	} else {
		output.WriteString("Project: none selected\n")
	}
	fmt.Fprintf(output, "Coverage: %s\n", scope.Completeness)
	output.WriteString("Roots:\n")
	for _, root := range scope.Roots {
		fmt.Fprintf(
			output,
			"  [%s] [%s] %s\n",
			root.State,
			root.Context,
			safeInline(root.Requested),
		)
	}
}

func renderFilters(output *strings.Builder, filters collection.Filters) {
	search := displayOptional(filters.Search)
	sourceRoot := displayOptional(filters.SourceRoot)
	problem := displayOptional(string(filters.Problem))
	fmt.Fprintf(
		output,
		"Filters: search=%s | source-root=%s | context=%s | problem=%s\n",
		search,
		sourceRoot,
		filters.Context,
		problem,
	)
}

func renderProblems(output *strings.Builder, problems []collection.Problem) {
	if len(problems) == 0 {
		return
	}
	fmt.Fprintf(output, "Problems: %d\n", len(problems))
	for _, problem := range problems {
		fmt.Fprintf(
			output,
			"  %s at %s: %s",
			problem.Code,
			safeInline(problem.Path),
			safeInline(problem.Summary),
		)
		if problem.Detail != "" {
			fmt.Fprintf(output, " (%s)", safeInline(problem.Detail))
		}
		if len(problem.SourceIDs) > 0 {
			selectors := make([]string, len(problem.SourceIDs))
			for index, id := range problem.SourceIDs {
				selectors[index] = safeInline(string(id))
			}
			fmt.Fprintf(output, " [sources: %s]", strings.Join(selectors, ", "))
		}
		output.WriteString("\n")
	}
	output.WriteString("\n")
}

func displayOptional(value string) string {
	if value == "" {
		return "(none)"
	}
	return safeInline(value)
}

func compactMetadata(metadata map[string]any) string {
	if len(metadata) == 0 {
		return "{}"
	}
	keys := make([]string, 0, len(metadata))
	for key := range metadata {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	ordered := make(map[string]any, len(metadata))
	for _, key := range keys {
		ordered[key] = metadata[key]
	}
	encoded, err := json.Marshal(ordered)
	if err != nil {
		return "{unavailable}"
	}
	return string(encoded)
}
