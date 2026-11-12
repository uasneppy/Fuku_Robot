---
phase: "16"
slug: "scheduled-deletion-of-service-and-bot-messages"
status: verified
# threats_open = count of OPEN threats at or above workflow.security_block_on severity (the blocking gate, "high")
threats_open: 0
threats_total: 42
threats_closed: 42
findings_open_nonblocking: 5
findings_fixed: 0
findings_accepted: 0
hardening_pass: "pending (16-SECURITY-FIX.md)"
asvs_level: 1
register_authored_at_plan_time: true
created: "2026-11-12"
---

# Phase 16 — Security

> Per-phase security contract: threat register, accepted risks, and audit trail.
> Audited by `gsd-security-auditor` against the code at `b5ad8e9` on branch `claude/ecstatic-goldberg-qsss05` (all 9 plans, the
> code review fix pass `490a450..6e02f16` and the review disposition `b5ad8e9`). Every mitigation was read in the code, not
> taken from a plan or summary, and the security-relevant tests were run (about 70 named Phase 16 tests, plus the database
> tests on a throwaway PostgreSQL 16 cluster). Depth is ASVS L1 from `.planning/config.json` (`security_asvs_level: 1`,
> `security_block_on: high`), with L2-depth traces (data flow, ordering, boundary placement) on the DEL-07, authorization and
> cross-chat paths. The auditor's mandate was wider than "verify the register" (as for Phase 15): it also hunted for threats
> a group member, a forged callback or a misconfiguration can actually cause. This audit's findings are `SEC-`; the code
> review's items keep their `16-REVIEW-DISPOSITION.md` names (`RV-` when cited here). Paths are under `fuku/modules/` unless a
> path is given.

---

## Trust Boundaries

| # | Boundary | Data crossing |
|---|----------|---------------|
| 1 | Telegram group → service watcher (group `-5`) and `cleanService` | service messages and their senders; a bot sender; chat type and ID; message ID and date |
| 2 | Group member / admin → the bot's own send sites (welcome, goodbye, rules, manual punishment replies, requirement notices, antiflood, blacklist, aispam breaker notice) | the sent message Telegram answered (chat, type, ID, date) |
| 3 | Settings menu (group or private) → Deletion page (`dl`, `dr`, controls `st`) | callback data (attacker-controlled: `c`, `u`, `t`, `v`), live `getChatMember` records, the owner gate |
| 4 | `/cleanservice` and the shared flag → `chat_delete_rules` | an admin command; the joined/left rows and the flag in one transaction |
| 5 | Bot process → PostgreSQL (`chat_delete_rules`, `scheduled_deletions`) | chat IDs, message IDs, kinds, delays, claim tokens; never message text |
| 6 | Bot process → Redis (`fuku:delete:*` pacer and latch) | slot reservations, a 10 minute scoped latch per chat |
| 7 | Deletion worker → Telegram (`deleteMessages`, `deleteMessage`, `getChatMember` of the bot) | the chat's own message IDs; the bot's own member record |
| 8 | Replicas and shutdown → the queue | conditional claims by token, stale-claim release, a 3 s stop wait inside a 60 s budget |
| 9 | Staff Group, lockdown, captcha, log-channel and private-chat messages → the deletion code (the exclusion boundary, DEL-07) | none may cross: three layers (call sites, `deletionAct`/worker, `Schedule`) |

---

## Threat Register

All 42 threats are `mitigate`, all CLOSED. Evidence is the auditor's, read in the code; every named test exists, asserts the
behaviour and passes. (`dl` = `fuku/db/deletion`, `sched` = `deletion_schedule.go`, `svc` = `deletion_service.go`,
`work` = `deletion_worker.go`, `right` = `deletion_right.go`, `bot` = `deletion_bot.go`, `sdel` = `settings_deletion.go`.)

