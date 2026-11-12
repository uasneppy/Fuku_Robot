//go:build testtools

package modules

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	logrustest "github.com/sirupsen/logrus/hooks/test"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/deletion"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// TestDeletionWorkerBatches is the tracer of plan 16-04: the real worker claims due rows, deletes
// them in batch calls of at most 100 IDs through the pacer and removes them, and leaves rows that
// are not due yet alone.
func TestDeletionWorkerBatches(t *testing.T) {
	env := newDeletionEnv(t)
	other := uniqueModuleChatID()
	deletionCleanup(t, other)

	// 250 due rows for the first chat, 3 later due rows for a second chat and 5 rows due in an
	// hour for the first chat.
	firstIDs := env.seedDue(env.chat.Id, 1000, 250, deletion.KindPinned)
	for i := int64(0); i < 3; i++ {
		seedDeletionRow(t, other, 2000+i, env.now.Add(-10*time.Minute+time.Duration(i)*time.Second),
			env.now.Add(-2*time.Hour), deletion.KindPinned)
	}
	for i := int64(0); i < 5; i++ {
		seedDeletionRow(t, env.chat.Id, 3000+i, env.now.Add(time.Hour), env.now.Add(-time.Minute), deletion.KindPinned)
	}

	env.pass()

	batches := env.batchCalls(env.chat.Id)
	if len(batches) != 3 || len(batches[0]) != 100 || len(batches[1]) != 100 || len(batches[2]) != 50 {
		sizes := make([]int, 0, len(batches))
		for _, batch := range batches {
			sizes = append(sizes, len(batch))
		}
		t.Fatalf("deleteMessages calls for the first chat have sizes %v, want [100 100 50]", sizes)
	}
	var sent []int64
	for _, batch := range batches {
		sent = append(sent, batch...)
	}
	for i, id := range sent {
		if id != firstIDs[i] {
			t.Fatalf("batch ID %d = %d, want %d (ascending by delete_at)", i, id, firstIDs[i])
		}
	}
	if second := env.batchCalls(other); len(second) != 1 || len(second[0]) != 3 {
		t.Fatalf("deleteMessages calls for the second chat = %v, want one call of 3 IDs", second)
	}
	if singles := env.singleCalls(env.chat.Id); len(singles) != 0 {
		t.Fatalf("deleteMessage (single) calls = %v, want none when every batch succeeded", singles)
	}

	rows := env.queuedNoWait(env.chat.Id)
	if len(rows) != 5 {
		t.Fatalf("rows left for the first chat = %d, want the 5 that are not due", len(rows))
	}
	for _, row := range rows {
		if row.State != models.ScheduledDeletionPending || row.ClaimToken != "" || row.Attempts != 0 {
			t.Fatalf("future row %d = %+v, want untouched pending", row.MessageID, row)
		}
	}
	if left := env.queuedNoWait(other); len(left) != 0 {
		t.Fatalf("rows left for the second chat = %d, want 0", len(left))
	}

	// A cycle that finds nothing due makes no Telegram call.
	before := len(env.fake.callsFor("deleteMessages"))
	if env.cycle() {
		t.Fatal("a cycle with nothing due reported progress")
	}
	if after := len(env.fake.callsFor("deleteMessages")); after != before {
		t.Fatalf("an idle cycle made %d deleteMessages calls, want 0", after-before)
	}
}

// deletionRefusal is the answer Telegram gives for one message it will not delete.
func deletionRefusal() error {
	return staffFakeError(400, "Bad Request: message can't be deleted for everyone")
}

// scriptDelete scripts the delete calls: batch is what every deleteMessages call answers (nil lets
// it through) and perID is what the single deleteMessage call of each message answers.
func (e *deletionEnv) scriptDelete(batch error, perID map[int64]error) {
	e.script.set(func(_ context.Context, method string, params map[string]any) error {
		switch method {
		case "deleteMessages":
			return batch
		case "deleteMessage":
			return perID[staffParamInt(params, "message_id")]
		}
		return nil
	})
}

