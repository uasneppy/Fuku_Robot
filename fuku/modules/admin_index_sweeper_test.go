//go:build testtools

package modules

import (
	"context"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/adminindex"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/utils/cache"
)

// adminIndexSweepFast removes the pause between the sweep's Telegram calls for the test.
func adminIndexSweepFast(t *testing.T) {
	t.Helper()
	previous := adminIndexSweepPace
	adminIndexSweepPace = 0
	t.Cleanup(func() { adminIndexSweepPace = previous })
}

// adminIndexSweepIsolate removes every group row of chats, so the sweep walks only the chats
// the test creates, and again after the test.
func adminIndexSweepIsolate(t *testing.T) {
	t.Helper()
	clean := func() { db.DB.Where("chat_id < ?", 0).Delete(&models.Chat{}) }
	clean()
	t.Cleanup(clean)
}

// adminIndexSweepChats creates n group rows in chats, with the owner of each as its creator in
// the env's fake and one listed administrator, and returns their IDs in ascending order (the
// order the sweep walks them).
func adminIndexSweepChats(t *testing.T, env *settingsEnv, n int) (ids []int64, owners map[int64]int64) {
	t.Helper()
	owners = make(map[int64]int64, n)
	for i := 0; i < n; i++ {
		id := uniqueModuleChatID()
		// settingsCleanup cleans at once, so it comes before the row is created.
		settingsCleanup(t, id)
		if err := db.DB.Create(&models.Chat{ChatId: id, ChatName: "Sweep Chat"}).Error; err != nil {
			t.Fatalf("seed chat %d: %v", id, err)
		}
		owner := env.newOwner(gotgbot.Chat{Id: id})
		owners[id] = owner.Id
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids, owners
}

func TestAdminIndexSweepBackfill(t *testing.T) {
	adminIndexSweepFast(t)
	env := newSettingsEnv(t)
	adminIndexSweepIsolate(t)
	ids, owners := adminIndexSweepChats(t, env, 3)
	a, b, c := ids[0], ids[1], ids[2]

	// The bot is a plain member of C, where rows from earlier are left.
	env.fake.setBotRole(c, staffRoleMember)
	seedAdminIndexRoster(t, c, adminindex.Entry{UserID: 55, Role: models.AdminIndexRoleCreator})

	report := runAdminIndexSweep(context.Background(), env.bot)

	if !report.Ran || report.Chats != 3 || report.Replaced != 2 || report.Cleared != 1 || report.Unchanged != 0 || report.RateLimited {
		t.Fatalf("report = %+v, want Ran, Chats 3, Replaced 2, Cleared 1", report)
	}
	for _, id := range []int64{a, b} {
		wantAdminIndexRoster(t, id, map[int64]string{
			owners[id]:   models.AdminIndexRoleCreator,
			env.admin.Id: models.AdminIndexRoleAdministrator,
		})
	}
	wantAdminIndexRoster(t, c, map[int64]string{})
}

// adminIndexSweepLifecycleSetup shortens the sweeper's timers, counts cycles through the hook
// and guarantees the sweeper is stopped before the timers are restored. Redis is switched off
// so the sweep lock does not gate the short intervals.
func adminIndexSweepLifecycleSetup(t *testing.T, firstDelay, interval time.Duration) *atomic.Int32 {
	t.Helper()
	previousDelay, previousInterval, previousHook := adminIndexSweepFirstDelay, adminIndexSweepInterval, adminIndexSweepCycleHook
	t.Cleanup(func() {
		adminIndexSweepFirstDelay, adminIndexSweepInterval, adminIndexSweepCycleHook = previousDelay, previousInterval, previousHook
	})
	t.Cleanup(cache.DisableRedisForTest())
	adminIndexSweepFast(t)
	// Registered last so it runs first: nothing is restored under a live loop.
	t.Cleanup(StopAdminIndexSweeper)

	cycles := &atomic.Int32{}
	adminIndexSweepFirstDelay = func() time.Duration { return firstDelay }
	adminIndexSweepInterval = interval
	adminIndexSweepCycleHook = func() { cycles.Add(1) }
	return cycles
}

// adminIndexSweepWaitCycles fails the test unless at least n cycles have started within d.
func adminIndexSweepWaitCycles(t *testing.T, cycles *atomic.Int32, n int32, d time.Duration) {
	t.Helper()
	deadline := time.After(d)
	for cycles.Load() < n {
		select {
		case <-deadline:
			t.Fatalf("only %d cycle(s) started within %v, want at least %d", cycles.Load(), d, n)
		case <-time.After(5 * time.Millisecond):
		}
	}
}

func TestAdminIndexSweepLifecycle(t *testing.T) {
	t.Run("start runs cycles and stop ends them", func(t *testing.T) {
		env := newSettingsEnv(t)
		cycles := adminIndexSweepLifecycleSetup(t, 10*time.Millisecond, 50*time.Millisecond)
		adminIndexSweepIsolate(t)

		StartAdminIndexSweeper(env.bot)
		adminIndexSweepWaitCycles(t, cycles, 2, time.Second)

		sweepWithin(t, time.Second, StopAdminIndexSweeper)

		settled := cycles.Load()
		time.Sleep(150 * time.Millisecond)
		if after := cycles.Load(); after != settled {
			t.Fatalf("%d cycle(s) ran after Stop returned, want none", after-settled)
		}
		// Stop is idempotent.
		sweepWithin(t, time.Second, StopAdminIndexSweeper)
	})

	t.Run("start twice runs one loop", func(t *testing.T) {
		// One loop gives its only cycle at 10ms and the next at 210ms; a second loop would add
		// a second cycle at 10ms.
		env := newSettingsEnv(t)
		cycles := adminIndexSweepLifecycleSetup(t, 10*time.Millisecond, 200*time.Millisecond)
		adminIndexSweepIsolate(t)

		StartAdminIndexSweeper(env.bot)
		StartAdminIndexSweeper(env.bot)
		adminIndexSweepWaitCycles(t, cycles, 1, time.Second)
		time.Sleep(100 * time.Millisecond)

		if got := cycles.Load(); got != 1 {
			t.Fatalf("%d cycles ran in the first interval, want 1 (Start must be idempotent)", got)
		}
	})

	t.Run("the first cycle fills the record by itself", func(t *testing.T) {
		env := newSettingsEnv(t)
		adminIndexSweepLifecycleSetup(t, 10*time.Millisecond, time.Hour)
		adminIndexSweepCycleHook = nil
		adminIndexSweepIsolate(t)
		ids, owners := adminIndexSweepChats(t, env, 1)

		StartAdminIndexSweeper(env.bot)

		deadline := time.After(2 * time.Second)
		for len(adminIndexRoster(t, ids[0])) == 0 {
			select {
			case <-deadline:
				t.Fatal("the first cycle did not fill the roster within 2s")
			case <-time.After(5 * time.Millisecond):
			}
		}
		wantAdminIndexRoster(t, ids[0], map[int64]string{
			owners[ids[0]]: models.AdminIndexRoleCreator,
			env.admin.Id:   models.AdminIndexRoleAdministrator,
		})
	})
}

func TestAdminIndexSweepPanicSurvives(t *testing.T) {
	env := newSettingsEnv(t)
	cycles := adminIndexSweepLifecycleSetup(t, 10*time.Millisecond, 50*time.Millisecond)
	adminIndexSweepCycleHook = func() {
		if cycles.Add(1) == 1 {
			panic("scripted sweep failure")
		}
	}
	adminIndexSweepIsolate(t)

	StartAdminIndexSweeper(env.bot)

	// The first cycle panicked; the loop must still be running and finish more.
	adminIndexSweepWaitCycles(t, cycles, 3, 2*time.Second)
	sweepWithin(t, time.Second, StopAdminIndexSweeper)
}

func TestAdminIndexSweepLock(t *testing.T) {
	adminIndexSweepFast(t)

	t.Run("a held lock makes no Telegram call", func(t *testing.T) {
		env := newSettingsEnv(t)
		adminIndexSweepIsolate(t)
		ids, _ := adminIndexSweepChats(t, env, 2)
		seedAdminIndexRoster(t, ids[0], adminindex.Entry{UserID: 55, Role: models.AdminIndexRoleCreator})
		if err := cache.GetRedisClient().Set(cache.Context, adminIndexSweepLockKey, "another-replica", time.Hour).Err(); err != nil {
			t.Fatalf("seed lock: %v", err)
		}

		report := runAdminIndexSweep(context.Background(), env.bot)

		if report.Ran {
			t.Fatalf("report = %+v, want Ran=false while another replica holds the lock", report)
		}
		if n := len(env.fake.callsFor("getChatMember")) + len(env.fake.callsFor("getChatAdministrators")); n != 0 {
			t.Fatalf("%d Telegram calls were made, want none", n)
		}
		wantAdminIndexRoster(t, ids[0], map[int64]string{55: models.AdminIndexRoleCreator})
		if got := cache.GetRedisClient().Get(cache.Context, adminIndexSweepLockKey).Val(); got != "another-replica" {
			t.Fatalf("lock = %q, want the other replica's value untouched", got)
		}
	})

	t.Run("two replicas starting together make one runner", func(t *testing.T) {
		env := newSettingsEnv(t)
		adminIndexSweepIsolate(t)
		adminIndexSweepChats(t, env, 2)

		reports := make([]adminIndexSweepReport, 2)
		start := make(chan struct{})
		var wg sync.WaitGroup
		for i := range reports {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				reports[i] = runAdminIndexSweep(context.Background(), env.bot)
			}()
		}
		close(start)
		wg.Wait()

		ran := 0
		for _, r := range reports {
			if r.Ran {
				ran++
			}
		}
		if ran != 1 {
			t.Fatalf("reports = %+v, want exactly one with Ran=true", reports)
		}
		// One runner: two chats, one bot lookup each.
		if n := len(env.fake.callsFor("getChatMember")); n != 2 {
			t.Fatalf("getChatMember was called %d times, want 2 (one runner)", n)
		}
		// A finished cycle keeps the lock until its TTL.
		if got := cache.GetRedisClient().Get(cache.Context, adminIndexSweepLockKey).Val(); got != adminIndexSweepOwner {
			t.Fatalf("lock = %q, want this process's owner after a finished cycle", got)
		}
		ttl := cache.GetRedisClient().TTL(cache.Context, adminIndexSweepLockKey).Val()
		if ttl <= 0 || ttl > adminIndexSweepInterval-time.Minute {
			t.Fatalf("lock TTL = %v, want in (0, %v]", ttl, adminIndexSweepInterval-time.Minute)
		}
	})
}

