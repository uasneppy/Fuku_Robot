//go:build testtools

package lockdown

import (
	"testing"
	"time"

	"github.com/divkix/Alita_Robot/alita/db"
	"github.com/divkix/Alita_Robot/alita/db/models"
)

// cleanupBurstLockdowns deletes the burst rows of the given chats, and their lockdown
// and alert rows, when the test ends.
func cleanupBurstLockdowns(t *testing.T, chatIDs ...int64) {
	t.Helper()
	cleanupAlertLockdowns(t, chatIDs...)
	t.Cleanup(func() {
		db.DB.Where("chat_id IN ?", chatIDs).Delete(&models.ChatLockdownBurst{})
	})
}

// preRow is a pre-lockdown burst row of ld for userID that joined at the given time.
func preRow(ld *models.ChatLockdown, userID int64, name string, joinedAt time.Time) models.ChatLockdownBurst {
	return models.ChatLockdownBurst{
		LockdownID: ld.ID,
		ChatID:     ld.ChatID,
		UserID:     userID,
		FirstName:  name,
		Username:   "u" + name,
		InviteLink: "https://t.me/+" + name,
		Source:     models.BurstSourcePre,
		JoinedAt:   &joinedAt,
	}
}

func readBurstRows(t *testing.T, lockdownID uint) []models.ChatLockdownBurst {
	t.Helper()
	var rows []models.ChatLockdownBurst
	if err := db.DB.Where("lockdown_id = ?", lockdownID).Order("id").Find(&rows).Error; err != nil {
		t.Fatalf("read burst rows: %v", err)
	}
	return rows
}

func accountIDs(accounts []BurstAccount) []int64 {
	ids := make([]int64, 0, len(accounts))
	for _, account := range accounts {
		ids = append(ids, account.UserID)
	}
	return ids
}

