---
last_mapped_commit: 4d3518f822a0014f925b59b6748acaee47241152
last_mapped_at: 2026-10-04
---
# Codebase Concerns

**Analysis Date:** 2026-10-04

## Tech Debt

**Goroutine Panic Recovery Gaps:**
- Issue: Several fire-and-forget goroutines lack `defer error_handling.RecoverFromPanic()`, violating AGENTS.md requirement
- Files: `alita/modules/aispam.go:453` (drain WaitGroup observer), `alita/modules/captcha.go:195` (lifecycle task runner), `alita/modules/purges.go:129,202,353,458` (worker pool), `alita/modules/users.go:27` (async update spawner)
- Impact: Unhandled panics in these goroutines will crash the bot without recovery. Other modules/subsystems lose signal that initialization succeeded.
- Fix approach: Add `defer error_handling.RecoverFromPanic("functionName", "moduleName")` to all goroutine bodies. Test by injecting panics in unit tests.

**Module Complexity & Maintainability:**
- Issue: Five modules exceed 1000 lines, making changes risky
- Files: `alita/modules/captcha.go` (2095 lines), `alita/modules/federations.go` (1283 lines), `alita/modules/greetings.go` (1159 lines), `alita/modules/bans.go` (1139 lines), `alita/modules/notes.go` (1099 lines)
- Impact: Difficult to understand control flow, test isolated features, or refactor without side effects. High cognitive load for reviewers.
- Fix approach: Break into smaller packages by responsibility (e.g., `captcha/{math,image,settings}`). Move shared logic to `captcha/shared.go`. Establish max 800-line target per file.

**Test Coverage Gaps in Critical Modules:**
- Issue: 15+ high-impact modules have no test files despite having complex business logic
- Files: `alita/modules/antiflood.go`, `alita/modules/bans.go`, `alita/modules/captcha.go`, `alita/modules/connections.go`, `alita/modules/federations.go`, `alita/modules/greetings.go`, `alita/modules/help.go`, `alita/modules/logchannels.go`, `alita/modules/mute.go`, `alita/modules/pins.go`, `alita/modules/reports.go`, `alita/modules/rules.go`, `alita/modules/warns.go`, `alita/modules/locks.go`
- Impact: Silent regressions in command handlers, permission checks, and message processing. Deployment risk is high for these paths.
- Fix approach: Prioritize tests for permission-guarded commands and handler logic. Start with `TestAntifoodCommandRequiresAdmin`, `TestBanCommandValidation`.

**Pending Backup State Unbounded Map Growth:**
- Issue: `pendingImports` and `pendingResets` maps in `alita/modules/backup.go:50-51` have no hard size limit, only TTL cleanup
- Files: `alita/modules/backup.go:50-51,160-170`
- Impact: If cleanup ticker (`startPendingCleanupTicker` at line 172) fails or is blocked by slow code, stale entries accumulate indefinitely, consuming memory. 10-minute TTL means up to 10M per chat with pending backups.
- Fix approach: Add a max-size check before storing: `if len(pendingImports) >= maxPendingImports { reject }`. Set limit to ~1000 per chat. Log evictions as warnings.

**Goroutine Lifecycle Fragility:**
- Issue: Goroutines started at module load time (via `go` statements in `init()` and `RegisterLegacyModule` callbacks) lack coordination. Context cancellation only available for some.
- Files: `alita/modules/antiflood.go:66` (cleanup loop without context), `alita/modules/aispam.go:393,891` (workers spawned twice — once init, once on demand), `alita/modules/captcha.go:195` (task scheduling without shutdown signal)
- Impact: Goroutines may attempt work after shutdown begins. Module reload during tests may spawn duplicates. Shutdown hangs if a goroutine waits on stale Redis connection.
- Fix approach: Centralize goroutine lifecycle: add `ShutdownFn` callback to `RegisterLegacyModule` signature. Pass context to all worker loops. Test by calling `StopXxxModule()` in reverse load order.

## Known Bugs

**Antiflood In-Process State Loss on Replica Failover:**
- Symptoms: Antiflood counter resets when a replica restarts; a user can abuse the bot by sending N messages per second for M seconds, then waiting 60s, then repeating without penalty.
- Files: `alita/modules/antiflood.go:40-44` (sync.Map is in-process only), `alita/modules/antiflood.go:72-84` (cleanup only on stale activity)
- Trigger: Pod restart, process crash, network partition
- Workaround: Increase antiflood thresholds to tolerate one burst per replica lifetime; accept short windows of abuse after failover.

