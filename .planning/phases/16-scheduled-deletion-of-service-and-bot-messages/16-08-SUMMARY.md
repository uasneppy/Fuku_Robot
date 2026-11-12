---
phase: 16-scheduled-deletion-of-service-and-bot-messages
plan: 08
subsystem: settings-menu
tags: [deletion, settings-menu, hidden-page, delete-right-line, staff-group, tracer]
status: complete

requires:
  - phase: 16-scheduled-deletion-of-service-and-bot-messages
    provides: "16-02 row tables, applyDeleteRule and applyCleanServiceRule; 16-03 deletionStaffLookup; 16-04 deletionReadBotRight and deletionPaced; 16-07 the 57 locale keys and limits"
provides:
  - "the Deletion page (page d, Order 85): hub, Service and Bot lists, one screen per row (17), preset choices through the apply functions, MainState, Open, Hidden, strict validators, the delete-right line and the Staff Group gate (fuku/modules/settings_deletion.go)"
  - "settingsPage.Hidden (optional): skipped in settingsMainEntries without calling MainState and refused in openPage with common_callback_invalid_request (the only edits to settings_pages.go and settings_menu.go, 6 and 5 lines)"
  - "tests: TestSettingsDeletion{OpensFromMain,MainState,Writes,Hidden,StaffGroup,RightLine,Cost,Validation,CallbackBytes,RowBudget,States,NoCard,StaleState,ReadsAndWritesFail,PageFitsEveryLocale}, TestSettingsAuthorityDeletion, TestSettingsDriftDeletion, TestSettingsParityCleanService, the extended TestSettingsControlActionAllowList"
affects: [16-09]

plan_head_before: 8c66a96dd0d04cad49762fe527a856760e715fb2
plan_head_after: ea7acac1d6158ab6fd780094d3e25a7f4fb3443e

actuals:
  tokens: 26000   # chars/4 over the added lines of 8c66a96..ea7acac (104221 chars: one page file, five test files, AGENTS.md)
  tasks: 3
  commits: 5      # MEASURED: git rev-list --count 8c66a96..HEAD (before this SUMMARY commit)

tech-stack:
  added: []
  patterns:
    - "a page drawn from two tables (sections and row tables) with one state function; nothing branches on a row code or kind"
    - "a framework hook added once as an optional field (Hidden), nil keeping every other page as it was"
    - "draw order database, answer empty, one live paced right read, edit; a redraw after a write reads once too"

key-files:
  created:
    - fuku/modules/settings_deletion.go
    - fuku/modules/settings_deletion_env_test.go
    - fuku/modules/settings_deletion_test.go
    - fuku/modules/settings_deletion_right_test.go
    - fuku/modules/settings_deletion_screens_test.go
  modified:
    - fuku/modules/settings_pages.go
    - fuku/modules/settings_menu.go
    - fuku/modules/settings_main_test.go
    - fuku/modules/settings_edit_test.go
    - fuku/modules/settings_mreq_test.go
    - AGENTS.md

key-decisions:
  - "The right line, the Staff Group gate and the Hidden hook were written together with the tracer page, not in a second step: Task 2 is therefore tests only (they passed on first run) and the feat commit message of Task 1 says the line 'comes in the next commit', which is not what happened (it is already in c1d2d08)"
  - "A 429 answer to the live read of the bot costs 4 requests (the pacer's MaxRetries of 3); the cost and right-line tests say so, every other outcome costs exactly one"
  - "'Redis down' in TestSettingsDeletionCost removes the Redis client the pacer reads (cache.SetCacheState with nil), not the miniredis server: closing the server adds a 5 s pacer timeout per draw and 19 s to the test"
  - "A preset a row does not offer (10 s on a bot row) is drawn as After 10 s with no choice marked, the same rule as a stored 37 s"

patterns-established:
  - "settingsDelMenu / settingsDelMenus: one toolkit that runs a case in a group menu and in a private menu, each in its own environment"
  - "pure screen tests (settings_deletion_screens_test.go) render straight from a view and the data a draw read, so every row, state and language is cheap"

requirements-completed: []

