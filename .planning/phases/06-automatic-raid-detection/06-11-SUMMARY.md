---
phase: 06-automatic-raid-detection
plan: 11
subsystem: database
tags: [raid-detection, developer-stats, postgres, migration-test, gorm]

requires:
  - phase: 06-automatic-raid-detection
    provides: "chat_raid_detection table, ChatRaidDetection model, its carry-over migration 20261018130000 (plans 06-02, 06-05)"
provides:
  - "raiddetection.LoadRaidDetectionStats: custom-settings, join-surge-off and flood-off chat counts"
  - "Developer /stats Raid detection lines in place of the AntiRaid lines (D-20)"
  - "TestRaidDetectionCarryOver: PostgreSQL-only proof of the migration's threshold carry-over (D-03, D-18, D-22)"
affects: [06-13 Makefile test-postgres-integrity list, 06-12 docs]

plan_head_before: d1ab55c1beee7a89271f564a6bb0167ec3f5d7bb
plan_head_after: b1fd5acabc32c8cc020eea8fe9f19403eed87bf2

actuals:
  tokens: 3900
  tasks: 2
  commits: 3

tech-stack:
  added: []
  patterns:
    - "Stats counters are thin db.CountRows wrappers in the domain package (copied from LoadAntiRaidStats)"
    - "Migration data-statement tests reuse the runner's cleanSupabaseSQL/splitSQLStatements and roll back a single transaction"

key-files:
  created:
    - alita/db/raiddetection/stats.go
    - alita/db/raiddetection/stats_test.go
    - alita/db/migrations/raid_carry_over_test.go
  modified:
    - alita/db/devs/repository.go
    - alita/db/devs/repository_test.go
    - alita/db/devs/testmain_test.go

key-decisions:
  - "Stats lines: Raid detection with Custom settings, Join surge off and Flood off; chats without a row use the defaults and are not counted"
  - "The carry-over test lives in package migrations and executes the migration file exactly as the runner would, then again to prove idempotence"
  - "TestLoadRaidDetectionStats counts the change from a baseline, because other tests in the package share the database"

requirements-completed: [LOCK-15]

coverage:
  - id: D1
    description: "Developer /stats reports chats with their own raid detection settings, chats with the join surge off and chats with the flood off from chat_raid_detection"
    requirement: LOCK-15
    verification:
      - kind: unit
        ref: "alita/db/raiddetection/stats_test.go#TestLoadRaidDetectionStats"
        status: pass
      - kind: unit
        ref: "alita/db/devs/repository_test.go#TestLoadAllStats_IncludesModuleCounts"
        status: pass
      - kind: unit
        ref: "alita/db/devs/repository_test.go#TestLoadAllStats"
        status: pass
    human_judgment: false
  - id: D2
    description: "PostgreSQL test proving the migration's carry-over: clamp 1 to 3 and 900 to 500, keep 20, no row for 0, keep an existing row, idempotent"
    requirement: LOCK-15
    verification: []
    human_judgment: true
    rationale: "The test needs a migrated PostgreSQL server, which does not exist in this environment; it compiles and skips here and runs in CI (make test-postgres-integrity once plan 06-13 lists it). No pass was observed here."

duration: 20min
completed: 2026-10-18
status: complete
---

# Phase 06 Plan 11: Raid detection stats and carry-over proof Summary

**The developer /stats now counts the new chat_raid_detection settings instead of the retired antiraid table, and a rolled-back PostgreSQL test (CI only) checks that the migration clamps and carries old auto thresholds into join surge thresholds.**

## Performance

- **Duration:** about 20 min
- **Tasks:** 2
- **Files modified:** 6 (3 created, 3 modified)

## Accomplishments

- `raiddetection.LoadRaidDetectionStats()` returns (configured, joinOff, floodOff) through `db.CountRows` over `models.ChatRaidDetection`.
- `devs.LoadAllStats` renders `<b>Raid detection:</b>` with `Custom settings`, `Join surge off` and `Flood off` lines and no longer imports the antiraid package.
- `TestRaidDetectionCarryOver` (package migrations) inserts chats and `antiraid_settings` rows for thresholds 0, 1, 20 and 900 plus a chat with an existing row (threshold 40 old, join_enabled false and 25 new), executes every statement of `20261018130000_add_chat_raid_detection.sql` twice, and asserts: no row for 0, join threshold 3 / 20 / 500 with both triggers on and flood threshold 30, the existing row untouched. One transaction, always rolled back, random fresh chat IDs.

