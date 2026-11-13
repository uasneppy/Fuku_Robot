---
phase: 17-command-edit-and-self-destruct-deletion
reviewed: 2026-11-12T12:00:00Z
depth: deep
files_reviewed: 41
files_reviewed_list:
  - fuku/db/deletion/kinds.go
  - fuku/db/deletion/queue.go
  - fuku/db/deletion/rules.go
  - fuku/modules/command_deletion.go
  - fuku/modules/member_deletion.go
  - fuku/modules/member_deletion_batcher.go
  - fuku/modules/deletion_apply.go
  - fuku/modules/deletion_kinds.go
  - fuku/modules/deletion_right.go
  - fuku/modules/deletion_schedule.go
  - fuku/modules/deletion_worker.go
  - fuku/modules/settings_deletion.go
  - fuku/modules/pins.go
  - fuku/modules/staff_action.go
  - fuku/modules/settings_input.go
  - fuku/utils/helpers/command_registry.go
  - fuku/utils/helpers/command_pipeline.go
  - fuku/utils/helpers/decorators.go
  - main.go
  - scripts/generate_docs/main.go
  - scripts/generate_docs/parsers.go
  - fuku/modules/admin.go
  - fuku/modules/antiflood.go
  - fuku/modules/approvals.go
  - fuku/modules/backup.go
  - fuku/modules/blacklists.go
  - fuku/modules/connections.go
  - fuku/modules/devs.go
  - fuku/modules/disabling.go
  - fuku/modules/federations.go
  - fuku/modules/filters.go
  - fuku/modules/help.go
  - fuku/modules/language.go
  - fuku/modules/locks.go
  - fuku/modules/logchannels.go
  - fuku/modules/misc.go
  - fuku/modules/misc_extras.go
  - fuku/modules/notes.go
  - fuku/modules/reactions.go
  - fuku/modules/reports.go
  - fuku/modules/rules.go
findings:
  critical: 0
  warning: 3
  info: 3
  total: 6
status: issues_found
---

# Phase 17: Code Review Report

**Reviewed:** 2026-11-12T12:00:00Z
**Depth:** deep
**Files Reviewed:** 41 (production only; tests read for what they prove)
**Status:** issues_found

## Summary

I reviewed `git diff 42c48c4..fd40ac2` against the Phase 17 rules in AGENTS.md. Where a claim depended on library behaviour I read gotgbot at `v2.0.0-rc.36.0.20260919140833-240296efadb4` (`ext/handlers/command.go`, `ext/dispatcher.go`, `ext/context.go`, `ext/processor.go`). `CGO_ENABLED` build is clean and `make check-translations` passes.

No Critical findings. Three Warnings, all about the member self-destruct path, and three Info items. The command-deletion wrapper, the edit handler, the repository validators and the settings page held up under adversarial tracing.

Severity mapping for the caller's scale: WR-01 medium, WR-02 low-medium, WR-03 low; IN-01..IN-03 info/low.

### Areas that are clean (verified, not assumed)

- **`CommandDeletionProcessor` (`command_deletion.go`)**
  - `botCommandName` is a faithful copy of gotgbot's `Command.checkMessage`: `GetText`/`GetEntities` caption preference, the offset-0 entity rule, `IndexAny` first field, `SplitN(ToLower, "@", 2)` against `b.Username`, and the rich-message branch including the case-sensitive compare. I found no divergence.
  - The `HasPrefix(text, "/")` pre-check matches the only trigger (`/`). `SetTriggers` and `handlers.Command{}` literals appear nowhere outside `command_registry.go`.
  - The sender filter holds for `sender_chat`, bots, service IDs and the bot itself.
  - The post-step reads `ctx.Message` and `ctx.EffectiveSender` after `Inner` returned. `verifyAnonymousAdmin` (`bot_updates.go:134`) rebuilds `ctx.Update`, clears `msg.SenderChat` and sets `EffectiveSender` to the tapper, so the anonymous-admin ordering holds.
  - No handler mutates `Text`, `Caption`, `Entities` or `Chat` of the message (grep), so re-deriving the command after the handlers ran is safe.
  - A handler panic is recovered by `Dispatcher.ProcessUpdate` above the wrapper (`dispatcher.go:259`), so skipping the delete is correct. The wrapper's own work recovers panics and runs on fresh contexts.
  - The rule read treats any stored value other than 0 as Off.
  - A Staff Group and a private chat are refused (`Chat.Id >= 0` first, then `deletionAct`).
