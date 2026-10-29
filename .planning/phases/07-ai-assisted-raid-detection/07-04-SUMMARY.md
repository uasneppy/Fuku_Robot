---
phase: 07-ai-assisted-raid-detection
plan: 04
subsystem: raid-detection
tags: [postgres, gorm, migration, cache, redis, aispam, go]

requires:
  - phase: 07-ai-assisted-raid-detection
    provides: "raidAIRecordVerdict, raidAICounts and the AI verdict counter (plan 07-01)"
  - phase: 06-smarter-raid-detection
    provides: "chat_raid_detection, raiddetection.GetSettings, raidSettingsLookup, column-targeted upserts"
provides:
  - "chat_raid_detection.ai_enabled (default true) and ai_threshold (default 5, chk_chat_raid_detection_ai CHECK 3..500)"
  - "raiddetection.DefaultAIThreshold, AI fields in Defaults, SetAIEnabled and SetAIThreshold"
  - "GetSettings normalisation: a value cached by the previous build (ai_threshold 0) reads as AI on at 5"
  - "raidAIRecordVerdict follows the chat's own switch and threshold on every verdict"
affects: [07-07, 07-14]

actuals:
  tokens: 13000
  tasks: 2
  commits: 4
plan_head_before: 64b384ed107868efc0042c2a1f8ae75120812bb6
plan_head_after: d98a8d0faca5d4ef41a20ef1e66816939270ee2d

tech-stack:
  added: []
  patterns:
    - "Stale-cache guard: normalise the cache-or-load result, not only the loader, when a model gains columns whose zero value is wrong"
    - "Column-targeted upsert per trigger and kind, now six of them"

key-files:
  created:
    - migrations/20261029120000_add_raid_detection_ai.sql
  modified:
    - alita/db/models/raid_detection.go
    - alita/db/raiddetection/repository.go
    - alita/db/raiddetection/repository_test.go
    - alita/db/raiddetection/stats_test.go
    - alita/db/devs/repository_test.go
    - alita/modules/raid_join_surge_test.go
    - alita/modules/raid_ai_count.go
    - alita/modules/raid_ai_burst_test.go
    - AGENTS.md

key-decisions:
  - "The CHECK is inline on the ADD COLUMN so the statement stays idempotent (PostgreSQL has no ADD CONSTRAINT IF NOT EXISTS)"
  - "GetSettings treats ai_threshold 0 as 'not populated' (stored rows are never below 3) and returns the AI defaults on the freshly decoded value"
  - "raidAIRecordVerdict reads settings after the Redis check and before the script: an off switch or unreadable setting writes nothing to the :av/:aa sets"
  - "raidAIDefaultThreshold removed; the stored default of 5 plays its role"

requirements-completed: [RAID-03]

coverage:
  - id: D1
    description: "chat_raid_detection stores each group's AI switch and threshold behind a CHECK 3..500, on at 5 by default, written by column-targeted upserts that invalidate the cache"
    requirement: RAID-03
    verification:
      - kind: integration
        ref: "alita/db/raiddetection/repository_test.go#TestRaidDetectionSettings"
        status: pass
    human_judgment: false
  - id: D2
    description: "A settings value cached by the previous build (no AI fields) reads as AI on at 5, never as off or 0"
    requirement: RAID-03
    verification:
      - kind: integration
        ref: "alita/db/raiddetection/repository_test.go#TestRaidDetectionStaleCacheDefaults"
        status: pass
    human_judgment: false
  - id: D3
    description: "The AI burst follows each group's own setting: stored threshold 3 locks on 3 verdicts from 3 accounts, an off switch and an unreadable setting count nothing, the default stays 5"
    requirement: RAID-03
    verification:
      - kind: integration
        ref: "alita/modules/raid_ai_burst_test.go#TestRaidAIBurstSettings"
        status: pass
    human_judgment: false
  - id: D4
    description: "Migration 20261029120000 applies on PostgreSQL and the CHECK refuses ai_threshold below 3 there"
    requirement: RAID-03
    verification: []
    human_judgment: true
    rationale: "PostgreSQL cannot be started in this environment; the migration chain and make test-postgres-integrity (TestRaidDetectionSettings) run in CI"

duration: 40min
completed: 2026-10-29
status: complete
---

# Phase 07 Plan 04: Per-group AI burst settings Summary

**Each group gets its own AI trigger switch and threshold (on at 5, CHECK 3..500) stored in chat_raid_detection, read through a stale-cache-safe GetSettings, and the AI verdict counter now follows them on every verdict.**

## Performance

- **Duration:** about 40 min
- **Tasks:** 2 of 2
- **Files:** 1 created, 9 modified

## Accomplishments

