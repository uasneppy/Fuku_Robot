---
phase: 17-command-edit-and-self-destruct-deletion
plan: 08
subsystem: i18n
tags: [locales, settings-menu, deletion, member-messages, 8-languages]

requires:
  - phase: 17-command-edit-and-self-destruct-deletion
    provides: the approved 17-UI-SPEC.md copy table (keys 58 to 75, ceilings, native-read list) and Phase 16's 57 settings_del_* keys
provides:
  - the 18 new settings_ keys of the Member messages section in en, es, fr, hi, id, pt, ru and uk (330 settings_ keys per file)
  - the changed settings_del_intro (names service notices, my own messages and members' messages)
  - 18 new rows in settingsLocaleLimits and extended label-length groups in TestSettingsLabelLengths
affects: [17-09, 17-10, 17-11, 17-12]

plan_head_before: f9f4809
plan_head_after: 7965e22b3031c068e678a2cd56966a4547a4e7de

actuals:
  tokens: 5500   # chars/4 over the added lines of f9f4809..7965e22 (22086 chars), locale text included
  tasks: 2
  commits: 2     # MEASURED: git rev-list --count f9f4809..HEAD before this SUMMARY commit

tech-stack:
  added: []
  patterns:
    - "keys read through table fields (NameKey, DescKey, HintKey) are guarded by the limits table and the label-length test, not by make check-translations"

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
  - "English text is the UI-SPEC's exactly; the other seven are best natural wording under the Max other ceilings, written as one block after settings_del_desc_manual in each file."
  - "The 20-code-point name check renders every deletion name with all four icons (the three state icons and the section's 💬), which is slightly stricter than the plan's pairing of 💬 with the section name only; every name passes."
  - "state_on pairs with each locale's settings_state_off_plain: es Encendido/Apagado, fr On/Off, pt On/Off, id Nyala/Mati, ru Вкл/Выкл, uk Увімк./Вимк., hi चालू/बंद."

requirements-completed: []
status: complete
---

# Phase 17 Plan 08: Member messages strings Summary

**The 18 Member messages words and the changed deletion intro exist in all 8 languages within their UI-SPEC ceilings, with limits rows and cell-length checks, so the page plans can render real text.**

## What changed

- `locales/{en,es,fr,hi,id,pt,ru,uk}.yml`: 18 keys added after `settings_del_desc_manual` in the UI-SPEC order (`settings_del_sec_member` to `settings_del_confirm_self`), and `settings_del_intro` rewritten. `settings_del_confirm_self` holds a real blank line as `\n\n` in a double-quoted YAML value, like `settings_edit_wait_text`. Placeholders: `{delay}` in `settings_del_state_grace` and `settings_del_confirm_self` only. No new text names a group, member, ID, count, the Staff Group or a command.
- `fuku/i18n/settings_locale_test.go`: 18 rows in `settingsLocaleLimits` with the UI-SPEC "Max EN" and "Max other" columns (the intro row stays `{125, 176}`); `TestSettingsLabelLengths` gains the four member names in `deletionNames` (icons now `➖ 🗑 ⏳ 💬`) and `settings_del_state_on`, `settings_del_delay_30s`, `settings_del_delay_5m`, `settings_del_delay_15m` in `options` (`✅ ` + label at most 13 EN, 15 elsewhere).
- `AGENTS.md`: the Testing locale sentence reads 330 `settings_` keys, and a Phase 17 sentence records the 18 keys, the changed intro, the guards and that the member rows' keys are invisible to `make check-translations`.

## Verification (real output)

- `for f in en es fr hi id pt ru uk: grep -cE '^settings_'` prints one line: `330` (baseline 312).
- `grep -c '^settings_del_confirm_self:'` per file, `sort -u`: `1`. `grep -c 'messages from members' locales/en.yml`: `2`.
- `go test -tags testtools -race -count=1 -run '^(TestSettingsLocaleKeys|TestSettingsReusedKeyPlaceholders)$' -v ./fuku/i18n`: both PASS, all 8 language subtests.
- `make check-translations`: `All translations present`, `Summary: All translations are present!`.
- `go test ... -run '^(TestSettingsLabelLengths|TestSettingsLocaleKeys|TestSettingsHelpFitsOneMessage|TestSettingsEditLocaleTokens)$' -v ./fuku/i18n`: all four PASS.
- `go test -tags testtools -race -count=1 ./fuku/i18n`: `ok ... 25.793s`.
- `go test -tags testtools -race -count=1 -run '^(TestSettingsDeletionPageFitsEveryLocale|TestDeletionKindKeysExist)$' ./fuku/modules`: `ok ... 4.042s`.
- `gofmt -l fuku/i18n`: empty. `go vet -tags testtools ./fuku/i18n`: clean.
- `git diff --exit-code 8c0c827 -- go.mod go.sum`: exit 0.
- `grep -c 'now hold 312' AGENTS.md`: 0; `grep -c '330 `settings_` keys' AGENTS.md`: 1.

Not run: the full settings family (`-run '^(TestSettings|TestLoadSettings)' ./fuku/modules ./fuku/i18n`, about 4.5 minutes), because this plan changes no Go code of `fuku/modules` and its `<verification>` lists the narrower runs above; `make test` and `make lint` (plan 17-12 and the sandbox respectively). The lint stand-in was not run: only a test file and non-Go files changed, and the test file compiles, vets and is gofmt-clean.

## Deviations from Plan

None. Plan executed as written. The plan did not require creating the commit-ledger file of protocol 0c before the first commit; `plan_head_before` was taken from the HEAD before this plan's first commit (`f9f4809`) and `commits:` is measured from it.

## Native-speaker read (owner step, not done)

None of the seven translations has had a native read. Where it matters most, in order (UI-SPEC A17-A13):

1. `settings_del_state_on`, the On/Off pairing: es `Encendido` against `Apagado`, id `Nyala` against `Mati`, hi `चालू`/`बंद`, ru `Вкл`/`Выкл`, uk `Увімк.`/`Вимк.`, fr and pt `On` against the existing `Off`.
2. The unit words: `settings_del_delay_30s`, `_5m`, `_15m` (hi spells out `30 सेकंड`, `5 मिनट`, `15 मिनट`, id `dtk`/`mnt`, ru `с`/`мин`, uk `с`/`хв`) and `settings_del_state_grace` with its word for "grace": es `Gracia`, fr `Grâce`, pt `Tolerância`, id `Tenggang`, ru `Отсрочка`, uk `Відстрочка`, hi `ग्रेस`. "Grace time" is the least certain term in every language and `{delay}` must read as a phrase in each word order.
3. Phone-width check (A16-A9) for the cells that look wide: hi `settings_del_mem_edited` (`एडिट किए संदेश`, 14 code points with combining marks), hi `settings_del_sec_member`, ru `settings_del_mem_edited` (`Отредактированные`, 17), uk `settings_del_state_grace` (`Відстрочка {delay}`, 18 before the delay), uk `settings_del_mem_self` and `settings_del_mem_edited`.
4. Section names shortened to fit 18: es `De miembros`, pt `De membros`, ru `От участников`, uk `Від учасників`; and "approved members" uses each language's Approvals word (es aprobados, fr approuvés, hi स्वीकृत, id disetujui, pt aprovados, ru одобренные, uk затверджені).
5. `settings_del_desc_commands` and `settings_del_hint_commands` ("my reply stays", "another bot"), the three self-destruct texts (pinned wording) and `settings_del_intro`, which must still name all three groups.

## Known Stubs

None.

## Threat Flags

None. No endpoint, auth path, file access or schema change.

## Requirement coverage

Contributes to DEL-03, DEL-04, DEL-05 and PLAT-05; completes none. `requirements-completed: []`. `requirements.mark-complete` was not called.

## Self-Check: PASSED

- `locales/en.yml` through `locales/uk.yml`, `fuku/i18n/settings_locale_test.go`, `AGENTS.md`: present and modified.
- Commits `36a1e29` (feat) and `7965e22` (test) are ancestors of HEAD.
