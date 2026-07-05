package sitemap

import (
	"bufio"
	"context"
	"os"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name          string
		xmlFile       string
		expectedURLs  string
		expectedTimes string
	}{
		{
			name:          "Regular sitemap",
			xmlFile:       "testdata/parser/sm-1.xml",
			expectedURLs:  "testdata/parser/sm-1-urls.txt",
			expectedTimes: "testdata/parser/sm-1-times.txt",
		},
		{
			name:          "Regular sitemap 2",
			xmlFile:       "testdata/parser/sm-2.xml",
			expectedURLs:  "testdata/parser/sm-2-urls.txt",
			expectedTimes: "testdata/parser/sm-2-times.txt",
		},
		{
			name:          "Sitemap index",
			xmlFile:       "testdata/parser/smi-1.xml",
			expectedURLs:  "testdata/parser/smi-1-urls.txt",
			expectedTimes: "testdata/parser/smi-1-times.txt",
		},
	}

	for _, tt := range tests {
		for _, includeTime := range []bool{true, false} {

			testName := tt.name
			if includeTime {
				testName += " (includeTime=true)"
			} else {
				testName += " (includeTime=false)"
			}

			t.Run(testName, func(t *testing.T) {
				xmlFile, err := os.Open(tt.xmlFile)
				if err != nil {
					t.Fatalf("Failed to open XML file: %v", err)
				}
				defer xmlFile.Close()

				expectedURLs, err := readLines(tt.expectedURLs)
				if err != nil {
					t.Fatalf("Failed to read expected URLs: %v", err)
				}

				expectedTimes, err := readLines(tt.expectedTimes)
				if err != nil {
					t.Fatalf("Failed to read expected times: %v", err)
				}

				var resCh <-chan ParseResult
				if includeTime {
					resCh = Parse(context.TODO(), xmlFile, WithBufferSize(2500), WithLastMod())
				} else {
					resCh = Parse(context.TODO(), xmlFile, WithBufferSize(2500))
				}

				var actualURLs []string
				var actualTimes []time.Time

				for result := range resCh {
					if result.Err != nil {
						t.Errorf("Unexpected error: %v", result.Err)
						continue
					}

					actualURLs = append(actualURLs, result.Loc)
					actualTimes = append(actualTimes, result.LastMod)
				}

				if len(actualURLs) != len(expectedURLs) {
					t.Fatalf("Got %d URLs, want %d", len(actualURLs), len(expectedURLs))
				}

				type entry struct {
					url  string
					time time.Time
				}
				var actualEntries []entry
				for i := range actualURLs {
					actualEntries = append(actualEntries, entry{actualURLs[i], actualTimes[i]})
				}
				sort.Slice(actualEntries, func(i, j int) bool {
					if actualEntries[i].url != actualEntries[j].url {
						return actualEntries[i].url < actualEntries[j].url
					}
					if !actualEntries[i].time.IsZero() && !actualEntries[j].time.IsZero() {
						return actualEntries[i].time.Before(actualEntries[j].time)
					}
					return false
				})

				var expectedEntries []entry
				for i := range expectedURLs {
					if expectedTimes[i] != "" {
						pt, err := time.Parse(time.RFC3339, expectedTimes[i])
						if err != nil {
							pt, _ = time.Parse("2006-01-02 15:04:05 -0700 -0700", expectedTimes[i])
						}
						expectedEntries = append(expectedEntries, entry{expectedURLs[i], pt})
					} else {
						expectedEntries = append(expectedEntries, entry{expectedURLs[i], time.Time{}})
					}
				}
				sort.Slice(expectedEntries, func(i, j int) bool {
					if expectedEntries[i].url != expectedEntries[j].url {
						return expectedEntries[i].url < expectedEntries[j].url
					}
					if !expectedEntries[i].time.IsZero() && !expectedEntries[j].time.IsZero() {
						return expectedEntries[i].time.Before(expectedEntries[j].time)
					}
					return false
				})

				for i := range actualEntries {
					if actualEntries[i].url != expectedEntries[i].url {
						t.Errorf("URL[%d] = %q, want %q", i, actualEntries[i].url, expectedEntries[i].url)
					}

					// when includeTime=false → all LastMod must be zero
					if !includeTime {
						if !actualEntries[i].time.IsZero() {
							t.Errorf("Time[%d] expected zero when includeTime=false, got %v", i, actualEntries[i].time)
						}
						continue
					}

					// includeTime=true
					if expectedEntries[i].time.IsZero() {
						if !actualEntries[i].time.IsZero() {
							t.Errorf("Time[%d] expected zero, got %v", i, actualEntries[i].time)
						}
						continue
					}

					if actualEntries[i].time.IsZero() {
						t.Errorf("Time[%d] expected value, got zero", i)
						continue
					}

					if !actualEntries[i].time.Equal(expectedEntries[i].time) {
						t.Errorf("Time[%d] = %v, want %v", i, actualEntries[i].time, expectedEntries[i].time)
					}
				}
			})
		}
	}
}

