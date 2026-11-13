//go:build testtools

package modules

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db"
	dbcache "github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/locks"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
	"github.com/uasneppy/fuku_robot/fuku/utils/cache"
)

// settingsLocksPagerCode is the action code of the Locks page's pager, which a control code must
// never equal.
const settingsLocksPagerCode = "kg"

// TestSettingsLocksTableMatchesCommand holds settingsLockRows equal to what /lock and /unlock
// accept: a lock added to lockMap or restrMap without a row fails here, and so does a row that
// names no lock, so the page and the commands can never split.
func TestSettingsLocksTableMatchesCommand(t *testing.T) {
	commandKinds := locksModule.getLockMapAsArray()

	tableKinds := make([]string, 0, len(settingsLockRows))
	for _, row := range settingsLockRows {
		tableKinds = append(tableKinds, row.Kind)
	}
	for _, kind := range commandKinds {
		if !slices.Contains(tableKinds, kind) {
			t.Errorf("the command accepts %q and the table has no row for it", kind)
		}
	}
	for _, kind := range tableKinds {
		if !slices.Contains(commandKinds, kind) {
			t.Errorf("the table names %q, which /lock does not accept", kind)
		}
	}
	if len(settingsLockRows) != 22 {
		t.Errorf("table has %d rows, want 22", len(settingsLockRows))
	}
	if len(tableKinds) != len(commandKinds) {
		t.Errorf("table lists %d kinds and the command %d: a kind is repeated", len(tableKinds), len(commandKinds))
	}

	shape := regexp.MustCompile(`^[a-z]{2}$`)
	seen := make(map[string]string, len(settingsLockRows))
	for _, row := range settingsLockRows {
		if !shape.MatchString(row.Code) {
			t.Errorf("row %q has code %q, want two lower-case letters", row.Kind, row.Code)
		}
		if other, dup := seen[row.Code]; dup {
			t.Errorf("code %q is used by %q and %q", row.Code, other, row.Kind)
		}
		seen[row.Code] = row.Kind
		if row.Code == settingsLocksPagerCode {
			t.Errorf("row %q uses the pager's code %q", row.Kind, row.Code)
		}
		if slices.Contains(settingsCoreActions, row.Code) {
			t.Errorf("row %q uses the core action code %q", row.Kind, row.Code)
		}
		if row.Code == row.Kind {
			t.Errorf("row %q carries the lock's own name as its code", row.Kind)
		}
		if want := "settings_lock_" + row.Kind; row.NameKey != want {
			t.Errorf("row %q has name key %q, want %q", row.Kind, row.NameKey, want)
		}
		if row.Page != 1 && row.Page != 2 {
			t.Errorf("row %q is on page %d, want 1 or 2", row.Kind, row.Page)
		}
	}

	// 12 rows on page 1 then 10 on page 2, in the UI-SPEC's order, with "all" last.
	var page1, page2 []string
	for i, row := range settingsLockRows {
		switch row.Page {
		case 1:
			page1 = append(page1, row.Code)
			if i >= settingsLockPageSize {
				t.Errorf("row %q on page 1 comes after a page 2 row", row.Kind)
			}
		case 2:
			page2 = append(page2, row.Code)
		}
	}
	if want := []string{"sk", "gf", "ph", "vd", "vn", "vo", "au", "dc", "ct", "lc", "gm", "ur"}; !slices.Equal(page1, want) {
		t.Errorf("page 1 = %v, want %v", page1, want)
	}
	if want := []string{"fw", "rt", "ac", "bt", "ms", "cm", "md", "ot", "pv", "al"}; !slices.Equal(page2, want) {
		t.Errorf("page 2 = %v, want %v", page2, want)
	}
	if len(page1) != settingsLockPageSize {
		t.Errorf("page 1 holds %d rows, want the page size %d", len(page1), settingsLockPageSize)
	}
	if last := settingsLockRows[len(settingsLockRows)-1]; last.Kind != "all" {
		t.Errorf("the last row is %q, want all", last.Kind)
	}
}

