---
phase: 05-lockdown-alerts-and-response
plan: 01
subsystem: lockdown
tags: [lockdown, alerts, staff-group, worker, migration, i18n]
status: complete
requires:
  - phase: 04-manual-lockdown
    provides: lockdown worker, lockdown pacer, chat_lockdowns, Staff Group links
provides:
  - chat_lockdown_alerts (one row per posted copy) and chat_lockdowns.chat_title / alert_started_at
  - lockdown.BeginAlert exactly-once gate and the copy claim/record repository
  - worker steps lockdownPlanAlerts and lockdownPostAlerts
  - first lockdown alert in the Staff Group (LOCK-11, D-01)
affects: [05-02, 05-03, 05-06, 05-07, 05-13]
tech-stack:
  added: []
  patterns:
    - "alert decided once by a conditional update plus the copy rows in one transaction, run only by the worker"
    - "posted copy remembers message_id and the exact body so later edits append to what was sent"
key-files:
  created:
    - migrations/20261011120000_add_lockdown_alerts.sql
    - alita/db/lockdown/alerts.go
    - alita/modules/lockdown_alert.go
    - alita/modules/lockdown_alert_test.go
  modified:
    - alita/db/models/lockdown.go
    - alita/modules/lockdown.go
    - alita/modules/lockdown_worker.go
    - alita/modules/test_harness_test.go
    - AGENTS.md
    - locales/en.yml
    - locales/es.yml
    - locales/fr.yml
    - locales/hi.yml
    - locales/id.yml
    - locales/pt.yml
    - locales/ru.yml
key-decisions:
  - "The whole chat_lockdown_alerts row shape (buttons_live, edit_pending, closed_at, link, attempts, claim, detail) is created in this one migration so plans 05-02..05-13 add no ALTER on it."
  - "A copy stuck in 'posting' after a hard crash between the Telegram send and RecordAlertPosted is not auto-released: releasing it could post a second alert, which the plan's prohibition forbids."
requirements-completed: [LOCK-11]
duration: 1h
completed: 2026-10-11
commits: 3
plan_head_before: eb76dc05348f9e31450515f7568766f4758be5d9
plan_head_after: f017410bd277e2a087ec8b4ed9e4f8a6274f62f8
actuals:
  tokens: 11300
  tasks: 2
  commits: 3
---

# Phase 5 Plan 01: Staff Group lockdown alert Summary

**A manual `/lockdown` in a group linked to a Staff Group now posts exactly one alert there (group, trigger, who, when, reason), decided by a conditional `BeginAlert` update and posted by the lockdown worker through the lockdown pacer.**

Note: `commits: 3` counts the code and test commits made before this SUMMARY; the SUMMARY commit follows it.

## What was built

- **Migration `20261011120000_add_lockdown_alerts.sql`**: `chat_lockdowns.chat_title`, `chat_lockdowns.alert_started_at`, and `chat_lockdown_alerts` with the unique slot `uk_chat_lockdown_alerts_slot (lockdown_id, kind, seq, dest)`, three CHECK constraints, two indexes, and a SQL-only cascade foreign key. No top-level transaction statements, every statement idempotent.
- **Models**: `ChatLockdownAlert`, `AlertKind*`, `AlertDest*`, `AlertState*`, plus `ChatLockdown.ChatTitle` and `AlertStartedAt`. The model is in the `alita/modules` test harness AutoMigrate list.
- **Repository `alita/db/lockdown/alerts.go`**: `AlertPlan`, `BeginAlert`, `ListAlertsToPlanFresh`, `ListPendingAlertSlotsFresh` (pending copies of active lockdowns only), `GetAlertSlotFresh`, `ClaimAlertSlot`, `RecordAlertPosted`, `ReleaseAlertSlot`, `FailAlertSlot`. All fresh reads, conditional updates with `RowsAffected == 1`.
- **Module `alita/modules/lockdown_alert.go`**: `lockdownAlertDestinations` (uncached `staff.GetLinkOfGroupFresh`), `lockdownPlanAlerts`, `lockdownPostAlerts`, `lockdownRenderAlert` (group, name and reason spliced in after translation through tokens), `lockdownGroupLabel` (bold escaped title, or the chat ID in code tags when empty).
- **Wiring**: `/lockdown` stores the title (capped at 128 runes) and calls `wakeLockdownWorker()` after `ConfirmLocked`; its replies are unchanged. `runLockdownCycle` runs plan then post after the settle step.
- **Locale keys** `lockdown_alert_title` and `lockdown_alert_trigger_manual` in all 7 locales.
- **AGENTS.md**: Data bullet for the new table/columns and the Testing note.

