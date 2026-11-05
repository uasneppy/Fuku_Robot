//go:build testtools

package modules

import (
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/greetings"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// greetOnceSetup stores a welcome text for a fresh supergroup and returns the chat and a bot over a fresh fake.
func greetOnceSetup(t *testing.T) (gotgbot.Chat, *moduleBotClient, *gotgbot.Bot) {
	t.Helper()
	client := newModuleBotClient()
	chat := gotgbot.Chat{Id: uniqueModuleChatID(), Type: "supergroup", Title: "Greeting Chat"}
	if err := greetings.SetWelcomeText(chat.Id, "Welcome {first}", "", nil, db.TEXT); err != nil {
		t.Fatalf("SetWelcomeText setup error = %v", err)
	}
	return chat, client, newModuleTestBot(client)
}

// greetOnceJoin drives one join of the member through the chat_member handler, with the in-process dedupe cleared
// first so a second call is a real second join and not the same one delivered twice.
func greetOnceJoin(t *testing.T, bot *gotgbot.Bot, chat gotgbot.Chat, member gotgbot.User) {
	t.Helper()
	admin := gotgbot.User{Id: 777000, FirstName: "Telegram"}
	ctx := newChatMemberContext(bot, chat, admin,
		gotgbot.ChatMemberLeft{User: member}, gotgbot.ChatMemberMember{User: member})
	clearRecentJoinProcessing(chat.Id, member.Id)
	if err := greetingsModule.newMember(bot, ctx); err != ext.EndGroups {
		t.Fatalf("newMember error = %v, want EndGroups", err)
	}
}

// greetOnceLeave drives the member's leave through the chat_member handler.
func greetOnceLeave(t *testing.T, bot *gotgbot.Bot, chat gotgbot.Chat, member gotgbot.User) {
	t.Helper()
	admin := gotgbot.User{Id: 777000, FirstName: "Telegram"}
	ctx := newChatMemberContext(bot, chat, admin,
		gotgbot.ChatMemberMember{User: member}, gotgbot.ChatMemberLeft{User: member})
	if err := greetingsModule.leftMember(bot, ctx); err != ext.EndGroups {
		t.Fatalf("leftMember error = %v, want EndGroups", err)
	}
}

// greetedRows counts the greeted_members rows of one chat.
func greetedRows(t *testing.T, chatID int64) int64 {
	t.Helper()
	var n int64
	if err := db.DB.Model(&models.GreetedMember{}).Where("chat_id = ?", chatID).Count(&n).Error; err != nil {
		t.Fatalf("count greeted_members: %v", err)
	}
	return n
}

// greetOnceWelcomes counts the welcome messages the fake received (the goodbye a leave sends is not one).
func greetOnceWelcomes(client *moduleBotClient) int {
	n := 0
	for _, call := range client.callsFor("sendMessage") {
		if text, _ := call.Params["text"].(string); strings.Contains(text, "Welcome") {
			n++
		}
	}
	return n
}

func TestGreetOnceFirstJoinOnly(t *testing.T) {
	chat, client, bot := greetOnceSetup(t)
	memberA := gotgbot.User{Id: 4242, FirstName: "Alice"}
	memberB := gotgbot.User{Id: 4343, FirstName: "Bob"}
	if res := applyWelcomeOnce(t.Context(), chat.Id, true); res.Outcome != greetingApplied {
		t.Fatalf("applyWelcomeOnce(true) outcome = %v, err = %v", res.Outcome, res.Err)
	}

	greetOnceJoin(t, bot, chat, memberA)
	if got := greetOnceWelcomes(client); got != 1 {
		t.Fatalf("welcomes after A's first join = %d, want 1", got)
	}
	if got := greetedRows(t, chat.Id); got != 1 {
		t.Fatalf("greeted rows after A's first join = %d, want 1", got)
	}

	greetOnceLeave(t, bot, chat, memberA)
	greetOnceJoin(t, bot, chat, memberA)
	if got := greetOnceWelcomes(client); got != 1 {
		t.Fatalf("welcomes after A rejoined = %d, want still 1", got)
	}
	if got := greetedRows(t, chat.Id); got != 1 {
		t.Fatalf("greeted rows after A rejoined = %d, want still 1", got)
	}

	greetOnceJoin(t, bot, chat, memberB)
	if got := greetOnceWelcomes(client); got != 2 {
		t.Fatalf("welcomes after B's first join = %d, want 2", got)
	}

	if res := applyWelcomeOnce(t.Context(), chat.Id, false); res.Outcome != greetingApplied {
		t.Fatalf("applyWelcomeOnce(false) outcome = %v, err = %v", res.Outcome, res.Err)
	}
	greetOnceJoin(t, bot, chat, memberA)
	if got := greetOnceWelcomes(client); got != 3 {
		t.Fatalf("welcomes after A joined with Greet once off = %d, want 3", got)
	}
}
