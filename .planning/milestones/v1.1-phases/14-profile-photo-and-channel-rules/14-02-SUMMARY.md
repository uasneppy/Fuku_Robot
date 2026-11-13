---
phase: 14-profile-photo-and-channel-rules
plan: 02
subsystem: i18n
tags: [locales, i18n, member-requirements, settings-menu, translations]

requires:
  - phase: 14-profile-photo-and-channel-rules
    provides: plan 14-01 photo and channel rule storage
provides:
  - 4 new mreq_ keys and 12 new settings_ keys in all 8 locale files
  - 7 reworded keys (6 mreq_, settings_mreq_hint_mute) that no longer name a profile
  - memberReqLocaleLimits covering exactly 29 mreq_ keys, the settings limits table exactly 247 settings_ keys
  - rendered checks at notice cap 520, still-broken 160/200 with the four longest phrases, count answer at n 99
  - TestMemberReqNoProfileInReworkedKeys
affects: [14-05, 14-06, 14-07, 14-08, 14-09, 14-10, 14-11]

plan_head_before: e3c44823e20610185255dbeafacf6e24a82682af
plan_head_after: c42b65be50c55ba5e788ea075184cd7da5580653
actuals:
  tokens: 60000
  tasks: 2
  commits: 4

tech-stack:
  added: []
  patterns:
    - "A RED test commit (limits tables and rendered checks) precedes each GREEN locale commit; the locale files are edited by a script that replaces or inserts one key at a time and asserts each key matched once"

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
    - fuku/i18n/memberreq_locale_test.go
    - fuku/i18n/settings_locale_test.go

key-decisions:
  - "memberReqRender now takes a tail argument and a shared memberReqSplice; the remedy sentence is appended after one space to mreq_notice_warn, _kick and _mute only, as plan 14-08's memberReqNoticeText will"
  - "settings_mreq_here_nolink joined settingsAlertKeys (template at most 200), the other three here_ keys are line texts"
  - "hi mreq_rule_photo shortened to the 30-code-point 'कोई दिखती प्रोफ़ाइल फ़ोटो नहीं' to fit the 32 ceiling; no ceiling or check was changed"

requirements-completed: []

coverage:
  - id: D1
    description: "the group-facing strings of both new rules, the photo remedy and the count answer exist in 8 languages within 520, 160 and 200"
    requirement: "MREQ-10, MREQ-11, MREQ-12 (contribution only)"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^TestMemberReqLocaleKeys$' ./fuku/i18n"
        status: pass
    human_judgment: true
  - id: D2
    description: "the settings page strings, the rule cells at 20 code points with five icons, and no reworded key naming a profile"
    verification:
      - kind: unit
        ref: "TestSettingsLocaleKeys, TestSettingsLabelLengths, TestMemberReqNoProfileInReworkedKeys"
        status: pass
    human_judgment: true

duration: 35min
completed: 2026-11-08
status: complete
---

# Phase 14 Plan 02: Locale strings for the photo and channel rules Summary

**16 new and 7 reworded locale keys in all 8 languages: the photo and channel phrases, the photo remedy, the count answer, the settings page words of both rules, and neutral member notices that no longer tell a channel-only breach to fix a profile, all inside their ceilings and proved by the two locale test files.**

## Accomplishments

- Four new `mreq_` keys (`mreq_rule_photo`, `mreq_rule_channel`, `mreq_remedy_photo`, `mreq_fixed_still_broken_many`) and six reworded ones (`mreq_notice_warn`, `_kick`, `_mute`, `_fixed`, `_fixed_lockdown`, `mreq_fixed_still_broken`) in en, es, fr, hi, id, pt, ru and uk. English is the plan's table text exactly; the others are real translations under the "other" ceilings. `mreq_notice_mute` keeps each language's `mreq_btn_fixed` words verbatim inside its quotes.
- Twelve new `settings_` keys (two rule names, two cells, two descriptions, two hints, four status lines) and the reworded `settings_mreq_hint_mute` in all 8 files, placed with their families (rule names after `settings_mreq_rule_chinese`, cells after `settings_btn_mreq_chinese`, descriptions after `settings_mreq_desc_chinese`, hints after `settings_mreq_hint_blank`, status lines after `settings_mreq_confirm_mute`). The privacy sentence lives in `settings_mreq_desc_photo` only.
- `memberreq_locale_test.go`: four limit rows (29 keys), seven rule phrases, `memberReqNoticeMax` 520 (comment and messages follow), `mreq_fixed_still_broken_many` in `memberReqAnswerKeys`, `memberReqRender` with a tail and `memberReqSplice`/`memberReqLongestPhrases` helpers, the still-broken render with the four longest phrases, the `_many` render at n 99, the three notices also rendered with the photo remedy after one space, and the new `TestMemberReqNoProfileInReworkedKeys` (per-language stems, exactly four English photo keys say "profile" across the three families, "counts as missing" only in `settings_mreq_desc_photo`).
- `settings_locale_test.go`: twelve rows (247 keys), `mreqRuleCells` has seven entries rendered with the five icons, `settings_mreq_here_nolink` is in the alert list.

## Task Commits

