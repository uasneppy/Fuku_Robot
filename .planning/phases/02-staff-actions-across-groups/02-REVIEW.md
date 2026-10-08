---
phase: 02-staff-actions-across-groups
reviewed: 2026-10-05T12:00:00Z
depth: standard
files_reviewed: 10
files_reviewed_list:
  - AGENTS.md
  - alita/modules/staff_action_card.go
  - alita/modules/staff_action_fake_test.go
  - alita/modules/staff_action_flood_test.go
  - alita/modules/staff_action_lifecycle_test.go
  - alita/modules/staff_action_progress_test.go
  - alita/modules/staff_action_run.go
  - alita/modules/staff_action_summary.go
  - alita/utils/ratelimit/telegram_pacer.go
  - alita/utils/ratelimit/telegram_pacer_test.go
findings:
  critical: 0
  warning: 1
  info: 4
  total: 5
status: issues_found
---

# Phase 2: Code Review Report (incremental re-review after gap-closure plans 02-08, 02-09, 02-10)

**Reviewed:** 2026-10-05T12:00:00Z
**Depth:** standard
**Files Reviewed:** 10
**Status:** issues_found

## Summary

This is an incremental re-review of the 10 files that plans 02-08, 02-09 and 02-10 changed since 03f2650 (`git diff 03f2650..HEAD`). It judges the changes against the two warnings of the first review and against the rules in AGENTS.md.

All finding IDs below (WR-03, IN-07 and so on) are new to this re-review. Their numbering continues the first review's, so they do not collide with the IDs recorded in the 03f2650 review.

Findings from the 03f2650 review in files outside this scope (the parser, the decision table, the locales, the docs, `main.go` and so on) stay as recorded in `02-REVIEW-DISPOSITION.md`. They are not re-listed here.

No security vulnerability and no data-loss path was found in the new code. The per-group authority chain, the decision table and the Lua compare-and-set transitions are untouched. The new Lua script (`renewStaffTargetLockScript`) is atomic and only writes under the card's own token or onto a missing key. It never extends a foreign lock.

### Disposition of the two first-review warnings

**First-review WR-01 (final summary lost under flood control): RESOLVED**, with a small residual for waits longer than 60 s (see IN-08 below).

Evidence:
- The shared 15 s `staffActionDeliverTimeout` is gone. `deliverStaffActionFinal` (`staff_action_summary.go:508-534`) now opens a fresh `newStaffDeliverContext()` for the final edit, for the fallback send and for each continuation part. Each uses `context.Background()`, so a shutdown cancel cannot starve it.
- The budget is `staffActionFinalAttempts * (staffActionRetryAfterCap + staffActionEditTimeout)`, which is 210 s. The worst case for one message is one hold wait (at most 60 s) plus two capped retry waits (2 x 60 s) plus three attempts of at most 10 s each, which is 210 s. Every wait the retry loops can request therefore fits the budget it runs under.
- Progress edits no longer pile onto a flooded chat. A 429 on a progress edit sets `editHoldUntil` (`staff_action_run.go:218-267`). The tick is skipped before the snapshot, so the changes stay marked, and `markDirty` keeps the unsent snapshot pending. The final edit waits out the hold, capped at one 60 s wait, through `notBefore`.
- The tests exercise the budget-versus-wait relationship at production durations (`withVirtualStaffSleep`, `TestStaffActionFinalEditLongRetryAfter`, `TestStaffActionFinalPartsOwnBudget`) and the hold behaviour in real time (`TestStaffActionProgressBacksOffAfter429`, `TestStaffActionFinalEditWaitsForHold`).

**First-review WR-02 (shared pacer block above MaxWait stalls the fleet and can outlive the target lock): RESOLVED.** Both halves of the finding are addressed.

