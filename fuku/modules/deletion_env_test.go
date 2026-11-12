//go:build testtools

package modules

import (
	"sync"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/alicebob/miniredis/v2"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/db/staff"
	"github.com/uasneppy/fuku_robot/fuku/utils/ratelimit"
)

// deletionEnv is one supergroup in which the bot is an administrator with the delete right, a
// real dispatcher with the ServiceDeletion and Greetings modules loaded, and miniredis behind it.
// The clock is fixed, so every message it sends is dated deletionNow() and no queue row is
// ever in 1970.
type deletionEnv struct {
	t          *testing.T
	fake       *lockdownFake
	bot        *gotgbot.Bot
	dispatcher *ext.Dispatcher
	chat       gotgbot.Chat
	admin      gotgbot.User
	redis      *miniredis.Miniredis
	now        time.Time

	mu         sync.Mutex
	nextUpdate int64
}

// withFastDeletionPacer installs a pacer with a 1 ms interval and a 5 ms retry_after unit as
// deletionPacer for the test, exactly as withFastLockdownPacer does for the lockdown pacer.
func withFastDeletionPacer(t *testing.T) {
	t.Helper()
	mreqSwap(t, &deletionPacer, ratelimit.NewTelegramPacer(ratelimit.TelegramPacerOptions{
		NextKey:        "fuku:delete:pace:next",
		BlockKey:       "fuku:delete:pace:block",
		Interval:       time.Millisecond,
		MaxRetries:     3,
		MaxWait:        60 * time.Second,
		RetryAfterUnit: 5 * time.Millisecond,
	}))
}

func newDeletionEnv(t *testing.T) *deletionEnv {
	t.Helper()
	redisServer := withMiniredisServer(t)
	withStaffLocale(t)
	withFastDeletionPacer(t)

	now := time.Now().UTC().Truncate(time.Second)
	mreqSwap(t, &deletionNow, func() time.Time { return now })

	fake := newLockdownFake()
	bot := newModuleTestBot(fake.moduleBotClient)
	bot.BotClient = fake

	env := &deletionEnv{
		t:          t,
		redis:      redisServer,
		fake:       fake,
		bot:        bot,
		now:        now,
		chat:       gotgbot.Chat{Id: uniqueModuleChatID(), Type: "supergroup", Title: "Deletion Chat"},
		admin:      gotgbot.User{Id: uniqueLinkOwnerID(), FirstName: "Admin"},
		nextUpdate: 100,
	}
	deletionCleanup(t, env.chat.Id)
	staffCleanup(t, env.chat.Id)
	fake.setBotMember(env.chat.Id, lockdownBotAdminJSON(false, true, false))

	env.dispatcher = ext.NewDispatcher(&ext.DispatcherOpts{MaxRoutines: -1})
	LoadServiceDeletion(env.dispatcher)
	LoadGreetings(env.dispatcher)
	// The inline deletes run on goroutines; this is registered last, so it runs before the
	// cleanups above and no delete is still in flight when the pacer and the clients it uses
	// are put back.
	t.Cleanup(deletionInline.wait)
	return env
}

func (e *deletionEnv) updateID() int64 {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.nextUpdate++
	return e.nextUpdate
}

// sendService runs a message of the env chat through the real dispatcher: from sent it (nil
// means a message with no sender), apply sets the field the test wants (a pin, a title, a
// SenderChat for an anonymous admin) and its Date is deletionNow(). It returns the message ID.
func (e *deletionEnv) sendService(from *gotgbot.User, apply func(*gotgbot.Message)) int64 {
	e.t.Helper()
	return e.sendServiceTo(e.chat, from, apply)
}

// sendServiceTo is sendService for another chat, whose rules and state the test set up itself.
func (e *deletionEnv) sendServiceTo(chat gotgbot.Chat, from *gotgbot.User, apply func(*gotgbot.Message)) int64 {
	e.t.Helper()
	id := e.updateID()
	msg := &gotgbot.Message{MessageId: id, Date: deletionNow().Unix(), Chat: chat, From: from}
	if apply != nil {
		apply(msg)
	}
	update := &gotgbot.Update{UpdateId: id, Message: msg}
	if err := e.dispatcher.ProcessUpdate(e.bot, update, nil); err != nil {
		e.t.Fatalf("ProcessUpdate(%d) error = %v", id, err)
	}
	return id
}

// sendPin runs a pinned-message notice of the env chat sent by the env's administrator.
func (e *deletionEnv) sendPin() int64 {
	e.t.Helper()
	return e.sendService(&e.admin, func(m *gotgbot.Message) {
		m.PinnedMessage = gotgbot.Message{MessageId: 1, Chat: e.chat}
	})
}

// deletes returns the deleteMessage calls addressed to chatID once every inline delete has
// finished.
func (e *deletionEnv) deletes(chatID int64) []moduleBotCall {
	e.t.Helper()
	deletionInline.wait()
	var matched []moduleBotCall
	for _, call := range e.fake.callsFor("deleteMessage") {
		if staffParamInt(call.Params, "chat_id") == chatID {
			matched = append(matched, call)
		}
	}
	return matched
}

// queued returns the queue rows of chatID.
func (e *deletionEnv) queued(chatID int64) []models.ScheduledDeletion {
	e.t.Helper()
	var rows []models.ScheduledDeletion
	if err := db.DB.Where("chat_id = ?", chatID).Order("message_id").Find(&rows).Error; err != nil {
		e.t.Fatalf("read the queue of chat %d: %v", chatID, err)
	}
	return rows
}

// makeStaffGroup turns chatID into a Staff Group.
func (e *deletionEnv) makeStaffGroup(chatID int64) {
	e.t.Helper()
	staffCleanup(e.t, chatID)
	if _, err := staff.CreateStaffGroup(chatID, e.admin.Id, "Staff"); err != nil {
		e.t.Fatalf("CreateStaffGroup(%d) error = %v", chatID, err)
	}
}
