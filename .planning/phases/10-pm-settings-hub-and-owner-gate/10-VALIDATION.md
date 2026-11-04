---
phase: "10"
slug: "pm-settings-hub-and-owner-gate"
# status lifecycle: draft (seeded by plan-phase) → validated (set by validate-phase §6)
status: validated
nyquist_compliant: true
wave_0_complete: true
created: "2026-11-04"
---

# Phase 10 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution. Seeded from `10-RESEARCH.md` § Validation Architecture; the per-task map is filled by the planner and executor as tasks get IDs.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go `testing`, build tag `testtools`, miniredis, hand-written `gotgbot.BotClient` fakes (no mock libraries) |
| **Config file** | none; `Makefile` target `test` |
| **Quick run command** | `go test -tags testtools -race -count=1 -run '^(TestSettings\|TestLoadSettings\|TestAdminIndex\|TestMenuAccess)' ./fuku/modules ./fuku/i18n ./fuku/db/adminindex ./fuku/db/menuaccess ./fuku/utils/cache` |
| **Full suite command** | `make test` (never a bare `go test ./...`) |
| **Estimated runtime** | quick: under 30 s for a focused `-run`; full: several minutes |

---

## Sampling Rate

- **After every task commit:** the quick run command (settings + hub + gate), narrowed with `-run` to the task's tests.
- **After every plan wave:** `go test -tags testtools -race -count=1 ./fuku/modules ./fuku/i18n ./fuku/db/... ./fuku/utils/cache` plus `make lint`.
- **Before `/gsd-verify-work`:** `make test`, `make lint`, `make check-translations`, `make generate-docs` then `make check-docs`; `make test-postgres-integrity` runs in CI (needs a PostgreSQL server).
- **Max feedback latency:** 60 seconds for a focused task run.

---

## Per-Requirement Verification Map

The planner expands each row into task-level rows (task ID, plan, wave) in its plans' `<verify>` blocks.

