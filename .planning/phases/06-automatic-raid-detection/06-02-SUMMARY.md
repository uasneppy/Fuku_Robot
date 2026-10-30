---
phase: 06-automatic-raid-detection
plan: 02
subsystem: lockdown
tags: [raid-detection, join-surge, redis, lua, postgres-migration, gotgbot]

requires:
  - phase: 06-automatic-raid-detection
    provides: startLockdownRow, the trigger columns, lockdownAlertTriggerLine
provides:
  - chat_raid_detection (flags, thresholds 3..500, carry-over of the old auto threshold clamped per D-18 and D-22)
  - raiddetection.GetSettings and Defaults, a cached read where a missing row means both triggers on (15 and 30)
  - the alita:raid:{chat}:joined join counter and its Lua script, inert without Redis
  - raidCountMemberJoin in the group -7 chat_member guard and raidFire, the shared trigger start
affects: [06-03, 06-04, 06-05, 06-06, phase-07-ai-trigger]

actuals:
  tokens: 9500
  tasks: 2
  commits: 3

plan_head_before: 2268e3ebbae0822526dd6e1f7e4b17b752b87465
plan_head_after: 08bcf65d5bbcdbedfbd9bebdd974338eaaaa14ec

commits: 3

tech-stack:
  added: []
  patterns:
    - "Redis counters keyed by user ID in one hash-tagged sorted set, written by one Lua script"
    - "A counter that already made the live performer lookup hands the status to the guard"
    - "Settings read through GetFromCacheOrLoad with defaults on a missing row and an error on anything else"

key-files:
  created:
    - migrations/20261018130000_add_chat_raid_detection.sql
    - alita/db/models/raid_detection.go
    - alita/db/raiddetection/repository.go
    - alita/modules/raid_counters.go
    - alita/modules/raid_join.go
    - alita/modules/raid_trigger.go
    - alita/modules/raid_join_surge_test.go
  modified:
    - alita/modules/lockdown_guard.go
    - alita/modules/test_harness_test.go
    - AGENTS.md

key-decisions:
  - "The carry-over clamps with LEAST(GREATEST(x, 3), 500), so an old threshold above 500 cannot break the CHECK and abort startup (D-22)"
  - "The model has no default tag on its booleans; the defaults live in Go and in the SQL DEFAULTs"
  - "The counter's member is the user ID with ZADD (no NX), so both deliveries of a join are one member and a re-join refreshes it"
  - "The trigger runs synchronously in the join handler; a crash half way leaves an unconfirmed row the worker settles"
  - "The fire latch only sheds load; one lockdown is still uk_chat_lockdowns_active plus lockdown.Start"

patterns-established:
  - "raidCountMemberJoin returns (performerStatus, looked) and the guard skips its own lookup when looked"

requirements-completed: [RAID-01, RAID-06]

duration: 25min
completed: 2026-10-18
status: complete
---

# Phase 6 Plan 02: Join surge tracer Summary

**15 different accounts joining a supergroup within 60 seconds now lock it through the same startLockdownRow path as /lockdown, with the bot as starter and the surge's numbers on the row, and a group with no settings row is protected by default.**

## Performance

- **Duration:** about 25 min
- **Completed:** 2026-10-18T12:03Z
- **Tasks:** 2 (the tracer, then the settings pin)
- **Files modified:** 10

## Accomplishments

- The chat_member join guard (group -7) counts each qualifying join in a Redis sorted set keyed by user ID before it reads the lockdown. At the threshold, `raidFire` starts the lockdown through `startLockdownRow` with `trigger_kind` join_surge, `trigger_count` 15, `trigger_window` 60 and the bot as starter. The join that tips the threshold is handled by the lockdown it started (a pending joiner row the worker bans).
- The existing worker posts the Phase 5 alert to the linked Staff Group, and it carries the join surge trigger line and the counts line (before 15).
- A 16th join starts no second lockdown and makes no second `setChatPermissions` call.
- `chat_raid_detection` holds per-chat flags and thresholds (CHECK 3..500). A missing row means both triggers on at 15 and 30, a stored threshold or an off switch is followed, and an unreadable settings read counts nothing. The migration copies every non-zero old auto threshold, clamped into 3..500 in both directions.
- A join still makes at most one live `getChatMember` of its performer: the counter hands the status it read to the guard.
- AGENTS.md documents the settings table, the `alita:raid:*` keys, the join surge rule and the harness entry.

## Task Commits

1. **Task 1 (tracer): a join surge locks the group, and the alert names it**
   - `7833ff9` (test) TestRaidJoinSurge, with compile scaffolding
   - `fd4e94c` (feat) settings read, Redis counter, hook, trigger, AGENTS.md
2. **Task 2: each group's settings decide the join surge**
   - `08bcf65` (test) TestRaidJoinSurgeSettings (four cases; no implementation commit was needed, see below)

**Plan metadata:** the docs commit adding this file.

## TDD Gate Compliance

