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
