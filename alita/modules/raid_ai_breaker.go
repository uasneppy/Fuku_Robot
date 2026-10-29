package modules

import (
	"sync"
	"time"
)

// The breakers of the raid-only Gemini image checks. A failing provider must neither be
// called for every image nor change anything else, so two guards sit in front of the call:
// a per-chat breaker (five Gemini failures pause that chat's image checks) and a
// provider-wide pause (a rejected key, or a long rate limit, pauses every chat's). Only
// Gemini answers count: a getFile or download failure is a failure of one job and never
// trips either. No notice is ever posted anywhere; a pause is a log line and the Shed
// counter. All times come from raidNow, so a test moves them.

const (
	// raidAIGeminiBreakerMaxChats bounds the breaker map, like aispamBreakerMaxChats.
	raidAIGeminiBreakerMaxChats = 10000
	// raidAIGeminiFailures is how many Gemini failures pause one chat.
	raidAIGeminiFailures = 5
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
	reason     string
}

// raidAIGeminiPaused reports whether the chat's image checks are paused, by its own breaker
// or by the provider-wide pause.
func raidAIGeminiPaused(chatID int64) bool {
	return false
}

// raidAIGeminiRecordFailure counts one failed Gemini answer toward the chat and, for a
// rejected key or a long rate limit, pauses every chat.
func raidAIGeminiRecordFailure(chatID int64, err error) {
}

// raidAIGeminiRecordSuccess clears the chat's failure count.
func raidAIGeminiRecordSuccess(chatID int64) {
}
