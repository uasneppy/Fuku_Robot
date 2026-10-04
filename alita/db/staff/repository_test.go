//go:build testtools

package staff

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/divkix/Alita_Robot/alita/db"
	"github.com/divkix/Alita_Robot/alita/db/backup"
	"github.com/divkix/Alita_Robot/alita/db/cache"
	"github.com/divkix/Alita_Robot/alita/db/models"
	utilsCache "github.com/divkix/Alita_Robot/alita/utils/cache"
)

// uniqueStaffChatID returns a random large negative chat ID, so tests sharing
// one database never collide.
func uniqueStaffChatID() int64 {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		panic("uniqueStaffChatID: crypto/rand failed: " + err.Error())
	}
	n := int64(binary.BigEndian.Uint64(buf[:]) & 0x7fffffffffffffff)
	return -1000000000000 - n
}

// cleanupStaffRows deletes staff rows touching chatIDs when the test ends.
func cleanupStaffRows(t *testing.T, chatIDs ...int64) {
	t.Helper()
	t.Cleanup(func() {
		db.DB.Where("chat_id IN ?", chatIDs).Delete(&models.StaffGroup{})
		db.DB.Where("group_chat_id IN ? OR staff_chat_id IN ?", chatIDs, chatIDs).Delete(&models.StaffGroupLink{})
	})
}

func TestStaffConstraintsRejectDuplicateStaffChat(t *testing.T) {
	chatID := uniqueStaffChatID()
	cleanupStaffRows(t, chatID)

	if err := db.DB.Create(&models.StaffGroup{ChatID: chatID, OwnerUserID: 1}).Error; err != nil {
		t.Fatalf("first insert: %v", err)
	}
	if err := db.DB.Create(&models.StaffGroup{ChatID: chatID, OwnerUserID: 2}).Error; err == nil {
		t.Fatal("second staff_groups row with the same chat_id was accepted")
	}
}

func TestStaffConstraintsRejectSecondLinkForGroup(t *testing.T) {
	groupID, staffA, staffB := uniqueStaffChatID(), uniqueStaffChatID(), uniqueStaffChatID()
	cleanupStaffRows(t, groupID, staffA, staffB)

	first := &models.StaffGroupLink{GroupChatID: groupID, StaffChatID: staffA, OwnerUserID: 1}
	if err := db.DB.Create(first).Error; err != nil {
		t.Fatalf("first link: %v", err)
	}
	second := &models.StaffGroupLink{GroupChatID: groupID, StaffChatID: staffB, OwnerUserID: 1}
	if err := db.DB.Create(second).Error; err == nil {
		t.Fatal("a second link for the same group_chat_id was accepted")
	}
}

func TestStaffConstraintsRejectSelfLink(t *testing.T) {
	chatID := uniqueStaffChatID()
	cleanupStaffRows(t, chatID)

	self := &models.StaffGroupLink{GroupChatID: chatID, StaffChatID: chatID, OwnerUserID: 1}
	if err := db.DB.Create(self).Error; err == nil {
		t.Fatal("a link from a group to itself was accepted (chk_staff_link_distinct)")
	}
}

func TestStaffConstraintsRejectUnknownHealth(t *testing.T) {
	groupID, staffID := uniqueStaffChatID(), uniqueStaffChatID()
	cleanupStaffRows(t, groupID, staffID)

	link := &models.StaffGroupLink{GroupChatID: groupID, StaffChatID: staffID, OwnerUserID: 1, Health: "broken"}
	if err := db.DB.Create(link).Error; err == nil {
		t.Fatal(`a link with health "broken" was accepted (chk_staff_link_health)`)
	}
	for _, health := range []string{
		models.StaffHealthOK,
		models.StaffHealthBotMissing,
		models.StaffHealthBotNotAdmin,
		models.StaffHealthBotCannotRestrict,
	} {
		group := uniqueStaffChatID()
		cleanupStaffRows(t, group)
		ok := &models.StaffGroupLink{GroupChatID: group, StaffChatID: staffID, OwnerUserID: 1, Health: health}
		if err := db.DB.Create(ok).Error; err != nil {
			t.Errorf("health %q rejected: %v", health, err)
		}
	}
}

