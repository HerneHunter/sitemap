package sitemap

import (
	"regexp"
	"testing"
)

func TestExtractURI(t *testing.T) {
	tests := []struct {
		loc      string
		expected string
	}{
		{"https://example.com/path", "/path"},
		{"http://example.com/path?q=1", "/path?q=1"},
		{"https://example.com/", "/"},
		{"https://example.com", "/"}, // fallback
		{"http://example.com", "/"},  // fallback
		{"https://example.com?foo=bar", "?foo=bar"},
		{"/just/a/path", "/just/a/path"},
		{"invalid-url", "invalid-url"},
	}

	for _, tt := range tests {
		actual := extractURI(tt.loc)
		if actual != tt.expected {
			t.Errorf("extractURI(%q) = %q; expected %q", tt.loc, actual, tt.expected)
		}
	}
}

func TestFilterMatch(t *testing.T) {
	// Nil filter test
	var fNil *Filter
	if !fNil.match("https://example.com") {
		t.Error("Expected match for nil filter")
	}

	// RawMatch test
	fRaw := &Filter{
		RawMatch: regexp.MustCompile(`.*example\.com.*`),
	}
	if !fRaw.match("https://example.com/test") {
		t.Error("Expected match for raw filter")
	}
	if fRaw.match("https://other.com/test") {
		t.Error("Expected no match for raw filter")
	}

	// Whitelist test
	fWhite := &Filter{
		Whitelist: []*regexp.Regexp{
			regexp.MustCompile(`^/allow`),
		},
	}
	if !fWhite.match("https://example.com/allow/me") {
		t.Error("Expected match for whitelist")
	}
	if fWhite.match("https://example.com/deny/me") {
		t.Error("Expected no match for whitelist")
	}

	// Blacklist test
	fBlack := &Filter{
		Blacklist: []*regexp.Regexp{
			regexp.MustCompile(`^/deny`),
		},
	}
	if fBlack.match("https://example.com/deny/me") {
		t.Error("Expected no match for blacklist")
	}
	if !fBlack.match("https://example.com/allow/me") {
		t.Error("Expected match for blacklist")
	}

	// Whitelist and Blacklist test
	fBoth := &Filter{
		Whitelist: []*regexp.Regexp{
			regexp.MustCompile(`^/api`),
		},
		Blacklist: []*regexp.Regexp{
			regexp.MustCompile(`v2`),
		},
	}
	if !fBoth.match("https://example.com/api/v1") {
		t.Error("Expected match for both")
	}
	if fBoth.match("https://example.com/api/v2") {
		t.Error("Expected no match because of blacklist")
	}
	if fBoth.match("https://example.com/other") {
		t.Error("Expected no match because of whitelist")
	}
}

func BenchmarkExtractURI(b *testing.B) {
	loc := "https://www.example.com/products/item-42?ref=sitemap&lang=en"
	b.ReportAllocs()
	for b.Loop() {
		_ = extractURI(loc)
	}
}

func BenchmarkFilterMatch_NilFilter(b *testing.B) {
	var f *Filter
	b.ReportAllocs()
	for b.Loop() {
		f.match("https://example.com/some/path")
	}
}

func BenchmarkFilterMatch_RawOnly(b *testing.B) {
	f := &Filter{RawMatch: regexp.MustCompile(`example\.com`)}
	loc := "https://example.com/api/v1/resource"
	b.ReportAllocs()
	for b.Loop() {
		f.match(loc)
	}
}

func BenchmarkFilterMatch_WhitelistOnly(b *testing.B) {
	f := &Filter{
		Whitelist: []*regexp.Regexp{
			regexp.MustCompile(`^/api`),
			regexp.MustCompile(`^/admin`),
		},
	}
	loc := "https://example.com/api/v1/resource"
	b.ReportAllocs()
	for b.Loop() {
		f.match(loc)
	}
}

func BenchmarkFilterMatch_BlacklistOnly(b *testing.B) {
	f := &Filter{
		Blacklist: []*regexp.Regexp{
			regexp.MustCompile(`^/deny`),
			regexp.MustCompile(`^/blocked`),
		},
	}
	loc := "https://example.com/api/v1/resource"
	b.ReportAllocs()
	for b.Loop() {
		f.match(loc)
	}
}

func BenchmarkFilterMatch_WhitelistAndBlacklist(b *testing.B) {
	f := &Filter{
		Whitelist: []*regexp.Regexp{regexp.MustCompile(`^/api`)},
		Blacklist: []*regexp.Regexp{regexp.MustCompile(`v2`)},
	}
	loc := "https://example.com/api/v1/resource"
	b.ReportAllocs()
	for b.Loop() {
		f.match(loc)
	}
}

func BenchmarkFilterMatch_AllFilters(b *testing.B) {
	f := &Filter{
		RawMatch:  regexp.MustCompile(`example\.com`),
		Whitelist: []*regexp.Regexp{regexp.MustCompile(`^/api`)},
		Blacklist: []*regexp.Regexp{regexp.MustCompile(`v2`)},
	}
	loc := "https://example.com/api/v1/resource"
	b.ReportAllocs()
	for b.Loop() {
		f.match(loc)
	}
}
