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

// noteNameInvalid reports whether a note name could never be fetched or would not fit: it contains white
// space, starts with #, or is over listKeywordMaxRunes code points. #name and /get take the first
// space-separated token after the # or the command, so a name with a space, or one that starts with #, that
// was stored could never be called back. The length bound also bounds the typed wait's one text field.
func noteNameInvalid(name string) bool {
	if strings.HasPrefix(name, "#") || utf8.RuneCountInString(name) > listKeywordMaxRunes {
		return true
	}
	return strings.IndexFunc(name, unicode.IsSpace) >= 0
}