## Task Commits

1. **Task 1 RED:** `8a357da` test(06-11): show the developer stats still count the retired raid mode
2. **Task 1 GREEN:** `d1c35d8` feat(06-11): count automatic raid detection settings in the developer stats
3. **Task 2:** `b1fd5ac` test(06-11): prove the raid threshold carry-over on PostgreSQL

## TDD Gate Compliance

- Task 1: RED commit precedes GREEN. The RED commit includes a compile scaffold (`stats.go` returning zeros) so the target tests fail on their assertions rather than on a build error: `TestLoadRaidDetectionStats` (grew by 0, want 3/1/1), `TestLoadAllStats` (missing the new labels) and `TestLoadAllStats_IncludesModuleCounts` (missing the Raid detection block). The GREEN commit replaces the scaffold with the real counting.
- Task 2: `tdd="true"`, but the test cannot fail or pass without PostgreSQL, so there is no separate RED/GREEN pair; it is a single test commit proving existing migration behaviour (plan 06-02), with no implementation to write. At this point it compiles and skips.

## Files Created/Modified

- `alita/db/raiddetection/stats.go` - LoadRaidDetectionStats
- `alita/db/raiddetection/stats_test.go` - TestLoadRaidDetectionStats
- `alita/db/devs/repository.go` - stats lines repointed, antiraid import dropped
- `alita/db/devs/repository_test.go` - new labels, seed rows and expected text
- `alita/db/devs/testmain_test.go` - `ChatRaidDetection` added to AutoMigrate (`AntiRaidSettings` kept, the model still exists)
- `alita/db/migrations/raid_carry_over_test.go` - TestRaidDetectionCarryOver

## Decisions Made

See frontmatter `key-decisions`. The test gates on `ALITA_TEST_DATABASE=true` plus `DATABASE_URL` (skip otherwise), not on the migration-chain flag `ALITA_TEST_MIGRATION_CHAIN`, as the plan specifies; if the migrated tables are absent it fails with a message naming the missing table.

## Deviations from Plan

None - plan executed exactly as written. (The compile scaffold in the RED commit is the allowed Go exception noted in the task brief.)

## Verification

- `CGO_ENABLED=1 go test -tags testtools -race -count=1 ./alita/db/raiddetection ./alita/db/devs ./alita/db/migrations` - all ok.
- `CGO_ENABLED=1 go test -tags testtools -count=1 -run '^TestRaidDetectionCarryOver$' -v ./alita/db/migrations` - `--- SKIP: TestRaidDetectionCarryOver`.
- `CGO_ENABLED=0 go build ./...` ok; `go vet -tags testtools ./alita/db/...` clean; `gofmt -l alita/db` prints nothing.
- Acceptance greps pass (`raiddetection.LoadRaidDetectionStats()` in devs/repository.go; `models.ChatRaidDetection{}` count 1 in devs/testmain_test.go; migration filename and `Rollback` present in the carry-over test).
- `go.mod` and `go.sum` unchanged against the base commit.
- `make lint` could not run here (golangci-lint cannot lint Go 1.26 in this environment); `go vet` and `gofmt -l` stood in. The full `make test` was left to the orchestrator.
- No PostgreSQL server was available and none was started. **TestRaidDetectionCarryOver has not been run against PostgreSQL**; it runs in CI once plan 06-13 adds it to the `test-postgres-integrity` list.

## Known Stubs

None.

## Threat Flags

None. The carry-over test never commits (T-06-34); stats expose counts only behind the developer-only /stats (T-06-35).

## Next Phase Readiness

Plan 06-13 must add `TestRaidDetectionCarryOver` to the `-run` list of `make test-postgres-integrity` (and keep the migration-chain step before it) so CI actually exercises it.

## Self-Check: PASSED

- Created files exist: stats.go, stats_test.go, raid_carry_over_test.go, this SUMMARY.
- Commits 8a357da, d1c35d8, b1fd5ac found in `git log`.
