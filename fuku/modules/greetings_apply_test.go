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

func TestGreetingApplyPartWrites(t *testing.T) {
	button := models.Button{Name: "Site", Url: "https://example.com"}
	for _, isWelcome := range []bool{true, false} {
		name := "goodbye"
		if isWelcome {
			name = "welcome"
		}
		t.Run(name, func(t *testing.T) {
			chatID := uniqueModuleChatID()
			require.NoError(t, greetings.SetWelcomeText(chatID, "W", "wf", nil, 2))
			require.NoError(t, greetings.SetGoodbyeText(chatID, "G", "gf", nil, 2))
			before := loadGreetingRow(t, chatID)

			text := applyGreetingText(context.Background(), chatID, isWelcome, "<b>x</b>")
			require.Equal(t, greetingApplied, text.Outcome)
			require.NoError(t, text.Err)
			afterText := loadGreetingRow(t, chatID)
			sideText, otherText := partOf(afterText, isWelcome), partOf(afterText, !isWelcome)
			assert.Equal(t, "<b>x</b>", sideText.text)
			assert.Equal(t, partOf(before, isWelcome).fileID, sideText.fileID, "text write leaves the file")
			assert.Equal(t, partOf(before, !isWelcome), otherText, "text write leaves the other side")

			media := applyGreetingMedia(context.Background(), chatID, isWelcome, "f1", 3)
			require.Equal(t, greetingApplied, media.Outcome)
			afterMedia := partOf(loadGreetingRow(t, chatID), isWelcome)
			assert.Equal(t, "f1", afterMedia.fileID)
			assert.Equal(t, 3, afterMedia.dataType)
			assert.Equal(t, "<b>x</b>", afterMedia.text, "media write leaves the text")

			btns := applyGreetingButtons(context.Background(), chatID, isWelcome, []models.Button{button})
			require.Equal(t, greetingApplied, btns.Outcome)
			afterBtns := partOf(loadGreetingRow(t, chatID), isWelcome)
			assert.Equal(t, []models.Button{button}, afterBtns.buttons)
			assert.Equal(t, "f1", afterBtns.fileID, "buttons write leaves the file")
			assert.Equal(t, partOf(before, !isWelcome), partOf(loadGreetingRow(t, chatID), !isWelcome))
		})
	}
}

type greetingPart struct {
	text     string
	fileID   string
	dataType int
	buttons  []models.Button
	enabled  bool
	clean    bool
}

func partOf(s *models.GreetingSettings, welcome bool) greetingPart {
	if welcome {
		w := s.WelcomeSettings
		return greetingPart{w.WelcomeText, w.FileID, w.WelcomeType, normalizeButtons(w.Button), w.ShouldWelcome, w.CleanWelcome}
	}
	g := s.GoodbyeSettings
	return greetingPart{g.GoodbyeText, g.FileID, g.GoodbyeType, normalizeButtons(g.Button), g.ShouldGoodbye, g.CleanGoodbye}
}

func normalizeButtons(b models.ButtonArray) []models.Button {
	if len(b) == 0 {
		return nil
	}
	return []models.Button(b)
}

// TestGreetingRemovesEqualReset: the three Remove applies together leave the row one
// /resetwelcome (or /resetgoodbye) leaves on a twin chat.
func TestGreetingRemovesEqualReset(t *testing.T) {
	admin := gotgbot.User{Id: 777000, FirstName: "Telegram"}
	button := models.Button{Name: "Site", Url: "https://example.com"}
	for _, isWelcome := range []bool{true, false} {
		name, command := "goodbye", "/resetgoodbye"
		run := greetingsModule.resetGoodbye
		if isWelcome {
			name, command, run = "welcome", "/resetwelcome", greetingsModule.resetWelcome
		}
		t.Run(name, func(t *testing.T) {
			client := newModuleBotClient()
			bot := newModuleTestBot(client)
			chatA := gotgbot.Chat{Id: uniqueModuleChatID(), Type: "supergroup", Title: "A"}
			chatB := gotgbot.Chat{Id: uniqueModuleChatID(), Type: "supergroup", Title: "B"}
			for _, id := range []int64{chatA.Id, chatB.Id} {
				require.NoError(t, greetings.SetWelcomeText(id, "custom W", "wf", []models.Button{button}, 3))
				require.NoError(t, greetings.SetGoodbyeText(id, "custom G", "gf", []models.Button{button}, 3))
			}

			ctx := context.Background()
			for _, res := range []greetingApplyResult{
				applyGreetingRemoveButtons(ctx, chatA.Id, isWelcome),
				applyGreetingRemoveText(ctx, chatA.Id, isWelcome),
				applyGreetingRemoveMedia(ctx, chatA.Id, isWelcome),
			} {
				require.Equal(t, greetingApplied, res.Outcome)
				require.NoError(t, res.Err)
			}
			require.Equal(t, ext.EndGroups, run(bot, newGreetingMessageContext(bot, chatB, admin, command)))

			requireSameGreetingRow(t, loadGreetingRow(t, chatB.Id), loadGreetingRow(t, chatA.Id))
			part := partOf(loadGreetingRow(t, chatA.Id), isWelcome)
			assert.Empty(t, part.fileID)
			assert.Empty(t, part.buttons)
			assert.Equal(t, 1, part.dataType)
		})
	}
}

// TestGreetingApplyResetEqualsCommand: applyGreetingReset stores what the reset commands store.
func TestGreetingApplyResetEqualsCommand(t *testing.T) {
	admin := gotgbot.User{Id: 777000, FirstName: "Telegram"}
	client := newModuleBotClient()
	bot := newModuleTestBot(client)
	chatA := gotgbot.Chat{Id: uniqueModuleChatID(), Type: "supergroup", Title: "A"}
	chatB := uniqueModuleChatID()
	for _, id := range []int64{chatA.Id, chatB} {
		require.NoError(t, greetings.SetWelcomeText(id, "custom", "wf", []models.Button{{Name: "n", Url: "https://e.x"}}, 3))
	}
	require.Equal(t, ext.EndGroups, greetingsModule.resetWelcome(bot, newGreetingMessageContext(bot, chatA, admin, "/resetwelcome")))
	res := applyGreetingReset(context.Background(), chatB, true)
	require.Equal(t, greetingApplied, res.Outcome)
	requireSameGreetingRow(t, loadGreetingRow(t, chatA.Id), loadGreetingRow(t, chatB))
}
