//go:build testtools

package modules

import (
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/divkix/Alita_Robot/alita/db/lockdown"
	"github.com/divkix/Alita_Robot/alita/db/models"
)

// handEditedPermissions is the locked set with can_send_messages turned back on, the
// way an admin reopening the group by hand would leave it.
var handEditedPermissions = strings.Replace(
	lockdownLockedPermissions, `"can_send_messages":false`, `"can_send_messages":true`, 1)

// freshRow reads one lockdown row by ID or fails the test.
func (e *lockdownEnv) freshRow(id uint) *models.ChatLockdown {
	e.t.Helper()
	row, err := lockdown.GetFresh(id)
	if err != nil || row == nil {
		e.t.Fatalf("GetFresh(%d) = %v, %v, want the row", id, row, err)
	}
	return row
}

// repliesWith returns the texts of the bot's messages to the chat that contain want.
func (e *lockdownEnv) repliesWith(want string) []string {
	var matched []string
	for _, sent := range e.replies() {
		if text, _ := sent.Params["text"].(string); strings.Contains(text, want) {
			matched = append(matched, text)
		}
	}
	return matched
}

func TestLockdownLiftManualChange(t *testing.T) {
	t.Run("a hand edit is replaced and said so", func(t *testing.T) {
		env := newLockdownEnv(t)
		row := env.lockAsAdmin("raid")
		env.fake.setChatPermsRaw(env.chat.Id, handEditedPermissions)

		// The restore must reach Telegram before the lift is recorded.
		var mu sync.Mutex
		var stateAtRestore string
		var liftedByAtRestore *int64
		env.fake.setOnSetPermissions(func(chatID int64) {
			current, err := lockdown.GetFresh(row.ID)
			mu.Lock()
			defer mu.Unlock()
			if err == nil && current != nil {
				stateAtRestore = current.State
				liftedByAtRestore = current.LiftedBy
			}
		})

		env.send(env.admin, "/unlockdown")

		if got := env.fake.chatPermsRaw(env.chat.Id); got != lockdownTestPrePermissions {
			t.Errorf("permissions after the lift = %q, want the pre-lockdown snapshot", got)
		}
		lifted := env.freshRow(row.ID)
		if lifted.State != models.LockdownStateLifted {
			t.Errorf("State = %q, want lifted", lifted.State)
		}
		if !lifted.ManualChange {
			t.Error("ManualChange = false, want true: the live permissions were not the locked set")
		}
		env.wantReplyHas(staffMarker("lockdown_lifted"), staffMarker("lockdown_lifted_manual_change"))

		mu.Lock()
		defer mu.Unlock()
		if stateAtRestore != models.LockdownStateActive || liftedByAtRestore != nil {
			t.Errorf("at the restore call the row was %q with lifted_by %v, want active with none: restore comes before the record",
				stateAtRestore, liftedByAtRestore)
		}
	})

	t.Run("no hand edit", func(t *testing.T) {
		env := newLockdownEnv(t)
		row := env.lockAsAdmin("raid")

		env.send(env.admin, "/unlockdown")

		if lifted := env.freshRow(row.ID); lifted.ManualChange {
			t.Error("ManualChange = true, want false: the live permissions were still the locked set")
		}
		env.wantReplyHas(staffMarker("lockdown_lifted"))
		env.wantReplyLacks(staffMarker("lockdown_lifted_manual_change"))
	})

	t.Run("one flag different is a hand edit", func(t *testing.T) {
		env := newLockdownEnv(t)
		row := env.lockAsAdmin("raid")
		env.fake.setChatPermsRaw(env.chat.Id, strings.Replace(
			lockdownLockedPermissions, `"can_pin_messages":false`, `"can_pin_messages":true`, 1))

		env.send(env.admin, "/unlockdown")

		if lifted := env.freshRow(row.ID); !lifted.ManualChange {
			t.Error("ManualChange = false, want true for one flag turned on")
		}
		env.wantReplyHas(staffMarker("lockdown_lifted_manual_change"))
	})

	t.Run("unreadable live permissions do not block the lift", func(t *testing.T) {
		env := newLockdownEnv(t)
		row := env.lockAsAdmin("raid")
		env.fake.script("getChat", env.chat.Id, staffFakeError(500, "Internal Server Error"))

		env.send(env.admin, "/unlockdown")

		if lifted := env.freshRow(row.ID); lifted.State != models.LockdownStateLifted || lifted.ManualChange {
			t.Errorf("row = %+v, want lifted with manual_change false: a failed read only skips the comparison", lifted)
		}
		if got := env.fake.chatPermsRaw(env.chat.Id); got != lockdownTestPrePermissions {
			t.Errorf("permissions after the lift = %q, want the snapshot", got)
		}
		env.wantReplyLacks(staffMarker("lockdown_lifted_manual_change"))
	})

	t.Run("pre equals locked set", func(t *testing.T) {
		env := newLockdownEnv(t)
		env.fake.setChatPermsRaw(env.chat.Id, lockdownLockedPermissions)
		row := env.lockAsAdmin("raid")

		env.send(env.admin, "/unlockdown")

		sets := env.calls("setChatPermissions")
		if len(sets) != 2 {
			t.Fatalf("setChatPermissions calls = %d, want 2 (lock, restore)", len(sets))
		}
		if got := lockdownParamText(sets[1].Params["permissions"]); got != lockdownLockedPermissions {
			t.Errorf("restore sent %q, want the all-false snapshot", got)
		}
		lifted := env.freshRow(row.ID)
		if lifted.State != models.LockdownStateLifted || lifted.ManualChange {
			t.Errorf("row = %+v, want lifted with manual_change false", lifted)
		}
		env.wantReplyHas(staffMarker("lockdown_lifted"))
		env.wantReplyLacks(staffMarker("lockdown_lifted_manual_change"))
	})

	t.Run("empty snapshot", func(t *testing.T) {
		env := newLockdownEnv(t)
		env.fake.setChatPermsRaw(env.chat.Id, "{}")
		row := env.lockAsAdmin("raid")
		if row.PrePermissions != "{}" {
			t.Fatalf("PrePermissions = %q, want %q", row.PrePermissions, "{}")
		}

		env.send(env.admin, "/unlockdown")

		sets := env.calls("setChatPermissions")
		if len(sets) != 2 {
			t.Fatalf("setChatPermissions calls = %d, want 2 (lock, restore)", len(sets))
		}
		if got := lockdownParamText(sets[1].Params["permissions"]); got != "{}" {
			t.Errorf("restore sent %q, want exactly {}", got)
		}
		if got := env.fake.chatPermsRaw(env.chat.Id); got != "{}" {
			t.Errorf("permissions after the lift = %q, want {}", got)
		}
		env.wantReplyHas(staffMarker("lockdown_lifted"))
	})
}

