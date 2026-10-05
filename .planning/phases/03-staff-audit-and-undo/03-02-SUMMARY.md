---
phase: 03-staff-audit-and-undo
plan: 02
subsystem: api
tags: [telegram, staff-actions, undo, decision-table, tdd]

requires:
  - phase: 03-staff-audit-and-undo
    provides: staffPriorState (captured prior state) and the audit record from plan 03-01
  - phase: 02-staff-actions-across-groups
    provides: decideStaffAction, staffVerdict, staffTargetState, staffReasonOutcome
provides:
  - decideStaffUndo, the one pure table that chooses an undo's Telegram call
  - staffUndoVerdict, staffAppliedState, staffCallRestore, staffUndoMinRemaining
  - the four undone_* reasons and the four new skip reasons, all mapped in staffReasonOutcome
affects: [03-06 undo run, 03-08 end-to-end undo proof]

actuals:
  tokens: 7800
  tasks: 2
  commits: 4
plan_head_before: f48b9884c29cf9a1c8ac90d274a6072b9355acdc
plan_head_after: 211a7618eabf70a9f53f9e0c3847dff605c9479a

tech-stack:
  added: []
  patterns:
    - "Embedding wrapper (staffUndoVerdict embeds staffVerdict) so a Phase 2 type with unkeyed test literals never changes"
    - "Left-behind check before the per-kind column: undo acts only on the state the staff action itself left"
    - "Property loop over every combination (copied from TestStaffActionDecisionInvariants), with a call-count guard so it cannot pass by never calling"

key-files:
  created:
    - alita/modules/staff_action_undo_decide_test.go
  modified:
    - alita/modules/staff_action_decide.go

key-decisions:
  - "Kick and any unknown kind are rejected first with fail_lookup, ahead of the admin and prior-state guards"
  - "A mute's left-behind check compares the restriction (muted, same end date) and not membership"
  - "A ban or mute whose own end date has passed is skip_restriction_ended before the left-behind check"
  - "A restricted prior with no stored permission set is skip_no_prior_state, never a guessed full mute"
  - "staffUndoMinRemaining is 120 seconds: a closer prior end date counts as ended"

patterns-established:
  - "Only decideStaffUndo may send a restrict or ban to a kicked or left target, and only when staffUndoLeftBehind holds"

requirements-completed: [STAFF-11]

coverage:
  - id: D1
    description: "decideStaffUndo restores the recorded prior state for ban, mute, unban and unmute (unban over member or left, ban over a restriction or shorter ban put back with its permissions and end date, unban re-banned, unmute re-restricted) and never just lifts"
    requirement: "STAFF-11"
    verification:
      - kind: unit
        ref: "alita/modules/staff_action_undo_decide_test.go#TestStaffUndoDecisionTable"
        status: pass
    human_judgment: false
  - id: D2
    description: "A group whose live state differs from what the staff action left is skipped as skip_changed_since; a live creator or administrator is skip_target_admin; a row with no captured prior state is skip_no_prior_state; an ended ban or mute is skip_restriction_ended"
    requirement: "STAFF-11"
    verification:
      - kind: unit
        ref: "alita/modules/staff_action_undo_decide_test.go#TestStaffUndoDecisionTable"
        status: pass
      - kind: unit
        ref: "alita/modules/staff_action_undo_decide_test.go#TestStaffUndoDecisionInvariants"
        status: pass
    human_judgment: false
  - id: D3
    description: "The 120-second margin is exact on both sides, a live end date must equal the recorded one (one second off is skipped), and a live permanent ban matches a recorded end date under 30 seconds after applied_at"
    requirement: "STAFF-11"
    verification:
      - kind: unit
        ref: "alita/modules/staff_action_undo_decide_test.go#TestStaffUndoDecisionTable"
        status: pass
    human_judgment: false
  - id: D4
    description: "Over every combination of kind, prior status, permissions and end date, live status and end date, and applied end date: calls reach a kicked or left target only when it is what the action left, never an administrator, unban only a kicked target, no re-applied end date within the margin, a call exactly when the outcome is done, never mute or kick, restores carry exactly the recorded permissions"
    requirement: "STAFF-11"
    verification:
      - kind: unit
        ref: "alita/modules/staff_action_undo_decide_test.go#TestStaffUndoDecisionInvariants"
        status: pass
    human_judgment: false
  - id: D5
    description: "The four undone_* reasons read as done and the four new skip reasons as skipped; Phase 2's decision table and invariants pass unmodified"
    requirement: "STAFF-11"
    verification:
      - kind: unit
        ref: "alita/modules/staff_action_undo_decide_test.go#TestStaffUndoReasonOutcomes"
        status: pass
      - kind: unit
        ref: "alita/modules/staff_action_decide_test.go#TestStaffActionDecisionInvariants"
        status: pass
    human_judgment: false

