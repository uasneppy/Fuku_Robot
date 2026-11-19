package backup

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/chats"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// TestChatConversationsStayOutOfBackup proves that the conversation setting is never a backup
// module: export carries none of it, an import writes no row into the destination chat (a
// restored backup can never switch conversations on), and a reset leaves the source row alone.
func TestChatConversationsStayOutOfBackup(t *testing.T) {
	skipIfNoDb(t)

	srcChat := time.Now().UnixNano()
	dstChat := srcChat + 1
	require.NoError(t, chats.EnsureChatInDb(srcChat, "conv_source"))
	require.NoError(t, chats.EnsureChatInDb(dstChat, "conv_destination"))
	t.Cleanup(func() {
		cleanupBackupChat(t, srcChat)
		cleanupBackupChat(t, dstChat)
		require.NoError(t, db.DB.Where("chat_id IN ?", []int64{srcChat, dstChat}).Delete(&models.ChatConversation{}).Error)
	})
	require.NoError(t, db.DB.Create(&models.ChatConversation{ChatID: srcChat, Enabled: true, Provider: "google", Audience: 1}).Error)

	countRows := func(chatID int64) int64 {
		var n int64
		require.NoError(t, db.DB.Model(&models.ChatConversation{}).Where("chat_id = ?", chatID).Count(&n).Error)
		return n
	}

	exported, err := ExportChatData(srcChat, "source", 42, nil)
	require.NoError(t, err)
	for module := range exported.Data {
		assert.True(t, IsValidModule(module), "export holds only backup modules, got %q", module)
	}
	for _, name := range []string{"chat_conversations", "chat_conv", "conversations"} {
		_, err = ExportChatData(srcChat, "source", 42, []string{name})
		assert.Error(t, err, "%s is not a backup module", name)
	}
	raw, err := exported.ToJSON()
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "chat_conversations", "the export file names no conversation table")
	decoded, err := BackupFormatFromJSON(raw)
	require.NoError(t, err)
	require.NoError(t, ImportChatData(dstChat, decoded, nil))
	assert.Zero(t, countRows(dstChat), "an import creates no chat_conversations row")

	require.NoError(t, ClearChatData(srcChat, nil))
	assert.EqualValues(t, 1, countRows(srcChat), "a reset leaves the conversation setting in place")
}
