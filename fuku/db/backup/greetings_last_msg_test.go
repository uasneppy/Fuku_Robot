package backup

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/chats"
)

// TestGreetingsBackupDropsLastMessageIDs proves that the IDs of the last welcome and goodbye a group got
// never travel in a backup: an export leaves them out and an import stores zero, so "Delete last" in the
// target group never deletes a message ID that belongs to the source group.
func TestGreetingsBackupDropsLastMessageIDs(t *testing.T) {
	skipIfNoDb(t)

	source := time.Now().UnixNano()
	target := source + 1
	for _, id := range []int64{source, target} {
		require.NoError(t, chats.EnsureChatInDb(id, "greetings_last_msg"))
		chatID := id
		t.Cleanup(func() { cleanupBackupChat(t, chatID) })
	}

	require.NoError(t, ImportModuleData(source, BackupModuleGreetings, map[string]any{"settings": map[string]any{
		"welcome_settings": map[string]any{"welcome_text": "hi", "welcome_enabled": true},
		"goodbye_settings": map[string]any{"text": "bye", "enabled": true},
	}}))
	require.NoError(t, db.DB.Exec(
		"UPDATE greetings SET welcome_last_msg_id = 111, goodbye_last_msg_id = 222 WHERE chat_id = ?", source).Error)

	exported, err := ExportModuleData(source, BackupModuleGreetings)
	require.NoError(t, err)
	raw, err := json.Marshal(exported)
	require.NoError(t, err)
	assert.False(t, strings.Contains(string(raw), "last_msg_id"), "the export carries a last message ID: %s", raw)

	require.NoError(t, ImportModuleData(target, BackupModuleGreetings, map[string]any{"settings": map[string]any{
		"welcome_settings": map[string]any{"welcome_text": "hi", "welcome_enabled": true, "last_msg_id": 333},
		"goodbye_settings": map[string]any{"text": "bye", "enabled": true, "last_msg_id": 444},
	}}))
	var welcomeLast, goodbyeLast int64
	require.NoError(t, db.DB.Raw("SELECT welcome_last_msg_id, goodbye_last_msg_id FROM greetings WHERE chat_id = ?",
		target).Row().Scan(&welcomeLast, &goodbyeLast))
	assert.Zero(t, welcomeLast, "an imported welcome last message ID")
	assert.Zero(t, goodbyeLast, "an imported goodbye last message ID")
}
