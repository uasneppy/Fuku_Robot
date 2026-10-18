//go:build testtools

package modules

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"

	"github.com/divkix/Alita_Robot/alita/db"
	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/utils/cache"
)

// botMemberLookups counts the getChatMember calls about the bot itself in the env's chat:
// the first call startLockdownRow makes.
func (e *lockdownEnv) botMemberLookups() int {
	e.t.Helper()
	n := 0
	for _, call := range e.calls("getChatMember") {
		if staffParamInt(call.Params, "user_id") == staffTestBotID {
			n++
		}
	}
	return n
}

// raidLatchTTL is the remaining life of the env's chat's fire latch, or a non-positive
// duration when there is none.
func (e *lockdownEnv) raidLatchTTL() time.Duration {
	e.t.Helper()
	client := cache.GetRedisClient()
	if client == nil {
		e.t.Fatal("no Redis client in the test")
	}
	ttl, err := client.PTTL(context.Background(), raidKey(e.chat.Id, raidPartFire)).Result()
	if err != nil {
		e.t.Fatalf("read the fire latch: %v", err)
	}
	return ttl
}

// dropRaidLatch deletes the env's chat's fire latch, so the next trigger reaches the
// start path as if the latch had expired or Redis had been flushed.
func (e *lockdownEnv) dropRaidLatch() {
	e.t.Helper()
	client := cache.GetRedisClient()
	if client == nil {
		e.t.Fatal("no Redis client in the test")
	}
	if err := client.Del(context.Background(), raidKey(e.chat.Id, raidPartFire)).Err(); err != nil {
		e.t.Fatalf("delete the fire latch: %v", err)
	}
}

// joinOn runs one self-join of user in the env's chat through the given dispatcher, the
// way a second replica receiving the update would. It reports a failure with Errorf so a
// goroutine may call it.
func (e *lockdownEnv) joinOn(d *ext.Dispatcher, user gotgbot.User) {
	id := e.updateID()
	update := &gotgbot.Update{
		UpdateId: id,
		ChatMember: &gotgbot.ChatMemberUpdated{
			Chat:          e.chat,
			From:          user,
			Date:          1,
			OldChatMember: gotgbot.ChatMemberLeft{User: user},
			NewChatMember: gotgbot.ChatMemberMember{User: user},
		},
	}
	if err := d.ProcessUpdate(e.bot, update, nil); err != nil {
		e.t.Errorf("ProcessUpdate(%d) error = %v", id, err)
	}
}

// lockdownRowCount is how many lockdown rows, of any state, the env's chat has.
func (e *lockdownEnv) lockdownRowCount() int64 {
	e.t.Helper()
	var rows int64
	if err := db.DB.Model(&models.ChatLockdown{}).Where("chat_id = ?", e.chat.Id).Count(&rows).Error; err != nil {
		e.t.Fatalf("count lockdown rows: %v", err)
	}
	return rows
}

// alertCopyCount is how many alert copies (not reminders) the lockdown has.
func (e *lockdownEnv) alertCopyCount(lockdownID uint) int64 {
	e.t.Helper()
	var copies int64
	err := db.DB.Model(&models.ChatLockdownAlert{}).
		Where("lockdown_id = ? AND kind = ?", lockdownID, models.AlertKindAlert).Count(&copies).Error
	if err != nil {
		e.t.Fatalf("count alert copies: %v", err)
	}
	return copies
}

