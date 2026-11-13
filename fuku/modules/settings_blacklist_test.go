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

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/blacklists"
	dbcache "github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// settingsBlacklistTexts is every text the bot sent or edited and every callback answer of the fake so far, joined, so a
// privacy test can search all of it for a sentinel at once.
func settingsBlacklistTexts(env *settingsListEnv) string {
	var sb strings.Builder
	for _, method := range []string{"editMessageText", "sendMessage", "answerCallbackQuery"} {
		for _, call := range env.fake.callsFor(method) {
			fmt.Fprint(&sb, call.Params["text"], call.Params["reply_markup"], "\n")
		}
	}
	return sb.String()
}

// settingsBlacklistCountSeams replaces the page's read and count seams with counting wrappers and restores them.
func settingsBlacklistCountSeams(t *testing.T) (reads, counts *atomic.Int64) {
	t.Helper()
	reads, counts = new(atomic.Int64), new(atomic.Int64)
	read, count := settingsBlacklistRead, settingsBlacklistCount
	settingsBlacklistRead = func(ctx context.Context, chatID int64) ([]settingsListEntry, error) {
		reads.Add(1)
		return read(ctx, chatID)
	}
	settingsBlacklistCount = func(ctx context.Context, chatID int64) (int, error) {
		counts.Add(1)
		return count(ctx, chatID)
	}
	t.Cleanup(func() { settingsBlacklistRead, settingsBlacklistCount = read, count })
	return reads, counts
}

func TestSettingsBlacklistOpens(t *testing.T) {
	t.Run("a private menu lists the words in order with the intro, the count and the one action", func(t *testing.T) {
		env := newSettingsListEnv(t)
		settingsListSeedBlacklist(t, env.chat.Id, "gamma", "alpha", "beta")
		_, text, keyboard := settingsListOpenPrivate(env, env.admin, settingsPageBlacklist)

		for _, want := range []string{
			staffMarker("settings_header"), staffMarker("settings_page_blacklist"), staffMarker("settings_bl_intro"),
			staffMarker("settings_bl_count") + " 3", staffMarker("settings_bl_action_line") + " " + staffMarker("settings_opt_warn"),
		} {
			if !strings.Contains(text, want) {
				t.Errorf("the page lacks %q:\n%s", want, text)
			}
		}
		last := -1
		for i, word := range []string{"alpha", "beta", "gamma"} {
			line := fmt.Sprintf("%d. <code>%s</code>", i+1, word)
			at := strings.Index(text, line)
			if at < 0 || at < last {
				t.Errorf("the line %q is missing or out of order:\n%s", line, text)
			}
			last = at
		}
		if got := settingsListRowWidths(keyboard); fmt.Sprint(got) != fmt.Sprint([]int{2}) {
			t.Errorf("row widths = %v, want the navigation row alone", got)
		}
		if back := keyboard[len(keyboard)-1]; len(back) != 2 || !strings.Contains(back[0].Text, staffMarker("settings_btn_back")) {
			t.Errorf("the page does not end with Back and Close: %+v", back)
		}
	})

	t.Run("Main's Blacklist line is the count in digits, and the unknown mark when the count fails", func(t *testing.T) {
		env := newSettingsListEnv(t)
		settingsListSeedBlacklist(t, env.chat.Id, settingsListWords(3)...)
		msgID := env.updateID()
		env.pressPrivate(env.admin, msgID, env.privateFields(env.chat.Id, "a", settingsActBack))
		text, keyboard := env.lastEdit(env.admin.Id, msgID)
		line, ok := settingsMainStateLine(text, "settings_page_blacklist")
		if !ok || !strings.HasSuffix(line, " 3") {
			t.Fatalf("Main's Blacklist line = %q, want it to end in the count 3:\n%s", line, text)
		}
		if _, ok := settingsButtonByMarker(keyboard, "settings_page_blacklist"); !ok {
			t.Errorf("Main has no Blacklist cell")
		}

		count := settingsBlacklistCount
		settingsBlacklistCount = func(context.Context, int64) (int, error) { return 0, errors.New("count failed") }
		t.Cleanup(func() { settingsBlacklistCount = count })
		failedMsg := env.updateID()
		env.pressPrivate(env.admin, failedMsg, env.privateFields(env.chat.Id, "a", settingsActBack))
		failed, _ := env.lastEdit(env.admin.Id, failedMsg)
		unknown, ok := settingsMainStateLine(failed, "settings_page_blacklist")
		if !ok || !strings.HasSuffix(unknown, settingsUnknownState) {
			t.Errorf("a failing count reads %q, want the unknown mark", unknown)
		}
		// Every other line is unchanged.
		for _, key := range settingsMainTitleKeys {
			if key == "settings_page_blacklist" {
				continue
			}
			before, _ := settingsMainStateLine(text, key)
			after, _ := settingsMainStateLine(failed, key)
			if before != after {
				t.Errorf("the failing count changed the %s line: %q -> %q", key, before, after)
			}
		}
	})

	t.Run("opening Main and the page on a chat with no rows creates no row and fills no cache key", func(t *testing.T) {
		env := newSettingsListEnv(t)
		chat := env.addChat()
		settingsListCleanup(t, chat.Id)
		mainMsg, pageMsg := env.updateID(), env.updateID()
		env.pressPrivate(env.admin, mainMsg, env.privateFields(chat.Id, "a", settingsActBack))
		env.pressPrivate(env.admin, pageMsg, env.privateFields(chat.Id, "a", settingsActOpen, "p", settingsPageBlacklist))
		text, _ := env.lastEdit(env.admin.Id, pageMsg)
		if !strings.Contains(text, staffMarker("settings_bl_empty")) {
			t.Fatalf("the empty page did not draw:\n%s", text)
		}
		var rows, chats int64
		db.DB.Model(&models.BlacklistSettings{}).Where("chat_id = ?", chat.Id).Count(&rows)
		db.DB.Model(&models.Chat{}).Where("chat_id = ?", chat.Id).Count(&chats)
		if rows != 0 || chats != 0 {
			t.Errorf("opening created %d blacklist rows and %d chat rows", rows, chats)
		}
		if key := fmt.Sprintf("fuku:cache:blacklist:%d", chat.Id); env.redis.Exists(key) {
			t.Errorf("opening filled the command's cache key %s", key)
		}
	})

	t.Run("a failing read is the cannot-open edit", func(t *testing.T) {
		env := newSettingsListEnv(t)
		read := settingsBlacklistRead
		settingsBlacklistRead = func(context.Context, int64) ([]settingsListEntry, error) { return nil, errors.New("read failed") }
		t.Cleanup(func() { settingsBlacklistRead = read })
		msgID := env.updateID()
		env.pressPrivate(env.admin, msgID, env.privateFields(env.chat.Id, "a", settingsActOpen, "p", settingsPageBlacklist))
		text, keyboard := env.lastEdit(env.admin.Id, msgID)
		if !strings.Contains(text, staffMarker("settings_cannot_open")) || len(keyboard) != 0 {
			t.Errorf("a failing read drew %q with %d rows, want the cannot-open edit with no keyboard", text, len(keyboard))
		}
	})
}

