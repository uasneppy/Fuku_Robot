//go:build testtools

package modules

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
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

	// script scripts the answers of the bot's client per method and per message; see
	// deletionScriptClient.
	script *deletionScriptClient

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
	// Every message the fake sends or edits is dated by the fixed clock, so no queued row is in 1970.
	fake.sendDate = func() int64 { return deletionNow().Unix() }
	bot := newModuleTestBot(fake.moduleBotClient)
	script := &deletionScriptClient{BotClient: fake, fake: fake}
	bot.BotClient = script

	env := &deletionEnv{
		t:          t,
		redis:      redisServer,
		fake:       fake,
		bot:        bot,
		script:     script,
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
	e.deliver(id, chat, from, apply)
	return id
}

// deliver runs the message with the given ID through the real dispatcher. Delivering one ID
// twice is how a test replays an update.
func (e *deletionEnv) deliver(id int64, chat gotgbot.Chat, from *gotgbot.User, apply func(*gotgbot.Message)) {
	e.t.Helper()
	msg := &gotgbot.Message{MessageId: id, Date: deletionNow().Unix(), Chat: chat, From: from}
	if apply != nil {
		apply(msg)
	}
	update := &gotgbot.Update{UpdateId: id, Message: msg}
	if err := e.dispatcher.ProcessUpdate(e.bot, update, nil); err != nil {
		e.t.Fatalf("ProcessUpdate(%d) error = %v", id, err)
	}
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
	deletionInline.wait()
	return e.queuedNoWait(chatID)
}

