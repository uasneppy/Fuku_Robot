---
phase: 04-manual-lockdown
plan: 06
subsystem: lockdown
tags: [telegram, lockdown, durability, restart, redis, stale-claims, gotgbot, postgres]

requires:
  - phase: 04-manual-lockdown
    provides: "plans 04-01..04-05: the joiner state machine (acting and unbanning claims), the paced worker for bans, unbans and declines, the lift and tally, the guard on all three join paths, fetchLockdownChat and samePermissions"
provides:
  - "lockdown.ReleaseStaleClaims: one transaction that gives back claims older than the cut-off without counting an attempt"
  - "Every worker cycle releases stale claims first (lockdownStaleClaim, 2 minutes) so any replica resumes a ban, decline or unban after a restart"
  - "Unconfirmed-lock settling (settleUnconfirmedLockdown, lockdownSettleUnconfirmed, ListUnconfirmedFresh, TouchLockdown): confirmed from the live permissions or deleted, never announced"
  - "/lockdown settles an unconfirmed row older than the grace before it reports it or starts a new lockdown"
  - "Worker lifecycle with one done channel per run, so Start after a Stop that timed out shares nothing with the cut-off worker"
  - "Durability tests: restart mid-lift, stop during a blocked call, retry accounting, unconfirmed locks, Redis flush and absence, never auto-lift"
affects: [04-08, Phase 6 (retires antiraid), live-Telegram UAT]

actuals:
  tokens: 11700
  tasks: 2
  commits: 4

tech-stack:
  added: []
  patterns:
    - "Stale-claim release as the first step of every worker cycle: no lease table, the claimed_at stamp plus a cut-off far above the call timeout"
    - "A row that another replica may still be working on (an unconfirmed lock younger than the grace) is never settled by anyone else"
    - "Tests wait with polling and a deadline, never a bare sleep, and age rows with direct updates to pass a two-minute mark"

key-files:
  created:
    - alita/modules/lockdown_durability_test.go
  modified:
    - alita/db/lockdown/joiners.go
    - alita/db/lockdown/joiners_test.go
    - alita/db/lockdown/repository.go
    - alita/db/lockdown/repository_test.go
    - alita/modules/lockdown_worker.go
    - alita/modules/lockdown.go
    - AGENTS.md

key-decisions:
  - "The age gate (unconfirmed for over a minute) also applies when /lockdown meets an unconfirmed row: a younger row may still be its own command's between Start and ConfirmLocked, and deleting it would leave a locked group with no row. The plan only named the worker's grace; the command path needed it too."
  - "ReleaseStaleClaims runs at the start of runLockdownCycle, so a claim released in a cycle is retried in the same cycle (a stale unbanning row is unbanned at once, not one cycle later as the plan's test text described)."
  - "The worker lifecycle uses a per-run done channel instead of a shared sync.WaitGroup: a Stop that timed out left a waiter goroutine on the WaitGroup, and a later Start's Add raced with it (the race detector flagged it)."
  - "/lockdown re-reads the active row after settling instead of trusting the settle result, so a row another replica settled first is reported or replaced correctly."

patterns-established:
  - "Settling reads the live permissions through the same pacer as every worker call and never posts a message"
  - "A failed read of the permissions touches updated_at and retries after another grace period, not on every cycle"

requirements-completed: [LOCK-05]

