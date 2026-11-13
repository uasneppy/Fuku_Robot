package modules

import (
	"context"
	"fmt"
	"html"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers/filters/callbackquery"
	log "github.com/sirupsen/logrus"

	db_filters "github.com/uasneppy/fuku_robot/fuku/db/filters"
	"github.com/uasneppy/fuku_robot/fuku/db/lang"
	"github.com/uasneppy/fuku_robot/fuku/utils/chat_status"
	"github.com/uasneppy/fuku_robot/fuku/utils/formatting"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/i18n"

	"github.com/uasneppy/fuku_robot/fuku/utils/content"
	"github.com/uasneppy/fuku_robot/fuku/utils/extraction"
	"github.com/uasneppy/fuku_robot/fuku/utils/helpers"
	"github.com/uasneppy/fuku_robot/fuku/utils/media"
	"github.com/uasneppy/fuku_robot/fuku/utils/tracing"

	"github.com/uasneppy/fuku_robot/fuku/utils/keyword_matcher"
)

var filtersModule = moduleStruct{
	moduleName:   "Filters",
	handlerGroup: 9,
}

func filterOverwriteCacheKey(token string) string {
	return overwriteCacheKey("filter", token)
}

func setFilterOverwriteCache(token string, data overwriteFilter) error {
	return setOverwriteCache(filterOverwriteCacheKey(token), data)
}

func consumeFilterOverwriteCache(token string) (*overwriteFilter, error) {
	return consumeOverwriteCache[overwriteFilter](filterOverwriteCacheKey(token))
}

func deleteFilterOverwriteCache(token string) {
	deleteOverwriteCache(filterOverwriteCacheKey(token))
}

// filtersCapMax is the most filters a chat may hold.
const filtersCapMax = 150

// filterWriteSeams are the repository calls the filter apply functions make, a variable so a test can
// make one fail.
type filterWriteSeams struct {
	// Add stores one filter and reports whether a row was inserted (false: the keyword was already there).
	Add    func(ctx context.Context, chatID int64, keyword, reply, fileID string, buttons []db.Button, dataType int) (bool, error)
	Remove func(ctx context.Context, chatID int64, keyword string) error
	Count  func(ctx context.Context, chatID int64) (int64, error)
	Exists func(ctx context.Context, chatID int64, keyword string) (bool, error)
}

// filterWrites is the one place the filter commands reach the repository through.
var filterWrites = filterWriteSeams{
	Add:    db_filters.AddFilterContext,
	Remove: db_filters.RemoveFilterContext,
	Count:  db_filters.CountFiltersContext,
	Exists: db_filters.FilterExistsContext,
}

// filterAddOutcome is what applyFilterAdd did; its zero value is a stored filter.
type filterAddOutcome int

const (
	// filterAdded means the filter was stored (and, from checkFilterKeyword, that no check refused it).
	filterAdded filterAddOutcome = iota
	// filterKeywordBlank means the keyword is empty.
	filterKeywordBlank
	// filterKeywordTooLong means the keyword is over listKeywordMaxRunes.
	filterKeywordTooLong
	// filterExists means the chat already has a filter with this keyword; the command offers to overwrite it.
	filterExists
	// filterCapReached means the chat already holds filtersCapMax filters.
	filterCapReached
	// filterRaced means another writer stored the keyword between the check and the insert; /filter
	// answers it like a stored filter, as it always did.
	filterRaced
	// filterSaveFailed means a database call failed and nothing was stored.
	filterSaveFailed
)

// filterAddInput is the filter content applyFilterAdd stores.
type filterAddInput struct {
	Keyword  string
	Text     string
	FileID   string
	Buttons  []db.Button
	DataType int
}

// filterAddResult is what applyFilterAdd and checkFilterKeyword return for the caller to word.
type filterAddResult struct {
	Outcome filterAddOutcome
	Err     error
}

