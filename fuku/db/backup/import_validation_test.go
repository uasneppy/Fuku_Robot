package backup

import (
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/chats"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// TestImportNeverTakesTheLogChannel proves that an import never links the target group to the log channel the
// backup names: with no log channel in the target nothing is stored and the report says the categories were
// left out, and with one the target keeps its own channel and takes the backup's category flags.
func TestImportNeverTakesTheLogChannel(t *testing.T) {
	skipIfNoDb(t)
	payload := map[string]any{"settings": map[string]any{
		"chat_id": 1, "log_channel_id": -1009999999999,
		"cat_settings": false, "cat_admin": true, "cat_user": false, "cat_reports": true,
	}}

	t.Run("no log channel in the target", func(t *testing.T) {
		chatID := time.Now().UnixNano()
		require.NoError(t, chats.EnsureChatInDb(chatID, "import_log_none"))
		t.Cleanup(func() { cleanupBackupChat(t, chatID) })

		bkp := NewBackupFormat(chatID, "x", 1, []string{BackupModuleLogChannels})
		bkp.Data[BackupModuleLogChannels] = payload
		report, err := ImportChatDataReport(chatID, bkp, nil)
		require.NoError(t, err)
		assert.True(t, report.LogChannelSkipped, "the report says the categories were left out")
		var count int64
		require.NoError(t, db.DB.Model(&models.LogChannel{}).Where("chat_id = ?", chatID).Count(&count).Error)
		assert.Zero(t, count, "an import created a log channel link")
	})

	t.Run("the target has its own log channel", func(t *testing.T) {
		chatID := time.Now().UnixNano()
		own := -chatID
		require.NoError(t, chats.EnsureChatInDb(chatID, "import_log_own"))
		t.Cleanup(func() { cleanupBackupChat(t, chatID) })
		require.NoError(t, db.DB.Create(&models.LogChannel{ChatID: chatID, LogChannelID: own, CatSettings: true,
			CatAdmin: false, CatUser: true, CatReports: false, CatAutomated: true, CatOther: true}).Error)

		bkp := NewBackupFormat(chatID, "x", 1, []string{BackupModuleLogChannels})
		bkp.Data[BackupModuleLogChannels] = payload
		report, err := ImportChatDataReport(chatID, bkp, nil)
		require.NoError(t, err)
		assert.False(t, report.LogChannelSkipped)
		var row models.LogChannel
		require.NoError(t, db.DB.Where("chat_id = ?", chatID).First(&row).Error)
		assert.Equal(t, own, row.LogChannelID, "the target's own log channel is kept")
		assert.False(t, row.CatSettings)
		assert.True(t, row.CatAdmin)
		assert.False(t, row.CatUser)
		assert.True(t, row.CatReports)
	})
}

// TestImportSkipsInvalidListEntries proves that imported filters, blacklist words and note names get the
// checks the commands make: a keyword or word that is too broad or too long, a note name that could never be
// fetched and an entry with a button link that is not http or https are left out and counted, names and
// words are stored lower-cased, and a repeat after lower-casing is left out too.
func TestImportSkipsInvalidListEntries(t *testing.T) {
	skipIfNoDb(t)
	chatID := time.Now().UnixNano()
	require.NoError(t, chats.EnsureChatInDb(chatID, "import_lists"))
	t.Cleanup(func() { cleanupBackupChat(t, chatID) })

	long := strings.Repeat("w", 101)
	modules := []string{BackupModuleFilters, BackupModuleBlacklists, BackupModuleNotes}
	bkp := NewBackupFormat(chatID, "x", 1, modules)
	bkp.Data[BackupModuleFilters] = map[string]any{"filters": []any{
		map[string]any{"keyword": "Hello There", "filter_reply": "hi", "msgtype": 1},
		map[string]any{"keyword": "a", "filter_reply": "too broad", "msgtype": 1},
		map[string]any{"keyword": long, "filter_reply": "too long", "msgtype": 1},
		map[string]any{"keyword": "bad link", "filter_reply": "x", "msgtype": 1,
			"filter_buttons": []any{map[string]any{"name": "go", "url": "javascript:alert(1)"}}},
	}}
	bkp.Data[BackupModuleBlacklists] = map[string]any{"entries": []any{
		map[string]any{"word": "Spam", "action": "warn"},
		map[string]any{"word": "x", "action": "warn"},
		map[string]any{"word": "SPAM", "action": "ban"},
		map[string]any{"word": long, "action": "warn"},
	}}
	bkp.Data[BackupModuleNotes] = map[string]any{"notes": []any{
		map[string]any{"note_name": "Rules", "note_content": "be kind", "msg_type": 1},
		map[string]any{"note_name": "#bad", "note_content": "x", "msg_type": 1},
		map[string]any{"note_name": "two words", "note_content": "x", "msg_type": 1},
		map[string]any{"note_name": "rules", "note_content": "repeat", "msg_type": 1},
	}}

	report, err := ImportChatDataReport(chatID, bkp, nil)
	require.NoError(t, err)
	assert.Equal(t, 9, report.Skipped, "entries left out")

	var keywords, words, names []string
	require.NoError(t, db.DB.Model(&models.ChatFilters{}).Where("chat_id = ?", chatID).Pluck("keyword", &keywords).Error)
	require.NoError(t, db.DB.Model(&models.BlacklistSettings{}).Where("chat_id = ?", chatID).Pluck("word", &words).Error)
	require.NoError(t, db.DB.Model(&models.Notes{}).Where("chat_id = ?", chatID).Pluck("note_name", &names).Error)
	sort.Strings(keywords)
	assert.Equal(t, []string{"hello there"}, keywords)
	assert.Equal(t, []string{"spam"}, words)
	assert.Equal(t, []string{"rules"}, names)
}
