---
phase: 16-scheduled-deletion-of-service-and-bot-messages
plan: 01
subsystem: database
tags: [postgres, sqlite, gorm, migrations, cache, queue, claim, scheduled-deletion]

requires:
  - phase: 15-ai-spam-name-check
    provides: migration chain up to 20261109120000, the PostgreSQL integrity target, the skipLocal and backup-exclusion patterns
provides:
  - migration 20261110120000 with chat_delete_rules and scheduled_deletions (named constraints, partial indexes)
  - models.DeleteRule and models.ScheduledDeletion
  - fuku/db/deletion (kind registry, cached rules, ApplyJoinLeftRule and ApplyCleanServiceFlag, the queue)
  - greetings.SetCleanServiceTx
  - the delete_rules key in skipLocal
  - three PostgreSQL guarantees in make test-postgres-integrity
affects: [16-02, 16-03, 16-04, 16-05, 16-06, 16-07, 16-08, 16-09]

plan_head_before: dc324b3 (HEAD when the plan started; the per-plan ledger file of protocol 0c was not created, see Deviations)
plan_head_after: e9bf55bf88a73ea514bf1a1667c201b699a9207f

actuals:
  tokens: 22400   # chars/4 over the added lines of dc324b3..HEAD (89453 chars), tests and docs included
  tasks: 3
  commits: 5      # MEASURED: git rev-list --count dc324b3..HEAD (before this SUMMARY commit)

tech-stack:
  added: []
  patterns:
    - "queue claim by one conditional UPDATE read back by token, token-guarded completion"
    - "earliest-wins upsert with a per-dialect table-qualified LEAST or MIN"
    - "transaction hook variable with a testtools setter to fail a second step without DDL"

key-files:
  created:
    - migrations/20261110120000_add_scheduled_deletion.sql
    - fuku/db/models/delete_rule.go
    - fuku/db/models/scheduled_deletion.go
    - fuku/db/deletion/kinds.go
    - fuku/db/deletion/rules.go
    - fuku/db/deletion/queue.go
    - fuku/db/deletion/hook_testtools.go
    - fuku/db/deletion/testmain_test.go
    - fuku/db/deletion/rules_test.go
    - fuku/db/deletion/queue_test.go
    - fuku/db/migrations/scheduled_deletion_migration_test.go
    - fuku/db/backup/deletion_backup_test.go
  modified:
    - fuku/db/greetings/repository.go
    - fuku/db/greetings/repository_test.go
    - fuku/db/cache/local.go
    - fuku/db/cache/local_test.go
    - fuku/db/backup/testmain_test.go
    - fuku/modules/test_harness_test.go
    - Makefile
    - AGENTS.md

key-decisions:
  - "Complete, Release return error only; ReleaseStale and DropChatPending return (int64 rows, error); ClaimDue returns []models.ScheduledDeletion"
  - "Every token-taking function refuses an empty token (ErrInvalidToken): a pending row carries an empty token, so an empty one would match every pending row"
  - "Release runs delete-then-update in one transaction, both guarded by token and state acting"
  - "ApplyJoinLeftRule takes a typed JoinLeftState{Mode: JoinLeftKeep|JoinLeftNow|JoinLeftAfter, DelaySecs}; kind must share the flag (ErrNotJoinLeft)"
  - "Schedule also refuses a zero chat, a non-positive message ID and a zero sent time (ErrInvalidEntry)"

patterns-established:
  - "deletion.SetApplyHookForTest (testtools file) fails the step between the flag write and the rule writes"
  - "staffGroupLookup package variable is the Staff Group read seam of Schedule"

requirements-completed: []

