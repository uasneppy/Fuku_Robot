package modules

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/divkix/Alita_Robot/alita/utils/claude"
)

// The breakers of the raid-only image checks. A failing provider must neither be
// called for every image nor change anything else, so two guards sit in front of the call:
// a per-chat breaker (five provider failures pause that chat's image checks) and a
// provider-wide pause (a rejected key or account, or a long rate limit, pauses every chat's).
// Only provider answers count: a getFile or download failure is a failure of one job and never
// trips either. No notice is ever posted anywhere; a pause is one log line and the Shed
// counter. All times come from raidNow, so a test moves them.

const (
	// raidAIImageBreakerMaxChats bounds the breaker map, like aispamBreakerMaxChats.
	raidAIImageBreakerMaxChats = 10000
	// raidAIImageFailures is how many provider failures pause one chat.
	raidAIImageFailures = 5
	// raidAIImageLongRateLimit is the retry delay of a 429 above which every chat is paused.
	// The Claude client retries a shorter wait once itself and drops a longer one.
	raidAIImageLongRateLimit = 2 * time.Second
	// raidAIImageMaxPause caps a pause taken from the provider's own retry delay.
	raidAIImageMaxPause = 5 * time.Minute
)

var (
	// raidAIImageCooldown is how long a chat's image checks stay paused. A variable only so a
	// test can shorten it.
	raidAIImageCooldown = 5 * time.Minute
	// raidAIImageKeyPause is how long every chat's image checks stay paused after the key was
	// rejected. A variable only so a test can shorten it.
	raidAIImageKeyPause = 5 * time.Minute
)

type raidAIImageBreaker struct {
	failures   int
	pausedTill time.Time
}

// raidAIImageCapWarned makes the "breaker map is full" warning appear once per process.
var raidAIImageCapWarned atomic.Bool

// raidAIImagePruneLocked drops the breakers that are neither paused nor counting failures,
// the ones a success or an expired pause left idle, so a full map makes room for a chat that
// is failing now. The caller holds raidAIImageBreakers.mu.
func raidAIImagePruneLocked(now time.Time) {
	for chatID, breaker := range raidAIImageBreakers.m {
		if breaker.failures == 0 && (breaker.pausedTill.IsZero() || !now.Before(breaker.pausedTill)) {
			delete(raidAIImageBreakers.m, chatID)
		}
	}
}

// raidAIImageBreakers holds the per-chat breakers.
var raidAIImageBreakers = struct {
	mu sync.Mutex
	m  map[int64]*raidAIImageBreaker
}{m: make(map[int64]*raidAIImageBreaker)}

// raidAIImagePause is the provider-wide pause.
var raidAIImagePause struct {
	mu         sync.Mutex
	pausedTill time.Time
}

// raidAIImagePaused reports whether the chat's image checks are paused, by its own breaker
// or by the provider-wide pause.
func raidAIImagePaused(chatID int64) bool {
	now := raidNow()

	raidAIImagePause.mu.Lock()
	providerPaused := now.Before(raidAIImagePause.pausedTill)
	raidAIImagePause.mu.Unlock()
	if providerPaused {
		return true
	}

	raidAIImageBreakers.mu.Lock()
	defer raidAIImageBreakers.mu.Unlock()
	breaker, ok := raidAIImageBreakers.m[chatID]
	if !ok {
		return false
	}
	return !breaker.pausedTill.IsZero() && now.Before(breaker.pausedTill)
}

// raidAIImageRecordFailure counts one failed provider answer toward the chat, which five of
// them pause for raidAIImageCooldown, and, for a rejected key or a 429 with a long retry
// delay, pauses every chat. Each pause logs one warning and posts nothing. A chat that is
// already paused counts nothing more, so the checks still in flight when it tripped do not
// pause it again.
func raidAIImageRecordFailure(chatID int64, err error) {
	now := raidNow()
	raidAIImagePauseProvider(now, err)

	raidAIImageBreakers.mu.Lock()
	breaker, ok := raidAIImageBreakers.m[chatID]
	if !ok {
		if len(raidAIImageBreakers.m) >= raidAIImageBreakerMaxChats {
			raidAIImagePruneLocked(now)
		}
		if len(raidAIImageBreakers.m) >= raidAIImageBreakerMaxChats {
			raidAIImageBreakers.mu.Unlock()
			// Every tracked chat is failing or paused: this one stays untracked, so it can
			// never be paused. Said once per process, not per failure.
			if raidAIImageCapWarned.CompareAndSwap(false, true) {
				log.Warnf("[RaidDetection] the image check breaker map is full (%d chats failing or paused); "+
					"further chats are not tracked and cannot be paused", raidAIImageBreakerMaxChats)
			}
			return
		}
		breaker = &raidAIImageBreaker{}
		raidAIImageBreakers.m[chatID] = breaker
	}
	if !breaker.pausedTill.IsZero() {
		if now.Before(breaker.pausedTill) {
			raidAIImageBreakers.mu.Unlock()
			return
		}
		breaker.failures = 0
		breaker.pausedTill = time.Time{}
	}
	breaker.failures++
	if breaker.failures < raidAIImageFailures {
		raidAIImageBreakers.mu.Unlock()
		return
	}
	breaker.failures = 0
	breaker.pausedTill = now.Add(raidAIImageCooldown)
	till := breaker.pausedTill
	raidAIImageBreakers.mu.Unlock()

	log.WithField("chat_id", chatID).Warnf(
		"[RaidDetection] AI image checks paused for this chat after %d provider failures, until %s",
		raidAIImageFailures, till.UTC().Format(time.RFC3339))
}

// raidAIImagePauseProvider pauses every chat's image checks when err says the key or its
// account is rejected or the provider asked for a long wait. It logs one warning naming the
// reason (the status and the error type only) and the end of the pause, and does nothing
// while a pause is already running.
func raidAIImagePauseProvider(now time.Time, err error) {
	var httpErr *claude.HTTPError
	if !errors.As(err, &httpErr) {
		return
	}
	var span time.Duration
	var reason string
	switch {
	case httpErr.KeyRejected():
		span = raidAIImageKeyPause
		reason = fmt.Sprintf("the key or its account was rejected (HTTP %d %s)", httpErr.Status, httpErr.Type)
	case httpErr.Status == 429 && httpErr.RetryDelay > raidAIImageLongRateLimit:
		span = min(httpErr.RetryDelay, raidAIImageMaxPause)
		reason = fmt.Sprintf("rate limited for %s (HTTP 429 %s)", httpErr.RetryDelay.Round(time.Second), httpErr.Type)
	default:
		return
	}

	raidAIImagePause.mu.Lock()
	if now.Before(raidAIImagePause.pausedTill) {
		raidAIImagePause.mu.Unlock()
		return
	}
	till := now.Add(span)
	raidAIImagePause.pausedTill = till
	raidAIImagePause.mu.Unlock()

	log.Warnf("[RaidDetection] AI image checks paused for every chat: %s; they resume at %s",
		reason, till.UTC().Format(time.RFC3339))
}

// raidAIImageRecordSuccess clears the chat's failure count and pause: the provider answered.
func raidAIImageRecordSuccess(chatID int64) {
	raidAIImageBreakers.mu.Lock()
	defer raidAIImageBreakers.mu.Unlock()
	if breaker, ok := raidAIImageBreakers.m[chatID]; ok {
		breaker.failures = 0
		breaker.pausedTill = time.Time{}
	}
}
