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

// ParseResult represents a parsed sitemap entry or an error.
// Loc is always populated. Other fields are only populated if their
// corresponding With* option was provided; otherwise they remain zero-valued
// (or -1 for Priority).
type ParseResult struct {
	Loc        string
	LastMod    time.Time
	ChangeFreq ChangeFreq
	Priority   float64
	Err        error
}
