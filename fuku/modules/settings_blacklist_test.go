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
	"github.com/uasneppy/fuku_robot/fuku/i18n"
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
		if got := settingsListRowWidths(keyboard); fmt.Sprint(got) != fmt.Sprint([]int{3, 2, 2}) {
			t.Errorf("row widths = %v, want the delete row, the Add and Action row and the navigation row", got)
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
		env.pressPrivate(env.admin, msgID, env.privateFields(env.chat.Id, "a", "bp", "v", v))
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
	env.press(env.admin, msgID, env.groupFields(env.admin, "a", "bp", "v", "1"))
	if got, alert := env.lastAnswer(); !strings.Contains(got, staffMarker("common_callback_invalid_request")) || !alert {
		t.Errorf("bp from a group menu answered %q (alert %v), want the invalid request alert", got, alert)
	}
	if got := env.memberLookupsOf(env.chat.Id, env.admin.Id); got != lookups {
		t.Errorf("bp from a group menu made %d getChatMember calls, want none", got-lookups)
	}
	if got := len(env.edits(env.chat.Id, msgID)); got != edits {
		t.Errorf("bp from a group menu edited the message")
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
	user  gotgbot.User
	msgID int64
}

// settingsBlacklistOpenMenu opens the Blacklist page of chat in a fresh private message of the env's administrator.
func settingsBlacklistOpenMenu(env *settingsListEnv, chat gotgbot.Chat) *settingsBlacklistMenu {
	env.t.Helper()
	return settingsBlacklistOpenMenuAs(env, chat, env.admin)
}

// settingsBlacklistOpenMenuAs is settingsBlacklistOpenMenu pressed by another user.
func settingsBlacklistOpenMenuAs(env *settingsListEnv, chat gotgbot.Chat, user gotgbot.User) *settingsBlacklistMenu {
	env.t.Helper()
	m := &settingsBlacklistMenu{env: env, chat: chat, user: user, msgID: env.updateID()}
	m.press("a", settingsActOpen, "p", settingsPageBlacklist)
	return m
}

// press presses a button of the menu with the given fields (the owner field c is added).
func (m *settingsBlacklistMenu) press(kv ...string) {
	m.env.t.Helper()
	m.env.pressPrivate(m.user, m.msgID, m.env.privateFields(m.chat.Id, kv...))
}

// last is the text and keyboard the menu message was last edited to.
func (m *settingsBlacklistMenu) last() (string, [][]gotgbot.InlineKeyboardButton) {
	m.env.t.Helper()
	return m.env.lastEdit(m.user.Id, m.msgID)
}

// tap presses a button as drawn, through its own callback data.
func (m *settingsBlacklistMenu) tap(button gotgbot.InlineKeyboardButton) {
	m.env.t.Helper()
	m.env.pressRaw(gotgbot.Chat{Id: m.user.Id, Type: "private"}, m.user, m.msgID, button.CallbackData)
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
		if f := settingsButtonFields(t, no); f["a"] != "bp" || f["v"] != "1" || len(f) != 3 {
			t.Errorf("No carries %v, want a=bp, v=1 and c", f)
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
		m.press("a", "bp", "v", "2")
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
	forged := settingsFiltersForgedPresses(valid)
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
		"bp with both v and r":    {"a", "bp", "v", "1", "r", "5"},
		"bg without v":            {"a", "bp"},
		"bp with a letter v":      {"a", "bp", "v", "x"},
		"st naming page b":        {"a", "st", "p", "b", "t", "x", "v", "1"},
		"tg naming page b":        {"a", "tg", "p", "b", "t", "x", "v", "1"},
		"bp with a 5-digit v":     {"a", "bp", "v", "12345"},
		"bx with a 6-digit v":     {"a", "bx", "r", "12345678901234567", "v", "123456"},
	} {
		if !tooLong(t, name, kv) {
			refused(t, name, func() { m.press(kv...) })
		}
	}
	// The Filters page's codes are refused the same way (plan 19-07).
	for name, kv := range forged {
		if !tooLong(t, name, kv) {
			refused(t, name, func() { m.press(kv...) })
		}
	}
	// So are the Notes page's (plan 19-09).
	for name, kv := range settingsNotesForgedPresses(valid) {
		if !tooLong(t, name, kv) {
			refused(t, name, func() { m.press(kv...) })
		}
	}

	// The same press of a group menu (u instead of c) is refused for every list code.
	for _, kv := range [][]string{
		{"a", "bp", "v", "1"},
		{"a", "bx", "r", "5", "v", "1"},
		{"a", "by", "r", "5", "e", valid},
		{"a", "ig", "v", "1"},
		{"a", "ix", "r", "5", "v", "1"},
		{"a", "iy", "r", "5", "e", valid},
		{"a", "ia"},
		{"a", "ic", "t", "k"},
		{"a", "eg", "v", "1"},
		{"a", "ex", "r", "5", "v", "1"},
		{"a", "ey", "r", "5", "e", valid},
		{"a", "ea"},
		{"a", "ec", "t", "n"},
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

// settingsBlacklistSeedActions stores one word per action of actions, in the order given, w1, w2, ...
func settingsBlacklistSeedActions(t *testing.T, chatID int64, actions ...string) {
	t.Helper()
	for i, action := range actions {
		settingsListSeedBlacklistAction(t, chatID, fmt.Sprintf("w%02d", i+1), action)
	}
}

func TestSettingsBlacklistAction(t *testing.T) {
	actionLine := func(action string) string {
		return staffMarker("settings_bl_action_line") + " " + action
	}
	countLine := func(action string, n int) string {
		return staffMarker("settings_bl_action_count") + " " + action + " " + strconv.Itoa(n)
	}
	checked := func(keyboard [][]gotgbot.InlineKeyboardButton) []string {
		var marked []string
		for _, button := range settingsAllButtons(keyboard) {
			if strings.HasPrefix(button.Text, settingsSelectedPrefix) {
				marked = append(marked, button.Text)
			}
		}
		return marked
	}

	t.Run("a uniform list shows the action alone and marks that choice", func(t *testing.T) {
		env := newSettingsListEnv(t)
		settingsBlacklistSeedActions(t, env.chat.Id, "ban", "ban", "ban")
		m := settingsBlacklistOpenMenu(env, env.chat)
		text, keyboard := m.last()
		if !strings.Contains(text, actionLine(staffMarker("settings_opt_ban"))) {
			t.Errorf("the list page does not say Action: Ban:\n%s", text)
		}
		if _, ok := settingsButtonByMarker(keyboard, "settings_btn_bl_action"); !ok {
			t.Fatalf("the list page has no Action button: %v", settingsListRowWidths(keyboard))
		}
		m.tap(m.button(staffMarker("settings_btn_bl_action")))
		text, keyboard = m.last()
		if !strings.Contains(text, actionLine(staffMarker("settings_opt_ban"))) || strings.Contains(text, staffMarker("settings_bl_action_count")) || strings.Contains(text, staffMarker("settings_bl_action_mixed_hint")) {
			t.Errorf("the uniform action screen drew:\n%s", text)
		}
		if got := settingsListRowWidths(keyboard); fmt.Sprint(got) != fmt.Sprint([]int{3, 2, 2}) {
			t.Errorf("action screen rows = %v, want {3,2,2}", got)
		}
		if got, want := checked(keyboard), []string{settingsSelectedPrefix + staffMarker("settings_opt_ban")}; !slices.Equal(got, want) {
			t.Errorf("marked choices = %v, want %v", got, want)
		}
		back := settingsAllButtons(keyboard)[5]
		if f := settingsButtonFields(t, back); f["a"] != settingsActOpen || f["p"] != settingsPageBlacklist {
			t.Errorf("Back carries %v, want a=op p=b", f)
		}
	})

	t.Run("words on different actions show Mixed with a count line each, most words first, and no mark", func(t *testing.T) {
		env := newSettingsListEnv(t)
		actions := append(slices.Repeat([]string{"ban"}, 17), "warn", "warn", "warn")
		settingsBlacklistSeedActions(t, env.chat.Id, actions...)
		m := settingsBlacklistOpenMenu(env, env.chat)
		if text, _ := m.last(); !strings.Contains(text, actionLine(staffMarker("settings_bl_action_mixed"))) {
			t.Errorf("the list page does not say Mixed:\n%s", text)
		}
		m.tap(m.button(staffMarker("settings_btn_bl_action")))
		text, keyboard := m.last()
		ban, warn := strings.Index(text, countLine(staffMarker("settings_opt_ban"), 17)), strings.Index(text, countLine(staffMarker("settings_opt_warn"), 3))
		if ban < 0 || warn < 0 || ban > warn {
			t.Errorf("the count lines are missing or out of order (ban at %d, warn at %d):\n%s", ban, warn, text)
		}
		if !strings.Contains(text, actionLine(staffMarker("settings_bl_action_mixed"))) || !strings.Contains(text, staffMarker("settings_bl_action_mixed_hint")) {
			t.Errorf("the mixed screen lacks Mixed or its hint:\n%s", text)
		}
		if marked := checked(keyboard); len(marked) != 0 {
			t.Errorf("a mixed screen marks %v", marked)
		}
	})

	t.Run("a uniform stored value the command never writes is shown as stored with the inert line and no mark", func(t *testing.T) {
		env := newSettingsListEnv(t)
		settingsBlacklistSeedActions(t, env.chat.Id, "tban", "tban")
		m := settingsBlacklistOpenMenu(env, env.chat)
		m.tap(m.button(staffMarker("settings_btn_bl_action")))
		text, keyboard := m.last()
		if !strings.Contains(text, actionLine("tban")) || !strings.Contains(text, staffMarker("settings_bl_action_inert")) {
			t.Errorf("the inert screen drew:\n%s", text)
		}
		if marked := checked(keyboard); len(marked) != 0 {
			t.Errorf("an inert screen marks %v", marked)
		}
	})

	t.Run("all eight stored values draw eight count lines and none is worded Delete only", func(t *testing.T) {
		env := newSettingsListEnv(t)
		settingsBlacklistSeedActions(t, env.chat.Id, "warn", "mute", "kick", "ban", "none", "tban", "tmute", "delete")
		m := settingsBlacklistOpenMenu(env, env.chat)
		m.tap(m.button(staffMarker("settings_btn_bl_action")))
		text, _ := m.last()
		if got := strings.Count(text, staffMarker("settings_bl_action_count")); got != 8 {
			t.Errorf("the screen has %d count lines, want 8:\n%s", got, text)
		}
		for _, word := range []string{staffMarker("settings_bl_opt_none"), "tban", "tmute", "delete"} {
			if !strings.Contains(text, countLine(word, 1)) {
				t.Errorf("the screen lacks the count line for %q:\n%s", word, text)
			}
		}
	})

	t.Run("a choice writes the action on every word, Delete only stores none, and the same choice again shows no notice", func(t *testing.T) {
		env := newSettingsListEnv(t)
		settingsBlacklistSeedActions(t, env.chat.Id, "ban", "warn", "mute")
		cacheKey := dbcache.CacheKey("blacklist", env.chat.Id)
		blacklists.GetBlacklistSettingsContext(context.Background(), env.chat.Id)
		m := settingsBlacklistOpenMenu(env, env.chat)
		m.tap(m.button(staffMarker("settings_btn_bl_action")))

		answers := env.answerCount()
		m.tap(m.button(staffMarker("settings_bl_opt_none")))
		if got := env.answerCount() - answers; got != 1 {
			t.Errorf("the press was answered %d times, want once", got)
		}
		for word, action := range settingsBlacklistStored(t, env.chat.Id) {
			if action != "none" {
				t.Errorf("word %q has action %q, want none", word, action)
			}
		}
		text, keyboard := m.last()
		if !strings.Contains(text, actionLine(staffMarker("settings_bl_opt_none"))) || strings.Contains(text, settingsNoticeOKIcon) || strings.Contains(text, settingsNoticeWarnIcon) {
			t.Errorf("the redraw after Delete only drew:\n%s", text)
		}
		if got, want := checked(keyboard), []string{settingsSelectedPrefix + staffMarker("settings_bl_opt_none")}; !slices.Equal(got, want) {
			t.Errorf("marked choices = %v, want %v", got, want)
		}
		if env.redis.Exists(cacheKey) {
			t.Errorf("the write did not drop %s", cacheKey)
		}

		// The value every word already has: the same rows, no notice.
		m.tap(m.button(settingsSelectedPrefix + staffMarker("settings_bl_opt_none")))
		if text, _ = m.last(); strings.Contains(text, settingsNoticeOKIcon) || strings.Contains(text, settingsNoticeWarnIcon) {
			t.Errorf("a press of the stored value showed a notice:\n%s", text)
		}
	})

	t.Run("an empty list has no Action button, a stale bo draws the no-words line and a stale bs answers the notice", func(t *testing.T) {
		env := newSettingsListEnv(t)
		m := settingsBlacklistOpenMenu(env, env.chat)
		if _, keyboard := m.last(); len(settingsAllButtons(keyboard)) != 3 {
			t.Errorf("an empty list draws %d buttons, want Add, Back and Close", len(settingsAllButtons(keyboard)))
		}
		if _, keyboard := m.last(); keyboard != nil {
			if _, ok := settingsButtonByMarker(keyboard, "settings_btn_bl_action"); ok {
				t.Errorf("an empty list draws the Action button")
			}
		}
		m.press("a", settingsActBlacklistOpen)
		text, keyboard := m.last()
		if !strings.Contains(text, staffMarker("settings_bl_action_no_words")) || strings.Contains(text, staffMarker("settings_bl_action_hint")) {
			t.Errorf("a stale bo drew:\n%s", text)
		}
		if got := settingsListRowWidths(keyboard); fmt.Sprint(got) != fmt.Sprint([]int{2}) {
			t.Errorf("a stale bo has rows %v, want the navigation row alone", got)
		}
		m.press("a", settingsActBlacklistSet, "v", "ban")
		text, _ = m.last()
		if !strings.Contains(text, settingsNoticeWarnIcon+staffMarker("settings_bl_action_no_words")) {
			t.Errorf("a stale bs drew:\n%s", text)
		}
		if got := settingsBlacklistStored(t, env.chat.Id); len(got) != 0 {
			t.Errorf("a stale bs created words: %v", got)
		}
	})

	t.Run("a failed save redraws the stored state with the save-failed notice", func(t *testing.T) {
		env := newSettingsListEnv(t)
		settingsBlacklistSeedActions(t, env.chat.Id, "warn", "warn")
		m := settingsBlacklistOpenMenu(env, env.chat)
		setAction := blacklistWrites.SetAction
		blacklistWrites.SetAction = func(context.Context, int64, string) (int64, error) { return 0, errors.New("update failed") }
		t.Cleanup(func() { blacklistWrites.SetAction = setAction })
		m.press("a", settingsActBlacklistSet, "v", "ban")
		text, _ := m.last()
		if !strings.Contains(text, settingsNoticeWarnIcon+staffMarker("common_settings_save_failed")) || !strings.Contains(text, actionLine(staffMarker("settings_opt_warn"))) {
			t.Errorf("a failed save drew:\n%s", text)
		}
	})

	t.Run("bs and bo with an invalid value or from a group menu are invalid requests before any lookup", func(t *testing.T) {
		env := newSettingsListEnv(t)
		settingsBlacklistSeedActions(t, env.chat.Id, "warn")
		m := settingsBlacklistOpenMenu(env, env.chat)
		groupMsg := env.openMenu(env.admin)
		check := func(name string, press func()) {
			t.Helper()
			members := len(env.fake.callsFor("getChatMember"))
			press()
			if text, alert := env.lastAnswer(); !strings.Contains(text, staffMarker("common_callback_invalid_request")) || !alert {
				t.Errorf("%s answered %q (alert %v), want the invalid request alert", name, text, alert)
			}
			if got := len(env.fake.callsFor("getChatMember")) - members; got != 0 {
				t.Errorf("%s made %d getChatMember calls, want none", name, got)
			}
		}
		for _, value := range []string{"Ban", "tban", "delete", "tmute", "WARN", "", "ban "} {
			kv := []string{"a", settingsActBlacklistSet, "v", value}
			if value == "" {
				kv = kv[:2]
			}
			check("bs v="+value, func() { m.press(kv...) })
		}
		check("bo with a v", func() { m.press("a", settingsActBlacklistOpen, "v", "ban") })
		check("bo from a group menu", func() { env.press(env.admin, groupMsg, env.groupFields(env.admin, "a", settingsActBlacklistOpen)) })
		check("bs from a group menu", func() {
			env.press(env.admin, groupMsg, env.groupFields(env.admin, "a", settingsActBlacklistSet, "v", "ban"))
		})
		if got := settingsBlacklistStored(t, env.chat.Id); got["w01"] != "warn" {
			t.Errorf("a forged bs changed the action: %v", got)
		}
	})
}

func TestSettingsParityBlacklistAction(t *testing.T) {
	env := newSettingsListEnv(t)
	pageChat := settingsBlacklistWithBot(env, staffRoleAdmin)
	commandChat := settingsBlacklistWithBot(env, staffRoleAdmin)
	for _, chat := range []gotgbot.Chat{pageChat, commandChat} {
		settingsBlacklistSeedActions(t, chat.Id, "warn", "mute", "kick")
		blacklists.GetBlacklistSettingsContext(context.Background(), chat.Id)
	}
	m := settingsBlacklistOpenMenu(env, pageChat)
	m.tap(m.button(staffMarker("settings_btn_bl_action")))
	m.tap(m.button(staffMarker("settings_opt_ban")))
	env.sendIn(commandChat, env.admin, "/blacklistaction ban")

	pageRows, commandRows := settingsBlacklistStored(t, pageChat.Id), settingsBlacklistStored(t, commandChat.Id)
	if !maps.Equal(pageRows, commandRows) || pageRows["w01"] != "ban" || pageRows["w03"] != "ban" {
		t.Errorf("the page left %v and the command %v, want the same rows, all on ban", pageRows, commandRows)
	}
	pageKey, commandKey := dbcache.CacheKey("blacklist", pageChat.Id), dbcache.CacheKey("blacklist", commandChat.Id)
	if env.redis.Exists(pageKey) || env.redis.Exists(commandKey) {
		t.Errorf("cache state differs from dropped: page key present %v, command key present %v", env.redis.Exists(pageKey), env.redis.Exists(commandKey))
	}
}

func TestSettingsDriftBlacklist(t *testing.T) {
	env := newSettingsListEnv(t)
	m := settingsBlacklistOpenMenu(env, env.chat)
	draw := func() string {
		t.Helper()
		m.press("a", settingsActOpen, "p", settingsPageBlacklist)
		text, _ := m.last()
		return text
	}

	env.send(env.admin, "/addblacklist spam scam")
	text := draw()
	for _, want := range []string{"1. <code>scam</code>", "2. <code>spam</code>", staffMarker("settings_bl_count") + " 2"} {
		if !strings.Contains(text, want) {
			t.Errorf("a word added by /addblacklist is not drawn (%q):\n%s", want, text)
		}
	}
	env.send(env.admin, "/blacklistaction mute")
	if text = draw(); !strings.Contains(text, staffMarker("settings_bl_action_line")+" "+staffMarker("settings_opt_mute")) {
		t.Errorf("an action set by /blacklistaction is not drawn:\n%s", text)
	}
	// A word added now takes the action every word shares (A19-3), so the list stays uniform.
	env.send(env.admin, "/addblacklist fraud")
	if text = draw(); !strings.Contains(text, staffMarker("settings_bl_action_line")+" "+staffMarker("settings_opt_mute")) {
		t.Errorf("a word added after the action is not on the shared action:\n%s", text)
	}
	// A mixed list and an inert stored value written by hand are drawn as they are.
	settingsListSeedBlacklistAction(t, env.chat.Id, "manual", "tban")
	if text = draw(); !strings.Contains(text, staffMarker("settings_bl_action_line")+" "+staffMarker("settings_bl_action_mixed")) {
		t.Errorf("a hand-written mixed list is not drawn as Mixed:\n%s", text)
	}
	env.send(env.admin, "/rmblacklist spam")
	if text = draw(); strings.Contains(text, "<code>spam</code>") {
		t.Errorf("a word removed by /rmblacklist is still drawn:\n%s", text)
	}
}

// settingsBlacklistRowsOf returns the chat's stored rows as word -> action/reason, in one comparable map.
func settingsBlacklistRowsOf(t *testing.T, chatID int64) map[string]string {
	t.Helper()
	rows, err := blacklists.ListBlacklistEntriesFresh(context.Background(), chatID)
	if err != nil {
		t.Fatalf("list the blacklist: %v", err)
	}
	stored := make(map[string]string, len(rows))
	for _, row := range rows {
		stored[row.Word] = row.Action + "/" + row.Reason
	}
	return stored
}

func TestSettingsParityBlacklistAdd(t *testing.T) {
	env := newSettingsListEnv(t)
	cases := []struct {
		name   string
		seeded []string
		typed  string
		want   map[string]string
	}{
		{name: "two words on a chat whose words are on ban take the shared action", seeded: []string{"alpha"}, typed: "Spam scam",
			want: map[string]string{"alpha": "ban", "spam": "ban", "scam": "ban"}},
		{name: "a one-character word is refused by both and the other word is stored", seeded: []string{"alpha"}, typed: "a spam",
			want: map[string]string{"alpha": "ban", "spam": "ban"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pageChat := settingsBlacklistWithBot(env, staffRoleAdmin)
			commandChat := settingsBlacklistWithBot(env, staffRoleAdmin)
			for _, chat := range []gotgbot.Chat{pageChat, commandChat} {
				for _, word := range tc.seeded {
					settingsListSeedBlacklistAction(t, chat.Id, word, "ban")
				}
				blacklists.GetBlacklistSettingsContext(context.Background(), chat.Id)
				if !env.redis.Exists(dbcache.CacheKey("blacklist", chat.Id)) {
					t.Fatalf("the cache key of chat %d was not primed", chat.Id)
				}
			}

			settingsListDoorWait(env, pageChat, env.admin)
			settingsListDoorSay(env, env.admin, tc.typed)
			env.sendIn(commandChat, env.admin, "/addblacklist "+tc.typed)

			pageRows, commandRows := settingsBlacklistRowsOf(t, pageChat.Id), settingsBlacklistRowsOf(t, commandChat.Id)
			if !maps.Equal(pageRows, commandRows) {
				t.Errorf("the typed reply left %v and the command %v, want the same rows", pageRows, commandRows)
			}
			stored := settingsBlacklistStored(t, pageChat.Id)
			if !maps.Equal(stored, tc.want) {
				t.Errorf("the typed reply left %v, want %v", stored, tc.want)
			}
			for _, chat := range []gotgbot.Chat{pageChat, commandChat} {
				if env.redis.Exists(dbcache.CacheKey("blacklist", chat.Id)) {
					t.Errorf("the cache key of chat %d survived the write", chat.Id)
				}
			}
		})
	}
}

func TestSettingsAuthorityParityBlacklist(t *testing.T) {
	type scenario struct {
		name string
		// botRole is the bot's role in the chat; member is the presser's member record (nil is the env's administrator, who
		// can restrict, as the creator of the cached list).
		botRole string
		member  *staffFakeMember
		// wantKey is the refusal the menu answers, alert or notice; empty means the menu writes.
		wantKey   string
		wantAlert bool
		// typedKey is the key a typed reply is answered with when the same records refuse it; empty means it writes.
		typedKey string
		// ownerGate puts a live owner in the group with the switch off, so the gate refuses the administrator.
		ownerGate bool
	}
	scenarios := []scenario{
		{name: "the bot is a member", botRole: staffRoleMember, wantKey: "connections_is_user_connected_bot_not_admin",
			typedKey: "connections_is_user_connected_bot_not_admin"},
		{name: "the bot is an administrator without can_restrict_members", botRole: staffRoleAdminNoRestrict, wantKey: "chat_status_bot_restrict_error",
			typedKey: "chat_status_bot_restrict_group_error"},
		{name: "an administrator without can_restrict_members", botRole: staffRoleAdmin,
			member: &staffFakeMember{Status: gotgbot.ChatMemberStatusAdministrator}, wantKey: "settings_no_restrict", wantAlert: true,
			typedKey: "settings_no_restrict"},
		{name: "a plain member", botRole: staffRoleAdmin,
			member: &staffFakeMember{Status: gotgbot.ChatMemberStatusMember}, wantKey: "settings_not_admin", wantAlert: true,
			typedKey: "settings_edit_refuse_access_lost"},
		{name: "the owner gate refuses a non-owner administrator", botRole: staffRoleAdmin, wantKey: "settings_gate_owner_only", wantAlert: true, ownerGate: true,
			typedKey: "settings_gate_owner_only"},
		{name: "every check held", botRole: staffRoleAdmin},
	}

	env := newSettingsListEnv(t)
	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			chat := settingsBlacklistWithBot(env, sc.botRole, "alpha", "beta")
			user := env.admin
			if sc.member != nil {
				user = gotgbot.User{Id: uniqueLinkOwnerID(), FirstName: "Presser"}
				env.fake.setMember(chat.Id, user.Id, *sc.member)
			}
			if sc.ownerGate {
				env.newOwner(chat)
				env.setAdminsAllowed(chat.Id, false)
			}
			alphaID := settingsBlacklistIDOf(t, chat.Id, "alpha")

			// The commands first: they refuse for the same member and bot records, or write.
			commandWrites := sc.wantKey == "" || sc.ownerGate
			env.sendIn(chat, user, "/blacklistaction mute")
			env.sendIn(chat, user, "/addblacklist gamma")
			stored := settingsBlacklistStored(t, chat.Id)
			if commandWrites != (stored["gamma"] != "" && stored["alpha"] == "mute") {
				t.Errorf("the commands' writes = %v, want %v (stored %v)", !commandWrites, commandWrites, stored)
			}
			// The typed reply is judged like a press. A wait is written directly, because the Add press itself is refused
			// when the records refuse; the door then reads the same records live. Where it writes, the command wrote too
			// (the commands ran above); the owner gate is the documented carve-out, which gates the menu and not the command.
			if _, err := settingsInputSet(context.Background(), user.Id, settingsInputWait{
				GroupID: chat.Id, Page: settingsPageBlacklist, Part: "w", PromptMsgID: 1, Nonce: "0123456789abcdef",
			}); err != nil {
				t.Fatalf("write the wait: %v", err)
			}
			typedBefore := len(env.fake.sentTo(user.Id))
			settingsListDoorSay(env, user, "typedword")
			typedStored := settingsBlacklistStored(t, chat.Id)
			if sc.typedKey != "" {
				if got := fmt.Sprint(env.fake.sentTo(user.Id)[typedBefore].Params["text"]); !strings.Contains(got, staffMarker(sc.typedKey)) {
					t.Errorf("the typed reply was answered %q, want %s", got, sc.typedKey)
				}
				if typedStored["typedword"] != "" {
					t.Errorf("a refused typed reply wrote: %v", typedStored)
				}
				if env.redis.Exists(settingsInputKey(user.Id)) {
					t.Errorf("the refused typed reply left the wait")
				}
			} else if typedStored["typedword"] == "" {
				t.Errorf("the typed reply was not stored where the commands write: %v", typedStored)
			}
			if sc.typedKey != "" && sc.typedKey != "settings_gate_owner_only" && commandWrites {
				t.Errorf("a typed reply was refused where the commands write (%s), and that is not the owner-gate carve-out", sc.typedKey)
			}
			before := settingsBlacklistStored(t, chat.Id)

			m := settingsBlacklistOpenMenuAs(env, chat, user)
			presses := map[string][]string{
				"the delete ask": {"a", "bx", "r", strconv.FormatInt(alphaID, 10), "v", "1"},
				"the delete Yes": {"a", "by", "r", strconv.FormatInt(alphaID, 10), "e", settingsCardExpiry(time.Now())},
				"the action set": {"a", settingsActBlacklistSet, "v", "ban"},
			}
			for _, name := range []string{"the delete ask", "the delete Yes", "the action set"} {
				answers, edits := env.answerCount(), len(env.edits(user.Id, m.msgID))
				m.press(presses[name]...)
				if got := env.answerCount() - answers; got != 1 {
					t.Errorf("%s: answered %d times, want once", name, got)
				}
				switch {
				case sc.wantKey == "":
					continue
				case sc.wantAlert:
					if text, alert := env.lastAnswer(); !strings.Contains(text, staffMarker(sc.wantKey)) || !alert {
						t.Errorf("%s: answered %q (alert %v), want the alert %s", name, text, alert, sc.wantKey)
					}
					if got := len(env.edits(user.Id, m.msgID)); got != edits {
						t.Errorf("%s: an alert refusal edited the menu", name)
					}
				default:
					if text, alert := env.lastAnswer(); text != "" || alert {
						t.Errorf("%s: a notice refusal answered %q (alert %v), want an empty answer", name, text, alert)
					}
					if text, _ := m.last(); !strings.Contains(text, settingsNoticeWarnIcon+staffMarker(sc.wantKey)) {
						t.Errorf("%s: the redraw lacks the notice %s:\n%s", name, sc.wantKey, text)
					}
				}
			}
			after := settingsBlacklistStored(t, chat.Id)
			if sc.wantKey != "" {
				if !maps.Equal(before, after) {
					t.Errorf("a refused press wrote: %v -> %v", before, after)
				}
				return
			}
			if after["alpha"] != "" || after["beta"] != "ban" {
				t.Errorf("the permitted presses left %v, want alpha deleted and the rest on ban", after)
			}
		})
	}
}

