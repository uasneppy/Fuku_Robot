//go:build testtools

package namecanon

import (
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"
)

var hex32 = regexp.MustCompile(`^[0-9a-f]{32}$`)

// canonOf requires a judgeable result.
func canonOf(t *testing.T, first, last, username string) Canon {
	t.Helper()
	c, ok := Of(first, last, username)
	if !ok {
		t.Fatalf("Of(%q, %q, %q) is not judgeable", first, last, username)
	}
	return c
}

func TestCanonForm(t *testing.T) {
	t.Run("every format and control rune is dropped", func(t *testing.T) {
		dropped := map[string]rune{
			"arabic letter mark":      0x061C,
			"left-to-right mark":      0x200E,
			"right-to-left mark":      0x200F,
			"left-to-right embedding": 0x202A,
			"right-to-left embedding": 0x202B,
			"pop directional":         0x202C,
			"left-to-right override":  0x202D,
			"right-to-left override":  0x202E,
			"left-to-right isolate":   0x2066,
			"right-to-left isolate":   0x2067,
			"first strong isolate":    0x2068,
			"pop directional isolate": 0x2069,
			"zero-width space":        0x200B,
			"zero-width non-joiner":   0x200C,
			"zero-width joiner":       0x200D,
			"byte order mark":         0xFEFF,
			"soft hyphen":             0x00AD,
			"word joiner":             0x2060,
			"NUL":                     0x0000,
			"bell":                    0x0007,
			"delete":                  0x007F,
			"C1 control":              0x009B,
		}
		for name, r := range dropped {
			in := "A" + string(r) + "b"
			c := canonOf(t, in, in, in)
			if c.First != "Ab" || c.Last != "Ab" || c.Username != "ab" {
				t.Errorf("%s (U+%04X): got %q %q %q, want Ab Ab ab", name, r, c.First, c.Last, c.Username)
			}
		}
	})

	t.Run("default-ignorable runes and the braille blank are dropped", func(t *testing.T) {
		for name, r := range map[string]rune{
			"hangul choseong filler":  0x115F,
			"hangul jungseong filler": 0x1160,
			"hangul filler":           0x3164,
			"halfwidth hangul filler": 0xFFA0,
			"combining grapheme join": 0x034F,
			"braille blank":           0x2800,
		} {
			in := "A" + string(r) + "b"
			if c := canonOf(t, in, "", ""); c.First != "Ab" {
				t.Errorf("%s (U+%04X): got %q, want Ab", name, r, c.First)
			}
		}
	})

	t.Run("NFKC folds look-alikes and composes", func(t *testing.T) {
		cases := map[string]string{
			"Ａｎｎａ":                                 "Anna",
			"\U0001D400\U0001D427\U0001D427\U0001D41A":                 "Anna",
			"ﬁsh":                                                 "fish",
			"é":                                                  "é",
			"① free":                                              "1 free",
			"नमस्ते": "नमस्ते",
		}
		for in, want := range cases {
			if got := canonOf(t, in, "", "").First; got != want {
				t.Errorf("First(%q) = %q, want %q", in, got, want)
			}
		}
	})

	t.Run("whitespace runs collapse to one space and the result is trimmed", func(t *testing.T) {
		in := " \t a \n　  b  c \r\n"
		if got := canonOf(t, in, "", "").First; got != "a b c" {
			t.Errorf("First = %q, want %q", got, "a b c")
		}
	})

	t.Run("a username loses one leading at sign and is lower-cased", func(t *testing.T) {
		cases := map[string]string{
			"@Anna_K":       "anna_k",
			"Anna_K":        "anna_k",
			"@@x":           "@x",
			"  @X":          "x",
			"​@Anna":   "anna",
			"＠Anna":    "anna",
			"@ SPACED  OUT": "spaced out",
		}
		for in, want := range cases {
			if got := canonOf(t, "x", "", in).Username; got != want {
				t.Errorf("Username(%q) = %q, want %q", in, got, want)
			}
		}
	})

	t.Run("first and last name keep their case", func(t *testing.T) {
		c := canonOf(t, "ANNA", "KoWaLsKi", "")
		if c.First != "ANNA" || c.Last != "KoWaLsKi" {
			t.Errorf("got %q %q", c.First, c.Last)
		}
	})
}

