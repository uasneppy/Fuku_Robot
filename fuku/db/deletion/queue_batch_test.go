//go:build testtools

package deletion

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// batchSent is the fixed message date of the batch tests.
var batchSent = time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)

// selfDestructEntry is a self-destruct entry for the chat and message with one of the six delays.
func selfDestructEntry(chatID, messageID int64, delay int) Entry {
	return Entry{ChatID: chatID, MessageID: messageID, Kind: KindMemberSelfDestruct, SentAt: batchSent, DelaySecs: delay}
}

// deleteAtOf reads the delete_at of every queue row of a chat by message ID.
func deleteAtOf(t *testing.T, chatID int64) map[int64]time.Time {
	t.Helper()
	out := map[int64]time.Time{}
	for _, row := range storedQueue(t, chatID) {
		out[row.MessageID] = row.DeleteAt
	}
	return out
}

// TestScheduleBatchKeepsEarliest runs on SQLite in make test and on PostgreSQL in the integrity
// target: a batch writes one row per entry, a later batch never moves a time later, and the kind
// and sent time of a first insert are never rewritten.
func TestScheduleBatchKeepsEarliest(t *testing.T) {
	ctx := context.Background()

	t.Run("a batch writes pending rows at sent plus delay", func(t *testing.T) {
		chatID := deletionChat(t)
		wrote, err := ScheduleBatch(ctx, []Entry{
			selfDestructEntry(chatID, 1, 60), selfDestructEntry(chatID, 2, 600), selfDestructEntry(chatID, 3, 86400),
		})
		if err != nil || wrote != 3 {
			t.Fatalf("ScheduleBatch = (%d, %v), want (3, nil)", wrote, err)
		}
		rows := storedQueue(t, chatID)
		if len(rows) != 3 {
			t.Fatalf("%d rows, want 3", len(rows))
		}
		for i, delay := range []int{60, 600, 86400} {
			row := rows[i]
			if row.State != models.ScheduledDeletionPending || row.ClaimToken != "" || row.Attempts != 0 || row.ClaimedAt != nil {
				t.Errorf("row %d = %+v, want pending with no claim", row.MessageID, row)
			}
			if row.Kind != KindMemberSelfDestruct || !row.SentAt.Equal(batchSent) ||
				!row.DeleteAt.Equal(batchSent.Add(time.Duration(delay)*time.Second)) {
				t.Errorf("row %d = kind %q sent %v delete %v, want self-destruct at sent + %ds", row.MessageID, row.Kind, row.SentAt, row.DeleteAt, delay)
			}
		}
	})

	t.Run("a later time stays and an earlier one wins", func(t *testing.T) {
		chatID := deletionChat(t)
		if _, err := ScheduleBatch(ctx, []Entry{selfDestructEntry(chatID, 1, 600), selfDestructEntry(chatID, 2, 600)}); err != nil {
			t.Fatal(err)
		}
		if _, err := ScheduleBatch(ctx, []Entry{selfDestructEntry(chatID, 1, 3600), selfDestructEntry(chatID, 2, 60), selfDestructEntry(chatID, 3, 300)}); err != nil {
			t.Fatal(err)
		}
		got := deleteAtOf(t, chatID)
		want := map[int64]time.Time{
			1: batchSent.Add(600 * time.Second), 2: batchSent.Add(60 * time.Second), 3: batchSent.Add(300 * time.Second),
		}
		for id, at := range want {
			if !got[id].Equal(at) {
				t.Errorf("message %d delete_at = %v, want %v", id, got[id], at)
			}
		}
	})

	t.Run("kind and sent time of the first insert stay", func(t *testing.T) {
		chatID := deletionChat(t)
		if _, err := Schedule(ctx, Entry{ChatID: chatID, MessageID: 7, Kind: KindBotWelcome, SentAt: batchSent, DelaySecs: 600}); err != nil {
			t.Fatal(err)
		}
		later := selfDestructEntry(chatID, 7, 60)
		later.SentAt = batchSent.Add(time.Hour)
		if _, err := ScheduleBatch(ctx, []Entry{later}); err != nil {
			t.Fatal(err)
		}
		row := storedQueue(t, chatID)[0]
		if row.Kind != KindBotWelcome || !row.SentAt.Equal(batchSent) || !row.DeleteAt.Equal(batchSent.Add(10*time.Minute)) {
			t.Errorf("row = kind %q sent %v delete %v, want the first insert's welcome, sent and sent + 600s", row.Kind, row.SentAt, row.DeleteAt)
		}
	})

	t.Run("concurrent callers over overlapping keys keep the minimum", func(t *testing.T) {
		chatID := deletionChat(t)
		presets := []int{60, 300, 600, 3600, 21600, 86400}
		const callers, keys = 8, 12
		want := map[int64]int{}
		batches := make([][]Entry, callers)
		for c := range callers {
			// Every caller names 8 of the 12 keys, starting at its own offset, so the key sets overlap.
			for k := range 8 {
				id := int64((c+k)%keys + 1)
				delay := presets[(c*3+k*5)%len(presets)]
				batches[c] = append(batches[c], selfDestructEntry(chatID, id, delay))
				if best, ok := want[id]; !ok || delay < best {
					want[id] = delay
				}
			}
		}
		var wg sync.WaitGroup
		errs := make(chan error, callers)
		start := make(chan struct{})
		for _, batch := range batches {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				_, err := ScheduleBatch(ctx, batch)
				errs <- err
			}()
		}
		close(start)
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatalf("a concurrent ScheduleBatch failed: %v", err)
			}
		}
		got := deleteAtOf(t, chatID)
		if len(got) != len(want) {
			t.Fatalf("%d rows, want %d (one per key)", len(got), len(want))
		}
		for id, delay := range want {
			if !got[id].Equal(batchSent.Add(time.Duration(delay) * time.Second)) {
				t.Errorf("message %d delete_at = %v, want sent + %ds", id, got[id], delay)
			}
		}
	})
}

