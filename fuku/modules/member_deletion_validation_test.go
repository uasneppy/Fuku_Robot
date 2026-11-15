//go:build testtools

package modules

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/deletion"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// Phase 17 validation audit tests: behaviours the phase's own tests leave to inference. Each drives
// the real dispatcher, batcher and worker with only Telegram and the clock faked.

// memberValidationGone is the answer Telegram gives for a message that no longer exists.
func memberValidationGone() error {
	return &gotgbot.TelegramError{Method: "deleteMessage", Code: 400, Description: "Bad Request: message to delete not found"}
}

// memberValidationAnswerGone makes every delete request answer "message to delete not found".
func memberValidationAnswerGone(env *commandDeletionEnv) {
	env.del.script.set(func(_ context.Context, method string, _ map[string]any) error {
		if method == "deleteMessage" || method == "deleteMessages" {
			return memberValidationGone()
		}
		return nil
	})
}

// TestValidationCommandAlsoSelfDestructQueued: a command of a member in a group with Commands and
// Self-destruct both on is answered and deleted at once, its self-destruct row is queued as well, and
// when that row falls due Telegram says the message is gone: the row completes quietly with no
// latch, no second deletion that counts and no log line.
func TestValidationCommandAlsoSelfDestructQueued(t *testing.T) {
	env := settingsDelDriftEnv(t)
	hook := captureLockdownLogs(t)
	member := env.mreqMember("Rae", "", "rae")
	env.setCommandsRule(true)
	env.del.setRule(deletion.KindMemberSelfDestruct, 600)
	start := env.now()

	id := env.command(&member, "/rules")
	if got := env.deleted(env.chat.Id, id); got != 1 {
		t.Fatalf("deletes of the command = %d, want exactly 1 at once", got)
	}
	reply, del := env.replyIndexTo(env.chat.Id, id), env.deleteIndexOf(env.chat.Id, id)
	if reply < 0 || del < 0 || reply >= del {
		t.Fatalf("reply at %d, delete at %d, want the answer first (methods %v)", reply, del, env.methods())
	}
	memberDeleteTestFlush(t)
	env.wantRow(id, deletion.KindMemberSelfDestruct, start, start.Add(600*time.Second))

	memberValidationAnswerGone(env)
	env.advance(601 * time.Second)
	env.pass()
	if rows := env.rows(); len(rows) != 0 {
		t.Fatalf("rows after the due cycle = %+v, want the queued row completed", rows)
	}
	if got := env.redisGet(deletionLatchKey(env.chat.Id)); got != "" {
		t.Fatalf("latch = %q, want none: a gone message says nothing about the bot's rights", got)
	}
	if lines := memberDeleteLogLines(hook, env.chat.Id); len(lines) != 0 {
		t.Fatalf("warn lines naming the chat = %q, want none", lines)
	}
}

// TestValidationEditOfSelfDestructQueuedMessage: a late edit of a message already queued for
// self-destruct is deleted at once by the edit rule, and its queued row later completes quietly.
func TestValidationEditOfSelfDestructQueuedMessage(t *testing.T) {
	env := settingsDelDriftEnv(t)
	hook := captureLockdownLogs(t)
	member := env.mreqMember("Rae", "", "rae")
	env.del.setRule(deletion.KindMemberEdited, 60)
	env.del.setRule(deletion.KindMemberSelfDestruct, 3600)
	start := env.now()

	id := memberE2EText(env, &member)
	memberDeleteTestFlush(t)
	env.wantRow(id, deletion.KindMemberSelfDestruct, start, start.Add(3600*time.Second))

	// The edit arrives 90 s after the message was sent, past the 60 s grace.
	edit := &gotgbot.Message{MessageId: id, Date: start.Unix(), EditDate: start.Unix() + 90, Chat: env.chat, From: &member, Text: "edited"}
	if err := env.dispatcher.ProcessUpdate(env.bot, &gotgbot.Update{UpdateId: env.updateID(), EditedMessage: edit}, nil); err != nil {
		t.Fatalf("ProcessUpdate(edit) error = %v", err)
	}
	if got := env.deleted(env.chat.Id, id); got != 1 {
		t.Fatalf("deletes after the late edit = %d, want exactly 1", got)
	}
	if _, ok := env.rows()[id]; !ok {
		t.Fatal("the self-destruct row vanished on an edit, want it left for the worker")
	}

	memberValidationAnswerGone(env)
	env.advance(3601 * time.Second)
	env.pass()
	if rows := env.rows(); len(rows) != 0 {
		t.Fatalf("rows after the due cycle = %+v, want completed", rows)
	}
	if got := env.redisGet(deletionLatchKey(env.chat.Id)); got != "" {
		t.Fatalf("latch = %q, want none", got)
	}
	if lines := memberDeleteLogLines(hook, env.chat.Id); len(lines) != 0 {
		t.Fatalf("warn lines naming the chat = %q, want none", lines)
	}
}

