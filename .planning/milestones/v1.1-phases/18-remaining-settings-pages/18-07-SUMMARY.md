---
phase: 18-remaining-settings-pages
plan: 07
subsystem: settings-menu
tags: [telegram, settings-menu, language, i18n, golden, apply-function, docs]
status: complete

requires:
  - phase: 18-remaining-settings-pages
    provides: "plans 18-02 (the Phase 18 string keys settings_page_lang, settings_lang_current, settings_lang_hint in 8 locales) and 18-06 (the Main-shape constants and the page-test toolkit)"
provides:
  - "TestLegacyLanguageRequests: 21 cases pinning the change_language callback (group codes, a code stored again, no code, unsupported and upper-case codes refused before the admin check, a non-admin, a failed save, a press with no message, the private branch that writes the user's own language) and /lang's display, committed alone before any production edit"
  - "applyGroupLanguage, groupLanguageCodes, the languageWrites seam and langApplyResult in language.go; the callback's group branch maps onto it and keeps its order and answers"
  - "lang.LoadGroupLanguageContext: the chat_lang key the bot reads, error returned, nothing created"
  - "The Language page (u, Order 87, icon globe): control l (st only, two lower-case letters, settingsRightAdmin), the grid of the enabled languages in their own words with a check mark in place of the stored one's flag, and the press translator swapped to the code just written in a group menu only"
affects: [18-08, 18-15]

plan_head_before: 3ef63af49d51cdb1d089d1575b1c7c066950c5da
plan_head_after: 14f211a0531c83b039a8e9fa88106e8298e9c29b

actuals:
  tokens: 19700   # chars/4 over the added non-planning lines of 3ef63af..14f211a (78888 chars), tests and docs included
  tasks: 4
  commits: 6   # MEASURED: git rev-list --count 3ef63af..HEAD (before the SUMMARY and state commits)

key-files:
  created:
    - fuku/modules/language_legacy_request_test.go
    - fuku/modules/language_apply_test.go
    - fuku/db/lang/load_group_test.go
    - fuku/modules/settings_language.go
    - fuku/modules/settings_language_test.go
    - fuku/modules/settings_language_screens_test.go
  modified:
    - fuku/modules/language.go
    - fuku/db/lang/repository.go
    - fuku/modules/settings_main_test.go
    - AGENTS.md
    - README.md
    - docs/src/content/docs/api-reference/callbacks.md
    - docs/src/content/docs/api-reference/permissions.md
    - .planning/phases/18-remaining-settings-pages/18-SPIKE.md

key-decisions:
  - "The group language write lives in the change_language callback (not in /lang, which only shows the keyboard), and so does the private branch that writes the user's own language. applyGroupLanguage is the group branch only; the page never reaches the private one (D-21): the grep for ChangeUserLanguage in settings_language.go finds nothing outside comments and TestSettingsLanguagePrivate checks users.language is unchanged."
  - "The callback keeps its order (code check against groupLanguageCodes, admin check, apply). The code check stays in the callback because it answers in the current language before the admin check, which the golden pins; applyGroupLanguage repeats the check as its own guard (a code outside the eight is langUnsupported with no write)."
  - "A stored code the bot has no locale for (a database edit) is drawn escaped as stored and marks nothing; a stored code that has a locale but is not enabled is drawn by its own name and marks nothing because no cell exists for it."
  - "The translator swap sits in the control's Set (settings_pages.go and the press pipeline are untouched): after a successful write in a group menu p.tr becomes i18n.MustNewTranslator(code just written), never a re-read, so another replica's local cache layer cannot undo it; in a private menu p.tr is not touched."
  - "applyGroupLanguage takes a context and refuses a cancelled one before writing, because lang.ChangeGroupLanguage takes none; the callback passes context.Background()."

patterns-established:
  - "A page whose command is a callback records the golden through the callback handler, not a command, and a table of cases with the stored language seeded by the real repository (a different code before each, because the repository returns early for an equal one)."

requirements-completed: []

