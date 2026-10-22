---
last_mapped_commit: 4d3518f822a0014f925b59b6748acaee47241152
last_mapped_at: 2026-10-04
---
# External Integrations

**Analysis Date:** 2026-10-04

## APIs & External Services

**Telegram Bot Platform:**
- Telegram Bot API - The primary integration; bot receives updates, sends messages, manages groups
  - SDK/Client: `github.com/PaulSonOfLars/gotgbot/v2` (v2.0.0-rc.36)
  - Auth: `BOT_TOKEN` environment variable
  - Endpoints: Standard Telegram Bot API endpoints or custom `API_SERVER` (for self-hosted telegram-bot-api)
  - Implementation: Long-polling or webhook mode (configurable via `USE_WEBHOOKS`)

**AI Spam Detection (Optional):**
- TypeSafe Jev - Per-chat AI-powered spam filter with calibrated probability decisions
  - Service: https://typesafe.ai (external SaaS)
  - SDK/Client: Custom HTTP client (`alita/modules/aispam_jev.go` via standard library `net/http`)
  - Auth: `TYPESAFE_API_KEY` environment variable (single bot-wide credential)
  - Endpoint: Configured via `OTEL_EXPORTER_OTLP_ENDPOINT` logic or hardcoded (production endpoint)
  - Feature gate: `ENABLE_AISPAM` (default: true) and `TYPESAFE_API_KEY` (default: empty, feature inert without it)
  - Implementation: Submits recent message summaries, receives probability-based verdicts

**Distributed Tracing (Optional):**
- OpenTelemetry Collector - Trace export for performance monitoring and debugging
  - Protocol: OTLP gRPC (via `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc`)
  - Endpoint: `OTEL_EXPORTER_OTLP_ENDPOINT` (e.g., `localhost:4317`)
  - Features: Includes DB operations, request handlers, custom spans
  - Fallback: Console exporter if `OTEL_EXPORTER_CONSOLE=true`
  - Configuration: TLS disabled if `OTEL_EXPORTER_OTLP_INSECURE=true`
  - Service name: `OTEL_SERVICE_NAME` (default: "alita_robot")
  - Sampling: `OTEL_TRACES_SAMPLE_RATE` (0.0-1.0, default: 1.0)

## Data Storage

**Databases:**
- PostgreSQL 12+ (production)
  - Connection: `DATABASE_URL` (required, format: `postgres://user:pass@host:port/db?sslmode=...`)
  - Client: `gorm.io/driver/postgres` via GORM
  - Schema: Applied via SQL migrations in `migrations/*.sql` (run on startup if `AUTO_MIGRATE=true`)
  - Connection pooling: `DB_MAX_IDLE_CONNS`, `DB_MAX_OPEN_CONNS`, `DB_CONN_MAX_LIFETIME_MIN`, `DB_CONN_MAX_IDLE_TIME_MIN`

- SQLite (testing only)
  - Location: In-memory or `gorm.io/driver/sqlite`
  - Used when `ALITA_TEST_DATABASE` is not set or false in test runs
  - Requires CGO in test builds

**File Storage:**
- Telegram Bot API file storage - All media (photos, documents, backups) are stored on Telegram servers
  - Access: Via `backupDownloadBaseURL` (`https://api.telegram.org/file/bot<token>/<path>`)
  - Implementation: `alita/modules/backup.go` downloads federation backups using this endpoint

**Caching:**
- Redis 7+ (required for production deployments unless `DisableCache=true`)
  - Connection: `REDIS_ADDRESS` (host:port) or `REDIS_URL` (auto-parsed)
  - Password: `REDIS_PASSWORD` (optional)
  - Database: `REDIS_DB` (default: 1, range 0-15)
  - Client: `github.com/redis/go-redis/v9`
  - Architecture: Two-layer read-through caching
    - L1: In-process LRU cache with `CACHE_LOCAL_TTL` (default 10s, max 300s)
    - L2: Redis backend for distributed invalidation
  - Key prefix: `alita:cache:*` for cacheable data; `alita:<domain>:*` for operational state
  - Max entries (L1): `CACHE_LOCAL_MAX_ENTRIES` (default: 50000)
  - Clear on startup: `CLEAR_CACHE_ON_STARTUP` (default: false)
  - In-memory substitute: `github.com/alicebob/miniredis/v2` for testing

## Authentication & Identity

**Auth Provider:**
- Telegram platform native - Bot token issued by @BotFather
  - Implementation: Custom; token passed to gotgbot, validates with Telegram servers
  - User identity: Telegram user IDs and chat IDs (numeric)
  - Admin roles: Managed in-database; fetched from Telegram API via `getChatAdministrators`
  - Anonymous admin support: Special header-based auth for channels and certain setups

**Bot Credentials:**
- `BOT_TOKEN` (required) - Used for all Telegram API calls
- `OWNER_ID` (required) - Telegram user ID of bot owner
- `MESSAGE_DUMP` (required) - Telegram chat ID for backup/logging channel

**Metrics Endpoint Auth (Optional):**
- Bearer token auth for `/metrics` and `/db_metrics` endpoints
  - Env: `METRICS_AUTH_TOKEN` (optional)
  - Constant-time comparison to prevent timing attacks
  - No auth required if unset (warning logged)

## Monitoring & Observability

**Error Tracking:**
- Not detected - Errors are logged locally via logrus

