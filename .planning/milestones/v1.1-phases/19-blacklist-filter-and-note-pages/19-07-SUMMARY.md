---
phase: 19-blacklist-filter-and-note-pages
plan: 07
subsystem: settings-menu-filters-page
tags: [telegram, settings-menu, list-engine, two-step-add, filters, locales]
status: complete

requires:
  - phase: 19-blacklist-filter-and-note-pages
    provides: 19-05's applyFilterAdd, applyFilterRemoveByID, the fresh filter readers and content.ExtractTypedNoteOrFilter; 19-06's two-step engine (settingsListTwoStep, AddGate, the q guard)
provides:
  - the Filters page (page i, Order 47, icon 🔁, PrivateOnly, settingsRightChangeInfo): list, paging, delete card, add through two typed steps, the cap of 150
  - generic list-table read adapters (settingsListReader, settingsListEntryReader, settingsListCounter)
  - 14 settings_ strings in 8 locales (438 settings_ keys) with limits rows
  - Main at 17 grid cells (18 entries with Access)
affects: [19-08, 19-09, 19-10, 19-11, 19-12]

plan_head_before: a7fde07c269f73cf7650826717aa35b507878d47
plan_head_after: 058c6841169496cbddb166ef754937c9c52e08cd

actuals:
  tokens: 23400   # chars/4 over the added lines of a7fde07..058c684 outside .planning (93684 chars)
  tasks: 3
  commits: 4      # MEASURED: git rev-list --count a7fde07..058c684 (before this SUMMARY commit)

tech-stack:
  added: []
  patterns:
    - "a table that reads a repository gives its row type and one entryOf function to settingsListReader / settingsListEntryReader / settingsListCounter"
    - "a two-step page's Apply hook takes the sender's translator, so a taken result words its own reason"

key-files:
  created:
    - fuku/modules/settings_filters.go
    - fuku/modules/settings_filters_test.go
  modified:
    - fuku/modules/settings_list.go
    - fuku/modules/settings_list_input.go
    - fuku/modules/settings_list_door_test.go
    - fuku/modules/settings_list_env_test.go
    - fuku/modules/settings_list_locale_test.go
    - fuku/modules/settings_blacklist_test.go
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
  - "Apply gets the sender's translator (settingsListTwoStep.Apply gained a tr argument): a taken result (a keyword stored between the steps, the cap reached meanwhile) needs a finished, translated reason with the keyword spliced in, and the hook had no translator."
  - "The Filters Check is checkFilterKeyword (blank, over 100, too broad, exists, cap, failed lookup) so /filter and the page refuse alike; a failed lookup is an error that keeps the wait."
  - "The Add cap gate redraws page 1 with the cap notice and creates no wait; a count that cannot be read answers the cannot-open alert and edit (the gate returns the cannot-open screen, because a nil return means 'go on')."
  - "The three read seams come from generic engine adapters, because the first version of the Filters table was a near copy of the Blacklist's and the dupl check caught it."

patterns-established:
  - "settingsFiltersFlow (newSettingsFiltersFlow, begin, say, reply, waitIs, endWait) and settingsFiltersOwner: a live creator holds can_change_info, so a Filters test uses one instead of env.admin, who lacks it"
  - "settingsFiltersCommandTwin: a second chat with the same creator and the filter commands loaded, to prove the page equals /filter and /stop"

requirements-completed: []

