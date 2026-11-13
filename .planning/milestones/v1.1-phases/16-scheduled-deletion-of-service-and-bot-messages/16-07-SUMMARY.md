---
phase: 16-scheduled-deletion-of-service-and-bot-messages
plan: 07
subsystem: i18n
tags: [deletion, locales, translations, settings-menu, limits-table, tracer]
status: complete

requires:
  - phase: 16-scheduled-deletion-of-service-and-bot-messages
    provides: "plan 16-02: deletionServiceRows and deletionBotRows with the NameKey, DescKey and HintKey this plan's table test reads; the UI-SPEC's 57 keys, ceilings and English"
provides:
  - "all 57 settings_ keys of the Deletion page (settings_page_del, settings_state_del_on and 55 settings_del_) in en, es, fr, hi, id, pt, ru and uk; every locale holds 312 settings_ keys"
  - "57 rows in settingsLocaleLimits, label-length groups for the page name, the two section names, the 12 service and 5 bot names (with each of the three state icons) and the 8 choice labels, settings_del_hint_punish_auto in memberReqQuotingKeys"
  - "TestDeletionKindKeysExist (fuku/modules): every table-read key, the delay words and the four parameter templates translate in all 8 languages"
affects: [16-08, 16-09]

plan_head_before: ef8a303
plan_head_after: bbfd4551aba35e711960eb73903888f5bf0226a5

actuals:
  tokens: 11900   # chars/4 over the added lines of ef8a303..bbfd455 (47537 chars; almost all of it is the 8 x 57 locale strings and their limits rows)
  tasks: 3
  commits: 5      # MEASURED: git rev-list --count ef8a303..HEAD (before this SUMMARY commit)

tech-stack:
  added: []
  patterns:
    - "rows before keys: a red commit adds the limits rows and tests, the green commit adds the keys in all 8 files (TestSettingsLocaleKeys requires the table to cover exactly the settings_ keys)"
    - "a table test for keys read through a row's NameKey, DescKey and HintKey, which make check-translations cannot see"

key-files:
  created:
    - fuku/modules/deletion_kinds_locale_test.go
  modified:
    - fuku/i18n/settings_locale_test.go
    - fuku/i18n/memberreq_locale_test.go
    - locales/en.yml
    - locales/es.yml
    - locales/fr.yml
    - locales/hi.yml
    - locales/id.yml
    - locales/pt.yml
    - locales/ru.yml
    - locales/uk.yml
    - AGENTS.md

key-decisions:
  - "The new keys sit directly before settings_help_msg in each file (it is the last settings_ key and follows the last Phase 15 block), in the UI-SPEC's table order"
  - "Hindi avoids a gendered first person (intro, lists, no-right and unknown lines, auto description are worded impersonally); ru and uk keep the masculine 'я не смог' the files already use"
  - "The two hub names are translated so that the rendered cell stays within 20: ru 'Служебные' and uk 'Службові' (the spec's hints), es 'Avisos de servicio', pt 'Avisos de serviço', fr 'Messages système'"
  - "fr 'Sanctions admin' stands for 'Manual punishments' because 'Sanctions manuelles' is 19 code points; the description beside it explains the commands"
  - "uk 'Негайно' for Delete now (the literal 'Видалити одразу' is 15, over the 13 ceiling)"

patterns-established:
  - "the label-length test gains a Deletion cell group (names x three icons) and the choice list gains the Deletion labels, in the existing style"

requirements-completed: []