| Requirement | Secure behavior / observable | Test type | Automated command (all with `-tags testtools -race -count=1`) | File exists |
|-------------|------------------------------|-----------|---------------------------------------------------------------|-------------|
| MENU-09 | private `/settings` list (one group is still a list; paging 8; empty text; order; label cut; pick edits list into Main with `[Groups \| Close]`); `shub` callbacks private only, rebuilt from `From.Id`, service identity refused, one answer; worst-case callback bytes | integration | `-run '^(TestSettingsHubList\|TestSettingsHubPaging\|TestSettingsHubEmpty\|TestSettingsHubPick\|TestSettingsHubCallbacks\|TestSettingsHubCallbackBytes\|TestSettingsAccessCallbackBytes\|TestSettingsCallbackBytes)$' ./fuku/modules` | ✅ |
| MENU-10 | roster written from admin-list loads (humans only), promotion/demotion reload, bot removed clears, failed load leaves rows, `/admincache` and group `/settings` refresh; repository upsert+prune; sweeper (lock, no-Redis, unreadable never deletes, 429 backs off, panic survives) | integration + unit (SQLite) | `-run '^(TestAdminIndex)' ./fuku/modules` and `./fuku/db/adminindex` | ✅ |
| MENU-10 | PostgreSQL guarantees: concurrent replace, CHECK, unique | PG integrity | `make test-postgres-integrity` after adding `TestAdminIndexReplaceConcurrent`, `TestMenuAccessUpsert` and their packages | ✅ names wired in Makefile; both green on SQLite here; PostgreSQL run is CI-only |
| MENU-11 | gate table (creator in with no admin-list call; admin + switch on in; admin + off + other creator refused; admin + off + no creator in; error/timeout/empty/own-entry-missing/unreadable switch → `settings_check_failed`; singleflight; positive memo ≤ 30 s, "no owner"/errors never memoised); every door decides the same; pick answers identical for non-member, bot-missing, gone, non-group, unrecognised 4xx; deep link classification | integration | `-run '^(TestSettingsGate\|TestSettingsOwnerState\|TestSettingsGateAllDoors\|TestSettingsPickSameAnswer\|TestSettingsStartDeepLink)' ./fuku/modules` | ✅ |
| MENU-12 | Access page live creator only; forged `op/no/tg/st/ay` by an allowed admin refused and nothing written; OFF one tap, ON two taps; expiry; re-check; write failure keeps old state | integration | `-run '^(TestSettingsAccess\|TestSettingsDriftAccess\|TestSettingsOwnerAuthorityAccess)' ./fuku/modules` | ✅ |
| MENU-13 | list hides a gated group from an admin; public refusal names no switch/Staff Group; in-group commands unchanged for the same admin; empty list identical for everyone; authority-parity carve-out | integration | `-run '^(TestSettingsListHidesGated\|TestSettingsInGroupRefusal\|TestSettingsCommandsUngated\|TestSettingsEmptyListSameForAll\|TestSettingsAuthorityParity)' ./fuku/modules` | ✅ |
| MENU-14 | two replicas: flip on A, next press on B sees it (both directions); anonymous owner via proof tap passes, anonymous admin refused, gate after the swap | integration | `-run '^(TestSettingsAccessAcrossReplicas\|TestSettingsAnonymousAdmin\|TestSettingsAnonymousOwnerGate)$' ./fuku/modules` | ✅ |
| MENU-15 | two-column grid, Access own last row for the live owner only, private nav `[Groups \| Close]`, group nav `[Close]`, Back on every sub-page; Staff Group never named in list/Main/Access in 8 languages | integration/pure | `-run '^(TestSettingsMainGrid\|TestSettingsNavigation\|TestSettingsStaffPrivacy)$' ./fuku/modules` | ✅ |
| MENU-15, PLAT-05 | pages fit every locale (≤ 3800 UTF-16 units, grid label ≤ 20, toggle ≤ 40, groups label ≤ 18); 19 keys in 8 locales (present, non-empty, ceilings, placeholders); help message fits; `staffLocales` equals `locales/*.yml` minus `config.yml` | unit | `-run '^(TestSettingsHubPageFitsEveryLocale\|TestSettingsAccessPageFitsEveryLocale\|TestSettingsLabelLengths\|TestSettingsLocaleKeys\|TestSettingsHelpFitsOneMessage\|TestLocalesAreTheEight)$' ./fuku/modules ./fuku/i18n` | ✅ |
| PLAT-05 | docs say 8 (AGENTS.md, README.md, PROJECT.md, CLAUDE.md) | shell | `! grep -nE '\b7 (locale\|language)\|all 7 files\|seven locale' AGENTS.md README.md .planning/PROJECT.md .claude/CLAUDE.md` | ✅ grep clean |
| (all) | access tables stay out of backup; migration statements idempotent, constraints inline, no `BEGIN` | unit | `-run '^TestAccessTablesStayOutOfBackup$' ./fuku/db/adminindex` and `-run '^TestSettingsHubMigrationStatements$' ./fuku/db/migrations` | ✅ |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [x] Migration `20261104120000_add_settings_hub_tables.sql` (re-check the latest filename at execution time), models, `fuku/db/adminindex`, `fuku/db/menuaccess` with their `testmain_test.go` (`testdb.Run`)
- [x] Add both models to the `AutoMigrate` list in `fuku/modules/test_harness_test.go`
- [x] Harness: seed `admins_allowed = true` in `newSettingsEnvWith` / `addChat` (so every Phase 9 press test keeps passing unchanged) plus cleanup, `env.owner`, `env.setAdminsAllowed`, `env.otherReplica()`, and administrator records in the fake's `getChatAdministrators` (it lists only the creator and the bot today)
- [x] `settings_hub_test.go`, `settings_gate_test.go`, `settings_access_test.go`, `admin_index_feed_test.go`, `admin_index_sweeper_test.go`
- [x] `fuku/i18n`: 19 rows, 3 alert keys and the help ceiling in `settings_locale_test.go`; a new locale-set guard test
- [x] `Makefile` `test-postgres-integrity`: add the two test names and two packages

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Hub list, pick, Access ON card and OFF, gated refusals in a real Telegram client (phone width, hi/ru label truncation) | MENU-09..15 | Needs the owner's bot token, real groups and an admin account | Run the live-check list the owner keeps in the phase SPIKE file, as for Phases 8 and 9 |
| A group whose owner account was deleted: what `getChatAdministrators` returns | MENU-11 (D-15) | Needs a real group with a deleted owner; the Bot API text could not be fetched | Record the answer, then confirm or drop assumption A1 in `10-RESEARCH.md` |
| A bot that is only a plain member calling `getChatAdministrators` | MENU-11 | Needs a real group; fail-closed either way | Record the answer (assumption A2) |
| Startup backfill and hourly sweep with several real groups, two replicas | MENU-10 | Needs the deployed bot and a PostgreSQL + Redis stack | Check logs for one runner per cycle and the filled roster |

---

## Validation Sign-Off

- [x] All tasks have `<automated>` verify or Wave 0 dependencies
- [x] Sampling continuity: no 3 consecutive tasks without automated verify
- [x] Wave 0 covers all MISSING references
- [x] No watch-mode flags
- [x] Feedback latency < 60 s
- [x] `nyquist_compliant: true` set in frontmatter

**Approval:** validated 2026-11-04. Audit: every test name in the map exists and every row was run green on SQLite/miniredis. CI-only: `make test-postgres-integrity` (PostgreSQL), `make lint`. Manual-only table unchanged.
