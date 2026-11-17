package modules

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/greetings"
)

// TestGreetingDisplayRepliesToCommand holds owner decision N2 of the 2026-11-17 review: the greeting
// that /welcome and /goodbye show is a reply to the command message, also with noformat, while the
// greeting sent at a join or a leave stays a plain message.
func TestGreetingDisplayRepliesToCommand(t *testing.T) {
	admin := gotgbot.User{Id: 777000, FirstName: "Telegram"}
	for _, tc := range []struct {
		name    string
		command string
		run     func(*gotgbot.Bot, *ext.Context) error
		set     func(chatID int64) error
	}{
		{name: "welcome", command: "/welcome", run: greetingsModule.welcome,
			set: func(id int64) error { return greetings.SetWelcomeText(id, "shown welcome", "", nil, db.TEXT) }},
		{name: "welcome noformat", command: "/welcome noformat", run: greetingsModule.welcome,
			set: func(id int64) error { return greetings.SetWelcomeText(id, "shown welcome", "", nil, db.TEXT) }},
		{name: "goodbye", command: "/goodbye", run: greetingsModule.goodbye,
			set: func(id int64) error { return greetings.SetGoodbyeText(id, "shown goodbye", "", nil, db.TEXT) }},
		{name: "goodbye photo", command: "/goodbye", run: greetingsModule.goodbye,
			set: func(id int64) error { return greetings.SetGoodbyeText(id, "shown goodbye", "photo-id", nil, db.PHOTO) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			chat := gotgbot.Chat{Id: uniqueModuleChatID(), Type: "supergroup", Title: "Reply Chat"}
			editCleanup(t, chat.Id)
			if err := tc.set(chat.Id); err != nil {
				t.Fatalf("set greeting: %v", err)
			}
			client := newModuleBotClient()
			bot := newModuleTestBot(client)
			ctx := newGreetingMessageContext(bot, chat, admin, tc.command)
			if err := tc.run(bot, ctx); err != ext.EndGroups {
				t.Fatalf("%s error = %v, want EndGroups", tc.command, err)
			}
			commandID := ctx.EffectiveMessage.MessageId
			found := 0
			client.mu.Lock()
			defer client.mu.Unlock()
			for _, call := range client.calls {
				caption, _ := call.Params["caption"].(string)
				text, _ := call.Params["text"].(string)
				if caption == "" && text == "" {
					continue
				}
				body := text + caption
				if !containsAny(body, "shown welcome", "shown goodbye") {
					continue
				}
				found++
				reply, ok := call.Params["reply_parameters"]
				if !ok {
					t.Fatalf("%s: the greeting was not sent as a reply to the command", call.Method)
				}
				if got := replyMessageID(t, reply); got != commandID {
					t.Fatalf("%s: the greeting replies to message %d, want the command %d", call.Method, got, commandID)
				}
			}
			if found != 1 {
				t.Fatalf("greeting sent %d times, want once", found)
			}
		})
	}
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// replyMessageID reads message_id from a recorded reply_parameters value, whatever form the client
// recorded it in.
func replyMessageID(t *testing.T, value any) int64 {
	t.Helper()
	raw, ok := value.(string)
	if !ok {
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatalf("encode reply_parameters: %v", err)
		}
		raw = string(encoded)
	}
	var params struct {
		MessageID int64 `json:"message_id"`
	}
	if err := json.Unmarshal([]byte(raw), &params); err != nil {
		t.Fatalf("decode reply_parameters %q: %v", raw, err)
	}
	return params.MessageID
}
