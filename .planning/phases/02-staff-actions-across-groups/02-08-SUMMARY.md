---
phase: 02-staff-actions-across-groups
plan: 08
subsystem: staff-actions
tags: [telegram, retry_after, flood-control, go, gap-closure]

requires:
  - phase: 02-staff-actions-across-groups
    provides: plan 02-06 pacer and bounded workers, plan 02-07 batched card edits and final delivery
provides:
  - per-message delivery budget (staffActionDeliverPartTimeout, 210 s) for the final edit, the fallback and each continuation
  - staffActionSleep seam plus a deadline-checking virtual clock for production-scale retry_after tests
  - progress-edit hold after a 429 and a hold-aware final edit
  - overflow-safe retry_after helpers (staffRetryAfterHold, staffRetryAfterMaxSeconds)
affects: [02-09, 02-10, phase-02-verification]

actuals:
  tokens: 9500
  tasks: 2
  commits: 4
plan_head_before: 12eb21ab3ba85ba462100cf534fbee12125bf79d
plan_head_after: 21818c5bfbff1a11f6a253640822d7b84287f056

tech-stack:
  added: []
  patterns:
    - "one fresh context per delivered message, never derived from the run context"
    - "package-level sleep seam (staffActionSleep) so tests can check waits against deadlines at production durations"
    - "coordinator-local hold time (editHoldUntil), no lock because only the coordinator goroutine touches it"

key-files:
  created: []
  modified:
    - alita/modules/staff_action_summary.go
    - alita/modules/staff_action_run.go
    - alita/modules/staff_action_progress_test.go
    - alita/modules/staff_action_fake_test.go
    - AGENTS.md

key-decisions:
  - "Per-message budgets (3 x (60 s cap + 10 s edit timeout) = 210 s) instead of one larger shared budget, so a long wait on one message never starves the next"
  - "The progress hold uses the full retry_after (clamped to 1 h only to avoid overflow); the final edit's pre-wait for that hold is capped at 60 s like every other wait"
  - "Only a 429 re-marks the progress snapshot as changed; other progress-edit errors keep the old behaviour so a deleted card is not hammered every tick"
  - "StopStaffActions keeps its 30 s bound; a delivery still waiting out a 429 at shutdown is cut off with the process and AGENTS.md says so"

patterns-established:
  - "retry_after is clamped to staffRetryAfterMaxSeconds before it is multiplied by a unit"

requirements-completed: [STAFF-08, STAFF-12]

coverage:
  - id: D1
    description: "A 429 on the final edit with retry_after up to the 60 s cap is waited out and retried; the issuer's card ends on the final summary with no fallback"
    requirement: "STAFF-12"
    verification:
      - kind: unit
        ref: "alita/modules/staff_action_progress_test.go#TestStaffActionFinalEditLongRetryAfter"
        status: pass
    human_judgment: false
  - id: D2
    description: "The fallback and every continuation part get their own full 60 s wait and are delivered once each, within 3800 UTF-16 units"
    requirement: "STAFF-08"
    verification:
      - kind: unit
        ref: "alita/modules/staff_action_progress_test.go#TestStaffActionFinalPartsOwnBudget"
        status: pass
    human_judgment: false
  - id: D3
    description: "Progress edits stop for as long as Telegram's retry_after runs, the newest results go out after it, and the final edit waits out the hold"
    requirement: "STAFF-12"
    verification:
      - kind: unit
        ref: "alita/modules/staff_action_progress_test.go#TestStaffActionProgressBacksOffAfter429"
        status: pass
      - kind: unit
        ref: "alita/modules/staff_action_progress_test.go#TestStaffActionFinalEditWaitsForHold"
        status: pass
    human_judgment: false
  - id: D4
    description: "An absurd retry_after (math.MaxInt64) gives a 60 s wait and a 1 h hold, never a negative duration"
    requirement: "STAFF-08"
    verification:
      - kind: unit
        ref: "alita/modules/staff_action_progress_test.go#TestStaffRetryAfterClamp"
        status: pass
    human_judgment: false

duration: 15min
completed: 2026-10-05
status: complete
---

# Phase 2 Plan 08: Final summary outlasts a long retry_after Summary

**Each staff summary message (final edit, fallback, every continuation) now opens its own 210 s delivery budget, progress edits hold off while Telegram's retry_after runs, and retry_after is clamped to 3600 s before it is multiplied.**

## Performance

- **Duration:** 15 min
- **Tasks:** 2 of 2
- **Files modified:** 5 (no files created)

## Accomplishments

