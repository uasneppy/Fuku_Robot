---
phase: "1"
slug: "staff-group-links"
status: verified
# threats_open = count of OPEN threats at or above workflow.security_block_on severity (the blocking gate)
threats_open: 0
asvs_level: 1
register_authored_at_plan_time: true
created: "2026-10-05"
---

# Phase 1 — Security

> Per-phase security contract: threat register, accepted risks, and audit trail.
> Register taken from the `<threat_model>` blocks of plans 01-01 to 01-10. Threat Flags taken from their SUMMARY.md files.

---

## Trust Boundaries

| Boundary | Description | Data Crossing |
|----------|-------------|---------------|
| Telegram user to bot command | The `/setstaff` sender's identity and claimed role are untrusted until Telegram confirms them live | User ID, chat ID |
| Telegram sender to command pipeline | Anonymous admins and channel identities cannot be tied to one user | Sender identity |
| Replica to shared cache | Another replica may hold a stale view of Staff Group status | Staff Group / link gate keys |
| Callback presser to staff callbacks | Anyone who can see the message can press the buttons; the callback data is client-supplied | Callback data (row IDs, actions) |
| Callback presser to unlink actions | Any Staff Group member can press; the callback data is client-supplied | Link row ID |
| Callback presser to Refresh / Page | Anyone who can see the panel can press; data is client-supplied | Page number |
| Group member to /linkstaff and /start@bot payload | The payload and the argument are attacker-controlled; anyone in any group can send them | Staff chat ID |
| Group member to /unlinkstaff | Anyone in a linked group can send the command | Command only |
| Group title to HTML notice / panel | Titles are attacker-controlled text rendered in HTML | Free text (up to 64 runes) |
| Bot to Staff Group | Messages posted into a Staff Group must not be triggerable by strangers | Notices |
| Bot to Telegram API | The panel and the sweep fan out live checks; Telegram may rate-limit or fail | getChatAdministrators, getChatMember |
| Telegram service messages / updates to watchers | Migrate, ownership and chat_member updates can be late, missing, reordered, duplicated, or about unrelated chats | Chat IDs, member status |
| Telegram my_chat_member updates to the watcher | Telegram's statements about the bot's own status; may repeat or race across replicas | Bot rights |
| Several replicas to one PostgreSQL / Redis | Concurrent transactions and sweeps from different replicas can interleave | Staff rows, sweep lock |

---

## Threat Register

