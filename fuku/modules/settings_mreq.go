package modules

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	log "github.com/sirupsen/logrus"

	"github.com/uasneppy/fuku_robot/fuku/db/memberreq"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
)

// The Member requirements page shows what each rule does to a member who breaks it and holds the
// two switches that apply to every rule (Check on join, Delete messages). It reads through
// memberReqSettingsLookup, the cached loader the message watcher reads, and creates no row (D-06,
// D-16). It never reads or names the Staff Group. Main makes no Telegram call; the page and the rule
// screens add one status read (settingsMreqReadStatus) that says in words when a rule cannot work
// here, costs at most two paced calls and is never an error (D-15, D-08).

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
	return settingsMreqDrawPage(p, row, notice), nil
}

// settingsMreqChannelState is what the status read found about the channel rule in this group. Its
// zero value is unread: nothing was read, so nothing is claimed.
type settingsMreqChannelState int

const (
	// settingsMreqChannelUnread means no read was made (or the screen is a plain Phase 13 draw).
	settingsMreqChannelUnread settingsMreqChannelState = iota
	// settingsMreqChannelOK means a linked channel whose bot is an administrator.
	settingsMreqChannelOK
	// settingsMreqChannelNotAdmin means a linked channel the bot is definitely no administrator of.
	settingsMreqChannelNotAdmin
	// settingsMreqChannelNoLink means no linked channel was found for the group.
	settingsMreqChannelNoLink
	// settingsMreqChannelUnknown means the read could not be answered: it draws the unknown mark.
	settingsMreqChannelUnknown
)

// settingsMreqStatus is what a draw knows about whether the rules can work in this group. Basic is
// read from the chat object the press already holds, Channel from the status read. The zero value
// is "nothing known", which draws the Phase 13 shape.
type settingsMreqStatus struct {
	Basic   bool
	Channel settingsMreqChannelState
}

// settingsMreqScope says which part of the status a draw needs.
type settingsMreqScope int

const (
	// settingsMreqScopeBasic needs only the basic-group flag (every rule screen but the channel's).
	settingsMreqScopeBasic settingsMreqScope = iota
	// settingsMreqScopePage reads the channel only while the channel rule is not Off.
	settingsMreqScopePage
	// settingsMreqScopeChannel reads the channel whatever the stored value is (its own screen).
	settingsMreqScopeChannel
)

// settingsMreqHereTimeout bounds the whole status read of one draw. A variable so a test can
// shorten it.
var settingsMreqHereTimeout = 3 * time.Second

// settingsMreqReadStatus reads the status a draw of the page or of a rule screen shows (D-15). A
// basic group is read from p.group.Type and returns at once: no call. The photo and name rules'
// screens and a page whose channel rule is Off need nothing more and make no call. Otherwise one
// context of settingsMreqHereTimeout covers memberReqLinkedChannel and, only for a linked channel,
// memberReqBotChannelStatus, both through the lookup pacer, so a draw costs at most two calls. fresh
// bypasses the caches on read and refreshes them (the channel screen's opening); a page or a redraw
// reads through them. Every answer that is not definite is unknown, never an error.
func settingsMreqReadStatus(p *settingsPress, row *models.ChatRequirements, scope settingsMreqScope, fresh bool) settingsMreqStatus {
	if p.group.Type == "group" {
		return settingsMreqStatus{Basic: true}
	}
	switch scope {
	case settingsMreqScopeBasic:
		return settingsMreqStatus{}
	case settingsMreqScopePage:
		if memberreq.ActionOf(row, memberreq.RuleChannel) == models.RequirementActionOff {
			return settingsMreqStatus{}
		}
	case settingsMreqScopeChannel:
	}
	ctx, cancel := context.WithTimeout(context.Background(), settingsMreqHereTimeout)
	defer cancel()
	mode := memberReqLookupMode{Fresh: fresh}
	channel, known := memberReqLinkedChannel(ctx, p.b, p.group.Id, mode)
	if !known {
		return settingsMreqStatus{Channel: settingsMreqChannelUnknown}
	}
	if channel == 0 {
		return settingsMreqStatus{Channel: settingsMreqChannelNoLink}
	}
	switch memberReqBotChannelStatus(ctx, p.b, channel, mode) {
	case memberReqBotAdmin:
		return settingsMreqStatus{Channel: settingsMreqChannelOK}
	case memberReqBotNotAdmin:
		return settingsMreqStatus{Channel: settingsMreqChannelNotAdmin}
	case memberReqBotUnknown:
	}
	return settingsMreqStatus{Channel: settingsMreqChannelUnknown}
}

