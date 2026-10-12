package modules

import (
	"context"
	"fmt"
	"html"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	log "github.com/sirupsen/logrus"

	"github.com/divkix/Alita_Robot/alita/db/lockdown"
	"github.com/divkix/Alita_Robot/alita/db/logchannels"
	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/db/staff"
	"github.com/divkix/Alita_Robot/alita/i18n"
	"github.com/divkix/Alita_Robot/alita/utils/actionlog"
	"github.com/divkix/Alita_Robot/alita/utils/formatting"
)

// A lockdown's alert is a message posted where staff will see it. The worker decides
// it once per lockdown (lockdownPlanAlerts, one conditional update that also creates
// one copy row per destination) and then posts each pending copy through the lockdown
// pacer (lockdownPostAlerts). The locked group itself never gets one.

const (
	// lockdownAlertBatch is how many pending copies one cycle posts.
	lockdownAlertBatch = 20
	// lockdownGroupToken stands in for the locked group's name until the translated
	// text is assembled, so a group title is never run through the translator.
	lockdownGroupToken = "<<lockdown-group>>"
	// lockdownSampleToken stands in for the burst sample's account lines until the
	// translated text is assembled, so a name is never run through the translator.
	lockdownSampleToken = "<<lockdown-sample>>"
	// lockdownLinkToken stands in for an invite link until the translated text is
	// assembled, so a link is never run through the translator.
	lockdownLinkToken = "<<lockdown-link>>"
	// lockdownAlertSampleMax is how many accounts of the burst an alert lists.
	lockdownAlertSampleMax = 10
	// lockdownDetailSuperseded is the detail of a reminder copy that was never posted
	// because a newer reminder was due by the time its turn came.
	lockdownDetailSuperseded = "superseded"
	// lockdownDetailNoLongerLinked is the detail of a staff copy that was never posted
	// because the locked group is not linked to that Staff Group any more.
	lockdownDetailNoLongerLinked = "no longer linked"
)

// The lockdown| callback namespace: the buttons of an alert and of the confirm cards
// they open. Field a is the action, r the alert copy row's ID, and Confirm and Cancel
// also carry the asker u and, on Confirm, the expiry e in unix minutes.
const (
	lockdownCallbackNamespace = "lockdown"
	// lockdownActLiftAsk is the alert's Lift lockdown button.
	lockdownActLiftAsk = "lk"
	// lockdownActLiftConfirm is the confirm card's Confirm button.
	lockdownActLiftConfirm = "lc"
	// lockdownActCancel is the confirm card's Cancel button.
	lockdownActCancel = "x"
	// lockdownActBanAsk is the alert's Ban N recent joiners button.
	lockdownActBanAsk = "bn"
	// lockdownActBanConfirm is the Ban confirm card's Confirm button.
	lockdownActBanConfirm = "bc"
	// lockdownActRevokeAsk is the alert's Revoke link button.
	lockdownActRevokeAsk = "rk"
	// lockdownActRevokeConfirm is the Revoke confirm card's Confirm button.
	lockdownActRevokeConfirm = "rc"
)

// lockdownReminderEvery is how often a lockdown nobody lifted is announced again,
// counted from the moment its alert was decided (alert_started_at), not from the lock.
// A variable so a test can age an alert instead of sleeping.
var lockdownReminderEvery = time.Hour

// lockdownCardLifetime is how long a confirm card's Confirm button stays valid. It is
// written into the button as an expiry minute, so the card needs no stored state and
// works with Redis down. A variable so a test can shorten it.
var lockdownCardLifetime = 5 * time.Minute

// lockdownAlertKeyboard is the keyboard an alert copy is posted with: a row holding the
// Lift lockdown button, a second row holding the Ban recent joiners button and, while
// the copy's stored link is one the bot can revoke and none was revoked yet, a third row
// holding the Revoke link button. The Ban label carries the number of accounts a run started now would act on (approved users,
// the bot and people an admin let in are left out), or a generic label when that number
// is 0 or cannot be read. A button whose data does not encode is left out and logged,
// because a dead button is never sent; with no button left it returns nil.
func lockdownAlertKeyboard(b *gotgbot.Bot, tr *i18n.Translator, slot *models.ChatLockdownAlert, ld *models.ChatLockdown) *gotgbot.InlineKeyboardMarkup {
	copyID := strconv.FormatUint(uint64(slot.ID), 10)
	var rows [][]gotgbot.InlineKeyboardButton

	if data := encodeCallbackData(lockdownCallbackNamespace, map[string]string{
		"a": lockdownActLiftAsk,
		"r": copyID,
	}); data != "" {
		rows = append(rows, []gotgbot.InlineKeyboardButton{
			{Text: lockdownText(tr, "lockdown_alert_btn_lift"), CallbackData: data},
		})
	} else {
		log.Errorf("[Lockdown] the Lift button of alert copy %d (lockdown %d) did not encode and is left out", slot.ID, ld.ID)
	}

	if data := encodeCallbackData(lockdownCallbackNamespace, map[string]string{
		"a": lockdownActBanAsk,
		"r": copyID,
	}); data != "" {
		rows = append(rows, []gotgbot.InlineKeyboardButton{
			{Text: lockdownBanButtonLabel(b, tr, ld), CallbackData: data},
		})
	} else {
		log.Errorf("[Lockdown] the Ban button of alert copy %d (lockdown %d) did not encode and is left out", slot.ID, ld.ID)
	}

	// Revoke only for a link the bot could revoke (shown in full, so it made it) and only
	// until a revoke was recorded: after that no copy offers it again.
	if lockdownLinkRevocable(slot.Link) && ld.LinkRevokedAt == nil {
		if data := encodeCallbackData(lockdownCallbackNamespace, map[string]string{
			"a": lockdownActRevokeAsk,
			"r": copyID,
		}); data != "" {
			rows = append(rows, []gotgbot.InlineKeyboardButton{
				{Text: lockdownText(tr, "lockdown_alert_btn_revoke"), CallbackData: data},
			})
		} else {
			log.Errorf("[Lockdown] the Revoke button of alert copy %d (lockdown %d) did not encode and is left out", slot.ID, ld.ID)
		}
	}

	if len(rows) == 0 {
		return nil
	}
	return &gotgbot.InlineKeyboardMarkup{InlineKeyboard: rows}
}

