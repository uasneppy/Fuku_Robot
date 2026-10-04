# Stack Research

**Domain:** Telegram group-management bot (Go / gotgbot v2): Staff Group fan-out moderation, AI-assisted raid lockdown, Turnstile Mini App captcha, inline settings menu
**Researched:** 2026-10-04
**Mode:** Ecosystem, subsequent milestone. The existing stack (Go 1.26.0, gotgbot v2, GORM/PostgreSQL, Redis, `alita/utils/httpserver`) is fixed and not re-researched. This file covers only what must be ADDED.
**Overall confidence:** MEDIUM. Library versions were verified against proxy.golang.org and module source. The Telegram and Cloudflare behaviour claims come from search-result summaries, because `core.telegram.org` and `developers.cloudflare.com` were blocked by the session's egress policy. Each phase must re-confirm the items marked "verify in phase" against primary docs.

## Headline Recommendation

The additions are small. **Add only two third-party Go modules**: `golang.org/x/time` (rate limiting for fan-out) and `github.com/anthropics/anthropic-sdk-go` (image and borderline-text classification). Everything else is stdlib plus modules already in `go.mod`.

- Turnstile `siteverify`: about 60 lines of `net/http`, in the style of the existing `aispam_jev.go`.
- Mini App `initData` HMAC validation: about 30 lines of `crypto/hmac`, hand-written.
- Mini App page: `embed` plus `html/template` served by the existing HTTP server.
- Settings menu: existing `callbackcodec`, `keyboard` and gotgbot types. No library.

## Recommended Stack

### Core Technologies (new)

| Technology | Version | Purpose | Why Recommended | Confidence |
|------------|---------|---------|-----------------|------------|
| `golang.org/x/time/rate` | v0.16.0 (2026-08-19) | Token-bucket limiter for Staff Group fan-out: one global bucket plus per-chat buckets | The standard Go limiter, maintained by the Go team. Telegram publishes no limits for admin calls, so the bot needs its own governor plus `retry_after` handling. Already in the `golang.org/x/*` family the repo uses (`x/sync` v0.23.0), so no new trust surface. | HIGH (version), MEDIUM (limit values) |
| `golang.org/x/sync` (`errgroup`, `semaphore`) | v0.23.0 (already required) | Bounded-concurrency fan-out across linked groups | Already a direct dependency. `errgroup.SetLimit` bounds in-flight calls. Do not add a worker-pool library. | HIGH |
| `github.com/anthropics/anthropic-sdk-go` | v1.78.0 (2026-09-30) | Classify borderline text and images in the raid pipeline using `claude-haiku-4-5` | TypeSafe Jev is text-only (see Alternatives), so images need a vision-capable model. Claude Haiku 4.5 is vision-capable, cheap ($1 / $5 per MTok in/out, 200K context) and supports structured outputs, so the verdict is schema-valid JSON. Provides typed errors and built-in retries (default 2 on 408/409/429/5xx). | MEDIUM-HIGH |
| Cloudflare Turnstile `siteverify` (stdlib `net/http`) | API v0 (`POST https://challenges.cloudflare.com/turnstile/v0/siteverify`) | Server-side validation of the Mini App token | No official or necessary Go SDK. The call is one form-encoded POST. Reuse the `aispam_jev.go` pattern: dedicated `http.Client`, 5 s timeout, 1 MiB response cap, single retry. | HIGH |
| Telegram Mini App `initData` validation (stdlib `crypto/hmac`, `crypto/sha256`) | n/a | Prove the Mini App request came from Telegram and identify the user | About 30 lines. Writing it in-repo gives constant-time compare, strict duplicate-key rejection and an exact `auth_date` window. See "initData validation" below. | HIGH |
| Go `embed` + `html/template` + `net/http` | stdlib (Go 1.26.0) | Serve the Mini App HTML/JS from the existing HTTP server | One binary, one origin, no frontend toolchain. Satisfies "the bot hosts the page". | HIGH |

