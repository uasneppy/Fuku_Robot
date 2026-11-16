---
phase: 13-member-requirements
plan: 01
subsystem: database
tags: [migration, gorm, postgres, member-requirements, hold-latch, insert-first-claim]
requires:
  - phase: 12-rules-media-media-under-text-and-greet-once
    provides: greeted_members and ClaimGreeting (the insert-first claim analog), migration 20261105120000
provides:
  - migration 20261106120000_add_member_requirements.sql with chat_requirements and chat_requirement_holds
  - models.ChatRequirements and models.ChatRequirementHold plus the five RequirementAction constants (0..4)
  - fuku/db/memberreq settings (Rule, Rules, ActionOf, AnyRuleOn, GetSettings, GetSettingsFresh, SetRuleAction, SetCheckOnJoin, SetDeleteMessages)
  - fuku/db/memberreq holds (ClaimHold, ReclaimStaleHold, MarkHoldPunished, SetHoldNotice, SetHoldWelcomeHeld, BackoffHold, RevertHoldToPending, TakeHoldByID, RestoreHold, ClearHoldOnClean, ClearHolds, TakeHoldsBefore, GetHoldFresh, GetHoldByIDFresh, StaleClaim)
  - three PostgreSQL guarantees in make test-postgres-integrity
affects: [13-02, 13-05, 13-06, 13-07, 13-08, 13-09, 13-12, 13-14, member-requirements]

actuals:
  tokens: 16000
  tasks: 3
  commits: 6

commits: 6
plan_head_before: 6767e5b4a9668b2a40d71fbc0f36160c3d484210
plan_head_after: 7dd6d380ce28551ef200ab27df991202f17d7c81

tech-stack:
  added: []
  patterns:
    - "Column-targeted upsert per setting (INSERT ... ON CONFLICT (chat_id) DO UPDATE of one column plus updated_at, then cache.DeleteCache), as raiddetection"
    - "Insert-first hold claim (ON CONFLICT DO NOTHING, won = RowsAffected == 1) and one-winner conditional UPDATE reclaim, as ClaimGreeting and the lockdown stale-claim release"
    - "DELETE ... RETURNING through clause.Returning{} for take operations (works on SQLite and PostgreSQL)"

key-files:
  created:
    - migrations/20261106120000_add_member_requirements.sql
    - fuku/db/models/member_requirements.go
    - fuku/db/memberreq/settings.go
    - fuku/db/memberreq/holds.go
    - fuku/db/memberreq/testmain_test.go
    - fuku/db/memberreq/settings_test.go
    - fuku/db/memberreq/holds_test.go
    - fuku/db/migrations/member_requirements_migration_test.go
  modified:
    - Makefile
    - AGENTS.md

key-decisions:
  - "ClaimHold returns (won, err) and leaves the new ID on the hold passed in, as the plan's Artifacts table says (research S-05 listed an extra id return)"
  - "The single-column hold updates and BackoffHold/RevertHoldToPending return (changed bool, err), and ClearHolds returns the number of rows removed, so later plans can tell a stale press from a change"
  - "ReclaimStaleHold reads the winner's ID through UPDATE ... RETURNING id, so the ID is never read in a second, racing query"
  - "In the Makefile the three names are ordered TestRequirementHolds first so the plan's literal acceptance grep 'TestRequirementHolds|' matches; order inside the anchored alternation is irrelevant"

patterns-established:
  - "A hold is a fresh database record with no cache and no Redis; settings are the only cached read of the package"

requirements-completed: []

coverage:
  - id: D1
    description: "A group's rule choices are stored one column at a time, a chat with no row reads every rule off, and the five CHECKs refuse 5 and -1"
    requirement: "MREQ-01"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^TestChatRequirementsSettings$' ./fuku/db/memberreq"
        status: pass
      - kind: integration
        ref: "same command on local PostgreSQL 16 (FUKU_TEST_DATABASE=true), schema built by TestRepositoryMigrationChain"
        status: pass
    human_judgment: false
  - id: D2
    description: "Of 16 concurrent claims of one (chat, user) exactly one wins, and of 16 concurrent reclaims of one stale pending hold exactly one wins"
    requirement: "MREQ-04"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^TestRequirementHoldClaimConcurrent$' ./fuku/db/memberreq"
        status: pass
      - kind: integration
        ref: "same command on local PostgreSQL 16"
        status: pass
    human_judgment: false
  - id: D3
    description: "The take, restore, clean-predicate, clear and take-before-join semantics hold"
    requirement: "MREQ-04"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^TestRequirementHolds$' ./fuku/db/memberreq"
        status: pass
      - kind: integration
        ref: "same command on local PostgreSQL 16"
        status: pass
    human_judgment: false
  - id: D4
    description: "The migration is two idempotent CREATE TABLE statements with every constraint inline and no transaction control, ADD CONSTRAINT or CONCURRENTLY"
    requirement: "MREQ-01"
    verification:
      - kind: unit
        ref: "go test -tags testtools -count=1 -run '^TestMemberRequirementsMigrationStatements$' ./fuku/db/migrations"
        status: pass
    human_judgment: false

