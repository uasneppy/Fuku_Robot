package modules

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sync/singleflight"

	"github.com/uasneppy/fuku_robot/fuku/db/menuaccess"
	"github.com/uasneppy/fuku_robot/fuku/utils/helpers"
)

// The settings owner gate. By default only a group's owner (its creator) may change the
// bot's settings through the menu; the owner can switch on "Admins can change settings"
// (chat_settings_access), and a group that has no live owner left lets its administrators
// in. settingsGate is the one decision every settings door calls after its own live
// getChatMember of the person, so no door copies a rule. Whatever cannot be established
// refuses: an unreadable switch or an unusable owner-state answer is never a way in.

// settingsGateVerdict is the answer of settingsGate. Its zero value is the refusing one.
type settingsGateVerdict int

const (
	// settingsGateUndecided means the switch or the owner state could not be established.
	// The door answers settings_check_failed and lets nobody in.
	settingsGateUndecided settingsGateVerdict = iota
	// settingsGateIn lets the person in.
	settingsGateIn
	// settingsGateOwnerOnly refuses an administrator because only the owner may change
	// the settings of this group right now.
	settingsGateOwnerOnly
)

// settingsOwnerStateResult is the answer of the owner-state read. Its zero value is unknown.
type settingsOwnerStateResult int

const (
	// settingsOwnerUnknown is any answer that cannot be used: an error, a timeout, an empty
	// list, a list without the presser's own administrator entry, or a creator entry that
	// is the presser.
	settingsOwnerUnknown settingsOwnerStateResult = iota
	// settingsOwnerExists means the group has a live owner.
	settingsOwnerExists
	// settingsOwnerNone means the group has no creator entry, or only a deleted account.
	settingsOwnerNone
)

const (
	// settingsOwnerMemoMaxEntries is the size at which a store first drops expired memos.
	settingsOwnerMemoMaxEntries = 4096
)

var (
	// settingsAccessLookup reads the owner switch fresh from PostgreSQL (D-12). It is a
	// variable only so a test can make the read fail; production code never reassigns it.
	settingsAccessLookup = menuaccess.AdminsAllowedFresh
	// settingsOwnerLookup is the owner-state read. It is a variable only so a test can
	// script the answer; production code never reassigns it.
	settingsOwnerLookup = settingsOwnerState
	// settingsOwnerFetch asks Telegram for the group's administrators. It is a variable
	// only so a test can script the list, block or fail it.
	settingsOwnerFetch = settingsFetchAdministrators
	// settingsOwnerFlight makes concurrent reads for one chat on this replica one call.
	settingsOwnerFlight singleflight.Group
	// settingsOwnerMemo remembers, per chat, when an "an owner exists" answer was read.
	// Only that answer is kept: it can only keep an administrator out.
	settingsOwnerMemo   = map[int64]time.Time{}
	settingsOwnerMemoMu sync.Mutex
	// settingsOwnerMemoTTL is how long an "owner exists" answer is reused. A memo exactly
	// this old is expired. A variable only so a test can shorten it.
	settingsOwnerMemoTTL = 30 * time.Second
	// settingsOwnerLookupTimeout bounds one getChatAdministrators call.
	settingsOwnerLookupTimeout = 3 * time.Second
	// settingsOwnerNow is the memo's clock, a variable only for tests.
	settingsOwnerNow = time.Now
)

// settingsGate decides whether the person may use the settings of chatID. member is the
// live getChatMember record the door already fetched for them. The order is fixed:
// a creator is in with no further read; any status other than administrator is undecided;
// for an administrator the switch is read fresh (an error is undecided, true is in), and
// only with the switch off is the owner state consulted. The switch is read before the
// owner-state memo, so a flip to on is seen at the very next press.
func settingsGate(b *gotgbot.Bot, chatID int64, presser gotgbot.User, member gotgbot.MergedChatMember) settingsGateVerdict {
	switch member.Status {
	case gotgbot.ChatMemberStatusCreator:
		return settingsGateIn
	case gotgbot.ChatMemberStatusAdministrator:
	default:
		return settingsGateUndecided
	}

	ctx, cancel := context.WithTimeout(context.Background(), settingsReadTimeout)
	defer cancel()
	allowed, err := settingsAccessLookup(ctx, chatID)
	if err != nil {
		log.Warnf("[Settings] reading the owner switch of chat %d failed: %v", chatID, err)
		return settingsGateUndecided
	}
	if allowed {
		return settingsGateIn
	}

	switch settingsOwnerLookup(b, chatID, presser.Id) {
	case settingsOwnerExists:
		return settingsGateOwnerOnly
	case settingsOwnerNone:
		return settingsGateIn
	}
	return settingsGateUndecided
}

// settingsFetchAdministrators is the production settingsOwnerFetch: one
// getChatAdministrators call, every entry converted with MergeChatMember as the admin
// cache does.
func settingsFetchAdministrators(ctx context.Context, b *gotgbot.Bot, chatID int64) ([]gotgbot.MergedChatMember, error) {
	admins, err := b.GetChatAdministratorsWithContext(ctx, chatID, nil)
	if err != nil {
		return nil, err
	}
	merged := make([]gotgbot.MergedChatMember, 0, len(admins))
	for _, admin := range admins {
		if admin == nil {
			continue
		}
		merged = append(merged, admin.MergeChatMember())
	}
	return merged, nil
}