coverage:
  - id: D1
    description: "the owner opens Main, the ninth cell, the hub, a list and a row screen, presses a preset: the rule is stored through the row's apply function, the cache key dropped, the state read back and drawn; a press of the stored state writes nothing; Main keeps 7 rows"
    requirement: "DEL-01"
    verification:
      - kind: unit
        ref: "TestSettingsDeletionOpensFromMain, TestSettingsDeletionMainState, TestSettingsDeletionWrites, TestSettingsMainGrid"
        status: pass
  - id: D2
    description: "every screen is built from the two tables with one state function; every row for none, 0, each preset, 37 s and the shared-flag cases is drawn as the contract says"
    requirement: "DEL-02"
    verification:
      - kind: unit
        ref: "TestSettingsDeletionStates, TestSettingsDriftDeletion"
        status: pass
  - id: D3
    description: "callbacks are strict (exactly n or the row's own canonical presets, tg refused, extra or missing keys refused) before any I/O and at most 50 bytes; widths 2, 7, 4, 4, 3 rows; no card and no e field"
    requirement: "DEL-01"
    verification:
      - kind: unit
        ref: "TestSettingsDeletionValidation, TestSettingsDeletionCallbackBytes, TestSettingsDeletionRowBudget, TestSettingsDeletionNoCard, TestSettingsControlActionAllowList"
        status: pass
  - id: D4
    description: "opening needs the admin check, a change needs can_change_info on a fresh record, joined and left leave the same flag, rows and cache as /cleanservice and applyCleanServiceRule"
    requirement: "DEL-01"
    verification:
      - kind: unit
        ref: "TestSettingsAuthorityDeletion, TestSettingsParityCleanService"
        status: pass
  - id: D5
    description: "every deletion screen carries one right line or none from one live paced read after the press is answered; never an error text; no button disabled; a rule can still be set"
    requirement: "DEL-08"
    verification:
      - kind: unit
        ref: "TestSettingsDeletionRightLine (17 answers, 5 screens, group and private), TestSettingsDeletionCost, TestSettingsDeletionStaleState"
        status: pass
  - id: D6
    description: "the page is not drawn or reachable in a Staff Group; forged presses and an unreadable row write nothing and name nothing"
    requirement: "DEL-07"
    verification:
      - kind: unit
        ref: "TestSettingsDeletionHidden, TestSettingsDeletionStaffGroup, TestSettingsDeletionReadsAndWritesFail"
        status: pass
  - id: D7
    description: "every screen fits 3800 UTF-16 units and 64-byte buttons in all 8 languages with a 64-rune multibyte title"
    requirement: "DEL-02"
    verification:
      - kind: unit
        ref: "TestSettingsDeletionPageFitsEveryLocale"
        status: pass
---

# Phase 16 Plan 08: the Deletion page Summary

**The settings menu has a Deletion page: a hub, a Service messages list and a Bot messages list and one preset screen per row (17), built from two tables with one state function, writing through `applyDeleteRule` and `applyCleanServiceRule`, with a live delete-right line on every screen and no existence at all inside a Staff Group.**

## Task commits

