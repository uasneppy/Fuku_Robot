---
phase: 03-staff-audit-and-undo
reviewed: 2026-10-05T00:00:00Z
depth: standard
files_reviewed: 42
files_reviewed_list:
  - AGENTS.md
  - alita/db/models/staff_action.go
  - alita/db/staff/actions.go
  - alita/db/staff/actions_test.go
  - alita/db/staff/rekey.go
  - alita/db/staff/rekey_test.go
  - alita/db/staff/testmain_test.go
  - alita/db/testmain_test.go
  - alita/modules/staff.go
  - alita/modules/staff_action_card.go
  - alita/modules/staff_action_decide.go
  - alita/modules/staff_action_fake_test.go
  - alita/modules/staff_action_record.go
  - alita/modules/staff_action_record_test.go
  - alita/modules/staff_action_run.go
  - alita/modules/staff_action_summary.go
  - alita/modules/staff_action_test.go
  - alita/modules/staff_action_undo_decide_test.go
  - alita/modules/staff_helpers_test.go
  - alita/modules/staff_history.go
  - alita/modules/staff_history_test.go
  - alita/modules/staff_history_undo_test.go
  - alita/modules/staff_log.go
  - alita/modules/staff_log_test.go
  - alita/modules/staff_panel.go
  - alita/modules/staff_panel_render_test.go
  - alita/modules/staff_undo.go
  - alita/modules/staff_undo_lifecycle_test.go
  - alita/modules/staff_undo_log_test.go
  - alita/modules/staff_undo_restore_test.go
  - alita/modules/staff_undo_test.go
  - alita/modules/test_harness_test.go
  - alita/utils/actionlog/actionlog.go
  - docs/src/content/docs/commands/staff/index.md
  - locales/en.yml
  - locales/es.yml
  - locales/fr.yml
  - locales/hi.yml
  - locales/id.yml
  - locales/pt.yml
  - locales/ru.yml
  - migrations/20261005120000_add_staff_actions.sql
findings:
  critical: 0
  warning: 4
  info: 6
  total: 10
status: issues_found
---

# Phase 03: Code Review Report

**Reviewed:** 2026-10-05
**Depth:** standard
**Files Reviewed:** 42
**Status:** issues_found

## Summary

Reviewed the staff audit record (migration, models, repository), write-ahead prior-state capture, log-channel posts, the Recent actions history and the Undo flow (ask, card, confirm, claim, run, decision table).

Checked and found sound:

