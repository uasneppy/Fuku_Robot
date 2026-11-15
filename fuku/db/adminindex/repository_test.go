//go:build testtools

package adminindex

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/backup"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// randomPositive returns a random positive 62-bit number.
func randomPositive(t *testing.T) int64 {
	t.Helper()
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		t.Fatalf("read random bytes: %v", err)
	}
	return int64(binary.BigEndian.Uint64(buf[:]) & 0x3fffffffffffffff)
}

// uniqueIndexChatID returns a random large negative chat ID, so tests sharing one database
// never collide and the ID is below zero like a group.
func uniqueIndexChatID(t *testing.T) int64 {
	t.Helper()
	return -1000000000000 - randomPositive(t)%1000000000000000
}

// uniqueIndexUserID returns a random user ID far above any ID a fixture uses.
func uniqueIndexUserID(t *testing.T) int64 {
	t.Helper()
	return 1000000000 + randomPositive(t)%1000000000000
}

// cleanupChat removes every row the test may leave for the chat in the three tables.
func cleanupChat(t *testing.T, chatIDs ...int64) {
	t.Helper()
	t.Cleanup(func() {
		for _, chatID := range chatIDs {
			if err := db.DB.Where("chat_id = ?", chatID).Delete(&models.ChatAdminIndex{}).Error; err != nil {
				t.Errorf("delete admin index rows of chat %d: %v", chatID, err)
			}
			if err := db.DB.Where("chat_id = ?", chatID).Delete(&models.ChatSettingsAccess{}).Error; err != nil {
				t.Errorf("delete settings access rows of chat %d: %v", chatID, err)
			}
			if err := db.DB.Where("chat_id = ?", chatID).Delete(&models.Chat{}).Error; err != nil {
				t.Errorf("delete chats row %d: %v", chatID, err)
			}
		}
	})
}

// rosterOf reads a chat's roster through ListChatFresh as a user ID to role map.
func rosterOf(t *testing.T, chatID int64) map[int64]string {
	t.Helper()
	rows, err := ListChatFresh(context.Background(), chatID)
	if err != nil {
		t.Fatalf("ListChatFresh(%d): %v", chatID, err)
	}
	roster := make(map[int64]string, len(rows))
	for _, row := range rows {
		roster[row.UserID] = row.Role
	}
	if len(roster) != len(rows) {
		t.Fatalf("chat %d lists a user twice: %+v", chatID, rows)
	}
	return roster
}

func requireRoster(t *testing.T, chatID int64, want map[int64]string) {
	t.Helper()
	got := rosterOf(t, chatID)
	if len(got) != len(want) {
		t.Fatalf("chat %d roster = %v, want %v", chatID, got, want)
	}
	for user, role := range want {
		if got[user] != role {
			t.Fatalf("chat %d roster = %v, want %v", chatID, got, want)
		}
	}
}

