package modules

import (
	"context"
	"errors"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/uasneppy/fuku_robot/fuku/db/greetings"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
)

// loadGreetingRow reads a chat's stored greetings row without creating anything.
func loadGreetingRow(t *testing.T, chatID int64) *models.GreetingSettings {
	t.Helper()
	settings, err := greetings.LoadSettingsContext(context.Background(), chatID)
	require.NoError(t, err)
	return settings
}

// requireSameGreetingRow compares what two chats store, ignoring the chat itself.
func requireSameGreetingRow(t *testing.T, want, got *models.GreetingSettings) {
	t.Helper()
	assert.Equal(t, want.ShouldCleanService, got.ShouldCleanService)
	assert.Equal(t, want.ShouldAutoApprove, got.ShouldAutoApprove)
	assert.Equal(t, want.WelcomeSettings, got.WelcomeSettings)
	assert.Equal(t, want.GoodbyeSettings, got.GoodbyeSettings)
}

func TestGreetingApplyParity(t *testing.T) {
	admin := gotgbot.User{Id: 777000, FirstName: "Telegram"}
	cases := []struct {
		name    string
		command string
		run     func(bot *gotgbot.Bot, ctx *ext.Context) error
		apply   func(chatID int64) greetingApplyResult
	}{
		{"welcome off", "/welcome off", greetingsModule.welcome,
			func(id int64) greetingApplyResult { return applyWelcomeEnabled(context.Background(), id, false) }},
		{"goodbye off", "/goodbye off", greetingsModule.goodbye,
			func(id int64) greetingApplyResult { return applyGoodbyeEnabled(context.Background(), id, false) }},
		{"goodbye on", "/goodbye on", greetingsModule.goodbye,
			func(id int64) greetingApplyResult { return applyGoodbyeEnabled(context.Background(), id, true) }},
		{"cleanwelcome on", "/cleanwelcome on", greetingsModule.cleanWelcome,
			func(id int64) greetingApplyResult { return applyCleanWelcome(context.Background(), id, true) }},
		{"cleangoodbye on", "/cleangoodbye on", greetingsModule.cleanGoodbye,
			func(id int64) greetingApplyResult { return applyCleanGoodbye(context.Background(), id, true) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := newModuleBotClient()
			bot := newModuleTestBot(client)
			chatA := gotgbot.Chat{Id: uniqueModuleChatID(), Type: "supergroup", Title: "A"}
			chatB := uniqueModuleChatID()

			require.Equal(t, ext.EndGroups, tc.run(bot, newGreetingMessageContext(bot, chatA, admin, tc.command)))
			result := tc.apply(chatB)
			require.Equal(t, greetingApplied, result.Outcome)
			require.NoError(t, result.Err)
			require.NotNil(t, result.Settings, "a successful write reads the stored row back")

			requireSameGreetingRow(t, loadGreetingRow(t, chatA.Id), loadGreetingRow(t, chatB))
			requireSameGreetingRow(t, result.Settings, loadGreetingRow(t, chatB))
		})
	}
}

func TestGreetingApplyIsAbsolute(t *testing.T) {
	chatID := uniqueModuleChatID()
	first := applyWelcomeEnabled(context.Background(), chatID, false)
	second := applyWelcomeEnabled(context.Background(), chatID, false)
	require.Equal(t, greetingApplied, first.Outcome)
	require.Equal(t, greetingApplied, second.Outcome)
	assert.False(t, first.Settings.WelcomeSettings.ShouldWelcome)
	requireSameGreetingRow(t, first.Settings, second.Settings)
}

func TestGreetingApplyWriteFailure(t *testing.T) {
	restoreI18n, err := i18n.OverrideManagerForTest("common_settings_save_failed: save failed\n")
	require.NoError(t, err)
	t.Cleanup(restoreI18n)

	boom := errors.New("write refused")
	saved := greetingWrites
	t.Cleanup(func() { greetingWrites = saved })
	greetingWrites.WelcomeToggle = func(int64, bool) error { return boom }

	chatID := uniqueModuleChatID()
	result := applyWelcomeEnabled(context.Background(), chatID, false)
	assert.Equal(t, greetingSaveFailed, result.Outcome)
	assert.ErrorIs(t, result.Err, boom)
	assert.Nil(t, result.Settings)

	client := newModuleBotClient()
	bot := newModuleTestBot(client)
	chat := gotgbot.Chat{Id: chatID, Type: "supergroup", Title: "Greeting Chat"}
	admin := gotgbot.User{Id: 777000, FirstName: "Telegram"}
	require.Equal(t, ext.EndGroups, greetingsModule.welcome(bot, newGreetingMessageContext(bot, chat, admin, "/welcome off")))
	calls := client.callsFor("sendMessage")
	require.Len(t, calls, 1)
	assert.Equal(t, "save failed", calls[0].Params["text"], "the command still answers the save-failed text")
}
