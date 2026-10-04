@../AGENTS.md

<!-- GSD:project-start source:PROJECT.md -->

## Project

**Fuku Robot**

Fuku Robot is a fork of Alita Robot, a Go Telegram group-management bot. It runs only for my own communities, and the goal is to make it work more like GroupHelp. The first step is a **Staff Group**: one Telegram group of trusted admins that manages all my other groups. Any of us can ban, mute or kick someone in every linked group with one command. After that come smarter raid protection, a web-based captcha, and a button settings menu.

**Core Value:** My trusted staff can protect every one of my communities from one place. We act on a bad actor across all groups at once, and the bot never lets anyone act in a group where they aren't an admin.

### Constraints

- **Tech stack:** Go 1.26.0, gotgbot v2, GORM on PostgreSQL (SQLite in tests), Redis. Stay on the existing stack.
- **Repo rules:** follow `AGENTS.md`:
  - Migrations are append-only, with timestamped names and one transaction per file.
  - Every write `cache.DeleteCache`s the keys it affects.
  - Commands go through `helpers.WrapCommand`.
  - Locale keys go in all 7 locale files.
  - Fire-and-forget goroutines recover from panics.
  - Tests use real fixtures (no mock libraries) and run through `make test`.
- **Security:**
  - The per-group admin check is never skipped.
  - Turnstile verification runs server-side.
  - New secrets, such as the Turnstile secret key, are registered with `logredact.RegisterSecret`.
- **Telegram limits:**
  - Fan-out across many groups has to respect Bot API rate limits and report partial failures, not drop them.
  - Callback data is capped at 64 bytes through `callbackcodec`.
- **Deployment:** Docker, with `AUTO_MIGRATE=true` in the deploy manifests.

<!-- GSD:project-end -->

<!-- GSD:stack-start source:codebase/STACK.md -->

## Technology Stack

## Languages

- Go 1.26.0 - Entire codebase, bot engine, migrations, utilities, and tests

## Runtime

- Linux (Docker-based containerized deployment)
- macOS and Windows binary support via goreleaser
- Go Modules (go.mod)
- Lockfile: `go.sum` (present, pinned versions)

## Frameworks

- `github.com/PaulSonOfLars/gotgbot/v2` v2.0.0-rc.36 - Telegram Bot API client and event dispatcher
- `gorm.io/gorm` v1.31.2 - Object-relational mapper for database operations
- `github.com/redis/go-redis/v9` v9.22.0 - Redis client for caching and operational state
- `github.com/eko/gocache/lib/v4` v4.4.0 - Multi-store caching abstraction
- `github.com/eko/gocache/store/redis/v4` v4.2.12 - Redis backend for gocache
- `gorm.io/driver/postgres` v1.6.3 - PostgreSQL dialect for GORM (production)
- `gorm.io/driver/sqlite` v1.6.0 - SQLite dialect for GORM (testing)
- `go.opentelemetry.io/otel` v1.46.0 - OpenTelemetry tracing SDK
- `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc` v1.46.0 - OTLP gRPC exporter
- `go.opentelemetry.io/otel/exporters/stdout/stdouttrace` v1.46.0 - Console exporter (debugging)
- `go.opentelemetry.io/otel/sdk` v1.46.0 - OpenTelemetry SDK
- `github.com/prometheus/client_golang` v1.24.1 - Prometheus metrics client
- `github.com/stretchr/testify` v1.12.1 - Test assertions only (AGENTS.md: no mock libraries; use real fixtures)
- `github.com/alicebob/miniredis/v2` v2.39.0 - In-memory Redis server for tests
- goreleaser v2 (via `.goreleaser.yaml`) - Automated cross-platform binary building and GitHub releases
- golangci-lint v2 (via Makefile) - Multi-linter runner
- `github.com/sirupsen/logrus` v1.10.2 - Structured logging
- `github.com/joho/godotenv` v1.5.1 - Environment variable loading from `.env`
- `github.com/google/uuid` v1.6.0 - UUID generation
- `github.com/vmihailenco/msgpack/v5` v5.4.1 - MessagePack serialization for cache
- `gopkg.in/yaml.v3` v3.0.1 - YAML parsing for locale files
- `golang.org/x/sync` v0.23.0 - Synchronization primitives
- `golang.org/x/text` v0.42.0 - Unicode text handling
- `github.com/mojocn/base64Captcha` v1.3.8 - CAPTCHA generation with base64 encoding and image rendering
- `github.com/cloudflare/ahocorasick` v0.0.0-20240916140611-054963ec9396 - Fast pattern matching for keyword detection
- `github.com/hashicorp/golang-lru/v2` v2.0.7 - LRU cache for in-process memory layer
- `github.com/PaulSonOfLars/gotg_md2html` v0.0.0-20260314092343-61634cbfb443 - Telegram Markdown to HTML conversion

