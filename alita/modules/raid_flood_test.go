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

	"github.com/divkix/Alita_Robot/alita/db"
	"github.com/divkix/Alita_Robot/alita/db/approvals"
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

// floodSizes reads the sizes of the chat's three flood sets: all items, new-member items
// and new-member accounts.
func floodSizes(t *testing.T, chatID int64) (items, newItems, accounts int64) {
	t.Helper()
	client := cache.GetRedisClient()
	if client == nil {
		t.Fatal("no Redis client in the test")
	}
	read := func(part string) int64 {
		n, err := client.ZCard(context.Background(), raidKey(chatID, part)).Result()
		if err != nil {
			t.Fatalf("read the %s set: %v", part, err)
		}
		return n
	}
	return read(raidPartFloodItems), read(raidPartFloodNew), read(raidPartFloodAccounts)
}

func TestRaidFloodItemKey(t *testing.T) {
	command := func(text string, length int64) gotgbot.Message {
		return gotgbot.Message{
			MessageId: 7, Text: text,
			Entities: []gotgbot.MessageEntity{{Type: "bot_command", Offset: 0, Length: length}},
		}
	}
	photo := []gotgbot.PhotoSize{{FileId: floodTestFile, FileUniqueId: floodTestFile, Width: 1, Height: 1}}
	file := gotgbot.Document{FileId: floodTestFile, FileUniqueId: floodTestFile}

	counted := []struct {
		name string
		msg  gotgbot.Message
		want string
	}{
		{"text", gotgbot.Message{MessageId: 7, Text: "hello"}, "m:7"},
		{"text that mentions a command later", gotgbot.Message{
			MessageId: 7, Text: "hello /cmd",
			Entities: []gotgbot.MessageEntity{{Type: "bot_command", Offset: 6, Length: 4}},
		}, "m:7"},
		{"a lone slash", gotgbot.Message{MessageId: 7, Text: "/ nothing"}, "m:7"},
		{"photo", gotgbot.Message{MessageId: 7, Photo: photo}, "m:7"},
		{"photo with a caption", gotgbot.Message{MessageId: 7, Photo: photo, Caption: "look"}, "m:7"},
		{"animation that is also a document", gotgbot.Message{
			MessageId: 7, Animation: &gotgbot.Animation{FileId: floodTestFile}, Document: &file,
		}, "m:7"},
		{"sticker", gotgbot.Message{MessageId: 7, Sticker: &gotgbot.Sticker{FileId: floodTestFile}}, "m:7"},
		{"video", gotgbot.Message{MessageId: 7, Video: &gotgbot.Video{FileId: floodTestFile}}, "m:7"},
		{"voice", gotgbot.Message{MessageId: 7, Voice: &gotgbot.Voice{FileId: floodTestFile}}, "m:7"},
		{"document", gotgbot.Message{MessageId: 7, Document: &file}, "m:7"},
		{"poll", gotgbot.Message{MessageId: 7, Poll: &gotgbot.Poll{Id: "p"}}, "m:7"},
		{"dice", gotgbot.Message{MessageId: 7, Dice: &gotgbot.Dice{Emoji: "x", Value: 3}}, "m:7"},
		{"album part", gotgbot.Message{MessageId: 7, Photo: photo, MediaGroupId: "alb1"}, "g:alb1"},
		{"another album part", gotgbot.Message{MessageId: 8, Photo: photo, MediaGroupId: "alb1"}, "g:alb1"},
	}
	for _, tc := range counted {
		t.Run("counts "+tc.name, func(t *testing.T) {
			got, ok := raidFloodItemKey(&tc.msg)
			if !ok || got != tc.want {
				t.Errorf("raidFloodItemKey = %q, %v, want %q, true", got, ok, tc.want)
			}
		})
	}

	ignored := []struct {
		name string
		msg  gotgbot.Message
	}{
		{"command", command("/cmd", 4)},
		{"command with a bot name and arguments", command("/cmd@alita_bot args", 14)},
		{"command with no entity", gotgbot.Message{MessageId: 7, Text: "/cmd args"}},
		{"new members", gotgbot.Message{MessageId: 7, NewChatMembers: []gotgbot.User{{Id: 5}}}},
		{"left member", gotgbot.Message{MessageId: 7, LeftChatMember: &gotgbot.User{Id: 5}}},
		{"pinned message", gotgbot.Message{MessageId: 7, PinnedMessage: gotgbot.Message{MessageId: 3}}},
		{"new title", gotgbot.Message{MessageId: 7, NewChatTitle: "A new title"}},
		{"new chat photo", gotgbot.Message{MessageId: 7, NewChatPhoto: photo}},
		{"empty message", gotgbot.Message{MessageId: 7}},
	}
	for _, tc := range ignored {
		t.Run("ignores "+tc.name, func(t *testing.T) {
			if got, ok := raidFloodItemKey(&tc.msg); ok {
				t.Errorf("raidFloodItemKey = %q, true, want it ignored", got)
			}
		})
	}

	if _, ok := raidFloodItemKey(nil); ok {
		t.Error("raidFloodItemKey(nil) counted")
	}
}

