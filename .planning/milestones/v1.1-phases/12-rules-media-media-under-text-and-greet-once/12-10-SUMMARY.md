---
phase: 12-rules-media-media-under-text-and-greet-once
plan: 10
subsystem: settings-editor
tags: [greet-once, welcome, settings-menu, switch-grid, locales, editor]
requires:
  - phase: 12-rules-media-media-under-text-and-greet-once
    provides: 12-05 the 13 locale keys and the rendered 20-code-point cell check, 12-06 control Validate and settingsEditSwitchValue, 12-09 applyWelcomeOnce and the greet-once runtime
provides:
  - settingsEditSwitch table and settingsEditSwitchGrid (two-column, icon-only buttons, one state body line per drawn switch, per-switch hints) replacing ToggleLabelKey, CleanLabelKey, CleanHintKey and settingsEditSwitchRows
  - control o (Greet once) on the Welcome page, settingsRightAdmin, applyWelcomeOnce
  - settingsStateIcon, settingsEditSwitchCells, settingsEditGreetingApply
  - the four label keys renamed to settings_edit_line_welcome, _goodbye, _clean_welcome, _clean_goodbye in all 8 locales
affects: [12-11, 12-12, 12-16]

actuals:
  tokens: 20600
  tasks: 3
  commits: 4

commits: 4
plan_head_before: 5a3cd060d847071ef2a7acd99caa0eca3326fee3
plan_head_after: a6a69c373c5841ef7903c1acc6c5a76d71673901

tech-stack:
  added: []
  patterns:
    - "A page's switches are a table (settingsEditSwitch); the grid, the body lines, the hints, the Controls and the apply adapter are all derived from it, so a later switch is one table row"
    - "A state icon alone on a half-width button, the word on a body line (UI-SPEC amendment M1)"

key-files:
  created:
    - fuku/modules/settings_edit_switch_test.go
  modified:
    - fuku/modules/settings_edit.go
    - fuku/modules/settings_pages.go
    - fuku/modules/settings_edit_test.go
    - fuku/modules/settings_edit_actions_test.go
    - fuku/modules/settings_edit_env_test.go
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
  - "The AGENTS.md edits (grid, control o, tg/st control check and Validate, Greet once write path) went into the Task 1 feat commit, not Task 3 as the plan listed: the rule they describe is invalidated by that commit, and AGENTS.md says to update it in the same commit. Task 3 only adds the Testing family sentence"
  - "settingsEditSwitch.Apply returns (outcome, *state) as the plan's type says; the adapter settingsEditGreetingApply logs a failed write (page key and chat ID only) and returns a nil state, so the caller reloads the freshly loaded page"
  - "The Drawn field is read by settingsEditSwitchCells but no cell sets it yet (12-11 adds Media under text); TestSettingsEditSwitchGrid drives it with a hidden cell so the closing-up behaviour is proved now"
  - "The page text now contains the check mark in state lines, so the Phase 11 'no notice' assertions that searched the whole text for the OK notice icon use settingsEditHasNoticeLine (a notice line starts with the icon; a state line starts with its locale key)"

patterns-established:
  - "settingsEditHasNoticeLine: tell a notice line from a state line by line prefix"

requirements-completed: [EDIT-09]

coverage:
  - id: D1
    description: "The Welcome page draws Greet once (line, cell with the state icon and tg p=w t=o v=0/1); a press stores welcome_once, redraws from the read-back with no notice and one answer; a double tap writes the same value; a member who joins, leaves and joins again is welcomed once; /resetwelcome leaves it; a failed write redraws with the save-failed notice and the old state; private and group menu"
    requirement: "EDIT-09"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^(TestSettingsEditOnceSwitch|TestSettingsEditWelcomeSwitch|TestSettingsParityGreetingSwitches)$' ./fuku/modules"
        status: pass
    human_judgment: false
  - id: D2
    description: "The switches are a two-column grid within the row budget (Welcome 7 rows with nothing set, 8 private and 9 in a group with everything set), every label is icon, one space and the name in all 8 languages at most 20 code points, one state line per drawn switch between the part lines and the hints, Greet once on Welcome only, every callback at most 64 bytes with Omitted 0, every screen at most 3800 UTF-16 units in 8 languages"
    requirement: "EDIT-09"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^(TestSettingsEditSwitchGrid|TestSettingsEditPages|TestSettingsEditRegistry|TestSettingsEditCallbackBytes|TestSettingsEditPagesFitEveryLocale|TestSettingsEditDefaultsMatchRuntime)$' ./fuku/modules"
        status: pass
    human_judgment: false
  - id: D3
    description: "Greet once reads what the join reads (drift) and needs only the live admin check and the owner gate: any live administrator may press it, a plain member gets settings_not_admin and nothing is written, and over the four owner worlds it decides exactly as the press door of TestSettingsGateAllDoors"
    requirement: "EDIT-09"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^(TestSettingsDriftOnce|TestSettingsAuthorityOnce|TestSettingsGateAllDoors|TestSettingsControlValidate)$' ./fuku/modules"
        status: pass
    human_judgment: false
  - id: D4
    description: "The four label keys are renamed (text unchanged) in all 8 locales and the limits table; 74 settings_edit_, 16 settings_btn_edit_ and 199 settings_ line-anchored keys in every locale file"
    requirement: "EDIT-09"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 ./fuku/i18n"
        status: pass
    human_judgment: false

