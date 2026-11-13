---
phase: 13-member-requirements
plan: 06
subsystem: settings-menu
tags: [settings-menu, member-requirements, mreq, rule-grid, rule-screen, confirm-card, action-allow-list]
requires:
  - phase: 13-member-requirements
    provides: page m, renderSettingsMreqPage, memberReqActionIcon, memberReqWrites, memberReqApplyResult, finishMemberReqApply (13-05); memberreq.SetRuleAction / GetSettingsFresh / ActionOf (13-01); the memberReqRules table with CellKey and DescKey (13-02); the settings_mreq_* and settings_btn_mreq_* keys (13-04)
provides:
  - settingsControl.Actions (an optional allow-list of core action codes, nil keeps tg and st) read by settingsControlFieldsValid(action, fields) in step 1
  - the rule grid on the page (five cells, two per row, the odd last alone) and the rule screen renderSettingsMreqRuleScreen (pure), page action mo
  - rule controls ln un ru ar zh (st only, v one digit 0..4, settingsRightRestrict)
  - applyMemberReqAction(ctx, chatID, rule, action) and the memberReqWrites.Action seam
  - the confirm card renderSettingsMreqCard for Kick, Ban and Mute and the page action my (settingsActMreqYes, fields t v e, settingsRightRestrict)
  - the settingsMreqFresh seam (the fresh database read of the same-value check)
affects: [13-07, 13-09, 13-10, 13-11, 13-13, member-requirements]

actuals:
  tokens: 14500
  tasks: 3
  commits: 5

commits: 5
plan_head_before: 0a57c0fdf9ba6c8f636e7f1bc4eb6eec6a0b8a22
plan_head_after: 28464855b6dd91b52a757ac115e7ed686dbe3255

tech-stack:
  added: []
  patterns:
    - "A control declares which core actions it accepts (Actions); the refusal is in step 1, never inside Set after the member lookup"
    - "A strict choice writes nothing until the card's Yes; the same-value check reads the database fresh through a seam, never the cache"

key-files:
  created:
    - fuku/modules/settings_mreq_card_test.go
  modified:
    - fuku/modules/settings_mreq.go
    - fuku/modules/memberreq_settings.go
    - fuku/modules/settings_pages.go
    - fuku/modules/settings_menu.go
    - fuku/modules/settings_mreq_test.go
    - fuku/modules/settings_staff_privacy_test.go
    - AGENTS.md

key-decisions:
  - "The same-value read goes through the variable settingsMreqFresh (a one-line wrapper over memberreq.GetSettingsFresh) so a test can make it fail; the apply function's own read-back keeps calling GetSettingsFresh directly"
  - "applyMemberReqAction validates the rule (memberreq.Rules) and the value 0..4 itself before the write seam, with the repository's sentinel errors, so a caller that skips the control's Validate still writes nothing"
  - "settingsMreqResult is the one unpacking of an apply result for a redraw, shared by the switches and the rule screen (it also keeps dupl quiet)"
  - "The rule screen's Back is built by hand (a=op, p=m); the shared navRow stays Back to Main for every other screen, so the card's Back opens Main as the contract says"

patterns-established:
  - "settingsMreqCountWrites wraps the write seam with a counter so a test proves a press wrote nothing without reading rows"

requirements-completed: []

