//go:build testtools

package staff

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"strings"
	"sync"
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

func TestStaffRepoCreateLinkRefusesMissingStaffGroup(t *testing.T) {
	staffID, groupID := uniqueStaffChatID(), uniqueStaffChatID()
	cleanupStaffRows(t, staffID, groupID)

	err := CreateLink(&models.StaffGroupLink{GroupChatID: groupID, StaffChatID: staffID, OwnerUserID: 7})
	if !errors.Is(err, ErrStaffGroupMissing) {
		t.Fatalf("CreateLink(no Staff Group) = %v, want ErrStaffGroupMissing", err)
	}
	if link, err := GetLinkOfGroupFresh(groupID); err != nil || link != nil {
		t.Fatalf("a refused CreateLink inserted %+v (err %v)", link, err)
	}
}

func TestStaffRepoCreateLinkRefusesStaffGroupAsLinkedGroup(t *testing.T) {
	staffID, groupID := uniqueStaffChatID(), uniqueStaffChatID()
	cleanupStaffRows(t, staffID, groupID)
	for _, id := range []int64{staffID, groupID} {
		if _, err := CreateStaffGroup(id, 7, "Staff"); err != nil {
			t.Fatalf("CreateStaffGroup(%d): %v", id, err)
		}
	}

	err := CreateLink(&models.StaffGroupLink{GroupChatID: groupID, StaffChatID: staffID, OwnerUserID: 7})
	if !errors.Is(err, ErrRoleConflict) {
		t.Fatalf("CreateLink(group is a Staff Group) = %v, want ErrRoleConflict", err)
	}
	if link, err := GetLinkOfGroupFresh(groupID); err != nil || link != nil {
		t.Fatalf("a refused CreateLink inserted %+v (err %v)", link, err)
	}
}

func TestStaffRepoCreateLinkStoresTrimmedTitleAndInvalidatesGate(t *testing.T) {
	utilsCache.SetupTestMemoryMarshaler(t)
	cache.ResetLocalForTest()
	staffID, groupID := uniqueStaffChatID(), uniqueStaffChatID()
	cleanupStaffRows(t, staffID, groupID)
	if _, err := CreateStaffGroup(staffID, 7, "Staff"); err != nil {
		t.Fatal(err)
	}

	// Cache the "not linked" sentinel; the write must invalidate it.
	if got := GetLinkOfGroup(groupID); got != nil {
		t.Fatalf("GetLinkOfGroup before link = %+v, want nil", got)
	}
	title := strings.Repeat("日", 70)
	link := &models.StaffGroupLink{
		GroupChatID: groupID, StaffChatID: staffID, OwnerUserID: 7, GroupTitle: "  " + title + "  ", Health: models.StaffHealthBotNotAdmin,
	}
	if err := CreateLink(link); err != nil {
		t.Fatalf("CreateLink: %v", err)
	}

	got := GetLinkOfGroup(groupID)
	if got == nil || got.StaffChatID != staffID || got.OwnerUserID != 7 || got.Health != models.StaffHealthBotNotAdmin {
		t.Fatalf("GetLinkOfGroup after link = %+v, want the new link (the write must invalidate)", got)
	}
	if utf8.RuneCountInString(got.GroupTitle) != 64 {
		t.Fatalf("stored title has %d runes, want 64", utf8.RuneCountInString(got.GroupTitle))
	}
	fresh, err := GetLinkOfGroupFresh(groupID)
	if err != nil || fresh == nil || fresh.ID != got.ID {
		t.Fatalf("GetLinkOfGroupFresh = (%+v, %v), want the same link", fresh, err)
	}
	if none, err := GetLinkOfGroupFresh(uniqueStaffChatID()); err != nil || none != nil {
		t.Fatalf("GetLinkOfGroupFresh(unlinked) = (%+v, %v), want (nil, nil)", none, err)
	}
	if GetLinkOfGroup(0) != nil {
		t.Fatal("GetLinkOfGroup(0) must be nil")
	}
}

