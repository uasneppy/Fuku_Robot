---
phase: 11-message-editor
plan: 01
subsystem: i18n
tags: [locales, i18n, settings-menu, message-editor, ui-spec, docs]
requires:
  - phase: 10-pm-settings-hub-and-owner-gate
    provides: the settings_ locale block, settingsLocaleLimits, the label-length and help tests
provides:
  - the 57 part-A editor strings (UI-SPEC rows 1-44, 50-58, 60-62, 67) in all 8 locales
  - the changed settings_help_msg naming the welcome, goodbye and rules messages and /setwelcome, /setgoodbye, /setrules
  - TestSettingsEditLocaleTokens and the editor label-length checks
affects: [11-02, 11-04, 11-08, 11-09, 11-11, settings-menu, message-editor]

actuals:
  tokens: 36000
  tasks: 3
  commits: 3

commits: 3
plan_head_before: f66298437ddcaeebbf12229906a7d7fb8e51260d
plan_head_after: 289d58996d81e67a553e49c79f0752728466a643

tech-stack:
  added: []
  patterns:
    - "Literal editor tokens ({first} ... {rules}, :same, buttonurl:// and the two example lines) are guarded byte-identical per language by TestSettingsEditLocaleTokens"
    - "Two-column editor buttons are asserted at 20 code points as written; switches at 40 rendered with the longer state"

key-files:
  created: []
  modified:
    - locales/en.yml
    - locales/es.yml
    - locales/fr.yml
    - locales/hi.yml
    - locales/id.yml
    - locales/pt.yml
    - locales/ru.yml
    - locales/uk.yml
    - fuku/i18n/settings_locale_test.go
    - docs/src/content/docs/commands/settings/index.md

key-decisions:
  - "TestSettingsReusedKeyPlaceholders now lets the literal {id} of settings_edit_wait_text through (and only that key and only {id}): the text prompt must show the welcome placeholder {id} as copyable code, and the token test holds it byte-identical. {staff} stays forbidden everywhere."
  - "No new string carries a placeholder beyond {state}, {n}, {max}, {time} and the literal editor tokens, so none can name the Staff Group, a group title, a user or a stored message."

requirements-completed: []

duration: 14min
completed: 2026-11-04
status: complete

coverage:
  - deliverable: "57 editor strings present, non-empty, within their ceilings, alert keys tag-free at most 160, in all 8 locales"
    verification:
      - kind: test
        ref: "fuku/i18n/settings_locale_test.go#TestSettingsLocaleKeys"
        status: pass
      - kind: command
        ref: "make check-translations"
        status: pass
    human_judgment: false
  - deliverable: "Grid labels at most 20 code points, editor buttons at most 20, switches at most 40 rendered, in all 8 languages"
    verification:
      - kind: test
        ref: "fuku/i18n/settings_locale_test.go#TestSettingsLabelLengths"
        status: pass
    human_judgment: false
  - deliverable: "Literal tokens and real parameters byte-identical / present where English has them in all 8 languages"
    verification:
      - kind: test
        ref: "fuku/i18n/settings_locale_test.go#TestSettingsEditLocaleTokens"
        status: pass
    human_judgment: false
  - deliverable: "settings_help_msg names the three messages and commands, fits one Telegram message; generated docs match"
    verification:
      - kind: test
        ref: "fuku/i18n/settings_locale_test.go#TestSettingsHelpFitsOneMessage"
        status: pass
      - kind: command
        ref: "make check-docs"
        status: pass
    human_judgment: false
  - deliverable: "Translation quality and natural reading of the new strings in es, fr, hi, id, pt, ru, uk"
    human_judgment: true
    rationale: "No test judges whether a translation reads naturally; the owner (uk is the default language) should read the Welcome, Goodbye and Rules pages and prompts once."
---

# Phase 11 Plan 01: Editor strings Summary

**57 message-editor strings (page names, state words, hints, switches, part buttons, wait prompts, notices, alerts) and the changed `/settings` help in all 8 locales, held to the UI-SPEC ceilings by new limits rows, a label-length check and a literal-token test.**

## Accomplishments

- Task 1 (tracer): `settings_page_welcome`, `settings_page_goodbye`, `settings_page_rules` and `settings_btn_edit_set_text` in all 8 locales, with their limits rows and the three Main grid label rows (🎉 👋 📜, at most 20 code points).
- Task 2: the other 53 keys in all 8 locales with one limits row each; `settings_edit_unavailable`, `settings_edit_prompt_stale`, `settings_edit_wait_expired` and `settings_no_change_info` added to `settingsAlertKeys`; editor buttons checked at 20 and switches at 40 rendered; new `TestSettingsEditLocaleTokens`.
- Task 3: `settings_help_msg` changed in all 8 locales (menu coverage sentence and command list), `/setwelcome`, `/setgoodbye`, `/setrules` added to `settingsHelpCommands`, settings command page regenerated. Help is 968 to 1117 UTF-16 units with its header, well inside 4096.

