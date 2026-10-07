//go:build testtools

package lockdown

import (
	"testing"

	"github.com/divkix/Alita_Robot/alita/db"
	"github.com/divkix/Alita_Robot/alita/db/models"
)

func newJoinRecord(ld *models.ChatLockdown, userID int64) JoinRecord {
	return JoinRecord{
		LockdownID:  ld.ID,
		ChatID:      ld.ChatID,
		UserID:      userID,
		FirstName:   "Raider",
		Username:    "raider",
		Path:        models.JoinPathMember,
		InviteLink:  "https://t.me/+abc",
		PerformerID: userID,
		State:       models.JoinerStatePending,
		BanUntil:    1_900_000_000,
	}
}

func mustRecord(t *testing.T, rec JoinRecord) *models.LockdownJoiner {
	t.Helper()
	row, recorded, err := RecordJoin(rec)
	if err != nil || !recorded || row == nil || row.ID == 0 {
		t.Fatalf("RecordJoin(user %d) = %+v, %v, %v, want a new row", rec.UserID, row, recorded, err)
	}
	return row
}

func readJoiner(t *testing.T, id uint) models.LockdownJoiner {
	t.Helper()
	var row models.LockdownJoiner
	if err := db.DB.Where("id = ?", id).First(&row).Error; err != nil {
		t.Fatalf("read joiner %d: %v", id, err)
	}
	return row
}

func TestRecordJoinOncePerUser(t *testing.T) {
	chat := uniqueLockdownChatID(t)
	cleanupLockdowns(t, chat)
	ld := mustStart(t, chat)

	first := mustRecord(t, newJoinRecord(ld, 501))
	if first.State != models.JoinerStatePending || first.FirstName != "Raider" || first.Username != "raider" ||
		first.JoinPath != models.JoinPathMember || first.InviteLink != "https://t.me/+abc" ||
		first.PerformerID != 501 || first.BanUntil != 1_900_000_000 || first.ChatID != chat {
		t.Errorf("recorded row = %+v, want every field of the record", first)
	}

	again := newJoinRecord(ld, 501)
	again.FirstName = "Someone else"
	again.State = models.JoinerStateExempt
	again.BanUntil = 0
	row, recorded, err := RecordJoin(again)
	if err != nil {
		t.Fatalf("second RecordJoin error = %v, want none", err)
	}
	if recorded {
		t.Error("second RecordJoin recorded = true, want false for the same lockdown and user")
	}
	if row == nil || row.ID != first.ID || row.FirstName != "Raider" || row.State != models.JoinerStatePending {
		t.Errorf("second RecordJoin row = %+v, want the existing row unchanged", row)
	}

	var count int64
	if err := db.DB.Model(&models.LockdownJoiner{}).Where("lockdown_id = ?", ld.ID).Count(&count).Error; err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if count != 1 {
		t.Errorf("rows of the lockdown = %d, want 1", count)
	}

	mustRecord(t, newJoinRecord(ld, 502))
}

