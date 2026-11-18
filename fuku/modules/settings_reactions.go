package modules

import (
	"context"
	"strings"
	"unicode/utf8"

	log "github.com/sirupsen/logrus"

	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/db/reactions"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
)

// The Reactions page of the settings menu (page x) is the fifth table of the list engine (settings_list.go). It
// lists the chat's reactions (a keyword and the emoji the bot sets on a message that contains it) eight a page,
// adds one from one typed message "keyword emoji", and deletes one behind the engine's two-tap card that binds the
// row. Every write goes through the apply functions of reactions.go, the functions /addreaction and
// /removereaction write through, so there is no second write path; a keyword the chat already has is refused,
// never overwritten. It reads through the fresh, chat-scoped, non-creating readers of the reactions repository and
// is private-only: a menu opened in the group shows only the framework's note.

const (
	// settingsPageReactions is the p value of the Reactions page.
	settingsPageReactions = "x"
	// settingsReactionKeywordMax is the longest keyword the page accepts, in code points.
	settingsReactionKeywordMax = 100
	// settingsReactionExamples are the valid emoji the refusal of an unsupported one names.
	settingsReactionExamples = "👍 ❤ 🔥 🎉 😁 🤔 👏 🙏"
)

// The reads of the page, variables only so a test can count or fail them.
var (
	// settingsReactionsRead lists the chat's reactions (ID, keyword and emoji).
	settingsReactionsRead = settingsListReader(reactions.ListReactionEntriesFresh, settingsReactionsEntryOf)
	// settingsReactionsEntry reads one reaction by ID, only when it belongs to the chat.
	settingsReactionsEntry = settingsListEntryReader(reactions.GetReactionEntryFresh, settingsReactionsEntryOf)
	// settingsReactionsCount counts the chat's reactions.
	settingsReactionsCount = settingsListCounter(reactions.CountReactionsFresh)
)

// settingsReactionsEntryOf is the engine's entry of a stored reaction: the keyword, with the emoji as its detail.
func settingsReactionsEntryOf(row models.Reactions) settingsListEntry {
	return settingsListEntry{ID: int64(row.ID), Text: row.Keyword, Detail: row.Emoji}
}

// settingsReactionsPrompt is the lines of the add prompt before its footer.
func settingsReactionsPrompt(v settingsView, _ string) []string {
	return []string{
		settingsText(v.Tr, "settings_rct_wait"),
		settingsText(v.Tr, "settings_rct_wait_rules", i18n.TranslationParams{"max": settingsReactionKeywordMax}),
	}
}

// settingsReactionsCapText is the refusal of an Add at the cap.
func settingsReactionsCapText(tr *i18n.Translator) string {
	return settingsText(tr, "settings_rct_refuse_cap", i18n.TranslationParams{"max": reactionsCapMax})
}

// settingsReactionsAddGate is consulted before the Add press writes a wait: at the cap (a fresh count) page 1 is
// redrawn with the cap notice and no wait is created. A count that cannot be read is the cannot-open edit.
func settingsReactionsAddGate(p *settingsPress) *settingsScreen {
	return settingsListCapGate(p, settingsPageReactions, settingsReactionsCount, reactionsCapMax, settingsReactionsCapText)
}

// settingsReactionsRemove deletes one reaction by its row ID through applyReactionRemoveByID.
func settingsReactionsRemove(ctx context.Context, chatID, id int64) settingsListRemoval {
	result := applyReactionRemoveByID(ctx, chatID, id)
	switch result.Outcome {
	case reactionRemoved:
		return settingsListRemoval{Outcome: settingsListRemoved, Text: result.Keyword}
	case reactionRemoveAbsent:
		return settingsListRemoval{Outcome: settingsListRemoveGone}
	}
	return settingsListRemoval{Outcome: settingsListRemoveFailed}
}