// filterRemoveOutcome is what applyFilterRemove did; its zero value is a removed filter.
type filterRemoveOutcome int

const (
	// filterRemoved means the delete ran (a keyword that is not stored is not an error).
	filterRemoved filterRemoveOutcome = iota
	// filterRemoveSaveFailed means the delete failed.
	filterRemoveSaveFailed
)

// filterRemoveResult is what applyFilterRemove returns.
type filterRemoveResult struct {
	Outcome filterRemoveOutcome
	Err     error
}

// checkFilterKeyword runs the checks every filter add shares, in order: a blank keyword, one over
// listKeywordMaxRunes, a keyword the chat already has, and the chat's cap. It returns a result whose
// outcome is filterAdded when nothing refuses the keyword. It logs the chat ID and the error only,
// never the keyword.
func checkFilterKeyword(ctx context.Context, chatID int64, keyword string) filterAddResult {
	if strings.TrimSpace(keyword) == "" {
		return filterAddResult{Outcome: filterKeywordBlank}
	}
	if utf8.RuneCountInString(keyword) > listKeywordMaxRunes {
		return filterAddResult{Outcome: filterKeywordTooLong}
	}
	exists, err := filterWrites.Exists(ctx, chatID, keyword)
	if err != nil {
		log.Errorf("[Filters] Existing keyword lookup failed for chat %d: %v", chatID, err)
	}
	if exists {
		return filterAddResult{Outcome: filterExists}
	}
	count, err := filterWrites.Count(ctx, chatID)
	if err != nil {
		log.Errorf("[Filters] Filter count lookup failed for chat %d: %v", chatID, err)
	}
	if count >= filtersCapMax {
		return filterAddResult{Outcome: filterCapReached}
	}
	return filterAddResult{}
}

// applyFilterAdd stores one filter for the chat after checkFilterKeyword accepts the keyword. The
// keyword is lower-cased here as the command always did. A keyword stored by another writer in the
// meantime is filterRaced. It judges no person's rights and logs the chat ID and the error only.
func applyFilterAdd(ctx context.Context, chatID int64, in filterAddInput) filterAddResult {
	keyword := strings.ToLower(in.Keyword)
	if refusal := checkFilterKeyword(ctx, chatID, keyword); refusal.Outcome != filterAdded {
		return refusal
	}
	inserted, err := filterWrites.Add(ctx, chatID, keyword, in.Text, in.FileID, in.Buttons, in.DataType)
	if err != nil {
		log.Errorf("[Filters] AddFilter failed for chat %d: %v", chatID, err)
		return filterAddResult{Outcome: filterSaveFailed, Err: err}
	}
	if !inserted {
		return filterAddResult{Outcome: filterRaced}
	}
	return filterAddResult{}
}

// applyFilterRemove deletes the chat's filter with this keyword (lower-cased here). It logs the chat ID
// and the error only.
func applyFilterRemove(ctx context.Context, chatID int64, keyword string) filterRemoveResult {
	if err := filterWrites.Remove(ctx, chatID, strings.ToLower(keyword)); err != nil {
		log.Errorf("[Filters] RemoveFilter failed for chat %d: %v", chatID, err)
		return filterRemoveResult{Outcome: filterRemoveSaveFailed, Err: err}
	}
	return filterRemoveResult{}
}

// replyFilterLimitExceeded answers that the chat holds the most filters it may.
func replyFilterLimitExceeded(b *gotgbot.Bot, ctx *ext.Context) error {
	tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
	text, _ := tr.GetString("filters_limit_exceeded")
	_, err := ctx.EffectiveMessage.Reply(b, text, formatting.Shtml())
	if err != nil {
		log.Error(err)
		return err
	}

	return ext.EndGroups
}

