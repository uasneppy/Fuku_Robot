//go:build testtools

package modules

import (
	"fmt"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
)

// TestAnonymousAdminBanAfterProof drives an anonymous /ban through the real
// dispatcher: the proof prompt, the tap, and the re-entered command. After the tap the
// command must find its chat, run every check for the tapper and act in that chat.
func TestAnonymousAdminBanAfterProof(t *testing.T) {
	setup := func(t *testing.T) (*lockdownEnv, gotgbot.User, gotgbot.User) {
		t.Helper()
		env, creator := newAnonLockdownEnv(t, false)
		LoadBans(env.dispatcher)
		target := env.newJoiner("Target")
		env.fake.setMember(env.chat.Id, target.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})
		return env, creator, target
	}

	t.Run("admin bans after the proof", func(t *testing.T) {
		env, creator, target := setup(t)

		env.sendAnonymous(fmt.Sprintf("/ban %d spam", target.Id))
		_, proofMsgID, _ := env.proofButton()
		if bans := env.calls("banChatMember"); len(bans) != 0 {
			t.Fatalf("banChatMember calls = %d before the proof, want 0", len(bans))
		}
		before := len(env.replies())

		env.tapProof(creator)

		bans := env.calls("banChatMember")
		if len(bans) != 1 {
			t.Fatalf("banChatMember calls = %d after the proof, want 1", len(bans))
		}
		if got := staffParamInt(bans[0].Params, "user_id"); got != target.Id {
			t.Errorf("banned user = %d, want the target %d", got, target.Id)
		}
		if got := staffParamInt(bans[0].Params, "chat_id"); got != env.chat.Id {
			t.Errorf("ban chat = %d, want %d", got, env.chat.Id)
		}
		if m := env.fake.member(env.chat.Id, target.Id); m == nil || m.Status != gotgbot.ChatMemberStatusKicked {
			t.Errorf("target record = %+v, want kicked", m)
		}
		replies := env.replies()
		if len(replies) <= before {
			t.Fatalf("no reply was posted after the proof (%d replies before, %d after)", before, len(replies))
		}
		found := false
		for _, r := range replies[before:] {
			if strings.Contains(fmt.Sprint(r.Params["text"]), staffMarker("bans_ban_normal_ban")) {
				found = true
			}
		}
		if !found {
			t.Errorf("no new reply contains %q", staffMarker("bans_ban_normal_ban"))
		}
		deletes := env.calls("deleteMessage")
		if len(deletes) != 1 {
			t.Fatalf("deleteMessage calls = %d, want the one proof message", len(deletes))
		}
		if got := staffParamInt(deletes[0].Params, "message_id"); got != proofMsgID {
			t.Errorf("deleted message = %d, want the proof message %d", got, proofMsgID)
		}
	})

	t.Run("a failed check is answered after the proof", func(t *testing.T) {
		env, creator, _ := setup(t)
		env.fake.setBotRole(env.chat.Id, staffRoleAdminNoRestrict)
		target := env.newJoiner("Target2")
		env.fake.setMember(env.chat.Id, target.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})

		env.sendAnonymous(fmt.Sprintf("/ban %d spam", target.Id))
		env.proofButton()
		before := len(env.replies())

		env.tapProof(creator)

		replies := env.replies()
		if len(replies) != before+1 {
			t.Fatalf("replies after the tap = %d new, want exactly 1", len(replies)-before)
		}
		last := replies[len(replies)-1]
		if got := fmt.Sprint(last.Params["text"]); !strings.Contains(got, staffMarker("chat_status_bot_restrict_group_error")) {
			t.Errorf("refusal %q does not contain %q", got, staffMarker("chat_status_bot_restrict_group_error"))
		}
		if last.ChatID != env.chat.Id {
			t.Errorf("refusal went to chat %d, want %d", last.ChatID, env.chat.Id)
		}
		if bans := env.calls("banChatMember"); len(bans) != 0 {
			t.Errorf("banChatMember calls = %d, want 0 when a check fails", len(bans))
		}
	})

	t.Run("a member who taps bans nobody", func(t *testing.T) {
		env, _, target := setup(t)
		member := env.newJoiner("Member")
		env.fake.setMember(env.chat.Id, member.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})

		env.sendAnonymous(fmt.Sprintf("/ban %d spam", target.Id))
		env.proofButton()

		env.tapProof(member)

		if bans := env.calls("banChatMember"); len(bans) != 0 {
			t.Errorf("banChatMember calls = %d, want 0 for a tapper who is not an admin", len(bans))
		}
		text, _ := env.lastAnswer()
		if !strings.Contains(text, staffMarker("bot_updates_need_admin")) {
			t.Errorf("answer %q does not contain %q", text, staffMarker("bot_updates_need_admin"))
		}
	})
}