- Task 1 RED (`7833ff9`): `TestRaidJoinSurge/fifteen_self-joins_lock` failed on its own assertion, "no active lockdown after the 15th join in a minute". It was the target test and the failure was the planned behaviour, not a compile or fixture error. GREEN (`fd4e94c`) made it pass.
- Task 2: the four `TestRaidJoinSurgeSettings` cases passed when first run, because Task 1's `raidCountMemberJoin` already read the settings, honoured the off switch and the stored threshold, and counted nothing on a read error. This is an unexpected GREEN on the test commit, investigated rather than ignored. Following the plan's own allowance ("or keep the test commit alone when the Task 1 code passes every case, with a mutation check"), each case was shown able to fail:
  - removing the `settings.JoinEnabled` check from the fire condition failed "join surge off";
  - replacing the stored threshold with `DefaultJoinThreshold` failed "a stored threshold";
  - falling back to `raiddetection.Defaults` on a settings error failed "settings unreadable".
  The mutations were temporary and reverted (`git diff` of `raid_join.go` is clean against `fd4e94c`). There is therefore no `fix(06-02)` commit.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] RED commit carries compile scaffolding**
- **Found during:** Task 1 (RED)
- **Issue:** `raidCleanup` in the test names `models.ChatRaidDetection`, and the harness must migrate it, so a test-only commit could not compile. The plan allows scaffolding in a RED commit.
- **Fix:** `7833ff9` also holds the migration, the model and the harness `AutoMigrate` entry, so the model never exists without its schema in any commit. Everything else (repository, counter, hook, trigger) is in the GREEN commit.
- **Files modified:** `migrations/20261018130000_add_chat_raid_detection.sql`, `alita/db/models/raid_detection.go`, `alita/modules/test_harness_test.go`
- **Committed in:** `7833ff9`

---

**Total deviations:** 1 auto-fixed (1 blocking). **Impact on plan:** none on behaviour; only the file split between the RED and GREEN commits differs from the plan's wording. No existing test needed its join trigger switched off (step 11): the Lockdown/Unlockdown/Raid suites and a wider Join/Greet/Captcha/Antiraid/Staff run pass unchanged.

## Issues Encountered

- `make lint` cannot run here (golangci-lint is built with go1.25 and the module needs go1.26). `gofmt -l alita/modules alita/db migrations` printed nothing and `go vet -tags testtools ./alita/modules ./alita/db/models ./alita/db/raiddetection` was clean in its place.
- No PostgreSQL server exists in this sandbox and none was started. The migration is plain idempotent DDL plus one `INSERT ... ON CONFLICT DO NOTHING` (no transaction statements, no concurrent build); `go test ./alita/db/migrations` passes. The PostgreSQL proof (applying the file, the CHECK against the clamped carry-over) is CI's migration chain and plan 06-11's carry-over test. The model's `check:` tags ran on SQLite through `AutoMigrate`.

## Verification Run

| Check | Result |
|-------|--------|
| `go test -tags testtools -race -run '^TestRaidJoinSurge$\|^TestRaidJoinSurgeSettings$' ./alita/modules` | pass (1 + 4 subtests) |
| `go test -tags testtools -race -run 'Lockdown\|Unlockdown\|Raid' ./alita/modules ./alita/db/lockdown` | pass (after the last commit) |
| `go test -tags testtools -race -run 'Join\|Greet\|Captcha\|Antiraid\|AntiRaid\|Staff' ./alita/modules` | pass |
| `go test -tags testtools ./alita/db/migrations` | pass |
| `CGO_ENABLED=0 go build ./...`, `go vet`, `gofmt -l` | clean |
| Task 1 acceptance criteria (latest migration name, no BEGIN/COMMIT/CONCURRENTLY, clamp expression once, D-22 named, hook before `lockdownActiveLookup`, `startLockdownRow(` in the trigger, harness entry, AGENTS.md mentions) | all met |
| Task 2 acceptance criteria (5 `t.Run(` calls, gofmt) | met |
| `go.mod` and `go.sum` | unchanged |

The full `make test` was not run; the orchestrator runs it after the merge.

## Known Stubs

None.

## Threat Flags

None beyond the plan's register. T-06-05: the start is `lockdown.Start` behind `uk_chat_lockdowns_active`, the latch only sheds load, and `TestRaidJoinSurge` asserts one row and one `setChatPermissions` after a 16th join. T-06-06: user-ID members, 24 h trim, cap 5000 and a 25 h TTL on every write. T-06-07: a count only calls `startLockdownRow`; the counter bans nothing.

## User Setup Required

None - no external service configuration required. `AUTO_MIGRATE=true` in the deploy manifests applies the migration.

## Next Phase Readiness

The service-message path, the shared input builders and the exemption tests (06-03), the flood trigger (06-04) and `/autolockdown` writes with their `skipLocal` entry (06-05) can build on `raiddetection.GetSettings`, `recordRaidJoins` and `raidFire`. No blockers.

---
*Phase: 06-automatic-raid-detection*
*Completed: 2026-10-18*

## Self-Check: PASSED

Created files exist (`migrations/20261018130000_add_chat_raid_detection.sql`, `alita/db/models/raid_detection.go`, `alita/db/raiddetection/repository.go`, `alita/modules/raid_counters.go`, `alita/modules/raid_join.go`, `alita/modules/raid_trigger.go`, `alita/modules/raid_join_surge_test.go`); commits `7833ff9`, `fd4e94c` and `08bcf65` are in `git log`.