func TestLockdownLiftRestoreFailure(t *testing.T) {
	env := newLockdownEnv(t)
	row := env.lockAsAdmin("raid")
	env.fake.script("setChatPermissions", env.chat.Id,
		staffFakeError(400, "Bad Request: not enough rights to change chat permissions"))

	env.send(env.admin, "/unlockdown")

	env.wantReplyHas(staffMarker("lockdown_restore_failed"), "not enough rights to change chat permissions")
	stuck := env.freshRow(row.ID)
	if stuck.State != models.LockdownStateActive {
		t.Errorf("State = %q, want active: a failed restore must not end the lockdown", stuck.State)
	}
	if stuck.LiftedBy != nil || stuck.LiftStartedAt != nil || stuck.LiftedAt != nil {
		t.Errorf("lift fields = %v %v %v, want all empty", stuck.LiftedBy, stuck.LiftStartedAt, stuck.LiftedAt)
	}
	if got := env.fake.chatPermsRaw(env.chat.Id); got != lockdownLockedPermissions {
		t.Errorf("permissions = %q, want the group to stay locked", got)
	}
	if lifted := env.repliesWith(staffMarker("lockdown_lifted")); len(lifted) != 0 {
		t.Errorf("replies announcing a lift = %v, want none", lifted)
	}
	if calls := env.fake.callsFor("unbanChatMember"); len(calls) != 0 {
		t.Errorf("unbanChatMember calls = %d, want 0: nobody is unbanned when the restore failed", len(calls))
	}
	if active, err := lockdown.GetActiveFresh(env.chat.Id); err != nil || active == nil {
		t.Errorf("GetActiveFresh = %v, %v, want the lockdown still active", active, err)
	}

	env.send(env.admin, "/unlockdown")

	if got := env.fake.chatPermsRaw(env.chat.Id); got != lockdownTestPrePermissions {
		t.Errorf("permissions after the second /unlockdown = %q, want the snapshot", got)
	}
	lifted := env.freshRow(row.ID)
	if lifted.State != models.LockdownStateLifted || lifted.LiftedBy == nil || *lifted.LiftedBy != env.admin.Id {
		t.Errorf("row = %+v, want lifted by the admin", lifted)
	}
	env.wantReplyHas(staffMarker("lockdown_lifted"))
}

