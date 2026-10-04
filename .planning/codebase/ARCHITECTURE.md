---
last_mapped_commit: 4d3518f822a0014f925b59b6748acaee47241152
last_mapped_at: 2026-10-04
---
<!-- refreshed: 2026-10-04 -->

# Architecture

**Analysis Date:** 2026-10-04

## System Overview

Alita Robot is a Telegram group-management bot built on a layered event-driven architecture with a priority-ordered handler system. The system processes Telegram updates through a dispatcher that routes them to priority-ordered handlers, which interact with a PostgreSQL database and Redis cache layer.

```text
┌──────────────────────────────────────────────────────────────────┐
│                  Telegram Bot Handlers                           │
│     (10+ priority groups: -10 to 11; process in order)           │
│  Priority: Captcha Sweeper → Fed-Ban → Antiraid → Admin Cache    │
│           → Users Tracker → Commands → Spam → Flood → Locks      │
│           → Blacklists → Reports → Filters → Pins → Logging      │
│  Location: `alita/modules/` (30+ module files)                   │
└──────────────────┬───────────────────────────────────────────────┘
                   │
        ┌──────────┴──────────┬────────────────┐
        │                     │                │
        ▼                     ▼                ▼
┌──────────────┐    ┌──────────────┐  ┌──────────────────┐
│   Moderation │    │  Security    │  │  Utilities &     │
│   Pipeline   │    │  Subsystems  │  │  Chat Features   │
│              │    │              │  │                  │
│ • Captcha    │    │ • Antiraid   │  │ • Greetings      │
│ • Antiflood  │    │ • Fed-ban    │  │ • Pins           │
│ • Locks      │    │ • Blacklist  │  │ • Notes          │
│ • Filters    │    │ • Reports    │  │ • Rules/Help     │
│ • Warns      │    │ • Reactions  │  │ • Reactions      │
│ • Bans       │    │ • Approvals  │  │ • Language       │
│              │    │ • AI Spam    │  │ • Misc Commands  │
│              │    │              │  │                  │
│ `alita/      │    │ `alita/      │  │ `alita/modules/` │
│ modules/`    │    │ modules/`    │  │                  │
└──────────────┘    └──────────────┘  └──────────────────┘
        │                   │                │
        └───────────────────┴────────────────┘
                    │
        ┌───────────▼───────────┐
        │  Command Pipeline &   │
        │  Permission Checks    │
        │                       │
        │ `helpers/cmd_pipeline`│
        │ `chat_status/`        │
        │ `extraction/`         │
        └───────────────────────┘
                    │
        ┌───────────▼───────────┐
        │   Internationalization│
        │   i18n Translator     │
        │                       │
        │ `alita/i18n/`         │
        │ `locales/*.yml` (7)   │
        └───────────────────────┘
                    │
        ┌───────────▼───────────────────────────────────┐
        │      Database & Cache Layer                   │
        ├───────────────────────────────────────────────┤
        │  PostgreSQL (GORM ORM)  │   Redis + Local LRU │
        │                         │   (two-tier cache)  │
        │ • `alita/db/migrations` │                     │
        │   (40+ SQL migrations)  │  • Generation-based │
        │ • `alita/db/models/`    │    invalidation     │
        │ • `alita/db/<domain>/`  │  • 10s local TTL    │
        │   (30+ repositories)    │    (configurable)   │
        │ • Direct queries (GORM) │  • Redis prefix:    │
        │ • Upserts & batches     │    `alita:cache:*`  │
        │ • Error tracing         │  • Oper. keys:      │
        │                         │    `alita:*`        │
        │ User, Chat, Filters,    │                     │
        │ Warns, Bans, Notes,     │                     │
        │ Locks, Federations,     │                     │
        │ Settings, etc.          │                     │
        └───────────────────────────────────────────────┘
```

## Component Responsibilities

| Component | Responsibility | File |
|-----------|----------------|------|
| Dispatcher | Routes updates to handlers by priority group | `main.go` (lines 129, 373-379) |
| Module Registry | Registers handlers with priority order; loads in sorted order | `alita/modules/registry.go` |
| Handler Groups | Organize execution: -10 (captcha) through 11 (logging) | `AGENTS.md` |
| Command Pipeline | Builds context, checks permissions, invokes command handler | `alita/utils/helpers/command_pipeline.go` |
| Database Repos | Read via cache layer; write with cache invalidation | `alita/db/<domain>/` (30+ files) |
| Cache Layer | Two-tier: local in-process LRU + Redis; generation-based invalidation | `alita/db/cache/` |
| i18n Translator | Loads 7 locales; translates strings on-demand | `alita/i18n/` |
| HTTP Server | Health checks, metrics, webhook (or polling health) | `alita/utils/httpserver/` |
| Shutdown Manager | LIFO drain of goroutines and resources within 60s | `alita/utils/shutdown/` |
| Monitoring | Stats collection, error tracking, activity monitoring, performance remediation | `alita/utils/monitoring/` |

## Pattern Overview

**Overall:** Priority-ordered event dispatcher with modular handler registration.

**Key Characteristics:**
- **Priority-based execution:** Handlers run in numeric order (-10 to 11); first matching handler can halt further processing
- **Module lazy-registration:** Each module registers itself via `init()` at module load time; duplicates are dropped
- **Request-scoped context:** `CommandContext` wraps bot, update, chat, message, user, and translator for each command
- **Two-tier caching:** Local LRU (10s TTL) + Redis (configurable TTL) with generation-based invalidation
- **Repository pattern:** Domain-organized repositories with GORM for reads/writes; cache.DeleteCache must be called on writes
- **Graceful shutdown:** LIFO handler drain (users writes → monitoring → tracer → anti-raid → captcha → AI spam)

## Layers

**Handler/Dispatcher Layer:**
- Purpose: Process Telegram updates; dispatch to priority-ordered modules
- Location: `alita/modules/`, `main.go` (lines 105-302)
- Contains: Handler registration, command wrappers, permission checks, callback routing
- Depends on: gotgbot/v2 dispatcher, i18n, chat_status utilities, database repos
- Used by: Main dispatcher loop in polling or webhook mode

**Command Pipeline Layer:**
- Purpose: Build CommandContext, check permissions, invoke handlers
- Location: `alita/utils/helpers/command_pipeline.go`
- Contains: CommandDescriptor, CheckFunc (permission predicates), WrapCommand wrapper
- Depends on: chat_status, i18n, extraction utilities, CommandContext builder
- Used by: Individual module command handlers

**Permission & Chat Status Layer:**
- Purpose: Predicate checks for admin status, group-only, disabled commands, approvals
- Location: `alita/utils/chat_status/`
- Contains: IsUserAdmin, IsGroupAdmin, RequireGroup, CheckDisabledCmd (return bools, never reply)
- Depends on: Cache layer (admin list caching), chat/user models
- Used by: Command pipeline, handlers via predicates or PermissionResponder

**Internationalization Layer:**
- Purpose: Localize messages across 7 languages (en, id, pt, ro, tr, hi, es)
- Location: `alita/i18n/`, `locales/` (embedded), `alita/db/lang/`
- Contains: Translator (stateful per language), cache of parsed YAML, fallback to English
- Depends on: Embedded locale files, user language preference from DB
- Used by: All handlers calling tr.GetString(key)

**Database/Persistence Layer:**
- Purpose: Model definitions and repository pattern for domain-specific data access
- Location: `alita/db/models/`, `alita/db/<domain>/` (30+ domain directories)
- Contains: GORM models (User, Chat, ChatFilters, etc.), repository functions (Create, Update, Delete, Get)
- Depends on: PostgreSQL connection, GORM ORM, cache layer for reads
- Used by: Handlers, cached loaders, upsert paths

**Cache Layer (Two-Tier):**
- Purpose: Reduce database load via local LRU (10s) + Redis (configurable)
- Location: `alita/db/cache/`, `alita/utils/cache/`
- Contains: GetFromCacheOrLoad (with concurrent-load dedup), DeleteCache, generation-based invalidation
- Depends on: Redis client, msgpack serialization, local LRU expirable map
- Used by: Repository read functions; generation counter bumped on DeleteCache to prevent stale writes

**HTTP Server & Monitoring Layer:**
- Purpose: Health checks, metrics export, webhook reception (alternative to polling)
- Location: `alita/utils/httpserver/`, `alita/utils/monitoring/`
- Contains: HTTP routes (/health, /metrics, /webhook), stats collection, error callbacks, activity tracking
- Depends on: Main dispatcher, shutdown manager, tracer provider
- Used by: Docker healthcheck, Prometheus scraping, Telegram webhook polling

## Data Flow

### Primary Request Path (Command)

1. **Update Arrival** → Dispatcher receives Telegram update (`main.go` line 267-302 polling or webhook)
2. **Tracing Span Open** → Dispatcher wraps in OpenTelemetry span (`tracing.TracingProcessor`)
3. **Handler Group Loop** → Iterates handlers in priority order (-10 to 11)
4. **Module Handler Match** → First matching handler (command, message filter, etc.) invokes callback
5. **CommandContext Build** → `helpers.BuildCommandContext` extracts bot, message, user, chat, translator
6. **Permission Checks** → `desc.RequiredChecks` predicates (IsUserAdmin, RequireGroup, etc.) return bool or end
7. **Handler Execution** → Module handler invokes business logic (command execution, filter check, etc.)
8. **Database Access** → Handler calls repository function (via `cache.GetFromCacheOrLoad` for reads)
9. **Cache Write** → On state change, handler calls `cache.DeleteCache(key)` to invalidate
10. **Telegram API Call** → Handler sends reply, edits message, kicks user, etc. via `c.Bot.SendMessage(...)`
11. **Tracing Span Close** → Span recorded with attributes and status
12. **Group Halt or Continue** → Handler returns `ext.EndGroups` (stop) or `ext.ContinueGroups` (next) or nil

### Antiraid Flow (Real-Time Join Monitoring)

1. **Join Message Arrives** → `ChatMemberUpdated` or service message with `new_chat_member`
2. **Membership Handler** → Captcha sweeper (-10) or antiraid (priority 230) runs
3. **Join Deduplication** → `claimRecentJoinProcessing` prevents duplicate processing
4. **Redis-Based Tracking** → Antiraid stores `alita:antiraid:*` counters (in-process backup)
5. **Rate Limit Check** → Compare join count against thresholds in timeframe
6. **Auto-Action** → Mute, kick, or ban users; restart captcha
7. **Expiry Poller** → Background task (`modules.StopAntiRaidExpiryPoller`) cleans expired records

### Cache Invalidation Flow

1. **Write Operation** → Repository (e.g., `user.UpdateUser`) calls GORM `Update/Upsert`
2. **Cache Invalidation** → Handler immediately calls `cache.DeleteCache(cache.CacheKey("user", id))`
3. **Local LRU Evict** → Generation counter bumped; in-flight load skips local write
4. **Redis Delete** → Key removed from Redis on this replica only
5. **Next Read** → `GetFromCacheOrLoad` misses cache, runs loader function, re-populates both tiers

### State Management

**In-Process:**
- Module singletons: `defaultHelpRegistry` (read-write under `ableMapMu`)
- Antiflood counters: Per-replica in-memory (no Redis)
- Admin cache: Refreshed per-command (memoized within update)

**Redis (Cluster-Wide):**
- Federation ban lookups: `alita:cache:fban:<fed>:<user>`
- Anti-raid join counters: `alita:antiraid:*` (NOT under cache prefix; survives `CLEAR_CACHE_ON_STARTUP`)
- Anonymous admin flag: `alita:anonAdmin:*`
- Captcha pending flag: `alita:cache:captcha_pending:<chat>` (MUST invalidate after insert)

**Database (Persistent):**
- Users, chats, filters, warns, bans, notes, locks, settings, federations
- Schema migrations tracked in `schema_migrations` with SHA-256 checksum

## Key Abstractions

**Module (moduleStruct):**
- Purpose: Encapsulates a bot feature (antiflood, filters, warns, etc.)
- Location: `alita/modules/<feature>.go`
- Pattern: Exported Load function + receiver methods (value receivers) + init() registration
- Example: `adminModule.adminlist(c *helpers.CommandContext) error` → returns `ext.EndGroups` on success or error

**CommandDescriptor:**
- Purpose: Metadata for command registration (name, aliases, group, checks, disableable flag)
- Location: `alita/utils/helpers/command_pipeline.go`
- Used by: `WrapCommand` to register with dispatcher and disabling system

**CheckFunc:**
- Purpose: Permission predicate returning bool (never replies)
- Signature: `func(c *CommandContext) bool`
- Examples: `IsUserAdmin`, `RequireGroup`, `CheckDisabled`
- Pattern: Return false to halt command; handler pipeline sends error reply via `PermissionResponder`

**CommandContext:**
- Purpose: Request-scoped bundling of bot, update, chat, message, user, translator
- Location: `alita/utils/helpers/command_pipeline.go`
- Passed to: All command handlers and check functions
- Lifetime: Built per-command, discarded after response

**Repository:**
- Purpose: Domain-specific data access (e.g., user.EnsureUserInDb, warns.GetWarnsCount)
- Location: `alita/db/<domain>/repository.go` (30+ domains)
- Pattern: Public functions using GORM; internal use of `cache.GetFromCacheOrLoad` and `cache.DeleteCache`
- Example: `func GetUserBasicInfoCached(userId int64) (*models.User, error)` wraps GORM in cache

## Entry Points

**Main Binary (`main.go`):**
- Location: `/home/user/Fuku_Robot/main.go`
- Triggers: Binary execution or Docker entrypoint
- Responsibilities:
  1. Parse CLI flags (--version, --health, normal startup)
  2. Initialize cache (Redis + local LRU)
  3. Initialize i18n (embed 7 locales)
  4. Initialize tracing (OpenTelemetry)
  5. Create gotgbot.Bot with HTTP/2 pooling
  6. Setup dispatcher and register handlers
  7. Start monitoring (stats, auto-remediation, activity)
  8. Register shutdown handlers (LIFO order)
  9. Choose mode: webhook or polling
  10. Enter idle loop

**Webhook Handler (when USE_WEBHOOKS=true):**
- Location: `alita/utils/httpserver/webhook.go`
- Triggers: Telegram sends POST to `/telegram/webhook/<token>`
- Responsibilities:
  1. Verify webhook signature (WEBHOOK_SECRET)
  2. Unmarshal Telegram update
  3. Dispatch to dispatcher
  4. Return 200 OK to Telegram
- Alternative: Polling updater (simpler, no public endpoint needed)

**CLI Modes:**
- `--version`: Print bot version, skip DB init, exit
- `--health`: Check `/health` endpoint, exit
- Normal: Start dispatcher loop

## Architectural Constraints

- **Threading:** Concurrent: the gotgbot dispatcher runs up to `DISPATCHER_MAX_ROUTINES` (default 200) updates in parallel goroutines (`main.go`, `alita/config/config.go`), so handlers must be race-safe; async writes to users table join WaitGroup drained by `modules.DrainUsersAsyncWrites` on shutdown
- **Global state:** 
  - `defaultHelpRegistry` (module enable/disable map) — guarded by `ableMapMu`
  - `db.DB` (GORM connection) — shared across handlers; migrations run on startup if `AUTO_MIGRATE=true`
  - `cacheGenerations` array (4096 stripe counters for generation-based invalidation)
- **Circular imports:** Import hierarchy: stdlib → third-party (gotgbot, gorm, logrus) → internal (`alita/db` → `alita/modules` → `alita/utils`)
- **Database initialization order:**
  - `alita/config` init → `alita/db` init (connects to Postgres, runs migrations, may fail startup)
  - No DB access in `init()` functions running before `alita/db` init
  - CLI modes (`--version`, `--health`) skip DB init via `isCliModeActive()` check
- **Shutdown order (LIFO within 60s):**
  1. Database close
  2. Users async writes drain
  3. Monitoring systems (stats, auto-remediation, activity monitor)
  4. Database monitoring context cancel
  5. Tracer shutdown
  6. Anti-raid expiry poller stop
  7. Captcha lifecycle stop
  8. AI spam checks drain
  9. HTTP server stop
  10. Polling updater stop (if not webhook)

## Anti-Patterns

### Discarding DB Errors on State-Changing Paths

**What happens:** Handler calls `db.UpdateRecord()`, ignores error, continues as if write succeeded.

**Why it's wrong:** Silent failures lead to stale state; cache invalidation (DeleteCache) may not be paired with failed write, causing database and cache to diverge.

**Do this instead:** Check error before cache invalidation. Example from `alita/db/user/repository.go` line 38: `result := db.DB.Where(...).Assign(...).FirstOrCreate(...); if result.Error != nil { log.Errorf(...); return result.Error }`

### Fire-and-Forget Goroutines Without Panic Recovery

**What happens:** Handler spawns goroutine without `defer error_handling.RecoverFromPanic()`.

**Why it's wrong:** Panic in goroutine crashes entire bot process; silently kills polling loop or webhook handler.

**Do this instead:** Wrap goroutine entry point with `defer error_handling.RecoverFromPanic("module", "function")`. All async user/chat writers must join `modules.DrainUsersAsyncWrites()` WaitGroup for clean shutdown.

### Typo'd i18n Keys

**What happens:** Handler calls `tr.GetString("typo_key")`; returns `""` with error callers ignore.

**Why it's wrong:** User sees empty message; message appears to be a bot bug.

**Do this instead:** Copy key from `locales/en.yml` exactly. Verify key exists in all 7 locale files before merging. Run `make check-translations` in CI.

### Unguarded i18n Fallback to Empty String

**What happens:** i18n loader wraps missing key; callers ignore error, use `""` in message.

**Why it's wrong:** Silent message loss; hard to debug in production.

**Do this instead:** Always pair GetString with error check or use `i18n.MustNewTranslator` + must-have keys. Test locale reload with `make check-translations`.

### Mocking in Tests Instead of Using Real Fixtures

**What happens:** Test uses gomock or testify/mock instead of real `internal/testdb.Run()` or miniredis.

**Why it's wrong:** Mocks diverge from real behavior; gate test passes but production fails.

**Do this instead:** Use real fixtures: `internal/testdb.Run(SQLite)`, `miniredis` for Redis, hand-written `gotgbot.BotClient` fakes. Assert observable behavior (reply sent, row persisted, cache invalidated), not test-double internals.

### Updating Record Without Checking Zero Values

**What happens:** Handler calls `db.UpdateRecord(model, where, updates)` with `false` or `0` or `""` fields; GORM skips them.

**Why it's wrong:** Cannot unset fields (e.g., disable a feature, clear a setting).

**Do this instead:** Use `db.UpdateRecordWithZeroValues(model, where, updates)` when intentional zero values must be written.

---

*Architecture analysis: 2026-10-04*
