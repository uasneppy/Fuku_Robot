//go:build testtools

package modules

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db/approvals"
	"github.com/uasneppy/fuku_robot/fuku/db/deletion"
	"github.com/uasneppy/fuku_robot/fuku/utils/cache"
)

// Member message deletion end to end (plan 17-11; DEL-03, DEL-04, DEL-05, DEL-07, DEL-08, PLAT-06).
// Everything is real but Telegram and the clock: the Deletion page's presses, the Processor wrapper
// and the real dispatcher (Bans, Greetings, MemberDeletion), the edit handler, the self-destruct
// watcher, the batcher's flush and the worker's cycle run through the registered code, and time moves
// only through the env's clock, never by sleeping. The environment is settingsDelDriftEnv: the
// Phase 16 end-to-end environment with the wrapper and the MemberDeletion module loaded, the admin
// cache primed (the env's admin is listed, a member is a known non-administrator) and the batcher's
// buffer drained when the test ends.

// memberE2ESet sets one Member messages row through the page, the way the owner does: open the row,
// press the choice, and confirm the Self-destruct card with Yes when the choice drew it.
func memberE2ESet(t *testing.T, m *settingsDelMenu, code, value string) {
	t.Helper()
	m.open("dr", code)
	m.choose(code, value)
	if code == "ms" && value != "n" {
		yes, _ := settingsDelCardOf(t, m)
		m.pressButton(yes)
	}
}

// memberE2EWantStored fails unless the stored delay of kind is want.
func memberE2EWantStored(t *testing.T, env *commandDeletionEnv, kind string, want int) {
	t.Helper()
	if delay, ok := env.del.stored(kind); !ok || delay != want {
		t.Fatalf("stored %s = %d (%v), want %d", kind, delay, ok, want)
	}
}

// memberE2EText delivers a plain text of from to the env chat and returns its message ID.
func memberE2EText(env *commandDeletionEnv, from *gotgbot.User) int64 {
	env.t.Helper()
	return env.deliver(from, func(m *gotgbot.Message) { m.Text = "hello" })
}

// memberE2EEdit delivers an edited_message of from whose original was sent age before the env's
// clock and edited gap seconds after it was sent, and returns its ID.
func memberE2EEdit(t *testing.T, env *commandDeletionEnv, from gotgbot.User, age time.Duration, gap int64) int64 {
	t.Helper()
	id := env.updateID()
	date := env.now().Add(-age).Unix()
	msg := &gotgbot.Message{MessageId: id, Date: date, EditDate: date + gap, Chat: env.chat, From: &from, Text: "edited"}
	if err := env.dispatcher.ProcessUpdate(env.bot, &gotgbot.Update{UpdateId: id, EditedMessage: msg}, nil); err != nil {
		t.Fatalf("ProcessUpdate(%d) error = %v", id, err)
	}
	return id
}

// memberE2EFailLookup makes the live getChatMember of one member answer err and nothing else fail.
func memberE2EFailLookup(env *commandDeletionEnv, userID int64, err error) {
	env.del.script.set(func(_ context.Context, method string, params map[string]any) error {
		if method == "getChatMember" && staffParamInt(params, "user_id") == userID {
			return err
		}
		return nil
	})
}

// memberE2EWantNothing fails when the env chat has a queue row after a flush or a delete request.
func memberE2EWantNothing(t *testing.T, env *commandDeletionEnv, what string) {
	t.Helper()
	memberDeleteTestFlush(t)
	if rows := env.rows(); len(rows) != 0 {
		t.Fatalf("%s: queue rows = %+v, want none", what, rows)
	}
	if got := env.deleteCalls(env.chat.Id); got != 0 {
		t.Fatalf("%s: delete requests = %d, want none", what, got)
	}
}

