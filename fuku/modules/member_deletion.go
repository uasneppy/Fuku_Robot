package modules

import (
	"context"
	"errors"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	log "github.com/sirupsen/logrus"

	"github.com/uasneppy/fuku_robot/fuku/db/approvals"
	"github.com/uasneppy/fuku_robot/fuku/db/deletion"
	"github.com/uasneppy/fuku_robot/fuku/utils/error_handling"
)

// The MemberDeletion module (plan 17-05, DEL-04). It hosts the member message rules that need a
// handler of their own: the edit rule now (a member's message edited later than the group's grace
// is deleted), and the self-destruct watcher from plan 17-07. Both live at memberDeleteGroup, never
// end an update and return ext.ContinueGroups on every path through a named result preset to it.
//
// Unknown never deletes (D-33). An exemption that cannot be read keeps the message: approvals are
// read through the tri-state reader (an error is exempt), an administrator is judged by
// memberDeletionAdminStatus (unknown is exempt) and a pending captcha that cannot be read is
// pending. The readers that turn a failed lookup into a negative answer are never used here.

// memberDeleteGroup is the handler group of the member deletion handlers. It runs after the
// captcha hold at -10 and before the lockdown join guard at -7, and no other handler lives
// there; gotgbot moves to the next handler of a group only on ext.ContinueGroups, which is why
// neither handler of the module ever ends an update.
const memberDeleteGroup = -8

// memberDeletionModule holds the member deletion handlers. It is not a help module, so it never
// calls SetModuleEnabled and /help and the docs generator see no new module.
var memberDeletionModule = moduleStruct{
	moduleName:   "MemberDeletion",
	handlerGroup: memberDeleteGroup,
}

var (
	// memberDeletionApprovedLookup is the tri-state approvals read: an error is an unknown, and an
	// unknown is exempt.
	memberDeletionApprovedLookup = approvals.IsUserApprovedContext
	// memberDeletionMemberLookup is the live read of a member's record. It is a variable only so a
	// test can script a 429, a 5xx, an empty answer or a hang without a mock library.
	memberDeletionMemberLookup = memberDeletionLiveMember
	// memberDeletionAdminSlots bounds the live administrator reads in flight on this replica. A
	// full semaphore answers unknown at once and never waits, so a burst of edits behind a cold
	// admin cache cannot hold dispatcher goroutines.
	memberDeletionAdminSlots = make(chan struct{}, 8)
)

// memberDeletionAdminTimeout bounds one live administrator read.
const memberDeletionAdminTimeout = 3 * time.Second

// memberDeletionAdmin is the answer of an administrator check. Its zero value is unknown.
type memberDeletionAdmin int

const (
	// memberDeletionAdminUnknown: the check could not be answered (an error of any kind, a 429, a
	// 5xx, a timeout, an empty or unrecognised answer, a full semaphore).
	memberDeletionAdminUnknown memberDeletionAdmin = iota
	// memberDeletionNotAdmin: the member is definitely not a creator or administrator.
	memberDeletionNotAdmin
	// memberDeletionIsAdmin: the member is the creator or an administrator.
	memberDeletionIsAdmin
)

// memberDeletionLiveMember reads one member's record from Telegram. An empty answer is an error.
func memberDeletionLiveMember(
	ctx context.Context, b *gotgbot.Bot, chatID, userID int64,
) (gotgbot.MergedChatMember, error) {
	member, err := b.GetChatMemberWithContext(ctx, chatID, userID, nil)
	if err != nil {
		return gotgbot.MergedChatMember{}, err
	}
	if member == nil {
		return gotgbot.MergedChatMember{}, errors.New("empty chat member answer")
	}
	return member.MergeChatMember(), nil
}

// memberDeletionAdminStatus says whether the member is an administrator. The cached admin list
// answers when it knows; otherwise one live getChatMember is made, bounded by
// memberDeletionAdminTimeout and by the semaphore, and classified by status alone: creator and
// administrator are admins; member, restricted, left and kicked are not; anything else, and every
// failure, is unknown. It never uses the antiflood or chat_status checks as its answer, because
// both read a failed lookup on a cold cache as "not an admin" (D-33).
func memberDeletionAdminStatus(b *gotgbot.Bot, chatID, userID int64) memberDeletionAdmin {
	if known, isAdmin := cachedAdminStatus(chatID, userID); known {
		if isAdmin {
			return memberDeletionIsAdmin
		}
		return memberDeletionNotAdmin
	}
	select {
	case memberDeletionAdminSlots <- struct{}{}:
		defer func() { <-memberDeletionAdminSlots }()
	default:
		log.Warnf("[MemberDeletion] the admin check of chat %d is saturated: the message is kept", chatID)
		return memberDeletionAdminUnknown
	}
	ctx, cancel := context.WithTimeout(context.Background(), memberDeletionAdminTimeout)
	defer cancel()
	member, err := memberDeletionMemberLookup(ctx, b, chatID, userID)
	if err != nil {
		log.Warnf("[MemberDeletion] the admin status of user %d in chat %d is unknown: %s",
			userID, chatID, memberReqErrText(err))
		return memberDeletionAdminUnknown
	}
	switch member.Status {
	case gotgbot.ChatMemberStatusCreator, gotgbot.ChatMemberStatusAdministrator:
		return memberDeletionIsAdmin
	case gotgbot.ChatMemberStatusMember, gotgbot.ChatMemberStatusRestricted,
		gotgbot.ChatMemberStatusLeft, gotgbot.ChatMemberStatusKicked:
		return memberDeletionNotAdmin
	}
	return memberDeletionAdminUnknown
}

