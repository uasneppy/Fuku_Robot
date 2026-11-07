//go:build testtools

package modules

import (
	"strings"
	"testing"
	"time"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/lockdown"
	"github.com/uasneppy/fuku_robot/fuku/db/memberreq"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/utils/shutdown"
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

// TestShutdownWaitsForLockdownWorker holds the lockdown worker inside an unban call
// and runs a real shutdown.Manager. The DB-close probe must only see the joiner row
// written once the blocked call settled, which needs the drain's own allowance.
func TestShutdownWaitsForLockdownWorker(t *testing.T) {
	env := newLockdownEnv(t)
	ld, _ := env.lockAndBan("Ann", "Bob")
	env.send(env.admin, "/unlockdown")
	if row := env.freshRow(ld.ID); row.State != models.LockdownStateLifting {
		t.Fatalf("State after /unlockdown = %q, want lifting", row.State)
	}

	previousTick, previousWait := lockdownWorkerTick, lockdownWorkerStopWait
	lockdownWorkerTick = 10 * time.Millisecond
	lockdownWorkerStopWait = 3 * time.Second
	t.Cleanup(func() {
		StopLockdownWorker()
		lockdownWorkerTick, lockdownWorkerStopWait = previousTick, previousWait
	})
	env.fake.setDelay("unbanChatMember", env.chat.Id, 300*time.Millisecond)

	StartLockdownWorker(env.bot)
	waitUntil(t, 5*time.Second, "the worker to be inside its first unban call", func() bool {
		return len(env.fake.requestTimes("unbanChatMember", env.chat.Id)) >= 1
	})

	type observation struct {
		unbanning int
		err       error
	}
	observed := make(chan observation, 1)
	m := shutdown.NewManager()
	// The probe stands for the DB-close handler and must not call t.Fatal.
	m.RegisterHandler(func() error {
		rows, err := lockdown.ListJoinersInState(ld.ID, models.JoinerStateUnbanning, 10)
		observed <- observation{unbanning: len(rows), err: err}
		return nil
	})
	RegisterLockdownWorkerDrain(m)

	if code := m.RunForTest(10*time.Second, 50*time.Millisecond); code != 0 {
		t.Fatalf("shutdown exit code = %d, want 0", code)
	}
	select {
	case obs := <-observed:
		if obs.err != nil {
			t.Fatalf("probe could not read the joiner rows: %v", obs.err)
		}
		if obs.unbanning != 0 {
			t.Errorf("%d joiner row(s) still unbanning when the database-close handler ran, want 0", obs.unbanning)
		}
	default:
		t.Fatal("the database-close probe never ran")
	}
}

// TestShutdownWaitsForRaidPosts holds the notice of an automatic lockdown inside a slow
// send and runs a real shutdown.Manager. The DB-close probe must only see the notice
// delivered once the post finished, which needs the drain's own allowance, far above the
// manager's 50 ms default.
func TestShutdownWaitsForRaidPosts(t *testing.T) {
	env := newLockdownEnv(t)
	linkLockdownStaffGroup(env)
	raidCleanup(t, env.chat.Id)
	env.fake.setDelay("sendMessage", env.chat.Id, 300*time.Millisecond)

	previous := raidPostStopWait
	raidPostStopWait = 3 * time.Second
	t.Cleanup(func() { raidPostStopWait = previous })

	env.selfJoins(15)

	observed := make(chan int, 1)
	m := shutdown.NewManager()
	// The probe stands for the DB-close handler and must not call t.Fatal.
	m.RegisterHandler(func() error {
		observed <- env.sentCount()
		return nil
	})
	RegisterRaidPostsDrain(m)

	if code := m.RunForTest(10*time.Second, 50*time.Millisecond); code != 0 {
		t.Fatalf("shutdown exit code = %d, want 0", code)
	}
	select {
	case sent := <-observed:
		if sent != 1 {
			t.Errorf("%d messages were sent when the database-close handler ran, want the notice delivered", sent)
		}
	default:
		t.Fatal("the database-close probe never ran")
	}
}

// TestShutdownWaitsForMemberReqJobs holds an enforcement job inside a slow lookup and runs a
// real shutdown.Manager. The DB-close probe (registered first, so it runs last) must only
// find the runner idle once the job has wound down, which needs the drain's own allowance,
// far above the manager's 50 ms default; the cancelled job punishes nobody, and a message
// after the drain starts no job.
func TestShutdownWaitsForMemberReqJobs(t *testing.T) {
	t.Run("the drain cancels the job, waits for it and a later message starts none", func(t *testing.T) {
		restoreMemberReqRunner(t)
		env := newMemberReqEnv(t)
		env.loadMemberReq()
		env.mreqRules(map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionMute})
		inFlight := env.mreqMember("Alex", "", "alex")
		late := env.mreqMember("Bea", "", "bea")
		mreqSwap(t, &memberReqStopWait, 3*time.Second)
		env.fake.setDelay("getChatMember", env.chat.Id, 400*time.Millisecond)

		env.mreqSend(inFlight, "in flight")

		idle := make(chan bool, 1)
		m := shutdown.NewManager()
		m.RegisterHandler(func() error {
			select {
			case <-memberReqJobs.idleChan():
				idle <- true
			default:
				idle <- false
			}
			return nil
		})
		RegisterMemberReqDrain(m)
		if code := m.RunForTest(10*time.Second, 50*time.Millisecond); code != 0 {
			t.Fatalf("shutdown exit code = %d, want 0", code)
		}
		select {
		case ok := <-idle:
			if !ok {
				t.Error("a job was still running when the database-close handler ran")
			}
		default:
			t.Fatal("the database-close probe never ran")
		}
		if n := mreqRestrictCalls(inFlight.Id, env); n != 0 {
			t.Errorf("the cancelled job restricted the member %d times, want 0", n)
		}
		if hold := env.mreqHold(inFlight.Id); hold != nil && hold.PunishedAt != nil {
			t.Errorf("the cancelled job left a punished hold %+v", hold)
		}

		env.fake.setDelay("getChatMember", env.chat.Id, 0)
		env.mreqSend(late, "after the drain")
		memberReqJobs.wait()
		if env.mreqHold(late.Id) != nil || env.mreqCallsAbout(late.Id, "getChatMember") != 0 {
			t.Error("a message after the drain started a job")
		}
	})

	t.Run("StopMemberReqJobs returns within its wait when a job does not end", func(t *testing.T) {
		restoreMemberReqRunner(t)
		env := newMemberReqEnv(t)
		env.loadMemberReq()
		env.mreqRules(map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionMute})
		stuck := env.mreqMember("Cy", "", "cy")
		mreqSwap(t, &memberReqStopWait, 200*time.Millisecond)
		env.fake.setDelay("getChatMember", env.chat.Id, 1500*time.Millisecond)
		env.mreqSend(stuck, "stuck")

		start := time.Now()
		if StopMemberReqJobs() {
			t.Error("StopMemberReqJobs reported idle although the job was still running")
		}
		if elapsed := time.Since(start); elapsed > memberReqStopWait+500*time.Millisecond {
			t.Errorf("StopMemberReqJobs took %v, want about %v", elapsed, memberReqStopWait)
		}
	})
}
