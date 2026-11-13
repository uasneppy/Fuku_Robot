---
phase: 11-message-editor
plan: 09
subsystem: modules
tags: [settings-menu, message-editor, redis, set, cancel, remove, tdd]
requires:
  - phase: 11-message-editor
    provides: plan 11-05 apply functions (applyGreetingRemove*, applyRulesClear), plan 11-06 settingsRightChangeInfo, plan 11-07 the Redis wait store, plan 11-08 the editor pages
provides:
  - the Set, Cancel and Remove actions of the Welcome (ws wr wc), Goodbye (gs gr gc) and Rules (rs rr rc) pages
  - the part rows ([Set] or [Set | Remove]) in renderSettingsEditor
  - the waiting prompt screen (renderSettingsEditPrompt) and settingsEditPromptMax
  - settingsEditValidatePart, the Validate hook that refuses a forged t before any I/O
affects: [11-10, 11-11, 11-12, 11-13, 11-14, settings-message-editor]

actuals:
  tokens: 27000
  tasks: 3
  commits: 4

commits: 4
plan_head_before: da1ed6da7bedf0434b3c185565186f8b6ed8b234
plan_head_after: f3af15f91abb685333efdfe2bd98c7c128b87469

tech-stack:
  added: []
  patterns:
    - "Actions of one page family come from one table-driven builder (settingsEditActions) with a private-only wrapper (settingsEditPrivateOnly), so group-menu refusal is written once"
    - "Cancel reads the wait through one compare-by-(group, page, part) script and maps each outcome to one honest answer"
    - "Remove reuses the commands' apply functions and redraws from the read-back, reloading only when the read-back is missing"

key-files:
  created:
    - fuku/modules/settings_edit_actions.go
    - fuku/modules/settings_edit_actions_test.go
  modified:
    - fuku/modules/settings_edit.go
    - fuku/modules/settings_edit_test.go
    - AGENTS.md
    - .planning/REQUIREMENTS.md

key-decisions:
  - "The text prompt's limit is 1024 while the stored media is set (a file ID of a known kind), not whenever the raw type column is not text, so a zero-valued state never shows the caption limit."
  - "Remove from a stale button on an empty part redraws with a warning and writes nothing; a failed write reloads the page so the old state and the save-failed notice are drawn."
  - "The editor actions carry an EditRight on the page definition (ChangeInfo for Welcome and Goodbye, AdminBotAdmin for Rules); Cancel always needs only the live admin check."

requirements-completed: [EDIT-01]

coverage:
  - id: D1
    description: "Each part has its row, [Set] while not set and [Set | Remove] while set, in the order of the state lines, drawn in group and private menus"
    requirement: EDIT-01
    verification:
      - kind: unit
        ref: "fuku/modules/settings_edit_test.go#TestSettingsEditPages"
        status: pass
      - kind: unit
        ref: "fuku/modules/settings_edit_test.go#TestSettingsEditCallbackBytes"
        status: pass
    human_judgment: false
  - id: D2
    description: "Set in a private menu writes the one ten-minute wait and turns the message into the prompt with its end time and one Cancel button; replace, refresh, no-Redis, stale and expired cases answer as the contract says"
    requirement: EDIT-02
    verification:
      - kind: unit
        ref: "fuku/modules/settings_edit_actions_test.go#TestSettingsEditSetWritesWait"
        status: pass
      - kind: unit
        ref: "fuku/modules/settings_edit_actions_test.go#TestSettingsEditReplaceWait"
        status: pass
      - kind: unit
        ref: "fuku/modules/settings_edit_actions_test.go#TestSettingsEditSetUnavailable"
        status: pass
      - kind: unit
        ref: "fuku/modules/settings_edit_actions_test.go#TestSettingsEditStaleCancel"
        status: pass
      - kind: unit
        ref: "fuku/modules/settings_edit_actions_test.go#TestSettingsEditExpiredPrompt"
        status: pass
    human_judgment: false
  - id: D3
    description: "Cancel ends the wait and returns to the page; a menu opened in the group never creates a wait; forged or extra fields are refused before any lookup"
    requirement: EDIT-02
    verification:
      - kind: unit
        ref: "fuku/modules/settings_edit_actions_test.go#TestSettingsEditCancelButton"
        status: pass
      - kind: unit
        ref: "fuku/modules/settings_edit_actions_test.go#TestSettingsEditGroupMenuRefusal"
        status: pass
      - kind: unit
        ref: "fuku/modules/settings_edit_actions_test.go#TestSettingsEditCallbackFields"
        status: pass
    human_judgment: false
  - id: D4
    description: "Remove takes a part out at once through the commands' apply functions; the three Remove presses equal /resetwelcome, /resetgoodbye and Rules Remove equals /clearrules; rights mirror the commands"
    requirement: EDIT-01
    verification:
      - kind: unit
        ref: "fuku/modules/settings_edit_actions_test.go#TestSettingsEditRemove"
        status: pass
      - kind: unit
        ref: "fuku/modules/settings_edit_actions_test.go#TestSettingsEditRemoveStaleAndFailed"
        status: pass
      - kind: unit
        ref: "fuku/modules/settings_edit_actions_test.go#TestSettingsEditRemoveParity"
        status: pass
      - kind: unit
        ref: "fuku/modules/settings_edit_actions_test.go#TestSettingsAuthorityParityEdit"
        status: pass
    human_judgment: false
  - id: D5
    description: "The prompt and every page state keep every button within 64 bytes and fit 3800 UTF-16 units in all 8 languages"
    requirement: EDIT-01
    verification:
      - kind: unit
        ref: "fuku/modules/settings_edit_test.go#TestSettingsEditPagesFitEveryLocale"
        status: pass
    human_judgment: false

