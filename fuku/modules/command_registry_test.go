//go:build testtools

package modules

import (
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"

	"github.com/uasneppy/fuku_robot/fuku/utils/helpers"
)

func TestModulesRecordTheirCommands(t *testing.T) {
	t.Run("rules module", func(t *testing.T) {
		client := newModuleBotClient()
		bot := newModuleTestBot(client)
		dispatcher := ext.NewDispatcher(&ext.DispatcherOpts{MaxRoutines: -1})
		LoadRules(dispatcher)

		for _, name := range []string{"rules", "setrules", "privaterules", "rulesbtn", "resetrulesbtn", "resetrules", "clearrules"} {
			if !helpers.IsBotCommand(name) {
				t.Errorf("IsBotCommand(%q) = false after LoadRules", name)
			}
		}
		if helpers.IsBotCommand("notacommandanyonehandles") {
			t.Error("IsBotCommand answered true for a name no module registers")
		}

		chatID := uniqueModuleChatID()
		text := "/rules"
		update := &gotgbot.Update{UpdateId: 1, Message: &gotgbot.Message{
			MessageId: 101,
			Date:      1,
			Chat:      gotgbot.Chat{Id: chatID, Type: "supergroup", Title: "Rules Chat"},
			From:      &gotgbot.User{Id: 777000, FirstName: "Telegram"},
			Text:      text,
			Entities:  []gotgbot.MessageEntity{{Type: "bot_command", Offset: 0, Length: int64(len(text))}},
		}}
		if err := dispatcher.ProcessUpdate(bot, update, nil); err != nil {
			t.Fatalf("ProcessUpdate(/rules) error = %v", err)
		}
		if calls := client.callsFor("sendMessage"); len(calls) != 1 {
			t.Fatalf("sendMessage calls = %d, want 1", len(calls))
		}
	})
}
