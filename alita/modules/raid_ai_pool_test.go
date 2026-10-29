//go:build testtools

package modules

import (
	"context"
	"encoding/json"
	"sync"
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
}
