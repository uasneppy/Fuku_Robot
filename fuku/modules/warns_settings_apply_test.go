//go:build testtools

package modules

import (
	"context"
	"errors"
	"testing"

	"github.com/uasneppy/fuku_robot/fuku/db"
	dbcache "github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/db/warns"
)

// warnApplyFreshChat returns a chat ID with no warn settings row and removes whatever the
// test writes for it, the row and its cache key.
func warnApplyFreshChat(t *testing.T) int64 {
	t.Helper()
	chatID := uniqueModuleChatID()
	clean := func() {
		db.DB.Where("chat_id = ?", chatID).Delete(&models.WarnSettings{})
		dbcache.DeleteCache(dbcache.CacheKey("warn_settings", chatID))
		dbcache.ResetLocalForTest()
	}
	clean()
	t.Cleanup(clean)
	return chatID
}

// warnApplyRows counts the chat's warn settings rows straight from the database.
func warnApplyRows(t *testing.T, chatID int64) int64 {
	t.Helper()
	var n int64
	if err := db.DB.Model(&models.WarnSettings{}).Where("chat_id = ?", chatID).Count(&n).Error; err != nil {
		t.Fatalf("count query failed: %v", err)
	}
	return n
}

// warnApplyFailingWrites makes both warn setting writes fail until the test ends.
func warnApplyFailingWrites(t *testing.T) {
	t.Helper()
	previous := warnSettingsWrites
	warnSettingsWrites = warnSettingsWriter{
		Limit: func(int64, int) error { return errors.New("database is down") },
		Mode:  func(int64, string) error { return errors.New("database is down") },
	}
	t.Cleanup(func() { warnSettingsWrites = previous })
}

func TestApplyWarnSettings(t *testing.T) {
	ctx := context.Background()

	t.Run("a limit out of range writes and reads nothing", func(t *testing.T) {
		for _, limit := range []int{0, 101, -5} {
			chatID := warnApplyFreshChat(t)
			got := applyWarnLimit(ctx, chatID, limit)
			if got.Outcome != warnSettingsOutOfRange || got.Key != "warns_limit_range_error" || got.Stored != nil || got.Err != nil {
				t.Fatalf("applyWarnLimit(%d) = %+v, want warnSettingsOutOfRange with no row", limit, got)
			}
			if n := warnApplyRows(t, chatID); n != 0 {
				t.Fatalf("applyWarnLimit(%d) left %d rows, want none", limit, n)
			}
		}
	})

	t.Run("a limit in range is stored and read back", func(t *testing.T) {
		for _, limit := range []int{1, 100, 7} {
			chatID := warnApplyFreshChat(t)
			got := applyWarnLimit(ctx, chatID, limit)
			if got.Outcome != warnSettingsApplied || got.Key != "warns_limit_updated" || got.Err != nil {
				t.Fatalf("applyWarnLimit(%d) = %+v, want warnSettingsApplied", limit, got)
			}
			if got.Stored == nil || got.Stored.WarnLimit != limit || got.Stored.WarnMode != "mute" {
				t.Fatalf("applyWarnLimit(%d) stored = %+v, want the row with that limit and mode mute", limit, got.Stored)
			}
		}
	})

	t.Run("a failed limit write is a save failure with no read-back", func(t *testing.T) {
		chatID := warnApplyFreshChat(t)
		warnApplyFailingWrites(t)
		got := applyWarnLimit(ctx, chatID, 7)
		if got.Outcome != warnSettingsSaveFailed || got.Key != "common_settings_save_failed" || got.Stored != nil || got.Err == nil {
			t.Fatalf("applyWarnLimit(7) = %+v, want warnSettingsSaveFailed with the error and no row", got)
		}
	})

	t.Run("the accepted modes are stored and read back", func(t *testing.T) {
		for _, mode := range []string{"ban", "kick", "mute"} {
			chatID := warnApplyFreshChat(t)
			got := applyWarnMode(ctx, chatID, mode)
			if got.Outcome != warnSettingsApplied || got.Key != "warns_mode_updated_"+mode || got.Err != nil {
				t.Fatalf("applyWarnMode(%q) = %+v, want warnSettingsApplied", mode, got)
			}
			if got.Stored == nil || got.Stored.WarnMode != mode || got.Stored.WarnLimit != 3 {
				t.Fatalf("applyWarnMode(%q) stored = %+v, want the row with that mode and limit 3", mode, got.Stored)
			}
		}
	})

	t.Run("every other word is unknown and writes nothing", func(t *testing.T) {
		for _, mode := range []string{"BAN", "tban", "tmute", "", "warn"} {
			chatID := warnApplyFreshChat(t)
			got := applyWarnMode(ctx, chatID, mode)
			if got.Outcome != warnSettingsUnknownMode || got.Key != "warns_mode_unknown" || got.Stored != nil || got.Err != nil {
				t.Fatalf("applyWarnMode(%q) = %+v, want warnSettingsUnknownMode", mode, got)
			}
			if n := warnApplyRows(t, chatID); n != 0 {
				t.Fatalf("applyWarnMode(%q) left %d rows, want none", mode, n)
			}
		}
	})

	t.Run("a failed mode write is a save failure", func(t *testing.T) {
		chatID := warnApplyFreshChat(t)
		warnApplyFailingWrites(t)
		got := applyWarnMode(ctx, chatID, "ban")
		if got.Outcome != warnSettingsSaveFailed || got.Key != "common_settings_save_failed" || got.Stored != nil || got.Err == nil {
			t.Fatalf("applyWarnMode(ban) = %+v, want warnSettingsSaveFailed with the error and no row", got)
		}
	})

	t.Run("a write drops the warn_settings key and a failed one leaves it", func(t *testing.T) {
		withMiniredisServer(t)
		chatID := warnApplyFreshChat(t)
		if err := warns.SetWarnLimit(chatID, 4); err != nil {
			t.Fatalf("setup SetWarnLimit() error = %v", err)
		}
		// Cache the row, then write through the apply functions: the command's own cached
		// read must see the new values, so the repository's drop ran.
		if got := warns.GetWarnSettingContext(ctx, chatID); got.WarnLimit != 4 {
			t.Fatalf("setup read = %+v, want limit 4", got)
		}
		if got := applyWarnLimit(ctx, chatID, 8); got.Outcome != warnSettingsApplied {
			t.Fatalf("applyWarnLimit(8) = %+v", got)
		}
		if got := warns.GetWarnSettingContext(ctx, chatID); got.WarnLimit != 8 {
			t.Fatalf("cached read after applyWarnLimit = %+v, want limit 8", got)
		}
		if got := applyWarnMode(ctx, chatID, "kick"); got.Outcome != warnSettingsApplied {
			t.Fatalf("applyWarnMode(kick) = %+v", got)
		}
		if got := warns.GetWarnSettingContext(ctx, chatID); got.WarnMode != "kick" {
			t.Fatalf("cached read after applyWarnMode = %+v, want mode kick", got)
		}

		warnApplyFailingWrites(t)
		if got := applyWarnLimit(ctx, chatID, 50); got.Outcome != warnSettingsSaveFailed {
			t.Fatalf("failing applyWarnLimit(50) = %+v", got)
		}
		if got := warns.GetWarnSettingContext(ctx, chatID); got.WarnLimit != 8 {
			t.Fatalf("cached read after a failed write = %+v, want the unchanged limit 8", got)
		}
	})
}