// TestSettingsLocksKeysExist is the guard of every locale key the Locks page reads through a table
// field or a table of the phase: make check-translations only sees keys that are literal GetString
// arguments, so a lock name, a page title or a log category name missing from one language would
// ship an empty button. Every key must translate to a non-empty string in all 8 languages.
func TestSettingsLocksKeysExist(t *testing.T) {
	withRealLocales(t)

	keys := []string{"settings_locks_state_on", "settings_locks_state_off", "settings_locks_intro"}
	for _, row := range settingsLockRows {
		keys = append(keys, row.NameKey)
	}
	// The six page titles of Phase 18: Warnings, Locks, Switches, Language, Log channel and
	// Anonymous admins.
	keys = append(keys,
		"settings_page_warns", "settings_page_locks", "settings_page_switches",
		"settings_page_lang", "settings_page_log", "settings_page_anon",
	)
	for _, cat := range settingsLogCategories {
		keys = append(keys, cat.NameKey)
	}

	for _, code := range memberReqScreenLanguages {
		tr := i18n.MustNewTranslator(code)
		for _, key := range keys {
			text, err := tr.GetString(key)
			if err != nil || strings.TrimSpace(text) == "" {
				t.Errorf("%s: key %q does not translate: %q, %v", code, key, text, err)
			}
		}
	}
}

// newSettingsLocksEnv is the Deletion page's environment (the owner who opens the menu, the bot as an
// administrator with the delete right, a scriptable record of the bot) plus the lock commands and
// watchers, so a parity test can run /lock and /unlock next to the page. The env's chat has no lock
// rows.
func newSettingsLocksEnv(t *testing.T) *settingsDelEnv {
	t.Helper()
	env := newSettingsDelEnv(t)
	LoadLocks(env.dispatcher)
	lockGoldenCleanup(t, env.chat.Id)
	return env
}

// settingsLocksMenus runs a case in a group menu and in a private menu of the Locks environment, each
// in its own environment.
func settingsLocksMenus(t *testing.T, run func(t *testing.T, m *settingsDelMenu)) {
	t.Helper()
	t.Run("group menu", func(t *testing.T) {
		run(t, newSettingsLocksEnv(t).groupMenu())
	})
	t.Run("private menu", func(t *testing.T) {
		run(t, newSettingsLocksEnv(t).privateMenu())
	})
}

// settingsLocksPress hand-builds a press of the Locks page on message msgID of chat, as the env's
// administrator in a group menu.
func settingsLocksPress(t *testing.T, env *settingsDelEnv, chat gotgbot.Chat, msgID int64, kv ...string) {
	t.Helper()
	data := encodeCallbackData(settingsCallbackNamespace, env.groupFields(env.admin, kv...))
	if data == "" {
		t.Fatalf("hand-built press %v did not encode", kv)
	}
	env.pressRaw(chat, env.admin, msgID, data)
}

// settingsLocksToggle is the tg press of one control of the page with the absolute value v.
func settingsLocksToggle(t *testing.T, env *settingsDelEnv, chat gotgbot.Chat, msgID int64, code, v string) {
	t.Helper()
	settingsLocksPress(t, env, chat, msgID, "a", "tg", "p", settingsPageLocks, "t", code, "v", v)
}

// settingsLocksCode is the control code of a lock kind.
func settingsLocksCode(t *testing.T, kind string) string {
	t.Helper()
	for _, row := range settingsLockRows {
		if row.Kind == kind {
			return row.Code
		}
	}
	t.Fatalf("the table has no row for %q", kind)
	return ""
}

// settingsLocksCell is the toggle button of a lock kind on a keyboard.
func settingsLocksCell(t *testing.T, keyboard [][]gotgbot.InlineKeyboardButton, kind string) gotgbot.InlineKeyboardButton {
	t.Helper()
	code := settingsLocksCode(t, kind)
	for _, button := range settingsAllButtons(keyboard) {
		if fields := settingsButtonFields(t, button); fields["a"] == "tg" && fields["p"] == settingsPageLocks && fields["t"] == code {
			return button
		}
	}
	t.Fatalf("the page has no cell for %s: %+v", kind, keyboard)
	return gotgbot.InlineKeyboardButton{}
}

// settingsLocksStored is the chat's stored rows read straight from the table.
func settingsLocksStored(t *testing.T, chatID int64) string {
	t.Helper()
	return lockGoldenStored(t, chatID)
}

// settingsLocksSeed stores the kinds locked through the real apply function.
func settingsLocksSeed(t *testing.T, chatID int64, kinds ...string) {
	t.Helper()
	for _, kind := range kinds {
		if res := applyLock(context.Background(), chatID, kind, true); res.Outcome != lockApplied {
			t.Fatalf("seed %s: %+v", kind, res)
		}
	}
}

