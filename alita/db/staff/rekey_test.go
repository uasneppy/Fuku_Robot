//go:build testtools

package staff

import (
	"testing"

	"github.com/divkix/Alita_Robot/alita/db"
	"github.com/divkix/Alita_Robot/alita/db/cache"
	"github.com/divkix/Alita_Robot/alita/db/models"
	utilsCache "github.com/divkix/Alita_Robot/alita/utils/cache"
)

// rekeySeed stores a Staff Group at staffChatID with one link per group chat ID.
func rekeySeed(t *testing.T, staffChatID int64, groupChatIDs ...int64) {
	t.Helper()
	cleanupStaffRows(t, append([]int64{staffChatID}, groupChatIDs...)...)
	if err := db.DB.Create(&models.StaffGroup{ChatID: staffChatID, OwnerUserID: 1, Title: "Staff"}).Error; err != nil {
		t.Fatalf("seed staff group: %v", err)
	}
	for _, groupID := range groupChatIDs {
		link := &models.StaffGroupLink{GroupChatID: groupID, StaffChatID: staffChatID, OwnerUserID: 1}
		if err := db.DB.Create(link).Error; err != nil {
			t.Fatalf("seed link %d: %v", groupID, err)
		}
	}
}

// rekeyCountStaff counts staff_groups rows with the given chat ID.
func rekeyCountStaff(t *testing.T, chatID int64) int64 {
	t.Helper()
	var n int64
	if err := db.DB.Model(&models.StaffGroup{}).Where("chat_id = ?", chatID).Count(&n).Error; err != nil {
		t.Fatal(err)
	}
	return n
}