## Key Dependencies

- `gorm.io/gorm` - ORM backbone; all database operations flow through it (`alita/db/*.go`)
- `github.com/PaulSonOfLars/gotgbot/v2` - Telegram Bot API wrapper; enables all bot commands, handlers, and updates (`alita/modules/*.go`)
- `github.com/redis/go-redis/v9` - Production cache layer; required unless `DisableCache=true`. Stores operational state and read-through cache (`alita/utils/cache/`)
- `go.opentelemetry.io/otel/*` v1.46.0 suite - Distributed tracing; exports to OTLP or stdout for observability
- `github.com/prometheus/client_golang` - Metrics collection; exposed on `/metrics` HTTP endpoint
- `github.com/sirupsen/logrus` - Structured logging throughout application

## Configuration

- Loaded via `github.com/joho/godotenv` from `.env` file (in cwd) before startup
- Bot token, database URL, Redis address must be set or startup fails (see `alita/config/config.go`)
- Feature flags (webhooks, AI spam, tracing, monitoring) controlled via environment variables
- `go.mod` and `go.sum` - Dependency version locking
- `.goreleaser.yaml` - Release configuration; builds for darwin/linux/windows with amd64/arm64 targets
- `Makefile` - Task runner; defines `make test`, `make lint`, `make build`, database migration commands
- `golangci-lint` configuration in `.golangci.yml` - Linter rules and exclusions

## Platform Requirements

- Go 1.26.0 with module support
- SQLite (via cgo for `gorm.io/driver/sqlite` when running tests with `-tags testtools`)
- PostgreSQL server for integration tests (optional; uses SQLite if `ALITA_TEST_DATABASE` is unset)
- Redis server or miniredis (in-memory substitute for testing)
- make, bash for Makefile targets
- Docker 20.10+ with Docker Compose 1.29+
- PostgreSQL 12+ (container or external)
- Redis 7+ (container or external; defaults to db 1)
- Linux x86_64 or ARM64 host
- ~1 GB memory allocation (container limit), 256 MB reserved
- GitHub Actions (workflows in `.github/workflows/`)
- Gosec for security scanning
- govulncheck for vulnerability detection
- golangci-lint for code quality

<!-- GSD:stack-end -->

<!-- GSD:conventions-start source:CONVENTIONS.md -->

## Conventions

## Naming Patterns

- `*_test.go`: Test files (conventional Go test naming)
- `testmain_test.go`: Package initialization for tests with database setup
- `*_pure_test.go`: Pure/isolated logic tests (no database dependencies)
- `*.go`: Standard package files using lowercase with underscores
- **Exported (public):** PascalCase (e.g., `WarnUser`, `GetWarnSetting`, `EnsureChatInDb`)
- **Unexported (private):** camelCase (e.g., `checkWarnSettings`, `checkWarns`)
- **Receiver methods:** Use value receivers on `moduleStruct`; follow `(m moduleStruct) methodName(...)` pattern
- **Test functions:** `Test<FunctionName>` with descriptive subtypes (e.g., `TestWarnUserCreatesMissingParentRows`)
- **Helper functions:** Private with clear purpose prefix (e.g., `skipIfNoDb`, `withChatSQLite`, `cleanupBackupChat`)
- **Private:** camelCase (e.g., `numWarns`, `chatID`, `userID`)
- **Exported:** PascalCase (e.g., `ChatId`, `UserId`, `WarnLimit`)
- **Constants:** PascalCase or ALL_CAPS depending on usage context
- **Loop indices:** Single letters (e.g., `i`, `j`) in simple loops; meaningful names in range loops
- **Structs:** PascalCase (e.g., `WarnSettings`, `Warns`, `User`, `Chat`)
- **Interfaces:** PascalCase (e.g., `Repository`, `Handler`)
- **Receiver variable names:** Short abbreviations (e.g., `m` for module, `c` for context)