status: complete
---

# Phase 13 Plan 01: Member requirements storage Summary

**Two tables (chat_requirements, chat_requirement_holds) and the fuku/db/memberreq repository: column-targeted per-rule settings that read all-off with no row, and a per-member hold that is the one-punishment latch (insert-first claim, one-winner stale reclaim) and the mute record, proved on SQLite and PostgreSQL 16.**

## Performance

- **Tasks:** 3 of 3 (task 1 tracer, tasks 2 and 3 TDD)
- **Commits:** 6 (three RED `test(13-01)`, three GREEN `feat(13-01)`)
- No behaviour reaches a real group: tables and a repository only, no module, handler, locale key or kill-switch change.

## Accomplishments

- Migration `20261106120000_add_member_requirements.sql`: `chat_requirements` (unique chat, five SMALLINT action columns each under its own named CHECK 0..4, `check_on_join` and `delete_messages` NOT NULL DEFAULT FALSE) and `chat_requirement_holds` (unique `(chat_id, user_id)`, `rules > 0`, `action BETWEEN 1 AND 4`, `mute_until`, `welcome_held`, `notice_msg_id`, `created_at`, nullable `punished_at`). Constraints inline, so `runner.go` and `scripts/migrate_psql.sh` are unchanged.
- Models with the same constraint names in their tags, no `default:` tag on any boolean, the five `RequirementAction*` constants.
- `memberreq.GetSettings` through `cache.GetFromCacheOrLoad` under `member_requirements:<chat>`, `GetSettingsFresh` without the cache, three column-targeted upserts that write a Go 0 or false, delete the key and refuse an unknown rule or a value outside 0..4 before any write.
- Hold operations, all fresh database statements with no cache and no Redis: `ClaimHold`, `ReclaimStaleHold`, `MarkHoldPunished`, `SetHoldNotice`, `SetHoldWelcomeHeld`, `BackoffHold`, `RevertHoldToPending`, `TakeHoldByID`, `RestoreHold`, `ClearHoldOnClean` (the research S-05 predicate with the 2 s margin), `ClearHolds`, `TakeHoldsBefore`, `GetHoldFresh`, `GetHoldByIDFresh`.
- Makefile: `TestRequirementHolds`, `TestChatRequirementsSettings`, `TestRequirementHoldClaimConcurrent` inside the `$$`-anchored `-run` list and `./fuku/db/memberreq` in the package list. AGENTS.md: two Data bullets and the Phase 13 guarantees paragraph in Testing.

## TDD Gate Compliance

| Gate | Commit | Evidence |
|------|--------|----------|
| RED task 1 | ba68614 `test(13-01): add failing tests for member requirement settings` | `go test ... -run '^TestChatRequirementsSettings$'` failed on assertions (for example `actions = [-1 -1 -1 -1 -1], want [0 0 0 3 0]`, `a refused write ... err = <nil>, want ErrActionOutOfRange`, `last_name_action = 5 was accepted by the database`), against a compile-only stub; semantic assessment: the target test ran and failed on the planned behaviour, not on a build error |
| GREEN task 1 | d2aa1c4 `feat(13-01): store each group's member requirement settings` | all 7 subtests pass |
| RED task 2 | a266628 `test(13-01): add failing tests for the requirement hold claim` | `winners = 16, losers = 0, want 1 and 15` and `record not found` for the reclaim, against a stub `ClaimHold` that always won; the same commit adds `TestMemberRequirementsMigrationStatements`, which passes at once because the migration already existed since the tracer (it is a guard, not a RED test) |
| GREEN task 2 | 99e9d8d `feat(13-01): claim a member's requirement hold exactly once` | both subtests pass |
| RED task 3 | e8eb600 `test(13-01): add failing tests for taking, restoring and cleaning holds` | `TestRequirementHolds` failed in 4 subtests on assertions against stubs (claim stores every column, take and restore, the clean predicate, clear all and take before a join); the subtests that exercise already-written functions passed |
| GREEN task 3 | 7dd6d38 `feat(13-01): take, restore and clean requirement holds` | all 7 subtests pass |

The `gsd_run check tdd-red-evidence` classifier was not run (the plan is `type: execute`, not a `type: tdd` plan); the RED evidence above is the real console output of the target test.

## Task Commits

1. Task 1 (tracer): ba68614 (RED), d2aa1c4 (GREEN)
2. Task 2: a266628 (RED), 99e9d8d (GREEN)
3. Task 3: e8eb600 (RED), 7dd6d38 (GREEN, also Makefile and AGENTS.md)

## Verification (real results)

