---
phase: 19-blacklist-filter-and-note-pages
plan: 05
subsystem: filters
tags: [telegram, filters, apply-function, characterization-golden, content-extraction, owner-confirmations]
status: complete

requires:
  - phase: 19-blacklist-filter-and-note-pages
    provides: 19-01's keywordTooBroad / listKeywordMaxRunes and the apply-function pattern (write seam, typed result, golden before the extraction)
provides:
  - applyFilterAdd, applyFilterRemove, applyFilterRemoveByID, checkFilterKeyword and the filterWrites seam in filters.go, used by /filter, /addfilter, /stop, /rmfilter and /removefilter
  - filters.AddFilterContext (reports whether a row was inserted), RemoveFilterContext, CountFiltersContext, FilterExistsContext, ListFilterEntriesFresh, GetFilterEntryFresh, RemoveFilterByID
  - content.ExtractTypedNoteOrFilter and extractReplyContent (the reply form of the note and filter extraction, shared with the command)
  - filters_keyword_too_broad in 8 locales
  - TestLegacyFilterRequests and TestExtractNoteAndFilterCharacterization, goldens committed before the change they protect
affects: [19-06, 19-07, 19-08, 19-09, 19-12]

plan_head_before: 3b0b506
plan_head_after: 9b24753

actuals:
  tokens: 23250   # chars/4 over the added lines of 3b0b506..9b24753 outside .planning (93003 chars, locales, goldens and AGENTS.md included)
  tasks: 3
  commits: 7      # MEASURED: git rev-list --count 3b0b506..9b24753 (before this SUMMARY commit)

tech-stack:
  added: []
  patterns:
    - "golden-first extraction: the characterization of a command (or of a shared extraction) is committed alone before the production change"
    - "owner-confirmation behaviour changes (A19-2, the fail-closed lookups) land in their own commits so declining one is a one-commit revert"
    - "a random value in a golden (the overwrite card's token) is replaced by a fixed marker after asserting its Redis key exists"

key-files:
  created:
    - fuku/modules/filters_legacy_request_test.go
    - fuku/modules/filters_apply_test.go
    - fuku/db/filters/entries_test.go
    - fuku/utils/content/extract_characterization_test.go
    - fuku/utils/content/extract_typed_test.go
  modified:
    - fuku/modules/filters.go
    - fuku/modules/list_validation_test.go
    - fuku/db/filters/repository.go
    - fuku/utils/content/content.go
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
  - "extractReplyContent builds its own result (newExtractResult) and ends in the shared finishExtract (preFixes plus NotesParser), so ExtractNoteAndFilter's reply branch is one call and the typed form cannot drift from it; the nil-message result is invalidMessageResult."
  - "A keyword refused as blank maps to filters_keyword_required in the command (unreachable from the command's own argument checks, reachable from the page)."
  - "A cap reached inside applyFilterAdd (a race after the command's own pre-check) maps to filters_limit_exceeded; the command's pre-check itself keeps its old hiding CountFilters read."

patterns-established:
  - "filterGoldenCardRe / filterGoldenCardMarker: assert the overwrite key of the card exists in the cache, then compare the request with the token replaced"

requirements-completed: []

coverage:
  - id: D1
    description: "/filter, /addfilter, /stop, /rmfilter and /filters keep every request and stored row after the extraction into apply functions"
    requirement: "PAGE-08 (contribution only)"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^(TestLegacyFilterRequests|TestApplyFilter|TestFilterCommandsWriteThroughApplyFunctions)$' ./fuku/modules (bare and with cienv.sh)"
        status: pass
    human_judgment: false
  - id: D2
    description: "a typed filter reply or note content is extracted exactly as /filter K or /save K replying to it, and ExtractNoteAndFilter's own output is unchanged"
    requirement: "PAGE-08 (contribution only)"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -v ./fuku/utils/content (TestExtractNoteAndFilterCharacterization, TestExtractTypedNoteOrFilter; bare and with cienv.sh)"
        status: pass
    human_judgment: false
  - id: D3
    description: "chat-scoped fresh readers and a delete by row ID that never touches another chat"
    requirement: "PAGE-08 (contribution only)"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^(TestFilterEntriesFresh|TestRemoveFilterByID)$' ./fuku/db/filters"
        status: pass
    human_judgment: false
  - id: D4
    description: "owner confirmations A19-2 for filters (one-code-point keyword refused) and the fail-closed lookups of /filter are provisional and each revertible alone"
    requirement: "PAGE-10 (contribution only)"
    verification:
      - kind: unit
        ref: "TestApplyFilterTooBroad, TestKeywordTooBroad, TestApplyFilter (failing lookups), TestFilterCommandRefusesWhenLookupFails"
        status: pass
    human_judgment: true
