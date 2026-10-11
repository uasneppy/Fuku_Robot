package modules

import (
	"context"
	"fmt"
	"html"
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
)

// lockdownCardLifetime is how long a confirm card's Confirm button stays valid. It is
// written into the button as an expiry minute, so the card needs no stored state and
// works with Redis down. A variable so a test can shorten it.
var lockdownCardLifetime = 5 * time.Minute

// lockdownAlertKeyboard is the keyboard an alert copy is posted with: one row holding
// the Lift lockdown button. A button whose data does not encode is left out and logged,
// because a dead button is never sent; with no button left it returns nil.
func lockdownAlertKeyboard(tr *i18n.Translator, slot *models.ChatLockdownAlert, ld *models.ChatLockdown) *gotgbot.InlineKeyboardMarkup {
	data := encodeCallbackData(lockdownCallbackNamespace, map[string]string{
		"a": lockdownActLiftAsk,
		"r": strconv.FormatUint(uint64(slot.ID), 10),
	})
	if data == "" {
		log.Errorf("[Lockdown] the Lift button of alert copy %d (lockdown %d) did not encode and is left out", slot.ID, ld.ID)
		return nil
	}
	return &gotgbot.InlineKeyboardMarkup{
		InlineKeyboard: [][]gotgbot.InlineKeyboardButton{{
			{Text: lockdownText(tr, "lockdown_alert_btn_lift"), CallbackData: data},
		}},
	}
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

// lockdownPlanAlerts decides the alert of every confirmed active lockdown that has
// none yet. Whoever wins lockdown.BeginAlert for a lockdown creates its copies; a
// lockdown with no destination is still stamped, so a Staff Group linked later never
// gets an alert for a lockdown that began before the link. It reports whether any
// lockdown was decided here.
func lockdownPlanAlerts(ctx context.Context, _ *gotgbot.Bot) bool {
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
		won, err := lockdown.BeginAlert(ld.ID, lockdown.AlertPlan{Slots: slots})
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

// lockdownRenderAlert is the text of a lockdown's first alert: which group, how the
// lockdown was triggered (by whom and when) and the reason. The group, the name and the
// reason are user-controlled, so they go in after translation, in place of their
// tokens.
func lockdownRenderAlert(tr *i18n.Translator, ld *models.ChatLockdown) string {
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
	return lockdownJoinLines([]string{title, trigger, lockdownReasonLine(tr, ld.Reason)})
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
	text := lockdownRenderAlert(tr, ld)
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
	opts := &gotgbot.SendMessageOpts{
		ParseMode:          formatting.HTML,
		LinkPreviewOptions: &gotgbot.LinkPreviewOptions{IsDisabled: true},
	}
	if keyboard := lockdownAlertKeyboard(tr, slot, ld); keyboard != nil {
		opts.ReplyMarkup = *keyboard
	}
	var sent *gotgbot.Message
	sendErr := lockdownPaced(ctx, func(callCtx context.Context) error {
		call, cancel := context.WithTimeout(callCtx, lockdownCallTimeout)
		defer cancel()
		var callErr error
		sent, callErr = b.SendMessageWithContext(call, slot.DestChatID, text, opts)
		return callErr
	})
	switch lockdownClassify(ctx, sendErr) {
	case lockdownCallDone:
		if _, err := lockdown.RecordAlertPosted(slot.ID, sent.MessageId, text, ""); err != nil {
			log.Errorf("[Lockdown] alert copy %d was sent but not recorded: %v", slot.ID, err)
			return false
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
		slot := &slots[i]
		claimed, err := lockdown.ClaimAlertClose(slot.ID)
		if err != nil || !claimed {
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
	lockdownCloseClaimed(ctx, b, slot, ld)
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
	text := slot.Body + "\n\n" + lockdownLiftedFooter(tr, ld)
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