// lockdownBanButtonLabel is the Ban button's text: the candidate count when it is above
// 0, the generic label when it is 0 or the approved users or the count cannot be read.
func lockdownBanButtonLabel(b *gotgbot.Bot, tr *i18n.Translator, ld *models.ChatLockdown) string {
	excluded, err := lockdownBanExcluded(b, ld.ChatID)
	if err != nil {
		log.Warnf("[Lockdown] the approved users of chat %d could not be read for the Ban button: %v", ld.ChatID, err)
		return lockdownText(tr, "lockdown_alert_btn_ban_any")
	}
	count, err := lockdown.CountBanCandidatesFresh(ld.ID, excluded)
	if err != nil || count <= 0 {
		return lockdownText(tr, "lockdown_alert_btn_ban_any")
	}
	return lockdownText(tr, "lockdown_alert_btn_ban", i18n.TranslationParams{
		"count": strconv.FormatInt(count, 10),
	})
}

// lockdownLogChat is the locked group as actionlog.Destination wants it, to resolve the
// group's log channel and the header its posts start with. A lockdown only exists in a
// supergroup, and the stored title may be empty for a lockdown from before titles were
// kept.
func lockdownLogChat(ld *models.ChatLockdown) *gotgbot.Chat {
	return &gotgbot.Chat{Id: ld.ChatID, Title: ld.ChatTitle, Type: "supergroup"}
}

// lockdownAlertDestinations lists the copies a lockdown's alert is posted as: one for
// the Staff Group the locked group is linked to, read uncached at decision time, and one
// for the group's log channel when its admin category is on (the same resolver as every
// log post, actionlog.Destination). A group with neither has no copy. The locked group
// itself is never a destination. A failed lookup of the link is returned, so the cycle
// retries and nothing is decided.
func lockdownAlertDestinations(ld *models.ChatLockdown) ([]models.ChatLockdownAlert, error) {
	var slots []models.ChatLockdownAlert
	link, err := staff.GetLinkOfGroupFresh(ld.ChatID)
	if err != nil {
		return nil, err
	}
	if link != nil {
		slots = append(slots, models.ChatLockdownAlert{
			LockdownID: ld.ID,
			ChatID:     ld.ChatID,
			Kind:       models.AlertKindAlert,
			Seq:        0,
			Dest:       models.AlertDestStaff,
			DestChatID: link.StaffChatID,
		})
	}
	if channelID, _, ok := actionlog.Destination(lockdownLogChat(ld), logchannels.CategoryAdmin); ok {
		slots = append(slots, models.ChatLockdownAlert{
			LockdownID: ld.ID,
			ChatID:     ld.ChatID,
			Kind:       models.AlertKindAlert,
			Seq:        0,
			Dest:       models.AlertDestLog,
			DestChatID: channelID,
		})
	}
	return slots, nil
}

// lockdownBurstSnapshot reads the recent-joins tracker for the ten minutes before the
// lock (locked_at minus lockdownRecentWindow, up to locked_at) and returns its joins as
// pre-lockdown burst rows, to be saved with the alert in the same transaction, so
// nothing later needs Redis. Joins after locked_at are the lockdown's own joiner rows.
// The tracker is best effort: without Redis it returns nothing, and a failed read is
// logged at warn and treated as empty, because the alert must still post. The bot's
// own ID is skipped.
func lockdownBurstSnapshot(b *gotgbot.Bot, ld *models.ChatLockdown) []models.ChatLockdownBurst {
	if ld.LockedAt == nil {
		return nil
	}
	lockedAt := *ld.LockedAt
	joins, err := lockdownRecentLookup(ld.ChatID, lockedAt.Add(-lockdownRecentWindow), lockedAt)
	if err != nil {
		log.Warnf("[Lockdown] the recent joins of chat %d could not be read for the alert of lockdown %d: %v", ld.ChatID, ld.ID, err)
		return nil
	}
	rows := make([]models.ChatLockdownBurst, 0, len(joins))
	for _, join := range joins {
		if b != nil && join.UserID == b.Id {
			continue
		}
		joinedAt := join.At.UTC()
		rows = append(rows, models.ChatLockdownBurst{
			ChatID:     ld.ChatID,
			UserID:     join.UserID,
			FirstName:  join.Name,
			Username:   join.Username,
			IsBot:      join.Bot,
			InviteLink: join.Link,
			Source:     models.BurstSourcePre,
			JoinedAt:   &joinedAt,
		})
	}
	return rows
}

