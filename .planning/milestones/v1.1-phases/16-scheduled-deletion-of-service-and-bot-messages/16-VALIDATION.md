---
phase: "16"
slug: "scheduled-deletion-of-service-and-bot-messages"
# status lifecycle: draft (seeded by plan-phase) → validated (set by validate-phase §6)
status: validated
nyquist_compliant: true
wave_0_complete: true
created: "2026-11-12"
---

# Phase 16 — Validation Strategy

> Reconstructed (State B) from the nine plans 16-01 to 16-09, their summaries, `16-RESEARCH.md` (Validation Architecture), the review and security fix reports and the test files. The test names are the plans' contract; behaviour claims were checked by reading each test, then by running it.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| Framework | Go `testing`, build tag `testtools`, testify in `fuku/db` tests only, miniredis, hand-written `gotgbot.BotClient` fakes (`lockdownFake`, `deletionScriptClient`, `staffActionFake`), SQLite via `internal/testdb.Run`, PostgreSQL for the integrity list, no mock libraries |
| Config file | none; `Makefile` targets `test` (30 min per package) and `test-postgres-integrity` |
| Quick run, queue and rules | `go test -tags testtools -race -count=1 ./fuku/db/deletion ./fuku/db/backup ./fuku/db/migrations` |
| Quick run, deletion core, watcher, worker, bot sites | `go test -tags testtools -race -count=1 -run '^(TestDeletion\|TestServiceWatcher\|TestServiceKind\|TestCleanService\|TestApply\|TestScheduleBotMessage\|TestBot\|TestWarnThisUserSource\|TestMemberReqNoticeScheduled\|TestMemberReqMuteNoticeAtCap\|TestMemberReqFixedReschedule\|TestValidation)' ./fuku/modules` (about 90 s) |
| Quick run, Deletion page | `go test -tags testtools -race -count=1 -run '^(TestSettingsDeletion\|TestSettingsAuthorityDeletion\|TestSettingsParityCleanService\|TestSettingsDriftDeletion)' ./fuku/modules` |
| Quick run, locales | `go test -tags testtools -race -count=1 ./fuku/i18n` (about 20 s) |
| Full suite command | `make test`, then `make lint`, `make check-translations`, `make generate-docs` then `make check-docs`; CI: `make test-postgres-integrity` |
| CI gate for the new families | `go test -tags testtools -json -race -count=1 -timeout 30m -run '^(<the Phase 16 names>)$' <packages> > file.json` then `go run ./scripts/check_test_results < file.json` (exit 0 at validation: 82 top-level `fuku/modules` tests pass, 0 skips; the db packages pass with the one allowed PostgreSQL-only skip) |
| Deletion globals | `deletionNow`, `deletionPacer`, `deletionBatch`, `deletionChunk`, `deletionWorkerTick`, `deletionInlineTimeout`, `deletionQueueWrite`, the lookups and the latch are package variables swapped through `mreqSwap` and restored in `t.Cleanup`; a test that calls `StopDeletionWorker` calls `deletionWorkerStopped` / `freshDeletionRoot`; no deletion or settings test is parallel |

---

## Sampling Rate

- **Per task commit:** the focused `-run '^(...)$'` of the task's tests plus the package run of any `fuku/db/*` package it touched.
- **Per plan end:** the plan's family run; the settings family (`-run '^(TestSettings|TestLoadSettings)' ./fuku/modules ./fuku/i18n`, about 8 minutes) at the end of 16-07 and 16-08.
- **Per wave merge:** `go test -tags testtools -race -count=1 ./fuku/modules ./fuku/i18n ./fuku/db/...` plus the lint stand-in `python3 .planning/phases/13-member-requirements/handoff/lint-local.py 8c0c827` (exit 0, read directly).
- **Phase gate:** `make test`, `make lint`, `make check-translations`, `make check-docs`, `git diff --exit-code 8c0c827 -- go.mod go.sum`, CI `make test-postgres-integrity`, and the owner's live checklist `16-SPIKE.md` (deferred like Phases 7 to 15).

---

## Per-Requirement Verification Map

Every command is `go test -tags testtools -race -count=1` with the given `-run` and package. 49 requirement claims were checked: all COVERED, 3 of them by tests added in this audit.