// TestScheduleBatchDuplicateKey runs on SQLite in make test and on PostgreSQL in the integrity
// target: one call that names the same (chat, message) several times writes one row at the
// earliest time. One PostgreSQL statement cannot touch a key twice, so this proves the Go dedupe.
func TestScheduleBatchDuplicateKey(t *testing.T) {
	ctx := context.Background()
	chatID := deletionChat(t)
	wrote, err := ScheduleBatch(ctx, []Entry{
		selfDestructEntry(chatID, 5, 3600), selfDestructEntry(chatID, 6, 300),
		selfDestructEntry(chatID, 5, 60), selfDestructEntry(chatID, 5, 600),
	})
	if err != nil {
		t.Fatalf("ScheduleBatch with a repeated key: %v", err)
	}
	if wrote != 2 {
		t.Errorf("wrote %d, want 2 (one per distinct key)", wrote)
	}
	got := deleteAtOf(t, chatID)
	if len(got) != 2 || !got[5].Equal(batchSent.Add(time.Minute)) || !got[6].Equal(batchSent.Add(5*time.Minute)) {
		t.Errorf("rows = %v, want message 5 at sent + 60s and message 6 at sent + 300s", got)
	}
}

// insertStatementCounter counts the multi-row INSERTs into scheduled_deletions and the rows each
// carried, through a GORM create callback removed when the test ends.
func insertStatementCounter(t *testing.T) func() []int {
	t.Helper()
	var (
		mu    sync.Mutex
		sizes []int
	)
	name := "test:count_scheduled_deletion_inserts"
	err := db.DB.Callback().Create().Before("gorm:create").Register(name, func(tx *gorm.DB) {
		if tx.Statement.Table != "scheduled_deletions" {
			return
		}
		size := 1
		if tx.Statement.ReflectValue.Kind() == reflect.Slice {
			size = tx.Statement.ReflectValue.Len()
		}
		mu.Lock()
		sizes = append(sizes, size)
		mu.Unlock()
	})
	if err != nil {
		t.Fatalf("register the insert counter: %v", err)
	}
	t.Cleanup(func() {
		if err := db.DB.Callback().Create().Remove(name); err != nil {
			t.Errorf("remove the insert counter: %v", err)
		}
	})
	return func() []int {
		mu.Lock()
		defer mu.Unlock()
		return slices.Clone(sizes)
	}
}