func TestSettingsListPager(t *testing.T) {
	env := newSettingsListEnv(t)
	settingsListSeedBlacklist(t, env.chat.Id, settingsListWords(20)...)
	msgID := env.updateID()
	page := func(v string) (string, [][]gotgbot.InlineKeyboardButton) {
		t.Helper()
		env.pressPrivate(env.admin, msgID, env.privateFields(env.chat.Id, "a", "bg", "v", v))
		return env.lastEdit(env.admin.Id, msgID)
	}

	text, keyboard := page("1")
	if !strings.Contains(text, "1. <code>w01</code>") || strings.Contains(text, "9. <code>") {
		t.Errorf("page 1 drew:\n%s", text)
	}
	next, ok := settingsButtonByMarker(keyboard, "staff_panel_next")
	if !ok {
		t.Fatalf("page 1 has no Next: %v", settingsListRowWidths(keyboard))
	}
	if _, ok := settingsButtonByMarker(keyboard, "staff_panel_prev"); ok {
		t.Errorf("page 1 has Prev")
	}

	// Next leads to page 2 through the button's own data.
	env.pressRaw(gotgbot.Chat{Id: env.admin.Id, Type: "private"}, env.admin, msgID, next.CallbackData)
	text, keyboard = env.lastEdit(env.admin.Id, msgID)
	if !strings.Contains(text, "9. <code>w09</code>") || !strings.Contains(text, "16. <code>w16</code>") {
		t.Errorf("Next did not lead to page 2:\n%s", text)
	}
	if _, ok := settingsButtonByMarker(keyboard, "staff_panel_prev"); !ok {
		t.Errorf("page 2 has no Prev")
	}

	text, keyboard = page("3")
	if !strings.Contains(text, "17. <code>w17</code>") || strings.Contains(text, "16. <code>") {
		t.Errorf("page 3 drew:\n%s", text)
	}
	prev, ok := settingsButtonByMarker(keyboard, "staff_panel_prev")
	if _, hasNext := settingsButtonByMarker(keyboard, "staff_panel_next"); !ok || hasNext {
		t.Errorf("page 3 must have Prev and no Next: %v", settingsListRowWidths(keyboard))
	}
	env.pressRaw(gotgbot.Chat{Id: env.admin.Id, Type: "private"}, env.admin, msgID, prev.CallbackData)
	if text, _ = env.lastEdit(env.admin.Id, msgID); !strings.Contains(text, "9. <code>w09</code>") {
		t.Errorf("Prev from page 3 did not lead to page 2:\n%s", text)
	}

	if text, _ = page("9999"); !strings.Contains(text, "17. <code>w17</code>") {
		t.Errorf("v=9999 did not draw the last page:\n%s", text)
	}
	if text, _ = page("0"); !strings.Contains(text, "1. <code>w01</code>") {
		t.Errorf("v=0 did not draw page 1:\n%s", text)
	}

	// A word deleted by the command between two presses is gone at the next press.
	env.send(env.admin, "/rmblacklist w01")
	if text, _ = page("1"); strings.Contains(text, "<code>w01</code>") || !strings.Contains(text, "1. <code>w02</code>") {
		t.Errorf("a word removed by /rmblacklist is still listed:\n%s", text)
	}
}