// floodPrime makes the env's chat hold 24 counted items from three recent joiners, so
// that six more counted items would reach the default threshold of 30. It returns them.
func floodPrime(env *lockdownEnv) []gotgbot.User {
	env.t.Helper()
	recent := env.newMembers(3, 2*time.Hour)
	env.sendItems(recent, 24)
	if items, newItems, accounts := floodSizes(env.t, env.chat.Id); items != 24 || newItems != 24 || accounts != 3 {
		env.t.Fatalf("priming left %d items, %d new, %d accounts, want 24, 24, 3", items, newItems, accounts)
	}
	return recent
}

// newFloodEnv is a lockdown env with a Staff Group, clean raid settings and env.admin as
// the chat's creator, so the cached admin list holds them.
func newFloodEnv(t *testing.T) *lockdownEnv {
	t.Helper()
	env := newLockdownEnv(t)
	linkLockdownStaffGroup(env)
	raidCleanup(t, env.chat.Id)
	env.fake.setCreator(env.chat.Id, env.admin.Id)
	return env
}

func TestRaidFloodExemptions(t *testing.T) {
	channel := gotgbot.Chat{Id: -1001234567890, Type: "channel", Title: "News"}
	asUser := func(kind string) func(env *lockdownEnv, user gotgbot.User) {
		return func(env *lockdownEnv, user gotgbot.User) { env.sendItem(user, kind) }
	}
	withSenderChat := func(chat func(env *lockdownEnv) gotgbot.Chat, forward bool) func(env *lockdownEnv, user gotgbot.User) {
		return func(env *lockdownEnv, user gotgbot.User) {
			msg := floodMessage(env.t, user, "text")
			sender := chat(env)
			msg.SenderChat = &sender
			msg.IsAutomaticForward = forward
			env.dispatchMessage(env.chat, msg)
		}
	}

	// Each case is a sender that must never count. The sender is seeded as a recent joiner
	// (when it has a user ID of its own), so it would count were it not exempt.
	cases := []struct {
		name string
		// user is the sender; the zero value means a fresh account (or env.admin for "admin").
		user gotgbot.User
		// setup runs before the chat is primed; an approval has to exist first.
		setup func(env *lockdownEnv, user gotgbot.User)
		send  func(env *lockdownEnv, user gotgbot.User)
	}{
		{name: "admin", send: asUser("text")},
		{
			name: "approved user",
			setup: func(env *lockdownEnv, user gotgbot.User) {
				if err := approvals.AddApprovedUser(env.chat.Id, user.Id, env.admin.Id, ""); err != nil {
					env.t.Fatalf("approve %d: %v", user.Id, err)
				}
				env.t.Cleanup(func() { db.DB.Where("chat_id = ?", env.chat.Id).Delete(&models.ApprovedUsers{}) })
			},
			send: asUser("text"),
		},
		{name: "service account 777000", user: gotgbot.User{Id: 777000, FirstName: "Telegram"}, send: asUser("text")},
		{
			name: "group anonymous bot with sender_chat the group",
			user: gotgbot.User{Id: lockdownGroupAnonymousBot, IsBot: true, FirstName: "Group", Username: "GroupAnonymousBot"},
			send: withSenderChat(func(env *lockdownEnv) gotgbot.Chat { return env.chat }, false),
		},
		{
			name: "channel bot 136817688",
			user: gotgbot.User{Id: 136817688, IsBot: true, FirstName: "Channel", Username: "Channel_Bot"},
			send: withSenderChat(func(*lockdownEnv) gotgbot.Chat { return channel }, false),
		},
		{
			name: "linked channel automatic forward",
			user: gotgbot.User{Id: 777000, FirstName: "Telegram"},
			send: withSenderChat(func(*lockdownEnv) gotgbot.Chat { return channel }, true),
		},
		{
			// Not what Telegram sends, but it pins the sender_chat skip on its own: an ordinary
			// user ID is no service account, so only the skip keeps this out.
			name: "sender_chat on an ordinary user",
			send: withSenderChat(func(*lockdownEnv) gotgbot.Chat { return channel }, false),
		},
		{name: "commands", send: asUser("command")},
		{
			name: "edited message",
			send: func(env *lockdownEnv, user gotgbot.User) {
				id := env.updateID()
				msg := floodMessage(env.t, user, "text")
				msg.MessageId, msg.Date, msg.Chat, msg.EditDate = id, 1, env.chat, 2
				update := &gotgbot.Update{UpdateId: id, EditedMessage: &msg}
				if err := env.dispatcher.ProcessUpdate(env.bot, update, nil); err != nil {
					env.t.Fatalf("ProcessUpdate(%d) error = %v", id, err)
				}
			},
		},
		{name: "bot sender", user: gotgbot.User{IsBot: true, FirstName: "OtherBot"}, send: asUser("text")},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := newFloodEnv(t)
			user := tc.user
			switch {
			case tc.name == "admin":
				user = env.admin
			case user.Id == 0:
				user.Id = uniqueLinkOwnerID()
				user.FirstName = "Exempt"
			}
			if tc.setup != nil {
				tc.setup(env, user)
			}
			seedRecentJoin(env, user.Id, time.Hour)
			floodPrime(env)

			for i := 0; i < 6; i++ {
				tc.send(env, user)
			}

			if items, newItems, accounts := floodSizes(t, env.chat.Id); items != 24 || newItems != 24 || accounts != 3 {
				t.Errorf("after six exempt items the sets hold %d items, %d new, %d accounts, want 24, 24, 3", items, newItems, accounts)
			}
			if row := env.activeLockdown(); row != nil {
				t.Errorf("a lockdown (kind %q) exists after six exempt items, want none", row.TriggerKind)
			}
			if got := len(env.calls("setChatPermissions")); got != 0 {
				t.Errorf("setChatPermissions calls = %d, want 0", got)
			}
		})
	}

	t.Run("a recent joiner is the positive control", func(t *testing.T) {
		env := newFloodEnv(t)
		recent := floodPrime(env)

		for i := 0; i < 6; i++ {
			env.sendItem(recent[0], "text")
		}

		row := env.activeLockdown()
		if row == nil {
			t.Fatal("no lockdown after 24 + 6 items from recent joiners")
		}
		if row.TriggerKind != models.LockdownTriggerFlood {
			t.Errorf("TriggerKind = %q, want flood", row.TriggerKind)
		}
	})

	t.Run("basic group", func(t *testing.T) {
		env := newFloodEnv(t)
		group := gotgbot.Chat{Id: uniqueModuleChatID(), Type: "group", Title: "Small"}
		lockdownCleanup(t, group.Id)
		client := cache.GetRedisClient()
		users := make([]gotgbot.User, 0, 4)
		for i := 0; i < 4; i++ {
			user := env.newJoiner("Basic")
			users = append(users, user)
			err := client.ZAdd(context.Background(), raidKey(group.Id, raidPartJoined),
				redis.Z{Score: float64(raidNow().Add(-time.Hour).UnixMilli()), Member: fmt.Sprint(user.Id)}).Err()
			if err != nil {
				t.Fatalf("seed the joined set: %v", err)
			}
		}

		for i := 0; i < 40; i++ {
			env.dispatchMessage(group, floodMessage(t, users[i%len(users)], "text"))
		}

		if items, newItems, accounts := floodSizes(t, group.Id); items+newItems+accounts != 0 {
			t.Errorf("a basic group counted %d items, %d new, %d accounts, want nothing", items, newItems, accounts)
		}
		if got := len(callsToChat(env.fake.staffBotClient, "setChatPermissions", group.Id)); got != 0 {
			t.Errorf("setChatPermissions calls in the basic group = %d, want 0", got)
		}
	})
}

