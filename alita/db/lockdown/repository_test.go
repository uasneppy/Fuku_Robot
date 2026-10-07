//go:build testtools

package lockdown

import (
	"crypto/rand"
	"encoding/binary"
	"sync"
	"testing"
	"time"

	"github.com/divkix/Alita_Robot/alita/db"
	"github.com/divkix/Alita_Robot/alita/db/models"
)

// uniqueLockdownChatID returns a random negative chat ID, so tests sharing one
// PostgreSQL database never meet each other's rows.
func uniqueLockdownChatID(t *testing.T) int64 {
	t.Helper()
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		t.Fatalf("crypto/rand: %v", err)
	}
	return -1_000_000_000_000 - int64(binary.BigEndian.Uint64(buf[:])&0x3fffffffffffffff)
}

// cleanupLockdowns deletes the lockdown rows of the given chats when the test ends,
// joiners first.
func cleanupLockdowns(t *testing.T, chatIDs ...int64) {
	t.Helper()
	t.Cleanup(func() {
		db.DB.Where("chat_id IN ?", chatIDs).Delete(&models.LockdownJoiner{})
		db.DB.Where("chat_id IN ?", chatIDs).Delete(&models.ChatLockdown{})
	})
}

func newLockdownRow(chatID int64) *models.ChatLockdown {
	return &models.ChatLockdown{
		ChatID:            chatID,
		TriggerKind:       models.LockdownTriggerManual,
		StartedBy:         42,
		StartedByName:     "Admin",
		PrePermissions:    `{"can_send_messages":true}`,
		LockedPermissions: `{"can_send_messages":false}`,
	}
}

func mustStart(t *testing.T, chatID int64) *models.ChatLockdown {
	t.Helper()
	row := newLockdownRow(chatID)
	started, err := Start(row)
	if err != nil || !started {
		t.Fatalf("Start = %v, %v, want true, nil", started, err)
	}
	if row.ID == 0 {
		t.Fatal("Start left the row ID unset")
	}
	return row
}

// TestStartLockdownOneActivePerChat proves at most one active lockdown exists per
// chat. It runs on SQLite by default and on PostgreSQL with the migration chain
// applied when DATABASE_URL and ALITA_TEST_DATABASE=true are set.
func TestStartLockdownOneActivePerChat(t *testing.T) {
	t.Logf("lockdown repository backend: %s", db.DB.Name())

	t.Run("a second start is refused", func(t *testing.T) {
		chat := uniqueLockdownChatID(t)
		cleanupLockdowns(t, chat)

		mustStart(t, chat)
		started, err := Start(newLockdownRow(chat))
		if err != nil {
			t.Fatalf("second Start error = %v, want none", err)
		}
		if started {
			t.Error("second Start = true, want false while the first is active")
		}
		var rows int64
		if err := db.DB.Model(&models.ChatLockdown{}).Where("chat_id = ?", chat).Count(&rows).Error; err != nil {
			t.Fatalf("count rows: %v", err)
		}
		if rows != 1 {
			t.Errorf("rows for the chat = %d, want 1", rows)
		}
	})

	t.Run("eight concurrent starts produce one active row", func(t *testing.T) {
		chat := uniqueLockdownChatID(t)
		cleanupLockdowns(t, chat)

		const callers = 8
		var wg sync.WaitGroup
		results := make([]bool, callers)
		errs := make([]error, callers)
		for i := 0; i < callers; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				results[i], errs[i] = Start(newLockdownRow(chat))
			}(i)
		}
		wg.Wait()

		winners := 0
		for i := range results {
			if errs[i] != nil {
				t.Errorf("Start #%d error = %v", i, errs[i])
			}
			if results[i] {
				winners++
			}
		}
		if winners != 1 {
			t.Errorf("Start winners = %d, want exactly 1", winners)
		}
		var active int64
		if err := db.DB.Model(&models.ChatLockdown{}).
			Where("chat_id = ? AND state = ?", chat, models.LockdownStateActive).Count(&active).Error; err != nil {
			t.Fatalf("count active rows: %v", err)
		}
		if active != 1 {
			t.Errorf("active rows = %d, want 1", active)
		}
	})

	t.Run("the index refuses a raw duplicate active insert", func(t *testing.T) {
		chat := uniqueLockdownChatID(t)
		cleanupLockdowns(t, chat)

		mustStart(t, chat)
		duplicate := newLockdownRow(chat)
		duplicate.State = models.LockdownStateActive
		if err := db.DB.Create(duplicate).Error; err == nil {
			t.Error("a raw second active insert succeeded, want uk_chat_lockdowns_active to refuse it")
		}
	})

	t.Run("a new lockdown can start after a lift", func(t *testing.T) {
		chat := uniqueLockdownChatID(t)
		cleanupLockdowns(t, chat)

		first := mustStart(t, chat)
		if began, err := BeginLift(first.ID, 42, "Admin", false); err != nil || !began {
			t.Fatalf("BeginLift = %v, %v", began, err)
		}
		if finished, err := FinishLift(first.ID); err != nil || !finished {
			t.Fatalf("FinishLift = %v, %v", finished, err)
		}
		second := mustStart(t, chat)
		if second.ID == first.ID {
			t.Error("the second lockdown reused the first row")
		}
	})

	t.Run("two chats are independent", func(t *testing.T) {
		chatA := uniqueLockdownChatID(t)
		chatB := uniqueLockdownChatID(t)
		cleanupLockdowns(t, chatA, chatB)

		mustStart(t, chatA)
		mustStart(t, chatB)
		if row, err := GetActiveFresh(chatA); err != nil || row == nil || row.ChatID != chatA {
			t.Errorf("GetActiveFresh(A) = %v, %v", row, err)
		}
		if row, err := GetActiveFresh(chatB); err != nil || row == nil || row.ChatID != chatB {
			t.Errorf("GetActiveFresh(B) = %v, %v", row, err)
		}
	})
}

