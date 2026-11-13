---
phase: 10-pm-settings-hub-and-owner-gate
plan: 08
subsystem: settings
tags: [sweeper, admin-index, roster, redis-lock, shutdown, telegram]
requires:
  - phase: 10-pm-settings-hub-and-owner-gate
    provides: adminindex.ListCandidateChatIDsFresh and the roster repository (10-01), the owner gate and isTelegramClientError (10-05), the settings test fixtures (10-04), refreshChatAdminIndex and the feed (10-07), the deep link gate (10-11)
provides:
  - StartAdminIndexSweeper and StopAdminIndexSweeper, the startup backfill and hourly sweep of chat_admin_index
  - The operational Redis key fuku:settings:sweep:lock (new family fuku:settings:*)
  - settingsRepairAdminIndex, the group /settings repair path (second after /admincache)
  - AGENTS.md rules for the roster feed, the sweep, the lock key, the shutdown handler and the repair path
affects: [10-09, 10-10]

status: complete
actuals:
  tokens: 21000
  tasks: 3
  commits: 3

commits: 3
plan_head_before: aaf0311b5f3ea47e19f45892298afbd1bd22d75e
plan_head_after: ab5da1accd9c8c79eec415dad34e0a4d6a5e1616

tech-stack:
  added: []
  patterns:
    - "A BotClient wrapper (adminIndexPacedClient) paces and bounds every Telegram request of a background cycle from the cycle's own context, so a cancel returns at once and no call escapes the pace"
    - "A SETNX cycle lock released by compare-and-delete only when a cancel cut the cycle off, kept to its TTL otherwise"

key-files:
  created:
    - fuku/modules/admin_index_sweeper.go
    - fuku/modules/admin_index_sweeper_test.go
  modified:
    - main.go
    - fuku/modules/settings.go
    - fuku/modules/admin_index_feed_test.go
    - fuku/modules/settings_gate_test.go
    - AGENTS.md

key-decisions:
  - "Pacing and the 5 s bound live in a BotClient wrapper rather than around refreshChatAdminIndex, so each of a group's two calls is paced and bounded as the must-have says (the feed function stays unchanged and sleep-free)"
  - "The cycle hook runs at the start of runAdminIndexSweep, before the lock, so a test counts cycles even when another runner holds the lock"
  - "A cycle ended by a second 429 keeps its lock (rate limiting is not a cancel); only a cancel releases it"
  - "The repair refresh is synchronous after the reply with a 10 s context and only a Debug log, as the plan says, with no new goroutine"

requirements-completed: []

coverage:
  - id: D1
    description: "The first cycle fills chat_admin_index for every group where the bot is an administrator, clears a group where it is not, and the loop does it by itself shortly after start"
    requirement: MENU-10
    verification:
      - kind: unit
        ref: "fuku/modules/admin_index_sweeper_test.go#TestAdminIndexSweepBackfill"
        status: pass
      - kind: unit
        ref: "fuku/modules/admin_index_sweeper_test.go#TestAdminIndexSweepLifecycle"
        status: pass
    human_judgment: false
  - id: D2
    description: "One runner per cycle across replicas; a held lock makes no Telegram call; without Redis or on a Redis error the cycle runs unguarded"
    requirement: MENU-10
    verification:
      - kind: unit
        ref: "fuku/modules/admin_index_sweeper_test.go#TestAdminIndexSweepLock"
        status: pass
      - kind: unit
        ref: "fuku/modules/admin_index_sweeper_test.go#TestAdminIndexSweepWithoutRedis"
        status: pass
    human_judgment: false
  - id: D3
    description: "An error at the bot lookup or the list leaves a group's rows untouched and the cycle goes on; a 429 waits min(retry_after, cap) and retries once; a second 429 ends the cycle with later groups untouched"
    requirement: MENU-10
    verification:
      - kind: unit
        ref: "fuku/modules/admin_index_sweeper_test.go#TestAdminIndexSweepKeepsRowsOnError"
        status: pass
      - kind: unit
        ref: "fuku/modules/admin_index_sweeper_test.go#TestAdminIndexSweepRateLimit"
        status: pass
    human_judgment: false
  - id: D4
    description: "A cancel returns within 500 ms with no further call and releases the lock; a panicking cycle is recovered; Start twice runs one loop; Stop is idempotent"
    requirement: MENU-10
    verification:
      - kind: unit
        ref: "fuku/modules/admin_index_sweeper_test.go#TestAdminIndexSweepStops"
        status: pass
      - kind: unit
        ref: "fuku/modules/admin_index_sweeper_test.go#TestAdminIndexSweepPanicSurvives"
        status: pass
    human_judgment: false
  - id: D5
    description: "The sweep's stop is registered right after the staff sweeper's stop and before the staff actions drain, so LIFO runs it after that drain and before the database closes"
    requirement: MENU-10
    verification:
      - kind: command
        ref: "grep -n 'StopStaffSweeper|StopAdminIndexSweeper|RegisterStaffActionsDrain' main.go (lines 206, 216, 223)"
        status: pass
    human_judgment: true
    rationale: "The order is a source-order fact checked by grep; the 60 s LIFO budget itself is not exercised by a test"
  - id: D6
    description: "A group /settings by an owner or allowed administrator refreshes that group's record after the choice message; a failed refresh changes no row and not the reply; a gated administrator or plain member triggers none"
    requirement: MENU-10
    verification:
      - kind: unit
        ref: "fuku/modules/admin_index_feed_test.go#TestAdminIndexSettingsCommandRefresh"
        status: pass
    human_judgment: false
  - id: D7
    description: "The private list that reads these rows and its empty-list hint are not built yet; MENU-10 is delivered only with plan 10-09"
    requirement: MENU-10
    verification: []
    human_judgment: true
    rationale: "This plan delivers the record's freshness (backfill, sweep, both repair paths); the list and the hint belong to plan 10-09."
