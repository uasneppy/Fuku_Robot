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
	// settingsCaptchaEnabled is the t value of the on/off toggle.
	settingsCaptchaEnabled = "e"
	// settingsCaptchaMode is the t value of the mode choice.
	settingsCaptchaMode = "m"
	// settingsCaptchaAction is the t value of the failure action choice.
	settingsCaptchaAction = "f"
	// settingsCaptchaUnavailableMark prefixes the Turnstile label when no web captcha runtime
	// is installed, so the choice is visibly one the bot would refuse.
	settingsCaptchaUnavailableMark = "🚫 "
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
			settingsCaptchaEnabled: {Right: settingsRightAdminBotAdmin, Set: settingsCaptchaSetEnabled},
			settingsCaptchaMode:    {Right: settingsRightAdmin, Set: settingsCaptchaSetMode},
			settingsCaptchaTime:    {Right: settingsRightAdmin, Set: settingsCaptchaSetTime},
			settingsCaptchaAction:  {Right: settingsRightAdmin, Set: settingsCaptchaSetAction},
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
	return renderSettingsCaptcha(p.view(), settings, settingsCaptchaWebOf(p.b), notice), nil
}

// settingsCaptchaWeb is what the page knows about the Turnstile web captcha when it draws.
type settingsCaptchaWeb struct {
	// Installed is true when a web captcha runtime is installed and the bot has a username;
	// it does no I/O, so the page can label Turnstile without waiting.
	Installed bool
	// Available is true when, besides that, Redis answered the memoised 5 second probe, so
	// a join in a Turnstile group gets the web challenge and not the math fallback.
	Available bool
}

// settingsCaptchaWebOf reads the web captcha state for a page draw: the installed check
// has no I/O and the availability is the 5 second memo a join uses.
func settingsCaptchaWebOf(b *gotgbot.Bot) settingsCaptchaWeb {
	web := settingsCaptchaWeb{Installed: captchaWebInstalled(b)}
	if web.Installed {
		_, web.Available = captchaWebAvailable(b)
	}
	return web
}

// settingsCaptchaClamp keeps a stepper target inside the time limit range.
func settingsCaptchaClamp(minutes int) int {
	return min(max(minutes, settingsCaptchaMinTime), settingsCaptchaMaxTime)
}

// settingsCaptchaModeWords returns the option word and the description of a captcha mode.
// The keys are literals so the locale check sees them. A stored mode outside the three the
// table's check allows reads as math, which is what a join does with a mode it cannot run.
func settingsCaptchaModeWords(tr *i18n.Translator, mode string) (word, desc string) {
	switch mode {
	case "text":
		return settingsText(tr, "settings_opt_text"), settingsText(tr, "captcha_mode_text_desc")
	case "turnstile":
		return settingsText(tr, "settings_opt_turnstile"), settingsText(tr, "captcha_mode_turnstile_desc")
	}
	return settingsText(tr, "settings_opt_math"), settingsText(tr, "captcha_mode_math_desc")
}

// settingsCaptchaActionWord returns the option word of a failure action; an unknown stored
// value reads as kick, the default.
func settingsCaptchaActionWord(tr *i18n.Translator, action string) string {
	switch action {
	case "ban":
		return settingsText(tr, "settings_opt_ban")
	case "mute":
		return settingsText(tr, "settings_opt_mute")
	}
	return settingsText(tr, "settings_opt_kick")
}

// renderSettingsCaptcha draws the Captcha page: the switch, the mode with its description
// (and the fallback note when a stored Turnstile cannot run), the time limit, the failure
// action, and a keyboard with the toggle, the mode choice, the time stepper and the action
// choice. The Turnstile label carries the unavailable mark when no web captcha runtime is
// installed; that costs no I/O.
func renderSettingsCaptcha(v settingsView, s *models.CaptchaSettings, web settingsCaptchaWeb, notice string) settingsScreen {
	modeWord, modeDesc := settingsCaptchaModeWords(v.Tr, s.CaptchaMode)
	body := []string{
		settingsText(v.Tr, "settings_captcha_status", i18n.TranslationParams{"state": settingsStateWord(v.Tr, s.Enabled)}),
		settingsText(v.Tr, "settings_captcha_mode", i18n.TranslationParams{"mode": modeWord, "desc": modeDesc}),
	}
	if s.CaptchaMode == "turnstile" && !web.Available {
		body = append(body, settingsText(v.Tr, "settings_captcha_fallback_note"))
	}
	body = append(body,
		settingsText(v.Tr, "settings_captcha_time", i18n.TranslationParams{"n": s.Timeout}),
		settingsText(v.Tr, "settings_captcha_action", i18n.TranslationParams{"action": settingsCaptchaActionWord(v.Tr, s.FailureAction)}),
	)

	k := newSettingsKeyboard(v)
	toggleLabel := settingsText(v.Tr, "settings_btn_captcha_toggle", i18n.TranslationParams{"state": settingsStateWord(v.Tr, s.Enabled)})
	k.row(settingsToggleButton(k, settingsPageCaptcha, settingsCaptchaEnabled, toggleLabel, s.Enabled)...)

	turnstileLabel := settingsText(v.Tr, "settings_opt_turnstile")
	if !web.Installed {
		turnstileLabel = settingsCaptchaUnavailableMark + turnstileLabel
	}
	k.row(settingsChoiceRow(k, settingsPageCaptcha, settingsCaptchaMode, []settingsOption{
		{Value: "math", Label: settingsText(v.Tr, "settings_opt_math")},
		{Value: "text", Label: settingsText(v.Tr, "settings_opt_text")},
		{Value: "turnstile", Label: turnstileLabel},
	}, s.CaptchaMode)...)

	valueLabel := settingsText(v.Tr, "settings_btn_minutes", i18n.TranslationParams{"n": s.Timeout})
	k.row(settingsStepperRow(k, settingsPageCaptcha, settingsCaptchaTime, s.Timeout, []int{-1, 1}, settingsCaptchaClamp, valueLabel)...)

	k.row(settingsChoiceRow(k, settingsPageCaptcha, settingsCaptchaAction, []settingsOption{
		{Value: "kick", Label: settingsText(v.Tr, "settings_opt_kick")},
		{Value: "ban", Label: settingsText(v.Tr, "settings_opt_ban")},
		{Value: "mute", Label: settingsText(v.Tr, "settings_opt_mute")},
	}, s.FailureAction)...)
	k.navRow(false)
	return k.screen(settingsCompose(v, "settings_page_captcha", body, notice, false))
}

