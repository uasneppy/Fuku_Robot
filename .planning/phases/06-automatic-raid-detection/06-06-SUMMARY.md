---
phase: 06-automatic-raid-detection
plan: 06
subsystem: lockdown
tags: [raid-detection, redis, postgresql, idempotency, latch, cooldown]

requires:
  - phase: 06-automatic-raid-detection
    provides: raidFire, raidTrigger, the fire latch and join counters (06-02, 06-03), the flood watcher (06-04), startLockdownRow and its pinned outcomes (06-01)
provides:
  - raidFire stays silent, with no Telegram call, while the chat has an active lockdown
  - lockdown.RecentlyLiftedFresh and the 10 minute post-lift quiet period (raidLiftQuiet)
  - the fire latch length per startLockdownRow outcome (60 s, raidRetryLatch 10 s, raidCantLockLatch 10 minutes)
  - end-to-end proof that two triggers and two replicas give one lockdown and one alert
affects: [06-05, 06-07]

actuals:
  tokens: 9500
  tasks: 2
  commits: 4

plan_head_before: 946dd04c83f44b06bebf81d7ecbd97ebe074111a
plan_head_after: d6e253daf8c529d734e9b20c2f1b7396fd1b1e79

commits: 4

tech-stack:
  added: []
  patterns:
    - "Redis latch as load shedding only; exactly-once stays in conditional PostgreSQL writes"
    - "Concurrency test that deletes the latch in a loop while two dispatchers run, so the database alone must give one row"
    - "Latch length chosen by outcome in one switch (raidOutcomeLatch)"

key-files:
  created:
    - alita/db/lockdown/cooldown.go
    - alita/db/lockdown/cooldown_test.go
    - alita/modules/raid_trigger_test.go
  modified:
    - alita/modules/raid_trigger.go
    - AGENTS.md

key-decisions:
  - "The quiet period reads lift_started_at from PostgreSQL with real time (not raidNow), so it survives a Redis flush and needs no code on the lift path"
  - "raidFire order after the latch: GetActiveFresh, then RecentlyLiftedFresh, then startLockdownRow; a read error shortens the latch to 10 s and returns"
  - "NotSupergroup, BotCannotRestrict and LockFailed lengthen the latch to 10 minutes; BotCheckFailed, PermissionsUnreadable and StateFailed shorten it to 10 s; Started, AlreadyActive and LockUnknown keep 60 s"

requirements-completed: [LOCK-10]

coverage:
  - id: D1
    description: "A join surge and a flood in one group, in either order, give one lockdown, one lock call and one alert, and the second trigger makes no Telegram call"
    requirement: "LOCK-10"
    verification:
      - kind: integration
        ref: "alita/modules/raid_trigger_test.go#TestRaidTriggersSingleLockdown"
        status: pass
    human_judgment: false
  - id: D2
    description: "Two replicas handling one burst, with and without the fire latch, start exactly one lockdown and one alert copy"
    requirement: "LOCK-10"
    verification:
      - kind: integration
        ref: "alita/modules/raid_trigger_test.go#TestRaidTriggersConcurrent"
        status: pass
    human_judgment: false
  - id: D3
    description: "For 10 minutes after a lift the triggers start nothing and call nothing; after that they lock again"
    verification:
      - kind: integration
        ref: "alita/modules/raid_trigger_test.go#TestRaidCooldownAfterLift"
        status: pass
      - kind: unit
        ref: "alita/db/lockdown/cooldown_test.go#TestRecentlyLiftedFresh"
        status: pass
    human_judgment: false
  - id: D4
    description: "The fire latch lasts 60 s, 10 s or 10 minutes by outcome, and a held latch makes no Telegram call"
    verification:
      - kind: integration
        ref: "alita/modules/raid_trigger_test.go#TestRaidLatch"
        status: pass
    human_judgment: false

duration: 55min
completed: 2026-10-18
status: complete
---

# Phase 6 Plan 06: One raid, one lockdown Summary

**raidFire now stays silent while a lockdown is active, leaves a group alone for 10 minutes after an admin lifts it, and sets its Redis latch by start outcome; the join surge and the flood, on one replica or two, end in one lockdown and one alert.**

## Performance

- **Duration:** about 55 min
- **Completed:** 2026-10-18
- **Tasks:** 2
- **Files modified:** 5 (3 created)

## Accomplishments

