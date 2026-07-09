package sitemap

import (
	"context"
	"io"
	"strconv"
)

// Caller must have already consumed the opening tag.
func processEntry(lx *xmlLexer, flags parseFlags, selfClose bool, locBuf, lastModBuf, changeFreqBuf, priorityBuf *[]byte) bool {
	if selfClose {
		return true
	}

	depth := 0
	curField := fieldNone

	for {
		dst, mode := selectDst(curField, locBuf, lastModBuf, changeFreqBuf, priorityBuf)
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
			curField = matchField(localName(lx.nameBuf), flags)
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

	locBuf := make([]byte, 0, 2048)
	var lastModBuf, changeFreqBuf, priorityBuf []byte
	if flags.lastMod {
		lastModBuf = make([]byte, 0, 128)
	}
	if flags.changeFreq {
		changeFreqBuf = make([]byte, 0, 32)
	}
	if flags.priority {
		priorityBuf = make([]byte, 0, 16)
	}

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

		locBuf = locBuf[:0]
		if flags.lastMod {
			lastModBuf = lastModBuf[:0]
		}
		if flags.changeFreq {
			changeFreqBuf = changeFreqBuf[:0]
		}
		if flags.priority {
			priorityBuf = priorityBuf[:0]
		}

		if !processEntry(lx, flags, selfClose, &locBuf, &lastModBuf, &changeFreqBuf, &priorityBuf) {
			yield(ParseResult{Err: ErrMalformedXML})
			return
		}

		result := ParseResult{Priority: -1}
		if len(locBuf) > 0 {
			result.Loc = string(locBuf)
		}
		if flags.lastMod && len(lastModBuf) > 0 {
			if t, err := ParseTime(string(lastModBuf)); err == nil {
				result.LastMod = t
			}
		}
		if flags.changeFreq && len(changeFreqBuf) > 0 {
			result.ChangeFreq = ChangeFreq(changeFreqBuf)
		}
		if flags.priority && len(priorityBuf) > 0 {
			if p, err := strconv.ParseFloat(string(priorityBuf), 64); err == nil {
				result.Priority = p
			}
		}

		if !yield(result) {
			return
		}
	}
}
