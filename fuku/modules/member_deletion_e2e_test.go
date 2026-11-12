//go:build testtools

package modules

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/approvals"
	"github.com/uasneppy/fuku_robot/fuku/db/captcha"
	"github.com/uasneppy/fuku_robot/fuku/db/deletion"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/utils/actionlog"
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
	return memberE2EEditText(t, env, from, age, gap, "edited")
}

// memberE2EEditText is memberE2EEdit with the text the edited message carries.
func memberE2EEditText(t *testing.T, env *commandDeletionEnv, from gotgbot.User, age time.Duration, gap int64, text string) int64 {
	t.Helper()
	id := env.updateID()
	date := env.now().Add(-age).Unix()
	msg := &gotgbot.Message{MessageId: id, Date: date, EditDate: date + gap, Chat: env.chat, From: &from, Text: text}
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
			// The list cannot be loaded either (the warm is a no-op), so only the live read could say.
			mreqSwap(t, &memberDeletionAdminWarm, func(*gotgbot.Bot, int64) {})
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

// memberShortestRules stores the shortest valid preset of the three member rules for each chat
// (commands 0, edited 30, self-destruct 60), removed when the test ends.
func memberShortestRules(t *testing.T, chatIDs ...int64) {
	t.Helper()
	deletionCleanup(t, chatIDs...)
	for _, chatID := range chatIDs {
		for kind, delay := range map[string]int{
			deletion.KindMemberCommands:     0,
			deletion.KindMemberEdited:       30,
			deletion.KindMemberSelfDestruct: 60,
		} {
			if err := deletion.SetRule(t.Context(), chatID, kind, delay); err != nil {
				t.Fatalf("SetRule(%d, %s, %d) error = %v", chatID, kind, delay, err)
			}
		}
	}
}

// memberExcludedProducers are the identifiers a message producer of the never-delete families would
// use to reach a deletion: the self-destruct batcher's offer, the queue writers, the bot-message
// helper and the core's act.
var memberExcludedProducers = []string{
	"memberDeleteOffer", "ScheduleBatch", "scheduleBotMessage", "deletionSchedule", "deletionAct", "deletion.Schedule",
}

// TestMemberDeletionExcludedMessages proves DEL-07 for the three member rules: with each at its
// shortest valid preset, a Staff Group card or summary, a lockdown alert copy, a captcha challenge
// and a log-channel post, each sent through its real sender, leave no row of any kind and get no
// delete request, also after a worker cycle a day and a half later; no file of those families calls
// a deletion producer; and the bot's own messages, delivered as updates, are never touched.
func TestMemberDeletionExcludedMessages(t *testing.T) {
	t.Run("a Staff Group action card and summary", func(t *testing.T) {
		advance := deletionClockFixed(t)
		withFastDeletionPacer(t)
		env := newStaffActionEnv(t, 2)
		LoadServiceDeletion(env.dispatcher)
		all := append([]int64{env.staffChat}, env.groups...)
		memberShortestRules(t, all...)
		for _, group := range env.groups {
			env.fake.setMember(group, staffTestTarget, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})
		}

		summary, msgID := env.runStaffCommand("/mute 4242 flooding")
		if summary == "" || msgID == 0 {
			t.Fatalf("the staff action produced no summary (%q, %d), so the case proves nothing", summary, msgID)
		}
		if n := len(env.fake.sentTo(env.staffChat)); n == 0 {
			t.Fatal("nothing was sent to the Staff Group, so the case proves nothing")
		}
		advance()
		deletionWantUntouched(t, env.bot, "staff card and summary", func(chatID int64) int {
			return len(env.callsTo("deleteMessage", chatID)) + len(env.callsTo("deleteMessages", chatID))
		}, all...)
	})

	t.Run("a lockdown alert copy", func(t *testing.T) {
		advance := deletionClockFixed(t)
		withFastDeletionPacer(t)
		env := newLockdownEnv(t)
		LoadServiceDeletion(env.dispatcher)
		staffChat := linkLockdownStaffGroup(env)
		memberShortestRules(t, env.chat.Id, staffChat.Id)

		env.send(env.admin, "/lockdown raid wave")
		env.cycle()
		env.cycle()
		if n := len(env.fake.sentTo(staffChat.Id)); n != 1 {
			t.Fatalf("alert copies in the Staff Group = %d, want the one, so the case proves something", n)
		}
		advance()
		deletionWantUntouched(t, env.bot, "lockdown alert", func(chatID int64) int {
			return len(callsToChat(env.fake.staffBotClient, "deleteMessage", chatID)) +
				len(callsToChat(env.fake.staffBotClient, "deleteMessages", chatID))
		}, env.chat.Id, staffChat.Id)
	})

	t.Run("a captcha challenge", func(t *testing.T) {
		advance := deletionClockFixed(t)
		withMiniredis(t)
		withFastDeletionPacer(t)
		s := newBotSite(t, "supergroup")
		memberShortestRules(t, s.chat.Id)
		member := uniqueLinkOwnerID()
		t.Cleanup(func() {
			db.DB.Where("chat_id = ?", s.chat.Id).Delete(&models.CaptchaAttempts{})
			db.DB.Where("chat_id = ?", s.chat.Id).Delete(&models.CaptchaSettings{})
		})
		if err := captcha.SetCaptchaEnabled(s.chat.Id, true); err != nil {
			t.Fatalf("SetCaptchaEnabled() error = %v", err)
		}
		ctx := newModuleMessageContext(s.bot, s.chat, gotgbot.User{Id: 777000, FirstName: "Telegram"}, "join")
		ctx.EffectiveMessage.Date = s.now.Unix()

		if err := SendCaptcha(s.bot, ctx, member, "Member"); err != nil {
			t.Fatalf("SendCaptcha() error = %v", err)
		}
		if len(s.client.callsFor("sendPhoto"))+len(s.client.callsFor("sendMessage")) == 0 {
			t.Fatal("no challenge was sent, so the case proves nothing")
		}
		advance()
		deletionWantUntouched(t, s.bot, "captcha challenge", func(chatID int64) int {
			return len(chatCalls(s.client, "deleteMessage", chatID)) + len(chatCalls(s.client, "deleteMessages", chatID))
		}, s.chat.Id)
	})

	t.Run("a log-channel post", func(t *testing.T) {
		advance := deletionClockFixed(t)
		withMiniredis(t)
		withFastDeletionPacer(t)
		s := newBotSite(t, "supergroup")
		channelID := uniqueModuleChatID()
		memberShortestRules(t, s.chat.Id, channelID)
		setStaffLogChannel(t, s.chat.Id, channelID)

		actionlog.Admin(s.bot, &s.chat, &gotgbot.User{Id: 55, FirstName: "Actor"}, "BAN", 42, "why")
		posted := 0
		for _, call := range s.client.callsFor("sendMessage") {
			if staffParamInt(call.Params, "chat_id") == channelID {
				posted++
			}
		}
		if posted != 1 {
			t.Fatalf("posts to the log channel = %d, want the one, so the case proves something", posted)
		}
		advance()
		deletionWantUntouched(t, s.bot, "log post", func(chatID int64) int {
			return len(chatCalls(s.client, "deleteMessage", chatID)) + len(chatCalls(s.client, "deleteMessages", chatID))
		}, s.chat.Id, channelID)
	})

	t.Run("the bot's own messages delivered as updates", func(t *testing.T) {
		env := settingsDelDriftEnv(t)
		env.del.setRule(deletion.KindMemberCommands, 0)
		env.del.setRule(deletion.KindMemberEdited, 30)
		env.del.setRule(deletion.KindMemberSelfDestruct, 60)
		own := env.bot.User

		// The bot's reply, a message that looks like a command and a late edit of one, all from the bot.
		env.deliver(&own, func(m *gotgbot.Message) { m.Text = "a reply of the bot" })
		cmd := env.deliver(&own, commandMessage("/rules"))
		edited := memberE2EEdit(t, env, own, 5*time.Minute, 90)
		memberE2EWantNothing(t, env, "the bot's own messages")
		for _, id := range []int64{cmd, edited} {
			if got := env.deleted(env.chat.Id, id); got != 0 {
				t.Errorf("the bot's own message %d was deleted %d times", id, got)
			}
		}
	})

	t.Run("no file of the never-delete families calls a deletion producer", func(t *testing.T) {
		var paths []string
		for _, pattern := range []string{
			"staff*.go", "lockdown*.go", "captcha*.go", "raid*.go", "logchannels.go", "../utils/actionlog/*.go",
		} {
			matches, err := filepath.Glob(pattern)
			if err != nil {
				t.Fatalf("glob %s: %v", pattern, err)
			}
			paths = append(paths, matches...)
		}
		checked := 0
		for _, path := range paths {
			if strings.HasSuffix(path, "_test.go") {
				continue
			}
			src, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}
			checked++
			for _, name := range memberExcludedProducers {
				if strings.Contains(string(src), name) {
					t.Errorf("%s calls or names %s: a message of a never-delete family must not reach a deletion producer", path, name)
				}
			}
		}
		if checked < 10 {
			t.Fatalf("only %d files were checked, so the glob no longer finds the families", checked)
		}
	})
}

