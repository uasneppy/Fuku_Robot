//go:build testtools

package staff

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	utilsCache "github.com/uasneppy/fuku_robot/fuku/utils/cache"
)

// fwdSeedStaffGroup stores a Staff Group row and registers cleanup of every row the test may add.
func fwdSeedStaffGroup(t *testing.T) int64 {
	t.Helper()
	staffID := uniqueStaffChatID()
	cleanupStaffRows(t, staffID)
	if err := db.DB.Create(&models.StaffGroup{ChatID: staffID, OwnerUserID: 1, Title: "Staff"}).Error; err != nil {
		t.Fatalf("seed staff group: %v", err)
	}
	return staffID
}

func fwdAdd(t *testing.T, staffID, source int64, title string) bool {
	t.Helper()
	added, err := AddForwardBlock(context.Background(), staffID, source, title, 7)
	if err != nil {
		t.Fatalf("AddForwardBlock(%d): %v", source, err)
	}
	return added
}

func TestStaffForwardBlockRepository(t *testing.T) {
	utilsCache.SetupTestMemoryMarshaler(t)
	cache.ResetLocalForTest()
	ctx := context.Background()
	staffID := fwdSeedStaffGroup(t)
	other := fwdSeedStaffGroup(t)
	src, src2 := uniqueStaffChatID(), uniqueStaffChatID()

	if !fwdAdd(t, staffID, src, "Channel One") {
		t.Fatal("first add reported added=false")
	}
	if fwdAdd(t, staffID, src, "Renamed") {
		t.Fatal("a repeat add reported added=true")
	}
	rows, err := ListForwardBlocksFresh(ctx, staffID)
	if err != nil || len(rows) != 1 || rows[0].SourceTitle != "Channel One" || rows[0].CreatedBy != 7 {
		t.Fatalf("ListForwardBlocksFresh = (%+v, %v), want the first row untouched by the repeat", rows, err)
	}
	fwdAdd(t, staffID, src2, "")
	fwdAdd(t, other, src, "Other")

	if n, err := CountForwardBlocksFresh(ctx, staffID); err != nil || n != 2 {
		t.Fatalf("CountForwardBlocksFresh = (%d, %v), want 2", n, err)
	}

	missing := uniqueStaffChatID()
	if _, err := AddForwardBlock(ctx, missing, src, "x", 7); !errors.Is(err, ErrStaffGroupMissing) {
		t.Fatalf("add to a missing Staff Group = %v, want ErrStaffGroupMissing", err)
	}

	// Remove by ID: another Staff Group's ID and an unknown ID remove nothing.
	list, _ := ListForwardBlocksFresh(ctx, staffID)
	otherRows, _ := ListForwardBlocksFresh(ctx, other)
	if got, err := RemoveForwardBlockByID(ctx, staffID, otherRows[0].ID); err != nil || got != nil {
		t.Fatalf("RemoveForwardBlockByID(foreign) = (%+v, %v), want (nil, nil)", got, err)
	}
	if got, err := GetForwardBlockFresh(ctx, staffID, otherRows[0].ID); err != nil || got != nil {
		t.Fatalf("GetForwardBlockFresh(foreign) = (%+v, %v), want (nil, nil)", got, err)
	}
	if got, err := RemoveForwardBlockByID(ctx, staffID, 999999999); err != nil || got != nil {
		t.Fatalf("RemoveForwardBlockByID(unknown) = (%+v, %v), want (nil, nil)", got, err)
	}
	if n, _ := CountForwardBlocksFresh(ctx, other); n != 1 {
		t.Fatal("a foreign ID removed another Staff Group's row")
	}
	got, err := RemoveForwardBlockByID(ctx, staffID, list[0].ID)
	if err != nil || got == nil || got.SourceChatID != src {
		t.Fatalf("RemoveForwardBlockByID = (%+v, %v), want the removed row", got, err)
	}
	if got, _ := RemoveForwardBlockByID(ctx, staffID, list[0].ID); got != nil {
		t.Fatal("a second removal of the same ID removed a row")
	}

	// Remove by source.
	if ok, err := RemoveForwardBlockBySource(ctx, staffID, src); err != nil || ok {
		t.Fatalf("RemoveForwardBlockBySource(already gone) = (%v, %v)", ok, err)
	}
	if ok, err := RemoveForwardBlockBySource(ctx, staffID, src2); err != nil || !ok {
		t.Fatalf("RemoveForwardBlockBySource = (%v, %v), want (true, nil)", ok, err)
	}
	if n, _ := CountForwardBlocksFresh(ctx, staffID); n != 0 {
		t.Fatalf("count after removals = %d, want 0", n)
	}
}