| Threat ID | Category | Component | Severity | Status | Evidence |
|-----------|----------|-----------|----------|--------|----------|
| T-16-01 | Tampering | delay or kind out of range | medium | CLOSED | `dl/kinds.go:75-102` `ValidKind`, `validateDelay`; `dl/queue.go:79-87`; `dl/rules.go:68-74`; migration CHECKs; `TestDeleteRules`, `TestScheduleRules` |
| T-16-02 | Tampering | migration in both appliers | medium | CLOSED | migration lines 25-53 (`IF NOT EXISTS`, inline constraints, nothing destructive); runner and `migrate_psql.sh` unchanged; `TestScheduledDeletionMigrationStatements`, `...Defaults` (PostgreSQL 16) |
| T-16-03 | Tampering | Staff Group scheduled | high | CLOSED | `dl/queue.go:85` (non-negative refused), `:91-97` fresh Staff Group read failing closed; `TestScheduleRefusesStaffGroup` (SQLite and PostgreSQL) |
| T-16-04 | DoS | double claim | low | CLOSED | `dl/queue.go:156-176` one conditional `UPDATE` read back by token, token-guarded `Complete`/`Release`; `TestClaimBatchOneWinner`, `TestCompleteIsTokenGuarded` (PostgreSQL) |
| T-16-05 | EoP | import or reset | medium | CLOSED | neither table is in `AllExportableModules`; `TestDeletionTablesStayOutOfBackup` (reset keeps the rules on purpose, asserted) |
| T-16-06 | Tampering | creation, migrate, topic-created or giveaway-post message deleted | high | CLOSED | `deletion_kinds.go:86-92`, `:113-119`, `:138-150`; `TestServiceKindTable`, `TestServiceKindsCoverMessageFields` |
| T-16-07 | Tampering | kind or delay reaching the repository | medium | CLOSED | `deletion_apply.go:107-111,134-144,169-173,197-207`; `sdel:120-128`; `TestApplyDeleteRule`, `TestApplyCleanServiceRule` |
| T-16-08 | Repudiation | stale delay revives | low | CLOSED | `dl/rules.go:240-252`; `greetings.go:354-356,794-798`; `TestCleanServiceCommandResetsDelayRows` |
| T-16-09 | Tampering | delete in a Staff Group (watcher, `cleanService`) | high | CLOSED | `sched:93-100` (a read error counts as Staff), `:357`; `svc:88`; `greetings.go:1163-1165`; `TestServiceWatcherSkipsStaffGroup` |
| T-16-10 | DoS | watcher ends an update | high | CLOSED | `svc:100-109` named result preset, `RecoverFromPanic`; group `-5` has one handler; `TestServiceWatcherNeverEndsUpdate`, `TestServiceWatcherSenders` |
| T-16-11 | DoS | unbounded goroutines or shared pacer | medium | CLOSED | `sched:34-40` own pacer keys, `:104` cap 64, `:364-395` cap or root cancel falls back to the durable path; `TestDeletionInlineCapFallsBackToQueue`, `TestDeletionPacerIsIndependent` (leftover gap: SEC-02) |
| T-16-12 | DoS | hammering a chat | medium | CLOSED | `sched:254-272,398-409`; `right:90-102`; `TestDeletionClassify`, `TestDeletionLatch` (leftover gap: SEC-02) |
| T-16-13 | Tampering | unreadable rule fails open | medium | CLOSED | `svc:59-73,79-84`; `TestCleanServiceEffectiveRule` |
| T-16-14 | InfoDisc | logs | low | CLOSED | every log call in the deletion files names a chat ID and counts only (grepped); `TestDeletionLogsCarryIDsOnly` |
| T-16-15 | Tampering | worker deletes in a Staff Group | high | CLOSED | `work:307-318` rows dropped with no call, a read error releases them free; `TestDeletionWorkerSkipsStaffGroup` |
| T-16-16 | DoS | 429 storm | medium | CLOSED | `work:238,375-376,409-410`; `telegram_pacer.go:172-173`; `TestDeletionWorkerRateLimitedFree` |
| T-16-17 | DoS | latch scope | medium | CLOSED | `work:326-344,422-437`; `right:90-102`; `TestDeletionServiceLatchNeedsLiveCheck`, `TestDeletionWorkerLatchedChat` |
| T-16-18 | Repudiation | double delete across replicas | low | CLOSED | `work:28-34,228`; token guard; `TestDeletionWorkerOneDeleteAcrossReplicas`, `TestDeletionWorkerStaleClaimNotDoubleProcessed` |
| T-16-19 | DoS | shutdown budget | medium | CLOSED | `work:83-102`; `main.go:228-232` (registration order verified); AGENTS.md states 68 s against 60 s; `TestShutdownEndsDeletionWorker` |
| T-16-20 | InfoDisc | worker logs | low | CLOSED | `work:322-323` counts only; `TestDeletionWorkerLogsNoMessageIDs` |
| T-16-21 | Tampering | false latch in a basic group | low | CLOSED | `right:53-57,91`; `TestDeletionBotRightClassifier` |
| T-16-22 | Tampering | catch-all hook | high | CLOSED | `bot:69-74`; no scheduling helper in the staff, lockdown, captcha, raid, log-channel, actionlog, helpers or media files; `settings_edit_preview.go:93` discards the message; `TestBotRulesPrivateNeverScheduled`, `TestScheduleBotMessage`, `TestDeletionExcludedMessagesNeverScheduled` |
| T-16-23 | Tampering | helper changes a send | medium | CLOSED | `bot:31-57` returns nothing; the Phase 12 request goldens unchanged and passing |
| T-16-24 | DoS | message dates | low | CLOSED | `sched:288-298`; `work:246-257`; `TestScheduleBotMessageClampsDate` |
| T-16-25 | Repudiation | warn source | low | CLOSED | `warns.go:327`; `blacklists.go:579`; `TestWarnThisUserSource` |
| T-16-26 | DoS | mute notice deleted early | high | CLOSED | `bot:84-90`; `memberreq_notice.go:217-223`; `TestMemberReqMuteNoticeAtCap` |
| T-16-27 | Tampering | reschedule base | medium | CLOSED | `bot:97-102` uses now as the base; `TestMemberReqFixedRescheduleMovesUp` (leftover gap: SEC-01) |
| T-16-28 | Tampering | non-notice post scheduled | high | CLOSED | `aispam.go:362-369` hooks only the breaker notice, `aispamMirrorDeletion` untouched; `blacklists.go:528,551,573`; `antiflood.go:422`; `TestBotAutoPunishmentNotices` |
| T-16-29 | InfoDisc | hook logs | low | CLOSED | `bot:37,55` chat ID and kind only |
| T-16-30 | InfoDisc | strings | medium | CLOSED | `locales/en.yml:3290-3346` have no staff or ID placeholders; `TestSettingsLocaleKeys`, `TestSettingsReusedKeyPlaceholders`, `TestSettingsStaffPrivacy` |
| T-16-31 | Tampering | translations | low | CLOSED | `TestSettingsLabelLengths`, `TestDeletionKindKeysExist`; `make check-translations` |
| T-16-32 | Tampering | forged callback | high | CLOSED | `sdel:95-98,120-128,163-172,277-288`; `settings_menu.go:263-269,303-316`; `TestSettingsDeletionValidation` (27 forged shapes, no `getChatMember`), `TestSettingsDeletionStaffGroup` |
| T-16-33 | EoP | rights | high | CLOSED | `sdel:96`; `settings_menu.go:99-109,388-425` (live record and owner gate on every press); `TestSettingsAuthorityDeletion`, `TestSettingsParityCleanService` |
| T-16-34 | InfoDisc | Staff Group named | high | CLOSED | `TestSettingsStaffPrivacy`; the page is hidden in a Staff Group |
| T-16-35 | DoS | press loop | low | CLOSED | `sdel:294-303,316-323`; `TestSettingsDeletionCost`, `TestSettingsDeletionRightLine` |
| T-16-36 | Tampering | fail open | medium | CLOSED | `sdel:228-246,261-288,486-495`; `TestSettingsDeletionStaffGroup`, `TestSettingsDriftDeletion` |
| T-16-37 | Repudiation | stale screen | low | CLOSED | `fuku/db/cache/local.go` `skipLocal` has `delete_rules`; `deletion_apply.go:117`; `TestSettingsDeletionStaleState` |
| T-16-38 | Tampering | framework edit | low | CLOSED | `settings_pages.go:373-375,489-491` (+6 lines); `settings_menu.go:515-519` |
| T-16-39 | Repudiation | docs drift | medium | CLOSED | AGENTS.md has the 68 s figure, group `-5` and the IN-02 sentence (one small drift: SEC-04) |
| T-16-40 | Repudiation | early requirement ticks | medium | CLOSED | the ticks (`0b5f694`) come after the end-to-end test (`1ac05e3`) and the gate; DEL-03 to DEL-05 stay unticked |
| T-16-41 | Tampering | future hook | high | CLOSED | `TestDeletionExcludedMessagesNeverScheduled` (staff card and summary, lockdown alert, captcha, log post), the grep gate and the log sweep |
| T-16-SC | Tampering | `go.mod` and `go.sum` | high | CLOSED | `git diff --exit-code 8c0c827 -- go.mod go.sum` exits 0; no new env var, no new secret |

