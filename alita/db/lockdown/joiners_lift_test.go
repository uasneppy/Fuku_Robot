//go:build testtools

package lockdown

import (
	"testing"

	"github.com/divkix/Alita_Robot/alita/db/models"
)

func TestCancelPendingAndListJoinersInState(t *testing.T) {
	chat := uniqueLockdownChatID(t)
	other := uniqueLockdownChatID(t)
	cleanupLockdowns(t, chat, other)
	ld := mustStart(t, chat)
	otherLd := mustStart(t, other)

	first := mustRecord(t, newJoinRecord(ld, 901))
	second := mustRecord(t, newJoinRecord(ld, 902))
	bannedRec := newJoinRecord(ld, 903)
	bannedRec.State = models.JoinerStateBanned
	banned := mustRecord(t, bannedRec)
	foreign := mustRecord(t, newJoinRecord(otherLd, 904))

	cancelled, err := CancelPending(ld.ID)
	if err != nil || cancelled != 2 {
		t.Fatalf("CancelPending = %d, %v, want 2, nil", cancelled, err)
	}
	for _, id := range []uint{first.ID, second.ID} {
		if row := readJoiner(t, id); row.State != models.JoinerStateCancelled {
			t.Errorf("joiner %d state = %q, want cancelled", id, row.State)
		}
	}
	if row := readJoiner(t, banned.ID); row.State != models.JoinerStateBanned {
		t.Errorf("banned joiner state = %q, want untouched", row.State)
	}
	if row := readJoiner(t, foreign.ID); row.State != models.JoinerStatePending {
		t.Errorf("another lockdown's pending joiner state = %q, want untouched", row.State)
	}
	if again, err := CancelPending(ld.ID); err != nil || again != 0 {
		t.Errorf("second CancelPending = %d, %v, want 0, nil", again, err)
	}

	moreRec := newJoinRecord(ld, 905)
	moreRec.State = models.JoinerStateBanned
	third := mustRecord(t, moreRec)
	rows, err := ListJoinersInState(ld.ID, models.JoinerStateBanned, 10)
	if err != nil || len(rows) != 2 || rows[0].ID != banned.ID || rows[1].ID != third.ID {
		t.Errorf("ListJoinersInState = %+v, %v, want the two banned rows, oldest first", rows, err)
	}
	limited, err := ListJoinersInState(ld.ID, models.JoinerStateBanned, 1)
	if err != nil || len(limited) != 1 || limited[0].ID != banned.ID {
		t.Errorf("ListJoinersInState(limit 1) = %+v, %v, want the oldest row only", limited, err)
	}
}

func TestListLiftingFresh(t *testing.T) {
	chatA := uniqueLockdownChatID(t)
	chatB := uniqueLockdownChatID(t)
	cleanupLockdowns(t, chatA, chatB)
	lifting := mustStart(t, chatA)
	mustStart(t, chatB)
	if won, err := BeginLift(lifting.ID, 1, "Admin", false); err != nil || !won {
		t.Fatalf("BeginLift = %v, %v", won, err)
	}

	rows, err := ListLiftingFresh()
	if err != nil {
		t.Fatalf("ListLiftingFresh error = %v", err)
	}
	var ours []uint
	for _, row := range rows {
		if row.ChatID == chatA || row.ChatID == chatB {
			ours = append(ours, row.ID)
		}
	}
	if len(ours) != 1 || ours[0] != lifting.ID {
		t.Errorf("lifting lockdowns of our chats = %v, want only [%d]: an active lockdown is not lifting", ours, lifting.ID)
	}
}
