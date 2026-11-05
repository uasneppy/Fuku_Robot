package modules

import (
	"context"
	"errors"
	"strconv"

	"github.com/PaulSonOfLars/gotgbot/v2"
	log "github.com/sirupsen/logrus"

	"github.com/uasneppy/fuku_robot/fuku/db/memberreq"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
)

// The Member requirements page shows what each rule does to a member who breaks it and holds the
// two switches that apply to every rule (Check on join, Delete messages). It reads through
// memberReqSettingsLookup, the cached loader the message watcher reads, makes no Telegram call and
// creates no row (D-06, D-16). It never reads or names the Staff Group.

const (
	// settingsPageMreq is the p value of the Member requirements page.
	settingsPageMreq = "m"
	// settingsMreqControlJoin is the t value of the Check on join switch.
	settingsMreqControlJoin = "j"
	// settingsMreqControlDelete is the t value of the Delete messages switch.
	settingsMreqControlDelete = "d"
	// settingsActMreqOpen is the action code of a rule cell: it opens that rule's screen (field t).
	settingsActMreqOpen = "mo"
	// settingsActMreqYes is the action code of the card's Yes (fields t, v, e): v is Kick, Ban or Mute.
	settingsActMreqYes = "my"
)

// settingsMreqFresh is the fresh database read of a group's requirement settings that the
// same-value check of a punishment press makes, never the cached loader (D-05). It is a variable
// only so a test can make it fail.
var settingsMreqFresh = func(ctx context.Context, chatID int64) (*models.ChatRequirements, error) {
	return memberreq.GetSettingsFresh(ctx, chatID)
}

func init() {
	controls := map[string]settingsControl{
		settingsMreqControlJoin:   {Right: settingsRightRestrict, Validate: settingsEditSwitchValue, Set: settingsMreqSetJoin},
		settingsMreqControlDelete: {Right: settingsRightRestrict, Validate: settingsEditSwitchValue, Set: settingsMreqSetDelete},
	}
	for _, rule := range memberReqRules {
		// A rule control takes st only: a tg naming it is refused in step 1 (UI-SPEC section 2).
		controls[rule.Code] = settingsControl{
			Right: settingsRightRestrict, Actions: []string{settingsActSet},
			Validate: settingsMreqActionValue, Set: settingsMreqRuleSet(rule),
		}
	}
	registerSettingsPage(settingsPage{
		Key:       settingsPageMreq,
		Order:     80,
		Icon:      "🛂",
		TitleKey:  "settings_page_mreq",
		MainState: settingsMreqMainState,
		Open:      settingsMreqOpen,
		Controls:  controls,
		Actions: map[string]settingsPageAction{
			settingsActMreqOpen: {Fields: []string{"t"}, Right: settingsRightAdmin, Validate: settingsMreqOpenFields, Run: settingsMreqOpenRule},
			settingsActMreqYes:  {Fields: []string{"t", "v", "e"}, Right: settingsRightRestrict, Validate: settingsMreqYesFields, Run: settingsMreqYes},
		},
	})
}

// memberReqActionIcon is the icon of a stored punishment: ❌ off, ❗ warn, 👢 kick, ⛔ ban, 🔇 mute.
// The icons are not localized; an out-of-range value reads as the unknown mark.
func memberReqActionIcon(action int) string {
	switch action {
	case models.RequirementActionOff:
		return "❌"
	case models.RequirementActionWarn:
		return "❗"
	case models.RequirementActionKick:
		return "👢"
	case models.RequirementActionBan:
		return "⛔"
	case models.RequirementActionMute:
		return "🔇"
	}
	return settingsUnknownState
}

// settingsMreqActionWord is the plain option word of a stored punishment, the same words the other
// settings pages use for their choices.
func settingsMreqActionWord(tr *i18n.Translator, action int) string {
	switch action {
	case models.RequirementActionOff:
		return settingsText(tr, "settings_state_off_plain")
	case models.RequirementActionWarn:
		return settingsText(tr, "settings_opt_warn")
	case models.RequirementActionKick:
		return settingsText(tr, "settings_opt_kick")
	case models.RequirementActionBan:
		return settingsText(tr, "settings_opt_ban")
	case models.RequirementActionMute:
		return settingsText(tr, "settings_opt_mute")
	}
	return settingsUnknownState
}

