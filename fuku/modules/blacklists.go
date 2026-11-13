package modules

import (
	"context"
	"errors"
	"fmt"
	"html"
	"slices"
	"strings"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers/filters/callbackquery"
	log "github.com/sirupsen/logrus"

	"github.com/uasneppy/fuku_robot/fuku/db/blacklists"
	"github.com/uasneppy/fuku_robot/fuku/db/deletion"
	"github.com/uasneppy/fuku_robot/fuku/db/lang"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
	"github.com/uasneppy/fuku_robot/fuku/utils/chat_status"
	"github.com/uasneppy/fuku_robot/fuku/utils/formatting"
	"github.com/uasneppy/fuku_robot/fuku/utils/helpers"
	"github.com/uasneppy/fuku_robot/fuku/utils/keyword_matcher"
	"github.com/uasneppy/fuku_robot/fuku/utils/tracing"
)

var blacklistsModule = moduleStruct{
	moduleName:   "Blacklists",
	handlerGroup: 7,
}

// blacklistWordMaxRunes is the longest blacklist word /addblacklist accepts, in code points.
const blacklistWordMaxRunes = 100

// blacklistWordPreviewRunes is how many code points of a too-long word its refusal shows.
const blacklistWordPreviewRunes = 20

// blacklistActionValues are the actions /blacklistaction accepts, and so the actions the
// menu may write.
var blacklistActionValues = []string{"mute", "kick", "warn", "ban", "none"}

// blacklistWriteSeams are the repository writes the apply functions call, a variable so a
// test can make one fail.
type blacklistWriteSeams struct {
	Add    func(ctx context.Context, chatID int64, word, action string) error
	Remove func(ctx context.Context, chatID int64, word string) error
	// RemoveByID deletes one row by ID only when it belongs to the chat and returns its word.
	RemoveByID func(ctx context.Context, chatID, id int64) (word string, removed bool, err error)
	SetAction  func(ctx context.Context, chatID int64, action string) (int64, error)
}

// blacklistWrites is the one place the blacklist commands and the settings menu reach the
// repository's writes through.
var blacklistWrites = blacklistWriteSeams{
	Add:        blacklists.AddBlacklistContext,
	Remove:     blacklists.RemoveBlacklistContext,
	RemoveByID: blacklists.RemoveBlacklistByID,
	SetAction:  blacklists.SetBlacklistActionContext,
}

// blacklistAddOutcome is what applyBlacklistAdd did overall; its zero value is applied.
type blacklistAddOutcome int

const (
	// blacklistAdded means at least one word was stored.
	blacklistAdded blacklistAddOutcome = iota
	// blacklistNothingAdded means every word was already listed or refused.
	blacklistNothingAdded
	// blacklistAddSaveFailed means no word was stored and at least one write failed.
	blacklistAddSaveFailed
)

// blacklistAddResult is what applyBlacklistAdd returns for the caller to word.
type blacklistAddResult struct {
	Outcome blacklistAddOutcome
	// Added are the stored words, lower-cased and raw (the caller escapes them).
	Added []string
	// Already are the words that were already listed, lower-cased.
	Already []string
	// TooLong are the previews of the words over the length limit, as the command shows them.
	TooLong []string
	// Failed counts the words whose write failed.
	Failed int
}

// blacklistRemoveResult is what applyBlacklistRemove returns.
type blacklistRemoveResult struct {
	// Removed are the words that were deleted, lower-cased and in the order given.
	Removed []string
	// Failed counts the words whose delete failed.
	Failed int
}

// blacklistRemoveByIDOutcome is what applyBlacklistRemoveByID did; its zero value is removed.
type blacklistRemoveByIDOutcome int

const (
	// blacklistRemovedByID means the chat's row was deleted.
	blacklistRemovedByID blacklistRemoveByIDOutcome = iota
	// blacklistRemoveGone means no row of this chat has the ID: already removed, never
	// existed or another chat's row, which are all one answer.
	blacklistRemoveGone
	// blacklistRemoveByIDSaveFailed means the delete failed.
	blacklistRemoveByIDSaveFailed
)

// blacklistRemoveByIDResult is what applyBlacklistRemoveByID returns.
type blacklistRemoveByIDResult struct {
	Outcome blacklistRemoveByIDOutcome
	// Word is the stored word of the deleted row, for the caller's notice.
	Word string
	// Err is the repository error of a failed delete, for the caller's own handling.
	Err error
}