coverage:
  - id: D1
    description: "The page carries a five-cell rule grid above the switch row; each cell opens its rule's screen (a=mo, exactly t); the rule screen shows the name, what breaks it (the blank hint on the last-name rule only), Punishment, both hints, [Off | Warn] and [Kick | Ban | Mute] with the check mark on the stored value, and Back to the page"
    requirement: "MREQ-01"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^(TestSettingsMreqRuleScreen|TestSettingsControlActionAllowList)$' -v ./fuku/modules (RED at c49bc39 on assertions, GREEN at 71c895e)"
        status: pass
    human_judgment: false
  - id: D2
    description: "Off and Warn write at once through applyMemberReqAction (a Go 0 written, the cache key dropped) and redraw the rule screen; a press of the stored value, judged by a fresh database read, writes nothing and draws no card; a failed fresh read redraws with the save-failed notice and writes nothing"
    requirement: "MREQ-01"
    verification:
      - kind: unit
        ref: "TestSettingsMreqRuleScreen, TestApplyMemberReqAction, TestSettingsMreqCard (the stale-cache subtest changes the row behind a cached Off and a Mute press draws no card while a Ban press does)"
        status: pass
    human_judgment: false
  - id: D3
    description: "Kick, Ban and Mute write nothing and draw the card for each of the five rules; Yes writes, an expired Yes writes nothing and redraws the rule screen with settings_card_expired, No redraws the page, the card's Back opens Main; a presser demoted between the taps is refused at Yes with settings_no_restrict; a failed write redraws with common_settings_save_failed; forged my fields are refused before any getChatMember"
    requirement: "MREQ-01"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^TestSettingsMreqCard$' -v ./fuku/modules (RED at 40a0f3b on assertions, GREEN at 4be02fd)"
        status: pass
    human_judgment: false
  - id: D4
    description: "A tg on a rule control is an invalid request in step 1 with no getChatMember; st is accepted; st with v 5, 04, x, empty, -1 or 10 is refused; tg and st stay accepted on j and d and on every control of the other pages (nil Actions)"
    requirement: "MREQ-01"
    verification:
      - kind: unit
        ref: "TestSettingsControlActionAllowList, TestSettingsControlValidate and TestSettingsPageActionValidate (unchanged, pass)"
        status: pass
    human_judgment: false
  - id: D5
    description: "An administrator without can_restrict_members opens the page and every rule screen and is refused every choice (v 0..4) and every Yes with settings_no_restrict, nothing drawn, edited or written; the owner gate still decides before the right"
    requirement: "MREQ-01"
    verification:
      - kind: unit
        ref: "TestSettingsAuthorityMreq. Negative proof: with the my action temporarily on settingsRightAdmin the test failed on 'my ln v=2 edited the menu' and 'answered \"\"'; restored"
        status: pass
    human_judgment: false
  - id: D6
    description: "Worst-case bytes (rule cell 38 private and 34 group, choice 46 and 42, card Yes 53 and 49, card No 37 and 33, rule screen Back 37 and 33), Omitted 0, the page 5 rows, a rule screen 3 rows, a card 2 rows, the page plus the five rule screens (each under every notice) and the fifteen cards within 3800 UTF-16 units in 8 languages with 64-rune titles, and none of them names the Staff Group"
    requirement: "MREQ-13"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^(TestSettingsMreqRowBudget|TestSettingsMreqCallbackBytes|TestSettingsMreqPageFitsEveryLocale|TestSettingsStaffPrivacy)$' -v ./fuku/modules. Negative proof: with the my code temporarily 'myy' TestSettingsMreqCallbackBytes failed on 54 bytes and on the code shape; restored"
        status: pass
    human_judgment: false

duration: 70min
completed: 2026-11-05
status: complete
---

# Phase 13 Plan 06: Rule grid, rule screens, the confirm card and the control action allow-list Summary

**The Member requirements page now lets the owner give each of this phase's five rules (must have a last name, must have a username, no Russian name, no Arabic name, no Chinese name) its own punishment, all off by default; MREQ-01 stays Pending because its text names eight rules and the other three arrive through MREQ-11, MREQ-12 and MREQ-14.**

## Accomplishments

- Task 1 (tracer): the rule grid on the page (icon plus short name, two per row, the odd last alone) opens a rule screen through the page action `mo`. The rule screen has the two choice rows and Back to the page. Off and Warn write at once through the new `applyMemberReqAction` and redraw the rule screen; a press of the stored value, judged by a fresh read, writes nothing. `settingsControl` gained `Actions` and `settingsControlFieldsValid` now takes the action, so a `tg` naming a rule control is refused in step 1 before any lookup while every existing control keeps accepting `tg` and `st`.
- Task 2: Kick, Ban and Mute write nothing and draw the shared card (`renderSettingsMreqCard`, `settingsCardRows`). Yes is the page action `my` (exactly `t`, `v` 2..4, `e`, right `settingsRightRestrict`): it runs the whole pipeline again, so a demoted presser is refused, an expired Yes writes nothing, and a live Yes writes through `applyMemberReqAction`. The four exits are asserted: Yes and an expired Yes redraw the rule screen, No redraws the page, Back opens Main.
- Task 3: tests for the authority of every choice and Yes, the row budget, the worst-case bytes, the 8-language fit of the rule screens and cards, and the Staff Group privacy check; AGENTS.md describes the grid, the rule screen, the card, the `Actions` allow-list and the rights.
- Nothing here restricts, kicks, bans, warns or deletes a member: the punishments are only stored. No watcher, no enforcement and no kill switch exist in this plan.

