package modules

import (
	"context"
	"slices"
	"strings"

	"github.com/PaulSonOfLars/gotgbot/v2"
	log "github.com/sirupsen/logrus"

	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/db/staff"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
)

// The Blocked sources page of the settings menu (page v) is a table of the list engine (settings_list.go) plus the
// action screen. It lists the forward blocklist of a Staff Group, the channels and groups whose forwards the bot deletes
// in every linked group, eight a page, adds one source per typed message, deletes one behind a two-tap card that binds
// the row and sets the list's one action. The page exists only for a Staff Group (its Hidden hook) and only in the
// private chat. Writes are the Staff Group's live creator only (settingsRightStaffOwner), exactly as the commands'
// writes are; an administrator who passed the owner gate can view and page. Every write goes through the apply
// functions of staff_forwardblock.go, the functions /addfwdblock, /rmfwdblock and /fwdblockaction call, so there is no
// second write path. It reads through the fresh, chat-scoped, non-creating readers of the staff repository, never the
// cached rules the watcher reads.

const (
	// settingsPageFwdBlock is the p value of the Blocked sources page.
	settingsPageFwdBlock = "v"
	// settingsActFwdBlockOpen opens the action screen (no field besides the owner), settingsActFwdBlockSet sets the
	// list's action (the field v, one of blacklistActionValues).
	settingsActFwdBlockOpen = "ve"
	settingsActFwdBlockSet  = "vs"
	// settingsFwdBlockApplyTimeout bounds one typed add: a live getChat and one write.
	settingsFwdBlockApplyTimeout = forwardBlockResolveTimeout + settingsReadTimeout
)

// The reads and the Staff Group gate of the page, variables only so a test can count or fail them.
var (
	// settingsFwdBlockStaffGate says whether a chat is a Staff Group by the cached gate every replica shares.
	settingsFwdBlockStaffGate = func(chatID int64) bool { return staff.GetStaffGroup(chatID) != nil }
	// settingsFwdBlockRead lists the Staff Group's sources, each stamped with the list's one action.
	settingsFwdBlockRead = func(ctx context.Context, chatID int64) ([]settingsListEntry, error) {
		entries, err := settingsListReader(staff.ListForwardBlocksFresh, settingsFwdBlockEntryOf)(ctx, chatID)
		if err != nil || len(entries) == 0 {
			return entries, err
		}
		action, err := settingsFwdBlockActionRead(ctx, chatID)
		if err != nil {
			return nil, err
		}
		for i := range entries {
			entries[i].Action = action
		}
		return entries, nil
	}
	// settingsFwdBlockEntry reads one source by ID, only when it belongs to the Staff Group.
	settingsFwdBlockEntry = settingsListEntryReader(staff.GetForwardBlockFresh, settingsFwdBlockEntryOf)
	// settingsFwdBlockCount counts the Staff Group's sources.
	settingsFwdBlockCount = settingsListCounter(staff.CountForwardBlocksFresh)
	// settingsFwdBlockActionRead reads the list's one action, fresh. A Staff Group with no row reads none.
	settingsFwdBlockActionRead = func(ctx context.Context, chatID int64) (string, error) {
		rules, err := staff.GetForwardBlockRulesFresh(ctx, chatID)
		return rules.Action, err
	}
)

// settingsFwdBlockHidden is the page's Hidden: the page exists only for a chat the cached gate knows as a Staff Group.
// An unreadable or missing row hides it, so a group that is not a Staff Group never shows it and never opens it.
func settingsFwdBlockHidden(group *gotgbot.Chat) bool {
	return group == nil || !settingsFwdBlockStaffGate(group.Id)
}

// settingsFwdBlockEntryOf is the engine's entry of a stored source: its text is the ID and the title.
func settingsFwdBlockEntryOf(row models.StaffForwardBlock) settingsListEntry {
	return settingsListEntry{ID: row.ID, Text: forwardBlockEntryText(row.SourceChatID, row.SourceTitle)}
}

