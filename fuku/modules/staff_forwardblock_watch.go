package modules

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	log "github.com/sirupsen/logrus"

	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/db/staff"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
	"github.com/uasneppy/fuku_robot/fuku/utils/chat_status"
	"github.com/uasneppy/fuku_robot/fuku/utils/error_handling"
	"github.com/uasneppy/fuku_robot/fuku/utils/helpers"
)

// The forward blocklist watcher. A message a member forwards from a source the Staff Group lists is
// deleted in the group where it was posted, and the list's one action is applied to the member. The
// watcher reads the list through the group's own link, so it acts only in that group (there is no
// fan-out), and it never ends an update: later groups, the raid flood counter and the blacklist
// watcher still see the message. It starts no goroutine, writes nothing to Redis and has no drain.
//
// Unknown never acts: a list, link, owner check, approvals read or admin status that cannot be read
// deletes and punishes nothing.

const (
	// staffForwardBlockGroup is the handler group of the watcher, the only free group below 12. It
	// holds no other handler.
	staffForwardBlockGroup = -9
	// forwardBlockPunishWindow is how long a member is punished once: an album of blocked forwards
	// deletes every item and punishes the member one time inside this window.
	forwardBlockPunishWindow = 60 * time.Second
	// forwardBlockAuthMemoTTL is how long one successful live authority recheck of a link is trusted.
	forwardBlockAuthMemoTTL = 60 * time.Second
	// forwardBlockStateMax bounds the in-process memo and latch tables.
	forwardBlockStateMax = 10000
)

// The failure kinds of the watcher's hourly per-chat log gates.
const (
	forwardBlockWarnCannotAct = iota
	forwardBlockWarnListUnreadable
	forwardBlockWarnAuthUnknown
	forwardBlockWarnDeleteFailed
	forwardBlockWarnPunishFailed
	forwardBlockWarnKinds
)

// forwardBlockWarnGates lets one line per kind per chat per hour through. Each kind has its own gate
// so one kind never hides another.
var forwardBlockWarnGates = [forwardBlockWarnKinds]memberDeleteWarnGate{
	{label: "the forward block failure log"}, {label: "the forward block failure log"},
	{label: "the forward block failure log"}, {label: "the forward block failure log"},
	{label: "the forward block failure log"},
}

// forwardBlockWarnf logs a failure line of one kind for chatID through that kind's gate. Lines carry
// chat and user IDs and the Telegram error code or class only.
func forwardBlockWarnf(kind int, chatID int64, format string, args ...any) {
	if forwardBlockWarnGates[kind].allow(chatID, deletionNow()) {
		log.Warnf(format, args...)
	}
}

// forwardBlockErrClass names an error by its Telegram code or a fixed class, never by its text: a
// wrapped delete error carries a message ID.
func forwardBlockErrClass(err error) string {
	var tgErr *gotgbot.TelegramError
	if errors.As(err, &tgErr) {
		return fmt.Sprintf("telegram error %d", tgErr.Code)
	}
	return "error"
}

// The seams of the watcher's reads, variables only so a test can count, fail or script them.
var (
	forwardBlockLinkLookup   = staff.GetLinkOfGroup
	forwardBlockFreshLink    = staff.GetLinkOfGroupFresh
	forwardBlockRulesLookup  = staff.GetForwardBlockRules
	forwardBlockRecheck      = recheckLink
	forwardBlockRulesTimeout = 3 * time.Second
)

// forwardBlockEntry is one entry of the memo and the latch.
type forwardBlockEntry struct {
	expires     time.Time
	linkID      uint
	staffChatID int64
}

// forwardBlockTable is a mutex-guarded table of expiring entries bounded at forwardBlockStateMax: a
// full table forgets its expired entries and, when none can be forgotten, records nothing.
type forwardBlockTable[K comparable] struct {
	mu      sync.Mutex
	entries map[K]forwardBlockEntry
}