// memberLogLine is the category prefix of the member deletion code's log lines.
var memberLogLine = regexp.MustCompile(`^\[(Deletion|MemberDeletion)\]`)

// TestMemberDeletionLogsCarryIDsOnly drives every failing seam of the member rules (the command
// wrapper's core, the edit handler, the watcher, the exemption reads, the batcher's sheds and failed
// write, the worker's rule screen and the pin drop) with message texts, names and a username that
// carry sentinels, and asserts no log line holds a sentinel, a message ID or a claim token, and that
// each shed is one line for its chat and reason with the chat ID only (D-23, D-31).
func TestMemberDeletionLogsCarryIDsOnly(t *testing.T) {
	const textSentinel, nameSentinel, userSentinel = "ZZLEAKTEXT", "ZZLEAKNAME", "zzleakuser"
	env := settingsDelDriftEnv(t)
	hook := mreqShedHook(t)
	env.del.setRule(deletion.KindMemberCommands, 0)
	env.del.setRule(deletion.KindMemberEdited, 60)
	env.del.setRule(deletion.KindMemberSelfDestruct, 600)
	member := env.mreqMember(nameSentinel, "", userSentinel)
	failing := errors.New("lookup failed")
	var ids []int64
	sendText := func() {
		ids = append(ids, env.deliver(&member, func(m *gotgbot.Message) { m.Text = textSentinel }))
	}
	sendEdit := func() {
		ids = append(ids, memberE2EEditText(t, env, member, 5*time.Minute, 90, textSentinel))
	}
	warm := func() {
		env.del.script.set(nil)
		cache.InvalidateAdminCache(env.chat.Id)
		if got := cache.LoadAdminCache(env.bot, env.chat.Id); !got.Cached {
			t.Fatalf("LoadAdminCache() = %+v, want a cached list", got)
		}
	}
	// cold empties the admin cache and keeps it empty: the shared loader's warm is a no-op, so the
	// handler has to ask Telegram about the member (a cache that can be warmed is
	// TestMemberDeletionAdminWarm's case).
	cold := func() {
		cache.InvalidateAdminCache(env.chat.Id)
		mreqSwap(t, &memberDeletionAdminWarm, func(*gotgbot.Bot, int64) {})
	}

	t.Run("the approvals read fails", func(t *testing.T) {
		mreqSwap(t, &memberDeletionApprovedLookup,
			func(context.Context, int64, int64) (bool, error) { return false, failing })
		sendText()
		sendEdit()
	})
	t.Run("the live administrator read fails behind a cold cache", func(t *testing.T) {
		memberE2EFailLookup(env, member.Id, memberDeletionTelegramError(429))
		cold()
		sendText()
		sendEdit()
		warm()
	})
	t.Run("the administrator check is saturated", func(t *testing.T) {
		for range cap(memberDeletionAdminSlots) {
			memberDeletionAdminSlots <- struct{}{}
		}
		t.Cleanup(func() {
			for len(memberDeletionAdminSlots) > 0 {
				<-memberDeletionAdminSlots
			}
		})
		cold()
		sendText()
		sendEdit()
		for len(memberDeletionAdminSlots) > 0 {
			<-memberDeletionAdminSlots
		}
		warm()
	})
	t.Run("the captcha reads fail", func(t *testing.T) {
		mreqSwap(t, &memberReqCaptchaPendingLookup, func(context.Context, int64) (bool, error) { return false, failing })
		sendEdit()
		mreqSwap(t, &memberReqCaptchaPendingLookup, func(context.Context, int64) (bool, error) { return true, nil })
		mreqSwap(t, &memberReqCaptchaAttemptLookup, func(int64, int64) (*models.CaptchaAttempts, error) { return nil, failing })
		sendEdit()
	})
	t.Run("the Staff Group read fails", func(t *testing.T) {
		mreqSwap(t, &deletionStaffLookup, func(int64) (*models.StaffGroup, error) { return nil, failing })
		ids = append(ids, env.deliver(&member, commandMessage("/rules "+textSentinel)))
		sendEdit()
	})
	t.Run("the batcher sheds a full buffer", func(t *testing.T) {
		resetMemberDeleteState()
		mreqSwap(t, &memberDeleteQueue, make(chan deletion.Entry, 1))
		sendText()
		sendText()
		sendText()
		memberDeleteTestFlush(t)
	})
	t.Run("the batcher cannot read the rules", func(t *testing.T) {
		resetMemberDeleteState()
		sendText()
		mreqSwap(t, &deletionRulesLookup, func(context.Context, int64) (*deletion.Rules, error) { return nil, failing })
		memberDeleteTestFlush(t)
	})
	t.Run("the batcher cannot count the queued rows", func(t *testing.T) {
		resetMemberDeleteState()
		sendText()
		mreqSwap(t, &memberDeleteCountPending, func(context.Context, int64, string, int) (int64, error) { return 0, failing })
		memberDeleteTestFlush(t)
	})
	t.Run("the batch write fails", func(t *testing.T) {
		resetMemberDeleteState()
		sendText()
		mreqSwap(t, &memberDeleteBatchWrite, func(context.Context, []deletion.Entry) (int, error) { return 0, failing })
		memberDeleteTestFlush(t)
	})
	t.Run("the worker cannot read the rules of a chat with due rows", func(t *testing.T) {
		const first = 880001000
		for i := int64(0); i < 3; i++ {
			seedDeletionRow(t, env.chat.Id, first+i, env.now().Add(-time.Hour), env.now().Add(-2*time.Hour), deletion.KindMemberSelfDestruct)
		}
		ids = append(ids, first, first+1, first+2)
		mreqSwap(t, &deletionRulesLookup, func(context.Context, int64) (*deletion.Rules, error) { return nil, failing })
		env.cycle()
		db.DB.Where("chat_id = ?", env.chat.Id).Delete(&models.ScheduledDeletion{})
	})
	t.Run("the pin drop fails", func(t *testing.T) {
		const name = "member_e2e_fail_pin_drop"
		if err := db.DB.Callback().Delete().Before("gorm:delete").Register(name, func(tx *gorm.DB) {
			if tx.Statement.Table == "scheduled_deletions" {
				_ = tx.AddError(failing)
			}
		}); err != nil {
			t.Fatalf("register the failing callback: %v", err)
		}
		t.Cleanup(func() {
			if err := db.DB.Callback().Delete().Remove(name); err != nil {
				t.Errorf("remove the failing callback: %v", err)
			}
		})
		pinned := env.deliver(&member, func(m *gotgbot.Message) { m.Text = textSentinel })
		ids = append(ids, pinned, env.deliver(&env.del.owner, func(m *gotgbot.Message) {
			m.PinnedMessage = gotgbot.Message{MessageId: pinned, Chat: env.chat}
		}))
	})

	entries := hook.AllEntries()
	if len(entries) == 0 {
		t.Fatal("nothing was logged at debug level, so the test cannot discriminate")
	}
	deletionWantNoMessageIDs(t, hook, ids)
	categories := map[string]bool{}
	shed := map[string]int{}
	for _, entry := range entries {
		text := entry.Message
		for key, value := range entry.Data {
			text += " " + key + "=" + fmt.Sprint(value)
		}
		for _, leak := range []string{textSentinel, nameSentinel, userSentinel} {
			if strings.Contains(text, leak) {
				t.Errorf("log line at %s carries %q: %q", entry.Level, leak, text)
			}
		}
		if entry.Level > log.WarnLevel || !memberLogLine.MatchString(entry.Message) {
			continue
		}
		categories[entry.Message[:strings.Index(entry.Message, "]")+1]] = true
		// A Warn or Error line of the member deletion code names a chat, except the batch write's own line
		// (a count of chats) and the worker's.
		if !regexp.MustCompile(`chat -?\d+`).MatchString(entry.Message) && !strings.Contains(entry.Message, "chats failed") &&
			!strings.Contains(entry.Message, "stale claims") && !strings.Contains(entry.Message, "due rows") &&
			!strings.Contains(entry.Message, "rows:") {
			t.Errorf("a Warn line of the member deletion code names no chat: %q", entry.Message)
		}
		if strings.Contains(entry.Message, "were not queued") {
			shed[entry.Message]++
		}
	}
	for _, want := range []string{"[Deletion]", "[MemberDeletion]"} {
		if !categories[want] {
			t.Errorf("no Warn line of %s was logged, so the failing seams of its code were not driven (saw %v)", want, categories)
		}
	}
	for _, want := range []string{
		"approval of user",
		"the admin status of user",
		"is saturated",
		"captcha state of chat",
		"captcha state of user",
		"could not check chat",
		"(queue full)",
		"(rules unreadable)",
		"(count unreadable)",
		"(write failed)",
		"could not read the rules of chat",
		"could not be taken off the self-destruct queue",
	} {
		found := false
		for _, entry := range entries {
			if entry.Level <= log.WarnLevel && strings.Contains(entry.Message, want) {
				found = true
			}
		}
		if !found {
			t.Errorf("no Warn line contains %q: the failing seam was not driven", want)
		}
	}
	// Two sheds of one chat and reason in the full-buffer case are one line.
	for line, n := range shed {
		if strings.Contains(line, "(queue full)") && n != 1 {
			t.Errorf("the full-buffer shed was logged %d times, want one line per chat per hour: %q", n, line)
		}
	}
}