// settingsMreqRead reads the requirement settings of a group through the one seam. It never
// returns a nil row without an error, so a caller cannot draw a guessed default.
func settingsMreqRead(chatID int64) (*models.ChatRequirements, error) {
	ctx, cancel := context.WithTimeout(context.Background(), settingsReadTimeout)
	defer cancel()
	row, err := memberReqSettingsLookup(ctx, chatID)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, errors.New("member requirement settings read back empty")
	}
	return row, nil
}

// settingsMreqMainState is the Requirements line of the Main page: Off when every rule is off
// (whatever the switches say), else "{n} on" for the n rules that are not off. ok is false when
// the settings could not be read.
func settingsMreqMainState(_ *gotgbot.Bot, tr *i18n.Translator, group *gotgbot.Chat) (string, bool) {
	row, err := settingsMreqRead(group.Id)
	if err != nil {
		return "", false
	}
	on := 0
	for _, rule := range memberReqRules {
		if memberreq.ActionOf(row, rule.Store) != models.RequirementActionOff {
			on++
		}
	}
	if on == 0 {
		return settingsText(tr, "settings_state_off_plain"), true
	}
	return settingsText(tr, "settings_state_mreq_on", i18n.TranslationParams{"n": on}), true
}

// settingsMreqOpen reads the settings and draws the page. A read error is returned, which the
// dispatcher turns into the cannot-open edit.
func settingsMreqOpen(p *settingsPress, notice string) (settingsScreen, error) {
	row, err := settingsMreqRead(p.group.Id)
	if err != nil {
		return settingsScreen{}, err
	}
	return renderSettingsMreqPage(p.view(), row, notice), nil
}

// settingsMreqResult unpacks an apply result for a redraw: the row to draw (the read-back, or a
// reload through the cached loader when the read-back failed) and the notice a failed write adds.
// ok is false after the cannot-open edit: the reload failed too and the press is already answered.
func settingsMreqResult(p *settingsPress, what string, result memberReqApplyResult) (row *models.ChatRequirements, notice string, ok bool) {
	if result.Outcome == memberReqSaveFailed {
		log.Errorf("[Settings] write of the %s of chat %d failed: %v", what, p.group.Id, result.Err)
		notice = settingsNoticeWarn(settingsText(p.tr, result.Key))
	}
	row = result.Row
	if row == nil {
		var err error
		if row, err = settingsMreqRead(p.group.Id); err != nil {
			log.Warnf("[Settings] the requirements page of chat %d could not be read after a press: %v", p.group.Id, err)
			p.cannotOpen()
			return nil, "", false
		}
	}
	return row, notice, true
}

// settingsMreqSetSwitch serves a switch control: it calls the apply function with the absolute
// target (the control's Validate already refused anything but "0" and "1" in step 1) and redraws
// the page from the row the function read back, with no notice. A failed write redraws the stored
// state with the save-failed notice; a read-back that failed too reloads the page.
func settingsMreqSetSwitch(
	p *settingsPress, what string, on bool,
	apply func(context.Context, int64, bool) memberReqApplyResult,
) *settingsScreen {
	ctx, cancel := context.WithTimeout(context.Background(), settingsReadTimeout)
	defer cancel()
	row, notice, ok := settingsMreqResult(p, what+" switch", apply(ctx, p.group.Id, on))
	if !ok {
		return nil
	}
	screen := renderSettingsMreqPage(p.view(), row, notice)
	return &screen
}

// settingsMreqSetJoin serves tg and st on the Check on join switch.
func settingsMreqSetJoin(p *settingsPress, value string) *settingsScreen {
	return settingsMreqSetSwitch(p, "check on join", value == "1", applyMemberReqJoin)
}

// settingsMreqSetDelete serves tg and st on the Delete messages switch.
func settingsMreqSetDelete(p *settingsPress, value string) *settingsScreen {
	return settingsMreqSetSwitch(p, "delete messages", value == "1", applyMemberReqDelete)
}

