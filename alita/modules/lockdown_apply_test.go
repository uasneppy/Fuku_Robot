//go:build testtools

package modules

import (
	"context"
	"errors"
	"html"
	"strings"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/divkix/Alita_Robot/alita/db/lockdown"
	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/i18n"
)

// TestApplyAutoLockdown pins the one write path of the automatic lockdown triggers: the
// outcomes of applyAutoLockdown and the rows it leaves behind.
func TestApplyAutoLockdown(t *testing.T) {
	t.Run("a threshold is saved and turns its trigger on", func(t *testing.T) {
		env := newLockdownEnv(t)
		raidCleanup(t, env.chat.Id)

		got := applyAutoLockdown(&env.chat, autoLockdownChange{
			Trigger: autoTriggerJoins, SetThreshold: true, Threshold: 20, Enable: true,
		})

		if got.Outcome != autoApplySaved || got.Err != nil || got.ReadErr != nil {
			t.Fatalf("result = %+v, want saved with no error", got)
		}
		if got.Settings == nil || got.Settings.JoinThreshold != 20 || !got.Settings.JoinEnabled {
			t.Errorf("settings read back = %+v, want joins on at 20", got.Settings)
		}
		env.wantAutoRow(true, 20, true, 30)
	})

	t.Run("a switch is saved", func(t *testing.T) {
		env := newLockdownEnv(t)
		raidCleanup(t, env.chat.Id)

		got := applyAutoLockdown(&env.chat, autoLockdownChange{Trigger: autoTriggerFlood, Enable: false})

		if got.Outcome != autoApplySaved || got.Settings == nil || got.Settings.FloodEnabled {
			t.Fatalf("result = %+v, want saved with flood off", got)
		}
		env.wantAutoRow(true, 15, false, 30)
	})

	t.Run("a threshold outside 3..500 is a range refusal and changes nothing", func(t *testing.T) {
		for _, n := range []int{2, 501} {
			env := newLockdownEnv(t)
			raidCleanup(t, env.chat.Id)
			seeded := applyAutoLockdown(&env.chat, autoLockdownChange{
				Trigger: autoTriggerAI, SetThreshold: true, Threshold: 7, Enable: true,
			})
			if seeded.Outcome != autoApplySaved {
				t.Fatalf("setup: result = %+v, want saved", seeded)
			}

			got := applyAutoLockdown(&env.chat, autoLockdownChange{
				Trigger: autoTriggerAI, SetThreshold: true, Threshold: n, Enable: true,
			})

			if got.Outcome != autoApplyRange {
				t.Errorf("threshold %d: outcome = %v, want autoApplyRange", n, got.Outcome)
			}
			env.wantAutoAIRow(true, 7)
		}
	})

	t.Run("a basic group is refused and writes nothing", func(t *testing.T) {
		env := newLockdownEnv(t)
		raidCleanup(t, env.chat.Id)
		env.chat.Type = "group"

		got := applyAutoLockdown(&env.chat, autoLockdownChange{
			Trigger: autoTriggerJoins, SetThreshold: true, Threshold: 20, Enable: true,
		})

		if got.Outcome != autoApplyBasicGroup {
			t.Errorf("outcome = %v, want autoApplyBasicGroup", got.Outcome)
		}
		env.wantNoAutoRow()
	})

	t.Run("a failed write is a save failure carrying the error", func(t *testing.T) {
		env := newLockdownEnv(t)
		raidCleanup(t, env.chat.Id)
		previous := autolockdownWrite
		autolockdownWrite = func(int64, autoLockdownChange) error { return errors.New("database is gone") }
		t.Cleanup(func() { autolockdownWrite = previous })

		got := applyAutoLockdown(&env.chat, autoLockdownChange{Trigger: autoTriggerJoins, Enable: false})

		if got.Outcome != autoApplySaveFailed || got.Err == nil {
			t.Errorf("result = %+v, want autoApplySaveFailed with Err set", got)
		}
		env.wantNoAutoRow()
	})

	t.Run("a failed readback after the write is saved with ReadErr", func(t *testing.T) {
		env := newLockdownEnv(t)
		raidCleanup(t, env.chat.Id)
		previous := raidSettingsLookup
		raidSettingsLookup = func(context.Context, int64) (*models.ChatRaidDetection, error) {
			return nil, errors.New("settings unreadable")
		}
		t.Cleanup(func() { raidSettingsLookup = previous })

		got := applyAutoLockdown(&env.chat, autoLockdownChange{Trigger: autoTriggerJoins, Enable: false})

		if got.Outcome != autoApplySaved || got.ReadErr == nil || got.Settings != nil {
			t.Errorf("result = %+v, want saved with ReadErr set and no settings", got)
		}
		env.wantAutoRow(false, 15, true, 30)
	})
}

