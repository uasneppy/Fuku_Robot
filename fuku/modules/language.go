package modules

import (
	"context"
	"slices"

	log "github.com/sirupsen/logrus"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers/filters/callbackquery"

	"github.com/uasneppy/fuku_robot/fuku/db/lang"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
	"github.com/uasneppy/fuku_robot/fuku/utils/chat_status"
	"github.com/uasneppy/fuku_robot/fuku/utils/formatting"
	"github.com/uasneppy/fuku_robot/fuku/utils/helpers"
	"github.com/uasneppy/fuku_robot/fuku/utils/keyboard"
)

var languagesModule = moduleStruct{moduleName: "Languages"}

// groupLanguageCodes are the languages the change_language callback accepts, in the order it
// has always listed them. The /lang keyboard offers the enabled subset of them
// (config.AppConfig.ValidLangCodes); this list is the validation of a press.
var groupLanguageCodes = []string{"uk", "en", "es", "fr", "hi", "ru", "pt", "id"}

// languageWrites is the group language write applyGroupLanguage goes through. It is a variable
// only so a test can make the write fail.
var languageWrites = lang.ChangeGroupLanguage

// langApplyOutcome is how applyGroupLanguage ended.
type langApplyOutcome int

const (
	// langApplied is the zero value: the code was stored (or was already the stored one).
	langApplied langApplyOutcome = iota
	// langUnsupported is a code outside groupLanguageCodes; nothing was written.
	langUnsupported
	// langSaveFailed is a write that failed; Err holds the error.
	langSaveFailed
)

// langApplyResult is what applyGroupLanguage reports: the outcome, the locale key of the reply
// the callback sends with its parameter (the new language's name and flag, `s`), and the write
// error of a failed save.
type langApplyResult struct {
	Outcome langApplyOutcome
	Key     string
	Params  i18n.TranslationParams
	Err     error
}

// applyGroupLanguage stores a group's language. It is the one write path of the group branch of
// the change_language callback (the buttons of /lang) and of the settings menu's Language page;
// the private branch writes the user's own language and never reaches it. A code outside
// groupLanguageCodes is refused before any write. lang.ChangeGroupLanguage drops the chat_lang,
// chat_settings and chat cache keys after a write and writes nothing for a code that is already
// stored.
func applyGroupLanguage(ctx context.Context, chatID int64, code string) langApplyResult {
	if !slices.Contains(groupLanguageCodes, code) {
		log.Warnf("[Language] Unsupported callback language: %s", code)
		return langApplyResult{Outcome: langUnsupported, Key: "language_invalid_selection"}
	}
	if err := ctx.Err(); err != nil {
		return langApplyResult{Outcome: langSaveFailed, Key: "common_settings_save_failed", Err: err}
	}
	if err := languageWrites(chatID, code); err != nil {
		log.Errorf("[Language] ChangeGroupLanguage failed for chat %d: %v", chatID, err)
		return langApplyResult{Outcome: langSaveFailed, Key: "common_settings_save_failed", Err: err}
	}
	return langApplyResult{
		Outcome: langApplied,
		Key:     "language_changed_group",
		Params:  i18n.TranslationParams{"s": keyboard.GetLangFormat(code)},
	}
}

func (moduleStruct) genFullLanguageKb() [][]gotgbot.InlineKeyboardButton {
	keyboard := keyboard.MakeLanguageKeyboard()
	tr := i18n.MustNewTranslator("uk")
	helpTranslateText, _ := tr.GetString("language_help_translate")
	keyboard = append(
		keyboard,
		[]gotgbot.InlineKeyboardButton{
			{
				Text: helpTranslateText,
				Url:  "https://crowdin.com/project/fuku_robot",
			},
		},
	)
	return keyboard
}

func (m moduleStruct) changeLanguage(b *gotgbot.Bot, ctx *ext.Context) error {
	user := chat_status.RequireUser(b, ctx)
	if user == nil {
		return ext.EndGroups
	}
	chat := ctx.EffectiveChat
	msg := ctx.EffectiveMessage

	var replyString string

	cLang := lang.GetLanguage(ctx)
	tr := i18n.MustNewTranslator(cLang)

	if ctx.Message.Chat.Type == "private" {
		replyString, _ = tr.GetString("language_current_user", i18n.TranslationParams{"s": keyboard.GetLangFormat(cLang)})
	} else {

		if !chat_status.RequireUserAdmin(b, ctx, chat, user.Id) {
			chat_status.NewPermissionResponder(b).Respond(ctx, "chat_status_user_admin_cmd_error", "chat_status_user_admin_button_error", chat_status.WithReplyFallback())
			return ext.EndGroups
		}

		replyString, _ = tr.GetString("language_current_group", i18n.TranslationParams{"s": keyboard.GetLangFormat(cLang)})
	}

	_, err := msg.Reply(
		b,
		replyString,
		&gotgbot.SendMessageOpts{
			ReplyMarkup: gotgbot.InlineKeyboardMarkup{
				InlineKeyboard: m.genFullLanguageKb(),
			},
		},
	)
	if err != nil {
		log.Error(err)
		return err
	}

	return ext.EndGroups
}

