//go:build testtools

package modules

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
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

	// The channel families: a hundred callers of one key make one request, hold the call open
	// until every caller has had time to join the flight, and then all get the same answer.
	channelFamily := func(t *testing.T, name string, hold func(s *mreqLookupState, group, channel, user int64) string,
		prepare func(s *mreqLookupState, channel, user int64),
		call func(e *memberReqEnv, channel, user int64) any, calls func(e *memberReqEnv, channel, user int64) int,
		want func(channel int64) any) {
		t.Run("100 callers of "+name+" make one request", func(t *testing.T) {
			env, state := mreqLookupEnv(t)
			channel := mreqChannel(t, state, env.chat.Id, gotgbot.ChatMemberStatusAdministrator)
			user := mreqLookupUser(t)
			prepare(state, channel, user)
			release := state.holdKey(hold(state, env.chat.Id, channel, user))
			t.Cleanup(release)

			start := make(chan struct{})
			results := make(chan any, 100)
			for i := 0; i < 100; i++ {
				go func() {
					<-start
					results <- call(env, channel, user)
				}()
			}
			close(start)
			deadline := time.Now().Add(5 * time.Second)
			for calls(env, channel, user) < 1 {
				if time.Now().After(deadline) {
					t.Fatal("the fake saw no request")
				}
				time.Sleep(2 * time.Millisecond)
			}
			time.Sleep(150 * time.Millisecond)
			release()
			for i := 0; i < 100; i++ {
				select {
				case got := <-results:
					if got != want(channel) {
						t.Fatalf("caller %d got %v, want %v", i, got, want(channel))
					}
				case <-time.After(5 * time.Second):
					t.Fatalf("only %d of 100 callers returned", i)
				}
			}
			if n := calls(env, channel, user); n != 1 {
				t.Fatalf("requests for 100 callers = %d, want 1", n)
			}
			// A call after the flight: a cached answer asks nobody.
			call(env, channel, user)
			if n := calls(env, channel, user); n != 1 {
				t.Errorf("requests after a later call = %d, want 1", n)
			}
		})
	}
	noPrepare := func(*mreqLookupState, int64, int64) {}
	channelFamily(t, "the linked channel (l:<chat>)",
		func(_ *mreqLookupState, group, _, _ int64) string { return mreqChannelKey("getChat", group, 0) },
		noPrepare,
		func(e *memberReqEnv, _, _ int64) any {
			id, known := mreqLinked(e, memberReqLookupMode{})
			return fmt.Sprintf("%d/%v", id, known)
		},
		func(e *memberReqEnv, _, _ int64) int { return mreqChatCalls(e, e.chat.Id) },
		func(channel int64) any { return fmt.Sprintf("%d/true", channel) })
	channelFamily(t, "a subscription (s:<channel>:<user>)",
		func(_ *mreqLookupState, _, channel, user int64) string {
			return mreqChannelKey("getChatMember", channel, user)
		},
		func(s *mreqLookupState, channel, user int64) {
			s.setMember(channel, user, mreqChannelMember{Status: gotgbot.ChatMemberStatusMember})
		},
		func(e *memberReqEnv, _, user int64) any { return mreqChannelVerdict(e, user, memberReqLookupMode{}) },
		func(e *memberReqEnv, channel, user int64) int { return mreqMemberCalls(e, channel, user) },
		func(int64) any { return memberReqPass })
	channelFamily(t, "the bot's status (b:<channel>)",
		func(_ *mreqLookupState, _, channel, _ int64) string {
			return mreqChannelKey("getChatMember", channel, staffTestBotID)
		},
		noPrepare,
		func(e *memberReqEnv, channel, _ int64) any { return mreqBotStatus(e, channel, memberReqLookupMode{}) },
		func(e *memberReqEnv, channel, _ int64) int { return mreqMemberCalls(e, channel, staffTestBotID) },
		func(int64) any { return memberReqBotAdmin })
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

// mreqLinked runs the linked-channel lookup of the environment's group with a 5 s deadline.
func mreqLinked(e *memberReqEnv, mode memberReqLookupMode) (int64, bool) {
	e.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return memberReqLinkedChannel(ctx, e.bot, e.chat.Id, mode)
}

// mreqChannelVerdict runs the channel rule for userID in the environment's group with a 5 s
// deadline.
func mreqChannelVerdict(e *memberReqEnv, userID int64, mode memberReqLookupMode) memberReqVerdict {
	e.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return memberReqChannelRule(ctx, e.bot, e.chat.Id, userID, mode)
}

