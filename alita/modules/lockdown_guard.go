package modules

import (
	"slices"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers/filters/chatjoinrequest"
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

// lockdownActiveLookup is the guard's read of a chat's active lockdown. It is a
// variable only so a test can make the read fail; production code never reassigns it.
var lockdownActiveLookup = lockdown.GetActiveFresh

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

	active, err := lockdownActiveLookup(chat.Id)
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

// lockdownOnJoinMessage is the join guard for the new_chat_members service message.
// It decides every user the message names, records each one the same way the
// chat_member path does and keeps only the users it let in (and the bot itself) in
// ctx.EffectiveMessage.NewChatMembers, which later handlers share, so greetings
// welcome only them. A message that holds nobody it let in ends update handling; one
// that does continues. When every user in the message was banned the message's ID is
// stored on their rows and the worker deletes it after the bans. The performer is
// the message's sender, or the anonymous admin identity (sender_chat equal to the
// group, or the Group Anonymous Bot); it is judged by one live getChatMember.
func (m moduleStruct) lockdownOnJoinMessage(b *gotgbot.Bot, ctx *ext.Context) error {
	defer error_handling.RecoverFromPanic("lockdownOnJoinMessage", "Lockdown")

	chat := ctx.EffectiveChat
	msg := ctx.EffectiveMessage
	if chat == nil || msg == nil || len(msg.NewChatMembers) == 0 || chat.Type != "supergroup" {
		return ext.ContinueGroups
	}

	active, err := lockdownActiveLookup(chat.Id)
	if err != nil {
		// Fail open: the joiners are still muted by the locked default permissions.
		log.Errorf("[Lockdown] join guard could not read the lockdown of chat %d: %v", chat.Id, err)
		return ext.ContinueGroups
	}
	if active == nil || active.LockedAt == nil {
		return ext.ContinueGroups
	}

	var performerID int64
	if msg.From != nil {
		performerID = msg.From.Id
	}
	anonymous := (msg.SenderChat != nil && msg.SenderChat.Id == chat.Id) || performerID == lockdownGroupAnonymousBot

	members := append([]gotgbot.User(nil), msg.NewChatMembers...)
	inputs := make([]lockdownJoinInput, len(members))
	needsLookup := false
	for i, member := range members {
		inputs[i] = lockdownJoinInput{
			Path:                    models.JoinPathService,
			PerformerID:             performerID,
			MemberID:                member.Id,
			MemberIsBot:             member.IsBot,
			BotID:                   b.Id,
			PerformerAnonymousAdmin: anonymous,
		}
		needsLookup = needsLookup || lockdownNeedsPerformerLookup(inputs[i])
	}
	if needsLookup && performerID > 0 {
		// A failed lookup leaves the status empty, which is never exempt.
		if live, lookupErr := lockdownLiveMember(b, chat.Id, performerID); lookupErr != nil {
			log.Warnf("[Lockdown] join guard could not check performer %d in chat %d: %v", performerID, chat.Id, lookupErr)
		} else {
			for i := range inputs {
				inputs[i].PerformerStatus = live.Status
			}
		}
	}

	verdicts := make([]lockdownJoinVerdict, len(members))
	banned, allBanned := 0, true
	for i := range members {
		verdicts[i] = decideLockdownJoin(inputs[i])
		switch verdicts[i] {
		case lockdownJoinIgnore:
		case lockdownJoinBan:
			banned++
		default:
			allBanned = false
		}
	}
	var joinMsgID int64
	if allBanned && banned > 0 {
		joinMsgID = msg.MessageId
	}

	kept := make([]gotgbot.User, 0, len(members))
	for i, member := range members {
		if verdicts[i] == lockdownJoinIgnore {
			kept = append(kept, member)
			continue
		}
		rec := lockdown.JoinRecord{
			LockdownID:  active.ID,
			ChatID:      chat.Id,
			UserID:      member.Id,
			FirstName:   lockdownCapRunes(member.FirstName, lockdownNameMaxRunes),
			Username:    lockdownCapRunes(member.Username, lockdownNameMaxRunes),
			IsBot:       member.IsBot,
			Path:        models.JoinPathService,
			PerformerID: performerID,
			JoinMsgID:   joinMsgID,
		}
		if recordLockdownJoin(b, active, rec, verdicts[i]) {
			kept = append(kept, member)
		}
	}

	// Later handlers share this message, so the filtered list is assigned in place.
	if len(kept) != len(members) {
		msg.NewChatMembers = kept
	}
	if len(kept) == 0 {
		return ext.EndGroups
	}
	return ext.ContinueGroups
}

// lockdownOnJoinRequest is the guard for a chat_join_request. While a confirmed
// lockdown holds the chat it records the request as a joiner row (path request) and
// ends handling, so auto-approve and the approve card never see it; the worker
// declines the request, and the person can ask again after the lift (D-06). The path
// fails closed, unlike the join paths: when the lockdown cannot be read or the row
// cannot be stored the request is left pending, which is harmless, rather than
// handed on to auto-approve. A chat with no confirmed lockdown is none of the
// guard's business (ext.ContinueGroups).
func (m moduleStruct) lockdownOnJoinRequest(b *gotgbot.Bot, ctx *ext.Context) error {
	defer error_handling.RecoverFromPanic("lockdownOnJoinRequest", "Lockdown")

	request := ctx.ChatJoinRequest
	if request == nil || request.Chat.Type != "supergroup" {
		return ext.ContinueGroups
	}

	active, err := lockdownActiveLookup(request.Chat.Id)
	if err != nil {
		log.Errorf("[Lockdown] join guard could not read the lockdown of chat %d, leaving the request of user %d pending: %v",
			request.Chat.Id, request.From.Id, err)
		return ext.EndGroups
	}
	if active == nil || active.LockedAt == nil {
		return ext.ContinueGroups
	}

	rec := lockdown.JoinRecord{
		LockdownID:  active.ID,
		ChatID:      request.Chat.Id,
		UserID:      request.From.Id,
		FirstName:   lockdownCapRunes(request.From.FirstName, lockdownNameMaxRunes),
		Username:    lockdownCapRunes(request.From.Username, lockdownNameMaxRunes),
		IsBot:       request.From.IsBot,
		Path:        models.JoinPathRequest,
		State:       models.JoinerStatePending,
		PerformerID: request.From.Id,
	}
	if request.InviteLink != nil {
		rec.InviteLink = request.InviteLink.InviteLink
	}
	if !recordLockdownRequest(active, rec) {
		// The lift started while the request was being recorded, so the lockdown no
		// longer holds it and the group's own join-request handling applies.
		return ext.ContinueGroups
	}
	return ext.EndGroups
}

// recordLockdownRequest stores the joiner row of a join request and reports whether
// the lockdown holds the request (true) or the lift got there first (false). A new
// request is a new row; a person with a finished row (declined earlier, or let in and
// gone again) gets it reclaimed at once with no dedupe window, because Telegram
// delivers each request once; a request whose row is still pending or being worked on
// is the same one. A failed read or write holds the request (fail closed).
func recordLockdownRequest(ld *models.ChatLockdown, rec lockdown.JoinRecord) bool {
	for range lockdownRecordLooks {
		row, recorded, err := lockdownRecordJoin(rec)
		if err != nil {
			log.Errorf("[Lockdown] join request of user %d in chat %d was not recorded, leaving it pending: %v", rec.UserID, rec.ChatID, err)
			return true
		}
		if recorded {
			return !lockdownPendingHeld(ld, row.ID, rec.UserID, false)
		}
		if !slices.Contains(lockdownReclaimStates, row.State) {
			// Pending or being worked on: the worker declines it.
			wakeLockdownWorker()
			return true
		}
		// The margin keeps a row changed this very moment reclaimable: any age will do.
		won, reclaimErr := lockdown.ReclaimJoin(row.ID, lockdownReclaimStates, time.Now().Add(time.Second), rec)
		if reclaimErr != nil {
			log.Errorf("[Lockdown] joiner row %d could not be reclaimed for the request of user %d, leaving it pending: %v", row.ID, rec.UserID, reclaimErr)
			return true
		}
		if won {
			return !lockdownPendingHeld(ld, row.ID, rec.UserID, true)
		}
	}
	// Another delivery or replica kept winning the row; it is handling this request.
	return true
}

const (
	// lockdownJoinDedupeWindow is how long after a joiner row last changed a second
	// delivery of a join counts as the same join. A delivery that comes later is a
	// new join: the person was unbanned or their request was handled in between.
	lockdownJoinDedupeWindow = 20 * time.Second
	// lockdownRecordLooks is how many times recordLockdownJoin looks at a row again
	// after losing a race for it. A real race needs one look.
	lockdownRecordLooks = 3
)

// lockdownReclaimStates are the finished joiner states a later join of the same user
// may take a row back from. A row still pending or acting is being processed.
var lockdownReclaimStates = []string{
	models.JoinerStateBanned,
	models.JoinerStateBanFailed,
	models.JoinerStateExempt,
	models.JoinerStateDeclined,
	models.JoinerStateDeclineFailed,
	models.JoinerStateCancelled,
}

// recordLockdownJoin stores the joiner row the verdict calls for and reports whether
// the joiner is let in (true) or handled by the lockdown (false). The row is written
// before anything else can happen to the joiner, so a ban is never placed without a
// record the lift can find; when the write fails the joiner is let in, still muted by
// the locked default permissions, because an unrecorded ban would never be lifted.
// Deduplication is the (lockdown, user) row alone, never anything held in Redis: a
// join seen again is decided by the row the first delivery wrote.
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

	for range lockdownRecordLooks {
		row, recorded, err := lockdownRecordJoin(rec)
		if err != nil {
			log.Errorf("[Lockdown] joiner %d of chat %d was not recorded, letting them in: %v", rec.UserID, rec.ChatID, err)
			return true
		}
		if recorded {
			if verdict == lockdownJoinExempt {
				return true
			}
			return lockdownPendingHeld(ld, row.ID, rec.UserID, false)
		}
		if letIn, done := lockdownExistingJoin(ld, row, rec, verdict); done {
			return letIn
		}
	}
	// Another delivery or replica kept winning the row; it is handling this join.
	return false
}

