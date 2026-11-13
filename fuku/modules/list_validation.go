package modules

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// listKeywordMaxRunes is the longest blacklist word or filter keyword a command or the
// settings menu accepts, in code points.
const listKeywordMaxRunes = 100

// keywordTooBroad reports whether a blacklist word or filter keyword would act on almost
// every message: a blank word, and a word of exactly one code point that is not a symbol of
// Unicode category So. The matchers compare lower-cased literal substrings, so one common
// letter, digit or punctuation mark matches most messages, while a single emoji or other
// symbol stays allowed.
func keywordTooBroad(word string) bool {
	if strings.TrimSpace(word) == "" {
		return true
	}
	if utf8.RuneCountInString(word) != 1 {
		return false
	}
	r, _ := utf8.DecodeRuneInString(word)
	return !unicode.Is(unicode.So, r)
}
