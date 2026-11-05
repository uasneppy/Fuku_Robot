---
phase: 12-rules-media-media-under-text-and-greet-once
plan: 12
subsystem: settings-editor
tags: [rules, media, buttons, media-under-text, clearrules, settings-menu, remove-parity]
requires:
  - phase: 12-rules-media-media-under-text-and-greet-once
    provides: 12-03 rules writers, LoadRulesSettingsContext and ClearRules, 12-07 buildRulesMessage and the rules sender, 12-10 and 12-11 the settingsEditSwitch table, settingsEditSwitchGrid, Drawn, settingsEditStaleSwitch
provides:
  - applyRulesMedia, applyRulesButtons, applyRulesMediaBelow and applyRulesClear over rules.ClearRules, rulesApplyResult.Row, the rulesWrites seam fields
  - the Rules page with the parts x m b, settingsEditRulesStateOf, the settingsEditRulesRead seam on rules.LoadRulesSettingsContext
  - Remove per Rules part (text, media, buttons), equal to /clearrules together
  - the Rules u switch (Media under text, settingsRightAdminBotAdmin) and Main reading Set by HasContent
affects: [12-13, 12-14, 12-16]

actuals:
  tokens: 11200
  tasks: 3
  commits: 7

commits: 7
plan_head_before: 703057546f7b8eae57c8a4fda129b9b613787177
plan_head_after: 1a1c6b4a7ade86056910aa1727a121f965f2857b

tech-stack:
  added: []
  patterns:
    - "settingsEditRulesApply adapts a rulesApplyResult to a switch cell the way settingsEditGreetingApply does for the greetings; Remove on Rules is one part switch over three apply functions"

key-files:
  created:
    - fuku/modules/settings_edit_rules_test.go
  modified:
    - fuku/modules/rules.go
    - fuku/modules/settings_edit.go
    - fuku/modules/settings_edit_actions.go
    - fuku/modules/settings_edit_test.go
    - fuku/modules/settings_edit_actions_test.go
    - fuku/modules/rules_apply_test.go
    - fuku/modules/settings_input_test.go
    - AGENTS.md

key-decisions:
  - "applyRulesButtons takes []models.Button (the same type as db.Button, an alias) so rules.go needs no new import"
  - "The Rules u cell's right is the literal settingsRightAdminBotAdmin, equal to the page's EditRight; TestSettingsEditRegistry asserts it for all three pages so a drift fails"
  - "Preview rows of the Rules media and buttons parts are drawn now (settingsEditPreviewable is generic) and their press still sends the old text preview; 12-14 replaces that, as the plan says"

patterns-established:
  - "rulesApplyResult.Row carries the whole read-back row; Rules (the text) stays for the commands and rules_apply_test.go"

requirements-completed: []

coverage:
  - id: D1
    description: "The owner opens Rules and sees the text, media and buttons lines with three [Set | Remove] rows; Remove media clears just the file and the type (text and buttons kept) and redraws with the removed-media notice; Set media writes the wait for part m"
    requirement: "EDIT-08"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^(TestSettingsEditRulesParts|TestSettingsEditPages)$' ./fuku/modules"
        status: pass
    human_judgment: false
  - id: D2
    description: "The three Remove presses store exactly what /clearrules stores (no text, file, type 0, no buttons) and leave Media under text, private rules and the rules button; a stale Remove writes nothing; a failed write redraws the old state with the save-failed notice"
    requirement: "EDIT-08"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^(TestSettingsEditRulesRemoveParity|TestSettingsEditRemoveParity|TestRulesApplyParity|TestSettingsEditRemoveStaleAndFailed)$' ./fuku/modules"
        status: pass
    human_judgment: false
  - id: D3
    description: "Main reads Set for rules with only a photo or only buttons and Not set for a row with only Media under text, the private switch and the label; a failed read is the unknown mark and opening the pages creates no rules row"
    requirement: "EDIT-08"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^(TestSettingsEditRulesMainState|TestSettingsEditReadFailure|TestSettingsEditReadsCreateNothing)$' ./fuku/modules"
        status: pass
    human_judgment: false
  - id: D4
    description: "Rules Set, Remove and Cancel accept t=m and t=b; t=z or a missing t is an invalid request before any lookup; Set and Remove need the bot as administrator (notice chat_status_bot_not_admin, nothing written) and write nothing in a group menu (group note, Open in private row)"
    requirement: "EDIT-08"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^(TestSettingsEditCallbackFields|TestSettingsEditRulesCallbackParts|TestSettingsAuthorityParityEdit|TestSettingsEditGroupMenuRefusal)$' ./fuku/modules"
        status: pass
    human_judgment: false
  - id: D5
    description: "The Rules page offers Media under text only for a photo, video or animation; the press stores media_below through applyRulesMediaBelow, /rules then sends the photo with show_caption_above_media true, a press after the media became a sticker writes nothing and warns settings_edit_below_unavailable, the bot must be an administrator, t=o, t=e and t=d on p=r are invalid requests"
    requirement: "EDIT-07"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^TestSettingsRulesMediaBelow$' ./fuku/modules"
        status: pass
    human_judgment: false
  - id: D6
    description: "Registry (Welcome 4, Goodbye 3, Rules 1 controls, u's right is each page's EditRight), callback bytes with Omitted 0 and Rules at most 7 rows private and 8 in a group menu, 3800 UTF-16 units in all 8 languages for the Rules states including a photo"
    requirement: "EDIT-07"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^(TestSettingsEditRegistry|TestSettingsEditCallbackBytes|TestSettingsEditPagesFitEveryLocale)$' ./fuku/modules"
        status: pass
    human_judgment: false