| Requirement | Claim | Test Type | Automated Command | Status |
|---|---|---|---|---|
| DEL-01 | 12 service rows (joined, left, photo, title, pinned, video chats, forum topics, boosts, timer, background, giveaways and gifts, other) from one table; each `gotgbot.Message` field is in exactly one of deletable, never-deleted, content (a gotgbot upgrade fails it) | unit | `-run '^(TestServiceKindTable\|TestServiceKindsCoverMessageFields\|TestDeletionKindRegistryMatchesRepository)$' ./fuku/modules` | green |
| DEL-01 | creation, migrate and forum-topic-creation messages and the giveaway post are never matched or deleted | unit | `-run '^TestServiceKindTable$' ./fuku/modules` | green |
| DEL-01 | keep, delete now, delete after N: now is one paced `deleteMessage` and no row; N is one row at date + N; replay keeps one row; the earlier time wins; a delay of 0 writes no row; a failed queue write logs the chat only | integration (real dispatcher) | `-run '^(TestServiceWatcherDeletesNow\|TestServiceWatcherSchedulesAfterN\|TestServiceWatcherZeroWritesNoRow\|TestDeletionEffectiveRule)$' ./fuku/modules` | green |
| DEL-01 | the watcher never ends an update (a recovered panic included), handles bot and anonymous senders | integration | `-run '^(TestServiceWatcherNeverEndsUpdate\|TestServiceWatcherSenders)$' ./fuku/modules` | green |
| DEL-01 | joined and left share storage with `/cleanservice`: flag off is Keep even with a stale row, flag on with no row is now, flag on with a row is the delay; written together in one transaction; `/cleanservice on` and `off` clear both rows; join processing runs before the delete; a bot-caused join or leave is the watcher's | repository + integration | `-run '^(TestApplyJoinLeftRule\|TestApplyCleanServiceRule\|TestCleanServiceEffectiveRule\|TestCleanServiceDeletesAfterJoin\|TestServiceWatcherBotJoin\|TestSettingsParityCleanService)$' ./fuku/db/deletion ./fuku/modules` | green |
| DEL-01 | a chosen delay persists (stored row read back, cache dropped, page reads what the watcher reads, absolute `v`, preset list per section, a stale card state) | integration | `-run '^(TestDeleteRules\|TestSettingsDeletionWrites\|TestSettingsDriftDeletion\|TestSettingsDeletionStaleState\|TestSettingsDeletionValidation\|TestSettingsDeletionStates)$' ./fuku/db/deletion ./fuku/modules` | green |
| DEL-01 | the Service messages page shows its rows with stored states, one Main cell with `{n} on` or the unknown mark | integration | `-run '^(TestSettingsDeletionOpensFromMain\|TestSettingsDeletionMainState\|TestSettingsDeletionReadsAndWritesFail)$' ./fuku/modules` | green |
| DEL-01 | a Staff Group chat is never touched by the watcher | integration | `-run '^TestServiceWatcherSkipsStaffGroup$' ./fuku/modules` | green |
| DEL-02 | five bot kinds (welcome, goodbye, rules, automatic punishments, manual punishments), no other kind (never staff, lockdown, captcha, log or raid) | unit | `-run '^(TestDeletionKindsNeverStaffLockdownCaptchaLog\|TestDeletionKindRegistryMatchesRepository)$' ./fuku/modules` | green |
| DEL-02 | opt-in helper: nil, private, channel, no rule, latch, Staff Group, failed read write nothing; the date clamp; a past date is never stretched | unit/integration | `-run '^(TestScheduleBotMessage\|TestScheduleBotMessageClampsDate)$' ./fuku/modules` | green |
| DEL-02 | welcome, goodbye and `/rules` (group reply and click-to-read) are queued; the private rules message and previews are not | integration | `-run '^(TestBotWelcomeScheduled\|TestBotGoodbyeScheduled\|TestBotRulesScheduled\|TestBotRulesPrivateNeverScheduled)$' ./fuku/modules` | green |
| DEL-02 | the success replies of `/ban /tban /kick /mute /tmute /unban /unmute /warn` and the d-variants are queued as manual; validation replies, lists, `/resetwarns`, `/rmwarn`, question cards and the `/kickme` family are not; `/warn` and the blacklist warn pick their own kind | integration | `-run '^(TestBotManualPunishmentReplies\|TestBotManualRepliesNotCovered\|TestWarnThisUserSource)$' ./fuku/modules` | green |
| DEL-02 | antiflood, blacklist and aispam notices are queued as automatic | integration | `-run '^TestBotAutoPunishmentNotices$' ./fuku/modules` | green |
| DEL-02 | the Bot messages page: Off and the 1, 10, 60 minute, 6 h, 24 h presets persist through the real dispatcher and press pipeline | integration | `-run '^(TestSettingsDeletionWrites\|TestDeletionEndToEnd)$' ./fuku/modules` | green |
| DEL-02 | switching a rule off drops its pending rows (WR-03) and an acting row, another kind or chat is kept | integration | `-run '^(TestApplyDeleteRuleRemoveDropsPendingRows\|TestApplyCleanServiceRuleKeepDropsPendingRows\|TestApplyCleanServiceFlagOffDropsPendingRows)$' ./fuku/modules` | green |
| DEL-06 | restart: stop mid-call leaves every row pending with no token and no attempt; a crashed `acting` row with an old claim is released; each row is deleted exactly once after the restart | integration | `-run '^(TestDeletionWorkerResumesAfterRestart\|TestDeletionWorkerStaleClaimNotDoubleProcessed\|TestReleaseStaleClaims)$' ./fuku/modules ./fuku/db/deletion` | green |
| DEL-06 | once across replicas: eight concurrent claims win disjoint sets; two worker replicas delete each of 40 messages exactly once | integration (SQLite and PostgreSQL) | `-run '^(TestClaimBatchOneWinner\|TestDeletionWorkerOneDeleteAcrossReplicas)$' ./fuku/db/deletion ./fuku/modules` | green |
| DEL-06 | scheduling one message twice keeps the earliest `delete_at` (table-qualified `LEAST`) | repository (SQLite and PostgreSQL) | `-run '^TestScheduleKeepsEarliest$' ./fuku/db/deletion` | green |
| DEL-06 | delays are capped at 86400 s (0 to 86400 accepted, -1 and 86401 refused with no row; DB CHECK 0..86400); the stored rule range is checked before any write | unit + repository | `-run '^(TestDeletionDelayCapped\|TestScheduleRules\|TestDeleteRules)$' ./fuku/modules ./fuku/db/deletion` | green |
| DEL-06 | a row over 47 hours past the message's own date is dropped quietly with no request and no ID in any log; younger rows go out in one batch | integration | `-run '^(TestDeletionWorkerDropsOldRows\|TestDeletionWorkerLogsNoMessageIDs\|TestDeletionLogsCarryIDsOnly)$' ./fuku/modules` | green |
| DEL-06 | late messages: `delete_at` is date + delay, never counted from arrival; a bot message's past date is kept; a delayed service message is due at once and a 50 hour old one is dropped by the worker | integration | `-run '^(TestScheduleBotMessageClampsDate\|TestValidationServiceMessageDeliveredLate)$' ./fuku/modules` | green (new) |
| DEL-06 | a refused inline delete, a 429 or a flood-control wait over the inline deadline queues the message instead of losing it (WR-01); a counted retry backs off a minute (WR-02) | integration | `-run '^(TestDeletionInlineCapFallsBackToQueue\|TestDeletionInlineShedFallsBackToQueue\|TestDeletionInlineDeadlineFallsBackToQueue\|TestDeletionWorkerClassifiesErrors\|TestDeletionWorkerRateLimitedFree)$' ./fuku/modules` | green |
| DEL-07 | layer 1, call sites: a Staff Group action card and summary, a lockdown alert copy, a captcha challenge and a log-channel post get no row and no delete, also a day and a half later after a worker cycle | integration | `-run '^TestDeletionExcludedMessagesNeverScheduled$' ./fuku/modules` | green |
| DEL-07 | layer 2, writers: `deletion.Schedule` refuses a Staff Group, a private chat and non-negative IDs (IN-05, SEC-05); the watcher, the inline path and the worker skip a Staff Group; an unreadable Staff Group read counts as one | repository + integration | `-run '^(TestScheduleRefusesStaffGroup\|TestScheduleRules\|TestDeletionActRefusesNonNegativeChat\|TestServiceWatcherSkipsStaffGroup\|TestDeletionWorkerSkipsStaffGroup)$' ./fuku/db/deletion ./fuku/modules` | green |
| DEL-07 | layer 3, page: the Deletion page is not drawn in a Staff Group, a forged press is refused and writes nothing, a failed read hides nothing and fails closed | integration | `-run '^(TestSettingsDeletionHidden\|TestSettingsDeletionStaffGroup)$' ./fuku/modules` | green |
| DEL-07 | the tables never travel in backup, export, import or reset | integration (SQLite) | `-run '^TestDeletionTablesStayOutOfBackup$' ./fuku/db/backup` | green |
| DEL-08 | the no-delete-right line is on every deletion screen (hub, both lists, every row screen, a private menu), once, in its slot, never changes row widths, and the press is answered before the live read; creator and an administrator with the right draw none | integration (table of 17 bot shapes) | `-run '^(TestSettingsDeletionRightLine\|TestDeletionBotRightClassifier\|TestDeletionReadBotRight)$' ./fuku/modules` | green |
| DEL-08 | an unreadable answer (429, 5xx, transport error, timeout, shed, null) draws the unknown line, never an error, refusal or the cannot-open edit; one read per draw | integration | `-run '^(TestSettingsDeletionRightLine\|TestSettingsDeletionCost)$' ./fuku/modules` | green |
| DEL-08 | a plain-member bot: the warning on every screen, the service latch only after a live check, bot kinds keep deleting; a supergroup administrator's flag is trusted (SEC-02) | integration | `-run '^(TestDeletionServiceLatchNeedsLiveCheck\|TestDeletionWorkerTrustsAdminFlagInSupergroups\|TestDeletionInlineRefusalDecidesLatch\|TestDeletionLatch\|TestDeletionWorkerLatchedChat\|TestDeletionEndToEnd)$' ./fuku/modules` | green |
| MREQ-15 | every requirement notice (warn, kick, ban, mute, the cap's pause notice) is queued by the automatic-punishments timer; kind off writes no row; a failed send writes none; a retried notice counts from its own date | integration | `-run '^(TestMemberReqNoticeScheduled\|TestDeletionEndToEnd)$' ./fuku/modules` | green |
| MREQ-15 | a mute notice with the button is queued at the 24 h cap whatever the timer says; the tap reschedules the edited notice from the tap and moves the row up, never later; a refused tap, a failed edit, a still-broken tap schedules nothing; a button on another message reschedules nothing (SEC-01) | integration | `-run '^(TestMemberReqMuteNoticeAtCap\|TestMemberReqFixedRescheduleMovesUp\|TestMemberReqFixedReschedulesOnlyTheNotice)$' ./fuku/modules` | green |
| PLAT-05 | 57 new strings in all 8 locales: keys exist and are not empty, ceilings and cell widths per language, every deletion screen within 3800 UTF-16 units | locale + integration | `-run '^(TestDeletionKindKeysExist\|TestSettingsDeletionPageFitsEveryLocale\|TestSettingsStaffPrivacy)$' ./fuku/modules` and `./fuku/i18n` (whole package) | green |
| PLAT-06 | pacing shared across replicas through `fuku:delete:pace:*` and independent of the member requirements, lockdown and staff pacers | integration | `-run '^(TestDeletionPacerIsIndependent\|TestDeletionWorkerRateLimitedFree)$' ./fuku/modules` | green |
| PLAT-06 | recovers from crashes: a panic in a worker cycle is recovered and its rows go back free, a panic in the inline delete releases its slot, a panic in the watcher returns `ext.ContinueGroups` | integration | `-run '^(TestValidationWorkerCycleRecoversFromPanic\|TestValidationInlineDeleteRecoversFromPanic\|TestServiceWatcherNeverEndsUpdate)$' ./fuku/modules` | green (new) |
| PLAT-06 | drained inside the shutdown budget: a cycle inside a hung call is ended by the stop, its rows go back to pending and a restart resumes them; Start and Stop are idempotent | integration | `-run '^(TestShutdownEndsDeletionWorker\|TestDeletionWorkerStartStopIdempotent)$' ./fuku/modules` | green |

### Decision-level guarantees

| Guarantee | Test | Status |
|---|---|---|
| Queue SQL that needs PostgreSQL: `uk_scheduled_deletions_message`, the partial indexes, `chk_*` CHECKs (delay 86401, state `x`, negative attempts), idempotent migration statements, the earliest-time `LEAST` upsert, one-winner claims | `TestScheduledDeletionMigrationDefaults` (`./fuku/db/migrations`), `TestScheduleKeepsEarliest`, `TestClaimBatchOneWinner` (`./fuku/db/deletion`); all three are in `make test-postgres-integrity`'s `-run` list and its package list (`./fuku/db/migrations`, `./fuku/db/deletion`) | green; all three also run on a throwaway PostgreSQL 16 at validation with no skip |
| WR-02 counted-release statement and WR-03 `kind IN ?` drop on PostgreSQL | the whole `./fuku/db/deletion` package was run on PostgreSQL 16 at validation (9 tests pass); `TestReleaseStaleClaims`, `TestApplyJoinLeftRule` and `TestScheduleRules` are not in the Makefile list | green once; see Validation Audit, warning W-1 |
| `delete_rules:*` is in `skipLocal` (a rule switched off stops acting on every replica at the next message) | `TestSkipLocalMatchesFreshnessCriticalKeys` (`./fuku/db/cache`, prefix table) | green |
| Callback bytes (at most 64) and rows | `TestSettingsDeletionCallbackBytes`, `TestSettingsDeletionRowBudget`, `TestSettingsDeletionNoCard` | green |
| 8-language fit of every page and cell | `TestSettingsDeletionPageFitsEveryLocale`, `TestSettingsLabelLengths` and the limits table (`./fuku/i18n`), `TestDeletionKindKeysExist` | green |
| Authority parity: the menu refuses what `/cleanservice`, `/cleanwelcome` and `/cleangoodbye` refuse | `TestSettingsAuthorityDeletion`, `TestSettingsParityCleanService` | green |
| Staff privacy of every Deletion screen in 8 languages | `TestSettingsStaffPrivacy` (Deletion hub, lists and all 17 row screens, a private run by a non-creator administrator) | green |
| Review and security fixes each have a RED/GREEN test | WR-01 `TestDeletionInlineDeadlineFallsBackToQueue`; WR-02 `TestReleaseStaleClaims` and `TestDeletionWorkerClassifiesErrors`; WR-03 `TestApply*DropsPendingRows`; IN-01 `TestBotManualRepliesNotCovered`; IN-05 `TestScheduleRules`; SEC-01 `TestMemberReqFixedReschedulesOnlyTheNotice`; SEC-02 `TestDeletionInlineRefusalDecidesLatch` and `TestDeletionWorkerTrustsAdminFlagInSupergroups`; SEC-05 `TestDeletionActRefusesNonNegativeChat` | green |

---

## Per-Task Verification Map

| Plan | Requirements | Tests | Automated Command | Status |
|------|--------------|-------|-------------------|--------|
| 16-01 (tracer: migration, models, repository, cache, backup) | DEL-01, DEL-06, DEL-07 | `TestScheduleRules`, `TestScheduleKeepsEarliest`, `TestScheduleRefusesStaffGroup`, `TestClaimBatchOneWinner`, `TestCompleteIsTokenGuarded`, `TestReleaseStaleClaims`, `TestDeleteRules`, `TestKindRegistry`, `TestApplyJoinLeftRule`, `TestScheduledDeletionMigrationStatements`, `TestScheduledDeletionMigrationDefaults`, `TestDeletionTablesStayOutOfBackup` | `./fuku/db/deletion ./fuku/db/migrations ./fuku/db/backup` | green |
| 16-02 (kind table, effective rule) | DEL-01, DEL-02, DEL-07 | `TestDeletionEffectiveRule`, `TestServiceKind*`, `TestDeletionKindRegistryMatchesRepository`, `TestDeletionKindsNeverStaffLockdownCaptchaLog`, `TestDeletionKindKeysExist` | `-run` as in the map, `./fuku/modules` | green |
| 16-03 (service watcher, `cleanService`, apply functions) | DEL-01, DEL-06, DEL-07 | `TestServiceWatcher*`, `TestCleanService*`, `TestApplyDeleteRule`, `TestApplyCleanServiceRule` | `./fuku/modules` | green |
| 16-04 (core, worker, latch, right reading, shutdown) | DEL-06, DEL-07, DEL-08, PLAT-06 | `TestDeletionClassify`, `TestDeletionLatch`, `TestDeletionInline*`, `TestDeletionPacerIsIndependent`, `TestDeletionDelayCapped`, `TestDeletionWorker*`, `TestDeletionBotRightClassifier`, `TestDeletionReadBotRight`, `TestDeletionServiceLatchNeedsLiveCheck`, `TestShutdownEndsDeletionWorker` | `./fuku/modules` | green |
| 16-05 (bot-message sites) | DEL-02, DEL-07 | `TestScheduleBotMessage*`, `TestBotWelcomeScheduled`, `TestBotGoodbyeScheduled`, `TestBotRulesScheduled`, `TestBotRulesPrivateNeverScheduled`, `TestBotManualPunishmentReplies`, `TestBotManualRepliesNotCovered`, `TestWarnThisUserSource` | `./fuku/modules` | green |
| 16-06 (automatic notices, MREQ-15) | DEL-02, DEL-07, MREQ-15 | `TestBotAutoPunishmentNotices`, `TestMemberReqNoticeScheduled`, `TestMemberReqMuteNoticeAtCap`, `TestMemberReqFixedRescheduleMovesUp` | `./fuku/modules` | green |
| 16-07 (Deletion page, right line, locales) | DEL-01, DEL-02, DEL-08, PLAT-05 | `TestSettingsDeletion*`, `TestSettingsAuthorityDeletion`, `TestSettingsParityCleanService`, `TestSettingsDriftDeletion`, i18n limits tables | `./fuku/modules ./fuku/i18n` | green |
| 16-08 (page wiring, Staff Group hide, privacy) | DEL-01, DEL-02, DEL-07, DEL-08, MREQ-15 | `TestSettingsDeletionHidden`, `TestSettingsDeletionStaffGroup`, `TestSettingsStaffPrivacy` (Deletion cases) | `./fuku/modules` | green |
| 16-09 (closing sweeps, e2e, AGENTS.md) | all six, PLAT-05, PLAT-06 | `TestDeletionEndToEnd`, `TestDeletionExcludedMessagesNeverScheduled`, `TestDeletionLogsCarryIDsOnly`, the CI gate | `./fuku/modules` | green |
| Validation audit | DEL-06, PLAT-06 | `TestValidationServiceMessageDeliveredLate`, `TestValidationWorkerCycleRecoversFromPanic`, `TestValidationInlineDeleteRecoversFromPanic` (`deletion_validation_test.go`, commit 32544cf) | `-run '^TestValidation' ./fuku/modules` | green |

---

## Wave 0 Requirements

- [x] `fuku/db/deletion/queue_test.go`, `rules_test.go`, `testmain_test.go` (queue and rules repository)
- [x] `fuku/db/migrations/scheduled_deletion_migration_test.go`, `fuku/db/backup/deletion_backup_test.go`
- [x] `fuku/modules/deletion_env_test.go` (`newDeletionEnv`, `deletionScriptClient`, `seedDue`, `replica`), `settings_deletion_env_test.go`
- [x] `fuku/modules/deletion_*_test.go`, `settings_deletion*_test.go`, `memberreq_notice_schedule_test.go`
- [x] `AutoMigrate` lists (module harness, `fuku/db/deletion`, `fuku/db/backup`) carry `DeleteRule` and `ScheduledDeletion`

---

## Manual-Only Verifications

All live-Telegram checks are the owner's checklist `16-SPIKE.md`, deferred like Phases 7 to 15; the code is safe under every outcome (a batch that fails falls back to per-ID calls, an unrecognised refusal drops that one row, an unreadable right draws the unknown mark).

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Batch delete: one undeletable ID inside `deleteMessages` (does Telegram fail the call or skip the ID) | DEL-06 | live Bot API behaviour | 16-SPIKE.md check 1 |
| The delete-right line for a plain-member bot and for a basic group's administrator (`can_delete_messages` on a basic group) | DEL-08 | what `getChatMember` returns is Telegram's | 16-SPIKE.md check 2 |
| Phone-width cells in Hindi, Russian and Ukrainian | PLAT-05 | needs a real client | 16-SPIKE.md check 3 |
| Two replicas against one queue, live | DEL-06 | tests prove it on SQLite and PostgreSQL with fakes | 16-SPIKE.md check 4 |
| A restart with queued rows, live | DEL-06 | needs a real deploy | 16-SPIKE.md check 5 |
| Native-speaker read of the 57 new strings | PLAT-05 | language quality | 16-SPIKE.md check 6 |
| Which service message types Telegram really lets a bot delete; forum topics and giveaways live | DEL-01 | live Bot API behaviour | 16-SPIKE.md checks 9 and 10 |
| The joined and left coupling through one flag, live | DEL-01 | end-to-end with real join messages | 16-SPIKE.md check 10 |
| The real error strings of a refused delete and a 429 on `deleteMessages` | DEL-06 | classifier fragments are `[ASSUMED]` | 16-SPIKE.md check 11 |
| DEL-07 live: nothing excluded is ever deleted | DEL-07 | confirmation in a real group | 16-SPIKE.md check 12 |
| CI: lint, the full test run and `make test-postgres-integrity` | all | CI is the authority | 16-SPIKE.md check 14 |

---

## Validation Sign-Off

- [x] All requirement claims have an automated verification
- [x] Sampling continuity: every plan has focused commands
- [x] Wave 0 covers all missing references
- [x] No watch-mode flags
- [x] Feedback latency: focused family about 90 s; settings family about 8 minutes
- [x] `nyquist_compliant: true` set in frontmatter

**Approval:** validated 2026-11-12

---

## Validation Audit 2026-11-12

| Metric | Count |
|--------|-------|
| Requirement claims checked | 49 (DEL-01: 9, DEL-02: 7, DEL-06: 9, DEL-07: 5, DEL-08: 3, MREQ-15: 2, PLAT-05: 1, PLAT-06: 4, plus 9 decision-level guarantees) |
| Gaps found | 3 (all MISSING behaviours, none PARTIAL) |
| Resolved | 3 |
| Escalated | 0 |
| Manual-only | 11 |

Gaps and resolutions (commit 32544cf, `fuku/modules/deletion_validation_test.go`):
1. DEL-06 late messages through the service watcher (only the bot helper's date clamp was asserted): `TestValidationServiceMessageDeliveredLate`.
2. PLAT-06 crash recovery of the worker cycle: `TestValidationWorkerCycleRecoversFromPanic`.
3. PLAT-06 crash recovery of the inline "delete now" goroutine (slot released, next notice still deleted): `TestValidationInlineDeleteRecoversFromPanic`.

Runs: 82 top-level `fuku/modules` tests of the Phase 16 families through CI's own gate (exit 0, 0 skips); `./fuku/db/deletion`, `./fuku/db/migrations`, `./fuku/db/backup` through the gate (exit 0, one allowed PostgreSQL-only skip); `./fuku/i18n` whole package; the whole `./fuku/db/deletion` package plus `TestScheduledDeletionMigration*` on a throwaway PostgreSQL 16 (migration chain applied first), no skip, cluster removed afterwards; the lint stand-in exit 0; `gofmt -l` clean.

Warnings:
- **W-1.** `make test-postgres-integrity` lists only `TestScheduleKeepsEarliest`, `TestClaimBatchOneWinner` and `TestScheduledDeletionMigrationDefaults`. The review-fix statements (WR-02's counted release setting `delete_at` from a bound time, WR-03's `kind IN ?` drop inside the rule transactions) are exercised on SQLite by CI. They passed on PostgreSQL 16 here (whole `./fuku/db/deletion` package), but a regression would not be caught by CI's PostgreSQL step. Suggested follow-up (the Makefile is not a test file, so not changed here): add `TestReleaseStaleClaims|TestApplyJoinLeftRule|TestDeleteRules` to its `-run` list (`./fuku/db/deletion` is already in the package list).
- **W-1 resolved by the orchestrator (2026-11-12):** `TestReleaseStaleClaims`, `TestDeleteRules` and `TestApplyJoinLeftRule` were added to the `-run` list of `make test-postgres-integrity` (all three are in `./fuku/db/deletion`, already in the package list, and passed on PostgreSQL 16 in the security and validation audits), and AGENTS.md's Phase 16 guarantees bullet names them; CI now proves the review-fix SQL on PostgreSQL.
- **W-2.** The module-level deletion tests run on SQLite only (the `fuku/modules` harness does not run on PostgreSQL, as in Phases 14 and 15).
- **W-3.** The settings family (`-run '^(TestSettings|TestLoadSettings)'`, about 8 minutes) was not run in full; the Deletion page tests, `TestSettingsStaffPrivacy` and the i18n package were.
