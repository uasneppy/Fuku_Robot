---
phase: 09-settings-menu
plan: 05
subsystem: i18n
tags: [locales, settings-menu, i18n, tests]
requires:
  - phase: 09-01
    provides: settings_locale_test.go limits table, the tracer's 23 settings_ keys
provides:
  - 56 page and card settings_ keys in all 7 locale files (79 settings_ keys per file)
  - TestSettingsLabelLengths and TestSettingsReusedKeyPlaceholders
affects: [09-08, 09-09, 09-10, 09-11, 09-12]
tech-stack:
  added: []
  patterns: [limits-table row per settings_ key, composite-label sizing test, reused-key privacy placeholder test]
key-files:
  created: []
  modified:
    - alita/i18n/settings_locale_test.go
    - locales/en.yml
    - locales/es.yml
    - locales/fr.yml
    - locales/hi.yml
    - locales/id.yml
    - locales/pt.yml
    - locales/ru.yml
key-decisions:
  - "settings_lockdown_reason is not added; the reason line reuses lockdown_reason_line (D-29)"
  - "Main-page label check adds the status icon to common_status_on/off, because the page renders icon plus word"
requirements-completed: [MENU-04, MENU-05, MENU-06, MENU-07]
status: complete
duration: 25min
completed: 2026-10-30
commits: 2
plan_head_before: e595cb83fa4a76a0e7ca611cac47c16bb11933c3
plan_head_after: a61add37c93551e381e0c4a09a034e593e31f265
actuals:
  tokens: 30000
  tasks: 2
  commits: 2
---

# Phase 9 Plan 05: Settings menu page strings Summary

**All 56 remaining `settings_` keys of the Staff Group, Lockdown, Captcha and Antiflood pages and the two-tap cards exist in en, es, fr, hi, id, pt and ru, held by the limits table, a composite-label sizing test and a reused-Staff-key privacy test.**

## Accomplishments

- Extended `settingsLocaleLimits` with the 56 UI-SPEC rows (Max EN and Max other). The parity test now requires exactly 79 `settings_` keys in every locale.
- Added `TestSettingsLabelLengths`: each main-page button (icon, page name, ` · `, longest state, with count and limit set to 100 and the status icon on On/Off) stays within 34 characters in English and 44 elsewhere, and each `✅ ` choice button within 13 and 15.
- Added `TestSettingsReusedKeyPlaceholders` (D-21 privacy): the reused Staff and lift texts carry no placeholder or only `{group}` in all 7 languages, and `settings_staff_linked_to` is the only `settings_` key with `{staff}` or `{id}`.
- Added the English source exactly as the UI-SPEC table gives it, then the six translations using each file's established terms (Grupo de Staff / groupe Staff / स्टाफ ग्रुप / Grup Staf / штаб-группа; bloqueo / verrouillage / लॉकडाउन / lockdown / bloqueio / блокировка).

## Task Commits

1. Task 1: locale test and English source: `7fe437f` (`test(09-05)`)
2. Task 2: six translations: `a61add3` (`feat(09-05)`)

## Verification

- `go test -tags testtools -race -count=1 -v -run '^(TestSettingsLocaleKeys|TestSettingsLabelLengths|TestSettingsReusedKeyPlaceholders|TestLockdownLocaleKeys)$' ./alita/i18n`: all 7 language subtests of each pass.
- Whole `./alita/i18n` package passes; `make check-translations` and `make check-docs` pass; `go vet -tags testtools ./alita/i18n/` and `gofmt -l alita/i18n` are clean.
- `grep -c "^settings_"` prints 79 for every locale file; `settings_lockdown_reason` count is 0.
- `make lint` was not run (golangci-lint in this sandbox is built with go1.25 against go1.26.0); vet and gofmt stood in for it, as the project notes direct.

## Deviations from Plan

None to the plan's scope. Two small notes:

- The tracer's existing 23 keys in each locale file are not in alphabetical order. The 56 new keys are appended after them as one contiguous block, alphabetical among themselves, rather than re-sorting the tracer's lines.
- Commit trailers use the attribution lines from the harness (Claude Sonnet 5.5 and the session link), not the Opus line printed in the plan text, as the task notes direct.

## Known Stubs

None.

## Pending human verification

None. Translations are my own and have not been reviewed by a native speaker; the owner may want to skim the hi and ru wording.

## Self-Check: PASSED

- Commits `7fe437f` and `a61add3` exist on the branch; the files named above exist; all focused tests pass.
