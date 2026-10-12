---
phase: 03-staff-audit-and-undo
plan: 11
subsystem: staff-actions
tags: [undo, shutdown, wait-group, drain, i18n]
gap_closure: true
gap_ids: [G-03-4]

requires:
  - phase: 03-staff-audit-and-undo
    provides: "ClaimUndo claim time, ReleaseUndo, staffGroupResult.Reached and the undo run's Finish/Delivered hooks from plan 03-10"
provides:
  - "joinStaffRuns / startJoinedStaffRun: a caller can register a run with the shutdown drain before the work the drain must wait for"
  - "an undo Confirm that joins staffActionRunsWG before it claims, so StopStaffActions waits for a Confirm that already claimed"
  - "an undo Confirm after StopStaffActions cancelled the run context claims nothing and aborts its card with staff_undo_abort_restarting"
affects: [phase-03-verification, staff-undo, shutdown]

actuals:
  tokens: 8200
  tasks: 2
  commits: 4

plan_head_before: b342f07b8b39914d826a1994e4b6e096637831e2
plan_head_after: 4eadd0cd51b0a19624b36597fe3b4b8f56b54c39
commits: 4

tech-stack:
  added: []
  patterns:
    - "register with the shutdown wait group before the claim, give the registration back on every path that does not start the run, let the started run's coordinator own the Done"

key-files:
  created:
    - alita/modules/staff_undo_shutdown_test.go
  modified:
    - alita/modules/staff_action_run.go
    - alita/modules/staff_undo.go
    - locales/en.yml
    - locales/es.yml
    - locales/fr.yml
    - locales/hi.yml
    - locales/id.yml
    - locales/pt.yml
    - locales/ru.yml
    - AGENTS.md

key-decisions:
  - "startStaffRun stays as a one-call wrapper over joinStaffRuns + startJoinedStaffRun, so staff actions start exactly as before"
  - "the restart refusal check sits right after joinStaffRuns, before the claim: a refused Confirm claims nothing and the action keeps its one undo (D-09, owner decision b)"

requirements-completed: [STAFF-11]

coverage:
  - id: D1
    description: "A shutdown that starts after an undo Confirm claimed waits for that undo's run; with no group written the claim is given back before StopStaffActions returns"
    requirement: STAFF-11
    verification:
      - kind: unit
        ref: "alita/modules/staff_undo_shutdown_test.go#TestStaffUndoShutdownWindow"
        status: pass
    human_judgment: false
  - id: D2
    description: "An undo Confirm tapped after the shutdown cancelled the run context claims nothing, calls no linked group, leaves the original summary alone, frees the target lock and tells the presser to retry in a minute"
    requirement: STAFF-11
    verification:
      - kind: unit
        ref: "alita/modules/staff_undo_shutdown_test.go#TestStaffUndoConfirmAfterStop"
        status: pass
    human_judgment: false
  - id: D3
    description: "Restart refusal text exists in all 7 locales; AGENTS.md states the drain rule"
    requirement: STAFF-11
    verification:
      - kind: other
        ref: "make check-translations; go test -tags testtools ./alita/i18n -run TestStaffLocaleKeys"
        status: pass
    human_judgment: false

duration: 11min
completed: 2026-10-06
status: complete
---

# Phase 3 Plan 11: Undo Confirm Joins the Shutdown Drain Summary

**An undo Confirm now registers with the shutdown wait group before it claims, so StopStaffActions waits for a claim already taken, and a Confirm tapped after the shutdown began claims nothing and says the bot is restarting.**

## Performance

- **Duration:** 11 min
- **Completed:** 2026-10-06T11:23:13Z
- **Tasks:** 2 (each TDD: failing test commit, then fix commit)
- **Files modified:** 11 (1 created)

## Accomplishments

