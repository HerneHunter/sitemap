package sitemap

import (
	"strconv"
	"unicode/utf8"
)

// Malformed or unrecognized entities are emitted verbatim to stay lenient like most XML consumers.
func (l *xmlLexer) decodeEntity(dst *[]byte) {
	var raw [16]byte
	n := 0
	for {
		b, ok := l.peek()
		if !ok {
			*dst = append(*dst, '&')
			*dst = append(*dst, raw[:n]...)
			return
		}
		if b == ';' {
			l.pos++
			break
		}
		if b == '<' || n >= len(raw) {
			*dst = append(*dst, '&')
			*dst = append(*dst, raw[:n]...)
			return
		}
		raw[n] = b
		n++
		l.pos++
	}
	ent := raw[:n]
	if n > 1 && ent[0] == '#' {
		var code int64
		var err error
		if n > 2 && (ent[1] == 'x' || ent[1] == 'X') {
			code, err = strconv.ParseInt(string(ent[2:]), 16, 32)
		} else {
			code, err = strconv.ParseInt(string(ent[1:]), 10, 32)
		}
		if err == nil && code >= 0 && code <= utf8.MaxRune {
			*dst = utf8.AppendRune(*dst, rune(code))
		} else {
			*dst = append(*dst, '&')
			*dst = append(*dst, ent...)
			*dst = append(*dst, ';')
		}
		return
	}
	switch string(ent) {
	case "amp":
		*dst = append(*dst, '&')
	case "lt":
		*dst = append(*dst, '<')
	case "gt":
		*dst = append(*dst, '>')
	case "quot":
		*dst = append(*dst, '"')
	case "apos":
		*dst = append(*dst, '\'')
	default:
		*dst = append(*dst, '&')
		*dst = append(*dst, ent...)
		*dst = append(*dst, ';')
	}
}