func TestRaidTriggersSingleLockdown(t *testing.T) {
	t.Run("surge then flood", func(t *testing.T) {
		env := newLockdownEnv(t)
		staffChat := linkLockdownStaffGroup(env)
		raidCleanup(t, env.chat.Id)
		recent := env.newMembers(4, 2*time.Hour)

		env.selfJoins(15)
		row := env.activeLockdown()
		if row == nil || row.TriggerKind != models.LockdownTriggerJoinSurge {
			t.Fatalf("setup: active lockdown = %+v, want one started by the join surge", row)
		}
		// The first counted message loads the chat's admin list, which reads the bot's own
		// membership and getChat. Send it before the snapshot so only the trigger's calls
		// are compared.
		env.sendItems(recent, 1)
		botLookups := env.botMemberLookups()
		chatReads := len(env.calls("getChat"))

		env.dropRaidLatch()
		env.sendItems(recent, 29)

		if env.raidLatchTTL() <= 0 {
			t.Fatal("the fire latch was not taken again, so the flood trigger never fired and the test proves nothing")
		}
		if got := env.lockdownRowCount(); got != 1 {
			t.Errorf("lockdown rows = %d, want 1", got)
		}
		if after := env.activeLockdown(); after == nil || after.ID != row.ID || after.TriggerKind != models.LockdownTriggerJoinSurge {
			t.Errorf("active lockdown after the flood = %+v, want the surge's lockdown %d unchanged", after, row.ID)
		}
		if got := len(env.calls("setChatPermissions")); got != 1 {
			t.Errorf("setChatPermissions calls = %d, want exactly 1", got)
		}
		if got := env.botMemberLookups(); got != botLookups {
			t.Errorf("getChatMember calls about the bot = %d, want %d: a trigger that finds a lockdown active makes no Telegram call", got, botLookups)
		}
		if got := len(env.calls("getChat")); got != chatReads {
			t.Errorf("getChat calls = %d, want %d", got, chatReads)
		}

		env.cycle()
		if sent := env.fake.sentTo(staffChat.Id); len(sent) != 1 {
			t.Errorf("%d messages reached the Staff Group, want exactly 1", len(sent))
		}
	})

	t.Run("flood then surge", func(t *testing.T) {
		env := newLockdownEnv(t)
		staffChat := linkLockdownStaffGroup(env)
		raidCleanup(t, env.chat.Id)
		recent := env.newMembers(4, 2*time.Hour)

		env.sendItems(recent, 30)
		row := env.activeLockdown()
		if row == nil || row.TriggerKind != models.LockdownTriggerFlood {
			t.Fatalf("setup: active lockdown = %+v, want one started by the flood", row)
		}
		botLookups := env.botMemberLookups()
		chatReads := len(env.calls("getChat"))

		env.dropRaidLatch()
		env.selfJoins(15)

		if env.raidLatchTTL() <= 0 {
			t.Fatal("the fire latch was not taken again, so the join surge never fired and the test proves nothing")
		}
		if got := env.lockdownRowCount(); got != 1 {
			t.Errorf("lockdown rows = %d, want 1", got)
		}
		if after := env.activeLockdown(); after == nil || after.ID != row.ID || after.TriggerKind != models.LockdownTriggerFlood {
			t.Errorf("active lockdown after the surge = %+v, want the flood's lockdown %d unchanged", after, row.ID)
		}
		if got := len(env.calls("setChatPermissions")); got != 1 {
			t.Errorf("setChatPermissions calls = %d, want exactly 1", got)
		}
		if got := env.botMemberLookups(); got != botLookups {
			t.Errorf("getChatMember calls about the bot = %d, want %d: a trigger that finds a lockdown active makes no Telegram call", got, botLookups)
		}
		if got := len(env.calls("getChat")); got != chatReads {
			t.Errorf("getChat calls = %d, want %d", got, chatReads)
		}

		env.cycle()
		if sent := env.fake.sentTo(staffChat.Id); len(sent) != 1 {
			t.Errorf("%d messages reached the Staff Group, want exactly 1", len(sent))
		}
	})
}

// runReplicas feeds the users to the two dispatchers, the first half to the first and the
// rest to the second, from two goroutines at once. While it runs, a third goroutine keeps
// deleting the fire latch when dropLatch is set, so every counted join past the threshold
// reaches the start path: the database alone has to give one lockdown.
func runReplicas(env *lockdownEnv, second *ext.Dispatcher, users []gotgbot.User, dropLatch bool) {
	env.t.Helper()
	half := len(users) / 2

	stop := make(chan struct{})
	var dropper sync.WaitGroup
	if dropLatch {
		client := cache.GetRedisClient()
		dropper.Add(1)
		go func() {
			defer dropper.Done()
			for {
				select {
				case <-stop:
					return
				default:
					_ = client.Del(context.Background(), raidKey(env.chat.Id, raidPartFire)).Err()
					time.Sleep(time.Millisecond)
				}
			}
		}()
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for _, user := range users[:half] {
			env.joinOn(env.dispatcher, user)
		}
	}()
	go func() {
		defer wg.Done()
		for _, user := range users[half:] {
			env.joinOn(second, user)
		}
	}()
	wg.Wait()
	close(stop)
	dropper.Wait()
}

