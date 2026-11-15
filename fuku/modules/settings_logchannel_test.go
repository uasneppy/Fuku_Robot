//go:build testtools

package modules

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db"
	dbcache "github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/chats"
	"github.com/uasneppy/fuku_robot/fuku/db/logchannels"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/utils/actionlog"
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
	settingsLogLinkTo(t, chat, channelID, "log channel")
	return channelID
}

// settingsLogLinkTo links the channel with the given ID and title to chat, with every category on.
func settingsLogLinkTo(t *testing.T, chat gotgbot.Chat, channelID int64, title string) {
	t.Helper()
	settingsLogCleanup(t, chat.Id, channelID)
	if err := chats.EnsureChatInDb(chat.Id, chat.Title); err != nil {
		t.Fatalf("ensure chat %d: %v", chat.Id, err)
	}
	if err := chats.EnsureChatInDb(channelID, title); err != nil {
		t.Fatalf("ensure channel %d: %v", channelID, err)
	}
	if err := logchannels.Set(chat.Id, chat.Title, channelID); err != nil {
		t.Fatalf("link log channel: %v", err)
	}
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

// settingsLogMenu is one open Log channel page: the message it sits on, the chat that message is in
// (the group for a group menu, the presser's private chat for a private one) and the presser.
type settingsLogMenu struct {
	env   *settingsEnv
	chat  gotgbot.Chat
	user  gotgbot.User
	msgID int64
}

// settingsLogOpenGroupMenu opens the page in a group menu opened by the env's administrator.
func settingsLogOpenGroupMenu(t *testing.T, env *settingsEnv) *settingsLogMenu {
	t.Helper()
	msgID, _, _ := settingsLogOpenPage(t, env)
	return &settingsLogMenu{env: env, chat: env.chat, user: env.admin, msgID: msgID}
}

// settingsLogOpenPrivateMenu opens the page in a private menu of user, who must be an
// administrator of the env's chat.
func settingsLogOpenPrivateMenu(t *testing.T, env *settingsEnv, user gotgbot.User) *settingsLogMenu {
	t.Helper()
	env.sendPrivate(user, fmt.Sprintf("/start settings_%d", env.chat.Id))
	sentMain := settingsPrivateOnlySent(t, env, user)
	button, ok := settingsPageButton(t, staffKeyboardOf(sentMain.Params["reply_markup"]), settingsPageLog)
	if !ok {
		t.Fatal("the private Main has no Log channel button")
	}
	menu := &settingsLogMenu{env: env, chat: settingsPrivateChat(user), user: user, msgID: sentMain.MessageID}
	env.pressRaw(menu.chat, user, menu.msgID, button.CallbackData)
	return menu
}

// last is the text and keyboard of the page's last edit.
func (m *settingsLogMenu) last() (string, [][]gotgbot.InlineKeyboardButton) {
	m.env.t.Helper()
	return m.env.lastEdit(m.chat.Id, m.msgID)
}

// pressCell presses the page's current cell of the named category, as drawn.
func (m *settingsLogMenu) pressCell(name string) {
	m.env.t.Helper()
	_, keyboard := m.last()
	m.env.pressRaw(m.chat, m.user, m.msgID, settingsLogCell(m.env.t, keyboard, name).CallbackData)
}

func TestSettingsLogChannelNotLinked(t *testing.T) {
	t.Run("a press is refused with the command's notice and writes nothing", func(t *testing.T) {
		env := newSettingsLogEnv(t)
		menu := settingsLogOpenGroupMenu(t, env)
		text, keyboard := menu.last()
		if !strings.Contains(text, staffMarker("settings_log_status")+" "+staffMarker("settings_state_not_linked")) {
			t.Fatalf("page %q does not say Not linked", text)
		}
		for _, cat := range settingsLogCategories {
			line := staffMarker("settings_main_state_line") + " ❌ " + staffMarker(cat.NameKey) + " " + staffMarker("common_status_off")
			if !strings.Contains(text, line) {
				t.Errorf("page %q has no Off line %q", text, line)
			}
			if cell := settingsLogCell(t, keyboard, cat.Name); !strings.HasPrefix(cell.Text, "❌ ") {
				t.Errorf("cell %s = %q, want the cross", cat.Name, cell.Text)
			}
		}

		for _, cat := range settingsLogCategories {
			sends := len(env.fake.callsFor("sendMessage"))
			answers := env.answerCount()
			menu.pressCell(cat.Name)
			if got := env.answerCount() - answers; got != 1 {
				t.Errorf("a press of %s answered %d times, want once", cat.Name, got)
			}
			text, keyboard = menu.last()
			if !strings.Contains(text, "⚠️ "+staffMarker("logs_not_set")) {
				t.Errorf("a press of %s drew %q, want the notice logs_not_set", cat.Name, text)
			}
			if got := len(settingsAllButtons(keyboard)); got != settingsLogCategoryCount+2 {
				t.Errorf("the redraw has %d buttons, want the six cells plus Back and Close", got)
			}
			if got := len(env.fake.callsFor("sendMessage")); got != sends {
				t.Errorf("a refused press of %s sent %d messages, want none", cat.Name, got-sends)
			}
			if got := logGoldenFlags(t, env.chat.Id); got != "none" {
				t.Errorf("a refused press of %s left a row: %s", cat.Name, got)
			}
		}
		// The opposite value is refused the same way.
		settingsLogToggle(t, env, env.chat, menu.msgID, "a", "0")
		if text, _ = menu.last(); !strings.Contains(text, staffMarker("logs_not_set")) {
			t.Errorf("an off press drew %q, want logs_not_set", text)
		}
		var rows int64
		if err := db.DB.Model(&models.LogChannel{}).Where("chat_id = ?", env.chat.Id).Count(&rows).Error; err != nil || rows != 0 {
			t.Errorf("log_channels rows = %d (err %v), want none", rows, err)
		}
	})

	t.Run("a channel unlinked after the page was drawn is refused and redrawn as Not linked", func(t *testing.T) {
		env := newSettingsLogEnv(t)
		channelID := settingsLogLink(t, env.chat)
		menu := settingsLogOpenGroupMenu(t, env)
		if err := logchannels.Unset(env.chat.Id); err != nil {
			t.Fatalf("unset: %v", err)
		}
		menu.pressCell(logchannels.CategoryAdmin)
		text, _ := menu.last()
		if !strings.Contains(text, "⚠️ "+staffMarker("logs_not_set")) ||
			!strings.Contains(text, staffMarker("settings_log_status")+" "+staffMarker("settings_state_not_linked")) {
			t.Errorf("the press drew %q, want the notice and the Not linked state", text)
		}
		if posts := settingsLogPosts(env, channelID); len(posts) != 0 {
			t.Errorf("a refused press posted %v", posts)
		}
	})
}

// settingsLogLeakText is everything the bot sent or answered during the test except the posts to
// the log channel itself: every edit and its keyboard, every answer and every other message.
func settingsLogLeakText(t *testing.T, env *settingsEnv, channelID int64) string {
	t.Helper()
	var sb strings.Builder
	for _, method := range []string{"editMessageText", "answerCallbackQuery", "sendMessage", "getChat", "getChatMember"} {
		for _, call := range env.fake.callsFor(method) {
			if method == "sendMessage" && staffParamInt(call.Params, "chat_id") == channelID {
				continue
			}
			sb.WriteString(method + " " + legacyRequestJSON(t, call) + "\n")
		}
	}
	return sb.String()
}

func TestSettingsLogChannelPrivacy(t *testing.T) {
	const (
		channelID    = int64(-1002468013579)
		channelTitle = "ZZ Log Channel Marker"
		digits       = "1002468013579"
	)

	// run drives the page through every kind of press (open, a change on, a change off, a failed
	// save, a refusal after an unlink) and then searches all the bot sent for the channel.
	run := func(t *testing.T, open func(t *testing.T, env *settingsEnv) *settingsLogMenu) {
		t.Helper()
		env := newSettingsLogEnv(t)
		settingsLogLinkTo(t, env.chat, channelID, channelTitle)
		menu := open(t, env)

		menu.pressCell(logchannels.CategoryAdmin)
		menu.pressCell(logchannels.CategoryAdmin)
		for _, cat := range settingsLogCategories {
			menu.pressCell(cat.Name)
		}
		working := logCategoryWrites
		t.Cleanup(func() { logCategoryWrites = working })
		logCategoryWrites = func(int64, string, bool) error { return errors.New("disk is full") }
		menu.pressCell(logchannels.CategoryUser)
		logCategoryWrites = working
		if text, _ := menu.last(); !strings.Contains(text, staffMarker("common_settings_save_failed")) {
			t.Errorf("a failed save drew %q, want the save-failed notice", text)
		}
		if err := logchannels.Unset(env.chat.Id); err != nil {
			t.Fatalf("unset: %v", err)
		}
		menu.pressCell(logchannels.CategoryReports)
		if text, _ := menu.last(); !strings.Contains(text, staffMarker("logs_not_set")) {
			t.Errorf("a press after the unlink drew %q, want logs_not_set", text)
		}

		leaked := settingsLogLeakText(t, env, channelID)
		if !strings.Contains(leaked, staffMarker("settings_page_log")) {
			t.Fatal("the search text holds no page, so the privacy check would prove nothing")
		}
		for _, secret := range []string{channelTitle, digits, strconv.FormatInt(channelID, 10)} {
			if strings.Contains(leaked, secret) {
				t.Errorf("something the bot sent or answered names the log channel (%q)", secret)
			}
		}
	}

	t.Run("a group menu", func(t *testing.T) {
		run(t, func(t *testing.T, env *settingsEnv) *settingsLogMenu { return settingsLogOpenGroupMenu(t, env) })
	})
	t.Run("a private menu of an administrator who is not the creator", func(t *testing.T) {
		run(t, func(t *testing.T, env *settingsEnv) *settingsLogMenu {
			return settingsLogOpenPrivateMenu(t, env, env.admin)
		})
	})

	t.Run("the search would find the channel if a screen named it", func(t *testing.T) {
		env := newSettingsLogEnv(t)
		settingsLogLinkTo(t, env.chat, channelID, channelTitle)
		menu := settingsLogOpenGroupMenu(t, env)
		menu.pressCell(logchannels.CategoryAdmin)
		posts := settingsLogPosts(env, channelID)
		if len(posts) != 1 {
			t.Fatalf("the change posted %d audit messages, want 1", len(posts))
		}
		everything := ""
		for _, call := range env.fake.callsFor("sendMessage") {
			everything += legacyRequestJSON(t, call)
		}
		if !strings.Contains(everything, digits) {
			t.Error("the audit post does not carry the channel's ID, so the exclusion in the leak search proves nothing")
		}
	})
}

func TestSettingsLogChannelForged(t *testing.T) {
	const menuMsgID = 8383

	type forgedCase struct {
		name   string
		fields []string
		extra  map[string]string
		both   bool
	}
	tg := func(control, v string) []string {
		return []string{"a", "tg", "p", settingsPageLog, "t", control, "v", v}
	}
	var forged []forgedCase
	for _, cat := range settingsLogCategories {
		forged = append(forged,
			forgedCase{name: "st on " + cat.Code, fields: []string{"a", "st", "p", settingsPageLog, "t", cat.Code, "v", "1"}},
			forgedCase{name: "v=2 on " + cat.Code, fields: tg(cat.Code, "2")},
			forgedCase{name: "empty v on " + cat.Code, fields: tg(cat.Code, "")},
			forgedCase{name: "an extra key on " + cat.Code, fields: tg(cat.Code, "0"), extra: map[string]string{"x": "1"}},
			forgedCase{name: "c and u together on " + cat.Code, fields: tg(cat.Code, "0"), both: true},
			// The repository name is not a control code.
			forgedCase{name: "the repository name " + cat.Name, fields: tg(cat.Name, "0")},
		)
	}
	forged = append(forged, forgedCase{name: "an unknown control", fields: tg("x", "0")})

	for _, tc := range forged {
		t.Run(tc.name, func(t *testing.T) {
			env := newSettingsLogEnv(t)
			channelID := settingsLogLink(t, env.chat)
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
			before := settingsMenuMarkOf(env)
			env.pressRaw(env.chat, env.admin, menuMsgID, data)
			settingsMenuWantRefused(t, env, before, "common_callback_invalid_request", true)
			if got := logGoldenFlags(t, env.chat.Id); got != "111111" {
				t.Errorf("a forged press changed the flags to %s", got)
			}
			if posts := settingsLogPosts(env, channelID); len(posts) != 0 {
				t.Errorf("a forged press posted %v", posts)
			}
		})
	}
}

func TestSettingsLogChannelFailedSave(t *testing.T) {
	env := newSettingsLogEnv(t)
	channelID := settingsLogLink(t, env.chat)
	menu := settingsLogOpenGroupMenu(t, env)
	mreqSwap(t, &logCategoryWrites, func(int64, string, bool) error { return errors.New("disk is full") })

	answers := env.answerCount()
	menu.pressCell(logchannels.CategoryAdmin)
	if got := env.answerCount() - answers; got != 1 {
		t.Errorf("the press answered %d times, want once", got)
	}
	text, keyboard := menu.last()
	if !strings.Contains(text, "⚠️ "+staffMarker("common_settings_save_failed")) {
		t.Errorf("the page drew %q, want the save-failed notice", text)
	}
	if cell := settingsLogCell(t, keyboard, logchannels.CategoryAdmin); !strings.HasPrefix(cell.Text, "✅ ") {
		t.Errorf("the cell = %q, want the stored state (on) drawn", cell.Text)
	}
	if got := logGoldenFlags(t, env.chat.Id); got != "111111" {
		t.Errorf("a failed save changed the flags to %s", got)
	}
	if posts := settingsLogPosts(env, channelID); len(posts) != 0 {
		t.Errorf("a failed save posted %v, want nothing", posts)
	}
}

func TestSettingsDriftLogChannel(t *testing.T) {
	const menuMsgID = 8484
	env := newSettingsLogEnv(t)
	chat := env.chat
	channelID := settingsLogLink(t, chat)

	// A command write is what the next draw shows, across the cache.
	settingsLogRunCommand(env, chat, false, logchannels.CategoryReports)
	dbcache.ResetLocalForTest()
	env.pressRaw(chat, env.admin, menuMsgID, encodeCallbackData(settingsCallbackNamespace,
		env.groupFields(env.admin, "a", "op", "p", settingsPageLog)))
	text, keyboard := env.lastEdit(chat.Id, menuMsgID)
	line := staffMarker("settings_main_state_line") + " ❌ " + staffMarker("settings_log_cat_reports") + " " + staffMarker("common_status_off")
	if !strings.Contains(text, line) {
		t.Errorf("after /nolog reports the page drew %q, want %q", text, line)
	}
	cell := settingsLogCell(t, keyboard, logchannels.CategoryReports)
	if fields := settingsButtonFields(t, cell); fields["v"] != "1" || !strings.HasPrefix(cell.Text, "❌ ") {
		t.Errorf("Reports cell = %q fields %v after /nolog reports, want the cross and target 1", cell.Text, fields)
	}

	// A page press is what the log code reads: a post of a category turned off is not sent, one of
	// a category turned on is. The page's own #SETTINGS audit post is the first message.
	settingsLogToggle(t, env, chat, menuMsgID, "a", "0")
	dbcache.ResetLocalForTest()
	if logchannels.CategoryEnabled(logchannels.Get(chat.Id), logchannels.CategoryAdmin) {
		t.Error("the log code reads the admin category on after the page turned it off")
	}
	posts := len(settingsLogPosts(env, channelID))
	actionlog.Admin(env.bot, &chat, &env.admin, "ban", 5, "")
	if got := len(settingsLogPosts(env, channelID)); got != posts {
		t.Errorf("an admin-category log post was sent while the category is off (%d posts, was %d)", got, posts)
	}
	settingsLogToggle(t, env, chat, menuMsgID, "a", "1")
	dbcache.ResetLocalForTest()
	posts = len(settingsLogPosts(env, channelID))
	actionlog.Admin(env.bot, &chat, &env.admin, "ban", 5, "")
	if got := len(settingsLogPosts(env, channelID)); got != posts+1 {
		t.Errorf("no admin-category log post was sent after the page turned the category on (%d posts, was %d)", got, posts)
	}
}

// settingsLogAuthorityCase is one category of the authority table: the command that changes it
// and the press that changes it the same way.
type settingsLogAuthorityCase struct {
	name    string
	command string
	press   []string
}

// settingsLogAuthorityCases is the Admin actions category switched off, by the command and by
// the page.
func settingsLogAuthorityCases() []settingsLogAuthorityCase {
	return []settingsLogAuthorityCase{
		{"Admin actions off", "/nolog admin", []string{"a", "tg", "p", settingsPageLog, "t", "a", "v", "0"}},
		{"Reports off", "/nolog reports", []string{"a", "tg", "p", settingsPageLog, "t", "r", "v", "0"}},
	}
}

func TestSettingsAuthorityParityLogChannel(t *testing.T) {
	const menuMsgID = 8585

	// newChat adds a linked chat whose administrator list makes env.admin an administrator.
	newChat := func(t *testing.T, env *settingsEnv) gotgbot.Chat {
		t.Helper()
		chat := env.addChat()
		settingsLogLink(t, chat)
		return chat
	}
	wrote := func(t *testing.T, chatID int64) bool {
		t.Helper()
		return logGoldenFlags(t, chatID) != "111111"
	}
	press := func(env *settingsEnv, chat gotgbot.Chat, kv []string) {
		env.pressRaw(chat, env.admin, menuMsgID, encodeCallbackData(settingsCallbackNamespace, env.groupFields(env.admin, kv...)))
	}

	t.Run("a live administrator writes by both, and neither asks for a bot check", func(t *testing.T) {
		for _, tc := range settingsLogAuthorityCases() {
			t.Run(tc.name, func(t *testing.T) {
				env := newSettingsLogEnv(t)
				commandChat, menuChat := newChat(t, env), newChat(t, env)
				// /log and /nolog have no bot check, so a bot that is no administrator changes nothing.
				env.fake.setBotRole(commandChat.Id, staffRoleMember)
				env.fake.setBotRole(menuChat.Id, staffRoleMember)

				env.sendIn(commandChat, env.admin, tc.command)
				answers := env.answerCount()
				press(env, menuChat, tc.press)
				if got := env.answerCount() - answers; got != 1 {
					t.Errorf("the press answered %d times, want once", got)
				}
				for _, id := range []int64{commandChat.Id, menuChat.Id} {
					if !wrote(t, id) {
						t.Errorf("chat %d: the change was not stored", id)
					}
				}
				if c, m := logGoldenFlags(t, commandChat.Id), logGoldenFlags(t, menuChat.Id); c != m {
					t.Errorf("flags: command %s, page %s, want the same", c, m)
				}
			})
		}
	})

	t.Run("a plain member is refused by both", func(t *testing.T) {
		for _, tc := range settingsLogAuthorityCases() {
			t.Run(tc.name, func(t *testing.T) {
				env := newSettingsLogEnv(t)
				chat := newChat(t, env)
				member := gotgbot.User{Id: uniqueLinkOwnerID() + 44, FirstName: "Plain"}
				t.Cleanup(func() {
					db.DB.Where("user_id IN ?", []int64{member.Id}).Delete(&models.User{})
					dbcache.DeleteCache(dbcache.CacheKey("user", member.Id))
					dbcache.ResetLocalForTest()
				})
				env.fake.setMember(chat.Id, member.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})

				env.sendIn(chat, member, tc.command)
				if !settingsAntifloodSentHas(env, chat.Id, staffMarker("chat_status_user_admin_cmd_error")) {
					t.Error("the command did not refuse a plain member")
				}
				answers := env.answerCount()
				env.pressPrivate(member, 777, env.privateFields(chat.Id, tc.press...))
				if got := env.answerCount() - answers; got != 1 {
					t.Errorf("the press answered %d times, want once", got)
				}
				if answer, alert := env.lastAnswer(); !alert || !strings.Contains(answer, staffMarker("settings_not_admin")) {
					t.Errorf("answer = %q (alert %v), want the alert settings_not_admin", answer, alert)
				}
				if wrote(t, chat.Id) {
					t.Error("a plain member's press or command wrote")
				}
			})
		}
	})

	t.Run("an administrator the owner gate refuses is refused by the menu only (the documented carve-out)", func(t *testing.T) {
		for _, tc := range settingsLogAuthorityCases() {
			t.Run(tc.name, func(t *testing.T) {
				resetSettingsOwnerMemo(t)
				env := newSettingsLogEnv(t)
				commandChat, menuChat := newChat(t, env), newChat(t, env)
				for _, chat := range []gotgbot.Chat{commandChat, menuChat} {
					env.newOwner(chat)
					env.setAdminsAllowed(chat.Id, false)
				}

				answers := env.answerCount()
				press(env, menuChat, tc.press)
				if got := env.answerCount() - answers; got != 1 {
					t.Errorf("the gated press answered %d times, want once", got)
				}
				if answer, alert := env.lastAnswer(); !alert || !strings.Contains(answer, staffMarker("settings_gate_owner_only")) {
					t.Errorf("answer = %q (alert %v), want the alert settings_gate_owner_only", answer, alert)
				}
				if wrote(t, menuChat.Id) {
					t.Error("the gated press wrote")
				}
				env.sendIn(commandChat, env.admin, tc.command)
				if !wrote(t, commandChat.Id) {
					t.Errorf("%s did not write for an administrator the menu refuses", tc.command)
				}
				env.setAdminsAllowed(menuChat.Id, true)
				press(env, menuChat, tc.press)
				if !wrote(t, menuChat.Id) {
					t.Error("with the switch on the press did not write")
				}
			})
		}
	})
}

