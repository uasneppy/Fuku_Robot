//go:build testtools

package modules

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	log "github.com/sirupsen/logrus"

	"github.com/uasneppy/fuku_robot/fuku/utils/cache"
	"github.com/uasneppy/fuku_robot/fuku/utils/ratelimit"
)

// mreqLookupEnv builds a requirements environment whose bot answers getUserProfilePhotos from
// a fresh shared state, with the lookup pacer made fast.
func mreqLookupEnv(t *testing.T) (*memberReqEnv, *mreqLookupState) {
	t.Helper()
	env := newMemberReqEnv(t)
	withFastLookupPacer(t)
	state := newMreqLookupState()
	mreqInstallLookupFake(env, state)
	return env, state
}

// mreqLookupUser mints a user ID whose photo key and rows are deleted when the test ends.
func mreqLookupUser(t *testing.T) int64 {
	t.Helper()
	id := uniqueLinkOwnerID()
	mreqLookupCleanup(t, []int64{id}, nil)
	return id
}

// mreqSetKey stores the value 1 under key in the environment's Redis with a TTL.
func mreqSetKey(t *testing.T, e *memberReqEnv, key string, ttl time.Duration) {
	t.Helper()
	if err := e.redis.Set(key, "1"); err != nil {
		t.Fatalf("set %s: %v", key, err)
	}
	e.redis.SetTTL(key, ttl)
}

// mreqPhoto runs the photo rule for userID with a 5 s caller deadline.
func mreqPhoto(e *memberReqEnv, userID int64, mode memberReqLookupMode) memberReqVerdict {
	e.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return memberReqPhotoRule(ctx, e.bot, e.chat.Id, userID, mode)
}

func TestMemberReqVerdictZeroValue(t *testing.T) {
	var zero memberReqVerdict
	if zero != memberReqUnknown {
		t.Fatalf("the zero verdict = %d, want memberReqUnknown (%d): a path that sets none must do nothing", zero, memberReqUnknown)
	}
	seen := map[memberReqVerdict]bool{}
	for _, v := range []memberReqVerdict{memberReqUnknown, memberReqPass, memberReqBreach, memberReqInert} {
		if seen[v] {
			t.Fatalf("verdict %d appears twice: the four verdicts must be distinct", v)
		}
		seen[v] = true
	}
	if memberReqBreach == zero {
		t.Fatal("a breach equals the zero value")
	}
}

