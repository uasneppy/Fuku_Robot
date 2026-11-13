---
phase: 16-scheduled-deletion-of-service-and-bot-messages
verified: 2026-11-12T06:10:00Z
status: human_needed
score: "12/12 must-haves verified (5 ROADMAP success criteria plus 7 supporting truths); DEL-01, DEL-02, DEL-06, DEL-07, DEL-08 and MREQ-15 judged satisfied, so the ticks of plan 16-09 are true; PLAT-05 and PLAT-06 judged still satisfied"
covered_files:
  - ".planning/phases/16-scheduled-deletion-of-service-and-bot-messages/16-01-PLAN.md"
  - ".planning/phases/16-scheduled-deletion-of-service-and-bot-messages/16-01-SUMMARY.md"
  - ".planning/phases/16-scheduled-deletion-of-service-and-bot-messages/16-02-PLAN.md"
  - ".planning/phases/16-scheduled-deletion-of-service-and-bot-messages/16-02-SUMMARY.md"
  - ".planning/phases/16-scheduled-deletion-of-service-and-bot-messages/16-03-PLAN.md"
  - ".planning/phases/16-scheduled-deletion-of-service-and-bot-messages/16-03-SUMMARY.md"
  - ".planning/phases/16-scheduled-deletion-of-service-and-bot-messages/16-04-PLAN.md"
  - ".planning/phases/16-scheduled-deletion-of-service-and-bot-messages/16-04-SUMMARY.md"
  - ".planning/phases/16-scheduled-deletion-of-service-and-bot-messages/16-05-PLAN.md"
  - ".planning/phases/16-scheduled-deletion-of-service-and-bot-messages/16-05-SUMMARY.md"
  - ".planning/phases/16-scheduled-deletion-of-service-and-bot-messages/16-06-PLAN.md"
  - ".planning/phases/16-scheduled-deletion-of-service-and-bot-messages/16-06-SUMMARY.md"
  - ".planning/phases/16-scheduled-deletion-of-service-and-bot-messages/16-07-PLAN.md"
  - ".planning/phases/16-scheduled-deletion-of-service-and-bot-messages/16-07-SUMMARY.md"
  - ".planning/phases/16-scheduled-deletion-of-service-and-bot-messages/16-08-PLAN.md"
  - ".planning/phases/16-scheduled-deletion-of-service-and-bot-messages/16-08-SUMMARY.md"
  - ".planning/phases/16-scheduled-deletion-of-service-and-bot-messages/16-09-PLAN.md"
  - ".planning/phases/16-scheduled-deletion-of-service-and-bot-messages/16-09-SUMMARY.md"
  - "AGENTS.md"
  - "Makefile"
  - "README.md"
  - "docs/src/content/docs/api-reference/callbacks.md"
  - "docs/src/content/docs/api-reference/permissions.md"
  - "docs/src/content/docs/architecture/handler-groups.md"
  - "fuku/db/cache/local.go"
  - "fuku/db/deletion/hook_testtools.go"
  - "fuku/db/deletion/kinds.go"
  - "fuku/db/deletion/queue.go"
  - "fuku/db/deletion/rules.go"
  - "fuku/db/greetings/repository.go"
  - "fuku/db/models/delete_rule.go"
  - "fuku/db/models/scheduled_deletion.go"
  - "fuku/modules/aispam.go"
  - "fuku/modules/antiflood.go"
  - "fuku/modules/bans.go"
  - "fuku/modules/blacklists.go"
  - "fuku/modules/deletion_apply.go"
  - "fuku/modules/deletion_bot.go"
  - "fuku/modules/deletion_kinds.go"
  - "fuku/modules/deletion_right.go"
  - "fuku/modules/deletion_schedule.go"
  - "fuku/modules/deletion_service.go"
  - "fuku/modules/deletion_worker.go"
  - "fuku/modules/greetings.go"
  - "fuku/modules/memberreq_fixed.go"
  - "fuku/modules/memberreq_notice.go"
  - "fuku/modules/moderation.go"
  - "fuku/modules/mute.go"
  - "fuku/modules/rules.go"
  - "fuku/modules/settings_deletion.go"
  - "fuku/modules/settings_edit_preview.go"
  - "fuku/modules/settings_menu.go"
  - "fuku/modules/settings_pages.go"
  - "fuku/modules/warns.go"
  - "locales/en.yml"
  - "locales/es.yml"
  - "locales/fr.yml"
  - "locales/hi.yml"
  - "locales/id.yml"
  - "locales/pt.yml"
  - "locales/ru.yml"
  - "locales/uk.yml"
  - "main.go"
  - "migrations/20261110120000_add_scheduled_deletion.sql"