- **`helpers.NewBotCommand`**
  - It lower-cases under a `sync.RWMutex` and returns `handlers.NewCommand(name, resp)` unchanged, so chained setters work.
  - `WrapCommand`'s `register`, `MultiCommand` and the raw handlers all go through it.
  - The 20-module rewrite is import adjustments plus the call token only. I diffed each file with the token normalised and found no other differences.
  - The `staff_action.go` token to placeholder rename is mechanical and compiles.
  - The generate-docs regexp follows the new token.
- **Edit handler**
  - The `CheckUpdate` is `EditedMessage != nil` only, so it cannot see a new message.
  - The result is preset to `ext.ContinueGroups` under `RecoverFromPanic`, and no path returns `nil` or `EndGroups`.
  - Grace arithmetic is correct at the boundaries: a gap equal to the grace is kept, a missing or non-later `edit_date` is kept (the field is an `int64`, so "nil" is 0), and a message older than 47 h is kept.
  - Edits from `sender_chat`, automatic forwards, bots, `via_bot` and service IDs are skipped before any I/O.
- **Group -8 sharing.** `memberEditedHandler` matches edits only and the watcher matches `ctx.Message != nil` only, so the two never run for one update. ContinueGroups semantics in `iterateOverHandlerGroups` (`dispatcher.go:294`) match AGENTS.md.
- **Tri-state exemptions (D-33).** There is no path where an error reads as "not exempt".
  - `memberDeletionExempt` returns true on an approvals error.
  - It returns true unless the admin status is definitely `NotAdmin`.
  - The captcha check treats a read error as pending.
  - `memberDeletionAdminStatus` maps every error, an empty answer and an unrecognised status to unknown.
- **`fuku/db/deletion`**
  - `ValidateRuleDelay` and `ValidateEntryDelay` check by value, so a hand-written 45 is refused.
  - `ScheduleBatch` validates every entry before any read or write.
  - It merges by `(chat, message)` before the statement, which avoids PostgreSQL's "cannot affect row a second time".
  - It sorts rows so concurrent batches lock in one order, reads the Staff Group once per chat and fails closed per chat, and chunks at 100.
  - `leastExpr` is table-qualified for PostgreSQL and uses `MIN` for SQLite.
  - `CountPending` is a valid `LIMIT ?` subquery on both dialects.
  - `DropPendingMessage` matches the pending state, exact chat, message and kind.
  - `SetRule` and `RemoveRule` both `cache.DeleteCache` the `delete_rules` key, and `RemoveRule` drops the exact kind's pending rows in the same transaction.
- **Worker**
  - Every latch site (`deletionLatched`, `deletionInlineDecideLatch`, `maybeLatchService`, and the `DropChatPending` of `service:` and `member:` in `deletionDecideServiceLatch`) uses `NeedsDeleteRight`; I found no remaining `IsServiceKind` latch site.
  - `screenSelfDestruct` sits after the latch and Staff Group screens and only touches `member:selfdestruct` rows.
  - A rules read failure keeps the row (unknown never deletes).
  - `StopDeletionWorker` waits for the worker and batcher under one `time.NewTimer`, so no allowance is added.
- **Settings**
  - `dy` validates `t` (a row with a `ConfirmKey`), `v` (a canonical preset, never `n`) and `e` before any I/O, and carries `settingsRightChangeInfo`, so a demoted presser is refused in the pipeline.
  - The Staff Group gate runs first, then the expiry, then `applyDeleteRule`, so a double Yes is idempotent.
  - `settingsDelEffective` reads an unacceptable stored member value as Off for the hub, list, row and count.
  - No renderer branches on a row code. Differences are table fields.
  - `make check-translations` is green for the new keys.