func TestMemberReqLinkedChannel(t *testing.T) {
	t.Run("the link is read from getChat of the group, cached for 10 minutes and served from the cache", func(t *testing.T) {
		env, state := mreqLookupEnv(t)
		channel := mreqChannel(t, state, env.chat.Id, gotgbot.ChatMemberStatusAdministrator)

		got, known := mreqLinked(env, memberReqLookupMode{})
		if !known || got != channel {
			t.Fatalf("linked channel = (%d, %v), want (%d, true)", got, known, channel)
		}
		key := memberReqLinkedKey(env.chat.Id)
		if key != "fuku:memberreq:lc:"+strconv.FormatInt(env.chat.Id, 10) {
			t.Fatalf("key = %q, want fuku:memberreq:lc:<chat>", key)
		}
		if value, err := env.redis.Get(key); err != nil || value != strconv.FormatInt(channel, 10) {
			t.Fatalf("cached value = %q (%v), want the decimal channel ID", value, err)
		}
		if ttl := env.redis.TTL(key); ttl < 9*time.Minute || ttl > 10*time.Minute {
			t.Errorf("TTL = %s, want about 10 minutes", ttl)
		}
		if again, ok := mreqLinked(env, memberReqLookupMode{}); !ok || again != channel {
			t.Fatalf("second answer = (%d, %v), want (%d, true)", again, ok, channel)
		}
		if n := mreqChatCalls(env, env.chat.Id); n != 1 {
			t.Errorf("getChat requests = %d, want 1 (the second call is served from the cache)", n)
		}
	})

	t.Run("the request is getChat for the group's own ID and carries nothing else", func(t *testing.T) {
		env, state := mreqLookupEnv(t)
		mreqChannel(t, state, env.chat.Id, gotgbot.ChatMemberStatusAdministrator)
		mreqLinked(env, memberReqLookupMode{})
		calls := env.fake.callsFor("getChat")
		if len(calls) != 1 {
			t.Fatalf("getChat requests = %d, want 1", len(calls))
		}
		if len(calls[0].Params) != 1 || staffParamInt(calls[0].Params, "chat_id") != env.chat.Id {
			t.Errorf("request = %v, want exactly chat_id %d", calls[0].Params, env.chat.Id)
		}
	})

	t.Run("a group with no linked channel is none, cached as 0 and served from the cache", func(t *testing.T) {
		env, state := mreqLookupEnv(t)
		state.link(env.chat.Id, 0)
		mreqLookupCleanup(t, nil, []int64{env.chat.Id})

		got, known := mreqLinked(env, memberReqLookupMode{})
		if !known || got != 0 {
			t.Fatalf("linked channel = (%d, %v), want (0, true)", got, known)
		}
		key := memberReqLinkedKey(env.chat.Id)
		if value, err := env.redis.Get(key); err != nil || value != "0" {
			t.Fatalf("cached value = %q (%v), want 0", value, err)
		}
		mreqLinked(env, memberReqLookupMode{})
		if n := mreqChatCalls(env, env.chat.Id); n != 1 {
			t.Errorf("getChat requests = %d, want 1", n)
		}
	})

	t.Run("a getChat error is not known and writes nothing", func(t *testing.T) {
		env, state := mreqLookupEnv(t)
		mreqChannel(t, state, env.chat.Id, gotgbot.ChatMemberStatusAdministrator)
		state.scriptChat(env.chat.Id, errors.New("boom"))
		if got, known := mreqLinked(env, memberReqLookupMode{}); known || got != 0 {
			t.Fatalf("linked channel = (%d, %v), want (0, false)", got, known)
		}
		if env.redis.Exists(memberReqLinkedKey(env.chat.Id)) {
			t.Error("an error was cached")
		}
	})

	t.Run("a null answer is not known and writes nothing", func(t *testing.T) {
		env, state := mreqLookupEnv(t)
		mreqChannel(t, state, env.chat.Id, gotgbot.ChatMemberStatusAdministrator)
		state.nullGetChat(env.chat.Id)
		if got, known := mreqLinked(env, memberReqLookupMode{}); known || got != 0 {
			t.Fatalf("linked channel = (%d, %v), want (0, false)", got, known)
		}
		if env.redis.Exists(memberReqLinkedKey(env.chat.Id)) {
			t.Error("a null answer was cached")
		}
	})

	t.Run("a positive linked_chat_id reads as none and is not cached", func(t *testing.T) {
		env, state := mreqLookupEnv(t)
		state.link(env.chat.Id, 424242)
		mreqLookupCleanup(t, nil, []int64{env.chat.Id})
		if got, known := mreqLinked(env, memberReqLookupMode{}); !known || got != 0 {
			t.Fatalf("linked channel = (%d, %v), want (0, true)", got, known)
		}
		if env.redis.Exists(memberReqLinkedKey(env.chat.Id)) {
			t.Error("a positive linked_chat_id was cached")
		}
	})

	failures := []struct {
		name  string
		setup func(t *testing.T, env *memberReqEnv, state *mreqLookupState)
	}{
		{"a 403", func(_ *testing.T, e *memberReqEnv, s *mreqLookupState) {
			s.scriptChat(e.chat.Id, staffFakeError(403, "Forbidden"))
		}},
		{"a 429 that keeps coming", func(_ *testing.T, e *memberReqEnv, s *mreqLookupState) {
			s.scriptChat(e.chat.Id, staffFake429(1), staffFake429(1))
		}},
		{"a timeout", func(t *testing.T, e *memberReqEnv, s *mreqLookupState) {
			mreqSwap(t, &memberReqLookupFlightTimeout, 60*time.Millisecond)
			t.Cleanup(s.holdKey(mreqChannelKey("getChat", e.chat.Id, 0)))
		}},
		{"a pacer shed", func(t *testing.T, e *memberReqEnv, _ *mreqLookupState) {
			mreqSetKey(t, e, "fuku:memberreq:lkp:pace:block", time.Minute)
			t.Cleanup(func() { e.redis.Del("fuku:memberreq:lkp:pace:block") })
		}},
	}
	for _, tc := range failures {
		t.Run(tc.name+" is not known and writes nothing", func(t *testing.T) {
			env, state := mreqLookupEnv(t)
			mreqChannel(t, state, env.chat.Id, gotgbot.ChatMemberStatusAdministrator)
			tc.setup(t, env, state)
			if got, known := mreqLinked(env, memberReqLookupMode{}); known || got != 0 {
				t.Fatalf("linked channel = (%d, %v), want (0, false)", got, known)
			}
			if env.redis.Exists(memberReqLinkedKey(env.chat.Id)) {
				t.Error("a failure was cached")
			}
		})
	}

	t.Run("a cancelled context is not known and asks nobody", func(t *testing.T) {
		env, state := mreqLookupEnv(t)
		mreqChannel(t, state, env.chat.Id, gotgbot.ChatMemberStatusAdministrator)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, known := memberReqLinkedChannel(ctx, env.bot, env.chat.Id, memberReqLookupMode{}); known {
			t.Fatal("a cancelled context was known")
		}
		if n := mreqChatCalls(env, env.chat.Id); n != 0 {
			t.Errorf("getChat requests = %d, want 0", n)
		}
	})

	t.Run("Fresh bypasses a cached link and refreshes it, and an unreadable cached value is a miss", func(t *testing.T) {
		env, state := mreqLookupEnv(t)
		channel := mreqChannel(t, state, env.chat.Id, gotgbot.ChatMemberStatusAdministrator)
		key := memberReqLinkedKey(env.chat.Id)
		if err := env.redis.Set(key, "0"); err != nil {
			t.Fatal(err)
		}
		if got, known := mreqLinked(env, memberReqLookupMode{}); !known || got != 0 {
			t.Fatalf("cached none = (%d, %v), want (0, true)", got, known)
		}
		if n := mreqChatCalls(env, env.chat.Id); n != 0 {
			t.Fatalf("getChat requests from the cache = %d, want 0", n)
		}
		if got, known := mreqLinked(env, memberReqLookupMode{Fresh: true}); !known || got != channel {
			t.Fatalf("Fresh = (%d, %v), want (%d, true)", got, known, channel)
		}
		if value, err := env.redis.Get(key); err != nil || value != strconv.FormatInt(channel, 10) {
			t.Errorf("refreshed value = %q (%v), want the channel ID", value, err)
		}
		for _, junk := range []string{"abc", "7"} {
			if err := env.redis.Set(key, junk); err != nil {
				t.Fatal(err)
			}
			before := mreqChatCalls(env, env.chat.Id)
			if got, known := mreqLinked(env, memberReqLookupMode{}); !known || got != channel {
				t.Fatalf("junk %q = (%d, %v), want (%d, true)", junk, got, known, channel)
			}
			if mreqChatCalls(env, env.chat.Id) != before+1 {
				t.Errorf("junk %q was served from the cache, want a miss", junk)
			}
		}
	})
}

