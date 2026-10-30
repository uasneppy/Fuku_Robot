---
phase: "9"
slug: "settings-menu"
# status lifecycle: draft (seeded by plan-phase) → validated (set by validate-phase §6)
# audit-milestone §5.5 distinguishes NOT-VALIDATED (draft) from PARTIAL (validated + nyquist_compliant: false) (#2117)
status: draft
nyquist_compliant: false
wave_0_complete: false
created: "2026-10-30"
---

# Phase 9 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go `testing` with build tag `testtools`, testify assertions only; real fixtures: the module harness and `internal/testdb` (SQLite), miniredis, the lockdown env dispatcher, hand-written `gotgbot.BotClient` fakes (`moduleBotClient`, `lockdownFake`, `staffActionFake`). No mock libraries. |
| **Config file** | none; `-tags testtools` is mandatory (AGENTS.md). Harness: `alita/modules/test_harness_test.go` |
| **Quick run command** | `go test -tags testtools -race -count=1 -run '^TestSettings' ./alita/modules ./alita/i18n` |
| **Full suite command** | `make test` (then `make check-translations`, `make check-docs`, `CGO_ENABLED=0 go build ./...`) |
| **Estimated runtime** | ~60 seconds quick, ~6 minutes full |

---

## Sampling Rate

- **After every task commit:** Run the quick run command for the files touched
- **After every plan wave:** `go test -tags testtools -race -count=1 ./alita/modules ./alita/i18n ./alita/db/antiflood ./alita/db/captcha` plus `go vet -tags testtools ./...` and `gofmt -l` (golangci-lint cannot lint Go 1.26 in this sandbox); the orchestrator runs `make test` between waves
- **Before `/gsd-verify-work`:** `make test`, `make check-translations`, `make check-docs` and `CGO_ENABLED=0 go build ./...` green
- **Max feedback latency:** 90 seconds
- No new PostgreSQL guarantee (no migration, index, CHECK or conditional update): nothing to add to `make test-postgres-integrity`, and PostgreSQL is never started locally

---

## Per-Task Verification Map