// lockdownPlanAlerts decides the alert of every confirmed active lockdown that has
// none yet. Whoever wins lockdown.BeginAlert for a lockdown creates its copies and
// saves the tracker's pre-lockdown joins as its burst rows; a lockdown with no
// destination is still stamped, so a Staff Group linked later never gets an alert for
// a lockdown that began before the link. It reports whether any lockdown was decided
// here.
func lockdownPlanAlerts(ctx context.Context, b *gotgbot.Bot) bool {
	rows, err := lockdown.ListAlertsToPlanFresh(lockdownWorkerBatch)
	if err != nil {
		log.Errorf("[Lockdown] alerts to plan could not be listed: %v", err)
		return false
	}
	progress := false
	for i := range rows {
		if ctx.Err() != nil {
			return progress
		}
		ld := &rows[i]
		slots, err := lockdownAlertDestinations(ld)
		if err != nil {
			log.Warnf("[Lockdown] alert destinations of lockdown %d could not be read: %v", ld.ID, err)
			continue
		}
		burst := lockdownBurstSnapshot(b, ld)
		won, err := lockdown.BeginAlert(ld.ID, lockdown.AlertPlan{Slots: slots, Burst: burst})
		if err != nil {
			log.Warnf("[Lockdown] alert of lockdown %d could not be decided: %v", ld.ID, err)
			continue
		}
		if won {
			progress = true
		}
	}
	return progress
}

// lockdownReminderSeq is the sequence number of the reminder due at now: how many whole
// lockdownReminderEvery periods passed since the lockdown's alert was decided. It is 0
// while the first period is not over, and for a lockdown whose alert was not decided.
// Only the current number is ever planned, so a long outage never owes a backlog.
func lockdownReminderSeq(ld *models.ChatLockdown, now time.Time) int {
	if ld.AlertStartedAt == nil || lockdownReminderEvery <= 0 {
		return 0
	}
	elapsed := now.Sub(*ld.AlertStartedAt)
	if elapsed < 0 {
		return 0
	}
	return int(elapsed / lockdownReminderEvery)
}

// lockdownPlanReminders plans the reminder that is due for every confirmed active
// lockdown whose alert was decided. The destinations are the alert's destination kinds,
// each resolved again now (lockdownAlertDestinations): a Staff Group the group was
// unlinked from, a log channel whose admin category is off and a kind the alert never
// went to get no reminder. The copies are inserted with ON CONFLICT DO NOTHING, so
// whichever replica plans first creates them and the others add nothing. It reports
// whether any copy was planned here.
func lockdownPlanReminders(ctx context.Context, _ *gotgbot.Bot) bool {
	rows, err := lockdown.ListRemindableFresh(lockdownWorkerBatch)
	if err != nil {
		log.Errorf("[Lockdown] lockdowns that may need a reminder could not be listed: %v", err)
		return false
	}
	now := time.Now()
	progress := false
	for i := range rows {
		if ctx.Err() != nil {
			return progress
		}
		ld := &rows[i]
		seq := lockdownReminderSeq(ld, now)
		if seq < 1 {
			continue
		}
		planned, err := lockdown.ReminderPlanned(ld.ID, seq)
		if err != nil {
			log.Warnf("[Lockdown] reminder %d of lockdown %d could not be looked up: %v", seq, ld.ID, err)
			continue
		}
		if planned {
			continue
		}
		kinds, err := lockdown.AlertDestKinds(ld.ID)
		if err != nil {
			log.Warnf("[Lockdown] the alert destinations of lockdown %d could not be read: %v", ld.ID, err)
			continue
		}
		if len(kinds) == 0 {
			continue
		}
		slots, err := lockdownAlertDestinations(ld)
		if err != nil {
			log.Warnf("[Lockdown] reminder destinations of lockdown %d could not be read: %v", ld.ID, err)
			continue
		}
		var plan []models.ChatLockdownAlert
		for _, slot := range slots {
			if !slices.Contains(kinds, slot.Dest) {
				continue
			}
			slot.Kind = models.AlertKindReminder
			slot.Seq = seq
			plan = append(plan, slot)
		}
		if len(plan) == 0 {
			continue
		}
		inserted, err := lockdown.PlanReminderSlots(ld.ID, seq, plan)
		if err != nil {
			log.Warnf("[Lockdown] reminder %d of lockdown %d could not be planned: %v", seq, ld.ID, err)
			continue
		}
		if inserted > 0 {
			progress = true
		}
	}
	return progress
}

// lockdownSlotTranslator is the translator of the chat a copy is posted in: the Staff
// Group for a staff copy, the locked group (whose log channel gets it) for a log copy.
func lockdownSlotTranslator(slot *models.ChatLockdownAlert) *i18n.Translator {
	if slot.Dest == models.AlertDestLog {
		return staffChatTranslator(slot.ChatID)
	}
	return staffChatTranslator(slot.DestChatID)
}

// lockdownGroupLabel names the locked group for an alert: its stored title, escaped and
// in bold, or the chat ID in code tags when the title was never stored (a lockdown from
// before titles were kept).
func lockdownGroupLabel(ld *models.ChatLockdown) string {
	if ld.ChatTitle == "" {
		return fmt.Sprintf("<code>%d</code>", ld.ChatID)
	}
	return "<b>" + html.EscapeString(ld.ChatTitle) + "</b>"
}