// rowOf reads one queue row of chatID, or nil when it is gone.
func (e *deletionEnv) rowOf(chatID, messageID int64) *models.ScheduledDeletion {
	e.t.Helper()
	for _, row := range e.queuedNoWait(chatID) {
		if row.MessageID == messageID {
			return &row
		}
	}
	return nil
}

func deletionLatchValue(t *testing.T, env *deletionEnv, chatID int64) string {
	t.Helper()
	value, err := env.redis.Get(deletionLatchKey(chatID))
	if err != nil {
		return ""
	}
	return value
}

func TestDeletionWorkerFallsBackPerID(t *testing.T) {
	env := newDeletionEnv(t)
	hook := mreqShedHook(t)
	ids := env.seedDue(env.chat.Id, 41000, 5, deletion.KindBotWelcome)
	env.scriptDelete(deletionRefusal(), map[int64]error{ids[2]: deletionRefusal()})

	env.pass()

	if batches := env.batchCalls(env.chat.Id); len(batches) != 1 || len(batches[0]) != 5 {
		t.Fatalf("batch calls = %v, want one call of 5 IDs", batches)
	}
	singles := env.singleCalls(env.chat.Id)
	if len(singles) != 5 {
		t.Fatalf("single deleteMessage calls = %v, want one per ID", singles)
	}
	for i, id := range ids {
		if singles[i] != id {
			t.Fatalf("single call %d = %d, want %d (claim order)", i, singles[i], id)
		}
	}
	if left := env.queuedNoWait(env.chat.Id); len(left) != 0 {
		t.Fatalf("rows left = %+v, want none: the undeletable message is dropped quietly, the rest deleted", left)
	}
	deletionWantNoMessageIDs(t, hook, ids)
}

// deletionWantNoMessageIDs fails when any captured log entry names one of the message IDs or
// holds something shaped like a claim token.
func deletionWantNoMessageIDs(t *testing.T, hook *logrustest.Hook, ids []int64) {
	t.Helper()
	tokenShape := regexp.MustCompile(`\b[0-9a-f]{32}\b`)
	for _, entry := range hook.AllEntries() {
		text := entry.Message
		for key, value := range entry.Data {
			text += " " + key + "=" + fmt.Sprint(value)
		}
		if tokenShape.MatchString(text) {
			t.Errorf("log line holds a claim token: %q", text)
		}
		for _, id := range ids {
			if regexp.MustCompile(`\b` + strconv.FormatInt(id, 10) + `\b`).MatchString(text) {
				t.Errorf("log line at %s names message %d: %q", entry.Level, id, text)
			}
		}
	}
}

