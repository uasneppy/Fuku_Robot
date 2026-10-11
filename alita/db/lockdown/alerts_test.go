//go:build testtools

package lockdown

import (
	"fmt"
	"testing"
	"time"

	"github.com/divkix/Alita_Robot/alita/db"
	"github.com/divkix/Alita_Robot/alita/db/models"
)

// cleanupAlertLockdowns deletes the lockdown rows of the given chats, and their alert
// copies first, when the test ends.
func cleanupAlertLockdowns(t *testing.T, chatIDs ...int64) {
	t.Helper()
	cleanupLockdowns(t, chatIDs...)
	t.Cleanup(func() {
		db.DB.Where("chat_id IN ?", chatIDs).Delete(&models.ChatLockdownAlert{})
	})
}

// alertSlot is one copy of the plan: an alert for destination dest at sequence seq.
func alertSlot(ld *models.ChatLockdown, kind string, seq int, dest string, destChat int64) models.ChatLockdownAlert {
	return models.ChatLockdownAlert{
		LockdownID: ld.ID,
		ChatID:     ld.ChatID,
		Kind:       kind,
		Seq:        seq,
		Dest:       dest,
		DestChatID: destChat,
	}
}

// confirmedLockdown starts and confirms a lockdown in a fresh chat.
func confirmedLockdown(t *testing.T) *models.ChatLockdown {
	t.Helper()
	chat := uniqueLockdownChatID(t)
	cleanupAlertLockdowns(t, chat)
	ld := mustStart(t, chat)
	mustConfirm(t, ld.ID)
	return ld
}

// mustBegin decides the alert of ld with the given copies and returns its rows, oldest
// first.
func mustBegin(t *testing.T, ld *models.ChatLockdown, slots ...models.ChatLockdownAlert) []models.ChatLockdownAlert {
	t.Helper()
	won, err := BeginAlert(ld.ID, AlertPlan{Slots: slots})
	if err != nil || !won {
		t.Fatalf("BeginAlert = %v, %v, want true", won, err)
	}
	return readAlertRows(t, ld.ID)
}

func readAlertRows(t *testing.T, lockdownID uint) []models.ChatLockdownAlert {
	t.Helper()
	var rows []models.ChatLockdownAlert
	if err := db.DB.Where("lockdown_id = ?", lockdownID).Order("id").Find(&rows).Error; err != nil {
		t.Fatalf("read alert rows: %v", err)
	}
	return rows
}

func readAlert(t *testing.T, id uint) models.ChatLockdownAlert {
	t.Helper()
	row, err := GetAlertSlotFresh(id)
	if err != nil || row == nil {
		t.Fatalf("GetAlertSlotFresh(%d) = %v, %v, want the row", id, row, err)
	}
	return *row
}

// mustPost moves a pending copy to posted with message ID msgID.
func mustPost(t *testing.T, id uint, msgID int64) {
	t.Helper()
	if ok, err := ClaimAlertSlot(id); err != nil || !ok {
		t.Fatalf("ClaimAlertSlot(%d) = %v, %v, want true", id, ok, err)
	}
	if ok, err := RecordAlertPosted(id, msgID, "body", ""); err != nil || !ok {
		t.Fatalf("RecordAlertPosted(%d) = %v, %v, want true", id, ok, err)
	}
}

func mustLift(t *testing.T, id uint) {
	t.Helper()
	if ok, err := BeginLift(id, 7, "Lifter", false); err != nil || !ok {
		t.Fatalf("BeginLift(%d) = %v, %v, want true", id, ok, err)
	}
}

// idsOf lists the IDs of rows that belong to lockdownID, in the order given.
func idsOf(rows []models.ChatLockdownAlert, lockdownID uint) []uint {
	var ids []uint
	for _, row := range rows {
		if row.LockdownID == lockdownID {
			ids = append(ids, row.ID)
		}
	}
	return ids
}

