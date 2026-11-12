//go:build testtools

package modules

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
)

// liveMemberLookups counts the getChatMember calls of the fake that name someone other than the bot.
func (e *memberDeletionEnv) liveMemberLookups() int {
	count := 0
	for _, call := range e.fake.callsFor("getChatMember") {
		if staffParamInt(call.Params, "user_id") != staffTestBotID {
			count++
		}
	}
	return count
}

// adminListLoads counts the getChatAdministrators calls the fake has answered so far.
func (e *memberDeletionEnv) adminListLoads() int {
	return len(e.fake.callsFor("getChatAdministrators"))
}

// TestMemberDeletionAdminWarm holds WR-01 and SEC-17-01: behind a cold admin cache the exemption warms
// the list once through the shared loader instead of asking Telegram about every member, falls back to
// the per-user read only when the cache still cannot say, and never reads a failure as "not exempt".
func TestMemberDeletionAdminWarm(t *testing.T) {
	t.Run("many messages behind a cold cache make one list load", func(t *testing.T) {
		env := newSelfDestructEnv(t, memberSelfDestructRuleDelay)
		resetMemberDeletionFailureState(t)
		env.warmableAdminCache()
		loadsBefore := env.adminListLoads()
		members := []gotgbot.User{env.member(), env.member(), env.member(), env.member(), env.member()}
		for i := range members {
			env.sendMember(&members[i], func(m *gotgbot.Message) { m.Text = "hello" })
		}
		if got := env.adminListLoads() - loadsBefore; got != 1 {
			t.Fatalf("getChatAdministrators calls = %d, want 1 (the list is loaded once, then read from the cache)", got)
		}
		if got := env.liveMemberLookups(); got != 0 {
			t.Fatalf("live getChatMember calls for members = %d, want 0", got)
		}
		if got := memberDeleteTestFlush(t); got != len(members) {
			t.Fatalf("memberDeleteFlush() = %d, want %d queued", got, len(members))
		}
		env.sendMember(&env.admin, func(m *gotgbot.Message) { m.Text = "admin" })
		if got := memberDeleteTestFlush(t); got != 0 {
			t.Fatalf("an administrator's message was offered (%d written): the warmed list must know the administrator", got)
		}
		if got := env.adminListLoads() - loadsBefore; got != 1 {
			t.Fatalf("getChatAdministrators calls after the administrator = %d, want still 1", got)
		}
	})

	t.Run("concurrent checks behind a cold cache share one load", func(t *testing.T) {
		env := newMemberDeletionEnv(t)
		resetMemberDeletionFailureState(t)
		env.warmableAdminCache()
		loadsBefore := env.adminListLoads()
		members := make([]gotgbot.User, 6)
		for i := range members {
			members[i] = env.member()
		}
		got := make([]memberDeletionAdmin, len(members))
		var wg sync.WaitGroup
		for i := range members {
			wg.Add(1)
			go func() {
				defer wg.Done()
				got[i] = memberDeletionAdminStatus(env.bot, env.chat.Id, members[i].Id)
			}()
		}
		wg.Wait()
		for i, status := range got {
			if status != memberDeletionNotAdmin {
				t.Fatalf("member %d status = %v, want not an admin", i, status)
			}
		}
		if calls := env.adminListLoads() - loadsBefore; calls != 1 {
			t.Fatalf("getChatAdministrators calls = %d, want 1 for six concurrent checks", calls)
		}
		if lookups := env.liveMemberLookups(); lookups != 0 {
			t.Fatalf("live getChatMember calls = %d, want 0", lookups)
		}
	})

	t.Run("a list that cannot be loaded falls back to the live read and is not retried at once", func(t *testing.T) {
		env := newMemberDeletionEnv(t)
		resetMemberDeletionFailureState(t)
		env.warmableAdminCache()
		warms := 0
		mreqSwap(t, &memberDeletionAdminWarm, func(*gotgbot.Bot, int64) { warms++ })
		live := 0
		mreqSwap(t, &memberDeletionMemberLookup,
			func(_ context.Context, _ *gotgbot.Bot, _, userID int64) (gotgbot.MergedChatMember, error) {
				live++
				if live == 3 {
					return gotgbot.MergedChatMember{}, errors.New("connection reset")
				}
				return gotgbot.MergedChatMember{Status: "member", User: gotgbot.User{Id: userID}}, nil
			})
		member := env.member()
		for i, want := range []memberDeletionAdmin{memberDeletionNotAdmin, memberDeletionNotAdmin, memberDeletionAdminUnknown} {
			if got := memberDeletionAdminStatus(env.bot, env.chat.Id, member.Id); got != want {
				t.Fatalf("check %d = %v, want %v", i, got, want)
			}
		}
		if warms != 1 {
			t.Fatalf("list loads = %d, want 1: the chat is in its backoff after a load that stored nothing", warms)
		}
		if live != 3 {
			t.Fatalf("live reads = %d, want 3", live)
		}
		// Once the backoff is over the list is asked for again.
		current := deletionNow()
		mreqSwap(t, &deletionNow, func() time.Time { return current.Add(memberDeletionWarmBackoffFor + time.Second) })
		memberDeletionAdminStatus(env.bot, env.chat.Id, member.Id)
		if warms != 2 {
			t.Fatalf("list loads after the backoff = %d, want 2", warms)
		}
	})

	t.Run("a saturated check keeps the messages and logs once per window", func(t *testing.T) {
		env := newSelfDestructEnv(t, memberSelfDestructRuleDelay)
		resetMemberDeletionFailureState(t)
		hook := captureLockdownLogs(t)
		env.coldAdminCache()
		for range cap(memberDeletionAdminSlots) {
			memberDeletionAdminSlots <- struct{}{}
		}
		t.Cleanup(func() {
			for len(memberDeletionAdminSlots) > 0 {
				<-memberDeletionAdminSlots
			}
		})
		member := env.member()
		for range 5 {
			env.sendMember(&member, func(m *gotgbot.Message) { m.Text = "busy" })
		}
		if got := memberDeleteTestFlush(t); got != 0 {
			t.Fatalf("memberDeleteFlush() = %d, want 0: a saturated check keeps every message", got)
		}
		saturated := func() int {
			count := 0
			for _, line := range memberDeleteLogLines(hook, env.chat.Id) {
				if strings.Contains(line, "saturated") {
					count++
				}
			}
			return count
		}
		if got := saturated(); got != 1 {
			t.Fatalf("saturated lines = %d for five messages, want 1", got)
		}
		current := deletionNow()
		mreqSwap(t, &deletionNow, func() time.Time { return current.Add(memberDeleteWarnEvery + time.Second) })
		env.sendMember(&member, func(m *gotgbot.Message) { m.Text = "later" })
		if got := saturated(); got != 2 {
			t.Fatalf("saturated lines after the window = %d, want 2", got)
		}
	})

	t.Run("failing exemption reads log once per chat per window and keep the messages", func(t *testing.T) {
		env := newSelfDestructEnv(t, memberSelfDestructRuleDelay)
		resetMemberDeletionFailureState(t)
		hook := captureLockdownLogs(t)
		mreqSwap(t, &memberDeletionApprovedLookup,
			func(context.Context, int64, int64) (bool, error) { return false, errors.New("approvals down") })
		member := env.member()
		for range 5 {
			env.sendMember(&member, func(m *gotgbot.Message) { m.Text = "hello" })
		}
		if got := memberDeleteTestFlush(t); got != 0 {
			t.Fatalf("memberDeleteFlush() = %d, want 0: an unreadable approvals answer is exempt", got)
		}
		lines := memberDeleteLogLines(hook, env.chat.Id)
		if len(lines) != 1 {
			t.Fatalf("lines for five failing reads = %d (%v), want 1", len(lines), lines)
		}
		if strings.Contains(lines[0], "hello") {
			t.Fatalf("line %q carries message text", lines[0])
		}
	})
}
