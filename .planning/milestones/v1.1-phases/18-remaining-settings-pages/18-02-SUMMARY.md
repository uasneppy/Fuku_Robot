---
phase: 18-remaining-settings-pages
plan: 02
subsystem: settings-menu
tags: [i18n, locales, settings-menu, limits-table, label-lengths]
status: complete

requires:
  - phase: 18-remaining-settings-pages
    provides: plan 18-01's four Warnings keys (334 settings_ keys) and the settingsLocaleLimits table this plan extends
provides:
  - the 51 remaining Phase 18 settings_ keys in all 8 locale files (385 settings_ keys each)
  - 51 rows in settingsLocaleLimits and settings_sw_ai_unavailable in settingsAlertKeys
  - the Phase 18 groups of TestSettingsLabelLengths (page cells, lock names with both icons, log categories, the eight Language cells, the four toggles, the two Main state words)
  - AGENTS.md records 385 keys and which keys are invisible to make check-translations
affects: [18-03, 18-04, 18-05, 18-06, 18-07, 18-08, 18-09, 18-10, 18-11, 18-12, 18-13, 18-14, 18-15]

plan_head_before: 2d41cf563afa9437bc1db0ee3bf592b8c7ee4312
plan_head_after: d8b299e21b111bfde54c899a64ee8ee0e1cdc040

actuals:
  tokens: 21500   # chars/4 over the added lines of 2d41cf5..d8b299e (86000 chars: 8 locale blocks, limits table, label groups, AGENTS.md)
  tasks: 2
  commits: 2      # MEASURED: git rev-list --count 2d41cf5..HEAD (before this SUMMARY commit)

tech-stack:
  added: []
  patterns:
    - "strings-first plan: every word a later page reads exists in 8 languages, within its ceiling, before the page code"
    - "table-field keys (page, category and lock names) guarded by the limits table and label-length groups, since check-translations cannot see them"

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
    - AGENTS.md

key-decisions:
  - "English text and ceilings are the UI-SPEC's word for word; other languages are shortened to fit, never truncated"
  - "Locks state pair: ru and uk use Закрыто/Открыто and Закрито/Відкрито (matching the page name 'Замки'), hi uses लॉक/अनलॉक, id uses Terkunci/Tidak terkunci"
  - "settings_lock_other kept as the three-way 'Game/sticker/GIF' in every language (it fits in 18 everywhere)"

requirements-completed: []

duration: ~45min
completed: 2026-11-12
---

# Phase 18 Plan 02: the strings Summary

**51 settings_ keys (Switches, Language, Log channel, Anonymous admins and Locks words) in all 8 locales with their limits rows, the Phase 18 cell-length checks, and 385 keys recorded in AGENTS.md.**

## What was done

- Task 1 (`ddddfc0`): 26 keys (five page names, `settings_state_sw_on`, `settings_state_locks_on`, the two Warnings action lines, the three Switches toggles, the two AI lines, the Language current line and hint, the Log channel status, hint and six category names, the Anonymous admins hint and toggle) in all 8 files after `settings_warns_hint`, same position and order in each; 26 rows in `settingsLocaleLimits`; `settings_sw_ai_unavailable` added to `settingsAlertKeys`.
- Task 2 (`d8b299e`): 25 keys (`settings_locks_intro`, `settings_locks_state_on`/`_off` and the 22 `settings_lock_*` names); 25 limits rows; the label-length groups; AGENTS.md Testing bullet (330 to 385, the 55 Phase 18 keys, the table-field keys and their guards).
- Ceilings, placeholders and markup were checked by a script (kept in the scratchpad, not committed) before any file was written: every value is within its "Max EN" / "Max other", `{...}` placeholders equal the English key's, and the markup tags equal English's (`<b>` only).
- `TestSettingsLabelLengths` now also holds: the five new page names with their icons at most 20; each of the 22 lock names with 🔒 and 🔓 at most 20; the six log categories with ✅ and ❌ at most 20; each locale's own `language_flag` + space + `language_name`, and `✅` + space + name, at most 20 (M36); the three Switches toggles and the Anonymous admins toggle rendered with the longer of On/Off at most 40; `settings_state_sw_on` with n=3 (at most 15) and `settings_state_locks_on` with n=22 (at most 25).

## Verification (real results)

