---
phase: 19-blacklist-filter-and-note-pages
plan: 02
subsystem: settings-menu-framework
tags: [telegram, settings-menu, framework, private-only, card, rights]

requires:
  - phase: 19-blacklist-filter-and-note-pages
    provides: 19-01's blacklist apply functions and fresh readers, which the ninth right's blacklist commands now have behind them
provides:
  - settingsPage.PrivateOnly and the private argument of settingsMainEntries (M39), with renderSettingsPrivateOnlyNote and the group note
  - settingsCardRowsTo (M40), which settingsCardRows now delegates to
  - the ninth right settingsRightRestrictBotRestrict, and the shared bot halves settingsRightBotFirst and settingsRightBotAfter(typed) (M42, U-9)
  - settings_state_private_only and settings_entries_group_note in 8 locales (387 settings_ keys)
affects: [19-03, 19-04, 19-05, 19-06, 19-12]

plan_head_before: 0ce13bf932738d7512afd0ea7e755fac8cab48a2
plan_head_after: 6f70bb7

actuals:
  tokens: 7500    # chars/4 over the added lines of 0ce13bf..6f70bb7 outside .planning (30105 chars)
  tasks: 3
  commits: 2      # MEASURED: git rev-list --count 0ce13bf..6f70bb7 (before this SUMMARY commit); task 3 changed nothing

tech-stack:
  added: []
  patterns:
    - "a framework hook proved through a page the test registers (counting MainState and Open) and removes in t.Cleanup"
    - "bot-first / member / bot-after: one shared pair of bot halves for every right, the press and the door"

key-files:
  created:
    - fuku/modules/settings_list_framework_test.go
  modified:
    - fuku/modules/settings_pages.go
    - fuku/modules/settings_menu.go
    - fuku/modules/settings_card.go
    - fuku/modules/settings.go
    - fuku/modules/settings_main_test.go
    - fuku/modules/settings_phase18_test.go
    - fuku/modules/settings_menu_test.go
    - fuku/modules/settings_edit_hooks_test.go
    - fuku/i18n/settings_locale_test.go
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
  - "The note renderer renderSettingsPrivateOnlyNote sits in settings_menu.go beside openPage, so the only edit to settings_pages.go is M39's field and argument."
  - "The PrivateOnly branch of openPage comes after the OwnerOnly and Hidden guards and before Open, so no route (open, Back, a forged Refresh) reaches page code from a group menu."
  - "settingsRightBotAfter(typed) picks chat_status_bot_restrict_group_error for a typed reply and chat_status_bot_restrict_error for a press (U-9); the door of 19-04 is the first typed caller."

patterns-established:
  - "registerPrivateOnlyProbe: a test page with counting MainState and Open, registered and deleted from settingsPages in t.Cleanup"

requirements-completed: []

coverage:
  - id: D1
    description: "a PrivateOnly page in a group menu shows the fixed state and the note with no page code running; a private menu calls MainState and Open"
    requirement: "PAGE-07 (contribution only)"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^TestSettingsPrivateOnlyPage$' ./fuku/modules (bare and with cienv.sh)"
        status: pass
    human_judgment: false
  - id: D2
    description: "settingsCardRowsTo draws the caller's No, and every existing card draws what it drew"
    requirement: "PAGE-07 (contribution only)"
    verification:
      - kind: unit
        ref: "TestSettingsCardRowsTo, TestSettingsStaffUnlink, TestSettingsLockdownLift, TestSettingsLockdownLiftExpired, TestSettingsMreqCard, TestSettingsAccessCard, TestSettingsDeletionMemberCard"
        status: pass
    human_judgment: false
  - id: D3
    description: "the ninth right judges /addblacklist's order and every older right answers as before"
    requirement: "PAGE-10 (contribution only)"
    verification:
      - kind: unit
        ref: "TestSettingsRights (mirrors /addblacklist), TestSettingsRightRefusalForParity, TestSettingsRightBotHalves, TestSettingsGateAllDoors, the Phase 11/12 door tests, TestSettingsAuthorityParitySwitches, TestSettingsAuthorityParityAnonAdmin"
        status: pass
    human_judgment: false
  - id: D4
    description: "two strings in 8 locales, 387 settings_ keys, under their ceilings"
    requirement: "PLAT-05 (contribution only)"
    verification:
      - kind: unit
        ref: "go test ./fuku/i18n (TestSettingsLocaleKeys, TestSettingsLabelLengths) and make check-translations"
        status: pass
    human_judgment: false
---