`threats_open: 0`. No `## Threat Flags` in any of the nine SUMMARY files (all say "none"), so there are no unregistered flags.

---

## New Threats Found by the Audit (unregistered)

None reaches `high`; none blocks the phase. Dispositions are the orchestrator's, executed in the hardening pass.

- **SEC-01 (low, Tampering): "I fixed it" reschedules whatever message was pressed, not the hold's notice.**
  `memberreq_fixed.go:214` passes `query.Message.GetMessageId()` and `:315` schedules that ID through `scheduleBotMessageFromNow`;
  nothing compares it with `ChatRequirementHold.NoticeMsgID`, which Phase 13 deliberately never compares for the unmute. A
  throwaway test had a muted member press their own valid `mreq|v1` data on another message (777001): one edit of the other
  message, no edit of the notice, and a `bot:punish_auto` queue row for 777001. Exploit: an attacker who holds a requirement
  mute, with a modified client that attaches the fix data to another bot message with a keyboard (another member's captcha
  challenge, a DEL-07 exclusion), gets that message deleted at the Auto timer; the edit already happened since Phase 13, Phase 16
  adds the deletion. It needs Telegram not to check callback data against the message's keyboard (this repository assumes it does
  not, "callback data is not authenticated"); not verified live. Staff Groups stay protected by `Schedule`.
  *Disposition: fix.* Reschedule only when the pressed message is the hold's stored notice (`NoticeMsgID` equals the message ID);
  the 24 hour row of the real notice stays when they differ.