func TestStaffRepoCreateLinkSecondInsertIsAlreadyLinked(t *testing.T) {
	staffA, staffB, groupID := uniqueStaffChatID(), uniqueStaffChatID(), uniqueStaffChatID()
	cleanupStaffRows(t, staffA, staffB, groupID)
	for _, id := range []int64{staffA, staffB} {
		if _, err := CreateStaffGroup(id, 7, "Staff"); err != nil {
			t.Fatal(err)
		}
	}
	if err := CreateLink(&models.StaffGroupLink{GroupChatID: groupID, StaffChatID: staffA, OwnerUserID: 7}); err != nil {
		t.Fatalf("first CreateLink: %v", err)
	}

	err := CreateLink(&models.StaffGroupLink{GroupChatID: groupID, StaffChatID: staffB, OwnerUserID: 8})
	if !errors.Is(err, ErrAlreadyLinked) {
		t.Fatalf("second CreateLink = %v, want ErrAlreadyLinked", err)
	}
	link, err := GetLinkOfGroupFresh(groupID)
	if err != nil || link == nil || link.StaffChatID != staffA || link.OwnerUserID != 7 {
		t.Fatalf("link = (%+v, %v), want the first link untouched", link, err)
	}
}

// Two writers racing for the same group, through the same or different Staff
// Groups, end with exactly one row: one nil error, one ErrAlreadyLinked.
func TestStaffRepoCreateLinkConcurrentSameGroupMakesOneRow(t *testing.T) {
	for round := 0; round < 10; round++ {
		staffA, staffB, groupID := uniqueStaffChatID(), uniqueStaffChatID(), uniqueStaffChatID()
		cleanupStaffRows(t, staffA, staffB, groupID)
		for _, id := range []int64{staffA, staffB} {
			if _, err := CreateStaffGroup(id, 7, "Staff"); err != nil {
				t.Fatal(err)
			}
		}

		errs := make([]error, 2)
		start := make(chan struct{})
		var wg sync.WaitGroup
		for i, staffID := range []int64{staffA, staffB} {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				errs[i] = CreateLink(&models.StaffGroupLink{GroupChatID: groupID, StaffChatID: staffID, OwnerUserID: 7})
			}()
		}
		close(start)
		wg.Wait()

		var nilCount, alreadyCount int
		for _, err := range errs {
			switch {
			case err == nil:
				nilCount++
			case errors.Is(err, ErrAlreadyLinked):
				alreadyCount++
			default:
				t.Fatalf("round %d: unexpected error %v", round, err)
			}
		}
		if nilCount != 1 || alreadyCount != 1 {
			t.Fatalf("round %d: errors = %v, want exactly one nil and one ErrAlreadyLinked", round, errs)
		}
		var rows int64
		if err := db.DB.Model(&models.StaffGroupLink{}).Where("group_chat_id = ?", groupID).Count(&rows).Error; err != nil || rows != 1 {
			t.Fatalf("round %d: rows = %d (err %v), want exactly one", round, rows, err)
		}
	}
}

func TestStaffRepoListLinksByStaffFresh(t *testing.T) {
	staffA, staffB, g1, g2, g3 := uniqueStaffChatID(), uniqueStaffChatID(), uniqueStaffChatID(), uniqueStaffChatID(), uniqueStaffChatID()
	cleanupStaffRows(t, staffA, staffB, g1, g2, g3)
	for _, id := range []int64{staffA, staffB} {
		if _, err := CreateStaffGroup(id, 7, "Staff"); err != nil {
			t.Fatal(err)
		}
	}
	for _, link := range []*models.StaffGroupLink{
		{GroupChatID: g1, StaffChatID: staffA, OwnerUserID: 7, GroupTitle: "One"},
		{GroupChatID: g2, StaffChatID: staffB, OwnerUserID: 7, GroupTitle: "Other staff"},
		{GroupChatID: g3, StaffChatID: staffA, OwnerUserID: 7, GroupTitle: "Three"},
	} {
		if err := CreateLink(link); err != nil {
			t.Fatal(err)
		}
	}

	links, err := ListLinksByStaffFresh(staffA)
	if err != nil || len(links) != 2 || links[0].GroupChatID != g1 || links[1].GroupChatID != g3 {
		t.Fatalf("ListLinksByStaffFresh = (%+v, %v), want g1 then g3 in id order", links, err)
	}
	if none, err := ListLinksByStaffFresh(uniqueStaffChatID()); err != nil || len(none) != 0 {
		t.Fatalf("ListLinksByStaffFresh(unknown) = (%+v, %v), want empty", none, err)
	}
}

