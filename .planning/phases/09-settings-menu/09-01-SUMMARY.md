---
phase: 09-settings-menu
plan: 01
subsystem: telegram-ui
tags: [settings-menu, callbacks, captcha, i18n, gotgbot]

requires:
  - phase: 08-web-captcha
    provides: captcha_settings getters and setters, the lockdown live-member helpers and the staff fixtures the menu reuses
provides:
  - "/settings command with a live-checked choice message (Open here, Open in private)"
  - "settings|v1 callback namespace with the full press pipeline (structure, service identity, opener, live admin check, per-control right, answer once, edit in place)"
  - "page registry (registerSettingsPage) and shared renderer; Main page and Captcha page with the time-limit stepper"
  - "applyCaptchaTimeout, the one write path shared by /captchatime and the menu"
  - "23 settings_ locale keys in 7 languages and the locale parity test"
  - "shared settings test toolkit (settings_env_test.go)"
affects: [09-02, 09-03, 09-04, 09-05, 09-06, 09-07, 09-08, 09-09, 09-10, 09-11, 09-12, 09-13, 09-14]

plan_head_before: 5df191f8683da0eca1c46c354afc11ad23532f77
plan_head_after: a726978fb38b1bd0cd505dce465e9d73d043dafa

actuals:
  tokens: 37000
  tasks: 3
  commits: 4

tech-stack:
  added: []
  patterns:
    - "page registry: a page registers itself from its own init with registerSettingsPage; the renderer, pipeline and keyboard helpers never change"
    - "stateless menu: owner field (c or u) in every button, absolute target values for steppers and toggles, no Redis key"
    - "one setting, one code path: apply function beside the command returning a typed result (captchaApplyResult)"

key-files:
  created:
    - alita/modules/settings.go
    - alita/modules/settings_menu.go
    - alita/modules/settings_pages.go
    - alita/modules/settings_captcha.go
    - alita/modules/settings_env_test.go
    - alita/modules/settings_test.go
    - alita/modules/settings_menu_test.go
    - alita/i18n/settings_locale_test.go
  modified:
    - alita/modules/captcha.go
    - alita/i18n/test_helper.go
    - locales/en.yml
    - locales/es.yml
    - locales/fr.yml
    - locales/hi.yml
    - locales/id.yml
    - locales/pt.yml
    - locales/ru.yml
    - AGENTS.md

key-decisions:
  - "Page action field lists (settingsPageAction.Fields) are allowed, not required: a page action such as Link takes an optional s, and its Run parses its fields strictly"
  - "settingsChoiceRow, settingsToggleButton and settingsStepperRow all return a button slice (the toggle returns one button or none), so every caller writes k.row(helper(...)...)"
  - "A group menu's u and a private menu's c must be in canonical decimal form (no leading zero or plus sign) as well as positive/negative as required"
  - "A press on an inaccessible message (date 0) is answered like a missing message"
  - "Real-locale tests install the real locale files through a new testtools helper, i18n.OverrideManagerLanguagesForTest, because the test binary has no locale manager"

patterns-established:
  - "settingsPress.answer is the only way a settings press is answered; the dispatcher answers empty after a handler that did not answer"
  - "Rights are a pure function of the press (settingsRightRefusal) built from the commands' own predicates"

requirements-completed: [MENU-01, MENU-02, MENU-03, MENU-04, MENU-07, MENU-08]

coverage:
  - id: D1
    description: "/settings in a group, by a live creator or administrator, replies with the choice message (Open here, Open in private); a plain member, a failed lookup and a private chat are refused; anonymous admins get the proof first"
    requirement: "MENU-01"
    verification:
      - kind: unit
        ref: "alita/modules/settings_test.go#TestSettingsCommand"
        status: pass
      - kind: unit
        ref: "alita/modules/settings_menu_test.go#TestSettingsAnonymousAdmin"
        status: pass
    human_judgment: false
  - id: D2
    description: "Open here edits the choice message into the Main page; one message with Back and Close; every press re-checks the presser live and is answered once"
    requirement: "MENU-02, MENU-03, MENU-04"
    verification:
      - kind: unit
        ref: "alita/modules/settings_test.go#TestSettingsTracer"
        status: pass
      - kind: unit
        ref: "alita/modules/settings_menu_test.go#TestSettingsPressPipeline"
        status: pass
      - kind: unit
        ref: "alita/modules/settings_menu_test.go#TestSettingsOwnership"
        status: pass
      - kind: unit
        ref: "alita/modules/settings_menu_test.go#TestSettingsNavigation"
        status: pass
    human_judgment: false
  - id: D3
    description: "Captcha page time-limit stepper writes through applyCaptchaTimeout; the command and the menu agree in both directions, bounds and forged values are handled"
    requirement: "MENU-07, MENU-08"
    verification:
      - kind: unit
        ref: "alita/modules/settings_test.go#TestSettingsParityCaptchaTime"
        status: pass
      - kind: unit
        ref: "alita/modules/settings_test.go#TestSettingsDriftCaptchaTime"
        status: pass
    human_judgment: false
  - id: D4
    description: "Per-control rights mirror the matching commands (captcha on/off, antiflood, lockdown restrict)"
    requirement: "MENU-03"
    verification:
      - kind: unit
        ref: "alita/modules/settings_menu_test.go#TestSettingsRights"
        status: pass
    human_judgment: false
  - id: D5
    description: "Callback data fits 64 bytes in both menu variants; locale keys exist in all 7 languages within their ceilings"
    verification:
      - kind: unit
        ref: "alita/modules/settings_menu_test.go#TestSettingsCallbackBytes"
        status: pass
      - kind: unit
        ref: "alita/i18n/settings_locale_test.go#TestSettingsLocaleKeys"
        status: pass
    human_judgment: false
  - id: D6
    description: "How the menu reads and behaves on real Telegram clients (button truncation in hi and ru, alert rendering, a URL button into /start for a user who never started the bot)"
    verification: []
    human_judgment: true
    rationale: "Client rendering cannot be asserted by a fake; it is covered by the deferred real-device checklist 09-SPIKE.md"

