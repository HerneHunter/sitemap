package sitemap

import (
	"bytes"
	"io"
)

var (
	piEndSeq      = []byte("?>")
	commentEndSeq = []byte("-->")
	cdataEndSeq   = []byte("]]>")
	cdataLitSeq   = []byte("CDATA[")
)

const xmlLexerBufSize = 32 * 1024

// xmlLexer is a small hand-rolled, allocation-free (in the steady state)
// forward-only XML tokenizer tailored to sitemap documents. It reads from
// the underlying io.Reader in fixed-size chunks and never buffers the whole
// document in memory. It intentionally supports only the subset of XML that
// can appear in a sitemap: elements (with attributes, ignored), text,
// CDATA sections, comments, processing instructions and a permissive
// DOCTYPE skip.
type xmlLexer struct {
	r   io.Reader
	buf []byte
	pos int
	end int
	eof bool

	// truncated is set when the stream ends in the middle of a tag/entity/
	// comment/CDATA/PI construct, as opposed to a clean end-of-document.
	// Only meaningful to inspect immediately after nextEvent returns ok=false.
	truncated bool

	// nameBuf holds the most recently read element name (raw, possibly
	// namespace-prefixed) and is reused across calls.
	nameBuf []byte
}

func newXMLLexer(r io.Reader) *xmlLexer {
	return &xmlLexer{
		r:       r,
		buf:     make([]byte, xmlLexerBufSize),
		nameBuf: make([]byte, 0, 32),
	}
}

// Must only be called when pos >= end.
func (l *xmlLexer) fill() bool {
	if l.eof {
		return false
	}
	for {
		n, err := l.r.Read(l.buf)
		if n > 0 {
			l.pos, l.end = 0, n
			if err != nil {
				l.eof = true
			}
			return true
		}
		if err != nil {
			l.eof = true
			return false
		}
		// n == 0, err == nil: per io.Reader's contract this should be rare;
		// retry rather than treating it as EOF.
	}
}

func (l *xmlLexer) peek() (byte, bool) {
	if l.pos >= l.end {
		if !l.fill() {
			return 0, false
		}
	}
	return l.buf[l.pos], true
}

func (l *xmlLexer) skipUntil(delim byte) bool {
	for {
		if l.pos >= l.end {
			if !l.fill() {
				return false
			}
		}
		idx := bytes.IndexByte(l.buf[l.pos:l.end], delim)
		if idx >= 0 {
			l.pos += idx
			return true
		}
		l.pos = l.end
	}
}

func (l *xmlLexer) appendUntil(dst *[]byte, delim byte) bool {
	for {
		if l.pos >= l.end {
			if !l.fill() {
				return false
			}
		}
		idx := bytes.IndexByte(l.buf[l.pos:l.end], delim)
		if idx >= 0 {
			*dst = append(*dst, l.buf[l.pos:l.pos+idx]...)
			l.pos += idx
			return true
		}
		*dst = append(*dst, l.buf[l.pos:l.end]...)
		l.pos = l.end
	}
}

// Used only for <changefreq>. Values are short enough that a byte-at-a-time loop is fine.
func (l *xmlLexer) appendUntilLower(dst *[]byte) bool {
	for {
		b, ok := l.peek()
		if !ok {
			return false
		}
		if b == '<' {
			return true
		}
		l.pos++
		if b >= 'A' && b <= 'Z' {
			b += 'a' - 'A'
		}
		*dst = append(*dst, b)
	}
}

// Hot path for <loc>. The common case (no entities) is handled via a single bulk IndexAny scan.
func (l *xmlLexer) appendUntilUnescape(dst *[]byte) bool {
	for {
		if l.pos >= l.end {
			if !l.fill() {
				return false
			}
		}
		chunk := l.buf[l.pos:l.end]
		idx := bytes.IndexAny(chunk, "&<")
		if idx < 0 {
			*dst = append(*dst, chunk...)
			l.pos = l.end
			continue
		}
		if idx > 0 {
			*dst = append(*dst, chunk[:idx]...)
			l.pos += idx
		}
		if l.buf[l.pos] == '<' {
			return true
		}
		l.pos++ // consume '&'
		l.decodeEntity(dst)
	}
}

// Byte-at-a-time matchers for PIs, comments, and CDATA terminators.
// These are rare in sitemaps, so the simple loop is fast enough.
func (l *xmlLexer) skipUntilSeq(seq []byte) bool {
	matched := 0
	for {
		b, ok := l.peek()
		if !ok {
			return false
		}
		l.pos++
		switch b {
		case seq[matched]:
			matched++
			if matched == len(seq) {
				return true
			}
		case seq[0]:
			matched = 1
		default:
			matched = 0
		}
	}
}

func (l *xmlLexer) appendUntilSeq(dst *[]byte, seq []byte) bool {
	matched := 0
	for {
		b, ok := l.peek()
		if !ok {
			return false
		}
		l.pos++
		*dst = append(*dst, b)
		switch b {
		case seq[matched]:
			matched++
			if matched == len(seq) {
				*dst = (*dst)[:len(*dst)-len(seq)]
				return true
			}
		case seq[0]:
			matched = 1
		default:
			matched = 0
		}
	}
}