func TestMemberReqChannelRule(t *testing.T) {
	t.Run("a subscribed member passes, is cached for 30 minutes and the second call asks nobody", func(t *testing.T) {
		env, state := mreqLookupEnv(t)
		channel := mreqChannel(t, state, env.chat.Id, gotgbot.ChatMemberStatusAdministrator)
		user := mreqLookupUser(t)
		state.setMember(channel, user, mreqChannelMember{Status: gotgbot.ChatMemberStatusMember})

		if got := mreqChannelVerdict(env, user, memberReqLookupMode{}); got != memberReqPass {
			t.Fatalf("verdict = %d, want pass", got)
		}
		key := memberReqSubKey(channel, user)
		if !env.redis.Exists(key) {
			t.Fatalf("key %s was not written for a pass", key)
		}
		if ttl := env.redis.TTL(key); ttl < 29*time.Minute || ttl > 30*time.Minute {
			t.Errorf("TTL = %s, want about 30 minutes", ttl)
		}
		if got := mreqChannelVerdict(env, user, memberReqLookupMode{}); got != memberReqPass {
			t.Fatalf("second verdict = %d, want pass", got)
		}
		if n := mreqMemberCalls(env, channel, user); n != 1 {
			t.Errorf("member requests = %d, want 1 (the second call is served from the cache)", n)
		}
	})

	t.Run("a member who left is a breach when the bot administers the channel, and nothing is cached for them", func(t *testing.T) {
		env, state := mreqLookupEnv(t)
		channel := mreqChannel(t, state, env.chat.Id, gotgbot.ChatMemberStatusAdministrator)
		user := mreqLookupUser(t)
		state.setMember(channel, user, mreqChannelMember{Status: gotgbot.ChatMemberStatusLeft})

		if got := mreqChannelVerdict(env, user, memberReqLookupMode{}); got != memberReqBreach {
			t.Fatalf("verdict = %d, want breach", got)
		}
		if env.redis.Exists(memberReqSubKey(channel, user)) {
			t.Error("a breach was cached as a subscription")
		}
	})

	t.Run("the same left answer is inert when the bot is not an administrator of the channel", func(t *testing.T) {
		env, state := mreqLookupEnv(t)
		channel := mreqChannel(t, state, env.chat.Id, gotgbot.ChatMemberStatusMember)
		user := mreqLookupUser(t)
		state.setMember(channel, user, mreqChannelMember{Status: gotgbot.ChatMemberStatusLeft})

		if got := mreqChannelVerdict(env, user, memberReqLookupMode{}); got != memberReqInert {
			t.Fatalf("verdict = %d, want inert", got)
		}
		if env.redis.Exists(memberReqSubKey(channel, user)) || env.redis.Exists(memberReqBotAdminKey(channel)) {
			t.Error("an inert verdict wrote a cache key")
		}
	})

	t.Run("a group with no linked channel is inert and makes no member request at all", func(t *testing.T) {
		env, state := mreqLookupEnv(t)
		state.link(env.chat.Id, 0)
		mreqLookupCleanup(t, nil, []int64{env.chat.Id})
		user := mreqLookupUser(t)

		if got := mreqChannelVerdict(env, user, memberReqLookupMode{}); got != memberReqInert {
			t.Fatalf("verdict = %d, want inert", got)
		}
		if n := len(env.fake.callsFor("getChatMember")); n != 0 {
			t.Errorf("getChatMember requests = %d, want 0", n)
		}
	})

	t.Run("a subscribed answer passes and anything else that says not subscribed is a breach only with an administrator bot", func(t *testing.T) {
		answers := []struct {
			name   string
			answer mreqChannelMember
			want   memberReqVerdict // with an administrator bot
		}{
			{"creator", mreqChannelMember{Status: gotgbot.ChatMemberStatusCreator}, memberReqPass},
			{"administrator", mreqChannelMember{Status: gotgbot.ChatMemberStatusAdministrator}, memberReqPass},
			{"member", mreqChannelMember{Status: gotgbot.ChatMemberStatusMember}, memberReqPass},
			{"restricted with is_member", mreqChannelMember{Status: gotgbot.ChatMemberStatusRestricted, IsMember: true}, memberReqPass},
			{"restricted without is_member", mreqChannelMember{Status: gotgbot.ChatMemberStatusRestricted}, memberReqBreach},
			{"left", mreqChannelMember{Status: gotgbot.ChatMemberStatusLeft}, memberReqBreach},
			{"kicked", mreqChannelMember{Status: gotgbot.ChatMemberStatusKicked}, memberReqBreach},
		}
		for _, tc := range answers {
			env, state := mreqLookupEnv(t)
			channel := mreqChannel(t, state, env.chat.Id, gotgbot.ChatMemberStatusAdministrator)
			user := mreqLookupUser(t)
			state.setMember(channel, user, tc.answer)
			if got := mreqChannelVerdict(env, user, memberReqLookupMode{}); got != tc.want {
				t.Errorf("%s with an administrator bot: verdict = %d, want %d", tc.name, got, tc.want)
			}
			if cached := env.redis.Exists(memberReqSubKey(channel, user)); cached != (tc.want == memberReqPass) {
				t.Errorf("%s: subscription key cached = %v, want %v (a pass only)", tc.name, cached, tc.want == memberReqPass)
			}

			// The same answer to a bot that is not an administrator is never a breach.
			state.setMember(channel, staffTestBotID, mreqChannelMember{Status: gotgbot.ChatMemberStatusMember})
			env.redis.Del(memberReqBotAdminKey(channel))
			env.redis.Del(memberReqSubKey(channel, user))
			wantInert := tc.want
			if tc.want == memberReqBreach {
				wantInert = memberReqInert
			}
			if got := mreqChannelVerdict(env, user, memberReqLookupMode{}); got != wantInert {
				t.Errorf("%s with a member bot: verdict = %d, want %d", tc.name, got, wantInert)
			}
		}
	})

	// Every member-lookup failure is unknown, caches nothing and never asks about the bot's
	// standing: the bot here is an administrator, so a candidate would be a breach.
	failures := []struct {
		name  string
		setup func(t *testing.T, env *memberReqEnv, state *mreqLookupState, channel, user int64)
	}{
		{"a plain error", func(_ *testing.T, _ *memberReqEnv, s *mreqLookupState, c, u int64) {
			s.scriptMember(c, u, errors.New("boom"))
		}},
		{"a 400", func(_ *testing.T, _ *memberReqEnv, s *mreqLookupState, c, u int64) {
			s.scriptMember(c, u, staffFakeError(400, "Bad Request: PARTICIPANT_ID_INVALID"))
		}},
		{"a 403", func(_ *testing.T, _ *memberReqEnv, s *mreqLookupState, c, u int64) {
			s.scriptMember(c, u, staffFakeError(403, "Forbidden: bot was kicked from the channel chat"))
		}},
		{"a 429 that keeps coming", func(_ *testing.T, _ *memberReqEnv, s *mreqLookupState, c, u int64) {
			s.scriptMember(c, u, staffFake429(1), staffFake429(1))
		}},
		{"a 500", func(_ *testing.T, _ *memberReqEnv, s *mreqLookupState, c, u int64) {
			s.scriptMember(c, u, staffFakeError(500, "Internal Server Error"))
		}},
		{"a timeout", func(t *testing.T, _ *memberReqEnv, s *mreqLookupState, c, u int64) {
			mreqSwap(t, &memberReqLookupFlightTimeout, 60*time.Millisecond)
			t.Cleanup(s.holdKey(mreqChannelKey("getChatMember", c, u)))
		}},
		{"a pacer shed", func(t *testing.T, e *memberReqEnv, _ *mreqLookupState, _, _ int64) {
			// The link is cached first, so the shed hits the member lookup.
			if _, known := mreqLinked(e, memberReqLookupMode{}); !known {
				t.Fatal("the linked channel was not read")
			}
			mreqSetKey(t, e, "fuku:memberreq:lkp:pace:block", time.Minute)
			t.Cleanup(func() { e.redis.Del("fuku:memberreq:lkp:pace:block") })
		}},
		{"a null answer", func(_ *testing.T, _ *memberReqEnv, s *mreqLookupState, c, u int64) {
			s.setMember(c, u, mreqChannelMember{Null: true})
		}},
		{"an unrecognised status", func(_ *testing.T, _ *memberReqEnv, s *mreqLookupState, c, u int64) {
			s.setMember(c, u, mreqChannelMember{Status: "something_new"})
		}},
	}
	for _, tc := range failures {
		t.Run(tc.name+" is unknown, caches nothing and never reads the bot's status", func(t *testing.T) {
			env, state := mreqLookupEnv(t)
			channel := mreqChannel(t, state, env.chat.Id, gotgbot.ChatMemberStatusAdministrator)
			user := mreqLookupUser(t)
			tc.setup(t, env, state, channel, user)

			if got := mreqChannelVerdict(env, user, memberReqLookupMode{}); got != memberReqUnknown {
				t.Fatalf("verdict = %d, want unknown", got)
			}
			if env.redis.Exists(memberReqSubKey(channel, user)) || env.redis.Exists(memberReqBotAdminKey(channel)) {
				t.Error("an unknown answer wrote a cache key")
			}
			if n := mreqMemberCalls(env, channel, staffTestBotID); n != 0 {
				t.Errorf("bot status requests = %d, want 0 (no extra call without Classify)", n)
			}
		})
	}

	t.Run("a cancelled context is unknown and asks nobody", func(t *testing.T) {
		env, state := mreqLookupEnv(t)
		channel := mreqChannel(t, state, env.chat.Id, gotgbot.ChatMemberStatusAdministrator)
		user := mreqLookupUser(t)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if got := memberReqChannelRule(ctx, env.bot, env.chat.Id, user, memberReqLookupMode{}); got != memberReqUnknown {
			t.Fatalf("verdict = %d, want unknown", got)
		}
		if n := len(env.fake.callsFor("getChat")) + mreqMemberCalls(env, channel, user); n != 0 {
			t.Errorf("requests = %d, want 0", n)
		}
	})

	t.Run("an unreadable link is unknown and asks about no member", func(t *testing.T) {
		env, state := mreqLookupEnv(t)
		channel := mreqChannel(t, state, env.chat.Id, gotgbot.ChatMemberStatusAdministrator)
		user := mreqLookupUser(t)
		state.scriptChat(env.chat.Id, staffFakeError(500, "Internal Server Error"))
		if got := mreqChannelVerdict(env, user, memberReqLookupMode{}); got != memberReqUnknown {
			t.Fatalf("verdict = %d, want unknown", got)
		}
		if n := mreqMemberCalls(env, channel, user); n != 0 {
			t.Errorf("member requests = %d, want 0", n)
		}
	})

	t.Run("the bot's status is read only for a candidate", func(t *testing.T) {
		env, state := mreqLookupEnv(t)
		channel := mreqChannel(t, state, env.chat.Id, gotgbot.ChatMemberStatusAdministrator)
		passing, leaving := mreqLookupUser(t), mreqLookupUser(t)
		state.setMember(channel, passing, mreqChannelMember{Status: gotgbot.ChatMemberStatusMember})
		state.setMember(channel, leaving, mreqChannelMember{Status: gotgbot.ChatMemberStatusLeft})
		mreqChannelVerdict(env, passing, memberReqLookupMode{})
		if n := mreqMemberCalls(env, channel, staffTestBotID); n != 0 {
			t.Fatalf("bot status requests after a pass = %d, want 0", n)
		}
		mreqChannelVerdict(env, leaving, memberReqLookupMode{})
		if n := mreqMemberCalls(env, channel, staffTestBotID); n != 1 {
			t.Errorf("bot status requests after a candidate = %d, want 1", n)
		}
	})

	t.Run("Classify makes a lost lookup inert for a bot that is no administrator and nothing else", func(t *testing.T) {
		unanswerable := []struct {
			name  string
			setup func(s *mreqLookupState, c, u int64)
		}{
			{"a plain error", func(s *mreqLookupState, c, u int64) { s.scriptMember(c, u, errors.New("boom")) }},
			{"a null answer", func(s *mreqLookupState, c, u int64) { s.setMember(c, u, mreqChannelMember{Null: true}) }},
			{"an unrecognised status", func(s *mreqLookupState, c, u int64) { s.setMember(c, u, mreqChannelMember{Status: "something_new"}) }},
		}
		bots := []struct {
			name string
			set  func(t *testing.T, s *mreqLookupState, c int64)
			want memberReqVerdict
		}{
			{"member", func(_ *testing.T, s *mreqLookupState, c int64) {
				s.setMember(c, staffTestBotID, mreqChannelMember{Status: gotgbot.ChatMemberStatusMember})
			}, memberReqInert},
			{"left", func(_ *testing.T, s *mreqLookupState, c int64) {
				s.setMember(c, staffTestBotID, mreqChannelMember{Status: gotgbot.ChatMemberStatusLeft})
			}, memberReqInert},
			{"kicked", func(_ *testing.T, s *mreqLookupState, c int64) {
				s.setMember(c, staffTestBotID, mreqChannelMember{Status: gotgbot.ChatMemberStatusKicked})
			}, memberReqInert},
			{"restricted", func(_ *testing.T, s *mreqLookupState, c int64) {
				s.setMember(c, staffTestBotID, mreqChannelMember{Status: gotgbot.ChatMemberStatusRestricted})
			}, memberReqInert},
			{"a 400", func(_ *testing.T, s *mreqLookupState, c int64) {
				s.scriptMember(c, staffTestBotID, staffFakeError(400, "Bad Request: chat not found"))
			}, memberReqInert},
			{"a 403", func(_ *testing.T, s *mreqLookupState, c int64) {
				s.scriptMember(c, staffTestBotID, staffFakeError(403, "Forbidden: bot was kicked"))
			}, memberReqInert},
			{"an administrator", func(_ *testing.T, s *mreqLookupState, c int64) {
				s.setMember(c, staffTestBotID, mreqChannelMember{Status: gotgbot.ChatMemberStatusAdministrator})
			}, memberReqUnknown},
			{"a 429", func(_ *testing.T, s *mreqLookupState, c int64) {
				s.scriptMember(c, staffTestBotID, staffFake429(1), staffFake429(1))
			}, memberReqUnknown},
			{"a 500", func(_ *testing.T, s *mreqLookupState, c int64) {
				s.scriptMember(c, staffTestBotID, staffFakeError(500, "Internal"))
			}, memberReqUnknown},
			{"a timeout", func(t *testing.T, s *mreqLookupState, c int64) {
				mreqSwap(t, &memberReqLookupFlightTimeout, 60*time.Millisecond)
				t.Cleanup(s.holdKey(mreqChannelKey("getChatMember", c, staffTestBotID)))
			}, memberReqUnknown},
		}
		for _, lost := range unanswerable {
			for _, bot := range bots {
				t.Run(lost.name+" with a bot that is "+bot.name, func(t *testing.T) {
					env, state := mreqLookupEnv(t)
					channel := mreqChannel(t, state, env.chat.Id, gotgbot.ChatMemberStatusAdministrator)
					user := mreqLookupUser(t)
					lost.setup(state, channel, user)
					bot.set(t, state, channel)
					if got := mreqChannelVerdict(env, user, memberReqLookupMode{Classify: true}); got != bot.want {
						t.Fatalf("verdict = %d, want %d", got, bot.want)
					}
					if env.redis.Exists(memberReqSubKey(channel, user)) {
						t.Error("a lost lookup wrote the subscription key")
					}
				})
			}
		}
	})

	t.Run("without Classify the same lost lookup makes no bot status request", func(t *testing.T) {
		env, state := mreqLookupEnv(t)
		channel := mreqChannel(t, state, env.chat.Id, gotgbot.ChatMemberStatusMember)
		user := mreqLookupUser(t)
		state.scriptMember(channel, user, errors.New("boom"))
		if got := mreqChannelVerdict(env, user, memberReqLookupMode{}); got != memberReqUnknown {
			t.Fatalf("verdict = %d, want unknown", got)
		}
		if n := mreqMemberCalls(env, channel, staffTestBotID); n != 0 {
			t.Errorf("bot status requests = %d, want 0", n)
		}
	})

	t.Run("Classify does not touch a pass or a candidate", func(t *testing.T) {
		env, state := mreqLookupEnv(t)
		channel := mreqChannel(t, state, env.chat.Id, gotgbot.ChatMemberStatusMember)
		passing, leaving := mreqLookupUser(t), mreqLookupUser(t)
		state.setMember(channel, passing, mreqChannelMember{Status: gotgbot.ChatMemberStatusMember})
		state.setMember(channel, leaving, mreqChannelMember{Status: gotgbot.ChatMemberStatusLeft})
		if got := mreqChannelVerdict(env, passing, memberReqLookupMode{Classify: true}); got != memberReqPass {
			t.Errorf("a subscribed member = %d, want pass", got)
		}
		if got := mreqChannelVerdict(env, leaving, memberReqLookupMode{Classify: true}); got != memberReqInert {
			t.Errorf("a leaving member of a channel the bot does not administer = %d, want inert", got)
		}
	})
}

