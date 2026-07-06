package sitemap

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
)

type lexerTestCase struct {
	ID    int      `json:"id"`
	Group string   `json:"group"`
	Desc  string   `json:"desc"`
	
	Method   string   `json:"method"`
	Chunks   []string `json:"chunks"`
	EOFErr   bool     `json:"eof_err"`
	
	ArgString string `json:"arg_string"`
	ArgInt    int    `json:"arg_int"`
	
	ExpectedOK    bool   `json:"expected_ok"`
	ExpectedDst   string `json:"expected_dst"`
	ExpectedRest  string `json:"expected_rest"`
	ExpectedEnd   bool   `json:"expected_end"`
	ExpectedSC    bool   `json:"expected_sc"`
	ExpectedTrunc bool   `json:"expected_trunc"`
	ExpectedName  string `json:"expected_name"`
}

// expandMacro replaces "[32KB_A]" with 32768 'A's.
func expandMacro(s string) string {
	if strings.Contains(s, "[32KB_A]") {
		return strings.ReplaceAll(s, "[32KB_A]", strings.Repeat("A", 32768))
	}
	return s
}

type mockReader struct {
	chunks []string
	pos    int
	eofErr bool
}

func (m *mockReader) Read(p []byte) (n int, err error) {
	if m.pos >= len(m.chunks) {
		return 0, io.EOF
	}

	chunk := m.chunks[m.pos]
	chunk = expandMacro(chunk)

	copied := copy(p, chunk)
	if copied < len(chunk) {
		m.chunks[m.pos] = chunk[copied:]
		return copied, nil
	}

	m.pos++
	if m.pos == len(m.chunks) && m.eofErr {
		return copied, io.EOF
	}
	return copied, nil
}

// readRest drains the remaining lexer buffer and underlying reader.
func (m *mockReader) readRest(l *xmlLexer) string {
	var rest []byte
	if l.pos < l.end {
		rest = append(rest, l.buf[l.pos:l.end]...)
	}
	for m.pos < len(m.chunks) {
		rest = append(rest, expandMacro(m.chunks[m.pos])...)
		m.pos++
	}
	return string(rest)
}

func TestLexer(t *testing.T) {
	data, err := os.ReadFile("testdata/lexer_tests.json")
	if err != nil {
		t.Fatalf("Failed to read test data: %v", err)
	}

	var testCases []lexerTestCase
	if err := json.Unmarshal(data, &testCases); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	for _, tc := range testCases {
		groupName := strings.ReplaceAll(tc.Group, " ", "")
		groupName = strings.ReplaceAll(groupName, "/", "_")
		testName := fmt.Sprintf("%s/Case_%d", groupName, tc.ID)

		t.Run(testName, func(t *testing.T) {
			mr := &mockReader{
				chunks: append([]string(nil), tc.Chunks...), // shallow copy
				eofErr: tc.EOFErr,
			}
			lexer := newXMLLexer(mr)
			
			var dst []byte
			var ok bool
			
			switch tc.Method {
			case "peek":
				b, peekOk := lexer.peek()
				ok = peekOk
				if ok {
					dst = append(dst, b)
				}
			case "readAll":
				// Used to test chunk boundaries
				for {
					b, peekOk := lexer.peek()
					if !peekOk {
						break
					}
					dst = append(dst, b)
					lexer.pos++
				}
				ok = true
			case "skipUntil":
				ok = lexer.skipUntil(tc.ArgString[0])
			case "appendUntil":
				ok = lexer.appendUntil(&dst, tc.ArgString[0])
			case "appendUntilLower":
				ok = lexer.appendUntilLower(&dst)
			case "appendUntilUnescape":
				ok = lexer.appendUntilUnescape(&dst)
			case "skipUntilSeq":
				ok = lexer.skipUntilSeq([]byte(tc.ArgString))
			case "appendUntilSeq":
				ok = lexer.appendUntilSeq(&dst, []byte(tc.ArgString))
			case "appendUntilSeqLower":
				ok = lexer.appendUntilSeqLower(&dst, []byte(tc.ArgString))
			case "skipN":
				ok = lexer.skipN(tc.ArgInt)
			case "skipDoctype":
				ok = lexer.skipDoctype()
			case "skipAttrs":
				sc, scOk := lexer.skipAttrs()
				ok = scOk
				if sc != tc.ExpectedSC {
					t.Errorf("skipAttrs SC mismatch: expected %v, got %v", tc.ExpectedSC, sc)
				}
			case "localName":
				dst = localName([]byte(tc.ArgString))
				ok = true // not actually reading
			case "nextEvent":
				isEnd, sc, nextOk := lexer.nextEvent(&dst, 0)
				ok = nextOk
				if isEnd != tc.ExpectedEnd {
					t.Errorf("isEnd mismatch: expected %v, got %v", tc.ExpectedEnd, isEnd)
				}
				if sc != tc.ExpectedSC {
					t.Errorf("selfClose mismatch: expected %v, got %v", tc.ExpectedSC, sc)
				}
				if lexer.truncated != tc.ExpectedTrunc {
					t.Errorf("truncated mismatch: expected %v, got %v", tc.ExpectedTrunc, lexer.truncated)
				}
				// End tags don't populate nameBuf.
				if tc.ExpectedName != "" && !isEnd {
					if string(lexer.nameBuf) != tc.ExpectedName {
						t.Errorf("nameBuf mismatch: expected %q, got %q", tc.ExpectedName, string(lexer.nameBuf))
					}
				}
			default:
				t.Fatalf("Unknown method %q", tc.Method)
			}
			
			if ok != tc.ExpectedOK {
				t.Errorf("ok mismatch: expected %v, got %v", tc.ExpectedOK, ok)
			}
			
			if tc.ExpectedDst != "" && string(dst) != tc.ExpectedDst {
				t.Errorf("dst mismatch: expected %q, got %q", tc.ExpectedDst, string(dst))
			}
			
			if tc.Method != "localName" {
				actualRest := mr.readRest(lexer)
				expectedRest := expandMacro(tc.ExpectedRest)
				if tc.ExpectedRest != "" && actualRest != expectedRest {
					t.Errorf("rest mismatch:\nExpected: %q\nGot:      %q", expectedRest, actualRest)
				}
				if tc.ExpectedRest == "" && actualRest != "" {
					t.Errorf("rest mismatch: expected empty string, got %q", actualRest)
				}
			}
		})
	}
}
