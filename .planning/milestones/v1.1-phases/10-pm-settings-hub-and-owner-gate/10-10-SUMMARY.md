---
phase: 10-pm-settings-hub-and-owner-gate
plan: 10
subsystem: settings
tags: [settings, main-page, grid, status-block, navigation, privacy, locale-fit, phase-gate]
requires:
  - phase: 10-pm-settings-hub-and-owner-gate
    provides: the Access page and OwnerOnly (10-06), the private list, the shub|v1 namespace and the pick (10-09), the 19 locale strings and the two-column label limits (10-02), the gate doors (10-05, 10-11)
provides:
  - renderSettingsMain as a two-column grid with a status block and an anchored owner-only row
  - settingsHubGroupsButton and the Groups button on a private Main page
  - TestSettingsMainGrid, TestSettingsMainPageFitsEveryLocale, TestSettingsHubGroupsButton and the extended navigation, byte and Staff privacy tests
  - AGENTS.md rules for the Main grid and navigation
  - 10-SPIKE.md, the owner's non-blocking live checks
affects: []

status: complete
actuals:
  tokens: 12600
  tasks: 3
  commits: 4

commits: 4
plan_head_before: e5b7f6a2f0209398a0ab388560fe1bedb14f78bc
plan_head_after: 74e78b289859888bc10f65ad64a0cb29fa7aeb33

tech-stack:
  added: []
  patterns:
    - "A page's state lives in a status block in the message body, never in a button label, so a button is only 'icon name' and fits half the screen width in every language"
    - "An owner-only page is never paired into the grid: it gets its own full-width row after the grid, so the owner's grid and an admin's differ by that row only"
    - "The marker-locale tests read a page's state through settingsMainStateLine, the body line carrying the page title's marker"

key-files:
  created:
    - fuku/modules/settings_main_test.go
    - .planning/phases/10-pm-settings-hub-and-owner-gate/10-SPIKE.md
  modified:
    - fuku/modules/settings_pages.go
    - fuku/modules/settings_access_test.go
    - fuku/modules/settings_antiflood_test.go
    - fuku/modules/settings_lockdown_test.go
    - fuku/modules/settings_menu_test.go
    - fuku/modules/settings_private_test.go
    - fuku/modules/settings_staff_test.go
    - fuku/modules/settings_staff_privacy_test.go
    - fuku/modules/settings_test.go
    - AGENTS.md

key-decisions:
  - "navRow(true) decides by the view: a private Main gets Groups then Close, a group Main Close alone; the Groups button is built by settingsHubGroupsButton and an encode failure is counted in Omitted like any other button"
  - "settingsStateSeparator was removed (nothing else used it); settingsUnknownState stays and now fills the state of an unreadable status line"
  - "settingsPageButton (the shared test helper) skips shub buttons instead of every caller learning about them"
  - "Task 2 added no renderer change: its behaviour was already true after Task 1, so the new tests passed on first run and prove the existing code rather than driving a new one"

requirements-completed: [MENU-15, MENU-12, MENU-09, PLAT-05]

coverage:
  - id: D1
    description: "The Main page is a two-column grid of 'icon name' buttons in page order with an odd last one alone, the Access button on its own full-width row after the grid for the owner only, and one settings_main_state_line per page in a status block (Access last, unknown mark when unreadable)"
    requirement: MENU-15
    verification:
      - kind: unit
        ref: "fuku/modules/settings_main_test.go#TestSettingsMainGrid"
        status: pass
    human_judgment: false
  - id: D2
    description: "With no visible page the Main page still renders: the hint (and the owner line in a group view), no status block and no blank line, and the navigation row alone"
    requirement: MENU-15
    verification:
      - kind: unit
        ref: "fuku/modules/settings_main_test.go#TestSettingsMainGrid"
        status: pass
    human_judgment: false
  - id: D3
    description: "A private Main ends with Groups and Close, a group Main with Close alone, every other page with Back and Close; Back from every page redraws Main; Groups redraws the private list at page 1 in the same message, answered once with no new message"
    requirement: MENU-09
    verification:
      - kind: unit
        ref: "fuku/modules/settings_menu_test.go#TestSettingsNavigation"
        status: pass
      - kind: unit
        ref: "fuku/modules/settings_main_test.go#TestSettingsHubGroupsButton"
        status: pass
    human_judgment: false
  - id: D4
    description: "An allowed admin's Main has no Access row and no Access line, and the Access state is not read for them"
    requirement: MENU-12
    verification:
      - kind: unit
        ref: "fuku/modules/settings_main_test.go#TestSettingsMainGrid"
        status: pass
      - kind: unit
        ref: "fuku/modules/settings_access_test.go#TestSettingsAccessHiddenFromAdmins"
        status: pass
    human_judgment: false
  - id: D5
    description: "The list, the Main page with its status block, and the Access page and card, in a group view and in a private view of an admin who is not the creator, never contain the Staff Group's title or chat ID in any of the 8 languages"
    requirement: PLAT-05
    verification:
      - kind: unit
        ref: "fuku/modules/settings_staff_privacy_test.go#TestSettingsStaffPrivacy"
        status: pass
    human_judgment: false
  - id: D6
    description: "The Main page with the Access row fits 3800 UTF-16 units with a 64-rune title (ampersands, astral, multibyte) in all 8 languages; every grid button is at most 20 code points and the Groups button at most 18; every Main callback, the Groups one included, is at most 64 bytes (Groups at most 16) with Omitted 0"
    requirement: PLAT-05
    verification:
      - kind: unit
        ref: "fuku/modules/settings_main_test.go#TestSettingsMainPageFitsEveryLocale"
        status: pass
      - kind: unit
        ref: "fuku/modules/settings_menu_test.go#TestSettingsCallbackBytes"
        status: pass
      - kind: unit
        ref: "fuku/i18n/settings_locale_test.go#TestSettingsLabelLengths"
        status: pass
    human_judgment: false
  - id: D7
    description: "Every locale has every key (make check-translations exit 0) and no project document or Go test file states the old locale count; AGENTS.md reflects the 8-locale set"
    requirement: PLAT-05
    verification:
      - kind: unit
        ref: "make check-translations (exit 0, all 8 locales complete)"
        status: pass
      - kind: unit
        ref: "grep of the old count over AGENTS.md README.md .planning/PROJECT.md .claude/CLAUDE.md and fuku/**/*_test.go (no match)"
        status: pass
    human_judgment: false
  - id: D8
    description: "On real Telegram clients the grid names and the Access toggle are not ellipsized in hi, ru and uk, the <code> commands in the hint copy on tap, and Close works on an old private message"
    requirement: MENU-15
    verification:
      - kind: backstop
        ref: ".planning/phases/10-pm-settings-hub-and-owner-gate/10-SPIKE.md check 3"
        status: pending
    human_judgment: true