duration: 14min
completed: 2026-10-05
status: complete
---

# Phase 3 Plan 2: Undo decision table Summary

**One pure function, decideStaffUndo, now decides every undo call: it restores the recorded prior state (permissions and end date included), skips any group whose live state is not exactly what the staff action left, and never re-applies an end date within 120 seconds of now.**

## Performance

- **Duration:** 14 min
- **Tasks:** 2 (both TDD)
- **Files modified:** 2 (1 created, 1 modified)

## Accomplishments

- `decideStaffUndo(orig, prior, applied, live, now)` sits beside `decideStaffAction` in `alita/modules/staff_action_decide.go`. `decideStaffAction`, `staffVerdict` and every Phase 2 constant are unchanged, and the Phase 2 decision tests pass unmodified.
- The shared guards run in a fixed order: unsupported kind, live creator or administrator, no captured prior state, a ban or mute whose own end date has passed, then the left-behind check (`staffUndoLeftBehind`), then the per-kind column. A restore or re-ban reaches a kicked or left target only after the left-behind check, which is the one sanctioned exception to the Phase 2 rule and is documented in the function's GoDoc.
- Restores: ban over member or left is unbanned; ban or mute over a running restriction is put back with the recorded permission set and end date (`staffCallRestore`); ban over a shorter ban still running is re-banned with its original end date; unban is re-banned and unmute re-restricted. A prior end date at or inside `now + 120` is lifted (ban, mute) or skipped as `skip_restriction_ended` (unban, unmute).
- `TestStaffUndoDecisionInvariants` walks 5 kinds x 50 priors x 3 applied states x 9 live statuses x 3 live end dates (20250 combinations) and checks eight invariants, the six the plan names plus two more (a restore carries exactly the recorded permission set; no call without a captured prior state). It fails if no combination makes a call.
- Mutation check: disabling the left-behind check made the invariant test fail on "unban sent to a target who is not banned", so the invariants do bind the table.

## Task Commits

1. **Task 1: ban and mute columns, shared guards, left-behind check** - `2c63efa` (test, RED) then `ee0dc53` (feat, GREEN)
2. **Task 2: unban and unmute columns, invariants** - `1797279` (test, RED) then `211a761` (feat, GREEN)

**Plan metadata:** the `docs(03-02)` commit that holds this file.

## Files Created/Modified

- `alita/modules/staff_action_decide.go` - undo reasons, `staffCallRestore`, `staffUndoMinRemaining`, `staffAppliedState`, `staffUndoVerdict`, `staffUndoEnded`, `staffUndoSameUntil`, `staffUndoLeftBehind`, `decideStaffUndo` and the four per-kind columns; `staffReasonOutcome` arms for the eight new reasons
- `alita/modules/staff_action_undo_decide_test.go` - `TestStaffUndoDecisionTable` (ban, mute, unban, unmute, kick, unknown kind), `TestStaffUndoDecisionInvariants`, `TestStaffUndoReasonOutcomes`

## Decisions Made

