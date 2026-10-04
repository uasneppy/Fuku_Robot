//go:build testtools

package staff

import (
	"crypto/rand"
	"encoding/binary"
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
