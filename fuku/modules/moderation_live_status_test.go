package modules

import (
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/stretchr/testify/require"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/chats"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// TestButtonsActOnlyOnMembersInChat holds M3: the report card's Kick, the
// /restrict card's Kick and Mute and the unrestrict card's Unmute read the live
// member first and make no write when the target is banned (14) or gone (13):
// a member-removing unban would lift a ban, and a restrict would replace it.
func TestButtonsActOnlyOnMembersInChat(t *testing.T) {
	admin := gotgbot.User{Id: 777000, FirstName: "Telegram"}
	cases := []struct {
		name string
		data func(user string) string
		run  func(*gotgbot.Bot, *ext.Context) error
	}{
		{name: "report kick", run: reportsModule.markResolvedButtonHandler, data: func(u string) string {
			return encodeCallbackData("report", map[string]string{"a": "kick", "u": u, "m": "55"})
		}},
		{name: "restrict kick", run: bansModule.restrictButtonHandler, data: func(u string) string {
			return encodeCallbackData("restrict", map[string]string{"a": "kick", "u": u})
		}},
		{name: "restrict mute", run: bansModule.restrictButtonHandler, data: func(u string) string {
			return encodeCallbackData("restrict", map[string]string{"a": "mute", "u": u})
		}},
		{name: "unrestrict unmute", run: bansModule.unrestrictButtonHandler, data: func(u string) string {
			return encodeCallbackData("unrestrict", map[string]string{"a": "unmute", "u": u})
		}},
	}
	for _, tc := range cases {
		for _, user := range []string{"13", "14"} {
			t.Run(tc.name+" "+user, func(t *testing.T) {
				client := newModuleBotClient()
				bot := newModuleTestBot(client)
				chat := gotgbot.Chat{Id: uniqueModuleChatID(), Type: "supergroup", Title: "Live Chat"}
				ctx := newModuleCallbackContext(bot, chat, admin, tc.data(user))
				if err := tc.run(bot, ctx); err != ext.EndGroups {
					t.Fatalf("error = %v, want EndGroups", err)
				}
				for _, method := range []string{"unbanChatMember", "restrictChatMember", "banChatMember", "editMessageText"} {
					if calls := client.callsFor(method); len(calls) != 0 {
						t.Fatalf("%s calls = %d, want none", method, len(calls))
					}
				}
				if calls := client.callsFor("answerCallbackQuery"); len(calls) != 1 {
					t.Fatalf("answerCallbackQuery calls = %d, want 1", len(calls))
				}
			})
		}
	}
}

// TestZombiesSkipsBannedDeletedAccounts holds M3 for /zombies: a deleted
// account that is banned is not "kicked", because the member-removing unban
// would lift its ban.
func TestZombiesSkipsBannedDeletedAccounts(t *testing.T) {
	client := newModuleBotClient()
	client.responses["getChatMember"] = []byte(
		`{"status":"kicked","user":{"id":88,"is_bot":false,"first_name":"Deleted Account"}}`,
	)
	bot := newModuleTestBot(client)
	chatID := uniqueModuleChatID()
	require.NoError(t, chats.EnsureChatInDb(chatID, "zombie chat"))
	require.NoError(t, db.DB.Model(&models.Chat{}).Where("chat_id = ?", chatID).
		Update("users", models.Int64Array{88}).Error)
	t.Cleanup(func() { _ = db.DB.Where("chat_id = ?", chatID).Delete(&models.Chat{}).Error })

	chat := gotgbot.Chat{Id: chatID, Type: "supergroup", Title: "Zombie Chat"}
	if err := miscModule.zombies(bot, newModuleMessageContext(bot, chat, admin777000(), "/zombies")); err != ext.EndGroups {
		t.Fatalf("zombies: %v", err)
	}
	if calls := client.callsFor("unbanChatMember"); len(calls) != 0 {
		t.Fatalf("unbanChatMember calls = %d, want none for a banned deleted account", len(calls))
	}
}

func admin777000() gotgbot.User { return gotgbot.User{Id: 777000, FirstName: "Telegram"} }
