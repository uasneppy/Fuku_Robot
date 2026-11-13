---
phase: 17-command-edit-and-self-destruct-deletion
plan: 09
subsystem: settings-menu
tags: [settings-menu, deletion, member-messages, confirm-card, callbacks, tests]

requires:
  - phase: 17-command-edit-and-self-destruct-deletion
    provides: the three member kinds and their delay rules (17-01), the producers of the three rules (17-03 to 17-07), the 18 Member messages strings in 8 languages (17-08)
provides:
  - the Member messages section (m, icon 💬) of the Deletion page from the page's own tables, with rows mc, me and ms
  - the optional presentation fields of deletionRow (M31) and the settingsDelWords struct behind the one state function
  - the Self-destruct confirm card and its Yes, the page action dy
  - TestSettingsDeletionPhase16ScreensUnchanged, a characterization of the 2 Phase 16 lists and 17 row screens recorded before the renderer changed
affects: [17-10, 17-11, 17-12]

plan_head_before: 91e820d775a5a800a40065f62198d9a702691ded
plan_head_after: 237751816aa10dffffc34c0eb14a55ea022e09a9

actuals:
  tokens: 14500   # chars/4 over the added lines of 91e820d..2377518 in fuku/ and AGENTS.md (58022 chars), locale text none
  tasks: 3
  commits: 7      # MEASURED: git rev-list --count 91e820d..HEAD before this SUMMARY commit; 6 are this plan's, 1 (4582725, test(17-04), deletion_member_right_test.go, 4 lines) is another writer's

tech-stack:
  added: []
  patterns:
    - "a row's differences from its section are optional table fields (ChoiceRows, LabelKeys, ZeroKey, AfterKey, TimeKey, NoTime, ConfirmKey); no renderer, validator or gate names a row code"
    - "a card exists only for a row with a ConfirmKey and a delay, drawn by the control's Set through the shared settingsCardRows helper; its Yes is one page action that re-runs the press pipeline"

key-files:
  created:
    - fuku/modules/settings_deletion_golden_test.go
    - fuku/modules/settings_deletion_member_test.go
  modified:
    - fuku/modules/deletion_kinds.go
    - fuku/modules/settings_deletion.go
    - fuku/modules/settings_deletion_screens_test.go
    - fuku/modules/settings_deletion_test.go
    - fuku/modules/settings_deletion_right_test.go
    - fuku/modules/deletion_kinds_locale_test.go
    - AGENTS.md

key-decisions:
  - "renderSettingsDelCard takes (prep, row, value, notice), not the plan's (prep, section, row, value, notice): the section is not read by the card, and an unused parameter would be dead weight."
  - "settingsDelAsk (the card path) draws from one settingsDelPrepare (one rules read, one right read) and compares the stored delay from that read, so a card costs the same one getChatMember as every deletion screen; a nil rules read counts as nothing stored."
  - "settingsDelRunYes defers the stored-state check to the write: applyDeleteRule answers deletionUnchanged with no write, so a double Yes writes once and redraws the row."
  - "The test helper settingsDelEveryScreen now also renders the card of every row with a ConfirmKey (at its longest preset), so the bytes, fit and privacy sweeps cover it with no new loop; the no-card test skips those screens and checks cards live by pressing every choice."

patterns-established:
  - "characterization before change: digests of the old renders recorded green, never re-recorded, and the renderer change is judged against them"

requirements-completed: []
status: complete
---

# Phase 17 Plan 09: Member messages page Summary

**The Deletion page has its third section, Member messages (Commands, Edited messages, Self-destruct) built from the page's own tables, with Self-destruct switched on through the shared two-tap card, and every Phase 16 screen still renders byte-identically.**

## What changed

