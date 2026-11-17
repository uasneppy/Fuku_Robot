//go:build testtools

package modules

import (
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"

	dbcache "github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/filters"
	"github.com/uasneppy/fuku_robot/fuku/db/notes"
)

// topicCommand is a command sent in a forum topic: Telegram sets reply_to_message to the topic's
// forum_topic_created service message although the member replied to nothing.
func topicCommand(bot *gotgbot.Bot, chat gotgbot.Chat, from gotgbot.User, text string) *ext.Context {
	ctx := newModuleMessageContext(bot, chat, from, text)
	ctx.EffectiveMessage.IsTopicMessage = true
	ctx.EffectiveMessage.MessageThreadId = 9
	ctx.EffectiveMessage.ReplyToMessage = &gotgbot.Message{MessageId: 9, Text: "topic root text",
		ForumTopicCreated: &gotgbot.ForumTopicCreated{Name: "Topic"}}
	return ctx
}

// TestSaveAndFilterInForumTopic proves that /save and /filter in a forum topic take the command's own text,
// never the topic's root message, as the content.
func TestSaveAndFilterInForumTopic(t *testing.T) {
	admin := gotgbot.User{Id: 777000, FirstName: "Telegram"}
	chat := gotgbot.Chat{Id: uniqueModuleChatID(), Type: "supergroup", Title: "Forum", IsForum: true}
	noteGoldenCleanup(t, chat.Id)
	filterGoldenCleanup(t, chat.Id)

	client := newModuleBotClient()
	bot := newModuleTestBot(client)
	_ = notesModule.addNote(bot, topicCommand(bot, chat, admin, "/save rules be kind"))
	dbcache.ResetLocalForTest()
	if note := notes.GetNote(chat.Id, "rules"); note == nil || !strings.Contains(note.NoteContent, "be kind") {
		t.Fatalf("the topic /save stored %+v, want the note rules with the command's text", note)
	}

	_ = filtersModule.addFilter(bot, topicCommand(bot, chat, admin, "/filter hello hi there"))
	dbcache.ResetLocalForTest()
	found := false
	for _, f := range filters.GetFiltersList(chat.Id) {
		if f == "hello" {
			found = true
		}
	}
	if !found {
		t.Fatalf("the topic /filter stored %v, want the keyword hello", filters.GetFiltersList(chat.Id))
	}
}
