---
last_mapped_commit: 4d3518f822a0014f925b59b6748acaee47241152
last_mapped_at: 2026-10-04
---
# Codebase Structure

**Analysis Date:** 2026-10-04

## Directory Layout

```
/home/user/Fuku_Robot/
├── main.go                         # Entry point; CLI flags, startup flow
├── main_test.go                    # Main binary tests
├── Makefile                        # Build, test, lint, version bump, docs generation
├── go.mod, go.sum                  # Go module dependencies (gotgbot, gorm, redis, logrus, etc.)
├── go.work                         # Workspace file (if monorepo)
├── AGENTS.md                       # Project-specific rules: handlers, callbacks, permissions, migrations, testing
├── README.md                       # High-level overview
├── LICENSE                         # MIT or similar
├── Procfile                        # Heroku deployment config (run command)
├── app.json                        # Heroku app manifest (env vars, addons)
├── .env                            # Local development env (auto-loaded, not versioned)
├── .dockerignore                   # Docker build exclusions
├── .gitignore                      # Git exclusions
├── .golangci.yml                   # golangci-lint configuration
├── .goreleaser.yaml                # GoReleaser config (build multi-platform binaries)
├── .github/
│   ├── workflows/                  # CI/CD: test, lint, build, release
│   └── ...
├── .pre-commit-config.yaml         # Git pre-commit hooks
├── .claude/                        # Claude Code configuration
│   └── ...
├── .planning/
│   └── codebase/                   # This directory: architecture & structure docs
├── debug.docker-compose.yml        # Local dev: PostgreSQL + Redis + bot
│
├── alita/                          # Core bot package
│   ├── main.go                     # LoadModules, InitialChecks, ListModules
│   ├── main_test.go                # Package-level tests
│   │
│   ├── config/                     # Configuration & environment
│   │   ├── config.go               # Environment parsing (DATABASE_URL, REDIS_URL, ports, etc.)
│   │   ├── types.go                # AppConfig struct (BotToken, HTTPPort, DispatcherMaxRoutines, etc.)
│   │   ├── config_test.go          # Config parsing tests
│   │   └── config_pure_test.go     # Pure logic tests (no setup)
│   │
│   ├── db/                         # Database layer: GORM ORM + PostgreSQL + cache
│   │   ├── db.go                   # Global functions (CreateRecord, UpdateRecord, DeleteRecord)
│   │   ├── conn.go                 # PostgreSQL connection setup, migrations
│   │   ├── migrations/             # SQL migration files (named by timestamp)
│   │   │   ├── 001_*.sql           # Initial schema (users, chats, filters, warns, etc.)
│   │   │   ├── 002_*.sql           # Schema additions/alterations
│   │   │   ├── runner.go           # Migration executor
│   │   │   └── ...
│   │   ├── models/                 # GORM data models
│   │   │   ├── user.go             # User struct with user_id, username, name, language
│   │   │   ├── chat.go             # Chat struct (if exists)
│   │   │   ├── admin.go            # Admin (per-chat admin list)
│   │   │   ├── warns.go            # Warns (user warnings in chat)
│   │   │   ├── bans.go             # Bans (ban list)
│   │   │   ├── locks.go            # Locks (locked media types per chat)
│   │   │   ├── filters.go          # Filters (chat-specific keyword filters)
│   │   │   ├── notes.go            # Notes (saved messages)
│   │   │   ├── federations.go      # Federations (fed bans)
│   │   │   ├── connections.go      # Connections (chat auth tokens)
│   │   │   ├── antiflood.go        # Antiflood settings
│   │   │   ├── antiraid.go         # Antiraid settings
│   │   │   ├── captcha.go          # Captcha attempts & settings
│   │   │   ├── aispam.go           # AI spam detection log
│   │   │   ├── approvals.go        # Approved users (skip moderation)
│   │   │   ├── blacklists.go       # User/chat blacklists
│   │   │   ├── channels.go         # Monitored channels
│   │   │   ├── devs.go             # Developer/owner settings
│   │   │   ├── disabling.go        # Disabled commands per chat
│   │   │   ├── greetings.go        # Welcome/goodbye messages
│   │   │   ├── lang.go             # User language preference
│   │   │   ├── logchannels.go      # Logging destination chats
│   │   │   ├── pins.go             # Pinned messages tracking
│   │   │   ├── reactions.go        # Emoji reactions tracking
│   │   │   ├── reports.go          # Report history
│   │   │   ├── rules.go            # Chat rules
│   │   │   └── ... (30+ model files)
│   │   ├── <domain>/               # Domain repositories (one per feature)
│   │   │   ├── user/               # User repo: EnsureUserInDb, UpdateUser, GetUserBasicInfoCached
│   │   │   │   ├── repository.go   # Public functions (EnsureBotInDb, UpdateUser, etc.)
│   │   │   │   ├── optimized.go    # Batch/optimized queries
│   │   │   │   └── repository_test.go
│   │   │   ├── warns/              # Warns repo: GetWarnsCount, AddWarn, ResetWarns
│   │   │   │   ├── repository.go
│   │   │   │   └── repository_test.go
│   │   │   ├── filters/            # Filters repo: GetChatFilters, AddFilter, DeleteFilter
│   │   │   ├── bans/               # Bans repo
│   │   │   ├── federations/        # Federations repo: GetFedBan, AddBan
│   │   │   ├── locks/              # Locks repo
│   │   │   ├── notes/              # Notes repo
│   │   │   ├── approvals/          # Approvals repo
│   │   │   ├── admin/              # Admin repo (cache loading)
│   │   │   ├── captcha/            # Captcha repo
│   │   │   ├── ... (30+ domains)
│   │   ├── cache/                  # Cache layer: Redis + local LRU
│   │   │   ├── loader.go           # GetFromCacheOrLoad, generation-based invalidation
│   │   │   ├── keys.go             # Cache key builders (CacheKey function)
│   │   │   ├── ttl.go              # TTL configuration per key type
│   │   │   ├── local.go            # Local in-process LRU (10s TTL)
│   │   │   ├── local_testtools.go  # Test helpers (ResetLocalForTest)
│   │   │   └── ... (bench, tests)
│   │   ├── test_helpers.go         # Shared DB test utilities
│   │   ├── testmain_test.go        # Test setup (SQLite AutoMigrate, miniredis)
│   │   ├── metrics.go              # DB query metrics (Prometheus)
│   │   └── ... (tests, constraint checks)
│   │
│   ├── i18n/                       # Internationalization
│   │   ├── manager.go              # I18nManager (loads, parses, translates YAML locales)
│   │   ├── translator.go           # Translator (per-language instance, GetString, caching)
│   │   └── testdata/               # Test fixtures
│   │
│   ├── modules/                    # Bot features (30+ modules)
│   │   ├── core.go                 # moduleStruct, GetAbleMap, SetModuleEnabled, help registry
│   │   ├── registry.go             # RegisterLegacyModule, LoadAllModules, priority sorting
│   │   │
│   │   # Moderation modules (handler groups -10 to 11)
│   │   ├── captcha.go              # Captcha sweeper (-10 priority); LoadCaptcha
│   │   ├── antiraid.go             # Anti-raid join detection (priority 230); LoadAntiRaid
│   │   ├── federations.go          # Fed-ban lookup (priority -6); LoadFederations
│   │   ├── antiflood.go            # Anti-spam flood (priority 4); LoadAntiflood
│   │   ├── locks.go                # Content locks (priority 5-6); LoadLocks
│   │   ├── blacklists.go           # Blacklist enforcement (priority 7); LoadBlacklists
│   │   ├── filters.go              # Keyword filters (priority 9); LoadFilters
│   │   ├── warns.go                # Warning system; LoadWarns
│   │   ├── bans.go                 # Ban enforcement; LoadBans
│   │   │
│   │   # Admin commands
│   │   ├── admin.go                # /adminlist, demote, promote; LoadAdmin
│   │   ├── approvals.go            # /approve, /unapprove; LoadApprovals
│   │   ├── devs.go                 # /dev (owner-only); LoadDevs
│   │   ├── moderation.go           # /kick, /ban, /warn, /mute, /unmute; LoadModeration
│   │   ├── disabling.go            # /disable, /enable (per-chat); LoadDisabling
│   │   ├── reports.go              # Report system; LoadReports
│   │   │
│   │   # Feature modules
│   │   ├── greetings.go            # /welcome, /goodbye; LoadGreetings
│   │   ├── notes.go                # /save, /get, /del note; LoadNotes
│   │   ├── pins.go                 # /pin, /unpin (with logging); LoadPins
│   │   ├── rules.go                # /rules, /setrules; LoadRules
│   │   ├── reactions.go            # Emoji reaction tracking; LoadReactions
│   │   ├── backups.go              # /backup (export chat settings); LoadBackup
│   │   ├── connections.go          # /connect (sync settings across chats); LoadConnections
│   │   │
│   │   # Utility modules
│   │   ├── help.go                 # /help, help buttons; LoadHelp (loaded last)
│   │   ├── language.go             # /lang (user language); LoadLanguage
│   │   ├── misc.go                 # /start, /info, /quote; LoadMisc
│   │   ├── users.go                # /user (info lookup); LoadUsers
│   │   ├── aispam.go               # AI spam detection (TYPESAFE_API_KEY); LoadAISpam
│   │   │
│   │   # Internal helpers
│   │   ├── callback_context.go     # Callback update parsing & routing
│   │   ├── callback_codec.go       # Encode/decode callback_data (64-byte cap)
│   │   ├── callback_parse_overwrite.go  # Handle button data overwrite logic
│   │   ├── anonymous_admin_router.go    # Route commands for anonymous admins
│   │   ├── bot_updates.go          # Process ChatMemberUpdated (join/leave)
│   │   ├── error_logging.go        # Log errors to message dump
│   │   ├── membership.go           # Track user joins/leaves
│   │   ├── chat_permissions.go     # Apply/restore Telegram permissions
│   │   ├── moderation_pipeline.go  # Shared pipeline (kick, ban, warn)
│   │   ├── moderation_input.go     # Parse /kick, /ban targets
│   │   ├── deeplink_router.go      # Handle /start deep links
│   │   ├── formatting.go           # Format messages (replies, mentions, etc.)
│   │   ├── i18n_helpers.go         # i18n wrapper helpers
│   │   ├── help_system.go          # Help button rendering
│   │   ├── logchannels.go          # Send action logs to log channel
│   │   ├── purges.go               # /purge (delete messages)
│   │   ├── rules_format.go         # Format rules display
│   │   ├── federations_io.go       # Import/export federation data
│   │   ├── overwrite.go            # Handle command overwrite system
│   │   ├── mute.go                 # Mute/unmute user
│   │   ├── keyboard.go             # Inline keyboard builders
│   │   │
│   │   # Tests
│   │   ├── *_test.go               # Unit tests per module
│   │   ├── *_command_test.go       # Command-specific tests
│   │   ├── test_harness_test.go    # Shared test fixtures
│   │   ├── module_wait_test.go     # WaitGroup test helpers
│   │   └── ... (60+ test files)
│   │
│   ├── utils/                      # Utility packages
│   │   ├── cache/                  # Cache utilities (InitCache, GetMarshal — separate from db/cache)
│   │   │   ├── cache.go            # Redis client setup
│   │   │   └── cache_test.go
│   │   ├── chat_status/            # Permission predicates
│   │   │   ├── admin_status.go     # IsUserAdmin, IsGroupAdmin (return bool)
│   │   │   ├── chat_status.go      # Chat type checks, RequireGroup, RequirePrivate
│   │   │   ├── permission_responder.go  # PermissionResponder (sends error replies)
│   │   │   └── ... (tests)
│   │   ├── constants/              # Time constants (DefaultTimeout, VeryLongTimeout)
│   │   ├── content/                # Content type extraction
│   │   ├── error_handling/         # RecoverFromPanic, error callbacks
│   │   ├── errors/                 # WrappedError (file, line, function info)
│   │   ├── extraction/             # Extract user/chat from message (parse @username, reply_to_user, etc.)
│   │   ├── formatting/             # HTML escaping, mention builders
│   │   ├── helpers/                # Command pipeline, decorators
│   │   │   ├── command_pipeline.go # CommandContext, WrapCommand, CheckFunc
│   │   │   ├── decorators.go       # Administrative decorators
│   │   │   ├── telegram_helpers.go # Telegram-specific helpers (IsExpectedTelegramError, etc.)
│   │   │   └── ... (test helpers)
│   │   ├── httpserver/             # HTTP server (health, metrics, webhook)
│   │   │   ├── server.go           # HTTP server setup, route registration
│   │   │   ├── webhook.go          # Webhook handler (signature verification, update dispatch)
│   │   │   ├── health.go           # /health endpoint
│   │   │   ├── metrics.go          # /metrics endpoint (Prometheus)
│   │   │   └── ... (test helpers)
│   │   ├── keyboard/               # Telegram inline keyboard builders
│   │   ├── keyword_matcher/        # Regex keyword matching for filters
│   │   ├── logredact/              # Log redaction (RegisterSecret)
│   │   ├── media/                  # Media type detection
│   │   ├── metrics/                # Prometheus metrics registration
│   │   ├── monitoring/             # Stats collection, auto-remediation, activity monitor
│   │   ├── ratelimit/              # Rate limiter (optional; not all modules use)
│   │   ├── shutdown/               # Shutdown manager (LIFO handler drain)
│   │   ├── tracing/                # OpenTelemetry integration
│   │   ├── actionlog/              # Action logging (kick, ban, etc.)
│   │   ├── updatememo/             # Memoize data per update (admin cache, etc.)
│   │   └── ... (utility packages)
│   │
│   └── ... (other packages)
│
├── locales/                        # Embedded i18n files
│   ├── en.yml                      # English (default, used by docs generator)
│   ├── id.yml                      # Indonesian
│   ├── pt.yml                      # Portuguese
│   ├── ro.yml                      # Romanian
│   ├── tr.yml                      # Turkish
│   ├── hi.yml                      # Hindi
│   ├── es.yml                      # Spanish
│   └── config.yml                  # Help alt-names (pseudo-locale, not a language)
│
├── scripts/                        # Build and utility scripts
│   ├── validate_orphaned_data.go   # Check for orphaned database records
│   ├── check_translations/         # Verify all locale keys match en.yml
│   │   ├── main.go                 # Check logic
│   │   └── main_test.go
│   ├── migrate_psql.sh             # PostgreSQL migration runner (paired with db/migrations/runner.go)
│   └── ... (other scripts)
│
└── docs/                           # Generated documentation
    ├── api-reference/              # Auto-generated from help registry
    │   ├── commands.md
    │   ├── lock-types.md
    │   └── ... (frozen with <!-- MANUALLY MAINTAINED -->)
    └── ...
```

