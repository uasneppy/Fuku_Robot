---
phase: "4"
slug: "manual-lockdown"
# status lifecycle: draft (seeded by plan-phase) → validated (set by validate-phase §6)
# audit-milestone §5.5 distinguishes NOT-VALIDATED (draft) from PARTIAL (validated + nyquist_compliant: false) (#2117)
status: draft
nyquist_compliant: false
wave_0_complete: false
created: "2026-10-06"
---

# Phase 4 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test + testify assertions; real fixtures (SQLite via `internal/testdb.Run`, miniredis, hand-written `gotgbot.BotClient` fakes extending `staffActionFake`) |
| **Config file** | none; every test file starts with `//go:build testtools`; `CGO_ENABLED=1` |
| **Quick run command** | `go test -tags testtools -race -count=1 -run 'Lockdown' ./alita/db/lockdown ./alita/modules` |
| **Full suite command** | `make test` (sandbox: `go test -tags testtools -race -count=1 ./alita/db/... ./alita/modules/...`) |
| **Estimated runtime** | `make test` ~180 s; first cold `-race` build ~150 s; each narrow `-run` ~1-2 s once compiled |

---

## Sampling Rate

- **After every task commit:** Run the narrow `-run` for the touched tests (see the map below)
- **After every plan wave:** Run `go test -tags testtools -race -count=1 ./alita/db/... ./alita/modules/...` and `make check-translations`
- **Before `/gsd-verify-work`:** `make test`, `make lint`, `make check-translations`, `make generate-docs && make check-docs` must be green, plus the PostgreSQL integrity run locally
- **Max feedback latency:** 60 seconds

---

## Per-Task Verification Map

