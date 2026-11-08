package modules

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sync/singleflight"

	"github.com/uasneppy/fuku_robot/fuku/utils/cache"
	"github.com/uasneppy/fuku_robot/fuku/utils/error_handling"
	"github.com/uasneppy/fuku_robot/fuku/utils/ratelimit"
)

// The lookup layer of the member requirements: the Telegram questions a rule asks about a
// member that the update itself does not answer (the profile photo and the subscription to the
// linked channel). Everything here
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
	// Classify lets the channel rule read a member lookup that could not be answered as
	// inert when the bot is shown to be no administrator of the channel (the held release
	// and "I fixed it" set it; the punishing path never does, so it makes no extra call).
	Classify bool
}

const (
	// memberReqLookupFlightTimeoutDefault is how long one shared lookup may run.
	memberReqLookupFlightTimeoutDefault = 15 * time.Second
	// memberReqPositiveTTLDefault is how long a pass stays cached.
	memberReqPositiveTTLDefault = 30 * time.Minute
	// memberReqLinkedTTL is how long a group's linked channel (or "none") stays cached.
	memberReqLinkedTTL = 10 * time.Minute
	// memberReqBotAdminTTL is how long the bot's definite administrator status in a channel
	// stays cached. It bounds how long a demoted bot is still read as an administrator.
	memberReqBotAdminTTL = 2 * time.Minute
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

// memberReqLookupFlight coalesces the lookups of one key: p:<user> for the photo, l:<chat>
// for a group's linked channel, s:<channel>:<user> for a subscription and b:<channel> for the
// bot's status in a channel.
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
	memberReqCacheSetValue(ctx, key, "1", ttl)
}

// memberReqCacheSetValue stores value under key for ttl, a silent no-op without Redis or on
// an error.
func memberReqCacheSetValue(ctx context.Context, key, value string, ttl time.Duration) {
	client := memberReqRedisClient()
	if client == nil {
		return
	}
	writeCtx, cancel := context.WithTimeout(ctx, raidRedisTimeout)
	defer cancel()
	if err := client.Set(writeCtx, key, value, ttl).Err(); err != nil {
		log.Debugf("[MemberReq] a lookup cache write failed: %s", memberReqErrText(err))
	}
}