// settingsBlacklistWorstEntries makes n entries with 17-digit row IDs, the worst case the delete buttons carry.
func settingsBlacklistWorstEntries(n int) []settingsListEntry {
	entries := make([]settingsListEntry, 0, n)
	for i := 0; i < n; i++ {
		entries = append(entries, settingsListEntry{ID: 12345678901234500 + int64(i), Text: fmt.Sprintf("w%06d", i), Action: "ban"})
	}
	return entries
}

// settingsBlacklistWorstViews are the two menus at the worst-case IDs of the UI-SPEC's callback table: a 14-character chat
// ID in a private menu and a 10-digit opener in a group menu.
func settingsBlacklistWorstViews(tr *i18n.Translator) map[string]settingsView {
	base := settingsView{Tr: tr, GroupID: -1001234567890, GroupTitle: "Group"}
	private, group := base, base
	private.Private, private.OwnerKey, private.OwnerValue = true, "c", "-1001234567890"
	group.OwnerKey, group.OwnerValue = "u", "1234567890"
	return map[string]settingsView{"private": private, "group": group}
}

func TestSettingsListBlacklistCallbackBytes(t *testing.T) {
	withRealLocales(t)
	tr := i18n.MustNewTranslator("en")
	views := settingsBlacklistWorstViews(tr)
	private := views["private"]
	def := settingsBlacklistDef()
	// 79,992 entries are 9,999 pages: the pager cursor v reaches its four digits.
	worst := settingsBlacklistWorstEntries(settingsListPageSize * 9999)

	// sizes records the largest button of each action code on a screen.
	sizes := make(map[string]int)
	measure := func(name string, screen settingsScreen) {
		t.Helper()
		if screen.Omitted != 0 {
			t.Errorf("%s: %d buttons were omitted for their size, want none", name, screen.Omitted)
		}
		for _, button := range settingsAllButtons(screen.Keyboard.InlineKeyboard) {
			if button.CallbackData == "" {
				continue
			}
			if size := len(button.CallbackData); size > 64 {
				t.Errorf("%s: button %q carries %d bytes, over the 64-byte cap", name, button.Text, size)
			}
			fields := settingsButtonFields(t, button)
			key := fields["a"]
			if key == settingsActBlacklistSet {
				key += ":" + fields["v"]
			}
			sizes[key] = max(sizes[key], len(button.CallbackData))
		}
	}

	for name, page := range map[string]int{"first": 1, "middle": 5000, "page 9998": 9998, "last": 9999} {
		measure("list "+name, renderSettingsList(private, def, worst, page, ""))
	}
	measure("a one-entry list", renderSettingsList(private, def, worst[:1], 1, ""))
	measure("an empty list", renderSettingsList(private, def, nil, 1, ""))
	measure("the card", renderSettingsListCard(private, def, worst[0], 9999))
	measure("the uniform action screen", renderSettingsBlacklistAction(private, worst[:3], ""))
	mixed := slices.Clone(worst[:3])
	mixed[0].Action = "warn"
	measure("the mixed action screen", renderSettingsBlacklistAction(private, mixed, ""))
	measure("the empty action screen", renderSettingsBlacklistAction(private, nil, ""))
	measure("the group note", renderSettingsPrivateOnlyNote(views["group"], def.TitleKey, "FukuTestBot"))

	for code, want := range map[string]int{
		"bp": 40, "bx": 60, "by": 64, "bo": 33,
		settingsActBlacklistSet + ":warn": 40, settingsActBlacklistSet + ":mute": 40,
		settingsActBlacklistSet + ":kick": 40, settingsActBlacklistSet + ":none": 40, settingsActBlacklistSet + ":ban": 39,
	} {
		if got := sizes[code]; got != want {
			t.Errorf("the largest %s button is %d bytes, the UI-SPEC table says %d", code, got, want)
		}
	}
	if got := sizes[settingsActBack]; got != 33 {
		t.Errorf("Back is %d bytes in a private menu, want 33", got)
	}

	// Main's cell of the page and the group menu's Back and Close.
	cell := func(v settingsView) int {
		screen := renderSettingsMain(v, []settingsMainEntry{{Key: settingsPageBlacklist, Icon: def.Icon, Title: "Blacklist", State: "1", Known: true}}, "")
		for _, button := range settingsAllButtons(screen.Keyboard.InlineKeyboard) {
			if f := settingsButtonFields(t, button); f["a"] == settingsActOpen {
				return len(button.CallbackData)
			}
		}
		return 0
	}
	if got := cell(private); got != 37 {
		t.Errorf("the Main cell is %d bytes in a private menu, want 37", got)
	}
	if got := cell(views["group"]); got != 33 {
		t.Errorf("the Main cell is %d bytes in a group menu, want 33", got)
	}
	groupNote := renderSettingsPrivateOnlyNote(views["group"], def.TitleKey, "FukuTestBot")
	for _, button := range settingsAllButtons(groupNote.Keyboard.InlineKeyboard) {
		if button.CallbackData != "" && len(button.CallbackData) != 29 {
			t.Errorf("group note button %q is %d bytes, want 29", button.Text, len(button.CallbackData))
		}
	}

	t.Run("a row whose ID has 18 digits has no delete button and is counted", func(t *testing.T) {
		entries := settingsBlacklistWorstEntries(3)
		entries[1].ID = 123456789012345678
		screen := renderSettingsList(private, def, entries, 1, "")
		if screen.Omitted != 1 {
			t.Errorf("Omitted = %d, want 1", screen.Omitted)
		}
		if got := len(screen.Keyboard.InlineKeyboard[0]); got != 2 {
			t.Errorf("the delete row has %d buttons, want 2", got)
		}
	})
}