- **SEC-02 (low, DoS): one misconfigured group can use up the fleet-wide deletion budget.** A refused inline delete never runs
  the live right check, so it never latches (`sched:398-409`, asserted by `deletion_schedule_test.go:129`); the worker never
  latches an administrator bot, because it does not trust the administrator flag (`right:53-57,91`, asserted by
  `deletion_right_test.go:142`); and callers with a deadline shorter than the pacer's 60 s `MaxWait` reserve a slot and abandon it
  on timeout (`telegram_pacer.go:89-94,178-185`: inline 15 s, page right read 3 s, latch check 10 s). Scenario: the bot is an
  administrator without "Delete messages" in a group where `/cleanservice` or any service rule is on; join and leave churn costs
  one refused paced call each, forever, and 64 inline goroutines keep the 60 s reservation window full, so other groups' deletes
  are shed to the queue and worker cycles are cut. Rows are not lost (they wait, up to 47 h) and the other subsystems' pacers are
  unaffected. *Disposition: fix (1) and (2); accept (3).* (1) After a refused inline service delete decide the service latch at
  most once per chat per 10 minutes (a Redis `SET NX EX` guard); (2) the worker trusts the administrator flag for supergroup IDs
  (Bot API: `-100…`), where the flag is reliable, and keeps distrusting it for basic groups. (3) Capping each pacer wait at the
  caller's remaining deadline changes the shared pacer used by every subsystem: not done, recorded below.