func TestMemberDeletionEndToEnd(t *testing.T) {
	t.Run("the owner sets the three rules and each one acts", func(t *testing.T) {
		env := settingsDelDriftEnv(t)
		member := env.mreqMember("Rae", "", "rae")
		m := env.del.groupMenu()

		// The owner presses Main, Deletion, Member messages, Commands On, Edited messages 1 min and
		// Self-destruct 10 min with the card's Yes.
		m.openHub()
		m.open("dl", "m")
		memberE2ESet(t, m, "mc", "0")
		memberE2ESet(t, m, "me", "60")
		memberE2ESet(t, m, "ms", "600")
		memberE2EWantStored(t, env, deletion.KindMemberCommands, 0)
		memberE2EWantStored(t, env, deletion.KindMemberEdited, 60)
		memberE2EWantStored(t, env, deletion.KindMemberSelfDestruct, 600)
		hub, _ := m.openHub()
		settingsDelWantLine(t, hub, "settings_del_sec_member", "💬", staffMarker("settings_state_del_on")+" 3")

		// A command is answered, then deleted.
		start := env.now()
		cmdID := env.deliver(&member, commandMessage("/rules"))
		if got := env.deleted(env.chat.Id, cmdID); got < 1 {
			t.Fatalf("deletes of the command = %d, want at least the inline one", got)
		}
		reply, del := env.replyIndexTo(env.chat.Id, cmdID), env.deleteIndexOf(env.chat.Id, cmdID)
		if reply < 0 || del < 0 || reply >= del {
			t.Fatalf("reply at %d, delete at %d, want the answer first (methods %v)", reply, del, env.methods())
		}

		// A late edit is deleted at once and an early one kept.
		kept := memberE2EEdit(t, env, member, 5*time.Minute, 20)
		late := memberE2EEdit(t, env, member, 5*time.Minute, 90)
		if env.deleted(env.chat.Id, kept) != 0 || env.deleted(env.chat.Id, late) != 1 {
			t.Fatalf("edits: the 20 s one deleted %d times, the 90 s one %d, want 0 and 1",
				env.deleted(env.chat.Id, kept), env.deleted(env.chat.Id, late))
		}

		// A message is queued at its date plus 600 s and the worker deletes it then, not before.
		textID := memberE2EText(env, &member)
		memberDeleteTestFlush(t)
		env.wantRow(textID, deletion.KindMemberSelfDestruct, start, start.Add(600*time.Second))
		env.advance(599 * time.Second)
		env.pass()
		if got := env.deleted(env.chat.Id, textID); got != 0 {
			t.Fatalf("the worker at +599 s deleted the message %d times, want nothing yet", got)
		}
		env.advance(2 * time.Second)
		env.pass()
		if got := env.deleted(env.chat.Id, textID); got != 1 {
			t.Fatalf("deletes of the message at +601 s = %d, want exactly 1", got)
		}
		if rows := env.rows(); len(rows) != 0 {
			t.Fatalf("queue rows after the worker = %+v, want none", rows)
		}
	})

	t.Run("commands", func(t *testing.T) {
		env := settingsDelDriftEnv(t)
		member := env.mreqMember("Rae", "", "rae")
		target := env.mreqMember("Bad", "Actor", "bad")
		m := env.del.groupMenu()
		memberE2ESet(t, m, "mc", "0")

		// An admin's /ban and a member's /rules are answered and then deleted.
		banID := env.deliver(&env.del.owner, commandMessage(fmt.Sprintf("/ban %d", target.Id)))
		rulesID := env.deliver(&member, commandMessage("/rules"))
		for _, id := range []int64{banID, rulesID} {
			if got := env.deleted(env.chat.Id, id); got != 1 {
				t.Fatalf("deletes of command %d = %d, want exactly 1", id, got)
			}
			reply, del := env.replyIndexTo(env.chat.Id, id), env.deleteIndexOf(env.chat.Id, id)
			if reply < 0 || del < 0 || reply >= del {
				t.Fatalf("command %d: reply at %d, delete at %d, want the answer first (methods %v)", id, reply, del, env.methods())
			}
		}
		// Another bot's command and an unknown word stay, and so do the bot's own replies.
		otherID := env.deliver(&member, commandMessage("/rules@otherbot"))
		unknownID := env.deliver(&member, commandMessage("/nosuchcommand17"))
		for _, id := range []int64{otherID, unknownID} {
			if got := env.deleted(env.chat.Id, id); got != 0 {
				t.Errorf("message %d was deleted %d times, want it kept", id, got)
			}
		}
		for _, sent := range env.sentIDs() {
			if got := env.deleted(env.chat.Id, sent); got != 0 {
				t.Errorf("the bot's own message %d was deleted %d times, want it kept", sent, got)
			}
		}
		if rows := env.rows(); len(rows) != 0 {
			t.Fatalf("queue rows = %+v, want none", rows)
		}
	})

	t.Run("an anonymous admin's command", func(t *testing.T) {
		t.Run("is deleted after the proof tap", func(t *testing.T) {
			env := newAnonDeletionEnv(t, false)
			env.sendAnonymous(fmt.Sprintf("/ban %d spam", env.target.Id))
			cmdID := env.commandID()
			env.tapProof(env.creator)

			log := env.callLog()
			ban := indexOfEntry(log, "banChatMember")
			del := indexOfEntry(log, fmt.Sprintf("deleteMessage:%d", cmdID))
			if ban < 0 || del < 0 || ban >= del {
				t.Fatalf("call log %v: the ban is at %d and the command delete at %d, want the delete after the ban", log, ban, del)
			}
		})
		t.Run("stays with AnonAdmin on (D-34)", func(t *testing.T) {
			env := newAnonDeletionEnv(t, true)
			env.sendAnonymous(fmt.Sprintf("/ban %d spam", env.target.Id))
			cmdID := env.commandID()

			log := env.callLog()
			if indexOfEntry(log, "banChatMember") < 0 {
				t.Fatalf("the ban did not run at the first update: %v", log)
			}
			if got := indexOfEntry(log, fmt.Sprintf("deleteMessage:%d", cmdID)); got != -1 {
				t.Fatalf("the command was deleted (log %v), the stated limit says it stays", log)
			}
			if rows := env.queueRowsOfChat(); len(rows) != 0 {
				t.Fatalf("queue rows = %+v, want none", rows)
			}
		})
	})

	t.Run("edits", func(t *testing.T) {
		env := settingsDelDriftEnv(t)
		member := env.mreqMember("Rae", "", "rae")
		approved := env.mreqMember("Ada", "", "ada")
		if err := approvals.AddApprovedUser(env.chat.Id, approved.Id, env.admin.Id, "trusted"); err != nil {
			t.Fatalf("AddApprovedUser error = %v", err)
		}
		m := env.del.groupMenu()
		memberE2ESet(t, m, "me", "60")

		kept := map[string]int64{
			"an edit at 20 s":                  memberE2EEdit(t, env, member, 5*time.Minute, 20),
			"an edit at exactly the grace":     memberE2EEdit(t, env, member, 5*time.Minute, 60),
			"an admin's late edit":             memberE2EEdit(t, env, env.admin, 5*time.Minute, 90),
			"an approved member's late edit":   memberE2EEdit(t, env, approved, 5*time.Minute, 90),
			"a late edit of a 48 hour old one": memberE2EEdit(t, env, member, 48*time.Hour, 90),
		}
		for name, id := range kept {
			if got := env.deleted(env.chat.Id, id); got != 0 {
				t.Errorf("%s was deleted %d times, want it kept", name, got)
			}
		}
		late := memberE2EEdit(t, env, member, 5*time.Minute, 90)
		if got := env.deleted(env.chat.Id, late); got != 1 {
			t.Fatalf("deletes of the 90 s edit = %d, want exactly 1", got)
		}
		if rows := env.rows(); len(rows) != 0 {
			t.Fatalf("queue rows = %+v, want none: an edit is deleted at once", rows)
		}
	})

	t.Run("self-destruct", func(t *testing.T) {
		env := settingsDelDriftEnv(t)
		member := env.mreqMember("Rae", "", "rae")
		approved := env.mreqMember("Ada", "", "ada")
		if err := approvals.AddApprovedUser(env.chat.Id, approved.Id, env.admin.Id, "trusted"); err != nil {
			t.Fatalf("AddApprovedUser error = %v", err)
		}
		m := env.del.groupMenu()
		memberE2ESet(t, m, "ms", "600")
		start := env.now()

		first := memberE2EText(env, &member)
		pinned := memberE2EText(env, &member)
		memberE2EText(env, &env.admin)
		memberE2EText(env, &approved)
		if got := memberDeleteTestFlush(t); got != 2 {
			t.Fatalf("memberDeleteFlush wrote %d rows, want the 2 of the member: an admin's and an approved member's are never offered", got)
		}
		env.wantRow(first, deletion.KindMemberSelfDestruct, start, start.Add(600*time.Second))

		// A message a pinned_message service message names stays: its row is dropped.
		env.deliver(&env.del.owner, func(msg *gotgbot.Message) {
			msg.PinnedMessage = gotgbot.Message{MessageId: pinned, Chat: env.chat}
		})
		if _, ok := env.rows()[pinned]; ok {
			t.Fatal("the pinned message's row is still queued")
		}
		env.advance(601 * time.Second)
		env.pass()
		if env.deleted(env.chat.Id, first) != 1 || env.deleted(env.chat.Id, pinned) != 0 {
			t.Fatalf("at +601 s the first message was deleted %d times and the pinned one %d, want 1 and 0",
				env.deleted(env.chat.Id, first), env.deleted(env.chat.Id, pinned))
		}

		// Off cancels the rows still waiting, and later messages are not queued.
		waiting := memberE2EText(env, &member)
		memberDeleteTestFlush(t)
		if _, ok := env.rows()[waiting]; !ok {
			t.Fatal("the message was not queued while the rule was on")
		}
		m.choose("ms", "n")
		if rows := env.rows(); len(rows) != 0 {
			t.Fatalf("Off left waiting rows: %+v", rows)
		}
		memberE2EText(env, &member)
		memberE2EWantNothingNew(t, env)
	})

	t.Run("unknown never deletes (D-33)", func(t *testing.T) {
		t.Run("a failed approvals read", func(t *testing.T) {
			env := settingsDelDriftEnv(t)
			member := env.mreqMember("Rae", "", "rae")
			env.del.setRule(deletion.KindMemberEdited, 60)
			env.del.setRule(deletion.KindMemberSelfDestruct, 600)
			mreqSwap(t, &memberDeletionApprovedLookup,
				func(context.Context, int64, int64) (bool, error) { return false, errors.New("approvals unreadable") })

			id := memberE2EEdit(t, env, member, 5*time.Minute, 90)
			if got := env.deleted(env.chat.Id, id); got != 0 {
				t.Fatalf("a late edit was deleted %d times with the approvals unreadable, want it kept", got)
			}
			memberE2EText(env, &member)
			memberE2EWantNothing(t, env, "a message with the approvals unreadable")
		})
		t.Run("a cold admin cache whose live read answers 429", func(t *testing.T) {
			env := settingsDelDriftEnv(t)
			member := env.mreqMember("Rae", "", "rae")
			env.del.setRule(deletion.KindMemberEdited, 60)
			env.del.setRule(deletion.KindMemberSelfDestruct, 600)
			memberE2EFailLookup(env, member.Id, memberDeletionTelegramError(429))
			cache.InvalidateAdminCache(env.chat.Id)

			id := memberE2EEdit(t, env, member, 5*time.Minute, 90)
			if got := env.deleted(env.chat.Id, id); got != 0 {
				t.Fatalf("a late edit was deleted %d times behind a cold cache and a 429, want it kept", got)
			}
			memberE2EText(env, &member)
			memberE2EWantNothing(t, env, "a message behind a cold cache and a 429")
		})
	})

	t.Run("the bot cannot delete", func(t *testing.T) {
		noRight := staffMarker("settings_del_no_right")
		env := settingsDelDriftEnv(t)
		member := env.mreqMember("Rae", "", "rae")
		m := env.del.groupMenu()
		env.del.setBot(settingsDelBotJSON("member", false))

		// Every member screen shows the warning once, and the rules can still be set.
		for _, screen := range settingsDelScreens {
			text := screen.draw(m)
			if n := strings.Count(text, noRight); n != 1 {
				t.Errorf("%s: the warning line appears %d times, want once: %q", screen.name, n, text)
			}
		}
		yes, _ := settingsDelCardOf(t, m)
		m.pressButton(yes)
		memberE2ESet(t, m, "mc", "0")
		memberE2ESet(t, m, "me", "60")
		memberE2EWantStored(t, env, deletion.KindMemberCommands, 0)
		memberE2EWantStored(t, env, deletion.KindMemberEdited, 60)
		memberE2EWantStored(t, env, deletion.KindMemberSelfDestruct, 600)

		// Telegram refuses the first delete: one refused call and one live check of the bot, then
		// the shared latch keeps every rule from calling again.
		env.del.script.set(func(_ context.Context, method string, _ map[string]any) error {
			if method == "deleteMessage" || method == "deleteMessages" {
				return deletionRefusal()
			}
			return nil
		})
		reads := env.del.botReads(env.chat.Id)
		env.deliver(&member, commandMessage("/rules"))
		if got := env.deleteCalls(env.chat.Id); got != 1 {
			t.Fatalf("delete requests after the first command = %d, want the one refused call", got)
		}
		if got := env.del.botReads(env.chat.Id) - reads; got != 1 {
			t.Fatalf("live checks of the bot = %d, want exactly 1", got)
		}
		if got := env.redisGet(deletionLatchKey(env.chat.Id)); got != deletionScopeService {
			t.Fatalf("latch = %q, want %q", got, deletionScopeService)
		}
		env.deliver(&member, commandMessage("/rules"))
		memberE2EEdit(t, env, member, 5*time.Minute, 90)
		memberE2EText(env, &member)
		memberDeleteTestFlush(t)
		env.advance(601 * time.Second)
		env.pass()
		if got := env.deleteCalls(env.chat.Id); got != 1 {
			t.Fatalf("delete requests after every rule ran = %d, want still the one: the latch stops them", got)
		}
		if got := env.del.botReads(env.chat.Id) - reads; got != 1 {
			t.Fatalf("live checks of the bot after every rule ran = %d, want still 1", got)
		}
		if rows := env.rows(); len(rows) != 0 {
			t.Fatalf("queue rows = %+v, want none: a latched chat's member rows are dropped", rows)
		}
	})

	t.Run("Staff Group", func(t *testing.T) {
		env := settingsDelDriftEnv(t)
		member := env.mreqMember("Rae", "", "rae")
		m := env.del.groupMenu()
		// Every rule is stored first, so a refusal is the Staff Group's and not a missing rule.
		env.del.setRule(deletion.KindMemberCommands, 0)
		env.del.setRule(deletion.KindMemberEdited, 60)
		env.del.setRule(deletion.KindMemberSelfDestruct, 600)
		env.del.makeStaff()

		cmdID := env.deliver(&member, commandMessage("/rules"))
		editID := memberE2EEdit(t, env, member, 5*time.Minute, 90)
		memberE2EText(env, &member)
		memberDeleteTestFlush(t)
		env.advance(601 * time.Second)
		env.pass()
		for _, id := range []int64{cmdID, editID} {
			if got := env.deleted(env.chat.Id, id); got != 0 {
				t.Errorf("message %d of a Staff Group was deleted %d times", id, got)
			}
		}
		if rows := env.rows(); len(rows) != 0 {
			t.Fatalf("queue rows of a Staff Group = %+v, want none", rows)
		}
		if got := env.deleteCalls(env.chat.Id); got != 0 {
			t.Fatalf("delete requests in a Staff Group = %d, want 0", got)
		}

		// No page: Main has no Deletion cell, and every forged press is invalid.
		reopened := env.del.groupMenu()
		for _, button := range settingsAllButtons(reopened.mainKeyboard) {
			if decoded, ok := decodeCallbackData(button.CallbackData, settingsCallbackNamespace); ok && decoded.Fields["p"] == "d" {
				t.Errorf("the Main page of a Staff Group has the Deletion cell %q", button.Text)
			}
		}
		for _, kv := range settingsDelForged {
			answers := env.del.answerCount()
			m.press(kv...)
			if text, alert := env.del.lastAnswer(); text != staffMarker("common_callback_invalid_request") || !alert {
				t.Errorf("%v answered %q alert=%v, want the invalid-request alert", kv, text, alert)
			}
			if got := env.del.answerCount() - answers; got != 1 {
				t.Errorf("%v was answered %d times, want once", kv, got)
			}
		}
	})

	t.Run("restart and replicas", func(t *testing.T) {
		env := settingsDelDriftEnv(t)
		member := env.mreqMember("Rae", "", "rae")
		env.del.setRule(deletion.KindMemberSelfDestruct, 600)
		deletionWorkerStopped(t)
		deletionDrainWake()
		mreqSwap(t, &deletionWorkerTick, 10*time.Millisecond)
		mreqSwap(t, &memberDeleteFlushEvery, time.Hour)

		// The batcher runs but its period is an hour, so the messages are still in its buffer when the
		// stop runs, and the final flush of StopDeletionWorker writes them.
		StartDeletionWorker(env.bot)
		start := env.now()
		var ids []int64
		for range 3 {
			ids = append(ids, memberE2EText(env, &member))
		}
		if rows := env.rows(); len(rows) != 0 {
			t.Fatalf("queue rows before the final flush = %+v, want none: the batcher's period had not come", rows)
		}
		StopDeletionWorker()
		for _, id := range ids {
			env.wantRow(id, deletion.KindMemberSelfDestruct, start, start.Add(600*time.Second))
		}

		// After a restart two workers over one database delete each message once.
		freshDeletionRoot()
		env.advance(601 * time.Second)
		second := env.replicaBot()
		done := make(chan struct{}, 2)
		run := func(bot *gotgbot.Bot) {
			defer func() { done <- struct{}{} }()
			deadline := time.Now().Add(30 * time.Second)
			for time.Now().Before(deadline) && len(env.rows()) > 0 {
				runDeletionCycle(context.Background(), bot)
			}
		}
		go run(env.bot)
		go run(second.bot)
		<-done
		<-done
		if rows := env.rows(); len(rows) != 0 {
			t.Fatalf("queue rows after the two workers = %+v, want none", rows)
		}
		for _, id := range ids {
			if got := env.deleted(env.chat.Id, id) + second.deleted(env.chat.Id, id); got != 1 {
				t.Errorf("message %d was deleted %d times across the two workers, want exactly 1", id, got)
			}
		}
	})
}

// memberE2EWantNothingNew flushes the batcher and fails when it wrote a row.
func memberE2EWantNothingNew(t *testing.T, env *commandDeletionEnv) {
	t.Helper()
	if got := memberDeleteTestFlush(t); got != 0 {
		t.Fatalf("memberDeleteFlush wrote %d rows after Off, want none", got)
	}
	if rows := env.rows(); len(rows) != 0 {
		t.Fatalf("queue rows after Off = %+v, want none", rows)
	}
}