func TestCanonBounds(t *testing.T) {
	t.Run("first and last are cut at 64 runes and the username at 32", func(t *testing.T) {
		c := canonOf(t, strings.Repeat("é", 70), strings.Repeat("x", 65), strings.Repeat("u", 40))
		if n := utf8.RuneCountInString(c.First); n != 64 {
			t.Errorf("first has %d runes, want 64", n)
		}
		if n := utf8.RuneCountInString(c.Last); n != 64 {
			t.Errorf("last has %d runes, want 64", n)
		}
		if n := utf8.RuneCountInString(c.Username); n != 32 {
			t.Errorf("username has %d runes, want 32", n)
		}
	})

	t.Run("a cut never splits a multibyte character", func(t *testing.T) {
		c := canonOf(t, strings.Repeat("日", 65), strings.Repeat("\U0001F600", 65), strings.Repeat("ü", 40))
		for name, s := range map[string]string{"first": c.First, "last": c.Last, "username": c.Username} {
			if !utf8.ValidString(s) {
				t.Errorf("%s is not valid UTF-8 after the cut", name)
			}
		}
	})

	t.Run("exactly at the limit nothing is lost", func(t *testing.T) {
		c := canonOf(t, strings.Repeat("a", 64), strings.Repeat("b", 64), strings.Repeat("c", 32))
		if len(c.First) != 64 || len(c.Last) != 64 || len(c.Username) != 32 {
			t.Errorf("got %d %d %d", len(c.First), len(c.Last), len(c.Username))
		}
	})

	t.Run("invisible padding does not push the visible part out", func(t *testing.T) {
		pad := strings.Repeat("​ㅤ⠀", 100)
		c := canonOf(t, pad+"visible"+pad, pad+"last"+pad, pad+"user"+pad)
		if c.First != "visible" || c.Last != "last" || c.Username != "user" {
			t.Errorf("got %q %q %q", c.First, c.Last, c.Username)
		}
	})

	t.Run("a cut that ends on a space is trimmed", func(t *testing.T) {
		c := canonOf(t, strings.Repeat("a", 63)+" "+strings.Repeat("b", 10), "", "")
		if c.First != strings.Repeat("a", 63) {
			t.Errorf("First = %q", c.First)
		}
	})
}

func TestCanonHash(t *testing.T) {
	t.Run("is 32 lower-case hex characters", func(t *testing.T) {
		if h := canonOf(t, "Anna", "K", "annak").Hash; !hex32.MatchString(h) {
			t.Errorf("Hash = %q", h)
		}
	})

	t.Run("equal canonical names give equal hashes whatever the input form", func(t *testing.T) {
		a := canonOf(t, "Anna", "K", "annak")
		b := canonOf(t, "Ａnna​", " K ", "@AnnaK")
		if a.Hash != b.Hash {
			t.Errorf("hashes differ: %s %s", a.Hash, b.Hash)
		}
	})

	t.Run("case never matters in the hash", func(t *testing.T) {
		a := canonOf(t, "ANNA", "KOWALSKI", "AnnaK")
		b := canonOf(t, "anna", "kowalski", "annak")
		if a.Hash != b.Hash {
			t.Errorf("hashes differ: %s %s", a.Hash, b.Hash)
		}
		if a.First == b.First {
			t.Error("the text that is sent must keep its case")
		}
	})

	t.Run("a character moved across a field boundary changes the hash", func(t *testing.T) {
		pairs := [][2][3]string{
			{{"a b", "c", ""}, {"a", "b c", ""}},
			{{"ab", "c", ""}, {"a", "bc", ""}},
			{{"x", "", ""}, {"", "x", ""}},
			{{"x", "", ""}, {"", "", "x"}},
			{{"", "x", ""}, {"", "", "x"}},
			{{"x", "1:y", "z"}, {"x1", ":yz", ""}},
			{{"ab", "", "c"}, {"a", "b", "c"}},
		}
		for _, p := range pairs {
			left, lok := Of(p[0][0], p[0][1], p[0][2])
			right, rok := Of(p[1][0], p[1][1], p[1][2])
			if !lok || !rok {
				t.Fatalf("pair %v is not judgeable", p)
			}
			if left.Hash == right.Hash {
				t.Errorf("%v and %v share the hash %s", p[0], p[1], left.Hash)
			}
		}
	})

	t.Run("a missing field differs from a field holding text", func(t *testing.T) {
		none := canonOf(t, "Anna", "", "")
		withLast := canonOf(t, "Anna", "K", "")
		withUser := canonOf(t, "Anna", "", "k")
		if none.Hash == withLast.Hash || none.Hash == withUser.Hash || withLast.Hash == withUser.Hash {
			t.Error("a missing field must not hash like a field holding text")
		}
	})

	t.Run("the format is pinned", func(t *testing.T) {
		// SHA-256 of "mrn1\x00" + "4:anna" + "1:k" + "5:annak", first 16 bytes. A
		// silent change of this format would orphan every cached baseline and
		// verdict, so the literal is asserted on purpose.
		if got := canonOf(t, "Anna", "K", "AnnaK").Hash; got != "a0bef801bfd1c34c855019bf0d328f90" {
			t.Errorf("Hash = %s", got)
		}
		// Lengths are bytes: "привет" is 12 bytes, 6 runes.
		if got := canonOf(t, "Привет", "", "").Hash; got != "13cb65eda63d1609961b5ee8f6b4c4b7" {
			t.Errorf("Hash = %s", got)
		}
	})
}

