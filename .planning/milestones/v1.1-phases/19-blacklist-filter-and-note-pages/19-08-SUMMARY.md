---
phase: 19-blacklist-filter-and-note-pages
plan: 08
subsystem: notes
tags: [telegram, notes, apply-function, characterization-golden, owner-confirmations]
status: complete

requires:
  - phase: 19-blacklist-filter-and-note-pages
    provides: 19-01's listKeywordMaxRunes and the apply-function pattern, 19-05's golden technique (card token placeholder) and the filters' reader shapes
provides:
  - applyNoteAdd, applyNoteRemove, applyNoteRemoveByID, checkNoteName and the noteWrites seam in notes.go, used by /save, /addnote, /clear and /rmnote
  - noteNameInvalid (A19-4) in list_validation.go
  - notes.AddNoteContext (reports whether a row was inserted), RemoveNoteContext, NoteExistsContext, ListNoteEntriesFresh, GetNoteEntryFresh, CountNotesFresh, RemoveNoteByID
  - notes_name_invalid in 8 locales
  - TestLegacyNoteRequests, a golden committed before the extraction
affects: [19-09, 19-12]

plan_head_before: 342c827
plan_head_after: 666f9af

actuals:
  tokens: 21000   # chars/4 over the lines added by 342c827..666f9af outside .planning (goldens, locales and AGENTS.md included)
  tasks: 3
  commits: 6      # MEASURED: git rev-list --count 342c827..666f9af (before this SUMMARY commit)

tech-stack:
  added: []
  patterns:
    - "golden-first extraction: the characterization of a command is committed alone before the production change"
    - "owner-confirmation behaviour changes (A19-4, the fail-closed lookup) land in their own commits so declining one is a one-commit revert"

key-files:
  created:
    - fuku/modules/notes_legacy_request_test.go
    - fuku/modules/notes_apply_test.go
    - fuku/db/notes/entries_test.go
  modified:
    - fuku/modules/notes.go
    - fuku/modules/list_validation.go
    - fuku/modules/list_validation_test.go
    - fuku/db/notes/repository.go
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
  - "The shape outcome of checkNoteName is noteNameBadShape, not noteNameInvalid as the plan lists, because the plan also names the validation function noteNameInvalid and one package cannot hold a constant and a function of one name."
  - "A name refused as blank maps to notes_keyword_required in /save (unreachable from the command's own argument checks, reachable from the page), as 19-05 did for filters."
  - "The apply functions take a plain context.Context; /save and /clear pass tracing.UpdateContext(ctx) as /filter and /stop do (the old calls used context.Background())."

patterns-established:
  - "noteGoldenCardMarker: assert the overwrite key of the card exists in the cache, then compare the request with the token replaced"

requirements-completed: []

coverage:
  - id: D1
    description: "/save, /addnote, /clear, /rmnote and /notes keep every request and stored row after the extraction into apply functions"
    requirement: "PAGE-09 (contribution only)"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^(TestLegacyNoteRequests|TestApplyNote|TestNoteCommandsWriteThroughApplyFunctions)$' ./fuku/modules (bare and with cienv.sh)"
        status: pass
    human_judgment: false
  - id: D2
    description: "chat-scoped fresh readers and a delete by row ID that never touches another chat or notes_settings"
    requirement: "PAGE-09 (contribution only)"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^(TestNoteEntriesFresh|TestRemoveNoteByID)$' ./fuku/db/notes"
        status: pass
    human_judgment: false
  - id: D3
    description: "a note the apply function stores is fetched at once by #name and /get, and gone after the delete by row ID"
    requirement: "PAGE-09 (contribution only)"
    verification:
      - kind: unit
        ref: "TestNoteApplyThenFetch"
        status: pass
    human_judgment: false
  - id: D4
    description: "owner confirmations A19-4 (note-name rule) and the fail-closed existence lookup of /save are provisional and each revertible alone"
    requirement: "PAGE-10 (contribution only)"
    verification:
      - kind: unit
        ref: "TestNoteNameInvalid, TestApplyNoteNameRule, TestApplyNote (failing lookup), TestNoteCommandRefusesWhenLookupFails"
        status: pass
    human_judgment: true