func TestRaidTriggersConcurrent(t *testing.T) {
	for _, tc := range []struct {
		name      string
		dropLatch bool
	}{
		{name: "two replicas", dropLatch: false},
		{name: "latch deleted", dropLatch: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := newLockdownEnv(t)
			staffChat := linkLockdownStaffGroup(env)
			raidCleanup(t, env.chat.Id)
			second := ext.NewDispatcher(&ext.DispatcherOpts{MaxRoutines: -1})
			LoadLockdown(second)

			users := make([]gotgbot.User, 0, 20)
			for i := 0; i < 20; i++ {
				users = append(users, env.newJoiner(fmt.Sprintf("Raider%d", i)))
			}
			runReplicas(env, second, users, tc.dropLatch)

			row := env.activeLockdown()
			if row == nil {
				t.Fatal("no active lockdown after 20 joins in a minute")
			}
			if row.TriggerKind != models.LockdownTriggerJoinSurge {
				t.Errorf("TriggerKind = %q, want %q", row.TriggerKind, models.LockdownTriggerJoinSurge)
			}
			if got := env.lockdownRowCount(); got != 1 {
				t.Errorf("lockdown rows = %d, want exactly 1", got)
			}
			if got := len(env.calls("setChatPermissions")); got != 1 {
				t.Errorf("setChatPermissions calls = %d, want exactly 1", got)
			}

			env.cycleUntilIdle()
			if copies := env.alertCopyCount(row.ID); copies != 1 {
				t.Errorf("alert copies = %d, want 1 (the Staff Group)", copies)
			}
			if sent := env.fake.sentTo(staffChat.Id); len(sent) != 1 {
				t.Errorf("%d messages reached the Staff Group, want exactly 1", len(sent))
			}
		})
	}
}

// setupRaidLift locks the env's chat with a join surge, lifts it as the administrator and
// lets the worker finish the lift. It returns the lifted lockdown's ID.
func setupRaidLift(env *lockdownEnv) uint {
	env.t.Helper()
	env.selfJoins(15)
	row := env.activeLockdown()
	if row == nil {
		env.t.Fatal("setup: the join surge did not lock the group")
	}
	env.send(env.admin, "/unlockdown")
	env.cycleUntilIdle()
	if env.activeLockdown() != nil {
		env.t.Fatal("setup: the lockdown is still active after /unlockdown")
	}
	if lifted := env.freshRow(row.ID); lifted.LiftStartedAt == nil {
		env.t.Fatal("setup: the lifted lockdown has no lift_started_at")
	}
	return row.ID
}

func TestRaidCooldownAfterLift(t *testing.T) {
	env := newLockdownEnv(t)
	linkLockdownStaffGroup(env)
	raidCleanup(t, env.chat.Id)
	liftedID := setupRaidLift(env)

	// Within the quiet period the triggers start nothing and ask Telegram nothing. The
	// earlier joins still sit in the 60 second window, so every new join is past the
	// threshold.
	env.dropRaidLatch()
	locks := len(env.calls("setChatPermissions"))
	botLookups := env.botMemberLookups()
	chatReads := len(env.calls("getChat"))

	env.selfJoins(15)

	if env.raidLatchTTL() <= 0 {
		t.Fatal("the fire latch was not taken, so the trigger never fired and the test proves nothing")
	}
	if row := env.activeLockdown(); row != nil {
		t.Errorf("a lockdown (kind %q) started within 10 minutes of a lift, want none", row.TriggerKind)
	}
	if got := len(env.calls("setChatPermissions")); got != locks {
		t.Errorf("setChatPermissions calls = %d, want %d", got, locks)
	}
	if got := env.botMemberLookups(); got != botLookups {
		t.Errorf("getChatMember calls about the bot = %d, want %d", got, botLookups)
	}
	if got := len(env.calls("getChat")); got != chatReads {
		t.Errorf("getChat calls = %d, want %d", got, chatReads)
	}

	// Once the lift is more than 10 minutes old the triggers lock again.
	err := db.DB.Model(&models.ChatLockdown{}).Where("id = ?", liftedID).
		UpdateColumn("lift_started_at", time.Now().UTC().Add(-11*time.Minute)).Error
	if err != nil {
		t.Fatalf("move the lift back: %v", err)
	}
	env.dropRaidLatch()
	env.selfJoins(1)

	row := env.activeLockdown()
	if row == nil {
		t.Fatal("no lockdown 11 minutes after the lift, want the join surge to lock again")
	}
	if row.ID == liftedID || row.TriggerKind != models.LockdownTriggerJoinSurge {
		t.Errorf("active lockdown = %d (%s), want a new join_surge lockdown", row.ID, row.TriggerKind)
	}
	if got := len(env.calls("setChatPermissions")); got != locks+1 {
		t.Errorf("setChatPermissions calls = %d, want %d", got, locks+1)
	}
}

