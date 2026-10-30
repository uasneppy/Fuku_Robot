package modules

import (
	"context"
	"strconv"

	"github.com/PaulSonOfLars/gotgbot/v2"
	log "github.com/sirupsen/logrus"

	"github.com/divkix/Alita_Robot/alita/db/lockdown"
	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/db/raiddetection"
	"github.com/divkix/Alita_Robot/alita/i18n"
)

const (
	// settingsPageLockdown is the p value of the Lockdown page.
	settingsPageLockdown = "l"
	// settingsTriggerJoins, settingsTriggerFlood and settingsTriggerAI are the t values of
	// the three trigger controls.
	settingsTriggerJoins = "j"
	settingsTriggerFlood = "f"
	settingsTriggerAI    = "a"
	// settingsLockdownReasonRunes is how much of a manual lockdown's reason the page shows.
	settingsLockdownReasonRunes = 200

	// settingsActLiftAsk is the first tap of Lift and settingsActLiftYes the second (e is the
	// card's expiry minute).
	settingsActLiftAsk = "ls"
	settingsActLiftYes = "ly"
)

func init() {
	registerSettingsPage(settingsPage{
		Key:       settingsPageLockdown,
		Order:     20,
		Icon:      "🔒",
		TitleKey:  "settings_page_lockdown",
		MainState: settingsLockdownMainState,
		Open:      settingsLockdownOpen,
		Actions: map[string]settingsPageAction{
			settingsActLiftAsk: {Right: settingsRightRestrict, Run: settingsLockdownLiftAsk},
			settingsActLiftYes: {Fields: []string{"e"}, Right: settingsRightRestrict, Run: settingsLockdownLiftYes},
		},
		Controls: map[string]settingsControl{
			settingsTriggerJoins: {Right: settingsRightRestrict, Set: settingsLockdownSetTrigger(autoTriggerJoins)},
			settingsTriggerFlood: {Right: settingsRightRestrict, Set: settingsLockdownSetTrigger(autoTriggerFlood)},
			settingsTriggerAI:    {Right: settingsRightRestrict, Set: settingsLockdownSetTrigger(autoTriggerAI)},
		},
	})
}

// settingsLockdownMainState is the Lockdown button's state: not active, active or
// lifting, from a fresh read. A failed read is not ok.
func settingsLockdownMainState(_ *gotgbot.Bot, tr *i18n.Translator, group *gotgbot.Chat) (string, bool) {
	row, err := lockdownCurrentLookup(group.Id)
	if err != nil {
		return "", false
	}
	switch {
	case row == nil:
		return settingsText(tr, "settings_state_lock_none"), true
	case row.State == models.LockdownStateLifting:
		return settingsText(tr, "settings_state_lock_lifting"), true
	}
	return settingsText(tr, "settings_state_lock_active"), true
}

// settingsLockdownSnapshot is everything the Lockdown page shows, read once.
type settingsLockdownSnapshot struct {
	// Basic is true for a chat that is not a supergroup; nothing else is read then.
	Basic bool
	// Unreadable is true when any of the reads failed: the page then guesses nothing.
	Unreadable bool
	// Row is the chat's current lockdown (active, else one being lifted), nil when none.
	Row *models.ChatLockdown
	// Tally counts the joiners of a lifting lockdown by state.
	Tally map[string]int64
	// Triggers is the chat's automatic trigger settings.
	Triggers *models.ChatRaidDetection
	// AIAvailable says whether an AI provider of the raid path is set up.
	AIAvailable bool
}

// settingsLockdownLoad reads the page's data for the press's group.
func settingsLockdownLoad(p *settingsPress) settingsLockdownSnapshot {
	return settingsLockdownLoadWith(p, nil)
}