func TestJoinerClaims(t *testing.T) {
	t.Run("claim and move are conditional on the state", func(t *testing.T) {
		chat := uniqueLockdownChatID(t)
		cleanupLockdowns(t, chat)
		ld := mustStart(t, chat)
		row := mustRecord(t, newJoinRecord(ld, 601))

		won, err := ClaimJoiner(row.ID, models.JoinerStatePending, models.JoinerStateActing)
		if err != nil || !won {
			t.Fatalf("first ClaimJoiner = %v, %v, want true, nil", won, err)
		}
		if claimed := readJoiner(t, row.ID); claimed.State != models.JoinerStateActing || claimed.ClaimedAt == nil {
			t.Errorf("claimed row = %+v, want acting with claimed_at set", claimed)
		}
		won, err = ClaimJoiner(row.ID, models.JoinerStatePending, models.JoinerStateActing)
		if err != nil || won {
			t.Errorf("second ClaimJoiner = %v, %v, want false, nil", won, err)
		}

		moved, err := MoveJoiner(row.ID, models.JoinerStatePending, models.JoinerStateBanned, "", false)
		if err != nil || moved {
			t.Errorf("MoveJoiner from the wrong state = %v, %v, want false, nil", moved, err)
		}

		moved, err = MoveJoiner(row.ID, models.JoinerStateActing, models.JoinerStatePending, "boom", true)
		if err != nil || !moved {
			t.Fatalf("MoveJoiner back to pending = %v, %v, want true, nil", moved, err)
		}
		back := readJoiner(t, row.ID)
		if back.State != models.JoinerStatePending || back.Detail != "boom" || back.Attempts != 1 || back.ClaimedAt != nil {
			t.Errorf("row after a counted move = %+v, want pending, detail boom, 1 attempt, no claim", back)
		}

		if won, _ := ClaimJoiner(row.ID, models.JoinerStatePending, models.JoinerStateActing); !won {
			t.Fatal("re-claim failed")
		}
		if moved, _ := MoveJoiner(row.ID, models.JoinerStateActing, models.JoinerStatePending, "", false); !moved {
			t.Fatal("uncounted move failed")
		}
		if again := readJoiner(t, row.ID); again.Attempts != 1 || again.Detail != "" {
			t.Errorf("row after an uncounted move = %+v, want attempts unchanged at 1 and no detail", again)
		}
	})

	t.Run("a ban that took effect starts the unban with no attempts", func(t *testing.T) {
		chat := uniqueLockdownChatID(t)
		cleanupLockdowns(t, chat)
		ld := mustStart(t, chat)
		row := mustRecord(t, newJoinRecord(ld, 602))
		ClaimJoiner(row.ID, models.JoinerStatePending, models.JoinerStateActing)
		MoveJoiner(row.ID, models.JoinerStateActing, models.JoinerStatePending, "x", true)
		ClaimJoiner(row.ID, models.JoinerStatePending, models.JoinerStateActing)

		if moved, err := MoveJoiner(row.ID, models.JoinerStateActing, models.JoinerStateBanned, "", false); err != nil || !moved {
			t.Fatalf("MoveJoiner to banned = %v, %v", moved, err)
		}
		if banned := readJoiner(t, row.ID); banned.Attempts != 0 {
			t.Errorf("attempts after the ban = %d, want 0: the unban gets its own tries", banned.Attempts)
		}
	})

	t.Run("ListPendingFresh reads only confirmed active lockdowns, oldest first", func(t *testing.T) {
		chatA := uniqueLockdownChatID(t)
		chatB := uniqueLockdownChatID(t)
		chatC := uniqueLockdownChatID(t)
		cleanupLockdowns(t, chatA, chatB, chatC)

		confirmed := mustStart(t, chatA)
		if ok, err := ConfirmLocked(confirmed.ID); err != nil || !ok {
			t.Fatalf("ConfirmLocked = %v, %v", ok, err)
		}
		unconfirmed := mustStart(t, chatB)
		lifting := mustStart(t, chatC)
		if ok, err := ConfirmLocked(lifting.ID); err != nil || !ok {
			t.Fatalf("ConfirmLocked(lifting) = %v, %v", ok, err)
		}

		first := mustRecord(t, newJoinRecord(confirmed, 701))
		second := mustRecord(t, newJoinRecord(confirmed, 702))
		banned := newJoinRecord(confirmed, 703)
		banned.State = models.JoinerStateBanned
		mustRecord(t, banned)
		mustRecord(t, newJoinRecord(unconfirmed, 704))
		mustRecord(t, newJoinRecord(lifting, 705))
		if won, err := BeginLift(lifting.ID, 1, "Admin", false); err != nil || !won {
			t.Fatalf("BeginLift = %v, %v", won, err)
		}

		rows, err := ListPendingFresh(1000)
		if err != nil {
			t.Fatalf("ListPendingFresh error = %v", err)
		}
		var ours []uint
		for _, row := range rows {
			switch row.ChatID {
			case chatA, chatB, chatC:
				ours = append(ours, row.ID)
			}
		}
		if len(ours) != 2 || ours[0] != first.ID || ours[1] != second.ID {
			t.Errorf("pending rows of our chats = %v, want [%d %d]: only the confirmed active lockdown's pending rows, oldest first",
				ours, first.ID, second.ID)
		}

		limited, err := ListPendingFresh(1)
		if err != nil || len(limited) > 1 {
			t.Errorf("ListPendingFresh(1) = %d rows, %v, want at most 1", len(limited), err)
		}
	})

	t.Run("DeletePendingJoin deletes only a pending row", func(t *testing.T) {
		chat := uniqueLockdownChatID(t)
		cleanupLockdowns(t, chat)
		ld := mustStart(t, chat)
		pending := mustRecord(t, newJoinRecord(ld, 801))
		bannedRec := newJoinRecord(ld, 802)
		bannedRec.State = models.JoinerStateBanned
		banned := mustRecord(t, bannedRec)

		if deleted, err := DeletePendingJoin(banned.ID); err != nil || deleted {
			t.Errorf("DeletePendingJoin(banned) = %v, %v, want false, nil", deleted, err)
		}
		readJoiner(t, banned.ID)
		if deleted, err := DeletePendingJoin(pending.ID); err != nil || !deleted {
			t.Errorf("DeletePendingJoin(pending) = %v, %v, want true, nil", deleted, err)
		}
		var left int64
		db.DB.Model(&models.LockdownJoiner{}).Where("id = ?", pending.ID).Count(&left)
		if left != 0 {
			t.Error("the pending row is still there")
		}
	})

}
