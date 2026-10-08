---
phase: "4"
slug: "manual-lockdown"
status: verified
# threats_open = count of OPEN threats at or above workflow.security_block_on severity (the blocking gate)
threats_open: 0
asvs_level: 1
register_authored_at_plan_time: true
created: "2026-10-08"
---

# Phase 4 — Security

> Per-phase security contract: threat register, accepted risks, and audit trail.

---

## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| Admin or captcha pass to a user's own restriction | The per-user record outlives the lockdown |
| Admin's button press to approval | A card posted before the lockdown can still be pressed during it |
| Admin-typed reason and Telegram names to group messages | User-controlled text is rendered as HTML in the group |
| Anonymous admin to the proof button | The real person behind an anonymous post is unknown until they tap the button |
| Bot process to PostgreSQL | A crash or restart can stop work between a claim and its result |
| Bot to Redis | Redis can be flushed or unreachable at any time |
| Bot to Telegram (getChat, setChatPermissions) | The snapshot and the lock are Telegram answers the bot must store and replay without loss |
| Dispatcher goroutines to Telegram | A slow or rate-limited call can pin a handler |
| Group member to /lockdown and /unlockdown | Any member can type the commands; only a live owner or restrict-capable admin may change the group's permissions |
| Group member to /lockdownstatus | The reason and who locked are for the group's admins |
| Join performer to the exemption | The person who added a user decides whether that user gets in |
| Joining user to the group | During a raid, many untrusted accounts join at once |
| Lift to bans made by others | The lift must tell its own bans from deliberate ones |
| Linked group's lockdown to the Staff Group panel | Staff see other groups' state; the reason and starter belong to that group's admins |
| One group's lockdown to the bot's other groups | Lockdown work must stay keyed to its own chat |
| Requester to the group | A join request is another way in during a raid |
| Staff ban to a lockdown joiner | A staff decision interacts with the lockdown's own ban and its lift |
| Telegram's duplicate deliveries to the guard | One join can arrive twice, in any order, on different replicas |
| Two admins to one lockdown row | Concurrent lifts race on the same row across replicas |

---

## Threat Register

Built from the `<threat_model>` blocks of plans 04-01 to 04-08. The supply-chain row (T-04-SC) appears in every plan and is listed once.

