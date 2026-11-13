---
phase: 16-scheduled-deletion-of-service-and-bot-messages
reviewed: 2026-11-12T04:30:00Z
depth: deep
files_reviewed: 33
files_reviewed_list:
  - Makefile
  - fuku/db/cache/local.go
  - fuku/db/deletion/hook_testtools.go
  - fuku/db/deletion/kinds.go
  - fuku/db/deletion/queue.go
  - fuku/db/deletion/rules.go
  - fuku/db/greetings/repository.go
  - fuku/db/models/delete_rule.go
  - fuku/db/models/scheduled_deletion.go
  - fuku/modules/aispam.go
  - fuku/modules/antiflood.go
  - fuku/modules/bans.go
  - fuku/modules/blacklists.go
  - fuku/modules/deletion_apply.go
  - fuku/modules/deletion_bot.go
  - fuku/modules/deletion_kinds.go
  - fuku/modules/deletion_right.go
  - fuku/modules/deletion_schedule.go
  - fuku/modules/deletion_service.go
  - fuku/modules/deletion_worker.go
  - fuku/modules/greetings.go
  - fuku/modules/memberreq_fixed.go
  - fuku/modules/memberreq_notice.go
  - fuku/modules/moderation.go
  - fuku/modules/mute.go
  - fuku/modules/rules.go
  - fuku/modules/settings_deletion.go
  - fuku/modules/settings_edit_preview.go
  - fuku/modules/settings_menu.go
  - fuku/modules/settings_pages.go
  - fuku/modules/warns.go
  - main.go
  - migrations/20261110120000_add_scheduled_deletion.sql
findings:
  critical: 0
  warning: 3
  info: 5
  total: 8
status: issues_found
---

# Phase 16: Code Review Report

**Reviewed:** 2026-11-12T04:30:00Z
**Depth:** deep
**Files Reviewed:** 33
**Status:** issues_found

## Summary

I reviewed the phase's diff (`git diff 7e58561 HEAD`) against the AGENTS.md Phase 16 rules and decisions D-01..D-28. I also traced call chains across the queue repository, the worker, the inline delete path, the send-site hooks, the Deletion settings page and the shutdown wiring in `main.go`.

No Critical findings. These invariants held:

- **DEL-07:** Every scheduling call site is one of the named opt-in hooks. A grep of production code finds no other caller of `scheduleBotMessage*`, `deletionSchedule`, `deletionAct` or `deletion.Schedule`.
- **Staff Group refusal:** It fails closed in every place that reads it: `Schedule`, `deletionAct`, the worker's `process`, the page's gate, `Main`'s state and `Open`.
- **Previews and private chats:** Previews and the private rules message discard their sent message. `scheduleBotMessage*` refuses any chat type other than group or supergroup.
- **Migration:** It is idempotent and has inline named CHECKs. The statement splitter yields four statements and `TestScheduledDeletionMigrationStatements` passes.
- **Cache discipline:** Every `chat_delete_rules` write drops `delete_rules`. The joined/left transaction drops both `greetings` and `delete_rules`. A Go 0 delay is written, not skipped. `delete_rules:*` is in `skipLocal`.
- **Callback data:** The Deletion page's callback data is strictly validated. The preset validator accepts only canonical decimal seconds of the row's own presets, and `tg` is refused on row controls.
- **Locales:** All referenced locale keys exist in the 8 language files.
- **Tests:** The phase's targeted test families (`TestDeletion*`, `TestSchedule*`, `TestBotWelcomeScheduled`, `TestShutdownEndsDeletionWorker`, `./fuku/db/deletion`) pass.

The three Warnings are behavioural defects that I reproduced with throwaway tests. The throwaway files were deleted and `git status` is clean. Three of the Info items trace to design decisions (D-08, D-13, D-18) that I think need a second look.

## Warnings

### WR-01: A flood-control wait longer than 15 s silently loses inline "delete now" messages instead of queueing them

**File:** `fuku/modules/deletion_schedule.go:46` and `fuku/modules/deletion_schedule.go:398-409`

**Issue:** `deletionInlineTimeout` is 15 s and bounds the whole inline delete, including the pacer's slot wait. The pacer accepts a shared block of up to `MaxWait` (60 s) and sleeps through it. When a 429 block or a long slot queue is longer than 15 s, the inline context expires inside `p.wait`, which returns `context.DeadlineExceeded`.

`deletionClassify` reads that as `deletionVerdictRetryFree`, because `ctx.Err() != nil`. `deletionInlineSettle` then queues a row only when `errors.Is(err, ratelimit.ErrRateLimited)`. A deadline is not `ErrRateLimited`, so no request is made, no row is written, and the message is never deleted.

This is the opposite of the documented behaviour that "a delete the pacer shed becomes a row due now". It hits exactly when it matters: a join raid with `/cleanservice` on or a "Delete now" service rule produces a burst of inline deletes, which is what causes flood control.

