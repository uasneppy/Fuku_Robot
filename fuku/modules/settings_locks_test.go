//go:build testtools

package modules

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db/locks"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
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
