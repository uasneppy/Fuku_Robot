---
phase: 03-staff-audit-and-undo
verified: 2026-10-06T12:00:00Z
status: passed
score: 12/12 must-haves verified
covered_files:
  - .planning/phases/03-staff-audit-and-undo/03-01-PLAN.md
  - .planning/phases/03-staff-audit-and-undo/03-01-SUMMARY.md
  - .planning/phases/03-staff-audit-and-undo/03-02-PLAN.md
  - .planning/phases/03-staff-audit-and-undo/03-02-SUMMARY.md
  - .planning/phases/03-staff-audit-and-undo/03-03-PLAN.md
  - .planning/phases/03-staff-audit-and-undo/03-03-SUMMARY.md
  - .planning/phases/03-staff-audit-and-undo/03-04-PLAN.md
  - .planning/phases/03-staff-audit-and-undo/03-04-SUMMARY.md
  - .planning/phases/03-staff-audit-and-undo/03-05-PLAN.md
  - .planning/phases/03-staff-audit-and-undo/03-05-SUMMARY.md
  - .planning/phases/03-staff-audit-and-undo/03-06-PLAN.md
  - .planning/phases/03-staff-audit-and-undo/03-06-SUMMARY.md
  - .planning/phases/03-staff-audit-and-undo/03-07-PLAN.md
  - .planning/phases/03-staff-audit-and-undo/03-07-SUMMARY.md
  - .planning/phases/03-staff-audit-and-undo/03-08-PLAN.md
  - .planning/phases/03-staff-audit-and-undo/03-08-SUMMARY.md
  - .planning/phases/03-staff-audit-and-undo/03-09-PLAN.md
  - .planning/phases/03-staff-audit-and-undo/03-09-SUMMARY.md
  - .planning/phases/03-staff-audit-and-undo/03-10-PLAN.md
  - .planning/phases/03-staff-audit-and-undo/03-10-SUMMARY.md
  - .planning/phases/03-staff-audit-and-undo/03-11-PLAN.md
  - .planning/phases/03-staff-audit-and-undo/03-11-SUMMARY.md
  - .planning/phases/03-staff-audit-and-undo/03-12-PLAN.md
  - .planning/phases/03-staff-audit-and-undo/03-12-SUMMARY.md
  - alita/db/models/staff_action.go
  - alita/db/staff/actions.go
  - alita/db/staff/rekey.go
  - alita/modules/staff.go
  - alita/modules/staff_action_card.go
  - alita/modules/staff_action_decide.go
  - alita/modules/staff_action_record.go
  - alita/modules/staff_action_run.go
  - alita/modules/staff_action_summary.go
  - alita/modules/staff_history.go
  - alita/modules/staff_log.go
  - alita/modules/staff_panel.go
  - alita/modules/staff_undo.go
  - alita/utils/actionlog/actionlog.go
  - migrations/20261005120000_add_staff_actions.sql

covered_digest: "v2:sha256:0aeff01275aaacd9ea42953425a0793a65a865fd6640f3a5ea92d074e1debcea"
behavior_unverified: 0
overrides_applied: 1
overrides:
  - must_have: "D-02/D-04 undo restores exactly the recorded prior state and never overrules a later decision (unmute undo uses the broad left-behind predicate)"
    reason: "Owner accepted the documented broad left-behind predicate for an unmute undo at UAT test 3 (option a); no gap plan."
    accepted_by: "owner (03-UAT.md test 3)"
    accepted_at: "2026-10-06T09:10:23Z"
re_verification:
  previous_status: human_needed
  previous_score: 4/6
  gaps_closed:
    - "G-03-4: the one-undo claim is spent by an undo that changed nothing (03-10, WR-01 main trigger)"
    - "G-03-4: the shutdown can miss an undo Confirm, and a Confirm after the shutdown claims and runs (03-11)"
    - "G-03-4: history, answers and the original summary say 'undone' whatever the undo did; a crashed undo shows pending (03-12, WR-02)"
  gaps_remaining: []
  regressions: []
