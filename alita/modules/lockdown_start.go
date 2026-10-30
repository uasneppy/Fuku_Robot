package modules

import (
	"context"

	"github.com/PaulSonOfLars/gotgbot/v2"
	log "github.com/sirupsen/logrus"

	"github.com/divkix/Alita_Robot/alita/db/lockdown"
	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/utils/chat_status"
)

// lockdownStartOutcome is how one attempt to start a lockdown ended. Every outcome is
// defined: a refusal records nothing and makes no lock call, a lock Telegram refused
// leaves no row, a lock whose answer was lost keeps its row, and a chat that is already
// locked reports the lockdown that runs.
type lockdownStartOutcome int

const (
	// lockdownStartStarted means the lock took effect and was recorded.
	lockdownStartStarted lockdownStartOutcome = iota
	// lockdownStartNotSupergroup means the chat is not a supergroup. Nothing was read
	// or written.
	lockdownStartNotSupergroup
	// lockdownStartBotCheckFailed means the bot's own rights could not be read. Nothing
	// was written.
	lockdownStartBotCheckFailed
	// lockdownStartBotCannotRestrict means the bot is not an administrator with
	// can_restrict_members. Nothing was written.
	lockdownStartBotCannotRestrict
	// lockdownStartPermissionsUnreadable means the group's permissions could not be read,
	// so they could not be put back at the lift. Nothing was written.
	lockdownStartPermissionsUnreadable
	// lockdownStartStateFailed means the lockdown row could not be written or read back.
	lockdownStartStateFailed
	// lockdownStartAlreadyActive means the chat already has an active lockdown, returned
	// as Row. No lock call was made.
	lockdownStartAlreadyActive
	// lockdownStartLockFailed means Telegram refused the lock. The row was removed.
	lockdownStartLockFailed
	// lockdownStartLockUnknown means the lock call may or may not have taken effect. The
	// unconfirmed row is kept, because it holds the only copy of the permissions to
	// restore, and the worker settles it later.
	lockdownStartLockUnknown
)

// lockdownStartParams is what starts a lockdown: the chat, what started it (a trigger
// kind and, for an automatic one, its numbers), the reason, and who is recorded as the
// starter (an admin for /lockdown, the bot for a trigger).
type lockdownStartParams struct {
	Chat        *gotgbot.Chat
	TriggerKind string
	Reason      string
	StarterID   int64
	StarterName string
	// TriggerCount, TriggerWindow (seconds) and TriggerPercent are the numbers behind an
	// automatic trigger; 0 for a manual lockdown.
	TriggerCount   int
	TriggerWindow  int
	TriggerPercent int
}

// lockdownStartResult is what startLockdownRow reports. Row is the started row, or the
// active one for lockdownStartAlreadyActive, or the kept unconfirmed one for
// lockdownStartLockUnknown. BotMember is the bot's own live membership, set once it was
// read.
type lockdownStartResult struct {
	Outcome   lockdownStartOutcome
	Row       *models.ChatLockdown
	BotMember gotgbot.MergedChatMember
}

