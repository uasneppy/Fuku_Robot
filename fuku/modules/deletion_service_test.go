//go:build testtools

package modules

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"

	"github.com/uasneppy/fuku_robot/fuku/db/deletion"
	"github.com/uasneppy/fuku_robot/fuku/db/greetings"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// deletionSetRule stores a rule through the apply function, as the settings page does.
func deletionSetRule(t *testing.T, chatID int64, kind string, delay int) {
	t.Helper()
	res := applyDeleteRule(context.Background(), chatID, kind, deletionSet(delay))
	if res.Outcome != deletionApplied {
		t.Fatalf("applyDeleteRule(%d, %s, %d) = %+v, want applied", chatID, kind, delay, res)
	}
}

// TestServiceWatcherDeletesNow is the phase's tracer: a pin notice goes from a rule stored by the
// apply function, through the real dispatcher, to one paced deleteMessage call.
func TestServiceWatcherDeletesNow(t *testing.T) {
	env := newDeletionEnv(t)
	otherChat := gotgbot.Chat{Id: uniqueModuleChatID(), Type: "supergroup", Title: "Other"}
	deletionCleanup(t, otherChat.Id)

	t.Run("no rule leaves the message alone", func(t *testing.T) {
		env.sendPin()
		if calls := env.deletes(env.chat.Id); len(calls) != 0 {
			t.Fatalf("deleteMessage calls with no rule = %d, want 0", len(calls))
		}
	})

	t.Run("a rule of another chat never applies", func(t *testing.T) {
		deletionSetRule(t, otherChat.Id, deletion.KindPinned, 0)
		env.sendPin()
		if calls := env.deletes(env.chat.Id); len(calls) != 0 {
			t.Fatalf("deleteMessage calls with only another chat's rule = %d, want 0", len(calls))
		}
	})

	deletionSetRule(t, env.chat.Id, deletion.KindPinned, 0)

	t.Run("a rule of zero deletes the notice at once through the pacer", func(t *testing.T) {
		id := env.sendPin()
		calls := env.deletes(env.chat.Id)
		if len(calls) != 1 {
			t.Fatalf("deleteMessage calls = %d, want 1", len(calls))
		}
		if got := staffParamInt(calls[0].Params, "message_id"); got != id {
			t.Fatalf("deleted message %d, want the notice %d", got, id)
		}
		if rows := env.queued(env.chat.Id); len(rows) != 0 {
			t.Fatalf("queue rows after an inline delete = %d, want 0", len(rows))
		}
	})

	t.Run("content is never deleted", func(t *testing.T) {
		env.sendService(&env.admin, func(m *gotgbot.Message) { m.Text = "hello" })
		if calls := env.deletes(env.chat.Id); len(calls) != 1 {
			t.Fatalf("deleteMessage calls after a text message = %d, want still 1", len(calls))
		}
	})
}

func TestServiceWatcherSkipsStaffGroup(t *testing.T) {
	t.Run("a Staff Group is never touched", func(t *testing.T) {
		env := newDeletionEnv(t)
		env.makeStaffGroup(env.chat.Id)
		deletionSetRule(t, env.chat.Id, deletion.KindPinned, 0)
		env.sendPin()
		if calls := env.deletes(env.chat.Id); len(calls) != 0 {
			t.Fatalf("deleteMessage calls in a Staff Group = %d, want 0", len(calls))
		}
		if rows := env.queued(env.chat.Id); len(rows) != 0 {
			t.Fatalf("queue rows in a Staff Group = %d, want 0", len(rows))
		}
	})

	t.Run("a failing Staff Group read deletes nothing", func(t *testing.T) {
		env := newDeletionEnv(t)
		deletionSetRule(t, env.chat.Id, deletion.KindPinned, 0)
		mreqSwap(t, &deletionStaffLookup, func(int64) (*models.StaffGroup, error) {
			return nil, errors.New("staff lookup failed")
		})
		env.sendPin()
		if calls := env.deletes(env.chat.Id); len(calls) != 0 {
			t.Fatalf("deleteMessage calls after a failed Staff Group read = %d, want 0", len(calls))
		}
	})
}

