package modules

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/federations"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
)

// TestFedFanoutReportsFailedGroups holds M13: an /fban whose ban fails in the
// federation's groups is followed in the federation log by "N of M groups
// failed" instead of dropping the failures.
func TestFedFanoutReportsFailedGroups(t *testing.T) {
	client := newModuleBotClient()
	client.errors["banChatMember"] = errors.New("Bad Request: not enough rights to restrict/unrestrict chat member")
	bot := newModuleTestBot(client)
	owner := uniquePositiveUserID()
	fed, err := federations.CreateFederation(owner, "Fanout Fed")
	require.NoError(t, err)
	t.Cleanup(func() { _ = federations.DeleteFederation(fed.FedID) })
	target := uniquePositiveUserID()
	t.Cleanup(func() { db.DB.Where("user_id IN ?", []int64{owner, target}).Delete(&models.User{}) })
	logChat := uniqueModuleChatID()
	require.NoError(t, federations.SetFedLogChat(fed.FedID, logChat))

	first := fedChatWithUsers(t, fed.FedID, target)
	_ = fedChatWithUsers(t, fed.FedID, target)
	admin := gotgbot.User{Id: 777000, FirstName: "Telegram"}
	require.NoError(t, federations.PromoteFedAdmin(fed.FedID, admin.Id))

	group := gotgbot.Chat{Id: first, Type: "supergroup", Title: "fed chat"}
	if err := federationsModule.fban(bot, newModuleMessageContext(bot, group, admin, fmt.Sprintf("/fban %d spam", target))); err != ext.EndGroups {
		t.Fatalf("fban: %v", err)
	}
	reported := func() bool {
		for _, call := range client.callsFor("sendMessage") {
			if fmt.Sprint(call.Params["chat_id"]) == fmt.Sprint(logChat) &&
				strings.Contains(fmt.Sprint(call.Params["text"]), "2 of 2 groups failed") {
				return true
			}
		}
		return false
	}
	require.Eventually(t, reported, 5*time.Second, 10*time.Millisecond,
		"the federation log was not told that the ban failed in 2 of 2 groups")
}

// TestUnfbanSaveFailureIsNotNotBanned holds M13: a database error removing an
// fban answers common_settings_save_failed, never "not banned".
func TestUnfbanSaveFailureIsNotNotBanned(t *testing.T) {
	withRealLocales(t)
	client := newModuleBotClient()
	bot := newModuleTestBot(client)
	owner := uniquePositiveUserID()
	fed, err := federations.CreateFederation(owner, "Unfban Fault Fed")
	require.NoError(t, err)
	t.Cleanup(func() { _ = federations.DeleteFederation(fed.FedID) })
	target := uniquePositiveUserID()
	t.Cleanup(func() { db.DB.Where("user_id IN ?", []int64{owner, target}).Delete(&models.User{}) })
	chatID := fedChatWithUsers(t, fed.FedID)
	admin := gotgbot.User{Id: 777000, FirstName: "Telegram"}
	require.NoError(t, federations.PromoteFedAdmin(fed.FedID, admin.Id))
	_, _, err = federations.Fban(fed.FedID, target, admin.Id, "spam")
	require.NoError(t, err)

	name := fmt.Sprintf("fed-test:fail-ban-delete:%d", target)
	require.NoError(t, db.DB.Callback().Delete().Before("gorm:delete").Register(name, func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Table == (models.FederationBan{}).TableName() {
			_ = tx.AddError(errors.New("delete failed"))
		}
	}))
	t.Cleanup(func() { _ = db.DB.Callback().Delete().Remove(name) })

	group := gotgbot.Chat{Id: chatID, Type: "supergroup", Title: "fed chat"}
	if err := federationsModule.unfban(bot, newModuleMessageContext(bot, group, admin, fmt.Sprintf("/unfban %d", target))); err != ext.EndGroups {
		t.Fatalf("unfban: %v", err)
	}
	calls := client.callsFor("sendMessage")
	require.Len(t, calls, 1)
	want := trS(i18n.MustNewTranslator("uk"), "common_settings_save_failed")
	require.Equal(t, want, fmt.Sprint(calls[0].Params["text"]))
}