func sameIDs(got, want []uint) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func TestBeginAlert(t *testing.T) {
	t.Run("decided once with its rows", func(t *testing.T) {
		ld := confirmedLockdown(t)
		plan := AlertPlan{Slots: []models.ChatLockdownAlert{
			alertSlot(ld, models.AlertKindAlert, 0, models.AlertDestStaff, -100111),
			alertSlot(ld, models.AlertKindAlert, 0, models.AlertDestLog, -100222),
		}}

		won, err := BeginAlert(ld.ID, plan)
		if err != nil || !won {
			t.Fatalf("first BeginAlert = %v, %v, want true", won, err)
		}
		rows := readAlertRows(t, ld.ID)
		if len(rows) != 2 {
			t.Fatalf("%d rows inserted, want 2", len(rows))
		}
		for _, row := range rows {
			if row.State != models.AlertStatePending || row.LockdownID != ld.ID || row.ChatID != ld.ChatID {
				t.Errorf("row = %+v, want a pending copy of lockdown %d", row, ld.ID)
			}
		}
		current, err := GetFresh(ld.ID)
		if err != nil || current == nil || current.AlertStartedAt == nil {
			t.Fatalf("GetFresh = %v, %v, want alert_started_at set", current, err)
		}

		won, err = BeginAlert(ld.ID, plan)
		if err != nil || won {
			t.Errorf("second BeginAlert = %v, %v, want false and no error", won, err)
		}
		if rows := readAlertRows(t, ld.ID); len(rows) != 2 {
			t.Errorf("%d rows after the second call, want still 2", len(rows))
		}
	})

	t.Run("a plan with no copies still decides", func(t *testing.T) {
		ld := confirmedLockdown(t)

		won, err := BeginAlert(ld.ID, AlertPlan{})
		if err != nil || !won {
			t.Fatalf("BeginAlert = %v, %v, want true", won, err)
		}
		current, err := GetFresh(ld.ID)
		if err != nil || current == nil || current.AlertStartedAt == nil {
			t.Errorf("GetFresh = %v, %v, want alert_started_at set", current, err)
		}
		if rows := readAlertRows(t, ld.ID); len(rows) != 0 {
			t.Errorf("%d rows, want none", len(rows))
		}
	})

	t.Run("an unconfirmed lockdown is not decided", func(t *testing.T) {
		chat := uniqueLockdownChatID(t)
		cleanupAlertLockdowns(t, chat)
		ld := mustStart(t, chat)

		won, err := BeginAlert(ld.ID, AlertPlan{Slots: []models.ChatLockdownAlert{
			alertSlot(ld, models.AlertKindAlert, 0, models.AlertDestStaff, -100111),
		}})
		if err != nil || won {
			t.Fatalf("BeginAlert = %v, %v, want false and no error", won, err)
		}
		if rows := readAlertRows(t, ld.ID); len(rows) != 0 {
			t.Errorf("%d rows, want none", len(rows))
		}
		if current, _ := GetFresh(ld.ID); current == nil || current.AlertStartedAt != nil {
			t.Errorf("lockdown = %+v, want alert_started_at still unset", current)
		}
	})

	t.Run("a lifting lockdown is not decided", func(t *testing.T) {
		ld := confirmedLockdown(t)
		mustLift(t, ld.ID)

		won, err := BeginAlert(ld.ID, AlertPlan{Slots: []models.ChatLockdownAlert{
			alertSlot(ld, models.AlertKindAlert, 0, models.AlertDestStaff, -100111),
		}})
		if err != nil || won {
			t.Fatalf("BeginAlert = %v, %v, want false and no error", won, err)
		}
		if rows := readAlertRows(t, ld.ID); len(rows) != 0 {
			t.Errorf("%d rows, want none", len(rows))
		}
	})
}