## Warnings

### WR-01: Self-destruct and edit exemption makes a live `getChatMember` per message when the admin cache is cold, and never warms it (medium)

**File:** `fuku/modules/member_deletion.go:92-122` (called from `:140`, `:236`, `:320`)
**Issue:** `memberDeletionAdminStatus` falls back to a live `getChatMember` whenever `cachedAdminStatus` says "unknown". Unlike `antifloodStruct.adminCheckWithTimeout`, which calls `chat_status.IsUserAdmin` and so fills the admin cache through `cache.LoadAdminCache`, this path never calls `LoadAdminCache`, and it does not cache its own answer.

The admin list is loaded only at bot-join (`bot_updates.go:118`), by `/admincache`, by admin commands and by `chat_status` checks of features that happen to be on. It expires after 30 minutes (`constants.AdminCacheTTL`). It is also negative-cached for 2 minutes when the bot cannot list admins.

Failure scenario: a group has only self-destruct (or only Edited messages) switched on, which is the typical use of this phase, and no other feature that checks admins.
1. 30 minutes after the bot joined, the cache expires.
2. From then on every message of every non-admin member runs `memberSelfDestructWatch` synchronously, in the dispatcher goroutine, at group -8, before group 0 and every other group. It takes an approvals read plus one `getChatMember` (about 100-300 ms, up to the 3 s timeout) for each message.
3. `memberDeletionAdminSlots` is 8 per replica, so above roughly 30-80 messages per second the semaphore is full and the answer is "unknown", which is exempt by design. Those messages silently never self-destruct.
4. Each saturated or failed lookup logs a `Warnf` with no rate limit (`:103`, `:110`), so a busy chat floods the log with one line per message.

The failure direction is safe (messages are kept), but the feature is partly dead in exactly the groups that rely on it. The latency is also added to every other handler of the update.

**Fix:** use the existing singleflighted loader instead of per-user live calls. When `cachedAdminStatus` is unknown, call `cache.LoadAdminCache(b, chatID)` (one `getChatAdministrators` per chat per 30 min) behind the same semaphore and timeout, then re-run `cachedAdminStatus`:
```go
if known, isAdmin := cachedAdminStatus(chatID, userID); known { ... }
// unknown: warm the list once, bounded, then read it
if acquireSlot() {
    defer releaseSlot()
    cache.LoadAdminCache(b, chatID) // singleflight, stores for 30 min
    if known, isAdmin := cachedAdminStatus(chatID, userID); known { ... }
}
// only then fall back to the per-user getChatMember
```
Also rate-limit the two warn lines to once per chat per interval, as `memberDeleteShedWarnFor` does.

### WR-02: The batcher ignores the delete-right latch, so a chat where the bot cannot delete keeps writing and counting rows (low-medium)

**File:** `fuku/modules/member_deletion_batcher.go:223-248`, `fuku/modules/member_deletion.go:303-331`
**Issue:** Every other write path reaches the latch through `deletionAct` or `deletionSchedule` (`deletionLatched`, now `NeedsDeleteRight`). The watcher and batcher bypass both on purpose (D-27) and call `deletion.ScheduleBatch` directly. `ScheduleBatch` checks only the Staff Group.

Failure scenario: the bot is demoted or lacks `can_delete_messages` in a busy self-destruct chat, and one member command or edit delete discovers it and sets the `fuku:delete:norights:<chat>` latch.
1. `deletionDecideServiceLatch` drops the chat's pending `member:` rows once.
2. The watcher keeps offering every message and the batcher keeps writing them. Each flush costs 1 rules read, 1 `CountPending` and 1 Staff Group read per chat, then a multi-row insert.
3. Those rows count toward the 20,000 ceiling.
4. They are only dropped at their due time, and only if the 10-minute latch is still set. After it expires the worker retries them, hits a refusal and re-decides.

Nothing is wrongly deleted, but this is wasted database work and ceiling consumption for a state the process already knows about.

