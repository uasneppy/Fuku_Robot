//go:build testtools

package modules

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/redis/go-redis/v9"
	log "github.com/sirupsen/logrus"
	logrustest "github.com/sirupsen/logrus/hooks/test"

	"github.com/divkix/Alita_Robot/alita/db"
	"github.com/divkix/Alita_Robot/alita/db/approvals"
	"github.com/divkix/Alita_Robot/alita/db/captcha"
	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/utils/cache"
)

// resetRaidAIStats zeroes the raid AI counters, so a test reads only its own checks.
func resetRaidAIStats() {
	raidAIChecked.Store(0)
	raidAISpam.Store(0)
	raidAIKept.Store(0)
	raidAIShed.Store(0)
	raidAIFailed.Store(0)
	raidAIBlocked.Store(0)
	raidAICapped.Store(0)
	raidAICached.Store(0)
}

// newRaidAIEnv is a flood env (Staff Group, clean raid settings, env.admin the chat's
// creator) with TypeSafe configured and answering from the fake of serveTypeSafe, and
// /aispam off for its chat, so the raid path alone judges. The raid AI jobs it starts
// are waited for when the test ends.
func newRaidAIEnv(t *testing.T) *lockdownEnv {
	t.Helper()
	env := newFloodEnv(t)
	env.serveTypeSafe()
	resetRaidAIStats()
	t.Cleanup(env.settleAI)
	return env
}

// sendText has user send text in the env's chat through the real dispatcher.
func (e *lockdownEnv) sendText(user gotgbot.User, text string) {
	e.t.Helper()
	e.dispatchMessage(e.chat, gotgbot.Message{From: &user, Text: text})
}

// watchDirect runs the group 2 watcher on msg in chat and requires ext.ContinueGroups.
func (e *lockdownEnv) watchDirect(chat gotgbot.Chat, msg gotgbot.Message) {
	e.t.Helper()
	id := e.updateID()
	msg.MessageId, msg.Date, msg.Chat = id, 1, chat
	ctx := ext.NewContext(e.bot, &gotgbot.Update{UpdateId: id, Message: &msg}, nil)
	if err := lockdownModule.raidAIWatcher(e.bot, ctx); !errors.Is(err, ext.ContinueGroups) {
		e.t.Fatalf("raidAIWatcher returned %v, want ext.ContinueGroups", err)
	}
}

// wantNoActions fails when the bot deleted, sent, banned or restricted anything in the
// env's chat: a raid-only check only reports a verdict.
func (e *lockdownEnv) wantNoActions() {
	e.t.Helper()
	for _, method := range []string{"deleteMessage", "sendMessage", "banChatMember", "restrictChatMember"} {
		if got := len(e.calls(method)); got != 0 {
			e.t.Errorf("%s calls = %d, want 0: the raid AI path never acts", method, got)
		}
	}
}

// seedJoinedIn seeds userID as having joined chatID age ago, the way the join counter does.
func seedJoinedIn(t *testing.T, chatID, userID int64, age time.Duration) {
	t.Helper()
	client := cache.GetRedisClient()
	if client == nil {
		t.Fatal("no Redis client in the test")
	}
	z := redis.Z{Score: float64(raidNow().Add(-age).UnixMilli()), Member: fmt.Sprint(userID)}
	if err := client.ZAdd(context.Background(), raidKey(chatID, raidPartJoined), z).Err(); err != nil {
		t.Fatalf("seed the joined set of %d: %v", chatID, err)
	}
}

// spamLink is a text the TypeSafe fake judges spam and the candidate rule finds suspicious.
const spamLink = "SPAM deal t.me/scam"

