---
phase: 03-staff-audit-and-undo
plan: 07
subsystem: staff-actions
tags: [telegram, staff-actions, undo, card-lifecycle, redis-lock, shutdown, i18n, tdd]
status: complete

requires:
  - phase: 03-staff-audit-and-undo
    provides: undo card, ClaimUndo, staffClaimUndo seam, staffGroupPrechecks, undo run spec and button rules (plan 03-06)
provides:
  - Undo cards that cancel, expire and refuse exactly like Phase 2 cards, with presser-only wording
  - Service-identity refusal at undo Ask and Confirm
  - A lifecycle test file proving refusals, exactly-once across cards and replicas, the shared target lock, shutdown mid-undo and no time limit
  - AGENTS.md rules for the undo drain and the undo target lock
affects: [03-08, 03-09]

actuals:
  tokens: 21000
  tasks: 2
  commits: 4
plan_head_before: a677f61ff626aae2d9573fed8d452ea06eef92b6
plan_head_after: 9199e85f069715ae4be7dab974f8674e78b80aa6

tech-stack:
  added: []
  patterns:
    - "A card kind picks its own refusal wording inside the shared answerStaffCardClaim instead of forking the handler"
    - "Concurrency tests drive two real dispatchers (otherReplica) on shared Redis and database and assert the invariant (one claim, one summary), not an interleaving"

key-files:
  created:
    - alita/modules/staff_undo_lifecycle_test.go
  modified:
    - alita/modules/staff_undo.go
    - alita/modules/staff_action_card.go
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
  - "Presser-only wording lives in answerStaffCardClaim keyed on card.Kind == staffKindUndo, so Cancel and Confirm taps on an undo card share one answer"
  - "The service-identity check runs before any lookup, ahead of the card load at Confirm"
  - "Task 2 needed no production change: the claim, lock and drain built in 03-06 already satisfy it; the tests were validated by mutation"

patterns-established:
  - "Mutation-check a test that passes at once: remove the guard, see the named test fail, revert"

requirements-completed: [STAFF-11]

coverage:
  - id: D1
    description: "An undo card is cancelled by its presser (edited to the Cancelled text under the undo header, no keyboard, nothing claimed, Undo can be pressed again), expires after 5 minutes by timer and on a late tap, and a double Confirm runs once"
    requirement: "STAFF-11"
    verification:
      - kind: unit
        ref: "alita/modules/staff_undo_lifecycle_test.go#TestStaffUndoCardCancel"
        status: pass
      - kind: unit
        ref: "alita/modules/staff_undo_lifecycle_test.go#TestStaffUndoCardExpires"
        status: pass
      - kind: unit
        ref: "alita/modules/staff_undo_lifecycle_test.go#TestStaffUndoCardDoubleTap"
        status: pass
    human_judgment: false
  - id: D2
    description: "Another member's Confirm or Cancel on an undo card gets the presser-only alert and changes nothing; the presser's Confirm then runs the undo"
    requirement: "STAFF-11"
    verification:
      - kind: unit
        ref: "alita/modules/staff_undo_lifecycle_test.go#TestStaffUndoCardPresserOnly"
        status: pass
    human_judgment: false
  - id: D3
    description: "A non-member, a Telegram service identity, a forged or missing record, a kick, an already-undone record, Redis being down and a presser who left each get their own refusal and no card or claim"
    requirement: "STAFF-11"
    verification:
      - kind: unit
        ref: "alita/modules/staff_undo_lifecycle_test.go#TestStaffUndoOutsider"
        status: pass
    human_judgment: false
  - id: D4
    description: "One undo per action: a second card, a second replica and a repeated Undo press never run it twice, and the loser is told who won or that the target is busy"
    requirement: "STAFF-11"
    verification:
      - kind: unit
        ref: "alita/modules/staff_undo_lifecycle_test.go#TestStaffUndoOnce"
        status: pass
      - kind: unit
        ref: "alita/modules/staff_undo_lifecycle_test.go#TestStaffUndoOnceAcrossReplicas"
        status: pass
    human_judgment: false
  - id: D5
    description: "An undo and a staff action on one person never run at the same time, in either order, because the undo Confirm takes the same per-target lock"
    requirement: "STAFF-11"
    verification:
      - kind: unit
        ref: "alita/modules/staff_undo_lifecycle_test.go#TestStaffUndoTargetLock"
        status: pass
    human_judgment: false
  - id: D6
    description: "A shutdown mid-undo returns within the drain, leaves a final summary with no pending line, records undo_finished_at and marks unfinished groups failed with fail_interrupted; an undo has no time limit"
    requirement: "STAFF-11"
    verification:
      - kind: unit
        ref: "alita/modules/staff_undo_lifecycle_test.go#TestStopStaffActionsUndo"
        status: pass
      - kind: unit
        ref: "alita/modules/staff_undo_lifecycle_test.go#TestStaffUndoNoTimeLimit"
        status: pass
    human_judgment: false