---

# Phase 19 Plan 08: the note writes behind apply functions, the note-name rule and fresh readers Summary

**`/save` and `/clear` now write through `applyNoteAdd` and `applyNoteRemove` behind a golden that was committed first, `/save` refuses a name that `#name` and `/get` could never fetch, a failed existence lookup no longer lets a note through, and the Notes page's chat-scoped reads and delete by row ID exist.**

## What was built

- **Golden first** (`1f159fc`): `TestLegacyNoteRequests` pins 25 cases of `/save`, `/addnote`, `/clear`, `/rmnote` and `/notes` (request JSON and stored rows): the non-admin, bot-not-administrator and no-`can_change_info` refusals, `/save` alone and with no reply, a poll reply, an inline multi-line add (stored collapsed, `line one line <b>two</b>`), a mixed-case name, a reply to text, a photo reply with a caption holding `{admin}` and a markdown button (stored type 4, file ID, admin only, the directive kept in the text), the private-flag conflict (both flags false, the text keeps `{private} {noprivate}`), the alias, an existing name with the cache (card with its token replaced by a fixed marker, the overwrite key asserted present) and without it (`notes_overwrite_token_failed`), a write failure (`notes_save_failed`), `/clear` alone, an absent name, a removal, the stripped leading `#`, the original case, the `/rmnote` alias, an admin without `can_change_info` (refused after the argument check), a delete failure (`error_generic`), the empty and filled list. Chats from `-1009919001301`. It holds no name with whitespace, a leading `#` or over 100 code points and no failing existence read. It passed unchanged after every later commit, bare and with `cienv.sh`.
- **Extraction** (`6d3f326`): `applyNoteAdd`, `applyNoteRemove`, `checkNoteName`, the seam `noteWrites`, the typed results and `offerNoteOverwrite` (the card code moved out of `addNote` unchanged); the repository gained `AddNoteContext` (the inserted flag), `RemoveNoteContext` and `NoteExistsContext`, and `AddNote`, `RemoveNote` and `DoesNoteExists` are wrappers with their old hiding behaviour. In this commit a failed existence lookup still read as not found. The two log lines that printed a note name (`Failed to add note %s`, `Failed to remove note %s`) are gone; the apply functions log the chat ID and the error only.
- **A19-4** (`2eeac8f`): `noteNameInvalid` (white space, a leading `#`, over 100 code points); `checkNoteName` returns `noteNameBadShape` after the blank check and before the existence read; `/save` answers `notes_name_invalid` (new key, 8 locales).
- **Fail-closed lookup** (`ed7988a`): a failed existing-name read in `checkNoteName` is `noteSaveFailed`; `/save` answers `notes_save_failed` and stores nothing.
- **Readers and delete by ID** (`82ac0a5`): `ListNoteEntriesFresh`, `GetNoteEntryFresh`, `CountNotesFresh` (the same shapes as the filters readers, so 19-09's generic adapters apply with no new clone) and `RemoveNoteByID` (one `DELETE ... WHERE id = ? AND chat_id = ? RETURNING`, `notes_list` dropped only when a row went, `notes_settings` never touched), and `applyNoteRemoveByID` (removed, gone, save failed) behind `noteWrites.RemoveByID`.
- **Module proof** (`666f9af`): `TestNoteApplyThenFetch` drives the real `#name` watcher and `/get`.
- **Callers (fresh grep of every production caller of the note repository writes)**: `AddNote` and `RemoveNote` are called only by `addNote` and `rmNote` (and the existing tests), both now on the apply functions (`grep -n 'notes.AddNote(\|notes.RemoveNote(' fuku/modules/notes.go` finds nothing). `UpdateNote` (the overwrite callback), `RemoveAllNotes` (`/clearall`), `TooglePrivateNote` and the backup import are other writes, out of scope, untouched. Wiring is proved by `TestNoteCommandsWriteThroughApplyFunctions`, which presses the real `/save` and `/clear` handlers with counting seams.

## Owner confirmations / command behaviour changed (for `19-SPIKE.md`, plan 19-12)

1. **A19-4 (provisional)**: `/save` refuses a note name that contains white space, starts with `#` or is over 100 code points with the reply key `notes_name_invalid` ("A note name must be one word of at most 100 characters and must not start with #.") and stores nothing, so every stored note can be fetched by `#name` and `/get`. Commit `2eeac8f feat(19-08): /save refuses a note name that could never be fetched (A19-4)`; `git revert 2eeac8f` declines it (the locale key goes with it). Before, `/save #x hi` stored `#x`. `/clear` is untouched and still strips a leading `#`.
2. **Fail-closed lookup (provisional)**: when its existing-name read fails, `/save` answers `notes_save_failed` ("Failed to save note. Please try again later.", the key `/save` already used for a failed write) and stores nothing; before this phase it silently added the note. Commit `ed7988a fix(19-08): /save refuses to add when its existence lookup fails`; `git revert ed7988a` restores the old behaviour.

## Verification (real results)

- Task 1: `go test -tags testtools -race -count=1 -run '^(TestLegacyNoteRequests|TestApplyNote|TestNoteCommandsWriteThroughApplyFunctions|TestAddGetListAndRemoveTextNote|TestAddExistingNoteUsesOverwriteConfirmation|TestAddNoteValidationAndPrivateFlagConflict|TestRemoveNoteAndClearAllValidationBranches|TestNoteOverwriteHandlerCancelAndSuccess|TestNoteCommandsPropagateGotgbotReplyErrors)$' ./fuku/modules`: `ok`. The golden also passed bare and with `. cienv.sh` after every commit (`ok ... 2.4 s` each time; commit `1f159fc` alone first, then `6d3f326`, `2eeac8f`, `ed7988a`, `82ac0a5`, `666f9af`).
- Task 2: `TestNoteNameInvalid`, `TestApplyNoteNameRule`, `TestApplyNote`, `TestNoteCommandRefusesWhenLookupFails`, `TestLegacyNoteRequests`: `ok` bare and with `cienv.sh`; `go test -tags testtools -race -count=1 -v -run '^(TestNoteEntriesFresh|TestRemoveNoteByID)$' ./fuku/db/notes` shows both and their nine subtests passing; `go test -tags testtools -race -count=1 ./fuku/db/notes`: `ok` (bare and with `cienv.sh`); `make check-translations`: "All translations are present!"; `make check-docs`: no drift; `go test -tags testtools -race -count=1 ./fuku/i18n`: `ok` (26 s).
- Task 3: `TestNoteApplyThenFetch`, `TestApplyNote`, `TestLegacyNoteRequests`: `ok` bare and with `cienv.sh`.
- Whole `fuku/modules` package under the CI environment (`. cienv.sh; go test -tags testtools -race -count=1 -timeout 40m -v ./fuku/modules`, in the background, output in the scratchpad `modules-19-08.log`): `ok ... 1959.767s`, `exit 0`, 1822 top-level `--- PASS`, 0 `--- FAIL`, 0 `--- SKIP`, 0 `^FAIL`; `--- PASS: TestLegacyNoteRequests (1.24s)` present once. This run includes the settings test family (384 top-level `TestSettings*`/`TestLoadSettings*` passes), so it was not run a second time on its own. The run was 1960 s against 1861 s for plan 19-05's; the difference is the machine (this run shared the CPU with short focused runs of mine for part of its length), not 5 s of tests.
- Gates: `go vet -tags testtools ./fuku/modules ./fuku/db/notes` clean; `CGO_ENABLED=0 go build ./...` clean; `gofmt -l` on every file this plan created or edited prints nothing (the other files `gofmt -l ./fuku/modules` lists are older than this phase and not touched); the lint stand-in (`lint-local.py 8c0c827`, exit read directly, run after the extraction, after the readers and again before this SUMMARY) exits 0 ("no dupl, gocyclo or godox findings on lines added since 8c0c827"); `git diff --exit-code 8c0c827 -- go.mod go.sum` exits 0; `grep -c 'Failed to add note %s\|Failed to remove note %s' fuku/modules/notes.go` is 0 (baseline 2); `nolint` count in `notes.go` is 2, unchanged; the `settings_` key count is 438 in all 8 locale files (this plan adds `notes_name_invalid`, a command key); no `t.Skip` in the new files and no `reflect.Ptr` or `ioutil` in code this plan added.
- Not run: `make lint` and gosec (cannot run in the sandbox), `make test` (only the closing plan runs it), PostgreSQL (no migration or constraint in this plan; `RemoveNoteByID` uses the same `DELETE ... RETURNING` shape as `RemoveFilterByID` and `RemoveBlacklistByID`, proved on SQLite only here, and the `ON CONFLICT DO NOTHING` insert is unchanged).

## Time added to the fuku/modules run

About 4.8 s: `TestLegacyNoteRequests` 1.28 s, `TestApplyNoteNameRule` 1.16 s, `TestNoteCommandRefusesWhenLookupFails` 1.13 s, `TestNoteApplyThenFetch` 1.22 s (each of these installs the real locale files, about 1 s), `TestApplyNote` 0.02 s, `TestNoteCommandsWriteThroughApplyFunctions` 0.02 s, `TestNoteNameInvalid` 0.00 s. No new test is over 10 s. `fuku/db/notes` adds about 0.05 s.

## Deviations from Plan

- **Outcome name**: `noteNameBadShape`, not `noteNameInvalid` (see key-decisions); the function keeps the planned name `noteNameInvalid`.
- **Reply-form test for A19-4**: the plan says `/save two words` replying to a text replies `notes_name_invalid`; in the reply form the keyword is the first word, so the test uses the quoted form `/save "two words"` (and `/save #x` in both forms), which reaches the same refusal.
- **Extra tests**: `TestNoteCommandsWriteThroughApplyFunctions` (counting seams through the real handlers, the wiring proof the brief asks for) and `TestNoteCommandRefusesWhenLookupFails` (the command half of commit B).
- **AGENTS.md lines**: each owner-confirmation commit carries its own rule sentence (A19-4 in `2eeac8f`, the fail-closed lookup in `ed7988a`, the readers in `82ac0a5`), so each reverts cleanly alone.
- **Golden client**: the bot-not-administrator refusal is reached through a test client that shows the bot as a plain member (`noteGoldenClient`), which the filters golden did not cover.
- **Plan base**: the per-plan HEAD ledger was created after the golden commit, with the parent `342c827` as its value.
- No other deviation: the golden was committed alone and never re-recorded; no request or reply byte of `/save`, `/clear` or `/notes` changed except the two named owner-confirmation changes.

## Open items for later plans and the owner

- `19-SPIKE.md` (plan 19-12) must record the two owner confirmations above, each revertible by one commit (`2eeac8f`, `ed7988a`).
- Pre-existing quirks kept on purpose (research C-10): the success text of `/save` splices the name three times unescaped; `/clear` replies `notes_removed_success` even when the quoted form matched nothing; a lost insert race is replied as a success; the `/save` caption panic of research C-10 is left for `19-SPIKE.md`.
- `applyNoteRemoveByID`, `applyNoteAdd`'s `noteRaced` and `noteNameBlank` outcomes and the fresh readers have the Notes page of plan 19-09 as their only production user; each is exercised by a test of this plan.

## Known Stubs

None.

## Self-Check: PASSED