duration: 25min
completed: 2026-10-30
status: complete
---

# Phase 9 Plan 01: Settings menu tracer Summary

**An admin sends /settings, taps Open here, opens the Captcha page and steps the time limit; the stored value is written by applyCaptchaTimeout, the same function /captchatime calls, and every press passes a five-step live-checked pipeline.**

## Performance

- **Duration:** about 25 min
- **Tasks:** 3 of 3
- **Files:** 18 changed (8 created, 10 modified), 3253 insertions
- **Commits:** 4 (see below)

## Accomplishments

- `/settings` (`settings.go`): `helpers.WrapCommand` with `requireLockdownGroup()` and `requireLockdownAuthority(false)`, not `Disableable`, anonymous-admin re-entry; the reply carries Open here (`a=oh&u=<sender>`) and an Open in private URL button `https://t.me/<bot>?start=settings_<chat id>`.
- Press pipeline (`settings_menu.go`, `settingsVerifyPress`): decode and field allow-list with the c/u structural rule (c only on a private message, u only on a non-private one), service identity, opener check, one live `getChatMember` of the presser in the configured group (and one `getChat` for a private menu), then the control's `settingsRight`. The four rights use the commands' own predicates. `settingsPress.answer` answers once; edits treat "message is not modified" as success and never retry.
- Page registry and renderer (`settings_pages.go`): `registerSettingsPage` (panics at init on a duplicate page key or action code), `settingsCompose`, `settingsKeyboard`, `settingsStepperRow`, `settingsChoiceRow`, `settingsToggleButton`, `renderSettingsMain`. A button whose data does not fit 64 bytes is counted and logged, never silent.
- Captcha page (`settings_captcha.go`): the time-limit stepper writes through `applyCaptchaTimeout` (new in `captcha.go`, `captchaTimeCommand` now calls it with unchanged replies); a bound step answers a toast and writes nothing; an unreadable page is the cannot-open edit.
- 23 `settings_` locale keys in all 7 languages, `TestSettingsLocaleKeys` (ceilings, placeholders, tags, alert length) and the AGENTS.md rules.

## Task Commits

1. RED tests (toolkit, tracer, parity, drift, command): `faabfe4` test(09-01)
2. Task 1 GREEN (command, pipeline, registry, Captcha page, apply function, 23 locale keys): `9582256` feat(09-01)
3. Task 2 pipeline tests: `c2f6e25` test(09-01)
4. Task 3 locale parity test and AGENTS.md: `a726978` test(09-01)

## Verification