func TestDeletionWorkerClassifiesErrors(t *testing.T) {
	migrated := &gotgbot.TelegramError{
		Code:           400,
		Description:    "Bad Request: group chat was upgraded to a supergroup chat",
		ResponseParams: &gotgbot.ResponseParameters{MigrateToChatId: -100123456},
	}
	cases := []struct {
		name string
		err  error
		// verdict is what happens to the one message the error names.
		verdict string // "drop", "retry" or "gone"
	}{
		{"not found", staffFakeError(400, "Bad Request: message to delete not found"), "drop"},
		{"cannot be deleted", deletionRefusal(), "drop"},
		{"message id invalid", staffFakeError(400, "Bad Request: MESSAGE_ID_INVALID"), "drop"},
		{"forbidden", staffFakeError(403, "Forbidden: not enough rights"), "drop"},
		{"server error", staffFakeError(500, "Internal Server Error"), "retry"},
		{"transport error", errors.New("dial tcp: i/o timeout"), "retry"},
		{"bot kicked", staffFakeError(403, "Forbidden: bot was kicked from the supergroup chat"), "gone"},
		{"chat not found", staffFakeError(400, "Bad Request: chat not found"), "gone"},
		{"upgraded", migrated, "gone"},
	}
	for _, mode := range []string{"per ID", "batch first"} {
		for _, tc := range cases {
			t.Run(mode+": "+tc.name, func(t *testing.T) {
				env := newDeletionEnv(t)
				ids := env.seedDue(env.chat.Id, 42000, 3, deletion.KindBotWelcome)
				bad := ids[1]
				// A row due later in the same chat shows what a gone chat drops.
				seedDeletionRow(t, env.chat.Id, 42900, env.now.Add(time.Hour), env.now.Add(-time.Minute), deletion.KindBotWelcome)
				batch := error(staffFakeError(400, "Bad Request: MESSAGE_ID_INVALID"))
				if mode == "batch first" {
					batch = tc.err
				}
				env.scriptDelete(batch, map[int64]error{bad: tc.err})

				env.cycle()

				switch tc.verdict {
				case "drop":
					if left := env.queuedNoWait(env.chat.Id); len(left) != 1 || left[0].MessageID != 42900 {
						t.Fatalf("rows left = %+v, want only the row not due", left)
					}
				case "retry":
					row := env.rowOf(env.chat.Id, bad)
					if row == nil || row.State != models.ScheduledDeletionPending || row.ClaimToken != "" || row.Attempts != 1 {
						t.Fatalf("failed row = %+v, want pending with one counted attempt", row)
					}
					if env.rowOf(env.chat.Id, ids[0]) != nil || env.rowOf(env.chat.Id, ids[2]) != nil {
						t.Fatal("a deletable neighbour of the failing row was kept")
					}
					env.cycle()
					if row := env.rowOf(env.chat.Id, bad); row == nil || row.Attempts != 2 {
						t.Fatalf("after the second failure row = %+v, want attempts 2", row)
					}
					env.cycle()
					if row := env.rowOf(env.chat.Id, bad); row != nil {
						t.Fatalf("after the third counted failure row = %+v, want it dropped", row)
					}
					if row := env.rowOf(env.chat.Id, 42900); row == nil || row.Attempts != 0 {
						t.Fatalf("the row not due = %+v, want untouched", row)
					}
				case "gone":
					if left := env.queuedNoWait(env.chat.Id); len(left) != 0 {
						t.Fatalf("rows left = %+v, want every claimed and pending row of the gone chat dropped", left)
					}
					if got := deletionLatchValue(t, env, env.chat.Id); got != deletionScopeGone {
						t.Fatalf("latch = %q, want %q", got, deletionScopeGone)
					}
				}
			})
		}
	}
}

