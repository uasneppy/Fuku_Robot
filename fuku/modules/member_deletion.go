package modules

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers/filters/message"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sync/singleflight"

	"github.com/uasneppy/fuku_robot/fuku/db/approvals"
	"github.com/uasneppy/fuku_robot/fuku/db/deletion"
	"github.com/uasneppy/fuku_robot/fuku/utils/cache"
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
	// memberDeletionAdminWarm loads a chat's administrator list into the admin cache. It is the
	// shared singleflight loader, so concurrent messages of one chat make one getChatAdministrators
	// call; it is a variable only so a test can keep the cache cold or count the loads.
	memberDeletionAdminWarm = func(b *gotgbot.Bot, chatID int64) { cache.LoadAdminCache(b, chatID) }
)

// The failure lines of this file (WR-01, SEC-17-01) run per message, so a Telegram or database
// incident in a busy group would write one Warn per message. Each kind of line has its own hourly
// per-chat gate (the batcher's gate type), so one kind never hides another. The lines carry IDs
// and the error class only.
const (
	memberDeletionWarnSaturated = iota
	memberDeletionWarnAdminUnknown
	memberDeletionWarnApprovalsUnknown
	memberDeletionWarnCaptchaChat
	memberDeletionWarnCaptchaUser
	memberDeletionWarnKinds
)

var memberDeletionWarnGates = [memberDeletionWarnKinds]memberDeleteWarnGate{
	{label: "the member deletion failure log"}, {label: "the member deletion failure log"},
	{label: "the member deletion failure log"}, {label: "the member deletion failure log"},
	{label: "the member deletion failure log"},
}

// memberDeletionWarnf logs a failure line of one kind for chatID through that kind's hourly gate.
func memberDeletionWarnf(kind int, chatID int64, format string, args ...any) {
	if memberDeletionWarnGates[kind].allow(chatID, deletionNow()) {
		log.Warnf(format, args...)
	}
}

// memberDeletionWarmBackoffFor is how long a chat is not warmed again after a load that left the
// admin cache without an entry (a failing getChatAdministrators stores nothing).
const memberDeletionWarmBackoffFor = 30 * time.Second

// memberDeletionWarmBackoff remembers the chats whose last warm left no cache entry, so a
// Telegram incident costs one load per chat per window and not one per message. The map is
// bounded: a full map forgets its expired entries and, when none expired, records nothing.
var memberDeletionWarmBackoff struct {
	mu    sync.Mutex
	until map[int64]time.Time
}

// memberDeletionWarmBlocked reports whether chatID is inside its warm backoff at now.
func memberDeletionWarmBlocked(chatID int64, now time.Time) bool {
	memberDeletionWarmBackoff.mu.Lock()
	defer memberDeletionWarmBackoff.mu.Unlock()
	return now.Before(memberDeletionWarmBackoff.until[chatID])
}

// memberDeletionWarmMark starts the warm backoff of chatID at now.
func memberDeletionWarmMark(chatID int64, now time.Time) {
	memberDeletionWarmBackoff.mu.Lock()
	defer memberDeletionWarmBackoff.mu.Unlock()
	if memberDeletionWarmBackoff.until == nil {
		memberDeletionWarmBackoff.until = make(map[int64]time.Time)
	}
	if _, known := memberDeletionWarmBackoff.until[chatID]; !known &&
		len(memberDeletionWarmBackoff.until) >= memberDeleteWarnMax {
		for id, end := range memberDeletionWarmBackoff.until {
			if !now.Before(end) {
				delete(memberDeletionWarmBackoff.until, id)
			}
		}
		if len(memberDeletionWarmBackoff.until) >= memberDeleteWarnMax {
			return
		}
	}
	memberDeletionWarmBackoff.until[chatID] = now.Add(memberDeletionWarmBackoffFor)
}

// memberDeletionWarmFlight makes every concurrent warm of one chat one load.
var memberDeletionWarmFlight singleflight.Group