func TestAlertSlots(t *testing.T) {
	t.Run("claim, post and read back", func(t *testing.T) {
		ld := confirmedLockdown(t)
		rows := mustBegin(t, ld, alertSlot(ld, models.AlertKindAlert, 0, models.AlertDestStaff, -100111))
		id := rows[0].ID

		if ok, err := ClaimAlertSlot(id); err != nil || !ok {
			t.Fatalf("ClaimAlertSlot = %v, %v, want true", ok, err)
		}
		if ok, err := ClaimAlertSlot(id); err != nil || ok {
			t.Errorf("second ClaimAlertSlot = %v, %v, want false: one claim per copy", ok, err)
		}
		if row := readAlert(t, id); row.State != models.AlertStatePosting || row.ClaimedAt == nil {
			t.Errorf("claimed row = %+v, want posting with a claim", row)
		}

		if ok, err := RecordAlertPosted(id, 4242, "the text sent", "https://t.me/c/1/2"); err != nil || !ok {
			t.Fatalf("RecordAlertPosted = %v, %v, want true", ok, err)
		}
		row := readAlert(t, id)
		if row.State != models.AlertStatePosted || row.MessageID != 4242 || row.Body != "the text sent" ||
			row.Link != "https://t.me/c/1/2" || row.Attempts != 0 || row.ClaimedAt != nil {
			t.Errorf("posted row = %+v, want posted with message, body, link, no attempts and no claim", row)
		}
		if ok, err := RecordAlertPosted(id, 9999, "again", ""); err != nil || ok {
			t.Errorf("second RecordAlertPosted = %v, %v, want false: only a posting row moves", ok, err)
		}
		if got := readAlert(t, id); got.MessageID != 4242 {
			t.Errorf("MessageID = %d after a refused record, want 4242", got.MessageID)
		}
	})

	t.Run("release counts an attempt only when asked", func(t *testing.T) {
		ld := confirmedLockdown(t)
		rows := mustBegin(t, ld, alertSlot(ld, models.AlertKindAlert, 0, models.AlertDestStaff, -100111))
		id := rows[0].ID

		if ok, err := ReleaseAlertSlot(id, "x", true); err != nil || ok {
			t.Errorf("ReleaseAlertSlot on a pending copy = %v, %v, want false", ok, err)
		}
		if ok, err := ClaimAlertSlot(id); err != nil || !ok {
			t.Fatalf("ClaimAlertSlot = %v, %v, want true", ok, err)
		}
		if ok, err := ReleaseAlertSlot(id, "", false); err != nil || !ok {
			t.Fatalf("ReleaseAlertSlot = %v, %v, want true", ok, err)
		}
		if row := readAlert(t, id); row.State != models.AlertStatePending || row.Attempts != 0 || row.ClaimedAt != nil {
			t.Errorf("row after a free release = %+v, want pending, 0 attempts, no claim", row)
		}
		if ok, _ := ClaimAlertSlot(id); !ok {
			t.Fatal("ClaimAlertSlot after a release = false, want true")
		}
		if ok, err := ReleaseAlertSlot(id, "chat not found", true); err != nil || !ok {
			t.Fatalf("ReleaseAlertSlot counting = %v, %v, want true", ok, err)
		}
		if row := readAlert(t, id); row.State != models.AlertStatePending || row.Attempts != 1 || row.Detail != "chat not found" {
			t.Errorf("row after a counted release = %+v, want pending, 1 attempt, the detail", row)
		}
	})

	t.Run("fail is final", func(t *testing.T) {
		ld := confirmedLockdown(t)
		rows := mustBegin(t, ld, alertSlot(ld, models.AlertKindAlert, 0, models.AlertDestStaff, -100111))
		id := rows[0].ID
		if ok, _ := ClaimAlertSlot(id); !ok {
			t.Fatal("ClaimAlertSlot = false, want true")
		}

		if ok, err := FailAlertSlot(id, "kicked"); err != nil || !ok {
			t.Fatalf("FailAlertSlot = %v, %v, want true", ok, err)
		}
		row := readAlert(t, id)
		if row.State != models.AlertStateFailed || row.Attempts != 1 || row.Detail != "kicked" || row.ClaimedAt != nil {
			t.Errorf("failed row = %+v, want failed, 1 attempt, detail, no claim", row)
		}
		if ok, _ := ClaimAlertSlot(id); ok {
			t.Error("ClaimAlertSlot on a failed copy = true, want false")
		}
		if ok, _ := ReleaseAlertSlot(id, "", false); ok {
			t.Error("ReleaseAlertSlot on a failed copy = true, want false")
		}
	})

	t.Run("pending list holds only pending copies of active lockdowns, oldest first", func(t *testing.T) {
		active := confirmedLockdown(t)
		activeRows := mustBegin(t, active,
			alertSlot(active, models.AlertKindAlert, 0, models.AlertDestStaff, -100111),
			alertSlot(active, models.AlertKindAlert, 0, models.AlertDestLog, -100222),
			alertSlot(active, models.AlertKindReminder, 1, models.AlertDestStaff, -100111),
		)
		if ok, _ := ClaimAlertSlot(activeRows[1].ID); !ok {
			t.Fatal("ClaimAlertSlot = false, want true")
		}
		lifting := confirmedLockdown(t)
		liftingRows := mustBegin(t, lifting, alertSlot(lifting, models.AlertKindAlert, 0, models.AlertDestStaff, -100111))
		mustLift(t, lifting.ID)

		listed, err := ListPendingAlertSlotsFresh(1000)
		if err != nil {
			t.Fatalf("ListPendingAlertSlotsFresh error = %v", err)
		}
		want := []uint{activeRows[0].ID, activeRows[2].ID}
		if got := idsOf(listed, active.ID); !sameIDs(got, want) {
			t.Errorf("pending copies of the active lockdown = %v, want %v (the claimed one left out, oldest first)", got, want)
		}
		if got := idsOf(listed, lifting.ID); len(got) != 0 {
			t.Errorf("pending copies of the lifting lockdown = %v, want none (row %d)", got, liftingRows[0].ID)
		}
	})

	t.Run("a copy slot exists once", func(t *testing.T) {
		ld := confirmedLockdown(t)
		mustBegin(t, ld, alertSlot(ld, models.AlertKindAlert, 0, models.AlertDestStaff, -100111))

		duplicate := alertSlot(ld, models.AlertKindAlert, 0, models.AlertDestStaff, -100999)
		duplicate.State = models.AlertStatePending
		if err := db.DB.Create(&duplicate).Error; err == nil {
			t.Error("a second row for the same (lockdown, kind, seq, dest) was accepted, want it refused")
		}
		if rows := readAlertRows(t, ld.ID); len(rows) != 1 {
			t.Errorf("%d rows, want 1", len(rows))
		}
	})
}