- **Record-chat binding:** every press path binds the record to the pressed message's chat. Undo Ask, undo Confirm and history Detail compare `staff_chat_id` to `query.Message` chat, and History List filters by chat.
- **Forged data:** callback data carries only numeric IDs or a 16-hex token. Tokens are regex-checked, the card's Staff chat is compared, `Kind` is checked in both Confirm handlers, and the issuer is compared inside the Lua compare-and-set.
- **Per-group authority:** it is read live through `staffGroupPrechecks` with the presser as actor. `decideStaffUndo` keeps the "never lift a ban by accident" invariant: restore or re-ban reaches a kicked or left target only when the live state equals what the staff action left.
- **Migration:** it parses with the repo's statement splitter (line comments, with `;` and `'` inside them, are handled) and matches the models column for column. Locale placeholders are consistent across all 7 files, which I checked with a script.
- **Build and tests:** `go build`, `go vet -tags testtools`, `make check-translations` and `go test -tags testtools` over `alita/db/...`, `alita/i18n` and the staff, undo and history tests in `alita/modules` all pass.

No security or data-loss defects found. The remaining issues are about the one-shot undo claim, misleading status after an undo, one silent failure path and a loose "left behind" predicate.

## Warnings

### WR-01: The one-shot undo claim is consumed even when no group was (or could be) undone

**File:** `alita/modules/staff_undo.go:394-428` (claim), `alita/db/staff/actions.go:270-285` (`ClaimUndo`), `alita/modules/staff_undo.go:549-556` (finalize)
**Issue:** `ClaimUndo` sets `undo_started_at` permanently, before any group is attempted. `staffActionUndoable` and the Ask handler then refuse any later attempt. The record keeps the claim whatever the run does, so these cases burn it with nothing changed:
- A Staff Group member who is an administrator in none of the linked groups presses Undo and Confirm. Ask and Confirm only require live Staff Group membership (D-06). Every group is then skipped `skip_issuer_not_admin`, and nobody with real authority can undo that action any more.
- A shutdown, a 429 outlasting the pacer, a Redis hiccup or a Telegram outage turns the groups into `fail_*` results. `startStaffRun` uses `staffActionsContext()`, so a Confirm tapped after `StopStaffActions` cancelled the context claims the undo and then marks every group `fail_interrupted` without a single call.
- Confirm runs outside `staffActionRunsWG` until `startStaffRun` calls `Add`. A shutdown between the claim (line 399) and `Add` leaves a claimed undo with no run and no finalize.

The failed groups cannot be retried either: `staffActionUndoable` is false once `UndoStartedAt != nil`. That is the same outcome as a deliberate "exactly once", but here it comes from a no-op or transient failure rather than from a completed undo.

**Fix:** Keep exactly-once for groups that were actually written, but release or narrow the claim when nothing was. Options, cheapest first:
```go
// in Finish, after FinalizeUndo: if no group reached a Telegram write
// (every result is skip_issuer_*/fail_interrupted/fail_rate_limited/fail_lookup/fail_owner_unknown),
// reopen the record so the undo can be asked again:
//   UPDATE staff_actions SET undo_started_at=NULL, undo_by=NULL, undo_by_name='', undo_finished_at=NULL
//   WHERE id=? AND NOT EXISTS (SELECT 1 FROM staff_action_groups WHERE action_id=? AND undo_outcome='done')
```
A sturdier option is a per-group claim: a conditional update `undo_outcome '' -> 'pending'` per group, so groups that were skipped or failed stay undoable by a later press. At minimum, refuse to start the run (before claiming) when `staffActionsContext().Err() != nil`, and require that the presser can restrict members in at least one applied group before the claim is taken.

### WR-02: History and the original summary say "undone" regardless of what the undo did; a crashed undo reads "undone" with ⏳ lines forever

**File:** `alita/modules/staff_history.go:69-80`, `alita/modules/staff_history.go:369-380`, `alita/modules/staff_undo.go:414-416`
**Issue:**
- `staffHistoryState` checks `a.UndoStartedAt != nil` first, so the list line reads "↩ undone" for any claimed undo. That includes one where every group was skipped or failed (see WR-01), and one that is still running.
- `renderStaffHistoryDetail` only converts pending groups to "interrupted" when `a.FinishedAt == nil` (the action's own run). A crashed undo has `undo_started_at` set and `undo_finished_at` NULL. `staffUndoResultsFromRecord` renders its pending groups as ⏳ forever, and the header says "Undone by …".
- `editStaffUndoneOriginal` edits the original summary to "Undone by X, see the reply" before the first group is attempted, so the permanent history message claims an undo that may do nothing.

This is audit-record accuracy: the record is the accountability trail and it reports success it cannot vouch for.

**Fix:**
```go
case a.UndoStartedAt != nil && a.UndoFinishedAt == nil && now.Sub(a.UpdatedAt) < staffTargetLockTTL:
    text, _ = tr.GetString("staff_history_undo_running")
case a.UndoStartedAt != nil && a.UndoFinishedAt == nil:
    text, _ = tr.GetString("staff_history_undo_interrupted")
case a.UndoStartedAt != nil:
    // "undone" only if at least one group's undo_outcome is done; otherwise "undo attempted, nothing changed"
