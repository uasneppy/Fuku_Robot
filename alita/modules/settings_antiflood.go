package modules

import (
	"context"
	"errors"
	"fmt"
	"html"
	"strconv"

	"github.com/PaulSonOfLars/gotgbot/v2"
	log "github.com/sirupsen/logrus"

	"github.com/divkix/Alita_Robot/alita/db/antiflood"
	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/i18n"
)

const (
	// settingsPageAntiflood is the p value of the Antiflood page.
	settingsPageAntiflood = "f"
	// settingsFloodLimitKey, settingsFloodModeKey and settingsFloodDeleteKey are the t values
	// of the limit, action and delete controls.
	settingsFloodLimitKey  = "l"
	settingsFloodModeKey   = "m"
	settingsFloodDeleteKey = "d"
)

// The limit control is Off (0), then settingsFloodMinLimit to settingsFloodMaxLimit, the range
// applyFloodLimit enforces. Off sits just below the lowest limit, so the steps count from
// settingsFloodMinLimit-1 while the limit is Off.
const (
	settingsFloodMinLimit = 3
	settingsFloodMaxLimit = 100
	// settingsFloodModeMaxLen is the longest action word a press may carry (kick, mute, ban,
	// and the longest word a forged press is still answered with the command's own refusal).
	settingsFloodModeMaxLen = 10
)

// settingsFloodRead reads a chat's antiflood settings. It is the cached getter /flood and the
// setters read, with a real error, so a failed read is the cannot-open edit and never "off".
// It is a variable only so a test can make the read fail.
var settingsFloodRead = antiflood.GetAntifloodSettingsCachedContext

func init() {
	registerSettingsPage(settingsPage{
		Key:       settingsPageAntiflood,
		Order:     40,
		Icon:      "🌊",
		TitleKey:  "settings_page_antiflood",
		MainState: settingsFloodMainState,
		Open:      settingsFloodOpen,
		Controls: map[string]settingsControl{
			settingsFloodLimitKey:  {Right: settingsRightAdminFloodBot, Set: settingsFloodSetLimit},
			settingsFloodModeKey:   {Right: settingsRightAdminFloodBot, Set: settingsFloodSetMode},
			settingsFloodDeleteKey: {Right: settingsRightAdminFloodBot, Set: settingsFloodSetDelete},
		},
	})
}

// settingsFloodMainState is the Antiflood button's state: "Limit {n}", or Off.
func settingsFloodMainState(_ *gotgbot.Bot, tr *i18n.Translator, group *gotgbot.Chat) (string, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), settingsReadTimeout)
	defer cancel()
	settings, err := settingsFloodRead(ctx, group.Id)
	if err != nil || settings == nil {
		return "", false
	}
	if settings.Limit == 0 {
		return settingsText(tr, "settings_state_off_plain"), true
	}
	return settingsText(tr, "settings_state_limit", i18n.TranslationParams{"n": settings.Limit}), true
}

// settingsFloodOpen reads the stored settings and draws the page. A read error is returned,
// which the dispatcher turns into the cannot-open edit.
func settingsFloodOpen(p *settingsPress, notice string) (settingsScreen, error) {
	ctx, cancel := context.WithTimeout(context.Background(), settingsReadTimeout)
	defer cancel()
	settings, err := settingsFloodRead(ctx, p.group.Id)
	if err != nil {
		return settingsScreen{}, err
	}
	if settings == nil {
		return settingsScreen{}, errors.New("antiflood settings: no row and no default")
	}
	return renderSettingsAntiflood(p.view(), settings, notice), nil
}

// settingsFloodActionWord is the word of a stored action. The literal keys keep a typo from
// shipping an empty word; an action outside the database check is shown as stored, escaped.
func settingsFloodActionWord(tr *i18n.Translator, action string) string {
	switch action {
	case "mute":
		return settingsText(tr, "settings_opt_mute")
	case "ban":
		return settingsText(tr, "settings_opt_ban")
	case "kick":
		return settingsText(tr, "settings_opt_kick")
	case "warn":
		return settingsText(tr, "settings_opt_warn")
	case "tban":
		return settingsText(tr, "settings_opt_tban")
	case "tmute":
		return settingsText(tr, "settings_opt_tmute")
	}
	return html.EscapeString(action)
}

// settingsFloodSettable reports whether the menu can write the action, which is what
// /setfloodmode accepts. A stored warn, tban or tmute is shown read-only.
func settingsFloodSettable(action string) bool {
	switch action {
	case "mute", "ban", "kick":
		return true
	}
	return false
}

