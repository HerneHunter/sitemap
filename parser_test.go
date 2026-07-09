package sitemap

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

type parserTestCase struct {
	ID     int    `json:"id"`
	Group  string `json:"group"`
	Method string `json:"method"`
	Desc   string `json:"desc"`

	Input     string   `json:"input"`
	SelfClose bool     `json:"self_close"`
	Flags     []string `json:"flags"`

	ExpectedOK   bool   `json:"expected_ok"`
	ExpectedLoc  string `json:"expected_loc"`
	ExpectedMod  string `json:"expected_mod"`
	ExpectedFreq string `json:"expected_freq"`
	ExpectedPri  string `json:"expected_pri"`

	ExpectedKindIndex bool          `json:"expected_kind_index"`
	ExpectedResults   []expectedRes `json:"expected_results"`

	CancelAt    *int `json:"cancel_at"`
	YieldStopAt *int `json:"yield_stop_at"`

	InputFile    string `json:"input_file"`
	ExpectedFile string `json:"expected_file"`
}

type expectedRes struct {
	Loc        string   `json:"Loc"`
	LastMod    string   `json:"LastMod"`
	ChangeFreq string   `json:"ChangeFreq"`
	Priority   *float64 `json:"Priority"`
	Err        string   `json:"Err"` // matched as a substring
}

func parseFlagsFromList(flags []string) parseFlags {
	var pf parseFlags
	for _, f := range flags {
		switch f {
		case "lastMod":
			pf.lastMod = true
		case "changeFreq":
			pf.changeFreq = true
		case "priority":
			pf.priority = true
		}
	}
	return pf
}

// checkEntryBuffers compares the accumulated entry field buffers against the
// expected strings, reporting any mismatch with the field name.
func checkEntryBuffers(t *testing.T, b *entryBuffers, wantLoc, wantMod, wantFreq, wantPri string) {
	t.Helper()
	if got, want := string(b.locBuf), wantLoc; got != want {
		t.Errorf("loc = %q, want %q", got, want)
	}
	if got, want := string(b.lastModBuf), wantMod; got != want {
		t.Errorf("lastMod = %q, want %q", got, want)
	}
	if got, want := string(b.changeFreqBuf), wantFreq; got != want {
		t.Errorf("changeFreq = %q, want %q", got, want)
	}
	if got, want := string(b.priorityBuf), wantPri; got != want {
		t.Errorf("priority = %q, want %q", got, want)
	}
}

// checkResult compares a single ParseResult against an expectedRes, covering
// Loc, LastMod, ChangeFreq, Priority and the optional error substring.
func checkResult(t *testing.T, i int, res ParseResult, exp expectedRes) {
	t.Helper()
	if res.Loc != exp.Loc {
		t.Errorf("Result %d: Loc = %q, want %q", i, res.Loc, exp.Loc)
	}
	if exp.LastMod != "" {
		expTime, err := time.Parse(time.RFC3339, exp.LastMod)
		if err != nil {
			t.Fatalf("Result %d: Invalid LastMod format in expected JSON %q: %v", i, exp.LastMod, err)
		}
		if !res.LastMod.Equal(expTime) {
			t.Errorf("Result %d: LastMod = %v, want %v", i, res.LastMod, expTime)
		}
	} else {
		if !res.LastMod.IsZero() {
			t.Errorf("Result %d: expected zero LastMod, got %v", i, res.LastMod)
		}
	}

	if string(res.ChangeFreq) != exp.ChangeFreq {
		t.Errorf("Result %d: ChangeFreq = %q, want %q", i, res.ChangeFreq, exp.ChangeFreq)
	}

	if exp.Priority != nil {
		if res.Priority != *exp.Priority {
			t.Errorf("Result %d: Priority = %v, want %v", i, res.Priority, *exp.Priority)
		}
	} else {
		if res.Priority != -1 && res.Err == nil {
			t.Errorf("Result %d: Priority = %v, want -1", i, res.Priority)
		}
	}

	if exp.Err != "" {
		if res.Err == nil {
			t.Errorf("Result %d: expected error containing %q, got nil", i, exp.Err)
		} else if !strings.Contains(res.Err.Error(), exp.Err) {
			t.Errorf("Result %d: expected error containing %q, got %q", i, exp.Err, res.Err.Error())
		}
	} else {
		if res.Err != nil {
			t.Errorf("Result %d: expected no error, got %v", i, res.Err)
		}
	}
}

