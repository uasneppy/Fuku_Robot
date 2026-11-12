//go:build testtools

package modules

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db"
	dbcache "github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/chats"
	"github.com/uasneppy/fuku_robot/fuku/db/logchannels"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// settingsLogCleanup deletes the log channel rows, the chats rows and the cached log channel copies
// of the given chats before the test and when it ends, then resets the local cache layer.
func settingsLogCleanup(t *testing.T, chatIDs ...int64) {
	t.Helper()
	clean := func() {
		db.DB.Where("chat_id IN ?", chatIDs).Delete(&models.LogChannel{})
		db.DB.Where("chat_id IN ?", chatIDs).Delete(&models.Chat{})
		for _, id := range chatIDs {
			dbcache.DeleteCache(dbcache.CacheKey("log_channel", id))
			dbcache.DeleteCache(dbcache.CacheKey("chat", id))
		}
		dbcache.ResetLocalForTest()
	}
	clean()
	t.Cleanup(clean)
}

// newSettingsLogEnv is the settings environment (marker locale) plus the log channel commands, so a
// parity test can run /log and /nolog next to the page. The env's chat has no log channel.
func newSettingsLogEnv(t *testing.T) *settingsEnv {
	t.Helper()
	env := newSettingsEnv(t)
	LoadLogChannels(env.dispatcher)
	settingsLogCleanup(t, env.chat.Id)
	return env
}

// settingsLogLink links a fresh log channel to chat through the real repository, with every
// category on, and returns the channel's ID.
func settingsLogLink(t *testing.T, chat gotgbot.Chat) int64 {
	t.Helper()
	channelID := uniqueModuleChatID()
	settingsLogCleanup(t, chat.Id, channelID)
	if err := chats.EnsureChatInDb(chat.Id, chat.Title); err != nil {
		t.Fatalf("ensure chat %d: %v", chat.Id, err)
	}
	if err := chats.EnsureChatInDb(channelID, "log channel"); err != nil {
		t.Fatalf("ensure channel %d: %v", channelID, err)
	}
	if err := logchannels.Set(chat.Id, chat.Title, channelID); err != nil {
		t.Fatalf("link log channel: %v", err)
	}
	return channelID
}

// settingsLogSeed turns the named categories of chat off (and every other one on) through the
// real repository.
func settingsLogSeed(t *testing.T, chatID int64, off ...string) {
	t.Helper()
	for _, name := range logchannels.AllCategories {
		if err := logchannels.SetCategory(chatID, name, !slices.Contains(off, name)); err != nil {
			t.Fatalf("seed category %s: %v", name, err)
		}
	}
}

// settingsLogToggle hand-builds a tg press of one category control with the absolute value v on
// message msgID of chat, as the env's administrator in a group menu.
func settingsLogToggle(t *testing.T, env *settingsEnv, chat gotgbot.Chat, msgID int64, control, v string) {
	t.Helper()
	fields := env.groupFields(env.admin, "a", "tg", "p", settingsPageLog, "t", control, "v", v)
	data := encodeCallbackData(settingsCallbackNamespace, fields)
	if data == "" {
		t.Fatalf("hand-built press %v did not encode", fields)
	}
	env.pressRaw(chat, env.admin, msgID, data)
}

// settingsLogOpenPage opens the Log channel page of the env's chat in a group menu and returns the
// message ID, the page text and its keyboard.
func settingsLogOpenPage(t *testing.T, env *settingsEnv) (int64, string, [][]gotgbot.InlineKeyboardButton) {
	t.Helper()
	msgID := env.openMenu(env.admin)
	_, mainKeyboard := env.lastEdit(env.chat.Id, msgID)
	button, ok := settingsPageButton(t, mainKeyboard, settingsPageLog)
	if !ok {
		t.Fatalf("Main has no Log channel button: %+v", mainKeyboard)
	}
	env.pressRaw(env.chat, env.admin, msgID, button.CallbackData)
	text, keyboard := env.lastEdit(env.chat.Id, msgID)
	return msgID, text, keyboard
}

// settingsLogCell is the grid button of the category named name (the repository name).
func settingsLogCell(t *testing.T, keyboard [][]gotgbot.InlineKeyboardButton, name string) gotgbot.InlineKeyboardButton {
	t.Helper()
	for _, cat := range settingsLogCategories {
		if cat.Name != name {
			continue
		}
		for _, button := range settingsAllButtons(keyboard) {
			if fields := settingsButtonFields(t, button); fields["a"] == "tg" && fields["p"] == settingsPageLog && fields["t"] == cat.Code {
				return button
			}
		}
	}
	t.Fatalf("the page has no cell for %s: %+v", name, keyboard)
	return gotgbot.InlineKeyboardButton{}
}

// settingsLogPosts is the messages the bot sent to the log channel, as text and parse mode.
func settingsLogPosts(env *settingsEnv, channelID int64) []string {
	var posts []string
	for _, sent := range env.fake.sentTo(channelID) {
		posts = append(posts, fmt.Sprintf("%v|%v", sent.Params["text"], sent.Params["parse_mode"]))
	}
	return posts
}