func (l *xmlLexer) appendUntilSeqLower(dst *[]byte, seq []byte) bool {
	matched := 0
	for {
		b, ok := l.peek()
		if !ok {
			return false
		}
		l.pos++
		if b >= 'A' && b <= 'Z' {
			b += 'a' - 'A'
		}
		*dst = append(*dst, b)
		switch b {
		case seq[matched]:
			matched++
			if matched == len(seq) {
				*dst = (*dst)[:len(*dst)-len(seq)]
				return true
			}
		case seq[0]:
			matched = 1
		default:
			matched = 0
		}
	}
}

func (l *xmlLexer) skipN(n int) bool {
	for n > 0 {
		if l.pos >= l.end {
			if !l.fill() {
				return false
			}
		}
		avail := min(l.end-l.pos, n)
		l.pos += avail
		n -= avail
	}
	return true
}

// Tracks '[' ']' nesting so internal subsets don't terminate early on '>'.
func (l *xmlLexer) skipDoctype() bool {
	depth := 0
	for {
		b, ok := l.peek()
		if !ok {
			return false
		}
		l.pos++
		switch b {
		case '[':
			depth++
		case ']':
			if depth > 0 {
				depth--
			}
		case '>':
			if depth == 0 {
				return true
			}
		}
	}
}

func isNameEnd(b byte) bool {
	switch b {
	case ' ', '\t', '\r', '\n', '>', '/':
		return true
	}
	return false
}

func (l *xmlLexer) readName(dst *[]byte) bool {
	for {
		if l.pos >= l.end {
			if !l.fill() {
				return false
			}
		}
		start := l.pos
		for l.pos < l.end && !isNameEnd(l.buf[l.pos]) {
			l.pos++
		}
		*dst = append(*dst, l.buf[start:l.pos]...)
		if l.pos < l.end {
			return true
		}
	}
}

// Skips attributes up to '>'. Returns true if the tag was self-closing.
func (l *xmlLexer) skipAttrs() (selfClose, ok bool) {
	for {
		b, pOk := l.peek()
		if !pOk {
			return false, false
		}
		switch b {
		case '"':
			l.pos++
			if !l.skipUntil('"') {
				return false, false
			}
			l.pos++
		case '\'':
			l.pos++
			if !l.skipUntil('\'') {
				return false, false
			}
			l.pos++
		case '>':
			l.pos++
			return false, true
		case '/':
			l.pos++
			nb, nOk := l.peek()
			if nOk && nb == '>' {
				l.pos++
				return true, true
			}
		default:
			l.pos++
		}
	}
}

// localName strips a namespace prefix (e.g. "sm:loc" -> "loc"), if any.
func localName(name []byte) []byte {
	if _, after, ok := bytes.Cut(name, []byte{':'}); ok {
		return after
	}
	return name
}


func (l *xmlLexer) readTextRun(dst *[]byte, mode int) bool {
	if dst == nil {
		return l.skipUntil('<')
	}
	switch mode {
	case capUnescape:
		return l.appendUntilUnescape(dst)
	case capLower:
		return l.appendUntilLower(dst)
	default:
		return l.appendUntil(dst, '<')
	}
}

func (l *xmlLexer) readCDATA(dst *[]byte, mode int) bool {
	if dst == nil {
		return l.skipUntilSeq(cdataEndSeq)
	}
	if mode == capLower {
		return l.appendUntilSeqLower(dst, cdataEndSeq)
	}
	// CDATA content is always literal: never entity-expanded, even when
	// mode == capUnescape.
	return l.appendUntilSeq(dst, cdataEndSeq)
}

// ok is false at EOF. l.truncated distinguishes a clean EOF from mid-construct breakage.
func (l *xmlLexer) nextEvent(dst *[]byte, mode int) (isEnd, selfClose, ok bool) {
	for {
		if !l.readTextRun(dst, mode) {
			return false, false, false
		}
		l.pos++ // consume '<'

		b, pOk := l.peek()
		if !pOk {
			l.truncated = true
			return false, false, false
		}
		switch {
		case b == '/':
			l.pos++
			if !l.skipUntil('>') {
				l.truncated = true
				return false, false, false
			}
			l.pos++
			return true, false, true
		case b == '?':
			l.pos++
			if !l.skipUntilSeq(piEndSeq) {
				l.truncated = true
				return false, false, false
			}
			continue
		case b == '!':
			l.pos++
			b2, pOk2 := l.peek()
			if !pOk2 {
				l.truncated = true
				return false, false, false
			}
			switch b2 {
			case '-':
				l.pos++
				if pb, ok3 := l.peek(); ok3 && pb == '-' {
					l.pos++
				}
				if !l.skipUntilSeq(commentEndSeq) {
					l.truncated = true
					return false, false, false
				}
			case '[':
				l.pos++
				if !l.skipN(len(cdataLitSeq)) {
					l.truncated = true
					return false, false, false
				}
				if !l.readCDATA(dst, mode) {
					l.truncated = true
					return false, false, false
				}
			default:
				if !l.skipDoctype() {
					l.truncated = true
					return false, false, false
				}
			}
			continue
		default:
			l.nameBuf = l.nameBuf[:0]
			if !l.readName(&l.nameBuf) {
				l.truncated = true
				return false, false, false
			}
			sc, aOk := l.skipAttrs()
			if !aOk {
				l.truncated = true
				return false, false, false
			}
			return false, sc, true
		}
	}
}
