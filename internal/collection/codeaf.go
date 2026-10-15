package collection

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"unicode/utf8"

	"go.yaml.in/yaml/v3"
)

const codeafValidatedVersion = "CodeAF source v0.7.1 / 7e303356 (installed +dirty build differs)"

var codeafSkillRoots = []string{".codeaf/skills", ".agents/skills", ".claude/skills", ".codex/skills", ".cursor/skills", ".gemini/skills", ".opencode/skills", ".goose/skills"}

type codeafRoot struct {
	DiscoveryRoot
	rank        int
	plugin      string
	version     string
	installRoot string
	permission  PermissionExplanation
}
type codeafInputs struct {
	scope           AgentScope
	roots           map[RootID]codeafRoot
	problems        []Problem
	enabled         map[string]bool
	settingsUnknown bool
}

func codeafEvidence(kind EvidenceKind, rule, path, setting, reason string) Evidence {
	return Evidence{Kind: kind, Rule: rule, InputPath: path, Setting: setting, Reason: reason, AgentVersion: codeafValidatedVersion}
}
func codeafEnvironment() (string, string, error) {
	login := strings.TrimSpace(codeafEnv("CODEAF_HOME"))
	state := login
	if login == "" {
		login = os.Getenv("HOME")
		if login == "" {
			return "", "", fmt.Errorf("--agent codeaf requires HOME or CODEAF_HOME to bound discovery")
		}
		state = filepath.Join(login, ".codeaf")
		if info, err := os.Stat(state); err != nil || !info.IsDir() {
			legacy := filepath.Join(login, ".aforge")
			if info, err := os.Stat(legacy); err == nil && info.IsDir() {
				state = legacy
			}
		}
	}
	login, err := filepath.Abs(login)
	if err != nil {
		return "", "", err
	}
	profile := strings.TrimSpace(codeafEnv("CODEAF_PROFILE_DIR"))
	if profile == "" {
		profile = state
	}
	profile, err = filepath.Abs(profile)
	return login, profile, err
}
func (inputs *codeafInputs) problem(path, reason string) {
	inputs.problems = append(inputs.problems, makeProblem(ProblemNativeInput, path, reason, nil, true))
}
func (inputs *codeafInputs) addRoot(request *Request, path string, context ReferenceContext, plugin, version string, permission PermissionExplanation, installRoot string) {
	root := DiscoveryRoot{ID: RootID(stableID("root_codeaf", fmt.Sprint(len(inputs.roots)), path)), Path: path, Context: context, Origin: "codeaf-" + string(context), Optional: true, childrenOnly: plugin == ""}
	if plugin != "" {
		_, err := os.Stat(filepath.Join(path, "SKILL.md"))
		root.childrenOnly = err != nil
		root.inspectRootManifest = err != nil
	}
	inputs.roots[root.ID] = codeafRoot{DiscoveryRoot: root, rank: len(inputs.roots), plugin: plugin, version: version, permission: permission, installRoot: installRoot}
	request.Roots = append(request.Roots, root)
}
func buildCodeAF(ctx context.Context, request Request) (*Snapshot, error) {
	original := cloneRequest(request)
	inputs := codeafInputs{roots: make(map[RootID]codeafRoot), enabled: make(map[string]bool)}
	inputs.scope = AgentScope{Application: "codeaf", ValidatedVersion: codeafValidatedVersion, VersionEvidence: "copied native loader probe validates pinned source files; installed v0.7.1+dirty binary build metadata was inspected, not its live discovery", Evidence: codeafEvidence(EvidenceObserved, "recorded-source-verification", "tools/verify_codeaf.py", "", "disposable source-loader and installed version receipts; see docs/verification/ale-39.md"), Gaps: []string{
		"The installed binary is a modified build. Source-loader verification does not establish binary/source equivalence or the behavior of other CodeAF builds.",
		"Existing conversation attachments, stored facts and model use are not inspected. Detachment is conversation state, not persistent per-skill disabling.",
		"Persistent CodeAF-only per-skill enable/disable and project overrides are unsupported. Changing another application's setting or deleting a shared source is not such a control.",
	}}
	inputs.scope.ConfigLayers = append(inputs.scope.ConfigLayers, PermissionLayer{Path: request.codeafProfile, Scope: "profile", State: "not-read", Reason: "CODEAF_PROFILE_DIR selects a profile, not the discovery login base; no persistent per-skill control is supported", Evidence: codeafEvidence(EvidenceObserved, "profile-environment", request.codeafProfile, "CODEAF_PROFILE_DIR", "profile contents are unnecessary for this read-only journey")})
	for _, path := range []string{request.home, request.Project.Path} {
		if path == "" {
			continue
		}
		info, err := os.Stat(path)
		if err != nil || !info.IsDir() {
			inputs.problem(path, "selected native base is not a readable directory")
		}
	}
	inputs.readSettings(request)
	userPlugins, projectPlugins := inputs.readPlugins(request)
	type base struct {
		path    string
		context ReferenceContext
	}
	var bases []base
	if request.Project.Selected {
		bases = append(bases, base{request.Project.Path, ContextProject})
	}
	folded := request.Project.Selected && request.Project.Path == request.home
	if !folded {
		bases = append(bases, base{request.home, ContextGlobal})
	}
	for _, base := range bases {
		permission := PermissionExplanation{State: PermissionAllowed, Evidence: codeafEvidence(EvidenceInferred, "ordinary-skill-no-toggle", base.path, "", "native loader does not consult another agent's per-skill settings")}
		for _, root := range codeafSkillRoots {
			inputs.addRoot(&request, filepath.Join(base.path, root), base.context, "", "", permission, "")
		}
		if base.path == request.home {
			for _, root := range []string{".config/opencode/skills", ".config/goose/skills"} {
				inputs.addRoot(&request, filepath.Join(base.path, root), base.context, "", "", permission, "")
			}
		}
		plugins := userPlugins
		if base.context == ContextProject {
			plugins = projectPlugins
			if folded {
				plugins = append(plugins, userPlugins...)
			}
		}
		for _, plugin := range plugins {
			for _, folder := range plugin.folders {
				inputs.addRoot(&request, folder, base.context, plugin.id, plugin.version, plugin.permission, plugin.installRoot)
			}
		}
		inputs.addRoot(&request, filepath.Join(base.path, ".codex/skills/.system"), base.context, "", "", permission, "")
	}
	snapshot, err := discover(ctx, request)
	if err != nil {
		return nil, err
	}
	snapshot.request = original
	snapshot.scope.Agent = &inputs.scope
	for i := range snapshot.scope.Roots {
		root := &snapshot.scope.Roots[i]
		if _, ok := inputs.roots[root.ID]; ok {
			e := codeafEvidence(EvidenceObserved, "direct-child-root", root.Requested, "", "scanner observed "+string(root.State)+"; only direct child skill folders are native")
			root.Evidence = &e
		}
	}
	type occurrence struct {
		ref    ReferenceID
		source SourceID
		name   string
		root   codeafRoot
	}
	var candidates []occurrence
	nativeNames := make(map[SourceID]string)
	nativeReasons := make(map[SourceID]string)
	for _, id := range snapshot.order {
		name, reason := codeafNativeName(snapshot.sources[id])
		nativeNames[id] = name
		nativeReasons[id] = reason
	}
	for _, rid := range snapshot.referenceOrder {
		ref := snapshot.references[rid]
		root, ok := inputs.roots[ref.Root]
		if !ok || root.permission.State == PermissionDenied || nativeReasons[ref.Source] != "" {
			continue
		}
		// Ordinary roots are containers only. A plugin may name a bundle itself.
		if root.plugin == "" && filepath.Clean(ref.DiscoveredPath) == root.Path {
			continue
		}
		candidates = append(candidates, occurrence{rid, ref.Source, nativeNames[ref.Source], root})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].root.rank != candidates[j].root.rank {
			return candidates[i].root.rank < candidates[j].root.rank
		}
		return snapshot.references[candidates[i].ref].DiscoveredPath < snapshot.references[candidates[j].ref].DiscoveredPath
	})
	winners := make(map[string]occurrence)
	groups := make(map[string][]SourceID)
	for _, candidate := range candidates {
		groups[candidate.name] = appendUniqueSourceIDs(groups[candidate.name], candidate.source)
		if _, ok := winners[candidate.name]; !ok && candidate.root.permission.State == PermissionAllowed && len(groups[candidate.name]) == 1 {
			winners[candidate.name] = candidate
		}
	}
	for _, id := range snapshot.order {
		source := snapshot.sources[id]
		name := nativeNames[id]
		evidence := codeafEvidence(EvidenceUnknown, "inspection-only", source.Directory, "", "no eligible native reference was established")
		value := AvailabilityExplanation{State: AvailabilityUnknown, Reason: evidence.Reason, Evidence: &evidence, NativeName: name}
		if name != "" {
			e := codeafEvidence(EvidenceObserved, "native-frontmatter-name", source.Manifest.Path, "name", "native 64 KiB head parser identifies the name separately from source/reference identity")
			value.NativeNameEvidence = &e
		}
		native, eligible, uncertain := false, false, false
		for _, rid := range source.References {
			ref := snapshot.references[rid]
			fact := ReferenceAvailability{Reference: rid, Participation: ParticipationInspection, Evidence: codeafEvidence(EvidenceUnknown, "inspection-only", ref.DiscoveredPath, "", "explicit roots are browseable but do not add a native loader root"), Permission: PermissionExplanation{State: PermissionUnknown, Evidence: evidence}, Provenance: Provenance{Kind: "custom", Evidence: evidence}}
			if root, ok := inputs.roots[ref.Root]; ok {
				native = true
				fact.Permission = root.permission
				fact.Participation = ParticipationExcluded
				fact.Provenance = Provenance{Kind: string(root.Context), Evidence: codeafEvidence(EvidenceObserved, "native-root", root.Path, "", "CodeAF reads this conventional location in place")}
				if root.plugin != "" {
					fact.Provenance = Provenance{Kind: "plugin", PluginID: root.plugin, Version: root.version, Evidence: codeafEvidence(EvidenceObserved, "claude-plugin-registration", root.Path, "", "registered Claude plugin ownership; its whole-plugin setting is not a CodeAF-only per-skill control")}
					if codeafInsideInstall(source.Directory, root.installRoot) {
						source.Ownership = EvidenceState{State: "managed", Reason: "source is inside the registered Claude plugin installation " + root.plugin}
					}
				}
				reason := nativeReasons[id]
				if root.plugin == "" && filepath.Clean(ref.DiscoveredPath) == root.Path {
					reason = "native ordinary roots load direct children, not a root manifest"
				}
				if reason == "" && root.permission.State == PermissionAllowed {
					if winner, ok := winners[name]; !ok {
						fact.Participation = ParticipationUnknown
						uncertain = true
						reason = "an earlier plugin's uncertain enablement prevents resolving this native name"
					} else if winner.ref == rid {
						fact.Participation = ParticipationEligible
						value.ReferenceID = rid
						eligible = true
						reason = "first loaded definition owns the native name"
					} else {
						reason = "an earlier native definition shadows this reference"
					}
				} else if root.permission.State == PermissionUnknown {
					fact.Participation = ParticipationUnknown
					uncertain = true
					reason = "plugin settings or registration cannot be reliably interpreted"
				} else if reason == "" {
					reason = "registered plugin is not enabled for this context"
				}
				fact.Evidence = codeafEvidence(EvidenceInferred, "codeaf-native-discovery", ref.DiscoveredPath, "", reason)
				if fact.Participation == ParticipationUnknown {
					fact.Evidence.Kind = EvidenceUnknown
				}
			}
			value.References = append(value.References, fact)
		}
		if native {
			value.State = AvailabilityUnavailable
			value.Reason = "native definition is skipped or its registered plugin is not enabled"
			winner, hasWinner := winners[name]
			precedence := PrecedenceExplanation{State: PrecedenceUnknown, Evidence: codeafEvidence(EvidenceInferred, "project-root-name-precedence", source.Directory, "", "project before home; conventional root order, then sorted plugins, then Codex system skills")}
			if eligible {
				value.State = AvailabilityAvailable
				value.Reason = "winning native definition is offered automatically and through conversation attachment"
				precedence.State = PrecedenceUnique
				if len(groups[name]) > 1 {
					precedence.State = PrecedenceWinning
					precedence.Winner = id
				}
			} else if hasWinner && nativeReasons[id] == "" {
				// Disabled plugin copies do not compete in native precedence.
				for _, candidate := range candidates {
					if candidate.source == id && candidate.root.permission.State == PermissionAllowed {
						value.State = AvailabilityShadowed
						value.Reason = "earlier loaded definition owns this native name"
						precedence.State = PrecedenceShadowed
						precedence.Winner = winner.source
						break
					}
				}
			}
			if uncertain && !eligible {
				value.State = AvailabilityUnknown
				value.Reason = "plugin input is uncertain; native name precedence cannot establish this source as available"
				if !hasWinner {
					precedence.Evidence.Kind = EvidenceUnknown
					precedence.Evidence.Reason = value.Reason
				}
			}
			for _, other := range groups[name] {
				if other != id {
					precedence.RelatedSources = append(precedence.RelatedSources, other)
				}
			}
			value.Precedence = &precedence
			*value.Evidence = codeafEvidence(EvidenceInferred, "source-version-availability", source.Directory, "", value.Reason)
			if value.State == AvailabilityUnknown {
				value.Evidence.Kind = EvidenceUnknown
			}
		}
		value.Invocation = &SessionExplanation{State: "automatic-and-attachment", Evidence: codeafEvidence(EvidenceInferred, "native-offering", source.Directory, "", "winning definitions are offered to the model and /skill attachment; another application's per-skill invocation setting is not honored")}
		value.Session = &SessionExplanation{State: "unknown", Evidence: codeafEvidence(EvidenceUnknown, "conversation-not-observed", "", "", "attachment and stored facts are not inspected; detaching does not disable automatic discovery")}
		value.Reload = &SessionExplanation{State: "unknown", Evidence: codeafEvidence(EvidenceUnknown, "fresh-launch", "", "", "native documentation describes rescanning at launch; viewer refresh does not update a running conversation")}
		for _, operation := range []string{"enable", "disable", "project-override", "delete"} {
			reason := "persistent CodeAF-only per-skill control is unsupported; conversation detachment and another agent's setting are not equivalent; shared sources and settings are never mutated"
			if operation == "delete" {
				reason = "read-only adapter has no deletion writer; deleting a shared source would affect its other consumers"
			}
			value.Capabilities = append(value.Capabilities, Capability{Operation: operation, Scope: "codeaf-only", State: "unsupported", Evidence: codeafEvidence(EvidenceInferred, "unsupported-persistent-control", source.Directory, "", reason)})
		}
		source.Availability = value
		snapshot.sources[id] = source
	}
	rootMap := make(map[RootID]DiscoveryRoot)
	for _, root := range request.Roots {
		rootMap[root.ID] = root
	}
	applyNativeProblems(snapshot, inputs.problems, rootMap)
	return snapshot, nil
}

