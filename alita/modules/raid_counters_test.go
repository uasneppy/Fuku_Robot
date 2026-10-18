//go:build testtools

package modules

import (
	"context"
	"strings"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
	logrustest "github.com/sirupsen/logrus/hooks/test"

	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/utils/cache"
)

// raidSetClock moves the counters' clock for the test and restores it when the test ends.
func raidSetClock(t *testing.T, now func() time.Time) {
	t.Helper()
	previous := raidNow
	raidNow = now
	t.Cleanup(func() { raidNow = previous })
}

// raidJoinedMembers returns the user IDs in a chat's joined set, oldest first.
func raidJoinedMembers(t *testing.T, chatID int64) []string {
	t.Helper()
	client := cache.GetRedisClient()
	if client == nil {
		t.Fatal("no Redis client in the test")
	}
	members, err := client.ZRange(context.Background(), raidKey(chatID, raidPartJoined), 0, -1).Result()
	if err != nil {
		t.Fatalf("read the joined set: %v", err)
	}
	return members
}

func TestRaidJoinScript(t *testing.T) {
	t.Run("the same user twice counts once", func(t *testing.T) {
		withMiniredis(t)
		chatID := uniqueModuleChatID()

		first, err := recordRaidJoins(chatID, []int64{42})
		if err != nil {
			t.Fatalf("recordRaidJoins: %v", err)
		}
		second, err := recordRaidJoins(chatID, []int64{42})
		if err != nil {
			t.Fatalf("recordRaidJoins: %v", err)
		}
		if first != 1 || second != 1 {
			t.Errorf("counts = %d then %d, want 1 and 1: both deliveries of a join are one account", first, second)
		}
		if members := raidJoinedMembers(t, chatID); len(members) != 1 {
			t.Errorf("the joined set holds %v, want one member", members)
		}
	})

	t.Run("a join older than the window stays in the set but is not counted", func(t *testing.T) {
		withMiniredis(t)
		chatID := uniqueModuleChatID()
		now := time.Now()

		raidSetClock(t, func() time.Time { return now.Add(-61 * time.Second) })
		if _, err := recordRaidJoins(chatID, []int64{10}); err != nil {
			t.Fatalf("recordRaidJoins: %v", err)
		}
		raidSetClock(t, func() time.Time { return now })
		count, err := recordRaidJoins(chatID, []int64{11})
		if err != nil {
			t.Fatalf("recordRaidJoins: %v", err)
		}

		if count != 1 {
			t.Errorf("count = %d, want 1: the join from 61 s ago is outside the window", count)
		}
		if members := raidJoinedMembers(t, chatID); len(members) != 2 {
			t.Errorf("the joined set holds %v, want both accounts: the flood trigger needs the 24 h history", members)
		}
	})

	t.Run("a join older than 24 hours is trimmed", func(t *testing.T) {
		withMiniredis(t)
		chatID := uniqueModuleChatID()
		now := time.Now()

		raidSetClock(t, func() time.Time { return now.Add(-25 * time.Hour) })
		if _, err := recordRaidJoins(chatID, []int64{20}); err != nil {
			t.Fatalf("recordRaidJoins: %v", err)
		}
		raidSetClock(t, func() time.Time { return now })
		if _, err := recordRaidJoins(chatID, []int64{21}); err != nil {
			t.Fatalf("recordRaidJoins: %v", err)
		}

		members := raidJoinedMembers(t, chatID)
		if len(members) != 1 || members[0] != "21" {
			t.Errorf("the joined set holds %v, want only account 21", members)
		}
	})

	t.Run("the cap keeps the newest", func(t *testing.T) {
		withMiniredis(t)
		chatID := uniqueModuleChatID()
		previousCap := raidJoinedCap
		raidJoinedCap = 3
		t.Cleanup(func() { raidJoinedCap = previousCap })
		start := time.Now().Add(-time.Hour)

		for i := int64(1); i <= 5; i++ {
			at := start.Add(time.Duration(i) * time.Second)
			raidSetClock(t, func() time.Time { return at })
			if _, err := recordRaidJoins(chatID, []int64{i}); err != nil {
				t.Fatalf("recordRaidJoins: %v", err)
			}
		}

		members := raidJoinedMembers(t, chatID)
		if strings.Join(members, ",") != "3,4,5" {
			t.Errorf("the joined set holds %v, want the 3 newest: 3,4,5", members)
		}
	})

	t.Run("the key expires after 25 hours", func(t *testing.T) {
		withMiniredis(t)
		chatID := uniqueModuleChatID()

		if _, err := recordRaidJoins(chatID, []int64{7}); err != nil {
			t.Fatalf("recordRaidJoins: %v", err)
		}

		ttl, err := cache.GetRedisClient().PTTL(context.Background(), raidKey(chatID, raidPartJoined)).Result()
		if err != nil {
			t.Fatalf("read the TTL: %v", err)
		}
		if ttl <= 24*time.Hour || ttl > 25*time.Hour {
			t.Errorf("TTL = %v, want above 24 h and at most 25 h", ttl)
		}
	})
}

func TestRaidInertWithoutRedis(t *testing.T) {
	env := newLockdownEnv(t)
	linkLockdownStaffGroup(env)
	raidCleanup(t, env.chat.Id)
	t.Cleanup(cache.DisableRedisForTest())

	raidNoRedisLogged.Store(false)
	t.Cleanup(func() { raidNoRedisLogged.Store(false) })
	previousHooks := log.StandardLogger().ReplaceHooks(make(log.LevelHooks))
	hook := logrustest.NewGlobal()
	previousLevel := log.GetLevel()
	log.SetLevel(log.InfoLevel)
	t.Cleanup(func() {
		log.SetLevel(previousLevel)
		log.StandardLogger().ReplaceHooks(previousHooks)
	})

	env.selfJoins(20)

	if row := env.activeLockdown(); row != nil {
		t.Errorf("a lockdown (kind %q) exists after 20 joins without Redis, want none", row.TriggerKind)
	}
	if got := len(env.calls("getChat")); got != 0 {
		t.Errorf("getChat calls = %d, want 0", got)
	}
	if got := len(env.calls("setChatPermissions")); got != 0 {
		t.Errorf("setChatPermissions calls = %d, want 0", got)
	}
	notices := 0
	for _, entry := range hook.AllEntries() {
		if entry.Level == log.InfoLevel && strings.Contains(entry.Message, "automatic raid detection is off") {
			notices++
		}
	}
	if notices != 1 {
		t.Errorf("the no-Redis notice was logged %d times, want exactly once", notices)
	}

	// /lockdown does not need Redis.
	env.send(env.admin, "/lockdown raid")
	row := env.activeLockdown()
	if row == nil || row.LockedAt == nil {
		t.Fatalf("lockdown row = %+v, want a confirmed manual lockdown without Redis", row)
	}
	if row.TriggerKind != models.LockdownTriggerManual {
		t.Errorf("TriggerKind = %q, want %q", row.TriggerKind, models.LockdownTriggerManual)
	}
	env.wantReplyHas(staffMarker("lockdown_started"))
}