| Threat ID | Category | Component | Severity | Disposition | Mitigation | Status |
|-----------|----------|-----------|----------|-------------|------------|--------|
| T-04-01 | Elevation of Privilege | requireLockdownAuthority | high | mitigate | Live getChatMember per command (creator, or administrator with can_restrict_members); lookup failure refuses (TestLockdownCommandsRefuseNonAuthority) | closed |
| T-04-02 | Spoofing | admin cache and Telegram service IDs treated as admins | high | mitigate | The check never reads the admin cache or the service-ID shortcut; "cache says admin, live says member" is refused (TestLockdownCommandsRefuseNonAuthority) | closed |
| T-04-03 | Tampering | lossy permission snapshot | high | mitigate | Raw getChat JSON stored before the lock and replayed verbatim with use_independent_chat_permissions (TestLockdownLiftRestoresExactSnapshot) | closed |
| T-04-04 | Tampering | reason and names injected into HTML or the translator's printf pass | medium | mitigate | 300-rune cap, html escaping and MentionHtml, token splice after translation (TestLockdownCommandStarts asserts "Ad&lt;b&gt;min") | closed |
| T-04-05 | Repudiation | lock announced without effect, or a row left for a lock that failed | medium | mitigate | Notice only after setChatPermissions returned true; a refused lock deletes its unconfirmed row (TestLockdownRefusals "lock call refused") | closed |
| T-04-06 | Elevation of Privilege | anonymous admin with AnonAdmin mode on | high | mitigate | requireLockdownAuthority always calls PromptAnonAdminProof; anonPipelineHandler re-runs the live check for the tapper (TestLockdownAnonymousAdminProof) | closed |
| T-04-07 | Tampering | concurrent /unlockdown | medium | mitigate | BeginLift is one conditional update; only RowsAffected 1 announces (TestUnlockdownLiftsOnce, run 10 times under -race) | closed |
| T-04-08 | Repudiation | lift reported while permissions were not restored | high | mitigate | Restore first; an error replies lockdown_restore_failed and leaves the row active (TestLockdownLiftRestoreFailure) | closed |
| T-04-09 | Information Disclosure | /lockdownstatus content to non-admins | low | mitigate | requireLockdownAuthority(false) checks creator or administrator live (TestLockdownStatusCommand "plain member") | closed |
| T-04-10 | Denial of Service | join guard during a raid | high | mitigate | The guard makes no Telegram write; it records and returns; the worker bans under lockdownPacer with MaxWait (TestLockdownGuardBansChatMemberJoin, TestLockdownWorkerStartStop) | closed |
| T-04-11 | Tampering | ban without a record (never lifted) | high | mitigate | RecordJoin before the worker can ban; a failed insert lets the joiner in muted (TestLockdownGuardBansChatMemberJoin, recordLockdownJoin) | closed |
| T-04-12 | Tampering | lift lifting a deliberate ban | high | mitigate | isLockdownBan compares the live until_date with the row's ban_until; anything else is kept (TestLockdownLiftKeepsDeliberateBan) | closed |
| T-04-13 | Denial of Service | Bot API rate limits across replicas | medium | mitigate | One fleet-wide Redis pacer for lockdown calls; rate-limited rows retried without an attempt (lockdownBanPending) | closed |
| T-04-14 | Spoofing | performer treated as admin from cached or stale data | high | mitigate | Live getChatMember of the performer; a failed lookup bans (TestLockdownAdminAddedJoinerExempt, TestDecideLockdownJoin) | closed |
| T-04-15 | Tampering | duplicate delivery causing two bans or a Redis outage skipping the ban | medium | mitigate | One row per (lockdown, user) with a 20 s window, no Redis in the decision (TestLockdownGuardDedupesTwoPaths) | closed |
| T-04-16 | Elevation of Privilege | an admin-added bot or a self-join during a lockdown | high | mitigate | Bots and self-joins always ban (D-07, D-08; TestDecideLockdownJoin invariants) | closed |
| T-04-17 | Elevation of Privilege | join request approved during a lockdown (auto-approve or Accept) | high | mitigate | Group -7 ends request handling and fails closed; Accept answers an alert (TestLockdownGuardDeclinesJoinRequest, TestLockdownAcceptButtonRefuses, TestLockdownJoinRequestFailsClosed) | closed |
| T-04-18 | Elevation of Privilege | a lockdown acting in another group | high | mitigate | Every query keyed by chat_id or lockdown_id; worker calls use the row's chat; two-group test in both lift orders (TestLockdownAffectsOnlyItsChat) | closed |
| T-04-19 | Denial of Service | a raid looping join requests | low | accept | Each request costs one paced decline; Phase 6 detection revisits it (research A11, Open Question 6) | closed |
| T-04-20 | Repudiation | a crash mid-lift leaving joiners banned for 330 days | high | mitigate | ReleaseStaleClaims and the DB-driven cycle resume every unban on any replica (TestLockdownLiftResumesAfterRestart, TestLockdownStopDuringCall) | closed |
| T-04-21 | Denial of Service | a Redis flush or outage ending or disabling a lockdown | high | mitigate | State only in PostgreSQL; the pacer falls back to a local interval (TestLockdownSurvivesRedisFlush) | closed |
| T-04-22 | Tampering | an unconfirmed lock row (crash between insert and lock) | medium | mitigate | Settled from live permissions: confirmed or deleted, never announced (TestLockdownSettlesUnconfirmedLock) | closed |
| T-04-23 | Tampering | unmute storing the locked set as the user's own restriction | high | mitigate | resolveUnmutePermissions returns the stored snapshot during a lockdown (TestUnmuteDuringLockdownUsesSnapshot, all four callers) | closed |
| T-04-24 | Repudiation | an unmute announced when the lockdown lookup failed | medium | mitigate | The error is returned; no caller restricts or replies after it (TestUnmuteLockdownLookupFails) | closed |
| T-04-25 | Information Disclosure | /staff panel row | low | mitigate | The row carries only LockedSince; renderStaffRow reads no reason or name (TestStaffPanelShowsLockedGroup) | closed |
| T-04-26 | Tampering | staff ban swallowed, then lifted by the lockdown | high | mitigate | LockdownBan makes decideStaffBan re-issue the ban; a failed lookup fails the group closed (TestStaffBanOnLockdownJoinerSurvivesLift, TestStaffBanLockdownLookupFails) | closed |
| T-04-SC | Tampering | npm/pip/cargo installs | low | accept | No package installs; go.mod and go.sum are unchanged | closed |