covered_digest: "v4:sha256:f0b6adf185e1921e028d48c686f4d200d0a780a756cdffeba9c4c732bce5f4d5"
behavior_unverified: 0
overrides_applied: 0
human_verification:
  - test: "SPIKE check 1: the batch delete. In a test supergroup, send a `deleteMessages` batch where one ID is too old or cannot be deleted."
    expected: "Telegram's real answer is what the per-ID fallback expects (the batch fails or partly succeeds, and each ID is then isolated by its own call)."
    why_human: "Needs a live Telegram group. The code is safe under every outcome: a batch that fails for a reason of its own falls back to one call per message, and an unknown answer deletes nothing."
  - test: "SPIKE check 2: the delete-right line. Open the Deletion page with the bot as a plain member, as an administrator without the delete right, and in a basic group where an administrator reads can_delete_messages false."
    expected: "The red line shows for a definite cannot, the unknown line when the right cannot be established, and nothing when the bot can delete. No button is disabled."
    why_human: "What the real Bot API returns for the bot's own record in a basic group versus a supergroup needs a live group. The classifier, the flag trust by chat ID range and the unknown fallback are proven against fakes."
  - test: "SPIKE check 3: phone-width cells in Hindi, Russian and Ukrainian on the Deletion hub, lists and row screens."
    expected: "No cell is cut off or wraps unreadably."
    why_human: "Visual check on a real phone. Code points are bounded by TestSettingsLabelLengths and the 64-byte callback limit by TestSettingsDeletionCallbackBytes, which cannot see the rendered width."
  - test: "SPIKE check 4: two replicas over one PostgreSQL and Redis. Set a rule on one, send a pin handled by the other, then switch the rule to Keep."
    expected: "One deletion of the one message, and no row written after the switch to Keep."
    why_human: "Needs two live processes. One winner per row is proven by TestClaimBatchOneWinner and TestDeletionWorkerOneDeleteAcrossReplicas, and the rule key is in skipLocal."
  - test: "SPIKE check 5: a restart with queued rows (SIGTERM, then kill -9 during a cycle)."
    expected: "Rows resume after a clean stop, and a claim held by a killed worker is released and deleted within about 2 minutes, with no double deletion."
    why_human: "Needs a real process and real signals. Resume is proven in tests (TestDeletionWorkerResumesAfterRestart, TestDeletionWorkerStaleClaimNotDoubleProcessed, TestShutdownEndsDeletionWorker)."
  - test: "SPIKE check 6: a native-speaker read of the 57 new locale strings in all 8 locales."
    expected: "The wording reads naturally and means what the English says."
    why_human: "Language quality cannot be checked by a program. Presence, length and markup are guarded by make check-translations, the limits tables and TestSettingsDeletionPageFitsEveryLocale."
  - test: "SPIKE check 7: confirm the planner assumptions A16-A2, A16-A3, A16-A4, A16-A6 and A16-A8."
    expected: "The owner confirms or changes each starting value."
    why_human: "Owner decisions."
  - test: "SPIKE check 8: confirm the amendments D-20 to D-28."
    expected: "The owner accepts or changes each amendment."
    why_human: "Owner decisions, the standing policy of this run."
  - test: "SPIKE check 9: forum topics and Giveaways, live."
    expected: "The service types the table lists as forum and giveaway are really sent and really deletable by the bot."
    why_human: "Needs a live forum supergroup. TestServiceKindsCoverMessageFields proves the table covers every service field of the pinned gotgbot version."
  - test: "SPIKE check 10: the joined and left coupling through one flag (/cleanservice and the Deletion page on a real group)."
    expected: "Joined and left follow /cleanservice exactly, with the shared flag and the stored delay."
    why_human: "Live confirmation. The coupling is proven by TestSettingsParityCleanService, TestCleanServiceEffectiveRule and TestApplyCleanServiceRule."
  - test: "SPIKE check 11: the real error strings of a refused delete, and a 429 on deleteMessages."
    expected: "The classifier's fragments match the real descriptions, or an unmatched one falls to refused or retry."
    why_human: "Needs live Telegram. The classifier matches by code and by several lower-cased fragments, never one string, and places everything it cannot read as a retry."
  - test: "SPIKE check 12: DEL-07 live. Trigger a Staff Group card, a lockdown alert, a captcha challenge and a log-channel post with every rule at its shortest delay."
    expected: "None of them is ever deleted."
    why_human: "Live confirmation. Proven in the fake-driven TestDeletionExcludedMessagesNeverScheduled, which drives the four real producers."
  - test: "SPIKE check 14: read the CI run of the pushed head: make lint, make test and make test-postgres-integrity."
    expected: "All green, including the Phase 16 PostgreSQL names TestScheduleKeepsEarliest, TestClaimBatchOneWinner, TestScheduledDeletionMigrationDefaults, and the review-fix names TestReleaseStaleClaims, TestDeleteRules, TestApplyJoinLeftRule."
    why_human: "make lint cannot run in this sandbox and the fuku/modules harness does not run on PostgreSQL. CI of head 6e02f16 passed, and CI of head 76163a0 was still running when this report was written. The three PostgreSQL names added after 6e02f16 (commit 5fe0e8a) have no completed CI run yet."
