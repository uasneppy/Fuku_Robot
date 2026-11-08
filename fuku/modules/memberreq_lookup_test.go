//go:build testtools

package modules

import (
	"context"
	"errors"
	"sort"
	"testing"
	"time"
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
		{"a pacer shed behind a long shared block", func(_ *testing.T, e *memberReqEnv, _ *mreqLookupState, _ int64) {
			e.redis.Set("fuku:memberreq:lkp:pace:block", "1")
			e.redis.SetTTL("fuku:memberreq:lkp:pace:block", time.Minute)
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
			env.redis.Set(key, "1")
			env.redis.SetTTL(key, time.Minute)
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
