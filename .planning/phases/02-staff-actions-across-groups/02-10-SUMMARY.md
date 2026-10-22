---
phase: 02-staff-actions-across-groups
plan: 10
subsystem: staff-actions
tags: [redis, lua, lock-renewal, flood-control, staff-actions, gap-closure]

requires:
  - phase: 02-staff-actions-across-groups
    provides: "02-05 per-target lock (SET NX, card token, compare-and-delete release); 02-08 coordinator select loop; 02-09 pacer MaxWait refusal"
provides:
  - "renewStaffTargetLockScript and renewStaffTargetLock: one Lua compare-and-set that extends the lock of the running card, re-takes a vanished key, and leaves another card's lock untouched"
  - "staffTargetLockRenewEvery (10 min, injectable): the coordinator's second ticker renews the target lock while the fan-out runs"
  - "AGENTS.md rules for the lock renewal and the pacer MaxWait refusal (completes WR-02)"
affects: [phase-02-verification, staff-actions]

actuals:
  tokens: 2300
  tasks: 2
  commits: 4

plan_head_before: 8b16c7c675c6250c2f199c720525a6d20075fa15
plan_head_after: 36f15f8c5d10c955a261704645bb2e2eb7555b94

tech-stack:
  added: []
  patterns:
    - "Lock renewal as a Redis Lua compare-and-set on the owner token, driven by a ticker inside the run's single coordinator goroutine, so the lock's life ends with the loop"

key-files:
  created: []
  modified:
    - alita/modules/staff_action_card.go
    - alita/modules/staff_action_run.go
    - alita/modules/staff_action_lifecycle_test.go
    - AGENTS.md

key-decisions:
  - "Renewal lives in the coordinator's select loop (second ticker), not a new goroutine; it stops when the fan-out ends and the deferred compare-and-delete release still frees the lock after delivery"
  - "A vanished key is re-taken with the run's token (script returns 2); another card's token is only logged at warn, never extended and never cancels the run"
  - "Interval is 10 minutes, a third of the 30-minute TTL, so two renewals can fail in a row before the lock expires"

patterns-established:
  - "Injectable interval var (staffTargetLockRenewEvery) plus a gated BotClient fake to hold a run open and test a ticker deterministically under -race"

requirements-completed: [PLAT-01, STAFF-12]

coverage:
  - id: D1
    description: "A live staff run renews its target lock every staffTargetLockRenewEvery back to the full 30-minute TTL, and the lock is released after the run"
    requirement: "PLAT-01"
    verification:
      - kind: unit
        ref: "alita/modules/staff_action_lifecycle_test.go#TestStaffActionTargetLockRenewedDuringRun/renews_its_own_lock"
        status: pass
    human_judgment: false
  - id: D2
    description: "Renewal re-takes a lock whose key vanished and never extends or takes over a lock holding another card's token"
    requirement: "PLAT-01"
    verification:
      - kind: unit
        ref: "alita/modules/staff_action_lifecycle_test.go#TestStaffActionTargetLockRenewedDuringRun/re-takes_a_lock_that_vanished"
        status: pass
      - kind: unit
        ref: "alita/modules/staff_action_lifecycle_test.go#TestStaffActionTargetLockRenewedDuringRun/leaves_a_foreign_lock_alone"
        status: pass
    human_judgment: false
  - id: D3
    description: "AGENTS.md states the lock renewal and the pacer MaxWait refusal; full gate green (make test, check-translations, check-docs, go vet, CGO_ENABLED=0 build)"
    requirement: "STAFF-12"
    verification:
      - kind: other
        ref: "make test; make check-translations; make check-docs; go vet -tags testtools ./...; CGO_ENABLED=0 go build ./..."
        status: pass
    human_judgment: false
  - id: D4
    description: "Under real Telegram flood control and two bot replicas, every card ends on a final summary, groups refused by Telegram read rate limited, and a second Confirm on the same target is answered target busy for as long as the first run lasts"
    requirement: "STAFF-12"
    verification: []
    human_judgment: true
    rationale: "Real Telegram flood limits and multi-replica timing cannot be reproduced by the hand-written fake and miniredis (plan human-check)"

duration: 30min
completed: 2026-10-05
status: complete
---

# Phase 2 Plan 10: Target lock renewal (WR-02) Summary

**A staff run now renews its per-target Redis lock every 10 minutes through a Lua compare-and-set on the card token, so a run slowed by flood control keeps exclusive hold of its target past the 30-minute TTL; a lock Redis lost is re-taken and another card's lock is never touched.**

## Performance

- **Duration:** about 30 min
- **Tasks:** 2 of 2
- **Files modified:** 4 (183 lines added, 3 removed)

## Accomplishments

