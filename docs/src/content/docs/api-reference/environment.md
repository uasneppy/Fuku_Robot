---
title: Environment Variables
description: Configuration reference for all environment variables
---
<!-- MANUALLY MAINTAINED: do not regenerate -->

# ⚙️ Environment Variables

This page documents all environment variables used to configure Alita Robot.

## 📂 Activity monitoring configuration

### `ACTIVITY_CHECK_INTERVAL`

Hours between activity checks

| Property | Value |
|----------|-------|
| **Type** | `integer` |
| **Required** | No |
| **Default** | `1` |
| **Validation** | min=1,max=24 |

### `ENABLE_AUTO_CLEANUP`

Whether to automatically mark inactive chats

| Property | Value |
|----------|-------|
| **Type** | `boolean` |
| **Required** | No |
| **Default** | `true` |

### `INACTIVITY_THRESHOLD_DAYS`

Days before marking a chat as inactive

| Property | Value |
|----------|-------|
| **Type** | `integer` |
| **Required** | No |
| **Default** | `30` |
| **Validation** | min=1,max=365 |

## 📂 AI spam filter configuration

### `ENABLE_AISPAM`

Global kill switch for every AI check. When `false`, no chat runs `/aispam`
checks regardless of its per-chat setting, and the AI raid trigger sends nothing
to either provider: no text goes to TypeSafe and no image goes to Anthropic,
even when `ANTHROPIC_API_KEY` is set. `/autolockdown` then shows the "AI checks are
not set up" notice. The text checks are inert without `TYPESAFE_API_KEY` either
way.

| Property | Value |
|----------|-------|
| **Type** | `boolean` |
| **Required** | No |
| **Default** | `true` |

### `TYPESAFE_API_KEY`

TypeSafe API key used by every AI spam check. One credential for the whole bot;
chats cannot bring their own. Leaving it empty disables the feature and makes
`/aispam on` refuse.

| Property | Value |
|----------|-------|
| **Type** | `string` |
| **Required** | No |

### `ANTHROPIC_API_KEY`

Anthropic API key for the image checks of the AI raid trigger. When it is set,
suspicious photos, GIFs and stickers posted by members who joined in the last 24
hours are sent to Anthropic to be judged as spam or not. A burst of spam
verdicts from several accounts can lock the group; the AI never locks a group on
its own. When it is unset, no image is sent anywhere and the text checks
(through `TYPESAFE_API_KEY`) are unchanged. Setting `ENABLE_AISPAM` to `false`
also stops the image checks.

Every check sends that image to Anthropic under the terms of the owner's API
account, so the owner decides before setting the key. The key is a secret: it is
registered with the log redactor at startup, and any string shaped like an
Anthropic key (`sk-ant-...`) is scrubbed from log lines even when it was never
registered. It is sent only in the `x-api-key` request header, never in a URL.
The bot reads this standard name, so a key exported for another tool on the same
host turns the checks on.

| Property | Value |
|----------|-------|
| **Type** | `string` |
| **Required** | No |

### `ANTHROPIC_MODEL`

Claude model used for the image checks. Only used when `ANTHROPIC_API_KEY` is
set. The name must be a plain model id of 1 to 64 letters, digits, dots,
underscores or hyphens, starting with a letter or digit. Any other value is
replaced by the default with a warning in the log, so a bad name never reaches a
request URL.

| Property | Value |
|----------|-------|
| **Type** | `string` |
| **Required** | No |
| **Default** | `claude-haiku-5-5` |

## 📂 Web captcha configuration

The web captcha verifies new members with Cloudflare Turnstile inside a Telegram
Mini App. It is on only when all four of `CAPTCHA_WEB_BASE_URL`,
`TELEGRAM_MINI_APP_SHORT_NAME`, `TURNSTILE_SITE_KEY` and `TURNSTILE_SECRET_KEY`
are set. With none of them set the bot starts as before and the web captcha is
off. With some but not all of them set, the bot refuses to start and the error
names every missing variable; it never prints a value. No variable has a
default.

### `CAPTCHA_WEB_BASE_URL`

Public address of the bot's HTTP port, as reachable from the internet behind
your reverse proxy. It must be an `https` URL with a host, no user info, no
query, no fragment, and a path that is empty or `/`. The captcha page is served
at `<base>/captcha`, and the Turnstile answer is checked against this host.

