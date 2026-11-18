package modules

import (
	"context"
	"fmt"
	"html"
	"slices"
	"strings"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers/filters/callbackquery"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers/filters/message"
	log "github.com/sirupsen/logrus"

	"github.com/uasneppy/fuku_robot/fuku/db/lang"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/db/reactions"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
	"github.com/uasneppy/fuku_robot/fuku/utils/chat_status"
	"github.com/uasneppy/fuku_robot/fuku/utils/formatting"
	"github.com/uasneppy/fuku_robot/fuku/utils/helpers"
	"github.com/uasneppy/fuku_robot/fuku/utils/tracing"
)

var reactionsModule = moduleStruct{
	moduleName:   "Reactions",
	handlerGroup: 8,
}

var supportedReactionEmoji = []string{
	"❤", "👍", "👎", "🔥", "🥰", "👏", "😁", "🤔", "🤯", "😱", "🤬", "😢",
	"🎉", "🤩", "🤮", "💩", "🙏", "👌", "🕊", "🤡", "🥱", "🥴", "😍", "🐳",
	"❤‍🔥", "🌚", "🌭", "💯", "🤣", "⚡", "🍌", "🏆", "💔", "🤨", "😐", "🍓",
	"🍾", "💋", "🖕", "😈", "😴", "😭", "🤓", "👻", "👨‍💻", "👀", "🎃", "🙈",
	"😇", "😨", "🤝", "✍", "🤗", "🫡", "🎅", "🎄", "☃", "💅", "🤪", "🗿",
	"🆒", "💘", "🙉", "🦄", "😘", "💊", "🙊", "😎", "👾", "🤷‍♂", "🤷",
	"🤷‍♀", "😡",
}

func LoadReactions(dispatcher *ext.Dispatcher) {
	dispatcher.AddHandler(helpers.NewBotCommand("addreaction", reactionsModule.addReaction))
	dispatcher.AddHandler(helpers.NewBotCommand("removereaction", reactionsModule.removeReaction))
	dispatcher.AddHandler(helpers.NewBotCommand("reactions", reactionsModule.listReactions))
	dispatcher.AddHandler(helpers.NewBotCommand("resetreactions", reactionsModule.resetReactions))
	dispatcher.AddHandler(handlers.NewCallback(callbackquery.Prefix("reactions_help"), reactionsModule.reactionsHelpHandler))

	dispatcher.AddHandlerToGroup(handlers.NewMessage(message.All, reactionsModule.checkReactions), reactionsModule.handlerGroup)

	SetModuleEnabled(reactionsModule.moduleName, true)

	DefaultHelpRegistry().AltHelpOptions["Reactions"] = []string{"reaction"}
	SetModuleHelpFunc(reactionsModule.moduleName, reactionsHelpKb)

	log.Info("[Modules] Reactions module loaded")
}

func (m moduleStruct) reactionsHelpHandler(b *gotgbot.Bot, ctx *ext.Context) error {
	query, ok := callbackQueryFromContext(ctx)
	if !ok {
		return ext.EndGroups
	}
	if query == nil {
		return ext.EndGroups
	}

	action := ""
	if decoded, ok := decodeCallbackData(query.Data, "reactions_help"); ok {
		action, _ = decoded.Field("action")
	}
	tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
	if action == "" {
		return answerInvalidCallback(b, ctx, query)
	}

	var helpText string
	switch action {
	case "add":
		helpText, _ = tr.GetString("reactions_add_usage")
	case "remove":
		helpText, _ = tr.GetString("reactions_remove_usage")
	default:
		return answerInvalidCallback(b, ctx, query)
	}

	if query.Message == nil {
		_, _ = query.Answer(b, &gotgbot.AnswerCallbackQueryOpts{Text: helpText})
		return ext.EndGroups
	}

	backText, _ := tr.GetString("common_back")
	_, _, err := query.Message.EditText(b, &gotgbot.EditMessageTextOpts{Text: helpText, ParseMode: formatting.HTML,
		ReplyMarkup: gotgbot.InlineKeyboardMarkup{
			InlineKeyboard: [][]gotgbot.InlineKeyboardButton{
				{
					{
						Text:         backText,
						CallbackData: encodeCallbackData("helpq", map[string]string{"m": "Reactions"}),
					},
				},
			},
		}})
	if err != nil {
		log.Error(err)
		return err
	}

	_, err = query.Answer(b, nil)
	if err != nil {
		log.Error(err)
		return err
	}

	return ext.EndGroups
}