// mustJoiner writes one joiner row of a lockdown directly.
func mustJoiner(t *testing.T, lockdownID uint, chatID, userID int64, state string) {
	t.Helper()
	joiner := &models.LockdownJoiner{
		LockdownID: lockdownID,
		ChatID:     chatID,
		UserID:     userID,
		FirstName:  "Joiner",
		State:      state,
	}
	if err := db.DB.Create(joiner).Error; err != nil {
		t.Fatalf("create joiner %d in state %s: %v", userID, state, err)
	}
}

func TestLockdownCurrentAndTally(t *testing.T) {
	t.Run("a chat that was never locked has no current lockdown", func(t *testing.T) {
		row, err := GetCurrentFresh(uniqueLockdownChatID(t))
		if err != nil || row != nil {
			t.Errorf("GetCurrentFresh = %v, %v, want nil, nil", row, err)
		}
	})

	t.Run("the active row is current", func(t *testing.T) {
		chat := uniqueLockdownChatID(t)
		cleanupLockdowns(t, chat)
		active := mustStart(t, chat)

		row, err := GetCurrentFresh(chat)
		if err != nil || row == nil || row.ID != active.ID {
			t.Fatalf("GetCurrentFresh = %v, %v, want the active row %d", row, err, active.ID)
		}
		if row.State != models.LockdownStateActive {
			t.Errorf("State = %q, want active", row.State)
		}
	})

	t.Run("the active row wins over a lifting one", func(t *testing.T) {
		chat := uniqueLockdownChatID(t)
		cleanupLockdowns(t, chat)
		first := mustStart(t, chat)
		if began, err := BeginLift(first.ID, 7, "Lifter", false); err != nil || !began {
			t.Fatalf("BeginLift = %v, %v", began, err)
		}
		second := mustStart(t, chat)

		row, err := GetCurrentFresh(chat)
		if err != nil || row == nil || row.ID != second.ID {
			t.Fatalf("GetCurrentFresh = %v, %v, want the active row %d", row, err, second.ID)
		}
	})

	t.Run("without an active row the newest lifting row is current and lifted rows are ignored", func(t *testing.T) {
		chat := uniqueLockdownChatID(t)
		cleanupLockdowns(t, chat)
		older := mustStart(t, chat)
		if began, err := BeginLift(older.ID, 7, "Lifter", false); err != nil || !began {
			t.Fatalf("BeginLift older = %v, %v", began, err)
		}
		newer := mustStart(t, chat)
		if began, err := BeginLift(newer.ID, 8, "Lifter", false); err != nil || !began {
			t.Fatalf("BeginLift newer = %v, %v", began, err)
		}
		done := mustStart(t, chat)
		if began, err := BeginLift(done.ID, 9, "Lifter", false); err != nil || !began {
			t.Fatalf("BeginLift done = %v, %v", began, err)
		}
		if finished, err := FinishLift(done.ID); err != nil || !finished {
			t.Fatalf("FinishLift = %v, %v", finished, err)
		}

		row, err := GetCurrentFresh(chat)
		if err != nil || row == nil || row.ID != newer.ID {
			t.Fatalf("GetCurrentFresh = %v, %v, want the newest lifting row %d", row, err, newer.ID)
		}
	})

	t.Run("only lifted rows leave no current lockdown", func(t *testing.T) {
		chat := uniqueLockdownChatID(t)
		cleanupLockdowns(t, chat)
		done := mustStart(t, chat)
		if began, err := BeginLift(done.ID, 9, "Lifter", false); err != nil || !began {
			t.Fatalf("BeginLift = %v, %v", began, err)
		}
		if finished, err := FinishLift(done.ID); err != nil || !finished {
			t.Fatalf("FinishLift = %v, %v", finished, err)
		}

		row, err := GetCurrentFresh(chat)
		if err != nil || row != nil {
			t.Errorf("GetCurrentFresh = %v, %v, want nil, nil", row, err)
		}
	})

	t.Run("tally counts one lockdown by state", func(t *testing.T) {
		chat := uniqueLockdownChatID(t)
		cleanupLockdowns(t, chat)
		first := mustStart(t, chat)
		if began, err := BeginLift(first.ID, 7, "Lifter", false); err != nil || !began {
			t.Fatalf("BeginLift = %v, %v", began, err)
		}
		second := mustStart(t, chat)

		mustJoiner(t, second.ID, chat, 601, models.JoinerStateBanned)
		mustJoiner(t, second.ID, chat, 602, models.JoinerStateBanned)
		mustJoiner(t, second.ID, chat, 603, models.JoinerStateBanFailed)
		mustJoiner(t, second.ID, chat, 604, models.JoinerStateDeclined)
		mustJoiner(t, first.ID, chat, 601, models.JoinerStateBanned)
		mustJoiner(t, first.ID, chat, 605, models.JoinerStateUnbanned)

		got, err := TallyJoiners(second.ID)
		if err != nil {
			t.Fatalf("TallyJoiners error = %v", err)
		}
		want := map[string]int64{
			models.JoinerStateBanned:    2,
			models.JoinerStateBanFailed: 1,
			models.JoinerStateDeclined:  1,
		}
		if len(got) != len(want) {
			t.Errorf("TallyJoiners = %v, want %v", got, want)
		}
		for state, count := range want {
			if got[state] != count {
				t.Errorf("TallyJoiners[%s] = %d, want %d", state, got[state], count)
			}
		}

		empty, err := TallyJoiners(first.ID + 100000)
		if err != nil || len(empty) != 0 {
			t.Errorf("TallyJoiners of an unknown lockdown = %v, %v, want an empty map", empty, err)
		}
	})
}