// settingsLockdownLoadWith is settingsLockdownLoad with the trigger settings already in
// hand (the readback of a save); nil reads them.
func settingsLockdownLoadWith(p *settingsPress, triggers *models.ChatRaidDetection) settingsLockdownSnapshot {
	if p.group.Type != "supergroup" {
		return settingsLockdownSnapshot{Basic: true}
	}
	snap := settingsLockdownSnapshot{AIAvailable: raidAIConfigured()}

	row, err := lockdownCurrentLookup(p.group.Id)
	if err != nil {
		log.Warnf("[Settings] lockdown state of chat %d could not be read: %v", p.group.Id, err)
		snap.Unreadable = true
	}
	snap.Row = row
	if err == nil && row != nil && row.State == models.LockdownStateLifting {
		tally, tallyErr := lockdown.TallyJoiners(row.ID)
		if tallyErr != nil {
			log.Warnf("[Settings] lockdown joiners of chat %d could not be counted: %v", p.group.Id, tallyErr)
			snap.Unreadable = true
		}
		snap.Tally = tally
	}

	if triggers == nil {
		var readErr error
		triggers, readErr = autolockdownReadSettings(p.group.Id)
		if readErr != nil {
			log.Warnf("[Settings] automatic lockdown settings of chat %d could not be read: %v", p.group.Id, readErr)
			snap.Unreadable = true
		}
	}
	snap.Triggers = triggers
	if snap.Triggers == nil {
		snap.Unreadable = true
	}
	return snap
}

// settingsLockdownOpen draws the page. A read failure is the unreadable body, never an
// error, so the admin gets a Refresh button instead of the cannot-open edit.
func settingsLockdownOpen(p *settingsPress, notice string) (settingsScreen, error) {
	return renderSettingsLockdown(p.view(), settingsLockdownLoad(p), notice), nil
}

// settingsLockdownClamp keeps a stepper target inside the trigger threshold range.
func settingsLockdownClamp(n int) int {
	return min(max(n, raiddetection.MinThreshold), raiddetection.MaxThreshold)
}

// settingsLockdownStateLines is the page's state block, in /lockdownstatus's words.
func settingsLockdownStateLines(v settingsView, s settingsLockdownSnapshot) []string {
	switch {
	case s.Row == nil:
		return []string{settingsText(v.Tr, "settings_lockdown_none")}
	case s.Row.State == models.LockdownStateLifting:
		return []string{lockdownLiftingStatusText(v.Tr, s.Row, s.Tally)}
	}
	return lockdownStatusHead(v.Tr, s.Row, settingsLockdownReasonRunes)
}

// renderSettingsLockdown draws the Lockdown page: the current state, the three automatic
// triggers with a switch and a stepper each, and the navigation row. A basic group and an
// unreadable state each get a short body and no trigger controls.
func renderSettingsLockdown(v settingsView, s settingsLockdownSnapshot, notice string) settingsScreen {
	k := newSettingsKeyboard(v)

	if s.Basic {
		k.navRow(false)
		return k.screen(settingsCompose(v, "settings_page_lockdown",
			[]string{settingsText(v.Tr, "settings_lockdown_basic")}, notice, false))
	}
	if s.Unreadable || s.Triggers == nil {
		if button, ok := k.button(settingsText(v.Tr, "staff_panel_refresh_button"),
			map[string]string{"a": settingsActOpen, "p": settingsPageLockdown}); ok {
			k.row(button)
		}
		k.navRow(false)
		return k.screen(settingsCompose(v, "settings_page_lockdown",
			[]string{settingsText(v.Tr, "settings_lockdown_unreadable")}, notice, true))
	}

	body := settingsLockdownStateLines(v, s)
	body = append(body, "", settingsText(v.Tr, "settings_lockdown_triggers"))

	type triggerView struct {
		control, lineKey, buttonKey string
		enabled                     bool
		threshold                   int
	}
	triggers := []triggerView{
		{settingsTriggerJoins, "settings_trigger_joins", "settings_btn_trigger_joins", s.Triggers.JoinEnabled, s.Triggers.JoinThreshold},
		{settingsTriggerFlood, "settings_trigger_flood", "settings_btn_trigger_flood", s.Triggers.FloodEnabled, s.Triggers.FloodThreshold},
		{settingsTriggerAI, "settings_trigger_ai", "settings_btn_trigger_ai", s.Triggers.AIEnabled, s.Triggers.AIThreshold},
	}
	for _, t := range triggers {
		body = append(body, settingsText(v.Tr, t.lineKey, i18n.TranslationParams{
			"state":     settingsStateWord(v.Tr, t.enabled),
			"threshold": t.threshold,
		}))
	}
	if !s.AIAvailable {
		body = append(body, settingsText(v.Tr, "lockdown_auto_ai_unavailable"))
	}

	// The Lift row of an active lockdown goes first among the content rows, above the trigger
	// rows. A lockdown that is being lifted, or none, offers no Lift.
	if s.Row != nil && s.Row.State == models.LockdownStateActive {
		if button, ok := k.button(settingsText(v.Tr, "lockdown_alert_btn_lift"),
			map[string]string{"a": settingsActLiftAsk}); ok {
			k.row(button)
		}
	}
	for _, t := range triggers {
		label := settingsText(v.Tr, t.buttonKey, i18n.TranslationParams{"state": settingsStateWord(v.Tr, t.enabled)})
		k.row(settingsToggleButton(k, settingsPageLockdown, t.control, label, t.enabled)...)
		k.row(settingsStepperRow(k, settingsPageLockdown, t.control, t.threshold,
			[]int{-10, -1, 1, 10}, settingsLockdownClamp, strconv.Itoa(t.threshold))...)
	}
	k.navRow(false)
	return k.screen(settingsCompose(v, "settings_page_lockdown", body, notice, true))
}

