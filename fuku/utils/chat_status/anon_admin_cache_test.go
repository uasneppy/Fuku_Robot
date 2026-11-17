//go:build testtools

package chat_status

import (
	"fmt"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/utils/cache"
)

// anonCommandChat is the supergroup the proof-cache tests store their commands for.
var anonCommandChat = gotgbot.Chat{Id: -1009926116120, Type: "supergroup", Title: "Anon cache chat"}

// anonCommand is an anonymous admin's command that replies to target.
func anonCommand(id int64, target *gotgbot.Message) *gotgbot.Message {
	senderChat := anonCommandChat
	return &gotgbot.Message{
		MessageId:  id,
		Date:       1700000000,
		Chat:       anonCommandChat,
		From:       &gotgbot.User{Id: groupAnonymousBot, IsBot: true, FirstName: "Group", Username: "GroupAnonymousBot"},
		SenderChat: &senderChat,
		Text:       "/ban spam",
		Entities: []gotgbot.MessageEntity{
			{Type: gotgbot.MessageEntityTypeBotCommand, Offset: 0, Length: 4},
		},
		ReplyToMessage: target,
	}
}

func TestAnonAdminCommandRoundTrip(t *testing.T) {
	cache.SetupTestMemoryMarshaler(t)

	target := gotgbot.User{Id: 424242, FirstName: "Target"}
	original := gotgbot.User{Id: 515151, FirstName: "Original"}
	forwarded := &gotgbot.Message{
		MessageId:     70,
		Date:          1699999000,
		Chat:          anonCommandChat,
		From:          &target,
		Text:          "forwarded spam",
		ForwardOrigin: gotgbot.MessageOriginUser{Date: 1699990000, SenderUser: original},
	}
	pinService := &gotgbot.Message{
		MessageId: 71,
		Date:      1699999100,
		Chat:      anonCommandChat,
		From:      &target,
		PinnedMessage: gotgbot.Message{
			MessageId: 60,
			Date:      1699998000,
			Chat:      anonCommandChat,
			From:      &original,
			Text:      "pinned text",
		},
	}

	for _, tc := range []struct {
		name   string
		cmdID  int64
		target *gotgbot.Message
		check  func(t *testing.T, reply *gotgbot.Message)
	}{
		{
			name:   "reply to a forwarded message",
			cmdID:  801,
			target: forwarded,
			check: func(t *testing.T, reply *gotgbot.Message) {
				origin, ok := reply.ForwardOrigin.(gotgbot.MessageOriginUser)
				if !ok {
					t.Fatalf("ForwardOrigin = %T, want gotgbot.MessageOriginUser", reply.ForwardOrigin)
				}
				if origin.SenderUser.Id != original.Id || origin.Date != 1699990000 {
					t.Fatalf("ForwardOrigin = %#v, want the original sender and date", origin)
				}
			},
		},
		{
			name:   "reply to a pin service message",
			cmdID:  802,
			target: pinService,
			check: func(t *testing.T, reply *gotgbot.Message) {
				pinned, ok := reply.PinnedMessage.(gotgbot.Message)
				if !ok {
					t.Fatalf("PinnedMessage = %T, want gotgbot.Message", reply.PinnedMessage)
				}
				if pinned.MessageId != 60 || pinned.Text != "pinned text" || pinned.From == nil || pinned.From.Id != original.Id {
					t.Fatalf("PinnedMessage = %#v, want the pinned message", pinned)
				}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stored := anonCommand(tc.cmdID, tc.target)
			if err := StoreAnonAdminCommand(anonCommandChat.Id, stored); err != nil {
				t.Fatalf("StoreAnonAdminCommand() error = %v", err)
			}
			got, err := LoadAnonAdminCommand(anonCommandChat.Id, tc.cmdID)
			if err != nil {
				t.Fatalf("LoadAnonAdminCommand() error = %v", err)
			}
			if got.MessageId != stored.MessageId || got.Text != stored.Text || got.Chat.Id != anonCommandChat.Id {
				t.Fatalf("loaded command = %#v, want the stored one", got)
			}
			if got.From == nil || got.From.Id != groupAnonymousBot || got.SenderChat == nil || got.SenderChat.Id != anonCommandChat.Id {
				t.Fatalf("loaded sender = %#v / %#v, want the anonymous admin", got.From, got.SenderChat)
			}
			if len(got.Entities) != 1 || got.Entities[0].Type != gotgbot.MessageEntityTypeBotCommand {
				t.Fatalf("loaded entities = %#v, want the bot command entity", got.Entities)
			}
			reply := got.ReplyToMessage
			if reply == nil || reply.MessageId != tc.target.MessageId || reply.From == nil || reply.From.Id != target.Id {
				t.Fatalf("loaded reply target = %#v, want message %d from %d", reply, tc.target.MessageId, target.Id)
			}
			tc.check(t, reply)
		})
	}
}

func TestAnonAdminCommandOldFormatReadsAsExpired(t *testing.T) {
	cache.SetupTestMemoryMarshaler(t)

	// The format before the fix: the whole message through the msgpack marshaler.
	old := anonCommand(901, &gotgbot.Message{MessageId: 9, Date: 1699999000, Chat: anonCommandChat, Text: "plain"})
	key := fmt.Sprintf("fuku:anonAdmin:%d:%d", anonCommandChat.Id, old.MessageId)
	if err := cache.GetMarshal().Set(cache.Context, key, old); err != nil {
		t.Fatalf("old-format set: %v", err)
	}

	got, err := LoadAnonAdminCommand(anonCommandChat.Id, old.MessageId)
	if err == nil || got != nil {
		t.Fatalf("LoadAnonAdminCommand(old format) = (%#v, %v), want nil and an error", got, err)
	}

	// A missing key is an error too.
	if got, err := LoadAnonAdminCommand(anonCommandChat.Id, 902); err == nil || got != nil {
		t.Fatalf("LoadAnonAdminCommand(missing) = (%#v, %v), want nil and an error", got, err)
	}
}
