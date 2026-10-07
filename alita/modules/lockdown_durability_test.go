//go:build testtools

package modules

import (
	"strings"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/divkix/Alita_Robot/alita/db"
	"github.com/divkix/Alita_Robot/alita/db/models"
)

// claimJoiner puts a joiner row straight into a claimed state, claimed that long ago,
// the way a worker that stopped mid-call leaves it.
func (e *lockdownEnv) claimJoiner(userID int64, state string, claimedAgo time.Duration) {
	e.t.Helper()
	row := e.joinerRow(userID)
	err := db.DB.Model(&models.LockdownJoiner{}).Where("id = ?", row.ID).
		Updates(map[string]any{"state": state, "claimed_at": time.Now().UTC().Add(-claimedAgo)}).Error
	if err != nil {
		e.t.Fatalf("claim joiner %d: %v", userID, err)
	}
}

// ageClaims moves the claim time of every claimed joiner row of a lockdown back to
// claimedAgo, so a test can pass the two-minute mark without waiting for it.
func ageClaims(t *testing.T, lockdownID uint, claimedAgo time.Duration) {
	t.Helper()
	err := db.DB.Model(&models.LockdownJoiner{}).
		Where("lockdown_id = ? AND state IN ?", lockdownID, []string{models.JoinerStateActing, models.JoinerStateUnbanning}).
		Update("claimed_at", time.Now().UTC().Add(-claimedAgo)).Error
	if err != nil {
		t.Fatalf("age claims of lockdown %d: %v", lockdownID, err)
	}
}

