//go:build testtools

package modules

import (
	"strings"
	"testing"
	"time"

	"github.com/divkix/Alita_Robot/alita/db"
	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/utils/shutdown"
)

// staffShutdownObservation is what the DB-close probe saw when the manager reached it.
type staffShutdownObservation struct {
	finished bool
	rowErr   error
	summary  string
}

// TestShutdownWaitsForStaffRunSummary runs a real shutdown.Manager with a DB-close
// probe registered first (so it runs last) and the staff drain registered through
// RegisterStaffActionsDrain. The run winds down far slower than the 50 ms default
// allowance, so the probe only sees a finalized record and a delivered summary when
// the drain was given its own, longer allowance.
func TestShutdownWaitsForStaffRunSummary(t *testing.T) {
	env := newStaffActionEnv(t, 8)
	seedMembers(env)
	withStaffActionTimers(t, time.Hour, 5*time.Millisecond)
	resetStaffActionsContextAfter(t)
	staffActionStopWait = 5 * time.Second
	for _, group := range env.groups {
		env.fake.setDelay("getChatMember", group, 200*time.Millisecond)
	}

	_, msgID := env.startRun("/ban 4242")
	// Only now slow the Staff Group's edits, so the Confirm's own edits are not delayed.
	env.fake.setDelay("editMessageText", env.staffChat, 400*time.Millisecond)
	time.Sleep(50 * time.Millisecond)

	observed := make(chan staffShutdownObservation, 1)
	m := shutdown.NewManager()
	// The probe stands for the DB-close handler. It runs on the manager's goroutine,
	// so it must not call t.Fatal.
	m.RegisterHandler(func() error {
		var obs staffShutdownObservation
		var action models.StaffAction
		obs.rowErr = db.DB.Where("summary_msg_id = ?", msgID).Order("id DESC").First(&action).Error
		obs.finished = obs.rowErr == nil && action.FinishedAt != nil
		if edits := env.edits(env.staffChat, msgID); len(edits) > 0 {
			obs.summary, _ = edits[len(edits)-1].Params["text"].(string)
		}
		observed <- obs
		return nil
	})
	RegisterStaffActionsDrain(m)

	code := m.RunForTest(10*time.Second, 50*time.Millisecond)
	env.waitRuns()

	if code != 0 {
		t.Fatalf("shutdown exit code = %d, want 0", code)
	}
	select {
	case obs := <-observed:
		if obs.rowErr != nil {
			t.Fatalf("probe could not read the audit record: %v", obs.rowErr)
		}
		if !obs.finished {
			t.Error("the record was not finalized (finished_at nil) when the database-close handler ran")
		}
		if !strings.Contains(obs.summary, staffMarker("staff_act_fail_interrupted")) {
			t.Errorf("last card edit at database close is not the final summary:\n%s", obs.summary)
		}
		if strings.Contains(obs.summary, "⏳") {
			t.Errorf("card still had a pending line at database close:\n%s", obs.summary)
		}
	default:
		t.Fatal("the database-close probe never ran")
	}
}