// lockdownRenderSampleAccount is one line of the burst sample: a bullet, the name
// (capped and escaped), the @username when there is one, the ID in code tags and a
// translated mark for a bot. Every user string is escaped here and never passes through
// the translator.
func lockdownRenderSampleAccount(tr *i18n.Translator, account lockdown.BurstAccount) string {
	var line strings.Builder
	line.WriteString("• ")
	line.WriteString(html.EscapeString(lockdownCapRunes(account.FirstName, lockdownNameMaxRunes)))
	if account.Username != "" {
		line.WriteString(" @")
		line.WriteString(html.EscapeString(lockdownCapRunes(account.Username, lockdownRecentUsernameMaxRunes)))
	}
	line.WriteString(" (<code>")
	line.WriteString(strconv.FormatInt(account.UserID, 10))
	line.WriteString("</code>)")
	if account.IsBot {
		line.WriteString(" · ")
		line.WriteString(lockdownText(tr, "lockdown_alert_bot_mark"))
	}
	return line.String()
}

// lockdownRenderSample is the burst part of an alert: the counts behind the trigger,
// then, when the burst is not empty, a title, one line per listed account and an "and N
// more" line for the accounts not listed. The account lines hold user text, so they are
// built outside the translator and spliced in after it, in place of a token.
func lockdownRenderSample(tr *i18n.Translator, accounts []lockdown.BurstAccount, counts lockdown.BurstCounts) string {
	lines := []string{lockdownText(tr, "lockdown_alert_counts", i18n.TranslationParams{
		"before": strconv.FormatInt(counts.Before, 10),
		"since":  strconv.FormatInt(counts.Since, 10),
	})}
	total := counts.Before + counts.Since
	if total == 0 {
		return lockdownJoinLines(lines)
	}
	lines = append(lines, lockdownText(tr, "lockdown_alert_sample_title"), lockdownSampleToken)
	if rest := total - int64(len(accounts)); rest > 0 {
		lines = append(lines, lockdownText(tr, "lockdown_alert_sample_more", i18n.TranslationParams{
			"count": strconv.FormatInt(rest, 10),
		}))
	}
	listed := make([]string, 0, len(accounts))
	for _, account := range accounts {
		listed = append(listed, lockdownRenderSampleAccount(tr, account))
	}
	return lockdownSplice(lockdownJoinLines(lines), lockdownSampleToken, strings.Join(listed, "\n"))
}

// lockdownLinkRevocable reports whether the bot could revoke an invite link it saw:
// the link is not empty and was not cut short. Telegram reports a link made by another
// admin cut short with "..." (or "…"), and a bot can revoke only a link in full.
func lockdownLinkRevocable(link string) bool {
	return link != "" && !strings.Contains(link, "...") && !strings.Contains(link, "…")
}

// lockdownRenderLinkLine is the invite-link part of an alert: the link most of the burst
// used with how many joined through it, a pointer to Telegram's own settings when that
// link was cut short and so cannot be revoked by the bot, and, when other links were used
// too, how many others. links is TopInviteLinksFresh's answer, most used first. It
// returns the text, empty when no link was reported, and the top link as the alert shows
// it. The link is HTML-escaped in code tags and spliced in after the translator.
func lockdownRenderLinkLine(tr *i18n.Translator, links []lockdown.InviteLinkCount) (text, top string) {
	if len(links) == 0 {
		return "", ""
	}
	top = links[0].Link
	lines := []string{lockdownSplice(
		lockdownText(tr, "lockdown_alert_link_line", i18n.TranslationParams{
			"link":  lockdownLinkToken,
			"count": strconv.FormatInt(links[0].Count, 10),
		}),
		lockdownLinkToken, "<code>"+html.EscapeString(top)+"</code>",
	)}
	if !lockdownLinkRevocable(top) {
		lines = append(lines, lockdownText(tr, "lockdown_alert_link_foreign"))
	}
	if others := len(links) - 1; others > 0 {
		lines = append(lines, lockdownText(tr, "lockdown_alert_link_others", i18n.TranslationParams{
			"count": strconv.Itoa(others),
		}))
	}
	return lockdownJoinLines(lines), top
}

// lockdownRenderAlert is the text of a lockdown's first alert: which group, how the
// lockdown was triggered (by whom and when), the reason, the burst behind it (the
// counts and a sample of its accounts) and the invite link it used, all read fresh from
// the database; a failed read leaves that part out and the alert still posts. The group,
// the name, the reason and the link are user-controlled, so they go in after
// translation, in place of their tokens. It also returns the link the alert shows, to
// be stored on the copy, empty when none is shown.
func lockdownRenderAlert(tr *i18n.Translator, ld *models.ChatLockdown) (text, link string) {
	lockedAt := ld.CreatedAt
	if ld.LockedAt != nil {
		lockedAt = *ld.LockedAt
	}
	title := lockdownSplice(
		lockdownText(tr, "lockdown_alert_title", i18n.TranslationParams{"group": lockdownGroupToken}),
		lockdownGroupToken, lockdownGroupLabel(ld),
	)
	trigger := lockdownSplice(
		lockdownText(tr, "lockdown_alert_trigger_manual", i18n.TranslationParams{
			"name": lockdownNameToken,
			"time": lockdownTime(lockedAt),
		}),
		lockdownNameToken, lockdownMention(ld.StartedBy, ld.StartedByName),
	)
	lines := []string{title, trigger, lockdownReasonLine(tr, ld.Reason)}
	accounts, counts, err := lockdown.BurstSampleFresh(ld.ID, lockdownAlertSampleMax)
	if err != nil {
		log.Warnf("[Lockdown] the burst of lockdown %d could not be read for its alert: %v", ld.ID, err)
	} else {
		lines = append(lines, lockdownRenderSample(tr, accounts, counts))
	}
	links, err := lockdown.TopInviteLinksFresh(ld.ID)
	if err != nil {
		log.Warnf("[Lockdown] the invite links of lockdown %d could not be read for its alert: %v", ld.ID, err)
	} else {
		var linkLine string
		linkLine, link = lockdownRenderLinkLine(tr, links)
		lines = append(lines, linkLine)
	}
	return lockdownJoinLines(lines), link
}

