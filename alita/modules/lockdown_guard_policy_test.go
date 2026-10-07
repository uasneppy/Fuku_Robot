//go:build testtools

package modules

import (
	"errors"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/divkix/Alita_Robot/alita/db/lockdown"
	"github.com/divkix/Alita_Robot/alita/db/models"
)

func TestDecideLockdownJoin(t *testing.T) {
	const bot, member, admin int64 = 999, 5001, 6001
	base := lockdownJoinInput{Path: models.JoinPathMember, PerformerID: admin, MemberID: member, BotID: bot}
	with := func(edit func(in *lockdownJoinInput)) lockdownJoinInput {
		in := base
		edit(&in)
		return in
	}

	tests := []struct {
		name       string
		in         lockdownJoinInput
		want       lockdownJoinVerdict
		wantLookup bool
	}{
		{"the bot itself joining is none of the lockdown's business",
			with(func(in *lockdownJoinInput) { in.MemberID = bot }), lockdownJoinIgnore, false},
		{"a join request is declined",
			with(func(in *lockdownJoinInput) { in.Path = models.JoinPathRequest }), lockdownJoinDecline, false},
		{"a bot account is banned even when an admin added it",
			with(func(in *lockdownJoinInput) {
				in.MemberIsBot = true
				in.PerformerStatus = gotgbot.ChatMemberStatusCreator
			}),
			lockdownJoinBan, false},
		{"a self-join is banned whatever the performer status says",
			with(func(in *lockdownJoinInput) {
				in.PerformerID = member
				in.PerformerStatus = gotgbot.ChatMemberStatusAdministrator
			}),
			lockdownJoinBan, false},
		{"a join the bot performed is banned",
			with(func(in *lockdownJoinInput) {
				in.PerformerID = bot
				in.PerformerStatus = gotgbot.ChatMemberStatusAdministrator
			}),
			lockdownJoinBan, false},
		{"an anonymous admin adding someone is exempt",
			with(func(in *lockdownJoinInput) { in.PerformerAnonymousAdmin = true }), lockdownJoinExempt, false},
		{"a live administrator adding someone is exempt",
			with(func(in *lockdownJoinInput) { in.PerformerStatus = gotgbot.ChatMemberStatusAdministrator }),
			lockdownJoinExempt, true},
		{"a live creator adding someone is exempt",
			with(func(in *lockdownJoinInput) { in.PerformerStatus = gotgbot.ChatMemberStatusCreator }),
			lockdownJoinExempt, true},
		{"a plain member adding someone is banned",
			with(func(in *lockdownJoinInput) { in.PerformerStatus = gotgbot.ChatMemberStatusMember }),
			lockdownJoinBan, true},
		{"a performer who left is banned",
			with(func(in *lockdownJoinInput) { in.PerformerStatus = gotgbot.ChatMemberStatusLeft }),
			lockdownJoinBan, true},
		{"a failed lookup (empty status) is banned",
			with(func(in *lockdownJoinInput) { in.PerformerStatus = "" }), lockdownJoinBan, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := decideLockdownJoin(tt.in); got != tt.want {
				t.Errorf("decideLockdownJoin = %v, want %v", got, tt.want)
			}
			if got := lockdownNeedsPerformerLookup(tt.in); got != tt.wantLookup {
				t.Errorf("lockdownNeedsPerformerLookup = %v, want %v", got, tt.wantLookup)
			}
		})
	}
}

func TestLockdownGuardLetsInAUserAnAdminAdded(t *testing.T) {
	env := newLockdownEnv(t)
	env.loadJoinModules()
	env.enableJoinWelcome(false)
	env.lockAsAdmin("raid")
	messagesBefore, photosBefore, _ := env.chatSends()

	guest := env.newJoiner("Guest")
	env.join(guest, env.admin, "")

	row := env.joinerRow(guest.Id)
	if row.State != models.JoinerStateExempt || row.PerformerID != env.admin.Id {
		t.Errorf("row = %+v, want exempt, performed by the admin", row)
	}
	messagesAfter, photosAfter, _ := env.chatSends()
	if messagesAfter+photosAfter == messagesBefore+photosBefore {
		t.Error("no welcome followed: an exempt joiner is let through to the greetings")
	}
	env.cycle()
	if bans := env.bansOf(guest.Id); len(bans) != 0 {
		t.Errorf("banChatMember calls for an exempt joiner = %d, want none", len(bans))
	}
}

func TestLockdownGuardBansWhenAMemberAddsSomeone(t *testing.T) {
	env := newLockdownEnv(t)
	env.lockAsAdmin("raid")

	member := env.newJoiner("Member")
	env.fake.setMember(env.chat.Id, member.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})
	guest := env.newJoiner("Guest")
	env.join(guest, member, "")

	if row := env.joinerRow(guest.Id); row.State != models.JoinerStatePending {
		t.Errorf("state = %q, want pending: a plain member's invite does not exempt", row.State)
	}
}

func TestLockdownGuardFailsOpenWhenTheRecordCannotBeWritten(t *testing.T) {
	env := newLockdownEnv(t)
	env.loadJoinModules()
	env.enableJoinWelcome(false)
	env.lockAsAdmin("raid")
	messagesBefore, photosBefore, _ := env.chatSends()

	previous := lockdownRecordJoin
	lockdownRecordJoin = func(lockdown.JoinRecord) (*models.LockdownJoiner, bool, error) {
		return nil, false, errors.New("database is down")
	}
	t.Cleanup(func() { lockdownRecordJoin = previous })

	raider := env.newJoiner("Raider")
	env.join(raider, raider, "https://t.me/+abc")
	env.cycle()

	if rows := env.joinerRows(raider.Id); len(rows) != 0 {
		t.Errorf("joiner rows = %d, want none", len(rows))
	}
	if bans := env.bansOf(raider.Id); len(bans) != 0 {
		t.Errorf("banChatMember calls = %d, want none: a ban without a record would never be lifted", len(bans))
	}
	messagesAfter, photosAfter, _ := env.chatSends()
	if messagesAfter+photosAfter == messagesBefore+photosBefore {
		t.Error("the joiner was not let through to the welcome: a failed record write lets them in, muted by the locked defaults")
	}
}

func TestLockdownGuardWithdrawsAJoinerWhenTheLiftStarted(t *testing.T) {
	env := newLockdownEnv(t)
	active := env.lockAsAdmin("raid")
	if won, err := lockdown.BeginLift(active.ID, env.admin.Id, "Admin", false); err != nil || !won {
		t.Fatalf("BeginLift = %v, %v", won, err)
	}

	raider := env.newJoiner("Raider")
	rec := lockdown.JoinRecord{
		LockdownID: active.ID, ChatID: env.chat.Id, UserID: raider.Id, FirstName: "Raider",
		Path: models.JoinPathMember, PerformerID: raider.Id,
	}
	// The guard read the lockdown while it was still active; the lift began before its insert.
	if letIn := recordLockdownJoin(env.bot, active, rec, lockdownJoinBan); !letIn {
		t.Error("recordLockdownJoin = handled, want let in: the lockdown is no longer active")
	}
	if rows := env.joinerRows(raider.Id); len(rows) != 0 {
		t.Errorf("joiner rows = %d, want none: the guard withdraws its own pending row", len(rows))
	}
}
