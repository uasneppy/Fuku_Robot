//go:build testtools

package modules

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// wantOneNoticeIn fails unless exactly one message reached the env's chat and returns it.
func (e *lockdownEnv) wantOneNoticeIn() string {
	e.t.Helper()
	sent := e.replies()
	if len(sent) != 1 {
		e.t.Fatalf("%d messages reached the locked group, want exactly 1", len(sent))
	}
	return fmt.Sprint(sent[0].Params["text"])
}

func TestRaidAutoNotice(t *testing.T) {
	t.Run("surge notice", func(t *testing.T) {
		env := newLockdownEnv(t)
		linkLockdownStaffGroup(env)
		raidCleanup(t, env.chat.Id)

		env.selfJoins(15)
		if row := env.activeLockdown(); row == nil || row.LockedAt == nil {
			t.Fatalf("setup: active lockdown = %+v, want one confirmed by the join surge", row)
		}

		text := env.wantOneNoticeIn()
		for _, want := range []string{
			staffMarker("lockdown_auto_started_join_surge") + " 15 60",
			staffMarker("lockdown_note_approved_muted"),
		} {
			if !strings.Contains(text, want) {
				t.Errorf("notice %q does not contain %q", text, want)
			}
		}
		for _, unwanted := range []string{staffMarker("lockdown_note_no_delete"), staffMarker("lockdown_note_no_invite")} {
			if strings.Contains(text, unwanted) {
				t.Errorf("notice %q contains %q although the bot holds both rights", text, unwanted)
			}
		}

		// The latch gone, more joins past the threshold find the lockdown active and post
		// nothing more.
		env.dropRaidLatch()
		env.selfJoins(3)
		if got := len(env.replies()); got != 1 {
			t.Errorf("%d messages reached the locked group after more joins, want still 1", got)
		}
	})

	t.Run("flood notice", func(t *testing.T) {
		env := newLockdownEnv(t)
		linkLockdownStaffGroup(env)
		raidCleanup(t, env.chat.Id)
		recent := env.newMembers(4, 2*time.Hour)

		env.sendItems(recent, 30)
		if row := env.activeLockdown(); row == nil || row.LockedAt == nil {
			t.Fatalf("setup: active lockdown = %+v, want one confirmed by the flood", row)
		}

		text := env.wantOneNoticeIn()
		want := staffMarker("lockdown_auto_started_flood") + " 30 30 100"
		if !strings.Contains(text, want) {
			t.Errorf("notice %q does not contain %q", text, want)
		}
		if !strings.Contains(text, staffMarker("lockdown_note_approved_muted")) {
			t.Errorf("notice %q does not say approved users are muted too", text)
		}
	})

	t.Run("no delete right", func(t *testing.T) {
		env := newLockdownEnv(t)
		linkLockdownStaffGroup(env)
		raidCleanup(t, env.chat.Id)
		env.fake.setBotMember(env.chat.Id, lockdownBotAdminJSON(true, false, true))

		env.selfJoins(15)

		text := env.wantOneNoticeIn()
		if !strings.Contains(text, staffMarker("lockdown_note_no_delete")) {
			t.Errorf("notice %q does not contain %q", text, staffMarker("lockdown_note_no_delete"))
		}
		if strings.Contains(text, staffMarker("lockdown_note_no_invite")) {
			t.Errorf("notice %q contains %q although the bot may invite", text, staffMarker("lockdown_note_no_invite"))
		}
	})

	t.Run("no invite right", func(t *testing.T) {
		env := newLockdownEnv(t)
		linkLockdownStaffGroup(env)
		raidCleanup(t, env.chat.Id)
		env.fake.setBotMember(env.chat.Id, lockdownBotAdminJSON(true, true, false))

		env.selfJoins(15)

		text := env.wantOneNoticeIn()
		if !strings.Contains(text, staffMarker("lockdown_note_no_invite")) {
			t.Errorf("notice %q does not contain %q", text, staffMarker("lockdown_note_no_invite"))
		}
		if strings.Contains(text, staffMarker("lockdown_note_no_delete")) {
			t.Errorf("notice %q contains %q although the bot may delete", text, staffMarker("lockdown_note_no_delete"))
		}
	})

	t.Run("an admin's lockdown gets no automatic notice", func(t *testing.T) {
		env := newLockdownEnv(t)
		linkLockdownStaffGroup(env)
		raidCleanup(t, env.chat.Id)

		env.send(env.admin, "/lockdown")
		if got := len(env.replies()); got != 1 {
			t.Fatalf("setup: %d replies to /lockdown, want 1", got)
		}
		env.dropRaidLatch()
		env.selfJoins(15)

		if got := len(env.replies()); got != 1 {
			t.Errorf("%d messages reached the group, want only the /lockdown reply: a trigger that finds a lockdown posts nothing", got)
		}
		for _, sent := range env.replies() {
			if text := fmt.Sprint(sent.Params["text"]); strings.Contains(text, staffMarker("lockdown_auto_started_join_surge")) {
				t.Errorf("message %q announces an automatic lockdown", text)
			}
		}
	})

	t.Run("notice fails", func(t *testing.T) {
		env := newLockdownEnv(t)
		staffChat := linkLockdownStaffGroup(env)
		raidCleanup(t, env.chat.Id)
		env.fake.script("sendMessage", env.chat.Id, staffFakeError(400, "Bad Request: have no rights to send a message"))

		env.selfJoins(15)

		row := env.activeLockdown()
		if row == nil || row.LockedAt == nil {
			t.Fatalf("active lockdown = %+v, want it still active and confirmed after a failed notice", row)
		}
		if got := len(env.calls("setChatPermissions")); got != 1 {
			t.Errorf("setChatPermissions calls = %d, want exactly 1", got)
		}
		if got := len(env.replies()); got != 0 {
			t.Errorf("%d messages reached the locked group, want none: the failed notice is not retried", got)
		}
		env.dropRaidLatch()
		env.selfJoins(2)
		if got := len(env.replies()); got != 0 {
			t.Errorf("%d messages reached the locked group after more joins, want none", got)
		}

		// The failure did not touch the alert.
		env.cycle()
		if sent := env.fake.sentTo(staffChat.Id); len(sent) != 1 {
			t.Errorf("%d messages reached the Staff Group, want exactly 1", len(sent))
		}
	})
}
