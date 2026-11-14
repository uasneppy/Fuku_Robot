//go:build testtools

package modules

import (
	"context"
	"errors"
	"testing"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/admin"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// anonApplyChat mints a chat for an apply test and removes its admin row before and after,
// because CI runs all of fuku/modules in one database.
func anonApplyChat(t *testing.T) int64 {
	t.Helper()
	chatID := uniqueModuleChatID()
	anonGoldenCleanup(t, chatID)
	return chatID
}

// anonCountingWrite swaps the write seam for one that counts its calls and still writes, and
// restores the real one when the test ends.
func anonCountingWrite(t *testing.T) *int {
	t.Helper()
	calls := 0
	saved := anonAdminWrites
	t.Cleanup(func() { anonAdminWrites = saved })
	anonAdminWrites.SetMode = func(chatID int64, on bool) error {
		calls++
		return saved.SetMode(chatID, on)
	}
	return &calls
}

func TestApplyAnonAdmin(t *testing.T) {
	ctx := context.Background()

	t.Run("a change writes and reads back", func(t *testing.T) {
		chatID := anonApplyChat(t)
		calls := anonCountingWrite(t)

		on := applyAnonAdmin(ctx, chatID, false, true)
		if on.Outcome != anonAdminApplied || !on.On || on.Key != "admin_anon_admin_enabled_now" || on.Err != nil {
			t.Fatalf("on = %+v", on)
		}
		if stored, err := admin.LoadAnonAdminContext(ctx, chatID); err != nil || !stored {
			t.Fatalf("stored after on = %v, %v; want true, nil", stored, err)
		}
		off := applyAnonAdmin(ctx, chatID, true, false)
		if off.Outcome != anonAdminApplied || off.On || off.Key != "admin_anon_admin_disabled_now" {
			t.Fatalf("off = %+v", off)
		}
		if stored, err := admin.LoadAnonAdminContext(ctx, chatID); err != nil || stored {
			t.Fatalf("stored after off = %v, %v; want false, nil", stored, err)
		}
		if *calls != 2 {
			t.Errorf("writes = %d, want 2", *calls)
		}
	})

	t.Run("the current value writes nothing", func(t *testing.T) {
		chatID := anonApplyChat(t)
		calls := anonCountingWrite(t)

		off := applyAnonAdmin(ctx, chatID, false, false)
		if off.Outcome != anonAdminAlready || off.On || off.Key != "admin_anon_admin_already_disabled" {
			t.Fatalf("already off = %+v", off)
		}
		on := applyAnonAdmin(ctx, chatID, true, true)
		if on.Outcome != anonAdminAlready || !on.On || on.Key != "admin_anon_admin_already_enabled" {
			t.Fatalf("already on = %+v", on)
		}
		if *calls != 0 {
			t.Errorf("writes = %d, want 0", *calls)
		}
		var rows int64
		if err := db.DB.Model(&models.AdminSettings{}).Where("chat_id = ?", chatID).Count(&rows).Error; err != nil || rows != 0 {
			t.Errorf("admin rows = %d, %v; want none", rows, err)
		}
	})

	t.Run("a failed write is a failed save and keeps the current value", func(t *testing.T) {
		chatID := anonApplyChat(t)
		saved := anonAdminWrites
		t.Cleanup(func() { anonAdminWrites = saved })
		failure := errors.New("database is down")
		anonAdminWrites.SetMode = func(int64, bool) error { return failure }

		result := applyAnonAdmin(ctx, chatID, false, true)
		if result.Outcome != anonAdminSaveFailed || result.On || result.Key != "admin_anon_admin_db_error" || !errors.Is(result.Err, failure) {
			t.Fatalf("result = %+v", result)
		}
	})

	t.Run("a cancelled caller still stores and reads the value", func(t *testing.T) {
		chatID := anonApplyChat(t)
		cancelled, cancel := context.WithCancel(ctx)
		cancel()
		result := applyAnonAdmin(cancelled, chatID, false, true)
		if result.Outcome != anonAdminApplied || !result.On {
			t.Fatalf("result = %+v", result)
		}
	})
}
