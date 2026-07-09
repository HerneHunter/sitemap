package sitemap

import "strconv"

// entryBuffers owns the reusable byte buffers for a single sitemap entry's
// fields and centralises the allocate / reset / drain logic that was
// previously duplicated between the standard-library parser (parsesafe.go)
// and the custom-lexer parser (parsefast.go).
type entryBuffers struct {
	flags         parseFlags
	locBuf        []byte
	lastModBuf    []byte
	changeFreqBuf []byte
	priorityBuf   []byte
}

// newEntryBuffers allocates the field buffers, but only for the fields the
// caller actually requested via flags. The loc buffer is always present.
func newEntryBuffers(flags parseFlags) entryBuffers {
	b := entryBuffers{
		flags:  flags,
		locBuf: make([]byte, 0, 2048),
	}
	if flags.lastMod {
		b.lastModBuf = make([]byte, 0, 128)
	}
	if flags.changeFreq {
		b.changeFreqBuf = make([]byte, 0, 32)
	}
	if flags.priority {
		b.priorityBuf = make([]byte, 0, 16)
	}
	return b
}

// reset truncates every allocated buffer back to zero length so it can be
// reused for the next entry without reallocating.
func (b *entryBuffers) reset() {
	b.locBuf = b.locBuf[:0]
	if b.flags.lastMod {
		b.lastModBuf = b.lastModBuf[:0]
	}
	if b.flags.changeFreq {
		b.changeFreqBuf = b.changeFreqBuf[:0]
	}
	if b.flags.priority {
		b.priorityBuf = b.priorityBuf[:0]
	}
}

// dst selects the destination buffer and capture mode for the active field.
// It returns a pointer into the receiver so appended bytes land directly in
// the right buffer without an extra copy.
func (b *entryBuffers) dst(field int) (*[]byte, int) {
	switch field {
	case fieldLoc:
		return &b.locBuf, capUnescape
	case fieldLastMod:
		return &b.lastModBuf, capRaw
	case fieldChangeFreq:
		return &b.changeFreqBuf, capLower
	case fieldPriority:
		return &b.priorityBuf, capRaw
	}
	return nil, capNone
}

// buildResult drains the accumulated buffers into a ParseResult, decoding the
// well-known fields (lastmod, changefreq, priority) the same way both parsers
// did previously. Priority defaults to -1 to match the existing contract.
func (b *entryBuffers) buildResult() ParseResult {
	result := ParseResult{Priority: -1}
	if len(b.locBuf) > 0 {
		result.Loc = string(b.locBuf)
	}
	if b.flags.lastMod && len(b.lastModBuf) > 0 {
		if t, err := ParseTime(string(b.lastModBuf)); err == nil {
			result.LastMod = t
		}
	}
	if b.flags.changeFreq && len(b.changeFreqBuf) > 0 {
		result.ChangeFreq = ChangeFreq(b.changeFreqBuf)
	}
	if b.flags.priority && len(b.priorityBuf) > 0 {
		if p, err := strconv.ParseFloat(string(b.priorityBuf), 64); err == nil {
			result.Priority = p
		}
	}
	return result
}
