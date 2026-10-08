//go:build testtools

package staff

import (
	"strings"
	"sync"
	"testing"

	"gorm.io/gorm"

	"github.com/divkix/Alita_Robot/alita/db"
)

const (
	exclusivityInsertStaff = "INSERT INTO staff_groups (chat_id, owner_user_id, title) VALUES (?, 1, 'staff')"
	exclusivityInsertLink  = "INSERT INTO staff_group_links (group_chat_id, staff_chat_id, owner_user_id, group_title) VALUES (?, ?, 1, 'group')"
	exclusivityConflict    = "staff role conflict"
)

// exclusivityRequireConflict fails the test unless err is the trigger's role-conflict rejection.
func exclusivityRequireConflict(t *testing.T, err error, what string) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s: insert succeeded, want the exclusivity trigger to reject it", what)
	}
	if !strings.Contains(err.Error(), exclusivityConflict) {
		t.Fatalf("%s: error = %v, want it to mention %q", what, err, exclusivityConflict)
	}
}

// exclusivityCount returns how many rows of table match the chat_id column.
func exclusivityCount(t *testing.T, table, column string, chatID int64) int64 {
	t.Helper()
	var n int64
	if err := db.DB.Table(table).Where(column+" = ?", chatID).Count(&n).Error; err != nil {
		t.Fatalf("count %s.%s: %v", table, column, err)
	}
	return n
}

// TestStaffExclusivityTrigger checks D-10 in the database: a chat is never both a
// Staff Group and a linked group, even when two connections race to make it so.
// The trigger exists only on PostgreSQL, so SQLite runs skip it.
func TestStaffExclusivityTrigger(t *testing.T) {
	if db.DB.Name() != "postgres" {
		t.Skip("PostgreSQL trigger: run make test-postgres-integrity")
	}

	t.Run("StaffGroupForLinkedChatRejected", func(t *testing.T) {
		group, staff := uniqueStaffChatID(), uniqueStaffChatID()
		cleanupStaffRows(t, group, staff)
		if err := db.DB.Exec(exclusivityInsertLink, group, staff).Error; err != nil {
			t.Fatalf("seed link: %v", err)
		}
		exclusivityRequireConflict(t, db.DB.Exec(exclusivityInsertStaff, group).Error, "Staff Group for a linked chat")
	})

	t.Run("LinkOfStaffGroupRejected", func(t *testing.T) {
		staffGroup, otherStaff := uniqueStaffChatID(), uniqueStaffChatID()
		cleanupStaffRows(t, staffGroup, otherStaff)
		if err := db.DB.Exec(exclusivityInsertStaff, staffGroup).Error; err != nil {
			t.Fatalf("seed staff group: %v", err)
		}
		exclusivityRequireConflict(t, db.DB.Exec(exclusivityInsertLink, staffGroup, otherStaff).Error, "link of a Staff Group")
	})

	t.Run("LinkToLinkedStaffChatRejected", func(t *testing.T) {
		linked, staff, other := uniqueStaffChatID(), uniqueStaffChatID(), uniqueStaffChatID()
		cleanupStaffRows(t, linked, staff, other)
		if err := db.DB.Exec(exclusivityInsertLink, linked, staff).Error; err != nil {
			t.Fatalf("seed link: %v", err)
		}
		exclusivityRequireConflict(t, db.DB.Exec(exclusivityInsertLink, other, linked).Error, "link whose Staff Group is itself linked")
	})

	t.Run("LegitimateRowsAccepted", func(t *testing.T) {
		staff, rekeyed, group := uniqueStaffChatID(), uniqueStaffChatID(), uniqueStaffChatID()
		cleanupStaffRows(t, staff, rekeyed, group)
		if err := db.DB.Exec(exclusivityInsertStaff, staff).Error; err != nil {
			t.Fatalf("fresh Staff Group rejected: %v", err)
		}
		if err := db.DB.Exec(exclusivityInsertLink, group, staff).Error; err != nil {
			t.Fatalf("fresh link rejected: %v", err)
		}
		// A supergroup migration re-keys the Staff Group and its links together.
		err := db.DB.Transaction(func(tx *gorm.DB) error {
			if err := tx.Exec("UPDATE staff_groups SET chat_id = ? WHERE chat_id = ?", rekeyed, staff).Error; err != nil {
				return err
			}
			return tx.Exec("UPDATE staff_group_links SET staff_chat_id = ? WHERE staff_chat_id = ?", rekeyed, staff).Error
		})
		if err != nil {
			t.Fatalf("re-key of a Staff Group rejected: %v", err)
		}
		if got := exclusivityCount(t, "staff_groups", "chat_id", rekeyed); got != 1 {
			t.Errorf("staff_groups rows for re-keyed chat = %d, want 1", got)
		}
		if got := exclusivityCount(t, "staff_group_links", "staff_chat_id", rekeyed); got != 1 {
			t.Errorf("links pointing at re-keyed Staff Group = %d, want 1", got)
		}
	})

	t.Run("ConcurrentSetStaffAndLinkNeverBothCommit", func(t *testing.T) {
		const rounds = 20
		for round := 0; round < rounds; round++ {
			contested, staff := uniqueStaffChatID(), uniqueStaffChatID()
			cleanupStaffRows(t, contested, staff)

			// Each side inserts, then holds its transaction open briefly so the other
			// side's check runs while this row is still uncommitted. Without the
			// per-chat advisory lock both checks pass and both rows commit.
			attempts := []func(tx *gorm.DB) error{
				func(tx *gorm.DB) error { return tx.Exec(exclusivityInsertStaff, contested).Error },
				func(tx *gorm.DB) error { return tx.Exec(exclusivityInsertLink, contested, staff).Error },
			}
			errs := make([]error, len(attempts))
			start := make(chan struct{})
			var wg sync.WaitGroup
			for i, attempt := range attempts {
				wg.Add(1)
				go func() {
					defer wg.Done()
					<-start
					errs[i] = db.DB.Transaction(func(tx *gorm.DB) error {
						if err := attempt(tx); err != nil {
							return err
						}
						return tx.Exec("SELECT pg_sleep(0.05)").Error
					})
				}()
			}
			close(start)
			wg.Wait()

			asStaff := exclusivityCount(t, "staff_groups", "chat_id", contested)
			asLinked := exclusivityCount(t, "staff_group_links", "group_chat_id", contested)
			if asStaff > 0 && asLinked > 0 {
				t.Fatalf("round %d: chat is both a Staff Group and a linked group (errs = %v)", round, errs)
			}
			committed := 0
			for _, err := range errs {
				if err == nil {
					committed++
				}
			}
			if committed != 1 {
				t.Fatalf("round %d: %d transactions committed, want exactly 1 (errs = %v)", round, committed, errs)
			}
		}
	})
}