coverage:
  - id: D1
    description: "A restart in the middle of a lift resumes it: a claim older than 2 minutes is released and finished by any replica, a live claim of another replica is left alone, the lockdown ends lifted and exactly one tally is posted"
    requirement: "LOCK-05"
    verification:
      - kind: integration
        ref: "alita/modules/lockdown_durability_test.go#TestLockdownLiftResumesAfterRestart"
        status: pass
      - kind: integration
        ref: "alita/db/lockdown/joiners_test.go#TestReleaseStaleClaims"
        status: pass
    human_judgment: false
  - id: D2
    description: "StopLockdownWorker returns within its wait while a Telegram call is blocked, and a new worker finishes the lift within seconds"
    requirement: "LOCK-05"
    verification:
      - kind: integration
        ref: "alita/modules/lockdown_durability_test.go#TestLockdownStopDuringCall"
        status: pass
    human_judgment: false
  - id: D3
    description: "A rate-limited ban is retried without counting an attempt; a ban that fails three times is ban_failed with Telegram's escaped reason and counted by /lockdownstatus"
    requirement: "LOCK-05"
    verification:
      - kind: integration
        ref: "alita/modules/lockdown_durability_test.go#TestLockdownBanRetriesAndFailures"
        status: pass
    human_judgment: false
  - id: D4
    description: "An unconfirmed lock older than a minute is settled from the live permissions (confirmed or deleted, nothing announced); /lockdown settles it first; a younger row is left alone"
    requirement: "LOCK-05"
    verification:
      - kind: integration
        ref: "alita/modules/lockdown_durability_test.go#TestLockdownSettlesUnconfirmedLock"
        status: pass
      - kind: integration
        ref: "alita/db/lockdown/repository_test.go#TestListUnconfirmed"
        status: pass
    human_judgment: false
  - id: D5
    description: "Flushing Redis or running with no Redis changes nothing but pacing: joiners are recorded and banned, status and /unlockdown work, the lift unbans everyone"
    requirement: "LOCK-05"
    verification:
      - kind: integration
        ref: "alita/modules/lockdown_durability_test.go#TestLockdownSurvivesRedisFlush"
        status: pass
    human_judgment: true
    rationale: "The tests use miniredis and a nil client; how the production pacer behaves against a real Redis outage mid-run is a live check"
  - id: D6
    description: "A lockdown never lifts on its own: 400 days old with every ban expired, 20 worker cycles leave it active with no setChatPermissions, unbanChatMember or message"
    requirement: "LOCK-05"
    verification:
      - kind: integration
        ref: "alita/modules/lockdown_durability_test.go#TestLockdownNeverLiftsOnItsOwn"
        status: pass
    human_judgment: false

duration: ~45 min
completed: 2026-10-07
status: complete

plan_head_before: 9f22decaca7994f1bd73413cf13bd08abfce640c
plan_head_after: 9bc14d23aa275ae1436bfad7fee494a5df0a9e46
---

# Phase 4 Plan 06: Durability of lockdowns Summary

**A lockdown survives restarts, a stopped worker and a lost Redis: stale claims are released after 2 minutes by any replica, a half-started lock is settled from the live permissions, and nothing but /unlockdown ever ends a lockdown.**

## Performance

- **Duration:** about 45 min
- **Tasks:** 2 (Task 1 tracer, Task 2 auto, both TDD)
- **Commits:** 4 (2 test, 2 feat); measured `git rev-list --count 9f22dec..HEAD` before this summary commit
- **Files:** 8 touched by the code commits (1 created, 7 modified)

## Accomplishments

- **Stale claims** (`lockdown.ReleaseStaleClaims`, `lockdownReleaseStale`): the first step of every cycle gives back claims older than `lockdownStaleClaim` (2 min) in one transaction of four disjoint conditional updates, with no attempt counted: acting goes to pending in an active lockdown, to cancelled (request) or banned (ban) in a lifting one; unbanning goes to banned. The lift's live `getChatMember` check keeps a repeated unban safe.
- **Restart mid-lift** (tracer): a worker's abandoned unbanning claim is released and finished by the next cycle, another replica's live claim is untouched, and once it passes 2 minutes it is taken over; the lockdown ends lifted with one tally.
- **Worker lifecycle**: `StopLockdownWorker` returns within its wait even while a Telegram call is blocked (tested with a 1 s delayed unban and a 100 ms wait), and a new `StartLockdownWorker` finishes the lift. The shared `sync.WaitGroup` was replaced by a per-run done channel after the race detector showed a Start racing with a Stop that had timed out.
- **Retry accounting**: pinned by `TestLockdownBanRetriesAndFailures` (four 429 answers leave the row pending with 0 attempts; three 400 answers end `ban_failed` with the escaped reason, and `/lockdownstatus` counts it). The existing accounting was already correct, so no production change was needed there.
- **Unconfirmed locks** (`settleUnconfirmedLockdown`, `lockdownSettleUnconfirmed`, `ListUnconfirmedFresh`, `TouchLockdown`): a row with `locked_at` NULL for over a minute is confirmed when the live permissions equal the locked set and deleted otherwise, never announced; an unreadable answer touches the row and is retried after another grace period. `/lockdown` settles such a row first, then reports it (`lockdown_already_active`), starts a new lockdown, or refuses with `lockdown_permissions_unreadable`.
- **Redis**: a flush and a missing client leave the lockdown working (joiners recorded and banned, status, `/unlockdown`, lift); only the pacer falls back to one replica's interval.
- **Never auto-lift**: a 400-day-old lockdown with expired bans stays active through 20 cycles with no write; `BeginLift` is reached only through `lockdownBeginLift` in `unlockdown`.
- **AGENTS.md**: stale claim release, unconfirmed rows, no-Redis operation and never-auto-lift added to the shutdown and data sections.

## Task Commits

