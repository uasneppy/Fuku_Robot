//go:build testtools

package modules

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"gorm.io/gorm"

	"github.com/uasneppy/fuku_robot/fuku/config"
	"github.com/uasneppy/fuku_robot/fuku/db"
	dbcache "github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/lang"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
	"github.com/uasneppy/fuku_robot/fuku/utils/keyboard"
)

// settingsLangDefaultCodes is the default enabled set, in the order ENABLED_LOCALES falls back to.
var settingsLangDefaultCodes = []string{"uk", "en", "es", "fr", "hi", "id", "pt", "ru"}

// settingsLangWithCodes sets the enabled languages for the test, which the test process does not
// otherwise have, and puts the previous value back when it ends.
func settingsLangWithCodes(t *testing.T, codes []string) {
	t.Helper()
	previous := config.AppConfig.ValidLangCodes
	config.AppConfig.ValidLangCodes = codes
	t.Cleanup(func() { config.AppConfig.ValidLangCodes = previous })
}

// settingsLangCleanup deletes the chats' rows and the members' users rows with their language,
// settings and user cache keys and the admin cache before the test and when it ends, then resets
// the local cache layer.
func settingsLangCleanup(t *testing.T, chatIDs, userIDs []int64) {
	t.Helper()
	languageGoldenCleanup(t, chatIDs, userIDs)
	t.Cleanup(dbcache.ResetLocalForTest)
}

// newSettingsLanguageEnv is the settings environment with the real locale files, the /lang module
// and the default enabled languages. The env's chat has no chats row.
func newSettingsLanguageEnv(t *testing.T) *settingsEnv {
	t.Helper()
	env := newSettingsEnvWith(t, settingsEnvOptions{RealLocale: true})
	LoadLanguage(env.dispatcher)
	settingsLangWithCodes(t, settingsLangDefaultCodes)
	settingsLangCleanup(t, []int64{env.chat.Id}, nil)
	return env
}

// settingsLangStored is the language of the chat's chats row read straight from the database, and
// whether the chat has a row.
func settingsLangStored(t *testing.T, chatID int64) (string, bool) {
	t.Helper()
	var row models.Chat
	err := db.DB.Where("chat_id = ?", chatID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", false
	}
	if err != nil {
		t.Fatalf("read chats row of %d: %v", chatID, err)
	}
	return row.Language, true
}

// settingsLangCell is the grid button that presses code, and whether the page has one.
func settingsLangCell(t *testing.T, kb [][]gotgbot.InlineKeyboardButton, code string) (gotgbot.InlineKeyboardButton, bool) {
	t.Helper()
	for _, button := range settingsAllButtons(kb) {
		fields := settingsButtonFields(t, button)
		if fields["a"] == "st" && fields["p"] == settingsPageLang && fields["t"] == settingsLangControlKey && fields["v"] == code {
			return button, true
		}
	}
	return gotgbot.InlineKeyboardButton{}, false
}

// settingsLangRowWidths is the number of buttons of each row.
func settingsLangRowWidths(kb [][]gotgbot.InlineKeyboardButton) []int {
	widths := make([]int, len(kb))
	for i, row := range kb {
		widths[i] = len(row)
	}
	return widths
}

// settingsLangOpenPage opens the Language page of the env's chat in a group menu opened by the
// env's administrator and returns the message ID, the page text and its keyboard.
func settingsLangOpenPage(t *testing.T, env *settingsEnv) (int64, string, [][]gotgbot.InlineKeyboardButton) {
	t.Helper()
	msgID := env.openMenu(env.admin)
	_, mainKeyboard := env.lastEdit(env.chat.Id, msgID)
	button, ok := settingsPageButton(t, mainKeyboard, settingsPageLang)
	if !ok {
		t.Fatalf("Main has no Language button: %+v", mainKeyboard)
	}
	env.pressRaw(env.chat, env.admin, msgID, button.CallbackData)
	text, kb := env.lastEdit(env.chat.Id, msgID)
	return msgID, text, kb
}

// settingsLangChatRows counts the chats rows of the chat.
func settingsLangChatRows(t *testing.T, chatID int64) int64 {
	t.Helper()
	var n int64
	if err := db.DB.Model(&models.Chat{}).Where("chat_id = ?", chatID).Count(&n).Error; err != nil {
		t.Fatalf("count chats of %d: %v", chatID, err)
	}
	return n
}

// settingsLangLastSent is the text of the last message the bot sent to the chat.
func settingsLangLastSent(t *testing.T, env *settingsEnv, chatID int64) string {
	t.Helper()
	sent := env.fake.sentTo(chatID)
	if len(sent) == 0 {
		t.Fatalf("the bot sent nothing to chat %d", chatID)
	}
	return fmt.Sprint(sent[len(sent)-1].Params["text"])
}