func TestSettingsLogChannelReadsCreateNothing(t *testing.T) {
	const menuMsgID = 8686

	// logKeys is the Redis keys of the log channel family of chat.
	logKeys := func(env *settingsEnv, chatID int64) []string {
		var keys []string
		for _, key := range env.redis.Keys() {
			if strings.Contains(key, "log_channel") && strings.Contains(key, fmt.Sprint(chatID)) {
				keys = append(keys, key)
			}
		}
		slices.Sort(keys)
		return keys
	}

	t.Run("opening Main and the page with no channel leaves no row and no key", func(t *testing.T) {
		env := newSettingsLogEnv(t)
		chat := env.addChat()
		settingsLogCleanup(t, chat.Id)
		env.pressRaw(chat, env.admin, menuMsgID, encodeCallbackData(settingsCallbackNamespace,
			env.groupFields(env.admin, "a", "op", "p", settingsPageLog)))
		text, _ := env.lastEdit(chat.Id, menuMsgID)
		if !strings.Contains(text, staffMarker("settings_state_not_linked")) {
			t.Fatalf("the page drew %q, want Not linked", text)
		}
		env.pressRaw(chat, env.admin, menuMsgID, encodeCallbackData(settingsCallbackNamespace, env.groupFields(env.admin, "a", "bk")))
		mainText, _ := env.lastEdit(chat.Id, menuMsgID)
		if line, found := settingsMainStateLine(mainText, "settings_page_log"); !found || strings.HasSuffix(line, settingsUnknownState) {
			t.Errorf("Main status line = %q (found %v), want Not linked", line, found)
		}
		if got := logGoldenFlags(t, chat.Id); got != "none" {
			t.Errorf("opening created a log_channels row: %s", got)
		}
		var chatRows int64
		if err := db.DB.Model(&models.Chat{}).Where("chat_id = ?", chat.Id).Count(&chatRows).Error; err != nil || chatRows != 0 {
			t.Errorf("opening created %d chats rows (err %v)", chatRows, err)
		}
		if keys := logKeys(env, chat.Id); len(keys) != 0 {
			t.Errorf("the reads cached %v for a chat with no channel", keys)
		}
	})

	t.Run("with a channel the reads leave only the key the commands' loader makes", func(t *testing.T) {
		env := newSettingsLogEnv(t)
		chat := env.chat
		settingsLogLink(t, chat)
		dbcache.DeleteCache(dbcache.CacheKey("log_channel", chat.Id))
		dbcache.ResetLocalForTest()
		env.pressRaw(chat, env.admin, menuMsgID, encodeCallbackData(settingsCallbackNamespace,
			env.groupFields(env.admin, "a", "op", "p", settingsPageLog)))
		fromPage := logKeys(env, chat.Id)

		dbcache.DeleteCache(dbcache.CacheKey("log_channel", chat.Id))
		dbcache.ResetLocalForTest()
		if logchannels.GetContext(context.Background(), chat.Id) == nil {
			t.Fatal("the commands' loader found no row")
		}
		fromCommand := logKeys(env, chat.Id)
		if len(fromPage) == 0 || !slices.Equal(fromPage, fromCommand) {
			t.Errorf("the page left %v, the commands' loader %v, want the same keys", fromPage, fromCommand)
		}
	})

	t.Run("a failing reader is the cannot-open edit and the unknown mark, and a press writes nothing", func(t *testing.T) {
		env := newSettingsLogEnv(t)
		settingsLogLink(t, env.chat)
		menu := settingsLogOpenGroupMenu(t, env)
		_, keyboard := menu.last()
		cell := settingsLogCell(t, keyboard, logchannels.CategoryAdmin)
		mreqSwap(t, &settingsLogRead, func(context.Context, int64) (*models.LogChannel, error) {
			return nil, errors.New("database is down")
		})

		msgID := env.openMenu(env.admin)
		mainText, _ := env.lastEdit(env.chat.Id, msgID)
		if line, found := settingsMainStateLine(mainText, "settings_page_log"); !found || !strings.HasSuffix(line, settingsUnknownState) {
			t.Errorf("Log channel status line = %q (found %v), want the unknown mark", line, found)
		}
		if line, found := settingsMainStateLine(mainText, "settings_page_antiflood"); !found || strings.HasSuffix(line, settingsUnknownState) {
			t.Errorf("Antiflood status line = %q (found %v), want its own state intact", line, found)
		}
		_, mainKeyboard := env.lastEdit(env.chat.Id, msgID)
		button, ok := settingsPageButton(t, mainKeyboard, settingsPageLog)
		if !ok {
			t.Fatal("Main has no Log channel button")
		}
		env.pressRaw(env.chat, env.admin, msgID, button.CallbackData)
		text, kb := env.lastEdit(env.chat.Id, msgID)
		if !strings.Contains(text, staffMarker("settings_cannot_open")) || len(kb) != 0 {
			t.Errorf("a failed read drew %q with %d rows, want the cannot-open edit", text, len(kb))
		}

		// A press of a cell drawn before the failure is the cannot-open edit too and writes nothing.
		env.pressRaw(env.chat, env.admin, menu.msgID, cell.CallbackData)
		if text, _ = env.lastEdit(env.chat.Id, menu.msgID); !strings.Contains(text, staffMarker("settings_cannot_open")) {
			t.Errorf("a press with a failing reader drew %q, want the cannot-open edit", text)
		}
		if got := logGoldenFlags(t, env.chat.Id); got != "111111" {
			t.Errorf("a press with a failing reader changed the flags to %s", got)
		}
	})
}
