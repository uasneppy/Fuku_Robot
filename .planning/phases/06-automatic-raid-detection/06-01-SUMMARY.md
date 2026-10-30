---
phase: 06-automatic-raid-detection
plan: 01
subsystem: lockdown
tags: [lockdown, raid-detection, postgres-migration, i18n, gotgbot]

requires:
  - phase: 05-lockdown-alerts-and-response
    provides: lockdown rows, alert copies, worker that decides and posts the alert
provides:
  - startLockdownRow, the one lockdown start path shared by /lockdown and the automatic triggers
  - chat_lockdowns trigger_count, trigger_window, trigger_percent and idx_chat_lockdowns_chat_lift
  - LockdownTriggerJoinSurge and LockdownTriggerFlood trigger kinds
  - lockdownAlertTriggerLine, the alert's trigger line per kind, in every destination's language
affects: [06-02, 06-03, 06-04, 06-05, 06-06, phase-07-ai-trigger]

actuals:
  tokens: 9500
  tasks: 2
  commits: 4

plan_head_before: 90f6f9d6b5186c801e6c838620b8f67f2b1c75f1
plan_head_after: c66e15b

commits: 4

tech-stack:
  added: []
  patterns:
    - "One start function with a defined outcome per refusal; the command maps each outcome to its reply"
    - "Trigger numbers as integer columns rendered per language, never inside the free-text reason"

key-files:
  created:
    - migrations/20261018120000_add_lockdown_trigger_columns.sql
    - alita/modules/lockdown_start.go
    - alita/modules/lockdown_start_test.go
  modified:
    - alita/db/models/lockdown.go
    - alita/modules/lockdown.go
    - alita/modules/lockdown_alert.go
    - AGENTS.md
    - locales/en.yml
    - locales/es.yml
    - locales/fr.yml
    - locales/hi.yml
    - locales/id.yml
    - locales/pt.yml
    - locales/ru.yml

key-decisions:
  - "startLockdownRow does not repeat /lockdown's GetActiveFresh and settle pre-step; a trigger that meets an unconfirmed row gets AlreadyActive from the insert conflict, and /lockdown keeps its settle step"
  - "A lock whose answer was lost returns the kept unconfirmed row as Row, so a caller can name it"
  - "trigger_window is stored in seconds; all three numbers are 0 for a manual lockdown"

patterns-established:
  - "Behaviour-preserving extraction pinned by an outcome-per-subtest test plus the untouched existing suites"

requirements-completed: [LOCK-10, RAID-01]

duration: 40min
completed: 2026-10-18
status: complete
---

# Phase 6 Plan 01: Shared lockdown start path and trigger line Summary

**startLockdownRow is now the one path that starts a lockdown for /lockdown and for the coming join-surge and flood triggers, rows record their trigger kind and numbers, and the alert names a join surge or flood with its count, window and percent in each destination's language.**

## Performance

- **Duration:** about 40 min
- **Completed:** 2026-10-18T11:45:44Z
- **Tasks:** 2 (each as a test commit, then an implementation commit)
- **Files modified:** 14

## Accomplishments

- `/lockdown` starts through `startLockdownRow` (bot rights read live, raw permissions snapshot, insert behind the one-active-lockdown index, lock call, confirmation, worker wake). Every outcome maps to the reply it sent before; the existing `Lockdown|Unlockdown` suites in `alita/modules` and `alita/db/lockdown` pass unchanged under `-race`.
- Two simultaneous starts of one chat give exactly one `Started` and one `AlreadyActive`, one row and one `setChatPermissions` call (LOCK-10).
- Migration 20261018120000 adds `trigger_count`, `trigger_window`, `trigger_percent` (all `INTEGER NOT NULL DEFAULT 0`) and `idx_chat_lockdowns_chat_lift` for the post-lift quiet period; the model, the new `join_surge` and `flood` kinds and AGENTS.md carry them.
- `lockdownAlertTriggerLine` renders the join-surge line (count, window, time) and the flood line (count, window, percent, time) from the row's columns, falling back to the manual line for any other kind (RAID-01). Two locale keys in all 7 files.

## Task Commits

1. **Task 1: shared start path and trigger columns**
   - `e9dac9d` (test) TestLockdownStartRow, with compile scaffolding
   - `a5f7e91` (refactor) startLockdownRow, /lockdown refactor, AGENTS.md
2. **Task 2: the alert's trigger line**
   - `3b2a61e` (test) TestLockdownAlertTriggerLine
   - `c66e15b` (feat) lockdownAlertTriggerLine and the two locale keys in 7 files

**Plan metadata:** the docs commit adding this file.

## TDD Gate Compliance

Both tasks have a failing-test commit before the implementation commit.

- Task 1 RED (`e9dac9d`): 9 of 10 subtests failed on assertions against the not-yet-implemented function (Outcome 0 instead of the expected outcome, Row nil, two Started instead of one of each). The tenth, "manual row", passed by design: it pins today's `/lockdown` behaviour, which the refactor must preserve.
- Task 2 RED (`3b2a61e`): "join surge" and "flood" failed because the alert carried the manual trigger line; "manual unchanged" passed by design.

