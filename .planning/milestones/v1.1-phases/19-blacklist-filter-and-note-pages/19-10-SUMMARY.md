---
phase: 19-blacklist-filter-and-note-pages
plan: 10
subsystem: settings-menu-list-pages-sweeps
tags: [telegram, settings-menu, list-engine, privacy, tests]
status: complete

requires:
  - phase: 19-blacklist-filter-and-note-pages
    provides: 19-03 to 19-09, the list engine and the Blacklist, Filters and Notes tables, their apply functions, the door's list branch and Main at its final 18 grid cells
provides:
  - TestSettingsPhase19Codes and TestSettingsMainPhase19 (codes, part table, Main lines in both menus)
  - TestSettingsListGroupMenuShowsNothing and TestSettingsListLogsCarryIDsOnly (group-menu privacy, logs over every failing seam)
  - settingsPrivacyPhase19Screens and settingsPrivacyPhase19Run in TestSettingsStaffPrivacy (8 languages, both menu kinds)
  - a list-doors column in TestSettingsGateAllDoors, TestSettingsListReadsCreateNothing, TestSettingsListCost, TestSettingsListSavedNoticeFits
affects: [19-11, 19-12]

plan_head_before: 023e5e9ea1e1ee109594fc3818504cbfec4480f9
plan_head_after: a1a99661e857ac6201d2af640d54be588a1154b1

actuals:
  tokens: 16900   # chars/4 over the added lines of 023e5e9..a1a9966 outside .planning (67640 chars)
  tasks: 3
  commits: 3      # MEASURED: git rev-list --count 023e5e9..HEAD (before this SUMMARY commit)

tech-stack:
  added: []
  patterns:
    - "a cross-page sweep reuses each page's own toolkit (counting seams, flows, worst entries) instead of cloning it"
    - "a gate door column builds one world per fixture and runs the press and the typed reply in it"

key-files:
  created:
    - fuku/modules/settings_phase19_test.go
    - fuku/modules/settings_list_privacy_test.go
  modified:
    - fuku/modules/settings_staff_privacy_test.go
    - fuku/modules/settings_gate_test.go
    - AGENTS.md

key-decisions:
  - "Test-only plan: no production file changed; no sweep exposed a defect."
  - "The two list doors share one column of TestSettingsGateAllDoors (one environment per fixture) to keep the test light; the column reports 'yes/typed' when the two decisions differ."

patterns-established:
  - "settingsPhase19Codes: the 17 list action codes written out beside the registry and compared in both directions"
  - "settingsListLogged: a debug-level logrus hook with mark/since/wantChat for failure-path log sweeps"

requirements-completed: []

coverage:
  - id: D1
    description: "the 19 page keys, the 17 list action codes and the five-part table are registered once, collide with nothing, and every list action is refused in step 1 from a group menu, with both owner fields or with an extra key"
    requirement: "PAGE-07, PAGE-08, PAGE-09 (evidence for the cross-page guarantees; completes none)"
    verification:
      - kind: unit
        ref: "TestSettingsPhase19Codes"
        status: pass
    human_judgment: false
  - id: D2
    description: "Main reads 12, 12/150 and 3 (0, 0/150, 0 on an empty chat), the unknown mark on one failing count with every other line unchanged, In private in a group menu with no seam called, and makes no Telegram call of its own"
    requirement: "PAGE-07, PAGE-08, PAGE-09 (evidence only)"
    verification:
      - kind: unit
        ref: "TestSettingsMainPhase19, TestSettingsListCost"
        status: pass
    human_judgment: false
  - id: D3
    description: "no list screen in 8 languages, and no run of an administrator who is not the creator in either menu kind, names the Staff Group; a group menu shows and reads no entry, count or action; no log line of the engine, the door's list branch or the ten apply functions carries a word, keyword, name or content"
    requirement: "PAGE-10, PLAT-05 (evidence only)"
    verification:
      - kind: unit
        ref: "TestSettingsStaffPrivacy (Phase 19 screens and runs), TestSettingsListGroupMenuShowsNothing, TestSettingsListLogsCarryIDsOnly (mutation check: logging the typed words in settingsBlacklistTyped fails it)"
        status: pass
    human_judgment: false
  - id: D4
    description: "the owner gate decides the same at the list Yes and list typed-reply doors over the four owner worlds; opening Main and every list screen in both menus creates no row and fills no command cache key; each draw costs what UI-SPEC section 9 says, also with Redis down; the saved pages and refusals fit 3800 UTF-16 units in 8 languages"
    requirement: "PAGE-07, PAGE-08, PAGE-09, PLAT-05 (evidence only)"
    verification:
      - kind: unit
        ref: "TestSettingsGateAllDoors, TestSettingsListReadsCreateNothing, TestSettingsListCost, TestSettingsListSavedNoticeFits"
        status: pass
    human_judgment: false
