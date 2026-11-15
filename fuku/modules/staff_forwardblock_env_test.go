//go:build testtools

package modules

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	logrustest "github.com/sirupsen/logrus/hooks/test"

	"github.com/uasneppy/fuku_robot/fuku/db/staff"
	"github.com/uasneppy/fuku_robot/fuku/utils/cache"
)

// The forward block watcher toolkit. The environment is the staff action environment with one linked
// group (its creator is the Staff Group's creator, the bot an administrator with every right), the
// real dispatcher with LoadStaff loaded (so the watcher is registered at group -9), a listed channel
// and a settable clock for the deletion clock the watcher's memo and latch read.

// fwdClock is a settable clock.
type fwdClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fwdClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fwdClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// fwdWatchClient wraps the stateful staff fake to take the delete right away from the bot in the
// chats a test names: the bot's own record in getChatAdministrators and getChatMember then says
// can_delete_messages false.
type fwdWatchClient struct {
	*staffActionFake

	mu       sync.Mutex
	noDelete map[int64]bool
}

func (c *fwdWatchClient) takeDeleteRight(chatID int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.noDelete == nil {
		c.noDelete = make(map[int64]bool)
	}
	c.noDelete[chatID] = true
}

func (c *fwdWatchClient) RequestWithContext(
	ctx context.Context, token, method string, params map[string]any, opts *gotgbot.RequestOpts,
) (json.RawMessage, error) {
	raw, err := c.staffActionFake.RequestWithContext(ctx, token, method, params, opts)
	if err != nil || len(raw) == 0 || (method != "getChatAdministrators" && method != "getChatMember") {
		return raw, err
	}
	c.mu.Lock()
	off := c.noDelete[staffParamInt(params, "chat_id")]
	c.mu.Unlock()
	if !off {
		return raw, nil
	}
	var decoded any
	if json.Unmarshal(raw, &decoded) != nil {
		return raw, nil
	}
	entries, isList := decoded.([]any)
	if !isList {
		entries = []any{decoded}
	}
	for _, entry := range entries {
		record, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		user, _ := record["user"].(map[string]any)
		if id, _ := user["id"].(float64); int64(id) == staffTestBotID {
			record["can_delete_messages"] = false
		}
	}
	out, marshalErr := json.Marshal(decoded)
	if marshalErr != nil {
		return raw, nil
	}
	return out, nil
}

// fwdWatchEnv is the environment of one watcher test.
type fwdWatchEnv struct {
	*staffActionEnv

	group     int64
	groupChat gotgbot.Chat
	source    int64
	client    *fwdWatchClient
	clock     *fwdClock
	logs      *logrustest.Hook
}

const fwdSourceTitle = "Blocked <Chan>"

// newFwdWatchEnv builds the environment with the source listed and the list's action set.
func newFwdWatchEnv(t *testing.T, action string) *fwdWatchEnv {
	t.Helper()
	base := newStaffActionEnv(t, 1)
	client := &fwdWatchClient{staffActionFake: base.fake}
	base.bot.BotClient = client
	env := &fwdWatchEnv{
		staffActionEnv: base,
		group:          base.groups[0],
		source:         uniqueModuleChatID(),
		client:         client,
		clock:          &fwdClock{now: time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)},
		logs:           logrustest.NewGlobal(),
	}
	env.groupChat = gotgbot.Chat{Id: env.group, Type: "supergroup", Title: "Linked Group"}
	t.Cleanup(env.logs.Reset)

	mreqSwap(t, &deletionNow, env.clock.Now)
	resetMemberDeletionFailureState(t)
	resetForwardBlockState(t)
	memberReqCleanup(t, []int64{env.group}, nil)
	t.Cleanup(func() { cache.InvalidateAdminCache(env.group) })
	cache.InvalidateAdminCache(env.group)

	ctx := context.Background()
	if _, err := staff.AddForwardBlock(ctx, env.staffChat, env.source, fwdSourceTitle, env.owner); err != nil {
		t.Fatalf("list the source: %v", err)
	}
	env.setAction(action)
	return env
}