**Captcha Replay Vulnerability (Low Risk):**
- Symptoms: A user can copy the captcha image/question ID from one attempt and reuse it in a second session, bypassing freshness check.
- Files: `alita/modules/captcha.go:1815+` (pending captcha storage), `alita/db/captcha/repository.go:176` (advisory lock on per-chat pending flag)
- Trigger: User saves captcha ID, leaves, re-joins same chat within grace period (configurable, default unclear)
- Workaround: None needed if grace period is < 30 seconds. Check `alita/config/config.go` for any captcha TTL config.

**Handler Precedence Brittleness:**
- Symptoms: Adding a handler at priority -1 (users tracker) after a group -10 captcha handler silently changes which handler runs first, silently skipping captcha for that update.
- Files: `alita/modules/core.go:*` (handler registration), gotgbot dispatcher documentation recommends sequential priority but does not enforce it.
- Trigger: Refactoring module load order or registering duplicate priorities.
- Workaround: Document priorities in a const; enforce in lint/CI rule. Test loads modules in intended order.

## Security Considerations

**Backup Import DoS:**
- Risk: A malicious user uploads a 10MB backup file repeatedly, exhausting bot memory and disk I/O.
- Files: `alita/modules/backup.go:319` (maxBackupFileSize = 10MB), `alita/modules/backup.go:200+` (file download/parse with no rate limiting)
- Current mitigation: Hard size cap (10MB), imports stored in-memory map with 10-minute TTL.
- Recommendations: Add per-user rate limit (e.g., 1 import per 5 minutes). Add per-chat quota (e.g., max 3 concurrent restores). Monitor disk usage of Telegram file cache.

**Callback Data Overflow Silent Failure:**
- Risk: Callback payloads exceeding 64 bytes silently return empty string, shipping a dead button with no error indication.
- Files: `alita/utils/callbackcodec/codec.go:encodeCallbackData` (returns empty on overflow)
- Current mitigation: Large user text moved to Redis behind short token (AGENTS.md section 3.1).
- Recommendations: Log a warning every time overflow occurs; consider increasing buffer or implementing chunked encoding for very large payloads.

**Federation Ban Storms:**
- Risk: A malicious admin bans a large user ID (e.g., -1001234567890) in a federation, triggering goroutines to ban across hundreds of chats concurrently, overwhelming Telegram API and database.
- Files: `alita/modules/federations.go:735` (spawns goroutine per federation member), `alita/modules/federations.go:756+` (applyActiveFban)
- Current mitigation: Goroutines have panic recovery; no explicit rate limiting.
- Recommendations: Add concurrent goroutine cap (e.g., max 10 parallel bans per federation). Implement exponential backoff on Telegram 429 Too Many Requests.

**Redis Key Namespace Collision:**
- Risk: Operational Redis keys (`alita:antiraid:*`, `alita:anonAdmin:*`) are outside `alita:cache:` prefix, but no formal registry or enforcement prevents two subsystems from using the same key.
- Files: `alita/modules/antiraid.go:36-37` (alita:antiraid:*), `alita/db/cache/loader.go:*` (alita:cache:*), any admin-related keys
- Current mitigation: Code review; keys are scattered across modules.
- Recommendations: Create `alita/utils/redis/keys.go` with typed key builders; constants for all prefix patterns. Add CI check to reject `alita:` prefixes not from this file.

## Performance Bottlenecks

**Database Query N+1 in Federations:**
- Problem: `applyActiveFban` iterates members of a federation and may query the database once per member to check ban status.
- Files: `alita/modules/federations.go:756+`
- Cause: No batching of user ban checks; each goroutine may hit DB independently.
- Improvement path: Batch `GetBanStatus` calls; use `ZADD` in Redis to track ban state across federation chats in one transaction.

**Captcha Math Image Generation Blocking:**
- Problem: Generating captcha images for every join is CPU-bound and unthrottled; a raid of 100 joins/sec will spin up 100 image generations.
- Files: `alita/modules/captcha.go:500+` (GenerateIdQuestionAnswer), `mojocn/base64Captcha` library
- Cause: No request queue or worker pool for image generation.
- Improvement path: Add a bounded goroutine pool (e.g., 10 workers) for captcha generation. Queue requests with timeout.