// mreqBotStatus runs the bot-status lookup of channel with a 5 s deadline.
func mreqBotStatus(e *memberReqEnv, channel int64, mode memberReqLookupMode) memberReqBotStatus {
	e.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return memberReqBotChannelStatus(ctx, e.bot, channel, mode)
}

func TestMemberReqBotStatusClassifier(t *testing.T) {
	merged := func(status string) gotgbot.MergedChatMember { return gotgbot.MergedChatMember{Status: status} }
	wrapped := func(err error) error { return fmt.Errorf("lookup failed: %w", err) }

	t.Run("pure: the status without an error", func(t *testing.T) {
		cases := []struct {
			status string
			want   memberReqBotStatus
		}{
			{gotgbot.ChatMemberStatusAdministrator, memberReqBotAdmin},
			{gotgbot.ChatMemberStatusCreator, memberReqBotAdmin},
			{gotgbot.ChatMemberStatusMember, memberReqBotNotAdmin},
			{gotgbot.ChatMemberStatusRestricted, memberReqBotNotAdmin},
			{gotgbot.ChatMemberStatusLeft, memberReqBotNotAdmin},
			{gotgbot.ChatMemberStatusKicked, memberReqBotNotAdmin},
			{"", memberReqBotUnknown},
			{"something_new", memberReqBotUnknown},
		}
		for _, tc := range cases {
			if got := memberReqClassifyBot(merged(tc.status), nil); got != tc.want {
				t.Errorf("status %q = %d, want %d", tc.status, got, tc.want)
			}
		}
	})

	t.Run("pure: an error is classified by its code alone", func(t *testing.T) {
		cases := []struct {
			name string
			err  error
			want memberReqBotStatus
		}{
			{"400", staffFakeError(400, "Bad Request: CHAT_ADMIN_REQUIRED"), memberReqBotNotAdmin},
			{"400 chat not found", staffFakeError(400, "Bad Request: chat not found"), memberReqBotNotAdmin},
			{"400 wrapped", wrapped(staffFakeError(400, "anything")), memberReqBotNotAdmin},
			{"400 with no description", staffFakeError(400, ""), memberReqBotNotAdmin},
			{"403", staffFakeError(403, "Forbidden: bot was kicked from the channel chat"), memberReqBotNotAdmin},
			{"403 wrapped", wrapped(staffFakeError(403, "Forbidden")), memberReqBotNotAdmin},
			{"429", staffFake429(1), memberReqBotUnknown},
			{"429 wrapped", wrapped(staffFake429(1)), memberReqBotUnknown},
			{"shed", fmt.Errorf("%w: next slot is 1m away", ratelimit.ErrRateLimited), memberReqBotUnknown},
			{"401", staffFakeError(401, "Unauthorized"), memberReqBotUnknown},
			{"404", staffFakeError(404, "Not Found"), memberReqBotUnknown},
			{"409", staffFakeError(409, "Conflict"), memberReqBotUnknown},
			{"500", staffFakeError(500, "Internal Server Error"), memberReqBotUnknown},
			{"500 whose text says chat not found", staffFakeError(500, "Bad Request: chat not found"), memberReqBotUnknown},
			{"502", staffFakeError(502, "Bad Gateway"), memberReqBotUnknown},
			{"a transport error", errors.New("connection reset by peer"), memberReqBotUnknown},
			{"a transport error that says chat not found", errors.New("chat not found"), memberReqBotUnknown},
			{"a deadline", context.DeadlineExceeded, memberReqBotUnknown},
			{"a cancelled context", context.Canceled, memberReqBotUnknown},
		}
		for _, tc := range cases {
			// An error wins over a status, so even an administrator-looking answer that came
			// with an error is judged by the error alone.
			if got := memberReqClassifyBot(merged(gotgbot.ChatMemberStatusAdministrator), tc.err); got != tc.want {
				t.Errorf("%s = %d, want %d", tc.name, got, tc.want)
			}
		}
	})

	t.Run("the lookup classifies what the fake answers and caches only an administrator", func(t *testing.T) {
		cases := []struct {
			name  string
			setup func(t *testing.T, s *mreqLookupState, c int64)
			want  memberReqBotStatus
		}{
			{"administrator", func(_ *testing.T, s *mreqLookupState, c int64) {
				s.setMember(c, staffTestBotID, mreqChannelMember{Status: gotgbot.ChatMemberStatusAdministrator})
			}, memberReqBotAdmin},
			{"creator", func(_ *testing.T, s *mreqLookupState, c int64) {
				s.setMember(c, staffTestBotID, mreqChannelMember{Status: gotgbot.ChatMemberStatusCreator})
			}, memberReqBotAdmin},
			{"member", func(_ *testing.T, s *mreqLookupState, c int64) {
				s.setMember(c, staffTestBotID, mreqChannelMember{Status: gotgbot.ChatMemberStatusMember})
			}, memberReqBotNotAdmin},
			{"left", func(_ *testing.T, s *mreqLookupState, c int64) {
				s.setMember(c, staffTestBotID, mreqChannelMember{Status: gotgbot.ChatMemberStatusLeft})
			}, memberReqBotNotAdmin},
			{"a null answer", func(_ *testing.T, s *mreqLookupState, c int64) {
				s.setMember(c, staffTestBotID, mreqChannelMember{Null: true})
			}, memberReqBotUnknown},
			{"an unrecognised status", func(_ *testing.T, s *mreqLookupState, c int64) {
				s.setMember(c, staffTestBotID, mreqChannelMember{Status: "something_new"})
			}, memberReqBotUnknown},
			{"a 400", func(_ *testing.T, s *mreqLookupState, c int64) {
				s.scriptMember(c, staffTestBotID, staffFakeError(400, "Bad Request: chat not found"))
			}, memberReqBotNotAdmin},
			{"a 403", func(_ *testing.T, s *mreqLookupState, c int64) {
				s.scriptMember(c, staffTestBotID, staffFakeError(403, "Forbidden"))
			}, memberReqBotNotAdmin},
			{"a 429 that keeps coming", func(_ *testing.T, s *mreqLookupState, c int64) {
				s.scriptMember(c, staffTestBotID, staffFake429(1), staffFake429(1))
			}, memberReqBotUnknown},
			{"a 500 whose text says chat not found", func(_ *testing.T, s *mreqLookupState, c int64) {
				s.scriptMember(c, staffTestBotID, staffFakeError(500, "Bad Request: chat not found"))
			}, memberReqBotUnknown},
			{"a plain error", func(_ *testing.T, s *mreqLookupState, c int64) { s.scriptMember(c, staffTestBotID, errors.New("boom")) }, memberReqBotUnknown},
			{"a timeout", func(t *testing.T, s *mreqLookupState, c int64) {
				mreqSwap(t, &memberReqLookupFlightTimeout, 60*time.Millisecond)
				t.Cleanup(s.holdKey(mreqChannelKey("getChatMember", c, staffTestBotID)))
			}, memberReqBotUnknown},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				env, state := mreqLookupEnv(t)
				channel := mreqChannel(t, state, env.chat.Id, gotgbot.ChatMemberStatusMember)
				tc.setup(t, state, channel)
				if got := mreqBotStatus(env, channel, memberReqLookupMode{}); got != tc.want {
					t.Fatalf("status = %d, want %d", got, tc.want)
				}
				if cached := env.redis.Exists(memberReqBotAdminKey(channel)); cached != (tc.want == memberReqBotAdmin) {
					t.Errorf("ba key cached = %v, want %v (an administrator only)", cached, tc.want == memberReqBotAdmin)
				}
			})
		}
	})

	t.Run("a cancelled context and a pacer shed are unknown", func(t *testing.T) {
		env, state := mreqLookupEnv(t)
		channel := mreqChannel(t, state, env.chat.Id, gotgbot.ChatMemberStatusAdministrator)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if got := memberReqBotChannelStatus(ctx, env.bot, channel, memberReqLookupMode{}); got != memberReqBotUnknown {
			t.Errorf("a cancelled context = %d, want unknown", got)
		}
		mreqSetKey(t, env, "fuku:memberreq:lkp:pace:block", time.Minute)
		t.Cleanup(func() { env.redis.Del("fuku:memberreq:lkp:pace:block") })
		if got := mreqBotStatus(env, channel, memberReqLookupMode{}); got != memberReqBotUnknown {
			t.Errorf("a pacer shed = %d, want unknown", got)
		}
		if n := mreqMemberCalls(env, channel, staffTestBotID); n != 0 {
			t.Errorf("bot status requests = %d, want 0", n)
		}
	})
}