func TestStaffRepoDeleteLinkOnlyNamedRow(t *testing.T) {
	utilsCache.SetupTestMemoryMarshaler(t)
	cache.ResetLocalForTest()
	staffID, g1, g2 := uniqueStaffChatID(), uniqueStaffChatID(), uniqueStaffChatID()
	cleanupStaffRows(t, staffID, g1, g2)
	if _, err := CreateStaffGroup(staffID, 7, "Staff"); err != nil {
		t.Fatal(err)
	}
	first := &models.StaffGroupLink{GroupChatID: g1, StaffChatID: staffID, OwnerUserID: 7, GroupTitle: "One"}
	second := &models.StaffGroupLink{GroupChatID: g2, StaffChatID: staffID, OwnerUserID: 7, GroupTitle: "Two"}
	for _, link := range []*models.StaffGroupLink{first, second} {
		if err := CreateLink(link); err != nil {
			t.Fatal(err)
		}
	}
	// Cache both lookups, so the delete has something to invalidate.
	if GetLinkOfGroup(g1) == nil || GetLinkOfGroup(g2) == nil {
		t.Fatal("both groups should be linked before the delete")
	}

	deleted, err := DeleteLink(first.ID)
	if err != nil || !deleted {
		t.Fatalf("DeleteLink(first) = (%v, %v), want (true, nil)", deleted, err)
	}
	if got := GetLinkOfGroup(g1); got != nil {
		t.Fatalf("GetLinkOfGroup(deleted) = %+v, want nil (the delete must invalidate the cache)", got)
	}
	if got := GetLinkOfGroup(g2); got == nil || got.ID != second.ID {
		t.Fatalf("GetLinkOfGroup(other) = %+v, want the untouched link", got)
	}
	if staffRow, err := GetStaffGroupFresh(staffID); err != nil || staffRow == nil {
		t.Fatalf("the Staff Group itself must stay: (%+v, %v)", staffRow, err)
	}
	links, err := ListLinksByStaffFresh(staffID)
	if err != nil || len(links) != 1 || links[0].ID != second.ID {
		t.Fatalf("remaining links = (%+v, %v), want only the second", links, err)
	}
}

func TestStaffRepoDeleteLinkSecondCallIsNoop(t *testing.T) {
	staffID, groupID := uniqueStaffChatID(), uniqueStaffChatID()
	cleanupStaffRows(t, staffID, groupID)
	if _, err := CreateStaffGroup(staffID, 7, "Staff"); err != nil {
		t.Fatal(err)
	}
	link := &models.StaffGroupLink{GroupChatID: groupID, StaffChatID: staffID, OwnerUserID: 7}
	if err := CreateLink(link); err != nil {
		t.Fatal(err)
	}

	if deleted, err := DeleteLink(link.ID); err != nil || !deleted {
		t.Fatalf("first DeleteLink = (%v, %v), want (true, nil)", deleted, err)
	}
	if deleted, err := DeleteLink(link.ID); err != nil || deleted {
		t.Fatalf("second DeleteLink = (%v, %v), want (false, nil)", deleted, err)
	}
	if got, err := GetLinkOfGroupFresh(groupID); err != nil || got != nil {
		t.Fatalf("GetLinkOfGroupFresh after delete = (%+v, %v), want (nil, nil)", got, err)
	}
	if deleted, err := DeleteLink(0); err != nil || deleted {
		t.Fatalf("DeleteLink(0) = (%v, %v), want (false, nil)", deleted, err)
	}
}

func TestStaffRepoDeleteLinkConcurrentOneWinner(t *testing.T) {
	staffID, groupID := uniqueStaffChatID(), uniqueStaffChatID()
	cleanupStaffRows(t, staffID, groupID)
	if _, err := CreateStaffGroup(staffID, 7, "Staff"); err != nil {
		t.Fatal(err)
	}
	link := &models.StaffGroupLink{GroupChatID: groupID, StaffChatID: staffID, OwnerUserID: 7}
	if err := CreateLink(link); err != nil {
		t.Fatal(err)
	}

	const callers = 4
	var wins, failures int
	var mu sync.Mutex
	var wg sync.WaitGroup
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			deleted, err := DeleteLink(link.ID)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				failures++
			}
			if deleted {
				wins++
			}
		}()
	}
	wg.Wait()
	if wins != 1 {
		t.Fatalf("%d callers reported a delete (%d errors), want exactly 1", wins, failures)
	}
}

