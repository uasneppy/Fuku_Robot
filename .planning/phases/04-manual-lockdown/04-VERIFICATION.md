---
phase: 04-manual-lockdown
verified: 2026-10-07T04:40:00Z
status: human_needed
score: 11/12 must-haves verified
covered_files:
  - ".planning/phases/04-manual-lockdown/04-01-PLAN.md"
  - ".planning/phases/04-manual-lockdown/04-01-SUMMARY.md"
  - ".planning/phases/04-manual-lockdown/04-02-PLAN.md"
  - ".planning/phases/04-manual-lockdown/04-02-SUMMARY.md"
  - ".planning/phases/04-manual-lockdown/04-03-PLAN.md"
  - ".planning/phases/04-manual-lockdown/04-03-SUMMARY.md"
  - ".planning/phases/04-manual-lockdown/04-04-PLAN.md"
  - ".planning/phases/04-manual-lockdown/04-04-SUMMARY.md"
  - ".planning/phases/04-manual-lockdown/04-05-PLAN.md"
  - ".planning/phases/04-manual-lockdown/04-05-SUMMARY.md"
  - ".planning/phases/04-manual-lockdown/04-06-PLAN.md"
  - ".planning/phases/04-manual-lockdown/04-06-SUMMARY.md"
  - ".planning/phases/04-manual-lockdown/04-07-PLAN.md"
  - ".planning/phases/04-manual-lockdown/04-07-SUMMARY.md"
  - ".planning/phases/04-manual-lockdown/04-08-PLAN.md"
  - ".planning/phases/04-manual-lockdown/04-08-SUMMARY.md"
  - "AGENTS.md"
  - "Makefile"
  - "alita/db/lockdown/joiners.go"
  - "alita/db/lockdown/joiners_lift_test.go"
  - "alita/db/lockdown/joiners_test.go"
  - "alita/db/lockdown/repository.go"
  - "alita/db/lockdown/repository_test.go"
  - "alita/db/lockdown/testmain_test.go"
  - "alita/db/models/lockdown.go"
  - "alita/db/testmain_test.go"
  - "alita/i18n/lockdown_locale_test.go"
  - "alita/modules/antiraid.go"
  - "alita/modules/bans.go"
  - "alita/modules/captcha.go"
  - "alita/modules/chat_permissions.go"
  - "alita/modules/chat_permissions_test.go"
  - "alita/modules/greetings.go"
  - "alita/modules/lockdown.go"
  - "alita/modules/lockdown_anon_test.go"
  - "alita/modules/lockdown_ban_live_test.go"
  - "alita/modules/lockdown_decide_test.go"
  - "alita/modules/lockdown_durability_test.go"
  - "alita/modules/lockdown_expired_test.go"
  - "alita/modules/lockdown_fake_test.go"
  - "alita/modules/lockdown_guard.go"
  - "alita/modules/lockdown_guard_policy_test.go"
  - "alita/modules/lockdown_guard_test.go"
  - "alita/modules/lockdown_isolation_test.go"
  - "alita/modules/lockdown_lift_joiners_test.go"
  - "alita/modules/lockdown_lift_test.go"
  - "alita/modules/lockdown_lock_unknown_test.go"
  - "alita/modules/lockdown_marker.go"
  - "alita/modules/lockdown_paths_test.go"
  - "alita/modules/lockdown_perms.go"
  - "alita/modules/lockdown_perms_test.go"
  - "alita/modules/lockdown_refusals_test.go"
  - "alita/modules/lockdown_requests_test.go"
  - "alita/modules/lockdown_staff_test.go"
  - "alita/modules/lockdown_status.go"
  - "alita/modules/lockdown_status_test.go"
  - "alita/modules/lockdown_test.go"
  - "alita/modules/lockdown_unmute_test.go"
  - "alita/modules/lockdown_worker.go"
  - "alita/modules/mute.go"
  - "alita/modules/staff_action_decide.go"
  - "alita/modules/staff_action_decide_test.go"
  - "alita/modules/staff_action_run.go"
  - "alita/modules/staff_panel.go"
  - "alita/modules/staff_panel_render_test.go"
  - "alita/modules/test_harness_test.go"
  - "alita/utils/chat_status/chat_status.go"
  - "docs/src/content/docs/commands/lockdown/index.md"
  - "docs/src/content/docs/commands/staff/index.md"
  - "locales/config.yml"
  - "locales/en.yml"
  - "locales/es.yml"
  - "locales/fr.yml"
  - "locales/hi.yml"
  - "locales/id.yml"
  - "locales/pt.yml"
  - "locales/ru.yml"
  - "main.go"
  - "migrations/20261006120000_add_chat_lockdowns.sql"