audit_acknowledged:
  milestone: v1.1
  at: 2026-11-13
  status: human_needed
---

# Phase 16: Scheduled Deletion of Service and Bot Messages Verification Report

**Phase Goal:** As a group owner, I want Telegram's service messages and the bot's own messages removed on a schedule I set, so that the chat stays readable.
**Verified:** 2026-11-12T06:10:00Z
**Status:** human_needed
**Re-verification:** No, initial verification

The phase is MVP mode. The goal is a user story, and its outcome clause ("the chat stays readable", by a schedule the owner sets) is the thing traced below: an admin sets a delay on a page, the matching message is deleted at that delay, and the exclusions and the right-to-delete line hold.

## Goal Achievement

### User Flow Coverage

| Step of the story | Expected | Evidence in the code | Status |
| ----------------- | -------- | -------------------- | ------ |
| The owner sets a delay | The Deletion page writes a rule per kind | `fuku/modules/settings_deletion.go` (page `d`, controls from the row tables, `applyDeleteRule` and `applyCleanServiceRule`), `fuku/db/deletion/rules.go` (`SetRule` with `cache.DeleteCache`) | VERIFIED |
| A service message arrives | The group `-5` watcher or `cleanService` applies the effective rule | `fuku/modules/deletion_service.go` (`serviceDeleteWatcher`, `deletionApplyMessage`), `fuku/modules/greetings.go` (`cleanService` calls `deletionApplyMessage` after join processing) | VERIFIED |
| The bot sends its own message | The named send site queues it for the stored delay | `scheduleBotMessage` at welcome, goodbye, rules, manual replies, warn, antiflood, blacklist, aispam notice and requirement notices | VERIFIED |
| The time comes | The worker deletes it once, on any replica | `fuku/modules/deletion_worker.go`, `fuku/db/deletion/queue.go` (`ClaimDue`) | VERIFIED |
| The bot cannot delete | Every deletion screen says so | `settingsDelReadRight` on hub, lists and row screens | VERIFIED |

### Observable Truths