## Directory Purposes

**`/home/user/Fuku_Robot/`:**
- Purpose: Project root; Go module entry point, build configs, CI/CD definitions
- Contains: main.go, Makefile, go.mod, configs (Docker, Heroku, pre-commit)

**`alita/`:**
- Purpose: Core bot package (github.com/divkix/Alita_Robot/alita)
- Contains: Config, database, modules, utilities, i18n

**`alita/config/`:**
- Purpose: Environment variable parsing and configuration
- Contains: AppConfig struct (BotToken, HTTPPort, etc.), env value getters

**`alita/db/`:**
- Purpose: Data persistence layer (GORM + PostgreSQL + cache)
- Key files:
  - `db.go`: Global CRUD functions (CreateRecord, UpdateRecord, DeleteRecord)
  - `conn.go`: Database connection and migration initialization
  - `models/`: GORM struct definitions
  - `<domain>/`: Domain-specific repositories
  - `cache/`: Two-tier cache (local LRU + Redis)
  - `migrations/`: SQL migration files (schema source of truth)

**`alita/db/models/`:**
- Purpose: GORM data models (User, Chat, Warns, Bans, etc.)
- Convention: Each model has `TableName()` method returning migration table name

**`alita/db/<domain>/`:**
- Purpose: Feature-specific data access (user, warns, filters, etc.)
- Contains: `repository.go` (public CRUD), `optimized.go` (batch queries), tests
- Pattern: Reads use `cache.GetFromCacheOrLoad`; writes call `cache.DeleteCache` after GORM update

