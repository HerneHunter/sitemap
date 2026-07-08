package sitemap

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// testCase mirrors the JSON structure in testdata/time/*.json
type testCase struct {
	Desc     string `json:"desc"`
	Input    string `json:"input"`
	Expected string `json:"expected"` // RFC3339; empty when Error is true
	Error    bool   `json:"error"`
}

func loadCases(t *testing.T, filename string) []testCase {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "time", filename))
	if err != nil {
		t.Fatalf("cannot read %s: %v", filename, err)
	}
	var cases []testCase
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatalf("cannot parse %s: %v", filename, err)
	}
	return cases
}

// assertTime checks that got matches the RFC3339 expected string.
// For Shamsi results the timezone offset is part of the expected value,
// so we compare Unix timestamps to avoid location-name mismatches.
func assertTime(t *testing.T, desc, expected string, got time.Time) {
	t.Helper()
	want, err := time.Parse(time.RFC3339, expected)
	if err != nil {
		t.Fatalf("[%s] bad expected value %q: %v", desc, expected, err)
	}
	if got.Unix() != want.Unix() {
		t.Errorf("[%s] got %s, want %s", desc, got.Format(time.RFC3339), want.Format(time.RFC3339))
	}
}

// runTimeCases loads the cases in filename and runs check as a subtest per case.
func runTimeCases(t *testing.T, filename string, check func(t *testing.T, tc testCase, got time.Time, err error)) {
	t.Helper()
	for _, tc := range loadCases(t, filename) {
		t.Run(tc.Desc, func(t *testing.T) {
			got, err := ParseTime(tc.Input)
			check(t, tc, got, err)
		})
	}
}

func TestParseTime_ShamsiValid(t *testing.T) {
	runTimeCases(t, "shamsi_valid.json", func(t *testing.T, tc testCase, got time.Time, err error) {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		assertTime(t, tc.Desc, tc.Expected, got)
	})
}

func TestParseTime_ShamsiPersianDigits(t *testing.T) {
	runTimeCases(t, "shamsi_persian_digits.json", func(t *testing.T, tc testCase, got time.Time, err error) {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		assertTime(t, tc.Desc, tc.Expected, got)
	})
}

func TestParseTime_GregorianValid(t *testing.T) {
	runTimeCases(t, "gregorian_valid.json", func(t *testing.T, tc testCase, got time.Time, err error) {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Only compare date part for formats that lose time info
		want, _ := time.Parse(time.RFC3339, tc.Expected)
		if got.Year() != want.Year() || got.Month() != want.Month() || got.Day() != want.Day() {
			t.Errorf("[%s] date mismatch: got %s, want %s",
				tc.Desc, got.Format("2006-01-02"), want.Format("2006-01-02"))
		}
	})
}

func TestParseTime_Invalid(t *testing.T) {
	runTimeCases(t, "invalid.json", func(t *testing.T, tc testCase, got time.Time, err error) {
		if err == nil {
			t.Errorf("[%s] expected error but got %v", tc.Desc, got)
		}
		if !got.IsZero() {
			t.Errorf("[%s] expected zero result on error, got %v", tc.Desc, got)
		}
	})
}

func TestParseTime_ErrorContainsRawInput(t *testing.T) {
	input := "garbage-input-xyz"
	_, err := ParseTime(input)
	if err == nil {
		t.Fatal("expected error")
	}
	var tpe *TimeParseError
	ok := errors.As(err, &tpe)
	if !ok {
		t.Fatalf("expected *TimeParseError, got %T", err)
	}
	if tpe.Raw != input {
		t.Errorf("Raw = %q, want %q", tpe.Raw, input)
	}
}

func TestNormalizeDigits(t *testing.T) {
	cases := []struct {
		input, want string
	}{
		{"۱۴۰۳/۰۶/۱۵", "1403/06/15"},
		{"١٤٠٣/٠٦/١٥", "1403/06/15"},
		{"1403/06/15", "1403/06/15"},   // already ASCII
		{"hello world", "hello world"}, // no digits
		{"", ""},
	}
	for _, tc := range cases {
		got := normalizeDigits(tc.input)
		if got != tc.want {
			t.Errorf("normalizeDigits(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestIsShamsi(t *testing.T) {
	trueInputs := []string{
		"1403/06/15",
		"1400-01-01",
		"1399/12/30",
		"1402/12/29",
	}
	falseInputs := []string{
		"2024-09-05",
		"09/05/2024",
		"September 5, 2024",
		"",
	}

	for _, s := range trueInputs {
		if !isJalali(s) {
			t.Errorf("isShamsi(%q) = false, want true", s)
		}
	}
	for _, s := range falseInputs {
		if isJalali(s) {
			t.Errorf("isShamsi(%q) = true, want false", s)
		}
	}
}



func BenchmarkParseTime_GregorianISO(b *testing.B) {
	for b.Loop() {
		ParseTime("2024-09-05T14:30:00Z")
	}
}

func BenchmarkParseTime_GregorianDateOnly(b *testing.B) {
	for b.Loop() {
		ParseTime("2024-09-05")
	}
}

func BenchmarkParseTime_ShamsiSlash(b *testing.B) {
	for b.Loop() {
		ParseTime("1403/06/15")
	}
}

func BenchmarkParseTime_ShamsiDash(b *testing.B) {
	for b.Loop() {
		ParseTime("1399-06-31")
	}
}

func BenchmarkParseTime_ShamsiWithTime(b *testing.B) {
	for b.Loop() {
		ParseTime("1403/06/15T14:30:00")
	}
}

func BenchmarkParseTime_PersianDigits(b *testing.B) {
	for b.Loop() {
		ParseTime("۱۴۰۳/۰۶/۱۵")
	}
}

func BenchmarkParseTime_DateparseFallback(b *testing.B) {
	for b.Loop() {
		ParseTime("Sep 5, 2024")
	}
}

func BenchmarkParseTime_Invalid(b *testing.B) {
	for b.Loop() {
		ParseTime("garbage-input-xyz")
	}
}