| #  | Truth | Status | Evidence |
| -- | ----- | ------ | -------- |
| 1 | SC1: A Service messages page sets each service type to keep, delete now or delete after N seconds; types include joined, left, new photo, new title, pinned, forum topics, boosts, video chats, auto-delete timer and the rest; joined and left share storage with `/cleanservice` | VERIFIED | `deletionServiceRows` in `fuku/modules/deletion_kinds.go` has 12 rows (`sj sl sp sh sn sv sf sb sa sk sg so`), each with a predicate over the gotgbot message and presets 0, 10 s, 1 m, 10 m, 1 h, 6 h, 24 h. `deletionEffectiveDelay` makes the `/cleanservice` flag the on/off switch of joined and left and the row only the delay. `applyCleanService` (`greetings.go`) and `deletion.ApplyCleanServiceFlag` write flag and rows in one transaction. Tests: `TestServiceKindTable`, `TestServiceKindsCoverMessageFields`, `TestDeletionEffectiveRule`, `TestApplyCleanServiceRule`, `TestSettingsParityCleanService`, `TestCleanServiceDeletesAfterJoin`, `TestServiceWatcherDeletesNow`, `TestServiceWatcherSchedulesAfterN` (all passed) |
| 2 | SC2: A Bot messages page sets each kind of the bot's own message to keep or delete after N minutes: welcome, goodbye, rules, automatic punishments (requirement notices included) and replies to manual `/ban`, `/mute`, `/kick`, `/warn` | VERIFIED | `deletionBotRows` has `bw bg br ba bm` with presets 1 m to 24 h. Hooks in code: `greetings.go:973,1065`, `rules.go:375,387`, `moderation.go:40` (`replySuccess`, 5 sites in `bans.go`, 3 in `mute.go`), `warns.go:327` (source picks manual or auto kind), `antiflood.go:422`, `blacklists.go:528,551,573`, `aispam.go:369`, `memberreq_notice.go:219,222`. Tests: `TestBotWelcomeScheduled`, `TestBotGoodbyeScheduled`, `TestBotRulesScheduled`, `TestBotManualPunishmentReplies`, `TestWarnThisUserSource`, `TestBotAutoPunishmentNotices` (passed) |
| 3 | SC3: Scheduled deletions survive a restart and run once across replicas; delays are capped below Telegram's 48 hours; a message too old to delete is dropped quietly | VERIFIED | Durable queue `scheduled_deletions` (migration `20261110120000`). `ClaimDue` is one conditional `UPDATE ... AND state = 'pending'` read back by token, so each row has one winner. `Complete` and `Release` act only on the caller's token. `ReleaseStale` returns claims over 2 minutes old free. Delay capped by Go (`deletion.MaxDelaySecs` 86400, checked in `Schedule`, `SetRule` and `deletionSchedule`) and by the named CHECK `chk_chat_delete_rules_delay`. The worker drops a row whose `sent_at` is over 47 hours old with no call (`deletionDropAge`). Behavioral tests ran: `TestDeletionWorkerResumesAfterRestart`, `TestDeletionWorkerOneDeleteAcrossReplicas`, `TestDeletionWorkerStaleClaimNotDoubleProcessed`, `TestDeletionWorkerDropsOldRows`, `TestDeletionDelayCapped`, `TestShutdownEndsDeletionWorker`; `./fuku/db/deletion` passed on SQLite (`TestScheduleKeepsEarliest`, `TestClaimBatchOneWinner`); the PostgreSQL run is CI's (see human item 14) |
| 4 | SC4: Deletion never touches Staff Group summaries and cards, lockdown alerts, captcha challenges, log-channel posts or anything in a Staff Group | VERIFIED | Four independent guards. (a) Opt-in: only the named send sites call `scheduleBotMessage`; nothing hooks `SendMessageWithErrorHandling`, `media.Send` or the bot client, and `deletionBotRows` has no kind for staff, lockdown, captcha, log or raid. (b) `deletionAct` returns for a Staff Group or one that cannot be read, `deletion.Schedule` refuses a Staff Group again after a fresh read, the worker's `process` drops a Staff Group's rows with no call. (c) The Deletion page is `Hidden` in a Staff Group and `dl`, `dr` and every control run `settingsDelStaffGate`. (d) A non-group chat ID (zero or positive) is refused on both branches. Tests: `TestDeletionExcludedMessagesNeverScheduled` (drives a real staff action, a real lockdown alert, a real captcha challenge and a real log post at the shortest delay, advances a day and a half, and asserts no delete call), `TestDeletionKindsNeverStaffLockdownCaptchaLog`, `TestServiceWatcherSkipsStaffGroup`, `TestDeletionWorkerSkipsStaffGroup`, `TestSettingsDeletionStaffGroup`, `TestSettingsDeletionHidden`, `TestDeletionActRefusesNonNegativeChat` (passed) |
| 5 | SC5: When the bot lacks the right to delete, every deletion page says so instead of failing silently | VERIFIED | `settingsDelPrepare` makes one live paced `getChatMember` of the bot per draw and the result is spliced into the hub (`renderSettingsDelHub`), both lists (`renderSettingsDelList`) and every row screen (`renderSettingsDelRow`); a definite cannot is the red line, an unestablished answer the unknown line, never an error text, and no button is disabled. The worker's latch uses the same classifier (`deletionClassifyBotRight`). Tests: `TestSettingsDeletionRightLine`, `TestDeletionBotRightClassifier`, `TestDeletionReadBotRight`, `TestDeletionServiceLatchNeedsLiveCheck`, `TestDeletionWorkerTrustsAdminFlagInSupergroups` (passed) |
| 6 | The Deletion page is wired into Main and every press goes through the one settings pipeline: owner gate, live member record, `can_change_info`, strict presets, 64-byte buttons | VERIFIED | `registerSettingsPage` with `Order` 85, `Hidden` hook added to `settingsPage` and honoured in `settingsMainEntries` and `openPage`. Controls take `st` only, `v` must be `n` or a preset of that row. Tests: `TestSettingsDeletionOpensFromMain`, `TestSettingsDeletionValidation`, `TestSettingsAuthorityDeletion`, `TestSettingsDriftDeletion`, `TestSettingsDeletionCallbackBytes`, `TestSettingsDeletionRowBudget`, `TestSettingsDeletionWrites`, `TestSettingsDeletionNoCard`, `TestSettingsDeletionReadsAndWritesFail`, `TestSettingsStaffPrivacy` (passed) |
| 7 | The worker is wired into startup and shutdown inside the budget (PLAT-06) | VERIFIED | `main.go:516` `StartDeletionWorker` in `postInit` after `StartLockdownWorker`; `main.go:229-230` registers `StopDeletionWorker` after DB-close, right before `RegisterMemberReqDrain`, so LIFO runs it before the database closes. It cancels the deletion root (loop, every paced call and every inline delete derive from it) and waits at most 3 s. Rows are released free on `context.WithoutCancel` writes. The 68 s worst case is recorded in AGENTS.md against the 60 s budget with `stop_grace_period: 75s`. Calls share the fleet-wide `fuku:delete:pace:*` pacer (`TestDeletionPacerIsIndependent`). Every goroutine recovers (`TestValidationInlineDeleteRecoversFromPanic`, `TestValidationWorkerCycleRecoversFromPanic`) |
| 8 | MREQ-15: Requirement notices are deleted automatically when the "automatic punishments" timer is set | VERIFIED | `memberReqPostNotice` calls `memberReqScheduleNotice` right after a successful send (`memberreq_notice.go:195`); a plain notice uses `scheduleBotMessage(KindBotAuto)`, the mute notice (with "I fixed it") is queued at the 24 h cap whatever the timer says, and the button's confirmation is rescheduled from the tap (`memberreq_fixed.go:327`, only for the hold's own notice). With the kind Off nothing is written. Tests: `TestMemberReqNoticeScheduled`, `TestMemberReqMuteNoticeAtCap`, `TestMemberReqFixedRescheduleMovesUp`, `TestMemberReqFixedReschedulesOnlyTheNotice` (passed) |
| 9 | Switching a rule to Keep or Off stops messages already queued, so a deleted rule never deletes later | VERIFIED | `applyDeleteRule` and the `/cleanservice` off path drop the chat's pending rows of that kind in the same write path (review WR-03, fixed). Tests: `TestApplyDeleteRuleRemoveDropsPendingRows`, `TestApplyCleanServiceFlagOffDropsPendingRows`, `TestApplyCleanServiceRuleKeepDropsPendingRows` (passed) |
| 10 | The tables stay out of backup, export, import and reset, and the rule key is fresh on every replica | VERIFIED | `fuku/db/backup/deletion_backup_test.go` (`TestDeletionTablesStayOutOfBackup`, passed in `./fuku/db/backup`); `delete_rules:*` is in `skipLocal` (`fuku/db/cache/local.go`, `./fuku/db/cache` passed); every rule write calls `cache.DeleteCache` |
| 11 | The migration is append-only, idempotent and carries its constraints inline | VERIFIED | `20261110120000_add_scheduled_deletion.sql` is the newest file, `IF NOT EXISTS` on both tables and both partial indexes, CHECKs inline (no top-level `ADD CONSTRAINT`), no foreign key, no `CONCURRENTLY`. `./fuku/db/migrations` passed (database-free half); `Makefile` adds the PostgreSQL names and `./fuku/db/deletion` to `test-postgres-integrity` |
| 12 | PLAT-05: every new string exists in all 8 locales, and AGENTS.md reflects them | VERIFIED | `make check-translations` exit 0 ("All translations are present"); `./fuku/i18n` passed (limits tables, `TestSettingsDeletionPageFitsEveryLocale`, `TestDeletionKindKeysExist`); AGENTS.md documents the 8-locale set and every new rule of this phase |