func TestSettingsListGroupNote(t *testing.T) {
	setCachedBotUsername(t, "FukuTestBot")
	env := newSettingsListEnv(t)
	settingsListSeedBlacklist(t, env.chat.Id, settingsListSentinel+"1", settingsListSentinel+"2")
	reads, counts := settingsBlacklistCountSeams(t)

	msgID := env.openMenu(env.admin)
	mainText, _ := env.lastEdit(env.chat.Id, msgID)
	line, ok := settingsMainStateLine(mainText, "settings_page_blacklist")
	if !ok || !strings.HasSuffix(line, staffMarker("settings_state_private_only")) {
		t.Errorf("a group Main's Blacklist line = %q, want In private", line)
	}
	if counts.Load() != 0 {
		t.Errorf("a group Main counted the list %d times, want 0", counts.Load())
	}

	env.press(env.admin, msgID, env.groupFields(env.admin, "a", settingsActOpen, "p", settingsPageBlacklist))
	text, keyboard := env.lastEdit(env.chat.Id, msgID)
	if !strings.Contains(text, staffMarker("settings_entries_group_note")) {
		t.Errorf("the group page drew %q, want the note", text)
	}
	if urls, want := settingsURLButtons(keyboard), settingsPrivateURL("FukuTestBot", env.chat.Id); len(urls) != 1 || urls[0] != want {
		t.Errorf("Open in private URL buttons = %v, want [%s]", urls, want)
	}
	if got := settingsListRowWidths(keyboard); fmt.Sprint(got) != fmt.Sprint([]int{1, 2}) {
		t.Errorf("the note's rows = %v, want {1,2}", got)
	}
	if reads.Load() != 0 || counts.Load() != 0 {
		t.Errorf("a group menu read the list %d times and counted it %d times, want none", reads.Load(), counts.Load())
	}

	// The pager of a group menu is an invalid request in step 1: no getChatMember, no edit.
	lookups, edits := env.memberLookupsOf(env.chat.Id, env.admin.Id), len(env.edits(env.chat.Id, msgID))
	env.press(env.admin, msgID, env.groupFields(env.admin, "a", "bg", "v", "1"))
	if got, alert := env.lastAnswer(); !strings.Contains(got, staffMarker("common_callback_invalid_request")) || !alert {
		t.Errorf("bg from a group menu answered %q (alert %v), want the invalid request alert", got, alert)
	}
	if got := env.memberLookupsOf(env.chat.Id, env.admin.Id); got != lookups {
		t.Errorf("bg from a group menu made %d getChatMember calls, want none", got-lookups)
	}
	if got := len(env.edits(env.chat.Id, msgID)); got != edits {
		t.Errorf("bg from a group menu edited the message")
	}
	if reads.Load() != 0 {
		t.Errorf("a forged pager press read the list")
	}

	if all := settingsBlacklistTexts(env); strings.Contains(all, settingsListSentinel) {
		t.Errorf("a group menu leaked the sentinel word:\n%s", all)
	}
}

