//go:build testtools

package modules

import (
	"context"
	"errors"
	"maps"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/uasneppy/fuku_robot/fuku/i18n"
)

// settingsPhase19Pages are the three list pages Phase 19 registered, in grid order.
var settingsPhase19Pages = []string{settingsPageBlacklist, settingsPageFilters, settingsPageNotes}

// settingsPhase19Code is one action code of a list page with the fields of a valid press of a private menu (the owner
// field c is added by the test).
type settingsPhase19Code struct {
	code, page string
	fields     map[string]string
}

// settingsPhase19Codes lists every action code the three list pages register and a press that is valid for it. The
// list is written out here and compared with what the registry holds in both directions, so a code added to a page
// without a row here, or a row without a registered code, fails TestSettingsPhase19Codes.
func settingsPhase19Codes(minute string) []settingsPhase19Code {
	return []settingsPhase19Code{
		{"bp", settingsPageBlacklist, map[string]string{"v": "1"}},
		{"bx", settingsPageBlacklist, map[string]string{"r": "5", "v": "1"}},
		{"by", settingsPageBlacklist, map[string]string{"r": "5", "e": minute}},
		{"bn", settingsPageBlacklist, map[string]string{}},
		{"bc", settingsPageBlacklist, map[string]string{"t": "w"}},
		{"bo", settingsPageBlacklist, map[string]string{}},
		{"bs", settingsPageBlacklist, map[string]string{"v": "warn"}},
		{"ig", settingsPageFilters, map[string]string{"v": "1"}},
		{"ix", settingsPageFilters, map[string]string{"r": "5", "v": "1"}},
		{"iy", settingsPageFilters, map[string]string{"r": "5", "e": minute}},
		{"ia", settingsPageFilters, map[string]string{}},
		{"ic", settingsPageFilters, map[string]string{"t": "k"}},
		{"eg", settingsPageNotes, map[string]string{"v": "1"}},
		{"ex", settingsPageNotes, map[string]string{"r": "5", "v": "1"}},
		{"ey", settingsPageNotes, map[string]string{"r": "5", "e": minute}},
		{"ea", settingsPageNotes, map[string]string{}},
		{"ec", settingsPageNotes, map[string]string{"t": "n"}},
	}
}

