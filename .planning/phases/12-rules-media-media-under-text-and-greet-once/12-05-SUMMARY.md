---
phase: 12-rules-media-media-under-text-and-greet-once
plan: 05
subsystem: i18n
tags: [locales, i18n, settings-editor, switch-labels, rules-media, greet-once]
requires:
  - phase: 12-rules-media-media-under-text-and-greet-once
    provides: 12-UI-SPEC locale key table and per-locale length note
provides:
  - the 13 new editor strings of the UI-SPEC in en, es, fr, hi, id, pt, ru and uk
  - settings_edit_hint_rules reworded for a three-part Rules page, in all 8 locales
  - 13 rows in settingsLocaleLimits (199 settings_ keys covered, 186 before)
  - editorCellMax = 20 and editorCells, the rendered 20-code-point check of the five switch cells, with the icon alone in place of {state}
  - {state}, {n} and {max} token rows for the new keys
affects: [12-04, 12-10, 12-11, 12-12, 12-13, 12-16, settings-editor]

actuals:
  tokens: 21000
  tasks: 2
  commits: 2

commits: 2
plan_head_before: 186fe12b891bae4086eae2e2680cc4d81ee98801
plan_head_after: c64dba2204f875437a002639c175ae04719036bc

tech-stack:
  added: []
  patterns:
    - "A switch label is a template with {state} as the icon alone: its binding ceiling is the rendered 20 code points, while its limits row counts the template as written ({26, 26})"

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

key-decisions:
  - "The five switch rows are {26, 26} and the new rendered check is the binding rule, as the plan's planner resolution says; the check replaces {state} by the single icon (checked with both the check mark and the cross) and fails above 20"
  - "The new limits rows sit in their own block after a blank line and a comment, because gofmt re-aligns the whole existing map when a shorter key is added inside it (that would have turned a 25-line change into a 100-line diff)"
  - "The existing four-full-width-switch check (editorSwitches, 40 code points) and the four *_toggle / clean_* keys are left alone; plan 12-10 renames them"
  - "hi Media under text is the short name 'मीडिया नीचे' (11 code points): the natural 'टेक्स्ट के नीचे मीडिया' is 22 and would break the 18-code-point name rule; the full phrase is used on the body line settings_edit_line_below and in the hints"
  - "ru and uk body line / hint use the full 'Приветствовать один раз' / 'Вітати один раз' while the button keeps the UI-SPEC's 'Один раз'"

patterns-established:
  - "Rendered-size locale check for a button that carries a state icon: ReplaceAll the placeholder with the icon, count code points, both icons"

requirements-completed: []

coverage:
  - id: D1
    description: "The 13 new keys exist with a non-empty value in all 8 locale files, within their limits, with their placeholders and tags identical to English"
    requirement: "EDIT-07"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^(TestSettingsLocaleKeys|TestSettingsLabelLengths|TestSettingsEditLocaleTokens|TestSettingsHelpFitsOneMessage|TestSettingsReusedKeyPlaceholders)$' ./fuku/i18n"
        status: pass
    human_judgment: false
  - id: D2
    description: "The five switch labels rendered with the check mark and with the cross are at most 20 code points in every language"
    requirement: "EDIT-09"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^TestSettingsLabelLengths$' ./fuku/i18n (negative proof: a deliberately long English label failed with 'has 27 characters, ceiling 20' before it was restored)"
        status: pass
    human_judgment: false
  - id: D3
    description: "settings_edit_hint_rules says Set takes the next private message as that part, Preview shows one part, Full preview shows what /rules sends, with <code>/rules</code> kept in all 8 locales"
    requirement: "EDIT-08"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^TestSettingsEditLocaleTokens$' ./fuku/i18n"
        status: pass
    human_judgment: false
  - id: D4
    description: "hi labels with combining marks look no wider than their cell on a phone"
    requirement: "EDIT-07"
    verification:
      - kind: backstop
        ref: "owner phone check of the hi Main/editor page, recorded in 12-SPIKE.md (planned live check, not run here)"
        status: not-run
    human_judgment: true

duration: 20min
completed: 2026-11-05
status: complete
---

# Phase 12 Plan 05: The 13 locale strings and the rendered switch-cell check Summary

**Thirteen new editor strings and the reworded Rules hint in all 8 languages, with 13 limits rows and a test that renders the five half-width switch labels with the icon alone and holds them to 20 code points.**

## Accomplishments