func TestScheduleBatchValidates(t *testing.T) {
	ctx := context.Background()

	t.Run("one invalid entry refuses the whole call before any write", func(t *testing.T) {
		chatID := deletionChat(t)
		good := selfDestructEntry(chatID, 1, 60)
		cases := []struct {
			name string
			bad  Entry
			want error
		}{
			{"unknown kind", Entry{ChatID: chatID, MessageID: 2, Kind: "member:nope", SentAt: batchSent, DelaySecs: 60}, ErrUnknownKind},
			{"a delay the kind refuses", selfDestructEntry(chatID, 3, 45), ErrDelayOutOfRange},
			{"an edit entry that waits", Entry{ChatID: chatID, MessageID: 4, Kind: KindMemberEdited, SentAt: batchSent, DelaySecs: 60}, ErrDelayOutOfRange},
			{"a private chat", selfDestructEntry(123456789, 5, 60), ErrInvalidEntry},
			{"a zero chat", selfDestructEntry(0, 6, 60), ErrInvalidEntry},
			{"a zero message", selfDestructEntry(chatID, 0, 60), ErrInvalidEntry},
			{"a negative message", selfDestructEntry(chatID, -3, 60), ErrInvalidEntry},
			{"no sent time", Entry{ChatID: chatID, MessageID: 7, Kind: KindMemberSelfDestruct, DelaySecs: 60}, ErrInvalidEntry},
		}
		reads := 0
		previous := staffGroupLookup
		staffGroupLookup = func(id int64) (*models.StaffGroup, error) { reads++; return previous(id) }
		t.Cleanup(func() { staffGroupLookup = previous })
		for _, tc := range cases {
			wrote, err := ScheduleBatch(ctx, []Entry{good, tc.bad})
			if wrote != 0 || !errors.Is(err, tc.want) {
				t.Errorf("%s: ScheduleBatch = (%d, %v), want (0, %v)", tc.name, wrote, err, tc.want)
			}
		}
		if reads != 0 {
			t.Errorf("a refused batch made %d Staff Group reads, want 0 (validation comes first)", reads)
		}
		if n := len(storedQueue(t, chatID)); n != 0 {
			t.Fatalf("%d rows exist after refused batches, want 0", n)
		}
	})

	t.Run("an empty batch writes nothing", func(t *testing.T) {
		for _, entries := range [][]Entry{nil, {}} {
			if wrote, err := ScheduleBatch(ctx, entries); wrote != 0 || err != nil {
				t.Errorf("ScheduleBatch(%v) = (%d, %v), want (0, nil)", entries, wrote, err)
			}
		}
	})

	t.Run("250 entries are written in statements of at most 100 rows", func(t *testing.T) {
		chatID := deletionChat(t)
		sizes := insertStatementCounter(t)
		entries := make([]Entry, 0, 250)
		for i := range 250 {
			entries = append(entries, selfDestructEntry(chatID, int64(i+1), 3600))
		}
		wrote, err := ScheduleBatch(ctx, entries)
		if err != nil || wrote != 250 {
			t.Fatalf("ScheduleBatch = (%d, %v), want (250, nil)", wrote, err)
		}
		if got := sizes(); !slices.Equal(got, []int{100, 100, 50}) {
			t.Errorf("insert statements carried %v rows, want [100 100 50]", got)
		}
		if n := len(storedQueue(t, chatID)); n != 250 {
			t.Errorf("%d rows stored, want 250", n)
		}
		if ScheduleBatchMax != 100 {
			t.Errorf("ScheduleBatchMax = %d, want 100", ScheduleBatchMax)
		}
	})
}

