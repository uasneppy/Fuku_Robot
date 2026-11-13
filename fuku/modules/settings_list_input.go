package modules

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	log "github.com/sirupsen/logrus"

	"github.com/uasneppy/fuku_robot/fuku/i18n"
)

// This file is the list engine's side of the typed-reply door (Phase 19, D-04, M43). A list page's Add button writes
// the owner's one wait through the same store the editor's Set uses and edits the menu message into the prompt; the
// owner's next private message is then read by the same door, which hands a wait whose page is a list page to the
// list branch (settingsInputReply.runList). One door, one wait per user and one set of ends (Set, Cancel, /cancel,
// finish, expiry) serve the editor and the three list pages. Nothing in this file names a word, keyword or note name
// in a log line, a button or callback data.

// settingsListPart is one part of a list page's typed add: Step is 1 for the part the Add button waits for and 2 for
// the part a two-step add moves to, Next is the step-2 part of a step-1 part, and WordKey names the part in the
// wait-ended note.
type settingsListPart struct {
	Page    string
	Code    string
	Step    int
	Next    string
	WordKey string
}

// settingsListParts is the static part table of the list pages (UI-SPEC section 1): the blacklist's words, the
// filter's keyword and reply and the note's name and content. It is static so the wait store can validate a part
// without a registered page.
var settingsListParts = []settingsListPart{
	{Page: settingsPageBlacklist, Code: "w", Step: 1, WordKey: "settings_entries_part_words"},
	{Page: "i", Code: "k", Step: 1, Next: "y", WordKey: "settings_entries_part_keyword"},
	{Page: "i", Code: "y", Step: 2, WordKey: "settings_entries_part_reply"},
	{Page: "e", Code: "n", Step: 1, Next: "c", WordKey: "settings_entries_part_name"},
	{Page: "e", Code: "c", Step: 2, WordKey: "settings_entries_part_content"},
}

// settingsListPartByCode finds a part of a list page by its t value, nil when the page has no such part.
func settingsListPartByCode(page, code string) *settingsListPart {
	i := slices.IndexFunc(settingsListParts, func(part settingsListPart) bool {
		return part.Page == page && part.Code == code
	})
	if i < 0 {
		return nil
	}
	return &settingsListParts[i]
}

// settingsListFirstPart is the part the page's Add button waits for: its step-1 part.
func settingsListFirstPart(page string) *settingsListPart {
	i := slices.IndexFunc(settingsListParts, func(part settingsListPart) bool {
		return part.Page == page && part.Step == 1
	})
	if i < 0 {
		return nil
	}
	return &settingsListParts[i]
}

// settingsListValidateCancel is the Validate hook of Cancel: the owner field c must be present and t must be one
// of the page's parts, checked before any I/O.
func settingsListValidateCancel(page string) func(map[string]string) bool {
	return func(fields map[string]string) bool {
		part, present := fields["t"]
		return settingsListPrivateField(fields) && present && settingsListPartByCode(page, part) != nil
	}
}

// renderSettingsListPrompt draws the waiting prompt of a list page's Add (Screen 5). It is pure: the header and
// group line, the lines the table's Prompt gives for the part, the end time in UTC, the replaced note when an
// earlier wait was cancelled by this one, and one Cancel button with no Back and no Close.
func renderSettingsListPrompt(v settingsView, def settingsListDef, part string, until time.Time, replaced bool) settingsScreen {
	var body []string
	if def.Prompt != nil {
		body = append(body, def.Prompt(v, part)...)
	}
	body = append(body, "", settingsText(v.Tr, "settings_edit_wait_footer", i18n.TranslationParams{
		"time": until.UTC().Format("15:04"),
	}))
	if replaced {
		body = append(body, "", settingsText(v.Tr, "settings_edit_replaced_note"))
	}
	k := newSettingsKeyboard(v)
	if cancel, ok := k.button(settingsText(v.Tr, "settings_btn_edit_cancel"), map[string]string{"a": def.CancelCode, "t": part}); ok {
		k.row(cancel)
	}
	return k.screen(settingsCompose(v, def.TitleKey, body, "", false))
}

