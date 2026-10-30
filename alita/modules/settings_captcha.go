package modules

import (
	"context"
	"errors"

	"github.com/PaulSonOfLars/gotgbot/v2"
	log "github.com/sirupsen/logrus"

	"github.com/divkix/Alita_Robot/alita/db/captcha"
	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/i18n"
)

const (
	// settingsPageCaptcha is the p value of the Captcha page.
	settingsPageCaptcha = "c"
	// settingsCaptchaTime is the t value of the time limit control.
	settingsCaptchaTime = "t"
)

// The captcha time limit range, which applyCaptchaTimeout enforces and the stepper clamps to.
const (
	settingsCaptchaMinTime = 1
	settingsCaptchaMaxTime = 10
)

// settingsCaptchaRead reads a group's captcha settings. It is the getter /captcha reads,
// with a real error, so a failed read is the cannot-open edit and never a default shown as
// the stored value. It is a variable only so a test can make the read fail.
var settingsCaptchaRead = captcha.GetCaptchaSettingsContext

func init() {
	registerSettingsPage(settingsPage{
		Key:       settingsPageCaptcha,
		Order:     30,
		Icon:      "🧩",
		TitleKey:  "settings_page_captcha",
		MainState: settingsCaptchaMainState,
		Open:      settingsCaptchaOpen,
		Controls: map[string]settingsControl{
			settingsCaptchaTime: {Right: settingsRightAdmin, Set: settingsCaptchaSetTime},
		},
	})
}

// settingsCaptchaMainState is the Captcha button's state: the plain On or Off word.
func settingsCaptchaMainState(_ *gotgbot.Bot, tr *i18n.Translator, group *gotgbot.Chat) (string, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), settingsReadTimeout)
	defer cancel()
	settings, err := settingsCaptchaRead(ctx, group.Id)
	if err != nil || settings == nil {
		return "", false
	}
	if settings.Enabled {
		return settingsText(tr, "common_status_on"), true
	}
	return settingsText(tr, "common_status_off"), true
}

// settingsCaptchaOpen reads the stored settings and draws the page. A read error is
// returned, which the dispatcher turns into the cannot-open edit.
func settingsCaptchaOpen(p *settingsPress, notice string) (settingsScreen, error) {
	ctx, cancel := context.WithTimeout(context.Background(), settingsReadTimeout)
	defer cancel()
	settings, err := settingsCaptchaRead(ctx, p.group.Id)
	if err != nil {
		return settingsScreen{}, err
	}
	if settings == nil {
		return settingsScreen{}, errors.New("captcha settings: no row and no default")
	}
	return renderSettingsCaptcha(p.view(), settings, notice), nil
}

// settingsCaptchaClamp keeps a stepper target inside the time limit range.
func settingsCaptchaClamp(minutes int) int {
	return min(max(minutes, settingsCaptchaMinTime), settingsCaptchaMaxTime)
}

// renderSettingsCaptcha draws the Captcha page: the time limit line and its stepper.
func renderSettingsCaptcha(v settingsView, s *models.CaptchaSettings, notice string) settingsScreen {
	body := []string{
		settingsText(v.Tr, "settings_captcha_time", i18n.TranslationParams{"n": s.Timeout}),
	}
	k := newSettingsKeyboard(v)
	valueLabel := settingsText(v.Tr, "settings_btn_minutes", i18n.TranslationParams{"n": s.Timeout})
	k.row(settingsStepperRow(k, settingsPageCaptcha, settingsCaptchaTime, s.Timeout, []int{-1, 1}, settingsCaptchaClamp, valueLabel)...)
	k.navRow(false)
	return k.screen(settingsCompose(v, "settings_page_captcha", body, notice, false))
}

// settingsCaptchaSetTime applies an absolute time limit through applyCaptchaTimeout, the
// function /captchatime calls, and redraws the page from the stored value. A step at a
// bound answers a toast and writes nothing.
func settingsCaptchaSetTime(p *settingsPress, value string) *settingsScreen {
	minutes, ok := settingsParseNumber(value, 3)
	if !ok {
		p.answer(settingsText(p.tr, "common_callback_invalid_request"), true)
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), settingsReadTimeout)
	defer cancel()

	stored, err := settingsCaptchaRead(ctx, p.group.Id)
	if err != nil || stored == nil {
		log.Warnf("[Settings] captcha settings of chat %d could not be read before a step: %v", p.group.Id, err)
		p.cannotOpen()
		return nil
	}
	if minutes == stored.Timeout {
		switch minutes {
		case settingsCaptchaMinTime:
			p.answer(settingsText(p.tr, "settings_min_value", i18n.TranslationParams{"min": settingsCaptchaMinTime}), false)
			return nil
		case settingsCaptchaMaxTime:
			p.answer(settingsText(p.tr, "settings_max_value", i18n.TranslationParams{"max": settingsCaptchaMaxTime}), false)
			return nil
		}
	}

	result := applyCaptchaTimeout(ctx, p.group.Id, int64(minutes))
	notice := ""
	current := stored
	switch result.Outcome {
	case captchaApplied:
		current = result.Settings
	default:
		notice = settingsNoticeWarn(settingsText(p.tr, result.Key))
	}
	if current == nil {
		// The write succeeded but its read-back failed: read again.
		current, err = settingsCaptchaRead(ctx, p.group.Id)
		if err != nil || current == nil {
			log.Warnf("[Settings] captcha settings of chat %d could not be read after a step: %v", p.group.Id, err)
			p.cannotOpen()
			return nil
		}
	}
	screen := renderSettingsCaptcha(p.view(), current, notice)
	return &screen
}