func TestSettingsPhase19Codes(t *testing.T) {
	codes := settingsPhase19Codes(settingsCardExpiry(time.Now()))

	t.Run("the page keys are registered once, the three list pages are private-only and each has its table", func(t *testing.T) {
		seen := make(map[string]bool)
		for _, pg := range settingsPagesInOrder() {
			if seen[pg.Key] {
				t.Errorf("page key %q is registered twice", pg.Key)
			}
			seen[pg.Key] = true
		}
		want := []string{"s", "l", "c", "f", "n", "k", "b", "i", "w", "g", "r", "e", "m", "d", "t", "u", "o", "y", "a"}
		if got := slices.Sorted(maps.Keys(seen)); !slices.Equal(got, slices.Sorted(slices.Values(want))) {
			t.Errorf("registered pages = %v, want %v", got, slices.Sorted(slices.Values(want)))
		}
		for _, key := range settingsPhase19Pages {
			pg, ok := settingsPageByKey(key)
			if !ok || !pg.PrivateOnly {
				t.Errorf("list page %q registered %v, private-only %v, want both", key, ok, pg.PrivateOnly)
			}
			if _, ok := settingsListByPage(key); !ok {
				t.Errorf("list page %q has no list table", key)
			}
		}
	})

	t.Run("every list action code belongs to one page and is never a core code, and the table here is the registry's", func(t *testing.T) {
		registered := make(map[string]string)
		for _, key := range settingsPhase19Pages {
			pg, _ := settingsPageByKey(key)
			for code := range pg.Actions {
				registered[code] = key
			}
		}
		if len(registered) != len(codes) {
			t.Errorf("the three pages register %d action codes, this test lists %d", len(registered), len(codes))
		}
		for _, row := range codes {
			if registered[row.code] != row.page {
				t.Errorf("code %q is registered by page %q, want %q", row.code, registered[row.code], row.page)
			}
			if slices.Contains(settingsCoreActions, row.code) {
				t.Errorf("list code %q is a core code", row.code)
			}
			if entry, ok := settingsActions[row.code]; !ok || entry.pageKey != row.page {
				t.Errorf("the action table maps %q to %+v, want page %s", row.code, entry, row.page)
			}
		}
		// Not one code collides with another page's: every code of the action table has exactly one page.
		owners := make(map[string][]string)
		for _, pg := range settingsPagesInOrder() {
			for code := range pg.Actions {
				owners[code] = append(owners[code], pg.Key)
			}
		}
		for code, pages := range owners {
			if len(pages) != 1 {
				t.Errorf("action code %q is declared by pages %v, want exactly one", code, pages)
			}
		}
	})

	t.Run("every list action needs the owner field c in step 1 and exactly its own fields", func(t *testing.T) {
		const chatID = "-1001234567890"
		for _, row := range codes {
			owned := map[string]string{"a": row.code, "c": chatID}
			maps.Copy(owned, row.fields)
			if !settingsCheckFields(row.code, owned) {
				t.Errorf("%s with c and %v is refused in step 1, want it accepted", row.code, row.fields)
			}
			group := map[string]string{"a": row.code, "u": "9999999999"}
			maps.Copy(group, row.fields)
			if settingsCheckFields(row.code, group) {
				t.Errorf("%s with u (a group menu) is accepted in step 1, want it refused", row.code)
			}
			both := maps.Clone(owned)
			both["u"] = "9999999999"
			if settingsCheckFields(row.code, both) {
				t.Errorf("%s with both c and u is accepted, want it refused", row.code)
			}
			extra := maps.Clone(owned)
			extra["x"] = "1"
			if settingsCheckFields(row.code, extra) {
				t.Errorf("%s with an extra key is accepted, want it refused", row.code)
			}
			// The action's own Validate also refuses a group menu's fields, before the allow-list is consulted.
			if validate := settingsActions[row.code].action.Validate; validate != nil && validate(group) {
				t.Errorf("%s: Validate accepts the fields of a group menu", row.code)
			}
		}
	})

	t.Run("the part table is exactly the five parts, and tg and st name no control of a list page", func(t *testing.T) {
		type part struct {
			page, code string
			step       int
		}
		var got []part
		for _, p := range settingsListParts {
			got = append(got, part{p.Page, p.Code, p.Step})
		}
		want := []part{{"b", "w", 1}, {"i", "k", 1}, {"i", "y", 2}, {"e", "n", 1}, {"e", "c", 2}}
		if !slices.Equal(got, want) {
			t.Errorf("settingsListParts = %v, want %v", got, want)
		}
		for _, page := range settingsPhase19Pages {
			for _, action := range []string{settingsActToggle, settingsActSet} {
				fields := map[string]string{"p": page, "t": "x", "v": "1"}
				if settingsControlFieldsValid(action, fields) {
					t.Errorf("%s naming page %s passes the control check, want it refused", action, page)
				}
				owned := map[string]string{"a": action, "c": "-1001234567890", "p": page, "t": "x", "v": "1"}
				if settingsCheckFields(action, owned) {
					t.Errorf("%s naming page %s is accepted in step 1, want it refused", action, page)
				}
			}
		}
	})
}