| Property | Value |
|----------|-------|
| **Type** | `string` (https URL) |
| **Required** | No (all four together) |

### `TELEGRAM_MINI_APP_SHORT_NAME`

Short name of the Mini App you created with BotFather `/newapp`. It must be 1 to
64 letters, digits or underscores.

| Property | Value |
|----------|-------|
| **Type** | `string` |
| **Required** | No (all four together) |

### `TURNSTILE_SITE_KEY`

Site key of your Cloudflare Turnstile widget. It is public, because the browser
needs it to show the widget, so it is not scrubbed from logs.

| Property | Value |
|----------|-------|
| **Type** | `string` |
| **Required** | No (all four together) |

### `TURNSTILE_SECRET_KEY`

Secret key of your Cloudflare Turnstile widget. The bot sends it only to
Cloudflare's `siteverify` endpoint, from the server. It must be at least 6
characters so the log redactor can scrub it. It is registered with the log
redactor at startup, and its exact value is scrubbed from every log line.

| Property | Value |
|----------|-------|
| **Type** | `string` |
| **Required** | No (all four together) |

### `CAPTCHA_WEB_TRUSTED_PROXIES`

Comma-separated list of IP addresses or CIDR prefixes (for example
`10.0.0.0/8, 192.168.1.10, ::1`) of the reverse proxies whose `X-Forwarded-For`
header the verification route may trust. When it is empty the header is ignored.
Setting it without the four variables above stops startup, because a proxy list
alone is a misconfiguration.

| Property | Value |
|----------|-------|
| **Type** | `string` (comma-separated IPs or CIDRs) |
| **Required** | No |

## 📂 Bot settings

### `MESSAGE_DUMP` (Required)

| Property | Value |
|----------|-------|
| **Type** | `integer` |
| **Required** | Yes |
| **Validation** | required,min=1 |

### `OWNER_ID` (Required)

| Property | Value |
|----------|-------|
| **Type** | `integer` |
| **Required** | Yes |
| **Validation** | required,min=1 |

### `DROP_PENDING_UPDATES`

| Property | Value |
|----------|-------|
| **Type** | `boolean` |
| **Required** | No |
| **Default** | `false` |

### `ENABLED_LOCALES`

Comma-separated list of language codes shown in the language picker (e.g.,
`en,es,fr,hi`). All embedded locale files are loaded regardless of this setting.

| Property | Value |
|----------|-------|
| **Type** | `string[]` |
| **Required** | No |
| **Default** | `en` |

## 📂 Core configuration

### `BOT_TOKEN` (Required)

| Property | Value |
|----------|-------|
| **Type** | `string` |
| **Required** | Yes |
| **Validation** | required |

### `API_SERVER`

Custom Telegram Bot API server URL. Used with local telegram-bot-api server.

| Property | Value |
|----------|-------|
| **Type** | `string` |
| **Required** | No |
| **Default** | `https://api.telegram.org` |

### `DEBUG`

Enables debug logging and disables automatic performance monitoring.

| Property | Value |
|----------|-------|
| **Type** | `boolean` |
| **Required** | No |
| **Default** | `false` |

## 📂 Database configuration

### `DATABASE_URL` (Required)

| Property | Value |
|----------|-------|
| **Type** | `string` |
| **Required** | Yes |
| **Validation** | required |

## 📂 Database connection pool configuration

### `DB_CONN_MAX_IDLE_TIME_MIN`

Max idle time in minutes

| Property | Value |
|----------|-------|
| **Type** | `integer` |
| **Required** | No |
| **Default** | `60` |
| **Validation** | min=1,max=60 |

### `DB_CONN_MAX_LIFETIME_MIN`

Max lifetime in minutes

| Property | Value |
|----------|-------|
| **Type** | `integer` |
| **Required** | No |
| **Default** | `240` |
| **Validation** | min=1,max=1440 |

### `DB_MAX_IDLE_CONNS`

| Property | Value |
|----------|-------|
| **Type** | `integer` |
| **Required** | No |
| **Default** | `50` |
| **Validation** | min=1,max=100 |

### `DB_MAX_OPEN_CONNS`