// settingsFwdBlockChoiceRow builds one row of the action choices: each button sets its own value, the stored one is
// marked, and the labels are the option words of the blacklist's action screen.
func settingsFwdBlockChoiceRow(k *settingsKeyboard, selected string, values ...string) []gotgbot.InlineKeyboardButton {
	row := make([]gotgbot.InlineKeyboardButton, 0, len(values))
	for _, value := range values {
		label := settingsBlacklistActionWord(k.view.Tr, value)
		if value == selected {
			label = settingsSelectedPrefix + label
		}
		if button, ok := k.button(label, map[string]string{"a": settingsActFwdBlockSet, "v": value}); ok {
			row = append(row, button)
		}
	}
	return row
}

// renderSettingsFwdBlockAction draws the action screen: the intro, the stored action, the hint, the two choice rows with
// a check mark on the stored one, and Back to the list (page 1) and Close. Unlike the blacklist's, it is drawn for an
// empty list too, because the action is a list-wide setting. It is pure.
func renderSettingsFwdBlockAction(v settingsView, action, notice string) settingsScreen {
	k := newSettingsKeyboard(v)
	k.row(settingsFwdBlockChoiceRow(k, action, "warn", "mute", "kick")...)
	k.row(settingsFwdBlockChoiceRow(k, action, "ban", "none")...)
	var nav []gotgbot.InlineKeyboardButton
	if back, ok := k.button(settingsText(v.Tr, "settings_btn_back"), map[string]string{"a": settingsActOpen, "p": settingsPageFwdBlock}); ok {
		nav = append(nav, back)
	}
	if closeButton, ok := k.button(settingsText(v.Tr, "settings_btn_close"), map[string]string{"a": settingsActClose}); ok {
		nav = append(nav, closeButton)
	}
	k.row(nav...)
	body := []string{
		settingsText(v.Tr, "settings_bl_action_intro"),
		settingsText(v.Tr, "settings_bl_action_line", i18n.TranslationParams{"action": settingsBlacklistActionWord(v.Tr, action)}),
		"",
		settingsText(v.Tr, "settings_fwd_action_hint"),
	}
	return k.screen(settingsCompose(v, "settings_page_fwdblock", body, notice, false))
}

// settingsFwdBlockDrawAction reads the list's action fresh and draws the action screen with the notice.
func settingsFwdBlockDrawAction(p *settingsPress, notice string) (settingsScreen, error) {
	ctx, cancel := context.WithTimeout(context.Background(), settingsReadTimeout)
	defer cancel()
	action, err := settingsFwdBlockActionRead(ctx, p.group.Id)
	if err != nil {
		return settingsScreen{}, err
	}
	return renderSettingsFwdBlockAction(p.view(), action, notice), nil
}

// settingsFwdBlockShowAction turns a draw of the action screen into the screen of a page action: a read error is the
// cannot-open edit.
func settingsFwdBlockShowAction(p *settingsPress, screen settingsScreen, err error) *settingsScreen {
	if err != nil {
		log.Warnf("[Settings] the forward block action screen of chat %d could not be read: %v", p.group.Id, err)
		p.cannotOpen()
		return nil
	}
	return &screen
}

// settingsFwdBlockRunOpen handles ve: a fresh read and the action screen.
func settingsFwdBlockRunOpen(p *settingsPress) *settingsScreen {
	screen, err := settingsFwdBlockDrawAction(p, "")
	return settingsFwdBlockShowAction(p, screen, err)
}