covered_digest: "v2:sha256:f40a68f1d9d7c1e7d7e334c1aa4051ddd2e5b6ff5c979f24f012c7cd9417555d"
behavior_unverified: 0
overrides_applied: 0
coincidental_reliance_items:
  - truth: "The lift unbans only the lockdown's own ban and keeps a deliberate one (D-05, SC5 side of LOCK-07)"
    reason: fixture-only
    harden: "Every test sets the banned member's until_date itself in the fake (staffFakeMember.UntilDate). In production the recognition holds only if Telegram echoes until_date from getChatMember to within 2 s of what banChatMember was sent (research A2). Save a real getChatMember answer for a lockdown-banned user as a fixture and assert isLockdownBan against it."
  - truth: "The lift restores the exact pre-lockdown permissions (LOCK-07)"
    reason: fixture-only
    harden: "lockdownTestPrePermissions is a hand-written 16-key object the fake returns from getChat. Production fidelity relies on a real getChat answer carrying every key, in a shape that setChatPermissions with use_independent_chat_permissions replays unchanged (research A1). Save a real getChat permissions answer as a fixture."
human_verification:
  - test: "Run the PostgreSQL 16 checks that the executors could not run: apply the whole migration chain, then TestStartLockdownOneActivePerChat against that database (the command is in 04-08-SUMMARY.md under 'Items for the orchestrator')"
    expected: "'--- PASS: TestRepositoryMigrationChain', a 'lockdown repository backend: postgres' line, '--- PASS: TestStartLockdownOneActivePerChat', and no '--- SKIP'"
    why_human: "Needs 'su postgres' for a throwaway cluster, which the sandbox refuses (it was refused for the executors and was not circumvented here). Every lockdown repository test so far ran on SQLite, so the uk_chat_lockdowns_active partial unique index, ON CONFLICT DO NOTHING without a target, the NOT EXISTS in FinishLift and the IN (SELECT ...) in ReleaseStaleClaims are unproven on the production database."
  - test: "Run make lint on a machine whose golangci-lint is built with Go 1.26"
    expected: "Exit 0 with no new issues in the phase's files"
    why_human: "The installed golangci-lint is built with go1.25 and refuses the go1.26.0 module, so the lint gate has never run on Phase 4 code."
  - test: "In a real supergroup set unusual default permissions (text on, photos off, reactions off, invite on), run /lockdown, compare the Permissions screen, run /unlockdown and compare again. Save the raw getChat permissions answers as a test fixture."
    expected: "The Permissions screen after /unlockdown is identical to before. Locked: members cannot send anything, admins still can."
    why_human: "The raw getChat permissions shape and the setChatPermissions replay (research A1) can only be confirmed against real Telegram. The phase's central promise (settings survive) rests on it."
  - test: "In a test supergroup run /lockdown, then join through (a) a normal invite link, (b) a join-request link and (c) an admin adding the user directly. Also repeat with a bot account being added by an admin."
    expected: "Each normal joiner is banned once with no welcome and no captcha, the join request is declined, the admin-added human stays, the bot is banned; all banned joiners can rejoin after /unlockdown."
    why_human: "Live delivery order and duplicate delivery of chat_member, new_chat_members and chat_join_request (D-03) cannot be reproduced by the fake dispatcher. ext.EndGroups on a real update is only proven in the fake."
  - test: "Leave a join request pending, run /lockdown, then approve that request from Telegram's own request list as an admin."
    expected: "The person gets in. If the accepted D-24 race bans them instead, /unlockdown unbans them."
    why_human: "Depends on Telegram's update order (research A3/A5); the residual race is accepted by the owner (D-24) but its real outcome was never seen."
  - test: "During a lockdown let a joiner be banned, read getChatMember for them and compare until_date with the stored ban_until (join row), then lift."
    expected: "until_date is within 2 s of the stored value, and the joiner is unbanned at the lift"
    why_human: "Telegram's until_date echo and rounding (research A2) is assumed by isLockdownBan; if it is off, every lift would keep every raider banned."
  - test: "Lock a group that is linked to a Staff Group, open /staff in the Staff Group, then lift and press Refresh"
    expected: "A '🔒 in lockdown since <date> UTC' line appears on that group's row only, with no reason and no name, and disappears after the lift and refresh"
    why_human: "Live panel rendering in a real Staff Group (SETUP-08 deferred status)."
  - test: "With auto-approve on and join requests required, run /lockdown and request to join from a second account; then with auto-approve off tap Accept on a posted approve card during a lockdown; run /unlockdown and try again"
    expected: "The request is declined within seconds, requesting again is declined again, Accept shows the lockdown alert and approves nothing, and after the lift new requests behave normally"
    why_human: "Whether a declined person can request again at once (research A11) and the Accept button alert in a real client are live behaviours."
  - test: "Lock two real groups, lift one"
    expected: "The other keeps its restricted permissions, its removed joiners and its /staff marker"
    why_human: "Cross-group isolation (LOCK-04) is proven in the fake; a live two-group run confirms nothing leaks in real Telegram."
  - test: "Lock a group, let a test account be banned by the lockdown, send /tban <id> 1d from the Staff Group, lift the lockdown"
    expected: "The account is still banned in that group after the lift"
    why_human: "The staff-ban-over-lockdown-ban branch (D-05) is proven with a fake; the real until_date semantics of banChatMember on a kicked user are assumed."
  - test: "Restart drills: (1) restart the bot with joiners pending and (2) restart in the middle of a lift with many banned joiners, then (3) flush Redis during a lockdown, ideally with two replicas running"
    expected: "Pending joiners are banned after the restart, the lift resumes and posts one tally, a Redis flush changes nothing, and both replicas enforce the lockdown without banning anyone twice"
    why_human: "Restart, Redis-loss and multi-replica behaviour are proven with an in-process worker and miniredis, not with real processes."
  - test: "As an anonymous admin run /lockdown, tap the proof button, then do the same for /unlockdown and /lockdownstatus, with the group's AnonAdmin mode on and off"
    expected: "The proof button always appears, the tapper is the one named as locking or lifting, and a non-admin tapper is refused"
    why_human: "The anonymous-admin flow is proven with the fake; a real tap on the real proof button is a live check."
  - test: "In a real group with a captcha pending, run /lockdown, let the user pass the captcha, run /unmute on another muted user, then /unlockdown"
    expected: "Both users can talk after the lift (LOCK-08)"
    why_human: "Real per-user restriction semantics (a user's own restriction copied from the snapshot while the default is locked) is what keeps them from being muted after the lift; the fake cannot prove how Telegram combines the two."