- Task 1 (tracer): `settings_btn_edit_switch_once` and `settings_edit_line_once` in all 8 files, their limits rows, token rows, and the new rendered check (`editorCellMax = 20`, `editorCells`) wired through `TestSettingsLabelLengths` for both icons.
- Task 2: the other eleven keys (`settings_btn_edit_switch_welcome`, `_goodbye`, `_clean`, `_below`, `settings_edit_line_below`, `settings_edit_hint_once`, `settings_edit_hint_below_file`, `settings_edit_media_animation`, `settings_edit_refuse_media_rules_long`, `settings_edit_below_unavailable`, `settings_edit_refuse_text_rules_long`), the new `settings_edit_hint_rules` text in all 8 files (`<code>/rules</code>` kept), all five keys in `editorCells`, token rows (`{state}` for the five switch labels and `settings_edit_line_below`; `{n}` and `{max}` for the two Rules refusals).
- Translations follow the UI-SPEC per-locale note (es "Medio bajo texto", id "Media bawah teks", fr "Suppr. dernier", ru "Один раз"). The widest rendered labels were checked while inserting; none exceeded 20.
- Counts after the change: `grep -cE '^settings_' locales/en.yml` 199, `^settings_edit_` 70, `^settings_btn_edit_` 20.
- The four Phase 12 backed-up Ukrainian golden keys (`rules_for_chat`, `rules_click_for_rules`, `rules_not_set`, `rules_no_rules_set`) and `settings_edit_refuse_too_long_filled` were not touched; the four renames stay with plan 12-10.
- The generated docs did not change: `make generate-docs` then `make check-docs` reported no drift, so no docs commit was needed.

## Task Commits

1. **Task 1 (tracer): the Greet once strings** - `d07f46b` (feat)
2. **Task 2: the other eleven strings and the new Rules hint** - `c64dba2` (feat)

## Verification (real results)

- `go test -tags testtools -race -count=1 -run '^(TestSettingsLocaleKeys|TestSettingsLabelLengths|TestSettingsEditLocaleTokens)$' ./fuku/i18n` (Task 1): `ok`.
- `go test -tags testtools -race -count=1 -run '^(TestSettingsLocaleKeys|TestSettingsLabelLengths|TestSettingsEditLocaleTokens|TestSettingsHelpFitsOneMessage|TestSettingsReusedKeyPlaceholders)$' ./fuku/i18n` (Task 2): `ok`.
- `go test -tags testtools -race -count=1 ./fuku/i18n`: `ok` (14.5 s).
- Negative proof of the new check: with English `settings_btn_edit_switch_once` temporarily set to `{state} Greet once very very long`, `TestSettingsLabelLengths/en` failed with `editor cell "✅ Greet once very very long" ... has 27 characters, ceiling 20`; the value was restored before the commit.
- `make check-translations`: all translations present (run after each task).
- `make generate-docs` then `make check-docs`: no drift.
- Acceptance: `grep -c '^settings_btn_edit_switch_once:' locales/*.yml` gives 1 in each of the 8 language files; `grep -c editorCellMax` is at least 1; `grep -cE '^settings_' locales/en.yml` prints 199; `<code>/rules</code>` is present in all 8 files; the new English `settings_edit_hint_rules` starts with "Set takes my next private message as that part" (count 1).
- `python3 .../lint-local.py 72a692b`: exit 0 ("no dupl, gocyclo or godox findings"), before each code commit. `gofmt -l fuku/i18n` clean, `go vet -tags testtools ./fuku/i18n` clean.
- `git diff --exit-code 72a692b -- go.mod go.sum`: exit 0.
- Not run: `make lint` and `make test` (cannot run here, the plan does not ask), the settings test family over `./fuku/modules` (the plan does not ask), the hi phone check (a live check for `12-SPIKE.md`).

## Deviations from Plan

- **[Rule 3 - formatting] The new limits rows are a separate block** at the end of `settingsLocaleLimits` (blank line and comment first), not placed inside the editor block, because gofmt re-aligned the whole existing block when a shorter key joined it. Content is as the plan says.
- The `hi` Media under text button label is a shortened name (see key-decisions); a native-speaker read is the planned live check.

## Known Stubs

None. The new keys are not yet used by Go code (plans 12-04 and 12-10 to 12-13 are the users); they are locale data, not code, so no unused-helper finding applies.

## Threat Flags

None. T-12-15 mitigated by the rendered check (both icons, all 8 locales); T-12-16 mitigated: the only placeholders are `{state}`, `{n}` and `{max}`, asserted by the token test, and no new string names a group, user, stored text, button, link or the Staff Group; T-12-SC: go.mod and go.sum unchanged.

## Requirement coverage

`requirements-completed: []`. The plan contributes the strings of EDIT-07, EDIT-08 and EDIT-09 and completes none; all three stay Pending in `.planning/REQUIREMENTS.md`.

## Notes for later plans

- Plan 12-10 renames the four `*_toggle` / `clean_*` keys and replaces the `editorSwitches` check; the five-cell check already exists.
- A `settings_edit_media_animation` word ("GIF" in all 8 languages) is ready for plan 12-04.
- A plan that adds a settings key must add a limits row; the new rows live in the "Phase 12" block at the end of the table.

## Self-Check: PASSED

- All nine modified files exist and contain the new keys; commits `d07f46b` and `c64dba2` are ancestors of HEAD.