// deletionAfterWatcher registers a second handler in the watcher's group and returns the count of
// messages it saw. It runs only when the watcher returned ext.ContinueGroups: any other result
// leaves the group.
func deletionAfterWatcher(env *deletionEnv) *atomic.Int64 {
	var ran atomic.Int64
	env.dispatcher.AddHandlerToGroup(
		handlers.NewMessage(func(*gotgbot.Message) bool { return true }, func(*gotgbot.Bot, *ext.Context) error {
			ran.Add(1)
			return ext.ContinueGroups
		}).SetAllowBot(true),
		serviceDeleteGroup,
	)
	return &ran
}

func TestServiceWatcherSchedulesAfterN(t *testing.T) {
	env := newDeletionEnv(t)
	deletionSetRule(t, env.chat.Id, deletion.KindPinned, 600)

	id := env.sendPin()
	if calls := env.deletes(env.chat.Id); len(calls) != 0 {
		t.Fatalf("deleteMessage calls for a rule of 600 s = %d, want 0", len(calls))
	}
	rows := env.queued(env.chat.Id)
	if len(rows) != 1 {
		t.Fatalf("queue rows = %d, want 1", len(rows))
	}
	row := rows[0]
	if row.MessageID != id || row.Kind != deletion.KindPinned || row.State != models.ScheduledDeletionPending {
		t.Fatalf("row = %+v, want message %d, kind %s, pending", row, id, deletion.KindPinned)
	}
	if !row.SentAt.Equal(env.now) || !row.DeleteAt.Equal(env.now.Add(600*time.Second)) {
		t.Fatalf("sent_at = %s, delete_at = %s, want %s and +600 s", row.SentAt, row.DeleteAt, env.now)
	}

	t.Run("the same update twice leaves one row", func(t *testing.T) {
		env.deliver(id, env.chat, &env.admin, func(m *gotgbot.Message) {
			m.PinnedMessage = gotgbot.Message{MessageId: 1, Chat: env.chat}
		})
		if rows := env.queued(env.chat.Id); len(rows) != 1 {
			t.Fatalf("queue rows after a replay = %d, want 1", len(rows))
		}
	})

	redeliver := func() {
		env.deliver(id, env.chat, &env.admin, func(m *gotgbot.Message) {
			m.PinnedMessage = gotgbot.Message{MessageId: 1, Chat: env.chat}
		})
	}

	t.Run("the earlier time wins and never moves later", func(t *testing.T) {
		deletionSetRule(t, env.chat.Id, deletion.KindPinned, 60)
		redeliver()
		if got := env.queued(env.chat.Id)[0].DeleteAt; !got.Equal(env.now.Add(60 * time.Second)) {
			t.Fatalf("delete_at after a rule of 60 s = %s, want +60 s", got)
		}
		deletionSetRule(t, env.chat.Id, deletion.KindPinned, 3600)
		redeliver()
		if got := env.queued(env.chat.Id)[0].DeleteAt; !got.Equal(env.now.Add(60 * time.Second)) {
			t.Fatalf("delete_at after a later rule of 3600 s = %s, want it kept at +60 s", got)
		}
	})

	t.Run("a failed queue write writes nothing, logs the chat only and keeps the group going", func(t *testing.T) {
		hook := mreqLogCapture(t)
		ran := deletionAfterWatcher(env)
		mreqSwap(t, &deletionQueueWrite, func(context.Context, deletion.Entry) (bool, error) {
			return false, errors.New("queue is down")
		})
		env.sendPin()
		if rows := env.queued(env.chat.Id); len(rows) != 1 {
			t.Fatalf("queue rows after a failed write = %d, want the 1 from before", len(rows))
		}
		if ran.Load() != 1 {
			t.Fatalf("a later handler of the group ran %d times, want 1", ran.Load())
		}
		chat := strconv.FormatInt(env.chat.Id, 10)
		var seen bool
		for _, line := range mreqLogLines(hook) {
			if strings.Contains(line, "queue") && strings.Contains(line, chat) {
				seen = true
			}
		}
		if !seen {
			t.Fatalf("no log line names the chat %s for the failed write: %v", chat, mreqLogLines(hook))
		}
	})
}

func TestServiceWatcherZeroWritesNoRow(t *testing.T) {
	env := newDeletionEnv(t)
	deletionSetRule(t, env.chat.Id, deletion.KindTitle, 0)
	env.sendService(&env.admin, func(m *gotgbot.Message) { m.NewChatTitle = "A new title" })
	if calls := env.deletes(env.chat.Id); len(calls) != 1 {
		t.Fatalf("deleteMessage calls for a rule of 0 = %d, want 1", len(calls))
	}
	if rows := env.queued(env.chat.Id); len(rows) != 0 {
		t.Fatalf("queue rows for a rule of 0 = %d, want none", len(rows))
	}
}

