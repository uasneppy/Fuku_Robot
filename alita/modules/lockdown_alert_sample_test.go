//go:build testtools

package modules

import (
	"fmt"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/divkix/Alita_Robot/alita/db"
	"github.com/divkix/Alita_Robot/alita/db/models"
)

// lockdownBurstEnv is linkLockdownStaffGroup plus the cleanup of the env's chat's burst
// rows, and returns the Staff Group.
func lockdownBurstEnv(env *lockdownEnv) gotgbot.Chat {
	env.t.Helper()
	staffChat := linkLockdownStaffGroup(env)
	env.t.Cleanup(func() {
		db.DB.Where("chat_id = ?", env.chat.Id).Delete(&models.ChatLockdownBurst{})
	})
	return staffChat
}

// burstRows reads every burst row of the env's chat, oldest first.
func (e *lockdownEnv) burstRows() []models.ChatLockdownBurst {
	e.t.Helper()
	var rows []models.ChatLockdownBurst
	if err := db.DB.Where("chat_id = ?", e.chat.Id).Order("id").Find(&rows).Error; err != nil {
		e.t.Fatalf("read burst rows: %v", err)
	}
	return rows
}

// alertText is the text of the one alert the Staff Group received.
func alertText(t *testing.T, env *lockdownEnv, staffChat gotgbot.Chat) string {
	t.Helper()
	sent := env.fake.sentTo(staffChat.Id)
	if len(sent) != 1 {
		t.Fatalf("%d messages reached the Staff Group, want exactly 1", len(sent))
	}
	return fmt.Sprint(sent[0].Params["text"])
}

func TestLockdownAlertSample(t *testing.T) {
	t.Run("pre-lockdown joiner listed", func(t *testing.T) {
		env := newLockdownEnv(t)
		staffChat := lockdownBurstEnv(env)

		early := gotgbot.User{Id: uniqueLinkOwnerID(), FirstName: "Pre<i>", Username: "early"}
		env.join(early, early, "https://t.me/+pre")

		env.send(env.admin, "/lockdown raid")
		env.cycle()

		text := alertText(t, env, staffChat)
		for _, want := range []string{
			"Pre&lt;i&gt;",
			"@early",
			fmt.Sprintf("<code>%d</code>", early.Id),
			staffMarker("lockdown_alert_counts"),
		} {
			if !strings.Contains(text, want) {
				t.Errorf("alert %q does not contain %q", text, want)
			}
		}

		rows := env.burstRows()
		if len(rows) != 1 {
			t.Fatalf("%d burst rows, want 1", len(rows))
		}
		row := rows[0]
		if row.UserID != early.Id || row.Source != models.BurstSourcePre || row.InviteLink != "https://t.me/+pre" {
			t.Errorf("burst row = %+v, want user %d, source pre and the invite link", row, early.Id)
		}
		if row.FirstName != "Pre<i>" || row.Username != "early" || row.JoinedAt == nil {
			t.Errorf("burst row = %+v, want the stored name, username and join time", row)
		}
	})
}
