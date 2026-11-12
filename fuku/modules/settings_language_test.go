//go:build testtools

package modules

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
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

// settingsLangPress hand-builds a press of the Language page on message msgID of chat, as the
// env's administrator in a group menu.
func settingsLangPress(t *testing.T, env *settingsEnv, chat gotgbot.Chat, msgID int64, kv ...string) {
	t.Helper()
	data := encodeCallbackData(settingsCallbackNamespace, env.groupFields(env.admin, kv...))
	if data == "" {
		t.Fatalf("hand-built press %v did not encode", kv)
	}
	env.pressRaw(chat, env.admin, msgID, data)
}

// settingsLangSetPress is the st press of the language code on the Language page.
func settingsLangSetPress(t *testing.T, env *settingsEnv, chat gotgbot.Chat, msgID int64, code string) {
	t.Helper()
	settingsLangPress(t, env, chat, msgID, "a", "st", "p", settingsPageLang, "t", settingsLangControlKey, "v", code)
}

// settingsLangCallback presses the language button of /lang for code in chat as user, through the
// callback's own handler.
func settingsLangCallback(t *testing.T, env *settingsEnv, chat gotgbot.Chat, user gotgbot.User, code string) {
	t.Helper()
	data := encodeCallbackData("change_language", map[string]string{"l": code})
	if err := languagesModule.langBtnHandler(env.bot, newModuleCallbackContext(env.bot, chat, user, data)); !errors.Is(err, ext.EndGroups) {
		t.Fatalf("change_language(%s): error = %v, want EndGroups", code, err)
	}
}

// settingsLangRowJSON is the whole chats row of the chat with its ID and timestamps blanked, as
// JSON, or "none" when there is no row.
func settingsLangRowJSON(t *testing.T, chatID int64) string {
	t.Helper()
	var row models.Chat
	err := db.DB.Where("chat_id = ?", chatID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "none"
	}
	if err != nil {
		t.Fatalf("read chats row of %d: %v", chatID, err)
	}
	row.ChatId = 0
	row.CreatedAt, row.UpdatedAt = time.Time{}, time.Time{}
	raw, err := json.Marshal(row)
	if err != nil {
		t.Fatalf("encode chats row: %v", err)
	}
	return string(raw)
}

func TestSettingsParityLanguage(t *testing.T) {
	const menuMsgID = 7474

	for _, tc := range []struct{ name, seed, code string }{
		{"fresh chats, fr", "", "fr"},
		{"stored en, ru", "en", "ru"},
		{"stored es, uk", "es", "uk"},
		{"the stored code again", "id", "id"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := newSettingsLanguageEnv(t)
			callbackChat, menuChat := env.addChat(), env.addChat()
			settingsLangCleanup(t, []int64{callbackChat.Id, menuChat.Id}, nil)
			for _, chat := range []gotgbot.Chat{callbackChat, menuChat} {
				if tc.seed != "" {
					if err := lang.ChangeGroupLanguage(chat.Id, tc.seed); err != nil {
						t.Fatalf("seed: %v", err)
					}
				}
				languageApplyPrime(t, chat.Id)
			}

			settingsLangCallback(t, env, callbackChat, env.admin, tc.code)
			settingsLangSetPress(t, env, menuChat, menuMsgID, tc.code)

			if a, b := settingsLangRowJSON(t, callbackChat.Id), settingsLangRowJSON(t, menuChat.Id); a != b {
				t.Errorf("chats rows differ\n callback %s\n menu     %s", a, b)
			}
			dropped := func(chatID int64) []bool {
				var flags []bool
				for _, key := range languageApplyKeys(chatID) {
					flags = append(flags, languageApplyDropped(t, chatID)[key])
				}
				return flags
			}
			if a, b := fmt.Sprint(dropped(callbackChat.Id)), fmt.Sprint(dropped(menuChat.Id)); a != b {
				t.Errorf("dropped cache keys differ: callback %s, menu %s", a, b)
			}
			if a, b := languageApplyChatRead(callbackChat.Id), languageApplyChatRead(menuChat.Id); a != b || a != tc.code {
				t.Errorf("cached chat entries read %q (callback) and %q (menu), want %q", a, b, tc.code)
			}
		})
	}
}