- `fuku/modules/deletion_kinds.go`: `deletionRow` gains seven optional presentation fields, each empty for every Phase 16 row. The member rows set them as the UI-SPEC table says: `mc` `ChoiceRows {2}`, `LabelKeys {0: settings_del_state_on}`, `ZeroKey settings_del_state_on`, `NoTime`; `me` `ChoiceRows {3,3}`, `AfterKey settings_del_state_grace`, `TimeKey settings_del_hint_grace`; `ms` `ChoiceRows {1,3,3}`, `ConfirmKey settings_del_confirm_self`.
- `fuku/modules/settings_deletion.go`: the `m` section (icon 💬, `settings_del_sec_member`, `settings_del_list_mem`, no list hints) appended to `settingsDelSections`; `settingsDelDelayKeys` gains 30, 300 and 900; `settingsDelWords` (none, zero, after keys) built by `settingsDelWordsOf` and fed to `settingsDelStateOf`; `settingsDelPresetLabel` and `settingsDelChoiceRows` (the row's layout, else the section's); `renderSettingsDelRow` draws the time line from `NoTime`, `TimeKey` or the shared hint; `renderSettingsDelCard`, `settingsDelAsk`, `settingsDelWrite`, `settingsDelStoredDelay`, `settingsDelYesFields` and `settingsDelRunYes`; the action `dy` (`settingsActDelYes`, fields `t`, `v`, `e`, `settingsRightChangeInfo`) registered beside `dl` and `dr`. `settings_pages.go`, `settings_menu.go` and `settings_card.go` are untouched. The controls of the three rows come from the existing `init` loop (`st` only, the per-row preset validator, `settingsRightChangeInfo`), so no new registration code names a row.
- Behaviour: Commands and Edited messages write at once through `applyDeleteRule`; Self-destruct Off is one tap that removes the row and (through `RemoveRule`) drops the chat's waiting `member:selfdestruct` rows; a Self-destruct delay equal to the stored one writes nothing and draws no card; any other delay writes nothing and draws the card (with the right line); Yes re-runs the whole press pipeline (a demoted presser gets `settings_no_change_info`), then the Staff Group gate, the expiry (`settings_card_expired`, nothing written), the write and the redraw from the read-back; No is `a=no&p=d`.
- Tests: `settings_deletion_golden_test.go` (76 digests: both lists and 17 row screens, no rule and first preset stored, with and without the 🚫 line) and `settings_deletion_member_test.go` (`TestSettingsDeletionMemberOpens`, `...MemberWrites`, `...MemberCard`, `...MemberCodesUnique`, `...MemberNavigation`); extended `TestSettingsDeletionRowBudget`, `...CallbackBytes`, `...NoCard`, `...States`, `...PageFitsEveryLocale` (through `settingsDelEveryScreen`), `...MainState`, `...Validation` (31 forged member and `dy` presses), `...StaffGroup` (four member presses), `TestDeletionKindKeysExist` (member table, every new field, `{delay}` templates).
- `AGENTS.md`: the Deletion page bullet describes the third section, the row fields, the card and `dy`, the hub and list shapes `[2 1 2]`, the 57-byte Yes, and the Main sentence says the Deletion line counts the member rules too.

## Phase 16 tests that changed (legitimate, named by the plan)

- `settings_deletion_test.go` `TestSettingsDeletionOpensFromMain`: hub widths `[2 2]` to `[2 1 2]`, the Member cell is `keyboard[1][0]`, the core Back moved to `keyboard[2][0]`, and the hub's Member state line is asserted.
- `settings_deletion_right_test.go`: the hub entry of `settingsDelScreens` has the before-key `settings_del_sec_member` and widths `[2 1 2]`.
- `settings_deletion_screens_test.go`: the hub width in the budget table is `[2 1 2]` (plus the member entries); `settingsDelWantState` takes a `settingsDelWantWords` struct (and `TestSettingsDriftDeletion` in `settings_deletion_test.go` calls it that way); `TestSettingsDeletionNoCard` has the card exception; `settingsDelEveryRuleSet` includes the three member rules.
- `settings_deletion_test.go` also gains `settingsDelMemberCases` and the member presses in `settingsDelForged`. No Phase 16 characterization was re-recorded; `TestSettingsDeletionPhase16ScreensUnchanged` passed unchanged after the renderer change.