```
Apply the same dead-run conversion of pending undo groups to failed/interrupted in `renderStaffHistoryDetail`, and move the "Undone" marker on the original to the run's final delivery, once at least one group is done. Add the new keys to all 7 locale files.

### WR-03: A history page press is answered before the data is read, so a database error is a silent no-op

**File:** `alita/modules/staff_history.go:295-310`
**Issue:** `staffHistoryList` calls `answerStaffCallback(b, query, "", false)` at line 295, then reads `ListActionsFresh` and `TallyActionGroups`. On error it logs and returns `ext.EndGroups`. The press was already answered with an empty toast and the message is not edited, so the user sees nothing and cannot tell a failure from a slow bot. `staffHistoryDetail` does it correctly (reads, then answers), so the two handlers are inconsistent.

**Fix:** Read first, answer once on the matching path:
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

### WR-04: The "left behind" predicate for an unmute accepts any non-muted state, so undo can overwrite another admin's newer restriction

**File:** `alita/modules/staff_action_decide.go:375-381`
**Issue:** D-04 says undo acts only when the live state is exactly what the staff action left. For `staffKindUnmute` the predicate returns true for any `member`, any `left`, and any `restricted` target that is not fully muted (`!live.Muted`). If another admin applies a partial restriction after the unmute (for example no media), `live` is `restricted` with `CanSendMessages == true`, so `!live.Muted` holds. Undo then restores the older mute through `staffUndoRestore` and replaces the newer decision. A `member` who was kicked and rejoined, or a `left` target who was banned and unbanned, is likewise indistinguishable from "untouched". The ban, mute and unban columns compare end date or status and are tighter.

**Fix:** Record what the unmute applied. Persist the applied permission set (or a hash of it) in the group row, as `prior_permissions` does for the prior state, and require equality in `staffUndoLeftBehind`. At minimum, for a live `restricted` target, require that the live permissions equal the group's default permissions that `resolveUnmutePermissions` produced at action time, instead of accepting `!live.Muted`.

## Info

### IN-01: `staffCallRestore` is defined by arithmetic outside the iota block

**File:** `alita/modules/staff_action_decide.go:300`
**Issue:** `const staffCallRestore staffAPICall = staffCallUnmute + 1` sits apart from the iota block of calls. A new call added after `staffCallUnmute` in the block would silently equal `staffCallRestore`. Go would flag the duplicate case in `executeStaffUndoCall`, but only if that switch contains both.
**Fix:** Add `staffCallRestore` to the iota block, with a comment that only `decideStaffUndo` returns it.

### IN-02: `ClaimUndo` is not bound to the Staff Group in SQL

**File:** `alita/db/staff/actions.go:270-273`
**Issue:** The claim is `WHERE id = ? AND undo_started_at IS NULL`. Chat binding is enforced only by the callers (Ask and Confirm both check `StaffChatID`). A future caller that skips that check could claim another Staff Group's record. This is defense in depth only, not a present defect.
**Fix:** Add a `staffChatID` parameter and `AND staff_chat_id = ?` to the conditional update.

### IN-03: `RekeyChat` bumps `updated_at` on every history row, which can flip dead runs back to "running"

**File:** `alita/db/staff/rekey.go:48-53`
**Issue:** The `staff_actions.staff_chat_id` update writes `updated_at = now` to every row of the old chat. `staffHistoryState` treats `finished_at IS NULL` with `updated_at` younger than the 30-minute lock TTL as "running". After a chat migration, every old crashed record reads "running" for 30 minutes instead of "interrupted".
**Fix:** For this table, update only `staff_chat_id` and leave `updated_at` alone, for example `Update(u.column, newChatID)` with `UpdateColumn`, which skips the timestamp.

### IN-04: Two tests synchronise with a fixed `time.Sleep(50ms)`

**File:** `alita/modules/staff_undo_lifecycle_test.go:572`, `alita/modules/staff_action_record_test.go:423`
**Issue:** Both tests sleep 50 ms and then call `StopStaffActions`, assuming the run has reached its delayed `getChatMember` calls. Under `-race` or a loaded CI host the run may not have started, so the "interrupted" assertions become flaky.
**Fix:** Poll for the observable condition, such as the first delayed `getChatMember` call recorded by the fake, instead of sleeping.

### IN-05: The history timestamp format is English-only in every locale

**File:** `alita/modules/staff_history.go:124`, `alita/modules/staff_history.go:359`, `alita/modules/staff_history.go:376`
**Issue:** `Format("2 Jan 15:04")` emits English month abbreviations, so es, fr, hi, id, pt and ru users see "5 Oct". The rest of the feature is localized.
**Fix:** Use a numeric format such as `2006-01-02 15:04`, or a locale key for the month names.

### IN-06: The Prev offset ignores a page that was shrunk to fit the length cap

**File:** `alita/modules/staff_history.go:197`
**Issue:** Next starts right after the last line shown, so pages can start at offsets that are not multiples of 10. Prev always goes back `staffHistoryPageSize` (10), so from a shrunk page it lands in the middle of the previous page and entries appear on two pages with shifting numbers. No entry is hidden.
**Fix:** Carry the previous page's start in the callback (`"p"` field), or accept the overlap and document it.

---

_Reviewed: 2026-10-05_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
