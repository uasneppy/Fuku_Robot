//go:build testtools

package modules

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/uasneppy/fuku_robot/fuku/db"
	dbcache "github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
	"github.com/uasneppy/fuku_robot/fuku/utils/cache"
	"github.com/uasneppy/fuku_robot/fuku/utils/content"
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

	env := newSettingsDelEnv(t)
	seed(t, env, 12, 12, 3)
	m := env.privateMenu()

	t.Run("a private menu reads each list's fresh count, in grid order, and Main makes no Telegram call of its own", func(t *testing.T) {

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
		empty := env.addChat()
		settingsListCleanup(t, empty.Id)
		msgID := env.updateID()
		env.pressPrivate(env.admin, msgID, env.privateFields(empty.Id, "a", settingsActBack))
		text, _ := env.lastEdit(env.admin.Id, msgID)
		for titleKey, state := range map[string]string{
			"settings_page_blacklist": "0", "settings_page_filters": "0/150", "settings_page_notes": "0",
		} {
			line, _ := settingsMainStateLine(text, titleKey)
			if want := lineOf(t, titleKey, state); line != want {
				t.Errorf("%s line = %q, want %q", titleKey, line, want)
			}
		}
	})

	t.Run("a count that fails is the unknown mark on that line alone", func(t *testing.T) {
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

// settingsPhase19Tables are the tables a list page reads or a command of the three lists writes, each with a chat_id column.
var settingsPhase19Tables = []struct {
	name  string
	model any
}{
	{"blacklists", &models.BlacklistSettings{}},
	{"filters", &db.ChatFilters{}},
	{"notes", &models.Notes{}},
	{"notes_settings", &models.NotesSettings{}},
	{"chats", &models.Chat{}},
}

// settingsPhase19RowCounts counts the chat's rows in each table of settingsPhase19Tables that has any, by name.
func settingsPhase19RowCounts(t *testing.T, chatID int64) map[string]int64 {
	t.Helper()
	counts := make(map[string]int64)
	for _, table := range settingsPhase19Tables {
		var n int64
		if err := db.DB.Model(table.model).Where("chat_id = ?", chatID).Count(&n).Error; err != nil {
			t.Fatalf("count %s rows of %d: %v", table.name, chatID, err)
		}
		if n != 0 {
			counts[table.name] = n
		}
	}
	return counts
}

// settingsPhase19CommandKeys are the command cache keys of the three lists' chat: a page's read fills none of them.
func settingsPhase19CommandKeys(chatID int64) []string {
	var keys []string
	for _, name := range []string{"blacklist", "filter_list", "filters_optimized", "notes_list"} {
		keys = append(keys, dbcache.CacheKey(name, chatID))
	}
	return keys
}

func TestSettingsListReadsCreateNothing(t *testing.T) {
	setCachedBotUsername(t, "FukuTestBot")
	env := newSettingsListEnv(t)
	for _, kind := range []string{"group menu", "private menu"} {
		t.Run(kind, func(t *testing.T) {
			private := kind == "private menu"
			chat := env.addChat()
			env.fake.setBotRole(chat.Id, staffRoleAdmin)
			settingsListCleanup(t, chat.Id)
			if before := settingsPhase19RowCounts(t, chat.Id); len(before) != 0 {
				t.Fatalf("the fresh chat %d already has rows: %v", chat.Id, before)
			}
			const msgID = 7941
			open := func(kv ...string) { settingsPhase18Open(env.settingsEnv, chat, msgID, private, kv...) }

			open("a", settingsActBack)
			for _, page := range settingsPhase19Pages {
				open("a", settingsActOpen, "p", page)
			}
			if private {
				// The action screen, a pager past the end, an ask and a Yes naming a row that does not exist, and a pick of an
				// action on an empty list.
				open("a", settingsActBlacklistOpen)
				open("a", settingsActBlacklistSet, "v", "ban")
				for _, ask := range []string{"bx", "ix", "ex"} {
					open("a", ask, "r", "42", "v", "1")
				}
				for _, page := range []string{"bp", "ig", "eg"} {
					open("a", page, "v", "9")
				}
				for _, yes := range []string{"by", "iy", "ey"} {
					open("a", yes, "r", "42", "e", settingsCardExpiry(time.Now()))
				}
				if text, _ := env.lastEdit(env.admin.Id, msgID); !containsMarker(text, "settings_entries_already_removed") {
					t.Errorf("the run's last edit is %q, want the already-removed notice", text)
				}
			} else if text, _ := env.lastEdit(chat.Id, msgID); !containsMarker(text, "settings_entries_group_note") {
				t.Errorf("the run's last edit is %q, want the group note", text)
			}

			if got := settingsPhase19RowCounts(t, chat.Id); len(got) != 0 {
				t.Errorf("opening Main and the list pages created rows: %v", got)
			}
			for _, key := range settingsPhase19CommandKeys(chat.Id) {
				if env.redis.Exists(key) {
					t.Errorf("opening filled the command's cache key %s", key)
				}
			}
		})
	}

	t.Run("a failing read is the cannot-open edit on each page, never an empty list shown as stored", func(t *testing.T) {
		broken := errors.New("read failed")
		mreqSwap(t, &settingsBlacklistRead, func(context.Context, int64) ([]settingsListEntry, error) { return nil, broken })
		mreqSwap(t, &settingsFiltersRead, func(context.Context, int64) ([]settingsListEntry, error) { return nil, broken })
		mreqSwap(t, &settingsNotesRead, func(context.Context, int64) ([]settingsListEntry, error) { return nil, broken })
		for _, page := range settingsPhase19Pages {
			msgID := env.updateID()
			env.pressPrivate(env.admin, msgID, env.privateFields(env.chat.Id, "a", settingsActOpen, "p", page))
			text, keyboard := env.lastEdit(env.admin.Id, msgID)
			if !containsMarker(text, "settings_cannot_open") || len(keyboard) != 0 {
				t.Errorf("page %s with a failing read drew %q with %d rows, want the cannot-open edit with no keyboard", page, text, len(keyboard))
			}
			for _, empty := range []string{"settings_bl_empty", "settings_flt_empty", "settings_notes_empty"} {
				if containsMarker(text, empty) {
					t.Errorf("page %s showed the empty list for a failed read: %q", page, text)
				}
			}
		}
	})
}

func TestSettingsListCost(t *testing.T) {
	setCachedBotUsername(t, "FukuTestBot")
	env := newSettingsListEnv(t)
	chat := env.chat
	owner := settingsFiltersOwner(env, chat)
	settingsListSeedBlacklist(t, chat.Id, settingsListWords(3)...)
	settingsListSeedFilters(t, chat.Id, settingsListWords(3)...)
	settingsListSeedNotes(t, chat.Id, settingsListWords(3)...)
	bReads, bCounts := settingsBlacklistCountSeams(t)
	fReads, fCounts := settingsFiltersCountSeams(t)
	nReads, nCounts := settingsNotesCountSeams(t)
	reads := func() int64 { return bReads.Load() + fReads.Load() + nReads.Load() }
	counts := func() int64 { return bCounts.Load() + fCounts.Load() + nCounts.Load() }
	const msgID = 7942
	private := map[string]int{"getChatMember": 1, "getChat": 1, "editMessageText": 1, "answerCallbackQuery": 1}
	// press is one press of the owner's menu in the private chat and what it cost in Telegram calls.
	press := func(private bool, kv ...string) map[string]int {
		before := settingsPhase18Calls(env.settingsEnv)
		if private {
			env.pressPrivate(owner, msgID, env.privateFields(chat.Id, kv...))
		} else {
			env.pressRaw(chat, owner, msgID, encodeCallbackData(settingsCallbackNamespace, env.groupFields(owner, kv...)))
		}
		return settingsPhase18CallsSince(env.settingsEnv, before)
	}

	t.Run("Main's three lines cost three counts in a private menu, none in a group menu, and no Telegram call", func(t *testing.T) {
		before := settingsPhase18Calls(env.settingsEnv)
		settingsMainEntries(env.bot, i18n.MustNewTranslator("en"), &chat, true, true)
		if got := counts(); got != 3 {
			t.Errorf("a private Main made %d count reads, want 3", got)
		}
		settingsMainEntries(env.bot, i18n.MustNewTranslator("en"), &chat, true, false)
		if got := counts(); got != 3 || reads() != 0 {
			t.Errorf("a group Main raised the count reads to %d (list reads %d), want 3 in all and no list read", got, reads())
		}
		if delta := settingsPhase18CallsSince(env.settingsEnv, before); len(delta) != 0 {
			t.Errorf("the Main states made Telegram calls %v, want none", delta)
		}
	})

	t.Run("a list draw is one list read and the pipeline's calls, a group note no read and one lookup", func(t *testing.T) {
		for _, page := range settingsPhase19Pages {
			def, _ := settingsListByPage(page)
			for name, kv := range map[string][]string{
				"open":  {"a", settingsActOpen, "p", page},
				"pager": {"a", def.PagerCode, "v", "1"},
			} {
				r := reads()
				if delta := press(true, kv...); !maps.Equal(delta, private) {
					t.Errorf("%s of page %s made %v, want %v", name, page, delta, private)
				}
				if got := reads() - r; got != 1 {
					t.Errorf("%s of page %s made %d list reads, want 1", name, page, got)
				}
			}
			r := reads()
			want := map[string]int{"getChatMember": 1, "editMessageText": 1, "answerCallbackQuery": 1}
			if delta := press(false, "a", settingsActOpen, "p", page); !maps.Equal(delta, want) {
				t.Errorf("the group note of page %s made %v, want %v", page, delta, want)
			}
			if got := reads() - r; got != 0 {
				t.Errorf("the group note of page %s made %d list reads, want none", page, got)
			}
		}
	})

	t.Run("a press refused in step 1 costs its answer and nothing else", func(t *testing.T) {
		r := reads() + counts()
		for name, kv := range map[string][]string{
			"a pager with a letter":      {"a", "bp", "v", "x"},
			"a delete ask without v":     {"a", "ix", "r", "5"},
			"a Yes with a zero e":        {"a", "ey", "r", "5", "e", "0"},
			"a Cancel of another part":   {"a", "bc", "t", "k"},
			"an action that is no value": {"a", settingsActBlacklistSet, "v", "Ban"},
		} {
			if delta, want := press(true, kv...), (map[string]int{"answerCallbackQuery": 1}); !maps.Equal(delta, want) {
				t.Errorf("%s made %v, want %v", name, delta, want)
			}
		}
		if got := reads() + counts() - r; got != 0 {
			t.Errorf("refused presses made %d reads, want none", got)
		}
	})

	t.Run("a typed step makes one live lookup of the sender, one of the group and at most one of the bot", func(t *testing.T) {
		for page, say := range map[string]string{settingsPageBlacklist: "gamma", settingsPageFilters: "keyword", settingsPageNotes: "name"} {
			def, _ := settingsListByPage(page)
			m := settingsListOpenMenuPage(env, chat, owner, page)
			add, ok := m.markedButton(def.AddKey)
			if !ok {
				t.Fatalf("page %s has no Add button", page)
			}
			m.tap(add)
			sender, bot, groups := env.memberLookupsOf(chat.Id, owner.Id), env.memberLookupsOf(chat.Id, staffTestBotID), len(env.fake.callsFor("getChat"))
			settingsListDoorSay(env, owner, say)
			if got := env.memberLookupsOf(chat.Id, owner.Id) - sender; got != 1 {
				t.Errorf("a typed message on page %s made %d lookups of the sender, want 1", page, got)
			}
			if got := env.memberLookupsOf(chat.Id, staffTestBotID) - bot; got > 1 {
				t.Errorf("a typed message on page %s made %d lookups of the bot, want at most 1", page, got)
			}
			if got := len(env.fake.callsFor("getChat")) - groups; got != 1 {
				t.Errorf("a typed message on page %s made %d getChat calls, want 1", page, got)
			}
			env.redis.Del(settingsInputKey(owner.Id))
			env.redis.Del(settingsInputDoneKey(owner.Id))
		}
	})

	t.Run("with Redis down the list, the card and the action cost the same and Add is the alert", func(t *testing.T) {
		id := strconv.FormatInt(settingsBlacklistIDOf(t, chat.Id, "w01"), 10)
		draws := map[string][]string{
			"the list":   {"a", settingsActOpen, "p", settingsPageBlacklist},
			"the card":   {"a", "bx", "r", id, "v", "1"},
			"the action": {"a", settingsActBlacklistOpen},
		}
		up := make(map[string]map[string]int)
		for name, kv := range draws {
			up[name] = press(true, kv...)
		}
		restore := cache.DisableRedisForTest()
		defer restore()
		for name, kv := range draws {
			if got := press(true, kv...); !maps.Equal(got, up[name]) {
				t.Errorf("with Redis down %s made %v, with Redis %v", name, got, up[name])
			}
		}
		for _, page := range settingsPhase19Pages {
			def, _ := settingsListByPage(page)
			if got, want := press(true, "a", def.AddCode), (map[string]int{"getChatMember": 1, "getChat": 1, "answerCallbackQuery": 1}); !maps.Equal(got, want) {
				// Filters counts its filters before the wait, a read and no Telegram call, so the calls are the same.
				t.Errorf("Add of page %s without Redis made %v, want %v", page, got, want)
			}
			if text, alert := env.lastAnswer(); !alert || !containsMarker(text, "settings_entries_add_unavailable") {
				t.Errorf("Add of page %s without Redis answered %q (alert %v), want the add-unavailable alert", page, text, alert)
			}
		}
	})
}

func TestSettingsListSavedNoticeFits(t *testing.T) {
	withRealLocales(t)
	title := strings.Repeat("Ж", 64)
	ampersands := strings.Repeat("&", 100)
	// Eight entries of 32 ampersands each (five UTF-16 units once escaped), every one admin-only so a Notes line carries its tag.
	entries := make([]settingsListEntry, 0, 8)
	for i := 0; i < 8; i++ {
		entries = append(entries, settingsListEntry{ID: int64(10 + i), Text: strings.Repeat("&", 32) + string(rune('a'+i)), Action: "ban", AdminOnly: true})
	}
	allCounts := settingsBlacklistCounts{Added: 99999, Already: 99999, TooLong: 99999, TooBroad: 99999, Failed: 99999}

	for _, code := range memberReqScreenLanguages {
		t.Run(code, func(t *testing.T) {
			tr := i18n.MustNewTranslator(code)
			view := settingsBlacklistWorstViews(tr)["private"]
			view.GroupTitle = title
			fits := func(name, text string) {
				t.Helper()
				if units := panelUTF16Len(text); units == 0 || units > 3800 {
					t.Errorf("%s is %d UTF-16 units, want 1..3800", name, units)
				}
			}
			saved := func(name string, def settingsListDef, notice string) {
				t.Helper()
				screen := renderSettingsList(view, def, entries, 1, notice)
				fits(name, screen.Text)
				if screen.Omitted != 0 {
					t.Errorf("%s omitted %d buttons", name, screen.Omitted)
				}
			}
			warning := strings.TrimLeft(settingsText(tr, "notes_private_conflict_warning"), "\n")
			album := settingsText(tr, "settings_edit_album_cut")
			filters, notes := settingsFiltersDef(), settingsNotesDef()
			saved("the Blacklist saved page with five five-digit count lines", settingsBlacklistDef(), settingsBlacklistSavedNotice(tr, allCounts, true))
			saved("the Filters saved page", filters, settingsNoticeOK(settingsListEntryText(tr, filters.TwoStep.SavedKey, filters.EntryParam, ampersands))+"\n"+album)
			saved("the Notes saved page with the conflict warning", notes,
				settingsNoticeOK(settingsListEntryText(tr, notes.TwoStep.SavedKey, notes.EntryParam, ampersands))+"\n"+warning+"\n"+album)

			// The longest refusal of each page, alone and with the line that ends a wait.
			refusals := map[string][]string{
				settingsPageBlacklist: {
					settingsBlacklistRefusalText(tr, allCounts), settingsText(tr, "settings_edit_refuse_need_text"),
					settingsText(tr, "chat_status_bot_restrict_group_error"), settingsText(tr, "settings_no_restrict"),
				},
				settingsPageFilters: {
					settingsFiltersExistsText(tr, ampersands), settingsFiltersCapText(tr), settingsText(tr, "filters_keyword_too_broad"),
					settingsText(tr, "settings_edit_refuse_need_text"), content.TextLengthRefusal(strings.Repeat("x", 5000), db.TEXT, code),
					content.TextLengthRefusal(strings.Repeat("x", 2000), db.PHOTO, code), settingsText(tr, "settings_no_change_info"),
				},
				settingsPageNotes: {
					settingsNotesExistsText(tr, ampersands), settingsText(tr, "notes_name_invalid"), settingsText(tr, "settings_edit_refuse_need_text"),
					content.TextLengthRefusal(strings.Repeat("x", 5000), db.TEXT, code), content.TextLengthRefusal(strings.Repeat("x", 2000), db.PHOTO, code),
					settingsText(tr, "connections_is_user_connected_bot_not_admin"), settingsText(tr, "settings_no_change_info"),
				},
			}
			for page, reasons := range refusals {
				def, _ := settingsListByPage(page)
				first := settingsListFirstPart(page)
				for _, reason := range settingsBlacklistLongest(reasons, 3) {
					fits(fmt.Sprintf("a refusal of page %s", page), renderSettingsInputRefusalCode(view, def.CancelCode, first.Code, reason).Text)
					fits(fmt.Sprintf("the wait-ended reply of page %s", page), reason+"\n\n"+settingsText(tr, "settings_edit_wait_stopped"))
				}
			}
		})
	}
}