One row per plan task (30 tasks in 14 plans). Every command below is run from `/home/user/Fuku_Robot` as `CGO_ENABLED=1 go test -tags testtools -race -count=1 -v -run '<pattern>' <package>` unless it is written out in full; the exact command and its failure signal are in the task's `<verify>` / `<fails_when>`. The Nyquist audit fills the status.

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 09-01-01 | 01 | 1 | MENU-01, MENU-04, MENU-07, MENU-08 | T-09-07, T-09-11 | `/settings` (live admin, not the cache) replies with Open here and the deep-link URL; Open here edits the choice into Main; the Captcha time stepper writes through `applyCaptchaTimeout`, the function `/captchatime` calls; parity and drift (0, 1, 10, 11); Back and Close with the edit fallback | integration (dispatcher) | `^(TestSettingsTracer\|TestSettingsParityCaptchaTime\|TestSettingsDriftCaptchaTime\|TestSettingsCommand)$` plus the five captcha pinning tests, `./alita/modules` | ❌ W0 (new settings_test.go, settings_env_test.go) | ⬜ pending |
| 09-01-02 | 01 | 1 | MENU-02, MENU-03, MENU-04 | T-09-01..06, T-09-08..10 | Press order (structure with no I/O, service identity, opener, live check, right, one answer); forwarded menus refused; the four rights refuse what their commands refuse; 64-byte worst case with no omission; anonymous-admin re-entry; group language; read failure is the cannot-open edit | integration, pure render | `^(TestSettingsOwnership\|TestSettingsPressPipeline\|TestSettingsRights\|TestSettingsNavigation\|TestSettingsCallbackBytes\|TestSettingsKeyboardHelpers\|TestSettingsAnonymousAdmin\|TestSettingsGroupLanguage\|TestSettingsCaptchaReadFailure\|TestSettingsTracer)$`, `./alita/modules` | ❌ W0 (new settings_menu_test.go) | ⬜ pending |
| 09-01-03 | 01 | 1 | MENU-01..04 | T-09-07 | The 23 tracer keys in all 7 locales, ceilings, identical placeholders, only `b`/`code`, alerts without markup and at most 200 characters; AGENTS.md rules | unit | `^(TestSettingsLocaleKeys\|TestLockdownLocaleKeys)$`, `./alita/i18n` then `make check-translations` | ❌ W0 (new settings_locale_test.go) | ⬜ pending |
| 09-02-01 | 02 | 1 | MENU-05 | T-09-13 | `renderStaffRow` = head + status, the status never names a group; the panel text honours a length limit; `/staff` output unchanged | unit | `^(TestStaffRowSplitMatchesRow\|TestStaffPanelTextLimit\|TestStaffPanelRender...)$` (12 tests), `./alita/modules` | ❌ W0 (new staff_seams_test.go) | ⬜ pending |
| 09-02-02 | 02 | 1 | MENU-05 | T-09-12 | `listVerifiedOwnStaffGroups`, `linkToStaffGroup`, `announceStaffLink` split out with every link refusal and post unchanged | integration | `^(TestListVerifiedOwnStaffGroups\|TestLinkToStaffGroup\|TestAnnounceStaffLink\|TestLinkStaff...\|TestStaffPicker...\|TestUnlinkStaff...)$` (20 tests), `./alita/modules` | ✅ pinning files exist | ⬜ pending |
| 09-03-01 | 03 | 1 | MENU-07, MENU-08 | T-09-15 | Characterization: the stored limit after the first `SetFloodMode` / `SetFloodMsgDel` on a chat with no row | unit | `^(TestSetFloodModeOnFreshChatStoredLimit\|TestSetFloodMsgDelOnFreshChatStoredLimit\|TestSetFloodMode\|...)$`, `./alita/db/antiflood` | ✅ repository_test.go | ⬜ pending |
| 09-03-02 | 03 | 1 | MENU-07, MENU-08 | T-09-14 | `applyFloodLimit/Mode/Delete` with a database read-back; the commands keep every reply | integration | `^(TestApplyFloodLimit\|TestApplyFloodMode\|TestApplyFloodDelete\|TestAntifloodCommandsUseApply\|TestAntifloodCommands...)$`, `./alita/modules` | ❌ W0 (new antiflood_apply_test.go) | ⬜ pending |
| 09-04-01 | 04 | 1 | MENU-06, MENU-08 | T-09-18 | `applyAutoLockdown` (basic group, range, save failure, read-back) shared with `/autolockdown` | integration | `^(TestApplyAutoLockdown\|TestAutoLockdown...\|TestAutolockdownAI)$` (9 tests), `./alita/modules` | ❌ W0 (new lockdown_apply_test.go) | ⬜ pending |
| 09-04-02 | 04 | 1 | MENU-06, MENU-08 | T-09-16, T-09-17 | `applyLockdownLift` (restore then the one conditional record), `lockdownAnnounceLift` (public notice, worker wake) shared with the alert, `lockdownStatusHead` shared with `/lockdownstatus` | integration | `^(TestApplyLockdownLift\|TestLockdownAnnounceLift\|TestLockdownStatusHead\|TestLockdownLift...\|TestUnlockdown...\|TestLockdownAlertLift...\|TestLockdownStatusCommand\|TestLockdownAnonymousAdminProof)$`, `./alita/modules` | ❌ W0 (lockdown_apply_test.go) | ⬜ pending |
| 09-05-01 | 05 | 2 | MENU-04..07 | T-09-21 | The 56 page and card keys in English with their ceilings; reused Staff and lift texts carry only `{group}` | unit | `^TestSettingsLocaleKeys$/^en$` and `^TestSettingsReusedKeyPlaceholders$`, `./alita/i18n` | ✅ settings_locale_test.go (09-01) | ⬜ pending |
| 09-05-02 | 05 | 2 | MENU-04..07 | T-09-19, T-09-20 | Six translations within ceilings; composite labels at most 34/44 and choice buttons 13/15 | unit | `^(TestSettingsLocaleKeys\|TestSettingsLabelLengths\|TestSettingsReusedKeyPlaceholders\|TestLockdownLocaleKeys)$`, `./alita/i18n` then `make check-translations` | ✅ | ⬜ pending |
| 09-06-01 | 06 | 2 | MENU-07, MENU-08 | T-09-23 | `applyCaptchaEnabled` (off cleanup, partial failure) and `applyCaptchaAction` shared with `/captcha`, `/captchaaction` | integration | `^(TestApplyCaptchaEnabled\|TestApplyCaptchaAction\|TestCaptchaCommand...\|TestCaptchaSubcommands...\|TestCaptchaDisableFinalizesAttemptsAndRestoresUsers)$`, `./alita/modules` | ❌ W0 (new captcha_apply_test.go) | ⬜ pending |
| 09-06-02 | 06 | 2 | MENU-07, MENU-08 | T-09-22 | `applyCaptchaMode` with Turnstile's three ordered refusals; `captchaWebInstalled` | integration | `^(TestApplyCaptchaMode\|TestCaptchaWebInstalled\|TestCaptchaModeTurnstile...\|TestLoadCaptchaRegistersHelpAndHandlers)$`, `./alita/modules` | ❌ W0 (captcha_apply_test.go) | ⬜ pending |
| 09-07-01 | 07 | 2 | MENU-01, MENU-02 | T-09-24, T-09-25 | `/start settings_<id>`: strict parse, live admin check, bot missing and wrong chat type, Main page in private with `c`; `chat_status.IsBotMissingError` exported | integration, unit | `^TestIsBotMissingError$` in `./alita/utils/chat_status`; `^(TestSettingsStartDeepLink\|TestSettingsPrivateRoundTrip\|TestSettingsTracer\|TestHandleGroupDeepLinkUsesLongestPrefix)$`, `./alita/modules` | ❌ W0 (new settings_private_test.go) | ⬜ pending |
| 09-07-02 | 07 | 2 | MENU-02, MENU-03 | T-09-26, T-09-27 | A dead private menu gets the cannot-open edit, other lookup errors the check-failed alert; a private menu writes only to its `c` | integration | `./alita/utils/chat_status` (all); `^(TestSettingsPrivateMenu\|TestSettingsStartDeepLink\|TestSettingsOwnership\|TestSettingsPressPipeline)$`, `./alita/modules` | ❌ W0 (settings_private_test.go) | ⬜ pending |
| 09-08-01 | 08 | 3 | MENU-07, MENU-08 | T-09-29, T-09-30 | Captcha toggle, mode (with 🚫 Turnstile and the fallback note) and action through the commands' apply functions; off cleanup answered first | integration | `^(TestSettingsCaptchaPage\|TestSettingsCaptchaToggle\|TestSettingsCaptchaTurnstile\|TestSettingsTracer\|TestSettingsParityCaptchaTime)$`, `./alita/modules` | ❌ W0 (new settings_captcha_test.go) | ⬜ pending |
| 09-08-02 | 08 | 3 | MENU-03, MENU-07, MENU-08 | T-09-28 | Captcha parity, drift, authority parity (bot admin for the toggle only), 53-byte worst case, length in 7 locales | integration, pure render | `^(TestSettingsParityCaptcha\|TestSettingsDriftCaptcha\|TestSettingsAuthorityParityCaptcha\|TestSettingsCaptchaCallbackBytes\|TestSettingsCaptchaPageFitsEveryLocale\|...)$`, `./alita/modules` | ❌ W0 | ⬜ pending |
| 09-09-01 | 09 | 3 | MENU-07, MENU-08 | T-09-32, T-09-33 | Antiflood page: Off then 3..100 stepper, mute/ban/kick, delete switch, stored warn/tban/tmute read-only, fresh-chat read-back, read failure is cannot-open | integration | `^(TestSettingsAntifloodPage\|TestSettingsAntifloodStepper\|TestSettingsAntifloodFreshChat\|TestSettingsAntifloodStoredLegacyAction\|TestSettingsAntifloodReadFailure)$`, `./alita/modules` | ❌ W0 (new settings_antiflood_test.go) | ⬜ pending |
| 09-09-02 | 09 | 3 | MENU-03, MENU-07, MENU-08 | T-09-31 | Antiflood parity, drift (1, 2, 3, 100, 101, off; warn, ban), authority parity with the bot-admin rule, bytes, length | integration, pure render | `^(TestSettingsParityAntiflood\|TestSettingsDriftAntiflood\|TestSettingsAuthorityParityAntiflood\|TestSettingsAntifloodCallbackBytes\|TestSettingsAntifloodPageFitsEveryLocale\|...)$`, `./alita/modules` | ❌ W0 | ⬜ pending |
| 09-10-01 | 10 | 3 | MENU-06, MENU-08 | T-09-35, T-09-36, T-09-37 | Lockdown page states in `/lockdownstatus`'s words, trigger toggles and 3..500 steppers through `applyAutoLockdown`, basic group, unreadable state | integration | `^(TestSettingsLockdownPage\|TestSettingsLockdownBasicGroup\|TestSettingsLockdownUnreadable\|TestSettingsLockdownTriggers)$`, `./alita/modules` | ❌ W0 (new settings_lockdown_test.go) | ⬜ pending |
| 09-10-02 | 10 | 3 | MENU-03, MENU-06, MENU-08 | T-09-34 | Trigger parity, drift (2, 3, 500, 501), restrict authority parity, bytes, length with a 200-rune reason | integration, pure render | `^(TestSettingsParityLockdownTriggers\|TestSettingsDriftLockdownTriggers\|TestSettingsAuthorityParityLockdownTriggers\|TestSettingsLockdownCallbackBytes\|TestSettingsLockdownPageFitsEveryLocale\|...)$`, `./alita/modules` | ❌ W0 | ⬜ pending |
| 09-11-01 | 11 | 3 | MENU-05 | T-09-43 | Staff page variants (full in private, masked in a group menu), not linked, read-only `/staff` panel with paging; answered before the live build | integration | `^(TestSettingsStaffPageVariants\|TestSettingsStaffReadOnly)$`, `./alita/modules` | ❌ W0 (new settings_staff_test.go) | ⬜ pending |
| 09-11-02 | 11 | 3 | MENU-04, MENU-05 | T-09-39, T-09-40, T-09-41, T-09-42 | Link (none, one, picker), pick by row ID with a forged `s` refused, Unlink two-tap with expiry and a double Yes; the shared card helper | integration | `^(TestSettingsCardExpiry\|TestSettingsStaffLink\|TestSettingsStaffPick\|TestSettingsStaffUnlink\|...link pinning tests)$`, `./alita/modules` | ❌ W0 | ⬜ pending |
| 09-11-03 | 11 | 3 | MENU-03, MENU-05 | T-09-38 | No group-menu text, label, notice, alert or callback data names the Staff Group, in 7 locales and end to end; authority parity with `/linkstaff` and `/unlinkstaff`; bytes | integration, pure render | `^(TestSettingsStaffPrivacy\|TestSettingsAuthorityParityStaff\|TestSettingsStaffCallbackBytes\|...)$`, `./alita/modules` | ❌ W0 (new settings_staff_privacy_test.go) | ⬜ pending |
| 09-12-01 | 12 | 4 | MENU-06, MENU-08 | T-09-47 | Lift button, two-tap card, Yes through `applyLockdownLift` with the public notice; restore and record failures keep the lockdown | integration | `^(TestSettingsLockdownLift\|TestSettingsLockdownLiftOutcomes\|TestSettingsLockdownPage\|TestSettingsLockdownTriggers)$`, `./alita/modules` | ❌ W0 (new settings_lockdown_lift_test.go) | ⬜ pending |
| 09-12-02 | 12 | 4 | MENU-03, MENU-06, MENU-08 | T-09-44, T-09-45, T-09-46, T-09-48 | Double Yes lifts once with one notice; expiry; restrict authority at ask and Yes; private variant; parity with `/unlockdown`; bytes | integration | `^(TestSettingsLockdownLiftTwice\|TestSettingsLockdownLiftExpired\|TestSettingsLockdownLiftAuthority\|TestSettingsLockdownLiftPrivate\|TestSettingsParityLockdownLift\|TestSettingsLockdownLiftCallbackBytes\|...)$`, `./alita/modules` | ❌ W0 | ⬜ pending |
| 09-13-01 | 13 | 5 | MENU-01..08 | T-09-50 | `settings_help_msg` in 7 locales fits one message; `alt_names.Settings`; the module listed in `/help` | unit, integration | `^(TestSettingsHelpFitsOneMessage\|TestSettingsLocaleKeys\|TestLockdownHelpFitsOneMessage)$` in `./alita/i18n`; `^(TestLoadSettingsRegistersHelp\|TestSettingsTracer)$` in `./alita/modules`; `make check-translations` | ❌ W0 (new settings_help_test.go) | ⬜ pending |
| 09-13-02 | 13 | 5 | MENU-01..08 | T-09-49 | Generated Settings command page, manual command and permission references | docs | `make check-docs && test -s docs/src/content/docs/commands/settings/index.md` | ❌ (generated) | ⬜ pending |
| 09-13-03 | 13 | 5 | MENU-01..08 | — | AGENTS.md testing notes and the antiflood first-write trap; every settings test of the phase together | integration | `CGO_ENABLED=0 go build ./... && go test -tags testtools -race -count=1 -run '^TestSettings' ./alita/modules ./alita/i18n && make check-translations && make check-docs` | ✅ | ⬜ pending |
| 09-14-01 | 14 | 5 | MENU-01..04 | T-09-51 | The owner's real-client checklist with at least 12 checks | docs | `test -s .planning/phases/09-settings-menu/09-SPIKE.md` and the heading and "Pass looks like" counts | ❌ (new 09-SPIKE.md) | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] Characterization tests that pin today's reply keys for `/captcha*`, `/setflood*`, `/delflood`, `/autolockdown`, `/unlockdown`, `/linkstaff` and `/unlinkstaff` before the refactor: the existing pinning tests are named in the verify commands of 09-01 Task 1, 09-02, 09-03 Task 2, 09-04 and 09-06, and new apply-function tests are added there (TestApplyFlood*, TestApplyAutoLockdown, TestApplyLockdownLift, TestApplyCaptcha*, TestAntifloodCommandsUseApply)
- [ ] `TestSetFloodModeOnFreshChatStoredLimit` and `TestSetFloodMsgDelOnFreshChatStoredLimit` in `alita/db/antiflood` (09-03 Task 1)
- [ ] `settingsEnv` fixture in `alita/modules/settings_env_test.go` (09-01 Task 1): the lockdown env with LoadSettings, LoadCaptcha, LoadAntiflood, LoadStaff, LoadHelp and LoadBotUpdates, a real-locale option, `openMenu`, `press`, `pressRaw`, `pressNoMessage`, `pressPrivate`, `sendPrivate`, `sendIn`, `edits`, `lastEdit`, `answerCount`, `settingsButtonByMarker`, `settingsButtonFields`; later plans never edit it
- [ ] `alita/i18n/settings_locale_test.go` (09-01 Task 3, extended by 09-05 and 09-13)
- [ ] No harness `AutoMigrate` list change is needed: no plan adds a model, and every model the pages read (`CaptchaSettings`, `AntifloodSettings`, `StaffGroup`, `StaffGroupLink`, `ChatLockdown`, `LockdownJoiner`, `ChatRaidDetection`) is already in `alita/modules/test_harness_test.go`

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| The Open in private URL button into `/start` for a user who never started the bot; the menu arrives and manages only that group | MENU-01, MENU-02 | Needs a real bot and Telegram clients | `09-SPIKE.md` |
| Close on a private menu message older than 48 hours; "not modified" and rate-limit behaviour when tapping a stepper quickly; alert rendering on iOS, Android and Desktop; button text truncation in hi and ru; a forwarded menu; a forum-topic group; callback validity after a slow Staff page build | MENU-03, MENU-04 | Telegram client and platform behaviour | `09-SPIKE.md` |
| Native-speaker read of the new `settings_*` strings in es, fr, hi, id, pt and ru | MENU-01..07 | Translation quality | Review the new keys |
| Lift from a group menu and from a private menu posts the public notice once on real clients; Link and Unlink from a group menu leave no trace of the Staff Group in the group | MENU-05, MENU-06 | Real clients and real groups | `09-SPIKE.md` (plan 09-14) |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 90s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