// lockdownApplyNoCalls fails when the fake saw a Telegram call that reads or changes
// the group's permissions or members, in any chat.
func lockdownApplyNoCalls(t *testing.T, env *lockdownEnv, why string) {
	t.Helper()
	if got := len(env.fake.callsFor("getChat")) + env.lockdownWriteCalls(); got != 0 {
		t.Errorf("%d Telegram call(s) were made, want none: %s", got, why)
	}
}

// lockdownApplyDrainWake empties the worker's wake channel and reports whether it held
// a token.
func lockdownApplyDrainWake() bool {
	select {
	case <-lockdownWake:
		return true
	default:
		return false
	}
}

// TestApplyLockdownLift pins the shared lift: it lifts only through liftLockdownRow and
// every outcome has the text /unlockdown replies.
func TestApplyLockdownLift(t *testing.T) {
	tr := func(t *testing.T) *i18n.Translator {
		t.Helper()
		return i18n.MustNewTranslator("en")
	}

	t.Run("no lockdown makes no Telegram call", func(t *testing.T) {
		env := newLockdownEnv(t)

		got := applyLockdownLift(context.Background(), env.bot, env.chat.Id, &env.admin)

		if got.Outcome != liftApplyNotActive || got.Row != nil {
			t.Errorf("result = %+v, want liftApplyNotActive with no row", got)
		}
		lockdownApplyNoCalls(t, env, "there is no lockdown to lift")
		if text := lockdownLiftApplyText(tr(t), got, env.admin.Id); !strings.Contains(text, staffMarker("lockdown_not_active")) {
			t.Errorf("text = %q, want %q", text, staffMarker("lockdown_not_active"))
		}
	})

	t.Run("an unreadable state makes no Telegram call and lifts nothing", func(t *testing.T) {
		env := newLockdownEnv(t)
		row := env.lockAsAdmin("raid")
		writesBefore := len(env.calls("setChatPermissions"))
		previous := lockdownCurrentLookup
		lockdownCurrentLookup = func(int64) (*models.ChatLockdown, error) { return nil, errors.New("database is down") }
		t.Cleanup(func() { lockdownCurrentLookup = previous })

		got := applyLockdownLift(context.Background(), env.bot, env.chat.Id, &env.admin)

		if got.Outcome != liftApplyStateFailed {
			t.Errorf("outcome = %v, want liftApplyStateFailed", got.Outcome)
		}
		if after := len(env.calls("setChatPermissions")); after != writesBefore {
			t.Errorf("setChatPermissions calls = %d, were %d: an unreadable state must not lift", after, writesBefore)
		}
		if still := env.freshRow(row.ID); still.State != models.LockdownStateActive {
			t.Errorf("State = %q, want active", still.State)
		}
		if text := lockdownLiftApplyText(tr(t), got, env.admin.Id); !strings.Contains(text, staffMarker("lockdown_state_failed")) {
			t.Errorf("text = %q, want %q", text, staffMarker("lockdown_state_failed"))
		}
	})

	t.Run("an active lockdown is restored, recorded and lifted once", func(t *testing.T) {
		env := newLockdownEnv(t)
		row := env.lockAsAdmin("raid")
		actor := gotgbot.User{Id: env.admin.Id, FirstName: strings.Repeat("N", 100)}

		got := applyLockdownLift(context.Background(), env.bot, env.chat.Id, &actor)

		if got.Outcome != liftApplyDone || got.Row == nil || got.Row.ID != row.ID {
			t.Fatalf("result = %+v, want liftApplyDone for lockdown %d", got, row.ID)
		}
		if got.Lift.Outcome != lockdownLiftDone || !got.Lift.Finished {
			t.Errorf("Lift = %+v, want done and finished: no joiner was left to unban", got.Lift)
		}
		if n := len([]rune(got.ActorName)); n != lockdownNameMaxRunes {
			t.Errorf("ActorName has %d runes, want it capped at %d", n, lockdownNameMaxRunes)
		}
		if perms := env.fake.chatPermsRaw(env.chat.Id); perms != lockdownTestPrePermissions {
			t.Errorf("permissions after the lift = %q, want the pre-lockdown snapshot", perms)
		}
		lifted := env.freshRow(row.ID)
		if lifted.State != models.LockdownStateLifted || lifted.LiftedBy == nil || *lifted.LiftedBy != actor.Id {
			t.Errorf("row = %+v, want lifted by the actor", lifted)
		}
		if lifted.LiftedByName != got.ActorName {
			t.Errorf("LiftedByName = %q, want the capped name %q", lifted.LiftedByName, got.ActorName)
		}
		text := lockdownLiftApplyText(tr(t), got, actor.Id)
		if want := lockdownLiftedText(tr(t), actor.Id, got.ActorName, got.Lift); text != want || !strings.Contains(text, staffMarker("lockdown_lifted")) {
			t.Errorf("text = %q, want the lifted notice %q", text, want)
		}

		writesBefore := len(env.calls("setChatPermissions"))
		again := applyLockdownLift(context.Background(), env.bot, env.chat.Id, &actor)
		if again.Outcome != liftApplyNotActive {
			t.Errorf("second outcome = %v, want liftApplyNotActive", again.Outcome)
		}
		if after := len(env.calls("setChatPermissions")); after != writesBefore {
			t.Errorf("setChatPermissions calls = %d after a second lift, were %d", after, writesBefore)
		}
	})

	t.Run("a lift under way is reported and not repeated", func(t *testing.T) {
		env := newLockdownEnv(t)
		row := env.lockAsAdmin("raid")
		if began, err := lockdown.BeginLift(row.ID, env.admin.Id, "Ad<b>min", false); err != nil || !began {
			t.Fatalf("setup: BeginLift = %v, %v", began, err)
		}
		seedLockdownJoiner(t, row.ID, env.chat.Id, 9601, models.JoinerStateBanned)
		writesBefore := len(env.calls("setChatPermissions"))

		got := applyLockdownLift(context.Background(), env.bot, env.chat.Id, &env.admin)

		if got.Outcome != liftApplyInProgress || got.Row == nil || got.Row.State != models.LockdownStateLifting {
			t.Fatalf("result = %+v, want liftApplyInProgress with the lifting row", got)
		}
		if after := len(env.calls("setChatPermissions")); after != writesBefore {
			t.Errorf("setChatPermissions calls = %d, were %d: a lift under way must not restore again", after, writesBefore)
		}
		text := lockdownLiftApplyText(tr(t), got, env.admin.Id)
		if !strings.Contains(text, staffMarker("lockdown_lift_in_progress")) || !strings.Contains(text, "Ad&lt;b&gt;min") {
			t.Errorf("text = %q, want the in-progress notice naming the escaped lifter", text)
		}
	})

	t.Run("a refused restore leaves the lockdown active", func(t *testing.T) {
		env := newLockdownEnv(t)
		row := env.lockAsAdmin("raid")
		env.fake.script("setChatPermissions", env.chat.Id,
			staffFakeError(400, "Bad Request: not enough rights to change chat permissions"))

		got := applyLockdownLift(context.Background(), env.bot, env.chat.Id, &env.admin)

		if got.Outcome != liftApplyRestoreFailed {
			t.Fatalf("outcome = %v, want liftApplyRestoreFailed", got.Outcome)
		}
		if still := env.freshRow(row.ID); still.State != models.LockdownStateActive || still.LiftedBy != nil {
			t.Errorf("row = %+v, want it active with no lifter", still)
		}
		if perms := env.fake.chatPermsRaw(env.chat.Id); perms != lockdownLockedPermissions {
			t.Errorf("permissions = %q, want the group to stay locked", perms)
		}
		text := lockdownLiftApplyText(tr(t), got, env.admin.Id)
		if !strings.Contains(text, staffMarker("lockdown_restore_failed")) || strings.Contains(text, "not enough rights") {
			t.Errorf("text = %q, want the plain restore refusal", text)
		}
	})

	t.Run("a failed record leaves the lockdown active", func(t *testing.T) {
		env := newLockdownEnv(t)
		row := env.lockAsAdmin("raid")
		previous := lockdownBeginLift
		lockdownBeginLift = func(uint, int64, string, bool) (bool, error) { return false, errors.New("database is down") }
		t.Cleanup(func() { lockdownBeginLift = previous })

		got := applyLockdownLift(context.Background(), env.bot, env.chat.Id, &env.admin)

		if got.Outcome != liftApplyRecordFailed {
			t.Fatalf("outcome = %v, want liftApplyRecordFailed", got.Outcome)
		}
		if still := env.freshRow(row.ID); still.State != models.LockdownStateActive || still.LiftedBy != nil {
			t.Errorf("row = %+v, want it active with no lifter", still)
		}
		if text := lockdownLiftApplyText(tr(t), got, env.admin.Id); !strings.Contains(text, staffMarker("lockdown_lift_record_failed")) {
			t.Errorf("text = %q, want %q", text, staffMarker("lockdown_lift_record_failed"))
		}
	})

	t.Run("a lift another caller recorded first is lost", func(t *testing.T) {
		env := newLockdownEnv(t)
		env.lockAsAdmin("raid")
		previous := lockdownBeginLift
		lockdownBeginLift = func(uint, int64, string, bool) (bool, error) { return false, nil }
		t.Cleanup(func() { lockdownBeginLift = previous })

		got := applyLockdownLift(context.Background(), env.bot, env.chat.Id, &env.admin)

		if got.Outcome != liftApplyLost {
			t.Fatalf("outcome = %v, want liftApplyLost", got.Outcome)
		}
		if text := lockdownLiftApplyText(tr(t), got, env.admin.Id); !strings.Contains(text, staffMarker("lockdown_already_lifted")) {
			t.Errorf("text = %q, want %q", text, staffMarker("lockdown_already_lifted"))
		}
	})
}