behavior_unverified_items: []
human_verification:
  - test: "Re-run UAT test 4 live (03-12 plan asks for it). In a test Staff Group with two linked test supergroups, (a) have a Staff Group member who is an admin in neither group press Undo on a finished ban and Confirm; (b) then have a member with restrict rights press Undo on the same summary, Confirm, and let it finish; (c) /staff, Recent actions, and the detail view."
    expected: "(a) Every group is skipped, the undo's summary says no group was changed and the action can still be undone, the original summary keeps its text and its Undo button, and history shows no undo. (b) The Undo button still works and the ban is lifted in both groups; the original then reads 'Undone by <name>' and loses its button; history shows the entry as undone. Also trigger an undo that fails in every group (for example remove the bot's restrict right first): the original reads 'Undo by <name> changed nothing' and the entry reads 'undo changed nothing'. Stopping the bot mid-undo and restarting shows 'undo interrupted' and groups as 'interrupted by restart', not a stuck hourglass."
    why_human: "The claim, release and label paths were proven only against the real SQLite database, miniredis and the hand-written Telegram fake. How real Telegram answers (permission errors, retry_after, a restart mid-run) can't be reproduced by a fake. UAT test 1 (restore on live Telegram) passed before gap closure; the restore call itself (decideStaffUndo, executeStaffUndoCall) is unchanged by 03-10..03-12, but the claim lifecycle around it is new."
  - test: "make lint on a go1.26 toolchain (or CI)"
    expected: "No new lint findings in the Phase 3 files."
    why_human: "The installed golangci-lint was built with go1.25 and refuses this module (go 1.26.0). UAT test 5 is still skipped. AGENTS.md and ROADMAP require it at the end of every phase."
  - test: "OPTIONAL DECISION: pacer-refused, 429 and mid-wait-cancelled calls count as 'reached' (code review WR-01)"
    expected: "Owner decides whether to accept the rule as written and documented (the claim is kept whenever a group's write call was attempted, whatever it returned, which fails closed), or to open a small follow-up plan so that outcomes that provably made no write (ratelimit.ErrRateLimited, a context cancelled while waiting for a slot) give the claim back."
    why_human: "The code, the plan prohibition ('whatever that call returned'), AGENTS.md and the staff docs all say the same thing, so it is not a breach of any must-have. It is narrower than the literal wording of owner decision b in a rare case (a saturated fleet pacer, or a shutdown during a slot wait), so the owner should confirm."
  - test: "OPTIONAL DECISION: 'changed nothing' wording for groups whose write may have applied (code review WR-02)"
    expected: "Owner decides whether 'Undo by X changed nothing' is acceptable when the only non-skipped groups ended in a panic, an interruption or a timeout, or whether those get a neutral 'tried, result not confirmed, check the groups' wording."
    why_human: "Owner decision b literally says 'otherwise say nothing changed', and plan 03-10 specifies the wording for the all-tried-and-failed case, so the code matches the decision. The over-claim only affects groups whose outcome is unknown (the code review shows TestStaffUndoPanicKeepsClaim asserts it)."
---

# Phase 3: Staff Audit and Undo Verification Report

**Phase Goal:** As a staff member, I want to log, list and undo every staff action, so that mistakes can be reversed and we stay accountable.
**Verified:** 2026-10-06T12:00:00Z
**Status:** human_needed
**Re-verification:** Yes, after gap closure (plans 03-10, 03-11, 03-12 against G-03-4)

## Goal Achievement

Gap G-03-4 is closed. I read the merged code rather than the summaries. The four roadmap criteria still hold and the three gap-closure plans deliver what their `must_haves` say. I found no regression against the earlier must-haves, and no blocker. The status stays `human_needed` for two reasons: the plan for 03-12 asks for UAT test 4 to be re-run on live Telegram, and `make lint` has never run. Two optional owner decisions arise from the code review (WR-01 and WR-02), and neither breaches a must-have.

