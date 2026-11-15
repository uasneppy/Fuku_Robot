---
phase: 12-rules-media-media-under-text-and-greet-once
fixed_at: 2026-11-05
review_path: .planning/phases/12-rules-media-media-under-text-and-greet-once/12-REVIEW.md
iteration: 1
findings_in_scope: 3
fixed: 3
skipped: 0
status: all_fixed
---

# Phase 12: Code Review Fix Report

Scope was WR-01, WR-02 and IN-01. IN-02 and IN-03 were deferred on purpose and the security audit's NEW-03 is accepted, so none of them is counted here.

| Finding | Action | Commit |
|---------|--------|--------|
| WR-01 | Code fix, test first. `greetings.LoadGreetingSettings` is the old read with its error returned (fallback defaults come back next to it). `checkGreetingSettings` is now a thin wrapper that logs the error once and keeps the fallback, so `GetGreetingSettings`, the buttons readers and every other caller behave as before, and a successful read and a missing row (including the row creation for a known chat) are unchanged. `SendWelcomeMessage` reads through the new function and on any error logs the chat ID and the error, sends nothing and claims nothing. The goodbye path is untouched. AGENTS.md (greet-once bullet) says so. | RED e92a260, GREEN 25cde9f |
| IN-01 | Code fix, tests first. `SendWelcomeMessage` tracks the won claim and `sendAttempted` (set just before `media.SendGreetingMessage`). Its recover releases the claim through `greetOnceStore.Release` (`releaseUnsentWelcome`, chat and user IDs and the error only) when a claim was won and no send was attempted. A panic after the send was attempted keeps the claim, and the existing give-back after a send error is untouched and runs on the normal return path, so the claim is never released twice. New seam `welcomeRender` (a variable only so a test can make the render step panic). AGENTS.md (greet-once bullet) says so. | RED f993551, GREEN 3ffb87c |
| WR-02 | Code fix, test first, proved on PostgreSQL 16. `importGreetings` fills a missing `WelcomeSettings` or `GoodbyeSettings` with an empty block before `replaceChatSetting`, so no NULL meets the three `NOT NULL` switch columns; an empty block writes the zero values the old NULLs read as. The rules import has no hole (its model is flat and `ButtonArray` writes `[]`), which the new test also proves. `TestImportGreetingsOmittedBlocks` joins the anchored `-run` list of `make test-postgres-integrity` (`./fuku/db/backup` was already in its package list) and AGENTS.md's Testing paragraph names it. | RED d614491, GREEN 230f82c |

## Deviations from the brief

- **WR-01 RED does not cover the db-level function.** A test of a function that does not exist yet cannot compile, so the RED commit (e92a260) holds the module test that fails on an assertion (two welcomes sent with a failing read), and the db-level tests (`TestLoadGreetingSettingsReturnsReadError`, `TestLoadGreetingSettingsMatchesGetGreetingSettings`) arrive in the GREEN commit with the function.
- **No new seam for the failing read.** The brief suggested a seam variable. The module test uses the repo's existing `settingsStaffTestFailRead`, a real GORM query callback that fails every read of the `greetings` table, and the db test registers the same kind of callback. That exercises the real read path, so no production seam was added for WR-01.
- **IN-01 RED commit carries the seam.** `welcomeRender` is introduced in the RED commit f993551 so the tests compile; the fix commit 3ffb87c holds the release logic.
- **"Panic after the send" is made deterministic with a wrapping bot client** (`greetOncePanicAfterSendClient`: records the request through the fake, then panics), which needed no production seam. That test passes before and after the fix (it pins the keep-the-claim side), so only the before-send test is RED.

## Tests

