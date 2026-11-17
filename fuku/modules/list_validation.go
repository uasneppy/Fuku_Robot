package modules

import "github.com/uasneppy/fuku_robot/fuku/utils/content"

// listKeywordMaxRunes is the longest blacklist word or filter keyword a command or the
// settings menu accepts, in code points.
const listKeywordMaxRunes = content.ListKeywordMaxRunes

// keywordTooBroad reports whether a blacklist word or filter keyword would act on almost
// every message: a blank word, and a word of exactly one code point that is not a symbol of
// Unicode category So. The matchers compare lower-cased literal substrings, so one common
// letter, digit or punctuation mark matches most messages. It is content.KeywordTooBroad,
// which a backup import applies too.
func keywordTooBroad(word string) bool {
	return content.KeywordTooBroad(word)
}

// noteNameInvalid reports whether a note name could never be fetched or would not fit: it contains white
// space, starts with #, or is over listKeywordMaxRunes code points. It is content.NoteNameInvalid, which a
// backup import applies too. The length bound also bounds the typed wait's one text field.
func noteNameInvalid(name string) bool {
	return content.NoteNameInvalid(name)
}
