package modules

import (
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"
	log "github.com/sirupsen/logrus"

	"github.com/divkix/Alita_Robot/alita/db/lockdown"
	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/utils/chat_status"
	"github.com/divkix/Alita_Robot/alita/utils/error_handling"
)

// lockdownGroupAnonymousBot is the user ID Telegram shows for an administrator who
// acts anonymously, so a join they performed carries it as the performer.
const lockdownGroupAnonymousBot int64 = 1087968824

// lockdownJoinVerdict is what the guard decides about one user who joined a locked
// group.
type lockdownJoinVerdict int

const (
	// lockdownJoinIgnore means the join is none of the lockdown's business (the bot
	// itself joining).
	lockdownJoinIgnore lockdownJoinVerdict = iota
	// lockdownJoinExempt means a live administrator added the person, so they stay.
	lockdownJoinExempt
	// lockdownJoinBan means the person is removed until the lift.
	lockdownJoinBan
	// lockdownJoinDecline means a join request is declined instead of approved.
	lockdownJoinDecline
)

// lockdownJoinInput is everything decideLockdownJoin looks at.
type lockdownJoinInput struct {
	// Path is how the join arrived (models.JoinPath*).
	Path string
	// PerformerID is who caused the join; MemberID is who joined.
	PerformerID, MemberID int64
	MemberIsBot           bool
	BotID                 int64
	// PerformerAnonymousAdmin is true when the performer is the anonymous-admin identity.
	PerformerAnonymousAdmin bool
	// PerformerStatus is the performer's live status in the group, empty when it
	// could not be read.
	PerformerStatus string
}

// decideLockdownJoin is the whole policy for a user who joined a locked group, as a
// pure function. Like decideStaffAction it never lifts a restriction by accident:
// only a user added by a live creator or administrator gets in. Everything else is
// banned, including a self-join, a join the bot itself performed, a bot account and
// any join whose performer's status could not be read. A join request is declined,
// and the bot's own join is ignored.
func decideLockdownJoin(in lockdownJoinInput) lockdownJoinVerdict {
	switch {
	case in.MemberID == in.BotID:
		return lockdownJoinIgnore
	case in.Path == models.JoinPathRequest:
		return lockdownJoinDecline
	case in.MemberIsBot:
		return lockdownJoinBan
	case in.PerformerID == in.MemberID:
		return lockdownJoinBan
	case in.PerformerID == in.BotID:
		return lockdownJoinBan
	case in.PerformerAnonymousAdmin:
		return lockdownJoinExempt
	case in.PerformerStatus == gotgbot.ChatMemberStatusCreator ||
		in.PerformerStatus == gotgbot.ChatMemberStatusAdministrator:
		return lockdownJoinExempt
	}
	return lockdownJoinBan
}

// lockdownNeedsPerformerLookup reports whether the verdict depends on the
// performer's live status, so the guard asks Telegram only when it has to.
func lockdownNeedsPerformerLookup(in lockdownJoinInput) bool {
	return in.MemberID != in.BotID &&
		in.Path != models.JoinPathRequest &&
		!in.MemberIsBot &&
		in.PerformerID != in.MemberID &&
		in.PerformerID != in.BotID &&
		!in.PerformerAnonymousAdmin
}

// lockdownRecordJoin is the call that stores a joiner row. It is a variable only so a
// test can make the write fail; production code never reassigns it.
var lockdownRecordJoin = lockdown.RecordJoin

// lockdownJoinFilter selects a user who became a member of a chat: the same test the
// greetings module uses for its welcome.
func lockdownJoinFilter(u *gotgbot.ChatMemberUpdated) bool {
	wasMember, isMember := chat_status.ExtractJoinLeftStatusChange(u)
	return !wasMember && isMember
}

