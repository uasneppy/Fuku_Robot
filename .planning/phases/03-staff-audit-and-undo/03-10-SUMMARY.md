---
phase: 03-staff-audit-and-undo
plan: 10
subsystem: staff-actions
tags: [undo, audit, claim, gorm, telegram, i18n]
gap_closure: true
gap_ids: [G-03-4]

requires:
  - phase: 03-staff-audit-and-undo
    provides: "staff undo run, ClaimUndo one-undo latch (D-09), original-summary marker (D-08) from plans 03-05 to 03-09"
provides:
  - "ClaimUndo returns the microsecond claim time; ReleaseUndo gives one claim back"
  - "staffGroupResult.Reached marker; an undo that reached no Telegram write gives the action's one undo back"
  - "original summary edited at the end of the undo run: Undone by, changed nothing, or left alone"
affects: [phase-03-verification, staff-undo, staff-help-docs]

actuals:
  tokens: 18400
  tasks: 2
  commits: 4

plan_head_before: 44e923adc5fa5ece8e05d0cf3f8f645231f24aba
plan_head_after: 36bef0c06730beac5f3cfc018edc7387c494ab23
commits: 4

tech-stack:
  added: []
  patterns:
    - "claim named by its exact microsecond timestamp so a release can match only that claim"
    - "per-group Reached marker set once the write call returned, swept groups count as reached"

key-files:
  created:
    - alita/modules/staff_undo_claim_test.go
  modified:
    - alita/db/staff/actions.go
    - alita/db/staff/actions_test.go
    - alita/modules/staff_action_run.go
    - alita/modules/staff_undo.go
    - alita/modules/staff_undo_test.go
    - alita/modules/staff_undo_lifecycle_test.go
    - locales/en.yml
    - locales/es.yml
    - locales/fr.yml
    - locales/hi.yml
    - locales/id.yml
    - locales/pt.yml
    - locales/ru.yml
    - docs/src/content/docs/commands/staff/index.md
    - AGENTS.md

key-decisions:
  - "Owner decision b (UAT test 4): an undo whose run reached no Telegram write gives the one-undo claim back; once any group reached its write, D-09 holds"
  - "A group lost to a panic (swept by sweepPending) counts as reached, so the claim is kept (fail closed)"
  - "A release that errors twice or no longer matches falls through to FinalizeUndo, keeping the claim"
  - "The original summary is edited in the undo spec's Delivered hook on its own newStaffDeliverContext budget, never at Confirm"

patterns-established:
  - "Release-by-claim: conditional update on id, claimer and exact claim time, group columns cleared in the same transaction only after RowsAffected == 1"

requirements-completed: [STAFF-11]

coverage:
  - id: D1
    description: "A member who is an admin in no linked group presses Undo and Confirm: no write, claim given back (all undo columns cleared), original keeps its text and Undo button, and a member with rights then undoes the action everywhere"
    requirement: STAFF-11
    verification:
      - kind: unit
        ref: "alita/modules/staff_undo_claim_test.go#TestStaffUndoClaimReleased"
        status: pass
      - kind: unit
        ref: "alita/modules/staff_undo_test.go#TestStaffUndoAllSkipped"
        status: pass
    human_judgment: false
  - id: D2
    description: "ReleaseUndo matches only the claim it is given and clears the parent and group undo columns in one transaction; ClaimUndo returns an exactly round-tripping claim time"
    requirement: STAFF-11
    verification:
      - kind: unit
        ref: "alita/db/staff/actions_test.go#TestStaffActionReleaseUndo"
        status: pass
      - kind: unit
        ref: "alita/db/staff/actions_test.go#TestStaffActionClaimUndo"
        status: pass
    human_judgment: false
  - id: D3
    description: "D-09 still holds once a write was reached: all-failed writes keep the claim and the original reads 'changed nothing'; a panicked group keeps the claim; a shutdown after a write keeps it, before any write gives it back"
    requirement: STAFF-11
    verification:
      - kind: unit
        ref: "alita/modules/staff_undo_claim_test.go#TestStaffUndoNothingChanged"
        status: pass
      - kind: unit
        ref: "alita/modules/staff_undo_claim_test.go#TestStaffUndoPanicKeepsClaim"
        status: pass
      - kind: unit
        ref: "alita/modules/staff_undo_lifecycle_test.go#TestStopStaffActionsUndo"
        status: pass
    human_judgment: false
  - id: D4
    description: "D-08: the original summary gains 'Undone by <name>' only at the end of the run, after the last linked-group write, and only when at least one group was undone"
    requirement: STAFF-11
    verification:
      - kind: unit
        ref: "alita/modules/staff_undo_test.go#TestStaffUndoTracer"
        status: pass
      - kind: unit
        ref: "alita/modules/staff_undo_test.go#TestStaffUndoOriginalEditFails"
        status: pass
    human_judgment: false
  - id: D5
    description: "The /staff help in 7 languages, the regenerated docs and AGENTS.md describe the released and kept undo rules; the wording of the 6 translated help sentences and notes is read by a native speaker"
    requirement: STAFF-11
    verification:
      - kind: other
        ref: "make check-translations && make check-docs && go test -tags testtools -run '^TestStaffLocaleKeys$' ./alita/i18n"
        status: pass
    human_judgment: true
    rationale: "Key presence and doc drift are automated; translation quality of es, fr, hi, id, pt and ru text needs a human reader"