## Verification (real output)

- `go test -tags testtools -race -count=1 -run '^(TestSettingsDeletion.*|TestDeletionKindKeysExist|TestSettingsMainGrid|TestSettingsMainPageFitsEveryLocale)$' -v ./fuku/modules`: all 24 tests PASS (`--- PASS` for each of `TestSettingsDeletionPhase16ScreensUnchanged`, `...MemberOpens`, `...MemberWrites`, `...MemberCard`, `...MemberCodesUnique`, `...MemberNavigation`, `...RowBudget`, `...CallbackBytes`, `...NoCard`, `...States`, `...PageFitsEveryLocale`, `...Validation`, `...MainState`, `TestDeletionKindKeysExist`, `TestSettingsMainGrid`, `TestSettingsMainPageFitsEveryLocale`), `ok ... 28.863s`, no `--- SKIP`.
- Task gates: the tracer gate after `f284d71` and the card gate after `e9ac3fe` (`-run '^(TestSettingsDeletion.*|TestSettingsMreqCard)$'`, 20 PASS) were green before each commit; the red commits `04a89fd` and `da82039` were red for the right reason (no section, no card).
- `go test -tags testtools -race -count=1 -run '^(TestSettings|TestLoadSettings)' ./fuku/modules ./fuku/i18n` once at the end: `ok github.com/uasneppy/fuku_robot/fuku/modules 538.012s` and `ok github.com/uasneppy/fuku_robot/fuku/i18n 8.192s`.
- `CGO_ENABLED=0 go build ./...`: ok. `go vet -tags testtools ./fuku/modules`: no output. `gofmt -l` over the nine files of this plan: no output.
- `python3 .planning/phases/13-member-requirements/handoff/lint-local.py 8c0c827`: exit 0, `no dupl, gocyclo or godox findings on lines added since 8c0c827` (exit code read directly, written to a file).
- `git diff --exit-code 8c0c827 -- go.mod go.sum`: exit 0. `git diff --exit-code 42c48c4 -- fuku/modules/settings_pages.go fuku/modules/settings_menu.go fuku/modules/settings_card.go`: exit 0. `grep -v '^\s*//' fuku/modules/settings_deletion.go | grep -cE '"(mc|me|ms)"'`: 0. `grep -c 'settingsActDelYes' ...`: 4; `grep -c 'settingsCardRows(\|settingsCardExpired(' ...`: 2; `grep -c '57 bytes' AGENTS.md`: 1.
- Mutation check: renaming `ConfirmKey` to a missing key made `TestDeletionKindKeysExist` fail in all 8 languages (8 "does not translate" lines); restored.

## Deviations from Plan

None - the plan was executed as written. Two small choices are listed under key-decisions (the card renderer's signature, the single prepare read in the card path). A foreign commit (`4582725`, a 4-line edit of `deletion_member_right_test.go` for plan 17-04) landed on the branch while this plan ran; it is not part of this plan and touches no file of it.

## Not run

- `make test`, `make lint`, `make check-docs`, `make check-translations` (the plan does not ask for them; `make lint` cannot run in this sandbox; no locale or docs-generator input changed).
- `gosec` (cannot run locally; the new code has no discarded error, no `math/rand` and no string literal assigned to a secret-like name).
- The live Telegram checks of the phase (phone-width cells, the claimed batch at Off) stay with plan 17-12's `17-SPIKE.md`.

## Known Stubs

None.

## Requirements

This plan contributes to DEL-03, DEL-04, DEL-05 and DEL-08 (the page that switches each rule, the grace presets, the self-destruct presets, the delete-right line on every new screen) and completes none; `requirements.mark-complete` was not called (plan 17-12 closes them).

## Self-Check: PASSED

- Files: `fuku/modules/settings_deletion_golden_test.go` FOUND, `fuku/modules/settings_deletion_member_test.go` FOUND.
- Commits: `4e543d5`, `04a89fd`, `f284d71`, `da82039`, `e9ac3fe`, `2377518` all ancestors of HEAD.
