---
phase: 19-blacklist-filter-and-note-pages
plan: 06
subsystem: settings-menu-two-step-add-engine
tags: [telegram, settings-menu, typed-reply-door, list-engine, two-step-add, wait-store, test-only-page]
status: complete

requires:
  - phase: 19-blacklist-filter-and-note-pages
    provides: 19-04's door list branch, part table, settingsInputAdvance / settingsInputSetPrompt and the w / q wait fields; 19-05's content.ExtractTypedNoteOrFilter
provides:
  - the generic two-step add of the door's list branch (listStepOne, listStepTwo), driven by settingsListDef.TwoStep hooks and consulted by AddGate
  - the step lines and the step-2 prompt of renderSettingsListPrompt
  - four settings_ strings in 8 locales (424 settings_ keys)
  - withTestSettingsList, the swap-and-restore installer of a test-only list page, and the engine's proof on it
affects: [19-07, 19-08, 19-09, 19-10, 19-11, 19-12]

plan_head_before: 5dda0ba39feb46f845b33120f8e176955c160fd1
plan_head_after: 2f80eb68a105a1efb65934e1533e68408b4b17b7

actuals:
  tokens: 11500   # chars/4 over the added lines of 5dda0ba..2f80eb6 outside .planning (46137 chars, locales, tests and AGENTS.md included)
  tasks: 3
  commits: 3      # MEASURED: git rev-list --count 5dda0ba..2f80eb6 (before this SUMMARY commit)

tech-stack:
  added: []
  patterns:
    - "a page of a two-step add gives hooks only (Normalise, Check, IsFilter, Apply, KeyLineKey, SavedKey) and writes no flow code"
    - "a test-only page is installed by saving and replacing the registry entries of its key, list table and action codes, and restored in t.Cleanup; registerSettingsPage is never called"

key-files:
  created:
    - fuku/modules/settings_list_twostep_test.go
  modified:
    - fuku/modules/settings_list_input.go
    - fuku/modules/settings_list.go
    - fuku/modules/settings_input_door.go
    - fuku/modules/settings_list_env_test.go
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
  - "A refused extraction is DataType == -1, not ErrorMsg != \"\": ExtractTypedNoteOrFilter (like ExtractNoteAndFilter) starts from its needs-content refusal text and never clears it, so ErrorMsg is non-empty on success too. The commands judge it by DataType and so does the engine. The plan text said ErrorMsg; the first test run exposed it."
  - "The step-2 key line and the saved notice splice the escaped normalised entry under the table's EntryParam (so the page's KeyLineKey / SavedKey templates take {keyword} or {name} like the delete card does), through settingsListEntryText: cut at 100 code points, escaped, never through the translator."
  - "end(key) was split into cancelWait plus its old text, and endWith(text) / endText added beside it; end behaves byte-identically. A taken Apply result finishes the wait itself and answers endText, because the wait is already deleted."
  - "The blank-first-step branch and its key (settings_entries_refuse_blank) land in Task 2 with the tests that need them, so Task 1's tree never names a key that is not in the locale files (423 after Task 1, 424 after Task 2, as the plan's criteria say)."

patterns-established:
  - "settingsListTwoStepEnv (newSettingsListTwoStepEnv, begin, advance, say, sayAgain, reply, cancelFields, waitStays, reset): one environment per test, one test page, cases that reset the wait and the store"
  - "settingsListTestStore: the in-memory list behind the test page with knobs for Check's refusal or error, the Add gate's screen and Apply's outcome, and a record of every Apply call"

requirements-completed: []

coverage:
  - id: D1
    description: "a two-step list page takes the first typed message as its keyword or name, moves the wait atomically to step 2 (w, q, a new nonce, ten fresh minutes), sends the step-2 prompt as a reply recorded as m, closes the step-1 prompt, then extracts the second message as the command's reply form and hands it to the page's Apply once"
    requirement: "PAGE-08 / PAGE-09 (contribution only)"
    verification:
      - kind: unit
        ref: "TestSettingsListTwoStepFlow (bare and under cienv.sh)"
        status: pass
    human_judgment: false
  - id: D2
    description: "every refusal, race and edge keeps or ends the wait as the contract says: need-text, blank (U+3000), a Check refusal and error at step 1, the Add gate, the q guard, a poll, a Check that now refuses, a taken and a failed Apply, an album, Add at step 2, expiry, /cancel"
    requirement: "PAGE-08 / PAGE-09 (contribution only)"
    verification:
      - kind: unit
        ref: "TestSettingsListTwoStepBranches (12 subtests; mutation check: with the q guard disabled the redelivery subtest fails)"
        status: pass
    human_judgment: false
  - id: D3
    description: "the test-only page leaves the registries as it found them, so the tests still pass after 19-07 registers the real Filters page under key i"
    verification:
      - kind: unit
        ref: "TestSettingsListTwoStepFlow compares the key, list and action-code entries before and after the installer's cleanup"
        status: pass
    human_judgment: false
  - id: D4
    description: "four strings in 8 languages under their ceilings (424 settings_ keys); the one-step door and the editor's door are unchanged"
    requirement: "PLAT-05 (contribution only)"
    verification:
      - kind: unit
        ref: "TestSettingsLocaleKeys, TestSettingsLabelLengths, TestSettingsReusedKeyPlaceholders, make check-translations, TestSettingsListDoor* , git diff --exit-code d93ec1d on the three Phase 11 and 12 door test files"
        status: pass
    human_judgment: false