func TestDeletionWorkerRateLimitedFree(t *testing.T) {
	assertFree := func(t *testing.T, env *deletionEnv, chats ...int64) {
		t.Helper()
		total := 0
		for _, chatID := range chats {
			for _, row := range env.queuedNoWait(chatID) {
				total++
				if row.State != models.ScheduledDeletionPending || row.ClaimToken != "" || row.Attempts != 0 || row.ClaimedAt != nil {
					t.Fatalf("row %+v, want pending, no token, no attempt", row)
				}
			}
		}
		if total != 8 {
			t.Fatalf("rows left = %d, want all 8 released", total)
		}
	}

	t.Run("a 429 that outlasts the retries", func(t *testing.T) {
		env := newDeletionEnv(t)
		other := uniqueModuleChatID()
		deletionCleanup(t, other)
		env.seedDue(env.chat.Id, 43000, 5, deletion.KindBotWelcome)
		for i := int64(0); i < 3; i++ {
			seedDeletionRow(t, other, 43500+i, env.now.Add(-time.Minute), env.now.Add(-time.Hour), deletion.KindBotWelcome)
		}
		// 20000 units of 5 ms is over the pacer's 60 s cap: the call fails at once as rate limited.
		env.script.set(func(_ context.Context, method string, _ map[string]any) error {
			if method == "deleteMessages" || method == "deleteMessage" {
				return staffFake429(20000)
			}
			return nil
		})

		if env.cycle() {
			t.Fatal("a rate limited cycle reported progress")
		}
		calls := len(env.fake.callsFor("deleteMessages")) + len(env.fake.callsFor("deleteMessage"))
		if calls != 1 {
			t.Fatalf("Telegram requests = %d, want 1 (the cycle ends at the first rate limit)", calls)
		}
		assertFree(t, env, env.chat.Id, other)

		// The shared block stands: the next cycle's first call fails at once with no request.
		env.cycle()
		after := len(env.fake.callsFor("deleteMessages")) + len(env.fake.callsFor("deleteMessage"))
		if after != calls {
			t.Fatalf("Telegram requests after the next cycle = %d, want %d (no request under the block)", after, calls)
		}
		assertFree(t, env, env.chat.Id, other)
	})

	t.Run("a cancelled context", func(t *testing.T) {
		env := newDeletionEnv(t)
		other := uniqueModuleChatID()
		deletionCleanup(t, other)
		env.seedDue(env.chat.Id, 44000, 5, deletion.KindBotWelcome)
		for i := int64(0); i < 3; i++ {
			seedDeletionRow(t, other, 44500+i, env.now.Add(-time.Minute), env.now.Add(-time.Hour), deletion.KindBotWelcome)
		}
		started := make(chan struct{}, 8)
		env.script.set(func(ctx context.Context, method string, _ map[string]any) error {
			if method == "deleteMessages" {
				started <- struct{}{}
				<-ctx.Done()
				return ctx.Err()
			}
			return nil
		})
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan bool, 1)
		go func() { done <- runDeletionCycle(ctx, env.bot) }()
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			t.Fatal("the cycle never reached its first call")
		}
		cancel()
		select {
		case progress := <-done:
			if progress {
				t.Error("a cancelled cycle reported progress")
			}
		case <-time.After(5 * time.Second):
			t.Fatal("the cycle did not end after the cancel")
		}
		assertFree(t, env, env.chat.Id, other)
	})
}

func TestDeletionWorkerDropsOldRows(t *testing.T) {
	env := newDeletionEnv(t)
	hook := mreqShedHook(t)
	old, fresh := int64(45001), int64(45002)
	seedDeletionRow(t, env.chat.Id, old, env.now.Add(-time.Minute), env.now.Add(-48*time.Hour), deletion.KindBotWelcome)
	seedDeletionRow(t, env.chat.Id, fresh, env.now.Add(-time.Minute), env.now.Add(-46*time.Hour), deletion.KindBotWelcome)

	// A row scheduled at the cap deletes at sent_at + 24 h.
	capped := int64(45003)
	sentAt := env.now.Add(-24 * time.Hour)
	written, err := deletionSchedule(context.Background(), deletion.Entry{
		ChatID: env.chat.Id, MessageID: capped, Kind: deletion.KindBotRules, SentAt: sentAt, DelaySecs: deletion.MaxDelaySecs,
	})
	if err != nil || !written {
		t.Fatalf("schedule at the cap = %v, %v, want written", written, err)
	}
	if row := env.rowOf(env.chat.Id, capped); row == nil || !row.DeleteAt.Equal(sentAt.Add(24*time.Hour).UTC().Truncate(time.Second)) {
		t.Fatalf("capped row = %+v, want delete_at = sent_at + 24 h", row)
	}
	if _, err := deletionSchedule(context.Background(), deletion.Entry{
		ChatID: env.chat.Id, MessageID: 45004, Kind: deletion.KindBotRules, SentAt: sentAt, DelaySecs: deletion.MaxDelaySecs + 1,
	}); err == nil {
		t.Fatal("a delay over 24 hours was accepted")
	}

	env.pass()

	batches := env.batchCalls(env.chat.Id)
	if len(batches) != 1 || len(batches[0]) != 2 {
		t.Fatalf("batch calls = %v, want one call without the 48 hour old message", batches)
	}
	for _, id := range batches[0] {
		if id == old {
			t.Fatal("the 48 hour old message was sent to Telegram")
		}
	}
	if left := env.queuedNoWait(env.chat.Id); len(left) != 0 {
		t.Fatalf("rows left = %+v, want none: the old row is dropped, the others deleted", left)
	}
	deletionWantNoMessageIDs(t, hook, []int64{old, fresh, capped})
}

