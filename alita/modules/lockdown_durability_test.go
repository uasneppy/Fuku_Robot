//go:build testtools

package modules

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/divkix/Alita_Robot/alita/db"
	"github.com/divkix/Alita_Robot/alita/db/lockdown"
	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/utils/cache"
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
	// The tally follows the record that the lift finished.
	waitUntil(t, 3*time.Second, "the tally to be posted", func() bool {
		return env.messagesWith(staffMarker("lockdown_lift_tally")+" 2") >= 1
	})
	StopLockdownWorker()
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

// ageLockdown moves created_at and updated_at of a lockdown row back by the given time
// with a direct update that leaves updated_at alone afterwards, so a test can pass the
// one-minute mark without waiting for it.
func ageLockdown(t *testing.T, id uint, by time.Duration) {
	t.Helper()
	at := time.Now().UTC().Add(-by)
	err := db.DB.Model(&models.ChatLockdown{}).Where("id = ?", id).
		UpdateColumns(map[string]any{"created_at": at, "updated_at": at}).Error
	if err != nil {
		t.Fatalf("age lockdown %d: %v", id, err)
	}
}

// seedUnconfirmed stores an active lockdown whose lock Telegram never confirmed (the
// bot stopped between storing it and the lock call), made that long ago.
func (e *lockdownEnv) seedUnconfirmed(age time.Duration) *models.ChatLockdown {
	e.t.Helper()
	row := &models.ChatLockdown{
		ChatID:            e.chat.Id,
		TriggerKind:       models.LockdownTriggerManual,
		Reason:            "half started",
		StartedBy:         e.admin.Id,
		StartedByName:     "Admin",
		PrePermissions:    lockdownTestPrePermissions,
		LockedPermissions: lockdownLockedPermissions,
	}
	started, err := lockdown.Start(row)
	if err != nil || !started {
		e.t.Fatalf("seed unconfirmed lockdown = %v, %v, want true, nil", started, err)
	}
	ageLockdown(e.t, row.ID, age)
	return row
}

// lockdownRows returns every lockdown row of the env's chat, oldest first.
func (e *lockdownEnv) lockdownRows() []models.ChatLockdown {
	e.t.Helper()
	var rows []models.ChatLockdown
	if err := db.DB.Where("chat_id = ?", e.chat.Id).Order("id").Find(&rows).Error; err != nil {
		e.t.Fatalf("read lockdown rows: %v", err)
	}
	return rows
}