## Decisions Made

- `startLockdownRow` leaves the `GetActiveFresh` plus `settleUnconfirmedLockdown` pre-step in `/lockdown`, as the plan directs. A trigger that meets an unconfirmed row gets `AlreadyActive` from the insert conflict and the worker settles that row on its own cycle.
- For `LockUnknown` the result carries the kept row, which costs nothing and lets a trigger caller log or name it.
- Percent is passed to the translator as a string built with `strconv`; no locale string has `%` directly followed by a printf verb letter, so the translator's legacy printf pass never runs on it.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] RED commit carries compile scaffolding**
- **Found during:** Task 1 (RED)
- **Issue:** Go will not build a test package that names `startLockdownRow`, the outcome constants, `LockdownTriggerJoinSurge` or the new row fields, so a test-only commit could not reach a meaningful RED (a compile error is an invalid RED).
- **Fix:** The RED commit `e9dac9d` also holds the trigger constants and model columns, the migration (so the model never exists without its schema in any commit) and a `startLockdownRow` stub that returns a zero result. The GREEN commit replaces the stub with the real implementation.
- **Files modified:** `alita/db/models/lockdown.go`, `migrations/20261018120000_add_lockdown_trigger_columns.sql`, `alita/modules/lockdown_start.go`
- **Committed in:** `e9dac9d`

---

**Total deviations:** 1 auto-fixed (1 blocking)
**Impact on plan:** None on behaviour. Only the split of files between the RED and GREEN commits differs from the plan's wording.

## Issues Encountered

- `make lint` cannot run here (golangci-lint is built with go1.25 and the module needs go1.26). `gofmt -l alita/modules alita/db` printed nothing and `go vet -tags testtools ./alita/modules ./alita/db/models ./alita/db/lockdown ./alita/i18n` was clean in its place.
- No PostgreSQL server exists in this sandbox and none was started. The migration is idempotent plain DDL (`ADD COLUMN IF NOT EXISTS`, `CREATE INDEX IF NOT EXISTS`, no transaction statements, no concurrent build); `go test ./alita/db/migrations` passes, and the PostgreSQL proof is CI's migration chain. The concurrency test ran on SQLite under `-race`; the PostgreSQL guarantee behind it (`uk_chat_lockdowns_active`) is already in `make test-postgres-integrity` via `TestStartLockdownOneActivePerChat`.

## Verification Run

| Check | Result |
|-------|--------|
| `go test -tags testtools -race -run '^TestLockdownStartRow$'` | pass, 10 subtests |
| `go test -tags testtools -race -run '^TestLockdownAlertTriggerLine$'` | pass, 3 subtests |
| `go test -tags testtools -race -run 'Lockdown\|Unlockdown' ./alita/modules ./alita/db/lockdown` | pass (re-run after the last commit) |
| `go test -tags testtools -race -run 'LockdownAlert' ./alita/modules` | pass |
| `go test -tags testtools -run '^TestLockdownLocaleKeys$\|^TestLockdownStringsHaveBalancedTags$' ./alita/i18n` | pass |
| `go test -tags testtools ./alita/db/migrations` | pass |
| `CGO_ENABLED=0 go build ./...`, `go vet`, `gofmt -l` | clean |
| `make check-translations`, `make check-docs` | pass, no drift |
| `go.mod` and `go.sum` | unchanged |
| Task 1 acceptance criteria (latest migration name, no BEGIN/COMMIT/CONCURRENTLY, `startLockdownRow(` in lockdown.go, 10 `t.Run(` calls, AGENTS.md mention) | all met |
| Task 2 acceptance criteria (both keys once in each of 7 locales, `func lockdownAlertTriggerLine(` present, gofmt) | all met |

The full `make test` was not run; the orchestrator runs it after the merge.

## Known Stubs

None. The only stub (`startLockdownRow` returning a zero result) existed only in the RED commit and is replaced in `a5f7e91`.

## Threat Flags

None. No new endpoint, auth path or trust boundary. T-06-01 and T-06-02 are mitigated as planned (the sequence moved verbatim and pinned per outcome; one winner under the unique index). T-06-03: the trigger line holds only integers from the row and a time.

## User Setup Required

None - no external service configuration required. `AUTO_MIGRATE=true` in the deploy manifests applies the migration.

## Next Phase Readiness

The join-surge and flood triggers (later plans) can call `startLockdownRow` with `LockdownTriggerJoinSurge` or `LockdownTriggerFlood`, the bot as starter and their numbers, and read `idx_chat_lockdowns_chat_lift` for the quiet period. No blockers.

---
*Phase: 06-automatic-raid-detection*
*Completed: 2026-10-18*

## Self-Check: PASSED

Created files exist (`migrations/20261018120000_add_lockdown_trigger_columns.sql`, `alita/modules/lockdown_start.go`, `alita/modules/lockdown_start_test.go`); commits `e9dac9d`, `a5f7e91`, `3b2a61e` and `c66e15b` are in `git log`.
