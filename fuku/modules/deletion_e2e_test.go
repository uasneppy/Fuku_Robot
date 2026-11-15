//go:build testtools

package modules

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	log "github.com/sirupsen/logrus"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/captcha"
	"github.com/uasneppy/fuku_robot/fuku/db/deletion"
	"github.com/uasneppy/fuku_robot/fuku/db/greetings"
	"github.com/uasneppy/fuku_robot/fuku/db/memberreq"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/utils/actionlog"
)

// Scheduled deletion end to end (plan 16-09; DEL-01, DEL-02, DEL-06, DEL-07, DEL-08, MREQ-15).
// Everything is real but Telegram: the Deletion page, the real dispatcher (the ServiceDeletion,
// Greetings, Bans, Warns and member requirements modules), the queue in the database and the
// worker's cycle run through the registered code; only the Bot API is the hand-written fake. Time
// moves only through deletionNow, never by sleeping.

// deletionE2E is the whole slice in one supergroup: the settings environment (a live creator who
// presses the page), the member requirements toolkit, the deletion clock under the test's control
// and a real worker cycle over the same database.
type deletionE2E struct {
	*mreqSchedEnv
	del *settingsDelEnv
}

// newDeletionE2E builds the environment. The bot is an administrator with the delete right, the
// welcome text is "Welcome <first name>", and no rule is stored.
func newDeletionE2E(t *testing.T) *deletionE2E {
	t.Helper()
	del := newSettingsDelEnv(t)
	base := &memberReqEnv{settingsEnv: del.settingsEnv}
	withFastMemberReqPacer(t)
	memberReqCleanup(t, []int64{base.chat.Id}, nil)
	withMemberReqLive(t, true)
	t.Cleanup(memberReqJobs.wait)

	env := &deletionE2E{mreqSchedEnv: &mreqSchedEnv{memberReqEnv: base, clock: &atomic.Int64{}}, del: del}
	env.clock.Store(time.Now().UTC().Truncate(time.Second).Unix())
	mreqSwap(t, &deletionNow, func() time.Time { return time.Unix(env.clock.Load(), 0).UTC() })
	env.fake.sendDate = func() int64 { return env.clock.Load() - env.sentBack.Load() }

	LoadServiceDeletion(env.dispatcher)
	LoadBans(env.dispatcher)
	env.loadMemberReq()
	if err := greetings.SetWelcomeText(env.chat.Id, "Welcome {first}", "", nil, db.TEXT); err != nil {
		t.Fatalf("SetWelcomeText error = %v", err)
	}
	// Registered last, so it runs first: no inline delete is still in flight when the pacer and the
	// clients are put back.
	t.Cleanup(deletionInline.wait)
	return env
}

// deliver runs a message of the env chat through the real dispatcher, dated by the deletion clock.
func (e *deletionE2E) deliver(from *gotgbot.User, apply func(*gotgbot.Message)) int64 {
	e.t.Helper()
	id := e.updateID()
	msg := &gotgbot.Message{MessageId: id, Date: e.now().Unix(), Chat: e.chat, From: from}
	if apply != nil {
		apply(msg)
	}
	if err := e.dispatcher.ProcessUpdate(e.bot, &gotgbot.Update{UpdateId: id, Message: msg}, nil); err != nil {
		e.t.Fatalf("ProcessUpdate(%d) error = %v", id, err)
	}
	return id
}

// pin delivers a pinned-message notice sent by the creator.
func (e *deletionE2E) pin() int64 {
	e.t.Helper()
	return e.deliver(&e.del.owner, func(m *gotgbot.Message) {
		m.PinnedMessage = gotgbot.Message{MessageId: 1, Chat: e.chat}
	})
}

// cycle runs one real worker cycle with the env's bot.
func (e *deletionE2E) cycle() bool {
	e.t.Helper()
	return runDeletionCycle(context.Background(), e.bot)
}

// pass runs cycles back to back until one makes no progress, as the worker's loop does.
func (e *deletionE2E) pass() {
	e.t.Helper()
	for i := 0; e.cycle(); i++ {
		if i > 50 {
			e.t.Fatal("the worker made progress for 50 cycles in a row")
		}
	}
}

// deleted counts how many times Telegram was asked to delete message id of chatID, by a single
// deleteMessage or inside a deleteMessages batch, after every inline delete has finished.
func (e *deletionE2E) deleted(chatID, id int64) int {
	e.t.Helper()
	deletionInline.wait()
	n := 0
	for _, call := range e.fake.callsFor("deleteMessage") {
		if staffParamInt(call.Params, "chat_id") == chatID && staffParamInt(call.Params, "message_id") == id {
			n++
		}
	}
	for _, call := range e.fake.callsFor("deleteMessages") {
		if staffParamInt(call.Params, "chat_id") != chatID {
			continue
		}
		ids, ok := call.Params["message_ids"].([]int64)
		if !ok {
			e.t.Fatalf("deleteMessages message_ids = %T, want []int64", call.Params["message_ids"])
		}
		for _, got := range ids {
			if got == id {
				n++
			}
		}
	}
	return n
}

// deleteCalls is how many delete requests of either method went to chatID so far.
func (e *deletionE2E) deleteCalls(chatID int64) int {
	e.t.Helper()
	deletionInline.wait()
	n := 0
	for _, method := range []string{"deleteMessage", "deleteMessages"} {
		for _, call := range e.fake.callsFor(method) {
			if staffParamInt(call.Params, "chat_id") == chatID {
				n++
			}
		}
	}
	return n
}