// waitUntil polls cond until it holds or the deadline passes, and fails the test then.
func waitUntil(t *testing.T, within time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(within)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("gave up after %s waiting for %s", within, what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestLockdownLiftResumesAfterRestart(t *testing.T) {
	env := newLockdownEnv(t)
	ld, users := env.lockAndBan("Ann", "Bob", "Cid", "Dee")
	a, b, c, d := users[0], users[1], users[2], users[3]

	env.send(env.admin, "/unlockdown")
	if row := env.freshRow(ld.ID); row.State != models.LockdownStateLifting {
		t.Fatalf("State after /unlockdown = %q, want lifting", row.State)
	}

	// The worker that was unbanning A stopped five minutes ago. B is being unbanned
	// by another replica right now.
	env.claimJoiner(a.Id, models.JoinerStateUnbanning, 5*time.Minute)
	env.claimJoiner(b.Id, models.JoinerStateUnbanning, 0)

	// A fresh replica after the restart.
	env.cycle()

	for _, user := range []struct {
		name string
		id   int64
	}{{"C", c.Id}, {"D", d.Id}, {"A", a.Id}} {
		if row := env.joinerRow(user.id); row.State != models.JoinerStateUnbanned {
			t.Errorf("joiner %s state = %q, want unbanned: its claim was released or never held", user.name, row.State)
		}
	}
	if row := env.joinerRow(b.Id); row.State != models.JoinerStateUnbanning || row.ClaimedAt == nil {
		t.Errorf("B = %+v, want still claimed by the other replica", row)
	}
	if got := len(env.unbansOf(b.Id)); got != 0 {
		t.Errorf("unbanChatMember calls for B = %d, want none while another replica holds the claim", got)
	}
	if row := env.freshRow(ld.ID); row.State != models.LockdownStateLifting {
		t.Errorf("lockdown State = %q, want lifting while B is still claimed", row.State)
	}
	if got := env.messagesWith(staffMarker("lockdown_lift_tally")); got != 0 {
		t.Errorf("tally messages = %d, want none before the lift is finished", got)
	}

	// B's claim passes two minutes: the replica that held it is gone.
	env.claimJoiner(b.Id, models.JoinerStateUnbanning, 3*time.Minute)
	env.cycle()

	if row := env.joinerRow(b.Id); row.State != models.JoinerStateUnbanned {
		t.Errorf("B state = %q, want unbanned once its claim went stale", row.State)
	}
	if row := env.freshRow(ld.ID); row.State != models.LockdownStateLifted {
		t.Errorf("lockdown State = %q, want lifted", row.State)
	}
	for _, user := range users {
		if got := len(env.unbansOf(user.Id)); got != 1 {
			t.Errorf("unbanChatMember calls for user %d = %d, want exactly 1", user.Id, got)
		}
		if member := env.fake.member(env.chat.Id, user.Id); member == nil || member.Status != gotgbot.ChatMemberStatusLeft {
			t.Errorf("user %d in the chat = %+v, want left (able to rejoin)", user.Id, member)
		}
	}
	if got := env.messagesWith(staffMarker("lockdown_lift_tally") + " 4"); got != 1 {
		t.Errorf("tally messages = %d, want exactly 1 saying 4 were unbanned", got)
	}
}

func TestLockdownStopDuringCall(t *testing.T) {
	env := newLockdownEnv(t)
	ld, users := env.lockAndBan("Ann", "Bob")
	env.send(env.admin, "/unlockdown")
	if row := env.freshRow(ld.ID); row.State != models.LockdownStateLifting {
		t.Fatalf("State after /unlockdown = %q, want lifting", row.State)
	}

	previousTick, previousWait := lockdownWorkerTick, lockdownWorkerStopWait
	lockdownWorkerTick = 10 * time.Millisecond
	lockdownWorkerStopWait = 100 * time.Millisecond
	t.Cleanup(func() {
		StopLockdownWorker()
		lockdownWorkerTick, lockdownWorkerStopWait = previousTick, previousWait
	})
	env.fake.setDelay("unbanChatMember", env.chat.Id, time.Second)

	StartLockdownWorker(env.bot)
	waitUntil(t, 5*time.Second, "the worker to be inside its first unban call", func() bool {
		return len(env.fake.requestTimes("unbanChatMember", env.chat.Id)) >= 1
	})

	started := time.Now()
	StopLockdownWorker()
	if took := time.Since(started); took > 600*time.Millisecond {
		t.Errorf("StopLockdownWorker took %s with a call blocked, want it back within its wait", took)
	}

	// The blocked call settles on its own; nothing may be lost by it.
	waitUntil(t, 10*time.Second, "the blocked unban call to settle", func() bool {
		return env.joinerRow(users[0].Id).State != models.JoinerStateUnbanning
	})
	if row := env.freshRow(ld.ID); row.State != models.LockdownStateLifting {
		t.Fatalf("lockdown State = %q, want lifting: one joiner is still banned", row.State)
	}
	if got := env.messagesWith(staffMarker("lockdown_lift_tally")); got != 0 {
		t.Fatalf("tally messages = %d, want none from a worker that was stopped", got)
	}

	// Whatever claim the stopped worker left is two minutes old by now.
	ageClaims(t, ld.ID, 5*time.Minute)
	env.fake.setDelay("unbanChatMember", env.chat.Id, 0)
	StartLockdownWorker(env.bot)

	waitUntil(t, 3*time.Second, "the lift to finish under a new worker", func() bool {
		return env.freshRow(ld.ID).State == models.LockdownStateLifted
	})
	for _, user := range users {
		if row := env.joinerRow(user.Id); row.State != models.JoinerStateUnbanned {
			t.Errorf("joiner %d state = %q, want unbanned", user.Id, row.State)
		}
	}
	if got := env.messagesWith(staffMarker("lockdown_lift_tally") + " 2"); got != 1 {
		t.Errorf("tally messages = %d, want exactly 1 saying 2 were unbanned", got)
	}
}

func TestLockdownBanRetriesAndFailures(t *testing.T) {
	env := newLockdownEnv(t)
	ld := env.lockAsAdmin("raid")

	// Telegram rate-limits R four times in a row: the pacer retries three times, then
	// gives up, and that never counts as an attempt.
	rate := env.newJoiner("Rae")
	env.join(rate, rate, "https://t.me/+abc")
	limited := staffFake429(1)
	env.fake.script("banChatMember", env.chat.Id, limited, limited, limited, limited)

	env.cycle()

	if row := env.joinerRow(rate.Id); row.State != models.JoinerStatePending || row.Attempts != 0 || row.ClaimedAt != nil {
		t.Fatalf("R after a rate-limited cycle = %+v, want pending, 0 attempts and no claim", row)
	}
	env.cycle()
	if row := env.joinerRow(rate.Id); row.State != models.JoinerStateBanned {
		t.Errorf("R state after the next cycle = %q, want banned", row.State)
	}

	// F cannot be banned: the answer is the same three times.
	fail := env.newJoiner("Fay")
	env.join(fail, fail, "https://t.me/+abc")
	refusal := staffFakeError(400, "Bad Request: user is an administrator of the chat <b>")
	env.fake.script("banChatMember", env.chat.Id, refusal, refusal, refusal)

	env.cycle()
	env.cycle()
	if row := env.joinerRow(fail.Id); row.State != models.JoinerStatePending || row.Attempts != 2 {
		t.Fatalf("F after two failed cycles = %+v, want pending with 2 attempts", row)
	}
	env.cycle()

	row := env.joinerRow(fail.Id)
	if row.State != models.JoinerStateBanFailed || row.Attempts != 3 {
		t.Fatalf("F = %+v, want ban_failed after 3 attempts", row)
	}
	if !strings.Contains(row.Detail, "user is an administrator of the chat &lt;b&gt;") {
		t.Errorf("F detail = %q, want Telegram's reason, escaped", row.Detail)
	}
	if got := len(env.bansOf(fail.Id)); got != 3 {
		t.Errorf("banChatMember calls for F = %d, want 3: a failed ban is not tried a fourth time", got)
	}
	env.cycle()
	if got := len(env.bansOf(fail.Id)); got != 3 {
		t.Errorf("banChatMember calls for F after a further cycle = %d, want still 3", got)
	}

	env.askStatus(env.admin)
	env.wantReplyHas(staffMarker("lockdown_status_ban_failed")+" 1", staffMarker("lockdown_status_removed")+" 1")
	if row := env.freshRow(ld.ID); row.State != models.LockdownStateActive {
		t.Errorf("lockdown State = %q, want active: a failed ban never ends the lockdown", row.State)
	}
}