coverage:
  - id: D1
    description: "the 57 keys exist, non-empty, in all 8 locale files within the UI-SPEC ceilings with placeholders equal to English's and no markup, and the limits table covers exactly the 312 settings_ keys"
    requirement: "PLAT-05"
    verification:
      - kind: unit
        ref: "TestSettingsLocaleKeys (red at 6d027ca and ec7df96, green at f42a040 and d7a596d)"
        status: pass
  - id: D2
    description: "the page name with its icon, each of the two section, twelve service and five bot names with each of the three state icons, and the Keep, Delete now and six delay choices with the selected prefix fit 20 and 13 EN / 15 other code points in every language"
    requirement: "DEL-01"
    verification:
      - kind: unit
        ref: "TestSettingsLabelLengths"
        status: pass
  - id: D3
    description: "settings_del_hint_punish_auto contains the words of mreq_btn_fixed verbatim in every language"
    requirement: "DEL-02"
    verification:
      - kind: unit
        ref: "TestMemberReqLocaleKeys (memberReqQuotingKeys gains the key)"
        status: pass
  - id: D4
    description: "every NameKey, DescKey and HintKey of the service and bot tables, the section names, the delay words and the {delay}, {state}, {n} templates resolve to a non-empty string carrying their parameter in all 8 languages"
    requirement: "DEL-08"
    verification:
      - kind: unit
        ref: "TestDeletionKindKeysExist"
        status: pass
---

# Phase 16 Plan 07: Deletion page strings in all 8 locales Summary

**The 57 settings_ keys of the Deletion page now exist in en, es, fr, hi, id, pt, ru and uk inside the UI-SPEC's ceilings, with their limits rows, label-length and quote checks and a table test for the keys the page reads through its row tables, so plan 16-08's page renders real translations in every language.**

## Task commits

1. Task 1 RED `6d027ca` test(16-07): the Deletion page name and state have limits
2. Task 1 GREEN (tracer) `f42a040` feat(16-07): the Deletion page name and state in all 8 locales
3. Task 2 RED `ec7df96` test(16-07): the Deletion page strings have limits
4. Task 2 GREEN `d7a596d` feat(16-07): the Deletion page strings in all 8 locales
5. Task 3 `bbfd455` docs(16-07): the locale count and the Deletion page strings guard

## Accomplishments

- `fuku/i18n/settings_locale_test.go`: 57 rows with the UI-SPEC ceilings, the page-list entry `{"🧹", "settings_page_del"}`, a `deletionNames` x `➖ 🗑 ⏳` cell group at most 20, and the Deletion labels in the `options` list (`✅ ` + label at most 13 EN / 15 other).
- `fuku/i18n/memberreq_locale_test.go`: `settings_del_hint_punish_auto` in `memberReqQuotingKeys`.
- `fuku/modules/deletion_kinds_locale_test.go`: `TestDeletionKindKeysExist` over both row tables, the section names, the delay words, and the four templates (`{delay}`, `{state}`, `{n}` twice) in 8 languages through `withRealLocales`.
- `locales/*.yml`: 55 + 2 keys in each of the eight files (312 `settings_` keys each, 55 `settings_del_`); `locales/config.yml` untouched.
- AGENTS.md: the Testing bullet says 312, that Phase 16 added 57, names the label groups, the quote key and `TestDeletionKindKeysExist` as the guard for table-read keys.

## Verification (real results)

All from /home/user/Fuku_Robot; these are i18n and table tests with no database.

| Gate | Result |
|------|--------|
| Task 1 `go test -tags testtools -race -count=1 -run '^(TestSettingsLocaleKeys\|TestSettingsLabelLengths)$' -v ./fuku/i18n` | red at `6d027ca` (missing key in all 8 languages), PASS at `f42a040` |
| Task 2 red at `ec7df96` | `TestSettingsLocaleKeys`, `TestMemberReqLocaleKeys` fail with the missing keys |
| Task 2 `-run '^(TestSettingsLocaleKeys\|TestSettingsLabelLengths\|TestSettingsReusedKeyPlaceholders\|TestMemberReqLocaleKeys\|TestSettingsHelpFitsOneMessage)$' -v ./fuku/i18n` | all 5 PASS |
| Task 2 `-run '^(TestDeletionKindKeysExist\|TestMemberReqRuleKeysExist\|TestServiceKindTable)$' -v ./fuku/modules` | all 3 PASS |
| `go test -tags testtools -race -count=1 ./fuku/i18n` (whole package) | ok (22.7 s) |
| `make check-translations` | "All translations present" for every file |
| `for f in locales/*.yml; grep -cE '^settings_'` | 312 for en, es, fr, hi, id, pt, ru, uk; 0 for config.yml; 55 `settings_del_` each |
| `make check-docs` | no drift |
| `gofmt -l` on the three touched Go files | no output |
| `go vet -tags testtools ./fuku/i18n ./fuku/modules` | exit 0 |
| `python3 .../lint-local.py 8c0c827` (exit read directly) | exit 0, "no dupl, gocyclo or godox findings" |
| `git diff --exit-code 8c0c827 -- go.mod go.sum` | exit 0 |
| `git diff --exit-code dd61b2c -- locales/config.yml` | exit 0 |
| Acceptance greps: `settings_page_del` per file 1; `settings_del_` per file 55; `settings_` per file 312; `settings_del_hint_punish_auto` in memberreq_locale_test.go 1; `255 `settings_` keys` in AGENTS.md 0; `312 `settings_` keys` and `TestDeletionKindKeysExist` in AGENTS.md at least 1 | as stated |