func TestMemberReqChannelBreachNeedsBotAdmin(t *testing.T) {
	// The fake answers "left" for every member of the channel, as a bot that cannot see the
	// member list might (spike S2): whatever the bot's own standing says, nobody is a breach
	// unless the bot is an administrator.
	t.Run("a left answer is never a breach unless the bot is an administrator", func(t *testing.T) {
		bots := []struct {
			name string
			set  func(s *mreqLookupState, c int64)
			want memberReqVerdict
		}{
			{"member", func(s *mreqLookupState, c int64) {
				s.setMember(c, staffTestBotID, mreqChannelMember{Status: gotgbot.ChatMemberStatusMember})
			}, memberReqInert},
			{"left", func(s *mreqLookupState, c int64) {
				s.setMember(c, staffTestBotID, mreqChannelMember{Status: gotgbot.ChatMemberStatusLeft})
			}, memberReqInert},
			{"kicked", func(s *mreqLookupState, c int64) {
				s.setMember(c, staffTestBotID, mreqChannelMember{Status: gotgbot.ChatMemberStatusKicked})
			}, memberReqInert},
			{"restricted", func(s *mreqLookupState, c int64) {
				s.setMember(c, staffTestBotID, mreqChannelMember{Status: gotgbot.ChatMemberStatusRestricted})
			}, memberReqInert},
			{"a 403", func(s *mreqLookupState, c int64) { s.scriptMember(c, staffTestBotID, staffFakeError(403, "Forbidden")) }, memberReqInert},
			{"a 400", func(s *mreqLookupState, c int64) {
				s.scriptMember(c, staffTestBotID, staffFakeError(400, "Bad Request"))
			}, memberReqInert},
			{"a 500", func(s *mreqLookupState, c int64) { s.scriptMember(c, staffTestBotID, staffFakeError(500, "Internal")) }, memberReqUnknown},
			{"a null answer", func(s *mreqLookupState, c int64) { s.setMember(c, staffTestBotID, mreqChannelMember{Null: true}) }, memberReqUnknown},
			{"an administrator", func(s *mreqLookupState, c int64) {
				s.setMember(c, staffTestBotID, mreqChannelMember{Status: gotgbot.ChatMemberStatusAdministrator})
			}, memberReqBreach},
			{"the creator", func(s *mreqLookupState, c int64) {
				s.setMember(c, staffTestBotID, mreqChannelMember{Status: gotgbot.ChatMemberStatusCreator})
			}, memberReqBreach},
		}
		for _, bot := range bots {
			t.Run("bot is "+bot.name, func(t *testing.T) {
				env, state := mreqLookupEnv(t)
				channel := mreqChannel(t, state, env.chat.Id, gotgbot.ChatMemberStatusMember)
				bot.set(state, channel)
				// Only the first lookup reads the scripted error; the cache of an
				// administrator answers the second member.
				for i := 0; i < 2; i++ {
					user := mreqLookupUser(t)
					state.setMember(channel, user, mreqChannelMember{Status: gotgbot.ChatMemberStatusLeft})
					got := mreqChannelVerdict(env, user, memberReqLookupMode{})
					want := bot.want
					if i == 1 && (bot.name == "a 403" || bot.name == "a 400" || bot.name == "a 500") {
						// The scripted error is spent; the fake's own answer for the bot
						// (a plain member) is what the second lookup reads.
						want = memberReqInert
					}
					if got != want {
						t.Errorf("member %d: verdict = %d, want %d", i, got, want)
					}
				}
			})
		}
	})

	t.Run("an administrator bot is cached for 2 minutes and the second breach asks nothing about the bot", func(t *testing.T) {
		env, state := mreqLookupEnv(t)
		channel := mreqChannel(t, state, env.chat.Id, gotgbot.ChatMemberStatusAdministrator)
		first, second := mreqLookupUser(t), mreqLookupUser(t)
		state.setMember(channel, first, mreqChannelMember{Status: gotgbot.ChatMemberStatusLeft})
		state.setMember(channel, second, mreqChannelMember{Status: gotgbot.ChatMemberStatusKicked})

		if got := mreqChannelVerdict(env, first, memberReqLookupMode{}); got != memberReqBreach {
			t.Fatalf("first verdict = %d, want breach", got)
		}
		key := memberReqBotAdminKey(channel)
		if key != "fuku:memberreq:ba:"+strconv.FormatInt(channel, 10) {
			t.Fatalf("key = %q", key)
		}
		if ttl := env.redis.TTL(key); ttl < time.Minute || ttl > 2*time.Minute {
			t.Errorf("TTL = %s, want about 2 minutes", ttl)
		}
		if got := mreqChannelVerdict(env, second, memberReqLookupMode{}); got != memberReqBreach {
			t.Fatalf("second verdict = %d, want breach", got)
		}
		if n := mreqMemberCalls(env, channel, staffTestBotID); n != 1 {
			t.Errorf("bot status requests = %d, want 1", n)
		}
	})

	t.Run("a not-administrator and an unknown bot status are never cached", func(t *testing.T) {
		env, state := mreqLookupEnv(t)
		channel := mreqChannel(t, state, env.chat.Id, gotgbot.ChatMemberStatusMember)
		user := mreqLookupUser(t)
		state.setMember(channel, user, mreqChannelMember{Status: gotgbot.ChatMemberStatusLeft})
		mreqChannelVerdict(env, user, memberReqLookupMode{})
		if env.redis.Exists(memberReqBotAdminKey(channel)) {
			t.Error("a not-administrator was cached")
		}
		state.scriptMember(channel, staffTestBotID, staffFakeError(500, "Internal"))
		mreqChannelVerdict(env, user, memberReqLookupMode{})
		if env.redis.Exists(memberReqBotAdminKey(channel)) {
			t.Error("an unknown status was cached")
		}
	})

	t.Run("Fresh bypasses the ba cache and drops a stale positive on a definite not-administrator", func(t *testing.T) {
		env, state := mreqLookupEnv(t)
		channel := mreqChannel(t, state, env.chat.Id, gotgbot.ChatMemberStatusMember)
		user := mreqLookupUser(t)
		state.setMember(channel, user, mreqChannelMember{Status: gotgbot.ChatMemberStatusLeft})
		key := memberReqBotAdminKey(channel)
		mreqSetKey(t, env, key, 2*time.Minute)

		// The stale positive is believed without Fresh (that is the 2 minute bound) ...
		if got := mreqChannelVerdict(env, user, memberReqLookupMode{}); got != memberReqBreach {
			t.Fatalf("verdict from the stale positive = %d, want breach", got)
		}
		if n := mreqMemberCalls(env, channel, staffTestBotID); n != 0 {
			t.Fatalf("bot status requests = %d, want 0 (served from the cache)", n)
		}
		// ... and not with Fresh, which reads the demotion and removes the key.
		if got := mreqChannelVerdict(env, user, memberReqLookupMode{Fresh: true}); got != memberReqInert {
			t.Fatalf("Fresh verdict = %d, want inert", got)
		}
		if env.redis.Exists(key) {
			t.Error("a Fresh read left the stale positive")
		}
	})

	t.Run("Fresh on a definite breach drops a stale subscription pass", func(t *testing.T) {
		env, state := mreqLookupEnv(t)
		channel := mreqChannel(t, state, env.chat.Id, gotgbot.ChatMemberStatusAdministrator)
		user := mreqLookupUser(t)
		state.setMember(channel, user, mreqChannelMember{Status: gotgbot.ChatMemberStatusLeft})
		key := memberReqSubKey(channel, user)
		mreqSetKey(t, env, key, time.Minute)

		if got := mreqChannelVerdict(env, user, memberReqLookupMode{}); got != memberReqPass {
			t.Fatalf("verdict from the stale pass = %d, want pass", got)
		}
		if got := mreqChannelVerdict(env, user, memberReqLookupMode{Fresh: true}); got != memberReqBreach {
			t.Fatalf("Fresh verdict = %d, want breach", got)
		}
		if env.redis.Exists(key) {
			t.Error("a Fresh breach left the stale pass")
		}
	})
}