**Fix:** in `memberDeleteScreenChat`, after the rules check, call `deletionLatched(ctx, chatID, deletion.KindMemberSelfDestruct)` once per chat per flush (a single Redis read) and drop the entries as a counted non-shed reason when it holds.

### WR-03: A pinned message can still self-destruct: the pin drop races the batcher buffer and a failed drop is only logged (low)

**File:** `fuku/modules/member_deletion.go:284-291, 311-314`; `fuku/modules/pins.go:336-338`; `fuku/modules/member_deletion_batcher.go:163-187`
**Issue:** D-11 says a pinned message stays. `memberDeletePinned` is a one-shot `DELETE ... WHERE state = 'pending'` that runs when the pin service message (or `/pin`) arrives. Two cases leave the row (or a later row) in place:
- **Race with the buffer.** A message offered at t0 sits in the in-memory channel for up to `memberDeleteFlushEvery` (300 ms). It sits longer if a flush is slow, up to the 5 s record timeout, and the queue is shared. An admin who pins within that window (a `/pin` typed as a reply to a fresh message) gets a no-op drop. The batcher then writes the row after the drop, and the pinned message is deleted at its due time.
- **Failed drop.** A database error or timeout is logged and the code comments that the only cost is that the message is still deleted. The pin event is not retried.

**Fix:** keep a small bounded in-process set of recently pinned `(chat, message)` keys (a few minutes of TTL, a few thousand entries). Have `memberDeletePinned` add to it and have `memberDeleteTake` or `memberDeleteScreenChat` drop matching entries. A failed `DropPendingMessage` could retry once or twice on the same fresh context.

## Info

### IN-01: `screenSelfDestruct` releases rows free when the rules are unreadable, so the next 1 s tick re-claims them and logs a Warn per chat every second (info)

**File:** `fuku/modules/deletion_worker.go:379-386`
**Issue:** An uncounted `Release` leaves `delete_at` alone, so rows due now go straight back into the next claim. During a Redis or database fault on `delete_rules`, each chat with due self-destruct rows logs `could not read the rules of chat ... to screen self-destruct rows` once per tick. No progress is counted, so the pass does not spin, but the cadence is 1 Hz per chat with unbounded log volume.
**Fix:** release with a short backoff (use the counted-release `retryAt` path without counting an attempt, or set `delete_at` to now plus 30-60 s), or rate-limit the log line per chat like `memberDeleteShedWarnFor`.

### IN-02: One 5 s context covers a whole flush, so a slow database loses the screened entries as well as the unscreened ones (info)

**File:** `fuku/modules/member_deletion_batcher.go:252-267, 305-312`
**Issue:** `memberDeleteFlushOnce(deletionRecordTimeout)` bounds screening of every chat plus the final `ScheduleBatch` with one 5 s deadline. If screening many chats under database latency consumes the budget, `ScheduleBatch` runs on an expired context and fails. Every already-screened entry is counted as "write failed" and dropped, although screening succeeded for them.
This is the safe direction (messages stay) and only appears under database stress. It is also the main reason offers can be lost during a database slowdown.
**Fix:** give the write its own bounded context (`context.WithTimeout(context.WithoutCancel(ctx), deletionRecordTimeout)`), or write per chat so one slow chat does not cancel the rest.

### IN-03: `memberDeletePinned` runs a synchronous database write for every pinned-message service message, in every chat, before any rule or sender check (info)

**File:** `fuku/modules/member_deletion.go:311-314`
**Issue:** Pin events are rare, so the cost is small. The write is synchronous in the dispatcher goroutine (up to 5 s) and happens for chats that have no self-destruct rule and for Staff Groups, for which the delete can never match a row. A cheap early return (rule on, or at least a chat with any pending `member:selfdestruct` row) would avoid the write. It would also remove the only place where a database stall delays group -8 for a chat that does not use the feature.
**Fix:** read the cached rules first and drop only when the rule is on or rows could exist. Combine with the in-memory pinned set from WR-03 so the cheap path covers both.

---

_Reviewed: 2026-11-12T12:00:00Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: deep_
