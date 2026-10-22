//go:build testtools

package modules

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/redis/go-redis/v9"
	"github.com/vmihailenco/msgpack/v5"

	"github.com/divkix/Alita_Robot/alita/db"
	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/utils/cache"
)

// legacyRaidState is the value the old raid mode kept under alita:antiraid:state:<chat>:
// msgpack of a struct with these field names, written by a replica that still runs the
// old code.
type legacyRaidState struct {
	Active    bool
	StartedAt int64
	ExpiresAt int64
}

// seedLegacyRaidKeys writes the Redis keys an old replica leaves behind for chatID: an
// active raid state that expires in an hour and a join list with one entry.
func seedLegacyRaidKeys(t *testing.T, chatID int64) {
	t.Helper()
	now := time.Now().Unix()
	payload, err := msgpack.Marshal(legacyRaidState{Active: true, StartedAt: now, ExpiresAt: now + 3600})
	if err != nil {
		t.Fatalf("encode the legacy raid state: %v", err)
	}
	rdb := cache.GetRedisClient()
	if rdb == nil {
		t.Fatal("no Redis client: the test needs miniredis")
	}
	if err := rdb.Set(cache.Context, fmt.Sprintf("alita:antiraid:state:%d", chatID), payload, time.Hour).Err(); err != nil {
		t.Fatalf("write the legacy raid state: %v", err)
	}
	if err := rdb.ZAdd(cache.Context, fmt.Sprintf("alita:antiraid:joins:%d", chatID),
		redis.Z{Score: float64(now), Member: "1:old"}).Err(); err != nil {
		t.Fatalf("write the legacy join list: %v", err)
	}
}

// seedAntiRaidSettings writes an antiraid_settings row with recognisable values, so a
// test can show the retired commands leave it as it was.
func seedAntiRaidSettings(t *testing.T, chatID int64) models.AntiRaidSettings {
	t.Helper()
	row := models.AntiRaidSettings{ChatID: chatID, RaidTime: 777, RaidActionTime: 888, AutoAntiRaidThreshold: 44}
	if err := db.DB.Create(&row).Error; err != nil {
		t.Fatalf("seed antiraid_settings: %v", err)
	}
	t.Cleanup(func() { db.DB.Where("chat_id = ?", chatID).Delete(&models.AntiRaidSettings{}) })
	return row
}

