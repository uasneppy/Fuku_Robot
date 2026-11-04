//go:build testtools

package menuaccess

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"testing"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// uniqueAccessChatID returns a random large negative chat ID, so tests sharing one
// database never collide.
func uniqueAccessChatID(t *testing.T) int64 {
	t.Helper()
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		t.Fatalf("read random bytes: %v", err)
	}
	return -1000000000000 - int64(binary.BigEndian.Uint64(buf[:])&0x3fffffffffffffff)
}

// cleanupAccessRow deletes the chat's row when the test ends, so the test also runs against a
// shared PostgreSQL database.
func cleanupAccessRow(t *testing.T, chatID int64) {
	t.Helper()
	t.Cleanup(func() {
		if err := db.DB.Where("chat_id = ?", chatID).Delete(&models.ChatSettingsAccess{}).Error; err != nil {
			t.Errorf("delete the settings access row of chat %d: %v", chatID, err)
		}
	})
}

// TestMenuAccessUpsert proves the switch contract: no row is owner only, a Go false and a Go
// true are both stored faithfully, there is one row per chat, and the last writer wins.
func TestMenuAccessUpsert(t *testing.T) {
	ctx := context.Background()
	chatID := uniqueAccessChatID(t)
	cleanupAccessRow(t, chatID)

	allowed, err := AdminsAllowedFresh(ctx, chatID)
	if err != nil || allowed {
		t.Fatalf("a chat with no row: got (%v, %v), want (false, nil)", allowed, err)
	}

	// A first write of false stores a row that reads false (the zero value is written).
	if err := SetAdminsAllowed(ctx, chatID, false, 5); err != nil {
		t.Fatalf("SetAdminsAllowed(false): %v", err)
	}
	var stored models.ChatSettingsAccess
	if err := db.DB.Where("chat_id = ?", chatID).First(&stored).Error; err != nil {
		t.Fatalf("read the stored row: %v", err)
	}
	if stored.AdminsAllowed || stored.UpdatedBy != 5 {
		t.Fatalf("stored row after the first false write = %+v, want admins_allowed false, updated_by 5", stored)
	}

	if err := SetAdminsAllowed(ctx, chatID, true, 11); err != nil {
		t.Fatalf("SetAdminsAllowed(true): %v", err)
	}
	allowed, err = AdminsAllowedFresh(ctx, chatID)
	if err != nil || !allowed {
		t.Fatalf("after switching on: got (%v, %v), want (true, nil)", allowed, err)
	}

	if err := SetAdminsAllowed(ctx, chatID, false, 22); err != nil {
		t.Fatalf("SetAdminsAllowed(false) second time: %v", err)
	}
	allowed, err = AdminsAllowedFresh(ctx, chatID)
	if err != nil || allowed {
		t.Fatalf("after switching off: got (%v, %v), want (false, nil)", allowed, err)
	}

	var rows []models.ChatSettingsAccess
	if err := db.DB.Where("chat_id = ?", chatID).Find(&rows).Error; err != nil {
		t.Fatalf("list the rows of the chat: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows for the chat = %d, want exactly 1", len(rows))
	}
	if rows[0].UpdatedBy != 22 || rows[0].AdminsAllowed {
		t.Fatalf("row = %+v, want updated_by 22 (last writer) and admins_allowed false", rows[0])
	}
}

// TestMenuAccessReadError proves a read failure is an error and never a value: with an
// already cancelled context the switch is unreadable, so a caller can refuse.
func TestMenuAccessReadError(t *testing.T) {
	chatID := uniqueAccessChatID(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	allowed, err := AdminsAllowedFresh(ctx, chatID)
	if err == nil {
		t.Fatalf("AdminsAllowedFresh with a cancelled context returned (%v, nil), want an error", allowed)
	}
	if allowed {
		t.Fatalf("AdminsAllowedFresh returned true together with an error")
	}
}