func TestStaffForwardBlockCap(t *testing.T) {
	ctx := context.Background()
	staffID := fwdSeedStaffGroup(t)
	rows := make([]models.StaffForwardBlock, ForwardBlockMax)
	for i := range rows {
		rows[i] = models.StaffForwardBlock{StaffChatID: staffID, SourceChatID: -int64(i) - 1}
	}
	if err := db.DB.CreateInBatches(&rows, 50).Error; err != nil {
		t.Fatalf("seed %d sources: %v", ForwardBlockMax, err)
	}
	if _, err := AddForwardBlock(ctx, staffID, -5000000, "x", 7); !errors.Is(err, ErrForwardBlockFull) {
		t.Fatalf("add past the cap = %v, want ErrForwardBlockFull", err)
	}
	if n, _ := CountForwardBlocksFresh(ctx, staffID); n != ForwardBlockMax {
		t.Fatalf("count = %d, want %d", n, ForwardBlockMax)
	}
}

func TestStaffForwardBlockAction(t *testing.T) {
	utilsCache.SetupTestMemoryMarshaler(t)
	cache.ResetLocalForTest()
	ctx := context.Background()
	staffID := fwdSeedStaffGroup(t)

	rules, err := GetForwardBlockRulesFresh(ctx, staffID)
	if err != nil || rules.Action != ForwardBlockActionNone || len(rules.Sources) != 0 {
		t.Fatalf("default rules = (%+v, %v), want action none and no sources", rules, err)
	}
	var settingsRows int64
	db.DB.Model(&models.StaffForwardBlockSettings{}).Where("staff_chat_id = ?", staffID).Count(&settingsRows)
	if settingsRows != 0 {
		t.Fatal("a read created a settings row")
	}

	for _, action := range []string{"warn", "mute", "kick", "ban", "none"} {
		if err := SetForwardBlockAction(ctx, staffID, 9, action); err != nil {
			t.Fatalf("SetForwardBlockAction(%q): %v", action, err)
		}
		got, err := GetForwardBlockRulesFresh(ctx, staffID)
		if err != nil || got.Action != action {
			t.Fatalf("after setting %q the action reads (%+v, %v)", action, got, err)
		}
	}
	if err := SetForwardBlockAction(ctx, staffID, 9, "ban"); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"", "tban", "tmute", "delete", "WARN", "ban "} {
		if err := SetForwardBlockAction(ctx, staffID, 9, bad); !errors.Is(err, ErrForwardBlockAction) {
			t.Errorf("SetForwardBlockAction(%q) = %v, want ErrForwardBlockAction", bad, err)
		}
	}
	if got, _ := GetForwardBlockRulesFresh(ctx, staffID); got.Action != "ban" {
		t.Fatalf("a refused action changed the stored one to %q", got.Action)
	}
	if err := SetForwardBlockAction(ctx, uniqueStaffChatID(), 9, "warn"); !errors.Is(err, ErrStaffGroupMissing) {
		t.Fatalf("action of a missing Staff Group = %v, want ErrStaffGroupMissing", err)
	}
	if !slices.Equal(ForwardBlockActions, []string{"none", "warn", "mute", "kick", "ban"}) {
		t.Fatalf("ForwardBlockActions = %v", ForwardBlockActions)
	}
}

