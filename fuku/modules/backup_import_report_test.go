//go:build testtools

package modules

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/stretchr/testify/require"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/backup"
	"github.com/uasneppy/fuku_robot/fuku/db/chats"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// TestBackupConfirmImportReportsWhatItLeftOut proves that the import's success message says how many list
// entries were left out as invalid and that the log channel categories were not imported into a group with no
// log channel.
func TestBackupConfirmImportReportsWhatItLeftOut(t *testing.T) {
	withStaffLocale(t)
	client := newModuleBotClient()
	bot := newModuleTestBot(client)
	chat := gotgbot.Chat{Id: uniqueModuleChatID(), Type: "supergroup", Title: "Backup Chat"}
	owner := gotgbot.User{Id: 777000, FirstName: "Telegram"}
	require.NoError(t, chats.EnsureChatInDb(chat.Id, chat.Title))
	t.Cleanup(func() {
		db.DB.Where("chat_id = ?", chat.Id).Delete(&models.ChatFilters{})
		db.DB.Where("chat_id = ?", chat.Id).Delete(&models.LogChannel{})
	})

	modules := []string{"filters", "logchannels"}
	bkp := backup.NewBackupFormat(chat.Id, chat.Title, owner.Id, modules)
	bkp.Data["filters"] = map[string]any{"filters": []any{
		map[string]any{"keyword": "hello", "filter_reply": "hi", "msgtype": 1},
		map[string]any{"keyword": "a", "filter_reply": "too broad", "msgtype": 1},
	}}
	bkp.Data["logchannels"] = map[string]any{"settings": map[string]any{"log_channel_id": -100123, "cat_admin": true}}
	token, err := storePendingImport(chat.Id, bkp, modules)
	require.NoError(t, err)
	t.Cleanup(func() { dropPendingImport(chat.Id) })

	callback := encodeCallbackData("backup",
		map[string]string{"a": backupActionConfirmImport, "c": strconv.FormatInt(chat.Id, 10), "t": token})
	_ = backupModule.backupCallbackHandler(bot, newModuleCallbackContext(bot, chat, owner, callback))

	calls := client.callsFor("sendMessage")
	require.Len(t, calls, 1)
	text := fmt.Sprint(calls[0].Params["text"])
	for _, key := range []string{"backup_import_success", "backup_import_skipped_entries", "backup_import_log_skipped"} {
		require.True(t, strings.Contains(text, staffMarker(key)), "the message %q lacks %s", text, key)
	}
	require.Contains(t, text, staffMarker("backup_import_skipped_entries")+" 1", "the count of entries left out")
}
