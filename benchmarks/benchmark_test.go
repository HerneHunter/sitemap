package benchmark

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"sort"
	"sync"
	"testing"
	"time"

	hernehunter "github.com/HerneHunter/sitemap"

	aafeher "github.com/aafeher/go-sitemap-parser"
	oxffaa "github.com/oxffaa/gopher-parse-sitemap"
	snabb "github.com/snabb/sitemap"
	yterajima "github.com/yterajima/go-sitemap"
)

const numURLs = 50_000

var (
	sitemapOnce sync.Once
	sitemapData []byte
)

// generateSitemap builds the payload at runtime rather than committing a large XML fixture.
func generateSitemap(n int) []byte {
	var buf bytes.Buffer
	buf.Grow(n * 230)

	buf.WriteString(`<?xml version="1.0" encoding="UTF-8"?><?xml-stylesheet type="text/xsl" href="https://example.com/main-sitemap.xsl"?>
<urlset xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
        xmlns:image="http://www.google.com/schemas/sitemap-image/1.1"
        xsi:schemaLocation="http://www.sitemaps.org/schemas/sitemap/0.9 http://www.sitemaps.org/schemas/sitemap/0.9/sitemap.xsd http://www.google.com/schemas/sitemap-image/1.1 http://www.google.com/schemas/sitemap-image/1.1/sitemap-image.xsd"
        xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
`)

	base := time.Now()

	fmt.Fprintf(&buf, `    <url>
        <loc>https://example.com</loc>
        <lastmod>%s</lastmod>
        <changefreq>daily</changefreq>
        <priority>1.0</priority>
    </url>
`, base.Format(time.RFC3339))

	freqs := [2]string{"weekly", "monthly"}
	for i := 1; i < n; i++ {
		// Vary fields so parsers can't shortcut on identical repeated content.
		freq := freqs[i%2]
		priority := 0.8
		if i%3 == 0 {
			priority = 0.6
		}
		lastmod := base.Add(-time.Duration(i) * time.Hour)

		fmt.Fprintf(&buf, `    <url>
        <loc>https://example.com/posts/post-%d</loc>
        <lastmod>%s</lastmod>
        <changefreq>%s</changefreq>
        <priority>%.1f</priority>
        <image:image>
            <image:loc>https://example.com.com/images/posters/img-%d.jpg</image:loc>
        </image:image>
    </url>
`, i, lastmod.Format(time.RFC3339), freq, priority, i)
	}

	buf.WriteString(`</urlset>`)
	return buf.Bytes()
}

func loadData(tb testing.TB) []byte {
	tb.Helper()
	sitemapOnce.Do(func() {
		// Generated once and shared so setup cost doesn't leak into the measured iterations.
		sitemapData = generateSitemap(numURLs)
		fmt.Fprintf(os.Stderr, "generated sitemap: %d bytes (%.1f MB), %d urls\n",
			len(sitemapData), float64(len(sitemapData))/1024/1024, numURLs)
	})
	return sitemapData
}

func collectHerneHunter(t *testing.T, data []byte) []string {
	ctx := context.Background()
	reader := bytes.NewReader(data)
	resCh := hernehunter.Parse(ctx, reader, hernehunter.WithCustomLexer())
	var got []string
	for r := range resCh {
		if r.Err != nil {
			t.Fatalf("hernehunter parse error: %v", r.Err)
		}
		got = append(got, r.Loc)
	}
	return got
}

func collectOxffaa(t *testing.T, data []byte) []string {
	reader := bytes.NewReader(data)
	var got []string
	err := oxffaa.Parse(reader, func(e oxffaa.Entry) error {
		got = append(got, e.GetLocation())
		return nil
	})
	if err != nil {
		t.Fatalf("oxffaa parse error: %v", err)
	}
	return got
}

func collectSnabb(t *testing.T, data []byte) []string {
	reader := bytes.NewReader(data)
	sm := snabb.New()
	if _, err := sm.ReadFrom(reader); err != nil {
		t.Fatalf("snabb parse error: %v", err)
	}
	var got []string
	for _, u := range sm.URLs {
		got = append(got, u.Loc)
	}
	return got
}

func collectMatsushima(t *testing.T, data []byte) []string {
	sm, err := yterajima.Parse(data)
	if err != nil {
		t.Fatalf("yterajima parse error: %v", err)
	}
	var got []string
	for _, u := range sm.URL {
		got = append(got, u.Loc)
	}
	return got
}

func collectAafeher(t *testing.T, data []byte) []string {
	content := string(data)
	s := aafeher.New().SetMultiThread(false)
	s, err := s.Parse("http://localhost/sitemap.xml", &content)
	if err != nil {
		t.Fatalf("aafeher parse error: %v", err)
	}
	var got []string
	for _, u := range s.GetURLs() {
		got = append(got, u.Loc)
	}
	return got
}