**Logs:**
- Logrus structured logging (`github.com/sirupsen/logrus`)
- Output: Stdout (parsed by Docker/container logs)
- Redaction: Secrets (6+ chars) registered with `logredact.RegisterSecret()` before logging
- Levels: Info, Warn, Error based on severity
- Format: Unstructured text (configurable to JSON via hook)

**Metrics:**
- Prometheus client (`github.com/prometheus/client_golang`)
  - Endpoint: `/metrics` (HTTP port configurable via `HTTP_PORT`)
  - Format: Prometheus text format
  - Auth: Optional bearer token via `METRICS_AUTH_TOKEN`
  - Includes: Request counts, handler latencies, cache hits/misses, database pool stats
  - DB pool monitoring: Enabled via `ENABLE_DB_MONITORING=true`

**Tracing:**
- OpenTelemetry distributed tracing (optional)
  - Exporters:
    - OTLP gRPC: Exports to `OTEL_EXPORTER_OTLP_ENDPOINT` (e.g., Datadog, Jaeger, Grafana Tempo)
    - Stdout: Console output if `OTEL_EXPORTER_CONSOLE=true`
  - Spans: Generated for DB operations, HTTP requests, module processing
  - Initialization: `alita/utils/tracing/tracing.go`

**Performance Monitoring:**
- Flag: `ENABLE_PERFORMANCE_MONITORING` (default: true in production, false in debug)
- Flag: `ENABLE_BACKGROUND_STATS` (default: true in production, false in debug)
- Profiling (development only):
  - pprof endpoints: `/debug/pprof/*` if `ENABLE_PPROF=true`
  - WARNING: Never enable in production (exposes runtime internals)

## CI/CD & Deployment

**Hosting:**
- Self-hosted: Linux server running Docker containers or standalone binary
- Dokploy-compatible: Predefined Docker Compose stack; environment variables set via UI
- Containerization: Multi-stage Dockerfile (see `docker/` directory) with distroless images for production
- Image registry: GHCR (`ghcr.io/divkix/alita_robot:latest`)

**CI Pipeline:**
- GitHub Actions (`.github/workflows/`)
  - Triggers: Push to main, PRs, manual dispatch
  - Security: Gosec + govulncheck scanning, SARIF upload to GitHub Security tab
  - Quality: golangci-lint checks (v2)
  - Testing: Full suite with race detection and coverage
  - Cross-platform: Builds for darwin/linux/windows, amd64/arm64

**Release Pipeline:**
- goreleaser v2 (`.goreleaser.yaml`)
  - Builds: CGO_ENABLED=0 for distroless Linux binaries
  - Artifacts: GitHub releases with archives for each platform
  - Docker: Pushes to GHCR (requires GHCR token in CI)
  - Changelog: Auto-generated from git history (Conventional Commits)

## Environment Configuration

**Required env vars:**
- `BOT_TOKEN` - Telegram bot token from @BotFather
- `OWNER_ID` - Bot owner's Telegram user ID (numeric)
- `MESSAGE_DUMP` - Log channel ID for backups and error messages (numeric)
- `DATABASE_URL` - PostgreSQL connection string
- `REDIS_ADDRESS` or `REDIS_URL` - Redis server address (unless `DisableCache=true`)

**Optional env vars (feature gates):**
- `USE_WEBHOOKS` - Enable webhook mode instead of polling
- `WEBHOOK_DOMAIN` - Domain for webhook URL (required if `USE_WEBHOOKS=true`)
- `WEBHOOK_SECRET` - Webhook signature secret (required if `USE_WEBHOOKS=true`)
- `API_SERVER` - Custom Telegram Bot API endpoint (for self-hosted)
- `TYPESAFE_API_KEY` - TypeSafe Jev API key (enables AI spam filter)
- `ENABLE_AISPAM` - Global kill switch for AI spam filter
- `OTEL_EXPORTER_OTLP_ENDPOINT` - OpenTelemetry collector endpoint
- `OTEL_EXPORTER_CONSOLE` - Output traces to console (debug)
- `ENABLE_PERFORMANCE_MONITORING` - Enable performance tracking
- `ENABLE_PPROF` - Enable pprof debug endpoints (dev only)
- `METRICS_AUTH_TOKEN` - Bearer token for `/metrics` endpoint

**Secrets location:**
- `.env` file in working directory (auto-loaded, never committed)
- Environment variables (from container runtime, secrets manager, etc.)
- GitHub Secrets (for CI/CD)
- Dokploy UI environment variable configuration

## Webhooks & Callbacks

**Incoming:**
- Telegram webhook (if `USE_WEBHOOKS=true`)
  - URL: `<WEBHOOK_DOMAIN>/webhook/` (path matches gotgbot routing)
  - Method: POST
  - Signature: Secret verified via `WEBHOOK_SECRET` header
  - Payload: JSON update object from Telegram API

- Internal HTTP endpoints:
  - `/health` - Health check for Docker healthcheck probes
  - `/metrics` - Prometheus metrics scraping (optional auth via `METRICS_AUTH_TOKEN`)
  - `/db_metrics` - Database pool metrics (optional auth)
  - `/debug/pprof/*` - pprof endpoints (if `ENABLE_PPROF=true`, dev only)

**Outgoing:**
- Telegram Bot API - All bot actions (send message, edit, delete, etc.)
- TypeSafe Jev API - AI spam decisions (if enabled)
- OpenTelemetry Collector - Trace export (if configured)
- Telegram file downloads - Federation backups and media access

---

*Integration audit: 2026-10-04*