func TestAdminIndexSweepWithoutRedis(t *testing.T) {
	adminIndexSweepFast(t)

	t.Run("no Redis client", func(t *testing.T) {
		t.Cleanup(cache.DisableRedisForTest())
		env := newSettingsEnv(t)
		adminIndexSweepIsolate(t)
		ids, owners := adminIndexSweepChats(t, env, 1)

		report := runAdminIndexSweep(context.Background(), env.bot)

		if !report.Ran || report.Replaced != 1 {
			t.Fatalf("report = %+v, want Ran with one chat replaced", report)
		}
		wantAdminIndexRoster(t, ids[0], map[int64]string{
			owners[ids[0]]: models.AdminIndexRoleCreator,
			env.admin.Id:   models.AdminIndexRoleAdministrator,
		})
	})

	t.Run("a Redis error", func(t *testing.T) {
		env := newSettingsEnv(t)
		adminIndexSweepIsolate(t)
		ids, owners := adminIndexSweepChats(t, env, 1)
		// A closed client makes SetNX fail; correctness must not depend on the lock.
		if err := cache.GetRedisClient().Close(); err != nil {
			t.Fatalf("close redis client: %v", err)
		}

		report := runAdminIndexSweep(context.Background(), env.bot)

		if !report.Ran || report.Replaced != 1 {
			t.Fatalf("report = %+v, want Ran with one chat replaced despite the Redis error", report)
		}
		wantAdminIndexRoster(t, ids[0], map[int64]string{
			owners[ids[0]]: models.AdminIndexRoleCreator,
			env.admin.Id:   models.AdminIndexRoleAdministrator,
		})
	})
}

