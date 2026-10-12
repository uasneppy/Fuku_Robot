---
phase: 03-staff-audit-and-undo
plan: 12
subsystem: staff-actions
tags: [undo, history, audit, i18n, tally]
gap_closure: true
gap_ids: [G-03-4]

requires:
  - phase: 03-staff-audit-and-undo
    provides: "ClaimUndo claim time, ReleaseUndo, the end-of-run original-summary edit (03-10) and the shutdown drain join (03-11)"
provides:
  - "ActionTally.UndoDone, counted by TallyActionGroups in the same single query"
  - "staffUndoStateOf / staffUndoDoneCount: the one place an undo is classified as none, running, interrupted, undone or changed nothing"
  - "Recent actions line and detail view that say undone only when a group was undone, and show a crashed undo as interrupted"
  - "staffUndoClaimedText: Ask and Confirm on a claimed record say running, interrupted, undone-by or changed nothing"
affects: [phase-03-verification, staff-history, staff-undo]

actuals:
  tokens: 12400
  tasks: 2
  commits: 4

plan_head_before: 6a39d6c74b538a0e03eef8cd4192af0dc923ac46
plan_head_after: 861a8f05c87adece63323683c507e3265823dc03
commits: 4

tech-stack:
  added: []
  patterns:
    - "one state function over stored outcomes plus the heartbeat drives every surface that reports an undo (list, detail, Ask, Confirm)"
    - "a dead run's pending rows are converted to failed/interrupted at render time only, nothing is written"

key-files:
  created: []
  modified:
    - alita/db/staff/actions.go
    - alita/db/staff/actions_test.go
    - alita/modules/staff_undo.go
    - alita/modules/staff_history.go
    - alita/modules/staff_history_test.go
    - alita/modules/staff_history_undo_test.go
    - alita/modules/staff_undo_claim_test.go
    - alita/modules/staff_undo_test.go
    - locales/en.yml
    - locales/es.yml
    - locales/fr.yml
    - locales/hi.yml
    - locales/id.yml
    - locales/pt.yml
    - locales/ru.yml
    - AGENTS.md

key-decisions:
  - "staffUndoStateOf is the single classifier (owner decision b on D-09); the claim alone never reads as undone"
  - "the claim loser in undo Confirm re-reads the winner's groups so its text reflects the winner's current state, and aborts with staff_act_abort_check_failed if that read fails"
  - "staff_history_undone_by was removed (no caller left) and replaced by staff_history_undo_by; staff_history_undone is kept for the undone state"

requirements-completed: [SETUP-09, STAFF-10, STAFF-11]

coverage:
  - id: D1
    description: "Recent actions and the detail view say undone only when a group was undone; undo running, undo interrupted and undo changed nothing read as such"
    requirement: STAFF-10
    verification:
      - kind: unit
        ref: "alita/modules/staff_history_test.go#TestStaffHistoryUndoStates"
        status: pass
      - kind: unit
        ref: "alita/modules/staff_history_test.go#TestStaffHistoryDetailUndoOutcome"
        status: pass
      - kind: unit
        ref: "alita/modules/staff_history_test.go#TestStaffHistoryUndoNothingLine"
        status: pass
    human_judgment: false
  - id: D2
    description: "A crashed undo shows its unfinished groups as interrupted by restart and viewing it writes nothing"
    requirement: STAFF-11
    verification:
      - kind: unit
        ref: "alita/modules/staff_history_test.go#TestStaffHistoryDetailUndoOutcome"
        status: pass
    human_judgment: false
  - id: D3
    description: "TallyActionGroups counts undone groups in one query"
    requirement: SETUP-09
    verification:
      - kind: unit
        ref: "alita/db/staff/actions_test.go#TestStaffActionTally"
        status: pass
    human_judgment: false
  - id: D4
    description: "Ask and Confirm on a claimed record say whether the undo is running, was interrupted, undid something or changed nothing, and post and write nothing"
    requirement: STAFF-11
    verification:
      - kind: unit
        ref: "alita/modules/staff_undo_claim_test.go#TestStaffUndoClaimedTexts"
        status: pass
      - kind: unit
        ref: "alita/modules/staff_undo_claim_test.go#TestStaffUndoNothingChanged"
        status: pass
    human_judgment: false
  - id: D5
    description: "Live re-run of UAT test 4 in real groups (non-admin undo writes nothing, admin undo unbans, Recent actions shows undone with per-group results)"
    requirement: STAFF-11
    verification: []
    human_judgment: true
    rationale: "The fakes prove the flow; the plan's human-check asks the owner to confirm it in real Telegram groups"

