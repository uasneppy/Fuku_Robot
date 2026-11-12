package modules

import (
	"cmp"
	"context"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/uasneppy/fuku_robot/fuku/db/deletion"
	"github.com/uasneppy/fuku_robot/fuku/utils/error_handling"
)

// The member self-destruct batcher (Phase 17, D-07, D-08, D-09, D-27). The self-destruct watcher
// of plan 17-07 sees every message of a group whose member:selfdestruct rule is on, so a busy group
// is hundreds of messages a second. Writing one queue row per message from the dispatcher goroutine
// would make the database the bottleneck of the dispatcher, so the watcher only offers an entry
// here and this file turns the offers into batched queue writes on its own goroutine.
//
// The contract:
//   - memberDeleteOffer never blocks and never touches the database: one non-blocking send into a
//     buffered channel (memberDeleteQueueSize entries). A full channel, a draining process and a
//     wrong kind refuse the entry, and every refusal keeps the message (a shed).
//   - The batcher goroutine flushes every memberDeleteFlushEvery, or at once when the buffer holds
//     memberDeleteFlushAt entries. A flush takes what is buffered, merges entries of one message to
//     the earliest time, and per chat (ascending) re-reads the rules once (D-26a: a rule that was
//     switched Off since the offer drops the chat's entries), counts the chat's queued self-destruct
//     rows exactly up to memberSelfDestructCeiling and sheds what would pass it (D-09, D-31), then
//     writes every kept entry through deletion.ScheduleBatch, which writes statements of at most 100
//     rows and refuses a Staff Group (D-17).
//   - Unknown never deletes: an unreadable rule, an unreadable count, a failed write and a failed
//     Staff Group read all shed, and a shed message is simply kept (D-33). Each shed is counted by
//     reason and logged at most once per chat per hour with the chat ID and the reason only, never a
//     message ID (D-31).
//   - It joins StopDeletionWorker's existing wait: it adds no shutdown drain and no allowance. The
//     stop sets the draining flag, asks for one final flush bounded to memberDeleteFinalFlushWait on a
//     context detached from the cancelled deletion root, and the worker and the batcher are waited
//     for under one timer. A flush cut off loses only entries that were never queued, whose
//     messages stay (D-08, D-19).
//   - The batcher makes database writes only. It never calls Telegram, never deletes a message and
//     never calls deletionAct or deletionSchedule (D-27).
//
// The per-chat ceiling and what it means for each delay is in AGENTS.md (Subsystem traps, "Member
// deletion, the self-destruct batcher").

const (
	// memberDeleteQueueSize is how many entries the buffer holds.
	memberDeleteQueueSize = 4096
	// memberDeleteWarnEvery is how long a chat's shed log stays quiet after a line.
	memberDeleteWarnEvery = time.Hour
)

// Package variables, so a test can shrink them.
var (
	// memberDeleteQueue is the bounded buffer the watcher offers to.
	memberDeleteQueue = make(chan deletion.Entry, memberDeleteQueueSize)
	// memberDeleteKick asks the batcher for a flush at once; it holds one pending request.
	memberDeleteKick = make(chan struct{}, 1)
	// memberDeleteFlushEvery is the flush period.
	memberDeleteFlushEvery = 300 * time.Millisecond
	// memberDeleteFlushAt is the buffered count that asks for a flush before the period ends.
	memberDeleteFlushAt = 100
	// memberSelfDestructCeiling is the most self-destruct rows one chat may have queued.
	memberSelfDestructCeiling = 20000
	// memberDeleteFinalFlushWait bounds the one final flush a stop asks for.
	memberDeleteFinalFlushWait = 2 * time.Second
	// memberDeleteBatchWrite is the batch writer; it is a variable only so a test can make it fail
	// or hang.
	memberDeleteBatchWrite = deletion.ScheduleBatch
	// memberDeleteCountPending counts a chat's queued rows up to a limit; it is a variable only so a
	// test can make it fail (D-33).
	memberDeleteCountPending = deletion.CountPending
	// memberDeleteWarnMax is how many chats the shed log gate tracks.
	memberDeleteWarnMax = 10000
)

