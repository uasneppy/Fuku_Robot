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

// raidSettingsTimeout bounds every settings read of the raid triggers and of /autolockdown.
const raidSettingsTimeout = 2 * time.Second

// raidSettingsLookup is the read of a chat's raid detection settings used by the join
// counters, the flood watchers and /autolockdown, always under raidSettingsTimeout. It is
// a variable only so a test can make the read fail or see its deadline; production code
// never reassigns it.
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
	if chat == nil || update == nil {
		return "", false
	}
	settings, ok := raidJoinSettings(chat)
	if !ok {
		return "", false
	}

	in := lockdownMemberJoinInput(b, update)
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

	if raidJoinsCountedFor(chat.Id, in) {
		raidRecordAndFire(b, chat, settings, []int64{in.MemberID})
	}
	return performerStatus, looked
}

// raidCountMessageJoins counts the joins a new_chat_members service message reports by
// the same rule as raidCountMemberJoin, in one counter write, and starts a lockdown when
// enough different accounts joined within a minute. A user who also arrived as a
// chat_member update is the same account and counts once.
//
// It returns the performer status it read and looked true whenever it made the live
// lookup of the message's performer, whether it worked (status set) or failed (status
// empty), so the join guard never asks Telegram a second time about the same message.
func raidCountMessageJoins(b *gotgbot.Bot, chat *gotgbot.Chat, msg *gotgbot.Message) (performerStatus string, looked bool) {
	if chat == nil || msg == nil || len(msg.NewChatMembers) == 0 {
		return "", false
	}
	settings, ok := raidJoinSettings(chat)
	if !ok {
		return "", false
	}

	inputs, performerID := lockdownMessageJoinInputs(b, chat, msg)
	needsLookup := false
	for _, in := range inputs {
		needsLookup = needsLookup || raidNeedsPerformerLookup(in)
	}
	if needsLookup && performerID > 0 {
		looked = true
		// A failed lookup leaves the status empty, which counts.
		if live, lookupErr := lockdownLiveMember(b, chat.Id, performerID); lookupErr != nil {
			log.Warnf("[RaidDetection] could not check performer %d in chat %d: %v", performerID, chat.Id, lookupErr)
		} else {
			performerStatus = live.Status
		}
	}

	counted := make([]int64, 0, len(inputs))
	for _, in := range inputs {
		in.PerformerStatus = performerStatus
		if raidJoinsCountedFor(chat.Id, in) {
			counted = append(counted, in.MemberID)
		}
	}
	raidRecordAndFire(b, chat, settings, counted)
	return performerStatus, looked
}

// raidJoinSettings is what both join counters check before they count anything. It reports
// the chat's raid detection settings and true when joins are to be counted: the chat is a
// supergroup, Redis is available, the settings could be read (one bounded read under
// raidSettingsTimeout; an unreadable answer counts nothing) and at least one trigger is on.
// The flood switch counts too, because the join list is also what the flood trigger
// reads to tell new members.
func raidJoinSettings(chat *gotgbot.Chat) (*models.ChatRaidDetection, bool) {
	if chat.Type != "supergroup" || raidRedis() == nil {
		return nil, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), raidSettingsTimeout)
	settings, err := raidSettingsLookup(ctx, chat.Id)
	cancel()
	if err != nil {
		log.Debugf("[RaidDetection] settings of chat %d could not be read, counting nothing: %v", chat.Id, err)
		return nil, false
	}
	if settings == nil || (!settings.JoinEnabled && !settings.FloodEnabled) {
		return nil, false
	}
	return settings, true
}

// raidJoinsCountedFor applies raidJoinCounts to one join. The approved-user read is made
// only for a join that would count otherwise.
func raidJoinsCountedFor(chatID int64, in lockdownJoinInput) bool {
	if !raidJoinCounts(in, false) {
		return false
	}
	return raidJoinCounts(in, approvals.IsUserApproved(chatID, in.MemberID))
}

// raidRecordAndFire stores the counted accounts in the chat's joined set in one script
// call and starts the lockdown when the chat's join surge is on and the number of
// different accounts that joined within the window reaches its threshold.
func raidRecordAndFire(b *gotgbot.Bot, chat *gotgbot.Chat, settings *models.ChatRaidDetection, userIDs []int64) {
	if len(userIDs) == 0 {
		return
	}
	count, err := recordRaidJoins(chat.Id, userIDs)
	if err != nil {
		log.Debugf("[RaidDetection] join counter write for chat %d failed: %v", chat.Id, err)
		return
	}
	if settings.JoinEnabled && count >= settings.JoinThreshold {
		raidFire(b, chat, raidTrigger{
			Kind:   models.LockdownTriggerJoinSurge,
			Count:  count,
			Window: int(raidJoinWindow / time.Second),
		})
	}
}
