//go:build testtools

package modules

import (
	"context"
	"strconv"
	"strings"
	"sync"
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
		// Only the in-flight member's lookup is slow: a delay on the whole chat would also hold
		// the update goroutine inside the other handlers' own lookups.
		var reached sync.Once
		started := make(chan struct{})
		env.fake.setOnGetMember(func(_, userID int64) {
			if userID == inFlight.Id {
				reached.Do(func() { close(started) })
				time.Sleep(400 * time.Millisecond)
			}
		})

		env.mreqSend(inFlight, "in flight")
		mreqAwait(t, started, "the job never reached its lookup")

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
		var reached, freed sync.Once
		started, release := make(chan struct{}), make(chan struct{})
		t.Cleanup(func() { freed.Do(func() { close(release) }) })
		env.fake.setOnGetMember(func(_, userID int64) {
			if userID == stuck.Id {
				reached.Do(func() { close(started) })
				<-release
			}
		})
		env.mreqSend(stuck, "stuck")
		mreqAwait(t, started, "the job never reached its lookup")

		start := time.Now()
		if StopMemberReqJobs() {
			t.Error("StopMemberReqJobs reported idle although the job was still running")
		}
		if elapsed := time.Since(start); elapsed > memberReqStopWait+500*time.Millisecond {
			t.Errorf("StopMemberReqJobs took %v, want about %v", elapsed, memberReqStopWait)
		}
	})
}

// mreqAwait waits for ch to close, so a test only acts once its job is inside the lookup it
// holds.
func mreqAwait(t *testing.T, ch <-chan struct{}, failure string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatal(failure)
	}
}

// TestShutdownEndsMemberReqLookups: a job that is inside a hung photo lookup when
// StopMemberReqJobs runs is ended by the drain's own cancel of the root context, inside the
// drain's existing wait. The lookups add no drain and no pool: the job leaves no hold, no
// notice and no write, a later message starts no job, and the lookup's flight goroutine has
// ended (a fresh flight of the same key runs at once instead of joining a stuck one).
func TestShutdownEndsMemberReqLookups(t *testing.T) {
	restoreMemberReqRunner(t)
	env, state := mreqLookupEnv(t)
	env.mreqRules(map[memberreq.Rule]int{memberreq.RulePhoto: models.RequirementActionMute})
	inFlight := mreqLookupMember(t, env, state, mreqPhotoAnswer{})
	late := mreqLookupMember(t, env, state, mreqPhotoAnswer{})
	mreqSwap(t, &memberReqStopWait, 3*time.Second)

	// The lookup is held open and is never released: only the drain's cancel can end it.
	release := state.hold(inFlight.Id)
	t.Cleanup(release)
	var reached sync.Once
	started := make(chan struct{})
	state.setOnPhoto(func(int64) { reached.Do(func() { close(started) }) })

	if !memberReqRunAsync(env.bot, env.mreqCase(inFlight)) {
		t.Fatal("the job was not started")
	}
	mreqAwait(t, started, "the job never reached its photo lookup")

	begun := time.Now()
	if !StopMemberReqJobs() {
		t.Fatal("StopMemberReqJobs reported a job still running: a lookup must end when the root is cancelled")
	}
	if elapsed := time.Since(begun); elapsed > memberReqStopWait {
		t.Errorf("StopMemberReqJobs took %s, want inside memberReqStopWait (%s)", elapsed, memberReqStopWait)
	}
	if n := env.mreqCallCount("restrictChatMember", "banChatMember", "unbanChatMember", "sendMessage"); n != 0 {
		t.Errorf("the cancelled job made %d writes or messages, want none", n)
	}
	if hold := env.mreqHold(inFlight.Id); hold != nil {
		t.Errorf("the cancelled job left a hold %+v", hold)
	}
	if row := env.mreqWarnRow(inFlight.Id); row != nil {
		t.Errorf("the cancelled job left a warns row %+v", row)
	}

	// A message after the drain starts no job, so nothing asks about the late member.
	if memberReqRunAsync(env.bot, env.mreqCase(late)) {
		t.Error("a job started after the drain")
	}
	if n := mreqLookupPhotoCalls(env); n != 1 {
		t.Errorf("photo requests = %d, want only the in-flight job's", n)
	}

	// The flight goroutine of the cancelled lookup has ended: a fresh flight of its key runs
	// its own function at once. A flight still stuck in the held request would be joined, and
	// its result would come back shared and late.
	// StopMemberReqJobs returns when the job has left its lookup, which can be a moment before the
	// flight's goroutine has unwound after its own cancel (under a loaded machine CI saw a shared
	// result at once), so the fresh flight is asked again until it is no longer joined: a flight
	// stuck in the held request would stay joined for the whole two seconds and fail.
	key := "p:" + strconv.FormatInt(inFlight.Id, 10)
	deadline := time.Now().Add(2 * time.Second)
	for {
		fresh := memberReqLookupFlight.DoChan(key, func() (any, error) { return "fresh", nil })
		select {
		case result := <-fresh:
			if result.Val == "fresh" && !result.Shared {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("flight result = %+v, want a fresh flight of its own (the cancelled one has ended)", result)
			}
			time.Sleep(10 * time.Millisecond)
			continue
		case <-time.After(2 * time.Second):
			t.Fatal("the flight of the cancelled lookup is still running")
		}
		break
	}

	// With the runner restored, the second member's lookup is answered.
	memberReqDraining.Store(false)
	memberReqRoot.mu.Lock()
	memberReqRoot.ctx, memberReqRoot.cancel = context.WithCancel(context.Background())
	memberReqRoot.mu.Unlock()
	if got := mreqPhoto(env, late.Id, memberReqLookupMode{}); got != memberReqBreach {
		t.Errorf("a lookup after the runner was restored = %d, want a breach", got)
	}
}

