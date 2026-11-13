---
phase: 17-command-edit-and-self-destruct-deletion
plan: 01
subsystem: database
tags: [postgres, sqlite, gorm, queue, scheduled-deletion, member-kinds, batch-insert]

requires:
  - phase: 16-scheduled-deletion-of-service-and-bot-messages
    provides: fuku/db/deletion (kind registry, cached rules, the queue, Schedule), applyDeleteRule, the Deletion page row tables, the PostgreSQL integrity target
provides:
  - the kinds member:commands, member:edited and member:selfdestruct (20 kinds in all), validated in Go with no migration
  - deletion.IsMemberKind, deletion.NeedsDeleteRight, deletion.ValidateRuleDelay and deletion.ValidateEntryDelay (member kinds checked by value)
  - deletion.ScheduleBatch (ScheduleBatchMax 100), deletion.CountPending and deletion.DropPendingMessage
  - deletionMemberRows (mc, me, ms) in the page's Go row table
  - four PostgreSQL guarantees in make test-postgres-integrity
affects: [17-02, 17-03, 17-04, 17-05, 17-06, 17-07, 17-08, 17-09, 17-10, 17-11, 17-12]

plan_head_before: c4298717f8e6140004202adfdf1927648b5221d9
plan_head_after: 414882884be5c09a79d82ea653f1651e63ae3430

actuals:
  tokens: 11900   # chars/4 over the added lines of c429871..HEAD (47661 chars), tests and docs included
  tasks: 3
  commits: 5      # MEASURED: git rev-list --count c429871..HEAD (before this SUMMARY commit)

tech-stack:
  added: []
  patterns:
    - "per-kind delay validation by value (two validators: one for stored rules, one for queue entries)"
    - "multi-row ON CONFLICT upsert in chunks of 100 after a Go dedupe by key and a sort by key (one lock order across callers)"
    - "GORM create callback registered in a test to count INSERT statements and their row counts"

key-files:
  created:
    - fuku/db/deletion/queue_batch_test.go
  modified:
    - fuku/db/deletion/kinds.go
    - fuku/db/deletion/rules.go
    - fuku/db/deletion/queue.go
    - fuku/db/deletion/rules_test.go
    - fuku/db/deletion/queue_test.go
    - fuku/modules/deletion_apply.go
    - fuku/modules/deletion_apply_test.go
    - fuku/modules/deletion_kinds.go
    - fuku/modules/deletion_kinds_test.go
    - fuku/modules/deletion_e2e_test.go
    - Makefile
    - AGENTS.md

key-decisions:
  - "ScheduleBatch sorts the merged rows by (chat, message) before writing, so two concurrent batches (two replicas) take their row locks in one order and cannot deadlock on PostgreSQL; the plan only asked for the Go dedupe."
  - "A failed Staff Group read inside ScheduleBatch drops that chat's entries and is returned joined with other errors (errors.Join), while other chats are still written (plan deviation note 1, accepted)."
  - "IsMemberKind is a pure prefix test (like IsServiceKind); the registry test checks 'exactly three member kinds' through Kinds(), ValidKind refuses 'member:'."

patterns-established:
  - "Kind-class helpers (IsServiceKind, IsMemberKind, NeedsDeleteRight) in kinds.go; IsBotKind stays a test helper."
  - "Row tables of the Deletion page: deletionServiceRows, deletionBotRows and now deletionMemberRows, all deletionRow."

requirements-completed: []

coverage:
  - id: D1
    description: "Three member kinds exist (20 kinds) and a rule delay is checked by value per kind before any write (SetRule and applyDeleteRule)"
    requirement: "DEL-03, DEL-04, DEL-05 (contribution only)"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -run '^(TestKindRegistry|TestMemberKindDelays|TestDeleteRules|TestApplyDeleteRuleMemberKinds|TestMemberRowPresetsMatchRuleDelays)$' ./fuku/db/deletion ./fuku/modules"
        status: pass
    human_judgment: false
  - id: D2
    description: "ScheduleBatch writes up to 100 entries per statement keeping the earliest time, dedupes in Go, validates first and refuses a Staff Group; CountPending and DropPendingMessage work; all four guarantees pass on SQLite and PostgreSQL 16"
    requirement: "DEL-05, DEL-07 (contribution only)"
    verification:
      - kind: integration
        ref: "FUKU_TEST_DATABASE=true go test -tags testtools -race -p 1 -run '^(TestScheduleBatchKeepsEarliest|TestScheduleBatchDuplicateKey|TestCountPending|TestDropPendingMessage)$' ./fuku/db/deletion (throwaway PostgreSQL 16)"
        status: pass
    human_judgment: false

duration: 45min
completed: 2026-11-12
status: complete
---

# Phase 17 Plan 01: Member kinds, per-kind delays and the batch queue Summary