// settingsLockdownRedraw draws the page from a fresh load with a notice.
func settingsLockdownRedraw(p *settingsPress, triggers *models.ChatRaidDetection, notice string) *settingsScreen {
	screen := renderSettingsLockdown(p.view(), settingsLockdownLoadWith(p, triggers), notice)
	return &screen
}

// settingsLockdownSetTrigger returns the Set function of one trigger control. A toggle
// (tg, v 0 or 1) and a step (st, v a threshold) both write through applyAutoLockdown, the
// function /autolockdown uses, so a threshold also turns its trigger on and a value outside
// 3..500 is refused with the command's own text. The press's right (settingsRightRestrict)
// was judged before this runs.
func settingsLockdownSetTrigger(trigger string) func(p *settingsPress, value string) *settingsScreen {
	return func(p *settingsPress, value string) *settingsScreen {
		change := autoLockdownChange{Trigger: trigger}
		if p.action == settingsActToggle {
			switch value {
			case "0":
				change.Enable = false
			case "1":
				change.Enable = true
			default:
				p.answer(settingsText(p.tr, "common_callback_invalid_request"), true)
				return nil
			}
		} else {
			target, ok := settingsParseNumber(value, 3)
			if !ok {
				p.answer(settingsText(p.tr, "common_callback_invalid_request"), true)
				return nil
			}
			if p.group.Type == "supergroup" && settingsLockdownAtBound(p, trigger, target) {
				return nil
			}
			change.SetThreshold = true
			change.Threshold = target
			change.Enable = true
		}

		result := applyAutoLockdown(p.group, change)
		if refusal := autolockdownApplyText(p.tr, result); refusal != "" {
			return settingsLockdownRedraw(p, nil, settingsNoticeWarn(refusal))
		}
		// Saved. The readback redraws the page; when it failed, the page reads again.
		return settingsLockdownRedraw(p, result.Settings, "")
	}
}

// settingsLockdownAtBound answers the bound toast and returns true when target is the
// stored threshold of trigger and sits at 3 or 500: a step at a bound writes nothing.
func settingsLockdownAtBound(p *settingsPress, trigger string, target int) bool {
	if target != raiddetection.MinThreshold && target != raiddetection.MaxThreshold {
		return false
	}
	stored, err := autolockdownReadSettings(p.group.Id)
	if err != nil || stored == nil {
		return false
	}
	var current int
	switch trigger {
	case autoTriggerJoins:
		current = stored.JoinThreshold
	case autoTriggerFlood:
		current = stored.FloodThreshold
	case autoTriggerAI:
		current = stored.AIThreshold
	}
	if current != target {
		return false
	}
	if target == raiddetection.MinThreshold {
		p.answer(settingsText(p.tr, "settings_min_value", i18n.TranslationParams{"min": raiddetection.MinThreshold}), false)
	} else {
		p.answer(settingsText(p.tr, "settings_max_value", i18n.TranslationParams{"max": raiddetection.MaxThreshold}), false)
	}
	return true
}