// settingsLocksWantLines fails unless the page text has the state line of every row of the page, in
// table order, locked when the row's kind is in locked and unlocked otherwise.
func settingsLocksWantLines(t *testing.T, text string, page int, locked ...string) {
	t.Helper()
	at := -1
	for _, row := range settingsLocksRowsOf(page) {
		icon, word := "🔓", staffMarker("settings_locks_state_off")
		if slices.Contains(locked, row.Kind) {
			icon, word = "🔒", staffMarker("settings_locks_state_on")
		}
		settingsDelWantLine(t, text, row.NameKey, icon, word)
		line, _ := settingsMainStateLine(text, row.NameKey)
		next := strings.Index(text, line)
		if next <= at {
			t.Errorf("the line of %s is out of table order in %q", row.Kind, text)
		}
		at = next
	}
}

func TestSettingsLocksOpens(t *testing.T) {
	settingsLocksMenus(t, func(t *testing.T, m *settingsDelMenu) {
		chatID := m.env.chat.Id
		// A chat with no rows reads every lock Unlocked, as the enforcement does.
		line, found := settingsMainStateLine(m.mainText, "settings_page_locks")
		if !found || !strings.Contains(line, "🔐") || !strings.Contains(line, staffMarker("settings_state_locks_on")) {
			t.Errorf("Main status line = %q (found %v), want 🔐 and the {n} locked state", line, found)
		}
		button, ok := settingsPageButton(t, m.mainKeyboard, settingsPageLocks)
		if !ok || !strings.HasPrefix(button.Text, "🔐 ") || !containsMarker(button.Text, "settings_page_locks") {
			t.Fatalf("Main has no 🔐 Locks button: %+v", m.mainKeyboard)
		}
		m.pressButton(button)
		text, keyboard := m.last()

		if !containsMarker(text, "settings_locks_intro") || !containsMarker(text, "staff_panel_page") {
			t.Errorf("page 1 = %q, want the intro and the page line", text)
		}
		settingsLocksWantLines(t, text, 1)
		if strings.Contains(text, staffMarker("settings_lock_forward")) {
			t.Errorf("page 1 draws a lock of page 2: %q", text)
		}
		if containsMarker(text, "settings_del_no_right") || containsMarker(text, "settings_del_right_unknown") {
			t.Errorf("the bot can delete and the page drew a right line: %q", text)
		}
		if got, want := fmt.Sprint(settingsDelWidths(keyboard)), "[2 2 2 2 2 2 1 2]"; got != want {
			t.Errorf("page 1 row widths = %s, want %s", got, want)
		}
		if next, ok := settingsButtonByMarker(keyboard, "staff_panel_next"); !ok || settingsButtonFields(t, next)["v"] != "2" {
			t.Errorf("page 1 has no Next to page 2: %+v", keyboard)
		}
		if _, ok := settingsButtonByMarker(keyboard, "staff_panel_prev"); ok {
			t.Error("page 1 draws Prev")
		}

		// A stored lock reads Locked on the page and in the Main line's count.
		settingsLocksSeed(t, chatID, "gif", "forward")
		m.press("a", "op", "p", settingsPageLocks)
		text, _ = m.last()
		settingsLocksWantLines(t, text, 1, "gif")
		m.press("a", "bk")
		mainText, _ := m.last()
		if line, found := settingsMainStateLine(mainText, "settings_page_locks"); !found || strings.HasSuffix(line, settingsUnknownState) {
			t.Errorf("Main line after two locks = %q (found %v), want a state", line, found)
		}
	})
}