func TestScheduleBatchRefusesStaffGroup(t *testing.T) {
	ctx := context.Background()

	t.Run("a Staff Group chat gets no row and an ordinary chat is written", func(t *testing.T) {
		staffChat, ordinary := deletionChat(t), deletionChat(t)
		if err := db.DB.Create(&models.StaffGroup{ChatID: staffChat, OwnerUserID: 1, Title: "staff"}).Error; err != nil {
			t.Fatal(err)
		}
		wrote, err := ScheduleBatch(ctx, []Entry{
			selfDestructEntry(staffChat, 1, 60), selfDestructEntry(ordinary, 1, 60),
			selfDestructEntry(staffChat, 2, 60), selfDestructEntry(ordinary, 2, 60),
		})
		if err != nil || wrote != 2 {
			t.Fatalf("ScheduleBatch = (%d, %v), want (2, nil)", wrote, err)
		}
		if n := len(storedQueue(t, staffChat)); n != 0 {
			t.Errorf("%d rows written for a Staff Group", n)
		}
		if n := len(storedQueue(t, ordinary)); n != 2 {
			t.Errorf("%d rows for the ordinary chat, want 2", n)
		}
	})

	t.Run("one read per distinct chat", func(t *testing.T) {
		a, b := deletionChat(t), deletionChat(t)
		reads := map[int64]int{}
		previous := staffGroupLookup
		staffGroupLookup = func(id int64) (*models.StaffGroup, error) { reads[id]++; return previous(id) }
		t.Cleanup(func() { staffGroupLookup = previous })
		entries := []Entry{}
		for i := range 6 {
			entries = append(entries, selfDestructEntry(a, int64(i+1), 60), selfDestructEntry(b, int64(i+1), 60))
		}
		if _, err := ScheduleBatch(ctx, entries); err != nil {
			t.Fatal(err)
		}
		if reads[a] != 1 || reads[b] != 1 || len(reads) != 2 {
			t.Errorf("Staff Group reads = %v, want exactly one per distinct chat", reads)
		}
	})

	t.Run("a failed read drops that chat and writes the others", func(t *testing.T) {
		failed, ordinary := deletionChat(t), deletionChat(t)
		boom := errors.New("staff read failed")
		previous := staffGroupLookup
		staffGroupLookup = func(id int64) (*models.StaffGroup, error) {
			if id == failed {
				return nil, boom
			}
			return previous(id)
		}
		t.Cleanup(func() { staffGroupLookup = previous })
		wrote, err := ScheduleBatch(ctx, []Entry{selfDestructEntry(failed, 1, 60), selfDestructEntry(ordinary, 1, 60)})
		if wrote != 1 || !errors.Is(err, boom) || !strings.Contains(err.Error(), strconv.FormatInt(failed, 10)) {
			t.Fatalf("ScheduleBatch = (%d, %v), want (1, an error naming chat %d and wrapping the lookup error)", wrote, err, failed)
		}
		if n := len(storedQueue(t, failed)); n != 0 {
			t.Errorf("%d rows written for the chat whose read failed", n)
		}
		if n := len(storedQueue(t, ordinary)); n != 1 {
			t.Errorf("%d rows for the other chat, want 1", n)
		}
	})
}

// seedKind inserts count pending rows of a chat and kind, and acting extra rows, directly.
func seedKind(t *testing.T, chatID int64, kind string, firstMessage int64, pending, acting int) {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Second)
	for i := range pending + acting {
		row := models.ScheduledDeletion{
			ChatID: chatID, MessageID: firstMessage + int64(i), Kind: kind,
			SentAt: now, DeleteAt: now.Add(time.Hour), State: models.ScheduledDeletionPending, CreatedAt: now,
		}
		if i >= pending {
			row.State, row.ClaimToken, row.ClaimedAt = models.ScheduledDeletionActing, "tok", &now
		}
		if err := db.DB.Create(&row).Error; err != nil {
			t.Fatalf("seed row %d: %v", i, err)
		}
	}
}

