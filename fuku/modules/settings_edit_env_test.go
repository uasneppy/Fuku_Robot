//go:build testtools

package modules

import (
	"fmt"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db"
	dbcache "github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// newSettingsEditEnv is the settings environment plus the Greetings and Rules commands on its
// dispatcher, so a parity test can run /welcome, /cleanwelcome and the rest next to the menu.
// The env's chat has no greetings or rules row to begin with.
func newSettingsEditEnv(t *testing.T) *settingsEnv {
	t.Helper()
	env := newSettingsEnv(t)
	LoadGreetings(env.dispatcher)
	LoadRules(env.dispatcher)
	editCleanup(t, env.chat.Id)
	return env
}

// editCleanup removes the greetings and rules rows of the given chats, and the cached greetings
// copy, before and after the test.
func editCleanup(t *testing.T, chatIDs ...int64) {
	t.Helper()
	clean := func() {
		if len(chatIDs) == 0 {
			return
		}
		db.DB.Where("chat_id IN ?", chatIDs).Delete(&models.GreetingSettings{})
		db.DB.Where("chat_id IN ?", chatIDs).Delete(&models.RulesSettings{})
		for _, id := range chatIDs {
			dbcache.DeleteCache(dbcache.CacheKey("greetings", id))
		}
		dbcache.ResetLocalForTest()
	}
	clean()
	t.Cleanup(clean)
}

// settingsEditAdminJSON is a getChatMember answer for userID as an administrator who holds
// can_change_info or not and nothing else that matters to the editor.
func settingsEditAdminJSON(userID int64, canChangeInfo bool) string {
	return fmt.Sprintf(
		`{"status":"administrator","user":{"id":%d,"is_bot":false,"first_name":"Admin%d"},"can_be_edited":false,`+
			`"is_anonymous":false,"can_manage_chat":true,"can_delete_messages":false,"can_manage_video_chats":false,`+
			`"can_restrict_members":false,"can_promote_members":false,"can_change_info":%t,"can_invite_users":false}`,
		userID, userID, canChangeInfo)
}

// settingsEditStored reads the greetings row of chatID straight from the database, past every
// cache layer. ok is false when the chat has no row.
func settingsEditStored(t *testing.T, chatID int64) (row models.GreetingSettings, ok bool) {
	t.Helper()
	if err := db.DB.Where("chat_id = ?", chatID).Limit(1).Find(&row).Error; err != nil {
		t.Fatalf("read greetings of %d: %v", chatID, err)
	}
	return row, row.ID != 0
}

// settingsEditRulesStored reads the rules row of chatID straight from the database. ok is false
// when the chat has no row.
func settingsEditRulesStored(t *testing.T, chatID int64) (row models.RulesSettings, ok bool) {
	t.Helper()
	if err := db.DB.Where("chat_id = ?", chatID).Limit(1).Find(&row).Error; err != nil {
		t.Fatalf("read rules of %d: %v", chatID, err)
	}
	return row, row.ChatId == chatID
}

// openEditPage opens the menu in the env's group as opener, presses the button of page pageKey
// and returns the menu message's ID.
func openEditPage(env *settingsEnv, opener gotgbot.User, pageKey string) int64 {
	env.t.Helper()
	msgID := env.openMenu(opener)
	_, keyboard := env.lastEdit(env.chat.Id, msgID)
	button, ok := settingsPageButton(env.t, keyboard, pageKey)
	if !ok {
		env.t.Fatalf("the Main page has no button for page %q", pageKey)
	}
	env.pressRaw(env.chat, opener, msgID, button.CallbackData)
	return msgID
}

// openEditPagePrivate opens the group's menu in user's private chat through the deep link,
// presses the button of page pageKey and returns the ID of the private menu message.
func openEditPagePrivate(env *settingsEnv, user gotgbot.User, pageKey string) int64 {
	env.t.Helper()
	env.sendPrivate(user, fmt.Sprintf("/start settings_%d", env.chat.Id))
	sent := settingsPrivateOnlySent(env.t, env, user)
	button, ok := settingsPageButton(env.t, staffKeyboardOf(sent.Params["reply_markup"]), pageKey)
	if !ok {
		env.t.Fatalf("the private Main page has no button for page %q", pageKey)
	}
	env.pressRaw(settingsPrivateChat(user), user, sent.MessageID, button.CallbackData)
	return sent.MessageID
}
