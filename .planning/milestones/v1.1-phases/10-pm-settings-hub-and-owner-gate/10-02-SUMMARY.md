---
phase: 10-pm-settings-hub-and-owner-gate
plan: 02
subsystem: i18n
tags: [locales, i18n, settings-menu, owner-gate, ui-spec, docs]
requires:
  - phase: 09-captcha-web
    provides: the settings_ locale block, settingsLocaleLimits and the settings locale guard test
provides:
  - the 19 new settings_ keys (hub list, Access page, owner-gate refusals) in all 8 locales
  - the rewritten settings_help_msg in all 8 locales
  - TestSettingsLabelLengths rewritten to the two-column grid sizing checks
affects: [10-03, 10-04, 10-05, 10-06, 10-07, 10-08, 10-09, settings-hub, owner-gate]

actuals:
  tokens: 10900
  tasks: 2
  commits: 2

commits: 2
plan_head_before: 1f01dab4f5a12362511226a937b5a5a82866ffd0
plan_head_after: 62b7ca93a5a727b4f0a430e9c8d9dacd3bdc2001

tech-stack:
  added: []
  patterns:
    - "Every new settings_ key gets a settingsLocaleLimits row in the same commit; alert keys join settingsAlertKeys"
    - "Button sizing is asserted on the rendered label in code points (icon + space + name), never on the template"

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
  - "No new string carries a placeholder other than {icon}, {page} and {state}, so no hub or refusal text can carry a group title, a user name or the Staff Group."
  - "The three alert keys are held to 160 code points and tag-free in every language, so a translation still fits Telegram's 200."
  - "The French status line keeps the French space before the colon; the placeholders stay byte-identical to English."

requirements-completed: [PLAT-05, MENU-13]

duration: 8min
completed: 2026-11-04
status: complete

coverage:
  - deliverable: "19 new settings_ keys present, non-empty and within ceiling in all 8 locales"
    verification:
      - kind: test
        ref: "fuku/i18n/settings_locale_test.go#TestSettingsLocaleKeys"
        status: pass
      - kind: command
        ref: "make check-translations"
        status: pass
    human_judgment: false
  - deliverable: "Alert keys tag-free and at most 160 code points; no new placeholder beyond icon, page and state"
    verification:
      - kind: test
        ref: "fuku/i18n/settings_locale_test.go#TestSettingsLocaleKeys"
        status: pass
      - kind: test
        ref: "fuku/i18n/settings_locale_test.go#TestSettingsReusedKeyPlaceholders"
        status: pass
    human_judgment: false
  - deliverable: "Rendered grid label at most 20, Access toggle at most 40, Groups button at most 18 in all 8 languages"
    verification:
      - kind: test
        ref: "fuku/i18n/settings_locale_test.go#TestSettingsLabelLengths"
        status: pass
    human_judgment: false
  - deliverable: "settings_help_msg rewritten in all 8 locales, b tag only, names every command, fits one message"
    verification:
      - kind: test
        ref: "fuku/i18n/settings_locale_test.go#TestSettingsHelpFitsOneMessage"
        status: pass
      - kind: command
        ref: "make check-docs"
        status: pass
    human_judgment: false
  - deliverable: "Quality of the es, fr, hi, id, pt, ru and uk wording (idiom, tone, real-device truncation of the grid names and toggle, tap-to-copy of the code tags)"
    human_judgment: true
    rationale: "Length and placeholder rules are tested; whether a translation reads naturally and is not ellipsized on a phone is a native-speaker and device check (UI-SPEC backstop row)."
---

# Phase 10 Plan 02: Settings hub locale strings Summary

**The 19 hub, Access and owner-gate strings plus the new /help text in all 8 languages, guarded by per-key code-point ceilings and the two-column button sizing checks.**

## Performance

- **Duration:** 8 min
- **Started:** 2026-11-04T12:33:00Z (approximate)
- **Completed:** 2026-11-04T12:41:00Z
- **Tasks:** 2 (a tracer and an auto task)
- **Files modified:** 10 (0 created)

## Accomplishments