// settingsBlacklistMenu is a private menu of one chat opened on the Blacklist page: the message the page was drawn into
// and the helpers that press its buttons as the env's administrator.
type settingsBlacklistMenu struct {
	env   *settingsListEnv
	chat  gotgbot.Chat
	msgID int64
}

// settingsBlacklistOpenMenu opens the Blacklist page of chat in a fresh private message of the env's administrator.
func settingsBlacklistOpenMenu(env *settingsListEnv, chat gotgbot.Chat) *settingsBlacklistMenu {
	env.t.Helper()
	m := &settingsBlacklistMenu{env: env, chat: chat, msgID: env.updateID()}
	m.press("a", settingsActOpen, "p", settingsPageBlacklist)
	return m
}

// press presses a button of the menu with the given fields (the owner field c is added).
func (m *settingsBlacklistMenu) press(kv ...string) {
	m.env.t.Helper()
	m.env.pressPrivate(m.env.admin, m.msgID, m.env.privateFields(m.chat.Id, kv...))
}

// last is the text and keyboard the menu message was last edited to.
func (m *settingsBlacklistMenu) last() (string, [][]gotgbot.InlineKeyboardButton) {
	m.env.t.Helper()
	return m.env.lastEdit(m.env.admin.Id, m.msgID)
}

// tap presses a button as drawn, through its own callback data.
func (m *settingsBlacklistMenu) tap(button gotgbot.InlineKeyboardButton) {
	m.env.t.Helper()
	m.env.pressRaw(gotgbot.Chat{Id: m.env.admin.Id, Type: "private"}, m.env.admin, m.msgID, button.CallbackData)
}

// button finds the drawn button with the exact label.
func (m *settingsBlacklistMenu) button(label string) gotgbot.InlineKeyboardButton {
	m.env.t.Helper()
	_, keyboard := m.last()
	for _, button := range settingsAllButtons(keyboard) {
		if button.Text == label {
			return button
		}
	}
	m.env.t.Fatalf("the page has no button %q", label)
	return gotgbot.InlineKeyboardButton{}
}

// markedButton finds the drawn button whose label carries the marker of the locale key.
func (m *settingsBlacklistMenu) markedButton(key string) (gotgbot.InlineKeyboardButton, bool) {
	m.env.t.Helper()
	_, keyboard := m.last()
	return settingsButtonByMarker(keyboard, key)
}

// settingsBlacklistIDOf is the row ID of the chat's stored word, failing the test when it is not stored.
func settingsBlacklistIDOf(t *testing.T, chatID int64, word string) int64 {
	t.Helper()
	rows, err := blacklists.ListBlacklistEntriesFresh(context.Background(), chatID)
	if err != nil {
		t.Fatalf("list the blacklist: %v", err)
	}
	for _, row := range rows {
		if row.Word == word {
			return int64(row.ID)
		}
	}
	t.Fatalf("word %q is not stored in chat %d", word, chatID)
	return 0
}

// settingsBlacklistStored returns the chat's stored words with their actions.
func settingsBlacklistStored(t *testing.T, chatID int64) map[string]string {
	t.Helper()
	rows, err := blacklists.ListBlacklistEntriesFresh(context.Background(), chatID)
	if err != nil {
		t.Fatalf("list the blacklist: %v", err)
	}
	stored := make(map[string]string, len(rows))
	for _, row := range rows {
		stored[row.Word] = row.Action
	}
	return stored
}

// settingsBlacklistWithBot returns a fresh chat of the env with the bot's role and one stored word, cleaned up with the test.
func settingsBlacklistWithBot(env *settingsListEnv, role string, words ...string) gotgbot.Chat {
	env.t.Helper()
	chat := env.addChat()
	env.fake.setBotRole(chat.Id, role)
	settingsListCleanup(env.t, chat.Id)
	settingsListSeedBlacklist(env.t, chat.Id, words...)
	return chat
}

