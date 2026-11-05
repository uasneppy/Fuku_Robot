---
phase: 13-member-requirements
plan: 04
subsystem: i18n
tags: [locales, i18n, member-requirements, settings-menu, mreq, help-text]
requires:
  - phase: 13-member-requirements
    provides: 13-UI-SPEC copywriting contract, limits tables and per-locale length notes (wave 1, depends_on is empty)
provides:
  - the 25 mreq_ keys (five rule phrases, the unknown-name fallback, the six group notices with the three warn-limit variants, the I fixed it button, the two edited notices, the toast and six alerts, the stored warn reason, the pause notice) in en, es, fr, hi, id, pt, ru and uk
  - the 36 settings_ keys of the Member requirements page (page name and Main line, intro, rule names and lines, five hints, seven half-width cells, the rule screen, the confirm card) in the same 8 locales
  - settings_help_msg changed in all 8 locales (covers member requirements, they have no command) and the regenerated settings help docs page
  - TestMemberReqLocaleKeys with memberReqLocaleLimits (25 rows), the rendered notice and alert checks, the button-name quote consistency and the literal Russian letters check
  - 36 new rows in settingsLocaleLimits (199 to 235 exact), the rendered requirements-cell checks, the two new choice cells and the 🛂 grid label in TestSettingsLabelLengths, the help check in TestSettingsHelpFitsOneMessage
affects: [13-05, 13-06, 13-07, 13-08, 13-09, 13-10, 13-11, 13-12, 13-13, member-requirements]

actuals:
  tokens: 25100
  tasks: 3
  commits: 6

commits: 6
plan_head_before: ab87e03e8a057328145d000d3ffee8576dbd7972
plan_head_after: b526dc8c710b720ef6a8e4ef92882fec7aba9802

tech-stack:
  added: []
  patterns:
    - "A locale family with no Go reader yet is held by its own exact-key test (memberReqLocaleLimits), because make check-translations only sees literal GetString keys"
    - "The rendered worst case is the binding length check: a 66 code point name, the five rule phrases joined by a comma and space, and two-digit numbers"
    - "A button name that hints quote is checked as a verbatim substring across the two key families, so a text never names a button the member cannot find"

key-files:
  created:
    - fuku/i18n/memberreq_locale_test.go
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
  - "All 25 mreq_ keys are written, mreq_notice_paused included: the plan and research keep the per-chat punishment cap"
  - "The new keys sit immediately before settings_help_msg in every file, mreq_ first and settings_ second, so no block scalar is split"
  - "The mreq_ test forbids < > and & in every template (not only < and >): a notice is sent in HTML parse mode, where a bare ampersand is invalid"
  - "ru and uk name the button Ispravleno (Исправлено / Виправлено) and es, fr, pt, id, hi use first-person past (Ya lo arreglé, J'ai corrigé, Já corrigi, Sudah saya perbaiki, मैंने ठीक कर लिया): the Slavic form is gender-neutral, so no notice has to guess the member's gender"
  - "Notices in es, fr, pt, ru and uk are phrased impersonally or with the grammatically neutral noun where the English they/them would force a gender (se le expulsó, a pessoa foi expulsa, участника удалили)"
  - "fr mreq_fixed_still_broken is 'Corrigez votre profil, puis retouchez.' because the natural 'touchez de nouveau' was 74 code points against a 70 ceiling; no ceiling was widened"
  - "uk delete cell is 'Видаляти повідом.' (17 code points): the full 'Видалення повідомлень' is 21 and breaks the 18-code-point name rule; the body line settings_mreq_line_delete carries the full words"
  - "No AGENTS.md edit: no rule is invalidated, and the page, namespace and tables sections arrive with the plans that build them"

patterns-established:
  - "Cross-family locale check inside one test: memberReqCrossFamilyFailures reads mreq_ and settings_mreq_ keys of one language together"

requirements-completed: []

