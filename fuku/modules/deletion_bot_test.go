//go:build testtools

package modules

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db"
	dbcache "github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/deletion"
	"github.com/uasneppy/fuku_robot/fuku/db/greetings"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// setBotRule stores the delay of a bot kind for a chat through the repository the page uses.
func (e *deletionEnv) setBotRule(chatID int64, kind string, delay int) {
	e.t.Helper()
	if err := deletion.SetRule(e.t.Context(), chatID, kind, delay); err != nil {
		e.t.Fatalf("SetRule(%d, %s, %d) error = %v", chatID, kind, delay, err)
	}
}

// removeBotRule removes the rule of a bot kind for a chat.
func (e *deletionEnv) removeBotRule(chatID int64, kind string) {
	e.t.Helper()
	if err := deletion.RemoveRule(e.t.Context(), chatID, kind); err != nil {
		e.t.Fatalf("RemoveRule(%d, %s) error = %v", chatID, kind, err)
	}
}

// greetingMemberCleanup deletes the users rows and cached copies of the members a test joins, and
// the bookkeeping rows of the join, when the test ends.
func greetingMemberCleanup(t *testing.T, ids ...int64) {
	t.Helper()
	t.Cleanup(func() {
		db.DB.Where("user_id IN ?", ids).Delete(&models.User{})
		for _, id := range ids {
			dbcache.DeleteCache(dbcache.CacheKey("user", id))
		}
		dbcache.ResetLocalForTest()
	})
}

// newGreetingChat returns a fresh supergroup with a welcome text and a goodbye text stored, whose
// rows and cache entries are removed when the test ends.
func (e *deletionEnv) newGreetingChat(title string) gotgbot.Chat {
	e.t.Helper()
	chat := gotgbot.Chat{Id: uniqueModuleChatID(), Type: "supergroup", Title: title}
	deletionCleanup(e.t, chat.Id)
	if err := greetings.SetWelcomeText(chat.Id, "Welcome {first}", "", nil, db.TEXT); err != nil {
		e.t.Fatalf("SetWelcomeText error = %v", err)
	}
	if err := greetings.SetGoodbyeText(chat.Id, "Goodbye {first}", "", nil, db.TEXT); err != nil {
		e.t.Fatalf("SetGoodbyeText error = %v", err)
	}
	return chat
}

// member returns a fresh member whose rows are removed when the test ends.
func (e *deletionEnv) member(name string) gotgbot.User {
	e.t.Helper()
	user := gotgbot.User{Id: uniqueLinkOwnerID(), FirstName: name}
	greetingMemberCleanup(e.t, user.Id)
	return user
}

// joinMember runs a chat_member update that adds the member to the chat through the real dispatcher.
func (e *deletionEnv) joinMember(chat gotgbot.Chat, member gotgbot.User) {
	e.t.Helper()
	e.deliverMemberUpdate(chat, gotgbot.ChatMemberLeft{User: member}, gotgbot.ChatMemberMember{User: member})
}

// leaveMember runs a chat_member update that removes the member from the chat.
func (e *deletionEnv) leaveMember(chat gotgbot.Chat, member gotgbot.User) {
	e.t.Helper()
	e.deliverMemberUpdate(chat, gotgbot.ChatMemberMember{User: member}, gotgbot.ChatMemberLeft{User: member})
}

func (e *deletionEnv) deliverMemberUpdate(chat gotgbot.Chat, oldMember, newMember gotgbot.ChatMember) {
	e.t.Helper()
	id := e.updateID()
	update := &gotgbot.Update{UpdateId: id, ChatMember: &gotgbot.ChatMemberUpdated{
		Chat:          chat,
		From:          gotgbot.User{Id: 777000, FirstName: "Telegram"},
		Date:          deletionNow().Unix(),
		OldChatMember: oldMember,
		NewChatMember: newMember,
	}}
	if err := e.dispatcher.ProcessUpdate(e.bot, update, nil); err != nil {
		e.t.Fatalf("ProcessUpdate(%d) error = %v", id, err)
	}
}

