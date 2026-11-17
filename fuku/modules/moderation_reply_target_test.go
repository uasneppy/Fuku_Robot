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
