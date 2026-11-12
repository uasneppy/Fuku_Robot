//go:build testtools

package modules

import (
	"strings"
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

	// Each construction path of the module package records its names: a descriptor through
	// WrapCommand, the StaffActions table loop, the settings cancel pair (the raw pass-on
	// beside the pipeline command), literal raw sites and MultiCommand aliases.
	paths := []struct {
		name  string
		load  func(*ext.Dispatcher)
		names []string
	}{
		{"WrapCommand", LoadBans, []string{"ban", "kick", "tban"}},
		{"table loop", LoadStaffActions, staffCommandNames()},
		{"settings and cancel", LoadSettings, []string{"settings", "cancel"}},
		{"raw sites and MultiCommand in federations", LoadFederations, []string{"fban", "unfban", "funban"}},
		{"raw sites and MultiCommand in notes", LoadNotes, []string{"get", "privnote", "privatenotes"}},
	}
	for _, tc := range paths {
		t.Run(tc.name, func(t *testing.T) {
			tc.load(ext.NewDispatcher(&ext.DispatcherOpts{MaxRoutines: -1}))
			for _, name := range tc.names {
				if !helpers.IsBotCommand(name) {
					t.Errorf("IsBotCommand(%q) = false after loading the module", name)
				}
				if !helpers.IsBotCommand(strings.ToUpper(name)) {
					t.Errorf("IsBotCommand(%q) = false: the lookup must ignore case", strings.ToUpper(name))
				}
			}
		})
	}
	if helpers.IsBotCommand("notacommandanyonehandles") {
		t.Error("IsBotCommand answered true for a name no module registers")
	}
}

// staffCommandNames lists the names of the StaffActions table.
func staffCommandNames() []string {
	names := make([]string, 0, len(staffActionCommands))
	for _, spec := range staffActionCommands {
		names = append(names, spec.Name)
	}
	return names
}
