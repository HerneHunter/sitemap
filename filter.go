package sitemap

import (
	"regexp"
	"strings"
)

// Filter provides rules for filtering sitemap or index URLs.
type Filter struct {
	// Applied to the URI (path + query). If set, the URI must match at least one.
	Whitelist []*regexp.Regexp

	// Applied to the URI (path + query). If set, the URI must not match any.
	Blacklist []*regexp.Regexp

	// Applied to the full loc string, including the domain.
	RawMatch *regexp.Regexp
}

func (f *Filter) match(loc string) bool {
	if f == nil {
		return true
	}

	if f.RawMatch != nil && !f.RawMatch.MatchString(loc) {
		return false
	}

	hasWhitelist := len(f.Whitelist) > 0
	hasBlacklist := len(f.Blacklist) > 0

	// Skip URI extraction if we're only doing a RawMatch.
	if !hasWhitelist && !hasBlacklist {
		return true
	}

	uri := extractURI(loc)

	if hasBlacklist {
		for _, re := range f.Blacklist {
			if re.MatchString(uri) {
				return false
			}
		}
	}

	if hasWhitelist {
		matched := false
		for _, re := range f.Whitelist {
			if re.MatchString(uri) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	return true
}

// extractURI parses out the path and query string without allocating or pulling in net/url.
func extractURI(loc string) string {
	_, after, ok := strings.Cut(loc, "://")
	if !ok {
		return loc // Fallback if no scheme
	}

	rest := after

	// Find the first '/' in the host portion, which marks the start of the path.
	slash := strings.IndexByte(rest, '/')
	if slash != -1 {
		return rest[slash:]
	}

	// No slash found, check for a bare query string like "example.com?foo=bar".
	qmark := strings.IndexByte(rest, '?')
	if qmark != -1 {
		return rest[qmark:]
	}

	return "/"
}