duration: 75min
completed: 2026-11-04
status: complete
---

# Phase 11 Plan 09: Set, Cancel and Remove on the editor pages Summary

**Every part of Welcome, Goodbye and Rules now has a Set button (and Remove while set) on its page; Set in the private menu writes the one ten-minute Redis wait and shows a prompt with its end time and Cancel, Cancel and Remove behave as the contract says with the commands' rights and stored results, and a menu opened in the group only points to the private chat.**

## What changed

- `fuku/modules/settings_edit.go`: the parts table gained `SetLabelKey`, `RemoveLabelKey`, `RemovedKey` and `WaitKey` (Rules has its own text part); the page definitions gained `SetCode`, `RemoveCode`, `CancelCode` and `EditRight`; `renderSettingsEditor` has a part-rows block right after the body; the init registers the three actions per page through `settingsEditActions`.
- `fuku/modules/settings_edit_actions.go` (new): `settingsEditValidatePart`, `settingsEditPromptMax` (Rules 3800, text 1024 with stored media, else 4096), `renderSettingsEditPrompt` (pure; body, UTC end time, replaced note, one Cancel button), `settingsEditSet`, `settingsEditCancel`, `settingsEditRemove` and `settingsEditApplyRemove`. A shared `settingsEditPrivateOnly` wrapper sends a group menu's press to the group note.
- `fuku/modules/settings_edit_actions_test.go` (new) and `settings_edit_test.go` (extended): 15 new tests plus the part-rows subtest and prompt coverage in the byte and fit tests.
- `AGENTS.md`: the editor pages bullet now describes the part rows, the nine action codes with their rights, `Validate`, the private-only wait, the prompt, Cancel's compare, Remove's path and the new test families.
- `.planning/REQUIREMENTS.md`: EDIT-01 ticked (checkbox and traceability row).

## Task commits

| Task | Commit | Subject |
|------|--------|---------|
| 1 (tracer) | 22d8584 | feat(11-09): add Set and Cancel to the message editor pages |
| 2 | 57daa02 | test(11-09): hold the wait to its contract for replace, stale, expired, unavailable, group menus and forged fields |
| 3 RED | 12d2469 | test(11-09): add failing tests for Remove, its parity with the reset and clear commands, and the editor rights |
| 3 GREEN | f3af15f | feat(11-09): add Remove to the message editor pages |

## TDD notes

