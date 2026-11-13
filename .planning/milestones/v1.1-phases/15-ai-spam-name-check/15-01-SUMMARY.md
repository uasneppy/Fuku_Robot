---
phase: 15-ai-spam-name-check
plan: 01
subsystem: database
tags: [postgres, migration, gorm, member-requirements, check-constraint]

requires:
  - phase: 14-profile-photo-and-channel-rules
    provides: chat_requirements with seven action columns, memberreq.Rules/SetRuleAction, the cached member_requirements key
provides:
  - ai_action column on chat_requirements (NOT NULL DEFAULT 0, named CHECK chk_chat_requirements_ai 0..4)
  - models.ChatRequirements.AIAction (no gorm default tag)
  - memberreq.RuleAI ("ai") in Rules, ruleColumns and actionField
  - TestMemberRequirementAIMigrationDefaults in CI's PostgreSQL integrity list
affects: [15-04, 15-05, 15-08, 15-10, 15-11]

plan_head_before: a155f5068284a29e670d796980bdc1e3fe9fe6c3
plan_head_after: 55e881ab7dffb0ec1cce92bf58bfb25bc12bc817
actuals:
  tokens: 16000
  tasks: 3
  commits: 4

tech-stack:
  added: []
  patterns:
    - "A new rule is a column plus a Rule value appended to Rules, ruleColumns and actionField; SetRuleAction, AnyRuleOn and the readers need no change"
    - "A migration that adds a CHECK uses ADD COLUMN IF NOT EXISTS with the CONSTRAINT inline, never a top-level ADD CONSTRAINT"

key-files:
  created:
    - migrations/20261109120000_add_member_requirement_ai_rule.sql
    - fuku/db/migrations/member_requirement_ai_migration_test.go
  modified:
    - fuku/db/models/member_requirements.go
    - fuku/db/memberreq/settings.go
    - fuku/db/memberreq/settings_test.go
    - fuku/db/backup/memberreq_backup_test.go
    - Makefile
    - AGENTS.md

key-decisions:
  - "Rule ID is ai; the column stays <id>_action (ai_action)"
  - "Migration timestamp 20261109120000 as D-31 fixes it (the last file at HEAD was 20261108120000)"

requirements-completed: []

coverage:
  - id: D1
    description: "ai_action stored one column at a time, read back, an old row reads it off"
    requirement: "MREQ-01, MREQ-14 (contribution only; completes none)"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^TestChatRequirementsSettings$' ./fuku/db/memberreq"
        status: pass
      - kind: integration
        ref: "same test on PostgreSQL 16 against the migration-built schema"
        status: pass
    human_judgment: false
  - id: D2
    description: "the migration's statement, constraint name, default and idempotence"
    verification:
      - kind: unit
        ref: "TestMemberRequirementAIMigrationStatements"
        status: pass
      - kind: integration
        ref: "TestMemberRequirementAIMigrationDefaults on PostgreSQL 16"
        status: pass
    human_judgment: false

duration: 25min
completed: 2026-11-08
status: complete
---

# Phase 15 Plan 01: AI spam-name rule storage Summary

**chat_requirements gets ai_action (SMALLINT NOT NULL DEFAULT 0, named CHECK 0..4 inline) with the matching model field and memberreq.RuleAI, so every existing row and every cached value from the previous build reads the AI rule off, and CI proves the CHECK on PostgreSQL.**

## Accomplishments

- Migration `20261109120000_add_member_requirement_ai_rule.sql`: one `ALTER TABLE chat_requirements ADD COLUMN IF NOT EXISTS ai_action ...` statement with the CHECK inline (`chk_chat_requirements_ai`); no transaction control, no `ADD CONSTRAINT`, no `NOT VALID`. `runner.go` and `scripts/migrate_psql.sh` are unchanged.
- `models.ChatRequirements` gained `AIAction` (no gorm default tag; the code-lines-only grep prints 0). `memberreq` gained `RuleAI`, appended to `Rules`, `ruleColumns` and `actionField`. `SetRuleAction`, `AnyRuleOn`, `GetSettings`, `GetSettingsFresh` and `upsertSettings` needed no change.
- `TestChatRequirementsSettings` now covers eight columns: the actions array is `[8]int`; AI writes (including a written Go 0 and the two switches) touch no other column; five rules written at once all land; 5 and -1 are refused for `ai` before any write; `profile_photo` stays unknown; the CHECKs refuse 5 and -1 on every rule's column; an old row and a previous-build cached value read off; rule order ends `photo, channel, ai`; `AnyRuleOn` is true with only `ai` on.
- `TestMemberRequirementAIMigrationStatements` (database-free, in `make test`) and `TestMemberRequirementAIMigrationDefaults` (PostgreSQL only) pin the statement, the zero default, the constraint name in `pg_constraint`, the second run of the statement and the refusal of 5 with SQLSTATE 23514 naming the constraint.
- Makefile: `TestMemberRequirementAIMigrationDefaults` joins the anchored `-run` list of `test-postgres-integrity`. `TestMemberRequirementsStayOutOfBackup` sets `AIAction`, checks `ai_action` is absent from the export and untouched by a reset. AGENTS.md: the `chat_requirements` Data bullet (eight columns, migration, off for old rows and previous-build cache values), the pass-fingerprint sentence (eight rules, one more harmless miss after the deploy), the join paragraph's "eight-rule table" and the Testing bullet (new guarantee). Statements about the seven-row rule table and the seven page cells stay true until plan 15-04 and were left alone.