Reproduced with a throwaway test. `fuku:delete:pace:block` was set with a 30 s TTL and `deletionAct` was called with `Delay: 0`:

```
=== RUN   TestZZInlineFloodLoss
    zz_review_throwaway_test.go:25: waited 15.010572821s; delete calls=0 queued rows=0
--- PASS: TestZZInlineFloodLoss (15.29s)
```

After the 15 s wait there were no `deleteMessage` calls and no queue row.

**Fix:** In `deletionInlineSettle`, treat a deadline of the inline context while the root is still live as "shed", the same as `ErrRateLimited`:

```go
case deletionVerdictRetryFree:
    shed := errors.Is(err, ratelimit.ErrRateLimited) ||
        (errors.Is(err, context.DeadlineExceeded) && deletionContext().Err() == nil)
    if shed {
        deletionQueue(a)
    }
```

Alternatively, make `deletionInlineTimeout` at least `MaxWait` plus the call timeout (70 s). The queue path is the better fix, because 64 goroutines would otherwise sit parked for a minute.

### WR-02: Counted retries have no backoff, so a few seconds of Telegram 5xx or timeouts permanently drops rows

**File:** `fuku/modules/deletion_worker.go:397-411` (`settleOne`), `fuku/db/deletion/queue.go:200-236` (`Release`), `fuku/modules/deletion_worker.go:107-115` (loop)

**Issue:** A 5xx, a transport error or a per-call timeout is `deletionVerdictRetry`. `settleOne` releases the row with `countAttempt = true`, so `Release` increments `attempts` and returns it to pending with `delete_at` unchanged. The row is immediately due again.

The loop re-claims it on the next 1 s tick, because a put-back row is not "progress" and so the loop waits only for the tick. After `deletionMaxAttempts` (3) such ticks, `attempts + 1 >= 3` deletes the row. A Telegram outage or Bot API blip of about 3 s therefore drops every due row: the messages are never deleted and the owner is never told. Rows queued with a 24 h delay are lost the same way.

D-07 says a 5xx or timeout "puts the rows back to pending (three tries)". It does not say the three tries may all fall within three seconds. The existing test `TestDeletionWorkerClassifiesErrors` encodes three back-to-back cycles as the intended behaviour.

Reproduced. Two rows were due and every `deleteMessages` and `deleteMessage` call answered 502. Three consecutive `cycle()` calls gave:

```
after cycle 1: 2 rows left ... Attempts:1
after cycle 2: 2 rows left ... Attempts:2
after cycle 3: 0 rows left
```

In production the same thing takes about 3 s of wall time.

**Fix:** Space the counted retries by pushing `delete_at` forward on a counted release. For example, in `Release` with `countAttempt` set `delete_at` to `now + backoff * (attempts + 1)`, where the backoff is 1 to 2 minutes and the 47 h drop and the 24 h cap still bound the row's life. Alternatively, count an attempt at most once per `deletionStaleClaim`.

### WR-03: Switching a rule to Keep or Off (or `/cleanservice off`) does not stop messages already queued; the worker never re-reads the rule

**File:** `fuku/modules/deletion_apply.go:107-130` (`applyDeleteRule`), `fuku/modules/deletion_apply.go:169-194` (`applyCleanServiceRule`), `fuku/db/deletion/rules.go:93-106` and `fuku/db/deletion/rules.go:209-228`, `fuku/modules/deletion_worker.go` (no rules read)

**Issue:** `RemoveRule`, `ApplyJoinLeftRule` (Keep) and `ApplyCleanServiceFlag` (off) only delete rule rows and flip the flag. They leave `scheduled_deletions` alone, and the worker never consults the rules, so every already-queued row of that kind is still deleted, up to 24 h later.

An owner who presses Off or Keep because the group does not want welcomes or notices deleted still sees them disappear for up to a day. This contradicts the stated contract that "a rule switched off must stop acting on every replica at the next message" (the `skipLocal` comment and D-01). The apply functions report success and the page redraws "Off".

Reproduced. `bot:welcome` was set to 600 s, a row was queued, and the owner removed the rule through `applyDeleteRule` (`outcome=0`, `rules=map[]`). Advancing the clock past `delete_at` and running a cycle still deleted the message:

```
cycle at now: deleteMessages=[] single=[] rows=1
cycle after delete_at (rule is Off): deleteMessages=[[55001]] single=[] rows=0
```

**Fix:** On a successful Remove, Keep or flag-off write, drop the chat's pending rows of that kind with the existing helper:

- `RemoveRule`: `DropChatPending(ctx, chatID, kind)`, using an exact kind match. `DropChatPending` currently takes a prefix and is LIKE-escaped, which is fine for a full kind string.
- `ApplyJoinLeftRule` (Keep) and `ApplyCleanServiceFlag`: do the same for `service:new_chat_members` and `service:left_chat_member` in the same transaction.