// rows returns the queue rows of the env chat by message ID.
func (e *deletionE2E) rows() map[int64]models.ScheduledDeletion {
	e.t.Helper()
	deletionInline.wait()
	return e.queueRows()
}

// wantRow fails unless the queue holds a row for message id of kind, sent at sentAt and due at due.
func (e *deletionE2E) wantRow(id int64, kind string, sentAt, due time.Time) {
	e.t.Helper()
	row, ok := e.rows()[id]
	if !ok {
		e.t.Fatalf("queue rows = %+v, want one for message %d", e.rows(), id)
	}
	if row.Kind != kind || !row.SentAt.Equal(sentAt) || !row.DeleteAt.Equal(due) {
		e.t.Fatalf("queue row of message %d = kind %s sent %s due %s, want kind %s sent %s due %s",
			id, row.Kind, row.SentAt, row.DeleteAt, kind, sentAt, due)
	}
}

// sentIDs returns the message IDs of the messages the bot sent to the env chat.
func (e *deletionE2E) sentIDs() []int64 {
	e.t.Helper()
	var ids []int64
	for _, sent := range e.mreqNotices() {
		ids = append(ids, sent.MessageID)
	}
	return ids
}

// newSentSince returns the one message the bot sent to the env chat after the first n.
func (e *deletionE2E) newSentSince(n int) staffSentMessage {
	e.t.Helper()
	sent := e.mreqNotices()
	if len(sent) != n+1 {
		e.t.Fatalf("messages to the group = %d, want %d (one new): %q", len(sent), n+1, e.mreqSentTexts())
	}
	return sent[len(sent)-1]
}

// dueRows returns the queue rows ordered by their due time.
func (e *deletionE2E) dueRows() []models.ScheduledDeletion {
	e.t.Helper()
	var out []models.ScheduledDeletion
	for _, row := range e.rows() {
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].DeleteAt.Before(out[j].DeleteAt) })
	return out
}