// sentMessageIDs returns the IDs of the messages the bot sent to the chat whose text holds word.
func (e *deletionEnv) sentMessageIDs(chatID int64, word string) []int64 {
	e.t.Helper()
	var ids []int64
	for _, sent := range e.fake.sentTo(chatID) {
		if text, _ := sent.Params["text"].(string); strings.Contains(text, word) {
			ids = append(ids, sent.MessageID)
		}
	}
	return ids
}

// wantOneRow fails unless the chat's queue holds exactly one row, for messageID, of the kind, sent
// at the fixed clock and due delay seconds later.
func (e *deletionEnv) wantOneRow(chatID, messageID int64, kind string, delay time.Duration) {
	e.t.Helper()
	rows := e.queued(chatID)
	if len(rows) != 1 {
		e.t.Fatalf("queue rows of chat %d = %+v, want exactly one", chatID, rows)
	}
	row := rows[0]
	if row.MessageID != messageID || row.Kind != kind {
		e.t.Fatalf("queue row = message %d kind %s, want message %d kind %s", row.MessageID, row.Kind, messageID, kind)
	}
	if !row.SentAt.Equal(e.now) || !row.DeleteAt.Equal(e.now.Add(delay)) {
		e.t.Fatalf("queue row sent_at %s delete_at %s, want %s and %s", row.SentAt, row.DeleteAt, e.now, e.now.Add(delay))
	}
}

