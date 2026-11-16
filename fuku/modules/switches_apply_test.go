package modules

import (
	"context"
	"errors"
	"testing"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/greetings"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/db/reports"
)

// reportsApplyChat mints a chat for an apply test and removes its report and chats rows before
// and after, because CI runs all of fuku/modules in one database.
func reportsApplyChat(t *testing.T) int64 {
	t.Helper()
	chatID := uniqueModuleChatID()
	clean := func() {
		db.DB.Where("chat_id = ?", chatID).Delete(&models.ReportChatSettings{})
		db.DB.Where("chat_id = ?", chatID).Delete(&models.Chat{})
	}
	clean()
	t.Cleanup(clean)
	return chatID
}

func TestApplyReportsEnabled(t *testing.T) {
	t.Run("on and off store enabled and status together", func(t *testing.T) {
		chatID := reportsApplyChat(t)
		for _, tc := range []struct {
			on  bool
			key string
		}{
			{false, "reports_turned_off_group"},
			{true, "reports_turned_on_group"},
		} {
			res := applyReportsEnabled(context.Background(), chatID, tc.on)
			if res.Outcome != reportsApplied || res.Err != nil || res.Key != tc.key || res.Status != tc.on {
				t.Fatalf("applyReportsEnabled(%v) = %+v, want applied with key %s and status %v", tc.on, res, tc.key, tc.on)
			}
			var row models.ReportChatSettings
			if err := db.DB.Where("chat_id = ?", chatID).First(&row).Error; err != nil {
				t.Fatalf("read row: %v", err)
			}
			if row.Enabled != tc.on || row.Status != tc.on {
				t.Fatalf("stored enabled %v status %v, want both %v", row.Enabled, row.Status, tc.on)
			}
		}
	})

	t.Run("a group with no row gets the row the write creates", func(t *testing.T) {
		chatID := reportsApplyChat(t)
		if res := applyReportsEnabled(context.Background(), chatID, false); res.Outcome != reportsApplied {
			t.Fatalf("outcome = %v, want applied", res.Outcome)
		}
		on, err := reports.LoadChatStatusContext(context.Background(), chatID)
		if err != nil || on {
			t.Fatalf("status after off = %v, %v; want false, nil", on, err)
		}
	})

	t.Run("a failing write is a save failure with the error", func(t *testing.T) {
		chatID := reportsApplyChat(t)
		boom := errors.New("database is down")
		saved := reportsWrites
		t.Cleanup(func() { reportsWrites = saved })
		reportsWrites.ChatStatus = func(int64, bool) error { return boom }
		res := applyReportsEnabled(context.Background(), chatID, false)
		if res.Outcome != reportsSaveFailed || !errors.Is(res.Err, boom) || res.Key != "common_settings_save_failed" {
			t.Fatalf("applyReportsEnabled() = %+v, want a save failure carrying the write error", res)
		}
	})
}

func TestApplyAutoApprove(t *testing.T) {
	t.Run("on and off write and read back", func(t *testing.T) {
		chatID := uniqueModuleChatID()
		legacyCleanup(t, chatID)
		for _, on := range []bool{true, false, true} {
			res := applyAutoApprove(context.Background(), chatID, on)
			if res.Outcome != greetingApplied || res.Err != nil || res.Settings == nil || res.Settings.ShouldAutoApprove != on {
				t.Fatalf("applyAutoApprove(%v) = %+v, want applied with the value read back", on, res)
			}
			stored, err := greetings.LoadSettingsContext(context.Background(), chatID)
			if err != nil || stored.ShouldAutoApprove != on {
				t.Fatalf("stored auto_approve = %v, %v; want %v", stored, err, on)
			}
		}
	})

	t.Run("the write drops the cached greetings so the next cached read sees it", func(t *testing.T) {
		chatID := uniqueModuleChatID()
		legacyCleanup(t, chatID)
		if res := applyAutoApprove(context.Background(), chatID, false); res.Outcome != greetingApplied {
			t.Fatalf("seed outcome = %v", res.Outcome)
		}
		if greetings.GetGreetingSettings(chatID).ShouldAutoApprove {
			t.Fatal("cached read before the write is already on")
		}
		if res := applyAutoApprove(context.Background(), chatID, true); res.Outcome != greetingApplied {
			t.Fatalf("outcome = %v", res.Outcome)
		}
		if !greetings.GetGreetingSettings(chatID).ShouldAutoApprove {
			t.Fatal("the cached read still shows the old value after the write")
		}
	})

	t.Run("a failing write is a save failure with the error", func(t *testing.T) {
		chatID := uniqueModuleChatID()
		legacyCleanup(t, chatID)
		boom := errors.New("database is down")
		saved := greetingWrites
		t.Cleanup(func() { greetingWrites = saved })
		greetingWrites.AutoApprove = func(int64, bool) error { return boom }
		res := applyAutoApprove(context.Background(), chatID, true)
		if res.Outcome != greetingSaveFailed || !errors.Is(res.Err, boom) || res.Settings != nil {
			t.Fatalf("applyAutoApprove() = %+v, want a save failure carrying the write error", res)
		}
	})
}