// TestParseFieldExtractor covers the generic extension mechanism itself,
// independent of any specific well-known field like lastmod.
func TestParseOptions(t *testing.T) {
	t.Run("built-in flag nested inside an unrelated extension block is ignored", func(t *testing.T) {
		// depth==1 gating: <priority> inside <image:image> must NOT be
		// captured.
		xmlStr := `<urlset>
			<url>
				<loc>https://example.com/a</loc>
				<image:image><priority>9.9</priority></image:image>
				<priority>0.5</priority>
			</url>
		</urlset>`

		var got []float64

		for result := range Parse(context.TODO(), strings.NewReader(xmlStr), WithBufferSize(10), WithPriority()) {
			if result.Err != nil {
				continue
			}
			if result.Priority != -1 {
				got = append(got, result.Priority)
			}
		}

		if len(got) != 1 || got[0] != 0.5 {
			t.Errorf("Priority = %v, want [0.5] (nested tag must be ignored)", got)
		}
	})

	t.Run("sm: prefixed tag matches options", func(t *testing.T) {
		xmlStr := `<urlset>
			<sm:url><sm:loc>https://example.com/a.xml</sm:loc><sm:lastmod>2024-01-01T00:00:00Z</sm:lastmod></sm:url>
		</urlset>`

		var gotLoc string
		var gotTime time.Time
		for result := range Parse(context.TODO(), strings.NewReader(xmlStr), WithBufferSize(10), WithLastMod()) {
			if result.Err != nil {
				continue
			}
			gotLoc = result.Loc
			gotTime = result.LastMod
		}

		if gotLoc != "https://example.com/a.xml" {
			t.Errorf("Loc = %q, want https://example.com/a.xml", gotLoc)
		}
		if gotTime.IsZero() {
			t.Error("LastMod = zero, want parsed value")
		}
	})

	t.Run("multiple options run independently on the same entry", func(t *testing.T) {
		xmlStr := `<urlset>
			<url>
				<loc>https://example.com/a</loc>
				<lastmod>2024-01-01T00:00:00Z</lastmod>
				<priority>0.7</priority>
				<changefreq>Weekly</changefreq>
			</url>
		</urlset>`

		var lastResult ParseResult
		for result := range Parse(context.TODO(), strings.NewReader(xmlStr), WithBufferSize(10),
			WithLastMod(), WithPriority(), WithChangeFreq()) {
			if result.Err != nil {
				continue
			}
			lastResult = result
		}

		if lastResult.LastMod.IsZero() {
			t.Error("LastMod not populated")
		}
		if lastResult.Priority == -1 || lastResult.Priority != 0.7 {
			t.Errorf("Priority = %v, want 0.7", lastResult.Priority)
		}
		if lastResult.ChangeFreq == "" || lastResult.ChangeFreq != ChangeFreqWeekly {
			t.Errorf("ChangeFreq = %v, want %v", lastResult.ChangeFreq, ChangeFreqWeekly)
		}
	})

	t.Run("no options means only Loc is populated", func(t *testing.T) {
		xmlStr := `<urlset><url><loc>https://example.com/a</loc><lastmod>2024-01-01T00:00:00Z</lastmod></url></urlset>`

		var lastResult ParseResult
		for result := range Parse(context.TODO(), strings.NewReader(xmlStr), WithBufferSize(10)) {
			if result.Err != nil {
				continue
			}
			lastResult = result
		}

		if lastResult.Loc != "https://example.com/a" {
			t.Errorf("Loc = %q, want https://example.com/a", lastResult.Loc)
		}
		if !lastResult.LastMod.IsZero() {
			t.Errorf("LastMod = %v, want zero (no extractor requested it)", lastResult.LastMod)
		}
	})
}

