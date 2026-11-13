---
phase: 19-blacklist-filter-and-note-pages
plan: 01
subsystem: blacklists
tags: [telegram, settings-menu, blacklists, apply-function, characterization-golden, migration, postgresql]

requires:
  - phase: 18-remaining-settings-pages
    provides: the apply-function shape (write seam, typed result, golden before the extraction) that this plan applies to the blacklist commands
provides:
  - applyBlacklistAdd, applyBlacklistRemove, applyBlacklistRemoveByID and applyBlacklistAction in blacklists.go (typed results, write seam blacklistWrites) used by /addblacklist, /rmblacklist and /blacklistaction
  - blacklists.ListBlacklistEntriesFresh, GetBlacklistEntryFresh, CountBlacklistFresh and RemoveBlacklistByID, fresh chat-scoped non-creating readers and the delete by row ID
  - keywordTooBroad and listKeywordMaxRunes in list_validation.go (shared by the blacklist now, the filter keyword in 19-05)
  - blacklists_blacklist_word_too_broad in 8 locales
  - migration 20261113120000 dropping the legacy chk_blacklists_action, and TestBlacklistActionValuesOnPostgres in make test-postgres-integrity
  - TestLegacyBlacklistRequests, the golden of the four blacklist commands, recorded before the extraction
affects: [19-02, 19-03, 19-04, 19-11, 19-12]

plan_head_before: 31f17f2cea8769fab92447b69b55384af8028d00
plan_head_after: 76408de67fd6109fd5dd94f6580deb98ea23cc2f

actuals:
  tokens: 18800   # chars/4 over the added lines of d93ec1d..76408de outside .planning (75344 chars)
  tasks: 4
  commits: 6      # MEASURED: git rev-list --count 31f17f2..HEAD (before this SUMMARY commit)

tech-stack:
  added: []
  patterns:
    - "golden-first extraction: the characterization test of a command is committed alone before the production change"
    - "owner-confirmation behaviour changes (A19-2, A19-3) land in their own commits so declining one is a one-commit revert"
    - "PostgreSQL-only integrity test with a database-free statements half for a migration"

key-files:
  created:
    - fuku/modules/blacklists_legacy_request_test.go
    - fuku/modules/blacklists_apply_test.go
    - fuku/modules/list_validation.go
    - fuku/modules/list_validation_test.go
    - fuku/db/blacklists/entries_test.go
    - fuku/db/migrations/blacklist_action_check_test.go
    - migrations/20261113120000_drop_legacy_blacklist_action_check.sql
  modified:
    - fuku/modules/blacklists.go
    - fuku/db/blacklists/repository.go
    - Makefile
    - locales/en.yml
    - locales/es.yml
    - locales/fr.yml
    - locales/hi.yml
    - locales/id.yml
    - locales/pt.yml
    - locales/ru.yml
    - locales/uk.yml
    - AGENTS.md

key-decisions:
  - "blacklistSortWords takes only the words (the plan's second `existing` argument had no use: the already-listed check lives in applyBlacklistAdd); applyBlacklistAdd takes the existing rows as models.BlacklistSettingsSlice, as the plan's signature says, so the action can be inherited."
  - "A new word inherits the shared action only when it is one of mute, kick, warn, ban, none; a stored value the commands never write (tban, delete) falls back to warn."
  - "The too-broad refusal shows the word as typed (escaped), is checked before the already-listed check and is a separate reply right after the too-long one."
  - "The C-16 proof said none IS refused on a chain-built database, so the migration was written (one statement, DROP CONSTRAINT IF EXISTS chk_blacklists_action)."

patterns-established:
  - "blacklistGoldenClient / blacklistGoldenFail test doubles: a member without can_restrict_members, a bot without it, and a GORM callback that fails one write kind of one chat (create, delete or update) matched on the statement's WHERE values"

requirements-completed: []

