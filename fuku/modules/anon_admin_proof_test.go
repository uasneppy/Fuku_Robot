//go:build testtools

package modules

import (
	"fmt"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/utils/chat_status"
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

// tapProofData presses an anonymous-admin proof button with the given callback data,
// on message msgID of chat, as from.
func (e *lockdownEnv) tapProofData(from gotgbot.User, chat gotgbot.Chat, msgID int64, data string) {
	e.t.Helper()
	id := e.updateID()
	update := &gotgbot.Update{
		UpdateId: id,
		CallbackQuery: &gotgbot.CallbackQuery{
			Id:           fmt.Sprintf("anon-%d", id),
			From:         from,
			Message:      gotgbot.Message{MessageId: msgID, Date: 1, Chat: chat},
			Data:         data,
			ChatInstance: "anon-proof-test",
		},
	}
	if err := e.dispatcher.ProcessUpdate(e.bot, update, nil); err != nil {
		e.t.Fatalf("ProcessUpdate(%d) error = %v", id, err)
	}
}

// seedAnonCommand stores text as an anonymous admin's command in chat (message msgID) for
// the proof tap and returns the proof button's callback data.
func (e *lockdownEnv) seedAnonCommand(chat gotgbot.Chat, msgID int64, text string) string {
	e.t.Helper()
	senderChat := chat
	cmd := &gotgbot.Message{
		MessageId:  msgID,
		Date:       1,
		Chat:       chat,
		From:       &gotgbot.User{Id: lockdownTestAnonBotID, IsBot: true, FirstName: "Group"},
		SenderChat: &senderChat,
		Text:       text,
	}
	if err := chat_status.StoreAnonAdminCommand(e.chat.Id, cmd); err != nil {
		e.t.Fatalf("StoreAnonAdminCommand() error = %v", err)
	}
	return encodeCallbackData("anon_admin", map[string]string{
		"c": fmt.Sprint(e.chat.Id),
		"m": fmt.Sprint(msgID),
	})
}

func TestAnonAdminReentryKickAndPurge(t *testing.T) {
	t.Run("kick", func(t *testing.T) {
		env, creator, target := newAnonBanEnv(t)
		env.sendAnonymous(fmt.Sprintf("/kick %d spam", target.Id))
		env.proofButton()
		if calls := env.calls("unbanChatMember"); len(calls) != 0 {
			t.Fatalf("unbanChatMember calls = %d before the proof, want 0", len(calls))
		}
		env.tapProof(creator)
		calls := env.calls("unbanChatMember")
		if len(calls) != 1 || staffParamInt(calls[0].Params, "user_id") != target.Id {
			t.Fatalf("unbanChatMember calls = %+v after the proof, want the one kick of %d", calls, target.Id)
		}
	})

	t.Run("dkick", func(t *testing.T) {
		env, creator, target := newAnonBanEnv(t)
		reply := &gotgbot.Message{MessageId: 33, Date: 1, Chat: env.chat, From: &target, Text: "spam"}
		env.sendAnonymousMessage("/dkick", reply)
		_, proofID, _ := env.proofButton()
		env.tapProof(creator)
		calls := env.calls("unbanChatMember")
		if len(calls) != 1 || staffParamInt(calls[0].Params, "user_id") != target.Id {
			t.Fatalf("unbanChatMember calls = %+v after the proof, want the one kick of %d", calls, target.Id)
		}
		deleted := map[int64]bool{}
		for _, call := range env.calls("deleteMessage") {
			deleted[staffParamInt(call.Params, "message_id")] = true
		}
		if !deleted[proofID] || !deleted[reply.MessageId] {
			t.Fatalf("deleted messages = %v, want the proof %d and the replied message %d", deleted, proofID, reply.MessageId)
		}
	})

	t.Run("purgefrom", func(t *testing.T) {
		env, creator, _ := newAnonBanEnv(t)
		LoadPurges(env.dispatcher)
		delMsgs.Delete(env.chat.Id)
		t.Cleanup(func() { delMsgs.Delete(env.chat.Id) })
		reply := &gotgbot.Message{MessageId: 34, Date: 1, Chat: env.chat, Text: "first"}
		env.sendAnonymousMessage("/purgefrom", reply)
		env.proofButton()
		if _, ok := delMsgs.Load(env.chat.Id); ok {
			t.Fatal("purgefrom marked a message before the proof")
		}
		env.tapProof(creator)
		stored, ok := delMsgs.Load(env.chat.Id)
		entry, isEntry := stored.(delMsgEntry)
		if !ok || !isEntry || entry.msgID != reply.MessageId || entry.userID != creator.Id {
			t.Fatalf("purge marker = %#v, want message %d marked by the tapper %d", stored, reply.MessageId, creator.Id)
		}
	})

	t.Run("purgeto", func(t *testing.T) {
		env, creator, _ := newAnonBanEnv(t)
		LoadPurges(env.dispatcher)
		delMsgs.Store(env.chat.Id, delMsgEntry{userID: creator.Id, msgID: 30})
		t.Cleanup(func() { delMsgs.Delete(env.chat.Id) })
		reply := &gotgbot.Message{MessageId: 32, Date: 1, Chat: env.chat, Text: "last"}
		env.sendAnonymousMessage("/purgeto", reply)
		env.proofButton()
		if _, ok := delMsgs.Load(env.chat.Id); !ok {
			t.Fatal("purgeto ran before the proof")
		}
		env.tapProof(creator)
		if _, ok := delMsgs.Load(env.chat.Id); ok {
			t.Fatal("the purge marker is still set after the proof: purgeto did not run")
		}
	})
}

// wantUnsupportedAlert fails unless the tap was answered exactly once, with the
// unsupported-command alert, after answers had been given before it.
func (e *lockdownEnv) wantUnsupportedAlert(answersBefore int) {
	e.t.Helper()
	if got := e.answerCount() - answersBefore; got != 1 {
		e.t.Fatalf("answerCallbackQuery calls for the tap = %d, want 1", got)
	}
	text, alert := e.lastAnswer()
	if !alert || !strings.Contains(text, staffMarker("bot_updates_anon_unsupported_command")) {
		e.t.Fatalf("answer = %q (alert %v), want the unsupported-command alert", text, alert)
	}
}

func TestAnonAdminTapUnsupportedCommand(t *testing.T) {
	t.Run("a prompting command with no re-entry", func(t *testing.T) {
		env, creator := newAnonLockdownEnv(t, false)
		LoadFilters(env.dispatcher)
		env.sendAnonymous("/filter hello world")
		_, proofID, _ := env.proofButton()
		answers, sent := env.answerCount(), len(env.replies())

		env.tapProof(creator)

		env.wantUnsupportedAlert(answers)
		deletes := env.calls("deleteMessage")
		if len(deletes) != 1 || staffParamInt(deletes[0].Params, "message_id") != proofID {
			t.Fatalf("deleteMessage calls = %+v, want the proof message %d deleted", deletes, proofID)
		}
		if got := len(env.replies()); got != sent {
			t.Fatalf("messages sent after the tap = %d, want none", got-sent)
		}
	})

	t.Run("a cached name nobody registered", func(t *testing.T) {
		env, creator := newAnonLockdownEnv(t, false)
		data := env.seedAnonCommand(env.chat, 77, "/nosuchanoncmd now")
		answers, sent := env.answerCount(), len(env.replies())

		env.tapProofData(creator, env.chat, 78, data)

		env.wantUnsupportedAlert(answers)
		if got := len(env.replies()); got != sent {
			t.Fatalf("messages sent after the tap = %d, want none", got-sent)
		}
	})
}

func TestAnonAdminTapCommandNameForms(t *testing.T) {
	for _, tc := range []struct {
		name string
		text func(target int64) string
	}{
		{"upper case", func(target int64) string { return fmt.Sprintf("/Ban %d spam", target) }},
		{"newline after the name", func(target int64) string { return fmt.Sprintf("/ban\n%d spam", target) }},
		{"addressed to the bot", func(target int64) string { return fmt.Sprintf("/ban@FukuTestBot %d spam", target) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env, creator, target := newAnonBanEnv(t)
			env.sendAnonymous(tc.text(target.Id))
			env.proofButton()
			env.tapProof(creator)
			bans := env.calls("banChatMember")
			if len(bans) != 1 || staffParamInt(bans[0].Params, "user_id") != target.Id {
				t.Fatalf("banChatMember calls = %+v after the proof, want the one ban of %d", bans, target.Id)
			}
		})
	}
}

func TestAnonAdminTapAnswersOnce(t *testing.T) {
	for _, tc := range []struct {
		name string
		tap  func(env *lockdownEnv, creator gotgbot.User)
	}{
		{"success", func(env *lockdownEnv, creator gotgbot.User) {
			data := env.seedAnonCommand(env.chat, 61, "/lockdownstatus")
			env.tapProofData(creator, env.chat, 62, data)
		}},
		{"expired", func(env *lockdownEnv, creator gotgbot.User) {
			data := encodeCallbackData("anon_admin", map[string]string{"c": fmt.Sprint(env.chat.Id), "m": "63"})
			env.tapProofData(creator, env.chat, 64, data)
		}},
		{"unsupported", func(env *lockdownEnv, creator gotgbot.User) {
			data := env.seedAnonCommand(env.chat, 65, "/nosuchanoncmd")
			env.tapProofData(creator, env.chat, 66, data)
		}},
		{"non-admin presser", func(env *lockdownEnv, _ gotgbot.User) {
			data := env.seedAnonCommand(env.chat, 67, "/lockdownstatus")
			member := env.newJoiner("Member")
			env.fake.setMember(env.chat.Id, member.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})
			env.tapProofData(member, env.chat, 68, data)
		}},
		{"invalid data", func(env *lockdownEnv, creator gotgbot.User) {
			env.tapProofData(creator, env.chat, 69, "anon_admin|v1|broken")
		}},
		{"chat mismatch", func(env *lockdownEnv, creator gotgbot.User) {
			other := gotgbot.Chat{Id: uniqueModuleChatID(), Type: "supergroup", Title: "Other"}
			data := env.seedAnonCommand(other, 70, "/lockdownstatus")
			env.tapProofData(creator, env.chat, 71, data)
		}},
		{"delete failure", func(env *lockdownEnv, creator gotgbot.User) {
			data := env.seedAnonCommand(env.chat, 72, "/lockdownstatus")
			env.fake.script("deleteMessage", env.chat.Id, staffFakeError(400, "Bad Request: message can't be deleted"))
			env.tapProofData(creator, env.chat, 73, data)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env, creator := newAnonLockdownEnv(t, false)
			tc.tap(env, creator)
			if got := env.answerCount(); got != 1 {
				t.Fatalf("answerCallbackQuery calls = %d, want exactly 1", got)
			}
		})
	}
}

func TestAnonAdminProofSingleReply(t *testing.T) {
	t.Run("the prompt is the only reply", func(t *testing.T) {
		env, _, target := newAnonBanEnv(t)
		env.sendAnonymous(fmt.Sprintf("/ban %d spam", target.Id))
		sent := env.replies()
		if len(sent) != 1 {
			t.Fatalf("messages sent for the anonymous /ban = %d, want the one proof prompt", len(sent))
		}
		env.wantProofPrompt()
		env.wantNoBotReplyContains(staffMarker("chat_status_restrict_cmd_error"))
	})

	t.Run("a prompt that could not be sent leaves the refusal", func(t *testing.T) {
		env, _, target := newAnonBanEnv(t)
		env.fake.script("sendMessage", env.chat.Id, staffFakeError(400, "Bad Request: not enough rights to send text messages"))
		env.sendAnonymous(fmt.Sprintf("/ban %d spam", target.Id))
		if calls := env.calls("sendMessage"); len(calls) != 2 {
			t.Fatalf("sendMessage calls = %d, want the failed prompt and the refusal", len(calls))
		}
		env.wantReplyHas(staffMarker("chat_status_restrict_cmd_error"))
	})
}