// offerFilterOverwrite answers /filter for a keyword the chat already has with the overwrite card: the
// new content is cached under a fresh token and the Yes and No buttons carry only that token.
func offerFilterOverwrite(b *gotgbot.Bot, ctx *ext.Context, base overwriteBase) error {
	msg := ctx.EffectiveMessage
	token, tokenErr := newOverwriteToken()
	if tokenErr != nil {
		log.Errorf("[Filters] Failed to generate overwrite token: %v", tokenErr)
		tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
		errorText, _ := tr.GetString("filters_overwrite_token_failed")
		_, _ = msg.Reply(b, errorText, formatting.Shtml())
		return ext.EndGroups
	}

	err := setFilterOverwriteCache(token, overwriteFilter{overwriteBase: base})
	if err != nil {
		log.Errorf("[Filters] Failed to cache overwrite data: %v", err)
		tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
		errorText, _ := tr.GetString("filters_overwrite_token_failed")
		_, _ = msg.Reply(b, errorText, formatting.Shtml())
		return ext.EndGroups
	}

	tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
	confirmText, _ := tr.GetString("filters_overwrite_confirm")
	yesText, _ := tr.GetString("common_yes")
	noText, _ := tr.GetString("common_no")
	_, err = msg.Reply(b,
		confirmText,
		&gotgbot.SendMessageOpts{
			ParseMode: formatting.HTML,
			ReplyMarkup: gotgbot.InlineKeyboardMarkup{
				InlineKeyboard: [][]gotgbot.InlineKeyboardButton{
					{
						{
							Text: yesText,
							CallbackData: encodeCallbackData("filters_overwrite", map[string]string{
								"a": "yes",
								"t": token,
							}),
						},
						{
							Text: noText,
							CallbackData: encodeCallbackData("filters_overwrite", map[string]string{
								"a": "cancel",
								"t": token,
							}),
						},
					},
				},
			},
		},
	)
	if err != nil {
		deleteFilterOverwriteCache(token)
		log.Error(err)
		return err
	}
	return ext.EndGroups
}

//nolint:dupl // addFilter shares validation logic with notes module by design
func (m moduleStruct) addFilter(b *gotgbot.Bot, ctx *ext.Context) error {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("[Filters][addFilter] Recovered from panic: %v", r)
		}
	}()
	msg := ctx.EffectiveMessage
	connectedChat := chat_status.IsUserConnected(b, ctx, true, false)
	if connectedChat == nil {
		return ext.EndGroups
	}
	ctx.EffectiveChat = connectedChat
	chat := ctx.EffectiveChat
	user := chat_status.RequireUser(b, ctx)
	if user == nil {
		return ext.EndGroups
	}
	args := ctx.Args()

	if !chat_status.CanUserChangeInfo(b, ctx, chat, user.Id) {
		chat_status.NewPermissionResponder(b).Respond(ctx, "chat_status_change_info_cmd_error", "chat_status_change_info_button_error")
		return ext.EndGroups
	}

	filtersNum := db_filters.CountFilters(chat.Id)
	if filtersNum >= filtersCapMax {
		return replyFilterLimitExceeded(b, ctx)
	}

	if msg.ReplyToMessage != nil && len(args) <= 1 {
		tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
		text, _ := tr.GetString("filters_keyword_required")
		_, err := msg.Reply(b, text, formatting.Shtml())
		if err != nil {
			log.Error(err)
			return err
		}
		return ext.EndGroups
	} else if len(args) <= 2 && msg.ReplyToMessage == nil {
		tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
		text, _ := tr.GetString("filters_invalid")
		_, err := msg.Reply(b, text, formatting.Shtml())
		if err != nil {
			log.Error(err)
			return err
		}
		return ext.EndGroups
	}

	result := content.ExtractNoteAndFilter(msg, true, lang.GetLanguage(ctx))
	filterWord, fileid, text, dataType, buttons, errorMsg := result.KeyWord, result.FileID, result.Text, result.DataType, result.Buttons, result.ErrorMsg
	if dataType == -1 {
		_, err := msg.Reply(b, errorMsg, formatting.Shtml())
		if err != nil {
			log.Error(err)
			return err
		}
		return ext.EndGroups
	}

	filterWord = strings.ToLower(filterWord)

	updateCtx := tracing.UpdateContext(ctx)
	added := applyFilterAdd(updateCtx, chat.Id, filterAddInput{
		Keyword:  filterWord,
		Text:     text,
		FileID:   fileid,
		Buttons:  buttons,
		DataType: dataType,
	})
	switch added.Outcome {
	case filterKeywordTooLong:
		tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
		text, _ := tr.GetString("filters_keyword_too_long")
		_, err := msg.Reply(b, text, formatting.Shtml())
		if err != nil {
			log.Error(err)
			return err
		}
		return ext.EndGroups
	case filterKeywordBlank:
		tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
		text, _ := tr.GetString("filters_keyword_required")
		_, err := msg.Reply(b, text, formatting.Shtml())
		if err != nil {
			log.Error(err)
			return err
		}
		return ext.EndGroups
	case filterCapReached:
		return replyFilterLimitExceeded(b, ctx)
	case filterExists:
		return offerFilterOverwrite(b, ctx, overwriteBase{
			ChatID:   chat.Id,
			UserID:   user.Id,
			ItemName: filterWord,
			Text:     text,
			FileID:   fileid,
			Buttons:  buttons,
			DataType: dataType,
		})
	case filterSaveFailed:
		tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
		errText, _ := tr.GetString("common_settings_save_failed")
		_, _ = msg.Reply(b, errText, formatting.Shtml())
		return ext.EndGroups
	}

	tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
	successText, _ := tr.GetString("filters_added_success")
	_, err := msg.Reply(b, fmt.Sprintf(successText, filterWord), formatting.Shtml())
	if err != nil {
		log.Error(err)
		return err
	}

	return ext.EndGroups
}