## Code Style

- Tool: `gofmt` (configured in pre-commit hooks)
- Automatic on commit via pre-commit hook: `go fmt`
- Max line length: Not explicitly enforced; follow reasonable Go conventions (~120 chars)
- Indentation: Tabs (Go standard)
- Tool: `golangci-lint` v2 (configured in `.golangci.yml`)
- Run: `make lint` or via pre-commit hooks
- Enabled linters:
- Exclusions: Test files, migrations, and generated docs exclude gocyclo/dupl checks
- Policy: `new: true` — only fails on new issues to allow gradual improvement
- No path aliases used in this codebase; full import paths are standard practice

## Error Handling

- Location: `alita/utils/errors/errors.go`
- Use `errors.Wrap(err, message)` to wrap errors with context (file, line, function)
- Use `errors.Wrapf(err, format, args...)` for formatted error messages
- Pattern: Return `nil` if err is nil; always wrap before returning
- Example:
- Never discard DB errors on state-changing paths (critical rule from AGENTS.md)
- Use `gorm.ErrRecordNotFound` to check for missing records
- Log errors with context: `log.Errorf("[Category][Function]: %v", err)`
- Return sentinel errors (e.g., `ext.EndGroups`, `ext.ContinueGroups`) from handlers
- Built-in `error` interface for most cases
- Sentinel errors (e.g., `gotgbot.TelegramError`, `gorm.ErrRecordNotFound`)
- Custom errors implement `Error()` string method

## Logging

- Categorized messages: `log.Infof("[Category][Subcategory]: message")`
- Categories observed: `[Database]`, `[ActionLog]`, `[Shutdown]`, `[Media]`
- Levels used:
- Never log errors from control-flow mechanisms (e.g., `ext.EndGroups`) as errors
- Register secrets with `logredact.RegisterSecret(secret)` (≥6 chars) to prevent logging
- Example:

## Comments

- Comment exported functions/types: Required GoDoc style
- Comment non-obvious logic: Complex algorithms, workarounds, important invariants
- Skip obvious comments: Don't comment `count++` or `if x != nil`
- Mark frozen content: `<!-- MANUALLY MAINTAINED: do not regenerate -->` in docs
- Start with the function/type name: "Comment describes what the function does"
- Single-line comments above exported identifiers
- Multi-line comments for complex behavior
- Example:

## Function Design

- Use context.Context as first parameter for database-accessing functions
- Combine related parameters into structs for clarity (e.g., `*helpers.CommandContext`)
- Avoid more than 4-5 parameters; use struct receivers for methods
- Example:
- Use explicit types (not bare returns after named parameters)
- Error as last return value in multi-return functions
- Nil checks: `if errors.Is(err, gorm.ErrRecordNotFound)` not equality
- Return sentinel values from handlers: `ext.EndGroups`, `ext.ContinueGroups`, `nil`
- Example:

## Module Design

- Public functions use PascalCase
- Unexported helpers use camelCase
- Avoid public global variables; use functions to access singletons
- Example: `db.DB` is public singleton; `GetWarnSetting` is exported function
- No barrel/index files pattern used; each package imports specifically needed files
- Domain-focused packages: `alita/db/<domain>/` for data access
- Module packages: `alita/modules/<feature>.go` for feature implementations
- Utility packages: `alita/utils/<concern>/` for cross-cutting functionality
- Config packages: `alita/config/` for environment and startup configuration

## Receiver Functions

- **Always use value receivers** on `moduleStruct` (not pointer receivers)
- Pattern:

## Database Conventions

- **Repository pattern:** Read via `cache.GetFromCacheOrLoad`, write with `cache.DeleteCache` to invalidate
- **Models:** Define `TableName()` method returning the migration table name
- **Queries:** Use GORM's Where/First/Create patterns
- **Caching:** Two-layer: in-process (`CACHE_LOCAL_TTL` default 10s) + Redis
- **Migrations:** SQL files in `migrations/` are schema source of truth; never edit applied files

<!-- GSD:conventions-end -->

