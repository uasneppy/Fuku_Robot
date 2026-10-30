//go:build testtools

package captchaweb

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestWindowLimiter(t *testing.T) {
	mr := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rc.Close() })
	limiter := NewLimiter(rc)
	ctx := context.Background()
	const key = "alita:captcha:rl:t:window-test"
	const limit = 8
	window := time.Minute

	for i := 1; i <= limit; i++ {
		allowed, _, err := limiter.Allow(ctx, key, limit, window)
		if err != nil || !allowed {
			t.Fatalf("call %d: allowed=%v err=%v, want allowed", i, allowed, err)
		}
	}
	allowed, retryAfter, err := limiter.Allow(ctx, key, limit, window)
	if err != nil {
		t.Fatalf("over-limit call: err = %v", err)
	}
	if allowed {
		t.Fatalf("call %d was allowed, want refused", limit+1)
	}
	if retryAfter <= 0 || retryAfter > window {
		t.Fatalf("retryAfter = %v, want within (0, %v]", retryAfter, window)
	}

	mr.FastForward(window + time.Second)
	if mr.Exists(key) {
		t.Fatalf("window key survived its window")
	}
	allowed, _, err = limiter.Allow(ctx, key, limit, window)
	if err != nil || !allowed {
		t.Fatalf("after the window: allowed=%v err=%v, want allowed", allowed, err)
	}

	mr.Close()
	allowed, _, err = limiter.Allow(ctx, key, limit, window)
	if err == nil {
		t.Fatalf("closed redis: err = nil, want an error")
	}
	if allowed {
		t.Fatalf("closed redis: allowed = true, an error must never allow")
	}
}
