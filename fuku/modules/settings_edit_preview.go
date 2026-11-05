package modules

import (
	"context"
	"errors"

	"github.com/PaulSonOfLars/gotgbot/v2"
	log "github.com/sirupsen/logrus"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/lang"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
	"github.com/uasneppy/fuku_robot/fuku/utils/formatting"
	"github.com/uasneppy/fuku_robot/fuku/utils/keyboard"
	"github.com/uasneppy/fuku_robot/fuku/utils/media"
)

// This file is the editor pages' previews. A preview is built by the group's own builder
// (renderGreeting, the one the join path uses) and sent by the group's own send function
// (media.SendGreeting), but only ever to the presser's own private chat, filled in with the
// presser's name and the group's title. It writes no greeting or bookkeeping: it never calls the
// senders that keep the clean-old bookkeeping, never touches last_msg_id and never deletes a
// message (D-10). One side effect remains, and it is the join path's own: a Full preview, or a
// text preview, of a greeting that contains {rules} reads the rules through the join path's
// reader (renderGreeting, then rules.GetChatRulesInfo), which creates an empty rules row (and a
// chats row) for a group that has none, exactly as a real join does, and nothing else is written.
// The Rules Full preview is the message /rules sends in the group, built by buildRulesMessage in
// the group's language (read by chat ID, as /rules reads it) and sent by sendRulesMessage; only the
// header line before it is in the presser's language.

// settingsEditIsForbidden reports whether err is a Telegram 403, also when wrapped: the answer
// to a send to a user who never started the bot. It is classified by code, never by wording.
func settingsEditIsForbidden(err error) bool {
	var tgErr *gotgbot.TelegramError
	return errors.As(err, &tgErr) && tgErr != nil && tgErr.Code == 403
}

// settingsEditPreviewHeaderKey is the locale key of the line sent before a preview.
func settingsEditPreviewHeaderKey(def settingsEditPageDef) string {
	if def.Kind == settingsEditRules {
		return "settings_edit_preview_header_rules"
	}
	return "settings_edit_preview_header"
}

// settingsEditPreviewSend sends the header and then the content of a preview to the presser's
// private chat, and nothing anywhere else. part is the part code of a Preview, or "" for Full
// preview. A failed header stops before the content; nothing is retried.
func settingsEditPreviewSend(p *settingsPress, def settingsEditPageDef, st settingsEditState, part string) error {
	target := p.query.From.Id
	if _, err := p.b.SendMessage(target, settingsText(p.tr, settingsEditPreviewHeaderKey(def)), &gotgbot.SendMessageOpts{
		ParseMode: formatting.HTML,
	}); err != nil {
		return err
	}
	if def.Kind == settingsEditRules {
		return settingsEditPreviewRules(p, st, part)
	}
	return settingsEditPreviewGreeting(p, st, part)
}

// settingsEditPreviewMediaAlone sends the stored media of a page to the presser with no caption and no
// keyboard: what Preview media shows on every page, so it never carries Media under text.
func settingsEditPreviewMediaAlone(p *settingsPress, st settingsEditState) error {
	_, err := media.SendGreeting(p.b, p.query.From.Id, "", st.FileID, st.MediaType, &gotgbot.InlineKeyboardMarkup{}, 0)
	return err
}

// settingsEditPreviewButtonsAlone sends the buttons of a page to the presser under the line that says what
// they are: what Preview buttons shows on every page.
func settingsEditPreviewButtonsAlone(p *settingsPress, kb *gotgbot.InlineKeyboardMarkup) error {
	_, err := media.SendGreeting(p.b, p.query.From.Id, settingsText(p.tr, "settings_edit_preview_buttons_body"), "", db.TEXT, kb, 0)
	return err
}

// settingsEditPreviewRules sends the Rules content. Full preview is the message /rules sends in the
// group with private rules off for the same stored row: buildRulesMessage (the group form) in the
// group's language, sent by sendRulesMessage without the member-path degrade, so a refused media is
// seen as a failed preview and never as a different message. Preview text is the stored rules made
// safe for HTML, Preview media the media alone and Preview buttons the rules keyboard.
func settingsEditPreviewRules(p *settingsPress, st settingsEditState, part string) error {
	target := p.query.From.Id
	opts := &gotgbot.SendMessageOpts{ParseMode: formatting.HTML}
	switch part {
	case "":
		ctx, cancel := context.WithTimeout(context.Background(), settingsReadTimeout)
		defer cancel()
		row := models.RulesSettings{
			Rules: st.Text, FileID: st.FileID, MediaType: st.MediaType, Buttons: st.Buttons, MediaBelow: st.MediaBelow,
		}
		m := buildRulesMessage(i18n.MustNewTranslator(lang.GetGroupLanguage(ctx, p.group.Id)), p.group.Title, &row, rulesFormGroup)
		return sendRulesMessage(p.b, target, m, opts, false)
	case settingsEditPartText:
		_, err := p.b.SendMessage(target, normalizeRulesForHTML(st.Text), opts)
		return err
	case settingsEditPartMedia:
		return settingsEditPreviewMediaAlone(p, st)
	case settingsEditPartButtons:
		return settingsEditPreviewButtonsAlone(p, &gotgbot.InlineKeyboardMarkup{InlineKeyboard: keyboard.BuildKeyboard(st.Buttons)})
	}
	return errors.New("settings editor: no such part to preview")
}