**`alita/db/cache/`:**
- Purpose: Two-tier cache system (local in-process LRU + Redis)
- Key files:
  - `loader.go`: `GetFromCacheOrLoad` (concurrent load dedup, generation counters)
  - `keys.go`: Cache key builders
  - `ttl.go`: TTL configuration per key type
  - `local.go`: In-process LRU (10s TTL by default)

**`alita/db/migrations/`:**
- Purpose: SQL schema migrations (source of truth)
- Naming: Timestamp (e.g., 1693650789_create_users.sql); sorted by filename
- Key file: `runner.go` pairs with `scripts/migrate_psql.sh`

**`alita/i18n/`:**
- Purpose: Localization system
- Contains: Translator (per-language instance), Manager (loads and parses YAML)
- Embedded locales: `locales/` directory (7 languages + config.yml)

**`alita/modules/`:**
- Purpose: Bot feature implementations (30+ feature modules)
- Key files:
  - `core.go`: Module registry structure, enable/disable map
  - `registry.go`: RegisterLegacyModule, LoadAllModules (priority sort)
  - Individual modules: `antiraid.go`, `filters.go`, `warns.go`, etc. (each has init() registration)
- Pattern: Each module registers via `RegisterLegacyModule(name, priority, LoadFunc)` in init()
- Handler groups: -10 (captcha) through 11 (logging)

