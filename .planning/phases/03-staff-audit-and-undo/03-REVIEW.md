---
phase: 03-staff-audit-and-undo
reviewed: 2026-10-06T00:00:00Z
depth: standard
files_reviewed: 20
files_reviewed_list:
  - AGENTS.md
  - alita/db/staff/actions.go
  - alita/db/staff/actions_test.go
  - alita/modules/staff_action_run.go
  - alita/modules/staff_history.go
  - alita/modules/staff_history_test.go
  - alita/modules/staff_history_undo_test.go
  - alita/modules/staff_undo.go
  - alita/modules/staff_undo_claim_test.go
  - alita/modules/staff_undo_lifecycle_test.go
  - alita/modules/staff_undo_shutdown_test.go
  - alita/modules/staff_undo_test.go
  - docs/src/content/docs/commands/staff/index.md
  - locales/en.yml
  - locales/es.yml
  - locales/fr.yml
  - locales/hi.yml
  - locales/id.yml
  - locales/pt.yml
  - locales/ru.yml
findings:
  critical: 0
  warning: 4
  info: 7
  total: 11
status: issues_found
---

# Phase 03: Code Review Report (re-review after gap-closure plans 03-10, 03-11, 03-12)

**Reviewed:** 2026-10-06
**Depth:** standard
**Files Reviewed:** 20
**Status:** issues_found

## Summary

Re-review of the changes since `d45631a` (`ReleaseUndo` and the `Reached` marker, `joinStaffRuns` and the shutdown drain for an undo Confirm, `staffUndoStateOf` with its history and answer texts, the `TallyActionGroups` regrouping, the end-of-run original-summary edit). I read the changed files in full and checked the diffs.

**Resolution of the earlier findings**

- **WR-01 (claim spent by an undo that changed nothing; shutdown can miss an undo Confirm): mostly resolved.**
  - The "presser administers no group" case, which was the main trigger, is fixed: every group skips before a write, so `ReleaseUndo` gives the claim back and clears the group undo columns in one transaction.
  - The shutdown window is closed. `joinStaffRuns` registers before the claim, the `runStarted` defer returns the registration on every non-start path, and a Confirm after the cancel is refused before it claims.
  - Residual: outcomes that provably made no write (a pacer refusal, a context cancelled while waiting for a slot, a 429) still burn the claim, because `Reached` is set whatever the call returned. See WR-01 below.
- **WR-02 (history and answers say "undone" whatever the undo did; a crashed undo stays pending): resolved for what it named.**
  - `staffUndoStateOf` is the single decision point, and `staff_history_undone_by` was replaced by `staff_history_undo_by` everywhere.
  - A dead undo's pending groups render as interrupted without a write, and the original summary is edited only at the end of the run, from the real count of groups undone.
  - Residual: the replacement wording "changed nothing" claims more than the record knows. See WR-02 below.
- **WR-03 (history page answered before the data is read): still open.** Carried forward as WR-04 below.
- **WR-04 (unmute "left behind" predicate): unchanged, accepted by the owner at UAT.** Carried as IN-02.
- **IN-01, IN-02, IN-03, IN-05, IN-06: still open.** Carried as IN-07, IN-03, IN-04, IN-05 and IN-06. The old IN-04 (50 ms sleeps) is not in this review's scope.

**Checked and found sound**

- **`ReleaseUndo` match conditions.** It matches on `id`, `undo_by`, the exact claim time and `undo_finished_at IS NULL`. `ClaimUndo` returns the stored value, truncated to the microsecond in UTC, which is exact for `TIMESTAMP WITH TIME ZONE`. It clears the parent and all group undo columns in one `db.DB.Transaction`, and `RowsAffected != 1` returns without writing. The tests cover another presser, another claim time, and a finalized claim.
- **`StaffActionRunsWG` pairing.**
  - Early returns before `joinStaffRuns` never Add.
  - After it, the deferred `Done` runs while `runStarted` is false. It covers the shutdown abort, the claim error, a lost claim and a panic before the run starts.
  - Once `runStarted = true`, the coordinator goroutine owns the `Done`. That defer is registered first, so it runs after the target-lock release and the panic recovery.
  - `startStaffRun` still pairs through `joinStaffRuns`.
- **`sweepPending` marking swept groups `Reached`.** A panicked worker keeps the claim (fail closed), and `TestStaffUndoPanicKeepsClaim` proves it.
- **The end-of-run original edit.** It runs from `Delivered` on its own `newStaffDeliverContext` budget, not the cancelled run context. It is skipped when the claim was released.
- **`TallyActionGroups`.** The extra `undo_outcome` grouping only splits rows, and the per-outcome counts stay additive.
- **Locales.** `make check-translations` passes, and `make check-docs` shows no drift. I checked placeholders by script: `{name}` and `{time}` match in all 7 files for every new key. Every key used in Go exists in all 7 locales. `staff_history_undone_by` has no remaining reference.
- **Build and tests.** `go vet -tags testtools ./alita/...` is clean. `go test -tags testtools -race -run 'Undo|History|Release|Tally|Claim'` over `alita/db/staff` and `alita/modules` passes.
- **Test quality.** The new tests use the real SQLite harness, miniredis and the hand-written Telegram fake, and they assert persisted rows, edits and writes. The only sleeps are 5 ms polling loops with a deadline.

