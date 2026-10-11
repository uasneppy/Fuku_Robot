package modules

import (
	"context"
	"fmt"
	"html"
	"strconv"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	log "github.com/sirupsen/logrus"

	"github.com/divkix/Alita_Robot/alita/db/lockdown"
	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/db/staff"
	"github.com/divkix/Alita_Robot/alita/i18n"
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

// lockdownAlertDestinations lists the copies a lockdown's alert is posted as: one for
// the Staff Group the locked group is linked to, read uncached at decision time. A
// group with no link has no copy. The locked group itself is never a destination. A
// failed lookup is returned, so the cycle retries and nothing is decided.
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