---

# Phase 19 Plan 06: the generic two-step add engine Summary

**The settings menu's door can now take a keyword or name and then the content in two typed messages, written and tested once in the list engine on a test-only page, so the Filters and Notes pages need to give only their hooks.**

## What was built

- **Task 1, the tracer (`ae26585`).** `settings_list_input.go`: `settingsListTwoStep` (`Normalise`, `Check`, `IsFilter`, `Apply`, `KeyLineKey`, `SavedKey`), `settingsListApplyResult` (outcome saved, taken or failed; `Text`; `Notice`), `listStepOne` (need text, normalise, the page's Check, a fresh nonce, `settingsInputAdvance`, the step-2 prompt as a reply, `settingsInputSetPrompt`, the step-1 prompt edited to `settings_entries_step_done`), `listStepTwo` (the `q` guard, `content.ExtractTypedNoteOrFilter`, Check again, claim, Apply, finish, the saved page, the prompt closed; taken ends the wait; failed releases the claim) and the step lines of `renderSettingsListPrompt` (the step-2 prompt starts with the escaped key line and names the 4096 and 1024 limits). `runList` sends a step-1 part with a `Next` and a step-2 part of a `TwoStep` page to the engine and keeps `Typed` for one-step pages. `settings_list.go`: `TwoStep` and `AddGate` on the table; the Add press consults the gate first and draws its screen without creating a wait. `settings_input_door.go`: `endWith`, `endText`, `cancelWait` and `editPrompt` (the old `closePrompt` is now `editPrompt` of the closed line). Locale rows 11, 12 and 14 in 8 files. Test toolkit `withTestSettingsList` and the test page (`settingsListTestTwoStepDef`, key `i`, codes `zg zx zy za zc`) and `TestSettingsListTwoStepFlow`. AGENTS.md: the door's Handlers bullet, the wait Data bullet, the list engine bullet and the Testing bullet.
- **Task 2 (`32b52a3`).** The blank first step (the engine's own check: `strings.Fields` of U+3000 is empty) is refused with `settings_entries_refuse_blank`, row 13, in 8 files, and `TestSettingsListTwoStepBranches` proves the rest on the test page: photo, U+3000 and a Check refusal each keep the wait at step 1 without moving or extending it (Cancel `{a: zc, t: k}`, noted refusal, the next text read as a keyword), a Check error keeps the wait, the Add gate creates no wait, a redelivered keyword message gets no reply and is not stored, a poll is refused with `content_need_filter_content` keeping `y` and `w`, a Check that now refuses and a taken Apply end the wait with their text and `settings_edit_wait_stopped`, a failed Apply releases the claim and the next reply is applied, an album is applied once from its first item with `settings_edit_album_cut` and the page's notice line, Add at step 2 restarts at step 1 with the replaced note and no `q`, a reply after ten minutes names `settings_entries_part_reply` once, `/cancel` at step 2 deletes the wait with its `w`.
- **Task 3 (`2f80eb6`).** AGENTS.md read against the code once more: the key count (424), the four new keys, the new test names and the engine's blank-entry check.

`settingsInputAdvance` and `settingsInputSetPrompt` now have their production caller (`listStepOne`); `ExtractTypedNoteOrFilter` has its first production caller (`listStepTwo`). No real page is registered here: Filters is 19-07 and Notes 19-09, and the engine names no page.

## Verification (real results)

- Task gates, bare and under `cienv.sh`: `TestSettingsListTwoStepFlow`, `TestSettingsListTwoStepBranches`, `TestSettingsListDoorOneStep`, `TestSettingsInputAdvance`, `TestSettingsListKeysExist`: ok. `./fuku/i18n` `TestSettingsLocaleKeys`, `TestSettingsLabelLengths`, `TestSettingsReusedKeyPlaceholders`: ok. `make check-translations`: "All translations are present!". Each locale file has 423 `settings_` lines after Task 1 and 424 after Task 2 (checked by `grep -cE '^settings_'` over the eight files).
- Acceptance greps: `grep -c 'settingsInputAdvance(' fuku/modules/settings_list_input.go` prints 1; `grep -c 'ExtractTypedNoteOrFilter(' ...` prints 1; `grep -c 'func withTestSettingsList(' fuku/modules/settings_list_env_test.go` prints 1; `grep -c 'func TestSettingsListTwoStepBranches(' fuku/modules/settings_list_twostep_test.go` prints 1.
- Invariants: `git diff --exit-code d93ec1d -- fuku/modules/settings_input_door_test.go fuku/modules/settings_input_media_test.go fuku/modules/settings_input_rules_test.go` exits 0; `git diff --exit-code 8c0c827 -- go.mod go.sum` exits 0; no `t.Skip`, `reflect.Ptr` or `ioutil` in the new code; no log line in `settings_list_input.go` names the entry or `Pending`.
- Mutation check: with the `q` guard disabled the redelivery subtest failed (the redelivery was answered and stored); the guard was restored before the commit.
- Gates: `go vet -tags testtools ./fuku/modules ./fuku/i18n` clean; `CGO_ENABLED=0 go build ./...` clean; `gofmt -l` over the files this plan created or edited prints nothing; `lint-local.py 8c0c827` exits 0 (read from the exit code of a run with output redirected: "no dupl, gocyclo or godox findings on lines added since 8c0c827"). `make lint`, gosec and `make test` were not run (the sandbox cannot run lint; `make test` is the closing plan's).
- Whole `fuku/modules` package under the CI environment (`go test -tags testtools -race -count=1 -timeout 40m -v ./fuku/modules`, background, log `/tmp/claude-0/-home-user-Fuku-Robot/4ce545b5-6832-57a3-a863-43c972932c7c/scratchpad/modules-19-06.log`): `ok  github.com/uasneppy/fuku_robot/fuku/modules  1813.758s`, last line `exit 0`, 1803 top-level PASS, 0 FAIL, 0 SKIP, 0 `^FAIL`; `--- PASS: TestSettingsListTwoStepFlow (0.28s)`, `--- PASS: TestSettingsListTwoStepBranches (0.45s)` and `--- PASS: TestSettingsListDoorOneStep (0.38s)` are in it.
- Settings family under the CI environment (`go test -tags testtools -race -count=1 -timeout 40m -run '^(TestSettings|TestLoadSettings)' ./fuku/modules ./fuku/i18n`, log `/tmp/claude-0/-home-user-Fuku-Robot/4ce545b5-6832-57a3-a863-43c972932c7c/scratchpad/family-19-06.log`): `ok fuku/modules 714.366s`, `ok fuku/i18n 7.529s`, `exit 0`.

## Time added to the whole-package run

About 1 s: `TestSettingsListTwoStepFlow` 0.28 s and `TestSettingsListTwoStepBranches` 0.45 s (twelve subtests, one shared environment, 0.00 to 0.04 s each); the plan estimated about 3 s. No new test is over 1 s.

## Deviations from Plan

**1. [Rule 1 - Bug in the plan text] A refused extraction is `DataType == -1`, not `ErrorMsg != ""`.** The plan says an `ErrorMsg` refusal keeps the wait. `ExtractTypedNoteOrFilter` returns a result that starts from the needs-content text and `preFixes` never clears it, so `ErrorMsg` is non-empty on a successful extraction too; the first flow test run showed a valid photo refused. The engine reads `DataType == -1`, exactly as `/filter` (`filters.go`) does, and says so in a comment and in AGENTS.md. No production code of an earlier plan changed.
**2. [Structure] The blank-first-step branch and key 13 were added in Task 2, not Task 1,** so the Task 1 commit does not name a key missing from the locale files; the 423 and 424 counts of the plan's criteria hold at each commit.
**3. [Structure] `end` was split** into `cancelWait` plus the old text building so `endWith` shares it (the plan said to leave `end` unchanged; its behaviour and answers are byte-identical, and every Phase 11, 12 and 19 door test passes).
**4. [Detail the plan left open]** The key line and the saved notice take the entry under `def.EntryParam`, so the Filters page's `settings_flt_keyword_line` and `settings_flt_saved` must use `{keyword}` (rows 47 and 53 do) and Notes' `{name}`. `renderSettingsListPrompt` gained a `keyword` argument (empty for step 1). A taken `Apply` finishes the wait and answers `endText` instead of `endWith`, because the claim already deleted it. After a lost-race `Check` at step 2 the wait ends with `endWith`.

No auth gates.

## Known Stubs

None. `settingsListTwoStep.Apply`, `Check` and the Add gate are exercised by the test page only until 19-07 and 19-09 register Filters and Notes, which is the plan's design; the production engine code has no function that only the test page uses.

## Threat Flags

None new. T-19-28 (replay): the `q` guard and the nonce-checked advance and claim, proved by the redelivery subtest and the mutation check. T-19-29 (taken between the steps): `Check` re-run at step 2 and a taken `Apply` both end the wait. T-19-30 (the keyword leaking): `w` only in the wait hash, shown escaped and cut at 100 code points in the prompt, never in a log line, label or callback data (grep clean). T-19-31 (test isolation): `withTestSettingsList` restores every entry it touched and the flow test compares them before and after.

## Requirements

None completed: `requirements-completed: []`. PAGE-08, PAGE-09 and PLAT-05 stay Pending (19-12 completes PAGE-08 and PAGE-09).

## Self-Check: PASSED

- Files exist: `fuku/modules/settings_list_twostep_test.go`, `fuku/modules/settings_list_input.go`, `fuku/modules/settings_list_env_test.go`.
- Commits reachable from HEAD: `ae26585`, `32b52a3`, `2f80eb6`.