- `CGO_ENABLED=0 go build ./...`: ok
- `go test -tags testtools -race -count=1 -v -run '^TestSettings'` on `./alita/modules` (14 top-level tests, all PASS) and `TestSettingsLocaleKeys` / `TestLockdownLocaleKeys` on `./alita/i18n` (all 7 language subtests PASS)
- The five captcha pinning tests (`TestCaptchaSubcommandsCreateSettingsWithDefaults`, `...RequireArgumentsOrShowCurrentSettings`, `...RejectInvalidValues`, `TestCaptchaCommandsPropagateGotgbotRequestErrors`, `TestLoadCaptchaRegistersHelpAndHandlers`): PASS. Also `-run 'Help|Anonymous|Captcha|Antiflood|Registry|Registered|LoadAllModules'` on `./alita/modules` and the whole `./alita/i18n` and `./alita/db/captcha` packages: PASS.
- `make check-translations`: all translations present. `make check-docs`: no drift.
- `go vet -tags testtools ./alita/modules/ ./alita/i18n/` and `gofmt -l alita/`: clean. `make lint` was not run: golangci-lint is built with go1.25 and cannot lint this go1.26.0 module in the sandbox, so `go vet` and `gofmt` stand in. The whole `make test` was not run (the orchestrator runs it between waves).
- Mutation check of the security ordering: disabling the opener check, or the admin-status check, in `settingsVerifyPress` made `TestSettingsOwnership` and `TestSettingsPressPipeline` fail, and the code was restored.
- Acceptance greps: `applyCaptchaTimeout` defined in `captcha.go` and called from `settings_captcha.go`; `RegisterLegacyModule("Settings", 239, LoadSettings)`, `RegisterAnonymousAdminHandler("settings"` and `requireLockdownAuthority(false)` present in `settings.go`; `grep -c "^settings_"` prints 23 for each of the 7 locale files; `query.Answer(` appears in none of the four settings files; AGENTS.md has `settings|v1`, `settingsRightAdminFloodBot`, `applyCaptchaTimeout` and `registerSettingsPage`.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Real-locale tests need a locale manager the test binary does not have**
- **Found during:** Task 1 (the "command sees the menu's change" and `TestSettingsGroupLanguage` tests with `RealLocale: true`)
- **Issue:** the plan assumes `i18n.MustNewTranslator("es")` returns the real Spanish text in a module test, but no locale manager is initialised in the `alita/modules` test binary (`locale manager not initialized`), and `OverrideManagerForTest` only installs one English string set.
- **Fix:** added `OverrideManagerLanguagesForTest(yamlByLang)` (testtools build tag) to `alita/i18n/test_helper.go`; `newSettingsEnvWith(RealLocale)` installs the seven real locale files through it.
- **Files modified:** `alita/i18n/test_helper.go` (not in the plan's `files_modified`), `alita/modules/settings_env_test.go`
- **Commit:** `9582256`

**2. [Rule 1 - Bug in the plan's test] "/captcha shows 3" cannot be asserted with the marker locale**
- **Found during:** Task 1
- **Issue:** `captcha_settings_timeout` is `Timeout: <code>%d minutes</code>`; the marker locale keeps only `{name}` placeholders, so the number is lost and "marker followed by 3" can never hold.
- **Fix:** that one subtest runs on the real locale and compares the reply with the real translator's own `captcha_settings_timeout` text for 3.
- **Commit:** `faabfe4`, `9582256`

**3. [Plan ordering] Task 2's tests were already green when written**
- The tracer commit implemented the whole press pipeline (it cannot be split from the tracer), so the nine Task 2 tests passed on first run and there was no RED step for them. They were not weakened: a mutation check (above) shows they fail when the opener or admin check is removed. The Task 2 commit contains the tests only, no code change.

**4. [Rule 3 - Environment] Plan-commit ledger file not written**
- The harness refused the compound git commands and a write into the shared `.git` path, so the `gsd-plan-head-before-09-01` ledger was not stored. `commits` and the two head SHAs above were measured directly with `git rev-list --count 5df191f..HEAD` (4, taken before the SUMMARY commit).

**Total deviations:** 4 (1 blocking test-infrastructure helper outside `files_modified`, 1 plan test corrected, 2 process notes). **Impact:** none on behaviour; the extra file is a testtools-only helper.

## Known Stubs

None. Not wired by design in this plan, and tracked by later plans: the Main page lists only the Captcha page (Staff Group, Lockdown, Antiflood register in 09-09, 09-10, 09-11); the Captcha page has only the time limit (on/off, mode, action in 09-08); the Open in private button is final but its `/start settings_<id>` handler arrives in 09-07 (until then such a `/start` gets the default start reply); `settings_help_msg`, `alt_names.Settings` and `SetModuleEnabled` come with the docs in 09-13, so `/help` does not list the module yet. `settings_start_not_admin` and `settings_start_invalid` exist in all 7 locales but are read only by 09-07.

## Threat Flags

None beyond the plan's threat model (T-09-01 to T-09-11 are covered by the tests named in the register). No new endpoint, schema or secret.

## Accepted limits (recorded)

- A group menu and its choice message stay until used or closed; there is no auto-close timer.
- The "Only {name} can use this menu." line uses an escaped name, not a mention, so it pings nobody.
- `captcha_settings` is not in `skipLocal`: another replica can show an old time limit for up to `CACHE_LOCAL_TTL` (10 s), as the commands can.

## Pending human verification (deferred to 09-SPIKE.md, not blocking)

On a real bot and group, as a group admin: send `/settings`; tap Open here (the same message becomes the menu); open Captcha and tap -1 and +1 (the stored value changes, `/captcha` shows it); tap Close (the message disappears); tap Open in private (needs plan 09-07 for the menu itself). Check that button labels are not cut in hi and ru, and that alerts render on iOS, Android and Desktop.

## Self-Check: PASSED

- Created files exist: `alita/modules/settings.go`, `settings_menu.go`, `settings_pages.go`, `settings_captcha.go`, `settings_env_test.go`, `settings_test.go`, `settings_menu_test.go`, `alita/i18n/settings_locale_test.go` (checked on disk).
- Commits present: `faabfe4`, `9582256`, `c2f6e25`, `a726978` (`git log` on this branch).
- All three tasks' `<verify>` commands and acceptance criteria re-run and green (see Verification).
