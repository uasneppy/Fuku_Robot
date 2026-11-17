package modules

import (
	"fmt"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/stretchr/testify/require"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/chats"
	"github.com/uasneppy/fuku_robot/fuku/db/federations"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// fedChatWithUsers records a group that joined fedID and whose recorded members are
// users, and removes both when the test ends.
func fedChatWithUsers(t *testing.T, fedID string, users ...int64) int64 {
	t.Helper()
	chatID := uniqueModuleChatID()
	require.NoError(t, chats.EnsureChatInDb(chatID, "fed chat"))
	t.Cleanup(func() { db.DB.Where("chat_id = ?", chatID).Delete(&models.Chat{}) })
	require.NoError(t, db.DB.Model(&models.Chat{}).Where("chat_id = ?", chatID).
		Update("users", models.Int64Array(users)).Error)
	require.NoError(t, federations.JoinFed(chatID, "fed chat", fedID))
	return chatID
}

// unbansOf returns the unbanChatMember calls the client recorded for userID.
func unbansOf(client *moduleBotClient, userID int64) []moduleBotCall {
	var out []moduleBotCall
	for _, call := range client.callsFor("unbanChatMember") {
		if fmt.Sprint(call.Params["user_id"]) == fmt.Sprint(userID) {
			out = append(out, call)
		}
	}
	return out
}

func TestUnfbanUnbansTheUserInEveryFedChat(t *testing.T) {
	client := newModuleBotClient()
	bot := newModuleTestBot(client)
	owner := uniquePositiveUserID()
	fed, err := federations.CreateFederation(owner, "Unban Fed")
	require.NoError(t, err)
	t.Cleanup(func() { _ = federations.DeleteFederation(fed.FedID) })
	target := uniquePositiveUserID()
	t.Cleanup(func() { db.DB.Where("user_id IN ?", []int64{owner, target}).Delete(&models.User{}) })

	knows := fedChatWithUsers(t, fed.FedID, target)
	_ = fedChatWithUsers(t, fed.FedID)
	admin := gotgbot.User{Id: 777000, FirstName: "Telegram"}
	require.NoError(t, federations.PromoteFedAdmin(fed.FedID, admin.Id))
	_, _, err = federations.Fban(fed.FedID, target, admin.Id, "spam")
	require.NoError(t, err)

	group := gotgbot.Chat{Id: knows, Type: "supergroup", Title: "fed chat"}
	if err := federationsModule.unfban(bot, newModuleMessageContext(bot, group, admin, fmt.Sprintf("/unfban %d", target))); err != ext.EndGroups {
		t.Fatalf("unfban: %v", err)
	}
	require.Nil(t, federations.GetFedBan(fed.FedID, target), "the ban row must be gone")

	require.Eventually(t, func() bool { return len(unbansOf(client, target)) == 1 }, 5*time.Second, 10*time.Millisecond,
		"want one unbanChatMember for the chat that knows the user")
	time.Sleep(50 * time.Millisecond)
	calls := unbansOf(client, target)
	require.Len(t, calls, 1, "no unban for a chat that never saw the user")
	require.Equal(t, fmt.Sprint(knows), fmt.Sprint(calls[0].Params["chat_id"]))
	require.Equal(t, true, calls[0].Params["only_if_banned"])
}

func TestApplyActiveUnfbanKeepsBansFromSubscribedFeds(t *testing.T) {
	client := newModuleBotClient()
	bot := newModuleTestBot(client)
	fedA, err := federations.CreateFederation(uniquePositiveUserID(), "Fed A")
	require.NoError(t, err)
	t.Cleanup(func() { _ = federations.DeleteFederation(fedA.FedID) })
	fedB, err := federations.CreateFederation(uniquePositiveUserID(), "Fed B")
	require.NoError(t, err)
	t.Cleanup(func() { _ = federations.DeleteFederation(fedB.FedID) })
	require.NoError(t, federations.SubscribeFed(fedA.FedID, fedB.FedID))
	target := uniquePositiveUserID()
	t.Cleanup(func() {
		db.DB.Where("user_id IN ?", []int64{fedA.OwnerID, fedB.OwnerID, target}).Delete(&models.User{})
	})
	chatID := fedChatWithUsers(t, fedA.FedID, target)

	_, _, err = federations.Fban(fedB.FedID, target, fedB.OwnerID, "raid")
	require.NoError(t, err)

	applyActiveUnfban(bot, fedA.FedID, target)
	require.Empty(t, unbansOf(client, target), "B still bans the user, so A's chats must keep them out")

	require.NoError(t, federations.Unfban(fedB.FedID, target))
	applyActiveUnfban(bot, fedA.FedID, target)
	calls := unbansOf(client, target)
	require.Len(t, calls, 1)
	require.Equal(t, fmt.Sprint(chatID), fmt.Sprint(calls[0].Params["chat_id"]))
	require.Equal(t, true, calls[0].Params["only_if_banned"])
}

func TestFbanChangedOnlyWhenNewOrReasonChanged(t *testing.T) {
	require.True(t, fbanChanged(true, nil, "spam"))
	require.False(t, fbanChanged(false, &models.FederationBan{Reason: "spam"}, "  spam  "))
	require.True(t, fbanChanged(false, &models.FederationBan{Reason: "spam"}, "raid"))

	client := newModuleBotClient()
	bot := newModuleTestBot(client)
	owner := uniquePositiveUserID()
	fed, err := federations.CreateFederation(owner, "Quiet Fed")
	require.NoError(t, err)
	t.Cleanup(func() { _ = federations.DeleteFederation(fed.FedID) })
	target := uniquePositiveUserID()
	t.Cleanup(func() { db.DB.Where("user_id IN ?", []int64{owner, target}).Delete(&models.User{}) })
	group := gotgbot.Chat{Id: fedChatWithUsers(t, fed.FedID), Type: "supergroup", Title: "fed chat"}
	admin := gotgbot.User{Id: 777000, FirstName: "Telegram"}
	require.NoError(t, federations.PromoteFedAdmin(fed.FedID, admin.Id))

	ownerNotes := func() int {
		count := 0
		for _, call := range client.callsFor("sendMessage") {
			if fmt.Sprint(call.Params["chat_id"]) == fmt.Sprint(owner) {
				count++
			}
		}
		return count
	}
	fban := func(text string) {
		t.Helper()
		if err := federationsModule.fban(bot, newModuleMessageContext(bot, group, admin, text)); err != ext.EndGroups {
			t.Fatalf("%s: %v", text, err)
		}
	}
	fban(fmt.Sprintf("/fban %d spam", target))
	require.Equal(t, 1, ownerNotes(), "a new fban notifies the owner")
	fban(fmt.Sprintf("/fban %d spam", target))
	require.Equal(t, 1, ownerNotes(), "a repeated fban with the same reason announces nothing")
	fban(fmt.Sprintf("/fban %d raid", target))
	require.Equal(t, 2, ownerNotes(), "a changed reason is announced")
}
