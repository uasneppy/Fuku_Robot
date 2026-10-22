---
phase: 01-staff-group-links
plan: 10
subsystem: staff-groups
tags: [telegram, redis, setnx, background-sweeper, gorm, staff-group]

requires:
  - phase: 01-staff-group-links
    provides: "recheckStaffGroup, recheckLink, applyLinkHealth, staffHealthFromBot, rekeyFromTelegramError, chat_status.FetchBotMember, staff.*Fresh repository reads (plans 01-07 and 01-08)"
provides:
  - "Hourly background recheck of every Staff Group and link, with a 1-5 minute jittered first run after startup"
  - "Redis lock alita:staff:sweep:lock so one replica runs each cycle; unguarded and still correct without Redis"
  - "staff.ListStaffGroupsFresh and staff.DeleteOrphanLinks"
  - "StartStaffSweeper / StopStaffSweeper wired into postInit and the shutdown manager"
affects: [02-staff-moderation-fanout, staff-panel]

actuals:
  tokens: 8500
  tasks: 3
  commits: 5

tech-stack:
  added: []
  patterns:
    - "Background loop lifecycle: mutex + cancel func + WaitGroup, Stop waits without holding the mutex (antiraid poller precedent)"
    - "Optional Redis SETNX cycle lock where failure to lock means run unguarded because every write is conditional"
    - "Per-cycle panic recovery inside the loop in addition to goroutine-level recovery"

key-files:
  created:
    - alita/modules/staff_sweeper.go
    - alita/modules/staff_sweeper_test.go
  modified:
    - alita/db/staff/repository.go
    - alita/db/staff/repository_test.go
    - main.go
    - main_test.go
    - AGENTS.md

key-decisions:
  - "DeleteOrphanLinks is a single DELETE ... RETURNING whose NOT IN (staff_groups.chat_id) condition is evaluated at write time, instead of select-then-delete-by-id, so a Staff Group created concurrently is never mistaken for missing"
  - "The lock is never released early; TTL is interval minus one minute (interval/2 when the interval is shortened below a minute, which only tests do)"
  - "Sweep skips (does not store) bot health when the bot lookup is unknown, and re-keys on a 400 migrate_to_chat_id instead"
  - "Orphan cleanup runs after taking the lock, so only the lock holder deletes"

patterns-established:
  - "Sweep test helpers use a sweep prefix so parallel plan 01-09 panel tests compile alongside in package alita/modules"

requirements-completed: [SETUP-06, SETUP-07]

coverage:
  - id: D1
    description: "A single sweep cycle removes a link whose owner changed and refreshes link bot health with no command run, each notice posted once and nothing on a repeat run"
    requirement: SETUP-06
    verification:
      - kind: unit
        ref: "alita/modules/staff_sweeper_test.go#TestStaffSweepTracer"
        status: pass
    human_judgment: false
  - id: D2
    description: "One replica sweeps per cycle via the Redis lock; the sweep skips when the lock is held and runs unguarded without Redis or on a Redis error"
    requirement: SETUP-06
    verification:
      - kind: unit
        ref: "alita/modules/staff_sweeper_test.go#TestStaffSweepLockOneRunner"
        status: pass
      - kind: unit
        ref: "alita/modules/staff_sweeper_test.go#TestStaffSweepSkipsWhenLockHeld"
        status: pass
      - kind: unit
        ref: "alita/modules/staff_sweeper_test.go#TestStaffSweepRunsWithoutRedis"
        status: pass
      - kind: unit
        ref: "alita/modules/staff_sweeper_test.go#TestStaffSweepRunsOnRedisError"
        status: pass
    human_judgment: false
  - id: D3
    description: "A Telegram error never deletes a link or Staff Group; a 400 migrate_to_chat_id re-keys the link; orphan links are deleted silently and their cache keys invalidated"
    requirement: SETUP-06
    verification:
      - kind: unit
        ref: "alita/modules/staff_sweeper_test.go#TestStaffSweepErrorsNeverDelete"
        status: pass
      - kind: unit
        ref: "alita/modules/staff_sweeper_test.go#TestStaffSweepRekeysOnMigrateError"
        status: pass
      - kind: unit
        ref: "alita/modules/staff_sweeper_test.go#TestStaffSweepDeletesOrphanLinks"
        status: pass
      - kind: unit
        ref: "alita/db/staff/repository_test.go#TestStaffRepoDeleteOrphanLinks"
        status: pass
    human_judgment: false
  - id: D4
    description: "Sweep stops promptly on cancel, starts idempotently, survives a panicking cycle, stops before the DB closes at shutdown, and startup still makes exactly two bot calls"
    requirement: SETUP-07
    verification:
      - kind: unit
        ref: "alita/modules/staff_sweeper_test.go#TestStaffSweepStopsOnCancel"
        status: pass
      - kind: unit
        ref: "alita/modules/staff_sweeper_test.go#TestStaffSweepLifecycleStartStop"
        status: pass
      - kind: unit
        ref: "alita/modules/staff_sweeper_test.go#TestStaffSweepLifecycleStartTwiceRunsOneLoop"
        status: pass
      - kind: unit
        ref: "alita/modules/staff_sweeper_test.go#TestStaffSweepCyclePanicDoesNotKillLoop"
        status: pass
      - kind: unit
        ref: "main_test.go#TestPostInitSetsCommandsAndStartupMessage"
        status: pass
    human_judgment: false
  - id: D5
    description: "A non-admin bot can read the admin list (getChatAdministrators) and the link is removed within one sweep after an ownership transfer in a group where the bot was demoted to a plain member (research assumptions A2 and A5)"
    requirement: SETUP-06
    verification: []
    human_judgment: true
    rationale: "Whether a non-admin bot can call getChatAdministrators and whether chat_owner_changed still reaches it is Telegram server behaviour that only a live group shows; the fake used in tests scripts both answers"