func (moduleStruct) rmFilter(b *gotgbot.Bot, ctx *ext.Context) error {
	connectedChat := chat_status.IsUserConnected(b, ctx, true, false)
	if connectedChat == nil {
		return ext.EndGroups
	}
	ctx.EffectiveChat = connectedChat
	chat := ctx.EffectiveChat
	msg := ctx.EffectiveMessage
	user := chat_status.RequireUser(b, ctx)
	if user == nil {
		return ext.EndGroups
	}
	args := ctx.Args()[1:]

	if !chat_status.CanUserChangeInfo(b, ctx, chat, user.Id) {
		chat_status.NewPermissionResponder(b).Respond(ctx, "chat_status_change_info_cmd_error", "chat_status_change_info_button_error")
		return ext.EndGroups
	}

	if len(args) == 0 {
		tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
		text, _ := tr.GetString("filters_remove_keyword_required")
		_, err := msg.Reply(b, text, formatting.Shtml())
		if err != nil {
			log.Error(err)
			return err
		}
	} else {

		filterWord, _ := extraction.ExtractQuotes(strings.Join(args, " "), true, true)

		tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
		if !slices.Contains(db_filters.GetFiltersList(chat.Id), strings.ToLower(filterWord)) {
			text, _ := tr.GetString("filters_not_exists")
			_, err := msg.Reply(b, text, formatting.Shtml())
			if err != nil {
				log.Error(err)
				return err
			}
		} else {
			if removed := applyFilterRemove(tracing.UpdateContext(ctx), chat.Id, filterWord); removed.Outcome == filterRemoveSaveFailed {
				errText, _ := tr.GetString("common_settings_save_failed")
				_, _ = msg.Reply(b, errText, formatting.Shtml())
				return ext.EndGroups
			}
			successText, _ := tr.GetString("filters_removed_success")
			_, err := msg.Reply(b, fmt.Sprintf(successText, filterWord), formatting.Shtml())
			if err != nil {
				log.Error(err)
				return err
			}
		}
	}
	return ext.EndGroups
}