// startLockdownRow is the one path that starts a lockdown, shared by /lockdown and the
// automatic triggers, so a trigger's lockdown is exactly a /lockdown. It judges the bot's
// own rights live, snapshots the group's raw permissions, inserts the row behind the
// one-active-lockdown index (so of two simultaneous callers exactly one starts and the
// other gets lockdownStartAlreadyActive), sets the locked permissions, confirms the lock
// and wakes the worker that decides the alert. It does not judge any person's authority:
// the caller has. It never touches another chat.
func startLockdownRow(ctx context.Context, b *gotgbot.Bot, p lockdownStartParams) lockdownStartResult {
	chat := p.Chat
	// A basic group cannot honour "banned until the lift", so it is refused before
	// anything is read or written.
	if chat == nil || chat.Type != "supergroup" {
		return lockdownStartResult{Outcome: lockdownStartNotSupergroup}
	}

	// The bot's own rights are read live. An unknown answer fails closed.
	botMember, botResult, botErr := chat_status.FetchBotMember(b, chat.Id)
	if botResult == chat_status.BotMemberUnknown {
		log.Warnf("[Lockdown] bot rights check in chat %d failed: %v", chat.Id, botErr)
		return lockdownStartResult{Outcome: lockdownStartBotCheckFailed}
	}
	if botResult == chat_status.BotMemberMissing ||
		botMember.Status != gotgbot.ChatMemberStatusAdministrator || !botMember.CanRestrictMembers {
		return lockdownStartResult{Outcome: lockdownStartBotCannotRestrict}
	}
	res := lockdownStartResult{BotMember: botMember}

	chatInfo, err := fetchLockdownChat(ctx, b, chat.Id)
	if err != nil {
		log.Warnf("[Lockdown] getChat for chat %d failed: %v", chat.Id, err)
		res.Outcome = lockdownStartPermissionsUnreadable
		return res
	}
	if !lockdownHasPermissions(chatInfo.Permissions) {
		log.Warnf("[Lockdown] getChat for chat %d returned no permissions", chat.Id)
		res.Outcome = lockdownStartPermissionsUnreadable
		return res
	}

	row := &models.ChatLockdown{
		ChatID:            chat.Id,
		TriggerKind:       p.TriggerKind,
		Reason:            p.Reason,
		StartedBy:         p.StarterID,
		StartedByName:     p.StarterName,
		ChatTitle:         lockdownCapRunes(chat.Title, lockdownTitleMaxRunes),
		PrePermissions:    string(chatInfo.Permissions),
		LockedPermissions: lockdownLockedPermissions,
		TriggerCount:      p.TriggerCount,
		TriggerWindow:     p.TriggerWindow,
		TriggerPercent:    p.TriggerPercent,
	}
	started, err := lockdown.Start(row)
	if err != nil {
		log.Errorf("[Lockdown] lockdown of chat %d could not be started: %v", chat.Id, err)
		res.Outcome = lockdownStartStateFailed
		return res
	}
	if !started {
		existing, err := lockdown.GetActiveFresh(chat.Id)
		if err != nil || existing == nil {
			log.Errorf("[Lockdown] the active lockdown of chat %d could not be read: %v", chat.Id, err)
			res.Outcome = lockdownStartStateFailed
			return res
		}
		res.Outcome = lockdownStartAlreadyActive
		res.Row = existing
		return res
	}

	if err := setLockdownPermissions(ctx, b, chat.Id, lockdownLockedPermissions); err != nil {
		log.Warnf("[Lockdown] lock call for chat %d failed: %v", chat.Id, err)
		if !lockdownDefinitiveRefusal(err) {
			// A timeout, a dropped connection or an answer that cannot be read does not
			// say the lock failed: it may have taken effect, and this row holds the only
			// copy of the permissions to restore. It is kept unconfirmed, and the worker
			// settles it from the live permissions after the grace period (a lock that
			// took effect is confirmed, one that did not is dropped), while /unlockdown
			// can already restore the stored permissions.
			res.Outcome = lockdownStartLockUnknown
			res.Row = row
			return res
		}
		if _, delErr := lockdown.DeleteUnconfirmed(row.ID); delErr != nil {
			log.Errorf("[Lockdown] lockdown %d of chat %d could not be removed after a refused lock: %v", row.ID, chat.Id, delErr)
		}
		res.Outcome = lockdownStartLockFailed
		return res
	}
	if _, err := lockdown.ConfirmLocked(row.ID); err != nil {
		if _, retryErr := lockdown.ConfirmLocked(row.ID); retryErr != nil {
			log.Errorf("[Lockdown] lockdown %d of chat %d is locked but not confirmed: %v", row.ID, chat.Id, retryErr)
		}
	}
	// The alert is decided and posted by the worker, so a lock confirmed here, one
	// confirmed by the settle step and a crash in between are one code path.
	wakeLockdownWorker()

	res.Outcome = lockdownStartStarted
	res.Row = row
	return res
}
