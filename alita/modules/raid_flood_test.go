//go:build testtools

package modules

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/redis/go-redis/v9"

	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/utils/cache"
)

// seedRecentJoin writes a join of userID into the env's chat's joined set directly, age
// ago, the way the join counter would have. Tests seed the set instead of replaying the
// joins, so the flood trigger is judged on its own.
func seedRecentJoin(env *lockdownEnv, userID int64, age time.Duration) {
	env.t.Helper()
	client := cache.GetRedisClient()
	if client == nil {
		env.t.Fatal("no Redis client in the test")
	}
	score := float64(raidNow().Add(-age).UnixMilli())
	err := client.ZAdd(context.Background(), raidKey(env.chat.Id, raidPartJoined), redis.Z{Score: score, Member: fmt.Sprint(userID)}).Err()
	if err != nil {
		env.t.Fatalf("seed the joined set: %v", err)
	}
}

// floodTestFile is the file reference every media item of the tests carries.
const floodTestFile = "AgAD-test-file"

// dispatchMessage runs one message update in chat through the env's real dispatcher.
func (e *lockdownEnv) dispatchMessage(chat gotgbot.Chat, msg gotgbot.Message) {
	e.t.Helper()
	id := e.updateID()
	msg.MessageId = id
	msg.Date = 1
	msg.Chat = chat
	if err := e.dispatcher.ProcessUpdate(e.bot, &gotgbot.Update{UpdateId: id, Message: &msg}, nil); err != nil {
		e.t.Fatalf("ProcessUpdate(%d) error = %v", id, err)
	}
}

// floodMessage builds the message of one item kind: text, photo, gif (an animation, which
// Telegram also sends as a document), sticker, command, or album:<id> (one photo of an
// album).
func floodMessage(t *testing.T, from gotgbot.User, kind string) gotgbot.Message {
	t.Helper()
	msg := gotgbot.Message{From: &from}
	photo := []gotgbot.PhotoSize{{FileId: floodTestFile, FileUniqueId: floodTestFile, Width: 1, Height: 1}}
	switch {
	case kind == "text":
		msg.Text = "hello there"
	case kind == "photo":
		msg.Photo = photo
	case kind == "gif":
		msg.Animation = &gotgbot.Animation{FileId: floodTestFile, FileUniqueId: floodTestFile}
		msg.Document = &gotgbot.Document{FileId: floodTestFile, FileUniqueId: floodTestFile}
	case kind == "sticker":
		msg.Sticker = &gotgbot.Sticker{FileId: floodTestFile, FileUniqueId: floodTestFile}
	case kind == "command":
		msg.Text = "/cmd now"
		msg.Entities = []gotgbot.MessageEntity{{Type: "bot_command", Offset: 0, Length: 4}}
	case strings.HasPrefix(kind, "album:"):
		msg.MediaGroupId = strings.TrimPrefix(kind, "album:")
		msg.Photo = photo
	default:
		t.Fatalf("unknown item kind %q", kind)
	}
	return msg
}

// sendItem sends one item of the given kind to the env's chat as from.
func (e *lockdownEnv) sendItem(from gotgbot.User, kind string) {
	e.t.Helper()
	e.dispatchMessage(e.chat, floodMessage(e.t, from, kind))
}

// sendItems sends n items, one per call in turn from the given users, cycling through
// the item kinds of ordinary chat.
func (e *lockdownEnv) sendItems(users []gotgbot.User, n int) {
	e.t.Helper()
	kinds := []string{"text", "photo", "gif", "sticker"}
	for i := 0; i < n; i++ {
		e.sendItem(users[i%len(users)], kinds[i%len(kinds)])
	}
}

// newMembers makes n joiners and seeds each as having joined the given time ago.
func (e *lockdownEnv) newMembers(n int, age time.Duration) []gotgbot.User {
	e.t.Helper()
	users := make([]gotgbot.User, 0, n)
	for i := 0; i < n; i++ {
		user := e.newJoiner(fmt.Sprintf("Member%d", i))
		if age > 0 {
			seedRecentJoin(e, user.Id, age)
		}
		users = append(users, user)
	}
	return users
}