**`alita/utils/`:**
- Purpose: Cross-cutting utilities
- Sub-packages:
  - `helpers/`: Command pipeline, CommandContext, WrapCommand
  - `chat_status/`: Permission predicates (IsUserAdmin, etc.)
  - `error_handling/`: Panic recovery, error callbacks
  - `httpserver/`: HTTP server (health, metrics, webhook)
  - `tracing/`: OpenTelemetry spans
  - `shutdown/`: LIFO shutdown manager
  - `monitoring/`: Stats, error tracking, activity
  - `extraction/`: User/chat/text extraction from Telegram updates
  - `formatting/`: HTML escaping, mentions
  - Others: cache, constants, content, errors, keyboard, etc.

**`locales/`:**
- Purpose: Embedded i18n files (7 languages)
- Files: `en.yml` (default), `id.yml`, `pt.yml`, `ro.yml`, `tr.yml`, `hi.yml`, `es.yml`, `config.yml`
- Referenced by: `i18n.Manager` loads from embedded `locales/` directory at runtime

**`scripts/`:**
- Purpose: Build, validation, and utility scripts
- Key files:
  - `check_translations/`: Verify locale key completeness
  - `migrate_psql.sh`: Apply SQL migrations to PostgreSQL
  - `validate_orphaned_data.go`: Check for orphaned records