## Tests

- `TestLockdownAlertPostsOnce`: one message to the Staff Group after two cycles, marker keys, escaped name and reason, one `posted` row with the sent message ID and exact body, `alert_started_at` and `chat_title` set, nothing extra in the locked group.
- `TestLockdownAlertGate`: no destination (stamped, no row, no message elsewhere), linked later (never alerted), lifted before posting (never sent), older lockdown with an empty title (alerted once, chat ID in code tags), failed send (pending with 1 attempt, then posted).

## Task commits

| Task | Commit | Message |
| ---- | ------ | ------- |
| 1 (RED) | facb0ae | test(05-01): show a lockdown never reaches its Staff Group |
| 1 (GREEN) | 36c9056 | feat(05-01): alert the Staff Group when a linked group is locked down |
| 2 | f017410 | test(05-01): show a lockdown alert reaches groups linked later and lifted lockdowns |

## Deviations from Plan

**1. [Task 2 - no fix commit] The gate tests passed on their first run.** Task 1's implementation already contained the behaviour Task 2 pins (stamp with no destination, only pending copies of active lockdowns, retry bookkeeping), so there was nothing to fix and the planned `fix(05-01)` commit was not made. The lifted-before-posting case is double-guarded: both `ListPendingAlertSlotsFresh` and `lockdownPostAlertSlot` (which releases a copy whose lockdown is no longer active) refuse it; a mutation of the list filter alone does not fail the test because the second guard still holds.

**2. [Task 1 RED] The RED commit fails to compile** (`models.ChatLockdownAlert` undefined), which is the red state for a feature whose model does not exist yet.

Otherwise the plan was executed as written.

## Known limitations

- A copy left in `posting` by a hard crash between the Telegram send and `RecordAlertPosted` is not released automatically, to honour "never more than one alert per lockdown and destination". Plan 05-13 (reminders and the shared copy rows) is the place to decide a recovery rule.
- Destination rows are created at decision time only; the log-channel row arrives with plan 05-06.

## Verification

- `go test -tags testtools -race -count=1 -run '^TestLockdownAlertGate$|^TestLockdownAlertPostsOnce$' ./alita/modules`: pass.
- `go test -tags testtools -race -count=1 -run 'Lockdown|Unlockdown|StaffPanel|StaffBan' ./alita/modules`: pass (Task 1 run), and `Lockdown|Unlockdown` again after Task 2: pass.
- `go test -tags testtools -race ./alita/db/lockdown ./alita/db/models ./alita/db/migrations ./alita/db`: pass.
- `CGO_ENABLED=0 go build ./...`, `go vet -tags testtools ./alita/modules ./alita/db/lockdown ./alita/db/models`: clean.
- `TestLockdownLocaleKeys` and `make check-translations`: pass.
- `gofmt -l` on all touched Go files prints nothing. golangci-lint was not run: the installed one is built with Go 1.25 and cannot lint Go 1.26 code, so `go vet` and `gofmt` were used instead. (`alita/modules/greetings_command_test.go` is reported by `gofmt -l` but is pre-existing and untouched.)
- `go.mod` and `go.sum` unchanged.
- Not run: the PostgreSQL human-check for applying the migration (no PostgreSQL server in the sandbox). The migration is plain idempotent SQL; the SQLite tests exercise the GORM model, not the SQL file.

## Threat surface

No new surface beyond the plan's threat model. T-05-01 (escaped text), T-05-02 (conditional gate and claims) and T-05-03 (destination from the uncached link read at decision time only) are covered by the tests above.

## Known Stubs

None.

## Self-Check: PASSED

All created files exist (migration, `alerts.go`, `lockdown_alert.go`, `lockdown_alert_test.go`), and commits facb0ae, 36c9056 and f017410 are on the branch.
