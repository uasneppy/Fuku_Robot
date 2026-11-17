package modules

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// TestUsersTrackerSkipsChatsAndServiceAccounts holds M10: the users tracker
// records real users only, never a sender_chat (an anonymous admin's group, a
// linked channel) or a Telegram service account, as a user or a chat member.
func TestUsersTrackerSkipsChatsAndServiceAccounts(t *testing.T) {
	bot := newModuleTestBot(newModuleBotClient())
	chatID := uniqueModuleChatID()
	chat := gotgbot.Chat{Id: chatID, Type: "supergroup", Title: "Tracker Chat"}
	linked := gotgbot.Chat{Id: chatID - 1, Type: "channel", Title: "Linked"}
	service := gotgbot.User{Id: 1087968824, IsBot: true, FirstName: "Group"}
	tg := gotgbot.User{Id: 777000, FirstName: "Telegram"}
	t.Cleanup(func() {
		_ = db.DB.Where("chat_id = ?", chatID).Delete(&models.Chat{}).Error
		_ = db.DB.Where("user_id IN ?", []int64{chatID, chatID - 1}).Delete(&models.User{}).Error
	})

	messages := []*gotgbot.Message{
		// An anonymous admin: the group itself is sender_chat.
		{MessageId: 1, Date: 1, Chat: chat, From: &service, SenderChat: &chat, Text: "anon"},
		// A linked channel's automatic forward.
		{MessageId: 2, Date: 1, Chat: chat, From: &tg, SenderChat: &linked, IsAutomaticForward: true, Text: "post"},
		// A reply to the anonymous admin's message.
		{MessageId: 3, Date: 1, Chat: chat, From: &gotgbot.User{Id: 777000, FirstName: "Telegram"}, Text: "re",
			ReplyToMessage: &gotgbot.Message{MessageId: 1, Date: 1, Chat: chat, From: &service, SenderChat: &chat}},
	}
	for _, msg := range messages {
		ctx := ext.NewContext(bot, &gotgbot.Update{UpdateId: msg.MessageId, Message: msg}, nil)
		if err := usersModule.logUsers(bot, ctx); err != ext.ContinueGroups {
			t.Fatalf("logUsers error = %v, want ContinueGroups", err)
		}
	}
	DrainUsersAsyncWrites()

	var users int64
	if err := db.DB.Model(&models.User{}).Where("user_id IN ?", []int64{chatID, chatID - 1, 1087968824}).Count(&users).Error; err != nil {
		t.Fatalf("count users: %v", err)
	}
	if users != 0 {
		t.Fatalf("user rows for chat or service IDs = %d, want 0", users)
	}
	var row models.Chat
	if err := db.DB.Where("chat_id = ?", chatID).Limit(1).Find(&row).Error; err != nil {
		t.Fatalf("read chat: %v", err)
	}
	for _, id := range row.Users {
		if id <= 0 || id == 777000 || id == 1087968824 {
			t.Fatalf("chat users = %v, want no chat or service ID", row.Users)
		}
	}
}

// panickingBotClient answers its first request (the purge's own synchronous
// delete of the command) and panics on every later one, standing in for a bug
// inside a fire-and-forget goroutine.
type panickingBotClient struct {
	moduleBotClient
	calls atomic.Int32
}

func (c *panickingBotClient) RequestWithContext(context.Context, string, string, map[string]any, *gotgbot.RequestOpts) (json.RawMessage, error) {
	if c.calls.Add(1) == 1 {
		return json.RawMessage(`true`), nil
	}
	panic("request panicked")
}

// TestAsyncWorkersRecoverFromPanics holds M11: a panic inside a users tracker
// write or a purge worker is recovered and the WaitGroup still drains, instead
// of crashing the process.
func TestAsyncWorkersRecoverFromPanics(t *testing.T) {
	spawnAsyncUpdate(func() { panic("async write panicked") })
	DrainUsersAsyncWrites()

	bot := &gotgbot.Bot{Token: "999:test", BotClient: &panickingBotClient{}, User: gotgbot.User{Id: 999, IsBot: true}}
	chat := &gotgbot.Chat{Id: uniqueModuleChatID(), Type: "supergroup"}
	if !purgesModule.purgeMsgsConcurrent(bot, chat, false, 100, 130) {
		t.Fatal("purge returned false")
	}
}