func TestSettingsLogChannelOpens(t *testing.T) {
	// wantPage checks the body and the keyboard of the page for a chat whose User actions
	// category is off and whose others are on.
	wantPage := func(t *testing.T, text string, keyboard [][]gotgbot.InlineKeyboardButton, private bool) {
		t.Helper()
		for _, want := range []string{
			staffMarker("settings_header") + " " + staffMarker("settings_page_log"),
			staffMarker("settings_log_status") + " " + staffMarker("settings_state_linked"),
			staffMarker("settings_main_state_line") + " ✅ " + staffMarker("settings_log_cat_admin") + " " + staffMarker("common_status_on"),
			staffMarker("settings_main_state_line") + " ❌ " + staffMarker("settings_log_cat_user") + " " + staffMarker("common_status_off"),
			staffMarker("settings_log_hint"),
		} {
			if !strings.Contains(text, want) {
				t.Errorf("Log channel page %q does not contain %q", text, want)
			}
		}
		if got := fmt.Sprint(settingsSwWidths(keyboard)); got != "[2 2 2 2]" {
			t.Errorf("page rows = %s, want [2 2 2 2] (three rows of two cells, then Back and Close)", got)
		}
		last := keyboard[len(keyboard)-1]
		if len(last) != 2 || !containsMarker(last[0].Text, "settings_btn_back") || !containsMarker(last[1].Text, "settings_btn_close") {
			t.Errorf("last row = %+v, want Back and Close", last)
		}
		for i, cat := range settingsLogCategories {
			cell := settingsLogCell(t, keyboard, cat.Name)
			wantIcon, wantValue := "✅ ", "0"
			if cat.Name == logchannels.CategoryUser {
				wantIcon, wantValue = "❌ ", "1"
			}
			if want := wantIcon + staffMarker(cat.NameKey); cell.Text != want {
				t.Errorf("cell %d label = %q, want %q", i, cell.Text, want)
			}
			fields := settingsButtonFields(t, cell)
			if fields["v"] != wantValue {
				t.Errorf("cell %s sends v=%s, want the opposite of what it shows (%s)", cat.Name, fields["v"], wantValue)
			}
			if private && (fields["c"] == "" || fields["u"] != "") || !private && (fields["u"] == "" || fields["c"] != "") {
				t.Errorf("cell %s owner fields = %v for private=%v", cat.Name, fields, private)
			}
		}
	}

	t.Run("in a group menu", func(t *testing.T) {
		env := newSettingsLogEnv(t)
		settingsLogLink(t, env.chat)
		settingsLogSeed(t, env.chat.Id, logchannels.CategoryUser)
		msgID := env.openMenu(env.admin)
		mainText, _ := env.lastEdit(env.chat.Id, msgID)
		line, found := settingsMainStateLine(mainText, "settings_page_log")
		if !found || !strings.Contains(line, "🧾") || !strings.Contains(line, staffMarker("settings_state_linked")) {
			t.Errorf("Main status line = %q (found %v), want 🧾 and Linked", line, found)
		}
		_, text, keyboard := settingsLogOpenPage(t, env)
		wantPage(t, text, keyboard, false)
	})

	t.Run("in a private menu", func(t *testing.T) {
		env := newSettingsLogEnv(t)
		settingsLogLink(t, env.chat)
		settingsLogSeed(t, env.chat.Id, logchannels.CategoryUser)
		owner := env.newOwner(env.chat)
		env.sendPrivate(owner, fmt.Sprintf("/start settings_%d", env.chat.Id))
		sentMain := settingsPrivateOnlySent(t, env, owner)
		button, ok := settingsPageButton(t, staffKeyboardOf(sentMain.Params["reply_markup"]), settingsPageLog)
		if !ok {
			t.Fatal("the private Main has no Log channel button")
		}
		env.pressRaw(settingsPrivateChat(owner), owner, sentMain.MessageID, button.CallbackData)
		text, keyboard := env.lastEdit(owner.Id, sentMain.MessageID)
		wantPage(t, text, keyboard, true)
	})

	t.Run("with no channel linked Main and the page read Not linked", func(t *testing.T) {
		env := newSettingsLogEnv(t)
		msgID := env.openMenu(env.admin)
		mainText, _ := env.lastEdit(env.chat.Id, msgID)
		line, found := settingsMainStateLine(mainText, "settings_page_log")
		if !found || !strings.Contains(line, staffMarker("settings_state_not_linked")) {
			t.Errorf("Main status line = %q (found %v), want Not linked", line, found)
		}
		_, text, keyboard := settingsLogOpenPage(t, env)
		if !strings.Contains(text, staffMarker("settings_log_status")+" "+staffMarker("settings_state_not_linked")) {
			t.Errorf("page %q does not say Not linked", text)
		}
		if got := len(settingsAllButtons(keyboard)); got != settingsLogCategoryCount+2 {
			t.Errorf("page has %d buttons, want the six cells plus Back and Close", got)
		}
	})
}

