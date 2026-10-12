---
phase: 01-staff-group-links
plan: 04
subsystem: staff-groups
tags: [postgres, plpgsql, trigger, advisory-lock, migrations, ci, d-10]

requires:
  - phase: 01-01
    provides: "staff_groups and staff_group_links tables, uniqueStaffChatID and cleanupStaffRows test helpers"
provides:
  - "staff_role_exclusivity_guard() plpgsql function and triggers trg_staff_groups_role_exclusivity / trg_staff_group_links_role_exclusivity (migration 20261004130000)"
  - "Advisory-lock key format alita:staff_role:<chat_id>"
  - "TestStaffExclusivityTrigger (PostgreSQL-gated, 5 subtests including a 20-round concurrency race)"
  - "Makefile test-postgres-integrity and check_test_results postgresSkip wiring for alita/db/staff"
affects: [01-02, 01-03, 01-05, 01-10]

actuals:
  tokens: 3500
  tasks: 2
  commits: 2

plan_head_before: 78fe65cca2f1c9e0700017f97590e3ff68edc7bf
plan_head_after: 8d14295712db061572663c640d39751c284ab9af

tech-stack:
  added: []
  patterns:
    - "BEFORE trigger takes pg_advisory_xact_lock per chat (ascending order for two-chat writes), then checks the other table; READ COMMITTED gives the check a fresh snapshot after the lock wait"
    - "PostgreSQL-only test skips on SQLite and is whitelisted in scripts/check_test_results only while ALITA_TEST_DATABASE is not true"

key-files:
  created:
    - migrations/20261004130000_add_staff_role_exclusivity_trigger.sql
    - alita/db/staff/exclusivity_postgres_test.go
  modified:
    - Makefile
    - scripts/check_test_results/main.go
    - scripts/check_test_results/main_test.go

key-decisions:
  - "Owner chose pg-trigger for D-10 (Task 1 checkpoint:decision, blocking-human): enforce cross-table role exclusivity in PostgreSQL with a trigger and per-chat advisory lock, not app-level checks only"
  - "A trigger rejection reaches the repository as a generic DB error (fail closed); the precise reason comes from the app-level check on retry, so alita/db/staff/repository.go is untouched"

patterns-established:
  - "Concurrency tests for DB guards widen the race window with pg_sleep inside the open transaction and were mutation-checked by removing the lock"

requirements-completed: [SETUP-04]

coverage:
  - id: D1
    description: "PostgreSQL rejects a Staff Group for a linked chat, a link of a Staff Group, and a link whose Staff Group is itself linked, with a 'staff role conflict' check_violation"
    requirement: "SETUP-04"
    verification:
      - kind: integration
        ref: "ALITA_TEST_DATABASE=true make test-postgres-integrity#TestStaffExclusivityTrigger"
        status: pass
    human_judgment: false
  - id: D2
    description: "Concurrent /setstaff and link of the same chat on separate connections never both commit (20 rounds)"
    requirement: "SETUP-04"
    verification:
      - kind: integration
        ref: "ALITA_TEST_DATABASE=true make test-postgres-integrity#TestStaffExclusivityTrigger/ConcurrentSetStaffAndLinkNeverBothCommit"
        status: pass
    human_judgment: false
  - id: D3
    description: "A legitimate Staff Group, a legitimate link, and a re-key of a Staff Group chat ID with its links still succeed"
    requirement: "SETUP-04"
    verification:
      - kind: integration
        ref: "ALITA_TEST_DATABASE=true make test-postgres-integrity#TestStaffExclusivityTrigger/LegitimateRowsAccepted"
        status: pass
    human_judgment: false
  - id: D4
    description: "make test accepts the trigger test's SQLite skip only when ALITA_TEST_DATABASE is not true; make test-postgres-integrity runs it"
    requirement: "SETUP-04"
    verification:
      - kind: unit
        ref: "go test -count=1 ./scripts/check_test_results#TestCheckResultsAcceptsStaffTriggerSkipOnlyWithoutPostgres"
        status: pass
      - kind: other
        ref: "make test (ALITA_TEST_DATABASE unset) exit 0, zero 'unexpected test skip' lines"
        status: pass
    human_judgment: false

duration: 25min
completed: 2026-10-04
status: complete
---

# Phase 1 Plan 04: Staff Role Exclusivity Trigger Summary

**A PostgreSQL BEFORE trigger with a per-chat advisory lock makes it impossible for a chat to be both a Staff Group and a linked group, even when two replicas race, proven against a real PostgreSQL 16 including a 20-round concurrency subtest.**

## Task 1 outcome: the D-10 decision

Task 1 was a `checkpoint:decision` with `gate="blocking-human"`. The orchestrator put both options to the owner before dispatch, with pg-trigger marked recommended and not as a default. The owner's reply was exactly **"pg-trigger"**.

