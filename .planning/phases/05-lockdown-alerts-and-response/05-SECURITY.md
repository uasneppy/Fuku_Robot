---
phase: "5"
slug: "lockdown-alerts-and-response"
status: verified
# threats_open = count of OPEN threats at or above workflow.security_block_on severity (the blocking gate)
threats_open: 0
asvs_level: 1
register_authored_at_plan_time: true
created: "2026-10-18"
---

# Phase 5 — Security

> Per-phase security contract: threat register, accepted risks, and audit trail.

---

## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| User text to HTML messages | Group titles, names and reasons are attacker-controlled |
| Joining users to Redis | Names, usernames and invite links in the 10-minute tracker are attacker-controlled |
| Join hot path to Redis | Every supergroup join now makes one best-effort Redis round trip |
| Joining users to the alert | Names, usernames and links reach staff-facing HTML |
| Button presser to bot | Staff Group members, log channel readers and service identities can press buttons that act on another group |
| Callback data to handler | Client-supplied, forgeable and replayable; can be pressed long after its lockdown ended or name a different link |
| Replicas to Telegram | Several replicas run the worker and may try the same copy, run or report |
| Worker to Telegram | Many copies to close or edit at one lift; hourly posts and edits across every locked group |
| Bot to log channel | A channel others read; its owner chooses the categories it receives |
| Bot to Staff Groups | The group may have been unlinked, or linked elsewhere, since the alert |
| Ban run to Telegram | Bans with no end date that the bot cannot take back, paced across replicas and lasting across lifts and restarts |
| Response ban to the Phase 4 worker and lift | Three writers can act on the same joiner row |
| Telegram errors to staff-facing text | Raw API text must not reach cards |
| Docs to admins | Admins rely on the help text to know what a button does and who may press it |

---

## Threat Register

Built from the `<threat_model>` blocks of plans 05-01 to 05-14. The supply-chain row (T-05-SC) appears in every plan and is listed once. File references are to the phase head as audited; see the audit trail for the test evidence.

