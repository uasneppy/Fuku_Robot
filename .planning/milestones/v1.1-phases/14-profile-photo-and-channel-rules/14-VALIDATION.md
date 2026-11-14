---
phase: "14"
slug: "profile-photo-and-channel-rules"
# status lifecycle: draft (seeded by plan-phase) → validated (set by validate-phase §6)
status: validated
nyquist_compliant: true
wave_0_complete: true
created: "2026-11-08"
---

# Phase 14 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution. Seeded from `14-RESEARCH.md` § Validation Architecture and the twelve plans 14-01 to 14-12; the test names are the plans' contract, and the executors set the status.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| Framework | Go `testing`, build tag `testtools`, testify assertions in `fuku/db` tests only, miniredis, hand-written `gotgbot.BotClient` fakes (the shared `lockdownFake`/`staffActionFake` and the new `mreqLookupFake` wrapper from 14-03), SQLite via `internal/testdb.Run`, no mock libraries |
| Config file | none; `Makefile` targets `test` (30 min per package) and `test-postgres-integrity` |
| Quick run, lookup layer and executor | `go test -tags testtools -race -count=1 -run '^(TestMemberReq)' ./fuku/modules` |
| Quick run, settings page | `go test -tags testtools -race -count=1 -run '^(TestSettings\|TestLoadSettings)' ./fuku/modules ./fuku/i18n` (about 4.5 minutes; only at plan end) |
| Quick run, data and locales | `go test -tags testtools -race -count=1 ./fuku/db/memberreq ./fuku/db/migrations ./fuku/db/backup ./fuku/i18n` |
| Full suite command | `make test` (never a bare `go test ./...`), then `make lint`, `make check-translations`, `make generate-docs` then `make check-docs`; CI: `make test-postgres-integrity` |
| Enforcement switch | `memberReqEnforcementLive` stays true by default (Phase 13); tests write it only through `withMemberReqLive`; no test that uses it runs in parallel |
| Lookup globals | the lookup pacer (`memberReqLookupPacer`), the singleflight group and the shutdown root are package globals: tests install `withFastLookupPacer`, release a held flight in `t.Cleanup` and call `restoreMemberReqRunner` after `StopMemberReqJobs` |

---

## Sampling Rate

- **Per task commit:** the focused `-run '^(...)$'` of the task's tests (seconds each), plus the package run of any `fuku/db/*` package or `./fuku/i18n` the task touched.
- **Per plan end:** the plan's family run in its `<verification>` block; the Phase 13 guarantee tests the plan names are re-run unchanged whenever the plan edited the code that carries them (14-06 executor and join, 14-07 watcher and held branch, 14-08 notice and button); the settings family at the end of 14-05, 14-09 and 14-10.
- **Per wave merge:** `go test -tags testtools -race -count=1 ./fuku/modules ./fuku/i18n ./fuku/db/...` (about 11 to 15 minutes for `fuku/modules`) plus the lint stand-in `python3 .planning/phases/13-member-requirements/handoff/lint-local.py 8c0c827` (an invariant gate: it exits 0 at HEAD and fails only on lines the phase added).
- **Flake discipline:** every new concurrency test runs `-race -count=20` once (the singleflight, pacer, watcher-singleflight, one-punishment-across-replicas and join tests); any test that joins, rejoins or sends with a date takes the date before the message at least 2 s apart (`mreqSendAt`, the Phase 13 second-boundary lesson, commit 96b2f73); multi-goroutine tests use Mute or Kick, never Warn (SQLite "database is locked" with concurrent warn writers); fresh IDs and cleanups per `memberreq_env_test.go`.
- **Phase gate:** `make test` (or the documented sandbox fallback of 12-16-SUMMARY and 13-14-SUMMARY), `make lint` (or its fallback), `make check-translations`, `make generate-docs` then `make check-docs`, `git diff --exit-code 8c0c827 -- go.mod go.sum`, CI `make test-postgres-integrity`, and the owner's live checklist `14-SPIKE.md` (deferred like Phases 7 to 13 unless the owner says otherwise).

---

## Per-Requirement Verification Map

Every command is `go test -tags testtools -race -count=1` with the given `-run` and package. "Completes" names the one plan that completes the requirement.