1. `c275dc0` test(14-02): the mreq locale checks for seven rules and the photo remedy (RED: 32 missing-key failures)
2. `173e131` feat(14-02): the photo and channel phrases and the neutral member notices in 8 languages (GREEN)
3. `89c8d5e` test(14-02): the settings locale checks for the photo and channel rules (RED: new cell and limit rows failed in every language)
4. `c42b65b` feat(14-02): the settings page words of the photo and channel rules in 8 languages (GREEN)

## Verification (real results)

- RED confirmed before each GREEN commit (`go test ... -run '^TestMemberReqLocaleKeys$' ./fuku/i18n` FAIL on missing keys; the settings tests FAIL on `settings_btn_mreq_photo is missing`); each test and its commit were separate commands.
- Task 1 GREEN: first run failed on one real finding (hi `mreq_rule_photo` 35 > 32), fixed by shortening the translation; then `TestMemberReqLocaleKeys` passes in all 8 languages.
- `go test -tags testtools -race -count=1 -run '^(TestSettingsLocaleKeys|TestSettingsLabelLengths|TestSettingsHelpFitsOneMessage|TestMemberReqLocaleKeys|TestMemberReqNoProfileInReworkedKeys)$' ./fuku/i18n`: ok (passed on the first run after the keys went in).
- `go test -tags testtools -race -count=1 ./fuku/i18n`: ok (whole package, after both tasks).
- `go test -tags testtools -race -count=1 -run '^(TestMemberReqScreensFitEveryLocale|TestSettingsMreqPageFitsEveryLocale|TestSettingsMainPageFitsEveryLocale|TestMemberReqNoticeLanguage|TestMemberReqFixed)' ./fuku/modules`: ok (SQLite, 21 s).
- `go test -tags testtools -race -count=1 -run '^(TestMemberReq|TestSettingsMreq|TestSettingsMain)' ./fuku/modules`: ok (SQLite, 88 s). The full settings family (`^(TestSettings|TestLoadSettings)`) was not run: no Go code of the plan changed and the brief reserves it for plans that say so.
- `make check-translations`: "All translations are present". `make generate-docs` then `make check-docs`: "No drift detected" and `git status` shows no generated page changed (none of these strings is command help).
- `gofmt -l fuku/i18n`: nothing. `go vet -tags testtools ./fuku/i18n`: clean.
- Lint stand-in `python3 .planning/phases/13-member-requirements/handoff/lint-local.py 8c0c827`: exit 0 read directly ("no dupl, gocyclo or godox findings on lines added since 8c0c827"). `make lint` and gosec cannot run in this sandbox (Go 1.25 golangci-lint); no new non-test Go code was added.
- `git diff --exit-code 8c0c827 -- go.mod go.sum`: clean.
- Acceptance greps: every locale file has at least 29 `^mreq_` lines and 247 `^settings_` lines; `memberReqNoticeMax = 520` present; `settings_btn_mreq_channel` appears in the settings test twice; the English `profile` count over the three families is 4.

## Deviations from Plan

None - plan executed as written. One judgment call: the hi photo phrase was shortened to fit its ceiling (a translation change, not a ceiling change).

## Known Stubs

None. No Go code reads the new keys yet (plans 14-05 to 14-10 do); the test tables above are their only users, as the plan intends.

## Keys that most need a native-speaker read

Translated by the executor without a native read; the owner's read is listed in plan 14-12's `14-SPIKE.md`. Highest risk first:

- hi (Hindi), all new and reworded keys, above all `mreq_rule_photo` ("कोई दिखती प्रोफ़ाइल फ़ोटो नहीं", a shortened noun phrase), `mreq_rule_channel` ("चैनल सब्सक्रिप्शन नहीं"), `mreq_notice_kick`/`_mute` ("जो सूचीबद्ध है ..."), `settings_mreq_desc_photo`, `settings_mreq_hint_mute`.
- ru and uk, the phrases that fill the `{rules}` slot (`mreq_rule_photo`, `mreq_rule_channel`) and `mreq_fixed_still_broken_many` ("нарушено правил — {n}" / "порушено правил — {n}", grammar of the count), and the kick notices ("когда указанное будет исправлено").
- id, `mreq_rule_channel` ("belum berlangganan kanal": Telegram's Indonesian word for a channel) and `settings_mreq_hint_photo` (kept the English action names Kick and Ban as the page's existing words do).
- fr, `mreq_notice_kick` ("une fois ce qui est indiqué corrigé"), `settings_mreq_here_*` with the space before the colon.
- es and pt, `mreq_rule_channel` ("sin suscripción al canal", "sem inscrição no canal") and the neutral "lo indicado" / "o que foi listado" wording of the notices.

## Threat Flags

None. No string carries a group title, member name, chat ID, count, channel title or the Staff Group; placeholders are `{state}`, `{rule}`, `{n}`, `{name}`, `{rules}` and `{limit}` only (T-14-06).

## Self-Check: PASSED

- Commits `c275dc0`, `173e131`, `89c8d5e` and `c42b65b` are ancestors of HEAD; `git rev-list --count e3c4482..HEAD` was 4 before this SUMMARY.
- All 8 locale files and both test files exist and are committed.