func TestDeletionEndToEnd(t *testing.T) {
	t.Run("owner sets rules and notices are deleted", func(t *testing.T) {
		env := newDeletionE2E(t)
		m := env.del.groupMenu()

		// The owner presses Main, Deletion, Service messages, Pinned message and 10 min, then Chat
		// title and Delete now.
		m.openHub()
		m.open("dl", "s")
		m.open("dr", "sn")
		m.choose("sn", "600")
		m.open("dr", "sh")
		m.choose("sh", "0")
		if delay, ok := env.del.stored(deletion.KindPinned); !ok || delay != 600 {
			t.Fatalf("stored pinned = %d (%v), want 600", delay, ok)
		}
		if delay, ok := env.del.stored(deletion.KindTitle); !ok || delay != 0 {
			t.Fatalf("stored title = %d (%v), want a stored 0 (Delete now)", delay, ok)
		}

		// A title-change notice is deleted at once: one delete, no row.
		start := env.now()
		titleID := env.deliver(&env.del.owner, func(m *gotgbot.Message) { m.NewChatTitle = "A new title" })
		if got := env.deleted(env.chat.Id, titleID); got != 1 {
			t.Fatalf("deletes of the title notice = %d, want exactly 1", got)
		}
		if rows := env.rows(); len(rows) != 0 {
			t.Fatalf("queue rows after an inline delete = %+v, want none", rows)
		}

		// A service type with no rule is left alone.
		photoID := env.deliver(&env.del.owner, func(m *gotgbot.Message) { m.DeleteChatPhoto = true })
		if got := env.deleted(env.chat.Id, photoID); got != 0 {
			t.Fatalf("deletes of a photo notice with no rule = %d, want 0", got)
		}

		// A pin notice is queued at its date plus 600 s, and the worker deletes it when the clock
		// reaches that time and not before.
		pinID := env.pin()
		env.wantRow(pinID, deletion.KindPinned, start, start.Add(600*time.Second))
		callsBefore := env.deleteCalls(env.chat.Id)
		env.advance(599 * time.Second)
		env.pass()
		if got := env.deleted(env.chat.Id, pinID); got != 0 || env.deleteCalls(env.chat.Id) != callsBefore {
			t.Fatalf("the worker at +599 s deleted the pin notice %d times (%d new calls), want nothing yet", got, env.deleteCalls(env.chat.Id)-callsBefore)
		}
		if _, ok := env.rows()[pinID]; !ok {
			t.Fatal("the pin row left the queue before its time")
		}
		env.advance(2 * time.Second)
		env.pass()
		if got := env.deleted(env.chat.Id, pinID); got != 1 {
			t.Fatalf("deletes of the pin notice at +601 s = %d, want exactly 1", got)
		}
		if batches := env.fake.callsFor("deleteMessages"); len(batches) != 1 {
			t.Errorf("deleteMessages calls = %d, want the one batch of the worker", len(batches))
		}
		if rows := env.rows(); len(rows) != 0 {
			t.Fatalf("queue rows after the worker = %+v, want none", rows)
		}
		if got := env.deleted(env.chat.Id, photoID); got != 0 {
			t.Fatalf("the photo notice was deleted %d times, want never", got)
		}
	})

	t.Run("bot messages", func(t *testing.T) {
		env := newDeletionE2E(t)
		m := env.del.groupMenu()
		// Welcome at 1 minute, Manual punishments at 10 minutes and Auto punishments at 1 hour,
		// set through the page.
		m.openHub()
		m.open("dl", "b")
		for _, set := range [][2]string{{"bw", "60"}, {"bm", "600"}, {"ba", "3600"}} {
			m.open("dr", set[0])
			m.choose(set[0], set[1])
		}
		for kind, want := range map[string]int{deletion.KindBotWelcome: 60, deletion.KindBotManual: 600, deletion.KindBotAuto: 3600} {
			if delay, ok := env.del.stored(kind); !ok || delay != want {
				t.Fatalf("stored %s = %d (%v), want %d", kind, delay, ok, want)
			}
		}

		// A join gets one welcome and one row at its date plus 60 s.
		joiner := env.mreqMember("Jo", "Join", "jo")
		before := len(env.mreqNotices())
		sentAt := env.now()
		env.mreqJoin(joiner)
		welcome := env.newSentSince(before)
		if !strings.Contains(mreqText(welcome), mreqWelcomeWord) {
			t.Fatalf("the message after the join = %q, want the welcome", mreqText(welcome))
		}
		env.wantRow(welcome.MessageID, deletion.KindBotWelcome, sentAt, sentAt.Add(60*time.Second))

		// An admin's /ban reply writes one row at + 600 s.
		target := env.mreqMember("Bad", "Actor", "bad")
		before = len(env.mreqNotices())
		env.deliver(&env.del.owner, func(m *gotgbot.Message) {
			m.Text = fmt.Sprintf("/ban %d", target.Id)
			m.Entities = []gotgbot.MessageEntity{{Type: "bot_command", Offset: 0, Length: 4}}
		})
		banReply := env.newSentSince(before)
		env.wantRow(banReply.MessageID, deletion.KindBotManual, sentAt, sentAt.Add(600*time.Second))

		// A requirement Mute notice with its button writes one row at its date + 86400 s, and the
		// member's I fixed it tap moves that row to the tap + 3600 s.
		env.mreqRules(map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionMute})
		member := env.mreqMember("Alex", "", "alex")
		muteNotice := env.mreqMuteByMessage(member)
		env.wantRow(muteNotice.MessageID, deletion.KindBotAuto, sentAt, sentAt.Add(24*time.Hour))
		fixed := env.mreqFix(member)
		env.advance(20 * time.Minute)
		tapped := env.now()
		env.mreqPressFixed(fixed, muteNotice)
		env.wantRow(muteNotice.MessageID, deletion.KindBotAuto, sentAt, tapped.Add(3600*time.Second))

		// A requirement Warn notice (no button) writes a row at + 3600 s.
		env.mreqRules(map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionWarn})
		warned := env.mreqMember("Wren", "", "wren")
		before = len(env.mreqNotices())
		warnSent := env.now()
		env.mreqSend(warned, "plain text")
		memberReqJobs.wait()
		warnNotice := env.newSentSince(before)
		env.wantRow(warnNotice.MessageID, deletion.KindBotAuto, warnSent, warnSent.Add(3600*time.Second))

		// The worker deletes each at its time, and not before.
		due := env.dueRows()
		if len(due) != 4 {
			t.Fatalf("queue rows = %+v, want four (welcome, ban reply, mute notice, warn notice)", due)
		}
		for _, row := range due {
			if at := row.DeleteAt.Add(-time.Second); at.After(env.now()) {
				env.advance(at.Sub(env.now()))
				env.pass()
				if got := env.deleted(env.chat.Id, row.MessageID); got != 0 {
					t.Fatalf("message %d (%s) deleted %d times one second before its time", row.MessageID, row.Kind, got)
				}
			}
			env.advance(row.DeleteAt.Sub(env.now()))
			env.pass()
			if got := env.deleted(env.chat.Id, row.MessageID); got != 1 {
				t.Fatalf("message %d (%s) deleted %d times at its time, want exactly 1", row.MessageID, row.Kind, got)
			}
		}
		if rows := env.rows(); len(rows) != 0 {
			t.Fatalf("queue rows after every time passed = %+v, want none", rows)
		}
	})

	t.Run("joined and left share /cleanservice", func(t *testing.T) {
		env := newDeletionE2E(t)
		m := env.del.groupMenu()
		now, keep := "🗑 "+staffMarker("settings_del_state_now"), "➖ "+staffMarker("settings_del_state_keep")
		pageLine := func(code string) string {
			text, _ := m.open("dr", code)
			return text
		}
		flagOn := func() bool {
			settings, err := deletionFlagLookup(context.Background(), env.chat.Id)
			if err != nil || settings == nil {
				t.Fatalf("read the shared flag = %v, %v", settings, err)
			}
			return settings.ShouldCleanService
		}
		welcomes := func() int { return mreqCountText(env.mreqSentTexts(), mreqWelcomeWord) }
		newcomer := func(name string) gotgbot.User {
			return gotgbot.User{Id: uniqueLinkOwnerID(), FirstName: name}
		}
		memberCleanup := func(users ...gotgbot.User) {
			ids := make([]int64, 0, len(users))
			for _, user := range users {
				ids = append(ids, user.Id)
			}
			memberReqCleanup(t, nil, ids)
		}

		// /cleanservice on: the page reads both as Delete now and a join message is deleted after
		// its welcome.
		env.command(&env.del.owner, "/cleanservice on")
		if !flagOn() {
			t.Fatal("/cleanservice on did not turn the shared flag on")
		}
		for _, code := range []string{"sj", "sl"} {
			if text := pageLine(code); !strings.Contains(text, now) {
				t.Errorf("after /cleanservice on the %s screen reads %q, want Delete now", code, text)
			}
		}
		first := newcomer("First")
		memberCleanup(first)
		joinID := env.deliver(&first, deletionJoinApply(first))
		if got := env.deleted(env.chat.Id, joinID); got != 1 {
			t.Fatalf("deletes of the join message after /cleanservice on = %d, want 1", got)
		}
		if got := welcomes(); got != 1 {
			t.Fatalf("welcomes = %d, want the join still welcomed once before its message was deleted", got)
		}
		order := env.methods()
		send, del := -1, -1
		for i, method := range order {
			if method == "sendMessage" && send < 0 {
				send = i
			}
			if method == "deleteMessage" && del < 0 {
				del = i
			}
		}
		if send < 0 || del < 0 || send > del {
			t.Fatalf("call order = %v, want the welcome before the join message's delete", order)
		}

		// A join service message caused by a bot is the watcher's and follows the same rule.
		adder := gotgbot.User{Id: uniqueLinkOwnerID(), IsBot: true, FirstName: "Adder"}
		second := newcomer("Second")
		memberCleanup(second)
		botJoinID := env.deliver(&adder, deletionJoinApply(second))
		if got := env.deleted(env.chat.Id, botJoinID); got != 1 {
			t.Fatalf("deletes of a bot-caused join message with the flag on = %d, want 1 (the watcher's)", got)
		}

		// /cleanservice off: both Keep, both delay rows removed, and nothing is deleted.
		if err := deletion.SetRule(t.Context(), env.chat.Id, deletion.KindLeft, 600); err != nil {
			t.Fatalf("SetRule(left) error = %v", err)
		}
		env.command(&env.del.owner, "/cleanservice off")
		if flagOn() {
			t.Fatal("/cleanservice off left the shared flag on")
		}
		for _, code := range []string{"sj", "sl"} {
			if text := pageLine(code); !strings.Contains(text, keep) {
				t.Errorf("after /cleanservice off the %s screen reads %q, want Keep", code, text)
			}
		}
		for _, kind := range []string{deletion.KindJoined, deletion.KindLeft} {
			if _, ok := env.del.stored(kind); ok {
				t.Errorf("/cleanservice off left the %s delay row stored", kind)
			}
		}
		third := newcomer("Third")
		memberCleanup(third)
		keptID := env.deliver(&third, deletionJoinApply(third))
		if got := env.deleted(env.chat.Id, keptID); got != 0 || len(env.rows()) != 0 {
			t.Fatalf("with Keep the join message was deleted %d times and wrote %d rows, want neither", got, len(env.rows()))
		}

		// Member joined After 10 min from the page writes the row and the shared flag.
		m.open("dr", "sj")
		m.choose("sj", "600")
		if delay, ok := env.del.stored(deletion.KindJoined); !ok || delay != 600 {
			t.Fatalf("stored joined = %d (%v), want 600", delay, ok)
		}
		if !flagOn() {
			t.Fatal("After 10 min on Member joined did not turn the shared flag on")
		}
		if text := pageLine("sl"); !strings.Contains(text, now) {
			t.Errorf("Member left reads %q, want Delete now through the shared flag", text)
		}
		fourth := newcomer("Fourth")
		memberCleanup(fourth)
		sentAt := env.now()
		queuedID := env.deliver(&fourth, deletionJoinApply(fourth))
		env.wantRow(queuedID, deletion.KindJoined, sentAt, sentAt.Add(600*time.Second))
		if got := env.deleted(env.chat.Id, queuedID); got != 0 {
			t.Fatalf("the After 10 min join message was deleted %d times at once, want 0", got)
		}
		leftID := env.deliver(&fourth, deletionLeftApply(fourth))
		if got := env.deleted(env.chat.Id, leftID); got != 1 {
			t.Fatalf("deletes of the leave message with the flag on and no row = %d, want 1", got)
		}
		// A bot-caused join writes one more row, by the watcher.
		fifth := newcomer("Fifth")
		memberCleanup(fifth)
		botRowID := env.deliver(&adder, deletionJoinApply(fifth))
		env.wantRow(botRowID, deletion.KindJoined, sentAt, sentAt.Add(600*time.Second))
		env.advance(601 * time.Second)
		env.pass()
		for _, id := range []int64{queuedID, botRowID} {
			if got := env.deleted(env.chat.Id, id); got != 1 {
				t.Fatalf("message %d deleted %d times at its time, want 1", id, got)
			}
		}
	})

	t.Run("the bot cannot delete", func(t *testing.T) {
		noRight, unknown := staffMarker("settings_del_no_right"), staffMarker("settings_del_right_unknown")
		t.Run("a plain member bot: the warning on every screen, the service latch only after the live check", func(t *testing.T) {
			env := newDeletionE2E(t)
			m := env.del.groupMenu()
			m.open("dr", "sn")
			m.choose("sn", "600")
			m.open("dr", "bw")
			m.choose("bw", "60")
			env.del.setBot(settingsDelBotJSON("member", false))
			for _, screen := range settingsDelScreens {
				text := screen.draw(m)
				if n := strings.Count(text, noRight); n != 1 {
					t.Errorf("%s: the warning line appears %d times, want once: %q", screen.name, n, text)
				}
				if containsMarker(text, "settings_cannot_open") {
					t.Errorf("%s: the screen failed to open", screen.name)
				}
			}
			if _, ok := env.del.stored(deletion.KindPinned); !ok {
				t.Fatal("the rule was not kept while the warning was shown")
			}

			// A pin notice and a welcome are queued; Telegram refuses the pin notice.
			pinID := env.pin()
			joiner := env.mreqMember("Jo", "", "jo")
			before := len(env.mreqNotices())
			env.mreqJoin(joiner)
			welcomeID := env.newSentSince(before).MessageID
			env.del.script.set(func(_ context.Context, method string, params map[string]any) error {
				switch method {
				case "deleteMessages":
					return deletionRefusal()
				case "deleteMessage":
					if staffParamInt(params, "message_id") == pinID {
						return deletionRefusal()
					}
				}
				return nil
			})
			if got := env.redisGet(deletionLatchKey(env.chat.Id)); got != "" {
				t.Fatalf("latch before the worker = %q, want none", got)
			}
			reads := env.del.botReads(env.chat.Id)
			env.advance(601 * time.Second)
			env.pass()
			if got := env.redisGet(deletionLatchKey(env.chat.Id)); got != deletionScopeService {
				t.Fatalf("latch after the refusal = %q, want %q", got, deletionScopeService)
			}
			if n := env.del.botReads(env.chat.Id) - reads; n != 1 {
				t.Fatalf("live reads of the bot's right = %d, want exactly 1 after the refusal", n)
			}
			// The batch is refused as a whole, so each message is then tried alone: the welcome is
			// deleted by its own single call, the pin notice is only tried and dropped.
			singles := 0
			for _, id := range (&deletionEnv{t: t, fake: env.fake}).singleCalls(env.chat.Id) {
				if id == welcomeID {
					singles++
				}
			}
			if singles != 1 {
				t.Fatalf("single deletes of the welcome of the same chat = %d, want 1 (bot kinds are not latched)", singles)
			}
			if rows := env.rows(); len(rows) != 0 {
				t.Fatalf("queue rows after the worker = %+v, want none", rows)
			}
			// The latched chat takes no more service rows, and still takes bot rows.
			env.del.script.set(nil)
			latchedPin := env.pin()
			if _, ok := env.rows()[latchedPin]; ok || env.deleted(env.chat.Id, latchedPin) != 0 {
				t.Fatal("a pin notice of a latched chat was queued or deleted")
			}
			again := env.mreqMember("Jay", "", "jay")
			before = len(env.mreqNotices())
			env.mreqJoin(again)
			env.wantRow(env.newSentSince(before).MessageID, deletion.KindBotWelcome, env.now(), env.now().Add(60*time.Second))
		})

		t.Run("an unreadable right draws the unknown line and never fails a page", func(t *testing.T) {
			mreqSwap(t, &settingsDelHereTimeout, 50*time.Millisecond)
			env := newDeletionE2E(t)
			m := env.del.groupMenu()
			m.open("dr", "sn")
			m.choose("sn", "600")
			env.del.scriptBot(false, staffFakeError(500, "Internal Server Error"))
			for _, screen := range settingsDelScreens {
				text := screen.draw(m)
				if n := strings.Count(text, unknown); n != 1 || strings.Contains(text, noRight) {
					t.Errorf("%s: unknown line %d times, no-right line present = %v: %q", screen.name, n, strings.Contains(text, noRight), text)
				}
				if containsMarker(text, "settings_cannot_open") {
					t.Errorf("%s: the screen failed to open", screen.name)
				}
			}
			if delay, ok := env.del.stored(deletion.KindPinned); !ok || delay != 600 {
				t.Fatalf("stored pinned = %d (%v), want the rule untouched", delay, ok)
			}
			// A rule can still be set while the right is unknown.
			m.choose("sn", "3600")
			if delay, ok := env.del.stored(deletion.KindPinned); !ok || delay != 3600 {
				t.Fatalf("stored pinned after a press = %d (%v), want 3600", delay, ok)
			}
		})
	})

	t.Run("Staff Group", func(t *testing.T) {
		env := newDeletionE2E(t)
		m := env.del.groupMenu()
		// Every rule is stored first, so a refusal is the Staff Group's and not a missing rule.
		for _, row := range deletionServiceRows {
			if !deletion.SharesCleanServiceFlag(row.Kind) {
				env.del.setRule(row.Kind, 0)
			}
		}
		for _, row := range deletionBotRows {
			env.del.setRule(row.Kind, 60)
		}
		env.del.setJoinLeft(deletion.KindJoined, deletion.JoinLeftState{Mode: deletion.JoinLeftNow})
		env.del.makeStaff()

		// Every service type: no row, no delete.
		for name, code := range deletionServiceFields {
			id := env.deliver(&env.del.owner, func(msg *gotgbot.Message) {
				built := deletionMessageWith(t, name)
				built.MessageId, built.Date, built.Chat, built.From = msg.MessageId, msg.Date, msg.Chat, msg.From
				*msg = *built
			})
			if got := env.deleted(env.chat.Id, id); got != 0 {
				t.Errorf("%s (%s): a Staff Group message was deleted %d times", name, code, got)
			}
		}
		for _, apply := range []func(*gotgbot.Message){
			deletionJoinApply(gotgbot.User{Id: uniqueLinkOwnerID(), FirstName: "Mem"}),
			deletionLeftApply(gotgbot.User{Id: uniqueLinkOwnerID(), FirstName: "Mem"}),
		} {
			id := env.deliver(&env.del.owner, apply)
			if got := env.deleted(env.chat.Id, id); got != 0 {
				t.Errorf("a joined or left notice of a Staff Group was deleted %d times", got)
			}
		}
		// Every bot kind: the helper refuses the chat.
		for _, row := range deletionBotRows {
			scheduleBotMessage(&gotgbot.Message{MessageId: 880000000, Date: env.now().Unix(), Chat: env.chat}, row.Kind)
		}
		if rows := env.rows(); len(rows) != 0 {
			t.Fatalf("queue rows of a Staff Group = %+v, want none", rows)
		}
		if got := env.deleteCalls(env.chat.Id); got != 0 {
			t.Fatalf("delete calls in a Staff Group = %d, want 0", got)
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
		if _, ok := env.del.stored(deletion.KindPinned); !ok {
			t.Error("the stored pinned rule vanished: a forged press wrote")
		}
	})

	t.Run("restart, replicas, old rows", func(t *testing.T) {
		env := newDeletionE2E(t)
		env.del.setRule(deletion.KindPinned, 600)
		start := env.now()
		var pins []int64
		for i := 0; i < 3; i++ {
			pins = append(pins, env.pin())
		}
		// A worker that crashed left a row acting with an old claim, and a row of a message 48
		// hours old is waiting.
		claimedAt := start.Add(10*time.Minute - 3*time.Minute)
		crashed := models.ScheduledDeletion{
			ChatID: env.chat.Id, MessageID: 880000100, Kind: deletion.KindPinned, SentAt: start, DeleteAt: start.Add(10 * time.Minute),
			State: models.ScheduledDeletionActing, ClaimToken: "deadbeefdeadbeefdeadbeefdeadbeef", ClaimedAt: &claimedAt, CreatedAt: start,
		}
		if err := db.DB.Create(&crashed).Error; err != nil {
			t.Fatalf("seed the crashed row: %v", err)
		}
		oldID := int64(880000200)
		seedDeletionRow(t, env.chat.Id, oldID, start, start.Add(-48*time.Hour), deletion.KindPinned)
		// A delay over 24 hours is refused outright.
		if _, err := deletionSchedule(context.Background(), deletion.Entry{
			ChatID: env.chat.Id, MessageID: 880000300, Kind: deletion.KindBotRules, SentAt: start, DelaySecs: deletion.MaxDelaySecs + 1,
		}); err == nil {
			t.Fatal("a delay over 24 hours was accepted")
		}
		env.advance(10*time.Minute + time.Second)
		hook := mreqShedHook(t)

		// Two workers over one database: each message goes out once. The second has a fake of its own.
		second := env.replicaBot()
		var wg atomic.Int32
		wg.Store(2)
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
		wantOnce := append(append([]int64{}, pins...), crashed.MessageID)
		for _, id := range wantOnce {
			if got := env.deleted(env.chat.Id, id) + second.deleted(env.chat.Id, id); got != 1 {
				t.Errorf("message %d was deleted %d times across the two workers, want exactly 1", id, got)
			}
		}
		if got := env.deleted(env.chat.Id, oldID) + second.deleted(env.chat.Id, oldID); got != 0 {
			t.Errorf("the 48 hour old message was sent to Telegram %d times, want never", got)
		}
		deletionWantNoMessageIDs(t, hook, append(wantOnce, oldID))
	})
}

