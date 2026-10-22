//go:build testtools

package modules

import (
	"fmt"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"

	"github.com/divkix/Alita_Robot/alita/db/staff"
)

// sentTexts returns the text of every sendMessage call the client recorded.
func sentTexts(client *staffBotClient) []string {
	var texts []string
	for _, call := range client.callsFor("sendMessage") {
		texts = append(texts, fmt.Sprint(call.Params["text"]))
	}
	return texts
}

func staffSupergroup(id int64) gotgbot.Chat {
	return gotgbot.Chat{Id: id, Type: "supergroup", Title: "Staff Test Group"}
}

func TestStaffTracerSetStaffThenStaffShowsPanel(t *testing.T) {
	withStaffLocale(t)
	client := newStaffBotClient()
	bot := newModuleTestBot(client.moduleBotClient)
	bot.BotClient = client

	chatID := uniqueModuleChatID()
	otherChatID := uniqueModuleChatID()
	const ownerID int64 = 4242
	staffCleanup(t, chatID, otherChatID)
	client.setCreator(chatID, ownerID)

	chat := staffSupergroup(chatID)
	owner := gotgbot.User{Id: ownerID, FirstName: "Owner"}

	err := runStaffCommand(t, bot, newModuleMessageContext(bot, chat, owner, "/setstaff"), setStaffDesc, staffModule.setStaff)
	if err != ext.EndGroups {
		t.Fatalf("/setstaff returned %v, want ext.EndGroups", err)
	}

	row, err := staff.GetStaffGroupFresh(chatID)
	if err != nil {
		t.Fatalf("GetStaffGroupFresh: %v", err)
	}
	if row == nil || row.OwnerUserID != ownerID {
		t.Fatalf("staff group row = %+v, want owner %d", row, ownerID)
	}
	texts := sentTexts(client)
	if len(texts) != 1 || !strings.Contains(texts[0], staffMarker("staff_set_done")) ||
		!strings.Contains(texts[0], fmt.Sprint(chatID)) {
		t.Fatalf("/setstaff replies = %q, want staff_set_done with chat ID %d", texts, chatID)
	}

	err = runStaffCommand(t, bot, newModuleMessageContext(bot, chat, owner, "/staff"), staffDesc, staffModule.staffPanel)
	if err != ext.EndGroups {
		t.Fatalf("/staff returned %v, want ext.EndGroups", err)
	}
	texts = sentTexts(client)
	if len(texts) != 2 {
		t.Fatalf("after /staff sent %d messages, want 2: %q", len(texts), texts)
	}
	for _, want := range []string{
		staffMarker("staff_help_msg"),
		staffMarker("staff_panel_chat_id"),
		staffMarker("staff_panel_no_links"),
		fmt.Sprint(chatID),
	} {
		if !strings.Contains(texts[1], want) {
			t.Errorf("/staff text %q missing %q", texts[1], want)
		}
	}

	otherChat := staffSupergroup(otherChatID)
	err = runStaffCommand(t, bot, newModuleMessageContext(bot, otherChat, owner, "/staff"), staffDesc, staffModule.staffPanel)
	if err != ext.EndGroups {
		t.Fatalf("/staff in a non-Staff Group returned %v, want ext.EndGroups", err)
	}
	if got := len(sentTexts(client)); got != 2 {
		t.Fatalf("/staff in a non-Staff Group sent a message (total %d, want 2)", got)
	}

	privateChat := gotgbot.Chat{Id: ownerID, Type: "private", FirstName: "Owner"}
	err = runStaffCommand(t, bot, newModuleMessageContext(bot, privateChat, owner, "/staff"), staffDesc, staffModule.staffPanel)
	if err != ext.EndGroups {
		t.Fatalf("/staff in a private chat returned %v, want ext.EndGroups", err)
	}
	if got := len(sentTexts(client)); got != 2 {
		t.Fatalf("/staff in a private chat sent a message (total %d, want 2)", got)
	}
}

func TestStaffTracerSetStaffRefusesNonOwnerAndUnknown(t *testing.T) {
	withStaffLocale(t)
	client := newStaffBotClient()
	bot := newModuleTestBot(client.moduleBotClient)
	bot.BotClient = client

	chatID := uniqueModuleChatID()
	failChatID := uniqueModuleChatID()
	const ownerID int64 = 4242
	const intruderID int64 = 5151
	staffCleanup(t, chatID, failChatID)
	client.setCreator(chatID, ownerID)
	client.setCreator(failChatID, ownerID)
	client.setFailure("getChatAdministrators", failChatID, &gotgbot.TelegramError{
		Method:      "getChatAdministrators",
		Code:        429,
		Description: "Too Many Requests: retry after 5",
	})

	intruder := gotgbot.User{Id: intruderID, FirstName: "Intruder"}
	owner := gotgbot.User{Id: ownerID, FirstName: "Owner"}

	err := runStaffCommand(t, bot,
		newModuleMessageContext(bot, staffSupergroup(chatID), intruder, "/setstaff"),
		setStaffDesc, staffModule.setStaff)
	if err != ext.EndGroups {
		t.Fatalf("non-owner /setstaff returned %v, want ext.EndGroups", err)
	}
	texts := sentTexts(client)
	if len(texts) != 1 || !strings.Contains(texts[0], staffMarker("staff_refuse_not_owner")) {
		t.Fatalf("non-owner replies = %q, want staff_refuse_not_owner", texts)
	}
	if row, err := staff.GetStaffGroupFresh(chatID); err != nil || row != nil {
		t.Fatalf("non-owner created a row: %+v (err %v)", row, err)
	}

	err = runStaffCommand(t, bot,
		newModuleMessageContext(bot, staffSupergroup(failChatID), owner, "/setstaff"),
		setStaffDesc, staffModule.setStaff)
	if err != ext.EndGroups {
		t.Fatalf("/setstaff with a Telegram error returned %v, want ext.EndGroups", err)
	}
	texts = sentTexts(client)
	if len(texts) != 2 || !strings.Contains(texts[1], staffMarker("staff_check_failed")) {
		t.Fatalf("API-error replies = %q, want staff_check_failed", texts)
	}
	if row, err := staff.GetStaffGroupFresh(failChatID); err != nil || row != nil {
		t.Fatalf("API error created a row: %+v (err %v)", row, err)
	}
}