## Task Commits

1. `cabaddb` test(15-01): the AI rule has its own column (RED: does not compile until the enum and field exist)
2. `27829ca` feat(15-01): store the AI spam-name rule punishment (GREEN)
3. `2c3b031` test(15-01): pin the AI rule migration (characterization test, passes on first run, as the plan says)
4. `55e881a` docs(15-01): list the AI rule column and its PostgreSQL guarantee

## Verification (real results)

- `go test -tags testtools -race -count=1 -run '^TestChatRequirementsSettings$' -v ./fuku/db/memberreq`: all 8 subtests PASS on SQLite (exit 0).
- `go test -tags testtools -race -count=1 ./fuku/db/memberreq ./fuku/db/models ./fuku/db/backup`: ok, ok, ok.
- `go test -tags testtools -count=1 -run '^(TestMemberRequirementAIMigrationStatements|TestMemberRequirementLookupMigrationStatements|TestMemberRequirementsMigrationStatements|TestRulesMediaMigrationStatements)$' ./fuku/db/migrations`: ok. Without PostgreSQL `TestMemberRequirementAIMigrationDefaults` prints SKIP, as expected.
- `go test -tags testtools -race -count=1 -run '^TestMemberRequirementsStayOutOfBackup$' ./fuku/db/backup`: ok (exit 0).
- `go test -tags testtools -race -count=1 -run '^(TestMemberReqRuleTable|TestMemberReqPassValue|TestMemberReqStrictest)$' -v ./fuku/modules`: all three PASS on SQLite.
- `CGO_ENABLED=0 go build ./...`: exit 0. `go vet -tags testtools` on memberreq, migrations, models, backup: exit 0. `gofmt -l` on the files I touched: nothing (pre-existing unformatted files elsewhere in `fuku/db` are not mine).
- `git diff --exit-code 8c0c827 -- go.mod go.sum`: exit 0. `git diff --exit-code 8c0c827 -- fuku/db/migrations/runner.go scripts/migrate_psql.sh`: exit 0. `make -n test-postgres-integrity | grep -c TestMemberRequirementAIMigrationDefaults` prints 1.
- Lint stand-in `python3 .planning/phases/13-member-requirements/handoff/lint-local.py 8c0c827`: exit 0 read directly ("no dupl, gocyclo or godox findings on lines added since 8c0c827"). `make lint` and gosec cannot run in this sandbox.
- PostgreSQL 16 proof (throwaway cluster `/var/lib/postgresql/fuku-ai-pg`, trust auth, 127.0.0.1:55435, database `fuku_test`): `FUKU_TEST_MIGRATION_CHAIN=true ... -run '^TestRepositoryMigrationChain$' ./fuku/db/migrations` ok (exit 0); `information_schema` then showed `ai_action`, default 0, NOT NULL, and `chk_chat_requirements_ai` in `pg_constraint`. Then `FUKU_TEST_DATABASE=true ... -run '^(TestChatRequirementsSettings|TestMemberRequirementAIMigrationDefaults)$' -v ./fuku/db/memberreq ./fuku/db/migrations`: all 8 subtests (the old-row subtest ran its chat_id-only INSERT branch) and the Defaults test PASS, no SKIP (exit 0). The cluster was stopped with `pg_ctl -m fast stop` (exit 0) and the data directory deleted (`test ! -e /var/lib/postgresql/fuku-ai-pg` holds). No DSN with credentials was written to any file.
- Not run: `make lint`, gosec, `make test` (this plan does not require it), the settings test family (this plan does not touch the menu).

## Deviations from Plan

None - plan executed as written. One slip, fixed in the same task before any commit: my first scripted edit of `settings.go` missed the `ruleColumns` entry (tab alignment), the tests caught it as `ErrUnknownRule`, and I added it before the GREEN commit.

## Requirements

`requirements-completed: []`. MREQ-01 and MREQ-14 get their storage here but stay Pending, as do MREQ-10 (not touched) and the rest; they complete in plan 15-11. REQUIREMENTS.md is unchanged.

## Known Stubs

None.

## Threat Flags

None. No new endpoint, auth path or file access; one schema change at a trust boundary already in the threat model (T-15-01..03 mitigated as planned, T-15-SC: go.mod and go.sum unchanged).
