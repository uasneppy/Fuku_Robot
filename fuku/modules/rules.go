package modules

import (
	"context"
	"fmt"
	"html"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"

	tgmd2html "github.com/PaulSonOfLars/gotg_md2html"
	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"

	"github.com/uasneppy/fuku_robot/fuku/db/lang"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/db/rules"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
	"github.com/uasneppy/fuku_robot/fuku/utils/chat_status"
	"github.com/uasneppy/fuku_robot/fuku/utils/formatting"
	"github.com/uasneppy/fuku_robot/fuku/utils/helpers"
	"github.com/uasneppy/fuku_robot/fuku/utils/keyboard"
	"github.com/uasneppy/fuku_robot/fuku/utils/media"
)

var rulesModule = moduleStruct{
	moduleName:      "Rules",
	defaultRulesBtn: "Rules",
}

// buildRulesText is the one builder of the /rules text: the heading with the escaped
// group title, a blank line and the stored rules made safe for HTML, or the no-rules
// line when nothing is stored. /rules and the menu's preview send what it returns.
func buildRulesText(tr *i18n.Translator, title, rawRules string) string {
	normalized := normalizeRulesForHTML(rawRules)
	if normalized == "" {
		text, _ := tr.GetString("rules_no_rules_set")
		return text
	}
	header, _ := tr.GetString("rules_for_chat_header")
	return fmt.Sprintf(header, html.EscapeString(title)) + "\n\n" + normalized
}

// rulesForm is the wording of a rules message: the heading and the no-rules line differ between what
// /rules shows in a chat and what the private-rules message (the deep link) shows.
type rulesForm int

const (
	// rulesFormGroup is /rules: the heading rules_for_chat_header and the line rules_no_rules_set.
	rulesFormGroup rulesForm = iota
	// rulesFormPrivate is the private-rules message: the heading rules_for_chat and the line rules_not_set.
	rulesFormPrivate
)

// noRulesKey is the locale key of the line said when the rules hold nothing.
func (f rulesForm) noRulesKey() string {
	if f == rulesFormPrivate {
		return "rules_not_set"
	}
	return "rules_no_rules_set"
}

// text is the heading with the escaped group title and the stored rules made safe for HTML. With no text
// (a rules message of media or buttons only) it is the heading alone.
func (f rulesForm) text(tr *i18n.Translator, title, rawRules string) string {
	body := normalizeRulesForHTML(rawRules)
	if f == rulesFormPrivate {
		text, _ := tr.GetString("rules_for_chat", i18n.TranslationParams{
			"first":  html.EscapeString(title),
			"second": body,
		})
		if body == "" {
			text = strings.TrimRight(text, "\n")
		}
		return text
	}
	if body != "" {
		return buildRulesText(tr, title, rawRules)
	}
	header, _ := tr.GetString("rules_for_chat_header")
	return fmt.Sprintf(header, html.EscapeString(title))
}

// rulesMessage is a rules message ready to send: the text (or the caption of the media), the media, whether
// the media is shown under the text, the rules' URL buttons and whether the rules hold nothing at all.
type rulesMessage struct {
	Text     string
	FileID   string
	MsgType  int
	Below    bool
	Keyboard *gotgbot.InlineKeyboardMarkup
	Empty    bool
}

// buildRulesMessage is the one builder of a member-facing rules message. Rules are set when the row has a text,
// a media or a button (RulesSettings.HasContent); a row with none of them is Empty and carries the form's
// no-rules line. The keyboard exists only when the row has buttons.
func buildRulesMessage(tr *i18n.Translator, title string, row *models.RulesSettings, form rulesForm) rulesMessage {
	if !row.HasContent() {
		text, _ := tr.GetString(form.noRulesKey())
		return rulesMessage{Text: text, Empty: true}
	}
	m := rulesMessage{
		Text:    form.text(tr, title, row.Rules),
		FileID:  row.FileID,
		MsgType: row.MediaType,
		Below:   row.MediaBelow,
	}
	if len(row.Buttons) > 0 {
		m.Keyboard = &gotgbot.InlineKeyboardMarkup{InlineKeyboard: keyboard.BuildKeyboard(row.Buttons)}
	}
	return m
}

