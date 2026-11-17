package modules

import (
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"

	"github.com/uasneppy/fuku_robot/fuku/utils/helpers"
)

// topicCommandContext is a command sent inside a forum topic that is not an
// explicit reply: Telegram sets reply_to_message to the topic's root.
func topicCommandContext(t *testing.T, bot *gotgbot.Bot, chat gotgbot.Chat, text string) *helpers.CommandContext {
	t.Helper()
	user := gotgbot.User{Id: 777000, FirstName: "Telegram"}
	ctx := newModuleMessageContext(bot, chat, user, text)
	ctx.EffectiveMessage.IsTopicMessage = true
	ctx.EffectiveMessage.MessageThreadId = 70
	ctx.EffectiveMessage.ReplyToMessage = &gotgbot.Message{
		MessageId:         70,
		Date:              1,
		Chat:              chat,
		ForumTopicCreated: &gotgbot.ForumTopicCreated{Name: "topic"},
	}
	cmdCtx, err := helpers.BuildCommandContext(bot, ctx)
	if err != nil {
		t.Fatalf("BuildCommandContext failed: %v", err)
	}
	return cmdCtx
}

// TestPinCommandsIgnoreForumTopicRoot holds M7: /pin, /unpin and /permapin in a
// forum topic never take the topic's root for the replied message.
func TestPinCommandsIgnoreForumTopicRoot(t *testing.T) {
	chat := gotgbot.Chat{Id: uniqueModuleChatID(), Type: "supergroup", Title: "Forum", IsForum: true}

	t.Run("pin", func(t *testing.T) {
		client := newModuleBotClient()
		bot := newModuleTestBot(client)
		if err := pinsModule.pin(topicCommandContext(t, bot, chat, "/pin")); err != ext.EndGroups {
			t.Fatalf("pin() error = %v, want EndGroups", err)
		}
		if calls := client.callsFor("pinChatMessage"); len(calls) != 0 {
			t.Fatalf("pinChatMessage calls = %d, want none for the topic's root", len(calls))
		}
	})

	t.Run("unpin", func(t *testing.T) {
		client := newModuleBotClient()
		bot := newModuleTestBot(client)
		if err := pinsModule.unpin(topicCommandContext(t, bot, chat, "/unpin")); err != ext.EndGroups {
			t.Fatalf("unpin() error = %v, want EndGroups", err)
		}
		calls := client.callsFor("unpinChatMessage")
		if len(calls) != 1 || calls[0].Params["message_id"] != nil {
			t.Fatalf("unpinChatMessage calls = %v, want one without a message ID (the last pin)", calls)
		}
	})

	t.Run("permapin", func(t *testing.T) {
		client := newModuleBotClient()
		bot := newModuleTestBot(client)
		if err := pinsModule.permaPin(topicCommandContext(t, bot, chat, "/permapin")); err != ext.EndGroups {
			t.Fatalf("permaPin() error = %v, want EndGroups", err)
		}
		if calls := client.callsFor("pinChatMessage"); len(calls) != 0 {
			t.Fatalf("pinChatMessage calls = %d, want none for the topic's root", len(calls))
		}
	})
}