duration: 40min
completed: 2026-10-06
status: complete
---

# Phase 3 Plan 10: Undo claim given back when nothing was written Summary

**An undo run that reached no Telegram write in any group now gives the action's one undo back through a claim-scoped ReleaseUndo, and the original summary says "Undone by" only when a group was undone, "changed nothing" when writes failed, and is left alone when the claim was released.**

## Performance

- **Duration:** about 40 min
- **Completed:** 2026-10-06
- **Tasks:** 2 (both TDD, 4 commits)
- **Files modified:** 16 (one new test file)

## Accomplishments

- Closed the claim half of gap G-03-4 (WR-01) per owner decision b: a member who is an admin nowhere can no longer spend an action's only undo with no effect. `ReleaseUndo` clears exactly the claim a run took (record ID, `undo_by`, exact `undo_started_at`, undo unfinished) and the groups' undo columns in one transaction.
- `ClaimUndo` now returns the claim time truncated to the microsecond, so the value round-trips exactly through PostgreSQL and SQLite and can name the claim.
- `staffGroupResult.Reached` is set once `executeStaffUndoCall` returned, whatever it returned, and for every group `sweepPending` swept (panic or shutdown), so D-09 holds as soon as any write may have happened.
- The undo run's `Finish` releases the claim when no group reached a write (one retry, then falls through to `FinalizeUndo` so the claim is kept); `Delivered` edits the original summary on its own delivery budget, "Undone by" when at least one group was undone, "changed nothing" otherwise, and not at all when released. The released undo's own summary carries a note that the action can still be undone.
- Two new locale keys in 7 files, the `/staff` help text in 7 languages, regenerated docs and AGENTS.md rules.

## Task Commits

1. **Task 1 RED: prove a no-effect undo uses up the only undo** - `2320085` (test)
2. **Task 1 GREEN: give the undo back when no group reached a Telegram write** - `4438d66` (fix)
3. **Task 2 RED: prove the claim and the Undone label ignore what the run did** - `cc4e88c` (test)
4. **Task 2 GREEN: say "changed nothing" when an undo tried and failed, and keep its claim** - `36bef0c` (fix)

**Plan metadata:** committed separately as `docs(03-10)` (this SUMMARY).

## TDD Gate Compliance

- Task 1: RED `2320085` (`TestStaffUndoClaimReleased` failed on its assertion that the claim was given back: "record after a given-back undo = undo_by ... name \"Bob\" ... want all empty"), then GREEN `4438d66`.
- Task 2: RED `cc4e88c` (`TestStaffUndoClaimReleased` released note, `TestStaffUndoNothingChanged` and `TestStaffUndoPanicKeepsClaim` failed on their assertions), then GREEN `36bef0c`.
- No REFACTOR commit was needed.
- Note on Task 1 RED: only the module test could be written before the implementation (the db tests need the new signatures), so `TestStaffActionReleaseUndo` and the updated `TestStaffActionClaimUndo` arrived in the GREEN commit, as the plan describes.

## Files Created/Modified

