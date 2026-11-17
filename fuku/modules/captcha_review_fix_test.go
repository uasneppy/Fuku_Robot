//go:build testtools

package modules

import (
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db"
	dbcache "github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/captcha"
)

// waitCaptchaAttemptGone waits up to limit for the attempt row to be deleted and reports
// whether it was.
func waitCaptchaAttemptGone(t *testing.T, id uint, limit time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(limit)
	for {
		row, err := captcha.GetCaptchaAttemptByID(id)
		if err == nil && row == nil {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// TestCaptchaTimerIgnoresMicrosecondRounding pins C1: PostgreSQL keeps expires_at to the
// microsecond, so the attempt read back by the timer differs from the in-memory one by its
// nanoseconds. The timer must still see the same attempt and apply the group's action.
func TestCaptchaTimerIgnoresMicrosecondRounding(t *testing.T) {
	withCaptchaLifecycleForTest(t)
	client := newModuleBotClient()
	bot := newModuleTestBot(client)
	chat := gotgbot.Chat{Id: uniqueModuleChatID(), Type: "supergroup", Title: "Captcha Chat"}
	member := gotgbot.User{Id: uniqueLinkOwnerID(), FirstName: "Member"}
	selfDestructCaptchaCleanup(t, chat.Id)
	memberReqCleanup(t, nil, []int64{member.Id})

	if err := captcha.SetCaptchaEnabled(chat.Id, true); err != nil {
		t.Fatalf("SetCaptchaEnabled() error = %v", err)
	}
	attempt, err := captcha.CreateCaptchaAttemptPreMessage(member.Id, chat.Id, "7", 2)
	if err != nil {
		t.Fatalf("CreateCaptchaAttemptPreMessage() error = %v", err)
	}
	if err := captcha.UpdateCaptchaAttemptMessageID(attempt.ID, 123); err != nil {
		t.Fatalf("UpdateCaptchaAttemptMessageID() error = %v", err)
	}
	attempt.MessageID = 123

	// What PostgreSQL stores: the microsecond-rounded time. The timer holds the in-memory value,
	// which carries nanoseconds.
	stored := time.Now().Add(800 * time.Millisecond).UTC().Truncate(time.Microsecond)
	if err := db.DB.Model(&db.CaptchaAttempts{}).Where("id = ?", attempt.ID).Update("expires_at", stored).Error; err != nil {
		t.Fatalf("store the rounded expiry: %v", err)
	}
	dbcache.DeleteCache(dbcache.CacheKey("captcha_pending", chat.Id))
	attempt.ExpiresAt = stored.Add(437 * time.Nanosecond)

	scheduleCaptchaTimeout(bot, attempt)

	if !waitCaptchaAttemptGone(t, attempt.ID, 5*time.Second) {
		t.Fatal("the timer treated the attempt as stale and left it pending")
	}
	if got := kicksOf(client, member.Id); got != 1 {
		t.Fatalf("kicks = %d, want the group's action applied once", got)
	}
}