func TestServiceWatcherSenders(t *testing.T) {
	env := newDeletionEnv(t)
	deletionSetRule(t, env.chat.Id, deletion.KindPinned, 0)
	ran := deletionAfterWatcher(env)
	bot := gotgbot.User{Id: uniqueLinkOwnerID(), IsBot: true, FirstName: "Helper"}
	member := gotgbot.User{Id: uniqueLinkOwnerID(), FirstName: "Member"}
	pin := func(m *gotgbot.Message) { m.PinnedMessage = gotgbot.Message{MessageId: 1, Chat: env.chat} }

	cases := []struct {
		name  string
		from  *gotgbot.User
		apply func(*gotgbot.Message)
	}{
		{"a user", &member, pin},
		{"a bot", &bot, pin},
		{"an anonymous admin", &env.admin, func(m *gotgbot.Message) {
			pin(m)
			m.SenderChat = &env.chat
		}},
		{"no sender at all", nil, pin},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env.sendService(tc.from, tc.apply)
			if calls := env.deletes(env.chat.Id); len(calls) != i+1 {
				t.Fatalf("deleteMessage calls after %s = %d, want %d", tc.name, len(calls), i+1)
			}
			if got := ran.Load(); got != int64(i+1) {
				t.Fatalf("a later handler of the group ran %d times after %s, want %d: the update was ended", got, tc.name, i+1)
			}
		})
	}
}

func TestServiceWatcherNeverEndsUpdate(t *testing.T) {
	pin := func(chat gotgbot.Chat) func(*gotgbot.Message) {
		return func(m *gotgbot.Message) { m.PinnedMessage = gotgbot.Message{MessageId: 1, Chat: chat} }
	}
	cases := []struct {
		name  string
		setup func(t *testing.T, env *deletionEnv)
		chat  func(env *deletionEnv) gotgbot.Chat
		apply func(env *deletionEnv, chat gotgbot.Chat) func(*gotgbot.Message)
	}{
		{"a deletable message", func(t *testing.T, env *deletionEnv) { deletionSetRule(t, env.chat.Id, deletion.KindPinned, 0) },
			nil, func(_ *deletionEnv, c gotgbot.Chat) func(*gotgbot.Message) { return pin(c) }},
		{"a panic inside the body", func(t *testing.T, env *deletionEnv) {
			deletionSetRule(t, env.chat.Id, deletion.KindPinned, 0)
			mreqSwap(t, &deletionRulesLookup, func(context.Context, int64) (*deletion.Rules, error) {
				panic("injected panic in the rules read")
			})
		}, nil, func(_ *deletionEnv, c gotgbot.Chat) func(*gotgbot.Message) { return pin(c) }},
		{"a message that is not a service type", func(t *testing.T, env *deletionEnv) { deletionSetRule(t, env.chat.Id, deletion.KindPinned, 0) },
			nil, func(*deletionEnv, gotgbot.Chat) func(*gotgbot.Message) {
				return func(m *gotgbot.Message) { m.Text = "plain" }
			}},
		{"an unreadable rules read", func(t *testing.T, env *deletionEnv) {
			mreqSwap(t, &deletionRulesLookup, func(context.Context, int64) (*deletion.Rules, error) {
				return nil, errors.New("rules are down")
			})
		}, nil, func(_ *deletionEnv, c gotgbot.Chat) func(*gotgbot.Message) { return pin(c) }},
		{"a Staff Group", func(t *testing.T, env *deletionEnv) {
			env.makeStaffGroup(env.chat.Id)
			deletionSetRule(t, env.chat.Id, deletion.KindPinned, 0)
		}, nil, func(_ *deletionEnv, c gotgbot.Chat) func(*gotgbot.Message) { return pin(c) }},
		{"a private chat", nil, func(*deletionEnv) gotgbot.Chat {
			return gotgbot.Chat{Id: uniqueLinkOwnerID(), Type: "private"}
		}, func(_ *deletionEnv, c gotgbot.Chat) func(*gotgbot.Message) { return pin(c) }},
		{"a channel", nil, func(*deletionEnv) gotgbot.Chat {
			return gotgbot.Chat{Id: uniqueModuleChatID(), Type: "channel", Title: "News"}
		}, func(_ *deletionEnv, c gotgbot.Chat) func(*gotgbot.Message) { return pin(c) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := newDeletionEnv(t)
			chat := env.chat
			if tc.chat != nil {
				chat = tc.chat(env)
				deletionCleanup(t, chat.Id)
			}
			if tc.setup != nil {
				tc.setup(t, env)
			}
			ran := deletionAfterWatcher(env)
			env.sendServiceTo(chat, &env.admin, tc.apply(env, chat))
			if ran.Load() != 1 {
				t.Fatalf("a later handler of the group ran %d times, want 1: the watcher ended the update or returned nil", ran.Load())
			}
			if calls := env.deletes(chat.Id); chat.Id != env.chat.Id && len(calls) != 0 {
				t.Fatalf("deleteMessage calls in a chat that is not a group = %d, want 0", len(calls))
			}
		})
	}
}