// wantAntiRaidSettings fails unless the chat's antiraid_settings row still has the
// values seedAntiRaidSettings wrote.
func wantAntiRaidSettings(t *testing.T, chatID int64) {
	t.Helper()
	var rows []models.AntiRaidSettings
	if err := db.DB.Where("chat_id = ?", chatID).Find(&rows).Error; err != nil {
		t.Fatalf("read antiraid_settings: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("antiraid_settings rows = %d, want the 1 that was seeded", len(rows))
	}
	got := rows[0]
	if got.RaidTime != 777 || got.RaidActionTime != 888 || got.AutoAntiRaidThreshold != 44 {
		t.Errorf("antiraid_settings = raid %d, action %d, auto %d; want it untouched at 777, 888, 44",
			got.RaidTime, got.RaidActionTime, got.AutoAntiRaidThreshold)
	}
}

// quietAntiRaidSweep marks the startup sweep of the retired keys as already run, so a
// test that loads the module does not start a background sweep against its Redis. The
// tests of the sweep itself call it directly or reset the Once.
func quietAntiRaidSweep(t *testing.T) {
	t.Helper()
	antiraidSweepOnce.Do(func() {})
}

// loadAntiRaid loads the AntiRaid module on the env's dispatcher without the background
// sweep.
func (e *lockdownEnv) loadAntiRaid() {
	e.t.Helper()
	quietAntiRaidSweep(e.t)
	LoadAntiRaid(e.dispatcher)
}

// pressAntiRaid presses an old antiraid button (data built the way the old command
// built it) as from, on message msgID of the env's chat.
func (e *lockdownEnv) pressAntiRaid(from gotgbot.User, msgID int64, action string) {
	e.t.Helper()
	data := encodeCallbackData("antiraid", map[string]string{"a": action})
	if data == "" {
		e.t.Fatalf("callback data for antiraid action %s did not encode", action)
	}
	id := e.updateID()
	update := &gotgbot.Update{
		UpdateId: id,
		CallbackQuery: &gotgbot.CallbackQuery{
			Id:           fmt.Sprintf("cb-%d", id),
			From:         from,
			Message:      gotgbot.Message{MessageId: msgID, Date: 1, Chat: e.chat},
			Data:         data,
			ChatInstance: "antiraid-retired-test",
		},
	}
	if err := e.dispatcher.ProcessUpdate(e.bot, update, nil); err != nil {
		e.t.Fatalf("ProcessUpdate(%d) error = %v", id, err)
	}
}

func TestAntiRaidRetired(t *testing.T) {
	t.Run("pointers", func(t *testing.T) {
		env := newLockdownEnv(t)
		env.loadAntiRaid()
		raidCleanup(t, env.chat.Id)
		seedAntiRaidSettings(t, env.chat.Id)

		for _, text := range []string{"/antiraid", "/antiraid on", "/raidtime 6h", "/raidactiontime 1h"} {
			env.send(env.admin, text)
			env.wantReplyHas(staffMarker("lockdown_antiraid_retired"))
		}

		wantAntiRaidSettings(t, env.chat.Id)
		env.wantNoAutoRow()
		if got := len(env.calls("getChatMember")); got != 0 {
			t.Errorf("getChatMember calls = %d, want 0: the pointer is static text and judges nobody", got)
		}
	})

	t.Run("alias sets the surge", func(t *testing.T) {
		env := newLockdownEnv(t)
		env.loadAntiRaid()
		raidCleanup(t, env.chat.Id)

		env.send(env.admin, "/autoantiraid 20")
		env.wantReplyHas(staffMarker("lockdown_auto_saved"), staffMarker("lockdown_auto_joins_on")+" 20")
		env.wantAutoRow(true, 20, true, 30)

		env.send(env.admin, "/autoantiraid off")
		env.wantReplyHas(staffMarker("lockdown_auto_saved"))
		env.wantAutoRow(false, 20, true, 30)

		env.send(env.admin, "/autoantiraid")
		env.wantReplyHas(staffMarker("lockdown_auto_title"), staffMarker("lockdown_auto_joins_off")+" 20")
		env.wantAutoRow(false, 20, true, 30)

		env.send(env.admin, "/autoantiraid 2")
		env.wantReplyHas(staffMarker("lockdown_auto_range") + " 3 500")
		env.wantAutoRow(false, 20, true, 30)
	})

	t.Run("alias authority", func(t *testing.T) {
		env := newLockdownEnv(t)
		env.loadAntiRaid()
		raidCleanup(t, env.chat.Id)
		member := env.newOutsider()
		env.fake.setMember(env.chat.Id, member.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})

		env.send(member, "/autoantiraid 20")
		env.wantReplyHas(staffMarker("chat_status_user_admin_cmd_error"))
		env.wantNoAutoRow()

		helper := env.newOutsider()
		env.fake.setMember(env.chat.Id, helper.Id, staffFakeMember{
			Status:             gotgbot.ChatMemberStatusAdministrator,
			CanRestrictMembers: false,
		})
		env.send(helper, "/autoantiraid 20")
		env.wantReplyHas(staffMarker("chat_status_restrict_cmd_error"))
		env.wantNoAutoRow()
	})

	t.Run("alias anonymous admin", func(t *testing.T) {
		env, creator := newAnonLockdownEnv(t, false)
		env.loadAntiRaid()
		raidCleanup(t, env.chat.Id)

		env.sendAnonymous("/autoantiraid 20")
		env.wantProofPrompt()
		env.wantNoAutoRow()

		env.tapProof(creator)
		env.wantReplyHas(staffMarker("lockdown_auto_saved"), staffMarker("lockdown_auto_joins_on")+" 20")
		env.wantAutoRow(true, 20, true, 30)
	})

	t.Run("old button", func(t *testing.T) {
		env := newLockdownEnv(t)
		env.loadAntiRaid()
		raidCleanup(t, env.chat.Id)
		seedAntiRaidSettings(t, env.chat.Id)
		seedLegacyRaidKeys(t, env.chat.Id)

		for _, action := range []string{"on", "off"} {
			before := len(env.fake.callsFor("answerCallbackQuery"))
			env.pressAntiRaid(env.admin, 4242, action)

			if got := len(env.fake.callsFor("answerCallbackQuery")) - before; got != 1 {
				t.Fatalf("press %q answered %d times, want exactly 1", action, got)
			}
			text, _ := env.lastAnswer()
			if want := staffMarker("lockdown_antiraid_retired"); !strings.Contains(text, want) {
				t.Errorf("answer to %q = %q, want it to contain %q", action, text, want)
			}
		}

		if got := len(env.calls("editMessageText")); got != 0 {
			t.Errorf("editMessageText calls = %d, want 0: an old button changes nothing", got)
		}
		wantAntiRaidSettings(t, env.chat.Id)
		env.wantNoAutoRow()
		rdb := cache.GetRedisClient()
		if n, err := rdb.Exists(cache.Context, fmt.Sprintf("alita:antiraid:state:%d", env.chat.Id)).Result(); err != nil || n != 1 {
			t.Errorf("legacy state key exists = %d, %v; want it left alone by the button", n, err)
		}
	})
}