I did not trust the summaries or the 03-REVIEW.md text. I checked each point against the source, and ran the Phase 3 test families again with the race detector.

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | SC1: every group where a staff action was applied gets a log-channel post with action, target, issuer and reason | ✓ VERIFIED (regression check) | `staff_action_run.go:376-381` posts only for `staffOutcomeDone`, after the group result is saved. The log tests (`TestStaffActionLog*`) pass under `-race`. Undo posts go through `postStaffUndoLog` (`staff_undo.go:653-655`). |
| 2 | SC2: every staff action is recorded with issuer, target, action, duration, reason, time and per-group outcomes; it survives restarts | ✓ VERIFIED (regression check) | Migration `20261005120000_add_staff_actions.sql` is still the highest migration file and is unchanged by the gap closure (the diff since `d45631a` touches no migration). `StopStaffActions` is registered in `main.go:218`. `TestStaffActionRecord*` and `TestStopStaffActionsRecordsInterrupted` pass. |
| 3 | SC3: `/staff` lists recent staff actions with who, what, target, when, reason and the outcome in each group | ✓ VERIFIED | `staffHistoryLine` and `renderStaffHistoryDetail` (`staff_history.go:72-139`, `335-440`). The earlier caveat (a claimed undo read "undone") is gone: the state now comes from `staffUndoStateOf` and the tally's `UndoDone` (`actions.go:196-234`). `TestStaffHistory*` pass. |
| 4 | SC4: "Undo everywhere" reverses the action in each group where the presser is a Staff Group member and a restrict-rights admin; others are skipped with the reason; same summary; an outsider can't undo | ✓ VERIFIED against the fake | `runStaffUndoInGroup` runs `staffGroupPrechecks` with the presser as actor, then `decideStaffUndo`, then `executeStaffUndoCall` (`staff_undo.go:726-778`). Access, presser-rights, outsider, once, target-lock and rekey tests pass. The restore call path is unchanged by the gap closure (`staff_action_decide.go` has no diff since `d45631a`). |
| 5 | D-02/D-04: undo restores the recorded prior state and never overrules a later decision | ✓ PASSED (override) | Verified for ban, mute and unban. The unmute column keeps the broad left-behind predicate (`staff_action_decide.go:375-381`). The owner accepted it at UAT test 3, so I record it as an override instead of a gap. |
| 6 | Live Telegram semantics of restore and re-ban (research A2, A3) | ✓ VERIFIED by owner UAT (human evidence) | `03-UAT.md` test 1 records `result: pass` for the live restore check and test 2 for the log-channel posts. I cannot reproduce live Telegram myself. The restore code path is unchanged since, so the earlier pass still applies; the new claim lifecycle needs its own live pass (human item 1). |
| 7 | G-03-4 / 03-10: a Staff Group member who is an admin in no linked group presses Undo and Confirm; no write is made, the claim is given back, and a member with rights can retry | ✓ VERIFIED | `Finish` calls `staff.ReleaseUndo(a.ID, card.Issuer, claimedAt)` when `!anyStaffGroupReached(results)` (`staff_undo.go:667-678`). `ReleaseUndo` (`actions.go:308-345`) matches the record ID, `undo_by`, the exact claim time and `undo_finished_at IS NULL`, then clears the parent and every group's undo columns in one `db.DB` transaction. `TestStaffUndoClaimReleased` and `TestStaffActionReleaseUndo` pass. |
| 8 | 03-10: an undo that gave its claim back never edits the original; the undo summary says nothing changed and the action can still be undone | ✓ VERIFIED | `Delivered` returns at once when `released` (`staff_undo.go:705-707`); `staffUndoSummaryHeader(..., final && released)` adds `staff_undo_released_note` (`staff_undo.go:586-596`). Covered by `TestStaffUndoClaimReleased`. |
| 9 | 03-10 / D-09: once any group reached its write, the claim is kept; a panicked group counts as reached; "Undone by" only at the end of the run and only if at least one group was undone | ✓ VERIFIED | `res.Reached = true` is set in `reached()` after the write call returns (`staff_undo.go:767-777`); `sweepPending` sets it for swept groups (`staff_action_run.go:154`); `Delivered` picks `staff_undo_marker` or `staff_undo_marker_nothing` from `undone` (`staff_undo.go:710-714`). `TestStaffUndoNothingChanged`, `TestStaffUndoPanicKeepsClaim` and `TestStaffUndoTracer` pass. |
| 10 | 03-11: an undo Confirm joins the shutdown drain before it claims; a Confirm after the shutdown claims nothing; the wait group stays balanced | ✓ VERIFIED | `joinStaffRuns()` runs before the claim (`staff_undo.go:459`). A deferred `staffActionRunsWG.Done()` covers every path where `runStarted` stays false (`staff_undo.go:460-464`). A cancelled run context aborts with `staff_undo_abort_restarting` before any claim (`staff_undo.go:467-470`). The started run's coordinator owns the matching `Done` (`staff_action_run.go:253-257`). `startStaffRun` keeps its old behaviour through the same pair (`staff_action_run.go:244-246`). `TestStaffUndoShutdownWindow`, `TestStaffUndoConfirmAfterStop` and `TestStopStaffActionsUndo` pass. |
| 11 | 03-12: Recent actions, the detail view and the Ask and Confirm answers derive an undo's state from stored outcomes (running, interrupted, undone, changed nothing), never from the claim alone | ✓ VERIFIED | `staffUndoStateOf` (`staff_undo.go:118-131`) is the only decision point; `staffHistoryState`, the detail header and `staffUndoClaimedText` all call it (`staff_history.go:72-139, 373, 382`; `staff_undo.go:57-73, 267, 438`). `ActionTally.UndoDone` is filled by the same single query (`actions.go:196-234`). `TestStaffHistoryUndoStates`, `TestStaffHistoryUndoNothingLine`, `TestStaffActionTally` and `TestStaffUndoClaimedTexts` pass. |
| 12 | 03-12: a crashed undo (claimed, unfinished, heartbeat older than `staffTargetLockTTL`) shows its unfinished groups as interrupted and viewing it writes nothing | ✓ VERIFIED | The conversion runs only on the in-memory `undoResults` when `state == staffUndoInterrupted` (`staff_history.go:383-401`); nothing in the view writes. `TestStaffHistoryDetailUndoOutcome` (crashed case checks the row's `UndoOutcome` stays empty) passes. |

**Score:** 12/12 truths verified (1 by override, 1 by owner's recorded live UAT, 0 present-but-behavior-unverified)

### Is G-03-4 closed?

Yes. The gap listed eight missing items. Each one exists in the merged code:

| Missing item from UAT | Where it is now |
|-----------------------|-----------------|
| Record per group that the run reached its write | `staffGroupResult.Reached` (`staff_action_run.go:31-34`), set in `runStaffUndoInGroup` and `sweepPending` |
| Release the claim when no group reached a write, scoped to the claim | `staff.ReleaseUndo` in `Finish` (`staff_undo.go:667-678`) while the run still holds the target lock |
| Refuse to claim after the shutdown began; register before the claim | `joinStaffRuns` and the `runCtx.Err()` check (`staff_undo.go:455-470`) |
| Count undo outcomes; four states | `ActionTally.UndoDone`; `staffUndoStateOf` |
| Move the original's edit to the end of the run, with a "changed nothing" marker and no edit when released | `Delivered` hook (`staff_undo.go:702-715`) |
| "Already" text distinguishes running, undone, changed nothing | `staffUndoClaimedText` |
| Detail view shows a dead undo's pending groups as interrupted, display only | `staff_history.go:383-401` |
| Locale keys in 7 files, AGENTS.md and docs, regression tests | All ten new keys exist in all 7 locale files (grep count 7 each), `make check-translations` passes, AGENTS.md lines 24-32, 82-83, 105-111 and 198-203 and the staff docs page state the rule, and the named tests exist and pass |

`staff_history_undone_by` has no remaining reference in `alita/` or `locales/`.

### Judgment on the code review's new warnings

| Finding | Is it real in the source? | Contradicts a must-have or the owner's decision? | Classification |
|---------|---------------------------|--------------------------------------------------|----------------|
| WR-01: `Reached` is set whatever `executeStaffUndoCall` returned | Yes. `reached()` wraps both the error and the success return (`staff_undo.go:767-777`). `staffPaced` returns `ErrRateLimited` without a Telegram request when the slot is over `MaxWait` (`telegram_pacer.go:152-173`), and a cancelled context during the slot wait returns `ctx.Err()`; both then keep the claim. No test covers a pacer refusal. | Not a must-have breach. 03-10's prohibition says the claim must not be given back once a group reached a write "whatever that call returned"; AGENTS.md (lines 105-111) and the docs say the same, and the error direction is fail closed. It is narrower than owner decision b's wording only in a rare case. AGENTS.md line 31-32 ("when the shutdown cut every group off before its write") over-states it slightly for a cut inside a paced call. | Warning, optional owner decision (human item 3). Not a gap. |
| WR-02: "changed nothing" is shown when the unfinished groups ended in a panic, interruption or timeout | Yes. `Delivered` and `staffUndoStateOf` treat `undone == 0` as "changed nothing"; `TestStaffUndoPanicKeepsClaim` asserts the original reads `staff_undo_marker_nothing` after a panic whose write may have applied. | No. Owner decision b says "otherwise say nothing changed", and 03-10 specifies the marker for the all-tried-and-failed case. It over-claims for groups whose effect is unknown. | Warning, optional owner decision (human item 4). Not a gap. |
| WR-03: after an ambiguous `ReleaseUndo` error, the retry returns `(false, nil)` and the code falls through to `FinalizeUndo` | Yes. The loop breaks on `err == nil` (`staff_undo.go:669-673`); the `default` case only logs and falls through (`staff_undo.go:679-681`); `FinalizeUndo` then sets `undo_finished_at` with no condition on the claim (`actions.go:406-409`), so a record with no claim and a stale finish time results. | No must-have covers a commit-then-error on the first attempt. It needs a double fault (a transaction that committed but returned an error, then a retry). It does not affect any scenario the owner reported. | Warning, recommended small follow-up (make `FinalizeUndo` conditional on the claim, or re-read the record after an errored first attempt). Not a gap. |
| WR-04 (was WR-03): history page press answered before the data is read | Yes, unchanged at `staff_history.go:307-325`. | No. Pre-existing, outside G-03-4, and the owner did not raise it. | Warning, carried forward. |

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `alita/db/staff/actions.go` | `ClaimUndo` returns the claim time; `ReleaseUndo`; `ActionTally.UndoDone` | ✓ VERIFIED | `ClaimUndo` truncates to the microsecond UTC (`actions.go:280-298`); `ReleaseUndo` at `308-345`; the tally at `172-234`. Never cached, so no `DeleteCache` applies. |
| `alita/modules/staff_action_run.go` | `Reached`, `joinStaffRuns`, `startJoinedStaffRun` | ✓ VERIFIED | `staff_action_run.go:31-34, 231-256`. |
| `alita/modules/staff_undo.go` | `staffUndoStateOf`, `staffUndoClaimedText`, release in `Finish`, end-of-run original edit, drain join | ✓ VERIFIED | See Truths 7-12. |
| `alita/modules/staff_history.go` | four-state segment and display-only conversion | ✓ VERIFIED | `staff_history.go:72-139, 373-401`. |
| `alita/modules/staff_undo_claim_test.go`, `staff_undo_shutdown_test.go`, `staff_history_test.go`, `actions_test.go` | named regression tests | ✓ VERIFIED | All 13 named tests exist, use the real SQLite harness, miniredis and the hand-written Telegram fake, and pass. |
| `locales/*.yml` (7), `AGENTS.md`, `docs/.../commands/staff/index.md` | new keys, rules, help text | ✓ VERIFIED | Grep shows 7 of 7 locales for every new key; `make check-translations` passes. |

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|-----|--------|---------|
| `startStaffUndoRun` Finish | `staff.ReleaseUndo` | presser and `claimedAt` returned by `ClaimUndo` | WIRED | `staff_undo.go:670`. |
| `runStaffUndoInGroup` | `anyStaffGroupReached` in Finish | `Reached` marker on the result | WIRED | `staff_undo.go:767-777, 599-608`. |
| `staffUndoConfirm` | `joinStaffRuns`, `startJoinedStaffRun` | registered before the claim, same context passed on | WIRED | `staff_undo.go:459, 508, 640`. |
| `StopStaffActions` | undo runs | same `staffActionRunsWG` | WIRED | `staff_action_run.go:85`; `main.go:218`. |
| `staffHistoryLine` / detail / Ask / Confirm | `staffUndoStateOf` | one decision point | WIRED | `staff_history.go:74, 138, 373, 382`; `staff_undo.go:63`. |
| `TallyActionGroups` | `ActionTally.UndoDone` | grouping by `undo_outcome` | WIRED | `actions.go:207-231`. |
| Earlier links (record create, prior-state write-ahead, log posts, `staffCallback` routes, panel button) | | | WIRED (regression check) | Tests for all of them pass. |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
|----------|---------------|--------|--------------------|--------|
| History list state | `t.UndoDone` | SQL `GROUP BY action_id, outcome, undo_outcome` | Yes | ✓ FLOWING |
| Detail undo block | `groups[].UndoOutcome`, `a.UndoStartedAt/FinishedAt/UpdatedAt` | `GetActionFresh`, `ListActionGroupsFresh` | Yes | ✓ FLOWING |
| Undo Finish | `results[].Reached` | set after the real write call, or by the panic sweep | Yes | ✓ FLOWING |
| Original-summary marker | `undone` | `staffSummaryTally(results)` | Yes | ✓ FLOWING |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| Phase 3 test families, including the 13 gap-closure tests | `CGO_ENABLED=1 go test -tags testtools -race -count=1 -run '^TestStaff(Undo\|History\|ActionLog\|ActionRecord\|ActionTally\|ActionReleaseUndo)\|^TestStopStaffActions' ./alita/modules ./alita/db/staff` | `ok alita/modules 36.6s`, `ok alita/db/staff 1.1s` | ✓ PASS |
| Production build | `CGO_ENABLED=0 go build ./...` | exit 0 | ✓ PASS |
| Locale parity | `make check-translations` | "All translations are present!" | ✓ PASS |
| Debt markers in the changed source | grep for TBD, FIXME, XXX in `staff_undo.go`, `staff_history.go`, `actions.go`, `staff_action_run.go` | none | ✓ PASS |
| Full suite | not re-run by me, per the single-run rule; the orchestrator's `make test` (-race, -tags testtools) on head `4f9ce77` exited 0 with 60 packages ok | accepted as supplementary evidence | n/a |

The behavior-dependent truths (claim given back, claim kept after a panic, the shutdown window, a Confirm after the shutdown, a crashed undo reading interrupted) each have a named test that exercises the transition, and those tests passed in my run. None of them rests on symbol presence.

### Probe Execution

Step 7c: SKIPPED. The phase declares no `probe-*.sh` scripts and none exist.

### Requirements Coverage

| Requirement | Source Plans | Description | Status | Evidence |
|-------------|--------------|-------------|--------|----------|
| STAFF-09 | 03-03, 03-08 | Each applied action and its reason posted to the log channel of every group where it was applied | ✓ SATISFIED | Truth 1; live log-channel check passed at UAT test 2 |
| STAFF-10 | 03-01, 03-12 | Every staff action recorded with issuer, target, action, duration, reason, time, per-group outcomes | ✓ SATISFIED | Truth 2 |
| STAFF-11 | 03-02, 03-06..03-12 | "Undo everywhere" with per-group live checks and the same summary | ✓ SATISFIED in code; live re-run of the new claim lifecycle pending (human item 1) | Truths 4, 5, 7-12 |
| SETUP-09 | 03-04, 03-05, 03-09, 03-12 | `/staff` lists recent actions with who, what, target, when, reason, per-group outcome | ✓ SATISFIED | Truths 3, 11, 12 |

All four IDs appear in plan frontmatter and in REQUIREMENTS.md. Nothing in REQUIREMENTS.md maps to Phase 3 without a plan, so there are no orphans. REQUIREMENTS.md still shows the four boxes unchecked and the traceability rows "Pending". Update them when the phase closes.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| `alita/modules/staff_undo.go` | 667-682 | Falls through to `FinalizeUndo` after an ambiguous release (WR-03) | ⚠️ Warning | Rare double fault leaves a record with no claim and a stale finish time. |
| `alita/modules/staff_undo.go` | 762-777 | `Reached` set for pacer refusals, 429s and mid-wait cancels (WR-01) | ⚠️ Warning | Keeps the claim in a rare case where nothing was written. Fails closed. |
| `alita/modules/staff_undo.go` | 702-714 | "Changed nothing" for groups of unknown effect (WR-02) | ⚠️ Warning | Over-claims for panicked or interrupted groups. |
| `alita/modules/staff_undo.go` | 702-715 | `Delivered` ignores `landedMsgID`, so the original says "see the reply" when no reply landed (review IN-01) | ℹ️ Info | The record is unaffected. |
| `alita/modules/staff_history.go` | 307-325 | History press answered before the read (WR-04, carried) | ⚠️ Warning | A database error on a page press is a silent no-op. |
| `alita/db/staff/rekey.go` | 53 | Re-key bumps `updated_at`, which flips dead undos to "running" for 30 minutes (review IN-04) | ℹ️ Info | The undo states widen this existing effect. |
| `alita/modules/staff_history.go` | 136, 372, 389 | English-only timestamp format; Prev ignores a shrunk page; `ClaimUndo` not bound to the Staff Group in SQL; `staffCallRestore` arithmetic (review IN-03, IN-05..IN-07) | ℹ️ Info | Carried forward, no effect on any criterion. |
| Phase 3 files | - | TBD, FIXME, XXX | none found | |

### Human Verification Required

See the `human_verification` frontmatter. In short:

1. **Re-run UAT test 4 on live Telegram** (the 03-12 plan asks for it): the no-rights undo gives the claim back, a member with rights can retry, the labels read undone, changed nothing or interrupted correctly, and a restart mid-undo shows interrupted.
2. **`make lint`** on a go1.26 toolchain or in CI (UAT test 5, still skipped).
3. **Optional decision on WR-01:** accept "reached means the write call was attempted" or open a small follow-up for pacer refusals and mid-wait cancels.
4. **Optional decision on WR-02:** accept "changed nothing" for unknown-effect groups or add a neutral wording.

### Gaps Summary

There are no gaps. G-03-4 is closed, no earlier must-have regressed, and every requirement ID is accounted for. What remains needs a person: a live Telegram re-run of the gap-closure flow, a lint run on a go1.26 toolchain, and two optional wording and edge-case decisions that the code, plans and docs already agree on. WR-03 (retry after an ambiguous release) and WR-04 (silent history press error) are worth folding into a small follow-up, but neither blocks the phase.

---

_Verified: 2026-10-06T12:00:00Z_
_Verifier: Claude (gsd-verifier)_