func (moduleStruct) filtersList(b *gotgbot.Bot, ctx *ext.Context) error {
	msg := ctx.EffectiveMessage
	if chat_status.CheckDisabledCmd(b, msg, "filters") {
		return ext.EndGroups
	}
	connectedChat := chat_status.IsUserConnected(b, ctx, false, true)
	if connectedChat == nil {
		return ext.EndGroups
	}
	ctx.EffectiveChat = connectedChat
	chat := ctx.EffectiveChat

	var replyMsgId int64

	if reply := msg.ReplyToMessage; reply != nil {
		replyMsgId = reply.MessageId
	} else {
		replyMsgId = msg.MessageId
	}

	tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
	filterKeys := db_filters.GetFiltersList(chat.Id)
	info, _ := tr.GetString("filters_none_in_chat")
	newFilterKeys := make([]string, 0, len(filterKeys))

	for _, fkey := range filterKeys {
		newFilterKeys = append(newFilterKeys, fmt.Sprintf("<code>%s</code>", html.EscapeString(fkey)))
	}

	if len(newFilterKeys) > 0 {
		info, _ = tr.GetString("filters_current_in_chat")
		info += "\n - " + strings.Join(newFilterKeys, "\n - ")
	}

	_, err := msg.Reply(b,
		info,
		&gotgbot.SendMessageOpts{
			ParseMode: formatting.HTML,
			ReplyParameters: &gotgbot.ReplyParameters{
				MessageId:                replyMsgId,
				AllowSendingWithoutReply: true,
			},
		},
	)
	if err != nil {
		log.Error(err)
		return err
	}

	return ext.EndGroups
}

//nolint:dupl // rmAllFilters shares confirmation pattern with notes module by design
func (moduleStruct) rmAllFilters(b *gotgbot.Bot, ctx *ext.Context) error {
	chat := ctx.EffectiveChat
	user := chat_status.RequireUser(b, ctx)
	if user == nil {
		return ext.EndGroups
	}
	msg := ctx.EffectiveMessage
	filterKeys := db_filters.GetFiltersList(chat.Id)

	if len(filterKeys) == 0 {
		tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
		text, _ := tr.GetString("filters_none_in_chat")
		_, err := msg.Reply(b, text, formatting.Shtml())
		if err != nil {
			log.Error(err)
			return err
		}

		return ext.EndGroups
	}

	if chat_status.RequireUserOwner(b, ctx, chat, user.Id) {
		tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
		confirmText, _ := tr.GetString("filters_clear_all_confirm")
		yesText, _ := tr.GetString("common_yes")
		noText, _ := tr.GetString("common_no")
		_, err := msg.Reply(b, confirmText,
			&gotgbot.SendMessageOpts{
				ReplyMarkup: gotgbot.InlineKeyboardMarkup{
					InlineKeyboard: [][]gotgbot.InlineKeyboardButton{
						{
							{
								Text:         yesText,
								CallbackData: encodeCallbackData("rmAllFilters", map[string]string{"a": "yes"}),
							},
							{
								Text:         noText,
								CallbackData: encodeCallbackData("rmAllFilters", map[string]string{"a": "no"}),
							},
						},
					},
				},
			},
		)
		if err != nil {
			log.Error(err)
			return err
		}
	}

	return ext.EndGroups
}