func TestAlertClose(t *testing.T) {
	// postedCopies decides an alert of three posted copies for a fresh lockdown.
	postedCopies := func(t *testing.T) (*models.ChatLockdown, []uint) {
		t.Helper()
		ld := confirmedLockdown(t)
		rows := mustBegin(t, ld,
			alertSlot(ld, models.AlertKindAlert, 0, models.AlertDestStaff, -100111),
			alertSlot(ld, models.AlertKindReminder, 1, models.AlertDestStaff, -100111),
			alertSlot(ld, models.AlertKindReminder, 2, models.AlertDestStaff, -100111),
		)
		ids := make([]uint, len(rows))
		for i, row := range rows {
			mustPost(t, row.ID, int64(5000+i))
			ids[i] = row.ID
		}
		return ld, ids
	}

	t.Run("an active lockdown's copies are never listed", func(t *testing.T) {
		ld, _ := postedCopies(t)

		listed, err := ListAlertSlotsToCloseFresh(1000)
		if err != nil {
			t.Fatalf("ListAlertSlotsToCloseFresh error = %v", err)
		}
		if got := idsOf(listed, ld.ID); len(got) != 0 {
			t.Errorf("copies of an active lockdown listed to close = %v, want none", got)
		}
	})

	t.Run("a lifted lockdown's posted copies are listed newest first", func(t *testing.T) {
		ld, ids := postedCopies(t)
		pending := confirmedLockdown(t)
		pendingRows := mustBegin(t, pending, alertSlot(pending, models.AlertKindAlert, 0, models.AlertDestStaff, -100111))
		mustLift(t, ld.ID)
		mustLift(t, pending.ID)

		listed, err := ListAlertSlotsToCloseFresh(1000)
		if err != nil {
			t.Fatalf("ListAlertSlotsToCloseFresh error = %v", err)
		}
		want := []uint{ids[2], ids[1], ids[0]}
		if got := idsOf(listed, ld.ID); !sameIDs(got, want) {
			t.Errorf("copies listed to close = %v, want %v, newest first", got, want)
		}
		if got := idsOf(listed, pending.ID); len(got) != 0 {
			t.Errorf("a copy that was never posted is listed to close (%v, row %d), want none", got, pendingRows[0].ID)
		}

		limited, err := ListAlertSlotsToCloseFresh(2)
		if err != nil || len(limited) > 2 {
			t.Errorf("ListAlertSlotsToCloseFresh(2) = %d rows, %v, want at most 2", len(limited), err)
		}
	})

	t.Run("a claim is won once and a claimed copy is not listed", func(t *testing.T) {
		ld, ids := postedCopies(t)
		mustLift(t, ld.ID)

		if ok, err := ClaimAlertClose(ids[0]); err != nil || !ok {
			t.Fatalf("ClaimAlertClose = %v, %v, want true", ok, err)
		}
		if ok, err := ClaimAlertClose(ids[0]); err != nil || ok {
			t.Errorf("second ClaimAlertClose = %v, %v, want false", ok, err)
		}
		if row := readAlert(t, ids[0]); row.ClaimedAt == nil || row.State != models.AlertStatePosted {
			t.Errorf("claimed row = %+v, want posted with a claim", row)
		}
		listed, err := ListAlertSlotsToCloseFresh(1000)
		if err != nil {
			t.Fatalf("ListAlertSlotsToCloseFresh error = %v", err)
		}
		want := []uint{ids[2], ids[1]}
		if got := idsOf(listed, ld.ID); !sameIDs(got, want) {
			t.Errorf("copies listed to close = %v, want %v (the claimed one left out)", got, want)
		}
	})

	t.Run("a closed copy keeps its detail and is not listed again", func(t *testing.T) {
		ld, ids := postedCopies(t)
		mustLift(t, ld.ID)
		if ok, _ := ClaimAlertClose(ids[0]); !ok {
			t.Fatal("ClaimAlertClose = false, want true")
		}

		if ok, err := RecordAlertClosed(ids[0], "gone"); err != nil || !ok {
			t.Fatalf("RecordAlertClosed = %v, %v, want true", ok, err)
		}
		row := readAlert(t, ids[0])
		if row.ClosedAt == nil || row.ClaimedAt != nil || row.ButtonsLive || row.EditPending || row.Detail != "gone" {
			t.Errorf("closed row = %+v, want closed_at set, no claim, buttons_live and edit_pending false, detail gone", row)
		}
		if ok, err := RecordAlertClosed(ids[0], "again"); err != nil || ok {
			t.Errorf("second RecordAlertClosed = %v, %v, want false", ok, err)
		}
		if ok, _ := ClaimAlertClose(ids[0]); ok {
			t.Error("ClaimAlertClose on a closed copy = true, want false")
		}
		listed, _ := ListAlertSlotsToCloseFresh(1000)
		for _, id := range idsOf(listed, ld.ID) {
			if id == ids[0] {
				t.Errorf("closed copy %d is listed to close again", id)
			}
		}
	})

	t.Run("release clears the claim and counts an attempt only when asked", func(t *testing.T) {
		ld, ids := postedCopies(t)
		mustLift(t, ld.ID)

		if ok, err := ReleaseAlertClose(ids[1], true); err != nil || ok {
			t.Errorf("ReleaseAlertClose on an unclaimed copy = %v, %v, want false", ok, err)
		}
		if ok, _ := ClaimAlertClose(ids[1]); !ok {
			t.Fatal("ClaimAlertClose = false, want true")
		}
		if ok, err := ReleaseAlertClose(ids[1], false); err != nil || !ok {
			t.Fatalf("ReleaseAlertClose = %v, %v, want true", ok, err)
		}
		if row := readAlert(t, ids[1]); row.ClaimedAt != nil || row.Attempts != 0 || row.ClosedAt != nil {
			t.Errorf("row after a free release = %+v, want open, unclaimed, 0 attempts", row)
		}
		if ok, _ := ClaimAlertClose(ids[1]); !ok {
			t.Fatal("ClaimAlertClose after a release = false, want true")
		}
		if ok, err := ReleaseAlertClose(ids[1], true); err != nil || !ok {
			t.Fatalf("ReleaseAlertClose counting = %v, %v, want true", ok, err)
		}
		if row := readAlert(t, ids[1]); row.ClaimedAt != nil || row.Attempts != 1 || row.ClosedAt != nil {
			t.Errorf("row after a counted release = %+v, want open, unclaimed, 1 attempt", row)
		}
		listed, _ := ListAlertSlotsToCloseFresh(1000)
		found := false
		for _, id := range idsOf(listed, ld.ID) {
			found = found || id == ids[1]
		}
		if !found {
			t.Error("a released copy is not listed to close again, want it listed")
		}
	})
}

// ageAlertClaim moves the claim time of a copy row back by claimedAgo.
func ageAlertClaim(t *testing.T, id uint, claimedAgo time.Duration) {
	t.Helper()
	err := db.DB.Model(&models.ChatLockdownAlert{}).Where("id = ?", id).
		UpdateColumn("claimed_at", time.Now().UTC().Add(-claimedAgo)).Error
	if err != nil {
		t.Fatalf("age the claim of alert copy %d: %v", id, err)
	}
}