// memberDeletionHasAdminEntry reports whether the admin cache holds an entry for the chat, of any
// kind (a list, an empty list or a negative answer).
func memberDeletionHasAdminEntry(chatID int64) bool {
	ok, cached := cache.GetAdminCacheList(chatID)
	return ok && cached.Cached
}

// memberDeletionWarmAdminList loads the chat's administrator list once when the cache has no
// entry for it, waiting at most until ctx ends (the load itself cannot be cancelled and finishes
// on its own goroutine, so a slow answer still warms the cache for the next message). It does
// nothing while the chat is in its backoff or when the cache already holds an entry, a negative
// one included: those answers are stored for their own TTL, so asking again would only repeat the
// bot's own getChatMember on every message.
func memberDeletionWarmAdminList(ctx context.Context, b *gotgbot.Bot, chatID int64) {
	now := deletionNow()
	if memberDeletionWarmBlocked(chatID, now) || memberDeletionHasAdminEntry(chatID) {
		return
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		// The flight re-reads the cache first: a flight that starts right after another one ended
		// finds the list that one stored and loads nothing. The leader recovers inside the function,
		// because a singleflight leader that panics crashes the process.
		_, _, _ = memberDeletionWarmFlight.Do(strconv.FormatInt(chatID, 10), func() (any, error) {
			defer error_handling.RecoverFromPanic("memberDeletionWarm", "Deletion")
			if !memberDeletionHasAdminEntry(chatID) {
				memberDeletionAdminWarm(b, chatID)
			}
			return nil, nil
		})
	}()
	select {
	case <-done:
	case <-ctx.Done():
	}
	if !memberDeletionHasAdminEntry(chatID) {
		memberDeletionWarmMark(chatID, now)
	}
}