# Phase 19 Plan 02: the framework pieces the list pages need Summary

**A settings page can now be kept out of group menus entirely (`PrivateOnly`), the two-tap card takes the caller's No (`settingsCardRowsTo`), and a ninth right judges what `/addblacklist` judges, with the bot halves shared and every older right answering exactly as before.**

## What was built

- **Task 1, `PrivateOnly` (M39)**, commit `747055f`. `settingsPage.PrivateOnly bool`; `settingsMainEntries(b, tr, group, viewerIsOwner, private bool)` reads the fixed word `settings_state_private_only` for such a page in a group menu without calling `MainState`; `openPage` edits a group menu into `renderSettingsPrivateOnlyNote` (header, `settings_entries_group_note`, the Open in private row, Back and Close) without calling `Open`. The four call sites pass the menu kind (`settings.go`, `settingsMainScreen`, and the two tests, which pass `true`). Two strings added to all 8 locales with limits rows (`{16,20}`, `{140,200}`) and the label-length check (at most 20). No page is registered, so Main still has 15 cells.
- **Task 2, card and ninth right (M40, M42)**, commit `6f70bb7`. `settingsCardRowsTo(k, yes, no)`; `settingsCardRows` delegates with `{a: no, p: page}`. `settingsRightRestrictBotRestrict` appended after `settingsRightOwnerBotAdmin`; `settingsRightBotFirst` (the two composites' checks unchanged, plus `IsUserAdmin` then `IsBotAdmin` for the new right) and `settingsRightBotAfter(typed)` (the `/captcha` and `/setflood` after-checks moved unchanged, plus `CanBotRestrict` with the press and typed keys). `settingsRightRefusal` is bot first, member part, bot after. The old press-only helper is gone (`grep -c BotFirstRefusal fuku/modules/*.go` finds no file). The member part of the new right is `settingsRightRestrict`'s.
- **Task 3**: re-read the AGENTS.md bullets against the code (no drift, no edit, no commit) and ran the gates below.
- AGENTS.md (in the task commits): a `PrivateOnly` bullet, the ninth right's Permissions bullet with the bot halves, the card sentence naming `settingsCardRowsTo`, and the 387 key count.

## Verification (real results)

- `go test -tags testtools -race -count=1 -run '^(TestSettingsPrivateOnlyPage|TestSettingsMainGrid|TestSettingsMainPhase18|TestSettingsPhase18Codes|TestSettingsMainPageFitsEveryLocale|TestSettingsDeletionOpensFromMain|TestSettingsMreqOpensFromMain|TestSettingsEditRegistry)$' ./fuku/modules`: pass; `./fuku/i18n` (`TestSettingsLocaleKeys`, `TestSettingsLabelLengths`): pass; `make check-translations`: "All translations are present". Focused runs also pass under the CI environment (`cienv.sh`).
- `go test ... -run '^(TestSettingsCardRowsTo|TestSettingsRightBotHalves|TestSettingsRights|TestSettingsRightRefusalForParity|TestSettingsStaffUnlink|TestSettingsLockdownLift|TestSettingsLockdownLiftExpired|TestSettingsMreqCard|TestSettingsAccessCard|TestSettingsDeletionMemberCard|TestSettingsAuthorityParitySwitches|TestSettingsAuthorityParityAnonAdmin)$'`: pass bare; the four new or extended ones also pass with `cienv.sh`.
- Door and rights block of Task 3 (every `TestSettingsEditTyped*`, `Door*`, `Album*`, `RulesTyped*`, `TestSettingsInput*`, `TestSettingsRights`, `TestSettingsRightRefusalForParity`, `TestSettingsMainGrid`, plus `TestSettingsGateAllDoors`): `ok ... 40.572s`. `git diff --exit-code d93ec1d -- fuku/modules/settings_input_door.go settings_input.go settings_input_door_test.go settings_input_media_test.go settings_input_rules_test.go settings_input_test.go` exits 0: no door file and no Phase 11/12 door test changed.
- Whole `fuku/modules` package under the CI environment (`go test -tags testtools -race -count=1 -timeout 40m -v ./fuku/modules`, in the background, log `/tmp/claude-0/-home-user-Fuku-Robot/4ce545b5-6832-57a3-a863-43c972932c7c/scratchpad/modules-19-02.log`): `ok  github.com/uasneppy/fuku_robot/fuku/modules  1780.323s`, `exit 0`, 1768 top-level PASS (19-01's 1765 plus the 3 new tests), 0 FAIL, 0 SKIP. `--- PASS: TestSettingsPrivateOnlyPage (0.96s)`, `TestSettingsCardRowsTo (0.01s)`, `TestSettingsRightBotHalves (0.48s)`, `TestSettingsRights (1.51s)`.
- Settings family under the CI environment (`-run '^(TestSettings|TestLoadSettings)' ./fuku/modules ./fuku/i18n`): `ok fuku/modules 715.336s`, `ok fuku/i18n 8.104s`, `exit 0`. The plan's command has no `-timeout`, and the family needs more than Go's default 10 minutes, so the first attempt was killed by `panic: test timed out after 10m0s` (a timing fact of the command, not a failure of a test); the rerun with `-timeout 40m` is the result above.
- Gates: `go vet -tags testtools ./fuku/modules ./fuku/i18n` clean; `CGO_ENABLED=0 go build ./...` clean; `gofmt -l` over the Go files this plan created or edited prints nothing; `lint-local.py 8c0c827` exits 0 ("no dupl, gocyclo or godox findings"); `git diff --exit-code 8c0c827 -- go.mod go.sum` exits 0; no `t.Skip` in the new tests.
- Acceptance greps: `PrivateOnly bool` 1; `func renderSettingsPrivateOnlyNote(` 1; Main shape constants unchanged (15 cells, `settingsMainOwnerRows = 10`); each of the 8 locale files has 387 `settings_` lines; `func settingsCardRowsTo(` 1 and `settingsCardRowsTo(` 2 occurrences in `settings_card.go` (the declaration and its caller `settingsCardRows`; the plan's "at least 2"); `settingsRightRestrictBotRestrict` appears in `settings_menu.go` 5 times; `settingsRightBotFirst` and `settingsRightBotAfter` are each declared once.
- Not run: `make lint` and gosec (cannot run in the sandbox), `make test` (only the closing plan runs it), PostgreSQL (no database change in this plan).

## Time added to the fuku/modules run

About 2.5 s: `TestSettingsPrivateOnlyPage` 0.96 s, `TestSettingsRightBotHalves` 0.48 s, the "mirrors /addblacklist" subtest about 0.9 s of `TestSettingsRights` (1.51 s whole), `TestSettingsCardRowsTo` 0.01 s, the extra parity rows 0 s. No new test is over 10 s. The whole-package run measured 1780 s here (19-01's was 1730 s on the same machine, so the difference is noise between runs, not these tests).

## Deviations from Plan

- **Acceptance count** (not a deviation): the plan says `settingsRightRestrictBotRestrict` appears "at least 3" times in `settings_menu.go`; it appears 5 times (declaration, member part, the two bot halves, one comment), so the plan's "at least 3" holds; no deviation. (Recorded here only because the count was first written wrong.)
- **Verify command**: the settings-family command of Task 3 needs `-timeout 40m` (it takes 715 s here); noted above. The plan's other commands ran as written.
- **Test of the bot-first order**: the plan asks for a case where the bot is an administrator by the list but not by `IsBotAdmin` (notice `chat_status_bot_not_admin`). I did not find a way to build it with the existing fake (both predicates read the one bot role it answers with) and did not change the fake. It is proved instead where the two predicates read different sources already: `TestSettingsRightBotHalves` checks `settingsRightBotFirst` and `settingsRightBotAfter` for all nine rights in three bot states, and the `/captcha` and `/anonadmin` subtests of `TestSettingsRights` (unchanged) cover the `IsBotAdmin`-keyed refusal. The `IsBotAdmin` step of the ninth right's first half is therefore proved by reading the code and by the member case (both fail together), not by an isolated case.
- No other deviation: no golden was touched, no door file or Phase 11/12 test was edited, no migration, Redis key, handler group or shutdown change.

## Notes for later plans

- 19-03 is the first page to declare `PrivateOnly: true` and `settingsRightRestrictBotRestrict`; 19-04's door is the first typed caller of `settingsRightBotAfter(..., true)` and must classify a cached false with one live lookup, as the GoDocs say.
- `requirements-completed: []`: this plan contributes to PAGE-07, PAGE-10 and PLAT-05 and completes none; only 19-12 completes PAGE-07 to PAGE-09.

## Known Stubs

None.

## Threat Flags

None: no endpoint, auth path or trust boundary was added; T-19-06 to T-19-09 are mitigated by the tests above.

## Self-Check: PASSED

- Files exist: `fuku/modules/settings_list_framework_test.go`; the two new keys are in all 8 locale files.
- Commits are ancestors of HEAD: `747055f`, `6f70bb7`.
