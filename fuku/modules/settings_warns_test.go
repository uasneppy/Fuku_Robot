//go:build testtools

package modules

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"

	"github.com/uasneppy/fuku_robot/fuku/db"
	dbcache "github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/db/warns"
)

// settingsWarnsCleanup deletes the chats' warn settings rows and cache keys when the test
// ends (and before it starts), then resets the local cache layer.
func settingsWarnsCleanup(t *testing.T, chatIDs ...int64) {
	t.Helper()
	clean := func() {
		db.DB.Where("chat_id IN ?", chatIDs).Delete(&models.WarnSettings{})
		for _, id := range chatIDs {
			dbcache.DeleteCache(dbcache.CacheKey("warn_settings", id))
		}
		dbcache.ResetLocalForTest()
	}
	clean()
	t.Cleanup(clean)
}

// settingsWarnsRow reads the warn settings row of chatID straight from the database, past
// every cache layer. ok is false when the chat has no row.
func settingsWarnsRow(t *testing.T, chatID int64) (row models.WarnSettings, ok bool) {
	t.Helper()
	if err := db.DB.Where("chat_id = ?", chatID).Limit(1).Find(&row).Error; err != nil {
		t.Fatalf("read warns_settings of %d: %v", chatID, err)
	}
	return row, row.ID != 0
}

// settingsWarnsSeed writes a warn settings row directly, as a database edit would.
func settingsWarnsSeed(t *testing.T, chatID int64, limit int, mode string) {
	t.Helper()
	if err := db.DB.Create(&models.WarnSettings{ChatId: chatID, WarnLimit: limit, WarnMode: mode}).Error; err != nil {
		t.Fatalf("seed warns_settings of %d: %v", chatID, err)
	}
	dbcache.DeleteCache(dbcache.CacheKey("warn_settings", chatID))
	dbcache.ResetLocalForTest()
}

// settingsWarnsPress hand-builds a press of the Warnings page on message msgID of chat, as
// the env's administrator.
func settingsWarnsPress(t *testing.T, env *settingsEnv, chat gotgbot.Chat, msgID int64, kv ...string) {
	t.Helper()
	fields := env.groupFields(env.admin, kv...)
	data := encodeCallbackData(settingsCallbackNamespace, fields)
	if data == "" {
		t.Fatalf("hand-built press %v did not encode", fields)
	}
	env.pressRaw(chat, env.admin, msgID, data)
}

// settingsWarnsCommand runs /setwarnlimit, /setwarnmode or /warnings as the env's
// administrator in chat, through the command's own handler (the settings environment does not
// load the Warns module).
func settingsWarnsCommand(env *settingsEnv, chat gotgbot.Chat, text string) {
	env.t.Helper()
	handler := warnsModule.setWarnLimit
	switch {
	case strings.HasPrefix(text, "/setwarnmode"):
		handler = warnsModule.setWarnMode
	case strings.HasPrefix(text, "/warnings"):
		handler = warnsModule.warnings
	}
	ctx := newModuleMessageContext(env.bot, chat, env.admin, text)
	if err := handler(env.bot, ctx); !errors.Is(err, ext.EndGroups) {
		env.t.Fatalf("%s: error = %v, want EndGroups", text, err)
	}
}

// settingsWarnsCached reports the warn settings the command's cached read returns for chatID
// and whether the cache holds a key for the chat.
func settingsWarnsCached(env *settingsEnv, chatID int64) (limit int, mode string, keyed bool) {
	for _, key := range env.redis.Keys() {
		if strings.Contains(key, "warn_settings") && strings.Contains(key, fmt.Sprint(chatID)) {
			keyed = true
		}
	}
	stored := warns.GetWarnSettingContext(context.Background(), chatID)
	return stored.WarnLimit, stored.WarnMode, keyed
}

