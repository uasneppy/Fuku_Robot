package modules

import (
	"context"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	log "github.com/sirupsen/logrus"

	"github.com/divkix/Alita_Robot/alita/db/lang"
	"github.com/divkix/Alita_Robot/alita/db/lockdown"
	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/i18n"
	"github.com/divkix/Alita_Robot/alita/utils/helpers"
)

// lockdownStatusDesc is deliberately not Disableable. The reason and who locked the
// group are for its admins only, so the check is a live getChatMember of the sender
// (the creator or any administrator), never the admin cache.
var lockdownStatusDesc = helpers.CommandDescriptor{
	Name:           "lockdownstatus",
	RequiredChecks: []helpers.CheckFunc{helpers.RequireGroup(), requireLockdownAuthority(false)},
}

// lockdownStatus tells an admin whether the group is locked, since when (UTC), by
// whom, why, how many joiners were removed so far and whether someone changed the
// group's permissions by hand. A lockdown that is being lifted shows who lifted it
// and how far the unbanning is. It only reads: the database rows, and the group's
// current permissions from Telegram. It writes nothing and makes no restrict or ban
// call.
func (m moduleStruct) lockdownStatus(b *gotgbot.Bot, ctx *ext.Context) error {
	chat := ctx.EffectiveChat
	msg := ctx.EffectiveMessage
	if chat == nil || msg == nil {
		return ext.EndGroups
	}
	tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
	reply := func(text string) error {
		replyLockdown(b, msg, text)
		return ext.EndGroups
	}
	failed := func() error { return reply(lockdownText(tr, "lockdown_status_failed")) }

	row, err := lockdown.GetCurrentFresh(chat.Id)
	if err != nil {
		return failed()
	}
	if row == nil {
		return reply(lockdownText(tr, "lockdown_not_active"))
	}
	tally, err := lockdown.TallyJoiners(row.ID)
	if err != nil {
		return failed()
	}

	if row.State == models.LockdownStateLifting {
		return reply(lockdownLiftingStatusText(tr, row, tally))
	}

	since := row.CreatedAt
	if row.LockedAt != nil {
		since = *row.LockedAt
	}
	active := lockdownText(tr, "lockdown_status_active", i18n.TranslationParams{
		"since": lockdownTime(since),
		"name":  lockdownNameToken,
	})
	parts := []string{lockdownSplice(active, lockdownNameToken, lockdownMention(row.StartedBy, row.StartedByName))}
	if line := lockdownReasonLine(tr, row.Reason); line != "" {
		parts = append(parts, line)
	}
	if row.LockedAt == nil {
		parts = append(parts, lockdownText(tr, "lockdown_status_unconfirmed"))
	}
	parts = append(parts, lockdownText(tr, "lockdown_status_removed",
		i18n.TranslationParams{"count": tally[models.JoinerStateBanned]}))
	if count := tally[models.JoinerStateBanFailed]; count > 0 {
		parts = append(parts, lockdownText(tr, "lockdown_status_ban_failed", i18n.TranslationParams{"count": count}))
	}
	if count := tally[models.JoinerStateDeclined]; count > 0 {
		parts = append(parts, lockdownText(tr, "lockdown_status_declined", i18n.TranslationParams{"count": count}))
	}
	// Until Telegram confirmed the lock the permissions may not have changed yet, so
	// comparing them would only raise a false warning.
	if row.LockedAt != nil {
		if line := lockdownPermissionsLine(b, tr, chat.Id, row); line != "" {
			parts = append(parts, line)
		}
	}
	return reply(lockdownJoinLines(parts))
}

// lockdownLiftingStatusText is the status of a lockdown whose permissions were
// restored and whose removed joiners are still being unbanned: who lifted it and
// how many of the joiners are settled.
func lockdownLiftingStatusText(tr *i18n.Translator, row *models.ChatLockdown, tally map[string]int64) string {
	done := tally[models.JoinerStateUnbanned] + tally[models.JoinerStateKept] + tally[models.JoinerStateUnbanFailed]
	total := done + tally[models.JoinerStateBanned] + tally[models.JoinerStateUnbanning]
	var lifterID int64
	if row.LiftedBy != nil {
		lifterID = *row.LiftedBy
	}
	text := lockdownText(tr, "lockdown_status_lifting", i18n.TranslationParams{
		"name":  lockdownNameToken,
		"done":  done,
		"total": total,
	})
	return lockdownJoinLines([]string{lockdownSplice(text, lockdownNameToken, lockdownMention(lifterID, row.LiftedByName))})
}

// lockdownPermissionsLine compares the group's live permissions with the locked set
// and returns the manual-change warning when they differ, the "could not check" line
// when the permissions cannot be read or compared, and "" when they match.
func lockdownPermissionsLine(b *gotgbot.Bot, tr *i18n.Translator, chatID int64, row *models.ChatLockdown) string {
	live, err := fetchLockdownChat(context.Background(), b, chatID)
	if err != nil || !lockdownHasPermissions(live.Permissions) {
		log.Warnf("[Lockdown] status: permissions of chat %d unreadable: %v", chatID, err)
		return lockdownText(tr, "lockdown_status_permissions_unknown")
	}
	same, err := samePermissions(string(live.Permissions), row.LockedPermissions)
	if err != nil {
		log.Warnf("[Lockdown] status: permissions of chat %d not comparable: %v", chatID, err)
		return lockdownText(tr, "lockdown_status_permissions_unknown")
	}
	if !same {
		return lockdownText(tr, "lockdown_status_manual_change")
	}
	return ""
}
