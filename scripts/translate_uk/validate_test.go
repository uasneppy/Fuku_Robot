package main

import (
	"strings"
	"testing"
)

func hasCode(issues []Issue, code string) bool {
	for _, i := range issues {
		if i.Code == code {
			return true
		}
	}
	return false
}

func codesOf(issues []Issue) []string {
	var out []string
	for _, i := range issues {
		out = append(out, i.Code)
	}
	return out
}

type blockCase struct {
	name     string
	en, uk   string
	wantCode string // empty means the pair must be accepted
}

func runBlockCases(t *testing.T, cases []blockCase) {
	t.Helper()
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := BlockingIssues([]string{c.en}, []string{c.uk})
			if c.wantCode == "" {
				if len(got) != 0 {
					t.Fatalf("want accepted, got %v", got)
				}
				return
			}
			if !hasCode(got, c.wantCode) {
				t.Fatalf("want code %q, got %v", c.wantCode, codesOf(got))
			}
		})
	}
}

func TestPlaceholdersMustMatch(t *testing.T) {
	runBlockCases(t, []blockCase{
		{"missing %s", "Hi %s", "Привіт", "placeholders"},
		{"extra %d", "Hi", "Привіт %d", "placeholders"},
		{"renamed {name}", "Hi {name}", "Привіт {ім'я}", "placeholders"},
		{"reordered verbs accepted", "%[1]s and %[2]s", "%[2]s і %[1]s", ""},
		{"percent pair kept", "100%% sure %s", "100%% певно %s", ""},
		{"percent pair dropped", "100%% sure", "100% певно", "placeholders"},
		{"percent then space is prose", "100% of users", "100% користувачів", ""},
		{"percent space letter is prose", "Done 50% o", "Готово 50% о", ""},
		{"reordered braces accepted", "{first} met {last}", "{last} зустрів {first}", ""},
	})
}

func TestTagsMustMatch(t *testing.T) {
	runBlockCases(t, []blockCase{
		{"missing closing tag", "<b>x</b>", "<b>х", "tags"},
		{"extra tag", "x", "<i>х</i>", "tags"},
		{"altered link target", `<a href="https://a.example">x</a>`, `<a href="https://b.example">х</a>`, "tags"},
		{"reordered tags accepted", "<b>a</b> <code>b</code>", "<code>б</code> <b>а</b>", ""},
		// <userhandle>, <keyword> and the like are argument placeholders in usage
		// lines, not Telegram tags; ru.yml localizes them and so may uk.yml.
		{"pseudo-tag kept", "Use <userhandle> or <yes>", "Використовуйте <userhandle> або <yes>", ""},
		{"pseudo-tag renamed is a translation", "Use /ban <userhandle> or <yes>", "Використовуйте /ban <користувач> або <так>", ""},
		{"pseudo-tag dropped", "Use <userhandle>", "Використовуйте користувача", "tags"},
		{"pseudo-tag added", "Use it", "Використовуйте <ще>", "tags"},
		{"telegram tag misspelled", "<code>x</code>", "<cod>х</code>", "tags"},
		// Telegram's other spellings are real tags too: renaming one would make
		// Telegram reject the whole message.
		{"strike is a telegram tag", "<strike>x</strike>", "<закреслено>х</закреслено>", "tags"},
		{"del is a telegram tag", "<del>x</del>", "<видалено>х</видалено>", "tags"},
		{"ins is a telegram tag", "<ins>x</ins>", "<вставлено>х</вставлено>", "tags"},
		{"tg-emoji is a telegram tag", `<tg-emoji emoji-id="1">x</tg-emoji>`, `<емодзі>х</емодзі>`, "tags"},
		{"comparison signs are not tags", "if a < b and c > d", "якщо a < b і c > d", ""},
	})
}

func TestNestingChecked(t *testing.T) {
	runBlockCases(t, []blockCase{
		{"crossed tags refused", "<b><i>x</i></b>", "<b><i>х</b></i>", "nesting"},
		{"well nested accepted", "<b><i>x</i></b>", "<i><b>х</b></i>", ""},
		// A quirk already in the English text must not block the translation.
		{"crossed strike is refused", "<strike><b>x</b></strike>", "<strike><b>х</strike></b>", "nesting"},
		{"english already crossed is skipped", "<b>x<i>y</b></i>", "<b>х<i>у</b></i>", ""},
	})
}

func TestAngleBracketsAndEntitiesMustMatch(t *testing.T) {
	runBlockCases(t, []blockCase{
		{"entity replaced by a bracket", "a &lt; b", "а < б", "entities"},
		{"entity kept", "Use &lt;user&gt;", "Використовуйте &lt;user&gt;", ""},
		{"stray bracket added", "plain", "plain <", "angle_brackets"},
		{"stray closing bracket added", "plain", "plain >", "angle_brackets"},
		{"entity dropped", "Fish &amp; chips", "Риба і чіпси", "entities"},
	})
}

func TestListLengthMustMatch(t *testing.T) {
	en := []string{"a", "b", "c", "d"}
	got := BlockingIssues(en, []string{"а", "б", "в"})
	if !hasCode(got, "list_length") {
		t.Fatalf("want list_length, got %v", codesOf(got))
	}
	if got := BlockingIssues(en, []string{"а", "б", "в", "г"}); len(got) != 0 {
		t.Fatalf("equal-length list refused: %v", got)
	}
}

func TestListItemsAreComparedIndexByIndex(t *testing.T) {
	en := []string{"one %s", "two"}
	uk := []string{"один", "два %s"} // placeholders swapped between items
	got := BlockingIssues(en, uk)
	if !hasCode(got, "placeholders") {
		t.Fatalf("want placeholders, got %v", codesOf(got))
	}
	if !strings.Contains(got[0].Message, "item 1") {
		t.Fatalf("message should name the item: %q", got[0].Message)
	}
}

func TestWarnings(t *testing.T) {
	long := func(r rune, n int) string { return strings.Repeat(string(r), n) }
	cases := []struct {
		name   string
		en, uk string
		want   string // empty means no warnings at all
	}{
		{"russian letter ы", "Hello", "Привет ы", "cyrillic_ru"},
		{"russian letter э upper", "Hello", "Э", "cyrillic_ru"},
		{"russian letter ъ", "Hello", "объявление", "cyrillic_ru"},
		{"russian letter ё", "Hello", "всё", "cyrillic_ru"},
		{"clean ukrainian", "Hello", "Привіт, світе! ї є ґ", ""},
		{"over 1.4x and at least 40 runes warns", long('x', 50), long('а', 71), "length"},
		{"exactly 1.4x does not warn", long('x', 50), long('а', 70), ""},
		{"short strings are exempt", long('x', 10), long('а', 30), ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Warnings([]string{c.en}, []string{c.uk})
			if c.want == "" {
				if len(got) != 0 {
					t.Fatalf("want none, got %v", got)
				}
				return
			}
			if !hasCode(got, c.want) {
				t.Fatalf("want %q, got %v", c.want, codesOf(got))
			}
		})
	}
}

func TestTokenHelpersReturnSortedMultisets(t *testing.T) {
	got := PlaceholdersOf("{b} %s {a} %s 100% of")
	want := []string{"%s", "%s", "{a}", "{b}"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("PlaceholdersOf = %v, want %v", got, want)
	}
	tags := TagsOf(`<b>x</b> <a href="u">y</a> if 1 < 2`)
	if len(tags) != 4 || tags[0] != "</a>" {
		t.Fatalf("TagsOf = %v", tags)
	}
}