func (moduleStruct) filtersButtonHandler(b *gotgbot.Bot, ctx *ext.Context) error {
	query, ok := callbackQueryFromContext(ctx)
	if !ok {
		return ext.EndGroups
	}
	user := query.From
	chat := ctx.EffectiveChat
	if chat == nil {
		return ext.EndGroups
	}

	if !chat_status.RequireUserOwner(b, ctx, nil, user.Id) {
		chat_status.NewPermissionResponder(b).Respond(ctx, "chat_status_owner_cmd_error", "chat_status_owner_button_error", chat_status.WithReply())
		return ext.EndGroups
	}

	tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
	response := ""
	if decoded, ok := decodeCallbackData(query.Data, "rmAllFilters"); ok {
		response, _ = decoded.Field("a")
	}
	if response == "" {
		log.Warnf("[Filters] Invalid callback data format: %s", query.Data)
		return answerInvalidCallback(b, ctx, query)
	}
	var helpText string

	switch response {
	case "yes":
		if err := db_filters.RemoveAllFilters(chat.Id); err != nil {
			helpText, _ = tr.GetString("filters_clear_all_failed")
			if helpText == "" {
				helpText = "Failed to remove all Filters from this Chat ❌"
			}
		} else {
			helpText, _ = tr.GetString("filters_clear_all_success")
		}
	case "no":
		helpText, _ = tr.GetString("filters_clear_all_cancelled")
	default:
		return answerInvalidCallback(b, ctx, query)
	}

	if query.Message == nil {
		_, _ = query.Answer(b, &gotgbot.AnswerCallbackQueryOpts{Text: helpText})
		return ext.EndGroups
	}

	_, _, err := query.Message.EditText(b, &gotgbot.EditMessageTextOpts{Text: helpText})
	if err != nil {
		log.Error(err)
		return err
	}

	_, err = query.Answer(b,
		&gotgbot.AnswerCallbackQueryOpts{
			Text: helpText,
		},
	)
	if err != nil {
		log.Error(err)
		return err
	}

	return ext.EndGroups
}

func (m moduleStruct) filterOverWriteHandler(b *gotgbot.Bot, ctx *ext.Context) error {
	query, ok := callbackQueryFromContext(ctx)
	if !ok {
		return ext.EndGroups
	}
	user := query.From
	chat := ctx.EffectiveChat
	if chat == nil {
		return ext.EndGroups
	}

	if !chat_status.RequireUserAdmin(b, ctx, nil, user.Id) {
		chat_status.NewPermissionResponder(b).Respond(ctx, "chat_status_user_admin_cmd_error", "chat_status_user_admin_button_error", chat_status.WithReplyFallback())
		return ext.EndGroups
	}

	tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
	action, token, ok := parseOverwriteCallbackData(query.Data, "filters_overwrite")
	if !ok {
		log.Error("[Filters] Invalid callback data format")
		return ext.EndGroups
	}
	if action != "yes" && action != "cancel" {
		log.WithField("action", action).Warn("[Filters] Invalid overwrite action")
		return answerInvalidCallback(b, ctx, query)
	}
	var helpText string

	if action == "cancel" {
		if token != "" {
			if data, err := consumeFilterOverwriteCache(token); err == nil {
				if data.UserID != 0 && data.UserID != user.Id {
					helpText, _ = tr.GetString("filters_overwrite_expired")
					if query.Message != nil {
						_, _, _ = query.Message.EditText(b, &gotgbot.EditMessageTextOpts{Text: helpText})
					}
					_, _ = query.Answer(b, &gotgbot.AnswerCallbackQueryOpts{Text: helpText})
					return ext.EndGroups
				}
				if data.ChatID != 0 && data.ChatID != chat.Id {
					helpText, _ = tr.GetString("filters_overwrite_expired")
					if query.Message != nil {
						_, _, _ = query.Message.EditText(b, &gotgbot.EditMessageTextOpts{Text: helpText})
					}
					_, _ = query.Answer(b, &gotgbot.AnswerCallbackQueryOpts{Text: helpText})
					return ext.EndGroups
				}
			}
		}
		helpText, _ = tr.GetString("filters_overwrite_cancelled")
		if query.Message != nil {
			_, _, editErr := query.Message.EditText(b, &gotgbot.EditMessageTextOpts{Text: helpText})
			if editErr != nil {
				log.Error(editErr)
			}
		}
		_, answerErr := query.Answer(b, &gotgbot.AnswerCallbackQueryOpts{Text: helpText})
		if answerErr != nil {
			log.Error(answerErr)
		}
		return ext.EndGroups
	}

	filterData, err := consumeFilterOverwriteCache(token)
	if err != nil || filterData == nil {
		log.Debugf("[Filters] Failed to retrieve overwrite data from cache: %v", err)
		helpText, _ = tr.GetString("filters_overwrite_expired")
		if query.Message != nil {
			_, _, editErr := query.Message.EditText(b, &gotgbot.EditMessageTextOpts{Text: helpText})
			if editErr != nil {
				log.Error(editErr)
			}
		}
		_, answerErr := query.Answer(b, &gotgbot.AnswerCallbackQueryOpts{Text: helpText})
		if answerErr != nil {
			log.Error(answerErr)
		}
		return ext.EndGroups
	}
	if (filterData.UserID != 0 && filterData.UserID != user.Id) ||
		(filterData.ChatID != 0 && filterData.ChatID != chat.Id) {
		helpText, _ = tr.GetString("filters_overwrite_expired")
		if query.Message != nil {
			_, _, _ = query.Message.EditText(b, &gotgbot.EditMessageTextOpts{Text: helpText})
		}
		_, _ = query.Answer(b, &gotgbot.AnswerCallbackQueryOpts{Text: helpText})
		return ext.EndGroups
	}

	updated, updateErr := db_filters.UpdateFilter(
		chat.Id,
		filterData.ItemName,
		filterData.Text,
		filterData.FileID,
		filterData.Buttons,
		filterData.DataType,
	)
	if updateErr != nil {
		log.Errorf("[Filters] UpdateFilter failed for chat %d: %v", chat.Id, updateErr)
		helpText, _ = tr.GetString("common_settings_save_failed")
	} else if updated {
		helpText, _ = tr.GetString("filters_overwrite_success")
	} else {
		helpText, _ = tr.GetString("filters_overwrite_cancelled")
	}

	if query.Message == nil {
		_, err = query.Answer(b, &gotgbot.AnswerCallbackQueryOpts{Text: helpText})
		return err
	}
	_, _, err = query.Message.EditText(b, &gotgbot.EditMessageTextOpts{Text: helpText})
	if err != nil {
		log.Error(err)
		return err
	}

	_, err = query.Answer(b,
		&gotgbot.AnswerCallbackQueryOpts{
			Text: helpText,
		},
	)
	if err != nil {
		log.Error(err)
		return err
	}

	return ext.EndGroups
}