// lockdownOnJoinMember is the join guard for the chat_member update. It reads the
// chat's lockdown fresh from PostgreSQL, never from Redis, records the joiner and
// returns, and the worker makes the Telegram write. It ends update handling
// (ext.EndGroups) for a joiner it recorded, so no welcome, no captcha challenge and
// no captcha attempt follow, and lets everyone else through (ext.ContinueGroups).
// The performer is judged by a live getChatMember, never the admin cache.
func (m moduleStruct) lockdownOnJoinMember(b *gotgbot.Bot, ctx *ext.Context) error {
	defer error_handling.RecoverFromPanic("lockdownOnJoinMember", "Lockdown")

	chat := ctx.EffectiveChat
	update := ctx.ChatMember
	if chat == nil || update == nil || chat.Type != "supergroup" {
		return ext.ContinueGroups
	}

	active, err := lockdown.GetActiveFresh(chat.Id)
	if err != nil {
		// Fail open: the joiner is still muted by the locked default permissions.
		log.Errorf("[Lockdown] join guard could not read the lockdown of chat %d: %v", chat.Id, err)
		return ext.ContinueGroups
	}
	if active == nil || active.LockedAt == nil {
		return ext.ContinueGroups
	}

	member := update.NewChatMember.MergeChatMember().User
	input := lockdownJoinInput{
		Path:                    models.JoinPathMember,
		PerformerID:             update.From.Id,
		MemberID:                member.Id,
		MemberIsBot:             member.IsBot,
		BotID:                   b.Id,
		PerformerAnonymousAdmin: update.From.Id == lockdownGroupAnonymousBot,
	}
	if lockdownNeedsPerformerLookup(input) {
		// A failed lookup leaves the status empty, which is never exempt.
		if live, lookupErr := lockdownLiveMember(b, chat.Id, update.From.Id); lookupErr != nil {
			log.Warnf("[Lockdown] join guard could not check performer %d in chat %d: %v", update.From.Id, chat.Id, lookupErr)
		} else {
			input.PerformerStatus = live.Status
		}
	}
	verdict := decideLockdownJoin(input)

	rec := lockdown.JoinRecord{
		LockdownID:     active.ID,
		ChatID:         chat.Id,
		UserID:         member.Id,
		FirstName:      lockdownCapRunes(member.FirstName, lockdownNameMaxRunes),
		Username:       lockdownCapRunes(member.Username, lockdownNameMaxRunes),
		IsBot:          member.IsBot,
		Path:           models.JoinPathMember,
		ViaJoinRequest: update.ViaJoinRequest,
		PerformerID:    update.From.Id,
	}
	if update.InviteLink != nil {
		rec.InviteLink = update.InviteLink.InviteLink
	}

	if recordLockdownJoin(b, active, rec, verdict) {
		return ext.ContinueGroups
	}
	return ext.EndGroups
}

// recordLockdownJoin stores the joiner row the verdict calls for and reports whether
// the joiner is let in (true) or handled by the lockdown (false). The row is written
// before anything else can happen to the joiner, so a ban is never placed without a
// record the lift can find; when the write fails the joiner is let in, still muted by
// the locked default permissions, because an unrecorded ban would never be lifted.
func recordLockdownJoin(b *gotgbot.Bot, ld *models.ChatLockdown, rec lockdown.JoinRecord, verdict lockdownJoinVerdict) bool {
	switch verdict {
	case lockdownJoinExempt:
		rec.State = models.JoinerStateExempt
	case lockdownJoinBan:
		rec.State = models.JoinerStatePending
		rec.BanUntil = lockdownBanUntil(time.Now())
	default:
		// Ignore, and the verdicts the chat_member path never produces.
		return true
	}

	row, recorded, err := lockdownRecordJoin(rec)
	if err != nil {
		log.Errorf("[Lockdown] joiner %d of chat %d was not recorded, letting them in: %v", rec.UserID, rec.ChatID, err)
		return true
	}
	if !recorded {
		// A row exists already: a second delivery of the same join, or a rejoin. Plan
		// 04-04 adds the rejoin and cross-path rules; an exempt row stays exempt and
		// anything else is handled by the lockdown.
		return row.State == models.JoinerStateExempt
	}
	if verdict == lockdownJoinExempt {
		return true
	}

	// The lift may have started between the read of the lockdown and the insert. A
	// pending row of a lockdown that is no longer active would never be banned, so
	// the guard withdraws it and lets the joiner go.
	current, err := lockdown.GetFresh(ld.ID)
	if err == nil && (current == nil || current.State != models.LockdownStateActive) {
		if _, delErr := lockdown.DeletePendingJoin(row.ID); delErr != nil {
			log.Errorf("[Lockdown] pending joiner row %d could not be withdrawn: %v", row.ID, delErr)
		}
		return true
	}
	if err != nil {
		log.Errorf("[Lockdown] lockdown %d could not be re-read after recording joiner %d: %v", ld.ID, rec.UserID, err)
	}
	wakeLockdownWorker()
	return false
}

// loadLockdownGuard registers the join guard at the lockdown module's handler group,
// ahead of fed-ban, antiraid, greetings and captcha.
func loadLockdownGuard(dispatcher *ext.Dispatcher) {
	dispatcher.AddHandlerToGroup(
		handlers.NewChatMember(lockdownJoinFilter, lockdownModule.lockdownOnJoinMember),
		lockdownModule.handlerGroup,
	)
}
