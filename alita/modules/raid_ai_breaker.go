package modules

import (
	"errors"
	"fmt"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/divkix/Alita_Robot/alita/utils/gemini"
)

// The breakers of the raid-only Gemini image checks. A failing provider must neither be
// called for every image nor change anything else, so two guards sit in front of the call:
// a per-chat breaker (five Gemini failures pause that chat's image checks) and a
// provider-wide pause (a rejected key, or a long rate limit, pauses every chat's). Only
// Gemini answers count: a getFile or download failure is a failure of one job and never
// trips either. No notice is ever posted anywhere; a pause is one log line and the Shed
// counter. All times come from raidNow, so a test moves them.

const (
	// raidAIGeminiBreakerMaxChats bounds the breaker map, like aispamBreakerMaxChats.
	raidAIGeminiBreakerMaxChats = 10000
	// raidAIGeminiFailures is how many Gemini failures pause one chat.
	raidAIGeminiFailures = 5
	// raidAIGeminiLongRateLimit is the retry delay of a 429 above which every chat is paused.
	// The Gemini client retries a shorter wait once itself and drops a longer one.
	raidAIGeminiLongRateLimit = 2 * time.Second
	// raidAIGeminiMaxPause caps a pause taken from the provider's own retry delay.
	raidAIGeminiMaxPause = 5 * time.Minute
)

var (
	// raidAIGeminiCooldown is how long a chat's image checks stay paused. A variable only so a
	// test can shorten it.
	raidAIGeminiCooldown = 5 * time.Minute
	// raidAIGeminiKeyPause is how long every chat's image checks stay paused after the key was
	// rejected. A variable only so a test can shorten it.
	raidAIGeminiKeyPause = 5 * time.Minute
)

type raidAIGeminiBreaker struct {
	failures   int
	pausedTill time.Time
}

// raidAIGeminiBreakers holds the per-chat breakers.
var raidAIGeminiBreakers = struct {
	mu sync.Mutex
	m  map[int64]*raidAIGeminiBreaker
}{m: make(map[int64]*raidAIGeminiBreaker)}

// raidAIGeminiPause is the provider-wide pause.
var raidAIGeminiPause struct {
	mu         sync.Mutex
	pausedTill time.Time
}

// raidAIGeminiPaused reports whether the chat's image checks are paused, by its own breaker
// or by the provider-wide pause.
func raidAIGeminiPaused(chatID int64) bool {
	now := raidNow()

	raidAIGeminiPause.mu.Lock()
	providerPaused := now.Before(raidAIGeminiPause.pausedTill)
	raidAIGeminiPause.mu.Unlock()
	if providerPaused {
		return true
	}

	raidAIGeminiBreakers.mu.Lock()
	defer raidAIGeminiBreakers.mu.Unlock()
	breaker, ok := raidAIGeminiBreakers.m[chatID]
	if !ok {
		return false
	}
	return !breaker.pausedTill.IsZero() && now.Before(breaker.pausedTill)
}

// raidAIGeminiRecordFailure counts one failed Gemini answer toward the chat, which five of
// them pause for raidAIGeminiCooldown, and, for a rejected key or a 429 with a long retry
// delay, pauses every chat. Each pause logs one warning and posts nothing. A chat that is
// already paused counts nothing more, so the checks still in flight when it tripped do not
// pause it again.
func raidAIGeminiRecordFailure(chatID int64, err error) {
	now := raidNow()
	raidAIGeminiPauseProvider(now, err)

	raidAIGeminiBreakers.mu.Lock()
	breaker, ok := raidAIGeminiBreakers.m[chatID]
	if !ok {
		if len(raidAIGeminiBreakers.m) >= raidAIGeminiBreakerMaxChats {
			raidAIGeminiBreakers.mu.Unlock()
			return
		}
		breaker = &raidAIGeminiBreaker{}
		raidAIGeminiBreakers.m[chatID] = breaker
	}
	if !breaker.pausedTill.IsZero() {
		if now.Before(breaker.pausedTill) {
			raidAIGeminiBreakers.mu.Unlock()
			return
		}
		breaker.failures = 0
		breaker.pausedTill = time.Time{}
	}
	breaker.failures++
	if breaker.failures < raidAIGeminiFailures {
		raidAIGeminiBreakers.mu.Unlock()
		return
	}
	breaker.failures = 0
	breaker.pausedTill = now.Add(raidAIGeminiCooldown)
	till := breaker.pausedTill
	raidAIGeminiBreakers.mu.Unlock()

	log.WithField("chat_id", chatID).Warnf(
		"[RaidDetection] AI image checks paused for this chat after %d Gemini failures, until %s",
		raidAIGeminiFailures, till.UTC().Format(time.RFC3339))
}

// raidAIGeminiPauseProvider pauses every chat's image checks when err says the key is
// rejected or the provider asked for a long wait. It logs one warning naming the reason (the
// status and the status word only) and the end of the pause, and does nothing while a pause
// is already running.
func raidAIGeminiPauseProvider(now time.Time, err error) {
	var httpErr *gemini.HTTPError
	if !errors.As(err, &httpErr) {
		return
	}
	var span time.Duration
	var reason string
	switch {
	case httpErr.KeyRejected():
		span = raidAIGeminiKeyPause
		reason = fmt.Sprintf("the key was rejected (HTTP %d %s)", httpErr.Status, httpErr.Code)
	case httpErr.Status == 429 && httpErr.RetryDelay > raidAIGeminiLongRateLimit:
		span = min(httpErr.RetryDelay, raidAIGeminiMaxPause)
		reason = fmt.Sprintf("rate limited for %s (HTTP 429 %s)", httpErr.RetryDelay.Round(time.Second), httpErr.Code)
	default:
		return
	}

	raidAIGeminiPause.mu.Lock()
	if now.Before(raidAIGeminiPause.pausedTill) {
		raidAIGeminiPause.mu.Unlock()
		return
	}
	till := now.Add(span)
	raidAIGeminiPause.pausedTill = till
	raidAIGeminiPause.mu.Unlock()

	log.Warnf("[RaidDetection] AI image checks paused for every chat: %s; they resume at %s",
		reason, till.UTC().Format(time.RFC3339))
}

// raidAIGeminiRecordSuccess clears the chat's failure count and pause: Gemini answered.
func raidAIGeminiRecordSuccess(chatID int64) {
	raidAIGeminiBreakers.mu.Lock()
	defer raidAIGeminiBreakers.mu.Unlock()
	if breaker, ok := raidAIGeminiBreakers.m[chatID]; ok {
		breaker.failures = 0
		breaker.pausedTill = time.Time{}
	}
}
