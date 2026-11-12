//go:build testtools

package modules

import (
	"slices"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db/approvals"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// memberSelfDestructRuleDelay is the delay of the member:selfdestruct rule most tests store.
const memberSelfDestructRuleDelay = 600

// selfDestructClock moves deletionNow for the test and returns the setter; the message dates the
// env sends and the worker's clock both follow it.
func selfDestructClock(t *testing.T, start time.Time) func(time.Time) {
	t.Helper()
	current := start
	mreqSwap(t, &deletionNow, func() time.Time { return current })
	return func(at time.Time) { current = at }
}

// TestMemberSelfDestructQueuesMessages is the plan's tracer (D-09, D-10, D-27, D-29): every kind of
// member message is offered by the watcher with no write, the flush writes one pending row each at
// the message's own date plus the delay, and the real worker deletes them at plus 601 s and not at
// plus 599 s. An administrator's and an approved member's messages are never queued, and a queued
// message named by a pinned_message service message loses its row.
func TestMemberSelfDestructQueuesMessages(t *testing.T) {
	t.Run("every member message is queued and deleted on time", func(t *testing.T) {
		env := newSelfDestructEnv(t, memberSelfDestructRuleDelay)
		setNow := selfDestructClock(t, env.now)
		member := env.member()
		via := gotgbot.User{Id: uniqueLinkOwnerID(), IsBot: true, FirstName: "Inline"}
		album := "album-" + time.Now().Format("150405.000000000")
		kinds := []struct {
			name  string
			apply func(*gotgbot.Message)
		}{
			{"text", func(m *gotgbot.Message) { m.Text = "hello" }},
			{"photo", func(m *gotgbot.Message) {
				m.Photo = []gotgbot.PhotoSize{{FileId: "f", FileUniqueId: "u", Width: 1, Height: 1}}
			}},
			{"sticker", func(m *gotgbot.Message) { m.Sticker = &gotgbot.Sticker{FileId: "s", FileUniqueId: "su"} }},
			{"poll", func(m *gotgbot.Message) { m.Poll = &gotgbot.Poll{Id: "p"} }},
			{"forward", func(m *gotgbot.Message) {
				m.ForwardOrigin = gotgbot.MessageOriginHiddenUser{Date: 1, SenderUserName: "hidden"}
			}},
			{"reply", func(m *gotgbot.Message) {
				m.Text = "reply"
				m.ReplyToMessage = &gotgbot.Message{MessageId: 1, Chat: env.chat}
			}},
			{"via bot", func(m *gotgbot.Message) { m.Text = "via"; m.ViaBot = &via }},
			{"command", commandMessage("/anything now")},
			{"album item one", func(m *gotgbot.Message) {
				m.MediaGroupId = album
				m.Photo = []gotgbot.PhotoSize{{FileId: "a1", FileUniqueId: "a1u", Width: 1, Height: 1}}
			}},
			{"album item two", func(m *gotgbot.Message) {
				m.MediaGroupId = album
				m.Photo = []gotgbot.PhotoSize{{FileId: "a2", FileUniqueId: "a2u", Width: 1, Height: 1}}
			}},
		}
		var ids []int64
		for _, kind := range kinds {
			ids = append(ids, env.sendMember(&member, kind.apply))
		}
		if rows := env.queuedNoWait(env.chat.Id); len(rows) != 0 {
			t.Fatalf("rows before the flush = %+v, want none: the watcher writes nothing", rows)
		}
		if got := memberDeleteTestFlush(t); got != len(kinds) {
			t.Fatalf("memberDeleteFlush() = %d, want %d written", got, len(kinds))
		}
		rows := env.selfDestructRows(env.chat.Id)
		if len(rows) != len(kinds) {
			t.Fatalf("rows after the flush = %d, want %d (one per message)", len(rows), len(kinds))
		}
		wantAt := env.now.Add(memberSelfDestructRuleDelay * time.Second)
		for i, row := range rows {
			if row.State != models.ScheduledDeletionPending || !row.DeleteAt.Equal(wantAt) || !row.SentAt.Equal(env.now) ||
				row.MessageID != ids[i] {
				t.Fatalf("row %d = %+v, want pending message %d sent %s due %s", i, row, ids[i], env.now, wantAt)
			}
		}

		setNow(env.now.Add(599 * time.Second))
		env.pass()
		if calls := env.batchCalls(env.chat.Id); len(calls) != 0 {
			t.Fatalf("deleteMessages calls at date+599s = %v, want none", calls)
		}
		setNow(env.now.Add(601 * time.Second))
		env.pass()
		calls := env.batchCalls(env.chat.Id)
		if len(calls) != 1 {
			t.Fatalf("deleteMessages calls at date+601s = %v, want one batch", calls)
		}
		got := slices.Clone(calls[0])
		slices.Sort(got)
		if !slices.Equal(got, ids) {
			t.Fatalf("deleted ids = %v, want %v", got, ids)
		}
		if rows := env.queuedNoWait(env.chat.Id); len(rows) != 0 {
			t.Fatalf("rows after the delete = %+v, want none", rows)
		}
	})

	t.Run("an administrator's and an approved member's messages are never queued", func(t *testing.T) {
		env := newSelfDestructEnv(t, memberSelfDestructRuleDelay)
		selfDestructClock(t, env.now)
		approved := env.member()
		if err := approvals.AddApprovedUser(env.chat.Id, approved.Id, env.admin.Id, "trusted"); err != nil {
			t.Fatalf("AddApprovedUser error = %v", err)
		}
		env.sendMember(&env.admin, func(m *gotgbot.Message) { m.Text = "admin" })
		env.sendMember(&approved, func(m *gotgbot.Message) { m.Text = "approved" })
		if got := memberDeleteTestFlush(t); got != 0 {
			t.Fatalf("memberDeleteFlush() = %d, want 0: nothing was offered", got)
		}
		if rows := env.queuedNoWait(env.chat.Id); len(rows) != 0 {
			t.Fatalf("rows = %+v, want none", rows)
		}
	})

	t.Run("a pinned_message service message drops the row of the message it names", func(t *testing.T) {
		env := newSelfDestructEnv(t, memberSelfDestructRuleDelay)
		setNow := selfDestructClock(t, env.now)
		member := env.member()
		pinned := env.sendMember(&member, func(m *gotgbot.Message) { m.Text = "pin me" })
		other := env.sendMember(&member, func(m *gotgbot.Message) { m.Text = "not pinned" })
		if got := memberDeleteTestFlush(t); got != 2 {
			t.Fatalf("memberDeleteFlush() = %d, want 2", got)
		}
		env.sendPinOf(env.chat, &env.admin, pinned)
		rows := env.selfDestructRows(env.chat.Id)
		if len(rows) != 1 || rows[0].MessageID != other {
			t.Fatalf("rows after the pin = %+v, want only message %d", rows, other)
		}
		setNow(env.now.Add(601 * time.Second))
		env.pass()
		calls := env.batchCalls(env.chat.Id)
		if len(calls) != 1 || len(calls[0]) != 1 || calls[0][0] != other {
			t.Fatalf("deleteMessages calls = %v, want one call with message %d and never the pinned one", calls, other)
		}
	})
}
