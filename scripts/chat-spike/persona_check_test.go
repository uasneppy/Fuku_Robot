//go:build testtools

package main

import (
	"os"
	"strings"
	"testing"
)

// TestPersonaCheckFilesAgree keeps the question file and the owner's check
// sheet in step: every question the tool sends is on the sheet, in the same
// order, and there are at least forty of them.
func TestPersonaCheckFilesAgree(t *testing.T) {
	questions, err := readQuestions("persona-check.txt")
	if err != nil {
		t.Fatal(err)
	}
	if len(questions) < 40 {
		t.Fatalf("%d questions, want at least 40", len(questions))
	}
	sheet, err := os.ReadFile("../../.planning/phases/20-conversations/20-PERSONA-CHECK.md")
	if err != nil {
		t.Fatal(err)
	}
	rest := string(sheet)
	for i, q := range questions {
		at := strings.Index(rest, "**"+q+"**")
		if at < 0 {
			t.Fatalf("question %d is not on the sheet in order: %q", i+1, q)
		}
		rest = rest[at:]
	}
}
