---
phase: 03-staff-audit-and-undo
plan: 01
subsystem: database
tags: [gorm, postgres, sqlite, audit, telegram, staff-actions, write-ahead]

requires:
  - phase: 02-staff-actions-across-groups
    provides: staff action card, fan-out coordinator, decideStaffAction, staffPaced, RekeyChat
provides:
  - staff_actions and staff_action_groups tables (migration 20261005120000) with prior-state and undo columns
  - models.StaffAction, models.StaffActionGroup and outcome constants
  - staff.CreateAction, SavePrior, SaveGroupResult, FinalizeAction, GetActionFresh, ListActionGroupsFresh
  - staffPriorState / staffPriorFromMember / staffPriorFromRow and the record glue in the modules package
  - a durable record for every confirmed staff action, written at Confirm, per group before each Telegram write, and finalized when the run ends
affects: [03-02 undo decision, 03-03 log posts, 03-04 history view, 03-06, 03-07, 03-08]

actuals:
  tokens: 17700
  tasks: 3
  commits: 4
plan_head_before: 5754f9e0795a55f0c06e247d16e186d8d8ee3218
plan_head_after: 4e27b91e89e6ba3a52384c1c82a9b01a8352a575

tech-stack:
  added: []
  patterns:
    - "Write-ahead prior state: the target's live state is committed before the Telegram write, and a failed write fails that group closed"
    - "Audit tables are never cached: fresh reads only, so no DeleteCache applies"
    - "Test seams as package variables (staffCreateActionRecord, staffSavePrior) for fail-closed paths with real fixtures"

key-files:
  created:
    - migrations/20261005120000_add_staff_actions.sql
    - alita/db/models/staff_action.go
    - alita/db/staff/actions.go
    - alita/db/staff/actions_test.go
    - alita/modules/staff_action_record.go
    - alita/modules/staff_action_record_test.go
  modified:
    - alita/db/staff/rekey.go
    - alita/db/staff/rekey_test.go
    - alita/db/staff/testmain_test.go
    - alita/db/testmain_test.go
    - alita/modules/test_harness_test.go
    - alita/modules/staff_helpers_test.go
    - alita/modules/staff_action_fake_test.go
    - alita/modules/staff_action_card.go
    - alita/modules/staff_action_run.go
    - AGENTS.md

key-decisions:
  - "Prior-state columns are snapshot-json: prior_status, prior_is_member, prior_until, prior_permissions (owner answer at Task 1)"
  - "Record created at Confirm after every check that can still abort; a failed create aborts the card with no write (fail closed)"
  - "Prior state written before each Telegram write via staffSavePrior; failure returns fail_internal with no write"
  - "summary_chat_id is immutable; staff_chat_id follows RekeyChat; group_chat_id is never re-keyed"
  - "Record writes use db.DB directly, never the run's context, which a shutdown cancels first"

patterns-established:
  - "Audit record: parent row at Confirm, one pending child row per link in link order, results as groups finish, one finalizing transaction"
  - "SQL-only parent/child foreign key; GORM models declare no relation"

requirements-completed: [STAFF-10]

