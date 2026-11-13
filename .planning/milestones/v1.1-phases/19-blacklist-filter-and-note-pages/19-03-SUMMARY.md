---
phase: 19-blacklist-filter-and-note-pages
plan: 03
subsystem: settings-menu-list-pages
tags: [telegram, settings-menu, list-engine, blacklist, private-only, delete-card, action-screen, tracer]
status: complete

requires:
  - phase: 19-blacklist-filter-and-note-pages
    provides: 19-01's blacklist apply functions and fresh readers, 19-02's PrivateOnly, settingsCardRowsTo and the ninth right
provides:
  - settings_list.go, the list engine (settingsListDef table, fresh chat-scoped read, order and paging, list and card renderers, pager, delete ask and Yes by row ID)
  - the Blacklist page (page b, Order 46, icon 🔤): list, pager bp, delete card (bx, by) and action screen (bo, bs), PrivateOnly
  - 17 settings_ strings in 8 locales (404 settings_ keys), limits and label-length rows, TestSettingsListKeysExist
  - Main at 16 cells with the shape constants updated
affects: [19-04, 19-05, 19-06, 19-07, 19-08, 19-09, 19-10, 19-11, 19-12]

plan_head_before: 29c166d1e179d2bafbbb518c2715e1aa79b77f26
plan_head_after: 1a007b5ac87b171f3a97002ca0de16686d89a208

actuals:
  tokens: 35200   # chars/4 over the added lines of 29c166d..1a007b5 outside .planning (140922 chars, locales and AGENTS.md included)
  tasks: 4
  commits: 5      # MEASURED: git rev-list --count 29c166d..1a007b5 (before this SUMMARY commit)

tech-stack:
  added: []
  patterns:
    - "one list engine and three tables: the page is built from a settingsListDef, no renderer or gate asks which page it serves"
    - "entries are numbered body lines, never button labels; a delete button carries the row ID and the page only"
    - "a notice refusal redraws through the page's Open, which chooses its screen from the press (ask page, page holding r, a table Redraw entry)"

key-files:
  created:
    - fuku/modules/settings_list.go
    - fuku/modules/settings_blacklist.go
    - fuku/modules/settings_list_env_test.go
    - fuku/modules/settings_list_test.go
    - fuku/modules/settings_blacklist_test.go
    - fuku/modules/settings_list_locale_test.go
  modified:
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
  - "The pager code is bp, not the UI-SPEC's bg: bg is the Deletion page's Bot goodbye row code, and TestSettingsDeletionMemberCodesUnique keeps the section and row codes apart from every action code. The same table rules out ba (Bot auto) for the Blacklist's Add button, which 19-04 must name differently."
  - "The engine's delete ask and Yes landed in task 1's file with the rest of the engine but are wired to the Blacklist table only in task 2 (AskCode, YesCode, Right and Remove), so the task-1 page shows no delete row."
  - "The Action button (settings_btn_bl_action, a task-3 string) is added in task 3, so the task-1 and task-2 list pages have no add row; the {4,4,2,1,2} shape of the plan holds from task 3."
  - "The action choices are built by a local row helper pressing bs, not settingsChoiceRow, because that helper's fields are the control press st."

patterns-established:
  - "settingsListEnv toolkit: newSettingsListEnv, settingsListSeedBlacklist(+Action), settingsListCleanup, settingsListOpenPrivate, settingsListSentinel, plus the page's menu helper settingsBlacklistMenu in settings_blacklist_test.go"

requirements-completed: []