func TestDeletionWorkerSkipsStaffGroup(t *testing.T) {
	t.Run("a Staff Group is dropped with no call", func(t *testing.T) {
		env := newDeletionEnv(t)
		env.seedDue(env.chat.Id, 46000, 4, deletion.KindBotWelcome)
		env.makeStaffGroup(env.chat.Id)

		env.pass()

		if n := len(env.fake.callsFor("deleteMessages")) + len(env.fake.callsFor("deleteMessage")); n != 0 {
			t.Fatalf("Telegram requests in a Staff Group = %d, want 0", n)
		}
		if left := env.queuedNoWait(env.chat.Id); len(left) != 0 {
			t.Fatalf("rows left = %+v, want them dropped", left)
		}
	})

	t.Run("a failing Staff Group read releases the rows free", func(t *testing.T) {
		env := newDeletionEnv(t)
		env.seedDue(env.chat.Id, 47000, 4, deletion.KindBotWelcome)
		mreqSwap(t, &deletionStaffLookup, func(int64) (*models.StaffGroup, error) { return nil, errors.New("database is down") })

		if env.cycle() {
			t.Fatal("a cycle that could not read the Staff Group reported progress")
		}

		if n := len(env.fake.callsFor("deleteMessages")) + len(env.fake.callsFor("deleteMessage")); n != 0 {
			t.Fatalf("Telegram requests with an unreadable Staff Group = %d, want 0", n)
		}
		rows := env.queuedNoWait(env.chat.Id)
		if len(rows) != 4 {
			t.Fatalf("rows left = %d, want all 4", len(rows))
		}
		for _, row := range rows {
			if row.State != models.ScheduledDeletionPending || row.ClaimToken != "" || row.Attempts != 0 {
				t.Fatalf("row %+v, want released free", row)
			}
		}
	})
}

func TestDeletionWorkerLatchedChat(t *testing.T) {
	setup := func(t *testing.T, scope string) (*deletionEnv, []int64, []int64) {
		env := newDeletionEnv(t)
		service := env.seedDue(env.chat.Id, 48000, 3, deletion.KindPinned)
		bots := env.seedDue(env.chat.Id, 48100, 2, deletion.KindBotWelcome)
		deletionSetLatch(context.Background(), env.chat.Id, scope)
		return env, service, bots
	}

	t.Run("a gone latch drops every kind with no call", func(t *testing.T) {
		env, _, _ := setup(t, deletionScopeGone)
		env.pass()
		if n := len(env.fake.callsFor("deleteMessages")) + len(env.fake.callsFor("deleteMessage")); n != 0 {
			t.Fatalf("Telegram requests in a latched chat = %d, want 0", n)
		}
		if left := env.queuedNoWait(env.chat.Id); len(left) != 0 {
			t.Fatalf("rows left = %+v, want none", left)
		}
	})

	t.Run("a service latch drops service rows and deletes bot rows", func(t *testing.T) {
		env, _, bots := setup(t, deletionScopeService)
		env.pass()
		batches := env.batchCalls(env.chat.Id)
		if len(batches) != 1 || len(batches[0]) != len(bots) {
			t.Fatalf("batch calls = %v, want one call of the bot rows only", batches)
		}
		for i, id := range batches[0] {
			if id != bots[i] {
				t.Fatalf("batch = %v, want %v", batches[0], bots)
			}
		}
		if left := env.queuedNoWait(env.chat.Id); len(left) != 0 {
			t.Fatalf("rows left = %+v, want none", left)
		}
	})
}