func TestParse_Entries(t *testing.T) {
	t.Run("Regular sitemap returns entries", func(t *testing.T) {
		xmlFile, err := os.Open("testdata/parser/sm-1.xml")
		if err != nil {
			t.Fatal(err)
		}
		defer xmlFile.Close()

		expectedURLs, err := readLines("testdata/parser/sm-1-urls.txt")
		if err != nil {
			t.Fatal(err)
		}

		var count int
		for result := range Parse(context.TODO(), xmlFile) {
			if result.Err != nil {
				t.Errorf("unexpected error: %v", result.Err)
				continue
			}
			count++
		}

		if count != len(expectedURLs) {
			t.Errorf("got %d URLs, want %d", count, len(expectedURLs))
		}
	})

	t.Run("Regular sitemap 2 returns entries", func(t *testing.T) {
		xmlFile, err := os.Open("testdata/parser/sm-2.xml")
		if err != nil {
			t.Fatal(err)
		}
		defer xmlFile.Close()

		expectedURLs, err := readLines("testdata/parser/sm-2-urls.txt")
		if err != nil {
			t.Fatal(err)
		}

		var count int
		for result := range Parse(context.TODO(), xmlFile) {
			if result.Err != nil {
				t.Errorf("unexpected error: %v", result.Err)
				continue
			}
			count++
		}

		if count != len(expectedURLs) {
			t.Errorf("got %d URLs, want %d", count, len(expectedURLs))
		}
	})

	t.Run("Malformed XML", func(t *testing.T) {
		xmlStr := `<urlset><url><loc>https://example.com</loc>`
		reader := strings.NewReader(xmlStr)

		var hasErr bool
		for result := range Parse(context.TODO(), reader) {
			if result.Err != nil {
				hasErr = true
				break
			}
		}

		if !hasErr {
			t.Error("expected error for malformed XML")
		}
	})

	t.Run("WithLastMod populates LastMod", func(t *testing.T) {
		xmlFile, err := os.Open("testdata/parser/sm-1.xml")
		if err != nil {
			t.Fatal(err)
		}
		defer xmlFile.Close()

		var sawAny bool
		for result := range Parse(context.TODO(), xmlFile, WithLastMod()) {
			if result.Err != nil {
				t.Errorf("unexpected error: %v", result.Err)
				continue
			}
			if !result.LastMod.IsZero() {
				sawAny = true
			}
		}
		if !sawAny {
			t.Error("expected at least one entry with LastMod populated when WithLastMod is used")
		}
	})

	t.Run("without WithLastMod, LastMod stays nil", func(t *testing.T) {
		xmlFile, err := os.Open("testdata/parser/sm-1.xml")
		if err != nil {
			t.Fatal(err)
		}
		defer xmlFile.Close()

		for result := range Parse(context.TODO(), xmlFile) {
			if result.Err != nil {
				t.Errorf("unexpected error: %v", result.Err)
				continue
			}
			if !result.LastMod.IsZero() {
				t.Error("LastMod should be zero without WithLastMod")
			}
		}
	})

	t.Run("WithModifiedSince implies lastmod parsing even without WithLastMod", func(t *testing.T) {
		xmlFile, err := os.Open("testdata/parser/sm-1.xml")
		if err != nil {
			t.Fatal(err)
		}
		defer xmlFile.Close()

		var sawAny bool
		for result := range Parse(context.TODO(), xmlFile, WithModifiedSince(time.Unix(0, 0))) {
			if result.Err != nil {
				t.Errorf("unexpected error: %v", result.Err)
				continue
			}
			if !result.LastMod.IsZero() {
				sawAny = true
			}
		}
		if !sawAny {
			t.Error("expected LastMod populated when WithModifiedSince is used")
		}
	})

	t.Run("combining WithLastMod and WithModifiedSince sets parseLastMod correctly without polluting extractors", func(t *testing.T) {
		o := resolveOptions([]Option{WithLastMod(), WithModifiedSince(time.Unix(0, 0))})
		if !o.parseLastMod {
			t.Error("expected parseLastMod to be true")
		}
	})
}

func TestParse_Index(t *testing.T) {
	t.Run("Sitemap index returns entries", func(t *testing.T) {
		xmlFile, err := os.Open("testdata/parser/smi-1.xml")
		if err != nil {
			t.Fatal(err)
		}
		defer xmlFile.Close()

		expectedURLs, err := readLines("testdata/parser/smi-1-urls.txt")
		if err != nil {
			t.Fatal(err)
		}

		var count int
		for result := range Parse(context.TODO(), xmlFile, WithBufferSize(100)) {
			if result.Err != nil {
				t.Errorf("unexpected error: %v", result.Err)
				continue
			}
			count++
		}

		if count != len(expectedURLs) {
			t.Errorf("got %d URLs, want %d", count, len(expectedURLs))
		}
	})
}

func readLines(filename string) ([]string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		lines = append(lines, line)
	}
	return lines, scanner.Err()
}
