package sitemap

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
)

type entityTestCase struct {
	ID       int    `json:"id"`
	Group    string `json:"group"`
	Input    string `json:"input"`
	Expected string `json:"expected"`
}

func TestDecodeEntity(t *testing.T) {
	data, err := os.ReadFile("testdata/entities.json")
	if err != nil {
		t.Fatalf("Failed to read test data: %v", err)
	}

	var testCases []entityTestCase
	if err := json.Unmarshal(data, &testCases); err != nil {
		t.Fatalf("Failed to parse JSON test data: %v", err)
	}

	for _, tc := range testCases {
		groupName := strings.ReplaceAll(tc.Group, " ", "")
		testName := fmt.Sprintf("%s/Case_%d", groupName, tc.ID)

		t.Run(testName, func(t *testing.T) {
			lexer := newXMLLexer(strings.NewReader(tc.Input))
			var result []byte

			lexer.appendUntilUnescape(&result)

			if !bytes.Equal(result, []byte(tc.Expected)) {
				t.Errorf("\nInput:    %q\nExpected: %q\nGot:      %q", tc.Input, tc.Expected, string(result))
			}
		})
	}
}