## Task Commits

1. Task 1 RED: `c49bc39` test(13-06): add failing tests for the rule screens and the control action allow-list
2. Task 1 GREEN (tracer): `71c895e` feat(13-06): choose a rule's punishment on its own screen
3. Task 2 RED: `40a0f3b` test(13-06): add failing tests for the Kick, Ban and Mute confirm card
4. Task 2 GREEN: `4be02fd` feat(13-06): confirm kick, ban and mute for a rule on a card
5. Task 3: `2846485` test(13-06): punishment choices need the restrict right and fit every limit (tests and AGENTS.md)

RED evidence. Task 1 RED used two compile-only stand-ins, said here as the plan permits: the `memberReqWrites.Action` field (the real `memberreq.SetRuleAction`) and a `settingsControlFieldsValid(_ string, fields)` signature that ignored the action. It failed on assertions: `the page has 2 keyboard rows, want the three grid rows...`, `the rule screen ... lacks "@@settings-mreq-desc-lastname@@"`, `stored = {... row false}, want only the Arabic rule on Warn`, `st ln v=1 stored [0 0 0 0 0], want the last-name rule on Warn`. Task 2 RED needed no stand-in and failed on assertions in 19 subtests (a strict press still redrew the rule screen and there was no `my`). Task 3 is a TDD task whose tests pass at once on the code of tasks 1 and 2, so there is no RED commit for it; the authority and byte tests were instead shown to fail under two deliberate mutations (D5 and D6 above, both restored). The tracer's verify tests were re-run end to end after the expansion tasks (they are part of the green runs below). The `gsd_run check tdd-red-evidence` classifier was not run (the plan is `type: execute`).

## Verification (real results)