// lockdownPendingHeld runs after a pending row was written. The lift may have started
// between the read of the lockdown and the write, and a pending row of a lockdown
// that is no longer active would never be banned (and would keep the lift from
// finishing), so the guard withdraws it and lets the joiner go: false means the
// lockdown holds the joiner, true that they are let in. A reclaimed row is cancelled
// rather than deleted, because it carries an earlier join's history.
func lockdownPendingHeld(ld *models.ChatLockdown, rowID uint, userID int64, reclaimed bool) bool {
	current, err := lockdown.GetFresh(ld.ID)
	if err == nil && (current == nil || current.State != models.LockdownStateActive) {
		var withdrawErr error
		if reclaimed {
			_, withdrawErr = lockdown.MoveJoiner(rowID, models.JoinerStatePending, models.JoinerStateCancelled, "", false)
		} else {
			_, withdrawErr = lockdown.DeletePendingJoin(rowID)
		}
		if withdrawErr != nil {
			log.Errorf("[Lockdown] pending joiner row %d could not be withdrawn: %v", rowID, withdrawErr)
		}
		return true
	}
	if err != nil {
		log.Errorf("[Lockdown] lockdown %d could not be re-read after recording joiner %d: %v", ld.ID, userID, err)
	}
	wakeLockdownWorker()
	return false
}