func TestStaffRepoGetLinkByIDFresh(t *testing.T) {
	staffID, groupID := uniqueStaffChatID(), uniqueStaffChatID()
	cleanupStaffRows(t, staffID, groupID)
	if _, err := CreateStaffGroup(staffID, 7, "Staff"); err != nil {
		t.Fatal(err)
	}
	link := &models.StaffGroupLink{GroupChatID: groupID, StaffChatID: staffID, OwnerUserID: 7, GroupTitle: "One"}
	if err := CreateLink(link); err != nil {
		t.Fatal(err)
	}

	got, err := GetLinkByIDFresh(link.ID)
	if err != nil || got == nil || got.GroupChatID != groupID || got.StaffChatID != staffID {
		t.Fatalf("GetLinkByIDFresh = (%+v, %v), want the link", got, err)
	}
	if none, err := GetLinkByIDFresh(link.ID + 1_000_000); err != nil || none != nil {
		t.Fatalf("GetLinkByIDFresh(unknown) = (%+v, %v), want (nil, nil)", none, err)
	}
}

func TestStaffRepoDeleteLinkIfOwnerOnlyWhenMakerMatches(t *testing.T) {
	utilsCache.SetupTestMemoryMarshaler(t)
	cache.ResetLocalForTest()
	staffID, g1, g2 := uniqueStaffChatID(), uniqueStaffChatID(), uniqueStaffChatID()
	cleanupStaffRows(t, staffID, g1, g2)
	if _, err := CreateStaffGroup(staffID, 7, "Staff"); err != nil {
		t.Fatal(err)
	}
	first := &models.StaffGroupLink{GroupChatID: g1, StaffChatID: staffID, OwnerUserID: 7, GroupTitle: "One"}
	second := &models.StaffGroupLink{GroupChatID: g2, StaffChatID: staffID, OwnerUserID: 7, GroupTitle: "Two"}
	for _, link := range []*models.StaffGroupLink{first, second} {
		if err := CreateLink(link); err != nil {
			t.Fatal(err)
		}
	}
	if GetLinkOfGroup(g1) == nil || GetLinkOfGroup(g2) == nil {
		t.Fatal("both groups should be linked before the delete")
	}

	if deleted, err := DeleteLinkIfOwner(first.ID, 8); err != nil || deleted {
		t.Fatalf("DeleteLinkIfOwner(wrong maker) = (%v, %v), want (false, nil)", deleted, err)
	}
	if got, err := GetLinkByIDFresh(first.ID); err != nil || got == nil {
		t.Fatalf("the link must survive a wrong-maker delete: (%+v, %v)", got, err)
	}

	deleted, err := DeleteLinkIfOwner(first.ID, 7)
	if err != nil || !deleted {
		t.Fatalf("DeleteLinkIfOwner(right maker) = (%v, %v), want (true, nil)", deleted, err)
	}
	if got := GetLinkOfGroup(g1); got != nil {
		t.Fatalf("GetLinkOfGroup(deleted) = %+v, want nil (the delete must invalidate the cache)", got)
	}
	if got := GetLinkOfGroup(g2); got == nil || got.ID != second.ID {
		t.Fatalf("GetLinkOfGroup(other) = %+v, want the untouched link", got)
	}
	if staffRow, err := GetStaffGroupFresh(staffID); err != nil || staffRow == nil {
		t.Fatalf("the Staff Group itself must stay: (%+v, %v)", staffRow, err)
	}
}

func TestStaffRepoDeleteLinkIfOwnerSecondCallIsNoop(t *testing.T) {
	staffID, groupID := uniqueStaffChatID(), uniqueStaffChatID()
	cleanupStaffRows(t, staffID, groupID)
	if _, err := CreateStaffGroup(staffID, 7, "Staff"); err != nil {
		t.Fatal(err)
	}
	link := &models.StaffGroupLink{GroupChatID: groupID, StaffChatID: staffID, OwnerUserID: 7}
	if err := CreateLink(link); err != nil {
		t.Fatal(err)
	}

	if deleted, err := DeleteLinkIfOwner(link.ID, 7); err != nil || !deleted {
		t.Fatalf("first DeleteLinkIfOwner = (%v, %v), want (true, nil)", deleted, err)
	}
	if deleted, err := DeleteLinkIfOwner(link.ID, 7); err != nil || deleted {
		t.Fatalf("second DeleteLinkIfOwner = (%v, %v), want (false, nil)", deleted, err)
	}
	if deleted, err := DeleteLinkIfOwner(0, 7); err != nil || deleted {
		t.Fatalf("DeleteLinkIfOwner(0) = (%v, %v), want (false, nil)", deleted, err)
	}
}

