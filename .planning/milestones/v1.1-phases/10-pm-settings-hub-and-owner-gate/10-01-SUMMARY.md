---
phase: 10-pm-settings-hub-and-owner-gate
plan: 01
subsystem: database
tags: [postgres, gorm, migrations, repository, advisory-lock, settings-hub]

requires:
  - phase: 09-captcha-web
    provides: migration chain up to 20261101120000 and the testdb harness
provides:
  - chat_settings_access table and menuaccess repository (owner switch, read fresh)
  - chat_admin_index table and adminindex repository (roster hint for the private list)
affects: [10-02, 10-03, 10-04, 10-05, 10-06, 10-07, 10-08, 10-09, settings-hub, owner-gate]

actuals:
  tokens: 21000
  tasks: 2
  commits: 3

commits: 3
plan_head_before: be442b4170ffce3c55ced105a14ae5a5fe3c40aa
plan_head_after: 00ab8453b26a753582fea192561bfc30b8cb6bd5

tech-stack:
  added: []
  patterns:
    - "Fresh-read authority state: no cache key, no skipLocal entry, no DeleteCache"
    - "Roster replace = per-chat pg_advisory_xact_lock, upsert, NOT IN prune in one transaction"

key-files:
  created:
    - migrations/20261104120000_add_settings_hub_tables.sql
    - fuku/db/models/chat_settings_access.go
    - fuku/db/models/chat_admin_index.go
    - fuku/db/menuaccess/repository.go
    - fuku/db/menuaccess/repository_test.go
    - fuku/db/menuaccess/testmain_test.go
    - fuku/db/adminindex/repository.go
    - fuku/db/adminindex/repository_test.go
    - fuku/db/adminindex/testmain_test.go
    - fuku/db/migrations/settings_hub_migration_test.go
  modified: []

key-decisions:
  - "The owner switch is read with a fresh query on every call and never cached, so a change is visible on every replica at once (D-12)."
  - "AdminsAllowed carries no GORM default tag and is written by a column-targeted upsert, so a Go false is always stored as false."
  - "ReplaceChat is upsert plus NOT IN prune under a per-chat advisory lock on PostgreSQL, never delete-then-insert."
  - "ListForUserFresh never filters on chats.is_inactive and uses three portable queries with no JOIN."
  - "The migration-order assertion pins only that the file sorts after 20261101120000, so later migrations do not break the test."

patterns-established:
  - "menuaccess.AdminsAllowedFresh returns an error for every failure but a missing row, so callers refuse on an unreadable switch."

requirements-completed: [MENU-10, MENU-12, MENU-14]

duration: 6min
completed: 2026-11-04
status: complete
---

# Phase 10 Plan 01: Settings hub stored state Summary

**Two append-only tables (owner switch `chat_settings_access`, roster hint `chat_admin_index`) with fresh-read repositories, a per-chat advisory-locked roster replace, and SQLite tests that run unchanged on PostgreSQL.**

## Performance

- **Duration:** 6 min
- **Started:** 2026-11-04T12:28:07Z
- **Completed:** 2026-11-04T12:34:14Z
- **Tasks:** 2 (a tracer and a TDD task)
- **Files modified:** 10 created, 0 modified

## Accomplishments

- Migration `20261104120000_add_settings_hub_tables.sql`: two `CREATE TABLE IF NOT EXISTS` with every constraint inline, one `CREATE INDEX IF NOT EXISTS`, no transaction control, no applier change.
- `menuaccess.AdminsAllowedFresh` / `SetAdminsAllowed`: missing row reads as owner only, a Go false and true are both stored, last writer wins, any read failure is an error.
- `adminindex.ReplaceChat` / `ClearChat` / `ListChatFresh` / `ListForUserFresh` / `ListCandidateChatIDsFresh`: exact-roster replace with creator-wins dedupe, `ErrUnknownRole`, the D-15 listing rule, and the sweep's candidate IDs.
- Both tables proven absent from backup (`TestAccessTablesStayOutOfBackup`).

## Task Commits

1. **Task 1 (tracer): owner switch from migration to fresh read** - `6a6345c` (feat)
2. **Task 2 (TDD): roster repository** - RED `b291a37` (test), GREEN `00ab845` (feat); no refactor needed