// renderSettingsLockdownLiftCard draws the Lift card: the alert's own confirm prompt naming
// the locked group (its stored title, escaped and spliced in after translation), Yes and No,
// and the navigation row.
func renderSettingsLockdownLiftCard(v settingsView, row *models.ChatLockdown) settingsScreen {
	prompt := lockdownGroupText(v.Tr, "lockdown_alert_lift_ask", row)
	k := newSettingsKeyboard(v)
	settingsCardRows(k, map[string]string{"a": settingsActLiftYes}, settingsPageLockdown)
	return k.screen(settingsCompose(v, "settings_page_lockdown", []string{prompt}, "", false))
}

// settingsLockdownLiftAsk handles ls, the first tap of Lift. The press's right
// (settingsRightRestrict, a live member record) was judged before this runs. The lockdown is
// read fresh: none, one being lifted or an unreadable state redraws the page with the
// command's own words, and only an active one gets the card.
func settingsLockdownLiftAsk(p *settingsPress) *settingsScreen {
	row, err := lockdownCurrentLookup(p.group.Id)
	switch {
	case err != nil:
		log.Warnf("[Settings] lockdown state of chat %d could not be read for a Lift press: %v", p.group.Id, err)
		return settingsLockdownLiftRefusal(p, lockdownLiftApply{Outcome: liftApplyStateFailed})
	case row == nil:
		return settingsLockdownLiftRefusal(p, lockdownLiftApply{Outcome: liftApplyNotActive})
	case row.State != models.LockdownStateActive:
		return settingsLockdownLiftRefusal(p, lockdownLiftApply{Outcome: liftApplyInProgress, Row: row})
	}
	screen := renderSettingsLockdownLiftCard(p.view(), row)
	return &screen
}

// settingsLockdownLiftRefusal redraws the page with the text of a lift result that is not a
// lift, as a warning.
func settingsLockdownLiftRefusal(p *settingsPress, result lockdownLiftApply) *settingsScreen {
	return settingsLockdownRedraw(p, nil, settingsNoticeWarn(lockdownLiftApplyText(p.tr, result, p.query.From.Id)))
}

// settingsLockdownLiftYes handles ly, the second tap. A Yes whose expiry minute has passed
// redraws the page with settings_card_expired and lifts nothing. Otherwise the press is
// answered first (the lift can outlast the callback, D-31) and the lift goes only through
// applyLockdownLift, the path /unlockdown uses, which reads the lockdown fresh and records
// the lift through the conditional BeginLift, so of two Yes presses one wins. A recorded lift
// is announced in the group like the alert's Lift (lockdownAnnounceLift); every outcome
// redraws the page with the command's words as a notice.
func settingsLockdownLiftYes(p *settingsPress) *settingsScreen {
	expired, ok := settingsCardExpired(p.fields)
	if !ok {
		p.answer(settingsText(p.tr, "common_callback_invalid_request"), true)
		return nil
	}
	if expired {
		return settingsLockdownRedraw(p, nil, settingsNoticeWarn(settingsText(p.tr, "settings_card_expired")))
	}
	p.answer("", false)

	result := applyLockdownLift(context.Background(), p.b, p.group.Id, &p.query.From)
	if result.Outcome != liftApplyDone {
		return settingsLockdownLiftRefusal(p, result)
	}
	lockdownAnnounceLift(p.b, result.Row, p.query.From.Id, result.ActorName, result.Lift)
	return settingsLockdownRedraw(p, nil, settingsNoticeOK(lockdownLiftApplyText(p.tr, result, p.query.From.Id)))
}