// resetForwardBlockState clears the watcher's memo, latch and log gates now and when the test ends.
func resetForwardBlockState(t *testing.T) {
	t.Helper()
	reset := func() {
		forwardBlockAuthMemo.reset()
		forwardBlockPunished.reset()
		for i := range forwardBlockWarnGates {
			forwardBlockWarnGates[i].mu.Lock()
			forwardBlockWarnGates[i].last = nil
			forwardBlockWarnGates[i].capWarned = false
			forwardBlockWarnGates[i].mu.Unlock()
		}
	}
	reset()
	t.Cleanup(reset)
}

// setAction stores the list's action through the repository.
func (e *fwdWatchEnv) setAction(action string) {
	e.t.Helper()
	if err := staff.SetForwardBlockAction(context.Background(), e.staffChat, e.owner, action); err != nil {
		e.t.Fatalf("set the action %q: %v", action, err)
	}
}

// member returns a plain member with a fresh ID, whose rows go when the test ends.
func (e *fwdWatchEnv) member() gotgbot.User {
	e.t.Helper()
	user := gotgbot.User{Id: uniqueLinkOwnerID(), FirstName: "Member"}
	memberReqCleanup(e.t, nil, []int64{user.Id})
	return user
}

// channelOrigin is the origin of a forward from a channel.
func channelOrigin(id int64) gotgbot.MessageOrigin {
	return gotgbot.MessageOriginChannel{Date: 1, Chat: gotgbot.Chat{Id: id, Type: "channel", Title: fwdSourceTitle}, MessageId: 5}
}

// forwardUpdate builds a message update in chat from from carrying the origin.
func (e *fwdWatchEnv) forwardUpdate(chat gotgbot.Chat, from *gotgbot.User, origin gotgbot.MessageOrigin) *gotgbot.Update {
	update := e.messageUpdate(chat, from, "forwarded text")
	update.Message.ForwardOrigin = origin
	return update
}

// forward runs a forward of the listed source into the linked group as from and returns its message.
func (e *fwdWatchEnv) forward(from gotgbot.User) *gotgbot.Message {
	e.t.Helper()
	update := e.forwardUpdate(e.groupChat, &from, channelOrigin(e.source))
	e.process(update)
	return update.Message
}

// deletes returns the deleteMessage calls addressed to the linked group.
func (e *fwdWatchEnv) deletes() []moduleBotCall {
	return e.callsTo("deleteMessage", e.group)
}

// deletedIDs returns the message IDs the bot deleted in the linked group.
func (e *fwdWatchEnv) deletedIDs() []int64 {
	var ids []int64
	for _, call := range e.deletes() {
		ids = append(ids, staffParamInt(call.Params, "message_id"))
	}
	return ids
}

// activity counts what the bot did in chat: deleted messages, ban, restrict and unban calls, and
// messages sent.
func (e *fwdWatchEnv) activity(chat int64) (deleted, moderated, sent int) {
	return len(e.callsTo("deleteMessage", chat)), len(e.writes(chat)), len(e.fake.sentTo(chat))
}

// groupWrites counts the ban, restrict and unban calls and the messages the bot sent to the linked
// group.
func (e *fwdWatchEnv) groupWrites() (moderation, sent int) {
	_, moderation, sent = e.activity(e.group)
	return moderation, sent
}

// wantUntouchedIn requires that the bot deleted, punished and sent nothing in chat.
func (e *fwdWatchEnv) wantUntouchedIn(t *testing.T, chat int64, what string) {
	t.Helper()
	if deleted, moderated, sent := e.activity(chat); deleted != 0 || moderated != 0 || sent != 0 {
		t.Fatalf("%s: the bot deleted %d, moderated %d and sent %d, want nothing", what, deleted, moderated, sent)
	}
}

// wantUntouched requires that the bot deleted, punished and sent nothing in the linked group.
func (e *fwdWatchEnv) wantUntouched(t *testing.T, what string) {
	t.Helper()
	e.wantUntouchedIn(t, e.group, what)
}

// totalCalls counts every request the bot made.
func (e *fwdWatchEnv) totalCalls() int {
	client := e.fake.moduleBotClient
	client.mu.Lock()
	defer client.mu.Unlock()
	return len(client.calls)
}

// warnLines returns the logged lines of the watcher that are Warn level or above.
func (e *fwdWatchEnv) warnLines() []string {
	var lines []string
	for _, entry := range e.logs.AllEntries() {
		if strings.Contains(entry.Message, "forward blocklist") {
			lines = append(lines, entry.Message)
		}
	}
	return lines
}
