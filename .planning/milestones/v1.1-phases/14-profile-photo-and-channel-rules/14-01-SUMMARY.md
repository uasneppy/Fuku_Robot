---
phase: 14-profile-photo-and-channel-rules
plan: 01
subsystem: database
tags: [postgres, migration, gorm, member-requirements, check-constraint]

requires:
  - phase: 13-member-requirements
    provides: chat_requirements table, memberreq.Rules/SetRuleAction, the cached member_requirements key
provides:
  - photo_action and channel_action columns on chat_requirements (NOT NULL DEFAULT 0, named CHECK 0..4)
  - models.ChatRequirements.PhotoAction and ChannelAction (no gorm default tag)
  - memberreq.RulePhoto ("photo") and memberreq.RuleChannel ("channel") in Rules, ruleColumns and actionField
  - TestMemberRequirementLookupMigrationDefaults in CI's PostgreSQL integrity list
affects: [14-05, 14-06, 14-08, 14-10, 14-12]

plan_head_before: cc19bbab4abb7b73fbbce942da9e5a141a79208d
plan_head_after: 555bad3
actuals:
  tokens: 14000
  tasks: 3
  commits: 6

tech-stack:
  added: []
  patterns:
    - "A new rule is a column plus a Rule value appended to Rules, ruleColumns and actionField; SetRuleAction, AnyRuleOn and the readers need no change"
    - "A migration that adds a CHECK uses ADD COLUMN IF NOT EXISTS with the CONSTRAINT inline, never a top-level ADD CONSTRAINT"

key-files:
  created:
    - migrations/20261108120000_add_member_requirement_lookup_rules.sql
    - fuku/db/migrations/member_requirement_lookup_migration_test.go
  modified:
    - fuku/db/models/member_requirements.go
    - fuku/db/memberreq/settings.go
    - fuku/db/memberreq/settings_test.go
    - Makefile
    - AGENTS.md

key-decisions:
  - "Rule IDs are photo and channel; the column of a rule stays <id>_action"
  - "Migration timestamp 20261108120000 (the last file at HEAD was 20261106120000)"
  - "On SQLite the old-row subtest checks the previous build's cached value (msgpack decode) instead of a chat_id-only INSERT, because the SQLite harness builds the table from the model with no column defaults; the raw INSERT runs on PostgreSQL only"

requirements-completed: []

coverage:
  - id: D1
    description: "photo_action and channel_action stored one column at a time, read back, an old row reads both off"
    requirement: "MREQ-11, MREQ-12 (contribution only)"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^TestChatRequirementsSettings$' ./fuku/db/memberreq"
        status: pass
      - kind: integration
        ref: "same test on PostgreSQL 16 against the migration-built schema"
        status: pass
    human_judgment: false
  - id: D2
    description: "the migration's statements, constraint names, defaults and idempotence"
    verification:
      - kind: unit
        ref: "TestMemberRequirementLookupMigrationStatements"
        status: pass
      - kind: integration
        ref: "TestMemberRequirementLookupMigrationDefaults on PostgreSQL 16"
        status: pass
    human_judgment: false

duration: 40min
completed: 2026-11-08
status: complete
---

# Phase 14 Plan 01: Photo and channel rule storage Summary

**chat_requirements gets photo_action and channel_action (SMALLINT NOT NULL DEFAULT 0, named CHECK 0..4 inline) with the matching model fields and memberreq rules, so every existing row and every cached value from the previous build reads both rules off, and CI proves the CHECKs on PostgreSQL.**

## Performance

- **Duration:** about 40 min
- **Tasks:** 3 (tracer, auto, auto)
- **Files modified:** 7

## Accomplishments

- Migration `20261108120000_add_member_requirement_lookup_rules.sql`: two `ALTER TABLE chat_requirements ADD COLUMN IF NOT EXISTS` statements with the CHECK inline (`chk_chat_requirements_photo`, `chk_chat_requirements_channel`); no transaction control, no `ADD CONSTRAINT`, no `NOT VALID`. `runner.go` and `scripts/migrate_psql.sh` are unchanged.
- `models.ChatRequirements` gained `PhotoAction` and `ChannelAction` (no gorm default tag; the code-lines-only grep for a default tag prints 0). `memberreq` gained `RulePhoto` and `RuleChannel`, appended to `Rules`, `ruleColumns` and `actionField`. `SetRuleAction`, `AnyRuleOn`, `GetSettings`, `GetSettingsFresh` and `upsertSettings` needed no change.
- `TestChatRequirementsSettings` now covers seven columns: the actions array is `[7]int`, photo and channel writes (including a written Go 0) touch no other column, four rules written at once all land, 5 and -1 are refused for the new rules before any write, `profile_photo` stays an unknown rule, the CHECKs refuse 5 and -1 on every rule's column, an old row reads off, and the rule order is last_name, username, russian, arabic, chinese, photo, channel.
- `TestMemberRequirementLookupMigrationStatements` (database-free, in `make test`) and `TestMemberRequirementLookupMigrationDefaults` (PostgreSQL only) pin the statements, the defaults, the two constraint names in `pg_constraint`, the second run of the statements and the refusal of 5 with SQLSTATE 23514 naming the constraint.
- Makefile: `TestMemberRequirementLookupMigrationDefaults` joins the anchored `-run` list of `test-postgres-integrity`. AGENTS.md: the `chat_requirements` Data bullet (seven columns, migration, off for old rows and previous-build cache values), the pass-fingerprint sentence (covers seven rules, one harmless miss after the deploy) and the Testing bullet (new guarantee).

## Task Commits

