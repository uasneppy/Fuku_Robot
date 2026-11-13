---
phase: 12-rules-media-media-under-text-and-greet-once
plan: 11
subsystem: settings-editor
tags: [media-under-text, welcome, goodbye, settings-menu, switch-grid, stale-press]
requires:
  - phase: 12-rules-media-media-under-text-and-greet-once
    provides: 12-04 media.CaptionAboveEligible and the sender, 12-05 the locale keys, 12-06 control Validate, 12-08 applyWelcomeMediaBelow / applyGoodbyeMediaBelow, 12-10 the settingsEditSwitch table, settingsEditSwitchGrid and the Drawn field
provides:
  - control u (Media under text) on the Welcome page (cells e d u o) and the Goodbye page (cells e d u)
  - settingsEditMediaBelowEligible (one predicate with media.CaptionAboveEligible), settingsEditState.MediaBelow, settingsEditSwitch.UnavailableKey
  - settingsEditStaleSwitch (a press of a cell that is no longer drawn writes nothing and redraws with settings_edit_below_unavailable)
  - settings_edit_hint_below_file while the stored media is a document
affects: [12-12, 12-14, 12-16]

actuals:
  tokens: 11900
  tasks: 3
  commits: 7

commits: 7
plan_head_before: e26acca7e48786f7fdbe43ecf042882231e4eb3e
plan_head_after: 1a1c5afa6cccf94e08b3408a07e7db92bd998f5e

tech-stack:
  added: []
  patterns:
    - "A switch cell with a Drawn function is re-checked against a fresh load at press time (settingsEditStaleSwitch); the sender ignores a stored on for an ineligible kind anyway, so the check-then-write gap is harmless"

key-files:
  created: []
  modified:
    - fuku/modules/settings_edit.go
    - fuku/modules/settings_edit_switch_test.go
    - fuku/modules/settings_edit_test.go
    - fuku/modules/settings_edit_screens_test.go
    - AGENTS.md

key-decisions:
  - "The u cell's right is the literal settingsRightChangeInfo, equal to both pages' EditRight (the cells are package-level values that cannot read the page definition); TestSettingsEditRegistry asserts control u's Right equals def.EditRight on both pages, so a drift fails"
  - "settingsEditStaleSwitch returns (screen, stop) so a page that cannot be read before the press ends as the cannot-open edit with no second edit"
  - "The AGENTS.md control and rule text went into the Task 1 feat commit (the sentence 'Media under text u arrives with plan 12-11' was invalidated there), the Testing family sentence into Task 3"

patterns-established:
  - "settingsEditPageFor: open an editor page of the env's chat in the group or the private menu and return a press function (settingsOncePage now delegates to it)"

requirements-completed: []

coverage:
  - id: D1
    description: "The owner turns Media under text on and off for a Welcome photo and a Goodbye video from the group menu and the private menu; the line and the cell redraw from the read-back with no notice and one answer, a double tap writes the same value, and the next join or leave sends the media with show_caption_above_media true"
    requirement: "EDIT-07"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^(TestSettingsEditMediaBelowSwitch|TestSettingsDriftMediaBelow)$' ./fuku/modules"
        status: pass
    human_judgment: false
  - id: D2
    description: "The cell and its line are drawn exactly for a photo, video and animation with a file, not for document, audio, voice, sticker, video note, no media or a type with no file; the grid closes up; a document (a GIF saved earlier) reads File or GIF and shows the file hint after the other hints, an animation reads GIF"
    requirement: "EDIT-07"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^TestSettingsEditMediaBelowEligibility$' ./fuku/modules"
        status: pass
    human_judgment: false
  - id: D3
    description: "A press of the switch after the media became ineligible writes nothing and redraws the page with the warning settings_edit_below_unavailable, in both menus, answered once; a stored on is kept and ignored for a sticker and comes back for a video; Remove media, Remove text and /resetwelcome leave it"
    requirement: "EDIT-07"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^(TestSettingsEditMediaBelowStalePress|TestSettingsEditMediaBelowStoredOnIgnored)$' ./fuku/modules"
        status: pass
    human_judgment: false
  - id: D4
    description: "The press stores what applyWelcomeMediaBelow / applyGoodbyeMediaBelow store (equal rows, no other column, cache entry gone, a caption at the 1024 limit written with no length refusal); an administrator without can_change_info gets the alert settings_no_change_info and nothing is written"
    requirement: "EDIT-07"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^(TestSettingsParityMediaBelow|TestSettingsAuthorityMediaBelow)$' ./fuku/modules"
        status: pass
    human_judgment: false
  - id: D5
    description: "Registry (Welcome e d u o, Goodbye e d u, Rules none, u's right is the page's EditRight), callback bytes (45 private, 41 group, Omitted 0, u drawn only for eligible media on the greeting pages), grid (Welcome 8 rows private / 9 group with [below | once], Goodbye 8 / 9 with [below]), 3800 UTF-16 units in 8 languages with a 64-rune title, 20-code-point cells in 8 languages"
    requirement: "EDIT-07"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^(TestSettingsEditRegistry|TestSettingsEditCallbackBytes|TestSettingsEditPagesFitEveryLocale|TestSettingsEditSwitchGrid|TestSettingsEditScreensFitEveryLocale)$' ./fuku/modules"
        status: pass
    human_judgment: false