- Task 3 RED (12d2469): with Remove unregistered the run failed on planned assertions: `TestSettingsEditRemove`, `TestSettingsEditRemoveStaleAndFailed`, `TestSettingsEditRemoveParity` (all three subtests), `TestSettingsAuthorityParityEdit` (three subtests), the `wr/gr/rr` cases of `TestSettingsEditGroupMenuRefusal`, and the new part-rows subtest of `TestSettingsEditPages`. GREEN (f3af15f) passed all of them. `gsd_run check tdd-red-evidence` was not run.
- Task 2: the plan puts the group-menu branch, replaced flag, stale/expired/unavailable answers and `settingsEditPromptMax` into Task 1's action, so the Task 2 tests passed on first run (after fixing test-side mistakes). I checked one of them is not vacuous by mutation: making the group branch never fire made `TestSettingsEditGroupMenuRefusal` fail; the change was reverted.
- To keep Task 1's commit free of dead buttons, I held the Remove code (RemoveCode values, Remove buttons, `settingsEditRemove`) out of it and added it in Task 3 GREEN.

## Verification (real results)

- Task 1 command (`TestSettingsEditSetWritesWait|TestSettingsEditCancelButton`): ok.
- Task 2 command (seven tests): ok.
- Task 3 command (nine tests incl. `TestSettingsEditRemoveStaleAndFailed`, group menu, callback fields): ok.
- Whole settings set `go test -tags testtools -race -count=1 -run '^(TestSettings|TestLoadSettings)' ./fuku/modules ./fuku/i18n`, run after Task 1 and at the final code state: `ok fuku/modules 140.602s`, `ok fuku/i18n 6.068s` (final).
- `CGO_ENABLED=0 go build ./...`: ok. `go vet -tags testtools ./fuku/modules`: clean. `git diff --exit-code 72a692b -- go.mod go.sum`: exit 0. `gofmt -l` on the settings_edit files prints nothing.
- `python3 .../lint-local.py 72a692b`: "no dupl, gocyclo or godox findings" (before each code commit).
- Acceptance greps: `settingsInputSet(` 1 and `settingsInputCancel(` 1 in the actions file; six codes `ws gs rs wc gc rc` and three `wr gr rr` in settings_edit.go; `applyGreetingRemove*(`/`applyRulesClear(` 4; `settings_edit_group_note` in AGENTS.md 1.
- Not run: `make lint` (golangci-lint here is built with Go 1.25), `make test`, `make check-translations` and `make check-docs` (no locale or docs source changed), PostgreSQL targets (nothing here touches the schema).

## Deviations from Plan

None in behaviour. Notes:

- No earlier test's expectation changed. The plan expected `TestSettingsEditPages`, `TestSettingsEditCallbackBytes` and `TestSettingsEditPagesFitEveryLocale` to need new rows; they still passed with the part rows (they count rows and bytes only), so I extended them (a part-rows subtest and the prompt screens) rather than correcting them.
- `settingsEditPromptMax` keys the caption limit on stored media being set (`settingsEditPartIsSet` for media) instead of `MediaType != db.TEXT`: `db.TEXT` is 1, so a zero-valued type would otherwise read as media.
- A test-only detail: the private menu can be opened once per test through the deep link, so later pages in a test switch with an `op` press (`settingsEditSwitchPage`).

## Requirements

- EDIT-01 is COMPLETE and ticked (checkbox and traceability row): the pages show which parts are set with a Set or Remove button each (Rules has the text part only, as the plan states).
- EDIT-02 stays Pending: Set starts the wait and Cancel ends it, but the next private message is read by the door in plan 11-11, which completes it. `requirements-completed` lists EDIT-01 only.

## Known Stubs

None. The Set prompt waits for a typed reply that plan 11-11's door answers; until then a reply is not read, by design of the plan order.

## Threat Flags

None beyond the plan's register. The new surface is the nine callback actions, all behind the existing press pipeline, `Validate`, and per-action rights (T-11-29 to T-11-33 are covered by `TestSettingsEditCallbackFields`, `TestSettingsAuthorityParityEdit`, `TestSettingsEditGroupMenuRefusal`, `TestSettingsEditReplaceWait` and `TestSettingsEditSetUnavailable`).

## Self-Check: PASSED

- Created files exist: `fuku/modules/settings_edit_actions.go`, `fuku/modules/settings_edit_actions_test.go`.
- Commits 22d8584, 57daa02, 12d2469 and f3af15f are ancestors of HEAD; `git rev-list --count da1ed6d..HEAD` printed 4 before this SUMMARY.