**Score:** 12/12 truths verified (0 present, behavior-unverified). The two behavior-dependent truths (3 and 9) each have a passing behavioral test; the part that needs real Telegram or two live processes is listed under Human Verification.

### Required Artifacts

| Artifact | Expected | Status | Details |
| -------- | -------- | ------ | ------- |
| `migrations/20261110120000_add_scheduled_deletion.sql` | rules and queue tables | VERIFIED | 53 lines, newest migration, idempotent |
| `fuku/db/models/delete_rule.go`, `scheduled_deletion.go` | models with `TableName()` | VERIFIED | Match the migration |
| `fuku/db/deletion/{kinds,rules,queue}.go` | kinds, cached rules, queue | VERIFIED | `ClaimDue`, `Complete`, `Release`, `ReleaseStale`, `DropChatPending`, `Schedule`; `hook_testtools.go` is test-tag only |
| `fuku/modules/deletion_kinds.go` | service and bot row tables | VERIFIED | 12 service rows, 5 bot rows |
| `fuku/modules/deletion_schedule.go` | pacer, inline delete, classifier, latch, `deletionAct` | VERIFIED | Substantive and wired from the watcher, `cleanService`, the hooks and the worker |
| `fuku/modules/deletion_service.go` | group `-5` watcher | VERIFIED | Registered through `RegisterLegacyModule("ServiceDeletion", 242, ...)` |
| `fuku/modules/deletion_worker.go` | worker with stop | VERIFIED | Started and stopped in `main.go` |
| `fuku/modules/deletion_right.go` | delete-right reader | VERIFIED | Used by the worker latch and the page |
| `fuku/modules/deletion_bot.go` | `scheduleBotMessage` family | VERIFIED | Called at the named send sites only |
| `fuku/modules/deletion_apply.go` | apply functions | VERIFIED | One write path for the page and `/cleanservice` |
| `fuku/modules/settings_deletion.go` | Deletion page | VERIFIED | 595 lines, registered, tested |