func TestAdminIndexSweepKeepsRowsOnError(t *testing.T) {
	adminIndexSweepFast(t)
	env := newSettingsEnv(t)
	adminIndexSweepIsolate(t)
	ids, owners := adminIndexSweepChats(t, env, 3)
	botLookupFails, listFails, healthy := ids[0], ids[1], ids[2]

	keep := adminindex.Entry{UserID: 55, Role: models.AdminIndexRoleCreator}
	seedAdminIndexRoster(t, botLookupFails, keep)
	seedAdminIndexRoster(t, listFails, keep)
	env.fake.setFailure("getChatMember", botLookupFails,
		&gotgbot.TelegramError{Method: "getChatMember", Code: 500, Description: "Internal Server Error"})
	env.fake.setFailure("getChatAdministrators", listFails,
		&gotgbot.TelegramError{Method: "getChatAdministrators", Code: 500, Description: "Internal Server Error"})

	report := runAdminIndexSweep(context.Background(), env.bot)

	if !report.Ran || report.Chats != 3 || report.Unchanged != 2 || report.Replaced != 1 || report.Cleared != 0 {
		t.Fatalf("report = %+v, want 3 chats, 2 unchanged, 1 replaced, none cleared", report)
	}
	for _, id := range []int64{botLookupFails, listFails} {
		wantAdminIndexRoster(t, id, map[int64]string{55: models.AdminIndexRoleCreator})
	}
	// The cycle went on to the next group after the failures.
	wantAdminIndexRoster(t, healthy, map[int64]string{
		owners[healthy]: models.AdminIndexRoleCreator,
		env.admin.Id:    models.AdminIndexRoleAdministrator,
	})
}