func TestAntiRaidRetiredNoBan(t *testing.T) {
	env := newLockdownEnv(t)
	env.loadAntiRaid()
	raidCleanup(t, env.chat.Id)
	seedLegacyRaidKeys(t, env.chat.Id)

	// Two joins by their own doing through chat_member updates, two through service
	// messages: every path a joiner can arrive by.
	for i := 0; i < 2; i++ {
		user := env.newJoiner(fmt.Sprintf("Member%d", i))
		env.join(user, user, "")
	}
	for i := 0; i < 2; i++ {
		user := env.newJoiner(fmt.Sprintf("Message%d", i))
		env.serviceJoin(user, nil, user)
	}

	if got := len(env.calls("banChatMember")); got != 0 {
		t.Errorf("banChatMember calls = %d, want 0: the old raid mode no longer temp-bans", got)
	}
	if got := len(env.calls("restrictChatMember")); got != 0 {
		t.Errorf("restrictChatMember calls = %d, want 0", got)
	}
	if row := env.activeLockdown(); row != nil {
		t.Errorf("a lockdown (kind %q) exists after 4 joins, want none", row.TriggerKind)
	}
}

func TestAntiRaidKeySweep(t *testing.T) {
	retired := []string{
		"alita:antiraid:state:-1001",
		"alita:antiraid:joins:-1001",
		"alita:antiraid:state:-1002",
	}
	kept := []string{
		"alita:lockdown:recent:-1001",
		"alita:raid:{-1001}:joined",
	}

	t.Run("removes exactly the retired keys", func(t *testing.T) {
		mr := withMiniredisServer(t)
		for _, key := range append(append([]string{}, retired...), kept...) {
			mr.Set(key, "x")
		}

		removed, err := sweepRetiredAntiRaidKeys(context.Background())
		if err != nil || removed != 3 {
			t.Fatalf("first sweep = %d, %v; want 3, nil", removed, err)
		}
		for _, key := range retired {
			if mr.Exists(key) {
				t.Errorf("retired key %s survived the sweep", key)
			}
		}
		for _, key := range kept {
			if !mr.Exists(key) {
				t.Errorf("key %s was removed, but it is not a retired key", key)
			}
		}

		removed, err = sweepRetiredAntiRaidKeys(context.Background())
		if err != nil || removed != 0 {
			t.Errorf("second sweep = %d, %v; want 0, nil", removed, err)
		}
	})

	t.Run("sweeps more keys than one scan batch", func(t *testing.T) {
		mr := withMiniredisServer(t)
		for i := 0; i < 350; i++ {
			mr.Set(fmt.Sprintf("alita:antiraid:state:%d", -2000-i), "x")
		}
		mr.Set("alita:raid:{-2000}:joined", "x")

		removed, err := sweepRetiredAntiRaidKeys(context.Background())
		if err != nil || removed != 350 {
			t.Fatalf("sweep = %d, %v; want 350, nil", removed, err)
		}
		if got := len(mr.Keys()); got != 1 {
			t.Errorf("keys left = %d, want only the one that is not retired", got)
		}
	})

	t.Run("without Redis it returns at once", func(t *testing.T) {
		restore := cache.DisableRedisForTest()
		t.Cleanup(restore)

		removed, err := sweepRetiredAntiRaidKeys(context.Background())
		if err != nil || removed != 0 {
			t.Errorf("sweep without Redis = %d, %v; want 0, nil", removed, err)
		}
	})

	t.Run("a Redis failure is returned and keeps the keys", func(t *testing.T) {
		mr := withMiniredisServer(t)
		mr.Set(retired[0], "x")
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		removed, err := sweepRetiredAntiRaidKeys(ctx)
		if err == nil {
			t.Error("sweep with a cancelled context returned no error")
		}
		if removed != 0 || !mr.Exists(retired[0]) {
			t.Errorf("removed = %d, key exists = %v; want nothing removed", removed, mr.Exists(retired[0]))
		}
	})
}

func TestAntiRaidSweepRunsOnceOnLoad(t *testing.T) {
	mr := withMiniredisServer(t)
	antiraidSweepOnce = sync.Once{}
	t.Cleanup(func() { quietAntiRaidSweep(t) })

	mr.Set("alita:antiraid:state:-3001", "x")
	LoadAntiRaid(ext.NewDispatcher(&ext.DispatcherOpts{MaxRoutines: -1}))

	deadline := time.Now().Add(5 * time.Second)
	for mr.Exists("alita:antiraid:state:-3001") {
		if time.Now().After(deadline) {
			t.Fatal("the retired key was still in Redis 5 s after the module loaded")
		}
		time.Sleep(10 * time.Millisecond)
	}

	// A second load in the same process does not sweep again.
	mr.Set("alita:antiraid:state:-3002", "x")
	LoadAntiRaid(ext.NewDispatcher(&ext.DispatcherOpts{MaxRoutines: -1}))
	time.Sleep(300 * time.Millisecond)
	if !mr.Exists("alita:antiraid:state:-3002") {
		t.Error("a second load swept again, want one sweep per process")
	}
}