func TestUnlockdownLiftsOnce(t *testing.T) {
	env := newLockdownEnv(t)
	second := gotgbot.User{Id: uniqueLinkOwnerID() + 11, FirstName: "Second"}
	env.fake.setMember(env.chat.Id, second.Id, staffFakeMember{
		Status:             gotgbot.ChatMemberStatusAdministrator,
		CanRestrictMembers: true,
	})
	row := env.lockAsAdmin("raid")

	// Hold both lifts at the restore call until both have arrived, so each has read
	// the lockdown as active before either records the lift.
	var arrived atomic.Int32
	release := make(chan struct{})
	env.fake.setOnSetPermissions(func(int64) {
		if arrived.Add(1) == 2 {
			close(release)
		}
		select {
		case <-release:
		case <-time.After(10 * time.Second):
		}
	})

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i, admin := range []gotgbot.User{env.admin, second} {
		id := env.updateID()
		update := &gotgbot.Update{
			UpdateId: id,
			Message: &gotgbot.Message{
				MessageId: id,
				Date:      1,
				Chat:      env.chat,
				From:      &admin,
				Text:      "/unlockdown",
			},
		}
		wg.Add(1)
		go func(i int, update *gotgbot.Update) {
			defer wg.Done()
			errs[i] = env.dispatcher.ProcessUpdate(env.bot, update, nil)
		}(i, update)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("ProcessUpdate #%d error = %v", i, err)
		}
	}
	if got := arrived.Load(); got != 2 {
		t.Fatalf("restore calls that arrived = %d, want 2: the two lifts did not overlap", got)
	}

	lifts := env.repliesWith(staffMarker("lockdown_lifted"))
	if len(lifts) != 1 {
		t.Fatalf("replies announcing a lift = %d (%v), want exactly 1", len(lifts), lifts)
	}
	losers := env.repliesWith(staffMarker("lockdown_already_lifted"))
	losers = append(losers, env.repliesWith(staffMarker("lockdown_not_active"))...)
	if len(losers) != 1 {
		t.Errorf("replies of the losing lift = %d (%v), want exactly 1", len(losers), losers)
	}

	winner := env.admin.Id
	if strings.Contains(lifts[0], "Second") {
		winner = second.Id
	}
	lifted := env.freshRow(row.ID)
	if lifted.State != models.LockdownStateLifted {
		t.Errorf("State = %q, want lifted", lifted.State)
	}
	if lifted.LiftedBy == nil || *lifted.LiftedBy != winner {
		t.Errorf("LiftedBy = %v, want %d, the admin of the winning reply", lifted.LiftedBy, winner)
	}
	if got := env.fake.chatPermsRaw(env.chat.Id); got != lockdownTestPrePermissions {
		t.Errorf("permissions after the lifts = %q, want the snapshot", got)
	}
}