// settingsMreqBody is the body of the page: the intro, the all-off line while every rule is off,
// one line per rule in rule order, the two switch lines and the hints.
func settingsMreqBody(v settingsView, row *models.ChatRequirements) []string {
	tr := v.Tr
	body := []string{settingsText(tr, "settings_mreq_intro")}
	if !memberreq.AnyRuleOn(row) {
		body = append(body, settingsText(tr, "settings_mreq_all_off"))
	}
	body = append(body, "")
	for _, rule := range memberReqRules {
		action := memberreq.ActionOf(row, rule.Store)
		body = append(body, settingsText(tr, "settings_mreq_line", i18n.TranslationParams{
			"rule":   settingsText(tr, rule.LineKey),
			"action": memberReqActionIcon(action) + " " + settingsMreqActionWord(tr, action),
		}))
	}
	body = append(body, "",
		settingsText(tr, "settings_mreq_line_join", i18n.TranslationParams{"state": settingsStateWord(tr, row.CheckOnJoin)}),
		settingsText(tr, "settings_mreq_line_delete", i18n.TranslationParams{"state": settingsStateWord(tr, row.DeleteMessages)}),
	)
	for _, hint := range []string{
		"settings_mreq_hint_letters", "settings_mreq_hint_blank", "settings_mreq_hint_join",
		"settings_mreq_hint_delete", "settings_mreq_hint_mute",
	} {
		body = append(body, "", settingsText(tr, hint))
	}
	return body
}

// settingsMreqRuleGrid adds the rule grid: one cell per rule in rule order, the rule's punishment
// icon and short name, two per row filled left to right with an odd last cell alone. A cell opens
// that rule's screen (UI-SPEC amendment M4).
func settingsMreqRuleGrid(k *settingsKeyboard, row *models.ChatRequirements) {
	var cells []gotgbot.InlineKeyboardButton
	for _, rule := range memberReqRules {
		label := settingsText(k.view.Tr, rule.CellKey, i18n.TranslationParams{
			"state": memberReqActionIcon(memberreq.ActionOf(row, rule.Store)),
		})
		if button, ok := k.button(label, map[string]string{"a": settingsActMreqOpen, "t": rule.Code}); ok {
			cells = append(cells, button)
		}
	}
	for i := 0; i < len(cells); i += 2 {
		k.row(cells[i:min(i+2, len(cells))]...)
	}
}

// renderSettingsMreqPage draws the page. It is pure. A nil row draws everything off. The keyboard
// is the rule grid, the switch row and the navigation row.
func renderSettingsMreqPage(v settingsView, row *models.ChatRequirements, notice string) settingsScreen {
	if row == nil {
		row = &models.ChatRequirements{}
	}
	k := newSettingsKeyboard(v)
	settingsMreqRuleGrid(k, row)
	var cells []gotgbot.InlineKeyboardButton
	for _, sw := range []struct {
		control, labelKey string
		on                bool
	}{
		{settingsMreqControlJoin, "settings_btn_mreq_switch_join", row.CheckOnJoin},
		{settingsMreqControlDelete, "settings_btn_mreq_switch_delete", row.DeleteMessages},
	} {
		label := settingsText(v.Tr, sw.labelKey, i18n.TranslationParams{"state": settingsStateIcon(sw.on)})
		cells = append(cells, settingsToggleButton(k, settingsPageMreq, sw.control, label, sw.on)...)
	}
	k.row(cells...)
	k.navRow(false)
	return k.screen(settingsCompose(v, "settings_page_mreq", settingsMreqBody(v, row), notice, false))
}

// settingsMreqActionValue is the Validate of a rule control: v is exactly one ASCII digit 0 to 4
// (a stored punishment number), so "5", "04" and "x" are refused in step 1.
func settingsMreqActionValue(value string) bool {
	return len(value) == 1 && value[0] >= '0' && value[0] <= '4'
}

// settingsMreqOpenFields is the Validate of the mo action: its one field t is a rule code of this
// page.
func settingsMreqOpenFields(fields map[string]string) bool {
	_, ok := memberReqRuleByCode(fields["t"])
	return ok
}