func TestSettingsParityLock(t *testing.T) {
	const menuMsgID = 7313

	// run changes the same lock two ways, /lock or /unlock on one chat and presses of the page on
	// another, and requires the same stored rows, the same cache and the same /locks listing.
	run := func(t *testing.T, kind string, seed bool, on bool, taps int) {
		t.Helper()
		env := newSettingsLocksEnv(t)
		commandChat := env.chat
		menuChat := env.addChat()
		lockGoldenCleanup(t, menuChat.Id)
		if seed {
			for _, id := range []int64{commandChat.Id, menuChat.Id} {
				settingsLocksSeed(t, id, kind)
			}
			if on {
				for _, id := range []int64{commandChat.Id, menuChat.Id} {
					if err := locks.UpdateLock(id, kind, false); err != nil {
						t.Fatalf("seed unlocked: %v", err)
					}
				}
			}
		}
		// Prime the cache with the old state: the write must drop it.
		for _, id := range []int64{commandChat.Id, menuChat.Id} {
			if _, err := settingsLocksRead(context.Background(), id); err != nil {
				t.Fatalf("prime: %v", err)
			}
		}
		verb, value := "/unlock ", "0"
		if on {
			verb, value = "/lock ", "1"
		}
		env.sendIn(commandChat, env.admin, verb+kind)
		for range taps {
			settingsLocksToggle(t, env, menuChat, menuMsgID, settingsLocksCode(t, kind), value)
		}

		commandRows, menuRows := settingsLocksStored(t, commandChat.Id), settingsLocksStored(t, menuChat.Id)
		if want := fmt.Sprintf("%s=%t", kind, on); commandRows != want {
			t.Errorf("%s%s stored %q, want %q", verb, kind, commandRows, want)
		}
		if menuRows != commandRows {
			t.Errorf("the page stored %q, the command %q, want the same rows", menuRows, commandRows)
		}
		for _, id := range []int64{commandChat.Id, menuChat.Id} {
			state, err := settingsLocksRead(context.Background(), id)
			if err != nil || state[kind] != on {
				t.Errorf("chat %d reads %v (%v) through the cache, want %s=%t", id, state, err, kind, on)
			}
		}
		// /locks then lists the stored value on both.
		listing := fmt.Sprintf("%s = %t", kind, on)
		for _, chat := range []gotgbot.Chat{commandChat, menuChat} {
			before := len(env.fake.sentTo(chat.Id))
			env.sendIn(chat, env.admin, "/locks")
			sent := env.fake.sentTo(chat.Id)
			if len(sent) != before+1 || !strings.Contains(fmt.Sprint(sent[len(sent)-1].Params["text"]), listing) {
				t.Errorf("/locks in chat %d answered %+v, want a line %q", chat.Id, sent[before:], listing)
			}
		}
		// The page redraws the page the lock is on with the stored value, and no notice.
		text, keyboard := env.lastEdit(menuChat.Id, menuMsgID)
		var locked []string
		if on {
			locked = []string{kind}
		}
		for _, row := range settingsLockRows {
			if row.Kind != kind {
				continue
			}
			settingsLocksWantLines(t, text, row.Page, locked...)
			if got := settingsLocksCell(t, keyboard, kind); !strings.HasPrefix(got.Text, settingsLockIcon(on)+" ") {
				t.Errorf("the cell of %s reads %q, want the %s icon", kind, got.Text, settingsLockIcon(on))
			}
		}
		if strings.Contains(text, "⚠️") {
			t.Errorf("the press drew a notice: %q", text)
		}
	}

	t.Run("lock gif on a chat with no row", func(t *testing.T) { run(t, "gif", false, true, 1) })
	t.Run("unlock gif on a chat with no row", func(t *testing.T) { run(t, "gif", false, false, 1) })
	t.Run("lock gif over an unlocked row", func(t *testing.T) { run(t, "gif", true, true, 1) })
	t.Run("unlock gif over a locked row", func(t *testing.T) { run(t, "gif", true, false, 1) })
	t.Run("a double tap writes the same row", func(t *testing.T) { run(t, "gif", false, true, 2) })
	t.Run("a restriction of page 2", func(t *testing.T) { run(t, "all", false, true, 1) })

	t.Run("a bot that is not an admin is refused like the command", func(t *testing.T) {
		env := newSettingsLocksEnv(t)
		commandChat, menuChat := env.chat, env.addChat()
		lockGoldenCleanup(t, menuChat.Id)
		for _, id := range []int64{commandChat.Id, menuChat.Id} {
			env.fake.setBotRole(id, staffRoleMember)
		}
		refusal := staffMarker("connections_is_user_connected_bot_not_admin")

		env.sendIn(commandChat, env.admin, "/lock gif")
		if !settingsAntifloodSentHas(env.settingsEnv, commandChat.Id, refusal) {
			t.Fatal("/lock gif was not refused for a bot that is not an admin")
		}
		answers := env.answerCount()
		settingsLocksToggle(t, env, menuChat, menuMsgID, settingsLocksCode(t, "gif"), "1")
		text, _ := env.lastEdit(menuChat.Id, menuMsgID)
		if !strings.Contains(text, refusal) {
			t.Errorf("the press drew %q, want the notice %s", text, refusal)
		}
		if got := env.answerCount() - answers; got != 1 {
			t.Errorf("the press answered %d times, want once", got)
		}
		for _, id := range []int64{commandChat.Id, menuChat.Id} {
			if got := settingsLocksStored(t, id); got != "" {
				t.Errorf("chat %d stored %q after a refusal, want nothing", id, got)
			}
		}
	})
}