*Status: open · closed · open — below high threshold (non-blocking)*
*Severity: critical > high > medium > low — only open threats at or above workflow.security_block_on count toward threats_open*
*Disposition: mitigate (implementation required) · accept (documented risk) · transfer (third-party)*

---

## Accepted Risks Log

| Risk ID | Threat Ref | Rationale | Accepted By | Date |
|---------|------------|-----------|-------------|------|
| AR-04-01 | T-04-SC | No packages were installed: `go.mod` and `go.sum` are unchanged from 1fd9cea (phase 4 start) to the phase head | Plan threat models 04-01 to 04-08 | 2026-10-08 |
| AR-04-02 | T-04-19 | A raid looping join requests costs one paced decline per request; Telegram lets a declined user request again at once (research A11). Phase 6 raid detection revisits it | Plan 04-05 threat model, owner decision D-25 | 2026-10-08 |

Related owner decisions (not register threats): D-24 accepts the residual race where an admin approves a pre-lockdown request from Telegram's own list and the guard bans the person first; `/unlockdown` unbans them (04-UAT test 5 passed live). The 330-day ban span is an accepted limit documented in `AGENTS.md` and the help text. Code-review info notes IN-01 to IN-04 stay open in `04-REVIEW-DISPOSITION.md` and are below the block threshold.

*Accepted risks do not resurface in future audit runs.*

---

## Security Audit Trail

| Audit Date | Threats Total | Closed | Open | Run By |
|------------|---------------|--------|------|--------|
| 2026-10-08 | 27 | 27 | 0 | gsd-secure-phase (L1, orchestrator) |

## Security Audit 2026-10-08
| Metric | Count |
|--------|-------|
| Threats found | 27 |
| Closed | 27 |
| Open | 0 |

Evidence at L1 depth:
- Every test a mitigation names exists and passed under `-race` on the phase head: the 26 tests of the `04-VALIDATION.md` map, plus `TestLockdownWorkerStartStop`, `TestLockdownAdminAddedJoinerExempt`, `TestLockdownAcceptButtonRefuses`, `TestLockdownJoinRequestFailsClosed`, `TestLockdownStopDuringCall`, `TestLockdownSettlesUnconfirmedLock`, `TestUnmuteLockdownLookupFails`, `TestStaffPanelShowsLockedGroup`, `TestStaffBanOnLockdownJoinerSurvivesLift` and `TestStaffBanLockdownLookupFails`.
- The mitigation that names no test was checked in code. T-04-13: `lockdownPacer` in `alita/modules/lockdown_worker.go` is a fleet-wide `ratelimit.NewTelegramPacer` with `MaxWait` 60 s, and `lockdownBanPending` returns rate-limited rows without counting an attempt. T-04-04's escaping is `html.EscapeString` and `formatting.MentionHtml` in `alita/modules/lockdown.go`.
- No SUMMARY of plans 04-01 to 04-08 raised a threat flag.
- The live UAT (04-UAT, 13/13) passed against a real bot. That covers the authority and anonymous-admin checks (test 12), the ban marker round trip (test 6), Redis loss and restarts (test 11), cross-group isolation (test 9) and the staff ban over a lockdown ban (test 10). The PostgreSQL partial unique index was proven on PostgreSQL 16 (test 1).

---

## Sign-Off

- [x] All threats have a disposition (mitigate / accept / transfer)
- [x] Accepted risks documented in Accepted Risks Log
- [x] `threats_open: 0` confirmed
- [x] `status: verified` set in frontmatter

**Approval:** verified 2026-10-08
