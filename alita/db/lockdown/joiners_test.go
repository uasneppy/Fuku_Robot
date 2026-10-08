//go:build testtools

package lockdown

import (
	"testing"
	"time"

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

// reclaimStates are the finished states a re-join may take a row back from.
var reclaimStates = []string{
	models.JoinerStateBanned,
	models.JoinerStateBanFailed,
	models.JoinerStateExempt,
	models.JoinerStateDeclined,
	models.JoinerStateDeclineFailed,
	models.JoinerStateCancelled,
}

// setJoinerColumns writes columns of a joiner row directly, so a test can put a row
// in a state and age no repository function would produce.
func setJoinerColumns(t *testing.T, id uint, columns map[string]any) {
	t.Helper()
	if err := db.DB.Model(&models.LockdownJoiner{}).Where("id = ?", id).UpdateColumns(columns).Error; err != nil {
		t.Fatalf("set joiner %d columns: %v", id, err)
	}
}

func TestReclaimJoin(t *testing.T) {
	newRecord := func(ld *models.ChatLockdown, userID int64) JoinRecord {
		return JoinRecord{
			LockdownID:     ld.ID,
			ChatID:         ld.ChatID,
			UserID:         userID,
			FirstName:      "Again",
			Username:       "again",
			Path:           models.JoinPathService,
			InviteLink:     "https://t.me/+again",
			ViaJoinRequest: true,
			PerformerID:    777,
			State:          models.JoinerStatePending,
			BanUntil:       1_950_000_000,
			JoinMsgID:      4242,
		}
	}

	t.Run("wins from every listed state and resets the row", func(t *testing.T) {
		for i, from := range reclaimStates {
			chat := uniqueLockdownChatID(t)
			cleanupLockdowns(t, chat)
			ld := mustStart(t, chat)
			row := mustRecord(t, newJoinRecord(ld, int64(9100+i)))
			claimed := time.Now().UTC()
			setJoinerColumns(t, row.ID, map[string]any{
				"state": from, "attempts": 2, "detail": "old failure", "claimed_at": claimed,
				"join_msg_id": 11, "updated_at": time.Now().UTC().Add(-time.Minute),
			})

			won, err := ReclaimJoin(row.ID, reclaimStates, time.Now().Add(-20*time.Second), newRecord(ld, row.UserID))
			if err != nil || !won {
				t.Fatalf("ReclaimJoin from %s = %v, %v, want true, nil", from, won, err)
			}
			got := readJoiner(t, row.ID)
			if got.State != models.JoinerStatePending || got.JoinPath != models.JoinPathService ||
				got.BanUntil != 1_950_000_000 || got.PerformerID != 777 || got.InviteLink != "https://t.me/+again" ||
				!got.ViaJoinRequest || got.JoinMsgID != 4242 || got.FirstName != "Again" || got.Username != "again" {
				t.Errorf("from %s: row = %+v, want every field of the new record", from, got)
			}
			if got.Attempts != 0 || got.Detail != "" || got.ClaimedAt != nil {
				t.Errorf("from %s: attempts = %d, detail = %q, claimed_at = %v, want 0, empty, nil", from, got.Attempts, got.Detail, got.ClaimedAt)
			}
			if !got.UpdatedAt.After(time.Now().Add(-10 * time.Second)) {
				t.Errorf("from %s: updated_at = %v, want about now: the reclaim opens a new dedupe window", from, got.UpdatedAt)
			}
		}
	})

	t.Run("loses from a state that is not listed", func(t *testing.T) {
		for i, from := range []string{models.JoinerStatePending, models.JoinerStateActing, models.JoinerStateUnbanning, models.JoinerStateKept} {
			chat := uniqueLockdownChatID(t)
			cleanupLockdowns(t, chat)
			ld := mustStart(t, chat)
			row := mustRecord(t, newJoinRecord(ld, int64(9200+i)))
			setJoinerColumns(t, row.ID, map[string]any{"state": from, "updated_at": time.Now().UTC().Add(-time.Minute)})

			won, err := ReclaimJoin(row.ID, reclaimStates, time.Now(), newRecord(ld, row.UserID))
			if err != nil || won {
				t.Errorf("ReclaimJoin from %s = %v, %v, want false, nil", from, won, err)
			}
			if got := readJoiner(t, row.ID); got.State != from || got.JoinPath != models.JoinPathMember {
				t.Errorf("from %s: row = %+v, want it untouched", from, got)
			}
		}
	})

	t.Run("loses while the row changed after the cut-off", func(t *testing.T) {
		chat := uniqueLockdownChatID(t)
		cleanupLockdowns(t, chat)
		ld := mustStart(t, chat)
		row := mustRecord(t, newJoinRecord(ld, 9300))
		setJoinerColumns(t, row.ID, map[string]any{"state": models.JoinerStateBanned, "updated_at": time.Now().UTC().Add(-5 * time.Second)})

		won, err := ReclaimJoin(row.ID, reclaimStates, time.Now().Add(-20*time.Second), newRecord(ld, row.UserID))
		if err != nil || won {
			t.Fatalf("ReclaimJoin inside the window = %v, %v, want false, nil", won, err)
		}
		if got := readJoiner(t, row.ID); got.State != models.JoinerStateBanned {
			t.Errorf("state = %q, want banned: a join inside the dedupe window is the same join", got.State)
		}

		won, err = ReclaimJoin(row.ID, reclaimStates, time.Now(), newRecord(ld, row.UserID))
		if err != nil || !won {
			t.Fatalf("ReclaimJoin with a cut-off after the change = %v, %v, want true, nil", won, err)
		}
		if again, _ := ReclaimJoin(row.ID, reclaimStates, time.Now().Add(time.Hour), newRecord(ld, row.UserID)); again {
			t.Error("a second reclaim of the now pending row won, want exactly one winner")
		}
	})
}

func TestJoinMsgs(t *testing.T) {
	t.Run("SetJoinMsg only sets an unset message ID and leaves the dedupe window alone", func(t *testing.T) {
		chat := uniqueLockdownChatID(t)
		cleanupLockdowns(t, chat)
		ld := mustStart(t, chat)
		row := mustRecord(t, newJoinRecord(ld, 9400))
		aged := time.Now().UTC().Add(-time.Minute)
		setJoinerColumns(t, row.ID, map[string]any{"updated_at": aged})

		if err := SetJoinMsg(row.ID, 77); err != nil {
			t.Fatalf("SetJoinMsg error = %v", err)
		}
		got := readJoiner(t, row.ID)
		if got.JoinMsgID != 77 {
			t.Errorf("join_msg_id = %d, want 77", got.JoinMsgID)
		}
		if got.UpdatedAt.After(aged.Add(5 * time.Second)) {
			t.Errorf("updated_at = %v, want it unchanged: a second delivery does not open a new window", got.UpdatedAt)
		}
		if err := SetJoinMsg(row.ID, 88); err != nil {
			t.Fatalf("second SetJoinMsg error = %v", err)
		}
		if got := readJoiner(t, row.ID); got.JoinMsgID != 77 {
			t.Errorf("join_msg_id = %d, want it to stay 77: the first message wins", got.JoinMsgID)
		}
	})

	t.Run("ListJoinMsgsToDeleteFresh and ClearJoinMsg", func(t *testing.T) {
		chatActive := uniqueLockdownChatID(t)
		chatLifting := uniqueLockdownChatID(t)
		chatLifted := uniqueLockdownChatID(t)
		cleanupLockdowns(t, chatActive, chatLifting, chatLifted)

		active := mustStart(t, chatActive)
		lifting := mustStart(t, chatLifting)
		lifted := mustStart(t, chatLifted)
		if won, err := BeginLift(lifting.ID, 1, "Admin", false); err != nil || !won {
			t.Fatalf("BeginLift(lifting) = %v, %v", won, err)
		}
		if won, err := BeginLift(lifted.ID, 1, "Admin", false); err != nil || !won {
			t.Fatalf("BeginLift(lifted) = %v, %v", won, err)
		}
		if done, err := FinishLift(lifted.ID); err != nil || !done {
			t.Fatalf("FinishLift = %v, %v", done, err)
		}

		withMsg := func(ld *models.ChatLockdown, userID int64, state string, msg int64) *models.LockdownJoiner {
			rec := newJoinRecord(ld, userID)
			rec.State = state
			rec.JoinMsgID = msg
			return mustRecord(t, rec)
		}
		wantA := withMsg(active, 9501, models.JoinerStateBanned, 301)
		wantB := withMsg(active, 9502, models.JoinerStateBanned, 302)
		withMsg(active, 9503, models.JoinerStateBanned, 0)
		withMsg(active, 9504, models.JoinerStatePending, 303)
		withMsg(active, 9505, models.JoinerStateBanFailed, 304)
		wantC := withMsg(lifting, 9506, models.JoinerStateBanned, 305)
		withMsg(lifted, 9507, models.JoinerStateBanned, 306)

		listOurs := func(limit int) []uint {
			rows, err := ListJoinMsgsToDeleteFresh(limit)
			if err != nil {
				t.Fatalf("ListJoinMsgsToDeleteFresh error = %v", err)
			}
			var ours []uint
			for _, row := range rows {
				switch row.ChatID {
				case chatActive, chatLifting, chatLifted:
					ours = append(ours, row.ID)
				}
			}
			return ours
		}
		ours := listOurs(1000)
		if len(ours) != 3 || ours[0] != wantA.ID || ours[1] != wantB.ID || ours[2] != wantC.ID {
			t.Errorf("rows to delete = %v, want [%d %d %d]: banned rows with a message of an active or lifting lockdown, by ID",
				ours, wantA.ID, wantB.ID, wantC.ID)
		}
		if limited, err := ListJoinMsgsToDeleteFresh(1); err != nil || len(limited) > 1 {
			t.Errorf("ListJoinMsgsToDeleteFresh(1) = %d rows, %v, want at most 1", len(limited), err)
		}

		if err := ClearJoinMsg(wantA.ID); err != nil {
			t.Fatalf("ClearJoinMsg error = %v", err)
		}
		if got := readJoiner(t, wantA.ID); got.JoinMsgID != 0 || got.State != models.JoinerStateBanned {
			t.Errorf("row after ClearJoinMsg = %+v, want join_msg_id 0 and still banned", got)
		}
		if ours := listOurs(1000); len(ours) != 2 || ours[0] != wantB.ID || ours[1] != wantC.ID {
			t.Errorf("rows to delete after the clear = %v, want [%d %d]", ours, wantB.ID, wantC.ID)
		}
	})
}

func TestHasJoinerBanFresh(t *testing.T) {
	chat := uniqueLockdownChatID(t)
	otherChat := uniqueLockdownChatID(t)
	cleanupLockdowns(t, chat, otherChat)
	ld := mustStart(t, chat)
	const until int64 = 1_900_000_000
	rec := newJoinRecord(ld, 9600)
	rec.State = models.JoinerStateBanned
	rec.BanUntil = until
	mustRecord(t, rec)

	tests := []struct {
		name  string
		chat  int64
		user  int64
		until int64
		want  bool
	}{
		{"the exact end date", chat, 9600, until, true},
		{"2 s earlier", chat, 9600, until - 2, true},
		{"2 s later", chat, 9600, until + 2, true},
		{"3 s earlier", chat, 9600, until - 3, false},
		{"3 s later", chat, 9600, until + 3, false},
		{"a permanent ban", chat, 9600, 0, false},
		{"another chat", otherChat, 9600, until, false},
		{"another user", chat, 9601, until, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := HasJoinerBanFresh(tt.chat, tt.user, tt.until)
			if err != nil || got != tt.want {
				t.Errorf("HasJoinerBanFresh(%d, %d, %d) = %v, %v, want %v, nil", tt.chat, tt.user, tt.until, got, err, tt.want)
			}
		})
	}
}