Not run: `make test`, `make lint` (cannot run here: the installed golangci-lint is built with Go 1.25), the `fuku/modules` settings family (the plan does not ask for it; no Go production code changed), `TestSettingsStaffPrivacy` (plan 16-09 owns the privacy sweep of the new page).

## Deviations from Plan

None. The plan executed as written. Notes that are not deviations:

- The brief's commit ledger file (`gsd-plan-head-before-16-07`) was not created; `plan_head_before` is the known HEAD `ef8a303` at the start and the commit count was measured with `git rev-list --count ef8a303..HEAD` (5).
- The keys were placed directly before `settings_help_msg` (the last `settings_` key of every file), which is "after the last Phase 15 `settings_` block".
- No plan gate text was broken.

## Known Stubs

None.

## Threat Flags

None. The strings carry no group title, member, ID, count or Staff Group reference; `TestSettingsReusedKeyPlaceholders` (no `{staff}`, no `{id}`) and the placeholder-equality check pass; the privacy sweep over the rendered page is plan 16-09's `TestSettingsStaffPrivacy`.

## Lists for the owner and for 16-SPIKE.md (plan 16-09 carries these)

### New keys, all 8 languages (native read needed for the seven translations)

en is the UI-SPEC's English verbatim. es, fr, hi, id, pt, ru, uk are the executor's translations of all 57 keys:
`settings_page_del`, `settings_state_del_on`, `settings_del_intro`, `settings_del_sec_service`, `settings_del_sec_bot`, `settings_del_list_svc`, `settings_del_list_bot`, `settings_del_hint_time`, `settings_del_hint_cleanservice`, `settings_del_hint_punish_auto`, `settings_del_no_right`, `settings_del_right_unknown`, `settings_del_state_keep`, `settings_del_state_now`, `settings_del_state_after`, `settings_del_delay_10s`, `_1m`, `_10m`, `_1h`, `_6h`, `_24h`, `settings_del_delay_custom`, `settings_del_currently`, `settings_del_svc_` (`joined left photo title pinned video forum boost timer background gifts other`), `settings_del_desc_` (same twelve), `settings_del_bot_` (`welcome goodbye rules auto manual`), `settings_del_desc_` (`welcome goodbye rules auto manual`).

### Keys within one character of their ceiling

| Key | Language | Length / ceiling |
|-----|----------|------------------|
| `settings_del_sec_service` | es "Avisos de servicio" | 18 / 18 |
| `settings_del_sec_bot` | uk "Повідомлення бота" | 17 / 18 |
| `settings_del_sec_service` | pt "Avisos de serviço" | 17 / 18 |
| `settings_del_state_now` | fr "Immédiatement", ru "Удалить сразу" | 13 / 13 (cell with `✅ ` is 15 / 15) |
| `settings_del_state_now` | id "Hapus segera" | 12 / 13 |
| `settings_del_state_now`, `settings_del_delay_1m`, `settings_del_delay_10m` | en | 10 / 11, 5 / 6, 6 / 6 |
| `settings_del_bot_manual` | en "Manual punishments" | 18 / 18 (cell 20 / 20) |
| names at 17 (cell 19 / 20) | en `timer`; es `bot_manual`, `gifts`; fr `gifts`; id `joined`; pt `timer`, `gifts`; ru `gifts`; uk `joined`, `gifts` | 17 / 18 |