### Key Link Verification

| From | To | Via | Status | Details |
| ---- | -- | --- | ------ | ------- |
| `main.go` `postInit` | `StartDeletionWorker` | direct call | WIRED | line 516 |
| shutdown manager | `StopDeletionWorker` | `RegisterHandler` after DB-close | WIRED | lines 229-230, before `RegisterMemberReqDrain` |
| `cleanService` (group 0) | `deletionApplyMessage` | after join processing, joined and left only | WIRED | `greetings.go` diff |
| group `-5` watcher | `deletionApplyMessage` | `serviceDeleteCandidate` (bot senders only for joined and left) | WIRED | no double handling with `cleanService` (`TestServiceWatcherSenders`, `TestServiceWatcherBotJoin`) |
| send sites | `deletionSchedule` -> `deletion.Schedule` | `scheduleBotRow` | WIRED | latch and Staff Group refusal apply |
| `memberReqPostNotice` | `memberReqScheduleNotice` | after a successful send | WIRED | `memberreq_notice.go:195` |
| Deletion page controls | rules repository and cache | `applyDeleteRule`, `applyCleanServiceRule` | WIRED | `cache.DeleteCache` on every write |
| Deletion page | Main | `registerSettingsPage` plus `Hidden` | WIRED | `settings_pages.go` |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
| -------- | ------------- | ------ | ------------------ | ------ |
| service watcher | delay and on | `deletionRulesLookup` (cached `chat_delete_rules` read) plus the greetings flag | yes, a database query behind `GetFromCacheOrLoad` | FLOWING |
| worker | due rows | `ClaimDue` on `scheduled_deletions` | yes | FLOWING |
| Deletion page | rule states, right line | `deletionRulesLookup`, flag lookup, live `getChatMember` | yes | FLOWING |
| send-site hooks | stored delay | same rules read | yes | FLOWING |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
| -------- | ------- | ------ | ------ |
| Phase 16 module tests (81 tests: `TestDeletionEndToEnd`, `TestDeletionExcludedMessagesNeverScheduled`, `TestDeletionLogsCarryIDsOnly`, the worker, watcher, hooks, page, `TestSettingsStaffPrivacy`, `TestShutdownEndsDeletionWorker`, the MREQ-15 notice tests) | `go test -tags testtools -race -count=1 -run '^(...)$' ./fuku/modules` | `ok ... 95.939s` | PASS |
| Repositories | `go test -tags testtools -race -count=1 ./fuku/db/deletion ./fuku/db/backup ./fuku/db/greetings ./fuku/db/cache` | all `ok` | PASS |
| Migrations and locales | `go test -tags testtools -race -count=1 ./fuku/db/migrations ./fuku/i18n` | both `ok` | PASS |
| Production build | `CGO_ENABLED=0 go build ./...` | exit 0 | PASS |
| Vet | `go vet -tags testtools ./fuku/modules ./fuku/db/...` | no output | PASS |
| Docs drift | `make check-docs` | "No drift detected" | PASS |
| Translations | `make check-translations` | "All translations are present" | PASS |
| Dependencies untouched | `git diff --exit-code 8c0c827 -- go.mod go.sum` | exit 0 | PASS |