// settingsListRunAdd serves Add: it writes the owner's one wait for the page's first part and edits the menu message
// into the waiting prompt. Without Redis, or when the wait cannot be written, it answers the alert
// settings_entries_add_unavailable and edits nothing.
func settingsListRunAdd(def settingsListDef) func(*settingsPress) *settingsScreen {
	return func(p *settingsPress) *settingsScreen {
		part := settingsListFirstPart(def.Key)
		if part == nil {
			p.answer(settingsText(p.tr, "common_callback_invalid_request"), true)
			return nil
		}
		ctx, cancel := context.WithTimeout(context.Background(), settingsReadTimeout)
		defer cancel()
		nonce, err := newSettingsInputNonce()
		if err != nil {
			log.Errorf("[Settings] no nonce for a wait: %v", err)
			p.answer(settingsText(p.tr, "settings_entries_add_unavailable"), true)
			return nil
		}
		until := time.Now().Add(settingsInputTTL)
		previous, err := settingsInputSet(ctx, p.query.From.Id, settingsInputWait{
			GroupID:     p.group.Id,
			Page:        def.Key,
			Part:        part.Code,
			PromptMsgID: p.query.Message.GetMessageId(),
			Nonce:       nonce,
			ExpiresAt:   until,
		})
		if err != nil {
			if !errors.Is(err, errSettingsInputUnavailable) {
				log.Warnf("[Settings] the wait of user %d could not be written: %v", p.query.From.Id, err)
			}
			p.answer(settingsText(p.tr, "settings_entries_add_unavailable"), true)
			return nil
		}
		screen := renderSettingsListPrompt(p.view(), def, part.Code, until,
			settingsEditWaitReplaced(previous, p.group.Id, def.Key, part.Code))
		return &screen
	}
}

// settingsListRunCancel serves Cancel: it deletes the wait and its marker only when the wait is for this group, page
// and part and the button sits on the wait's own prompt or a refusal noted for it, and edits the message into the
// list's page 1. A prompt whose wait was replaced, used or is being saved answers settings_edit_prompt_stale and leaves
// the live wait alone; one after the ten minutes answers settings_edit_wait_expired; a Redis error that is not "no
// Redis" answers the alert settings_edit_unavailable and edits nothing, because the wait may still be live.
func settingsListRunCancel(def settingsListDef) func(*settingsPress) *settingsScreen {
	return func(p *settingsPress) *settingsScreen {
		ctx, cancel := context.WithTimeout(context.Background(), settingsReadTimeout)
		defer cancel()
		outcome, err := settingsInputCancel(ctx, p.query.From.Id, p.group.Id, def.Key, p.fields["t"], p.query.Message.GetMessageId())
		notice := ""
		switch {
		case errors.Is(err, errSettingsInputUnavailable):
			// No Redis means no wait can exist, so the page is the right thing to show.
			p.answer(settingsText(p.tr, "settings_edit_unavailable"), true)
		case err != nil:
			log.Warnf("[Settings] the wait of user %d could not be cancelled: %v", p.query.From.Id, err)
			p.answer(settingsText(p.tr, "settings_edit_unavailable"), true)
			return nil
		case outcome == settingsInputCancelDeleted:
			notice = settingsNoticeOK(settingsText(p.tr, "settings_edit_cancelled"))
		case outcome == settingsInputCancelExpired:
			p.answer(settingsText(p.tr, "settings_edit_wait_expired"), true)
		default:
			p.answer(settingsText(p.tr, "settings_edit_prompt_stale"), true)
		}
		screen, err := settingsListDraw(p, def, 1, notice)
		return settingsListShow(p, def, screen, err)
	}
}