// lockdownRenderReminder is the text of a lockdown's hourly reminder: which group is still
// locked and since when, the reason, how many joiners the lockdown removed so far (its own
// bans plus the response bans, the number /lockdownstatus gives) and how many are still
// pending, and the invite link line of the alert. It carries no burst sample. It also
// returns the invite link the text shows, to be stored on the copy, empty when none is
// shown. The group, the reason and the link are user-controlled, so they go in after
// translation, in place of their tokens. A failed read leaves that line out and the
// reminder still posts. The footers for what happened since (a revoked link) are not part
// of it: the post adds them to what it sends, as a refresh does.
func lockdownRenderReminder(tr *i18n.Translator, ld *models.ChatLockdown) (text, link string) {
	lockedAt := ld.CreatedAt
	if ld.LockedAt != nil {
		lockedAt = *ld.LockedAt
	}
	title := lockdownSplice(
		lockdownText(tr, "lockdown_reminder_title", i18n.TranslationParams{
			"group": lockdownGroupToken,
			"since": lockdownTime(lockedAt),
		}),
		lockdownGroupToken, lockdownGroupLabel(ld),
	)
	lines := []string{title, lockdownReasonLine(tr, ld.Reason)}
	tally, err := lockdown.TallyJoiners(ld.ID)
	if err != nil {
		log.Warnf("[Lockdown] the joiners of lockdown %d could not be counted for its reminder: %v", ld.ID, err)
	} else {
		lines = append(lines, lockdownText(tr, "lockdown_reminder_counts", i18n.TranslationParams{
			"removed": strconv.FormatInt(lockdownRemovedSoFar(ld.ID, tally), 10),
			"pending": strconv.FormatInt(tally[models.JoinerStatePending]+tally[models.JoinerStateActing], 10),
		}))
	}
	links, err := lockdown.TopInviteLinksFresh(ld.ID)
	if err != nil {
		log.Warnf("[Lockdown] the invite links of lockdown %d could not be read for its reminder: %v", ld.ID, err)
	} else {
		var linkLine string
		linkLine, link = lockdownRenderLinkLine(tr, links)
		lines = append(lines, linkLine)
	}
	return lockdownJoinLines(lines), link
}

// lockdownPostAlerts posts the pending copies of active lockdowns, oldest first. Each
// copy is claimed first, so one replica sends it exactly once; the message ID and the
// exact text sent are recorded so later edits start from what was posted. A rate limit
// or a shutdown puts the copy back without costing an attempt; any other failure
// costs one, and the last of lockdownMaxAttempts ends the copy as failed. It reports
// whether any copy was posted or ended.
func lockdownPostAlerts(ctx context.Context, b *gotgbot.Bot) bool {
	slots, err := lockdown.ListPendingAlertSlotsFresh(lockdownAlertBatch)
	if err != nil {
		log.Errorf("[Lockdown] pending alert copies could not be listed: %v", err)
		return false
	}
	progress := false
	for i := range slots {
		if ctx.Err() != nil {
			return progress
		}
		if lockdownPostAlertSlot(ctx, b, &slots[i]) {
			progress = true
		}
	}
	return progress
}