// blacklistActionOutcome is what applyBlacklistAction did; its zero value is applied.
type blacklistActionOutcome int

const (
	// blacklistActionApplied means every word of the chat now has the action.
	blacklistActionApplied blacklistActionOutcome = iota
	// blacklistActionNoWords means the update succeeded and matched no row because the chat
	// has no words; the command still answers its success text (research C-02).
	blacklistActionNoWords
	// blacklistActionUnknown means the action is not one of blacklistActionValues; nothing
	// was written.
	blacklistActionUnknown
	// blacklistActionSaveFailed means the update failed.
	blacklistActionSaveFailed
)

// blacklistActionResult is what applyBlacklistAction returns.
type blacklistActionResult struct {
	Outcome blacklistActionOutcome
	// Rows is how many words the update matched.
	Rows int64
	// Err is the repository error of a failed update, for the caller's own handling.
	Err error
}

// blacklistSortWords splits typed words into those within the length limit and the previews
// of those over it (the first 20 code points and an ellipsis when longer than that).
func blacklistSortWords(words []string) (valid, tooLong []string) {
	valid = make([]string, 0, len(words))
	for _, word := range words {
		runes := []rune(word)
		if len(runes) <= blacklistWordMaxRunes {
			valid = append(valid, word)
			continue
		}
		preview := word
		if len(runes) > blacklistWordPreviewRunes {
			preview = string(runes[:blacklistWordPreviewRunes]) + "..."
		}
		tooLong = append(tooLong, preview)
	}
	return valid, tooLong
}

// applyBlacklistAdd stores the typed words for the chat, each lower-cased with the warn
// action, and reports which were added, already listed, too long or failed. existing is the
// caller's read of the chat's current words. It judges no person's rights and logs the chat
// ID and the error only, never a word.
func applyBlacklistAdd(ctx context.Context, chatID int64, words []string, existing models.BlacklistSettingsSlice) blacklistAddResult {
	valid, tooLong := blacklistSortWords(words)
	result := blacklistAddResult{TooLong: tooLong}
	seen := make(map[string]struct{}, len(existing)+len(valid))
	for _, word := range existing.Triggers() {
		seen[word] = struct{}{}
	}
	for _, word := range valid {
		word = strings.ToLower(word)
		if _, exists := seen[word]; exists {
			result.Already = append(result.Already, word)
			continue
		}
		if err := blacklistWrites.Add(ctx, chatID, word, "warn"); err != nil {
			log.WithFields(log.Fields{
				"chatId": chatID,
				"error":  err,
			}).Error("[Blacklists] Failed to add a blacklist word")
			result.Failed++
			continue
		}
		seen[word] = struct{}{}
		result.Added = append(result.Added, word)
	}
	switch {
	case len(result.Added) > 0:
		result.Outcome = blacklistAdded
	case result.Failed > 0:
		result.Outcome = blacklistAddSaveFailed
	default:
		result.Outcome = blacklistNothingAdded
	}
	return result
}

// applyBlacklistRemove deletes each typed word that is in existing (the caller's read of
// the chat's words) and reports the ones that went and how many deletes failed. It logs the
// chat ID and the error only, never a word.
func applyBlacklistRemove(ctx context.Context, chatID int64, words, existing []string) blacklistRemoveResult {
	var result blacklistRemoveResult
	for _, word := range words {
		word = strings.ToLower(word)
		if !slices.Contains(existing, word) {
			continue
		}
		if err := blacklistWrites.Remove(ctx, chatID, word); err != nil {
			log.WithFields(log.Fields{
				"chatId": chatID,
				"error":  err,
			}).Error("[Blacklists] Failed to remove a blacklist word")
			result.Failed++
			continue
		}
		result.Removed = append(result.Removed, word)
	}
	return result
}

// applyBlacklistRemoveByID deletes one word by its row ID, only when the row belongs to the
// chat, so an ID of another chat deletes nothing. It judges no person's rights and logs the
// chat ID and the error only, never the word.
func applyBlacklistRemoveByID(ctx context.Context, chatID, id int64) blacklistRemoveByIDResult {
	word, removed, err := blacklistWrites.RemoveByID(ctx, chatID, id)
	if err != nil {
		log.WithFields(log.Fields{
			"chatId": chatID,
			"error":  err,
		}).Error("[Blacklists] Failed to remove a blacklist word by ID")
		return blacklistRemoveByIDResult{Outcome: blacklistRemoveByIDSaveFailed, Err: err}
	}
	if !removed {
		return blacklistRemoveByIDResult{Outcome: blacklistRemoveGone}
	}
	return blacklistRemoveByIDResult{Outcome: blacklistRemovedByID, Word: word}
}

