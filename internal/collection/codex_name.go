package collection

import "path/filepath"

const maxNativeNameAncestors = 128

// Native dispatch naming follows canonical ancestor metadata independently of ownership.
// Only the validated name/version manifest form is interpreted; other forms stay unknown.
func codexNativeName(source Source) (string, Evidence, *Problem) {
	for path, depth := source.Directory, 0; depth < maxNativeNameAncestors; path, depth = filepath.Dir(path), depth+1 {
		metadata := filepath.Join(path, ".codex-plugin", "plugin.json")
		manifest, err := readPluginManifest(path)
		if err == nil {
			return manifest.Name + ":" + source.Name, codexEvidence(EvidenceInferred, "ancestor-plugin-manifest", metadata, "name", "valid canonical ancestor manifest supplies dispatch namespace; it does not establish ownership or activation"), nil
		}
		if !nativeAbsent(metadata, err) {
			problem := makeProblem(ProblemNativeInput, metadata, "native namespace metadata is unreadable or outside the validated manifest form", nil, true)
			problem.SourceIDs = []SourceID{source.ID}
			return "", codexEvidence(EvidenceUnknown, "ancestor-plugin-manifest", metadata, "name", "namespace cannot be reliably interpreted; ownership and permission remain separate"), &problem
		}
		if filepath.Dir(path) == path {
			return source.Name, codexEvidence(EvidenceInferred, "plain-skill-name", source.Manifest.Path, "name", "no ancestor plugin manifest found; skill retains its plain dispatch name"), nil
		}
	}
	problem := makeProblem(ProblemNativeInput, source.Directory, "native namespace ancestry exceeds inspection limit", nil, true)
	problem.SourceIDs = []SourceID{source.ID}
	return "", codexEvidence(EvidenceUnknown, "ancestor-plugin-manifest", source.Directory, "name", "ancestry inspection limit reached"), &problem
}
