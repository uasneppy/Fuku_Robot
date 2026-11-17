//go:build testtools

package modules

import (
	"fmt"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db"
	dbcache "github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/captcha"
	"github.com/uasneppy/fuku_robot/fuku/utils/cache"
)

// TestCaptchaRefreshSchedulesNewTimeout proves that a successful refresh leaves a live timer
// (audit item 31): the original timer sees the bumped refresh count and exits as stale, so the
// refresh must schedule a replacement, and the failure action then runs when the attempt ends
// (here about 1.5 s after the refresh) instead of at the next 5-minute cleanup tick.
func TestCaptchaRefreshSchedulesNewTimeout(t *testing.T) {
	withCaptchaLifecycleForTest(t)
	client := newModuleBotClient()
	bot := newModuleTestBot(client)
	chat := gotgbot.Chat{Id: uniqueModuleChatID(), Type: "supergroup", Title: "Captcha Chat"}
	member := gotgbot.User{Id: uniqueLinkOwnerID(), FirstName: "Member"}
	selfDestructCaptchaCleanup(t, chat.Id)
	memberReqCleanup(t, nil, []int64{member.Id})
	cooldownKey := fmt.Sprintf("fuku:captcha:refresh:cooldown:%d:%d", chat.Id, member.Id)
	t.Cleanup(func() {
		if m := cache.GetMarshal(); m != nil {
			_ = m.Delete(cache.Context, cooldownKey)
		}
	})

	if err := captcha.SetCaptchaEnabled(chat.Id, true); err != nil {
		t.Fatalf("SetCaptchaEnabled() error = %v", err)
	}
	if err := captcha.SetCaptchaMode(chat.Id, "text"); err != nil {
		t.Fatalf("SetCaptchaMode() error = %v", err)
	}
	attempt, err := captcha.CreateCaptchaAttemptPreMessage(member.Id, chat.Id, "old-answer", 2)
	if err != nil {
		t.Fatalf("CreateCaptchaAttemptPreMessage() error = %v", err)
	}
	if err := captcha.UpdateCaptchaAttemptMessageID(attempt.ID, 123); err != nil {
		t.Fatalf("UpdateCaptchaAttemptMessageID() error = %v", err)
	}
	expires := time.Now().Add(1500 * time.Millisecond).UTC()
	if err := db.DB.Model(&db.CaptchaAttempts{}).Where("id = ?", attempt.ID).Update("expires_at", expires).Error; err != nil {
		t.Fatalf("shorten the attempt: %v", err)
	}
	dbcache.DeleteCache(dbcache.CacheKey("captcha_pending", chat.Id))
	original, err := captcha.GetCaptchaAttemptByID(attempt.ID)
	if err != nil || original == nil {
		t.Fatalf("GetCaptchaAttemptByID() = %#v, %v", original, err)
	}
	// The timer the join scheduled, as production does.
	scheduleCaptchaTimeout(bot, original)

	data := encodeCallbackData("captcha_refresh", map[string]string{"a": fmt.Sprint(attempt.ID), "u": fmt.Sprint(member.Id)})
	if err := captchaModule.captchaRefreshCallback(bot, newModuleCallbackContext(bot, chat, member, data)); err != nil {
		t.Fatalf("captchaRefreshCallback() error = %v", err)
	}
	refreshed, err := captcha.GetCaptchaAttemptByID(attempt.ID)
	if err != nil || refreshed == nil || refreshed.RefreshCount != 1 {
		t.Fatalf("attempt after refresh = %#v, %v; want refresh count 1", refreshed, err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for {
		row, err := captcha.GetCaptchaAttemptByID(attempt.ID)
		if err == nil && row == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the attempt was still pending 3.5 s after it ended (row %#v, err %v): no timer ran", row, err)
		}
		time.Sleep(50 * time.Millisecond)
	}

	deletedRefreshed := false
	for _, call := range client.callsFor("deleteMessage") {
		if staffParamInt(call.Params, "message_id") == refreshed.MessageID {
			deletedRefreshed = true
		}
	}
	if !deletedRefreshed {
		t.Errorf("the refreshed challenge message %d was not deleted by the timeout", refreshed.MessageID)
	}
}
