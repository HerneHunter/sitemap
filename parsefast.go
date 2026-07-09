package sitemap

import (
	"context"
	"io"
)

// Caller must have already consumed the opening tag.
func processEntry(lx *xmlLexer, b *entryBuffers, selfClose bool) bool {
	if selfClose {
		return true
	}

	depth := 0
	curField := fieldNone

	for {
		dst, mode := b.dst(curField)
		isEnd, sc, ok := lx.nextEvent(dst, mode)
		if !ok {
			return false
		}
		if isEnd {
			if depth == 0 {
				return true
			}
			depth--
			curField = fieldNone
			continue
		}

		depth++
		// We only extract text from immediate children (depth 1). Nested tags are ignored.
		if depth == 1 {
			curField = matchField(localName(lx.nameBuf), b.flags)
		} else {
			curField = fieldNone
		}
		if sc {
			depth--
			curField = fieldNone
		}
	}
}

func parseWithCustomLexer(ctx context.Context, reader io.Reader, flags parseFlags, kindOut *bool, yield func(ParseResult) bool) {
	lx := newXMLLexer(reader)

	b := newEntryBuffers(flags)

	detected := false
	isIndex := false

	for {
		isEnd, selfClose, ok := lx.nextEvent(nil, capNone)
		if !ok {
			if lx.truncated {
				yield(ParseResult{Err: ErrMalformedXML})
			}
			return
		}
		if isEnd {
			continue
		}

		name := string(localName(lx.nameBuf))
		if !detected {
			switch name {
			case "sitemap", "sitemapindex":
				isIndex = true
				*kindOut = true
				detected = true
			case "url", "urlset":
				detected = true
			default:
				continue
			}
			// skip the root tag itself
			continue
		}

		matchedEntry := (isIndex && name == "sitemap") || (!isIndex && name == "url")
		if !matchedEntry {
			continue
		}

		select {
		// Bail early on massive files if the caller gives up
		case <-ctx.Done():
			yield(ParseResult{Err: ctx.Err()})
			return
		default:
		}

		b.reset()

		if !processEntry(lx, &b, selfClose) {
			yield(ParseResult{Err: ErrMalformedXML})
			return
		}

		result := b.buildResult()

		if !yield(result) {
			return
		}
	}
}