coverage:
  - id: D1
    description: "the owner adds a filter from the private Filters page by typing the keyword, then the reply; the stored row equals what /filter K replying to that message stores, and both filter cache keys are dropped"
    requirement: "PAGE-08 (contribution only)"
    verification:
      - kind: unit
        ref: "TestSettingsListDoorTwoStep, TestSettingsParityFilterAdd (text with entities and newlines, photo with caption, sticker, markdown buttons with a javascript: link)"
        status: pass
    human_judgment: false
  - id: D2
    description: "every refusal keeps or ends the wait as the contract says (blank, over 100, too broad, exists at step 1; over 4096 and a poll at step 2; a keyword taken or the cap reached between the steps; expiry), Add at the cap creates no wait, and nothing is overwritten"
    requirement: "PAGE-08 (contribution only)"
    verification:
      - kind: unit
        ref: "TestSettingsListDoorTwoStepRefusals, TestSettingsListDoorTwoStepCap; mutation check: with the Add gate disabled the cap test fails"
        status: pass
    human_judgment: false
  - id: D3
    description: "the page lists, pages and deletes through the engine with settingsRightChangeInfo on every write; another chat's row ID deletes nothing; forged presses are refused in step 1"
    requirement: "PAGE-08 / PAGE-10 (contribution only)"
    verification:
      - kind: unit
        ref: "TestSettingsFiltersOpens, TestSettingsFiltersDelete, TestSettingsParityFilterRemove, TestSettingsDriftFilters, TestSettingsAuthorityParityFilters (mutation check: with the right weakened to settingsRightAdmin the authority and delete tests fail), TestSettingsListForged"
        status: pass
    human_judgment: false
  - id: D4
    description: "14 strings in 8 locales under their ceilings (438 settings_ keys); Main has 17 cells; every Filters button is at most 64 bytes (the Yes exactly 64); every screen fits 3800 UTF-16 units in 8 languages; reads create nothing"
    requirement: "PLAT-05 / PAGE-10 (contribution only)"
    verification:
      - kind: unit
        ref: "TestSettingsLocaleKeys, TestSettingsLabelLengths, TestSettingsReusedKeyPlaceholders, make check-translations, TestSettingsListKeysExist, TestSettingsMainGrid, TestSettingsMainPhase18, TestSettingsPhase18Codes, TestSettingsListFiltersCallbackBytes, TestSettingsListFiltersPageFitsEveryLocale, TestSettingsFiltersReadsCreateNothing"
        status: pass
    human_judgment: false
---

# Phase 19 Plan 07: the Filters page Summary

**The owner now manages the group's filters from the private settings menu: page through them, delete one behind a card, and add one by typing the keyword and then the reply, stored and cached exactly as `/filter` stores it, with a table of hooks and no flow code of its own.**

## What was built