func TestDeletionWorkerStaleClaimNotDoubleProcessed(t *testing.T) {
	env := newDeletionEnv(t)
	second := env.replica()
	ids := env.seedDue(env.chat.Id, 49000, 6, deletion.KindBotWelcome)
	mreqSwap(t, &deletionCycleGuard, 300*time.Millisecond)

	// The first replica's cycle hangs inside its call: its rows are held.
	started := make(chan struct{}, 4)
	env.script.set(func(ctx context.Context, method string, _ map[string]any) error {
		if method == "deleteMessages" {
			started <- struct{}{}
			<-ctx.Done()
			return ctx.Err()
		}
		return nil
	})
	done := make(chan struct{})
	go func() {
		defer close(done)
		env.cycle()
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("the first replica never reached its call")
	}

	// While the claim is fresh the second replica's stale release leaves it alone.
	if second.cycle() {
		t.Error("the second replica made progress on rows the first still holds")
	}
	if n := len(second.fake.callsFor("deleteMessages")); n != 0 {
		t.Fatalf("the second replica sent %d requests for rows the first holds, want 0", n)
	}

	// The guard cuts the hung call off and the first replica releases what it has not finished,
	// free of cost, before it returns.
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the first replica's cycle outlived its guard")
	}
	rows := env.queuedNoWait(env.chat.Id)
	if len(rows) != len(ids) {
		t.Fatalf("rows after the guard = %d, want %d released", len(rows), len(ids))
	}
	for _, row := range rows {
		if row.State != models.ScheduledDeletionPending || row.ClaimToken != "" || row.Attempts != 0 {
			t.Fatalf("row %+v, want pending, no token, no attempt", row)
		}
	}

	// Now the second replica takes them, once.
	second.pass()
	if got := second.batchCalls(env.chat.Id); len(got) != 1 || len(got[0]) != len(ids) {
		t.Fatalf("second replica's batches = %v, want one call of every ID", got)
	}
	if left := env.queuedNoWait(env.chat.Id); len(left) != 0 {
		t.Fatalf("rows left = %+v, want none", left)
	}

	// A late completion of a claim that a stale release handed on removes nothing.
	later := env.seedDue(env.chat.Id, 49100, 2, deletion.KindBotWelcome)
	firstToken, secondToken := deletion.NewClaimToken(), deletion.NewClaimToken()
	if rows, err := deletion.ClaimDue(context.Background(), env.now, 10, firstToken); err != nil || len(rows) != len(later) {
		t.Fatalf("first claim = %d rows, %v", len(rows), err)
	}
	if _, err := deletion.ReleaseStale(context.Background(), env.now.Add(time.Minute)); err != nil {
		t.Fatalf("stale release: %v", err)
	}
	held, err := deletion.ClaimDue(context.Background(), env.now, 10, secondToken)
	if err != nil || len(held) != len(later) {
		t.Fatalf("second claim = %d rows, %v", len(held), err)
	}
	if err := deletion.Complete(context.Background(), firstToken, deletionIDs(held)); err != nil {
		t.Fatalf("late completion: %v", err)
	}
	if rows := env.queuedNoWait(env.chat.Id); len(rows) != len(later) {
		t.Fatalf("rows after the late completion = %d, want %d: it must remove nothing the second claimer holds", len(rows), len(later))
	}
}