| Threat ID | Category | Component | Severity | Disposition | Mitigation | Status |
|-----------|----------|-----------|----------|-------------|------------|--------|
| T-01-01 | Spoofing / Elevation | `setStaff` (`alita/modules/staff.go`) | high | mitigate | Live `chat_status.CheckOwner` must return OwnerMatch before `CreateStaffGroup`; `TestSetStaff*` refusal tests | closed |
| T-01-02 | Tampering | Cached `staff_group:` gate across replicas | medium | mitigate | `staff_group:` and `staff_link_of:` prefixes in `skipLocal` (`alita/db/cache/local.go`); `invalidateStaffKeys` after every write (`alita/db/staff/repository.go`); `TestSkipLocalMatchesFreshnessCriticalKeys` | closed |
| T-01-03 | Information disclosure | `/staff` outside a Staff Group | medium | mitigate | Returns `ext.EndGroups` with no reply; `TestStaffTracer*` asserts zero sends | closed |
| T-01-04 | Denial of service | Refusing/unlinking on a transient Telegram error | medium | mitigate | `CheckOwner` returns OwnerUnknown on any error, handler writes nothing; `TestCheckOwnerUnknownOnError` | closed |
| T-01-05 | Spoofing | `/setstaff`, `/unsetstaff` senders | high | mitigate | `helpers.RejectAnonymousSender()` first in RequiredChecks (anonymous admin, channels, 1087968824, 777000); `TestSetStaffRejectsAnonymousAdmin`, `TestRejectAnonymousSender*` | closed |
| T-01-06 | Elevation | Unset-confirm callback | high | mitigate | Chat taken from `query.Message`, live `CheckOwner(chatID, query.From.Id)` on every press; `TestUnsetStaff*` | closed |
| T-01-07 | Tampering / Repudiation | Racing unset confirms on two replicas | medium | mitigate | `DeleteStaffGroupWithLinks` in one transaction; only the RowsAffected == 1 caller posts | closed |
| T-01-08 | Tampering | Linked group made a Staff Group | medium | mitigate | In-transaction `ErrRoleConflict` in `CreateStaffGroup`; PostgreSQL trigger (T-01-11) for the cross-replica race | closed |
| T-01-09 | Tampering | `RekeyChat` partial update | medium | mitigate | One transaction for all UPDATEs, idempotent; `TestRekeyChat*` incl. rollback | closed |
| T-01-10 | Denial of service | Migrate watcher at group -3 swallowing updates | medium | mitigate | `onMigrateMessage` (`alita/modules/staff_watchers.go`) recovers from panics and returns `ext.ContinueGroups` on every path; `TestStaffMigrateUnrelatedChat` | closed |
| T-01-11 | Tampering / Elevation | Concurrent `/setstaff X` and link of X on two replicas | high | mitigate | BEFORE triggers take `pg_advisory_xact_lock(hashtextextended('alita:staff_role:'...))` then check the other table (`migrations/20261004130000_add_staff_role_exclusivity_trigger.sql`); `TestStaffExclusivityTrigger/ConcurrentSetStaffAndLinkNeverBothCommit` passed on PostgreSQL 16 on 2026-10-05 | closed |
| T-01-12 | Denial of service | Advisory-lock deadlock | low | mitigate | Links lock both IDs in `LEAST`/`GREATEST` order; set-staff locks one ID; 20-round concurrency subtest completed | closed |
| T-01-13 | Spoofing / Elevation | Forged `stf_<id>` payload or `/linkstaff <id>` | high | mitigate | `runLinkGroup` requires live `CheckOwner` for the issuer on both chats before `CreateLink`; `TestStaffPickerRefusesStranger` | closed |
| T-01-14 | Information disclosure | Probing which IDs are Staff Groups | medium | mitigate | One uniform `staff_link_refuse_not_staff_owner`; `TestLinkStaffRefusalsUniformForStranger` | closed |
| T-01-15 | Denial of service / Tampering | Notice spam into someone else's Staff Group | high | mitigate | Pre-verification refusals are self-deleting replies in the issuing chat; `TestLinkStaffRefusalsUniformForStranger` asserts zero `sendMessage` to the named chats | closed |
| T-01-16 | Tampering | Double link through check-then-insert race | medium | mitigate | `UNIQUE(group_chat_id)` + `ON CONFLICT DO NOTHING` + RowsAffected → `ErrAlreadyLinked`; `TestStaffRepoCreateLink*` concurrent test | closed |
| T-01-17 | Tampering | HTML injection via group titles in notices | medium | mitigate | `staffDisplayTitle` caps at 64 runes and calls `html.EscapeString` on every rendered title | closed |
| T-01-18 | Tampering | Malformed `/linkstaff` argument or picker payload | low | mitigate | `parseStaffChatIDArg` / `parseStaffPickerPayload` digits-only with length caps; `TestStartGroupPayloadIgnoresJunk` | closed |
| T-01-19 | Tampering | Unlink callback replay / forged link ID | high | mitigate | Row loaded fresh; query message chat must equal `link.StaffChatID`; `TestStaffUnlinkButtonWrongChat`, `TestStaffUnlinkButtonExpired` | closed |
| T-01-20 | Elevation | Unlink by non-owner or after ownership change | high | mitigate | Live `CheckOwner` on both chats at Ask and Confirm; `TestStaffUnlinkButtonOwnerChangedBetweenPresses` | closed |
| T-01-21 | Denial of service | Notice spam via `/unlinkstaff` | medium | mitigate | Self-deleting pre-verification refusals; notice only from the `DeleteLink` RowsAffected == 1 caller; `TestUnlinkStaffStrangerNeverPostsInStaffGroup`, `TestStaffUnlinkButtonConfirmTwiceRemovesOnce` | closed |
| T-01-22 | Elevation | Stale link granting a former owner power | high | mitigate | Live `CheckOwner` on four triggers; maker-must-own-both rule in `recheckLink` / `recheckStaffGroup`; `TestStaffOwnership*` | closed |
| T-01-23 | Spoofing / Tampering | Trusted update payload or wrong-chat chat_member update | medium | mitigate | Payload is only a hint, decision from live admin list; `TestStaffOwnershipPayloadIsOnlyAHint`, `TestStaffOwnershipChatMemberCreatorTransition` | closed |
| T-01-24 | Denial of service | Mass unlinking on 429/timeout | high | mitigate | OwnerUnknown never deletes; `TestStaffOwnershipUnknownKeepsLink` | closed |
| T-01-25 | Repudiation | Duplicate notices from racing triggers/replicas | medium | mitigate | `DeleteLinkIfOwner` RowsAffected picks the single poster; `TestStaffOwnershipConcurrentRechecksPostOnce` | closed |
| T-01-26 | Repudiation / Denial of service | Heads-up spam from repeated checks | medium | mitigate | `SetLinkHealth` conditional UPDATE `WHERE health <> new`; `TestStaffHealthNoRepeat`, `TestStaffHealthConcurrentPostsOnce` | closed |
| T-01-27 | Tampering | Health value derived from update payload | low | accept | See Accepted Risks Log AR-01-01 | closed |
| T-01-28 | Information disclosure | Panel content outside a Staff Group | medium | mitigate | `/staff` silent outside Staff Groups; callbacks require `GetStaffGroupFresh` for the message chat | closed |
| T-01-29 | Denial of service | API bursts from panel opens | medium | mitigate | errgroup `SetLimit(4)`; one staff-owner check per pass (`staffOwnerPass`); `TestStaffPanelCallBudget` | closed |
| T-01-30 | Tampering | HTML injection via titles in the panel | medium | mitigate | `staffDisplayTitle`; `TestStaffPanelRenderEscapesAndTruncatesTitles` | closed |
| T-01-31 | Elevation | Former member refreshing/paging a panel | low | mitigate | Live `IsUserInChatWithError` per press; owner-only actions keep live owner-of-both checks | closed |
| T-01-32 | Denial of service | Mass unlinking on sweep errors | high | mitigate | Errors are unknown and never delete; `TestStaffSweepErrorsNeverDelete` | closed |
| T-01-33 | Denial of service | API bursts / duplicate sweeps across replicas | medium | mitigate | SETNX `alita:staff:sweep:lock`, 250 ms pacing; `TestStaffSweepLockOneRunner` | closed |
| T-01-34 | Repudiation / Elevation | Stale links surviving without updates | high | mitigate | Hourly live recheck plus startup run 1-5 min after boot; `TestStaffSweepTracer` | closed |
| T-01-SC | Tampering | Package installs | low | accept | See Accepted Risks Log AR-01-02; `git diff 34ba3f6 -- go.mod go.sum` is empty | closed |
| TF-01-06 | Information disclosure | `/unlinkstaff` refusal wording (plan 01-06 Threat Flags residual) | low | — | A stranger's `/unlinkstaff` gets "only the creator … can unlink it" in a linked group and "not linked" otherwise, so it reveals whether the group is linked. Both replies delete themselves after 30 s. Hiding it conflicts with the plan's Staff Group creator behaviour. Not yet accepted | open — below high threshold (non-blocking) |

