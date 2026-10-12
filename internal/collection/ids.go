package collection

import (
	"crypto/sha256"
	"encoding/base32"
	"strings"
)

func stableID(prefix string, parts ...string) string {
	hash := sha256.New()
	for _, part := range append([]string{prefix}, parts...) {
		hash.Write([]byte(part))
		hash.Write([]byte{0})
	}
	sum := hash.Sum(nil)
	return strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(sum[:16]))
}

func referenceID(root RootID, relativePath string) ReferenceID {
	return ReferenceID("ref_" + stableID("skillglass-reference-v1", string(root), relativePath))
}

func sourceID(token objectToken) SourceID {
	return SourceID("src_" + stableID("skillglass-source-darwin-v1", token.String()))
}

func problemID(code ProblemCode, path string) ProblemID {
	return ProblemID("problem_" + stableID("skillglass-problem-v1", string(code), path))
}

func makeProblem(code ProblemCode, path, summary string, err error, limitsCoverage bool) Problem {
	detail := ""
	if err != nil {
		detail = err.Error()
	}
	return Problem{
		ID:             problemID(code, path),
		Code:           code,
		Path:           path,
		Summary:        summary,
		Detail:         detail,
		LimitsCoverage: limitsCoverage,
	}
}
