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
	RequiredChecks: []helpers.CheckFunc{requireLockdownGroup(), requireLockdownAuthority(false)},
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
	var parts []string
	if lockdownStartedAutomatically(row) {
		// An automatic lockdown was started by the bot on a trigger, so it says that and
		// names the trigger, rendered from the row's trigger columns in the reader's
		// language, instead of "started by" the bot; it has no reason.
		parts = []string{
			lockdownText(tr, "lockdown_status_active_auto", i18n.TranslationParams{"since": lockdownTime(since)}),
			lockdownTriggerSentence(tr, row.TriggerKind, row.TriggerCount, row.TriggerWindow, row.TriggerPercent),
		}
	} else {
		active := lockdownText(tr, "lockdown_status_active", i18n.TranslationParams{
			"since": lockdownTime(since),
			"name":  lockdownNameToken,
		})
		parts = []string{lockdownSplice(active, lockdownNameToken, lockdownMention(row.StartedBy, row.StartedByName))}
		if line := lockdownReasonLine(tr, row.Reason); line != "" {
			parts = append(parts, line)
		}
	}
	if row.LockedAt == nil {
		parts = append(parts, lockdownText(tr, "lockdown_status_unconfirmed"))
	}
	parts = append(parts, lockdownText(tr, "lockdown_status_removed",
		i18n.TranslationParams{"count": lockdownRemovedSoFar(row.ID, tally)}))
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

// lockdownStartedAutomatically reports whether the bot locked the group on its own: the
// lockdown's trigger is a join surge or a flood. Any other kind, a manual lockdown
// included, reads as it always did.
func lockdownStartedAutomatically(ld *models.ChatLockdown) bool {
	if ld == nil {
		return false
	}
	return ld.TriggerKind == models.LockdownTriggerJoinSurge || ld.TriggerKind == models.LockdownTriggerFlood
}

// lockdownRemovedSoFar is how many joiners a lockdown removed: the joiners under its
// own ban plus the ones "Ban N recent joiners" banned for good, which the run moves to
// kept with detail response_ban and which are still removed people. A read error is
// logged and leaves the banned count alone.
func lockdownRemovedSoFar(lockdownID uint, tally map[string]int64) int64 {
	removed := tally[models.JoinerStateBanned]
	forGood, err := lockdown.CountJoinersWithDetail(lockdownID, models.JoinerStateKept, models.JoinerDetailResponseBan)
	if err != nil {
		log.Warnf("[Lockdown] response bans of lockdown %d could not be counted: %v", lockdownID, err)
		return removed
	}
	return removed + forGood
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
