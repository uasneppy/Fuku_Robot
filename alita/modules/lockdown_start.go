package modules

import (
	"context"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/divkix/Alita_Robot/alita/db/models"
)

// lockdownStartOutcome is how one attempt to start a lockdown ended.
type lockdownStartOutcome int

const (
	// lockdownStartStarted means the lock took effect and was recorded.
	lockdownStartStarted lockdownStartOutcome = iota
	// lockdownStartNotSupergroup means the chat is not a supergroup.
	lockdownStartNotSupergroup
	// lockdownStartBotCheckFailed means the bot's own rights could not be read.
	lockdownStartBotCheckFailed
	// lockdownStartBotCannotRestrict means the bot is not an administrator with
	// can_restrict_members.
	lockdownStartBotCannotRestrict
	// lockdownStartPermissionsUnreadable means the group's permissions could not be read.
	lockdownStartPermissionsUnreadable
	// lockdownStartStateFailed means the lockdown row could not be written or read.
	lockdownStartStateFailed
	// lockdownStartAlreadyActive means the chat already has an active lockdown.
	lockdownStartAlreadyActive
	// lockdownStartLockFailed means Telegram refused the lock.
	lockdownStartLockFailed
	// lockdownStartLockUnknown means the lock call's outcome is unknown.
	lockdownStartLockUnknown
)

// lockdownStartParams is what starts a lockdown.
type lockdownStartParams struct {
	Chat                                        *gotgbot.Chat
	TriggerKind, Reason                         string
	StarterID                                   int64
	StarterName                                 string
	TriggerCount, TriggerWindow, TriggerPercent int
}

// lockdownStartResult is what startLockdownRow reports.
type lockdownStartResult struct {
	Outcome   lockdownStartOutcome
	Row       *models.ChatLockdown
	BotMember gotgbot.MergedChatMember
}

// startLockdownRow is not implemented yet.
func startLockdownRow(_ context.Context, _ *gotgbot.Bot, _ lockdownStartParams) lockdownStartResult {
	return lockdownStartResult{}
}