// live returns the unexpired entry of key.
func (t *forwardBlockTable[K]) live(key K, now time.Time) (forwardBlockEntry, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	entry, ok := t.entries[key]
	if !ok || !now.Before(entry.expires) {
		return forwardBlockEntry{}, false
	}
	return entry, true
}

// claim records entry under key unless an unexpired entry exists, in one step, and reports whether it
// did. A full table that cannot make room records nothing and reports false.
func (t *forwardBlockTable[K]) claim(key K, entry forwardBlockEntry, now time.Time) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if current, ok := t.entries[key]; ok && now.Before(current.expires) {
		return false
	}
	return t.putLocked(key, entry, now)
}

// put records entry under key, replacing any entry, and reports whether it was recorded.
func (t *forwardBlockTable[K]) put(key K, entry forwardBlockEntry, now time.Time) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.putLocked(key, entry, now)
}

func (t *forwardBlockTable[K]) putLocked(key K, entry forwardBlockEntry, now time.Time) bool {
	if t.entries == nil {
		t.entries = make(map[K]forwardBlockEntry)
	}
	if _, known := t.entries[key]; !known && len(t.entries) >= forwardBlockStateMax {
		for k, e := range t.entries {
			if !now.Before(e.expires) {
				delete(t.entries, k)
			}
		}
		if len(t.entries) >= forwardBlockStateMax {
			return false
		}
	}
	t.entries[key] = entry
	return true
}

// reset empties the table.
func (t *forwardBlockTable[K]) reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.entries = nil
}

var (
	// forwardBlockAuthMemo remembers, per group, one successful authority recheck of a link, keyed by
	// the group and holding the link row ID, its Staff Group and the expiry. Only a success is kept.
	forwardBlockAuthMemo forwardBlockTable[int64]
	// forwardBlockPunished is the punishment latch of one (group, member) pair for the window.
	forwardBlockPunished forwardBlockTable[[2]int64]
)

// forwardBlockFilter is the handler filter: a message that carries a forward origin.
func forwardBlockFilter(msg *gotgbot.Message) bool {
	return msg != nil && msg.ForwardOrigin != nil
}

// forwardBlockSourceOf returns the chat ID of the source a message was forwarded from, when it is a
// channel, or a group whose anonymous administrator posted it, with a negative ID. A forward from a
// user or a hidden user has no chat and never matches.
func forwardBlockSourceOf(msg *gotgbot.Message) (int64, bool) {
	if msg == nil || msg.ForwardOrigin == nil {
		return 0, false
	}
	merged := msg.ForwardOrigin.MergeMessageOrigin()
	source := merged.Chat
	if source == nil {
		source = merged.SenderChat
	}
	if source == nil || source.Id >= 0 {
		return 0, false
	}
	switch source.Type {
	case "channel", "supergroup", "group":
		return source.Id, true
	}
	return 0, false
}

// forwardBlockWatcher is the group -9 handler. Its result is a named value preset to
// ext.ContinueGroups and the recover comes first, so no path and no recovered panic returns nil
// (which would leave the group and skip every later handler of group 0) or ends the update.
func (moduleStruct) forwardBlockWatcher(b *gotgbot.Bot, ctx *ext.Context) (result error) {
	result = ext.ContinueGroups
	defer error_handling.RecoverFromPanic("forwardBlockWatcher", "Staff")
	forwardBlockProcess(b, ctx)
	return result
}