// memberDeleteDraining is set by the stop: later offers are shed.
var memberDeleteDraining atomic.Bool

// Why an entry was shed. The order indexes memberDeleteShedCounts and memberDeleteShedNames.
const (
	memberDeleteShedFull = iota
	memberDeleteShedDraining
	memberDeleteShedCeiling
	memberDeleteShedRule
	memberDeleteShedCount
	memberDeleteShedWrite
	memberDeleteShedReasons
)

var memberDeleteShedNames = [memberDeleteShedReasons]string{
	"queue full", "stopping", "ceiling", "rules unreadable", "count unreadable", "write failed",
}

var (
	// memberDeleteShedCounts counts the shed entries by reason, for this process since its start.
	memberDeleteShedCounts [memberDeleteShedReasons]atomic.Int64
	// memberDeleteWritten counts the entries a flush handed to a successful write.
	memberDeleteWritten atomic.Int64
	// memberDeleteDroppedOff counts the entries a flush dropped because the rule is no longer on:
	// correct, not a shed.
	memberDeleteDroppedOff atomic.Int64
)

// memberDeleteWarnGate lets one Warn per chat per hour through, with a bounded map.
type memberDeleteWarnGate struct {
	mu        sync.Mutex
	last      map[int64]time.Time
	capWarned bool
}

var memberDeleteShedWarn memberDeleteWarnGate

// allow reports whether a line for chatID may be logged at now, and records it. When the map holds
// memberDeleteWarnMax chats it forgets the entries older than an hour; when none can be forgotten a
// new chat logs nothing, and one process-wide line says so once.
func (g *memberDeleteWarnGate) allow(chatID int64, now time.Time) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	last, known := g.last[chatID]
	if known && now.Sub(last) < memberDeleteWarnEvery {
		return false
	}
	if g.last == nil {
		g.last = make(map[int64]time.Time)
	}
	if !known && len(g.last) >= memberDeleteWarnMax {
		for id, at := range g.last {
			if now.Sub(at) >= memberDeleteWarnEvery {
				delete(g.last, id)
			}
		}
		if len(g.last) >= memberDeleteWarnMax {
			if !g.capWarned {
				g.capWarned = true
				log.Warnf("[Deletion] the self-destruct shed log tracks %d chats; later chats are shed without a line", memberDeleteWarnMax)
			}
			return false
		}
	}
	g.last[chatID] = now
	return true
}

// memberDeleteShedWarnFor logs one Warn for chatID and reason at most once an hour: the chat ID and
// the reason name only.
func memberDeleteShedWarnFor(chatID int64, reason int) {
	if memberDeleteShedWarn.allow(chatID, deletionNow()) {
		log.Warnf("[Deletion] self-destruct entries of chat %d were not queued (%s); the messages stay",
			chatID, memberDeleteShedNames[reason])
	}
}

// memberDeleteShed counts n shed entries of chatID and logs the first of the hour.
func memberDeleteShed(chatID int64, reason, n int) {
	memberDeleteShedCounts[reason].Add(int64(n))
	memberDeleteShedWarnFor(chatID, reason)
}

// memberDeleteOffer hands one self-destruct entry to the batcher and reports whether it was
// accepted. It never blocks and never touches the database; a refusal keeps the message. Only the
// self-destruct kind is accepted, because the flush judges every entry by that kind's rule.
func memberDeleteOffer(e deletion.Entry) bool {
	if e.Kind != deletion.KindMemberSelfDestruct {
		return false
	}
	if memberDeleteDraining.Load() {
		memberDeleteShed(e.ChatID, memberDeleteShedDraining, 1)
		return false
	}
	select {
	case memberDeleteQueue <- e:
	default:
		memberDeleteShed(e.ChatID, memberDeleteShedFull, 1)
		return false
	}
	if len(memberDeleteQueue) >= memberDeleteFlushAt {
		select {
		case memberDeleteKick <- struct{}{}:
		default:
		}
	}
	return true
}