func (m moduleStruct) addReaction(b *gotgbot.Bot, ctx *ext.Context) error {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("[Reactions][addReaction] Recovered from panic: %v", r)
		}
	}()

	msg := ctx.EffectiveMessage
	chat := ctx.EffectiveChat
	user := chat_status.RequireUser(b, ctx)
	if user == nil {
		return ext.EndGroups
	}

	if !chat_status.CanUserChangeInfo(b, ctx, chat, user.Id) {
		chat_status.NewPermissionResponder(b).Respond(ctx, "chat_status_change_info_cmd_error", "chat_status_change_info_button_error")
		return ext.EndGroups
	}

	args := ctx.Args()
	if len(args) < 3 {
		tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
		text, _ := tr.GetString("reactions_add_usage")
		_, err := msg.Reply(b, text, formatting.Shtml())
		if err != nil {
			log.Error(err)
			return err
		}
		return ext.EndGroups
	}

	keyword := strings.ToLower(strings.TrimSpace(args[1]))
	result := applyReactionAdd(tracing.UpdateContext(ctx), chat.Id, keyword, args[2], true)
	tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
	switch result.Outcome {
	case reactionBadEmoji:
		// ReactionTypeEmoji accepts only Telegram's documented reaction set.
		text, _ := tr.GetString("reactions_invalid_emoji")
		_, err := msg.Reply(b, text, formatting.Shtml())
		if err != nil {
			log.Error(err)
			return err
		}
		return ext.EndGroups
	case reactionAdded:
	default:
		log.Errorf("[Reactions] Failed to save reaction: %v", result.Err)
		text, _ := tr.GetString("reactions_add_error")
		_, _ = msg.Reply(b, text, formatting.Shtml())
		return ext.EndGroups
	}

	text, _ := tr.GetString("reactions_add_success", i18n.TranslationParams{
		"keyword": html.EscapeString(keyword),
		"emoji":   html.EscapeString(result.Emoji),
	})
	_, err := msg.Reply(b, text, formatting.Shtml())
	if err != nil {
		log.Error(err)
		return err
	}

	return ext.EndGroups
}

func (m moduleStruct) removeReaction(b *gotgbot.Bot, ctx *ext.Context) error {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("[Reactions][removeReaction] Recovered from panic: %v", r)
		}
	}()

	msg := ctx.EffectiveMessage
	chat := ctx.EffectiveChat
	user := chat_status.RequireUser(b, ctx)
	if user == nil {
		return ext.EndGroups
	}

	if !chat_status.CanUserChangeInfo(b, ctx, chat, user.Id) {
		chat_status.NewPermissionResponder(b).Respond(ctx, "chat_status_change_info_cmd_error", "chat_status_change_info_button_error")
		return ext.EndGroups
	}

	args := ctx.Args()
	if len(args) < 2 {
		tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
		text, _ := tr.GetString("reactions_remove_usage")
		_, err := msg.Reply(b, text, formatting.Shtml())
		if err != nil {
			log.Error(err)
			return err
		}
		return ext.EndGroups
	}

	keyword := strings.ToLower(strings.TrimSpace(args[1]))
	result := applyReactionRemove(tracing.UpdateContext(ctx), chat.Id, keyword)
	tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
	switch result.Outcome {
	case reactionRemoveAbsent:
		text, _ := tr.GetString("reactions_keyword_not_found", i18n.TranslationParams{
			"keyword": html.EscapeString(keyword),
		})
		_, _ = msg.Reply(b, text, formatting.Shtml())
		return ext.EndGroups
	case reactionRemoveFailed:
		log.Errorf("[Reactions] Failed to update reactions: %v", result.Err)
		text, _ := tr.GetString("reactions_remove_error")
		_, _ = msg.Reply(b, text, formatting.Shtml())
		return ext.EndGroups
	}

	text, _ := tr.GetString("reactions_remove_success", i18n.TranslationParams{
		"keyword": html.EscapeString(keyword),
	})
	_, err := msg.Reply(b, text, formatting.Shtml())
	if err != nil {
		log.Error(err)
		return err
	}

	return ext.EndGroups
}

func (m moduleStruct) listReactions(b *gotgbot.Bot, ctx *ext.Context) error {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("[Reactions][listReactions] Recovered from panic: %v", r)
		}
	}()

	msg := ctx.EffectiveMessage
	chat := ctx.EffectiveChat

	reactionsMap := reactions.GetReactionsContext(tracing.UpdateContext(ctx), chat.Id)
	if len(reactionsMap) == 0 {
		tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
		text, _ := tr.GetString("reactions_none")
		_, _ = msg.Reply(b, text, formatting.Shtml())
		return ext.EndGroups
	}

	var sb strings.Builder
	for keyword, emoji := range reactionsMap {
		fmt.Fprintf(&sb, "• %s → %s\n", html.EscapeString(keyword), html.EscapeString(emoji))
	}

	tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
	text, _ := tr.GetString("reactions_list_header", i18n.TranslationParams{
		"list": sb.String(),
	})
	_, err := msg.Reply(b, text, formatting.Shtml())
	if err != nil {
		log.Error(err)
		return err
	}

	return ext.EndGroups
}

