package modules

import (
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
)

// lockdownBanUntilTolerance is how many seconds the live end date of a ban may differ
// from the stored one and still count as the lockdown's own.
const lockdownBanUntilTolerance int64 = 2

// isLockdownBan reports whether a live chat member is under the lockdown's own ban:
// kicked, with an end date within lockdownBanUntilTolerance seconds of the row's
// ban_until. The lockdown's own ban is recognised by its end date alone, because every
// other ban path (/ban, staff /ban, fed-ban, captcha, Phase 5's "Ban N recent
// joiners") replaces the status and with it the end date, so a deliberate ban never
// carries it. A user who left, was unbanned or was restricted is not under it, and
// neither is anyone when no end date was stored.
func isLockdownBan(m gotgbot.MergedChatMember, banUntil int64) bool {
	if m.Status != gotgbot.ChatMemberStatusKicked || banUntil == 0 || m.UntilDate == 0 {
		return false
	}
	diff := m.UntilDate - banUntil
	if diff < 0 {
		diff = -diff
	}
	return diff <= lockdownBanUntilTolerance
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