// renderSettingsAntiflood draws the Antiflood page: the limit (or the off line), the action
// with a note when it cannot be set from the menu, the delete switch, and the controls.
func renderSettingsAntiflood(v settingsView, s *models.AntifloodSettings, notice string) settingsScreen {
	var body []string
	if s.Limit == 0 {
		body = append(body, settingsText(v.Tr, "settings_antiflood_off"))
	} else {
		body = append(body, settingsText(v.Tr, "settings_antiflood_limit", i18n.TranslationParams{"n": s.Limit}))
	}
	body = append(body, settingsText(v.Tr, "settings_antiflood_action", i18n.TranslationParams{
		"action": settingsFloodActionWord(v.Tr, s.Action),
	}))
	if !settingsFloodSettable(s.Action) {
		body = append(body, settingsText(v.Tr, "settings_antiflood_action_note"))
	}
	state := settingsStateWord(v.Tr, s.DeleteAntifloodMessage)
	body = append(body, settingsText(v.Tr, "settings_antiflood_delete", i18n.TranslationParams{"state": state}))

	k := newSettingsKeyboard(v)
	k.row(settingsFloodLimitRow(k, s.Limit)...)

	selected := ""
	if settingsFloodSettable(s.Action) {
		selected = s.Action
	}
	k.row(settingsChoiceRow(k, settingsPageAntiflood, settingsFloodModeKey, []settingsOption{
		{Value: "mute", Label: settingsText(v.Tr, "settings_opt_mute")},
		{Value: "ban", Label: settingsText(v.Tr, "settings_opt_ban")},
		{Value: "kick", Label: settingsText(v.Tr, "settings_opt_kick")},
	}, selected)...)

	toggleLabel := settingsText(v.Tr, "settings_btn_antiflood_delete", i18n.TranslationParams{"state": state})
	k.row(settingsToggleButton(k, settingsPageAntiflood, settingsFloodDeleteKey, toggleLabel, s.DeleteAntifloodMessage)...)
	k.navRow(false)
	return k.screen(settingsCompose(v, "settings_page_antiflood", body, notice, false))
}

// settingsFloodStepTarget is the absolute value a step to n lands on: below the lowest limit
// is Off, above the highest is the highest.
func settingsFloodStepTarget(n int) int {
	switch {
	case n < settingsFloodMinLimit:
		return 0
	case n > settingsFloodMaxLimit:
		return settingsFloodMaxLimit
	}
	return n
}

// settingsFloodLimitRow builds the limit stepper [−10 | −1 | value | +1 | +10]. A step sends
// the absolute target, so a double tap is harmless. While the limit is Off (or below the
// lowest limit) the steps count from just below the lowest limit, so +1 gives 3 and +10 gives
// 12, and a step whose target equals the stored limit, which only happens at Off or at the
// highest limit, is labelled "·" and keeps valid data.
func settingsFloodLimitRow(k *settingsKeyboard, limit int) []gotgbot.InlineKeyboardButton {
	base := limit
	if limit < settingsFloodMinLimit {
		base = settingsFloodMinLimit - 1
	}
	valueLabel := strconv.Itoa(limit)
	if limit == 0 {
		valueLabel = settingsText(k.view.Tr, "settings_state_off_plain")
	}

	var row []gotgbot.InlineKeyboardButton
	placed := false
	place := func() {
		placed = true
		if button, ok := k.button(valueLabel, map[string]string{"a": settingsActOpen, "p": settingsPageAntiflood}); ok {
			row = append(row, button)
		}
	}
	for _, step := range []int{-10, -1, 1, 10} {
		if step > 0 && !placed {
			place()
		}
		target := settingsFloodStepTarget(base + step)
		label := settingsStepLabel(step)
		if target == limit {
			label = settingsBoundLabel
		}
		fields := map[string]string{
			"a": settingsActSet, "p": settingsPageAntiflood, "t": settingsFloodLimitKey, "v": strconv.Itoa(target),
		}
		if button, ok := k.button(label, fields); ok {
			row = append(row, button)
		}
	}
	if !placed {
		place()
	}
	return row
}

// settingsFloodNotice is the notice line of an apply outcome: none for a stored value, the
// command's own text for a refusal or a failed save. word is the action a press named.
func settingsFloodNotice(p *settingsPress, result floodApplyResult, word string) string {
	switch result.Outcome {
	case floodOutOfRange:
		return settingsNoticeWarn(settingsText(p.tr, "antiflood_errors_set_in_limit"))
	case floodUnknownMode:
		// The locale text is the command's printf template; the word is escaped because the
		// page is sent as HTML.
		return settingsNoticeWarn(fmt.Sprintf(settingsText(p.tr, "antiflood_setfloodmode_unknown_type"), html.EscapeString(word)))
	case floodSaveFailed:
		log.Errorf("[Settings] antiflood write for chat %d failed: %v", p.group.Id, result.Err)
		return settingsNoticeWarn(settingsText(p.tr, "common_settings_save_failed"))
	}
	return ""
}

// settingsFloodTurnedOn reports whether a write left antiflood on where it was off.
// On a chat with no row the repository's first insert stores the model default limit (D-27),
// so a tap meant for the action or the switch turns antiflood on at that default. The
// commands behave the same and the repository is unchanged; the menu says so instead of
// letting a browsing tap pass in silence (D-34). Only the action and the delete switch ask
// (sideEffect of settingsFloodRedraw): a limit press names the limit itself.
func settingsFloodTurnedOn(result floodApplyResult, before *models.AntifloodSettings) bool {
	return result.Outcome == floodApplied && before != nil && before.Limit == 0 &&
		result.Stored != nil && result.Stored.Limit > 0
}