duration: 75min
completed: 2026-11-05
status: complete
---

# Phase 12 Plan 12: Rules media, buttons and Media under text in the editor Summary

**The Rules page now holds the rules' text, media and buttons, the owner removes any one of them with one tap, `/clearrules` clears all three, and a rules photo, video or GIF can be put under its text from the page.**

## What changed

- `rules.go`: `rulesWrites` has the four new seam fields (`SetRulesMediaOnly`, `SetRulesButtonsOnly`, `SetRulesMediaBelow`, `ClearRules`); `rulesApplyResult.Row` (whole row read back with `rules.LoadRulesSettingsContext`, `Rules` equals `Row.Rules`); `applyRulesMedia`, `applyRulesButtons`, `applyRulesMediaBelow`; `applyRulesClear` now writes `rules.ClearRules` (text, media and buttons together, never Media under text, private rules or the rules button).
- `settings_edit.go`: `settingsEditRulesRead = rules.LoadRulesSettingsContext`; `settingsEditRulesStateOf`; the Rules parts table `{text, media, buttons}`; the Rules `u` cell (`settingsEditRulesBelowSwitch`, `settingsRightAdminBotAdmin`, `Drawn: settingsEditMediaBelowEligible`, `settingsEditRulesApply(applyRulesMediaBelow)`); Main reads Set by `RulesSettings.HasContent`.
- `settings_edit_actions.go`: `settingsEditApplyRemoveRules`, one part switch over `applyRulesText("")`, `applyRulesMedia("", 0)` and `applyRulesButtons(nil)`.
- Tests: new `settings_edit_rules_test.go` (`TestSettingsEditRulesParts`, `TestSettingsEditRulesRemoveParity`, `TestSettingsEditRulesMainState`, `TestSettingsRulesMediaBelow`); rewritten expectations in `settings_edit_test.go` (the Rules text-only subtest is now "Rules has three parts", rows, mock states incl. a photo with the switch, registry 4/3/1, callback-byte limits), `settings_edit_actions_test.go` (callback fields, group-menu refusal and bot-administrator refusal for m and b, the new `TestSettingsEditRulesCallbackParts`, Remove parity with `/clearrules`), `rules_apply_test.go` ("clear" compares file_id, media_type, btns and the kept columns).
- `AGENTS.md`: the message editor bullet (Rules parts `x m b`, its `u` switch and right, Remove per part, `/clearrules` over `rules.ClearRules`, the whole-row read, Main by `HasContent`) and the Testing sentence naming `rules.LoadRulesSettingsContext`.

## TDD record

- **Task 1 (tracer)** RED `db5bfbe`: `TestSettingsEditRulesParts` failed on assertions (no media line, no buttons line, Remove buttons `x` not `x,m,b`). GREEN `6749e8c`.
- **Task 2** RED `e70284d`: `TestSettingsEditRulesMainState/media_only` and `/buttons_only` failed on assertions (status line Not set); `gsd_run check tdd-red-evidence` answered `RED_EVIDENCE_OK`, semantic assessment: the target executed and failed on the planned assertion. The other Task 2 expectations (three-part page, m and b callback parts, three Removes equal `/clearrules`) already held on the tracer code, so they are regression guards. GREEN `783c6fb` (Main by `HasContent`, plus the m and b group-menu and bot-administrator tests).
- **Task 3** RED `536a044`: `TestSettingsRulesMediaBelow` failed on assertions (no `settings_btn_edit_switch_below` cell, no line), registry and mock-row expectations failed; `RED_EVIDENCE_OK`, semantic assessment as above. GREEN `2439b33`.
- Extra `1a1c6b4`: see deviation 1.