func TestSettingsDriftLanguage(t *testing.T) {
	const menuMsgID = 7575

	t.Run("a write of the callback is on the page at the next draw", func(t *testing.T) {
		env := newSettingsLanguageEnv(t)
		if err := lang.ChangeGroupLanguage(env.chat.Id, "en"); err != nil {
			t.Fatalf("seed: %v", err)
		}
		_, _, before := settingsLangOpenPage(t, env)
		if cell, _ := settingsLangCell(t, before, "en"); cell.Text != "✅ English" {
			t.Fatalf("English cell before = %q", cell.Text)
		}
		settingsLangCallback(t, env, env.chat, env.admin, "ru")
		dbcache.ResetLocalForTest()
		_, text, kb := settingsLangOpenPage(t, env)
		ru := i18n.MustNewTranslator("ru")
		wantCurrent, _ := ru.GetString("settings_lang_current", i18n.TranslationParams{"lang": keyboard.GetLangFormat("ru")})
		if !strings.Contains(text, wantCurrent) {
			t.Errorf("the page after the callback's write = %q, want %q", text, wantCurrent)
		}
		if cell, _ := settingsLangCell(t, kb, "ru"); cell.Text != "✅ Русский" {
			t.Errorf("Russian cell = %q, want it marked", cell.Text)
		}
	})

	t.Run("a write of the page is what the bot reads for the next update and the next press", func(t *testing.T) {
		env := newSettingsLanguageEnv(t)
		if err := lang.ChangeGroupLanguage(env.chat.Id, "en"); err != nil {
			t.Fatalf("seed: %v", err)
		}
		settingsLangOpenPage(t, env)
		settingsLangSetPress(t, env, env.chat, menuMsgID, "pt")
		dbcache.ResetLocalForTest()

		ctx := newModuleMessageContext(env.bot, env.chat, env.admin, "hello")
		if got := lang.GetLanguage(ctx); got != "pt" {
			t.Errorf("lang.GetLanguage for the group's next update = %q, want pt", got)
		}
		pt := i18n.MustNewTranslator("pt")
		wantBack, _ := pt.GetString("settings_btn_back")
		if gotBack, _ := ctxTr(ctx).GetString("settings_btn_back"); gotBack != wantBack {
			t.Errorf("the next press's translator says %q, want %q", gotBack, wantBack)
		}
		// The next press of the menu is built from the stored language.
		settingsLangSetPress(t, env, env.chat, menuMsgID, "pt")
		text, kb := env.lastEdit(env.chat.Id, menuMsgID)
		if hint, _ := pt.GetString("settings_lang_hint"); !strings.Contains(text, hint) {
			t.Errorf("the next press drew %q, want it in Portuguese", text)
		}
		if last := kb[len(kb)-1]; len(last) != 2 || last[0].Text != wantBack {
			t.Errorf("the next press's navigation row = %+v, want the Portuguese Back", last)
		}
	})
}

func TestSettingsAuthorityParityLanguage(t *testing.T) {
	const menuMsgID = 7676

	t.Run("a live administrator writes, as the callback lets one", func(t *testing.T) {
		env := newSettingsLanguageEnv(t)
		callbackChat, menuChat := env.addChat(), env.addChat()
		settingsLangCleanup(t, []int64{callbackChat.Id, menuChat.Id}, nil)
		settingsLangCallback(t, env, callbackChat, env.admin, "es")
		settingsLangSetPress(t, env, menuChat, menuMsgID, "es")
		for _, chat := range []gotgbot.Chat{callbackChat, menuChat} {
			if stored, _ := settingsLangStored(t, chat.Id); stored != "es" {
				t.Errorf("chat %d stored %q, want es", chat.Id, stored)
			}
		}
	})

	t.Run("a plain member is refused by both", func(t *testing.T) {
		env := newSettingsLanguageEnv(t)
		callbackChat, menuChat := env.addChat(), env.addChat()
		member := gotgbot.User{Id: uniqueLinkOwnerID() + 43, FirstName: "Plain"}
		settingsLangCleanup(t, []int64{callbackChat.Id, menuChat.Id}, []int64{member.Id})
		for _, chat := range []gotgbot.Chat{callbackChat, menuChat} {
			env.fake.setMember(chat.Id, member.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})
		}

		settingsLangCallback(t, env, callbackChat, member, "es")
		env.pressPrivate(member, 777, env.privateFields(menuChat.Id, "a", "st", "p", settingsPageLang, "t", settingsLangControlKey, "v", "es"))
		notAdmin, _ := i18n.MustNewTranslator("uk").GetString("settings_not_admin")
		if answer, alert := env.lastAnswer(); !alert || answer != notAdmin {
			t.Errorf("answer = %q (alert %v), want the alert settings_not_admin (%q)", answer, alert, notAdmin)
		}
		for _, chat := range []gotgbot.Chat{callbackChat, menuChat} {
			if _, has := settingsLangStored(t, chat.Id); has {
				t.Errorf("a plain member's press or callback wrote a chats row for chat %d", chat.Id)
			}
		}
	})

	t.Run("an administrator the owner gate refuses is refused by the menu only (the documented carve-out)", func(t *testing.T) {
		resetSettingsOwnerMemo(t)
		env := newSettingsLanguageEnv(t)
		callbackChat, menuChat := env.addChat(), env.addChat()
		settingsLangCleanup(t, []int64{callbackChat.Id, menuChat.Id}, nil)
		for _, chat := range []gotgbot.Chat{callbackChat, menuChat} {
			env.newOwner(chat)
			env.setAdminsAllowed(chat.Id, false)
		}

		answers := env.answerCount()
		settingsLangSetPress(t, env, menuChat, menuMsgID, "es")
		if got := env.answerCount() - answers; got != 1 {
			t.Errorf("the gated press answered %d times, want once", got)
		}
		gated, _ := i18n.MustNewTranslator("uk").GetString("settings_gate_owner_only")
		if answer, alert := env.lastAnswer(); !alert || answer != gated {
			t.Errorf("answer = %q (alert %v), want the alert settings_gate_owner_only (%q)", answer, alert, gated)
		}
		if _, has := settingsLangStored(t, menuChat.Id); has {
			t.Error("the gated press wrote a chats row")
		}

		// The callback is not tightened: the same administrator still sets it there.
		settingsLangCallback(t, env, callbackChat, env.admin, "es")
		if stored, _ := settingsLangStored(t, callbackChat.Id); stored != "es" {
			t.Errorf("the callback stored %q for an administrator the menu refuses, want es", stored)
		}

		// Where the menu allows the control, the callback allows it too.
		env.setAdminsAllowed(menuChat.Id, true)
		settingsLangSetPress(t, env, menuChat, menuMsgID, "es")
		if stored, _ := settingsLangStored(t, menuChat.Id); stored != "es" {
			t.Errorf("with the switch on the press stored %q, want es", stored)
		}
	})
}