// TestShutdownEndsMemberReqAI: a job that is inside a hung provider call of the AI spam-name rule
// when StopMemberReqJobs runs is ended by the drain's own cancel of the root context, inside the
// drain's existing wait. The AI call adds no drain, pool or queue (the 65 s worst case of the drain
// list is unchanged): the job leaves no hold, notice, write or warn row, the cancel is not a provider
// failure for the breaker, nothing is cached, a later message starts no job, and neither the flight
// goroutine nor the gate slot of the cancelled call remains.
func TestShutdownEndsMemberReqAI(t *testing.T) {
	restoreMemberReqRunner(t)
	withAIGate(t, 1, 3*time.Second)
	env, ai := mreqAIEnvOf(t, map[memberreq.Rule]int{memberreq.RuleAI: models.RequirementActionMute}, false)
	inFlight := mreqAIMember(env, "Casino")
	late := mreqAIMember(env, "Casino")
	mreqAIMarkNew(t, env.chat.Id, inFlight.Id)
	mreqAIMarkNew(t, env.chat.Id, late.Id)
	mreqSwap(t, &memberReqStopWait, 3*time.Second)

	// The provider stand-in holds the request and is only released by the test's cleanup: nothing
	// but the drain's cancel can end the call.
	ai.setDefault(mreqAIHang())

	if !memberReqRunAsync(env.bot, env.mreqCase(inFlight)) {
		t.Fatal("the job was not started")
	}
	ai.waitCount(t, 1)

	begun := time.Now()
	if !StopMemberReqJobs() {
		t.Fatal("StopMemberReqJobs reported a job still running: a provider call must end when the root is cancelled")
	}
	if elapsed := time.Since(begun); elapsed > memberReqStopWait {
		t.Errorf("StopMemberReqJobs took %s, want inside memberReqStopWait (%s)", elapsed, memberReqStopWait)
	}
	if n := env.mreqCallCount("restrictChatMember", "banChatMember", "unbanChatMember", "sendMessage"); n != 0 {
		t.Errorf("the cancelled job made %d writes or messages, want none", n)
	}
	if hold := env.mreqHold(inFlight.Id); hold != nil {
		t.Errorf("the cancelled job left a hold %+v", hold)
	}
	if row := env.mreqWarnRow(inFlight.Id); row != nil {
		t.Errorf("the cancelled job left a warns row %+v", row)
	}
	if keys := mreqAIVerdictKeys(t); len(keys) != 0 {
		t.Errorf("verdict keys = %v, want none: a cancelled call caches nothing", keys)
	}
	memberReqAIBreakers.mu.Lock()
	group := memberReqAIBreakers.groups[env.chat.Id]
	memberReqAIBreakers.mu.Unlock()
	if group != nil && (group.failures != 0 || !group.pausedTill.IsZero()) {
		t.Errorf("breaker state after the cancel = %+v, want no failure counted: a shutdown is not a provider fault", *group)
	}

	// A message after the drain starts no job, so the provider is never asked about the late member.
	if memberReqRunAsync(env.bot, env.mreqCase(late)) {
		t.Error("a job started after the drain")
	}
	if n := ai.count(); n != 1 {
		t.Errorf("provider requests = %d, want only the in-flight job's", n)
	}

	// The flight goroutine of the cancelled call has ended: a fresh flight of its key runs its own
	// function at once. A flight still stuck in the held request would be joined and come back
	// shared. The goroutine can unwind a moment after the job has left, so it is asked again until
	// it is no longer joined.
	key := "n:" + mreqAIKeyOf(t, mreqAINamesOfUser(inFlight))
	deadline := time.Now().Add(2 * time.Second)
	for {
		fresh := memberReqLookupFlight.DoChan(key, func() (any, error) { return "fresh", nil })
		var result struct {
			val    any
			shared bool
		}
		select {
		case r := <-fresh:
			result.val, result.shared = r.Val, r.Shared
		case <-time.After(2 * time.Second):
			t.Fatal("the flight of the cancelled call is still running")
		}
		if result.val == "fresh" && !result.shared {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("flight result = %+v, want a fresh flight of its own (the cancelled one has ended)", result)
		}
		time.Sleep(10 * time.Millisecond)
	}

	// The gate slot is free again (the gate here has one slot), asked again for the same reason.
	for !memberReqAIGate.TryAcquire(1) {
		if time.Now().After(deadline.Add(2 * time.Second)) {
			t.Fatal("the gate slot of the cancelled call was never released")
		}
		time.Sleep(10 * time.Millisecond)
	}
	memberReqAIGate.Release(1)
}