// TestLockdownAnnounceLift pins the one public lift notice that every button lift shares.
func TestLockdownAnnounceLift(t *testing.T) {
	t.Run("one notice in the locked chat and the worker is woken", func(t *testing.T) {
		env := newLockdownEnv(t)
		row := env.lockAsAdmin("raid")
		before := len(env.replies())
		lockdownApplyDrainWake()

		lockdownAnnounceLift(env.bot, row, env.admin.Id, "Ad<b>min", lockdownLiftResult{})

		replies := env.replies()
		if len(replies) != before+1 {
			t.Fatalf("%d messages in the locked chat, want %d: exactly one notice", len(replies), before+1)
		}
		text, _ := replies[len(replies)-1].Params["text"].(string)
		if !strings.Contains(text, staffMarker("lockdown_lifted")) || !strings.Contains(text, "Ad&lt;b&gt;min") {
			t.Errorf("notice = %q, want the lifted notice naming the escaped lifter", text)
		}
		env.wantSentOnlyTo(env.chat.Id)
		if !lockdownApplyDrainWake() {
			t.Error("the lockdown worker was not woken")
		}
	})

	t.Run("a failed post is only logged", func(t *testing.T) {
		env := newLockdownEnv(t)
		row := env.lockAsAdmin("raid")
		before := len(env.replies())
		sendsBefore := len(env.calls("sendMessage"))
		env.fake.script("sendMessage", env.chat.Id, staffFakeError(400, "Bad Request: chat not found"))
		hook := captureLockdownLogs(t)

		lockdownAnnounceLift(env.bot, row, env.admin.Id, "Admin", lockdownLiftResult{})

		if got := len(env.calls("sendMessage")) - sendsBefore; got != 1 {
			t.Errorf("%d send attempts, want exactly 1", got)
		}
		if got := len(env.replies()); got != before {
			t.Errorf("%d messages in the chat, want %d: the failed post delivered nothing", got, before)
		}
		wantLockdownLogged(t, hook, "could not be posted")
	})
}