// renderSettingsMreqRuleScreen draws one rule's screen. It is pure: the rule's name and what breaks
// it, the blank last name hint on that rule only, the stored punishment, the two hints, the choice
// rows [Off | Warn] and [Kick | Ban | Mute] with the check mark on the stored value, and Back to the
// page (not Main, UI-SPEC amendment M5) with Close. A nil row draws the rule off.
func renderSettingsMreqRuleScreen(v settingsView, rule memberReqRule, row *models.ChatRequirements, notice string) settingsScreen {
	if row == nil {
		row = &models.ChatRequirements{}
	}
	tr := v.Tr
	action := memberreq.ActionOf(row, rule.Store)
	body := []string{settingsText(tr, rule.LineKey), settingsText(tr, rule.DescKey)}
	if rule.Store == memberreq.RuleLastName {
		body = append(body, settingsText(tr, "settings_mreq_hint_blank"))
	}
	body = append(body, "",
		settingsText(tr, "settings_mreq_now", i18n.TranslationParams{
			"action": memberReqActionIcon(action) + " " + settingsMreqActionWord(tr, action),
		}),
		"",
		settingsText(tr, "settings_mreq_choose_hint"),
		settingsText(tr, "settings_mreq_actions_hint"),
	)

	k := newSettingsKeyboard(v)
	selected := strconv.Itoa(action)
	for _, values := range [][]int{{models.RequirementActionOff, models.RequirementActionWarn},
		{models.RequirementActionKick, models.RequirementActionBan, models.RequirementActionMute}} {
		options := make([]settingsOption, 0, len(values))
		for _, value := range values {
			options = append(options, settingsOption{Value: strconv.Itoa(value), Label: settingsMreqActionWord(tr, value)})
		}
		k.row(settingsChoiceRow(k, settingsPageMreq, rule.Code, options, selected)...)
	}
	var nav []gotgbot.InlineKeyboardButton
	if back, ok := k.button(settingsText(tr, "settings_btn_back"), map[string]string{"a": settingsActOpen, "p": settingsPageMreq}); ok {
		nav = append(nav, back)
	}
	if closeButton, ok := k.button(settingsText(tr, "settings_btn_close"), map[string]string{"a": settingsActClose}); ok {
		nav = append(nav, closeButton)
	}
	k.row(nav...)
	return k.screen(settingsCompose(v, "settings_page_mreq", body, notice, false))
}

// settingsMreqOpenRule serves the mo action: it reads the settings and draws the rule's screen. A
// read error is the cannot-open edit. The rule code was checked by settingsMreqOpenFields.
func settingsMreqOpenRule(p *settingsPress) *settingsScreen {
	rule, ok := memberReqRuleByCode(p.fields["t"])
	if !ok {
		p.answer(settingsText(p.tr, "common_callback_invalid_request"), true)
		return nil
	}
	row, err := settingsMreqRead(p.group.Id)
	if err != nil {
		log.Warnf("[Settings] the rule screen of chat %d could not be read: %v", p.group.Id, err)
		p.cannotOpen()
		return nil
	}
	screen := renderSettingsMreqRuleScreen(p.view(), rule, row, "")
	return &screen
}

// settingsMreqRuleSet makes the Set of a rule control. The value is one digit 0..4 (the control's
// Validate checked it in step 1).
func settingsMreqRuleSet(rule memberReqRule) func(p *settingsPress, value string) *settingsScreen {
	return func(p *settingsPress, value string) *settingsScreen {
		return settingsMreqChoose(p, rule, int(value[0]-'0'))
	}
}

// settingsMreqRedrawRule draws a rule's screen from row with the given notice.
func settingsMreqRedrawRule(p *settingsPress, rule memberReqRule, row *models.ChatRequirements, notice string) *settingsScreen {
	screen := renderSettingsMreqRuleScreen(p.view(), rule, row, notice)
	return &screen
}

// settingsMreqChoose serves a punishment press. The stored value is read fresh from the database,
// never from the cache (a stale "same value" would skip a card the owner should see, D-05): a
// press of the stored value writes nothing, Off and Warn write at once, and a failed read is the
// save-failed notice with no write.
func settingsMreqChoose(p *settingsPress, rule memberReqRule, action int) *settingsScreen {
	ctx, cancel := context.WithTimeout(context.Background(), settingsReadTimeout)
	defer cancel()
	stored, err := settingsMreqFresh(ctx, p.group.Id)
	if err != nil {
		log.Errorf("[Settings] fresh read of the requirements of chat %d failed: %v", p.group.Id, err)
		row, readErr := settingsMreqRead(p.group.Id)
		if readErr != nil {
			p.cannotOpen()
			return nil
		}
		return settingsMreqRedrawRule(p, rule, row, settingsNoticeWarn(settingsText(p.tr, "common_settings_save_failed")))
	}
	if memberreq.ActionOf(stored, rule.Store) == action {
		return settingsMreqRedrawRule(p, rule, stored, "")
	}
	if action >= models.RequirementActionKick {
		// Kick, Ban and Mute write nothing before the card's Yes: they ask first (D-05).
		screen := renderSettingsMreqCard(p.view(), rule, action)
		return &screen
	}
	return settingsMreqApplyRule(p, rule, action)
}

