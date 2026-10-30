---
phase: 08-turnstile-web-captcha
plan: 04
subsystem: captcha-storage
tags: [migration, postgresql, check-constraint, captcha, turnstile, go]
status: complete

requires:
  - phase: 08-turnstile-web-captcha
    provides: "plan 08-01: the SQLite model tag that already lists turnstile in chk_captcha_mode"
provides:
  - "migrations/20261030120000_allow_turnstile_captcha_mode.sql: PostgreSQL accepts captcha_mode 'turnstile', exactly one captcha_mode CHECK left"
  - "captcha.SetCaptchaMode accepts turnstile and invalidates the cached settings"
  - "TestCaptchaModeMigrationStatements (make test) and TestCaptchaModeTurnstileConstraint (make test-postgres-integrity, CI only)"
affects: [08-05, 08-06, 08-07, 08-08, 08-09, 08-10]

requirements-completed: [CAPT-01]

actuals:
  tokens: 14000
  tasks: 2
  commits: 4
plan_head_before: f9560b78b4854f75cca428dbf59882850f236056
plan_head_after: 7cef49a7777aa5f7eb87981572607eace02207f8

tech-stack:
  added: []
  patterns:
    - "Drop-before-add migration for a CHECK: DROP IF EXISTS the named check, a pg_constraint DO block for every other check on the column, then ADD ... NOT VALID, because both appliers swallow duplicate_object on a top-level ADD CONSTRAINT"
    - "Database-free statement test: run the real file through cleanSupabaseSQL and splitSQLStatements and assert the exact statement sequence the runner executes"

key-files:
  created:
    - migrations/20261030120000_allow_turnstile_captcha_mode.sql
    - alita/db/migrations/captcha_mode_test.go
  modified:
    - alita/db/captcha/repository.go
    - alita/db/captcha/repository_test.go
    - alita/db/constraints_test.go
    - Makefile
    - AGENTS.md

key-decisions:
  - "The migration drops the legacy inline check (captcha_settings_captcha_mode_check of 20250807103246) through a pg_constraint DO block, not by a hard-coded name, so a database where it was renamed or already removed still ends with exactly one captcha_mode check"
  - "The header comment avoids the phrases the appliers' regexes rewrite (alter table ... add constraint, create table/index), because cleanSupabaseSQL and the shell perl run over comments too"
  - "SQLSTATE 23514 is matched on the error text instead of importing pgconn, which would turn the indirect pgx/v5 requirement into a direct one and touch go.mod"

duration: 10 min
completed: 2026-10-30
---

# Phase 8 Plan 04: Turnstile captcha mode in storage Summary

A group's Turnstile choice can now be stored on PostgreSQL and SQLite: an append-only migration widens `chk_captcha_mode` to math, text and turnstile after removing both existing copies of the old check, and `SetCaptchaMode` accepts the new value.

## What was built

- **Migration `20261030120000_allow_turnstile_captcha_mode.sql`** (D-05, D-29), three statements in this order: `DROP CONSTRAINT IF EXISTS chk_captcha_mode`; a `DO $$` block that drops every other CHECK on `captcha_settings` whose definition mentions `captcha_mode` (the unnamed inline check of 20250807103246, which PostgreSQL named `captcha_settings_captcha_mode_check`); then `ADD CONSTRAINT chk_captcha_mode CHECK (captcha_mode IN ('math', 'text', 'turnstile')) NOT VALID`. No transaction control. Neither `runner.go` nor `scripts/migrate_psql.sh` changed.
- **`TestCaptchaModeMigrationStatements`** (runs in `make test`, no database): the file through `cleanSupabaseSQL` and `splitSQLStatements` gives exactly three non-transaction statements in order; the DO block is not wrapped by the duplicate_object rewrite and the ADD is.
- **`TestCaptchaModeTurnstileConstraint`** (PostgreSQL only, skips without `ALITA_TEST_DATABASE=true` and `DATABASE_URL`): in one rolled-back transaction, exactly one CHECK mentioning captcha_mode named `chk_captcha_mode`, the migration statements run twice, `turnstile`/`math`/`text` insert, `bogus` fails with SQLSTATE 23514 behind a savepoint. Added to the `test-postgres-integrity` `-run` alternation in the Makefile (the package was already listed).
- **`captcha.SetCaptchaMode`** accepts `turnstile`; the `cache.DeleteCache` of `captcha_settings` after the write is unchanged. `TestSetCaptchaModeAcceptsTurnstile` primes the cache with the default, stores turnstile, reads it straight back, checks that `bogus` returns `ErrInvalidCaptchaMode` and leaves turnstile stored, and that math and text still work. `TestCaptchaSettingsConstraint_ValidModes` now also creates a turnstile row.
- **AGENTS.md**: a Data bullet for the widened check and why the drops come first, and the Testing paragraph for `make test-postgres-integrity` lists the Phase 8 guarantee.

## Verification

| Check | Result |
|-------|--------|
| `go test -tags testtools -count=1 -run '^(TestCaptchaModeMigrationStatements\|TestCaptchaModeTurnstileConstraint)$' -v ./alita/db/migrations` | PASS for the statements test; the PostgreSQL test SKIPs locally, as required |
| `go test -tags testtools -race -count=1 -run '^TestSetCaptchaModeAcceptsTurnstile$' -v ./alita/db/captcha` | PASS |
| `go test -tags testtools -race -count=1 -run '^TestCaptchaSettingsConstraint_ValidModes$' -v ./alita/db` | PASS |
| `go test -tags testtools -race -count=1 ./alita/db/captcha ./alita/db/migrations` | ok |
| `CGO_ENABLED=0 go build ./...`, `go vet -tags testtools ./alita/db/captcha ./alita/db/migrations ./alita/db`, `gofmt -l alita/db` | clean |
| `make check-docs` | no drift |
| Task 1 and Task 2 acceptance greps | all pass |

`make lint` was not run: golangci-lint in this sandbox is built with go1.25 against the module's go1.26.0, so `gofmt -l` and `go vet -tags testtools` stood in for it. `make test` was not run (the orchestrator runs it between waves) and PostgreSQL was never started.

## TDD gate compliance

RED then GREEN for both tasks: `4c76b34` (test, fails on the missing migration file) then `b700c1a` (feat); `f8f72c1` (test, `SetCaptchaMode(turnstile)` returned `INVALID_CAPTCHA_MODE`) then `7cef49a` (feat). `TestCaptchaSettingsConstraint_ValidModes` already passed in the RED commit because plan 08-01 put turnstile in the model tag; the new row only extends its coverage.

## Deviations from Plan

None - plan executed exactly as written. The plan's commit-trailer text names a different model line than the harness attribution; the harness's trailer was used, per the dispatch notes.

## Pending human verification

- **CI only:** `make test-postgres-integrity` must run `TestCaptchaModeTurnstileConstraint` against a database the migration chain already ran on. It proves the real `pg_constraint` result (one captcha_mode check, no legacy inline check) that SQLite cannot. It was deliberately not run here.

## Known Stubs

None.

## Threat Flags

None. No new endpoint, auth path or file access; the trust-boundary changes are the ones the plan's threat model covers (T-08-15 to T-08-17), all mitigated as planned.

## Self-Check: PASSED

- FOUND: migrations/20261030120000_allow_turnstile_captcha_mode.sql, alita/db/migrations/captcha_mode_test.go
- FOUND commits: 4c76b34, b700c1a, f8f72c1, 7cef49a