duration: 75min
completed: 2026-11-05
status: complete
---

# Phase 12 Plan 10: Greet once switch and the editor switch grid Summary

**The owner turns Greet once on from the Welcome page and a member is welcomed only the first time they ever join; the editor's switches are now a table-driven two-column grid of icon buttons with each state in words on a body line.**

## What changed

- `settings_edit.go`: `settingsEditSwitch` (control, label key, line key, hint key, right, value, drawn, apply) and `settingsEditPageDef.Switches` replace `ToggleLabelKey`, `CleanLabelKey` and `CleanHintKey`. Welcome has the cells e, d, o (Greet once: `settings_btn_edit_switch_once`, `settings_edit_line_once`, hint `settings_edit_hint_once`, `settingsRightAdmin`, `applyWelcomeOnce`), Goodbye e and d, Rules none. `settingsEditSwitchGrid` lays the drawn cells two to a row (an odd last one alone); `settingsEditBody` writes one state line per drawn cell after the part lines and the no-caption note, then the page hint and each cell's hint paragraph; `settingsEditSetSwitch` finds the cell by control and calls its `Apply` (adapter `settingsEditGreetingApply`); `init` registers `Controls` from the cells with `settingsEditSwitchValue`. `settingsEditState.Once` is copied from `WelcomeSettings.Once`. The page never reads `greeted_members`.
- `settings_pages.go`: `settingsStateIcon`.
- Locales: the four `settings_btn_edit_*_toggle` / `clean_*` keys are now `settings_edit_line_welcome`, `_goodbye`, `_clean_welcome`, `_clean_goodbye` (text unchanged) in all 8 files and in the limits and token rows of `settings_locale_test.go`; the full-width `editorSwitches` list and its 40-code-point loop and `editorSwitchMax` are gone (the 12-05 five-cell check covers the buttons).
- Tests: new `settings_edit_switch_test.go` (`TestSettingsEditOnceSwitch`, `TestSettingsEditSwitchGrid`, `TestSettingsDriftOnce`, `TestSettingsAuthorityOnce`); the Phase 11 editor tests moved to the new label keys and to icon-only state assertions; the mock-rows subtest, `TestSettingsEditRegistry` (Welcome e, d, o; Goodbye e, d; Rules none, o on Welcome only with `settingsRightAdmin`), `TestSettingsEditCallbackBytes`/`FitEveryLocale` (states with Greet once on and off) and `TestSettingsEditDefaultsMatchRuntime` (Greet once Off for a chat with no row, runtime agrees) were rewritten for the grid; `editCleanup` also deletes `greeted_members` rows.
- `AGENTS.md`: the `settings|v1` paragraph (a tg or st press must name a registered control, a control may declare `Validate` of `v`, both in step 1), the editor-pages bullet (grid, icon buttons, `settings_edit_line_*` body lines, controls e d o, Greet once is Welcome only, no parity test, `/resetwelcome` leaves it), Permissions (`settingsRightAdmin` covers the editor switches and Greet once), the greeted_members bullet (the write path now has its caller) and the Testing family list.

## TDD record