func TestLockdownSettlesUnconfirmedLock(t *testing.T) {
	t.Run("lock took effect", func(t *testing.T) {
		env := newLockdownEnv(t)
		row := env.seedUnconfirmed(2 * time.Minute)
		env.fake.setChatPermsRaw(env.chat.Id, lockdownLockedPermissions)
		sent, locks := len(env.replies()), len(env.calls("setChatPermissions"))

		env.cycle()

		if settled := env.freshRow(row.ID); settled.LockedAt == nil || settled.State != models.LockdownStateActive {
			t.Errorf("row = %+v, want active and confirmed: the live permissions are the locked set", settled)
		}
		if got := len(env.replies()); got != sent {
			t.Errorf("settling posted %d message(s), want none: nothing is announced", got-sent)
		}
		if got := len(env.calls("setChatPermissions")); got != locks {
			t.Errorf("setChatPermissions calls = %d, want %d: settling never writes permissions", got, locks)
		}
	})

	t.Run("lock never applied", func(t *testing.T) {
		env := newLockdownEnv(t)
		env.seedUnconfirmed(2 * time.Minute)
		sent, locks := len(env.replies()), len(env.calls("setChatPermissions"))

		env.cycle()

		if rows := env.lockdownRows(); len(rows) != 0 {
			t.Errorf("lockdown rows = %+v, want none: the lock never took effect", rows)
		}
		if active, err := lockdown.GetActiveFresh(env.chat.Id); err != nil || active != nil {
			t.Errorf("GetActiveFresh = %v, %v, want nil, nil", active, err)
		}
		if got := len(env.replies()); got != sent {
			t.Errorf("settling posted %d message(s), want none", got-sent)
		}
		if got := len(env.calls("setChatPermissions")); got != locks {
			t.Errorf("setChatPermissions calls = %d, want %d", got, locks)
		}
	})

	t.Run("permissions unreadable", func(t *testing.T) {
		env := newLockdownEnv(t)
		row := env.seedUnconfirmed(2 * time.Minute)
		env.fake.setChatPermsRaw(env.chat.Id, lockdownLockedPermissions)
		before := env.freshRow(row.ID).UpdatedAt
		env.fake.script("getChat", env.chat.Id, staffFakeError(500, "Internal Server Error"))

		env.cycle()

		kept := env.freshRow(row.ID)
		if kept.LockedAt != nil || kept.State != models.LockdownStateActive {
			t.Fatalf("row = %+v, want it left unconfirmed when the permissions cannot be read", kept)
		}
		if !kept.UpdatedAt.After(before) {
			t.Errorf("updated_at = %v, want it moved past %v so the row is retried after another grace period", kept.UpdatedAt, before)
		}

		chatReads := len(env.calls("getChat"))
		env.cycle()
		if got := len(env.calls("getChat")); got != chatReads {
			t.Errorf("getChat calls = %d after an immediate second cycle, want %d: the row is younger than the grace again", got, chatReads)
		}

		ageLockdown(t, row.ID, 2*time.Minute)
		env.cycle()
		if settled := env.freshRow(row.ID); settled.LockedAt == nil {
			t.Errorf("row = %+v, want it confirmed once the permissions are readable and the row is aged again", settled)
		}
	})

	t.Run("younger than a minute", func(t *testing.T) {
		env := newLockdownEnv(t)
		row := env.seedUnconfirmed(30 * time.Second)
		reads := len(env.calls("getChat"))
		before := env.freshRow(row.ID)

		env.cycle()

		after := env.freshRow(row.ID)
		if after.LockedAt != nil || !after.UpdatedAt.Equal(before.UpdatedAt) {
			t.Errorf("row = %+v, want it untouched: another replica may still be locking", after)
		}
		if got := len(env.calls("getChat")); got != reads {
			t.Errorf("getChat calls = %d, want %d: a young row is not read", got, reads)
		}
	})

	t.Run("lockdown meets a row whose lock never applied", func(t *testing.T) {
		env := newLockdownEnv(t)
		old := env.seedUnconfirmed(2 * time.Minute)
		current := strings.Replace(lockdownTestPrePermissions, `"can_send_polls":false`, `"can_send_polls":true`, 1)
		env.fake.setChatPermsRaw(env.chat.Id, current)

		env.send(env.admin, "/lockdown fresh start")

		rows := env.lockdownRows()
		if len(rows) != 1 || rows[0].ID == old.ID {
			t.Fatalf("lockdown rows = %+v, want only a new one: the old one never took effect", rows)
		}
		if rows[0].LockedAt == nil || rows[0].State != models.LockdownStateActive || rows[0].PrePermissions != current {
			t.Errorf("new row = %+v, want active, confirmed, with the current permissions as its snapshot", rows[0])
		}
		env.wantReplyHas(staffMarker("lockdown_started"))
		if got := env.fake.chatPermsRaw(env.chat.Id); got != lockdownLockedPermissions {
			t.Errorf("permissions = %q, want the locked set", got)
		}
	})

	t.Run("lockdown meets a row whose lock took effect", func(t *testing.T) {
		env := newLockdownEnv(t)
		old := env.seedUnconfirmed(2 * time.Minute)
		env.fake.setChatPermsRaw(env.chat.Id, lockdownLockedPermissions)
		locks := len(env.calls("setChatPermissions"))

		env.send(env.admin, "/lockdown again")

		env.wantReplyHas(staffMarker("lockdown_already_active"))
		rows := env.lockdownRows()
		if len(rows) != 1 || rows[0].ID != old.ID || rows[0].LockedAt == nil {
			t.Errorf("lockdown rows = %+v, want the old row, now confirmed", rows)
		}
		if got := len(env.calls("setChatPermissions")); got != locks {
			t.Errorf("setChatPermissions calls = %d, want %d: nothing is locked twice", got, locks)
		}
	})

	t.Run("lockdown meets a row it cannot read the permissions for", func(t *testing.T) {
		env := newLockdownEnv(t)
		old := env.seedUnconfirmed(2 * time.Minute)
		env.fake.script("getChat", env.chat.Id, staffFakeError(500, "Internal Server Error"))

		env.send(env.admin, "/lockdown again")

		env.wantReplyHas(staffMarker("lockdown_permissions_unreadable"))
		if rows := env.lockdownRows(); len(rows) != 1 || rows[0].ID != old.ID {
			t.Errorf("lockdown rows = %+v, want the old row kept", rows)
		}
	})

	t.Run("lockdown meets a young unconfirmed row", func(t *testing.T) {
		env := newLockdownEnv(t)
		young := env.seedUnconfirmed(10 * time.Second)
		reads := len(env.calls("getChat"))

		env.send(env.admin, "/lockdown again")

		env.wantReplyHas(staffMarker("lockdown_already_active"))
		rows := env.lockdownRows()
		if len(rows) != 1 || rows[0].ID != young.ID || rows[0].LockedAt != nil {
			t.Errorf("lockdown rows = %+v, want the young row untouched: its own /lockdown may still be locking", rows)
		}
		if got := len(env.calls("getChat")); got != reads {
			t.Errorf("getChat calls = %d, want %d", got, reads)
		}
	})
}

