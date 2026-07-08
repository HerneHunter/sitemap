/*
	Utilities for parsing sitemap <lastmod> timestamps.

	Sitemaps in the wild often use different timestamp formats. This file
	implements tolerant parsing that normalizes these values into Go
	time.Time.

	Common ISO‑8601 variants are parsed using the Go standard library for
	performance, since they cover the majority of real‑world sitemaps. For
	less common or irregular formats, the parser falls back to the dateparse
	library.

	Jalali (Shamsi) dates and non‑ASCII digits (Persian/Arabic) are also
	detected and normalized before returning the final time value.
*/

package sitemap

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/araddon/dateparse"
	ptime "github.com/yaa110/go-persian-calendar"
)

var (
	// iranLocation holds the Asia/Tehran timezone location
	iranLocation    *time.Location
	errIranLocation error

	// standardFormats contains common time format layouts to try parsing
	standardFormats = []string{
		time.RFC3339,
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05",
		"2006-01-02",
	}

	// digitMap maps Persian and Arabic digits to ASCII digits
	digitMap = map[rune]rune{
		'۰': '0', '۱': '1', '۲': '2', '۳': '3', '۴': '4',
		'۵': '5', '۶': '6', '۷': '7', '۸': '8', '۹': '9',
		'٠': '0', '١': '1', '٢': '2', '٣': '3', '٤': '4',
		'٥': '5', '٦': '6', '٧': '7', '٨': '8', '٩': '9',
	}
)

// init loads the Iran timezone on package initialization
func init() {
	iranLocation, errIranLocation = time.LoadLocation("Asia/Tehran")
	if errIranLocation != nil {
		fmt.Printf("Error loading Iran timezone: %v\n", errIranLocation)
	}
}

// ParseTime attempts to parse a time string using multiple strategies
// It tries standard formats, dateparse library, and Jalali calendar parsing
func ParseTime(input string) (time.Time, error) {
	normalized := normalizeDigits(input)

	// Try standard time formats
	if t, ok := tryStandardFormats(normalized); ok {
		return t, nil
	}

	// Try parsing as Jalali date (before dateparse to avoid expensive fallback)
	if isJalali(normalized) {
		if jalaliTime, err := parseJalaliTime(normalized); err == nil {
			return jalaliTime, nil
		}
	}

	// Try using dateparse package (last resort for unusual Gregorian formats)
	if t, ok := tryDateparse(normalized); ok {
		return t, nil
	}

	return time.Time{}, &TimeParseError{Raw: input}
}

// tryStandardFormats attempts to parse time using predefined standard formats
func tryStandardFormats(normalized string) (time.Time, bool) {
	for _, format := range standardFormats {
		if parsedTime, err := time.Parse(format, normalized); err == nil {
			return convertIfJalali(parsedTime), true
		}
	}
	return time.Time{}, false
}

// tryDateparse attempts to parse time using the dateparse library
func tryDateparse(normalized string) (time.Time, bool) {
	parsedTime, err := dateparse.ParseAny(normalized)
	if err == nil {
		return convertIfJalali(parsedTime), true
	}
	return time.Time{}, false
}

// convertIfJalali checks if the parsed year looks like a Jalali year
// and converts it to Gregorian if needed
func convertIfJalali(t time.Time) time.Time {
	if looksLikeJalali(t.Year()) {
		gregorianTime := jalaliToGregorian(
			t.Year(),
			int(t.Month()),
			t.Day(),
			t.Hour(),
			t.Minute(),
			t.Second(),
		)
		return gregorianTime
	}
	return t
}

// looksLikeJalali checks if a year value is in the typical Jalali calendar range
func looksLikeJalali(year int) bool {
	return year >= 1300 && year < 1500
}

// isJalali checks if a string starts with typical Jalali year prefixes (13xx or 14xx)
func isJalali(s string) bool {
	return len(s) >= 4 && (s[0:2] == "13" || s[0:2] == "14") && (len(s) == 4 || !isDigit(s[4]))
}

// isDigit checks if a byte is an ASCII digit
func isDigit(b byte) bool {
	return b >= '0' && b <= '9'
}

// jalaliToGregorian converts a Jalali date to Gregorian
func jalaliToGregorian(year, month, day, hour, minute, second int) time.Time {
	pt := ptime.Date(year, ptime.Month(month), day, hour, minute, second, 0, iranLocation)
	return pt.Time()
}

// parseJalaliTime parses a Shamsi date string and converts it to Gregorian time
func parseJalaliTime(s string) (time.Time, error) {
	// Split by common date/time separators
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == '-' || r == '/' || r == 'T' || r == ':' || r == ' ' || r == '+'
	})

	if len(parts) < 3 {
		return time.Time{}, &TimeParseError{Raw: s}
	}

	// Parse date components
	year, _ := strconv.Atoi(parts[0])
	month, _ := strconv.Atoi(parts[1])
	day, _ := strconv.Atoi(parts[2])

	// Parse time components if available
	hour, minute, second := 0, 0, 0
	if len(parts) >= 5 {
		hour, _ = strconv.Atoi(parts[3])
		minute, _ = strconv.Atoi(parts[4])
	}
	if len(parts) >= 6 {
		second, _ = strconv.Atoi(parts[5])
	}

	// Create Jalali date and convert to Gregorian
	return jalaliToGregorian(year, month, day, hour, minute, second), nil
}

// normalizeDigits converts Persian and Arabic digits to ASCII digits
func normalizeDigits(s string) string {
	needsNormalization := false
	for _, r := range s {
		if _, ok := digitMap[r]; ok {
			needsNormalization = true
			break
		}
	}

	if !needsNormalization {
		return s
	}

	var b strings.Builder
	b.Grow(len(s))

	for _, r := range s {
		if ascii, ok := digitMap[r]; ok {
			b.WriteRune(ascii)
		} else {
			b.WriteRune(r)
		}
	}

	return b.String()
}