---

# Phase 19 Plan 05: the filter writes behind apply functions, typed-content extraction and fresh readers Summary

**`/filter` and `/stop` now write through `applyFilterAdd` and `applyFilterRemove` behind a golden that was committed first, a typed reply is extracted exactly as `/filter K` replying to it is (`content.ExtractTypedNoteOrFilter`, behind its own characterization), a one-character keyword is refused, a failed lookup no longer lets a filter through, and the Filters page's chat-scoped reads and delete by row ID exist.**

## What was built

- **Golden first** (`c223cb2`): `TestLegacyFilterRequests` pins 23 cases of `/filter`, `/stop` and `/filters` (the non-admin and no-`can_change_info` refusals, the cap at 150, no reply and no keyword, a poll, a text over 4096, a 101-rune keyword, an inline add with a mixed-case keyword, a reply to text, a photo reply with a caption and a markdown button (stored type 4, file ID, button), the `/addfilter` alias, an existing keyword with the cache (card with its token replaced by a fixed marker, the overwrite key asserted present) and without it (`filters_overwrite_token_failed`), a write failure, `/stop` alone, an absent keyword, the original case in the reply, a quoted two-word keyword, the `/rmfilter` alias, a delete failure, the empty and filled list). It holds no single-code-point keyword and no failing existence or count read. It passed unchanged after every later commit, bare and with `cienv.sh`.
- **Extraction** (`deef07b`): `applyFilterAdd`, `applyFilterRemove`, `checkFilterKeyword`, the seam `filterWrites`, the typed results and `filtersCapMax`; the repository gained `AddFilterContext` (the inserted flag), `RemoveFilterContext`, `CountFiltersContext` and `FilterExistsContext` and the old functions are wrappers with their old hiding behaviour. `/filter` keeps its recover, cap pre-check, argument checks, extraction and overwrite card (now `offerFilterOverwrite`); a lost insert race is replied as a success as before. In this commit a failed lookup still read as not found and 0.
- **Content** (`e696952` then `ca22c2e`): `TestExtractNoteAndFilterCharacterization` (25 command messages x `/filter` and `/save`, every field of the result) committed alone; then `extractReplyContent`, `finishExtract`, `newExtractResult`, `invalidMessageResult` and the exported `ExtractTypedNoteOrFilter`. The characterization passes unchanged; `TestExtractTypedNoteOrFilter` proves the typed form equals the command's reply form over the reply-form table for both modes, and documents the two-word-keyword difference.
- **A19-2 for filters** (`f0dd7f7`): `checkFilterKeyword` refuses `keywordTooBroad` after the length check; `/filter` answers `filters_keyword_too_broad` (new key, 8 locales) and stores nothing.
- **Fail-closed lookups** (`4f794f1`): a failed existing-keyword or filter-count read in `checkFilterKeyword` is `filterSaveFailed`; `/filter` answers `common_settings_save_failed` and stores nothing.
- **Readers and delete by ID** (`9b24753`): `ListFilterEntriesFresh`, `GetFilterEntryFresh`, `RemoveFilterByID` (one `DELETE ... WHERE id = ? AND chat_id = ? RETURNING`, both caches dropped only when a row went) and `applyFilterRemoveByID` (removed, gone, save failed) with `filterWrites.RemoveByID`.
- **Callers (grep of every production caller of the filter repository writes)**: `AddFilter` and `RemoveFilter` are called only by `addFilter` and `rmFilter`, both now on the apply functions (`grep -c 'db_filters.AddFilter(\|db_filters.RemoveFilter(' fuku/modules/filters.go` is 0). `CountFilters` is called by `addFilter`'s pre-check (kept on purpose) and `DoesFilterExists` only by tests now. `UpdateFilter` (the overwrite callback), `RemoveAllFilters` (`/stopall`) and the backup import are other writes, out of scope, untouched. Wiring is proved by `TestFilterCommandsWriteThroughApplyFunctions`, which presses the real `/filter` and `/stop` handlers with counting seams.