*Status: open · closed · open — below high threshold (non-blocking)*
*Severity: critical > high > medium > low — only open threats at or above workflow.security_block_on count toward threats_open*
*Disposition: mitigate (implementation required) · accept (documented risk) · transfer (third-party)*

---

## Accepted Risks Log

| Risk ID | Threat Ref | Rationale | Accepted By | Date |
|---------|------------|-----------|-------------|------|
| AR-01-01 | T-01-27 | `my_chat_member` is Telegram's own statement about the bot and cannot be forged by users. The DB CHECK limits the allowed values, and the hourly sweep re-derives health live | Plan 01-08 threat model | 2026-10-04 |
| AR-01-02 | T-01-SC | No package installs in this phase: `golang.org/x/sync`, `uuid` and `go-redis` were already required; `go.mod` and `go.sum` unchanged since `main` | Plans 01-01 to 01-10 threat models | 2026-10-04 |

*Accepted risks do not resurface in future audit runs.*

---

## Security Audit Trail

| Audit Date | Threats Total | Closed | Open | Run By |
|------------|---------------|--------|------|--------|
| 2026-10-05 | 35 (+1 flag) | 35 | 0 blocking (1 low flag, non-blocking) | /gsd-secure-phase, ASVS L1 grep-depth (auditor skipped by the short-circuit rule) |

## Security Audit 2026-10-05

| Metric | Count |
|--------|-------|
| Threats found | 35 register entries + 1 summary threat flag |
| Closed | 35 |
| Open | 0 blocking; 1 below threshold (TF-01-06, low) |

Evidence: each mitigation's code symbol and named test were found by grep. All named tests passed under `go test -tags testtools -race` on 2026-10-05 (169 top-level tests). `TestStaffExclusivityTrigger` (all 5 subtests) passed against PostgreSQL 16 the same day.

---

## Sign-Off

- [x] All threats have a disposition (mitigate / accept / transfer)
- [x] Accepted risks documented in Accepted Risks Log
- [x] `threats_open: 0` confirmed
- [x] `status: verified` set in frontmatter

**Approval:** verified 2026-10-05
