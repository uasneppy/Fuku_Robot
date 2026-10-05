//go:build testtools

package ratelimit

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/divkix/Alita_Robot/alita/utils/cache"
)

// newPacerRedis starts a miniredis and installs a client for it as the package
// cache's Redis client. The client does not retry a failed command, so a closed
// server fails fast.
func newPacerRedis(t *testing.T) *miniredis.Miniredis {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run() error = %v", err)
	}
	t.Cleanup(mr.Close)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr(), MaxRetries: -1})
	t.Cleanup(func() { _ = client.Close() })
	cache.SetRedisClientForTest(t, client)
	return mr
}

// pacerOpts returns options with key names unique to the test.
func pacerOpts(t *testing.T) TelegramPacerOptions {
	t.Helper()
	return TelegramPacerOptions{
		NextKey:  "test:pace:next:" + t.Name(),
		BlockKey: "test:pace:block:" + t.Name(),
	}
}

func tg429(retryAfter int64) error {
	return &gotgbot.TelegramError{
		Code:           429,
		Description:    fmt.Sprintf("Too Many Requests: retry after %d", retryAfter),
		ResponseParams: &gotgbot.ResponseParameters{RetryAfter: retryAfter},
	}
}

// callLog records when and how often a paced call ran.
type callLog struct {
	mu    sync.Mutex
	times []time.Time
}

func (l *callLog) record() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.times = append(l.times, time.Now())
	return len(l.times)
}

func (l *callLog) count() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.times)
}

func (l *callLog) at(i int) time.Time {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.times[i]
}

func TestTelegramPacerFleetSpacing(t *testing.T) {
	newPacerRedis(t)
	opts := pacerOpts(t)
	opts.Interval = 40 * time.Millisecond
	pacers := []*TelegramPacer{NewTelegramPacer(opts), NewTelegramPacer(opts)}

	var calls callLog
	var wg sync.WaitGroup
	t0 := time.Now()
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func(p *TelegramPacer) {
			defer wg.Done()
			err := p.Do(context.Background(), func(context.Context) error {
				calls.record()
				return nil
			})
			if err != nil {
				t.Errorf("Do() error = %v, want nil", err)
			}
		}(pacers[i%2])
	}
	wg.Wait()

	if calls.count() != 6 {
		t.Fatalf("calls = %d, want 6", calls.count())
	}
	starts := make([]time.Time, 0, 6)
	for i := 0; i < 6; i++ {
		starts = append(starts, calls.at(i))
	}
	sort.Slice(starts, func(i, j int) bool { return starts[i].Before(starts[j]) })
	// The fleet reserves one slot per interval, so the i-th call cannot start
	// before t0 + i*interval. A late timer wake-up only delays a call, so this
	// bound holds under scheduler jitter where neighbour gaps do not. The 2ms
	// slack covers the script's millisecond flooring of Redis TIME.
	for i, start := range starts {
		earliest := time.Duration(i)*opts.Interval - 2*time.Millisecond
		if got := start.Sub(t0); got < earliest {
			t.Fatalf("call %d started %v after the first request, want at least %v across the fleet", i, got, earliest)
		}
	}
}

