package modules

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sync/singleflight"

	"github.com/uasneppy/fuku_robot/fuku/utils/error_handling"
	"github.com/uasneppy/fuku_robot/fuku/utils/ratelimit"
)

// The lookup layer of the member requirements: the Telegram questions a rule asks about a
// member that the update itself does not answer (today the profile photo). Everything here
// is built so that a failure of a lookup punishes nobody: the verdict's zero value is
// unknown, a failed, shed, cancelled or null answer is unknown, and only a definite answer
// is a pass or a breach. Logs carry chat and user IDs and the error code only, never a
// name, a title, a file ID or Telegram's description.

// memberReqVerdict is the judgement of one lookup. Its zero value is memberReqUnknown, so a
// path that forgets to set a verdict, a recovered panic and a zero-valued flight result all
// read as "does nothing": only memberReqBreach ever leads to a punishment.
type memberReqVerdict int

const (
	// memberReqUnknown is the answer when the lookup could not decide. It never punishes.
	memberReqUnknown memberReqVerdict = iota
	// memberReqPass means the member meets the rule.
	memberReqPass
	// memberReqBreach means a definite answer shows the member breaks the rule.
	memberReqBreach
	// memberReqInert means the rule cannot be judged in this group (for example it needs a
	// channel the group has none of): it never punishes and the page says so.
	memberReqInert
)

// memberReqLookupMode tunes one lookup.
type memberReqLookupMode struct {
	// Fresh bypasses the positive cache, for a caller that must see the member as they are
	// now ("I fixed it"), and drops a stale positive when the answer is a breach.
	Fresh bool
}

const (
	// memberReqLookupFlightTimeoutDefault is how long one shared lookup may run.
	memberReqLookupFlightTimeoutDefault = 15 * time.Second
	// memberReqPositiveTTLDefault is how long a pass stays cached.
	memberReqPositiveTTLDefault = 30 * time.Minute
)

var (
	// memberReqLookupFlightTimeout bounds one flight: the paced call and the cache write. A
	// variable so a test can shorten it.
	memberReqLookupFlightTimeout = memberReqLookupFlightTimeoutDefault

	// memberReqPositiveTTL is the life of a cached pass. A variable so a test can shorten it.
	memberReqPositiveTTL = memberReqPositiveTTLDefault
)

// memberReqLookupPacerOptions are the options of the lookup pacer. It is a variable so a test
// can build a second pacer over the same Redis keys (a second replica) or a fast one.
var memberReqLookupPacerOptions = ratelimit.TelegramPacerOptions{
	NextKey:  "fuku:memberreq:lkp:pace:next",
	BlockKey: "fuku:memberreq:lkp:pace:block",
	Interval: 100 * time.Millisecond,
	// MaxRetries 1 is the smallest the pacer accepts (a value of 0 or less becomes 3).
	MaxRetries: 1,
	// A slot, or a shared block, more than 5 s away is shed as unknown with no Telegram
	// request: a lookup that cannot start soon is not worth the wait.
	MaxWait: 5 * time.Second,
}

// memberReqLookupPacer paces every photo and channel lookup across the fleet. Its keys are
// not the requirement pacer's, so a burst of lookups never delays a mute notice. It is a
// variable so tests can install a fast one.
var memberReqLookupPacer = ratelimit.NewTelegramPacer(memberReqLookupPacerOptions)

// memberReqLookupFlight coalesces the lookups of one key: p:<user> for the photo.
var memberReqLookupFlight singleflight.Group

// memberReqLookupPaced runs one lookup call under the lookup pacer: it waits for its slot,
// bounds the call by memberReqCallTimeout and, on a 429, waits out a short retry_after once.
// A slot more than the pacer's MaxWait away fails at once as ratelimit.ErrRateLimited with
// no Telegram request.
func memberReqLookupPaced(ctx context.Context, call func(context.Context) error) error {
	return memberReqLookupPacer.Do(ctx, func(callCtx context.Context) error {
		bounded, cancel := context.WithTimeout(callCtx, memberReqCallTimeout)
		defer cancel()
		return call(bounded)
	})
}

// memberReqPhotoKey is the Redis key of a member's cached pass. It holds no chat: a profile
// photo belongs to the user. It sits outside the fuku:cache: prefix, so
// CLEAR_CACHE_ON_STARTUP keeps it.
func memberReqPhotoKey(userID int64) string {
	return "fuku:memberreq:photo:" + strconv.FormatInt(userID, 10)
}

// memberReqCacheHas reports whether key exists. A Redis error, no Redis and a missing key
// are all a miss: the lookup then runs, which is correct and only slower.
func memberReqCacheHas(ctx context.Context, key string) bool {
	client := memberReqRedisClient()
	if client == nil {
		return false
	}
	readCtx, cancel := context.WithTimeout(ctx, raidRedisTimeout)
	defer cancel()
	n, err := client.Exists(readCtx, key).Result()
	return err == nil && n > 0
}