---

# Phase 4: Manual Lockdown Verification Report

**Phase Goal:** As a group admin, I want to lock my group down during a raid and lift it when safe, so that attackers are stopped and settings survive.
**Verified:** 2026-10-07T04:40:00Z
**Status:** human_needed
**Re-verification:** No, initial verification
**Head verified:** `b00d5f2` on branch `claude/cool-hamilton-hvs9nl`, working tree clean (apart from this report).

## Summary

The phase goal is achieved in the code. I found no failed truth and no blocker. Every claim I checked against the code was backed by a real implementation and by a passing, non-trivial test, and the three review findings fixed after execution (CR-01, WR-01, WR-02) are present in the code and covered by regression tests.

The status is `human_needed` and not `passed` for three reasons that no amount of reading can close:

1. The PostgreSQL leg (migration chain and one-active-lockdown test on PostgreSQL 16) has never run. Every repository test ran on SQLite. I did not circumvent the sandbox refusal of `su postgres`.
2. `make lint` has never run on this code (toolchain mismatch).
3. Every Telegram-facing assumption (research A1 `getChat` shape, A2 `until_date` echo, A3/A5 join update order, A11 re-request after decline) is only exercised against hand-written fakes. The phase's two central promises, "settings survive" and "the lift unbans only its own bans", rest on A1 and A2.

## Goal Achievement

### Observable Truths