func TestUnlockdownWithoutLockdown(t *testing.T) {
	t.Run("never locked", func(t *testing.T) {
		env := newLockdownEnv(t)

		env.send(env.admin, "/unlockdown")

		env.wantReplyHas(staffMarker("lockdown_not_active"))
		if calls := env.calls("setChatPermissions"); len(calls) != 0 {
			t.Errorf("setChatPermissions calls = %d, want 0", len(calls))
		}
		if calls := env.calls("getChat"); len(calls) != 0 {
			t.Errorf("getChat calls = %d, want 0", len(calls))
		}
	})

	t.Run("already lifted", func(t *testing.T) {
		env := newLockdownEnv(t)
		env.lockAsAdmin("raid")
		env.send(env.admin, "/unlockdown")
		writesBefore := len(env.calls("setChatPermissions"))

		env.send(env.admin, "/unlockdown")

		env.wantReplyHas(staffMarker("lockdown_not_active"))
		if got := len(env.calls("setChatPermissions")); got != writesBefore {
			t.Errorf("setChatPermissions calls = %d after a second /unlockdown, were %d", got, writesBefore)
		}
	})

	t.Run("lift still unbanning joiners", func(t *testing.T) {
		env := newLockdownEnv(t)
		row := env.lockAsAdmin("raid")
		if began, err := lockdown.BeginLift(row.ID, env.admin.Id, "Ad<b>min", false); err != nil || !began {
			t.Fatalf("setup: BeginLift = %v, %v", began, err)
		}
		seedLockdownJoiner(t, row.ID, env.chat.Id, 9401, models.JoinerStateBanned)
		writesBefore := len(env.calls("setChatPermissions"))

		env.send(env.admin, "/unlockdown")

		env.wantReplyHas(staffMarker("lockdown_lift_in_progress"), "Ad&lt;b&gt;min")
		if got := len(env.calls("setChatPermissions")); got != writesBefore {
			t.Errorf("setChatPermissions calls = %d, were %d: a lift under way must not restore again", got, writesBefore)
		}
		if calls := env.calls("getChat"); len(calls) != 1 {
			t.Errorf("getChat calls = %d, want only the one /lockdown made", len(calls))
		}
	})
}

func TestLockdownLiftRecordFailure(t *testing.T) {
	env := newLockdownEnv(t)
	row := env.lockAsAdmin("raid")

	previous := lockdownBeginLift
	t.Cleanup(func() { lockdownBeginLift = previous })
	lockdownBeginLift = func(uint, int64, string, bool) (bool, error) {
		return false, errors.New("database is down")
	}

	env.send(env.admin, "/unlockdown")

	env.wantReplyHas(staffMarker("lockdown_lift_record_failed"))
	if lifted := env.repliesWith(staffMarker("lockdown_lifted")); len(lifted) != 0 {
		t.Errorf("replies announcing a lift = %v, want none: nothing was recorded", lifted)
	}
	stuck := env.freshRow(row.ID)
	if stuck.State != models.LockdownStateActive || stuck.LiftedBy != nil {
		t.Errorf("row = %+v, want it still active with no lifter", stuck)
	}

	lockdownBeginLift = previous
	env.send(env.admin, "/unlockdown")

	lifted := env.freshRow(row.ID)
	if lifted.State != models.LockdownStateLifted || lifted.LiftedBy == nil || *lifted.LiftedBy != env.admin.Id {
		t.Errorf("row = %+v, want lifted by the admin after the seam was restored", lifted)
	}
	env.wantReplyHas(staffMarker("lockdown_lifted"))
}
