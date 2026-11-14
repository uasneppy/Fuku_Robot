package modules

import (
	"context"
	"errors"
	"fmt"
	"html"
	"slices"
	"strconv"

	"github.com/PaulSonOfLars/gotgbot/v2"
	log "github.com/sirupsen/logrus"

	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/db/warns"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
)

const (
	// settingsPageWarns is the p value of the Warnings page.
	settingsPageWarns = "n"
	// settingsWarnsLimitKey is the t value of the limit stepper.
	settingsWarnsLimitKey = "l"
	// settingsWarnsModeKey is the t value of the action choice.
	settingsWarnsModeKey = "m"
)

// The warn limit is settingsWarnsMinLimit to settingsWarnsMaxLimit, the range applyWarnLimit
// enforces (the command's own bounds).
const (
	settingsWarnsMinLimit = 1
	settingsWarnsMaxLimit = 100
	// settingsWarnsLimitDigits is the longest limit a press may carry; the apply function
	// refuses 0 and anything over the maximum.
	settingsWarnsLimitDigits = 3
)

// settingsWarnsRead reads a chat's warn settings through the key /setwarnlimit and the warn
// module read, with a real error and without creating a row, so a failed read is the
// cannot-open edit and never a guessed default. It is a variable only so a test can make the
// read fail.
var settingsWarnsRead = warns.LoadWarnSettingsContext

func init() {
	registerSettingsPage(settingsPage{
		Key:       settingsPageWarns,
		Order:     42,
		Icon:      "🚩",
		TitleKey:  "settings_page_warns",
		MainState: settingsWarnsMainState,
		Open:      settingsWarnsOpen,
		Controls: map[string]settingsControl{
			settingsWarnsLimitKey: {
				Right:   settingsRightAdminFloodBot,
				Set:     settingsWarnsSetLimit,
				Actions: []string{settingsActSet},
				Validate: func(v string) bool {
					_, ok := settingsParseNumber(v, settingsWarnsLimitDigits)
					return ok
				},
			},
			// The action word has the Antiflood shape: a forged long or upper-case word is
			// invalid before any I/O, and a lower-case word that is not ban, kick or mute
			// reaches applyWarnMode's own refusal.
			settingsWarnsModeKey: {
				Right:    settingsRightAdminFloodBot,
				Set:      settingsWarnsSetMode,
				Actions:  []string{settingsActSet},
				Validate: settingsFloodModeWord,
			},
		},
	})
}

// settingsWarnsMainState is the Warnings button's state: "Limit {n}, {action}".
func settingsWarnsMainState(_ *gotgbot.Bot, tr *i18n.Translator, group *gotgbot.Chat) (string, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), settingsReadTimeout)
	defer cancel()
	settings, err := settingsWarnsRead(ctx, group.Id)
	if err != nil || settings == nil {
		return "", false
	}
	return settingsText(tr, "settings_state_warns", i18n.TranslationParams{
		"n": settings.WarnLimit, "action": settingsWarnsActionWord(tr, settings.WarnMode),
	}), true
}

// settingsWarnsOpen reads the stored settings and draws the page. A read error is returned,
// which the dispatcher turns into the cannot-open edit.
func settingsWarnsOpen(p *settingsPress, notice string) (settingsScreen, error) {
	ctx, cancel := context.WithTimeout(context.Background(), settingsReadTimeout)
	defer cancel()
	settings, err := settingsWarnsRead(ctx, p.group.Id)
	if err != nil {
		return settingsScreen{}, err
	}
	if settings == nil {
		return settingsScreen{}, errors.New("warn settings: no row and no default")
	}
	return renderSettingsWarns(p.view(), settings, notice), nil
}

// settingsWarnsStepTarget is the absolute value a step to n lands on, clamped to the limit's
// range.
func settingsWarnsStepTarget(n int) int {
	return min(max(n, settingsWarnsMinLimit), settingsWarnsMaxLimit)
}

// settingsWarnsActionWord is the word of the stored action: the Antiflood words for a mode
// the check allows, the stored text (escaped) for any other, and Not set for an empty one.
func settingsWarnsActionWord(tr *i18n.Translator, mode string) string {
	if mode == "" {
		return settingsText(tr, "settings_state_not_set")
	}
	return settingsFloodActionWord(tr, mode)
}