// command delivers a bot command sent by from to the env chat.
func (e *deletionE2E) command(from *gotgbot.User, text string) int64 {
	e.t.Helper()
	length := len(text)
	if space := strings.IndexByte(text, ' '); space >= 0 {
		length = space
	}
	return e.deliver(from, func(m *gotgbot.Message) {
		m.Text = text
		m.Entities = []gotgbot.MessageEntity{{Type: "bot_command", Offset: 0, Length: int64(length)}}
	})
}

// redisGet reads a key of the env's Redis, or "" when it is not there.
func (e *deletionE2E) redisGet(key string) string {
	value, err := e.redis.Get(key)
	if err != nil {
		return ""
	}
	return value
}

// methods returns the Bot API methods the env's fake was asked for, in order.
func (e *deletionE2E) methods() []string {
	e.t.Helper()
	deletionInline.wait()
	return (&deletionEnv{t: e.t, fake: e.fake}).methods()
}

// deletionE2EReplica is a second bot process over the same database and Redis with its own fake.
type deletionE2EReplica struct {
	bot  *gotgbot.Bot
	fake *lockdownFake
}

// replicaBot builds the second process; it too is an administrator with the delete right.
func (e *deletionE2E) replicaBot() *deletionE2EReplica {
	e.t.Helper()
	fake := newLockdownFake()
	fake.sendDate = func() int64 { return e.clock.Load() }
	bot := newModuleTestBot(fake.moduleBotClient)
	bot.BotClient = &deletionScriptClient{BotClient: fake, fake: fake}
	fake.setBotMember(e.chat.Id, lockdownBotAdminJSON(false, true, false))
	return &deletionE2EReplica{bot: bot, fake: fake}
}