// memberDeleteDeleteAt is when an entry is due.
func memberDeleteDeleteAt(e deletion.Entry) time.Time {
	return e.SentAt.Add(time.Duration(e.DelaySecs) * time.Second)
}

// memberDeleteTake removes what is buffered, at most one buffer's worth, and merges the entries of
// one (chat, message) to the earliest time.
func memberDeleteTake() []deletion.Entry {
	type key struct{ chat, message int64 }
	index := make(map[key]int)
	var out []deletion.Entry
	for range cap(memberDeleteQueue) {
		var e deletion.Entry
		select {
		case e = <-memberDeleteQueue:
		default:
			return out
		}
		k := key{e.ChatID, e.MessageID}
		if i, ok := index[k]; ok {
			if memberDeleteDeleteAt(e).Before(memberDeleteDeleteAt(out[i])) {
				out[i] = e
			}
			continue
		}
		index[k] = len(out)
		out = append(out, e)
	}
	return out
}

// memberDeleteScreenChat applies the per-chat checks to one chat's entries and returns those that
// may be written: the rule must still be on (D-26a), and the chat's queued self-destruct rows plus
// the entries must stay within the ceiling. Entries past the ceiling are shed latest first.
func memberDeleteScreenChat(ctx context.Context, chatID int64, entries []deletion.Entry) []deletion.Entry {
	rules, err := deletionRulesLookup(ctx, chatID)
	if err != nil || rules == nil {
		memberDeleteShed(chatID, memberDeleteShedRule, len(entries))
		return nil
	}
	delay, present := rules.Delays[deletion.KindMemberSelfDestruct]
	if !present || deletion.ValidateRuleDelay(deletion.KindMemberSelfDestruct, delay) != nil {
		memberDeleteDroppedOff.Add(int64(len(entries)))
		return nil
	}
	queued, err := memberDeleteCountPending(ctx, chatID, deletion.KindMemberSelfDestruct, memberSelfDestructCeiling)
	if err != nil {
		memberDeleteShed(chatID, memberDeleteShedCount, len(entries))
		return nil
	}
	room := max(memberSelfDestructCeiling-int(queued), 0)
	if len(entries) <= room {
		return entries
	}
	slices.SortStableFunc(entries, func(a, b deletion.Entry) int {
		return memberDeleteDeleteAt(a).Compare(memberDeleteDeleteAt(b))
	})
	memberDeleteShed(chatID, memberDeleteShedCeiling, len(entries)-room)
	return entries[:room]
}

// memberDeleteFlush writes what is buffered and returns how many entries the writer reported
// written. Every entry that is not written is shed, and its message stays.
func memberDeleteFlush(ctx context.Context) int {
	entries := memberDeleteTake()
	if len(entries) == 0 {
		return 0
	}
	slices.SortStableFunc(entries, func(a, b deletion.Entry) int { return cmp.Compare(a.ChatID, b.ChatID) })
	var kept []deletion.Entry
	for start := 0; start < len(entries); {
		end := start
		for end < len(entries) && entries[end].ChatID == entries[start].ChatID {
			end++
		}
		kept = append(kept, memberDeleteScreenChat(ctx, entries[start].ChatID, entries[start:end])...)
		start = end
	}
	return memberDeleteWriteKept(ctx, kept)
}