---

# Phase 19 Plan 10: the cross-page sweeps Summary

**Test-only plan. The three list pages are now held together by guarantees no single page test sees: Main's final shape and states, unique and private-only codes, privacy in 8 languages and in group menus, logs without user text, the owner gate at the list doors, reads that create nothing and the cost of each draw. No production code changed.**

## What was built

- **Task 1, the tracer (`b418784`).** `settings_phase19_test.go`. `TestSettingsPhase19Codes`: the 19 registered page keys, the three list pages private-only with a table each, the **17** list action codes (`bp bx by bn bc bo bs`, `ig ix iy ia ic`, `eg ex ey ea ec`; the plan text says sixteen, the registry holds seventeen) written out beside the registry and compared in both directions, each owned by one page and never a core code; every code accepted in step 1 with `c` and its exact fields and refused with `u`, with both and with an extra key; the part table is exactly `(b,w) (i,k) (i,y) (e,n) (e,c)` with steps 1, 1, 2, 1, 2; `tg` and `st` naming `b`, `i` or `e` fail both the control check and step 1. `TestSettingsMainPhase19` (one environment): in the owner's private Main the three lines read `12`, `12/150`, `3` in grid order with the 11 owner rows and the private shape constant, each earlier page's line equals its own `MainState`, Main makes no Telegram call; an empty chat reads `0`, `0/150`, `0`; each of the three counts failing alone is the unknown mark on that line with the other 18 lines unchanged; a group Main reads `In private` three times, calls none of the six page seams, and the administrator's group Main has 10 rows.
- **Task 2 (`052cabf`).** `settings_list_privacy_test.go` and `settings_staff_privacy_test.go`. `TestSettingsListGroupMenuShowsNothing`: sentinels in all three lists, a group menu opened on each list cell, Back and a forged Cancel draws only the group note, every one of the 17 codes pressed with `u` is the invalid-request alert with no `getChatMember` or `getChat`, no sentinel, count line, action line or entry line appears anywhere the group could read, the six seams are called zero times and the rows are untouched. `TestSettingsListLogsCarryIDsOnly`: a debug-level logrus hook over every failing seam (each page's read, the Filters Add count, each delete write, the action write, typed words, both typed steps of Filters and of Notes with a failing check and a failing write, the wait store under a miniredis error, and 13 failure paths of the ten apply functions), sentinel word, keyword, name and content everywhere; every failure logs a line naming the chat and no line holds a sentinel. `TestSettingsStaffPrivacy` gains `settingsPrivacyPhase19Screens` (per language: the group note with and without a bot username, and in the private admin view each page's empty, one-entry, eight-entry and middle-page lists under the longest two notices, the card, the prompts, four refusals, the saved page, and the Blacklist action screen uniform, mixed, inert and empty) and `settingsPrivacyPhase19Run` in a private and a group-menu run of an administrator who is not the creator (permitted presses and typed replies reach stored rows in the private run, only the group note is drawn in the group run), all searched for the Staff Group's title and ID.
- **Task 3 (`a1a9966`).** `settings_gate_test.go`, `settings_phase19_test.go`, `AGENTS.md`. `TestSettingsGateAllDoors` gains the list doors (below). `TestSettingsListReadsCreateNothing`: Main, every page, the action screen, forged asks, a pager past the end and forged Yes presses in both menu kinds on a chat with no rows leave no row in `blacklists`, `filters`, `notes`, `notes_settings` or `chats` and fill none of the four command cache keys; a failing read is the cannot-open edit with no keyboard on each page, never an empty list. `TestSettingsListCost`: three count reads in a private Main and none in a group Main with no Telegram call; open and pager of each page make exactly one list read and the pipeline's `getChatMember`, `getChat`, edit and answer; a group note makes no read and one `getChatMember`; five forged presses cost only their answer; a typed step on each page costs one lookup of the sender, one `getChat` and at most one lookup of the bot; the list, the card and the action cost the same with Redis down, and Add is the alert. `TestSettingsListSavedNoticeFits`: the Blacklist saved page at five five-digit counts, the Filters and Notes saved pages (Notes with the conflict warning) and the three longest refusals of each page, alone and with the wait-ended line, at most 3800 UTF-16 units in 8 languages with a 64-rune title. AGENTS.md: one scoped insertion (the Phase 19 test families, the two plan 19-10 files, the door column).