// sendRulesMessage is the one sender of a member-facing rules message. Without media it is the caller's own
// SendMessage call, with its options exactly as they were (textOpts), and only the rules keyboard added when
// the rules have buttons, so a rules row saved before media existed makes the request it always made. It is
// never routed through the greeting sender, which turns link previews off and drops the reply target. With media
// it is one media message: the text as its caption, the buttons under it, and the caption above the media when
// Media under text is on and the kind allows it.
func sendRulesMessage(b *gotgbot.Bot, chatID int64, m rulesMessage, textOpts *gotgbot.SendMessageOpts, degrade bool) error {
	if m.FileID == "" {
		return sendRulesText(b, chatID, m, textOpts)
	}
	var (
		replyMsgID        int64
		allowWithoutReply bool
	)
	if textOpts != nil && textOpts.ReplyParameters != nil {
		replyMsgID = textOpts.ReplyParameters.MessageId
		allowWithoutReply = textOpts.ReplyParameters.AllowSendingWithoutReply
	}
	_, err := media.Send(b, media.Content{
		Text:         m.Text,
		FileID:       m.FileID,
		MsgType:      m.MsgType,
		CaptionAbove: m.Below && media.CaptionAboveEligible(m.MsgType),
		Name:         "rules",
	}, media.Options{
		ChatID:            chatID,
		ReplyMsgID:        replyMsgID,
		AllowWithoutReply: allowWithoutReply,
		Keyboard:          m.Keyboard,
	})
	_ = degrade
	return err
}

// sendRulesText sends the rules as a text message with the caller's options. The rules keyboard replaces the
// options' ReplyMarkup only when the rules have buttons.
func sendRulesText(b *gotgbot.Bot, chatID int64, m rulesMessage, textOpts *gotgbot.SendMessageOpts) error {
	var opts gotgbot.SendMessageOpts
	if textOpts != nil {
		opts = *textOpts
	}
	if m.Keyboard != nil {
		opts.ReplyMarkup = *m.Keyboard
	}
	_, err := b.SendMessage(chatID, m.Text, &opts)
	return err
}

// rulesApplyResult is what the rules apply functions return.
type rulesApplyResult struct {
	Outcome greetingApplyOutcome
	Rules   string
	Err     error
}

// rulesWrites is the rules repository write the apply functions call. It is a variable
// only so a test can make the write fail.
var rulesWrites = struct {
	SetChatRules func(chatID int64, rules string) error
}{rules.SetChatRules}

// applyRulesText stores the rules text (the HTML /setrules and the editor keep) and reads
// it back. It is the one write path of /setrules and the menu.
func applyRulesText(ctx context.Context, chatID int64, html string) rulesApplyResult {
	return finishRulesApply(ctx, chatID, "set", rulesWrites.SetChatRules(chatID, html))
}

// applyRulesClear stores empty rules, the one write path of /clearrules and the menu's
// Remove.
func applyRulesClear(ctx context.Context, chatID int64) rulesApplyResult {
	return finishRulesApply(ctx, chatID, "clear", rulesWrites.SetChatRules(chatID, ""))
}

// finishRulesApply turns the error of the rules write into the apply result. A failed
// read-back is only logged, because the write succeeded.
func finishRulesApply(ctx context.Context, chatID int64, what string, writeErr error) rulesApplyResult {
	stored, outcome, err := applyReadBack(ctx, writeErr,
		func(e error) { log.Errorf("[Rules] Failed to %s rules for chat %d: %v", what, chatID, e) },
		func(e error) { log.Warnf("[Rules] read back rules of chat %d after %s: %v", chatID, what, e) },
		func(c context.Context) (string, error) { return rules.LoadRulesContext(c, chatID) })
	return rulesApplyResult{Outcome: outcome, Rules: stored, Err: err}
}