func TestRaidAICandidateText(t *testing.T) {
	channel := gotgbot.Chat{Id: -1001234567890, Type: "channel", Title: "News"}

	t.Run("a new member's link is checked and counts", func(t *testing.T) {
		env := newRaidAIEnv(t)
		member := env.newMembers(1, 2*time.Hour)[0]

		env.sendText(member, spamLink)
		env.settleAI()

		if got := env.aiRequests(); got != 1 {
			t.Fatalf("TypeSafe requests = %d, want exactly 1", got)
		}
		items, err := env.redis.ZMembers(raidKey(env.chat.Id, "av"))
		if err != nil || len(items) != 1 || !strings.HasPrefix(items[0], "m:") {
			t.Errorf("verdict items = %v (error %v), want one m:<message id>", items, err)
		}
		env.wantNoActions()
		if stats := GetRaidAIStats(); stats.Checked != 1 || stats.Spam != 1 || stats.Kept != 0 || stats.Failed != 0 {
			t.Errorf("stats = %+v, want 1 checked, 1 spam", stats)
		}
	})

	t.Run("a keep verdict is checked and counts for nothing", func(t *testing.T) {
		env := newRaidAIEnv(t)
		member := env.newMembers(1, 2*time.Hour)[0]

		env.sendText(member, "my site is https://example.org/page")
		env.settleAI()

		if got := env.aiRequests(); got != 1 {
			t.Fatalf("TypeSafe requests = %d, want exactly 1", got)
		}
		if got := aiVerdictSetSize(env, "av"); got != 0 {
			t.Errorf("verdict items = %d, want 0 for a keep verdict", got)
		}
		if stats := GetRaidAIStats(); stats.Checked != 1 || stats.Kept != 1 || stats.Spam != 0 {
			t.Errorf("stats = %+v, want 1 checked, 1 kept", stats)
		}
	})

	t.Run("a forward is checked", func(t *testing.T) {
		env := newRaidAIEnv(t)
		member := env.newMembers(1, 2*time.Hour)[0]

		env.dispatchMessage(env.chat, gotgbot.Message{
			From:          &member,
			Text:          "SPAM please read this",
			ForwardOrigin: gotgbot.MessageOriginChannel{Date: 1, Chat: channel, MessageId: 5},
		})
		env.settleAI()

		if got := env.aiRequests(); got != 1 {
			t.Fatalf("TypeSafe requests = %d, want exactly 1", got)
		}
		if got := aiVerdictSetSize(env, "av"); got != 1 {
			t.Errorf("verdict items = %d, want 1", got)
		}
		env.wantNoActions()
	})

	t.Run("a repeated text is checked the second time", func(t *testing.T) {
		env := newRaidAIEnv(t)
		member := env.newMembers(1, 2*time.Hour)[0]
		const repeated = "SPAM join my channel!"

		env.sendText(member, repeated)
		env.settleAI()
		if got := env.aiRequests(); got != 0 {
			t.Fatalf("TypeSafe requests after the first text = %d, want 0", got)
		}

		env.sendText(member, "  "+strings.ToLower(repeated)+"  ")
		env.settleAI()
		if got := env.aiRequests(); got != 1 {
			t.Fatalf("TypeSafe requests after the repeat = %d, want 1 (case and spacing do not hide a repeat)", got)
		}
		env.wantNoActions()
	})

	notSent := []struct {
		name string
		run  func(env *lockdownEnv)
	}{
		{"a new member's plain text", func(env *lockdownEnv) {
			env.sendText(env.newMembers(1, time.Hour)[0], "hello everyone, how are you today")
		}},
		{"a short text sent twice", func(env *lockdownEnv) {
			member := env.newMembers(1, time.Hour)[0]
			env.sendText(member, "buy now")
			env.sendText(member, "buy now")
		}},
		{"an established member's link", func(env *lockdownEnv) {
			env.sendText(env.newMembers(1, 25*time.Hour)[0], spamLink)
		}},
		{"a member never seen joining", func(env *lockdownEnv) {
			env.sendText(env.newJoiner("Unseen"), spamLink)
		}},
		{"the creator", func(env *lockdownEnv) {
			seedRecentJoin(env, env.admin.Id, time.Hour)
			env.sendText(env.admin, spamLink)
		}},
		{"an approved user", func(env *lockdownEnv) {
			member := env.newMembers(1, time.Hour)[0]
			if err := approvals.AddApprovedUser(env.chat.Id, member.Id, env.admin.Id, ""); err != nil {
				env.t.Fatalf("approve %d: %v", member.Id, err)
			}
			env.t.Cleanup(func() { db.DB.Where("chat_id = ?", env.chat.Id).Delete(&models.ApprovedUsers{}) })
			env.sendText(member, spamLink)
		}},
		{"a member with a pending captcha", func(env *lockdownEnv) {
			member := env.newMembers(1, time.Hour)[0]
			if _, err := captcha.CreateCaptchaAttemptPreMessage(member.Id, env.chat.Id, "7", 5); err != nil {
				env.t.Fatalf("CreateCaptchaAttemptPreMessage() error = %v", err)
			}
			env.t.Cleanup(func() { db.DB.Where("chat_id = ?", env.chat.Id).Delete(&models.CaptchaAttempts{}) })
			env.sendText(member, spamLink)
		}},
		{"an anonymous administrator", func(env *lockdownEnv) {
			member := env.newMembers(1, time.Hour)[0]
			sender := env.chat
			env.dispatchMessage(env.chat, gotgbot.Message{From: &member, SenderChat: &sender, Text: spamLink})
		}},
		{"a channel post", func(env *lockdownEnv) {
			member := env.newMembers(1, time.Hour)[0]
			env.dispatchMessage(env.chat, gotgbot.Message{From: &member, SenderChat: &channel, Text: spamLink})
		}},
		{"an automatic forward", func(env *lockdownEnv) {
			telegram := gotgbot.User{Id: 777000, FirstName: "Telegram"}
			env.dispatchMessage(env.chat, gotgbot.Message{From: &telegram, SenderChat: &channel, IsAutomaticForward: true, Text: spamLink})
		}},
		{"Telegram service account 777000", func(env *lockdownEnv) {
			telegram := gotgbot.User{Id: 777000, FirstName: "Telegram"}
			seedRecentJoin(env, telegram.Id, time.Hour)
			env.sendText(telegram, spamLink)
		}},
		{"the bot itself", func(env *lockdownEnv) {
			self := gotgbot.User{Id: env.bot.Id, IsBot: true, FirstName: "Bot"}
			seedRecentJoin(env, self.Id, time.Hour)
			env.sendText(self, spamLink)
		}},
		{"a basic group", func(env *lockdownEnv) {
			group := gotgbot.Chat{Id: uniqueModuleChatID(), Type: "group", Title: "Small"}
			lockdownCleanup(env.t, group.Id)
			member := env.newJoiner("Basic")
			seedJoinedIn(env.t, group.Id, member.Id, time.Hour)
			env.dispatchMessage(group, gotgbot.Message{From: &member, Text: spamLink})
		}},
		{"a chat with the AI trigger off", func(env *lockdownEnv) {
			seedRaidSettings(env.t, models.ChatRaidDetection{
				ChatID: env.chat.Id, JoinEnabled: true, JoinThreshold: 15, FloodEnabled: true, FloodThreshold: 30,
				AIEnabled: false, AIThreshold: 5,
			})
			env.sendText(env.newMembers(1, time.Hour)[0], spamLink)
		}},
		{"a photo with a link caption", func(env *lockdownEnv) {
			member := env.newMembers(1, time.Hour)[0]
			env.dispatchMessage(env.chat, gotgbot.Message{
				From:    &member,
				Photo:   []gotgbot.PhotoSize{{FileId: floodTestFile, FileUniqueId: floodTestFile, Width: 1, Height: 1}},
				Caption: spamLink,
			})
		}},
	}
	for _, tc := range notSent {
		t.Run("not sent: "+tc.name, func(t *testing.T) {
			env := newRaidAIEnv(t)

			tc.run(env)
			env.settleAI()

			if got := env.aiRequests(); got != 0 {
				t.Errorf("TypeSafe requests = %d, want 0", got)
			}
			if got := aiVerdictSetSize(env, "av"); got != 0 {
				t.Errorf("verdict items = %d, want 0", got)
			}
		})
	}

	t.Run("the watcher continues the update on every path", func(t *testing.T) {
		env := newRaidAIEnv(t)
		member := env.newMembers(1, time.Hour)[0]
		group := gotgbot.Chat{Id: uniqueModuleChatID(), Type: "group", Title: "Small"}

		env.watchDirect(env.chat, gotgbot.Message{From: &member, Text: spamLink})
		env.watchDirect(env.chat, gotgbot.Message{From: &member, Text: "hello there"})
		env.watchDirect(env.chat, gotgbot.Message{From: &member})
		env.watchDirect(env.chat, gotgbot.Message{Text: spamLink})
		env.watchDirect(group, gotgbot.Message{From: &member, Text: spamLink})
		env.settleAI()
	})

	t.Run("a hung provider never holds the update", func(t *testing.T) {
		env := newRaidAIEnv(t)
		member := env.newMembers(1, time.Hour)[0]

		release := make(chan struct{})
		var once sync.Once
		open := func() { once.Do(func() { close(release) }) }
		started := make(chan struct{}, 1)
		withAISpamJevServer(t, func(w http.ResponseWriter, r *http.Request) {
			aiSpamRequests.Add(1)
			select {
			case started <- struct{}{}:
			default:
			}
			<-release
			_, _ = w.Write([]byte(aispamDecisionBody(0.95, "scam")))
		})
		t.Cleanup(open)

		if !returnsWithin(500*time.Millisecond, func() { env.sendText(member, spamLink) }) {
			open()
			t.Fatal("the update was held while the provider hung")
		}
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			open()
			t.Fatal("the provider never received the check")
		}
		open()
		env.settleAI()

		if got := env.aiRequests(); got != 1 {
			t.Errorf("TypeSafe requests = %d, want 1", got)
		}
		if got := aiVerdictSetSize(env, "av"); got != 1 {
			t.Errorf("verdict items = %d, want 1 once the provider answered", got)
		}
	})

	t.Run("no plain text stored", func(t *testing.T) {
		env := newRaidAIEnv(t)
		member := env.newMembers(1, time.Hour)[0]
		const repeated = "join my channel now!"

		env.sendText(member, repeated)
		env.sendText(member, repeated)
		env.settleAI()

		if got := env.aiRequests(); got != 1 {
			t.Fatalf("TypeSafe requests = %d, want 1: the case would prove nothing", got)
		}
		for _, key := range env.redis.Keys() {
			if strings.HasPrefix(key, "alita:aispam:") {
				t.Errorf("Redis key %q exists: the raid path must not touch the /aispam sender history", key)
			}
		}
		history, err := env.redis.List(raidKey(env.chat.Id, fmt.Sprintf("ar:%d", member.Id)))
		if err != nil {
			t.Fatalf("read the repeat history: %v", err)
		}
		hash := regexp.MustCompile(`^[0-9a-f]{16}$`)
		if len(history) == 0 {
			t.Error("the repeat history is empty")
		}
		for _, entry := range history {
			if !hash.MatchString(entry) || strings.Contains(entry, "channel") {
				t.Errorf("repeat history entry %q, want a 16-character hex hash and no text", entry)
			}
		}
		for _, key := range env.redis.Keys() {
			if values, err := env.redis.List(key); err == nil {
				for _, value := range values {
					if strings.Contains(value, "channel") {
						t.Errorf("Redis key %q holds message text %q", key, value)
					}
				}
			}
		}
	})
}