func (moduleStruct) filtersWatcher(b *gotgbot.Bot, ctx *ext.Context) error {
	if ctx == nil || ctx.EffectiveSender == nil {
		return ext.ContinueGroups
	}

	chat := ctx.EffectiveChat
	msg := ctx.EffectiveMessage
	user := chat_status.RequireUser(b, ctx)
	if user == nil {
		return ext.ContinueGroups
	}

	// Match against the keyword-only list; filter bodies load only on a hit.
	keywords := db_filters.GetFiltersListContext(tracing.UpdateContext(ctx), chat.Id)
	if len(keywords) == 0 {
		return ext.ContinueGroups
	}

	// Only build the match text once the chat is known to have filters.
	matchText := buildModerationMatchText(msg)
	if matchText == "" {
		return ext.ContinueGroups
	}

	matcher := keyword_matcher.GetNamedCache("filters").GetOrCreateMatcher(chat.Id, keywords)
	firstPattern, found := matcher.FirstMatch(matchText)
	if !found {
		return ext.ContinueGroups
	}

	allFilters, err := db_filters.GetChatFiltersCached(chat.Id)
	if err != nil {
		log.WithField("chatId", chat.Id).WithError(err).Error("Failed to get chat filters")
		return ext.ContinueGroups
	}

	noformatPattern := firstPattern + " noformat"
	noformatMatch := strings.Contains(strings.ToLower(matchText), strings.ToLower(noformatPattern))

	// Keywords are unique per chat (uk_filters_chat_keyword), so the first
	// keyword-identical entry is the one the previous keyword map returned.
	// nil also covers the keyword list and filter bodies briefly disagreeing
	// during a write.
	var filtData *db.ChatFilters
	for _, filter := range allFilters {
		if filter.KeyWord == firstPattern {
			filtData = filter
			break
		}
	}
	if filtData == nil {
		return ext.ContinueGroups
	}

	if noformatMatch {
		if !chat_status.RequireUserAdmin(b, ctx, nil, user.Id) {
			chat_status.NewPermissionResponder(b).Respond(ctx, "chat_status_user_admin_cmd_error", "chat_status_user_admin_button_error", chat_status.WithReplyFallback())
			return ext.EndGroups
		}

		// Copy: filtData aliases the shared read-through cache entry (loader hands
		// one pointer to all coalesced waiters); mutating it races concurrent watchers.
		noformatData := *filtData
		noformatData.FilterReply = formatting.ReverseHTML2MD(noformatData.FilterReply)

		noformatData.FilterReply += content.RevertButtons(noformatData.Buttons)

		var err error
		_, err = media.Send(b, media.Content{
			Text:    noformatData.FilterReply,
			FileID:  noformatData.FileID,
			MsgType: noformatData.MsgType,
			Name:    noformatData.KeyWord,
		}, media.Options{
			ChatID:            ctx.Message.Chat.Id,
			ReplyMsgID:        msg.MessageId,
			ThreadID:          ctx.Message.MessageThreadId,
			Keyboard:          &gotgbot.InlineKeyboardMarkup{InlineKeyboard: nil},
			NoFormat:          true,
			NoNotif:           noformatData.NoNotif,
			AllowWithoutReply: true,
		})
		if err != nil {
			log.Error(err)
			return err
		}

	} else {
		var err error
		_, err = media.SendFilter(b, ctx, filtData, msg.MessageId)
		if err != nil {
			log.Error(err)
			return err
		}
	}

	return ext.ContinueGroups
}