// seedClaimedJoiner records a joiner row and puts it straight into a claimed state
// with the given attempts, claimed that long ago.
func seedClaimedJoiner(t *testing.T, ld *models.ChatLockdown, userID int64, path, state string, attempts int, claimedAgo time.Duration) uint {
	t.Helper()
	rec := newJoinRecord(ld, userID)
	rec.Path = path
	row := mustRecord(t, rec)
	claimed := time.Now().UTC().Add(-claimedAgo)
	err := db.DB.Model(&models.LockdownJoiner{}).Where("id = ?", row.ID).
		Updates(map[string]any{"state": state, "attempts": attempts, "claimed_at": claimed}).Error
	if err != nil {
		t.Fatalf("seed claimed joiner %d: %v", userID, err)
	}
	return row.ID
}

func TestReleaseStaleClaims(t *testing.T) {
	activeChat := uniqueLockdownChatID(t)
	liftingChat := uniqueLockdownChatID(t)
	cleanupLockdowns(t, activeChat, liftingChat)
	active := mustStart(t, activeChat)
	lifting := mustStart(t, liftingChat)
	if won, err := BeginLift(lifting.ID, 7, "Lifter", false); err != nil || !won {
		t.Fatalf("BeginLift = %v, %v, want true, nil", won, err)
	}

	const stale = 5 * time.Minute
	activeActing := seedClaimedJoiner(t, active, 7001, models.JoinPathMember, models.JoinerStateActing, 2, stale)
	activeRequest := seedClaimedJoiner(t, active, 7002, models.JoinPathRequest, models.JoinerStateActing, 1, stale)
	activeUnbanning := seedClaimedJoiner(t, active, 7003, models.JoinPathMember, models.JoinerStateUnbanning, 1, stale)
	activeFresh := seedClaimedJoiner(t, active, 7004, models.JoinPathMember, models.JoinerStateActing, 0, 0)
	liftingBan := seedClaimedJoiner(t, lifting, 7101, models.JoinPathMember, models.JoinerStateActing, 2, stale)
	liftingRequest := seedClaimedJoiner(t, lifting, 7102, models.JoinPathRequest, models.JoinerStateActing, 0, stale)
	liftingUnbanning := seedClaimedJoiner(t, lifting, 7103, models.JoinPathMember, models.JoinerStateUnbanning, 2, stale)
	liftingFresh := seedClaimedJoiner(t, lifting, 7104, models.JoinPathMember, models.JoinerStateUnbanning, 0, time.Minute)
	untouched := mustRecord(t, newJoinRecord(active, 7005))

	released, err := ReleaseStaleClaims(time.Now().UTC().Add(-2 * time.Minute))
	if err != nil {
		t.Fatalf("ReleaseStaleClaims error = %v, want none", err)
	}
	if released != 6 {
		t.Errorf("ReleaseStaleClaims released %d rows, want the 6 stale ones", released)
	}

	tests := []struct {
		name     string
		id       uint
		state    string
		attempts int
		claimed  bool
	}{
		{"stale acting ban of an active lockdown is retried", activeActing, models.JoinerStatePending, 2, false},
		{"stale acting request of an active lockdown is retried", activeRequest, models.JoinerStatePending, 1, false},
		{"stale unbanning goes back to banned", activeUnbanning, models.JoinerStateBanned, 1, false},
		{"fresh acting claim stays", activeFresh, models.JoinerStateActing, 0, true},
		{"stale acting ban of a lifting lockdown is banned", liftingBan, models.JoinerStateBanned, 2, false},
		{"stale acting request of a lifting lockdown is cancelled", liftingRequest, models.JoinerStateCancelled, 0, false},
		{"stale unbanning of a lifting lockdown is banned", liftingUnbanning, models.JoinerStateBanned, 2, false},
		{"unbanning claimed a minute ago stays", liftingFresh, models.JoinerStateUnbanning, 0, true},
		{"a row nobody claimed is untouched", untouched.ID, models.JoinerStatePending, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			row := readJoiner(t, tt.id)
			if row.State != tt.state || row.Attempts != tt.attempts || (row.ClaimedAt != nil) != tt.claimed {
				t.Errorf("row = state %q, attempts %d, claimed %v; want state %q, attempts %d, claimed %v",
					row.State, row.Attempts, row.ClaimedAt != nil, tt.state, tt.attempts, tt.claimed)
			}
		})
	}

	again, err := ReleaseStaleClaims(time.Now().UTC().Add(-2 * time.Minute))
	if err != nil || again != 0 {
		t.Errorf("a second ReleaseStaleClaims = %d, %v, want 0, nil: released rows are not released twice", again, err)
	}
}
