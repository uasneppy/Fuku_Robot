package backup

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/chats"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// stripNewGreetingKeys removes from a decoded backup the keys this phase added to the
// greetings and rules objects, which is exactly what an export made before it looks like.
func stripNewGreetingKeys(t *testing.T, raw []byte) []byte {
	t.Helper()
	var doc map[string]any
	require.NoError(t, json.Unmarshal(raw, &doc))
	data, ok := doc["data"].(map[string]any)
	require.True(t, ok, "the export has a data object")

	object := func(parent map[string]any, key string) map[string]any {
		child, ok := parent[key].(map[string]any)
		require.True(t, ok, "the export has an object %q", key)
		return child
	}
	drop := func(obj map[string]any, keys ...string) {
		for _, key := range keys {
			_, present := obj[key]
			require.True(t, present, "the new export carries %q", key)
			delete(obj, key)
		}
	}

	greetingsSettings := object(object(data, BackupModuleGreetings), "settings")
	drop(object(greetingsSettings, "welcome_settings"), "media_below", "once")
	drop(object(greetingsSettings, "goodbye_settings"), "media_below")
	drop(object(object(data, BackupModuleRules), "settings"), "file_id", "media_type", "btns", "media_below")

	out, err := json.Marshal(doc)
	require.NoError(t, err)
	return out
}

// TestOldGreetingsExportDecodesToZeros proves D-13: an export made before Phase 12 has none
// of the new keys, and importing it leaves every new column at its zero value, the old
// behaviour, even over a destination chat that had them all set.
func TestOldGreetingsExportDecodesToZeros(t *testing.T) {
	skipIfNoDb(t)

	srcChat := time.Now().UnixNano()
	dstChat := srcChat + 1
	require.NoError(t, chats.EnsureChatInDb(srcChat, "old_export_source"))
	require.NoError(t, chats.EnsureChatInDb(dstChat, "old_export_destination"))
	t.Cleanup(func() {
		cleanupBackupChat(t, srcChat)
		cleanupBackupChat(t, dstChat)
	})

	buttons := models.ButtonArray{{Name: "docs", Url: "https://example.com"}}
	for _, chatID := range []int64{srcChat, dstChat} {
		require.NoError(t, db.DB.Create(&models.GreetingSettings{
			ChatID: chatID,
			WelcomeSettings: &models.WelcomeSettings{
				ShouldWelcome: true, WelcomeText: "welcome", MediaBelow: true, Once: true,
			},
			GoodbyeSettings: &models.GoodbyeSettings{
				ShouldGoodbye: true, GoodbyeText: "goodbye", MediaBelow: true,
			},
		}).Error)
		require.NoError(t, db.DB.Create(&models.RulesSettings{
			ChatId: chatID, Rules: "be kind",
			FileID: "rules-file", MediaType: 4, Buttons: buttons, MediaBelow: true,
		}).Error)
	}

	exported, err := ExportChatData(srcChat, "source", 42, nil)
	require.NoError(t, err)
	raw, err := exported.ToJSON()
	require.NoError(t, err)

	decoded, err := BackupFormatFromJSON(stripNewGreetingKeys(t, raw))
	require.NoError(t, err)
	require.NoError(t, ImportChatData(dstChat, decoded, nil))

	greetingsData, err := exportGreetingsData(dstChat)
	require.NoError(t, err)
	require.NotNil(t, greetingsData.Settings)
	require.NotNil(t, greetingsData.Settings.WelcomeSettings)
	require.NotNil(t, greetingsData.Settings.GoodbyeSettings)
	assert.Equal(t, "welcome", greetingsData.Settings.WelcomeSettings.WelcomeText)
	assert.False(t, greetingsData.Settings.WelcomeSettings.MediaBelow)
	assert.False(t, greetingsData.Settings.WelcomeSettings.Once)
	assert.False(t, greetingsData.Settings.GoodbyeSettings.MediaBelow)

	rulesData, err := exportRulesData(dstChat)
	require.NoError(t, err)
	require.NotNil(t, rulesData.Settings)
	assert.Equal(t, "be kind", rulesData.Settings.Rules)
	assert.Empty(t, rulesData.Settings.FileID)
	assert.Zero(t, rulesData.Settings.MediaType)
	assert.Empty(t, rulesData.Settings.Buttons)
	assert.False(t, rulesData.Settings.MediaBelow)
}

// TestGreetedMembersStayOutOfBackup proves D-13: the greet-once record is never a backup
// module, so export carries none of it, import writes none into the destination chat, and
// reset leaves the source chat's rows alone.
func TestGreetedMembersStayOutOfBackup(t *testing.T) {
	skipIfNoDb(t)

	srcChat := time.Now().UnixNano()
	dstChat := srcChat + 1
	require.NoError(t, chats.EnsureChatInDb(srcChat, "greeted_source"))
	require.NoError(t, chats.EnsureChatInDb(dstChat, "greeted_destination"))
	t.Cleanup(func() {
		cleanupBackupChat(t, srcChat)
		cleanupBackupChat(t, dstChat)
		require.NoError(t, db.DB.Where("chat_id IN ?", []int64{srcChat, dstChat}).Delete(&models.GreetedMember{}).Error)
	})

	for _, userID := range []int64{101, 102} {
		require.NoError(t, db.DB.Create(&models.GreetedMember{ChatID: srcChat, UserID: userID}).Error)
	}
	countRows := func(chatID int64) int64 {
		t.Helper()
		var n int64
		require.NoError(t, db.DB.Model(&models.GreetedMember{}).Where("chat_id = ?", chatID).Count(&n).Error)
		return n
	}

	exported, err := ExportChatData(srcChat, "source", 42, nil)
	require.NoError(t, err)
	assert.Len(t, exported.Data, len(AllExportableModules()))
	for module := range exported.Data {
		assert.True(t, IsValidModule(module), "export holds only backup modules, got %q", module)
	}
	_, err = ExportChatData(srcChat, "source", 42, []string{"greeted_members"})
	assert.Error(t, err, "greeted_members is not a backup module")

	raw, err := exported.ToJSON()
	require.NoError(t, err)
	decoded, err := BackupFormatFromJSON(raw)
	require.NoError(t, err)
	require.NoError(t, ImportChatData(dstChat, decoded, nil))
	assert.Zero(t, countRows(dstChat), "an import creates no greeted_members row")

	require.NoError(t, ClearChatData(srcChat, nil))
	assert.EqualValues(t, 2, countRows(srcChat), "a reset leaves the greet-once record in place")
}
