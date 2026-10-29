//go:build testtools

package modules

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/divkix/Alita_Robot/alita/db/lockdown"
)

// raidGatedClient holds every sendMessage until its gate opens, and says when one arrived.
// Every other method goes straight to the wrapped client.
type raidGatedClient struct {
	gotgbot.BotClient
	gate    chan struct{}
	entered chan struct{}
	once    sync.Once
}

func (g *raidGatedClient) open() { g.once.Do(func() { close(g.gate) }) }

func (g *raidGatedClient) RequestWithContext(
	ctx context.Context,
	token, method string,
	params map[string]any,
	opts *gotgbot.RequestOpts,
) (json.RawMessage, error) {
	if method == "sendMessage" {
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

// gateSends makes the env's bot hold every sendMessage until the returned client is
// opened. The gate is opened and every post in flight is waited for when the test ends, so
// a failing test leaves no goroutine behind.
func gateSends(env *lockdownEnv) *raidGatedClient {
	env.t.Helper()
	gated := &raidGatedClient{BotClient: env.fake, gate: make(chan struct{}), entered: make(chan struct{}, 1)}
	env.bot.BotClient = gated
	env.t.Cleanup(func() {
		gated.open()
		raidPosts.wait()
	})
	return gated
}

// sentCount is how many messages the fake has answered so far. It does not wait for the
// posts in flight, which is the point of the tests that use it.
func (e *lockdownEnv) sentCount() int {
	e.fake.fmu.Lock()
	defer e.fake.fmu.Unlock()
	return len(e.fake.sentLog)
}

// returnsWithin runs fn on its own goroutine and reports whether it returned within d.
func returnsWithin(d time.Duration, fn func()) bool {
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn()
	}()
	select {
	case <-done:
		return true
	case <-time.After(d):
		return false
	}
}

func TestRaidPostsDoNotBlockTheTrigger(t *testing.T) {
	t.Run("the join that tips the surge is recorded while the notice waits for a send", func(t *testing.T) {
		env := newLockdownEnv(t)
		linkLockdownStaffGroup(env)
		raidCleanup(t, env.chat.Id)
		gated := gateSends(env)

		env.selfJoins(14)
		tipping := env.newJoiner("Tipping")
		if !returnsWithin(10*time.Second, func() { env.joinOn(env.dispatcher, tipping) }) {
			gated.open()
			t.Fatal("the join handler did not return while the notice send was held back")
		}

		select {
		case <-gated.entered:
		case <-time.After(10 * time.Second):
			t.Fatal("the notice never reached its send")
		}
		row := env.activeLockdown()
		if row == nil || row.LockedAt == nil {
			t.Fatalf("active lockdown = %+v, want one confirmed by the join surge", row)
		}
		joiner, err := lockdown.GetJoinerOfUserFresh(row.ID, tipping.Id)
		if err != nil || joiner == nil {
			t.Fatalf("joiner row of the tipping joiner = %+v, %v, want one recorded before the notice is sent", joiner, err)
		}
		if got := env.sentCount(); got != 0 {
			t.Errorf("%d messages were sent while the gate was closed, want none", got)
		}

		gated.open()
		text := env.wantOneNoticeIn()
		if want := staffMarker("lockdown_auto_started_join_surge") + " 15 60"; !strings.Contains(text, want) {
			t.Errorf("notice %q does not contain %q", text, want)
		}
	})

	t.Run("the message that tips the flood goes on to later groups while the notice waits", func(t *testing.T) {
		env := newFloodEnv(t)
		gated := gateSends(env)
		recent := env.newMembers(4, 2*time.Hour)

		env.sendItems(recent, 29)
		tipping := floodMessage(t, recent[0], "text")
		id := env.updateID()
		tipping.MessageId, tipping.Date, tipping.Chat = id, 1, env.chat
		update := &gotgbot.Update{UpdateId: id, Message: &tipping}
		if !returnsWithin(10*time.Second, func() {
			if err := env.dispatcher.ProcessUpdate(env.bot, update, nil); err != nil {
				t.Errorf("ProcessUpdate(%d) error = %v", id, err)
			}
		}) {
			gated.open()
			t.Fatal("the message pipeline did not return while the notice send was held back")
		}

		select {
		case <-gated.entered:
		case <-time.After(10 * time.Second):
			t.Fatal("the notice never reached its send")
		}
		if row := env.activeLockdown(); row == nil || row.LockedAt == nil {
			t.Fatalf("active lockdown = %+v, want one confirmed by the flood", row)
		}

		gated.open()
		text := env.wantOneNoticeIn()
		if want := staffMarker("lockdown_auto_started_flood") + " 30 30 100"; !strings.Contains(text, want) {
			t.Errorf("notice %q does not contain %q", text, want)
		}
	})

	t.Run("the cannot-lock warning is sent from the goroutine too", func(t *testing.T) {
		env, staffChat, channel := warnEnv(t)
		env.fake.setBotRole(env.chat.Id, staffRoleAdminNoRestrict)
		gated := gateSends(env)

		if !returnsWithin(10*time.Second, func() { env.selfJoins(15) }) {
			gated.open()
			t.Fatal("the join handlers did not return while the warning send was held back")
		}
		select {
		case <-gated.entered:
		case <-time.After(10 * time.Second):
			t.Fatal("the warning never reached its send")
		}
		if got := env.sentCount(); got != 0 {
			t.Errorf("%d messages were sent while the gate was closed, want none", got)
		}

		gated.open()
		env.wantWarnings(staffChat, channel, 1)
	})
}

func TestStopRaidPosts(t *testing.T) {
	t.Run("waits for a post in flight", func(t *testing.T) {
		env := newLockdownEnv(t)
		linkLockdownStaffGroup(env)
		raidCleanup(t, env.chat.Id)
		gated := gateSends(env)
		env.selfJoins(15)
		select {
		case <-gated.entered:
		case <-time.After(10 * time.Second):
			t.Fatal("the notice never reached its send")
		}

		previous := raidPostStopWait
		raidPostStopWait = 20 * time.Millisecond
		t.Cleanup(func() { raidPostStopWait = previous })
		if StopRaidPosts() {
			t.Fatal("StopRaidPosts reported the posts done while a send was held back")
		}

		raidPostStopWait = 10 * time.Second
		gated.open()
		if !StopRaidPosts() {
			t.Fatal("StopRaidPosts did not report the posts done after the send went through")
		}
		if got := len(env.replies()); got != 1 {
			t.Errorf("%d messages reached the locked group, want the notice", got)
		}
	})

	t.Run("a panicking post is recovered and counted done", func(t *testing.T) {
		raidPostAsync("test post", func() { panic(fmt.Sprintf("post %d", 1)) })
		if !StopRaidPosts() {
			t.Error("StopRaidPosts did not report a recovered post as done")
		}
	})
}
