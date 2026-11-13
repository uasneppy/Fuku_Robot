//go:build testtools

package modules

import (
	"strings"
	"testing"
)

func TestKeywordTooBroad(t *testing.T) {
	cases := []struct {
		name string
		word string
		want bool
	}{
		{"empty", "", true},
		{"blank", "  ", true},
		{"lower letter", "a", true},
		{"upper letter", "A", true},
		{"digit", "7", true},
		{"punctuation", ".", true},
		{"cyrillic letter", "ж", true},
		{"sharp s", "ß", true},
		{"currency sign is not So", "€", true},
		{"combining mark", "́", true},
		{"emoji is So", "🙂", false},
		{"copyright sign is So", "©", false},
		{"two letters", "ab", false},
		{"two punctuation marks", "**", false},
		{"regex-like", ".*", false},
		{"longest word", strings.Repeat("x", listKeywordMaxRunes), false},
	}
	for _, tc := range cases {
		if got := keywordTooBroad(tc.word); got != tc.want {
			t.Errorf("%s: keywordTooBroad(%q) = %v, want %v", tc.name, tc.word, got, tc.want)
		}
	}
}
