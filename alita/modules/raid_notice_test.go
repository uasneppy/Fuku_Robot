//go:build testtools

package modules

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/divkix/Alita_Robot/alita/db/models"
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

// warnEnv is an env whose chat is linked to a Staff Group and has a log channel taking the
// admin category, with no lockdown started.
func warnEnv(t *testing.T) (env *lockdownEnv, staffChat int64, channel int64) {
	t.Helper()
	env = newLockdownEnv(t)
	staffGroup := linkLockdownStaffGroup(env)
	channel = uniqueModuleChatID()
	setStaffLogChannel(t, env.chat.Id, channel)
	raidCleanup(t, env.chat.Id)
	return env, staffGroup.Id, channel
}

// textsTo returns the text of every message the fake sent to chatID, in order.
func (e *lockdownEnv) textsTo(chatID int64) []string {
	e.t.Helper()
	var texts []string
	for _, sent := range e.fake.sentTo(chatID) {
		texts = append(texts, fmt.Sprint(sent.Params["text"]))
	}
	return texts
}

// wantWarnings fails unless the Staff Group and the log channel each hold exactly want
// messages.
func (e *lockdownEnv) wantWarnings(staffChat, channel int64, want int) {
	e.t.Helper()
	if got := len(e.fake.sentTo(staffChat)); got != want {
		e.t.Errorf("%d messages reached the Staff Group, want %d", got, want)
	}
	if got := len(e.fake.sentTo(channel)); got != want {
		e.t.Errorf("%d messages reached the log channel, want %d", got, want)
	}
}