// queuedNoWait is queued without waiting for the inline deletes, for a test that holds some
// of them open.
func (e *deletionEnv) queuedNoWait(chatID int64) []models.ScheduledDeletion {
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

// freshDeletionRoot puts a fresh, uncancelled root context in place of the current one.
func freshDeletionRoot() {
	deletionRoot.mu.Lock()
	defer deletionRoot.mu.Unlock()
	deletionRoot.ctx, deletionRoot.cancel = context.WithCancel(context.Background())
	// A stop of the worker also set the self-destruct batcher's draining flag; a fresh root puts
	// both back (plan 17-06).
	memberDeleteDraining.Store(false)
}

// restoreDeletionWorker puts a fresh root context back when the test ends, as
// restoreMemberReqRunner does for the member requirements runner: a test that cancels the root
// through cancelDeletionContext() would otherwise leave every later deletion call cancelled.
func restoreDeletionWorker(t *testing.T) {
	t.Helper()
	t.Cleanup(freshDeletionRoot)
}

// deletionHangClient wraps a bot client and holds every deleteMessage request until its context
// ends (while hang is set), so a test can park the inline deletes inside the pacer.
type deletionHangClient struct {
	gotgbot.BotClient
	hang atomic.Bool
}

func (c *deletionHangClient) RequestWithContext(
	ctx context.Context,
	token, method string,
	params map[string]any,
	opts *gotgbot.RequestOpts,
) (json.RawMessage, error) {
	if method == "deleteMessage" && c.hang.Load() {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return c.BotClient.RequestWithContext(ctx, token, method, params, opts)
}

// deletionInlineRunning is how many inline deletes are in flight now.
func deletionInlineRunning() int {
	deletionInline.mu.Lock()
	defer deletionInline.mu.Unlock()
	return deletionInline.running
}

// deletionScriptClient wraps a bot client and lets a test script the answer of any request by
// method and by message: the script returns the error to answer with (it may block until the
// request context ends to model a hung call), or nil to let the request through to the fake.
// A scripted request is recorded on the fake like any other, so the call lists stay complete.
type deletionScriptClient struct {
	gotgbot.BotClient
	fake *lockdownFake

	mu     sync.Mutex
	script func(ctx context.Context, method string, params map[string]any) error
}

// set installs the script; nil removes it.
func (c *deletionScriptClient) set(script func(ctx context.Context, method string, params map[string]any) error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.script = script
}

func (c *deletionScriptClient) RequestWithContext(
	ctx context.Context,
	token, method string,
	params map[string]any,
	opts *gotgbot.RequestOpts,
) (json.RawMessage, error) {
	c.mu.Lock()
	script := c.script
	c.mu.Unlock()
	if script != nil {
		if err := script(ctx, method, params); err != nil {
			c.fake.record(method, params)
			return nil, err
		}
	}
	return c.BotClient.RequestWithContext(ctx, token, method, params, opts)
}

// replica builds a second bot process over the same database and Redis, with a fake of its
// own: the two see the same queue and the same pacer budget but record their calls apart.
func (e *deletionEnv) replica() *deletionEnv {
	e.t.Helper()
	fake := newLockdownFake()
	// Every message the fake sends or edits is dated by the fixed clock, so no queued row is in 1970.
	fake.sendDate = func() int64 { return deletionNow().Unix() }
	bot := newModuleTestBot(fake.moduleBotClient)
	script := &deletionScriptClient{BotClient: fake, fake: fake}
	bot.BotClient = script
	fake.setBotMember(e.chat.Id, lockdownBotAdminJSON(false, true, false))
	return &deletionEnv{t: e.t, redis: e.redis, fake: fake, bot: bot, script: script, now: e.now, chat: e.chat, admin: e.admin}
}

// cycle runs one worker cycle with the env bot and reports whether it made progress.
func (e *deletionEnv) cycle() bool {
	e.t.Helper()
	return runDeletionCycle(context.Background(), e.bot)
}

// pass runs cycles back to back until one makes no progress, as the worker's loop does, and
// returns how many ran.
func (e *deletionEnv) pass() int {
	e.t.Helper()
	cycles := 0
	for e.cycle() {
		cycles++
		if cycles > 50 {
			e.t.Fatal("the worker made progress for 50 cycles in a row")
		}
	}
	return cycles + 1
}

// seedDeletionRow writes one pending queue row of a message sent at sentAt that is due at
// deleteAt, directly through the model.
func seedDeletionRow(t *testing.T, chatID, messageID int64, deleteAt, sentAt time.Time, kind string) models.ScheduledDeletion {
	t.Helper()
	row := models.ScheduledDeletion{
		ChatID:    chatID,
		MessageID: messageID,
		Kind:      kind,
		SentAt:    sentAt.UTC().Truncate(time.Second),
		DeleteAt:  deleteAt.UTC().Truncate(time.Second),
		State:     models.ScheduledDeletionPending,
		CreatedAt: sentAt.UTC().Truncate(time.Second),
	}
	if err := db.DB.Create(&row).Error; err != nil {
		t.Fatalf("seed queue row %d of chat %d: %v", messageID, chatID, err)
	}
	return row
}

// seedDue seeds n rows of one kind for chatID, message IDs first..first+n-1, all due before now
// in ascending order of their ID, and returns the IDs.
func (e *deletionEnv) seedDue(chatID, first int64, n int, kind string) []int64 {
	e.t.Helper()
	ids := make([]int64, 0, n)
	for i := 0; i < n; i++ {
		id := first + int64(i)
		at := e.now.Add(-time.Hour + time.Duration(i)*time.Second)
		seedDeletionRow(e.t, chatID, id, at, e.now.Add(-2*time.Hour), kind)
		ids = append(ids, id)
	}
	return ids
}

// batchCalls returns the message IDs of every deleteMessages call this env's fake answered for
// chatID, in order.
func (e *deletionEnv) batchCalls(chatID int64) [][]int64 {
	e.t.Helper()
	var out [][]int64
	for _, call := range e.fake.callsFor("deleteMessages") {
		if staffParamInt(call.Params, "chat_id") != chatID {
			continue
		}
		ids, ok := call.Params["message_ids"].([]int64)
		if !ok {
			e.t.Fatalf("deleteMessages message_ids = %T, want []int64", call.Params["message_ids"])
		}
		out = append(out, ids)
	}
	return out
}

// singleCalls returns the message IDs of every deleteMessage call for chatID, in order.
func (e *deletionEnv) singleCalls(chatID int64) []int64 {
	e.t.Helper()
	var out []int64
	for _, call := range e.fake.callsFor("deleteMessage") {
		if staffParamInt(call.Params, "chat_id") == chatID {
			out = append(out, staffParamInt(call.Params, "message_id"))
		}
	}
	return out
}