// settingsFwdBlockRunSet handles vs: the list's action through applyForwardBlockAction, the function /fwdblockaction
// uses, then the action screen from a fresh read. A press of the stored action writes the same row again and shows no
// notice; a failed save redraws the stored state with the save-failed notice.
func settingsFwdBlockRunSet(p *settingsPress) *settingsScreen {
	ctx, cancel := context.WithTimeout(context.Background(), settingsReadTimeout)
	defer cancel()
	notice := ""
	switch applyForwardBlockAction(ctx, p.group.Id, p.query.From.Id, p.fields["v"]).Outcome {
	case forwardBlockActionNoStaffGroup:
		notice = settingsNoticeWarn(settingsText(p.tr, "staff_unset_not_staff"))
	case forwardBlockActionSaveFailed:
		notice = settingsNoticeWarn(settingsText(p.tr, "common_settings_save_failed"))
	case forwardBlockActionUnknown:
		// Unreachable: step 1 only lets the command's own values through.
		p.answer(settingsText(p.tr, "common_callback_invalid_request"), true)
		return nil
	}
	p.answer("", false)
	screen, err := settingsFwdBlockDrawAction(p, notice)
	return settingsFwdBlockShowAction(p, screen, err)
}

// settingsFwdBlockAddRow is the part of the row after the pager that follows the Add button: the Action button, always
// drawn, because the action is set for the whole list even before it holds a source.
func settingsFwdBlockAddRow(k *settingsKeyboard, _ []settingsListEntry) []gotgbot.InlineKeyboardButton {
	button, ok := k.button(settingsText(k.view.Tr, "settings_btn_bl_action"), map[string]string{"a": settingsActFwdBlockOpen})
	if !ok {
		return nil
	}
	return []gotgbot.InlineKeyboardButton{button}
}

// settingsFwdBlockActions are the page's own actions: the action screen (viewing) and the action set (the creator's).
func settingsFwdBlockActions() map[string]settingsPageAction {
	return map[string]settingsPageAction{
		settingsActFwdBlockOpen: {
			Right:    settingsRightAdmin,
			Validate: settingsListPrivateField,
			Run:      settingsFwdBlockRunOpen,
		},
		settingsActFwdBlockSet: {
			Fields: []string{"v"},
			Right:  settingsRightStaffOwner,
			Validate: func(fields map[string]string) bool {
				return settingsListPrivateField(fields) && slices.Contains(blacklistActionValues, fields["v"])
			},
			Run: settingsFwdBlockRunSet,
		},
	}
}

// settingsFwdBlockRemove deletes one source by its row ID through applyForwardBlockRemoveByID, the function the
// commands' removal shares with the page: one conditional DELETE by (id, staff chat) and the cache drop.
func settingsFwdBlockRemove(ctx context.Context, chatID, id int64) settingsListRemoval {
	result := applyForwardBlockRemoveByID(ctx, chatID, id)
	switch result.Outcome {
	case forwardBlockRemoved:
		return settingsListRemoval{Outcome: settingsListRemoved, Text: forwardBlockEntryText(result.SourceChatID, result.Title)}
	case forwardBlockGone:
		return settingsListRemoval{Outcome: settingsListRemoveGone}
	}
	return settingsListRemoval{Outcome: settingsListRemoveFailed}
}

// settingsFwdBlockPrompt is the lines of the add prompt before its footer: what the creator is asked to send and what a
// source can be.
func settingsFwdBlockPrompt(v settingsView, _ string) []string {
	return []string{
		settingsText(v.Tr, "settings_fwd_wait"),
		settingsText(v.Tr, "settings_fwd_wait_rules", i18n.TranslationParams{"max": staff.ForwardBlockMax}),
	}
}

// settingsFwdBlockSavedNotice is the notice of the saved page: the success mark on the saved line naming the source
// and, for a message cut from an album, the album note.
func settingsFwdBlockSavedNotice(tr *i18n.Translator, result forwardBlockAddResult, album bool) string {
	notice := settingsNoticeOK(settingsListEntryText(tr, "settings_fwd_saved", "source", forwardBlockEntryText(result.SourceChatID, result.Title)))
	if album {
		notice += "\n" + settingsText(tr, "settings_edit_album_cut")
	}
	return notice
}