// settingsLocksWarmBotRight lists the bot as an administrator with the delete right and loads the
// chat's admin cache while the bot reads so, so the right of a lock control (the bot as an
// administrator, by the commands' cached predicate) is answered from the cache. A cold cache loads
// through a getChatMember of the bot of its own, which a test that counts the right line's one read
// must not see. The test sets the bot's record for the case it runs after this.
func settingsLocksWarmBotRight(env *settingsDelEnv) {
	env.listAdmin(env.chat.Id, staffTestBotID)
	env.setBot(settingsDelBotJSON("administrator", true))
	cache.InvalidateAdminCache(env.chat.Id)
	cache.LoadAdminCache(env.bot, env.chat.Id)
}

// settingsLocksPageOf is the page number a draw of the Locks page says it is, read from its text.
func settingsLocksPageOf(t *testing.T, text string) int {
	t.Helper()
	for page := 1; page <= settingsLocksPages; page++ {
		rows := settingsLocksRowsOf(page)
		if len(rows) > 0 && strings.Contains(text, staffMarker(rows[0].NameKey)) {
			return page
		}
	}
	t.Fatalf("the text shows no lock of any page: %q", text)
	return 0
}

func TestSettingsLocksPager(t *testing.T) {
	settingsLocksMenus(t, func(t *testing.T, m *settingsDelMenu) {
		turn := func(kv ...string) (string, [][]gotgbot.InlineKeyboardButton) {
			t.Helper()
			m.press(kv...)
			return m.last()
		}
		wantPage := func(name string, text string, keyboard [][]gotgbot.InlineKeyboardButton, page int, widths string, locked ...string) {
			t.Helper()
			if got := settingsLocksPageOf(t, text); got != page {
				t.Errorf("%s drew page %d, want %d: %q", name, got, page, text)
			}
			settingsLocksWantLines(t, text, page, locked...)
			for other := 1; other <= settingsLocksPages; other++ {
				if other == page {
					continue
				}
				for _, row := range settingsLocksRowsOf(other) {
					if strings.Contains(text, staffMarker(row.NameKey)) {
						t.Errorf("%s draws %s of page %d", name, row.Kind, other)
					}
				}
			}
			if got := fmt.Sprint(settingsDelWidths(keyboard)); got != widths {
				t.Errorf("%s row widths = %s, want %s", name, got, widths)
			}
			next, hasNext := settingsButtonByMarker(keyboard, "staff_panel_next")
			prev, hasPrev := settingsButtonByMarker(keyboard, "staff_panel_prev")
			if wantNext := page < settingsLocksPages; hasNext != wantNext || hasPrev != (page > 1) {
				t.Errorf("%s pager: Next %v, Prev %v on page %d of %d", name, hasNext, hasPrev, page, settingsLocksPages)
			}
			if hasNext {
				if fields := settingsButtonFields(t, next); fields["a"] != settingsActLocksPage || fields["v"] != fmt.Sprint(page+1) {
					t.Errorf("%s Next sends %v, want the absolute page %d", name, fields, page+1)
				}
			}
			if hasPrev {
				if fields := settingsButtonFields(t, prev); fields["a"] != settingsActLocksPage || fields["v"] != fmt.Sprint(page-1) {
					t.Errorf("%s Prev sends %v, want the absolute page %d", name, fields, page-1)
				}
			}
			if !containsMarker(text, "staff_panel_page") {
				t.Errorf("%s has no page line: %q", name, text)
			}
		}
		const page1, page2 = "[2 2 2 2 2 2 1 2]", "[2 2 2 2 2 1 2]"

		text, keyboard := turn("a", "op", "p", settingsPageLocks)
		wantPage("open", text, keyboard, 1, page1)
		text, keyboard = turn("a", settingsActLocksPage, "v", "2")
		wantPage("v=2", text, keyboard, 2, page2)
		if n := len(settingsLocksRowsOf(2)); n != 10 {
			t.Errorf("page 2 holds %d locks, want 10", n)
		}
		text, keyboard = turn("a", settingsActLocksPage, "v", "1")
		wantPage("v=1", text, keyboard, 1, page1)
		// A page past the end is the last page, and 0 is the first.
		text, keyboard = turn("a", settingsActLocksPage, "v", "9")
		wantPage("v=9", text, keyboard, 2, page2)
		text, keyboard = turn("a", settingsActLocksPage, "v", "0")
		wantPage("v=0", text, keyboard, 1, page1)

		// A press of a lock of page 2 redraws page 2, with the state it stored.
		turn("a", settingsActLocksPage, "v", "2")
		text, keyboard = turn("a", "tg", "p", settingsPageLocks, "t", settingsLocksCode(t, "bots"), "v", "1")
		wantPage("a toggle of page 2", text, keyboard, 2, page2, "bots")
		// Refresh opens page 1; the stored lock is still on page 2.
		text, keyboard = turn("a", "op", "p", settingsPageLocks)
		wantPage("refresh", text, keyboard, 1, page1)
		text, keyboard = turn("a", settingsActLocksPage, "v", "2")
		wantPage("page 2 again", text, keyboard, 2, page2, "bots")
	})
}