// TestAdminIndexReplaceChat covers the replace contract: the given roster is exactly what is
// left, an empty roster clears the chat, a duplicate user collapses with the creator winning
// in either order, an unknown role writes nothing, and other chats are never touched.
func TestAdminIndexReplaceChat(t *testing.T) {
	ctx := context.Background()
	chatID, otherChat := uniqueIndexChatID(t), uniqueIndexChatID(t)
	cleanupChat(t, chatID, otherChat)
	a, b, c := uniqueIndexUserID(t), uniqueIndexUserID(t), uniqueIndexUserID(t)

	if err := ReplaceChat(ctx, otherChat, []Entry{{UserID: a, Role: models.AdminIndexRoleCreator}}); err != nil {
		t.Fatalf("seed the other chat: %v", err)
	}

	if err := ReplaceChat(ctx, chatID, []Entry{
		{UserID: a, Role: models.AdminIndexRoleCreator},
		{UserID: b, Role: models.AdminIndexRoleAdministrator},
	}); err != nil {
		t.Fatalf("first replace: %v", err)
	}
	requireRoster(t, chatID, map[int64]string{a: models.AdminIndexRoleCreator, b: models.AdminIndexRoleAdministrator})

	// The second roster drops A, keeps B and adds C.
	if err := ReplaceChat(ctx, chatID, []Entry{
		{UserID: b, Role: models.AdminIndexRoleAdministrator},
		{UserID: c, Role: models.AdminIndexRoleAdministrator},
	}); err != nil {
		t.Fatalf("second replace: %v", err)
	}
	requireRoster(t, chatID, map[int64]string{b: models.AdminIndexRoleAdministrator, c: models.AdminIndexRoleAdministrator})
	requireRoster(t, otherChat, map[int64]string{a: models.AdminIndexRoleCreator})

	// A role change of a kept user is an update, not a second row.
	if err := ReplaceChat(ctx, chatID, []Entry{{UserID: b, Role: models.AdminIndexRoleCreator}}); err != nil {
		t.Fatalf("role change replace: %v", err)
	}
	requireRoster(t, chatID, map[int64]string{b: models.AdminIndexRoleCreator})

	// A user listed twice is stored once and the creator wins, whatever the input order.
	for name, entries := range map[string][]Entry{
		"administrator first": {{UserID: a, Role: models.AdminIndexRoleAdministrator}, {UserID: a, Role: models.AdminIndexRoleCreator}},
		"creator first":       {{UserID: a, Role: models.AdminIndexRoleCreator}, {UserID: a, Role: models.AdminIndexRoleAdministrator}},
	} {
		if err := ReplaceChat(ctx, chatID, entries); err != nil {
			t.Fatalf("%s: replace: %v", name, err)
		}
		requireRoster(t, chatID, map[int64]string{a: models.AdminIndexRoleCreator})
	}

	// An unknown role is refused before any write: the earlier rows stay.
	err := ReplaceChat(ctx, chatID, []Entry{
		{UserID: c, Role: models.AdminIndexRoleAdministrator},
		{UserID: b, Role: "member"},
	})
	if !errors.Is(err, ErrUnknownRole) {
		t.Fatalf("a role of member: got %v, want ErrUnknownRole", err)
	}
	requireRoster(t, chatID, map[int64]string{a: models.AdminIndexRoleCreator})

	// An empty roster removes every row of the chat and no other.
	if err := ReplaceChat(ctx, chatID, nil); err != nil {
		t.Fatalf("empty replace: %v", err)
	}
	requireRoster(t, chatID, map[int64]string{})
	requireRoster(t, otherChat, map[int64]string{a: models.AdminIndexRoleCreator})

	// ClearChat does the same as an empty roster.
	if err := ReplaceChat(ctx, chatID, []Entry{{UserID: b, Role: models.AdminIndexRoleAdministrator}}); err != nil {
		t.Fatalf("seed before clear: %v", err)
	}
	if err := ClearChat(ctx, chatID); err != nil {
		t.Fatalf("ClearChat: %v", err)
	}
	requireRoster(t, chatID, map[int64]string{})
	requireRoster(t, otherChat, map[int64]string{a: models.AdminIndexRoleCreator})
}

// seedChat inserts a chats row; inactive marks it the way the activity monitor does.
func seedChat(t *testing.T, chatID int64, title string, inactive bool) {
	t.Helper()
	row := models.Chat{ChatId: chatID, ChatName: title, Language: "en", IsInactive: inactive}
	if err := db.DB.Create(&row).Error; err != nil {
		t.Fatalf("seed chat %d: %v", chatID, err)
	}
}

