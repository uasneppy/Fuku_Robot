---
phase: 10-pm-settings-hub-and-owner-gate
plan: 03
subsystem: i18n
tags: [locales, i18n, docs, tests, plat-05]
requires:
  - phase: 10-pm-settings-hub-and-owner-gate
    provides: staffLocales already lists the 8 codes (uk added earlier in the phase)
provides:
  - TestLocalesAreTheEight, a set-comparison guard that locales/*.yml (minus config.yml) equals staffLocales and has exactly 8 members
  - locale-count comments of the staff, captcha web and lockdown guard tests and captcha_mode_turnstile_test.go say 8
  - README, PROJECT, CLAUDE.md and the two codebase-map files state 8 locales and list uk
affects: [10-04, 10-10]

status: complete
actuals:
  tokens: 4500
  tasks: 2
  commits: 2

commits: 2
plan_head_before: d0988d52d6c1732a6e760524edc1c9a836a1bf6d
plan_head_after: 23e763ab6706c528f87b2f7d7b8fc6856feaf384

tech-stack:
  added: []
  patterns:
    - "Locale-set drift is guarded by comparing the directory with the list the guard tests walk, as sets, through a small helper (localeSetDiff) that a subtest can drive with hand-built sets"

key-files:
  created:
    - fuku/i18n/locale_set_test.go
  modified:
    - fuku/i18n/staff_locale_test.go
    - fuku/i18n/captcha_web_locale_test.go
    - fuku/i18n/lockdown_locale_test.go
    - fuku/modules/captcha_mode_turnstile_test.go
    - README.md
    - .planning/PROJECT.md
    - .claude/CLAUDE.md
    - .planning/codebase/ARCHITECTURE.md
    - .planning/codebase/STRUCTURE.md

key-decisions:
  - "The CLAUDE.md i18n purpose line was edited by hand to equal the ARCHITECTURE.md line character for character, so a regeneration gives the same text"
  - "AGENTS.md, the TestSettingsLocaleKeys comment (plan 10-02) and the settings privacy subtest name (plan 10-10) were left alone; the two 'seven send permissions' comments are untouched"

requirements-completed: [PLAT-05]

coverage:
  - id: D1
    description: "The locale set is guarded by a test: the 8 locale files equal staffLocales as sets, and a missing or extra code is named in the failure"
    requirement: PLAT-05
    verification:
      - kind: unit
        ref: "fuku/i18n/locale_set_test.go#TestLocalesAreTheEight"
        status: pass
    human_judgment: false
  - id: D2
    description: "Project documents outside AGENTS.md state 8 locales and list uk"
    requirement: PLAT-05
    verification:
      - kind: other
        ref: "! grep -nE '\\b7 (locale|language|interface language)|all 7 (files|locales|languages)|seven locale|7 languages' README.md .planning/PROJECT.md .claude/CLAUDE.md .planning/codebase/ARCHITECTURE.md .planning/codebase/STRUCTURE.md"
        status: pass
    human_judgment: false
---

# Phase 10 Plan 03: Locale count guard and 8-locale documents Summary

A set-comparison test now proves the locale set is exactly the 8 language files, and README, PROJECT, CLAUDE.md and the codebase map say 8 locales with uk listed.

## Performance

- **Tasks:** 2 (1 tracer, 1 auto), **Files:** 10 (1 created, 9 modified), **Commits:** 2

## Accomplishments

- `TestLocalesAreTheEight` (`fuku/i18n/locale_set_test.go`) reads `../../locales`, drops `config.yml`, and compares the remaining codes with `staffLocales` through `localeSetDiff` as sets; a failure names each missing and each extra code and the count. Three subtests drive the helper with hand-built sets: a missing plus an extra code are both named, order never matters, a ninth code fails the count.
- The doc comments of `TestStaffLocaleKeys`, `TestCaptchaWebLocaleKeys`, `TestLockdownLocaleKeys`, `TestLockdownWarningLines`, `TestLockdownStringsHaveBalancedTags` and the comment near line 230 of `captcha_mode_turnstile_test.go` say 8.
- README (feature line, tree line, contributing rule), PROJECT (Phase 8 item, Constraints bullet), CLAUDE.md (Constraints, Translator row, purpose line), ARCHITECTURE.md (4 places) and STRUCTURE.md (tree gains `uk.yml`, 5 count places) are at 8 locales.

## Verification results

- `go test -tags testtools -race -count=1 -run '^(TestLocalesAreTheEight|TestStaffLocaleKeys|TestCaptchaWebLocaleKeys|TestLockdownLocaleKeys)$' -v ./fuku/i18n`: all four PASS.
- `go test -tags testtools -race -count=1 ./fuku/i18n`: ok (15.7s).
- `go vet -tags testtools ./fuku/modules`: exit 0.
- Old-count grep over the four Go files: no output. Old-count grep over the five documents: no output.
- Purpose line count: 1 in `.claude/CLAUDE.md`, 1 in `.planning/codebase/ARCHITECTURE.md`. `en, es, fr, hi, id, pt, ru, uk` count: 1 and 1. README `uk, en, ... + config.yml`: 1. STRUCTURE.md `uk.yml`: 2, `en,es,fr,hi,id,pt,ru,uk`: 1. `head -n1 .claude/CLAUDE.md` is still `@../AGENTS.md`.
- `grep -c 'seven send'` still 1 in both `staff_action_fake_test.go` and `staff_undo_restore_test.go`.
- `git diff --exit-code 6ef6ad9 -- go.mod go.sum`: clean.

## Task Commits

1. Task 1 (tracer): `9950a71` test(10-03): guard that the locale set is exactly the 8 files
2. Task 2: `23e763a` docs(10-03): state the locale count as 8 and list uk in project documents

The tracer gate: its `<verify>` carries only automated commands, so it was re-run end to end and passed before Task 2.

## Deviations from Plan

None - plan executed exactly as written. (Ukrainian is described as the default language in README and STRUCTURE.md, as the plan directs.)

## Issues Encountered

None.

## Known Stubs

None.

## Threat Flags

None. T-10-35 (locale set drift) is mitigated by `TestLocalesAreTheEight`; T-10-SC: no dependency changed.

## Next Phase Readiness

Plan 10-04 owns the AGENTS.md locale count; plan 10-10 owns the settings privacy subtest name.

## Self-Check: PASSED

- `fuku/i18n/locale_set_test.go` exists; commits `9950a71` and `23e763a` are ancestors of HEAD.
