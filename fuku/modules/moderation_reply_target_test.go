package modules

import (
	"fmt"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"

	"github.com/uasneppy/fuku_robot/fuku/db/warns"
)

// TestDeleteCommandsRefuseMismatchedReply holds M1: /dban, /dmute and /dwarn
// with an explicit target that is not the sender of the replied message delete
// nothing and punish nobody; they answer one refusal.
func TestDeleteCommandsRefuseMismatchedReply(t *testing.T) {
	withRealLocales(t)
	cases := []struct {
		name string
		cmd  string
		run  func(*gotgbot.Bot, *ext.Context) error
	}{
		{name: "dban", cmd: "/dban", run: bansModule.dBan},
		{name: "dmute", cmd: "/dmute", run: mutesModule.dMute},
		{name: "dwarn", cmd: "/dwarn", run: warnsModule.dWarnUser},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := newModuleBotClient()
			bot := newModuleTestBot(client)
			chat := gotgbot.Chat{Id: uniqueModuleChatID(), Type: "supergroup", Title: "Reply Chat"}
			admin := gotgbot.User{Id: 777000, FirstName: "Telegram"}
			author := gotgbot.User{Id: uniqueLinkOwnerID(), FirstName: "Author"}
			other := uniqueLinkOwnerID()

			ctx := newBanReplyContext(bot, chat, admin, author, fmt.Sprintf("%s %d spam", tc.cmd, other))
			if err := tc.run(bot, ctx); err != ext.EndGroups {
				t.Fatalf("%s error = %v, want EndGroups", tc.name, err)
			}
			for _, method := range []string{"deleteMessage", "banChatMember", "restrictChatMember"} {
				if calls := client.callsFor(method); len(calls) != 0 {
					t.Fatalf("%s calls = %d, want none", method, len(calls))
				}
			}
			if n, _ := warns.GetWarns(other, chat.Id); n != 0 {
				t.Fatalf("warns of the explicit target = %d, want 0", n)
			}
			if n, _ := warns.GetWarns(author.Id, chat.Id); n != 0 {
				t.Fatalf("warns of the reply's author = %d, want 0", n)
			}
			sent := client.callsFor("sendMessage")
			if len(sent) != 1 || fmt.Sprint(sent[0].Params["text"]) == "" {
				t.Fatalf("sendMessage calls = %v, want one refusal", sent)
			}
		})
	}
}

// TestBanChannelActsOnExtractedChannel holds M2: /ban and /unban with a channel
// ID replying to another channel's post act on the channel named, never on the
// reply's sender.
func TestBanChannelActsOnExtractedChannel(t *testing.T) {
	named := int64(-1001000000000) - uniqueLinkOwnerID()%1000000
	replied := named - 1
	for _, tc := range []struct {
		name, cmd, method string
		run               func(*gotgbot.Bot, *ext.Context) error
	}{
		{name: "ban", cmd: "/ban", method: "banChatSenderChat", run: bansModule.ban},
		{name: "unban", cmd: "/unban", method: "unbanChatSenderChat", run: bansModule.unban},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := newModuleBotClient()
			bot := newModuleTestBot(client)
			chat := gotgbot.Chat{Id: uniqueModuleChatID(), Type: "supergroup", Title: "Ban Chat"}
			admin := gotgbot.User{Id: 777000, FirstName: "Telegram"}
			ctx := newModuleMessageContext(bot, chat, admin, fmt.Sprintf("%s %d", tc.cmd, named))
			ctx.EffectiveMessage.ReplyToMessage = &gotgbot.Message{
				MessageId:  304,
				Date:       1,
				Chat:       chat,
				SenderChat: &gotgbot.Chat{Id: replied, Type: "channel", Title: "Other Channel"},
				Text:       "channel post",
			}
			if err := tc.run(bot, ctx); err != ext.EndGroups {
				t.Fatalf("%s error = %v, want EndGroups", tc.name, err)
			}
			calls := client.callsFor(tc.method)
			if len(calls) != 1 {
				t.Fatalf("%s calls = %d, want 1", tc.method, len(calls))
			}
			if got := fmt.Sprint(calls[0].Params["sender_chat_id"]); got != fmt.Sprint(named) {
				t.Fatalf("%s sender_chat_id = %s, want the named channel %d", tc.method, got, named)
			}
		})
	}
}