coverage:
  - id: D1
    description: "/addblacklist, /rmblacklist, /blacklistaction and /blacklists keep every request and stored row after the extraction into apply functions"
    requirement: "PAGE-10 (contribution only)"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^(TestLegacyBlacklistRequests|TestApplyBlacklist)$' ./fuku/modules (bare and with cienv.sh)"
        status: pass
    human_judgment: false
  - id: D2
    description: "chat-scoped fresh readers and a delete by row ID that never touches another chat"
    requirement: "PAGE-07 (contribution only)"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^(TestBlacklistEntriesFresh|TestRemoveBlacklistByID)$' ./fuku/db/blacklists"
        status: pass
    human_judgment: false
  - id: D3
    description: "the chain-built PostgreSQL table accepts every action the command offers"
    requirement: "PAGE-07 (contribution only)"
    verification:
      - kind: integration
        ref: "TestBlacklistActionValuesOnPostgres on a throwaway PostgreSQL 16 cluster: failed before the migration, passes after (measured below)"
        status: pass
    human_judgment: false
  - id: D4
    description: "owner confirmations A19-2 (too-broad word refused) and A19-3 (new word inherits the shared action) are provisional and each revertible alone"
    requirement: "PAGE-07 (contribution only)"
    verification:
      - kind: unit
        ref: "TestKeywordTooBroad, TestApplyBlacklistTooBroad, TestApplyBlacklistInheritsAction"
        status: pass
    human_judgment: true
---

# Phase 19 Plan 01: the blacklist writes behind apply functions, fresh readers and a schema fix Summary

**The four blacklist commands now write through apply functions behind a golden that was committed first, the page's chat-scoped readers and delete by row ID exist, a one-character word is refused and a new word inherits the list's action, and a migration drops the legacy CHECK that made `/blacklistaction none` fail on PostgreSQL.**

## What was built

- **Golden first** (`a8de41a`): `TestLegacyBlacklistRequests` pins 27 cases (every gate the fake reaches, no argument, a 101-rune word, one and several words with a repeat, a word already present, a failed write on one of two words, the remove and action paths including the empty-list success of `/blacklistaction`, an update failure, the empty and filled `/blacklists` with escaping and sorting). It passes unchanged after every later commit, bare and with `cienv.sh`.
- **Extraction** (`f90a3ce`): `applyBlacklistAdd`, `applyBlacklistRemove`, `applyBlacklistAction`, the seam `blacklistWrites`, the typed results, `blacklistActionValues` and the pure `blacklistSortWords`. The repository gained `AddBlacklistContext`, `RemoveBlacklistContext` and `SetBlacklistActionContext` (the rows matched are returned); the old functions are thin wrappers. The two log lines carrying a word are gone (`grep -c '"word":'` is 0).
- **Readers and delete by ID** (`9616775`): `ListBlacklistEntriesFresh`, `GetBlacklistEntryFresh`, `CountBlacklistFresh` and `RemoveBlacklistByID` (one `DELETE ... WHERE id = ? AND chat_id = ? RETURNING`), plus `applyBlacklistRemoveByID` (removed, gone, save failed). A foreign row ID reads and deletes nothing and keeps the other chat's cache key.
- **A19-2** (`12ee6a4`) and **A19-3** (`3025b0f`): each in its own commit with its own test and AGENTS.md lines, so the owner can decline either by reverting one commit.
- **C-16** (`76408de`): proved on a throwaway PostgreSQL 16 cluster that the chain-built `blacklists` kept both checks and refused `none`; migration `20261113120000_drop_legacy_blacklist_action_check.sql` drops the legacy one; `TestBlacklistActionValuesOnPostgres` joined `make test-postgres-integrity` and `TestBlacklistActionMigrationStatements` runs in `make test`.

## Verification (real results)

- `go test -tags testtools -race -count=1 -run '^(TestLegacyBlacklistRequests|TestApplyBlacklist|TestApplyBlacklistTooBroad|TestApplyBlacklistInheritsAction|TestKeywordTooBroad|TestAddListActionAndRemoveBlacklistCommands|TestBlacklistCommandsHandleValidationBranches)$' ./fuku/modules`: pass bare, and pass with `. cienv.sh` for the golden, apply and validation tests.
- `go test -tags testtools -race -count=1 ./fuku/db/blacklists`: pass, bare and with cienv.sh. `./fuku/i18n`: pass. `make check-translations`: all present.
- PostgreSQL (throwaway PG16 cluster under `/var/lib/postgresql/fuku_pg19_01`, trust auth on 127.0.0.1, stopped and deleted afterwards): after `TestRepositoryMigrationChain` and before the migration, `TestBlacklistActionValuesOnPostgres` FAILED with `action "none" must be accepted ... violates check constraint "chk_blacklists_action" (SQLSTATE 23514)`. `pg_constraint` on `blacklists` read:
  - `chk_blacklist_action | NOT VALID | CHECK (action = ANY (warn, mute, ban, kick, tban, tmute, delete, none))`
  - `chk_blacklists_action | validated | CHECK (action = ANY (warn, mute, ban, kick, tban, tmute, delete))`
  After the migration, on a database rebuilt from scratch: only `chk_blacklist_action` remains, the test passes (warn, mute, kick, ban and none accepted, `x` refused with SQLSTATE 23514), and the whole `./fuku/db/migrations` package passes with `FUKU_TEST_DATABASE=true`. `schema_migrations` listed the new file last.