// memberDeletionExempt reports whether a member's message must be kept: approved, an
// administrator, or either of the two that could not be read. It is the shared exemption of the
// edit rule and, from plan 17-07, the self-destruct watcher (D-33). Approvals go through the
// tri-state reader and an error is exempt; the administrator check runs only for an approvals
// answer of "not approved", and unknown is exempt.
func memberDeletionExempt(b *gotgbot.Bot, chatID, userID int64) bool {
	ctx, cancel := context.WithTimeout(context.Background(), deletionRecordTimeout)
	defer cancel()
	approved, err := memberDeletionApprovedLookup(ctx, chatID, userID)
	if err != nil {
		log.Warnf("[MemberDeletion] approval of user %d in chat %d is unknown: %s", userID, chatID, memberReqErrText(err))
		return true
	}
	if approved {
		return true
	}
	return memberDeletionAdminStatus(b, chatID, userID) != memberDeletionNotAdmin
}

// memberEditPendingCaptcha reports whether the member has a pending captcha attempt, or whether
// that cannot be known: a read error is pending, so the message is kept. Only edits check it
// explicitly; the captcha module holds a pending member's other messages itself at group -10.
func memberEditPendingCaptcha(chatID, userID int64) bool {
	ctx, cancel := context.WithTimeout(context.Background(), deletionRecordTimeout)
	defer cancel()
	anyPending, err := memberReqCaptchaPendingLookup(ctx, chatID)
	if err != nil {
		log.Warnf("[MemberDeletion] captcha state of chat %d is unknown: %s", chatID, memberReqErrText(err))
		return true
	}
	if !anyPending {
		return false
	}
	attempt, err := memberReqCaptchaAttemptLookup(userID, chatID)
	if err != nil {
		log.Warnf("[MemberDeletion] captcha state of user %d in chat %d is unknown: %s",
			userID, chatID, memberReqErrText(err))
		return true
	}
	return attempt != nil
}

// memberEditDue is the pure time rule of the edit handler: a message is due for deletion when it
// was edited later than the grace after it was sent. A missing or non-later edit_date, a message
// with no date and a message older than deletionDropAge (47 hours) by its own date are never due,
// and a gap equal to the grace is kept.
func memberEditDue(date, editDate int64, grace int, now time.Time) bool {
	if date <= 0 || editDate <= date {
		return false
	}
	if now.Sub(time.Unix(date, 0)) > deletionDropAge {
		return false
	}
	return editDate-date > int64(grace)
}

// memberEditGrace reads the group's stored member:edited grace. Only a value the kind accepts (one
// of the five presets) counts: a value written by hand, such as 45, is treated as Off, and an
// unreadable rule deletes nothing.
func memberEditGrace(chatID int64) (grace int, on bool) {
	ctx, cancel := context.WithTimeout(context.Background(), deletionRecordTimeout)
	defer cancel()
	rules, err := deletionRulesLookup(ctx, chatID)
	if err != nil || rules == nil {
		return 0, false
	}
	grace, present := rules.Delays[deletion.KindMemberEdited]
	if !present || deletion.ValidateRuleDelay(deletion.KindMemberEdited, grace) != nil {
		return 0, false
	}
	return grace, true
}

// memberEditedHandler is the raw handler of edited messages. It is a handler of its own and not a
// message handler with edits allowed, so it sees nothing but edits and a new message never reaches
// it (D-06). It is the first handler of the bot that consumes edits.
type memberEditedHandler struct{}

// CheckUpdate matches edited messages only.
func (memberEditedHandler) CheckUpdate(_ *gotgbot.Bot, ctx *ext.Context) bool {
	return ctx.EditedMessage != nil
}

// Name is the handler's unique name.
func (memberEditedHandler) Name() string { return "member_edited" }

// HandleUpdate deletes a member's message edited after the group's grace. It never ends an
// update, never replies and never returns nil: the result is a named value preset to
// ext.ContinueGroups, so a recovered panic cannot leave group -8 and kill a later handler. The
// order is cheapest first and does no I/O before the rule read: the chat type, the sender's shape,
// the pure time check, the stored rule, the approvals and administrator exemptions, the pending
// captcha, then deletionAct with delay 0 (which refuses a Staff Group and a latched chat).
func (memberEditedHandler) HandleUpdate(b *gotgbot.Bot, ctx *ext.Context) (result error) {
	result = ext.ContinueGroups
	defer error_handling.RecoverFromPanic("memberEdited", "Deletion")

	msg := ctx.EditedMessage
	if msg == nil || (msg.Chat.Type != "group" && msg.Chat.Type != "supergroup") {
		return result
	}
	userID, ok := raidFloodSenderCounts(b, ctx)
	if !ok || ctx.EffectiveSender.IsBot() || msg.ViaBot != nil {
		return result
	}
	now := deletionNow()
	if !memberEditDue(msg.Date, msg.EditDate, 0, now) {
		return result
	}
	grace, on := memberEditGrace(msg.Chat.Id)
	if !on || !memberEditDue(msg.Date, msg.EditDate, grace, now) {
		return result
	}
	if memberDeletionExempt(b, msg.Chat.Id, userID) || memberEditPendingCaptcha(msg.Chat.Id, userID) {
		return result
	}
	deletionAct(b, deletionAction{
		ChatID:    msg.Chat.Id,
		MessageID: msg.MessageId,
		Kind:      deletion.KindMemberEdited,
		SentAt:    deletionSentAt(msg.Date),
		Delay:     0,
	})
	return result
}

// LoadMemberDeletion registers the edit handler at memberDeleteGroup. The handler arrives together
// with every exemption it needs, never before.
func LoadMemberDeletion(dispatcher *ext.Dispatcher) {
	dispatcher.AddHandlerToGroup(memberEditedHandler{}, memberDeletionModule.handlerGroup)
}

func init() {
	RegisterLegacyModule("MemberDeletion", 243, LoadMemberDeletion)
}