1. `10fff4b` test(14-01): the photo and channel rules have their own columns (RED: does not compile until the enum and fields exist)
2. `933b808` feat(14-01): store the photo and channel rule punishments (GREEN)
3. `37ae2fd` test(14-01): pin the lookup rules migration
4. `6303834` test(14-01): judge the migration's SQL, not its header comment (fix of 3)
5. `8cd297c` docs(14-01): list the lookup rule columns and their PostgreSQL guarantee
6. `555bad3` test(14-01): drop the embedded Dialector selector (lint stand-in QF finding)

## Verification (real results)

- `go test -tags testtools -race -count=1 -run '^TestChatRequirementsSettings$' -v ./fuku/db/memberreq`: all 8 subtests PASS on SQLite.
- `go test -tags testtools -race -count=1 ./fuku/db/memberreq ./fuku/db/models`: ok.
- `go test -tags testtools -count=1 -run '^(TestMemberRequirementLookupMigrationStatements|TestMemberRequirementsMigrationStatements|TestRulesMediaMigrationStatements|TestSettingsHubMigrationStatements)$' ./fuku/db/migrations`: ok. Without PostgreSQL `TestMemberRequirementLookupMigrationDefaults` prints SKIP, as expected.
- `go test -tags testtools -race -count=1 -run '^(TestMemberReqRuleTable|TestMemberReqPassValue|TestMemberReqStrictest)$' ./fuku/modules`: ok (the pattern matched tests of those names in this tree, run on SQLite).
- `CGO_ENABLED=0 go build ./...`: ok. `git diff --exit-code 8c0c827 -- go.mod go.sum`: clean. `git diff --exit-code 8c0c827 -- fuku/db/migrations/runner.go scripts/migrate_psql.sh`: clean. `make -n test-postgres-integrity | grep -c TestMemberRequirementLookupMigrationDefaults` prints 1.
- `gofmt -l` on the touched packages: nothing from my files (pre-existing unformatted files elsewhere in `fuku/db` are not mine). `go vet -tags testtools` on `memberreq`, `models`, `migrations`: clean.
- Lint stand-in `python3 .planning/phases/13-member-requirements/handoff/lint-local.py 8c0c827`: first run exit 1 on one QF finding (`db.DB.Dialector.Name()`), fixed in `555bad3`; re-run exit 0 read directly ("no dupl, gocyclo or godox findings on lines added since 8c0c827"). `make lint` and gosec cannot run in this sandbox.
- PostgreSQL 16 proof (throwaway cluster `/var/lib/postgresql/fuku-lookup-pg`, trust auth, 127.0.0.1:55434, database `fuku_test`): `TestRepositoryMigrationChain` ok (the chain applied the new migration: `schema_migrations` lists it last, both columns exist with default 0); then `FUKU_TEST_DATABASE=true ... -run '^(TestChatRequirementsSettings|TestMemberRequirementLookupMigrationDefaults)$'` over `./fuku/db/memberreq ./fuku/db/migrations`: all subtests and the Defaults test PASS, no SKIP. The cluster was stopped with `pg_ctl stop` and the data directory deleted (`test ! -e` holds, nothing listens on 55434). No DSN with credentials was written to any file.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] "an old row" subtest cannot insert a chat_id-only row on SQLite**
- **Found during:** Task 1 GREEN
- **Issue:** the SQLite harness creates `chat_requirements` from the model with no column defaults, so `INSERT ... (chat_id)` fails NOT NULL on the first action column.
- **Fix:** the raw chat_id-only INSERT and read-back run when `db.DB.Name()` is `postgres` (CI and the local PostgreSQL proof); on every dialect the subtest also decodes a msgpack value written by a struct without the two fields, which is the previous-build cached value (T-14-03). The PostgreSQL-only default is additionally proved by the migration test.
- **Files modified:** fuku/db/memberreq/settings_test.go
- **Commit:** 933b808, 555bad3

**2. [Rule 1 - Bug] Statement test tripped on its own header comment**
- **Found during:** Task 2 (committed with the failing test by a sequencing slip: I ran the test and the commit in one command line)
- **Issue:** the runner leaves the file's header comment in front of statement 1, and the comment names `ADD CONSTRAINT IF NOT EXISTS`, so the banned-word check fired.
- **Fix:** a `stripSQLComments` helper in the test drops `--` lines before the checks. Follow-up commit 6303834 (history not rewritten); the file was red only between 37ae2fd and 6303834.
- **Files modified:** fuku/db/migrations/member_requirement_lookup_migration_test.go

**Task 2 is a characterization test**, written against the migration of Task 1, so it passes on first run (apart from deviation 2) rather than red-first, as the plan says.

## Known Stubs

None.

## Threat Flags

None. No new endpoint, auth path or file access; one schema change at a trust boundary already in the threat model (T-14-01..03 mitigated as planned).

## Requirements

None completed. `requirements-completed: []`. MREQ-11 (completes in 14-08) and MREQ-12 (completes in 14-10) stay Pending; MREQ-01 stays Pending.

## Issues Encountered

None open. New concerns (named, not fixed): none. `memberReqPassValue` now hashes seven rule actions, which changes every member's pass fingerprint once after the deploy (a harmless pass-cache miss, AGENTS.md says so).

## Self-Check: PASSED

- FOUND: migrations/20261108120000_add_member_requirement_lookup_rules.sql, fuku/db/migrations/member_requirement_lookup_migration_test.go
- FOUND commits 10fff4b, 933b808, 37ae2fd, 6303834, 8cd297c, 555bad3 (ancestors of HEAD)