// memberDeleteWriteKept writes the screened entries in one call of the batch writer. A returned
// error sheds what the writer did not write; the log is one line by chat count, and each chat gets
// its hourly line.
func memberDeleteWriteKept(ctx context.Context, kept []deletion.Entry) int {
	if len(kept) == 0 {
		return 0
	}
	wrote, err := memberDeleteBatchWrite(ctx, kept)
	if err != nil {
		failed := max(len(kept)-wrote, 0)
		memberDeleteShedCounts[memberDeleteShedWrite].Add(int64(failed))
		chats := make(map[int64]bool)
		for _, e := range kept {
			chats[e.ChatID] = true
		}
		if memberDeleteShedWarn.allow(0, deletionNow()) {
			log.Warnf("[Deletion] the self-destruct batch write for %d chats failed: %v", len(chats), err)
		}
		for chatID := range chats {
			memberDeleteShedWarnFor(chatID, memberDeleteShedWrite)
		}
	}
	if wrote <= 0 {
		return 0
	}
	memberDeleteWritten.Add(int64(wrote))
	for _, e := range kept {
		if !memberDeleteDeleteAt(e).After(deletionNow()) {
			wakeDeletionWorker()
			break
		}
	}
	return wrote
}

// memberDeleteFlushOnce runs one flush on a context detached from the cancelled deletion root and
// bounded on its own, under its own panic recovery so a panicking flush loses only what it took.
func memberDeleteFlushOnce(wait time.Duration) {
	defer error_handling.RecoverFromPanic("memberDeleteFlush", "Deletion")
	ctx, cancel := context.WithTimeout(context.WithoutCancel(deletionContext()), wait)
	defer cancel()
	memberDeleteFlush(ctx)
}

// memberDeleteFinalFlush is the flush a stop asks for: it repeats until the buffer is empty or the
// final wait is over.
func memberDeleteFinalFlush() {
	defer error_handling.RecoverFromPanic("memberDeleteFinalFlush", "Deletion")
	ctx, cancel := context.WithTimeout(context.WithoutCancel(deletionContext()), memberDeleteFinalFlushWait)
	defer cancel()
	for len(memberDeleteQueue) > 0 && ctx.Err() == nil {
		memberDeleteFlush(ctx)
	}
}

// Lifecycle state of the batcher goroutine, as the worker's: one run, one done channel.
var memberDeleteRun struct {
	mu   sync.Mutex
	stop chan struct{}
	done chan struct{}
}

// memberDeleteStart starts the batcher goroutine. It is idempotent: while one runs, further
// calls do nothing. StartDeletionWorker calls it, so the batcher lives and stops with the worker.
func memberDeleteStart() {
	memberDeleteRun.mu.Lock()
	defer memberDeleteRun.mu.Unlock()
	if memberDeleteRun.stop != nil {
		return
	}
	memberDeleteDraining.Store(false)
	stop := make(chan struct{})
	done := make(chan struct{})
	memberDeleteRun.stop = stop
	memberDeleteRun.done = done
	every := memberDeleteFlushEvery
	go func() {
		defer close(done)
		defer error_handling.RecoverFromPanic("memberDeleteBatcher", "Deletion")
		memberDeleteLoop(stop, every)
	}()
}

// memberDeleteLoop flushes on a ticker and on a kick until stop is closed, then runs the final
// flush.
func memberDeleteLoop(stop <-chan struct{}, every time.Duration) {
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			memberDeleteFinalFlush()
			return
		case <-ticker.C:
			memberDeleteFlushOnce(deletionRecordTimeout)
		case <-memberDeleteKick:
			memberDeleteFlushOnce(deletionRecordTimeout)
		}
	}
}

// memberDeleteStop sets the draining flag (later offers are shed), asks the batcher for its
// final flush and returns its done channel, or nil when no batcher runs. It is safe to call more
// than once; StopDeletionWorker waits for the channel under its one timer. A later start runs a
// new batcher.
func memberDeleteStop() <-chan struct{} {
	memberDeleteDraining.Store(true)
	memberDeleteRun.mu.Lock()
	defer memberDeleteRun.mu.Unlock()
	if memberDeleteRun.stop == nil {
		return nil
	}
	close(memberDeleteRun.stop)
	done := memberDeleteRun.done
	memberDeleteRun.stop = nil
	memberDeleteRun.done = nil
	return done
}