duration: 70min
completed: 2026-11-05
status: complete
---

# Phase 12 Plan 11: Media under text on Welcome and Goodbye Summary

**The owner turns Media under text on from the Welcome or Goodbye page, and the next join or leave sends the photo, video or GIF with its caption above it; the switch is shown only for media that can take it and a stale press writes nothing.**

## What changed

- `settings_edit.go`: `settingsEditControlBelow = "u"`; `settingsEditState.MediaBelow` (copied from `WelcomeSettings.MediaBelow` / `GoodbyeSettings.MediaBelow` in `settingsEditStateOf`, never from anything but the stored column); `settingsEditMediaBelowEligible` (`st.FileID != "" && media.CaptionAboveEligible(st.MediaType)`); `settingsEditSwitch.UnavailableKey`; the cells `settingsEditWelcomeBelowSwitch` and `settingsEditGoodbyeBelowSwitch` (label `settings_btn_edit_switch_below`, line `settings_edit_line_below`, right `settingsRightChangeInfo`, `Drawn: settingsEditMediaBelowEligible`, apply `applyWelcomeMediaBelow` / `applyGoodbyeMediaBelow` through `settingsEditGreetingApply`), placed so the tables read Welcome `e d u o` and Goodbye `e d u`; `settingsEditStaleSwitch` called by `settingsEditSetSwitch` for any cell with a `Drawn` function; the `settings_edit_hint_below_file` paragraph in `settingsEditBody` after the other hints while the stored media is a document. No new locale key (the 13 keys exist from 12-05). The control registers through the existing `init` loop with `settingsEditSwitchValue`.
- Tests: `TestSettingsEditMediaBelowSwitch`, `...Eligibility`, `...StalePress`, `...StoredOnIgnored`, `TestSettingsParityMediaBelow`, `TestSettingsDriftMediaBelow`, `TestSettingsAuthorityMediaBelow` (new, `settings_edit_switch_test.go`, plus the helper `settingsEditPageFor`); `TestSettingsEditPages` (mock rows now derived from the cells drawn for each state), `TestSettingsEditRegistry` (4 / 3 / 0 controls, u's right), `TestSettingsEditCallbackBytes` and the callback states (photo with the switch on and off, animation, document), the grid test (Welcome and Goodbye worst cases, the 20-code-point loop with a photo stored) and the screens list (the page under `settings_edit_below_unavailable`).
- `AGENTS.md`: the message editor bullet (control u, right, eligibility, stale press, kept-but-ignored stored on, file hint, no length check) and the Testing family sentence.

## TDD record

- **Task 1 (tracer) RED** `4c3eb73`: `TestSettingsEditMediaBelowSwitch` failed on assertions in all four subtests (`lacks the off line`, `no button labelled settings_btn_edit_switch_below`); the only stub was the constant `settingsEditControlBelow`. **GREEN** `81df1d1`.
- **Task 2 RED** `97ff469`: `TestSettingsEditMediaBelowEligibility/the_file_hint_shows...` failed on assertions (hint not drawn for a document, on both pages). The stale-press and stored-on tests already passed because the tracer built that behaviour (as in 12-10); they are regression guards. **GREEN** `d58c774`: the file hint.
- **Task 3** `4597ea2`: written after the code, so the tests passed at once (the plan's "fix the code if a check fails" case). To show they are not vacuous I mutated the u cell's right to `settingsRightAdmin` (restored from a backup, tree diff clean afterwards): `TestSettingsAuthorityMediaBelow/w_can_change_info=false` and `TestSettingsEditRegistry` failed.

## Commits

- 4c3eb73 test(12-11): add failing test for Media under text on the Welcome and Goodbye pages
- 81df1d1 feat(12-11): Media under text on the Welcome and Goodbye pages
- 97ff469 test(12-11): add failing tests for Media under text eligibility, stale press and stored on
- d58c774 feat(12-11): Media under text appears only for media that can take it
- 4597ea2 test(12-11): Media under text matches its apply function and the join
- 33390b7 docs(12-11): name the Media under text apply calls in the switch cell comment
- 1a1c5af docs(12-11): one apply call per line in the switch cell comment

## Verification (real results)

- Task 1: `-run '^(TestSettingsEditMediaBelowSwitch|TestSettingsEditOnceSwitch|TestGreetingsMediaBelowEverywhere)$' ./fuku/modules` ok; acceptance greps (at least two lines with `apply...MediaBelow(` after the comment commits, exactly one `media.CaptionAboveEligible(`) hold.
- Task 2: the three tests print `--- PASS`; the `settings_edit_hint_below_file` count in `settings_edit.go` is 1 and `settings_edit_below_unavailable` is present.
- Task 3: `-run '^(TestSettingsParityMediaBelow|TestSettingsDriftMediaBelow|TestSettingsAuthorityMediaBelow|TestSettingsEditRegistry|TestSettingsEditCallbackBytes|TestSettingsEditPagesFitEveryLocale|TestSettingsEditSwitchGrid|TestSettingsEditScreensFitEveryLocale)$' ./fuku/modules` ok; `grep -c settingsEditMediaBelowEligible AGENTS.md` is non-zero.
- Plan end: `-run '^(TestSettings|TestLoadSettings)' ./fuku/modules ./fuku/i18n` ok (modules 207.1 s, i18n 5.2 s); `-run '^(TestGreetingsMediaBelow|TestLegacyGreetings|TestGreetOnce)' ./fuku/modules` ok; `CGO_ENABLED=0 go build ./...` ok; `make generate-docs` then `make check-docs` no drift; `make check-translations` all present; `go vet -tags testtools ./fuku/modules ./fuku/i18n` clean; `gofmt -l` on the touched files empty; `git diff --exit-code 72a692b -- go.mod go.sum` clean.
- `python3 .../lint-local.py 72a692b`: exit 0 (no dupl, gocyclo, godox, staticcheck or errcheck findings on added lines) before the Task 1 feat commit, before the last code commit and again before this SUMMARY.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Phase 12-10 editor tests broke when the u cell was added**
- **Found during:** Task 1 (the editor family run after the GREEN code)
- **Issue:** `TestSettingsEditPages` (mock rows were built from the first three `def.Switches`) and `TestSettingsEditRegistry` (4 / 3 controls) encoded the pre-12-11 tables.
- **Fix:** the mock rows now come from `settingsEditSwitchCells(def, state)` paired two to a row; the registry expects 4 / 3 / 0 and asserts u's right. Both were the plan's listed Task 3 edits, done in the Task 1 commit so that commit stays green.
- **Files modified:** `fuku/modules/settings_edit_test.go`

**2. [Documented] Two comment-only commits.** The Task 1 acceptance grep `applyWelcomeMediaBelow(\|applyGoodbyeMediaBelow(` counts call-form text on at least two lines, but the cells pass the functions as values (`settingsEditGreetingApply(applyWelcomeMediaBelow)`), so the switch-cell comment now names the calls `applyWelcomeMediaBelow(ctx, chatID, on)` and `applyGoodbyeMediaBelow(ctx, chatID, on)` on separate lines (33390b7, 1a1c5af; no code change).

**3. [Documented] The plan's acceptance command names `.planning/phases/11-message-editor/handoff/lint-local.py bd2f794`; the executor brief's stand-in with base `72a692b` was run instead.**

## Known Stubs

None.

## Threat Flags

None. T-12-34 (right: `TestSettingsAuthorityMediaBelow`, registry), T-12-35 (stale press reloads and refuses: `TestSettingsEditMediaBelowStalePress`, sender ignores an ineligible stored on: `TestSettingsEditMediaBelowStoredOnIgnored`), T-12-36 (failed write shown as the save-failed notice through the shared `settingsEditSetSwitch`, unchanged) and T-12-SC (go.mod and go.sum unchanged) are mitigated as planned.

## Requirements

`requirements-completed: []`. EDIT-07 stays Pending: Rules gets its switch in 12-12 and Full preview shows the placement in 12-14. The EDIT-09 tick from 12-10 is untouched.

## Not run

- `make test` (phase gate plan 12-16 only) and `make lint` (cannot run in this sandbox; the lint stand-in ran).
- PostgreSQL-only tests (`TestGreetedMemberClaimConcurrent`, `TestRulesMediaMigrationDefaults`): not part of this plan.
- Owner-confirmation assumptions A2 (right is the page's EditRight), A3 (Remove and resets leave the switch) and A5 (stale press warns) are implemented as recommended defaults and remain open for the owner.

## Self-Check: PASSED

- `fuku/modules/settings_edit.go`, `fuku/modules/settings_edit_switch_test.go` and this file exist.
- Commits 4c3eb73, 81df1d1, 97ff469, d58c774, 4597ea2, 33390b7 and 1a1c5af are on `claude/ecstatic-goldberg-qsss05` (`git merge-base --is-ancestor` for each).
