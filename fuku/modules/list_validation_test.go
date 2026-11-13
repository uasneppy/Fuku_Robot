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
		{"filter keyword with an inner space", "free followers", false},
	}
	for _, tc := range cases {
		if got := keywordTooBroad(tc.word); got != tc.want {
			t.Errorf("%s: keywordTooBroad(%q) = %v, want %v", tc.name, tc.word, got, tc.want)
		}
	}
}

func TestNoteNameInvalid(t *testing.T) {
	cases := []struct {
		name string
		note string
		want bool
	}{
		{"two words", "two words", true},
		{"tab inside", "a\tb", true},
		{"ideographic space", "a　b", true},
		{"leading hash", "#rules", true},
		{"101 code points", strings.Repeat("x", listKeywordMaxRunes+1), true},
		{"plain", "rules", false},
		{"hyphen", "staff-rules", false},
		{"cyrillic", "правила", false},
		{"inner hash", "a#b", false},
		{"100 code points", strings.Repeat("界", listKeywordMaxRunes), false},
	}
	for _, tc := range cases {
		if got := noteNameInvalid(tc.note); got != tc.want {
			t.Errorf("%s: noteNameInvalid(%q) = %v, want %v", tc.name, tc.note, got, tc.want)
		}
	}
}