## Owner confirmations this plan lands (for `19-SPIKE.md`, plan 19-12)

1. **A19-2 for filters (provisional)**: `/filter` refuses a keyword of exactly one code point outside Unicode category So (and a blank one) with the reply key `filters_keyword_too_broad` ("This filter keyword is too broad: a single letter, digit or punctuation mark would match almost every message.") and stores nothing. Commit `f0dd7f7 feat(19-05): /filter refuses a keyword that would match almost every message (A19-2)`; `git revert f0dd7f7` declines it (the locale key then goes too, which is correct).
2. **Fail-closed lookups (provisional)**: when its existing-keyword or filter-count read fails, `/filter` answers `common_settings_save_failed` and stores nothing; before this phase it silently added the filter (and could add past the cap or over an existing keyword). Commit `4f794f1 fix(19-05): /filter refuses to add when its existence or count lookup fails`; `git revert 4f794f1` restores the old behaviour. The command's own cap pre-check keeps its old hiding read, so a failed count there still lets the command continue to the apply function, whose own count read then refuses.

## Verification (real results)

- `go test -tags testtools -race -count=1 -run '^(TestLegacyFilterRequests|TestApplyFilter|TestApplyFilterTooBroad|TestKeywordTooBroad|TestFilterCommandRefusesWhenLookupFails|TestFilterCommandCapPreCheckStays|TestFilterCommandsWriteThroughApplyFunctions|TestAddListWatchAndRemoveTextFilter|TestFilterCommandValidationBranches|TestAddFilterRejectsLimitAndNonAdmin|TestFilterOverwriteCallbackReplacesExistingFilter|TestRemoveAndListFilterValidationBranches)$' ./fuku/modules`: pass bare, and the golden, apply and too-broad tests pass with `. cienv.sh` (re-run after each commit).
- `go test -tags testtools -race -count=1 ./fuku/db/filters`: pass (bare and with cienv.sh); `-v` shows `TestFilterEntriesFresh` and `TestRemoveFilterByID` and their subtests passing. `./fuku/utils/content`: pass bare and with cienv.sh. `./fuku/i18n`: pass (23 s). `make check-translations`: all present. `make check-docs`: no drift.
- Whole `fuku/modules` package under the CI environment (`go test -tags testtools -race -count=1 -timeout 40m -v ./fuku/modules`, in the background, output in the scratchpad): `ok ... 1861.351s`, `exit 0`, 1801 top-level `--- PASS`, 0 `--- FAIL`, 0 `--- SKIP`, 0 `^FAIL`; `--- PASS: TestLegacyFilterRequests` present once. This run includes the settings test family.
- Gates: `go vet -tags testtools` on `fuku/modules`, `fuku/db/filters` and `fuku/utils/content` clean; `CGO_ENABLED=0 go build ./...` clean; `gofmt -l` on every file this plan created or edited prints nothing; the lint stand-in (`lint-local.py 8c0c827`, exit read directly) exits 0 ("no dupl, gocyclo or godox findings on lines added since 8c0c827"); `git diff --exit-code 8c0c827 -- go.mod go.sum` exits 0; the `nolint` counts are unchanged (2 in `filters.go`, 1 in `content.go`); the settings key count is 420 in all 8 locale files; no `t.Skip`, `reflect.Ptr` or `ioutil` in code this plan added (the one `t.Skip` in `filters_test.go` line 51 is older than this phase and not touched).
- Not run: `make lint` and gosec (cannot run in the sandbox), `make test` (only the closing plan runs it), PostgreSQL (no migration or constraint in this plan; the new `RemoveFilterByID` uses the same `DELETE ... RETURNING` shape as `RemoveBlacklistByID`, which plan 19-01 ran on SQLite only, and `ON CONFLICT DO NOTHING` insert is unchanged).

