//go:build testtools

package modules

import (
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

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