func TestSettingsBlacklistDelete(t *testing.T) {
	t.Run("the ask draws a card naming the word, Yes deletes that row and drops the cache, No returns to the page", func(t *testing.T) {
		env := newSettingsListEnv(t)
		settingsListSeedBlacklist(t, env.chat.Id, "alpha", "beta", "gamma", "delta")
		cacheKey := dbcache.CacheKey("blacklist", env.chat.Id)
		blacklists.GetBlacklistSettingsContext(context.Background(), env.chat.Id)
		if !env.redis.Exists(cacheKey) {
			t.Fatalf("the command's cache key %s was not primed", cacheKey)
		}
		m := settingsBlacklistOpenMenu(env, env.chat)
		deltaID := settingsBlacklistIDOf(t, env.chat.Id, "delta")

		m.tap(m.button("🗑 3"))
		text, keyboard := m.last()
		if !strings.Contains(text, staffMarker("settings_bl_delete_ask")+" delta") {
			t.Fatalf("the card does not name the word:\n%s", text)
		}
		if got := settingsListRowWidths(keyboard); fmt.Sprint(got) != fmt.Sprint([]int{2, 2}) {
			t.Errorf("card rows = %v, want {2,2}", got)
		}
		yes, _ := m.markedButton("settings_btn_yes")
		yesFields := settingsButtonFields(t, yes)
		if yesFields["a"] != "by" || yesFields["r"] != strconv.FormatInt(deltaID, 10) || yesFields["e"] == "" || len(yesFields) != 4 {
			t.Errorf("Yes carries %v, want a=by, the row ID, an expiry and c", yesFields)
		}
		no, _ := m.markedButton("settings_btn_no")
		if f := settingsButtonFields(t, no); f["a"] != "bg" || f["v"] != "1" || len(f) != 3 {
			t.Errorf("No carries %v, want a=bg, v=1 and c", f)
		}
		if got := settingsBlacklistStored(t, env.chat.Id); len(got) != 4 {
			t.Errorf("the ask changed the list: %v", got)
		}

		answers := env.answerCount()
		m.tap(yes)
		if got := env.answerCount() - answers; got != 1 {
			t.Errorf("the Yes was answered %d times, want once", got)
		}
		text, _ = m.last()
		if !strings.Contains(text, settingsNoticeOKIcon+staffMarker("settings_bl_removed")+" delta") {
			t.Errorf("the page lacks the removed notice naming the word:\n%s", text)
		}
		for _, line := range []string{"1. <code>alpha</code>", "2. <code>beta</code>", "3. <code>gamma</code>"} {
			if !strings.Contains(text, line) {
				t.Errorf("the redrawn page lacks %q:\n%s", line, text)
			}
		}
		if _, still := settingsBlacklistStored(t, env.chat.Id)["delta"]; still {
			t.Errorf("the word is still stored")
		}
		if env.redis.Exists(cacheKey) {
			t.Errorf("the delete did not drop %s", cacheKey)
		}
		if slices.Contains(blacklists.GetBlacklistSettingsContext(context.Background(), env.chat.Id).Triggers(), "delta") {
			t.Errorf("the command's reader still serves the deleted word")
		}
	})

	t.Run("a Yes redraws the page where the word stood, and No returns to the ask's page", func(t *testing.T) {
		env := newSettingsListEnv(t)
		settingsListSeedBlacklist(t, env.chat.Id, settingsListWords(20)...)
		m := settingsBlacklistOpenMenu(env, env.chat)
		m.press("a", "bg", "v", "2")
		m.tap(m.button("🗑 10"))
		no, _ := m.markedButton("settings_btn_no")
		m.tap(no)
		text, _ := m.last()
		if !strings.Contains(text, "9. <code>w09</code>") || strings.Contains(text, settingsNoticeOKIcon) || strings.Contains(text, settingsNoticeWarnIcon) {
			t.Errorf("No did not return to page 2 without a notice:\n%s", text)
		}
		m.tap(m.button("🗑 10"))
		yes, _ := m.markedButton("settings_btn_yes")
		m.tap(yes)
		text, _ = m.last()
		if !strings.Contains(text, "10. <code>w11</code>") || !strings.Contains(text, staffMarker("staff_panel_page")+" 2 3") {
			t.Errorf("a Yes on page 2 did not redraw page 2:\n%s", text)
		}
	})

	t.Run("an expired Yes deletes nothing", func(t *testing.T) {
		env := newSettingsListEnv(t)
		settingsListSeedBlacklist(t, env.chat.Id, "alpha", "beta")
		lifetime := settingsCardLifetime
		settingsCardLifetime = -2 * time.Minute
		t.Cleanup(func() { settingsCardLifetime = lifetime })
		m := settingsBlacklistOpenMenu(env, env.chat)
		m.tap(m.button("🗑 1"))
		yes, _ := m.markedButton("settings_btn_yes")
		m.tap(yes)
		text, _ := m.last()
		if !strings.Contains(text, settingsNoticeWarnIcon+staffMarker("settings_card_expired")) {
			t.Errorf("an expired Yes drew:\n%s", text)
		}
		if got := settingsBlacklistStored(t, env.chat.Id); len(got) != 2 {
			t.Errorf("an expired Yes deleted a word: %v", got)
		}
	})

	t.Run("a Yes after the word was deleted and added again deletes nothing", func(t *testing.T) {
		env := newSettingsListEnv(t)
		settingsListSeedBlacklist(t, env.chat.Id, "alpha", "beta")
		m := settingsBlacklistOpenMenu(env, env.chat)
		m.tap(m.button("🗑 1"))
		yes, _ := m.markedButton("settings_btn_yes")
		oldID := settingsBlacklistIDOf(t, env.chat.Id, "alpha")
		env.send(env.admin, "/rmblacklist alpha")
		env.send(env.admin, "/addblacklist alpha")
		if newID := settingsBlacklistIDOf(t, env.chat.Id, "alpha"); newID == oldID {
			t.Fatalf("the re-added word kept its row ID %d", oldID)
		}
		m.tap(yes)
		text, _ := m.last()
		if !strings.Contains(text, settingsNoticeWarnIcon+staffMarker("settings_entries_already_removed")) {
			t.Errorf("a stale Yes drew:\n%s", text)
		}
		if _, ok := settingsBlacklistStored(t, env.chat.Id)["alpha"]; !ok {
			t.Errorf("a stale Yes deleted the re-added word")
		}
	})

	t.Run("another chat's row ID shows the already-removed notice, draws no card and deletes nothing", func(t *testing.T) {
		env := newSettingsListEnv(t)
		other := settingsBlacklistWithBot(env, staffRoleAdmin, "foreign")
		foreignID := settingsBlacklistIDOf(t, other.Id, "foreign")
		settingsListSeedBlacklist(t, env.chat.Id, "alpha")
		m := settingsBlacklistOpenMenu(env, env.chat)

		m.press("a", "bx", "r", strconv.FormatInt(foreignID, 10), "v", "1")
		text, keyboard := m.last()
		if !strings.Contains(text, settingsNoticeWarnIcon+staffMarker("settings_entries_already_removed")) {
			t.Errorf("a foreign ask drew:\n%s", text)
		}
		if _, ok := settingsButtonByMarker(keyboard, "settings_btn_yes"); ok {
			t.Errorf("a foreign ask drew a card")
		}
		m.press("a", "by", "r", strconv.FormatInt(foreignID, 10), "e", settingsCardExpiry(time.Now()))
		if text, _ = m.last(); !strings.Contains(text, staffMarker("settings_entries_already_removed")) {
			t.Errorf("a foreign Yes drew:\n%s", text)
		}
		if got := settingsBlacklistStored(t, other.Id); len(got) != 1 {
			t.Errorf("a foreign Yes deleted the other chat's word: %v", got)
		}
	})

	t.Run("a presser demoted between the ask and the Yes is refused and nothing is deleted", func(t *testing.T) {
		env := newSettingsListEnv(t)
		settingsListSeedBlacklist(t, env.chat.Id, "alpha")
		m := settingsBlacklistOpenMenu(env, env.chat)
		m.tap(m.button("🗑 1"))
		yes, _ := m.markedButton("settings_btn_yes")
		env.fake.setMember(env.chat.Id, env.admin.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusAdministrator})
		edits := len(env.edits(env.admin.Id, m.msgID))
		m.tap(yes)
		if text, alert := env.lastAnswer(); !strings.Contains(text, staffMarker("settings_no_restrict")) || !alert {
			t.Errorf("the demoted Yes answered %q (alert %v), want the alert settings_no_restrict", text, alert)
		}
		if got := len(env.edits(env.admin.Id, m.msgID)); got != edits {
			t.Errorf("the refused Yes edited the menu")
		}
		if got := settingsBlacklistStored(t, env.chat.Id); len(got) != 1 {
			t.Errorf("a demoted presser deleted a word: %v", got)
		}
	})

	t.Run("a bot that cannot restrict answers the notice and no card is drawn", func(t *testing.T) {
		env := newSettingsListEnv(t)
		chat := settingsBlacklistWithBot(env, staffRoleAdminNoRestrict, "alpha")
		m := settingsBlacklistOpenMenu(env, chat)
		m.tap(m.button("🗑 1"))
		text, keyboard := m.last()
		if !strings.Contains(text, settingsNoticeWarnIcon+staffMarker("chat_status_bot_restrict_error")) {
			t.Errorf("the ask drew:\n%s", text)
		}
		if _, ok := settingsButtonByMarker(keyboard, "settings_btn_yes"); ok {
			t.Errorf("a refused ask drew a card")
		}
		if got := settingsBlacklistStored(t, chat.Id); len(got) != 1 {
			t.Errorf("the refused press changed the list: %v", got)
		}
	})

	t.Run("a failed delete redraws with the save-failed notice and keeps the word", func(t *testing.T) {
		env := newSettingsListEnv(t)
		settingsListSeedBlacklist(t, env.chat.Id, "alpha", "beta")
		m := settingsBlacklistOpenMenu(env, env.chat)
		m.tap(m.button("🗑 2"))
		yes, _ := m.markedButton("settings_btn_yes")
		removeByID := blacklistWrites.RemoveByID
		blacklistWrites.RemoveByID = func(context.Context, int64, int64) (string, bool, error) {
			return "", false, errors.New("delete failed")
		}
		t.Cleanup(func() { blacklistWrites.RemoveByID = removeByID })
		m.tap(yes)
		text, _ := m.last()
		if !strings.Contains(text, settingsNoticeWarnIcon+staffMarker("common_settings_save_failed")) || !strings.Contains(text, "2. <code>beta</code>") {
			t.Errorf("a failed delete drew:\n%s", text)
		}
		if got := settingsBlacklistStored(t, env.chat.Id); len(got) != 2 {
			t.Errorf("a failed delete changed the list: %v", got)
		}
	})
}