func TestDeletionWorkerLogsNoMessageIDs(t *testing.T) {
	env := newDeletionEnv(t)
	hook := mreqShedHook(t)
	var all []int64
	chats := make([]int64, 0, 8)
	for i := 0; i < 7; i++ {
		chats = append(chats, uniqueModuleChatID())
	}
	deletionCleanup(t, chats...)
	staffCleanup(t, chats...)
	seed := func(chatID, first int64, n int, kind string) []int64 {
		ids := env.seedDue(chatID, first, n, kind)
		all = append(all, ids...)
		return ids
	}
	for _, chatID := range chats {
		env.fake.setBotMember(chatID, `{"status":"member","user":{"id":999,"is_bot":true,"first_name":"Fuku"}}`)
	}

	seed(chats[0], 880000100, 3, deletion.KindBotWelcome)
	refused := seed(chats[1], 880000200, 3, deletion.KindBotWelcome)
	failing := seed(chats[2], 880000300, 3, deletion.KindBotWelcome)
	transport := seed(chats[3], 880000400, 3, deletion.KindBotWelcome)
	seed(chats[4], 880000500, 3, deletion.KindBotWelcome)
	seed(chats[5], 880000600, 3, deletion.KindPinned)
	seed(chats[6], 880000700, 3, deletion.KindBotWelcome)
	env.makeStaffGroup(chats[6])
	oldID := int64(880000800)
	seedDeletionRow(t, chats[0], oldID, env.now.Add(-time.Minute), env.now.Add(-60*time.Hour), deletion.KindBotWelcome)
	all = append(all, oldID)

	env.script.set(func(_ context.Context, method string, params map[string]any) error {
		chatID := staffParamInt(params, "chat_id")
		messageID := staffParamInt(params, "message_id")
		switch chatID {
		case chats[1]:
			if method == "deleteMessages" || messageID == refused[1] {
				return deletionRefusal()
			}
		case chats[2]:
			if method == "deleteMessages" || messageID == failing[1] {
				return staffFakeError(500, "Internal Server Error")
			}
		case chats[3]:
			if method == "deleteMessages" || messageID == transport[1] {
				return errors.New("dial tcp 10.0.0.1:443: i/o timeout")
			}
		case chats[4]:
			return staffFakeError(403, "Forbidden: bot was kicked from the supergroup chat")
		case chats[5]:
			if method == "deleteMessages" || method == "deleteMessage" {
				return deletionRefusal()
			}
		}
		return nil
	})

	env.pass()
	if len(hook.AllEntries()) == 0 {
		t.Fatal("the worker logged nothing at debug level: the test cannot discriminate")
	}
	deletionWantNoMessageIDs(t, hook, all)
}

// deletionDrainWake empties the worker's wake channel, so a test starts with no pending wake.
func deletionDrainWake() {
	for len(deletionWake) > 0 {
		<-deletionWake
	}
}

// deletionRowCount counts the queue rows of chatID, in any state.
func (e *deletionEnv) deletionRowCount(chatID int64) int {
	e.t.Helper()
	return len(e.queuedNoWait(chatID))
}

// deletionWorkerStopped registers the clean-up of a test that starts the real worker: the stop
// first (it cancels the root), then the fresh root every later test needs.
func deletionWorkerStopped(t *testing.T) {
	t.Helper()
	restoreDeletionWorker(t)
	t.Cleanup(StopDeletionWorker)
}

func TestDeletionWorkerResumesAfterRestart(t *testing.T) {
	env := newDeletionEnv(t)
	deletionWorkerStopped(t)
	deletionDrainWake()
	mreqSwap(t, &deletionWorkerTick, 10*time.Millisecond)
	ids := env.seedDue(env.chat.Id, 54000, 6, deletion.KindBotWelcome)

	started := make(chan struct{}, 4)
	env.script.set(func(ctx context.Context, method string, _ map[string]any) error {
		if method == "deleteMessages" {
			started <- struct{}{}
			<-ctx.Done()
			return ctx.Err()
		}
		return nil
	})
	StartDeletionWorker(env.bot)
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("the worker never reached its first call")
	}

	StopDeletionWorker()

	rows := env.queuedNoWait(env.chat.Id)
	if len(rows) != len(ids) {
		t.Fatalf("rows after the stop = %d, want all %d still queued", len(rows), len(ids))
	}
	for _, row := range rows {
		if row.State != models.ScheduledDeletionPending || row.ClaimToken != "" || row.Attempts != 0 {
			t.Fatalf("row %+v after the stop, want pending with no token and no attempt", row)
		}
	}

	// A crash leaves a row acting with an old claim: the first cycle of the next worker releases it.
	claimedAt := env.now.Add(-3 * time.Minute)
	crashed := models.ScheduledDeletion{
		ChatID: env.chat.Id, MessageID: 54900, Kind: deletion.KindBotWelcome,
		SentAt: env.now.Add(-time.Hour), DeleteAt: env.now.Add(-time.Minute),
		State: models.ScheduledDeletionActing, ClaimToken: "deadbeefdeadbeefdeadbeefdeadbeef",
		ClaimedAt: &claimedAt, CreatedAt: env.now.Add(-time.Hour),
	}
	if err := db.DB.Create(&crashed).Error; err != nil {
		t.Fatalf("seed the crashed row: %v", err)
	}

	// The call the stop cut off is recorded too: only what the restarted worker sends counts.
	cutOff := len(env.batchCalls(env.chat.Id))
	freshDeletionRoot()
	env.script.set(nil)
	StartDeletionWorker(env.bot)
	waitUntil(t, 10*time.Second, "the restarted worker to delete every row", func() bool {
		return env.deletionRowCount(env.chat.Id) == 0
	})
	StopDeletionWorker()

	seen := make(map[int64]int)
	for _, batch := range env.batchCalls(env.chat.Id)[cutOff:] {
		for _, id := range batch {
			seen[id]++
		}
	}
	for _, id := range append(ids, crashed.MessageID) {
		if seen[id] != 1 {
			t.Errorf("message %d was sent %d times after the restart, want exactly once", id, seen[id])
		}
	}
}

