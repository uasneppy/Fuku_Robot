//go:build testtools

package lockdown

import (
	"testing"

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
