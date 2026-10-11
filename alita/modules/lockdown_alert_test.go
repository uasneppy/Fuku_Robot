//go:build testtools

package modules

import (
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
	staffChat := gotgbot.Chat{Id: uniqueModuleChatID(), Type: "supergroup", Title: "Staff HQ"}
	staffCleanup(env.t, staffChat.Id, env.chat.Id)
	env.t.Cleanup(func() {
		db.DB.Where("chat_id = ?", env.chat.Id).Delete(&models.ChatLockdownAlert{})
	})
	if _, err := staff.CreateStaffGroup(staffChat.Id, env.admin.Id, staffChat.Title); err != nil {
		env.t.Fatalf("create Staff Group: %v", err)
	}
	link := &models.StaffGroupLink{
		GroupChatID: env.chat.Id,
		StaffChatID: staffChat.Id,
		OwnerUserID: env.admin.Id,
		GroupTitle:  "Lock Chat",
	}
	if err := staff.CreateLink(link); err != nil {
		env.t.Fatalf("create link: %v", err)
	}
	return staffChat
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
