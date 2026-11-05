---
phase: 11-message-editor
plan: 04
subsystem: i18n
tags: [locales, message-editor, i18n, refusals, previews]
requires:
  - phase: 11-message-editor
    provides: plan 11-01 (the first 57 editor locale keys, the limits table and TestSettingsEditLocaleTokens)
provides:
  - 26 more settings_edit_ strings (saved, album, caption, still waiting, preview, refusals, preview headers, wait ended, part words) in all 8 locale files
  - limits rows for those 26 keys, four new alert keys and token checks for the syntax refusal, {n}, {max} and {part}
  - UI-SPEC copy table rows 80-83 (the four D-12 keys), so the contract and the locale files agree on 83 keys
affects: [11-10, 11-11, 11-12, 11-13, settings-message-editor]

actuals:
  tokens: 14000
  tasks: 2
  commits: 2

commits: 2
plan_head_before: 0a244f0e21fa9ff0934aecaa89af20408995522d
plan_head_after: 5eecd15ebe258ac5529b7fefcfbd4561f2da8300

tech-stack:
  added: []
  patterns:
    - "Part-specific sentences take a lower-case part word through {part} (settings_edit_part_text|media|buttons), never a key name built in code"

key-files:
  created: []
  modified:
    - locales/en.yml
    - locales/uk.yml
    - locales/es.yml
    - locales/fr.yml
    - locales/hi.yml
    - locales/id.yml
    - locales/pt.yml
    - locales/ru.yml
    - fuku/i18n/settings_locale_test.go
    - .planning/phases/11-message-editor/11-UI-SPEC.md

key-decisions:
  - "The wait-ended note puts {part} in parentheses in es, fr, pt, uk and ru ('La espera ({part}) terminó ...') so one lower-case noun fits every grammar without case endings."
  - "uk and hi say 'the start button' instead of quoting Telegram's localized Start label, because the label differs by client language and cannot be verified here."
  - "settings_edit_wait_ended_note is a plain reply, so it is not an alert key; the four preview alerts (rows 63-66) were added to settingsAlertKeys."

requirements-completed: []

duration: 15min
completed: 2026-11-04
status: complete

coverage:
  - id: D1
    description: "Every saved, album, caption, still-waiting, preview, refusal, preview-header and wait-ended string exists in all 8 locales, non-empty and within its ceiling, with the English placeholder list in the same order"
    requirement: EDIT-03
    verification:
      - kind: unit
        ref: "fuku/i18n/settings_locale_test.go#TestSettingsLocaleKeys"
        status: pass
    human_judgment: false
  - id: D2
    description: "The syntax refusal keeps its buttonurl example byte-identical, and {n}, {max} and {part} appear exactly where the English has them"
    requirement: EDIT-03
    verification:
      - kind: unit
        ref: "fuku/i18n/settings_locale_test.go#TestSettingsEditLocaleTokens"
        status: pass
    human_judgment: false
  - id: D3
    description: "The four preview alerts are tag-free and at most 200 characters in every language; no new key carries a placeholder that could name the Staff Group"
    requirement: EDIT-06
    verification:
      - kind: unit
        ref: "fuku/i18n/settings_locale_test.go#TestSettingsReusedKeyPlaceholders"
        status: pass
    human_judgment: false
---

# Phase 11 Plan 04: Editor refusal, preview and wait-ended strings Summary

**26 more editor strings (saved, album, caption, still waiting, previews, every refusal reason, the wait-ended reply and its three part words) in all 8 languages, held by the locale guards, and the UI contract extended to 83 keys.**

## Performance

- **Duration:** about 15 min
- **Completed:** 2026-11-04
- **Tasks:** 2 (a tracer task and an auto task)
- **Files modified:** 10

## Accomplishments

