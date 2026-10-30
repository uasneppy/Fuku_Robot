package captchaweb

import "sync/atomic"

// The web captcha's counters, shown in the developer /stats (D-17). They are this replica's
// memory only: they start at zero on every restart, are not fleet totals and hold numbers
// only, never a token, initData, ID or Cloudflare text. Each event is counted once.
var (
	statsIssued  atomic.Int64
	statsPassed  atomic.Int64
	statsFailed  atomic.Int64
	statsExpired atomic.Int64
	statsInvalid atomic.Int64
	statsRetry   atomic.Int64
	statsLimited atomic.Int64
	statsErrors  atomic.Int64
)

// Stats is a snapshot of the counters.
type Stats struct {
	// Issued counts challenge messages sent for a Turnstile attempt.
	Issued int64
	// Passed counts members unmuted through the web captcha.
	Passed int64
	// Failed counts solves Cloudflare provably refused.
	Failed int64
	// Expired counts Turnstile attempts the bot timed out, on whichever path ran the timeout.
	Expired int64
	// Invalid counts invalid and not_for_you answers.
	Invalid int64
	// Retry counts retry answers other than the ones counted as Limited.
	Retry int64
	// Limited counts requests refused by a spent budget (429) or by the in-flight cap.
	Limited int64
	// Errors counts configuration faults, backend errors and failed unmutes.
	Errors int64
}

// StatsSnapshot returns the counters as they are now.
func StatsSnapshot() Stats {
	return Stats{
		Issued:  statsIssued.Load(),
		Passed:  statsPassed.Load(),
		Failed:  statsFailed.Load(),
		Expired: statsExpired.Load(),
		Invalid: statsInvalid.Load(),
		Retry:   statsRetry.Load(),
		Limited: statsLimited.Load(),
		Errors:  statsErrors.Load(),
	}
}

// CountIssued records a Turnstile challenge message that was sent and stored.
func CountIssued() { statsIssued.Add(1) }

// CountExpired records a Turnstile attempt the bot timed out (the timer, the cleanup tick or
// a late check, whichever claimed it).
func CountExpired() { statsExpired.Add(1) }

// countAnswer records a decided answer by its result. An expired answer is not counted here:
// the expiry itself is counted once by CountExpired where the attempt was claimed.
func countAnswer(result Result) {
	switch result {
	case ResultPassed:
		statsPassed.Add(1)
	case ResultFailed:
		statsFailed.Add(1)
	case ResultInvalid, ResultNotForYou:
		statsInvalid.Add(1)
	case ResultRetry:
		statsRetry.Add(1)
	}
}

func countLimited() { statsLimited.Add(1) }

func countError() { statsErrors.Add(1) }
