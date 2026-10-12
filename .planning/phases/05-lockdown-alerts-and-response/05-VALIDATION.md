---
phase: "5"
slug: "lockdown-alerts-and-response"
# status lifecycle: draft (seeded by plan-phase) → validated (set by validate-phase §6)
# audit-milestone §5.5 distinguishes NOT-VALIDATED (draft) from PARTIAL (validated + nyquist_compliant: false) (#2117)
status: draft
nyquist_compliant: false
wave_0_complete: false
created: "2026-10-11"
---

# Phase 5 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go `testing` with build tag `testtools`, SQLite via `internal/testdb`, miniredis, hand-written `gotgbot.BotClient` fakes (`lockdownFake`) |
| **Config file** | none; `-tags testtools` is mandatory (AGENTS.md) |
| **Quick run command** | `go test -tags testtools -race -count=1 -run '^TestLockdown' ./alita/modules ./alita/db/lockdown` |
| **Full suite command** | `make test` |
| **Estimated runtime** | ~60 seconds quick, ~6 minutes full |

---

## Sampling Rate

- **After every task commit:** Run the quick run command, plus `go vet` and `gofmt -l alita` (the sandbox's golangci-lint is built with Go 1.25)
- **After every plan wave:** Run `go test -tags testtools -race -count=1 ./alita/...`
- **Before `/gsd-verify-work`:** `make test`, `make check-translations` and `make check-docs` must be green
- **Max feedback latency:** 60 seconds

---

## Per-Task Verification Map

Bound to the plans 05-01 to 05-14. Test files are created by the task that lists them (no separate Wave 0 plan): each task writes its failing tests first.

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 05-01-T1 | 05-01 | 1 | LOCK-11 | T-05-01, T-05-02 | /lockdown in a linked group posts one alert in the Staff Group (name, trigger, reason escaped); second cycle posts nothing; nothing posted into the group | integration | `go test -tags testtools -race -count=1 -run '^TestLockdownAlertPostsOnce$' ./alita/modules` | ❌ created by task | ⬜ pending |
| 05-01-T2 | 05-01 | 1 | LOCK-11 | T-05-03 | No destination posts nothing; a Staff Group linked later gets nothing; a lockdown lifted before posting is never alerted; older lockdowns alerted once; a failed send retried | integration | `go test -tags testtools -race -count=1 -run '^TestLockdownAlertGate$' ./alita/modules` | ❌ created by task | ⬜ pending |
| 05-02-T1 | 05-02 | 2 | LOCK-06 | T-05-04, T-05-05, T-05-06, T-05-07 | Lift from the Staff Group by a live admin of the locked group; non-admin refused; asker-only Confirm; restore precedes record; presser recorded; group gets the lift notice (D-19) | integration | `go test -tags testtools -race -count=1 -run '^TestLockdownAlertLift$' ./alita/modules` | ❌ created by task | ⬜ pending |
| 05-02-T2 | 05-02 | 2 | LOCK-06 | T-05-05, T-05-06 | Restore failure, record failure and a lost race reported on the card; Cancel asker-only; /unlockdown unchanged | integration | `go test -tags testtools -race -count=1 -run '^TestLockdownAlertLiftOutcomes$\|^TestLockdownLiftManualChange$\|^TestUnlockdownLiftsOnce$' ./alita/modules` | ❌ created by task | ⬜ pending |
| 05-03-T1 | 05-03 | 3 | LOCK-06 | T-05-09 | Lift via command or button closes every copy once, buttons removed; gone, not-modified and rate-limited edits handled | integration | `go test -tags testtools -race -count=1 -run '^TestLockdownAlertClosedAtLift$' ./alita/modules` | ❌ created by task | ⬜ pending |
| 05-03-T1 | 05-03 | 3 | repo | T-05-02 | BeginAlert once; slot key unique; copy claims and attempts; close claims newest first, once | repository | `go test -tags testtools -race -count=1 -run '^TestBeginAlert$\|^TestAlertSlots$\|^TestAlertClose$' ./alita/db/lockdown` | ❌ created by task | ⬜ pending |
| 05-03-T2 | 05-03 | 3 | LOCK-06 | T-05-08 | A lifted lockdown's button answers "Already lifted by X at T", closes that copy, changes nothing; an old alert never lifts a newer lockdown | integration | `go test -tags testtools -race -count=1 -run '^TestLockdownAlertStaleButtons$' ./alita/modules` | ❌ created by task | ⬜ pending |
| 05-04-T1 | 05-04 | 4 | LOCK-06 | T-05-11, T-05-12, T-05-13 | Staff Group member not admin, admin without restrict, failed lookup, service IDs, forged chat, demoted before Confirm: all refused, nothing changes; creator allowed | integration | `go test -tags testtools -race -count=1 -run '^TestLockdownAlertLiftAuthority$' ./alita/modules` | ❌ created by task | ⬜ pending |
| 05-04-T2 | 05-04 | 4 | LOCK-06 | T-05-10, T-05-13 | Late Confirm expired; double Confirm lifts once; two askers lift once; bad fields refused; works without Redis; one answer per press | integration | `go test -tags testtools -race -count=1 -run '^TestLockdownAlertConfirmCard$' ./alita/modules` | ❌ created by task | ⬜ pending |
| 05-04-T2 | 05-04 | 4 | all | T-05-13 | Callback data fits 64 bytes with 16-digit IDs; unknown action answered once; undecodable data answered invalid | unit | `go test -tags testtools -race -count=1 -run '^TestLockdownCallbackData$' ./alita/modules` | ❌ created by task | ⬜ pending |
| 05-05-T1 | 05-05 | 4 | LOCK-11 | T-05-16 | Both join paths record the joiner in the 10-minute tracker; snapshot returns name, @username, bot flag, link; the bot itself is not tracked; guard unchanged | integration | `go test -tags testtools -race -count=1 -run '^TestLockdownRecentTracked$' ./alita/modules` | ❌ created by task | ⬜ pending |
| 05-05-T2 | 05-05 | 4 | LOCK-11 | T-05-14, T-05-15 | Tracker dedupe across join paths, 10-minute trim, 500 cap, TTL; no Redis or a Redis failure is silent and the guard still records and bans | unit | `go test -tags testtools -race -count=1 -run '^TestLockdownRecentJoins$' ./alita/modules` | ❌ created by task | ⬜ pending |
| 05-06-T1 | 05-06 | 5 | LOCK-11 | T-05-17 | Alert to Staff Group and log channel (header, admin category on); category off; log only; neither; linked later gets nothing | integration | `go test -tags testtools -race -count=1 -run '^TestLockdownAlertDestinations$' ./alita/modules` | ❌ created by task | ⬜ pending |
| 05-06-T1 | 05-06 | 5 | LOCK-06 | T-05-04 | Lift from the log channel copy works for a locked-group admin; both copies close | integration | `go test -tags testtools -race -count=1 -run '^TestLockdownAlertLiftFromLogChannel$' ./alita/modules` | ❌ created by task | ⬜ pending |
| 05-06-T2 | 05-06 | 5 | LOCK-11 | T-05-18, T-05-19 | Send failure retried, third failure final without blocking the other copy; rate limit free; stale posting and close claims released; concurrent cycles post once | integration | `go test -tags testtools -race -count=1 -run '^TestLockdownAlertDelivery$' ./alita/modules` | ❌ created by task | ⬜ pending |
| 05-06-T2 | 05-06 | 5 | LOCK-11 | T-05-19 | Settle path (worker-confirmed lockdown) also alerts; a deleted unconfirmed row never does | integration | `go test -tags testtools -race -count=1 -run '^TestLockdownAlertAfterSettle$' ./alita/modules` | ❌ created by task | ⬜ pending |
| 05-06-T2 | 05-06 | 5 | repo | T-05-19 | Stale alert claims released without an attempt | repository | `go test -tags testtools -race -count=1 -run '^TestReleaseStaleAlertClaims$' ./alita/db/lockdown` | ❌ created by task | ⬜ pending |
| 05-07-T1, 05-07-T2 | 05-07 | 6 | LOCK-11, LOCK-01 | T-05-20, T-05-21 | Sample: pre-lockdown joiners (saved at the alert) then recorded joiners, 10 max, "and N more", bots marked, names escaped, counts line; no Redis or a failed lookup still posts | integration | `go test -tags testtools -race -count=1 -run '^TestLockdownAlertSample$' ./alita/modules` | ❌ created by task | ⬜ pending |
| 05-07-T1 | 05-07 | 6 | repo | T-05-20 | Burst sample order, dedupe of pre and joiner rows, counts; BeginAlert inserts burst rows only when it wins | repository | `go test -tags testtools -race -count=1 -run '^TestBurstSample$' ./alita/db/lockdown` | ❌ created by task | ⬜ pending |
| 05-08-T1 | 05-08 | 7 | LOCK-12 | T-05-23 | Decision table: lockdown's own ban upgraded, permanent or other end date left alone, left/member/restricted banned, admins skipped | unit | `go test -tags testtools -race -count=1 -run '^TestDecideLockdownResponseBan$' ./alita/modules` | ❌ created by task | ⬜ pending |
| 05-08-T1 | 05-08 | 7 | LOCK-12 | T-05-22, T-05-23 | One account: banned with no end date and no revoke_messages, or skipped (admin, approved, already banned, let in by an admin with no Telegram call (D-22), bot itself); bots and declined requests banned; failures counted, rate limit free | integration | `go test -tags testtools -race -count=1 -run '^TestLockdownResponseBanOne$' ./alita/modules` | ❌ created by task | ⬜ pending |
| 05-08-T1 | 05-08 | 7 | repo | T-05-22 | Burst claims conditional; joiner lookup by user | repository | `go test -tags testtools -race -count=1 -run '^TestBurstClaims$' ./alita/db/lockdown` | ❌ created by task | ⬜ pending |
| 05-08-T2 | 05-08 | 7 | LOCK-12 | T-05-22 | Lift keeps a response-banned joiner banned (no unban call), tally counts it as kept | integration | `go test -tags testtools -race -count=1 -run '^TestLockdownBanSurvivesLift$' ./alita/modules` | ❌ created by task | ⬜ pending |
| 05-08-T2 | 05-08 | 7 | LOCK-12 | T-05-22, T-05-23 | Races: response ban vs worker ban vs lift never overwrites a permanent ban; row turned exempt after queueing skipped; Pitfall 3 characterization | integration | `go test -tags testtools -race -count=1 -run '^TestLockdownBanRaces$' ./alita/modules` | ❌ created by task | ⬜ pending |
| 05-08-T2 | 05-08 | 7 | LOCK-12 | — | /lockdownstatus removed count includes response bans (Pitfall 4) | integration | `go test -tags testtools -race -count=1 -run '^TestLockdownStatusCountsResponseBans$' ./alita/modules` | ❌ created by task | ⬜ pending |
| 05-08-T3 | 05-08 | 7 | repo | T-05-23, T-05-24 | Ban run start once while running; queue only unhandled, non-excluded rows; exempt joiners marked skipped let_in and left out of N (D-22); finish once; tally; stale burst claims released | repository | `go test -tags testtools -race -count=1 -run '^TestBanRunStart$\|^TestReleaseStaleBurstClaims$' ./alita/db/lockdown` | ❌ created by task | ⬜ pending |
| 05-09-T1, 05-09-T2 | 05-09 | 7 | LOCK-13 | T-05-25 | Alert shows top link, count, "+N other links"; none when Telegram reported none; cut-short link flagged; link escaped; link stored on the copy | integration | `go test -tags testtools -race -count=1 -run '^TestLockdownAlertInviteLink$' ./alita/modules` | ❌ created by task | ⬜ pending |
| 05-09-T1 | 05-09 | 7 | repo | — | Top links merge pre and joiner rows and order by count | repository | `go test -tags testtools -race -count=1 -run '^TestTopInviteLinks$' ./alita/db/lockdown` | ❌ created by task | ⬜ pending |
| 05-10-T1 | 05-10 | 8 | LOCK-12 | T-05-27 | Button, asker-only card and run ban the burst for good; live admins, approved users, people an admin let in and the bot excluded; bots and declined requests banned; result on the card | integration | `go test -tags testtools -race -count=1 -run '^TestLockdownBanRecentJoiners$' ./alita/modules` | ❌ created by task | ⬜ pending |
| 05-10-T1 | 05-10 | 8 | repo | T-05-28 | Approved users read fresh and uncached, per chat | repository | `go test -tags testtools -race -count=1 -run '^TestListApprovedIDsFresh$' ./alita/db/approvals` | ❌ created by task | ⬜ pending |
| 05-10-T2 | 05-10 | 8 | LOCK-12 | T-05-28, T-05-29 | N excludes approved, exempt and the bot; a failed approvals read refuses the Ask and bans nobody in a run; nothing left; already running | integration | `go test -tags testtools -race -count=1 -run '^TestLockdownBanCount$' ./alita/modules` | ❌ created by task | ⬜ pending |
| 05-11-T1 | 05-11 | 9 | LOCK-12 | T-05-30 | Second press bans only unhandled accounts and N counts only those; resumes after restart; stops when lifted | integration | `go test -tags testtools -race -count=1 -run '^TestLockdownBanRun$' ./alita/modules` | ❌ created by task | ⬜ pending |
| 05-11-T1 | 05-11 | 9 | LOCK-12 | T-05-32 | Ban card: no restrict refused, demoted before Confirm refused, other user's Confirm/Cancel refused, expired card, service identity refused | integration | `go test -tags testtools -race -count=1 -run '^TestLockdownBanAuthority$' ./alita/modules` | ❌ created by task | ⬜ pending |
| 05-11-T1 | 05-11 | 9 | repo | T-05-30 | Unqueue a stopped run; stopped finish once | repository | `go test -tags testtools -race -count=1 -run '^TestUnqueueBanRun$' ./alita/db/lockdown` | ❌ created by task | ⬜ pending |
| 05-11-T2 | 05-11 | 9 | LOCK-12 | T-05-31 | Result reports banned (declined requests apart), skipped by reason (admin, approved, already banned, let in by an admin), failed by name; one #LOCKDOWN_BAN log post per run; no staff audit row | integration | `go test -tags testtools -race -count=1 -run '^TestLockdownBanResult$' ./alita/modules` | ❌ created by task | ⬜ pending |
| 05-12-T1 | 05-12 | 10 | LOCK-13 | T-05-33, T-05-34, T-05-36 | Revoke by a live admin with can_invite_users; link from the copy row; recorded once; every live copy says "Link revoked by X" and drops Revoke; cut-short link has no button | integration | `go test -tags testtools -race -count=1 -run '^TestLockdownRevokeLink$' ./alita/modules` | ❌ created by task | ⬜ pending |
| 05-12-T1 | 05-12 | 10 | repo | T-05-36 | Revoke recorded once and marks live copies; sync claims | repository | `go test -tags testtools -race -count=1 -run '^TestRecordLinkRevoked$\|^TestAlertSync$' ./alita/db/lockdown` | ❌ created by task | ⬜ pending |
| 05-12-T2 | 05-12 | 10 | LOCK-13 | T-05-33, T-05-35 | Restrict-only admin, staff member, service ID, failed lookup refused; Telegram refusals mapped to plain lines; nothing recorded | integration | `go test -tags testtools -race -count=1 -run '^TestLockdownRevokeLinkRefusals$' ./alita/modules` | ❌ created by task | ⬜ pending |
| 05-13-T1 | 05-13 | 11 | LOCK-14 | T-05-37, T-05-39 | One reminder per hour per destination, one replica posts, previous reminder loses buttons, alert keeps its, none after lift, no catch-up, stale pending superseded | integration | `go test -tags testtools -race -count=1 -run '^TestLockdownReminders$' ./alita/modules` | ❌ created by task | ⬜ pending |
| 05-13-T1 | 05-13 | 11 | repo | T-05-37 | Reminder slots once per seq and destination; supersede marks only earlier reminders of one destination | repository | `go test -tags testtools -race -count=1 -run '^TestReminderSlots$' ./alita/db/lockdown` | ❌ created by task | ⬜ pending |
| 05-13-T2 | 05-13 | 11 | LOCK-14 | T-05-38 | Reminder content (since, reason, removed incl. response bans, pending, no sample), working buttons, destinations limited to the alert's kinds and re-resolved | integration | `go test -tags testtools -race -count=1 -run '^TestLockdownReminderContent$\|^TestLockdownReminderDestinations$' ./alita/modules` | ❌ created by task | ⬜ pending |
| 05-14-T2 | 05-14 | 12 | platform | T-05-02, T-05-24 | Exactly-once gate and one running ban run proven on PostgreSQL (CI) | chain | `make test-postgres-integrity` (needs PostgreSQL; `-run` list gains TestBeginAlert and TestBanRunStart) | ✅ existing | ⬜ pending |
| 05-14-T2 | 05-14 | 12 | platform | — | Migrations 20261011120000-20261011150000 apply on PostgreSQL | chain | `ALITA_TEST_MIGRATION_CHAIN=true DATABASE_URL=... go test -tags testtools -run '^TestRepositoryMigrationChain$' ./alita/db/migrations` | ✅ existing | ⬜ pending |
| every plan with locale keys; 05-14-T1 | all | 1-12 | PLAT-03 | — | New keys in all 7 locale files with identical placeholders | script | `go test -tags testtools -count=1 -run '^TestLockdownLocaleKeys$' ./alita/i18n && make check-translations` | ✅ existing | ⬜ pending |
| 05-14-T1 | 05-14 | 12 | docs | T-05-40 | Help text (no claim the ban does not make) reflected in generated docs | script | `make check-docs` (fix: `make generate-docs`) | ✅ existing | ⬜ pending |
| 05-14-T2 | 05-14 | 12 | all | — | Full suite at the phase gate | suite | `make test` | ✅ existing | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

Covered inside the plans' tasks rather than a separate wave (each task's RED step):

