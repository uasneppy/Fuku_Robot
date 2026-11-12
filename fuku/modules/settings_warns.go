package modules

import (
	"context"
	"errors"
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
	action := settingsText(tr, "settings_state_not_set")
	if settings.WarnMode != "" {
		action = settingsFloodActionWord(tr, settings.WarnMode)
	}
	return settingsText(tr, "settings_state_warns", i18n.TranslationParams{
		"n": settings.WarnLimit, "action": action,
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

// renderSettingsWarns draws the Warnings page: the limit line, the hint and the stepper.
func renderSettingsWarns(v settingsView, s *models.WarnSettings, notice string) settingsScreen {
	body := []string{
		settingsText(v.Tr, "settings_warns_limit", i18n.TranslationParams{"n": s.WarnLimit}),
		"",
		settingsText(v.Tr, "settings_warns_hint"),
	}
	k := newSettingsKeyboard(v)
	k.row(settingsStepperRow(k, settingsPageWarns, settingsWarnsLimitKey, s.WarnLimit,
		[]int{-10, -1, 1, 10}, settingsWarnsStepTarget, strconv.Itoa(s.WarnLimit))...)
	k.navRow(false)
	return k.screen(settingsCompose(v, "settings_page_warns", body, notice, false))
}

// settingsWarnsNotice is the notice line of an apply outcome: none for a stored value, the
// command's own text for a refusal or a failed save.
func settingsWarnsNotice(p *settingsPress, result warnSettingsResult) string {
	switch result.Outcome {
	case warnSettingsOutOfRange, warnSettingsUnknownMode:
		return settingsNoticeWarn(settingsText(p.tr, result.Key))
	case warnSettingsSaveFailed:
		log.Errorf("[Settings] warn settings write for chat %d failed: %v", p.group.Id, result.Err)
		return settingsNoticeWarn(settingsText(p.tr, result.Key))
	}
	return ""
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

	before, err := settingsWarnsRead(ctx, p.group.Id)
	if err != nil || before == nil {
		log.Warnf("[Settings] warn settings of chat %d could not be read before a press: %v", p.group.Id, err)
		p.cannotOpen()
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

	result := applyWarnLimit(ctx, p.group.Id, limit)
	current := result.Stored
	if current == nil && result.Outcome != warnSettingsApplied {
		current = before
	}
	if current == nil {
		current, err = settingsWarnsRead(ctx, p.group.Id)
		if err != nil || current == nil {
			log.Warnf("[Settings] warn settings of chat %d could not be read after a press: %v", p.group.Id, err)
			p.cannotOpen()
			return nil
		}
	}
	screen := renderSettingsWarns(p.view(), current, settingsWarnsNotice(p, result))
	return &screen
}