---

# Phase 10 Plan 10: Main grid, status block, navigation and phase gate Summary

**The Main page is now a two-column grid of "icon name" buttons with each page's state in a status block above it, the owner's Access button on its own row, and Groups as the private Main's way back to the list; the navigation, byte, locale-fit and Staff privacy tests cover every new screen in all 8 languages.**

## Performance

- **Duration:** about 35 minutes
- **Started:** 2026-11-04T14:18Z
- **Completed:** 2026-11-04T14:53Z
- **Tasks:** 3 (one tracer, one tdd, one docs and gate)
- **Files modified:** 12 (780 insertions, 78 deletions)

## Accomplishments

- `renderSettingsMain` (`fuku/modules/settings_pages.go`) draws the grid (two buttons a row in page order, an odd last one alone), then each `OwnerOnly` entry on its own full-width row, then the navigation row. The body is the hint, the owner line in a group menu and, only when a page is visible, one blank line and one `settings_main_state_line` per page, the owner-only line last, the unknown mark for an unreadable state.
- `settingsMainEntry.OwnerOnly` is copied from the page, so an admin's Main differs from the owner's by the Access row and line only. `settingsHubGroupsButton` (`shub|v1 a=lp&v=1`, 16 bytes) is the first button of a private Main's navigation row; a group Main keeps Close alone and every sub-page Back and Close.
- `TestSettingsMainGrid`, `TestSettingsMainPageFitsEveryLocale` and `TestSettingsHubGroupsButton` are new; `TestSettingsNavigation`, `TestSettingsCallbackBytes` and `TestSettingsStaffPrivacy` are extended (the privacy test's pure-render subtest is renamed "pure renders in every language" and now renders the empty and populated list, the Main status block in a group view and in both private views, and the Access page and card).
- `AGENTS.md` gets a Main page bullet (grid, status block, owner-only row, navigation) and the new test families; `10-SPIKE.md` lists the owner's seven non-blocking live checks (A1, A2, real clients and truncation, the backfill and sweep on two replicas, PostgreSQL integrity, a native-speaker read, `make lint` on Go 1.26).

## Task Commits

1. **Task 1 (tracer): the two-column grid, status block, Access row and Groups way back** - `cbe0128` (feat)
2. **Task 2: navigation, bytes, locale fit and Staff privacy over every new screen** - `a2f5d41` (test)
3. **Task 3: the rulebook and the owner's live-check file** - `2c95cd0` (docs)
4. **Doc comment reword of the Main renderer** - `74e78b2` (docs)

## Old Main-layout tests rewritten

The plan listed four files to rewrite; the Main layout change also broke tests outside that list, which read a page's state from a Main button label or assumed one button per row or only settings-namespace buttons. Each was changed only where its expected layout changed:

- Listed: `settings_antiflood_test.go` (Main state subtest and the read-failure test), `settings_lockdown_test.go` (the Main-state and unreadable subtests), `settings_staff_test.go` (three Main assertions), `settings_staff_privacy_test.go` (the "main" screen).
- Not listed, changed under Rule 3 (see Deviations): `settings_access_test.go` (read failure reads the status line; the hidden-from-admins test no longer meets a `shub` button), `settings_menu_test.go` (the captcha read-failure state check), `settings_private_test.go` (the private Main loop skips the Groups button, which names no group), `settings_test.go` (`TestSettingsTracer` now expects grid rows of two and the captcha Off state in the status line; the shared `settingsPageButton` helper skips `shub` buttons).

## Phase gate

| Command | Result |
|---------|--------|
| `go test -tags testtools -race -count=1 -run '^(TestSettings\|TestLoadSettings)' ./fuku/modules ./fuku/i18n` | passed (before the last test fix: one failure in `TestSettingsTracer`, fixed and re-run green with the other touched tests) |
| `make test` | could not exit 0 in this sandbox: the Go toolchain here has no `covdata` tool, so `go test -coverpkg` reports a `build-fail` for `fuku/utils/actionlog` (no test files; `go: no such tool "covdata"`) and the pipeline's checker fails on that one event. Every package's tests passed and `go test -json` reported no test failure (6005 pass events, no `fail`); feeding the same JSON through `scripts/check_test_results` with only that environment `build-fail` event removed exits 0 (no unexpected skips) |
| `make check-translations` | exit 0, all 8 locales complete (no missing key at all, better than the recorded baseline) |
| `make generate-docs`, then `make check-docs` | no drift (exit 0) |
| `make lint` | could not run: golangci-lint 2.5.0 was built with go1.25.1 and refuses the repo's Go 1.26.0 target ("can't load config"). Fallback run: `go vet -tags testtools ./...` exit 0; `gofmt -l` over the files this plan changed prints nothing (it still lists ten older, untouched files such as `fuku/db/admin/repository.go`) |
| `git diff --exit-code 6ef6ad9 -- go.mod go.sum` | exit 0 |
| old locale count in `AGENTS.md README.md .planning/PROJECT.md .claude/CLAUDE.md` and in `fuku/**/*_test.go` | no match |

## Requirements

- MENU-15 (two-column grid with Close in the opener's language, Back on every page): true after this plan; ticked.
- MENU-12 and MENU-09: already complete; re-confirmed by the new tests.
- PLAT-05 (every new label in all 8 locales, AGENTS.md reflects the 8-locale set): `make check-translations` is complete for all 8 locales, `AGENTS.md` names the 8 files (`Adding a module` step 7 and the tests' wording), and no 7-locale wording is left; ticked.

## Decisions Made

See `key-decisions` above. The state moved out of the button is spliced as `MainState` returns it, which was already a plain word that never names the Staff Group, so no new escaping was needed.

## Deviations from Plan

**1. [Rule 3 - Blocking] Tests outside the plan's file list read the old Main layout**
- **Found during:** Task 1 (the first run of the settings suite)
- **Issue:** the plan assumed `settings_private_test.go` and `settings_menu_test.go` need no rewrite, but the new layout broke `TestSettingsAccessReadFailure`, `TestSettingsAccessHiddenFromAdmins`, `TestSettingsAntifloodReadFailure`, `TestSettingsCaptchaReadFailure`, `TestSettingsLockdownUnreadable`, `TestSettingsStartDeepLink` and `TestSettingsTracer` (each read a state from a button label, assumed one page per row, or decoded every button as a settings button).
- **Fix:** each was changed only where its expected layout changed, keeping its intent (listed above). The first commit therefore touches test files beyond the plan's Task 1 list.
- **Files modified:** `settings_access_test.go`, `settings_menu_test.go`, `settings_private_test.go`, `settings_test.go`
- **Commit:** `cbe0128`

**2. [Environment] `make test` and `make lint` cannot exit 0 in this sandbox**
- Both are tooling limits of the sandbox (no `covdata` tool; golangci-lint built with Go 1.25), not caused by this plan. See the gate table for what ran instead. The owner's list (10-SPIKE.md check 7) asks for `make lint` on a Go 1.26 build.

**Task 2 note:** it is marked `tdd="true"`, but the renderer it exercises was finished in Task 1 (the tracer), so the new tests passed on first run; no failing-first commit exists for it.

## Known Stubs

None.

## Threat Flags

None. The status block uses each page's `MainState`, which never names the Staff Group (T-10-32), Access is drawn only for the live creator and `TestSettingsMainGrid` and `TestSettingsAccessHiddenFromAdmins` hold it (T-10-33), and every Main button's encode failure is counted in `Omitted` with the worst-case test failing on any (T-10-34). No package was added (T-10-SC).

## Self-Check: PASSED

- `fuku/modules/settings_main_test.go`, `fuku/modules/settings_pages.go` and `.planning/phases/10-pm-settings-hub-and-owner-gate/10-SPIKE.md` exist; `grep -c 'Deleted Account'` on the SPIKE prints 2.
- Commits `cbe0128`, `a2f5d41`, `2c95cd0` and `74e78b2` are ancestors of HEAD.
- `grep -c settings_main_state_line fuku/modules/settings_pages.go` prints 1; `grep -c settingsHubGroupsButton` on `settings_pages.go` and on `AGENTS.md` print 3 and 1. The last commit only rewords the renderer's doc comment (no code) and is covered by the build.
