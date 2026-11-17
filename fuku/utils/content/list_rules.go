package content

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// ListKeywordMaxRunes is the longest blacklist word, filter keyword or note name a command, the settings menu
// or a backup import accepts, in code points.
const ListKeywordMaxRunes = 100

// KeywordTooBroad reports whether a blacklist word or filter keyword would act on almost every message: a
// blank word, and a word of exactly one code point that is not a symbol of Unicode category So. The matchers
// compare lower-cased literal substrings, so one common letter, digit or punctuation mark matches most
// messages, while a single emoji or other symbol stays allowed.
func KeywordTooBroad(word string) bool {
	if strings.TrimSpace(word) == "" {
		return true
	}
	if utf8.RuneCountInString(word) != 1 {
		return false
	}
	r, _ := utf8.DecodeRuneInString(word)
	return !unicode.Is(unicode.So, r)
}

// NoteNameInvalid reports whether a note name could never be fetched or would not fit: it contains white
// space, starts with #, or is over ListKeywordMaxRunes code points. #name and /get take the first
// space-separated token after the # or the command, so a name with a space, or one that starts with #, that
// was stored could never be called back.
func NoteNameInvalid(name string) bool {
	if strings.HasPrefix(name, "#") || utf8.RuneCountInString(name) > ListKeywordMaxRunes {
		return true
	}
	return strings.IndexFunc(name, unicode.IsSpace) >= 0
}