func TestRaidCannotLockWarnsOncePerDay(t *testing.T) {
	t.Run("no restrict right", func(t *testing.T) {
		env, staffChat, channel := warnEnv(t)
		env.fake.setBotRole(env.chat.Id, staffRoleAdminNoRestrict)

		env.selfJoins(15)

		if row := env.activeLockdown(); row != nil {
			t.Fatalf("a lockdown exists although the bot cannot restrict members: %+v", row)
		}
		if got := env.lockdownRowCount(); got != 0 {
			t.Errorf("lockdown rows = %d, want none", got)
		}
		for _, method := range []string{"getChat", "setChatPermissions"} {
			if got := len(env.calls(method)); got != 0 {
				t.Errorf("%s calls = %d, want 0", method, got)
			}
		}
		env.wantWarnings(staffChat, channel, 1)
		if got := len(env.replies()); got != 0 {
			t.Errorf("%d messages reached the group itself, want none", got)
		}

		staffTexts := env.textsTo(staffChat)
		for _, want := range []string{
			staffMarker("lockdown_auto_cant_lock_rights"),
			staffMarker("lockdown_trigger_join_surge") + " 15 60",
			"<b>Lock Chat</b>",
		} {
			if len(staffTexts) == 1 && !strings.Contains(staffTexts[0], want) {
				t.Errorf("Staff Group warning %q does not contain %q", staffTexts[0], want)
			}
		}
		logTexts := env.textsTo(channel)
		if len(logTexts) == 1 {
			if !strings.HasPrefix(logTexts[0], env.wantAlertHeader()) {
				t.Errorf("log warning %q does not start with the usual log header %q", logTexts[0], env.wantAlertHeader())
			}
			if !strings.Contains(logTexts[0], staffMarker("lockdown_auto_cant_lock_rights")) {
				t.Errorf("log warning %q does not contain %q", logTexts[0], staffMarker("lockdown_auto_cant_lock_rights"))
			}
		}

		// The latch gone, a sustained raid reaches the start path again and warns nobody
		// again: the day's warning was sent.
		env.dropRaidLatch()
		env.selfJoins(15)
		env.wantWarnings(staffChat, channel, 1)

		// A day and a minute later the next qualifying join warns once more.
		env.redis.FastForward(24*time.Hour + time.Minute)
		env.dropRaidLatch()
		env.selfJoins(1)
		env.wantWarnings(staffChat, channel, 2)
	})

	t.Run("lock refused", func(t *testing.T) {
		env, staffChat, channel := warnEnv(t)
		env.fake.script("setChatPermissions", env.chat.Id, staffFakeError(400, "Bad Request: not enough rights"))

		env.selfJoins(15)

		if got := env.lockdownRowCount(); got != 0 {
			t.Errorf("lockdown rows = %d, want none: a refused lock leaves no row", got)
		}
		env.wantWarnings(staffChat, channel, 1)
		for _, chatID := range []int64{staffChat, channel} {
			texts := env.textsTo(chatID)
			if len(texts) == 1 && !strings.Contains(texts[0], staffMarker("lockdown_auto_cant_lock_refused")) {
				t.Errorf("warning %q does not contain %q", texts[0], staffMarker("lockdown_auto_cant_lock_refused"))
			}
		}
	})

	t.Run("basic group", func(t *testing.T) {
		env, staffChat, channel := warnEnv(t)
		basic := env.chat
		basic.Type = "group"

		raidFire(env.bot, &basic, raidTrigger{Kind: models.LockdownTriggerJoinSurge, Count: 15, Window: 60})

		env.wantWarnings(staffChat, channel, 1)
		for _, chatID := range []int64{staffChat, channel} {
			texts := env.textsTo(chatID)
			if len(texts) == 1 && !strings.Contains(texts[0], staffMarker("lockdown_auto_cant_lock_basic")) {
				t.Errorf("warning %q does not contain %q", texts[0], staffMarker("lockdown_auto_cant_lock_basic"))
			}
		}
		if got := env.lockdownRowCount(); got != 0 {
			t.Errorf("lockdown rows = %d, want none", got)
		}
	})

	t.Run("flood trigger", func(t *testing.T) {
		env, staffChat, channel := warnEnv(t)
		env.fake.setBotRole(env.chat.Id, staffRoleAdminNoRestrict)
		recent := env.newMembers(4, 2*time.Hour)

		env.sendItems(recent, 30)

		env.wantWarnings(staffChat, channel, 1)
		texts := env.textsTo(staffChat)
		if want := staffMarker("lockdown_trigger_flood") + " 30 30 100"; len(texts) == 1 && !strings.Contains(texts[0], want) {
			t.Errorf("warning %q does not contain %q", texts[0], want)
		}
	})

	t.Run("transient failures", func(t *testing.T) {
		for _, tc := range []struct {
			name  string
			setup func(env *lockdownEnv)
		}{
			{"bot rights unreadable", func(env *lockdownEnv) {
				env.fake.script("getChatMember", env.chat.Id, staffFakeError(500, "Internal Server Error"))
			}},
			{"permissions unreadable", func(env *lockdownEnv) {
				env.fake.clearChatPerms(env.chat.Id)
			}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				env, staffChat, channel := warnEnv(t)
				tc.setup(env)

				env.selfJoins(15)

				if row := env.activeLockdown(); row != nil {
					t.Fatalf("a lockdown exists although the start could not be judged: %+v", row)
				}
				env.wantWarnings(staffChat, channel, 0)
				if got := len(env.replies()); got != 0 {
					t.Errorf("%d messages reached the group itself, want none", got)
				}
			})
		}
	})

	t.Run("only the Staff Group", func(t *testing.T) {
		env := newLockdownEnv(t)
		staffGroup := linkLockdownStaffGroup(env)
		raidCleanup(t, env.chat.Id)
		env.fake.setBotRole(env.chat.Id, staffRoleAdminNoRestrict)

		env.selfJoins(15)

		if got := len(env.fake.sentTo(staffGroup.Id)); got != 1 {
			t.Errorf("%d messages reached the Staff Group, want 1", got)
		}
		if got := len(env.sentChats()); got != 1 {
			t.Errorf("%d messages were sent in all, want only the Staff Group's", got)
		}
	})

	t.Run("no destinations", func(t *testing.T) {
		env := newLockdownEnv(t)
		raidCleanup(t, env.chat.Id)
		env.fake.setBotRole(env.chat.Id, staffRoleAdminNoRestrict)

		env.selfJoins(15)

		if row := env.activeLockdown(); row != nil {
			t.Fatalf("a lockdown exists although the bot cannot restrict members: %+v", row)
		}
		if got := len(env.sentChats()); got != 0 {
			t.Errorf("%d messages were sent, want none: nobody is linked and no log channel is set", got)
		}
	})

	t.Run("title is escaped", func(t *testing.T) {
		env, staffChat, _ := warnEnv(t)
		env.chat.Title = "Raid <i>HQ</i>"
		env.fake.setBotRole(env.chat.Id, staffRoleAdminNoRestrict)

		env.selfJoins(15)

		texts := env.textsTo(staffChat)
		if len(texts) != 1 {
			t.Fatalf("%d messages reached the Staff Group, want 1", len(texts))
		}
		if !strings.Contains(texts[0], "Raid &lt;i&gt;HQ&lt;/i&gt;") || strings.Contains(texts[0], "<i>") {
			t.Errorf("warning %q does not carry the escaped title", texts[0])
		}
	})
}
