//go:build testtools

package xenopersona

import (
	"regexp"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// TestStaticSystemTextIsStable proves the text is the same bytes on every call
// and is the persona followed by the archive, which is what makes a provider's
// prompt cache hit across chats and users.
func TestStaticSystemTextIsStable(t *testing.T) {
	first := StaticSystemText()
	for range 3 {
		if got := StaticSystemText(); got != first {
			t.Fatal("StaticSystemText changed between calls")
		}
	}
	persona := strings.TrimSpace(personaMD)
	if !strings.HasPrefix(first, persona+separator) {
		t.Error("the static text does not start with the persona and the separator")
	}
	if !strings.HasSuffix(first, strings.TrimSpace(archiveMD[strings.Index(archiveMD, "-->")+3:])) {
		t.Error("the static text does not end with the archive")
	}
	if strings.Contains(first, "<!--") || strings.Contains(first, "LORE.md") {
		t.Error("the archive's source comment reached the static text")
	}
}

// TestStaticSystemTextHasNothingThatVaries keeps every per-chat value out of
// the cached text: no template placeholder, no format verb and no date of the run.
func TestStaticSystemTextHasNothingThatVaries(t *testing.T) {
	text := StaticSystemText()
	patterns := map[string]*regexp.Regexp{
		"brace placeholder": regexp.MustCompile(`\{\{|\{[A-Za-z_][A-Za-z0-9_]*\}`),
		"format verb":       regexp.MustCompile(`%[-+# 0-9.]*[sdvqxf]`),
		"angle placeholder": regexp.MustCompile(`<(chat|title|name|user|date|group)[^>]*>`),
		"dollar variable":   regexp.MustCompile(`\$\{?[A-Za-z_]`),
	}
	for name, re := range patterns {
		if m := re.FindString(text); m != "" {
			t.Errorf("%s found in the static text: %q", name, m)
		}
	}
	now := time.Now().UTC()
	for _, layout := range []string{"2006-01-02", "02.01.2006", "2 January 2006", "January 2, 2006"} {
		if d := now.Format(layout); strings.Contains(text, d) {
			t.Errorf("the static text contains today's date %q", d)
		}
	}
}

// TestStaticSystemTextSize holds the text between 6,000 and 12,000 tokens by a
// rough count of four characters a token: above Gemini's 4,096-token minimum
// for implicit caching, small enough to stay cheap.
func TestStaticSystemTextSize(t *testing.T) {
	tokens := utf8.RuneCountInString(StaticSystemText()) / 4
	if tokens < 6000 || tokens > 12000 {
		t.Errorf("static text is about %d tokens, want 6,000 to 12,000", tokens)
	}
	t.Logf("static text: about %d tokens (persona %d, archive %d)",
		tokens, utf8.RuneCountInString(personaMD)/4, utf8.RuneCountInString(archiveMD)/4)
}

// TestStaticSystemTextNamesNoOldProject keeps the bot's earlier names out of
// what the model is told.
func TestStaticSystemTextNamesNoOldProject(t *testing.T) {
	lower := strings.ToLower(StaticSystemText())
	for _, word := range []string{"fuku", "alita", "divideprojects"} {
		if strings.Contains(lower, word) {
			t.Errorf("the static text contains %q", word)
		}
	}
}

// TestStaticSystemTextKeepsTheCanonRules checks that the facts the canon rules
// depend on reach the model exactly, so a fan cannot catch the bot out on them.
func TestStaticSystemTextKeepsTheCanonRules(t *testing.T) {
	text := StaticSystemText()
	for _, want := range []string{
		"364,117",          // Lawu City dead
		"4,045 m",          // Sky Scraper length
		"7,713.8 m",        // Sky Scraper height
		"open for 7 years", // the waste door
		"only 74 versions", // the Xenobot line
		"Xenobot-1 dates from 1987",
		"from 40 °C to 16 °C", // Global Freezing
		"## Contradictions",
		"## Not on record",
		"No record on file.",
		"It is explicitly not canon", // the children's book
		"Xenobot-73",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the static text lacks %q", want)
		}
	}
}

// TestArchiveWritesNoAddress keeps web addresses out of the archive, so the
// model has none to repeat; it names the places to read and watch instead.
func TestArchiveWritesNoAddress(t *testing.T) {
	re := regexp.MustCompile(`(?i)https?://|www\.|t\.me/|\b[a-z0-9-]+\.(art|com|dev|gg|org|net)\b`)
	if m := re.FindString(StaticSystemText()); m != "" {
		t.Errorf("the static text contains an address: %q", m)
	}
}