duration: 38min
completed: 2026-10-04
status: complete
plan_head_before: 5ce8c9124146793b493673e2fdbd7805b62e8194
commits: 5
plan_head_after: 69fdf26ba678d49dd491f461e516d692954a5090
---

# Phase 1 Plan 10: Hourly Staff Link Sweeper Summary

**Hourly background sweep that rechecks every Staff Group and link live (first run 1-5 minutes after startup), guarded by a Redis SETNX lock, never deleting on a Telegram error, re-keying migrated chats and cleaning orphan links**

## Performance

- **Duration:** 38 min
- **Started:** 2026-10-04T23:19:00Z
- **Completed:** 2026-10-04T23:57:00Z
- **Tasks:** 3 (1 tracer, 2 TDD)
- **Files modified:** 7 (2 created)

## Accomplishments

- `runStaffSweep` reuses `recheckStaffGroup` and `applyLinkHealth`, so a stale link disappears with the D-13 notice, and a bot-health change posts one heads-up, with no command run. A repeat sweep sends nothing.
- Only one replica runs a given cycle: `SETNX alita:staff:sweep:lock` with a TTL one minute below the interval. Without Redis, or on a Redis error, the sweep runs unguarded and stays correct because each write is one conditional statement.
- Telegram errors never delete a link or a Staff Group (only a successful answer showing a different or missing creator does); a 400 carrying `migrate_to_chat_id` re-keys the chat via `rekeyFromTelegramError`; orphan links (Staff Group row gone) are deleted silently by `staff.DeleteOrphanLinks` with their cache keys invalidated.
- Calls are paced at 250ms, the sweep stops promptly on cancel, and a panic in one cycle is recovered so the loop keeps going.
- `StartStaffSweeper` is called from `postInit`; its stop handler is registered after the DB-close handler, so LIFO shutdown stops it before the database closes. AGENTS.md documents the `alita:staff:*` key family and the sweeper rule.

## Task Commits

1. **Task 1: Tracer, one sweep cycle removes a stale link and refreshes health** - `0d342c6` (feat)
2. **Task 2 RED: failing tests for lock, orphans, re-key** - `28df6b3` (test)
3. **Task 2 GREEN: one runner, orphan cleanup, migrate re-key** - `604e024` (feat)
4. **Task 3 RED: failing lifecycle tests** - `76b5eea` (test)
5. **Task 3 GREEN: lifecycle wired into startup and shutdown, AGENTS.md** - `69fdf26` (feat)

**Tracer gate:** interactive run, `human_verify_mode=end-of-phase`, tracer `<verify>` is `<automated>` only, so it was re-run, passed, and expansion continued (no checkpoint).

## Files Created/Modified

- `alita/modules/staff_sweeper.go` - sweep cycle, Redis lock, pacing, lifecycle (Start/Stop), first-delay jitter
- `alita/modules/staff_sweeper_test.go` - 12 sweep tests, all helpers `sweep`-prefixed
- `alita/db/staff/repository.go` - `ListStaffGroupsFresh`, `DeleteOrphanLinks`
- `alita/db/staff/repository_test.go` - tests for both
- `main.go` - `modules.StartStaffSweeper(b)` in `postInit`; shutdown handler for `StopStaffSweeper`
- `main_test.go` - `modules.StopStaffSweeper()` in the postInit test cleanup
- `AGENTS.md` - `alita:staff:*` in the operational key list, sweeper bullet in "Subsystem traps"

## Decisions Made