// deleted counts the delete requests of message id in chatID this process sent.
func (r *deletionE2EReplica) deleted(chatID, id int64) int {
	n := 0
	for _, call := range r.fake.callsFor("deleteMessage") {
		if staffParamInt(call.Params, "chat_id") == chatID && staffParamInt(call.Params, "message_id") == id {
			n++
		}
	}
	for _, call := range r.fake.callsFor("deleteMessages") {
		if staffParamInt(call.Params, "chat_id") != chatID {
			continue
		}
		if ids, ok := call.Params["message_ids"].([]int64); ok {
			for _, got := range ids {
				if got == id {
					n++
				}
			}
		}
	}
	return n
}

// deletionShortestRules stores the shortest valid delay of every kind for each chat: Delete now for the
// service kinds (the shared joined and left rows included), 60 s for the bot kinds and, for the member
// kinds, the shortest preset each accepts (commands 0, edited 30, self-destruct 60: deletion.ValidateRuleDelay
// refuses anything else). The rows are removed when the test ends.
func deletionShortestRules(t *testing.T, chatIDs ...int64) {
	t.Helper()
	deletionCleanup(t, chatIDs...)
	for _, chatID := range chatIDs {
		for _, kind := range deletion.Kinds() {
			delay := 0
			switch {
			case deletion.IsBotKind(kind), kind == deletion.KindMemberSelfDestruct:
				delay = 60
			case kind == deletion.KindMemberEdited:
				delay = 30
			}
			if err := deletion.SetRule(t.Context(), chatID, kind, delay); err != nil {
				t.Fatalf("SetRule(%d, %s, %d) error = %v", chatID, kind, delay, err)
			}
		}
	}
}

