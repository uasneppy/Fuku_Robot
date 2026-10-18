//go:build testtools

package modules

import (
	"context"
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
		botLookups := env.botMemberLookups()
		chatReads := len(env.calls("getChat"))

		env.dropRaidLatch()
		env.sendItems(recent, 30)

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
