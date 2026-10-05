//go:build testtools

package user

import (
	"testing"
	"time"

	"github.com/divkix/Alita_Robot/alita/db"
	"github.com/divkix/Alita_Robot/alita/db/models"
)

// seedUsernameRows inserts users rows with explicit last_activity values and
// removes them when the test ends.
func seedUsernameRows(t *testing.T, rows ...models.User) {
	t.Helper()
	skipIfNoDb(t)
	ids := make([]int64, 0, len(rows))
	for i := range rows {
		ids = append(ids, rows[i].UserId)
	}
	t.Cleanup(func() { db.DB.Where("user_id IN ?", ids).Delete(&models.User{}) })
	for i := range rows {
		if err := db.DB.Create(&rows[i]).Error; err != nil {
			t.Fatalf("seed user %d: %v", rows[i].UserId, err)
		}
	}
}

func userIDsOf(rows []models.User) []int64 {
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.UserId)
	}
	return ids
}

func equalIDs(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestFindUsersByUsernameCaseInsensitiveNewestFirst(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	seedUsernameRows(t,
		models.User{UserId: 9101, UserName: "SpamBot1", Name: "Spam One", LastActivity: now.Add(-2 * time.Hour)},
		models.User{UserId: 9102, UserName: "spambot1", Name: "Spam Two", LastActivity: now.Add(-1 * time.Hour)},
		models.User{UserId: 9103, UserName: "Other", Name: "Other", LastActivity: now},
	)

	for _, query := range []string{"SPAMBOT1", "@spambot1", "  @SpamBot1 "} {
		got, err := FindUsersByUsername(query, 10)
		if err != nil {
			t.Fatalf("FindUsersByUsername(%q) error = %v", query, err)
		}
		if want := []int64{9102, 9101}; !equalIDs(userIDsOf(got), want) {
			t.Fatalf("FindUsersByUsername(%q) ids = %v, want %v (newest last_activity first)", query, userIDsOf(got), want)
		}
	}
}

func TestFindUsersByUsernameLimit(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	seedUsernameRows(t,
		models.User{UserId: 9111, UserName: "LimitUser", Name: "A", LastActivity: now.Add(-2 * time.Hour)},
		models.User{UserId: 9112, UserName: "limituser", Name: "B", LastActivity: now.Add(-1 * time.Hour)},
	)

	got, err := FindUsersByUsername("limituser", 1)
	if err != nil {
		t.Fatalf("FindUsersByUsername error = %v", err)
	}
	if want := []int64{9112}; !equalIDs(userIDsOf(got), want) {
		t.Fatalf("limit 1 ids = %v, want %v", userIDsOf(got), want)
	}
}

func TestFindUsersByUsernameNoMatch(t *testing.T) {
	skipIfNoDb(t)
	got, err := FindUsersByUsername("nobody_xyz_unseen", 10)
	if err != nil {
		t.Fatalf("FindUsersByUsername error = %v, want nil for no match", err)
	}
	if len(got) != 0 {
		t.Fatalf("FindUsersByUsername returned %d rows, want 0", len(got))
	}
}

func TestFindUsersByUsernameEmptyInput(t *testing.T) {
	skipIfNoDb(t)
	for _, query := range []string{"", "@", "  ", " @ "} {
		got, err := FindUsersByUsername(query, 10)
		if err != nil || got != nil {
			t.Fatalf("FindUsersByUsername(%q) = (%v, %v), want (nil, nil)", query, got, err)
		}
	}
}

func TestFindUsersByUsernameUnderscoreIsLiteral(t *testing.T) {
	seedUsernameRows(t,
		models.User{UserId: 9121, UserName: "spamXbot", Name: "Wrong", LastActivity: time.Now()},
		models.User{UserId: 9122, UserName: "spam_bot", Name: "Right", LastActivity: time.Now().Add(-time.Hour)},
	)

	got, err := FindUsersByUsername("spam_bot", 10)
	if err != nil {
		t.Fatalf("FindUsersByUsername error = %v", err)
	}
	if want := []int64{9122}; !equalIDs(userIDsOf(got), want) {
		t.Fatalf("ids = %v, want only %v: an underscore must match literally", userIDsOf(got), want)
	}
}

func TestFindUsersByUsernameCarriesFields(t *testing.T) {
	active := time.Now().UTC().Truncate(time.Second).Add(-3 * time.Hour)
	seedUsernameRows(t, models.User{UserId: 9131, UserName: "FieldUser", Name: "Field Name", LastActivity: active})

	got, err := FindUsersByUsername("fielduser", 10)
	if err != nil || len(got) != 1 {
		t.Fatalf("FindUsersByUsername = (%v, %v), want one row", got, err)
	}
	row := got[0]
	if row.UserId != 9131 || row.UserName != "FieldUser" || row.Name != "Field Name" {
		t.Fatalf("row = {%d %q %q}, want {9131 \"FieldUser\" \"Field Name\"}", row.UserId, row.UserName, row.Name)
	}
	if !row.LastActivity.Equal(active) {
		t.Fatalf("LastActivity = %v, want %v", row.LastActivity, active)
	}
}

func TestFindUsersByUsernameReturnsDatabaseError(t *testing.T) {
	skipIfNoDb(t)
	// A dropped table stands in for an unreachable database: the lookup must say so,
	// never report "no match".
	tx := db.DB.Begin()
	t.Cleanup(func() { tx.Rollback() })
	if err := tx.Exec("DROP TABLE users").Error; err != nil {
		t.Fatalf("drop users inside the transaction: %v", err)
	}
	original := db.DB
	db.DB = tx
	t.Cleanup(func() { db.DB = original })

	if _, err := FindUsersByUsername("anyone_here", 10); err == nil {
		t.Fatal("FindUsersByUsername error = nil, want the database error")
	}
}

func TestGetUserIdByUserNameUnchanged(t *testing.T) {
	seedUsernameRows(t, models.User{UserId: 9141, UserName: "ExactCase", Name: "Exact", LastActivity: time.Now()})
	if got := GetUserIdByUserName("ExactCase"); got != 9141 {
		t.Fatalf("GetUserIdByUserName exact = %d, want 9141", got)
	}
	if got := GetUserIdByUserName("exactcase"); got != 0 {
		t.Fatalf("GetUserIdByUserName wrong case = %d, want 0 (existing exact-case behaviour)", got)
	}
}