ROADMAP success criteria are SC1 to SC5 (the contract). Truths 6 to 12 are plan-level truths that carry the decisions D-01..D-25 and the review fixes.

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| SC1 | `/lockdown` mutes everyone except admins, anyone who joins is removed at once, removed users can rejoin after the lift, other linked groups are unaffected | VERIFIED | `lockdown.go:375` sends the all-false 16-key set via `setLockdownPermissions` with `use_independent_chat_permissions: true` (`lockdown_perms.go:65-69`), only after the snapshot row exists (`lockdown.Start` at `:363`). Guard at handler group `-7` (`lockdown_guard.go:557-574`) records a joiner row then `ext.EndGroups`; the worker bans with `until_date = ban_until` (`lockdown_worker.go:373-414`) and at the lift unbans only the lockdown's own ban with `OnlyIfBanned` (`:526-565`). Every query is keyed by `chat_id` or `lockdown_id`. Tests: `TestLockdownLockSendsLockedSet`, `TestLockdownGuardBansChatMemberJoin`, `TestLockdownLiftUnbansJoiners`, `TestLockdownRejoinAfterUnban`, `TestLockdownAffectsOnlyItsChat`, all PASS under `-race` |
| SC2 | Approved users: spike answered "can't", so approved users are muted too and the notice says so (D-01, D-02) | VERIFIED | `lockdown.go:397-407` always appends `lockdown_note_approved_muted` ("Approved users are muted too."); the lock makes no per-user call. `TestLockdownNoticeAndNoPerUserCalls` PASS (zero `restrictChatMember` calls with an approved user present). Prohibition "never tell the group approved users can talk" holds: no such string exists in `locales/en.yml` |
| SC3 | An admin can see whether the group is locked, since when and why; `/staff` marks the group; a second `/lockdown` reports the existing one | VERIFIED | `lockdown_status.go` (`/lockdownstatus`, live authority, read-only); `lockdownAlreadyActiveText` at `lockdown.go:264` used at `:329` and `:372`; `staff_panel.go:176-184` + `markLockedRows` `:464-480` called from `buildStaffPanel`. Tests: `TestLockdownStatusCommand`, `TestLockdownCommandReportsExisting` (one `setChatPermissions` call, one row), `TestRenderStaffRowLockdownMarker`, `TestStaffPanelShowsLockedGroup`, all PASS. Live panel rendering is a human item |
| SC4 | A lockdown never lifts on its own; survives restarts and a Redis flush; every replica enforces it; only an admin of the group can lift it, others refused | VERIFIED | State is PostgreSQL only: no cache or Redis read in `alita/db/lockdown/*.go`, `lockdown_guard.go`, `lockdown.go`, `lockdown_status.go` (grepped). The only callers of `BeginLift` are `lockdownBeginLift` in `unlockdown` (`lockdown.go:492`); `DeleteUnconfirmed` only ever touches unconfirmed rows. `StartLockdownWorker` runs on every replica (`main.go:442`), rows are claimed by conditional updates, stale claims released after 2 min (`joiners.go:300`), `StopLockdownWorker` registered after DB-close (`main.go:160` vs `:224`). Tests: `TestLockdownNeverLiftsOnItsOwn` (400-day-old lockdown, 20 cycles, still active), `TestLockdownSurvivesRedisFlush` (flushed and no-Redis), `TestLockdownLiftResumesAfterRestart`, `TestLockdownCommandsRefuseNonAuthority`, all PASS. Real multi-replica and restart drills are human items |
| SC5 | Lifting restores the permissions exactly as before; anyone unmuted during the lockdown (`/unmute`, captcha) can still talk after the lift | VERIFIED | Snapshot is the raw `permissions` member of `getChat` (`lockdown_perms.go:35-55`, stored at `lockdown.go:360`) and replayed byte for byte (`:485`); restore happens before the lift is recorded and a failed restore leaves the lockdown active and unbans nobody (`:485-498`). `resolveUnmutePermissions` (`chat_permissions.go:66`) returns the stored snapshot during an active lockdown and errors on a failed lookup; all four callers (`mute.go:228`, `bans.go:817`, `captcha.go:1396`, `staff_action_run.go:688`) go through it, and the staff-mute undo reaches it via `executeStaffCall`. Tests: `TestLockdownLiftRestoresExactSnapshot`, `TestLockdownLiftRestoreFailure`, `TestLockdownLiftRecordFailure`, `TestUnmuteDuringLockdownUsesSnapshot` (all four callers), `TestUnmuteLockdownLookupFails`, all PASS. Fidelity against a real `getChat` answer is a human item (A1) |
| 6 | Only a live creator or administrator with `can_restrict_members` can lock or lift; admin cache, service IDs and AnonAdmin mode never authorise; a failed lookup refuses; anonymous admins always get the proof button and the tapper is the one recorded (LOCK-06, D-11, D-12) | VERIFIED | `requireLockdownAuthority` (`lockdown.go:127-166`): live `getChatMember` only, no admin-cache symbol anywhere in the file. `TestLockdownCommandsRefuseNonAuthority` includes "cache says admin, live says member" and "lookup fails"; `TestLockdownAnonymousAdminProof` PASS |
| 7 | Refusals record nothing: basic group, bot lacks `can_restrict_members`, bot rights unreadable, `getChat` fails or has no permissions, 4xx refusal of the lock (D-20) | VERIFIED | Order in `lockdown.go:306-351` writes nothing before `lockdown.Start`; 4xx refusal deletes the unconfirmed row (`:386`). `TestLockdownRefusals` (all subtests) PASS |
| 8 | A join is handled once whichever path it arrives on (chat_member, service message, join request); only an admin-added human gets in; bots are always banned; no welcome, captcha or goodbye for a removed joiner; join requests are declined by the worker and the Accept button refuses (D-03..D-10, D-24, D-25) | VERIFIED | `decideLockdownJoin` (`lockdown_guard.go:60`) is a pure policy; dedupe is the `(lockdown_id, user_id)` row, never Redis (`lockdownExistingJoin` `:465`); request path fails closed (`:288-326`); Accept refusal in `greetings.go:966-975`; antiraid steps aside (`antiraid.go:459`). Tests: `TestDecideLockdownJoin`, `TestLockdownGuardDedupesTwoPaths`, `TestLockdownMixedServiceMessage`, `TestLockdownApprovalRace`, `TestLockdownSuppressesGoodbye`, `TestLockdownGuardDeclinesJoinRequest`, `TestLockdownJoinRequestFailsClosed`, `TestLockdownAcceptButtonRefuses`, `TestAntiRaidStepsAsideDuringLockdown`, all PASS |
| 9 | A deliberate ban is never lifted: the worker keeps (no ban call) a joiner already kicked on another date, and the lift keeps any ban that is not the lockdown's own, including a staff `/ban` or `/tban` placed over a lockdown ban (D-05, CR-01 fix) | VERIFIED | `lockdownBanOne` reads live `getChatMember` inside the same paced unit (`lockdown_worker.go:373-392`); unreadable status never turns into a ban; `isLockdownBan` (`lockdown_marker.go:20`); staff branch `staff_action_decide.go:216-219` + `staff_action_run.go:548-559`. Tests: `TestLockdownWorkerKeepsDeliberateBanPlacedBeforeIt`, `TestLockdownWorkerRecognisesItsOwnBanAlreadyInPlace`, `TestLockdownWorkerDoesNotBanWhenStatusUnreadable`, `TestLockdownLiftKeepsDeliberateBan`, `TestDecideStaffBanLockdownJoiner`, `TestStaffBanOnLockdownJoinerSurvivesLift`, `TestStaffBanLockdownLookupFails`, all PASS |
| 10 | An ambiguous lock failure (timeout, dropped connection, 5xx, unreadable answer) keeps the row holding the only snapshot, says the outcome is unknown and lets `/unlockdown` restore at once (WR-01 fix) | VERIFIED | `lockdownDefinitiveRefusal` (`lockdown.go:423-426`) deletes only on a 4xx `TelegramError`; else `lockdown_lock_unknown`. `TestLockdownLockOutcomeUnknown` (3 subtests: applied then lost, settled by the worker, not applied then dropped) PASS |
| 11 | An unconfirmed lockdown row left by a half-finished `/lockdown` is settled from the live permissions after the grace period and never announced; joiner bans last at most 330 days and a lapsed ban is reported as expired, not as "left on purpose" (WR-02 fix) | VERIFIED | `settleUnconfirmedLockdown` (`lockdown_worker.go:100-129`); `lockdownBanExpired` (`lockdown_marker.go:47`) and `JoinerDetailBanExpired` tally path (`lockdown_worker.go:635-651`); limit documented in AGENTS.md and the generated docs page. Tests: `TestLockdownSettlesUnconfirmedLock`, `TestLockdownLiftReportsExpiredBans`, `TestCountJoinersWithDetail`, all PASS |
| 12 | At most one active lockdown per chat, on SQLite and on PostgreSQL 16 (`uk_chat_lockdowns_active` partial unique index plus `ON CONFLICT DO NOTHING`) | ? UNCERTAIN | Migration `20261006120000` contains the index (`CREATE UNIQUE INDEX IF NOT EXISTS uk_chat_lockdowns_active ... WHERE state = 'active'`) and `Start` is the single conditional insert (`repository.go:57-71`). `TestStartLockdownOneActivePerChat` (8 concurrent starts, raw duplicate rejected) PASSES on SQLite. The PostgreSQL run was never executed (sandbox refuses `su postgres`), so the production-database half of the truth is unproven. Routed to human verification |