- `go test -tags testtools -race -count=1 ./fuku/db/memberreq`: ok (SQLite).
- `go test -tags testtools -count=1 -run '^(TestMemberRequirementsMigrationStatements|TestRulesMediaMigrationStatements|TestSettingsHubMigrationStatements)$' ./fuku/db/migrations`: ok.
- `CGO_ENABLED=0 go build ./...`: ok. `go vet -tags testtools` on `fuku/db/memberreq`, `fuku/db/models`, `fuku/db/migrations`: clean. `gofmt -l` on the files I touched: empty.
- `git diff --exit-code 8c0c827 -- go.mod go.sum` and `-- fuku/db/migrations/runner.go scripts/migrate_psql.sh`: exit 0.
- Lint stand-in `lint-local.py 8c0c827`: exit 0 ("no dupl, gocyclo or godox findings on lines added since 8c0c827"), run before the last code commit and again before this SUMMARY. `make lint` itself cannot run in this sandbox (golangci-lint built with Go 1.25).
- `make generate-docs` left no diff; `make check-docs`: "No drift detected"; `make check-translations`: "All translations are present". `make -n test-postgres-integrity` parses.
- Acceptance greps: the migration statements, `ls migrations | tail -1` prints `20261106120000_add_member_requirements.sql`, the model has 0 non-comment `default:` occurrences, holds.go has no `DeleteCache(`, `GetFromCacheOrLoad(` or `redis` in code lines, `RevertHoldToPending` appears once, the Makefile greps all pass with the `$$` anchor kept, `grep -c chat_requirement_holds AGENTS.md` is at least 1.
- **PostgreSQL 16 proof (local, throwaway cluster).** As the `postgres` OS user: `initdb -A trust` into `/var/lib/postgresql/fuku-mreq-pg`, started with `pg_ctl` on 127.0.0.1:55433 (socket directory inside the data directory), database `fuku_test`. `FUKU_TEST_MIGRATION_CHAIN=true go test -tags testtools -count=1 -run '^TestRepositoryMigrationChain$' ./fuku/db/migrations` under the CI job env: ok, and `pg_constraint` then listed exactly the nine named constraints of the two tables, so the schema came from the migration, not from AutoMigrate. Then `FUKU_TEST_DATABASE=true DATABASE_URL=<local trust DSN, no credential> go test -tags testtools -race -count=1 -run '^(TestChatRequirementsSettings|TestRequirementHoldClaimConcurrent|TestRequirementHolds)$' -v ./fuku/db/memberreq`: PASS for all three tests and all their subtests (the CHECK subtest saw SQLSTATE 23514 for every action column and for 5 and -1; 16-way claim and 16-way reclaim each had one winner; take, restore and clean semantics held with `DELETE ... RETURNING` and an explicit-ID restore). The tests left 0 rows in both tables. **Cleanup:** `pg_ctl -m fast stop`, then `rm -rf /var/lib/postgresql/fuku-mreq-pg`; `test ! -e /var/lib/postgresql/fuku-mreq-pg` exits 0. A defunct (zombie) postgres entry remained in the process table after the stop; it is a reaped-by-parent artefact, holds no port or data. No password or credential was written to any file, commit or this SUMMARY.

## Deviations from Plan

None - plan executed as written. Minor interpretation notes, none of them a rule violation:

- The plan names `ClaimHold(...) (won bool, err error)`; research S-05 listed an extra `id` return. I followed the plan (the ID is on the hold passed in).
- The plan leaves the return types of the single-column hold updates open; they return `(changed bool, error)` (and `ClearHolds` returns the rows removed), which a later plan needs to tell a stale button press from a change.
- `TestMemberRequirementsMigrationStatements` is committed in the task 2 RED commit but passes immediately, because the migration was written in task 1; it is a regression guard, not a failing test.
- SQLite in the repository test harness accepted `UPDATE ... RETURNING` and `DELETE ... RETURNING` through `clause.Returning{}`, so the read-then-delete fallback the plan allowed was not needed.

## Known Stubs

None. (The RED commits carried compile-only stubs; all are replaced in the GREEN commits.)

## Threat Flags

None. No new network endpoint, auth path or file access; the two new tables sit behind the existing database boundary and are covered by T-13-01 to T-13-03 (unique key plus insert-first claim, the 2 s clean margin, the repository range check plus five named CHECKs).

## Notes for later plans

- Every exported function has no production caller until 13-05 onward; each is exercised by a test of this plan.
- `skipLocal` for `member_requirements:*` arrives with 13-09 (the message watcher), the error-returning approvals reader with 13-07, and the backup exclusion test with 13-14, as the plan says.
- Requirements: this plan completes none. MREQ-01 stays Pending until 13-06, MREQ-04 until 13-12.

## Self-Check: PASSED

- Files exist: migration, `models/member_requirements.go`, `memberreq/settings.go`, `memberreq/holds.go`, the three memberreq test files, `migrations/member_requirements_migration_test.go` (all created by the commits below).
- Commits are ancestors of HEAD: ba68614, d2aa1c4, a266628, 99e9d8d, e8eb600, 7dd6d38.
- `git rev-list --count 6767e5b..HEAD` was 6 before this SUMMARY commit.