func TestStaffForwardBlockCache(t *testing.T) {
	utilsCache.SetupTestMemoryMarshaler(t)
	cache.ResetLocalForTest()
	ctx := context.Background()
	staffID := fwdSeedStaffGroup(t)
	src, src2 := uniqueStaffChatID(), uniqueStaffChatID()

	read := func() ForwardBlockRules {
		t.Helper()
		r, err := GetForwardBlockRules(ctx, staffID)
		if err != nil {
			t.Fatalf("GetForwardBlockRules: %v", err)
		}
		return r
	}
	if r := read(); len(r.Sources) != 0 || r.Action != "none" {
		t.Fatalf("empty rules = %+v", r)
	}
	// The empty read is cached: a row written behind the repository's back stays hidden.
	if err := db.DB.Create(&models.StaffForwardBlock{StaffChatID: staffID, SourceChatID: src2}).Error; err != nil {
		t.Fatal(err)
	}
	if r := read(); len(r.Sources) != 0 {
		t.Fatal("the rules are not cached: a direct write was visible")
	}
	cache.DeleteCache(cache.CacheKey(cachePrefixStaffForwardBlock, staffID))
	if r := read(); !r.IsBlocked(src2) {
		t.Fatal("the rules did not reload after the key was dropped")
	}

	// Every write drops the key, so the next read sees it, with the local layer on.
	fwdAdd(t, staffID, src, "t")
	if r := read(); !r.IsBlocked(src) || !r.IsBlocked(src2) {
		t.Fatalf("after add: %+v", r)
	}
	if err := SetForwardBlockAction(ctx, staffID, 1, "kick"); err != nil {
		t.Fatal(err)
	}
	if r := read(); r.Action != "kick" {
		t.Fatalf("after set action: %+v", r)
	}
	if ok, err := RemoveForwardBlockBySource(ctx, staffID, src); err != nil || !ok {
		t.Fatal(err)
	}
	if r := read(); r.IsBlocked(src) || !r.IsBlocked(src2) {
		t.Fatalf("after remove by source: %+v", r)
	}
	rows, _ := ListForwardBlocksFresh(ctx, staffID)
	if _, err := RemoveForwardBlockByID(ctx, staffID, rows[0].ID); err != nil {
		t.Fatal(err)
	}
	if r := read(); len(r.Sources) != 0 || r.Action != "kick" {
		t.Fatalf("after remove by ID: %+v", r)
	}
}