func TestReleaseStaleAlertClaims(t *testing.T) {
	ld := confirmedLockdown(t)
	rows := mustBegin(t, ld,
		alertSlot(ld, models.AlertKindAlert, 0, models.AlertDestStaff, -100111),
		alertSlot(ld, models.AlertKindAlert, 0, models.AlertDestLog, -100222),
		alertSlot(ld, models.AlertKindReminder, 1, models.AlertDestStaff, -100111),
		alertSlot(ld, models.AlertKindReminder, 1, models.AlertDestLog, -100222),
	)
	staleSend, liveSend, staleClose, liveClose := rows[0].ID, rows[1].ID, rows[2].ID, rows[3].ID

	// A copy whose send stopped half way: posting, with one failed try before.
	if ok, _ := ClaimAlertSlot(staleSend); !ok {
		t.Fatal("ClaimAlertSlot(staleSend) = false, want true")
	}
	if ok, err := ReleaseAlertSlot(staleSend, "earlier failure", true); err != nil || !ok {
		t.Fatalf("ReleaseAlertSlot = %v, %v, want true", ok, err)
	}
	if ok, _ := ClaimAlertSlot(staleSend); !ok {
		t.Fatal("second ClaimAlertSlot(staleSend) = false, want true")
	}
	ageAlertClaim(t, staleSend, 3*time.Minute)

	// A send that is still running.
	if ok, _ := ClaimAlertSlot(liveSend); !ok {
		t.Fatal("ClaimAlertSlot(liveSend) = false, want true")
	}
	ageAlertClaim(t, liveSend, 10*time.Second)

	// Posted copies whose close stopped half way, and one whose close is still running.
	mustPost(t, staleClose, 7001)
	mustPost(t, liveClose, 7002)
	for _, id := range []uint{staleClose, liveClose} {
		if ok, _ := ClaimAlertClose(id); !ok {
			t.Fatalf("ClaimAlertClose(%d) = false, want true", id)
		}
	}
	ageAlertClaim(t, staleClose, 3*time.Minute)
	ageAlertClaim(t, liveClose, 10*time.Second)

	released, err := ReleaseStaleAlertClaims(time.Now().Add(-2 * time.Minute))
	if err != nil {
		t.Fatalf("ReleaseStaleAlertClaims error = %v", err)
	}
	if released != 2 {
		t.Errorf("released = %d, want 2", released)
	}

	if row := readAlert(t, staleSend); row.State != models.AlertStatePending || row.ClaimedAt != nil || row.Attempts != 1 || row.Detail != "earlier failure" {
		t.Errorf("stale send = %+v, want pending, unclaimed, attempts still 1 and the earlier detail kept", row)
	}
	if row := readAlert(t, liveSend); row.State != models.AlertStatePosting || row.ClaimedAt == nil {
		t.Errorf("live send = %+v, want it still posting with its claim", row)
	}
	if row := readAlert(t, staleClose); row.State != models.AlertStatePosted || row.ClaimedAt != nil || row.ClosedAt != nil || row.MessageID != 7001 {
		t.Errorf("stale close = %+v, want posted, open, unclaimed, message ID kept", row)
	}
	if row := readAlert(t, liveClose); row.State != models.AlertStatePosted || row.ClaimedAt == nil {
		t.Errorf("live close = %+v, want it still claimed", row)
	}

	if again, err := ReleaseStaleAlertClaims(time.Now().Add(-2 * time.Minute)); err != nil || again != 0 {
		t.Errorf("second ReleaseStaleAlertClaims = %d, %v, want 0 released", again, err)
	}
}

// postedCopies decides the alert of a fresh confirmed lockdown with the given slots and
// posts every one of them, returning the rows oldest first.
func postedCopies(t *testing.T, ld *models.ChatLockdown, slots ...models.ChatLockdownAlert) []models.ChatLockdownAlert {
	t.Helper()
	rows := mustBegin(t, ld, slots...)
	for i, row := range rows {
		mustPost(t, row.ID, int64(900+i))
	}
	return readAlertRows(t, ld.ID)
}