### Probe Execution

Step 7c: SKIPPED. The phase declares no `probe-*.sh` scripts.

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
| ----------- | ----------- | ----------- | ------ | -------- |
| DEL-01 | 16-02, 16-03, 16-08 | Service messages page, per type keep, now or after N seconds; joined and left share `/cleanservice` | SATISFIED | Truth 1; tick in REQUIREMENTS.md is true |
| DEL-02 | 16-05, 16-06, 16-08 | Bot messages page, per kind keep or after N minutes | SATISFIED | Truth 2; tick is true |
| DEL-06 | 16-01, 16-04 | Survive restarts, once across replicas, capped below 48 h, too-old dropped quietly | SATISFIED | Truth 3; tick is true |
| DEL-07 | 16-03, 16-04, 16-05, 16-09 | Never touches staff cards, lockdown alerts, captcha challenges, log posts or a Staff Group | SATISFIED | Truth 4; tick is true |
| DEL-08 | 16-04, 16-08 | Every deletion page says so when the bot cannot delete | SATISFIED | Truth 5; tick is true |
| MREQ-15 | 16-06 | Requirement notices deleted by the automatic punishments timer | SATISFIED | Truth 8; tick is true |
| PLAT-05 (cross-cutting) | 16-07 | 8 locales and AGENTS.md | SATISFIED | Truth 12 |
| PLAT-06 (cross-cutting) | 16-04 | Shared rate budget, crash recovery, drained inside the shutdown budget | SATISFIED | Truth 7 |