1. **Task 1 (tracer): restart mid-lift, stop during a call, retry accounting** - `eb1db09` (test, RED), `8e2c9db` (feat, GREEN)
2. **Task 2: unconfirmed locks, Redis loss, never auto-lift** - `efe28db` (test, RED), `9bc14d2` (feat, GREEN)

**Plan metadata:** committed after this file (`docs(04-06): complete durability plan`).

## TDD Gate Compliance

Both tasks have a `test(04-06)` commit before their `feat(04-06)` commit. RED evidence (target tests failing on the planned behaviour, not on a build error):

- Task 1: `TestReleaseStaleClaims` ("ReleaseStaleClaims released 0 rows, want the 6 stale ones" and each row still claimed); `TestLockdownLiftResumesAfterRestart` ("joiner A state = unbanning, want unbanned", lockdown still lifting, no tally); `TestLockdownStopDuringCall` (data race between a timed-out Stop and the next Start). The RED commit carries a compile-only `ReleaseStaleClaims` placeholder returning `0, nil`.
- Task 2: `TestListUnconfirmed` (no rows listed, updated_at not moved), `TestLockdownSettlesUnconfirmedLock` subtests "lock took effect", "lock never applied", "permissions unreadable" and the three "/lockdown meets" subtests failing on the missing settling. The RED commit carries compile-only `ListUnconfirmedFresh` and `TouchLockdown` placeholders.
- Already green at RED, expected: `TestLockdownBanRetriesAndFailures`, `TestLockdownSurvivesRedisFlush`, `TestLockdownNeverLiftsOnItsOwn` and the "younger than a minute" subtests pin behaviour plans 04-01..04-05 already had (the plan says to fix whatever the retry test exposes, and it exposed nothing). The `gsd_run check tdd-red-evidence` record was not produced (the plan is `type: execute`, not `type: tdd`).

Tracer gate: the tracer's automated `<verify>` passed end to end (logged "Tracer verified end-to-end, expanding"); the live-Telegram check is an end-of-phase UAT item below, not a stop.

## Decisions Made

See `key-decisions` above.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Shared WaitGroup raced with a Start after a timed-out Stop**
- **Found during:** Task 1 (`TestLockdownStopDuringCall`, run under `-race`)
- **Issue:** `StopLockdownWorker` waited on `lockdownWorkerWG` in a helper goroutine that could outlive its own timeout; a later `StartLockdownWorker` called `Add(1)` on the same WaitGroup, which the race detector reported. The plan requires a new worker to finish the lift after a stopped one.
- **Fix:** one `done` channel per run (`lockdownWorkerDone`), closed when the worker goroutine ends; Stop waits on it with the timeout and clears the state only when it is still its own run.
- **Files modified:** alita/modules/lockdown_worker.go
- **Verification:** `TestLockdownStopDuringCall` passes 25 times in a row under `-race`.
- **Committed in:** 8e2c9db

**2. [Rule 2 - Missing Critical] /lockdown settles an unconfirmed row only after the grace**
- **Found during:** Task 2 (designing the D-14 branch)
- **Issue:** the plan runs the settling "synchronously when /lockdown meets an unconfirmed row" with no age condition. A row younger than a minute may be its own command's, between `Start` and `ConfirmLocked`; settling it reads permissions that are not locked yet and would delete it, leaving a locked group with no row.
- **Fix:** the command settles only a row older than `lockdownUnconfirmedGrace`; a younger one is reported as already active. It re-reads the active row after settling.
- **Files modified:** alita/modules/lockdown.go
- **Verification:** subtest "lockdown meets a young unconfirmed row"
- **Committed in:** 9bc14d2

**3. [Plan text] Restart test expectation adjusted to the specified ordering**
- **Found during:** Task 1 (writing `TestLockdownLiftResumesAfterRestart`)
- **Issue:** the plan's test text has a stale unbanning row released "back to banned" in one cycle and unbanned "the next cycle", but its own step 3 runs the release first in the cycle, so the row is unbanned in the same cycle.
- **Fix:** the test asserts the specified implementation: A is released and unbanned in the first cycle, B (live claim) is untouched until its claim passes 2 minutes, then unbanned; one tally with 4 unbanned.
- **Files modified:** alita/modules/lockdown_durability_test.go
- **Committed in:** eb1db09

---

**Total deviations:** 3 (1 bug, 1 missing critical, 1 plan-text adjustment). **Impact on plan:** none changes the specified behaviour; the first two close edges the plan did not cover.

## Issues Encountered

- This environment's Bash tool refused compound commands and heredocs combined with other commands; they were split into plain separate calls.
- A first version of `TestLockdownStopDuringCall` asserted the tally right after the lockdown read `lifted`, but the tally is posted just after that update; it flaked once in 15 runs. The test now waits for the tally message before asserting exactly one.
- Pre-existing, not touched: `gofmt -l alita/modules` lists `greetings_command_test.go` (noted in 04-04 and 04-05).