// applyBlacklistAction sets the action of every word of the chat. The caller lower-cases the
// action; one outside blacklistActionValues writes nothing. A chat with no words is
// blacklistActionNoWords, which /blacklistaction answers like a success. It logs the chat ID
// and the error only.
func applyBlacklistAction(ctx context.Context, chatID int64, action string) blacklistActionResult {
	if !slices.Contains(blacklistActionValues, action) {
		return blacklistActionResult{Outcome: blacklistActionUnknown}
	}
	rows, err := blacklistWrites.SetAction(ctx, chatID, action)
	if err != nil {
		log.WithFields(log.Fields{
			"chatId": chatID,
			"error":  err,
		}).Error("[Blacklists] Failed to persist blacklist action")
		return blacklistActionResult{Outcome: blacklistActionSaveFailed, Err: err}
	}
	if rows == 0 {
		return blacklistActionResult{Outcome: blacklistActionNoWords}
	}
	return blacklistActionResult{Outcome: blacklistActionApplied, Rows: rows}
}

func (m moduleStruct) addBlacklist(b *gotgbot.Bot, ctx *ext.Context) error {
	msg := ctx.EffectiveMessage
	connectedChat := chat_status.IsUserConnected(b, ctx, true, true)
	if connectedChat == nil {
		return ext.EndGroups
	}
	chat := connectedChat
	user := chat_status.RequireUser(b, ctx)
	if user == nil {
		return ext.EndGroups
	}
	args := ctx.Args()[1:]
	tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))

	var text string

	if !chat_status.IsUserAdmin(b, chat.Id, user.Id) {
		return ext.EndGroups
	}
	if !chat_status.IsBotAdmin(b, ctx, chat) {
		return ext.EndGroups
	}
	if !chat_status.CanUserRestrict(b, ctx, chat, user.Id) {
		chat_status.NewPermissionResponder(b).Respond(ctx, "chat_status_restrict_cmd_error", "chat_status_restrict_button_error")
		return ext.EndGroups
	}
	if !chat_status.CanBotRestrict(b, ctx, chat) {
		chat_status.NewPermissionResponder(b).Respond(ctx, "chat_status_bot_restrict_group_error", "chat_status_bot_restrict_error")
		return ext.EndGroups
	}

	if len(args) == 0 {
		text, _ := tr.GetString(strings.ToLower(m.moduleName) + "_blacklist_give_bl_word")
		_, err := msg.Reply(b, text, formatting.Shtml())
		if err != nil {
			log.Error(err)
			return err
		}
		return ext.EndGroups
	} else if len(args) >= 1 {
		updateCtx := tracing.UpdateContext(ctx)
		existing := blacklists.GetBlacklistSettingsContext(updateCtx, chat.Id)
		result := applyBlacklistAdd(updateCtx, chat.Id, args, existing)

		if len(result.TooLong) > 0 {
			text, _ := tr.GetString(strings.ToLower(m.moduleName) + "_blacklist_word_too_long")
			_, err := msg.Reply(b, fmt.Sprintf(text, strings.Join(result.TooLong, ", ")), formatting.Shtml())
			if err != nil {
				log.Error(err)
			}
		}
		if len(result.Added) == 0 && len(result.Already) == 0 && result.Failed == 0 {
			return ext.EndGroups
		}

		if len(result.Already) >= 1 {
			temp, _ := tr.GetString(strings.ToLower(m.moduleName) + "_blacklist_already_blacklisted")
			text += temp + fmt.Sprintf("\n - %s\n\n", strings.Join(result.Already, "\n - "))
		}
		if len(result.Added) >= 1 {
			newBlacklist := make([]string, 0, len(result.Added))
			for _, word := range result.Added {
				newBlacklist = append(newBlacklist, fmt.Sprintf("<code>%s</code>", html.EscapeString(word)))
			}
			temp, _ := tr.GetString(strings.ToLower(m.moduleName) + "_blacklist_added_bl")
			text += temp + fmt.Sprintf("\n - %s\n\n", strings.Join(newBlacklist, "\n - "))
		}
		if result.Failed > 0 {
			temp, _ := tr.GetString("common_settings_save_failed")
			text += temp
		}

		_, err := msg.Reply(b, text, formatting.Shtml())
		if err != nil {
			log.Error(err)
			return err
		}
	}

	return ext.EndGroups
}