func TestAdminIndexSweepRateLimit(t *testing.T) {
	adminIndexSweepFast(t)

	t.Run("one 429 waits and retries that group once", func(t *testing.T) {
		env := newSettingsEnv(t)
		adminIndexSweepIsolate(t)
		ids, owners := adminIndexSweepChats(t, env, 2)
		env.fake.script("getChatMember", ids[0], staffFake429(1))

		started := time.Now()
		report := runAdminIndexSweep(context.Background(), env.bot)
		elapsed := time.Since(started)

		if !report.Ran || report.RateLimited || report.Replaced != 2 || report.Chats != 2 {
			t.Fatalf("report = %+v, want both chats replaced and no rate-limit end", report)
		}
		if elapsed < 900*time.Millisecond {
			t.Fatalf("the sweep took %v, want it to wait out retry_after 1", elapsed)
		}
		if n := len(callsToChatFake(env, "getChatMember", ids[0])); n != 2 {
			t.Fatalf("getChatMember for the limited chat was called %d times, want 2", n)
		}
		wantAdminIndexRoster(t, ids[0], map[int64]string{
			owners[ids[0]]: models.AdminIndexRoleCreator,
			env.admin.Id:   models.AdminIndexRoleAdministrator,
		})
	})

	t.Run("two 429s end the cycle and leave later groups untouched", func(t *testing.T) {
		previous := adminIndexSweep429Max
		adminIndexSweep429Max = 10 * time.Millisecond
		t.Cleanup(func() { adminIndexSweep429Max = previous })

		env := newSettingsEnv(t)
		adminIndexSweepIsolate(t)
		ids, _ := adminIndexSweepChats(t, env, 3)
		later := ids[1:]
		stale := adminindex.Entry{UserID: 55, Role: models.AdminIndexRoleCreator}
		seedAdminIndexRoster(t, ids[0], stale)
		for _, id := range later {
			seedAdminIndexRoster(t, id, stale)
		}
		env.fake.script("getChatMember", ids[0], staffFake429(1), staffFake429(1))

		report := runAdminIndexSweep(context.Background(), env.bot)

		if !report.Ran || !report.RateLimited || report.Replaced != 0 {
			t.Fatalf("report = %+v, want Ran, RateLimited and nothing replaced", report)
		}
		for _, id := range later {
			if n := len(callsToChatFake(env, "getChatMember", id)); n != 0 {
				t.Fatalf("later chat %d got %d getChatMember call(s), want none", id, n)
			}
		}
		for _, id := range ids {
			wantAdminIndexRoster(t, id, map[int64]string{55: models.AdminIndexRoleCreator})
		}
		// A rate-limited end is not a cancel: the lock stays until its TTL.
		if got := cache.GetRedisClient().Get(cache.Context, adminIndexSweepLockKey).Val(); got != adminIndexSweepOwner {
			t.Fatalf("lock = %q, want it kept after a rate-limited end", got)
		}
	})

	t.Run("a long retry_after waits only the cap", func(t *testing.T) {
		previous := adminIndexSweep429Max
		adminIndexSweep429Max = 50 * time.Millisecond
		t.Cleanup(func() { adminIndexSweep429Max = previous })

		env := newSettingsEnv(t)
		adminIndexSweepIsolate(t)
		ids, _ := adminIndexSweepChats(t, env, 1)
		env.fake.script("getChatMember", ids[0], staffFake429(120))

		started := time.Now()
		report := runAdminIndexSweep(context.Background(), env.bot)

		if elapsed := time.Since(started); elapsed > 5*time.Second {
			t.Fatalf("the sweep took %v for retry_after 120 with a 50ms cap", elapsed)
		}
		if !report.Ran || report.Replaced != 1 || report.RateLimited {
			t.Fatalf("report = %+v, want the retry to succeed", report)
		}
	})
}