- `go test -tags testtools -race -count=1 -run '^(TestSettingsMreqRuleScreen|TestSettingsControlActionAllowList|TestSettingsControlValidate|TestSettingsPageActionValidate|TestSettingsMreqSwitches|TestSettingsMreqOpensFromMain)$' -v ./fuku/modules`: PASS at `71c895e`.
- `go test ... -run '^(TestSettingsMreqCard|TestSettingsMreqRuleScreen)$'`: PASS at `4be02fd`.
- `go test ... -run '^(TestSettingsAuthorityMreq|TestSettingsMreqRowBudget|TestSettingsMreqCallbackBytes|TestSettingsMreqPageFitsEveryLocale|TestSettingsDriftMreq|TestSettingsStaffPrivacy)$' -v ./fuku/modules`: all six PASS at `2846485` (`ok fuku/modules 15.190s`).
- The settings family, run once at plan end on SQLite in the background to `/tmp/claude-0/-home-user-Fuku-Robot/4ce545b5-6832-57a3-a863-43c972932c7c/scratchpad/13-06-family.txt`: `go test -tags testtools -race -count=1 -run '^(TestSettings|TestLoadSettings)' ./fuku/modules ./fuku/i18n` gave `ok fuku/modules 291.393s`, `ok fuku/i18n 5.334s`, `exit=0`.
- `gofmt -l` on every file I touched: empty. `go vet -tags testtools ./fuku/modules`: clean. `CGO_ENABLED=0 go build ./...`: ok. `make check-translations`: "All translations are present!". `make check-docs`: no drift. `git diff --exit-code 8c0c827 -- go.mod go.sum`: exit 0.
- Lint stand-in `python3 .../lint-local.py 8c0c827`: "no dupl, gocyclo or godox findings on lines added since 8c0c827", run after Task 1, before the last code commit and again before this SUMMARY.
- Acceptance greps: one `Actions []string` in `settings_pages.go`; `control.Actions`... present in `settings_menu.go` (as `control.Actions`); `func applyMemberReqAction(` once; `settingsCardRows(` and `settingsCardExpired(` in `settings_mreq.go`; `grep -c 'settingsActMreqYes\|a=my' AGENTS.md` prints 1; `renderSettingsMreqRuleScreen`/`renderSettingsMreqCard` appear in `settings_staff_privacy_test.go`.
- Not run: `make lint` (golangci-lint is built with Go 1.25 here and refuses Go 1.26.0), `make test` (not this plan's task), PostgreSQL (nothing here rests on it; the `fuku/modules` harness runs on SQLite).

## Deviations from Plan

- **Requirements bookkeeping (a decision against the plan's coverage note).** The plan says it completes MREQ-01 and asks for `requirements-completed: [MREQ-01]`. MREQ-01's text names eight rules and this phase delivers five of them (profile photo, linked-channel subscription and AI-flagged spam name arrive with MREQ-11, MREQ-12 and MREQ-14 in Phases 14 and 15), so by the executor brief's rule (a requirement is ticked only when its whole text is true after the plan) `requirements-completed` is `[]`, MREQ-01 is left Pending in REQUIREMENTS.md (checkbox and traceability row) and `requirements.mark-complete` was not run. The Member requirements page now lets the owner give each of this phase's five rules (must have a last name, must have a username, no Russian name, no Arabic name, no Chinese name) its own punishment, all off by default; MREQ-01 stays Pending because its text names eight rules and the other three arrive through MREQ-11, MREQ-12 and MREQ-14.
- **The lint stand-in path.** The plan names `.planning/phases/11-message-editor/handoff/lint-local.py`, which does not exist; the scratchpad stand-in from the executor brief was used (exit 0).
- **No RED commit for Task 3** (see above): guards that pass at once, proven by mutation instead.
- **Changed existing 13-05 tests (Rule 1, the new behaviour obviously requires it).** `settingsMreqWantSwitchRow` and `TestSettingsMreqCallbackBytes` now expect the five-row page (the grid, the switch row at index 3, the navigation row at index 4) instead of two rows; in `TestSettingsMreqSwitches` the refused-value cases `{"st", "ln", "1"}` was replaced by `{"st", "zz", "1"}` and `{"tg", "ln", "1"}`, because `st` on `ln` is now a valid rule press and only the `tg` form is refused. `TestSettingsControlValidate` and `TestSettingsPageActionValidate` are unchanged and pass.
- **Added beyond the plan's list:** `TestApplyMemberReqAction` (the apply function refuses an unknown rule and a value outside 0..4 before the write seam, drops the cache key, writes a Go 0) and the `settingsMreqFresh` seam (so the failed-fresh-read path is testable).
- **`settings_staff_privacy_test.go` live run:** the private admin run now also presses `mo`, a strict choice, its Yes and a Warn press on the rule controls.

No Rule 2 to 4 changes were needed.

## Known Stubs

None. Nothing is hard-coded empty or placeholder. The punishments are stored but not yet enforced: the watcher, the join entry and the kill switch (`memberReqEnforcementLive`) are later plans (13-09 and 13-13) by design, so a rule set here has no effect on members yet; that is a planned boundary, not a stub. No broken-windows ledger entry was needed.

## Threat Flags

None. No endpoint, auth path, file access or schema was added. T-13-15 is mitigated by `TestSettingsMreqCard` (Kick, Ban and Mute write nothing before Yes, and the card says it acts on every member), T-13-16 by `TestSettingsMreqCard` and `TestSettingsAuthorityMreq` (Yes runs the whole pipeline and the restrict right, `my` carries only v 2..4 and a well-formed e), T-13-17 by `TestSettingsControlActionAllowList` (the `Actions` allow-list and `Validate` in step 1), T-13-18 by the stale-cache subtest (the same-value check reads the database), T-13-SC by the unchanged `go.mod` and `go.sum`.

## Requirements

`requirements-completed: []`. The plan contributes to MREQ-01 (and the page's size limits to MREQ-13) and completes none: MREQ-01 stays Pending in REQUIREMENTS.md, see the bookkeeping deviation above.

## Notes for the owner

- A punishment choice, the card's Yes and both switches all need the right to restrict members (`settingsRightRestrict`, UI-SPEC Assumption A4); opening the page and a rule screen needs only the admin check.
- The page now has a `skipLocal` gap only for the cached read the page and the watcher share (added with the watcher); the same-value check does not depend on it because it reads the database.

## Self-Check: PASSED

- FOUND: fuku/modules/settings_mreq.go, fuku/modules/memberreq_settings.go, fuku/modules/settings_mreq_test.go, fuku/modules/settings_mreq_card_test.go.
- FOUND commits (all ancestors of HEAD): c49bc39, 71c895e, 40a0f3b, 4be02fd, 2846485.
