//go:build testtools

package approvals

import (
	"testing"

	"github.com/divkix/Alita_Robot/alita/db"
	"github.com/divkix/Alita_Robot/alita/db/models"
)

func skipIfNoDb(t *testing.T) {
	if db.DB == nil {
		t.Fatal("test database was not initialized")
	}
}

func TestIsUserApproved(t *testing.T) {
	skipIfNoDb(t)

	chatID := int64(-999999999999999)

	t.Cleanup(func() {
		_ = RemoveAllApprovedUsers(chatID)
	})

	if IsUserApproved(chatID, 12345) {
		t.Fatalf("IsUserApproved() = true, expected false for non-existent user")
	}

	if err := AddApprovedUser(chatID, 12345, 99999, "trusted member"); err != nil {
		t.Fatalf("AddApprovedUser() error = %v", err)
	}
	if !IsUserApproved(chatID, 12345) {
		t.Fatalf("IsUserApproved() = false, expected true after approval")
	}

	if IsUserApproved(chatID, 99999) {
		t.Fatalf("IsUserApproved() = true for unapproved user")
	}

	if IsUserApproved(-888888888888, 12345) {
		t.Fatalf("IsUserApproved() = true in wrong chat")
	}
}

func TestAddApprovedUser(t *testing.T) {
	skipIfNoDb(t)

	chatID := int64(-999999999999999)

	t.Cleanup(func() {
		_ = RemoveAllApprovedUsers(chatID)
	})

	if err := AddApprovedUser(chatID, 11111, 99999, "test reason"); err != nil {
		t.Fatalf("AddApprovedUser() error = %v", err)
	}

	users := GetApprovedUsers(chatID)
	if len(users) != 1 {
		t.Fatalf("GetApprovedUsers() returned %d users, expected 1", len(users))
	}
	if users[0].UserID != 11111 {
		t.Fatalf("expected UserID=11111, got %d", users[0].UserID)
	}
	if users[0].Reason != "test reason" {
		t.Fatalf("expected Reason='test reason', got %q", users[0].Reason)
	}
	if users[0].ApprovedBy != 99999 {
		t.Fatalf("expected ApprovedBy=99999, got %d", users[0].ApprovedBy)
	}
}

func TestRemoveApprovedUser(t *testing.T) {
	skipIfNoDb(t)

	chatID := int64(-999999999999999)

	t.Cleanup(func() {
		_ = RemoveAllApprovedUsers(chatID)
	})

	if err := AddApprovedUser(chatID, 100, 1, ""); err != nil {
		t.Fatalf("AddApprovedUser() error = %v", err)
	}
	if err := AddApprovedUser(chatID, 200, 1, ""); err != nil {
		t.Fatalf("AddApprovedUser() error = %v", err)
	}

	if err := RemoveApprovedUser(chatID, 100); err != nil {
		t.Fatalf("RemoveApprovedUser() error = %v", err)
	}

	if IsUserApproved(chatID, 100) {
		t.Fatalf("IsUserApproved() = true after removal")
	}
	if !IsUserApproved(chatID, 200) {
		t.Fatalf("IsUserApproved() = false for remaining user")
	}
}

func TestGetApprovedUsers(t *testing.T) {
	skipIfNoDb(t)

	chatID := int64(-999999999999999)

	t.Cleanup(func() {
		_ = RemoveAllApprovedUsers(chatID)
	})

	users := GetApprovedUsers(chatID)
	if users == nil {
		t.Fatalf("GetApprovedUsers() returned nil, expected empty slice")
	}
	if len(users) != 0 {
		t.Fatalf("expected 0 approved users for new chat, got %d", len(users))
	}

	if err := AddApprovedUser(chatID, 10, 1, "reason1"); err != nil {
		t.Fatalf("AddApprovedUser() error = %v", err)
	}
	if err := AddApprovedUser(chatID, 20, 1, "reason2"); err != nil {
		t.Fatalf("AddApprovedUser() error = %v", err)
	}

	users = GetApprovedUsers(chatID)
	if len(users) != 2 {
		t.Fatalf("expected 2 approved users, got %d", len(users))
	}
}

func TestRemoveAllApprovedUsers(t *testing.T) {
	skipIfNoDb(t)

	chatID := int64(-999999999999999)

	t.Cleanup(func() {
		_ = RemoveAllApprovedUsers(chatID)
	})

	for i := range 3 {
		if err := AddApprovedUser(chatID, int64(100+i), 1, ""); err != nil {
			t.Fatalf("AddApprovedUser() error = %v", err)
		}
	}

	if err := RemoveAllApprovedUsers(chatID); err != nil {
		t.Fatalf("RemoveAllApprovedUsers() error = %v", err)
	}

	users := GetApprovedUsers(chatID)
	if len(users) != 0 {
		t.Fatalf("expected 0 users after RemoveAllApprovedUsers, got %d", len(users))
	}
}

