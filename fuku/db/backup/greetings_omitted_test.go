package backup

import (
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/chats"
)

// greetingSwitchColumns reads the three switches migration 20261105120000 added to greetings straight from the
// table, as nullable values, so a NULL is visible on SQLite as well as a NOT NULL refusal on PostgreSQL.
func greetingSwitchColumns(t *testing.T, chatID int64) (welcomeBelow, goodbyeBelow, once sql.NullBool) {
	t.Helper()
	row := db.DB.Raw("SELECT welcome_media_below, goodbye_media_below, welcome_once FROM greetings WHERE chat_id = ?",
		chatID).Row()
	require.NoError(t, row.Scan(&welcomeBelow, &goodbyeBelow, &once), "the greetings row exists")
	return welcomeBelow, goodbyeBelow, once
}

// TestImportGreetingsOmittedBlocks proves that a greetings import whose payload leaves out the welcome or the
// goodbye object (or both) imports, and writes the three new switch columns as false, never as NULL. A missing
// embedded object used to insert SQL NULL for every one of its columns, which the NOT NULL columns of migration
// 20261105120000 refuse with SQLSTATE 23502 on PostgreSQL, rolling the whole multi-module import back. The rules
// module, whose model gained NOT NULL columns in the same migration, is checked with a text-only payload.
func TestImportGreetingsOmittedBlocks(t *testing.T) {
	skipIfNoDb(t)

	cases := []struct {
		name         string
		settings     map[string]any
		welcomeBelow bool
		goodbyeBelow bool
		once         bool
	}{
		{
			name:     "only welcome_settings",
			settings: map[string]any{"welcome_settings": map[string]any{"welcome_text": "hi", "welcome_enabled": true}},
		},
		{
			name:     "only goodbye_settings",
			settings: map[string]any{"goodbye_settings": map[string]any{"text": "bye", "enabled": true}},
		},
		{
			name:     "empty settings object",
			settings: map[string]any{},
		},
		{
			name: "every block present",
			settings: map[string]any{
				"welcome_settings": map[string]any{
					"welcome_text": "hi", "welcome_enabled": true, "media_below": true, "once": true},
				"goodbye_settings": map[string]any{"text": "bye", "enabled": true, "media_below": true},
			},
			welcomeBelow: true, goodbyeBelow: true, once: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			chatID := time.Now().UnixNano()
			require.NoError(t, chats.EnsureChatInDb(chatID, "omitted_blocks"))
			t.Cleanup(func() { cleanupBackupChat(t, chatID) })

			err := ImportModuleData(chatID, BackupModuleGreetings, map[string]any{"settings": tc.settings})
			require.NoError(t, err, "an import with an omitted block must not fail")

			welcomeBelow, goodbyeBelow, once := greetingSwitchColumns(t, chatID)
			for name, got := range map[string]sql.NullBool{
				"welcome_media_below": welcomeBelow, "goodbye_media_below": goodbyeBelow, "welcome_once": once,
			} {
				assert.True(t, got.Valid, "%s is written, never NULL", name)
			}
			assert.Equal(t, tc.welcomeBelow, welcomeBelow.Bool, "welcome_media_below")
			assert.Equal(t, tc.goodbyeBelow, goodbyeBelow.Bool, "goodbye_media_below")
			assert.Equal(t, tc.once, once.Bool, "welcome_once")

			exported, err := exportGreetingsData(chatID)
			require.NoError(t, err)
			require.NotNil(t, exported.Settings)
			require.NotNil(t, exported.Settings.WelcomeSettings)
			require.NotNil(t, exported.Settings.GoodbyeSettings)
			if _, has := tc.settings["welcome_settings"]; has {
				assert.Equal(t, "hi", exported.Settings.WelcomeSettings.WelcomeText)
			} else {
				assert.Empty(t, exported.Settings.WelcomeSettings.WelcomeText)
			}
			if _, has := tc.settings["goodbye_settings"]; has {
				assert.Equal(t, "bye", exported.Settings.GoodbyeSettings.GoodbyeText)
			} else {
				assert.Empty(t, exported.Settings.GoodbyeSettings.GoodbyeText)
			}
		})
	}

	t.Run("rules payload with only text", func(t *testing.T) {
		chatID := time.Now().UnixNano()
		require.NoError(t, chats.EnsureChatInDb(chatID, "omitted_rules"))
		t.Cleanup(func() { cleanupBackupChat(t, chatID) })

		err := ImportModuleData(chatID, BackupModuleRules, map[string]any{"settings": map[string]any{"rules": "be kind"}})
		require.NoError(t, err)

		exported, err := exportRulesData(chatID)
		require.NoError(t, err)
		require.NotNil(t, exported.Settings)
		assert.Equal(t, "be kind", exported.Settings.Rules)
		assert.Empty(t, exported.Settings.FileID)
		assert.Zero(t, exported.Settings.MediaType)
		assert.Empty(t, exported.Settings.Buttons)
		assert.False(t, exported.Settings.MediaBelow)
	})
}
