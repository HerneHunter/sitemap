package sitemap

import (
	"context"
	"encoding/xml"
	"io"
)

func parseSafe(ctx context.Context, reader io.Reader, flags parseFlags, kindOut *bool, yield func(ParseResult) bool) {
	decoder := xml.NewDecoder(reader)
	detected := false
	var isIndex bool

	// Pre-allocate buffers via the shared helper so allocation/reset/drain
	// logic is no longer duplicated with parsefast.go. xml.Decoder can split
	// text across multiple CharData tokens, so we accumulate manually.
	b := newEntryBuffers(flags)

	for {
		token, err := decoder.RawToken()
		if err == io.EOF {
			break
		}
		if err != nil {
			if !yield(ParseResult{Err: ErrMalformedXML}) {
				return
			}
			return
		}

		startElement, ok := token.(xml.StartElement)
		if !ok {
			continue
		}

		local := startElement.Name.Local
		if !detected {
			// Some generators incorrectly prefix standard tags with sm: instead of setting a default namespace
			switch local {
			case "sitemap", "sm:sitemap", "sitemapindex":
				isIndex = true
				*kindOut = true
				detected = true
			case "url", "sm:url", "urlset":
				detected = true
			default:
				continue
			}
		}
		if (isIndex && (local == "sitemap" || local == "sm:sitemap")) ||
			(!isIndex && (local == "url" || local == "sm:url")) {

			select {
			case <-ctx.Done():
				yield(ParseResult{Err: ctx.Err()})
				return
			default:
			}

			b.reset()

			depth := 0
			curField := fieldNone

		inner:
			for {
				t, err := decoder.RawToken()
				if err != nil {
					yield(ParseResult{Err: ErrMalformedXML})
					return
				}
				switch tok := t.(type) {
				case xml.StartElement:
					depth++
					if depth == 1 {
						tag := tok.Name.Local
						if len(tag) > 3 && tag[0:3] == "sm:" {
							tag = tag[3:]
						}
						curField = matchField([]byte(tag), flags)
					} else {
						curField = fieldNone
					}
				case xml.CharData:
					if depth == 1 {
						switch curField {
						case fieldLoc:
							b.locBuf = append(b.locBuf, tok...)
						case fieldLastMod:
							b.lastModBuf = append(b.lastModBuf, tok...)
						case fieldChangeFreq:
							// ASCII lowercase on the fly to save an allocation later
							for _, byt := range tok {
								if byt >= 'A' && byt <= 'Z' {
									byt += 'a' - 'A'
								}
								b.changeFreqBuf = append(b.changeFreqBuf, byt)
							}
						case fieldPriority:
							b.priorityBuf = append(b.priorityBuf, tok...)
						}
					}
				case xml.EndElement:
					if depth == 0 {
						break inner
					}
					depth--
					curField = fieldNone
				}
			}

			result := b.buildResult()

			if !yield(result) {
				return
			}
		}
	}
}