duration: 30min
completed: 2026-10-05
---

# Phase 3 Plan 7: Undo under every press pattern Summary

**An undo card now cancels, expires and refuses like a Phase 2 card with its own presser-only wording, service identities are refused up front, and tests prove one undo per action across cards and replicas, the shared per-target lock in both directions, a truthful shutdown mid-undo and no time limit.**

## Performance

- **Duration:** about 30 min
- **Tasks:** 2 (card lifecycle and refusals; exactly-once, lock, shutdown, AGENTS.md)
- **Files modified:** 12 (1 created, 11 modified)
- **Commits:** 4 task commits (two RED, two GREEN)

## Accomplishments

- **Presser-only wording.** `answerStaffCardClaim` answers a non-issuer tap on an undo card with `staff_undo_card_presser_only` (new key, all 7 locales); `staffUndoConfirm`'s early issuer check uses the same key. Cancel, expiry and abort edits already render the undo header through `staffActionHeader`, so they needed no change.
- **Service identities refused first.** `staffUndoAsk` and `staffUndoConfirm` answer a press from an ID in `staffServiceUserIDs` with `staff_post_as_yourself` as an alert before any lookup.
- **Ten tests in `staff_undo_lifecycle_test.go`** cover the lifecycle (Cancel then Undo again, presser-only, expiry by timer and late tap, double tap), eight refusals, one undo across two cards and across two replicas, the lock in both directions, a shutdown with eight groups mid-undo, and a 90-day-old record.
- **AGENTS.md** states that undo runs join the `StopStaffActions` drain (`FinalizeUndo` on `db.DB`, summary delivery inside the 30 s), that an undo's Confirm takes the same target lock, and the new refusals.

## Task Commits

1. **Task 1: card lifecycle and refusals** - `5ccb508` (test, RED) then `5e9a038` (feat, GREEN)
2. **Task 2: exactly-once, lock, shutdown, no time limit, AGENTS.md** - `3df295c` (test) then `9199e85` (feat, AGENTS.md only)

**Plan metadata:** the commit holding this file follows. No refactor commit was needed.

## TDD Gate Compliance

- **Task 1:** RED `5ccb508` failed on the two planned assertions only: `TestStaffUndoCardPresserOnly` ("answer to Carol's yc = staff-act-card-issuer-only, want the presser-only alert") and `TestStaffUndoOutsider/service_identity` ("answer staff-cb-members-only, want post-as-yourself"). The other eleven subtests and tests passed, as the plan anticipated for behavior built in 03-06. GREEN `5e9a038` passes everything.
- **Task 2:** all five tests passed against the existing code, which the plan allowed ("the claim and the lock are expected to hold"). They were checked by mutation instead of trusted: removing `takeStaffTargetLock` from the undo Confirm fails both `TestStaffUndoTargetLock` subtests; skipping `FinalizeUndo` in the undo run's `Finish` fails `TestStopStaffActionsUndo` on `undo_finished_at`. Both mutations were reverted (`git status` clean afterwards). `TestStaffUndoOnceAcrossReplicas` was run 15 times under `-race` with no failure. The `feat` commit of Task 2 holds only the AGENTS.md change, like Task 2 of plan 03-06.
- `gsd_run check tdd-red-evidence` was not run; RED evidence was read from the failing assertion output.

## Files Created/Modified

- `alita/modules/staff_undo_lifecycle_test.go` - ten tests and four small helpers (`undoMember`, `undoFinishedBan`, `unbansIn`, `undoStartedAt`)
- `alita/modules/staff_action_card.go` - undo wording branch in `answerStaffCardClaim`
- `alita/modules/staff_undo.go` - service-identity refusals, presser-only wording at the early issuer check
- `alita/modules/staff_undo_test.go` - one 03-06 assertion moved to the new wording (see Deviations)
- `locales/*.yml` (7 files) - `staff_undo_card_presser_only`
- `AGENTS.md` - undo drain, undo lock and refusal rules