coverage:
  - id: D1
    description: "chat_delete_rules and scheduled_deletions exist with named constraints and partial indexes, for both appliers, idempotent"
    requirement: "DEL-01"
    verification:
      - kind: unit
        ref: "go test -tags testtools -count=1 -run '^TestScheduledDeletionMigrationStatements$' ./fuku/db/migrations"
        status: pass
      - kind: integration
        ref: "TestScheduledDeletionMigrationDefaults on PostgreSQL 16 (throwaway cluster)"
        status: pass
  - id: D2
    description: "rules read through the cache with a key every replica reads fresh; writes drop it"
    requirement: "DEL-01"
    verification:
      - kind: unit
        ref: "TestDeleteRules, TestSkipLocalMatchesFreshnessCriticalKeys"
        status: pass
  - id: D3
    description: "queue keeps the earliest time, hands each row to one claimer, guards completion by token, refuses a Staff Group"
    requirement: "DEL-06"
    verification:
      - kind: integration
        ref: "TestScheduleKeepsEarliest, TestClaimBatchOneWinner (SQLite and PostgreSQL 16), TestCompleteIsTokenGuarded, TestReleaseStaleClaims, TestScheduleRefusesStaffGroup"
        status: pass
  - id: D4
    description: "the shared /cleanservice flag and the delay rows change in one transaction"
    requirement: "DEL-01"
    verification:
      - kind: unit
        ref: "TestApplyJoinLeftRule, TestSetCleanServiceTx"
        status: pass
  - id: D5
    description: "neither table is exported, imported or reset"
    requirement: "DEL-06"
    verification:
      - kind: unit
        ref: "TestDeletionTablesStayOutOfBackup"
        status: pass
---

# Phase 16 Plan 01: Deletion storage Summary

**Two tables, a rules cache read that no replica serves from process memory, and a durable queue with an earliest-wins upsert, a one-winner claim and token-guarded completion, with the three PostgreSQL guarantees proved on PostgreSQL 16 and wired into CI.**

## Performance

- **Duration:** about 15 minutes by the commit clock (09:12 to 09:27 UTC; the start time was not recorded with `date`)
- **Tasks:** 3 of 3
- **Files modified:** 20

## Accomplishments

- Migration `20261110120000_add_scheduled_deletion.sql`: four idempotent statements (two `CREATE TABLE IF NOT EXISTS`, two partial `CREATE INDEX IF NOT EXISTS`), inline named constraints, no top-level `ADD CONSTRAINT`, `runner.go` and `scripts/migrate_psql.sh` untouched.
- `fuku/db/deletion`: the 17 kinds (`kinds.go`), cached `GetRules`/`GetRulesFresh`, `SetRule`/`RemoveRule` (Go 0 written, kind and range checked before any write, key dropped), `Schedule`, `ClaimDue`, `Complete`, `Release`, `ReleaseStale`, `DropChatPending`, `NewClaimToken`, `ApplyJoinLeftRule`, `ApplyCleanServiceFlag`.
- `greetings.SetCleanServiceTx` (column-targeted flag write on a transaction handle).
- `delete_rules` prefix in `skipLocal` with its two test rows.
- `TestDeletionTablesStayOutOfBackup`; both models in the backup and module-harness AutoMigrate lists.
- Makefile `test-postgres-integrity` runs `TestScheduleKeepsEarliest`, `TestClaimBatchOneWinner`, `TestScheduledDeletionMigrationDefaults` and the `./fuku/db/deletion` package; AGENTS.md has two Data bullets and the guarantee list.

## Signatures later plans use

```go
Schedule(ctx, Entry{ChatID, MessageID int64; Kind string; SentAt time.Time; DelaySecs int}) (bool, error)
ClaimDue(ctx, now time.Time, limit int, token string) ([]models.ScheduledDeletion, error)   // use a fresh NewClaimToken() per call
Complete(ctx, token string, ids []int64) error
Release(ctx, token string, ids []int64, countAttempt bool, maxAttempts int) error
ReleaseStale(ctx, cutoff time.Time) (int64, error)
DropChatPending(ctx, chatID int64, kindPrefix string) (int64, error)
GetRules(ctx, chat) (*Rules{ChatID; Delays map[string]int}, error) / GetRulesFresh / SetRule / RemoveRule
ApplyJoinLeftRule(ctx, chat, kind, JoinLeftState{Mode, DelaySecs}) error
ApplyCleanServiceFlag(ctx, chat, on bool) error
```