func TestStaffRepoDeleteLinkIfOwnerConcurrentOneWinner(t *testing.T) {
	staffID, groupID := uniqueStaffChatID(), uniqueStaffChatID()
	cleanupStaffRows(t, staffID, groupID)
	if _, err := CreateStaffGroup(staffID, 7, "Staff"); err != nil {
		t.Fatal(err)
	}
	link := &models.StaffGroupLink{GroupChatID: groupID, StaffChatID: staffID, OwnerUserID: 7}
	if err := CreateLink(link); err != nil {
		t.Fatal(err)
	}

	const callers = 4
	var wins, failures int
	var mu sync.Mutex
	var wg sync.WaitGroup
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			deleted, err := DeleteLinkIfOwner(link.ID, 7)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				failures++
			}
			if deleted {
				wins++
			}
		}()
	}
	wg.Wait()
	if wins != 1 {
		t.Fatalf("%d callers reported a delete (%d errors), want exactly 1", wins, failures)
	}
}

func TestStaffRepoUpdateStaffGroupOwnerChangesAndInvalidates(t *testing.T) {
	utilsCache.SetupTestMemoryMarshaler(t)
	cache.ResetLocalForTest()
	chatID := uniqueStaffChatID()
	cleanupStaffRows(t, chatID)
	if _, err := CreateStaffGroup(chatID, 7, "Staff"); err != nil {
		t.Fatal(err)
	}
	if got := GetStaffGroup(chatID); got == nil || got.OwnerUserID != 7 {
		t.Fatalf("GetStaffGroup before = %+v, want owner 7", got)
	}

	changed, err := UpdateStaffGroupOwner(chatID, 9)
	if err != nil || !changed {
		t.Fatalf("UpdateStaffGroupOwner(9) = (%v, %v), want (true, nil)", changed, err)
	}
	if got := GetStaffGroup(chatID); got == nil || got.OwnerUserID != 9 {
		t.Fatalf("GetStaffGroup after = %+v, want owner 9 (the write must invalidate staff_group:<chat>)", got)
	}
	fresh, err := GetStaffGroupFresh(chatID)
	if err != nil || fresh == nil || fresh.OwnerUserID != 9 || fresh.Title != "Staff" {
		t.Fatalf("GetStaffGroupFresh = (%+v, %v), want owner 9 and the title untouched", fresh, err)
	}
}

func TestStaffRepoUpdateStaffGroupOwnerSameValueIsNotAChange(t *testing.T) {
	chatID := uniqueStaffChatID()
	cleanupStaffRows(t, chatID)
	if _, err := CreateStaffGroup(chatID, 7, "Staff"); err != nil {
		t.Fatal(err)
	}

	if changed, err := UpdateStaffGroupOwner(chatID, 7); err != nil || changed {
		t.Fatalf("UpdateStaffGroupOwner(same) = (%v, %v), want (false, nil)", changed, err)
	}
	if changed, err := UpdateStaffGroupOwner(uniqueStaffChatID(), 9); err != nil || changed {
		t.Fatalf("UpdateStaffGroupOwner(unknown chat) = (%v, %v), want (false, nil)", changed, err)
	}
}

// seedHealthLink creates a Staff Group and one link in it, both removed when the
// test ends, and returns the link.
func seedHealthLink(t *testing.T) *models.StaffGroupLink {
	t.Helper()
	staffID, groupID := uniqueStaffChatID(), uniqueStaffChatID()
	cleanupStaffRows(t, staffID, groupID)
	if _, err := CreateStaffGroup(staffID, 7, "Staff"); err != nil {
		t.Fatal(err)
	}
	link := &models.StaffGroupLink{GroupChatID: groupID, StaffChatID: staffID, OwnerUserID: 7, GroupTitle: "Group"}
	if err := CreateLink(link); err != nil {
		t.Fatal(err)
	}
	return link
}

