package modules

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/config"
)

// TestChatEndToEnd is the whole feature through the real dispatcher, with only Telegram and the
// provider faked: the group's owner turns conversations on from the private settings page (the
// toggle, the card, Yes), a member mentions the bot, the real Anthropic adapter asks the stand-in
// and the reply is posted as a reply; a second member replies to that answer and the next request
// carries the first exchange.
func TestChatEndToEnd(t *testing.T) {
	withChatProviders(t, true, true, true)
	settings := settingsChatEnv(t)
	owner := settings.newOwner(settings.chat)
	private := settingsPrivateChat(owner)
	msgID := settingsChatOpenPrivate(t, settings, owner)
	_, keyboard := settings.lastEdit(owner.Id, msgID)
	toggle, ok := settingsChatFind(t, keyboard, func(f map[string]string) bool { return f["a"] == "tg" && f["t"] == settingsChatControlSwitch })
	if !ok {
		t.Fatalf("no toggle on the page: %+v", keyboard)
	}
	settings.pressRaw(private, owner, msgID, toggle.CallbackData)
	_, keyboard = settings.lastEdit(owner.Id, msgID)
	yes, ok := settingsChatFind(t, keyboard, func(f map[string]string) bool { return f["a"] == settingsActChatYes })
	if !ok {
		t.Fatalf("no Yes on the card: %+v", keyboard)
	}
	settings.pressRaw(private, owner, msgID, yes.CallbackData)
	if !settingsChatStored(t, settings.chat.Id).Enabled {
		t.Fatalf("Yes did not turn conversations on")
	}

	env := newChatEnvFor(t, settings.chat.Id)
	standIn := &chatProviderStandIn{}
	server := httptest.NewServer(http.HandlerFunc(standIn.handler))
	t.Cleanup(server.Close)
	chatSwap(t, &chatProviderOf, chatProviderFor)
	chatSwap(t, &chatAnthropicEndpoint, server.URL)
	chatSwap(t, &chatProviderHTTP, server.Client())
	chatSwap(t, &config.AppConfig.ChatAnthropicModel, "claude-haiku-5-5")

	first := env.mention(env.member("Alice"), "what is the KS project?")
	env.deliver(first)
	if sends := env.sends(); len(sends) != 1 || sends[0] != "Claude unit on duty." {
		t.Fatalf("sends = %q, want the provider's reply", sends)
	}
	if reply := chatReplyTo(t, env.lastSend()); reply.MessageId != first.MessageId {
		t.Fatalf("the reply answers message %d, want %d", reply.MessageId, first.MessageId)
	}

	second := env.message(env.member("Bob"), "and who ran it?")
	second.ReplyToMessage = &gotgbot.Message{MessageId: 9001, Date: 1, Chat: second.Chat, From: &env.bot.User, Text: "Claude unit on duty."}
	env.deliver(second)
	if sends := env.sends(); len(sends) != 2 {
		t.Fatalf("sends after the reply = %q, want a second answer", sends)
	}
	if len(standIn.bodies) != 2 {
		t.Fatalf("provider calls = %d, want 2", len(standIn.bodies))
	}
	var body struct {
		Messages []struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal([]byte(standIn.bodies[1]), &body); err != nil || len(body.Messages) != 1 {
		t.Fatalf("second body: %v", err)
	}
	user := body.Messages[0].Content[0].Text
	for _, want := range []string{"Alice: what is the KS project?", "Xenobot-72: Claude unit on duty.", "Bob: and who ran it?"} {
		if !strings.Contains(user, want) {
			t.Fatalf("the second request lacks %q:\n%s", want, user)
		}
	}
}
