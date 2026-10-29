//go:build testtools

package modules

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/divkix/Alita_Robot/alita/db"
	"github.com/divkix/Alita_Robot/alita/db/lockdown"
	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/db/staff"
)

// linkLockdownStaffGroup makes a Staff Group owned by the env's administrator and
// links the env's chat to it. It registers the cleanup of both chats' staff rows and
// of the alert rows of the env's chat, and returns the Staff Group's chat.
func linkLockdownStaffGroup(env *lockdownEnv) gotgbot.Chat {
	env.t.Helper()
	staffChat := newLockdownStaffGroup(env)
	linkLockdownGroupTo(env, staffChat)
	return staffChat
}

// newLockdownStaffGroup makes a Staff Group owned by the env's administrator without
// linking anything to it, and registers the cleanup of the staff rows of both chats and
// of the alert rows of the env's chat.
func newLockdownStaffGroup(env *lockdownEnv) gotgbot.Chat {
	env.t.Helper()
	staffChat := gotgbot.Chat{Id: uniqueModuleChatID(), Type: "supergroup", Title: "Staff HQ"}
	staffCleanup(env.t, staffChat.Id, env.chat.Id)
	env.t.Cleanup(func() {
		db.DB.Where("chat_id = ?", env.chat.Id).Delete(&models.ChatLockdownAlert{})
	})
	if _, err := staff.CreateStaffGroup(staffChat.Id, env.admin.Id, staffChat.Title); err != nil {
		env.t.Fatalf("create Staff Group: %v", err)
	}
	return staffChat
}

// linkLockdownGroupTo links the env's chat to staffChat.
func linkLockdownGroupTo(env *lockdownEnv, staffChat gotgbot.Chat) {
	env.t.Helper()
	link := &models.StaffGroupLink{
		GroupChatID: env.chat.Id,
		StaffChatID: staffChat.Id,
		OwnerUserID: env.admin.Id,
		GroupTitle:  "Lock Chat",
	}
	if err := staff.CreateLink(link); err != nil {
		env.t.Fatalf("create link: %v", err)
	}
}

// sentChats returns the chat IDs every sendMessage of the fake went to, in order.
func (e *lockdownEnv) sentChats() []int64 {
	raidAIJobs.wait()
	raidPosts.wait()
	e.fake.fmu.Lock()
	defer e.fake.fmu.Unlock()
	chats := make([]int64, 0, len(e.fake.sentLog))
	for _, sent := range e.fake.sentLog {
		chats = append(chats, sent.ChatID)
	}
	return chats
}

// wantSentOnlyTo fails when any sendMessage went to a chat other than the env's chat.
func (e *lockdownEnv) wantSentOnlyTo(chatID int64) {
	e.t.Helper()
	for _, got := range e.sentChats() {
		if got != chatID {
			e.t.Errorf("a message went to chat %d, want only chat %d", got, chatID)
		}
	}
}

// alertRows reads every alert copy row of the env's chat, oldest first.
func (e *lockdownEnv) alertRows() []models.ChatLockdownAlert {
	e.t.Helper()
	var rows []models.ChatLockdownAlert
	if err := db.DB.Where("chat_id = ?", e.chat.Id).Order("id").Find(&rows).Error; err != nil {
		e.t.Fatalf("read alert rows: %v", err)
	}
	return rows
}

func TestLockdownAlertPostsOnce(t *testing.T) {
	env := newLockdownEnv(t)
	staffChat := linkLockdownStaffGroup(env)

	env.send(env.admin, "/lockdown raid <b>wave</b>")
	replies := len(env.replies())
	env.cycle()
	env.cycle()

	sent := env.fake.sentTo(staffChat.Id)
	if len(sent) != 1 {
		t.Fatalf("%d messages reached the Staff Group, want exactly 1", len(sent))
	}
	text := fmt.Sprint(sent[0].Params["text"])
	for _, want := range []string{
		staffMarker("lockdown_alert_title"),
		staffMarker("lockdown_alert_trigger_manual"),
		"Ad&lt;b&gt;min",
		"raid &lt;b&gt;wave&lt;/b&gt;",
		"Lock Chat",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("alert %q does not contain %q", text, want)
		}
	}

	rows := env.alertRows()
	if len(rows) != 1 {
		t.Fatalf("%d alert rows, want 1", len(rows))
	}
	row := rows[0]
	if row.Kind != models.AlertKindAlert || row.Seq != 0 || row.Dest != models.AlertDestStaff || row.DestChatID != staffChat.Id {
		t.Errorf("row = kind %q seq %d dest %q to %d, want an alert for the Staff Group", row.Kind, row.Seq, row.Dest, row.DestChatID)
	}
	if row.State != models.AlertStatePosted {
		t.Errorf("State = %q, want posted", row.State)
	}
	if row.MessageID != sent[0].MessageID {
		t.Errorf("MessageID = %d, want the sent message %d", row.MessageID, sent[0].MessageID)
	}
	if row.Body != text {
		t.Errorf("Body = %q, want the exact text sent %q", row.Body, text)
	}

	active, err := lockdown.GetActiveFresh(env.chat.Id)
	if err != nil || active == nil {
		t.Fatalf("GetActiveFresh = %v, %v, want the lockdown", active, err)
	}
	if active.AlertStartedAt == nil {
		t.Error("alert_started_at is nil, want the time the alert was decided")
	}
	if active.ChatTitle != "Lock Chat" {
		t.Errorf("chat_title = %q, want Lock Chat", active.ChatTitle)
	}

	if got := len(env.replies()); got != replies {
		t.Errorf("the locked group received %d messages after /lockdown's reply, want none", got-replies)
	}
}