**`docs/`:**
- Purpose: Generated documentation (auto-generated or manually maintained)
- Source: Docs generator reads `locales/en.yml` and help registry
- Special: Pages with `<!-- MANUALLY MAINTAINED: do not regenerate -->` are frozen

## Key File Locations

**Entry Points:**
- `main.go` (line 41): Bot startup, initialization sequence
- `alita/main.go`: LoadModules, InitialChecks
- `alita/utils/httpserver/webhook.go`: Webhook update reception

**Configuration:**
- `alita/config/config.go`: Environment parsing
- `alita/config/types.go`: AppConfig struct definition
- `.env`: Local development (auto-loaded by godotenv)
- `app.json`: Heroku manifest

**Core Logic:**
- `alita/modules/registry.go`: RegisterLegacyModule, LoadAllModules (execution order)
- `alita/modules/core.go`: Module struct, enable/disable map
- `alita/utils/helpers/command_pipeline.go`: Command dispatch, permission checks
- `alita/db/db.go`: Global CRUD functions (CreateRecord, UpdateRecord)
- `alita/db/cache/loader.go`: GetFromCacheOrLoad (cache invalidation strategy)

**Database & Persistence:**
- `alita/db/migrations/`: SQL schema files (source of truth)
- `alita/db/migrations/runner.go`: Migration applier
- `alita/db/models/`: GORM struct definitions
- `alita/db/<domain>/repository.go`: Domain-specific repository functions

**Testing:**
- `alita/db/testmain_test.go`: Test setup (AutoMigrate, miniredis)
- `alita/modules/test_harness_test.go`: Shared test fixtures
- `*_test.go`: Unit tests (co-located with implementation)
- `*_command_test.go`: Command-specific tests

## Naming Conventions

**Files:**
- `<feature>.go`: Main implementation file (e.g., `warns.go`, `antiflood.go`)
- `<feature>_test.go`: Unit tests for feature
- `<feature>_command_test.go`: Command-specific tests
- `<feature>_<subsystem>_test.go`: Subsystem-specific tests (e.g., `antiraid_miniredis_test.go`)
- `<feature>_testtools.go`: Build-tagged test helpers (go build -tags testtools)

**Directories:**
- `alita/db/<domain>/`: One directory per feature domain (user, warns, filters, etc.)
- `alita/modules/`: One .go file per feature module or logical grouping
- `alita/utils/<concern>/`: Utility packages organized by concern (cache, chat_status, helpers, etc.)

