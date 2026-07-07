# sitemap

A fast and memory-efficient XML sitemap parser for Go. It streams sitemaps and sitemap indexes incrementally via channels instead of loading the entire document into memory.

## Features

- **Automatic Recursion**: Transparently handles sitemap indexes by fetching child sitemaps concurrently.
- **Streaming, Low-Allocation Parsing**: Entries are streamed via channels as they're tokenized, keeping memory and allocations low even on large sitemaps.
- **Selective Field Extraction**: Parse only the fields you need (`<lastmod>`, `<changefreq>`, `<priority>`) each opt-in via its own option, so you don't pay for parsing you don't use.
- **Two Filtering Modes**:
  - Date-based, via `WithModifiedSince` (drops entries older than a cutoff).
  - Regex-based, via `WithSitemapFilter` / `WithSitemapIndexFilter` (whitelist/blacklist by URI or raw URL).
- **Jalali Date Support**: Automatically detects and parses Jalali dates, both in `<lastmod>` values and in `WithModifiedSince` cutoffs.
- **Customizable Requests**: Fully supports custom HTTP clients for proxies, timeouts, and specific configurations.

## Performance

Results from benchmarking a sitemap with 50,000 `<loc>` entries (the standard maximum for a single sitemap) alongside a few other Go packages:

| Package | Time/op | Throughput | Mem/Op | Allocs/Op |
| :--- | :--- | :--- | :--- | :--- |
| `github.com/HerneHunter/sitemap` (loc only) | 47.65 ms | 1.049M urls/s | 2.52 MB | 50,010 |
| `github.com/HerneHunter/sitemap` | 78.90 ms | 633.7k urls/s | 4.43 MB | 150,000 |
| `github.com/oxffaa/gopher-parse-sitemap` | 609.80 ms | 82.00k urls/s | 120.90 MB | 3,600,000 |
| `github.com/snabb/sitemap` | 615.80 ms | 81.20k urls/s | 117.70 MB | 3,500,000 |
| `github.com/yuya-matsushima/go-sitemap` | 616.10 ms | 81.16k urls/s | 128.90 MB | 3,450,000 |
| `github.com/aafeher/go-sitemap-parser` | 894.80 ms | 55.88k urls/s | 346.30 MB | 5,250,000 |

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

### `WithBufferSize(n int)`

Sets the output channel buffer size (default `2500`).

```go
sitemap.Fetch(ctx, reader, sitemap.WithBufferSize(500))
```

### `WithMaxConcurrency(n int)`

Overrides the maximum number of concurrent sitemap fetches when recursing through sitemap indexes (`Fetch`, `FetchAll`). (default `20`).

```go
sitemap.Fetch(ctx, url, sitemap.WithMaxConcurrency(50))
```

### `WithLastMod()`

Parses each entry's `<lastmod>` field into `LastMod`. Off by default,
since parsing timestamps you don't need has a cost.

```go
for result := range sitemap.Parse(ctx, reader, sitemap.WithLastMod()) {
	fmt.Println(result.Loc, result.LastMod)
}
```

### `WithChangeFreq()`

Parses each entry's `<changefreq>` field into `ChangeFreq`.

### `WithPriority()`

Parses each entry's `<priority>` field into `Priority`. If missing or this option is not used, it defaults to `-1`.

### `WithModifiedSince(t time.Time)`

Drops entries last modified before `t`. Implies `WithLastMod()`, so
you don't need to pass both. Entries with no `<lastmod>` are always kept,
since there's no way to know their age. If `t` looks like a Jalali date, it's converted to Gregorian automatically.

```go
cutoff := time.Now().AddDate(0, -1, 0) // last 30 days
sitemap.Fetch(ctx, url, sitemap.WithModifiedSince(cutoff))
```

### `WithSitemapFilter(f *sitemap.Filter)`

Applies regex-based filtering to skip specific URLs from parsed sitemaps. The `Filter` struct supports whitelisting and blacklisting against the URI (path + query), or raw string matching against the full URL.

```go
filter := &sitemap.Filter{
	Blacklist: []*regexp.Regexp{
		regexp.MustCompile(`^/admin`),
		regexp.MustCompile(`\?sort=`),
	},
}
sitemap.Fetch(ctx, url, sitemap.WithSitemapFilter(filter))
```

### `WithSitemapIndexFilter(f *sitemap.Filter)`

Similar to `WithSitemapFilter`, but applies to URLs within a sitemap index. Sitemaps that are filtered out will not be fetched or parsed.

### `WithHTTPClient(c *http.Client)`

Overrides the HTTP client used for fetching (`Fetch`, `FetchAll`,
`FetchIndex`). Has no effect on `Parse` or `ParseFile`,
which work on a reader or file you already have.

```go
client := &http.Client{Timeout: 5 * time.Minute}
sitemap.Fetch(ctx, url, sitemap.WithHTTPClient(client))
```

## Result and error handling

Every function returns/streams `ParseResult` values. A result carries the parsed fields (like `Loc`, `LastMod`, `ChangeFreq`, `Priority`) or an `Err`.

Errors you may see:

- `ErrMalformedXML`: the document couldn't be tokenized.
- `HTTPError`: a fetch returned a non-2xx status code.

If a `<lastmod>` value is present but fails to parse, it's not treated as an
error: `LastMod` is simply left as its zero value (`time.Time{}`), same as when the field is
missing entirely. This means if you are using a filter like `WithModifiedSince`,
URLs with unparsable timestamps are not filtered out. They will still appear
in the output channel with a zero value `LastMod`, leaving it up to you to decide whether to keep or drop them in your own code.
