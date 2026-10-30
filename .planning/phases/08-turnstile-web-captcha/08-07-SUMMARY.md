---
phase: 08-turnstile-web-captcha
plan: 07
subsystem: i18n
tags: [i18n, locales, captcha, turnstile, yaml]

requires:
  - phase: 08-turnstile-web-captcha
    provides: "captchaweb.PageKeys and captchaweb.PageLanguages (plan 08-05), captcha_web_welcome and captcha_web_button keys (plan 08-01)"
provides:
  - "26 captcha_web_ page keys in all 7 locale files (en es fr hi id pt ru)"
  - "LocaleManager.StringTable(langs, keys): one string table per language with English fallback, for the page (plan 08-10 calls it once at startup)"
  - "TestCaptchaWebLocaleKeys and TestStringTable"
affects: [08-10 wiring, 08-09 refusal keys (picked up by the prefix scan)]

plan_head_before: 20d409392ff3c7f1e5c2e0b79ed5ea237c0aed21
plan_head_after: c157db9a8e8a587a1cd1a0698d3c32373fb2972d

actuals:
  tokens: 11000
  tasks: 2
  commits: 4

tech-stack:
  added: []
  patterns:
    - "Prefix-scan locale parity test for keys that are read from a list, which make check-translations cannot see"

key-files:
  created:
    - alita/i18n/table.go
    - alita/i18n/table_test.go
    - alita/i18n/captcha_web_locale_test.go
  modified:
    - locales/en.yml
    - locales/es.yml
    - locales/fr.yml
    - locales/hi.yml
    - locales/id.yml
    - locales/pt.yml
    - locales/ru.yml

key-decisions:
  - "English text is exactly the UI-SPEC page strings table; the six translations stay within the UI-SPEC Max other lengths (counted in runes), shortened rather than truncated"
  - "StringTable rejects a value that is blank after trimming, not only an empty one, so a whitespace-only English value cannot ship a blank page state"
  - "StringTable errors name the language and key and never a value"

requirements-completed: [CAPT-08]

coverage:
  - id: D1
    description: "The 26 page strings exist in all seven languages as plain text within their UI-SPEC lengths; only the English kick bodies promise a rejoin"
    requirement: "CAPT-08"
    verification:
      - kind: unit
        ref: "alita/i18n/captcha_web_locale_test.go#TestCaptchaWebLocaleKeys"
        status: pass
    human_judgment: true
    rationale: "The test checks structure, length, plainness and the English rejoin rule only. Whether each translation reads naturally and keeps the meaning and tone is a native-speaker review item"
  - id: D2
    description: "LocaleManager.StringTable builds one table per language, falls back to English per key, and fails when English lacks a key or holds an empty one"
    requirement: "CAPT-08"
    verification:
      - kind: unit
        ref: "alita/i18n/table_test.go#TestStringTable"
        status: pass
    human_judgment: false

duration: 20min
completed: 2026-10-30
status: complete
---

# Phase 8 Plan 07: Captcha page strings in seven languages Summary

**26 `captcha_web_` page strings in en, es, fr, hi, id, pt and ru, a `LocaleManager.StringTable` builder that gives the page one non-empty table per language with English fallback, and a parity test that guards every `captcha_web_` key where `make check-translations` cannot see.**

## Performance

- **Duration:** about 20 min
- **Tasks:** 2 (both TDD: a failing test commit, then the implementation commit)
- **Files:** 3 created, 7 modified

## Accomplishments