- **Task 1 (tracer) RED** `5c48945`: `TestSettingsEditOnceSwitch` failed in both menus on `the Welcome page ... lacks the off line` and `the page has no button labelled settings_btn_edit_switch_once` (assertions, not a build error; the only stub is the const `settingsEditControlOnce`). **GREEN** `ddbd7db`: the table, grid, control o, renames and AGENTS.md.
- **Task 2** `e25d944` and **Task 3** `a6a69c3`: written after the tracer had built the behaviour, so they passed at once (as the plan's "fix the code if one fails" anticipates); they are regression guards, not RED tests, and there is no separate `feat` for them. To show they are not vacuous I ran two temporary mutations (restored from a backup; the tree diff then showed only the test files): one switch per row failed `TestSettingsEditSwitchGrid` (four subtests) and `TestSettingsEditCallbackBytes`; giving Greet once `settingsRightChangeInfo` and reading `Enabled` failed `TestSettingsAuthorityOnce` (the administrator and the four-worlds subtests). `TestSettingsDriftOnce` alone does not catch a wrong `Value` because Welcome defaults to on, so `TestSettingsEditOnceSwitch` (default off) is the guard for that.
- `gsd_run check tdd-red-evidence` was not run (plan is `type: execute`); the semantic check of the RED is the `--- FAIL` lines above.

## Commits

- 5c48945 test(12-10): add failing test for the Welcome page's Greet once switch
- ddbd7db feat(12-10): the owner turns Greet once on from the Welcome page
- e25d944 test(12-10): the editor switches sit two to a row within every limit
- a6a69c3 test(12-10): Greet once drifts nowhere and refuses what it must

## Verification (real results)

- Task 1 gates: `-run '^(TestSettingsEditOnceSwitch|TestSettingsEditWelcomeSwitch|TestSettingsParityGreetingSwitches)$' ./fuku/modules` ok; `-run '^(TestSettingsLocaleKeys|TestSettingsLabelLengths|TestSettingsEditLocaleTokens)$' ./fuku/i18n` ok; the old key names no longer appear in `fuku` or `locales`; `grep -q 'applyWelcomeOnce(' fuku/modules/settings_edit.go` and the no-`greeted_members` invariant both hold; every locale file has 74 / 16 / 199 line-anchored `settings_edit_` / `settings_btn_edit_` / `settings_` keys.
- Task 2 gate: `-run '^(TestSettingsEditSwitchGrid|TestSettingsEditPages|TestSettingsEditRegistry|TestSettingsEditCallbackBytes|TestSettingsEditPagesFitEveryLocale|TestSettingsEditDefaultsMatchRuntime)$'` ok; no `ToggleLabelKey` or `CleanLabelKey` left in `settings_edit_test.go`.
- Task 3 gate: `-run '^(TestSettingsDriftOnce|TestSettingsAuthorityOnce|TestSettingsGateAllDoors|TestSettingsControlValidate)$'` ok; `settingsEditSwitch` and `settingsStateIcon` are in `AGENTS.md`.
- Plan end: `-run '^(TestSettings|TestLoadSettings)' ./fuku/modules ./fuku/i18n` ok (modules 198.2 s, i18n 5.1 s); `-run '^(TestGreetOnce|TestLegacyGreetings)' ./fuku/modules` ok; `go test -tags testtools -race -count=1 ./fuku/i18n` ok; `make check-translations` all present; `make generate-docs` then `make check-docs` no drift; `CGO_ENABLED=0 go build ./...` ok; `go vet -tags testtools ./fuku/modules ./fuku/i18n` clean; `gofmt -l` on the touched files empty; `git diff --exit-code 72a692b -- go.mod go.sum` clean.
- `python3 .../lint-local.py 72a692b`: exit 0 (no dupl, gocyclo, godox, staticcheck or errcheck findings on added lines) before the last code commit and again before this SUMMARY.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] The Phase 11 "no notice" assertions matched the new state lines**
- **Found during:** Task 1
- **Issue:** `TestSettingsEditReplaceWait`, `TestSettingsEditStaleCancel` and `TestSettingsEditExpiredPrompt` assert the page text holds no `settingsNoticeOKIcon`, but the OK icon is the same "✅ " that starts a state word, now in the page body.
- **Fix:** a test helper `settingsEditHasNoticeLine` (a notice line starts with the icon, a state line starts with its locale key); the three assertions and the new switch test use it. No production change.
- **Files modified:** `fuku/modules/settings_edit_test.go`, `fuku/modules/settings_edit_actions_test.go`, `fuku/modules/settings_edit_switch_test.go`

**2. [Documented] AGENTS.md edits landed in the Task 1 commit**
- The plan lists AGENTS.md under Task 3. The editor-pages and `settings|v1` text it rewrites became wrong with the Task 1 commit, so the edits went in there (AGENTS.md: update in the same commit as the invalidating change); Task 3 added the Testing family sentence.

**3. [Documented] Task 1 also rewrote the mock-rows subtest and `TestSettingsEditRegistry`**
- Needed for Task 1's code to compile against green tests; Task 2 added the grid, byte and locale-fit expectations on top.

## Known Stubs

None. `settingsEditSwitch.Drawn` is read but no cell sets it yet; plan 12-11 (Media under text) is its first user, and `TestSettingsEditSwitchGrid` exercises it with a hidden cell.

## Threat Flags

None. The plan's threats are mitigated as planned: T-12-30 (right, owner gate, `TestSettingsAuthorityOnce` over the four worlds), T-12-31 (12-06's control existence check and `Validate`, `TestSettingsControlValidate` re-run green), T-12-32 (the page reads only `welcome_once` through the side-effect-free loader; `TestSettingsDriftOnce`; 12-16 extends `TestSettingsStaffPrivacy`), T-12-33 (grid, row budget and byte tests).

## Not run

- `make test` (phase gate plan 12-16 only) and `make lint` (cannot run in this sandbox; the lint stand-in ran).
- PostgreSQL-only tests (`TestGreetedMemberClaimConcurrent`, `TestRulesMediaMigrationDefaults`): not part of this plan.
- The live-client check of the five icon-only buttons (UI-SPEC assumptions A3, A4, A10) is for the owner.

## Self-Check: PASSED

- `fuku/modules/settings_edit_switch_test.go`, `fuku/modules/settings_edit.go` and this file exist.
- Commits 5c48945, ddbd7db, e25d944 and a6a69c3 are on `claude/ecstatic-goldberg-qsss05` (`git merge-base --is-ancestor` for each).