- Gap 1 (WR-01, success criterion 4) is closed. Before, the final summary shared one 15 s context while a single retry_after wait could last 60 s. The final edit gave up early, and the fallback and continuation parts inherited an expired context and never retried. Now `deliverStaffActionFinal` opens a fresh `newStaffDeliverContext()` per message, sized `staffActionDeliverPartTimeout = 3 x (60 s + 10 s) = 210 s`.
- Proven at production scale (`staffActionEditRetryUnit = 1 s`) without sleeping: `withVirtualStaffSleep` replaces the new `staffActionSleep` seam with a clock that refuses any wait its context cannot afford. A 45 s retry_after twice on the final edit is waited out (three attempts, no fallback). A 60 s retry_after on all three edit attempts plus one 60 s 429 before the fallback and before every continuation still delivers every part once. The waits add up to 120 s in the edit, then 60 s per later message, so any single shared budget up to 210 s would starve the first continuation.
- The coordinator keeps `editHoldUntil` and skips progress ticks until it passes. It skips before the snapshot, so changes stay marked; on a 429 only, it also calls `markDirty()` so the newest state goes out on the first tick after the hold. Any other progress-edit error keeps the old handling.
- The final edit waits for the hold before its first attempt (`notBefore`, at most one 60 s wait) and then succeeds on its first attempt.
- `staffRetryAfterHold` clamps seconds to `staffRetryAfterMaxSeconds` (3600) before multiplying; `staffRetryAfterWait` is now `min(hold, 60 s)`.
- AGENTS.md: the StopStaffActions bullet says a delivery still waiting out a 429 at the 30 s limit is cut off with the process; the card-coordinator bullet states the hold, the final edit's wait for it and the per-message budget.

## Task Commits

1. Task 1 (tracer), RED: `8d894bc` `test(02-08): prove the final summary is lost when retry_after outlasts the delivery budget`. Both new tests failed on assertions (1 final edit attempt instead of 3; fallback and continuations unretried).
2. Task 1, GREEN: `ca10430` `fix(02-08): give each staff summary message its own delivery budget so a long retry_after cannot lose it`
3. Task 2, RED: `fff7c0d` `test(02-08): add failing tests for 429 backoff of staff summary edits`. Failures: edits 39 ms and 112 ms after the rate-limited one, and `staffRetryAfterWait(MaxInt64) = -1s` (the real overflow).
4. Task 2, GREEN: `21818c5` `fix(02-08): back off staff summary edits after a 429 and wait out the hold before the final edit`

Tracer gate: the Task 1 verify command was re-run end to end after the GREEN commit and passed, so expansion continued (Task 2).

## TDD Gate Compliance

Both tasks have a `test(02-08)` commit that fails on assertions, followed by a `fix(02-08)` commit that passes. The commit type is `fix` rather than `feat` because the plan specifies user-visible fixes use `fix:`. No REFACTOR commit was needed.

## Deviations from Plan

None - plan executed exactly as written.

Notes that are not deviations:
- The RED commit of Task 2 also declares `const staffRetryAfterMaxSeconds = 3600`, besides the stubs the plan names, because the clamp test refers to it and the RED step must fail on assertions and not on compilation. The constant has no behaviour of its own.
- `staffRetryAfterWait` now calls `staffRetryAfterHold` and caps the result, which gives the same clamp in one place; its outputs for ordinary values are unchanged.

## Verification

- `go test -tags testtools -race -count=1 -run 'Staff|staff' ./alita/modules`: ok (111 s)
- Task 1 and Task 2 targeted verify commands: all listed tests `--- PASS`
- `go test -tags testtools -race -count=1 ./alita/utils/ratelimit ./alita/i18n`: ok
- `CGO_ENABLED=0 go build ./...`, `go vet -tags testtools ./alita/modules`: clean
- `make check-docs`: no drift; `make check-translations`: all present
- `git diff --exit-code go.mod go.sum`: unchanged
- `gofmt -l` on the four touched Go files: nothing
- All acceptance-criteria greps from both tasks match, including `cut off with the process` and `staffActionDeliverPartTimeout` in AGENTS.md
- `make lint` could not run: the installed golangci-lint was built with go1.25 and the module targets go1.26.0 ("can't load config"). `gofmt -l` and `go vet -tags testtools` stand in for it.
- Two commit-protocol steps were skipped: the harness refused shell writes into the worktree's git directory, so the spawn-toplevel sentinel and the commit ledger file could not be persisted there. The base in `plan_head_before` is the SHA given in the spawn prompt and `commits:` is measured with `git rev-list --count <base>..HEAD` (4, before this SUMMARY commit).

## Known Stubs

None.

## Threat Flags

None. No new endpoint, auth path or schema; all messages still go only to the Staff Group the card lives in.

## Issues Encountered

None. `ratelimit.TelegramPacer.Do` also multiplies retry_after by a unit without a clamp (`telegram_pacer.go`); that file belongs to plan 02-09 and was left alone.

## Self-Check: PASSED

- Files modified exist: staff_action_summary.go, staff_action_run.go, staff_action_progress_test.go, staff_action_fake_test.go, AGENTS.md
- Commits found: 8d894bc, ca10430, fff7c0d, 21818c5