// memberReqCacheGet reads key. A Redis error, no Redis and a missing key are all a miss.
func memberReqCacheGet(ctx context.Context, key string) (string, bool) {
	client := memberReqRedisClient()
	if client == nil {
		return "", false
	}
	readCtx, cancel := context.WithTimeout(ctx, raidRedisTimeout)
	defer cancel()
	value, err := client.Get(readCtx, key).Result()
	if err != nil {
		return "", false
	}
	return value, true
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
// It is memberReqFlightValue for a verdict, whose zero value is unknown.
func memberReqFlight(ctx context.Context, key string, fn func(context.Context) memberReqVerdict) memberReqVerdict {
	verdict, ok := memberReqFlightValue(ctx, key, fn)
	if !ok {
		return memberReqUnknown
	}
	return verdict
}

// memberReqFlightValue runs fn once for every concurrent caller of key and returns its
// value, and false when the caller got no value: its own context ended, or fn panicked.
//
// fn runs on a context derived from memberReqContext(), bounded by
// memberReqLookupFlightTimeout, and not from the caller: StopMemberReqJobs ends every lookup
// at once, and one caller's short deadline never cancels a lookup that other callers share.
// Each caller waits with its own deadline and gets no value when it runs out.
//
// fn is wrapped in its own panic recovery. A panic in a singleflight DoChan leader is
// re-raised by the library in a goroutine nothing can recover, which would crash the
// process, so the recovery has to be inside the function. A recovered panic returns no value.
// Keys of different value types must differ (the p:, l:, s: and b: families do).
func memberReqFlightValue[T any](ctx context.Context, key string, fn func(context.Context) T) (T, bool) {
	var zero T
	if ctx.Err() != nil {
		return zero, false
	}
	results := memberReqLookupFlight.DoChan(key, func() (any, error) {
		defer error_handling.RecoverFromPanic("memberReqFlight", "MemberReq")
		flightCtx, cancel := context.WithTimeout(memberReqContext(), memberReqLookupFlightTimeout)
		defer cancel()
		return fn(flightCtx), nil
	})
	select {
	case result := <-results:
		value, ok := result.Val.(T)
		if !ok || result.Err != nil {
			return zero, false
		}
		return value, true
	case <-ctx.Done():
		return zero, false
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

// memberReqLinkedKey is the Redis key of a group's linked channel: the decimal channel ID,
// or 0 for a group with none.
func memberReqLinkedKey(chatID int64) string {
	return "fuku:memberreq:lc:" + strconv.FormatInt(chatID, 10)
}

// memberReqSubKey is the Redis key of a member's cached subscription pass in a channel.
func memberReqSubKey(channelID, userID int64) string {
	return "fuku:memberreq:sub:" + strconv.FormatInt(channelID, 10) + ":" + strconv.FormatInt(userID, 10)
}

// memberReqBotAdminKey is the Redis key of the bot's cached administrator status in a channel.
func memberReqBotAdminKey(channelID int64) string {
	return "fuku:memberreq:ba:" + strconv.FormatInt(channelID, 10)
}

// memberReqLinkAnswer is the value of the linked channel flight.
type memberReqLinkAnswer struct {
	// ID is the channel's chat ID, 0 for none.
	ID int64
	// Known is false when the link could not be read.
	Known bool
}

// memberReqLinkedChannel returns the channel linked to chatID and whether that is known. The
// channel comes only from getChat(chatID).LinkedChatId, asked through the lookup pacer in
// one flight per group and cached as fuku:memberreq:lc:<chat> (the channel's decimal ID,
// or 0 for none): never from callback data or the update, so nobody can point the rule at a
// channel of their own. An error, a shed, a timeout and a null answer are not known and
// write nothing. A linked_chat_id of zero (Telegram omits it for a group with none) is
// none and is cached; a positive one is no chat ID of a channel, so it reads as none and is
// not cached. With mode.Fresh the cache is not read.
func memberReqLinkedChannel(ctx context.Context, b *gotgbot.Bot, chatID int64, mode memberReqLookupMode) (int64, bool) {
	key := memberReqLinkedKey(chatID)
	if !mode.Fresh {
		if value, ok := memberReqCacheGet(ctx, key); ok {
			if id, err := strconv.ParseInt(value, 10, 64); err == nil && id <= 0 {
				return id, true
			}
		}
	}
	answer, ok := memberReqFlightValue(ctx, "l:"+strconv.FormatInt(chatID, 10), func(flightCtx context.Context) memberReqLinkAnswer {
		var info *gotgbot.ChatFullInfo
		err := memberReqLookupPaced(flightCtx, func(callCtx context.Context) error {
			var callErr error
			info, callErr = b.GetChatWithContext(callCtx, chatID, nil)
			return callErr
		})
		if err != nil {
			memberReqLookupLogFailure("linked channel", chatID, 0, err)
			return memberReqLinkAnswer{}
		}
		if info == nil {
			log.Debugf("[MemberReq] the linked channel lookup of chat %d had an empty answer", chatID)
			return memberReqLinkAnswer{}
		}
		if info.LinkedChatId > 0 {
			return memberReqLinkAnswer{Known: true}
		}
		memberReqCacheSetValue(flightCtx, key, strconv.FormatInt(info.LinkedChatId, 10), memberReqLinkedTTL)
		return memberReqLinkAnswer{ID: info.LinkedChatId, Known: true}
	})
	if !ok || !answer.Known {
		return 0, false
	}
	return answer.ID, true
}

// memberReqSubscribed judges a member's getChatMember answer in the channel. A creator, an
// administrator and a member pass, and so does a restricted member who is still in the
// channel (is_member). Left, kicked and restricted without is_member are only a breach
// candidate: memberReqBreach here means "the answer says not subscribed", and the caller
// counts it only once the bot is shown to administer the channel (D-22). Every other
// status, the empty one of an unrecognised or null answer included, is unknown.
func memberReqSubscribed(member gotgbot.MergedChatMember) memberReqVerdict {
	switch member.Status {
	case gotgbot.ChatMemberStatusCreator, gotgbot.ChatMemberStatusAdministrator, gotgbot.ChatMemberStatusMember:
		return memberReqPass
	case gotgbot.ChatMemberStatusRestricted:
		if member.IsMember {
			return memberReqPass
		}
		return memberReqBreach
	case gotgbot.ChatMemberStatusLeft, gotgbot.ChatMemberStatusKicked:
		return memberReqBreach
	}
	return memberReqUnknown
}

// memberReqBotStatus is what the bot's own getChatMember answer says about its standing in
// a channel. The zero value is unknown.
type memberReqBotStatus int

const (
	// memberReqBotUnknown means the standing could not be established.
	memberReqBotUnknown memberReqBotStatus = iota
	// memberReqBotAdmin means the bot is the creator or an administrator of the channel.
	memberReqBotAdmin
	// memberReqBotNotAdmin means the bot is definitely no administrator of the channel.
	memberReqBotNotAdmin
)

// memberReqClassifyBot maps the bot's own getChatMember(channel, bot) answer to its standing,
// by status and by Telegram error code, and never by a description. With no error:
// administrator and creator are an administrator; member, restricted, left and kicked are
// not; any other status, the empty one of a null or unrecognised answer included, is unknown.
// With an error: any Telegram 400 or 403 (cache.IsBotGoneError) is not an administrator,
// which only errs toward inert, and everything else (a 429, a shed, a 5xx, a transport
// error, a timeout, a cancelled context) is unknown. It never produces a breach, and it is
// not chat_status.IsBotMissingError, whose markers read "chat not found" as the bot being
// missing.
func memberReqClassifyBot(member gotgbot.MergedChatMember, err error) memberReqBotStatus {
	if err != nil {
		if !errors.Is(err, ratelimit.ErrRateLimited) && cache.IsBotGoneError(err) {
			return memberReqBotNotAdmin
		}
		return memberReqBotUnknown
	}
	switch member.Status {
	case gotgbot.ChatMemberStatusCreator, gotgbot.ChatMemberStatusAdministrator:
		return memberReqBotAdmin
	case gotgbot.ChatMemberStatusMember, gotgbot.ChatMemberStatusRestricted,
		gotgbot.ChatMemberStatusLeft, gotgbot.ChatMemberStatusKicked:
		return memberReqBotNotAdmin
	}
	return memberReqBotUnknown
}

// memberReqBotChannelStatus establishes whether the bot administers channelID, with one
// getChatMember(channel, bot) through the lookup pacer in one flight per channel. Only an
// administrator is cached (fuku:memberreq:ba:<channel>, 2 minutes); a not-administrator and
// an unknown answer never are. With mode.Fresh the cache is not read and a definite
// not-administrator drops a stale positive, so a demoted bot is not hidden by it.
func memberReqBotChannelStatus(ctx context.Context, b *gotgbot.Bot, channelID int64, mode memberReqLookupMode) memberReqBotStatus {
	key := memberReqBotAdminKey(channelID)
	if !mode.Fresh && memberReqCacheHas(ctx, key) {
		return memberReqBotAdmin
	}
	status, ok := memberReqFlightValue(ctx, "b:"+strconv.FormatInt(channelID, 10), func(flightCtx context.Context) memberReqBotStatus {
		var answer gotgbot.ChatMember
		err := memberReqLookupPaced(flightCtx, func(callCtx context.Context) error {
			var callErr error
			answer, callErr = b.GetChatMemberWithContext(callCtx, channelID, b.Id, nil)
			return callErr
		})
		var merged gotgbot.MergedChatMember
		if err == nil && answer != nil {
			merged = answer.MergeChatMember()
		}
		if err != nil {
			memberReqLookupLogFailure("channel bot status", channelID, b.Id, err)
		}
		status := memberReqClassifyBot(merged, err)
		switch status {
		case memberReqBotAdmin:
			memberReqCacheSet(flightCtx, key, memberReqBotAdminTTL)
		case memberReqBotNotAdmin:
			if mode.Fresh {
				memberReqCacheDel(flightCtx, key)
			}
		case memberReqBotUnknown:
		}
		return status
	})
	if !ok {
		return memberReqBotUnknown
	}
	return status
}

// memberReqChannelRule judges whether a member is subscribed to the channel linked to chatID.
//
// The order is fixed. The linked channel (memberReqLinkedChannel): unreadable is unknown and
// none is inert, with no member request. The subscription cache (not with mode.Fresh): a
// hit is a pass. Then one getChatMember(channel, user) through the lookup pacer, in one
// flight per member, judged by memberReqSubscribed: a pass is cached for
// memberReqPositiveTTL and an error, a null answer or an unrecognised status is unknown and
// is never inspected for a breach. A not-subscribed answer is only a candidate (D-22): it is
// a breach when the bot is established as an administrator of the channel, inert when the
// bot is not (a bot that cannot see a channel's members may be told left about everyone),
// and unknown when that cannot be read. With mode.Classify an unanswerable member lookup is
// inert when the bot is definitely no channel administrator, and stays unknown when the bot
// administers the channel or its standing is unknown. Nothing but a pass is cached under
// the subscription key, and with mode.Fresh a definite breach drops a stale pass.
func memberReqChannelRule(ctx context.Context, b *gotgbot.Bot, chatID, userID int64, mode memberReqLookupMode) memberReqVerdict {
	channel, known := memberReqLinkedChannel(ctx, b, chatID, mode)
	if !known {
		return memberReqUnknown
	}
	if channel == 0 {
		return memberReqInert
	}
	key := memberReqSubKey(channel, userID)
	if !mode.Fresh && memberReqCacheHas(ctx, key) {
		return memberReqPass
	}
	flightKey := "s:" + strconv.FormatInt(channel, 10) + ":" + strconv.FormatInt(userID, 10)
	verdict := memberReqFlight(ctx, flightKey, func(flightCtx context.Context) memberReqVerdict {
		var answer gotgbot.ChatMember
		err := memberReqLookupPaced(flightCtx, func(callCtx context.Context) error {
			var callErr error
			answer, callErr = b.GetChatMemberWithContext(callCtx, channel, userID, nil)
			return callErr
		})
		if err != nil {
			memberReqLookupLogFailure("channel subscription", channel, userID, err)
			return memberReqUnknown
		}
		if answer == nil {
			log.Debugf("[MemberReq] the subscription lookup of user %d in channel %d had an empty answer", userID, channel)
			return memberReqUnknown
		}
		judged := memberReqSubscribed(answer.MergeChatMember())
		if judged == memberReqPass {
			memberReqCacheSet(flightCtx, key, memberReqPositiveTTL)
		}
		return judged
	})
	switch verdict {
	case memberReqBreach:
		verdict = memberReqGateBreach(ctx, b, channel, mode)
		if verdict == memberReqBreach && mode.Fresh {
			memberReqCacheDel(ctx, key)
		}
		return verdict
	case memberReqUnknown:
		if mode.Classify && memberReqBotChannelStatus(ctx, b, channel, mode) == memberReqBotNotAdmin {
			return memberReqInert
		}
	}
	return verdict
}

// memberReqGateBreach is the D-22 gate: a not-subscribed candidate of the channel is a breach
// only while the bot is established as an administrator of channelID, inert while it is not
// and unknown while that cannot be read.
func memberReqGateBreach(ctx context.Context, b *gotgbot.Bot, channelID int64, mode memberReqLookupMode) memberReqVerdict {
	switch memberReqBotChannelStatus(ctx, b, channelID, mode) {
	case memberReqBotAdmin:
		return memberReqBreach
	case memberReqBotNotAdmin:
		return memberReqInert
	}
	return memberReqUnknown
}
