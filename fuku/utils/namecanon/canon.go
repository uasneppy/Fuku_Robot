// Package namecanon is the one definition of a member's canonical names for the
// AI spam-name rule: the text that is sent to the model, the names hash that
// records a baseline, and the verdict key that caches a model answer.
//
// It imports the standard library and golang.org/x/text only, so the bot and
// scripts/claude-spike can share it without pulling in fuku/config or fuku/db,
// whose init functions connect to PostgreSQL before main. It logs nothing and
// holds no state.
package namecanon

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

const (
	// namesTag versions the names hash. Bump it in the same commit as any change
	// to the canonical form (Of), so a baseline recorded under the old form is
	// never compared with a hash of the new one.
	namesTag = "mrn1"
	// verdictTag versions the verdict key. Bump it in the same commit as any
	// change to the canonical form or to claude.NamesInstruction, so a cached
	// verdict of the old instruction is never served.
	verdictTag = "mrv1"

	maxFirstRunes    = 64
	maxLastRunes     = 64
	maxUsernameRunes = 32

	// hashBytes is how much of a SHA-256 is kept: 16 bytes, 32 hex characters.
	hashBytes = 16

	deletedAccount = "Deleted Account"
	brailleBlank   = 0x2800
)

// Canon is a member's three names in canonical form and the hash of them.
type Canon struct {
	// First and Last keep their case: it is the text that is sent to the model.
	First string
	Last  string
	// Username has no leading "@" and is lower-cased.
	Username string
	// Hash is the names hash, 32 lower-case hex characters.
	Hash string
}

// Of returns the canonical form of a member's names and whether there is
// anything to judge. All three fields empty after canonicalisation, or a lone
// "Deleted Account" first name with no last name and no username, is not
// judgeable and returns the zero Canon.
func Of(first, last, username string) (Canon, bool) {
	c := Canon{
		First:    canonField(first, maxFirstRunes),
		Last:     canonField(last, maxLastRunes),
		Username: strings.ToLower(canonUsername(username)),
	}
	if c.First == "" && c.Last == "" && c.Username == "" {
		return Canon{}, false
	}
	if c.Last == "" && c.Username == "" && strings.EqualFold(c.First, deletedAccount) {
		return Canon{}, false
	}
	c.Hash = namesHash(c)
	return c, true
}

// VerdictKey is the key of a cached verdict: the first 16 bytes of the SHA-256
// of the verdict tag, a NUL, the model, a NUL and the names hash, in 32 hex
// characters. The model is part of it, so a changed ANTHROPIC_MODEL never
// serves the old model's answer.
func VerdictKey(model string, c Canon) string {
	return digest(verdictTag + "\x00" + model + "\x00" + c.Hash)
}

// namesHash is the first 16 bytes of the SHA-256 of the names tag, a NUL and
// the three fields, each as its byte length in decimal, a colon and the field,
// so no field slides into its neighbour. First and last name are lower-cased
// here only: the text that is sent keeps its case.
func namesHash(c Canon) string {
	var b strings.Builder
	b.WriteString(namesTag)
	b.WriteByte(0)
	for _, field := range []string{strings.ToLower(c.First), strings.ToLower(c.Last), c.Username} {
		b.WriteString(strconv.Itoa(len(field)))
		b.WriteByte(':')
		b.WriteString(field)
	}
	return digest(b.String())
}

func digest(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:hashBytes])
}

// canonUsername strips one leading "@" (after canonicalisation, so an invisible
// rune in front of it does not hide it) and cuts the rest to the username limit.
func canonUsername(s string) string {
	s = canonField(s, 0)
	s = strings.TrimPrefix(s, "@")
	return cutRunes(strings.TrimSpace(s), maxUsernameRunes)
}

// canonField drops what cannot be seen, applies NFKC, collapses whitespace,
// trims, then cuts to limit runes (no cut when limit is 0).
func canonField(s string, limit int) string {
	s = dropInvisible(s)
	s = norm.NFKC.String(s)
	s = dropInvisible(s)
	s = strings.Join(strings.Fields(s), " ")
	if limit > 0 {
		s = strings.TrimSpace(cutRunes(s, limit))
	}
	return s
}

// dropInvisible removes every control (Cc), format (Cf) and default-ignorable
// rune and the Braille blank, none of which a person can see. Whitespace is
// kept, so the control characters that separate words (tab, newline) still
// collapse to one space. Marks are kept: they carry the vowels of many scripts.
func dropInvisible(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case unicode.IsSpace(r):
			return r
		case unicode.Is(unicode.Cc, r), unicode.Is(unicode.Cf, r),
			unicode.Is(unicode.Other_Default_Ignorable_Code_Point, r), r == brailleBlank:
			return -1
		}
		return r
	}, s)
}

// cutRunes returns the first limit runes of s, never cutting inside a character.
func cutRunes(s string, limit int) string {
	if utf8.RuneCountInString(s) <= limit {
		return s
	}
	n := 0
	for i := range s {
		if n == limit {
			return s[:i]
		}
		n++
	}
	return s
}