| Property | Value |
|----------|-------|
| **Type** | `integer` |
| **Required** | No |
| **Default** | `200` |
| **Validation** | min=1,max=1000 |

## 📂 Database migration settings

### `AUTO_MIGRATE`

Enable automatic database migrations on startup

| Property | Value |
|----------|-------|
| **Type** | `boolean` |
| **Required** | No |
| **Default** | `false` |

### `AUTO_MIGRATE_SILENT_FAIL`

Continue running even if migrations fail

| Property | Value |
|----------|-------|
| **Type** | `boolean` |
| **Required** | No |
| **Default** | `false` |

### `MIGRATIONS_PATH`

Path to migration files

| Property | Value |
|----------|-------|
| **Type** | `string` |
| **Required** | No |
| **Default** | `migrations` |

## 📂 Database monitoring configuration

### `ENABLE_DB_MONITORING`

| Property | Value |
|----------|-------|
| **Type** | `boolean` |
| **Required** | No |
| **Default** | `false` |

## 📂 Performance optimization settings

### `HTTP_MAX_IDLE_CONNS`

HTTP connection pool size

| Property | Value |
|----------|-------|
| **Type** | `integer` |
| **Required** | No |
| **Default** | `100` |
| **Validation** | min=10,max=1000 |

### `HTTP_MAX_IDLE_CONNS_PER_HOST`

HTTP connections per host

| Property | Value |
|----------|-------|
| **Type** | `integer` |
| **Required** | No |
| **Default** | `50` |
| **Validation** | min=5,max=500 |

## 📂 Profiling configuration

### `ENABLE_PPROF`

Enable pprof endpoints for performance profiling (development only)

| Property | Value |
|----------|-------|
| **Type** | `boolean` |
| **Required** | No |
| **Default** | `false` |

## 📂 OpenTelemetry tracing configuration

### `OTEL_EXPORTER_OTLP_ENDPOINT`

OTLP gRPC endpoint for trace export (e.g., `localhost:4317`). When set, the bot sends traces to this endpoint.

| Property | Value |
|----------|-------|
| **Type** | `string` |
| **Required** | No |

### `OTEL_EXPORTER_CONSOLE`

Enable console exporter for debugging traces (outputs to stderr).

| Property | Value |
|----------|-------|
| **Type** | `boolean` |
| **Required** | No |
| **Default** | `false` |

### `OTEL_EXPORTER_OTLP_INSECURE`

Use insecure OTLP gRPC connection (no TLS). Only used when `OTEL_EXPORTER_OTLP_ENDPOINT` is set.

| Property | Value |
|----------|-------|
| **Type** | `boolean` |
| **Required** | No |
| **Default** | `false` |

### `OTEL_SERVICE_NAME`

Service name for trace identification.

| Property | Value |
|----------|-------|
| **Type** | `string` |
| **Required** | No |
| **Default** | `alita_robot` |

### `OTEL_TRACES_SAMPLE_RATE`

Trace sampling rate from 0.0 (no traces) to 1.0 (all traces).

| Property | Value |
|----------|-------|
| **Type** | `float` |
| **Required** | No |
| **Default** | `1.0` |
| **Validation** | min=0.0,max=1.0 |

## 📂 HTTP Server configuration

### `HTTP_PORT`

Unified HTTP server for health checks, metrics, and webhooks. If `HTTP_PORT` is
unset, the bot uses `PORT` (as injected by Railway) before falling back to 8080.

| Property | Value |
|----------|-------|
| **Type** | `integer` |
| **Required** | No |
| **Default** | `8080` |
| **Validation** | min=1,max=65535 |

### `METRICS_AUTH_TOKEN`

Bearer token required by `/metrics` and `/db_metrics`. Leaving it empty exposes
those endpoints without authentication and logs a warning.

| Property | Value |
|----------|-------|
| **Type** | `string` |
| **Required** | No |

## 📂 Redis configuration

### `REDIS_ADDRESS`

Redis host and port. It takes precedence over `REDIS_URL`; if neither is set,
the bot connects to `localhost:6379`.

| Property | Value |
|----------|-------|
| **Type** | `string` |
| **Required** | No |
| **Default** | `localhost:6379` |

### `REDIS_URL`