coverage:
  - id: D1
    description: "The 25 mreq_ keys exist non-empty in all 8 locale files, within their ceilings, with no markup or ampersand, the placeholders of English, the toast and alerts at most 160 code points"
    requirement: "MREQ-05"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^TestMemberReqLocaleKeys$' -v ./fuku/i18n (RED at b21a74a on missing keys, GREEN at d322182)"
        status: pass
    human_judgment: false
  - id: D2
    description: "Rendered with the worst case (66 code point name, all five phrases, two-digit numbers) the still-broken alert is within 160 in English and 200 elsewhere and every notice within 400 code points in every language"
    requirement: "MREQ-06"
    verification:
      - kind: unit
        ref: "TestMemberReqLocaleKeys (memberReqRenderedFailures), all 8 subtests pass"
        status: pass
    human_judgment: false
  - id: D3
    description: "The 36 settings_ keys exist in all 8 locales and the limits table covers exactly the 235 settings_ keys; the Main grid label, the five rule cells with each punishment icon, the two switch cells with check and cross are at most 20 code points, the choice cells Off and Warn within 13 English and 15 elsewhere"
    requirement: "MREQ-01"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^(TestSettingsLocaleKeys|TestSettingsLabelLengths|TestMemberReqLocaleKeys|TestSettingsReusedKeyPlaceholders)$' ./fuku/i18n (RED at 3db1c87, GREEN at 2e2230b); grep -cE '^settings_' locales/en.yml prints 235"
        status: pass
    human_judgment: false
  - id: D4
    description: "In each language the words of mreq_btn_fixed without its check mark appear verbatim in mreq_notice_mute, settings_mreq_actions_hint, settings_mreq_hint_mute and settings_mreq_confirm_mute, and the letters ы ъ э ё are literal in settings_mreq_hint_letters and settings_mreq_desc_russian in all 8 locales"
    requirement: "MREQ-13"
    verification:
      - kind: unit
        ref: "TestMemberReqLocaleKeys (memberReqCrossFamilyFailures). Negative proof: with English settings_mreq_hint_mute temporarily saying 'I fixed this' the test failed with 'en: settings_mreq_hint_mute does not contain the button words \"I fixed it\" verbatim'; restored before the commit"
        status: pass
      - kind: other
        ref: "for l in en es fr hi id pt ru uk: grep -q '^settings_mreq_hint_letters:.*ы.*ъ.*э.*ё' locales/$l.yml (all 8 match)"
        status: pass
    human_judgment: false
  - id: D5
    description: "settings_help_msg says in all 8 locales that the menu covers member requirements and that they have no command, stays inside the help ceilings and one Telegram message, and the generated settings help page carries the new English text"
    requirement: "MREQ-01"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^(TestSettingsHelpFitsOneMessage|TestSettingsLocaleKeys)$' -v ./fuku/i18n (RED at bb489a7, GREEN at b526dc8); help is 1094 UTF-16 units with header in English, at most 1236 elsewhere, of 4096"
        status: pass
      - kind: other
        ref: "make check-translations (all present), make generate-docs then make check-docs ('No drift detected'); grep -c 'member requirements' on locales/en.yml and the docs page prints 1 each"
        status: pass
    human_judgment: false
  - id: D6
    description: "A native speaker reads the 61 new strings and the changed help in es, fr, hi, id, pt, ru and uk as natural"
    requirement: "MREQ-05"
    verification:
      - kind: backstop
        ref: "owner or native-speaker read of the seven translations, and a phone check of the hi cells with combining marks (planned live check, not run here)"
        status: not-run
    human_judgment: true

duration: 30min
completed: 2026-11-05
status: complete
---

# Phase 13 Plan 04: The member requirement strings in 8 languages Summary

**Sixty-one new strings (25 `mreq_`, 36 `settings_`) and the changed settings help in en, es, fr, hi, id, pt, ru and uk, held by a new exact-key locale test, rendered worst-case length checks, a button-name quote-consistency check and the extended settings limits and cell checks.**

## Accomplishments

- Task 1 (tracer): `TestMemberReqLocaleKeys` first (RED), then the 25 `mreq_` keys in all 8 files. The test covers exactly the keys of every locale (a missing or extra key fails), non-empty, the UI ceilings (the row 10 other-language ceiling is 170), no `<`, `>` or `&`, no printf verb, the placeholders of English, the toast and six alerts at most 160 code points, `mreq_fixed_still_broken` rendered with all five phrases at most 160 (English) or 200, and every notice (warn, the three warn-limit variants, kick, ban, mute, the two fixed edits, the pause) rendered with a 66 code point name, the five phrases and two-digit numbers at most 400.
- Task 2: the 36 limits rows (199 to 235 exact), the 36 `settings_` keys in all 8 files, the rendered requirements-cell checks (five rule cells with each of the five punishment icons, two switch cells with the check mark and the cross, at most 20 code points), `settings_state_off_plain` and `settings_opt_warn` added to the choice-row list, the 🛂 grid label, and in `TestMemberReqLocaleKeys` the quote-consistency and literal-letters checks.
- Task 3: the help check first (RED), then `settings_help_msg` changed in all 8 files: the covers paragraph adds member requirements, and one sentence at the end of that paragraph (after the command list) says they have no command. Every command name of `settingsHelpCommands` and only the `<b>` tag are kept. `make generate-docs` changed one line of `docs/src/content/docs/commands/settings/index.md`, and `make check-docs` is clean.
- Translations follow the UI contract's per-locale notes (uk "Перевіряти вхід", fr "Suppr. messages", es "Revisar al entrar", pt "Checar na entrada", grid names Requisitos, Exigences, Persyaratan, Требования, Вимоги, आवश्यकताएँ). uk keeps "Попередження" untouched (the choice cell is 14 of 15). ru and uk name Belarusian and Bulgarian, es/fr/pt/id/hi too, and none names Ukrainian. The four letters ы ъ э ё are literal in every language.
- The four Phase 12 backed-up Ukrainian golden keys (`rules_for_chat`, `rules_click_for_rules`, `rules_not_set`, `rules_no_rules_set`) were not touched.