| Threat ID | Category | Component | Severity | Disposition | Mitigation | Status |
|-----------|----------|-----------|----------|-------------|------------|--------|
| T-05-01 | Tampering | alert text | medium | mitigate | Title, names and reason escaped and spliced in after the translator (`lockdownGroupLabel`, `lockdownMention`, `lockdownSplice`) | closed |
| T-05-02 | Tampering | duplicate alerts across replicas | low | mitigate | `BeginAlert` conditional on `alert_started_at IS NULL` in the transaction that creates the copies; `ClaimAlertSlot` pending→posting | closed |
| T-05-03 | Info Disclosure | alert reaching a non-Staff-Group chat | medium | mitigate | Destination from uncached `staff.GetLinkOfGroupFresh`, re-checked before the send ("no longer linked"); a lockdown with no destination is still stamped | closed |
| T-05-04 | EoP | Lift button | high | mitigate | `lockdownButtonAuthority`: live `getChatMember` of the lockdown row's chat at Ask and Confirm; a failed lookup refuses | closed |
| T-05-05 | Tampering | callback data (r, u) | high | mitigate | Data carries IDs only; copy row and lockdown loaded fresh; Confirm and Cancel require `u` = presser | closed |
| T-05-06 | Tampering | lift order on the button path | high | mitigate | `liftLockdownRow` restores before `lockdownBeginLift`; shared with `/unlockdown` | closed |
| T-05-07 | Repudiation | who lifted | low | mitigate | `BeginLift` records `lifted_by` and name; the public notice names the presser | closed |
| T-05-08 | Tampering | stale button acting on a newer lockdown | high | mitigate | The copy row names its own lockdown; a non-active lockdown short-circuits through `lockdownStalePress` before any action | closed |
| T-05-09 | DoS | closing many copies at once | low | mitigate | `lockdownAlertBatch` (20) per cycle, newest first, paced, stops on rate limit; 5 s stop wait kept | closed |
| T-05-10 | Spoofing | Confirm/Cancel of another user's card | high | mitigate | Asker check answers "not yours" and edits nothing | closed |
| T-05-11 | EoP | service identities / anonymous presses | high | mitigate | `staffServiceUserIDs` refused before any read | closed |
| T-05-12 | EoP | no restrict right, failed lookup, demotion | high | mitigate | Live lookup requires `CanRestrictMembers`; repeated at Confirm | closed |
| T-05-13 | Tampering | forged or replayed data; overflow | medium | mitigate | Copy honoured only on its own `dest_chat_id`; strict field parsing; lazy expiry; overflow refuses the card and drops the button | closed |
| T-05-14 | DoS | tracker on the join path | medium | mitigate | One pipelined round trip under 250 ms, result ignored; the guard's verdict uses PostgreSQL only | closed |
| T-05-15 | DoS | unbounded tracker growth | low | mitigate | 10-minute trim, 500 cap, 700 s TTL | closed |
| T-05-16 | Tampering | attacker names and links in Redis | low | mitigate | Values capped at write, unparsable members skipped, escaped at render | closed |
| T-05-17 | Info Disclosure | log channel copy | medium | mitigate | `actionlog.Destination(..., CategoryAdmin)` at plan time and again at send time | closed |
| T-05-18 | DoS | one failing destination | medium | mitigate | Per-copy claims and attempts; rate limit or shutdown free; third failure ends the copy | closed |
| T-05-19 | Tampering | duplicate or lost alert across replicas | low | mitigate | `BeginAlert` gate, claims and `ReleaseStaleAlertClaims` (2-minute cut-off); duplicate accepted over lost | closed |
| T-05-20 | Tampering | sample HTML | medium | mitigate | Each sample line escaped, spliced in after translation | closed |
| T-05-21 | DoS | oversized alert | low | mitigate | 10 accounts, names 64 runes, usernames 32, reason 300, title 128 | closed |
| T-05-22 | Tampering | permanent ban overwritten or undone | high | mitigate | Joiner row claimed to acting before the call and moved to `kept`/`response_ban`; the worker and lift skip it | closed |
| T-05-23 | Tampering | banning admins, approved users, the bot or let-in joiners | high | mitigate | `decideLockdownResponseBan` skips admins; approved users, the bot and exempt joiners skipped before any call | closed |
| T-05-24 | Tampering | two concurrent runs | high | mitigate | Partial unique index `uk_chat_lockdown_ban_runs_running` plus `ON CONFLICT DO NOTHING` | closed |
| T-05-25 | Tampering | link HTML | low | mitigate | Link escaped inside `<code>`, spliced in after translation | closed |
| T-05-26 | Info Disclosure | live invite link shown to Staff Group members | low | accept | See AR-05-02 | closed |
| T-05-27 | EoP | Ban button by a non-admin | high | mitigate | `lockdownButtonAuthority` at Ask and Confirm; asker-only Confirm | closed |
| T-05-28 | Tampering | approved users banned after a failed read | high | mitigate | Uncached `ListApprovedIDsFresh` returns its error; Ask and Confirm refuse, a running run waits | closed |
| T-05-29 | DoS | ban run hammering Telegram | medium | mitigate | Fleet-wide `lockdownPaced` (MaxWait 60 s), batches of 50, one running run per lockdown | closed |
| T-05-30 | Tampering | run banning after the lift | high | mitigate | Lockdown re-read per run and before every account, and again inside the paced unit (WR-02); `UnqueueBanRun` then `FinishBanRun(stopped)` | closed |
| T-05-31 | Repudiation | who banned the burst | medium | mitigate | `StartedBy`/name stored at insert; `#LOCKDOWN_BAN` names the presser; resumable report (WR-01) | closed |
| T-05-32 | EoP | Ban card misuse | high | mitigate | Service identities refused; fields, asker and expiry checked; live authority at Confirm | closed |
| T-05-33 | EoP | Revoke without the invite right | high | mitigate | `lockdownInviteAuthority` (creator or admin with `CanInviteUsers`) at Ask and Confirm | closed |
| T-05-34 | Tampering | link taken from callback data | high | mitigate | Revoke and record use the stored `slot.Link` only | closed |
| T-05-35 | Info Disclosure | raw Telegram errors in cards | low | mitigate | `lockdownRevokeRefusalKey` maps to plain keys; raw text to the log only | closed |
| T-05-36 | Tampering | revoke recorded twice / copies edited twice | low | mitigate | `RecordLinkRevoked` conditional on `link_revoked_at IS NULL`; `ClaimAlertSync` and `RecordAlertSynced` | closed |
| T-05-37 | DoS | duplicate or backlogged reminders | medium | mitigate | Unique slot key plus `ON CONFLICT DO NOTHING` in a transaction checking the lockdown; current seq only; stale pending superseded | closed |
| T-05-38 | Info Disclosure | reminders to an unlinked Staff Group or a channel with admin off | medium | mitigate | Destinations re-resolved per reminder, limited to the alert's kinds; send-time checks | closed |
| T-05-39 | DoS | many copies to close or strip | low | mitigate | Sync and close bounded by `lockdownAlertBatch`, paced, stop on rate limit | closed |
| T-05-40 | Repudiation | help text misstating an irreversible action | low | mitigate | Help states the Ban is for good and cannot be undone; fixed in c21b400 to also name who may press Ban and that `/unlockdown` keeps response bans (all 7 locales, docs regenerated) | closed |
| T-05-SC | Tampering | npm/pip/cargo installs | low | accept | See AR-05-01 | closed |