func TestSettingsLanguageGroupRedraw(t *testing.T) {
	env := newSettingsLanguageEnv(t)
	if err := lang.ChangeGroupLanguage(env.chat.Id, "en"); err != nil {
		t.Fatalf("seed English: %v", err)
	}
	en := i18n.MustNewTranslator("en")
	es := i18n.MustNewTranslator("es")
	mustText := func(tr *i18n.Translator, key string, params ...i18n.TranslationParams) string {
		t.Helper()
		text, err := tr.GetString(key, params...)
		if err != nil || text == "" {
			t.Fatalf("locale key %s: %q, %v", key, text, err)
		}
		return text
	}

	// Main lists the Language page, which opens on the stored language with the grid.
	msgID, text, kb := settingsLangOpenPage(t, env)
	wantCurrent := mustText(en, "settings_lang_current", i18n.TranslationParams{"lang": keyboard.GetLangFormat("en")})
	for _, want := range []string{wantCurrent, mustText(en, "settings_lang_hint"), mustText(en, "settings_page_lang")} {
		if !strings.Contains(text, want) {
			t.Errorf("English page %q does not contain %q", text, want)
		}
	}
	if got, want := settingsLangRowWidths(kb), []int{2, 2, 2, 2, 2}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("page rows = %v, want %v (eight cells two to a row, then Back and Close)", got, want)
	}
	for _, code := range settingsLangDefaultCodes {
		if _, ok := settingsLangCell(t, kb, code); !ok {
			t.Errorf("the page has no cell for %s", code)
		}
	}
	enCell, _ := settingsLangCell(t, kb, "en")
	if enCell.Text != "✅ English" {
		t.Errorf("the stored language's cell = %q, want the check mark in place of the flag", enCell.Text)
	}
	if cell, _ := settingsLangCell(t, kb, "es"); cell.Text != "🇪🇸 Español" {
		t.Errorf("an unmarked cell = %q, want its flag and its own name", cell.Text)
	}
	chatRows := settingsLangChatRows(t, env.chat.Id)

	// A press of Español stores es and redraws the whole page in Spanish.
	if cached := lang.GetGroupLanguage(context.Background(), env.chat.Id); cached != "en" {
		t.Fatalf("language before the press = %q, want en", cached)
	}
	esCell, _ := settingsLangCell(t, kb, "es")
	answers := env.answerCount()
	env.pressRaw(env.chat, env.admin, msgID, esCell.CallbackData)
	if env.answerCount() != answers+1 {
		t.Errorf("answers after the press = %d, want exactly one more than %d", env.answerCount(), answers)
	}
	if stored, _ := settingsLangStored(t, env.chat.Id); stored != "es" {
		t.Fatalf("stored language = %q, want es", stored)
	}
	text, kb = env.lastEdit(env.chat.Id, msgID)
	wantNotice := "✅ " + mustText(es, "language_changed_group", i18n.TranslationParams{"s": keyboard.GetLangFormat("es")})
	for _, want := range []string{
		mustText(es, "settings_header", i18n.TranslationParams{"page": mustText(es, "settings_page_lang")}),
		mustText(es, "settings_lang_current", i18n.TranslationParams{"lang": keyboard.GetLangFormat("es")}),
		mustText(es, "settings_lang_hint"),
		wantNotice,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("Spanish redraw %q does not contain %q", text, want)
		}
	}
	if unwanted := mustText(en, "settings_lang_hint"); strings.Contains(text, unwanted) {
		t.Errorf("Spanish redraw %q still contains the English hint", text)
	}
	last := kb[len(kb)-1]
	if len(last) != 2 || last[0].Text != mustText(es, "settings_btn_back") || last[1].Text != mustText(es, "settings_btn_close") {
		t.Errorf("navigation row = %+v, want the Spanish Back and Close", last)
	}
	if cell, _ := settingsLangCell(t, kb, "es"); cell.Text != "✅ Español" {
		t.Errorf("the new stored language's cell = %q", cell.Text)
	}
	if cell, _ := settingsLangCell(t, kb, "en"); cell.Text != "🇺🇸 English" {
		t.Errorf("the old language's cell = %q", cell.Text)
	}

	// The three keys were dropped: the cached language reads es, and the next press builds its
	// translator from it.
	if got := lang.GetGroupLanguage(context.Background(), env.chat.Id); got != "es" {
		t.Errorf("cached group language after the press = %q, want es", got)
	}
	env.pressRaw(env.chat, env.admin, msgID, esCell.CallbackData)
	text, _ = env.lastEdit(env.chat.Id, msgID)
	if !strings.Contains(text, mustText(es, "settings_lang_hint")) || strings.Contains(text, "✅ "+mustText(es, "language_changed_group", i18n.TranslationParams{"s": keyboard.GetLangFormat("es")})) {
		t.Errorf("a press of the stored language drew %q, want the Spanish page with no notice", text)
	}

	// A group message the bot answers next is in Spanish.
	env.sendIn(env.chat, env.admin, "/lang")
	reply := settingsLangLastSent(t, env, env.chat.Id)
	wantReply := mustText(es, "language_current_group", i18n.TranslationParams{"s": keyboard.GetLangFormat("es")})
	if first, _, _ := strings.Cut(wantReply, "\n"); !strings.Contains(reply, first) {
		t.Errorf("the next group reply = %q, want it in Spanish (%q)", reply, first)
	}
	// Opening, pressing and redrawing created no chats row beyond the seed.
	if got := settingsLangChatRows(t, env.chat.Id); got != chatRows {
		t.Errorf("chats rows went from %d to %d", chatRows, got)
	}
}
