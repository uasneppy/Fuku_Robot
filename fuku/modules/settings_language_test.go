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

// settingsLangUserLanguage is the language of the user's users row, and whether the user has one.
func settingsLangUserLanguage(t *testing.T, userID int64) (string, bool) {
	t.Helper()
	var row models.User
	err := db.DB.Where("user_id = ?", userID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", false
	}
	if err != nil {
		t.Fatalf("read users row of %d: %v", userID, err)
	}
	return row.Language, true
}

// settingsLangCountWrites swaps the language write seam for one that counts its calls and then
// writes for real, and returns the counter.
func settingsLangCountWrites(t *testing.T) *int {
	t.Helper()
	writes := 0
	write := languageWrites
	mreqSwap(t, &languageWrites, func(chatID int64, code string) error {
		writes++
		return write(chatID, code)
	})
	return &writes
}

func TestSettingsLanguagePrivate(t *testing.T) {
	env := newSettingsLanguageEnv(t)
	owner := env.newOwner(env.chat)
	settingsLangCleanup(t, nil, []int64{owner.Id})
	// The group answers in French; the opener's own language is English.
	if err := lang.ChangeGroupLanguage(env.chat.Id, "fr"); err != nil {
		t.Fatalf("seed the group language: %v", err)
	}
	if err := lang.ChangeUserLanguage(owner.Id, "en"); err != nil {
		t.Fatalf("seed the opener's language: %v", err)
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

	env.sendPrivate(owner, fmt.Sprintf("/start settings_%d", env.chat.Id))
	sentMain := settingsPrivateOnlySent(t, env, owner)
	button, ok := settingsPageButton(t, staffKeyboardOf(sentMain.Params["reply_markup"]), settingsPageLang)
	if !ok {
		t.Fatal("the private Main has no Language button")
	}
	env.pressRaw(settingsPrivateChat(owner), owner, sentMain.MessageID, button.CallbackData)
	text, kb := env.lastEdit(owner.Id, sentMain.MessageID)
	if want := mustText(en, "settings_lang_current", i18n.TranslationParams{"lang": keyboard.GetLangFormat("fr")}); !strings.Contains(text, want) {
		t.Errorf("private page %q does not name the group's language (%q)", text, want)
	}
	if cell, _ := settingsLangCell(t, kb, "fr"); cell.Text != "✅ Français" {
		t.Errorf("the group's stored language cell = %q", cell.Text)
	}
	esCell, ok := settingsLangCell(t, kb, "es")
	if !ok {
		t.Fatal("the private page has no Spanish cell")
	}
	if fields := settingsButtonFields(t, esCell); fields["c"] != fmt.Sprint(env.chat.Id) || fields["u"] != "" {
		t.Errorf("private cell fields = %v, want the group in c and no u", fields)
	}

	env.pressRaw(settingsPrivateChat(owner), owner, sentMain.MessageID, esCell.CallbackData)
	if stored, _ := settingsLangStored(t, env.chat.Id); stored != "es" {
		t.Errorf("chats.language = %q, want es", stored)
	}
	if own, has := settingsLangUserLanguage(t, owner.Id); !has || own != "en" {
		t.Errorf("the opener's users.language = %q (row %v), want en unchanged", own, has)
	}
	text, kb = env.lastEdit(owner.Id, sentMain.MessageID)
	for _, want := range []string{
		mustText(en, "settings_header", i18n.TranslationParams{"page": mustText(en, "settings_page_lang")}),
		mustText(en, "settings_lang_current", i18n.TranslationParams{"lang": keyboard.GetLangFormat("es")}),
		mustText(en, "settings_lang_hint"),
		"✅ " + mustText(en, "language_changed_group", i18n.TranslationParams{"s": keyboard.GetLangFormat("es")}),
	} {
		if !strings.Contains(text, want) {
			t.Errorf("private redraw %q does not contain %q", text, want)
		}
	}
	if strings.Contains(text, mustText(es, "settings_lang_hint")) {
		t.Errorf("private redraw %q switched to the group's language", text)
	}
	if last := kb[len(kb)-1]; len(last) != 2 || last[0].Text != mustText(en, "settings_btn_back") {
		t.Errorf("private navigation row = %+v, want the opener's English Back", last)
	}
	if cell, _ := settingsLangCell(t, kb, "es"); cell.Text != "✅ Español" {
		t.Errorf("the new language's cell = %q", cell.Text)
	}
}

func TestSettingsLanguageSameAndUnknown(t *testing.T) {
	const menuMsgID = 7272

	t.Run("a press of the stored language writes nothing and shows no notice", func(t *testing.T) {
		env := newSettingsLanguageEnv(t)
		if err := lang.ChangeGroupLanguage(env.chat.Id, "es"); err != nil {
			t.Fatalf("seed: %v", err)
		}
		writes := settingsLangCountWrites(t)
		_, _, kb := settingsLangOpenPage(t, env)
		cell, _ := settingsLangCell(t, kb, "es")
		env.pressRaw(env.chat, env.admin, menuMsgID, cell.CallbackData)
		if *writes != 0 {
			t.Errorf("the write seam ran %d times for the stored language, want 0", *writes)
		}
		text, _ := env.lastEdit(env.chat.Id, menuMsgID)
		es := i18n.MustNewTranslator("es")
		changed, _ := es.GetString("language_changed_group", i18n.TranslationParams{"s": "x"})
		if prefix, _, _ := strings.Cut(changed, "x"); strings.Contains(text, prefix) || strings.Contains(text, "⚠️") {
			t.Errorf("a press of the stored language drew a notice: %q", text)
		}
		if got, _ := es.GetString("settings_lang_hint"); !strings.Contains(text, got) {
			t.Errorf("the page was not redrawn in the stored language: %q", text)
		}
	})

	t.Run("a stored code outside the eight is drawn as stored and marks nothing", func(t *testing.T) {
		env := newSettingsLanguageEnv(t)
		if err := db.DB.Create(&models.Chat{ChatId: env.chat.Id, Language: "xx"}).Error; err != nil {
			t.Fatalf("seed a hand-written language: %v", err)
		}
		_, text, kb := settingsLangOpenPage(t, env)
		if !strings.Contains(text, "<b>xx</b>") {
			t.Errorf("page %q does not name the stored code as stored", text)
		}
		for _, button := range settingsAllButtons(kb) {
			if strings.HasPrefix(button.Text, "✅") {
				t.Errorf("button %q is marked although the stored code is not an enabled language", button.Text)
			}
		}
		for _, code := range settingsLangDefaultCodes {
			if _, ok := settingsLangCell(t, kb, code); !ok {
				t.Errorf("the page does not offer %s", code)
			}
		}
	})

	t.Run("a stored code is escaped, never spliced raw", func(t *testing.T) {
		withRealLocales(t)
		view := settingsView{Tr: i18n.MustNewTranslator("en"), OwnerKey: "u", OwnerValue: "1", GroupID: -1001, GroupTitle: "Group"}
		screen := renderSettingsLanguage(view, "<i>x", "")
		if strings.Contains(screen.Text, "<i>x") || !strings.Contains(screen.Text, "&lt;i&gt;x") {
			t.Errorf("a stored code was not escaped: %q", screen.Text)
		}
	})

	t.Run("a two-letter code outside the eight is refused with the callback's text and writes nothing", func(t *testing.T) {
		env := newSettingsLanguageEnv(t)
		if err := lang.ChangeGroupLanguage(env.chat.Id, "en"); err != nil {
			t.Fatalf("seed: %v", err)
		}
		writes := settingsLangCountWrites(t)
		settingsLangOpenPage(t, env)
		data := encodeCallbackData(settingsCallbackNamespace, env.groupFields(env.admin, "a", "st", "p", settingsPageLang, "t", settingsLangControlKey, "v", "xx"))
		env.pressRaw(env.chat, env.admin, menuMsgID, data)
		text, _ := env.lastEdit(env.chat.Id, menuMsgID)
		en := i18n.MustNewTranslator("en")
		refusal, _ := en.GetString("language_invalid_selection")
		if !strings.Contains(text, "⚠️ "+refusal) {
			t.Errorf("v=xx drew %q, want the warn notice %q", text, refusal)
		}
		if *writes != 0 {
			t.Errorf("the write seam ran %d times for v=xx, want 0", *writes)
		}
		if stored, _ := settingsLangStored(t, env.chat.Id); stored != "en" {
			t.Errorf("stored language = %q, want en", stored)
		}
	})

	t.Run("a narrowed ENABLED_LOCALES offers only those languages", func(t *testing.T) {
		env := newSettingsLanguageEnv(t)
		settingsLangWithCodes(t, []string{"uk", "en", "es"})
		_, _, kb := settingsLangOpenPage(t, env)
		if got, want := settingsLangRowWidths(kb), []int{2, 1, 2}; fmt.Sprint(got) != fmt.Sprint(want) {
			t.Errorf("rows = %v, want %v (two grid rows and the navigation row)", got, want)
		}
		if _, ok := settingsLangCell(t, kb, "fr"); ok {
			t.Error("the page offers a language that is not enabled")
		}
	})
}

func TestSettingsLanguageForged(t *testing.T) {
	const menuMsgID = 7373

	forged := []struct {
		name   string
		fields []string
		extra  map[string]string
		both   bool
	}{
		{name: "tg on the language", fields: []string{"a", "tg", "p", "u", "t", "l", "v", "es"}},
		{name: "upper-case code", fields: []string{"a", "st", "p", "u", "t", "l", "v", "UK"}},
		{name: "mixed-case code", fields: []string{"a", "st", "p", "u", "t", "l", "v", "Es"}},
		{name: "three letters", fields: []string{"a", "st", "p", "u", "t", "l", "v", "ukr"}},
		{name: "one letter", fields: []string{"a", "st", "p", "u", "t", "l", "v", "e"}},
		{name: "the toggle shape", fields: []string{"a", "st", "p", "u", "t", "l", "v", "1"}},
		{name: "empty code", fields: []string{"a", "st", "p", "u", "t", "l", "v", ""}},
		{name: "a code with a digit", fields: []string{"a", "st", "p", "u", "t", "l", "v", "e1"}},
		{name: "an extra key", fields: []string{"a", "st", "p", "u", "t", "l", "v", "es"}, extra: map[string]string{"x": "1"}},
		{name: "an unknown control", fields: []string{"a", "st", "p", "u", "t", "x", "v", "es"}},
		{name: "c and u together", fields: []string{"a", "st", "p", "u", "t", "l", "v", "es"}, both: true},
	}
	for _, tc := range forged {
		t.Run(tc.name, func(t *testing.T) {
			env := newSettingsEnv(t)
			settingsLangCleanup(t, []int64{env.chat.Id}, nil)
			writes := settingsLangCountWrites(t)
			fields := env.groupFields(env.admin, tc.fields...)
			for k, v := range tc.extra {
				fields[k] = v
			}
			if tc.both {
				fields["c"] = "-1001" // short, so the pair still fits the 64-byte cap
			}
			data := encodeCallbackData(settingsCallbackNamespace, fields)
			if data == "" {
				t.Fatalf("forged press %v did not encode", fields)
			}
			before := settingsMenuMarkOf(env)
			env.pressRaw(env.chat, env.admin, menuMsgID, data)
			settingsMenuWantRefused(t, env, before, "common_callback_invalid_request", true)
			if *writes != 0 {
				t.Errorf("a forged press reached the write seam %d times", *writes)
			}
			if rows := settingsLangChatRows(t, env.chat.Id); rows != 0 {
				t.Errorf("a forged press created %d chats rows", rows)
			}
		})
	}
}
