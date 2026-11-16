//go:build testtools

package modules

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/uasneppy/fuku_robot/fuku/db"
	dbcache "github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/chats"
	"github.com/uasneppy/fuku_robot/fuku/db/logchannels"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// logApplyChat mints a chat and a log channel for an apply test, optionally links them, and
// removes their log_channels, chats and cache rows before and after, because CI runs all of
// fuku/modules in one database.
func logApplyChat(t *testing.T, linked bool) (chatID, channelID int64) {
	t.Helper()
	chatID = uniqueModuleChatID()
	channelID = uniqueModuleChatID()
	clean := func() {
		db.DB.Where("chat_id IN ?", []int64{chatID, channelID}).Delete(&models.LogChannel{})
		db.DB.Where("chat_id IN ?", []int64{chatID, channelID}).Delete(&models.Chat{})
		dbcache.DeleteCache(dbcache.CacheKey("log_channel", chatID))
		dbcache.ResetLocalForTest()
	}
	clean()
	t.Cleanup(clean)
	if err := chats.EnsureChatInDb(chatID, "log apply"); err != nil {
		t.Fatalf("EnsureChatInDb(chat): %v", err)
	}
	if err := chats.EnsureChatInDb(channelID, "log apply channel"); err != nil {
		t.Fatalf("EnsureChatInDb(channel): %v", err)
	}
	if linked {
		if err := logchannels.Set(chatID, "log apply", channelID); err != nil {
			t.Fatalf("logchannels.Set: %v", err)
		}
	}
	return chatID, channelID
}

// logApplyStoredFlags is the chat's stored category flags by name, read straight from the table.
func logApplyStoredFlags(t *testing.T, chatID int64) map[string]bool {
	t.Helper()
	var row models.LogChannel
	if err := db.DB.Where("chat_id = ?", chatID).First(&row).Error; err != nil {
		t.Fatalf("read log_channels row: %v", err)
	}
	flags := make(map[string]bool, len(logchannels.AllCategories))
	for _, name := range logchannels.AllCategories {
		flags[name] = logchannels.CategoryEnabled(&row, name)
	}
	return flags
}

// logApplyPrime replaces whatever the log_channel:<chat> key holds with a stale marker row.
func logApplyPrime(t *testing.T, chatID int64) {
	t.Helper()
	key := dbcache.CacheKey("log_channel", chatID)
	dbcache.DeleteCache(key)
	got, err := dbcache.GetFromCacheOrLoad(context.Background(), key, time.Minute, func(context.Context) (models.LogChannel, error) {
		return models.LogChannel{ChatID: chatID, LogChannelID: -1}, nil
	})
	if err != nil || got.LogChannelID != -1 {
		t.Fatalf("prime %s = %+v, %v", key, got, err)
	}
}

// logApplyKeyDropped reports whether the stale marker is gone. The write drops the key and the
// read-back may already have filled it with the real row, so only the marker itself is judged.
func logApplyKeyDropped(t *testing.T, chatID int64) bool {
	t.Helper()
	got, err := dbcache.GetFromCacheOrLoad(context.Background(), dbcache.CacheKey("log_channel", chatID), time.Minute, func(context.Context) (models.LogChannel, error) {
		return models.LogChannel{ChatID: chatID, LogChannelID: -2}, nil
	})
	if err != nil {
		t.Fatalf("probe log_channel key: %v", err)
	}
	return got.LogChannelID != -1
}

// logApplyCountWrites wraps the write seam so a test can count the writes that reached it.
func logApplyCountWrites(t *testing.T) *int {
	t.Helper()
	count := new(int)
	saved := logCategoryWrites
	t.Cleanup(func() { logCategoryWrites = saved })
	logCategoryWrites = func(chatID int64, name string, enabled bool) error {
		*count++
		return saved(chatID, name, enabled)
	}
	return count
}