func TestRaidFloodRule(t *testing.T) {
	t.Run("56 percent does not lock", func(t *testing.T) {
		env := newFloodEnv(t)
		established := env.newMembers(5, 0)
		recent := env.newMembers(3, 3*time.Hour)

		env.sendItems(established, 13)
		env.sendItems(recent, 17)

		if row := env.activeLockdown(); row != nil {
			t.Errorf("a lockdown exists with 17 of 30 items (56%%) from new members, want none")
		}
		if items, newItems, accounts := floodSizes(t, env.chat.Id); items != 30 || newItems != 17 || accounts != 3 {
			t.Errorf("sets hold %d items, %d new, %d accounts, want 30, 17, 3", items, newItems, accounts)
		}
	})

	t.Run("60 percent locks", func(t *testing.T) {
		env := newFloodEnv(t)
		established := env.newMembers(5, 0)
		recent := env.newMembers(3, 3*time.Hour)

		env.sendItems(established, 12)
		env.sendItems(recent, 18)

		row := env.activeLockdown()
		if row == nil {
			t.Fatal("no lockdown with 18 of 30 items (60%) from new members")
		}
		if row.TriggerKind != models.LockdownTriggerFlood || row.TriggerCount != 30 || row.TriggerPercent != 60 {
			t.Errorf("trigger = %q, %d items, %d%%, want flood, 30, 60%%", row.TriggerKind, row.TriggerCount, row.TriggerPercent)
		}
	})

	t.Run("two accounts do not lock", func(t *testing.T) {
		env := newFloodEnv(t)
		recent := env.newMembers(2, 3*time.Hour)

		env.sendItems(recent, 40)

		if row := env.activeLockdown(); row != nil {
			t.Errorf("a lockdown exists with 40 items from two new accounts, want none")
		}
		if got := len(env.calls("setChatPermissions")); got != 0 {
			t.Errorf("setChatPermissions calls = %d, want 0", got)
		}
	})

	t.Run("an album counts once", func(t *testing.T) {
		env := newFloodEnv(t)
		recent := env.newMembers(3, 3*time.Hour)

		for i, user := range recent {
			for part := 0; part < 10; part++ {
				env.sendItem(user, fmt.Sprintf("album:alb-%d", i))
			}
		}

		if items, newItems, accounts := floodSizes(t, env.chat.Id); items != 3 || newItems != 3 || accounts != 3 {
			t.Errorf("three albums of ten photos left %d items, %d new, %d accounts, want 3, 3, 3", items, newItems, accounts)
		}
		if row := env.activeLockdown(); row != nil {
			t.Errorf("a lockdown exists after three albums, want none")
		}
	})
}