- **Task 1, the tracer (`e91bd04`).** `settings_filters.go`: the read seams over `ListFilterEntriesFresh`, `GetFilterEntryFresh` and `CountFiltersContext`, the two-step hooks (`Normalise` is the words joined by one space and lower-cased, `Check` is `checkFilterKeyword`, `Apply` is `applyFilterAdd` with the engine's `ExtractTypedNoteOrFilter` result) and one `registerSettingsList` call (key `i`, `Order` 47, icon 🔁, codes `ig ix iy ia ic`, right `settingsRightChangeInfo`, `CountMax` 150). `settingsListTwoStep.Apply` gained the sender's translator. 10 strings in 8 locales (434), their limits rows, the `🔁` grid cell and the Add button in the label-length test, Main's shape constants (`b|i / w|g / r|m / d|t / u|o / y`, 11 owner rows), `settingsMainTitleKeys`, `settingsMainWorstState` (`99999/150`) and `TestSettingsPhase18Codes` (`i`). AGENTS.md: the Callbacks page list, the Main bullet, the Filters bullet and the key count. `TestSettingsFiltersOpens`, `TestSettingsListDoorTwoStep`.
- **Task 2 (`6bfc857`).** `Remove` over `applyFilterRemoveByID`, the card and removed keys, the exists and cap refusals at both steps (and a taken `Apply` for the lost-race and cap cases), the Add gate (`settingsFiltersAddGate`), 4 more strings (438). `TestSettingsFiltersDelete`, `TestSettingsListDoorTwoStepRefusals`, `TestSettingsListDoorTwoStepCap`; `TestSettingsListForged` also runs the Filters forged presses (`settingsFiltersForgedPresses`) and the same presses from a group menu.
- **Task 3 (`008e715`).** `TestSettingsParityFilterAdd`, `TestSettingsParityFilterRemove`, `TestSettingsDriftFilters`, `TestSettingsAuthorityParityFilters`, `TestSettingsListFiltersCallbackBytes`, `TestSettingsListFiltersPageFitsEveryLocale`, `TestSettingsFiltersReadsCreateNothing`; the generic read adapters in `settings_list.go` (the lint stand-in's `dupl` flagged the first Filters read seams as a clone of the Blacklist's); AGENTS.md read against the code.
- **Fix (`058c684`).** The whole-package run found `TestSettingsPhase18Cost` counting 17 Main entries; it is 18 now.

## Verification (real results)

- Task gates, bare and under `cienv.sh`: every test of the three plan verify blocks passes (`TestSettingsFiltersOpens`, `TestSettingsListDoorTwoStep`, `TestSettingsListDoorTwoStepRefusals`, `TestSettingsListDoorTwoStepCap`, `TestSettingsFiltersDelete`, `TestSettingsListForged`, `TestSettingsListTwoStepFlow`, `TestSettingsListTwoStepBranches`, `TestSettingsListKeysExist`, the Main and Phase 18 tests, the seven Task 3 tests, `TestLegacyFilterRequests`); `./fuku/i18n` `TestSettingsLocaleKeys`, `TestSettingsLabelLengths`, `TestSettingsReusedKeyPlaceholders`: ok; `make check-translations`: "All translations are present!". Each of the 8 locale files has 434 `settings_` lines after Task 1 and 438 after Task 2.
- Acceptance greps: `registerSettingsList(` once in `settings_filters.go`; `settingsInputAdvance|listStepOne|listStepTwo` 0 in it; `settingsMainOwnerRows    = 11` 1; `applyFilterRemoveByID` 2; `UpdateFilter|setFilterOverwriteCache` 0 in the page and the engine; the seven Task 3 test names each defined once.
- Mutation checks: weakening the page's right to `settingsRightAdmin` fails `TestSettingsAuthorityParityFilters` and `TestSettingsFiltersDelete`; disabling the cap gate fails `TestSettingsListDoorTwoStepCap`; both were restored before the commits.
- Gates: `go vet -tags testtools ./fuku/modules ./fuku/i18n` clean; `CGO_ENABLED=0 go build ./...` clean; `gofmt -l` over the files this plan created or edited prints nothing; `lint-local.py 8c0c827` exits 0 ("no dupl, gocyclo or godox findings on lines added since 8c0c827", read from the exit code of a run with output redirected); `git diff --exit-code 8c0c827 -- go.mod go.sum` exits 0; no `t.Skip`, `reflect.Ptr` or `ioutil` in the new code. `make lint`, gosec and `make test` were not run (the sandbox cannot run lint; `make test` is the closing plan's).
- Whole `fuku/modules` package under the CI environment (`go test -tags testtools -race -count=1 -timeout 40m -v ./fuku/modules`, background, log `/tmp/claude-0/-home-user-Fuku-Robot/4ce545b5-6832-57a3-a863-43c972932c7c/scratchpad/modules-19-07.log`): `ok  github.com/uasneppy/fuku_robot/fuku/modules  1929.562s`, last line `exit 0`, 1815 top-level PASS, 0 `--- FAIL`, 0 `--- SKIP`, 0 `^FAIL`; `--- PASS: TestSettingsParityFilterAdd (0.45s)` and `--- PASS: TestSettingsListTwoStepFlow (0.41s)` are in it. The first whole run (log `modules-19-07-first.log`, before the fix commit) had exactly one failure, `TestSettingsPhase18Cost/Main_makes_no_Telegram_call_of_its_own` ("Main has 18 entries, want 17"), fixed in `058c684` and the whole package re-run in full.
- Settings family under the CI environment (`go test -tags testtools -race -count=1 -timeout 40m -run '^(TestSettings|TestLoadSettings)' ./fuku/modules ./fuku/i18n`, log `family-19-07.log`): `ok fuku/modules 758.116s`, `ok fuku/i18n 7.551s`, `exit 0`.

## Time added to the whole-package run

About 9 s of test time: `TestSettingsFiltersOpens` 1.33 s, `TestSettingsFiltersDelete` 1.45 s, `TestSettingsListFiltersCallbackBytes` 1.26 s, `TestSettingsListFiltersPageFitsEveryLocale` 1.31 s, the three door tests 1.40 s together, the parity, drift, authority and reads tests 2.1 s together. The plan estimated 3 s; most of the difference is the fixed 0.3 s of one `newSettingsListEnv` per test (13 tests). No new test is over 1.5 s. The whole package took 1929 s here against 1814 s in 19-06 (machine noise on top of the 9 s).

## Deviations from Plan

**1. [Rule 3 - Blocking] `settingsListTwoStep.Apply` takes the sender's translator.** The 19-06 hook had no translator, but a taken result (a keyword stored between the steps, the cap reached meanwhile) must carry a finished, translated reason with the keyword spliced in. `settings_list_input.go` and the test store of `settings_list_env_test.go` changed by one argument; every 19-06 test passes unedited.
**2. [Structure] The Add cap gate and the exists/cap keys came in Task 2, not Task 1.** The plan's string counts (434 after Task 1, 438 after Task 2) put `settings_flt_refuse_exists` and `settings_flt_refuse_cap` in Task 2, so Task 1's `Check` answered those two outcomes with an error that keeps the wait (fail closed, no overwrite) until Task 2 gave them their words. Task 1's tree names no key missing from the locale files.
**3. [Rule 1 - Bug found by the whole-package run] `TestSettingsPhase18Cost` hard-coded 17 Main entries** (`settings_phase18_test.go`, in the plan's file list): now 18 (17 grid pages and Access).
**4. [Rule 3] `dupl` flagged the Filters read seams as a clone of the Blacklist's.** Added `settingsListReader`, `settingsListEntryReader` and `settingsListCounter` to `settings_list.go` and built the Filters seams with them; the Blacklist file is untouched (a later plan may move it onto them).
**5. [Detail the plan left open]** `TestSettingsListForged` (in `settings_blacklist_test.go`, not in the plan's file list) gained a loop over `settingsFiltersForgedPresses` and the Filters codes in its group-menu loop, because the plan says that test gains the Filters cases. A Filters test uses a live creator (`settingsFiltersOwner`), not `env.admin`, who has no `can_change_info`.

No auth gates.

## Known Stubs

None.

## Threat Flags

None new. T-19-32 (overwriting a filter): an existing keyword is refused at step 1, re-checked at step 2 and a lost insert race ends the wait; no page code calls `UpdateFilter` (grep clean; `TestSettingsListDoorTwoStepRefusals` shows the stored reply unchanged). T-19-33 (a press or typed step after losing `can_change_info`): `settingsRightChangeInfo` on every press and typed step, proved by `TestSettingsAuthorityParityFilters` and the mutation check. T-19-34: `ExtractTypedNoteOrFilter` through the engine, `TestSettingsParityFilterAdd`. T-19-35: delete by `(id, chat_id)`, `TestSettingsFiltersDelete` (another chat's ID), `TestSettingsListForged`. T-19-37: `TestSettingsListFiltersCallbackBytes` (Yes exactly 64, `Omitted` 0, 1 for an 18-digit ID).

## Requirements

None completed: `requirements-completed: []`. PAGE-08 (this plan completes its Filters capabilities, the closing plan 19-12 completes the requirement after the end-to-end test), PAGE-10 and PLAT-05 stay Pending.

## Self-Check: PASSED

- Files exist: `fuku/modules/settings_filters.go`, `fuku/modules/settings_filters_test.go`.
- Commits reachable from HEAD: `e91bd04`, `6bfc857`, `008e715`, `058c684`.
