package modules

import (
	"context"
	"errors"
	"slices"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/uasneppy/fuku_robot/fuku/i18n"
)

// This file is the editor pages' actions: Set (writes the wait for the owner's next private
// message and turns the menu message into the waiting prompt), Cancel (ends that wait) and
// Remove (takes a part out at once, through the commands' apply functions). They work only from
// a private menu; in a menu opened in the group they write nothing and await nothing, and the
// page is redrawn with the note that points to the private chat (D-01).

const (
	// settingsEditRulesPromptMax is the length the Rules prompt shows. The dry run is exact and
	// may accept slightly more.
	settingsEditRulesPromptMax = 3800
	// settingsEditTextMax is the text limit of a Welcome or Goodbye text.
	settingsEditTextMax = 4096
	// settingsEditCaptionMax is the limit of the text when the group already has media, because
	// the text is then the media's caption.
	settingsEditCaptionMax = 1024
)

// settingsEditValidatePart is the Validate hook of Set, Remove and Cancel: t must be present and
// one of the page's part codes. It runs before any I/O, so a forged or missing t is refused as
// an invalid request.
func settingsEditValidatePart(pageKey string) func(map[string]string) bool {
	return func(fields map[string]string) bool {
		def, ok := settingsEditPageByKey(pageKey)
		if !ok {
			return false
		}
		part, present := fields["t"]
		return present && settingsEditPartByCode(def, part) != nil
	}
}

// settingsEditPartByCode finds a part of a page by its t value, nil when the page has none.
func settingsEditPartByCode(def settingsEditPageDef, code string) *settingsEditPart {
	i := slices.IndexFunc(def.Parts, func(part settingsEditPart) bool { return part.Code == code })
	if i < 0 {
		return nil
	}
	return &def.Parts[i]
}

// settingsEditPromptMax is the number the text prompt shows after "Up to": 3800 on Rules, else
// 1024 while the group has stored media (the text is its caption) and 4096 without.
func settingsEditPromptMax(def settingsEditPageDef, st settingsEditState) int {
	switch {
	case def.Kind == settingsEditRules:
		return settingsEditRulesPromptMax
	case settingsEditPartIsSet(st, settingsEditPartMedia):
		return settingsEditCaptionMax
	}
	return settingsEditTextMax
}

