package modules

import (
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/db/staff"
	"github.com/divkix/Alita_Robot/alita/i18n"
)

const (
	// staffHistoryPageSize is how many entries one page of the history shows.
	staffHistoryPageSize = 10
	// staffHistoryNameRunes caps a target name or issuer name in one line.
	staffHistoryNameRunes = 16
	// staffHistoryReasonRunes caps a reason in one line.
	staffHistoryReasonRunes = 24
	// staffHistoryMaxOffset is the largest offset a callback may carry.
	staffHistoryMaxOffset = 1_000_000
)

// staffHistoryLine is not implemented yet.
func staffHistoryLine(tr *i18n.Translator, index int, a *models.StaffAction, t staff.ActionTally, now time.Time) string {
	return ""
}

// renderStaffHistory is not implemented yet.
func renderStaffHistory(
	tr *i18n.Translator,
	actions []models.StaffAction,
	tallies map[uint]staff.ActionTally,
	offset int,
	hasMore bool,
	now time.Time,
) (string, gotgbot.InlineKeyboardMarkup, int) {
	return "", gotgbot.InlineKeyboardMarkup{}, 0
}

// staffRecentButton is not implemented yet.
func staffRecentButton(tr *i18n.Translator) (gotgbot.InlineKeyboardButton, bool) {
	return gotgbot.InlineKeyboardButton{}, false
}