- Task 1 (tracer) proved the path first: `settings_edit_saved_text` and `settings_edit_still_waiting` in all 8 files with their limits rows passed `TestSettingsLocaleKeys` and `TestSettingsEditLocaleTokens` before the rest was added.
- Task 2 added the other 24 keys in en, uk, es, fr, hi, id, pt and ru. Each locale now holds 83 editor keys (the grep from the plan prints 83 for every file).
- `settings_locale_test.go` has one `settingsLocaleLimits` row per new key (UI-SPEC ceilings, research Q9 for the four D-12 keys), the four preview alerts in `settingsAlertKeys`, and `TestSettingsEditLocaleTokens` now requires `<code>[Name](buttonurl://https://example.com)</code>` byte-identical in every language plus `{n}`, `{max}` and `{part}` where the English has them.
- `11-UI-SPEC.md` lists the four D-12 keys as rows 80-83 under "Wait ended (D-12, research Q9)", says the table ends at 83 keys, and the two checklist lines that still said 79 now say 83.
- No new key carries `{id}`, `{staff}` or `{group}`; `settings_edit_refuse_access_lost` names no group and is one text for every refusal of that kind (T-11-11).

## Task Commits

1. **Task 1 (tracer): saved-text and still-waiting replies** - `e1fb943` (feat)
2. **Task 2: refusal, preview and wait-ended strings, limits, tokens, UI-SPEC rows** - `5eecd15` (feat)

## Verification (real results)

- `go test -tags testtools -race -count=1 -run '^(TestSettingsLocaleKeys|TestSettingsEditLocaleTokens)$' -v ./fuku/i18n` after Task 1: both PASS.
- `go test -tags testtools -race -count=1 ./fuku/i18n` after Task 2: `ok` (the whole package, 17 s).
- `grep -cE '^settings_(edit_|btn_edit_|state_set|state_not_set|state_default|page_welcome|page_goodbye|page_rules|no_change_info)' locales/<lang>.yml`: 83 for en, uk, es, fr, hi, id, pt and ru.
- `make check-translations`: "All translations are present" for every locale.
- `git diff --exit-code 72a692b -- go.mod go.sum`: exit 0.
- `gofmt -l fuku/i18n`: nothing. `go vet -tags testtools ./fuku/i18n`: exit 0.
- `grep -c settings_edit_part_buttons 11-UI-SPEC.md`: 1; `grep -c settings_edit_preview_need_start settings_locale_test.go`: 2.
- Not run: `make lint` (the installed golangci-lint is built with Go 1.25, as the brief says), `make test` (not asked by this plan), `make check-docs` (no English-doc input changed beyond locale keys the generator does not read).

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] gofmt re-aligned the whole `settingsLocaleLimits` table**
- **Found during:** Task 2
- **Issue:** The new key `settings_edit_refuse_media_text_long` is longer than the old alignment column, so `gofmt` re-aligned every row of the table (a large whitespace-only diff in the test file).
- **Fix:** Accepted the gofmt output; no value changed.
- **Files modified:** fuku/i18n/settings_locale_test.go
- **Commit:** 5eecd15

**2. [Rule 2 - Consistency] Two UI-SPEC checklist lines said 79 keys**
- **Issue:** Lines 905 and 1038 of `11-UI-SPEC.md` still said 79 new keys after rows 80-83 were added.
- **Fix:** Changed both to 83. The plan only named the "table ends at" sentence.
- **Commit:** 5eecd15

Otherwise the plan was executed as written.

## Requirements

`requirements-completed: []`. This plan contributes to EDIT-03 (the refusal reasons) and EDIT-06 (the preview words) but completes neither: EDIT-03 is completed by plan 11-11 and EDIT-06 by plan 11-12. Both stay Pending in REQUIREMENTS.md.

## Known Stubs

None. Every added string is real copy; no placeholder text.

## Threat Flags

None. No endpoint, auth path or schema was added; only locale text and tests.

## Self-Check: PASSED

- Commits `e1fb943` and `5eecd15` are on the branch (`git merge-base --is-ancestor`).
- All 10 modified files exist and the working tree was clean after the Task 2 commit apart from this summary.