## Known Limitations (carried forward, not stubs)

- A claim is released two minutes after it was made, so a restart can leave a joiner banned (during a lift) for up to two minutes while its row is still claimed; every other row of the lift proceeds meanwhile.
- A `/lockdown` that reaches Telegram but dies before `ConfirmLocked` leaves the admin without an answer; the row is later confirmed silently (nothing is announced), as the plan decided.
- The Redis tests use miniredis and a nil client; a real mid-run Redis outage uses the same `reserveLocal` fallback but is not exercised live.

## Known Stubs

None.

## Threat Flags

None beyond the plan's `<threat_model>`: T-04-20 (crash mid-lift leaving joiners banned: `TestLockdownLiftResumesAfterRestart`, `TestLockdownStopDuringCall`), T-04-21 (Redis flush or outage: `TestLockdownSurvivesRedisFlush`) and T-04-22 (unconfirmed lock row: `TestLockdownSettlesUnconfirmedLock`) are mitigated and tested; T-04-SC is accepted (no package installs, `go.mod` and `go.sum` unchanged).

## Verification Run

- Named plan tests, all with `-tags testtools -race -count=1` and passing: `TestLockdownLiftResumesAfterRestart`, `TestLockdownStopDuringCall`, `TestLockdownBanRetriesAndFailures`, `TestReleaseStaleClaims`, `TestLockdownSettlesUnconfirmedLock` (8 subtests), `TestLockdownSurvivesRedisFlush`, `TestLockdownNeverLiftsOnItsOwn`, `TestListUnconfirmed`. The restart, stop and retry tests also passed 25 runs in a row under `-race`.
- Plan-level: `go test -tags testtools -race -count=1 ./alita/modules ./alita/db/lockdown ./alita/db` ok; `git diff --exit-code go.mod go.sum` unchanged.
- `make test`: exit 0 (70.0% coverage, no FAIL lines in the log). The known flaky `TestStaffActionNonStaffUnchanged/tmute_as_a_reply` did not fail.
- `CGO_ENABLED=0 go build ./...`, `go vet -tags testtools ./alita/modules ./alita/db/lockdown .`, `make check-translations` (all present, no new keys), `make check-docs` (no drift), `gofmt -l` on the touched Go files (clean): all pass.
- Acceptance criteria re-run, all pass: `lockdownStaleClaim = 2 * time.Minute`, `lockdown.ReleaseStaleClaims(` and `lockdownUnconfirmedGrace = time.Minute` in lockdown_worker.go; `settleUnconfirmedLockdown(` in lockdown.go; the `BeginLift(` count outside lockdown.go prints 0; AGENTS.md contains "older than 2 minutes"; `git log` shows each `test(04-06)` before its `feat(04-06)`.
- `make lint`: not run. The installed golangci-lint 2.5.0 is built with go1.25 and refuses the go1.26.0 module (environment limitation).
- PostgreSQL: not run. No throwaway cluster was set up. `ReleaseStaleClaims` uses `lockdown_id IN (SELECT id FROM chat_lockdowns WHERE state = ?)` and `ListUnconfirmedFresh` a plain comparison on `updated_at`; both ran on SQLite only. Command left for the orchestrator or CI, with the migration chain applied first: `DATABASE_URL=<postgres url> ALITA_TEST_DATABASE=true CGO_ENABLED=1 go test -tags testtools -race -count=1 ./alita/db/lockdown`.

## End-of-Phase UAT Items (live Telegram, not run)

1. **Restart mid-lift.** In a test supergroup lock, let several accounts join (each is banned), run `/unlockdown`, and restart the bot while the unbans run. After the restart the remaining unbans finish (within about two minutes for any row that was mid-call) and one tally is posted.
2. **Stop with a call in flight.** Stop the bot during a lift (SIGTERM): it exits within the shutdown budget, and a restart resumes the lift.
3. **Half-started lock.** Kill the bot right after `/lockdown` is accepted but before it answers: after a minute either the group is locked and `/lockdownstatus` reports it, or `/lockdown` works again.
4. **Redis outage.** With a lockdown active, stop Redis: joiners are still banned, `/lockdownstatus` and `/unlockdown` work, and the lift unbans everyone.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- Lockdown state is now resumable end to end; 04-07 and later plans can rely on any replica finishing work a stopped worker left.
- No blockers.

---
*Phase: 04-manual-lockdown*
*Completed: 2026-10-07*

## Self-Check: PASSED