coverage:
  - id: D1
    description: "the Blacklist page lists the words of a private menu's group, eight a page, numbered globally, cut and escaped, with a pager; a group menu shows only the framework note and reads nothing"
    requirement: "PAGE-07 (contribution only)"
    verification:
      - kind: unit
        ref: "TestSettingsListRender, TestSettingsBlacklistOpens, TestSettingsListPager, TestSettingsListGroupNote (bare and with cienv.sh)"
        status: pass
    human_judgment: false
  - id: D2
    description: "a word is deleted by its row ID behind a card that binds the row, refused where /addblacklist refuses, leaving the commands' rows and cache state"
    requirement: "PAGE-10 (contribution only)"
    verification:
      - kind: unit
        ref: "TestSettingsBlacklistDelete, TestSettingsListForged, TestSettingsParityBlacklistRemove"
        status: pass
    human_judgment: false
  - id: D3
    description: "the action screen never claims one action unless every word has it, and every choice is stored through applyBlacklistAction"
    requirement: "PAGE-10 (contribution only)"
    verification:
      - kind: unit
        ref: "TestSettingsBlacklistAction, TestSettingsParityBlacklistAction, TestSettingsDriftBlacklist, TestSettingsAuthorityParityBlacklist"
        status: pass
    human_judgment: false
  - id: D4
    description: "every button at the worst IDs is at most 64 bytes with the UI-SPEC's exact sizes, every screen fits in 8 languages, 17 strings in 8 locales, Main at 16 cells"
    requirement: "PLAT-05 (contribution only)"
    verification:
      - kind: unit
        ref: "TestSettingsListBlacklistCallbackBytes, TestSettingsListBlacklistPageFitsEveryLocale, TestSettingsBlacklistReadsCreateNothing, TestSettingsListKeysExist, TestSettingsLocaleKeys, TestSettingsLabelLengths, make check-translations"
        status: pass
    human_judgment: false
---

# Phase 19 Plan 03: the list engine and the Blacklist page Summary

**An administrator opens a Blacklist page in the private menu, pages through the group's words eight at a time, deletes one behind a two-tap card that binds the row, and sets the action every word carries, shown as Mixed when the words differ; a menu opened in the group shows only the note and reads nothing.**

## What was built

- **Task 1, the engine and the page (`3ce940f`).** `settings_list.go`: the `settingsListDef` table type, `settingsListPage` (pure) and `registerSettingsList`, the fresh chat-scoped read, `settingsListSort` (lower-cased text in code point order, ties by row ID), `settingsListLabel` (bidi controls stripped, whitespace collapsed, cut at 32 code points, escaped after the cut), `settingsListParseID` (1 to 17 digits), `renderSettingsList` (numbered body lines, page line, delete rows of four, pager, the table's add row, navigation), `renderSettingsListCard`, and the ask, Yes and pager runners. `settings_blacklist.go`: the three seams (`settingsBlacklistRead`, `settingsBlacklistEntry`, `settingsBlacklistCount`, adapting 19-01's fresh readers), the action line (one action only when every word has it, else Mixed) and the registration. Seven strings in 8 locales; Main at 16 cells and the shape constants, `settingsMainTitleKeys`, `settingsMainWorstState` and the page set of `TestSettingsPhase18Codes` updated to the real shape.
- **Task 2, the delete (`5c59d47`).** The table gains `bx`, `by`, the right `settingsRightRestrictBotRestrict`, `settingsBlacklistRemove` (an adapter over `applyBlacklistRemoveByID`) and the card and notice keys. The card names the word (escaped, cut at 100), Yes carries the row ID and the expiry (exactly 64 bytes at a 17-digit ID), No returns to the page the ask came from through `settingsCardRowsTo`. A gone, foreign or re-added row answers one notice with no card and no delete. Three strings.
- **Task 3, the action (`8aff18d`).** `bo` (action screen) and `bs` (set), the Set action button on a list with words, count lines for the eight values the CHECK allows (`none` worded Delete only, `tban`, `tmute`, `delete` as stored), the inert line, and a `Redraw` entry so a notice refusal of `bs` redraws the action screen. Seven strings.
- **Task 4, the worst cases (`e87fdc6`).** `TestSettingsListBlacklistCallbackBytes` (a 79,992-entry list, 9,999 pages, 17-digit IDs: every size equals the UI-SPEC table), `TestSettingsListBlacklistPageFitsEveryLocale` (8 languages, 64-rune title, 32-ampersand entries, the two longest notices, eight count lines, a 100-code-point card, row widths), `TestSettingsBlacklistReadsCreateNothing`.
- **`1a007b5`**: a comment reworded so the acceptance grep for the hiding getter's name is 0.
- AGENTS.md (in the task commits): the Callbacks bullet (page `b`, codes `bp bx by bo bs`), the Main bullet (16 cells, 17 state lines, the Blacklist line), a new list-engine and Blacklist bullet (read, order, paging, delete, action screen, the worst-case tests), the Testing key count (404) and toolkit.

## Verification (real results)

