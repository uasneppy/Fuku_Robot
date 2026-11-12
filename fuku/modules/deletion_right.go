package modules

import (
	"context"
	"errors"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	log "github.com/sirupsen/logrus"

	"github.com/uasneppy/fuku_robot/fuku/db/deletion"
	"github.com/uasneppy/fuku_robot/fuku/utils/cache"
	"github.com/uasneppy/fuku_robot/fuku/utils/ratelimit"
)

// The delete-right reader (DEL-08, D-17, D-22): one classifier of the bot's own chat member record
// and one live read through the deletion pacer. The worker's service latch and the settings page
// use it unchanged.

// deletionRight is what the bot's own record says about deleting other people's messages. Its zero
// value is unknown, so a missing answer, a recovered panic and a shed all claim nothing.
type deletionRight int

const (
	// deletionRightUnknown means the right could not be established; nothing may be decided on it.
	deletionRightUnknown deletionRight = iota
	// deletionRightCan means the bot can delete other people's messages in the chat.
	deletionRightCan
	// deletionRightCannot means the bot definitely cannot.
	deletionRightCannot
)

// deletionServicePrefix is the prefix of every service kind, for dropping a chat's service rows.
const deletionServicePrefix = "service:"

// deletionClassifyBotRight maps the bot's own getChatMember answer (or the error) to its right,
// by status and by Telegram error code only, never by a description. With an error: a Telegram 400
// or 403 (cache.IsBotGoneError, never a shed pacer) is cannot and everything else (a 429, a 5xx, a
// transport error, a timeout, a shed pacer) is unknown. With no error: the creator can; an
// administrator can when it holds the delete right, or when the flag is not trusted (a basic
// group's administrator may read the flag false while still being able to delete); member,
// restricted, left and kicked cannot; a null record and any other status are unknown.
func deletionClassifyBotRight(member gotgbot.MergedChatMember, err error, trustAdminFlag bool) deletionRight {
	if err != nil {
		if !errors.Is(err, ratelimit.ErrRateLimited) && cache.IsBotGoneError(err) {
			return deletionRightCannot
		}
		return deletionRightUnknown
	}
	switch member.Status {
	case gotgbot.ChatMemberStatusCreator:
		return deletionRightCan
	case gotgbot.ChatMemberStatusAdministrator:
		if member.CanDeleteMessages || !trustAdminFlag {
			return deletionRightCan
		}
		return deletionRightCannot
	case gotgbot.ChatMemberStatusMember, gotgbot.ChatMemberStatusRestricted,
		gotgbot.ChatMemberStatusLeft, gotgbot.ChatMemberStatusKicked:
		return deletionRightCannot
	}
	return deletionRightUnknown
}

// deletionReadBotRight asks the bot's own record in chatID live, through the deletion pacer and
// under the caller's timeout (the wait for the pacer slot included), and classifies it. Nothing is
// cached: the answer is always the one Telegram gives now.
func deletionReadBotRight(ctx context.Context, b *gotgbot.Bot, chatID int64, timeout time.Duration, trustAdminFlag bool) deletionRight {
	readCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var answer gotgbot.ChatMember
	err := deletionPaced(readCtx, func(callCtx context.Context) error {
		var callErr error
		answer, callErr = b.GetChatMemberWithContext(callCtx, chatID, b.Id, nil)
		return callErr
	})
	var merged gotgbot.MergedChatMember
	if err == nil && answer != nil {
		merged = answer.MergeChatMember()
	}
	return deletionClassifyBotRight(merged, err, trustAdminFlag)
}

// deletionSupergroupMaxID is the highest chat ID of a supergroup or channel: the Bot API gives
// them IDs of -1000000000000 and below, and a basic group an ID above that (-1 to -999999999999).
const deletionSupergroupMaxID int64 = -1000000000000

// deletionTrustAdminFlag reports whether the worker may trust an administrator's can_delete_messages
// flag in chatID. It does for a supergroup or channel ID, where the flag is reliable (SEC-02); the
// worker has no chat type, so the ID range is the signal. A basic group's administrator may read the
// flag false while still being able to delete, so the flag is not trusted there, nor for any other ID.
func deletionTrustAdminFlag(chatID int64) bool {
	return chatID <= deletionSupergroupMaxID
}

// deletionDecideServiceLatch makes the one live right check behind a service latch. The worker has
// no chat type, so the administrator flag is trusted by the chat ID's range alone
// (deletionTrustAdminFlag): an administrator without the delete right is latched in a supergroup,
// and never in a basic group, which avoids a false latch there. A bot that definitely cannot delete
// latches the chat for service kinds only (10 minutes) and drops the chat's pending service rows;
// any other answer only returns, and the caller drops the one refused row. The writes run on a
// context detached from the run, so a shutdown never strands a half-made decision.
func deletionDecideServiceLatch(ctx context.Context, b *gotgbot.Bot, chatID int64) deletionRight {
	right := deletionReadBotRight(ctx, b, chatID, deletionCallTimeout, deletionTrustAdminFlag(chatID))
	if right != deletionRightCannot {
		return right
	}
	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), deletionRecordTimeout)
	defer cancel()
	deletionSetLatch(writeCtx, chatID, deletionScopeService)
	if _, err := deletion.DropChatPending(writeCtx, chatID, deletionServicePrefix); err != nil {
		log.Warnf("[Deletion] could not drop the pending service rows of chat %d: %v", chatID, err)
	}
	return right
}