func TestStaffRepoCreateStaffGroupIdempotent(t *testing.T) {
	chatID := uniqueStaffChatID()
	cleanupStaffRows(t, chatID)

	created, err := CreateStaffGroup(chatID, 7, "Staff")
	if err != nil || !created {
		t.Fatalf("first CreateStaffGroup = (%v, %v), want (true, nil)", created, err)
	}
	created, err = CreateStaffGroup(chatID, 8, "Other")
	if err != nil || created {
		t.Fatalf("second CreateStaffGroup = (%v, %v), want (false, nil)", created, err)
	}

	var rows []models.StaffGroup
	if err := db.DB.Where("chat_id = ?", chatID).Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].OwnerUserID != 7 {
		t.Fatalf("rows = %+v, want exactly one row still owned by 7", rows)
	}
}

func TestStaffRepoCreateStaffGroupRoleConflict(t *testing.T) {
	linkedGroup, otherStaff := uniqueStaffChatID(), uniqueStaffChatID()
	cleanupStaffRows(t, linkedGroup, otherStaff)

	link := &models.StaffGroupLink{GroupChatID: linkedGroup, StaffChatID: otherStaff, OwnerUserID: 1}
	if err := db.DB.Create(link).Error; err != nil {
		t.Fatalf("create link: %v", err)
	}

	created, err := CreateStaffGroup(linkedGroup, 7, "Linked")
	if !errors.Is(err, ErrRoleConflict) || created {
		t.Fatalf("CreateStaffGroup(linked group) = (%v, %v), want (false, ErrRoleConflict)", created, err)
	}
	if row, err := GetStaffGroupFresh(linkedGroup); err != nil || row != nil {
		t.Fatalf("a refused CreateStaffGroup inserted %+v (err %v)", row, err)
	}

	// The refusal is about this chat only: the Staff Group side of the link and
	// unrelated chats are unaffected.
	if created, err := CreateStaffGroup(otherStaff, 7, "Other"); err != nil || !created {
		t.Fatalf("CreateStaffGroup(staff side of the link) = (%v, %v), want (true, nil)", created, err)
	}
}

func TestStaffRepoGetStaffGroupSentinelAndInvalidation(t *testing.T) {
	utilsCache.SetupTestMemoryMarshaler(t)
	cache.ResetLocalForTest()
	chatID := uniqueStaffChatID()
	cleanupStaffRows(t, chatID)

	if got := GetStaffGroup(chatID); got != nil {
		t.Fatalf("GetStaffGroup before create = %+v, want nil", got)
	}

	// A row written behind the repository's back must stay hidden by the cached
	// "not found" sentinel; this proves the sentinel is really cached.
	hidden := uniqueStaffChatID()
	cleanupStaffRows(t, hidden)
	if got := GetStaffGroup(hidden); got != nil {
		t.Fatalf("GetStaffGroup(hidden) = %+v, want nil", got)
	}
	if err := db.DB.Create(&models.StaffGroup{ChatID: hidden, OwnerUserID: 1}).Error; err != nil {
		t.Fatal(err)
	}
	if got := GetStaffGroup(hidden); got != nil {
		t.Fatal("cached sentinel was not served; the gate is not cached")
	}

	if _, err := CreateStaffGroup(chatID, 7, "Staff"); err != nil {
		t.Fatalf("CreateStaffGroup: %v", err)
	}
	got := GetStaffGroup(chatID)
	if got == nil || got.OwnerUserID != 7 {
		t.Fatalf("GetStaffGroup after create = %+v, want the new row (the write must invalidate)", got)
	}

	fresh, err := GetStaffGroupFresh(uniqueStaffChatID())
	if fresh != nil || err != nil {
		t.Fatalf("GetStaffGroupFresh(missing) = (%+v, %v), want (nil, nil)", fresh, err)
	}
	fresh, err = GetStaffGroupFresh(chatID)
	if err != nil || fresh == nil || fresh.OwnerUserID != 7 {
		t.Fatalf("GetStaffGroupFresh(existing) = (%+v, %v)", fresh, err)
	}
	if GetStaffGroup(0) != nil {
		t.Fatal("GetStaffGroup(0) must be nil")
	}
}