func TestCanonJudgeable(t *testing.T) {
	notJudgeable := map[string][3]string{
		"all empty":                {"", "", ""},
		"only whitespace":          {" \t ", "\n", " "},
		"only invisible runes":     {"​‍﻿", "‮", "⁦⁩"},
		"only fillers":             {"ㅤ", "⠀", "ᅟᅠﾠ"},
		"lone at sign":             {"", "", "@"},
		"deleted account":          {"Deleted Account", "", ""},
		"deleted account any case": {"dELETED aCCOUNT", "", ""},
		"deleted account padded":   {" Deleted  Account​ ", "", ""},
		"deleted account, blanks":  {"Deleted Account", "ㅤ", "@​"},
	}
	for name, in := range notJudgeable {
		c, ok := Of(in[0], in[1], in[2])
		if ok {
			t.Errorf("%s: judgeable, want not", name)
		}
		if c != (Canon{}) {
			t.Errorf("%s: got %+v, want the zero Canon", name, c)
		}
	}

	judgeable := map[string][3]string{
		"only a username":                       {"", "", "spamchannel"},
		"filler first name with a username":     {"ㅤ", "", "ad_bot"},
		"braille first name with a username":    {"⠀", "", "ad_bot"},
		"deleted account with a last name":      {"Deleted Account", "Smith", ""},
		"deleted account with a username":       {"Deleted Account", "", "someone"},
		"deleted account as a last name":        {"", "Deleted Account", ""},
		"a name that only starts like it":       {"Deleted Accounts", "", ""},
		"an ordinary name":                      {"Anna", "", ""},
		"a name that is only a symbol":          {"❤", "", ""},
		"a name that is only a decorated mark":  {"नम", "", ""},
	}
	for name, in := range judgeable {
		c, ok := Of(in[0], in[1], in[2])
		if !ok {
			t.Errorf("%s: not judgeable, want judgeable", name)
			continue
		}
		if !hex32.MatchString(c.Hash) {
			t.Errorf("%s: Hash = %q", name, c.Hash)
		}
	}
}

func TestCanonVerdictKey(t *testing.T) {
	c := canonOf(t, "Anna", "K", "AnnaK")
	key := VerdictKey("claude-haiku-5-5", c)

	t.Run("is 32 lower-case hex characters and stable", func(t *testing.T) {
		if !hex32.MatchString(key) {
			t.Errorf("key = %q", key)
		}
		if again := VerdictKey("claude-haiku-5-5", canonOf(t, "Anna", "K", "AnnaK")); again != key {
			t.Errorf("same pair, different keys: %s %s", key, again)
		}
	})

	t.Run("changes with the model and with the names", func(t *testing.T) {
		if VerdictKey("claude-haiku-5-6", c) == key {
			t.Error("the model must be part of the key")
		}
		if VerdictKey("claude-haiku-5-5", canonOf(t, "Anna", "K", "annak2")) == key {
			t.Error("the names hash must be part of the key")
		}
	})

	t.Run("the model cannot slide into the hash", func(t *testing.T) {
		a := VerdictKey("m", Canon{Hash: "1" + c.Hash})
		b := VerdictKey("m1", Canon{Hash: c.Hash})
		if a == b {
			t.Error("model and hash share a boundary")
		}
	})

	t.Run("the format is pinned", func(t *testing.T) {
		// SHA-256 of "mrv1\x00" + model + "\x00" + names hash, first 16 bytes.
		if key != "57147973645232e303036e2a601f67db" {
			t.Errorf("key = %s", key)
		}
	})
}