// TestLockdownStatusHead pins the head lines /lockdownstatus and the settings menu share.
func TestLockdownStatusHead(t *testing.T) {
	withStaffLocale(t)
	tr := i18n.MustNewTranslator("en")
	locked := time.Date(2026, time.October, 30, 12, 30, 0, 0, time.UTC)
	manual := func() *models.ChatLockdown {
		return &models.ChatLockdown{
			StartedBy:     77,
			StartedByName: "Sta<r>ter",
			Reason:        "<" + strings.Repeat("a", 299),
			CreatedAt:     locked.Add(-time.Minute),
			LockedAt:      &locked,
		}
	}

	t.Run("a manual lockdown shows the starter and the whole reason when the cap is 0", func(t *testing.T) {
		lines := lockdownStatusHead(tr, manual(), 0)

		if len(lines) != 2 {
			t.Fatalf("lines = %q, want the active line and the reason line", lines)
		}
		if !strings.Contains(lines[0], staffMarker("lockdown_status_active")) ||
			!strings.Contains(lines[0], lockdownTime(locked)) || !strings.Contains(lines[0], "Sta&lt;r&gt;ter") {
			t.Errorf("active line = %q, want since %q and the escaped starter", lines[0], lockdownTime(locked))
		}
		wantReason := html.EscapeString("<" + strings.Repeat("a", 299))
		if !strings.Contains(lines[1], staffMarker("lockdown_reason_line")) || !strings.Contains(lines[1], wantReason) ||
			strings.Contains(lines[1], "…") {
			t.Errorf("reason line = %q, want the full escaped reason and no cut mark", lines[1])
		}
	})

	t.Run("a positive cap cuts the reason to that many runes plus a mark, escaped", func(t *testing.T) {
		lines := lockdownStatusHead(tr, manual(), 200)

		if len(lines) != 2 {
			t.Fatalf("lines = %q, want the active line and the reason line", lines)
		}
		wantReason := html.EscapeString("<"+strings.Repeat("a", 199)) + "…"
		if !strings.Contains(lines[1], wantReason) {
			t.Errorf("reason line = %q, want it to hold %q", lines[1], wantReason)
		}
		if strings.Contains(lines[1], strings.Repeat("a", 200)) {
			t.Errorf("reason line = %q, want the reason cut at 200 runes", lines[1])
		}
	})

	t.Run("a reason within the cap is left whole", func(t *testing.T) {
		row := manual()
		row.Reason = "short"

		lines := lockdownStatusHead(tr, row, 200)

		if len(lines) != 2 || !strings.Contains(lines[1], "short") || strings.Contains(lines[1], "…") {
			t.Errorf("lines = %q, want the short reason with no cut mark", lines)
		}
	})

	t.Run("an automatic lockdown names its trigger and has no reason line", func(t *testing.T) {
		row := manual()
		row.TriggerKind = models.LockdownTriggerJoinSurge
		row.TriggerCount = 18
		row.TriggerWindow = 60

		lines := lockdownStatusHead(tr, row, 200)

		if len(lines) != 2 {
			t.Fatalf("lines = %q, want the automatic line and the trigger sentence", lines)
		}
		if !strings.Contains(lines[0], staffMarker("lockdown_status_active_auto")) || !strings.Contains(lines[0], lockdownTime(locked)) {
			t.Errorf("first line = %q, want the automatic active line with the time", lines[0])
		}
		if !strings.Contains(lines[1], staffMarker("lockdown_trigger_join_surge")) {
			t.Errorf("second line = %q, want the join surge sentence", lines[1])
		}
		for _, line := range lines {
			if strings.Contains(line, staffMarker("lockdown_reason_line")) || strings.Contains(line, "aaaa") {
				t.Errorf("line %q carries the reason of an automatic lockdown, want none", line)
			}
		}
	})

	t.Run("an unconfirmed row says so and is timed from its creation", func(t *testing.T) {
		row := manual()
		row.LockedAt = nil

		lines := lockdownStatusHead(tr, row, 0)

		if len(lines) != 3 {
			t.Fatalf("lines = %q, want active, reason and unconfirmed", lines)
		}
		if !strings.Contains(lines[0], lockdownTime(row.CreatedAt)) {
			t.Errorf("active line = %q, want it timed from %q", lines[0], lockdownTime(row.CreatedAt))
		}
		if !strings.Contains(lines[2], staffMarker("lockdown_status_unconfirmed")) {
			t.Errorf("last line = %q, want %q", lines[2], staffMarker("lockdown_status_unconfirmed"))
		}
	})
}
