package modules

import (
	"context"
	"html"

	"github.com/uasneppy/fuku_robot/fuku/db/blacklists"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
)

// The Blacklist page of the settings menu (page b, Phase 19) is a table of the list engine
// (settings_list.go) plus the action screen. It lists the chat's blacklisted words eight a page, deletes one
// behind a two-tap card that binds the row, and sets the action every word carries through
// applyBlacklistAction, the function /blacklistaction uses. It reads through the fresh, chat-scoped,
// non-creating readers of the blacklists repository, never through GetBlacklistSettingsContext, which hides
// its read error. It is private-only: a menu opened in the group shows only the framework's note.

const (
	// settingsPageBlacklist is the p value of the Blacklist page.
	settingsPageBlacklist = "b"
	// settingsBlacklistActionMaxRunes caps a stored action value that has no option word when it is shown.
	settingsBlacklistActionMaxRunes = 16
)

// The reads of the page, variables only so a test can count or fail them. Each adapts the repository's
// row to the engine's entry.
var (
	// settingsBlacklistRead lists the chat's words with their actions.
	settingsBlacklistRead = func(ctx context.Context, chatID int64) ([]settingsListEntry, error) {
		rows, err := blacklists.ListBlacklistEntriesFresh(ctx, chatID)
		if err != nil {
			return nil, err
		}
		entries := make([]settingsListEntry, 0, len(rows))
		for _, row := range rows {
			entries = append(entries, settingsBlacklistEntryOf(row))
		}
		return entries, nil
	}
	// settingsBlacklistEntry reads one word by ID, only when it belongs to the chat.
	settingsBlacklistEntry = func(ctx context.Context, chatID, id int64) (*settingsListEntry, error) {
		row, err := blacklists.GetBlacklistEntryFresh(ctx, chatID, id)
		if err != nil || row == nil {
			return nil, err
		}
		entry := settingsBlacklistEntryOf(*row)
		return &entry, nil
	}
	// settingsBlacklistCount counts the chat's words.
	settingsBlacklistCount = func(ctx context.Context, chatID int64) (int, error) {
		n, err := blacklists.CountBlacklistFresh(ctx, chatID)
		return int(n), err
	}
)

// settingsBlacklistEntryOf is the engine's entry of a stored word.
func settingsBlacklistEntryOf(row models.BlacklistSettings) settingsListEntry {
	return settingsListEntry{ID: int64(row.ID), Text: row.Word, Action: row.Action}
}

// settingsBlacklistActionWord is the word of a stored action: the four option words of the commands, the
// Delete only word for none, and any other stored value (tban, tmute, delete) as stored, capped and escaped.
func settingsBlacklistActionWord(tr *i18n.Translator, action string) string {
	switch action {
	case "warn":
		return settingsText(tr, "settings_opt_warn")
	case "mute":
		return settingsText(tr, "settings_opt_mute")
	case "kick":
		return settingsText(tr, "settings_opt_kick")
	case "ban":
		return settingsText(tr, "settings_opt_ban")
	case "none":
		return settingsText(tr, "settings_bl_opt_none")
	}
	runes := []rune(action)
	if len(runes) > settingsBlacklistActionMaxRunes {
		action = string(runes[:settingsBlacklistActionMaxRunes])
	}
	return html.EscapeString(action)
}

// settingsBlacklistUniformAction returns the one action every word has, and false when the words differ or
// there are none: the page claims a single action only when it is true of every word (C-02).
func settingsBlacklistUniformAction(entries []settingsListEntry) (string, bool) {
	if len(entries) == 0 {
		return "", false
	}
	for _, e := range entries[1:] {
		if e.Action != entries[0].Action {
			return "", false
		}
	}
	return entries[0].Action, true
}

// settingsBlacklistActionLine is the action line of the list page and of the action screen: the action when
// every word has it, Mixed when they differ, no line for an empty list.
func settingsBlacklistActionLine(v settingsView, entries []settingsListEntry) []string {
	if len(entries) == 0 {
		return nil
	}
	word := settingsText(v.Tr, "settings_bl_action_mixed")
	if action, ok := settingsBlacklistUniformAction(entries); ok {
		word = settingsBlacklistActionWord(v.Tr, action)
	}
	return []string{settingsText(v.Tr, "settings_bl_action_line", i18n.TranslationParams{"action": word})}
}

// settingsBlacklistDef is the table of the Blacklist page.
func settingsBlacklistDef() settingsListDef {
	return settingsListDef{
		Key:       settingsPageBlacklist,
		Order:     46,
		Icon:      "🔤",
		TitleKey:  "settings_page_blacklist",
		PagerCode: "bg",
		Read: func(ctx context.Context, chatID int64) ([]settingsListEntry, error) {
			return settingsBlacklistRead(ctx, chatID)
		},
		Entry: func(ctx context.Context, chatID, id int64) (*settingsListEntry, error) {
			return settingsBlacklistEntry(ctx, chatID, id)
		},
		Count:     func(ctx context.Context, chatID int64) (int, error) { return settingsBlacklistCount(ctx, chatID) },
		IntroKey:  "settings_bl_intro",
		CountKey:  "settings_bl_count",
		EmptyKey:  "settings_bl_empty",
		BodyExtra: settingsBlacklistActionLine,
	}
}

func init() {
	registerSettingsList(settingsBlacklistDef())
}
