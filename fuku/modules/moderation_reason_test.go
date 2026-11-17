package modules

import (
	"fmt"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
)

func TestModerationRepliesEscapeReason(t *testing.T) {
	withRealLocales(t)
	const reason = "<b>raid & 100%d</b>"
	const escaped = "&lt;b&gt;raid &amp; 100%d&lt;/b&gt;"
	admin := gotgbot.User{Id: 777000, FirstName: "Telegram"}
	target := gotgbot.User{Id: 42, FirstName: "Member"}

	cases := []struct {
		name   string
		text   string
		run    func(*gotgbot.Bot, *ext.Context) error
		method string
	}{
		{name: "kick", text: "/kick " + reason, run: bansModule.kick, method: "unbanChatMember"},
		{name: "ban", text: "/ban " + reason, run: bansModule.ban, method: "banChatMember"},
		{name: "tban", text: "/tban 1h " + reason, run: bansModule.tBan, method: "banChatMember"},
		{name: "mute", text: "/mute " + reason, run: mutesModule.mute, method: "restrictChatMember"},
		{name: "tmute", text: "/tmute 1h " + reason, run: mutesModule.tMute, method: "restrictChatMember"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := newModuleBotClient()
			bot := newModuleTestBot(client)
			chat := gotgbot.Chat{Id: uniqueModuleChatID(), Type: "supergroup", Title: "Reason Chat"}
			ctx := newBanReplyContext(bot, chat, admin, target, tc.text)
			if err := tc.run(bot, ctx); err != ext.EndGroups {
				t.Fatalf("%s error = %v, want EndGroups", tc.text, err)
			}
			if calls := client.callsFor(tc.method); len(calls) != 1 {
				t.Fatalf("%s calls = %d, want 1", tc.method, len(calls))
			}
			sends := client.callsFor("sendMessage")
			if len(sends) != 1 {
				t.Fatalf("sendMessage calls = %d, want 1", len(sends))
			}
			text := fmt.Sprint(sends[0].Params["text"])
			if !strings.Contains(text, escaped) {
				t.Fatalf("reply = %q, want the reason escaped as %q", text, escaped)
			}
			if strings.Contains(text, "%!") {
				t.Fatalf("reply = %q, want no format-verb noise", text)
			}
			if !strings.Contains(text, "<b>") || !strings.Contains(text, "tg://user?id=") {
				t.Fatalf("reply = %q, want the template's bold reason label and the target's mention", text)
			}
		})
	}
}

func TestGetIdEscapesNames(t *testing.T) {
	withRealLocales(t)
	client := newModuleBotClient()
	bot := newModuleTestBot(client)
	chat := gotgbot.Chat{Id: uniqueModuleChatID(), Type: "supergroup", Title: "Misc Chat"}
	sender := gotgbot.User{Id: 42, FirstName: "Member"}
	id, _ := seedTargetUser(t, "trnamed", "<i>x</i> & y")

	if err := miscModule.getId(bot, newModuleMessageContext(bot, chat, sender, fmt.Sprintf("/id %d", id))); err != ext.EndGroups {
		t.Fatalf("getId error = %v, want EndGroups", err)
	}
	sends := client.callsFor("sendMessage")
	if len(sends) != 1 {
		t.Fatalf("sendMessage calls = %d, want 1", len(sends))
	}
	text := fmt.Sprint(sends[0].Params["text"])
	if !strings.Contains(text, "&lt;i&gt;x&lt;/i&gt; &amp; y") || strings.Contains(text, "<i>") {
		t.Fatalf("/id reply = %q, want the stored name escaped", text)
	}
}