// settingsMreqDrawPage is the one draw of the page: the caller has read the database, the press is
// answered empty now (the status read can take a moment and must not keep the spinner), then the
// status is read and the page drawn.
func settingsMreqDrawPage(p *settingsPress, row *models.ChatRequirements, notice string) settingsScreen {
	p.answer("", false)
	status := settingsMreqReadStatus(p, row, settingsMreqScopePage, false)
	return renderSettingsMreqPageStatus(p.view(), row, notice, status)
}

// settingsMreqDrawRule is the one draw of a rule's screen, in the same order as the page's: the
// press is answered empty, the status read follows (the channel screen's own read, fresh on its
// opening and from the caches on a redraw; every other rule only the basic-group flag) and the
// screen is drawn.
func settingsMreqDrawRule(p *settingsPress, rule memberReqRule, row *models.ChatRequirements, notice string, fresh bool) settingsScreen {
	p.answer("", false)
	scope := settingsMreqScopeBasic
	if rule.Store == memberreq.RuleChannel {
		scope = settingsMreqScopeChannel
	}
	status := settingsMreqReadStatus(p, row, scope, fresh)
	return renderSettingsMreqRuleScreenStatus(p.view(), rule, row, notice, status)
}

// settingsMreqChannelLine is the status line of the channel rule for a state, with its icon as a
// code-side prefix, or "" when the state needs no line (nothing read, or an administrator bot). It
// names no channel, group or member: {rule} is the translated rule name only.
func settingsMreqChannelLine(tr *i18n.Translator, state settingsMreqChannelState) string {
	var icon, key string
	switch state {
	case settingsMreqChannelNoLink:
		icon, key = "🚫 ", "settings_mreq_here_nolink"
	case settingsMreqChannelNotAdmin:
		icon, key = "🚫 ", "settings_mreq_here_notadmin"
	case settingsMreqChannelUnknown:
		icon, key = settingsUnknownState+" ", "settings_mreq_here_unknown"
	case settingsMreqChannelUnread, settingsMreqChannelOK:
		return ""
	}
	return icon + settingsText(tr, key, i18n.TranslationParams{"rule": settingsText(tr, "settings_mreq_rule_channel")})
}

// settingsMreqBasicLine is the one rule-agnostic line of a basic group.
func settingsMreqBasicLine(tr *i18n.Translator) string {
	return "🚫 " + settingsText(tr, "settings_mreq_here_basic")
}