// settingsMreqYesFields is the Validate of the my action: exactly t, v and e, t a rule code, v one
// of "2", "3", "4" (a Yes can never carry Off or Warn) and e the card's expiry minute.
func settingsMreqYesFields(fields map[string]string) bool {
	if !settingsMreqOpenFields(fields) {
		return false
	}
	if v := fields["v"]; len(v) != 1 || v[0] < '2' || v[0] > '4' {
		return false
	}
	_, ok := settingsCardNumber(fields["e"])
	return ok
}

// renderSettingsMreqCard draws the shared two-tap card for setting rule to a strict action (Kick, Ban
// or Mute): the question with the rule's name and the plain option word, a blank line and what
// happens (every member at their next message, members already in the group included), then
// [Yes | No] and [Back | Close]. It is pure and writes nothing; Yes carries only the strict value.
func renderSettingsMreqCard(v settingsView, rule memberReqRule, action int) settingsScreen {
	tr := v.Tr
	consequence := map[int]string{
		models.RequirementActionKick: "settings_mreq_confirm_kick",
		models.RequirementActionBan:  "settings_mreq_confirm_ban",
		models.RequirementActionMute: "settings_mreq_confirm_mute",
	}[action]
	body := []string{
		settingsText(tr, "settings_mreq_confirm", i18n.TranslationParams{
			"rule":   settingsText(tr, rule.LineKey),
			"action": settingsMreqActionWord(tr, action),
		}),
		"",
		settingsText(tr, consequence),
	}
	k := newSettingsKeyboard(v)
	settingsCardRows(k, map[string]string{"a": settingsActMreqYes, "t": rule.Code, "v": strconv.Itoa(action)}, settingsPageMreq)
	return k.screen(settingsCompose(v, "settings_page_mreq", body, "", false))
}

// settingsMreqYes serves the card's Yes. The press already ran the whole pipeline again, so a
// presser who lost the restrict right since the first tap never gets here. An expired Yes writes
// nothing and redraws the rule screen with the expiry note; a live one writes v through
// applyMemberReqAction and redraws the rule screen from the read-back.
func settingsMreqYes(p *settingsPress) *settingsScreen {
	rule, ok := memberReqRuleByCode(p.fields["t"])
	expired, validExpiry := settingsCardExpired(p.fields)
	if !ok || !validExpiry || !settingsMreqYesFields(p.fields) {
		p.answer(settingsText(p.tr, "common_callback_invalid_request"), true)
		return nil
	}
	if expired {
		row, err := settingsMreqRead(p.group.Id)
		if err != nil {
			log.Warnf("[Settings] the rule screen of chat %d could not be read after an expired card: %v", p.group.Id, err)
			p.cannotOpen()
			return nil
		}
		return settingsMreqRedrawRule(p, rule, row, settingsNoticeWarn(settingsText(p.tr, "settings_card_expired")))
	}
	return settingsMreqApplyRule(p, rule, int(p.fields["v"][0]-'0'))
}

// settingsMreqApplyRule writes the punishment through applyMemberReqAction and redraws the rule
// screen from the row read back, with the save-failed notice when the write failed.
func settingsMreqApplyRule(p *settingsPress, rule memberReqRule, action int) *settingsScreen {
	ctx, cancel := context.WithTimeout(context.Background(), settingsReadTimeout)
	defer cancel()
	row, notice, ok := settingsMreqResult(p, "punishment of rule "+rule.Code, applyMemberReqAction(ctx, p.group.Id, rule.Store, action))
	if !ok {
		return nil
	}
	return settingsMreqRedrawRule(p, rule, row, notice)
}
