package collection

import (
	"net/url"
	"strings"
	"unicode/utf8"

	whatwg "github.com/nationallibraryofnorway/whatwg-url/url"
)

type claudeHomepageValidity uint8

const (
	claudeHomepageSupported claudeHomepageValidity = iota
	claudeHomepageRejected
	claudeHomepageUnmodeled
)

type claudeHomepageAssessment struct {
	validity claudeHomepageValidity
	reason   string
}

// assessClaudeHomepage uses no-base WHATWG parsing, retaining recoverable
// validation errors such as extra slashes. The pinned parser's IDNA coverage
// has known false accepts, so those domain forms cannot establish activation.
func assessClaudeHomepage(homepage string) claudeHomepageAssessment {
	idnaUnmodeled := false
	parser := whatwg.NewParser(whatwg.WithPreParseHostFunc(func(u *whatwg.Url, host string) string {
		// Opaque hosts and bracketed IP literals do not use domain-to-ASCII.
		if !u.IsSpecialScheme() || strings.HasPrefix(host, "[") {
			return host
		}
		// PathUnescape decodes percent bytes without interpreting '+' as space.
		// Observe the original domain before IDNA can normalize away an A-label.
		domain, err := url.PathUnescape(host)
		if err != nil {
			idnaUnmodeled = true
			return host
		}
		for _, r := range domain {
			if r >= utf8.RuneSelf {
				idnaUnmodeled = true
			}
		}
		for _, label := range strings.Split(strings.ToLower(domain), ".") {
			if strings.HasPrefix(label, "xn--") {
				idnaUnmodeled = true
			}
		}
		return host
	}))
	_, err := parser.Parse(homepage)
	if idnaUnmodeled {
		return claudeHomepageAssessment{claudeHomepageUnmodeled, "homepage domain has unmodeled IDNA validity; native acceptance and plugin activation are unknown"}
	}
	if err != nil {
		return claudeHomepageAssessment{claudeHomepageRejected, "homepage fails supported WHATWG URL parsing; native plugin activation cannot be established: " + err.Error()}
	}
	return claudeHomepageAssessment{validity: claudeHomepageSupported}
}
