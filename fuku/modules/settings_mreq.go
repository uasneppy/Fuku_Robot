package modules

import (
	"context"
	"errors"

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
)

func init() {
	registerSettingsPage(settingsPage{
		Key:       settingsPageMreq,
		Order:     80,
		Icon:      "🛂",
		TitleKey:  "settings_page_mreq",
		MainState: settingsMreqMainState,
		Open:      settingsMreqOpen,
		Controls: map[string]settingsControl{
			settingsMreqControlJoin:   {Right: settingsRightRestrict, Validate: settingsEditSwitchValue, Set: settingsMreqSetJoin},
			settingsMreqControlDelete: {Right: settingsRightRestrict, Validate: settingsEditSwitchValue, Set: settingsMreqSetDelete},
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
	result := apply(ctx, p.group.Id, on)
	notice := ""
	if result.Outcome == memberReqSaveFailed {
		log.Errorf("[Settings] write of the %s switch of chat %d failed: %v", what, p.group.Id, result.Err)
		notice = settingsNoticeWarn(settingsText(p.tr, result.Key))
	}
	row := result.Row
	if row == nil {
		var err error
		if row, err = settingsMreqRead(p.group.Id); err != nil {
			log.Warnf("[Settings] the requirements page of chat %d could not be read after a press: %v", p.group.Id, err)
			p.cannotOpen()
			return nil
		}
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

// renderSettingsMreqPage draws the page. It is pure. A nil row draws everything off. The keyboard
// is the switch row and the navigation row; the rule grid joins with the rule screens.
func renderSettingsMreqPage(v settingsView, row *models.ChatRequirements, notice string) settingsScreen {
	if row == nil {
		row = &models.ChatRequirements{}
	}
	k := newSettingsKeyboard(v)
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
