//go:build testtools

package modules

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

const (
	// busyBurstSize is how many messages the burst sends: 30 deleteMessages calls of 100 IDs.
	busyBurstSize = 3000
	// busyRuleDelay is the self-destruct delay of the busy-group test.
	busyRuleDelay = 60
)

// busyOtherChat returns a second supergroup of the env whose own rows the test removes.
func busyOtherChat(t *testing.T) gotgbot.Chat {
	t.Helper()
	chat := gotgbot.Chat{Id: uniqueModuleChatID(), Type: "supergroup", Title: "Other"}
	deletionCleanup(t, chat.Id)
	return chat
}

// busySendCommand sends the reply command from a fresh member in chat through the real dispatcher.
func busySendCommand(env *memberDeletionEnv, name string, chat gotgbot.Chat) {
	member := env.member()
	env.sendMemberIn(chat, &member, commandMessage("/"+name))
}

// TestSelfDestructBusyGroup proves the busy-group guarantees of the self-destruct rule over the real
// worker and the real 100 ms deletion pacer with only Telegram faked (D-08, D-09, D-13 as amended by
// D-32, PLAT-06, research S-06): offers never wait and write nothing, the flush writes the burst, the
// deletions are exactly one paced call per 100 messages on a budget of their own, a block of that
// budget never slows a reply, a 429 only delays deletions, and the ceiling sheds the excess.
func TestSelfDestructBusyGroup(t *testing.T) {
	t.Run("burst", func(t *testing.T) {
		env := newSelfDestructEnv(t, busyRuleDelay)
		env.withRealDeletionPacer(t)
		setNow := selfDestructClock(t, env.now)
		members := []gotgbot.User{env.member(), env.member(), env.member()}
		ids := make([]int64, 0, busyBurstSize)
		for i := range busyBurstSize {
			ids = append(ids, env.sendMember(&members[i%len(members)], func(m *gotgbot.Message) { m.Text = "busy" }))
		}
		if got := len(memberDeleteQueue); got != busyBurstSize {
			t.Fatalf("buffered entries = %d, want %d: every update only offered", got, busyBurstSize)
		}
		var rowCount int64
		if err := db.DB.Model(&models.ScheduledDeletion{}).Where("chat_id = ?", env.chat.Id).Count(&rowCount).Error; err != nil {
			t.Fatalf("count the rows: %v", err)
		}
		if rowCount != 0 {
			t.Fatalf("rows before the flush = %d, want 0: no update wrote", rowCount)
		}

		if got := memberDeleteTestFlush(t); got != busyBurstSize {
			t.Fatalf("memberDeleteFlush() = %d, want all %d written", got, busyBurstSize)
		}
		for reason := range memberDeleteShedReasons {
			if got := memberDeleteShedTotal(reason); got != 0 {
				t.Fatalf("shed for %s = %d, want 0: the burst fits the buffer and the ceiling", memberDeleteShedNames[reason], got)
			}
		}
		if rows := env.selfDestructRows(env.chat.Id); len(rows) != busyBurstSize {
			t.Fatalf("rows after the flush = %d, want %d", len(rows), busyBurstSize)
		}

		setNow(env.now.Add((busyRuleDelay + 1) * time.Second))
		calls := func() int { return len(env.fake.callsFor("deleteMessages")) }
		total := 0
		started := time.Now()
		for cycles := 0; ; cycles++ {
			if cycles > 10 {
				t.Fatal("the worker made progress for 10 cycles in a row")
			}
			before := calls()
			progressed := env.cycle()
			// C-14: a cycle claims at most deletionBatch rows of one chat, so it makes at most
			// ceil(rows/100) calls.
			if delta := calls() - before; delta > (deletionBatch+99)/100 {
				t.Fatalf("a cycle made %d deleteMessages calls, want at most %d", delta, (deletionBatch+99)/100)
			}
			total = calls()
			if !progressed {
				break
			}
		}
		elapsed := time.Since(started)

		batches := env.batchCalls(env.chat.Id)
		if len(batches) != busyBurstSize/100 {
			t.Fatalf("deleteMessages calls = %d, want %d", len(batches), busyBurstSize/100)
		}
		var deleted []int64
		for _, batch := range batches {
			if len(batch) != 100 {
				t.Fatalf("a deleteMessages call held %d IDs, want 100", len(batch))
			}
			deleted = append(deleted, batch...)
		}
		slices.Sort(deleted)
		if !slices.Equal(deleted, ids) {
			t.Fatalf("deleted %d IDs, want exactly the %d sent", len(deleted), len(ids))
		}
		if singles := env.singleCalls(env.chat.Id); len(singles) != 0 {
			t.Fatalf("single deleteMessage calls = %d, want none", len(singles))
		}
		if total != busyBurstSize/100 {
			t.Fatalf("total calls = %d, want %d", total, busyBurstSize/100)
		}
		// One call per 100 ms slot of the real pacer, less 1 ms of the slot's own rounding.
		if floor := time.Duration(busyBurstSize/100-1)*100*time.Millisecond - 50*time.Millisecond; elapsed < floor {
			t.Fatalf("30 paced calls took %s, want at least %s: the pacer did not pace", elapsed, floor)
		}
		if elapsed >= deletionCycleGuard {
			t.Fatalf("the burst took %s, want under the cycle guard %s", elapsed, deletionCycleGuard)
		}
		if rows := env.queuedNoWait(env.chat.Id); len(rows) != 0 {
			t.Fatalf("rows after the deletes = %d, want none", len(rows))
		}
	})

	t.Run("replies are not slowed by a blocked deletion pacer", func(t *testing.T) {
		env := newSelfDestructEnv(t, busyRuleDelay)
		env.withRealDeletionPacer(t)
		name := env.registerReplyCommand()
		other := busyOtherChat(t)

		// A block as a 429 sets it: the key holds 1 and lives for the retry_after.
		if err := env.redis.Set(deletionPaceKeys[1], "1"); err != nil {
			t.Fatalf("set the block: %v", err)
		}
		env.redis.SetTTL(deletionPaceKeys[1], 30*time.Second)
		before := env.paceKeyState()
		if len(before) != 1 {
			t.Fatalf("pacer keys before = %v, want only the block", before)
		}

		busySendCommand(env, name, other)
		if got := env.repliesTo(other.Id); got != 1 {
			t.Fatalf("replies in the other group = %d, want 1 while the deletion pacer is blocked", got)
		}
		after := env.paceKeyState()
		if len(after) != len(before) || after[deletionPaceKeys[1]] != before[deletionPaceKeys[1]] {
			t.Fatalf("pacer keys after a reply = %v, want %v: a reply never touches the deletion budget", after, before)
		}
	})

	t.Run("a 429 delays only deletions", func(t *testing.T) {
		env := newSelfDestructEnv(t, busyRuleDelay)
		env.withRealDeletionPacer(t)
		setNow := selfDestructClock(t, env.now)
		name := env.registerReplyCommand()
		other := busyOtherChat(t)
		member := env.member()
		const queued = 150
		for range queued {
			env.sendMember(&member, func(m *gotgbot.Message) { m.Text = "wait" })
		}
		if got := memberDeleteTestFlush(t); got != queued {
			t.Fatalf("memberDeleteFlush() = %d, want %d", got, queued)
		}
		setNow(env.now.Add((busyRuleDelay + 1) * time.Second))
		// 61 s is over the pacer's 60 s cap, so the call fails at once as rate limited and the block
		// is recorded.
		env.script.set(func(_ context.Context, method string, _ map[string]any) error {
			if method == "deleteMessages" || method == "deleteMessage" {
				return staffFake429(61)
			}
			return nil
		})
		if env.cycle() {
			t.Fatal("a rate limited cycle reported progress")
		}
		if n := len(env.fake.callsFor("deleteMessages")); n != 1 {
			t.Fatalf("deleteMessages requests = %d, want 1: the cycle ends at the first rate limit", n)
		}
		rows := env.selfDestructRows(env.chat.Id)
		if len(rows) != queued {
			t.Fatalf("rows after the 429 = %d, want all %d still queued", len(rows), queued)
		}
		for _, row := range rows {
			if row.State != models.ScheduledDeletionPending || row.Attempts != 0 {
				t.Fatalf("row %+v after the 429, want pending with no attempt counted", row)
			}
		}
		if !env.redis.Exists(deletionPaceKeys[1]) {
			t.Fatal("the 429 recorded no block")
		}

		busySendCommand(env, name, other)
		if got := env.repliesTo(other.Id); got != 1 {
			t.Fatalf("replies in the other group = %d, want 1 under the deletion block", got)
		}
		env.cycle()
		if n := len(env.fake.callsFor("deleteMessages")); n != 1 {
			t.Fatalf("deleteMessages requests after the next cycle = %d, want still 1 (no request under the block)", n)
		}
		if rows := env.selfDestructRows(env.chat.Id); len(rows) != queued {
			t.Fatalf("rows after the second cycle = %d, want %d", len(rows), queued)
		}
	})

	t.Run("the ceiling sheds the excess and those messages are never deleted", func(t *testing.T) {
		env := newSelfDestructEnv(t, busyRuleDelay)
		setNow := selfDestructClock(t, env.now)
		const ceiling, sent = 50, 80
		mreqSwap(t, &memberSelfDestructCeiling, ceiling)
		member := env.member()
		ids := make([]int64, 0, sent)
		for range sent {
			ids = append(ids, env.sendMember(&member, func(m *gotgbot.Message) { m.Text = "many" }))
		}
		if got := memberDeleteTestFlush(t); got != ceiling {
			t.Fatalf("memberDeleteFlush() = %d, want %d written", got, ceiling)
		}
		if got := memberDeleteShedTotal(memberDeleteShedCeiling); got != sent-ceiling {
			t.Fatalf("shed over the ceiling = %d, want %d", got, sent-ceiling)
		}
		rows := env.selfDestructRows(env.chat.Id)
		if len(rows) != ceiling {
			t.Fatalf("rows = %d, want %d", len(rows), ceiling)
		}
		queued := map[int64]bool{}
		for _, row := range rows {
			queued[row.MessageID] = true
		}

		setNow(env.now.Add((busyRuleDelay + 1) * time.Second))
		env.pass()
		deleted := map[int64]bool{}
		for _, batch := range env.batchCalls(env.chat.Id) {
			for _, id := range batch {
				deleted[id] = true
			}
		}
		for _, id := range ids {
			if queued[id] != deleted[id] {
				t.Fatalf("message %d: queued = %v, deleted = %v, want a message deleted exactly when it was queued", id, queued[id], deleted[id])
			}
		}
		if len(deleted) != ceiling {
			t.Fatalf("deleted %d messages, want %d", len(deleted), ceiling)
		}
	})
}
