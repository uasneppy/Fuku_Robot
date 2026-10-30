---
status: complete
phase: 03-staff-audit-and-undo
source: [03-VERIFICATION.md]
started: 2026-10-05T20:35:00Z
updated: 2026-10-06T13:11:11Z
---

## Current Test

[testing complete]

## Tests

### 1. Live Telegram restore check (03-VALIDATION.md Manual-Only rows 1 and 2; 03-09 Task 2 human-check)
expected: Group A is restricted again with text allowed and media blocked, ending at the original time. Group B is banned again until the original end time. In /staff, Recent actions and the entry's detail, both actions show as undone and the detail lists each group's result and undo result. (Research A2, A3: fakes cannot prove live Telegram behaviour.)
result: pass

### 2. Log-channel posts end to end (03-VALIDATION.md Manual-Only row 3)
expected: Each linked group's log channel has a #STAFF_BAN or #STAFF_UNBAN post and a #STAFF_UNDO post naming the issuer and the presser, the target, the reason and "via Staff Group", and never the Staff Group's title or ID.
result: pass

### 3. DECISION: undo of an unmute and D-04 (code review WR-04)
expected: Owner chooses (a) accept the documented broad "left behind" predicate for unmute (member, left, or restricted-but-able-to-send), or (b) open a gap-closure plan that stores what the unmute applied and requires equality. Today staffUndoLeftBehind (alita/modules/staff_action_decide.go:375-381) lets an undo of an unmute re-apply the old mute over a later partial restriction by another admin, or over a target who was kicked and rejoined.
result: pass
decision: "a — accept the documented broad left-behind predicate for an unmute undo (WR-04 accepted, no gap plan)"

### 4. DECISION: undo claim spent before any group is attempted (WR-01) and history says "undone" regardless of effect (WR-02)
expected: Owner chooses whether D-09 ("one undo per action") covers the zero-effect case (a Staff Group member who is an admin in no linked group presses Undo and Confirm, every group is skipped, and nobody can undo that action any more), or whether the claim should be released when no group reached a Telegram write, and whether the history line should say "undone" only when at least one group's undo succeeded (alita/modules/staff_undo.go:399-428, alita/modules/staff_history.go:69-80).
result: pass
resolved_by: "03-10, 03-11, 03-12 (gap G-03-4)"
reverified_by: "test 6, live re-run after gap closure, passed 2026-10-06"
reported: "Decision b: give the undo back when no group reached a Telegram write, so someone with rights can retry; show 'undone' only when at least one group was actually undone (otherwise say nothing changed); a crashed undo shows interrupted, not ⏳ (WR-01 and WR-02)."
severity: major

### 5. make lint on a go1.26 toolchain (or CI)
expected: No new lint findings in the Phase 3 files. The installed golangci-lint was built with go1.25 and cannot run on this module (go 1.26.0).
result: pass
note: "Round 1: skipped (needs a go1.26 build of golangci-lint or CI). Re-opened by the post-gap-closure verification (03-VERIFICATION.md human_verification)."

### 6. Live re-run of test 4 after gap closure (03-10..03-12; 03-12 human-check)
expected: In a test Staff Group with two linked test supergroups. (a) A Staff Group member who is an admin in neither group presses Undo on a finished ban and confirms: every group is skipped, the undo's summary says no group was changed and the action can still be undone, the original summary keeps its text and its Undo button, and history shows no undo. (b) A member with restrict rights then presses Undo on the same summary and confirms: the ban is lifted in both groups, the original reads "Undone by <name>" and loses its button, and history shows the entry as undone. (c) An undo that fails in every group (for example after removing the bot's restrict right): the original reads "Undo by <name> changed nothing" and the entry reads "undo changed nothing". (d) Stopping the bot mid-undo and restarting shows "undo interrupted" and the groups as "interrupted by restart", not a stuck hourglass.
result: pass