// renderSettingsEditPrompt draws the waiting prompt. It is pure. It is a screen, not a page: the
// header and group line, the part's prompt body, the end time in UTC, the replaced note when an
// earlier wait was cancelled by this one, and one Cancel button, with no Back and no Close.
func renderSettingsEditPrompt(
	v settingsView, def settingsEditPageDef, part string, until time.Time, maxLen int, replaced bool,
) settingsScreen {
	var body []string
	if p := settingsEditPartByCode(def, part); p != nil {
		body = append(body, settingsText(v.Tr, p.WaitKey, i18n.TranslationParams{"max": maxLen}))
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

// settingsEditRedraw draws the page again with a notice. A read error is the cannot-open edit
// and a nil screen.
func settingsEditRedraw(p *settingsPress, def settingsEditPageDef, notice string) *settingsScreen {
	screen, err := settingsEditOpen(def)(p, notice)
	if err != nil {
		log.Warnf("[Settings] %s page of chat %d could not be read after a press: %v", def.Key, p.group.Id, err)
		p.cannotOpen()
		return nil
	}
	return &screen
}

// settingsEditPrivateOnly wraps an action that works only from a private menu. In a menu opened
// in the group it writes nothing and awaits nothing and redraws the page with the group note
// (which points to Open in private), whatever the action is.
func settingsEditPrivateOnly(
	def settingsEditPageDef, run func(p *settingsPress, part string) *settingsScreen,
) func(*settingsPress) *settingsScreen {
	return func(p *settingsPress) *settingsScreen {
		if !p.private {
			return settingsEditRedraw(p, def, settingsNoticeWarn(settingsText(p.tr, "settings_edit_group_note")))
		}
		return run(p, p.fields["t"])
	}
}

// settingsEditWaitReplaced reports whether the wait Set replaced belonged to another group,
// page or part. A wait for the same part is only refreshed.
func settingsEditWaitReplaced(previous *settingsInputWait, groupID int64, page, part string) bool {
	return previous != nil && (previous.GroupID != groupID || previous.Page != page || previous.Part != part)
}

// settingsEditSet serves Set: it writes the owner's one wait and edits the menu message into the
// waiting prompt. Without Redis, or when the wait cannot be written, it answers the alert
// settings_edit_unavailable and edits nothing.
func settingsEditSet(def settingsEditPageDef) func(*settingsPress) *settingsScreen {
	return settingsEditPrivateOnly(def, func(p *settingsPress, part string) *settingsScreen {
		ctx, cancel := context.WithTimeout(context.Background(), settingsReadTimeout)
		defer cancel()
		st, err := settingsEditLoad(ctx, def, p.group.Id)
		if err != nil {
			log.Warnf("[Settings] %s page of chat %d could not be read for Set: %v", def.Key, p.group.Id, err)
			p.cannotOpen()
			return nil
		}
		nonce, err := newSettingsInputNonce()
		if err != nil {
			log.Errorf("[Settings] no nonce for a wait: %v", err)
			p.answer(settingsText(p.tr, "settings_edit_unavailable"), true)
			return nil
		}
		until := time.Now().Add(settingsInputTTL)
		previous, err := settingsInputSet(ctx, p.query.From.Id, settingsInputWait{
			GroupID:     p.group.Id,
			Page:        def.Key,
			Part:        part,
			PromptMsgID: p.query.Message.GetMessageId(),
			Nonce:       nonce,
			ExpiresAt:   until,
		})
		if err != nil {
			if !errors.Is(err, errSettingsInputUnavailable) {
				log.Warnf("[Settings] the wait of user %d could not be written: %v", p.query.From.Id, err)
			}
			p.answer(settingsText(p.tr, "settings_edit_unavailable"), true)
			return nil
		}
		screen := renderSettingsEditPrompt(p.view(), def, part, until, settingsEditPromptMax(def, st),
			settingsEditWaitReplaced(previous, p.group.Id, def.Key, part))
		return &screen
	})
}

// settingsEditCancel serves Cancel: it deletes the wait and its marker only when the wait is for
// this group, page and part, and edits the message into the page. A prompt whose wait was
// replaced, used or is being saved answers settings_edit_prompt_stale and leaves the live wait
// alone; one after the 10 minutes answers settings_edit_wait_expired.
func settingsEditCancel(def settingsEditPageDef) func(*settingsPress) *settingsScreen {
	return settingsEditPrivateOnly(def, func(p *settingsPress, part string) *settingsScreen {
		ctx, cancel := context.WithTimeout(context.Background(), settingsReadTimeout)
		defer cancel()
		outcome, err := settingsInputCancel(ctx, p.query.From.Id, p.group.Id, def.Key, part)
		notice := ""
		switch {
		case err != nil:
			if !errors.Is(err, errSettingsInputUnavailable) {
				log.Warnf("[Settings] the wait of user %d could not be cancelled: %v", p.query.From.Id, err)
			}
			p.answer(settingsText(p.tr, "settings_edit_unavailable"), true)
		case outcome == settingsInputCancelDeleted:
			notice = settingsNoticeOK(settingsText(p.tr, "settings_edit_cancelled"))
		case outcome == settingsInputCancelExpired:
			p.answer(settingsText(p.tr, "settings_edit_wait_expired"), true)
		default:
			p.answer(settingsText(p.tr, "settings_edit_prompt_stale"), true)
		}
		return settingsEditRedraw(p, def, notice)
	})
}

// settingsEditApplyRemove removes one part through the apply function of the command it
// mirrors. It returns the outcome and the state read back, nil when the caller has to reload
// (the write stored but its read-back failed, or the write failed).
func settingsEditApplyRemove(
	ctx context.Context, def settingsEditPageDef, part string, chatID int64,
) (greetingApplyOutcome, *settingsEditState) {
	if def.Kind == settingsEditRules {
		res := applyRulesClear(ctx, chatID)
		if res.Outcome == greetingSaveFailed {
			return res.Outcome, nil
		}
		return res.Outcome, &settingsEditState{Kind: def.Kind}
	}
	isWelcome := def.Kind == settingsEditWelcome
	var res greetingApplyResult
	switch part {
	case settingsEditPartText:
		res = applyGreetingRemoveText(ctx, chatID, isWelcome)
	case settingsEditPartMedia:
		res = applyGreetingRemoveMedia(ctx, chatID, isWelcome)
	case settingsEditPartButtons:
		res = applyGreetingRemoveButtons(ctx, chatID, isWelcome)
	default:
		return greetingSaveFailed, nil
	}
	if res.Outcome == greetingSaveFailed || res.Settings == nil {
		return res.Outcome, nil
	}
	st, err := settingsEditStateOf(def, res.Settings)
	if err != nil {
		return res.Outcome, nil
	}
	return res.Outcome, &st
}

// settingsEditRemove serves Remove: one tap, no confirmation card. A part that is not set (a
// stale button) redraws with settings_edit_nothing_to_remove and writes nothing; a failed write
// redraws the freshly loaded page with common_settings_save_failed; a stored one redraws from
// the read-back with the part's removed notice.
func settingsEditRemove(def settingsEditPageDef) func(*settingsPress) *settingsScreen {
	return settingsEditPrivateOnly(def, func(p *settingsPress, part string) *settingsScreen {
		ctx, cancel := context.WithTimeout(context.Background(), settingsReadTimeout)
		defer cancel()
		st, err := settingsEditLoad(ctx, def, p.group.Id)
		if err != nil {
			log.Warnf("[Settings] %s page of chat %d could not be read for Remove: %v", def.Key, p.group.Id, err)
			p.cannotOpen()
			return nil
		}
		if !settingsEditPartIsSet(st, part) {
			return settingsEditRedraw(p, def, settingsNoticeWarn(settingsText(p.tr, "settings_edit_nothing_to_remove")))
		}
		notice := ""
		outcome, stored := settingsEditApplyRemove(ctx, def, part, p.group.Id)
		switch {
		case outcome == greetingSaveFailed:
			log.Errorf("[Settings] Remove %s of chat %d failed", part, p.group.Id)
			notice = settingsNoticeWarn(settingsText(p.tr, "common_settings_save_failed"))
		default:
			notice = settingsNoticeOK(settingsText(p.tr, settingsEditPartByCode(def, part).RemovedKey))
		}
		if stored == nil {
			return settingsEditRedraw(p, def, notice)
		}
		screen := renderSettingsEditor(p.view(), def, *stored, settingsEditScreenOpts{
			Notice: notice, BotUsername: getBotUsername(p.b),
		})
		return &screen
	})
}

// settingsEditActions are the page's Set, Remove and Cancel actions. Set and Remove need the
// right of the matching commands, Cancel only the admin check every press makes; all three have
// t checked before any I/O.
func settingsEditActions(def settingsEditPageDef) map[string]settingsPageAction {
	validate := settingsEditValidatePart(def.Key)
	return map[string]settingsPageAction{
		def.SetCode:    {Fields: []string{"t"}, Right: def.EditRight, Validate: validate, Run: settingsEditSet(def)},
		def.RemoveCode: {Fields: []string{"t"}, Right: def.EditRight, Validate: validate, Run: settingsEditRemove(def)},
		def.CancelCode: {Fields: []string{"t"}, Right: settingsRightAdmin, Validate: validate, Run: settingsEditCancel(def)},
	}
}
