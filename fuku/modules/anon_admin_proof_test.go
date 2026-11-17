//go:build testtools

package modules

import (
	"fmt"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
)

// sendAnonymousMessage runs an anonymous admin's message (From is the placeholder bot,
// SenderChat the group) through the real dispatcher, replying to reply when it is set.
func (e *lockdownEnv) sendAnonymousMessage(text string, reply *gotgbot.Message) {
	e.t.Helper()
	id := e.updateID()
	from := gotgbot.User{Id: lockdownTestAnonBotID, IsBot: true, FirstName: "Group", Username: "GroupAnonymousBot"}
	senderChat := e.chat
	update := &gotgbot.Update{
		UpdateId: id,
		Message: &gotgbot.Message{
			MessageId:      id,
			Date:           1,
			Chat:           e.chat,
			From:           &from,
			SenderChat:     &senderChat,
			Text:           text,
			ReplyToMessage: reply,
		},
	}
	if err := e.dispatcher.ProcessUpdate(e.bot, update, nil); err != nil {
		e.t.Fatalf("ProcessUpdate(%d) error = %v", id, err)
	}
}

// answerCount is how many callback queries the bot answered, in any chat.
func (e *lockdownEnv) answerCount() int {
	return len(e.fake.callsFor("answerCallbackQuery"))
}

// newAnonBanEnv is an anonymous-admin environment (AnonAdmin off) with the Bans module
// loaded and one plain member to act on.
func newAnonBanEnv(t *testing.T) (*lockdownEnv, gotgbot.User, gotgbot.User) {
	t.Helper()
	env, creator := newAnonLockdownEnv(t, false)
	LoadBans(env.dispatcher)
	target := env.newJoiner("Target")
	env.fake.setMember(env.chat.Id, target.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})
	return env, creator, target
}

func TestAnonAdminProofCachesInterfaceFields(t *testing.T) {
	for _, tc := range []struct {
		name  string
		reply func(env *lockdownEnv, target gotgbot.User) *gotgbot.Message
	}{
		{
			name: "reply to a forwarded message",
			reply: func(env *lockdownEnv, target gotgbot.User) *gotgbot.Message {
				return &gotgbot.Message{
					MessageId: 40,
					Date:      1,
					Chat:      env.chat,
					From:      &target,
					Text:      "forwarded spam",
					ForwardOrigin: gotgbot.MessageOriginUser{
						Date:       1,
						SenderUser: gotgbot.User{Id: uniqueLinkOwnerID(), FirstName: "Origin"},
					},
				}
			},
		},
		{
			name: "reply to a pin service message",
			reply: func(env *lockdownEnv, target gotgbot.User) *gotgbot.Message {
				return &gotgbot.Message{
					MessageId: 41,
					Date:      1,
					Chat:      env.chat,
					From:      &target,
					PinnedMessage: gotgbot.Message{
						MessageId: 39,
						Date:      1,
						Chat:      env.chat,
						Text:      "pinned",
					},
				}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env, creator, target := newAnonBanEnv(t)

			env.sendAnonymousMessage("/ban", tc.reply(env, target))
			env.proofButton()
			if bans := env.calls("banChatMember"); len(bans) != 0 {
				t.Fatalf("banChatMember calls = %d before the proof, want 0", len(bans))
			}

			env.tapProof(creator)

			if edits := env.calls("editMessageText"); len(edits) != 0 {
				t.Fatalf("editMessageText calls = %d, want no expired-button edit", len(edits))
			}
			bans := env.calls("banChatMember")
			if len(bans) != 1 {
				t.Fatalf("banChatMember calls = %d after the proof, want 1", len(bans))
			}
			if got := staffParamInt(bans[0].Params, "user_id"); got != target.Id {
				t.Errorf("banned user = %d, want the replied message's sender %d", got, target.Id)
			}
		})
	}
}

// wantNoBotReplyContains fails when any reply to the chat holds marker.
func (e *lockdownEnv) wantNoBotReplyContains(marker string) {
	e.t.Helper()
	for _, r := range e.replies() {
		if strings.Contains(fmt.Sprint(r.Params["text"]), marker) {
			e.t.Errorf("a reply holds %q: %q", marker, r.Params["text"])
		}
	}
}