// forwardBlockProcess is the watcher's pipeline, cheapest first: the chat type, the pure source, the
// cached link gate (the first I/O, nil for an unlinked group and for the Staff Group itself), the
// cached list, the pure sender shape, the exemptions, the bot's rights, the live authority step and
// then the actions.
func forwardBlockProcess(b *gotgbot.Bot, ctx *ext.Context) {
	msg := ctx.EffectiveMessage
	if msg == nil || (msg.Chat.Type != "group" && msg.Chat.Type != "supergroup") {
		return
	}
	source, ok := forwardBlockSourceOf(msg)
	if !ok {
		return
	}
	link := forwardBlockLinkLookup(msg.Chat.Id)
	if link == nil {
		return
	}
	rules, ok := forwardBlockRules(msg.Chat.Id, link.StaffChatID)
	if !ok || !rules.IsBlocked(source) {
		return
	}
	userID, ok := forwardBlockSender(b, ctx)
	if !ok || memberDeletionExempt(b, msg.Chat.Id, userID) {
		return
	}
	if !chat_status.IsBotAdmin(b, ctx, &msg.Chat) {
		forwardBlockWarnf(forwardBlockWarnCannotAct, msg.Chat.Id,
			"[Staff] forward blocklist: the bot is not an administrator of chat %d and cannot act", msg.Chat.Id)
		return
	}
	if !forwardBlockAuthorized(b, msg.Chat.Id, link) {
		return
	}
	forwardBlockEnforce(b, ctx, msg, userID, rules.Action)
}

// forwardBlockRules reads the Staff Group's cached list. An unreadable list acts on nothing.
func forwardBlockRules(chatID, staffChatID int64) (staff.ForwardBlockRules, bool) {
	readCtx, cancel := context.WithTimeout(context.Background(), forwardBlockRulesTimeout)
	defer cancel()
	rules, err := forwardBlockRulesLookup(readCtx, staffChatID)
	if err != nil {
		forwardBlockWarnf(forwardBlockWarnListUnreadable, chatID,
			"[Staff] forward blocklist: the list of chat %d could not be read: %s", chatID, forwardBlockErrClass(err))
		return staff.ForwardBlockRules{}, false
	}
	return rules, true
}

// forwardBlockSender returns the forwarding member's ID when the update's sender shape can be acted
// on: a real user, not a post as a group or channel, an automatic forward, a bot, a Telegram service
// account or the bot itself.
func forwardBlockSender(b *gotgbot.Bot, ctx *ext.Context) (int64, bool) {
	userID, ok := raidFloodSenderCounts(b, ctx)
	if !ok || ctx.EffectiveSender.IsBot() {
		return 0, false
	}
	return userID, true
}

// forwardBlockAuthorized is the live authority step. The link must still exist for the group and
// still name the cached Staff Group, and its maker must still be, live, the creator of both chats
// (recheckLink). A success is trusted for forwardBlockAuthMemoTTL, keyed by the link row, so a link
// that was removed and made again is asked about again; a removed, gone or unknown answer acts on
// nothing and is never remembered.
func forwardBlockAuthorized(b *gotgbot.Bot, chatID int64, cached *models.StaffGroupLink) bool {
	fresh, err := forwardBlockFreshLink(chatID)
	if err != nil {
		forwardBlockWarnf(forwardBlockWarnAuthUnknown, chatID,
			"[Staff] forward blocklist: the link of chat %d could not be read: %s", chatID, forwardBlockErrClass(err))
		return false
	}
	if fresh == nil || fresh.StaffChatID != cached.StaffChatID {
		return false
	}
	now := deletionNow()
	if entry, ok := forwardBlockAuthMemo.live(chatID, now); ok &&
		entry.linkID == fresh.ID && entry.staffChatID == fresh.StaffChatID {
		return true
	}
	switch forwardBlockRecheck(b, *fresh, newStaffOwnerPass()) {
	case staffRecheckOK:
		forwardBlockAuthMemo.put(chatID, forwardBlockEntry{
			expires:     now.Add(forwardBlockAuthMemoTTL),
			linkID:      fresh.ID,
			staffChatID: fresh.StaffChatID,
		}, now)
		return true
	case staffRecheckUnknown:
		forwardBlockWarnf(forwardBlockWarnAuthUnknown, chatID,
			"[Staff] forward blocklist: the owner of chat %d could not be checked", chatID)
	case staffRecheckRemoved, staffRecheckGone:
	}
	return false
}