func (m moduleStruct) resetReactions(b *gotgbot.Bot, ctx *ext.Context) error {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("[Reactions][resetReactions] Recovered from panic: %v", r)
		}
	}()

	msg := ctx.EffectiveMessage
	chat := ctx.EffectiveChat
	user := chat_status.RequireUser(b, ctx)
	if user == nil {
		return ext.EndGroups
	}

	if !chat_status.CanUserChangeInfo(b, ctx, chat, user.Id) {
		chat_status.NewPermissionResponder(b).Respond(ctx, "chat_status_change_info_cmd_error", "chat_status_change_info_button_error")
		return ext.EndGroups
	}

	if err := reactions.ResetReactions(chat.Id); err != nil {
		log.Errorf("[Reactions] Failed to reset reactions: %v", err)
		tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
		text, _ := tr.GetString("reactions_remove_error")
		_, _ = msg.Reply(b, text, formatting.Shtml())
		return ext.EndGroups
	}

	tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
	text, _ := tr.GetString("reactions_reset_success")
	_, err := msg.Reply(b, text, formatting.Shtml())
	if err != nil {
		log.Error(err)
		return err
	}

	return ext.EndGroups
}

func (m moduleStruct) checkReactions(b *gotgbot.Bot, ctx *ext.Context) error {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("[Reactions][checkReactions] Recovered from panic: %v", r)
		}
	}()

	msg := ctx.EffectiveMessage
	if msg == nil || msg.Text == "" {
		return ext.ContinueGroups
	}

	chat := ctx.EffectiveChat
	if chat == nil {
		return ext.ContinueGroups
	}

	reactionsMap := reactions.GetReactionsContext(tracing.UpdateContext(ctx), chat.Id)
	if len(reactionsMap) == 0 {
		return ext.ContinueGroups
	}

	lowerText := strings.ToLower(msg.Text)
	var matchedKeywords []string
	for keyword := range reactionsMap {
		if strings.Contains(lowerText, keyword) {
			matchedKeywords = append(matchedKeywords, keyword)
		}
	}
	if len(matchedKeywords) == 0 {
		return ext.ContinueGroups
	}
	slices.Sort(matchedKeywords)
	emoji := reactionsMap[matchedKeywords[0]]

	_, err := b.SetMessageReaction(
		chat.Id,
		msg.MessageId,
		&gotgbot.SetMessageReactionOpts{
			Reaction: []gotgbot.ReactionType{
				gotgbot.ReactionTypeEmoji{
					Emoji: emoji,
				},
			},
		},
	)
	if err != nil {
		log.Debugf("[Reactions] Failed to set reaction: %v", err)
	}

	return ext.ContinueGroups
}

func init() {
	RegisterLegacyModule("Reactions", 250, LoadReactions)
}

// reactionsHelpKb is the Reactions help keyboard in the reader's language.
func reactionsHelpKb(tr *i18n.Translator) [][]gotgbot.InlineKeyboardButton {
	return [][]gotgbot.InlineKeyboardButton{{
		{
			Text:         trS(tr, "reactions_button_add"),
			CallbackData: encodeCallbackData("reactions_help", map[string]string{"action": "add"}),
		},
		{
			Text:         trS(tr, "reactions_button_remove"),
			CallbackData: encodeCallbackData("reactions_help", map[string]string{"action": "remove"}),
		},
	}}
}

// reactionWrites is the write and read seam of the reaction apply functions, a variable only so a test can fail it.
var reactionWrites = struct {
	Add        func(ctx context.Context, chatID int64, keyword, emoji string, overwrite bool) (bool, error)
	Remove     func(chatID int64, keyword string) error
	RemoveByID func(ctx context.Context, chatID, id int64) (*models.Reactions, error)
	Lookup     func(ctx context.Context, chatID int64) map[string]string
	Exists     func(ctx context.Context, chatID int64, keyword string) (bool, error)
	Count      func(ctx context.Context, chatID int64) (int64, error)
}{
	Add:        reactions.AddReactionContext,
	Remove:     reactions.RemoveReaction,
	RemoveByID: reactions.RemoveReactionByID,
	Lookup:     reactions.GetReactionsContext,
	Exists:     reactions.ReactionExistsFresh,
	Count:      reactions.CountReactionsFresh,
}