// settingsFwdBlockRefuse answers a typed add that stored nothing, with the claim already given back. A lookup that
// could not be answered keeps the wait with the door's own plain reply, a Staff Group that no longer is one ends it,
// and every other refusal is the shared text with the wait kept and the Cancel button.
func settingsFwdBlockRefuse(r *settingsInputReply, result forwardBlockAddResult) {
	switch result.Outcome {
	case forwardBlockCheckFailed:
		r.keep("settings_check_failed")
	case forwardBlockNoStaffGroup:
		r.endWith(forwardBlockAddText(r.tr, result))
	default:
		r.refuse(forwardBlockAddText(r.tr, result))
	}
}

// settingsFwdBlockTyped is the door's handler of a typed reply to the page's wait (one step): the text must be exactly
// one token, an ID or an @username. It is claimed and stored through applyForwardBlockAdd, the function /addfwdblock
// stores through, with the wait's Staff Group and the sender. Anything but an added source gives the claim back and
// answers with the shared text; an added one finishes the wait and sends the list at the page of the new source.
func settingsFwdBlockTyped(r *settingsInputReply, _ settingsListPart) {
	fields := strings.Fields(r.msg.GetText())
	switch len(fields) {
	case 0:
		r.refuse(r.text("settings_edit_refuse_need_text"))
		return
	case 1:
	default:
		r.refuse(r.text("settings_fwd_refuse_one"))
		return
	}
	claim, claimed := r.claimWait()
	if !claimed {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), settingsFwdBlockApplyTimeout)
	defer cancel()
	result := applyForwardBlockAdd(ctx, r.b, r.wait.GroupID, r.user.Id, fields[0])
	if result.Outcome != forwardBlockAdded {
		r.releaseWait(claim)
		settingsFwdBlockRefuse(r, result)
		return
	}
	r.finishWait(claim)
	log.Infof("[Settings] user %d added a forward block source to staff chat %d", r.user.Id, r.wait.GroupID)
	holding := forwardBlockEntryText(result.SourceChatID, result.Title)
	r.sendSavedList(*r.list, holding, settingsFwdBlockSavedNotice(r.tr, result, settingsInputIsAlbum(r.msg)))
	r.closePrompt()
}

// settingsFwdBlockDef is the table of the Blocked sources page.
func settingsFwdBlockDef() settingsListDef {
	return settingsListDef{
		Key:       settingsPageFwdBlock,
		Order:     11,
		Icon:      "🚷",
		TitleKey:  "settings_page_fwdblock",
		PagerCode: "vg",
		AskCode:   "vx",
		YesCode:   "vy",
		// Every write declares the Staff Group creator's right, as /addfwdblock, /rmfwdblock and /fwdblockaction ask for.
		Right:        settingsRightStaffOwner,
		Hidden:       settingsFwdBlockHidden,
		Remove:       settingsFwdBlockRemove,
		DeleteAskKey: "settings_fwd_delete_ask",
		RemovedKey:   "settings_fwd_removed",
		EntryParam:   "source",
		Read: func(ctx context.Context, chatID int64) ([]settingsListEntry, error) {
			return settingsFwdBlockRead(ctx, chatID)
		},
		Entry: func(ctx context.Context, chatID, id int64) (*settingsListEntry, error) {
			return settingsFwdBlockEntry(ctx, chatID, id)
		},
		Count:       func(ctx context.Context, chatID int64) (int, error) { return settingsFwdBlockCount(ctx, chatID) },
		CountMax:    staff.ForwardBlockMax,
		AddCode:     "va",
		CancelCode:  "vc",
		AddKey:      "settings_btn_fwd_add",
		Prompt:      settingsFwdBlockPrompt,
		Typed:       settingsFwdBlockTyped,
		IntroKey:    "settings_fwd_intro",
		CountKey:    "settings_fwd_count",
		EmptyKey:    "settings_fwd_empty",
		BodyExtra:   settingsBlacklistActionLine,
		AddRowExtra: settingsFwdBlockAddRow,
		Actions:     settingsFwdBlockActions(),
	}
}

func init() {
	registerSettingsList(settingsFwdBlockDef())
}
