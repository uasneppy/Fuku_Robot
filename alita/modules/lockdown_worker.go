package modules

import (
	"context"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/divkix/Alita_Robot/alita/utils/ratelimit"
)

// lockdownPacer paces every Telegram call of the lockdown worker. It is a variable
// so tests install a fast one.
var lockdownPacer = ratelimit.NewTelegramPacer(ratelimit.TelegramPacerOptions{
	NextKey:    "alita:lockdown:pace:next",
	BlockKey:   "alita:lockdown:pace:block",
	Interval:   100 * time.Millisecond,
	MaxRetries: 3,
	MaxWait:    60 * time.Second,
})

var (
	// lockdownWorkerTick is how often the worker looks for work.
	lockdownWorkerTick = 2 * time.Second
	// lockdownWorkerStopWait is the longest StopLockdownWorker waits for the worker.
	lockdownWorkerStopWait = 5 * time.Second
)

// StartLockdownWorker is not implemented yet.
func StartLockdownWorker(b *gotgbot.Bot) {}

// StopLockdownWorker is not implemented yet.
func StopLockdownWorker() {}

// runLockdownCycle is not implemented yet.
func runLockdownCycle(ctx context.Context, b *gotgbot.Bot) bool {
	return false
}