// TestAdminIndexListForUser covers the private list's rule (D-15): a creator row is always
// listed, an administrator row is listed when the chat's switch is on or the chat has no
// creator row, an inactive chat is still listed, and the title is the chats row's name.
func TestAdminIndexListForUser(t *testing.T) {
	ctx := context.Background()
	user, owner := uniqueIndexUserID(t), uniqueIndexUserID(t)
	owned := uniqueIndexChatID(t)       // user is the creator
	gated := uniqueIndexChatID(t)       // user is an administrator, switch off, a creator exists
	opened := uniqueIndexChatID(t)      // user is an administrator, switch on
	ownerless := uniqueIndexChatID(t)   // user is an administrator, no creator row
	inactive := uniqueIndexChatID(t)    // user is the creator of a chat marked inactive
	untitled := uniqueIndexChatID(t)    // user is the creator, no chats row
	explicitOff := uniqueIndexChatID(t) // user is an administrator, switch row says off
	foreign := uniqueIndexChatID(t)     // another user's chat
	cleanupChat(t, owned, gated, opened, ownerless, inactive, untitled, explicitOff, foreign)

	seedChat(t, owned, "Owned group", false)
	seedChat(t, gated, "Gated group", false)
	seedChat(t, opened, "Opened group", false)
	seedChat(t, ownerless, "Ownerless group", false)
	seedChat(t, inactive, "Quiet group", true)
	seedChat(t, explicitOff, "Explicit off group", false)
	seedChat(t, foreign, "Foreign group", false)

	rosters := map[int64][]Entry{
		owned:       {{UserID: user, Role: models.AdminIndexRoleCreator}},
		gated:       {{UserID: owner, Role: models.AdminIndexRoleCreator}, {UserID: user, Role: models.AdminIndexRoleAdministrator}},
		opened:      {{UserID: owner, Role: models.AdminIndexRoleCreator}, {UserID: user, Role: models.AdminIndexRoleAdministrator}},
		ownerless:   {{UserID: user, Role: models.AdminIndexRoleAdministrator}},
		inactive:    {{UserID: user, Role: models.AdminIndexRoleCreator}},
		untitled:    {{UserID: user, Role: models.AdminIndexRoleCreator}},
		explicitOff: {{UserID: owner, Role: models.AdminIndexRoleCreator}, {UserID: user, Role: models.AdminIndexRoleAdministrator}},
		foreign:     {{UserID: owner, Role: models.AdminIndexRoleCreator}},
	}
	for chatID, roster := range rosters {
		if err := ReplaceChat(ctx, chatID, roster); err != nil {
			t.Fatalf("seed roster of chat %d: %v", chatID, err)
		}
	}
	for chatID, allowed := range map[int64]bool{opened: true, explicitOff: false} {
		row := models.ChatSettingsAccess{ChatID: chatID, AdminsAllowed: allowed, UpdatedBy: owner}
		if err := db.DB.Create(&row).Error; err != nil {
			t.Fatalf("seed the switch of chat %d: %v", chatID, err)
		}
	}

	groups, err := ListForUserFresh(ctx, user)
	if err != nil {
		t.Fatalf("ListForUserFresh: %v", err)
	}
	got := make(map[int64]string, len(groups))
	for _, group := range groups {
		got[group.ChatID] = group.Title
	}
	want := map[int64]string{
		owned:     "Owned group",
		opened:    "Opened group",
		ownerless: "Ownerless group",
		inactive:  "Quiet group",
		untitled:  "",
	}
	if len(got) != len(want) {
		t.Fatalf("listed groups = %v, want %v", got, want)
	}
	for chatID, title := range want {
		listed, ok := got[chatID]
		if !ok || listed != title {
			t.Fatalf("listed groups = %v, want %v", got, want)
		}
	}
	if len(groups) != len(want) {
		t.Fatalf("a group is listed twice: %+v", groups)
	}
	if !sort.SliceIsSorted(groups, func(i, j int) bool { return groups[i].ChatID < groups[j].ChatID }) {
		t.Fatalf("groups are not ordered by chat ID: %+v", groups)
	}

	// The owner is the creator of three chats and sees only those; the gated admin rule
	// never hides a creator row.
	ownerGroups, err := ListForUserFresh(ctx, owner)
	if err != nil {
		t.Fatalf("ListForUserFresh(owner): %v", err)
	}
	ownerIDs := make(map[int64]bool)
	for _, group := range ownerGroups {
		ownerIDs[group.ChatID] = true
	}
	for _, chatID := range []int64{gated, opened, explicitOff, foreign} {
		if !ownerIDs[chatID] {
			t.Fatalf("the creator does not see chat %d: %+v", chatID, ownerGroups)
		}
	}
	if ownerIDs[owned] {
		t.Fatalf("the creator sees a chat they have no row in: %+v", ownerGroups)
	}

	// A user with no row sees nothing.
	none, err := ListForUserFresh(ctx, uniqueIndexUserID(t))
	if err != nil || len(none) != 0 {
		t.Fatalf("a user with no rows: got (%v, %v), want an empty list", none, err)
	}
}

// TestAdminIndexCandidates proves the sweep's candidate list: every chats.chat_id below
// zero in ascending order, inactive chats included, and no positive ID.
func TestAdminIndexCandidates(t *testing.T) {
	ctx := context.Background()
	low, high, inactive, positive := uniqueIndexChatID(t), uniqueIndexChatID(t), uniqueIndexChatID(t), randomPositive(t)%1000000000+1
	cleanupChat(t, low, high, inactive, positive)
	seedChat(t, low, "low", false)
	seedChat(t, high, "high", false)
	seedChat(t, inactive, "inactive", true)
	seedChat(t, positive, "private chat", false)

	ids, err := ListCandidateChatIDsFresh(ctx)
	if err != nil {
		t.Fatalf("ListCandidateChatIDsFresh: %v", err)
	}
	if !sort.SliceIsSorted(ids, func(i, j int) bool { return ids[i] < ids[j] }) {
		t.Fatalf("candidates are not ascending: %v", ids)
	}
	present := make(map[int64]bool, len(ids))
	for _, id := range ids {
		if id >= 0 {
			t.Fatalf("candidate %d is not below zero", id)
		}
		present[id] = true
	}
	for _, id := range []int64{low, high, inactive} {
		if !present[id] {
			t.Fatalf("candidate %d is missing from %v", id, ids)
		}
	}
	if present[positive] {
		t.Fatalf("the positive chat ID %d is a candidate", positive)
	}
}