**Antiflood Cleanup O(n) Scan:**
- Problem: `cleanupOnce()` iterates all active `(chat, user)` pairs every 5 minutes.
- Files: `alita/modules/antiflood.go:72-84`
- Cause: sync.Map does not support filtered iteration.
- Improvement path: For chats with 10K+ users, consider switching to a per-chat RwMutex with map, allowing targeted cleanup of stale users. Benchmark first.

## Fragile Areas

**Captcha Lifecycle Context Cancellation:**
- Files: `alita/modules/captcha.go:150+` (StartCaptchaLifecycle), `main.go:202+` (StopCaptchaLifecycle)
- Why fragile: WaitGroup is used to track tasks, but if a task panics before calling `Done()`, WaitGroup.Wait() hangs the shutdown for 10 seconds (hardcoded in main.go).
- Safe modification: Test panic injection in captcha tasks. Verify StopCaptchaLifecycle timeout is logged. Add a separate `context.Context` passed to tasks so they can be cancelled independently of WaitGroup.
- Test coverage: `alita/modules/captcha_test.go` lacks tests for lifecycle panics.

**Cache Local Eviction During Load:**
- Files: `alita/db/cache/local.go:*` (skipLocal list), `alita/db/cache/loader.go:34-40` (DeleteCache bumps generation)
- Why fragile: Two packages are named `cache` (`alita/db/cache` and `alita/utils/cache`); a developer adding a freshness-critical key must find and edit `skipLocal` in the first one, not the second.
- Safe modification: Consolidate `cache` packages or rename one to `cachedb`. Document in ARCHITECTURE.md which package handles which layer.
- Test coverage: `alita/db/cache/local_test.go` has 100+ test cases; coverage is good for this subsystem.

**Federation Deletion Cascade:**
- Files: `alita/modules/federations.go:*`, `alita/db/federations/repository.go:*`
- Why fragile: Deleting a federation must cascade to remove all bans, chat memberships, and pending operations. No explicit transaction or foreign key constraint visible.
- Safe modification: Audit `DeleteFederation()` to confirm it deletes related tables in correct order. Add integration test that deletes a federation with 100 bans and verifies no orphaned rows remain.
- Test coverage: No `_test.go` file; manual testing only.

**Greetings/Goodbyes Message Template Injection:**
- Files: `alita/modules/greetings.go:*` (message templating with {first}, {last}, {mention})
- Why fragile: If a username contains template syntax like `{first}`, recursive substitution could cause infinite loops or code injection.
- Safe modification: Validate template variables for injection syntax; escape all user input before substitution. Write test for `username = "{first}{first}{first}..."`.
- Test coverage: `alita/modules/greetings_command_test.go` exists but likely doesn't cover injection cases.

## Scaling Limits

**Antiflood Hash Collision Under Scale:**
- Current capacity: In-process sync.Map with cleanup every 5 minutes; designed for ~10K active (chat, user) pairs.
- Limit: At 100K concurrent users across 1000 chats, cleanup loop starts blocking handler threads. Memory usage grows to 500MB+.
- Scaling path: Move to Redis-backed counter with Lua script for atomic increment. Replace sync.Map with per-chat sync.Map to shard cleanup work.

**Federations Query Latency on Large Federations:**
- Current capacity: Queries assume <1000 chats per federation (based on no explicit pagination in UI).
- Limit: Federations with 10K+ chats cause UI timeouts when listing members. `applyActiveFban` spawns 10K goroutines.
- Scaling path: Paginate federation member listing in UI. Batch `applyActiveFban` operations into pools of 50.

**Backup Export Memory Footprint:**
- Current capacity: Full backup format (all chats, settings, bans) serialized to JSON in memory.
- Limit: A bot managing 10K chats with 1M bans will generate 50-200MB exports; uploading to Telegram will OOM on small replicas.
- Scaling path: Stream export to Telegram file upload API instead of buffering. Implement incremental exports (e.g., "chats since last export").