// callsToChatFake returns the recorded calls of method addressed to chatID.
func callsToChatFake(env *settingsEnv, method string, chatID int64) []moduleBotCall {
	var matched []moduleBotCall
	for _, call := range env.fake.callsFor(method) {
		if staffParamInt(call.Params, "chat_id") == chatID {
			matched = append(matched, call)
		}
	}
	return matched
}

func TestAdminIndexSweepStops(t *testing.T) {
	previous := adminIndexSweepPace
	adminIndexSweepPace = 50 * time.Millisecond
	t.Cleanup(func() { adminIndexSweepPace = previous })

	env := newSettingsEnv(t)
	adminIndexSweepIsolate(t)
	adminIndexSweepChats(t, env, 10)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		runAdminIndexSweep(ctx, env.bot)
	}()

	// Wait until the sweep is in its first group, then cancel during the pace.
	deadline := time.After(5 * time.Second)
	for len(env.fake.callsFor("getChatMember")) < 1 {
		select {
		case <-deadline:
			t.Fatal("the sweep never reached its first group")
		case <-time.After(5 * time.Millisecond):
		}
	}
	cancel()

	select {
	case <-finished:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("runAdminIndexSweep did not return within 500ms of the cancel")
	}

	settled := len(env.fake.callsFor("getChatMember")) + len(env.fake.callsFor("getChatAdministrators"))
	time.Sleep(150 * time.Millisecond)
	if after := len(env.fake.callsFor("getChatMember")) + len(env.fake.callsFor("getChatAdministrators")); after != settled {
		t.Fatalf("%d Telegram call(s) were made after the sweep returned, want none", after-settled)
	}
	if settled >= 20 {
		t.Fatalf("%d calls were made, want the cancel to cut the sweep short of 10 groups", settled)
	}
	// A cycle cut off by the cancel gives its lock back, so a restart is not skipped.
	if n := cache.GetRedisClient().Exists(cache.Context, adminIndexSweepLockKey).Val(); n != 0 {
		t.Fatal("the lock is still held after a cancelled cycle, want it released")
	}
}
