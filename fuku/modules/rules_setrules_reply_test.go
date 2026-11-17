//go:build testtools

package modules

import (
	"fmt"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db/rules"
)

// TestSetRulesByReply proves that /setrules in reply to a message stores its text or its caption with the
// formatting, refuses a reply that has neither and leaves the stored rules alone, and never takes a forum
// topic's root message as the reply.
func TestSetRulesByReply(t *testing.T) {
	withStaffLocale(t)
	admin := gotgbot.User{Id: 777000, FirstName: "Telegram"}
	run := func(t *testing.T, existing string, text string, reply *gotgbot.Message, topic bool) (string, string) {
		t.Helper()
		client := newModuleBotClient()
		bot := newModuleTestBot(client)
		chat := gotgbot.Chat{Id: uniqueModuleChatID(), Type: "supergroup", Title: "Rules Chat", IsForum: topic}
		editCleanup(t, chat.Id)
		if existing != "" {
			rules.SetChatRules(chat.Id, existing)
		}
		ctx := newModuleMessageContext(bot, chat, admin, text)
		ctx.EffectiveMessage.ReplyToMessage = reply
		if topic {
			ctx.EffectiveMessage.IsTopicMessage = true
			ctx.EffectiveMessage.MessageThreadId = reply.MessageId
		}
		_ = rulesModule.setRules(bot, ctx)
		calls := client.callsFor("sendMessage")
		if len(calls) != 1 {
			t.Fatalf("sendMessage calls = %d, want one answer", len(calls))
		}
		return rules.GetChatRulesInfo(chat.Id).Rules, fmt.Sprint(calls[0].Params["text"])
	}

	t.Run("a caption with formatting", func(t *testing.T) {
		reply := &gotgbot.Message{MessageId: 5, Caption: "Be kind", Photo: []gotgbot.PhotoSize{{FileId: "p"}},
			CaptionEntities: []gotgbot.MessageEntity{{Type: "bold", Offset: 0, Length: 2}}}
		stored, answer := run(t, "", "/setrules", reply, false)
		if stored != "<b>Be</b> kind" {
			t.Fatalf("stored rules = %q, want the caption with its bold", stored)
		}
		if !strings.Contains(answer, staffMarker("rules_set_successfully")) {
			t.Fatalf("answer = %q, want the success text", answer)
		}
	})

	t.Run("a reply with no text or caption", func(t *testing.T) {
		reply := &gotgbot.Message{MessageId: 5, Sticker: &gotgbot.Sticker{FileId: "s"}}
		stored, answer := run(t, "existing rules", "/setrules", reply, false)
		if stored != "existing rules" {
			t.Fatalf("stored rules = %q, want the existing rules untouched", stored)
		}
		if !strings.Contains(answer, staffMarker("rules_reply_needs_text")) {
			t.Fatalf("answer = %q, want the refusal", answer)
		}
	})

	t.Run("a forum topic root is not a reply", func(t *testing.T) {
		root := &gotgbot.Message{MessageId: 9, Text: "topic root text",
			ForumTopicCreated: &gotgbot.ForumTopicCreated{Name: "Topic"}}
		stored, _ := run(t, "", "/setrules Fresh rules", root, true)
		if stored != "Fresh rules" {
			t.Fatalf("stored rules = %q, want the command's own text", stored)
		}
	})
}