**Plan metadata:** committed with this SUMMARY (docs).

## Files Created/Modified

- `migrations/20261104120000_add_settings_hub_tables.sql` - both tables, constraints inline, index on `user_id`
- `fuku/db/models/chat_settings_access.go` - `ChatSettingsAccess`, boolean without a default tag
- `fuku/db/models/chat_admin_index.go` - `ChatAdminIndex`, role constants, unique key and CHECK tags mirroring the migration
- `fuku/db/menuaccess/repository.go` - `AdminsAllowedFresh`, `SetAdminsAllowed`
- `fuku/db/adminindex/repository.go` - roster repository with `pg_advisory_xact_lock`
- `fuku/db/menuaccess/*_test.go`, `fuku/db/adminindex/*_test.go`, `fuku/db/migrations/settings_hub_migration_test.go` - tests

## Decisions Made

See `key-decisions` above. All follow the plan; no new decision was needed.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Brittle test] Migration-order assertion**
- **Found during:** Task 1
- **Issue:** The plan asked `TestSettingsHubMigrationStatements` to assert the file sorts after every other migration. That fails the moment any later migration is added.
- **Fix:** The test asserts the file sorts after `20261101120000_default_language_ukrainian.sql` (the newest file when the plan was executed) and that this file exists. The new file is still the newest today (`ls migrations | sort | tail -1`).
- **Files modified:** `fuku/db/migrations/settings_hub_migration_test.go`
- **Commit:** `6a6345c`

**Total deviations:** 1 auto-fixed (1 test brittleness). **Impact:** none on behaviour or scope.

## Verification Results

- `go test -tags testtools -race -count=1 -run '^(TestMenuAccessUpsert|TestMenuAccessReadError)$' ./fuku/db/menuaccess` - PASS
- `go test -tags testtools -race -count=1 -run '^TestSettingsHubMigrationStatements$' ./fuku/db/migrations` - PASS
- `go test -tags testtools -race -count=1 -v ./fuku/db/adminindex` - PASS for `TestAdminIndexReplaceChat`, `TestAdminIndexListForUser`, `TestAdminIndexCandidates`, `TestAdminIndexReplaceConcurrent`, `TestAccessTablesStayOutOfBackup`
- `go test -tags testtools -race -count=3 -run '^TestAdminIndexReplaceConcurrent$' ./fuku/db/adminindex` - PASS (no SQLite busy error)
- `go test -tags testtools -race -count=1 ./fuku/db/menuaccess ./fuku/db/adminindex ./fuku/db/migrations` - PASS
- `CGO_ENABLED=0 go build ./...` - OK; `go vet -tags testtools` on the new packages - OK
- Acceptance greps: unique constraint 1, CHECK 1, `default:` on `AdminsAllowed` 0, cache imports in menuaccess 0, `pg_advisory_xact_lock` outside comments 1, `is_inactive` outside comments 0, `NOT IN` 1
- `git diff --exit-code 6ef6ad9 -- fuku/db/migrations/runner.go scripts/migrate_psql.sh go.mod go.sum` - clean
- `golangci-lint` could not run: the installed binary was built with Go 1.25 and refuses the repo's Go 1.26.0 config. Not a code finding; CI lint should cover it.
- PostgreSQL was not available; the advisory-lock branch (`tx.Name() == "postgres"`) is not exercised here. Plan 10-04 adds the concurrency test to `make test-postgres-integrity`.

## Known Stubs

None.

## Threat Flags

None. The new tables are the surface the plan's threat model already covers (T-10-01 to T-10-05).

## Next Phase Readiness

Plan 10-02 onward can read `menuaccess.AdminsAllowedFresh` and use `adminindex` for the feed, sweep and hub list. Plan 10-04 still owns the settings test harness, the integrity-list entry and the AGENTS.md rules for these tables.

## Self-Check: PASSED

Files verified on disk, commits `6a6345c`, `b291a37`, `00ab845` verified as ancestors of HEAD, measured count of plan commits (`be442b4..HEAD` at the time of the last task commit) is 3.