## Verification (real results)

- Task gates, bare (`env -i`) and under `cienv.sh`: `TestSettingsPhase19Codes`, `TestSettingsMainPhase19`, `TestSettingsPhase18Codes`, `TestSettingsMainPhase18`, `TestSettingsMainPageFitsEveryLocale` (Task 1); `TestSettingsStaffPrivacy`, `TestSettingsListGroupMenuShowsNothing`, `TestSettingsListLogsCarryIDsOnly` (Task 2); `TestSettingsGateAllDoors`, `TestSettingsListReadsCreateNothing`, `TestSettingsListCost`, `TestSettingsListSavedNoticeFits` (Task 3): all `ok`, no `--- FAIL`, no `--- SKIP`. `./fuku/i18n`: `ok` bare (25.2 s) and under `cienv.sh` (26.1 s).
- Whole `fuku/modules` package under the CI environment (`go test -tags testtools -race -count=1 -timeout 40m -v ./fuku/modules`, background, log `/tmp/claude-0/-home-user-Fuku-Robot/4ce545b5-6832-57a3-a863-43c972932c7c/scratchpad/modules-19-10.log`): `ok  github.com/uasneppy/fuku_robot/fuku/modules  1988.922s`, last line `exit 0`, 1840 top-level PASS (402 of them `TestSettings*` or `TestLoadSettings*`, so the settings family ran inside it), 0 `--- FAIL`, 0 `--- SKIP`, 0 `^FAIL`; `--- PASS: TestSettingsGateAllDoors (17.22s)`, `TestSettingsListGroupMenuShowsNothing (0.42s)`, `TestSettingsListLogsCarryIDsOnly (1.19s)`, `TestSettingsStaffPrivacy (25.88s)`, `TestSettingsPhase19Codes`, `TestSettingsMainPhase19 (0.57s)`, `TestSettingsListReadsCreateNothing (0.41s)`, `TestSettingsListCost (0.42s)` and `TestSettingsListSavedNoticeFits (1.19s)` are in it. The count is 7 above the 1833 of 19-09: exactly the seven new top-level tests.
- Mutation check: logging the typed words in `settingsBlacklistTyped`'s save-failure line made `TestSettingsListLogsCarryIDsOnly` fail naming both sentinels; the file was restored byte for byte (`git status` clean for it).
- Gates: `go vet -tags testtools ./fuku/modules ./fuku/i18n` clean; `CGO_ENABLED=0 go build ./...` clean; `gofmt -l` over the five files this plan created or edited prints nothing; `lint-local.py 8c0c827` exits 0 (read from the exit code, run before the last code commit and again before this SUMMARY: "no dupl, gocyclo or godox findings on lines added since 8c0c827"); `git diff --exit-code 8c0c827 -- go.mod go.sum` exits 0; no `t.Skip`, `reflect.Ptr` or `ioutil` in the new files; the Task 1 invariant (only 19-02 edited `settings_pages.go` since `d93ec1d`) exits 0. `make lint`, gosec and `make test` were not run (the sandbox cannot run lint, and `make test` is the closing plan's).

## Time added to the whole-package run

About 6 s. New tests, from the whole-package log: `TestSettingsMainPhase19` 0.57 s, `TestSettingsListLogsCarryIDsOnly` 1.19 s, `TestSettingsListSavedNoticeFits` 1.19 s (a second of it is installing the real locales), `TestSettingsListGroupMenuShowsNothing` 0.42 s, `TestSettingsListReadsCreateNothing` 0.41 s, `TestSettingsListCost` 0.42 s, `TestSettingsPhase19Codes` 0 s: 4.2 s. Extended tests against the 19-09 log: `TestSettingsGateAllDoors` 15.99 to 17.22 s (+1.2 s), `TestSettingsStaffPrivacy` 25.56 to 25.88 s (+0.3 s; focused runs measured +1.6 s). The first drafts of `TestSettingsMainPhase19` and `TestSettingsListReadsCreateNothing` built one environment per subtest (1.6 s and 1.1 s); they now share one (0.5 s and 0.4 s). No new test is over 1.2 s. The plan estimated 5 s.

## Deviations from Plan

**1. [Facts newer than the plan text] 17 list action codes, not sixteen; ten apply functions, not seven.** The registry holds `bp bx by bn bc bo bs`, `ig ix iy ia ic` and `eg ex ey ea ec`: 17. The apply functions with a write seam are `applyBlacklist{Add,Remove,RemoveByID,Action}`, `applyFilter{Add,Remove,RemoveByID}` and `applyNote{Add,Remove,RemoveByID}`: ten, covered by 13 failure cases in the log sweep (the filter add and the note add each also fail at their existing-name check, and the filter add at its count). The tests count from the code.
**2. [Structure] One column of `TestSettingsGateAllDoors` for the two list doors**, named "list Yes and list typed reply", not two columns. The two share one environment and one world per fixture (the first version with two columns added 3.3 s, this one 1.2 s); the column answers with the common decision of the two, or `yes/typed` when they differ, so a door that decides differently still fails the fixture and names which one.
**3. [Detail the plan left open] The wait-store log case uses miniredis `SetError`**, because "no Redis" (`errSettingsInputUnavailable`) is deliberately not logged; a failing Redis is, with the user ID.
**4. [Unneeded edit] `settings_main_test.go` was not edited**, though the plan lists it: `settingsMainWorstState` already carries the Phase 19 states (`settings_state_private_only`, `99999/150`) from 19-02 and 19-03.
**5. [Environment] The sandbox disk was at 17 GB free; no trimming was needed this time.**

No auth gates.

## Known Stubs

None.

## Threat Flags

None new. T-19-48 (an entry, count or action reaching a group menu): `TestSettingsListGroupMenuShowsNothing` (sentinels, zero seam calls) and `TestSettingsMainPhase19`. T-19-49 (the Staff Group named by a list screen or a door reply): `TestSettingsStaffPrivacy` with `settingsPrivacyPhase19Screens` and `settingsPrivacyPhase19Run`. T-19-50 (user text in a log line): `TestSettingsListLogsCarryIDsOnly` plus the mutation check. T-19-51 (a list door bypassing the owner gate): the new column of `TestSettingsGateAllDoors` over the four owner worlds. T-19-52 (a draw that creates rows or costs more than the contract): `TestSettingsListReadsCreateNothing`, `TestSettingsListCost`. T-19-SC: `git diff --exit-code 8c0c827 -- go.mod go.sum` exits 0.

## Requirements

None completed: `requirements-completed: []`, and nothing was ticked in `REQUIREMENTS.md` (19-12 alone completes PAGE-07, PAGE-08 and PAGE-09 after its end-to-end test). This plan's tests are evidence for the cross-page guarantees of PAGE-07, PAGE-08 and PAGE-09 (privacy, codes, Main, reads, cost, the gate at the new doors) and for PAGE-10 and PLAT-05 (every list screen in 8 languages, no Staff Group, no user text in logs).

## Self-Check: PASSED

- Files exist: `fuku/modules/settings_phase19_test.go`, `fuku/modules/settings_list_privacy_test.go`.
- Commits reachable from HEAD: `b418784`, `052cabf`, `a1a9966`.