func TestCacheInvalidationOnWrite(t *testing.T) {
	skipIfNoDb(t)

	chatID := int64(-999999999999998)

	t.Cleanup(func() {
		_ = RemoveAllApprovedUsers(chatID)
	})

	if err := AddApprovedUser(chatID, 5555, 1, ""); err != nil {
		t.Fatalf("AddApprovedUser() error = %v", err)
	}

	users1 := GetApprovedUsers(chatID)
	if len(users1) != 1 {
		t.Fatalf("expected 1 user, got %d", len(users1))
	}

	if err := AddApprovedUser(chatID, 6666, 1, ""); err != nil {
		t.Fatalf("AddApprovedUser() error = %v", err)
	}

	users2 := GetApprovedUsers(chatID)
	if len(users2) != 2 {
		t.Fatalf("cache not invalidated: expected 2 users after add, got %d", len(users2))
	}

	if err := RemoveApprovedUser(chatID, 5555); err != nil {
		t.Fatalf("RemoveApprovedUser() error = %v", err)
	}

	users3 := GetApprovedUsers(chatID)
	if len(users3) != 1 {
		t.Fatalf("cache not invalidated: expected 1 user after remove, got %d", len(users3))
	}

	if err := RemoveAllApprovedUsers(chatID); err != nil {
		t.Fatalf("RemoveAllApprovedUsers() error = %v", err)
	}

	users4 := GetApprovedUsers(chatID)
	if len(users4) != 0 {
		t.Fatalf("cache not invalidated: expected 0 users after clear, got %d", len(users4))
	}
}

func TestDuplicateApprovalIsError(t *testing.T) {
	skipIfNoDb(t)

	chatID := int64(-999999999999997)

	t.Cleanup(func() {
		_ = RemoveAllApprovedUsers(chatID)
	})

	if err := AddApprovedUser(chatID, 7777, 1, ""); err != nil {
		t.Fatalf("AddApprovedUser() error = %v", err)
	}

	// Duplicate should error (GORM duplicate check on unique index)
	err := AddApprovedUser(chatID, 7777, 1, "other reason")
	if err == nil {
		t.Fatalf("AddApprovedUser() duplicate = nil, expected error")
	}
}

func TestListApprovedIDsFresh(t *testing.T) {
	skipIfNoDb(t)

	chatID := int64(-999999999999801)
	otherChatID := int64(-999999999999802)
	emptyChatID := int64(-999999999999803)

	t.Cleanup(func() {
		_ = RemoveAllApprovedUsers(chatID)
		_ = RemoveAllApprovedUsers(otherChatID)
	})

	if err := AddApprovedUser(chatID, 501, 1, ""); err != nil {
		t.Fatalf("AddApprovedUser() error = %v", err)
	}
	if err := AddApprovedUser(chatID, 502, 1, ""); err != nil {
		t.Fatalf("AddApprovedUser() error = %v", err)
	}
	if err := AddApprovedUser(otherChatID, 503, 1, ""); err != nil {
		t.Fatalf("AddApprovedUser() error = %v", err)
	}

	ids, err := ListApprovedIDsFresh(chatID)
	if err != nil {
		t.Fatalf("ListApprovedIDsFresh() error = %v", err)
	}
	if len(ids) != 2 || !ids[501] || !ids[502] {
		t.Errorf("ListApprovedIDsFresh() = %v, want exactly users 501 and 502", ids)
	}
	if ids[503] {
		t.Errorf("ListApprovedIDsFresh() lists user 503 of another chat")
	}

	empty, err := ListApprovedIDsFresh(emptyChatID)
	if err != nil {
		t.Fatalf("ListApprovedIDsFresh(empty chat) error = %v", err)
	}
	if empty == nil || len(empty) != 0 {
		t.Errorf("ListApprovedIDsFresh(empty chat) = %v, want an empty map", empty)
	}

	// A row written straight to the table is seen at once: nothing is cached in front.
	if err := db.DB.Create(&models.ApprovedUsers{ChatID: chatID, UserID: 504, ApprovedBy: 1}).Error; err != nil {
		t.Fatalf("create approved row: %v", err)
	}
	ids, err = ListApprovedIDsFresh(chatID)
	if err != nil {
		t.Fatalf("ListApprovedIDsFresh() after a direct write error = %v", err)
	}
	if !ids[504] {
		t.Errorf("ListApprovedIDsFresh() = %v, want user 504 written straight through db.DB", ids)
	}
}