// settingsWarnsStepper returns the five buttons of the limit stepper on the current page of
// msgID, after checking the page's layout: the stepper, the three actions, Back and Close.
func settingsWarnsStepper(t *testing.T, env *settingsEnv, msgID int64) []gotgbot.InlineKeyboardButton {
	t.Helper()
	_, keyboard := env.lastEdit(env.chat.Id, msgID)
	if len(keyboard) != 3 || len(keyboard[0]) != 5 || len(keyboard[1]) != 3 || len(keyboard[2]) != 2 {
		t.Fatalf("Warnings page keyboard = %v, want rows of 5, 3 and 2 buttons", settingsWarnsWidths(keyboard))
	}
	return keyboard[0]
}

// settingsWarnsWidths is the button count of every keyboard row.
func settingsWarnsWidths(keyboard [][]gotgbot.InlineKeyboardButton) []int {
	widths := make([]int, len(keyboard))
	for i, row := range keyboard {
		widths[i] = len(row)
	}
	return widths
}

func TestSettingsWarnsOpens(t *testing.T) {
	t.Run("in a group menu", func(t *testing.T) {
		env := newSettingsEnv(t)
		settingsWarnsCleanup(t, env.chat.Id)
		msgID := env.openMenu(env.admin)
		mainText, mainKeyboard := env.lastEdit(env.chat.Id, msgID)

		line, found := settingsMainStateLine(mainText, "settings_page_warns")
		if !found || !strings.Contains(line, "🚩") || !strings.Contains(line, staffMarker("settings_state_warns")) {
			t.Errorf("Main status line = %q (found %v), want 🚩 and the Limit, action state", line, found)
		}
		button, ok := settingsPageButton(t, mainKeyboard, settingsPageWarns)
		if !ok || !strings.HasPrefix(button.Text, "🚩 ") || !containsMarker(button.Text, "settings_page_warns") {
			t.Fatalf("Main has no 🚩 Warnings button: %+v", mainKeyboard)
		}
		env.pressRaw(env.chat, env.admin, msgID, button.CallbackData)

		text, keyboard := env.lastEdit(env.chat.Id, msgID)
		for _, want := range []string{
			staffMarker("settings_header") + " " + staffMarker("settings_page_warns"),
			staffMarker("settings_warns_limit") + " 3",
			staffMarker("settings_warns_hint"),
		} {
			if !strings.Contains(text, want) {
				t.Errorf("Warnings page %q does not contain %q", text, want)
			}
		}
		stepper := settingsWarnsStepper(t, env, msgID)
		for i, wantLabel := range []string{"−10", "−1", "3", "+1", "+10"} {
			if stepper[i].Text != wantLabel {
				t.Errorf("stepper button %d = %q, want %q", i, stepper[i].Text, wantLabel)
			}
		}
		if len(keyboard[2]) != 2 {
			t.Errorf("navigation row has %d buttons, want Back and Close", len(keyboard[2]))
		}
		if !strings.Contains(text, staffMarker("settings_warns_action")+" "+staffMarker("settings_opt_mute")) {
			t.Errorf("Warnings page %q does not show the default action Mute", text)
		}
		if mute := keyboard[1][0]; !strings.HasPrefix(mute.Text, "✅ ") || !containsMarker(mute.Text, "settings_opt_mute") {
			t.Errorf("first choice = %q, want Mute marked", mute.Text)
		}
		if _, has := settingsWarnsRow(t, env.chat.Id); has {
			t.Error("opening Main and the page created a warns_settings row")
		}
	})

	t.Run("in a private menu", func(t *testing.T) {
		env := newSettingsEnv(t)
		settingsWarnsCleanup(t, env.chat.Id)
		owner := env.newOwner(env.chat)
		env.sendPrivate(owner, fmt.Sprintf("/start settings_%d", env.chat.Id))
		sentMain := settingsPrivateOnlySent(t, env, owner)
		msgID := sentMain.MessageID
		mainKeyboard := staffKeyboardOf(sentMain.Params["reply_markup"])

		button, ok := settingsPageButton(t, mainKeyboard, settingsPageWarns)
		if !ok {
			t.Fatalf("the private Main has no Warnings button: %+v", mainKeyboard)
		}
		env.pressRaw(settingsPrivateChat(owner), owner, msgID, button.CallbackData)
		text, keyboard := env.lastEdit(owner.Id, msgID)
		if !strings.Contains(text, staffMarker("settings_warns_limit")+" 3") || len(keyboard) != 3 || len(keyboard[0]) != 5 {
			t.Errorf("private Warnings page = %q with %d rows, want the limit line and the stepper", text, len(keyboard))
		}
		fields := settingsButtonFields(t, keyboard[0][3])
		if fields["c"] != fmt.Sprint(env.chat.Id) || fields["a"] != "st" || fields["t"] != "l" || fields["v"] != "4" {
			t.Errorf("+1 button fields = %v, want a=st t=l v=4 and the group in c", fields)
		}
		if _, has := settingsWarnsRow(t, env.chat.Id); has {
			t.Error("a private open created a warns_settings row")
		}
	})

	t.Run("a stored row shows its limit and mode", func(t *testing.T) {
		env := newSettingsEnv(t)
		settingsWarnsCleanup(t, env.chat.Id)
		settingsWarnsSeed(t, env.chat.Id, 8, "ban")
		msgID := env.openMenu(env.admin)
		mainText, _ := env.lastEdit(env.chat.Id, msgID)
		line, found := settingsMainStateLine(mainText, "settings_page_warns")
		if !found || !strings.Contains(line, "8") || !strings.Contains(line, staffMarker("settings_opt_ban")) {
			t.Errorf("Main status line = %q (found %v), want limit 8 and the Ban word", line, found)
		}
	})

	t.Run("a failing read is the cannot-open edit and the unknown mark", func(t *testing.T) {
		env := newSettingsEnv(t)
		settingsWarnsCleanup(t, env.chat.Id)
		msgID := env.openMenu(env.admin)
		_, mainKeyboard := env.lastEdit(env.chat.Id, msgID)
		button, ok := settingsPageButton(t, mainKeyboard, settingsPageWarns)
		if !ok {
			t.Fatal("Main has no Warnings button")
		}

		previous := settingsWarnsRead
		settingsWarnsRead = func(context.Context, int64) (*models.WarnSettings, error) {
			return nil, errors.New("database is down")
		}
		t.Cleanup(func() { settingsWarnsRead = previous })

		env.pressRaw(env.chat, env.admin, msgID, button.CallbackData)
		text, _ := env.lastEdit(env.chat.Id, msgID)
		if !strings.Contains(text, staffMarker("settings_cannot_open")) {
			t.Errorf("a failed read drew %q, want the cannot-open edit", text)
		}
		if answer, alert := env.lastAnswer(); !alert || !strings.Contains(answer, staffMarker("settings_cannot_open_alert")) {
			t.Errorf("answer = %q (alert %v), want the cannot-open alert", answer, alert)
		}

		msgID = env.openMenu(env.admin)
		mainText, _ := env.lastEdit(env.chat.Id, msgID)
		line, found := settingsMainStateLine(mainText, "settings_page_warns")
		if !found || !strings.HasSuffix(line, settingsUnknownState) {
			t.Errorf("Main status line = %q (found %v), want the unknown mark", line, found)
		}
	})
}