coverage:
  - id: D1
    description: "The change_language callback and /lang send and store exactly what they did before the extraction"
    requirement: "PAGE-03 (contribution only)"
    verification:
      - kind: integration
        ref: "go test -tags testtools -race -count=1 -run '^(TestLegacyLanguageRequests|TestLanguageCallback.*)$' ./fuku/modules"
        status: pass
    human_judgment: false
  - id: D2
    description: "applyGroupLanguage writes each of the eight codes through lang.ChangeGroupLanguage and drops chat_lang, chat_settings and chat; unsupported codes and failed writes are typed results"
    requirement: "PAGE-03 (contribution only)"
    verification:
      - kind: integration
        ref: "go test -tags testtools -race -count=1 -run '^TestApplyGroupLanguage$' ./fuku/modules; go test -tags testtools -race -count=1 -run '^TestLoadGroupLanguageContext$' ./fuku/db/lang"
        status: pass
    human_judgment: false
  - id: D3
    description: "The Language page sets the group's language, redraws a group menu in the new language and keeps a private menu in the opener's language without writing it"
    requirement: "PAGE-03 (contribution only)"
    verification:
      - kind: integration
        ref: "go test -tags testtools -race -count=1 -run '^(TestSettingsLanguageGroupRedraw|TestSettingsLanguagePrivate|TestSettingsLanguageSameAndUnknown|TestSettingsLanguageForged)$' ./fuku/modules"
        status: pass
    human_judgment: true
  - id: D4
    description: "The page matches the callback on write, cache drop, read-back and rights, creates nothing on read, and fits 64-byte buttons and 3800 UTF-16 units in 8 languages"
    requirement: "PAGE-10 (contribution only)"
    verification:
      - kind: integration
        ref: "go test -tags testtools -race -count=1 -run '^(TestSettingsParityLanguage|TestSettingsDriftLanguage|TestSettingsAuthorityParityLanguage|TestSettingsLanguageReadsCreateNothing|TestSettingsLanguageCallbackBytes|TestSettingsLanguagePageFitsEveryLocale)$' ./fuku/modules"
        status: pass
    human_judgment: false
---

# Phase 18 Plan 07: The Language page Summary

The group's language is set from a Language page through the same write the language buttons of `/lang` use, a menu opened in the group redraws in the new language at once, and a menu opened in private keeps the opener's own language and never writes it.

## What changed