// settingsMreqHereLines is the cannot-work block of the page: a list of lines, empty when nothing
// needs saying. The page lists a problem only for a rule that is not Off: the basic-group line while
// some rule is on, else in a supergroup the channel rule's line while that rule is on. A later rule
// appends its own line here with no renderer change.
func settingsMreqHereLines(tr *i18n.Translator, row *models.ChatRequirements, status settingsMreqStatus) []string {
	if status.Basic {
		if memberreq.AnyRuleOn(row) {
			return []string{settingsMreqBasicLine(tr)}
		}
		return nil
	}
	var lines []string
	if memberreq.ActionOf(row, memberreq.RuleChannel) != models.RequirementActionOff {
		if line := settingsMreqChannelLine(tr, status.Channel); line != "" {
			lines = append(lines, line)
		}
	}
	return lines
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
	screen := settingsMreqDrawPage(p, row, notice)
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
// one line per rule in rule order, the two switch lines, the cannot-work block (omitted with its
// blank line when it has no line) and the hints.
func settingsMreqBody(v settingsView, row *models.ChatRequirements, status settingsMreqStatus) []string {
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
	if lines := settingsMreqHereLines(tr, row, status); len(lines) > 0 {
		body = append(body, "")
		body = append(body, lines...)
	}
	for _, hint := range []string{
		"settings_mreq_hint_letters", "settings_mreq_hint_blank", "settings_mreq_hint_lookup",
		"settings_mreq_hint_join", "settings_mreq_hint_delete", "settings_mreq_hint_mute",
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

// renderSettingsMreqPage draws the page with no status read (the zero status): the shape the page
// had before any lookup. It is pure.
func renderSettingsMreqPage(v settingsView, row *models.ChatRequirements, notice string) settingsScreen {
	return renderSettingsMreqPageStatus(v, row, notice, settingsMreqStatus{})
}

// renderSettingsMreqPageStatus draws the page for a status. It is pure. A nil row draws everything
// off. The keyboard is the rule grid, the switch row and the navigation row: the status is body
// lines only, never a row.
func renderSettingsMreqPageStatus(v settingsView, row *models.ChatRequirements, notice string, status settingsMreqStatus) settingsScreen {
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
	return k.screen(settingsCompose(v, "settings_page_mreq", settingsMreqBody(v, row, status), notice, false))
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

// renderSettingsMreqRuleScreen draws one rule's screen with no status read (the zero status): the
// shape the screen had before any lookup. It is pure.
func renderSettingsMreqRuleScreen(v settingsView, rule memberReqRule, row *models.ChatRequirements, notice string) settingsScreen {
	return renderSettingsMreqRuleScreenStatus(v, rule, row, notice, settingsMreqStatus{})
}

// settingsMreqRuleSlot is the one status line of a rule's screen: the basic-group line on every
// rule, the channel rule's own line on the channel screen, and nothing for the others. A rule
// screen always shows the line for its rule, because the screen exists to explain.
func settingsMreqRuleSlot(tr *i18n.Translator, rule memberReqRule, status settingsMreqStatus) string {
	if status.Basic {
		return settingsMreqBasicLine(tr)
	}
	if rule.Store == memberreq.RuleChannel {
		return settingsMreqChannelLine(tr, status.Channel)
	}
	return ""
}

// settingsMreqOffOnly reports whether the rule's screen offers Off alone: the channel rule in a group
// that cannot be linked to a channel (no linked channel found, or a basic group). A stored value
// stays visible and inert there, and Off clears it (D-05, UI-SPEC amendment M7).
func settingsMreqOffOnly(rule memberReqRule, status settingsMreqStatus) bool {
	return rule.Store == memberreq.RuleChannel && (status.Basic || status.Channel == settingsMreqChannelNoLink)
}

// renderSettingsMreqRuleScreenStatus draws one rule's screen for a status. It is pure: the rule's
// name and what breaks it, the rule's own hint (its HintKey: the blank last name hint, the photo
// rule's Warn or Mute recommendation) right after the description, the status slot (one line, none
// when nothing needs saying), the stored punishment, the two hints, the choice rows [Off | Warn] and
// [Kick | Ban | Mute] with the check mark on the stored value, and Back to the page (not Main,
// UI-SPEC amendment M5) with Close. While the rule cannot be linked (settingsMreqOffOnly) the choice
// is [Off] alone and the two hints, which describe options it does not offer, are omitted. A nil row
// draws the rule off.
func renderSettingsMreqRuleScreenStatus(v settingsView, rule memberReqRule, row *models.ChatRequirements, notice string, status settingsMreqStatus) settingsScreen {
	if row == nil {
		row = &models.ChatRequirements{}
	}
	tr := v.Tr
	action := memberreq.ActionOf(row, rule.Store)
	offOnly := settingsMreqOffOnly(rule, status)
	body := []string{settingsText(tr, rule.LineKey), settingsText(tr, rule.DescKey)}
	if rule.HintKey != "" {
		body = append(body, settingsText(tr, rule.HintKey))
	}
	if slot := settingsMreqRuleSlot(tr, rule, status); slot != "" {
		body = append(body, slot)
	}
	body = append(body, "",
		settingsText(tr, "settings_mreq_now", i18n.TranslationParams{
			"action": memberReqActionIcon(action) + " " + settingsMreqActionWord(tr, action),
		}),
	)
	choices := [][]int{{models.RequirementActionOff, models.RequirementActionWarn},
		{models.RequirementActionKick, models.RequirementActionBan, models.RequirementActionMute}}
	if offOnly {
		choices = [][]int{{models.RequirementActionOff}}
	} else {
		body = append(body, "", settingsText(tr, "settings_mreq_choose_hint"), settingsText(tr, "settings_mreq_actions_hint"))
	}

	k := newSettingsKeyboard(v)
	selected := strconv.Itoa(action)
	for _, values := range choices {
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
	// The channel screen reads its status fresh on its opening and refreshes the link and
	// administrator caches; every other rule only needs the basic-group flag.
	screen := settingsMreqDrawRule(p, rule, row, "", true)
	return &screen
}

// settingsMreqRuleSet makes the Set of a rule control. The value is one digit 0..4 (the control's
// Validate checked it in step 1).
func settingsMreqRuleSet(rule memberReqRule) func(p *settingsPress, value string) *settingsScreen {
	return func(p *settingsPress, value string) *settingsScreen {
		return settingsMreqChoose(p, rule, int(value[0]-'0'))
	}
}

// settingsMreqRedrawRule draws a rule's screen from row with the given notice, reading its status
// from the caches.
func settingsMreqRedrawRule(p *settingsPress, rule memberReqRule, row *models.ChatRequirements, notice string) *settingsScreen {
	screen := settingsMreqDrawRule(p, rule, row, notice, false)
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
	if !settingsMreqChannelGate(p, rule, stored, action) {
		return nil
	}
	if action >= models.RequirementActionKick {
		// Kick, Ban and Mute write nothing before the card's Yes: they ask first (D-05).
		screen := renderSettingsMreqCard(p.view(), rule, action)
		return &screen
	}
	return settingsMreqApplyRule(p, rule, action)
}

// settingsMreqChannelGate is the live link check of a punishing press of the channel rule (D-05,
// D-06, D-08). Every other rule, and Off, proceed without a look. Otherwise one fresh getChat of
// the group through the lookup pacer, bounded by settingsMreqHereTimeout, decides, and it runs before
// any answer because its alert is the press's one answer: a basic group or a group with no linked
// channel is refused with the status line's own text, writes and draws no card, and the screen is
// redrawn as the Off-only form from stored (read when nil) so a stale screen is repaired; a link that
// cannot be read answers could-not-check and edits nothing; a linked channel proceeds, whatever the
// bot's standing there (the check then fails open). The link comes only from this live read, never
// from callback data. It returns false when it refused, having answered and edited.
func settingsMreqChannelGate(p *settingsPress, rule memberReqRule, stored *models.ChatRequirements, action int) bool {
	if rule.Store != memberreq.RuleChannel || action == models.RequirementActionOff {
		return true
	}
	status := settingsMreqStatus{Channel: settingsMreqChannelNoLink}
	if p.group.Type == "group" {
		status = settingsMreqStatus{Basic: true}
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), settingsMreqHereTimeout)
		channel, known := memberReqLinkedChannel(ctx, p.b, p.group.Id, memberReqLookupMode{Fresh: true})
		cancel()
		if !known {
			p.answer(settingsText(p.tr, "mreq_fixed_check_failed"), true)
			return false
		}
		if channel != 0 {
			return true
		}
	}
	p.answer(settingsMreqNoLinkAlert(p.tr), true)
	if stored == nil {
		var err error
		if stored, err = settingsMreqRead(p.group.Id); err != nil {
			log.Warnf("[Settings] the channel screen of chat %d could not be read after a refused press: %v", p.group.Id, err)
			p.cannotOpen()
			return false
		}
	}
	p.edit(renderSettingsMreqRuleScreenStatus(p.view(), rule, stored, "", status))
	return false
}

// settingsMreqNoLinkAlert is the alert of a refused channel press: the no-link line with the
// channel rule's translated name. It names no channel, group, member or setting.
func settingsMreqNoLinkAlert(tr *i18n.Translator) string {
	return settingsText(tr, "settings_mreq_here_nolink", i18n.TranslationParams{
		"rule": settingsText(tr, "settings_mreq_rule_channel"),
	})
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
// nothing, makes no link check and redraws the rule screen with the expiry note; a live one passes
// the channel rule's live link check (settingsMreqChannelGate), then writes v through
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
	action := int(p.fields["v"][0] - '0')
	// The card binds nothing about the link: a Yes checks it again, after the right and the expiry.
	if !settingsMreqChannelGate(p, rule, nil, action) {
		return nil
	}
	return settingsMreqApplyRule(p, rule, action)
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