func TestMemberReqLookupCacheKeys(t *testing.T) {
	t.Run("mreqLookupCleanup removes every key a channel lookup made", func(t *testing.T) {
		env, state := mreqLookupEnv(t)
		var channel, user int64
		// The inner test owns the cleanup; the outer one still has its Redis afterwards.
		t.Run("lookups", func(t *testing.T) {
			channel = mreqChannel(t, state, env.chat.Id, gotgbot.ChatMemberStatusAdministrator)
			user = mreqLookupUser(t)
			state.setMember(channel, user, mreqChannelMember{Status: gotgbot.ChatMemberStatusMember})
			if got := mreqChannelVerdict(env, user, memberReqLookupMode{}); got != memberReqPass {
				t.Fatalf("verdict = %d, want pass", got)
			}
			leaving := mreqLookupUser(t)
			state.setMember(channel, leaving, mreqChannelMember{Status: gotgbot.ChatMemberStatusLeft})
			if got := mreqChannelVerdict(env, leaving, memberReqLookupMode{}); got != memberReqBreach {
				t.Fatalf("leaving verdict = %d, want breach", got)
			}
			for _, key := range []string{memberReqSubKey(channel, user), memberReqBotAdminKey(channel), memberReqLinkedKey(env.chat.Id)} {
				if !env.redis.Exists(key) {
					t.Fatalf("the lookups wrote no %s to clean", key)
				}
			}
		})
		for _, key := range env.redis.Keys() {
			if strings.HasPrefix(key, "fuku:memberreq:") && strings.Contains(key, strconv.FormatInt(channel, 10)) {
				t.Errorf("key %s survived mreqLookupCleanup", key)
			}
		}
	})

	t.Run("each key has its own shape and lifetime and only a pass, a link or a bot administrator is written", func(t *testing.T) {
		env, state := mreqLookupEnv(t)
		channel := mreqChannel(t, state, env.chat.Id, gotgbot.ChatMemberStatusAdministrator)
		subscribed, leaving := mreqLookupUser(t), mreqLookupUser(t)
		state.setMember(channel, subscribed, mreqChannelMember{Status: gotgbot.ChatMemberStatusMember})
		state.setMember(channel, leaving, mreqChannelMember{Status: gotgbot.ChatMemberStatusLeft})

		// Nothing is written by a breach, which is not even a cache candidate ...
		if got := mreqChannelVerdict(env, leaving, memberReqLookupMode{}); got != memberReqBreach {
			t.Fatalf("leaving member = %d, want breach", got)
		}
		if env.redis.Exists(memberReqSubKey(channel, leaving)) {
			t.Error("a breach wrote the subscription key")
		}
		// ... and a pass writes exactly the sub key, with the lc and ba keys the lookups made.
		if got := mreqChannelVerdict(env, subscribed, memberReqLookupMode{}); got != memberReqPass {
			t.Fatalf("subscribed member = %d, want pass", got)
		}
		chat, chanID := strconv.FormatInt(env.chat.Id, 10), strconv.FormatInt(channel, 10)
		wantKeys := map[string]struct {
			value    string
			low, top time.Duration
		}{
			"fuku:memberreq:lc:" + chat: {chanID, 9 * time.Minute, 10 * time.Minute},
			"fuku:memberreq:sub:" + chanID + ":" + strconv.FormatInt(subscribed, 10): {"1", 29 * time.Minute, 30 * time.Minute},
			"fuku:memberreq:ba:" + chanID:                                            {"1", time.Minute, 2 * time.Minute},
		}
		for key, want := range wantKeys {
			value, err := env.redis.Get(key)
			if err != nil || value != want.value {
				t.Errorf("%s = %q (%v), want %q", key, value, err, want.value)
			}
			if ttl := env.redis.TTL(key); ttl < want.low || ttl > want.top {
				t.Errorf("TTL of %s = %s, want between %s and %s", key, ttl, want.low, want.top)
			}
		}
		for _, key := range env.redis.Keys() {
			if strings.HasPrefix(key, "fuku:memberreq:") && !strings.HasPrefix(key, "fuku:memberreq:lkp:pace:") {
				if _, known := wantKeys[key]; !known {
					t.Errorf("unexpected key %s", key)
				}
			}
		}
	})

	t.Run("an inert, unknown or not-administrator answer writes none of the three keys", func(t *testing.T) {
		env, state := mreqLookupEnv(t)
		channel := mreqChannel(t, state, env.chat.Id, gotgbot.ChatMemberStatusMember)
		user := mreqLookupUser(t)
		state.setMember(channel, user, mreqChannelMember{Status: gotgbot.ChatMemberStatusLeft})
		state.scriptChat(env.chat.Id, staffFakeError(500, "Internal"))
		// An unreadable link, then an inert answer (the bot is a plain member), then an
		// unknown member answer: the only key allowed is the link of the second call.
		if got := mreqChannelVerdict(env, user, memberReqLookupMode{}); got != memberReqUnknown {
			t.Fatalf("unreadable link = %d, want unknown", got)
		}
		if env.redis.Exists(memberReqLinkedKey(env.chat.Id)) {
			t.Error("an unreadable link was cached")
		}
		if got := mreqChannelVerdict(env, user, memberReqLookupMode{}); got != memberReqInert {
			t.Fatalf("plain-member bot = %d, want inert", got)
		}
		state.scriptMember(channel, user, errors.New("boom"))
		if got := mreqChannelVerdict(env, user, memberReqLookupMode{}); got != memberReqUnknown {
			t.Fatalf("lost lookup = %d, want unknown", got)
		}
		if env.redis.Exists(memberReqSubKey(channel, user)) || env.redis.Exists(memberReqBotAdminKey(channel)) {
			t.Error("a non-pass wrote the sub or ba key")
		}
	})

	t.Run("a none link is cached as 0 and a negative ID as its decimal text", func(t *testing.T) {
		env, state := mreqLookupEnv(t)
		state.link(env.chat.Id, 0)
		mreqLookupCleanup(t, nil, []int64{env.chat.Id})
		mreqLinked(env, memberReqLookupMode{})
		value, err := env.redis.Get(memberReqLinkedKey(env.chat.Id))
		if err != nil || value != "0" {
			t.Errorf("none link = %q (%v), want 0", value, err)
		}
	})
}

