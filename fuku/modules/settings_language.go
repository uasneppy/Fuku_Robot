package modules

import (
	"context"
	"html"
	"slices"
	"strings"

	"github.com/PaulSonOfLars/gotgbot/v2"
	log "github.com/sirupsen/logrus"

	"github.com/uasneppy/fuku_robot/fuku/config"
	"github.com/uasneppy/fuku_robot/fuku/db/lang"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
)

// The Language page (page u) sets the language the bot answers in inside the group. It writes
// through applyGroupLanguage, the function the group branch of the change_language callback (the
// buttons of /lang) uses, and reads through lang.LoadGroupLanguageContext. The user's own language
// is a different setting, written by the callback's private branch, which this page never calls.

const (
	// settingsPageLang is the p value of the Language page.
	settingsPageLang = "u"
	// settingsLangControlKey is the t value of the language choice.
	settingsLangControlKey = "l"
	// settingsLangCodeLen is the length of a language code in a press: two lower-case ASCII letters.
	settingsLangCodeLen = 2
	// settingsLangMarker takes the place of the flag on the stored language's cell: a check mark
	// plus the name is 18 code points for the longest name, inside the 20 a cell allows, where
	// the prefix of a choice row would not be.
	settingsLangMarker = "✅"
)

// settingsLangRead reads the group's stored language. It is a variable only so a test can make
// the read fail.
var settingsLangRead = lang.LoadGroupLanguageContext

func init() {
	registerSettingsPage(settingsPage{
		Key:       settingsPageLang,
		Order:     87,
		Icon:      "🌐",
		TitleKey:  "settings_page_lang",
		MainState: settingsLangMainState,
		Open:      settingsLangOpen,
		Controls: map[string]settingsControl{
			// The callback asks for RequireUserAdmin and nothing more, so the live admin check every
			// press makes is the whole right.
			settingsLangControlKey: {
				Right:    settingsRightAdmin,
				Set:      settingsLangSet,
				Actions:  []string{settingsActSet},
				Validate: settingsLangCodeShape,
			},
		},
	})
}

// settingsLangCodeShape is the strict check of a press's v before any I/O: exactly two lower-case
// ASCII letters. Whether the code is one of the eight is the apply function's judgement.
func settingsLangCodeShape(v string) bool {
	if len(v) != settingsLangCodeLen {
		return false
	}
	for i := 0; i < len(v); i++ {
		if v[i] < 'a' || v[i] > 'z' {
			return false
		}
	}
	return true
}

// settingsLangNameFlag is a language's own name and flag from its own locale file.
func settingsLangNameFlag(code string) (name, flag string) {
	tr := i18n.MustNewTranslator(code)
	name, _ = tr.GetString("language_name")
	flag, _ = tr.GetString("language_flag")
	return name, flag
}

// settingsLangDisplay is how a stored language is named: its own name and flag for a language the
// bot has (groupLanguageCodes), else the stored value itself, escaped, because it is data.
func settingsLangDisplay(code string) string {
	if slices.Contains(groupLanguageCodes, code) {
		if name, flag := settingsLangNameFlag(code); strings.TrimSpace(name+flag) != "" {
			return name + " " + flag
		}
	}
	return html.EscapeString(code)
}

// settingsLangMainState is the Language button's state: the stored language by its own name. A
// failed read answers unknown.
func settingsLangMainState(_ *gotgbot.Bot, _ *i18n.Translator, group *gotgbot.Chat) (string, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), settingsReadTimeout)
	defer cancel()
	code, err := settingsLangRead(ctx, group.Id)
	if err != nil {
		return "", false
	}
	return settingsLangDisplay(code), true
}

// settingsLangOpen reads the stored language and draws the page. A read error is returned, which
// the dispatcher turns into the cannot-open edit.
func settingsLangOpen(p *settingsPress, notice string) (settingsScreen, error) {
	ctx, cancel := context.WithTimeout(context.Background(), settingsReadTimeout)
	defer cancel()
	code, err := settingsLangRead(ctx, p.group.Id)
	if err != nil {
		return settingsScreen{}, err
	}
	return renderSettingsLanguage(p.view(), code, notice), nil
}

// renderSettingsLanguage draws the Language page: the current language, the hint and a grid of the
// enabled languages in their own words, two to a row as /lang's keyboard lays them out. The stored
// language's cell carries a check mark in place of its flag; a stored code outside the enabled set
// is named as stored and marks nothing. The Help Us Translate link of /lang is not drawn.
func renderSettingsLanguage(v settingsView, stored, notice string) settingsScreen {
	body := []string{
		settingsText(v.Tr, "settings_lang_current", i18n.TranslationParams{"lang": settingsLangDisplay(stored)}),
		settingsText(v.Tr, "settings_lang_hint"),
	}
	k := newSettingsKeyboard(v)
	var cells []gotgbot.InlineKeyboardButton
	for _, code := range config.AppConfig.ValidLangCodes {
		name, flag := settingsLangNameFlag(code)
		if strings.TrimSpace(name+flag) == "" {
			continue
		}
		label := flag + " " + name
		if code == stored {
			label = settingsLangMarker + " " + name
		}
		fields := map[string]string{"a": settingsActSet, "p": settingsPageLang, "t": settingsLangControlKey, "v": code}
		if button, ok := k.button(label, fields); ok {
			cells = append(cells, button)
		}
	}
	for _, row := range slices.Collect(slices.Chunk(cells, 2)) {
		k.row(row...)
	}
	k.navRow(false)
	return k.screen(settingsCompose(v, "settings_page_lang", body, notice, false))
}

// settingsLangSet stores a language through applyGroupLanguage and redraws. The code was shaped by
// Validate; one that is not among the eight is refused by the apply function and the stored state
// is redrawn with its text as a warn notice. A press of the stored language writes nothing and shows
// no notice. After a write in a menu opened in the group the press's translator is replaced by the
// one built from the code just written (never re-read: another replica's local cache layer can
// still hold the old value), so the whole page, its notice and its Back and Close labels come out in
// the new language; a private menu keeps the opener's own language, which this page never touches.
func settingsLangSet(p *settingsPress, value string) *settingsScreen {
	ctx, cancel := context.WithTimeout(context.Background(), settingsReadTimeout)
	defer cancel()

	before, err := settingsLangRead(ctx, p.group.Id)
	if err != nil {
		log.Warnf("[Settings] language of chat %d could not be read before a press: %v", p.group.Id, err)
		p.cannotOpen()
		return nil
	}
	if before == value {
		screen := renderSettingsLanguage(p.view(), before, "")
		return &screen
	}

	result := applyGroupLanguage(ctx, p.group.Id, value)
	if result.Outcome != langApplied {
		screen := renderSettingsLanguage(p.view(), before, settingsNoticeWarn(settingsText(p.tr, result.Key)))
		return &screen
	}
	if !p.private {
		p.tr = i18n.MustNewTranslator(value)
	}
	screen := renderSettingsLanguage(p.view(), value, settingsNoticeOK(settingsText(p.tr, result.Key, result.Params)))
	return &screen
}