func (m moduleStruct) removeBlacklist(b *gotgbot.Bot, ctx *ext.Context) error {
	msg := ctx.EffectiveMessage
	connectedChat := chat_status.IsUserConnected(b, ctx, true, true)
	if connectedChat == nil {
		return ext.EndGroups
	}
	chat := connectedChat
	user := chat_status.RequireUser(b, ctx)
	if user == nil {
		return ext.EndGroups
	}
	args := ctx.Args()[1:]
	tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))

	if !chat_status.IsUserAdmin(b, chat.Id, user.Id) {
		return ext.EndGroups
	}
	if !chat_status.IsBotAdmin(b, ctx, chat) {
		return ext.EndGroups
	}
	if !chat_status.CanUserRestrict(b, ctx, chat, user.Id) {
		chat_status.NewPermissionResponder(b).Respond(ctx, "chat_status_restrict_cmd_error", "chat_status_restrict_button_error")
		return ext.EndGroups
	}
	if !chat_status.CanBotRestrict(b, ctx, chat) {
		chat_status.NewPermissionResponder(b).Respond(ctx, "chat_status_bot_restrict_group_error", "chat_status_bot_restrict_error")
		return ext.EndGroups
	}

	if len(args) == 0 {
		text, _ := tr.GetString(strings.ToLower(m.moduleName) + "_unblacklist_give_bl_word")
		_, err := msg.Reply(b, text, formatting.Shtml())
		if err != nil {
			log.Error(err)
			return err
		}
		return ext.EndGroups
	} else {
		updateCtx := tracing.UpdateContext(ctx)
		allBlWords := blacklists.GetBlacklistSettingsContext(updateCtx, chat.Id).Triggers()
		removedBlacklists := applyBlacklistRemove(updateCtx, chat.Id, args, allBlWords).Removed
		if len(removedBlacklists) <= 0 {
			text, _ := tr.GetString(strings.ToLower(m.moduleName) + "_unblacklist_no_removed_bl")
			_, err := msg.Reply(b, text, nil)
			if err != nil {
				log.Error(err)
				return err
			}
		} else {
			temp, _ := tr.GetString(strings.ToLower(m.moduleName) + "_unblacklist_removed_bl")
			_, err := msg.Reply(b, fmt.Sprintf(temp, strings.Join(removedBlacklists, ", ")), nil)
			if err != nil {
				log.Error(err)
				return err
			}
		}
	}
	return ext.EndGroups
}

func (m moduleStruct) listBlacklists(b *gotgbot.Bot, ctx *ext.Context) error {
	tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
	msg := ctx.EffectiveMessage
	if chat_status.CheckDisabledCmd(b, msg, "blacklists") {
		return ext.EndGroups
	}
	connectedChat := chat_status.IsUserConnected(b, ctx, false, true)
	if connectedChat == nil {
		return ext.EndGroups
	}
	chat := connectedChat

	var (
		replyMsgId     int64
		blacklistsText string
	)

	if reply := msg.ReplyToMessage; reply != nil {
		replyMsgId = reply.MessageId
	} else {
		replyMsgId = msg.MessageId
	}

	blSrc := blacklists.GetBlacklistSettingsContext(tracing.UpdateContext(ctx), chat.Id)
	triggers := blSrc.Triggers()
	slices.Sort(triggers)
	var sb strings.Builder
	for _, i := range triggers {
		fmt.Fprintf(&sb, "\n - <code>%s</code>", html.EscapeString(i))
	}
	blacklistsText += sb.String()

	if blacklistsText != "" {
		temp, _ := tr.GetString(strings.ToLower(m.moduleName) + "_ls_bl_list_bl")
		blacklistsText = temp + blacklistsText
	} else {
		blacklistsText, _ = tr.GetString(strings.ToLower(m.moduleName) + "_ls_bl_no_blacklisted")
	}

	_, err := msg.Reply(b,
		blacklistsText,
		&gotgbot.SendMessageOpts{
			ReplyParameters: &gotgbot.ReplyParameters{
				MessageId:                replyMsgId,
				AllowSendingWithoutReply: true,
			},
			ParseMode: formatting.HTML,
		},
	)
	if err != nil {
		log.Error(err)
		return err
	}

	return ext.EndGroups
}