func TestRaidLatch(t *testing.T) {
	t.Run("started", func(t *testing.T) {
		env := newLockdownEnv(t)
		linkLockdownStaffGroup(env)
		raidCleanup(t, env.chat.Id)

		env.selfJoins(15)

		if env.activeLockdown() == nil {
			t.Fatal("the join surge did not lock the group")
		}
		if ttl := env.raidLatchTTL(); ttl <= 50*time.Second || ttl > 60*time.Second {
			t.Errorf("latch after a started lockdown = %s, want above 50s and at most 60s", ttl)
		}
	})

	t.Run("lock answer lost", func(t *testing.T) {
		env := newLockdownEnv(t)
		linkLockdownStaffGroup(env)
		raidCleanup(t, env.chat.Id)
		env.fake.setFailAfterSetPermissions(errors.New("connection reset by peer"))

		env.selfJoins(15)

		row := env.activeLockdown()
		if row == nil || row.LockedAt != nil {
			t.Fatalf("active lockdown = %+v, want the kept unconfirmed row", row)
		}
		if ttl := env.raidLatchTTL(); ttl <= 50*time.Second || ttl > 60*time.Second {
			t.Errorf("latch after an unknown lock = %s, want above 50s and at most 60s: the worker settles the row", ttl)
		}
	})

	t.Run("transient", func(t *testing.T) {
		env := newLockdownEnv(t)
		linkLockdownStaffGroup(env)
		raidCleanup(t, env.chat.Id)
		env.fake.script("getChatMember", env.chat.Id, staffFakeError(500, "Internal Server Error"))

		env.selfJoins(15)

		if row := env.activeLockdown(); row != nil {
			t.Fatalf("a lockdown exists although the bot's rights could not be read: %+v", row)
		}
		if got := len(env.calls("setChatPermissions")); got != 0 {
			t.Errorf("setChatPermissions calls = %d, want 0", got)
		}
		if ttl := env.raidLatchTTL(); ttl <= 0 || ttl > 10*time.Second {
			t.Errorf("latch after a transient failure = %s, want above 0 and at most 10s", ttl)
		}

		// The latch is gone about 10 seconds later and the next qualifying join tries again.
		env.dropRaidLatch()
		env.selfJoins(1)
		if env.activeLockdown() == nil {
			t.Error("no lockdown after the retry, want the next join past the threshold to lock")
		}
	})

	t.Run("permissions unreadable", func(t *testing.T) {
		env := newLockdownEnv(t)
		linkLockdownStaffGroup(env)
		raidCleanup(t, env.chat.Id)
		env.fake.clearChatPerms(env.chat.Id)

		env.selfJoins(15)

		if row := env.activeLockdown(); row != nil {
			t.Fatalf("a lockdown exists although the permissions could not be read: %+v", row)
		}
		if ttl := env.raidLatchTTL(); ttl <= 0 || ttl > 10*time.Second {
			t.Errorf("latch after unreadable permissions = %s, want above 0 and at most 10s", ttl)
		}
	})

	t.Run("cannot lock", func(t *testing.T) {
		env := newLockdownEnv(t)
		linkLockdownStaffGroup(env)
		raidCleanup(t, env.chat.Id)
		env.fake.setBotRole(env.chat.Id, staffRoleAdminNoRestrict)

		env.selfJoins(15)

		if row := env.activeLockdown(); row != nil {
			t.Fatalf("a lockdown exists although the bot cannot restrict members: %+v", row)
		}
		if got := env.lockdownRowCount(); got != 0 {
			t.Errorf("lockdown rows = %d, want none recorded", got)
		}
		if ttl := env.raidLatchTTL(); ttl <= 9*time.Minute || ttl > 10*time.Minute {
			t.Errorf("latch for a group the bot cannot lock = %s, want above 9 minutes and at most 10", ttl)
		}
	})

	t.Run("lock refused", func(t *testing.T) {
		env := newLockdownEnv(t)
		linkLockdownStaffGroup(env)
		raidCleanup(t, env.chat.Id)
		env.fake.script("setChatPermissions", env.chat.Id, staffFakeError(400, "Bad Request: not enough rights"))

		env.selfJoins(15)

		if got := env.lockdownRowCount(); got != 0 {
			t.Errorf("lockdown rows = %d, want none: a refused lock leaves no row", got)
		}
		if ttl := env.raidLatchTTL(); ttl <= 9*time.Minute || ttl > 10*time.Minute {
			t.Errorf("latch after a refused lock = %s, want above 9 minutes and at most 10", ttl)
		}
	})

	t.Run("latched", func(t *testing.T) {
		env := newLockdownEnv(t)
		linkLockdownStaffGroup(env)
		raidCleanup(t, env.chat.Id)
		env.fake.setBotRole(env.chat.Id, staffRoleAdminNoRestrict)
		env.selfJoins(15)
		botLookups := env.botMemberLookups()
		chatReads := len(env.calls("getChat"))

		env.selfJoins(1)

		if got := env.botMemberLookups(); got != botLookups {
			t.Errorf("getChatMember calls about the bot = %d, want %d while the latch is held", got, botLookups)
		}
		if got := len(env.calls("getChat")); got != chatReads {
			t.Errorf("getChat calls = %d, want %d while the latch is held", got, chatReads)
		}
		if got := len(env.calls("setChatPermissions")); got != 0 {
			t.Errorf("setChatPermissions calls = %d, want 0 while the latch is held", got)
		}
	})
}
