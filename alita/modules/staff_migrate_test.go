//go:build testtools

package modules

import (
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"

	"github.com/divkix/Alita_Robot/alita/db"
	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/db/staff"
)

// migrateContext builds an update whose message is a chat-migration service
// message sent in chat. Exactly one of toChatID and fromChatID is normally set.
func migrateContext(bot *gotgbot.Bot, chat gotgbot.Chat, toChatID, fromChatID int64) *ext.Context {
	msg := &gotgbot.Message{
		MessageId:         103,
		Date:              1,
		Chat:              chat,
		MigrateToChatId:   toChatID,
		MigrateFromChatId: fromChatID,
	}
	return ext.NewContext(bot, &gotgbot.Update{UpdateId: 3, Message: msg}, nil)
}

// migrateSeed stores a Staff Group at staffChatID with one link per group chat ID.
func migrateSeed(t *testing.T, staffChatID int64, groupChatIDs ...int64) {
	t.Helper()
	ids := append([]int64{staffChatID}, groupChatIDs...)
	staffCleanup(t, ids...)
	if err := db.DB.Create(&models.StaffGroup{ChatID: staffChatID, OwnerUserID: 4242, Title: "Staff"}).Error; err != nil {
		t.Fatalf("seed staff group: %v", err)
	}
	for _, groupID := range groupChatIDs {
		link := &models.StaffGroupLink{GroupChatID: groupID, StaffChatID: staffChatID, OwnerUserID: 4242}
		if err := db.DB.Create(link).Error; err != nil {
			t.Fatalf("seed link %d: %v", groupID, err)
		}
	}
}

func TestStaffMigrateTracer(t *testing.T) {
	withStaffLocale(t)
	client := newStaffBotClient()
	bot := newModuleTestBot(client.moduleBotClient)
	bot.BotClient = client

	oldID, newID := uniqueModuleChatID(), uniqueModuleChatID()
	groupA, groupB := uniqueModuleChatID(), uniqueModuleChatID()
	migrateSeed(t, oldID, groupA, groupB)

	err := staffWatchersModule.onMigrateMessage(bot, migrateContext(bot, staffSupergroup(oldID), newID, 0))
	if err != ext.ContinueGroups {
		t.Fatalf("onMigrateMessage returned %v, want ext.ContinueGroups", err)
	}

	if row, err := staff.GetStaffGroupFresh(newID); err != nil || row == nil || row.OwnerUserID != 4242 {
		t.Fatalf("GetStaffGroupFresh(new) = (%+v, %v), want the re-keyed row", row, err)
	}
	if row, err := staff.GetStaffGroupFresh(oldID); err != nil || row != nil {
		t.Fatalf("GetStaffGroupFresh(old) = (%+v, %v), want (nil, nil)", row, err)
	}
	var links []models.StaffGroupLink
	if err := db.DB.Where("group_chat_id IN ?", []int64{groupA, groupB}).Find(&links).Error; err != nil {
		t.Fatal(err)
	}
	if len(links) != 2 {
		t.Fatalf("found %d links, want 2", len(links))
	}
	for _, link := range links {
		if link.StaffChatID != newID {
			t.Errorf("link %d has staff_chat_id %d, want %d", link.GroupChatID, link.StaffChatID, newID)
		}
	}
	if got := len(sentTexts(client)); got != 0 {
		t.Fatalf("the migrate watcher sent %d messages, want 0", got)
	}

	owner := gotgbot.User{Id: 4242, FirstName: "Owner"}
	err = runStaffCommand(t, bot,
		newModuleMessageContext(bot, staffSupergroup(newID), owner, "/staff"),
		staffDesc, staffModule.staffPanel)
	if err != ext.EndGroups {
		t.Fatalf("/staff on the new ID returned %v, want ext.EndGroups", err)
	}
	texts := sentTexts(client)
	if len(texts) != 1 || !strings.Contains(texts[0], staffMarker("staff_panel_chat_id")) {
		t.Fatalf("/staff on the new ID replies = %q, want the panel", texts)
	}
}
