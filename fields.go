package sitemap

// parseFlags indicates which well-known built-in tags to extract.
// Built-ins are handled directly (no closures) to keep Entry on the stack.
type parseFlags struct {
	lastMod    bool
	changeFreq bool
	priority   bool
}

// built-in tags at depth 1
const (
	fieldNone = iota
	fieldLoc
	fieldLastMod
	fieldChangeFreq
	fieldPriority
)

const (
	capNone     = iota // dst is nil: content is scanned and discarded
	capRaw             // copy bytes verbatim (lastmod, priority)
	capLower           // copy bytes, lower-casing ASCII letters (changefreq)
	capUnescape        // copy bytes, expanding XML entities (loc)
)

func matchField(local []byte, flags parseFlags) int {
	switch string(local) {
	case "loc":
		return fieldLoc
	case "lastmod":
		if flags.lastMod {
			return fieldLastMod
		}
	case "changefreq":
		if flags.changeFreq {
			return fieldChangeFreq
		}
	case "priority":
		if flags.priority {
			return fieldPriority
		}
	}
	return fieldNone
}

func selectDst(field int, locBuf, lastModBuf, changeFreqBuf, priorityBuf *[]byte) (*[]byte, int) {
	switch field {
	case fieldLoc:
		return locBuf, capUnescape
	case fieldLastMod:
		return lastModBuf, capRaw
	case fieldChangeFreq:
		return changeFreqBuf, capLower
	case fieldPriority:
		return priorityBuf, capRaw
	}
	return nil, capNone
}