Standard Redis URL (`redis://user:password@host:port/db` or `rediss://...`).
When `REDIS_ADDRESS` is unset, the full URL is used so usernames, TLS, and the
path database are preserved. Explicit `REDIS_PASSWORD` and `REDIS_DB` values
override their URL components.

| Property | Value |
|----------|-------|
| **Type** | `string` |
| **Required** | No |

### `REDIS_DB`

| Property | Value |
|----------|-------|
| **Type** | `integer` |
| **Required** | No |
| **Default** | `1` |

Set `REDIS_DB=0` explicitly to select database zero.

### `REDIS_PASSWORD`

| Property | Value |
|----------|-------|
| **Type** | `string` |
| **Required** | No |

## 📂 Local cache layer

Read-through lookups keep a short-lived copy in process memory in front of
Redis. See [Caching Architecture](/architecture/caching/#local-layer).

### `CACHE_LOCAL_TTL`

Seconds a replica serves a cached value from memory before reading Redis
again. Other replicas can see a change up to this long after a write; the
replica that wrote sees it immediately. `0` disables the local layer. An
unparsable value falls back to the default.

| Property | Value |
|----------|-------|
| **Type** | `integer` |
| **Required** | No |
| **Default** | `10` |
| **Validation** | min=0,max=300 |

### `CACHE_LOCAL_MAX_ENTRIES`

Maximum number of entries in the local layer; the least recently used entry is
evicted first.

| Property | Value |
|----------|-------|
| **Type** | `integer` |
| **Required** | No |
| **Default** | `50000` |
| **Validation** | min=100,max=1000000 |

## 📂 Resource monitoring limits

### `RESOURCE_GC_THRESHOLD_MB`

Memory threshold for triggering GC

| Property | Value |
|----------|-------|
| **Type** | `integer` |
| **Required** | No |
| **Default** | `400` |
| **Validation** | min=100,max=5000 |

### `RESOURCE_MAX_GOROUTINES`

Maximum goroutines before triggering cleanup

| Property | Value |
|----------|-------|
| **Type** | `integer` |
| **Required** | No |
| **Default** | `1000` |
| **Validation** | min=100,max=10000 |

### `RESOURCE_MAX_MEMORY_MB`

Maximum memory usage in MB

| Property | Value |
|----------|-------|
| **Type** | `integer` |
| **Required** | No |
| **Default** | `500` |
| **Validation** | min=100,max=10000 |

## 📂 Safety and performance limits

### `CLEAR_CACHE_ON_STARTUP`

Whether to clear disposable `alita:cache:*` entries on bot startup. Operational keys (lockdown, staff and raid detection state under `alita:lockdown:*`, `alita:staff:*` and `alita:raid:*`) and pending confirmations are preserved.

| Property | Value |
|----------|-------|
| **Type** | `boolean` |
| **Required** | No |
| **Default** | `false` |

### `DISPATCHER_MAX_ROUTINES`

Max concurrent goroutines for dispatcher

| Property | Value |
|----------|-------|
| **Type** | `integer` |
| **Required** | No |
| **Default** | `200` |
| **Validation** | min=1,max=1000 |

### `ENABLE_BACKGROUND_STATS`

Automatically enabled in production (when `DEBUG=false`). Set to `false` to disable.

| Property | Value |
|----------|-------|
| **Type** | `boolean` |
| **Required** | No |
| **Default** | `true` (production), `false` (debug) |

### `ENABLE_PERFORMANCE_MONITORING`

Automatically enabled in production (when `DEBUG=false`). Set to `false` to disable.

| Property | Value |
|----------|-------|
| **Type** | `boolean` |
| **Required** | No |
| **Default** | `true` (production), `false` (debug) |

## 📂 Webhook configuration

### `USE_WEBHOOKS`

| Property | Value |
|----------|-------|
| **Type** | `boolean` |
| **Required** | No |
| **Default** | `false` |

### `WEBHOOK_DOMAIN`

Required when `USE_WEBHOOKS=true`.

| Property | Value |
|----------|-------|
| **Type** | `string` |
| **Required** | Conditional |

### `WEBHOOK_SECRET`

Required when `USE_WEBHOOKS=true`.

| Property | Value |
|----------|-------|
| **Type** | `string` |
| **Required** | Conditional |

## Quick Reference

### Required Variables

```bash
BOT_TOKEN=
DATABASE_URL=
MESSAGE_DUMP=
OWNER_ID=
```

### Optional Variables

```bash
ACTIVITY_CHECK_INTERVAL=# hours between activity checks (default: 1)
ANTHROPIC_API_KEY=# Anthropic key for the image checks of the AI raid trigger (off when unset)
ANTHROPIC_MODEL=# Claude model for those checks (default: claude-haiku-5-5)
API_SERVER=# custom API server URL (default: https://api.telegram.org)
AUTO_MIGRATE=# enable automatic database migrations (default: false)
AUTO_MIGRATE_SILENT_FAIL=# continue even if migrations fail (default: false)
CAPTCHA_WEB_BASE_URL=# public https address of the bot's HTTP port for the web captcha (all four web captcha variables together or none)
CAPTCHA_WEB_TRUSTED_PROXIES=# comma-separated IPs or CIDRs of the reverse proxies whose X-Forwarded-For is trusted (empty: header ignored)
CLEAR_CACHE_ON_STARTUP=false # clear disposable cached data on startup
DB_CONN_MAX_IDLE_TIME_MIN=# max idle time in min (default: 60)
DB_CONN_MAX_LIFETIME_MIN=# max lifetime in min (default: 240)
DB_MAX_IDLE_CONNS=# (default: 50)
DB_MAX_OPEN_CONNS=# (default: 200)
DEBUG=# enable debug logging (default: false)
DISPATCHER_MAX_ROUTINES=# (default: 200)
DROP_PENDING_UPDATES=# (default: false)
ENABLE_AISPAM=# global kill switch for every AI check, text and images (default: true)
ENABLE_AUTO_CLEANUP=# (default: true)
ENABLE_BACKGROUND_STATS=# (default: true in prod, false in debug)
ENABLE_DB_MONITORING=# (default: false)
ENABLE_PERFORMANCE_MONITORING=# (default: true in prod, false in debug)
ENABLE_PPROF=# enable pprof endpoints (default: false)
ENABLED_LOCALES=# comma-separated language codes (default: en)
HTTP_MAX_IDLE_CONNS=# (default: 100)
HTTP_MAX_IDLE_CONNS_PER_HOST=# (default: 50)
HTTP_PORT=# unified HTTP server port (PORT fallback, then 8080)
INACTIVITY_THRESHOLD_DAYS=# days before marking inactive (default: 30)
METRICS_AUTH_TOKEN=# protects /metrics and /db_metrics
MIGRATIONS_PATH=# path to migration files (default: migrations)
OTEL_EXPORTER_CONSOLE=# enable console trace exporter (default: false)
OTEL_EXPORTER_OTLP_ENDPOINT=# OTLP gRPC endpoint (e.g., localhost:4317)
OTEL_EXPORTER_OTLP_INSECURE=# use insecure OTLP gRPC (default: false)
OTEL_SERVICE_NAME=# service name for traces (default: alita_robot)
OTEL_TRACES_SAMPLE_RATE=# trace sampling rate 0.0-1.0 (default: 1.0)
REDIS_DB=# Redis database number (default: 1)
REDIS_PASSWORD=# Redis password
REDIS_URL=# Redis URL (fallback for REDIS_ADDRESS + REDIS_PASSWORD)
RESOURCE_GC_THRESHOLD_MB=# GC threshold in MB (default: 400)
RESOURCE_MAX_GOROUTINES=# max goroutines (default: 1000)
RESOURCE_MAX_MEMORY_MB=# max memory in MB (default: 500)
TELEGRAM_MINI_APP_SHORT_NAME=# short name of the web captcha Mini App from BotFather /newapp (all four web captcha variables together or none)
TURNSTILE_SECRET_KEY=# Cloudflare Turnstile secret key (scrubbed from logs; all four web captcha variables together or none)
TURNSTILE_SITE_KEY=# Cloudflare Turnstile site key (public; all four web captcha variables together or none)
TYPESAFE_API_KEY=# TypeSafe API key for the AI spam filter
USE_WEBHOOKS=# enable webhook mode (default: false)
WEBHOOK_DOMAIN=# required if USE_WEBHOOKS=true
WEBHOOK_SECRET=# required if USE_WEBHOOKS=true
```