func TestSettingsLocksRightLine(t *testing.T) {
	noRight, unknown := staffMarker("settings_del_no_right"), staffMarker("settings_del_right_unknown")
	cases := []struct {
		name string
		raw  string
		err  error
		want string
	}{
		{name: "the bot can delete", raw: settingsDelBotJSON("administrator", true), want: "none"},
		{name: "a member bot", raw: settingsDelBotJSON("member", false), want: "cannot"},
		{name: "an administrator without the right", raw: settingsDelBotJSON("administrator", false), want: "cannot"},
		{name: "a lookup that fails", err: staffFakeError(500, "Internal Server Error"), want: "unknown"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			settingsLocksMenus(t, func(t *testing.T, m *settingsDelMenu) {
				env := m.env
				settingsLocksWarmBotRight(env)
				if tc.raw != "" {
					env.setBot(tc.raw)
				}
				reads := env.scriptBot(false, tc.err)
				draws := []struct {
					name string
					kv   []string
				}{
					{"open", []string{"a", "op", "p", settingsPageLocks}},
					{"a toggle", []string{"a", "tg", "p", settingsPageLocks, "t", settingsLocksCode(t, "gif"), "v", "1"}},
					{"a page turn", []string{"a", settingsActLocksPage, "v", "2"}},
					{"a toggle of page 2", []string{"a", "tg", "p", settingsPageLocks, "t", settingsLocksCode(t, "bots"), "v", "1"}},
				}
				for _, draw := range draws {
					answers, before := env.answerCount(), len(*reads)
					m.press(draw.kv...)
					text, keyboard := m.last()

					wantNo, wantUnknown := 0, 0
					switch tc.want {
					case "cannot":
						wantNo = 1
					case "unknown":
						wantUnknown = 1
					}
					if got := strings.Count(text, noRight); got != wantNo {
						t.Errorf("%s: %d no-right lines, want %d: %q", draw.name, got, wantNo, text)
					}
					if got := strings.Count(text, unknown); got != wantUnknown {
						t.Errorf("%s: %d unknown lines, want %d: %q", draw.name, got, wantUnknown, text)
					}
					if wantNo == 1 && !strings.Contains(text, "🚫 "+noRight) || wantUnknown == 1 && !strings.Contains(text, "❔ "+unknown) {
						t.Errorf("%s: the right line has no icon prefix: %q", draw.name, text)
					}
					// The slot follows the page line and nothing is disabled or dropped by the line.
					if wantNo+wantUnknown == 1 {
						line := strings.Index(text, staffMarker("staff_panel_page"))
						at := max(strings.Index(text, noRight), strings.Index(text, unknown))
						if line < 0 || at < line {
							t.Errorf("%s: the right line is not after the page line: %q", draw.name, text)
						}
					}
					if containsMarker(text, "settings_cannot_open") || len(keyboard) == 0 {
						t.Errorf("%s drew the cannot-open edit: %q", draw.name, text)
					}
					for _, button := range settingsAllButtons(keyboard) {
						if button.CallbackData == "" {
							t.Errorf("%s: a dead button %q", draw.name, button.Text)
						}
					}
					if got := env.answerCount() - answers; got != 1 {
						t.Errorf("%s answered %d times, want once", draw.name, got)
					}
					if got := len(*reads) - before; got != 1 {
						t.Errorf("%s made %d reads of the bot, want exactly one", draw.name, got)
					} else if at := (*reads)[before]; at != answers+1 {
						t.Errorf("%s read the bot with %d answers made, want it answered first (%d)", draw.name, at, answers+1)
					}
				}
				// The locks were stored with the line showing: nothing is refused for it.
				if got := settingsLocksStored(t, env.chat.Id); got != "bots=true,gif=true" {
					t.Errorf("with the line showing the page stored %q, want bots=true,gif=true", got)
				}
			})
		})
	}

	// A refusal that is a notice (the bot is not an administrator by the list) redraws the page with the
	// warning, like every page's, and the draw states the right too; nothing is written. The read count
	// is not asserted here: the pipeline's own bot check of a cold admin cache reads the bot as well.
	t.Run("a notice refusal redraws with the right line", func(t *testing.T) {
		env := newSettingsLocksEnv(t)
		env.fake.setBotRole(env.chat.Id, staffRoleMember)
		env.setBot(settingsDelBotJSON("member", false))
		cache.InvalidateAdminCache(env.chat.Id)
		cache.LoadAdminCache(env.bot, env.chat.Id)
		m := env.groupMenu()
		m.press("a", "tg", "p", settingsPageLocks, "t", settingsLocksCode(t, "gif"), "v", "1")
		text, _ := m.last()
		if !containsMarker(text, "connections_is_user_connected_bot_not_admin") || !containsMarker(text, "settings_del_no_right") {
			t.Errorf("the refusal drew %q, want the notice and the right line", text)
		}
		if got := settingsLocksStored(t, env.chat.Id); got != "" {
			t.Errorf("a refused press stored %q", got)
		}
	})

	t.Run("a refused press makes no read", func(t *testing.T) {
		env := newSettingsLocksEnv(t)
		settingsLocksWarmBotRight(env)
		env.setBot(settingsDelBotJSON("member", false))
		reads := env.scriptBot(false, nil)
		gif := settingsLocksCode(t, "gif")
		forged := env.groupFields(env.admin, "a", "st", "p", settingsPageLocks, "t", gif, "v", "1")
		env.pressRaw(env.chat, env.admin, 7414, encodeCallbackData(settingsCallbackNamespace, forged))

		member := gotgbot.User{Id: uniqueLinkOwnerID() + 46, FirstName: "Plain"}
		t.Cleanup(func() {
			db.DB.Where("user_id IN ?", []int64{member.Id}).Delete(&models.User{})
			dbcache.DeleteCache(dbcache.CacheKey("user", member.Id))
			dbcache.ResetLocalForTest()
		})
		env.fake.setMember(env.chat.Id, member.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})
		env.pressPrivate(member, 7415, env.privateFields(env.chat.Id, "a", "tg", "p", settingsPageLocks, "t", gif, "v", "1"))
		if answer, alert := env.lastAnswer(); !alert || !strings.Contains(answer, staffMarker("settings_not_admin")) {
			t.Errorf("a plain member's press answered %q (alert %v), want settings_not_admin", answer, alert)
		}
		if len(*reads) != 0 {
			t.Errorf("refused presses made %d reads of the bot, want none", len(*reads))
		}
		if got := settingsLocksStored(t, env.chat.Id); got != "" {
			t.Errorf("refused presses stored %q", got)
		}
	})
}