func TestStaffRepoSetLinkHealthChangesOnceAndInvalidates(t *testing.T) {
	utilsCache.SetupTestMemoryMarshaler(t)
	cache.ResetLocalForTest()
	link := seedHealthLink(t)
	if got := GetLinkOfGroup(link.GroupChatID); got == nil || got.Health != models.StaffHealthOK {
		t.Fatalf("GetLinkOfGroup before = %+v, want health ok", got)
	}

	changed, err := SetLinkHealth(link.ID, models.StaffHealthBotMissing)
	if err != nil || !changed {
		t.Fatalf("SetLinkHealth(bot_missing) = (%v, %v), want (true, nil)", changed, err)
	}
	if got := GetLinkOfGroup(link.GroupChatID); got == nil || got.Health != models.StaffHealthBotMissing {
		t.Fatalf("GetLinkOfGroup after = %+v, want health bot_missing (the write must invalidate the gate)", got)
	}
	changed, err = SetLinkHealth(link.ID, models.StaffHealthBotMissing)
	if err != nil || changed {
		t.Fatalf("SetLinkHealth(same value) = (%v, %v), want (false, nil)", changed, err)
	}
	changed, err = SetLinkHealth(link.ID, models.StaffHealthOK)
	if err != nil || !changed {
		t.Fatalf("SetLinkHealth(back to ok) = (%v, %v), want (true, nil)", changed, err)
	}
	fresh, err := GetLinkByIDFresh(link.ID)
	if err != nil || fresh == nil || fresh.Health != models.StaffHealthOK || fresh.OwnerUserID != 7 || fresh.GroupTitle != "Group" {
		t.Fatalf("GetLinkByIDFresh = (%+v, %v), want health ok and every other column untouched", fresh, err)
	}
}

func TestStaffRepoSetLinkHealthRejectsUnknownValue(t *testing.T) {
	link := seedHealthLink(t)

	changed, err := SetLinkHealth(link.ID, "broken")
	if err == nil || changed {
		t.Fatalf(`SetLinkHealth("broken") = (%v, %v), want (false, error from the CHECK constraint)`, changed, err)
	}
	fresh, err := GetLinkByIDFresh(link.ID)
	if err != nil || fresh == nil || fresh.Health != models.StaffHealthOK {
		t.Fatalf("link after the rejected write = (%+v, %v), want health ok", fresh, err)
	}
}

func TestStaffRepoSetLinkHealthMissingLinkIsNoop(t *testing.T) {
	if changed, err := SetLinkHealth(4_000_000_000, models.StaffHealthBotMissing); err != nil || changed {
		t.Fatalf("SetLinkHealth(unknown id) = (%v, %v), want (false, nil)", changed, err)
	}
}

func TestStaffRepoSetLinkHealthConcurrentOneWinner(t *testing.T) {
	link := seedHealthLink(t)

	const workers = 8
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		winners int
		errs    []error
	)
	start := make(chan struct{})
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			changed, err := SetLinkHealth(link.ID, models.StaffHealthBotNotAdmin)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errs = append(errs, err)
			}
			if changed {
				winners++
			}
		}()
	}
	close(start)
	wg.Wait()

	if len(errs) != 0 {
		t.Fatalf("concurrent SetLinkHealth errors: %v", errs)
	}
	if winners != 1 {
		t.Fatalf("%d callers got changed=true, want exactly 1", winners)
	}
}

func TestStaffRepoListStaffGroupsByOwner(t *testing.T) {
	owner := -uniqueStaffChatID() % 1_000_000_000_000
	otherOwner := owner + 1
	first, second, third := uniqueStaffChatID(), uniqueStaffChatID(), uniqueStaffChatID()
	cleanupStaffRows(t, first, second, third)
	for chatID, who := range map[int64]int64{first: owner, second: owner, third: otherOwner} {
		if _, err := CreateStaffGroup(chatID, who, "Staff"); err != nil {
			t.Fatal(err)
		}
	}

	groups, err := ListStaffGroupsByOwner(owner)
	if err != nil || len(groups) != 2 {
		t.Fatalf("ListStaffGroupsByOwner(owner) = (%+v, %v), want two groups", groups, err)
	}
	if groups[0].ID >= groups[1].ID {
		t.Fatalf("groups = %+v, want id order", groups)
	}
	got := map[int64]bool{groups[0].ChatID: true, groups[1].ChatID: true}
	if !got[first] || !got[second] || got[third] {
		t.Fatalf("groups = %+v, want exactly the owner's two Staff Groups", groups)
	}
	if none, err := ListStaffGroupsByOwner(owner + 99); err != nil || len(none) != 0 {
		t.Fatalf("ListStaffGroupsByOwner(stranger) = (%+v, %v), want empty", none, err)
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