duration: 25min
completed: 2026-10-06
status: complete
---

# Phase 3 Plan 12: Undo State in History and Undo Answers Summary

**Recent actions, the detail view and the Undo button's answers now report an undo from its stored outcomes (running, interrupted, undone, changed nothing) through one function, `staffUndoStateOf`, so "undone" is shown only when at least one group was really undone and a crashed undo reads interrupted instead of a pending hourglass forever.**

## Performance

- **Duration:** about 25 min
- **Tasks:** 2 (a tracer and an auto task, both TDD)
- **Commits:** 4 (RED and GREEN per task)
- **Files changed:** 16 (457 insertions, 82 deletions)

## Accomplishments

- `ActionTally.UndoDone` is counted by `TallyActionGroups` in the same single grouped query (it now groups by `undo_outcome` too); the list line reads it.
- `staffUndoState` and `staffUndoStateOf(a, undoDone, now)` decide an undo's state: none, running (heartbeat younger than `staffTargetLockTTL`), interrupted (unfinished and older), undone (finished, at least one group undone) or nothing (finished, none undone). `staffUndoDoneCount(groups)` supplies the count from the detail's rows.
- `staffHistoryState` takes `undoDone` and returns `staff_history_undo_running`, `staff_history_undo_interrupted`, `staff_history_undone` or `staff_history_undo_nothing` for a claimed undo, and keeps today's action running/interrupted cases otherwise.
- The detail view builds its undo header from the new `staff_history_undo_by` and, for an interrupted undo, shows its still-pending groups as failed with `staffReasonFailInterrupted`, display only. A test re-reads the row and requires its `undo_outcome` still empty.
- `staffUndoClaimedText` replaces `staffUndoAlreadyText`. Ask and Confirm (first check and the claim-loser path) answer a claimed record with `staff_undo_already_running`, `staff_undo_already_interrupted`, `staff_undo_already_nothing` or `staff_undo_already`, and post or write nothing else.
- Seven new keys in all 7 locales (`staff_history_undo_running`, `staff_history_undo_interrupted`, `staff_history_undo_nothing`, `staff_history_undo_by`, `staff_undo_already_running`, `staff_undo_already_interrupted`, `staff_undo_already_nothing`); `staff_history_undone_by` removed from all 7.
- AGENTS.md states the four undo states, the display-only conversion and the claimed-record answers.

## Task Commits

1. Task 1 RED: `27329fb` test(03-12): prove history says undone whatever the undo did and shows a crashed undo as pending
2. Task 1 GREEN: `eddd576` fix(03-12): show an undo as undone only when a group was undone, and a crashed undo as interrupted
3. Task 2 RED: `fde8a11` test(03-12): prove Undo on a claimed record always says already undone
4. Task 2 GREEN: `861a8f0` fix(03-12): say whether a claimed undo is running, interrupted, done or changed nothing

## TDD Gate Compliance

RED then GREEN commits exist for both tasks. Each RED run failed on assertions for the planned behavior: the detail test failed because the undo-by header and new state markers were absent from today's output and the list line still read "undone" for an undo that changed nothing; the claimed-text tests failed because every claimed record answered "Already undone by". The first RED could not reach every assertion in `TestStaffHistoryDetailUndoOutcome` (it stops at the missing undo header), which is inherent to renaming the header key.

## Tracer Gate