//nolint:dupl // clearRules has similar structure to resetRulesBtn
func (moduleStruct) clearRules(bot *gotgbot.Bot, ctx *ext.Context) error {
	msg := ctx.EffectiveMessage
	connectedChat := chat_status.IsUserConnected(bot, ctx, true, true)
	if connectedChat == nil {
		return ext.EndGroups
	}
	chat := connectedChat

	tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
	var text string
	applyCtx, cancel := greetingApplyContext()
	res := applyRulesClear(applyCtx, chat.Id)
	cancel()
	if res.Outcome == greetingSaveFailed {
		text, _ = tr.GetString("common_settings_save_failed")
	} else {
		text, _ = tr.GetString("rules_cleared_successfully")
	}
	_, err := msg.Reply(bot, text, formatting.Shtml())
	if err != nil {
		log.Error(err)
		return err
	}

	return ext.EndGroups
}

func (moduleStruct) privaterules(bot *gotgbot.Bot, ctx *ext.Context) error {
	msg := ctx.EffectiveMessage
	connectedChat := chat_status.IsUserConnected(bot, ctx, true, true)
	if connectedChat == nil {
		return ext.EndGroups
	}
	chat := connectedChat
	args := ctx.Args()
	tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
	var text string

	if len(args) >= 2 {
		switch strings.ToLower(args[1]) {
		case "on", "yes", "true":
			if err := rules.SetPrivateRules(chat.Id, true); err != nil {
				log.Errorf("[Rules] Failed to enable private rules for chat %d: %v", chat.Id, err)
				text, _ = tr.GetString("common_settings_save_failed")
			} else {
				text, _ = tr.GetString("rules_private_pm_usage")
			}
		case "off", "no", "false":
			if err := rules.SetPrivateRules(chat.Id, false); err != nil {
				log.Errorf("[Rules] Failed to disable private rules for chat %d: %v", chat.Id, err)
				text, _ = tr.GetString("common_settings_save_failed")
			} else {
				temp, _ := tr.GetString("rules_private_group_usage")
				text = fmt.Sprintf(temp, html.EscapeString(chat.Title))
			}
		default:
			text, _ = tr.GetString("pins_input_not_recognized")
		}
	} else {
		rulesprefs := rules.GetChatRulesInfo(chat.Id)
		if rulesprefs.Private {
			text, _ = tr.GetString("rules_private_current_pm")
		} else {
			temp2, _ := tr.GetString("rules_private_current_group")
			text = fmt.Sprintf(temp2, html.EscapeString(chat.Title))
		}
	}

	_, err := msg.Reply(bot, text, formatting.Shtml())
	if err != nil {
		log.Error(err)
		return err
	}

	return ext.EndGroups
}

func (m moduleStruct) sendRules(bot *gotgbot.Bot, ctx *ext.Context) error {
	msg := ctx.EffectiveMessage
	if chat_status.CheckDisabledCmd(bot, msg, "rules") {
		return ext.EndGroups
	}
	connectedChat := chat_status.IsUserConnected(bot, ctx, false, true)
	if connectedChat == nil {
		return ext.EndGroups
	}
	chat := connectedChat
	var (
		replyMsgId int64
		rulesBtn   string
	)

	if reply := msg.ReplyToMessage; reply != nil {
		replyMsgId = reply.MessageId
	} else {
		replyMsgId = msg.MessageId
	}

	row := rules.GetChatRulesInfo(chat.Id)
	rulesBtn = row.RulesBtn
	if rulesBtn == "" {
		rulesBtn = m.defaultRulesBtn
	}
	tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
	opts := &gotgbot.SendMessageOpts{
		ReplyParameters: &gotgbot.ReplyParameters{
			MessageId:                replyMsgId,
			AllowSendingWithoutReply: true,
		},
		ParseMode: formatting.HTML,
	}

	if chat_status.RequireGroup(bot, ctx, nil) && row.Private {
		text, _ := tr.GetString("rules_click_for_rules")
		opts.ReplyMarkup = gotgbot.InlineKeyboardMarkup{
			InlineKeyboard: [][]gotgbot.InlineKeyboardButton{
				{
					{
						Text: rulesBtn,
						Url:  fmt.Sprintf("t.me/%s?start=rules_%d", bot.Username, chat.Id),
					},
				},
			},
		}
		if _, err := msg.Reply(bot, text, opts); err != nil {
			log.Error(err)
			return err
		}
		return ext.EndGroups
	}

	// The zero-value markup is what /rules has always sent for a rules message with no buttons.
	opts.ReplyMarkup = gotgbot.InlineKeyboardMarkup{}
	rm := buildRulesMessage(tr, chat.Title, row, rulesFormGroup)
	if err := sendRulesMessage(bot, msg.Chat.Id, rm, opts, true); err != nil {
		log.Error(err)
		return err
	}

	return ext.EndGroups
}