## Task Commits

1. Task 1 (tracer): `f3afaa7` feat(11-01): add the editor page names and Set text label in all 8 locales
2. Task 2: `f6ab71c` feat(11-01): add the editor prompt, state, button and notice strings in all 8 locales
3. Task 3: `289d589` feat(11-01): settings help names the welcome, goodbye and rules messages

## Verification (real results)

- `go test -tags testtools -race -count=1 -run '^(TestSettingsLocaleKeys|TestSettingsLabelLengths)$' ./fuku/i18n` after Task 1: ok.
- `go test -tags testtools -race -count=1 -run '^(TestSettingsLocaleKeys|TestSettingsLabelLengths|TestSettingsEditLocaleTokens|TestSettingsReusedKeyPlaceholders)$' -v ./fuku/i18n` after Task 2: all four PASS.
- `go test -tags testtools -race -count=1 -run '^(TestSettingsHelpFitsOneMessage|TestSettingsLocaleKeys)$' ./fuku/i18n` after Task 3: both PASS.
- `go test -tags testtools -race -count=1 ./fuku/i18n` (whole package): ok.
- `go vet -tags testtools ./fuku/i18n` and `gofmt -l fuku/i18n`: clean.
- `make check-translations`: "All translations are present". `make generate-docs` then `make check-docs`: "No drift detected".
- `git diff --exit-code 72a692b -- go.mod go.sum`: exit 0.
- Acceptance counts: the Task 2 grep prints 57 in each of the 8 locale files; `grep -c '/setrules' locales/en.yml` 4; each locale names the three commands inside `settings_help_msg`; the docs page contains `/setwelcome`; `"settings_page_welcome"` appears twice in the test (limits row and label row), `"/setwelcome"` once, `func TestSettingsEditLocaleTokens` once.
- Not run: `make lint` (installed golangci-lint is built with Go 1.25 and refuses Go 1.26.0, a sandbox fact); `make test` (not asked by this plan, package `fuku/modules` untouched).

## Requirements

Contributes to EDIT-01 (page, state and button words) and EDIT-02 (prompt, Cancel, `/cancel` words). Neither is completed here (EDIT-01 by plan 11-09, EDIT-02 by plan 11-11), so `requirements-completed` is empty and both stay Pending in REQUIREMENTS.md. `requirements.mark-complete` was not run.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocker] TestSettingsReusedKeyPlaceholders forbade the literal {id} of the text prompt**
- **Found during:** Task 2 (planning the verify run; the plan lists this test in the Task 2 verify command)
- **Issue:** The test rejects `{id}` in every `settings_` key except `settings_staff_linked_to`, but the UI-SPEC (rows 39, 42 paragraph) requires `<code>{id}</code>` as one of the literal welcome placeholders in `settings_edit_wait_text`. The plan's threat T-11-01 quotes that rule as a mitigation.
- **Fix:** an exemption for exactly `{id}` in exactly `settings_edit_wait_text`; `{staff}` stays forbidden everywhere, and `TestSettingsEditLocaleTokens` holds the token byte-identical and present once in all 8 languages, so it can never become a filled placeholder.
- **Files modified:** fuku/i18n/settings_locale_test.go
- **Commit:** f6ab71c

**Total deviations:** 1 auto-fixed (1 blocking). **Impact:** none on scope; the privacy intent of T-11-01 is kept.

## Known Stubs

None. (Locale values only; no data flows to a UI yet.)

## Threat Flags

None. No new endpoint, auth path or schema; strings only.

## Notes for later plans

- Prompt strings carry the literal tokens inside `<code>`; the translator leaves unknown `{name}` tokens alone, so only `{max}`, `{time}`, `{n}` and `{state}` are interpolated.
- The Hindi, Indonesian and Portuguese button labels were checked only by code-point count; the UI-SPEC "check on a phone" note for hi is an owner-device step.
- The uk group note names the button "Відкрити в приватному чаті", matching the existing `settings_btn_open_private`.

## Self-Check: PASSED

- Commits f3afaa7, f6ab71c and 289d589 are ancestors of HEAD.
- All 8 locale files, `fuku/i18n/settings_locale_test.go` and `docs/src/content/docs/commands/settings/index.md` exist and are committed.