// settingsCaptchaRedraw finishes a control press: it draws the page from the stored row the
// apply function read back (or reads it again when the read-back failed or the apply
// function wrote nothing), with a warning line for every outcome but success. The warning
// is the command's own text for the same refusal. A row that cannot be read is the
// cannot-open edit.
func settingsCaptchaRedraw(p *settingsPress, result captchaApplyResult) *settingsScreen {
	notice := ""
	if result.Outcome != captchaApplied {
		notice = settingsNoticeWarn(settingsText(p.tr, result.Key))
	}
	current := result.Settings
	if current == nil {
		ctx, cancel := context.WithTimeout(context.Background(), settingsReadTimeout)
		defer cancel()
		var err error
		current, err = settingsCaptchaRead(ctx, p.group.Id)
		if err != nil || current == nil {
			log.Warnf("[Settings] captcha settings of chat %d could not be read after a press: %v", p.group.Id, err)
			p.cannotOpen()
			return nil
		}
	}
	screen := renderSettingsCaptcha(p.view(), current, settingsCaptchaWebOf(p.b), notice)
	return &screen
}

// settingsCaptchaSetEnabled switches the captcha on ("1") or off ("0") through
// applyCaptchaEnabled, the function /captcha on|off calls. The press is answered empty first,
// because turning the captcha off releases every pending member with Telegram calls that
// must not outlast the callback (D-31). A failed cleanup is the command's own
// captcha_disable_failed line on a page that shows Off.
func settingsCaptchaSetEnabled(p *settingsPress, value string) *settingsScreen {
	if value != "0" && value != "1" {
		p.answer(settingsText(p.tr, "common_callback_invalid_request"), true)
		return nil
	}
	p.answer("", false)
	return settingsCaptchaRedraw(p, applyCaptchaEnabled(p.b, p.group.Id, value == "1"))
}

// settingsCaptchaWordShape reports whether v has the shape of a mode or action word: 1 to
// 16 lower-case ASCII letters. A value of another shape is no button of this page and is
// answered as an invalid request. A word of the right shape that the command would refuse
// (a mode or action it does not know) goes to the apply function, which refuses it with the
// command's own text, so a hand-built press and the command fail the same way.
func settingsCaptchaWordShape(v string) bool {
	if v == "" || len(v) > 16 {
		return false
	}
	for i := 0; i < len(v); i++ {
		if v[i] < 'a' || v[i] > 'z' {
			return false
		}
	}
	return true
}

// settingsCaptchaSetMode stores the mode math, text or turnstile through applyCaptchaMode,
// the function /captchamode calls, so Turnstile meets the same three ordered refusals.
func settingsCaptchaSetMode(p *settingsPress, value string) *settingsScreen {
	if !settingsCaptchaWordShape(value) {
		p.answer(settingsText(p.tr, "common_callback_invalid_request"), true)
		return nil
	}
	return settingsCaptchaRedraw(p, applyCaptchaMode(p.b, p.group.Id, value))
}

// settingsCaptchaSetAction stores the failure action kick, ban or mute through
// applyCaptchaAction, the function /captchaaction calls.
func settingsCaptchaSetAction(p *settingsPress, value string) *settingsScreen {
	if !settingsCaptchaWordShape(value) {
		p.answer(settingsText(p.tr, "common_callback_invalid_request"), true)
		return nil
	}
	return settingsCaptchaRedraw(p, applyCaptchaAction(p.group.Id, value))
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
	screen := renderSettingsCaptcha(p.view(), current, settingsCaptchaWebOf(p.b), notice)
	return &screen
}