Errors: `ErrUnknownKind`, `ErrDelayOutOfRange`, `ErrInvalidEntry`, `ErrInvalidToken`, `ErrInvalidBatch`, `ErrNotJoinLeft`, `ErrInvalidJoinLeftState`. Models: `models.DeleteRule`, `models.ScheduledDeletion` (`ScheduledDeletionPending`/`Acting` constants).

## Task Commits

1. Task 1 RED `5378929` test(16-01): deletion rules are stored and read through the cache
2. Task 1 GREEN `1f675ae` feat(16-01): store the deletion rules and the queue tables
3. Task 2 RED `0168b29` test(16-01): the deletion queue claims once and keeps the earliest time
4. Task 2 GREEN `5835249` feat(16-01): claim and schedule deletions once, keep the earliest time
5. Task 3 `e9bf55b` docs(16-01): list the deletion tables and their PostgreSQL guarantees

The two RED commits do not compile on purpose (the package or its symbols do not exist yet); each GREEN commit makes them pass.

## Verification (real results)

All run from /home/user/Fuku_Robot.

| Gate | Result |
|------|--------|
| `go test -tags testtools -race -count=1 -run '^(TestScheduleRules\|TestScheduleKeepsEarliest\|TestScheduleRefusesStaffGroup\|TestClaimBatchOneWinner\|TestCompleteIsTokenGuarded\|TestReleaseStaleClaims\|TestApplyJoinLeftRule\|TestDeleteRules)$' -v ./fuku/db/deletion` | all PASS, no SKIP |
| `go test -tags testtools -race -count=20 -run '^TestClaimBatchOneWinner$' ./fuku/db/deletion` (SQLite) | ok |
| `-count=10 -run '^TestScheduleKeepsEarliest$'` (SQLite) | ok |
| `go test -tags testtools -race -count=1 ./fuku/db/deletion ./fuku/db/cache ./fuku/db/greetings ./fuku/db/backup ./fuku/db/models ./fuku/db/staff ./fuku/db/memberreq` | all ok |
| `go test -tags testtools -count=1 -run '^(TestScheduledDeletionMigrationStatements\|TestMemberRequirementAIMigrationStatements\|TestRulesMediaMigrationStatements)$' ./fuku/db/migrations` | PASS x3 |
| `go test -tags testtools -race -count=1 -run '^(TestDeletionTablesStayOutOfBackup\|TestMemberRequirementsStayOutOfBackup)$' ./fuku/db/backup` | PASS x2 (SQLite) |
| `go test -tags testtools -race -count=1 -run '^(TestSettingsMainGrid\|TestSettingsStaffPage)' ./fuku/modules` | ok (harness list compiles, settings suite unchanged) |
| `go vet -tags testtools ./fuku/db/... ./fuku/modules`, `CGO_ENABLED=0 go build ./...` | clean |
| `gofmt -l` on every file touched | nothing |
| `python3 .planning/phases/13-member-requirements/handoff/lint-local.py 8c0c827` (read directly, run twice) | exit 0 |
| `git diff --exit-code 8c0c827 -- go.mod go.sum`; `git diff --exit-code dd61b2c -- fuku/db/migrations/runner.go scripts/migrate_psql.sh .github/workflows/ci.yml` | exit 0 |
| Makefile gate (`make -n` capture: each of the three names inside the anchored alternation prints a single `1`; `./fuku/db/deletion` once; the `)$'` anchor once) | pass |
| acceptance greps (6 queue functions, `LEAST(`/`MIN(scheduled_deletions.delete_at` 1 each, `math/rand` 0, `SetCleanServiceTx` 1, no gorm default tags, both models in both lists) | pass |

