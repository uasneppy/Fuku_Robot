package modules

import (
	"context"
	"strconv"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/uasneppy/fuku_robot/fuku/db/memberreq"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// The recorded-warn marker (WR-01). A warn is stored first and its hold is marked punished
// right after, inside the warn lock. When that mark still fails after its retries the hold stays
// pending, and two minutes later memberreq.StaleClaim hands it to the member's next violating
// message, which would warn a second time. Kick and ban recover through the leave update and a
// mute through its live marker, so only the warn needs this: a short-lived Redis key, set as soon
// as the warn is stored, tells the reclaim that the warn already exists.
//
// It is best effort and Redis only (accepted, like the other fuku:memberreq:* keys): without
// Redis, or on a Redis error, the key is neither written nor found, and the reclaim warns again.

// memberReqWarnedGrace is how long the marker outlives memberreq.StaleClaim, so it is still
// there when the reclaim runs.
const memberReqWarnedGrace = 10 * time.Minute

// memberReqWarnedKey is the marker of a warn recorded for the member in the chat. Its value is
// the ID of the hold the warn belongs to, so a marker left by an earlier episode never matches
// the hold of a later one.
func memberReqWarnedKey(chatID, userID int64) string {
	return "fuku:memberreq:warned:" + strconv.FormatInt(chatID, 10) + ":" + strconv.FormatInt(userID, 10)
}

// memberReqNoteWarned records that the warn of hold was stored. A failure is only logged.
func memberReqNoteWarned(ctx context.Context, c memberReqCase, hold *models.ChatRequirementHold) {
	client := memberReqRedisClient()
	if client == nil {
		return
	}
	recordCtx, cancel := memberReqRecordCtx(ctx)
	defer cancel()
	key := memberReqWarnedKey(c.Chat.Id, c.User.Id)
	if err := client.Set(recordCtx, key, strconv.FormatInt(hold.ID, 10), memberreq.StaleClaim+memberReqWarnedGrace).Err(); err != nil {
		log.Warnf("[MemberReq] noting the warn of user %d in chat %d failed: %s", c.User.Id, c.Chat.Id, memberReqErrText(err))
	}
}

// memberReqWarnedNoted reports whether a recorded warn is noted for hold. No Redis and a Redis
// error are "not noted".
func memberReqWarnedNoted(ctx context.Context, c memberReqCase, hold *models.ChatRequirementHold) bool {
	client := memberReqRedisClient()
	if client == nil {
		return false
	}
	recordCtx, cancel := memberReqRecordCtx(ctx)
	defer cancel()
	value, err := client.Get(recordCtx, memberReqWarnedKey(c.Chat.Id, c.User.Id)).Result()
	return err == nil && value == strconv.FormatInt(hold.ID, 10)
}

// memberReqClearWarned drops the marker once the hold is punished. A failure is only logged: the
// marker expires on its own and names one hold.
func memberReqClearWarned(ctx context.Context, c memberReqCase) {
	client := memberReqRedisClient()
	if client == nil {
		return
	}
	recordCtx, cancel := memberReqRecordCtx(ctx)
	defer cancel()
	if err := client.Del(recordCtx, memberReqWarnedKey(c.Chat.Id, c.User.Id)).Err(); err != nil {
		log.Warnf("[MemberReq] clearing the warn note of user %d in chat %d failed: %s", c.User.Id, c.Chat.Id, memberReqErrText(err))
	}
}

// memberReqAdoptRecordedWarn handles a reclaimed warn hold whose warn was already stored: the
// stale hold was a warn, the new action is a warn too and the marker names this hold. It marks
// the hold punished (a failure leaves it pending, and the next reclaim adopts it again) and
// reports true, so the caller warns and announces nothing a second time.
func memberReqAdoptRecordedWarn(ctx context.Context, c memberReqCase, stale, hold *models.ChatRequirementHold, action int) bool {
	if stale == nil || stale.Action != models.RequirementActionWarn || action != models.RequirementActionWarn {
		return false
	}
	// The reclaim keeps the row, so the claimed hold carries the stale one's ID.
	if hold == nil || hold.ID != stale.ID || !memberReqWarnedNoted(ctx, c, hold) {
		return false
	}
	if memberReqMarkPunished(ctx, c, hold) {
		memberReqClearWarned(ctx, c)
	}
	log.Infof("[MemberReq] the warn of user %d in chat %d was already recorded, so it is not repeated", c.User.Id, c.Chat.Id)
	return true
}