Evidence for the stall:
- `reserveSlotScript` (`telegram_pacer.go:81-96`) now takes the caller's MaxWait as `ARGV[3]`. When the slot is further away it returns the distance without writing `SET`, so a refused caller takes no slot. `reserveLocal` (`:211-232`) does the same for the Redis-down fallback.
- `wait` (`:166-185`) turns a delay above MaxWait into `ErrRateLimited` before any sleep. `Do` therefore makes no Telegram request in that case. No sleep longer than MaxWait is left on the path, and the `staffActionCallTimeout` timer inside the closure now starts at most 60 s after the call began.
- The worker maps that error to the "rate limited" reason through `classifyStaffLookupFailure`, `classifyStaffFailure` and `staffOwnerUnknownReason`, so every group is still reported (`TestStaffActionBlockAboveMaxWaitFailsFast` checks no request reaches a group and the next run works once the block is gone).
- The overflow of an absurd `retry_after` into a negative `time.Duration` is closed by the clamps in `Do` (86400 s) and `staffRetryAfterHold` (3600 s).

Evidence for the lock:
- `startStaffActionRun` renews the target lock every `staffTargetLockRenewEvery` (10 min, a third of the TTL) from the coordinator goroutine, while the fan-out runs.
- `renewStaffTargetLockScript` (`staff_action_card.go:103-120`) is atomic. It extends its own token, re-takes a vanished key, and returns 0 and writes nothing when another token holds the lock. `TestStaffActionTargetLockRenewedDuringRun` covers all three cases.
- Renewal stops when the fan-out ends. That is correct, because no group write happens after it. The deferred compare-and-delete release still runs after delivery.

One residual weakness remains in each half, and neither blocks the resolution: WR-03 below (an edge in the new fail-fast threshold) and IN-07 (a lost lock is only logged).

## Warnings

### WR-03: A `retry_after` at the `MaxWait` cap makes all but one concurrent worker fail instead of retrying

**File:** `alita/utils/ratelimit/telegram_pacer.go:155` (`d > p.opts.MaxWait` in `Do`), `:172-174` (`wait`), `:91-93` (Lua threshold)
**Issue:** Two different thresholds guard the same wait. `Do` accepts a `retry_after` whose block `d` is at most `MaxWait`, so a 429 with `retry_after = 60` is retried. But the refusal in `wait` and in the script compares the whole slot distance `slot - now` against MaxWait, and that distance includes the queue behind the block.

Trace with the production values (4 workers, 100 ms interval, MaxWait 60 s):
1. Several workers get the same 429 with `retry_after = 60`. Each one records the block.
2. Each retry reserves a slot. The first gets `now + PTTL`, which is about 60 s minus a few ms, so it waits and retries.
3. The script sets the next slot to the first slot plus 100 ms. The second caller then sees a distance above 60 s and is refused with `ErrRateLimited`. The third and the fourth are refused the same way.

The refused groups are reported as "rate limited" instead of waiting 60 s and carrying on, even though `Do`'s own rule says a 60 s wait is acceptable. Telegram flood-control values of exactly 60 are common. The window is narrow (a `retry_after` within about N x 100 ms of the cap), but it sits at a value that really occurs. The new boundary test ("exactly the cap is waited") uses a single caller, so it does not see this.

The failure is reported, not silent, so it degrades the fan-out and does not lose data.

**Fix:** Judge the cap on the block alone, not on the queue behind it. Let the script refuse only when the block component is over the cap and let queue spacing add to it.

```lua
-- reserveSlotScript
local slot = math.max(now, nxt)
if blk > 0 then slot = math.max(slot, now + blk) end
local interval = tonumber(ARGV[1])
-- Refuse on the shared block alone; queued callers behind it still wait their turn.
if blk > tonumber(ARGV[3]) then return blk end
redis.call('SET', KEYS[1], slot + interval, 'PX', slot - now + interval + tonumber(ARGV[2]))
return slot - now
```

Apply the same rule in `reserveLocal` (compare `p.localBlock.Sub(now)`) and in `wait` (compare the returned delay against `MaxWait` plus a small allowance such as `MaxRetries * Interval * staffActionWorkers`, or have `reserve` return the block distance separately). Add a concurrent test with 3 callers and a block exactly at the cap.