// settingsListAddActions are the Add and Cancel actions of a table with a typed add. Both require the owner field c, so
// a group menu never reaches them; Add needs the page's write right and Cancel only the admin check every press makes.
func settingsListAddActions(def settingsListDef) map[string]settingsPageAction {
	return map[string]settingsPageAction{
		def.AddCode: {
			Right:    def.Right,
			Validate: settingsListPrivateField,
			Run:      settingsListRunAdd(def),
		},
		def.CancelCode: {
			Fields:   []string{"t"},
			Right:    settingsRightAdmin,
			Validate: settingsListValidateCancel(def.Key),
			Run:      settingsListRunCancel(def),
		},
	}
}

// settingsListAddRow is the row after the pager: the table's Add button first, then the table's own extra buttons.
func settingsListAddRow(k *settingsKeyboard, def settingsListDef, entries []settingsListEntry) []gotgbot.InlineKeyboardButton {
	var row []gotgbot.InlineKeyboardButton
	if def.AddCode != "" {
		if add, ok := k.button(settingsText(k.view.Tr, def.AddKey), map[string]string{"a": def.AddCode}); ok {
			row = append(row, add)
		}
	}
	if def.AddRowExtra != nil {
		row = append(row, def.AddRowExtra(k, entries)...)
	}
	return row
}

// claimWait claims the sender's wait for the holder of the wait's nonce, once. It answers settings_check_failed and
// keeps the wait when the claim cannot be made; a claim lost to another delivery (or to a Cancel) answers nothing. It
// reports whether the claim was taken, and the claim that finishWait and releaseWait need.
func (r *settingsInputReply) claimWait() (string, bool) {
	claim, claimed, err := settingsInputClaim(context.Background(), r.user.Id, r.wait.Nonce)
	if err != nil {
		log.Warnf("[Settings] claiming the wait of user %d failed: %v", r.user.Id, err)
		r.keep("settings_check_failed")
		return "", false
	}
	return claim, claimed
}

// finishWait deletes the wait and its marker for the holder of claim, logging a failure only.
func (r *settingsInputReply) finishWait(claim string) {
	if err := settingsInputFinish(context.Background(), r.user.Id, claim); err != nil {
		log.Warnf("[Settings] finishing the wait of user %d failed: %v", r.user.Id, err)
	}
}

// releaseWait gives the wait back with its remaining time, for the holder of claim, logging a failure only.
func (r *settingsInputReply) releaseWait(claim string) {
	if err := settingsInputRelease(context.Background(), r.user.Id, claim); err != nil {
		log.Warnf("[Settings] giving the wait of user %d back failed: %v", r.user.Id, err)
	}
}

// runList judges a reply to a wait whose page is a list page: authority first, exactly as a press judges the page's
// right, then the page's own typed handler. The editor's path (run) is not reached.
func (r *settingsInputReply) runList(def settingsListDef, part settingsListPart) {
	r.list = &def
	if !r.authorize(def.Right) {
		return
	}
	if def.Typed == nil {
		r.end("settings_check_failed")
		return
	}
	def.Typed(r, part)
}

// sendSavedList sends the list page as a new message in the private chat with the notice, at the page that holds the
// entry with the given text (page 1 when none does). When the list cannot be read the notice alone is sent.
func (r *settingsInputReply) sendSavedList(def settingsListDef, holding, notice string) {
	ctx, cancel := context.WithTimeout(context.Background(), settingsReadTimeout)
	defer cancel()
	entries, err := def.Read(ctx, r.wait.GroupID)
	if err != nil {
		log.Warnf("[Settings] the %s list of chat %d could not be read after a typed save: %v", def.Key, r.wait.GroupID, err)
		r.send(notice, nil, false)
		return
	}
	entries = settingsListSort(entries)
	page := 1
	if i := slices.IndexFunc(entries, func(e settingsListEntry) bool { return e.Text == holding }); i >= 0 {
		page = i/settingsListPageSize + 1
	}
	screen := renderSettingsList(r.view(), def, entries, page, notice)
	r.send(screen.Text, &screen.Keyboard, false)
}