- [ ] `ChatLockdownAlert` (05-01-T1 modules harness; 05-03-T1 lockdown harness), `ChatLockdownBurst` (05-07-T1), `ChatLockdownBanRun` (05-08-T3 lockdown harness; 05-10-T1 modules harness) in the AutoMigrate lists of `alita/modules/test_harness_test.go` and `alita/db/lockdown/testmain_test.go`; the `alita/db` package's own tests never touch these tables, so its list is left alone; each test file's helper deletes the rows of its chats
- [ ] Test helpers: `linkLockdownStaffGroup` (05-01), `pressLockdownButton` and `lockdownButtonData` (05-02); log channel through the existing `setStaffLogChannel` (05-06)
- [ ] `lockdownFake`: `revokeChatInviteLink` answer, `setMemberRaw` and `lockdownAdminJSON` (05-12-T1); `editMessageText` and scripted FIFO errors already exist in the staff fake
- [ ] Test seams as package variables: `lockdownRecentLookup` (05-05-T1), `lockdownApprovedLookup` (05-10-T1), `lockdownReminderEvery` (05-13-T1); alert aging by direct update inside the reminder tests

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| The alert renders correctly in a real Staff Group and a real log channel (channel post with buttons) | LOCK-11 | Live Telegram rendering of channel posts and callback presses from a channel | Lock a linked group with a log channel; check both copies, then press each button from both places |
| "Revoke link" on a bot-created link revokes it; another admin's link shows only the hint | LOCK-13 | Telegram's invite_link reporting and revoke semantics are assumed from the gotgbot docs (research F2) | Join through a bot-created link and through an admin's link during a lockdown; press Revoke |
| Hourly reminder arrives and the previous one loses its buttons; nothing after the lift | LOCK-14 | Wall-clock behavior in production | Keep a test group locked for over an hour, then lift |
| Migration applies on PostgreSQL 16 | platform | No PostgreSQL server in the executor sandbox | Run the migration-chain test against PostgreSQL 16 |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 60s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