// lockdownExistingJoin decides a join whose (lockdown, user) row already exists, from
// the row alone. done false means the call lost a race for the row and the caller
// should look at it again; otherwise letIn says whether the joiner is let in.
//
//   - A row from a join request met by a member or service join is a different event
//     and is reclaimed whatever its age.
//   - A row that is exempt and changed within lockdownJoinDedupeWindow lets the joiner in.
//   - On the chat_member path, an exempt verdict turns a still pending row exempt: that
//     update shows an admin added or approved the user (D-24). Once the worker claimed
//     the row the ban stands until the lift.
//   - Any other row that changed within the window is the same join delivered twice,
//     and so is an older row still pending or acting: it is handled. When this
//     delivery carries the join service message the row remembers it.
//   - An older finished row is a new join (the person was unbanned, or their request
//     was handled) and is reclaimed.
func lockdownExistingJoin(ld *models.ChatLockdown, row *models.LockdownJoiner, rec lockdown.JoinRecord, verdict lockdownJoinVerdict) (letIn, done bool) {
	now := time.Now()
	fresh := !row.UpdatedAt.Before(now.Add(-lockdownJoinDedupeWindow))
	finished := slices.Contains(lockdownReclaimStates, row.State)

	if row.JoinPath == models.JoinPathRequest && rec.Path != models.JoinPathRequest && finished {
		return lockdownReclaim(ld, row, rec, verdict, now)
	}
	if row.State == models.JoinerStateExempt && fresh {
		return true, true
	}
	if rec.Path == models.JoinPathMember && verdict == lockdownJoinExempt && row.State == models.JoinerStatePending {
		moved, err := lockdown.MoveJoiner(row.ID, models.JoinerStatePending, models.JoinerStateExempt, "", false)
		if err != nil {
			log.Errorf("[Lockdown] joiner row %d could not be made exempt: %v", row.ID, err)
			return false, true
		}
		return moved, true
	}
	if fresh || !finished {
		switch row.State {
		case models.JoinerStatePending, models.JoinerStateActing, models.JoinerStateBanned:
			if rec.JoinMsgID != 0 {
				if err := lockdown.SetJoinMsg(row.ID, rec.JoinMsgID); err != nil {
					log.Errorf("[Lockdown] join message of joiner row %d was not stored: %v", row.ID, err)
				} else {
					wakeLockdownWorker()
				}
			}
		}
		return false, true
	}
	return lockdownReclaim(ld, row, rec, verdict, now.Add(-lockdownJoinDedupeWindow))
}

