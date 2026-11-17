//go:build testtools

package modules

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
)

// TestPrivNoteReportsAFailedSave proves that /privnote on|off answers the save-failed text, never the success
// text, when the write fails.
func TestPrivNoteReportsAFailedSave(t *testing.T) {
	withStaffLocale(t)
	saved := noteWrites
	t.Cleanup(func() { noteWrites = saved })
	noteWrites.PrivateNotes = func(int64, bool) error { return errors.New("database down") }

	for _, option := range []string{"on", "off"} {
		client := newModuleBotClient()
		bot := newModuleTestBot(client)
		chat := gotgbot.Chat{Id: uniqueModuleChatID(), Type: "supergroup", Title: "Notes Chat"}
		admin := gotgbot.User{Id: 777000, FirstName: "Telegram"}
		_ = notesModule.privNote(bot, newModuleMessageContext(bot, chat, admin, "/privnote "+option))
		calls := client.callsFor("sendMessage")
		if len(calls) != 1 {
			t.Fatalf("/privnote %s sent %d messages, want one", option, len(calls))
		}
		if text := fmt.Sprint(calls[0].Params["text"]); !strings.Contains(text, staffMarker("common_settings_save_failed")) {
			t.Fatalf("/privnote %s answered %q, want the save-failed text", option, text)
		}
	}
}
