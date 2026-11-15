---
phase: 12-rules-media-media-under-text-and-greet-once
plan: 02
subsystem: database
tags: [migration, models, backup, greet-once, rules-media, media-below]
requires:
  - phase: 12-rules-media-media-under-text-and-greet-once
    provides: 12-01 goldens that pin what old rows send today (they stay green with the new columns present)
provides:
  - migration 20261105120000_add_rules_media_and_greet_once.sql (seven NOT NULL DEFAULT columns and the greeted_members table)
  - WelcomeSettings.MediaBelow and Once, GoodbyeSettings.MediaBelow, RulesSettings.FileID, MediaType, Buttons and MediaBelow, with no gorm default tag on any boolean
  - RulesSettings.HasContent, the one test of "Rules is set"
  - models.GreetedMember (TableName greeted_members) in the three harness AutoMigrate lists
  - TestRulesMediaMigrationStatements, TestRulesMediaMigrationDefaults (PostgreSQL only), TestHasContent, TestOldGreetingsExportDecodesToZeros, TestGreetedMembersStayOutOfBackup, and the extended TestAllModulesRoundTripEveryMeaningfulField
affects: [12-03, 12-04, 12-07, 12-09, 12-10, 12-14, rules, greetings, backup]

actuals:
  tokens: 8400
  tasks: 3
  commits: 4

commits: 4
plan_head_before: e3868491de555141b8dc98e360e65c999d5151b6
plan_head_after: a776523d081fc8278aef1f4bab0ea1103251c9ba

tech-stack:
  added: []
  patterns:
    - "A new nullable-on-SQLite, NOT NULL DEFAULT-on-PostgreSQL column set: the models carry no default tag, the migration carries the zero-value default"
    - "Old-export proof: strip the new keys from a real export's JSON, import over a chat that had them all set, read zeros back"

key-files:
  created:
    - migrations/20261105120000_add_rules_media_and_greet_once.sql
    - fuku/db/models/greeted_member.go
    - fuku/db/models/rules_test.go
    - fuku/db/migrations/rules_media_migration_test.go
    - fuku/db/backup/greeted_backup_test.go
  modified:
    - fuku/db/models/greetings.go
    - fuku/db/models/rules.go
    - fuku/db/backup/roundtrip_test.go
    - fuku/db/backup/testmain_test.go
    - fuku/db/greetings/testmain_test.go
    - fuku/modules/test_harness_test.go
    - AGENTS.md

key-decisions:
  - "The migration name is the plan's (20261105120000): ls migrations showed 20261104120000 as the newest, so no later timestamp was needed"
  - "fuku/db/migrations/runner.go and scripts/migrate_psql.sh are untouched: the runner reads every *.sql generically and the script loops over migrations/*.sql, confirmed by reading both and by git diff against bd2f794 and 72a692b (exit 0)"
  - "TestOldGreetingsExportDecodesToZeros removes the new keys per object (welcome_settings: media_below, once; goodbye_settings: media_below; rules settings: file_id, media_type, btns, media_below) and requires each key to be present first, so the strip cannot silently do nothing"
  - "TestRulesMediaMigrationDefaults reuses captchaModeChatID from the sibling test file for fresh chat IDs instead of duplicating it"

patterns-established:
  - "HasContent is the single definition of Rules set; later plans must call it instead of testing the text alone"

requirements-completed: []

coverage:
  - id: D1
    description: "The migration adds the seven columns and greeted_members with idempotent statements, no transaction control, no ADD CONSTRAINT, no CONCURRENTLY"
    requirement: "EDIT-07"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^TestRulesMediaMigrationStatements$' ./fuku/db/migrations"
        status: pass
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^TestRulesMediaMigrationDefaults$' ./fuku/db/migrations (PostgreSQL only, skips here, runs in CI)"
        status: not-run
    human_judgment: false
  - id: D2
    description: "New greetings and rules fields travel in backup JSON; an old export imports as zeros; greeted_members stays out of export, import and reset"
    requirement: "EDIT-08"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^(TestOldGreetingsExportDecodesToZeros|TestGreetedMembersStayOutOfBackup|TestAllModulesRoundTripEveryMeaningfulField)$' ./fuku/db/backup"
        status: pass
    human_judgment: false
  - id: D3
    description: "RulesSettings.HasContent is true for a non-blank text, a file ID or a button, so a legacy text-only row reads as today"
    requirement: "EDIT-08"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^TestHasContent$' ./fuku/db/models"
        status: pass
    human_judgment: false

duration: 25min
completed: 2026-11-05
status: complete
---

# Phase 12 Plan 02: Rules media, Media under text and Greet once storage Summary

**One idempotent migration, the model fields, `RulesSettings.HasContent` and `GreetedMember`, so the database holds rules media and buttons, the three Media under text and Greet once switches and the greet-once record, while every row saved before reads and exports exactly as before.**