// deletionJoinApply makes the message a "new members joined" notice naming member.
func deletionJoinApply(member gotgbot.User) func(*gotgbot.Message) {
	return func(m *gotgbot.Message) { m.NewChatMembers = []gotgbot.User{member} }
}

// deletionLeftApply makes the message a "member left" notice naming member.
func deletionLeftApply(member gotgbot.User) func(*gotgbot.Message) {
	return func(m *gotgbot.Message) { m.LeftChatMember = &member }
}

// deletionAfterCleanService registers a handler in group 0 after the greetings module's and
// returns how many messages it saw: cleanService ends the update, so it sees none.
func deletionAfterCleanService(env *deletionEnv) *atomic.Int64 {
	var ran atomic.Int64
	env.dispatcher.AddHandler(
		handlers.NewMessage(func(*gotgbot.Message) bool { return true }, func(*gotgbot.Bot, *ext.Context) error {
			ran.Add(1)
			return ext.ContinueGroups
		}).SetAllowBot(true),
	)
	return &ran
}

// methods returns the Telegram methods the fake saw, in the order they arrived.
func (e *deletionEnv) methods() []string {
	e.t.Helper()
	deletionInline.wait()
	e.fake.callsFor("sendMessage")
	client := e.fake.moduleBotClient
	client.mu.Lock()
	defer client.mu.Unlock()
	order := make([]string, 0, len(client.calls))
	for _, call := range client.calls {
		order = append(order, call.Method)
	}
	return order
}

var deletionJoinLeft = []struct {
	name  string
	kind  string
	apply func(gotgbot.User) func(*gotgbot.Message)
}{
	{"joined", deletion.KindJoined, deletionJoinApply},
	{"left", deletion.KindLeft, deletionLeftApply},
}

// deletionSetCleanService stores the shared /cleanservice flag.
func deletionSetCleanService(t *testing.T, chatID int64, on bool) {
	t.Helper()
	if err := greetings.SetShouldCleanService(chatID, on); err != nil {
		t.Fatalf("SetShouldCleanService(%d, %v) error = %v", chatID, on, err)
	}
}

