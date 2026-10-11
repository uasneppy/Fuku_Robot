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

Seeded from 05-RESEARCH.md "Validation Architecture". The planner binds Task ID, Plan, Wave and Threat Ref.

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| TBD | TBD | TBD | LOCK-11 | TBD | Alert gate runs once; two cycles/replicas post one alert per destination | integration | `go test -tags testtools -race -count=1 -run '^TestLockdownAlertPostsOnce$' ./alita/modules` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | LOCK-11 | TBD | Alert to Staff Group and log channel; none when neither exists; log category off | integration | `go test -tags testtools -race -count=1 -run '^TestLockdownAlertDestinations$' ./alita/modules` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | LOCK-11 | TBD | Settle path (unconfirmed row confirmed by worker) also alerts | integration | `go test -tags testtools -race -count=1 -run '^TestLockdownAlertAfterSettle$' ./alita/modules` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | LOCK-11 | TBD | Sample: 10 max, "and N more", bots marked, names HTML-escaped, pre-lockdown members from tracker | unit/integration | `go test -tags testtools -race -count=1 -run '^TestLockdownAlertSample$' ./alita/modules` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | LOCK-11 | TBD | Send failure retries, stale claim reposts, third failure is final and does not block others | integration | `go test -tags testtools -race -count=1 -run '^TestLockdownAlertDelivery$' ./alita/modules` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | LOCK-11 | TBD | Recent-joins tracker: dedup across join paths, 10 min window, cap, no Redis is silent | unit | `go test -tags testtools -race -count=1 -run '^TestLockdownRecentJoins$' ./alita/modules` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | LOCK-06 | TBD | Lift from Staff Group and from log channel works for a locked-group admin; presser recorded as lifter; restore precedes record | integration | `go test -tags testtools -race -count=1 -run '^TestLockdownAlertLift$' ./alita/modules` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | LOCK-06 | TBD | Staff Group admin who is not an admin of the locked group is refused, nothing changes; non-restrict admin refused; failed lookup refuses; service ID refused | integration | `go test -tags testtools -race -count=1 -run '^TestLockdownAlertLiftAuthority$' ./alita/modules` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | LOCK-06 | TBD | Only the asker can Confirm; Cancel edits the card; late Confirm is expired | integration | `go test -tags testtools -race -count=1 -run '^TestLockdownAlertConfirmCard$' ./alita/modules` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | LOCK-06 | TBD | Already lifted/lifting answers "Already lifted by X at T", edits that alert, changes nothing; old alert never lifts a newer lockdown | integration | `go test -tags testtools -race -count=1 -run '^TestLockdownAlertStaleButtons$' ./alita/modules` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | LOCK-06 | TBD | Lift via command or button closes every alert and reminder copy once, buttons removed, resumes after restart; command path regression | integration | `go test -tags testtools -race -count=1 -run '^TestLockdownAlertClosedAtLift$' ; existing ^TestLockdownLiftManualChange$ ./alita/modules` | ✅ | ⬜ pending |
| TBD | TBD | TBD | LOCK-12 | TBD | Targets exclude admins, approved, bot, exempt rows; bots banned; permanent ban (no `until_date`) | integration | `go test -tags testtools -race -count=1 -run '^TestLockdownBanRecentJoiners$' ./alita/modules` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | LOCK-12 | TBD | Decision table: kicked with marker upgraded, kicked with other end date or permanent left alone, left/member/restricted banned | unit | `go test -tags testtools -race -count=1 -run '^TestDecideLockdownResponseBan$' ./alita/modules` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | LOCK-12 | TBD | Lift keeps a response-banned joiner banned (no unban call), tally says "on purpose" | integration | `go test -tags testtools -race -count=1 -run '^TestLockdownBanSurvivesLift$' ./alita/modules` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | LOCK-12 | TBD | Race: response ban vs worker ban vs lift never overwrites a permanent ban | integration | `go test -tags testtools -race -count=1 -run '^TestLockdownBanRaces$' ./alita/modules` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | LOCK-12 | TBD | Second press bans only unhandled accounts; N counts only those; only one run at a time; run resumes after restart; stops when lifted | integration | `go test -tags testtools -race -count=1 -run '^TestLockdownBanRun$' ./alita/modules` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | LOCK-12 | TBD | Result reports banned/skipped (reasons)/failed; one `#LOCKDOWN_BAN` log post | integration | `go test -tags testtools -race -count=1 -run '^TestLockdownBanResult$' ./alita/modules` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | LOCK-13 | TBD | Alert shows top link, count, "+N other links"; none when Telegram reported none | integration | `go test -tags testtools -race -count=1 -run '^TestLockdownAlertInviteLink$' ./alita/modules` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | LOCK-13 | TBD | Revoke: only live creator/admin with can_invite_users; truncated (`...`) link has no button; success edits "Link revoked by X"; Telegram refusal answered with a mapped line; once only | integration | `go test -tags testtools -race -count=1 -run '^TestLockdownRevokeLink$' ./alita/modules` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | LOCK-14 | TBD | One reminder per hour per destination, one replica posts, previous reminder loses buttons, none after lift, no catch-up after downtime | integration | `go test -tags testtools -race -count=1 -run '^TestLockdownReminders$' ./alita/modules` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | all | TBD | Callback data fits 64 bytes with 16-digit IDs; unknown action answered once | unit | `go test -tags testtools -race -count=1 -run '^TestLockdownCallbackData$' ./alita/modules` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | repo | TBD | `BeginAlert` once; slot uniqueness; claim/release; close claim; link revoke once; burst candidate query; run uniqueness | repository | `go test -tags testtools -race -count=1 -run '^(TestBeginAlert | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | platform | TBD | Migration applies on PostgreSQL | chain | `ALITA_TEST_MIGRATION_CHAIN=true DATABASE_URL=... go test -tags testtools -run '^TestRepositoryMigrationChain$' ./alita/db/migrations` | ✅ | ⬜ pending |
| TBD | TBD | TBD | PLAT-03 | TBD | New keys in all 7 locale files | script | `make check-translations` | ✅ | ⬜ pending |
| TBD | TBD | TBD | docs | TBD | Help text change reflected in generated docs | script | `make check-docs` (fix: `make generate-docs`) | ✅ | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] Add `ChatLockdownAlert`, `ChatLockdownBurst`, `ChatLockdownBanRun` to the three `AutoMigrate` lists named in AGENTS.md: `alita/modules/test_harness_test.go`, `alita/db/testmain_test.go`, `alita/db/lockdown/testmain_test.go`; extend `lockdownCleanup` (`lockdown_fake_test.go`) to delete the new rows (children first).
- [ ] `lockdownFake`: handle `editMessageReplyMarkup`, `revokeChatInviteLink` (scriptable via `script(...)`, records `invite_link`), per-chat message IDs for a channel, a `banChatMember` hook (`onBan`) to interleave the race tests, and `getChatMember` for many burst users (`setMember`). The base `staffActionFake` already answers `sendMessage` (returns a message ID), `editMessageText`, `banChatMember`, `unbanChatMember`, `getChat` `[VERIFIED: staff_action_fake_test.go:296-418]`.
- [ ] `lockdownEnv` helpers: link the env chat to a Staff Group and set a log channel (`logchannels.Set`), `press(from, chat, msgID, data)` modelled on `pressJoinRequest` (`lockdown_fake_test.go:462`), `slots()` / `alertText()` readers, and an `ageAlert(...)` helper like `ageJoiner` (`lockdown_fake_test.go:496`) to make reminders due without sleeping (or a shortened `lockdownReminderEvery`).
- [ ] Test seams as package variables (Phase 4 style): `lockdownReminderEvery`, `lockdownRecentLookup`, `lockdownAlertDestinations`.

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
