package modules

import (
	"context"
	"strconv"
	"strings"

	"github.com/PaulSonOfLars/gotgbot/v2"
	log "github.com/sirupsen/logrus"

	"github.com/uasneppy/fuku_robot/fuku/i18n"
	"github.com/uasneppy/fuku_robot/fuku/utils/formatting"
)

// The member requirements notice: one message per episode, in the group's language, that
// names the member and every broken rule. The member's name is a token spliced in after
// translation (memberReqMention escapes it), so it never passes through the translator and a
// name such as "{rules}" is printed literally.

// memberReqNameToken stands for the member's mention in a template until it is spliced.
const memberReqNameToken = "<<mreq-name>>"

// memberReqRulesSeparator joins the rule phrases of a notice; all 8 languages use the comma.
const memberReqRulesSeparator = ", "

// memberReqText translates key, logging the error and returning "" for a missing key, as the
// other modules' helpers do. The text may hold the name token but never a name.
func memberReqText(tr *i18n.Translator, key string, params ...i18n.TranslationParams) string {
	text, err := tr.GetString(key, params...)
	if err != nil {
		log.Errorf("[MemberReq] translate %s: %v", key, err)
	}
	return text
}

// memberReqRulesText lists the rules in the broken mask as their phrases in rule order,
// joined by a comma and a space.
func memberReqRulesText(tr *i18n.Translator, broken int) string {
	phrases := make([]string, 0, len(memberReqRules))
	for _, rule := range memberReqRules {
		if broken&rule.Bit != 0 {
			phrases = append(phrases, memberReqText(tr, rule.PhraseKey))
		}
	}
	return strings.Join(phrases, memberReqRulesSeparator)
}

// memberReqNoticeParams are the template parameters of a notice. The name is the token; the
// numbers are the warn count and limit when a warn was recorded.
func memberReqNoticeParams(tr *i18n.Translator, broken int, warn *warnApplyResult) i18n.TranslationParams {
	params := i18n.TranslationParams{
		"name":  memberReqNameToken,
		"rules": memberReqRulesText(tr, broken),
	}
	if warn != nil {
		params["n"] = strconv.Itoa(warn.Count)
		params["limit"] = strconv.Itoa(warn.Limit)
	}
	return params
}

// memberReqNoticeText builds the notice of a punishment: the template of the punishment
// translated with the broken rules and the warn numbers, then the member's mention spliced in.
func memberReqNoticeText(tr *i18n.Translator, user gotgbot.User, broken, action int, warn *warnApplyResult) string {
	text := memberReqText(tr, "mreq_notice_warn", memberReqNoticeParams(tr, broken, warn))
	fallback := memberReqText(tr, "mreq_name_unknown", i18n.TranslationParams{"id": strconv.FormatInt(user.Id, 10)})
	return lockdownSplice(text, memberReqNameToken, memberReqMention(user, fallback))
}

// memberReqPostNotice sends the notice as a new message (never a reply: the member's message
// may be deleted) in HTML with link previews off, through the requirement pacer, and returns
// its message ID. A keyboard with no rows sends none.
func memberReqPostNotice(ctx context.Context, b *gotgbot.Bot, chatID int64, text string, keyboard gotgbot.InlineKeyboardMarkup) (int64, error) {
	opts := &gotgbot.SendMessageOpts{
		ParseMode:          formatting.HTML,
		LinkPreviewOptions: &gotgbot.LinkPreviewOptions{IsDisabled: true},
	}
	if len(keyboard.InlineKeyboard) > 0 {
		opts.ReplyMarkup = keyboard
	}
	var sent *gotgbot.Message
	err := memberReqPaced(ctx, func(callCtx context.Context) error {
		var sendErr error
		sent, sendErr = b.SendMessageWithContext(callCtx, chatID, text, opts)
		return sendErr
	})
	if err != nil {
		return 0, err
	}
	return sent.MessageId, nil
}