// lockdownPostAlertSlot claims and posts one copy. It reports whether the copy was
// posted or ended as failed.
func lockdownPostAlertSlot(ctx context.Context, b *gotgbot.Bot, slot *models.ChatLockdownAlert) bool {
	claimed, err := lockdown.ClaimAlertSlot(slot.ID)
	if err != nil || !claimed {
		return false
	}
	release := func(detail string, countAttempt bool) {
		if _, err := lockdown.ReleaseAlertSlot(slot.ID, detail, countAttempt); err != nil {
			log.Errorf("[Lockdown] alert copy %d could not be given back: %v", slot.ID, err)
		}
	}

	ld, err := lockdown.GetFresh(slot.LockdownID)
	if err != nil {
		release("", false)
		return false
	}
	if ld == nil || ld.State != models.LockdownStateActive {
		release("", false)
		return false
	}

	tr := lockdownSlotTranslator(slot)
	reminder := slot.Kind == models.AlertKindReminder
	var text, link string
	if reminder {
		// A reminder that is behind the current one (its send failed or waited out an
		// outage) is never posted late: only the current reminder is ever sent.
		if slot.Seq < lockdownReminderSeq(ld, time.Now()) {
			if _, err := lockdown.FailAlertSlot(slot.ID, lockdownDetailSuperseded); err != nil {
				log.Errorf("[Lockdown] reminder copy %d could not be ended: %v", slot.ID, err)
				return false
			}
			return true
		}
		text, link = lockdownRenderReminder(tr, ld)
	} else {
		text, link = lockdownRenderAlert(tr, ld)
	}
	if slot.Dest == models.AlertDestLog {
		// The log channel's owner chose which categories it receives, and may have
		// changed that since the alert was decided: nothing is sent to a channel that
		// would not get it now. The header is the one every log post starts with. The
		// lookup cannot tell a turned-off category from a failed read, so the copy is
		// tried again on the usual attempt budget before it is ended.
		channelID, header, ok := actionlog.Destination(lockdownLogChat(ld), logchannels.CategoryAdmin)
		if !ok || channelID != slot.DestChatID {
			const detail = "log channel no longer receives admin posts"
			if slot.Attempts+1 >= lockdownMaxAttempts {
				if _, err := lockdown.FailAlertSlot(slot.ID, detail); err != nil {
					log.Errorf("[Lockdown] alert copy %d could not be ended: %v", slot.ID, err)
					return false
				}
				return true
			}
			release(detail, true)
			return false
		}
		text = header + text
	}
	if slot.Dest == models.AlertDestStaff {
		// The Staff Group was chosen when the copy was planned. A group unlinked since, or
		// linked to another Staff Group, must not have its alert land in a former one, so
		// the link is read fresh once more right before the send.
		link, linkErr := staff.GetLinkOfGroupFresh(ld.ChatID)
		if linkErr != nil {
			// Nothing is known about the link, so nothing is sent; the copy waits.
			log.Warnf("[Lockdown] the Staff Group link of chat %d could not be read for alert copy %d: %v", ld.ChatID, slot.ID, linkErr)
			release("", false)
			return false
		}
		if link == nil || link.StaffChatID != slot.DestChatID {
			if _, err := lockdown.FailAlertSlot(slot.ID, lockdownDetailNoLongerLinked); err != nil {
				log.Errorf("[Lockdown] alert copy %d could not be ended: %v", slot.ID, err)
				return false
			}
			return true
		}
	}
	opts := &gotgbot.SendMessageOpts{
		ParseMode:          formatting.HTML,
		LinkPreviewOptions: &gotgbot.LinkPreviewOptions{IsDisabled: true},
	}
	// The copy is stored with the link its text shows, and its Revoke button revokes
	// exactly that link; the keyboard is built from the same value.
	slot.Link = link
	if keyboard := lockdownAlertKeyboard(b, tr, slot, ld); keyboard != nil {
		opts.ReplyMarkup = *keyboard
	}
	// A copy posted after a revoke (an alert as well as a reminder) shows the revoked
	// footer at once, because the revoke marked only the copies that were posted by then.
	// The stored body stays without it, because a later refresh appends the footers to the
	// stored body.
	sendText := lockdownCopyText(tr, &models.ChatLockdownAlert{Body: text}, ld, false)
	var sent *gotgbot.Message
	sendErr := lockdownPaced(ctx, func(callCtx context.Context) error {
		call, cancel := context.WithTimeout(callCtx, lockdownCallTimeout)
		defer cancel()
		var callErr error
		sent, callErr = b.SendMessageWithContext(call, slot.DestChatID, sendText, opts)
		return callErr
	})
	switch lockdownClassify(ctx, sendErr) {
	case lockdownCallDone:
		if _, err := lockdown.RecordAlertPosted(slot.ID, sent.MessageId, text, link); err != nil {
			log.Errorf("[Lockdown] alert copy %d was sent but not recorded: %v", slot.ID, err)
			return false
		}
		if reminder {
			// Only the newest reminder and the original alert keep their buttons.
			if err := lockdown.SupersedeReminders(slot.LockdownID, slot.Dest, slot.Seq); err != nil {
				log.Warnf("[Lockdown] the older reminders of lockdown %d keep their buttons: %v", slot.LockdownID, err)
			}
		}
		return true
	case lockdownCallRetryFree:
		release("", false)
		return false
	}

	log.Warnf("[Lockdown] alert copy %d to chat %d failed: %v", slot.ID, slot.DestChatID, sendErr)
	detail := telegramErrorDetail(sendErr)
	if slot.Attempts+1 >= lockdownMaxAttempts {
		if _, err := lockdown.FailAlertSlot(slot.ID, detail); err != nil {
			log.Errorf("[Lockdown] alert copy %d could not be ended: %v", slot.ID, err)
			return false
		}
		return true
	}
	release(detail, true)
	return false
}

// lockdownLiftedFooter is the line added under an alert's stored text once its lockdown
// is lifting or lifted: who lifted it and when, read from the lockdown row. The name is
// user-controlled, so it goes in after translation, escaped.
func lockdownLiftedFooter(tr *i18n.Translator, ld *models.ChatLockdown) string {
	when := ld.UpdatedAt
	if ld.LiftStartedAt != nil {
		when = *ld.LiftStartedAt
	}
	lifter := html.EscapeString(ld.LiftedByName)
	if ld.LiftedBy != nil {
		lifter = lockdownMention(*ld.LiftedBy, ld.LiftedByName)
	}
	text := lockdownText(tr, "lockdown_alert_lifted_footer", i18n.TranslationParams{
		"name": lockdownNameToken,
		"time": lockdownTime(when),
	})
	return lockdownSplice(text, lockdownNameToken, lifter)
}

// lockdownAlertFooters are the lines added under an alert's stored text for what
// happened to its lockdown while it was active: today, that an admin revoked the raid's
// invite link, with who and when. The name is user-controlled, so it goes in after
// translation, escaped. The lifted footer is separate (lockdownLiftedFooter) and always
// comes last.
func lockdownAlertFooters(tr *i18n.Translator, ld *models.ChatLockdown) []string {
	var footers []string
	if ld.LinkRevokedAt != nil {
		reviser := html.EscapeString(ld.LinkRevokedByName)
		if ld.LinkRevokedBy != nil {
			reviser = lockdownMention(*ld.LinkRevokedBy, ld.LinkRevokedByName)
		}
		text := lockdownText(tr, "lockdown_alert_link_revoked", i18n.TranslationParams{
			"name": lockdownNameToken,
			"time": lockdownTime(*ld.LinkRevokedAt),
		})
		footers = append(footers, lockdownSplice(text, lockdownNameToken, reviser))
	}
	return footers
}