// settingsOwnerMemoFresh reports whether an "owner exists" answer for chatID is younger than
// settingsOwnerMemoTTL.
func settingsOwnerMemoFresh(chatID int64) bool {
	settingsOwnerMemoMu.Lock()
	defer settingsOwnerMemoMu.Unlock()
	readAt, ok := settingsOwnerMemo[chatID]
	if !ok {
		return false
	}
	if settingsOwnerNow().Sub(readAt) >= settingsOwnerMemoTTL {
		delete(settingsOwnerMemo, chatID)
		return false
	}
	return true
}

// settingsOwnerMemoStore remembers that chatID has a live owner.
func settingsOwnerMemoStore(chatID int64) {
	now := settingsOwnerNow()
	settingsOwnerMemoMu.Lock()
	defer settingsOwnerMemoMu.Unlock()
	if len(settingsOwnerMemo) >= settingsOwnerMemoMaxEntries {
		for id, readAt := range settingsOwnerMemo {
			if now.Sub(readAt) >= settingsOwnerMemoTTL {
				delete(settingsOwnerMemo, id)
			}
		}
	}
	settingsOwnerMemo[chatID] = now
}

// settingsOwnerState reads whether chatID has a live owner, for an administrator who is not
// the creator. A fresh "owner exists" memo answers with no call. Otherwise one
// getChatAdministrators call, shared per chat by concurrent readers and bounded by
// settingsOwnerLookupTimeout, is evaluated for this presser: only "an owner exists" is
// remembered, and an error, a timeout or an unusable list is never remembered and is unknown.
func settingsOwnerState(b *gotgbot.Bot, chatID, presserID int64) settingsOwnerStateResult {
	if settingsOwnerMemoFresh(chatID) {
		return settingsOwnerExists
	}

	value, err, _ := settingsOwnerFlight.Do(strconv.FormatInt(chatID, 10), func() (any, error) {
		ctx, cancel := context.WithTimeout(context.Background(), settingsOwnerLookupTimeout)
		defer cancel()
		list, err := settingsOwnerFetch(ctx, b, chatID)
		// An answer that arrives after the deadline is not used.
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return list, err
	})
	if err != nil {
		log.Warnf("[Settings] reading the administrators of chat %d failed: %v", chatID, err)
		return settingsOwnerUnknown
	}
	list, _ := value.([]gotgbot.MergedChatMember)

	result := settingsEvaluateOwner(list, presserID)
	if result == settingsOwnerExists {
		settingsOwnerMemoStore(chatID)
	}
	return result
}

// settingsEvaluateOwner judges one administrator list for one presser.
func settingsEvaluateOwner(list []gotgbot.MergedChatMember, presserID int64) settingsOwnerStateResult {
	if len(list) == 0 {
		return settingsOwnerUnknown
	}
	presserListed := false
	liveOwner := false
	for _, entry := range list {
		switch entry.Status {
		case gotgbot.ChatMemberStatusCreator:
			if entry.User.Id == presserID {
				// The presser is the creator by this list but not by their own live record:
				// the two disagree, so nothing can be concluded.
				return settingsOwnerUnknown
			}
			if !settingsCreatorGone(entry.User) {
				liveOwner = true
			}
		case gotgbot.ChatMemberStatusAdministrator:
			if entry.User.Id == presserID {
				presserListed = true
			}
		}
	}
	if !presserListed {
		return settingsOwnerUnknown
	}
	if liveOwner {
		return settingsOwnerExists
	}
	return settingsOwnerNone
}

// settingsCreatorGone reports whether a creator entry of getChatAdministrators is a deleted
// account, so the group has no live owner. Assumption A1: Telegram lists a deleted owner as
// first name "Deleted Account" with no last name and no username. It reuses isDeletedAccount
// for the name, the one definition of a deleted account's name in this repository, and is
// stricter than it: isDeletedAccount only decides a display, but the gate lets administrators
// in on this answer, and a deleted account has neither a last name nor a username, so a live
// owner who merely names themselves that way keeps one of the two and still counts as the
// owner. It is used only by the owner-state read and the roster feeder.
func settingsCreatorGone(u gotgbot.User) bool {
	return isDeletedAccount(u.FirstName) && u.LastName == "" && u.Username == ""
}

// isTelegramClientError reports whether err is a Telegram answer in the 4xx range other
// than 429, also when wrapped. A 429 is a rate limit and says nothing about the request, and a
// 5xx or a transport error says nothing about it either, so none of them is a client error.
// It is the one classifier the settings doors use.
func isTelegramClientError(err error) bool {
	var tgErr *gotgbot.TelegramError
	if !errors.As(err, &tgErr) || tgErr == nil {
		return false
	}
	return tgErr.Code >= 400 && tgErr.Code <= 499 && tgErr.Code != 429
}

// requireSettingsAccess is the in-group /settings check, run after requireLockdownGroup. It
// takes the sender's live member record from lockdownAuthorityMember (a creator or an
// administrator; the anonymous proof and every plain-member refusal are that function's) and
// hands it to settingsGate. A refused administrator gets a plain public reply to the command
// with no keyboard that names neither the switch nor any Staff Group (D-13).
func requireSettingsAccess() helpers.CheckFunc {
	return func(c *helpers.CommandContext) bool {
		member, ok := lockdownAuthorityMember(c, false)
		if !ok {
			return false
		}
		switch settingsGate(c.Bot, c.Chat.Id, *c.User, member) {
		case settingsGateIn:
			return true
		case settingsGateOwnerOnly:
			return lockdownRefuse(c, "settings_refuse_owner_only")
		}
		return lockdownRefuse(c, "settings_check_failed")
	}
}
