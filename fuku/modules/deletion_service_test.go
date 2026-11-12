//go:build testtools

package modules

import (
	"context"
	"errors"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

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
