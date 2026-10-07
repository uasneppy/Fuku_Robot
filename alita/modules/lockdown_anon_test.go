//go:build testtools

package modules

import (
	"fmt"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/divkix/Alita_Robot/alita/db/admin"
	"github.com/divkix/Alita_Robot/alita/db/lockdown"
	"github.com/divkix/Alita_Robot/alita/db/models"
)

// lockdownTestAnonBotID is the placeholder user Telegram puts on a message an admin
// posts as the group itself.
const lockdownTestAnonBotID int64 = 1087968824

// sendAnonymous runs a message posted anonymously as the group itself: From is the
// placeholder bot and SenderChat is the group.
func (e *lockdownEnv) sendAnonymous(text string) {
	e.t.Helper()
	id := e.updateID()
	from := gotgbot.User{Id: lockdownTestAnonBotID, IsBot: true, FirstName: "Group", Username: "GroupAnonymousBot"}
	senderChat := e.chat
	update := &gotgbot.Update{
		UpdateId: id,
		Message: &gotgbot.Message{
			MessageId:  id,
			Date:       1,
			Chat:       e.chat,
			From:       &from,
			SenderChat: &senderChat,
			Text:       text,
		},
	}
	if err := e.dispatcher.ProcessUpdate(e.bot, update, nil); err != nil {
		e.t.Fatalf("ProcessUpdate(%d) error = %v", id, err)
	}
}

// proofButton returns the callback data and message ID of the newest anonymous-admin
// proof button the bot sent to the chat, and how many buttons that keyboard has.
func (e *lockdownEnv) proofButton() (data string, msgID int64, buttons int) {
	e.t.Helper()
	sent := e.replies()
	for i := len(sent) - 1; i >= 0; i-- {
		keyboard := staffKeyboardOf(sent[i].Params["reply_markup"])
		for _, row := range keyboard {
			for _, button := range row {
				if _, ok := decodeCallbackData(button.CallbackData, "anon_admin"); ok {
					count := 0
					for _, r := range keyboard {
						count += len(r)
					}
					return button.CallbackData, sent[i].MessageID, count
				}
			}
		}
	}
	e.t.Fatal("the bot sent no anonymous-admin proof button to the chat")
	return "", 0, 0
}

// tapProof presses the newest proof button as from.
func (e *lockdownEnv) tapProof(from gotgbot.User) {
	e.t.Helper()
	data, msgID, _ := e.proofButton()
	id := e.updateID()
	update := &gotgbot.Update{
		UpdateId: id,
		CallbackQuery: &gotgbot.CallbackQuery{
			Id:           fmt.Sprintf("anon-%d", id),
			From:         from,
			Message:      gotgbot.Message{MessageId: msgID, Date: 1, Chat: e.chat},
			Data:         data,
			ChatInstance: "lockdown-anon-test",
		},
	}
	if err := e.dispatcher.ProcessUpdate(e.bot, update, nil); err != nil {
		e.t.Fatalf("ProcessUpdate(%d) error = %v", id, err)
	}
}

// newAnonLockdownEnv is a lockdown environment with the anonymous-admin proof
// callback loaded, the chat's AnonAdmin mode set to anonMode, and a creator who
// is a live creator in the fake.
func newAnonLockdownEnv(t *testing.T, anonMode bool) (*lockdownEnv, gotgbot.User) {
	t.Helper()
	env := newLockdownEnv(t)
	LoadBotUpdates(env.dispatcher)

	creator := gotgbot.User{Id: uniqueLinkOwnerID() + 23, FirstName: "Tap<per"}
	env.fake.setCreator(env.chat.Id, creator.Id)
	env.fake.setMember(env.chat.Id, creator.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusCreator})

	if anonMode {
		if err := admin.SetAnonAdminMode(env.chat.Id, true); err != nil {
			t.Fatalf("SetAnonAdminMode(true) error = %v", err)
		}
		t.Cleanup(func() {
			if err := admin.SetAnonAdminMode(env.chat.Id, false); err != nil {
				t.Errorf("cleanup SetAnonAdminMode(false) error = %v", err)
			}
		})
	}
	return env, creator
}

// wantProofPrompt fails unless the last reply is the "prove you're admin" prompt
// with exactly one button, and nothing was looked up or written for the anonymous
// sender.
func (e *lockdownEnv) wantProofPrompt() {
	e.t.Helper()
	e.wantReplyHas(staffMarker("chat_status_anon_confirm"))
	if _, _, buttons := e.proofButton(); buttons != 1 {
		e.t.Errorf("proof prompt keyboard has %d buttons, want 1", buttons)
	}
	for _, call := range e.calls("getChatMember") {
		if staffParamInt(call.Params, "user_id") == lockdownTestAnonBotID {
			e.t.Error("the placeholder anonymous user was looked up live: the proof comes first, with no lookup")
		}
	}
}