func TestRecordLinkRevoked(t *testing.T) {
	const link = "https://t.me/+FullLink1"

	t.Run("records once and marks the live copies", func(t *testing.T) {
		ld := confirmedLockdown(t)
		rows := mustBegin(t, ld,
			alertSlot(ld, models.AlertKindAlert, 0, models.AlertDestStaff, -100111),
			alertSlot(ld, models.AlertKindAlert, 0, models.AlertDestLog, -100222),
			alertSlot(ld, models.AlertKindReminder, 1, models.AlertDestStaff, -100111),
			alertSlot(ld, models.AlertKindReminder, 2, models.AlertDestStaff, -100111),
		)
		// rows 0 and 1 are posted and open; row 2 stays pending; row 3 is posted and closed.
		mustPost(t, rows[0].ID, 901)
		mustPost(t, rows[1].ID, 902)
		mustPost(t, rows[3].ID, 904)
		if ok, err := ClaimAlertClose(rows[3].ID); err != nil || !ok {
			t.Fatalf("ClaimAlertClose = %v, %v, want true", ok, err)
		}
		if ok, err := RecordAlertClosed(rows[3].ID, ""); err != nil || !ok {
			t.Fatalf("RecordAlertClosed = %v, %v, want true", ok, err)
		}

		won, err := RecordLinkRevoked(ld.ID, 77, "Rev", link)
		if err != nil || !won {
			t.Fatalf("first RecordLinkRevoked = %v, %v, want true", won, err)
		}
		got, err := GetFresh(ld.ID)
		if err != nil || got == nil {
			t.Fatalf("GetFresh = %v, %v", got, err)
		}
		if got.LinkRevokedAt == nil || got.LinkRevokedBy == nil || *got.LinkRevokedBy != 77 ||
			got.LinkRevokedByName != "Rev" || got.LinkRevokedLink != link {
			t.Errorf("revoke record = %+v, want at set, by 77, name Rev, link %s", got, link)
		}
		for i, wantPending := range []bool{true, true, false, false} {
			if gotPending := readAlert(t, rows[i].ID).EditPending; gotPending != wantPending {
				t.Errorf("copy %d edit_pending = %v, want %v", i, gotPending, wantPending)
			}
		}
	})

	t.Run("a second call records nothing", func(t *testing.T) {
		ld := confirmedLockdown(t)
		rows := postedCopies(t, ld, alertSlot(ld, models.AlertKindAlert, 0, models.AlertDestStaff, -100111))
		if won, err := RecordLinkRevoked(ld.ID, 77, "Rev", link); err != nil || !won {
			t.Fatalf("first RecordLinkRevoked = %v, %v, want true", won, err)
		}
		if ok, err := RecordAlertSynced(rows[0].ID, true); err != nil || !ok {
			t.Fatalf("RecordAlertSynced = %v, %v, want true", ok, err)
		}

		won, err := RecordLinkRevoked(ld.ID, 88, "Other", "https://t.me/+Other")
		if err != nil || won {
			t.Fatalf("second RecordLinkRevoked = %v, %v, want false, nil", won, err)
		}
		got, err := GetFresh(ld.ID)
		if err != nil || got == nil || got.LinkRevokedBy == nil || *got.LinkRevokedBy != 77 ||
			got.LinkRevokedLink != link || got.LinkRevokedByName != "Rev" {
			t.Errorf("revoke record after a second call = %+v, want the first one", got)
		}
		if readAlert(t, rows[0].ID).EditPending {
			t.Error("a lost call marked a copy for an edit")
		}
	})

	t.Run("no such lockdown", func(t *testing.T) {
		if won, err := RecordLinkRevoked(1<<40, 77, "Rev", link); err != nil || won {
			t.Errorf("RecordLinkRevoked = %v, %v, want false, nil", won, err)
		}
	})
}

func TestAlertSync(t *testing.T) {
	const link = "https://t.me/+FullLink1"

	t.Run("lists claims and records", func(t *testing.T) {
		ld := confirmedLockdown(t)
		rows := postedCopies(t, ld,
			alertSlot(ld, models.AlertKindAlert, 0, models.AlertDestStaff, -100111),
			alertSlot(ld, models.AlertKindAlert, 0, models.AlertDestLog, -100222),
		)
		listed := func() []uint {
			all, err := ListAlertSlotsToSyncFresh(100)
			if err != nil {
				t.Fatalf("ListAlertSlotsToSyncFresh: %v", err)
			}
			return idsOf(all, ld.ID)
		}
		if got := listed(); len(got) != 0 {
			t.Fatalf("listed %v before any revoke, want none", got)
		}

		if won, err := RecordLinkRevoked(ld.ID, 77, "Rev", link); err != nil || !won {
			t.Fatalf("RecordLinkRevoked = %v, %v, want true", won, err)
		}
		if got, want := listed(), []uint{rows[0].ID, rows[1].ID}; !sameIDs(got, want) {
			t.Fatalf("listed %v, want %v oldest first", got, want)
		}

		if ok, err := ClaimAlertSync(rows[0].ID); err != nil || !ok {
			t.Fatalf("first ClaimAlertSync = %v, %v, want true", ok, err)
		}
		if ok, err := ClaimAlertSync(rows[0].ID); err != nil || ok {
			t.Fatalf("second ClaimAlertSync = %v, %v, want false", ok, err)
		}
		if got, want := listed(), []uint{rows[1].ID}; !sameIDs(got, want) {
			t.Fatalf("listed %v with the first claimed, want %v", got, want)
		}

		// A released claim is listed again and keeps its edit.
		if ok, err := ReleaseAlertClose(rows[0].ID, true); err != nil || !ok {
			t.Fatalf("ReleaseAlertClose = %v, %v, want true", ok, err)
		}
		if got, want := listed(), []uint{rows[0].ID, rows[1].ID}; !sameIDs(got, want) {
			t.Fatalf("listed %v after a release, want %v", got, want)
		}

		if ok, err := ClaimAlertSync(rows[0].ID); err != nil || !ok {
			t.Fatalf("ClaimAlertSync after a release = %v, %v, want true", ok, err)
		}
		if ok, err := RecordAlertSynced(rows[0].ID, true); err != nil || !ok {
			t.Fatalf("RecordAlertSynced = %v, %v, want true", ok, err)
		}
		done := readAlert(t, rows[0].ID)
		if done.EditPending || done.ClaimedAt != nil || done.Attempts != 0 {
			t.Errorf("synced copy = %+v, want edit_pending false, no claim and 0 attempts", done)
		}
		if got, want := listed(), []uint{rows[1].ID}; !sameIDs(got, want) {
			t.Fatalf("listed %v after a sync, want %v", got, want)
		}
	})

	t.Run("a lifted lockdown and a closed copy are not listed", func(t *testing.T) {
		ld := confirmedLockdown(t)
		rows := postedCopies(t, ld,
			alertSlot(ld, models.AlertKindAlert, 0, models.AlertDestStaff, -100111),
			alertSlot(ld, models.AlertKindAlert, 0, models.AlertDestLog, -100222),
		)
		if won, err := RecordLinkRevoked(ld.ID, 77, "Rev", link); err != nil || !won {
			t.Fatalf("RecordLinkRevoked = %v, %v, want true", won, err)
		}
		if ok, err := ClaimAlertClose(rows[0].ID); err != nil || !ok {
			t.Fatalf("ClaimAlertClose = %v, %v, want true", ok, err)
		}
		if ok, err := RecordAlertClosed(rows[0].ID, ""); err != nil || !ok {
			t.Fatalf("RecordAlertClosed = %v, %v, want true", ok, err)
		}
		if closed := readAlert(t, rows[0].ID); closed.EditPending {
			t.Errorf("a closed copy kept edit_pending: %+v", closed)
		}
		mustLift(t, ld.ID)

		all, err := ListAlertSlotsToSyncFresh(100)
		if err != nil {
			t.Fatalf("ListAlertSlotsToSyncFresh: %v", err)
		}
		if got := idsOf(all, ld.ID); len(got) != 0 {
			t.Errorf("listed %v for a lifting lockdown, want none", got)
		}
	})
}

