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
		require.NoError(t, rules.SetChatRules(chatA.Id, "old"))
		require.NoError(t, rules.SetChatRules(chatB, "old"))

		require.Equal(t, ext.EndGroups, rulesModule.clearRules(bot, newModuleMessageContext(bot, chatA, admin, "/clearrules")))
		res := applyRulesClear(context.Background(), chatB)
		require.Equal(t, greetingApplied, res.Outcome)
		assert.Equal(t, rules.GetChatRulesInfo(chatA.Id).Rules, rules.GetChatRulesInfo(chatB).Rules)
		assert.Empty(t, rules.GetChatRulesInfo(chatB).Rules)
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
