---
phase: "6"
slug: "automatic-raid-detection"
# status lifecycle: draft (seeded by plan-phase) → validated (set by validate-phase §6)
# audit-milestone §5.5 distinguishes NOT-VALIDATED (draft) from PARTIAL (validated + nyquist_compliant: false) (#2117)
status: validated
nyquist_compliant: true
wave_0_complete: true
created: "2026-10-18"
---

# Phase 6 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go `testing` with build tag `testtools`, SQLite via `internal/testdb`, miniredis, hand-written `gotgbot.BotClient` fakes (`lockdownFake`) |
| **Config file** | none; `-tags testtools` is mandatory (AGENTS.md) |
| **Quick run command** | `CGO_ENABLED=1 go test -tags testtools -race -count=1 -run '^TestRaid|^TestAutoLockdown|^TestAntiRaidRetired|^TestLockdown' ./alita/modules ./alita/db/raiddetection ./alita/db/lockdown` |
| **Full suite command** | `make test` |
| **Estimated runtime** | ~60 seconds quick, ~6 minutes full |

---

## Sampling Rate

- **After every task commit:** Run the quick run command (the task's `-run` subset)
- **After every plan wave:** Run `make test`
- **Before `/gsd-verify-work`:** Full suite must be green, plus `make check-translations` and `make check-docs`
- **Max feedback latency:** 60 seconds

---

## Per-Task Verification Map

Bound to plans 06-01 to 06-13. Test files are created by the task that lists them (no separate Wave 0 plan): each task writes its failing tests first. Every module command below is `CGO_ENABLED=1 go test -tags testtools -race -count=1 -run '<pattern>' ./alita/modules` unless another package is named.

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 06-01-T1 | 06-01 | 1 | all | T-06-01 | /lockdown and every Phase 4/5 lockdown test unchanged after the start-path extraction | integration | `-run 'Lockdown\|Unlockdown' ./alita/modules ./alita/db/lockdown` | ✅ existing | ✅ green |
| 06-01-T1 | 06-01 | 1 | LOCK-10 (D-14) | T-06-01, T-06-02 | Every start outcome: refusals record nothing and make no lock call, refused lock leaves no row, unknown lock keeps its row, already active returns the row; two racing starts give one Started and one AlreadyActive | integration | `-run '^TestLockdownStartRow$'` | ✅ exists | ✅ green |
| 06-01-T2 | 06-01 | 1 | RAID-01 (D-13, D-21) | T-06-03 | The alert of a join_surge row names count and window; a flood row's names count, window and percent; manual alerts unchanged; locale parity | integration + i18n | `-run '^TestLockdownAlertTriggerLine$'`; `go test -tags testtools -count=1 -run '^TestLockdownLocaleKeys$' ./alita/i18n` | ✅ exists | ✅ green |
| 06-02-T1 | 06-02 | 2 | RAID-01, RAID-06 | T-06-04, T-06-05, T-06-07 | 15 distinct self-joins within 60 s lock the group through startLockdownRow; row has `trigger_kind=join_surge`, count 15, window 60, the bot as starter; the tipping joiner is handled by the new lockdown; the Staff Group alert names the surge and its count | integration (dispatcher + fake bot + miniredis) | `-run '^TestRaidJoinSurge$'` | ✅ exists | ✅ green |
| 06-02-T2 | 06-02 | 2 | RAID-06 (D-11) | T-06-04 | No row means the defaults (15 joins); a stored threshold and an off switch are followed; an unreadable setting counts nothing | integration | `-run '^TestRaidJoinSurgeSettings$'` | ✅ exists | ✅ green |
| 06-03-T1 | 06-03 | 3 | RAID-01 | T-06-09, T-06-10 | A join delivered as chat_member and service message counts once; a 15-user service message locks; one performer lookup per join shared with the guard | integration | `-run '^TestRaidJoinDoubleDelivery$\|^TestRaidJoinServiceMessage$'` | ✅ exists | ✅ green |
| 06-03-T2 | 06-03 | 3 | RAID-07 | T-06-08 | Admin-added (bots included), anonymous-admin-added, approved, the bot itself, join requests and admin-approved requests never count; member-added bots, bot-approved requests and failed lookups count | integration | `-run '^TestRaidJoinExemptions$'` | ✅ exists | ✅ green |
| 06-03-T2 | 06-03 | 3 | RAID-01 | T-06-06 | Below threshold and outside the 60 s window do not lock | integration | `-run '^TestRaidJoinBelowThreshold$'` | ✅ exists | ✅ green |
| 06-03-T2 | 06-03 | 3 | RAID-01 | T-06-06 | Lua join script: dedupe, window, 24 h trim, cap, TTL (miniredis) | unit | `-run '^TestRaidJoinScript$'` | ✅ exists | ✅ green |
| 06-03-T2 | 06-03 | 3 | RAID-01 (D-04) | — | Without Redis: counters inert, one log line, `/lockdown` still works | integration | `-run '^TestRaidInertWithoutRedis$'` | ✅ exists | ✅ green |
| 06-04-T1 | 06-04 | 3 | RAID-02 | T-06-12 | 30 items from 4 recent joiners within 30 s lock the group (`flood`, percent 100); alert names the flood; established members and joiners older than 24 h do not lock; nothing deleted | integration | `-run '^TestRaidFlood$'` | ✅ exists | ✅ green |
| 06-04-T2 | 06-04 | 3 | RAID-02 | — | Item classification: text, photo, GIF, sticker and other content count; commands, service messages and empty messages do not; album parts share one key | unit | `-run '^TestRaidFloodItemKey$'` | ✅ exists | ✅ green |
| 06-04-T2 | 06-04 | 3 | RAID-07 | T-06-11 | Admin, approved user, service IDs (777000, 1087968824, 136817688), anonymous-admin post, linked-channel auto-forward, commands, edited messages, bot senders and basic groups never count | integration | `-run '^TestRaidFloodExemptions$'` | ✅ exists | ✅ green |
| 06-04-T2 | 06-04 | 3 | RAID-02 | T-06-14 | 56% does not lock, 60% does; 2 accounts do not; an album is one item | integration | `-run '^TestRaidFloodRule$'` | ✅ exists | ✅ green |
| 06-04-T2 | 06-04 | 3 | RAID-02 | T-06-12 | Lua flood script: counts, new-member subset, accounts, window drain, cap, TTL | unit | `-run '^TestRaidFloodScript$'` | ✅ exists | ✅ green |
| 06-05-T1 | 06-05 | 4 | RAID-06 | T-06-17 | Defaults shown for a chat with no row; `joins 20`, `joins off`/`on`, `flood off` change it; the next burst follows the new setting after the local layer is cleared | integration | `-run '^TestAutoLockdown$'` | ✅ exists | ✅ green |
| 06-05-T1 | 06-05 | 4 | RAID-06 | T-06-16, T-06-18 | Setting `false` persists through a cold read; one trigger's write never changes the other's columns; out-of-range writes nothing; writes invalidate the cache | repository | `go test -tags testtools -race -count=1 ./alita/db/raiddetection` | ✅ exists | ✅ green |
| 06-05-T1 | 06-05 | 4 | RAID-06 (D-12) | T-06-17 | `skipLocal` covers the raid_detection key and not `raid_detectionx` | unit | `go test -tags testtools -race -count=1 -run '^TestSkipLocalMatchesFreshnessCriticalKeys$' ./alita/db/cache` | extend | ✅ green |
| 06-05-T2 | 06-05 | 4 | RAID-06 (D-10) | T-06-15 | View needs any live admin; change needs the creator or `can_restrict_members`; anonymous admin proves first; member refused; failed lookup refuses; private chat refused; save failure changes nothing | integration | `-run '^TestAutoLockdownAuthority$\|^TestAutoLockdownBasicGroup$'` | ✅ exists | ✅ green |
| 06-05-T2 | 06-05 | 4 | RAID-06 (D-18, D-21) | T-06-16 | Grammar: joins/flood, on/off/N, range 3..500, usage on anything else | unit | `-run '^TestAutoLockdownArgs$'` | ✅ exists | ✅ green |
| 06-06-T1 | 06-06 | 4 | LOCK-10 | T-06-19 | Join surge and flood fire together: one `chat_lockdowns` row, one `setChatPermissions`, one alert copy, either order | integration | `-run '^TestRaidTriggersSingleLockdown$'` | ✅ exists | ✅ green |
| 06-06-T1 | 06-06 | 4 | LOCK-10 | T-06-19 | Two dispatchers ("replicas") feeding one burst at once, with and without the fire latch: one lockdown | integration (`-race`) | `-run '^TestRaidTriggersConcurrent$'` | ✅ exists | ✅ green |
| 06-06-T1 | 06-06 | 4 | LOCK-10 | T-06-19 | Existing guarantee `uk_chat_lockdowns_active` + `Start` + `BeginAlert` on PostgreSQL | PG integrity | already in `make test-postgres-integrity`: `TestStartLockdownOneActivePerChat`, `TestBeginAlert` (CI) | ✅ existing | ✅ green |
| 06-06-T2 | 06-06 | 4 | LOCK-10 (D-15) | T-06-21 | After `/unlockdown`, a burst within 10 minutes starts nothing and calls Telegram for nothing; after aging `lift_started_at` by 11 minutes it locks | integration | `-run '^TestRaidCooldownAfterLift$'` | ✅ exists | ✅ green |
| 06-06-T2 | 06-06 | 4 | LOCK-10 | T-06-20 | Latch: 60 s after a start, 10 s after a transient failure, 10 min when the bot cannot lock; a latched trigger calls nothing | integration | `-run '^TestRaidLatch$'` | ✅ exists | ✅ green |
| 06-06-T2 | 06-06 | 4 | LOCK-10 (D-15) | T-06-21 | RecentlyLiftedFresh per chat and cut-off | repository | `go test -tags testtools -race -count=1 -run '^TestRecentlyLiftedFresh$' ./alita/db/lockdown` | ✅ exists | ✅ green |
| 06-07-T1 | 06-07 | 5 | RAID-01, RAID-02 (D-13) | T-06-22 | Public notice posted once by the winner, per kind, with the approved-muted and missing-right notes; a failed notice changes nothing | integration | `-run '^TestRaidAutoNotice$'` | ✅ exists | ✅ green |
| 06-07-T2 | 06-07 | 5 | D-14, D-19 | T-06-22, T-06-23, T-06-24 | Bot cannot lock: no lockdown, one warning to the Staff Group and one to the log channel; nothing more the same day; again after miniredis `FastForward` 24 h; refused lock and basic group warn; transient failures and no destinations post nothing | integration | `-run '^TestRaidCannotLockWarnsOncePerDay$'` | ✅ exists | ✅ green |
| 06-07-T2 | 06-07 | 5 | PLAT-03 | — | The three warning texts start with the warning marker in all 7 locales | i18n | `go test -tags testtools -count=1 -run '^TestLockdownWarningLines$' ./alita/i18n` | ✅ existing (extended) | ✅ green |
| 06-08-T1 | 06-08 | 6 | RAID-01, RAID-02 (D-13, D-21) | T-06-25, T-06-26 | /lockdownstatus and /lockdown of an automatic lockdown say "locked automatically" and name the trigger with its numbers; manual unchanged | integration | `-run '^TestRaidLockdownStatus$'` | ✅ exists | ✅ green |
| 06-08-T2 | 06-08 | 6 | RAID-01, RAID-02 (D-13) | T-06-25 | Reminders of automatic lockdowns name the trigger; manual reminders keep the reason | integration | `-run '^TestRaidLockdownReminder$'` | ✅ exists | ✅ green |
| 06-09-T1 | 06-09 | 7 | RAID-02 (D-17) | T-06-27, T-06-29 | A flood alert lists the flooding accounts (joined hours earlier) with escaped names and the flood counts line; burst rows saved; established members not listed | integration | `-run '^TestRaidFloodBurst$'` | ✅ exists | ✅ green |
| 06-09-T1 | 06-09 | 7 | RAID-02 (D-17) | — | FindUsersByIDs: one query, unknown IDs ignored, empty input | repository | `go test -tags testtools -race -count=1 -run '^TestFindUsersByIDs$' ./alita/db/user` | ✅ exists | ✅ green |
| 06-09-T2 | 06-09 | 7 | RAID-02 (D-17) | T-06-28 | Ban recent joiners on a flood alert bans the flooders for good and leaves others; surge alerts unchanged; lost snapshot and unknown names still post; the bot skipped | integration | `-run '^TestRaidFloodBurst$\|^TestLockdownBanRecentJoiners$'` | ✅ exists | ✅ green |
| 06-10-T1 | 06-10 | 8 | LOCK-15 (D-16) | T-06-30, T-06-32 | `/antiraid`, `/raidtime`, `/raidactiontime` reply with the pointer and change nothing; `/autoantiraid 20` sets the join surge, `off` disables, range and authority as /autolockdown; old buttons answered | integration | `-run '^TestAntiRaidRetired$'` | ✅ exists | ✅ green |
| 06-10-T1 | 06-10 | 8 | LOCK-15 (D-16) | T-06-31 | A join while the old raid-mode keys exist bans and restricts nobody | integration | `-run '^TestAntiRaidRetiredNoBan$'` | ✅ exists | ✅ green |
| 06-10-T2 | 06-10 | 8 | LOCK-15 (D-16) | T-06-31, T-06-33 | The startup sweep deletes exactly the `alita:antiraid:*` keys; idempotent; inert without Redis | integration | `-run '^TestAntiRaidKeySweep$'` | ✅ exists | ✅ green |
| 06-11-T1 | 06-11 | 8 | LOCK-15 (D-20) | T-06-35 | Dev stats count chat_raid_detection rows, join-off and flood-off chats | repository | `go test -tags testtools -race -count=1 ./alita/db/raiddetection ./alita/db/devs` | ✅ existing (updated) | ✅ green |
| 06-11-T2 | 06-11 | 8 | LOCK-15 (D-03, D-18, D-22) | T-06-34 | Migration copies non-zero thresholds clamped into 3..500 (D-22 caps above 500), zero gets no row, existing rows kept, idempotent; rolled back | PG only (CI) | `go test -tags testtools -count=1 -run '^TestRaidDetectionCarryOver$' ./alita/db/migrations` (skips here; runs in `make test-postgres-integrity` after the migration chain) | ✅ exists | ⏭ CI-only (skips here, no PostgreSQL) |
| 06-12-T1 | 06-12 | 8 | LOCK-15, RAID-06 (D-16, D-20) | T-06-36 | Frozen reference pages describe the retirement, the triggers, the table and the backup gap; markers kept | gate | `make check-docs` | n/a | ✅ green |
| 06-12-T2 | 06-12 | 8 | LOCK-15, RAID-06 | T-06-36 | Architecture pages show handler groups -7 and -4, the alita:raid:* keys and the new files | gate | `make check-docs` | n/a | ✅ green |
| 06-13-T1 | 06-13 | 9 | all | T-06-37 | Help text in 7 languages with /autolockdown as the fourth bullet; generated docs in sync; no missing literal keys | gate + i18n | `make check-docs`; `go test -tags testtools -count=1 -run '^TestLockdownStringsHaveBalancedTags$\|^TestLockdownLocaleKeys$' ./alita/i18n`; `make check-translations` | ✅ existing (updated) | ✅ green |
| 06-13-T2 | 06-13 | 9 | all | T-06-38 | Full suite, build, vet, gofmt; Makefile PostgreSQL list holds `TestRaidDetectionSettings` and `TestRaidDetectionCarryOver` | gate | `make test`; `make check-translations && make check-docs && CGO_ENABLED=0 go build ./... && go vet -tags testtools ./...` | n/a | ✅ green (focused runs, check-docs, check-translations; full `make test` left to CI) |

*Status: ✅ green · ⏭ CI-only · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

No separate Wave 0 plan: each task's RED step creates the test files it lists. Where each scaffold lands:

- [x] `alita/modules/testredis_test.go` - receives `withMiniredis` and gains `withMiniredisServer` (06-07-T2), before plan 06-10 deletes the antiraid tests
- [x] `alita/modules/lockdown_start_test.go` (06-01), `raid_join_surge_test.go` (06-02), `raid_join_test.go` and `raid_counters_test.go` (06-03), `raid_flood_test.go` with `seedRecentJoin` and `sendItem` helpers (06-04), `autolockdown_test.go` (06-05), `raid_trigger_test.go` (06-06), `raid_notice_test.go` (06-07), `raid_status_test.go` (06-08), `raid_flood_burst_test.go` (06-09), `antiraid_retired_test.go` (06-10)
- [x] `alita/db/raiddetection/{repository_test.go,testmain_test.go}` (06-05), `stats_test.go` (06-11) - `testdb.Run(m, &models.ChatRaidDetection{})`
- [x] `alita/db/lockdown/cooldown_test.go` for `RecentlyLiftedFresh` (06-06); the trigger columns round-trip through `Start` in `TestLockdownStartRow` (06-01) and `TestRaidJoinSurge` (06-02)
- [x] `&models.ChatRaidDetection{}` in the AutoMigrate lists of `alita/modules/test_harness_test.go` (06-02) and `alita/db/devs/testmain_test.go` (06-11); the `alita/db` package's own tests never touch the table, so its list is left alone
- [x] Seams: `raidNow`, `raidJoinedCap`, `raidSettingsLookup` (06-02), `raidFloodCap` (06-04), `autolockdownWrite` (06-05), `raidFloodersLookup` (06-09), `raidNoRedisLogged` (06-02)
- [x] `alita/db/migrations/raid_carry_over_test.go` (06-11), PostgreSQL only, listed in the Makefile by 06-13
- [x] Framework install: none

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| A real join surge (many accounts joining a test supergroup) locks it once and posts one alert naming the surge | RAID-01, LOCK-10 | Needs real Telegram accounts and delivery timing | Join 15+ test accounts within a minute; check one lockdown, one alert, the public notice |
| A real flood from fresh accounts locks; a busy chat among old members does not | RAID-02 | Real message mix and timing | Have 3+ new accounts send 30+ items in 30 s; separately have established members chat busily |
| Migration carries old thresholds on PostgreSQL 16 | LOCK-15 | No PostgreSQL server in the sandbox | Run the migration-chain test and `make test-postgres-integrity` in CI |

---

## Validation Sign-Off

- [x] All tasks have `<automated>` verify or Wave 0 dependencies
- [x] Sampling continuity: no 3 consecutive tasks without automated verify
- [x] Wave 0 covers all MISSING references
- [x] No watch-mode flags
- [x] Feedback latency < 60s
- [x] `nyquist_compliant: true` set in frontmatter

**Approval:** validated 2026-10-29 (quick command green; PostgreSQL-only TestRaidDetectionCarryOver and the PG integrity list are CI-only)