// settingsReactionsTyped is the door's handler of a typed reply to the page's wait (one step): exactly two
// tokens, the keyword and the emoji. A refusal keeps the wait with the reason and the Cancel button: not two
// tokens, a keyword over settingsReactionKeywordMax code points, an emoji that is not one of Telegram's
// reactions, a keyword the chat already has, the cap reached. The reaction is then claimed and stored through
// applyReactionAdd without overwrite; anything but an added reaction gives the claim back.
func settingsReactionsTyped(r *settingsInputReply, _ settingsListPart) {
	fields := strings.Fields(r.msg.GetText())
	if len(fields) == 0 {
		r.refuse(r.text("settings_edit_refuse_need_text"))
		return
	}
	if len(fields) != 2 {
		r.refuse(r.text("settings_rct_refuse_two"))
		return
	}
	keyword := strings.ToLower(fields[0])
	if utf8.RuneCountInString(keyword) > settingsReactionKeywordMax {
		r.refuse(r.text("settings_rct_refuse_long", i18n.TranslationParams{"max": settingsReactionKeywordMax}))
		return
	}
	claim, claimed := r.claimWait()
	if !claimed {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), settingsReadTimeout)
	defer cancel()
	result := applyReactionAdd(ctx, r.wait.GroupID, keyword, fields[1], false)
	if result.Outcome != reactionAdded {
		r.releaseWait(claim)
		switch result.Outcome {
		case reactionBadEmoji:
			r.refuse(r.text("settings_rct_refuse_emoji", i18n.TranslationParams{"examples": settingsReactionExamples}))
		case reactionExists:
			r.refuse(settingsListEntryText(r.tr, "settings_rct_refuse_exists", "keyword", keyword))
		case reactionCapReached:
			r.refuse(settingsReactionsCapText(r.tr))
		default:
			r.keep("common_settings_save_failed")
		}
		return
	}
	r.finishWait(claim)
	log.Infof("[Settings] user %d added a reaction to chat %d", r.user.Id, r.wait.GroupID)
	notice := settingsNoticeOK(settingsListEntryText(r.tr, "settings_rct_saved", "keyword", keyword))
	if settingsInputIsAlbum(r.msg) {
		notice += "\n" + settingsText(r.tr, "settings_edit_album_cut")
	}
	r.sendSavedList(*r.list, keyword, notice)
	r.closePrompt()
}

// settingsReactionsDef is the table of the Reactions page.
func settingsReactionsDef() settingsListDef {
	return settingsListDef{
		Key:   settingsPageReactions,
		Order: 73,
		// 👍, not the plan's 🎭: the Anonymous admins page already shows 🎭 on Main.
		Icon:      "👍",
		TitleKey:  "settings_page_reactions",
		PagerCode: "xg",
		AskCode:   "xx",
		YesCode:   "xy",
		// Every reaction write declares the right /addreaction and /removereaction ask for.
		Right:        settingsRightChangeInfo,
		CountMax:     reactionsCapMax,
		Remove:       settingsReactionsRemove,
		DeleteAskKey: "settings_rct_delete_ask",
		RemovedKey:   "settings_rct_removed",
		EntryParam:   "keyword",
		Read: func(ctx context.Context, chatID int64) ([]settingsListEntry, error) {
			return settingsReactionsRead(ctx, chatID)
		},
		Entry: func(ctx context.Context, chatID, id int64) (*settingsListEntry, error) {
			return settingsReactionsEntry(ctx, chatID, id)
		},
		Count:      func(ctx context.Context, chatID int64) (int, error) { return settingsReactionsCount(ctx, chatID) },
		IntroKey:   "settings_rct_intro",
		CountKey:   "settings_rct_count",
		EmptyKey:   "settings_rct_empty",
		AddCode:    "xa",
		CancelCode: "xc",
		AddKey:     "settings_btn_rct_add",
		Prompt:     settingsReactionsPrompt,
		AddGate:    settingsReactionsAddGate,
		Typed:      settingsReactionsTyped,
	}
}

func init() {
	registerSettingsList(settingsReactionsDef())
}