// lockdownCopyText is the text of a posted copy as it is edited later: the exact text
// that was posted, then the footers for what happened since and, once its lockdown is
// lifting or lifted, the lifted footer.
func lockdownCopyText(tr *i18n.Translator, slot *models.ChatLockdownAlert, ld *models.ChatLockdown, lifted bool) string {
	parts := append([]string{slot.Body}, lockdownAlertFooters(tr, ld)...)
	if lifted {
		parts = append(parts, lockdownLiftedFooter(tr, ld))
	}
	return strings.Join(parts, "\n\n")
}

// lockdownSyncAlertCopies refreshes the posted copies of active lockdowns that were
// marked edit_pending (today by a recorded link revoke): each is edited to its stored
// text plus the footers, with the keyboard it would be posted with now, through the
// lockdown pacer. A copy is claimed first, so one replica edits it once. An edit that
// worked and a message that already shows that text clear the mark; a message that is
// gone closes the copy; a rate limit or a shutdown gives the claim back without costing
// an attempt and ends the step; any other failure costs an attempt and the third one
// gives the refresh up (logged at warn), so a copy never loops. It takes at most
// lockdownAlertBatch copies per cycle and reports whether any copy was refreshed or
// ended.
func lockdownSyncAlertCopies(ctx context.Context, b *gotgbot.Bot) bool {
	slots, err := lockdown.ListAlertSlotsToSyncFresh(lockdownAlertBatch)
	if err != nil {
		log.Errorf("[Lockdown] alert copies to refresh could not be listed: %v", err)
		return false
	}
	progress := false
	for i := range slots {
		if ctx.Err() != nil {
			return progress
		}
		claimed, err := lockdown.ClaimAlertSync(slots[i].ID)
		if err != nil || !claimed {
			continue
		}
		// The copy is read again once it is claimed: a reminder that took its buttons off
		// after the list was read must be edited without them.
		slot, err := lockdown.GetAlertSlotFresh(slots[i].ID)
		if err != nil || slot == nil {
			releaseAlertClose(slots[i].ID, false)
			continue
		}
		done, stop := lockdownSyncClaimed(ctx, b, slot)
		if done {
			progress = true
		}
		if stop {
			// The same pacer limits every copy, so the rest would only be refused too.
			break
		}
	}
	return progress
}

// lockdownSyncClaimed edits one claimed copy and records the result. done reports
// whether the copy was refreshed or ended; stop that the step should end for this cycle.
func lockdownSyncClaimed(ctx context.Context, b *gotgbot.Bot, slot *models.ChatLockdownAlert) (done, stop bool) {
	ld, err := lockdown.GetFresh(slot.LockdownID)
	if err != nil || ld == nil || ld.State != models.LockdownStateActive {
		// A lockdown that left the active state is closed, not refreshed; the claim goes
		// back and the close step takes the copy.
		releaseAlertClose(slot.ID, false)
		return false, false
	}
	// The mark is cleared only if the copy's buttons are still as this edit left them.
	recordSynced := func() bool {
		if _, err := lockdown.RecordAlertSynced(slot.ID, slot.ButtonsLive); err != nil {
			log.Errorf("[Lockdown] alert copy %d was refreshed but not recorded: %v", slot.ID, err)
			return false
		}
		return true
	}

	tr := lockdownSlotTranslator(slot)
	opts := &gotgbot.EditMessageTextOpts{
		Text:               lockdownCopyText(tr, slot, ld, false),
		ChatId:             slot.DestChatID,
		MessageId:          slot.MessageID,
		ParseMode:          formatting.HTML,
		LinkPreviewOptions: &gotgbot.LinkPreviewOptions{IsDisabled: true},
	}
	if slot.ButtonsLive {
		if keyboard := lockdownAlertKeyboard(b, tr, slot, ld); keyboard != nil {
			opts.ReplyMarkup = *keyboard
		}
	}
	editErr := lockdownPaced(ctx, func(callCtx context.Context) error {
		call, cancel := context.WithTimeout(callCtx, lockdownCallTimeout)
		defer cancel()
		_, _, err := b.EditMessageTextWithContext(call, opts)
		return err
	})

	switch {
	case lockdownClassify(ctx, editErr) == lockdownCallDone, isMessageNotModified(editErr):
		return recordSynced(), false
	case lockdownClassify(ctx, editErr) == lockdownCallRetryFree:
		releaseAlertClose(slot.ID, false)
		return false, true
	case lockdownEditGone(editErr):
		if _, err := lockdown.RecordAlertClosed(slot.ID, "gone"); err != nil {
			log.Errorf("[Lockdown] alert copy %d is gone but not recorded closed: %v", slot.ID, err)
			return false, false
		}
		return true, false
	}

	log.Warnf("[Lockdown] refreshing alert copy %d in chat %d failed: %v", slot.ID, slot.DestChatID, editErr)
	if slot.Attempts+1 >= lockdownMaxAttempts {
		log.Warnf("[Lockdown] alert copy %d keeps its old text: the refresh failed %d times", slot.ID, lockdownMaxAttempts)
		return recordSynced(), false
	}
	releaseAlertClose(slot.ID, true)
	return false, false
}