coverage:
  - id: D1
    description: "A confirmed staff action leaves one staff_actions row with issuer, target, action, duration, reason, until_date, group_count, summary message and finished_at, and one staff_action_groups row per linked group in link order with outcome, reason and applied_at"
    requirement: "STAFF-10"
    verification:
      - kind: unit
        ref: "alita/modules/staff_action_record_test.go#TestStaffActionRecordFinalize"
        status: pass
      - kind: unit
        ref: "alita/modules/staff_action_record_test.go#TestStaffActionRecordPerGroup"
        status: pass
    human_judgment: false
  - id: D2
    description: "The target's state before the action (status, membership, end date, full permission set as JSON for a restricted target) is committed per group before its Telegram write, for every action kind"
    requirement: "STAFF-10"
    verification:
      - kind: unit
        ref: "alita/modules/staff_action_record_test.go#TestStaffActionRecordsPriorState"
        status: pass
      - kind: unit
        ref: "alita/modules/staff_action_record_test.go#TestStaffActionRecordEveryKind"
        status: pass
    human_judgment: false
  - id: D3
    description: "A failed record create aborts the card with no write and releases the lock; a failed prior write fails that one group closed; an aborted card leaves no record"
    requirement: "STAFF-10"
    verification:
      - kind: unit
        ref: "alita/modules/staff_action_record_test.go#TestStaffActionRecordFailureFailsClosed"
        status: pass
      - kind: unit
        ref: "alita/modules/staff_action_record_test.go#TestStaffActionRecordNoRecordOnAbort"
        status: pass
    human_judgment: false
  - id: D4
    description: "A shutdown mid-run leaves a truthful record (no pending row, fail_interrupted, finished_at set), and the record is read back from the database with no in-memory state"
    requirement: "STAFF-10"
    verification:
      - kind: unit
        ref: "alita/modules/staff_action_record_test.go#TestStopStaffActionsRecordsInterrupted"
        status: pass
      - kind: unit
        ref: "alita/db/staff/actions_test.go#TestStaffActionRecordRoundTrip"
        status: pass
    human_judgment: false
  - id: D5
    description: "RekeyChat moves staff_actions.staff_chat_id and leaves summary_chat_id and group_chat_id unchanged"
    requirement: "STAFF-10"
    verification:
      - kind: unit
        ref: "alita/db/staff/rekey_test.go#TestRekeyChatMovesStaffActions"
        status: pass
    human_judgment: false
  - id: D6
    description: "Migration 20261005120000 applies on a real PostgreSQL server and its checksum is recorded"
    requirement: "STAFF-10"
    verification: []
    human_judgment: true
    rationale: "No PostgreSQL server could be started in this worktree (the isolation guard refused su and runuser, which the plan's throwaway-cluster command needs). The statement splitter was checked on the file (3 statements) and the same model schema runs on SQLite, but TestRepositoryMigrationChain must be run against PostgreSQL by CI or the orchestrator."

duration: 13min
completed: 2026-10-05
status: complete
---

# Phase 3 Plan 1: Staff audit record Summary

**Every confirmed staff action now leaves a durable audit record in two new tables, with each group's pre-action state (status, membership, end date, full permission set as JSON) committed before its Telegram write, results saved as groups finish, and one finalizing transaction at the end.**

## Performance

- **Duration:** 13 min
- **Started:** 2026-10-05T17:25:48Z
- **Completed:** 2026-10-05T17:38:25Z
- **Tasks:** 3 (Task 1 was a pre-answered decision checkpoint)
- **Files modified:** 16 (6 created, 10 modified)

## Accomplishments

- A confirmed staff `/ban` creates one `staff_actions` row and one `staff_action_groups` row per linked group (in link order) at Confirm. If that cannot be stored, the card aborts with `staff_act_abort_check_failed`, the target lock is released and no group gets a write.
- Each acted-on group's prior state is committed before its Telegram write from the same live `getChatMember` read the verdict used. A failed write gives `fail_internal` for that group and no write call there, while the other groups proceed.
- Results are saved as groups finish; the coordinator finalizes the record in one transaction right after `sweepPending`, so a shutdown mid-run leaves every unfinished group as `failed` / `fail_interrupted` with `finished_at` set. Record writes never use the run's cancelled context.
- `staff.RekeyChat` moves `staff_actions.staff_chat_id` with the Staff Group but never `summary_chat_id` or any `group_chat_id`.
- AGENTS.md states the audit record rules (Data, Subsystem traps, Testing).

## Task Commits

1. **Task 1: Confirm the pre-action state columns** - no commit (decision checkpoint, answered `snapshot-json` before this run started)
2. **Task 2: Tracer, a confirmed staff /ban is recorded** - `ccb681a` (test, RED) then `d91545c` (feat, GREEN)
3. **Task 3: Every kind, fail-closed writes, shutdown, chat migration, AGENTS.md** - `7542caf` (test, RED) then `4e27b91` (feat, GREEN)

**Plan metadata:** the `docs(03-01)` commit that holds this file.

## Files Created/Modified