**Score:** 11/12 truths verified (0 present-but-behavior-unverified, 1 uncertain)

Behavior-dependent truths (state transitions, ordering and cancellation invariants) were each tied to a named, passing behavioural test above. None was marked VERIFIED on presence alone. The two advisory `coincidental_reliance_items` are about the fakes standing in for Telegram, not about missing tests.

### Deferred Items

None. The Phase 5 (alert, Lift button, Revoke link) and Phase 6 (LOCK-10, LOCK-15) scope that the phase goal excludes is named in ROADMAP and was not counted as a gap here.

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `migrations/20261006120000_add_chat_lockdowns.sql` | Two tables, partial unique index, CHECKs, no top-level transaction | VERIFIED | 86 lines; sorts last in `migrations/`; idempotent statements; no `BEGIN`/`COMMIT` |
| `alita/db/models/lockdown.go` | `ChatLockdown`, `LockdownJoiner`, state constants, `TableName()` | VERIFIED | 123 lines; also carries `JoinerDetailBanExpired` (WR-02) |
| `alita/db/lockdown/repository.go` | `Start`, `GetActiveFresh`, `ConfirmLocked`, `BeginLift`, `FinishLift`, ... | VERIFIED | 294 lines; every state move is one conditional update; no cache symbol |
| `alita/db/lockdown/joiners.go` | `RecordJoin`, `ClaimJoiner`, `MoveJoiner`, `ReleaseStaleClaims`, `HasJoinerBanFresh`, ... | VERIFIED | 375 lines; substantive, wired from guard, worker and staff |
| `alita/modules/lockdown_perms.go` | Raw `getChat` / `setChatPermissions`, locked set, `canonicalPermissions` | VERIFIED | Never uses `gotgbot.ChatPermissions` (comment mentions only) |
| `alita/modules/lockdown.go` | `/lockdown`, `/unlockdown`, authority, anonymous-admin registration | VERIFIED | `RegisterLegacyModule("Lockdown", 238, ...)` and three `RegisterAnonymousAdminHandler` calls present; commands via `helpers.WrapCommand`, not Disableable |
| `alita/modules/lockdown_guard.go` | Group `-7` guard for all three join paths and the kicked update | VERIFIED | 574 lines; handlers registered at `lockdownModule.handlerGroup` (-7) |
| `alita/modules/lockdown_worker.go` | Paced worker, bans, declines, lift unbans, tally, settle | VERIFIED | 687 lines; started in `main.go:442`, drained in `main.go:224` after DB-close |
| `alita/modules/lockdown_marker.go` | `isLockdownBan`, `lockdownBanUntil`, `lockdownBanExpired` | VERIFIED | 49 lines |
| `alita/modules/lockdown_status.go` | `/lockdownstatus` | VERIFIED | 129 lines; read-only |
| `alita/modules/chat_permissions.go` | Lockdown-aware `resolveUnmutePermissions` returning `(perms, error)` | VERIFIED | Single choke point, 4 callers |
| `alita/modules/staff_panel.go` | `/staff` lockdown marker | VERIFIED | `LockedSince`, `markLockedRows`, one batch query |
| `locales/*.yml` (7 files) | 38 `lockdown_*` keys each plus panel and mute keys | VERIFIED | `grep -c '^lockdown_'` = 38 in all seven; `make check-translations` PASS; `TestLockdownLocaleKeys` PASS |
| `docs/src/content/docs/commands/lockdown/index.md` | Generated page | VERIFIED | `make check-docs`: no drift |
| `Makefile` `test-postgres-integrity` | Lockdown test and package added | VERIFIED | Present; the target itself was not run (needs PostgreSQL) |

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|-----|--------|---------|
| `lockdown.go` | `lockdown.Start` before the lock call, then `ConfirmLocked` | snapshot written first | WIRED | `:363` before `:375` before `:391` |
| `lockdown.go` | `setLockdownPermissions(row.PrePermissions)` | raw replay | WIRED | `:485` |
| `lockdown.go` | `helpers.WrapCommand` + `requireLockdownAuthority` | pipeline | WIRED | `:57-64`, `:528-530` |
| `lockdown_guard.go` | `lockdown.RecordJoin` / `ReclaimJoin` / `SetJoinMsg` | row-based dedupe | WIRED | `lockdownRecordJoin`, `lockdown.ReclaimJoin`, `lockdown.SetJoinMsg` |
| `lockdown_worker.go` | `lockdown.ReleaseStaleClaims` first in every cycle | restart durability | WIRED | `runLockdownCycle` calls `lockdownReleaseStale()` first |
| `lockdown.go` | `settleUnconfirmedLockdown` | half-started lock | WIRED | `:320` |
| `chat_permissions.go` | `lockdown.GetActiveFresh` via `lockdownSnapshotLookup` | D-23 choke point | WIRED | `:54`, `:68` |
| `staff_action_run.go` | `lockdown.HasJoinerBanFresh` via `staffLockdownBanLookup` | D-05 staff ban | WIRED | `:523`, `:553` |
| `staff_panel.go` | `lockdown.ListActiveByChatsFresh` | `/staff` marker | WIRED | `markLockedRows` called in `buildStaffPanel` |
| `greetings.go` / `antiraid.go` | `lockdown.GetActiveFresh` | Accept refusal, antiraid step-aside | WIRED | `greetings.go:970`, `antiraid.go:459` |
| `main.go` | `StartLockdownWorker` / `StopLockdownWorker` | lifecycle | WIRED | `:442` and `:224-226`; DB-close handler at `:160` runs last (LIFO) |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
|----------|---------------|--------|--------------------|--------|
| `/lockdown` notice | reason, name, notes | command args, `staffFullName(actor)`, live bot member | Yes | FLOWING |
| `/lockdownstatus` | row, tally, live permissions | `GetCurrentFresh`, `TallyJoiners`, raw `getChat` | Yes (DB and Telegram) | FLOWING |
| `/staff` marker | `LockedSince` | `ListActiveByChatsFresh` (one query) | Yes | FLOWING |
| Worker bans | pending rows | `ListPendingFresh` joined to confirmed active lockdowns | Yes | FLOWING |
| Lift restore | `row.PrePermissions` | raw `getChat` answer stored before the lock | Yes (fidelity vs real Telegram is a human item) | FLOWING |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| Lockdown, joiner, unmute, staff and permission tests | `go test -tags testtools -race -count=1 -run 'Lockdown\|Joiner\|Unmute\|StaffPanel\|DecideStaffBan\|CanonicalPermissions\|IsLockdownBan' -v ./alita/modules ./alita/db/lockdown` | `ok` for both packages, 296 PASS lines, 0 FAIL, 0 SKIP | PASS |
| One-lifter concurrency and locale parity | `go test -tags testtools -race -count=5 -run '^TestUnlockdownLiftsOnce$' ./alita/modules`; `-run '^TestStaffPanelShowsLockedGroup$\|^TestUnlockdown\|^TestLockdownLocaleKeys$' ./alita/modules ./alita/i18n` | 5 of 5 PASS; all PASS | PASS |
| Build and vet | `CGO_ENABLED=0 go build ./...`; `go vet -tags testtools ./alita/modules ./alita/db/lockdown ./alita/db/models` | clean | PASS |
| Translations and docs | `make check-translations`; `make check-docs` (writes only to a temp dir) | all translations present; no drift | PASS |
| Dependencies unchanged | `git diff --exit-code go.mod go.sum` | unchanged | PASS |
| Full suite | not re-run (single-run rule). Orchestrator reported `make test` exit 0 on this head, 61 packages ok | taken from the orchestrator | NOT RE-RUN |