func TestTelegramPacerRetriesThenSucceeds(t *testing.T) {
	newPacerRedis(t)
	opts := pacerOpts(t)
	opts.Interval = time.Millisecond
	opts.RetryAfterUnit = 10 * time.Millisecond
	pacer := NewTelegramPacer(opts)

	type args struct{ chat, until int64 }
	want := args{chat: -1001, until: 1790000000}
	var seen []args
	var calls callLog
	err := pacer.Do(context.Background(), func(context.Context) error {
		n := calls.record()
		seen = append(seen, want)
		if n <= 2 {
			return tg429(1)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Do() error = %v, want nil", err)
	}
	if calls.count() != 3 {
		t.Fatalf("invocations = %d, want exactly 3", calls.count())
	}
	for i, got := range seen {
		if got != want {
			t.Fatalf("invocation %d args = %+v, want the same %+v", i, got, want)
		}
	}
}

func TestTelegramPacerGivesUp(t *testing.T) {
	newPacerRedis(t)
	opts := pacerOpts(t)
	opts.Interval = time.Millisecond
	opts.RetryAfterUnit = 10 * time.Millisecond
	opts.MaxRetries = 3
	pacer := NewTelegramPacer(opts)

	var calls callLog
	err := pacer.Do(context.Background(), func(context.Context) error {
		calls.record()
		return tg429(1)
	})
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("Do() error = %v, want ErrRateLimited", err)
	}
	if calls.count() != 4 {
		t.Fatalf("invocations = %d, want 4 (one try and 3 retries)", calls.count())
	}
}

func TestTelegramPacerRetryAfterCap(t *testing.T) {
	t.Run("equal to the cap is waited and retried", func(t *testing.T) {
		newPacerRedis(t)
		opts := pacerOpts(t)
		opts.Interval = time.Millisecond
		opts.RetryAfterUnit = 10 * time.Millisecond
		opts.MaxWait = 50 * time.Millisecond
		pacer := NewTelegramPacer(opts)

		var calls callLog
		err := pacer.Do(context.Background(), func(context.Context) error {
			if calls.record() == 1 {
				return tg429(5)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("Do() error = %v, want nil after the retry", err)
		}
		if calls.count() != 2 {
			t.Fatalf("invocations = %d, want 2", calls.count())
		}
		if gap := calls.at(1).Sub(calls.at(0)); gap < 45*time.Millisecond {
			t.Fatalf("retry came after %v, want it to wait out the 50ms retry_after", gap)
		}
	})

	t.Run("above the cap fails at once and still sets the block", func(t *testing.T) {
		mr := newPacerRedis(t)
		opts := pacerOpts(t)
		opts.Interval = time.Millisecond
		opts.RetryAfterUnit = 10 * time.Millisecond
		opts.MaxWait = 50 * time.Millisecond
		pacer := NewTelegramPacer(opts)

		var calls callLog
		err := pacer.Do(context.Background(), func(context.Context) error {
			calls.record()
			return tg429(6)
		})
		if !errors.Is(err, ErrRateLimited) {
			t.Fatalf("Do() error = %v, want ErrRateLimited", err)
		}
		if calls.count() != 1 {
			t.Fatalf("invocations = %d, want 1", calls.count())
		}
		ttl := mr.TTL(opts.BlockKey)
		if ttl < time.Millisecond || ttl > 60*time.Millisecond {
			t.Fatalf("block TTL = %v, want between 1ms and 60ms", ttl)
		}
	})
}

func TestTelegramPacerSharedBlock(t *testing.T) {
	mr := newPacerRedis(t)
	opts := pacerOpts(t)
	opts.Interval = time.Millisecond
	opts.RetryAfterUnit = 10 * time.Millisecond

	// Pacer A gives up at once (its cap is below the 30ms block), so it returns
	// right after seeing the 429 and leaves only the shared block behind.
	optsA := opts
	optsA.MaxWait = 20 * time.Millisecond
	pacerA := NewTelegramPacer(optsA)
	var seen429 time.Time
	err := pacerA.Do(context.Background(), func(context.Context) error {
		seen429 = time.Now()
		return tg429(3)
	})
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("pacer A error = %v, want ErrRateLimited", err)
	}
	ttl := mr.TTL(opts.BlockKey)
	if ttl <= 0 || ttl > 30*time.Millisecond {
		t.Fatalf("block TTL = %v, want more than 0 and at most 30ms", ttl)
	}

	pacerB := NewTelegramPacer(opts)
	var b callLog
	if err := pacerB.Do(context.Background(), func(context.Context) error {
		b.record()
		return nil
	}); err != nil {
		t.Fatalf("pacer B error = %v, want nil", err)
	}
	blockEnd := seen429.Add(30 * time.Millisecond)
	if started := b.at(0); started.Before(blockEnd.Add(-5 * time.Millisecond)) {
		t.Fatalf("pacer B called %v before the shared block ended, want to wait for it", blockEnd.Sub(started))
	}
}

func TestTelegramPacerRedisDownFallsBack(t *testing.T) {
	mr := newPacerRedis(t)
	opts := pacerOpts(t)
	opts.Interval = 40 * time.Millisecond
	pacer := NewTelegramPacer(opts)
	mr.Close()

	var calls callLog
	for i := 0; i < 3; i++ {
		err := pacer.Do(context.Background(), func(context.Context) error {
			calls.record()
			return nil
		})
		if err != nil {
			t.Fatalf("Do() #%d error = %v, want nil with Redis down", i, err)
		}
	}
	if calls.count() != 3 {
		t.Fatalf("invocations = %d, want 3", calls.count())
	}
	for i := 1; i < 3; i++ {
		if gap := calls.at(i).Sub(calls.at(i - 1)); gap < 35*time.Millisecond {
			t.Fatalf("local gap %d = %v, want at least 35ms", i, gap)
		}
	}
}

func TestTelegramPacerContextCancel(t *testing.T) {
	mr := newPacerRedis(t)
	opts := pacerOpts(t)
	opts.Interval = time.Millisecond
	pacer := NewTelegramPacer(opts)
	mr.Set(opts.BlockKey, "1")
	mr.SetTTL(opts.BlockKey, time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	var calls callLog
	started := time.Now()
	err := pacer.Do(ctx, func(context.Context) error {
		calls.record()
		return nil
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Do() error = %v, want the context error", err)
	}
	if calls.count() != 0 {
		t.Fatalf("invocations = %d, want 0 after the context ended", calls.count())
	}
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("Do() took %v, want it to stop when the context ended", elapsed)
	}
}

func TestTelegramPacerPassesOtherErrors(t *testing.T) {
	newPacerRedis(t)
	opts := pacerOpts(t)
	opts.Interval = time.Millisecond
	pacer := NewTelegramPacer(opts)

	badRequest := &gotgbot.TelegramError{Code: 400, Description: "Bad Request: chat not found"}
	var calls callLog
	err := pacer.Do(context.Background(), func(context.Context) error {
		calls.record()
		return badRequest
	})
	if err != error(badRequest) {
		t.Fatalf("Do() error = %v, want the 400 returned as-is", err)
	}
	if calls.count() != 1 {
		t.Fatalf("invocations = %d, want exactly 1", calls.count())
	}
}

func TestRetryAfterSeconds(t *testing.T) {
	if got, ok := RetryAfterSeconds(tg429(3)); !ok || got != 3 {
		t.Fatalf("RetryAfterSeconds(429 retry_after=3) = (%d, %v), want (3, true)", got, ok)
	}
	wrapped := fmt.Errorf("call failed: %w", tg429(7))
	if got, ok := RetryAfterSeconds(wrapped); !ok || got != 7 {
		t.Fatalf("RetryAfterSeconds(wrapped 429) = (%d, %v), want (7, true)", got, ok)
	}
	for name, err := range map[string]error{
		"429 without parameters": &gotgbot.TelegramError{Code: 429, Description: "Too Many Requests"},
		"400":                    &gotgbot.TelegramError{Code: 400, Description: "Bad Request"},
		"nil":                    nil,
		"plain error":            errors.New("boom"),
	} {
		if got, ok := RetryAfterSeconds(err); ok {
			t.Fatalf("RetryAfterSeconds(%s) = (%d, true), want false", name, got)
		}
	}
}