func TestMemberReqPhotoRule(t *testing.T) {
	const ttlLow, ttlHigh = 29 * time.Minute, 30 * time.Minute

	t.Run("a photo is a pass, cached for 30 minutes, and the second call asks nobody", func(t *testing.T) {
		env, state := mreqLookupEnv(t)
		user := mreqLookupUser(t)
		state.set(user, mreqPhotoAnswer{Total: 1, List: true})

		if got := mreqPhoto(env, user, memberReqLookupMode{}); got != memberReqPass {
			t.Fatalf("verdict = %d, want pass", got)
		}
		key := memberReqPhotoKey(user)
		if !env.redis.Exists(key) {
			t.Fatalf("key %s was not written for a pass", key)
		}
		if ttl := env.redis.TTL(key); ttl < ttlLow || ttl > ttlHigh {
			t.Errorf("TTL of %s = %s, want about 30 minutes", key, ttl)
		}
		if got := mreqPhoto(env, user, memberReqLookupMode{}); got != memberReqPass {
			t.Fatalf("second verdict = %d, want pass", got)
		}
		if n := mreqLookupPhotoCalls(env); n != 1 {
			t.Errorf("Telegram requests = %d, want 1 (the second call is served from the cache)", n)
		}
	})

	t.Run("no photo is a breach that is never cached and is looked up again", func(t *testing.T) {
		env, _ := mreqLookupEnv(t)
		user := mreqLookupUser(t)

		if got := mreqPhoto(env, user, memberReqLookupMode{}); got != memberReqBreach {
			t.Fatalf("verdict = %d, want breach", got)
		}
		if env.redis.Exists(memberReqPhotoKey(user)) {
			t.Fatal("a breach was cached")
		}
		if got := mreqPhoto(env, user, memberReqLookupMode{}); got != memberReqBreach {
			t.Fatalf("second verdict = %d, want breach", got)
		}
		if n := mreqLookupPhotoCalls(env); n != 2 {
			t.Errorf("Telegram requests = %d, want 2 (a breach is asked again)", n)
		}
	})

	t.Run("a non-empty list is a pass even when the total says zero", func(t *testing.T) {
		env, state := mreqLookupEnv(t)
		user := mreqLookupUser(t)
		state.set(user, mreqPhotoAnswer{Total: 0, List: true})
		if got := mreqPhoto(env, user, memberReqLookupMode{}); got != memberReqPass {
			t.Fatalf("verdict = %d, want pass", got)
		}
	})

	t.Run("a total above zero with an empty list is a pass", func(t *testing.T) {
		env, state := mreqLookupEnv(t)
		user := mreqLookupUser(t)
		state.set(user, mreqPhotoAnswer{Total: 3})
		if got := mreqPhoto(env, user, memberReqLookupMode{}); got != memberReqPass {
			t.Fatalf("verdict = %d, want pass", got)
		}
	})

	t.Run("the request carries the user ID and a limit of one and nothing else", func(t *testing.T) {
		env, state := mreqLookupEnv(t)
		user := mreqLookupUser(t)
		state.set(user, mreqPhotoAnswer{Total: 1})
		mreqPhoto(env, user, memberReqLookupMode{})
		calls := env.fake.callsFor("getUserProfilePhotos")
		if len(calls) != 1 {
			t.Fatalf("requests = %d, want 1", len(calls))
		}
		var keys []string
		for key := range calls[0].Params {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		if len(keys) != 2 || keys[0] != "limit" || keys[1] != "user_id" {
			t.Fatalf("request fields = %v, want exactly limit and user_id", keys)
		}
		if staffParamInt(calls[0].Params, "user_id") != user || staffParamInt(calls[0].Params, "limit") != 1 {
			t.Errorf("request = %v, want user_id %d and limit 1", calls[0].Params, user)
		}
	})

	// Every failure of the call is unknown: never a breach, never a cached key.
	failures := []struct {
		name  string
		setup func(t *testing.T, env *memberReqEnv, state *mreqLookupState, user int64)
		// wantCalls is the Telegram requests the failure costs.
		wantCalls int
	}{
		{"a plain error", func(_ *testing.T, _ *memberReqEnv, s *mreqLookupState, u int64) { s.script(u, errors.New("boom")) }, 1},
		{"a 400", func(_ *testing.T, _ *memberReqEnv, s *mreqLookupState, u int64) {
			s.script(u, staffFakeError(400, "Bad Request"))
		}, 1},
		{"a 403", func(_ *testing.T, _ *memberReqEnv, s *mreqLookupState, u int64) {
			s.script(u, staffFakeError(403, "Forbidden"))
		}, 1},
		{"a 500", func(_ *testing.T, _ *memberReqEnv, s *mreqLookupState, u int64) {
			s.script(u, staffFakeError(500, "Internal"))
		}, 1},
		{"a 429 that keeps coming after the pacer's one retry", func(_ *testing.T, _ *memberReqEnv, s *mreqLookupState, u int64) {
			s.script(u, staffFake429(1), staffFake429(1))
		}, 2},
		{"a null body", func(_ *testing.T, _ *memberReqEnv, s *mreqLookupState, u int64) {
			s.set(u, mreqPhotoAnswer{Null: true})
		}, 1},
		{"a timeout", func(t *testing.T, _ *memberReqEnv, s *mreqLookupState, u int64) {
			// The call hangs until the flight's own bound ends it.
			mreqSwap(t, &memberReqLookupFlightTimeout, 60*time.Millisecond)
			t.Cleanup(s.hold(u))
		}, 1},
		{"a pacer shed behind a long shared block", func(t *testing.T, e *memberReqEnv, _ *mreqLookupState, _ int64) {
			mreqSetKey(t, e, "fuku:memberreq:lkp:pace:block", time.Minute)
		}, 0},
	}
	for _, tc := range failures {
		t.Run(tc.name+" is unknown and caches nothing", func(t *testing.T) {
			env, state := mreqLookupEnv(t)
			user := mreqLookupUser(t)
			tc.setup(t, env, state, user)
			t.Cleanup(func() { env.redis.Del("fuku:memberreq:lkp:pace:block") })

			if got := mreqPhoto(env, user, memberReqLookupMode{}); got != memberReqUnknown {
				t.Fatalf("verdict = %d, want unknown", got)
			}
			if env.redis.Exists(memberReqPhotoKey(user)) {
				t.Error("an unknown answer was cached")
			}
			if n := mreqLookupPhotoCalls(env); n != tc.wantCalls {
				t.Errorf("Telegram requests = %d, want %d", n, tc.wantCalls)
			}
		})
	}

	t.Run("a cancelled context is unknown and asks nobody", func(t *testing.T) {
		env, state := mreqLookupEnv(t)
		user := mreqLookupUser(t)
		state.set(user, mreqPhotoAnswer{Total: 1})
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if got := memberReqPhotoRule(ctx, env.bot, env.chat.Id, user, memberReqLookupMode{}); got != memberReqUnknown {
			t.Fatalf("verdict = %d, want unknown", got)
		}
		if n := mreqLookupPhotoCalls(env); n != 0 {
			t.Errorf("Telegram requests = %d, want 0", n)
		}
	})

	t.Run("Fresh bypasses a cached pass, rewrites it on a pass and drops a stale one on a breach", func(t *testing.T) {
		env, state := mreqLookupEnv(t)
		withPhoto := mreqLookupUser(t)
		without := mreqLookupUser(t)
		state.set(withPhoto, mreqPhotoAnswer{Total: 2})

		// A stale positive for the member with no photo, a short-lived one for the other.
		for _, user := range []int64{withPhoto, without} {
			key := memberReqPhotoKey(user)
			mreqSetKey(t, env, key, time.Minute)
		}
		if got := mreqPhoto(env, without, memberReqLookupMode{}); got != memberReqPass {
			t.Fatalf("a cached positive without Fresh = %d, want pass", got)
		}
		if n := mreqLookupPhotoCalls(env); n != 0 {
			t.Fatalf("a cached positive without Fresh made %d requests, want 0", n)
		}

		if got := mreqPhoto(env, withPhoto, memberReqLookupMode{Fresh: true}); got != memberReqPass {
			t.Fatalf("Fresh with a photo = %d, want pass", got)
		}
		if n := mreqLookupPhotoCalls(env); n != 1 {
			t.Errorf("Fresh made %d requests, want 1 (the cache is bypassed)", n)
		}
		if ttl := env.redis.TTL(memberReqPhotoKey(withPhoto)); ttl < ttlLow {
			t.Errorf("TTL after a Fresh pass = %s, want the key rewritten to about 30 minutes", ttl)
		}

		if got := mreqPhoto(env, without, memberReqLookupMode{Fresh: true}); got != memberReqBreach {
			t.Fatalf("Fresh without a photo = %d, want breach", got)
		}
		if env.redis.Exists(memberReqPhotoKey(without)) {
			t.Error("a Fresh breach left the stale positive key")
		}
	})
}

// mreqLookupGo runs the photo rule for userID in a goroutine and returns the channel of its
// verdict. The caller waits on the channel before the test ends, so no flight outlives it.
func mreqLookupGo(e *memberReqEnv, userID int64, timeout time.Duration, mode memberReqLookupMode) <-chan memberReqVerdict {
	out := make(chan memberReqVerdict, 1)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	go func() {
		defer cancel()
		out <- memberReqPhotoRule(ctx, e.bot, e.chat.Id, userID, mode)
	}()
	return out
}

// mreqLookupAwaitCalls waits until the fake has seen want photo requests.
func mreqLookupAwaitCalls(t *testing.T, e *memberReqEnv, want int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for mreqLookupPhotoCalls(e) < want {
		if time.Now().After(deadline) {
			t.Fatalf("the fake saw %d photo requests, want %d", mreqLookupPhotoCalls(e), want)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// mreqVerdictWithin returns the verdict from ch or fails after d.
func mreqVerdictWithin(t *testing.T, ch <-chan memberReqVerdict, d time.Duration, what string) memberReqVerdict {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(d):
		t.Fatalf("%s: no verdict within %s", what, d)
		return memberReqUnknown
	}
}

func TestMemberReqLookupSingleflight(t *testing.T) {
	// A hundred callers behind a start barrier share one request. The fake holds the call open
	// until every caller has had time to join the flight, so the count does not depend on the
	// cache write winning a race.
	hundred := func(t *testing.T, answer mreqPhotoAnswer, want memberReqVerdict) {
		env, state := mreqLookupEnv(t)
		user := mreqLookupUser(t)
		state.set(user, answer)
		release := state.hold(user)
		t.Cleanup(release)

		start := make(chan struct{})
		results := make(chan memberReqVerdict, 100)
		for i := 0; i < 100; i++ {
			go func() {
				<-start
				results <- mreqPhoto(env, user, memberReqLookupMode{})
			}()
		}
		close(start)
		mreqLookupAwaitCalls(t, env, 1)
		time.Sleep(150 * time.Millisecond)
		release()
		for i := 0; i < 100; i++ {
			select {
			case got := <-results:
				if got != want {
					t.Fatalf("caller %d got verdict %d, want %d", i, got, want)
				}
			case <-time.After(5 * time.Second):
				t.Fatalf("only %d of 100 callers returned", i)
			}
		}
		if n := mreqLookupPhotoCalls(env); n != 1 {
			t.Fatalf("Telegram requests for 100 callers = %d, want 1", n)
		}
		// A call made after the flight ended: a pass is served from the cache, a breach
		// (never cached) asks again.
		mreqPhoto(env, user, memberReqLookupMode{})
		wantAfter := 1
		if want == memberReqBreach {
			wantAfter = 2
		}
		if n := mreqLookupPhotoCalls(env); n != wantAfter {
			t.Errorf("Telegram requests after a later call = %d, want %d", n, wantAfter)
		}
	}
	t.Run("100 callers of a member with a photo make one request", func(t *testing.T) {
		hundred(t, mreqPhotoAnswer{Total: 1}, memberReqPass)
	})
	t.Run("100 callers of a member without a photo make one request and a later call asks again", func(t *testing.T) {
		hundred(t, mreqPhotoAnswer{}, memberReqBreach)
	})

	t.Run("a follower's short deadline returns unknown without cancelling the shared lookup", func(t *testing.T) {
		env, state := mreqLookupEnv(t)
		user := mreqLookupUser(t)
		state.set(user, mreqPhotoAnswer{Total: 1})
		release := state.hold(user)
		t.Cleanup(release)

		leader := mreqLookupGo(env, user, 5*time.Second, memberReqLookupMode{})
		mreqLookupAwaitCalls(t, env, 1)

		began := time.Now()
		follower := mreqLookupGo(env, user, 50*time.Millisecond, memberReqLookupMode{})
		if got := mreqVerdictWithin(t, follower, 2*time.Second, "the follower"); got != memberReqUnknown {
			t.Fatalf("follower verdict = %d, want unknown", got)
		}
		if took := time.Since(began); took > time.Second {
			t.Errorf("the follower took %s, want about its own 50 ms deadline", took)
		}
		release()
		if got := mreqVerdictWithin(t, leader, 5*time.Second, "the leader"); got != memberReqPass {
			t.Fatalf("leader verdict = %d, want pass: the follower's deadline cancelled the shared call", got)
		}
		if n := mreqLookupPhotoCalls(env); n != 1 {
			t.Errorf("Telegram requests = %d, want 1 (the follower joined the flight)", n)
		}
	})

	t.Run("a caller whose own context is cancelled does not cancel the call another waits on", func(t *testing.T) {
		env, state := mreqLookupEnv(t)
		user := mreqLookupUser(t)
		state.set(user, mreqPhotoAnswer{Total: 1})
		release := state.hold(user)
		t.Cleanup(release)

		firstCtx, cancelFirst := context.WithCancel(context.Background())
		first := make(chan memberReqVerdict, 1)
		go func() { first <- memberReqPhotoRule(firstCtx, env.bot, env.chat.Id, user, memberReqLookupMode{}) }()
		mreqLookupAwaitCalls(t, env, 1)
		second := mreqLookupGo(env, user, 5*time.Second, memberReqLookupMode{})
		time.Sleep(50 * time.Millisecond)

		cancelFirst()
		if got := mreqVerdictWithin(t, first, 2*time.Second, "the cancelled caller"); got != memberReqUnknown {
			t.Fatalf("cancelled caller verdict = %d, want unknown", got)
		}
		release()
		if got := mreqVerdictWithin(t, second, 5*time.Second, "the waiting caller"); got != memberReqPass {
			t.Fatalf("waiting caller verdict = %d, want pass", got)
		}
	})

	t.Run("a flight function that panics returns unknown and the process survives", func(t *testing.T) {
		hook := captureLogs(t, log.DebugLevel)
		got := memberReqFlight(context.Background(), "t:panic", func(context.Context) memberReqVerdict {
			panic("scripted lookup panic")
		})
		if got != memberReqUnknown {
			t.Fatalf("verdict after a panic = %d, want unknown", got)
		}
		logged := false
		for _, entry := range hook.AllEntries() {
			if entry.Level == log.ErrorLevel && strings.Contains(entry.Message, "Recovered from panic") {
				logged = true
			}
		}
		if !logged {
			t.Error("the recovered panic was not logged at error level")
		}
		// The key is free again and a normal function on it works.
		if got := memberReqFlight(context.Background(), "t:panic", func(context.Context) memberReqVerdict { return memberReqPass }); got != memberReqPass {
			t.Errorf("verdict of the flight after the panic = %d, want pass", got)
		}
	})

	t.Run("a hung lookup ends with unknown when the shutdown cancels the root, and a fresh runner works", func(t *testing.T) {
		restoreMemberReqRunner(t)
		env, state := mreqLookupEnv(t)
		user := mreqLookupUser(t)
		state.set(user, mreqPhotoAnswer{Total: 1})
		release := state.hold(user)
		t.Cleanup(release)

		hung := mreqLookupGo(env, user, 10*time.Second, memberReqLookupMode{})
		mreqLookupAwaitCalls(t, env, 1)
		mreqSwap(t, &memberReqStopWait, 200*time.Millisecond)
		began := time.Now()
		StopMemberReqJobs()
		if got := mreqVerdictWithin(t, hung, 2*time.Second, "the hung lookup after the shutdown"); got != memberReqUnknown {
			t.Fatalf("verdict after the shutdown = %d, want unknown", got)
		}
		if took := time.Since(began); took > 2*time.Second {
			t.Errorf("the lookup took %s to end after the shutdown", took)
		}
		// A lookup made after the shutdown asks nobody.
		other := mreqLookupUser(t)
		state.set(other, mreqPhotoAnswer{Total: 1})
		if got := mreqPhoto(env, other, memberReqLookupMode{}); got != memberReqUnknown {
			t.Errorf("verdict after the shutdown = %d, want unknown", got)
		}
		if n := mreqLookupPhotoCalls(env); n != 1 {
			t.Errorf("Telegram requests = %d, want 1 (none after the shutdown)", n)
		}
	})

	t.Run("a lookup in a runner restored after a shutdown works", func(t *testing.T) {
		env, state := mreqLookupEnv(t)
		user := mreqLookupUser(t)
		state.set(user, mreqPhotoAnswer{Total: 1})
		if got := mreqPhoto(env, user, memberReqLookupMode{}); got != memberReqPass {
			t.Fatalf("verdict in a fresh runner = %d, want pass", got)
		}
	})
}

func TestMemberReqLookupPacer(t *testing.T) {
	nextKey, blockKey := "fuku:memberreq:lkp:pace:next", "fuku:memberreq:lkp:pace:block"
	phase13Keys := []string{"fuku:memberreq:pace:next", "fuku:memberreq:pace:block"}

	t.Run("the pacer uses exactly its own two Redis keys", func(t *testing.T) {
		if memberReqLookupPacerOptions.NextKey != nextKey || memberReqLookupPacerOptions.BlockKey != blockKey {
			t.Fatalf("lookup pacer keys = %q and %q, want %q and %q",
				memberReqLookupPacerOptions.NextKey, memberReqLookupPacerOptions.BlockKey, nextKey, blockKey)
		}
		env, _ := mreqLookupEnv(t)
		user := mreqLookupUser(t)
		mreqPhoto(env, user, memberReqLookupMode{})
		if !env.redis.Exists(nextKey) {
			t.Errorf("a lookup did not write %s", nextKey)
		}
		for _, key := range phase13Keys {
			if env.redis.Exists(key) {
				t.Errorf("a lookup wrote the Phase 13 key %s", key)
			}
		}
		env.redis.FlushAll()
		if err := memberReqPaced(context.Background(), func(context.Context) error { return nil }); err != nil {
			t.Fatalf("memberReqPaced: %v", err)
		}
		if !env.redis.Exists(phase13Keys[0]) {
			t.Errorf("memberReqPaced did not write %s", phase13Keys[0])
		}
		if env.redis.Exists(nextKey) || env.redis.Exists(blockKey) {
			t.Error("a Phase 13 call wrote a lookup pacer key")
		}
	})

	t.Run("two replicas space ten parallel lookups across both", func(t *testing.T) {
		env, state := mreqLookupEnv(t)
		replica := env.mreqReplica()
		mreqInstallLookupFake(replica, state)
		opts := memberReqLookupPacerOptions
		opts.Interval = 50 * time.Millisecond
		pacers := []*ratelimit.TelegramPacer{ratelimit.NewTelegramPacer(opts), ratelimit.NewTelegramPacer(opts)}
		bots := []*memberReqEnv{env, replica}

		var wg sync.WaitGroup
		for i := 0; i < 10; i++ {
			user := mreqLookupUser(t)
			wg.Add(1)
			go func() {
				defer wg.Done()
				replicaIdx := i % 2
				err := pacers[replicaIdx].Do(context.Background(), func(ctx context.Context) error {
					_, callErr := bots[replicaIdx].bot.GetUserProfilePhotosWithContext(ctx, user, nil)
					return callErr
				})
				if err != nil {
					t.Errorf("lookup %d: %v", i, err)
				}
			}()
		}
		wg.Wait()
		arrivals := state.arrivals()
		if len(arrivals) != 10 {
			t.Fatalf("requests = %d, want 10", len(arrivals))
		}
		sort.Slice(arrivals, func(a, b int) bool { return arrivals[a].At.Before(arrivals[b].At) })
		for i := 1; i < len(arrivals); i++ {
			if gap := arrivals[i].At.Sub(arrivals[i-1].At); gap < 25*time.Millisecond {
				t.Errorf("requests %d and %d arrived %s apart across both replicas, want about 50 ms", i-1, i, gap)
			}
		}
		if span := arrivals[9].At.Sub(arrivals[0].At); span < 360*time.Millisecond {
			t.Errorf("ten requests spanned %s, want at least 9 intervals of about 50 ms", span)
		}
	})

	t.Run("a 429 on one replica blocks the other for its retry_after", func(t *testing.T) {
		env, state := mreqLookupEnv(t)
		replica := env.mreqReplica()
		mreqInstallLookupFake(replica, state)
		opts := memberReqLookupPacerOptions
		opts.Interval = time.Millisecond
		opts.RetryAfterUnit = 200 * time.Millisecond
		pacerA, pacerB := ratelimit.NewTelegramPacer(opts), ratelimit.NewTelegramPacer(opts)
		first, second := mreqLookupUser(t), mreqLookupUser(t)
		state.script(first, staffFake429(1))

		done := make(chan error, 1)
		go func() {
			done <- pacerA.Do(context.Background(), func(ctx context.Context) error {
				_, err := env.bot.GetUserProfilePhotosWithContext(ctx, first, nil)
				return err
			})
		}()
		mreqLookupAwaitCalls(t, env, 1)
		deadline := time.Now().Add(2 * time.Second)
		for !env.redis.Exists(blockKey) {
			if time.Now().After(deadline) {
				t.Fatal("the 429 did not write the shared block")
			}
			time.Sleep(time.Millisecond)
		}
		if err := pacerB.Do(context.Background(), func(ctx context.Context) error {
			_, err := replica.bot.GetUserProfilePhotosWithContext(ctx, second, nil)
			return err
		}); err != nil {
			t.Fatalf("the other replica's lookup: %v", err)
		}
		if err := <-done; err != nil {
			t.Fatalf("the first replica's lookup after its retry: %v", err)
		}
		var firstAt, secondAt time.Time
		for _, a := range state.arrivals() {
			switch {
			case a.User == first && firstAt.IsZero():
				firstAt = a.At
			case a.User == second:
				secondAt = a.At
			}
		}
		if gap := secondAt.Sub(firstAt); gap < 150*time.Millisecond {
			t.Errorf("the other replica's request came %s after the 429, want it held back for about 200 ms", gap)
		}
	})

	t.Run("a slot more than MaxWait away is shed as rate limited with no request and no slot taken", func(t *testing.T) {
		env, state := mreqLookupEnv(t)
		user := mreqLookupUser(t)
		mreqSetKey(t, env, blockKey, 30*time.Second)
		t.Cleanup(func() { env.redis.Del(blockKey) })

		err := memberReqLookupPaced(context.Background(), func(ctx context.Context) error {
			_, callErr := env.bot.GetUserProfilePhotosWithContext(ctx, user, nil)
			return callErr
		})
		if !errors.Is(err, ratelimit.ErrRateLimited) {
			t.Fatalf("error = %v, want ratelimit.ErrRateLimited", err)
		}
		if got := mreqPhoto(env, user, memberReqLookupMode{}); got != memberReqUnknown {
			t.Errorf("verdict of a shed lookup = %d, want unknown", got)
		}
		if n := len(state.arrivals()); n != 0 {
			t.Errorf("requests = %d, want 0 (a shed lookup makes no Telegram request)", n)
		}
		if env.redis.Exists(nextKey) {
			t.Error("a shed lookup took a slot")
		}
	})
}

func TestMemberReqLookupRedisDown(t *testing.T) {
	env, state := mreqLookupEnv(t)
	withPhoto, without := mreqLookupUser(t), mreqLookupUser(t)
	state.set(withPhoto, mreqPhotoAnswer{Total: 1})

	restore := cache.DisableRedisForTest()
	t.Cleanup(restore)
	mreqSwap(t, &memberReqRedisClient, func() *redis.Client { return nil })
	opts := memberReqLookupPacerOptions
	opts.Interval = 40 * time.Millisecond
	mreqSwap(t, &memberReqLookupPacer, ratelimit.NewTelegramPacer(opts))

	if got := mreqPhoto(env, withPhoto, memberReqLookupMode{}); got != memberReqPass {
		t.Fatalf("a member with a photo without Redis = %d, want pass", got)
	}
	if got := mreqPhoto(env, without, memberReqLookupMode{}); got != memberReqBreach {
		t.Fatalf("a member without a photo without Redis = %d, want breach", got)
	}
	// Nothing was cached, so the same member is asked again.
	if got := mreqPhoto(env, withPhoto, memberReqLookupMode{}); got != memberReqPass {
		t.Fatalf("second lookup without Redis = %d, want pass", got)
	}
	if n := mreqLookupPhotoCalls(env); n != 3 {
		t.Errorf("Telegram requests = %d, want 3 (every lookup is a cache miss)", n)
	}
	for _, key := range env.redis.Keys() {
		if strings.HasPrefix(key, "fuku:memberreq:") {
			t.Errorf("a lookup without Redis wrote %s", key)
		}
	}
	// The pacer fell back to its local interval: consecutive requests are spaced by it.
	arrivals := state.arrivals()
	for i := 1; i < len(arrivals); i++ {
		if gap := arrivals[i].At.Sub(arrivals[i-1].At); gap < 25*time.Millisecond {
			t.Errorf("requests %d and %d arrived %s apart, want the local interval of about 40 ms", i-1, i, gap)
		}
	}
}