// TestCountPending runs on SQLite in make test and on PostgreSQL in the integrity target.
func TestCountPending(t *testing.T) {
	ctx := context.Background()
	count := func(chatID int64, kind string, limit int) int64 {
		t.Helper()
		n, err := CountPending(ctx, chatID, kind, limit)
		if err != nil {
			t.Fatalf("CountPending(%d): %v", limit, err)
		}
		return n
	}

	chatID, other := deletionChat(t), deletionChat(t)
	if n := count(chatID, KindMemberSelfDestruct, 100); n != 0 {
		t.Errorf("an empty chat counts %d, want 0", n)
	}
	seedKind(t, chatID, KindMemberSelfDestruct, 1, 5, 2)
	seedKind(t, chatID, KindMemberEdited, 100, 4, 0)
	seedKind(t, other, KindMemberSelfDestruct, 1, 9, 0)
	if n := count(chatID, KindMemberSelfDestruct, 100); n != 7 {
		t.Errorf("count = %d, want 7 (pending and acting, this chat and kind only)", n)
	}
	if n := count(chatID, KindMemberEdited, 100); n != 4 {
		t.Errorf("edited count = %d, want 4", n)
	}
	if n := count(chatID, KindMemberSelfDestruct, 7); n != 7 {
		t.Errorf("count at limit 7 = %d, want 7", n)
	}
	if n := count(chatID, KindMemberSelfDestruct, 3); n != 3 {
		t.Errorf("count at limit 3 = %d, want the limit 3", n)
	}
	for _, limit := range []int{0, -5} {
		if n := count(chatID, KindMemberSelfDestruct, limit); n != 0 {
			t.Errorf("count at limit %d = %d, want 0 with no query", limit, n)
		}
	}
}

// TestDropPendingMessage runs on SQLite in make test and on PostgreSQL in the integrity target.
func TestDropPendingMessage(t *testing.T) {
	ctx := context.Background()
	chatID, other := deletionChat(t), deletionChat(t)
	seedKind(t, chatID, KindMemberSelfDestruct, 10, 1, 0) // message 10, pending
	seedKind(t, chatID, KindMemberSelfDestruct, 11, 0, 1) // message 11, acting
	seedKind(t, chatID, KindMemberEdited, 12, 1, 0)       // message 12, another kind
	seedKind(t, other, KindMemberSelfDestruct, 10, 1, 0)  // the same message ID in another chat

	drop := func(chat, message int64, kind string) int64 {
		t.Helper()
		n, err := DropPendingMessage(ctx, chat, message, kind)
		if err != nil {
			t.Fatalf("DropPendingMessage(%d, %d, %s): %v", chat, message, kind, err)
		}
		return n
	}
	if n := drop(chatID, 11, KindMemberSelfDestruct); n != 0 {
		t.Errorf("dropping an acting row returned %d, want 0", n)
	}
	if n := drop(chatID, 12, KindMemberSelfDestruct); n != 0 {
		t.Errorf("dropping with another kind returned %d, want 0", n)
	}
	if n := drop(chatID, 99, KindMemberSelfDestruct); n != 0 {
		t.Errorf("dropping an absent row returned %d, want 0", n)
	}
	if n := drop(chatID, 10, KindMemberSelfDestruct); n != 1 {
		t.Errorf("dropping the pending row returned %d, want 1", n)
	}
	if n := drop(chatID, 10, KindMemberSelfDestruct); n != 0 {
		t.Errorf("dropping it again returned %d, want 0", n)
	}
	left := map[int64]bool{}
	for _, row := range storedQueue(t, chatID) {
		left[row.MessageID] = true
	}
	if left[10] || !left[11] || !left[12] {
		t.Errorf("rows left = %v, want the acting row 11 and the other kind's row 12", left)
	}
	if n := len(storedQueue(t, other)); n != 1 {
		t.Errorf("another chat has %d rows, want its 1 untouched", n)
	}
}