- `migrations/20261005120000_add_staff_actions.sql` - two tables, three CHECK constraints, one index, SQL-only parent/child foreign key
- `alita/db/models/staff_action.go` - `StaffAction`, `StaffActionGroup`, outcome constants
- `alita/db/staff/actions.go` - `CreateAction`, `SavePrior`, `SaveGroupResult`, `FinalizeAction`, `GetActionFresh`, `ListActionGroupsFresh`; never cached
- `alita/db/staff/rekey.go` - re-keys `staff_actions.staff_chat_id`
- `alita/modules/staff_action_record.go` - prior-state capture and JSON mapping, record builder, test seams, result save and finalize
- `alita/modules/staff_action_card.go` - `ActionID` and `IssuerName` (in-memory only); the record is created in `staffActionConfirm` before the run
- `alita/modules/staff_action_run.go` - write-ahead prior state, per-group result save, finalize before delivery
- `alita/modules/staff_action_fake_test.go` - `staffFakeMember.Perms`, restricted members report and store a full permission set
- `alita/modules/staff_action_record_test.go`, `alita/db/staff/actions_test.go`, `alita/db/staff/rekey_test.go` - the tests listed in the plan
- `alita/modules/test_harness_test.go`, `alita/db/staff/testmain_test.go`, `alita/db/testmain_test.go`, `alita/modules/staff_helpers_test.go` - audit models in the AutoMigrate lists, audit rows in `staffCleanup`
- `AGENTS.md` - audit record rules

## Decisions Made

### Task 1 (checkpoint:decision, gate blocking-human): pre-action state columns

- **Owner answer:** `snapshot-json`
- **Columns fixed by the first migration:** `prior_status`, `prior_is_member`, `prior_until`, `prior_permissions` (the full `gotgbot.ChatPermissions` set as JSON text, filled only for restricted targets).
- **Rationale accepted by the owner:** `restrictChatMember` replaces the whole permission set (RESEARCH Pitfall 1), so only a stored permission set lets Undo restore a partial restriction exactly (D-02 "puts back exactly that"). JSON of `ChatPermissions` is self-describing, keeps permission names Telegram adds later, and behaves the same on PostgreSQL and SQLite. Plans 03-02, 03-06 and 03-08 read `prior_permissions`.

### Implementation decisions

- The record is created at Confirm after every check that can still abort; an action is never applied without one.
- The prior state is written before the Telegram write (write-ahead). A crash between the write call and the result write leaves that row `pending` with prior state set; it is not undoable (accepted residual hole, RESEARCH A8).
- `summary_chat_id` is immutable and `staff_chat_id` follows `RekeyChat`, because a message ID only means something in the chat it was sent to.
- Record writes use `db.DB` directly, never the run's context.
- `staffCreateActionRecord` and `staffSavePrior` are package-variable seams (the `staffUserLookup` precedent) so tests prove the fail-closed paths with real fixtures.

## TDD Gate Compliance

Both code tasks followed RED then GREEN with no refactor commit.

- Task 2: RED `ccb681a` (all three target tests failed on "no staff_actions row for card message", an assertion on the planned behavior), GREEN `d91545c`.
- Task 3: RED `7542caf` (only `TestRekeyChatMovesStaffActions` failed, on `staff_chat_id` not moved; the plan expected the other new tests to already pass because Task 2's design satisfies them), GREEN `4e27b91`.
- `gsd_run check tdd-red-evidence` could not run: the worktree copy of gsd-tools is missing `vendor/re2js.cjs`, and the main-checkout copy was refused by the worktree guard. RED evidence was verified by reading the failing test output.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Models and read functions are in the Task 2 RED commit**
- **Found during:** Task 2 (RED)
- **Issue:** The RED tests read the record through `models.StaffAction` and `staff.ListActionGroupsFresh`, and the harness AutoMigrate lists reference the models, so the test package does not compile without them. A compile error is INVALID_RED, not a failing assertion.
- **Fix:** `alita/db/models/staff_action.go` and the two read-only functions in `alita/db/staff/actions.go` went into the RED commit `ccb681a`; nothing wrote a record yet, so every test failed on its first assertion. The write functions came in the GREEN commit.
- **Files modified:** `alita/db/models/staff_action.go`, `alita/db/staff/actions.go`
- **Committed in:** `ccb681a`

**2. [Rule 1 - Bug in plan] "Unmute over a partial restriction" cannot use CanSendMessages true**
- **Found during:** Task 3 (tests)
- **Issue:** The plan's behavior gives the partial set `CanSendMessages true` yet says the member "reads muted". The fake reports `can_send_messages` from the permission set (as the plan also specifies), so a set with `CanSendMessages true` reads unmuted and `/unmute` is skipped with no write and no prior state.
- **Fix:** The test's partial set has `CanSendMessages false` (so the staff's unmute applies) and keeps `CanSendPolls`, `CanInviteUsers` and `CanPinMessages` true; it asserts the stored JSON decodes to exactly that set (the three optional flags come back as explicit false, since the live read carries them as plain booleans).
- **Files modified:** `alita/modules/staff_action_record_test.go`
- **Committed in:** `7542caf`