<!-- GSD:architecture-start source:ARCHITECTURE.md -->

## Architecture

## System Overview

```text

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

- **Priority-based execution:** Handlers run in numeric order (-10 to 11); first matching handler can halt further processing
- **Module lazy-registration:** Each module registers itself via `init()` at module load time; duplicates are dropped
- **Request-scoped context:** `CommandContext` wraps bot, update, chat, message, user, and translator for each command
- **Two-tier caching:** Local LRU (10s TTL) + Redis (configurable TTL) with generation-based invalidation
- **Repository pattern:** Domain-organized repositories with GORM for reads/writes; cache.DeleteCache must be called on writes
- **Graceful shutdown:** LIFO handler drain (users writes → monitoring → tracer → anti-raid → captcha → AI spam)

## Layers

- Purpose: Process Telegram updates; dispatch to priority-ordered modules
- Location: `alita/modules/`, `main.go` (lines 105-302)
- Contains: Handler registration, command wrappers, permission checks, callback routing
- Depends on: gotgbot/v2 dispatcher, i18n, chat_status utilities, database repos
- Used by: Main dispatcher loop in polling or webhook mode
- Purpose: Build CommandContext, check permissions, invoke handlers
- Location: `alita/utils/helpers/command_pipeline.go`
- Contains: CommandDescriptor, CheckFunc (permission predicates), WrapCommand wrapper
- Depends on: chat_status, i18n, extraction utilities, CommandContext builder
- Used by: Individual module command handlers
- Purpose: Predicate checks for admin status, group-only, disabled commands, approvals
- Location: `alita/utils/chat_status/`
- Contains: IsUserAdmin, IsGroupAdmin, RequireGroup, CheckDisabledCmd (return bools, never reply)
- Depends on: Cache layer (admin list caching), chat/user models
- Used by: Command pipeline, handlers via predicates or PermissionResponder
- Purpose: Localize messages across 7 languages (en, id, pt, ro, tr, hi, es)
- Location: `alita/i18n/`, `locales/` (embedded), `alita/db/lang/`
- Contains: Translator (stateful per language), cache of parsed YAML, fallback to English
- Depends on: Embedded locale files, user language preference from DB
- Used by: All handlers calling tr.GetString(key)
- Purpose: Model definitions and repository pattern for domain-specific data access
- Location: `alita/db/models/`, `alita/db/<domain>/` (30+ domain directories)
- Contains: GORM models (User, Chat, ChatFilters, etc.), repository functions (Create, Update, Delete, Get)
- Depends on: PostgreSQL connection, GORM ORM, cache layer for reads
- Used by: Handlers, cached loaders, upsert paths
- Purpose: Reduce database load via local LRU (10s) + Redis (configurable)
- Location: `alita/db/cache/`, `alita/utils/cache/`
- Contains: GetFromCacheOrLoad (with concurrent-load dedup), DeleteCache, generation-based invalidation
- Depends on: Redis client, msgpack serialization, local LRU expirable map
- Used by: Repository read functions; generation counter bumped on DeleteCache to prevent stale writes
- Purpose: Health checks, metrics export, webhook reception (alternative to polling)
- Location: `alita/utils/httpserver/`, `alita/utils/monitoring/`
- Contains: HTTP routes (/health, /metrics, /webhook), stats collection, error callbacks, activity tracking
- Depends on: Main dispatcher, shutdown manager, tracer provider
- Used by: Docker healthcheck, Prometheus scraping, Telegram webhook polling

## Data Flow

### Primary Request Path (Command)

### Antiraid Flow (Real-Time Join Monitoring)

### Cache Invalidation Flow

### State Management

- Module singletons: `defaultHelpRegistry` (read-write under `ableMapMu`)
- Antiflood counters: Per-replica in-memory (no Redis)
- Admin cache: Refreshed per-command (memoized within update)
- Federation ban lookups: `alita:cache:fban:<fed>:<user>`
- Anti-raid join counters: `alita:antiraid:*` (NOT under cache prefix; survives `CLEAR_CACHE_ON_STARTUP`)
- Anonymous admin flag: `alita:anonAdmin:*`
- Captcha pending flag: `alita:cache:captcha_pending:<chat>` (MUST invalidate after insert)
- Users, chats, filters, warns, bans, notes, locks, settings, federations
- Schema migrations tracked in `schema_migrations` with SHA-256 checksum

## Key Abstractions

- Purpose: Encapsulates a bot feature (antiflood, filters, warns, etc.)
- Location: `alita/modules/<feature>.go`
- Pattern: Exported Load function + receiver methods (value receivers) + init() registration
- Example: `adminModule.adminlist(c *helpers.CommandContext) error` → returns `ext.EndGroups` on success or error
- Purpose: Metadata for command registration (name, aliases, group, checks, disableable flag)
- Location: `alita/utils/helpers/command_pipeline.go`
- Used by: `WrapCommand` to register with dispatcher and disabling system
- Purpose: Permission predicate returning bool (never replies)
- Signature: `func(c *CommandContext) bool`
- Examples: `IsUserAdmin`, `RequireGroup`, `CheckDisabled`
- Pattern: Return false to halt command; handler pipeline sends error reply via `PermissionResponder`
- Purpose: Request-scoped bundling of bot, update, chat, message, user, translator
- Location: `alita/utils/helpers/command_pipeline.go`
- Passed to: All command handlers and check functions
- Lifetime: Built per-command, discarded after response
- Purpose: Domain-specific data access (e.g., user.EnsureUserInDb, warns.GetWarnsCount)
- Location: `alita/db/<domain>/repository.go` (30+ domains)
- Pattern: Public functions using GORM; internal use of `cache.GetFromCacheOrLoad` and `cache.DeleteCache`
- Example: `func GetUserBasicInfoCached(userId int64) (*models.User, error)` wraps GORM in cache

## Entry Points

- Location: `/home/user/Fuku_Robot/main.go`
- Triggers: Binary execution or Docker entrypoint
- Responsibilities:
- Location: `alita/utils/httpserver/webhook.go`
- Triggers: Telegram sends POST to `/telegram/webhook/<token>`
- Responsibilities:
- Alternative: Polling updater (simpler, no public endpoint needed)
- `--version`: Print bot version, skip DB init, exit
- `--health`: Check `/health` endpoint, exit
- Normal: Start dispatcher loop

## Architectural Constraints

- **Threading:** Concurrent: the gotgbot dispatcher runs up to `DISPATCHER_MAX_ROUTINES` (default 200) updates in parallel goroutines (`main.go`, `alita/config/config.go`), so handlers must be race-safe; async writes to users table join WaitGroup drained by `modules.DrainUsersAsyncWrites` on shutdown
- **Global state:** 
- **Circular imports:** Import hierarchy: stdlib → third-party (gotgbot, gorm, logrus) → internal (`alita/db` → `alita/modules` → `alita/utils`)
- **Database initialization order:**
- **Shutdown order (LIFO within 60s):**

## Anti-Patterns

### Discarding DB Errors on State-Changing Paths

### Fire-and-Forget Goroutines Without Panic Recovery

### Typo'd i18n Keys

### Unguarded i18n Fallback to Empty String

### Mocking in Tests Instead of Using Real Fixtures

### Updating Record Without Checking Zero Values

<!-- GSD:architecture-end -->

<!-- GSD:skills-start source:skills/ -->

## Project Skills

No project skills found. Add skills to any of: `.claude/skills/`, `.agents/skills/`, `.cursor/skills/`, `.github/skills/`, or `.codex/skills/` with a `SKILL.md` index file.
<!-- GSD:skills-end -->

<!-- GSD:workflow-start source:GSD defaults -->

## GSD Workflow Enforcement

Before using Edit, Write, or other file-changing tools, start work through a GSD command so planning artifacts and execution context stay in sync.

Use these entry points:
- `/gsd-quick` for small fixes, doc updates, and ad-hoc tasks
- `/gsd-debug` for investigation and bug fixing
- `/gsd-execute-phase` for planned phase work

Do not make direct repo edits outside a GSD workflow unless the user explicitly asks to bypass it.
<!-- GSD:workflow-end -->

<!-- GSD:profile-start -->

## Developer Profile

> Profile not yet configured. Run `/gsd-profile-user` to generate your developer profile.
> This section is managed by `generate-claude-profile` -- do not edit manually.
<!-- GSD:profile-end -->