Seeded from 04-RESEARCH.md "Validation Architecture". Task IDs are bound to plans once the plans exist.

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| TBD | TBD | TBD | LOCK-01 | TBD | A `chat_member` joiner is recorded, then banned; EndGroups stops welcome and captcha | integration | `go test -tags testtools -race -count=1 -run '^TestLockdownGuardBansChatMemberJoin$' ./alita/modules` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | LOCK-01 | TBD | A join delivered on two paths is acted on once | integration | `go test -tags testtools -race -count=1 -run '^TestLockdownGuardDedupesTwoPaths$' ./alita/modules` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | LOCK-01 | TBD | A join request is declined, never auto-approved; the Accept button refuses (D-25) | integration | `go test -tags testtools -race -count=1 -run '^TestLockdownGuardDeclinesJoinRequest$' ./alita/modules` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | LOCK-01 | TBD | Only a human added by a live admin is exempt; bots and self-joins are banned (D-07, D-08, D-24) | unit | `go test -tags testtools -race -count=1 -run '^TestDecideLockdownJoin$' ./alita/modules` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | LOCK-01 | TBD | No goodbye for a lockdown-banned joiner | integration | `go test -tags testtools -race -count=1 -run '^TestLockdownSuppressesGoodbye$' ./alita/modules` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | LOCK-01 | TBD | Joiners are unbanned at the lift and can rejoin | integration | `go test -tags testtools -race -count=1 -run '^TestLockdownLiftUnbansJoiners$' ./alita/modules` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | LOCK-02 | TBD | Lock sends the all-false set with independent permissions; no per-user restrict | integration | `go test -tags testtools -race -count=1 -run '^TestLockdownLockSendsLockedSet$' ./alita/modules` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | LOCK-03 | TBD | Notice says approved users are muted too; no approved-user calls (D-01, D-02) | integration | `go test -tags testtools -race -count=1 -run '^TestLockdownNoticeAndNoPerUserCalls$' ./alita/modules` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | LOCK-04 | TBD | Another chat is unaffected (guard and permissions) | integration | `go test -tags testtools -race -count=1 -run '^TestLockdownAffectsOnlyItsChat$' ./alita/modules` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | LOCK-05 | TBD | Enforcement holds with Redis flushed or absent; state lives in PostgreSQL | integration | `go test -tags testtools -race -count=1 -run '^TestLockdownSurvivesRedisFlush$' ./alita/modules` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | LOCK-05 | TBD | A restart mid-lift resumes the remaining unbans | integration | `go test -tags testtools -race -count=1 -run '^TestLockdownLiftResumesAfterRestart$' ./alita/modules` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | LOCK-05 | TBD | One active lockdown per chat (SQLite) | repository | `go test -tags testtools -race -count=1 -run '^TestStartLockdownOneActivePerChat$' ./alita/db/lockdown` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | LOCK-05 | TBD | One active lockdown per chat (PostgreSQL partial unique index) | repository (PG) | `make test-postgres-integrity` (test added to its `-run` list) | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | LOCK-06 | TBD | Non-admins and admins without restrict are refused; the admin cache is never used | integration | `go test -tags testtools -race -count=1 -run '^TestLockdownCommandsRefuseNonAuthority$' ./alita/modules` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | LOCK-06 | TBD | Anonymous admins always get the proof button, even in AnonAdmin mode; the tapper is checked live and named (D-12) | integration | `go test -tags testtools -race -count=1 -run '^TestLockdownAnonymousAdminProof$' ./alita/modules` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | LOCK-06 | TBD | A double `/unlockdown` lifts once | integration (concurrent) | `go test -tags testtools -race -count=1 -run '^TestUnlockdownLiftsOnce$' ./alita/modules` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | LOCK-07 | TBD | The lift restores the exact snapshot, pointer fields included | integration | `go test -tags testtools -race -count=1 -run '^TestLockdownLiftRestoresExactSnapshot$' ./alita/modules` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | LOCK-07 | TBD | A hand edit is replaced and reported; a failed restore leaves the group locked and unbans nobody (D-18, D-21) | integration | `go test -tags testtools -race -count=1 -run '^TestLockdownLift(ManualChange\|RestoreFailure)$' ./alita/modules` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | LOCK-07 | TBD | A deliberate ban survives the lift (D-05) | unit + integration | `go test -tags testtools -race -count=1 -run '^Test(IsLockdownBan\|LockdownLiftKeepsDeliberateBan)$' ./alita/modules` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | LOCK-08 | TBD | Unmute during a lockdown uses the snapshot for all four callers (D-23) | integration | `go test -tags testtools -race -count=1 -run '^TestUnmuteDuringLockdownUsesSnapshot$' ./alita/modules` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | LOCK-09 | TBD | `/lockdown` starts one; a second reports the existing one; `/lockdownstatus` content | integration | `go test -tags testtools -race -count=1 -run '^TestLockdownCommand(Starts\|ReportsExisting)$\|^TestLockdownStatusCommand$' ./alita/modules` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | LOCK-09 | TBD | Refusals (bot lacks rights, permissions unreadable, basic group) record nothing (D-20) | integration | `go test -tags testtools -race -count=1 -run '^TestLockdownRefusals$' ./alita/modules` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | SETUP-08 | TBD | `/staff` row shows the lockdown marker, also when the bot is missing (D-17, D-22) | unit | `go test -tags testtools -race -count=1 -run '^TestRenderStaffRowLockdownMarker$' ./alita/modules` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | LOCK-07 (D-05) | TBD | A staff timed `/ban` on a lockdown joiner re-issues the ban, so the lift keeps it | unit | `go test -tags testtools -race -count=1 -run '^TestDecideStaffBanLockdownJoiner$' ./alita/modules` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | platform | — | Migration chain and checksums | existing | `go test -tags testtools -race -count=1 -run '^TestRepositoryMigrationChain$' ./alita/db/migrations` | ✅ | ⬜ pending |
| TBD | TBD | TBD | platform | — | All 7 locales carry the new keys | script | `make check-translations` | ✅ | ⬜ pending |
| TBD | TBD | TBD | platform | — | Generated docs match | script | `make generate-docs && make check-docs` | ✅ | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `alita/db/lockdown/testmain_test.go` and repository tests (SQLite via `testdb.Run`, the two new lockdown models)
- [ ] Both new models in the `AutoMigrate` lists of `alita/modules/test_harness_test.go` and `alita/db/testmain_test.go`
- [ ] Extend `staffActionFake` (or a sibling `lockdownFake` embedding it): `setChatPermissions` storing raw permissions, `getChat` returning the stored raw permissions JSON, `declineChatJoinRequest`, `deleteMessage`, `getChatMember` for a kicked member echoing `until_date`, scripted errors per method
- [ ] Test helper `lockdownEnv`: dispatcher with Lockdown, Greetings, Captcha, AntiRaid and Staff loaded, miniredis, a fast lockdown pacer, deterministic worker ticks
- [ ] Lockdown cases in `chat_permissions_test.go` (the existing parallel pure cases stay on `Id == 0`)
- [ ] PostgreSQL index test added to the Makefile `test-postgres-integrity` `-run` list
- [ ] New keys in all 7 locale files, plus `config.yml` `alt_names`

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Joins arrive and are handled once on a real supergroup (`chat_member`, service message, join request) | LOCK-01 | Telegram's live delivery order and duplicate delivery can't be reproduced by the fake (D-03) | In a test supergroup, `/lockdown`, then join through an invite link, a join-request link, and by an admin adding a user. Check one ban or decline each, no welcome or captcha, and the joiner unbanned at `/unlockdown` |
| An admin approving a pre-lockdown request in Telegram's own list lets the person in | LOCK-01 (D-24) | Depends on Telegram's update order; the residual race is accepted | Leave a join request pending, `/lockdown`, approve it from Telegram's request list. The person should get in; if the race bans them, `/unlockdown` must unban them |
| Real `getChat` permissions round-trip exactly | LOCK-07 | The raw JSON shape is assumed (04-RESEARCH A1) | Set unusual default permissions, `/lockdown`, `/unlockdown`; compare the group's Permissions screen before and after. Save the raw `getChat` permissions JSON as a test fixture |
| A banned member's `until_date` matches what the bot sent | LOCK-07 (D-05) | Telegram's rounding of `until_date` is assumed (04-RESEARCH A2) | During a lockdown, let a joiner be banned, read `getChatMember` for them and compare `until_date` to the stored value. Save it as a test fixture |
| `/staff` shows "🔒 in lockdown" for a locked linked group | SETUP-08 | Live panel rendering | Lock a linked group, open `/staff` in the Staff Group, check the row marker; lift and refresh |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 60s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