func TestLockdownAlertGate(t *testing.T) {
	t.Run("no destination", func(t *testing.T) {
		env := newLockdownEnv(t)
		t.Cleanup(func() { db.DB.Where("chat_id = ?", env.chat.Id).Delete(&models.ChatLockdownAlert{}) })

		env.send(env.admin, "/lockdown raid")
		env.cycle()
		env.cycle()

		active, err := lockdown.GetActiveFresh(env.chat.Id)
		if err != nil || active == nil {
			t.Fatalf("GetActiveFresh = %v, %v, want the lockdown", active, err)
		}
		if active.AlertStartedAt == nil {
			t.Error("alert_started_at is nil, want it stamped even with no destination")
		}
		if rows := env.alertRows(); len(rows) != 0 {
			t.Errorf("%d alert rows, want none for a group with no Staff Group", len(rows))
		}
		env.wantSentOnlyTo(env.chat.Id)
	})

	t.Run("linked later", func(t *testing.T) {
		env := newLockdownEnv(t)
		staffChat := newLockdownStaffGroup(env)

		env.send(env.admin, "/lockdown raid")
		env.cycle()
		linkLockdownGroupTo(env, staffChat)
		env.cycle()
		env.cycle()

		if sent := env.fake.sentTo(staffChat.Id); len(sent) != 0 {
			t.Errorf("%d messages reached a Staff Group linked after the alert was decided, want none", len(sent))
		}
		if rows := env.alertRows(); len(rows) != 0 {
			t.Errorf("%d alert rows, want none", len(rows))
		}
	})

	t.Run("lifted before posting", func(t *testing.T) {
		env := newLockdownEnv(t)
		staffChat := linkLockdownStaffGroup(env)

		env.send(env.admin, "/lockdown raid")
		if !lockdownPlanAlerts(context.Background(), env.bot) {
			t.Fatal("lockdownPlanAlerts decided nothing, want the alert decided")
		}
		rows := env.alertRows()
		if len(rows) != 1 || rows[0].State != models.AlertStatePending {
			t.Fatalf("alert rows = %+v, want one pending copy", rows)
		}

		env.send(env.admin, "/unlockdown")
		env.cycle()
		env.cycle()

		if sent := env.fake.sentTo(staffChat.Id); len(sent) != 0 {
			t.Errorf("%d messages reached the Staff Group for a lockdown that was lifted, want none", len(sent))
		}
		if rows := env.alertRows(); len(rows) != 1 || rows[0].State != models.AlertStatePending {
			t.Errorf("alert rows = %+v, want the copy left pending and never sent", rows)
		}
	})

	t.Run("older lockdown", func(t *testing.T) {
		env := newLockdownEnv(t)
		staffChat := linkLockdownStaffGroup(env)

		row := &models.ChatLockdown{
			ChatID:            env.chat.Id,
			TriggerKind:       models.LockdownTriggerManual,
			Reason:            "before phase 5",
			StartedBy:         env.admin.Id,
			StartedByName:     "Older Admin",
			PrePermissions:    lockdownTestPrePermissions,
			LockedPermissions: lockdownLockedPermissions,
		}
		if started, err := lockdown.Start(row); err != nil || !started {
			t.Fatalf("lockdown.Start = %v, %v, want true", started, err)
		}
		if ok, err := lockdown.ConfirmLocked(row.ID); err != nil || !ok {
			t.Fatalf("lockdown.ConfirmLocked = %v, %v, want true", ok, err)
		}

		env.cycle()
		env.cycle()

		sent := env.fake.sentTo(staffChat.Id)
		if len(sent) != 1 {
			t.Fatalf("%d messages reached the Staff Group, want exactly 1", len(sent))
		}
		text := fmt.Sprint(sent[0].Params["text"])
		wantGroup := fmt.Sprintf("<code>%d</code>", env.chat.Id)
		if !strings.Contains(text, wantGroup) {
			t.Errorf("alert %q does not name the group by %q", text, wantGroup)
		}
		if !strings.Contains(text, "Older Admin") || !strings.Contains(text, "before phase 5") {
			t.Errorf("alert %q must name who locked the group and why", text)
		}
	})

	t.Run("send fails", func(t *testing.T) {
		env := newLockdownEnv(t)
		staffChat := linkLockdownStaffGroup(env)
		env.fake.script("sendMessage", staffChat.Id, staffFakeError(400, "Bad Request: chat not found"))

		env.send(env.admin, "/lockdown raid")
		env.cycle()

		rows := env.alertRows()
		if len(rows) != 1 {
			t.Fatalf("%d alert rows, want 1", len(rows))
		}
		if rows[0].State != models.AlertStatePending || rows[0].Attempts != 1 {
			t.Errorf("after a failed send the copy is %q with %d attempts, want pending with 1", rows[0].State, rows[0].Attempts)
		}
		if sent := env.fake.sentTo(staffChat.Id); len(sent) != 0 {
			t.Fatalf("%d messages reached the Staff Group after a failed send, want none", len(sent))
		}

		env.cycle()

		sent := env.fake.sentTo(staffChat.Id)
		if len(sent) != 1 {
			t.Fatalf("%d messages reached the Staff Group after the retry, want 1", len(sent))
		}
		rows = env.alertRows()
		if len(rows) != 1 || rows[0].State != models.AlertStatePosted || rows[0].MessageID != sent[0].MessageID {
			t.Errorf("alert rows = %+v, want the copy posted as message %d", rows, sent[0].MessageID)
		}
	})
}
