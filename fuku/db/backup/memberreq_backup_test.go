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

// TestMemberRequirementsStayOutOfBackup proves D-17: neither the member requirements setting
// nor the holds are a backup module, so export carries none of them, import writes none into
// the destination chat (a backup file can never widen what a group screens, or carry a hold
// that mutes or unmutes a member), and reset leaves the source chat's rows alone.
func TestMemberRequirementsStayOutOfBackup(t *testing.T) {
	skipIfNoDb(t)

	srcChat := time.Now().UnixNano()
	dstChat := srcChat + 1
	require.NoError(t, chats.EnsureChatInDb(srcChat, "requirements_source"))
	require.NoError(t, chats.EnsureChatInDb(dstChat, "requirements_destination"))
	t.Cleanup(func() {
		cleanupBackupChat(t, srcChat)
		cleanupBackupChat(t, dstChat)
		ids := []int64{srcChat, dstChat}
		require.NoError(t, db.DB.Where("chat_id IN ?", ids).Delete(&models.ChatRequirements{}).Error)
		require.NoError(t, db.DB.Where("chat_id IN ?", ids).Delete(&models.ChatRequirementHold{}).Error)
	})

	require.NoError(t, db.DB.Create(&models.ChatRequirements{
		ChatID: srcChat, LastNameAction: models.RequirementActionMute, CheckOnJoin: true, DeleteMessages: true,
		// The Phase 14 lookup rules: their columns stay out of a backup like the rest of the row.
		PhotoAction: models.RequirementActionKick, ChannelAction: models.RequirementActionMute,
		// The Phase 15 AI spam-name rule: its column stays out of a backup too.
		AIAction: models.RequirementActionBan,
	}).Error)
	require.NoError(t, db.DB.Create(&models.ChatRequirementHold{
		ChatID: srcChat, UserID: 7001, Rules: 1, Action: models.RequirementActionWarn, CreatedAt: time.Now(),
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
	for _, name := range []string{"chat_requirements", "chat_requirement_holds", "member_requirements"} {
		_, err = ExportChatData(srcChat, "source", 42, []string{name})
		assert.Error(t, err, "%s is not a backup module", name)
		_, named := exported.Data[name]
		assert.False(t, named, "the export has no %s key", name)
	}

	raw, err := exported.ToJSON()
	require.NoError(t, err)
	decoded, err := BackupFormatFromJSON(raw)
	require.NoError(t, err)
	for _, column := range []string{"ai_action", "photo_action", "channel_action", "last_name_action", "chat_requirements"} {
		assert.NotContains(t, string(raw), column, "the export file names no requirement column or table (%s)", column)
	}
	require.NoError(t, ImportChatData(dstChat, decoded, nil))
	assert.Zero(t, countRows(&models.ChatRequirements{}, dstChat), "an import creates no chat_requirements row")
	assert.Zero(t, countRows(&models.ChatRequirementHold{}, dstChat), "an import creates no chat_requirement_holds row")

	require.NoError(t, ClearChatData(srcChat, nil))
	assert.EqualValues(t, 1, countRows(&models.ChatRequirements{}, srcChat), "a reset leaves the requirement setting in place")
	assert.EqualValues(t, 1, countRows(&models.ChatRequirementHold{}, srcChat), "a reset leaves the holds in place")
	var kept models.ChatRequirements
	require.NoError(t, db.DB.Where("chat_id = ?", srcChat).First(&kept).Error)
	assert.Equal(t, models.RequirementActionKick, kept.PhotoAction, "a reset leaves the photo rule as it was")
	assert.Equal(t, models.RequirementActionMute, kept.ChannelAction, "a reset leaves the channel rule as it was")
	assert.Equal(t, models.RequirementActionBan, kept.AIAction, "a reset leaves the AI rule as it was")
}