func TestRaidAINoRedis(t *testing.T) {
	env := newRaidAIEnv(t)
	member := env.newMembers(1, time.Hour)[0]
	t.Cleanup(cache.DisableRedisForTest())

	raidNoRedisLogged.Store(false)
	t.Cleanup(func() { raidNoRedisLogged.Store(false) })
	previousHooks := log.StandardLogger().ReplaceHooks(make(log.LevelHooks))
	hook := logrustest.NewGlobal()
	previousLevel := log.GetLevel()
	log.SetLevel(log.InfoLevel)
	t.Cleanup(func() {
		log.SetLevel(previousLevel)
		log.StandardLogger().ReplaceHooks(previousHooks)
	})

	env.sendText(member, spamLink)
	env.sendText(member, spamLink)
	env.watchDirect(env.chat, gotgbot.Message{From: &member, Text: spamLink})
	env.settleAI()

	if got := env.aiRequests(); got != 0 {
		t.Errorf("TypeSafe requests = %d, want 0 without Redis", got)
	}
	if row := env.activeLockdown(); row != nil {
		t.Errorf("a lockdown (kind %q) exists without Redis, want none", row.TriggerKind)
	}
	notices := 0
	for _, entry := range hook.AllEntries() {
		if entry.Level == log.InfoLevel && strings.Contains(entry.Message, "automatic raid detection is off") {
			notices++
		}
	}
	if notices > 1 {
		t.Errorf("the no-Redis notice was logged %d times, want at most once", notices)
	}

	// /lockdown does not need Redis.
	env.send(env.admin, "/lockdown raid")
	row := env.activeLockdown()
	if row == nil || row.LockedAt == nil {
		t.Fatalf("lockdown row = %+v, want a confirmed manual lockdown without Redis", row)
	}
	if row.TriggerKind != models.LockdownTriggerManual {
		t.Errorf("TriggerKind = %q, want %q", row.TriggerKind, models.LockdownTriggerManual)
	}
}