- Kick and any unknown kind return `fail_lookup` before every other guard, so a kick never reads as "changed since" (the plan's order would have given `skip_changed_since` for a captured prior, because the left-behind rule is false for kick).
- A mute's left-behind check does not compare membership; a muted member who left keeps the same restriction.
- A ban or mute whose own end date passed is `skip_restriction_ended` before the left-behind check, since Telegram lifted it already.
- A restricted prior with no stored permission set is `skip_no_prior_state`.
- The 120-second margin is a single constant, `staffUndoMinRemaining`, as the plan asked (research A6).

## TDD Gate Compliance

Both tasks followed RED then GREEN with no refactor commit.

- Task 1: RED `2c63efa` (29 of the 34 ban and mute rows failed on assertions (the 5 rows that expect `skip_no_prior_state` pass against the placeholder); the placeholder returns `skip_no_prior_state`), GREEN `ee0dc53`.
- Task 2: RED `1797279` (the unban and unmute rows and the kick and unknown-kind checks failed on assertions), GREEN `211a761`. `TestStaffUndoDecisionInvariants` and `TestStaffUndoReasonOutcomes` already passed at RED, as expected: the invariants hold for the Task 1 table, and the reason arms were added in Task 1 GREEN as the plan says. The mutation check above shows the invariants are not vacuous.
- `gsd_run check tdd-red-evidence` was not run (the gsd-tools copy in the worktree is missing `vendor/re2js.cjs`, the same limitation as plan 03-01). RED evidence was verified by reading the failing test output.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Compile-only declarations in the Task 1 RED commit**
- **Found during:** Task 1 (RED)
- **Issue:** The test file uses `decideStaffUndo`, the new reasons, `staffCallRestore`, `staffAppliedState` and `staffUndoVerdict`, so the package does not compile without them. A compile error is INVALID_RED, not a failing assertion.
- **Fix:** The RED commit `2c63efa` carries the new constants, types and a placeholder `decideStaffUndo` that returns `skip_no_prior_state`, so every row fails on its own assertion. The table logic, helpers and `staffReasonOutcome` arms came in GREEN.
- **Files modified:** `alita/modules/staff_action_decide.go`
- **Commit:** `2c63efa`

**2. [Rule 1 - Bug in plan] Kick must be rejected before the left-behind check**
- **Found during:** Task 2 (RED)
- **Issue:** The plan orders the guards admin, prior, ended, left-behind, per-kind, and says a kick returns `fail_lookup`. `staffUndoLeftBehind` is false for kick, so a kick would never reach the per-kind default and would read `skip_changed_since`, `skip_target_admin` or `skip_no_prior_state` instead.
- **Fix:** `decideStaffUndo` returns `fail_lookup` for kick and any unknown kind first. Behaviour for ban, mute, unban and unmute is the plan's order.
- **Files modified:** `alita/modules/staff_action_decide.go`
- **Commit:** `211a761`

**3. Extra table rows beyond the plan's list.** The table also covers a live administrator and no captured prior state for each kind, a prior that is not a state the kind can follow (for example unban over a prior member), the exact 30-second boundary for Telegram's permanent rule, a dated mute extended since, and the margin on both sides for unban and unmute. These add coverage and change no behavior.

---

**Total deviations:** 2 auto-fixed (1 blocking, 1 plan ordering), 1 coverage addition
**Impact on plan:** No scope change. The plan's intent and every must-have truth hold.

## Issues Encountered

- `make lint` is unavailable (golangci-lint is built with go1.25 and refuses the go1.26.0 module); `gofmt -l`, `go vet -tags testtools ./alita/modules` and `CGO_ENABLED=0 go build ./...` are clean.
- The worktree guard refused a command that redirected output into the git directory, so the plan-commit ledger of the commit protocol was kept in the scratchpad instead; `commits: 4` and `plan_head_before` come from `git rev-list --count f48b988..HEAD` against the base SHA the spawn check verified.

## Verification Results

- `go test -tags testtools -race -count=1 -run '^TestStaffUndoDecision(Table|Invariants)$|^TestStaffUndoReasonOutcomes$' -v ./alita/modules` - all three PASS
- `go test -tags testtools -race -count=1 -run '^TestStaffAction' ./alita/modules` - ok (Phase 2 regression, unmodified)
- `go test -tags testtools -race -count=1 -run '^TestStaffUndoDecision|^TestStaffUndoReasonOutcomes$|^TestStaffActionDecision' ./alita/modules` - ok
- Acceptance greps: `func decideStaffUndo(` signature, `staffCallRestore staffAPICall = staffCallUnmute + 1`, `const staffUndoMinRemaining int64 = 120`, `staffVerdict` still two fields, `decideStaffUndoUnban`, `decideStaffUndoUnmute`, `staffReasonUndone` count 14 (8 or more required) - all pass
- `gofmt -l` on both files prints nothing; `git diff --exit-code go.mod go.sum` clean

## Known Stubs

None.

## Threat Flags

None. This plan adds a pure function and tests; it adds no endpoint, auth path, file access or schema change. T-03-05 (undo overruling a later decision) and T-03-06 (a restore turning permanent) are mitigated as the plan states, by `staffUndoLeftBehind` and `staffUndoMinRemaining`, and proven by invariants (a), (b), (c) and (d).

## Next Phase Readiness

- Plan 03-06 can call `decideStaffUndo` from the undo run. It must decide `skip_not_applied` (D-03) itself before any Telegram read, because that needs the original row's outcome, and it must execute `staffCallRestore` as `restrictChatMember` with `use_independent_chat_permissions` and the verdict's `Perms` and `Until`.
- `staffCallBan`, `staffCallUnban` and `staffCallUnmute` from an undo verdict can go through the existing `executeStaffCall`, with the verdict's `Until` as the ban end date.
- Plan 03-08 still has to prove the restores end to end against the stateful fake, including research assumption A2 (restrict on a left target) which needs a live check.

## Self-Check: PASSED

- Created file exists: `alita/modules/staff_action_undo_decide_test.go`.
- Commits `2c63efa`, `ee0dc53`, `1797279` and `211a761` exist; `test(03-02)` precedes `feat(03-02)` for both tasks.

---
*Phase: 03-staff-audit-and-undo*
*Completed: 2026-10-05*