// settingsLogRunCommand runs /log (on) or /nolog (off) with the category name as the env's
// administrator in chat, through the real dispatcher.
func settingsLogRunCommand(env *settingsEnv, chat gotgbot.Chat, on bool, name string) {
	env.t.Helper()
	command := "/nolog "
	if on {
		command = "/log "
	}
	env.sendIn(chat, env.admin, command+name)
}

func TestSettingsParityLogCategory(t *testing.T) {
	const menuMsgID = 8181

	// run changes one category two ways on the same chat, the command and then a press of the
	// page (the chat is put back in between with the repository), and requires the same stored
	// row, the same cache state and the same audit post to the log channel.
	run := func(t *testing.T, name string, on bool, taps int) {
		t.Helper()
		env := newSettingsLogEnv(t)
		chat := env.chat
		channelID := settingsLogLink(t, chat)
		// The change is a real one: the category starts in the state opposite to the target.
		if on {
			settingsLogSeed(t, chat.Id, name)
		} else {
			settingsLogSeed(t, chat.Id)
		}
		var code string
		for _, cat := range settingsLogCategories {
			if cat.Name == name {
				code = cat.Code
			}
		}
		value := "0"
		if on {
			value = "1"
		}

		settingsLogRunCommand(env, chat, on, name)
		commandFlags := logGoldenFlags(t, chat.Id)
		commandCached := fmt.Sprint(logchannels.CategoryEnabled(logchannels.Get(chat.Id), name))
		commandPosts := settingsLogPosts(env, channelID)
		if len(commandPosts) != 1 {
			t.Fatalf("the command posted %d audit messages, want 1: %v", len(commandPosts), commandPosts)
		}

		// Put the chat back to what it was before the command, then press the page.
		if err := logchannels.SetCategory(chat.Id, name, !on); err != nil {
			t.Fatalf("reset: %v", err)
		}
		answers := env.answerCount()
		for range taps {
			settingsLogToggle(t, env, chat, menuMsgID, code, value)
		}
		if got := env.answerCount() - answers; got != taps {
			t.Errorf("%d presses answered %d times, want once each", taps, got)
		}
		menuFlags := logGoldenFlags(t, chat.Id)
		if menuFlags != commandFlags {
			t.Errorf("the page stored flags %s, the command %s, want the same row", menuFlags, commandFlags)
		}
		if got := fmt.Sprint(logchannels.CategoryEnabled(logchannels.Get(chat.Id), name)); got != commandCached {
			t.Errorf("the cached row reads %s after the page, %s after the command", got, commandCached)
		}
		posts := settingsLogPosts(env, channelID)
		if len(posts) != 1+taps {
			t.Fatalf("the channel got %d posts, want the command's and one per tap (%d): %v", len(posts), 1+taps, posts)
		}
		for i, post := range posts[1:] {
			if post != commandPosts[0] {
				t.Errorf("audit post of tap %d = %q, want the command's %q", i+1, post, commandPosts[0])
			}
		}
		if !strings.Contains(commandPosts[0], "#SETTINGS") {
			t.Errorf("the audit post %q is not the #SETTINGS entry", commandPosts[0])
		}
		// The page redrew its screen from the stored row.
		text, keyboard := env.lastEdit(chat.Id, menuMsgID)
		if strings.Contains(text, "⚠️") {
			t.Errorf("the press drew a notice: %q", text)
		}
		wantIcon := "❌ "
		if on {
			wantIcon = "✅ "
		}
		if cell := settingsLogCell(t, keyboard, name); !strings.HasPrefix(cell.Text, wantIcon) {
			t.Errorf("the redrawn cell = %q, want it to start with %q", cell.Text, wantIcon)
		}
	}

	t.Run("admin off as /nolog admin", func(t *testing.T) { run(t, logchannels.CategoryAdmin, false, 1) })
	t.Run("user on as /log user", func(t *testing.T) { run(t, logchannels.CategoryUser, true, 1) })
	t.Run("a double tap leaves the same row and posts per tap", func(t *testing.T) { run(t, logchannels.CategoryReports, false, 2) })
	t.Run("every category off as the command", func(t *testing.T) {
		for _, cat := range settingsLogCategories {
			if cat.Name == logchannels.CategorySettings {
				continue // /nolog settings switches the audit post itself off, see below
			}
			run(t, cat.Name, false, 1)
		}
	})

	t.Run("turning settings off posts nothing, as /nolog settings", func(t *testing.T) {
		env := newSettingsLogEnv(t)
		channelID := settingsLogLink(t, env.chat)
		settingsLogToggle(t, env, env.chat, menuMsgID, "s", "0")
		if got := logGoldenFlags(t, env.chat.Id); got != "011111" {
			t.Errorf("flags after the press = %s, want only settings off", got)
		}
		if posts := settingsLogPosts(env, channelID); len(posts) != 0 {
			t.Errorf("turning the settings category off posted %v, want nothing (Destination reads it off)", posts)
		}
	})
}
