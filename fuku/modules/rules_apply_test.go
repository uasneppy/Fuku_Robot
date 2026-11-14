package modules

import (
	"context"
	"errors"
	"testing"

	tgmd2html "github.com/PaulSonOfLars/gotg_md2html"
	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/db/rules"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
)

// TestRulesApplyParity: the apply functions store what /setrules and /clearrules store
// for the same input on a twin chat.
func TestRulesApplyParity(t *testing.T) {
	admin := gotgbot.User{Id: 777000, FirstName: "Telegram"}
	t.Run("set", func(t *testing.T) {
		client := newModuleBotClient()
		bot := newModuleTestBot(client)
		chatA := gotgbot.Chat{Id: uniqueModuleChatID(), Type: "supergroup", Title: "A"}
		chatB := uniqueModuleChatID()

		require.Equal(t, ext.EndGroups, rulesModule.setRules(bot, newModuleMessageContext(bot, chatA, admin, "/setrules **Be kind**")))
		res := applyRulesText(context.Background(), chatB, tgmd2html.MD2HTMLV2("**Be kind**"))
		require.Equal(t, greetingApplied, res.Outcome)
		require.NoError(t, res.Err)

		want := rules.GetChatRulesInfo(chatA.Id).Rules
		assert.NotEmpty(t, want)
		assert.Equal(t, want, rules.GetChatRulesInfo(chatB).Rules)
		assert.Equal(t, want, res.Rules, "the stored text is read back")
	})
	t.Run("clear", func(t *testing.T) {
		client := newModuleBotClient()
		bot := newModuleTestBot(client)
		chatA := gotgbot.Chat{Id: uniqueModuleChatID(), Type: "supergroup", Title: "A"}
		chatB := uniqueModuleChatID()
		editCleanup(t, chatA.Id, chatB)
		for _, id := range []int64{chatA.Id, chatB} {
			settingsRulesSeedAll(t, id)
			settingsRulesSeedKept(t, id)
		}

		require.Equal(t, ext.EndGroups, rulesModule.clearRules(bot, newModuleMessageContext(bot, chatA, admin, "/clearrules")))
		res := applyRulesClear(context.Background(), chatB)
		require.Equal(t, greetingApplied, res.Outcome)
		require.NotNil(t, res.Row, "the whole row is read back")
		assert.Empty(t, res.Rules)
		rowA, rowB := rules.GetChatRulesInfo(chatA.Id), rules.GetChatRulesInfo(chatB)
		for name, row := range map[string]*models.RulesSettings{"command": rowA, "apply": rowB, "read back": res.Row} {
			assert.Empty(t, row.Rules, name)
			assert.Empty(t, row.FileID, name)
			assert.Zero(t, row.MediaType, name)
			assert.Empty(t, row.Buttons, name)
			assert.True(t, settingsRulesKeptOK(*row), "%s keeps Media under text, private rules and the rules button", name)
		}
	})
}

// TestRulesApplyWriteFailure: a failed write is the save-failed outcome and the command
// still answers common_settings_save_failed.
func TestRulesApplyWriteFailure(t *testing.T) {
	restoreI18n, err := i18n.OverrideManagerForTest("common_settings_save_failed: save failed\nrules_set_successfully: saved\n")
	require.NoError(t, err)
	t.Cleanup(restoreI18n)
	boom := errors.New("write refused")
	saved := rulesWrites
	t.Cleanup(func() { rulesWrites = saved })
	rulesWrites.SetChatRules = func(int64, string) error { return boom }

	res := applyRulesText(context.Background(), uniqueModuleChatID(), "x")
	assert.Equal(t, greetingSaveFailed, res.Outcome)
	assert.ErrorIs(t, res.Err, boom)

	client := newModuleBotClient()
	bot := newModuleTestBot(client)
	chat := gotgbot.Chat{Id: uniqueModuleChatID(), Type: "supergroup", Title: "A"}
	admin := gotgbot.User{Id: 777000, FirstName: "Telegram"}
	require.Equal(t, ext.EndGroups, rulesModule.setRules(bot, newModuleMessageContext(bot, chat, admin, "/setrules text")))
	calls := client.callsFor("sendMessage")
	require.Len(t, calls, 1)
	assert.Equal(t, "save failed", calls[0].Params["text"])
}

// TestBuildRulesTextMatchesSendRules: buildRulesText is the heading with the escaped title,
// a blank line and the normalized rules, or the no-rules line, and /rules sends exactly it.
func TestBuildRulesTextMatchesSendRules(t *testing.T) {
	restoreI18n, err := i18n.OverrideManagerForTest("rules_for_chat_header: \"Rules for %s\"\nrules_no_rules_set: no rules yet\n")
	require.NoError(t, err)
	t.Cleanup(restoreI18n)
	tr := i18n.MustNewTranslator("en")

	t.Run("heading and rules", func(t *testing.T) {
		got := buildRulesText(tr, "A & B", "<b>Be kind</b>")
		assert.Equal(t, "Rules for A &amp; B\n\n"+normalizeRulesForHTML("<b>Be kind</b>"), got)
	})
	t.Run("no rules", func(t *testing.T) {
		assert.Equal(t, "no rules yet", buildRulesText(tr, "A & B", ""))
	})
	for _, stored := range []string{"<b>Be kind</b>", ""} {
		client := newModuleBotClient()
		bot := newModuleTestBot(client)
		chat := gotgbot.Chat{Id: uniqueModuleChatID(), Type: "supergroup", Title: "A & B"}
		user := gotgbot.User{Id: 42, FirstName: "Member"}
		if stored != "" {
			require.NoError(t, rules.SetChatRules(chat.Id, stored))
		}
		ctx := newModuleMessageContext(bot, chat, user, "/rules")
		require.Equal(t, ext.EndGroups, rulesModule.sendRules(bot, ctx))
		calls := client.callsFor("sendMessage")
		require.Len(t, calls, 1)
		assert.Equal(t, buildRulesText(tr, chat.Title, stored), calls[0].Params["text"])
	}
}