- Closed the lock half of verification Gap 2 (code review WR-02): `renewStaffTargetLockScript` (`GET` equals token then `PEXPIRE` and return 1; key missing then `SET ... PX` and return 2; any other value returns 0 and writes nothing) and `renewStaffTargetLock(target, token) (held bool, err error)` in `alita/modules/staff_action_card.go`.
- The coordinator in `startStaffActionRun` has a `lockRenew` ticker (`staffTargetLockRenewEvery`) beside the edit ticker. An error or a lock found held by another card is logged at warn and the run continues; no group result changes. The ticker is stopped explicitly when the fan-out ends, so renewal never outlives the fan-out and the deferred compare-and-delete release still frees the lock after delivery.
- AGENTS.md carries both WR-02 rules: the renewal clause on the `alita:staff:lock:target:<id>` bullet and the pacer's "MaxWait (60 s) ... fails at once as rate limited, with no Telegram request and without taking the slot" sentence.

## Task Commits

1. **Task 1 RED:** `2555826` test(02-10): show the target lock expires under a long-running staff action
2. **Task 1 GREEN:** `be4f3e3` fix(02-10): renew the staff target lock while its run is alive so a slow run keeps it
3. **Task 2 RED:** `8a7f48c` test(02-10): cover a vanished and a foreign target lock during renewal
4. **Task 2 GREEN:** `36f15f8` fix(02-10): re-take a vanished staff target lock during renewal and document the WR-02 rules

## TDD Gate Compliance

RED then GREEN for both tasks, no refactor commit needed.

- Task 1 RED: `renews its own lock` failed on its assertion (`target lock TTL = 2s, want it renewed above 29m0s`; holder was correct), not on a compile or fixture error. GREEN: passes.
- Task 2 RED: `re-takes a lock that vanished` failed on its assertion (`target lock holder = "", want the running card ... to take it back`); `leaves a foreign lock alone` passed already, as the plan predicted, and guards the compare-and-set. GREEN: all three subtests pass.

## Tracer feedback gate

Task 1 is `type="tracer"` with an automated-only `<verify>`; both verify commands were re-run after the GREEN commit and passed (`TestStaffActionTargetLockRenewedDuringRun`, and the `TestStaffActionTargetLock*`, `TestStaffActionOwnCardDoubleTap`, `TestStaffActionConfirm*` regression set). Tracer verified end-to-end, then expanded to Task 2.

## Verification Results

| Check | Result |
|-------|--------|
| `go test -tags testtools -race -count=1 -run '^TestStaffActionTargetLockRenewedDuringRun$' -v ./alita/modules` | pass, three subtests |
| `go test ... -run '^TestStaffActionTargetLock\|^TestStaffActionOwnCardDoubleTap$\|^TestStaffActionConfirm'` | pass |
| `make test` (race, coverage, skip gate) | exit 0 |
| `make check-translations` | all present |
| `make check-docs` | no drift |
| `go vet -tags testtools ./...` | clean |
| `CGO_ENABLED=0 go build ./...` | clean |
| `git diff --exit-code go.mod go.sum` | unchanged |
| `gofmt -l alita main.go` | only the pre-existing `alita/modules/greetings_command_test.go` |
| `make lint` | could not run, see below |

Acceptance criteria greps (card.go declarations, run.go ticker and call, AGENTS.md `staffTargetLockRenewEvery`, flattened `without taking the slot` count 1, `PX` only in the re-take branch) all match.

**`make lint` could not run in this sandbox.** Exact error: `Error: can't load config: the Go language version (go1.25) used to build golangci-lint is lower than the targeted Go version (1.26.0)`. As in plans 02-06 and 02-07, `gofmt -l` on the touched files and `go vet -tags testtools ./...` were used instead; both are clean.

## Deviations from Plan

None - plan executed exactly as written.

Notes on harness, not deviations: `gsd_run query commit` was not used (known to fail in worktrees); commits were plain `git add <paths>` and `git commit`. Test helpers `targetLockPTTL` and `heldStaffRun` were added to the lifecycle test file to share the held-run set-up between the three subtests (the plan describes the same set-up three times).

## Known Stubs

None.

## Threat Flags

None. No new endpoints, auth paths or schema; the only new Redis traffic is the renewal script on the existing `alita:staff:lock:target:<id>` key.

## Pending Human Verification

The plan's `<human-check>` needs a real bot and real groups and was not faked. Steps:

1. Link several test groups to a test Staff Group; if possible run two bot replicas against the same Redis.
2. From the Staff Group run `/ban <numeric ID of a test account>` and Confirm, several times in quick succession on different targets, until Telegram answers 429.
3. While one run is still going, press Confirm on a second card for the same target.

Expected: every card ends on a final summary (or a new message carrying it) with no pending hourglass line; groups Telegram refused read "failed: rate limited, gave up after retries"; the card is not edited while Telegram's `retry_after` runs; the second card for the same target is answered "target busy" for as long as the first run is going; no run freezes the others.

## Self-Check: PASSED

- Files present: `alita/modules/staff_action_card.go`, `alita/modules/staff_action_run.go`, `alita/modules/staff_action_lifecycle_test.go`, `AGENTS.md`.
- Commits present: `2555826`, `be4f3e3`, `8a7f48c`, `36f15f8`; `git rev-list --count 8b16c7c..HEAD` is 4.