func TestCleanServiceEffectiveRule(t *testing.T) {
	ctx := context.Background()
	for _, k := range deletionJoinLeft {
		t.Run(k.name+": flag off with a stale row deletes nothing and writes no row", func(t *testing.T) {
			env := newDeletionEnv(t)
			ran := deletionAfterCleanService(env)
			deletionSetCleanService(t, env.chat.Id, false)
			if err := deletion.SetRule(ctx, env.chat.Id, k.kind, 600); err != nil {
				t.Fatalf("SetRule error = %v", err)
			}
			env.sendService(&env.admin, k.apply(gotgbot.User{Id: uniqueLinkOwnerID(), FirstName: "Member"}))
			if calls := env.deletes(env.chat.Id); len(calls) != 0 {
				t.Fatalf("deleteMessage calls = %d, want 0", len(calls))
			}
			if rows := env.queued(env.chat.Id); len(rows) != 0 {
				t.Fatalf("queue rows = %d, want 0", len(rows))
			}
			if ran.Load() != 0 {
				t.Fatalf("a later group-0 handler ran %d times: cleanService did not end the update", ran.Load())
			}
		})

		t.Run(k.name+": flag on with no row deletes the message once", func(t *testing.T) {
			env := newDeletionEnv(t)
			ran := deletionAfterCleanService(env)
			deletionSetCleanService(t, env.chat.Id, true)
			id := env.sendService(&env.admin, k.apply(gotgbot.User{Id: uniqueLinkOwnerID(), FirstName: "Member"}))
			calls := env.deletes(env.chat.Id)
			if len(calls) != 1 || staffParamInt(calls[0].Params, "message_id") != id {
				t.Fatalf("deleteMessage calls = %+v, want one for message %d", calls, id)
			}
			if rows := env.queued(env.chat.Id); len(rows) != 0 {
				t.Fatalf("queue rows = %d, want 0", len(rows))
			}
			if ran.Load() != 0 {
				t.Fatalf("a later group-0 handler ran %d times: cleanService did not end the update", ran.Load())
			}
		})

		t.Run(k.name+": flag on with a row of 600 queues the message at its date plus 600", func(t *testing.T) {
			env := newDeletionEnv(t)
			res := applyCleanServiceRule(ctx, env.chat.Id, k.kind,
				deletion.JoinLeftState{Mode: deletion.JoinLeftAfter, DelaySecs: 600})
			if res.Outcome != deletionApplied {
				t.Fatalf("applyCleanServiceRule = %+v, want applied", res)
			}
			id := env.sendService(&env.admin, k.apply(gotgbot.User{Id: uniqueLinkOwnerID(), FirstName: "Member"}))
			if calls := env.deletes(env.chat.Id); len(calls) != 0 {
				t.Fatalf("deleteMessage calls = %d, want 0 yet", len(calls))
			}
			rows := env.queued(env.chat.Id)
			if len(rows) != 1 || rows[0].MessageID != id || rows[0].Kind != k.kind ||
				!rows[0].DeleteAt.Equal(env.now.Add(600*time.Second)) {
				t.Fatalf("queue rows = %+v, want one for message %d at +600 s", rows, id)
			}
		})

		t.Run(k.name+": a failed flag read deletes nothing and still ends the update", func(t *testing.T) {
			env := newDeletionEnv(t)
			ran := deletionAfterCleanService(env)
			deletionSetCleanService(t, env.chat.Id, true)
			mreqSwap(t, &deletionFlagLookup, func(context.Context, int64) (*models.GreetingSettings, error) {
				return nil, errors.New("flag read failed")
			})
			env.sendService(&env.admin, k.apply(gotgbot.User{Id: uniqueLinkOwnerID(), FirstName: "Member"}))
			if calls := env.deletes(env.chat.Id); len(calls) != 0 {
				t.Fatalf("deleteMessage calls after a failed flag read = %d, want 0", len(calls))
			}
			if rows := env.queued(env.chat.Id); len(rows) != 0 {
				t.Fatalf("queue rows after a failed flag read = %d, want 0", len(rows))
			}
			if ran.Load() != 0 {
				t.Fatalf("a later group-0 handler ran %d times: cleanService did not end the update", ran.Load())
			}
		})
	}
}

func TestCleanServiceDeletesAfterJoin(t *testing.T) {
	env := newDeletionEnv(t)
	deletionSetCleanService(t, env.chat.Id, true)
	env.sendService(&env.admin, deletionJoinApply(gotgbot.User{Id: uniqueLinkOwnerID(), FirstName: "Newcomer"}))

	order := env.methods()
	send, del := -1, -1
	for i, method := range order {
		switch {
		case method == "sendMessage" && send < 0:
			send = i
		case method == "deleteMessage" && del < 0:
			del = i
		}
	}
	if send < 0 || del < 0 || send > del {
		t.Fatalf("call order = %v, want the welcome (sendMessage) before the join message's deleteMessage", order)
	}
	sent := env.fake.sentTo(env.chat.Id)
	if len(sent) != 1 {
		t.Fatalf("welcomes sent = %d, want 1", len(sent))
	}
	for _, key := range []string{"reply_parameters", "reply_to_message_id"} {
		if _, replies := sent[0].Params[key]; replies {
			t.Fatalf("the welcome carries %q: it must not depend on the join message surviving", key)
		}
	}
}

