package collection

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"
)

type geminiRootFact struct {
	rank          int
	participation ParticipationState
	evidence      Evidence
	provenance    Provenance
}

func buildGemini(ctx context.Context, request Request) (*Snapshot, error) {
	original := cloneRequest(request)
	in := readGeminiInputs(request)
	facts := make(map[RootID]geminiRootFact)
	add := func(path string, scope ReferenceContext, origin string, rank int, participation ParticipationState, evidence Evidence, provenance Provenance) {
		root := DiscoveryRoot{ID: RootID(stableID("root_gemini", origin, path)), Path: path, Context: scope, Origin: origin, Optional: true}
		request.Roots = append(request.Roots, root)
		facts[root.ID] = geminiRootFact{rank, participation, evidence, provenance}
	}
	if binary, err := exec.LookPath("gemini"); err == nil {
		if target, err := filepath.EvalSymlinks(binary); err == nil {
			builtin := filepath.Join(filepath.Dir(target), "builtin")
			if info, err := os.Stat(builtin); err == nil && info.IsDir() {
				add(builtin, ContextSystem, "gemini-builtin", 0, ParticipationEligible, geminiEvidence(EvidenceInferred, "builtin-bundle", builtin, "", "builtin directory beside resolved native executable"), Provenance{Kind: "system", Evidence: geminiEvidence(EvidenceInferred, "builtin-ownership", builtin, "", "native package owns bundled content")})
			}
		}
	}
	discoverGeminiExtensions(&in, add)
	for index, alias := range []string{".gemini", ".agents"} {
		path := filepath.Join(in.home, alias, "skills")
		add(path, ContextGlobal, "gemini-user"+alias, 20+index, ParticipationEligible, geminiEvidence(EvidenceInferred, "user-skill-root", path, "", "native user tier; .agents alias loaded after .gemini"), Provenance{Kind: "global", Evidence: geminiEvidence(EvidenceInferred, "native-root-provenance", path, "", "user discovery scope does not establish exclusive ownership")})
		if in.project != "" {
			path = filepath.Join(in.project, alias, "skills")
			add(path, ContextProject, "gemini-project"+alias, 30+index, in.trust, in.trustEvidence, Provenance{Kind: "project", Evidence: geminiEvidence(EvidenceInferred, "native-root-provenance", path, "", "selected workspace only; native discovery does not traverse parents or Git roots")})
		}
	}
	snapshot, err := discover(ctx, request)
	if err != nil {
		return nil, err
	}
	snapshot.request = original
	snapshot.scope.Agent = &in.scope
	roots := make(map[RootID]DiscoveryRoot)
	for _, root := range request.Roots {
		roots[root.ID] = root
	}
	for i := range snapshot.scope.Roots {
		root := &snapshot.scope.Roots[i]
		if _, ok := facts[root.ID]; ok {
			e := geminiEvidence(EvidenceObserved, "filesystem-root", root.Requested, "", "read-only scanner observed "+string(root.State))
			root.Evidence = &e
		}
	}
	ranks := make(map[SourceID]int)
	uncertainRanks := make(map[SourceID]bool)
	groups := make(map[string][]SourceID)
	opaqueRank := -1
	for _, id := range snapshot.order {
		source := snapshot.sources[id]
		name, nameEvidence, valid := geminiNativeName(source)
		permission := in.permission(name)
		explanation := AvailabilityExplanation{NativeName: name, NativeNameEvidence: &nameEvidence, State: AvailabilityUnknown, Reason: "inspection root does not establish native participation"}
		best := -1
		possible := -1
		excluded := false
		for _, rid := range source.References {
			ref := snapshot.references[rid]
			root := roots[ref.Root]
			fact := ReferenceAvailability{Reference: rid, Participation: ParticipationInspection, Evidence: geminiEvidence(EvidenceUnknown, "inspection-only", root.Path, "", "viewer custom roots are not native configured search roots"), Permission: PermissionExplanation{State: PermissionUnknown, Evidence: geminiEvidence(EvidenceUnknown, "inspection-only", root.Path, "", "native permission not established")}, Provenance: Provenance{Kind: "custom", Evidence: geminiEvidence(EvidenceUnknown, "ownership", source.Directory, "", "inspection alone proves no native ownership")}}
			if native, ok := facts[root.ID]; ok {
				fact.Participation = native.participation
				fact.Evidence = native.evidence
				fact.Permission = permission
				fact.Provenance = native.provenance
				if !valid && native.participation != ParticipationExcluded {
					fact.Participation = ParticipationExcluded
					if nameEvidence.Kind == EvidenceUnknown {
						fact.Participation = ParticipationUnknown
						opaqueRank = max(opaqueRank, native.rank)
					}
					fact.Evidence = nameEvidence
				}
				if (native.provenance.Kind == "plugin" || native.provenance.Kind == "system") && geminiManagedLocation(root.Path, source.Directory, native.provenance.Kind, in.home) {
					source.Ownership = EvidenceState{State: "managed", Reason: "Gemini extension or package manager owns this content"}
				}
				if fact.Participation == ParticipationUnknown && valid && native.rank > possible {
					possible = native.rank
				}
				if fact.Participation == ParticipationEligible && native.rank > best {
					best = native.rank
				}
				excluded = excluded || fact.Participation == ParticipationExcluded
			}
			// Resolve managed ownership through canonical source identity, including custom aliases.
			if fact.Participation == ParticipationInspection {
				for rootID, native := range facts {
					if native.provenance.Kind != "plugin" {
						continue
					}
					if geminiManagedLocation(roots[rootID].Path, source.Directory, "plugin", in.home) {
						fact.Provenance = native.provenance
						source.Ownership = EvidenceState{State: "managed", Reason: "canonical source lies within a Gemini extension"}
						break
					}
				}
			}
			explanation.References = append(explanation.References, fact)
		}
		if best >= 0 {
			explanation.State = AvailabilityAsk
			explanation.Reason = "eligible winning native definition; activation normally asks for consent"
			if best == 0 {
				explanation.State = AvailabilityAvailable
				explanation.Reason = "bundled builtin skill is pre-approved"
			}
			if permission.State == PermissionDenied {
				explanation.State = AvailabilityDisabled
				explanation.Reason = permission.Evidence.Reason
			} else if permission.State == PermissionUnknown {
				explanation.State = AvailabilityUnknown
				explanation.Reason = permission.Evidence.Reason
			}
		} else if excluded {
			explanation.State = AvailabilityUnavailable
			explanation.Reason = "native reference excluded by manifest, trust or extension activation rules"
		}
		if best >= 0 || possible >= 0 {
			ranks[id] = best
			if possible >= best {
				ranks[id] = possible
				uncertainRanks[id] = true
			}
			groups[name] = append(groups[name], id)
		}
		kind := EvidenceInferred
		if explanation.State == AvailabilityUnknown {
			kind = EvidenceUnknown
		}
		evidence := geminiEvidence(kind, "configured-availability", permission.Evidence.InputPath, permission.Evidence.Setting, explanation.Reason)
		explanation.Evidence = &evidence
		explanation.Precedence = &PrecedenceExplanation{State: PrecedenceUnknown, Evidence: geminiEvidence(EvidenceUnknown, "native-precedence", source.Directory, "", "no eligible native definition")}
		invocation := SessionExplanation{State: "ask", Evidence: geminiEvidence(EvidenceInferred, "activation-consent", source.Manifest.Path, "activate_skill", "ordinary skills normally require consent; policy/session decisions are not observed")}
		if best == 0 {
			invocation.State = "pre-approved"
		}
		if best < 0 {
			invocation.State = "unknown"
		}
		explanation.Invocation = &invocation
		explanation.Session = &SessionExplanation{State: "unknown", Evidence: geminiEvidence(EvidenceUnknown, "session-not-observed", "", "", "existing session and remote admin policy were not inspected")}
		explanation.Reload = &SessionExplanation{State: "unknown", Evidence: geminiEvidence(EvidenceUnknown, "session-reload", "", "", "refresh rereads local files; use native /skills reload or restart for existing sessions")}
		for _, operation := range []string{"enable", "disable", "project-override", "delete"} {
			scope := "source"
			if source.Ownership.State == "managed" {
				scope = "plugin"
			}
			explanation.Capabilities = append(explanation.Capabilities, Capability{Operation: operation, Scope: scope, State: "unsupported", Evidence: geminiEvidence(EvidenceUnknown, "read-only-ticket", source.Directory, "", "no verified writers exposed; native skill name controls do not isolate independent copies")})
		}
		source.Availability = explanation
		snapshot.sources[id] = source
	}
	applyGeminiPrecedence(snapshot, ranks, groups, uncertainRanks, opaqueRank)
	applyNativeProblems(snapshot, in.problems, roots)
	return snapshot, nil
}
func geminiWithin(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

const geminiWhitespace = "\t\n\v\f\r \u00a0\u1680\u2000\u2001\u2002\u2003\u2004\u2005\u2006\u2007\u2008\u2009\u200a\u2028\u2029\u202f\u205f\u3000\ufeff"

var geminiFrontmatter = regexp.MustCompile(`^---\r?\n([\s\S]*?)\r?\n---(?:\r?\n([\s\S]*))?`)
var geminiNameField = regexp.MustCompile("^[" + geminiWhitespace + "]*name:[" + geminiWhitespace + "]*([^\r\n\u2028\u2029]*)$")
var geminiDescriptionField = regexp.MustCompile("^[" + geminiWhitespace + "]*description:[" + geminiWhitespace + "]*([^\r\n\u2028\u2029]*)$")
var geminiDescriptionContinuation = regexp.MustCompile("^[ \t]+[^" + geminiWhitespace + "]")
var geminiNameCharacters = regexp.MustCompile(`[:\\/<>*?"|]`)

func geminiNativeName(source Source) (string, Evidence, bool) {
	path := source.Manifest.Path
	if source.Manifest.Status == ManifestUnreadable || source.Manifest.RawBytesBase64 != "" {
		return "", geminiEvidence(EvidenceUnknown, "native-manifest-inspection", path, "", "native manifest could not be fully inspected as bounded UTF-8"), false
	}
	match := geminiFrontmatter.FindStringSubmatch(source.Manifest.RawText)
	if match == nil {
		return "", geminiEvidence(EvidenceInferred, "native-manifest-rejection", path, "", "frontmatter must begin on first line with name and description"), false
	}
	var data map[string]any
	err := yaml.Unmarshal([]byte(match[1]), &data)
	name, nameOK := data["name"].(string)
	_, descriptionOK := data["description"].(string)
	rule := "native-frontmatter-name"
	if err != nil || !nameOK || !descriptionOK {
		nameOK, descriptionOK = false, false
		lines := strings.Split(strings.ReplaceAll(match[1], "\r\n", "\n"), "\n")
		for i := 0; i < len(lines); i++ {
			if field := geminiNameField.FindStringSubmatch(lines[i]); field != nil {
				name = strings.Trim(field[1], geminiWhitespace)
				nameOK = true
				continue
			}
			if geminiDescriptionField.MatchString(lines[i]) {
				descriptionOK = true
				for i+1 < len(lines) && geminiDescriptionContinuation.MatchString(lines[i+1]) {
					i++
				}
			}
		}
		rule = "native-simple-frontmatter"
	}
	if !nameOK || !descriptionOK {
		return "", geminiEvidence(EvidenceInferred, "native-manifest-rejection", path, "", "native loader requires name and description fields"), false
	}
	if strings.Contains(name, "${") {
		return "", geminiEvidence(EvidenceUnknown, "extension-hydration", path, "", "environment-expanded skill names are unverified"), false
	}
	return geminiNameCharacters.ReplaceAllString(name, "-"), geminiEvidence(EvidenceInferred, rule, path, "name", "native name sanitizes filename punctuation; reference identity remains separate"), true
}
func applyGeminiPrecedence(snapshot *Snapshot, ranks map[SourceID]int, groups map[string][]SourceID, uncertainRanks map[SourceID]bool, opaqueRank int) {
	for _, group := range groups {
		sort.Slice(group, func(i, j int) bool { return group[i] < group[j] })
		best := -1
		var winners []SourceID
		for _, id := range group {
			if ranks[id] > best {
				best = ranks[id]
				winners = []SourceID{id}
			} else if ranks[id] == best {
				winners = append(winners, id)
			}
		}
		uncertainWinner := opaqueRank >= best
		for _, id := range winners {
			uncertainWinner = uncertainWinner || uncertainRanks[id]
		}
		for _, id := range group {
			source := snapshot.sources[id]
			p := source.Availability.Precedence
			p.Evidence = geminiEvidence(EvidenceInferred, "native-tier-precedence", source.Directory, "", "builtin < extension < user .gemini < user .agents < workspace .gemini < workspace .agents; disabled winners do not fall back")
			for _, other := range group {
				if other != id {
					p.RelatedSources = append(p.RelatedSources, other)
				}
			}
			if len(winners) != 1 || uncertainWinner {
				p.State = PrecedenceUnknown
				p.Winner = ""
				p.Evidence.Kind = EvidenceUnknown
				source.Availability.State = AvailabilityUnknown
				source.Availability.Reason = "highest-tier native identity, participation or ordering is unverified"
				p.Evidence.Reason = source.Availability.Reason
				source.Availability.Evidence = &p.Evidence
			} else {
				p.Winner = winners[0]
				p.State = PrecedenceUnique
				if len(group) > 1 {
					if id == winners[0] {
						p.State = PrecedenceWinning
					} else {
						p.State = PrecedenceShadowed
						source.Availability.State = AvailabilityShadowed
						source.Availability.Reason = "higher-precedence independent definition owns this native name"
						e := geminiEvidence(EvidenceInferred, "native-tier-precedence", source.Directory, "name", source.Availability.Reason)
						source.Availability.Evidence = &e
					}
				}
			}
			snapshot.sources[id] = source
		}
	}
}

func geminiManagedLocation(root, path, kind, home string) bool {
	target, err := filepath.EvalSymlinks(filepath.Dir(root))
	if err != nil || !geminiWithin(target, path) {
		return false
	}
	if kind == "plugin" {
		registry, err := filepath.EvalSymlinks(filepath.Join(home, ".gemini", "extensions"))
		if err != nil || !geminiWithin(registry, target) || registry == target {
			return false
		}
	}
	return true
}