// memberReqCacheSet stores a pass under key for ttl. No Redis and a Redis error are a silent
// no-op: nothing correct depends on the key.
func memberReqCacheSet(ctx context.Context, key string, ttl time.Duration) {
	client := memberReqRedisClient()
	if client == nil {
		return
	}
	writeCtx, cancel := context.WithTimeout(ctx, raidRedisTimeout)
	defer cancel()
	if err := client.Set(writeCtx, key, "1", ttl).Err(); err != nil {
		log.Debugf("[MemberReq] a lookup cache write failed: %s", memberReqErrText(err))
	}
}

// memberReqCacheDel drops key, a silent no-op without Redis or on an error.
func memberReqCacheDel(ctx context.Context, key string) {
	client := memberReqRedisClient()
	if client == nil {
		return
	}
	delCtx, cancel := context.WithTimeout(ctx, raidRedisTimeout)
	defer cancel()
	if err := client.Del(delCtx, key).Err(); err != nil {
		log.Debugf("[MemberReq] a lookup cache delete failed: %s", memberReqErrText(err))
	}
}

// memberReqFlight runs fn once for every concurrent caller of key and returns its verdict.
//
// fn runs on a context derived from memberReqContext(), bounded by
// memberReqLookupFlightTimeout, and not from the caller: StopMemberReqJobs ends every lookup
// at once, and one caller's short deadline never cancels a lookup that other callers share.
// Each caller waits with its own deadline and gets unknown when it runs out.
//
// fn is wrapped in its own panic recovery. A panic in a singleflight DoChan leader is
// re-raised by the library in a goroutine nothing can recover, which would crash the
// process, so the recovery has to be inside the function. A recovered panic returns the
// zero verdict, memberReqUnknown.
func memberReqFlight(ctx context.Context, key string, fn func(context.Context) memberReqVerdict) memberReqVerdict {
	if ctx.Err() != nil {
		return memberReqUnknown
	}
	results := memberReqLookupFlight.DoChan(key, func() (any, error) {
		defer error_handling.RecoverFromPanic("memberReqFlight", "MemberReq")
		flightCtx, cancel := context.WithTimeout(memberReqContext(), memberReqLookupFlightTimeout)
		defer cancel()
		return fn(flightCtx), nil
	})
	select {
	case result := <-results:
		verdict, ok := result.Val.(memberReqVerdict)
		if !ok || result.Err != nil {
			return memberReqUnknown
		}
		return verdict
	case <-ctx.Done():
		return memberReqUnknown
	}
}

// memberReqLookupLogFailure logs a lookup that got no answer by IDs and a short reason
// only: the error code of a Telegram answer, "rate limited" for a shed or a 429 that kept
// coming (ErrRateLimited carries Telegram's description, which is never logged), or the
// context's own error. It is routine in a raid or at shutdown, so it logs at debug.
func memberReqLookupLogFailure(what string, chatID, userID int64, err error) {
	text := "rate limited"
	if !errors.Is(err, ratelimit.ErrRateLimited) {
		text = memberReqErrText(err)
	}
	log.Debugf("[MemberReq] the %s lookup of user %d in chat %d was not answered: %s", what, userID, chatID, text)
}

// memberReqPhotoRule judges whether a member has a profile photo.
//
// It asks getUserProfilePhotos for one photo of the user through the lookup pacer, in one
// flight per user. A total of zero with no photos is a breach. A total above zero or a
// non-empty list is a pass, cached for memberReqPositiveTTL. Everything else, an error, a
// 400, 403 or 429, a timeout, a shed, a cancelled context and a null answer, is unknown,
// and neither a breach nor an unknown answer is ever cached (D-24). With mode.Fresh the
// cache is not read, and a definite breach drops a stale pass, so a member who removed their
// photo is not waved through afterwards.
func memberReqPhotoRule(ctx context.Context, b *gotgbot.Bot, chatID, userID int64, mode memberReqLookupMode) memberReqVerdict {
	key := memberReqPhotoKey(userID)
	if !mode.Fresh && memberReqCacheHas(ctx, key) {
		return memberReqPass
	}
	return memberReqFlight(ctx, "p:"+strconv.FormatInt(userID, 10), func(flightCtx context.Context) memberReqVerdict {
		var photos *gotgbot.UserProfilePhotos
		err := memberReqLookupPaced(flightCtx, func(callCtx context.Context) error {
			var callErr error
			photos, callErr = b.GetUserProfilePhotosWithContext(callCtx, userID, &gotgbot.GetUserProfilePhotosOpts{Limit: 1})
			return callErr
		})
		if err != nil {
			memberReqLookupLogFailure("photo", chatID, userID, err)
			return memberReqUnknown
		}
		// A body of null decodes to a nil pointer and a nil error: not an answer.
		if photos == nil {
			log.Debugf("[MemberReq] the photo lookup of user %d in chat %d had an empty answer", userID, chatID)
			return memberReqUnknown
		}
		if photos.TotalCount > 0 || len(photos.Photos) > 0 {
			memberReqCacheSet(flightCtx, key, memberReqPositiveTTL)
			return memberReqPass
		}
		if mode.Fresh {
			memberReqCacheDel(flightCtx, key)
		}
		return memberReqBreach
	})
}