// reactionAddOutcome is what applyReactionAdd did; its zero value is added.
type reactionAddOutcome int

const (
	// reactionAdded means the reaction was stored.
	reactionAdded reactionAddOutcome = iota
	// reactionBadEmoji means the emoji is not one of Telegram's reactions; nothing was written.
	reactionBadEmoji
	// reactionExists means the keyword already has a reaction and the caller asked not to overwrite it.
	reactionExists
	// reactionSaveFailed means a read or the write failed; nothing is known to be stored.
	reactionSaveFailed
)

// reactionAddResult is the typed result of applyReactionAdd: the outcome, the stored emoji (U+FE0F stripped)
// and the error of a failed save.
type reactionAddResult struct {
	Outcome reactionAddOutcome
	Emoji   string
	Err     error
}

// normaliseReactionEmoji is the emoji as the reaction is stored: trimmed, with every U+FE0F stripped.
func normaliseReactionEmoji(raw string) string {
	return strings.ReplaceAll(strings.TrimSpace(raw), "\ufe0f", "")
}

// applyReactionAdd stores one reaction of a chat: the one write path of /addreaction and of the Reactions page.
// The emoji must be one of Telegram's reactions (supportedReactionEmoji, after U+FE0F is stripped). With
// overwrite (the command) an existing keyword gets the new emoji; without it (the page) an existing keyword is
// refused and never overwritten. It judges no person's rights and sends nothing.
func applyReactionAdd(ctx context.Context, chatID int64, keyword, rawEmoji string, overwrite bool) reactionAddResult {
	emoji := normaliseReactionEmoji(rawEmoji)
	if !slices.Contains(supportedReactionEmoji, emoji) {
		return reactionAddResult{Outcome: reactionBadEmoji}
	}
	added, err := reactionWrites.Add(ctx, chatID, keyword, emoji, overwrite)
	if err != nil {
		return reactionAddResult{Outcome: reactionSaveFailed, Emoji: emoji, Err: err}
	}
	if !added {
		return reactionAddResult{Outcome: reactionExists, Emoji: emoji}
	}
	return reactionAddResult{Outcome: reactionAdded, Emoji: emoji}
}

// reactionRemoveOutcome is what applyReactionRemove and applyReactionRemoveByID did; its zero value is removed.
type reactionRemoveOutcome int

const (
	// reactionRemoved means the chat's row was deleted.
	reactionRemoved reactionRemoveOutcome = iota
	// reactionRemoveAbsent means the chat has no such reaction; nothing was written.
	reactionRemoveAbsent
	// reactionRemoveFailed means the delete failed.
	reactionRemoveFailed
)

// reactionRemoveResult is the typed result of a reaction removal: the outcome, the removed row's keyword and
// emoji (by ID only) and the error of a failed delete.
type reactionRemoveResult struct {
	Outcome reactionRemoveOutcome
	Keyword string
	Emoji   string
	Err     error
}

// applyReactionRemove removes a chat's reaction by keyword, the write /removereaction makes: an absent keyword
// (read through the cached map, as the command always has) writes nothing.
func applyReactionRemove(ctx context.Context, chatID int64, keyword string) reactionRemoveResult {
	if _, exists := reactionWrites.Lookup(ctx, chatID)[keyword]; !exists {
		return reactionRemoveResult{Outcome: reactionRemoveAbsent, Keyword: keyword}
	}
	if err := reactionWrites.Remove(chatID, keyword); err != nil {
		return reactionRemoveResult{Outcome: reactionRemoveFailed, Keyword: keyword, Err: err}
	}
	return reactionRemoveResult{Outcome: reactionRemoved, Keyword: keyword}
}

// applyReactionRemoveByID removes one reaction by its row ID, only when it belongs to the chat (the page's
// delete): one conditional DELETE and the cache drop when a row went.
func applyReactionRemoveByID(ctx context.Context, chatID, id int64) reactionRemoveResult {
	row, err := reactionWrites.RemoveByID(ctx, chatID, id)
	if err != nil {
		log.Errorf("[Reactions] Failed to delete a reaction of chat %d: %v", chatID, err)
		return reactionRemoveResult{Outcome: reactionRemoveFailed, Err: err}
	}
	if row == nil {
		return reactionRemoveResult{Outcome: reactionRemoveAbsent}
	}
	return reactionRemoveResult{Outcome: reactionRemoved, Keyword: row.Keyword, Emoji: row.Emoji}
}