| Requirement | Behavior | Plan | Test Type | Automated Command | File Exists | Status |
|---|---|---|---|---|---|---|
| MREQ-11 (completes: 14-08) | the photo rule punishes a member with no photo the bot sees, spares one with a photo; a hidden photo counts as missing; positive cached 30 min, a missing photo never; Fresh bypasses and refreshes | 14-03, 14-06 | integration (fake bot, miniredis) | `-run '^(TestMemberReqPhotoRule\|TestMemberReqLookupPunishes\|TestMemberReqLookupSpares)$' ./fuku/modules` | ✅ | ✅ green |
| MREQ-11 | the watcher checks a message from cached positives alone and hands one job on a miss; 100 messages one lookup; exempt members cost nothing | 14-07 | integration | `-run '^(TestMemberReqWatcherLookupRules\|TestMemberReqWatcherLookupSingleflight\|TestMemberReqPassCacheWithLookups\|TestMemberReqWatcherLookupContinuesGroups)$' ./fuku/modules` | ✅ | ✅ green |
| MREQ-11 | the notice carries the photo remedy only for warn, kick and mute; the button re-checks the photo fresh; the page says a hidden photo counts as missing and recommends Warn or Mute | 14-08, 14-05 | integration | `-run '^(TestMemberReqNoticePhotoRemedy\|TestMemberReqFixedLookups\|TestSettingsMreqPhotoScreen)$' ./fuku/modules` | ✅ | ✅ green |
| MREQ-12 (completes: 14-10) | the linked channel is resolved by getChat and cached (positive and none); subscribed statuses spare; left, kicked and restricted-not-member punish only with an administrator bot; no link is inert | 14-04, 14-06 | integration | `-run '^(TestMemberReqLinkedChannel\|TestMemberReqChannelRule\|TestMemberReqChannelBreachNeedsBotAdmin\|TestMemberReqLookupPunishes)$' ./fuku/modules` | ✅ | ✅ green |
| MREQ-12 | the channel rule is offered only in a linked group: Off-only screen, refused punishing press, forged and stale presses | 14-09, 14-10 | integration | `-run '^(TestSettingsMreqChannelStatus\|TestSettingsMreqChannelRefusal\|TestSettingsMreqForgedFields)$' ./fuku/modules` | ✅ | ✅ green |
| MREQ-10 (Phase 14 part: 14-09; completed by Phase 15's AI line) | every unanswerable shape (500, 400, 403, 429, timeout, shed, cancelled, null body, unrecognised status) punishes nobody, leaves no hold, notice or write, in the job and the join | 14-06 | integration, table of shapes | `-run '^(TestMemberReqLookupUnanswerable\|TestMemberReqLookupDoesNotBlockNameBreach\|TestMemberReqLookupOrder)$' ./fuku/modules` | ✅ | ✅ green |
| MREQ-10 | the classifier is by status and code; a lost channel is inert, an unknown stays unknown; a held member is released only when clean or inert | 14-04, 14-07, 14-08 | integration | `-run '^(TestMemberReqBotStatusClassifier\|TestMemberReqHeldRelease\|TestMemberReqChannelInert)$' ./fuku/modules` | ✅ | ✅ green |
| MREQ-10 | the page shows no linked channel, the bot not administrator, basic group and could-not-check in words, with at most two paced calls, never an error text | 14-09 | integration | `-run '^(TestSettingsMreqChannelStatus\|TestSettingsMreqBasicGroup\|TestSettingsMreqOpenCost\|TestSettingsMreqRedrawKeepsStatus)$' ./fuku/modules` | ✅ | ✅ green |
| PLAT-06 (completes: 14-11; cross-cutting, binds Phases 15 to 17) | one lookup for 100 callers; the pacer shared across two replicas, a far slot shed with no request, Phase 13's keys untouched; a flight that panics recovers; jobs and flights end at StopMemberReqJobs | 14-03, 14-06 | integration, concurrency | `-run '^(TestMemberReqLookupSingleflight\|TestMemberReqLookupPacer\|TestMemberReqLookupRedisDown\|TestShutdownEndsMemberReqLookups)$' ./fuku/modules` | ✅ | ✅ green |
| PLAT-06 | the lookup budget and the punishment budget are independent at the job, the join, the button and the page | 14-11 | integration | `-run '^TestMemberReqPacersAreIndependent$' ./fuku/modules` | ✅ | ✅ green |
| MREQ-10, MREQ-11 | "I fixed it": step 4b approvals, fresh lookups under 5 s, broken first, count answer, unknown changes nothing | 14-08 | integration | `-run '^(TestMemberReqFixedApproved\|TestMemberReqFixedLookups\|TestMemberReqChannelInert\|TestMemberReqFixedNeverLiftsWithLookupRules)$' ./fuku/modules` | ✅ | ✅ green |
| MREQ-01 (seven of eight; stays Pending until MREQ-14) | CHECKs 0..4 on both new columns, zero defaults for an existing row, no clobbering; migration statements, idempotence, named constraints | 14-01 | repository (SQLite and PostgreSQL), unit | `-run '^TestChatRequirementsSettings$' ./fuku/db/memberreq` and `-run '^(TestMemberRequirementLookupMigrationStatements\|TestMemberRequirementLookupMigrationDefaults)$' ./fuku/db/migrations` (CI also runs both on PostgreSQL) | ✅ | ✅ green |
| MREQ-01 | the seven-rule table, page grid, rows, bytes, the lookup mask fold | 14-05 | unit and integration | `-run '^(TestMemberReqRuleTable\|TestMemberReqBrokenIgnoresLookupRules\|TestMemberReqStrictestLookupBits\|TestMemberReqLookupBroken\|TestMemberReqRuleKeysExist\|TestSettingsMreqLookupRulesStore\|TestSettingsMreqRowBudget\|TestSettingsMreqCallbackBytes)$' ./fuku/modules` | ✅ | ✅ green |
| UI / locale (PLAT-05 binds) | 29 `mreq_` and 247 `settings_` keys in 8 locales, ceilings, notice cap 520, count answer, no "profile" in the seven reworded keys | 14-02 | locale | `-run '^(TestMemberReqLocaleKeys\|TestMemberReqNoProfileInReworkedKeys\|TestSettingsLocaleKeys\|TestSettingsLabelLengths\|TestSettingsHelpFitsOneMessage)$' ./fuku/i18n` | ✅ (extended) | ✅ green |
| Privacy (D-18) | no Phase 14 screen, alert or log names the Staff Group, the channel, a file ID or a member; alerts within 200; the new columns stay out of every backup | 14-11 | integration | `-run '^(TestSettingsStaffPrivacy\|TestMemberReqLookupLogsCarryIDsOnly\|TestMemberReqLookupAlertsFitEveryLocale)$' ./fuku/modules` and `-run '^TestMemberRequirementsStayOutOfBackup$' ./fuku/db/backup` | ✅ (extended) | ✅ green |
| Vertical slice (all four) | page press, dispatcher message, notice with remedy and button, press, freed; channel likewise; nobody punished with the bot not an administrator | 14-12 | end to end | `-run '^TestMemberReqLookupEndToEnd$' ./fuku/modules` | ✅ | ✅ green |
| Phase 13 guarantees (re-asserted) | one punishment across replicas, held path, a clean message ends a warn hold, never lifts another mute, cap and pause, join behaviour, shutdown drain | 14-06, 14-07, 14-08 | integration | the anchored re-run lists of 14-06 Task 3, 14-07 Task 3 and 14-08 Task 3 | ✅ | ✅ green |

---

## Per-Task Verification Map

| Task | Plan | Wave | Requirement | Test(s) | Automated Command | Status |
|------|------|------|-------------|---------|-------------------|--------|
| 14-01-T1 (tracer) | 01 | 1 | MREQ-11, MREQ-12 | `TestChatRequirementsSettings` | `-run '^TestChatRequirementsSettings$' ./fuku/db/memberreq` | ✅ green |
| 14-01-T2 | 01 | 1 | MREQ-11, MREQ-12 | `TestMemberRequirementLookupMigrationStatements`, `TestMemberRequirementLookupMigrationDefaults` (PostgreSQL) | `-run '^TestMemberRequirementLookupMigrationStatements$' ./fuku/db/migrations` | ✅ green |
| 14-01-T3 | 01 | 1 | MREQ-11, MREQ-12 | the PostgreSQL pair on a throwaway cluster; `make -n test-postgres-integrity` | `make -n test-postgres-integrity \| grep -c TestMemberRequirementLookupMigrationDefaults` | ✅ green |
| 14-02-T1 (tracer) | 02 | 2 | MREQ-10, MREQ-11, MREQ-12 | `TestMemberReqLocaleKeys` | `-run '^TestMemberReqLocaleKeys$' ./fuku/i18n` | ✅ green |
| 14-02-T2 | 02 | 2 | MREQ-10, MREQ-11, MREQ-12 | `TestSettingsLocaleKeys`, `TestSettingsLabelLengths`, `TestSettingsHelpFitsOneMessage`, `TestMemberReqNoProfileInReworkedKeys` | `-run '^(TestSettingsLocaleKeys\|TestSettingsLabelLengths\|TestSettingsHelpFitsOneMessage\|TestMemberReqLocaleKeys\|TestMemberReqNoProfileInReworkedKeys)$' ./fuku/i18n` | ✅ green |
| 14-03-T1 (tracer) | 03 | 3 | MREQ-10, MREQ-11, PLAT-06 | `TestMemberReqVerdictZeroValue`, `TestMemberReqPhotoRule` | `-run '^(TestMemberReqVerdictZeroValue\|TestMemberReqPhotoRule)$' ./fuku/modules` | ✅ green |
| 14-03-T2 | 03 | 3 | PLAT-06 | `TestMemberReqLookupSingleflight`, `TestMemberReqLookupPacer`, `TestMemberReqLookupRedisDown` | `-run '^(TestMemberReqLookupSingleflight\|TestMemberReqLookupPacer\|TestMemberReqLookupRedisDown)$' ./fuku/modules` | ✅ green |
| 14-03-T3 | 03 | 3 | PLAT-06 | AGENTS.md lines, lint stand-in | `grep -c 'fuku:memberreq:lkp:pace:next' AGENTS.md` | ✅ green |
| 14-04-T1 (tracer) | 04 | 4 | MREQ-12 | `TestMemberReqLinkedChannel`, `TestMemberReqChannelRule` | `-run '^(TestMemberReqLinkedChannel\|TestMemberReqChannelRule)$' ./fuku/modules` | ✅ green |
| 14-04-T2 | 04 | 4 | MREQ-10, MREQ-12 | `TestMemberReqBotStatusClassifier`, `TestMemberReqChannelRule`, `TestMemberReqChannelBreachNeedsBotAdmin` | `-run '^(TestMemberReqBotStatusClassifier\|TestMemberReqChannelRule\|TestMemberReqChannelBreachNeedsBotAdmin)$' ./fuku/modules` | ✅ green |
| 14-04-T3 | 04 | 4 | PLAT-06 | `TestMemberReqLookupSingleflight`, `TestMemberReqLookupCacheKeys` | `-run '^(TestMemberReqLookupSingleflight\|TestMemberReqLookupCacheKeys)$' ./fuku/modules` | ✅ green |
| 14-05-T1 (tracer) | 05 | 5 | MREQ-01, MREQ-11, MREQ-12 | `TestMemberReqRuleTable`, `TestMemberReqBrokenIgnoresLookupRules`, `TestMemberReqStrictest`, `TestMemberReqStrictestLookupBits`, `TestMemberReqPassValue`, `TestMemberReqScreensFitEveryLocale`, `TestSettingsMreqLookupRulesStore`, `TestSettingsMreqRowBudget`, `TestSettingsMreqCallbackBytes` | `-run '^(TestMemberReqRuleTable\|TestSettingsMreqLookupRulesStore\|TestSettingsMreqRowBudget)$' ./fuku/modules` | ✅ green |
| 14-05-T2 | 05 | 5 | MREQ-11 | `TestSettingsMreqPhotoScreen`, `TestMemberReqRuleKeysExist`, `TestSettingsMreqRuleScreen`, `TestSettingsMreqCard`, `TestSettingsStaffPrivacy` | `-run '^(TestSettingsMreqPhotoScreen\|TestMemberReqRuleKeysExist)$' ./fuku/modules` | ✅ green |
| 14-05-T3 | 05 | 5 | MREQ-10 | `TestMemberReqLookupBroken`; the settings family once | `-run '^TestMemberReqLookupBroken$' ./fuku/modules` | ✅ green |
| 14-06-T1 (tracer) | 06 | 6 | MREQ-11 | `TestMemberReqLookupPunishes`, `TestMemberReqLookupSpares` | `-run '^(TestMemberReqLookupPunishes\|TestMemberReqLookupSpares)$' ./fuku/modules` | ✅ green |
| 14-06-T2 | 06 | 6 | MREQ-10, MREQ-12 | `TestMemberReqLookupUnanswerable`, `TestMemberReqLookupDoesNotBlockNameBreach`, `TestMemberReqLookupOrder`, `TestMemberReqLookupNeverReplacesRestriction`, `TestMemberReqLookupCapAndPause` | `-run '^(TestMemberReqLookupUnanswerable\|TestMemberReqLookupOrder)$' ./fuku/modules` | ✅ green |
| 14-06-T3 | 06 | 6 | MREQ-10, PLAT-06 | `TestMemberReqJoinLookups`, `TestMemberReqLookupOnePunishmentAcrossReplicas`, `TestShutdownEndsMemberReqLookups` + the Phase 13 re-run | `-run '^(TestMemberReqJoinLookups\|TestMemberReqLookupOnePunishmentAcrossReplicas\|TestShutdownEndsMemberReqLookups)$' ./fuku/modules` | ✅ green |
| 14-07-T1 (tracer) | 07 | 7 | MREQ-11, MREQ-12 | `TestMemberReqWatcherLookupRules` | `-run '^TestMemberReqWatcherLookupRules$' ./fuku/modules` | ✅ green |
| 14-07-T2 | 07 | 7 | MREQ-10, PLAT-06 | `TestMemberReqWatcherLookupRules`, `TestMemberReqWatcherLookupSingleflight`, `TestMemberReqPassCacheWithLookups`, `TestMemberReqWatcherLookupContinuesGroups` | `-run '^(TestMemberReqWatcherLookupSingleflight\|TestMemberReqPassCacheWithLookups\|TestMemberReqWatcherLookupContinuesGroups)$' ./fuku/modules` | ✅ green |
| 14-07-T3 | 07 | 7 | MREQ-10 | `TestMemberReqLookupHeldPath`, `TestMemberReqHeldRelease` + the Phase 13 re-run | `-run '^(TestMemberReqLookupHeldPath\|TestMemberReqHeldRelease)$' ./fuku/modules` | ✅ green |
| 14-08-T1 (tracer) | 08 | 8 | MREQ-11 | `TestMemberReqNoticePhotoRemedy`, `TestMemberReqStillBrokenText`, `TestMemberReqFixedLookups` | `-run '^(TestMemberReqNoticePhotoRemedy\|TestMemberReqStillBrokenText\|TestMemberReqFixedLookups)$' ./fuku/modules` | ✅ green |
| 14-08-T2 | 08 | 8 | MREQ-10, MREQ-12 | `TestMemberReqFixedApproved`, `TestMemberReqFixedLookups`, `TestMemberReqChannelInert` | `-run '^(TestMemberReqFixedApproved\|TestMemberReqChannelInert)$' ./fuku/modules` | ✅ green |
| 14-08-T3 | 08 | 8 | MREQ-11 | `TestMemberReqFixedNeverLiftsWithLookupRules`, `TestMemberReqScreensFitEveryLocale` + the Phase 13 re-run | `-run '^(TestMemberReqFixedNeverLiftsWithLookupRules\|TestMemberReqScreensFitEveryLocale)$' ./fuku/modules` | ✅ green |
| 14-09-T1 (tracer) | 09 | 9 | MREQ-10 | `TestSettingsMreqChannelStatus` | `-run '^TestSettingsMreqChannelStatus$' ./fuku/modules` | ✅ green |
| 14-09-T2 | 09 | 9 | MREQ-10, MREQ-12 | `TestSettingsMreqChannelStatus`, `TestSettingsMreqBasicGroup`, `TestSettingsMreqOpenCost`, `TestSettingsMreqRedrawKeepsStatus` | `-run '^(TestSettingsMreqBasicGroup\|TestSettingsMreqOpenCost\|TestSettingsMreqRedrawKeepsStatus)$' ./fuku/modules` | ✅ green |
| 14-09-T3 | 09 | 9 | MREQ-10 | `TestSettingsMreqRowBudget`, `TestSettingsMreqCallbackBytes`, `TestSettingsMreqPageFitsEveryLocale`, `TestSettingsMainPageFitsEveryLocale`; the settings family once | `-run '^(TestSettingsMreqRowBudget\|TestSettingsMreqCallbackBytes\|TestSettingsMreqPageFitsEveryLocale)$' ./fuku/modules` | ✅ green |
| 14-10-T1 (tracer) | 10 | 10 | MREQ-12 | `TestSettingsMreqChannelRefusal` | `-run '^TestSettingsMreqChannelRefusal$' ./fuku/modules` | ✅ green |
| 14-10-T2 | 10 | 10 | MREQ-12 | `TestSettingsMreqChannelRefusal`, `TestSettingsMreqCard` | `-run '^(TestSettingsMreqChannelRefusal\|TestSettingsMreqCard)$' ./fuku/modules` | ✅ green |
| 14-10-T3 | 10 | 10 | MREQ-12 | `TestSettingsMreqForgedFields`, `TestSettingsAuthorityMreq`, `TestSettingsDriftMreq`, `TestSettingsControlActionAllowList`, `TestSettingsMreqRuleScreen`; the settings family once | `-run '^(TestSettingsMreqForgedFields\|TestSettingsAuthorityMreq\|TestSettingsDriftMreq)$' ./fuku/modules` | ✅ green |
| 14-11-T1 (tracer) | 11 | 11 | PLAT-06 | `TestMemberReqPacersAreIndependent` | `-run '^TestMemberReqPacersAreIndependent$' ./fuku/modules` | ✅ green |
| 14-11-T2 | 11 | 11 | MREQ-10, MREQ-11, MREQ-12 | `TestMemberReqLookupLogsCarryIDsOnly`, `TestMemberReqLookupAlertsFitEveryLocale`, `TestSettingsStaffPrivacy` | `-run '^(TestMemberReqLookupLogsCarryIDsOnly\|TestMemberReqLookupAlertsFitEveryLocale\|TestSettingsStaffPrivacy)$' ./fuku/modules` | ✅ green |
| 14-11-T3 | 11 | 11 | PLAT-06 | `TestMemberRequirementsStayOutOfBackup` | `-run '^TestMemberRequirementsStayOutOfBackup$' ./fuku/db/backup` | ✅ green |
| 14-12-T1 (tracer) | 12 | 12 | all | `TestMemberReqLookupEndToEnd` | `-run '^TestMemberReqLookupEndToEnd$' ./fuku/modules` | ✅ green |
| 14-12-T2 | 12 | 12 | MREQ-01, MREQ-10 | docs, README, rulebook, requirement notes | `make check-docs` and the greps of the task | ✅ green |
| 14-12-T3 | 12 | 12 | all | the phase gate and `14-SPIKE.md` | `make test` (or the recorded fallback), `make check-translations`, `git diff --exit-code 8c0c827 -- go.mod go.sum` | ✅ green |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [x] `fuku/modules/memberreq_lookup_fake_test.go`: the wrapper over the shared lockdown fake (`getUserProfilePhotos` per user with a count, a null body, scripted errors and a hang that honours `ctx.Done()`; `getChat` with a configured `linked_chat_id`; channel `getChatMember` per (channel, user) and for the bot; calls recorded), `mreqInstallLookupFake`, `mreqChannel`, `mreqLinkChannel`, `withFastLookupPacer(t)`, `mreqLookupCleanup`; hooks that run when a lookup request arrives (14-03, extended by 14-04, 14-06, 14-10).
- [x] Test seams in production code, each read by the code and shortened by a test: `memberReqLookupPacerOptions` and `memberReqLookupPacer` (14-03), `memberReqLookupFlightTimeout` and `memberReqPositiveTTL` (14-03), `memberReqLinkedTTL` and `memberReqBotAdminTTL` (14-04), `memberReqLookupJobBudget` and `memberReqLookupJoinBudget` (14-06), `memberReqFixedLookupBudget` (14-08), `settingsMreqHereTimeout` (14-09); `memberReqRedisClient` exists for the Redis-down tests.
- [x] `TestChatRequirementsSettings` extended to seven columns and `TestMemberRequirementLookupMigrationDefaults` in the anchored `-run` list of `test-postgres-integrity`, both proved on a throwaway PostgreSQL 16 (14-01).
- [x] The fuku/i18n limits tables extended (+4 `mreq_` rows, +12 `settings_` rows, seven phrases, notice cap 520, seven cells) before the keys exist in code (14-02).
- [x] Every Phase 13 test that counted to five edited to seven in 14-05 (the table, the strictest and fingerprint tests, the screens test, the page helpers, grid, rows, bytes, the card and privacy tests), none re-recorded.
- [x] AGENTS.md lines in the same commit as each rule change (14-01, 14-03 to 14-11), read whole once and finished in 14-12.

---

## Manual-Only Verifications

Covered by the owner's live checklist `14-SPIKE.md` (written by plan 14-12; deferred like Phases 7 to 13 unless the owner says otherwise). None blocks the phase, and the code is safe under every outcome of each (an unknown never punishes; D-22 makes a status answer to a non-administrator bot inert).

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| S1: `getUserProfilePhotos(user, 1)` for an account that hides its photo returns zero or an error | MREQ-11 | Live API behaviour (core.telegram.org unreachable during research) | Read it for a hidden-photo account; if an error, reword `settings_mreq_desc_photo`, `mreq_remedy_photo` and `settings_mreq_hint_photo` in place |
| S2: the exact code and description of `getChatMember(channel, user)` and `(channel, bot)` when the bot is not the channel's administrator, and whether any status answer comes back | MREQ-10, MREQ-12 | Live API behaviour | Call both with a bot that is not, and one that was removed as, an administrator; decide whether `cache.IsBotGoneError` can be narrowed |
| S3: `getChat(group).linked_chat_id` reaches a bot that is a plain member of the supergroup | MREQ-12 | Live API behaviour | Call it as a non-administrator bot in a linked supergroup |
| A "public photo" fallback shown while the main photo is hidden | MREQ-11 | Live API behaviour | Hide the main photo, keep a public one, read `getUserProfilePhotos` |
| Member statuses in a channel: subscribed reads member, left reads left, banned reads kicked, an administrator posting as themselves is subscribed | MREQ-12 | Live API behaviour | Read `getChatMember(channel, user)` for each |
| The two new cells and the Off-only screen in hi, ru and uk on a phone are not ellipsized | MREQ-11, MREQ-12 | Real-client rendering | Open the page on Android, iOS and Desktop |
| Two live replicas over one PostgreSQL and one Redis: ten messages from a photo-less account give one punishment and lookups share about 10 a second | PLAT-06, MREQ-11 | Two real replicas | Run two bot instances and post from a violating account |
| A raid: the shared pacer sheds an admin's page open and the job's lookups without delaying any notice; a restart during a burst leaves no hold and punishes nobody | PLAT-06, MREQ-10 | Live API rate limits and a real SIGTERM | Burst joins and messages, watch the log, restart the bot mid-burst |
| Native-speaker read of the 16 new and 7 reworded keys in es, fr, hi, id, pt, ru and uk | MREQ-10, MREQ-11, MREQ-12 | Wording quality is human | Read each key listed in `14-SPIKE.md` check 11 |

---

## Validation Sign-Off

- [x] All tasks have `<automated>` verify or Wave 0 dependencies
- [x] Sampling continuity: no 3 consecutive tasks without automated verify
- [x] Wave 0 covers all MISSING references
- [x] No watch-mode flags
- [x] Feedback latency: a focused `-run` under 60 s; the settings family (about 4.5 minutes) and the `fuku/modules` package (about 11 to 15 minutes) only at plan end or wave merge
- [x] `nyquist_compliant: true` set in frontmatter (by validate-phase, after the plans ran)

**Approval:** validated 2026-11-08 (Nyquist audit, see below)

---

## Validation Audit 2026-11-08

| Metric | Count |
|--------|-------|
| Gaps found | 0 |
| Resolved | 0 (none needed) |
| Escalated | 0 |
| Tests added by the audit | 0 |

Method (done by the orchestrator, no new test was needed):

- Every `Test...` name in the two verification maps (74 tokens, 71 test functions and the three `-run` prefixes `TestLoadSettings`, `TestMemberReq`, `TestSettings`) was looked up as a `func Test...(` in a `_test.go` file: all 71 exist, none is missing.
- All 71 were run through CI's own gate on the Phase 14 tip (after the review fix and security hardening passes): `go test -tags testtools -json -race -count=1 -timeout 30m -run '^(<the 71 names>)$' ./fuku/modules ./fuku/i18n ./fuku/db/memberreq ./fuku/db/migrations ./fuku/db/backup > file.json`, then `go run ./scripts/check_test_results`, exit 0. Result: 70 top-level tests passed, 1 skipped, none failed. The one skip is `TestMemberRequirementLookupMigrationDefaults`, the PostgreSQL-only migration test that the gate allows when no database is configured; CI runs it on PostgreSQL through `make test-postgres-integrity`.
- Review and security fixes added their own tests after this file was seeded (`memberreq_sec_test.go`, the WR-01 to WR-05 RED/GREEN pairs); they are covered by the full-suite gate recorded in `14-REVIEW-FIX.md` and `14-SECURITY-FIX.md` (2412 passes, exit 0) and are not repeated here.
- Manual-only items stay with the owner's live checklist (`14-SPIKE.md`), deferred like Phases 7 to 13.