func (moduleStruct) setRules(bot *gotgbot.Bot, ctx *ext.Context) error {
	msg := ctx.EffectiveMessage
	connectedChat := chat_status.IsUserConnected(bot, ctx, true, true)
	if connectedChat == nil {
		return ext.EndGroups
	}
	chat := connectedChat
	args := ctx.Args()
	tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
	var text string

	if len(args) == 1 && msg.ReplyToMessage == nil {
		text, _ = tr.GetString("rules_need_text")
	} else {
		if msg.ReplyToMessage != nil {
			text = msg.ReplyToMessage.OriginalMDV2()
		} else {
			parts := strings.SplitN(msg.OriginalMDV2(), " ", 2)
			if len(parts) >= 2 {
				text = parts[1]
			} else {
				text, _ = tr.GetString("rules_need_text")
				_, err := msg.Reply(bot, text, formatting.Shtml())
				if err != nil {
					log.Error(err)
					return err
				}
				return ext.EndGroups
			}
		}
		applyCtx, cancel := greetingApplyContext()
		res := applyRulesText(applyCtx, chat.Id, tgmd2html.MD2HTMLV2(text))
		cancel()
		if res.Outcome == greetingSaveFailed {
			text, _ = tr.GetString("common_settings_save_failed")
		} else {
			text, _ = tr.GetString("rules_set_successfully")
		}
	}

	_, err := msg.Reply(bot, text, formatting.Shtml())
	if err != nil {
		log.Error(err)
		return err
	}

	return ext.EndGroups
}

func (m moduleStruct) rulesBtn(bot *gotgbot.Bot, ctx *ext.Context) error {
	msg := ctx.EffectiveMessage
	connectedChat := chat_status.IsUserConnected(bot, ctx, true, true)
	if connectedChat == nil {
		return ext.EndGroups
	}
	chat := connectedChat
	user := chat_status.RequireUser(bot, ctx)
	if user == nil {
		return ext.EndGroups
	}
	args := ctx.Args()
	var err error
	var text string

	if !chat_status.IsUserAdmin(bot, chat.Id, user.Id) {
		return ext.EndGroups
	}

	tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
	if len(args) >= 2 {
		rulesBtnCustomText := strings.Join(args[1:], " ")
		if len([]rune(rulesBtnCustomText)) > 30 {
			text, _ = tr.GetString("rules_button_too_long")
		} else {
			if err := rules.SetChatRulesButton(chat.Id, rulesBtnCustomText); err != nil {
				log.Errorf("[Rules] Failed to set rules button for chat %d: %v", chat.Id, err)
				text, _ = tr.GetString("common_settings_save_failed")
			} else {
				temp3, _ := tr.GetString("rules_button_set_successfully")
				text = fmt.Sprintf(temp3, html.EscapeString(rulesBtnCustomText))
			}
		}
	} else {
		customRulesBtn := rules.GetChatRulesInfo(chat.Id).RulesBtn
		if customRulesBtn == "" {
			temp4, _ := tr.GetString("rules_button_not_set")
			text = fmt.Sprintf(temp4, m.defaultRulesBtn)
		} else {
			temp5, _ := tr.GetString("rules_button_current")
			text = fmt.Sprintf(temp5, customRulesBtn)
		}
	}

	_, err = msg.Reply(bot, text, formatting.Shtml())
	if err != nil {
		log.Error(err)
		return err
	}

	return ext.EndGroups
}

