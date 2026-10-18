//go:build testtools

package modules

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/redis/go-redis/v9"

	"github.com/divkix/Alita_Robot/alita/db/approvals"
	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/utils/cache"
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

// raidCounted reports whether the account is in the join counter of the env's chat.
func (e *lockdownEnv) raidCounted(userID int64) bool {
	e.t.Helper()
	client := cache.GetRedisClient()
	if client == nil {
		e.t.Fatal("no Redis client in the test")
	}
	_, err := client.ZScore(context.Background(), raidKey(e.chat.Id, raidPartJoined), fmt.Sprint(userID)).Result()
	if errors.Is(err, redis.Nil) {
		return false
	}
	if err != nil {
		e.t.Fatalf("read the joined set: %v", err)
	}
	return true
}

// raidExemption runs one case of the counting rule on a chat that already holds 14
// counted self-joins, so one more counted join reaches the threshold of 15. act makes
// the join under test and returns the account that joined. A join that must not count
// leaves the chat unlocked and the account out of the counter, and one more self-join
// then locks with exactly 15; a join that must count locks the chat at once.
func raidExemption(t *testing.T, counts bool, act func(env *lockdownEnv) gotgbot.User) {
	t.Helper()
	env := newLockdownEnv(t)
	linkLockdownStaffGroup(env)
	raidCleanup(t, env.chat.Id)
	env.selfJoins(14)
	if row := env.activeLockdown(); row != nil {
		t.Fatalf("a lockdown exists after 14 joins, want none")
	}

	user := act(env)

	if counts {
		row := env.activeLockdown()
		if row == nil {
			t.Fatal("no active lockdown: the join was not counted, want it counted")
		}
		if row.TriggerKind != models.LockdownTriggerJoinSurge || row.TriggerCount != 15 {
			t.Errorf("trigger = %q with count %d, want join_surge with count 15", row.TriggerKind, row.TriggerCount)
		}
		if !env.raidCounted(user.Id) {
			t.Errorf("account %d is not in the joined set, want it counted", user.Id)
		}
		return
	}

	if row := env.activeLockdown(); row != nil {
		t.Fatalf("a lockdown (kind %q, count %d) exists: the join was counted, want it ignored", row.TriggerKind, row.TriggerCount)
	}
	if env.raidCounted(user.Id) {
		t.Errorf("account %d is in the joined set, want it left out", user.Id)
	}
	if got := env.raidJoinedCount(); got != 14 {
		t.Errorf("the joined set holds %d accounts, want the 14 self-joins", got)
	}
	env.selfJoins(1)
	row := env.activeLockdown()
	if row == nil {
		t.Fatal("no active lockdown after one more counted self-join")
	}
	if row.TriggerCount != 15 {
		t.Errorf("TriggerCount = %d, want 15: the ignored join must not have been counted", row.TriggerCount)
	}
}

// raidAnonymousAdmin is the identity Telegram shows for an administrator acting anonymously.
func raidAnonymousAdmin() gotgbot.User {
	return gotgbot.User{Id: lockdownGroupAnonymousBot, IsBot: true, FirstName: "Group"}
}

// raidBotAccount returns a bot account with a fresh ID.
func (e *lockdownEnv) raidBotAccount() gotgbot.User {
	return gotgbot.User{Id: e.newJoiner("").Id, IsBot: true, FirstName: "Helper", Username: "helper_bot"}
}

