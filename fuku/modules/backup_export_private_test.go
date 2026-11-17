//go:build testtools

package modules

import (
	"fmt"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/stretchr/testify/require"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/chats"
	"github.com/uasneppy/fuku_robot/fuku/db/notes"
)

// TestExportGoesToThePrivateChat proves that /export sends the backup file to the admin's private chat and
// never into the group, and that when the admin never started the bot (a 403) the group gets one line asking
// them to, with no file, and the export can be tried again at once.
func TestExportGoesToThePrivateChat(t *testing.T) {
	withStaffLocale(t)
	owner := gotgbot.User{Id: 777000, FirstName: "Telegram"}
	setup := func(t *testing.T) gotgbot.Chat {
		chat := gotgbot.Chat{Id: uniqueModuleChatID(), Type: "supergroup", Title: "Backup Chat"}
		require.NoError(t, chats.EnsureChatInDb(chat.Id, chat.Title))
		require.NoError(t, notes.AddNote(chat.Id, "welcome", "hello", "", nil, db.TEXT, false, false, false, true, false, false))
		t.Cleanup(func() { _ = notes.RemoveAllNotes(chat.Id) })
		return chat
	}

	t.Run("sent to the admin", func(t *testing.T) {
		chat := setup(t)
		client := newModuleBotClient()
		bot := newModuleTestBot(client)
		_ = backupModule.exportHandler(bot, newModuleMessageContext(bot, chat, owner, "/export notes"))
		docs := client.callsFor("sendDocument")
		require.Len(t, docs, 1)
		require.Equal(t, owner.Id, staffParamInt(docs[0].Params, "chat_id"), "the file went to the group")
	})

	t.Run("the admin never started the bot", func(t *testing.T) {
		chat := setup(t)
		client := newModuleBotClient()
		client.errors["sendDocument"] = &gotgbot.TelegramError{Code: 403, Description: "Forbidden: bot can't initiate conversation with a user"}
		bot := newModuleTestBot(client)
		_ = backupModule.exportHandler(bot, newModuleMessageContext(bot, chat, owner, "/export notes"))
		for _, doc := range client.callsFor("sendDocument") {
			require.NotEqual(t, chat.Id, staffParamInt(doc.Params, "chat_id"), "the file went to the group")
		}
		messages := client.callsFor("sendMessage")
		require.Len(t, messages, 1)
		require.Equal(t, chat.Id, staffParamInt(messages[0].Params, "chat_id"))
		text := fmt.Sprint(messages[0].Params["text"])
		require.True(t, strings.Contains(text, staffMarker("backup_export_start_bot")), "answer %q", text)

		// The refused export does not hold the cooldown.
		delete(client.errors, "sendDocument")
		_ = backupModule.exportHandler(bot, newModuleMessageContext(bot, chat, owner, "/export notes"))
		require.Len(t, client.callsFor("sendDocument"), 2, "a second export after starting the bot was refused")
	})
}
