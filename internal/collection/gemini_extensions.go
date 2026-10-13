package collection

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var geminiExtensionName = regexp.MustCompile(`^[a-zA-Z0-9-]+$`)

type geminiExtension struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

func discoverGeminiExtensions(in *geminiInputs, add func(string, ReferenceContext, string, int, ParticipationState, Evidence, Provenance)) {
	directory := filepath.Join(in.home, ".gemini", "extensions")
	entries, err := os.ReadDir(directory)
	if os.IsNotExist(err) {
		return
	}
	if err != nil {
		in.problem(directory, "extension registry cannot be listed")
		return
	}
	enablementPath := filepath.Join(directory, "extension-enablement.json")
	var enablement map[string]struct {
		Overrides *[]string `json:"overrides"`
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
	}
	type registration struct {
		path     string
		manifest geminiExtension
		valid    bool
		linked   bool
	}
	var registrations []registration
	counts := make(map[string]int)
	for _, entry := range entries {
		path := filepath.Join(directory, entry.Name())
		info, err := os.Stat(path)
		if err != nil || !info.IsDir() {
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
		valid := err == nil && json.Unmarshal(raw, &manifest) == nil && manifest.Name != "" && manifest.Version != "" && geminiExtensionName.MatchString(manifest.Name)
		if valid {
			counts[manifest.Name]++
		}
		registrations = append(registrations, registration{path, manifest, valid, linked})
	}
	for _, registration := range registrations {
		path := filepath.Join(registration.path, "skills")
		state := ParticipationEligible
		reason := "valid extension manifest registered under native user extension directory; enabled by default"
		if !registration.valid {
			state = ParticipationExcluded
			reason = "folder alone is not proof of extension registration; native manifest is missing or invalid"
		} else if unknown || in.extensionPolicy || counts[registration.manifest.Name] > 1 || registration.linked {
			state = ParticipationUnknown
			reason = "duplicate registration, security policy, linked extension, or unreadable enablement requires unverified activation evidence"
		} else {
			current := in.project
			var overrides []string
			if entry, ok := enablement[registration.manifest.Name]; ok && entry.Overrides != nil {
				overrides = *entry.Overrides
			}
			for _, rule := range overrides {
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
			provenance.Evidence.Reason = "unregistered extension candidate; folder does not establish ownership"
		}
		add(path, ContextManaged, "gemini-extension", 10, state, e, provenance)
	}
}
func geminiExtensionRule(rule, current string) bool {
	pattern := strings.TrimPrefix(rule, "!")
	var expression strings.Builder
	expression.WriteString("^")
	for i := 0; i < len(pattern); i++ {
		if pattern[i] == '*' {
			if i > 0 && pattern[i-1] == '/' { // native glob makes the preceding slash optional along with suffix
				value := expression.String()
				expression.Reset()
				expression.WriteString(strings.TrimSuffix(value, "/"))
				expression.WriteString("(/.*)?")
			} else {
				expression.WriteString("(.*)?")
			}
		} else {
			expression.WriteString(regexp.QuoteMeta(string(pattern[i])))
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
