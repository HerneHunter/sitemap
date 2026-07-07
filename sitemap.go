package sitemap

import (
	"context"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	defaultBufferSize     = 2500
	defaultMaxConcurrency = 20
)

// Option configures the streaming functions in this package.
type Option func(*options)

type options struct {
	bufferSize      int
	maxConcurrency  int
	since           time.Time
	httpClient      *http.Client
	parseLastMod    bool
	parseChangeFreq bool
	parsePriority   bool
	sitemapFilter   *Filter
	sitemapIndexFilter *Filter
}

func resolveOptions(opts []Option) options {
	o := options{
		bufferSize:     defaultBufferSize,
		maxConcurrency: defaultMaxConcurrency,
	}
	for _, opt := range opts {
		opt(&o)
	}
	return o
}

func (o *options) flags() parseFlags {
	return parseFlags{
		lastMod:    o.parseLastMod,
		changeFreq: o.parseChangeFreq,
		priority:   o.parsePriority,
	}
}

// WithBufferSize overrides the output channel buffer size.
func WithBufferSize(n int) Option {
	return func(o *options) {
		if n > 0 {
			o.bufferSize = n
		}
	}
}

// WithMaxConcurrency overrides the max number of concurrent fetches.
func WithMaxConcurrency(n int) Option {
	return func(o *options) {
		if n > 0 {
			o.maxConcurrency = n
		}
	}
}

// WithLastMod parses <lastmod> into Entry.LastMod.
func WithLastMod() Option {
	return func(o *options) { o.parseLastMod = true }
}

// WithChangeFreq parses <changefreq> into Entry.ChangeFreq.
func WithChangeFreq() Option {
	return func(o *options) { o.parseChangeFreq = true }
}

// WithPriority parses <priority> into Entry.Priority.
func WithPriority() Option {
	return func(o *options) { o.parsePriority = true }
}

// WithModifiedSince drops entries modified before t, implies WithLastMod.
// Jalali time is converted to Gregorian automatically.
func WithModifiedSince(t time.Time) Option {
	return func(o *options) {
		if !t.IsZero() && looksLikeJalali(t.Year()) {
			t = jalaliToGregorian(t.Year(), int(t.Month()), t.Day(), t.Hour(), t.Minute(), t.Second())
		}
		o.parseLastMod = true
		o.since = t
	}
}

// WithSitemapFilter applies a filter to sitemap entries.
func WithSitemapFilter(f *Filter) Option {
	return func(o *options) { o.sitemapFilter = f }
}

// WithSitemapIndexFilter applies a filter to sitemap index entries.
func WithSitemapIndexFilter(f *Filter) Option {
	return func(o *options) { o.sitemapIndexFilter = f }
}

// Parse parses sitemap/index entries from reader, fetching children concurrently.
func Parse(ctx context.Context, reader io.Reader, opts ...Option) <-chan ParseResult {
	o := resolveOptions(opts)
	outCh := make(chan ParseResult, o.bufferSize)

	go func() {
		defer close(outCh)
		var wg sync.WaitGroup
		sem := make(chan struct{}, o.maxConcurrency)
		consumeReader(ctx, reader, o, outCh, &wg, sem)
		wg.Wait()
	}()

	return outCh
}

// ParseFile parses entries from a local sitemap/index file, closing it when done.
func ParseFile(ctx context.Context, filepath string, opts ...Option) <-chan ParseResult {
	f, err := os.Open(filepath)
	if err != nil {
		out := make(chan ParseResult, 1)
		out <- ParseResult{Err: err}
		close(out)
		return out
	}
	// closeIfCloser inside consumeReader handles the normal path, but we need
	// this defer as a safety net in case the context is already cancelled and
	// the goroutine never reaches it. Double-close on *os.File is safe.
	defer f.Close()
	return Parse(ctx, f, opts...)
}

// Fetch retrieves entries from url, recursing into child sitemaps as needed.
func Fetch(ctx context.Context, url string, opts ...Option) <-chan ParseResult {
	return FetchAll(ctx, []string{url}, opts...)
}

// FetchAll fetches multiple URLs concurrently into a single combined stream.
func FetchAll(ctx context.Context, urls []string, opts ...Option) <-chan ParseResult {
	o := resolveOptions(opts)
	out := make(chan ParseResult, o.bufferSize)

	go func() {
		defer close(out)
		var wg sync.WaitGroup
		sem := make(chan struct{}, o.maxConcurrency)

		for _, url := range urls {
			scheduleFetch(ctx, url, o, out, &wg, sem)
		}
		wg.Wait()
	}()

	return out
}

func closeIfCloser(r io.Reader) {
	if rc, ok := r.(io.ReadCloser); ok {
		rc.Close()
	}
}

func openLoc(ctx context.Context, client *http.Client, loc string) (io.Reader, error) {
	if strings.HasPrefix(loc, "http://") || strings.HasPrefix(loc, "https://") {
		return fetchUrl(ctx, client, loc)
	}
	return os.Open(loc)
}

func scheduleFetch(ctx context.Context, loc string, o options, out chan<- ParseResult, wg *sync.WaitGroup, sem chan struct{}) {
	wg.Go(func() {
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			return
		}
		defer func() { <-sem }()
		fetchRecursive(ctx, loc, o, out, wg, sem)
	})
}

func fetchRecursive(ctx context.Context, loc string, o options, out chan<- ParseResult, wg *sync.WaitGroup, sem chan struct{}) {
	reader, err := openLoc(ctx, o.httpClient, loc)
	if err != nil {
		out <- ParseResult{Err: err}
		return
	}
	consumeReader(ctx, reader, o, out, wg, sem)
}

func consumeReader(ctx context.Context, reader io.Reader, o options, out chan<- ParseResult, wg *sync.WaitGroup, sem chan struct{}) {
	defer closeIfCloser(reader)

	var isIndex bool
	parse(ctx, reader, o.flags(), &isIndex, func(result ParseResult) bool {
		if err := ctx.Err(); err != nil {
			out <- ParseResult{Err: err}
			return false
		}

		switch {
		case result.Err != nil:
			out <- result
		case !o.since.IsZero() && shouldSkip(result.LastMod, o.since):
		case isIndex && !o.sitemapIndexFilter.match(result.Loc):
		case !isIndex && !o.sitemapFilter.match(result.Loc):
		case isIndex:
			scheduleFetch(ctx, result.Loc, o, out, wg, sem)
		default:
			out <- result
		}
		return true
	})
}

func shouldSkip(lastMod time.Time, since time.Time) bool {
	return !lastMod.IsZero() && lastMod.Before(since)
}