The deletion package now knows member:commands, member:edited and member:selfdestruct with their exact per-kind delays checked by value, and can write self-destruct entries in 100-row multi-row upserts that keep the earliest time, count a chat's queue with a bounded exact count and drop one pinned message's pending row. Nothing produces member rows yet.

## What was built

- `fuku/db/deletion/kinds.go`: three kind constants appended to the registry (20 kinds), `memberPrefix`, `IsMemberKind`, `NeedsDeleteRight` (service or member true, bot false), the grace presets (30, 60, 300, 900, 3600) and the self-destruct presets (60, 300, 600, 3600, 21600, 86400), `ValidateRuleDelay` (member:commands 0 only, the other two by value, others 0..86400) and `ValidateEntryDelay` (member:commands and member:edited 0 only, member:selfdestruct the six presets, others 0..86400). `validateDelay` stays as the range helper.
- `rules.go` `SetRule` and `queue.go` `Schedule` call the new validators; `deletion_apply.go` `deletionValidate` calls `ValidateRuleDelay` after the unknown-kind and shared-kind checks.
- `queue.go`: `ScheduleBatch` (validate all first, merge by (chat, message) to the earliest time in Go, one fresh Staff Group read per distinct chat, sort, chunk of `ScheduleBatchMax` = 100, one `clause.OnConflict` create per chunk with the table-qualified earliest-wins expression), `CountPending` (`SELECT count(*) FROM (SELECT 1 ... LIMIT ?)`), `DropPendingMessage`.
- `fuku/modules/deletion_kinds.go`: `deletionMemberRows` (mc, me, ms; the UI-SPEC keys; Match nil).
- `Makefile`: `TestScheduleBatchKeepsEarliest|TestScheduleBatchDuplicateKey|TestCountPending|TestDropPendingMessage` inside the anchored `-run` list; `.github/workflows/ci.yml` untouched.
- `AGENTS.md`: the `chat_delete_rules` and `scheduled_deletions` Data bullets and the Testing guarantee list (including the sentence that a stored value a rule does not accept is treated as Off by its reader).

## Verification results (real)