func TestLockdownRepositoryTransitions(t *testing.T) {
	t.Run("confirm happens once", func(t *testing.T) {
		chat := uniqueLockdownChatID(t)
		cleanupLockdowns(t, chat)
		row := mustStart(t, chat)

		if got, err := ConfirmLocked(row.ID); err != nil || !got {
			t.Fatalf("first ConfirmLocked = %v, %v, want true", got, err)
		}
		if got, err := ConfirmLocked(row.ID); err != nil || got {
			t.Errorf("second ConfirmLocked = %v, %v, want false", got, err)
		}
		fresh, err := GetFresh(row.ID)
		if err != nil || fresh == nil || fresh.LockedAt == nil {
			t.Errorf("GetFresh = %v, %v, want LockedAt set", fresh, err)
		}
	})

	t.Run("delete refuses a confirmed row and removes an unconfirmed one", func(t *testing.T) {
		confirmedChat := uniqueLockdownChatID(t)
		unconfirmedChat := uniqueLockdownChatID(t)
		cleanupLockdowns(t, confirmedChat, unconfirmedChat)

		confirmed := mustStart(t, confirmedChat)
		if _, err := ConfirmLocked(confirmed.ID); err != nil {
			t.Fatalf("ConfirmLocked error = %v", err)
		}
		if deleted, err := DeleteUnconfirmed(confirmed.ID); err != nil || deleted {
			t.Errorf("DeleteUnconfirmed(confirmed) = %v, %v, want false", deleted, err)
		}
		if row, err := GetFresh(confirmed.ID); err != nil || row == nil {
			t.Errorf("the confirmed row is gone: %v, %v", row, err)
		}

		unconfirmed := mustStart(t, unconfirmedChat)
		if deleted, err := DeleteUnconfirmed(unconfirmed.ID); err != nil || !deleted {
			t.Errorf("DeleteUnconfirmed(unconfirmed) = %v, %v, want true", deleted, err)
		}
		if row, err := GetFresh(unconfirmed.ID); err != nil || row != nil {
			t.Errorf("GetFresh after delete = %v, %v, want nil, nil", row, err)
		}
	})

	t.Run("begin lift happens once and records who", func(t *testing.T) {
		chat := uniqueLockdownChatID(t)
		cleanupLockdowns(t, chat)
		row := mustStart(t, chat)

		if got, err := BeginLift(row.ID, 7, "Lifter", true); err != nil || !got {
			t.Fatalf("first BeginLift = %v, %v, want true", got, err)
		}
		if got, err := BeginLift(row.ID, 8, "Other", false); err != nil || got {
			t.Errorf("second BeginLift = %v, %v, want false", got, err)
		}
		fresh, err := GetFresh(row.ID)
		if err != nil || fresh == nil {
			t.Fatalf("GetFresh = %v, %v", fresh, err)
		}
		if fresh.State != models.LockdownStateLifting || fresh.LiftedBy == nil || *fresh.LiftedBy != 7 ||
			fresh.LiftedByName != "Lifter" || !fresh.ManualChange || fresh.LiftStartedAt == nil {
			t.Errorf("row after BeginLift = %+v", fresh)
		}
		if active, err := GetActiveFresh(chat); err != nil || active != nil {
			t.Errorf("GetActiveFresh after BeginLift = %v, %v, want nil, nil", active, err)
		}
	})

	t.Run("finish lift waits for the joiners", func(t *testing.T) {
		chat := uniqueLockdownChatID(t)
		cleanupLockdowns(t, chat)
		row := mustStart(t, chat)

		if got, err := FinishLift(row.ID); err != nil || got {
			t.Errorf("FinishLift on an active row = %v, %v, want false", got, err)
		}
		joiner := &models.LockdownJoiner{
			LockdownID: row.ID,
			ChatID:     chat,
			UserID:     501,
			FirstName:  "Joiner",
			State:      models.JoinerStatePending,
		}
		if err := db.DB.Create(joiner).Error; err != nil {
			t.Fatalf("create joiner: %v", err)
		}
		if got, err := BeginLift(row.ID, 7, "Lifter", false); err != nil || !got {
			t.Fatalf("BeginLift = %v, %v", got, err)
		}
		if got, err := FinishLift(row.ID); err != nil || got {
			t.Errorf("FinishLift with a pending joiner = %v, %v, want false", got, err)
		}

		if err := db.DB.Model(&models.LockdownJoiner{}).Where("id = ?", joiner.ID).
			Update("state", models.JoinerStateCancelled).Error; err != nil {
			t.Fatalf("cancel joiner: %v", err)
		}
		if got, err := FinishLift(row.ID); err != nil || !got {
			t.Fatalf("FinishLift after the joiner was cancelled = %v, %v, want true", got, err)
		}
		fresh, err := GetFresh(row.ID)
		if err != nil || fresh == nil || fresh.State != models.LockdownStateLifted || fresh.LiftedAt == nil {
			t.Errorf("row after FinishLift = %+v, %v", fresh, err)
		}
	})

	t.Run("an unknown chat has no active lockdown", func(t *testing.T) {
		row, err := GetActiveFresh(uniqueLockdownChatID(t))
		if err != nil || row != nil {
			t.Errorf("GetActiveFresh = %v, %v, want nil, nil", row, err)
		}
	})
}