func TestSettingsLanguageReadsCreateNothing(t *testing.T) {
	t.Run("opening Main and the page creates no row and no key the bot's getter would not make", func(t *testing.T) {
		env := newSettingsLanguageEnv(t)
		chatsBefore := settingsLangChatRows(t, env.chat.Id)
		settingsLangOpenPage(t, env)
		if got := settingsLangChatRows(t, env.chat.Id); got != chatsBefore || got != 0 {
			t.Errorf("chats rows went from %d to %d on opening Main and the page", chatsBefore, got)
		}
		// Main also reads the other pages' states, which make their own keys; the language reader
		// makes the one key the bot's own group read makes and no chat or chat-settings entry.
		for _, key := range env.redis.Keys() {
			for _, module := range []string{"chat_settings", "chat"} {
				if key == dbcache.CacheKey(module, env.chat.Id) {
					t.Errorf("opening the page left the cache key %s", key)
				}
			}
		}
		if !slices.Contains(env.redis.Keys(), dbcache.CacheKey("chat_lang", env.chat.Id)) {
			t.Error("the page's read left no chat_lang entry: it did not read the key the bot reads")
		}
		// The page showed the bot's default, which nothing stored.
		if _, has := settingsLangStored(t, env.chat.Id); has {
			t.Error("a chats row exists")
		}
	})

	t.Run("a failing reader is the cannot-open edit and the unknown mark", func(t *testing.T) {
		env := newSettingsLanguageEnv(t)
		msgID := env.openMenu(env.admin)
		_, mainKeyboard := env.lastEdit(env.chat.Id, msgID)
		button, ok := settingsPageButton(t, mainKeyboard, settingsPageLang)
		if !ok {
			t.Fatal("Main has no Language button")
		}
		mreqSwap(t, &settingsLangRead, func(context.Context, int64) (string, error) {
			return "", errors.New("database is down")
		})
		env.pressRaw(env.chat, env.admin, msgID, button.CallbackData)
		text, _ := env.lastEdit(env.chat.Id, msgID)
		cannot, _ := i18n.MustNewTranslator("uk").GetString("settings_cannot_open")
		if !strings.Contains(text, cannot) {
			t.Errorf("a failed read drew %q, want the cannot-open edit", text)
		}
		msgID = env.openMenu(env.admin)
		mainText, _ := env.lastEdit(env.chat.Id, msgID)
		title, _ := i18n.MustNewTranslator("uk").GetString("settings_page_lang")
		found := false
		for _, line := range strings.Split(mainText, "\n") {
			if strings.Contains(line, title) && strings.HasSuffix(line, settingsUnknownState) {
				found = true
			}
		}
		if !found {
			t.Errorf("Main %q has no Language line ending in the unknown mark", mainText)
		}
	})
}