No security or data-loss defects. The remaining issues are a still-too-broad "reached" notion, an over-claiming label, one rare record-corrupting retry path and one carried silent failure.

## Warnings

### WR-01: `Reached` is set for outcomes that provably made no write, so the claim is still burned for them

**File:** `alita/modules/staff_undo.go:762-777`, `alita/modules/staff_undo.go:667-682`; `alita/utils/ratelimit/telegram_pacer.go:138-173`
**Issue:** `runStaffUndoInGroup` marks the group `Reached` as soon as `executeStaffUndoCall` returns, whatever it returned. Three results mean Telegram did not change the group, yet they keep the claim:
- `ratelimit.ErrRateLimited` from a pacer refusal ("next slot is X away, over the cap"), which the pacer documents as "no Telegram request is made".
- `ErrRateLimited` after 429s. A 429 is rejected, not applied.
- A context cancelled while the call waits for its pacer slot, or before the HTTP request goes out. `classifyStaffFailure` returns `fail_interrupted` for it.

Consequences:
- A Staff Group member who presses Undo while the fleet pacer is saturated, or just as the bot shuts down with a group waiting for its slot, burns the action's only undo with nothing changed.
- The record then reads "undo changed nothing / cannot be undone again" (`staff_undo_already_nothing`, `staff_history_undo_nothing`), and the original loses its Undo button.
- AGENTS.md says the claim is given back "when the shutdown cut every group off before its write". That holds only for cuts before the call, not for cuts inside the paced call.
- The staff help text promises "nothing is used up" only for an undo that cannot change any group. This is the same class of outcome with a different cause.

**Fix:** Set `Reached` only when a request could have been applied. In `runStaffUndoInGroup`, keep the claim for any other error, but not when the error is definitely no-write:
```go
if err := executeStaffUndoCall(...); err != nil {
    reason, detail := classifyStaffFailure(ctx, b, link.GroupChatID, err)
    res := result(reason, detail)
    res.Reached = !(errors.Is(err, ratelimit.ErrRateLimited) || (ctx.Err() != nil && errors.Is(err, ctx.Err())))
    return res
}
```
Also keep `Reached` true for a timeout, which may have been applied. Add a test with a pacer refusal and one with a shutdown during the slot wait. Update the AGENTS.md sentence to match.

### WR-02: "Undo changed nothing" is shown for groups whose outcome is unknown or probably applied

**File:** `alita/modules/staff_undo.go:702-714` (marker choice), `alita/modules/staff_undo.go:118-131` (`staffUndoNothing`), `alita/modules/staff_undo_claim_test.go:162-193`
**Issue:**
- `staffUndoStateOf` and the `Delivered` hook equate `undone == 0` with "changed nothing". The groups behind it can include `fail_internal` (a worker that panicked in or after the write), `fail_interrupted` (cancelled mid-call), and `fail_telegram` after a timeout, where the request may have been applied.
- `TestStaffUndoPanicKeepsClaim` asserts exactly this. Its own comment says "the run never learns whether the write happened", yet it requires the permanent original summary to read "Undo by X changed nothing" with no Undo button, and the history to say "undo changed nothing / cannot be undone again".
- This is the reverse of the earlier WR-02: the audit trail now asserts a negative it cannot verify, and tells staff there is nothing to check.

**Fix:** Make the third state "attempted, nothing confirmed". Count groups that failed with an unknown-effect reason (`fail_internal`, `fail_interrupted`, a timeout `fail_telegram`) and use the neutral wording ("undo tried, result not confirmed, check the groups") for them. Reserve "changed nothing" for undos whose every attempted group failed with a definite Telegram rejection (400, 403), or whose every group skipped. This needs `staffUndoStateOf` to receive a second count, and a key in all 7 locales.

### WR-03: A retry after an ambiguous `ReleaseUndo` error can run `FinalizeUndo` on an already released record

**File:** `alita/modules/staff_undo.go:667-682`, `alita/db/staff/actions.go:308-345`
**Issue:** `Finish` calls `ReleaseUndo` up to twice. If the first call's transaction committed but returned an error (a lost connection after COMMIT), the second call finds no matching claim and returns `(false, nil)`. The `default` branch logs "no longer matched; keeping it" and falls through to `FinalizeUndo`. That function writes the groups' undo results and sets `undo_finished_at` on a record whose `undo_started_at` is now NULL. Afterwards:
- `staffActionUndoable` is true (no claim) and the record has a stale `undo_finished_at`.
- The next claim reads as finished at once, so `staffUndoStateOf` says "undone" or "changed nothing" while the new run is going.
- The new run's own `ReleaseUndo` never matches, because it needs `undo_finished_at IS NULL`.