// TestMemberReqLookupFlightsKeepModesApart proves that a Fresh caller never joins a running
// non-Fresh flight (whose answer can come from a stale positive cache read) and the other way
// round, while callers of one mode still share one request.
func TestMemberReqLookupFlightsKeepModesApart(t *testing.T) {
	t.Run("a fresh photo caller makes its own request during a held non-fresh flight and ignores a stale pass", func(t *testing.T) {
		env, state := mreqLookupEnv(t)
		user := mreqLookupUser(t)
		state.set(user, mreqPhotoAnswer{})
		release := state.hold(user)
		t.Cleanup(release)

		leader := mreqLookupGo(env, user, 5*time.Second, memberReqLookupMode{})
		mreqLookupAwaitCalls(t, env, 1)
		// The member's photo was removed after the pass was cached: a stale positive appears.
		mreqSetKey(t, env, memberReqPhotoKey(user), time.Minute)

		fresh := mreqLookupGo(env, user, 5*time.Second, memberReqLookupMode{Fresh: true})
		mreqLookupAwaitCalls(t, env, 2)
		release()
		if got := mreqVerdictWithin(t, fresh, 5*time.Second, "the fresh caller"); got != memberReqBreach {
			t.Fatalf("fresh verdict = %d, want breach (the stale pass must be ignored)", got)
		}
		if got := mreqVerdictWithin(t, leader, 5*time.Second, "the non-fresh caller"); got != memberReqBreach {
			t.Fatalf("non-fresh verdict = %d, want breach", got)
		}
		if n := mreqLookupPhotoCalls(env); n != 2 {
			t.Errorf("Telegram requests = %d, want 2 (one per mode)", n)
		}
		if env.redis.Exists(memberReqPhotoKey(user)) {
			t.Error("the fresh breach left the stale pass in the cache")
		}
	})

	t.Run("a non-fresh photo caller does not join a held fresh flight either", func(t *testing.T) {
		env, state := mreqLookupEnv(t)
		user := mreqLookupUser(t)
		state.set(user, mreqPhotoAnswer{Total: 1})
		release := state.hold(user)
		t.Cleanup(release)

		fresh := mreqLookupGo(env, user, 5*time.Second, memberReqLookupMode{Fresh: true})
		mreqLookupAwaitCalls(t, env, 1)
		plain := mreqLookupGo(env, user, 5*time.Second, memberReqLookupMode{})
		mreqLookupAwaitCalls(t, env, 2)
		release()
		for name, ch := range map[string]<-chan memberReqVerdict{"fresh": fresh, "non-fresh": plain} {
			if got := mreqVerdictWithin(t, ch, 5*time.Second, name); got != memberReqPass {
				t.Errorf("%s verdict = %d, want pass", name, got)
			}
		}
	})

	t.Run("a hundred fresh callers still make one request", func(t *testing.T) {
		env, state := mreqLookupEnv(t)
		user := mreqLookupUser(t)
		state.set(user, mreqPhotoAnswer{})
		release := state.hold(user)
		t.Cleanup(release)

		start := make(chan struct{})
		results := make(chan memberReqVerdict, 100)
		for i := 0; i < 100; i++ {
			go func() {
				<-start
				results <- mreqPhoto(env, user, memberReqLookupMode{Fresh: true})
			}()
		}
		close(start)
		mreqLookupAwaitCalls(t, env, 1)
		time.Sleep(150 * time.Millisecond)
		release()
		for i := 0; i < 100; i++ {
			select {
			case got := <-results:
				if got != memberReqBreach {
					t.Fatalf("caller %d got verdict %d, want breach", i, got)
				}
			case <-time.After(5 * time.Second):
				t.Fatalf("only %d of 100 callers returned", i)
			}
		}
		if n := mreqLookupPhotoCalls(env); n != 1 {
			t.Fatalf("Telegram requests for 100 fresh callers = %d, want 1", n)
		}
	})

	t.Run("a fresh bot status caller makes its own request during a held non-fresh flight", func(t *testing.T) {
		env, state := mreqLookupEnv(t)
		channel := mreqChannel(t, state, env.chat.Id, gotgbot.ChatMemberStatusAdministrator)
		release := state.holdKey(mreqChannelKey("getChatMember", channel, staffTestBotID))
		t.Cleanup(release)

		leader := make(chan memberReqBotStatus, 1)
		go func() { leader <- mreqBotStatus(env, channel, memberReqLookupMode{}) }()
		mreqAwaitMemberCalls(t, env, channel, staffTestBotID, 1)
		// The bot was demoted after the leader's request was answered by "administrator".
		state.setMember(channel, staffTestBotID, mreqChannelMember{Status: gotgbot.ChatMemberStatusMember})

		fresh := make(chan memberReqBotStatus, 1)
		go func() { fresh <- mreqBotStatus(env, channel, memberReqLookupMode{Fresh: true}) }()
		mreqAwaitMemberCalls(t, env, channel, staffTestBotID, 2)
		release()
		select {
		case got := <-fresh:
			if got != memberReqBotNotAdmin {
				t.Errorf("fresh bot status = %d, want not an administrator", got)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("the fresh bot status caller got no answer")
		}
		select {
		case got := <-leader:
			if got != memberReqBotAdmin {
				t.Errorf("non-fresh bot status = %d, want administrator", got)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("the non-fresh bot status caller got no answer")
		}
	})

	t.Run("a fresh linked channel read makes its own request during a held non-fresh flight", func(t *testing.T) {
		env, state := mreqLookupEnv(t)
		channel := mreqChannel(t, state, env.chat.Id, gotgbot.ChatMemberStatusAdministrator)
		release := state.holdKey(mreqChannelKey("getChat", env.chat.Id, 0))
		t.Cleanup(release)

		leader := make(chan int64, 1)
		go func() { id, _ := mreqLinked(env, memberReqLookupMode{}); leader <- id }()
		mreqAwaitChatCalls(t, env, env.chat.Id, 1)
		state.link(env.chat.Id, 0)

		fresh := make(chan int64, 1)
		go func() { id, _ := mreqLinked(env, memberReqLookupMode{Fresh: true}); fresh <- id }()
		mreqAwaitChatCalls(t, env, env.chat.Id, 2)
		release()
		select {
		case got := <-fresh:
			if got != 0 {
				t.Errorf("fresh linked channel = %d, want 0 (the group was unlinked)", got)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("the fresh caller got no answer")
		}
		select {
		case got := <-leader:
			if got != channel {
				t.Errorf("non-fresh linked channel = %d, want %d", got, channel)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("the non-fresh caller got no answer")
		}
	})
}

// mreqAwaitMemberCalls waits until the fake has seen want getChatMember requests for user in chat.
func mreqAwaitMemberCalls(t *testing.T, e *memberReqEnv, chat, user int64, want int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for mreqMemberCalls(e, chat, user) < want {
		if time.Now().After(deadline) {
			t.Fatalf("the fake saw %d getChatMember requests, want %d", mreqMemberCalls(e, chat, user), want)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// mreqAwaitChatCalls waits until the fake has seen want getChat requests for chat.
func mreqAwaitChatCalls(t *testing.T, e *memberReqEnv, chat int64, want int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for mreqChatCalls(e, chat) < want {
		if time.Now().After(deadline) {
			t.Fatalf("the fake saw %d getChat requests, want %d", mreqChatCalls(e, chat), want)
		}
		time.Sleep(2 * time.Millisecond)
	}
}