### Probe Execution

Step 7c: SKIPPED. No `scripts/*/tests/probe-*.sh` exists for this phase and no plan declares a probe.

### Test Quality Audit

| Test file group | Linked req | Active | Skipped | Circular | Assertion level | Verdict |
|-----------------|-----------|--------|---------|----------|-----------------|---------|
| `lockdown_test.go`, `lockdown_lift_test.go`, `lockdown_refusals_test.go`, `lockdown_lock_unknown_test.go` | LOCK-02, 06, 07, 09 | all active | 0 | No (expected values are the constants `lockdownTestPrePermissions` / `lockdownLockedPermissions`, hand-written, not produced by the code under test) | Behavioral (call order, stored bytes, rows, replies) | OK |
| `lockdown_guard_test.go`, `lockdown_paths_test.go`, `lockdown_requests_test.go`, `lockdown_lift_joiners_test.go`, `lockdown_ban_live_test.go`, `lockdown_durability_test.go`, `lockdown_isolation_test.go` | LOCK-01, 04, 05, 07 | all active | 0 | No | Behavioral | OK; end dates in the fake are test-set (see coincidental-reliance advisory) |
| `lockdown_unmute_test.go`, `chat_permissions_test.go` | LOCK-08 | all active | 0 | No | Behavioral over all four callers | OK |
| `lockdown_staff_test.go`, `staff_panel_render_test.go`, `staff_action_decide_test.go` | SETUP-08, D-05 | all active | 0 | No | Behavioral / value | OK |
| `alita/db/lockdown/*_test.go` | LOCK-05 | all active | 0 on SQLite | No | Behavioral | PostgreSQL leg not run |