func sameInt64s(got, want []int64) bool {
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

func TestBurstSample(t *testing.T) {
	const a, b, c, d = int64(9001), int64(9002), int64(9003), int64(9004)

	// sampleLockdown decides the alert of a fresh lockdown with pre rows B and A (stored
	// in that order, A having joined first), then records joiner rows C, B and D.
	sampleLockdown := func(t *testing.T) *models.ChatLockdown {
		t.Helper()
		chat := uniqueLockdownChatID(t)
		cleanupBurstLockdowns(t, chat)
		ld := mustStart(t, chat)
		mustConfirm(t, ld.ID)

		base := time.Now().UTC().Add(-5 * time.Minute)
		plan := AlertPlan{Burst: []models.ChatLockdownBurst{
			preRow(ld, b, "B", base.Add(time.Minute)),
			preRow(ld, a, "A", base),
		}}
		if won, err := BeginAlert(ld.ID, plan); err != nil || !won {
			t.Fatalf("BeginAlert = %v, %v, want true", won, err)
		}
		for _, userID := range []int64{c, b, d} {
			mustRecord(t, newJoinRecord(ld, userID))
		}
		return ld
	}

	t.Run("pre rows then joiner rows", func(t *testing.T) {
		ld := sampleLockdown(t)

		accounts, counts, err := BurstSampleFresh(ld.ID, 10)
		if err != nil {
			t.Fatalf("BurstSampleFresh: %v", err)
		}
		if got, want := accountIDs(accounts), []int64{a, b, c, d}; !sameInt64s(got, want) {
			t.Errorf("accounts = %v, want %v (pre rows by join time, then joiner rows, B once)", got, want)
		}
		if counts.Before != 2 || counts.Since != 2 {
			t.Errorf("counts = %+v, want Before 2 and Since 2", counts)
		}
		if !accounts[0].Pre || !accounts[1].Pre || accounts[2].Pre || accounts[3].Pre {
			t.Errorf("Pre flags = %v %v %v %v, want true true false false",
				accounts[0].Pre, accounts[1].Pre, accounts[2].Pre, accounts[3].Pre)
		}
		if accounts[0].FirstName != "A" || accounts[0].Username != "uA" || accounts[0].InviteLink != "https://t.me/+A" {
			t.Errorf("account A = %+v, want the stored name, username and link", accounts[0])
		}
		if accounts[2].FirstName != "Raider" || accounts[2].Username != "raider" {
			t.Errorf("account C = %+v, want the joiner row's name and username", accounts[2])
		}
	})

	t.Run("limit keeps the counts", func(t *testing.T) {
		ld := sampleLockdown(t)

		accounts, counts, err := BurstSampleFresh(ld.ID, 2)
		if err != nil {
			t.Fatalf("BurstSampleFresh: %v", err)
		}
		if got, want := accountIDs(accounts), []int64{a, b}; !sameInt64s(got, want) {
			t.Errorf("accounts = %v, want %v", got, want)
		}
		if counts.Before != 2 || counts.Since != 2 {
			t.Errorf("counts = %+v, want the full Before 2 and Since 2", counts)
		}

		accounts, counts, err = BurstSampleFresh(ld.ID, 3)
		if err != nil {
			t.Fatalf("BurstSampleFresh: %v", err)
		}
		if got, want := accountIDs(accounts), []int64{a, b, c}; !sameInt64s(got, want) {
			t.Errorf("accounts with limit 3 = %v, want %v", got, want)
		}
		if counts.Before != 2 || counts.Since != 2 {
			t.Errorf("counts with limit 3 = %+v, want Before 2 and Since 2", counts)
		}

		accounts, counts, err = BurstSampleFresh(ld.ID, 0)
		if err != nil || len(accounts) != 0 || counts.Before != 2 || counts.Since != 2 {
			t.Errorf("limit 0 = %v, %+v, %v, want no accounts and the counts", accounts, counts, err)
		}
	})

	t.Run("no rows", func(t *testing.T) {
		chat := uniqueLockdownChatID(t)
		cleanupBurstLockdowns(t, chat)
		ld := mustStart(t, chat)
		mustConfirm(t, ld.ID)

		accounts, counts, err := BurstSampleFresh(ld.ID, 10)
		if err != nil || len(accounts) != 0 || counts.Before != 0 || counts.Since != 0 {
			t.Errorf("BurstSampleFresh = %v, %+v, %v, want nothing", accounts, counts, err)
		}
	})

	t.Run("another lockdown's rows are not read", func(t *testing.T) {
		ld := sampleLockdown(t)
		other := sampleLockdown(t)

		accounts, counts, err := BurstSampleFresh(other.ID, 10)
		if err != nil {
			t.Fatalf("BurstSampleFresh: %v", err)
		}
		if len(accounts) != 4 || counts.Before != 2 || counts.Since != 2 {
			t.Errorf("other lockdown read %d accounts with %+v, want its own 4 with Before 2 and Since 2", len(accounts), counts)
		}
		for _, row := range readBurstRows(t, ld.ID) {
			if row.LockdownID != ld.ID || row.ChatID != ld.ChatID {
				t.Errorf("burst row %+v belongs to another lockdown", row)
			}
		}
	})
}

func TestBeginAlertBurst(t *testing.T) {
	t.Run("the winner inserts the burst", func(t *testing.T) {
		chat := uniqueLockdownChatID(t)
		cleanupBurstLockdowns(t, chat)
		ld := mustStart(t, chat)
		mustConfirm(t, ld.ID)

		joined := time.Now().UTC().Add(-time.Minute)
		row := preRow(ld, 9101, "P", joined)
		row.IsBot = true
		plan := AlertPlan{Burst: []models.ChatLockdownBurst{row}}
		if won, err := BeginAlert(ld.ID, plan); err != nil || !won {
			t.Fatalf("BeginAlert = %v, %v, want true", won, err)
		}

		rows := readBurstRows(t, ld.ID)
		if len(rows) != 1 {
			t.Fatalf("%d burst rows, want 1", len(rows))
		}
		got := rows[0]
		if got.UserID != 9101 || got.Source != models.BurstSourcePre || got.InviteLink != "https://t.me/+P" || !got.IsBot {
			t.Errorf("burst row = %+v, want user 9101, source pre, its link and the bot flag", got)
		}
		if got.State != models.BurstStateNone || got.Attempts != 0 || got.ClaimedAt != nil || got.RunID != nil {
			t.Errorf("burst row = %+v, want no response state yet", got)
		}
		if got.JoinedAt == nil || got.JoinedAt.Sub(joined).Abs() > time.Second {
			t.Errorf("joined_at = %v, want %v", got.JoinedAt, joined)
		}
	})

	t.Run("the loser inserts nothing", func(t *testing.T) {
		chat := uniqueLockdownChatID(t)
		cleanupBurstLockdowns(t, chat)
		ld := mustStart(t, chat)
		mustConfirm(t, ld.ID)

		first := AlertPlan{Burst: []models.ChatLockdownBurst{preRow(ld, 9201, "First", time.Now())}}
		if won, err := BeginAlert(ld.ID, first); err != nil || !won {
			t.Fatalf("first BeginAlert = %v, %v, want true", won, err)
		}
		second := AlertPlan{Burst: []models.ChatLockdownBurst{preRow(ld, 9202, "Second", time.Now())}}
		if won, err := BeginAlert(ld.ID, second); err != nil || won {
			t.Fatalf("second BeginAlert = %v, %v, want false", won, err)
		}

		rows := readBurstRows(t, ld.ID)
		if len(rows) != 1 || rows[0].UserID != 9201 {
			t.Errorf("burst rows = %+v, want only the winner's row for 9201", rows)
		}
	})

	t.Run("an empty burst inserts nothing", func(t *testing.T) {
		chat := uniqueLockdownChatID(t)
		cleanupBurstLockdowns(t, chat)
		ld := mustStart(t, chat)
		mustConfirm(t, ld.ID)

		if won, err := BeginAlert(ld.ID, AlertPlan{}); err != nil || !won {
			t.Fatalf("BeginAlert = %v, %v, want true", won, err)
		}
		if rows := readBurstRows(t, ld.ID); len(rows) != 0 {
			t.Errorf("%d burst rows, want none", len(rows))
		}
	})

	t.Run("a repeated user is stored once", func(t *testing.T) {
		chat := uniqueLockdownChatID(t)
		cleanupBurstLockdowns(t, chat)
		ld := mustStart(t, chat)
		mustConfirm(t, ld.ID)

		now := time.Now().UTC()
		plan := AlertPlan{Burst: []models.ChatLockdownBurst{
			preRow(ld, 9301, "Dup", now.Add(-2*time.Minute)),
			preRow(ld, 9301, "Dup", now.Add(-time.Minute)),
		}}
		if won, err := BeginAlert(ld.ID, plan); err != nil || !won {
			t.Fatalf("BeginAlert = %v, %v, want true", won, err)
		}
		if rows := readBurstRows(t, ld.ID); len(rows) != 1 {
			t.Errorf("%d burst rows for one user, want 1", len(rows))
		}
	})
}

// queuedBurstRow writes a queued burst row of ld for userID and returns it.
func queuedBurstRow(t *testing.T, ld *models.ChatLockdown, userID int64) models.ChatLockdownBurst {
	t.Helper()
	row := preRow(ld, userID, "Q", time.Now().UTC().Add(-time.Minute))
	row.State = models.BurstStateQueued
	if err := db.DB.Create(&row).Error; err != nil {
		t.Fatalf("seed burst row %d: %v", userID, err)
	}
	return row
}

func readBurst(t *testing.T, id uint) models.ChatLockdownBurst {
	t.Helper()
	var row models.ChatLockdownBurst
	if err := db.DB.First(&row, id).Error; err != nil {
		t.Fatalf("read burst row %d: %v", id, err)
	}
	return row
}

func TestBurstClaims(t *testing.T) {
	startLockdown := func(t *testing.T) *models.ChatLockdown {
		t.Helper()
		chat := uniqueLockdownChatID(t)
		cleanupBurstLockdowns(t, chat)
		ld := mustStart(t, chat)
		mustConfirm(t, ld.ID)
		return ld
	}

	t.Run("ClaimBurst moves queued to acting once", func(t *testing.T) {
		ld := startLockdown(t)
		row := queuedBurstRow(t, ld, 9201)

		won, err := ClaimBurst(row.ID, models.BurstStateQueued, models.BurstStateActing)
		if err != nil || !won {
			t.Fatalf("first ClaimBurst = %v, %v, want true", won, err)
		}
		got := readBurst(t, row.ID)
		if got.State != models.BurstStateActing || got.ClaimedAt == nil {
			t.Errorf("row = %q, claim %v, want acting with claimed_at set", got.State, got.ClaimedAt)
		}
		if won, err := ClaimBurst(row.ID, models.BurstStateQueued, models.BurstStateActing); err != nil || won {
			t.Errorf("second ClaimBurst = %v, %v, want false: the row is no longer queued", won, err)
		}
	})

	t.Run("MoveBurst is conditional on the from-state", func(t *testing.T) {
		ld := startLockdown(t)
		row := queuedBurstRow(t, ld, 9202)
		if won, err := ClaimBurst(row.ID, models.BurstStateQueued, models.BurstStateActing); err != nil || !won {
			t.Fatalf("setup: ClaimBurst = %v, %v", won, err)
		}

		if moved, err := MoveBurst(row.ID, models.BurstStateQueued, models.BurstStateBanned, "", "", false); err != nil || moved {
			t.Errorf("MoveBurst from the wrong state = %v, %v, want false", moved, err)
		}
		if got := readBurst(t, row.ID); got.State != models.BurstStateActing {
			t.Errorf("row state = %q after a refused move, want acting", got.State)
		}

		moved, err := MoveBurst(row.ID, models.BurstStateActing, models.BurstStateSkipped, "admin", "", false)
		if err != nil || !moved {
			t.Fatalf("MoveBurst to skipped = %v, %v, want true", moved, err)
		}
		got := readBurst(t, row.ID)
		if got.State != models.BurstStateSkipped || got.Reason != "admin" || got.ClaimedAt != nil || got.Attempts != 0 {
			t.Errorf("row = %q / %q, claim %v, %d attempt(s), want skipped / admin, no claim, no attempt", got.State, got.Reason, got.ClaimedAt, got.Attempts)
		}
	})

	t.Run("MoveBurst counts an attempt only when asked", func(t *testing.T) {
		ld := startLockdown(t)
		row := queuedBurstRow(t, ld, 9203)
		claim := func() {
			t.Helper()
			if won, err := ClaimBurst(row.ID, models.BurstStateQueued, models.BurstStateActing); err != nil || !won {
				t.Fatalf("ClaimBurst = %v, %v, want true", won, err)
			}
		}

		claim()
		if moved, err := MoveBurst(row.ID, models.BurstStateActing, models.BurstStateQueued, "", "", false); err != nil || !moved {
			t.Fatalf("MoveBurst back = %v, %v", moved, err)
		}
		if got := readBurst(t, row.ID); got.Attempts != 0 || got.State != models.BurstStateQueued {
			t.Errorf("row = %q with %d attempt(s), want queued with 0", got.State, got.Attempts)
		}

		claim()
		if moved, err := MoveBurst(row.ID, models.BurstStateActing, models.BurstStateQueued, "", "boom", true); err != nil || !moved {
			t.Fatalf("MoveBurst counting = %v, %v", moved, err)
		}
		if got := readBurst(t, row.ID); got.Attempts != 1 || got.Detail != "boom" {
			t.Errorf("row = %d attempt(s), detail %q, want 1 and boom", got.Attempts, got.Detail)
		}
	})

	t.Run("GetJoinerOfUserFresh", func(t *testing.T) {
		ld := startLockdown(t)
		other := startLockdown(t)
		mustRecord(t, newJoinRecord(ld, 9204))

		got, err := GetJoinerOfUserFresh(ld.ID, 9204)
		if err != nil || got == nil || got.UserID != 9204 || got.LockdownID != ld.ID {
			t.Fatalf("GetJoinerOfUserFresh = %+v, %v, want the lockdown's row", got, err)
		}
		if got, err := GetJoinerOfUserFresh(ld.ID, 9205); err != nil || got != nil {
			t.Errorf("GetJoinerOfUserFresh for an unknown user = %+v, %v, want nil, nil", got, err)
		}
		if got, err := GetJoinerOfUserFresh(other.ID, 9204); err != nil || got != nil {
			t.Errorf("GetJoinerOfUserFresh in another lockdown = %+v, %v, want nil, nil", got, err)
		}
	})
}

// cleanupBanRuns deletes the ban runs of the given chats when the test ends.
func cleanupBanRuns(t *testing.T, chatIDs ...int64) {
	t.Helper()
	t.Cleanup(func() {
		db.DB.Where("chat_id IN ?", chatIDs).Delete(&models.ChatLockdownBanRun{})
	})
}

func newBanRun(ld *models.ChatLockdown) *models.ChatLockdownBanRun {
	return &models.ChatLockdownBanRun{
		LockdownID:    ld.ID,
		ChatID:        ld.ChatID,
		StartedBy:     77,
		StartedByName: "Starter",
	}
}

func readBanRun(t *testing.T, id uint) models.ChatLockdownBanRun {
	t.Helper()
	var run models.ChatLockdownBanRun
	if err := db.DB.First(&run, id).Error; err != nil {
		t.Fatalf("read ban run %d: %v", id, err)
	}
	return run
}

func burstRowOfUser(t *testing.T, lockdownID uint, userID int64) models.ChatLockdownBurst {
	t.Helper()
	var row models.ChatLockdownBurst
	if err := db.DB.Where("lockdown_id = ? AND user_id = ?", lockdownID, userID).First(&row).Error; err != nil {
		t.Fatalf("read burst row of %d: %v", userID, err)
	}
	return row
}

func TestBanRunStart(t *testing.T) {
	const a, b, c, d, e = int64(9401), int64(9402), int64(9403), int64(9404), int64(9405)

	// raidLockdown has pre rows A and B (nothing done to them yet), pending joiners C
	// and D, and E, a joiner an admin let in (exempt).
	raidLockdown := func(t *testing.T) *models.ChatLockdown {
		t.Helper()
		chat := uniqueLockdownChatID(t)
		cleanupBurstLockdowns(t, chat)
		cleanupBanRuns(t, chat)
		ld := mustStart(t, chat)
		mustConfirm(t, ld.ID)

		joined := time.Now().UTC().Add(-5 * time.Minute)
		for _, row := range []models.ChatLockdownBurst{
			preRow(ld, a, "A", joined),
			preRow(ld, b, "B", joined.Add(time.Minute)),
		} {
			if err := db.DB.Create(&row).Error; err != nil {
				t.Fatalf("seed pre row %d: %v", row.UserID, err)
			}
		}
		mustRecord(t, newJoinRecord(ld, c))
		mustRecord(t, newJoinRecord(ld, d))
		exempt := newJoinRecord(ld, e)
		exempt.State = models.JoinerStateExempt
		mustRecord(t, exempt)
		return ld
	}

	t.Run("candidates leave out excluded users and exempt joiners", func(t *testing.T) {
		ld := raidLockdown(t)

		for _, tt := range []struct {
			name    string
			exclude []int64
			want    int64
		}{
			{"no exclusion", nil, 4},
			{"empty exclusion", []int64{}, 4},
			{"a pre account excluded", []int64{a}, 3},
			{"a joiner and the exempt one excluded", []int64{c, e}, 3},
			{"everyone excluded", []int64{a, b, c, d}, 0},
		} {
			got, err := CountBanCandidatesFresh(ld.ID, tt.exclude)
			if err != nil || got != tt.want {
				t.Errorf("%s: CountBanCandidatesFresh = %d, %v, want %d", tt.name, got, err, tt.want)
			}
		}
	})

	t.Run("a run queues what is unhandled and skips the exempt", func(t *testing.T) {
		ld := raidLockdown(t)

		run := newBanRun(ld)
		started, queued, err := StartBanRun(run, nil)
		if err != nil || !started || queued != 4 || run.ID == 0 {
			t.Fatalf("StartBanRun = %v, %d, %v (run %d), want true, 4, nil", started, queued, err, run.ID)
		}
		if got := readBanRun(t, run.ID); got.State != models.BanRunStateRunning || got.CardDest != "staff" {
			t.Errorf("run = %q for %q, want running for staff", got.State, got.CardDest)
		}

		for _, userID := range []int64{a, b, c, d} {
			row := burstRowOfUser(t, ld.ID, userID)
			if row.State != models.BurstStateQueued || row.RunID == nil || *row.RunID != int64(run.ID) {
				t.Errorf("burst row of %d = %q, run %v, want queued for run %d", userID, row.State, row.RunID, run.ID)
			}
		}
		for _, userID := range []int64{c, d, e} {
			if row := burstRowOfUser(t, ld.ID, userID); row.Source != models.BurstSourceJoiner || row.JoinedAt == nil || row.FirstName != "Raider" {
				t.Errorf("copied row of %d = source %q, joined_at %v, name %q, want a joiner row with the joiner's data", userID, row.Source, row.JoinedAt, row.FirstName)
			}
		}
		skipped := burstRowOfUser(t, ld.ID, e)
		if skipped.State != models.BurstStateSkipped || skipped.Reason != "let_in" || skipped.RunID == nil || *skipped.RunID != int64(run.ID) {
			t.Errorf("exempt joiner's burst row = %q / %q, run %v, want skipped / let_in for run %d", skipped.State, skipped.Reason, skipped.RunID, run.ID)
		}
		if rows := readBurstRows(t, ld.ID); len(rows) != 5 {
			t.Errorf("%d burst rows, want 5 (A, B, and the three joiners copied once)", len(rows))
		}

		// A second run while this one runs is refused and writes nothing.
		second := newBanRun(ld)
		started, queued, err = StartBanRun(second, nil)
		if err != nil || started || queued != 0 {
			t.Errorf("second StartBanRun = %v, %d, %v, want false, 0, nil", started, queued, err)
		}
		var runs int64
		db.DB.Model(&models.ChatLockdownBanRun{}).Where("lockdown_id = ?", ld.ID).Count(&runs)
		if runs != 1 {
			t.Errorf("%d runs of the lockdown, want 1", runs)
		}
	})

	t.Run("excluded accounts are not queued", func(t *testing.T) {
		ld := raidLockdown(t)

		run := newBanRun(ld)
		started, queued, err := StartBanRun(run, []int64{a, c})
		if err != nil || !started || queued != 2 {
			t.Fatalf("StartBanRun = %v, %d, %v, want true, 2, nil", started, queued, err)
		}
		for userID, want := range map[int64]string{a: "", b: models.BurstStateQueued, c: "", d: models.BurstStateQueued} {
			if got := burstRowOfUser(t, ld.ID, userID); got.State != want {
				t.Errorf("burst row of %d = %q, want %q", userID, got.State, want)
			}
		}
	})

	t.Run("nothing to queue finishes the run", func(t *testing.T) {
		chat := uniqueLockdownChatID(t)
		cleanupBurstLockdowns(t, chat)
		cleanupBanRuns(t, chat)
		ld := mustStart(t, chat)
		mustConfirm(t, ld.ID)

		run := newBanRun(ld)
		started, queued, err := StartBanRun(run, nil)
		if err != nil || !started || queued != 0 {
			t.Fatalf("StartBanRun = %v, %d, %v, want true, 0, nil", started, queued, err)
		}
		if got := readBanRun(t, run.ID); got.State != models.BanRunStateDone || got.FinishedAt == nil {
			t.Errorf("run = %q, finished %v, want done with a finish time", got.State, got.FinishedAt)
		}
		// A done run no longer blocks a new one.
		if started, _, err := StartBanRun(newBanRun(ld), nil); err != nil || !started {
			t.Errorf("StartBanRun after a done run = %v, %v, want true", started, err)
		}
	})

	t.Run("a later run takes only what no earlier run handled", func(t *testing.T) {
		ld := raidLockdown(t)
		first := newBanRun(ld)
		if started, queued, err := StartBanRun(first, []int64{a}); err != nil || !started || queued != 3 {
			t.Fatalf("first StartBanRun = %v, %d, %v, want true, 3", started, queued, err)
		}
		if finished, err := FinishBanRun(first.ID, models.BanRunStateStopped); err != nil || !finished {
			t.Fatalf("FinishBanRun stopped = %v, %v, want true", finished, err)
		}

		second := newBanRun(ld)
		started, queued, err := StartBanRun(second, nil)
		if err != nil || !started || queued != 1 {
			t.Fatalf("second StartBanRun = %v, %d, %v, want true, 1: only A was unhandled", started, queued, err)
		}
		if got := burstRowOfUser(t, ld.ID, a); got.RunID == nil || *got.RunID != int64(second.ID) || got.State != models.BurstStateQueued {
			t.Errorf("row of A = %q, run %v, want queued for run %d", got.State, got.RunID, second.ID)
		}
		if got := burstRowOfUser(t, ld.ID, b); got.RunID == nil || *got.RunID != int64(first.ID) {
			t.Errorf("row of B run = %v, want it left with the first run %d", got.RunID, first.ID)
		}
	})

	t.Run("finish, tally, card and listings", func(t *testing.T) {
		ld := raidLockdown(t)
		run := newBanRun(ld)
		if started, queued, err := StartBanRun(run, nil); err != nil || !started || queued != 4 {
			t.Fatalf("StartBanRun = %v, %d, %v, want true, 4", started, queued, err)
		}

		running, err := ListRunningBanRunsFresh()
		found := false
		for _, r := range running {
			found = found || r.ID == run.ID
		}
		if err != nil || !found {
			t.Errorf("ListRunningBanRunsFresh = %d run(s), %v, want it to include run %d", len(running), err, run.ID)
		}

		listed, err := ListQueuedBurstFresh(run.ID, 3)
		if err != nil || len(listed) != 3 {
			t.Fatalf("ListQueuedBurstFresh(limit 3) = %d row(s), %v, want 3", len(listed), err)
		}
		if listed[0].UserID != a || listed[1].UserID != b {
			t.Errorf("queued rows start with %d, %d, want the order they were queued in (A, B first)", listed[0].UserID, listed[1].UserID)
		}

		if finished, err := FinishBanRun(run.ID, models.BanRunStateDone); err != nil || finished {
			t.Errorf("FinishBanRun done with rows queued = %v, %v, want false", finished, err)
		}

		// Move every queued row: A and B banned, C skipped as admin, D banned as a request.
		for userID, move := range map[int64][3]string{
			a: {models.BurstStateBanned, "", ""},
			b: {models.BurstStateBanned, "", ""},
			c: {models.BurstStateSkipped, "admin", ""},
			d: {models.BurstStateBanned, "request", ""},
		} {
			row := burstRowOfUser(t, ld.ID, userID)
			if won, err := ClaimBurst(row.ID, models.BurstStateQueued, models.BurstStateActing); err != nil || !won {
				t.Fatalf("ClaimBurst(%d) = %v, %v", userID, won, err)
			}
			if finished, _ := FinishBanRun(run.ID, models.BanRunStateDone); finished {
				t.Fatal("FinishBanRun done while a row is acting = true, want false")
			}
			if moved, err := MoveBurst(row.ID, models.BurstStateActing, move[0], move[1], move[2], false); err != nil || !moved {
				t.Fatalf("MoveBurst(%d) = %v, %v", userID, moved, err)
			}
		}

		tally, err := TallyBanRun(run.ID)
		if err != nil {
			t.Fatalf("TallyBanRun: %v", err)
		}
		if tally[models.BurstStateBanned][""] != 2 || tally[models.BurstStateBanned]["request"] != 1 ||
			tally[models.BurstStateSkipped]["admin"] != 1 || tally[models.BurstStateSkipped]["let_in"] != 1 {
			t.Errorf("tally = %v, want banned 2 and 1 request, skipped 1 admin and 1 let_in", tally)
		}

		if finished, err := FinishBanRun(run.ID, models.BanRunStateDone); err != nil || !finished {
			t.Errorf("FinishBanRun done = %v, %v, want true once nothing is queued or acting", finished, err)
		}
		if finished, err := FinishBanRun(run.ID, models.BanRunStateDone); err != nil || finished {
			t.Errorf("second FinishBanRun = %v, %v, want false: a run finishes exactly once", finished, err)
		}
		if got := readBanRun(t, run.ID); got.State != models.BanRunStateDone || got.FinishedAt == nil {
			t.Errorf("run = %q, finished %v, want done with a finish time", got.State, got.FinishedAt)
		}

		if err := SetBanRunCard(run.ID, -100123, 4242); err != nil {
			t.Fatalf("SetBanRunCard: %v", err)
		}
		if got := readBanRun(t, run.ID); got.CardChatID != -100123 || got.CardMsgID != 4242 {
			t.Errorf("run card = %d / %d, want -100123 / 4242", got.CardChatID, got.CardMsgID)
		}
	})
}

func TestReleaseStaleBurstClaims(t *testing.T) {
	chat := uniqueLockdownChatID(t)
	cleanupBurstLockdowns(t, chat)
	ld := mustStart(t, chat)
	mustConfirm(t, ld.ID)

	old := queuedBurstRow(t, ld, 9501)
	young := queuedBurstRow(t, ld, 9502)
	waiting := queuedBurstRow(t, ld, 9503)
	for _, id := range []uint{old.ID, young.ID} {
		if won, err := ClaimBurst(id, models.BurstStateQueued, models.BurstStateActing); err != nil || !won {
			t.Fatalf("setup: ClaimBurst(%d) = %v, %v", id, won, err)
		}
	}
	stale := time.Now().UTC().Add(-10 * time.Minute)
	if err := db.DB.Model(&models.ChatLockdownBurst{}).Where("id = ?", old.ID).
		Updates(map[string]any{"claimed_at": stale, "attempts": 2}).Error; err != nil {
		t.Fatalf("setup: age the claim: %v", err)
	}

	released, err := ReleaseStaleBurstClaims(time.Now().Add(-2 * time.Minute))
	if err != nil || released != 1 {
		t.Fatalf("ReleaseStaleBurstClaims = %d, %v, want 1: only the stale claim", released, err)
	}

	if got := readBurst(t, old.ID); got.State != models.BurstStateQueued || got.ClaimedAt != nil || got.Attempts != 2 {
		t.Errorf("stale row = %q, claim %v, %d attempt(s), want queued, no claim, attempts unchanged at 2", got.State, got.ClaimedAt, got.Attempts)
	}
	if got := readBurst(t, young.ID); got.State != models.BurstStateActing || got.ClaimedAt == nil {
		t.Errorf("young claim = %q, claim %v, want untouched (acting with its claim)", got.State, got.ClaimedAt)
	}
	if got := readBurst(t, waiting.ID); got.State != models.BurstStateQueued {
		t.Errorf("queued row = %q, want queued", got.State)
	}
}

func TestUnqueueBanRun(t *testing.T) {
	const a, b, c, d, e, f = int64(9601), int64(9602), int64(9603), int64(9604), int64(9605), int64(9606)

	chat := uniqueLockdownChatID(t)
	cleanupBurstLockdowns(t, chat)
	cleanupBanRuns(t, chat)
	ld := mustStart(t, chat)
	mustConfirm(t, ld.ID)

	joined := time.Now().UTC().Add(-5 * time.Minute)
	for _, userID := range []int64{a, b, c, d, e} {
		row := preRow(ld, userID, "Raider", joined)
		if err := db.DB.Create(&row).Error; err != nil {
			t.Fatalf("seed pre row %d: %v", userID, err)
		}
	}
	exempt := newJoinRecord(ld, f)
	exempt.State = models.JoinerStateExempt
	mustRecord(t, exempt)

	run := newBanRun(ld)
	if started, queued, err := StartBanRun(run, nil); err != nil || !started || queued != 5 {
		t.Fatalf("StartBanRun = %v, %d, %v, want true, 5, nil", started, queued, err)
	}
	// a is banned, b failed, c is being banned by a call in flight, d and e wait.
	move := func(userID int64, to string) {
		t.Helper()
		row := burstRowOfUser(t, ld.ID, userID)
		if won, err := MoveBurst(row.ID, models.BurstStateQueued, to, "", "", false); err != nil || !won {
			t.Fatalf("setup: MoveBurst(%d to %s) = %v, %v", userID, to, won, err)
		}
	}
	move(a, models.BurstStateBanned)
	move(b, models.BurstStateFailed)
	if won, err := ClaimBurst(burstRowOfUser(t, ld.ID, c).ID, models.BurstStateQueued, models.BurstStateActing); err != nil || !won {
		t.Fatalf("setup: ClaimBurst(c) = %v, %v", won, err)
	}

	unqueued, err := UnqueueBanRun(run.ID)
	if err != nil || unqueued != 2 {
		t.Fatalf("UnqueueBanRun = %d, %v, want 2: only the queued rows d and e", unqueued, err)
	}
	for _, userID := range []int64{d, e} {
		row := burstRowOfUser(t, ld.ID, userID)
		if row.State != models.BurstStateNone || row.RunID != nil || row.ClaimedAt != nil {
			t.Errorf("burst row of %d = %q, run %v, claim %v, want unhandled with no run and no claim", userID, row.State, row.RunID, row.ClaimedAt)
		}
	}
	for userID, want := range map[int64]string{
		a: models.BurstStateBanned,
		b: models.BurstStateFailed,
		c: models.BurstStateActing,
		f: models.BurstStateSkipped,
	} {
		row := burstRowOfUser(t, ld.ID, userID)
		if row.State != want || row.RunID == nil || *row.RunID != int64(run.ID) {
			t.Errorf("burst row of %d = %q, run %v, want %q still on run %d", userID, row.State, row.RunID, want, run.ID)
		}
	}

	// A stopped finish does not wait for the acting row, and happens once.
	if finished, err := FinishBanRun(run.ID, models.BanRunStateStopped); err != nil || !finished {
		t.Errorf("FinishBanRun stopped = %v, %v, want true although a row is still acting", finished, err)
	}
	if finished, err := FinishBanRun(run.ID, models.BanRunStateStopped); err != nil || finished {
		t.Errorf("second FinishBanRun = %v, %v, want false: a run finishes exactly once", finished, err)
	}
	if got := readBanRun(t, run.ID); got.State != models.BanRunStateStopped || got.FinishedAt == nil {
		t.Errorf("run = %q, finished %v, want stopped with a finish time", got.State, got.FinishedAt)
	}

	// The accounts it gave back are unhandled, so a later run could take them.
	if count, err := CountBanCandidatesFresh(ld.ID, nil); err != nil || count != 2 {
		t.Errorf("CountBanCandidatesFresh = %d, %v, want 2: d and e are unhandled again", count, err)
	}
}