## Time added to the fuku/modules run

About 4.5 s: `TestLegacyFilterRequests` 1.16 s, `TestApplyFilterTooBroad` 1.14 s, `TestFilterCommandRefusesWhenLookupFails` 1.13 s, `TestFilterCommandCapPreCheckStays` 1.08 s (each of the last three installs the real locale files, about 1 s), `TestApplyFilter` 0.03 s, `TestFilterCommandsWriteThroughApplyFunctions` 0.01 s, `TestKeywordTooBroad` 0.00 s. No new test is over 10 s. `fuku/utils/content` adds under 0.1 s and `fuku/db/filters` under 0.1 s.

## Deviations from Plan

- **Constant name**: the golden's card marker constant is `filterGoldenCardMarker`, not the planned `filterGoldenTokenPlaceholder`, so no identifier holding a string literal contains "token" (gosec G101, as the brief says).
- **Extra tests**: `TestFilterCommandsWriteThroughApplyFunctions` (counting seams through the real `/filter` and `/stop`, the wiring proof the brief asks for), `TestFilterCommandRefusesWhenLookupFails` and `TestFilterCommandCapPreCheckStays` (the command-level halves of commit B and of the kept pre-check). The plan listed only `TestApplyFilter` cases for these.
- **A19-2 AGENTS.md lines**: the plan puts the AGENTS.md lines in commits B and C; the A19-2 sentence is in commit A and the fail-closed sentence in B, so each owner-confirmation commit carries its own rule text and reverts cleanly alone.
- **Outcome map**: `filterKeywordBlank` and `filterCapReached` get replies in the command (`filters_keyword_required`, `filters_limit_exceeded`) although the command's own pre-checks make both unreachable except in a race; the plan did not say how to map them.
- **Plan base**: no per-plan HEAD ledger file was created at the first commit; `plan_head_before` is the HEAD at the start of the plan (`3b0b506`).
- No other deviation: the golden and the characterization were committed alone and never re-recorded; no request or reply byte of `/filter`, `/stop` or `/filters` changed except the two named owner-confirmation changes.

## Open items for later plans and the owner

- `19-SPIKE.md` (plan 19-12) must record the two owner confirmations above, each revertible by one commit (`f0dd7f7`, `4f794f1`).
- `/save` shares `ExtractNoteAndFilter`; its own golden is recorded in plan 19-08 (the characterization here pins the shared extraction). The `/save` caption panic (research C-10) is a pre-existing defect left for `19-SPIKE.md`.
- `applyFilterRemoveByID`, `applyFilterAdd`'s `filterRaced`/`filterCapReached` outcomes and the fresh readers have the Filters page of plans 19-06 and 19-07 as their only production user; each is exercised by a test of this plan.
- Known and left: the success reply of `/filter` splices the keyword unescaped; `/filters` lists in the repository's read order; a lost insert race is replied as a success.

## Known Stubs

None.

## Threat Flags

None: the plan added no endpoint, auth path or trust boundary; the delete by row ID is scoped by `chat_id` (T-19-25) and the typed extraction runs the command's own `preFixes` (T-19-24).

## Requirements

`requirements-completed: []`. PAGE-08 and PAGE-10 stay Pending; 19-12 completes PAGE-08.

## Self-Check: PASSED

- Files exist: `fuku/modules/filters_legacy_request_test.go`, `fuku/modules/filters_apply_test.go`, `fuku/db/filters/entries_test.go`, `fuku/utils/content/extract_characterization_test.go`, `fuku/utils/content/extract_typed_test.go`.
- Commits are ancestors of HEAD: `c223cb2`, `deef07b`, `e696952`, `ca22c2e`, `f0dd7f7`, `4f794f1`, `9b24753`.