After Task 1 the tracer's three automated verify commands were re-run and passed (`TestStaffActionTally`, the `TestStaffHistory*` set including `TestStaffHistoryUndoStates` and `TestStaffHistoryDetailUndoOutcome`, and the wider `TestStaffHistory|TestStaffUndo|TestStaffPanel`, `TestStaffLocaleKeys` and `make check-translations`) before Task 2 started.

## Verification Results

- `make test`: exit 0, every package ok, the `check_test_results` summary raised no failed or unexpectedly skipped test.
- `make check-translations`: all translations present.
- `make check-docs`: no drift detected.
- `go vet -tags testtools ./...`: clean.
- `CGO_ENABLED=0 go build ./...`: ok.
- `gofmt -l alita main.go`: only the pre-existing `alita/modules/greetings_command_test.go`.
- `git diff --exit-code go.mod go.sum`: no change.
- `go test -tags testtools -race -count=15` of `TestStaffUndoOnce` and `TestStaffUndoOnceAcrossReplicas`: ok (the claimed-text change could have raced them).
- `make lint`: could not run in this sandbox. Exact error: `can't load config: the Go language version (go1.25) used to build golangci-lint is lower than the targeted Go version (1.26.0)`. gofmt and go vet were relied on, as the plan allows.
- `TestStaffActionNonStaffUnchanged/tmute_as_a_reply` (known flaky) did not fail in any run.

## Decisions Made

- One classifier for every surface that reports an undo; the list, the detail and both undo answers cannot disagree.
- The heartbeat rule for a dead undo is the same as for a dead action (`staffTargetLockTTL`), so the two states behave alike.
- The claim loser re-reads the winner's groups. A failed re-read aborts the card with the check-failed text rather than guessing a state.

## Deviations from Plan

None - plan executed exactly as written.

Implementation notes that are not deviations: `TestStaffHistoryDetailUndoOutcome` uses sequential blocks instead of `t.Run`, because its helpers close over the outer `t` and a `Fatalf` from a subtest goroutine would act on the parent; the claimed-text test adds a small `pressUndoButton` env method because the existing `pressUndo` is a closure local to `TestStaffUndoOutsider`.

## Issues Encountered

None.

## Known Stubs

None.

## Threat Flags

None. No new endpoint, auth path or schema change. T-03-35, T-03-36 and T-03-37 are mitigated as planned: one state function over stored outcomes (T-03-35), the dead-undo conversion is display only and a test re-reads the row (T-03-36), and the new texts splice names after translation through `staffUserToken` with `html.EscapeString(staffPlainName(...))` for HTML and plain text for callback answers (T-03-37).

## Human Check Pending

The plan's `human-check` (re-run UAT test 4 live in a real Staff Group with two linked groups) has not been done; it needs a person with real groups. Expected: the non-admin undo writes nothing and the original summary keeps its Undo button; the admin undo unbans in both groups, the original reads "Undone by" with no button, and Recent actions shows "undone" with each group's undo result in the detail.

## Next Phase Readiness

With 03-10, 03-11 and this plan, every item of G-03-4's missing list is delivered and the phase gate is green. Ready for phase verification and the live UAT re-run.

## Self-Check: PASSED

- Files modified exist: actions.go, actions_test.go, staff_undo.go, staff_history.go, staff_history_test.go, staff_history_undo_test.go, staff_undo_claim_test.go, staff_undo_test.go, 7 locale files, AGENTS.md.
- Commits found: 27329fb, eddd576, fde8a11, 861a8f0.
- Acceptance criteria re-run: `UndoDone` and `undo_outcome, COUNT` in actions.go; `staffUndoStateOf` signature; 2 uses in staff_history.go; `staff_history_undo_nothing`, `staff_history_undo_by`, `staff_undo_already_nothing`, `staff_undo_already_running`, `staff_undo_already_interrupted` each in 7 locale files; `staffUndoClaimedText(` appears 3 times; both new tests present; `test(03-12)` precedes `fix(03-12)` in the log.