// TestValidationSelfDestructPresetEdges: the shortest and the longest preset are deleted exactly at
// date plus delay and not one second before.
func TestValidationSelfDestructPresetEdges(t *testing.T) {
	for _, delay := range []int{60, 86400} {
		t.Run(fmt.Sprintf("%d seconds", delay), func(t *testing.T) {
			env := settingsDelDriftEnv(t)
			member := env.mreqMember("Rae", "", "rae")
			env.del.setRule(deletion.KindMemberSelfDestruct, delay)
			start := env.now()
			id := memberE2EText(env, &member)
			memberDeleteTestFlush(t)
			env.wantRow(id, deletion.KindMemberSelfDestruct, start, start.Add(time.Duration(delay)*time.Second))

			env.advance(time.Duration(delay-1) * time.Second)
			env.pass()
			if got := env.deleted(env.chat.Id, id); got != 0 {
				t.Fatalf("deleted %d times one second before due, want 0", got)
			}
			env.advance(time.Second)
			env.pass()
			if got := env.deleted(env.chat.Id, id); got != 1 {
				t.Fatalf("deleted %d times at the due second, want 1", got)
			}
		})
	}
}

// TestValidationSelfDestructRealCeiling: with the real ceiling of 20,000 queued rows per chat, an
// offer when 19,998, 19,999, 20,000 and 20,001 rows are queued writes exactly the room left (2, 1,
// 0, 0) and sheds the rest, never going over and never panicking on a negative room.
func TestValidationSelfDestructRealCeiling(t *testing.T) {
	if memberSelfDestructCeiling != 20000 {
		t.Fatalf("memberSelfDestructCeiling = %d, the phase fixes it at 20000", memberSelfDestructCeiling)
	}
	cases := []struct {
		queued  int
		written int
	}{{19998, 2}, {19999, 1}, {20000, 0}, {20001, 0}}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("%d queued", tc.queued), func(t *testing.T) {
			env := newDeletionEnv(t)
			drainMemberDeleteQueue(t)
			restoreMemberDeleteBatcher(t)
			deletionSetRule(t, env.chat.Id, deletion.KindMemberSelfDestruct, 600)
			rows := make([]models.ScheduledDeletion, 0, tc.queued)
			at := env.now.UTC().Truncate(time.Second)
			for i := range tc.queued {
				rows = append(rows, models.ScheduledDeletion{
					ChatID: env.chat.Id, MessageID: 5_000_000 + int64(i), Kind: deletion.KindMemberSelfDestruct,
					SentAt: at, DeleteAt: at.Add(time.Hour), State: models.ScheduledDeletionPending, CreatedAt: at,
				})
			}
			if err := db.DB.CreateInBatches(rows, 500).Error; err != nil {
				t.Fatalf("seed %d rows: %v", tc.queued, err)
			}
			shedBefore := memberDeleteShedTotal(memberDeleteShedCeiling)

			memberDeleteOfferN(env.chat.Id, 9_000_000, 3, env.now)
			if got := memberDeleteTestFlush(t); got != tc.written {
				t.Fatalf("memberDeleteFlush() = %d, want %d", got, tc.written)
			}
			if got := memberDeleteShedTotal(memberDeleteShedCeiling) - shedBefore; got != int64(3-tc.written) {
				t.Fatalf("shed over the ceiling = %d, want %d", got, 3-tc.written)
			}
			var n int64
			if err := db.DB.Model(&models.ScheduledDeletion{}).Where("chat_id = ?", env.chat.Id).Count(&n).Error; err != nil {
				t.Fatalf("count: %v", err)
			}
			if want := int64(tc.queued + tc.written); n != want {
				t.Fatalf("queued rows = %d, want %d", n, want)
			}
		})
	}
}
