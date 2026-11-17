//go:build testtools

package helpers

import (
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
)

// TestIsAutomaticForward holds the predicate and that a command handler built by NewBotCommand
// passes an automatic forward on with ext.ContinueGroups without calling its response, while any
// other sender of the same text runs it.
func TestIsAutomaticForward(t *testing.T) {
	channel := gotgbot.Chat{Id: -1001777, Type: "channel", Title: "News"}
	group := gotgbot.Chat{Id: -1001234, Type: "supergroup"}
	service := &gotgbot.User{Id: 777000, FirstName: "Telegram"}
	member := &gotgbot.User{Id: 77, FirstName: "Member"}
	cases := []struct {
		name string
		msg  *gotgbot.Message
		want bool
	}{
		{name: "nil", msg: nil, want: false},
		{name: "member", msg: &gotgbot.Message{Chat: group, From: member}, want: false},
		{name: "flag set", msg: &gotgbot.Message{Chat: group, From: service, SenderChat: &channel, IsAutomaticForward: true}, want: true},
		{name: "flag set, other from", msg: &gotgbot.Message{Chat: group, From: member, IsAutomaticForward: true}, want: true},
		{name: "service account on behalf of a chat", msg: &gotgbot.Message{Chat: group, From: service, SenderChat: &channel}, want: true},
		{name: "service account with no sender chat", msg: &gotgbot.Message{Chat: group, From: service}, want: false},
		{name: "member on behalf of a chat", msg: &gotgbot.Message{Chat: group, From: member, SenderChat: &channel}, want: false},
	}
	for _, tc := range cases {
		if got := IsAutomaticForward(tc.msg); got != tc.want {
			t.Errorf("%s: IsAutomaticForward = %v, want %v", tc.name, got, tc.want)
		}
	}

	bot := &gotgbot.Bot{Token: "999:test", User: gotgbot.User{Id: 999, IsBot: true, Username: "FukuTestBot"}}
	calls := 0
	handler := NewBotCommand("probeautofwd", func(*gotgbot.Bot, *ext.Context) error {
		calls++
		return ext.EndGroups
	})
	forward := commandUpdate(bot, "/probeautofwd")
	forward.EffectiveMessage.IsAutomaticForward = true
	forward.EffectiveMessage.From = service
	forward.EffectiveMessage.SenderChat = &channel
	if err := handler.HandleUpdate(bot, forward); err != ext.ContinueGroups || calls != 0 {
		t.Fatalf("automatic forward: err = %v, calls = %d, want ContinueGroups and no call", err, calls)
	}
	if err := handler.HandleUpdate(bot, commandUpdate(bot, "/probeautofwd")); err != ext.EndGroups || calls != 1 {
		t.Fatalf("member: err = %v, calls = %d, want EndGroups and one call", err, calls)
	}
}
