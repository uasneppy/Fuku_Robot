//go:build testtools

package modules

import (
	"errors"
	"fmt"
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

// migrateRowState reports where the Staff Group lives and which staff chat each
// of the given group IDs is linked to, so end states can be compared.
func migrateRowState(t *testing.T, ids []int64, groupIDs []int64) string {
	t.Helper()
	var parts []string
	for _, id := range ids {
		row, err := staff.GetStaffGroupFresh(id)
		if err != nil {
			t.Fatalf("GetStaffGroupFresh(%d): %v", id, err)
		}
		parts = append(parts, fmt.Sprintf("staff[%d]=%t", id, row != nil))
	}
	for _, groupID := range groupIDs {
		var link models.StaffGroupLink
		if err := db.DB.Where("group_chat_id = ?", groupID).First(&link).Error; err != nil {
			t.Fatalf("link for group %d: %v", groupID, err)
		}
		parts = append(parts, fmt.Sprintf("link[%d]->%d", groupID, link.StaffChatID))
	}
	return strings.Join(parts, " ")
}

func TestStaffMigrateEitherOrder(t *testing.T) {
	client := newStaffBotClient()
	bot := newModuleTestBot(client.moduleBotClient)
	bot.BotClient = client

	toMsg := func(oldID, newID int64) *ext.Context {
		return migrateContext(bot, gotgbot.Chat{Id: oldID, Type: "group"}, newID, 0)
	}
	fromMsg := func(oldID, newID int64) *ext.Context {
		return migrateContext(bot, staffSupergroup(newID), 0, oldID)
	}
	cases := []struct {
		name  string
		steps []func(oldID, newID int64) *ext.Context
	}{
		{"from then to", []func(int64, int64) *ext.Context{fromMsg, toMsg}},
		{"to then from", []func(int64, int64) *ext.Context{toMsg, fromMsg}},
		{"duplicate to", []func(int64, int64) *ext.Context{toMsg, toMsg}},
		{"duplicate from", []func(int64, int64) *ext.Context{fromMsg, fromMsg}},
	}

	var want string
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			oldID, newID := uniqueModuleChatID(), uniqueModuleChatID()
			groupA, groupB := uniqueModuleChatID(), uniqueModuleChatID()
			migrateSeed(t, oldID, groupA, groupB)
			staffCleanup(t, newID)

			for _, step := range tc.steps {
				if err := staffWatchersModule.onMigrateMessage(bot, step(oldID, newID)); err != ext.ContinueGroups {
					t.Fatalf("onMigrateMessage returned %v, want ext.ContinueGroups", err)
				}
			}

			if row, err := staff.GetStaffGroupFresh(newID); err != nil || row == nil {
				t.Fatalf("GetStaffGroupFresh(new) = (%+v, %v), want the row", row, err)
			}
			if row, err := staff.GetStaffGroupFresh(oldID); err != nil || row != nil {
				t.Fatalf("GetStaffGroupFresh(old) = (%+v, %v), want (nil, nil)", row, err)
			}
			for _, groupID := range []int64{groupA, groupB} {
				var link models.StaffGroupLink
				if err := db.DB.Where("group_chat_id = ?", groupID).First(&link).Error; err != nil {
					t.Fatalf("link for group %d: %v", groupID, err)
				}
				if link.StaffChatID != newID {
					t.Fatalf("link for group %d has staff_chat_id %d, want %d", groupID, link.StaffChatID, newID)
				}
			}
			got := strings.NewReplacer(
				fmt.Sprint(oldID), "OLD", fmt.Sprint(newID), "NEW",
				fmt.Sprint(groupA), "A", fmt.Sprint(groupB), "B",
			).Replace(migrateRowState(t, []int64{oldID, newID}, []int64{groupA, groupB}))
			if want == "" {
				want = got
			} else if got != want {
				t.Fatalf("end state %q differs from the first case's %q", got, want)
			}
		})
	}
}