// deletionQueueRowsOf counts the queue rows of the given chats.
func deletionQueueRowsOf(t *testing.T, chatIDs ...int64) int {
	t.Helper()
	var rows []models.ScheduledDeletion
	if err := db.DB.Where("chat_id IN ?", chatIDs).Find(&rows).Error; err != nil {
		t.Fatalf("read the queue: %v", err)
	}
	return len(rows)
}

// deletionWantUntouched fails when any of the chats has a queue row or saw a delete request, then
// moves the clock a day and a half on, runs one worker cycle with bot and checks again: a row nobody
// can see would have gone out by then.
func deletionWantUntouched(t *testing.T, bot *gotgbot.Bot, what string, deletes func(chatID int64) int, chatIDs ...int64) {
	t.Helper()
	check := func(when string) {
		t.Helper()
		if n := deletionQueueRowsOf(t, chatIDs...); n != 0 {
			t.Errorf("%s (%s): %d queue rows, want none", what, when, n)
		}
		for _, chatID := range chatIDs {
			if n := deletes(chatID); n != 0 {
				t.Errorf("%s (%s): %d delete requests to chat %d, want none", what, when, n, chatID)
			}
		}
	}
	check("after the flow")
	runDeletionCycle(context.Background(), bot)
	check("after a worker cycle a day and a half later")
}