// reminderSlots is one reminder copy per destination for seq.
func reminderSlots(ld *models.ChatLockdown, seq int) []models.ChatLockdownAlert {
	return []models.ChatLockdownAlert{
		alertSlot(ld, models.AlertKindReminder, seq, models.AlertDestStaff, -100111),
		alertSlot(ld, models.AlertKindReminder, seq, models.AlertDestLog, -100222),
	}
}

// reminderRows are the reminder copies of ld, oldest first.
func reminderRows(t *testing.T, ld *models.ChatLockdown) []models.ChatLockdownAlert {
	t.Helper()
	var rows []models.ChatLockdownAlert
	for _, row := range readAlertRows(t, ld.ID) {
		if row.Kind == models.AlertKindReminder {
			rows = append(rows, row)
		}
	}
	return rows
}

func TestReminderSlots(t *testing.T) {
	t.Run("planned once per seq and destination", func(t *testing.T) {
		ld := confirmedLockdown(t)
		mustBegin(t, ld,
			alertSlot(ld, models.AlertKindAlert, 0, models.AlertDestStaff, -100111),
			alertSlot(ld, models.AlertKindAlert, 0, models.AlertDestLog, -100222),
		)
		if planned, err := ReminderPlanned(ld.ID, 1); err != nil || planned {
			t.Fatalf("ReminderPlanned before planning = %v, %v, want false", planned, err)
		}

		inserted, err := PlanReminderSlots(ld.ID, 1, reminderSlots(ld, 99))
		if err != nil || inserted != 2 {
			t.Fatalf("PlanReminderSlots = %d, %v, want 2 rows", inserted, err)
		}
		rows := reminderRows(t, ld)
		if len(rows) != 2 {
			t.Fatalf("%d reminder rows, want 2: %+v", len(rows), rows)
		}
		for _, row := range rows {
			if row.Seq != 1 || row.State != models.AlertStatePending || row.Kind != models.AlertKindReminder {
				t.Errorf("row = %+v, want a pending reminder of seq 1 (the seq argument wins)", row)
			}
		}
		if planned, err := ReminderPlanned(ld.ID, 1); err != nil || !planned {
			t.Errorf("ReminderPlanned(1) = %v, %v, want true", planned, err)
		}
		if planned, err := ReminderPlanned(ld.ID, 2); err != nil || planned {
			t.Errorf("ReminderPlanned(2) = %v, %v, want false", planned, err)
		}

		again, err := PlanReminderSlots(ld.ID, 1, reminderSlots(ld, 1))
		if err != nil || again != 0 {
			t.Errorf("a repeated PlanReminderSlots = %d, %v, want 0 rows", again, err)
		}
		if got := len(reminderRows(t, ld)); got != 2 {
			t.Errorf("%d reminder rows after a repeat, want still 2", got)
		}
	})

	t.Run("a lockdown that is not active gets none", func(t *testing.T) {
		ld := confirmedLockdown(t)
		mustBegin(t, ld, alertSlot(ld, models.AlertKindAlert, 0, models.AlertDestStaff, -100111))
		mustLift(t, ld.ID)

		inserted, err := PlanReminderSlots(ld.ID, 1, reminderSlots(ld, 1))
		if err != nil || inserted != 0 {
			t.Errorf("PlanReminderSlots for a lifting lockdown = %d, %v, want 0 rows", inserted, err)
		}
		if got := len(reminderRows(t, ld)); got != 0 {
			t.Errorf("%d reminder rows for a lifting lockdown, want none", got)
		}
	})

	t.Run("superseded reminders lose their buttons", func(t *testing.T) {
		ld := confirmedLockdown(t)
		rows := mustBegin(t, ld,
			alertSlot(ld, models.AlertKindAlert, 0, models.AlertDestStaff, -100111),
			alertSlot(ld, models.AlertKindAlert, 0, models.AlertDestLog, -100222),
		)
		for i, row := range rows {
			mustPost(t, row.ID, int64(900+i))
		}
		for seq := 1; seq <= 2; seq++ {
			if _, err := PlanReminderSlots(ld.ID, seq, reminderSlots(ld, seq)); err != nil {
				t.Fatalf("PlanReminderSlots(%d): %v", seq, err)
			}
		}
		for i, row := range reminderRows(t, ld) {
			mustPost(t, row.ID, int64(950+i))
		}
		byKey := map[string]models.ChatLockdownAlert{}
		for _, row := range reminderRows(t, ld) {
			byKey[fmt.Sprintf("%s%d", row.Dest, row.Seq)] = row
		}

		if err := SupersedeReminders(ld.ID, models.AlertDestStaff, 2); err != nil {
			t.Fatalf("SupersedeReminders: %v", err)
		}

		first := readAlert(t, byKey[models.AlertDestStaff+"1"].ID)
		if first.ButtonsLive || !first.EditPending {
			t.Errorf("staff reminder 1 = %+v, want buttons_live false and edit_pending true", first)
		}
		for name, id := range map[string]uint{
			"staff reminder 2": byKey[models.AlertDestStaff+"2"].ID,
			"log reminder 1":   byKey[models.AlertDestLog+"1"].ID,
			"log reminder 2":   byKey[models.AlertDestLog+"2"].ID,
			"staff alert":      rows[0].ID,
			"log alert":        rows[1].ID,
		} {
			if row := readAlert(t, id); !row.ButtonsLive || row.EditPending {
				t.Errorf("%s = %+v, want it left alone: buttons live, no edit pending", name, row)
			}
		}

		if err := SupersedeReminders(ld.ID, models.AlertDestStaff, 2); err != nil {
			t.Fatalf("a repeated SupersedeReminders: %v", err)
		}
	})

	t.Run("listing and destinations", func(t *testing.T) {
		ld := confirmedLockdown(t)
		other := confirmedLockdown(t)
		if kinds, err := AlertDestKinds(ld.ID); err != nil || len(kinds) != 0 {
			t.Fatalf("AlertDestKinds before the alert = %v, %v, want none", kinds, err)
		}
		listed := func(want *models.ChatLockdown) bool {
			all, err := ListRemindableFresh(1000)
			if err != nil {
				t.Fatalf("ListRemindableFresh: %v", err)
			}
			for _, row := range all {
				if row.ID == want.ID {
					return true
				}
			}
			return false
		}
		if listed(ld) {
			t.Fatal("ListRemindableFresh lists a lockdown whose alert is not decided")
		}

		mustBegin(t, ld,
			alertSlot(ld, models.AlertKindAlert, 0, models.AlertDestStaff, -100111),
			alertSlot(ld, models.AlertKindAlert, 0, models.AlertDestLog, -100222),
		)
		if !listed(ld) {
			t.Fatal("ListRemindableFresh does not list a confirmed lockdown with its alert decided")
		}
		if listed(other) {
			t.Error("ListRemindableFresh lists another lockdown whose alert is not decided")
		}
		kinds, err := AlertDestKinds(ld.ID)
		if err != nil || len(kinds) != 2 || kinds[0] != models.AlertDestLog || kinds[1] != models.AlertDestStaff {
			t.Errorf("AlertDestKinds = %v, %v, want log and staff once each", kinds, err)
		}

		// A reminder does not add a destination.
		if _, err := PlanReminderSlots(ld.ID, 1, reminderSlots(ld, 1)); err != nil {
			t.Fatalf("PlanReminderSlots: %v", err)
		}
		if kinds, err := AlertDestKinds(ld.ID); err != nil || len(kinds) != 2 {
			t.Errorf("AlertDestKinds after a reminder = %v, %v, want still 2", kinds, err)
		}

		mustLift(t, ld.ID)
		if listed(ld) {
			t.Error("ListRemindableFresh lists a lifting lockdown")
		}
	})
}

