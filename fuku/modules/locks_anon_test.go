//go:build testtools

package modules

import (
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"

	"github.com/uasneppy/fuku_robot/fuku/db/locks"
)

func TestLockCommandsAnonymousAdmin(t *testing.T) {
	t.Run("AnonAdmin on", func(t *testing.T) {
		env, _ := newAnonLockdownEnv(t, true)
		LoadLocks(env.dispatcher)

		env.sendAnonymous("/lock url")
		if !locks.IsPermLocked(env.chat.Id, "url") {
			t.Fatal("anonymous /lock url with AnonAdmin on stored nothing")
		}
		env.wantReplyHas(staffMarker("locks_locked_successfully"))
		env.wantNoBotReplyContains(staffMarker("chat_status_anon_confirm"))

		env.sendAnonymous("/unlock url")
		if locks.IsPermLocked(env.chat.Id, "url") {
			t.Fatal("anonymous /unlock url with AnonAdmin on left the lock")
		}
		env.wantReplyHas(staffMarker("locks_unlocked_successfully"))
	})

	t.Run("AnonAdmin off", func(t *testing.T) {
		env, creator := newAnonLockdownEnv(t, false)
		LoadLocks(env.dispatcher)
		member := env.newJoiner("Member")
		env.fake.setMember(env.chat.Id, member.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})

		env.sendAnonymous("/lock url")
		env.wantProofPrompt()
		if locks.IsPermLocked(env.chat.Id, "url") {
			t.Fatal("anonymous /lock url stored the lock before the proof")
		}
		env.tapProof(member)
		if locks.IsPermLocked(env.chat.Id, "url") {
			t.Fatal("a tap by a plain member stored the lock")
		}
		env.tapProof(creator)
		if !locks.IsPermLocked(env.chat.Id, "url") {
			t.Fatal("the tap by a live admin stored nothing")
		}
		env.wantReplyHas(staffMarker("locks_locked_successfully"))

		env.sendAnonymous("/unlock url")
		env.wantProofPrompt()
		if !locks.IsPermLocked(env.chat.Id, "url") {
			t.Fatal("anonymous /unlock url removed the lock before the proof")
		}
		env.tapProof(creator)
		if locks.IsPermLocked(env.chat.Id, "url") {
			t.Fatal("the tap by a live admin left the lock")
		}
		env.wantReplyHas(staffMarker("locks_unlocked_successfully"))
	})

	t.Run("real senders", func(t *testing.T) {
		env, _ := newAnonLockdownEnv(t, false)
		LoadLocks(env.dispatcher)
		realAdmin := gotgbot.User{Id: uniqueLinkOwnerID(), FirstName: "Real Admin"}
		env.fake.listAdministrator(env.chat.Id, realAdmin.Id)
		member := env.newJoiner("Member")
		env.fake.setMember(env.chat.Id, member.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})

		env.send(member, "/lock url")
		if locks.IsPermLocked(env.chat.Id, "url") {
			t.Fatal("a plain member's /lock url stored the lock")
		}
		env.send(realAdmin, "/lock url")
		if !locks.IsPermLocked(env.chat.Id, "url") {
			t.Fatal("a real administrator's /lock url stored nothing")
		}
		env.wantReplyHas(staffMarker("locks_locked_successfully"))
	})
}

// lockWatcherDeletes runs both lock watchers on msg in a chat with the named locks set and
// returns how many messages the bot deleted.
func lockWatcherDeletes(t *testing.T, msg *gotgbot.Message, lockKinds ...string) int {
	t.Helper()
	client := newModuleBotClient()
	bot := newModuleTestBot(client)
	for _, kind := range lockKinds {
		if err := locks.UpdateLock(msg.Chat.Id, kind, true); err != nil {
			t.Fatalf("UpdateLock(%s) error = %v", kind, err)
		}
	}
	for _, handler := range []func(*gotgbot.Bot, *ext.Context) error{locksModule.permHandler, locksModule.restHandler} {
		ctx := ext.NewContext(bot, &gotgbot.Update{UpdateId: 1, Message: msg}, nil)
		if err := handler(bot, ctx); err != ext.ContinueGroups {
			t.Fatalf("lock watcher error = %v, want ContinueGroups", err)
		}
	}
	return len(client.callsFor("deleteMessage"))
}