func TestSettingsParityWarnLimit(t *testing.T) {
	const menuMsgID = 6161

	// run sets the same value on two chats, one through /setwarnlimit and one through the
	// page, and requires the same stored row and the same cached state.
	run := func(t *testing.T, seed bool, target int, taps int) {
		t.Helper()
		env := newSettingsEnv(t)
		commandChat := env.chat
		menuChat := env.addChat()
		settingsWarnsCleanup(t, commandChat.Id, menuChat.Id)
		if seed {
			settingsWarnsSeed(t, commandChat.Id, 3, "ban")
			settingsWarnsSeed(t, menuChat.Id, 3, "ban")
		}

		settingsWarnsCommand(env, commandChat, fmt.Sprintf("/setwarnlimit %d", target))
		for range taps {
			settingsWarnsPress(t, env, menuChat, menuMsgID, "a", "st", "p", "n", "t", "l", "v", fmt.Sprint(target))
		}

		commandRow, ok := settingsWarnsRow(t, commandChat.Id)
		if !ok {
			t.Fatal("/setwarnlimit stored no row")
		}
		menuRow, ok := settingsWarnsRow(t, menuChat.Id)
		if !ok {
			t.Fatal("the page's stepper stored no row")
		}
		wantMode := "mute"
		if seed {
			wantMode = "ban"
		}
		if commandRow.WarnLimit != target || commandRow.WarnMode != wantMode {
			t.Errorf("/setwarnlimit stored limit %d mode %q, want %d and %q", commandRow.WarnLimit, commandRow.WarnMode, target, wantMode)
		}
		if menuRow.WarnLimit != commandRow.WarnLimit || menuRow.WarnMode != commandRow.WarnMode {
			t.Errorf("the page stored limit %d mode %q, the command limit %d mode %q, want the same row",
				menuRow.WarnLimit, menuRow.WarnMode, commandRow.WarnLimit, commandRow.WarnMode)
		}
		cl, cm, ck := settingsWarnsCached(env, commandChat.Id)
		ml, mm, mk := settingsWarnsCached(env, menuChat.Id)
		if cl != ml || cm != mm || ck != mk {
			t.Errorf("cache state differs: command (%d, %q, key %v), page (%d, %q, key %v)", cl, cm, ck, ml, mm, mk)
		}
		text, _ := env.lastEdit(menuChat.Id, menuMsgID)
		if !strings.Contains(text, staffMarker("settings_warns_limit")+" "+fmt.Sprint(target)) {
			t.Errorf("the page redrew %q, want the limit %d from the read-back", text, target)
		}
	}

	t.Run("a chat with no row", func(t *testing.T) { run(t, false, 4, 1) })
	t.Run("a chat with a row", func(t *testing.T) { run(t, true, 13, 1) })
	t.Run("a double tap leaves the same state", func(t *testing.T) { run(t, true, 13, 2) })

	t.Run("a step at the highest limit writes nothing", func(t *testing.T) {
		env := newSettingsEnv(t)
		settingsWarnsCleanup(t, env.chat.Id)
		settingsWarnsSeed(t, env.chat.Id, 100, "kick")
		answers := env.answerCount()
		settingsWarnsPress(t, env, env.chat, menuMsgID, "a", "st", "p", "n", "t", "l", "v", "100")
		if got := env.answerCount() - answers; got != 1 {
			t.Errorf("the press answered %d times, want once", got)
		}
		if answer, alert := env.lastAnswer(); alert || !strings.Contains(answer, staffMarker("settings_max_value")) {
			t.Errorf("answer = %q (alert %v), want the max toast", answer, alert)
		}
		if row, _ := settingsWarnsRow(t, env.chat.Id); row.WarnLimit != 100 || row.WarnMode != "kick" {
			t.Errorf("row = %+v, want it untouched", row)
		}
	})

	t.Run("a refused value redraws the stored state with the command's text", func(t *testing.T) {
		env := newSettingsEnv(t)
		settingsWarnsCleanup(t, env.chat.Id)
		settingsWarnsSeed(t, env.chat.Id, 5, "ban")
		for _, v := range []string{"0", "101"} {
			settingsWarnsPress(t, env, env.chat, menuMsgID, "a", "st", "p", "n", "t", "l", "v", v)
			text, _ := env.lastEdit(env.chat.Id, menuMsgID)
			if !strings.Contains(text, staffMarker("warns_limit_range_error")) || !strings.Contains(text, staffMarker("settings_warns_limit")+" 5") {
				t.Errorf("v=%s redrew %q, want the range notice over the stored limit 5", v, text)
			}
			if row, _ := settingsWarnsRow(t, env.chat.Id); row.WarnLimit != 5 {
				t.Errorf("v=%s changed the stored limit to %d", v, row.WarnLimit)
			}
		}
	})

	t.Run("a bot that is not an admin is refused like the command", func(t *testing.T) {
		env := newSettingsEnv(t)
		chat := env.addChat()
		settingsWarnsCleanup(t, chat.Id)
		env.fake.setBotRole(chat.Id, staffRoleMember)
		refusal := staffMarker("connections_is_user_connected_bot_not_admin")

		settingsWarnsCommand(env, chat, "/setwarnlimit 5")
		refusedByCommand := false
		for _, sent := range env.fake.sentTo(chat.Id) {
			if strings.Contains(fmt.Sprint(sent.Params["text"]), refusal) {
				refusedByCommand = true
			}
		}
		if !refusedByCommand {
			t.Fatal("/setwarnlimit 5 was not refused for a bot that is not an admin")
		}

		answers := env.answerCount()
		settingsWarnsPress(t, env, chat, menuMsgID, "a", "st", "p", "n", "t", "l", "v", "6")
		text, _ := env.lastEdit(chat.Id, menuMsgID)
		if !strings.Contains(text, refusal) {
			t.Errorf("the press drew %q, want the notice %s", text, refusal)
		}
		if got := env.answerCount() - answers; got != 1 {
			t.Errorf("the press answered %d times, want once", got)
		}
		if _, has := settingsWarnsRow(t, chat.Id); has {
			t.Error("a refused press or command wrote a row")
		}
	})
}