// TestApplyLogCategory proves the one-category write the commands and the Log channel page share:
// each category turned off and on changes that flag only and drops the commands' cache key, a
// mixed-case name is valid exactly as the repository treats it, an unknown name, a chat with no
// channel and a failing write are typed results, and a failed read-back never undoes a write.
func TestApplyLogCategory(t *testing.T) {
	ctx := context.Background()

	t.Run("each category changes that flag only and drops the cache key", func(t *testing.T) {
		chatID, _ := logApplyChat(t, true)
		// A baseline with two categories already off, so a no-op press is told from a change.
		for _, name := range []string{logchannels.CategoryUser, logchannels.CategoryOther} {
			if err := logchannels.SetCategory(chatID, name, false); err != nil {
				t.Fatalf("seed %s off: %v", name, err)
			}
		}
		for _, name := range logchannels.AllCategories {
			for _, on := range []bool{false, true} {
				before := logApplyStoredFlags(t, chatID)
				logApplyPrime(t, chatID)

				res := applyLogCategory(ctx, chatID, name, on)
				wantKey := "logs_disabled"
				if on {
					wantKey = "logs_enabled"
				}
				if res.Outcome != logApplied || res.Err != nil || res.Key != wantKey || res.Row == nil {
					t.Fatalf("applyLogCategory(%s, %v) = %+v, want applied with key %s and the row", name, on, res, wantKey)
				}
				if !logApplyKeyDropped(t, chatID) {
					t.Fatalf("applyLogCategory(%s, %v) left the log_channel key cached", name, on)
				}

				after := logApplyStoredFlags(t, chatID)
				for _, other := range logchannels.AllCategories {
					want := before[other]
					if other == name {
						want = on
					}
					if after[other] != want {
						t.Fatalf("applyLogCategory(%s, %v): stored %s = %v, want %v", name, on, other, after[other], want)
					}
					if got := logchannels.CategoryEnabled(res.Row, other); got != want {
						t.Fatalf("applyLogCategory(%s, %v): read-back %s = %v, want %v", name, on, other, got, want)
					}
				}
			}
		}
	})

	t.Run("a mixed-case name is valid and turns that flag", func(t *testing.T) {
		chatID, _ := logApplyChat(t, true)
		for _, spelled := range []string{"Settings", "  ADMIN "} {
			res := applyLogCategory(ctx, chatID, spelled, false)
			if res.Outcome != logApplied || res.Err != nil {
				t.Fatalf("applyLogCategory(%q) = %+v, want applied", spelled, res)
			}
		}
		flags := logApplyStoredFlags(t, chatID)
		if flags[logchannels.CategorySettings] || flags[logchannels.CategoryAdmin] || !flags[logchannels.CategoryUser] {
			t.Fatalf("stored flags = %v, want settings and admin off and user still on", flags)
		}
	})

	t.Run("an unknown name is refused with no write", func(t *testing.T) {
		chatID, _ := logApplyChat(t, true)
		writes := logApplyCountWrites(t)
		before := logApplyStoredFlags(t, chatID)
		for _, name := range []string{"bogus", "", "admin,user"} {
			res := applyLogCategory(ctx, chatID, name, false)
			if res.Outcome != logUnknownCategory || res.Key != "logs_unknown_category" || res.Row != nil {
				t.Fatalf("applyLogCategory(%q) = %+v, want an unknown category", name, res)
			}
		}
		if *writes != 0 {
			t.Fatalf("write seam called %d times for unknown names, want 0", *writes)
		}
		after := logApplyStoredFlags(t, chatID)
		for name, want := range before {
			if after[name] != want {
				t.Fatalf("flag %s changed to %v after refused names", name, after[name])
			}
		}
	})

	t.Run("a chat with no linked channel is not set and creates no row", func(t *testing.T) {
		chatID, _ := logApplyChat(t, false)
		res := applyLogCategory(ctx, chatID, logchannels.CategoryAdmin, false)
		if res.Outcome != logNotSet || res.Key != "logs_not_set" || res.Row != nil {
			t.Fatalf("applyLogCategory() = %+v, want not set", res)
		}
		var n int64
		if err := db.DB.Model(&models.LogChannel{}).Where("chat_id = ?", chatID).Count(&n).Error; err != nil {
			t.Fatalf("count log_channels rows: %v", err)
		}
		if n != 0 {
			t.Fatalf("log_channels rows = %d, want none created", n)
		}
	})

	t.Run("a failing write is a save failure with the error", func(t *testing.T) {
		chatID, _ := logApplyChat(t, true)
		boom := errors.New("database is down")
		saved := logCategoryWrites
		t.Cleanup(func() { logCategoryWrites = saved })
		logCategoryWrites = func(int64, string, bool) error { return boom }
		res := applyLogCategory(ctx, chatID, logchannels.CategoryAdmin, false)
		if res.Outcome != logSaveFailed || !errors.Is(res.Err, boom) || res.Key != "common_settings_save_failed" || res.Row != nil {
			t.Fatalf("applyLogCategory() = %+v, want a save failure carrying the write error", res)
		}
		if !logApplyStoredFlags(t, chatID)[logchannels.CategoryAdmin] {
			t.Fatal("admin flag changed although the write failed")
		}
	})

	t.Run("a failed read-back is still an applied write", func(t *testing.T) {
		chatID, _ := logApplyChat(t, true)
		cancelled, cancel := context.WithCancel(ctx)
		cancel()
		res := applyLogCategory(cancelled, chatID, logchannels.CategoryReports, false)
		if res.Outcome != logApplied || res.Row != nil || res.Err == nil {
			t.Fatalf("applyLogCategory() = %+v, want applied with no row and the read-back error", res)
		}
		if logApplyStoredFlags(t, chatID)[logchannels.CategoryReports] {
			t.Fatal("reports flag still on although the write was applied")
		}
	})
}