func (moduleStruct) langBtnHandler(b *gotgbot.Bot, ctx *ext.Context) error {
	query, ok := callbackQueryFromContext(ctx)
	if !ok {
		return ext.EndGroups
	}
	if query == nil {
		return ext.EndGroups
	}
	chat := ctx.EffectiveChat
	user := query.From
	if chat == nil || user.Id == 0 {
		return answerInvalidCallback(b, ctx, query)
	}

	language := ""
	if decoded, ok := decodeCallbackData(query.Data, "change_language"); ok {
		language, _ = decoded.Field("l")
	}
	if language == "" {
		log.Warnf("[Language] Invalid callback data format: %s", query.Data)
		tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
		errText, _ := tr.GetString("language_invalid_selection")
		_, _ = query.Answer(b, &gotgbot.AnswerCallbackQueryOpts{
			Text: errText,
		})
		return ext.EndGroups
	}
	if !slices.Contains(groupLanguageCodes, language) {
		log.Warnf("[Language] Unsupported callback language: %s", language)
		currentTr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
		errText, _ := currentTr.GetString("language_invalid_selection")
		_, _ = query.Answer(b, &gotgbot.AnswerCallbackQueryOpts{Text: errText})
		return ext.EndGroups
	}
	tr := i18n.MustNewTranslator(language)

	if chat.Type != "private" {
		if !chat_status.RequireUserAdmin(b, ctx, chat, user.Id) {
			chat_status.NewPermissionResponder(b).Respond(ctx, "chat_status_user_admin_cmd_error", "chat_status_user_admin_button_error", chat_status.WithReplyFallback())
			return ext.EndGroups
		}
	}

	var replyString string

	if chat.Type == "private" {
		if err := lang.ChangeUserLanguage(user.Id, language); err != nil {
			log.Errorf("[Language] ChangeUserLanguage failed for user %d: %v", user.Id, err)
			errText, _ := tr.GetString("common_settings_save_failed")
			_, _ = query.Answer(b, &gotgbot.AnswerCallbackQueryOpts{Text: errText})
			return ext.EndGroups
		}
		replyString, _ = tr.GetString("language_changed_user", i18n.TranslationParams{"s": keyboard.GetLangFormat(language)})
	} else {
		if err := lang.ChangeGroupLanguage(chat.Id, language); err != nil {
			log.Errorf("[Language] ChangeGroupLanguage failed for chat %d: %v", chat.Id, err)
			errText, _ := tr.GetString("common_settings_save_failed")
			_, _ = query.Answer(b, &gotgbot.AnswerCallbackQueryOpts{Text: errText})
			return ext.EndGroups
		}
		replyString, _ = tr.GetString("language_changed_group", i18n.TranslationParams{"s": keyboard.GetLangFormat(language)})
	}

	if query.Message == nil {
		_, err := query.Answer(b, &gotgbot.AnswerCallbackQueryOpts{Text: replyString})
		if err != nil {
			log.Error(err)
			return err
		}
		return ext.EndGroups
	}

	_, err := query.Answer(b, nil)
	if err != nil {
		log.Error(err)
	}

	_, _, err = query.Message.EditText(b, &gotgbot.EditMessageTextOpts{Text: replyString, ParseMode: formatting.HTML,
		LinkPreviewOptions: &gotgbot.LinkPreviewOptions{
			IsDisabled: true,
		}})
	if err != nil {
		log.Error(err)
		return err
	}

	return ext.EndGroups
}

func LoadLanguage(dispatcher *ext.Dispatcher) {
	SetModuleEnabled(languagesModule.moduleName, true)
	SetModuleHelp(languagesModule.moduleName, languagesModule.genFullLanguageKb())

	dispatcher.AddHandler(handlers.NewCallback(callbackquery.Prefix("change_language"), languagesModule.langBtnHandler))
	dispatcher.AddHandler(helpers.NewBotCommand("lang", languagesModule.changeLanguage))
}

func init() {
	RegisterLegacyModule("Languages", 20, LoadLanguage)
}