**Captcha Queue Capacity:**
- Current capacity: Fixed queue size `aispamQueueCapacity` (hardcoded, check `alita/modules/aispam.go` for value).
- Limit: Raids with join rate > queue capacity/second will shed checks. Shedding is acceptable per AGENTS.md section 5.1.
- Scaling path: Tune `aispamQueueCapacity` and worker count based on expected raid size. Monitor shed rate via metrics endpoint.

## Dependencies at Risk

**Redis Single-Point-of-Failure for Antiraid:**
- Risk: Antiraid module does nothing if Redis is down; users bypass raid detection during maintenance window.
- Impact: Undetected raid = manual cleanup of 100+ bot bans; federation bans may not synchronize.
- Migration plan: Implement in-process raid detection as fallback (join rate > N in last M seconds). Cache raid state in SQL when Redis unavailable. Mark alerts as "DEGRADED_RAID_DETECTION" in monitoring.

**base64Captcha Library Maintenance:**
- Risk: `mojocn/base64Captcha` has no releases since 2020; contains hardcoded image quality and font choices.
- Impact: Captcha images are 150+ KB each; image generation is slow on low-CPU replicas.
- Migration plan: Replace with lightweight Go-native captcha library (e.g., `dchest/captcha`, or in-house math-based only). Provide 1-year deprecation window.

**gotgbot Library Telegram API Version Drift:**
- Risk: If Telegram API changes and gotgbot does not update, bot handlers will fail silently or crash.
- Impact: New Telegram API features (e.g., new message types) are ignored; old handlers break on unexpected fields.
- Migration plan: Pin `github.com/PaulSonOfLars/gotgbot/v2` to major version; review release notes monthly. Set up CI to test against next major version in separate job.

## Missing Critical Features

**No Configuration Hot-Reload:**
- Problem: Changing environment variables requires full bot restart; a misconfiguration cascades to all shards.
- Blocks: Zero-downtime deployments; rapid feature flag toggles; per-shard overrides.
- Impact: Downtime = lost updates; missing context for debugging failed deployments.
- Workaround: Use external config server (e.g., Consul) with polling every 60s. Implement `/admin/reload-config` command.

**No Graceful Degradation for Telegram API Errors:**
- Problem: If Telegram API is slow or returns 503, handlers block on slow requests and dispatcher thread pool exhausts.
- Blocks: Tail latency under Telegram outages; cascade failures to other handlers.
- Impact: Entire bot becomes unresponsive during Telegram API issues, even for local operations.
- Workaround: Add circuit breaker to Telegram API calls; fail fast with cached response if circuit is open.

**No Rate Limiting on Admin Commands:**
- Problem: A malicious admin can spam `/start`, `/ban`, `/kick` without throttling.
- Blocks: Protection against accidental command loops or abuse.
- Impact: Database load spikes; message queue congestion.
- Workaround: Add per-admin per-command rate limit (e.g., 10/minute). Configurable via admin settings.

## Test Coverage Gaps

**Untested Antiflood Command:**
- What's not tested: `/antiflood` set/get/enable/disable commands; edge cases like concurrent updates.
- Files: `alita/modules/antiflood.go` (no test file exists)
- Risk: Regression goes unnoticed; users report settings not persisting.
- Priority: High — antiflood is deployed to 100+ chats.

**Untested Federation Operations:**
- What's not tested: Creating, joining, exporting, and deleting federations; ban propagation under errors; concurrent admin commands.
- Files: `alita/modules/federations.go`, `alita/modules/federations_io.go` (no test files)
- Risk: Silent data corruption (orphaned bans); federation bans fail on one shard but not others.
- Priority: High — used by large group networks.

**Untested Captcha Edge Cases:**
- What's not tested: Captcha timeouts; user solves captcha twice (stale attempt); concurrent joins same user/chat; image generation panics.
- Files: `alita/modules/captcha.go` (test file exists but may be incomplete)
- Risk: Users falsely banned during raids; captcha state leaks between sessions.
- Priority: High — captcha is critical for raid defense.

**Untested Backup Restore:**
- What's not tested: Restoring with corrupted JSON; missing fields; version mismatch; concurrent restore + live commands.
- Files: `alita/modules/backup.go`, `alita/modules/backup_test.go` (partial coverage)
- Risk: Unrecoverable data corruption; lost bans after restore.
- Priority: High — backup is data recovery safety net.

---

*Concerns audit: 2026-10-04*