## Performance

- **Duration:** about 25 min
- **Tasks:** 3 (a tracer, a TDD task and an auto task)
- **Commits:** 4 task commits plus the docs commit
- **Files:** 5 created, 7 modified

## Accomplishments

- `migrations/20261105120000_add_rules_media_and_greet_once.sql`: `greetings.welcome_media_below`, `goodbye_media_below`, `welcome_once` (BOOLEAN NOT NULL DEFAULT FALSE); `rules.file_id` (TEXT ''), `media_type` (BIGINT 0), `btns` (JSONB `'[]'::jsonb`), `media_below` (BOOLEAN FALSE); `CREATE TABLE IF NOT EXISTS greeted_members` with `PRIMARY KEY (chat_id, user_id)` and `created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()`. Eight statements, all idempotent, no transaction control.
- Models: the new fields with no `default:` tag on any boolean, a comment at each site giving the reason (copied from `chat_settings_access.go`), `models.GreetedMember` in `greeted_member.go`.
- `RulesSettings.HasContent()`: `strings.TrimSpace(Rules) != "" || FileID != "" || len(Buttons) > 0`.
- `GreetedMember` added to the AutoMigrate lists of `fuku/modules/test_harness_test.go`, `fuku/db/greetings/testmain_test.go` and `fuku/db/backup/testmain_test.go`.
- AGENTS.md: two Data bullets (the `greeted_members` table, the new columns with `HasContent`) and the harness sentence in Testing.

## TDD record (what was red and what was green)

A migration cannot be driven by a Go failing test the usual way, so the red and green are per piece:

- **Task 1 (tracer, migration and fields):** the tests were written together with the migration and the fields in one commit (`4553cb1`). There was no separate red commit for it: the statement-shape test fails with "read ... no such file" when the SQL file is absent and the extended round-trip test does not compile without the fields. Both passed on their first run once the code existed. Stated plainly: this task was not run red-then-green as separate commits.
- **Task 2, `HasContent` (real red, commit `014c707`):** `TestHasContent` first failed to build (`tc.row.HasContent undefined`, `[build failed]`, an INVALID_RED by the reference's rules). A compile-only stub `HasContent() bool { return false }` went into the red commit, after which the target test executed and failed on its assertions: `text`, `only_a_file`, `only_a_button` and `legacy_row_with_a_button_label` FAIL, the five negative cases PASS. Semantic assessment: the target test ran and failed on the planned assertion for the intended reason (the stub never answers true), not on setup, import or an unrelated test. `gsd_run check tdd-red-evidence` was not run (the evidence is a console report in this session, no record file was written); the semantic check above was done by reading the `--- FAIL` lines.
- **Task 2 green (commit `1a9ca51`):** the real body replaced the stub and `TestHasContent` passes (nine subtests).
- **Task 2 backup tests:** `TestOldGreetingsExportDecodesToZeros` and `TestGreetedMembersStayOutOfBackup` change no production code (they prove D-13 as built), so they passed on their first run and went into the red commit green. They are not red-first by nature. The first would fail if an import merged instead of replacing (the destination chat starts with every new field set) and the strip helper fails if the new export lacks a key it removes.
- **Task 3:** `TestRulesMediaMigrationDefaults` was written after the migration and cannot execute here (PostgreSQL is not available): it compiles, passes `go vet`, and SKIPs locally exactly as `TestCaptchaModeTurnstileConstraint` does. It runs in CI once the phase gate plan (12-16) adds it to `make test-postgres-integrity` (which also needs `./fuku/db/migrations` in its package list, already there for the captcha test). Its SQL (bare `INSERT INTO greetings (chat_id)` and `rules (chat_id)` after a `chats` row, the second run of every statement, `ON CONFLICT DO NOTHING` twice, a plain second insert expecting SQLSTATE 23505 behind a savepoint) has never executed against PostgreSQL; I checked the required columns of both tables in the initial migration (only `chat_id`, plus the `fk_greetings_chat` and `fk_rules_chat` keys to `chats`, which is why it inserts a chat row first).

## Task Commits

1. **Task 1 (tracer): migration, models, harness lists, round trip** - `4553cb1` (feat)
2. **Task 2 red: HasContent test with a stub, backup proofs** - `014c707` (test)
3. **Task 2 green: HasContent** - `1a9ca51` (feat)
4. **Task 3: PostgreSQL defaults test and AGENTS.md** - `a776523` (test)

## Verification (real results)

- `go test -tags testtools -race -count=1 -run '^TestRulesMediaMigrationStatements$' -v ./fuku/db/migrations`: PASS (`ok`).
- `go test -tags testtools -race -count=1 -run '^(TestRulesMediaMigrationStatements|TestRulesMediaMigrationDefaults)$' -v ./fuku/db/migrations`: Statements PASS, Defaults SKIP ("set FUKU_TEST_DATABASE=true and DATABASE_URL ... on PostgreSQL"), `ok`.
- `go test -tags testtools -race -count=1 -run '^TestAllModulesRoundTripEveryMeaningfulField$' -v ./fuku/db/backup`: PASS.
- `go test -tags testtools -race -count=1 -run '^TestHasContent$' -v ./fuku/db/models`: PASS.
- `go test -tags testtools -race -count=1 -run '^(TestOldGreetingsExportDecodesToZeros|TestGreetedMembersStayOutOfBackup|TestAllModulesRoundTripEveryMeaningfulField)$' -v ./fuku/db/backup`: all three PASS.
- `go test -tags testtools -race -count=1 ./fuku/db/migrations ./fuku/db/models ./fuku/db/backup ./fuku/db/greetings`: `ok` for all four packages.
- `go test -tags testtools -race -count=1 -run '^(TestRulesLegacyRowSendsTodaysRequest|TestLegacyGreetingsRequestsUnchanged|TestSettingsEditReadsCreateNothing)$' -v ./fuku/modules`: all three PASS (the 12-01 goldens stay green with the new columns present).
- `CGO_ENABLED=0 go build ./...`: exit 0. `go vet -tags testtools` on the four db packages: clean. `gofmt -l` on every file I touched: clean (it does list unrelated files such as `fuku/db/admin/repository.go`, untouched and pre-existing).
- `python3 .../lint-local.py 72a692b`: exit 0, "no dupl, gocyclo or godox findings", run before the last code commit.
- `git diff --exit-code 72a692b -- go.mod go.sum`: exit 0. `git diff --quiet bd2f794 -- fuku/db/migrations/runner.go scripts/migrate_psql.sh`: exit 0 (also against 72a692b).
- Acceptance greps: `grep -c 'greeted_members' AGENTS.md` prints 1 or more (baseline 0), `welcome_media_below` and `HasContent` each at least 1; no `default:` on a non-comment line of `greeted_member.go`; `grep greeted fuku/db/backup/types.go` finds nothing.
- Not run: `make lint` and `make test` (cannot run in this sandbox, the plan does not ask for them), the settings test family (the plan does not ask), `TestRulesMediaMigrationDefaults` on PostgreSQL (CI only).

## Deviations from Plan

- **[Rule 3 - process] A compile-only stub in the red commit.** `HasContent` is a method the test calls, so a red commit without it is a build failure, which the TDD reference classes as invalid RED. The red commit therefore holds `rules.go` with a stub returning false, replaced by the real body in the next commit. No behaviour ships in the stub (nothing calls `HasContent` yet).
- **Round-trip literals use the existing one-button `buttons` fixture** (shared by the other modules in that test) rather than two buttons; the assertions compare the stored array to it for the welcome, goodbye and rules rows.
- `TestOldGreetingsExportDecodesToZeros` strips the keys object by object rather than from "the greetings and rules objects" at large, so a `file_id` or `btns` that belongs to the old welcome or goodbye stays in the export (those keys existed before this phase).

## Known Stubs

None. The red-commit stub of `HasContent` was replaced in the following commit.

## Threat Flags

None beyond the plan's register. Mitigations in place: T-12-03 (append-only idempotent migration, statement test), T-12-04 (greeted_members outside `AllExportableModules`, `TestGreetedMembersStayOutOfBackup`), T-12-05 (no default tag on any new boolean), T-12-06 (NOT NULL DEFAULT zero value, `TestOldGreetingsExportDecodesToZeros`), T-12-SC (go.mod and go.sum unchanged).

## Requirement coverage

`requirements-completed: []`. The plan contributes storage to EDIT-07 (the three Media under text columns), EDIT-08 (rules media and buttons columns, `HasContent`) and EDIT-09 (`welcome_once`, `greeted_members`) and completes none; all three stay Pending in `.planning/REQUIREMENTS.md` (12-10 and 12-14 are the completing plans).

## Notes for later plans

- `TestRulesMediaMigrationDefaults` must be added to the `-run` list of `make test-postgres-integrity` by 12-16 (with `./fuku/db/greetings` in the package list for the claim test of 12-09).
- Importing a backup replaces a chat's switches with the export's values (replace semantics, an old export therefore turns Media under text and Greet once off): accepted in the plan.
- The SQLite test schema leaves the new columns nullable with no default, so the greetings upsert (which writes a map of the old columns) keeps working; a NULL reads as the zero value.
- The AGENTS.md bullet for `greeted_members` says the claim rule arrives with plan 12-09; that plan should extend the bullet in place.

## Self-Check: PASSED

- All five created files exist (migration, `greeted_member.go`, `rules_test.go`, `rules_media_migration_test.go`, `greeted_backup_test.go`).
- Commits `4553cb1`, `014c707`, `1a9ca51` and `a776523` are ancestors of HEAD.