No orphaned requirement: REQUIREMENTS.md maps exactly DEL-01, DEL-02, DEL-06, DEL-07, DEL-08 and MREQ-15 to Phase 16. DEL-03, DEL-04 and DEL-05 are Phase 17's and still Pending, as they should be.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
| ---- | ---- | ------- | -------- | ------ |
| `fuku/modules/deletion_*.go`, `settings_deletion.go`, `fuku/db/deletion/*.go` | - | `TBD`, `FIXME`, `XXX`, `TODO`, `HACK`, "not yet implemented" | none found | No debt marker in a file this phase changed |

Accepted limits recorded in `16-REVIEW-DISPOSITION.md` (deferred, not blockers, each documented in AGENTS.md or 16-SPIKE.md check 13): IN-03 (a failure between the claim UPDATE and its read-back strands the rows until the 2 minute stale release), NEW-01 (a send site that read a rule just before a switch-off can write one row after the drop), NEW-02 (switching Auto punishments Off drops a mute notice's 24 h row, so that notice and its button stay in the group), NEW-03 (an inline delete that hits its call timeout or a shutdown is lost). All fail in the safe direction: a message is kept, never a wrong one deleted.

### Human Verification Required

Everything automatable passed. The items below are the live checks and owner decisions the standing policy defers; each is in `16-SPIKE.md` under the number shown, and none changes what the code does when Telegram answers unexpectedly (it deletes nothing it should not).

1. **Batch delete at the limits** (SPIKE 1): one ID in a `deleteMessages` batch that is too old or undeletable. Expected: the per-ID fallback isolates it.
2. **Delete-right line** (SPIKE 2): plain-member bot, administrator without the right, basic group administrator. Expected: the correct line, no disabled button.
3. **Phone-width cells in hi, ru and uk** (SPIKE 3).
4. **Two replicas** (SPIKE 4): one deletion, and the rule seen at the next message.
5. **Restart with queued rows** (SPIKE 5): clean stop and `kill -9`.
6. **Native-speaker read of the 57 new strings** (SPIKE 6).
7. **Assumptions A16-A2, A16-A3, A16-A4, A16-A6, A16-A8** (SPIKE 7).
8. **Amendments D-20 to D-28** (SPIKE 8).
9. **Forum topics and Giveaways, live** (SPIKE 9).
10. **Joined and left coupling on a real group** (SPIKE 10).
11. **Real error strings of a refused delete, and a 429 on `deleteMessages`** (SPIKE 11).
12. **DEL-07 live** (SPIKE 12): staff card, lockdown alert, captcha challenge and log post never deleted.
13. **CI lint, full test run and PostgreSQL integrity target** (SPIKE 14). CI of head `6e02f16`: CI success, Documentation success. CI of head `76163a0`: Documentation success, CI in progress when this was written. Commit `5fe0e8a` (PostgreSQL run of the review-fix queue SQL: `TestReleaseStaleClaims`, `TestDeleteRules`, `TestApplyJoinLeftRule`) and the later `32544cf` and `711fa5b` have no completed CI run yet.

SPIKE check 13 (accepted limits found while building) is informational and needs no action beyond reading it.

### What was not run

- `make test` and the whole `fuku/modules` package (about 20 minutes), by instruction. CI and plan 16-09 ran the full gate.
- `make lint` (the sandbox's golangci-lint is built with an older Go); CI covers it.
- `make test-postgres-integrity` (needs PostgreSQL); the SQLite halves of its tests passed in `./fuku/db/deletion` and `./fuku/db/migrations`.
- Anything needing a live Telegram group (the human items above).

### Gaps Summary

No gap. Every ROADMAP success criterion and every requirement claim of Phase 16 is true in the code and held by a test that passed in this verification. DEL-01, DEL-02, DEL-06, DEL-07, DEL-08 and MREQ-15 are correctly ticked in REQUIREMENTS.md, and PLAT-05 and PLAT-06 still hold. The phase stays `human_needed` only because of the live Telegram checks, native-speaker reads and owner confirmations that the standing policy defers to `16-SPIKE.md`. The phase is not marked Complete here.

---

_Verified: 2026-11-12T06:10:00Z_
_Verifier: Claude (gsd-verifier)_