---

# Phase 10 Plan 08: Admin roster sweep and repair path Summary

**An hourly, lock-guarded, paced sweep (first cycle 30 to 90 s after start) keeps `chat_admin_index` filled for every group the bot administers, and a group `/settings` repairs that group's record after its choice message.**

## Accomplishments

- **`fuku/modules/admin_index_sweeper.go`**: `StartAdminIndexSweeper` / `StopAdminIndexSweeper` (mutex, cancel, WaitGroup, goroutine with `RecoverFromPanic`, a second recover per cycle), `runAdminIndexSweep` (candidates from `adminindex.ListCandidateChatIDsFresh`, each group through `refreshChatAdminIndex`), `acquireAdminIndexSweepLock` (SETNX `fuku:settings:sweep:lock`, per-process UUID owner, TTL interval minus one minute, unguarded without Redis or on a Redis error), `releaseAdminIndexSweepLock` (Lua compare-and-delete, only after a cancel), the 429 rule (wait `min(retry_after, 30 s)`, retry that group once, a second 429 ends the cycle with `RateLimited`), and `adminIndexPacedClient` (every request waits the 250 ms pace and runs under 5 s, both from the sweep context).
- **`main.go`**: `modules.StartAdminIndexSweeper(b)` right after `StartStaffSweeper(b)` in `postInit`; a plain `RegisterHandler` stop right after the staff sweeper's stop, so LIFO runs it after `StopStaffActions` and before the database close.
- **`fuku/modules/settings.go`**: `settingsRepairAdminIndex` runs `refreshChatAdminIndex` under a 10 s context after the choice message and only logs an error.
- **AGENTS.md**: a Startup and shutdown bullet for `StopAdminIndexSweeper`, the `fuku:settings:sweep:lock` key under Data, "The settings roster" bullet under Subsystem traps (feed and sweep), and the second-repair-path sentence in the Settings module paragraph.
- **Tests**: `TestAdminIndexSweepBackfill`, `TestAdminIndexSweepLifecycle`, `TestAdminIndexSweepLock`, `TestAdminIndexSweepWithoutRedis`, `TestAdminIndexSweepKeepsRowsOnError`, `TestAdminIndexSweepRateLimit`, `TestAdminIndexSweepStops`, `TestAdminIndexSweepPanicSurvives` (sweeper file) and `TestAdminIndexSettingsCommandRefresh` (feed test file, four cases).

## Task Commits

1. Task 1 (tracer): the backfill and hourly sweep, wired into `main.go` - `f4cf233`
2. Task 2 (TDD): the rulebook for the feed, the sweep, the lock key and the shutdown handler - `7379c38`
3. Task 3 (TDD): `/settings` in a group repairs that group's record - `ab5da1a`

Tracer gate: the tracer `<verify>` is automated only (`end-of-phase` mode), so it was re-run end to end after the commit and passed before the later tasks.

TDD note: the sweeper's code and all eight sweep tests were written together in Task 1 (the 429, release and lock behaviour is in the one file), so no separate RED state was committed for Task 2. I checked the release test bites by removing the release call: `TestAdminIndexSweepStops` then failed with "the lock is still held after a cancelled cycle" and passed again after restoring it. Task 3's RED was observed (`TestAdminIndexSettingsCommandRefresh` failed on the owner case before `settings()` called the refresh) and then went green.

