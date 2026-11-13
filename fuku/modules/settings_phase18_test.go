//go:build testtools

package modules

import (
	"context"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/logchannels"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
	"github.com/uasneppy/fuku_robot/fuku/utils/cache"
)

// settingsPhase18Tables are the tables a settings page reads: the nine of the Phase 18 readers and
// of the Main status block, each with a chat_id column. A read of Main or of a page creates a row in
// none of them.
var settingsPhase18Tables = []struct {
	name  string
	model any
}{
	{"warn_settings", &models.WarnSettings{}},
	{"report_chat_settings", &models.ReportChatSettings{}},
	{"ai_spam_settings", &models.AISpamSettings{}},
	{"greetings", &models.GreetingSettings{}},
	{"chats", &models.Chat{}},
	{"admin", &models.AdminSettings{}},
	{"log_channels", &models.LogChannel{}},
	{"locks", &models.LockSettings{}},
	{"rules", &models.RulesSettings{}},
}

// settingsPhase18RowCounts counts the chat's rows in each table of settingsPhase18Tables, by name,
// for the tables that have any.
func settingsPhase18RowCounts(t *testing.T, chatID int64) map[string]int64 {
	t.Helper()
	counts := make(map[string]int64)
	for _, table := range settingsPhase18Tables {
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

// settingsPhase18RowCount is how many rows the chat has in all of settingsPhase18Tables together.
func settingsPhase18RowCount(t *testing.T, chatID int64) int64 {
	t.Helper()
	var total int64
	for _, n := range settingsPhase18RowCounts(t, chatID) {
		total += n
	}
	return total
}

// TestSettingsPhase18InStaffGroup: none of the six pages is Hidden, so in a chat that is a Staff Group
// Main lists them and each opens and writes exactly as in any group (D-11). The Deletion page is the
// one page a Staff Group does not have, which proves the group really is a Staff Group here.
func TestSettingsPhase18InStaffGroup(t *testing.T) {
	settingsLangWithCodes(t, settingsLangDefaultCodes)
	env := newSettingsDelEnv(t)
	editCleanup(t, env.chat.Id)
	settingsWarnsCleanup(t, env.chat.Id)
	settingsSwCleanup(t, env.chat.Id)
	settingsAnonCleanup(t, env.chat.Id)
	lockGoldenCleanup(t, env.chat.Id)
	settingsLangCleanup(t, []int64{env.chat.Id}, nil)
	env.makeStaff()
	settingsLogLink(t, env.chat)

	m := env.groupMenu()
	pages := []string{settingsPageWarns, settingsPageLocks, settingsPageSwitches, settingsPageLang, settingsPageLog, settingsPageAnon}
	for _, key := range pages {
		if _, ok := settingsPageButton(t, m.mainKeyboard, key); !ok {
			t.Errorf("a Staff Group's Main has no cell for page %s", key)
		}
	}
	if _, ok := settingsPageButton(t, m.mainKeyboard, settingsPageDeletion); ok {
		t.Error("the chat is not a Staff Group: its Main still has the Deletion cell")
	}
	// Every page but Deletion, and the Blocked sources page, which exists only in a Staff Group: the same count as any
	// other chat's Main has.
	if got := len(settingsMainLines(m.mainText)); got != len(settingsMainTitleKeys) {
		t.Errorf("a Staff Group's Main has %d state lines, want %d (every page but Deletion, plus Blocked sources)", got, len(settingsMainTitleKeys))
	}
	for _, key := range []string{
		"settings_page_warns", "settings_page_locks", "settings_page_switches",
		"settings_page_lang", "settings_page_log", "settings_page_anon",
	} {
		line, ok := settingsMainStateLine(m.mainText, key)
		if !ok || strings.HasSuffix(line, settingsUnknownState) {
			t.Errorf("Main line of %s = %q (found %v), want a state", key, line, ok)
		}
	}

	// Each page opens, and a permitted press writes through the page's own apply function.
	for _, key := range pages {
		m.press("a", "op", "p", key)
		if text, _ := m.last(); containsMarker(text, "settings_cannot_open") || containsMarker(text, "common_callback_invalid_request") {
			t.Errorf("page %s in a Staff Group drew %q", key, text)
		}
	}
	m.press("a", "st", "p", settingsPageWarns, "t", settingsWarnsLimitKey, "v", "7")
	if row, ok := settingsWarnsRow(t, env.chat.Id); !ok || row.WarnLimit != 7 {
		t.Errorf("warn settings = %+v (stored %v), want limit 7", row, ok)
	}
	m.press("a", "tg", "p", settingsPageSwitches, "t", settingsSwReportsKey, "v", "0")
	if row, ok := settingsSwReportsRow(t, env.chat.Id); !ok || row.Enabled {
		t.Errorf("report settings = %+v (stored %v), want Reports off", row, ok)
	}
	m.press("a", "st", "p", settingsPageLang, "t", settingsLangControlKey, "v", "en")
	if got, ok := settingsLangStored(t, env.chat.Id); !ok || got != "en" {
		t.Errorf("group language = %q (stored %v), want en", got, ok)
	}
	m.press("a", "tg", "p", settingsPageLog, "t", "a", "v", "0")
	if row := logchannels.GetContext(context.Background(), env.chat.Id); row == nil || row.CatAdmin {
		t.Errorf("log channel row = %+v, want the admin category off", row)
	}
	m.press("a", "tg", "p", settingsPageAnon, "t", settingsAnonControl, "v", "1")
	if on, err := settingsAnonRead(context.Background(), env.chat.Id); err != nil || !on {
		t.Errorf("anonymous admins = %v (%v), want on", on, err)
	}
	m.press("a", "tg", "p", settingsPageLocks, "t", settingsLocksCode(t, "gif"), "v", "1")
	if got := settingsLocksStored(t, env.chat.Id); got != "gif=true" {
		t.Errorf("locks stored %q, want gif=true", got)
	}
}

// settingsPhase18PageKeys are the six pages Phase 18 registered, and settingsPhase18Controls the t
// values each declares (the Locks controls come from the table).
var (
	settingsPhase18PageKeys = []string{
		settingsPageWarns, settingsPageLocks, settingsPageSwitches, settingsPageLang, settingsPageLog, settingsPageAnon,
	}
	settingsPhase18Controls = map[string][]string{
		settingsPageWarns:    {settingsWarnsLimitKey, settingsWarnsModeKey},
		settingsPageSwitches: {settingsSwReportsKey, settingsSwAIKey, settingsSwApproveKey},
		settingsPageLang:     {settingsLangControlKey},
		settingsPageLog:      {"s", "a", "u", "m", "r", "o"},
		settingsPageAnon:     {settingsAnonControl},
	}
)

func TestSettingsPhase18Codes(t *testing.T) {
	t.Run("page keys are registered once and collide with no other page", func(t *testing.T) {
		seen := make(map[string]bool)
		for _, pg := range settingsPagesInOrder() {
			if seen[pg.Key] {
				t.Errorf("page key %q is registered twice", pg.Key)
			}
			seen[pg.Key] = true
		}
		for _, key := range settingsPhase18PageKeys {
			if !seen[key] {
				t.Errorf("Phase 18 page %q is not registered", key)
			}
		}
		// The sixteen pages of Phase 18, the Blacklist, Filters and Notes pages of Phase 19 and the Blocked sources page of a
		// Staff Group's forward blocklist ("v"), and no other.
		want := []string{"s", "v", "l", "c", "f", "n", "k", "b", "i", "w", "g", "r", "e", "m", "d", "t", "u", "o", "y", "a"}
		if got := slices.Sorted(maps.Keys(seen)); !slices.Equal(got, slices.Sorted(slices.Values(want))) {
			t.Errorf("registered pages = %v, want %v", got, slices.Sorted(slices.Values(want)))
		}
	})

	t.Run("an action code belongs to one page and is never a core code; kg is the Locks pager", func(t *testing.T) {
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
			if slices.Contains(settingsCoreActions, code) {
				t.Errorf("action code %q of page %v is a core code", code, pages)
			}
			if entry, ok := settingsActions[code]; !ok || entry.pageKey != pages[0] {
				t.Errorf("the action table maps %q to %+v, want page %s", code, entry, pages[0])
			}
		}
		if len(settingsActions) != len(owners) {
			t.Errorf("the action table has %d codes, the pages declare %d", len(settingsActions), len(owners))
		}
		if entry, ok := settingsActions["kg"]; !ok || entry.pageKey != settingsPageLocks || settingsActLocksPage != "kg" {
			t.Errorf("kg maps to %+v, want the Locks page", entry)
		}
		for _, row := range settingsLockRows {
			if row.Code == "kg" || slices.Contains(settingsCoreActions, row.Code) {
				t.Errorf("lock code %q collides with an action code", row.Code)
			}
		}
	})

	t.Run("each page declares exactly its controls, and every lock has its own", func(t *testing.T) {
		for page, want := range settingsPhase18Controls {
			pg, _ := settingsPageByKey(page)
			if got := slices.Sorted(maps.Keys(pg.Controls)); !slices.Equal(got, slices.Sorted(slices.Values(want))) {
				t.Errorf("page %s controls = %v, want %v", page, got, want)
			}
		}
		pg, _ := settingsPageByKey(settingsPageLocks)
		codes := make(map[string]bool)
		for _, row := range settingsLockRows {
			if codes[row.Code] {
				t.Errorf("lock code %q is used by two rows", row.Code)
			}
			codes[row.Code] = true
			if _, ok := pg.Controls[row.Code]; !ok {
				t.Errorf("the Locks page has no control for %q", row.Code)
			}
		}
		if len(pg.Controls) != len(settingsLockRows) || len(settingsLockRows) != 22 {
			t.Errorf("Locks has %d controls for %d rows, want 22 of each", len(pg.Controls), len(settingsLockRows))
		}
	})

	t.Run("each control's allow-list is the contract's", func(t *testing.T) {
		type control struct{ page, t, valid, invalid string }
		var controls []control
		// st only: the warn limit, the warn action and the language.
		stOnly := []control{
			{settingsPageWarns, settingsWarnsLimitKey, "5", "1000"}, {settingsPageWarns, settingsWarnsModeKey, "ban", "BAN"},
			{settingsPageLang, settingsLangControlKey, "en", "EN"},
		}
		for _, c := range stOnly {
			fields := func(v string) map[string]string { return map[string]string{"p": c.page, "t": c.t, "v": v} }
			if !settingsControlFieldsValid(settingsActSet, fields(c.valid)) {
				t.Errorf("st %s/%s v=%q is refused, want it accepted", c.page, c.t, c.valid)
			}
			if settingsControlFieldsValid(settingsActToggle, fields(c.valid)) {
				t.Errorf("tg %s/%s v=%q is accepted, want it refused (st only)", c.page, c.t, c.valid)
			}
			for _, bad := range []string{c.invalid, "", "-1", "x y"} {
				if settingsControlFieldsValid(settingsActSet, fields(bad)) {
					t.Errorf("st %s/%s v=%q is accepted, want it refused", c.page, c.t, bad)
				}
			}
		}
		// tg only, with v exactly 0 or 1: every switch, log category, the Anonymous admins toggle and every lock.
		for page, keys := range settingsPhase18Controls {
			if page == settingsPageWarns || page == settingsPageLang {
				continue
			}
			for _, key := range keys {
				controls = append(controls, control{page, key, "1", "2"})
			}
		}
		for _, row := range settingsLockRows {
			controls = append(controls, control{settingsPageLocks, row.Code, "0", "2"})
		}
		for _, c := range controls {
			fields := func(v string) map[string]string { return map[string]string{"p": c.page, "t": c.t, "v": v} }
			if !settingsControlFieldsValid(settingsActToggle, fields(c.valid)) {
				t.Errorf("tg %s/%s v=%q is refused, want it accepted", c.page, c.t, c.valid)
			}
			if settingsControlFieldsValid(settingsActSet, fields(c.valid)) {
				t.Errorf("st %s/%s v=%q is accepted, want it refused (tg only)", c.page, c.t, c.valid)
			}
			for _, bad := range []string{c.invalid, "", "01", "true"} {
				if settingsControlFieldsValid(settingsActToggle, fields(bad)) {
					t.Errorf("tg %s/%s v=%q is accepted, want it refused", c.page, c.t, bad)
				}
			}
		}
		// The pager takes v alone, one or two digits.
		pager := settingsActions["kg"].action
		if !slices.Equal(pager.Fields, []string{"v"}) {
			t.Errorf("the pager's fields = %v, want [v]", pager.Fields)
		}
		for v, want := range map[string]bool{"1": true, "2": true, "12": true, "": false, "x": false, "123": false, "-1": false} {
			if got := pager.Validate(map[string]string{"v": v}); got != want {
				t.Errorf("kg v=%q accepted = %v, want %v", v, got, want)
			}
		}
	})
}

// settingsPhase18Calls counts the Telegram requests the env's fake has received, by method.
func settingsPhase18Calls(env *settingsEnv) map[string]int {
	env.fake.mu.Lock()
	defer env.fake.mu.Unlock()
	counts := make(map[string]int)
	for _, call := range env.fake.calls {
		counts[call.Method]++
	}
	return counts
}

// settingsPhase18CallsSince is the requests made since before, by method, leaving out the methods
// that made none.
func settingsPhase18CallsSince(env *settingsEnv, before map[string]int) map[string]int {
	after := settingsPhase18Calls(env)
	delta := make(map[string]int)
	for method, n := range after {
		if n != before[method] {
			delta[method] = n - before[method]
		}
	}
	return delta
}

// settingsPhase18Open makes the one press that opens a page (and, for Locks, its second page) as
// the env's administrator, in a group menu or in the administrator's private menu.
func settingsPhase18Open(env *settingsEnv, chat gotgbot.Chat, msgID int64, private bool, kv ...string) {
	env.t.Helper()
	if private {
		env.pressPrivate(env.admin, msgID, env.privateFields(chat.Id, kv...))
		return
	}
	env.pressRaw(chat, env.admin, msgID, encodeCallbackData(settingsCallbackNamespace, env.groupFields(env.admin, kv...)))
}

// settingsPhase18OpenAll opens Main (the Back press) and every Phase 18 page, both Locks pages
// included, in a menu of the chat.
func settingsPhase18OpenAll(env *settingsEnv, chat gotgbot.Chat, msgID int64, private bool) {
	env.t.Helper()
	for _, key := range settingsPhase18PageKeys {
		settingsPhase18Open(env, chat, msgID, private, "a", "op", "p", key)
	}
	settingsPhase18Open(env, chat, msgID, private, "a", settingsActLocksPage, "v", "2")
	settingsPhase18Open(env, chat, msgID, private, "a", "bk")
}

func TestSettingsPhase18ReadsCreateNothing(t *testing.T) {
	settingsLangWithCodes(t, settingsLangDefaultCodes)
	const menuMsgID = 7931
	for _, kind := range []string{"group menu", "private menu"} {
		t.Run(kind, func(t *testing.T) {
			private := kind == "private menu"
			env := newSettingsDelEnv(t)
			// Two fresh chats, neither with a row anywhere: one gets Main and the six pages, the other the
			// Phase 17 Deletion page as the baseline of what the menu's own pipeline writes.
			chats := []gotgbot.Chat{env.addChat(), env.addChat()}
			for _, chat := range chats {
				editCleanup(t, chat.Id)
				settingsWarnsCleanup(t, chat.Id)
				settingsSwCleanup(t, chat.Id)
				settingsAnonCleanup(t, chat.Id)
				lockGoldenCleanup(t, chat.Id)
				settingsLogCleanup(t, chat.Id)
				settingsLangCleanup(t, []int64{chat.Id}, nil)
				env.fake.setBotMember(chat.Id, lockdownBotAdminJSON(false, true, false))
				if before := settingsPhase18RowCounts(t, chat.Id); len(before) != 0 {
					t.Fatalf("the fresh chat %d already has rows: %v", chat.Id, before)
				}
			}
			settingsPhase18OpenAll(env.settingsEnv, chats[0], menuMsgID, private)
			// A second message, because both menus of a private run sit in the same private chat.
			settingsPhase18Open(env.settingsEnv, chats[1], menuMsgID+1, private, "a", "op", "p", settingsPageDeletion)

			got, baseline := settingsPhase18RowCounts(t, chats[0].Id), settingsPhase18RowCounts(t, chats[1].Id)
			if !maps.Equal(got, baseline) {
				t.Errorf("opening Main and the six pages left rows %v, the Phase 17 page left %v", got, baseline)
			}
			if len(got) != 0 {
				t.Errorf("opening Main and the six pages created rows: %v", got)
			}
			// The pages really drew: the last press is Main and it holds the Locks line.
			if text, _ := env.lastEdit(settingsPhase18Target(env.settingsEnv, chats[0], private), menuMsgID); !containsMarker(text, "settings_state_locks_on") {
				t.Errorf("the run's last edit is %q, want Main with the Locks line", text)
			}
		})
	}
}

// settingsPhase18Target is the chat the edits of a menu are made in: the group, or the
// administrator's private chat.
func settingsPhase18Target(env *settingsEnv, chat gotgbot.Chat, private bool) int64 {
	if private {
		return env.admin.Id
	}
	return chat.Id
}

func TestSettingsPhase18Cost(t *testing.T) {
	const menuMsgID = 7932
	settingsLangWithCodes(t, settingsLangDefaultCodes)
	env := newSettingsDelEnv(t)
	settingsWarnsCleanup(t, env.chat.Id)
	settingsSwCleanup(t, env.chat.Id)
	settingsAnonCleanup(t, env.chat.Id)
	lockGoldenCleanup(t, env.chat.Id)
	settingsLogCleanup(t, env.chat.Id)
	settingsLangCleanup(t, []int64{env.chat.Id}, nil)
	// The bot is an administrator by the cached list too, so no draw pays for loading it.
	settingsLocksWarmBotRight(env)
	env.setBot(settingsDelBotJSON("administrator", true))

	t.Run("Main makes no Telegram call of its own", func(t *testing.T) {
		before := settingsPhase18Calls(env.settingsEnv)
		entries := settingsMainEntries(env.bot, i18n.MustNewTranslator("en"), &env.chat, true, true)
		if len(entries) != 19 {
			t.Fatalf("Main has %d entries, want 19", len(entries))
		}
		if delta := settingsPhase18CallsSince(env.settingsEnv, before); len(delta) != 0 {
			t.Errorf("the Main states made Telegram calls %v, want none", delta)
		}
	})

	// opens is what each open of a page costs. In a group menu the pipeline's own calls are the
	// presser's getChatMember, the edit and the answer; in the administrator's private menu it adds the
	// getChat of the group. The Locks page adds the one read of the bot its right line makes.
	opens := func(private bool) map[string]map[string]int {
		pipeline := map[string]int{"getChatMember": 1, "editMessageText": 1, "answerCallbackQuery": 1}
		if private {
			pipeline["getChat"] = 1
		}
		costs := make(map[string]map[string]int)
		for _, key := range settingsPhase18PageKeys {
			costs[key] = maps.Clone(pipeline)
		}
		costs[settingsPageLocks]["getChatMember"]++
		return costs
	}
	for _, private := range []bool{false, true} {
		kind := "group menu"
		if private {
			kind = "private menu"
		}
		t.Run("opening a page in a "+kind+" makes the pipeline's calls, and Locks one read of the bot", func(t *testing.T) {
			for key, want := range opens(private) {
				before := settingsPhase18Calls(env.settingsEnv)
				settingsPhase18Open(env.settingsEnv, env.chat, menuMsgID, private, "a", "op", "p", key)
				if delta := settingsPhase18CallsSince(env.settingsEnv, before); !maps.Equal(delta, want) {
					t.Errorf("opening page %s made %v, want %v", key, delta, want)
				}
			}
		})
	}

	t.Run("the same costs with Redis down", func(t *testing.T) {
		marshal, manager, client := cache.GetCacheState()
		cache.SetCacheState(marshal, manager, nil)
		t.Cleanup(func() { cache.SetCacheState(marshal, manager, client) })
		for key, want := range opens(false) {
			before := settingsPhase18Calls(env.settingsEnv)
			settingsPhase18Open(env.settingsEnv, env.chat, menuMsgID, false, "a", "op", "p", key)
			if delta := settingsPhase18CallsSince(env.settingsEnv, before); !maps.Equal(delta, want) {
				t.Errorf("with Redis down, opening page %s made %v, want %v", key, delta, want)
			}
		}
	})

	t.Run("a press refused in step 1 makes no Telegram call but its answer", func(t *testing.T) {
		for name, kv := range map[string][]string{
			"tg on the warn limit":         {"a", "tg", "p", settingsPageWarns, "t", settingsWarnsLimitKey, "v", "5"},
			"st on a switch":               {"a", "st", "p", settingsPageSwitches, "t", settingsSwReportsKey, "v", "1"},
			"a language that is a number":  {"a", "st", "p", settingsPageLang, "t", settingsLangControlKey, "v", "12"},
			"a log category with v=2":      {"a", "tg", "p", settingsPageLog, "t", "a", "v", "2"},
			"the anonymous admins with st": {"a", "st", "p", settingsPageAnon, "t", settingsAnonControl, "v", "1"},
			"a lock that does not exist":   {"a", "tg", "p", settingsPageLocks, "t", "zz", "v", "1"},
			"a pager page of three digits": {"a", settingsActLocksPage, "v", "123"},
		} {
			before := settingsPhase18Calls(env.settingsEnv)
			settingsPhase18Open(env.settingsEnv, env.chat, menuMsgID, false, kv...)
			if delta, want := settingsPhase18CallsSince(env.settingsEnv, before), (map[string]int{"answerCallbackQuery": 1}); !maps.Equal(delta, want) {
				t.Errorf("%s made %v, want %v", name, delta, want)
			}
		}
		if rows := settingsPhase18RowCount(t, env.chat.Id); rows != 0 {
			t.Errorf("the refused presses left %d rows", rows)
		}
	})
}