func TestDeletionWorkerOneDeleteAcrossReplicas(t *testing.T) {
	env := newDeletionEnv(t)
	second := env.replica()
	// Small claims and chunks make the two replicas share the rows instead of one taking all.
	mreqSwap(t, &deletionBatch, 7)
	mreqSwap(t, &deletionChunk, 5)
	ids := env.seedDue(env.chat.Id, 55000, 40, deletion.KindBotWelcome)

	run := func(replica *deletionEnv, start <-chan struct{}, wg *sync.WaitGroup) {
		defer wg.Done()
		<-start
		deadline := time.Now().Add(30 * time.Second)
		// A cycle that met a locked SQLite database claims nothing and is simply run again.
		for time.Now().Before(deadline) && replica.deletionRowCount(env.chat.Id) > 0 {
			replica.cycle()
		}
	}
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	go run(env, start, &wg)
	go run(second, start, &wg)
	close(start)
	wg.Wait()

	if left := env.queuedNoWait(env.chat.Id); len(left) != 0 {
		t.Fatalf("rows left = %d, want none", len(left))
	}
	seen := make(map[int64]int)
	for _, replica := range []*deletionEnv{env, second} {
		for _, batch := range replica.batchCalls(env.chat.Id) {
			for _, id := range batch {
				seen[id]++
			}
		}
	}
	for _, id := range ids {
		if seen[id] != 1 {
			t.Errorf("message %d was deleted %d times across the two replicas, want exactly once", id, seen[id])
		}
	}
	if len(seen) != len(ids) {
		t.Errorf("distinct messages deleted = %d, want %d", len(seen), len(ids))
	}
}

func TestDeletionWorkerStartStopIdempotent(t *testing.T) {
	env := newDeletionEnv(t)
	deletionWorkerStopped(t)

	// A stop with nothing running returns at once and cancels the root, so every later inline
	// delete takes the durable path.
	before := time.Now()
	StopDeletionWorker()
	StopDeletionWorker()
	if elapsed := time.Since(before); elapsed > time.Second {
		t.Fatalf("a stop with nothing running took %s, want it to return at once", elapsed)
	}
	if deletionContext().Err() == nil {
		t.Fatal("the stop left the root context alive")
	}

	freshDeletionRoot()
	StartDeletionWorker(env.bot)
	deletionWorkerMu.Lock()
	first := deletionWorkerDone
	deletionWorkerMu.Unlock()
	StartDeletionWorker(env.bot)
	deletionWorkerMu.Lock()
	again := deletionWorkerDone
	deletionWorkerMu.Unlock()
	if first == nil || first != again {
		t.Fatal("a second StartDeletionWorker started another loop")
	}
	StopDeletionWorker()
	StopDeletionWorker()
	deletionWorkerMu.Lock()
	defer deletionWorkerMu.Unlock()
	if deletionWorkerCancel != nil || deletionWorkerDone != nil {
		t.Fatal("the worker state was not cleared by the stop")
	}
}