## Verification Results

- `go test -tags testtools -race -count=1 -run '^(TestAdminIndexSweepBackfill|TestAdminIndexSweepLifecycle)$' ./fuku/modules` - PASS
- `CGO_ENABLED=0 go build ./...` - passes
- `go test -tags testtools -race -count=1 -run '^TestAdminIndexSweep' -v ./fuku/modules` - `--- PASS` for all eight sweep tests (6.9 s)
- `go test -tags testtools -race -count=1 -run '^TestStaffSweep' ./fuku/modules` - `ok 4.9 s`
- `go test -tags testtools -race -count=1 -run '^TestAdminIndexSettingsCommandRefresh$' -v ./fuku/modules` - `--- PASS` with all four cases
- Plan-level: `go test -tags testtools -race -count=1 -run '^(TestAdminIndex|TestStaffSweep|TestSettings|TestLoadSettings)' ./fuku/modules ./fuku/i18n` - `ok fuku/modules 118.5 s`, `ok fuku/i18n 5.6 s`
- `git diff --exit-code 6ef6ad9 -- go.mod go.sum` - clean
- Acceptance greps: `modules.StartAdminIndexSweeper(b)` in main.go 1; `modules.StopAdminIndexSweeper()` 1; stop line (216) after `StopStaffSweeper` (206) and before `RegisterStaffActionsDrain` (223); `RecoverFromPanic` in the sweeper 2; `fuku:settings:sweep:lock` in AGENTS.md 1; `StopAdminIndexSweeper` in AGENTS.md 2; `SetAdminListObserver` in AGENTS.md 1; `second repair path` in AGENTS.md 1; `refreshChatAdminIndex` in settings.go 1.
- `go vet -tags testtools ./fuku/modules` is clean. `make lint` and the full `make test` were not run (the installed golangci-lint refuses the repository's Go 1.26.0 target, as in earlier plans of this phase). `gofmt -l` flags `main.go`, but only for a spacing line in the `SetMyCommands` call that predates this plan and is out of scope.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Three gate tests counted the repair's list call as the gate's**
- **Found during:** Task 3
- **Issue:** `TestSettingsInGroupRefusal` (owner, and allowed administrator) and `TestSettingsGatePress` (allowed administrator) assert that the gate makes no `getChatAdministrators` call, counting all of them. The plan's repair refresh adds one after a successful `/settings`, so they failed.
- **Fix:** each of the three makes the bot a plain member of the chat, so the refresh stops after its bot lookup and any list call left is the gate's own. The assertions and what they prove are unchanged.
- **Files modified:** `fuku/modules/settings_gate_test.go`
- **Commit:** `ab5da1a`

### Implementation choices inside the plan's freedom

- Pacing and the 5 s bound are in `adminIndexPacedClient`, a `BotClient` wrapper over a copy of the bot, instead of one pace plus one bound around `refreshChatAdminIndex`. That is what makes each of a group's two calls paced and bounded (the must-have), and it leaves the feed function untouched. `adminIndexSweepPaceFunc` is still the pace function.
- The cycle hook runs at the top of `runAdminIndexSweep` (the plan's wording), not in the cycle wrapper as the staff sweeper does.
- The tests clear every negative `chats` row first (`adminIndexSweepIsolate`) so the report counts are exact on the shared SQLite database.

**Total deviations:** 1 auto-fixed (1 bug in existing tests). **Impact:** none on behaviour.

## Requirements

`MENU-10` is **left Pending** in `.planning/REQUIREMENTS.md` (checkbox and traceability row), and `requirements.mark-complete` was not run. This plan completes the record's freshness (backfill, hourly sweep, both repair paths), but the private list that reads it and the empty-list hint wording belong to plan 10-09.

## Known Stubs

None.

## Threat Flags

None. The new surface is the `fuku:settings:sweep:lock` key and the sweep's Telegram calls, which are T-10-23, T-10-24 and T-10-25 in the plan's threat model; the group `/settings` repair is T-10-38.

## Self-Check: PASSED

- Created files present: `fuku/modules/admin_index_sweeper.go`, `fuku/modules/admin_index_sweeper_test.go`.
- Commits `f4cf233`, `7379c38` and `ab5da1a` are ancestors of HEAD; the measured count from `aaf0311` is 3.
- Task acceptance criteria and the plan-level verification commands re-run and passing (except the lint and full `make test` runs noted above).