func TestSettingsListForged(t *testing.T) {
	env := newSettingsListEnv(t)
	settingsListSeedBlacklist(t, env.chat.Id, "alpha")
	m := settingsBlacklistOpenMenu(env, env.chat)
	valid := settingsCardExpiry(time.Now())
	groupMsg := env.openMenu(env.admin)

	// refused is a press that must be an invalid request before any I/O: no getChatMember, no getChat, no edit.
	refused := func(t *testing.T, name string, press func()) {
		t.Helper()
		members, chats := len(env.fake.callsFor("getChatMember")), len(env.fake.callsFor("getChat"))
		edits, answers := len(env.edits(env.admin.Id, m.msgID)), env.answerCount()
		press()
		if got := env.answerCount() - answers; got != 1 {
			t.Errorf("%s: answered %d times, want once", name, got)
		}
		if text, alert := env.lastAnswer(); !strings.Contains(text, staffMarker("common_callback_invalid_request")) || !alert {
			t.Errorf("%s: answered %q (alert %v), want the invalid request alert", name, text, alert)
		}
		if got := len(env.fake.callsFor("getChatMember")) - members; got != 0 {
			t.Errorf("%s: made %d getChatMember calls, want none", name, got)
		}
		if got := len(env.fake.callsFor("getChat")) - chats; got != 0 {
			t.Errorf("%s: made %d getChat calls, want none", name, got)
		}
		if got := len(env.edits(env.admin.Id, m.msgID)); got != edits {
			t.Errorf("%s: edited the menu", name)
		}
	}
	// A press whose data cannot even be encoded with the test chat's 19-digit ID is held to the step-1 check itself
	// with a realistic 14-character chat ID: Telegram would never deliver it, and the validator must refuse it anyway.
	tooLong := func(t *testing.T, name string, kv []string) bool {
		t.Helper()
		if encodeCallbackData(settingsCallbackNamespace, m.env.privateFields(m.chat.Id, kv...)) != "" {
			return false
		}
		fields := map[string]string{"c": "-1001234567890"}
		for i := 0; i+1 < len(kv); i += 2 {
			fields[kv[i]] = kv[i+1]
		}
		if settingsCheckFields(kv[1], fields) {
			t.Errorf("%s: step 1 accepted %v", name, fields)
		}
		return true
	}
	for name, kv := range map[string][]string{
		"bx with an 18-digit r":   {"a", "bx", "r", "123456789012345678", "v", "1"},
		"bx with a leading zero":  {"a", "bx", "r", "012", "v", "1"},
		"bx with a sign":          {"a", "bx", "r", "-5", "v", "1"},
		"bx with letters":         {"a", "bx", "r", "12ab", "v", "1"},
		"bx without v":            {"a", "bx", "r", "5"},
		"bx with a 5-digit v":     {"a", "bx", "r", "5", "v", "12345"},
		"bx without r":            {"a", "bx", "v", "1"},
		"bx with an extra key":    {"a", "bx", "r", "5", "v", "1", "x", "1"},
		"bx with u as well":       {"a", "bx", "r", "5", "v", "1", "u", "5"},
		"by without e":            {"a", "by", "r", "5"},
		"by with a non-numeric e": {"a", "by", "r", "5", "e", "abc"},
		"by with an 18-digit r":   {"a", "by", "r", "123456789012345678", "e", valid},
		"by with a v":             {"a", "by", "r", "5", "e", valid, "v", "1"},
		"by with u as well":       {"a", "by", "r", "5", "e", valid, "u", "5"},
		"bg with both v and r":    {"a", "bg", "v", "1", "r", "5"},
		"bg without v":            {"a", "bg"},
		"bg with a letter v":      {"a", "bg", "v", "x"},
		"st naming page b":        {"a", "st", "p", "b", "t", "x", "v", "1"},
		"tg naming page b":        {"a", "tg", "p", "b", "t", "x", "v", "1"},
		"bg with a 5-digit v":     {"a", "bg", "v", "12345"},
		"bx with a 6-digit v":     {"a", "bx", "r", "12345678901234567", "v", "123456"},
	} {
		if !tooLong(t, name, kv) {
			refused(t, name, func() { m.press(kv...) })
		}
	}

	// The same press of a group menu (u instead of c) is refused for every list code.
	for _, kv := range [][]string{
		{"a", "bg", "v", "1"},
		{"a", "bx", "r", "5", "v", "1"},
		{"a", "by", "r", "5", "e", valid},
	} {
		members := len(env.fake.callsFor("getChatMember"))
		env.press(env.admin, groupMsg, env.groupFields(env.admin, kv...))
		if text, alert := env.lastAnswer(); !strings.Contains(text, staffMarker("common_callback_invalid_request")) || !alert {
			t.Errorf("%s from a group menu answered %q (alert %v), want the invalid request alert", kv[1], text, alert)
		}
		if got := len(env.fake.callsFor("getChatMember")) - members; got != 0 {
			t.Errorf("%s from a group menu made %d getChatMember calls, want none", kv[1], got)
		}
	}
	if got := settingsBlacklistStored(t, env.chat.Id); len(got) != 1 {
		t.Errorf("a forged press changed the list: %v", got)
	}
}