func TestScheduleBotMessage(t *testing.T) {
	env := newDeletionEnv(t)
	ctx := t.Context()

	msgIDs := int64(5000)
	nextID := func() int64 { msgIDs++; return msgIDs }
	chatOf := func(typ string) gotgbot.Chat {
		chat := gotgbot.Chat{Id: uniqueModuleChatID(), Type: typ, Title: "Bot Message Chat"}
		deletionCleanup(t, chat.Id)
		return chat
	}

	cases := []struct {
		name string
		// setup prepares the chat and returns the message to hand to the helper (nil for none).
		setup    func(chat gotgbot.Chat) *gotgbot.Message
		chatType string
		rule     int // stored delay of bot:welcome; -1 stores none
		wantRow  bool
		wantDue  time.Duration
		// wantReads is the number of rule reads expected; -1 does not check.
		wantReads int
	}{
		{name: "nil message", chatType: "supergroup", rule: 60,
			setup:     func(gotgbot.Chat) *gotgbot.Message { return nil },
			wantReads: 0},
		{name: "private chat", chatType: "private", rule: 60, wantReads: 0},
		{name: "channel", chatType: "channel", rule: 60, wantReads: 0},
		{name: "empty chat type", chatType: "", rule: 60, wantReads: 0},
		{name: "no rule for the kind", chatType: "supergroup", rule: -1, wantReads: 1},
		{name: "supergroup with a rule", chatType: "supergroup", rule: 60, wantRow: true, wantDue: 60 * time.Second, wantReads: 1},
		{name: "basic group with a rule", chatType: "group", rule: 90, wantRow: true, wantDue: 90 * time.Second, wantReads: 1},
		{name: "stored zero is due now", chatType: "supergroup", rule: 0, wantRow: true, wantDue: 0, wantReads: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			chat := chatOf(tc.chatType)
			if tc.rule >= 0 {
				env.setBotRule(chat.Id, deletion.KindBotWelcome, tc.rule)
			}
			var msg *gotgbot.Message
			if tc.setup != nil {
				msg = tc.setup(chat)
			} else {
				msg = &gotgbot.Message{MessageId: nextID(), Date: deletionNow().Unix(), Chat: chat}
			}
			reads := 0
			mreqSwap(t, &deletionRulesLookup, func(c context.Context, id int64) (*deletion.Rules, error) {
				reads++
				return deletion.GetRules(c, id)
			})

			scheduleBotMessage(msg, deletion.KindBotWelcome)

			if reads != tc.wantReads {
				t.Fatalf("rule reads = %d, want %d", reads, tc.wantReads)
			}
			rows := env.queued(chat.Id)
			if !tc.wantRow {
				if len(rows) != 0 {
					t.Fatalf("queue rows = %+v, want none", rows)
				}
				return
			}
			env.wantOneRow(chat.Id, msg.MessageId, deletion.KindBotWelcome, tc.wantDue)
		})
	}

	t.Run("a chat latched gone writes no row", func(t *testing.T) {
		chat := chatOf("supergroup")
		env.setBotRule(chat.Id, deletion.KindBotWelcome, 60)
		deletionSetLatch(ctx, chat.Id, deletionScopeGone)
		t.Cleanup(func() { env.redis.Del(deletionLatchKey(chat.Id)) })
		scheduleBotMessage(&gotgbot.Message{MessageId: nextID(), Date: deletionNow().Unix(), Chat: chat}, deletion.KindBotWelcome)
		if rows := env.queued(chat.Id); len(rows) != 0 {
			t.Fatalf("queue rows of a latched chat = %+v, want none", rows)
		}
	})

	t.Run("a Staff Group writes no row", func(t *testing.T) {
		chat := chatOf("supergroup")
		env.setBotRule(chat.Id, deletion.KindBotWelcome, 60)
		env.makeStaffGroup(chat.Id)
		scheduleBotMessage(&gotgbot.Message{MessageId: nextID(), Date: deletionNow().Unix(), Chat: chat}, deletion.KindBotWelcome)
		if rows := env.queued(chat.Id); len(rows) != 0 {
			t.Fatalf("queue rows of a Staff Group = %+v, want none", rows)
		}
	})

	t.Run("a failed rules read writes nothing and returns", func(t *testing.T) {
		chat := chatOf("supergroup")
		env.setBotRule(chat.Id, deletion.KindBotWelcome, 60)
		mreqSwap(t, &deletionRulesLookup, func(context.Context, int64) (*deletion.Rules, error) {
			return nil, errors.New("rules unreadable")
		})
		scheduleBotMessage(&gotgbot.Message{MessageId: nextID(), Date: deletionNow().Unix(), Chat: chat}, deletion.KindBotWelcome)
		if rows := env.queued(chat.Id); len(rows) != 0 {
			t.Fatalf("queue rows after a failed read = %+v, want none", rows)
		}
	})

	t.Run("a nil rules answer writes nothing", func(t *testing.T) {
		chat := chatOf("supergroup")
		mreqSwap(t, &deletionRulesLookup, func(context.Context, int64) (*deletion.Rules, error) { return nil, nil })
		scheduleBotMessage(&gotgbot.Message{MessageId: nextID(), Date: deletionNow().Unix(), Chat: chat}, deletion.KindBotWelcome)
		if rows := env.queued(chat.Id); len(rows) != 0 {
			t.Fatalf("queue rows after a nil answer = %+v, want none", rows)
		}
	})

	t.Run("a failed write returns normally", func(t *testing.T) {
		chat := chatOf("supergroup")
		env.setBotRule(chat.Id, deletion.KindBotWelcome, 60)
		writes := 0
		mreqSwap(t, &deletionQueueWrite, func(context.Context, deletion.Entry) (bool, error) {
			writes++
			return false, errors.New("queue unwritable")
		})
		scheduleBotMessage(&gotgbot.Message{MessageId: nextID(), Date: deletionNow().Unix(), Chat: chat}, deletion.KindBotWelcome)
		if writes != 1 {
			t.Fatalf("queue writes attempted = %d, want 1", writes)
		}
		if rows := env.queued(chat.Id); len(rows) != 0 {
			t.Fatalf("queue rows after a failed write = %+v, want none", rows)
		}
	})

	t.Run("a delay override replaces the stored one", func(t *testing.T) {
		chat := chatOf("supergroup")
		env.setBotRule(chat.Id, deletion.KindBotWelcome, 60)
		override := 600
		id := nextID()
		scheduleBotRow(chat.Id, id, env.now, deletion.KindBotWelcome, &override)
		env.wantOneRow(chat.Id, id, deletion.KindBotWelcome, 600*time.Second)
	})
}

