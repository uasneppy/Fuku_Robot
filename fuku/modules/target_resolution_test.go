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

// forumTopicContext is a command sent inside a forum topic. With explicit false the
// message is not a reply, so Telegram sets reply_to_message to the topic's creation
// message; with explicit true it replies to a member's message in the topic.
func forumTopicContext(bot *gotgbot.Bot, chat gotgbot.Chat, from gotgbot.User, text string, explicit bool) *ext.Context {
	const threadID = 70
	ctx := newModuleMessageContext(bot, chat, from, text)
	ctx.EffectiveMessage.MessageId = 400
	ctx.EffectiveMessage.IsTopicMessage = true
	ctx.EffectiveMessage.MessageThreadId = threadID
	if explicit {
		ctx.EffectiveMessage.ReplyToMessage = &gotgbot.Message{
			MessageId:       390,
			Date:            1,
			Chat:            chat,
			From:            &gotgbot.User{Id: 42, FirstName: "Member"},
			Text:            "spam",
			IsTopicMessage:  true,
			MessageThreadId: threadID,
		}
		return ctx
	}
	ctx.EffectiveMessage.ReplyToMessage = &gotgbot.Message{
		MessageId:         threadID,
		Date:              1,
		Chat:              chat,
		From:              &gotgbot.User{Id: 5151, FirstName: "Topic Creator"},
		ForumTopicCreated: &gotgbot.ForumTopicCreated{Name: "topic"},
	}
	return ctx
}

func TestForumTopicImplicitReplyIsNoTarget(t *testing.T) {
	withRealLocales(t)
	admin := gotgbot.User{Id: 777000, FirstName: "Telegram"}
	destructive := []string{"banChatMember", "restrictChatMember", "unbanChatMember", "deleteMessage"}

	type handler func(*gotgbot.Bot, *ext.Context) error
	cases := []struct {
		name string
		text string
		run  handler
		// acts is the method a real reply inside the topic must reach.
		acts string
	}{
		{name: "ban", text: "/ban", run: bansModule.ban, acts: "banChatMember"},
		{name: "mute", text: "/mute", run: mutesModule.mute, acts: "restrictChatMember"},
		{name: "dkick", text: "/dkick", run: bansModule.dkick, acts: "unbanChatMember"},
		{name: "purge", text: "/purge", run: purgesModule.purge, acts: "deleteMessage"},
		{name: "del", text: "/del", run: purgesModule.delCmd, acts: "deleteMessage"},
		{name: "purgefrom", text: "/purgefrom", run: purgesModule.purgeFrom},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			chat := gotgbot.Chat{Id: uniqueModuleChatID(), Type: "supergroup", Title: "Forum", IsForum: true}
			t.Cleanup(func() { delMsgs.Delete(chat.Id) })

			client := newModuleBotClient()
			bot := newModuleTestBot(client)
			if err := tc.run(bot, forumTopicContext(bot, chat, admin, tc.text, false)); err != ext.EndGroups {
				t.Fatalf("%s implicit error = %v, want EndGroups", tc.text, err)
			}
			for _, method := range destructive {
				if calls := client.callsFor(method); len(calls) != 0 {
					t.Fatalf("%s on the topic's creation message made %d %s calls, want none", tc.text, len(calls), method)
				}
			}
			if calls := client.callsFor("sendMessage"); len(calls) != 1 {
				t.Fatalf("%s implicit sendMessage calls = %d, want one refusal", tc.text, len(calls))
			}
			if _, marked := delMsgs.Load(chat.Id); marked {
				t.Fatalf("%s marked the topic's creation message for a purge", tc.text)
			}
			if tc.acts == "" {
				return
			}

			client = newModuleBotClient()
			bot = newModuleTestBot(client)
			if err := tc.run(bot, forumTopicContext(bot, chat, admin, tc.text, true)); err != ext.EndGroups {
				t.Fatalf("%s explicit error = %v, want EndGroups", tc.text, err)
			}
			if calls := client.callsFor(tc.acts); len(calls) == 0 {
				t.Fatalf("%s as a real reply in the topic made no %s call", tc.text, tc.acts)
			}
		})
	}

	t.Run("id", func(t *testing.T) {
		chat := gotgbot.Chat{Id: uniqueModuleChatID(), Type: "supergroup", Title: "Forum", IsForum: true}
		client := newModuleBotClient()
		bot := newModuleTestBot(client)
		if err := miscModule.getId(bot, forumTopicContext(bot, chat, admin, "/id", false)); err != ext.EndGroups {
			t.Fatalf("/id implicit error = %v, want EndGroups", err)
		}
		calls := client.callsFor("sendMessage")
		if len(calls) != 1 {
			t.Fatalf("/id sendMessage calls = %d, want 1", len(calls))
		}
		text := fmt.Sprint(calls[0].Params["text"])
		if !strings.Contains(text, "777000") || strings.Contains(text, "5151") {
			t.Fatalf("/id in a topic = %q, want the sender's own ID and not the topic creator's", text)
		}
	})
}
