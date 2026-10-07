package modules

import (
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
)

// lockdownBanUntilTolerance is how many seconds the live end date of a ban may differ
// from the stored one and still count as the lockdown's own.
const lockdownBanUntilTolerance int64 = 2

// isLockdownBan is not implemented yet.
func isLockdownBan(m gotgbot.MergedChatMember, banUntil int64) bool {
	return false
}

// lockdownBanSpan is how long the lockdown's own ban lasts. It sits inside
// Telegram's finite-ban window (a ban shorter than 30 seconds or longer than 366
// days is permanent), so the ban ends on a date no deliberate ban shares: the lift
// recognises the lockdown's own ban by that end date alone.
const lockdownBanSpan = 330 * 24 * time.Hour

// lockdownBanUntil is the end date, in Unix seconds, of a lockdown ban placed at now.
func lockdownBanUntil(now time.Time) int64 {
	return now.Add(lockdownBanSpan).Unix()
}