func TestRaidFloodScript(t *testing.T) {
	withMiniredis(t)
	client := cache.GetRedisClient()
	if client == nil {
		t.Fatal("no Redis client in the test")
	}

	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	previousNow, previousCap := raidNow, raidFloodCap
	raidNow = func() time.Time { return now }
	t.Cleanup(func() { raidNow, raidFloodCap = previousNow, previousCap })

	const chatID int64 = -1001111111111
	const recentUser, oldUser, strangerUser int64 = 11, 12, 13
	join := func(user int64, age time.Duration) {
		t.Helper()
		err := client.ZAdd(context.Background(), raidKey(chatID, raidPartJoined),
			redis.Z{Score: float64(now.Add(-age).UnixMilli()), Member: fmt.Sprint(user)}).Err()
		if err != nil {
			t.Fatalf("seed the joined set: %v", err)
		}
	}
	join(recentUser, 2*time.Hour)
	join(oldUser, 25*time.Hour)

	record := func(user int64, item string) [3]int {
		t.Helper()
		items, newItems, accounts, err := recordRaidFloodItem(client, chatID, user, item)
		if err != nil {
			t.Fatalf("recordRaidFloodItem(%d, %s): %v", user, item, err)
		}
		return [3]int{items, newItems, accounts}
	}
	check := func(got, want [3]int, what string) {
		t.Helper()
		if got != want {
			t.Errorf("%s: sizes = %v, want %v", what, got, want)
		}
	}

	check(record(recentUser, "m:1"), [3]int{1, 1, 1}, "an item from a member who joined 2 h ago")
	check(record(oldUser, "m:2"), [3]int{2, 1, 1}, "an item from a member who joined 25 h ago")
	check(record(strangerUser, "m:3"), [3]int{3, 1, 1}, "an item from a member with no join entry")
	check(record(recentUser, "m:4"), [3]int{4, 2, 1}, "a second item from the new member")
	check(record(recentUser, "m:4"), [3]int{4, 2, 1}, "the same item key again")

	for _, part := range []string{raidPartFloodItems, raidPartFloodNew, raidPartFloodAccounts} {
		ttl, err := client.PTTL(context.Background(), raidKey(chatID, part)).Result()
		if err != nil {
			t.Fatalf("PTTL %s: %v", part, err)
		}
		if ttl <= 0 || ttl > raidFloodTTL {
			t.Errorf("TTL of %s = %v, want within (0, %v]", part, ttl, raidFloodTTL)
		}
	}

	now = now.Add(31 * time.Second)
	check(record(strangerUser, "m:5"), [3]int{1, 0, 0}, "one item from an unknown member 31 s later")

	raidFloodCap = 3
	for i := 0; i < 5; i++ {
		record(recentUser, fmt.Sprintf("m:cap-%d", i))
	}
	if items, newItems, _ := floodSizes(t, chatID); items != 3 || newItems != 3 {
		t.Errorf("with a cap of 3 the sets hold %d items and %d new items, want 3 and 3", items, newItems)
	}
}
