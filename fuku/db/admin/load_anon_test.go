//go:build testtools

package admin

import (
	"context"
	"errors"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

func adminRowCount(t *testing.T, chatID int64) int64 {
	t.Helper()
	var n int64
	if err := db.DB.Model(&models.AdminSettings{}).Where("chat_id = ?", chatID).Count(&n).Error; err != nil {
		t.Fatalf("count admin rows: %v", err)
	}
	return n
}

func TestLoadAnonAdminContext(t *testing.T) {
	skipIfNoDb(t)
	chatID := time.Now().UnixNano() + 9100
	t.Cleanup(func() {
		db.DB.Where("chat_id = ?", chatID).Delete(&models.AdminSettings{})
	})
	ctx := context.Background()

	t.Run("a missing row reads off and creates no row", func(t *testing.T) {
		on, err := LoadAnonAdminContext(ctx, chatID)
		if err != nil {
			t.Fatalf("LoadAnonAdminContext() error = %v", err)
		}
		if on {
			t.Error("a chat with no row read as on")
		}
		if n := adminRowCount(t, chatID); n != 0 {
			t.Errorf("admin rows after the read = %d, want 0", n)
		}
	})

	t.Run("a stored mode is read and follows a write", func(t *testing.T) {
		if err := SetAnonAdminMode(chatID, true); err != nil {
			t.Fatalf("SetAnonAdminMode(true) error = %v", err)
		}
		if on, err := LoadAnonAdminContext(ctx, chatID); err != nil || !on {
			t.Fatalf("after on: LoadAnonAdminContext() = %v, %v; want true, nil", on, err)
		}
		if err := SetAnonAdminMode(chatID, false); err != nil {
			t.Fatalf("SetAnonAdminMode(false) error = %v", err)
		}
		if on, err := LoadAnonAdminContext(ctx, chatID); err != nil || on {
			t.Fatalf("after off: LoadAnonAdminContext() = %v, %v; want false, nil", on, err)
		}
	})

	t.Run("a read error is returned, not hidden as off", func(t *testing.T) {
		name := "test:fail_admin_query_" + t.Name()
		err := db.DB.Callback().Query().Before("gorm:query").Register(name, func(tx *gorm.DB) {
			if tx.Statement.Table == "admin" {
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
		if _, err := LoadAnonAdminContext(ctx, chatID); err == nil {
			t.Error("a failing read returned no error")
		}
	})
}
