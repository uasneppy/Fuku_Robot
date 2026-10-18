//go:build testtools

package modules

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
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
		LoadAntiRaid(env.dispatcher)
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
		LoadAntiRaid(env.dispatcher)
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
		LoadAntiRaid(env.dispatcher)
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
		LoadAntiRaid(env.dispatcher)
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
		LoadAntiRaid(env.dispatcher)
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
	LoadAntiRaid(env.dispatcher)
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