func TestSettingsMainPhase19(t *testing.T) {
	settingsLangWithCodes(t, settingsLangDefaultCodes)
	// The translator is built after each environment installed the marker locale, so it is a function.
	trOf := func() *i18n.Translator { return i18n.MustNewTranslator("en") }
	listTitles := map[string]string{
		settingsPageBlacklist: "settings_page_blacklist", settingsPageFilters: "settings_page_filters", settingsPageNotes: "settings_page_notes",
	}
	// lineOf is the line Main draws for the page of the title key with the given state.
	lineOf := func(t *testing.T, titleKey, state string) string {
		t.Helper()
		for _, pg := range settingsPagesInOrder() {
			if pg.TitleKey == titleKey {
				return settingsText(trOf(), "settings_main_state_line", i18n.TranslationParams{
					"icon": pg.Icon, "page": settingsText(trOf(), titleKey), "state": state,
				})
			}
		}
		t.Fatalf("no page has the title key %s", titleKey)
		return ""
	}
	seed := func(t *testing.T, env *settingsDelEnv, words, filters, notes int) {
		t.Helper()
		settingsListCleanup(t, env.chat.Id)
		settingsListSeedBlacklist(t, env.chat.Id, settingsListWords(words)...)
		settingsListSeedFilters(t, env.chat.Id, settingsListWords(filters)...)
		settingsListSeedNotes(t, env.chat.Id, settingsListWords(notes)...)
	}
	lines := func(t *testing.T, text string) map[string]string {
		t.Helper()
		out := make(map[string]string)
		for _, key := range settingsMainTitleKeys {
			if line, ok := settingsMainStateLine(text, key); ok {
				out[key] = line
			}
		}
		if len(out) != len(settingsMainTitleKeys) {
			t.Fatalf("Main has %d state lines, want %d:\n%s", len(out), len(settingsMainTitleKeys), text)
		}
		return out
	}

	t.Run("a private menu reads each list's fresh count, in grid order, and Main makes no Telegram call of its own", func(t *testing.T) {
		env := newSettingsDelEnv(t)
		seed(t, env, 12, 12, 3)
		m := env.privateMenu()

		if got := settingsMainShape(t, m.mainKeyboard); got != settingsMainShapePrivate {
			t.Errorf("Main = %q, want %q", got, settingsMainShapePrivate)
		}
		if rows := len(m.mainKeyboard); rows != settingsMainOwnerRows {
			t.Errorf("the owner's Main has %d rows, want %d", rows, settingsMainOwnerRows)
		}
		got := lines(t, m.mainText)
		for titleKey, state := range map[string]string{
			"settings_page_blacklist": "12", "settings_page_filters": "12/150", "settings_page_notes": "3",
		} {
			if want := lineOf(t, titleKey, state); got[titleKey] != want {
				t.Errorf("%s line = %q, want %q", titleKey, got[titleKey], want)
			}
		}
		// The three lists sit in the grid's order, each pair on one row with the cells beside them unchanged.
		order := slices.Index(settingsMainTitleKeys, "settings_page_blacklist")
		if settingsMainTitleKeys[order+1] != "settings_page_filters" || slices.Index(settingsMainTitleKeys, "settings_page_notes") != slices.Index(settingsMainTitleKeys, "settings_page_rules")+1 {
			t.Errorf("the title order = %v, want Blacklist then Filters, Notes after Rules", settingsMainTitleKeys)
		}
		// Every earlier page's line is what its own MainState returns.
		for _, pg := range settingsPagesInOrder() {
			if pg.MainState == nil || slices.Contains(settingsPhase19Pages, pg.Key) {
				continue
			}
			state, known := pg.MainState(env.bot, trOf(), &env.chat)
			if !known {
				state = settingsUnknownState
			}
			if want := lineOf(t, pg.TitleKey, state); got[pg.TitleKey] != want {
				t.Errorf("%s line = %q, want %q (its own MainState)", pg.TitleKey, got[pg.TitleKey], want)
			}
		}

		before := settingsPhase18Calls(env.settingsEnv)
		if entries := settingsMainEntries(env.bot, trOf(), &env.chat, true, true); len(entries) != len(settingsMainTitleKeys) {
			t.Fatalf("Main has %d entries, want %d", len(entries), len(settingsMainTitleKeys))
		}
		if delta := settingsPhase18CallsSince(env.settingsEnv, before); len(delta) != 0 {
			t.Errorf("the Main states made Telegram calls %v, want none", delta)
		}
	})

	t.Run("an empty chat reads 0, 0/150 and 0", func(t *testing.T) {
		env := newSettingsDelEnv(t)
		seed(t, env, 0, 0, 0)
		got := lines(t, env.privateMenu().mainText)
		for titleKey, state := range map[string]string{
			"settings_page_blacklist": "0", "settings_page_filters": "0/150", "settings_page_notes": "0",
		} {
			if want := lineOf(t, titleKey, state); got[titleKey] != want {
				t.Errorf("%s line = %q, want %q", titleKey, got[titleKey], want)
			}
		}
	})

	t.Run("a count that fails is the unknown mark on that line alone", func(t *testing.T) {
		env := newSettingsDelEnv(t)
		seed(t, env, 12, 12, 3)
		m := env.privateMenu()
		baseline := lines(t, m.mainText)
		failure := errors.New("database is gone")
		for key, swap := range map[string]func(t *testing.T){
			"settings_page_blacklist": func(t *testing.T) {
				mreqSwap(t, &settingsBlacklistCount, func(context.Context, int64) (int, error) { return 0, failure })
			},
			"settings_page_filters": func(t *testing.T) {
				mreqSwap(t, &settingsFiltersCount, func(context.Context, int64) (int, error) { return 0, failure })
			},
			"settings_page_notes": func(t *testing.T) {
				mreqSwap(t, &settingsNotesCount, func(context.Context, int64) (int, error) { return 0, failure })
			},
		} {
			t.Run(key, func(t *testing.T) {
				swap(t)
				m.press("a", settingsActBack)
				text, _ := m.last()
				for titleKey, line := range lines(t, text) {
					if titleKey == key {
						if !strings.HasSuffix(line, settingsUnknownState) {
							t.Errorf("the failing list's line = %q, want the unknown mark", line)
						}
						continue
					}
					if line != baseline[titleKey] {
						t.Errorf("%s changed to %q from %q while another list's count failed", titleKey, line, baseline[titleKey])
					}
				}
			})
		}
	})

	t.Run("a group menu reads In private for the three lists and calls none of their seams", func(t *testing.T) {
		env := newSettingsDelEnv(t)
		seed(t, env, 12, 12, 3)
		var calls atomic.Int64
		for _, swap := range []func(){
			func() {
				read, count := settingsBlacklistRead, settingsBlacklistCount
				mreqSwap(t, &settingsBlacklistRead, func(ctx context.Context, id int64) ([]settingsListEntry, error) { calls.Add(1); return read(ctx, id) })
				mreqSwap(t, &settingsBlacklistCount, func(ctx context.Context, id int64) (int, error) { calls.Add(1); return count(ctx, id) })
			},
			func() {
				read, count := settingsFiltersRead, settingsFiltersCount
				mreqSwap(t, &settingsFiltersRead, func(ctx context.Context, id int64) ([]settingsListEntry, error) { calls.Add(1); return read(ctx, id) })
				mreqSwap(t, &settingsFiltersCount, func(ctx context.Context, id int64) (int, error) { calls.Add(1); return count(ctx, id) })
			},
			func() {
				read, count := settingsNotesRead, settingsNotesCount
				mreqSwap(t, &settingsNotesRead, func(ctx context.Context, id int64) ([]settingsListEntry, error) { calls.Add(1); return read(ctx, id) })
				mreqSwap(t, &settingsNotesCount, func(ctx context.Context, id int64) (int, error) { calls.Add(1); return count(ctx, id) })
			},
		} {
			swap()
		}
		m := env.groupMenu()
		if got := settingsMainShape(t, m.mainKeyboard); got != settingsMainShapeGroup {
			t.Errorf("Main = %q, want %q", got, settingsMainShapeGroup)
		}
		got := lines(t, m.mainText)
		for _, titleKey := range listTitles {
			if want := lineOf(t, titleKey, settingsText(trOf(), "settings_state_private_only")); got[titleKey] != want {
				t.Errorf("%s line = %q, want %q", titleKey, got[titleKey], want)
			}
		}
		if n := calls.Load(); n != 0 {
			t.Errorf("a group Main called the lists' seams %d times, want none", n)
		}
		// An allowed administrator's Main has no Access row: ten rows, eighteen lines.
		admin := env.groupMenuOf(env.admin)
		if rows := len(admin.mainKeyboard); rows != settingsMainOwnerRows-1 {
			t.Errorf("the administrator's Main has %d rows, want %d", rows, settingsMainOwnerRows-1)
		}
		if n := calls.Load(); n != 0 {
			t.Errorf("an administrator's group Main called the lists' seams %d times, want none", n)
		}
	})
}
