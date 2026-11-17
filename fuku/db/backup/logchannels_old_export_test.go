//go:build testtools

package backup

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/chats"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// TestImportLogChannelsOldExportKeepsAllSixFlags proves an export made while six log categories
// existed still imports after two of them stopped being offered: the log channel row is decoded
// whole, so the two columns nothing reads any more keep the values the export carried.
func TestImportLogChannelsOldExportKeepsAllSixFlags(t *testing.T) {
	skipIfNoDb(t)

	chatID := time.Now().UnixNano()
	channelID := -chatID
	require.NoError(t, chats.EnsureChatInDb(chatID, "test_import_old_log_export"))
	t.Cleanup(func() { cleanupBackupChat(t, chatID) })

	raw := fmt.Sprintf(`{"version":"1.0","bot_name":"FukuRobot","chat_id":%d,"chat_name":"Test","exported_by":1,`+
		`"modules":["logchannels"],"data":{"logchannels":{"settings":{"chat_id":%d,"log_channel_id":%d,`+
		`"cat_settings":true,"cat_admin":false,"cat_user":true,"cat_automated":false,"cat_reports":true,"cat_other":false}}},`+
		`"exported_at":"2024-01-01T00:00:00Z"}`, chatID, chatID, channelID)
	backup, err := BackupFormatFromJSON([]byte(raw))
	require.NoError(t, err)
	require.NoError(t, backup.Validate())
	require.NoError(t, ImportChatData(chatID, backup, []string{BackupModuleLogChannels}))

	var row models.LogChannel
	require.NoError(t, db.DB.Where("chat_id = ?", chatID).First(&row).Error)
	assert.Equal(t, channelID, row.LogChannelID)
	assert.True(t, row.CatSettings)
	assert.False(t, row.CatAdmin)
	assert.True(t, row.CatUser)
	assert.True(t, row.CatReports)
	assert.False(t, row.CatAutomated, "the old automated flag keeps its stored value")
	assert.False(t, row.CatOther, "the old other flag keeps its stored value")
}