func TestStaffForwardBlockRekeyAndUnset(t *testing.T) {
	utilsCache.SetupTestMemoryMarshaler(t)
	cache.ResetLocalForTest()
	ctx := context.Background()
	oldID, newID, bystander := fwdSeedStaffGroup(t), uniqueStaffChatID(), fwdSeedStaffGroup(t)
	cleanupStaffRows(t, newID)
	src := uniqueStaffChatID()

	fwdAdd(t, oldID, src, "a")
	fwdAdd(t, bystander, src, "b")
	if err := SetForwardBlockAction(ctx, oldID, 1, "mute"); err != nil {
		t.Fatal(err)
	}
	if err := SetForwardBlockAction(ctx, bystander, 1, "ban"); err != nil {
		t.Fatal(err)
	}
	// Warm both keys so the re-key has to drop them.
	_, _ = GetForwardBlockRules(ctx, oldID)
	_, _ = GetForwardBlockRules(ctx, newID)

	changed, err := RekeyChat(oldID, newID)
	if err != nil || !changed {
		t.Fatalf("RekeyChat = (%v, %v)", changed, err)
	}
	if r, _ := GetForwardBlockRules(ctx, oldID); len(r.Sources) != 0 || r.Action != "none" {
		t.Fatalf("the old key still holds rules: %+v", r)
	}
	r, err := GetForwardBlockRules(ctx, newID)
	if err != nil || !r.IsBlocked(src) || r.Action != "mute" {
		t.Fatalf("the new key reads (%+v, %v), want the moved list and action", r, err)
	}
	if b, _ := GetForwardBlockRules(ctx, bystander); !b.IsBlocked(src) || b.Action != "ban" {
		t.Fatalf("another Staff Group's list changed: %+v", b)
	}

	// A re-key of a Staff Group with only a blocklist (no group row) is not this path; unset
	// deletes both tables in the same transaction.
	_, deleted, err := DeleteStaffGroupWithLinks(newID)
	if err != nil || !deleted {
		t.Fatalf("DeleteStaffGroupWithLinks = (%v, %v)", deleted, err)
	}
	var blocks, settings int64
	db.DB.Model(&models.StaffForwardBlock{}).Where("staff_chat_id = ?", newID).Count(&blocks)
	db.DB.Model(&models.StaffForwardBlockSettings{}).Where("staff_chat_id = ?", newID).Count(&settings)
	if blocks != 0 || settings != 0 {
		t.Fatalf("after unset %d sources and %d settings rows remain", blocks, settings)
	}
	if r, _ := GetForwardBlockRules(ctx, newID); len(r.Sources) != 0 || r.Action != "none" {
		t.Fatalf("the key still serves rules after unset: %+v", r)
	}
	if _, deleted, _ := DeleteStaffGroupWithLinks(newID); deleted {
		t.Fatal("a second unset reported a deletion")
	}
	if b, _ := GetForwardBlockRules(ctx, bystander); !b.IsBlocked(src) {
		t.Fatal("unsetting one Staff Group deleted another's list")
	}
}

func TestStaffForwardBlockConstraints(t *testing.T) {
	staffID := uniqueStaffChatID()
	cleanupStaffRows(t, staffID)
	src := uniqueStaffChatID()

	if err := db.DB.Create(&models.StaffForwardBlock{StaffChatID: staffID, SourceChatID: src}).Error; err != nil {
		t.Fatalf("first insert: %v", err)
	}
	err := db.DB.Create(&models.StaffForwardBlock{StaffChatID: staffID, SourceChatID: src}).Error
	if err == nil {
		t.Fatal("a duplicate (staff chat, source) was accepted")
	}
	if strings.Contains(err.Error(), "SQLSTATE") && !strings.Contains(err.Error(), "uk_staff_forward_blocks_source") {
		t.Errorf("duplicate refused by %v, want uk_staff_forward_blocks_source", err)
	}
	for _, bad := range []int64{0, 5} {
		err := db.DB.Create(&models.StaffForwardBlock{StaffChatID: staffID, SourceChatID: bad}).Error
		if err == nil {
			t.Fatalf("source %d was accepted (chk_staff_forward_blocks_source)", bad)
		}
		if strings.Contains(err.Error(), "SQLSTATE") && !strings.Contains(err.Error(), "chk_staff_forward_blocks_source") {
			t.Errorf("source %d refused by %v", bad, err)
		}
	}
	for _, bad := range []string{"", "tban", "x"} {
		err := db.DB.Create(&models.StaffForwardBlockSettings{StaffChatID: staffID, Action: bad}).Error
		if err == nil {
			t.Fatalf("action %q was accepted (chk_staff_forward_block_action)", bad)
		}
		if strings.Contains(err.Error(), "SQLSTATE") && !strings.Contains(err.Error(), "chk_staff_forward_block_action") {
			t.Errorf("action %q refused by %v", bad, err)
		}
	}
	for _, good := range ForwardBlockActions {
		id := uniqueStaffChatID()
		cleanupStaffRows(t, id)
		if err := db.DB.Create(&models.StaffForwardBlockSettings{StaffChatID: id, Action: good}).Error; err != nil {
			t.Errorf("action %q refused: %v", good, err)
		}
	}
}