// lockedLinkMessage is a forwarded message holding a link, sent in chat by from, posted as
// senderChat when it is set.
func lockedLinkMessage(chat gotgbot.Chat, from gotgbot.User, senderChat *gotgbot.Chat, automatic bool) *gotgbot.Message {
	return &gotgbot.Message{
		MessageId:          101,
		Date:               1,
		Chat:               chat,
		From:               &from,
		SenderChat:         senderChat,
		IsAutomaticForward: automatic,
		Text:               "https://example.com",
		Entities:           []gotgbot.MessageEntity{{Type: "url", Offset: 0, Length: 19}},
		ForwardOrigin:      gotgbot.MessageOriginChannel{Date: 1, Chat: gotgbot.Chat{Id: -1009926116150, Type: "channel"}, MessageId: 5},
	}
}

func TestLockWatchersExemptAnonymousAdminAndLinkedChannel(t *testing.T) {
	anonBot := gotgbot.User{Id: 1087968824, IsBot: true, FirstName: "Group", Username: "GroupAnonymousBot"}
	telegram := gotgbot.User{Id: 777000, FirstName: "Telegram"}
	channelBot := gotgbot.User{Id: 136817688, IsBot: true, FirstName: "Channel", Username: "Channel_Bot"}
	member := gotgbot.User{Id: 42, FirstName: "Member"}
	everyLock := []string{"all", "url", "forward", "comments"}

	for _, tc := range []struct {
		name  string
		build func(chat gotgbot.Chat) *gotgbot.Message
		locks []string
		want  int
	}{
		{"anonymous admin", func(chat gotgbot.Chat) *gotgbot.Message {
			group := chat
			return lockedLinkMessage(chat, anonBot, &group, false)
		}, everyLock, 0},
		{"linked channel automatic forward", func(chat gotgbot.Chat) *gotgbot.Message {
			channel := gotgbot.Chat{Id: -1009926116151, Type: "channel", Title: "Linked"}
			return lockedLinkMessage(chat, telegram, &channel, true)
		}, everyLock, 0},
		{"member control", func(chat gotgbot.Chat) *gotgbot.Message {
			return lockedLinkMessage(chat, member, nil, false)
		}, everyLock, 2},
		{"anonymous channel under all", func(chat gotgbot.Chat) *gotgbot.Message {
			channel := gotgbot.Chat{Id: -1009926116152, Type: "channel", Title: "Elsewhere"}
			return lockedLinkMessage(chat, channelBot, &channel, false)
		}, []string{"all"}, 1},
		{"anonymous channel under anonchannel", func(chat gotgbot.Chat) *gotgbot.Message {
			channel := gotgbot.Chat{Id: -1009926116153, Type: "channel", Title: "Elsewhere"}
			return lockedLinkMessage(chat, channelBot, &channel, false)
		}, []string{"anonchannel"}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			chat := gotgbot.Chat{Id: uniqueModuleChatID(), Type: "supergroup", Title: "Lock Chat"}
			if got := lockWatcherDeletes(t, tc.build(chat), tc.locks...); got != tc.want {
				t.Fatalf("deleteMessage calls = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestAnonChannelLockIgnoresLinkedChannel(t *testing.T) {
	chat := gotgbot.Chat{Id: uniqueModuleChatID(), Type: "supergroup", Title: "Lock Chat"}
	filter := lockMap["anonchannel"]
	group := chat
	linked := gotgbot.Chat{Id: -1009926116161, Type: "channel", Title: "Linked"}
	other := gotgbot.Chat{Id: -1009926116162, Type: "channel", Title: "Other"}

	if !filter(&gotgbot.Message{Chat: chat, SenderChat: &other, From: &gotgbot.User{Id: 136817688, IsBot: true}}) {
		t.Error("anonchannel does not match an anonymous channel's post")
	}
	if filter(&gotgbot.Message{Chat: chat, SenderChat: &linked, IsAutomaticForward: true, From: &gotgbot.User{Id: 777000}}) {
		t.Error("anonchannel matches the linked channel's automatic forward")
	}
	if filter(&gotgbot.Message{Chat: chat, SenderChat: &group, From: &gotgbot.User{Id: 1087968824, IsBot: true}}) {
		t.Error("anonchannel matches an anonymous admin's message")
	}
}