func TestSettingsLocksForged(t *testing.T) {
	const menuMsgID = 7515
	type forgedCase struct {
		name   string
		fields []string
		extra  map[string]string
		both   bool
	}
	code := settingsLocksCode(t, "gif")
	tg := func(control, v string) []string {
		return []string{"a", "tg", "p", settingsPageLocks, "t", control, "v", v}
	}
	forged := []forgedCase{
		{name: "st on a lock", fields: []string{"a", "st", "p", settingsPageLocks, "t", code, "v", "1"}},
		{name: "v=2", fields: tg(code, "2")},
		{name: "an empty v", fields: tg(code, "")},
		{name: "an unknown code", fields: tg("zz", "1")},
		{name: "the kind as the control", fields: tg("gif", "1")},
		{name: "the pager as a control", fields: tg(settingsActLocksPage, "1")},
		{name: "a lock with an extra key", fields: tg(code, "1"), extra: map[string]string{"x": "1"}},
		{name: "c and u together on a lock", fields: tg(code, "1"), both: true},
		{name: "kg with v=x", fields: []string{"a", settingsActLocksPage, "v", "x"}},
		{name: "kg with a signed v", fields: []string{"a", settingsActLocksPage, "v", "-1"}},
		{name: "kg with three digits", fields: []string{"a", settingsActLocksPage, "v", "100"}},
		{name: "kg with no v", fields: []string{"a", settingsActLocksPage}},
		{name: "kg with a second key", fields: []string{"a", settingsActLocksPage, "v", "1"}, extra: map[string]string{"t": code}},
		{name: "kg with c and u together", fields: []string{"a", settingsActLocksPage, "v", "1"}, both: true},
	}
	for _, tc := range forged {
		t.Run(tc.name, func(t *testing.T) {
			env := newSettingsLocksEnv(t)
			reads := env.scriptBot(false, nil)
			fields := env.groupFields(env.admin, tc.fields...)
			for k, v := range tc.extra {
				fields[k] = v
			}
			if tc.both {
				fields["c"] = "-1001"
			}
			data := encodeCallbackData(settingsCallbackNamespace, fields)
			if data == "" {
				t.Fatalf("forged press %v did not encode", fields)
			}
			before := settingsMenuMarkOf(env.settingsEnv)
			env.pressRaw(env.chat, env.admin, menuMsgID, data)
			settingsMenuWantRefused(t, env.settingsEnv, before, "common_callback_invalid_request", true)
			if len(*reads) != 0 {
				t.Errorf("a forged press read the bot %d times, want none", len(*reads))
			}
			if got := settingsLocksStored(t, env.chat.Id); got != "" {
				t.Errorf("a forged press stored %q", got)
			}
		})
	}
}

