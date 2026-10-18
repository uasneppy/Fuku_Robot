//go:build testtools

package modules

import (
	"fmt"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/divkix/Alita_Robot/alita/db"
	dbcache "github.com/divkix/Alita_Robot/alita/db/cache"
	"github.com/divkix/Alita_Robot/alita/db/lockdown"
	"github.com/divkix/Alita_Robot/alita/db/models"
)

// raidCleanup deletes the chat_raid_detection rows of the given chats now and when the
// test ends, together with their cached copies, so no test sees another's settings.
func raidCleanup(t *testing.T, chatIDs ...int64) {
	t.Helper()
	clean := func() {
		if len(chatIDs) == 0 {
			return
		}
		db.DB.Where("chat_id IN ?", chatIDs).Delete(&models.ChatRaidDetection{})
		for _, id := range chatIDs {
			dbcache.DeleteCache(dbcache.CacheKey("raid_detection", id))
		}
	}
	clean()
	t.Cleanup(clean)
}

// selfJoins makes n different accounts join the env's chat on their own, one chat_member
// update each, and returns them in order.
func (e *lockdownEnv) selfJoins(n int) []gotgbot.User {
	e.t.Helper()
	users := make([]gotgbot.User, 0, n)
	for i := 0; i < n; i++ {
		user := e.newJoiner(fmt.Sprintf("Raider%d", i))
		e.join(user, user, "")
		users = append(users, user)
	}
	return users
}

// activeLockdown reads the env's chat's active lockdown fresh, nil when there is none.
func (e *lockdownEnv) activeLockdown() *models.ChatLockdown {
	e.t.Helper()
	row, err := lockdown.GetActiveFresh(e.chat.Id)
	if err != nil {
		e.t.Fatalf("read the active lockdown: %v", err)
	}
	return row
}

func TestRaidJoinSurge(t *testing.T) {
	t.Run("fifteen self-joins lock", func(t *testing.T) {
		env := newLockdownEnv(t)
		staffChat := linkLockdownStaffGroup(env)
		raidCleanup(t, env.chat.Id)

		env.selfJoins(14)
		if row := env.activeLockdown(); row != nil {
			t.Fatalf("a lockdown (kind %q) exists after 14 joins, want none", row.TriggerKind)
		}
		if got := len(env.calls("setChatPermissions")); got != 0 {
			t.Fatalf("setChatPermissions calls after 14 joins = %d, want 0", got)
		}

		fifteenth := env.newJoiner("Fifteenth")
		env.join(fifteenth, fifteenth, "")

		row := env.activeLockdown()
		if row == nil {
			t.Fatal("no active lockdown after the 15th join in a minute")
		}
		if row.TriggerKind != models.LockdownTriggerJoinSurge {
			t.Errorf("TriggerKind = %q, want %q", row.TriggerKind, models.LockdownTriggerJoinSurge)
		}
		if row.TriggerCount != 15 || row.TriggerWindow != 60 || row.TriggerPercent != 0 {
			t.Errorf("trigger numbers = %d joins in %d s, %d%%, want 15 in 60 s, 0%%",
				row.TriggerCount, row.TriggerWindow, row.TriggerPercent)
		}
		if row.StartedBy != staffTestBotID {
			t.Errorf("StartedBy = %d, want the bot %d", row.StartedBy, staffTestBotID)
		}
		if row.Reason != "" {
			t.Errorf("Reason = %q, want empty", row.Reason)
		}
		if row.LockedAt == nil {
			t.Error("LockedAt is nil, want the lock confirmed")
		}
		if got := len(env.calls("setChatPermissions")); got != 1 {
			t.Errorf("setChatPermissions calls = %d, want exactly 1", got)
		}

		joiner := env.joinerRow(fifteenth.Id)
		if joiner.State != models.JoinerStatePending {
			t.Errorf("the joiner who tipped the threshold is %q, want pending (handled by the lockdown it started)", joiner.State)
		}

		sixteenth := env.newJoiner("Sixteenth")
		env.join(sixteenth, sixteenth, "")
		if got := len(env.calls("setChatPermissions")); got != 1 {
			t.Errorf("setChatPermissions calls after a 16th join = %d, want still 1", got)
		}
		var rows int64
		if err := db.DB.Model(&models.ChatLockdown{}).Where("chat_id = ?", env.chat.Id).Count(&rows).Error; err != nil {
			t.Fatalf("count lockdown rows: %v", err)
		}
		if rows != 1 {
			t.Errorf("lockdown rows = %d, want 1", rows)
		}

		env.cycle()
		sent := env.fake.sentTo(staffChat.Id)
		if len(sent) != 1 {
			t.Fatalf("%d messages reached the Staff Group, want exactly 1", len(sent))
		}
		text := fmt.Sprint(sent[0].Params["text"])
		for _, want := range []string{
			staffMarker("lockdown_alert_trigger_join_surge") + " 15 60",
			staffMarker("lockdown_alert_counts") + " 15 ",
		} {
			if !strings.Contains(text, want) {
				t.Errorf("alert %q does not contain %q", text, want)
			}
		}
		if strings.Contains(text, staffMarker("lockdown_alert_trigger_manual")) {
			t.Errorf("alert %q names a manual lockdown", text)
		}
	})
}