func TestRaidFlood(t *testing.T) {
	t.Run("recent joiners flood", func(t *testing.T) {
		env := newLockdownEnv(t)
		staffChat := linkLockdownStaffGroup(env)
		raidCleanup(t, env.chat.Id)
		recent := env.newMembers(4, 2*time.Hour)

		env.sendItems(recent, 29)
		if row := env.activeLockdown(); row != nil {
			t.Fatalf("a lockdown (kind %q) exists after 29 items, want none", row.TriggerKind)
		}
		if got := len(env.calls("setChatPermissions")); got != 0 {
			t.Fatalf("setChatPermissions calls after 29 items = %d, want 0", got)
		}

		env.sendItem(recent[1], "text")

		row := env.activeLockdown()
		if row == nil {
			t.Fatal("no active lockdown after the 30th item in 30 seconds from recent joiners")
		}
		if row.TriggerKind != models.LockdownTriggerFlood {
			t.Errorf("TriggerKind = %q, want %q", row.TriggerKind, models.LockdownTriggerFlood)
		}
		if row.TriggerCount != 30 || row.TriggerWindow != 30 || row.TriggerPercent != 100 {
			t.Errorf("trigger numbers = %d items in %d s, %d%%, want 30 in 30 s, 100%%",
				row.TriggerCount, row.TriggerWindow, row.TriggerPercent)
		}
		if row.StartedBy != staffTestBotID {
			t.Errorf("StartedBy = %d, want the bot %d", row.StartedBy, staffTestBotID)
		}
		if got := len(env.calls("setChatPermissions")); got != 1 {
			t.Errorf("setChatPermissions calls = %d, want exactly 1", got)
		}
		if got := len(env.calls("deleteMessage")); got != 0 {
			t.Errorf("deleteMessage calls = %d, want 0: the flood trigger deletes nothing", got)
		}

		env.cycle()
		sent := env.fake.sentTo(staffChat.Id)
		if len(sent) != 1 {
			t.Fatalf("%d messages reached the Staff Group, want exactly 1", len(sent))
		}
		text := fmt.Sprint(sent[0].Params["text"])
		want := staffMarker("lockdown_alert_trigger_flood") + " 30 30 100"
		if !strings.Contains(text, want) {
			t.Errorf("alert %q does not contain %q", text, want)
		}
		if strings.Contains(text, staffMarker("lockdown_alert_trigger_manual")) {
			t.Errorf("alert %q names a manual lockdown", text)
		}
	})

	t.Run("established members", func(t *testing.T) {
		env := newLockdownEnv(t)
		linkLockdownStaffGroup(env)
		raidCleanup(t, env.chat.Id)
		established := env.newMembers(5, 0)

		env.sendItems(established, 40)

		if row := env.activeLockdown(); row != nil {
			t.Errorf("a lockdown (kind %q) exists after a busy chat of established members, want none", row.TriggerKind)
		}
		if got := len(env.calls("setChatPermissions")); got != 0 {
			t.Errorf("setChatPermissions calls = %d, want 0", got)
		}
	})

	t.Run("joined over a day ago", func(t *testing.T) {
		env := newLockdownEnv(t)
		linkLockdownStaffGroup(env)
		raidCleanup(t, env.chat.Id)
		old := env.newMembers(4, 25*time.Hour)

		env.sendItems(old, 40)

		if row := env.activeLockdown(); row != nil {
			t.Errorf("a lockdown (kind %q) exists for members who joined 25 hours ago, want none", row.TriggerKind)
		}
		if got := len(env.calls("setChatPermissions")); got != 0 {
			t.Errorf("setChatPermissions calls = %d, want 0", got)
		}
	})
}