func TestScheduleBotMessageClampsDate(t *testing.T) {
	env := newDeletionEnv(t)

	cases := []struct {
		name string
		date int64
		// wantSentAt is the sent_at the row must carry.
		wantSentAt time.Time
	}{
		{name: "zero date means now", date: 0, wantSentAt: env.now},
		{name: "negative date means now", date: -5, wantSentAt: env.now},
		{name: "date five minutes ahead means now", date: env.now.Add(5 * time.Minute).Unix(), wantSentAt: env.now},
		{name: "date ten minutes ago is kept, never stretched", date: env.now.Add(-10 * time.Minute).Unix(),
			wantSentAt: env.now.Add(-10 * time.Minute)},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			chat := gotgbot.Chat{Id: uniqueModuleChatID(), Type: "supergroup", Title: "Clamp Chat"}
			deletionCleanup(t, chat.Id)
			env.setBotRule(chat.Id, deletion.KindBotRules, 60)
			scheduleBotMessage(&gotgbot.Message{MessageId: int64(7000 + i), Date: tc.date, Chat: chat}, deletion.KindBotRules)

			rows := env.queued(chat.Id)
			if len(rows) != 1 {
				t.Fatalf("queue rows = %+v, want one", rows)
			}
			if !rows[0].SentAt.Equal(tc.wantSentAt) {
				t.Fatalf("sent_at = %s, want %s", rows[0].SentAt, tc.wantSentAt)
			}
			if want := tc.wantSentAt.Add(60 * time.Second); !rows[0].DeleteAt.Equal(want) {
				t.Fatalf("delete_at = %s, want %s", rows[0].DeleteAt, want)
			}
		})
	}
}

func TestBotWelcomeScheduled(t *testing.T) {
	env := newDeletionEnv(t)
	chat := env.newGreetingChat("Welcome Chat")
	env.setBotRule(chat.Id, deletion.KindBotWelcome, 60)

	first := env.member("Alice")
	env.joinMember(chat, first)
	ids := env.sentMessageIDs(chat.Id, "Welcome")
	if len(ids) != 1 {
		t.Fatalf("welcomes sent = %v, want exactly one", ids)
	}
	env.wantOneRow(chat.Id, ids[0], deletion.KindBotWelcome, 60*time.Second)

	// With the rule removed the next welcome is sent and nothing more is queued.
	env.removeBotRule(chat.Id, deletion.KindBotWelcome)
	second := env.member("Bob")
	env.joinMember(chat, second)
	if got := env.sentMessageIDs(chat.Id, "Welcome"); len(got) != 2 {
		t.Fatalf("welcomes sent after the second join = %v, want two", got)
	}
	if rows := env.queued(chat.Id); len(rows) != 1 {
		t.Fatalf("queue rows with the kind off = %+v, want still the first row only", rows)
	}

	// A Staff Group gets its welcome as before and nothing is queued for it.
	staffChat := env.newGreetingChat("Staff Welcome Chat")
	env.setBotRule(staffChat.Id, deletion.KindBotWelcome, 60)
	env.makeStaffGroup(staffChat.Id)
	env.joinMember(staffChat, env.member("Carol"))
	if got := env.sentMessageIDs(staffChat.Id, "Welcome"); len(got) != 1 {
		t.Fatalf("welcomes sent in the Staff Group = %v, want one", got)
	}
	if rows := env.queued(staffChat.Id); len(rows) != 0 {
		t.Fatalf("queue rows of a Staff Group = %+v, want none", rows)
	}
}
