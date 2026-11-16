//go:build testtools

package modules

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/uasneppy/fuku_robot/fuku/i18n"
)

// settingsListTestView is a private menu of the group the UI-SPEC's mock values use.
func settingsListTestView(tr *i18n.Translator) settingsView {
	return settingsView{
		Tr: tr, Private: true, OwnerKey: "c", OwnerValue: "-1001234567890",
		GroupID: -1001234567890, GroupTitle: "Fuku Test Group",
	}
}

// settingsListTestDef is the Blacklist table with the delete codes the page gets in plan 19-03 task 2, so the
// renderer is held to the full keyboard whatever the page registers.
func settingsListTestDef() settingsListDef {
	def := settingsBlacklistDef()
	def.AskCode, def.YesCode = "bx", "by"
	return def
}

// settingsListTestEntries makes n entries w01.. with row IDs 100+i, on warn.
func settingsListTestEntries(n int) []settingsListEntry {
	entries := make([]settingsListEntry, 0, n)
	for i, word := range settingsListWords(n) {
		entries = append(entries, settingsListEntry{ID: int64(100 + i), Text: word, Action: "warn"})
	}
	return entries
}

func TestSettingsListRender(t *testing.T) {
	withRealLocales(t)
	tr := i18n.MustNewTranslator("en")
	v := settingsListTestView(tr)
	def := settingsListTestDef()

	t.Run("the order is the lower-cased text in code point order, ties by row ID", func(t *testing.T) {
		sorted := settingsListSort([]settingsListEntry{
			{ID: 9, Text: "beta"}, {ID: 5, Text: "a"}, {ID: 2, Text: "A"}, {ID: 7, Text: "Zed"}, {ID: 3, Text: "a"}, {ID: 1, Text: "alpha"},
		})
		var ids []int64
		for _, e := range sorted {
			ids = append(ids, e.ID)
		}
		if want := []int64{2, 3, 5, 1, 9, 7}; fmt.Sprint(ids) != fmt.Sprint(want) {
			t.Errorf("order = %v, want %v", ids, want)
		}
	})

	t.Run("the keyed sort gives exactly the order of settingsListCompare, and leaves its input alone (SEC-19-01)", func(t *testing.T) {
		texts := []string{"Ёж", "ёж", "ÄB", "äb", "Zed", "zed", "a", "A", "", "я", "Я", "İ", "i", "🔤", "ß", "SS"}
		var entries []settingsListEntry
		for i := 0; i < 200; i++ {
			entries = append(entries, settingsListEntry{ID: int64(1000 - i), Text: texts[i%len(texts)]})
		}
		before := slices.Clone(entries)
		want := slices.Clone(entries)
		slices.SortStableFunc(want, settingsListCompare)
		if got := settingsListSort(entries); !slices.Equal(got, want) {
			t.Errorf("the keyed sort differs from the comparison sort:\n got %v\nwant %v", got, want)
		}
		if !slices.Equal(entries, before) {
			t.Error("settingsListSort changed its input")
		}
	})

	t.Run("pages: one up to eight entries, then one more for every eight", func(t *testing.T) {
		for n, want := range map[int]int{0: 1, 1: 1, 8: 1, 9: 2, 16: 2, 17: 3} {
			if got := settingsListPages(n); got != want {
				t.Errorf("settingsListPages(%d) = %d, want %d", n, got, want)
			}
		}
	})

	t.Run("a label is stripped, collapsed, cut by code points and escaped after the cut", func(t *testing.T) {
		for _, tc := range []struct{ name, in, want string }{
			{"bidi controls dropped", "ab\u202ecd\u2066e\u200f", "abcde"},
			{"line breaks and runs collapsed", "a\n\n b\t  c", "a b c"},
			{"markup escaped", "<b>&", "&lt;b&gt;&amp;"},
			{"32 code points stay whole", strings.Repeat("я", 32), strings.Repeat("я", 32)},
			{"33 code points are cut with an ellipsis", strings.Repeat("я", 33), strings.Repeat("я", 32) + "…"},
			{"32 ampersands cut from 40 give 32 escapes and the ellipsis", strings.Repeat("&", 40), strings.Repeat("&amp;", 32) + "…"},
		} {
			if got := settingsListLabel(tc.in, settingsListLineMax); got != tc.want {
				t.Errorf("%s: settingsListLabel = %q, want %q", tc.name, got, tc.want)
			}
		}
		if got, want := settingsListLabel(strings.Repeat("&", 101), settingsListCardMax), strings.Repeat("&amp;", 100)+"…"; got != want {
			t.Errorf("a card label of 101 ampersands = %q, want the cut at 100", got)
		}
	})

	t.Run("a row ID is 1 to 17 digits, no sign, no leading zero, positive", func(t *testing.T) {
		for raw, want := range map[string]bool{
			"1": true, "12345678901234567": true, "99999999999999999": true,
			"": false, "0": false, "012": false, "-5": false, "+5": false, "12a": false, "١٢": false,
			"123456789012345678": false, "9223372036854775807": false,
		} {
			if _, ok := settingsListParseID(raw); ok != want {
				t.Errorf("settingsListParseID(%q) ok = %v, want %v", raw, ok, want)
			}
		}
	})

	t.Run("a middle page numbers its entries globally and draws the delete rows and both pager buttons", func(t *testing.T) {
		entries := settingsListTestEntries(20)
		screen := renderSettingsList(v, def, entries, 2, "")
		for n := 9; n <= 16; n++ {
			if want := fmt.Sprintf("%d. <code>w%02d</code>", n, n); !strings.Contains(screen.Text, want) {
				t.Errorf("page 2 lacks the line %q:\n%s", want, screen.Text)
			}
		}
		for _, absent := range []string{"8. <code>", "17. <code>"} {
			if strings.Contains(screen.Text, absent) {
				t.Errorf("page 2 shows %q", absent)
			}
		}
		if !strings.Contains(screen.Text, "Page 2/3") || !strings.Contains(screen.Text, "Words: 20") {
			t.Errorf("page 2 lacks the page or count line:\n%s", screen.Text)
		}
		keyboard := screen.Keyboard.InlineKeyboard
		if got, want := settingsListRowWidths(keyboard), []int{4, 4, 2, 2, 2}; fmt.Sprint(got) != fmt.Sprint(want) {
			t.Fatalf("row widths = %v, want %v", got, want)
		}
		for i, button := range keyboard[0] {
			if want := fmt.Sprintf("🗑 %d", 9+i); button.Text != want {
				t.Errorf("delete button %d = %q, want %q", i, button.Text, want)
			}
			fields := settingsButtonFields(t, button)
			if fields["a"] != "bx" || fields["r"] != strconv.Itoa(108+i) || fields["v"] != "2" || fields["c"] != "-1001234567890" {
				t.Errorf("delete button %q carries %v", button.Text, fields)
			}
			if strings.Contains(button.CallbackData, fmt.Sprintf("w%02d", 9+i)) {
				t.Errorf("delete button data %q carries the entry text", button.CallbackData)
			}
		}
		prev, next := keyboard[2][0], keyboard[2][1]
		if f := settingsButtonFields(t, prev); f["a"] != "bp" || f["v"] != "1" || prev.Text != "⬅️ Prev" {
			t.Errorf("Prev = %q %v", prev.Text, f)
		}
		if f := settingsButtonFields(t, next); f["a"] != "bp" || f["v"] != "3" || next.Text != "Next ➡️" {
			t.Errorf("Next = %q %v", next.Text, f)
		}
		if screen.Omitted != 0 {
			t.Errorf("Omitted = %d, want 0", screen.Omitted)
		}
	})

	t.Run("the first page has Next only, the last Prev only, one page no pager and no page line", func(t *testing.T) {
		entries := settingsListTestEntries(20)
		first := renderSettingsList(v, def, entries, 1, "").Keyboard.InlineKeyboard
		if got := settingsListRowWidths(first); fmt.Sprint(got) != fmt.Sprint([]int{4, 4, 1, 2, 2}) || first[2][0].Text != "Next ➡️" {
			t.Errorf("first page rows = %v, pager %q", got, first[2][0].Text)
		}
		last := renderSettingsList(v, def, entries, 3, "")
		if got := settingsListRowWidths(last.Keyboard.InlineKeyboard); fmt.Sprint(got) != fmt.Sprint([]int{4, 1, 2, 2}) || last.Keyboard.InlineKeyboard[1][0].Text != "⬅️ Prev" {
			t.Errorf("last page (4 entries) rows = %v", got)
		}
		one := renderSettingsList(v, def, settingsListTestEntries(5), 1, "")
		if got := settingsListRowWidths(one.Keyboard.InlineKeyboard); fmt.Sprint(got) != fmt.Sprint([]int{4, 1, 2, 2}) {
			t.Errorf("a page of 5 entries has rows %v, want {4,1,2,2}", got)
		}
		if strings.Contains(one.Text, "Page 1/1") {
			t.Errorf("a one-page list shows a page line:\n%s", one.Text)
		}
	})

	t.Run("a page past the end is the last page and page 0 is page 1", func(t *testing.T) {
		entries := settingsListTestEntries(20)
		if text := renderSettingsList(v, def, entries, 9999, "").Text; !strings.Contains(text, "Page 3/3") || !strings.Contains(text, "17. <code>w17</code>") {
			t.Errorf("page 9999 did not draw the last page:\n%s", text)
		}
		if text := renderSettingsList(v, def, entries, 0, "").Text; !strings.Contains(text, "Page 1/3") {
			t.Errorf("page 0 did not draw page 1:\n%s", text)
		}
	})

	t.Run("an empty list has the intro and the empty line and no count, page or delete row", func(t *testing.T) {
		screen := renderSettingsList(v, def, nil, 1, "")
		if !strings.Contains(screen.Text, "There are no blacklisted words yet.") {
			t.Errorf("the empty list lacks its line:\n%s", screen.Text)
		}
		for _, absent := range []string{"Words:", "Page ", "Action"} {
			if strings.Contains(screen.Text, absent) {
				t.Errorf("the empty list shows %q:\n%s", absent, screen.Text)
			}
		}
		if got := settingsListRowWidths(screen.Keyboard.InlineKeyboard); fmt.Sprint(got) != fmt.Sprint([]int{1, 2}) {
			t.Errorf("the empty list has rows %v, want the Add button and the navigation row", got)
		}
	})

	t.Run("a row whose ID has 18 digits gets no delete button and is counted as omitted", func(t *testing.T) {
		entries := settingsListTestEntries(3)
		entries[1].ID = 123456789012345678
		screen := renderSettingsList(v, def, entries, 1, "")
		if screen.Omitted != 1 {
			t.Errorf("Omitted = %d, want 1", screen.Omitted)
		}
		if got := len(screen.Keyboard.InlineKeyboard[0]); got != 2 {
			t.Errorf("the delete row has %d buttons, want 2 (the 18-digit row has none)", got)
		}
		if !strings.Contains(screen.Text, "2. <code>w02</code>") {
			t.Errorf("the entry is still listed:\n%s", screen.Text)
		}
	})

	t.Run("a long entry is cut at 32 code points and escaped, and a notice follows the page", func(t *testing.T) {
		entries := []settingsListEntry{{ID: 1, Text: strings.Repeat("&", 40)}}
		screen := renderSettingsList(v, def, entries, 1, settingsNoticeWarn("NOTICE"))
		if !strings.Contains(screen.Text, "1. <code>"+strings.Repeat("&amp;", 32)+"…</code>") {
			t.Errorf("the entry line is wrong:\n%s", screen.Text)
		}
		if !strings.HasSuffix(screen.Text, "⚠️ NOTICE") {
			t.Errorf("the notice does not end the text:\n%s", screen.Text)
		}
	})

	t.Run("a table with an admin tag marks an admin-only entry", func(t *testing.T) {
		tagged := def
		tagged.AdminTagKey = "settings_state_private_only"
		entries := []settingsListEntry{{ID: 1, Text: "open"}, {ID: 2, Text: "staff", AdminOnly: true}}
		text := renderSettingsList(v, tagged, entries, 1, "").Text
		if !strings.Contains(text, "2. <code>staff</code> · In private") || strings.Contains(text, "1. <code>open</code> ·") {
			t.Errorf("the admin tag is wrong:\n%s", text)
		}
		if plain := renderSettingsList(v, def, entries, 1, "").Text; strings.Contains(plain, "</code> · ") {
			t.Errorf("a table without a tag key shows a tag:\n%s", plain)
		}
	})

	t.Run("no button of the list carries the entry text", func(t *testing.T) {
		screen := renderSettingsList(v, def, []settingsListEntry{{ID: 1, Text: "casino"}}, 1, "")
		for _, button := range settingsAllButtons(screen.Keyboard.InlineKeyboard) {
			if strings.Contains(button.Text, "casino") || strings.Contains(button.CallbackData, "casino") {
				t.Errorf("button %q (%q) carries the entry text", button.Text, button.CallbackData)
			}
		}
	})
}