func LoadFilters(dispatcher *ext.Dispatcher) {
	SetModuleEnabled(filtersModule.moduleName, true)

	SetModuleHelp(filtersModule.moduleName, [][]gotgbot.InlineKeyboardButton{
		{
			{
				Text: func() string {
					tr := i18n.MustNewTranslator("uk")
					t, _ := tr.GetString("common_formatting_button")
					return t
				}(),
				CallbackData: encodeCallbackData("helpq", map[string]string{"m": "Formatting"}),
			},
		},
	})
	dispatcher.AddHandler(helpers.NewBotCommand("filter", filtersModule.addFilter))
	dispatcher.AddHandler(helpers.NewBotCommand("addfilter", filtersModule.addFilter))
	dispatcher.AddHandler(helpers.NewBotCommand("stop", filtersModule.rmFilter))
	dispatcher.AddHandler(helpers.NewBotCommand("rmfilter", filtersModule.rmFilter))
	dispatcher.AddHandler(helpers.NewBotCommand("removefilter", filtersModule.rmFilter))
	dispatcher.AddHandler(helpers.NewBotCommand("filters", filtersModule.filtersList))
	helpers.AddCmdToDisableable("filters")
	dispatcher.AddHandler(helpers.NewBotCommand("stopall", filtersModule.rmAllFilters))
	dispatcher.AddHandler(handlers.NewCallback(callbackquery.Prefix("rmAllFilters"), filtersModule.filtersButtonHandler))
	dispatcher.AddHandler(handlers.NewCallback(callbackquery.Prefix("filters_overwrite"), filtersModule.filterOverWriteHandler))
	dispatcher.AddHandlerToGroup(handlers.NewMessage(func(msg *gotgbot.Message) bool {
		return msg.Text != "" || msg.Caption != ""
	}, filtersModule.filtersWatcher), filtersModule.handlerGroup)
}

func init() {
	RegisterLegacyModule("Filters", 140, LoadFilters)
}