func TestSettingsParityWarnMode(t *testing.T) {
	const menuMsgID = 6262

	// run sets the same action on two chats, one through /setwarnmode and one through the
	// page, and requires the same stored row and the same cached state.
	run := func(t *testing.T, seedLimit int, mode string, taps int) {
		t.Helper()
		env := newSettingsEnv(t)
		commandChat := env.chat
		menuChat := env.addChat()
		settingsWarnsCleanup(t, commandChat.Id, menuChat.Id)
		wantLimit := 3
		if seedLimit != 0 {
			settingsWarnsSeed(t, commandChat.Id, seedLimit, "mute")
			settingsWarnsSeed(t, menuChat.Id, seedLimit, "mute")
			wantLimit = seedLimit
		}

		settingsWarnsCommand(env, commandChat, "/setwarnmode "+mode)
		for range taps {
			settingsWarnsPress(t, env, menuChat, menuMsgID, "a", "st", "p", "n", "t", "m", "v", mode)
		}

		commandRow, ok := settingsWarnsRow(t, commandChat.Id)
		if !ok {
			t.Fatal("/setwarnmode stored no row")
		}
		menuRow, ok := settingsWarnsRow(t, menuChat.Id)
		if !ok {
			t.Fatal("the page's action choice stored no row")
		}
		if commandRow.WarnLimit != wantLimit || commandRow.WarnMode != mode {
			t.Errorf("/setwarnmode stored limit %d mode %q, want %d and %q", commandRow.WarnLimit, commandRow.WarnMode, wantLimit, mode)
		}
		if menuRow.WarnLimit != commandRow.WarnLimit || menuRow.WarnMode != commandRow.WarnMode {
			t.Errorf("the page stored limit %d mode %q, the command limit %d mode %q, want the same row",
				menuRow.WarnLimit, menuRow.WarnMode, commandRow.WarnLimit, commandRow.WarnMode)
		}
		cl, cm, ck := settingsWarnsCached(env, commandChat.Id)
		ml, mm, mk := settingsWarnsCached(env, menuChat.Id)
		if cl != ml || cm != mm || ck != mk {
			t.Errorf("cache state differs: command (%d, %q, key %v), page (%d, %q, key %v)", cl, cm, ck, ml, mm, mk)
		}

		text, keyboard := env.lastEdit(menuChat.Id, menuMsgID)
		if !strings.Contains(text, staffMarker("settings_warns_action")+" "+staffMarker("settings_opt_"+mode)) {
			t.Errorf("the page redrew %q, want the action %s from the read-back", text, mode)
		}
		if strings.Contains(text, "⚠️") {
			t.Errorf("the page redrew %q with a notice, want none", text)
		}
		marked := 0
		for _, button := range keyboard[1] {
			if strings.HasPrefix(button.Text, "✅ ") {
				marked++
				if !containsMarker(button.Text, "settings_opt_"+mode) {
					t.Errorf("marked choice = %q, want %s", button.Text, mode)
				}
			}
		}
		if marked != 1 {
			t.Errorf("%d choices are marked, want exactly the stored one", marked)
		}

		// /warnings and Main read the same row the page wrote.
		settingsWarnsCommand(env, menuChat, "/warnings")
		shown := false
		for _, sent := range env.fake.sentTo(menuChat.Id) {
			if text := fmt.Sprint(sent.Params["text"]); strings.Contains(text, staffMarker("warns_settings_display")) &&
				strings.Contains(text, mode) {
				shown = true
			}
		}
		if !shown {
			t.Errorf("/warnings in the menu's chat does not show the action %s", mode)
		}
		settingsWarnsPress(t, env, menuChat, menuMsgID, "a", "bk")
		mainText, _ := env.lastEdit(menuChat.Id, menuMsgID)
		line, found := settingsMainStateLine(mainText, "settings_page_warns")
		if !found || !strings.Contains(line, staffMarker("settings_opt_"+mode)) {
			t.Errorf("Main status line = %q (found %v), want the action %s", line, found, mode)
		}
	}

	t.Run("a chat with no row", func(t *testing.T) { run(t, 0, "ban", 1) })
	t.Run("a chat with a row", func(t *testing.T) { run(t, 7, "kick", 1) })
	t.Run("a double tap leaves the same state", func(t *testing.T) { run(t, 7, "ban", 2) })

	t.Run("a press of the stored value writes the same row and no notice", func(t *testing.T) {
		env := newSettingsEnv(t)
		settingsWarnsCleanup(t, env.chat.Id)
		settingsWarnsSeed(t, env.chat.Id, 7, "ban")
		settingsWarnsPress(t, env, env.chat, menuMsgID, "a", "st", "p", "n", "t", "m", "v", "ban")
		if row, _ := settingsWarnsRow(t, env.chat.Id); row.WarnLimit != 7 || row.WarnMode != "ban" {
			t.Errorf("row = %+v, want limit 7 mode ban", row)
		}
		text, _ := env.lastEdit(env.chat.Id, menuMsgID)
		if strings.Contains(text, "⚠️") || !strings.Contains(text, staffMarker("settings_warns_limit")+" 7") {
			t.Errorf("the page redrew %q, want the stored state with no notice", text)
		}
	})

	t.Run("a failed write redraws the stored state with the save-failed notice", func(t *testing.T) {
		env := newSettingsEnv(t)
		settingsWarnsCleanup(t, env.chat.Id)
		settingsWarnsSeed(t, env.chat.Id, 7, "ban")
		previous := warnSettingsWrites
		warnSettingsWrites.Mode = func(int64, string) error { return errors.New("write refused") }
		t.Cleanup(func() { warnSettingsWrites = previous })

		settingsWarnsPress(t, env, env.chat, menuMsgID, "a", "st", "p", "n", "t", "m", "v", "kick")
		text, _ := env.lastEdit(env.chat.Id, menuMsgID)
		if !strings.Contains(text, staffMarker("common_settings_save_failed")) ||
			!strings.Contains(text, staffMarker("settings_warns_action")+" "+staffMarker("settings_opt_ban")) {
			t.Errorf("the page redrew %q, want the save-failed notice over the stored action Ban", text)
		}
		if row, _ := settingsWarnsRow(t, env.chat.Id); row.WarnMode != "ban" {
			t.Errorf("stored mode = %q after a failed write, want ban", row.WarnMode)
		}
	})

	t.Run("the first limit write and the first action write store the other value's default", func(t *testing.T) {
		env := newSettingsEnv(t)
		limitChat := env.chat
		modeChat := env.addChat()
		settingsWarnsCleanup(t, limitChat.Id, modeChat.Id)
		settingsWarnsPress(t, env, limitChat, menuMsgID, "a", "st", "p", "n", "t", "l", "v", "9")
		settingsWarnsPress(t, env, modeChat, menuMsgID, "a", "st", "p", "n", "t", "m", "v", "kick")
		if row, _ := settingsWarnsRow(t, limitChat.Id); row.WarnLimit != 9 || row.WarnMode != "mute" {
			t.Errorf("first limit write stored %+v, want limit 9 and mode mute", row)
		}
		if row, _ := settingsWarnsRow(t, modeChat.Id); row.WarnLimit != 3 || row.WarnMode != "kick" {
			t.Errorf("first action write stored %+v, want limit 3 and mode kick", row)
		}
	})
}