- Whole `fuku/modules` package under the CI environment (`go test -tags testtools -race -count=1 -timeout 40m -v ./fuku/modules`, in the background): `ok ... 1729.660s`, exit 0, 1765 top-level PASS, 0 FAIL, 0 SKIP.
- Gates: `go vet -tags testtools` on `fuku/modules`, `fuku/db/blacklists`, `fuku/db/migrations` clean; `CGO_ENABLED=0 go build ./...` clean; `gofmt -l` on every file this plan touched prints nothing; the lint stand-in (`lint-local.py 8c0c827`) exits 0 ("no dupl, gocyclo or godox findings"); `git diff --exit-code 8c0c827 -- go.mod go.sum` exits 0; `fuku/db/migrations/runner.go` and `scripts/migrate_psql.sh` are unchanged against `d93ec1d`; no `t.Skip` in the new tests except the PostgreSQL-only skips of the migrations package, which CI's skip gate allows.
- Not run: `make lint` and gosec (cannot run in the sandbox), `make test` (only the closing plan runs it), `make test-postgres-integrity` as a whole (only the new test and the migrations package were run on PostgreSQL), the settings test family (no settings code changed), the `scripts/migrate_psql.sh` applier (the file is a single idempotent `ALTER TABLE ... DROP CONSTRAINT IF EXISTS`, which both appliers run unchanged).

## Time added to the fuku/modules run

About 2.2 s: `TestLegacyBlacklistRequests` 1.09 s, `TestApplyBlacklistTooBroad` 1.07 s (it installs the real locale files), `TestApplyBlacklist` 0.02 s, `TestApplyBlacklistInheritsAction` 0.01 s, `TestKeywordTooBroad` 0.00 s. No new test is over 10 s. The `fuku/db/blacklists` tests add under 0.1 s.

## Deviations from Plan

- **Signature**: `blacklistSortWords(words []string) (valid, tooLong []string)` without the plan's unused `existing map[string]bool` argument (it would have been an unused parameter; the already-listed check is in `applyBlacklistAdd`). No later plan calls it.
- **Test placement**: the plan puts `TestApplyBlacklistTooBroad` in commit B; it is in commit A (A19-2) with the AGENTS.md lines for each change in its own commit, so each owner-confirmation commit reverts cleanly alone.
- **Acceptance criterion wording**: `grep -c -- '--- PASS: TestLegacyBlacklistRequests'` prints 28, not 1, because the golden has 27 subtests that share the prefix; the parent test line is present once.
- **Added helper behaviour**: a new word inherits the shared action only for the five values the commands write (a stored `tban` or `delete` falls back to warn); the plan said "else warn" without this edge.
- **Plan base**: the per-plan HEAD ledger file of the commit protocol was not created at the first commit, so `plan_head_before` is the HEAD at the start of the plan (`31f17f2`).
- No other deviation: the golden was committed alone and unchanged; no request or reply byte of a command changed except the two named owner-confirmation changes.

## Open items for later plans and the owner

- `19-SPIKE.md` (plan 19-12) must record A19-2 (too-broad refusal) and A19-3 (inherited action) as provisional owner confirmations, each revertible by one commit (`12ee6a4`, `3025b0f`).
- Known and left (C-10 quirks, not fixes): `/blacklistaction <action>` on an empty list answers its success text and writes nothing; the repository's `[Database] AddBlacklist` error lines still carry the database error text (which on PostgreSQL can quote a word), unchanged by this plan.
- `RemoveAllBlacklist` (the `/rmallbl` button) stays outside the apply functions; it is not a page control.

## Known Stubs

None.

## Threat Flags

None: the plan added no endpoint, auth path or trust boundary; the one schema change loosens a constraint to match the values the commands already write.

## Self-Check: PASSED

- Files exist: `fuku/modules/blacklists_legacy_request_test.go`, `fuku/modules/blacklists_apply_test.go`, `fuku/modules/list_validation.go`, `fuku/modules/list_validation_test.go`, `fuku/db/blacklists/entries_test.go`, `fuku/db/migrations/blacklist_action_check_test.go`, `migrations/20261113120000_drop_legacy_blacklist_action_check.sql`.
- Commits are ancestors of HEAD: `a8de41a`, `f90a3ce`, `9616775`, `12ee6a4`, `3025b0f`, `76408de`.