// settingsFloodRedraw draws the page after an apply function ran. It shows the row the
// function read back from the database; when there is none it falls back to the row read
// before a refused write, and otherwise reads the page again. An unreadable page is the
// cannot-open edit.
func settingsFloodRedraw(
	ctx context.Context, p *settingsPress, result floodApplyResult, before *models.AntifloodSettings, word string,
	sideEffect bool,
) *settingsScreen {
	notice := settingsFloodNotice(p, result, word)
	if notice == "" && sideEffect && settingsFloodTurnedOn(result, before) {
		notice = settingsNoticeWarn(settingsText(p.tr, "settings_antiflood_now_on", i18n.TranslationParams{
			"n": result.Stored.Limit,
		}))
	}
	current := result.Stored
	wrote := result.Outcome == floodApplied || result.Outcome == floodDisabled
	if current == nil && !wrote {
		current = before
	}
	if current == nil {
		var err error
		current, err = settingsFloodRead(ctx, p.group.Id)
		if err != nil || current == nil {
			log.Warnf("[Settings] antiflood settings of chat %d could not be read after a press: %v", p.group.Id, err)
			p.cannotOpen()
			return nil
		}
	}
	screen := renderSettingsAntiflood(p.view(), current, notice)
	return &screen
}

// settingsFloodReadBefore reads the stored settings before a press, or answers the
// cannot-open edit and returns false.
func settingsFloodReadBefore(ctx context.Context, p *settingsPress) (*models.AntifloodSettings, bool) {
	stored, err := settingsFloodRead(ctx, p.group.Id)
	if err != nil || stored == nil {
		log.Warnf("[Settings] antiflood settings of chat %d could not be read before a press: %v", p.group.Id, err)
		p.cannotOpen()
		return nil, false
	}
	return stored, true
}

// settingsFloodSetLimit applies an absolute limit through applyFloodLimit, the function
// /setflood calls, and redraws from its read-back. A step at Off or at the highest limit
// answers a toast and writes nothing.
func settingsFloodSetLimit(p *settingsPress, value string) *settingsScreen {
	limit, ok := settingsParseNumber(value, 3)
	if !ok {
		p.answer(settingsText(p.tr, "common_callback_invalid_request"), true)
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), settingsReadTimeout)
	defer cancel()

	before, ok := settingsFloodReadBefore(ctx, p)
	if !ok {
		return nil
	}
	if limit == before.Limit {
		switch limit {
		case 0:
			off := settingsText(p.tr, "settings_state_off_plain")
			p.answer(settingsText(p.tr, "settings_min_value", i18n.TranslationParams{"min": off}), false)
			return nil
		case settingsFloodMaxLimit:
			p.answer(settingsText(p.tr, "settings_max_value", i18n.TranslationParams{"max": settingsFloodMaxLimit}), false)
			return nil
		}
	}
	return settingsFloodRedraw(ctx, p, applyFloodLimit(ctx, p.group.Id, limit), before, "", false)
}

// settingsFloodModeWord reports whether v is 1 to settingsFloodModeMaxLen lower-case ASCII
// letters. A word of that shape that is not ban, kick or mute is refused by applyFloodMode
// with the command's own text.
func settingsFloodModeWord(v string) bool {
	if v == "" || len(v) > settingsFloodModeMaxLen {
		return false
	}
	for i := 0; i < len(v); i++ {
		if v[i] < 'a' || v[i] > 'z' {
			return false
		}
	}
	return true
}

// settingsFloodSetMode applies an action through applyFloodMode, the function /setfloodmode
// calls, and redraws from its read-back.
func settingsFloodSetMode(p *settingsPress, value string) *settingsScreen {
	if !settingsFloodModeWord(value) {
		p.answer(settingsText(p.tr, "common_callback_invalid_request"), true)
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), settingsReadTimeout)
	defer cancel()

	before, ok := settingsFloodReadBefore(ctx, p)
	if !ok {
		return nil
	}
	return settingsFloodRedraw(ctx, p, applyFloodMode(ctx, p.group.Id, value), before, value, true)
}

// settingsFloodSetDelete applies the delete switch through applyFloodDelete, the function
// /delflood calls, and redraws from its read-back.
func settingsFloodSetDelete(p *settingsPress, value string) *settingsScreen {
	if value != "0" && value != "1" {
		p.answer(settingsText(p.tr, "common_callback_invalid_request"), true)
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), settingsReadTimeout)
	defer cancel()

	before, ok := settingsFloodReadBefore(ctx, p)
	if !ok {
		return nil
	}
	return settingsFloodRedraw(ctx, p, applyFloodDelete(ctx, p.group.Id, value == "1"), before, "", true)
}