- Added `settings_page_groups`, `settings_page_access`, `settings_btn_groups`, `settings_main_state_line`, `settings_state_access_on/off`, the list intro, hint and empty texts, the Access status, description, toggle and confirm texts, and the five refusals (`settings_gate_owner_only`, `settings_access_owner_only`, `settings_pick_unavailable`, `settings_refuse_owner_only`, `settings_start_owner_only`) to en, es, fr, hi, id, pt, ru and uk, in the UI-SPEC wording. `settings_list_hint` keeps `<code>/admincache</code>` and `<code>/settings</code>`; no Access string says "you" and no refusal names a group, the switch or the Staff Group.
- Extended `settingsLocaleLimits` with one row per key (UI-SPEC "Max EN" and "Max other"), added the three alert keys to `settingsAlertKeys`, and changed the guard's doc comment to 8 locales.
- Rewrote `settings_help_msg` in all 8 files (private list, owner gate, setting commands unchanged) with ceilings 1100 / 1540. The measured help totals are 856 to 982 UTF-16 units with the header, far under Telegram's 4096.
- Replaced the body of `TestSettingsLabelLengths` (name kept) with the rendered-size checks: grid label (icon, space, name) at most 20 for Staff Group, Lockdown, Captcha, Antiflood and Access; Access toggle rendered with "✅ " plus `common_status_on` and "❌ " plus `common_status_off` at most 40; Groups button at most 18; choice rows unchanged at 13 / 15. It prints one PASS subtest per language of the 8.
- `make generate-docs` rewrote only `docs/src/content/docs/commands/settings/index.md`; `api-reference/commands.md` needed no change. `make check-docs` reports no drift.

## Task Commits

1. **Task 1 (tracer): the 19 keys in all 8 locales and the guard rows** - `22f26d3` (feat)
2. **Task 2: new /help text, two-column sizing checks, regenerated docs** - `62b7ca9` (feat)

**Plan metadata:** committed with this SUMMARY (docs).

## Baseline and verification

- `make check-translations` baseline at HEAD (`1f01dab`): exit 0, "All translations are present". No older uk gap exists, so there is no "Owner decision" item. After the change: exit 0.
- `go test -tags testtools -race -count=1 ./fuku/i18n`: ok (15.8 s).
- `go test ... -run '^(TestSettingsLocaleKeys|TestSettingsReusedKeyPlaceholders)$' ./fuku/i18n`: PASS.
- `go test ... -run '^(TestSettingsLabelLengths|TestSettingsHelpFitsOneMessage|TestSettingsLocaleKeys)$' -v ./fuku/i18n`: PASS, 8 languages each.
- `make check-docs`: "No drift detected".
- `git diff --exit-code 6ef6ad9 -- go.mod go.sum`: clean (exit 0).
- Acceptance greps: `settings_gate_owner_only:` present once in each of the 8 files; 19 new keys in `locales/uk.yml`; `<code>/admincache</code>` once in en; 6 alert-key matches in the test; no "all 7" wording left in the test; `settings_help_msg": {1100, 1540}` once; `settings_btn_groups` twice; `🔑` once.

## Decisions Made

See `key-decisions` above. The wording follows the UI-SPEC English exactly; the other seven languages are new translations shortened to fit the rendered ceilings (for example uk "Адміни змінюють налаштування: {state}").

## Deviations from Plan

None - plan executed exactly as written.

Process note: the two commits were split by temporarily keeping the previous `settings_help_msg` block while committing Task 1, then restoring the finished files for Task 2, so each commit passes its own tests.

## Issues Encountered

None.

## Known Stubs

None.

## Threat Flags

None. The new strings add no network endpoint, auth path or schema; T-10-06 to T-10-08 are mitigated by the guard test (placeholders compared with en, tag allow-list, alert keys tag-free).

## Next Phase Readiness

Ready for 10-03. Later code plans only read these keys. Still open for other plans: the locale-set guard and 8-locale wording in project documents (10-03), the AGENTS.md locale count (10-04).

## Self-Check: PASSED

- All 8 locale files and `fuku/i18n/settings_locale_test.go` and the generated settings docs page exist.
- Commits `22f26d3` and `62b7ca9` are ancestors of HEAD.
