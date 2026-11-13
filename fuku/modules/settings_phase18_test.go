//go:build testtools

package modules

import (
	"testing"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// settingsPhase18Tables are the tables a settings page reads: the nine of the Phase 18 readers and
// of the Main status block, each with a chat_id column. A read of Main or of a page creates a row in
// none of them.
var settingsPhase18Tables = []struct {
	name  string
	model any
}{
	{"warn_settings", &models.WarnSettings{}},
	{"report_chat_settings", &models.ReportChatSettings{}},
	{"ai_spam_settings", &models.AISpamSettings{}},
	{"greetings", &models.GreetingSettings{}},
	{"chats", &models.Chat{}},
	{"admin", &models.AdminSettings{}},
	{"log_channels", &models.LogChannel{}},
	{"locks", &models.LockSettings{}},
	{"rules", &models.RulesSettings{}},
}

// settingsPhase18RowCounts counts the chat's rows in each table of settingsPhase18Tables, by name,
// for the tables that have any.
func settingsPhase18RowCounts(t *testing.T, chatID int64) map[string]int64 {
	t.Helper()
	counts := make(map[string]int64)
	for _, table := range settingsPhase18Tables {
		var n int64
		if err := db.DB.Model(table.model).Where("chat_id = ?", chatID).Count(&n).Error; err != nil {
			t.Fatalf("count %s rows of %d: %v", table.name, chatID, err)
		}
		if n != 0 {
			counts[table.name] = n
		}
	}
	return counts
}

// settingsPhase18RowCount is how many rows the chat has in all of settingsPhase18Tables together.
func settingsPhase18RowCount(t *testing.T, chatID int64) int64 {
	t.Helper()
	var total int64
	for _, n := range settingsPhase18RowCounts(t, chatID) {
		total += n
	}
	return total
}
