# sitemap

<p align="center">
  <a href="https://pkg.go.dev/github.com/HerneHunter/sitemap"><img src="https://pkg.go.dev/badge/github.com/HerneHunter/sitemap.svg" alt="Go Reference"></a>
  <a href="https://github.com/HerneHunter/sitemap/actions/workflows/go-ci.yml"><img src="https://github.com/HerneHunter/sitemap/actions/workflows/go-ci.yml/badge.svg" alt="Build Status"></a>
  <a href="https://codecov.io/gh/HerneHunter/sitemap"><img src="https://codecov.io/gh/HerneHunter/sitemap/graph/badge.svg" alt="codecov"></a>
</p>

A fast, memory-efficient XML sitemap parser for Go. It tokenizes sitemaps and sitemap indexes incrementally, reusing buffers across entries, and streams results via channels without loading entire documents into memory.

## Features

- **Automatic Recursion**: Handles sitemap indexes by fetching and parsing child sitemaps concurrently.
- **Streaming & Low-Allocation**: Entries are streamed via channels as they are tokenized, avoiding the need to hold the entire document in memory at once.
- **Dual Parsing Modes**: Defaults to the reliable standard library `encoding/xml`, with an opt-in ultra-fast custom lexer for maximum throughput.
- **Selective Extraction**: Extract only the fields you need (like `<lastmod>`, `<changefreq>`, or `<priority>`). Each field is opt-in, so you only pay for the parsing you actually use.
- **Flexible Filtering**:
  - **Date-based**: Drop entries older than a specific cutoff.
  - **Regex-based**: Whitelist or blacklist specific URIs or URLs.
- **Jalali Date Support**: Automatically detects and parses Jalali dates in both `<lastmod>` values and `WithModifiedSince` cutoffs.
- **Customizable HTTP Requests**: Fully supports custom HTTP clients, allowing you to easily configure proxies, timeouts, and other transport settings.

## Performance

The following results are from benchmarking a single sitemap containing 50,000 `<loc>` entries
(the maximum URLs allowed in a sitemap per the sitemaps.org protocol).

| Mode | Time/op | Urls/s | Mem/Op | Allocs/Op |
| :--- | :--- | :--- | :--- | :--- |
| Custom lexer (loc only) | 47.01 ms | 1.064M | 2.52 MB | 50,009 |
| Custom lexer (full) | 76.32 ms | 655.1k | 4.43 MB | 150,012 |
| Standard `encoding/xml` (loc only) | 476.0 ms | 105.0k | 57.42 MB | 2,150,021 |
| Standard `encoding/xml` (full) | 521.9 ms | 95.80k | 58.94 MB | 2,250,021 |

*(full: `<loc>` + `<lastmod>` + `<changefreq>` + `<priority>`)*

For reference, here are benchmark numbers for other Go sitemap packages:

| Package | Time/op | Urls/s | Mem/Op | Allocs/Op |
| :--- | :--- | :--- | :--- | :--- |
| `github.com/snabb/sitemap` | 611.0 ms | 81.84k | 117.70 MB | 3,500,053 |
| `github.com/yuya-matsushima/go-sitemap` | 611.2 ms | 81.81k | 128.90 MB | 3,450,053 |
| `github.com/oxffaa/gopher-parse-sitemap` | 615.7 ms | 81.21k | 120.90 MB | 3,600,027 |
| `github.com/aafeher/go-sitemap-parser` | 887.8 ms | 56.32k | 346.30 MB | 5,250,117 |

Run it yourself in [`/benchmarks`](./benchmarks), with:

```bash
go test -bench . -benchtime=5s -benchmem -count=6
```

## Install

```bash
go get github.com/HerneHunter/sitemap
```

## Quick start

### Fetch a sitemap (or sitemap index) over HTTP

`Fetch` follows sitemap indexes recursively and streams only the final page
URLs:

```go
package main

import (
	"context"
	"fmt"

	"github.com/HerneHunter/sitemap"
)

func main() {
	ctx := context.Background()

	for result := range sitemap.Fetch(ctx, "https://example.com/sitemap_index.xml") {
		fmt.Println(result.Loc)
	}
}
```