//nolint:dupl // resetRulesBtn has similar structure to clearRules
func (moduleStruct) resetRulesBtn(bot *gotgbot.Bot, ctx *ext.Context) error {
	msg := ctx.EffectiveMessage
	connectedChat := chat_status.IsUserConnected(bot, ctx, true, true)
	if connectedChat == nil {
		return ext.EndGroups
	}
	chat := connectedChat

	tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
	var text string
	if err := rules.SetChatRulesButton(chat.Id, ""); err != nil {
		log.Errorf("[Rules] Failed to clear rules button for chat %d: %v", chat.Id, err)
		text, _ = tr.GetString("common_settings_save_failed")
	} else {
		text, _ = tr.GetString("rules_button_cleared")
	}
	_, err := msg.Reply(bot, text, formatting.Shtml())
	if err != nil {
		log.Error(err)
		return err
	}

	return ext.EndGroups
}

func LoadRules(dispatcher *ext.Dispatcher) {
	SetModuleEnabled(rulesModule.moduleName, true)

	dispatcher.AddHandler(handlers.NewCommand("rules", rulesModule.sendRules))
	helpers.AddCmdToDisableable("rules")
	dispatcher.AddHandler(handlers.NewCommand("setrules", rulesModule.setRules))
	helpers.MultiCommand(dispatcher, []string{"resetrules", "clearrules"}, rulesModule.clearRules)
	dispatcher.AddHandler(handlers.NewCommand("privaterules", rulesModule.privaterules))
	dispatcher.AddHandler(handlers.NewCommand("rulesbutton", rulesModule.rulesBtn))
	dispatcher.AddHandler(handlers.NewCommand("rulesbtn", rulesModule.rulesBtn))
	dispatcher.AddHandler(handlers.NewCommand("clearrulesbutton", rulesModule.resetRulesBtn))
	dispatcher.AddHandler(handlers.NewCommand("clearrulesbtn", rulesModule.resetRulesBtn))
	dispatcher.AddHandler(handlers.NewCommand("resetrulesbutton", rulesModule.resetRulesBtn))
	dispatcher.AddHandler(handlers.NewCommand("resetrulesbtn", rulesModule.resetRulesBtn))
}

func init() {
	RegisterLegacyModule("Rules", 190, LoadRules)
	RegisterDeepLinkHandler("rules_", rulesDeepLinkHandler)
}

func rulesDeepLinkHandler(b *gotgbot.Bot, ctx *ext.Context, user *gotgbot.User, arg string) error {
	msg := ctx.EffectiveMessage

	parts := strings.Split(arg, "_")
	if len(parts) < 2 {
		tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
		text, _ := tr.GetString("helpers_invalid_deep_link")
		_, _ = msg.Reply(b, text, formatting.Shtml())
		return ext.EndGroups
	}

	chatID, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
		text, _ := tr.GetString("helpers_invalid_deep_link")
		_, _ = msg.Reply(b, text, formatting.Shtml())
		return ext.EndGroups
	}

	chatinfo, err := b.GetChat(chatID, nil)
	if err != nil || chatinfo == nil {
		tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
		text, _ := tr.GetString("helpers_chat_not_found")
		_, _ = msg.Reply(b, text, formatting.Shtml())
		return ext.EndGroups
	}

	_chat := chatinfo.ToChat()
	if !chat_status.IsUserInChat(b, &_chat, user.Id) {
		tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
		text, _ := tr.GetString("helpers_chat_not_found")
		_, _ = msg.Reply(b, text, formatting.Shtml())
		return ext.EndGroups
	}

	rulesrc := rules.GetChatRulesInfo(chatID)
	normalizedRules := normalizeRulesForHTML(rulesrc.Rules)

	if normalizedRules == "" {
		tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
		text, _ := tr.GetString("rules_not_set")
		_, err := msg.Reply(b, text, formatting.Shtml())
		if err != nil {
			log.Error(err)
			return err
		}
		return ext.EndGroups
	}

	tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
	text, _ := tr.GetString("rules_for_chat", i18n.TranslationParams{
		"first":  html.EscapeString(chatinfo.Title),
		"second": normalizedRules,
	})
	_, err = msg.Reply(b, text, formatting.Shtml())
	if err != nil {
		log.Error(err)
		return err
	}
	return ext.EndGroups
}