func TestStaffRepoTrimStaffTitle(t *testing.T) {
	chatID := uniqueStaffChatID()
	cleanupStaffRows(t, chatID)

	title := strings.Repeat("é", 35) + strings.Repeat("日", 35) // 70 runes, multi-byte
	if _, err := CreateStaffGroup(chatID, 7, "  "+title+"  "); err != nil {
		t.Fatalf("CreateStaffGroup: %v", err)
	}
	row, err := GetStaffGroupFresh(chatID)
	if err != nil || row == nil {
		t.Fatalf("GetStaffGroupFresh = (%+v, %v)", row, err)
	}
	if n := utf8.RuneCountInString(row.Title); n != 64 {
		t.Fatalf("stored title has %d runes, want 64", n)
	}
	if !utf8.ValidString(row.Title) {
		t.Fatal("stored title is not valid UTF-8")
	}
}

func TestStaffRepoDeleteStaffGroupWithLinksOnlyTarget(t *testing.T) {
	utilsCache.SetupTestMemoryMarshaler(t)
	cache.ResetLocalForTest()
	staffA, staffB := uniqueStaffChatID(), uniqueStaffChatID()
	groupA1, groupA2, groupB1 := uniqueStaffChatID(), uniqueStaffChatID(), uniqueStaffChatID()
	cleanupStaffRows(t, staffA, staffB, groupA1, groupA2, groupB1)

	for _, chatID := range []int64{staffA, staffB} {
		if _, err := CreateStaffGroup(chatID, 7, "Staff"); err != nil {
			t.Fatalf("CreateStaffGroup(%d): %v", chatID, err)
		}
	}
	// Insert in an order where link ids ascend but chat IDs do not, so a sort by
	// chat ID would be caught.
	links := []*models.StaffGroupLink{
		{GroupChatID: groupA2, StaffChatID: staffA, OwnerUserID: 7, GroupTitle: "A2"},
		{GroupChatID: groupB1, StaffChatID: staffB, OwnerUserID: 7, GroupTitle: "B1"},
		{GroupChatID: groupA1, StaffChatID: staffA, OwnerUserID: 7, GroupTitle: "A1"},
	}
	for _, link := range links {
		if err := db.DB.Create(link).Error; err != nil {
			t.Fatalf("create link: %v", err)
		}
	}

	removed, deleted, err := DeleteStaffGroupWithLinks(staffA)
	if err != nil || !deleted {
		t.Fatalf("DeleteStaffGroupWithLinks = (%v, %v, %v), want deleted with nil error", removed, deleted, err)
	}
	if len(removed) != 2 || removed[0].GroupChatID != groupA2 || removed[1].GroupChatID != groupA1 {
		t.Fatalf("removed = %+v, want [A2, A1] in link-id order", removed)
	}
	if removed[0].ID >= removed[1].ID {
		t.Fatalf("removed ids %d, %d are not ascending", removed[0].ID, removed[1].ID)
	}

	var left []models.StaffGroupLink
	if err := db.DB.Where("staff_chat_id IN ?", []int64{staffA, staffB}).Find(&left).Error; err != nil {
		t.Fatal(err)
	}
	if len(left) != 1 || left[0].GroupChatID != groupB1 {
		t.Fatalf("links left = %+v, want only the other Staff Group's link", left)
	}
	if got := GetStaffGroup(staffA); got != nil {
		t.Fatalf("GetStaffGroup after delete = %+v, want nil (cache must be invalidated)", got)
	}
	if got := GetStaffGroup(staffB); got == nil {
		t.Fatal("the other Staff Group of the same owner was removed")
	}
}