func codeafNativeName(source Source) (string, string) {
	raw, err := codeafManifestHead(source.Manifest.Path)
	if err != nil {
		return "", "manifest is not safely readable"
	}
	if len(raw) > 64*1024 {
		raw = raw[:64*1024]
	}
	text := strings.ReplaceAll(string(raw), "\r\n", "\n")
	if !strings.HasPrefix(text, "---\n") {
		return "", "native manifest requires opening frontmatter"
	}
	end := strings.Index(text[4:], "\n---\n")
	if end < 0 {
		return "", "frontmatter is not closed within native 64 KiB head"
	}
	lines := strings.Split(text[4:4+end], "\n")
	for i, line := range lines {
		trimmed := strings.TrimLeft(line, " \t")
		if !strings.HasPrefix(trimmed, "description:") {
			continue
		}
		value := strings.TrimSpace(strings.TrimPrefix(trimmed, "description:"))
		if value != "" && value[0] != '\'' && value[0] != '"' && strings.Contains(value, ":") {
			value = strings.ReplaceAll(strings.ReplaceAll(value, `\`, `\\`), `"`, `\"`)
			lines[i] = line[:len(line)-len(trimmed)] + `description: "` + value + `"`
		}
	}
	var parsed struct {
		Name        string `yaml:"name"`
		Description string `yaml:"description"`
	}
	if yaml.Unmarshal([]byte(strings.Join(lines, "\n")), &parsed) != nil {
		return "", "native frontmatter is invalid YAML"
	}
	name := strings.TrimSpace(parsed.Name)
	description := strings.TrimSpace(parsed.Description)
	if strings.ContainsAny(description, "\r\n") {
		description = strings.Join(strings.Fields(description), " ")
	}
	if name == "" || description == "" || utf8.RuneCountInString(description) > 1024 {
		return name, "native name/description requirement is not met"
	}
	return name, ""
}

func codeafManifestHead(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("manifest is not regular")
	}
	file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err = file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("manifest changed file kind")
	}
	return io.ReadAll(io.LimitReader(file, 64*1024))
}

func codeafEnv(name string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return os.Getenv("AFORGE_" + strings.TrimPrefix(name, "CODEAF_"))
}

func codeafInsideInstall(source, install string) bool {
	real, err := filepath.EvalSymlinks(install)
	if err != nil {
		return false
	}
	relative, err := filepath.Rel(real, source)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(os.PathSeparator))
}
