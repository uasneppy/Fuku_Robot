package modules

import (
	"context"
	"errors"

	"github.com/PaulSonOfLars/gotgbot/v2"
	log "github.com/sirupsen/logrus"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/utils/formatting"
	"github.com/uasneppy/fuku_robot/fuku/utils/media"
)

// This file is the editor pages' previews. A preview is built by the group's own builder
// (renderGreeting, the one the join path uses) and sent by the group's own send function
// (media.SendGreeting), but only ever to the presser's own private chat, filled in with the
// presser's name and the group's title. It writes nothing: it never calls the senders that keep
// the clean-old bookkeeping, never touches last_msg_id and never deletes a message (D-10).

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

// settingsEditPreviewRules sends the Rules content the way /rules sends it: Full preview is
// buildRulesText's message (the heading and the text, or the no-rules line), Preview text the
// stored rules made safe for HTML.
func settingsEditPreviewRules(p *settingsPress, st settingsEditState, part string) error {
	text := buildRulesText(p.tr, p.group.Title, st.Text)
	if part == settingsEditPartText {
		text = normalizeRulesForHTML(st.Text)
	}
	_, err := p.b.SendMessage(p.query.From.Id, text, &gotgbot.SendMessageOpts{ParseMode: formatting.HTML})
	return err
}

// settingsEditPreviewGreeting sends the Welcome or Goodbye content. Full preview is the whole
// message as the join sends it: the text, the media and the keyboard together. A Preview of one
// part sends only that part: the text with no keyboard, the media with no caption and no keyboard,
// or the buttons under a line that says what they are.
func settingsEditPreviewGreeting(p *settingsPress, st settingsEditState, part string) error {
	target := p.query.From.Id
	none := &gotgbot.InlineKeyboardMarkup{}
	switch part {
	case "":
		text, kb := renderGreeting(p.b, p.group, &p.query.From, st.Text, st.Buttons)
		_, err := media.SendGreeting(p.b, target, text, st.FileID, st.MediaType, kb, 0)
		return err
	case settingsEditPartText:
		text, _ := renderGreeting(p.b, p.group, &p.query.From, st.Text, nil)
		_, err := media.SendGreeting(p.b, target, text, "", db.TEXT, none, 0)
		return err
	case settingsEditPartMedia:
		_, err := media.SendGreeting(p.b, target, "", st.FileID, st.MediaType, none, 0)
		return err
	case settingsEditPartButtons:
		_, kb := renderGreeting(p.b, p.group, &p.query.From, "", st.Buttons)
		_, err := media.SendGreeting(p.b, target, settingsText(p.tr, "settings_edit_preview_buttons_body"), "", db.TEXT, kb, 0)
		return err
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
