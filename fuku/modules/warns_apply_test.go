package modules

import (
	"context"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db"
	dbcache "github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/db/warns"
)

// applyWarnEnv is a scripted bot, a fresh group with a warn limit and mode, and a fresh member.
type applyWarnEnv struct {
	client *moduleBotClient
	bot    *gotgbot.Bot
	chat   gotgbot.Chat
	userID int64
}

// newApplyWarnEnv mints a chat and a member of its own and removes every row and cache key
// they touch, before and after the test, because CI runs all of fuku/modules in one database.
func newApplyWarnEnv(t *testing.T, limit int, mode string) *applyWarnEnv {
	t.Helper()
	client := newModuleBotClient()
	env := &applyWarnEnv{
		client: client,
		bot:    newModuleTestBot(client),
		chat:   gotgbot.Chat{Id: uniqueModuleChatID(), Type: "supergroup", Title: "Apply Warn Chat"},
		userID: uniqueLinkOwnerID(),
	}
	clean := func() {
		db.DB.Where("chat_id = ?", env.chat.Id).Delete(&models.Warns{})
		db.DB.Where("chat_id = ?", env.chat.Id).Delete(&models.WarnSettings{})
		db.DB.Where("user_id = ?", env.userID).Delete(&models.User{})
		db.DB.Where("chat_id = ?", env.chat.Id).Delete(&models.Chat{})
		dbcache.DeleteCache(dbcache.CacheKey("warns", env.userID, env.chat.Id))
		dbcache.DeleteCache(dbcache.CacheKey("warn_settings", env.chat.Id))
		dbcache.DeleteCache(dbcache.CacheKey("user", env.userID))
		dbcache.DeleteCache(dbcache.CacheKey("chat", env.chat.Id))
		dbcache.ResetLocalForTest()
	}
	clean()
	t.Cleanup(clean)
	if err := warns.SetWarnLimit(env.chat.Id, limit); err != nil {
		t.Fatalf("SetWarnLimit() error = %v", err)
	}
	if err := warns.SetWarnMode(env.chat.Id, mode); err != nil {
		t.Fatalf("SetWarnMode() error = %v", err)
	}
	return env
}

// storedWarns counts the member's stored warns straight from the database.
func (e *applyWarnEnv) storedWarns(t *testing.T) int64 {
	t.Helper()
	var n int64
	if err := db.DB.Model(&models.Warns{}).Where("chat_id = ? AND user_id = ?", e.chat.Id, e.userID).Count(&n).Error; err != nil {
		t.Fatalf("count warns: %v", err)
	}
	return n
}

// assertNoTelegramWrite fails when the fake saw a message, a restriction, a ban or an unban.
func (e *applyWarnEnv) assertNoTelegramWrite(t *testing.T) {
	t.Helper()
	for _, method := range []string{"sendMessage", "restrictChatMember", "banChatMember", "unbanChatMember"} {
		if calls := e.client.callsFor(method); len(calls) != 0 {
			t.Fatalf("%s calls = %d, want none", method, len(calls))
		}
	}
}

func TestApplyWarn(t *testing.T) {
	t.Run("below the limit", func(t *testing.T) {
		env := newApplyWarnEnv(t, 3, "mute")
		res, err := applyWarn(context.Background(), env.bot, &env.chat, env.userID, "first reason", nil)
		if err != nil {
			t.Fatalf("applyWarn() error = %v", err)
		}
		if res.Count != 1 || res.Limit != 3 || res.LimitReached || res.LimitApplied || res.LimitErr != nil {
			t.Fatalf("result = %+v, want count 1 of 3 with no limit action", res)
		}
		if len(res.Reasons) != 1 || res.Reasons[0] != "first reason" {
			t.Fatalf("reasons = %v, want [first reason]", res.Reasons)
		}
		if n := env.storedWarns(t); n != 1 {
			t.Fatalf("stored warns = %d, want 1", n)
		}
		env.assertNoTelegramWrite(t)
	})

	t.Run("afterRecord runs once inside the lock after the warn is stored", func(t *testing.T) {
		env := newApplyWarnEnv(t, 3, "mute")
		calls := 0
		var storedInside int64
		var lockHeldInside bool
		res, err := applyWarn(context.Background(), env.bot, &env.chat, env.userID, "kept", func() {
			calls++
			storedInside = env.storedWarns(t)
			mu := warnPunishLock(env.chat.Id, env.userID)
			if mu.TryLock() {
				mu.Unlock()
			} else {
				lockHeldInside = true
			}
		})
		if err != nil {
			t.Fatalf("applyWarn() error = %v", err)
		}
		if calls != 1 {
			t.Fatalf("afterRecord calls = %d, want 1", calls)
		}
		if storedInside != 1 {
			t.Fatalf("warns stored when afterRecord ran = %d, want 1", storedInside)
		}
		if !lockHeldInside {
			t.Fatal("afterRecord ran outside warnPunishLock")
		}
		if res.Count != 1 {
			t.Fatalf("Count = %d, want 1", res.Count)
		}
		// The lock is free again once applyWarn has returned.
		mu := warnPunishLock(env.chat.Id, env.userID)
		if !mu.TryLock() {
			t.Fatal("warnPunishLock still held after applyWarn returned")
		}
		mu.Unlock()
	})

	t.Run("a warn that cannot be recorded runs no afterRecord and calls nothing", func(t *testing.T) {
		env := newApplyWarnEnv(t, 3, "mute")
		captchaApplyBrokenDB(t)
		calls := 0
		_, err := applyWarn(context.Background(), env.bot, &env.chat, env.userID, "lost", func() { calls++ })
		if err == nil {
			t.Fatal("applyWarn() error = nil, want the record error")
		}
		if calls != 0 {
			t.Fatalf("afterRecord calls = %d, want 0", calls)
		}
		env.assertNoTelegramWrite(t)
	})
}
