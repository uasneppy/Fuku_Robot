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

// TestDeletionTablesStayOutOfBackup proves D-01 and D-05: neither the deletion rules nor the
// deletion queue is a backup module, so export carries none of them, import writes none into
// the destination chat (a backup file can never revive a rule or a queued deletion), and
// reset leaves the source chat's rows alone. The shared /cleanservice flag travels in the
// greetings module as before; the delay rows count only while it is on.
func TestDeletionTablesStayOutOfBackup(t *testing.T) {
	skipIfNoDb(t)

	srcChat := time.Now().UnixNano()
	dstChat := srcChat + 1
	require.NoError(t, chats.EnsureChatInDb(srcChat, "deletion_source"))
	require.NoError(t, chats.EnsureChatInDb(dstChat, "deletion_destination"))
	t.Cleanup(func() {
		cleanupBackupChat(t, srcChat)
		cleanupBackupChat(t, dstChat)
		ids := []int64{srcChat, dstChat}
		require.NoError(t, db.DB.Where("chat_id IN ?", ids).Delete(&models.DeleteRule{}).Error)
		require.NoError(t, db.DB.Where("chat_id IN ?", ids).Delete(&models.ScheduledDeletion{}).Error)
	})

	now := time.Now().UTC().Truncate(time.Second)
	require.NoError(t, db.DB.Create(&models.DeleteRule{ChatID: srcChat, Kind: "service:pinned_message", DelaySecs: 777}).Error)
	require.NoError(t, db.DB.Create(&models.ScheduledDeletion{
		ChatID: srcChat, MessageID: 4242, Kind: "bot:welcome", SentAt: now, DeleteAt: now.Add(time.Hour),
		State: models.ScheduledDeletionPending, CreatedAt: now,
	}).Error)

	countRows := func(model any, chatID int64) int64 {
		t.Helper()
		var n int64
		require.NoError(t, db.DB.Model(model).Where("chat_id = ?", chatID).Count(&n).Error)
		return n
	}

	exported, err := ExportChatData(srcChat, "source", 42, nil)
	require.NoError(t, err)
	assert.Len(t, exported.Data, len(AllExportableModules()))
	for module := range exported.Data {
		assert.True(t, IsValidModule(module), "export holds only backup modules, got %q", module)
	}
	for _, name := range []string{"chat_delete_rules", "scheduled_deletions", "delete_rules"} {
		_, err = ExportChatData(srcChat, "source", 42, []string{name})
		assert.Error(t, err, "%s is not a backup module", name)
		_, named := exported.Data[name]
		assert.False(t, named, "the export has no %s key", name)
	}

	raw, err := exported.ToJSON()
	require.NoError(t, err)
	decoded, err := BackupFormatFromJSON(raw)
	require.NoError(t, err)
	for _, name := range []string{"chat_delete_rules", "scheduled_deletions", "delay_secs", "service:pinned_message"} {
		assert.NotContains(t, string(raw), name, "the export file names no deletion table or column (%s)", name)
	}
	require.NoError(t, ImportChatData(dstChat, decoded, nil))
	assert.Zero(t, countRows(&models.DeleteRule{}, dstChat), "an import creates no chat_delete_rules row")
	assert.Zero(t, countRows(&models.ScheduledDeletion{}, dstChat), "an import creates no scheduled_deletions row")

	require.NoError(t, ClearChatData(srcChat, nil))
	assert.EqualValues(t, 1, countRows(&models.DeleteRule{}, srcChat), "a reset leaves the deletion rules in place")
	assert.EqualValues(t, 1, countRows(&models.ScheduledDeletion{}, srcChat), "a reset leaves the queue in place")
	var kept models.DeleteRule
	require.NoError(t, db.DB.Where("chat_id = ?", srcChat).First(&kept).Error)
	assert.Equal(t, 777, kept.DelaySecs, "a reset leaves the delay as it was")
}
