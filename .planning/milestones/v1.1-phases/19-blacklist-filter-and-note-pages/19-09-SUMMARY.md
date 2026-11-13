---
phase: 19-blacklist-filter-and-note-pages
plan: 09
subsystem: settings-menu-notes-page
tags: [telegram, settings-menu, list-engine, two-step-add, notes, locales]
status: complete

requires:
  - phase: 19-blacklist-filter-and-note-pages
    provides: 19-08's applyNoteAdd, applyNoteRemoveByID, checkNoteName and the fresh note readers; 19-06's two-step engine; 19-07's generic read adapters and Apply hook that takes the sender's translator
provides:
  - the Notes page (page e, Order 72, icon 📝, PrivateOnly, settingsRightChangeInfoBotAdmin): list with an admins-only tag, paging, delete card, add through two typed steps (name, then content)
  - resolveNotePrivateConflict, the one pure helper /save and the page share for {private} plus {noprivate}
  - 14 settings_ strings in 8 locales (452 settings_ keys) with limits rows
  - Main at its final 18 grid cells (19 entries with Access)
affects: [19-10, 19-11, 19-12]

plan_head_before: 04f2c87b03f93491ba19d62f34051f1e50e79d08
plan_head_after: 1d102627ddefdc6eaf5e762e75222a485abaa401

actuals:
  tokens: 21700   # chars/4 over the added lines of 04f2c87..1d10262 outside .planning (86540 chars)
  tasks: 3
  commits: 3      # MEASURED: git rev-list --count 04f2c87..1d10262 (before this SUMMARY commit)

tech-stack:
  added: []
  patterns:
    - "a third list table needs no engine change: AdminTagKey already existed, so the Notes page is hooks and one registration"
    - "a shared pure helper (resolveNotePrivateConflict) keeps a command and its page from drifting on a rule"

key-files:
  created:
    - fuku/modules/settings_notes.go
    - fuku/modules/settings_notes_test.go
  modified:
    - fuku/modules/notes.go
    - fuku/modules/settings_blacklist_test.go
    - fuku/modules/settings_list_env_test.go
    - fuku/modules/settings_list_locale_test.go
    - fuku/modules/settings_main_test.go
    - fuku/modules/settings_phase18_test.go
    - fuku/i18n/settings_locale_test.go
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
  - "No engine change: the plan's settingsListDef.EntryTagKey already exists as AdminTagKey (19-03), so settings_list.go is untouched and the Notes table sets AdminTagKey."
  - "The private-flag conflict is resolved by resolveNotePrivateConflict in notes.go, called by addNote at the same place (the golden is byte-identical) and by the page's Apply, which returns /save's own notes_private_conflict_warning (leading blank lines trimmed) as the saved notice's extra line."
  - "Task 1's Check answered an existing name with an error that keeps the wait and Apply failed on noteExists/noteRaced (fail closed), until Task 2 gave them settings_notes_refuse_exists, as 19-07 did for Filters; the 449-key count of Task 1 depends on it."
  - "The Notes tests reuse the Filters flow through a small wrapper (settingsNotesFlow) and the live-creator helper settingsFiltersOwner instead of cloning them."

patterns-established:
  - "settingsNotesFlow wraps settingsFiltersFlow and overrides begin only"
  - "settingsNotesCommandTwin: the Filters twin chat plus an administrator bot, so /save runs"

requirements-completed: []

