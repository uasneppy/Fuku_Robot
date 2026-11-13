//go:build testtools

package modules

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db"
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