It is rare, but the code comments call this path fail-closed, and it is not.

**Fix:** After an errored first attempt, do not treat `(false, nil)` as "claim kept". Re-read the record: if `undo_started_at` is NULL or differs from `claimedAt`, treat the claim as released and return without finalizing. Alternatively, make `FinalizeUndo` conditional on the claim (`WHERE id = ? AND undo_started_at IS NOT NULL`) and fail when the parent matched no row.

### WR-04: A history page press is answered before the data is read, so a database error is a silent no-op (carried forward; was WR-03)

**File:** `alita/modules/staff_history.go:307-325`
**Issue:** Unchanged by this phase's fixes. `staffHistoryList` calls `answerStaffCallback(b, query, "", false)` at line 307, then reads `ListActionsFresh` and `TallyActionGroups`. On error it only logs and returns `ext.EndGroups`. The user gets an empty toast and the message does not change. `staffHistoryDetail` reads first and answers after, so the two handlers disagree.
**Fix:** Read both first and answer once per path:
```go
rows, err := staff.ListActionsFresh(chat.Id, offset, staffHistoryPageSize+1)
if err == nil { tallies, err = staff.TallyActionGroups(ids) }
if err != nil {
    log.Errorf(...)
    text, _ := tr.GetString("staff_check_failed")
    answerStaffCallback(b, query, text, true)
    return ext.EndGroups
}
answerStaffCallback(b, query, "", false)
```

## Info

### IN-01: The original summary is marked "see the reply" even when the undo's own summary was never delivered

**File:** `alita/modules/staff_undo.go:702-715`
**Issue:** `Delivered` ignores its `landedMsgID`. When delivery failed entirely (`landed == 0`), the original is still edited to "Undone by X, see the reply" with its Undo button removed, and there is no reply. The record is unaffected.
**Fix:** Take `landed` and skip the edit when it is 0, or use marker text that does not point to a reply.

### IN-02: The "left behind" predicate for an unmute accepts any non-muted state (carried forward; was WR-04, accepted by the owner at UAT)

**File:** `alita/modules/staff_action_decide.go:375-381`
**Issue:** Unchanged and out of this round's file set. Undo can overwrite another admin's newer partial restriction on an unmuted target.
**Fix:** Recorded as accepted; if it is ever reopened, persist the applied permission set and compare it.

### IN-03: `ClaimUndo` is not bound to the Staff Group in SQL (carried forward; was IN-02)

**File:** `alita/db/staff/actions.go:280-298`
**Issue:** `WHERE id = ? AND undo_started_at IS NULL` does not bind `staff_chat_id`. The callers check it, so this is defense in depth only. `ReleaseUndo` is bound by `undo_by` and the claim time.
**Fix:** Add a `staffChatID` parameter and `AND staff_chat_id = ?`.

### IN-04: `RekeyChat` bumps `updated_at`, which now also flips dead undos back to "running" (carried forward; was IN-03)

**File:** `alita/db/staff/rekey.go:53`
**Issue:** `staffUndoStateOf` and `staffHistoryState` use `updated_at` as the heartbeat. The re-key writes `updated_at = now` on every record of the old chat, so for 30 minutes after a Staff Group migration every crashed action or undo reads "running" instead of "interrupted". The new undo states make the effect wider.
**Fix:** Use `UpdateColumn` on `staff_chat_id` alone (no `updated_at`) for `staff_actions`.

### IN-05: The history timestamp format is English-only in every locale (carried forward)

**File:** `alita/modules/staff_history.go:136`, `alita/modules/staff_history.go:372`, `alita/modules/staff_history.go:389`
**Issue:** `Format("2 Jan 15:04")` prints English month abbreviations in all 7 locales.
**Fix:** Use a numeric format such as `2006-01-02 15:04`.

### IN-06: The Prev offset ignores a page that was shrunk to fit the length cap (carried forward)

**File:** `alita/modules/staff_history.go:197`
**Issue:** Pages start at arbitrary offsets after a shrink, and Prev always steps back 10, so entries can appear on two pages. No entry is hidden.
**Fix:** Carry the previous page's start in the callback, or document the overlap.

### IN-07: `staffCallRestore` is defined by arithmetic outside the iota block (carried forward; was IN-01)

**File:** `alita/modules/staff_action_decide.go:300`
**Issue:** `staffCallUnmute + 1` would collide with any call later added to the iota block.
**Fix:** Move it into the iota block and comment that only `decideStaffUndo` returns it.

---

_Reviewed: 2026-10-06_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