A change of delay to a longer one does not need this. A shorter one is already handled by the `LEAST` upsert only for new messages. Alternatively, have the worker check `deletionRulesLookup` per `(chat, kind)` before the call, and drop rows whose rule is gone.

## Info

### IN-01: The `/ban` and `/unban` "reply to a message" hint is queued as a manual-punishment success reply

**File:** `fuku/modules/bans.go:224-231` and `fuku/modules/bans.go:402-420`

**Issue:** For a channel target with no replied message, `execute` does nothing and `reply` sends `bans_anonymous_ban_reply_only` ("You can only ban an anonymous user by replying to their message") or `bans_anonymous_unban_reply_only`. The reply goes through `replySuccess`, so it is scheduled as `bot:punish_manual`. D-13 says validation and error replies are never covered. Impact is small, because a usage hint disappears after the timer.

Reproduced with `/ban -1001234567890` and `/unban -1001234567890` and the kind on at 600 s. A `bot:punish_manual` row for message 9001 was written in both cases. The test locale was not loaded, so the sent text was empty, but the hint branch is the only one reachable without a replied message.

**Fix:** In those two branches send with `c.Msg.Reply` directly, or give `replySuccess` a `schedule bool` argument, and keep `replySuccess` for the real-success texts.

### IN-02: `/cleanservice` no longer deletes join and leave messages in a Staff Group

**File:** `fuku/modules/greetings.go:1157-1166`, `fuku/modules/deletion_schedule.go:357`

**Issue:** Before the phase, `cleanService` deleted join and leave service messages in any chat with the flag on. It now goes through `deletionAct`, which refuses a Staff Group, so a Staff Group with `/cleanservice` already on silently stops cleaning. This follows from D-18 and DEL-07, but it is a behaviour change the owner may not expect, and AGENTS.md does not list it. Reproduced: with a Staff Group and the flag on, a join service message produced 0 deletes and 0 queue rows.

**Fix:** Add one line to the AGENTS.md Phase 16 bullets saying join and leave cleaning is off inside a Staff Group. Optionally make `/cleanservice` in a Staff Group reply that it has no effect there.

### IN-03: A failure between the claim `UPDATE` and the read-back in `ClaimDue` strands the claimed rows for two minutes

**File:** `fuku/db/deletion/queue.go:150-171`, `fuku/modules/deletion_worker.go:214-219`

**Issue:** `ClaimDue` commits the `UPDATE` (rows become `acting` with the token) and then reads the rows back in a second statement. If that second statement fails (the worker context is cancelled at shutdown, or a connection drops), it returns `(nil, err)`. `runDeletionCycle` logs and returns without releasing, so those rows sit `acting` until `ReleaseStale` hands them back after `deletionStaleClaim` (2 minutes). This is bounded and documented ("claimed until the 2 minute stale release"), so it is Info. I did not reproduce it, because it needs a failure between two statements.

**Fix:** On a read-back error, release by token on a detached context:

```go
if err != nil {
    // the UPDATE may have committed: give the token back
    rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), deletionRecordTimeout)
    defer cancel()
    _ = releaseToken(rctx, token) // UPDATE ... SET state='pending' WHERE claim_token = ? AND state = 'acting'
}
```

### IN-04: Test-only helpers in production code

**File:** `fuku/modules/deletion_kinds.go:184-204`, `fuku/db/deletion/kinds.go:77-80` and `fuku/db/deletion/kinds.go:97-100`

**Issue:** `deletionRowOf`, `deletionRowOfKind` (and `deletionFindRow` through them), `deletion.Kinds()` and `deletion.IsBotKind()` have no production caller. A grep shows they are used only by `*_test.go` files. They are unreferenced code in the shipped binary.

**Fix:** Move them to a `_test.go` file (or a `testtools`-tagged file), or delete them. The page already resolves rows through `settingsDelRowByCode`.

### IN-05: `deletion.Schedule` does not reject non-group chat IDs, so DEL-07's "never a private chat" rests only on the callers

**File:** `fuku/db/deletion/queue.go:80-82`

**Issue:** `Schedule` is documented as the one queue writer and refuses a Staff Group, but it accepts any non-zero chat ID, including a positive (private) one. Every current caller checks the chat type first, so nothing is wrong today. But D-18 lists "the call sites" as the layer for private chats, and a future call site that forgets the check would queue a private-chat message. `deletionIsStaffChat` and `Schedule` would not stop it.

**Fix:** Add `if e.ChatID >= 0 { return false, ErrInvalidEntry }` next to the zero check (Telegram group and supergroup IDs are negative). This is cheap defence in depth. Update `TestSchedule*` accordingly.

---

_Reviewed: 2026-11-12T04:30:00Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: deep_