// rekeyCountLinks counts link rows that name chatID as staff or group chat.
func rekeyCountLinks(t *testing.T, chatID int64) int64 {
	t.Helper()
	var n int64
	err := db.DB.Model(&models.StaffGroupLink{}).
		Where("staff_chat_id = ? OR group_chat_id = ?", chatID, chatID).Count(&n).Error
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestRekeyChatMovesStaffGroupAndLinks(t *testing.T) {
	oldID, newID := uniqueStaffChatID(), uniqueStaffChatID()
	groupA, groupB := uniqueStaffChatID(), uniqueStaffChatID()
	rekeySeed(t, oldID, groupA, groupB)
	cleanupStaffRows(t, newID)

	changed, err := RekeyChat(oldID, newID)
	if err != nil || !changed {
		t.Fatalf("RekeyChat = (%v, %v), want (true, nil)", changed, err)
	}
	if rekeyCountStaff(t, oldID) != 0 || rekeyCountStaff(t, newID) != 1 {
		t.Fatal("the staff_groups row did not move from the old ID to the new ID")
	}
	if rekeyCountLinks(t, oldID) != 0 {
		t.Fatal("a link still references the old chat ID")
	}
	var links []models.StaffGroupLink
	if err := db.DB.Where("staff_chat_id = ?", newID).Find(&links).Error; err != nil {
		t.Fatal(err)
	}
	if len(links) != 2 {
		t.Fatalf("%d links carry the new staff_chat_id, want 2", len(links))
	}
}

func TestRekeyChatIdempotent(t *testing.T) {
	oldID, newID, groupID := uniqueStaffChatID(), uniqueStaffChatID(), uniqueStaffChatID()
	rekeySeed(t, oldID, groupID)
	cleanupStaffRows(t, newID)

	if changed, err := RekeyChat(oldID, newID); err != nil || !changed {
		t.Fatalf("first RekeyChat = (%v, %v), want (true, nil)", changed, err)
	}
	changed, err := RekeyChat(oldID, newID)
	if err != nil || changed {
		t.Fatalf("second RekeyChat = (%v, %v), want (false, nil)", changed, err)
	}
	if rekeyCountStaff(t, newID) != 1 || rekeyCountLinks(t, newID) != 1 || rekeyCountStaff(t, oldID) != 0 {
		t.Fatal("state changed after the repeated RekeyChat")
	}
}

func TestRekeyChatNoMatch(t *testing.T) {
	known, unknownOld, unknownNew := uniqueStaffChatID(), uniqueStaffChatID(), uniqueStaffChatID()
	groupID := uniqueStaffChatID()
	rekeySeed(t, known, groupID)

	changed, err := RekeyChat(unknownOld, unknownNew)
	if err != nil || changed {
		t.Fatalf("RekeyChat(unknown) = (%v, %v), want (false, nil)", changed, err)
	}
	if rekeyCountStaff(t, known) != 1 || rekeyCountLinks(t, known) != 1 {
		t.Fatal("rows for an unrelated chat changed")
	}
	if rekeyCountStaff(t, unknownNew) != 0 || rekeyCountLinks(t, unknownNew) != 0 {
		t.Fatal("RekeyChat created rows for an unknown chat")
	}
}

func TestRekeyChatIgnoresZeroAndEqualIDs(t *testing.T) {
	chatID, groupID := uniqueStaffChatID(), uniqueStaffChatID()
	rekeySeed(t, chatID, groupID)

	for _, pair := range [][2]int64{{chatID, chatID}, {0, chatID}, {chatID, 0}, {0, 0}} {
		changed, err := RekeyChat(pair[0], pair[1])
		if err != nil || changed {
			t.Errorf("RekeyChat(%d, %d) = (%v, %v), want (false, nil)", pair[0], pair[1], changed, err)
		}
	}
	if rekeyCountStaff(t, chatID) != 1 || rekeyCountLinks(t, chatID) != 1 {
		t.Fatal("rows changed for a zero or equal ID pair")
	}
}

func TestRekeyChatLinkedGroupID(t *testing.T) {
	staffID, oldGroup, newGroup := uniqueStaffChatID(), uniqueStaffChatID(), uniqueStaffChatID()
	rekeySeed(t, staffID, oldGroup)
	cleanupStaffRows(t, newGroup)

	changed, err := RekeyChat(oldGroup, newGroup)
	if err != nil || !changed {
		t.Fatalf("RekeyChat = (%v, %v), want (true, nil)", changed, err)
	}
	var link models.StaffGroupLink
	if err := db.DB.Where("group_chat_id = ?", newGroup).First(&link).Error; err != nil {
		t.Fatalf("link not found under the new group ID: %v", err)
	}
	if link.StaffChatID != staffID {
		t.Fatalf("staff_chat_id = %d, want it untouched at %d", link.StaffChatID, staffID)
	}
	if rekeyCountLinks(t, oldGroup) != 0 {
		t.Fatal("a link still references the old group ID")
	}
	if rekeyCountStaff(t, staffID) != 1 {
		t.Fatal("the Staff Group row changed when only a linked group was re-keyed")
	}
}

func TestRekeyChatKeepsDistinctCheck(t *testing.T) {
	oldID, newID := uniqueStaffChatID(), uniqueStaffChatID()
	groupA, groupB := uniqueStaffChatID(), uniqueStaffChatID()
	rekeySeed(t, oldID, groupA, groupB)
	cleanupStaffRows(t, newID)

	if _, err := RekeyChat(oldID, newID); err != nil {
		t.Fatalf("RekeyChat: %v", err)
	}
	var selfLinks int64
	err := db.DB.Model(&models.StaffGroupLink{}).
		Where("group_chat_id = staff_chat_id AND (group_chat_id IN ? OR staff_chat_id IN ?)",
			[]int64{oldID, newID, groupA, groupB}, []int64{oldID, newID, groupA, groupB}).
		Count(&selfLinks).Error
	if err != nil {
		t.Fatal(err)
	}
	if selfLinks != 0 {
		t.Fatalf("%d links have group_chat_id == staff_chat_id after the re-key", selfLinks)
	}

	self := &models.StaffGroupLink{GroupChatID: newID, StaffChatID: newID, OwnerUserID: 1}
	if err := db.DB.Create(self).Error; err == nil {
		t.Fatal("a direct self-link insert was accepted; the CHECK is gone")
	}
}

// A re-key that would collapse a link into a self-link must fail as a whole:
// the staff_groups update that ran first is rolled back too.
func TestRekeyChatRollsBackOnConstraintViolation(t *testing.T) {
	oldID, groupID := uniqueStaffChatID(), uniqueStaffChatID()
	rekeySeed(t, oldID, groupID)

	changed, err := RekeyChat(oldID, groupID)
	if err == nil || changed {
		t.Fatalf("RekeyChat = (%v, %v), want (false, error)", changed, err)
	}
	if rekeyCountStaff(t, oldID) != 1 || rekeyCountStaff(t, groupID) != 0 {
		t.Fatal("the staff_groups row moved although the transaction failed")
	}
	var link models.StaffGroupLink
	if err := db.DB.Where("group_chat_id = ?", groupID).First(&link).Error; err != nil || link.StaffChatID != oldID {
		t.Fatalf("link = (%+v, %v), want it untouched under staff chat %d", link, err, oldID)
	}
}

func TestRekeyChatInvalidatesBothIDs(t *testing.T) {
	utilsCache.SetupTestMemoryMarshaler(t)
	cache.ResetLocalForTest()
	oldID, newID := uniqueStaffChatID(), uniqueStaffChatID()
	rekeySeed(t, oldID)
	cleanupStaffRows(t, newID)

	// Prime both entries: the old ID caches the row, the new ID caches "not found".
	if GetStaffGroup(oldID) == nil {
		t.Fatal("GetStaffGroup(old) = nil before the re-key")
	}
	if GetStaffGroup(newID) != nil {
		t.Fatal("GetStaffGroup(new) != nil before the re-key")
	}

	if changed, err := RekeyChat(oldID, newID); err != nil || !changed {
		t.Fatalf("RekeyChat = (%v, %v), want (true, nil)", changed, err)
	}
	if got := GetStaffGroup(oldID); got != nil {
		t.Fatalf("GetStaffGroup(old) = %+v after the re-key, want nil (stale cache)", got)
	}
	got := GetStaffGroup(newID)
	if got == nil || got.ChatID != newID {
		t.Fatalf("GetStaffGroup(new) = %+v after the re-key, want the moved row (stale sentinel)", got)
	}
}

// A re-key moves the history of the Staff Group with it, but not the message it
// was summarised in or the groups it acted on: a message ID only names a message
// in the chat it was sent to, and a restriction lives in the group it was made in.
func TestRekeyChatMovesStaffActions(t *testing.T) {
	oldID, newID, groupID := uniqueStaffChatID(), uniqueStaffChatID(), uniqueStaffChatID()
	rekeySeed(t, oldID)
	cleanupStaffRows(t, newID)
	cleanupActionRows(t, oldID, newID)

	action := &models.StaffAction{
		StaffChatID:   oldID,
		IssuerUserID:  1,
		TargetUserID:  2,
		Action:        "ban",
		GroupCount:    1,
		SummaryChatID: oldID,
		SummaryMsgID:  5,
	}
	groups := []models.StaffActionGroup{{Seq: 0, GroupChatID: groupID, Outcome: models.StaffActionOutcomePending}}
	if err := CreateAction(action, groups); err != nil {
		t.Fatalf("CreateAction: %v", err)
	}

	changed, err := RekeyChat(oldID, newID)
	if err != nil || !changed {
		t.Fatalf("RekeyChat = (%v, %v), want (true, nil)", changed, err)
	}

	got, err := GetActionFresh(action.ID)
	if err != nil || got == nil {
		t.Fatalf("GetActionFresh = %v, %v", got, err)
	}
	if got.StaffChatID != newID {
		t.Fatalf("staff_chat_id = %d, want it moved to %d", got.StaffChatID, newID)
	}
	if got.SummaryChatID != oldID || got.SummaryMsgID != 5 {
		t.Fatalf("summary = %d/%d, want it left at %d/5: the message lives in the old chat", got.SummaryChatID, got.SummaryMsgID, oldID)
	}
	rows, err := ListActionGroupsFresh(action.ID)
	if err != nil || len(rows) != 1 || rows[0].GroupChatID != groupID {
		t.Fatalf("group rows = %+v, %v, want the one row still on group %d", rows, err, groupID)
	}
}