- Added the 26 page keys of `captchaweb.PageKeys` to all 7 locale files in one block after `captcha_web_button`. English is exactly the UI-SPEC table. The other six are natural translations kept within "Max other" (titles 32/34, bodies 140, buttons 22 runes). Only the English `captcha_web_failed_body_kick` and `captcha_web_expired_body_kick` promise a rejoin.
- `TestCaptchaWebLocaleKeys`: the `captcha_web_` key set of every locale equals English's; every value is a non-empty string; page keys hold no `<`, `&`, `%` or `{` and no emoji, and fit their length ceiling (English against Max EN, the rest against Max other); the other `captcha_web_` keys (the welcome message and the button) keep English's `%s`/`%d` sequence; the length table covers exactly `captchaweb.PageKeys`; in English only the kick bodies say "rejoin".
- `LocaleManager.StringTable(langs, keys)` in `alita/i18n/table.go`: one `GetTranslator` per language, one `GetString` per key. A key a language lacks falls back to English, an unknown language gets the English values, and a key missing, empty or blank in English returns an error naming the language and key and no table.
- `TestStringTable` covers the real 7 locale files (26 non-empty strings in each of 7 maps, es differs from en, ru close button is the ru file's value), the English fallback, a key missing in English, a blank English value and an unknown language.

## Task Commits

1. Task 1 RED: `930c0a4` `test(08-07): show the captcha page strings are missing`
2. Task 1 GREEN: `f8a5508` `feat(08-07): add the captcha page strings in all seven languages`
3. Task 2 RED: `d1612be` `test(08-07): show there is no per-language string table` (fails to compile: `StringTable` undefined)
4. Task 2 GREEN: `c157db9` `feat(08-07): build per-language string tables for the captcha page`

## Verification

- `CGO_ENABLED=1 go test -tags testtools -count=1 -run '^TestCaptchaWebLocaleKeys$' -v ./alita/i18n`: PASS (ran, not "no tests").
- `CGO_ENABLED=1 go test -tags testtools -race -count=1 -run '^TestStringTable$' -v ./alita/i18n`: PASS, all 5 subtests ran.
- `CGO_ENABLED=1 go test -tags testtools -count=1 ./alita/i18n`: PASS.
- `make check-translations`: all translations present.
- `CGO_ENABLED=0 go build ./...`, `go vet -tags testtools ./alita/i18n`, `gofmt -l alita/i18n`: clean.
- Acceptance greps: the five sample keys count 5 in each of the 7 files; `grep -c '^captcha_web_' locales/en.yml` is 28; `captchaweb.PageKeys` appears 5 times in the parity test; the `StringTable` signature matches exactly; `go.mod` and `go.sum` unchanged.
- `make lint` was not run: golangci-lint in this sandbox is built with go1.25 against the module's go1.26.0. `gofmt` and `go vet -tags testtools` stand in for it.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Missing critical] StringTable also rejects a whitespace-only value**
- **Found during:** Task 2
- **Issue:** The plan says StringTable fails on an empty English value. `GetString` returns a whitespace-only value as found, so a blank English string would have shipped a blank page state (T-08-30).
- **Fix:** `StringTable` trims before the empty check; `TestStringTable` has an "empty in English" subtest using a blank value.
- **Files modified:** `alita/i18n/table.go`, `alita/i18n/table_test.go`
- **Commit:** `c157db9`

**2. [Harness instruction] Commit trailer**
- The plan text names a different model in the `Co-Authored-By` line than the trailer the harness supplied. The harness trailer (`Claude Sonnet 5.5`) is used on all four commits, per the orchestrator's instruction to use the harness's attribution lines. The first commit was amended once, before anything else was built on it, to use it.

**Total deviations:** 1 auto-fixed (Rule 2), 1 attribution note. **Impact:** none on scope; no files outside the plan's `files_modified`.

## Known Stubs

None. Nothing reads these strings until plan 08-10 wires `StringTable` into `captchaweb.NewPage`; that is by plan, not a stub.

## Pending human verification

- Native-speaker review of the es, fr, hi, id, pt and ru page strings for natural wording, tone and meaning, in particular that only the two kick bodies promise a rejoin in every language (the test checks that in English only; listed in the validation strategy).

## Threat Flags

None. No new endpoint, auth path or trust boundary. T-08-29 (markup or placeholder in a page string) is mitigated by `TestCaptchaWebLocaleKeys`; T-08-30 (empty string on the page) by `StringTable` plus `TestStringTable`.

## Notes for plan 08-10

- AGENTS.md is not edited here, as planned. Plan 08-10 should record the parity-test rule next to the route that makes these strings visible, and call `GetManager().StringTable(captchaweb.PageLanguages, captchaweb.PageKeys)` once at startup, passing the result to `captchaweb.NewPage`.
- The parity test already covers any `captcha_web_` key plans 08-01 and 08-09 add: non-page keys are checked for key parity and for an unchanged `%s`/`%d` sequence.

## Self-Check: PASSED

- Created files exist: `alita/i18n/table.go`, `alita/i18n/table_test.go`, `alita/i18n/captcha_web_locale_test.go`.
- Commits `930c0a4`, `f8a5508`, `d1612be`, `c157db9` are in `git log` (4 commits from the base, matching `commits: 4`).
