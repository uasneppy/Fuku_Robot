//go:build testtools

package modules

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/divkix/Alita_Robot/alita/db/models"
)

// raidMethodGate holds every call of one Bot API method until its gate opens, and says
// when one arrived. Every other method goes straight to the wrapped client.
type raidMethodGate struct {
	gotgbot.BotClient
	method  string
	gate    chan struct{}
	entered chan struct{}
	once    sync.Once
}

func (g *raidMethodGate) open() { g.once.Do(func() { close(g.gate) }) }

func (g *raidMethodGate) RequestWithContext(
	ctx context.Context,
	token, method string,
	params map[string]any,
	opts *gotgbot.RequestOpts,
) (json.RawMessage, error) {
	if method == g.method {
		select {
		case g.entered <- struct{}{}:
		default:
		}
		select {
		case <-g.gate:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return g.BotClient.RequestWithContext(ctx, token, method, params, opts)
}

// gateMethod makes the env's bot hold every call of method until the returned gate is
// opened. The gate is opened when the test ends, before the cleanups registered earlier
// run, so a failing test leaves no call held back.
func gateMethod(env *lockdownEnv, method string) *raidMethodGate {
	env.t.Helper()
	gated := &raidMethodGate{BotClient: env.fake, method: method, gate: make(chan struct{}), entered: make(chan struct{}, 1)}
	env.bot.BotClient = gated
	env.t.Cleanup(gated.open)
	return gated
}

// wantEntered fails the test unless the gated method was called within a few seconds.
func (g *raidMethodGate) wantEntered(t *testing.T) {
	t.Helper()
	select {
	case <-g.entered:
	case <-time.After(10 * time.Second):
		g.open()
		t.Fatalf("%s was never called", g.method)
	}
}

// raidAIFiresIdle reports whether no lockdown start asked for by an AI verdict is in flight.
func raidAIFiresIdle() bool {
	select {
	case <-raidAIFires.idleChan():
		return true
	default:
		return false
	}
}

func TestRaidAIPool(t *testing.T) {
	t.Run("a lockdown start runs off the worker", func(t *testing.T) {
		env := newRaidAIEnv(t)
		gated := gateMethod(env, "setChatPermissions")
		accounts := env.newMembers(3, 2*time.Hour)

		env.spamFrom(accounts[0], 2)
		env.spamFrom(accounts[1], 2)
		env.spamFrom(accounts[2], 1)
		gated.wantEntered(t)

		if !returnsWithin(10*time.Second, raidAIJobs.wait) {
			gated.open()
			t.Fatal("the raid AI jobs were still running while the lockdown start waited for Telegram")
		}
		if raidAIFiresIdle() {
			gated.open()
			t.Fatal("no lockdown start is in flight although its lock call was held back")
		}
		if row := env.activeLockdown(); row != nil && row.LockedAt != nil {
			t.Fatalf("the lockdown is confirmed (%+v) while its lock call was held back", row)
		}

		gated.open()
		env.settleAI()
		row := env.activeLockdown()
		if row == nil || row.TriggerKind != models.LockdownTriggerAIBurst || row.LockedAt == nil {
			t.Fatalf("active lockdown = %+v, want a confirmed AI burst lockdown once the lock went through", row)
		}
	})

	t.Run("the /aispam worker is not held either", func(t *testing.T) {
		env := newAIBurstEnv(t)
		t.Cleanup(env.settleAI)
		gated := gateMethod(env, "setChatPermissions")
		accounts := env.newMembers(3, 0)

		env.spamFrom(accounts[0], 2)
		env.spamFrom(accounts[1], 2)
		env.spamFrom(accounts[2], 1)
		gated.wantEntered(t)

		if !returnsWithin(10*time.Second, DrainAISpamChecks) {
			gated.open()
			t.Fatal("the /aispam checks were still running while the lockdown start waited for Telegram")
		}
		if raidAIFiresIdle() {
			gated.open()
			t.Fatal("no lockdown start is in flight although its lock call was held back")
		}

		gated.open()
		env.settleAI()
		row := env.activeLockdown()
		if row == nil || row.TriggerKind != models.LockdownTriggerAIBurst || row.LockedAt == nil {
			t.Fatalf("active lockdown = %+v, want a confirmed AI burst lockdown once the lock went through", row)
		}
	})

	t.Run("bounded and shed", func(t *testing.T) {
		raidAIJobs.wait()
		resetRaidAIStats()
		gate := make(chan struct{})
		var release sync.Once
		open := func() { release.Do(func() { close(gate) }) }
		started := make(chan struct{}, raidAIWorkerCount)
		var processed atomic.Int64
		previous := raidAIProcess
		raidAIProcess = func(raidAIJob) {
			select {
			case started <- struct{}{}:
			default:
			}
			<-gate
			processed.Add(1)
		}
		t.Cleanup(func() {
			open()
			raidAIJobs.wait()
			raidAIProcess = previous
		})
		job := raidAIJob{chat: gotgbot.Chat{Id: -1001234500777, Type: "supergroup"}, userID: 11, text: "SPAM t.me/x"}

		for i := range raidAIWorkerCount {
			if !raidAIEnqueue(job) {
				t.Fatalf("job %d was shed by an idle pool", i+1)
			}
		}
		for i := range raidAIWorkerCount {
			select {
			case <-started:
			case <-time.After(10 * time.Second):
				open()
				t.Fatalf("only %d of %d workers took a job", i, raidAIWorkerCount)
			}
		}
		for i := range raidAIQueueCapacity {
			if !raidAIEnqueue(job) {
				open()
				t.Fatalf("queued job %d of %d was shed before the queue was full", i+1, raidAIQueueCapacity)
			}
		}
		shedBefore := GetRaidAIStats().Shed
		if raidAIEnqueue(job) {
			open()
			t.Fatal("a job was accepted although the 4 workers and the 128-job queue were full")
		}
		if got := GetRaidAIStats().Shed - shedBefore; got != 1 {
			t.Errorf("Shed grew by %d, want 1", got)
		}

		open()
		if !returnsWithin(10*time.Second, raidAIJobs.wait) {
			t.Fatal("the pool did not finish its jobs once the gate opened")
		}
		if want := int64(raidAIWorkerCount + raidAIQueueCapacity); processed.Load() != want {
			t.Errorf("%d jobs ran, want all %d that were accepted", processed.Load(), want)
		}
	})

	t.Run("a panicking job is recovered", func(t *testing.T) {
		raidAIJobs.wait()
		var panics, ran atomic.Int64
		previous := raidAIProcess
		raidAIProcess = func(job raidAIJob) {
			if job.userID == 1 {
				panics.Add(1)
				panic("a job that fails")
			}
			ran.Add(1)
		}
		t.Cleanup(func() {
			raidAIJobs.wait()
			raidAIProcess = previous
		})

		// More panicking jobs than workers: a worker that a panic stopped would leave the
		// jobs behind them unprocessed.
		const panicking, healthy = raidAIWorkerCount + 2, 8
		for range panicking {
			if !raidAIEnqueue(raidAIJob{userID: 1}) {
				t.Fatal("a job was shed by an idle pool")
			}
		}
		for range healthy {
			if !raidAIEnqueue(raidAIJob{userID: 2}) {
				t.Fatal("a job was shed by an idle pool")
			}
		}
		if !returnsWithin(10*time.Second, raidAIJobs.wait) {
			t.Fatal("the pool did not finish its jobs after a job panicked")
		}
		if panics.Load() != panicking || ran.Load() != healthy {
			t.Errorf("%d panicking and %d healthy jobs ran, want %d and %d", panics.Load(), ran.Load(), panicking, healthy)
		}
	})
}