- `go test -tags testtools -race -count=1 ./fuku/db/deletion`: `ok` (whole package, SQLite).
- `-run '^(TestKindRegistry|TestMemberKindDelays|TestDeleteRules|TestScheduleRules|TestScheduleKeepsEarliest|TestApplyJoinLeftRule)$' -v ./fuku/db/deletion`: all PASS, no SKIP.
- `-run '^(TestApplyDeleteRule|TestApplyDeleteRuleMemberKinds|TestApplyCleanServiceRule|TestDeletionKindRegistryMatchesRepository|TestDeletionKindsNeverStaffLockdownCaptchaLog|TestMemberRowPresetsMatchRuleDelays|TestServiceKindTable|TestDeletionEffectiveRule|TestDeletionExcludedMessagesNeverScheduled)$' -v ./fuku/modules`: all 9 PASS, no SKIP (SQLite).
- Wider module regression `-run '^(TestSettingsDeletion|TestDeletion|TestSettingsAuthorityDeletion|TestSettingsDriftDeletion|TestSettingsParityCleanService|TestApplyDeleteRule|TestApplyDeleteRuleMemberKinds|TestMemberRowPresetsMatchRuleDelays|TestBot|TestScheduleBotMessage|TestShutdownEndsDeletionWorker)' ./fuku/modules`: `ok` in 63.8 s.
- `-count=20 -run '^TestScheduleBatchKeepsEarliest$' ./fuku/db/deletion` (SQLite): ok.
- Statement counting: 250 entries are written as INSERTs of [100 100 50] rows (test callback), a 100-row statement succeeds on SQLite.
- `go vet -tags testtools ./fuku/db/deletion ./fuku/modules` clean; `CGO_ENABLED=0 go build ./...` ok; `gofmt -l` on the 12 files prints nothing; `python3 .planning/phases/13-member-requirements/handoff/lint-local.py 8c0c827` exit 0 (read directly): "no dupl, gocyclo or godox findings".
- `git diff --exit-code 8c0c827 -- go.mod go.sum` exit 0; `git diff --exit-code 42c48c4 -- migrations fuku/db/migrations/runner.go scripts/migrate_psql.sh fuku/db/models .github/workflows/ci.yml` exit 0.
- Makefile gate: `make -n test-postgres-integrity` shows each of the four names inside the anchored alternation (each grep prints 1) and the `$` anchor is intact.
- Acceptance greps: `member:` in kinds.go 12; `TestMemberKindDelays` present; `KindMemberEdited` in the e2e test; the four helper funcs 4; `ScheduleBatch|CountPending|DropPendingMessage` funcs 3; `ScheduleBatchMax` 4; `math/rand` 0.
- **PostgreSQL 16 proof** (throwaway cluster `/var/lib/postgresql/fuku-17-01-pg`, trust auth, 127.0.0.1:55437, database `fuku_test`): `FUKU_TEST_MIGRATION_CHAIN=true ... -run '^TestRepositoryMigrationChain$' ./fuku/db/migrations` ok; then `FUKU_TEST_DATABASE=true ... -race -count=1 -p 1 -run '^(TestScheduleBatchKeepsEarliest|TestScheduleBatchDuplicateKey|TestCountPending|TestDropPendingMessage)$' -v ./fuku/db/deletion`: all PASS (the earliest test with its four subtests including the 8-caller concurrent one), no SKIP; `-count=20` of the earliest and duplicate-key tests ok; the whole `./fuku/db/deletion` package ok on PostgreSQL. The cluster was stopped with `pg_ctl -m fast stop` and its data directory deleted (`test ! -e /var/lib/postgresql/fuku-17-01-pg` holds). No credential or DSN with a password was written anywhere.
- Not run: `make lint` (cannot run here, Go 1.25 golangci-lint), `make test` (not this plan's task), the full settings family (the plan asks for it at plan end only in its verification list as a focused `TestSettingsDeletionWrites|TestSettingsDeletionValidation`, which match no test name; the broader `TestSettingsDeletion|TestDeletion...` run above covers the Deletion page).

## Deviations from Plan

### Legitimate Phase 16 test changes (C-12, named as the plan requires; none re-recorded, no assertion weakened)

1. `TestKindRegistry` (`fuku/db/deletion/rules_test.go`): 17 to 20 kinds, "exactly one of service and bot" became "exactly one of service, bot and member", plus the `NeedsDeleteRight` and `IsMemberKind` checks and `member:`/`member:commands ` refusals.
2. `TestDeletionKindRegistryMatchesRepository` (`fuku/modules/deletion_kinds_test.go`): the member table joins the comparison, the section letter follows the kind's prefix (s, b, m), and the member hint keys (`settings_del_hint_commands`, `_edited`, `_self`) join the allowed hint set.
3. `TestDeletionKindsNeverStaffLockdownCaptchaLog`: 17 to 20 and exactly three member kinds (the loop that refuses kinds naming staff, lockdown, captcha, log or raid is unchanged).
4. `deletionShortestRules` (`fuku/modules/deletion_e2e_test.go`): stores the shortest valid preset of each member kind (commands 0, edited 30, self-destruct 60) because `ValidateRuleDelay` now refuses 0 for the other two; its comment says so; no assertion of `TestDeletionExcludedMessagesNeverScheduled` changed, and it passes.

### Auto-added

5. **[Rule 2 - Missing critical functionality] `ScheduleBatch` sorts rows by (chat, message) before writing.** Two replicas flushing overlapping keys in different orders can deadlock on PostgreSQL row locks; a stable order removes it. Files: `fuku/db/deletion/queue.go`. Commit 59336ea.
6. `TestKindRegistry` initially asserted `IsMemberKind("member:") == false`; `IsMemberKind` is a prefix test by plan, so the test's refusal list uses `ValidKind` for `member:` and `IsMemberKind` only for other classes' kinds. Corrected in commit 028f9c6 (a test-authoring error, not a contract change).

### Process notes

- The two "red" test commits (5fee597, 4c42281) do not compile on their own (they name symbols the next commit adds), as the plan prescribes; the green commits follow immediately.
- `deletionMemberRows` has tests as its only users until plan 17-09 adds the page section (the brief's rule for a table a later plan is the only production user of): `TestDeletionKindRegistryMatchesRepository`, `TestMemberRowPresetsMatchRuleDelays`. Its `settings_del_mem_*`, `settings_del_desc_*` and `settings_del_hint_*` strings do not exist yet (plan 17-08); no test reads them before 17-09 extends `TestDeletionKindKeysExist`.
- `fuku/modules/deletion_kinds_helpers_test.go` (`deletionRowOf`, `deletionRowOfKind`) was left as is: it looks only in the service and bot tables and nothing needs a member lookup yet.

## Requirements

`requirements-completed: []`. DEL-03, DEL-04, DEL-05 and DEL-07 are contributed to and stay Pending/ticked as they were; only plan 17-12 completes DEL-03, DEL-04 and DEL-05. `requirements.mark-complete` was not called.

## Known Stubs

None. (`deletionMemberRows` is production data awaiting its page reader in 17-09, not a stub.)

## Threat Flags

None. The batch writer keeps D-17's Staff Group refusal (T-17-03), validates every entry (T-17-02), dedupes before the statement (T-17-04), the count is bounded (T-17-05), delays are checked by value (T-17-01); no new endpoint, auth path or schema.

## Self-Check: PASSED

- Created file `fuku/db/deletion/queue_batch_test.go`: FOUND.
- Commits 5fee597, 028f9c6, 4c42281, 59336ea, 4148828: all reachable from HEAD.