**Disabled tests on requirements:** 0. **Circular patterns detected:** 0. **Insufficient assertions:** 0. I also read the CR-01 and WR-01 regression tests in full: they assert call counts, row states, stored permission bytes and the post-lift member status, not literals.

### Decision Coverage

`gsd-tools query check.decision-coverage-verify`: 25 of 25 CONTEXT.md decisions honored by shipped artifacts, none missing (non-blocking gate).

### Requirements Coverage

Every ID in the PLAN frontmatter is accounted for, and every REQUIREMENTS.md ID mapped to Phase 4 appears in at least one plan. No orphans.

| Requirement | Source Plans | Description | Status | Evidence |
|-------------|--------------|-------------|--------|----------|
| LOCK-01 | 04-03, 04-04, 04-05 | Joiners removed at once, can rejoin after the lift, recorded for the alert | SATISFIED (the "listed on the alert" part is Phase 5; the joiner rows it will read are recorded) | Truths SC1 and 8; join rows carry name, username, link, path, performer |
| LOCK-02 | 04-01 | Everyone except admins muted | SATISFIED | SC1; `TestLockdownLockSendsLockedSet` |
| LOCK-03 | 04-01 | Approved users talk, or the owner's fallback | SATISFIED via the owner's fallback D-01/D-02, stated in the notice | SC2 |
| LOCK-04 | 04-05 | Affects only its own group | SATISFIED | `TestLockdownAffectsOnlyItsChat`; all queries keyed by chat or lockdown |
| LOCK-05 | 04-02, 04-03, 04-06 | Never lifts on its own; survives restarts and Redis loss | SATISFIED in code and fakes; live drills are human items | SC4 |
| LOCK-06 | 04-01, 04-02 | Only an admin lifts it (`/unlockdown`; the alert button is Phase 5) | SATISFIED for the Phase 4 scope | Truth 6 |
| LOCK-07 | 04-01, 04-02, 04-03, 04-08 | Lift restores permissions exactly | SATISFIED in code; real `getChat` fidelity is a human item | SC5, truth 9 |
| LOCK-08 | 04-07 | Unmuted during a lockdown can talk after the lift | SATISFIED | SC5; `TestUnmuteDuringLockdownUsesSnapshot` |
| LOCK-09 | 04-01, 04-02 | `/lockdown` and current status | SATISFIED | SC3 |
| SETUP-08 (deferred in-lockdown status) | 04-08 | `/staff` shows a group in lockdown | SATISFIED in code and render tests; live rendering is a human item | SC3 |