func TestLockdownSurvivesRedisFlush(t *testing.T) {
	env := newLockdownEnv(t)
	ld := env.lockAsAdmin("raid")

	if err := cache.GetRedisClient().FlushAll(context.Background()).Err(); err != nil {
		t.Fatalf("flush Redis: %v", err)
	}

	first := env.newJoiner("Ann")
	env.join(first, first, "https://t.me/+abc")
	env.cycle()
	if row := env.joinerRow(first.Id); row.State != models.JoinerStateBanned {
		t.Fatalf("joiner after a Redis flush: state = %q, want banned", row.State)
	}
	env.send(env.admin, "/lockdownstatus")
	env.wantReplyHas(staffMarker("lockdown_status_active"), staffMarker("lockdown_status_removed")+" 1")

	// No Redis at all.
	marshal, manager, client := cache.GetCacheState()
	cache.SetCacheState(nil, nil, nil)
	t.Cleanup(func() { cache.SetCacheState(marshal, manager, client) })

	second := env.newJoiner("Bob")
	env.join(second, second, "https://t.me/+abc")
	if row := env.joinerRow(second.Id); row.State != models.JoinerStatePending {
		t.Fatalf("joiner without Redis: state = %q, want pending: the guard never needs Redis", row.State)
	}
	env.cycle()
	if row := env.joinerRow(second.Id); row.State != models.JoinerStateBanned {
		t.Fatalf("joiner without Redis: state = %q, want banned", row.State)
	}
	if got := len(env.bansOf(second.Id)); got != 1 {
		t.Errorf("banChatMember calls for the joiner without Redis = %d, want 1", got)
	}
	env.send(env.admin, "/lockdownstatus")
	env.wantReplyHas(staffMarker("lockdown_status_active"), staffMarker("lockdown_status_removed")+" 2")

	env.send(env.admin, "/unlockdown")
	if got := env.fake.chatPermsRaw(env.chat.Id); got != lockdownTestPrePermissions {
		t.Errorf("permissions after /unlockdown = %q, want the pre-lockdown snapshot", got)
	}
	env.cycle()

	for _, user := range []gotgbot.User{first, second} {
		if row := env.joinerRow(user.Id); row.State != models.JoinerStateUnbanned {
			t.Errorf("joiner %d state = %q, want unbanned", user.Id, row.State)
		}
	}
	if row := env.freshRow(ld.ID); row.State != models.LockdownStateLifted {
		t.Errorf("lockdown State = %q, want lifted", row.State)
	}
	if got := env.messagesWith(staffMarker("lockdown_lift_tally") + " 2"); got != 1 {
		t.Errorf("tally messages = %d, want exactly 1 saying 2 were unbanned", got)
	}
}

func TestLockdownNeverLiftsOnItsOwn(t *testing.T) {
	env := newLockdownEnv(t)
	ld := env.lockAsAdmin("long raid")
	longAgo := time.Now().UTC().Add(-400 * 24 * time.Hour)
	err := db.DB.Model(&models.ChatLockdown{}).Where("id = ?", ld.ID).
		UpdateColumns(map[string]any{"created_at": longAgo, "locked_at": longAgo, "updated_at": longAgo}).Error
	if err != nil {
		t.Fatalf("age the lockdown: %v", err)
	}
	joiner := seedLockdownJoiner(t, ld.ID, env.chat.Id, 9101, models.JoinerStateBanned)
	err = db.DB.Model(&models.LockdownJoiner{}).Where("id = ?", joiner.ID).
		UpdateColumn("ban_until", time.Now().Add(-70*24*time.Hour).Unix()).Error
	if err != nil {
		t.Fatalf("expire the ban: %v", err)
	}

	locks := len(env.calls("setChatPermissions"))
	unbans := len(env.calls("unbanChatMember"))
	sent := len(env.replies())

	for range 20 {
		env.cycle()
	}

	row := env.freshRow(ld.ID)
	if row.State != models.LockdownStateActive || row.LiftedAt != nil || row.LiftedBy != nil {
		t.Errorf("lockdown = %+v after 20 cycles, want still active and never lifted", row)
	}
	if got := len(env.calls("setChatPermissions")); got != locks {
		t.Errorf("setChatPermissions calls = %d, want %d: nothing restores permissions on its own", got, locks)
	}
	if got := len(env.calls("unbanChatMember")); got != unbans {
		t.Errorf("unbanChatMember calls = %d, want %d: an expired ban is not a lift", got, unbans)
	}
	if got := len(env.replies()); got != sent {
		t.Errorf("messages = %d new, want none", got-sent)
	}
	if got := env.joinerRow(9101).State; got != models.JoinerStateBanned {
		t.Errorf("joiner state = %q, want banned", got)
	}

	env.askStatus(env.admin)
	env.wantReplyHas(staffMarker("lockdown_status_active"))
}