- Task 1 gate: `go test -tags testtools -race -count=1 -run '^(TestSettingsListRender|TestSettingsBlacklistOpens|TestSettingsListPager|TestSettingsListGroupNote|TestSettingsListKeysExist|TestSettingsPrivateOnlyPage|TestSettingsMainGrid|TestSettingsMainPhase18|TestSettingsPhase18Codes|TestSettingsDeletionOpensFromMain|TestSettingsDeletionRowBudget|TestSettingsMreqOpensFromMain|TestSettingsEditRegistry)$' ./fuku/modules`: ok; `./fuku/i18n` (`TestSettingsLocaleKeys`, `TestSettingsLabelLengths`, `TestSettingsReusedKeyPlaceholders`): ok; `make check-translations`: "All translations are present"; the CI-environment run (`cienv.sh`) of the page tests: ok.
- Task 2 and 3 gates (bare and `cienv.sh`): `TestSettingsBlacklistDelete`, `TestSettingsListForged`, `TestSettingsParityBlacklistRemove`, `TestSettingsRights`, `TestLegacyBlacklistRequests`, `TestSettingsBlacklistAction`, `TestSettingsParityBlacklistAction`, `TestSettingsDriftBlacklist`, `TestSettingsAuthorityParityBlacklist`: ok.
- Whole `fuku/modules` package under the CI environment (`go test -tags testtools -race -count=1 -timeout 40m -v ./fuku/modules`, background, log `/tmp/claude-0/-home-user-Fuku-Robot/4ce545b5-6832-57a3-a863-43c972932c7c/scratchpad/modules-19-03.log`): `ok  github.com/uasneppy/fuku_robot/fuku/modules  1849.270s`, `exit 0`, 1783 top-level PASS (19-02's 1768 plus this plan's 15), 0 FAIL, 0 SKIP. `--- PASS: TestSettingsListBlacklistCallbackBytes (1.49s)` and `--- PASS: TestLegacyBlacklistRequests (1.22s)` are in it.
- Settings family under the CI environment (`go test -tags testtools -race -count=1 -timeout 40m -run '^(TestSettings|TestLoadSettings)' ./fuku/modules ./fuku/i18n`, log `.../scratchpad/family-19-03.log`): `ok fuku/modules 698.431s`, `ok fuku/i18n 7.228s`, `exit 0`. An earlier family run on the task-1 code (before the code collision below was fixed) failed exactly one test, `TestSettingsDeletionMemberCodesUnique`.
- Gates: `go vet -tags testtools ./fuku/modules ./fuku/i18n` clean; `CGO_ENABLED=0 go build ./...` clean; `gofmt -l` over every Go file this plan created or edited prints nothing; `lint-local.py 8c0c827` exits 0 ("no dupl, gocyclo or godox findings") after each task; `git diff --exit-code 8c0c827 -- go.mod go.sum` exits 0; `make check-docs`: no drift; no `t.Skip` in the new tests; `git diff --name-only d93ec1d -- migrations fuku/db/models main.go` lists only 19-01's migration.
- Acceptance greps: `registerSettingsList(` in `settings_blacklist.go` 1; `func settingsListPage(` 1; the group Main shape constant 1; every locale file 404 `settings_` lines; `GetBlacklistSettingsContext` in the two files 0; `settingsCardRowsTo(` in `settings_list.go` 1; `applyBlacklistRemoveByID` in `settings_blacklist.go` 2; `applyBlacklistAction(` 1; `SetBlacklistAction` in the page files 0; each named test function exactly once.
- Not run: `make lint` and gosec (cannot run in the sandbox), `make test` (only the closing plan), PostgreSQL (no database change in this plan), a phone check of the screens (A19-A10, a live check for 19-SPIKE.md).

## Time added to the fuku/modules run