1. Task 1 RED `9d93f33` test(16-08): the Deletion page opens from Main and stores a preset (the page is absent: all four tracer tests fail; this is the tracer's red)
2. Task 1 GREEN (tracer) `c1d2d08` feat(16-08): the Deletion page opens from Main and stores a preset
3. Task 2 `84f76ad` test(16-08): every deletion screen says whether the bot can delete
4. Task 3 `34acb95` test(16-08): the Deletion page is strictly validated, sized and in parity
5. Task 3 `ea7acac` docs(16-08): the Deletion page in the rulebook

All three tasks are complete; the split rule was not used. This task 1 is the page's tracer and was proven green by `TestSettingsDeletionOpensFromMain` and `TestSettingsDeletionWrites`.

## What changed

- `fuku/modules/settings_deletion.go` (new): `settingsDelSections` (two sections: code, icon, name, list intro, list hints, the "no rule" word, choice-row widths, rows), `settingsDelStateOf` (the one state function), `settingsDelPresetValidator`, `renderSettingsDelHub/List/Row` (pure), `settingsDelPrepare` (database, answer empty, one live right read), `settingsDelReadRight` (`deletionReadBotRight` with the administrator flag trusted outside basic groups), `settingsDelStaffGate`, `settingsDelMainState`, `settingsDelHidden`, the controls built in a loop from the tables, registered once from `init`.
- `settings_pages.go` +6 lines (field `Hidden`, its skip in `settingsMainEntries`), `settings_menu.go` +5 lines (its guard in `openPage`); `git diff --numstat dd61b2c` of `settings_pages.go` is 6 changed lines against the gate of 14.
- Tests: the toolkit `settingsDelEnv` / `settingsDelMenu` (group and private menu, a scripted bot record and script, row, queue and staff cleanup, users rows and cache keys removed in `t.Cleanup`) and the families listed under `provides`.
- AGENTS.md: a Callbacks bullet for the page (Hidden, `dl` and `dr`, the codes, the presets, the right line, the gate), the `settings|v1` page list, the Main grid text and row count (7), the rights bullet and the D-21 bullet.

## Verification (real results)

All from /home/user/Fuku_Robot, `-tags testtools -race -count=1`.

| Gate | Result |
|------|--------|
| Task 1 verify 1 `-run '^(TestSettingsDeletionOpensFromMain\|TestSettingsDeletionMainState\|TestSettingsDeletionWrites\|TestSettingsDeletionHidden\|TestSettingsMainGrid)$' ./fuku/modules` | red at `9d93f33` (page absent: OpensFromMain, MainState, Writes, Hidden fail), PASS at `c1d2d08` |
| Task 1 verify 2, the whole family `-run '^(TestSettings\|TestLoadSettings)' ./fuku/modules ./fuku/i18n`, run before the tracer commit | ok modules 463.9 s, ok i18n 7.4 s, exit 0 |
| Task 2 verify 1 `-run '^(TestSettingsDeletionRightLine\|TestSettingsDeletionCost\|TestSettingsDeletionStaffGroup\|TestSettingsDeletionHidden\|TestSettingsDeletionWrites\|TestSettingsDeletionOpensFromMain)$'` | PASS (the tests passed on first run except two test-side fixes below) |
| Task 2 verify 2 `-run '^(TestDeletionBotRightClassifier\|TestSettingsMreqChannelStatus\|TestSettingsGateAllDoors\|TestSettingsControlActionAllowList)'` | ok 24.5 s |
| Task 3 verify 1 `-run '^(TestSettingsDeletion\|TestSettingsAuthorityDeletion\|TestSettingsDriftDeletion\|TestSettingsParityCleanService\|TestSettingsControlActionAllowList)' -v` | 19 PASS, 0 FAIL, 0 SKIP |
| Task 3 verify 2, the whole settings family again at the end (HEAD `34acb95`) | ok modules 462.9 s, ok i18n 6.9 s, exit 0 |
| `-run '^(TestDeletion\|TestServiceWatcher\|TestServiceKind\|TestApply\|TestCleanService\|TestScheduleBot\|TestBot)' ./fuku/modules` | ok 43.5 s |
| `go vet -tags testtools ./fuku/modules` and `CGO_ENABLED=0 go build ./...` | exit 0 |
| `gofmt -l` on every touched Go file | no output |
| `python3 .planning/phases/13-member-requirements/handoff/lint-local.py 8c0c827` (exit read directly) | exit 0, "no dupl, gocyclo or godox findings" |
| `git diff --exit-code 8c0c827 -- go.mod go.sum` | exit 0 |
| legacy names: `grep 'reflect.Ptr\|ioutil\.\|//nolint' fuku/modules/settings_deletion*.go` | none |
| Acceptance greps | `registerSettingsPage(` 1; `Hidden func(\|pg.Hidden != nil` 2; settings_pages.go diff 6 lines (limit 14); `r\|m / d / a` in settings_edit_test.go 1 and settings_mreq_test.go 2; `deletionReadBotRight(` 1; `deletionStaffLookup(` 3; `CanBotDelete\|FetchBotMember` 0; `"e"\|settingsActCancel\|settingsCardRows` 0; AGENTS.md `Deletion page` 5, `a=dl` 2; test funcs named by the task acceptance well over 15 |

Not run: `make test` (the plan does not ask; plan 16-09 does), `make lint` (cannot run here, the installed golangci-lint is built with Go 1.25), `make check-docs` and `make check-translations` (no locale or generated-doc change), the PostgreSQL run (the `fuku/modules` harness does not run on PostgreSQL; these tests run on SQLite), `TestSettingsStaffPrivacy` additions and the end-to-end test (plan 16-09's).

## Which tests were red

- Red, by the page being absent: the four tracer tests (`9d93f33`).
- Characterization, green on first run because the tracer already carried the right line and the gate: `TestSettingsDeletionRightLine`, `TestSettingsDeletionCost`, `TestSettingsDeletionStaffGroup`, `TestSettingsDeletionReadsAndWritesFail`, and in Task 3 `RowBudget`, `CallbackBytes`, `NoCard`, `PageFitsEveryLocale`, `Validation`, `StaleState`, `DriftDeletion`.
- Failed once and fixed in the TEST, never loosened: `TestSettingsDeletionMainState` (the opening of the menu itself reads the bot once, so the test now counts the delta after opening), `TestSettingsDeletionRightLine/a_429` (the pacer repeats a 429 call three times: expected reads 4), `TestSettingsDeletionStates` (a preset the row does not offer, 10 s on a bot row, marks nothing: the expectation now says so), `TestSettingsAuthorityDeletion` (the command reads the cached administrator list, which the test invalidates after flipping the right). No production fix commit was needed except one nil guard on the flag read, inside the Task 3 test commit.

## Deviations from Plan

1. **[Process] Tasks 1 and 2 merged in production, not in tests.** The delete-right line, `settingsDelStaffGate` and the Staff Group read in `Open` were written with the tracer page, so Task 2's commit is tests only and the Task 1 feat message ("The delete-right line comes in the next commit") is inaccurate. History is not rewritten; the code of `c1d2d08` is the truth.
2. **[Test files] One extra test file, `settings_deletion_screens_test.go`,** holds the pure families (row budget, bytes, states, no card, page fit) next to the plan's three named files; `TestSettingsDeletionReadsAndWritesFail` is an extra family for the read-failure and write-failure bullets of Task 2's behavior.
3. **[Extra] A nil guard** on the flag read in `settingsDelRead` (`settings != nil && settings.ShouldCleanService`, as `deletionRuleFor` has), found while writing the drift test; it sits in the Task 3 test commit with the file it touches.
4. **[Existing tests changed, as the plan names]** `settings_edit_test.go` (line 990), `settings_mreq_test.go` (lines 231 and 267) and `settingsMainWorstState` in `settings_main_test.go`, plus a nine-cell subtest in `TestSettingsMainGrid` and one subtest in `TestSettingsControlActionAllowList` (`tg` on a Deletion row control); no registry-driven test needed a change and none counted Telegram calls per page.
5. Plan gate texts were not broken.

## Known Stubs

None.

## Threat Flags

None beyond the plan's threat model: the page adds no endpoint, no table and no Redis key; the only new Telegram call is the live `getChatMember` of the bot through the existing deletion pacer.

## Notes for plan 16-09 and the owner

- Plan 16-09 owns the `TestSettingsStaffPrivacy` additions (hub, both lists, 17 row screens, every right-line outcome in 8 languages, a private run of an administrator who is not the creator) and the end-to-end test; the screens here already draw no Staff Group title or ID (checked in a linked group in `TestSettingsDeletionStaffGroup`).
- A 429 on the bot read is repeated by the pacer (3 retries); at the production unit a retry_after of 1 s or more runs into `settingsDelHereTimeout` (3 s) and ends as the unknown line, which is the intended outcome.

## Self-Check: PASSED

- Created files exist: the five new Go files and this SUMMARY FOUND.
- Commits are ancestors of HEAD: `9d93f33`, `c1d2d08`, `84f76ad`, `34acb95`, `ea7acac` FOUND.
- Requirements: DEL-01, DEL-02, DEL-06, DEL-07, DEL-08 and MREQ-15 left Pending (`requirements-completed: []`).
