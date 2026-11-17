//go:build testtools

package modules

import (
	"encoding/json"
	"fmt"
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

// captchaReviewTimeoutEnv is one fresh chat and member with a pending attempt, for the failed
// action cases of C2.
type captchaReviewTimeoutEnv struct {
	client *moduleBotClient
	bot    *gotgbot.Bot
	chatID int64
	userID int64
	id     uint
}

func newCaptchaReviewTimeoutEnv(t *testing.T) *captchaReviewTimeoutEnv {
	t.Helper()
	client := newModuleBotClient()
	chatID := uniqueModuleChatID()
	userID := uniqueLinkOwnerID()
	client.responses["getChat"] = json.RawMessage(fmt.Sprintf(
		`{"id":%d,"type":"supergroup","title":"Captcha Chat","permissions":{"can_send_messages":true}}`, chatID))
	selfDestructCaptchaCleanup(t, chatID)
	memberReqCleanup(t, nil, []int64{userID})
	t.Cleanup(func() { _ = captcha.DeleteMutedUser(userID, chatID) })
	if err := captcha.SetCaptchaEnabled(chatID, true); err != nil {
		t.Fatalf("SetCaptchaEnabled() error = %v", err)
	}
	attempt, err := captcha.CreateCaptchaAttemptPreMessage(userID, chatID, "7", 2)
	if err != nil {
		t.Fatalf("CreateCaptchaAttemptPreMessage() error = %v", err)
	}
	return &captchaReviewTimeoutEnv{client: client, bot: newModuleTestBot(client), chatID: chatID, userID: userID, id: attempt.ID}
}

// liveStatus makes the fake answer the member's getChatMember with status and no rights.
func (e *captchaReviewTimeoutEnv) liveStatus(status string) {
	e.client.responses["getChatMember"] = json.RawMessage(fmt.Sprintf(
		`{"status":%q,"user":{"id":%d,"is_bot":false,"first_name":"Member"},"is_member":false,"can_send_messages":false,"until_date":0}`,
		status, e.userID))
}

// TestCaptchaTimeoutFailedActionReadsLiveMember pins C2: a failure action Telegram answered
// with an error is judged from the member's live record. Removed is applied; a definite 4xx
// other than 429 with the member still there releases and unmutes them (WR-07) with no
// member-removing unban; any other answer, or an unreadable record, leaves them muted with
// the 24 hour fallback unmute scheduled.
func TestCaptchaTimeoutFailedActionReadsLiveMember(t *testing.T) {
	refused := &gotgbot.TelegramError{Code: 400, Description: "Bad Request: not enough rights to restrict/ban chat member"}
	limited := &gotgbot.TelegramError{Code: 429, Description: "Too Many Requests: retry after 5"}
	broken := &gotgbot.TelegramError{Code: 500, Description: "Internal Server Error"}

	cases := []struct {
		name        string
		action      string
		method      string
		actionErr   error
		live        string // "" keeps the fake's member record
		liveErr     error
		wantApplied bool
		wantUnmute  bool
		wantPending bool // the 24 hour fallback row exists
		wantKicks   int
	}{
		{name: "ban refused, member still there", action: "ban", method: "banChatMember", actionErr: refused, wantUnmute: true},
		{name: "ban rate limited", action: "ban", method: "banChatMember", actionErr: limited, wantPending: true},
		{name: "ban server error", action: "ban", method: "banChatMember", actionErr: broken, wantPending: true},
		{name: "ban server error but member banned", action: "ban", method: "banChatMember", actionErr: broken, live: "kicked", wantApplied: true},
		{name: "ban server error, record unreadable", action: "ban", method: "banChatMember", actionErr: broken, liveErr: broken, wantPending: true},
		{name: "kick refused, member still there", action: "kick", method: "unbanChatMember", actionErr: refused, wantUnmute: true, wantKicks: 1},
		{name: "kick server error but member gone", action: "kick", method: "unbanChatMember", actionErr: broken, live: "left", wantApplied: true, wantKicks: 1},
		{name: "mute server error but member muted", action: "mute", method: "restrictChatMember", actionErr: broken, live: "restricted", wantApplied: true, wantPending: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := newCaptchaReviewTimeoutEnv(t)
			env.client.errors[tc.method] = tc.actionErr
			if tc.live != "" {
				env.liveStatus(tc.live)
			}
			if tc.liveErr != nil {
				env.client.errors["getChatMember"] = tc.liveErr
			}

			claimed, applied := handleCaptchaTimeout(env.bot, env.chatID, env.userID, env.id, 123, tc.action)
			if !claimed || applied != tc.wantApplied {
				t.Fatalf("handleCaptchaTimeout() = (%v, %v), want (true, %v)", claimed, applied, tc.wantApplied)
			}
			if got := unmutesOf(t, env.client, env.userID); (got > 0) != tc.wantUnmute {
				t.Fatalf("unmutes = %d, want unmuted %v", got, tc.wantUnmute)
			}
			if got := kicksOf(env.client, env.userID); got != tc.wantKicks {
				t.Fatalf("member-removing unbans = %d, want %d", got, tc.wantKicks)
			}
			row, err := captcha.GetMutedUser(env.userID, env.chatID)
			if err != nil {
				t.Fatalf("GetMutedUser() error = %v", err)
			}
			if (row != nil) != tc.wantPending {
				t.Fatalf("fallback unmute row = %+v, want present %v", row, tc.wantPending)
			}
			if row != nil && time.Until(row.UnmuteAt) < 23*time.Hour {
				t.Fatalf("fallback unmute at %v, want about 24 hours ahead", row.UnmuteAt)
			}
			notices := len(env.client.callsFor("sendMessage"))
			if (notices > 0) != tc.wantApplied {
				t.Fatalf("failure notices = %d, want sent %v", notices, tc.wantApplied)
			}
		})
	}
}
