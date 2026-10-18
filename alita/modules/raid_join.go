package modules

import (
	"context"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	log "github.com/sirupsen/logrus"

	"github.com/divkix/Alita_Robot/alita/db/approvals"
	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/db/raiddetection"
)

// raidSettingsTimeout bounds the settings read on the join hot path.
const raidSettingsTimeout = 2 * time.Second

// raidSettingsLookup is the join counter's read of a chat's raid detection settings. It
// is a variable only so a test can make the read fail; production code never reassigns
// it.
var raidSettingsLookup = raiddetection.GetSettings

// raidNeedsPerformerLookup reports whether deciding if a join counts depends on the
// performer's live status: the member is not the bot, the join is not a request, and the
// performer is neither the member, the bot nor the anonymous admin. Unlike the lockdown
// guard's own test it includes a bot account added by someone, because an admin who adds a
// bot is not a raid.
func raidNeedsPerformerLookup(in lockdownJoinInput) bool {
	return in.MemberID != in.BotID &&
		in.Path != models.JoinPathRequest &&
		in.PerformerID != in.MemberID &&
		in.PerformerID != in.BotID &&
		!in.PerformerAnonymousAdmin
}

// raidJoinCounts is the counting rule of the join surge (D-02, RAID-07): whether one join
// is evidence of a raid. It does not count the bot itself, a join request (nobody got in),
// an approved user, or a member added by the anonymous admin or by a live creator or
// administrator other than themselves, a bot an admin added included. Everything else
// counts: self-joins, joins the bot performed, and joins whose performer's status is
// unknown (an empty PerformerStatus).
func raidJoinCounts(in lockdownJoinInput, approved bool) bool {
	switch {
	case in.MemberID == in.BotID, in.Path == models.JoinPathRequest, approved:
		return false
	case in.PerformerID == in.MemberID:
		return true
	case in.PerformerAnonymousAdmin:
		return false
	case in.PerformerStatus == gotgbot.ChatMemberStatusCreator,
		in.PerformerStatus == gotgbot.ChatMemberStatusAdministrator:
		return false
	}
	return true
}

// raidCountMemberJoin counts the join a chat_member update reports toward the chat's join
// surge and starts a lockdown when enough different accounts joined within a minute. It
// is best effort: without Redis, outside a supergroup or with unreadable settings it does
// nothing, and it never bans, restricts or deletes anything (the lockdown it starts bans
// through the worker and the guard, by their own rules).
//
// It returns the performer status it read and looked true whenever it made that live
// lookup, whether the lookup worked (status set) or failed (status empty), so the join
// guard never asks Telegram a second time about the same join.
func raidCountMemberJoin(b *gotgbot.Bot, chat *gotgbot.Chat, update *gotgbot.ChatMemberUpdated) (performerStatus string, looked bool) {
	if chat == nil || update == nil || chat.Type != "supergroup" || raidRedis() == nil {
		return "", false
	}

	ctx, cancel := context.WithTimeout(context.Background(), raidSettingsTimeout)
	settings, err := raidSettingsLookup(ctx, chat.Id)
	cancel()
	if err != nil {
		log.Debugf("[RaidDetection] settings of chat %d could not be read, counting nothing: %v", chat.Id, err)
		return "", false
	}
	if settings == nil || (!settings.JoinEnabled && !settings.FloodEnabled) {
		return "", false
	}

	member := update.NewChatMember.MergeChatMember().User
	in := lockdownJoinInput{
		Path:                    models.JoinPathMember,
		PerformerID:             update.From.Id,
		MemberID:                member.Id,
		MemberIsBot:             member.IsBot,
		BotID:                   b.Id,
		PerformerAnonymousAdmin: update.From.Id == lockdownGroupAnonymousBot,
	}
	if raidNeedsPerformerLookup(in) {
		looked = true
		// A failed lookup leaves the status empty, which counts.
		if live, lookupErr := lockdownLiveMember(b, chat.Id, update.From.Id); lookupErr != nil {
			log.Warnf("[RaidDetection] could not check performer %d in chat %d: %v", update.From.Id, chat.Id, lookupErr)
		} else {
			performerStatus = live.Status
			in.PerformerStatus = live.Status
		}
	}

	if !raidJoinCounts(in, false) {
		return performerStatus, looked
	}
	if !raidJoinCounts(in, approvals.IsUserApproved(chat.Id, member.Id)) {
		return performerStatus, looked
	}

	count, err := recordRaidJoins(chat.Id, []int64{member.Id})
	if err != nil {
		log.Debugf("[RaidDetection] join counter write for chat %d failed: %v", chat.Id, err)
		return performerStatus, looked
	}
	if settings.JoinEnabled && count >= settings.JoinThreshold {
		raidFire(b, chat, raidTrigger{
			Kind:   models.LockdownTriggerJoinSurge,
			Count:  count,
			Window: int(raidJoinWindow / time.Second),
		})
	}
	return performerStatus, looked
}
