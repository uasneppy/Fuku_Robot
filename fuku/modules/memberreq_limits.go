package modules

import (
	"context"
	"strconv"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
)

// The per-chat brake of the member requirements (D-13, D-21). Compile-only stand-in: the
// behaviour comes with the next commit.

var (
	// memberReqCapPerMinute is the most punishments that start per chat in one fixed minute.
	memberReqCapPerMinute = 10
	// memberReqPause is how long a chat's punishments stay paused once the cap is exceeded.
	memberReqPause = 10 * time.Minute
)

// memberReqCapKey is the counter of one chat's punishment attempts in one fixed minute.
func memberReqCapKey(chatID, minute int64) string {
	return "fuku:memberreq:{" + strconv.FormatInt(chatID, 10) + "}:cap:" + strconv.FormatInt(minute, 10)
}

// memberReqPausedKey is the pause latch of one chat.
func memberReqPausedKey(chatID int64) string {
	return "fuku:memberreq:{" + strconv.FormatInt(chatID, 10) + "}:paused"
}

func memberReqCapAllow(_ context.Context, _ *gotgbot.Bot, _ int64) bool { return true }

func memberReqPaused(_ context.Context, _ int64) bool { return false }