// Sanity check: all parsers must agree on the extracted URLs before we trust
// the timings. Otherwise a fast but broken parser looks great next to a correct
// but slow one. We compare full content, not just the count, because two parsers
// can return the same number of URLs and still disagree on the values.
func TestAllLibrariesProduceSameLocs(t *testing.T) {
	data := loadData(t)

	type result struct {
		name string
		locs []string
	}

	results := []result{
		{"HerneHunter", collectHerneHunter(t, data)},
		{"Oxffaa", collectOxffaa(t, data)},
		{"Snabb", collectSnabb(t, data)},
		{"Matsushima/yterajima", collectMatsushima(t, data)},
		{"Aafeher", collectAafeher(t, data)},
	}

	// Extraction order isn't guaranteed to match across libraries, so sort first.
	// We only care that the URL sets agree, not the order they came in.
	for i := range results {
		sort.Strings(results[i].locs)
	}

	ref := results[0]
	failed := false

	for i := 1; i < len(results); i++ {
		if len(results[i].locs) != len(ref.locs) {
			t.Errorf("Count mismatch: %s has %d URLs, but %s has %d URLs",
				ref.name, len(ref.locs), results[i].name, len(results[i].locs))
			failed = true
			continue
		}
		for j := range ref.locs {
			if ref.locs[j] != results[i].locs[j] {
				t.Errorf("Mismatch at index %d: %s=%q vs %s=%q",
					j, ref.name, ref.locs[j], results[i].name, results[i].locs[j])
				failed = true
				break
			}
		}
	}

	if failed {
		t.Fatalf("Test failed: parsers produced different URL sets.")
	}
}

func BenchmarkHerneHunterCustomLexer(b *testing.B) {
	data := loadData(b)
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()

	reader := bytes.NewReader(data)
	ctx := context.Background()
	b.ResetTimer()

	for b.Loop() {
		reader.Reset(data)
		resCh := hernehunter.Parse(ctx, reader,
			hernehunter.WithChangeFreq(),
			hernehunter.WithLastMod(),
			hernehunter.WithPriority(),
			hernehunter.WithCustomLexer(),
		)
		for range resCh {
		}
	}

	b.ReportMetric(float64(b.N)*float64(numURLs)/b.Elapsed().Seconds(), "urls/s")
}

func BenchmarkHerneHunterCustomLexerLocOnly(b *testing.B) {
	data := loadData(b)
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()

	reader := bytes.NewReader(data)
	ctx := context.Background()
	b.ResetTimer()

	for b.Loop() {
		reader.Reset(data)
		resCh := hernehunter.Parse(ctx, reader, hernehunter.WithCustomLexer())
		for range resCh {
		}
	}

	b.ReportMetric(float64(b.N)*float64(numURLs)/b.Elapsed().Seconds(), "urls/s")
}

func BenchmarkHerneHunter(b *testing.B) {
	data := loadData(b)
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()

	reader := bytes.NewReader(data)
	ctx := context.Background()
	b.ResetTimer()

	for b.Loop() {
		reader.Reset(data)
		resCh := hernehunter.Parse(ctx, reader,
			hernehunter.WithChangeFreq(),
			hernehunter.WithLastMod(),
			hernehunter.WithPriority(),
		)
		for range resCh {
		}
	}

	b.ReportMetric(float64(b.N)*float64(numURLs)/b.Elapsed().Seconds(), "urls/s")
}

// Like BenchmarkHerneHunter but with no With* options, so only Loc gets parsed.
// Lets us isolate the cost of the optional fields against the bare minimum.
func BenchmarkHerneHunterLocOnly(b *testing.B) {
	data := loadData(b)
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()

	reader := bytes.NewReader(data)
	ctx := context.Background()
	b.ResetTimer()

	for b.Loop() {
		reader.Reset(data)
		resCh := hernehunter.Parse(ctx, reader)
		for range resCh {
		}
	}

	b.ReportMetric(float64(b.N)*float64(numURLs)/b.Elapsed().Seconds(), "urls/s")
}

func BenchmarkOxffaa(b *testing.B) {
	data := loadData(b)
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()

	reader := bytes.NewReader(data)
	b.ResetTimer()

	for b.Loop() {
		reader.Reset(data)
		_ = oxffaa.Parse(reader, func(e oxffaa.Entry) error {
			_ = e.GetLocation()
			_ = e.GetLastModified()
			_ = e.GetChangeFrequency()
			_ = e.GetPriority()
			return nil
		})
	}

	b.ReportMetric(float64(b.N)*float64(numURLs)/b.Elapsed().Seconds(), "urls/s")
}

func BenchmarkSnabb(b *testing.B) {
	data := loadData(b)
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()

	reader := bytes.NewReader(data)
	b.ResetTimer()

	for b.Loop() {
		reader.Reset(data)
		sm := snabb.New()
		_, _ = sm.ReadFrom(reader)
	}

	b.ReportMetric(float64(b.N)*float64(numURLs)/b.Elapsed().Seconds(), "urls/s")
}

func BenchmarkMatsushima(b *testing.B) {
	data := loadData(b)
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_, _ = yterajima.Parse(data)
	}

	b.ReportMetric(float64(b.N)*float64(numURLs)/b.Elapsed().Seconds(), "urls/s")
}

func BenchmarkAafeher(b *testing.B) {
	data := loadData(b)
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		content := string(data)
		s := aafeher.New()
		_, _ = s.Parse("http://localhost/sitemap.xml", &content)
	}

	b.ReportMetric(float64(b.N)*float64(numURLs)/b.Elapsed().Seconds(), "urls/s")
}