func (m moduleStruct) setBlacklistAction(b *gotgbot.Bot, ctx *ext.Context) error {
	msg := ctx.EffectiveMessage
	connectedChat := chat_status.IsUserConnected(b, ctx, true, true)
	if connectedChat == nil {
		return ext.EndGroups
	}
	chat := connectedChat
	user := chat_status.RequireUser(b, ctx)
	if user == nil {
		return ext.EndGroups
	}
	args := ctx.Args()[1:]
	tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))

	var rMsg string

	if !chat_status.CanUserRestrict(b, ctx, chat, user.Id) {
		chat_status.NewPermissionResponder(b).Respond(ctx, "chat_status_restrict_cmd_error", "chat_status_restrict_button_error")
		return ext.EndGroups
	}
	if !chat_status.CanBotRestrict(b, ctx, chat) {
		chat_status.NewPermissionResponder(b).Respond(ctx, "chat_status_bot_restrict_group_error", "chat_status_bot_restrict_error")
		return ext.EndGroups
	}

	if len(args) == 0 {
		currAction := blacklists.GetBlacklistSettingsContext(tracing.UpdateContext(ctx), chat.Id).Action()
		temp, _ := tr.GetString(strings.ToLower(m.moduleName) + "_set_bl_action_current_mode")
		rMsg = fmt.Sprintf(temp, currAction)
	} else if len(args) == 1 {
		action := strings.ToLower(args[0])
		result := applyBlacklistAction(tracing.UpdateContext(ctx), chat.Id, action)
		switch result.Outcome {
		case blacklistActionUnknown:
			rMsg, _ = tr.GetString(strings.ToLower(m.moduleName) + "_set_bl_action_choose_correct_option")
		case blacklistActionSaveFailed:
			rMsg, _ = tr.GetString("blacklists_set_bl_action_update_failed")
		default:
			temp, _ := tr.GetString(strings.ToLower(m.moduleName) + "_set_bl_action_changed_mode")
			rMsg = fmt.Sprintf(temp, action)
		}
	} else {
		rMsg, _ = tr.GetString(strings.ToLower(m.moduleName) + "_set_bl_action_choose_correct_option")
	}
	_, err := msg.Reply(b, rMsg, formatting.Smarkdown())
	if err != nil {
		log.Error(err)
		return err
	}
	return ext.EndGroups
}

