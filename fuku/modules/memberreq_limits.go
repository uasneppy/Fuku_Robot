package modules

import (
	"context"
	"strconv"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	log "github.com/sirupsen/logrus"

	"github.com/uasneppy/fuku_robot/fuku/i18n"
)

// The per-chat brake of the member requirements (D-13, D-21; research S-07). A mis-set rule in a
// busy group must not punish everyone: at most memberReqCapPerMinute punishments start in a
// chat in one fixed minute, and the next violation pauses the chat's punishments for
// memberReqPause with one visible notice. The brake only stops new punishments and notices; it
// never drops a notice already due (the cap is counted before the claim, never after the
// punishment) and never stops the delete of an established non-admin's message.
//
// Everything here is Redis only and best effort (D-15): with no Redis, or on a Redis error, a
// punishment is allowed and nothing is paused, and the requirements work as before.

var (
	// memberReqCapPerMinute is the most punishments that start per chat in one fixed minute. A
	// variable only so a test can make it small; production code never reassigns it.
	memberReqCapPerMinute = 10
	// memberReqPause is how long a chat's punishments stay paused once the cap is exceeded. A
	// variable only so a test can shorten it; production code never reassigns it.
	memberReqPause = 10 * time.Minute
)

// memberReqCapCounterTTL is how long a minute's counter lives: two windows, so a counter that
// is read late is still there and an idle chat's counters expire on their own.
const memberReqCapCounterTTL = 120 * time.Second

// memberReqCapKey is the counter of one chat's punishment attempts in one fixed minute (the
// Unix time divided by 60). The chat ID is a hash tag, so the chat's keys share a cluster slot.
func memberReqCapKey(chatID, minute int64) string {
	return "fuku:memberreq:{" + strconv.FormatInt(chatID, 10) + "}:cap:" + strconv.FormatInt(minute, 10)
}

// memberReqPausedKey is the pause latch of one chat, set with SET NX for memberReqPause.
func memberReqPausedKey(chatID int64) string {
	return "fuku:memberreq:{" + strconv.FormatInt(chatID, 10) + "}:paused"
}

// memberReqCapAllow counts one punishment attempt of the chat in the current minute, before
// the claim, and reports whether the punishment may start. An attempt over the cap is
// refused and sets the pause latch with SET NX: only the caller whose SET NX succeeded posts
// the one pause notice, so one notice is posted however many replicas tip the cap at once.
// No Redis and a Redis error allow the punishment; a run that was cancelled punishes nobody.
func memberReqCapAllow(ctx context.Context, b *gotgbot.Bot, chatID int64) bool {
	client := memberReqRedisClient()
	if client == nil {
		return true
	}
	countCtx, cancel := context.WithTimeout(ctx, raidRedisTimeout)
	defer cancel()
	key := memberReqCapKey(chatID, raidNow().Unix()/60)
	pipe := client.Pipeline()
	count := pipe.Incr(countCtx, key)
	pipe.Expire(countCtx, key, memberReqCapCounterTTL)
	if _, err := pipe.Exec(countCtx); err != nil {
		if ctx.Err() != nil {
			return false
		}
		log.Warnf("[MemberReq] the punishment count of chat %d is unknown, so the punishment is allowed: %s", chatID, memberReqErrText(err))
		return true
	}
	if count.Val() <= int64(memberReqCapPerMinute) {
		return true
	}
	memberReqPauseChat(ctx, b, chatID)
	return false
}

// memberReqPauseChat sets the chat's pause latch and, for the one caller that set it, posts the
// pause notice. A failed latch write or notice is logged and changes nothing: the punishment
// that tipped the cap is refused either way.
func memberReqPauseChat(ctx context.Context, b *gotgbot.Bot, chatID int64) {
	client := memberReqRedisClient()
	if client == nil {
		return
	}
	latchCtx, cancel := context.WithTimeout(ctx, raidRedisTimeout)
	defer cancel()
	won, err := client.SetNX(latchCtx, memberReqPausedKey(chatID), "1", memberReqPause).Result()
	if err != nil {
		log.Warnf("[MemberReq] the pause of chat %d could not be set: %s", chatID, memberReqErrText(err))
		return
	}
	if !won {
		return
	}
	log.Warnf("[MemberReq] chat %d reached %d punishments in a minute, so its punishments are paused for %s", chatID, memberReqCapPerMinute, memberReqPause)
	tr := memberReqTranslator(ctx, chatID)
	text := memberReqText(tr, "mreq_notice_paused", i18n.TranslationParams{
		"n":       strconv.Itoa(memberReqCapPerMinute),
		"minutes": strconv.Itoa(int(memberReqPause / time.Minute)),
	})
	if _, err := memberReqPostNotice(ctx, b, chatID, text, gotgbot.InlineKeyboardMarkup{}); err != nil {
		log.Warnf("[MemberReq] the pause notice of chat %d was not delivered: %s", chatID, memberReqErrText(err))
	}
}

// memberReqPaused reports whether the chat's punishments are paused. No Redis and a Redis error
// are "not paused": the cap is the only thing that sets the latch, and it is Redis only too.
func memberReqPaused(ctx context.Context, chatID int64) bool {
	client := memberReqRedisClient()
	if client == nil {
		return false
	}
	checkCtx, cancel := context.WithTimeout(ctx, raidRedisTimeout)
	defer cancel()
	n, err := client.Exists(checkCtx, memberReqPausedKey(chatID)).Result()
	return err == nil && n > 0
}