---

**Total deviations:** 2 auto-fixed (1 blocking, 1 plan inconsistency)
**Impact on plan:** No scope change. Both keep the plan's intent.

## Issues Encountered

- **PostgreSQL migration chain not run here.** The plan's Task 3 verify starts a throwaway cluster with `su postgres -c ...`; the worktree isolation guard refuses `su` and `runuser`, and PostgreSQL will not run as root. In place of it: the runner's statement splitter was run on the new file (3 statements: table, index, table) with a throwaway test that was not committed, the migrations package tests pass, and the same models migrate on SQLite. `TestRepositoryMigrationChain` still has to run against PostgreSQL, in CI (which runs it) or from the main checkout by the orchestrator.
- `make lint` could not run: the installed golangci-lint is built with go1.25 and refuses the go1.26.0 module. `gofmt`, `go vet -tags testtools` and `CGO_ENABLED=0 go build ./...` are clean.
- Not in this plan's scope: `gofmt -l` flags `alita/modules/greetings_command_test.go`, which was already unformatted at the base commit.

## Verification Results

- `go test -tags testtools -race -count=1 ./alita/modules ./alita/db/staff ./alita/db` - ok (modules 115 s)
- `-run '^TestStaffAction|^TestStopStaffActions' ./alita/modules` (Phase 2 regression plus the new tests) - ok
- `CGO_ENABLED=0 go build ./...` and `go vet -tags testtools ./alita/modules ./alita/db/staff ./alita/db/models` - clean
- `git diff --exit-code go.mod go.sum` - clean
- Task 2 and Task 3 acceptance criteria (grep and file checks) - all pass
- `TestRepositoryMigrationChain` against PostgreSQL - not run (see Issues Encountered)

## Known Stubs

None.

## Threat Flags

None. The two new tables are the plan's own surface (T-03-01 to T-03-04): created before the run, never cached, outside backup/export/import/reset, and read only through fresh queries.

## User Setup Required

None - no external service configuration required. The migration applies with `AUTO_MIGRATE=true`, which the deploy manifests already set.

## Next Phase Readiness

- Plans 03-02, 03-03, 03-04, 03-06, 03-07 and 03-08 can read `staff_actions` and `staff_action_groups`; `prior_permissions` holds the JSON of `gotgbot.ChatPermissions` (explicit false for `CanReactToMessages`, `CanEditTag`, `CanManageTopics`), and `staffPriorFromRow` decodes it.
- The undo columns (`undo_by`, `undo_by_name`, `undo_started_at`, `undo_finished_at`, per-group `undo_*`) exist and are unused.
- Before relying on the migration in production, run `TestRepositoryMigrationChain` against PostgreSQL.

## Self-Check: PASSED

- Created files exist: migration, model, repository and its test, record glue and its test.
- Commits `ccb681a`, `d91545c`, `7542caf` and `4e27b91` exist; `test(03-01)` precedes `feat(03-01)` for both code tasks.

---
*Phase: 03-staff-audit-and-undo*
*Completed: 2026-10-05*