`FetchAll` does the same for several starting URLs concurrently, merging
everything into one channel.

### Parse from a local file

```go
for result := range sitemap.ParseFile(ctx, "sitemap.xml") {
	fmt.Println(result.Loc)
}
```

### Parse from any `io.Reader`

```go
// reader could be an os.File, strings.Reader, etc.
for result := range sitemap.Parse(ctx, reader) {
	fmt.Println(result.Loc)
}
```

## Options

All streaming/fetching functions take `...Option`. They can be combined freely.

| Option | Description | Default |
| :--- | :--- | :--- |
| `WithBufferSize(n int)` | Sets the output channel buffer size | `2500` |
| `WithMaxConcurrency(n int)` | Max concurrent sitemap fetches when recursing through indexes (`Fetch`, `FetchAll`) | `20` |
| `WithCustomLexer()` | Uses the high-performance custom XML lexer instead of `encoding/xml` | off (`encoding/xml`) |
| `WithLastMod()` | Parses `<lastmod>` into `LastMod` | off |
| `WithChangeFreq()` | Parses `<changefreq>` into `ChangeFreq` | off |
| `WithPriority()` | Parses `<priority>` into `Priority` | off (`-1` if unset) |
| `WithModifiedSince(t time.Time)` | Drops entries last modified before `t` (implies `WithLastMod()`) | off |
| `WithSitemapFilter(f *Filter)` | Regex whitelist/blacklist on sitemap entry URLs | off |
| `WithSitemapIndexFilter(f *Filter)` | Regex whitelist/blacklist on sitemap index URLs | off |
| `WithHTTPClient(c *http.Client)` | Overrides the HTTP client used for fetching | `http.DefaultClient` |

Every option follows the same pattern. pass it in as an argument, wherever it fits in the call:

```go
sitemap.Fetch(ctx, url, sitemap.WithLastMod())
```

The rest work exactly the same way. Combine as many as you need:

```go
cutoff := time.Now().AddDate(0, -1, 0) // last 30 days

sitemap.Fetch(ctx, url,
	sitemap.WithCustomLexer(),
	sitemap.WithChangeFreq(),
	sitemap.WithPriority(),
	sitemap.WithModifiedSince(cutoff),
	sitemap.WithMaxConcurrency(5),
)
```

### Filters

`WithSitemapFilter` and `WithSitemapIndexFilter` both take a `*Filter`, which is a bit richer than the other options since it has three independent fields:

```go
// Filter provides rules for filtering sitemap or index URLs.
type Filter struct {
	// Applied to the URI (path + query). If set, the URI must match at least one.
	Whitelist []*regexp.Regexp

	// Applied to the URI (path + query). If set, the URI must not match any.
	Blacklist []*regexp.Regexp

	// Applied to the full loc string, including the domain.
	RawMatch *regexp.Regexp
}
```

```go
filter := &sitemap.Filter{
	Blacklist: []*regexp.Regexp{
		regexp.MustCompile(`^/admin`),
		regexp.MustCompile(`\?sort=`),
	},
}
sitemap.Fetch(ctx, url, sitemap.WithSitemapFilter(filter))
```

`WithSitemapIndexFilter` works the same way, but applies to URLs within a sitemap index. sitemaps filtered out here are never fetched or parsed.

## Result and error handling

Every function returns/streams `ParseResult` values. A result carries the parsed fields (like `Loc`, `LastMod`, `ChangeFreq`, `Priority`) or an `Err`.

Errors you may see:

- `ErrMalformedXML`: the document couldn't be tokenized.
- `*HTTPError`: a fetch returned a non-2xx status code. Exposes `URL` and `StatusCode`.

If a `<lastmod>` value is present but fails to parse, it's not treated as an error: `LastMod` is simply left as its zero value (`time.Time{}`), same as when the field is missing entirely. So with `WithModifiedSince`, URLs with unparsable timestamps are not filtered out. They still appear in the output channel with a zero-value `LastMod`, leaving it up to your own code to decide whether to keep or drop them.