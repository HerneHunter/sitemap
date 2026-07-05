package sitemap

import "time"

// ChangeFreq is the value of a sitemap entry's <changefreq> tag.
type ChangeFreq string

const (
	ChangeFreqAlways  ChangeFreq = "always"
	ChangeFreqHourly  ChangeFreq = "hourly"
	ChangeFreqDaily   ChangeFreq = "daily"
	ChangeFreqWeekly  ChangeFreq = "weekly"
	ChangeFreqMonthly ChangeFreq = "monthly"
	ChangeFreqYearly  ChangeFreq = "yearly"
	ChangeFreqNever   ChangeFreq = "never"
)

// ParseResult carries a parsed URL entry or an error.
// Loc is always populated. Every other field is populated only if its
// matching With* option was passed in, and stays zero (or -1 for Priority) otherwise.
type ParseResult struct {
	Loc        string
	LastMod    time.Time
	ChangeFreq ChangeFreq
	Priority   float64
	Err        error
}