// settingsBlacklistLongest returns the n longest of texts by UTF-16 units.
func settingsBlacklistLongest(texts []string, n int) []string {
	sorted := slices.Clone(texts)
	slices.SortStableFunc(sorted, func(a, b string) int { return panelUTF16Len(b) - panelUTF16Len(a) })
	return sorted[:min(n, len(sorted))]
}

func TestSettingsListBlacklistPageFitsEveryLocale(t *testing.T) {
	withRealLocales(t)
	title := strings.Repeat("Ж", 64)
	ampersands := strings.Repeat("&", 100)
	def := settingsBlacklistDef()
	// The entries a middle page of the worst entries shows: 32 ampersands each, five UTF-16 units once escaped.
	entries := make([]settingsListEntry, 0, 24)
	for i := 0; i < 24; i++ {
		entries = append(entries, settingsListEntry{ID: int64(10 + i), Text: strings.Repeat("&", 32) + string(rune('a'+i)), Action: "ban"})
	}
	// Eight stored actions, one word each: the mixed screen's worst case.
	eight := make([]settingsListEntry, 0, 8)
	for i, action := range []string{"warn", "mute", "kick", "ban", "none", "tban", "tmute", "delete"} {
		eight = append(eight, settingsListEntry{ID: int64(i + 1), Text: fmt.Sprintf("w%d", i), Action: action})
	}
	widths := func(screen settingsScreen) string {
		return fmt.Sprint(settingsListRowWidths(screen.Keyboard.InlineKeyboard))
	}

	for _, code := range memberReqScreenLanguages {
		t.Run(code, func(t *testing.T) {
			tr := i18n.MustNewTranslator(code)
			views := settingsBlacklistWorstViews(tr)
			for name, v := range views {
				v.GroupTitle = title
				views[name] = v
			}
			private := views["private"]

			// A notice adds only its own length, so the two longest of the notices a page of this table can show hold the
			// worst case of every combination.
			notices := settingsBlacklistLongest([]string{
				settingsNoticeOK(settingsListEntryText(tr, "settings_bl_removed", "word", ampersands)),
				settingsNoticeWarn(settingsText(tr, "settings_card_expired")),
				settingsNoticeWarn(settingsText(tr, "settings_entries_already_removed")),
				settingsNoticeWarn(settingsText(tr, "common_settings_save_failed")),
				settingsNoticeWarn(settingsText(tr, "chat_status_bot_restrict_error")),
				settingsNoticeWarn(settingsText(tr, "connections_is_user_connected_bot_not_admin")),
			}, 2)

			check := func(name string, screen settingsScreen, wantWidths string) {
				t.Helper()
				if units := panelUTF16Len(screen.Text); units == 0 || units > 3800 {
					t.Errorf("%s is %d UTF-16 units, want 1..3800", name, units)
				}
				if screen.Omitted != 0 {
					t.Errorf("%s omitted %d buttons", name, screen.Omitted)
				}
				if got := widths(screen); got != wantWidths {
					t.Errorf("%s has row widths %s, want %s", name, got, wantWidths)
				}
				for _, button := range settingsAllButtons(screen.Keyboard.InlineKeyboard) {
					if label := []rune(button.Text); len(label) > 20 && !strings.HasPrefix(button.Text, "🗑") && button.Url == "" {
						t.Errorf("%s: button %q has %d code points, want at most 20", name, button.Text, len(label))
					}
				}
			}
			for i, notice := range notices {
				check(fmt.Sprintf("the list page with notice %d", i), renderSettingsList(private, def, entries, 2, notice), "[4 4 2 2 2]")
				check(fmt.Sprintf("the mixed action screen with notice %d", i), renderSettingsBlacklistAction(private, eight, notice), "[3 2 2]")
			}
			card := renderSettingsListCard(private, def, settingsListEntry{ID: 12345678901234567, Text: ampersands}, 9999)
			check("the card", card, "[2 2]")
			if !strings.Contains(card.Text, strings.Repeat("&amp;", 100)) {
				t.Errorf("the card does not name the 100-code-point entry")
			}
			check("the group note", renderSettingsPrivateOnlyNote(views["group"], def.TitleKey, "FukuTestBot"), "[1 2]")
			check("the group note without a bot username", renderSettingsPrivateOnlyNote(views["group"], def.TitleKey, ""), "[2]")
		})
	}
}