Out of scope and correctly unclaimed: LOCK-10, LOCK-15 (Phase 6), LOCK-11..14 (Phase 5).

**Bookkeeping drift (not a code gap):** `.planning/REQUIREMENTS.md` still shows LOCK-01..LOCK-09 as `[ ]` and "Pending" in the traceability table (lines 59-67 and 194-202). They should be ticked and set to Complete when the phase closes, after the human items are accepted.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| (all phase Go, SQL and migration files) | - | `TBD`/`FIXME`/`XXX`/`TODO`/`HACK`/`PLACEHOLDER` | none found | No debt markers; no stubs: empty-return and `[]`/`{}` initialisers are real accumulators, not rendered placeholders |
| `AGENTS.md` (Data bullet) vs `alita/modules/chat_permissions.go:73-77` | - | Rule says `pre_permissions` is "never passed through the typed `gotgbot.ChatPermissions`" but `resolveUnmutePermissions` unmarshals it into that type (open review finding IN-02) | Info | Doc and code disagree; the code is defensible (per-user restriction, same as before the phase) |
| `alita/modules/lockdown_guard.go:60-90` | - | `lockdownNeedsPerformerLookup` restates `decideLockdownJoin`'s conditions; the `lockdownJoinDecline` verdict is unreachable from production (IN-01) | Info | Drift risk only; a drift fails closed (ban) |
| `alita/modules/lockdown_guard.go:19-21` | - | Fourth copy of the Group Anonymous Bot ID (IN-03) | Info | Drift risk only |
| `alita/modules/lockdown_guard.go:266-278`, `lockdown_worker.go:342-345` | - | A fed-banned joiner of a locked group is handled only by the lockdown (fed-ban at `-6` never sees them), and a "request already gone" is counted as declined (IN-04) | Info | Behaviour follows D-03/D-10; the fed-ban interaction is not documented in AGENTS.md or the docs page |

No blockers. No warnings in the code.

### Human Verification Required

See the `human_verification` list in the frontmatter (14 items). In order of how much of the phase goal rests on them:

1. **PostgreSQL 16 run** (migration chain plus `TestStartLockdownOneActivePerChat`). Never executed, only SQLite. Truth 12 stays UNCERTAIN until it passes.
2. **Real `getChat` permission round trip** (research A1). "Settings survive" depends on it.
3. **Real `until_date` echo** (research A2). "The lift unbans only the lockdown's own ban" depends on it; if Telegram rounds by more than 2 s every raider would be left banned at the lift.
4. **Live joins on all three paths**, the D-24 admin-approval case, and the re-request after a decline (A3, A5, A11).
5. **`make lint`** on a Go 1.26 toolchain.
6. **`/staff` marker, anonymous-admin tap, Accept button, two-group isolation, staff `/tban` over a lockdown ban, restart, Redis-flush and two-replica drills.**
7. **LOCK-08 live check:** a user unmuted during the lockdown can talk after the lift.

### Gaps Summary

There are no gaps. Nothing the phase goal or the five success criteria require is missing, stubbed or unwired. The remaining risk is entirely in what the test environment cannot show: the production database (PostgreSQL), the lint gate, and the live behaviour of Telegram that the fakes assume. The two `coincidental_reliance_items` mark the two assumptions (A1 and A2) where a wrong guess would silently break a headline promise, so capturing real answers as fixtures is the most valuable hardening step.

Open review items IN-01..IN-04 are info-level and do not block. IN-02 and IN-04 are worth a documentation line in AGENTS.md (the typed-struct sentence, and the fed-ban interaction).

One pre-existing issue outside this phase is recorded in `deferred-items.md` (anonymous-admin re-entry loses the chat for `RequireGroup` in the `ban` family). Phase 4 works around it inside the lockdown commands; it should be triaged separately.

---

_Verified: 2026-10-07T04:40:00Z_
_Verifier: Claude (gsd-verifier)_