### 7. OPTIONAL DECISION: pacer-refused, 429 and mid-wait-cancelled calls count as "reached" (code review WR-01)
expected: Owner chooses (a) accept the rule as written and documented (the claim is kept whenever a group's write call was attempted, whatever it returned, which fails closed), or (b) a small follow-up plan so outcomes that provably made no write (ratelimit.ErrRateLimited, a context cancelled while waiting for a pacer slot) give the claim back.
result: pass
decision: "a — keep the rule as written: the claim is kept whenever a group's write call was attempted, whatever it returned (fails closed; code review WR-01 accepted)"

### 8. OPTIONAL DECISION: "changed nothing" wording for groups whose write may have applied (code review WR-02)
expected: Owner chooses (a) keep "Undo by <name> changed nothing" when the only non-skipped groups ended in a panic, an interruption or a timeout, or (b) use a neutral "tried, result not confirmed, check the groups" wording for those outcomes.
result: pass
decision: "a — keep 'changed nothing' when no group was confirmed undone, including panics, interruptions and timeouts (code review WR-02 accepted)"

## Summary

total: 8
passed: 8
issues: 0
pending: 0
skipped: 0
blocked: 0

## Gaps

- gap_id: G-03-4
  truth: "An undo claim is spent only when at least one group reached a Telegram write; history and the original summary say 'undone' only when at least one group was actually undone, and a crashed undo shows interrupted instead of pending"
  status: resolved
  resolved_by: 03-10-PLAN.md, 03-11-PLAN.md, 03-12-PLAN.md
  resolved_at: 2026-10-06
  reason: "User reported: Decision b: give the undo back when no group reached a Telegram write, so someone with rights can retry; show 'undone' only when at least one group was actually undone (otherwise say nothing changed); a crashed undo shows interrupted, not ⏳ (WR-01 and WR-02)."
  severity: major
  test: 4
  root_cause: "The undo flow treats 'an undo was claimed' as 'the action was undone'. staffUndoConfirm takes the D-09 latch (staff.ClaimUndo sets undo_started_at) and edits the original summary to 'Undone by X' before any group runs; Finish only calls FinalizeUndo and never looks at per-group results, no code ever releases the claim, and the run never records whether a group reached its Telegram write. Every 'undone' label (staffHistoryState, the detail header, editStaffUndoneOriginal, staffUndoAlreadyText) keys off the claim alone, and ActionTally counts no undo outcomes. The detail view's dead-run conversion applies only to the action's own run, so an unfinished undo's empty undo_outcome rows render as pending forever. The claim is also taken without checking staffActionsContext().Err() and before staffActionRunsWG.Add, so a Confirm racing shutdown claims and then fails every group without a call."
  artifacts:
    - path: "alita/modules/staff_undo.go"
      issue: "Claim and 'Undone' edit at Confirm before any group (394-416); Finish makes no claim or label decision (540-559); runStaffUndoInGroup (570-615) never records reaching executeStaffUndoCall; staffActionUndoable, Ask and Confirm refuse forever once claimed; staffUndoAlreadyText says undone regardless"
    - path: "alita/db/staff/actions.go"
      issue: "ClaimUndo (270-285) has no matching release; ActionTally/TallyActionGroups (172-228) do not count undo_outcome"
    - path: "alita/modules/staff_history.go"
      issue: "staffHistoryState (69-80) keys only on UndoStartedAt; renderStaffHistoryDetail dead-run conversion (338-349) is tied to the action, and the undo header (360-378) says 'Undone by' unconditionally"
    - path: "alita/modules/staff_action_record.go"
      issue: "staffUndoResultsFromRecord (208-226) maps an empty undo_outcome to pending with no staleness check"
    - path: "alita/modules/staff_action_run.go"
      issue: "startStaffRun adds to staffActionRunsWG only after Confirm has claimed; staffGroupPrechecks returns fail_interrupted with no call on a cancelled context"
  missing:
    - "Record per group that the run reached its Telegram write (set right before executeStaffUndoCall, whatever it returns; a panic-swept group counts as reached)"
    - "In Finish, release the claim when no group reached a write: one conditional update through db.DB scoped to this claim (undo_by, undo_started_at) that clears the undo columns on the action and its group rows, while the run still holds the target lock"
    - "Refuse to claim when staffActionsContext().Err() != nil, and register the run in staffActionRunsWG before the claim (Done on abort)"
    - "Count undo outcomes in ActionTally and give staffHistoryState four states: undone (at least one group done), changed nothing (finished, none done), running (unfinished, recent heartbeat), interrupted (unfinished, heartbeat older than staffTargetLockTTL)"
    - "Move editStaffUndoneOriginal to the end of the run on a fresh budget: 'Undone by X' when at least one group was undone, a 'changed nothing' marker when writes were tried and none succeeded, and leave the original and its Undo button alone when the claim was released"
    - "Make the Ask/Confirm 'already' text distinguish running, undone and changed nothing"
    - "Detail view: an unfinished undo whose heartbeat (updated_at) is older than staffTargetLockTTL shows its pending groups as interrupted, display only, no write"
    - "Locale keys in all 7 files; AGENTS.md and the staff docs page updated in the same commit; regression tests for: presser admin nowhere, Confirm after StopStaffActions, the shutdown window, and a crashed undo; update TestStaffUndoAllSkipped, TestStopStaffActionsUndo, TestStaffUndoOriginalEditFails, TestStaffUndoTracer, TestStaffHistoryDetailUndoOutcome"
  debug_session: .planning/debug/undo-claim-spent-and-undone-label.md

