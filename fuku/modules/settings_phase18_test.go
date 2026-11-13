//go:build testtools

package modules

import (
	"context"
	"strings"
	"testing"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/logchannels"
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

// TestSettingsPhase18InStaffGroup: none of the six pages is Hidden, so in a chat that is a Staff Group
// Main lists them and each opens and writes exactly as in any group (D-11). The Deletion page is the
// one page a Staff Group does not have, which proves the group really is a Staff Group here.
func TestSettingsPhase18InStaffGroup(t *testing.T) {
	settingsLangWithCodes(t, settingsLangDefaultCodes)
	env := newSettingsDelEnv(t)
	editCleanup(t, env.chat.Id)
	settingsWarnsCleanup(t, env.chat.Id)
	settingsSwCleanup(t, env.chat.Id)
	settingsAnonCleanup(t, env.chat.Id)
	lockGoldenCleanup(t, env.chat.Id)
	settingsLangCleanup(t, []int64{env.chat.Id}, nil)
	env.makeStaff()
	settingsLogLink(t, env.chat)

	m := env.groupMenu()
	pages := []string{settingsPageWarns, settingsPageLocks, settingsPageSwitches, settingsPageLang, settingsPageLog, settingsPageAnon}
	for _, key := range pages {
		if _, ok := settingsPageButton(t, m.mainKeyboard, key); !ok {
			t.Errorf("a Staff Group's Main has no cell for page %s", key)
		}
	}
	if _, ok := settingsPageButton(t, m.mainKeyboard, settingsPageDeletion); ok {
		t.Error("the chat is not a Staff Group: its Main still has the Deletion cell")
	}
	if got := len(settingsMainLines(m.mainText)); got != len(settingsMainTitleKeys)-1 {
		t.Errorf("a Staff Group's Main has %d state lines, want %d (every page but Deletion)", got, len(settingsMainTitleKeys)-1)
	}
	for _, key := range []string{
		"settings_page_warns", "settings_page_locks", "settings_page_switches",
		"settings_page_lang", "settings_page_log", "settings_page_anon",
	} {
		line, ok := settingsMainStateLine(m.mainText, key)
		if !ok || strings.HasSuffix(line, settingsUnknownState) {
			t.Errorf("Main line of %s = %q (found %v), want a state", key, line, ok)
		}
	}

	// Each page opens, and a permitted press writes through the page's own apply function.
	for _, key := range pages {
		m.press("a", "op", "p", key)
		if text, _ := m.last(); containsMarker(text, "settings_cannot_open") || containsMarker(text, "common_callback_invalid_request") {
			t.Errorf("page %s in a Staff Group drew %q", key, text)
		}
	}
	m.press("a", "st", "p", settingsPageWarns, "t", settingsWarnsLimitKey, "v", "7")
	if row, ok := settingsWarnsRow(t, env.chat.Id); !ok || row.WarnLimit != 7 {
		t.Errorf("warn settings = %+v (stored %v), want limit 7", row, ok)
	}
	m.press("a", "tg", "p", settingsPageSwitches, "t", settingsSwReportsKey, "v", "0")
	if row, ok := settingsSwReportsRow(t, env.chat.Id); !ok || row.Enabled {
		t.Errorf("report settings = %+v (stored %v), want Reports off", row, ok)
	}
	m.press("a", "st", "p", settingsPageLang, "t", settingsLangControlKey, "v", "en")
	if got, ok := settingsLangStored(t, env.chat.Id); !ok || got != "en" {
		t.Errorf("group language = %q (stored %v), want en", got, ok)
	}
	m.press("a", "tg", "p", settingsPageLog, "t", "a", "v", "0")
	if row := logchannels.GetContext(context.Background(), env.chat.Id); row == nil || row.CatAdmin {
		t.Errorf("log channel row = %+v, want the admin category off", row)
	}
	m.press("a", "tg", "p", settingsPageAnon, "t", settingsAnonControl, "v", "1")
	if on, err := settingsAnonRead(context.Background(), env.chat.Id); err != nil || !on {
		t.Errorf("anonymous admins = %v (%v), want on", on, err)
	}
	m.press("a", "tg", "p", settingsPageLocks, "t", settingsLocksCode(t, "gif"), "v", "1")
	if got := settingsLocksStored(t, env.chat.Id); got != "gif=true" {
		t.Errorf("locks stored %q, want gif=true", got)
	}
}