About 16 s: `TestSettingsBlacklistDelete` 2.99 s, `TestSettingsBlacklistAction` 2.80 s, `TestSettingsBlacklistOpens` 1.53 s, `TestSettingsListBlacklistCallbackBytes` 1.49 s, `TestSettingsListBlacklistPageFitsEveryLocale` 1.28 s, `TestSettingsListKeysExist` 1.16 s, `TestSettingsListRender` 1.16 s, and eight tests of 0.33 to 0.50 s. No new test is over 10 s. The plan estimated about 5 s: the nine press tests each build a real settings environment (about 0.35 s each), and three tests install the real locale files (about 1 s each). The whole-package run measured 1849 s (19-02's 1780 s on the same machine; the difference is mostly run-to-run noise).

## Deviations from Plan

1. **[Rule 1 - Bug in the UI-SPEC] The pager code is `bp`, not `bg`.** The UI-SPEC's collision check missed the Deletion page's bot row codes (`bw bg br ba bm`, `deletion_kinds.go`). `bg` (the pager) collided with Bot goodbye, and the task-1 family run failed `TestSettingsDeletionMemberCodesUnique` ("the code bg is both row bg and action bg"). That test is a Phase 17 guard and was not touched. The pager is `bp` (same 40 bytes), every test and AGENTS.md say so. **19-04 must not use `ba` for the Blacklist's Add** (it is Bot auto; `bn` is free, checked against every action, control, section and row code: the free set also holds `bc`, `ig ix iy ia ic` and `eg ex ey ea ec`, so Filters and Notes are unaffected).
2. **Engine ask and Yes arrive in task 1's commit.** The plan splits them across tasks 1 and 2; I wrote the engine in one piece, so `settingsListRunAsk` and `settingsListRunYes` are in `3ce940f` and are wired to the Blacklist only in `5c59d47`. Their tests are in task 2. No behaviour reached a user between the two commits.
3. **No delete row and no add row in the task-1 page.** The plan's task 1 behaviour mentions delete rows on the Blacklist page, but its own Artifacts note says the codes arrive in task 2, and the Action button's string `settings_btn_bl_action` is a task-3 key (acceptance counts 394 after task 1). The Blacklist page therefore had neither until tasks 2 and 3; `TestSettingsListRender` holds the full keyboard through a table copy that has the codes.
4. **Action choices do not use `settingsChoiceRow`** (the plan says to): that helper builds the control press `st`, and the screen needs `bs` with `v`. A local `settingsBlacklistChoiceRow` marks the selected value the same way (`✅ `).
5. **Forged-press test for IDs that do not encode.** The test chats have 19-digit IDs, so a forged `r` of 18 digits cannot be encoded into callback data. Those cases are checked against `settingsCheckFields` itself with a realistic 14-character chat ID (Telegram would never deliver them); every other case is a real press with no `getChatMember`, `getChat` or edit.
6. **Existing test edits** (the plan's list): `settingsMainShapeGroup/Private`, `settingsMainTitleKeys`, `settingsMainWorstState` (adds `In private` and `99999`), the page set of `TestSettingsPhase18Codes`, and, found by running the settings tests, the `len(entries) != 16` of `TestSettingsPhase18Cost`, now 17 (a real shape change). Each is the new real shape, nothing loosened. `settingsMainOwnerRows` stays 10 (16 cells are eight full rows, Access, navigation).
7. **Cleanup covers `blacklists` and `filters` only** (the plan says also `notes`): the module harness does not migrate the notes table yet; 19-08 adds it with its model.
8. No golden was touched, no door file, no migration, model, Redis key, handler group or shutdown change.

## Notes for later plans

- 19-04: the Add code must not be `ba`; `AddRowExtra` is `settingsBlacklistAddRow` (it returns the Action button only; Add goes first in the same row); the door reads `settingsRightBotAfter(..., true)` and the table's `Redraw` map decides what a notice refusal of Add redraws.
- 19-05 and 19-08: a table is `settingsListDef` plus one `registerSettingsList` call; `AdminTagKey`, `CountMax` and `BodyExtra` already work and are tested through a synthetic table. Only the Blacklist table sets `Redraw` and `Actions`.
- `requirements-completed: []`: this plan contributes to PAGE-07, PAGE-10 and PLAT-05 and completes none; only 19-12 completes PAGE-07 to PAGE-09.

## Known Stubs

None.

## Threat Flags

None: no endpoint, auth path or trust boundary was added; T-19-10 to T-19-15 are mitigated by the tests above (group menu leak: `TestSettingsListGroupNote`; forged and foreign row IDs: `TestSettingsBlacklistDelete`, `TestSettingsListForged`; rights: `TestSettingsAuthorityParityBlacklist`; display: `TestSettingsListRender`; bytes: `TestSettingsListBlacklistCallbackBytes`; honest action and reads: `TestSettingsBlacklistAction`, `TestSettingsBlacklistReadsCreateNothing`).

## Self-Check: PASSED

- Files exist: `fuku/modules/settings_list.go`, `settings_blacklist.go`, `settings_list_env_test.go`, `settings_list_test.go`, `settings_blacklist_test.go`, `settings_list_locale_test.go`; the 17 keys are in all 8 locale files (404 `settings_` lines each).
- Commits are ancestors of HEAD: `3ce940f`, `5c59d47`, `8aff18d`, `e87fdc6`, `1a007b5`.
