//go:build testtools

package user

import (
	"testing"
	"time"

	"github.com/divkix/Alita_Robot/alita/db"
	"github.com/divkix/Alita_Robot/alita/db/models"
)

func TestFindUsersByIDs(t *testing.T) {
	t.Run("returns the rows asked for and ignores unknown IDs", func(t *testing.T) {
		seedUsernameRows(t,
			models.User{UserId: 9201, UserName: "FirstAsked", Name: "First Asked", LastActivity: time.Now()},
			models.User{UserId: 9202, UserName: "SecondAsked", Name: "Second Asked", LastActivity: time.Now()},
			models.User{UserId: 9203, UserName: "NotAsked", Name: "Not Asked", LastActivity: time.Now()},
		)

		got, err := FindUsersByIDs([]int64{9201, 9202, 9299})
		if err != nil {
			t.Fatalf("FindUsersByIDs error = %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("FindUsersByIDs returned %d rows (%v), want 2: the unknown ID and the row not asked for are absent", len(got), got)
		}
		first, second := got[9201], got[9202]
		if first.UserName != "FirstAsked" || first.Name != "First Asked" {
			t.Errorf("row 9201 = {%q %q}, want {\"FirstAsked\" \"First Asked\"}", first.UserName, first.Name)
		}
		if second.UserName != "SecondAsked" || second.Name != "Second Asked" {
			t.Errorf("row 9202 = {%q %q}, want {\"SecondAsked\" \"Second Asked\"}", second.UserName, second.Name)
		}
		if _, ok := got[9299]; ok {
			t.Error("FindUsersByIDs returned a row for the unknown ID 9299")
		}
	})

	t.Run("an empty input is an empty map", func(t *testing.T) {
		skipIfNoDb(t)
		for _, ids := range [][]int64{nil, {}} {
			got, err := FindUsersByIDs(ids)
			if err != nil {
				t.Fatalf("FindUsersByIDs(%v) error = %v, want nil", ids, err)
			}
			if got == nil || len(got) != 0 {
				t.Fatalf("FindUsersByIDs(%v) = %v, want an empty map that is safe to read", ids, got)
			}
		}
	})

	t.Run("a database error is returned", func(t *testing.T) {
		skipIfNoDb(t)
		// A dropped table stands in for an unreachable database: the lookup must say so,
		// never report "no rows".
		tx := db.DB.Begin()
		t.Cleanup(func() { tx.Rollback() })
		if err := tx.Exec("DROP TABLE users").Error; err != nil {
			t.Fatalf("drop users inside the transaction: %v", err)
		}
		original := db.DB
		db.DB = tx
		t.Cleanup(func() { db.DB = original })

		if _, err := FindUsersByIDs([]int64{9211}); err == nil {
			t.Fatal("FindUsersByIDs error = nil, want the database error")
		}
	})
}
