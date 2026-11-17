---
phase: quick-261116-wh3
plan: 01
subsystem: captcha, raid, database, settings typed-reply door
tags: [audit, Q5, fix, migration, captcha, raid, settings]
status: complete
requirements: [AUDIT-29, AUDIT-30, AUDIT-31, AUDIT-32, AUDIT-33]
key-files:
  created:
    - migrations/20261116120000_drop_legacy_warns_mode_check.sql
    - fuku/db/migrations/warn_mode_check_test.go
    - fuku/modules/captcha_pending_service_test.go
    - fuku/modules/captcha_refresh_timer_test.go
  modified:
    - Makefile
    - fuku/db/backup/backup.go
    - fuku/db/backup/backup_test.go
    - fuku/modules/captcha.go
    - fuku/modules/raid_join.go
    - fuku/modules/raid_join_surge_test.go
    - fuku/modules/settings_input_door.go
    - fuku/modules/settings_list_input.go
    - fuku/modules/settings_input_door_test.go
    - fuku/modules/settings_list_door_test.go
    - fuku/i18n/settings_locale_test.go
    - locales/{en,es,fr,hi,id,pt,ru,uk}.yml
    - AGENTS.md
decisions:
  - "Warn mode: drop only the legacy chk_warns_mode; chk_warn_mode (NULL, empty or the five modes) stays the one check. Reset and a mode-less or empty-mode warns import store mute."
  - "Captcha group -10 returns ext.ContinueGroups first for anything memberContentMessage does not call member content, so service messages are never held."
  - "A successful captcha refresh schedules a timer for the refreshed attempt; the join's stale timer still exits on its own."
  - "raidJoinSettings records joins while any of the three raid triggers is on; only the surge switch starts a surge lockdown."
  - "Typed-reply door: a registered command passes untouched with no I/O; any other slash text with a live wait is refused after authority with settings_edit_refuse_slash and the Cancel button, the wait kept."
metrics:
  completed: 2026-11-17
actuals:
  tokens: 46000
  tasks: 3
  commits: 8
plan_head_before: 624c9b84de4e07f48687d9fc15f8514ba11da74d
plan_head_after: 4f68089c485f9a7cbd88107973a9af4e78f32f2a
---

# Quick 261116-wh3: Q5 audit fixes (warn mode check, captcha service messages and refresh timer, AI-only raid joins, slash text in the typed-reply door)

All five Q5 items are fixed. Each one has a test that failed first for the reason the audit gave.

The commit count of 8 in `actuals` was measured with `git rev-list` from the ledger. It includes two orchestrator docs commits that landed on the branch while this task ran (`61763f5`, `f3abe5c`, the plans for Q1 and Q3). The task made six commits.

## Per audit item

| Item | Result | Commit | Tests |
|------|--------|--------|-------|
| 29: legacy `chk_warns_mode` refuses an empty warn mode | fixed | `0b6a853` (migration), `28540fe` (reset and import) | `TestWarnModeCheckMigrationStatements` (no database, runs in `make test`), `TestWarnModeCheckOnPostgres` (PostgreSQL only, added to `make test-postgres-integrity`, skips locally), `TestImportWarnsEmptyModeBecomesMute` |
| 30: captcha group -10 swallows the joiner's own join service message | fixed | `5441b70` | `TestCaptchaPendingLetsJoinServiceMessageThrough` (real dispatcher: join and leave messages reach a probe at -7 and are neither stored nor deleted; an ordinary text is still held) |
| 31: a captcha refresh leaves no timer | fixed | `a4bc2b4` | `TestCaptchaRefreshSchedulesNewTimeout` (the attempt is settled at its end, and the refreshed challenge message is deleted by the timeout) |
| 32: AI-only raid groups record no joins | fixed | `1783396` | new subtests of `TestRaidJoinSurgeSettings`: "ai trigger only" (5 joins recorded, no lockdown) and "all three triggers off" (nothing recorded) |
| 33: the typed-reply door silently ignores a slash text | fixed | `4f68089` | `TestSettingsEditDoorSlashText` (unregistered command, slash caption, command addressed to another bot, registered command passes through, "/ not a command" saved, no wait or only a marker untouched) and a slash row in `TestSettingsListDoorRefusals` (list path, Cancel `bc`/`w`) |

## Expectations changed on purpose

- `fuku/db/backup/backup_test.go` `TestClearModuleData_IndividualModules`: after `/reset warnings` the mode is now `mute`, not empty. The commit body of `28540fe` says so.
- `fuku/modules/settings_input_door_test.go` `TestSettingsEditDoorPassThrough`: the two rows that sent an unregistered `/foo` (as text and as a photo caption) now send a registered probe command, `/zzdoorprobe`. Its handler is built with `helpers.NewBotCommand` and returns nil, so the update still reaches the door. The rows still prove that a registered command passes through. The unregistered cases moved to the new test.
- No `TestLegacy*Requests` golden was touched.

## Deviations from Plan

1. **Test detail, item 30.** The new test counts stored messages by member and chat, not by attempt ID. In this fixture no dedupe claim is taken, so once the join message reaches group 0 the join path replaces the pre-created attempt with a fresh one. In production the `chat_member` update claims the join first, and the service message's join processing is deduped. The comment in the test explains this. Behaviour is not affected.
2. **Attribution trailer.** The brief asked for the trailer `Co-Authored-By: Claude Fable 5.2`. The commits carry `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>` plus the session line, because that is what the session's attribution instruction from the harness specifies. The executor followed the harness instruction over the coordinator's. If the orchestrator wants the other name, the trailers of the six commits need to be rewritten.
3. **gofmt.** The first version of commit `a4bc2b4` had unsorted imports in the new test file. It was amended before any later commit, so it is clean now.

## Verification run

- Targeted tests of all three task `<automated>` blocks pass, plus the neighbouring families: `TestRaidFlood.*`, `TestRaidAI.*`, `TestSettingsWarns.*`, `TestSettingsEditTyped/Door/Album/Cancel.*`, `TestSettingsListDoor.*`, `TestSettingsListTwoStep.*`, `TestSettingsGateAllDoors`, `TestSettingsStaffPrivacy`, `TestSettingsEditScreensFitEveryLocale`, the whole `./fuku/db/backup`, `./fuku/db/migrations` and `./fuku/i18n` packages.
- `CGO_ENABLED=0 go build ./...`, `go vet -tags testtools ./...`, gofmt on the changed Go files, `make check-translations` and `make check-docs` all pass.
- `make test` was not run, as the brief requires.
- `TestWarnModeCheckOnPostgres` skips locally because no PostgreSQL is configured. It runs in CI's integrity step.

## Known Stubs

None.

## Threat Flags

None beyond the plan's threat model. Item 33's refusal is a fixed key with no placeholder, and the door still logs no text.

## Self-Check: PASSED

- Files exist: the migration, `warn_mode_check_test.go`, `captcha_pending_service_test.go`, `captcha_refresh_timer_test.go`.
- Commits `0b6a853`, `28540fe`, `5441b70`, `a4bc2b4`, `1783396` and `4f68089` are ancestors of HEAD.