- After the latch, `raidFire` reads `lockdown.GetActiveFresh`; an active row, confirmed or not, ends the attempt before any Telegram call. With the latch removed, the surge-then-flood and flood-then-surge cases show no extra `getChatMember` of the bot and no extra `getChat` after the lock, where before this plan each made one of each.
- `lockdown.RecentlyLiftedFresh(chatID, since)` is one fresh query on `(chat_id, lift_started_at)` (index from migration 20261018120000), any lockdown state, never cached. `raidFire` calls it with `time.Now() - raidLiftQuiet` (10 minutes); true means silence. A lift older than 11 minutes lets the next qualifying join lock again (`TestRaidCooldownAfterLift` moves `lift_started_at` back with a direct update).
- The latch is set by outcome in `raidOutcomeLatch`: 60 s for started, already active and lock unknown (the worker settles an unknown lock); `raidRetryLatch` (10 s) for a failed bot-rights check, unreadable permissions and a failed state write; `raidCantLockLatch` (10 minutes) for not a supergroup, a bot that cannot restrict, and a lock Telegram refused. An unreadable active or lift read also shortens it to 10 s.
- `TestRaidTriggersConcurrent` feeds 20 joins, 10 per dispatcher, from two goroutines on the same bot, fake, database and Redis. The "latch deleted" variant runs a third goroutine that deletes the latch in a loop, so every counted join past the threshold reaches the start path and the database alone has to give one `chat_lockdowns` row, one `setChatPermissions` call and one alert copy. Run 8 times under `-race` with no failure.
- AGENTS.md: the `alita:raid:{<chat>}:fire` bullet now states the active check, the quiet period and the latch lengths.

## Task Commits

1. **Task 1: two triggers and two replicas give one lockdown and one alert**
   - `bed6bc1` (test) TestRaidTriggersSingleLockdown, TestRaidTriggersConcurrent
   - `ea8efc4` (feat) active-lockdown check in raidFire, `raidRetryLatch`, `raidShortenLatch`
2. **Task 2: a lifted group stays quiet for 10 minutes, and the latch lasts by outcome**
   - `a8855fc` (test) TestRecentlyLiftedFresh, TestRaidCooldownAfterLift, TestRaidLatch, plus a stub `RecentlyLiftedFresh`
   - `d6e253d` (feat) the real query, `raidLiftQuiet`, `raidCantLockLatch`, `raidOutcomeLatch`, AGENTS.md

**Plan metadata:** the docs commit adding this file.

## TDD Gate Compliance

- Task 1 RED (`bed6bc1`) needed no scaffolding. `TestRaidTriggersSingleLockdown` failed on its own assertion in both subtests ("getChatMember calls about the bot = 3, want 1/2: a trigger that finds a lockdown active makes no Telegram call"). `TestRaidTriggersConcurrent` passed on the RED commit, as the plan anticipated: the database guarantee (`uk_chat_lockdowns_active` plus `lockdown.Start`) already gives one lockdown, so those cases pin that guarantee rather than the new code. GREEN (`ea8efc4`) made all pass; a mutation check (`if false && active != nil`) made both single-lockdown subtests fail again, then it was reverted.
- Task 1 test correction in the GREEN commit: the surge-then-flood subtest first counted calls that came from the flood watcher loading the chat's admin list on its first counted message (one `getChatMember` of the bot and one `getChat`), not from the trigger. The test now sends one item before taking its snapshot. The RED failure of the flood-then-surge subtest (which has a warm admin list) was already a true trigger-call failure.
- Task 2 RED (`a8855fc`) needed compile scaffolding: `alita/db/lockdown/cooldown.go` held a stub `RecentlyLiftedFresh` returning `false, nil`, because the test file calls it and Go needs it to compile. With it, `TestRecentlyLiftedFresh` failed on its assertions (three of five subtests), `TestRaidCooldownAfterLift` failed on "a lockdown started within 10 minutes of a lift", and four `TestRaidLatch` subtests failed on the latch length. GREEN (`d6e253d`) replaced the stub and made all pass.
- No `refactor(06-06)` commit was needed.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Missing critical functionality] Extra latch cases beyond the plan's four**
- **Found during:** Task 2
- **Issue:** the plan names "started", "transient", "cannot lock" and "latched". The outcome switch also has unreadable permissions, a refused lock and a lock whose answer was lost, and an untested branch of a switch that decides how long a failing group is left alone is a regression risk.
- **Fix:** `TestRaidLatch` also has "lock answer lost" (60 s kept, row kept unconfirmed), "permissions unreadable" (10 s) and "lock refused" (10 minutes, no row). "transient" also drops the latch and shows the next join locks.
- **Files modified:** `alita/modules/raid_trigger_test.go`
- **Commit:** `a8855fc`