- Migration `20261029120000_add_raid_detection_ai.sql` adds `ai_enabled BOOLEAN NOT NULL DEFAULT TRUE` and `ai_threshold INTEGER NOT NULL DEFAULT 5` with the named inline CHECK `chk_chat_raid_detection_ai`. It is the only file added to `migrations/`; `alita/db/migrations/runner.go` and `scripts/migrate_psql.sh` are unchanged.
- `raiddetection` gains `DefaultAIThreshold`, AI fields in `Defaults`, and `SetAIEnabled` / `SetAIThreshold` (column-targeted upserts through `upsertSettings`, so `cache.DeleteCache` follows every write; a threshold outside 3..500 returns `ErrThresholdOutOfRange` before any write; a threshold also turns the trigger on).
- `GetSettings` now wraps the cache-or-load result and turns `AIThreshold == 0` into `(true, 5)`, so a deploy over the previous build's cache never reads the AI trigger as off (research Pitfall 1, T-07-14).
- `raidAIRecordVerdict` reads the chat's settings through `raidSettingsLookup` under `raidSettingsTimeout` and judges `raidAICounts` with the chat's own `AIThreshold`; an off switch or an unreadable read counts nothing and writes nothing to Redis. `raidAIDefaultThreshold` is gone.
- AGENTS.md updated in the same commits as the code: the `chat_raid_detection` bullet (migration, columns, stale-cache rule, six writers), the Testing fixture rule (`AIThreshold` must be set, the CHECK applies on SQLite), `TestRaidDetectionSettings` covering the AI CHECK, and the "AI never decides alone" bullet.

## Task Commits

1. RED tests for the storage: `9160ee9` (test)
2. Task 1 GREEN, migration/model/repository/fixtures/AGENTS.md: `03704cd` (feat)
3. RED tests for the counter: `709558d` (test)
4. Task 2 GREEN, counter reads the chat's settings: `d98a8d0` (feat)

## Verification

- `go test -tags testtools -race -count=1 -run '^(TestRaidDetectionSettings|TestRaidDetectionStaleCacheDefaults|TestLoadRaidDetectionStats)$' -v ./alita/db/raiddetection`: PASS (including `--- PASS: TestRaidDetectionStaleCacheDefaults`).
- `go test -tags testtools -race -count=1 ./alita/db/devs ./alita/db/migrations`: ok.
- `go test -tags testtools -race -count=1 -run '^(TestRaidAIBurstSettings|TestRaidAIBurstFires|TestRaidAIAlert)$' -v ./alita/modules`: PASS.
- `go test -tags testtools -race -count=1 -run 'AISpam|Raid|Lockdown' ./alita/modules`: ok (130 s), also run after Task 1 with `Raid|AutoLockdown|Lockdown`: ok.
- `CGO_ENABLED=0 go build ./...`: ok. `go vet -tags testtools ./alita/db/... ./alita/modules`: clean. `gofmt -l alita/db alita/modules`: nothing.
- Acceptance criteria re-run for both tasks: migration is the last file by sort, 0 transaction statements, 2 `ADD COLUMN IF NOT EXISTS`, 1 `chk_chat_raid_detection_ai`, appliers unchanged, the migrations commit is a single `A` line, `func SetAIThreshold(` x1, `DefaultAIThreshold` x4, `20261029120000` in AGENTS.md, `raidSettingsLookup` and `AIThreshold` present and `raidAIDefaultThreshold` x0 in `raid_ai_count.go`.
- `make lint` could not run (golangci-lint is built with Go 1.25, module is Go 1.26); `go vet -tags testtools` and `gofmt -l` stood in. The orchestrator runs the full `make test`.

## Deviations from Plan

None - plan executed exactly as written. Two small notes: the migration header was reworded so the literal `chk_chat_raid_detection_ai` appears once (the acceptance criterion counts it), and the plan-commit ledger file under the shared git dir could not be written from the worktree sandbox, so `plan_head_before` is the base SHA the orchestrator gave (64b384e), which equals HEAD at start.

## Known Stubs

None.

## Threat Flags

None. T-07-14, T-07-15 and T-07-16 are mitigated as planned (stale-cache normalisation, `ErrThresholdOutOfRange` plus the CHECK, column-targeted upserts), each with a test.

## Pending Human Verification

- PostgreSQL half not run here (PostgreSQL must not be started in this environment): the migration chain applying `20261029120000` and `make test-postgres-integrity`'s `TestRaidDetectionSettings` with its "a row written directly with ai_threshold 2 is refused by the database" subtest running against the real migration CHECK. On SQLite this subtest passes against the CHECK GORM builds from the model tag. `TestRaidDetectionCarryOver` (`./alita/db/migrations`) is also CI-only.
- Live check: with a real bot, a group's stored `ai_threshold` change takes effect on the next `/aispam` delete verdict (the command that edits it is plan 07-07).

## Self-Check: PASSED

- Created file present: migrations/20261029120000_add_raid_detection_ai.sql.
- Commits 9160ee9, 03704cd, 709558d, d98a8d0 present in `git log`.