func TestParser(t *testing.T) {
	data, err := os.ReadFile("testdata/parser_tests.json")
	if err != nil {
		t.Fatalf("Failed to read test data: %v", err)
	}

	var testCases []parserTestCase
	if err := json.Unmarshal(data, &testCases); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	for _, tc := range testCases {
		testName := fmt.Sprintf("%s/Case_%d", tc.Group, tc.ID)
		t.Run(testName, func(t *testing.T) {
			flags := parseFlagsFromList(tc.Flags)

			switch tc.Method {
			case "processEntry":
				lx := newXMLLexer(strings.NewReader(tc.Input))
				b := newEntryBuffers(flags)
				ok := processEntry(lx, &b, tc.SelfClose)

				if ok != tc.ExpectedOK {
					t.Errorf("ok = %v, want %v", ok, tc.ExpectedOK)
				}
				checkEntryBuffers(t, &b, tc.ExpectedLoc, tc.ExpectedMod, tc.ExpectedFreq, tc.ExpectedPri)
			case "parse", "parse_file":
				runTest := func(t *testing.T, useCustomLexer bool) {
					var reader io.Reader
					if tc.Method == "parse_file" {
						f, err := os.Open(tc.InputFile)
						if err != nil {
							t.Fatalf("Failed to open input file: %v", err)
						}
						defer f.Close()
						reader = f
					} else {
						reader = strings.NewReader(tc.Input)
					}

					ctx, cancel := context.WithCancel(context.Background())
					defer cancel()

					if tc.CancelAt != nil && *tc.CancelAt == 0 {
						cancel()
					}

					var kindOut bool
					var indexResults []ParseResult

					yield := func(res ParseResult) bool {
						indexResults = append(indexResults, res)
						if tc.CancelAt != nil && len(indexResults) == *tc.CancelAt {
							cancel()
						}
						if tc.YieldStopAt != nil && len(indexResults) == *tc.YieldStopAt {
							return false
						}
						return true
					}

					parse(ctx, reader, flags, useCustomLexer, &kindOut, yield)

					if kindOut != tc.ExpectedKindIndex {
						t.Errorf("kindOut = %v, want %v", kindOut, tc.ExpectedKindIndex)
					}

					var finalResults []ParseResult
					if kindOut && tc.Method == "parse_file" {
						// Recursively parse child sitemaps.
						for _, res := range indexResults {
							if res.Err != nil {
								finalResults = append(finalResults, res)
								continue
							}
							sf, err := os.Open(res.Loc)
							if err != nil {
								finalResults = append(finalResults, ParseResult{Err: err, Priority: -1})
								continue
							}
							var childKind bool
							parse(ctx, sf, flags, useCustomLexer, &childKind, func(cres ParseResult) bool {
								finalResults = append(finalResults, cres)
								return true
							})
							sf.Close()
						}
					} else {
						finalResults = indexResults
					}

					var expected []expectedRes
					if tc.Method == "parse_file" {
						expData, err := os.ReadFile(tc.ExpectedFile)
						if err != nil {
							t.Fatalf("Failed to read expected file: %v", err)
						}
						if err := json.Unmarshal(expData, &expected); err != nil {
							t.Fatalf("Failed to parse expected JSON: %v", err)
						}

						if tc.ExpectedKindIndex {
							var expanded []expectedRes
							for _, exp := range expected {
								if exp.Err == "" && strings.HasSuffix(exp.Loc, ".xml") {
									childPath := strings.TrimSuffix(exp.Loc, ".xml") + ".json"
									childData, err := os.ReadFile(childPath)
									if err == nil {
										var childExp []expectedRes
										if err := json.Unmarshal(childData, &childExp); err != nil {
											t.Fatalf("Failed to parse expected JSON for child %s: %v", childPath, err)
										}
										expanded = append(expanded, childExp...)
										continue
									}
								}
								expanded = append(expanded, exp)
							}
							expected = expanded
						}
					} else {
						expected = tc.ExpectedResults
					}

					if len(finalResults) != len(expected) {
						t.Fatalf("got %d results, want %d", len(finalResults), len(expected))
					}

					for i, res := range finalResults {
						checkResult(t, i, res, expected[i])
					}
				}

				t.Run("StdParser", func(t *testing.T) { runTest(t, false) })
				t.Run("CustomLexer", func(t *testing.T) { runTest(t, true) })
			}
		})
	}
}