func (m moduleStruct) rmAllBlacklists(b *gotgbot.Bot, ctx *ext.Context) error {
	chat := ctx.EffectiveChat
	user := chat_status.RequireUser(b, ctx)
	if user == nil {
		return ext.EndGroups
	}
	msg := ctx.EffectiveMessage
	tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))

	if !chat_status.RequireGroup(b, ctx, nil) {
		chat_status.NewPermissionResponder(b).Respond(ctx, "chat_status_group_only_error", "", chat_status.WithReply())
		return ext.EndGroups
	}
	if !chat_status.RequireUserOwner(b, ctx, chat, user.Id) {
		chat_status.NewPermissionResponder(b).Respond(ctx, "chat_status_owner_cmd_error", "chat_status_owner_button_error", chat_status.WithReply())
		return ext.EndGroups
	}

	text, _ := tr.GetString(strings.ToLower(m.moduleName) + "_rm_all_bl_ask")
	yesText, _ := tr.GetString("button_yes")
	noText, _ := tr.GetString("button_no")
	_, err := msg.Reply(b, text,
		&gotgbot.SendMessageOpts{
			ReplyMarkup: gotgbot.InlineKeyboardMarkup{
				InlineKeyboard: [][]gotgbot.InlineKeyboardButton{
					{
						{
							Text:         yesText,
							CallbackData: encodeCallbackData("rmAllBlacklist", map[string]string{"a": "yes"}),
						},
						{
							Text:         noText,
							CallbackData: encodeCallbackData("rmAllBlacklist", map[string]string{"a": "no"}),
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

	return ext.EndGroups
}

func (m moduleStruct) buttonHandler(b *gotgbot.Bot, ctx *ext.Context) error {
	query, ok := callbackQueryFromContext(ctx)
	if !ok {
		return ext.EndGroups
	}
	user := query.From
	tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
	if query.Message == nil {
		text, _ := tr.GetString("common_callback_message_unavailable")
		_, _ = query.Answer(b, &gotgbot.AnswerCallbackQueryOpts{Text: text})
		return ext.EndGroups
	}

	if !chat_status.RequireUserOwner(b, ctx, nil, user.Id) {
		chat_status.NewPermissionResponder(b).Respond(ctx, "chat_status_owner_cmd_error", "chat_status_owner_button_error", chat_status.WithReply())
		return ext.EndGroups
	}

	creatorAction := ""
	if decoded, ok := decodeCallbackData(query.Data, "rmAllBlacklist"); ok {
		creatorAction, _ = decoded.Field("a")
	}
	if creatorAction == "" {
		log.Warnf("[Blacklists] Invalid callback data format: %s", query.Data)
		return answerInvalidCallback(b, ctx, query)
	}
	var helpText string

	switch creatorAction {
	case "yes":
		chatID := query.Message.GetChat().Id
		if err := blacklists.RemoveAllBlacklist(chatID); err != nil {
			log.WithFields(log.Fields{
				"chatId": chatID,
				"error":  err,
			}).Error("Failed to remove all blacklists")
			helpText, _ = tr.GetString("common_settings_save_failed")
			break
		}
		helpText, _ = tr.GetString(strings.ToLower(m.moduleName) + "_rm_all_bl_button_handler_yes")
	case "no":
		helpText, _ = tr.GetString(strings.ToLower(m.moduleName) + "_rm_all_bl_button_handler_no")
	default:
		return answerInvalidCallback(b, ctx, query)
	}

	_, _, err := query.Message.EditText(b, &gotgbot.EditMessageTextOpts{Text: helpText, ParseMode: formatting.HTML})
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

func (m moduleStruct) blacklistWatcher(b *gotgbot.Bot, ctx *ext.Context) error {
	chat := ctx.EffectiveChat
	user := ctx.EffectiveSender
	if user == nil {
		return ext.ContinueGroups
	}
	if user.IsAnonymousAdmin() {
		return ext.ContinueGroups
	}

	blSettings := blacklists.GetBlacklistSettingsContext(tracing.UpdateContext(ctx), chat.Id)
	triggers := blSettings.Triggers()
	if len(triggers) == 0 {
		return ext.ContinueGroups
	}

	msg := ctx.EffectiveMessage
	matchText := buildModerationMatchText(msg)
	if matchText == "" {
		return ext.ContinueGroups
	}

	cache := keyword_matcher.GetNamedCache("blacklists")
	matcher := cache.GetOrCreateMatcher(chat.Id, triggers)

	firstPattern, found := matcher.FirstMatch(matchText)
	if !found {
		return ext.ContinueGroups
	}
	i := firstPattern
	matched := blSettings.Find(i)
	if matched == nil {
		return ext.ContinueGroups
	}

	if !user.IsAnonymousChannel() && user.IsUser() && user.Id() > 0 && chat_status.IsUserAdminForUpdate(b, ctx, chat.Id, user.Id()) {
		return ext.ContinueGroups
	}
	if !user.IsAnonymousChannel() && user.IsUser() && user.Id() > 0 && chat_status.IsApprovedForUpdate(b, ctx, chat.Id, user.Id()) {
		return ext.ContinueGroups
	}

	if !chat_status.IsBotAdmin(b, ctx, chat) {
		return ext.ContinueGroups
	}

	tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
	reason := matched.Reason
	if reason == "" {
		reason = "Blacklisted word: '%s'"
	}

	_ = helpers.DeleteMessageWithErrorHandling(b, chat.Id, msg.MessageId)
	var err error
	switch matched.Action {
	case "mute":
		if user.IsAnonymousChannel() {
			return ext.ContinueGroups
		}

		_, err = b.RestrictChatMember(chat.Id, user.Id(), MutedPermissions, nil)
		if err != nil {
			log.Error(err)
			return err
		}

		var sent *gotgbot.Message
		sent, err = b.SendMessage(chat.Id,
			func() string {
				temp, _ := tr.GetString(strings.ToLower(m.moduleName) + "_bl_watcher_muted_user")
				return fmt.Sprintf(temp, formatting.MentionHtml(user.Id(), user.Name()), fmt.Sprintf(reason, i))
			}(),
			formatting.Shtml())
		if err != nil {
			log.Error(err)
			return err
		}
		scheduleBotMessage(sent, deletion.KindBotAuto)
	case "ban":
		if user.IsAnonymousChannel() {
			_, err = b.BanChatSenderChat(chat.Id, user.Id(), nil)
		} else {
			_, err = b.BanChatMember(chat.Id, user.Id(), nil)
		}
		if err != nil {
			log.Error(err)
			return err
		}

		var sent *gotgbot.Message
		sent, err = b.SendMessage(chat.Id,
			func() string {
				temp, _ := tr.GetString(strings.ToLower(m.moduleName) + "_bl_watcher_banned_user")
				return fmt.Sprintf(temp, formatting.MentionHtml(user.Id(), user.Name()), fmt.Sprintf(reason, i))
			}(),
			formatting.Shtml())
		if err != nil {
			log.Error(err)
			return err
		}
		scheduleBotMessage(sent, deletion.KindBotAuto)
	case "kick":
		if user.IsAnonymousChannel() {
			return ext.ContinueGroups
		}

		if err = kickMember(b, chat.Id, user.Id()); err != nil {
			log.Error(err)
			return err
		}

		var sent *gotgbot.Message
		sent, err = b.SendMessage(chat.Id,
			func() string {
				temp, _ := tr.GetString(strings.ToLower(m.moduleName) + "_bl_watcher_kicked_user")
				return fmt.Sprintf(temp, formatting.MentionHtml(user.Id(), user.Name()), fmt.Sprintf(reason, i))
			}(),
			formatting.Shtml())
		if err != nil {
			log.Error(err)
			return err
		}
		scheduleBotMessage(sent, deletion.KindBotAuto)
	case "warn":
		if user.IsAnonymousChannel() {
			return ext.ContinueGroups
		}

		err = warnsModule.warnThisUser(b, ctx, user.Id(), fmt.Sprintf(reason, i), "warn", warnSourceAuto)
		if err != nil {
			if !errors.Is(err, ext.EndGroups) && !errors.Is(err, ext.ContinueGroups) {
				log.Error(err)
			}
			return err
		}
	case "none":
		return ext.ContinueGroups
	}

	return ext.ContinueGroups
}

func LoadBlacklists(dispatcher *ext.Dispatcher) {
	SetModuleEnabled(blacklistsModule.moduleName, true)

	dispatcher.AddHandler(helpers.NewBotCommand("blacklists", blacklistsModule.listBlacklists))
	helpers.AddCmdToDisableable("blacklists")
	dispatcher.AddHandler(helpers.NewBotCommand("blocklists", blacklistsModule.listBlacklists))
	helpers.AddCmdToDisableable("blocklists")
	dispatcher.AddHandler(helpers.NewBotCommand("addblacklist", blacklistsModule.addBlacklist))
	dispatcher.AddHandler(helpers.NewBotCommand("blacklist", blacklistsModule.addBlacklist))
	dispatcher.AddHandler(helpers.NewBotCommand("addblocklist", blacklistsModule.addBlacklist))
	dispatcher.AddHandler(helpers.NewBotCommand("blocklist", blacklistsModule.addBlacklist))
	dispatcher.AddHandler(helpers.NewBotCommand("rmblacklist", blacklistsModule.removeBlacklist))
	dispatcher.AddHandler(helpers.NewBotCommand("rmblocklist", blacklistsModule.removeBlacklist))
	dispatcher.AddHandler(helpers.NewBotCommand("unblacklist", blacklistsModule.removeBlacklist))
	dispatcher.AddHandler(helpers.NewBotCommand("blaction", blacklistsModule.setBlacklistAction))
	dispatcher.AddHandler(helpers.NewBotCommand("blacklistaction", blacklistsModule.setBlacklistAction))
	helpers.MultiCommand(dispatcher, []string{"remallbl", "rmallbl", "rmblocklistall", "rmallblocklist"}, blacklistsModule.rmAllBlacklists)
	dispatcher.AddHandler(handlers.NewCallback(callbackquery.Prefix("rmAllBlacklist"), blacklistsModule.buttonHandler))
	dispatcher.AddHandlerToGroup(handlers.NewMessage(func(msg *gotgbot.Message) bool {
		return msg.Text != "" || msg.Caption != ""
	}, blacklistsModule.blacklistWatcher), blacklistsModule.handlerGroup)
}

func init() {
	RegisterLegacyModule("Blacklists", 240, LoadBlacklists)
}
