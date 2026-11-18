package backup

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/chats"
	"github.com/uasneppy/fuku_robot/fuku/db/eventmsg"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// TestEventMessagesStayOutOfBackup proves D-07: a group's custom event messages are not a backup
// module, so an export carries none of them, an import writes none into the destination chat and
// a reset leaves the source chat's rows alone. A later phase adds them to backups on purpose.
func TestEventMessagesStayOutOfBackup(t *testing.T) {
	skipIfNoDb(t)

	ctx := context.Background()
	srcChat := time.Now().UnixNano()
	dstChat := srcChat + 1
	require.NoError(t, chats.EnsureChatInDb(srcChat, "event_messages_source"))
	require.NoError(t, chats.EnsureChatInDb(dstChat, "event_messages_destination"))
	t.Cleanup(func() {
		cleanupBackupChat(t, srcChat)
		cleanupBackupChat(t, dstChat)
		require.NoError(t, db.DB.Where("chat_id IN ?", []int64{srcChat, dstChat}).Delete(&models.ChatEventMessage{}).Error)
	})

	require.NoError(t, eventmsg.SetText(ctx, srcChat, eventmsg.EventBan, eventmsg.DestGroup, "CUSTOM-EVENT-TEXT", 42))
	require.NoError(t, eventmsg.SetMedia(ctx, srcChat, eventmsg.EventBan, eventmsg.DestGroup, "CUSTOM-FILE-ID", 2, 42))
	require.NoError(t, eventmsg.SetButtons(ctx, srcChat, eventmsg.EventBan, eventmsg.DestGroup,
		[]models.Button{{Name: "CUSTOM-BUTTON", Url: "https://example.org/custom"}}, 42))

	countRows := func(chatID int64) int64 {
		t.Helper()
		var n int64
		require.NoError(t, db.DB.Model(&models.ChatEventMessage{}).Where("chat_id = ?", chatID).Count(&n).Error)
		return n
	}
	require.EqualValues(t, 1, countRows(srcChat))

	exported, err := ExportChatData(srcChat, "source", 42, nil)
	require.NoError(t, err)
	assert.Len(t, exported.Data, len(AllExportableModules()))
	for module := range exported.Data {
		assert.True(t, IsValidModule(module), "export holds only backup modules, got %q", module)
	}
	for _, name := range []string{"chat_event_messages", "event_messages"} {
		_, err = ExportChatData(srcChat, "source", 42, []string{name})
		assert.Error(t, err, "%s is not a backup module", name)
		_, named := exported.Data[name]
		assert.False(t, named, "the export has no %s key", name)
	}

	raw, err := exported.ToJSON()
	require.NoError(t, err)
	for _, needle := range []string{"chat_event_messages", "CUSTOM-EVENT-TEXT", "CUSTOM-FILE-ID", "CUSTOM-BUTTON"} {
		assert.NotContains(t, string(raw), needle, "the export file carries no event message (%s)", needle)
	}
	decoded, err := BackupFormatFromJSON(raw)
	require.NoError(t, err)
	require.NoError(t, ImportChatData(dstChat, decoded, nil))
	assert.Zero(t, countRows(dstChat), "an import creates no chat_event_messages row")

	require.NoError(t, ClearChatData(srcChat, nil))
	assert.EqualValues(t, 1, countRows(srcChat), "a reset leaves the event messages in place")
	kept, err := eventmsg.GetFresh(ctx, srcChat)
	require.NoError(t, err)
	msg, ok := kept.Lookup(eventmsg.EventBan, eventmsg.DestGroup)
	require.True(t, ok)
	assert.Equal(t, "CUSTOM-EVENT-TEXT", msg.Text, "a reset leaves the text as it was")
	assert.Equal(t, "CUSTOM-FILE-ID", msg.FileID, "a reset leaves the media as it was")
	assert.Len(t, msg.Buttons, 1, "a reset leaves the buttons as they were")
}