- `DeleteOrphanLinks` is one `DELETE ... WHERE staff_chat_id NOT IN (SELECT chat_id FROM staff_groups) RETURNING`, rather than select then delete by id. The condition is evaluated at write time, so a Staff Group created in between is never treated as missing; the same RETURNING pattern is already used by `DeleteLink`.
- The lock TTL falls back to `interval/2` when the interval is under one minute. Production uses one hour, so the TTL is 59 minutes; the fallback only matters for tests with a shortened interval.
- Orphan cleanup runs after the lock is taken, so only one replica deletes.
- On an unknown bot lookup the sweep stores nothing (the plan's "when the result is known"), because `staffHealthFromBot` would otherwise map unknown to `ok`.

## Deviations from Plan

None - plan executed exactly as written. Two minor implementation notes, neither a rule deviation:

- `DeleteOrphanLinks` uses the single-statement form described under Decisions Made instead of the plan's "select, then delete by id in one transaction". Behaviour and the stated return value (count) are the same.
- The plan listed the lifecycle behaviour with one test; it is split into `TestStaffSweepLifecycleStartStop` (50ms interval, at least one cycle, prompt Stop, second Stop a no-op) and `TestStaffSweepLifecycleStartTwiceRunsOneLoop` (200ms interval) because the two assertions need different intervals. Extra test `TestStaffSweepRunsOnRedisError` covers the "on a Redis error" truth.

## TDD Gate Compliance

Task 2 and Task 3 each have a `test(01-10)` commit before their `feat(01-10)` commit. In each RED commit the target tests failed on assertions (lock tests got `Ran:true` twice or while the lock was held; re-key test found no row under the new ID; orphan tests found `Orphans 0`; lifecycle tests saw 0 cycles), using no-op placeholders so everything compiled. A few Task 2 tests (`ErrorsNeverDelete`, `StopsOnCancel`, `RunsWithoutRedis`, `RunsOnRedisError`) already passed against the Task 1 sweep, since they pin behaviour the tracer had already established; they stay as regression guards. The persisted `check tdd-red-evidence` record was not produced because this plan is `type: execute` (task-level `tdd="true"`), not a `type: tdd` plan.

## Issues Encountered

None. `-race -count=5` on the sweep tests was stable.

## Known Stubs

None. The RED-phase placeholders (`DeleteOrphanLinks`, `StartStaffSweeper`, `StopStaffSweeper`) were replaced in the GREEN commits.

## Pending Human Verification

Task 2's `<human-check>` (research A2/A5) was **not run**: it needs a live Telegram supergroup, and in `end-of-phase` mode it is consolidated into the phase's UAT. Procedure: demote the bot to a plain member in a linked throwaway supergroup, transfer that group's ownership, then wait for (or restart the bot to trigger) the next sweep. Expected: the link is removed and the Staff Group gets exactly one "its owner changed" notice; `getChatAdministrators` succeeded for the non-admin bot (no "unknown" log for that group). Record whether `chat_owner_changed` still reached the bot. If the non-admin admin list is not readable, those links stay unknown and are never removed until the bot is admin (the documented A2 fallback).

## User Setup Required

None - no external service configuration required.

## Threat Flags

None. The only new surface is the Redis key `alita:staff:sweep:lock`, covered by the plan's T-01-33; no new endpoints, auth paths or schema.

## Next Phase Readiness

- Phase 2's pre-action check can rely on the same `recheckLink`/`recheckStaffGroup` the sweep uses.
- Verification run: `go test -tags testtools -race -count=1 -timeout 10m ./...` passes, `go vet -tags testtools ./...` clean, `make check-translations` and `make check-docs` clean, `go.mod`/`go.sum` unchanged.
- Parallel plan 01-09 touches disjoint files; this plan's test helpers carry a `sweep` prefix to avoid clashes in package `alita/modules`.

## Self-Check: PASSED

- Created files exist: `alita/modules/staff_sweeper.go`, `alita/modules/staff_sweeper_test.go`, this SUMMARY.
- Commits found in `git log`: `0d342c6`, `28df6b3`, `604e024`, `76b5eea`, `69fdf26`.
- Acceptance criteria re-run: `runStaffSweep` and `ListStaffGroupsFresh` signatures match; no `IsUserAdmin|LoadAdminCache|RequireUserOwner` in the sweeper; `staffSweepLockKey` and `SetNX(` present; `modules.StartStaffSweeper(b)` and `modules.StopStaffSweeper()` in `main.go`; `alita:staff:` in `AGENTS.md`; `RecoverFromPanic` appears twice in the sweeper; 9 `TestStaffSweep` tests plus 3 lifecycle tests pass.

---
*Phase: 01-staff-group-links*
*Completed: 2026-10-04*
