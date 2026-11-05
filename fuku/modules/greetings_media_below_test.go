//go:build testtools

package modules

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/greetings"
)

// mediaBelowClient is the greeting fake that also answers every media send, so a send succeeds.
func mediaBelowClient() *moduleBotClient {
	client := legacyGreetingClient()
	message := json.RawMessage(`{"message_id":9010,"date":1,"chat":{"id":-1001,"type":"supergroup","title":"Test Chat"}}`)
	for _, method := range []string{"sendSticker", "sendAudio", "sendVoice", "sendVideoNote"} {
		client.responses[method] = message
	}
	return client
}

// mediaBelowJoin drives one member's join through the chat_member handler.
func mediaBelowJoin(t *testing.T, client *moduleBotClient, chat gotgbot.Chat) {
	t.Helper()
	bot := newModuleTestBot(client)
	admin := gotgbot.User{Id: 777000, FirstName: "Telegram"}
	member := gotgbot.User{Id: 4242, FirstName: "Newbie"}
	ctx := newChatMemberContext(bot, chat, admin,
		gotgbot.ChatMemberLeft{User: member}, gotgbot.ChatMemberMember{User: member})
	clearRecentJoinProcessing(chat.Id, member.Id)
	if err := greetingsModule.newMember(bot, ctx); err != ext.EndGroups {
		t.Fatalf("newMember error = %v, want EndGroups", err)
	}
}

// mediaBelowOnlyCall returns the one recorded call of the method and fails when there is not exactly one.
func mediaBelowOnlyCall(t *testing.T, client *moduleBotClient, method string) moduleBotCall {
	t.Helper()
	calls := client.callsFor(method)
	if len(calls) != 1 {
		t.Fatalf("%s calls = %d, want 1", method, len(calls))
	}
	return calls[0]
}

func TestGreetingsMediaBelowEverywhere(t *testing.T) {
	withRealLocales(t)
	group := func(id int64) gotgbot.Chat {
		return gotgbot.Chat{Id: id, Type: "supergroup", Title: "Greeting Chat"}
	}
	buttons := []db.Button{{Name: "Rules", Url: "https://example.com/rules"}}

	t.Run("join_photo_switch_on", func(t *testing.T) {
		chatID := uniqueModuleChatID()
		legacyCleanup(t, chatID)
		mustLegacy(t, greetings.SetWelcomeText(chatID, "Hi {first}", "w-photo", buttons, db.PHOTO))
		if res := applyWelcomeMediaBelow(context.Background(), chatID, true); res.Outcome != greetingApplied {
			t.Fatalf("applyWelcomeMediaBelow outcome = %v, want applied", res.Outcome)
		}
		client := mediaBelowClient()
		mediaBelowJoin(t, client, group(chatID))
		call := mediaBelowOnlyCall(t, client, "sendPhoto")
		if call.Params["show_caption_above_media"] != true {
			t.Fatalf("show_caption_above_media = %v, want true", call.Params["show_caption_above_media"])
		}
		if call.Params["caption"] != "Hi Newbie" || call.Params["photo"] != "w-photo" {
			t.Fatalf("caption/photo = %v / %v", call.Params["caption"], call.Params["photo"])
		}
		if call.Params["reply_markup"] == nil {
			t.Fatalf("the stored keyboard is missing from the request")
		}
	})
	t.Run("join_photo_switch_back_off", func(t *testing.T) {
		chatID := uniqueModuleChatID()
		legacyCleanup(t, chatID)
		mustLegacy(t, greetings.SetWelcomeText(chatID, "Hi {first}", "w-photo", buttons, db.PHOTO))
		applyWelcomeMediaBelow(context.Background(), chatID, true)
		if res := applyWelcomeMediaBelow(context.Background(), chatID, false); res.Outcome != greetingApplied {
			t.Fatalf("applyWelcomeMediaBelow(false) outcome = %v, want applied", res.Outcome)
		}
		client := mediaBelowClient()
		mediaBelowJoin(t, client, group(chatID))
		call := mediaBelowOnlyCall(t, client, "sendPhoto")
		if _, ok := call.Params["show_caption_above_media"]; ok {
			t.Fatalf("show_caption_above_media is present with the switch off")
		}
	})
}