// lockdownReclaim takes a finished row back for a new join. notAfter is the cut-off
// the row's last change must be older than. done false means another caller won.
func lockdownReclaim(ld *models.ChatLockdown, row *models.LockdownJoiner, rec lockdown.JoinRecord, verdict lockdownJoinVerdict, notAfter time.Time) (letIn, done bool) {
	won, err := lockdown.ReclaimJoin(row.ID, lockdownReclaimStates, notAfter, rec)
	if err != nil {
		// Fail open, like a failed insert: a ban the lift cannot find is never undone.
		log.Errorf("[Lockdown] joiner row %d could not be reclaimed for user %d, letting them in: %v", row.ID, rec.UserID, err)
		return true, true
	}
	if !won {
		return false, false
	}
	if verdict == lockdownJoinExempt {
		return true, true
	}
	return lockdownPendingHeld(ld, row.ID, rec.UserID, true), true
}

// lockdownKickedFilter selects a member who was banned: the user was a member and the
// new status is kicked.
func lockdownKickedFilter(u *gotgbot.ChatMemberUpdated) bool {
	wasMember, isMember := chat_status.ExtractJoinLeftStatusChange(u)
	return wasMember && !isMember && u.NewChatMember.MergeChatMember().Status == gotgbot.ChatMemberStatusKicked
}

// lockdownOnKicked ends handling of the update that reports the lockdown's own ban,
// so the greetings module sends no goodbye for a raider the lockdown removed and
// leaves captcha rows alone (the joiner never got a challenge). The ban is
// recognised by its end date: a joiner row of this chat and user whose ban_until the
// update's until_date matches. The update's sender is the bot for every bot ban, so
// it says nothing. Any other ban, and a lookup that fails (logged), continues, so a
// deliberate ban still gets the group's goodbye. It reads no lockdown state, so it
// works during the lift as well.
func (m moduleStruct) lockdownOnKicked(b *gotgbot.Bot, ctx *ext.Context) error {
	defer error_handling.RecoverFromPanic("lockdownOnKicked", "Lockdown")

	chat := ctx.EffectiveChat
	update := ctx.ChatMember
	if chat == nil || update == nil {
		return ext.ContinueGroups
	}
	banned := update.NewChatMember.MergeChatMember()
	own, err := lockdown.HasJoinerBanFresh(chat.Id, banned.User.Id, banned.UntilDate)
	if err != nil {
		log.Errorf("[Lockdown] ban of user %d in chat %d could not be matched to a joiner: %v", banned.User.Id, chat.Id, err)
		return ext.ContinueGroups
	}
	if own {
		return ext.EndGroups
	}
	return ext.ContinueGroups
}

// loadLockdownGuard registers the join guard at the lockdown module's handler group,
// ahead of fed-ban, antiraid, greetings and captcha. The service message handler also
// accepts messages sent by bots, because a bot or an anonymous administrator can add
// users.
func loadLockdownGuard(dispatcher *ext.Dispatcher) {
	dispatcher.AddHandlerToGroup(
		handlers.NewChatMember(lockdownJoinFilter, lockdownModule.lockdownOnJoinMember),
		lockdownModule.handlerGroup,
	)
	dispatcher.AddHandlerToGroup(
		handlers.NewMessage(func(m *gotgbot.Message) bool { return m.NewChatMembers != nil }, lockdownModule.lockdownOnJoinMessage).SetAllowBot(true),
		lockdownModule.handlerGroup,
	)
	dispatcher.AddHandlerToGroup(
		handlers.NewChatMember(lockdownKickedFilter, lockdownModule.lockdownOnKicked),
		lockdownModule.handlerGroup,
	)
	dispatcher.AddHandlerToGroup(
		handlers.NewChatJoinRequest(chatjoinrequest.All, lockdownModule.lockdownOnJoinRequest),
		lockdownModule.handlerGroup,
	)
}