// forwardBlockEnforce deletes the forwarded message when the bot can, then applies the list's action
// once per member per window when the bot can.
func forwardBlockEnforce(b *gotgbot.Bot, ctx *ext.Context, msg *gotgbot.Message, userID int64, action string) {
	chatID := msg.Chat.Id
	if chat_status.CanBotDelete(b, ctx, &msg.Chat) {
		if err := helpers.DeleteMessageWithErrorHandling(b, chatID, msg.MessageId); err != nil {
			forwardBlockWarnf(forwardBlockWarnDeleteFailed, chatID,
				"[Staff] forward blocklist: the delete in chat %d failed: %s", chatID, forwardBlockErrClass(err))
		}
	} else {
		forwardBlockWarnf(forwardBlockWarnDeleteFailed, chatID,
			"[Staff] forward blocklist: the bot has no delete right in chat %d", chatID)
	}
	punish, acts := forwardBlockPunishments[action]
	if !acts {
		return
	}
	if action != "warn" && !chat_status.CanBotRestrict(b, ctx, &msg.Chat) {
		forwardBlockWarnf(forwardBlockWarnPunishFailed, chatID,
			"[Staff] forward blocklist: the bot has no restrict right in chat %d", chatID)
		return
	}
	now := deletionNow()
	granted := forwardBlockPunished.claim([2]int64{chatID, userID},
		forwardBlockEntry{expires: now.Add(forwardBlockPunishWindow)}, now)
	if !granted {
		return
	}
	if err := punish(b, ctx, userID); err != nil {
		forwardBlockWarnf(forwardBlockWarnPunishFailed, chatID,
			"[Staff] forward blocklist: the %s of user %d in chat %d failed: %s",
			action, userID, chatID, forwardBlockErrClass(err))
	}
}

// forwardBlockPunishments maps the list's action to the function that applies it. none has no entry:
// it is the delete alone.
var forwardBlockPunishments = map[string]func(*gotgbot.Bot, *ext.Context, int64) error{
	"warn": forwardBlockWarn,
	"mute": forwardBlockMute,
	"kick": forwardBlockKick,
	"ban":  forwardBlockBan,
}

// forwardBlockReason is the translated reason of a warn and of a notice. It names neither the source
// nor the Staff Group.
func forwardBlockReason(tr *i18n.Translator) string {
	reason, _ := tr.GetString("staff_fwdblock_reason")
	return reason
}

// forwardBlockWarn issues one automatic warn, exactly as the blacklist's warn action does.
func forwardBlockWarn(b *gotgbot.Bot, ctx *ext.Context, userID int64) error {
	err := warnsModule.warnThisUser(b, ctx, userID, forwardBlockReason(ctxTr(ctx)), "warn", warnSourceAuto)
	if err != nil && !errors.Is(err, ext.EndGroups) && !errors.Is(err, ext.ContinueGroups) {
		return err
	}
	return nil
}

// forwardBlockMute restricts the member with MutedPermissions and no end date, then posts the notice.
func forwardBlockMute(b *gotgbot.Bot, ctx *ext.Context, userID int64) error {
	chatID := ctx.EffectiveMessage.Chat.Id
	if _, err := b.RestrictChatMember(chatID, userID, MutedPermissions, nil); err != nil {
		return err
	}
	return forwardBlockNotice(b, ctx, userID, "blacklists_bl_watcher_muted_user")
}

// forwardBlockKick removes the member (they can join again), then posts the notice.
func forwardBlockKick(b *gotgbot.Bot, ctx *ext.Context, userID int64) error {
	if err := kickMember(b, ctx.EffectiveMessage.Chat.Id, userID); err != nil {
		return err
	}
	return forwardBlockNotice(b, ctx, userID, "blacklists_bl_watcher_kicked_user")
}

// forwardBlockBan bans the member with no end date, then posts the notice.
func forwardBlockBan(b *gotgbot.Bot, ctx *ext.Context, userID int64) error {
	if _, err := b.BanChatMember(ctx.EffectiveMessage.Chat.Id, userID, nil); err != nil {
		return err
	}
	return forwardBlockNotice(b, ctx, userID, "blacklists_bl_watcher_banned_user")
}