- `TestImportGreetingsOmittedBlocks` (`fuku/db/backup/greetings_omitted_test.go`): payloads with only `welcome_settings`, only `goodbye_settings`, an empty settings object, every block present (the three switches true and carried), and a rules payload with only text. It asserts the import succeeds, that the three columns are written (never NULL, read through raw SQL as nullable values) with the expected values, and the exported texts. RED on PostgreSQL (SQLSTATE 23502 for the three omitted-block payloads) and on SQLite (the columns came back NULL); green on both after the fix.
- `TestLoadGreetingSettingsReturnsReadError` and `TestLoadGreetingSettingsMatchesGetGreetingSettings` (`fuku/db/greetings/load_settings_test.go`): the error comes back with a failing read while `GetGreetingSettings` keeps its fallback; a stored row, a known chat with no row (the row is created) and an unknown chat (nothing is written) read as before. Also green on PostgreSQL.
- `TestGreetOnceReadErrorFailsClosed` (`greet_once_test.go`): Greet once on, one member already in `greeted_members`, another new, every `greetings` read failing: no `sendMessage`, no new row.
- `TestGreetOncePanicBeforeSendReleasesClaim` and `TestGreetOncePanicAfterSendKeepsClaim` (`greet_once_test.go`): a panic in the render step leaves no row and the next join is welcomed; a panic after the send request keeps the row and the next join sends nothing. The new tests use the file's `greetOnceSetup` and `greetOnceCleanup`, which delete the `greeted_members` rows and the members' `users` rows and cached copies in `t.Cleanup`, and fresh chat IDs.

## Verification

Run in the main checkout, on branch `claude/ecstatic-goldberg-qsss05`, after the last code commit:

- PostgreSQL 16 was started from `/usr/lib/postgresql/16/bin` on a throwaway data directory under `/var/lib/postgresql` (trust authentication on 127.0.0.1, no credentials written anywhere), the repository migration chain was applied with `FUKU_TEST_MIGRATION_CHAIN=true go test -tags testtools -count=1 -run '^TestRepositoryMigrationChain$' ./fuku/db/migrations` (ok), and the cluster and its data directory were stopped and deleted at the end.
- RED reproduced on PostgreSQL: `TestImportGreetingsOmittedBlocks` failed for the three omitted-block payloads with `null value in column "goodbye_media_below"` / `"welcome_media_below"` ... `(SQLSTATE 23502)`.
- After the fix on PostgreSQL (`DATABASE_URL`, `FUKU_TEST_DATABASE=true`): `TestImportGreetingsOmittedBlocks`, `TestLegacyBackupPreservesFieldsThatVersionDidNotExport`, `TestOldGreetingsExportDecodesToZeros`, `TestAllModulesRoundTripEveryMeaningfulField`, `TestImportChatDataRollsBackEarlierModules`, `TestGreetedMemberClaimConcurrent`, `TestLoadGreetingSettingsReturnsReadError` and `TestLoadGreetingSettingsMatchesGetGreetingSettings` all pass.
- On SQLite: `go test -tags testtools -race -count=1 ./fuku/db/greetings ./fuku/db/backup ./fuku/db/rules ./fuku/db/lang` passes.
- `go test -tags testtools -race -count=1 -run '^(TestGreeting|TestMemberJoin|TestLeftMember|TestCaptcha|TestLockdown|TestLegacyGreetings|TestRulesLegacy|TestSetWelcome|TestProcessSingleNewMember|TestGreetOnce|TestGreetingsMediaBelow)' ./fuku/modules` passes (83 s), with `TestLegacyGreetingsRequestsUnchanged` and `TestRulesLegacyRowSendsTodaysRequest` unchanged.
- `CGO_ENABLED=0 go build ./...` passes; `go vet -tags testtools ./fuku/modules ./fuku/db/greetings ./fuku/db/backup` passes; `gofmt -l` is clean on every touched file; the `lint-local.py 72a692b` stand-in exits 0.
- `make check-translations` passes, `make generate-docs` then `make check-docs` report no drift, `git diff --exit-code 72a692b -- go.mod go.sum` is clean.
- Not run: `make lint` and `make test` (the installed golangci-lint refuses Go 1.26.0 here, and `make test` is the phase gate's). `make test-postgres-integrity` as a whole was not run; the tests of the three fixes were run on PostgreSQL by name instead. The `fuku/modules` package cannot start on the migration-built PostgreSQL schema here (its harness `AutoMigrate` fails on `uni_users_user_id`), so the module tests ran on SQLite only, as the rest of that package does.

---

_Fixed: 2026-11-05_
_Fixer: Claude (gsd-code-fixer)_
_Iteration: 1_
