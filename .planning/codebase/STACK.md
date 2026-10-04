---
last_mapped_commit: 4d3518f822a0014f925b59b6748acaee47241152
last_mapped_at: 2026-10-04
---
# Technology Stack

**Analysis Date:** 2026-10-04

## Languages

**Primary:**
- Go 1.26.0 - Entire codebase, bot engine, migrations, utilities, and tests

## Runtime

**Environment:**
- Linux (Docker-based containerized deployment)
- macOS and Windows binary support via goreleaser

**Package Manager:**
- Go Modules (go.mod)
- Lockfile: `go.sum` (present, pinned versions)

## Frameworks

**Core:**
- `github.com/PaulSonOfLars/gotgbot/v2` v2.0.0-rc.36 - Telegram Bot API client and event dispatcher
- `gorm.io/gorm` v1.31.2 - Object-relational mapper for database operations

**Messaging/API:**
- `github.com/redis/go-redis/v9` v9.22.0 - Redis client for caching and operational state
- `github.com/eko/gocache/lib/v4` v4.4.0 - Multi-store caching abstraction
- `github.com/eko/gocache/store/redis/v4` v4.2.12 - Redis backend for gocache

**Database:**
- `gorm.io/driver/postgres` v1.6.3 - PostgreSQL dialect for GORM (production)
- `gorm.io/driver/sqlite` v1.6.0 - SQLite dialect for GORM (testing)

**Observability & Monitoring:**
- `go.opentelemetry.io/otel` v1.46.0 - OpenTelemetry tracing SDK
- `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc` v1.46.0 - OTLP gRPC exporter
- `go.opentelemetry.io/otel/exporters/stdout/stdouttrace` v1.46.0 - Console exporter (debugging)
- `go.opentelemetry.io/otel/sdk` v1.46.0 - OpenTelemetry SDK
- `github.com/prometheus/client_golang` v1.24.1 - Prometheus metrics client

**Testing & Fixtures:**
- `github.com/stretchr/testify` v1.12.1 - Test assertions and mocking
- `github.com/alicebob/miniredis/v2` v2.39.0 - In-memory Redis server for tests

**Build & Release:**
- goreleaser v2 (via `.goreleaser.yaml`) - Automated cross-platform binary building and GitHub releases
- golangci-lint v2 (via Makefile) - Multi-linter runner

**Utilities:**
- `github.com/sirupsen/logrus` v1.10.2 - Structured logging
- `github.com/joho/godotenv` v1.5.1 - Environment variable loading from `.env`
- `github.com/google/uuid` v1.6.0 - UUID generation
- `github.com/vmihailenco/msgpack/v5` v5.4.1 - MessagePack serialization for cache
- `gopkg.in/yaml.v3` v3.0.1 - YAML parsing for locale files
- `golang.org/x/sync` v0.23.0 - Synchronization primitives
- `golang.org/x/text` v0.42.0 - Unicode text handling

**Performance & Image Processing:**
- `github.com/mojocn/base64Captcha` v1.3.8 - CAPTCHA generation with base64 encoding and image rendering
- `github.com/cloudflare/ahocorasick` v0.0.0-20240916140611-054963ec9396 - Fast pattern matching for keyword detection
- `github.com/hashicorp/golang-lru/v2` v2.0.7 - LRU cache for in-process memory layer

**Documentation & Markdown:**
- `github.com/PaulSonOfLars/gotg_md2html` v0.0.0-20260314092343-61634cbfb443 - Telegram Markdown to HTML conversion

## Key Dependencies

**Critical:**
- `gorm.io/gorm` - ORM backbone; all database operations flow through it (`alita/db/*.go`)
- `github.com/PaulSonOfLars/gotgbot/v2` - Telegram Bot API wrapper; enables all bot commands, handlers, and updates (`alita/modules/*.go`)
- `github.com/redis/go-redis/v9` - Production cache layer; required unless `DisableCache=true`. Stores operational state and read-through cache (`alita/utils/cache/`)

**Infrastructure:**
- `go.opentelemetry.io/otel/*` v1.46.0 suite - Distributed tracing; exports to OTLP or stdout for observability
- `github.com/prometheus/client_golang` - Metrics collection; exposed on `/metrics` HTTP endpoint
- `github.com/sirupsen/logrus` - Structured logging throughout application

## Configuration

**Environment:**
- Loaded via `github.com/joho/godotenv` from `.env` file (in cwd) before startup
- Bot token, database URL, Redis address must be set or startup fails (see `alita/config/config.go`)
- Feature flags (webhooks, AI spam, tracing, monitoring) controlled via environment variables

**Build:**
- `go.mod` and `go.sum` - Dependency version locking
- `.goreleaser.yaml` - Release configuration; builds for darwin/linux/windows with amd64/arm64 targets
- `Makefile` - Task runner; defines `make test`, `make lint`, `make build`, database migration commands
- `golangci-lint` configuration in `.golangci.yml` - Linter rules and exclusions

## Platform Requirements

**Development:**
- Go 1.26.0 with module support
- SQLite (via cgo for `gorm.io/driver/sqlite` when running tests with `-tags testtools`)
- PostgreSQL server for integration tests (optional; uses SQLite if `ALITA_TEST_DATABASE` is unset)
- Redis server or miniredis (in-memory substitute for testing)
- make, bash for Makefile targets

**Production:**
- Docker 20.10+ with Docker Compose 1.29+
- PostgreSQL 12+ (container or external)
- Redis 7+ (container or external; defaults to db 1)
- Linux x86_64 or ARM64 host
- ~1 GB memory allocation (container limit), 256 MB reserved

**CI/CD:**
- GitHub Actions (workflows in `.github/workflows/`)
- Gosec for security scanning
- govulncheck for vulnerability detection
- golangci-lint for code quality

---

*Stack analysis: 2026-10-04*