coverage:
  - id: D1
    description: "the owner adds a note from the private Notes page by typing the name, then the content; the stored row equals what /save NAME replying to that message stores (directives and resolved private flags included) and notes_list is dropped"
    requirement: "PAGE-09 (contribution only)"
    verification:
      - kind: unit
        ref: "TestSettingsListDoorTwoStepNotes, TestSettingsParityNoteAdd (text with entities, photo with {admin} caption, video, markdown buttons, both {private} and {noprivate}); mutation check: with the conflict helper bypassed both fail"
        status: pass
    human_judgment: false
  - id: D2
    description: "every refusal keeps or ends the wait as the contract says (photo, white space, leading #, over 100, blank and existing name at step 1; a poll at step 2; a name saved between the steps; expiry; the bot or the sender losing a right), and nothing is overwritten"
    requirement: "PAGE-09 (contribution only)"
    verification:
      - kind: unit
        ref: "TestSettingsListDoorNotesRefusals, TestSettingsListForged"
        status: pass
    human_judgment: false
  - id: D3
    description: "the page lists, pages and deletes through the engine with settingsRightChangeInfoBotAdmin on every write; another chat's row ID deletes nothing"
    requirement: "PAGE-09 / PAGE-10 (contribution only)"
    verification:
      - kind: unit
        ref: "TestSettingsNotesOpens, TestSettingsNotesDelete, TestSettingsParityNoteRemove, TestSettingsDriftNotes, TestSettingsAuthorityParityNotes (mutation check: with the right weakened to settingsRightAdmin the delete and refusal tests fail)"
        status: pass
    human_judgment: false
  - id: D4
    description: "14 strings in 8 locales under their ceilings (452 settings_ keys); Main has 18 grid cells; every Notes button is at most 64 bytes (the Yes exactly 64); every screen fits 3800 UTF-16 units in 8 languages; reads create no notes, notes_settings or chats row"
    requirement: "PLAT-05 / PAGE-10 (contribution only)"
    verification:
      - kind: unit
        ref: "TestSettingsLocaleKeys, TestSettingsLabelLengths, TestSettingsReusedKeyPlaceholders, make check-translations, TestSettingsListKeysExist, TestSettingsMainGrid, TestSettingsMainPhase18, TestSettingsPhase18Codes, TestSettingsPhase18Cost, TestSettingsListNotesCallbackBytes, TestSettingsListNotesPageFitsEveryLocale, TestSettingsNotesReadsCreateNothing"
        status: pass
    human_judgment: false
---

# Phase 19 Plan 09: the Notes page Summary

**The owner now manages the group's notes from the private settings menu: page through them with an admins-only tag, delete one behind a card, and add one by typing the name and then the content, stored and cached exactly as `/save` stores it, with a table of hooks and no flow code of its own. Main reaches its final 18 grid cells.**

## What was built

- **Task 1, the tracer (`25fbd8a`).** `settings_notes.go`: the read seams over `ListNoteEntriesFresh`, `GetNoteEntryFresh` and `CountNotesFresh` (built by the engine's generic adapters, so no read-adapter clone), the two-step hooks (`Normalise` is trimmed and lower-cased, `Check` is `checkNoteName`, `Apply` is `applyNoteAdd` with the engine's `ExtractTypedNoteOrFilter` result after `resolveNotePrivateConflict`) and one `registerSettingsList` call (key `e`, `Order` 72, icon 📝, codes `eg ex ey ea ec`, right `settingsRightChangeInfoBotAdmin`, `AdminTagKey`). `notes.go`: the two conflict lines of `addNote` became `resolveNotePrivateConflict` (called at the same place; `TestLegacyNoteRequests` passes unchanged). 11 strings in 8 locales (449), their limits rows, the 📝 grid cell and the Add button in the label-length test, Main's shape constants (`b|i / w|g / r|e / m|d / t|u / o|y`, 11 owner rows), `settingsMainTitleKeys` and `TestSettingsPhase18Codes` (`e`); `TestSettingsPhase18Cost` 18 to 19. AGENTS.md: the page list, the Main bullet, a Notes bullet, the key count. `TestSettingsNotesOpens`, `TestSettingsListDoorTwoStepNotes`.
- **Task 2 (`dfa53c1`).** `Remove` over `applyNoteRemoveByID`, the card and removed keys, the exists refusal at step 1 and the taken result at step 2 (`noteExists`, `noteRaced`), 3 more strings (452). `TestSettingsNotesDelete`, `TestSettingsListDoorNotesRefusals`; `TestSettingsListForged` (in `settings_blacklist_test.go`) also runs the Notes forged presses and the same presses from a group menu.
- **Task 3 (`1d10262`).** `TestSettingsParityNoteAdd`, `TestSettingsParityNoteRemove`, `TestSettingsDriftNotes`, `TestSettingsAuthorityParityNotes`, `TestSettingsListNotesCallbackBytes`, `TestSettingsListNotesPageFitsEveryLocale`, `TestSettingsNotesReadsCreateNothing`; AGENTS.md read against the code.

