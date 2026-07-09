package sitemap

import (
	"context"
	"encoding/xml"
	"io"
	"strconv"
)


func parseSafe(ctx context.Context, reader io.Reader, flags parseFlags, kindOut *bool, yield func(ParseResult) bool) {
	decoder := xml.NewDecoder(reader)
	detected := false
	var isIndex bool

	// Pre-allocate buffers. xml.Decoder can split text across multiple CharData tokens,
	// so we have to accumulate it manually.
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
						if tag == "loc" {
							curField = fieldLoc
						} else if flags.lastMod && tag == "lastmod" {
							curField = fieldLastMod
						} else if flags.changeFreq && tag == "changefreq" {
							curField = fieldChangeFreq
						} else if flags.priority && tag == "priority" {
							curField = fieldPriority
						} else {
							curField = fieldNone
						}
					} else {
						curField = fieldNone
					}
				case xml.CharData:
					if depth == 1 {
						switch curField {
						case fieldLoc:
							locBuf = append(locBuf, tok...)
						case fieldLastMod:
							lastModBuf = append(lastModBuf, tok...)
						case fieldChangeFreq:
							// ASCII lowercase on the fly to save an allocation later
							for _, b := range tok {
								if b >= 'A' && b <= 'Z' {
									b += 'a' - 'A'
								}
								changeFreqBuf = append(changeFreqBuf, b)
							}
						case fieldPriority:
							priorityBuf = append(priorityBuf, tok...)
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
}
