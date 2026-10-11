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
