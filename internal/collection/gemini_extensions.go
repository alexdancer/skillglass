package collection

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"
)

var geminiExtensionName = regexp.MustCompile(`^[a-zA-Z0-9-]+$`)

type geminiExtension struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

func discoverGeminiExtensions(in *geminiInputs, add func(string, ReferenceContext, string, int, ParticipationState, Evidence, Provenance)) {
	directory := filepath.Join(in.home, ".gemini", "extensions")
	entries, err := os.ReadDir(directory)
	coverage := ManagerEvidence{Manager: "extensions", Boundary: directory, State: ManagerObserved, Evidence: geminiEvidence(EvidenceObserved, "extension-registry", directory, "", "extension installation boundaries are separate from skill loader roots and activation")}
	if nativeAbsent(directory, err) {
		coverage.State = ManagerAbsent
	} else if err != nil {
		coverage.State = ManagerUncertain
	}
	in.managers = append(in.managers, coverage)
	if os.IsNotExist(err) {
		return
	}
	if err != nil {
		in.problem(directory, "extension registry cannot be listed")
		return
	}
	enablementPath := filepath.Join(directory, "extension-enablement.json")
	var enablement map[string]struct {
		Overrides []any `json:"overrides"`
	}
	raw, readErr := nativeBytes(enablementPath)
	unknown := false
	if !nativeAbsent(enablementPath, readErr) && (readErr != nil || json.Unmarshal(raw, &enablement) != nil || enablement == nil) {
		in.problem(enablementPath, "extension enablement registry is unreadable or invalid")
		unknown = true
	}
	for _, entry := range enablement {
		if entry.Overrides == nil {
			in.problem(enablementPath, "extension overrides must be an array of strings")
			unknown = true
		}
		for _, rule := range entry.Overrides {
			if _, ok := rule.(string); !ok {
				in.problem(enablementPath, "extension overrides must contain only strings")
				unknown = true
			}
		}
	}
	type registration struct {
		path        string
		manifest    geminiExtension
		valid       bool
		uninspected bool
		linked      bool
	}
	var registrations []registration
	counts := make(map[string]int)
	for _, entry := range entries {
		path := filepath.Join(directory, entry.Name())
		installation := path
		info, err := os.Stat(path)
		if err != nil {
			in.managers = append(in.managers, ManagerEvidence{Manager: entry.Name(), Boundary: path, State: ManagerUncertain, Evidence: geminiEvidence(EvidenceUnknown, "extension-installation", path, "", "installation cannot be inspected")})
			continue
		}
		if !info.IsDir() {
			continue
		}
		linked := false
		metadataPath := filepath.Join(path, ".gemini-extension-install.json")
		raw, err := nativeBytes(metadataPath)
		if !nativeAbsent(metadataPath, err) {
			var metadata struct {
				Type   string `json:"type"`
				Source string `json:"source"`
			}
			if err != nil || json.Unmarshal(raw, &metadata) != nil {
				in.problem(metadataPath, "extension install metadata is invalid")
				linked = true
			} else if metadata.Type == "link" {
				linked = true
				if filepath.IsAbs(metadata.Source) {
					path = metadata.Source
				} else {
					in.problem(metadataPath, "linked extension source must be absolute")
				}
			}
		}
		manifestPath := filepath.Join(path, "gemini-extension.json")
		raw, err = nativeBytes(manifestPath)
		var manifest geminiExtension
		uninspected := !nativeAbsent(manifestPath, err) && (err != nil || !utf8.Valid(raw))
		if uninspected {
			in.problem(manifestPath, "extension manifest could not be fully inspected as bounded UTF-8")
		}
		valid := !uninspected && err == nil && json.Unmarshal(raw, &manifest) == nil && manifest.Name != "" && manifest.Version != "" && geminiExtensionName.MatchString(manifest.Name)
		if valid {
			counts[manifest.Name]++
		}
		if real, err := filepath.EvalSymlinks(installation); err == nil && geminiManagedLocation(filepath.Join(installation, "skills"), real, "plugin", in.home) {
			state := ManagerBoundary
			if !valid {
				state = ManagerUncertain
			}
			in.managers = append(in.managers, ManagerEvidence{Manager: entry.Name(), Boundary: installation, State: state, Evidence: geminiEvidence(EvidenceObserved, "extension-installation", metadataPath, "", "installed extension boundary; linked external sources remain separate")})
		}
		registrations = append(registrations, registration{path, manifest, valid, uninspected, linked})
	}
	for _, registration := range registrations {
		path := filepath.Join(registration.path, "skills")
		state := ParticipationEligible
		reason := "valid extension manifest registered under native user extension directory; enabled by default"
		if registration.uninspected {
			state = ParticipationUnknown
			reason = "extension registration manifest could not be fully inspected"
		} else if !registration.valid {
			state = ParticipationExcluded
			reason = "folder alone is not proof of extension registration; native manifest is missing or invalid"
		} else if unknown || in.extensionPolicy || counts[registration.manifest.Name] > 1 || registration.linked {
			state = ParticipationUnknown
			reason = "duplicate registration, security policy, linked extension, or unreadable enablement requires unverified activation evidence"
		} else {
			current := in.project
			overrides := enablement[registration.manifest.Name].Overrides
			for _, value := range overrides {
				rule := value.(string)
				if current == "" && rule != "/*" && rule != "!/*" {
					state = ParticipationUnknown
					reason = "project-specific extension override requires a selected workspace"
					continue
				}
				if geminiExtensionRule(rule, current) {
					if strings.HasPrefix(rule, "!") {
						state = ParticipationExcluded
						reason = "last matching native extension override disables this extension"
					} else {
						state = ParticipationEligible
						reason = "last matching native extension override enables this extension"
					}
				}
			}
		}
		e := geminiEvidence(EvidenceInferred, "extension-registration-and-enablement", enablementPath, "overrides", reason)
		if state == ParticipationUnknown {
			e.Kind = EvidenceUnknown
		}
		provenance := Provenance{Kind: "plugin", PluginID: registration.manifest.Name, Version: registration.manifest.Version, Evidence: geminiEvidence(EvidenceObserved, "extension-manifest", filepath.Join(registration.path, "gemini-extension.json"), "name/version", "registered extension provides this reference; canonical location determines managed content ownership")}
		if !registration.valid {
			provenance.Kind = "managed-candidate"
			provenance.Evidence.Kind = EvidenceUnknown
			provenance.Evidence.Reason = reason + "; folder does not establish ownership"
		}
		add(path, ContextManaged, "gemini-extension", 10, state, e, provenance)
	}
}
func geminiExtensionRule(rule, current string) bool {
	pattern := strings.TrimPrefix(rule, "!")
	var expression strings.Builder
	expression.WriteString("^")
	for i, character := range pattern {
		if character == '*' {
			if i > 0 && pattern[i-1] == '/' { // native glob makes the preceding slash optional along with suffix
				value := expression.String()
				expression.Reset()
				expression.WriteString(strings.TrimSuffix(value, "/"))
				expression.WriteString("(/.*)?")
			} else {
				expression.WriteString("(.*)?")
			}
		} else {
			expression.WriteString(regexp.QuoteMeta(string(character)))
		}
	}
	expression.WriteString("$")
	target := strings.ReplaceAll(current, "\\", "/")
	if !strings.HasPrefix(target, "/") {
		target = "/" + target
	}
	if !strings.HasSuffix(target, "/") {
		target += "/"
	}
	match, err := regexp.MatchString(expression.String(), target)
	return err == nil && match
}