## Info

### IN-07: A lost target lock is only logged; the run keeps writing

**File:** `alita/modules/staff_action_run.go:231-236`
**Issue:** When the renewal returns `held == false` (another card now holds the target lock), the coordinator logs a warning and the fan-out continues. The lock exists to stop two staff actions on one target from running at once and, with them, the restrict-replaces-ban race that `decideStaffAction` guards against. Reaching this branch needs the key to vanish and a second Confirm to take it, so it is rare. But the run then has no exclusivity and proceeds anyway.
**Fix:** Cancel the run's own fan-out context when the lock is lost, and let the remaining groups read "interrupted". For example, derive `runCtx, cancelRun := context.WithCancel(ctx)` for the fan-out and call `cancelRun()` in the `!held` branch. Keep the plain warning for a Redis error, which is "unknown", not "lost".

### IN-08: Retry waits are truncated to 60 s, so a `retry_after` above 60 s burns attempts

**File:** `alita/modules/staff_action_summary.go:411-416` (`staffRetryAfterWait`), `:458-470` (`editStaffActionFinal`), `:473-486` (`sendStaffSummaryPart`)
**Issue:** The cap makes `staffRetryAfterWait` return at most 60 s, however long Telegram asked. With a 3-attempt loop, a `retry_after` above about 120 s makes the final edit fail, and the fallback send faces the same chat-level flood and may fail as well. The new 210 s budget could cover a longer wait, but the loop never uses it. This only matters for flood waits above a minute, which is rare for message edits, so the first review's WR-01 is resolved for the cases it described.
**Fix:** When `retry_after` fits the remaining budget (`wait <= time.Until(deadline)`), wait the full `retry_after` instead of the cap. Only a value that cannot fit should end the loop, and it should end it at once so the fallback runs.

### IN-09: Two new timing tests install their 429 script after the run has started

**File:** `alita/modules/staff_action_progress_test.go:325-327` (`TestStaffActionProgressBacksOffAfter429`), `:362-364` (`TestStaffActionFinalEditWaitsForHold`)
**Issue:** Both tests call `env.startRun(...)` and only then `env.fake.script("editMessageText", ..., staffFake429(n))`. The run's coordinator already ticks every 20 ms. Under `-race` load, a progress edit can land before the script is queued. The 429 then goes to a later edit, and the assertions index `times[1]` as the rate-limited edit, so they would measure the wrong gap or fail. The fan-out delays (25 ms and up) make this unlikely, not impossible, so the tests are a flake risk.
**Fix:** Queue the script before the Confirm tap (script `nil` for the Confirm edit first, then the 429, as `TestStaffActionFinalEditLongRetryAfter` does), or split `startRun` so the script goes in between the card and the tap.

### IN-10: New tests leak shared state when they fail early

**File:** `alita/modules/staff_action_flood_test.go:22-30`, `alita/modules/staff_action_lifecycle_test.go:652-680` (`heldStaffRun` subtests)
**Issue:** `TestStaffActionBlockAboveMaxWaitFailsFast` sets the shared `alita:staff:pace:block` key and deletes it only on the happy path. A `t.Fatalf` between the two leaves a 2-minute block in miniredis. Every later staff test then fails fast with "rate limited", which hides the real cause of the second failure. In `heldStaffRun`, a failure before `finish()` leaves the run goroutine alive, and the `t.Cleanup` that restores `staffTargetLockRenewEvery` can then race with that goroutine, which reads it.
**Fix:** Register `t.Cleanup(func() { client.Del(cache.Context, blockKey) })` right after setting the block. In `heldStaffRun`, register `t.Cleanup(env.waitRuns)` after `releaseRun`. Cleanups run last in first out, so the wait runs after the release and before the variable is restored.

---

_Reviewed: 2026-10-05T12:00:00Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