### Existing modules to reuse (do not replace)

| Module | Reuse for |
|--------|-----------|
| `github.com/redis/go-redis/v9` v9.22.0 | Captcha nonce store. Consume with `GETDEL` for single use. Also the lockdown counters and alert-token store (callback data is capped at 64 bytes). Operational keys sit outside `alita:cache:` per AGENTS.md. |
| `github.com/google/uuid` v1.6.0 | Turnstile `idempotency_key`, nonces |
| `alita/utils/callbackcodec` | All settings-menu and lockdown-alert buttons |
| `alita/utils/logredact` | `RegisterSecret` for `TURNSTILE_SECRET_KEY`, `ANTHROPIC_API_KEY` |
| gotgbot `TelegramError.ResponseParams.RetryAfter` | 429 handling. gotgbot (rc.36 plus the repo's Sep-2026 pseudo-version) exposes it. It has **no built-in retry or limiter**, so the bot must wrap it. |

### Supporting Libraries

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `github.com/telegram-mini-apps/init-data-golang` | v1.5.0 (2025-03-09), zero deps | Ready-made `initdata.Validate` / `Parse` | **Optional fallback only.** Last released March 2025. Its `Validate` compares the hash with `!=`, which is not constant-time, and it takes `v[0]` for duplicate keys. Prefer the hand-written validator. Use it only as a cross-check in tests (see Installation). |
| Telegram `telegram-web-app.js` (`https://telegram.org/js/telegram-web-app.js`) | served by Telegram | Mini App client SDK (`Telegram.WebApp.initData`, `ready()`, `close()`) | Always, on the captcha page. Load it from Telegram, not a vendored copy, and allow it in CSP `script-src`. |
| Cloudflare Turnstile `api.js?render=explicit` | served by Cloudflare | Client widget | Always. Use **explicit render** so the widget lifecycle (`callback`, `error-callback`, `expired-callback`, `timeout-callback`) is controlled from the page. |
| Cloudflare Turnstile test keys | n/a | Tests and local dev | Sitekey `1x00000000000000000000AA` always passes. Cloudflare also documents always-fail sitekeys and a dummy secret for `siteverify`. Use them in `httptest`-based tests. Do not call the live endpoint in `make test`. |
| `alicebob/miniredis/v2` v2.39.0 (existing) | n/a | Test nonce/`GETDEL` and lockdown counters | Already the repo's Redis fixture. |

### Development Tools

| Tool | Purpose | Notes |
|------|---------|-------|
| `httptest.Server` (stdlib) | Fake `siteverify` and Anthropic endpoints | Make the endpoints package-level vars, as `aispamJevEndpoint` is. AGENTS.md forbids mock libraries and wants hand-written fakes. |
| Telegram test environment (`UseTestEnvironment` on gotgbot's `BaseBotClient`) | Exercise Mini App launch end to end | Mini Apps can be tested against Telegram's test servers. The initData third-party signature uses a separate test public key. |
| BotFather `/newapp` | Register a **Direct Link Mini App** (short name plus HTTPS URL) | Manual one-time ops step. Must be in the deploy runbook. |

## Capability-by-Capability Findings

### 1. Staff Group fan-out: rate limits and partial failure (MEDIUM)

- Telegram does not publish limits for admin methods (`banChatMember`, `restrictChatMember`). Community guidance is roughly 30 messages/s global and about 20 messages/min per group for sends. Admin calls are "more forgiving", but no number is guaranteed. A hard-coded limit is therefore a guess. Treat `429` with `retry_after` as the source of truth.
- **Recommended design** (stdlib plus `x/time` plus `x/sync`):
  - One process-wide `rate.Limiter`, about 20 rps with a small burst, shared by all fan-out calls. Config-tunable.
  - `errgroup` with `SetLimit(~8)` across linked groups.
  - Wrap each Bot API call in `callWithRetry`. On `*gotgbot.TelegramError` with `Code == 429`, sleep `ResponseParams.RetryAfter` (cap about 30 s, honour context), then retry up to 2 times.
  - Classify results per group as `done`, `skipped` (not admin, or missing the right), or `failed` (bot lacks rights, chat gone, retries exhausted). Never drop a failure. This is the core PROJECT.md requirement.
  - Send **one** summary message to the issuer. Per-group log-channel posts go through the same limiter (1 message/s per chat).
- The limiter is per-process. AGENTS.md notes antiflood counters are per-replica. If deployed with more than one replica, fan-out limits are per-replica too. If multi-replica is real, use a Redis token bucket. The repo already has the Redis `SetNX` pattern in `ratelimit/backup_ratelimit.go`. For a single-owner bot, in-process is sufficient.
- Timed actions: `until_date` under 30 s or over 366 days from now is treated as **permanent** by Telegram. Validate durations in the command parser and reject or clamp them. gotgbot's `BanChatMemberOpts.UntilDate` / `RestrictChatMemberOpts.UntilDate` doc comments state this.
- Admin gating needs `getChatMember(chat, issuer)` per linked group, which is one extra call per group per action. Add it to the same limiter budget. Cache admin status through the existing admin cache where it is fresh enough. **Never skip the check** (PROJECT.md constraint).
- Use `getChatMember` status `creator` for the owner check that gates linking. React to `chat_member` updates to auto-unlink on ownership change. `chat_member` is **already in `AllowedUpdates`** (`alita/config/config.go`), and the bot must be an admin in the chat to receive it. Verify the owner-transfer path in phase.

### 2. Raid lockdown: Bot API mechanics (MEDIUM, one LOW risk)

- Chat-wide mute: `setChatPermissions` with all permissions false (one call, no member enumeration). The bot cannot list members, so per-member muting at lockdown time is impossible. Admins are unaffected by default permissions.
- Joiner kick: on each new member while locked, `banChatMember` then `unbanChatMember` (kick that allows rejoin), or a timed ban. Record joiner IDs in a Redis list for the "Ban N recent joiners" button.
- **LOW-confidence risk to spike in the first raid phase: exempting approved users.** The requirement is "mute everyone except admins and approved users". Whether `restrictChatMember` (with `use_independent_chat_permissions=true`) can grant a specific member **more** than the chat default could not be confirmed from primary docs. One search summary claimed it can, but it conflated the flag's documented meaning (permission implication) with exceeding the default. Run a 30-minute experiment on a throwaway supergroup before designing around it. Fallbacks:
  - (a) Instead of a default-permission mute, enforce the lockdown with a watcher that `deleteMessage`s non-exempt senders. This costs more API calls.
  - (b) Grant approved users a no-rights admin promotion for the duration.
  - (c) Accept that approved users are muted and note it in the alert.
- Alert buttons: `callbackcodec`-encoded, with a short Redis token for the burst's joiner list. Do not put user lists in callback data (64-byte cap, and `encodeCallbackData` returns `""` on overflow, which ships a dead button).
- Detection (joins/min, media flood) is plain Redis counters (`INCR` plus `EXPIRE`), as `antiraid` does today. **No library needed.** Do not use per-replica in-process counters for the surge trigger.

### 3. AI classification for borderline text and images (MEDIUM-HIGH)

- **Text (borderline):** keep the existing TypeSafe Jev path (`jev-latest`, text-only, calibrated probabilities, about 70-500 ms, $0.042/MTok input). It is already integrated, cheap and fast. Reuse its circuit breaker and shed-on-overload behaviour.
- **Images: Jev cannot do this.** Multiple sources agree Jev takes text only (no images, audio or video). Use **Claude Haiku 4.5** (`claude-haiku-4-5`, 200K context, $1 in / $5 out per MTok, vision). It is the cheapest current vision-capable Claude. Newer Sonnet-tier models (`claude-sonnet-5-5`, $2 / $10) are available if Haiku's accuracy on spam and porn imagery proves insufficient. Make the model ID a config value.
- Image path:
  1. Pick a mid-size `PhotoSize` (about 320-800 px) from the Telegram message. For stickers, GIFs and video, use `Thumbnail`. Animated TGS stickers cannot be classified.
  2. `bot.GetFile`, then download from `https://api.telegram.org/file/bot<token>/<path>`. Do **not** download originals. The `backup.go` downloader already shows the base-URL pattern.
  3. Send it as a base64 image block to Claude. SDK helper: `anthropic.NewImageBlockBase64(mediaType, b64)`.
  4. Constrain the output with structured outputs: `OutputConfig.Format` (a `JSONOutputFormatParam` with a JSON Schema) such as `{verdict: "spam"|"ok"|"unsure", category, confidence}`.
- **Prompt-injection hardening** (user text and image text are untrusted):
  - Verdict-only output through the JSON schema.
  - The model never receives tool access, so it cannot act.
  - The model's output only raises or lowers a score. **Rules, not the model, trigger lockdown** (a PROJECT.md decision).
  - Truncate message text, wrap it in delimiters, and tell the model in the system prompt that content inside the delimiters is data.
  - Put the static system prompt first so prompt caching applies. Verify `usage.cache_read_input_tokens` is non-zero. The minimum cacheable prefix is model-dependent (about 1-4K tokens), so a short prompt will not cache and that is fine.
- Cost control: classify only borderline cases (rule score in a middle band), only from non-approved users, with a per-chat per-minute cap and the same breaker/shed semantics as `aispam`. Use structured outputs (`OutputConfig.Format`) rather than forced tool use to get JSON back.
- Register `ANTHROPIC_API_KEY` with `logredact.RegisterSecret`. Feature is inert when the key is empty, matching the `TYPESAFE_API_KEY` convention.
- Dependency note: `anthropic-sdk-go` v1.78.0 has a large module graph (AWS, Google, MCP SDKs for its Bedrock/Vertex backends). With Go module pruning only the imported packages compile, but `go.mod`/`go.sum` will grow. If that is unacceptable, call `POST /v1/messages` with `net/http`. It is one endpoint and the repo already does this for Jev. The tradeoff is losing typed errors and retries. **Recommendation: use the SDK**, run `govulncheck` (already in CI), and revisit only if `go mod tidy` pulls something objectionable. Note that its `go.mod` declares `go 1.24`, so it is compatible with Go 1.26.0.

### 4. Turnstile Mini App captcha (MEDIUM, with items to verify in phase)

**Launch mechanism, the most important finding.** The `web_app` inline-keyboard button is **private chats only** (gotgbot's own doc comment on `InlineKeyboardButton.WebApp` says "Available only in private chats between a user and the bot"). A group welcome message therefore cannot carry a `web_app` button. Use a **Direct Link Mini App**: an inline `url` button pointing at `https://t.me/<bot_username>/<app_short_name>?startapp=<nonce>`. This opens in any chat. `startapp` is delivered to the app as `start_param` inside signed `initData`. The Mini App must be registered with BotFather `/newapp` first. The `startapp` value is limited to `A-Za-z0-9_-` and 512 chars, so use a 128-bit random nonce (base64url or hex). **Verify the exact limits in phase.**

**Flow:**
1. User joins and is restricted (existing captcha flow). Bot creates a nonce, stores it in Redis (`alita:captcha:miniapp:<nonce>` mapping to `{chat, user}`, TTL equal to the captcha timeout) and sends the `url` button.
2. Mini App page loads. JS reads `Telegram.WebApp.initData` and renders Turnstile explicitly (`action: "join"`, `cdata: <nonce>`).
3. On the widget callback, JS `POST`s `{initData, turnstileToken}` to `/captcha/verify` (same origin).
4. Server, in this order:
   - Validate the `initData` HMAC.
   - Check `auth_date` is within the allowed window (about 5-10 min).
   - Read `start_param` and `GETDEL` the nonce.
   - **Require `user.id` from the signed initData to equal the stored pending user.** Without this, anyone with the link could verify on someone else's behalf.
   - Call `siteverify`.
   - On `success` with matching `hostname`, `action` and `cdata`, run the existing "captcha passed" path: unrestrict, replay pending messages, `cache.DeleteCache("alita:cache:captcha_pending:<chat>")` per AGENTS.md.
5. Failure paths keep the existing "one attempt per `(user, chat)`" semantics (a deliberate PROJECT.md decision, to reconcile with "can rejoin to try again").

**`siteverify` specifics (verify in phase against Cloudflare docs):**
- Form-encoded POST (JSON also accepted). Fields: `secret`, `response` (token), optional `remoteip`, optional `idempotency_key` (UUID, so a retried request after a network error is not rejected as a duplicate).
- Response: `success`, `error-codes[]`, `challenge_ts`, `hostname`, `action`, `cdata`. **Check `hostname`, `action` and `cdata` yourself.** Success alone does not prove the token was minted for your page and nonce.
- Tokens are single-use and expire after 300 s. Error `timeout-or-duplicate` means expired or replayed. Do not retry the siteverify call with the same token unless an `idempotency_key` is supplied.
- Fail **closed** on network error or non-200 (the user stays restricted, the timeout kick applies). Do not fail open.
- `remoteip` is optional, so omit it unless a trusted proxy header is configured.

**initData validation (stdlib, about 30 lines):**
1. `url.ParseQuery(initData)`. Reject duplicate keys and a missing `hash`.
2. Data-check string: every pair except `hash`, as `key=value` (decoded values), sorted by key, joined with `\n`. For **first-party** validation, `signature` stays in the string and only `hash` is excluded. The Mini Apps library's `Validate` does this.
3. `secret = HMAC_SHA256(key="WebAppData", msg=botToken)`, then `calc = hex(HMAC_SHA256(key=secret, msg=dataCheckString))`.
4. `hmac.Equal` (constant time) against `hash`.
5. Enforce `auth_date` freshness. A valid signature only proves origin, not recency, so without this check a leaked `initData` string is valid forever.
6. Ed25519 `signature` / third-party validation is **not needed** (the bot owns the token). The public keys, if ever needed, are in `init-data-golang` v1.5.0 `validate_third_party.go`: production `e7bf03a2fa4602af4580703d88dda5bb59f32ed8b02a56c187fe7d34caed242d`, test `40055058a4ee38156a06562e52eece92a771bcd8346a8c4615cb7376eddf72ec`.
7. Never trust `initDataUnsafe` on the server. Only the validated raw string.

**Hosting and security:**
- Mini Apps require a **public HTTPS** URL. The deploy needs a TLS-terminating reverse proxy or tunnel (Caddy, nginx, Cloudflare Tunnel) in front of the bot's HTTP port. The current `Server` has no TLS. The `docker-compose.yml` should gain the proxy or document it. **This is a deploy-side change the roadmap must schedule.** The Turnstile widget's allowed hostname list must include this domain.
- Mount routes on `Server.mux` only, never on `http.DefaultServeMux`. `server.go` exposes `DefaultServeMux` for pprof, and routes added there would be exposed by that path.
- The server's `WriteTimeout` is 10 s, which is fine for a verify call bounded at about 5 s.
- Security headers on the page: a strict CSP allowing `script-src 'self' https://telegram.org https://challenges.cloudflare.com` (use a nonce for inline script), `frame-src https://challenges.cloudflare.com`, `connect-src 'self'`, plus `Referrer-Policy: no-referrer` and `X-Content-Type-Options: nosniff`. Do **not** send `X-Frame-Options: DENY` or `frame-ancestors 'none'`. Telegram Web and some clients embed Mini Apps in an iframe, so a `frame-ancestors` allowlist of Telegram origins is the right control. Verify the exact origins in phase.
- **Bot API 10.2 (2026-07-14) Mini App origin protection**, enforced by default from 2026-07-20 (opt-out via BotFather): Mini App methods are blocked when called from an origin other than the Mini App's registered domain. Keep **every** `Telegram.WebApp.*` call on the bot's own origin, and never call them from inside the Turnstile iframe or any third-party frame. The design above does (the Turnstile callback hands the token to the same-origin page, which does the `fetch`). Source is secondary, so verify in phase.
- Rate-limit `/captcha/verify` per IP and per nonce with Redis `INCR`/`EXPIRE`. No middleware library is needed.
- Register `TURNSTILE_SECRET_KEY` with `logredact.RegisterSecret` (6+ chars). The sitekey is public and goes in the page template.

### 5. Inline settings menu (HIGH)

No library. Use gotgbot's `InlineKeyboardMarkup`, `EditMessageText` and `AnswerCallbackQuery` with the repo's `callbackcodec` (`<ns>|v1|<url-encoded>`, 64 B). Keep menu state in the callback payload (page and chat) plus short Redis tokens for anything larger. Gate every callback with a fresh admin check (not a cached message-author assumption). Always `AnswerCallbackQuery`. Edit in place instead of sending new messages to stay under per-chat send limits. Handlers follow AGENTS.md (value receivers, `WrapCommand` for the `/settings` entry point, anonymous-admin registration where needed). Use `RegisterLegacyModule` with a unique name.

## Installation

```bash
# Run from the repo root. AGENTS.md: stay on the latest upstream versions.
go get golang.org/x/time@v0.16.0
go get github.com/anthropics/anthropic-sdk-go@v1.78.0
go mod tidy

# Optional, tests only (cross-check the hand-written validator):
# go get github.com/telegram-mini-apps/init-data-golang@v1.5.0

# No new dev tooling. Reuse: make test / make lint / govulncheck
```

New environment variables (all new secrets registered via `logredact.RegisterSecret`):
`TURNSTILE_SITE_KEY`, `TURNSTILE_SECRET_KEY`, `MINIAPP_BASE_URL`, `MINIAPP_SHORT_NAME`, `ANTHROPIC_API_KEY`, `RAID_VISION_MODEL` (default `claude-haiku-4-5`), `FANOUT_RPS`.

## Alternatives Considered

| Recommended | Alternative | When to Use Alternative |
|-------------|-------------|-------------------------|
| Hand-written `initData` validator | `telegram-mini-apps/init-data-golang` v1.5.0 | If you want the maintained-looking API and accept a stale module (last release 2025-03-09). It has zero dependencies, so risk is low. Choose it if you would rather not own crypto code, and patch the constant-time compare upstream. |
| Claude Haiku 4.5 for images | OpenAI `omni-moderation-latest` (free) | Only for NSFW/violence/self-harm. Spam, scams and promotional images are **not categories** (images are covered only for violence, self-harm and sexual). Not a fit for raid spam. |
| Claude Haiku 4.5 for images | A larger tier (`claude-sonnet-5-5`, $2/$10) | If Haiku misclassifies in practice. Keep the model ID in config. |
| Anthropic Go SDK | Raw `net/http` to `/v1/messages` | If the module-graph growth is unacceptable. Matches the repo's Jev style, but you lose typed errors and retries. |
| `x/time/rate` in-process | Redis token bucket | If the bot runs multiple replicas, so fan-out limits must be global. |
| Direct Link Mini App via `url` button | `web_app` inline button | Never in groups (private chats only). Could be used in a PM fallback ("verify in PM") if the owner dislikes direct links. |
| Reuse Jev for text | Send borderline text to Haiku too | If one provider is preferred. Jev is cheaper and already built. Do not rewrite `aispam`. |
| Cloudflare Turnstile | reCAPTCHA / hCaptcha | Out of scope per PROJECT.md. |

## What NOT to Use

| Avoid | Why | Use Instead |
|-------|-----|-------------|
| `web_app` inline button in the group welcome message | Telegram only allows it in private chats, so the button would be rejected or useless | `url` button to a `t.me/<bot>/<app>?startapp=<nonce>` direct link |
| Trusting `Telegram.WebApp.initDataUnsafe` or a client-sent `user_id` | Client-controlled; enables verifying for someone else | Server-side HMAC over raw `initData`, then compare `user.id` to the pending user |
| Skipping the `auth_date` check | A valid HMAC never expires on its own, so captured `initData` is replayable forever | Reject if older than the window (about 5-10 min) |
| Treating `siteverify` `success:true` as sufficient | It does not bind the token to your page or this attempt | Also check `hostname`, `action`, `cdata` (nonce) |
| Failing open when Cloudflare is unreachable | Lets raiders through during an outage | Fail closed, keep the user restricted, let the timeout kick apply |
| `github.com/go-telegram-bot-api/*`, `telebot`, `go-telegram/bot` | A second Telegram client duplicates gotgbot and splits the handler model | gotgbot v2 only |
| Any unofficial Turnstile Go wrapper | Tiny, unaudited, and the call is 60 lines | Stdlib `net/http` |
| Sending every message or image to an AI model | Out of Scope in PROJECT.md; slow and costly | Rule-based triggers; AI only for the borderline band |
| Letting the AI verdict trigger lockdown | Prompt-injectable and non-deterministic | Rules trigger; AI adjusts a score |
| Per-member `restrictChatMember` loops for lockdown | The bot cannot enumerate members and the loop would exceed rate limits | `setChatPermissions` (chat-wide) plus targeted exceptions |
| In-process counters for the join-surge trigger | Per-replica, lost on restart (the antiflood limitation in AGENTS.md) | Redis `INCR`/`EXPIRE` |
| Registering Mini App routes on `http.DefaultServeMux` | pprof lives there | `Server.mux` |
| A frontend build toolchain (npm, React, Vite) for the captcha page | One widget and one `fetch`; adds supply-chain and CI burden | Plain embedded HTML + about 60 lines of JS |
| Hard-coding "30 msg/s, 20 msg/min" as a guarantee | Telegram does not document admin-call limits and they vary | Configurable limiter plus mandatory `retry_after` handling |

## Stack Patterns by Variant

**If the bot runs a single replica (likely, for a one-owner bot):**
- Use the in-process `rate.Limiter` and Redis for nonces and counters.
- Because it is simplest and matches how everything else here is deployed.

**If it runs multiple replicas:**
- Use a Redis-backed token bucket for fan-out, and note the lockdown state must live in Redis (as `antiraid` does).
- Because per-replica limiters multiply the real request rate.

**If the owner will not set up a public HTTPS domain:**
- The Turnstile Mini App is not feasible (Telegram requires HTTPS for Mini Apps). Fall back to the existing `base64Captcha` flow.
- Because there is no Mini App without a public HTTPS origin. Raise this at roadmap time, before the captcha phase.

**If the approved-user exemption spike fails (LOW-risk item above):**
- Use delete-on-send enforcement for lockdown instead of a default-permission mute.
- Because default permissions cannot be selectively overridden.

## Version Compatibility

| Package A | Compatible With | Notes |
|-----------|-----------------|-------|
| gotgbot `v2.0.0-rc.36.0.20260919140833-240296efadb4` (repo's pin) | Telegram Bot API 10.x | Has `WebAppInfo`, `ChatPermissions` (incl. `can_react_to_messages`, `can_edit_tag`), `ResponseParameters.RetryAfter`. Latest tag is rc.36 (2026-08-02), and the repo is on a newer commit. Do not move off it for this milestone. |
| `anthropic-sdk-go` v1.78.0 | Go 1.24+ (declares `go 1.24`, `toolchain go1.25.8`) | Fine under Go 1.26.0. Has `NewImageBlockBase64` and `OutputConfigParam.Format`. |
| `golang.org/x/time` v0.16.0 | Go 1.26.0 | No known issues. |
| `CGO_ENABLED=0` production builds | All additions | All additions are pure Go. Tests still need `CGO_ENABLED=1` for go-sqlite3, unchanged. |
| Redis 7+ | `GETDEL` (Redis 6.2+) | go-redis v9.22.0 `GetDel`. Fine. |
| Bot API 10.2 Mini App origin protection | Same-origin design above | Calls to `Telegram.WebApp.*` from another origin are blocked. |

## Sources

- proxy.golang.org module index and `.zip`/`.mod` (HIGH): `golang.org/x/time` v0.16.0 (2026-08-19), `anthropic-sdk-go` v1.78.0 (2026-09-30), `openai-go/v3` v3.71.1 (considered, rejected), `init-data-golang` v1.5.0 (2025-03-09, source read: `validate.go`, `validate_third_party.go`), `gotgbot/v2` rc.36 (2026-08-02) and repo pseudo-version (source read: `request.go`, `gen_types.go`, `gen_methods.go`).
- `pkg.go.dev/github.com/telegram-mini-apps/init-data-golang` and `pkg.go.dev/github.com/PaulSonOfLars/gotgbot/v2` (MEDIUM): versions, function signatures.
- Claude API skill (bundled, cached 2026-09-25) (HIGH for model IDs and pricing): `claude-haiku-4-5` $1/$5, 200K; Sonnet 5.5 $2/$10; Go SDK image and structured-output types.
- Web search summaries, secondary (MEDIUM, **verify in phase against primary docs**; `core.telegram.org` and `developers.cloudflare.com` were egress-blocked):
  - Cloudflare Turnstile `siteverify` endpoint, response fields, 300 s single-use tokens, `idempotency_key`, error codes, explicit render, CSP requirements, test sitekey `1x00000000000000000000AA` (Cloudflare docs excerpts and the `@marsidev/react-turnstile` server-validation guide).
  - Mini App `initData` algorithm: HMAC `WebAppData` and Ed25519 third-party validation (`docs.telegram-mini-apps.com`, various ports).
  - Web App inline button is private-chat-only, and Direct Link Mini Apps work in any chat with `startapp` to `start_param` (gotgbot's own struct comments agree, which raises confidence to MEDIUM-HIGH).
  - Bot API 10.2 (2026-07-14) Mini App origin protection, enforced from 2026-07-20 (BotNews / community migration guides, single-lineage, **MEDIUM-LOW**).
  - Telegram rate-limit folklore: about 30 msg/s global, about 20 msg/min per group, 429 `retry_after` (Telegram FAQ excerpt via search; admin-call limits undocumented).
  - `chat_member` updates require the bot to be admin and an explicit `allowed_updates` entry (grammY / aiogram docs), and the repo already includes it.
  - Jev is text-only, `jev-1.13.0`, 64K tokens (several secondary write-ups, consistent).
  - OpenAI moderation image coverage is limited to violence, self-harm and sexual (OpenAI moderation guide).
- Repo files read: `alita/modules/aispam.go`, `aispam_jev.go`, `alita/utils/httpserver/server.go`, `alita/config/config.go` (AllowedUpdates), `alita/utils/ratelimit/backup_ratelimit.go`, `alita/modules/federations.go` (`applyActiveFban`: sequential, unthrottled, errors only logged at debug, which is not enough for Staff Group).

**Not verified (open):** restricted-member vs. chat-default permission semantics; exact `startapp` charset/length and `frame-ancestors` origins; whether Turnstile renders reliably in every Telegram client WebView (iOS/Android/Desktop/Web). Plan a manual device pass in the captcha phase.

---
*Stack research for: Telegram group-management bot milestone (Staff Group, raid lockdown, Turnstile Mini App, settings menu)*
*Researched: 2026-10-04*