func TestRaidJoinExemptions(t *testing.T) {
	t.Run("admin added", func(t *testing.T) {
		raidExemption(t, false, func(env *lockdownEnv) gotgbot.User {
			guest := env.newJoiner("Guest")
			env.join(guest, env.admin, "")
			return guest
		})
	})

	t.Run("admin added by service message", func(t *testing.T) {
		raidExemption(t, false, func(env *lockdownEnv) gotgbot.User {
			guest := env.newJoiner("Guest")
			env.serviceJoin(env.admin, nil, guest)
			return guest
		})
	})

	t.Run("anonymous admin added by service message", func(t *testing.T) {
		raidExemption(t, false, func(env *lockdownEnv) gotgbot.User {
			guest := env.newJoiner("Guest")
			chat := env.chat
			env.serviceJoin(raidAnonymousAdmin(), &chat, guest)
			return guest
		})
	})

	t.Run("anonymous admin added by chat member", func(t *testing.T) {
		raidExemption(t, false, func(env *lockdownEnv) gotgbot.User {
			guest := env.newJoiner("Guest")
			env.join(guest, raidAnonymousAdmin(), "")
			return guest
		})
	})

	t.Run("admin added a bot", func(t *testing.T) {
		raidExemption(t, false, func(env *lockdownEnv) gotgbot.User {
			helper := env.raidBotAccount()
			env.join(helper, env.admin, "")
			return helper
		})
	})

	t.Run("admin added a bot by service message", func(t *testing.T) {
		raidExemption(t, false, func(env *lockdownEnv) gotgbot.User {
			helper := env.raidBotAccount()
			env.serviceJoin(env.admin, nil, helper)
			return helper
		})
	})

	t.Run("approved user", func(t *testing.T) {
		raidExemption(t, false, func(env *lockdownEnv) gotgbot.User {
			guest := env.newJoiner("Guest")
			if err := approvals.AddApprovedUser(env.chat.Id, guest.Id, env.admin.Id, "trusted"); err != nil {
				env.t.Fatalf("approve the guest: %v", err)
			}
			env.t.Cleanup(func() { _ = approvals.RemoveAllApprovedUsers(env.chat.Id) })
			env.join(guest, guest, "")
			return guest
		})
	})

	t.Run("approved user by service message", func(t *testing.T) {
		raidExemption(t, false, func(env *lockdownEnv) gotgbot.User {
			guest := env.newJoiner("Guest")
			if err := approvals.AddApprovedUser(env.chat.Id, guest.Id, env.admin.Id, "trusted"); err != nil {
				env.t.Fatalf("approve the guest: %v", err)
			}
			env.t.Cleanup(func() { _ = approvals.RemoveAllApprovedUsers(env.chat.Id) })
			env.serviceJoin(guest, nil, guest)
			return guest
		})
	})

	t.Run("the bot itself", func(t *testing.T) {
		raidExemption(t, false, func(env *lockdownEnv) gotgbot.User {
			self := gotgbot.User{Id: staffTestBotID, IsBot: true, FirstName: "Alita"}
			env.join(self, env.admin, "")
			return self
		})
	})

	t.Run("join request", func(t *testing.T) {
		raidExemption(t, false, func(env *lockdownEnv) gotgbot.User {
			guest := env.newJoiner("Guest")
			env.joinRequest(guest, "")
			return guest
		})
	})

	t.Run("approved request by an admin", func(t *testing.T) {
		raidExemption(t, false, func(env *lockdownEnv) gotgbot.User {
			guest := env.newJoiner("Guest")
			env.memberUpdate(env.admin, gotgbot.ChatMemberLeft{User: guest}, gotgbot.ChatMemberMember{User: guest}, true)
			return guest
		})
	})

	t.Run("a member added a bot", func(t *testing.T) {
		raidExemption(t, true, func(env *lockdownEnv) gotgbot.User {
			plain := env.newJoiner("Plain")
			env.fake.setMember(env.chat.Id, plain.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})
			helper := env.raidBotAccount()
			env.join(helper, plain, "")
			return helper
		})
	})

	t.Run("approved request by the bot", func(t *testing.T) {
		raidExemption(t, true, func(env *lockdownEnv) gotgbot.User {
			guest := env.newJoiner("Guest")
			self := gotgbot.User{Id: staffTestBotID, IsBot: true, FirstName: "Alita"}
			env.memberUpdate(self, gotgbot.ChatMemberLeft{User: guest}, gotgbot.ChatMemberMember{User: guest}, true)
			return guest
		})
	})

	t.Run("performer lookup failed", func(t *testing.T) {
		raidExemption(t, true, func(env *lockdownEnv) gotgbot.User {
			guest := env.newJoiner("Guest")
			env.fake.script("getChatMember", env.chat.Id, errors.New("telegram is down"))
			env.join(guest, env.admin, "")
			return guest
		})
	})
}

func TestRaidJoinBelowThreshold(t *testing.T) {
	t.Run("fourteen do not lock", func(t *testing.T) {
		env := newLockdownEnv(t)
		linkLockdownStaffGroup(env)
		raidCleanup(t, env.chat.Id)

		env.selfJoins(14)

		if row := env.activeLockdown(); row != nil {
			t.Errorf("a lockdown exists after 14 joins, want none")
		}
		if got := len(env.calls("setChatPermissions")); got != 0 {
			t.Errorf("setChatPermissions calls = %d, want 0", got)
		}
	})

	t.Run("a join older than the window does not count", func(t *testing.T) {
		env := newLockdownEnv(t)
		linkLockdownStaffGroup(env)
		raidCleanup(t, env.chat.Id)
		realNow := raidNow
		t.Cleanup(func() { raidNow = realNow })

		raidNow = func() time.Time { return realNow().Add(-61 * time.Second) }
		env.selfJoins(1)
		raidNow = realNow
		env.selfJoins(14)

		if got := env.raidJoinedCount(); got != 15 {
			t.Fatalf("the joined set holds %d accounts, want 15 (an old join stays in the set)", got)
		}
		if row := env.activeLockdown(); row != nil {
			t.Fatalf("a lockdown (count %d) exists: the join from 61 s ago was counted, want 14 in the window", row.TriggerCount)
		}

		env.selfJoins(1)
		row := env.activeLockdown()
		if row == nil {
			t.Fatal("no active lockdown after the 15th join inside the window")
		}
		if row.TriggerCount != 15 {
			t.Errorf("TriggerCount = %d, want 15 (the old join is not one of them)", row.TriggerCount)
		}
	})
}