func TestStaffRepoDeleteStaffGroupWithLinksSecondCallIsNoop(t *testing.T) {
	staffID, groupID := uniqueStaffChatID(), uniqueStaffChatID()
	cleanupStaffRows(t, staffID, groupID)

	if _, err := CreateStaffGroup(staffID, 7, "Staff"); err != nil {
		t.Fatal(err)
	}
	if err := db.DB.Create(&models.StaffGroupLink{GroupChatID: groupID, StaffChatID: staffID, OwnerUserID: 7}).Error; err != nil {
		t.Fatal(err)
	}

	if _, deleted, err := DeleteStaffGroupWithLinks(staffID); err != nil || !deleted {
		t.Fatalf("first call = (deleted %v, err %v), want deleted", deleted, err)
	}
	removed, deleted, err := DeleteStaffGroupWithLinks(staffID)
	if err != nil || deleted || len(removed) != 0 {
		t.Fatalf("second call = (%v, %v, %v), want (empty, false, nil)", removed, deleted, err)
	}
}

func TestStaffRepoDeleteStaffGroupWithLinksZeroLinks(t *testing.T) {
	utilsCache.SetupTestMemoryMarshaler(t)
	cache.ResetLocalForTest()
	staffID := uniqueStaffChatID()
	cleanupStaffRows(t, staffID)

	if _, err := CreateStaffGroup(staffID, 7, "Staff"); err != nil {
		t.Fatal(err)
	}
	if GetStaffGroup(staffID) == nil {
		t.Fatal("setup: GetStaffGroup returned nil for a created Staff Group")
	}

	removed, deleted, err := DeleteStaffGroupWithLinks(staffID)
	if err != nil || !deleted || len(removed) != 0 {
		t.Fatalf("DeleteStaffGroupWithLinks = (%v, %v, %v), want (empty, true, nil)", removed, deleted, err)
	}
	if got := GetStaffGroup(staffID); got != nil {
		t.Fatalf("GetStaffGroup after delete = %+v, want nil", got)
	}
	if fresh, err := GetStaffGroupFresh(staffID); err != nil || fresh != nil {
		t.Fatalf("GetStaffGroupFresh after delete = (%+v, %v), want (nil, nil)", fresh, err)
	}
}

func TestStaffRepoDeleteStaffGroupWithLinksMissingChat(t *testing.T) {
	removed, deleted, err := DeleteStaffGroupWithLinks(uniqueStaffChatID())
	if err != nil || deleted || len(removed) != 0 {
		t.Fatalf("delete of a non-Staff chat = (%v, %v, %v), want (empty, false, nil)", removed, deleted, err)
	}
}

func TestStaffRepoCountLinksByStaffFresh(t *testing.T) {
	staffID, g1, g2 := uniqueStaffChatID(), uniqueStaffChatID(), uniqueStaffChatID()
	cleanupStaffRows(t, staffID, g1, g2)

	if n, err := CountLinksByStaffFresh(staffID); err != nil || n != 0 {
		t.Fatalf("count with no links = (%d, %v), want (0, nil)", n, err)
	}
	for _, g := range []int64{g1, g2} {
		if err := db.DB.Create(&models.StaffGroupLink{GroupChatID: g, StaffChatID: staffID, OwnerUserID: 7}).Error; err != nil {
			t.Fatal(err)
		}
	}
	if n, err := CountLinksByStaffFresh(staffID); err != nil || n != 2 {
		t.Fatalf("count = (%d, %v), want (2, nil)", n, err)
	}
}

// Backup export, import and reset must never create, restore or erase Staff
// Groups or links, so no backup module may name them.
func TestStaffTablesStayOutOfBackup(t *testing.T) {
	for _, name := range []string{"staff", "staff_groups", "staff_group_links", "staff_group", "staff_links"} {
		if backup.IsValidModule(name) {
			t.Errorf("backup module %q is valid; Staff Groups must stay out of backup", name)
		}
	}
	for _, module := range backup.AllExportableModules() {
		if strings.Contains(strings.ToLower(module), "staff") {
			t.Errorf("exportable backup module %q mentions staff", module)
		}
	}
}