### Keys that need the owner's native read (UI-SPEC "Native-speaker read")

- Delay labels and `settings_del_state_after` in hi, ru, uk: hi "10 सेकंड", "1 मिनट", "1 घंटा", "6 घंटे", "{delay} बाद"; ru "10 с", "1 мин", "1 ч", "Через {delay}"; uk "10 с", "1 хв", "1 год", "Через {delay}"; id uses "dtk", "mnt", "jam".
- `settings_del_state_now`: es "Borrar ya", fr "Immédiatement", hi "तुरंत हटाएँ", id "Hapus segera", pt "Apagar já", ru "Удалить сразу", uk "Негайно" (a shorter word than the literal translation).
- `settings_del_no_right` and `settings_del_right_unknown`: the right's name in quotes must be what the Telegram client shows ("Eliminar mensajes", "Supprimer des messages", "संदेश हटाएँ", "Hapus pesan", "Apagar mensagens", "Удаление сообщений", "Видалення повідомлень" are the executor's best recollection of each client).
- `settings_del_svc_*` and `settings_del_bot_*`: fr "Sanctions auto" and "Sanctions admin" (admin stands for manual, see key-decisions), es "Castigos auto", pt "Punições auto", ru "Ручные наказания" and uk "Ручні покарання" use the Member requirements page's word for punishment; hi combining marks (see phone check).
- `settings_del_desc_*`: the Telegram terms ("pinned", "video chat", "forum topic", "boost") as members see them in each client: es "fijado", "videochat", "tema del foro", "impulsó"; fr "épinglé", "chat vidéo", "sujet", "boosté"; pt "fixada", "chat de vídeo", "tópico", "impulsionou"; ru "закреплённое", "видеочат", "забустил"; uk "закріплене", "відеочат", "забустив"; id "disematkan", "obrolan video", "mem-boost".
- `settings_del_hint_punish_auto`: the quote of the button in each language's own marks (es, hi, id, pt “ ”; fr « »; ru, uk « »); the words equal `mreq_btn_fixed` verbatim (the test proves it).
- `settings_state_del_on` and `settings_del_currently`: es "{n} activas", fr "{n} actives", pt "{n} ativas" (feminine, for rules), ru "включено: {n}", uk "увімк.: {n}", hi "{n} चालू".
- Hindi sentences are worded without a gendered first person on purpose; a native reader may prefer "मैं ... सकता/सकती हूँ".

### Cells that need a phone-width check (A16-A9)

- hi, ru and uk cells of the Service list (two per row, up to 20 code points): hi "ऑटो-डिलीट टाइमर", "ग्रुप बैकग्राउंड", "पिन किया संदेश", "सदस्य ने छोड़ा" (combining marks make them look wider than their count); ru "Участник вступил", "Розыгрыш, подарки"; uk "Учасник долучився", "Розіграш, дарунки".
- The choice row of three at 15 code points: fr "✅ Immédiatement" and ru "✅ Удалить сразу" (15 / 15), hi "✅ 10 सेकंड" with combining marks.
- If a cell wraps, shorten the key in place; no key is added.

## Self-Check: PASSED

- Created files exist: `fuku/modules/deletion_kinds_locale_test.go` FOUND.
- Commits are ancestors of HEAD: `6d027ca`, `f42a040`, `ec7df96`, `d7a596d`, `bbfd455` FOUND.
- Requirements: DEL-01, DEL-02, DEL-06, DEL-07, DEL-08 and MREQ-15 left Pending (`requirements-completed: []`); PLAT-05 stays as it was (already ticked, cross-cutting).