- `alita/db/staff/actions.go` - `ClaimUndo` returns the claim time; new `ReleaseUndo`
- `alita/db/staff/actions_test.go` - `TestStaffActionReleaseUndo`, updated `TestStaffActionClaimUndo`
- `alita/modules/staff_action_run.go` - `Reached` on `staffGroupResult`, set by `sweepPending`
- `alita/modules/staff_undo.go` - release in `Finish`, original edit in `Delivered`, released note in the final render, `Reached` in `runStaffUndoInGroup`
- `alita/modules/staff_undo_claim_test.go` - new: `TestStaffUndoClaimReleased`, `TestStaffUndoNothingChanged`, `TestStaffUndoPanicKeepsClaim`
- `alita/modules/staff_undo_test.go`, `alita/modules/staff_undo_lifecycle_test.go` - revised `TestStaffUndoAllSkipped`, `TestStaffUndoOriginalEditFails`, claim-winner subtest, `TestStopStaffActionsUndo` (now two subtests, polling the fake instead of sleeping), `TestStaffUndoTracer` ordering assertion
- `locales/*.yml` (7) - `staff_undo_marker_nothing`, `staff_undo_released_note`, extended `staff_help_msg`
- `docs/src/content/docs/commands/staff/index.md` - regenerated
- `AGENTS.md` - ReleaseUndo, the reached rule, the end-of-run original edit

## Decisions Made

- Followed owner decision b exactly; a release that cannot be confirmed keeps the claim (fail closed).
- Added test helpers `wantUndoClaimGivenBack`, `waitForStaffRequest` and `requestCounts` so the revised tests share one definition of "claim given back" and poll the fake instead of sleeping.

## Deviations from Plan

None - plan executed exactly as written.

## Authentication Gates

None.

## Verification

- `go test -tags testtools -race -count=1 -run '^TestStaffAction(ClaimUndo|ReleaseUndo|FinalizeUndo)$' ./alita/db/staff` - pass
- `go test -tags testtools -race -count=1 -run '^TestStaffUndo|^TestStopStaffActions|^TestStaffHistory|^TestStaffAction' ./alita/modules` - pass
- `go test -tags testtools -count=1 -run '^TestStaffLocaleKeys$' ./alita/i18n`, `make check-translations`, `make check-docs` - pass
- `go vet -tags testtools ./alita/modules ./alita/db/staff`, `CGO_ENABLED=0 go build ./...`, `gofmt -l` on touched files - clean
- `git diff go.mod go.sum` against the plan base - empty
- `make test` - exit 0 (full suite, race, coverage 69.1%)
- `make lint` - could not run: the installed golangci-lint was built with go1.25 and refuses the go1.26.0 module ("the Go language version (go1.25) used to build golangci-lint is lower than the targeted Go version (1.26.0)"). This is an environment limit, not a code finding. `gofmt` and `go vet` are clean.
- The known flaky `TestStaffActionNonStaffUnchanged/tmute_as_a_reply` did not fail in any run.
- `gofmt -l alita/modules` lists `greetings_command_test.go`, which this plan did not touch (pre-existing).

## Known Stubs

None.

## Threat Flags

None. The plan's threats T-03-30 (release after a write), T-03-31 (release of a claim it does not own) and T-03-32 (original claims "Undone" when nothing changed) are mitigated and covered by `TestStaffUndoNothingChanged`, `TestStaffUndoPanicKeepsClaim`, `TestStopStaffActionsUndo`, `TestStaffActionReleaseUndo`, `TestStaffUndoTracer` and `TestStaffUndoClaimReleased`.

## Issues Encountered

None blocking. The translated locale strings (es, fr, hi, id, pt, ru) were written by the executor and have not been read by a native speaker (see coverage D5).

## Next Phase Readiness

Gap G-03-4 claim half is closed; ready for the remaining gap-closure plan or re-verification of UAT test 4.

## Self-Check: PASSED

- Created file present: `alita/modules/staff_undo_claim_test.go`
- Commits present: `2320085`, `4438d66`, `cc4e88c`, `36bef0c`
- All acceptance criteria greps for both tasks matched; `git log` shows the `test(03-10)` commits before their `fix(03-10)` commits.