// renderSettingsWarns draws the Warnings page: the limit line, the action line, the hint, the
// stepper and the choice of the action. Only a mode /setwarnmode accepts is marked; any other
// stored action gets the inert line.
func renderSettingsWarns(v settingsView, s *models.WarnSettings, notice string) settingsScreen {
	body := []string{
		settingsText(v.Tr, "settings_warns_limit", i18n.TranslationParams{"n": s.WarnLimit}),
		settingsText(v.Tr, "settings_warns_action", i18n.TranslationParams{
			"action": settingsWarnsActionWord(v.Tr, s.WarnMode),
		}),
	}
	// A stored tban, tmute or empty action applies nothing at the limit (the warn module acts
	// on ban, kick and mute only), so the page says so and marks no choice. A press of Mute,
	// Ban or Kick replaces it.
	selected := ""
	if slices.Contains(warnSettingsModes, s.WarnMode) {
		selected = s.WarnMode
	} else {
		body = append(body, settingsText(v.Tr, "settings_warns_action_inert"))
	}
	body = append(body, "", settingsText(v.Tr, "settings_warns_hint"))
	k := newSettingsKeyboard(v)
	k.row(settingsStepperRow(k, settingsPageWarns, settingsWarnsLimitKey, s.WarnLimit,
		[]int{-10, -1, 1, 10}, settingsWarnsStepTarget, strconv.Itoa(s.WarnLimit))...)
	k.row(settingsChoiceRow(k, settingsPageWarns, settingsWarnsModeKey, []settingsOption{
		{Value: "mute", Label: settingsText(v.Tr, "settings_opt_mute")},
		{Value: "ban", Label: settingsText(v.Tr, "settings_opt_ban")},
		{Value: "kick", Label: settingsText(v.Tr, "settings_opt_kick")},
	}, selected)...)
	k.navRow(false)
	return k.screen(settingsCompose(v, "settings_page_warns", body, notice, false))
}

// settingsWarnsNotice is the notice line of an apply outcome: none for a stored value, the
// command's own text for a refusal or a failed save. word is the action a press named; the
// locale text of an unknown action is the command's printf template, so the word is escaped
// because the page is sent as HTML.
func settingsWarnsNotice(p *settingsPress, result warnSettingsResult, word string) string {
	switch result.Outcome {
	case warnSettingsOutOfRange:
		return settingsNoticeWarn(settingsText(p.tr, result.Key))
	case warnSettingsUnknownMode:
		return settingsNoticeWarn(fmt.Sprintf(settingsText(p.tr, result.Key), html.EscapeString(word)))
	case warnSettingsSaveFailed:
		log.Errorf("[Settings] warn settings write for chat %d failed: %v", p.group.Id, result.Err)
		return settingsNoticeWarn(settingsText(p.tr, result.Key))
	}
	return ""
}

// settingsWarnsReadBefore reads the stored settings before a press, or answers the
// cannot-open edit and returns false.
func settingsWarnsReadBefore(ctx context.Context, p *settingsPress) (*models.WarnSettings, bool) {
	before, err := settingsWarnsRead(ctx, p.group.Id)
	if err != nil || before == nil {
		log.Warnf("[Settings] warn settings of chat %d could not be read before a press: %v", p.group.Id, err)
		p.cannotOpen()
		return nil, false
	}
	return before, true
}

// settingsWarnsRedraw draws the page after an apply function ran: the row it read back, else
// the row read before a refused write, else a fresh read. An unreadable page is the
// cannot-open edit.
func settingsWarnsRedraw(
	ctx context.Context, p *settingsPress, result warnSettingsResult, before *models.WarnSettings, word string,
) *settingsScreen {
	current := result.Stored
	if current == nil && result.Outcome != warnSettingsApplied {
		current = before
	}
	if current == nil {
		var err error
		current, err = settingsWarnsRead(ctx, p.group.Id)
		if err != nil || current == nil {
			log.Warnf("[Settings] warn settings of chat %d could not be read after a press: %v", p.group.Id, err)
			p.cannotOpen()
			return nil
		}
	}
	screen := renderSettingsWarns(p.view(), current, settingsWarnsNotice(p, result, word))
	return &screen
}

// settingsWarnsSetLimit applies an absolute limit through applyWarnLimit, the function
// /setwarnlimit calls, and redraws from its read-back. A step at the lowest or highest limit
// answers a toast and writes nothing.
func settingsWarnsSetLimit(p *settingsPress, value string) *settingsScreen {
	limit, ok := settingsParseNumber(value, settingsWarnsLimitDigits)
	if !ok {
		p.answer(settingsText(p.tr, "common_callback_invalid_request"), true)
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), settingsReadTimeout)
	defer cancel()

	before, ok := settingsWarnsReadBefore(ctx, p)
	if !ok {
		return nil
	}
	if limit == before.WarnLimit {
		switch limit {
		case settingsWarnsMinLimit:
			p.answer(settingsText(p.tr, "settings_min_value", i18n.TranslationParams{"min": settingsWarnsMinLimit}), false)
			return nil
		case settingsWarnsMaxLimit:
			p.answer(settingsText(p.tr, "settings_max_value", i18n.TranslationParams{"max": settingsWarnsMaxLimit}), false)
			return nil
		}
	}
	return settingsWarnsRedraw(ctx, p, applyWarnLimit(ctx, p.group.Id, limit), before, "")
}

// settingsWarnsSetMode applies an action through applyWarnMode, the function /setwarnmode
// calls, and redraws from its read-back. The word is lower-case ASCII by Validate; one that is
// not ban, kick or mute is refused by the apply function with the command's own text.
func settingsWarnsSetMode(p *settingsPress, value string) *settingsScreen {
	if !settingsFloodModeWord(value) {
		p.answer(settingsText(p.tr, "common_callback_invalid_request"), true)
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), settingsReadTimeout)
	defer cancel()

	before, ok := settingsWarnsReadBefore(ctx, p)
	if !ok {
		return nil
	}
	return settingsWarnsRedraw(ctx, p, applyWarnMode(ctx, p.group.Id, value), before, value)
}