- **Chosen:** PostgreSQL trigger plus per-chat advisory lock (Assumption A8 / Open Question 4).
- **Rationale:** D-10 says to enforce the rule "in the database where possible, not only in code". The trigger closes the two-replica race inside the database and fails closed. The app-only option would leave a narrow race (only a chat's own owner can start `/setstaff` and a link of the same chat together) with no automatic repair, since the plan 01-10 sweep removes only orphan links. Plpgsql precedent already exists in the captcha migration.
- **Cost accepted:** PostgreSQL-only behaviour, one more migration, and one more test entry in the Makefile. It is one-way: removing it needs a new migration that drops the trigger.
- The decision was recorded here from the orchestrator's relayed answer. No separate commit exists for Task 1 because it produces no code.

## Performance

- **Duration:** about 25 min (start time was not captured at spawn, so this is approximate)
- **Tasks:** 2 (1 decision checkpoint resolved by the owner, 1 TDD task)
- **Files:** 5 changed (2 created, 3 modified)

## Accomplishments

- Migration `20261004130000_add_staff_role_exclusivity_trigger.sql`: `staff_role_exclusivity_guard()` plus one BEFORE trigger on each table. On `staff_groups` it locks `alita:staff_role:<chat_id>` and rejects a chat that is a link's `group_chat_id`. On `staff_group_links` it locks both chat IDs lowest first, rejects a `group_chat_id` that is a Staff Group, and rejects a `staff_chat_id` that is itself some other link's `group_chat_id`. All use `ERRCODE = 'check_violation'` and a message starting `staff role conflict`.
- `TestStaffExclusivityTrigger` with five subtests (three rejections, legitimate rows plus a Staff Group re-key in one transaction, and a 20-round concurrent race on separate transactions that hold their insert open with `pg_sleep(0.05)`).
- `make test-postgres-integrity` now runs the test on `./alita/db/staff`; `scripts/check_test_results` accepts its SQLite skip only while `ALITA_TEST_DATABASE != "true"` and still rejects any other staff-package skip.
- Verified for real on PostgreSQL 16 using CI's two commands against a fresh database (migration chain applied the new file; the integrity target then ran every listed test, all green with `-race`).
- **Mutation check:** replacing the function with a copy that has the advisory-lock lines removed makes the concurrency subtest fail in round 0 with both rows committed (`errs = [<nil> <nil>]`). The lock, not the plain check, is what closes the race.

## Task Commits

1. **Task 1: D-10 decision** - owner answered "pg-trigger" (no code, recorded above)
2. **Task 2 RED: failing PostgreSQL test** - `c48f895` (test). On PostgreSQL 4 of 5 subtests failed on the planned assertions (no trigger yet: inserts succeeded, concurrency round 0 committed both); the legitimate-rows subtest passed as expected.
3. **Task 2 GREEN: migration, Makefile, skip gate** - `8d14295` (feat)

## Verification Results

| Check | Result |
|-------|--------|
| `go test -tags testtools -count=1 -run '^TestStaffExclusivityTrigger$' -v ./alita/db/staff` (SQLite) | `--- SKIP: TestStaffExclusivityTrigger` |
| `go test -count=1 ./scripts/check_test_results` | ok |
| `grep -c TestStaffExclusivityTrigger Makefile` | 1 |
| `grep -c "CREATE TRIGGER trg_staff_"` / `pg_advisory_xact_lock` in the migration | 2 / 3 |
| `ALITA_TEST_MIGRATION_CHAIN=true go test ... TestRepositoryMigrationChain` on a fresh PostgreSQL database | PASS, new migration recorded, both triggers present |
| `ALITA_TEST_DATABASE=true make test-postgres-integrity` (same database) | all PASS, including 5 trigger subtests |
| `make test` with `ALITA_TEST_DATABASE` unset | exit 0, no "unexpected test skip" |
| `go build ./...`, `gofmt -l` on changed Go files | clean |
| golangci-lint v2.13.1 `--new-from-rev=78fe65c` (default and dupl passes) | 0 issues each |
| `go.mod` / `go.sum` vs base | unchanged |

## Deviations from Plan

None - plan executed exactly as written. Two environment notes, not deviations:

- `TestRepositoryMigrationChain` refuses a non-empty schema, so it was run against a throwaway database (`alita_chain04`, since dropped) rather than the orchestrator's populated `alita_test`. The integrity step then used that same database, mirroring CI's single database. `alita_test` was not modified.
- The TDD cycle ran RED (`test`) then GREEN (`feat`) with no REFACTOR commit; `workflow.tdd_mode` was off and the plan type is `execute`, so no gate enforcement applied.

## Known Stubs

None.

## Threat Flags

None beyond the plan's threat model. T-01-11 (concurrent set-staff and link) is mitigated and proven by the 20-round subtest plus the lock-removal mutation. T-01-12 (advisory-lock deadlock) is mitigated by ascending lock order; the concurrency subtest completed 20 rounds with no deadlock. T-01-SC: no package changes.

## Notes for Later Plans

- Plans 01-02 and 01-05 keep their in-transaction app checks. A trigger rejection arrives as a generic DB error, so the race loser should surface the could-not-verify reply and the retry gets the precise reason.
- Plan 01-03's `RekeyChat` UPDATEs fire these triggers on PostgreSQL. The `LegitimateRowsAccepted` subtest covers a Staff Group re-key with its links in one transaction. Updating `staff_groups.chat_id` or a link's chat columns takes advisory locks per statement, so a multi-statement re-key across several chats could in theory deadlock with another writer. PostgreSQL aborts one side and the write fails closed.

## Self-Check: PASSED

- Created files exist: migration, `alita/db/staff/exclusivity_postgres_test.go`.
- Commits `c48f895` and `8d14295` are on the branch (`git rev-list --count 78fe65c..HEAD` is 2 before this SUMMARY commit).
- All task acceptance criteria and plan-level verification re-run and passing.
