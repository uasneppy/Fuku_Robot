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

Seeded from the research's requirement-to-test map. The planner and executors replace each row with the real task IDs; the Nyquist audit fills the status.

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 09-XX-XX | XX | N | MENU-01 | T-09 (access control) | `/settings` replies with Open here and the deep-link URL button; non-admin and basic group refused; an anonymous admin gets the proof then the choice; `/start settings_<id>` handles valid, invalid, non-admin, check-failed and group-gone cases | integration (dispatcher) | `go test -tags testtools -race -count=1 -run '^(TestSettingsCommand\|TestSettingsStart)' ./alita/modules` | ❌ W0 | ⬜ pending |
| 09-XX-XX | XX | N | MENU-02 | T-09 (spoofing) | `u` and `c` structural rules; another member's press refused with `settings_not_your_menu`; a forwarded menu cannot act in another chat; a private menu manages only its `c` | integration | `... -run '^TestSettingsOwnership$' ./alita/modules` | ❌ W0 | ⬜ pending |
| 09-XX-XX | XX | N | MENU-03 | T-09 (elevation) | Press order: service identity before any lookup, opener before the live check, a demoted admin refused and nothing edited, a failed lookup refuses; exactly one answer per press | integration | `... -run '^TestSettingsPressPipeline$' ./alita/modules` | ❌ W0 | ⬜ pending |
| 09-XX-XX | XX | N | MENU-04 | T-09 (dead button, DoS) | One message edited in place; Back and Close on every page; Close falls back to an edit; "not modified" is success; no worst-case button exceeds 64 bytes or is omitted | integration, pure render | `... -run '^(TestSettingsNavigation\|TestSettingsCallbackBytes)$' ./alita/modules` | ❌ W0 | ⬜ pending |
| 09-XX-XX | XX | N | MENU-05 | T-09 (information disclosure) | Staff page variants (full, masked in a group menu, read-only in a Staff Group); link with none, one, several (picker) and a forged `s`; two-tap unlink; no group-menu text, label, notice, alert or callback data contains the Staff Group's title or ID, in all 7 locales | integration | `... -run '^TestSettingsStaff' ./alita/modules` | ❌ W0 | ⬜ pending |
| 09-XX-XX | XX | N | MENU-06 | T-09 (integrity) | Lockdown page states; Lift two-tap with expiry and a double Yes (lifts once, posts the public notice); trigger switches and steppers including bounds; basic group note | integration | `... -run '^TestSettingsLockdown' ./alita/modules` | ❌ W0 | ⬜ pending |
| 09-XX-XX | XX | N | MENU-07 | T-09 (integrity) | Captcha page (on/off, mode with Turnstile's three ordered refusals, time 1..10, action) and Antiflood page (Off then 3..100, mute/ban/kick, delete switch, stored warn/tban/tmute read-only, the fresh-chat stored limit) | integration | `... -run '^(TestSettingsCaptcha\|TestSettingsAntiflood)' ./alita/modules` | ❌ W0 | ⬜ pending |
| 09-XX-XX | XX | N | MENU-08 | T-09 (repudiation/drift) | Per setting: command-then-menu, menu-then-command, stored rows equal; one drift table gives the same verdict to the command and the menu; authority parity (including antiflood's bot-admin rule and `/captcha`'s) | integration | `... -run '^(TestSettingsParity\|TestSettingsDrift\|TestSettingsAuthorityParity)$' ./alita/modules` | ❌ W0 | ⬜ pending |
| 09-XX-XX | XX | N | MENU-01..08 | — | The `settings_*` locale keys are in all 7 files with identical placeholders, within the UI-SPEC ceilings (alerts at most 200 characters), `settings_help_msg` fits one message, no disallowed markup; the reused staff texts keep their placeholders | unit | `go test -tags testtools -race -count=1 -run '^TestSettingsLocale' ./alita/i18n` | ❌ W0 | ⬜ pending |
| 09-XX-XX | XX | N | MENU-08 | — | Characterization: the commands' replies and stored rows are unchanged by the refactor (the existing pinning tests stay green); `SetFloodMode` on a fresh chat | unit, integration | `go test -tags testtools -race -count=1 -run '^(TestCaptcha\|TestAntiflood\|TestAutoLockdown\|TestAutolockdownAI\|TestUnlockdown\|TestLockdownLift)' ./alita/modules` and `... -run '^TestSetFloodModeOnFreshChatStoredLimit$' ./alita/db/antiflood` | ❌ W0 | ⬜ pending |
| 09-XX-XX | XX | N | MENU-01..08 | — | Docs: `/settings` command page, help text and `alt_names` regenerate cleanly; AGENTS.md carries the menu rules | docs | `make generate-docs && make check-docs && make check-translations` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] Characterization tests that pin today's reply keys for `/captcha*`, `/setflood*`, `/delflood`, `/autolockdown`, `/unlockdown`, `/linkstaff` and `/unlinkstaff` before the refactor (several exist; the research lists them)
- [ ] `TestSetFloodModeOnFreshChatStoredLimit` in `alita/db/antiflood`
- [ ] `settingsEnv` fixture in `alita/modules/settings_env_test.go`: the lockdown env plus a Staff Group fixture, per-user member records, a press helper building `gotgbot.CallbackQuery{Message: ...}`, and `lastAnswer`/`edits` accessors
- [ ] `alita/i18n/settings_locale_test.go` (patterns: `lockdown_locale_test.go`, `captcha_web_locale_test.go`)
- [ ] No harness `AutoMigrate` list change is needed (every model the pages read is already in every list); confirm

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| The Open in private URL button into `/start` for a user who never started the bot; the menu arrives and manages only that group | MENU-01, MENU-02 | Needs a real bot and Telegram clients | `09-SPIKE.md` |
| Close on a private menu message older than 48 hours; "not modified" and rate-limit behaviour when tapping a stepper quickly; alert rendering on iOS, Android and Desktop; button text truncation in hi and ru; a forwarded menu; a forum-topic group; callback validity after a slow Staff page build | MENU-03, MENU-04 | Telegram client and platform behaviour | `09-SPIKE.md` |
| Native-speaker read of the new `settings_*` strings in es, fr, hi, id, pt and ru | MENU-01..07 | Translation quality | Review the new keys |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 90s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