- **Golden (commit 8d8aa49, alone, before any production edit).** `language_legacy_request_test.go`: `TestLegacyLanguageRequests`, 21 cases from `-1009918001501` and fresh member IDs. It records the `answerCallbackQuery`, `editMessageText` and `sendMessage` requests in order (chat ID blanked) and the stored `chats.language` or `users.language` for the group codes `en es hi uk id`, a code pressed again, a first write on a chat with no row, no `l`, `de`, `UK`, `de` pressed by a member (refused as invalid before the admin check), a non-admin, a failed save (a real GORM callback on the update chain of `chats`, and of `users` for private), a press with no message, the private branch (new user, existing user, unsupported code, failed save) and `/lang` in a group (admin and member) and in private. It passed unchanged after the extraction and was never re-recorded.
- **Extraction and reader (commit cd87587).** `language.go`: `groupLanguageCodes` (`uk en es fr hi ru pt id`), `languageWrites` (default `lang.ChangeGroupLanguage`), `langApplyResult` and `applyGroupLanguage(ctx, chatID, code)` (outcomes `langApplied` as the zero value, `langUnsupported`, `langSaveFailed`); the callback's code check reads `groupLanguageCodes` and its group branch maps the result, the private branch untouched. `fuku/db/lang/repository.go`: `LoadGroupLanguageContext` shares one loader with `getCachedLanguage` (now a wrapper that returns `uk` on an error as before). Tests: `TestApplyGroupLanguage` (each of the eight codes applied over a different seeded code with the three keys proved dropped, the stored code again writes and drops nothing, unsupported codes and failed writes, a cancelled context) and `TestLoadGroupLanguageContext`. AGENTS.md: the "One setting, one code path" bullet gains the group-language paragraph.
- **The page (commit 1b4e39a).** `settings_language.go` (page `u`, `Order` 87, icon 🌐, no `Hidden`): `settingsLangRead`, `settingsLangCodeShape` (two lower-case ASCII letters), `settingsLangMainState`, `settingsLangOpen`, the pure `renderSettingsLanguage` (current line, hint, cells of `config.AppConfig.ValidLangCodes` two to a row, each `language_flag` and `language_name` of its own locale, the stored one `✅` plus its name, Back and Close; no translate link) and `settingsLangSet` (read before; the stored code redraws with no notice and no write; otherwise `applyGroupLanguage`, then in a group menu `p.tr = i18n.MustNewTranslator(code)` before the redraw with the callback's own `language_changed_group` as the success notice, in a private menu the opener's translator kept). `settings_main_test.go`: the Main-shape constants become `... / m|d / t|u / a / close` and `... / groups|close` (still 8 owner rows). AGENTS.md: the Callbacks bullet lists page `u`, a Language page bullet, the editor bullet's Main order sentence.
- **Private menu, edge cases, docs (commits 92ddee2, e2e4873).** `TestSettingsLanguagePrivate`, `TestSettingsLanguageSameAndUnknown`, `TestSettingsLanguageForged`; README sentence, `callbacks.md` (page `u`, control `l`, `v`, largest button 46/42), `permissions.md` row, `18-SPIKE.md` checks 7 to 10 (two replicas within 10 s, a private menu keeping the opener's language, the eight cells on narrow phones in hi, ru and uk, the flag width of `Bahasa Indonesia`).
- **Proofs (commit 14f211a).** `TestSettingsParityLanguage`, `TestSettingsDriftLanguage`, `TestSettingsAuthorityParityLanguage`, `TestSettingsLanguageReadsCreateNothing`, `TestSettingsLanguageCallbackBytes`, `TestSettingsLanguagePageFitsEveryLocale`.

## Verification (real results)

- Task 1 verify: `go test -tags testtools -race -count=1 -run '^(TestLegacyLanguageRequests|TestApplyGroupLanguage|TestLanguageCallback.*)$' -v ./fuku/modules`: all `--- PASS` (the golden's 21 subtests, the five `TestLanguageCallback*` tests, the five subtests of `TestApplyGroupLanguage`); `go test ... -run '^TestLoadGroupLanguageContext$' -v ./fuku/db/lang`: four subtests `--- PASS`, `ok`. `git log --reverse --format=%s 29aca33..HEAD -- fuku/modules/language_legacy_request_test.go fuku/modules/language.go | head -n 1` prints `test(18-07): pin the language callback's requests before the extraction`. The six acceptance greps (`TestLegacyLanguageRequests`, `TestApplyGroupLanguage`, `TestLoadGroupLanguageContext`, `applyGroupLanguage`, `LoadGroupLanguageContext`) each print `1`.
- Task 2 verify: `-run '^(TestSettingsLanguageGroupRedraw|TestLegacyLanguageRequests|TestSettingsMainGrid|TestSettingsDeletionOpensFromMain|TestSettingsDeletionRowBudget|TestSettingsEditRegistry|TestSettingsMreqOpensFromMain|TestSettingsControlActionAllowList)$' -v`: eight `--- PASS`, `ok ... 7.780s`. Acceptance greps: `func TestSettingsLanguageGroupRedraw(` prints 1; `registerSettingsPage` in non-comment lines of `settings_language.go` prints 1; `ChangeUserLanguage` in non-comment lines prints 0. A mutation check removing the translator swap made `TestSettingsLanguageGroupRedraw` fail on the Spanish header, current line, hint, notice, English hint and navigation row; restored (no diff left).
- Task 3 verify: `-run '^(TestSettingsLanguagePrivate|TestSettingsLanguageSameAndUnknown|TestSettingsLanguageForged|TestSettingsLanguageGroupRedraw)$' -v`: all `--- PASS` (five subtests of SameAndUnknown, eleven of Forged), `ok ... 13.590s`; `make check-docs`: "No drift detected". The permissions acceptance exits 0 and the three test-name greps print `1`.
- Task 4 verify: `-run '^(TestSettingsParityLanguage|TestSettingsDriftLanguage|TestSettingsAuthorityParityLanguage|TestSettingsLanguageReadsCreateNothing|TestSettingsLanguageCallbackBytes|TestSettingsLanguagePageFitsEveryLocale)$' -v`: six `--- PASS`, `ok ... 16.460s`. The largest button is 46 bytes (private) and 42 (group) at the worst-case IDs, `Omitted` 0, every cell at most 20 code points, keyboards `{2,2,2,2,2}` in all 8 languages with ampersand, astral and multibyte 64-rune titles and every success notice.
- Whole `fuku/modules` package, `go test -tags testtools -race -count=1 -timeout 30m -v ./fuku/modules` (background, log `modules-18-07.log` in the scratchpad): `ok github.com/uasneppy/fuku_robot/fuku/modules 1649.178s`, `exit 0`, `grep -cE -- '--- (FAIL|SKIP)|^FAIL'` = 0, 1712 top-level `--- PASS` lines, 298 of them `TestSettings*` (the settings family runs inside it, so it was not run separately); `go test ... -run '^(TestSettings|TestLoadSettings)' ./fuku/i18n`: `ok 7.146s`; `make check-translations`: all translations present.
- `go vet -tags testtools ./fuku/modules ./fuku/db/lang` clean; `CGO_ENABLED=0 go build ./...` ok; `gofmt -l` on the nine plan Go files prints nothing; `python3 .planning/phases/13-member-requirements/handoff/lint-local.py 8c0c827` exit 0 ("no dupl, gocyclo or godox findings"), read directly; `git diff --exit-code 8c0c827 -- go.mod go.sum` exit 0; `git diff --exit-code 29aca33 -- fuku/modules/settings_pages.go fuku/db/cache/local.go migrations fuku/db/models main.go` exit 0.
- Not run: `make lint` (the installed golangci-lint is built with Go 1.25, a documented sandbox fact), gosec, `make test`, and the live checks of `18-SPIKE.md` 7 to 10 (owner steps).

## Deviations from Plan

### Auto-fixed Issues

None needing a production fix. Notes:

1. **[Test fixture] The test process has no enabled languages.** `config.AppConfig.ValidLangCodes` is empty under test, so `/lang`'s keyboard in the golden holds only the Help Us Translate button; the golden records that as it is (it pins the callback and the display shape, not the language list), and the page tests set the eight default languages with `settingsLangWithCodes`, restored in `t.Cleanup`.
2. **[Test design] The `chat` cache key is proved through its reader.** `chat` holds a typed entry that `chats.GetChatSettings` reads itself, so a string marker in it is not valid; `TestApplyGroupLanguage` and the parity test prove it dropped by reading `chats.GetChatSettings(...).Language` after the write, and use string markers for `chat_lang` and `chat_settings` (nothing reads `chat_settings` today).
3. **[Test scope] `TestSettingsLanguageReadsCreateNothing` allows the keys Main's other pages make.** Opening Main reads every page's state, which makes their own cache keys; the test asserts no `chats` row, no `chat` or `chat_settings` entry and that the `chat_lang` entry exists (the page reads the key the bot reads).
4. **Earlier-plan tests edited:** only `settings_main_test.go`, the two Main-shape constants (`t` to `t|u`), as the plan says; no other earlier test changed and no golden was re-recorded.

**Total deviations:** 0 auto-fixed, 4 notes. **Impact:** none on the contract.

## Known Stubs

None. The control writes through the callback's apply function and the reader reads the real table.

## Self-Check: PASSED

- Created files exist on disk (the six created files above) and the commits are ancestors of HEAD: 8d8aa49, cd87587, 1b4e39a, 92ddee2, e2e4873, 14f211a.
- Every task's acceptance criteria were re-run and print the required values (see Verification).