**PostgreSQL 16 proof** (throwaway cluster `/var/lib/postgresql/fuku-del-pg`, trust auth, 127.0.0.1:55436, database `fuku_test`): `FUKU_TEST_MIGRATION_CHAIN=true ... -run '^TestRepositoryMigrationChain$' ./fuku/db/migrations` ok; then `FUKU_TEST_DATABASE=true ... -race -p 1 -run '^(TestScheduleKeepsEarliest|TestClaimBatchOneWinner|TestScheduledDeletionMigrationDefaults)$' -v ./fuku/db/deletion ./fuku/db/migrations`: `TestScheduleKeepsEarliest` (3 subtests), `TestClaimBatchOneWinner` and `TestScheduledDeletionMigrationDefaults` all PASS, 0 SKIP lines. Also on PostgreSQL: `-count=20` of the claim and earliest tests ok, and the whole `./fuku/db/deletion` and `./fuku/db/greetings` packages ok. The cluster was stopped with `pg_ctl -m fast stop` and the data directory deleted (`test ! -e /var/lib/postgresql/fuku-del-pg` holds). No credential or DSN with a password was written anywhere.

**Not run:** `make lint` (cannot run here, golangci-lint is built with Go 1.25; the stand-in ran instead), `make test`, `make check-translations`/`check-docs` (no locale or docs change in this plan), the whole `fuku/modules` package, gosec.

`TestScheduledDeletionMigrationDefaults` and `TestScheduledDeletionMigrationStatements` are characterizations of the Task 1 migration and passed on first run.

## Decisions Made

See `key-decisions`. In short: token-taking functions refuse an empty token, `Release` is one transaction, and `Complete`/`Release` return only `error`.

## Deviations from Plan

**1. [Gate text] `grep -c 'delete_rules' fuku/db/cache/local.go` "at least 2".** The skipLocal clause names the constant `deleteRulesPrefix`, not the string, so the grep counted 1. The property (the key is listed in `skipLocal`) is proved by `TestSkipLocalMatchesFreshnessCriticalKeys`; I also put the literal `delete_rules` in the explanatory comment so the grep reads 2. Corrected gate text: "the line `deleteRulesPrefix = CacheKey("delete_rules") + ":"` and the `HasPrefix(key, deleteRulesPrefix)` clause both exist and the cache test rows pass".

**2. [Plan text] AGENTS.md has no existing `/cleanservice` mention** to extend, so the one sentence about the shared flag and the single transaction lives in the `chat_delete_rules` Data bullet instead (it also records the D-26 command write).

**3. [Added tests]** `TestKindRegistry` (kind registry invariants) and `TestSetCleanServiceTx` in `fuku/db/greetings/repository_test.go` (the plan's "greetings repository test"), plus a concurrent subtest in `TestScheduleKeepsEarliest` so the PostgreSQL guarantee covers real contention.

**4. [Process] Commit ledger.** Protocol 0c (the `gsd-plan-head-before-16-01` file) was not created before the first commit; `commits:` and `plan_head_before` are measured from `dc324b3`, the HEAD at plan start, so the count is still measured (5), not narrated.

**5. [Pre-existing, not touched]** On PostgreSQL the whole `./fuku/db/backup` package fails in `ClearChatData` (`chk_warns_mode`, `chk_captcha_mode` check violations while restoring defaults), `TestMemberRequirementsStayOutOfBackup` included; the backup package is in the integrity list only through its named tests, and `TestDeletionTablesStayOutOfBackup` is not in the list (it runs on SQLite in `make test`, like its model). Out of scope.

## Known Stubs

None. Nothing reads or acts on the tables yet (the plan's stated end state); later plans wire them.

## Threat Flags

None beyond the plan's threat model (T-16-01 to T-16-05, T-16-SC all mitigated as listed: validated kinds and delays plus the named CHECKs, idempotent migration for both appliers, Staff Group refusal in `Schedule`, one conditional claim with token-guarded completion, neither table in `AllExportableModules`, no change to `go.mod`/`go.sum`).

## Requirements

`requirements-completed: []`. DEL-01, DEL-06 and DEL-07 stay Pending; the closing plan 16-09 completes them.

## Self-Check: PASSED

Files created exist (migration, both models, `kinds.go`, `rules.go`, `queue.go`, `hook_testtools.go`, the three package test files, the migration test, the backup test) and commits `5378929`, `1f675ae`, `0168b29`, `5835249`, `e9bf55b` are ancestors of HEAD.
