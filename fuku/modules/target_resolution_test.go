package modules

import (
	"fmt"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"

	"github.com/uasneppy/fuku_robot/fuku/db"
	dbcache "github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/db/user"
)

// seedTargetUser stores a users row under a fresh ID and a username of its own, and
// deletes it again when the test ends.
func seedTargetUser(t *testing.T, prefix, name string) (int64, string) {
	t.Helper()
	id := uniqueLinkOwnerID()
	username := fmt.Sprintf("%s%d", prefix, id)
	if err := user.EnsureUserInDb(id, username, name); err != nil {
		t.Fatalf("EnsureUserInDb() error = %v", err)
	}
	t.Cleanup(func() {
		db.DB.Where("user_id = ?", id).Delete(&models.User{})
		dbcache.DeleteCache(dbcache.CacheKey("user", id))
	})
	return id, username
}

func TestBanExplicitTargetBeatsReply(t *testing.T) {
	client := newModuleBotClient()
	bot := newModuleTestBot(client)
	chat := gotgbot.Chat{Id: uniqueModuleChatID(), Type: "supergroup", Title: "Ban Chat"}
	admin := gotgbot.User{Id: 777000, FirstName: "Telegram"}
	bob := gotgbot.User{Id: 42, FirstName: "Bob"}
	aliceID, aliceName := seedTargetUser(t, "tralice", "Alice")

	ctx := newBanReplyContext(bot, chat, admin, bob, "/ban @"+aliceName+" spam")
	if err := bansModule.ban(bot, ctx); err != ext.EndGroups {
		t.Fatalf("ban() error = %v, want EndGroups", err)
	}
	calls := client.callsFor("banChatMember")
	if len(calls) != 1 {
		t.Fatalf("banChatMember calls = %d, want 1", len(calls))
	}
	if got := fmt.Sprint(calls[0].Params["user_id"]); got != fmt.Sprint(aliceID) {
		t.Fatalf("banChatMember user_id = %s, want Alice %d (never the replied Bob)", got, aliceID)
	}
}

func TestGetIdExplicitTargetBeatsReply(t *testing.T) {
	withRealLocales(t)
	client := newModuleBotClient()
	bot := newModuleTestBot(client)
	chat := gotgbot.Chat{Id: uniqueModuleChatID(), Type: "supergroup", Title: "Misc Chat"}
	sender := gotgbot.User{Id: 42, FirstName: "Member"}
	aliceID, aliceName := seedTargetUser(t, "tralice", "Alice")
	bobID := uniqueLinkOwnerID()

	reply := func(text string) *ext.Context {
		ctx := newModuleMessageContext(bot, chat, sender, text)
		ctx.EffectiveMessage.ReplyToMessage = &gotgbot.Message{
			MessageId: 55,
			Date:      1,
			Chat:      chat,
			From:      &gotgbot.User{Id: bobID, FirstName: "Bob"},
			Text:      "target",
		}
		return ctx
	}

	if err := miscModule.getId(bot, reply("/id @"+aliceName)); err != ext.EndGroups {
		t.Fatalf("getId explicit error = %v, want EndGroups", err)
	}
	if err := miscModule.getId(bot, reply("/id")); err != ext.EndGroups {
		t.Fatalf("getId reply error = %v, want EndGroups", err)
	}
	calls := client.callsFor("sendMessage")
	if len(calls) != 2 {
		t.Fatalf("sendMessage calls = %d, want 2", len(calls))
	}
	explicit := fmt.Sprint(calls[0].Params["text"])
	if !strings.Contains(explicit, fmt.Sprint(aliceID)) || strings.Contains(explicit, fmt.Sprint(bobID)) {
		t.Fatalf("/id @alice as a reply to Bob = %q, want Alice's ID and not Bob's", explicit)
	}
	bare := fmt.Sprint(calls[1].Params["text"])
	if !strings.Contains(bare, fmt.Sprint(bobID)) || !strings.Contains(bare, fmt.Sprint(chat.Id)) {
		t.Fatalf("bare /id as a reply = %q, want the reply details", bare)
	}
}