func TestSettingsLocksFailedWrite(t *testing.T) {
	for _, tc := range []struct {
		name, kind, value, key string
		seed                   bool
		wantStored             string
	}{
		{name: "lock", kind: "bots", value: "1", key: "locks_lock_failed", wantStored: ""},
		{name: "unlock", kind: "bots", value: "0", key: "locks_unlock_failed", seed: true, wantStored: "bots=true"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			settingsLocksMenus(t, func(t *testing.T, m *settingsDelMenu) {
				env := m.env
				if tc.seed {
					settingsLocksSeed(t, env.chat.Id, tc.kind)
				}
				mreqSwap(t, &lockWrites.Update, func(int64, string, bool) error { return errors.New("disk is full") })
				m.press("a", settingsActLocksPage, "v", "2")
				answers := env.answerCount()
				m.press("a", "tg", "p", settingsPageLocks, "t", settingsLocksCode(t, tc.kind), "v", tc.value)
				if got := env.answerCount() - answers; got != 1 {
					t.Errorf("the press answered %d times, want once", got)
				}
				text, keyboard := m.last()
				if !strings.Contains(text, "⚠️ "+staffMarker(tc.key)) || !strings.Contains(text, staffMarker("settings_lock_bots")) {
					t.Errorf("the page drew %q, want the notice %s naming the lock", text, tc.key)
				}
				var locked []string
				if tc.seed {
					locked = []string{tc.kind}
				}
				// The page the lock is on is redrawn with the stored state.
				if settingsLocksPageOf(t, text) != 2 {
					t.Errorf("a failed press redrew the wrong page: %q", text)
				}
				settingsLocksWantLines(t, text, 2, locked...)
				if cell := settingsLocksCell(t, keyboard, tc.kind); !strings.HasPrefix(cell.Text, settingsLockIcon(tc.seed)+" ") {
					t.Errorf("the cell reads %q, want the stored state", cell.Text)
				}
				if got := settingsLocksStored(t, env.chat.Id); got != tc.wantStored {
					t.Errorf("stored %q after a failed save, want %q", got, tc.wantStored)
				}
			})
		})
	}

	t.Run("the real texts name the lock, escaped", func(t *testing.T) {
		withRealLocales(t)
		p := &settingsPress{tr: i18n.MustNewTranslator("en")}
		got := settingsLocksFailureNotice(p, "locks_lock_failed", settingsLockRow{NameKey: "settings_lock_bots"})
		if want := "⚠️ Failed to lock: Bots. Please try again later."; got != want {
			t.Errorf("lock notice = %q, want %q", got, want)
		}
		got = settingsLocksFailureNotice(p, "locks_unlock_failed", settingsLockRow{NameKey: "settings_lock_other"})
		if want := "⚠️ Failed to unlock: Game/sticker/GIF. Please try again later."; got != want {
			t.Errorf("unlock notice = %q, want %q", got, want)
		}
	})
}