**Functions:**
- `Load<Feature>()`: Module loader function registered in init() (e.g., `LoadWarns()`, `LoadAntiflood()`)
- `<Action><Entity>()`: Command/action handlers (e.g., `adminlist()`, `addWarn()`)
- `Get<Entity><Qualifier>()`: Read functions (e.g., `GetWarnsCount()`, `GetUserBasicInfoCached()`)
- `Ensure<Entity>InDb()`: Upsert functions (e.g., `EnsureUserInDb()`, `EnsureBotInDb()`)
- `<Verb><Entity>()`: Mutation functions (e.g., `UpdateUser()`, `DeleteWarn()`)

**Database:**
- Tables: Lowercase plural (users, chats, chat_filters, warns, etc.) in `migrations/*.sql`
- Models: CamelCase (User, Chat, ChatFilters, Warns) in `alita/db/models/*.go`
- Columns: Lowercase with underscores (user_id, created_at, is_inactive) in SQL

**Telegram Message Keys (i18n):**
- Format: `<module>_<context>_<variant>` (e.g., `admin_adminlist_note_cached`)
- Defined in: `locales/en.yml`, `locales/<lang>.yml`
- Fallback: Missing keys return `""` with error; callers ignore error

## Where to Add New Code

**New Feature (e.g., reputation system):**
1. **Database:**
   - Add migration: `alita/db/migrations/1693999999_create_reputation.sql`
   - Add model: `alita/db/models/reputation.go`
   - Add repository: `alita/db/reputation/repository.go` (with GetFromCacheOrLoad, DeleteCache)
   
2. **Module:**
   - Add handler: `alita/modules/reputation.go` with:
     - `LoadReputation(dispatcher)` function
     - `init()` calling `RegisterLegacyModule("Reputation", <priority>, LoadReputation)`
     - Handler methods with value receiver on `moduleStruct`
   
3. **Localization:**
   - Add keys to all 7 locale files: `locales/{en,id,pt,ro,tr,hi,es}.yml`
   - Key format: `reputation_<context>` (e.g., `reputation_info`, `reputation_added`)
   - Run `make check-translations` to verify
   
4. **Tests:**
   - Co-locate: `alita/modules/reputation_test.go` (unit tests)
   - Add to AutoMigrate: `alita/db/testmain_test.go` (include RepModel)
   - Use fixtures: `internal/testdb.Run()` for SQLite, miniredis for Redis

**New Command (e.g., /rep_info):**
1. Add handler in module: `alita/modules/reputation.go`
2. Register with pipeline:
   ```go
   helpers.WrapCommand(dispatcher, helpers.CommandDescriptor{
     Name: "rep_info",
     Group: 0,
     RequiredChecks: []helpers.CheckFunc{helpers.RequireGroup()},
     Disableable: true,
   }, func(c *helpers.CommandContext) error {
     // Command logic
     return ext.EndGroups
   })
   ```
3. Add locale keys: `reputation_rep_info_*` in all locale files
4. Test: `alita/modules/reputation_command_test.go`

**New Utility:**
1. Determine concern: Is it permission-checking? Error handling? Formatting?
2. Add to existing package if same concern, or create new `alita/utils/<concern>/`
3. Keep functions pure (no side effects); export as needed
4. Add tests: `alita/utils/<concern>/<func>_test.go`

## Special Directories

**`alita/db/migrations/`:**
- Purpose: SQL schema source of truth
- Generated: No (hand-written SQL)
- Committed: Yes
- Key: Filenames sort by timestamp; checksums prevent edits to applied migrations
- Constraint: Each file runs in one transaction; no `CREATE INDEX CONCURRENTLY`

**`locales/`:**
- Purpose: Embedded i18n files
- Generated: No (hand-edited YAML)
- Committed: Yes
- Key: go:embed directive in main.go embeds all files at compile time
- Constraint: Must include all 7 language files + config.yml; missing keys break at runtime

**`alita/db/cache/`:**
- Purpose: Cache layer (distinct from `alita/utils/cache/`)
- Generated: No
- Committed: Yes
- Key: Generation-based invalidation (4096 stripes); local LRU (10s TTL); Redis backend

**`scripts/`:**
- Purpose: Build and validation scripts
- Generated: Some (docs)
- Committed: Yes
- Key: `check_translations`, `migrate_psql.sh` are mandatory CI/CD checks

**`docs/`:**
- Purpose: Generated documentation
- Generated: Yes (by `make generate-docs` from `locales/en.yml`)
- Committed: Yes (except frozen pages with `<!-- MANUALLY MAINTAINED -->`)
- Key: `api-reference/lock-types.md` ignores regeneration marker (always manual)

---

*Structure analysis: 2026-10-04*
