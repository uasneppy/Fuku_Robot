//go:build testtools

package captchaweb

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	log "github.com/sirupsen/logrus"
)

// failStreakHook fails every Redis command that touches the streak key, as a Redis that went
// away between two commands would.
type failStreakHook struct{ failing atomic.Bool }

func (h *failStreakHook) DialHook(next redis.DialHook) redis.DialHook { return next }

func (h *failStreakHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		if h.failing.Load() {
			for _, arg := range cmd.Args() {
				if s, ok := arg.(string); ok && s == StreakKey {
					return errors.New("redis went away")
				}
			}
		}
		return next(ctx, cmd)
	}
}

func (h *failStreakHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}

// postNewMember issues a challenge for a member with a fresh ID and posts what the page posts.
func (o *outcomeHandler) postNewMember(userID int64) (int, Answer, string) {
	o.t.Helper()
	token, _ := o.issue(userID)
	status, answer := o.post(userID, token)
	return status, answer, token
}

// TestVerifyRouteKeyMismatchBreaker pins WR-04: a mis-paired site key and secret (or the
// Cloudflare test keys) make Cloudflare call every honest solve invalid, and D-28 reads that as
// the member's fault. After StreakLimit failed solves with no pass in between, further ones are
// held as retry: the member stays muted, the attempt stays intact and nobody is removed.
func TestVerifyRouteKeyMismatchBreaker(t *testing.T) {
	invalid := SiteverifyAnswerForTest(200, failureBody("invalid-input-response"))

	t.Run("a single bad token never trips it", func(t *testing.T) {
		o := newOutcomeHandler(t)
		o.setSiteverify(invalid)
		status, answer, _ := o.postNewMember(1)
		if status != 200 || answer.Result != ResultFailed || answer.Action != "kick" {
			t.Fatalf("post = %d %+v, want 200 failed with kick", status, answer)
		}
		if got, err := o.Redis.Get(StreakKey); err != nil || got != "1" {
			t.Fatalf("streak = %q, %v; want 1", got, err)
		}
		if ttl := o.Redis.TTL(StreakKey); ttl <= 9*time.Minute || ttl > StreakTTL {
			t.Fatalf("streak ttl = %v, want about %v", ttl, StreakTTL)
		}

		// One pass later the streak is gone, and the next bad token is a streak of one again.
		o.setSiteverify(SiteverifyPassForTest(outcomeHost))
		if status, answer, _ := o.postNewMember(2); status != 200 || answer.Result != ResultPassed {
			t.Fatalf("pass = %d %+v, want 200 passed", status, answer)
		}
		if o.Redis.Exists(StreakKey) {
			t.Fatal("a confirmed pass left the streak behind")
		}
	})

	t.Run("failed solves up to the limit are enforced and the next ones are held", func(t *testing.T) {
		hook := captureLogs(t)
		o := newOutcomeHandler(t)
		o.setSiteverify(invalid)

		for i := 1; i <= StreakLimit; i++ {
			status, answer, _ := o.postNewMember(int64(i))
			if status != 200 || answer.Result != ResultFailed {
				t.Fatalf("failed solve %d = %d %+v, want 200 failed", i, status, answer)
			}
		}
		failsBefore := 0
		for _, call := range o.Backend.Calls() {
			if call == "fail" {
				failsBefore++
			}
		}
		if failsBefore != StreakLimit {
			t.Fatalf("fail calls = %d, want %d", failsBefore, StreakLimit)
		}

		for i := StreakLimit + 1; i <= StreakLimit+3; i++ {
			status, answer, token := o.postNewMember(int64(i))
			if status != 200 || answer.Result != ResultRetry || answer.Action != "" {
				t.Fatalf("held solve %d = %d %+v, want 200 retry", i, status, answer)
			}
			if got := o.Redis.HGet(o.key(token), "u"); got != strconv.Itoa(i) {
				t.Fatalf("held solve %d: challenge owner %q, want it given back", i, got)
			}
			if o.Redis.Exists(DoneKeyPrefix + token) {
				t.Fatalf("held solve %d wrote a decided-answer marker", i)
			}
		}
		fails := 0
		for _, call := range o.Backend.Calls() {
			if call == "fail" {
				fails++
			}
		}
		if fails != StreakLimit {
			t.Fatalf("fail calls = %d after the breaker tripped, want still %d (nobody else is removed)", fails, StreakLimit)
		}

		// One Error entry for three held solves, by reason name only.
		hook.mu.Lock()
		defer hook.mu.Unlock()
		var errorsLogged int
		for _, entry := range hook.entries {
			if strings.Contains(entry.text, "suspected key mismatch") {
				errorsLogged++
				if entry.level != log.ErrorLevel {
					t.Errorf("mismatch notice logged at %s, want error: %s", entry.level, entry.text)
				}
				if !strings.Contains(entry.text, "reason=invalid_token") {
					t.Errorf("mismatch notice does not name the reason: %s", entry.text)
				}
			}
		}
		if errorsLogged != 1 {
			t.Fatalf("mismatch notices = %d, want 1 for the minute", errorsLogged)
		}
	})

	t.Run("the notice is logged again after a minute", func(t *testing.T) {
		hook := captureLogs(t)
		o := newOutcomeHandler(t)
		o.setSiteverify(invalid)
		var offset atomic.Int64
		start := time.Now()
		o.rebuild(func(opts *Options) {
			opts.Now = func() time.Time { return start.Add(time.Duration(offset.Load())) }
		})
		for i := 1; i <= StreakLimit+1; i++ {
			o.postNewMember(int64(i))
		}
		offset.Store(int64(61 * time.Second))
		o.postNewMember(StreakLimit + 2)

		hook.mu.Lock()
		defer hook.mu.Unlock()
		var logged int
		for _, entry := range hook.entries {
			if strings.Contains(entry.text, "suspected key mismatch") {
				logged++
			}
		}
		if logged != 2 {
			t.Fatalf("mismatch notices = %d, want 2 (one per minute)", logged)
		}
	})

	t.Run("a pass resets the streak", func(t *testing.T) {
		o := newOutcomeHandler(t)
		o.setSiteverify(invalid)
		for i := 1; i <= StreakLimit; i++ {
			o.postNewMember(int64(i))
		}
		if _, answer, _ := o.postNewMember(100); answer.Result != ResultRetry {
			t.Fatalf("held solve = %+v, want retry", answer)
		}

		// The owner fixes the keys: a member passes, and the breaker is open again.
		o.setSiteverify(SiteverifyPassForTest(outcomeHost))
		if status, answer, _ := o.postNewMember(101); status != 200 || answer.Result != ResultPassed {
			t.Fatalf("pass = %d %+v, want 200 passed", status, answer)
		}
		if o.Redis.Exists(StreakKey) {
			t.Fatal("the pass did not end the streak")
		}
		o.setSiteverify(invalid)
		if status, answer, _ := o.postNewMember(102); status != 200 || answer.Result != ResultFailed {
			t.Fatalf("failed solve after the reset = %d %+v, want 200 failed", status, answer)
		}
	})

	t.Run("a quiet streak expires", func(t *testing.T) {
		o := newOutcomeHandler(t)
		o.setSiteverify(invalid)
		for i := 1; i <= StreakLimit+1; i++ {
			o.postNewMember(int64(i))
		}
		o.Redis.FastForward(StreakTTL + time.Second)
		if status, answer, _ := o.postNewMember(200); status != 200 || answer.Result != ResultFailed {
			t.Fatalf("failed solve after the streak expired = %d %+v, want 200 failed", status, answer)
		}
	})

	t.Run("Redis down fails open", func(t *testing.T) {
		o := newOutcomeHandler(t)
		o.setSiteverify(invalid)
		hook := &failStreakHook{}
		o.Store.client.AddHook(hook)
		for i := 1; i <= StreakLimit+2; i++ {
			o.postNewMember(int64(i))
		}
		hook.failing.Store(true)
		// The streak is far past the limit and cannot be read: D-28 stands, the solve is
		// enforced and the member is not blocked.
		status, answer, _ := o.postNewMember(300)
		if status != 200 || answer.Result != ResultFailed || answer.Action != "kick" {
			t.Fatalf("failed solve with the streak unreadable = %d %+v, want 200 failed with kick", status, answer)
		}

		// A pass still goes through when the streak cannot be cleared.
		o.setSiteverify(SiteverifyPassForTest(outcomeHost))
		if status, answer, _ := o.postNewMember(301); status != 200 || answer.Result != ResultPassed {
			t.Fatalf("pass with the streak unreadable = %d %+v, want 200 passed", status, answer)
		}
	})
}