// deletionDayAndAHalf moves the deletion clock to a fixed instant and returns a function that moves
// it a day and a half on.
func deletionClockFixed(t *testing.T) (advance func()) {
	t.Helper()
	var clock atomic.Int64
	clock.Store(time.Now().UTC().Truncate(time.Second).Unix())
	mreqSwap(t, &deletionNow, func() time.Time { return time.Unix(clock.Load(), 0).UTC() })
	return func() { clock.Add(int64(36 * time.Hour / time.Second)) }
}

func TestDeletionExcludedMessagesNeverScheduled(t *testing.T) {
	t.Run("a Staff Group action card and summary", func(t *testing.T) {
		advance := deletionClockFixed(t)
		withFastDeletionPacer(t)
		env := newStaffActionEnv(t, 2)
		LoadServiceDeletion(env.dispatcher)
		all := append([]int64{env.staffChat}, env.groups...)
		deletionShortestRules(t, all...)
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
		deletionShortestRules(t, env.chat.Id, staffChat.Id)

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
		deletionShortestRules(t, s.chat.Id)
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
		deletionShortestRules(t, s.chat.Id, channelID)
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
}

// chatCalls returns the calls of method the scripted client recorded for chatID.
func chatCalls(client *moduleBotClient, method string, chatID int64) []moduleBotCall {
	var matched []moduleBotCall
	for _, call := range client.callsFor(method) {
		if staffParamInt(call.Params, "chat_id") == chatID {
			matched = append(matched, call)
		}
	}
	return matched
}

func TestDeletionLogsCarryIDsOnly(t *testing.T) {
	const textSentinel, nameSentinel = "ZZLEAKTEXT", "ZZLEAKNAME"
	env := newDeletionE2E(t)
	hook := mreqShedHook(t)
	if err := greetings.SetWelcomeText(env.chat.Id, "Welcome {first} "+textSentinel, "", nil, db.TEXT); err != nil {
		t.Fatalf("SetWelcomeText error = %v", err)
	}
	var ids []int64
	member := func() gotgbot.User { return env.mreqMember(nameSentinel, "", "zzleakuser") }
	failing := errors.New("lookup failed")

	// The watcher with a rules read, a Staff Group read and a queue write that fail, and the
	// shared flag's read for joined and left.
	env.del.setRule(deletion.KindPinned, 600)
	t.Run("rules read fails", func(t *testing.T) {
		mreqSwap(t, &deletionRulesLookup, func(context.Context, int64) (*deletion.Rules, error) { return nil, failing })
		ids = append(ids, env.pin())
	})
	t.Run("Staff Group read fails", func(t *testing.T) {
		mreqSwap(t, &deletionStaffLookup, func(int64) (*models.StaffGroup, error) { return nil, failing })
		ids = append(ids, env.pin())
	})
	t.Run("queue write fails", func(t *testing.T) {
		mreqSwap(t, &deletionQueueWrite, func(context.Context, deletion.Entry) (bool, error) { return false, failing })
		ids = append(ids, env.pin())
	})
	t.Run("flag read fails", func(t *testing.T) {
		mreqSwap(t, &deletionFlagLookup, func(context.Context, int64) (*models.GreetingSettings, error) { return nil, failing })
		joiner := member()
		ids = append(ids, env.deliver(&joiner, deletionJoinApply(joiner)))
	})

	// The hooked sends: the welcome of a join with the rules read failing, then the queue write failing.
	env.del.setRule(deletion.KindBotWelcome, 60)
	for _, name := range []string{"a rules read that fails", "a queue write that fails"} {
		t.Run("hooked send with "+name, func(t *testing.T) {
			if strings.Contains(name, "rules") {
				mreqSwap(t, &deletionRulesLookup, func(context.Context, int64) (*deletion.Rules, error) { return nil, failing })
			} else {
				mreqSwap(t, &deletionQueueWrite, func(context.Context, deletion.Entry) (bool, error) { return false, failing })
			}
			joiner := member()
			env.mreqJoin(joiner)
			ids = append(ids, env.sentIDs()...)
		})
	}

	// A message that goes through everything, then the worker's failure shapes.
	chats := []int64{uniqueModuleChatID(), uniqueModuleChatID(), uniqueModuleChatID(), uniqueModuleChatID()}
	deletionCleanup(t, chats...)
	staffCleanup(t, chats...)
	seed := func(chatID, first int64) []int64 {
		out := make([]int64, 0, 3)
		for i := int64(0); i < 3; i++ {
			seedDeletionRow(t, chatID, first+i, env.now().Add(-time.Hour), env.now().Add(-2*time.Hour), deletion.KindBotWelcome)
			out = append(out, first+i)
		}
		ids = append(ids, out...)
		return out
	}
	refused, server, transport, _ := seed(chats[0], 880000100), seed(chats[1], 880000200), seed(chats[2], 880000300), seed(chats[3], 880000400)
	for _, chatID := range chats {
		env.fake.setBotMember(chatID, lockdownBotAdminJSON(false, true, false))
	}
	env.del.script.set(func(_ context.Context, method string, params map[string]any) error {
		chatID, messageID := staffParamInt(params, "chat_id"), staffParamInt(params, "message_id")
		switch chatID {
		case chats[0]:
			if method == "deleteMessages" || messageID == refused[1] {
				return deletionRefusal()
			}
		case chats[1]:
			if method == "deleteMessages" || messageID == server[1] {
				return staffFakeError(500, "Internal Server Error")
			}
		case chats[2]:
			if method == "deleteMessages" || messageID == transport[1] {
				return errors.New("dial tcp 10.0.0.1:443: i/o timeout")
			}
		case chats[3]:
			return staffFakeError(403, "Forbidden: bot was kicked from the supergroup chat")
		}
		return nil
	})
	env.pass()

	entries := hook.AllEntries()
	if len(entries) == 0 {
		t.Fatal("nothing was logged at debug level, so the test cannot discriminate")
	}
	deletionWantNoMessageIDs(t, hook, ids)
	categories := map[string]bool{}
	for _, entry := range entries {
		text := entry.Message
		for key, value := range entry.Data {
			text += " " + key + "=" + fmt.Sprint(value)
		}
		for _, leak := range []string{textSentinel, nameSentinel, "zzleakuser"} {
			if strings.Contains(text, leak) {
				t.Errorf("log line at %s carries %q: %q", entry.Level, leak, text)
			}
		}
		if entry.Level > log.WarnLevel || !deletionLogLine.MatchString(entry.Message) {
			continue
		}
		categories[entry.Message[:strings.Index(entry.Message, "]")+1]] = true
		// A Warn or Error line of the deletion code names a chat and, when there is one, a kind.
		if !regexp.MustCompile(`chat -?\d+`).MatchString(entry.Message) && !strings.Contains(entry.Message, "stale claims") &&
			!strings.Contains(entry.Message, "due rows") && !strings.Contains(entry.Message, "rows:") {
			t.Errorf("a Warn line of the deletion code names no chat: %q", entry.Message)
		}
	}
	for _, want := range []string{"[Deletion]", "[ServiceDeletion]"} {
		if !categories[want] {
			t.Errorf("no Warn line of %s was logged, so the failing seam of its code was not driven (saw %v)", want, categories)
		}
	}
	for _, want := range []string{
		"could not read the deletion rule of chat",
		"could not check chat",
		"could not queue a deletion in chat",
		"could not read the rules of chat",
		"could not schedule a bot:welcome message in chat",
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
}

// deletionLogLine matches the category prefix of the deletion code's log lines.
var deletionLogLine = regexp.MustCompile(`^\[(Deletion|ServiceDeletion)\]`)