func TestSettingsParityBlacklistRemove(t *testing.T) {
	env := newSettingsListEnv(t)
	pageChat := settingsBlacklistWithBot(env, staffRoleAdmin, "alpha", "beta", "gamma")
	commandChat := settingsBlacklistWithBot(env, staffRoleAdmin, "alpha", "beta", "gamma")
	prime := func(chatID int64) string {
		key := dbcache.CacheKey("blacklist", chatID)
		blacklists.GetBlacklistSettingsContext(context.Background(), chatID)
		if !env.redis.Exists(key) {
			t.Fatalf("the cache key %s was not primed", key)
		}
		return key
	}
	pageKey, commandKey := prime(pageChat.Id), prime(commandChat.Id)

	m := settingsBlacklistOpenMenu(env, pageChat)
	m.tap(m.button("🗑 2"))
	yes, _ := m.markedButton("settings_btn_yes")
	m.tap(yes)
	env.sendIn(commandChat, env.admin, "/rmblacklist beta")

	pageRows, commandRows := settingsBlacklistStored(t, pageChat.Id), settingsBlacklistStored(t, commandChat.Id)
	if !maps.Equal(pageRows, commandRows) || len(pageRows) != 2 {
		t.Errorf("the page left %v and the command %v, want the same two rows", pageRows, commandRows)
	}
	if env.redis.Exists(pageKey) != env.redis.Exists(commandKey) || env.redis.Exists(pageKey) {
		t.Errorf("cache state differs: page key present %v, command key present %v, want both dropped", env.redis.Exists(pageKey), env.redis.Exists(commandKey))
	}
}