func TestLockdownAnonymousAdminProof(t *testing.T) {
	// Each mode is a subtest: with the chat's AnonAdmin setting on, every other
	// command skips the proof; a lockdown command must not.
	for _, mode := range []struct {
		name     string
		anonMode bool
	}{
		{"AnonAdmin on", true},
		{"AnonAdmin off", false},
	} {
		t.Run(mode.name, func(t *testing.T) {
			env, creator := newAnonLockdownEnv(t, mode.anonMode)

			// /lockdown: the prompt comes first and nothing is written.
			env.sendAnonymous("/lockdown raid")
			env.wantProofPrompt()
			env.wantNothingRecorded()
			if calls := env.calls("setChatPermissions"); len(calls) != 0 {
				t.Fatalf("setChatPermissions calls = %d before the proof, want 0", len(calls))
			}

			// The person who taps is checked live and is the one recorded.
			env.tapProof(creator)
			row, err := lockdown.GetActiveFresh(env.chat.Id)
			if err != nil || row == nil {
				t.Fatalf("GetActiveFresh = %v, %v, want a lockdown after the proof", row, err)
			}
			if row.StartedBy != creator.Id || row.StartedByName != creator.FirstName {
				t.Errorf("started by %d %q, want the tapper %d %q", row.StartedBy, row.StartedByName, creator.Id, creator.FirstName)
			}
			if row.Reason != "raid" {
				t.Errorf("Reason = %q, want the reason typed with the anonymous command", row.Reason)
			}
			env.wantReplyHas(staffMarker("lockdown_started"), "Tap&lt;per")
			if calls := env.calls("setChatPermissions"); len(calls) != 1 {
				t.Errorf("setChatPermissions calls = %d, want the one lock", len(calls))
			}

			// /lockdownstatus: the prompt, then the status for the tapper.
			env.sendAnonymous("/lockdownstatus")
			env.wantProofPrompt()
			env.tapProof(creator)
			env.wantReplyHas(staffMarker("lockdown_status_active"), "Tap&lt;per", "raid")

			// /unlockdown: the prompt lifts nothing; the tap does, and records the tapper.
			env.sendAnonymous("/unlockdown")
			env.wantProofPrompt()
			if active, err := lockdown.GetActiveFresh(env.chat.Id); err != nil || active == nil {
				t.Fatalf("GetActiveFresh = %v, %v, want the lockdown still active before the proof", active, err)
			}
			if calls := env.calls("setChatPermissions"); len(calls) != 1 {
				t.Fatalf("setChatPermissions calls = %d before the proof, want only the lock", len(calls))
			}
			env.tapProof(creator)
			lifted := env.freshRow(row.ID)
			if lifted.State != models.LockdownStateLifted || lifted.LiftedBy == nil || *lifted.LiftedBy != creator.Id {
				t.Errorf("row = %+v, want lifted by the tapper %d", lifted, creator.Id)
			}
			if lifted.LiftedByName != creator.FirstName {
				t.Errorf("LiftedByName = %q, want %q", lifted.LiftedByName, creator.FirstName)
			}
			env.wantReplyHas(staffMarker("lockdown_lifted"), "Tap&lt;per")
			if got := env.fake.chatPermsRaw(env.chat.Id); got != lockdownTestPrePermissions {
				t.Errorf("permissions after the lift = %q, want the snapshot", got)
			}
		})
	}

	t.Run("tapper not live admin", func(t *testing.T) {
		env, _ := newAnonLockdownEnv(t, true)
		// The admin list shows the tapper as creator, but the live record says member.
		stale := gotgbot.User{Id: uniqueLinkOwnerID() + 31, FirstName: "Stale"}
		env.fake.setCreator(env.chat.Id, stale.Id)
		env.fake.setMember(env.chat.Id, stale.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})

		env.sendAnonymous("/lockdown raid")
		env.wantProofPrompt()
		env.tapProof(stale)

		env.wantReplyHas(staffMarker("chat_status_user_admin_cmd_error"))
		env.wantNothingRecorded()
		if calls := env.calls("setChatPermissions"); len(calls) != 0 {
			t.Errorf("setChatPermissions calls = %d, want 0 for a tapper who is not a live admin", len(calls))
		}
	})

	t.Run("tapper without the restrict right cannot lock but can see the status", func(t *testing.T) {
		env, creator := newAnonLockdownEnv(t, true)
		helper := gotgbot.User{Id: uniqueLinkOwnerID() + 37, FirstName: "Helper"}
		env.fake.setMember(env.chat.Id, helper.Id, staffFakeMember{
			Status:             gotgbot.ChatMemberStatusAdministrator,
			CanRestrictMembers: false,
		})
		env.fake.setCreator(env.chat.Id, helper.Id)

		env.sendAnonymous("/lockdown")
		env.wantProofPrompt()
		env.tapProof(helper)
		env.wantReplyHas(staffMarker("chat_status_restrict_cmd_error"))
		env.wantNothingRecorded()

		env.send(creator, "/lockdown raid")
		env.sendAnonymous("/lockdownstatus")
		env.wantProofPrompt()
		env.tapProof(helper)
		env.wantReplyHas(staffMarker("lockdown_status_active"))
		if strings.Contains(env.lastReply(), staffMarker("chat_status_restrict_cmd_error")) {
			t.Error("the status was refused to an administrator without the restrict right")
		}
	})
}