- **SEC-03 (info): `render.yaml` sets no shutdown delay.** Render's default 30 s before SIGKILL contradicts AGENTS.md ("deploy
  manifests must allow more than 60 s"). It predates Phase 16 and deletion stays correct under SIGKILL (rows resume, acting rows
  are released after 2 minutes). *Disposition: accept, pre-existing; carried to the owner's deploy checklist* (set a shutdown
  delay of at least 75 s where the service is deployed on Render).
- **SEC-04 (info): `docs/.../handler-groups.md:18` says the group `-5` filter is `message.All`;** the code uses
  `serviceDeleteFilter`. *Disposition: fix* (docs).
- **SEC-05 (info): the inline branch of `deletionAct` has no negative-ID check;** both callers handle groups only, and
  `Schedule` refuses a non-negative ID, so nothing is reachable today. *Disposition: fix* (defence in depth, one line, symmetric
  with IN-05).

Information items with no action: a `FromNow` row with no earlier row gets `sent_at` equal to the tap time (the 47 h drop counts
from the tap; harmless, Telegram refuses and the row is dropped quietly); Reset keeps the deletion rules, as for the other
operational tables (asserted by the backup test); `can_change_info` without `can_delete_messages` is enough to make the bot
delete any service notice and its own replies (D-17), limited by the owner gate; after the 24 hour cap a mute notice and its
button are gone while the mute lasts 330 days (leave and rejoin still lifts it, D-14).

---

## Accepted-limit challenges (16-REVIEW-DISPOSITION.md)

- **RV NEW-01 (challenged, still accepted with corrected wording).** The window after a switch-off is not milliseconds: the drop
  skips `acting` rows, and a worker can hold rows `acting` for up to 60 s while waiting for a pacer slot, or 90 s under the cycle
  guard; if that cycle is cut, the rows return to `pending` and outlive the switch-off. The impact is still only the owner's own
  declined deletions of the bot's own messages and service notices, and SEC-02's congestion widens the window. The reviewer's
  alternative (the worker re-reads the rule per chat and kind before the call) would close it but changes every worker test that
  seeds rows without a rule, for a one-time, owner-declined, bounded deletion: accepted.
- **RV NEW-04 (closed by this audit).** On PostgreSQL 16 `TestReleaseStaleClaims` (the retry time) passes, and a throwaway test of
  `RemoveRule` followed by `ApplyCleanServiceFlag(off)` dropped only the exact-kind pending rows of that chat and kept the acting
  row, the other kind, the other chat and the pinned-message row.
- **Agreed as bounded:** IN-03, NEW-02, NEW-03, NEW-05.

---

## Accepted Risks Log

| # | Risk | Why accepted |
|---|------|--------------|
| AR-1 | SEC-02 part 3: pacer waits are not capped at the caller's deadline | the pacer is shared by every subsystem; the two fixes above remove the amplifier (a chat that can never delete), the rest is bounded by 64 inline goroutines, the 60 s `MaxWait` and the durable queue |
| AR-2 | SEC-03: `render.yaml` shutdown delay | pre-existing, owner's deploy checklist; deletion is correct under SIGKILL |
| AR-3 | RV NEW-01 (corrected wording above) | one owner-declined deletion of the bot's own message, bounded by 90 s |
| AR-4 | After the 24 hour cap a mute notice and its button are gone while the mute lasts | D-14: leave and rejoin lifts the mute, `/unmute` and an approval remain |

---

## Security Audit Trail

| Date | Event | Result |
|------|-------|--------|
| 2026-11-12 | `gsd-security-auditor` at `b5ad8e9`, ASVS L1 with L2 traces, wider mandate | 42/42 threats CLOSED, `threats_open: 0`; SEC-01 and SEC-02 (low), SEC-03 to SEC-05 (info); RV NEW-01 challenged, RV NEW-04 closed on PostgreSQL 16 |
| 2026-11-12 | orchestrator dispositions | SEC-01, SEC-02 (1)(2), SEC-04, SEC-05 fix; SEC-03 and SEC-02 (3) accepted; hardening pass next |

| Threats found | Closed | Open |
|---------------|--------|------|
| 42 | 42 | 0 |
