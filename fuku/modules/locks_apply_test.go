//go:build testtools

package modules

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"gorm.io/gorm"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/locks"
)

// lockApplyChat mints a chat for an apply test and removes its lock rows, chats row and cached
// copy before and after, because CI runs all of fuku/modules in one database.
func lockApplyChat(t *testing.T) int64 {
	t.Helper()
	chatID := uniqueModuleChatID()
	lockGoldenCleanup(t, chatID)
	return chatID
}

// lockCountingWrite swaps the write seam for one that records the kinds it is asked to write and
// still writes, and restores the real one when the test ends.
func lockCountingWrite(t *testing.T) *[]string {
	t.Helper()
	var kinds []string
	saved := lockWrites
	t.Cleanup(func() { lockWrites = saved })
	lockWrites.Update = func(chatID int64, kind string, on bool) error {
		kinds = append(kinds, kind)
		return saved.Update(chatID, kind, on)
	}
	return &kinds
}

func TestApplyLock(t *testing.T) {
	ctx := context.Background()

	t.Run("every kind locks and unlocks and reads back", func(t *testing.T) {
		chatID := lockApplyChat(t)
		for _, kind := range locksModule.getLockMapAsArray() {
			for _, on := range []bool{true, false} {
				res := applyLock(ctx, chatID, kind, on)
				if res.Outcome != lockApplied || res.Err != nil {
					t.Fatalf("applyLock(%s, %v) = %+v, want applied", kind, on, res)
				}
				if value, present := res.Locks[kind]; !present || value != on {
					t.Fatalf("applyLock(%s, %v) read back %v, want the kind stored as %v", kind, on, res.Locks, on)
				}
			}
		}
		stored := locks.GetChatLocks(chatID)
		if len(stored) != len(locksModule.getLockMapAsArray()) {
			t.Fatalf("stored %d kinds, want %d", len(stored), len(locksModule.getLockMapAsArray()))
		}
	})

	t.Run("a write drops the cached map once the cache was filled", func(t *testing.T) {
		chatID := lockApplyChat(t)
		if cached := locks.GetChatLocks(chatID); len(cached) != 0 {
			t.Fatalf("a chat with no rows reads %v, want an empty map", cached)
		}
		res := applyLock(ctx, chatID, "url", true)
		if res.Outcome != lockApplied || !res.Locks["url"] {
			t.Fatalf("applyLock = %+v, want applied with url read back as locked", res)
		}
		if !locks.IsPermLocked(chatID, "url") {
			t.Fatal("the cached map still held the empty answer after the write")
		}
		if res := applyLock(ctx, chatID, "url", false); res.Outcome != lockApplied || res.Locks["url"] {
			t.Fatalf("applyLock(off) = %+v, want applied with url read back as unlocked", res)
		}
		if locks.IsPermLocked(chatID, "url") {
			t.Fatal("the cached map still held url locked after the unlock")
		}
	})

	t.Run("a repeated call stores the same row", func(t *testing.T) {
		chatID := lockApplyChat(t)
		for range 3 {
			if res := applyLock(ctx, chatID, "sticker", true); res.Outcome != lockApplied {
				t.Fatalf("applyLock = %+v, want applied", res)
			}
		}
		if got := lockGoldenStored(t, chatID); got != "sticker=true" {
			t.Fatalf("stored rows = %q, want one sticker row", got)
		}
	})

	for _, kind := range []string{"bogus", "URL", "", "lock", " url", "url "} {
		t.Run("unknown kind "+kind+" is refused before any write", func(t *testing.T) {
			chatID := lockApplyChat(t)
			writes := lockCountingWrite(t)
			res := applyLock(ctx, chatID, kind, true)
			if res.Outcome != lockUnknownType || !errors.Is(res.Err, errLockUnknownType) || res.Locks != nil {
				t.Fatalf("applyLock(%q) = %+v, want an unknown type with no map", kind, res)
			}
			if len(*writes) != 0 {
				t.Fatalf("the write seam ran for %q: %v", kind, *writes)
			}
			if got := lockGoldenStored(t, chatID); got != "" {
				t.Fatalf("stored rows = %q, want none", got)
			}
		})
	}

	t.Run("a failing write is a save failure with the error and stores nothing", func(t *testing.T) {
		chatID := lockApplyChat(t)
		lockGoldenFailWrite(t, chatID, "gif")
		res := applyLock(ctx, chatID, "gif", true)
		if res.Outcome != lockSaveFailed || res.Err == nil || res.Locks != nil {
			t.Fatalf("applyLock = %+v, want a save failure with the error and no map", res)
		}
		if got := lockGoldenStored(t, chatID); got != "" {
			t.Fatalf("stored rows = %q, want none", got)
		}
		if res := applyLock(ctx, chatID, "photo", true); res.Outcome != lockApplied {
			t.Fatalf("another kind of the same chat = %+v, want applied", res)
		}
	})

	t.Run("a failed read-back is still an applied write", func(t *testing.T) {
		chatID := lockApplyChat(t)
		failRead := true
		name := "test:fail_lock_read_back"
		err := db.DB.Callback().Query().Before("gorm:query").Register(name, func(tx *gorm.DB) {
			if failRead && tx.Statement.Table == "locks" {
				_ = tx.AddError(errors.New("database is down"))
			}
		})
		if err != nil {
			t.Fatalf("register callback: %v", err)
		}
		t.Cleanup(func() {
			if err := db.DB.Callback().Query().Remove(name); err != nil {
				t.Errorf("remove callback: %v", err)
			}
		})

		res := applyLock(ctx, chatID, "forward", true)
		failRead = false
		if res.Outcome != lockApplied || res.Err != nil || res.Locks != nil {
			t.Fatalf("applyLock = %+v, want applied with no map and no error", res)
		}
		if got := lockGoldenStored(t, chatID); got != "forward=true" {
			t.Fatalf("stored rows = %q, want the write to have landed", got)
		}
	})

	t.Run("the commands write each lock through it and make no other Telegram call", func(t *testing.T) {
		chatID := lockApplyChat(t)
		writes := lockCountingWrite(t)
		client := newModuleBotClient()
		bot := newModuleTestBot(client)
		chat := gotgbot.Chat{Id: chatID, Type: "supergroup", Title: "Lock Chat"}
		admin := gotgbot.User{Id: 777000, FirstName: "Telegram"}

		if err := locksModule.lockPerm(bot, newModuleMessageContext(bot, chat, admin, "/lock url sticker")); err != ext.EndGroups {
			t.Fatalf("lockPerm error = %v, want EndGroups", err)
		}
		if err := locksModule.unlockPerm(bot, newModuleMessageContext(bot, chat, admin, "/unlock sticker")); err != ext.EndGroups {
			t.Fatalf("unlockPerm error = %v, want EndGroups", err)
		}
		if want := []string{"url", "sticker", "sticker"}; !slices.Equal(*writes, want) {
			t.Fatalf("writes = %v, want %v", *writes, want)
		}
		if got := lockGoldenStored(t, chatID); got != "sticker=false,url=true" {
			t.Fatalf("stored rows = %q, want sticker unlocked and url locked", got)
		}
		for _, call := range client.calls {
			switch call.Method {
			case "sendMessage", "getChatMember", "getChat", "getMe", "getChatAdministrators":
			default:
				t.Errorf("a lock command made the Telegram call %s", call.Method)
			}
		}
	})
}