func TestServiceWatcherBotJoin(t *testing.T) {
	ctx := context.Background()
	bot := gotgbot.User{Id: uniqueLinkOwnerID(), IsBot: true, FirstName: "Adder"}
	member := gotgbot.User{Id: uniqueLinkOwnerID(), FirstName: "Member"}
	for _, k := range deletionJoinLeft {
		t.Run(k.name+" by a bot: flag on with no row deletes it", func(t *testing.T) {
			env := newDeletionEnv(t)
			deletionSetCleanService(t, env.chat.Id, true)
			id := env.sendService(&bot, k.apply(member))
			calls := env.deletes(env.chat.Id)
			if len(calls) != 1 || staffParamInt(calls[0].Params, "message_id") != id {
				t.Fatalf("deleteMessage calls = %+v, want one for message %d", calls, id)
			}
		})

		t.Run(k.name+" by a bot: flag off with a stale row does nothing", func(t *testing.T) {
			env := newDeletionEnv(t)
			deletionSetCleanService(t, env.chat.Id, false)
			if err := deletion.SetRule(ctx, env.chat.Id, k.kind, 600); err != nil {
				t.Fatalf("SetRule error = %v", err)
			}
			env.sendService(&bot, k.apply(member))
			if got := len(env.deletes(env.chat.Id)) + len(env.queued(env.chat.Id)); got != 0 {
				t.Fatalf("a bot-caused message with the flag off acted %d times, want 0", got)
			}
		})

		t.Run(k.name+" by a bot: a row of 600 queues it", func(t *testing.T) {
			env := newDeletionEnv(t)
			if res := applyCleanServiceRule(ctx, env.chat.Id, k.kind,
				deletion.JoinLeftState{Mode: deletion.JoinLeftAfter, DelaySecs: 600}); res.Outcome != deletionApplied {
				t.Fatalf("applyCleanServiceRule = %+v, want applied", res)
			}
			id := env.sendService(&bot, k.apply(member))
			rows := env.queued(env.chat.Id)
			if len(rows) != 1 || rows[0].MessageID != id || !rows[0].DeleteAt.Equal(env.now.Add(600*time.Second)) {
				t.Fatalf("queue rows = %+v, want one for message %d at +600 s", rows, id)
			}
		})
	}

	t.Run("a human sender is handled once, by cleanService", func(t *testing.T) {
		env := newDeletionEnv(t)
		deletionSetCleanService(t, env.chat.Id, true)
		env.sendService(&env.admin, deletionJoinApply(gotgbot.User{Id: uniqueLinkOwnerID(), FirstName: "Member"}))
		if calls := env.deletes(env.chat.Id); len(calls) != 1 {
			t.Fatalf("deleteMessage calls = %d, want exactly 1 (no double handling)", len(calls))
		}
	})

	t.Run("an anonymous admin is handled once, by cleanService", func(t *testing.T) {
		env := newDeletionEnv(t)
		deletionSetCleanService(t, env.chat.Id, true)
		env.sendService(&env.admin, func(m *gotgbot.Message) {
			deletionJoinApply(gotgbot.User{Id: uniqueLinkOwnerID(), FirstName: "Member"})(m)
			m.SenderChat = &env.chat
		})
		if calls := env.deletes(env.chat.Id); len(calls) != 1 {
			t.Fatalf("deleteMessage calls = %d, want exactly 1", len(calls))
		}
	})

	t.Run("a message with no sender is handled by neither path", func(t *testing.T) {
		for _, k := range deletionJoinLeft {
			env := newDeletionEnv(t)
			deletionSetCleanService(t, env.chat.Id, true)
			env.sendService(nil, k.apply(gotgbot.User{Id: uniqueLinkOwnerID(), FirstName: "Member"}))
			if got := len(env.deletes(env.chat.Id)) + len(env.queued(env.chat.Id)); got != 0 {
				t.Fatalf("%s: a join or leave with no sender acted %d times, want 0", k.name, got)
			}
		}
	})

	t.Run("a join the lockdown guard ended is handled by neither path", func(t *testing.T) {
		env := newDeletionEnv(t)
		LoadLockdown(env.dispatcher)
		startPanelLockdown(t, env.chat.Id, true)
		deletionSetCleanService(t, env.chat.Id, true)
		stranger := gotgbot.User{Id: uniqueLinkOwnerID(), FirstName: "Stranger"}
		env.sendService(&stranger, deletionJoinApply(gotgbot.User{Id: uniqueLinkOwnerID(), FirstName: "Raider"}))
		if got := len(env.deletes(env.chat.Id)) + len(env.queued(env.chat.Id)); got != 0 {
			t.Fatalf("a join ended by the lockdown guard acted %d times, want 0", got)
		}
	})
}