*Status: open · closed · open — below high threshold (non-blocking)*
*Severity: critical > high > medium > low — only open threats at or above workflow.security_block_on count toward threats_open*
*Disposition: mitigate (implementation required) · accept (documented risk) · transfer (third-party)*

---

## Accepted Risks Log

| Risk ID | Threat Ref | Rationale | Accepted By | Date |
|---------|------------|-----------|-------------|------|
| AR-05-01 | T-05-SC | No packages were installed: `go.mod` and `go.sum` are unchanged across Phase 5 (empty diff from 2cfd6b8, the parent of the first Phase 5 commit, to the phase head) | Plan threat models 05-01 to 05-14 | 2026-10-18 |
| AR-05-02 | T-05-26 | A live invite link is shown to Staff Group members who may not be admins of the locked group. Only links the bot sees in full (ones it made) are shown in full; staff are the owner's trusted admins, and the link is already public to everyone it was shared with | 05-09 threat model, owner decisions D-13 and D-18 | 2026-10-18 |

Related limits recorded in `AGENTS.md` and the plans, not register threats: a crash between send and record can post a duplicate alert copy, reminder or ban-run report step (duplicate preferred over lost); a reminder that fell behind is superseded on purpose; one revoke per lockdown (IN-09, stated in the help); response bans cannot be undone and write nothing to the staff audit (D-11).

*Accepted risks do not resurface in future audit runs.*

---

## Security Audit Trail

| Audit Date | Threats Total | Closed | Open | Run By |
|------------|---------------|--------|------|--------|
| 2026-10-18 | 41 | 41 | 0 | gsd-security-auditor (L1) + orchestrator fix of T-05-40 |

## Security Audit 2026-10-18
| Metric | Count |
|--------|-------|
| Threats found | 41 |
| Closed | 41 |
| Open | 0 |

Evidence at L1 depth:
- The auditor checked every `mitigate` threat in code with file and line evidence, and ran the 37 tests the mitigations name under `-race`. They all pass: 26 in `./alita/modules`, 9 in `./alita/db/lockdown` and 2 in `./alita/i18n`.
- T-05-40 was open, below the block threshold. The Ban line of the help did not say who may press it, and the `/unlockdown` line did not say that response bans stay. Both are fixed in c21b400 in all 7 locales; `make check-translations` and `make check-docs` are clean, and the locale and lockdown tests pass.
- All 12 code review findings (`05-REVIEW-FIX.md`) were confirmed fixed in code.
- The guarantees that rest on PostgreSQL-only behaviour are in `make test-postgres-integrity`: the partial unique index, the reminder-slot `ON CONFLICT` and the conditional updates. They run in CI. 05-14 ran an earlier version of the list on PostgreSQL 16; the seven tests added by WR-03 have not yet run on PostgreSQL.
- No SUMMARY raised an unregistered threat flag. 05-06's `SetAllowChannel(true)` note is covered by T-05-12 and T-05-13.

---

## Sign-Off

- [x] All threats have a disposition (mitigate / accept / transfer)
- [x] Accepted risks documented in Accepted Risks Log
- [x] `threats_open: 0` confirmed
- [x] `status: verified` set in frontmatter

**Approval:** verified 2026-10-18