## Decisions Made

See `key-decisions`. The one worth repeating: Task 2 changed no production code because the earlier plan's design (claim as the last step, same lock, same engine drain) already holds; the value of this plan is the proof.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Test made obsolete by the new behavior] 03-06 access test asserted the old issuer-only wording**
- **Found during:** Task 1 (GREEN)
- **Issue:** `TestStaffUndoAccess/only the member who pressed Undo can confirm` expected `staff_act_card_issuer_only`, which the plan replaces for undo cards.
- **Fix:** it now expects `staff_undo_card_presser_only`, same alert flag.
- **Files modified:** `alita/modules/staff_undo_test.go`
- **Committed in:** `5e9a038`

### Additions beyond the plan text

- The service-identity test also checks the Confirm path (a real member's card cannot be confirmed by a service identity) and loops over all three IDs in `staffServiceUserIDs`. Beyond the plan's single-ID wording, the same guard is covered.
- `AGENTS.md` also gains one sentence on the service-identity refusal and the presser-only alert, in the undo callback bullet.

**Total deviations:** 1 auto-fixed (obsolete assertion), 2 small additions. **Impact:** none on scope; every must-have truth holds.

## Issues Encountered

- `make lint` is unavailable here (golangci-lint is built with go1.25 and refuses the go1.26.0 module). `gofmt -l` on the touched files, `go vet -tags testtools ./alita/modules` and `CGO_ENABLED=0 go build ./...` are clean.
- Tests ran on SQLite only; nothing in this plan adds SQL.
- The worktree command guard refused chained shell commands and heredocs with non-ASCII text; the locale edit ran from a script written with the Write tool into the scratchpad, and git calls were issued one per command.
- Carried over: `TestStaffActionNonStaffUnchanged/tmute_as_a_reply` is flaky across a second boundary (STATE.md). It did not fail in the full package run here. New timing tests use the existing timer variables with generous margins and assert states, not wall-clock seconds.

## Verification Results

- `go test -tags testtools -race -count=1 -run '^TestStaffUndo(Card|Outsider)' -v ./alita/modules` - all PASS (including `TestStaffUndoOutsider` and `TestStaffUndoCardPresserOnly`)
- `go test -tags testtools -race -count=1 -run '^TestStaffUndo(Once|TargetLock|NoTimeLimit)|^TestStopStaffActionsUndo$' -v ./alita/modules` - all PASS (including `TestStaffUndoOnceAcrossReplicas` and `TestStopStaffActionsUndo`)
- `go test -tags testtools -race -count=1 ./alita/modules ./alita/db/staff` - ok (modules 134 s)
- `go test -tags testtools -count=1 -run '^TestStaffLocaleKeys$' ./alita/i18n`, `make check-translations`, `make check-docs` - pass, no drift
- Acceptance criteria re-run and passing: `staff_undo_card_presser_only` in `staff_action_card.go`, `staffServiceUserIDs[` twice in `staff_undo.go`, the key once in each of es fr hi id pt ru, `FinalizeUndo` and "undo's Confirm takes the same lock" in AGENTS.md, `gofmt -l` clean, `go.mod` and `go.sum` unchanged

## Known Stubs

None.

## Threat Flags

None. The surface touched is the plan's own (T-03-21 to T-03-24), each with a test: live membership and service-ID refusal plus presser-only taps (T-03-21), the claim as last step across cards and replicas (T-03-22), the shared target lock in both orders (T-03-23), and the shutdown sweep with `FinalizeUndo` off the cancelled context (T-03-24).

## User Setup Required

None.

## Next Phase Readiness

- Plans 03-08 and 03-09 can rely on an undo that is safe under double taps, second cards, two replicas, a competing action and a restart.
- Still open from earlier plans: `TestRepositoryMigrationChain` against PostgreSQL has to run in CI or from the main checkout; research assumption A2 (restrict on a left target) needs a live check.

## Self-Check: PASSED

- `alita/modules/staff_undo_lifecycle_test.go` exists; the modified files exist.
- Commits `5ccb508`, `5e9a038`, `3df295c` and `9199e85` exist; `test(03-07)` precedes `feat(03-07)` for both tasks.

---
*Phase: 03-staff-audit-and-undo*
*Completed: 2026-10-05*
