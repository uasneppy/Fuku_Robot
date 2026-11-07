package modules

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	log "github.com/sirupsen/logrus"

	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
	"github.com/uasneppy/fuku_robot/fuku/utils/formatting"
)

// The member requirements notice: one message per episode, in the group's language, that
// names the member and every broken rule. The member's name is a token spliced in after
// translation (memberReqMention escapes it), so it never passes through the translator and a
// name such as "{rules}" is printed literally.

// memberReqCallbackNamespace is the namespace of the "I fixed it" button (mreq|v1|...). The
// handler that answers it comes with the plan that turns enforcement on.
const memberReqCallbackNamespace = "mreq"

// memberReqNamePlaceholder stands for the member's mention in a template until it is spliced.
const memberReqNamePlaceholder = "<<mreq-name>>"

// memberReqRulesSeparator joins the rule phrases of a notice; all 8 languages use the comma.
const memberReqRulesSeparator = ", "

// memberReqNoticeRateLimitWait is a compile-only stand-in until the notice retry lands.
var memberReqNoticeRateLimitWait = 2 * time.Second

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
		"name":  memberReqNamePlaceholder,
		"rules": memberReqRulesText(tr, broken),
	}
	if warn != nil {
		params["n"] = strconv.Itoa(warn.Count)
		params["limit"] = strconv.Itoa(warn.Limit)
	}
	return params
}

// memberReqNoticeKey chooses the template of a punishment: kick, ban and mute have their own;
// a warn is the plain warn notice, or the variant of the group's warn mode when the limit
// action took effect (a refused limit action is the plain notice, so a removal that did not
// happen is never claimed).
func memberReqNoticeKey(action int, warn *warnApplyResult) string {
	switch action {
	case models.RequirementActionKick:
		return "mreq_notice_kick"
	case models.RequirementActionBan:
		return "mreq_notice_ban"
	case models.RequirementActionMute:
		return "mreq_notice_mute"
	}
	if warn != nil && warn.LimitReached && warn.LimitApplied {
		switch warn.Mode {
		case "kick", "ban", "mute":
			return "mreq_notice_warn_limit_" + warn.Mode
		}
	}
	return "mreq_notice_warn"
}

// memberReqNoticeText builds the notice of a punishment: the template of the punishment
// translated with the broken rules and the warn numbers, then the member's mention spliced in.
func memberReqNoticeText(tr *i18n.Translator, user gotgbot.User, broken, action int, warn *warnApplyResult) string {
	text := memberReqText(tr, memberReqNoticeKey(action, warn), memberReqNoticeParams(tr, broken, warn))
	fallback := memberReqText(tr, "mreq_name_unknown", i18n.TranslationParams{"id": strconv.FormatInt(user.Id, 10)})
	return lockdownSplice(text, memberReqNamePlaceholder, memberReqMention(user, fallback))
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

// memberReqFixedButton is the one full-width "I fixed it" button of a mute notice. Its data
// carries only numbers: a=f, h the hold ID and u the member, at most 52 bytes. Anyone else's
// tap is refused by the handler, so the button grants nothing by itself.
func memberReqFixedButton(tr *i18n.Translator, holdID, userID int64) gotgbot.InlineKeyboardMarkup {
	data := encodeCallbackData(memberReqCallbackNamespace, map[string]string{
		"a": "f",
		"h": strconv.FormatInt(holdID, 10),
		"u": strconv.FormatInt(userID, 10),
	})
	if data == "" {
		return gotgbot.InlineKeyboardMarkup{}
	}
	return gotgbot.InlineKeyboardMarkup{InlineKeyboard: [][]gotgbot.InlineKeyboardButton{{
		{Text: memberReqText(tr, "mreq_btn_fixed"), CallbackData: data},
	}}}
}

// memberReqNoticeKeyboard is the keyboard of a notice: the button for a mute, none for a warn
// (a warn-limit mute is the warn module's mute and is never lifted by the button), a kick or
// a ban.
func memberReqNoticeKeyboard(tr *i18n.Translator, action int, holdID, userID int64) gotgbot.InlineKeyboardMarkup {
	if action != models.RequirementActionMute {
		return gotgbot.InlineKeyboardMarkup{}
	}
	return memberReqFixedButton(tr, holdID, userID)
}