**2. [Rule 3 - Blocking] raidRetryLatch and its helper arrived in Task 1**
- **Found during:** Task 1
- **Issue:** Task 1's read-error path shortens the latch to 10 s, but the plan lists `raidRetryLatch` under Task 2.
- **Fix:** Task 1 added `raidRetryLatch` and `raidShortenLatch`; Task 2 renamed the helper `raidSetLatch` (it now also lengthens) and added the other two constants.
- **Files modified:** `alita/modules/raid_trigger.go`
- **Commits:** `ea8efc4`, `d6e253d`

**Total deviations:** 2 auto-fixed (1 missing coverage, 1 ordering). **Impact:** none on behaviour; AGENTS.md edits are confined to the one `:fire` bullet.

## Issues Encountered

- `make lint` cannot run here (golangci-lint cannot lint Go 1.26); `gofmt -l` on the touched files and `go vet -tags testtools ./alita/modules ./alita/db/lockdown` stand in and are clean. `alita/modules/greetings_command_test.go` is not gofmt-clean on the base commit and was left alone.
- No PostgreSQL server was available or started; all tests used SQLite and miniredis. `TestRecentlyLiftedFresh` compares timestamps and runs on both backends, but only SQLite was exercised. The partial unique index and `BeginAlert` remain proven on PostgreSQL by `TestStartLockdownOneActivePerChat` and `TestBeginAlert` in `make test-postgres-integrity`; this plan adds no new guarantee that rests on a conditional update, so that target's `-run` list is unchanged.
- The full `make test` was not run (the orchestrator owns it).
- Plan 06-05 runs in parallel and writes `chat_raid_detection` rows through `/autolockdown`; this plan uses only a chat's default settings and shares no file with it except `AGENTS.md`, where only the `:fire` bullet changed.

## Verification

- `TestRaidTriggersSingleLockdown`, `TestRaidTriggersConcurrent`, `TestLockdownStartRow`, `TestRaidCooldownAfterLift`, `TestRaidLatch` (`./alita/modules`) and `TestRecentlyLiftedFresh` (`./alita/db/lockdown`): pass with `-tags testtools -race`, no "DATA RACE".
- `-run 'Lockdown|Unlockdown|Raid' ./alita/modules ./alita/db/lockdown`: pass (about 105 s for the modules package).
- `CGO_ENABLED=0 go build ./...`, `go vet -tags testtools ./alita/modules ./alita/db/lockdown`, `gofmt -l` on the touched files: clean. `git diff --exit-code go.mod go.sum`: clean.
- Acceptance criteria: `lockdown.GetActiveFresh(chat.Id)` in `raid_trigger.go`; `grep -c 't.Run('` in `raid_trigger_test.go` prints 10 (at least 3 required); `raidLiftQuiet = 10 * time.Minute`; `func RecentlyLiftedFresh(` in `cooldown.go`.

## Known Stubs

None. The RED-commit stub of `RecentlyLiftedFresh` was replaced in `d6e253d`.

## Threat Flags

None beyond the plan's threat model: T-06-19 (one lockdown, one alert: `TestRaidTriggersConcurrent` with and without the latch), T-06-20 (60 s latch, active check before any call, 10 minute latch for a group the bot cannot lock: `TestRaidLatch`) and T-06-21 (10 minute quiet period from `lift_started_at`: `TestRaidCooldownAfterLift`) are implemented as written.

## Next Phase Readiness

- Plan 06-07 can add the notice and the warning at the same `raidOutcomeLatch` switch; `raidFire` already logs one info line per outcome with the chat ID and trigger numbers.
- `/autolockdown` settings (06-05) change which triggers fire; they do not touch the checks added here.

## Self-Check: PASSED

- `alita/db/lockdown/cooldown.go`, `alita/db/lockdown/cooldown_test.go`, `alita/modules/raid_trigger_test.go`: found.
- Commits `bed6bc1`, `ea8efc4`, `a8855fc`, `d6e253d`: found in `git log`.
- `git rev-list --count 946dd04c83f44b06bebf81d7ecbd97ebe074111a..HEAD` was 4 before this file.

---
*Phase: 06-automatic-raid-detection*
*Completed: 2026-10-18*