// lockdownEditGone reports whether err says the message to edit no longer exists or can
// no longer be edited: Telegram's "message to edit not found", "message can't be
// edited" and MESSAGE_ID_INVALID. Such a copy cannot be changed, so it counts as closed.
// The text is matched case-insensitively.
func lockdownEditGone(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	for _, gone := range []string{"message to edit not found", "message can't be edited", "message_id_invalid"} {
		if strings.Contains(text, gone) {
			return true
		}
	}
	return false
}

// lockdownCloseAlerts closes the posted copies of lockdowns that are no longer active:
// each is edited to its stored text plus the lifted footer, with no buttons. It runs
// whatever lifted the lockdown (/unlockdown or the alert's button), covers a restart,
// takes newest copies first and at most lockdownAlertBatch per cycle, so the 5 s wait
// of StopLockdownWorker holds. It reports whether any copy was closed.
func lockdownCloseAlerts(ctx context.Context, b *gotgbot.Bot) bool {
	slots, err := lockdown.ListAlertSlotsToCloseFresh(lockdownAlertBatch)
	if err != nil {
		log.Errorf("[Lockdown] alert copies to close could not be listed: %v", err)
		return false
	}
	progress := false
	for i := range slots {
		if ctx.Err() != nil {
			return progress
		}
		claimed, err := lockdown.ClaimAlertClose(slots[i].ID)
		if err != nil || !claimed {
			continue
		}
		// The copy is read again once it is claimed: the claim resets the attempts that
		// failed refreshes left, and the close must count from what the row holds now.
		slot, err := lockdown.GetAlertSlotFresh(slots[i].ID)
		if err != nil || slot == nil {
			releaseAlertClose(slots[i].ID, false)
			continue
		}
		ld, err := lockdown.GetFresh(slot.LockdownID)
		if err != nil || ld == nil || ld.State == models.LockdownStateActive {
			releaseAlertClose(slot.ID, false)
			continue
		}
		closed, stop := lockdownCloseClaimed(ctx, b, slot, ld)
		if closed {
			progress = true
		}
		if stop {
			// The same pacer limits every copy, so the rest would only be refused too.
			break
		}
	}
	return progress
}

// lockdownCloseSlotNow closes one copy at once, for a press on a button of a lockdown
// that is lifting or lifted: the same claim, edit and record as the worker's step. It
// returns false when the claim was lost, because the worker (or another press) owns the
// copy and will close it.
func lockdownCloseSlotNow(ctx context.Context, b *gotgbot.Bot, slot *models.ChatLockdownAlert, ld *models.ChatLockdown) bool {
	claimed, err := lockdown.ClaimAlertClose(slot.ID)
	if err != nil || !claimed {
		return false
	}
	// Read again after the claim, which resets the attempts failed refreshes left.
	fresh, err := lockdown.GetAlertSlotFresh(slot.ID)
	if err != nil || fresh == nil {
		releaseAlertClose(slot.ID, false)
		return true
	}
	lockdownCloseClaimed(ctx, b, fresh, ld)
	return true
}

// releaseAlertClose gives a claimed copy back, logging a failure.
func releaseAlertClose(id uint, countAttempt bool) {
	if _, err := lockdown.ReleaseAlertClose(id, countAttempt); err != nil {
		log.Errorf("[Lockdown] alert copy %d could not be given back after closing: %v", id, err)
	}
}

// lockdownCloseClaimed edits a claimed copy to its stored text plus the lifted footer,
// without buttons, through the lockdown pacer, and records the result. An edit that
// worked, a message that already shows that text and a message that is gone all close
// the copy. A rate limit or a shutdown gives the claim back without costing an attempt
// and reports stop; any other failure costs an attempt and the third one closes the
// copy with Telegram's reason. closed reports whether the copy was closed.
func lockdownCloseClaimed(ctx context.Context, b *gotgbot.Bot, slot *models.ChatLockdownAlert, ld *models.ChatLockdown) (closed, stop bool) {
	record := func(detail string) bool {
		if _, err := lockdown.RecordAlertClosed(slot.ID, detail); err != nil {
			log.Errorf("[Lockdown] alert copy %d was edited but not recorded closed: %v", slot.ID, err)
			return false
		}
		return true
	}

	tr := lockdownSlotTranslator(slot)
	text := lockdownCopyText(tr, slot, ld, true)
	err := lockdownPaced(ctx, func(callCtx context.Context) error {
		call, cancel := context.WithTimeout(callCtx, lockdownCallTimeout)
		defer cancel()
		_, _, editErr := b.EditMessageTextWithContext(call, &gotgbot.EditMessageTextOpts{
			Text:               text,
			ChatId:             slot.DestChatID,
			MessageId:          slot.MessageID,
			ParseMode:          formatting.HTML,
			LinkPreviewOptions: &gotgbot.LinkPreviewOptions{IsDisabled: true},
		})
		return editErr
	})

	switch {
	case lockdownClassify(ctx, err) == lockdownCallDone, isMessageNotModified(err):
		return record(""), false
	case lockdownClassify(ctx, err) == lockdownCallRetryFree:
		releaseAlertClose(slot.ID, false)
		return false, true
	case lockdownEditGone(err):
		return record("gone"), false
	}

	log.Warnf("[Lockdown] closing alert copy %d in chat %d failed: %v", slot.ID, slot.DestChatID, err)
	if slot.Attempts+1 >= lockdownMaxAttempts {
		return record(telegramErrorDetail(err)), false
	}
	releaseAlertClose(slot.ID, true)
	return false, false
}