// memberDeletionAdminTimeout bounds one live administrator read. It is a variable only so a test
// can shorten it.
var memberDeletionAdminTimeout = 3 * time.Second

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
// answers when it knows. When it does not, the list is warmed once through the shared loader
// (memberDeletionWarmAdminList: one getChatAdministrators per chat however many messages arrive,
// then every later message is answered from the cache) and read again; only when the cache still
// cannot say is one live getChatMember made for the member. Both steps run behind the same
// semaphore and under the same memberDeletionAdminTimeout, and the answer is classified by status
// alone: creator and administrator are admins; member, restricted, left and kicked are not;
// anything else, and every failure, is unknown. It never uses the antiflood or chat_status checks
// as its answer, because both read a failed lookup on a cold cache as "not an admin" (D-33).
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
		memberDeletionWarnf(memberDeletionWarnSaturated, chatID, "[MemberDeletion] the admin check of chat %d is saturated: the message is kept", chatID)
		return memberDeletionAdminUnknown
	}
	ctx, cancel := context.WithTimeout(context.Background(), memberDeletionAdminTimeout)
	defer cancel()
	memberDeletionWarmAdminList(ctx, b, chatID)
	if known, isAdmin := cachedAdminStatus(chatID, userID); known {
		if isAdmin {
			return memberDeletionIsAdmin
		}
		return memberDeletionNotAdmin
	}
	member, err := memberDeletionMemberLookup(ctx, b, chatID, userID)
	if err != nil {
		memberDeletionWarnf(memberDeletionWarnAdminUnknown, chatID, "[MemberDeletion] the admin status of user %d in chat %d is unknown: %s",
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
		memberDeletionWarnf(memberDeletionWarnApprovalsUnknown, chatID, "[MemberDeletion] approval of user %d in chat %d is unknown: %s",
			userID, chatID, memberReqErrText(err))
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
		memberDeletionWarnf(memberDeletionWarnCaptchaChat, chatID, "[MemberDeletion] captcha state of chat %d is unknown: %s", chatID, memberReqErrText(err))
		return true
	}
	if !anyPending {
		return false
	}
	attempt, err := memberReqCaptchaAttemptLookup(userID, chatID)
	if err != nil {
		memberDeletionWarnf(memberDeletionWarnCaptchaUser, chatID, "[MemberDeletion] captcha state of user %d in chat %d is unknown: %s",
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

// memberContentMessage reports whether a message is a member's own content: false for every
// message deletionServiceKindOf recognises (the service rules own those) and for the creation and
// migration notices, which are the group's own history; true for text, media, polls, forwards and
// everything else a member can post.
func memberContentMessage(msg *gotgbot.Message) bool {
	if msg == nil {
		return false
	}
	if _, service := deletionServiceKindOf(msg); service {
		return false
	}
	return !msg.GroupChatCreated && !msg.SupergroupChatCreated && !msg.ChannelChatCreated &&
		msg.MigrateToChatId == 0 && msg.MigrateFromChatId == 0 && msg.ForumTopicCreated == nil
}

// memberSelfDestructDelay reads the group's stored member:selfdestruct delay. Only a value the kind
// accepts (one of the six presets) counts: a value written by hand, such as 45, is Off, and an
// unreadable rule queues nothing (D-10, D-28, C-05).
func memberSelfDestructDelay(chatID int64) (delay int, on bool) {
	ctx, cancel := context.WithTimeout(context.Background(), deletionRecordTimeout)
	defer cancel()
	rules, err := deletionRulesLookup(ctx, chatID)
	if err != nil || rules == nil {
		return 0, false
	}
	delay, present := rules.Delays[deletion.KindMemberSelfDestruct]
	if !present || deletion.ValidateRuleDelay(deletion.KindMemberSelfDestruct, delay) != nil {
		return 0, false
	}
	return delay, true
}

// memberPinnedTTL is how long a pin is remembered in process (WR-03). It only has to outlive the
// time an offered entry can wait in the batcher's buffer and its screening (a flush is bounded by
// deletionRecordTimeout, 5 s), with margin.
const memberPinnedTTL = 10 * time.Second

// memberPinnedMax bounds the set of remembered pins; it is a variable only so a test can shrink it.
var memberPinnedMax = 4096

type memberPinKey struct{ chat, message int64 }

// memberPinnedNotes holds the (chat, message) keys of recently pinned messages on this replica. It
// is a bounded in-process set: expired notes are pruned on every write, and when the set is still
// full the oldest note is evicted, so the newest pin is always remembered.
var memberPinnedNotes struct {
	mu sync.Mutex
	at map[memberPinKey]time.Time
}

// memberPinnedNote remembers that the message was pinned at now.
func memberPinnedNote(chatID, messageID int64, now time.Time) {
	memberPinnedNotes.mu.Lock()
	defer memberPinnedNotes.mu.Unlock()
	if memberPinnedNotes.at == nil {
		memberPinnedNotes.at = make(map[memberPinKey]time.Time)
	}
	for key, at := range memberPinnedNotes.at {
		if now.Sub(at) > memberPinnedTTL {
			delete(memberPinnedNotes.at, key)
		}
	}
	key := memberPinKey{chatID, messageID}
	if _, known := memberPinnedNotes.at[key]; !known {
		for len(memberPinnedNotes.at) >= max(memberPinnedMax, 1) {
			var oldest memberPinKey
			var oldestAt time.Time
			first := true
			for k, at := range memberPinnedNotes.at {
				if first || at.Before(oldestAt) {
					oldest, oldestAt, first = k, at, false
				}
			}
			delete(memberPinnedNotes.at, oldest)
		}
	}
	memberPinnedNotes.at[key] = now
}

// memberPinnedRecently reports whether the message was pinned within memberPinnedTTL of now.
func memberPinnedRecently(chatID, messageID int64, now time.Time) bool {
	memberPinnedNotes.mu.Lock()
	defer memberPinnedNotes.mu.Unlock()
	at, known := memberPinnedNotes.at[memberPinKey{chatID, messageID}]
	return known && now.Sub(at) <= memberPinnedTTL
}

// memberDeleteDropPinned drops one pending self-destruct row; it is a variable only so a test can
// make it fail.
var memberDeleteDropPinned = deletion.DropPendingMessage

// memberDeletePinned keeps a pinned message out of the self-destruct queue (D-11). It first
// remembers the pin in process, so a flush that holds the message in its buffer, or is already
// screening it, drops it instead of writing it (WR-03: this closes that window on this replica only;
// a pin handled by another replica stays the accepted limit AR-5), and then drops the message's
// pending row, retrying the database drop once because the pin event is never delivered again. Each
// attempt runs on a fresh bounded context and a failure is logged by chat ID only; the only cost of
// a drop that fails twice is that the pinned message is still deleted when its time comes.
func memberDeletePinned(chatID, messageID int64) {
	memberPinnedNote(chatID, messageID, deletionNow())
	var err error
	for range 2 {
		ctx, cancel := context.WithTimeout(context.Background(), deletionRecordTimeout)
		_, err = memberDeleteDropPinned(ctx, chatID, messageID, deletion.KindMemberSelfDestruct)
		cancel()
		if err == nil {
			return
		}
	}
	log.Warnf("[MemberDeletion] a pinned message of chat %d could not be taken off the self-destruct queue: %s",
		chatID, memberReqErrText(err))
}

// memberSelfDestructWatch offers every message of a real member to the batcher when the group's
// member:selfdestruct rule is on (D-09, D-10, D-27, D-29). It never calls deletionAct or
// deletionSchedule, never writes to the database and never calls Telegram itself: the batcher
// writes the queue rows and the worker deletes. It never ends an update and never returns nil: the
// result is a named value preset to ext.ContinueGroups, so a recovered panic cannot leave group -8
// and kill a later handler. The order is cheapest first: a pinned_message service message drops the
// pinned message's row; the chat type; the sender's shape; member content (no service message);
// the stored rule; the approvals and administrator exemptions, which treat an unknown as exempt
// (D-33); the offer. There is no captcha lookup here, because group -10 ends a pending member's
// update before this group runs.
func (moduleStruct) memberSelfDestructWatch(b *gotgbot.Bot, ctx *ext.Context) (result error) {
	result = ext.ContinueGroups
	defer error_handling.RecoverFromPanic("memberSelfDestructWatch", "Deletion")

	msg := ctx.EffectiveMessage
	if msg == nil || (msg.Chat.Type != "group" && msg.Chat.Type != "supergroup") {
		return result
	}
	if msg.PinnedMessage != nil {
		memberDeletePinned(msg.Chat.Id, msg.PinnedMessage.GetMessageId())
		return result
	}
	userID, ok := raidFloodSenderCounts(b, ctx)
	if !ok || ctx.EffectiveSender.IsBot() || !memberContentMessage(msg) {
		return result
	}
	delay, on := memberSelfDestructDelay(msg.Chat.Id)
	if !on || memberDeletionExempt(b, msg.Chat.Id, userID) {
		return result
	}
	memberDeleteOffer(deletion.Entry{
		ChatID:    msg.Chat.Id,
		MessageID: msg.MessageId,
		Kind:      deletion.KindMemberSelfDestruct,
		SentAt:    deletionSentAt(msg.Date),
		DelaySecs: delay,
	})
	return result
}

// LoadMemberDeletion registers the edit handler and the self-destruct watcher at memberDeleteGroup.
// The watcher is registered with no SetAllow* (no edits, channel posts or bot senders) and arrives
// together with every exemption, the pin drop and the /pin hook, never before.
func LoadMemberDeletion(dispatcher *ext.Dispatcher) {
	dispatcher.AddHandlerToGroup(memberEditedHandler{}, memberDeletionModule.handlerGroup)
	dispatcher.AddHandlerToGroup(
		handlers.NewMessage(message.All, memberDeletionModule.memberSelfDestructWatch),
		memberDeletionModule.handlerGroup,
	)
}

func init() {
	RegisterLegacyModule("MemberDeletion", 243, LoadMemberDeletion)
}
