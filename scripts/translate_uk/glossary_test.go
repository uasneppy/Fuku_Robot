package main

import (
	"os"
	"path/filepath"
	"testing"
)

func testGlossary() *Glossary {
	return &Glossary{
		Terms: []Term{
			{English: "ban", Ukrainian: "бан", Forbidden: []string{"блок"}},
			{English: "staff group", Ukrainian: "штатна група"},
			{English: "mute"}, // not decided yet: must never warn
		},
		RussianWords: []string{"что", "это"},
	}
}

func TestLoadGlossaryReadsTermsAndWords(t *testing.T) {
	g, err := LoadGlossary("testdata/glossary.yml")
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Terms) != 3 || g.Terms[0].Ukrainian != "бан" || g.Terms[0].Forbidden[0] != "блок" {
		t.Fatalf("terms = %+v", g.Terms)
	}
	if len(g.RussianWords) != 2 {
		t.Fatalf("russian words = %v", g.RussianWords)
	}
}

func TestLoadGlossaryMissingFileIsEmpty(t *testing.T) {
	g, err := LoadGlossary(filepath.Join(t.TempDir(), "nope.yml"))
	if err != nil || g == nil || len(g.Terms) != 0 {
		t.Fatalf("got %+v, %v", g, err)
	}
}

func TestLoadGlossaryRejectsBrokenYAML(t *testing.T) {
	path := filepath.Join(t.TempDir(), "g.yml")
	if err := os.WriteFile(path, []byte("terms: [unclosed"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadGlossary(path); err == nil {
		t.Fatal("want an error")
	}
}

func TestGlossaryWarnsWhenChosenRenderingMissing(t *testing.T) {
	g := testGlossary()
	if got := g.Check([]string{"Ban a user"}, []string{"Забанити користувача на вічний бану"}); len(got) != 0 {
		t.Fatalf("a stem inside an inflected word must pass, got %v", got)
	}
	got := g.Check([]string{"Ban a user"}, []string{"Вигнати користувача"})
	if !hasCode(got, "glossary") {
		t.Fatalf("want a glossary warning, got %v", got)
	}
}

func TestGlossaryWarnsOnForbiddenVariant(t *testing.T) {
	g := testGlossary()
	got := g.Check([]string{"Ban a user"}, []string{"Бан: Блокування користувача"})
	if !hasCode(got, "glossary") {
		t.Fatalf("want a warning for the forbidden variant, got %v", got)
	}
}

func TestGlossarySkipsTermsWithoutUkrainian(t *testing.T) {
	g := testGlossary()
	if got := g.Check([]string{"Mute the user"}, []string{"Що завгодно"}); len(got) != 0 {
		t.Fatalf("an undecided term must stay silent, got %v", got)
	}
}

func TestGlossaryMatchesEnglishOnWholeWordsOnly(t *testing.T) {
	g := testGlossary()
	if got := g.Check([]string{"A banner is shown"}, []string{"Показується прапорець"}); len(got) != 0 {
		t.Fatalf("'banner' is not 'ban', got %v", got)
	}
	if got := g.Check([]string{"Bans a user"}, []string{"Вигнати"}); !hasCode(got, "glossary") {
		t.Fatalf("a plural form should still count as the term, got %v", got)
	}
	if got := g.Check([]string{"Open the Staff Group panel"}, []string{"Відкрийте панель"}); !hasCode(got, "glossary") {
		t.Fatalf("a two-word term should match case-insensitively, got %v", got)
	}
}

func TestHintsListsTermsPresentInEnglish(t *testing.T) {
	g := testGlossary()
	hints := g.Hints([]string{"Mute and ban people", "nothing here"})
	if len(hints) != 2 || hints[0].English != "ban" || hints[1].English != "mute" {
		t.Fatalf("hints = %+v", hints)
	}
}

func TestRussianWordsAreMatchedAsWholeCyrillicWords(t *testing.T) {
	g := testGlossary()
	if got := g.FlagRussian([]string{"что-то сталося"}); !hasCode(got, "ru_word") {
		t.Fatalf("'что-то' contains the word 'что', got %v", got)
	}
	if got := g.FlagRussian([]string{"Це те, що потрібно, якщо так"}); len(got) != 0 {
		t.Fatalf("Ukrainian 'що' and 'якщо' must not be flagged, got %v", got)
	}
	if got := g.FlagRussian([]string{"ЧТО"}); !hasCode(got, "ru_word") {
		t.Fatalf("matching is case-insensitive, got %v", got)
	}
}

func TestRussianWordsNamesTheListItem(t *testing.T) {
	g := testGlossary()
	got := g.FlagRussian([]string{"добре", "это"})
	if len(got) != 1 || got[0].Message[:6] != "item 2" {
		t.Fatalf("got %v", got)
	}
}