func TestSettingsBlacklistReadsCreateNothing(t *testing.T) {
	setCachedBotUsername(t, "FukuTestBot")
	env := newSettingsListEnv(t)
	chat := env.addChat()
	settingsListCleanup(t, chat.Id)
	count := func() (rows, chats int64) {
		db.DB.Model(&models.BlacklistSettings{}).Where("chat_id = ?", chat.Id).Count(&rows)
		db.DB.Model(&models.Chat{}).Where("chat_id = ?", chat.Id).Count(&chats)
		return rows, chats
	}
	cacheKey := dbcache.CacheKey("blacklist", chat.Id)

	// A private menu: Main, the list page, the action screen and a forged ask on a chat with no rows.
	rows, chats := count()
	if rows != 0 {
		t.Fatalf("the fresh chat already has %d blacklist rows", rows)
	}
	m := settingsBlacklistOpenMenu(env, chat)
	m.press("a", settingsActBack)
	m.press("a", settingsActOpen, "p", settingsPageBlacklist)
	m.press("a", settingsActBlacklistOpen)
	m.press("a", "bx", "r", "42", "v", "1")
	m.press("a", settingsActBlacklistSet, "v", "ban")
	if text, _ := m.last(); !strings.Contains(text, staffMarker("settings_bl_action_no_words")) {
		t.Fatalf("the screens did not draw as expected:\n%s", text)
	}
	if gotRows, gotChats := count(); gotRows != 0 || gotChats != chats {
		t.Errorf("a private menu left %d blacklist rows and changed the chat rows from %d to %d", gotRows, chats, gotChats)
	}
	if env.redis.Exists(cacheKey) {
		t.Errorf("a private menu filled %s", cacheKey)
	}

	// A group menu: Main and the page's note read nothing and create nothing.
	groupMsg := env.openMenu(env.admin)
	env.press(env.admin, groupMsg, env.groupFields(env.admin, "a", settingsActOpen, "p", settingsPageBlacklist))
	if gotRows, _ := count(); gotRows != 0 || env.redis.Exists(cacheKey) {
		t.Errorf("a group menu left %d blacklist rows or filled %s", gotRows, cacheKey)
	}
}
