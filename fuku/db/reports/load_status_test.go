package reports

import (
	"context"
	"testing"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

func TestLoadChatStatusContext(t *testing.T) {
	skipIfNoDb(t)

	cleanup := func(chatID int64) {
		t.Cleanup(func() {
			_ = db.DB.Where("chat_id = ?", chatID).Delete(&models.ReportChatSettings{}).Error
			_ = db.DB.Where("chat_id = ?", chatID).Delete(&models.Chat{}).Error
		})
	}
	count := func(model any, chatID int64) int64 {
		var n int64
		if err := db.DB.Model(model).Where("chat_id = ?", chatID).Count(&n).Error; err != nil {
			t.Fatalf("count: %v", err)
		}
		return n
	}

	t.Run("no row reads on and creates nothing", func(t *testing.T) {
		const chatID int64 = -1009918001401
		cleanup(chatID)
		on, err := LoadChatStatusContext(context.Background(), chatID)
		if err != nil || !on {
			t.Fatalf("LoadChatStatusContext() = %v, %v; want true, nil", on, err)
		}
		if n := count(&models.ReportChatSettings{}, chatID); n != 0 {
			t.Fatalf("report rows = %d, want none created", n)
		}
		if n := count(&models.Chat{}, chatID); n != 0 {
			t.Fatalf("chats rows = %d, want none created", n)
		}
	})

	t.Run("stored off reads off and stored on reads on", func(t *testing.T) {
		const chatID int64 = -1009918001402
		cleanup(chatID)
		if err := SetChatReportStatus(chatID, false); err != nil {
			t.Fatalf("SetChatReportStatus(false): %v", err)
		}
		on, err := LoadChatStatusContext(context.Background(), chatID)
		if err != nil || on {
			t.Fatalf("after off: %v, %v; want false, nil", on, err)
		}
		if err := SetChatReportStatus(chatID, true); err != nil {
			t.Fatalf("SetChatReportStatus(true): %v", err)
		}
		on, err = LoadChatStatusContext(context.Background(), chatID)
		if err != nil || !on {
			t.Fatalf("after on: %v, %v; want true, nil", on, err)
		}
	})

	t.Run("a read error is returned", func(t *testing.T) {
		const chatID int64 = -1009918001403
		cleanup(chatID)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := LoadChatStatusContext(ctx, chatID); err == nil {
			t.Fatal("LoadChatStatusContext() with a cancelled context returned a nil error")
		}
	})
}