| Gate | Result |
|------|--------|
| `grep -cE '^settings_' locales/<each of 8>.yml` | `385` in every file (baseline 334 after 18-01) |
| Task 1 acceptance grep (26 key pattern, all 8 files, `sort -u`) | single line `26` |
| `grep -c settings_sw_ai_unavailable fuku/i18n/settings_locale_test.go` | 2 |
| `grep -c settings_lock_anonchannel fuku/i18n/settings_locale_test.go` | 2 |
| `grep -c 385 AGENTS.md` | 1 |
| Go files in 18-02 commits | only `fuku/i18n/settings_locale_test.go` |
| `go test -tags testtools -race -count=1 -run '^(TestSettingsLocaleKeys\|TestSettingsReusedKeyPlaceholders\|TestSettingsEditLocaleTokens)$' ./fuku/i18n` (task 1) | 3 PASS |
| `go test ... -run '^(TestSettingsLocaleKeys\|TestSettingsLabelLengths\|TestSettingsReusedKeyPlaceholders\|TestSettingsHelpFitsOneMessage)$' -v ./fuku/i18n` | 4 PASS, no FAIL, no SKIP |
| whole `./fuku/i18n` package | ok |
| `make check-translations` | All translations present |
| `make check-docs` | No drift detected |
| `go vet -tags testtools ./fuku/i18n`; `gofmt -l fuku/i18n/settings_locale_test.go` | clean, prints nothing |
| `python3 .planning/phases/13-member-requirements/handoff/lint-local.py 8c0c827` | exit 0 (read directly) |
| `git diff --exit-code 8c0c827 -- go.mod go.sum` | exit 0 |
| root-pin guard before each edit/commit | exit 0 |
| whole `go test -tags testtools -race -count=1 -timeout 30m -v ./fuku/modules` (background, SQLite) | `ok ... 1606.885s`, `--- FAIL/SKIP` and `^FAIL` count 0, `exit 0` |

## Deviations from Plan

None. The plan executed as written. The AGENTS.md text names `TestSettingsLocksKeysExist` as a guard "from plan 18-11", as the plan's assumptions say; that test does not exist yet.

## Keys that most need a native-speaker read (recorded owner step, A18-A14)

1. The 22 `settings_lock_*` names in es, fr, hi, id, pt, ru, uk, first the ones where the Telegram client term is least certain: `videonote` (es "Nota de vídeo", fr "Message vidéo", pt "Mensagem de vídeo", id "Catatan video", ru "Видеосообщения"), `anonchannel` (the channel-posts wording), `previews`, `rtl`, `forward` (fr "Transferts", pt "Encaminhadas", id "Teruskan"), `game`, `other`, `media`, `all`.
2. `settings_locks_state_on` / `_off` pair: ru Закрыто/Открыто and uk Закрито/Відкрито (chosen to match the page name "Замки"), hi लॉक/अनलॉक, id Terkunci/Tidak terkunci, fr Verrouillé/Déverrouillé.
3. `settings_page_*` names (Anonymous admins and Log channel in particular; hi "गुमनाम एडमिन" looks wider than its code-point count: phone check).
4. `settings_warns_action_inert` ("I don't act on this action" must read as a fact, not an error) and `settings_sw_ai_off` / `settings_sw_ai_unavailable` (neutral, no operator or key named; the off sentence follows each language's existing `aispam_status_enabled` wording, e.g. es/pt "desactivado/desativado", id "sedang mati").
5. `settings_btn_sw_*` and `settings_btn_anon_toggle` in ru, uk, hi (the 40-point rendered limit is held by the test with the longer state word).
6. `settings_lang_hint` (own language vs group language unambiguous; `/lang` stays Latin), `settings_log_cat_*` (the user-actions words: ru "Действия юзеров", es "Usuarios" shortened to fit 18), `settings_anon_hint` ("owner" = creator).

## Known Stubs

None. No Go production code changed; the new keys are read by later page plans.

## Threat Flags

None.

## Requirements

Contributes to PAGE-01 to PAGE-06 (the words of their pages) and PLAT-05. Completes none: `requirements-completed: []`; `requirements.mark-complete` was not run. Plan 18-15 completes them.

## Self-Check: PASSED

- Commits `ddddfc0` and `d8b299e` exist and are ancestors of HEAD; all 8 locale files hold 385 `settings_` keys; the limits table covers exactly the `settings_` keys (`TestSettingsLocaleKeys` passes).