// ageRow moves created_at and updated_at of a lockdown row back by the given time.
func ageRow(t *testing.T, id uint, by time.Duration) {
	t.Helper()
	at := time.Now().UTC().Add(-by)
	err := db.DB.Model(&models.ChatLockdown{}).Where("id = ?", id).
		UpdateColumns(map[string]any{"created_at": at, "updated_at": at}).Error
	if err != nil {
		t.Fatalf("age lockdown %d: %v", id, err)
	}
}

func TestListUnconfirmed(t *testing.T) {
	oldChat := uniqueLockdownChatID(t)
	youngChat := uniqueLockdownChatID(t)
	confirmedChat := uniqueLockdownChatID(t)
	liftingChat := uniqueLockdownChatID(t)
	cleanupLockdowns(t, oldChat, youngChat, confirmedChat, liftingChat)

	old := mustStart(t, oldChat)
	ageRow(t, old.ID, 5*time.Minute)
	young := mustStart(t, youngChat)
	ageRow(t, young.ID, 10*time.Second)
	confirmed := mustStart(t, confirmedChat)
	if ok, err := ConfirmLocked(confirmed.ID); err != nil || !ok {
		t.Fatalf("ConfirmLocked = %v, %v, want true, nil", ok, err)
	}
	ageRow(t, confirmed.ID, 5*time.Minute)
	liftingRow := mustStart(t, liftingChat)
	if ok, err := BeginLift(liftingRow.ID, 7, "Lifter", false); err != nil || !ok {
		t.Fatalf("BeginLift = %v, %v, want true, nil", ok, err)
	}
	ageRow(t, liftingRow.ID, 5*time.Minute)

	rows, err := ListUnconfirmedFresh(time.Now().UTC().Add(-time.Minute))
	if err != nil {
		t.Fatalf("ListUnconfirmedFresh error = %v, want none", err)
	}
	var got []uint
	for _, row := range rows {
		if row.ChatID == oldChat || row.ChatID == youngChat || row.ChatID == confirmedChat || row.ChatID == liftingChat {
			got = append(got, row.ID)
		}
	}
	if len(got) != 1 || got[0] != old.ID {
		t.Errorf("unconfirmed rows = %v, want only the active, unconfirmed row older than the cut-off (%d)", got, old.ID)
	}

	if err := TouchLockdown(old.ID); err != nil {
		t.Fatalf("TouchLockdown error = %v, want none", err)
	}
	touched, err := GetFresh(old.ID)
	if err != nil || touched == nil {
		t.Fatalf("GetFresh = %v, %v, want the row", touched, err)
	}
	if time.Since(touched.UpdatedAt) > time.Minute {
		t.Errorf("updated_at = %v after TouchLockdown, want about now", touched.UpdatedAt)
	}
	if touched.LockedAt != nil || touched.State != models.LockdownStateActive {
		t.Errorf("row = %+v after TouchLockdown, want only updated_at changed", touched)
	}
	rows, err = ListUnconfirmedFresh(time.Now().UTC().Add(-time.Minute))
	if err != nil {
		t.Fatalf("ListUnconfirmedFresh error = %v, want none", err)
	}
	for _, row := range rows {
		if row.ID == old.ID {
			t.Error("a touched row is listed again at once, want it left alone for another grace period")
		}
	}
}