func TestStaffMigrateUnrelatedChat(t *testing.T) {
	client := newStaffBotClient()
	bot := newModuleTestBot(client.moduleBotClient)
	bot.BotClient = client

	known, groupID := uniqueModuleChatID(), uniqueModuleChatID()
	migrateSeed(t, known, groupID)
	before := migrateRowState(t, []int64{known}, []int64{groupID})

	unrelatedOld, unrelatedNew := uniqueModuleChatID(), uniqueModuleChatID()
	for _, ctx := range []*ext.Context{
		migrateContext(bot, staffSupergroup(unrelatedOld), unrelatedNew, 0),
		migrateContext(bot, staffSupergroup(unrelatedNew), 0, unrelatedOld),
	} {
		if err := staffWatchersModule.onMigrateMessage(bot, ctx); err != ext.ContinueGroups {
			t.Fatalf("onMigrateMessage returned %v, want ext.ContinueGroups", err)
		}
	}

	if after := migrateRowState(t, []int64{known}, []int64{groupID}); after != before {
		t.Fatalf("rows changed: before %q, after %q", before, after)
	}
	if row, err := staff.GetStaffGroupFresh(unrelatedNew); err != nil || row != nil {
		t.Fatalf("a Staff Group appeared for an unrelated chat: (%+v, %v)", row, err)
	}
	if got := len(sentTexts(client)); got != 0 {
		t.Fatalf("the watcher sent %d messages, want 0", got)
	}
}

func TestStaffMigrateFromTelegramError(t *testing.T) {
	oldID, newID, groupID := uniqueModuleChatID(), uniqueModuleChatID(), uniqueModuleChatID()
	migrateSeed(t, oldID, groupID)
	staffCleanup(t, newID)

	t.Run("not a re-key", func(t *testing.T) {
		for name, err := range map[string]error{
			"nil error":          nil,
			"plain error":        errors.New("boom"),
			"400 without params": &gotgbot.TelegramError{Code: 400, Description: "Bad Request: chat not found"},
			"400 empty params": &gotgbot.TelegramError{
				Code: 400, ResponseParams: &gotgbot.ResponseParameters{RetryAfter: 5},
			},
			"wrapped plain": fmt.Errorf("wrap: %w", errors.New("boom")),
		} {
			gotID, ok := rekeyFromTelegramError(oldID, err)
			if gotID != 0 || ok {
				t.Errorf("%s: rekeyFromTelegramError = (%d, %t), want (0, false)", name, gotID, ok)
			}
		}
		if row, err := staff.GetStaffGroupFresh(oldID); err != nil || row == nil {
			t.Fatalf("the Staff Group moved without a migrate parameter: (%+v, %v)", row, err)
		}
	})

	t.Run("migrate parameter", func(t *testing.T) {
		tgErr := &gotgbot.TelegramError{
			Method:         "sendMessage",
			Code:           400,
			Description:    "Bad Request: group chat was upgraded to a supergroup chat",
			ResponseParams: &gotgbot.ResponseParameters{MigrateToChatId: newID},
		}
		gotID, ok := rekeyFromTelegramError(oldID, fmt.Errorf("send failed: %w", tgErr))
		if gotID != newID || !ok {
			t.Fatalf("rekeyFromTelegramError = (%d, %t), want (%d, true)", gotID, ok, newID)
		}
		if row, err := staff.GetStaffGroupFresh(newID); err != nil || row == nil {
			t.Fatalf("GetStaffGroupFresh(new) = (%+v, %v), want the row", row, err)
		}
		if row, err := staff.GetStaffGroupFresh(oldID); err != nil || row != nil {
			t.Fatalf("GetStaffGroupFresh(old) = (%+v, %v), want (nil, nil)", row, err)
		}
		var link models.StaffGroupLink
		if err := db.DB.Where("group_chat_id = ?", groupID).First(&link).Error; err != nil || link.StaffChatID != newID {
			t.Fatalf("link = (%+v, %v), want staff_chat_id %d", link, err, newID)
		}
	})
}