## Verification (real results)

- Task gates, bare and under `cienv.sh`: every test of the three plan verify blocks passes (`TestSettingsNotesOpens`, `TestSettingsListDoorTwoStepNotes`, `TestLegacyNoteRequests`, `TestSettingsListKeysExist`, `TestSettingsMainGrid`, `TestSettingsMainPhase18`, `TestSettingsPhase18Codes`, `TestSettingsPhase18Cost`, `TestSettingsDeletionOpensFromMain`, `TestSettingsDeletionRowBudget`, `TestSettingsMreqOpensFromMain`, `TestSettingsEditRegistry`, `TestSettingsNotesDelete`, `TestSettingsListDoorNotesRefusals`, `TestSettingsListForged`, the seven Task 3 tests, the 19-06 `TestSettingsListTwoStep*` and the 19-07 Filters door tests); `./fuku/i18n` `TestSettingsLocaleKeys`, `TestSettingsLabelLengths`, `TestSettingsReusedKeyPlaceholders`: ok; `make check-translations`: "All translations are present!"; `make check-docs`: no drift; `./fuku/db/notes`: ok bare and under `cienv.sh`. Each of the 8 locale files has 449 `settings_` lines after Task 1 and 452 after Task 2 (counted over the eight language files; `locales/config.yml` is the pseudo-locale and has none).
- Acceptance greps: `registerSettingsList(` once in `settings_notes.go`; `func resolveNotePrivateConflict(` once; the page reads no `GetNotes(` or `getNotesSettings`; `settingsMainShapeGroup` literal present; `applyNoteRemoveByID` 2 in the page; `UpdateNote|setNoteOverwriteCache` 0 in the page and the engine; the seven Task 3 test names each defined once.
- Mutation checks: weakening the page's right to `settingsRightAdmin` fails `TestSettingsNotesDelete` (bot-not-administrator and no-`can_change_info` cases) and the typed bot case of `TestSettingsListDoorNotesRefusals`; bypassing `resolveNotePrivateConflict` in the page's Apply fails `TestSettingsParityNoteAdd` (both-directives case) and `TestSettingsListDoorTwoStepNotes`; both were restored before the commits (`git diff` clean afterwards).
- Gates: `go vet -tags testtools ./fuku/modules ./fuku/i18n ./fuku/db/notes` clean; `CGO_ENABLED=0 go build ./...` clean; `gofmt -l` over the files this plan created or edited prints nothing; `lint-local.py 8c0c827` exits 0 ("no dupl, gocyclo or godox findings on lines added since 8c0c827", read from the exit code of a run with output redirected; its first run caught an unused test helper, moved to Task 2 where it is used); `git diff --exit-code 8c0c827 -- go.mod go.sum` exits 0; no `t.Skip`, `reflect.Ptr` or `ioutil` in the new code. `make lint`, gosec and `make test` were not run (the sandbox cannot run lint; `make test` is the closing plan's).
- Whole `fuku/modules` package under the CI environment (`go test -tags testtools -race -count=1 -timeout 40m -v ./fuku/modules`, background, log `/tmp/claude-0/-home-user-Fuku-Robot/4ce545b5-6832-57a3-a863-43c972932c7c/scratchpad/modules-19-09.log`): `ok  github.com/uasneppy/fuku_robot/fuku/modules  1979.930s`, last line `exit 0`, 1833 top-level PASS (395 of them `TestSettings*` or `TestLoadSettings*`), 0 `--- FAIL`, 0 `--- SKIP`, 0 `^FAIL`; `--- PASS: TestSettingsParityNoteAdd (0.48s)`, `TestSettingsParityFilterAdd`, `TestSettingsPhase18Cost` and `TestLegacyNoteRequests` are in it. The run is 1980 s against 1930 s for 19-07 (machine noise on top of the 9 s of new tests; short focused runs of mine overlapped its first minutes).
- Settings family: not run on its own; the whole-package run includes it (every `TestSettings*` and `TestLoadSettings*` function is in the package).

## Time added to the whole-package run

About 9 s of test time: `TestSettingsNotesOpens` 1.55 s, `TestSettingsNotesDelete` 1.55 s, `TestSettingsListNotesCallbackBytes` 1.37 s, `TestSettingsListNotesPageFitsEveryLocale` 1.20 s, `TestSettingsAuthorityParityNotes` 0.55 s, `TestSettingsListDoorNotesRefusals` 0.47 s, `TestSettingsParityNoteAdd` 0.46 s, `TestSettingsListDoorTwoStepNotes` 0.41 s, `TestSettingsParityNoteRemove` 0.41 s, `TestSettingsDriftNotes` 0.39 s, `TestSettingsNotesReadsCreateNothing` 0.37 s. The plan estimated 4 s; most of the difference is the fixed 0.3 s of one `newSettingsListEnv` per test and the 1 s that installing the real locales costs in three of them. No new test is over 1.6 s.

## Deviations from Plan

**1. [Structure] No `EntryTagKey` was added to the engine.** The plan lists `settings_list.go` and a new `EntryTagKey` field, but 19-03 already built that field as `AdminTagKey` (used by the line renderer and by `settingsListKeys`), so the Notes table sets it and `settings_list.go` is unchanged.
**2. [Detail the plan left open] The existing-name refusal and the taken result came in Task 2.** The plan puts `settings_notes_refuse_exists` (row 64) in Task 2's 452 count, so Task 1's `Check` answered an existing name with an error that keeps the wait and its `Apply` failed on `noteExists`/`noteRaced` (fail closed, nothing overwritten) until Task 2 gave them their words, as 19-07 did for Filters.
**3. [Detail] `TestSettingsListForged` (in `settings_blacklist_test.go`, not in the plan's file list) gained the Notes forged presses** (`settingsNotesForgedPresses`) and the Notes codes in its group-menu loop, because the plan says that test gains the Notes cases.
**4. [Environment] The sandbox disk filled up during Task 1** (`no space left on device` while linking the test binary). I removed old log files from my own scratchpad and the entries of the Go build cache that had not been used for 10 hours (`find /root/.cache/go-build -type f -mmin +600 -delete`, the same trimming Go does itself); nothing in the repository was touched. A request to delete the orphaned `/tmp/go-build*` directories was refused by the permission system and not retried.

No auth gates.

## Known Stubs

None.

## Threat Flags

None new. T-19-43 (a typed step or press after losing the rights `/save` needs): `settingsRightChangeInfoBotAdmin` on every press and typed message, the bot half classified live in the door, proved by `TestSettingsAuthorityParityNotes`, `TestSettingsListDoorNotesRefusals` and the mutation check. T-19-44 (content stored with flags or links `/save` would not store): `ExtractTypedNoteOrFilter` through the engine and the shared `resolveNotePrivateConflict`, `TestSettingsParityNoteAdd`. T-19-45 (another chat's row ID, overwriting a note): delete by `(id, chat_id)`, an existing name refused at both steps, no `UpdateNote` in page code (grep clean), `TestSettingsNotesDelete`, `TestSettingsListDoorNotesRefusals`. T-19-46 (a name in a group menu, label, callback data or log line): `PrivateOnly`, numbered buttons, escaped body text only, IDs-only logs (the apply functions and the engine log IDs only). T-19-47 (a `notes_settings` row created): `TestSettingsNotesReadsCreateNothing`. T-19-SC: `git diff --exit-code 8c0c827 -- go.mod go.sum` exits 0.

## Requirements

None completed: `requirements-completed: []`. PAGE-09 (this plan completes the capabilities of the Notes page: list, page, delete and the two-step add; the closing plan 19-12 completes the requirement after the end-to-end test), PAGE-10 and PLAT-05 stay Pending, and nothing was ticked in `REQUIREMENTS.md`.

## Self-Check: PASSED

- Files exist: `fuku/modules/settings_notes.go`, `fuku/modules/settings_notes_test.go`.
- Commits reachable from HEAD: `25fbd8a`, `dfa53c1`, `1d10262`.
