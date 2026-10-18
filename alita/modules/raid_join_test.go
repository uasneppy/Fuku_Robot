//go:build testtools

package modules

import (
	"fmt"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/divkix/Alita_Robot/alita/db/models"
)

// newGuests returns n fresh accounts.
func (e *lockdownEnv) newGuests(n int) []gotgbot.User {
	e.t.Helper()
	guests := make([]gotgbot.User, 0, n)
	for i := 0; i < n; i++ {
		guests = append(guests, e.newJoiner(fmt.Sprintf("Guest%d", i)))
	}
	return guests
}

func TestRaidJoinDoubleDelivery(t *testing.T) {
	env := newLockdownEnv(t)
	linkLockdownStaffGroup(env)
	raidCleanup(t, env.chat.Id)

	// Telegram delivers one join as a chat_member update and as a service message.
	for i := 0; i < 14; i++ {
		user := env.newJoiner(fmt.Sprintf("Twice%d", i))
		env.join(user, user, "")
		env.serviceJoin(user, nil, user)
	}

	if row := env.activeLockdown(); row != nil {
		t.Fatalf("a lockdown (kind %q, count %d) exists after 14 joins delivered twice, want none: a join counts once",
			row.TriggerKind, row.TriggerCount)
	}
	if got := env.raidJoinedCount(); got != 14 {
		t.Errorf("the joined set holds %d accounts, want 14: both deliveries of a join are one account", got)
	}

	// The 15th account arrives only as a service message.
	fifteenth := env.newJoiner("OnlyMessage")
	env.serviceJoin(fifteenth, nil, fifteenth)

	row := env.activeLockdown()
	if row == nil {
		t.Fatal("no active lockdown after the 15th account joined by service message")
	}
	if row.TriggerKind != models.LockdownTriggerJoinSurge || row.TriggerCount != 15 {
		t.Errorf("trigger = %q with count %d, want join_surge with count 15", row.TriggerKind, row.TriggerCount)
	}
}

func TestRaidJoinServiceMessage(t *testing.T) {
	t.Run("member invites", func(t *testing.T) {
		env := newLockdownEnv(t)
		linkLockdownStaffGroup(env)
		raidCleanup(t, env.chat.Id)

		inviter := env.newJoiner("Inviter")
		env.fake.setMember(env.chat.Id, inviter.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})
		guests := env.newGuests(15)

		env.serviceJoin(inviter, nil, guests...)

		row := env.activeLockdown()
		if row == nil {
			t.Fatal("no active lockdown after a member added 15 accounts in one service message")
		}
		if row.TriggerKind != models.LockdownTriggerJoinSurge || row.TriggerCount != 15 || row.TriggerWindow != 60 {
			t.Errorf("trigger = %q with %d joins in %d s, want join_surge with 15 in 60 s",
				row.TriggerKind, row.TriggerCount, row.TriggerWindow)
		}
		if got := env.performerLookups(inviter.Id); got != 1 {
			t.Errorf("getChatMember calls for the inviter = %d, want exactly 1 shared by the counter and the guard", got)
		}
		for _, guest := range guests {
			if state := env.joinerRow(guest.Id).State; state != models.JoinerStatePending {
				t.Errorf("guest %d is %q, want pending: the guard handled them under the lockdown it saw", guest.Id, state)
			}
		}
	})

	t.Run("admin adds during a lockdown", func(t *testing.T) {
		env := newLockdownEnv(t)
		raidCleanup(t, env.chat.Id)
		env.lockAsAdmin("raid")
		before := env.performerLookups(env.admin.Id)

		guest := env.newJoiner("Guest")
		env.join(guest, env.admin, "")

		if got := env.performerLookups(env.admin.Id) - before; got != 1 {
			t.Errorf("getChatMember calls for the admin = %d, want exactly 1 shared by the counter and the guard", got)
		}
		if state := env.joinerRow(guest.Id).State; state != models.JoinerStateExempt {
			t.Errorf("guest is %q, want exempt: a live administrator added them", state)
		}
	})

	t.Run("admin adds by service message during a lockdown", func(t *testing.T) {
		env := newLockdownEnv(t)
		raidCleanup(t, env.chat.Id)
		env.lockAsAdmin("raid")
		before := env.performerLookups(env.admin.Id)

		guests := env.newGuests(3)
		env.serviceJoin(env.admin, nil, guests...)

		if got := env.performerLookups(env.admin.Id) - before; got != 1 {
			t.Errorf("getChatMember calls for the admin = %d, want exactly 1 for the whole message", got)
		}
		for _, guest := range guests {
			if state := env.joinerRow(guest.Id).State; state != models.JoinerStateExempt {
				t.Errorf("guest %d is %q, want exempt: a live administrator added them", guest.Id, state)
			}
		}
	})
}
