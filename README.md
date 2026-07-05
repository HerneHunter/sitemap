# sitemap

A fast and memory-efficient XML sitemap parser for Go. It streams sitemaps and sitemap indexes incrementally via channels instead of loading the entire document into memory.

## Features

- **Automatic Recursion**: Transparently handles sitemap indexes by fetching child sitemaps concurrently.
- **Selective Fields & Filtering**: Optionally extract extra XML fields or filter URLs by their last modified date.
- **Jalali Date Support**: Automatically detects and parses Jalali dates.
- **Customizable Requests**: Fully supports custom HTTP clients for proxies, timeouts, and specific configurations.

## Performance

Results from benchmarking a 60 MB sitemap file (156,520 `<loc>` entries) alongside a few other Go packages:

| Package | Speed | Mem/Op | Allocs/Op | Peak memory |
| :--- | :--- | :--- | :--- | :--- |
| `github.com/HerneHunter/sitemap` | ~1.68× | 180.4 MB | 6.73M | 13.60 MB |
| `github.com/oxffaa/gopher-parse-sitemap` | ~1.44× | 377.2 MB | 11.11M | 13.75 MB |
| `github.com/yuya-matsushima/go-sitemap` | ~1.36× | 406.9 MB | 10.80M | 179.57 MB |
| `github.com/snabb/sitemap` | ~1.29× | 370.9 MB | 10.96M | 54.69 MB |
| `github.com/aafeher/go-sitemap-parser` | 1× | 1.11 GB | 16.43M | 410.96 MB |

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
