//go:build testtools

package modules

import (
	"context"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db"
	dbcache "github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/eventmsg"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// settingsMsgEnv is the toolkit of the Messages page's tests: the settings environment (marker locale, one
// supergroup, an administrator the owner gate admits) with the env chat's event messages and Staff Group
// rows cleaned up before and after the test, and a bot username for the Open in private row.
type settingsMsgEnv struct {
	*settingsEnv
}

// newSettingsMsgEnv builds the environment.
func newSettingsMsgEnv(t *testing.T) *settingsMsgEnv {
	t.Helper()
	setCachedBotUsername(t, "FukuTestBot")
	env := &settingsMsgEnv{settingsEnv: newSettingsEnvWith(t, settingsEnvOptions{})}
	settingsMsgCleanup(t, env.chat.Id)
	staffCleanup(t, env.chat.Id)
	return env
}

// addChat adds a second supergroup and cleans its event messages too.
func (e *settingsMsgEnv) addChat() gotgbot.Chat {
	e.t.Helper()
	chat := e.settingsEnv.addChat()
	settingsMsgCleanup(e.t, chat.Id)
	return chat
}

// settingsMsgCleanup deletes the event messages of the chats and their cached copies before the test and
// again when it ends, then resets the in-process cache layer.
func settingsMsgCleanup(t *testing.T, chatIDs ...int64) {
	t.Helper()
	clean := func() {
		if len(chatIDs) == 0 {
			return
		}
		db.DB.Where("chat_id IN ?", chatIDs).Delete(&models.ChatEventMessage{})
		for _, id := range chatIDs {
			dbcache.DeleteCache(eventmsg.CacheKey(id))
		}
		dbcache.ResetLocalForTest()
	}
	clean()
	t.Cleanup(clean)
}

// settingsMsgSeed stores one event's group message through the repository: its text when text is not
// empty, and its Silent switch when silent is true.
func settingsMsgSeed(t *testing.T, chatID int64, event, text string, silent bool) {
	t.Helper()
	ctx := context.Background()
	if text != "" {
		if err := eventmsg.SetText(ctx, chatID, event, eventmsg.DestGroup, text, 1); err != nil {
			t.Fatalf("seed the %s text of chat %d: %v", event, chatID, err)
		}
	}
	if silent {
		if err := eventmsg.SetSilent(ctx, chatID, event, eventmsg.DestGroup, true, 1); err != nil {
			t.Fatalf("seed the %s Silent switch of chat %d: %v", event, chatID, err)
		}
	}
}

// settingsMsgRows counts the stored event message rows of a chat.
func settingsMsgRows(t *testing.T, chatID int64) int64 {
	t.Helper()
	var n int64
	if err := db.DB.Model(&models.ChatEventMessage{}).Where("chat_id = ?", chatID).Count(&n).Error; err != nil {
		t.Fatalf("count the event messages of chat %d: %v", chatID, err)
	}
	return n
}

// settingsMsgMenu is one private settings message of a group, pressed by one user.
type settingsMsgMenu struct {
	env   *settingsMsgEnv
	chat  gotgbot.Chat
	user  gotgbot.User
	msgID int64
}

// settingsMsgPrivateMenu starts a private menu of chat for user on a fresh message ID; nothing is pressed yet.
func settingsMsgPrivateMenu(e *settingsMsgEnv, chat gotgbot.Chat, user gotgbot.User) *settingsMsgMenu {
	e.t.Helper()
	return &settingsMsgMenu{env: e, chat: chat, user: user, msgID: e.updateID()}
}

// press presses a button of the menu built from the pairs plus the private owner field c, and returns the
// text and keyboard of the message after the press (the last edit; an unedited message fails the test).
func (m *settingsMsgMenu) press(kv ...string) (string, [][]gotgbot.InlineKeyboardButton) {
	m.env.t.Helper()
	m.env.pressPrivate(m.user, m.msgID, m.env.privateFields(m.chat.Id, kv...))
	return m.env.lastEdit(m.user.Id, m.msgID)
}

// pressButton presses an existing button of the menu and returns the message after it.
func (m *settingsMsgMenu) pressButton(button gotgbot.InlineKeyboardButton) (string, [][]gotgbot.InlineKeyboardButton) {
	m.env.t.Helper()
	m.env.pressRaw(settingsPrivateChat(m.user), m.user, m.msgID, button.CallbackData)
	return m.env.lastEdit(m.user.Id, m.msgID)
}

// settingsMsgOpenPrivate presses the Messages cell of Main in a private menu of the env's chat as user and
// returns the menu with the hub's text and keyboard.
func settingsMsgOpenPrivate(e *settingsMsgEnv, user gotgbot.User) (*settingsMsgMenu, string, [][]gotgbot.InlineKeyboardButton) {
	e.t.Helper()
	m := settingsMsgPrivateMenu(e, e.chat, user)
	text, keyboard := m.press("a", settingsActOpen, "p", settingsPageMessages)
	return m, text, keyboard
}