## Task Commits

1. **Task 1 RED:** `b21a74a` test(13-04): add failing locale test for the member requirement notices
2. **Task 1 GREEN (tracer):** `d322182` feat(13-04): the group notices and "I fixed it" answers in 8 languages
3. **Task 2 RED:** `3db1c87` test(13-04): add failing locale checks for the Member requirements page
4. **Task 2 GREEN:** `2e2230b` feat(13-04): the Member requirements page in 8 languages
5. **Task 3 RED:** `bb489a7` test(13-04): add a failing check that the settings help names member requirements
6. **Task 3 GREEN:** `b526dc8` feat(13-04): settings help names member requirements

The tracer gate: the tracer task's verify (`TestMemberReqLocaleKeys`) was re-run end to end after the expansion tasks and passes, so nothing was built on a broken slice.

## Verification (real results)

- `go test -tags testtools -race -count=1 -run '^TestMemberReqLocaleKeys$' -v ./fuku/i18n`: RED at `b21a74a` (per-language missing-key assertions), PASS at `d322182` and again at HEAD.
- `go test -tags testtools -race -count=1 -run '^(TestSettingsLocaleKeys|TestSettingsLabelLengths|TestMemberReqLocaleKeys|TestSettingsReusedKeyPlaceholders)$' ./fuku/i18n`: RED at `3db1c87`, `ok` at `2e2230b`.
- `go test -tags testtools -race -count=1 -run '^(TestSettingsHelpFitsOneMessage|TestSettingsLocaleKeys)$' -v ./fuku/i18n`: RED at `bb489a7`, PASS at `b526dc8`.
- `go test -tags testtools -race -count=1 ./fuku/i18n` (whole package): `ok` (15.7 s) at HEAD.
- `make check-translations`: "All translations are present!" (after each task).
- `make generate-docs` then `make check-docs`: "No drift detected".
- `gofmt -l fuku/i18n`: empty. `go vet -tags testtools ./fuku/i18n`: clean. `CGO_ENABLED=0 go build ./...`: ok. `git diff --exit-code 8c0c827 -- go.mod go.sum`: exit 0.
- Lint stand-in `python3 .../lint-local.py 8c0c827`: exit 0 ("no dupl, gocyclo or godox findings on lines added since 8c0c827"), run before the last code commit and again before this SUMMARY.
- Acceptance greps: `grep -cE '^mreq_'` is 25 in every file; `grep -cE '^settings_' locales/en.yml` is 235; the `ы.*ъ.*э.*ё` grep matches in all 8 files; `member requirements` is present in `locales/en.yml` and the docs page.
- Not run: `make lint` (cannot run in this sandbox, golangci-lint built with Go 1.25), `make test` (not this plan's task; the plan touches no Go in `fuku/modules`, so the settings family was not run), a phone or native-speaker read.

## Deviations from Plan

None. The plan executed as written. Two small choices worth naming (neither changes a contract): the `mreq_` test also forbids `&` (HTML parse mode), and `mreq_fixed_still_broken` in French was shortened to fit its 70 ceiling.

## Known Stubs

None. The 61 keys have no Go reader until plans 13-05 to 13-13 by design (this is why `TestMemberReqLocaleKeys` exists); that is not a UI stub. No broken-windows ledger entry was needed.

## Threat Flags

None. No endpoint, auth path, file access or schema was added. T-13-10 and T-13-11 are mitigated by the new tests (markup and placeholder checks, ceilings, rendered worst cases, quote consistency); T-13-SC by the unchanged `go.mod` and `go.sum`.

## Requirements

`requirements-completed: []`. The plan contributes the words of MREQ-01, MREQ-05, MREQ-06 and MREQ-13 and completes none; all four stay Pending in REQUIREMENTS.md.

## Deferred live checks

- A native-speaker read of the 61 new strings and the changed help in es, fr, hi, id, pt, ru and uk (UI Consideration "language"; the owner serves all eight).
- A phone check of the hi cells and choice row (combining marks look wider than the code-point count).

## Self-Check: PASSED

- FOUND: fuku/i18n/memberreq_locale_test.go; FOUND: the 25 `mreq_` keys and 36 `settings_mreq_`/page keys in all 8 locale files (counts above).
- FOUND commits: b21a74a, d322182, 3db1c87, 2e2230b, bb489a7, b526dc8 (all ancestors of HEAD).