## Commits

- db5bfbe test(12-12): the Rules page shows and removes media and buttons
- 6749e8c feat(12-12): the Rules page shows and removes media and buttons
- e70284d test(12-12): the three Removes store what /clearrules stores and Main reads Set by HasContent
- 783c6fb feat(12-12): /clearrules clears the text, media and buttons together
- 536a044 test(12-12): Media under text on the Rules page
- 2439b33 feat(12-12): Media under text on the Rules page
- 1a1c6b4 test(12-12): the wait marker of a Rules media or buttons part now parses

## Verification (real results)

- Task 1: `-run '^(TestSettingsEditRulesParts|TestSettingsEditReadFailure|TestRulesApplyParity|TestRulesApplyWriteFailure)$' ./fuku/modules` ok. Task 2 and Task 3 verify lists: ok. Acceptance greps: the three apply functions exist, `settingsEditRulesRead = rules.LoadRulesSettingsContext` once, `rulesWrites.ClearRules` present, `Rules has the text line only` gone, `HasContent()` and `applyRulesMediaBelow(` in `settings_edit.go`, `LoadRulesSettingsContext` in AGENTS.md, `Rules has \`x\` only` gone from AGENTS.md.
- Plan end: `-run '^(TestSettings|TestLoadSettings)' ./fuku/modules ./fuku/i18n` ran once (modules 208.7 s, i18n 5.3 s) and had one failure, `TestSettingsInputAlbumAndMarker/parse_marker` (deviation 1); that test passes after the fix and the whole family was not re-run afterwards, because it is the only test the run reported as failing. `-run '^(TestRules|TestSetRules|TestClearRules|TestPrivateRules|TestSendRules|TestBuildRules)' ./fuku/modules ./fuku/db/rules` ok.
- `CGO_ENABLED=0 go build ./...` ok; `go vet -tags testtools ./fuku/modules` clean; `gofmt -l` on the touched files empty (other `fuku/modules` files listed by `gofmt -l` are not mine and were already unformatted); `make generate-docs` then `make check-docs` no drift; `make check-translations` all present; `git diff --exit-code 72a692b -- go.mod go.sum` clean.
- `python3 .../lint-local.py 72a692b`: exit 0 before the last code commit and again before this SUMMARY.
- Not run: `make lint` (the installed golangci-lint is built with Go 1.25), `make test`, PostgreSQL-only tests (this plan adds none).

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] A test outside the plan's file list encoded the pre-12-12 Rules parts**
- **Found during:** plan-end settings family run
- **Issue:** `TestSettingsInputAlbumAndMarker/parse_marker` listed `-100|r|m` as an invalid marker, which was true while Rules had the text part alone.
- **Fix:** the refused case is now `-100|r|z`, and `-100|r|m` and `-100|r|b` are asserted to parse.
- **Files modified:** `fuku/modules/settings_input_test.go`
- **Commit:** 1a1c6b4

**2. [Process slip, no effect] `git stash` / `git stash pop` run once by mistake** (a stray command meant to look at the scratchpad). The tree was unchanged afterwards (`git stash list` empty, the same four modified files, `git diff --stat` identical) and nothing was lost; it is named because the brief prohibits stash.

**3. [Documented] The Task 1 RED commit holds a test that fails.** The plan says "write the test first (red commit)" and the commit subject is `test(12-12)`; the tracer's GREEN commit follows at once.

## Known Stubs

None. Until 12-13 a typed photo or button list for a Rules wait is still refused by the check's old Rules arm, and until 12-14 the Preview of Rules media and buttons still sends the old text preview (the Preview buttons of those parts are drawn already, as the generic renderer shows a Preview for any set part); the plan names both and neither is tested here.

## Requirements

EDIT-07 and EDIT-08 stay Pending; the EDIT-09 tick is untouched. `requirements-completed: []`.

## Threat Flags

None. No new endpoint, auth path, file access or schema change.

## Self-Check: PASSED

All eight changed files exist, and the seven commits above are ancestors of HEAD.
