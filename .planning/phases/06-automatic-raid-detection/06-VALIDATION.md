---
phase: "6"
slug: "automatic-raid-detection"
# status lifecycle: draft (seeded by plan-phase) → validated (set by validate-phase §6)
# audit-milestone §5.5 distinguishes NOT-VALIDATED (draft) from PARTIAL (validated + nyquist_compliant: false) (#2117)
status: draft
nyquist_compliant: false
wave_0_complete: false
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

Seeded from the research Validation Architecture; the planner replaces `(planner)` with the task IDs that own each row.

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| (planner) | — | — | RAID-01 | — | N distinct self-joins within 60 s lock the group; row has `trigger_kind=join_surge`, `trigger_count>=N`, `StartedBy=bot`; alert trigger line names the surge and count | integration (dispatcher + fake bot + miniredis) | `go test -tags testtools -race -count=1 -run '^TestRaidJoinSurge' ./alita/modules` | ❌ W0 | ⬜ pending |
| (planner) | — | — | RAID-01 | — | Same join delivered as chat_member and service message counts once | integration | `-run '^TestRaidJoinDoubleDelivery$'` | ❌ W0 | ⬜ pending |
| (planner) | — | — | RAID-01 | — | Below threshold / outside 60 s window does not lock | integration | `-run '^TestRaidJoinBelowThreshold$'` | ❌ W0 | ⬜ pending |
| (planner) | — | — | RAID-01 | — | Lua join script: dedupe, window, 24 h trim, cap (miniredis) | unit | `-run '^TestRaidJoinScript$'` | ❌ W0 | ⬜ pending |
| (planner) | — | — | RAID-02 | — | 35 items from 4 recent joiners within 30 s lock the group (`flood`, percent shown); 40 items from established members do not; fewer than 3 accounts do not; ratio just under 60 % does not | integration | `-run '^TestRaidFlood'` | ❌ W0 | ⬜ pending |
| (planner) | — | — | RAID-02 | — | Item classification: text/photo/GIF/sticker count; commands, edited, service messages, album parts (once per `media_group_id`) as specified | unit | `-run '^TestRaidFloodItemKey$'` | ❌ W0 | ⬜ pending |
| (planner) | — | — | RAID-02 | — | Lua flood script: counts, new-member subset, accounts, window drain | unit | `-run '^TestRaidFloodScript$'` | ❌ W0 | ⬜ pending |
| (planner) | — | — | RAID-07 | — | Admin-added joiner, approved joiner, the bot itself, a join request never count toward the surge | integration | `-run '^TestRaidJoinExemptions$'` | ❌ W0 | ⬜ pending |
| (planner) | — | — | RAID-07 | — | Admin, approved user, bot, service IDs (777000, 1087968824, 136817688), anonymous-admin post (`sender_chat` = group), linked-channel auto-forward never count toward flood | integration | `-run '^TestRaidFloodExemptions$'` | ❌ W0 | ⬜ pending |
| (planner) | — | — | RAID-06 | — | Both triggers on by default for a chat with no row; `/autolockdown` shows state; `joins 20`, `joins off`, `flood on` change it; the next burst follows the new setting on a second "replica" (clear the local layer) | integration + repo | `-run '^TestAutoLockdown'` ; `go test -tags testtools -race -count=1 ./alita/db/raiddetection` | ❌ W0 | ⬜ pending |
| (planner) | — | — | RAID-06 | — | Setting `false` persists (GORM zero-value trap); joins write does not clobber flood columns; `DeleteCache` invalidates | repo | `-run '^TestRaidDetectionSettings'` ./alita/db/raiddetection | ❌ W0 | ⬜ pending |
| (planner) | — | — | RAID-06 | — | `skipLocal` covers the raid_detection key and not `raid_detectionx` | unit | `go test -tags testtools -race -count=1 -run '^TestSkipLocalMatchesFreshnessCriticalKeys$' ./alita/db/cache` (extend `local_test.go:286`) | extend | ⬜ pending |
| (planner) | — | — | RAID-06 (D-10) | — | View needs any live admin; change needs creator or `can_restrict_members`; anonymous admin gets the proof prompt and re-enters; non-admin refused; failed lookup refuses | integration | `-run '^TestAutoLockdownAuthority$'` | ❌ W0 | ⬜ pending |
| (planner) | — | — | LOCK-10 | — | Join surge and flood fire together: exactly one `chat_lockdowns` active row, one `setChatPermissions` call, one alert slot set after the worker cycle | integration | `-run '^TestRaidTriggersSingleLockdown$'` | ❌ W0 | ⬜ pending |
| (planner) | — | — | LOCK-10 | — | Two goroutines ("replicas") evaluate the same burst concurrently: one lockdown | integration (`-race`) | `-run '^TestRaidTriggersConcurrent$'` | ❌ W0 | ⬜ pending |
| (planner) | — | — | LOCK-10 | — | Existing guarantee `uk_chat_lockdowns_active` + `Start` + `BeginAlert` (no new conditional update added) | PG integrity | already in `make test-postgres-integrity`: `TestStartLockdownOneActivePerChat`, `TestBeginAlert` | ✅ existing | ⬜ pending |
| (planner) | — | — | D-14 | — | Bot not admin / no restrict right: no lockdown, one warning to Staff Group and log channel; second trigger the same day posts nothing (miniredis `FastForward` 24 h then posts again) | integration | `-run '^TestRaidCannotLockWarnsOncePerDay$'` | ❌ W0 | ⬜ pending |
| (planner) | — | — | D-15 | — | After `/unlockdown`, a burst within 10 minutes starts nothing; after aging `lift_started_at` by 11 minutes it does | integration | `-run '^TestRaidCooldownAfterLift$'` | ❌ W0 | ⬜ pending |
| (planner) | — | — | D-13 | — | Public notice posted once in the locked group by the winner only; alert/status trigger lines per kind in all locales | integration + i18n | `-run '^TestRaidAutoNotice$'` ; `go test -tags testtools -race -count=1 -run '^TestLockdownLocaleKeys$' ./alita/i18n` | ❌ W0 | ⬜ pending |
| (planner) | — | — | D-04 | — | Without Redis: counters inert, one log line, `/lockdown` still works | integration | `-run '^TestRaidInertWithoutRedis$'` | ❌ W0 | ⬜ pending |
| (planner) | — | — | LOCK-15 | — | `/antiraid`, `/raidtime`, `/raidactiontime` reply with the pointer text; `/autoantiraid 20` sets joins=20 and enabled, `/autoantiraid off` disables | integration | `-run '^TestAntiRaidRetired'` | ❌ W0 | ⬜ pending |
| (planner) | — | — | LOCK-15 | — | A join while the old `alita:antiraid:state:<chat>` key exists bans nobody; the startup sweep deletes `alita:antiraid:*` keys | integration | `-run '^TestAntiRaidRetiredNoBan$'`, `-run '^TestAntiRaidKeySweep$'` | ❌ W0 | ⬜ pending |
| (planner) | — | — | LOCK-15 (D-03) | — | Migration copies non-zero thresholds only, leaves zero/missing rows to defaults, is idempotent | PG chain / PG integrity | `ALITA_TEST_MIGRATION_CHAIN=true` migration chain + a `TestRaidDetectionCarryOver` added to the `make test-postgres-integrity` `-run` list; CI only (no PostgreSQL here) | ❌ W0 | ⬜ pending |
| (planner) | — | — | Docs / i18n | — | Generated docs in sync; no missing literal keys | gate | `make generate-docs && make check-docs`; `make check-translations` | n/a | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `alita/modules/testredis_test.go` - receives `withMiniredis` before `antiraid_miniredis_test.go` is deleted
- [ ] `alita/modules/raid_*_test.go`, `autolockdown_test.go`, `antiraid_retired_test.go` - cover the rows above; add helpers on `lockdownEnv` to send photo/sticker/GIF/album messages with a given `From` and to seed the 24 h joined set at a chosen age
- [ ] `alita/db/raiddetection/{repository_test.go,testmain_test.go}` - `testdb.Run(m, &models.ChatRaidDetection{})`
- [ ] `alita/db/lockdown` test for `RecentlyLiftedFresh` and for the new trigger columns round-tripping through `Start`
- [ ] Add `&models.ChatRaidDetection{}` to the AutoMigrate lists of `alita/modules/test_harness_test.go` and `alita/db/testmain_test.go`
- [ ] Clock seam `raidNow` and a settings lookup seam (like `lockdownActiveLookup`) so tests control time and failures
- [ ] Framework install: none

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| A real join surge (many accounts joining a test supergroup) locks it once and posts one alert naming the surge | RAID-01, LOCK-10 | Needs real Telegram accounts and delivery timing | Join 15+ test accounts within a minute; check one lockdown, one alert, the public notice |
| A real flood from fresh accounts locks; a busy chat among old members does not | RAID-02 | Real message mix and timing | Have 3+ new accounts send 30+ items in 30 s; separately have established members chat busily |
| Migration carries old thresholds on PostgreSQL 16 | LOCK-15 | No PostgreSQL server in the sandbox | Run the migration-chain test and `make test-postgres-integrity` in CI |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 60s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
