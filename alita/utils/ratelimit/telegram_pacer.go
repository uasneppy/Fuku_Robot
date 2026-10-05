package ratelimit

import (
	"context"
	"errors"
	"time"
)

// TelegramPacerOptions tunes a TelegramPacer. Zero values take the defaults:
// 100 ms between calls, 3 retries, a 60 s cap on one wait and seconds as the
// unit of Telegram's retry_after.
type TelegramPacerOptions struct {
	NextKey        string
	BlockKey       string
	Interval       time.Duration
	MaxRetries     int
	MaxWait        time.Duration
	RetryAfterUnit time.Duration
}

// TelegramPacer paces Telegram calls across every replica sharing one Redis.
type TelegramPacer struct {
	opts TelegramPacerOptions
}

// ErrRateLimited is returned when Telegram kept answering 429 after the allowed retries.
var ErrRateLimited = errors.New("telegram rate limit: gave up after retries")

// NewTelegramPacer returns a pacer for opts.
func NewTelegramPacer(opts TelegramPacerOptions) *TelegramPacer {
	return &TelegramPacer{opts: opts}
}

// RetryAfterSeconds reports Telegram's retry_after for a 429 answer.
func RetryAfterSeconds(err error) (int64, bool) {
	return 0, false
}

// Do runs call under the pacer.
func (p *TelegramPacer) Do(ctx context.Context, call func(context.Context) error) error {
	return errors.New("telegram pacer: not implemented")
}