// TestAdminIndexReplaceConcurrent proves that concurrent replaces of one chat never fail and
// leave exactly one of the rosters, never a mix of two. The rosters are pairwise disjoint, so
// a union would be visible.
//
// On PostgreSQL every replace first takes a per-chat transaction advisory lock, which is the
// real proof (plan 10-04 lists this test in make test-postgres-integrity). On SQLite the
// shared test database is opened with a 10 s busy timeout and WAL mode
// (internal/testdb.Run), so a writer that finds the write lock held waits instead of failing,
// and SQLite keeps that lock until commit, which serializes the transactions. The first
// statement of a replace is a write (the upsert, or the delete for an empty roster) with no
// read before it, so no transaction has to upgrade a read snapshot to a write, the one case
// where SQLite answers SQLITE_BUSY without waiting. Sixteen small transactions finish far
// inside the timeout. No pool setting is changed.
func TestAdminIndexReplaceConcurrent(t *testing.T) {
	ctx := context.Background()
	chatID := uniqueIndexChatID(t)
	cleanupChat(t, chatID)

	const workers, rounds, perRoster = 8, 20, 3
	rosters := make([][]Entry, workers)
	base := uniqueIndexUserID(t)
	for w := range rosters {
		for i := 0; i < perRoster; i++ {
			rosters[w] = append(rosters[w], Entry{UserID: base + int64(w*perRoster+i), Role: models.AdminIndexRoleAdministrator})
		}
	}

	var wg sync.WaitGroup
	errs := make(chan error, workers*rounds)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for r := 0; r < rounds; r++ {
				if err := ReplaceChat(ctx, chatID, rosters[w]); err != nil {
					errs <- fmt.Errorf("worker %d round %d: %w", w, r, err)
					return
				}
			}
		}(w)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent replace failed: %v", err)
	}
	if t.Failed() {
		return
	}

	got := rosterOf(t, chatID)
	matches := 0
	for _, roster := range rosters {
		if len(roster) != len(got) {
			continue
		}
		same := true
		for _, entry := range roster {
			if got[entry.UserID] != entry.Role {
				same = false
				break
			}
		}
		if same {
			matches++
		}
	}
	if matches != 1 {
		t.Fatalf("final roster %v is not exactly one of the %d rosters", got, workers)
	}

	// The schema backs the repository: the CHECK refuses a role of member and the unique key
	// refuses a second row for one (chat, user).
	if err := db.DB.Exec(
		"INSERT INTO chat_admin_index (chat_id, user_id, role) VALUES (?, ?, 'member')",
		chatID, base+1000,
	).Error; err == nil {
		t.Fatalf("a raw insert of role member was accepted, want a CHECK violation")
	}
	const insertAdmin = "INSERT INTO chat_admin_index (chat_id, user_id, role) VALUES (?, ?, 'administrator')"
	if err := db.DB.Exec(insertAdmin, chatID, base+2000).Error; err != nil {
		t.Fatalf("a raw insert of a new administrator failed: %v", err)
	}
	if err := db.DB.Exec(insertAdmin, chatID, base+2000).Error; err == nil {
		t.Fatalf("a duplicate (chat_id, user_id) insert was accepted, want a unique violation")
	}
}

// TestAccessTablesStayOutOfBackup proves the owner switch and the admin roster are in no
// backup module, so restoring a backup can never widen who may change a group's settings.
func TestAccessTablesStayOutOfBackup(t *testing.T) {
	for _, name := range []string{
		"chat_admin_index", "chat_settings_access", "admin_index", "settings_access", "menu_access", "adminindex", "menuaccess",
	} {
		if backup.IsValidModule(name) {
			t.Errorf("backup module %q is valid; the settings access tables must stay out of backup", name)
		}
	}
	for _, module := range backup.AllExportableModules() {
		lower := strings.ToLower(module)
		for _, forbidden := range []string{"admin_index", "settings_access", "menu_access"} {
			if strings.Contains(lower, forbidden) {
				t.Errorf("exportable backup module %q mentions %q", module, forbidden)
			}
		}
	}
}