- `startStaffRun` is split into `joinStaffRuns` (the single place a run does `staffActionRunsWG.Add(1)`, then reads the context) and `startJoinedStaffRun` (the unchanged coordinator body, whose first defer is the `Done`). `startStaffRun` is now a one-line wrapper, so staff actions start as before.
- `staffUndoConfirm` calls `joinStaffRuns()` after its last read and before the claim. A deferred closure gives the registration back when `runStarted` is false (loser path, claim error, any abort, a panic); the started run's coordinator owns the matching `Done`. `startStaffUndoRun` takes the context and starts through `startJoinedStaffRun`.
- Closes debug-session evidence T3 (82 microsecond window between claim and run start): `StopStaffActions` now waits for that Confirm, and 03-10's release gives the claim back when the shutdown cut every group off before its write.
- Closes evidence T2: once `runCtx.Err() != nil` the Confirm aborts with `staff_undo_abort_restarting` before the claim. The deferred closures free the wait-group registration and the target lock; the action keeps its one undo.
- `staff_undo_abort_restarting` added to all 7 locales; AGENTS.md StopStaffActions bullet states both rules.

## Task Commits

1. **Task 1 (tracer): a shutdown waits for an undo Confirm that already claimed**
   - RED `385a6b4` (test) - TestStaffUndoShutdownWindow failed on "record still claimed after StopStaffActions returned"
   - GREEN `3b3aead` (fix)
2. **Task 2: an undo Confirm after the shutdown claims nothing**
   - RED `fc0f7d7` (test) - TestStaffUndoConfirmAfterStop failed on the missing restart text and card state `done` instead of `aborted`
   - GREEN `4eadd0c` (fix)

**Plan metadata:** committed separately (docs: complete plan)

## Files Created/Modified

- `alita/modules/staff_action_run.go` - `joinStaffRuns`, `startJoinedStaffRun`, `startStaffRun` wrapper
- `alita/modules/staff_undo.go` - Confirm joins the drain before the claim, restart refusal, `startStaffUndoRun` takes the run context
- `alita/modules/staff_undo_shutdown_test.go` - `TestStaffUndoShutdownWindow`, `TestStaffUndoConfirmAfterStop`, `resetStaffActionsContextAfter`
- `locales/{en,es,fr,hi,id,pt,ru}.yml` - `staff_undo_abort_restarting`
- `AGENTS.md` - the StopStaffActions drain rule for undo Confirms

## Decisions Made

- Kept `startStaffRun` as a wrapper rather than changing its callers, as the plan specifies (staff action tests stay untouched).
- The refusal sits after the Confirm's reads and the Staff Group membership check, immediately after `joinStaffRuns()`, as the plan says; it makes no call to any linked group.

## Deviations from Plan

None - plan executed exactly as written. Hindi and Indonesian wording refers to the button by those locales' own `staff_undo_label` ("पूर्ववत", "Batalkan") instead of the English word "Undo".

## Verification

- `go test -tags testtools -race -count=1 -run '^TestStaffUndo(ConfirmAfterStop|ShutdownWindow)$|^TestStopStaffActions' -v ./alita/modules`: PASS, both new tests plus the three TestStopStaffActions tests.
- `go test -tags testtools -race -count=1 -run '^TestStaffUndo|^TestStaffAction|^TestStaffHistory' ./alita/modules`: ok (run after each task).
- `go test -tags testtools -count=1 -run '^TestStaffLocaleKeys$' ./alita/i18n`: ok; `make check-translations`: all present; `make check-docs`: no drift.
- `go vet -tags testtools ./alita/modules ./alita/db/staff`: clean; `CGO_ENABLED=0 go build ./...`: ok; `git diff b342f07 -- go.mod go.sum`: empty.
- All acceptance-criteria greps and `gofmt -l` on the touched files: pass (the one `gofmt -l` hit, `alita/modules/greetings_command_test.go`, is pre-existing and untouched).
- `make test` (full run): exit 0, 60 packages ok, no FAIL. The known flaky `TestStaffActionNonStaffUnchanged/tmute_as_a_reply` did not fail.
- `make lint` was not run: the installed golangci-lint was built with go1.25 and refuses the go1.26.0 module.

## TDD Gate Compliance

Both tasks have a `test(03-11)` commit that precedes its `fix(03-11)` commit; each RED failed on the planned assertion.

## Known Stubs

None.

## Threat Flags

None. T-03-33 and T-03-34 are mitigated as planned: the Confirm joins the drain before the claim, and exactly one `Done` happens per registration (deferred when the run does not start, the coordinator's when it does; every undo test ends with `waitRuns`).

## Self-Check: PASSED

- FOUND: alita/modules/staff_undo_shutdown_test.go
- FOUND commits: 385a6b4, 3b3aead, fc0f7d7, 4eadd0c
