package modules

import (
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"

	"github.com/uasneppy/fuku_robot/fuku/utils/helpers"
)

// chatGroupRecorder is a handler at the conversation watcher's group that records every
// message it sees and passes it on.
type chatGroupRecorder struct {
	seen *[]int64
}

func (h chatGroupRecorder) CheckUpdate(_ *gotgbot.Bot, ctx *ext.Context) bool {
	return ctx.EffectiveMessage != nil
}

func (h chatGroupRecorder) HandleUpdate(_ *gotgbot.Bot, ctx *ext.Context) error {
	*h.seen = append(*h.seen, ctx.EffectiveMessage.MessageId)
	return ext.ContinueGroups
}

func (h chatGroupRecorder) Name() string { return "chat-group-recorder" }

// TestChatWatcherGroupReachedByMention is the check of plan section 5.2: with every module
// loaded, an ordinary member's message that addresses the bot (a mention in a text, a mention
// in a caption, a reply to the bot) is not ended by any handler before group 13, so a watcher
// there sees it.
func TestChatWatcherGroupReachedByMention(t *testing.T) {
	withRealLocales(t)
	withFreshHelpRegistry(t)
	previousDisable := append([]string(nil), helpers.DisableCmds...)
	t.Cleanup(func() { helpers.DisableCmds = previousDisable })

	dispatcher := ext.NewDispatcher(&ext.DispatcherOpts{MaxRoutines: -1})
	LoadAllModules(dispatcher)
	var seen []int64
	dispatcher.AddHandlerToGroup(chatGroupRecorder{seen: &seen}, chatWatchGroup)

	client := newModuleBotClient()
	bot := newModuleTestBot(client)
	chatID := uniqueModuleChatID()
	userID := uniqueLinkOwnerID()
	memberReqCleanup(t, []int64{chatID}, []int64{userID})
	chat := gotgbot.Chat{Id: chatID, Type: "supergroup", Title: "Facility"}
	from := &gotgbot.User{Id: userID, FirstName: "Resident"}
	mention := "@" + bot.Username

	messages := []gotgbot.Message{
		{
			MessageId: 101, Date: 1, Chat: chat, From: from, Text: mention + " hello",
			Entities: []gotgbot.MessageEntity{{Type: "mention", Offset: 0, Length: int64(len(mention))}},
		},
		{
			MessageId: 102, Date: 1, Chat: chat, From: from, Caption: mention + " look",
			Photo:           []gotgbot.PhotoSize{{FileId: "p", FileUniqueId: "pu", Width: 10, Height: 10}},
			CaptionEntities: []gotgbot.MessageEntity{{Type: "mention", Offset: 0, Length: int64(len(mention))}},
		},
		{
			MessageId: 103, Date: 1, Chat: chat, From: from, Text: "and you?",
			ReplyToMessage: &gotgbot.Message{MessageId: 90, Date: 1, Chat: chat, From: &bot.User, Text: "Request denied."},
		},
	}
	for i := range messages {
		update := &gotgbot.Update{UpdateId: int64(700 + i), Message: &messages[i]}
		if err := dispatcher.ProcessUpdate(bot, update, nil); err != nil {
			t.Fatalf("ProcessUpdate(%d): %v", messages[i].MessageId, err)
		}
	}

	want := []int64{101, 102, 103}
	if len(seen) != len(want) {
		t.Fatalf("group %d saw messages %v, want %v", chatWatchGroup, seen, want)
	}
	for i := range want {
		if seen[i] != want[i] {
			t.Fatalf("group %d saw messages %v, want %v", chatWatchGroup, seen, want)
		}
	}
}