// settingsEditPreviewGreeting sends the Welcome or Goodbye content. Full preview is the whole
// message as the join sends it, through the join's own sender: the text, the media with its caption
// above or below as Media under text says, and the keyboard together. A Preview of one part sends only
// that part: the text with no keyboard, the media with no caption and no keyboard, or the buttons under
// a line that says what they are.
func settingsEditPreviewGreeting(p *settingsPress, st settingsEditState, part string) error {
	target := p.query.From.Id
	none := &gotgbot.InlineKeyboardMarkup{}
	switch part {
	case "":
		text, kb := renderGreeting(p.b, p.group, &p.query.From, st.Text, st.Buttons)
		_, err := media.SendGreetingMessage(p.b, target, media.GreetingMessage{
			Text: text, FileID: st.FileID, MsgType: st.MediaType, CaptionAbove: st.MediaBelow, Keyboard: kb,
		})
		return err
	case settingsEditPartText:
		text, _ := renderGreeting(p.b, p.group, &p.query.From, st.Text, nil)
		_, err := media.SendGreeting(p.b, target, text, "", db.TEXT, none, 0)
		return err
	case settingsEditPartMedia:
		return settingsEditPreviewMediaAlone(p, st)
	case settingsEditPartButtons:
		_, kb := renderGreeting(p.b, p.group, &p.query.From, "", st.Buttons)
		return settingsEditPreviewButtonsAlone(p, kb)
	}
	return errors.New("settings editor: no such part to preview")
}

// settingsEditPreviewAnswer answers the press, once. A private menu answers empty, because the
// preview itself is the answer; a menu opened in the group answers the toast that says where the
// preview went. A 403 is the alert that tells the presser to start the bot, any other failure
// the generic alert; Telegram's own wording goes to the log only.
func settingsEditPreviewAnswer(p *settingsPress, err error) {
	switch {
	case err == nil && p.private:
		p.answer("", false)
	case err == nil:
		p.answer(settingsText(p.tr, "settings_edit_preview_sent"), false)
	case settingsEditIsForbidden(err):
		p.answer(settingsText(p.tr, "settings_edit_preview_need_start"), true)
	default:
		log.Warnf("[Settings] a preview for user %d of chat %d failed: %v", p.query.From.Id, p.group.Id, err)
		p.answer(settingsText(p.tr, "settings_edit_preview_failed"), true)
	}
}

// settingsEditRunPreview loads the page's stored message, refuses a part with nothing to show
// (a stale button) with the alert settings_edit_nothing_to_preview and no send, and otherwise
// sends the preview and answers the press. part is "" for Full preview. A read error is the
// cannot-open edit.
func settingsEditRunPreview(p *settingsPress, def settingsEditPageDef, part string) *settingsScreen {
	ctx, cancel := context.WithTimeout(context.Background(), settingsReadTimeout)
	defer cancel()
	st, err := settingsEditLoad(ctx, def, p.group.Id)
	if err != nil {
		log.Warnf("[Settings] %s page of chat %d could not be read for a preview: %v", def.Key, p.group.Id, err)
		p.cannotOpen()
		return nil
	}
	if part != "" && !settingsEditPreviewable(def, st, part) {
		p.answer(settingsText(p.tr, "settings_edit_nothing_to_preview"), true)
		return nil
	}
	settingsEditPreviewAnswer(p, settingsEditPreviewSend(p, def, st, part))
	return nil
}

// settingsEditPreview serves Preview: one part of the message, alone.
func settingsEditPreview(def settingsEditPageDef) func(*settingsPress) *settingsScreen {
	return func(p *settingsPress) *settingsScreen {
		return settingsEditRunPreview(p, def, p.fields["t"])
	}
}

// settingsEditFullPreview serves Full preview: the whole message exactly as a member sees it.
func settingsEditFullPreview(def settingsEditPageDef) func(*settingsPress) *settingsScreen {
	return func(p *settingsPress) *settingsScreen {
		return settingsEditRunPreview(p, def, "")
	}
}