func TestAlertSyncKeepsAMarkSetDuringTheEdit(t *testing.T) {
	ld := confirmedLockdown(t)
	rows := postedCopies(t, ld, alertSlot(ld, models.AlertKindReminder, 1, models.AlertDestStaff, -100111))
	if err := SupersedeReminders(ld.ID, models.AlertDestStaff, 2); err != nil {
		t.Fatalf("SupersedeReminders: %v", err)
	}
	// The mark was set (buttons now off) and a refresh claims the copy.
	if ok, err := ClaimAlertSync(rows[0].ID); err != nil || !ok {
		t.Fatalf("ClaimAlertSync = %v, %v, want true", ok, err)
	}

	// A refresh that edited the copy while its buttons were still on must not clear the mark.
	if ok, err := RecordAlertSynced(rows[0].ID, true); err != nil || !ok {
		t.Fatalf("RecordAlertSynced(live) = %v, %v, want the claim released", ok, err)
	}
	kept := readAlert(t, rows[0].ID)
	if !kept.EditPending || kept.ClaimedAt != nil || kept.Attempts != 0 {
		t.Fatalf("copy = %+v, want edit_pending kept, no claim and 0 attempts", kept)
	}

	// The refresh that edited it with its buttons off clears the mark.
	if ok, err := ClaimAlertSync(rows[0].ID); err != nil || !ok {
		t.Fatalf("second ClaimAlertSync = %v, %v, want true", ok, err)
	}
	if ok, err := RecordAlertSynced(rows[0].ID, false); err != nil || !ok {
		t.Fatalf("RecordAlertSynced(off) = %v, %v, want true", ok, err)
	}
	if done := readAlert(t, rows[0].ID); done.EditPending || done.ClaimedAt != nil {
		t.Errorf("copy = %+v, want edit_pending cleared", done)
	}
}
