# Phase 8: Turnstile Web Captcha - Research

**Researched:** 2026-10-29
**Domain:** Go Telegram bot; Telegram Mini App (direct link + signed `initData`); Cloudflare Turnstile (widget + `siteverify`); Redis one-time challenge tokens; PostgreSQL exactly-once attempt claim; embedded static page served by the bot's own HTTP server
**Confidence:** MEDIUM-HIGH (repo seams HIGH, read this session; Cloudflare widget and `siteverify` contract HIGH, read from Cloudflare's own docs repo; `initData` algorithm HIGH, reproduced against two published vectors; Telegram direct-link launch behaviour inside a real client MEDIUM/LOW, which is exactly what the deferred spike measures)

> Research constraints met: no Cloudflare or Telegram key was asked for or used, no PostgreSQL was started, no production code, test, migration or other planning file was changed. The only file written is this one. Nothing was committed.
>
> **Network reality of this session.** Blocked by the proxy (HTTP 403 on CONNECT or DNS failure): `core.telegram.org`, `telegram.org`, `docs.telegram-mini-apps.com`, `developers.cloudflare.com`, `challenges.cloudflare.com`. Allowed and used: `raw.githubusercontent.com` (Cloudflare docs source `cloudflare/cloudflare-docs`, the Telegram-Mini-Apps docs source, `telegram-mini-apps/init-data-golang`), `registry.npmjs.org` (a tarball of `@twa-dev/sdk` 8.0.2, which carries a copy of `telegram-web-app.js`), and the built-in WebSearch (summaries only). So every Cloudflare claim below is read from Cloudflare's own documentation source; every Telegram claim is from a mirror, a library or a search summary and is tagged accordingly. If the owner wants the official Telegram pages read directly, `core.telegram.org` has to be allowed in the session network settings.
> Tooling note: no MCP research provider is enabled in `.planning/config.json` (all `false`), so the `research-plan` seam was not used; sources were fetched directly.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

#### Who can pass a challenge and who decides (CAPT-03, CAPT-04)
- **D-01:** A challenge is a random 128-bit token kept in Redis that points at the existing `captcha_attempts` row (attempt ID, chat, user, expiry). The row stays the authority and keeps its one attempt per user per chat. The Mini App link carries only the token in `startapp` (letters, digits, `_` and `-`), never user text or an ID.
- **D-02:** The server verifies Telegram's signed `initData` (HMAC-SHA256 with the key derived from the bot token, `auth_date` no older than 5 minutes) and requires `user.id` to equal the challenge's user. Another member can open the link, but their verification fails and changes nothing.
- **D-03:** The token is consumed by an atomic compare-and-delete when verification starts, so a reused, parallel or expired token fails. The Turnstile widget is bound to the challenge through its `cdata` field (a value derived from the token); the server requires Cloudflare's `siteverify` answer to echo it, together with the expected hostname and `action`, so a Turnstile token minted for another challenge is rejected.
- **D-04:** The page decides nothing. It posts `initData`, the Turnstile token and the challenge token to the server and shows the server's answer. A pass is recorded only after `siteverify` returns success with the expected hostname, `action` and `cdata`.

#### The join flow, settings and failure handling (CAPT-01, CAPT-02, CAPT-05, CAPT-06, CAPT-07)
- **D-05:** `/captchamode turnstile` is a third mode beside `math` and `text`. An append-only migration widens the `captcha_mode` CHECK (`chk_captcha_mode`). The command refuses the mode with a clear reason unless the web captcha is fully configured (D-11). Math and text stay.
- **D-06:** A new member in a Turnstile group is muted and gets an attempt row as today, then a challenge message with one button that opens the Mini App through its direct link (`https://t.me/<bot>/<app>?startapp=<token>`). The message is edited or removed the way the existing captcha messages are.
- **D-07:** A pass runs the existing pass path: unmute, replay of pending messages, the greeting, and cleanup. That path becomes one shared function that the callback verify and the web verify both call, so the two cannot drift.
- **D-08:** A failed Turnstile check is "one solve only": the group's failure action runs at once (kick by default, so the member can rejoin). The group's time limit uses the existing timeout and cleanup machinery, so it works on any replica.
- **D-09:** If the web captcha is not available when a member joins a Turnstile group (not configured, or Redis unavailable), that member gets the group's math captcha instead, so nobody is left muted with no way in. A Cloudflare or network error during a check is not a failed solve: the page says "try again" and the timeout still applies.

#### The captcha page and hosting (CAPT-04, CAPT-08, PLAT-02)
- **D-10:** The bot's existing HTTP server serves the page and its API on the same port: `GET /captcha` (the page), `POST /captcha/verify` and `GET /captcha/health`. They are registered only when the web settings are complete. The owner puts TLS and the public hostname in front with their own reverse proxy.
- **D-11:** Settings: `CAPTCHA_WEB_BASE_URL` (public https), `TELEGRAM_MINI_APP_SHORT_NAME`, `TURNSTILE_SITE_KEY` and `TURNSTILE_SECRET_KEY`. The secret is registered with `logredact.RegisterSecret` and is sent only to Cloudflare. The verify route has a body-size cap, per-IP and per-token rate limits, `Cache-Control: no-store`, no CORS, a 5 s `siteverify` timeout, and returns generic errors to the page.
- **D-12:** The page is one static HTML file with a small script, embedded in the binary (`embed.FS`), with no framework and no build step. It loads Telegram's `telegram-web-app.js` and Cloudflare's Turnstile script, and a strict Content-Security-Policy allows only those two hosts.
- **D-13:** The page language comes from `language_code` in the signed `initData`, mapped to one of the bot's 7 locales with English as the fallback, and the strings are injected by the server. New page strings are locale keys in all 7 files.
- **D-14:** Because this is a user-facing screen, a UI design contract (`08-UI-SPEC.md`) is written before planning: the states loading, ready, verifying, passed, failed, expired and try-again; light and dark; small WebView sizes.

#### Deployment guide, the deferred spike and visibility (PLAT-01, PLAT-04)
- **D-15:** A self-hosting page documents the steps: a public HTTPS hostname and reverse proxy, BotFather `/newapp` Mini App registration, a Cloudflare Turnstile widget (managed mode, hostname allow-list), the four settings, a health check, what the bot sends to Cloudflare, and a troubleshooting list.
- **D-16:** Every replica can serve the page and verify, because challenge tokens and counters live in Redis and the attempt in PostgreSQL: a challenge issued by one replica can pass on another. Without Redis the Turnstile mode is unavailable and groups fall back to math (D-09).
- **D-17:** Counters for issued, passed, failed, expired and errors appear in the developer `/stats` for this replica, following the existing pattern. Tokens, `initData` and Turnstile responses are never logged.
- **D-18 (spike deferred, owner's choice):** The real-device spike gate is not run in this phase. `08-SPIKE.md` is a checklist for the owner to run on real Android, iOS and Desktop clients: (1) a direct-link button posted in a group opens the page with signed `initData` for the user who tapped, another user is rejected, and the `startapp` value fits; (2) the Turnstile widget renders and returns a token inside Telegram's WebViews, and `siteverify` reports the owner's hostname. Each step says what a result means and what to change.

#### Amendments after the UI design contract (orchestrator's calls, 2026-10-29)
- **D-19 (refines D-03):** `GET /captcha` is served before the challenge is known (Telegram passes `startapp` only in the URL fragment and in the signed `initData`), so the page computes the Turnstile `cdata` itself as `hex(SHA-256("cd1:" + start_param))`. The server recomputes it from the verified `initData` and requires `siteverify` to echo it, which still binds a Turnstile token to one challenge. No extra route is added.
- **D-20 (refines D-13):** The server cannot know the user's language before `initData` is verified, so it embeds all 7 locale string tables as a JSON block and the page picks one from `initDataUnsafe.user.language_code`, for display only, with English as the fallback. Nothing security-relevant depends on the language.
- **D-21 (refines D-03 and D-09):** A retryable outcome (Cloudflare or network error, the bot momentarily unable to reach Telegram) gives the challenge back: the server re-arms the same token with its remaining lifetime so the Try again button can succeed. A wrong user, a used or unknown token, or a failed solve never re-arms it.
- **D-22 (refines D-08):** A `failed` or `expired` answer carries the group's failure `action` (kick, ban or mute) so the page can say plainly whether the member can rejoin; the UI contract has separate copy for each.
- **D-23 (refines D-12):** The CSP uses a per-response nonce for the inline script and style, and `frame-ancestors` for the Telegram web hosts; no `X-Frame-Options` is sent, because it would break Telegram's web clients. The exact host list is confirmed in the real-device spike (`08-SPIKE.md`).
- **D-24:** The UI contract is `08-UI-SPEC.md` (hand-written page, `--tg-theme-*` variables with fallbacks, 9 states, 26 `captcha_web_*` page keys, plus the challenge message keys and three `/captchamode turnstile` refusal keys). Four existing keys are edited in all 7 locales (`captcha_mode_specify`, `captcha_mode_invalid`, `captcha_help_msg`, `captcha_invalid_mode_error`). Cloudflare's widget option names and language codes in it are from memory and must be confirmed by the executor or research; an unconfirmed language code maps to `'auto'`.

### Claude's Discretion
- Redis key names and TTLs for challenge tokens, the rate limiters and counters, and how the per-IP limit reads the client address behind the owner's proxy (trusted forwarding header settings).
- The exact `cdata` derivation, the Turnstile `action` string, and the verify route's JSON shape and status codes.
- How the migration widens the CHECK (drop and re-add inside one transaction) and how model, repository and tests change with it.
- Wording of the challenge message, button label, page strings, the refusal reasons of `/captchamode turnstile`, and all 7 locales.
- How the shared pass path and the timeout path are factored out of `captcha.go`, and which tests move with them.
- Where the page and API code live (a new package under `alita/utils/` or inside `httpserver`), keeping `alita/db` init-order rules.

### Deferred Ideas (OUT OF SCOPE)
- Running the real-device spike (direct-link launch from a group, Turnstile in Telegram WebViews) with the owner's hostname, Mini App and keys; `08-SPIKE.md` is the way to do it.
- Retries or a second solve for Turnstile, other captcha providers, and the Phase 9 Captcha settings page.
- The Ed25519 third-party `initData` signature path, and serving the page from a separate static host.

(Phase boundary, out of scope: the settings menu (Phase 9); the real-device spike; a second solve for Turnstile; image or audio captcha changes; other captcha providers; the Staff Group. **Owner decision carried in from the orchestrator:** the spike is deferred; plan and build with no live call to Telegram or Cloudflare; both are faked in tests; everything is inert until the owner sets the four settings; `08-SPIKE.md` is a checklist for the owner.)
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| CAPT-01 | An admin can set a group's captcha mode to Turnstile. Math and text modes stay available as fallbacks. | Migration widening `chk_captcha_mode` (Pattern 9), model tag + `SetCaptchaMode` + `captchaModeCommand` changes, three ordered refusals, four edited locale keys (Architecture Patterns 8-9, Code Map). |
| CAPT-02 | In Turnstile mode, a new member is muted on join and gets a challenge message with a button that opens the bot's captcha page as a Telegram Mini App. | `SendCaptcha` branch, URL button (`InlineKeyboardButton.Url`; `web_app` buttons are private-chat only), token issue, join-time fallback to math (Patterns 1, 8; Direct-link facts). |
| CAPT-03 | Only the new member the challenge was issued to can pass it. A challenge can't be reused or passed by anyone else. | Verified `initData` user binding, conditional (user-compared) consume script, forged-callback guard on the existing `captcha_verify` and `captcha_refresh` callbacks (Patterns 2, 3, 4; Pitfalls 1, 2). |
| CAPT-04 | Passing is decided only on the bot's server: Telegram's signed user data and Cloudflare's Turnstile verification. The page never reports a pass on its own. | `initData` validation (Pattern 2), `siteverify` classification table (Pattern 5), verify route contract (Pattern 6). |
| CAPT-05 | A member who passes is unmuted. Their pending messages are replayed, the greeting runs, and the challenge is cleaned up, exactly as with the existing captcha. | Shared `claimCaptchaPass` / `finishCaptchaPass` extraction from `captchaVerifyCallback` (Pattern 3). |
| CAPT-06 | A failed Turnstile check counts as failing the captcha (one solve only). The member is kicked and can rejoin to try again. | `handleCaptchaTimeout` reused for a failed solve; failed vs retryable classification (Patterns 4, 5). |
| CAPT-07 | A member who doesn't pass within the group's time limit is kicked and can rejoin to try again. | Existing timer, cleanup tick and orphan recovery already cover any attempt row; exactly-once race table (Pattern 4). |
| CAPT-08 | The captcha page is shown in the user's language, using the bot's 7 languages. | Embedded 7-language JSON from the i18n translators, page-side resolution, locale parity test that `make check-translations` cannot give (Pattern 7; Pitfall 16). |
| PLAT-04 | The deployment steps for the captcha page are documented: public HTTPS hostname, BotFather Mini App registration, Turnstile keys. | New self-hosting page, `environment.md`, `sample.env`, exposure warning for the shared port (Docs section; Pitfall 12). |
</phase_requirements>

## Project Constraints (from CLAUDE.md / AGENTS.md)

- Stay on Go 1.26.0, gotgbot v2, GORM on PostgreSQL (SQLite in tests), Redis. **This phase needs no new Go dependency** (stdlib `html/template`, `embed`, `crypto/hmac`, `net/netip`, existing `go-redis/v9`).
- Migrations are append-only, timestamp greater than every existing file (latest is `20261029120000_add_raid_detection_ai.sql`), one transaction per file, no top-level `BEGIN`/`COMMIT`, no `CREATE INDEX CONCURRENTLY`; `alita/db/migrations/runner.go` and `scripts/migrate_psql.sh` must agree (both rewrite `ALTER TABLE ... ADD CONSTRAINT` into a `DO` block that swallows `duplicate_object`; see Pitfall 15).
- Every write `cache.DeleteCache`s the keys it affects; after inserting into `captcha_attempts` the `alita:cache:captcha_pending:<chat>` flag must be invalidated after commit (`createCaptchaAttemptPreMessage` already does it, `alita/db/captcha/repository.go:224-226`).
- Commands go through `helpers.WrapCommand` (this phase adds a word to `/captchamode`, no new command). Value receivers on `moduleStruct`.
- Locale keys in all 7 files (`en es fr hi id pt ru`); `make check-translations` only sees literal `GetString("key")` calls, so dynamic page keys need their own parity test (Pitfall 16).
- Every fire-and-forget goroutine starts with `defer error_handling.RecoverFromPanic(funcName, modName)`; register every new secret with `logredact.RegisterSecret`.
- Tests: `-tags testtools`, real fixtures (`internal/testdb.Run` SQLite, miniredis, `httptest`, hand-written `gotgbot.BotClient` fakes), no mock libraries, assert observable behaviour, never source substrings; `make test` fails on an unexpected `t.Skip` (`scripts/check_test_results/main.go`; skips are allowed only for `alita/db/migrations` without PostgreSQL and two named tests).
- Shutdown runs LIFO within 60 s; a new drain registers after the DB-close handler; in-flight HTTP is stopped first (registered last in `main.go`).
- `AGENTS.md` is updated in the same commit as any change that invalidates a rule in it (this phase adds Redis key families, a callback guard, a readiness gate and a shutdown rule; see "AGENTS.md updates").
- Conventional Commits; user-visible work needs `feat:`/`fix:`. GSD workflow: all edits go through a GSD command (this research only writes this file).
- Security: per-group admin check is never skipped (`/captchamode` keeps `captchaAdminGate`); Turnstile verification is server side; the Turnstile secret is registered with `logredact.RegisterSecret`.

## Summary

The phase is one new leaf package, one new module file and a handful of surgical edits to the existing captcha, built on seams that already exist. The security core is small and was checked against primary material: the `initData` algorithm (HMAC key `WebAppData`, `data_check_string` of every field except `hash`, **`signature` included**) reproduces two published vectors, the Cloudflare `siteverify` contract (URL, fields, response fields, error codes, 300 s single-use tokens) and the widget parameter names are read from Cloudflare's own docs source, and the one-time token scripts were run on miniredis. The design that follows from them: the Redis challenge token is only a *handle*; the barrier is the signed `user.id`; the token is consumed by a **user-compared** Lua script (a wrong user changes nothing, which is what D-02 requires and what a plain compare-and-delete would break); the DB conditional delete of the attempt row decides pass vs failure vs timeout exactly once across replicas.

Three findings change the plan and are not in CONTEXT or the UI contract. **(1) A forged-callback bypass:** a Turnstile attempt row still has a normal `answer` and refresh count, so a modified client or userbot can send a hand-built `captcha_verify` or `captcha_refresh` callback for it. `captcha_refresh` would replace the Turnstile challenge with a math image the attacker can solve; `captcha_verify` would pass if the stored answer were guessable. The fix is a random unguessable reserved answer (`ts1:` + 128 random bits) and an explicit refusal of Turnstile attempts in both callbacks. **(2) Telegram may not put `start_param` into the signed `initData` for a direct link** (the telegram-apps docs contradict themselves; the actual `telegram-web-app.js` only parses `initDataUnsafe` out of `tgWebAppData`), so the page must also read `tgWebAppStartParam` from the page URL query and the server must treat a signed `start_param` as authoritative only when present. The token is not a secret, so the user binding keeps the design sound either way; the spike confirms which one Telegram does. **(3) The HTTP server's `WriteTimeout` is 10 s** (`alita/utils/httpserver/server.go:383`), shorter than a verify call (5 s `siteverify` + Telegram calls); a handler that overruns returns an empty reply. The verify route must extend its own write deadline (`http.NewResponseController(w).SetWriteDeadline`, verified to work) and must run every post-claim step detached from the request context.

Everything is inert until the owner sets all four settings; a partial or invalid set is a startup validation error. At join, a Turnstile group whose web captcha is not available (not configured, no Redis, no bot username) gets the group's math captcha (D-09). Without Redis nothing can pass, so the availability probe is a memoised `PING`, not the client-presence flag (`cache.IsRedisAvailable` only says a client object exists).

**Primary recommendation:** Build `alita/utils/captchaweb` (initData validator, Redis challenge store + limiters, `siteverify` client with a three-way outcome, embedded page, verify handler behind a `Backend` interface) first and test it with fakes; then extract `claimCaptchaPass`/`finishCaptchaPass` from `captchaVerifyCallback` (existing callback tests are the regression net), add the Turnstile branch to `SendCaptcha`, the reserved-answer guard to both callbacks, the CHECK migration, config validation with `RegisterSecret`, the readiness-gated route registration, locales, `/stats` section, docs and `AGENTS.md` in the same change set.

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Show widget, relay `initData` + Turnstile token + challenge, render the server's answer | Browser / Telegram WebView (Mini App) | API / Backend (serves the static page) | The page decides nothing (D-04); it only forwards and displays. |
| Authenticate who is verifying (signed `initData`) | API / Backend | Telegram (signs it) | Only the bot token holder can verify; never done in the page. |
| One-time challenge handle and rate limits | Redis | API / Backend (Lua scripts) | Shared by every replica (PLAT-01); short-lived; operational prefix outside `alita:cache:`. |
| Decide pass / fail / expire exactly once | Database (conditional delete of the `captcha_attempts` row) | Redis (only sheds load) | RowsAffected == 1 across replicas is the arbiter, as for the existing captcha. |
| Verify the Turnstile token | Cloudflare `siteverify` (called by the backend) | — | Tokens are forgeable client-side; the secret never leaves the server. |
| Mute, unmute, kick/ban/mute, delete challenge, greeting, held-message summary | Telegram Bot API (via the bot) | Database (muted-user schedule for durable unmute) | Same calls and same durable fallbacks as the callback path. |
| Challenge message and button | Telegram Bot API (`sendMessage` with a URL button) | — | `web_app` buttons exist only in private chats, so the group uses the direct link (gotgbot field comment, quoted below). |
| TLS, public hostname, path exposure | Owner's reverse proxy | Bot HTTP server (same port as `/health`, `/metrics`, `/webhook`) | Bot serves plain HTTP; the proxy must expose only `/captcha*` (Pitfall 12). |
| Page language | Browser (display only) | Server (embeds 7 string tables) | Nothing security-relevant depends on it (D-20). |
| Counters | In-process atomics (this replica) | Developer `/stats` | Same pattern as the raid AI counters (D-17). |

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| Go stdlib: `net/http`, `html/template`, `embed`, `crypto/hmac`, `crypto/sha256`, `crypto/rand`, `encoding/json`, `net/netip`, `net/url`, `time`, `context` | Go 1.26.0 | Routes, page templating with per-response nonce, `initData` HMAC, token generation, trusted-proxy parsing, `siteverify` client | D-12 (no framework, no build step); zero new dependency. [VERIFIED: go.mod `go 1.26.0` per CLAUDE.md stack list; `html/template` + `template.JS(json.Marshal(...))` behaviour executed in the scratchpad: the characters less-than, greater-than, ampersand and U+2028 are emitted as JSON unicode escapes inside the script block, and attribute values are HTML-escaped] |
| `github.com/redis/go-redis/v9` | v9.22.0 | Lua scripts (`redis.NewScript`) for consume, re-arm and fixed-window counters; `PING` probe | Already the project's Redis client; same pattern as `raidJoinScript`. [VERIFIED: CLAUDE.md stack list; scripts run on miniredis in the scratchpad] |
| `github.com/PaulSonOfLars/gotgbot/v2` | v2.0.0-rc.36.0.20260919140833-240296efadb4 | `InlineKeyboardButton.Url`, `RestrictChatMember`, `GetChatMemberWithContext`, `Bot.Username`, `Bot.Token` | In go.mod. [VERIFIED: go.mod line 7; module cache `gen_types.go` read] |
| `github.com/alicebob/miniredis/v2` | v2.39.0 | Redis + Lua in tests | Already the project's test Redis. [VERIFIED: ran the three scripts below on v2.39.0] |

### Supporting (in-repo seams to reuse, not new libraries)
| Seam | Where | Use |
|------|-------|-----|
| `CompleteCaptchaAttemptAtomic`, `ReleaseCaptchaAttemptAtomic`, `GetCaptchaAttemptByID` | `alita/db/captcha/repository.go:380-405`, `:407-422`, `:297-308` | The pass claim, the failure/timeout claim, the fresh attempt read. |
| `handleCaptchaTimeout`, `expireCaptchaAttempt`, `executeCaptchaFailureAction`, `restoreCaptchaPermissions` | `alita/modules/captcha.go:1259`, `:1231`, `:1368`, `:1404` | The failure path (reused as is for a failed solve), expiry, unmute with durable retry. |
| `startCaptchaLifecycleTask`, `StopCaptchaLifecycle` | `alita/modules/captcha.go:185-200`, `:174-183` | Tracks the post-pass completion work in the existing captcha drain, so no new shutdown budget is needed. |
| `chat_status.FetchBotMember` (live, tri-state; 8 s bound) | `alita/utils/chat_status/owner.go:71` | `/captchamode turnstile` rights refusal. A tighter variant is needed inside the verify route (Pattern 6). |
| `logredact.RegisterSecret`, `registerSecrets` | `alita/utils/logredact/logredact.go`, `alita/config/config.go:480-490` | Turnstile secret (PLAT-02). |
| `raidAIStatsSection` pattern | `alita/modules/devs.go:362-379` | Developer `/stats` counters section. |
| `cache.IsRedisAvailable`, `cache.GetRedisClient` | `alita/utils/cache/cache.go:188-198` | Redis client; note `IsRedisAvailable` is `redisClient != nil`, not liveness. |
| `i18n.GetManager().GetTranslator(lang).GetString(key)` | `alita/i18n/manager.go:54`, `translator.go:17` | Build the 7-language page string table (no i18n API change needed). |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| Own 40-line `initData` validator | `github.com/telegram-mini-apps/init-data-golang` | New dependency for trivial code and it would hide the two behaviours this phase must test (freshness, `signature` handling). Its README vectors are used as known-answer tests instead. `[ASSUMED]` package name, not installed, not recommended. |
| Consume-and-re-arm (D-03/D-21) | A verification lock that is released on retry | A lock has no gap in which the token is absent and survives a crash (lock TTL expires). Observable behaviour is identical; D-03/D-21 are locked, so the consume/re-arm design is specified and the crash gap is documented (Pitfall 9). |
| New package `captchaweb` | Code inside `httpserver` | `httpserver` imports `alita/db`; the logic must be testable without a database and `modules` must call `Issue` from `SendCaptcha`. A leaf with an injected `Backend` keeps `httpserver` generic (one new `Handle` method). |
| Page JS unit tests under a JS framework | `node --test` with the built-in `vm` module | Node 22.22.0 is installed here but is not a project dependency and not in `make test`; see Validation Architecture. |

**Installation:** none. No packages are added.

**Version verification:** not applicable (no new packages). Existing versions read from `go.mod` and the CLAUDE.md stack list. Node 22.22.0, npm 10.9.4 and `redis-server` 7.0.15 exist on this machine (`node --version`, `redis-server --version`), but none is required.

## Package Legitimacy Audit

> No external package is installed in this phase, so the `package-legitimacy` gate has nothing to check.

| Package | Registry | Age | Downloads | Source Repo | Verdict | Disposition |
|---------|----------|-----|-----------|-------------|---------|-------------|
| (none) | - | - | - | - | - | - |

**Packages removed due to [SLOP] verdict:** none
**Packages flagged as suspicious [SUS]:** none
`github.com/telegram-mini-apps/init-data-golang` and `@twa-dev/sdk` were *read* (a README, three Go files, one tarball unpacked into the scratchpad and never executed) to confirm protocol facts; neither is a dependency. Runtime assets the page loads are `https://telegram.org/js/telegram-web-app.js` and `https://challenges.cloudflare.com/turnstile/v0/api.js`; Cloudflare states the latter "must be fetched from the exact URL shown above. Proxying or caching this file will cause Turnstile to fail" [CITED: cloudflare-docs `turnstile/get-started/client-side-rendering/index.mdx`], so neither is vendored.

## Architecture Patterns

### System Architecture Diagram

```
Member joins (chat_member update / new_chat_members message)
   -> greetings.processSingleNewMember -> ProcessSingleJoin -> SendCaptcha            [alita/modules/captcha.go:1000]
        settings.CaptchaMode == "turnstile"?
          captchaweb.Available()  = configured && bot username && memoised Redis PING (<=5 s old)
            no  -> mode := "math"  (existing flow, nothing else changes)                       (D-09)
            yes -> create attempt row (answer = "ts1:" + 128 random bits)  -> mute (RestrictMember)
                -> token := 128 random bits (base64url, 22 chars) -> captchaweb.Issue (Redis hash, TTL = left + 5 min)
                -> sendMessage(text, ONE inline URL button https://t.me/<bot>/<short>?startapp=<token>)
                -> UpdateCaptchaAttemptMessageID -> scheduleCaptchaTimeout            (all existing)
                   (Issue / send failure -> rollbackCaptchaAttempt, existing)

Member taps the button -> Telegram client launches the Mini App -> GET /captcha (owner's proxy -> bot HTTP server)
   page: telegram-web-app.js (initData) + Turnstile api.js?render=explicit
         turnstile.render(sitekey, action "captcha_join", cData = hex(sha256("cd1:" + start_param)))
   widget callback(token) -> POST /captcha/verify {init_data, turnstile_token, challenge}   (exactly one request per widget token)

POST /captcha/verify (any replica)
   ready? -> in-flight semaphore -> per-IP window -> content-type/size/JSON -> initData HMAC + auth_date (5 min)
   -> per-user window -> signed start_param == challenge (when present) -> per-(token,user) window
   -> Redis consume-if-user  [0 unknown -> invalid | 2 wrong user -> not_for_you, NOTHING changed | 1 consumed]
   -> bot-rights preflight (3 s; missing/unknown -> re-arm the token, answer "retry")
   -> attempt row by ID: id/user/chat match + not past expiry + reserved "ts1:" answer   (else expired/invalid)
   -> siteverify (5 s):
        success + hostname + action + cdata match -> Backend.ClaimPass:
              CompleteCaptchaAttemptAtomic RowsAffected==1 -> unmute -> respond "passed"; finishCaptchaPass detached + tracked
              RowsAffected==0 -> lost to timeout/leave/disable -> Backend.Expire -> respond "expired" (+action)
        failed solve (invalid-input-response, timeout-or-duplicate, cdata mismatch) -> handleCaptchaTimeout -> "failed" (+action)
        retryable (transport, 5xx, internal-error, bad-request, secret errors, hostname/action mismatch) -> re-arm same token -> "retry"

Timeout on ANY replica: scheduleCaptchaTimeout timer | 5-min runCaptchaCleanupTick | startup runOrphanedCaptchaRecovery
   -> expireCaptchaAttempt -> handleCaptchaTimeout -> ReleaseCaptchaAttemptAtomic RowsAffected==1 -> delete challenge -> kick/ban/mute
```

### Recommended Project Structure
```
alita/utils/captchaweb/            # NEW package. No import of alita/db, alita/modules or alita/config (settings are injected).
├── settings.go                    # Settings, MiniAppLink(token), host/origin helpers
├── initdata.go                    # Validate(raw, key, now, maxAge) -> *InitData (user id, first name, language_code, start_param, auth_date)
├── store.go                       # Store: Issue / ConsumeIfUser / Rearm / MarkDone / Done (Redis hash + 3 Lua scripts)
├── limiter.go                     # fixed-window Redis counters + bounded in-flight semaphore
├── clientip.go                    # ClientIP(r, trusted) with trusted-proxy parsing (net/netip)
├── siteverify.go                  # Client.Verify -> Outcome{Pass, Failed, Retry} (+ ConfigError flag)
├── handler.go                     # Handler{Page, Verify, Health}, Backend interface, readiness flag, JSON answers
├── stats.go                       # atomic counters + Snapshot()
├── page/captcha.html              # //go:embed, html/template, nonce + i18n JSON + data-* attributes
└── *_test.go
alita/modules/
├── captcha_web.go                 # NEW: Backend implementation, Available(), issue helper, stats section, MarkCaptchaWebReady, finish tracker
├── captcha.go                     # EDIT: SendCaptcha branch, claim/finish extraction, callback guards, mode command, timeout stat hook
alita/utils/httpserver/server.go   # EDIT: one generic Handle(pattern, handler) method; log the /captcha endpoints in Start()
alita/config/config.go             # EDIT: 4 settings (+ optional CAPTCHA_WEB_TRUSTED_PROXIES), validation, registerSecrets
alita/db/models/captcha.go         # EDIT: CHECK tag adds 'turnstile'
alita/db/captcha/repository.go     # EDIT: SetCaptchaMode accepts turnstile
migrations/20261030120000_allow_turnstile_captcha_mode.sql
locales/*.yml (7)                  # NEW keys + 4 edited keys
docs/src/content/docs/self-hosting/captcha-web.md, api-reference/environment.md, sample.env, AGENTS.md
.planning/phases/08-turnstile-web-captcha/08-SPIKE.md   # owner checklist (deliverable of the phase)
```

### Pattern 1: Issue the challenge in `SendCaptcha` (CAPT-02, D-06, D-09)
**What:** Branch on a local `mode` variable instead of `settings.CaptchaMode`. Today the code is `if settings.CaptchaMode == "math" { ... } else { text }` [VERIFIED: alita/modules/captcha.go:1032-1062, quote: `if settings.CaptchaMode == "math" {` ... `} else {` ... `answer, imageBytes, options, err = generateTextCaptcha()`], so a stored `turnstile` would silently become a text captcha on a replica running old code (acceptable rollback behaviour, not a security hole). New code:
1. `mode := settings.CaptchaMode`. If `mode == "turnstile"` and `!captchaWebAvailable()` then `mode = "math"` (log at Info at most once a minute, no per-join error).
2. For `turnstile`: skip image/option generation; `answer = "ts1:" + hex(16 random bytes)`; the existing steps (`GetChatMember` validation, `EnsureUserInDb`, `EnsureChatInDb`, `CreateCaptchaAttemptPreMessageIfEnabled`, `RestrictMember`) run unchanged. `createCaptchaAttemptPreMessage` already deletes the member's previous attempt and stored messages in the same transaction and invalidates `alita:cache:captcha_pending:<chat>` after commit [VERIFIED: alita/db/captcha/repository.go:204-226, quote: `// Invalidate only after commit: inside the transaction a concurrent reader`], so a re-join invalidates the earlier challenge automatically (the old token then points at a vanished attempt ID).
3. After the mute: `token := captchaweb.NewToken()`; `captchaweb.Issue(ctx, token, Record{AttemptID, ChatID, UserID, ExpiresAt}, ttl)` with a 250 ms bound. On error, `rollbackCaptchaAttempt` (existing: deletes the row and unmutes) and return the error. Do not recurse into math from here; the up-front availability probe already narrowed that race to a single Redis blip, and the join error path already exists.
4. Message: `captcha_web_welcome` (key text and params in 08-UI-SPEC "Challenge message") with `i18n.TranslationParams{"first": formatting.MentionHtml(userID, userName), "number": settings.Timeout}` exactly like the existing `captcha_welcome_*` keys [VERIFIED: alita/modules/captcha.go:1135-1139]. Keyboard: `gotgbot.InlineKeyboardMarkup{InlineKeyboard: [][]gotgbot.InlineKeyboardButton{{{Text: <captcha_web_button>, Url: captchaweb.MiniAppLink(bot.Username, shortName, token)}}}}`. Send with `helpers.SendMessageWithErrorHandling` (plain message, not a photo). Keep the existing `UpdateCaptchaAttemptMessageID` + `scheduleCaptchaTimeout` tail.
5. The "keyboard empty" guard (`verifyCount == 0`, `captcha.go:1122-1129`) applies to math/text only.
6. Count `issued` in the stats after the message was sent.

**Availability (`captchaWebAvailable`)**: configured (all four settings) AND `bot.Username != ""` AND a memoised Redis `PING` under 250 ms (memo 5 s, in-memory `atomic`, so a raid of joins costs one probe per 5 s). `cache.IsRedisAvailable()` is *not* sufficient: it is `redisClient != nil` [VERIFIED: alita/utils/cache/cache.go:194-198, quote: `return redisClient != nil`], i.e. it says a client object exists, not that Redis answers.

**Direct-link facts (for the URL and the message):**
- Format `https://t.me/<bot_username>/<app_short_name>?startapp=<value>`; the app is created with BotFather `/newapp` and its URL edited later with `/myapps` > app > Edit link. [CITED: Telegram-Mini-Apps/telegram-apps `apps/docs/platform/creating-new-app.md`, `start-parameter.md`; consistent with a WebSearch summary of core.telegram.org/bots/webapps: "https://t.me/botusername/appname?startapp=command"]
- `startapp` characters: letters, digits, `_`, `-`. Length limit sources disagree (64 in one Mini Apps reference summary, 512 in the telegram-apps docs). A 22-character base64url token is inside both. [CITED: telegram-apps `start-parameter.md` ("Only the following characters are allowed: A-Z, a-z, 0-9, _ (underscore) and the - (minus)... up to 512 characters"); WebSearch summary of docs.telegram-mini-apps.com (older "64")]
- `&mode=compact` exists as a link parameter (the MTProto `messages.requestAppWebView` flag text: "Must be set if the mode parameter of the direct Mini App deep link is equal to compact") [CITED: WebSearch summary of core.telegram.org/method/messages.requestAppWebView]. The full list of `mode` values was not found. Recommendation: do not add it in v1; the owner tries it in the spike (UI-SPEC already treats it as optional).
- `InlineKeyboardButton.WebApp` is documented "Available only in private chats between a user and the bot" [VERIFIED: gotgbot `gen_types.go:3458` comment, quote: `Available only in private chats between a user and the bot.`], and `Url` as "HTTP or tg:// URL to be opened when the button is pressed" [VERIFIED: `gen_types.go:3454`]. So the group button must be a `Url` button to the direct link (D-06).
- Whether the page then receives the **tapper's** signed `initData`, whether `start_param` is inside it, and the first-launch confirmation dialog are `[ASSUMED]` from the platform's design (the launching client requests the web view under its own session; `chat_type` and `chat_instance` are "Returned only for applications opened by direct link" [CITED: telegram-apps `init-data.md`]). The spike confirms (Assumptions A1-A3).

### Pattern 2: `initData` validation (CAPT-03, CAPT-04, D-02)
**Algorithm (reproduced, not just cited).** [VERIFIED: executed in the scratchpad against the two published vectors in `telegram-mini-apps/init-data-golang` README and the telegram-apps docs: both `hash` values matched; the second vector matched only when the `signature` field is *included* in the data-check string]
1. Parse the raw string with `url.ParseQuery`; reject on parse error or a repeated key.
2. Take `hash` (64 lowercase hex), remove it; every other field, **including `signature` if present**, becomes `key=value` (value URL-decoded, the `user` JSON as the raw string); sort the lines bytewise; join with `\n`.
3. `secret_key = HMAC_SHA256(key = "WebAppData", message = bot_token)` (the token is the *message*, `WebAppData` the key); `expected = hex(HMAC_SHA256(key = secret_key, message = data_check_string))`.
4. Compare with `hmac.Equal` on decoded bytes (constant time). Only then read fields.
5. `auth_date` (unix seconds): reject older than 5 minutes (D-02) and more than 60 s in the future. Telegram's guidance is only "recommended to also check expiration" [CITED: telegram-apps `init-data.md` "Recommendation"; init-data-golang README]; 5 minutes is the project's decision (D-02).
6. `user` is a JSON object: require `id > 0`, `is_bot` not true; use `first_name` (for the mention) and `language_code` (not needed server side).
The third-party Ed25519 path (`signature` verified with Telegram's public key and `<bot_id>:WebAppData`) is deferred (CONTEXT). Note for the future: Telegram's `signature` base64 is unpadded and some languages reject it [CITED: telegram-apps `init-data.md` DANGER note].

Known-answer tests need an injectable clock because the published vectors are from 2022 and 2024; use the vectors' own `auth_date` as `now`. The vectors use public sample bot tokens; if `gosec` flags the literals, mark them `#nosec G101` with a comment saying they are the library's published sample.

```go
// Source: derived and checked against telegram-mini-apps/init-data-golang vectors (scratchpad run).
func deriveKey(botToken string) []byte {
	m := hmac.New(sha256.New, []byte("WebAppData"))
	m.Write([]byte(botToken))
	return m.Sum(nil)
}

func sign(key []byte, dataCheck string) []byte {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(dataCheck))
	return m.Sum(nil)
}
```

### Pattern 3: One shared pass function (CAPT-05, D-07)
**Problem.** The pass path lives inline in `captchaVerifyCallback` [VERIFIED: alita/modules/captcha.go:1499-1577]. Order today: read stored messages (before the claim, because the claim deletes them: `deleteCaptchaAttemptAtomic` deletes `StoredMessages` for the attempt in the same transaction, `repository.go:441`) -> `CompleteCaptchaAttemptAtomic(attempt.ID, user, chat, answer, messageID, refreshCount)` (conditional delete **and** a `captcha_muted_users` row `unmute_at = now` in the same transaction, so a crash after the claim still gets unmuted by the 5-minute worker) -> `restoreCaptchaPermissions` -> stored-messages summary (deleted after 30 s) -> delete challenge message -> success message (deleted after 5 s) -> `SendWelcomeMessage`.
"Replay of held messages" is, in this codebase, that **summary message** (the held messages were deleted at hold time; `handlePendingCaptchaMessage` stores type, content, file ID and deletes the original, `captcha.go:1918-1923`); the web path must produce exactly the same thing.

**Extraction (behaviour-preserving, two functions so the HTTP route can acknowledge early):**
```go
// captchaPassInput is everything the pass needs and nothing about how it was triggered.
type captchaPassInput struct {
	Bot       *gotgbot.Bot
	Chat      *gotgbot.Chat       // id, type, title, username (SendWelcomeMessage -> FormattingReplacer needs them)
	Attempt   *db.CaptchaAttempts // freshly read row (ID, Answer, MessageID, RefreshCount)
	UserID    int64
	FirstName string
	Ctx       *ext.Context        // callback: the real ctx; web: &ext.Context{EffectiveChat: chat} (as membershipDepsForJoin does)
}

// claimCaptchaPass reads the held messages, claims the attempt (RowsAffected == 1) and unmutes.
func claimCaptchaPass(in captchaPassInput) (stored []*db.StoredMessages, claimed bool, err error)
// finishCaptchaPass posts the held-message summary, deletes the challenge, posts the success note, runs the greeting.
func finishCaptchaPass(in captchaPassInput, stored []*db.StoredMessages)
```
- Callback: `claim` (false -> `captcha_expired_or_not_found`; unmute error -> delete challenge, `captcha_failed_verify`, exactly as today), then `finish`, then `query.Answer(captcha_verified_success_msg)`. These callback-only answers stay in the callback.
- Web: `claim` synchronously under the claimed-work rule (Pattern 6); then `finish` through a tracked detached task: `if !startCaptchaLifecycleTask(func(context.Context) { defer error_handling.RecoverFromPanic("captchaWebFinish", "captcha"); finishCaptchaPass(in, stored) }) { run it inline }`. `startCaptchaLifecycleTask` joins `captchaLifecycleWG`, which `StopCaptchaLifecycle` already waits on during shutdown [VERIFIED: captcha.go:174-200], so no new drain and no new shutdown budget is needed. The task ignores the lifecycle context (it must finish).
- Language: the callback path uses `lang.GetLanguage(ctx)`; for the web path build `&ext.Context{EffectiveChat: &gotgbot.Chat{Id: chatID}}` as `handleCaptchaTimeout` already does [VERIFIED: captcha.go:1321]. Thread: the existing challenge message is sent without a thread ID, so the callback's `ctx.EffectiveMessage.MessageThreadId` is the General topic; the web path passes no message and gets the same.
- `FirstName` comes from the **verified** `initData` `user.first_name`; the chat title for the greeting comes from `bot.GetChat` (already called inside `unmuteCaptchaUser`, `captcha.go:1392`); cache it between the two to avoid a second call.
- Regression net: `TestCaptchaVerifyCallbackCorrectAnswerUnmutesAndCleansAttempt` (`captcha_command_test.go:1180`), `...RestoresChatDefaultPermissions` (`:1219`), `...UnmuteFailureSchedulesRetryUnmute` (`:1291`) must pass unchanged after the extraction.

### Pattern 4: Exactly-once among pass, failed solve and timeout (CAPT-06, CAPT-07)
The arbiter is the conditional delete of the attempt row: both claims run `DELETE ... WHERE id = ? AND user_id = ? AND chat_id = ? ...` inside a transaction and report `RowsAffected > 0` as `deleted` [VERIFIED: alita/db/captcha/repository.go:424-453, quote: `if result.RowsAffected == 0 {` ... `deleted = true`]. The pass claim adds `AND answer = ? AND message_id = ? AND refresh_count = ? AND expires_at > ?` (`:392`); the failure/timeout claim (`ReleaseCaptchaAttemptAtomic`, `:407-422`) has none of the expiry predicate. So:

| Race | Winner | Loser behaviour |
|------|--------|-----------------|
| Pass vs timer / cleanup tick / recovery (any replica) | whichever transaction deletes the row first | Pass loser: no unmute, no greeting, answer `expired` + group action. Timeout loser: `handleCaptchaTimeout` returns `(false, false)` at `:1279-1282` and does nothing (existing). |
| Pass after `expires_at` but before any timer ran | pass claim fails (`expires_at > now` is false) | Call `expireCaptchaAttempt(bot, attempt)` right away (idempotent through its own claim), answer `expired` + action; do not wait up to 5 minutes for the cleanup tick. |
| Failed solve vs timeout | `handleCaptchaTimeout`'s claim | Failed-solve loser: nothing applied, answer `expired` + action. |
| Two requests with the same token | the Redis consume (one wins) | Loser: `invalid` (token gone) or the done-marker answer (Pattern 10). |
| Member leaves / admin `/captcha off` / re-join during the check | `leftMember` (`greetings.go`) or `disableCaptchaForChat` or the new attempt deletes the row | Claim fails -> `expired`; the old token never matches the new attempt ID (match on `attempt.ID == record.AttemptID`, not on "the user's current attempt"). |

`handleCaptchaTimeout(bot, chatID, userID, attemptID, fallbackMessageID, action)` is reused unchanged for a failed solve: it claims, deletes the challenge message, applies kick/ban/mute (kick = ban then unban so the member can rejoin, with an unban retry on failure), clears the fallback unmute for non-mute actions, posts the failure notice (deleted after 30 s) and returns `(claimed, applied)` [VERIFIED: captcha.go:1259-1336]. The group action comes from `captcha.GetCaptchaSettings(chatID).FailureAction` (default `kick`, `repository.go:41`).

### Pattern 5: `siteverify` client and the three-way outcome (CAPT-04, CAPT-06, D-09, D-21)
**Contract** [CITED: cloudflare-docs `turnstile/get-started/server-side-validation.mdx`]: `POST https://challenges.cloudflare.com/turnstile/v0/siteverify`; accepts JSON or form; request `secret` (required), `response` (required, max 2048 characters), `remoteip` (optional), `idempotency_key` (optional UUID for safe retries); always JSON back: `success`, `challenge_ts`, `hostname`, `error-codes[]`, `action`, `cdata`, `metadata.ephemeral_id` (Enterprise only). Tokens are valid 300 s and single use; a replay returns `timeout-or-duplicate`. Error codes: `missing-input-secret`, `invalid-input-secret`, `missing-input-response`, `invalid-input-response`, `bad-request`, `timeout-or-duplicate`, `internal-error`. The docs publish **no numeric rate limit** for `siteverify` (the only advice is "Rate limit to protect against validation flooding", i.e. on our side).
**Client rules:** JSON body with only `secret` and `response` (no `remoteip`: the address a reverse proxy shows is unreliable and the docs mark it optional; no `idempotency_key`: the page's Try again produces a fresh widget token, so a server-side retry buys nothing); `http.Client{Timeout: 5s}` (D-11); `CheckRedirect` returns `http.ErrUseLastResponse` (the secret is in the body); `io.LimitReader(resp.Body, 64<<10)`; decode JSON for any status below 500; endpoint in an unexported package variable with a test-only setter (the TypeSafe/Gemini pattern); never log or wrap the request, the response body or `*url.Error` (rebuild errors from the error class only).

**Classification (Outcome):**

| `siteverify` result | Outcome | Why |
|---|---|---|
| `success:true` and `hostname` == host of `CAPTCHA_WEB_BASE_URL` (case-insensitive, port stripped) and `action` == `captcha_join` and `cdata` == `hex(sha256("cd1:" + challenge))` | **Pass** | D-04 |
| `success:true`, `cdata` differs | **Failed** | A token minted for another challenge is not a valid solve (D-03). |
| `success:true`, `hostname` or `action` differs | **Retry** + loud Error log + `errors` counter | Our own constant or the owner's hostname setup is wrong; a member must not be kicked for the owner's misconfiguration (D-09). The timeout still applies. |
| `success:false`, codes include `invalid-input-response` or `timeout-or-duplicate` | **Failed** | The token is invalid, expired or already spent: not a connection problem (CAPT-06). |
| `success:false`, codes only `internal-error`, `bad-request`, `missing-input-response`, `missing-input-secret`, `invalid-input-secret`, unknown, or empty | **Retry** (+ Error log for the two secret codes) | Not the member's fault, or not provably theirs. Fail safe against kicks; garbage from an attacker only earns `retry` and is rate limited. |
| transport error, timeout, redirect, status >= 500, non-JSON, oversized body | **Retry** | D-09 |

Dummy keys (`1x00000000000000000000AA` sitekey, `1x0000000000000000000000000000000AA` secret; token `XXXX.DUMMY.TOKEN.XXXX`) are for documentation and the owner's smoke test only: Cloudflare says the success response then carries `"action":"test"` and `"cdata":"test-data"` and `hostname` `localhost`, so they can never satisfy the action/cdata/hostname checks (Pitfall 20) and must not appear in the suite's network calls (tests use `httptest`). [CITED: cloudflare-docs `turnstile/troubleshooting/testing.mdx`]

### Pattern 6: The verify route (CAPT-03, CAPT-04, D-11)
**Registration.** `Server.Handle(pattern, handler)` (new, generic) with Go 1.22 method patterns: `GET /captcha`, `POST /captcha/verify`, `GET /captcha/health`. Verified behaviour: `OPTIONS /captcha/verify` and `GET /captcha/verify` -> 405 with `Allow: POST`; `HEAD /captcha` -> 200; `GET /captcha/` -> 404 [VERIFIED: executed in the scratchpad], so no CORS preflight can succeed even by accident. Register only when the four settings are complete (D-10), before `Start()`; keep a **readiness gate**: in polling mode `httpServer.Start()` runs *before* `postInit` (`main.go:282` vs `:306`), so the routes would otherwise serve while `StartCaptchaLifecycle` has not run; `Verify` answers 503 `{"result":"retry"}` until `modules.MarkCaptchaWebReady()` is called at the end of `postInit` (`startCaptchaLifecycleTask` returns false before then).

**Request/response.**
- Request: `Content-Type: application/json` (parsed with `mime.ParseMediaType`; anything else 415), body capped at 16 KiB with `http.MaxBytesReader` (413 on overflow), `json.Decoder` with `DisallowUnknownFields` and a check that no second value follows: `{"init_data": string <=4096, "turnstile_token": string 1..2048, "challenge": string matching ^[A-Za-z0-9_-]{1,64}$}`. Malformed -> 400 `{"result":"invalid"}`.
- Response (always `Content-Type: application/json`, `Cache-Control: no-store`, `X-Content-Type-Options: nosniff`, no CORS headers): `{"result":"passed"}` | `{"result":"failed","action":"kick|ban|mute"}` | `{"result":"expired","action":"..."}` | `{"result":"invalid"}` | `{"result":"not_for_you"}` | `{"result":"retry"}`. No error text, ids or codes.
- Status: 200 for every decided result; 400/413/415 with `invalid`; 429 (+ `Retry-After`) with `retry`; 503 with `retry` (not ready, Redis down, over the in-flight cap). This matches the UI-SPEC table (transport statuses all map to `retry`; JSON `invalid` maps to `invalid`).
- **Claim won but the unmute call failed (known limitation).** `restoreCaptchaPermissions` has already queued a durable retry (`captcha_muted_users`, picked up by the 5-minute worker), but the attempt row is gone, so a second tap can only return `invalid`. The bot-rights preflight (step 6) makes this rare. When it still happens: log at Warn, count in `errors`, answer `{"result":"retry"}` (the approved page treats `retry` as a connection problem). The member is unmuted by the scheduled worker or by an admin (Open Question 3).

**Order of work (cheapest first; every refusal happens before the next step):**
1. Ready flag; non-blocking in-flight semaphore (64 slots; full -> 503 `retry`). Constant memory.
2. Per-IP window (`alita:captcha:rl:ip:<ip>`, 60 per 60 s). IP from `ClientIP(r, trustedProxies)`.
3. Content-type, size, JSON shape.
4. `initData` HMAC and freshness (Pattern 2). A bad hash or stale data -> `invalid` (never reveals which).
5. Per-user window (`alita:captcha:rl:u:<userID>`, 20 per 60 s); then if `initData` carries `start_param` it must equal `challenge` (else `invalid`); then per-(token,user) window (`alita:captcha:rl:t:<challenge>:<userID>`, 8 per 60 s). Keying the token window by the *verified user* matters: a window keyed by token alone lets any member exhaust the victim's budget by hammering the public token (Pitfall 2).
6. Consume-if-user (Pattern 10 script): `0` -> `invalid`; `2` -> `not_for_you` (nothing changed, nothing re-armed, nothing counted against the owner of the token); `1` -> continue holding the record and the remaining PTTL.
7. Bot-rights preflight, **after** the consume so the chat ID comes from the Redis record: a live `getChatMember` for the bot with a **3 s** context. `chat_status.FetchBotMember` uses an 8 s bound (`liveCheckTimeout = 8 * time.Second`, `owner.go:39`), too long for this route, so add a small exported variant that takes the bound. Missing Restrict right or an unknown answer -> re-arm the token and answer `retry` (nothing was claimed). Optional for the planner; without it the claim-then-unmute-failure branch above is hit more often.
8. Load the attempt by ID; require `ID`, `UserID`, `ChatID` match, `time.Now().Before(ExpiresAt)` and the reserved `ts1:` answer. Gone/expired -> `expired` + group action (the done-marker records it).
9. `siteverify` (Pattern 5). Then `Pass` / `Failed` / `Retry` handling as in the diagram. **Re-arm only on Retry** (and on the bot-rights preflight failure): `Rearm(token, record, remainingPTTL)`, a no-op if the key exists or the remaining time is not positive.
10. After a decision, write the done-marker (Pattern 10) so a lost response can be answered idempotently.

**Time budgets.** Client timeout in the page is 12 s (UI-SPEC). Server: request context 11 s for steps 1-9 up to the `siteverify` result; `siteverify` 5 s; once a claim has succeeded every further step runs on `context.WithoutCancel(r.Context())` with its own deadline, because a client that disconnects after the claim must not abandon the unmute (Pitfall 5).
**Write deadline.** The server is built with `ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second` [VERIFIED: alita/utils/httpserver/server.go:382-383]. A handler that overruns `WriteTimeout` produces an empty reply: reproduced in the scratchpad (`Post ... EOF`), and `http.NewResponseController(w).SetWriteDeadline(time.Now().Add(20 * time.Second))` at the top of `Verify` fixed it (response delivered). In `httptest.ResponseRecorder` the call returns an error that must be ignored.

**Client address and the proxy.** New optional setting `CAPTCHA_WEB_TRUSTED_PROXIES` (comma-separated CIDRs/IPs, default empty). `ClientIP`: if `RemoteAddr` is inside a trusted prefix, walk `X-Forwarded-For` from the right and return the first address not inside a trusted prefix (fall back to `RemoteAddr`); `X-Real-IP`/`CF-Connecting-IP` are not read. With an empty list the header is ignored (a client can set any `X-Forwarded-For`; trusting it would let one client mint unlimited per-IP buckets). Behind a proxy with the list empty every request shows the proxy's address, so one bucket would be shared by everyone: when the list is empty and `RemoteAddr` is loopback/private, **skip the per-IP bucket** (the per-user and per-token buckets and the in-flight cap still bound abuse) and log one Warn at startup telling the owner to set the list. Parse with `net/netip`.

### Pattern 7: The page, the CSP and the 7 locales (CAPT-08, D-12, D-13, D-20, D-23)
- `//go:embed page/captcha.html`; parsed once with `html/template`; one `Execute` per request with `{Nonce, SiteKey, Action, CDataPrefix, I18N template.JS}`. Nonce: 16 bytes from `crypto/rand`, base64 (std), fresh per response, put on the `<style>`, the i18n `<script type="application/json">`, the main `<script>` and both external `<script src>` tags. [VERIFIED: `html/template` accepts `nonce="{{.Nonce}}"`, escapes attribute values, and emits `template.JS(json.Marshal(...))` verbatim, with `<`, `>`, `&`, U+2028 already escaped by `json.Marshal`; run in the scratchpad]
- Cloudflare's CSP guidance: allow `script-src` and `frame-src` `https://challenges.cloudflare.com`; "Include your nonce in the `api.js` script tag and Turnstile will propagate it to dynamically loaded resources"; `connect-src 'self'` is needed only for pre-clearance. [CITED: cloudflare-docs `turnstile/reference/content-security-policy.mdx`] The UI-SPEC header value is consistent with it and stays: `default-src 'none'; script-src 'nonce-<n>' https://telegram.org https://challenges.cloudflare.com; style-src 'nonce-<n>'; frame-src https://challenges.cloudflare.com; connect-src 'self'; img-src 'self' data:; base-uri 'none'; form-action 'none'; frame-ancestors https://web.telegram.org https://webk.telegram.org https://webz.telegram.org`, plus `Cache-Control: no-store`, `Referrer-Policy: no-referrer`, `X-Content-Type-Options: nosniff`; **no** `X-Frame-Options` and no CORS headers. Two risks go to the spike: (a) `telegram-web-app.js` creates a `<style>` element and sets its `innerHTML` when Telegram Web posts a `set_custom_style` message (only from `https://web.telegram.org`) [VERIFIED: `@twa-dev/sdk` 8.0.2 `src/telegram-web-apps.js` lines 30-50], which a nonce-only `style-src` blocks (cosmetic: a console violation); the page's own styling is unaffected. (b) `Referrer-Policy: no-referrer` is not mentioned in Cloudflare's docs for Turnstile; if the widget reports `110200` (domain not authorized) in the real client, relax to `strict-origin` first.
- **Locales.** At startup (after `i18n.GetManager().Initialize`, `main.go:86`) build `map[lang]map[key]string` for `en es fr hi id pt ru` by calling `GetTranslator(lang)` and `GetString(key)` with no params for each of the 26 `captcha_web_*` page keys; `GetString` falls back to English per key when missing [VERIFIED: alita/i18n/translator.go:17-37]. Inject as one JSON block. All 7 are supported by the widget itself (`en`, `es`, `fr`, `hi`, `id`, `pt` -> Portuguese (Brazil), `ru` are two-letter codes in Cloudflare's table) [CITED: cloudflare-docs `turnstile/reference/supported-languages.mdx`], so `data-turnstile-lang` can be the resolved code; keep `auto` for anything else.
- **Widget parameters (corrections to the UI-SPEC from Cloudflare's reference table).** [CITED: cloudflare-docs `turnstile/get-started/client-side-rendering/widget-configurations.mdx`] The JavaScript render parameter is **`cData`** (camelCase; `data-cdata` is the attribute form) and the siteverify response field is lowercase `cdata`. `action`: at most 32 characters, `[A-Za-z0-9_-]`; `cData`: at most 255, same set (a 64-character hex digest fits). `theme`: `auto|light|dark`; `size`: `normal|flexible|compact` (flexible is "100% (min: 300px)", compact 150x140); `appearance`: `always|execute|interaction-only`; `retry`: `auto|never`; `refresh-expired` and `refresh-timeout`: `auto|manual|never`; `language`: `auto`, two-letter or four-letter code; callbacks `callback`, `error-callback`, `expired-callback`, `timeout-callback`, `before-interactive-callback`, `after-interactive-callback`, `unsupported-callback`. API: `turnstile.render`, `reset`, `remove`, `getResponse`, `isExpired`, `execute`, `ready`; script URL `https://challenges.cloudflare.com/turnstile/v0/api.js?render=explicit` with optional `&onload=<global fn>`. `error-callback` receives an error code; per Cloudflare's client-side error table `110100`, `110110`, `110200`, `200100`, `400020`, `400021`, `400070` are *not* retryable (sitekey/domain/config) while `110600`, `110620`, `200500`, `300*`, `600*` are [CITED: `turnstile/troubleshooting/client-side-errors/error-codes.mdx`]; the approved UI maps all to `retry`, so an owner misconfiguration shows an endless Try again (Pitfall 19).
- **WebView notes.** Cloudflare: Turnstile in a native WebView needs JavaScript, DOM storage, access to `challenges.cloudflare.com` and "about:blank and about:srcdoc"; "Custom or heavily modified browser engines and embedded browsers" are listed as *limited support* [CITED: cloudflare-docs `turnstile/get-started/mobile-implementation.mdx`, `cloudflare-challenges/reference/supported-browsers.mdx`]. That is the central risk of the whole phase and is why the spike exists.
- **Telegram web-app script facts used by the page** [VERIFIED: `@twa-dev/sdk` 8.0.2 `src/telegram-web-apps.js`, a copy of `telegram-web-app.js` at Bot API 8.0 level]: it reads only `tgWebAppData`, `tgWebAppThemeParams`, `tgWebAppVersion`, `tgWebAppPlatform` from `location.hash` and mirrors them into `sessionStorage` (so a reload keeps them); `initDataUnsafe` is the parsed `tgWebAppData` query string (JSON-valued fields parsed) so `initDataUnsafe.start_param` exists only if the signed string contains `start_param`; `WebApp.close()` posts `web_app_close` and needs nothing from the bot; theme variables are written as `--tg-theme-*` CSS custom properties via CSSOM (allowed under a nonce-only `style-src`); `isVersionAtLeast`, `disableVerticalSwipes`, `ready`, `colorScheme` exist. The telegram-apps docs state the start parameter "is not included in the location hash. Instead, it can be found in the URL query parameters" as `tgWebAppStartParam` [CITED: `apps/docs/platform/start-parameter.md`]. **Page rule:** read `initDataUnsafe.start_param`, and if empty read `new URLSearchParams(location.search).get('tgWebAppStartParam')`; the `challenge` field of the POST is whichever was found. The server then treats a *signed* `start_param` as authoritative only if present (Pattern 6 step 5) and the cdata is computed from `challenge` in both cases (D-19 unchanged).

### Pattern 8: Command, config and wiring (CAPT-01, PLAT-02, D-05, D-11)
- **`/captchamode`**: `captchaModeCommand` currently accepts `mode == "math" || mode == "text"` [VERIFIED: alita/modules/captcha.go:600-606] and builds the description with `if mode == "text" { modeDesc, _ = tr.GetString("captcha_mode_text_desc") }` [VERIFIED: `:624-627`]. Add `turnstile` with `captcha_mode_turnstile_desc`. Before `captcha.SetCaptchaMode`, for `turnstile` only, refuse in this order (no write on refusal): (1) `!captchaweb configured` -> `captcha_web_refuse_not_configured`; (2) Redis probe fails -> `captcha_web_refuse_no_redis`; (3) live `FetchBotMember`: not `administrator` with `CanRestrictMembers` (or unknown) -> `captcha_web_refuse_no_rights` (pattern: `lockdown_start.go:94-97`). The admin gate stays (`captchaAdminGate(bot, ctx, false)`, `captcha.go:588`): the per-group admin check is never skipped. `captcha.SetCaptchaMode` accepts the third value (`repository.go:73-76`, currently `if mode != "math" && mode != "text" { return ErrInvalidCaptchaMode }`).
- **Config.** `Config` gains `CaptchaWebBaseURL`, `TelegramMiniAppShortName`, `TurnstileSiteKey`, `TurnstileSecretKey`, optional `CaptchaWebTrustedProxies`; env names exactly as D-11. `ValidateConfig` (`config.go:212`): all four empty = feature off; some but not all = error naming the missing variables (never a value); `CAPTCHA_WEB_BASE_URL` must parse, scheme `https`, non-empty host, no userinfo/query/fragment, path empty or `/` (the routes are fixed at `/captcha*`; the owner registers `<base>/captcha` with BotFather); the short name `^[A-Za-z0-9_]{1,64}$` (BotFather's own length rules were not verified, so validate loosely; `[ASSUMED]`). Use stdlib only: `alita/config` must not import a package that reaches `alita/db` (db imports config), which is why the Gemini client is a leaf. `registerSecrets` adds `cfg.TurnstileSecretKey` [VERIFIED: `config.go:480-490`, quote: `cfg.TypeSafeAPIKey,` `cfg.GeminiAPIKey,`]. The site key is public and is not registered. No structural redaction pattern for Turnstile secrets is recommended: the real secret shape is not documented and the public site key has the same prefix style (`[ASSUMED]`), so exact-value registration plus the "never pass inputs to a logger" rule and a test are the controls.
- **Wiring (`main.go`).** After `httpServer.RegisterDBMetrics()` (`:238`): if the settings are complete, build the handler with the bot `b`, the injected `Backend` from `modules`, the Redis client (`cache.GetRedisClient()`), the string tables, and call `httpServer.Handle(...)` for the three routes. At the end of `postInit` call `modules.MarkCaptchaWebReady()`. Add `/captcha`, `/captcha/verify`, `/captcha/health` to the `endpoints` log list in `Start()`. In the startup log, if `MetricsAuthToken == ""` while the web captcha is enabled, add one Warn that the proxy must forward only `/captcha*` (Pitfall 12).
- **`GET /captcha/health`**: `200 {"status":"ok"}` when ready and Redis answers a 250 ms `PING`, else `503 {"status":"unavailable"}`; no ids, no config values, no secret names; `Cache-Control: no-store`. It lets the owner confirm that the public hostname reaches the bot (D-15).

### Pattern 9: Migration, model and harness (CAPT-01, D-05)
- Constraint name in PostgreSQL: `chk_captcha_mode` on `captcha_settings`, created `NOT VALID` by the migration `20260412000000_add_missing_check_constraints.sql` [VERIFIED: migrations/20260412000000_add_missing_check_constraints.sql:24-26, quote: `ADD CONSTRAINT chk_captcha_mode` / `CHECK (captcha_mode IN ('math', 'text')) NOT VALID;`; the same file drops it first at `:13`: `ALTER TABLE captcha_settings DROP CONSTRAINT IF EXISTS chk_captcha_mode;`]. The model tag is `check:chk_captcha_mode,captcha_mode IN ('math','text')` [VERIFIED: alita/db/models/captcha.go:9]; GORM builds the same CHECK from the tag in the SQLite harness, so the tag change is what makes SQLite tests accept `turnstile`.
- New file `migrations/20261030120000_allow_turnstile_captcha_mode.sql` (greater than the latest, `20261029120000`), statements in this order, no `BEGIN`/`COMMIT`:
```sql
ALTER TABLE captcha_settings DROP CONSTRAINT IF EXISTS chk_captcha_mode;
ALTER TABLE captcha_settings
ADD CONSTRAINT chk_captcha_mode
CHECK (captcha_mode IN ('math', 'text', 'turnstile')) NOT VALID;
```
`NOT VALID` mirrors the applied constraint (no validation scan of legacy rows, so it cannot abort startup) and still enforces every new or updated row. **The `DROP ... IF EXISTS` must come first**: both appliers rewrite `ADD CONSTRAINT` into a `DO` block that swallows `duplicate_object` [VERIFIED: alita/db/migrations/runner.go:533-575 and scripts/migrate_psql.sh:75], so an ADD without the preceding DROP would silently do nothing and leave the old constraint rejecting `turnstile` in production. No change to either applier is needed (plain statements). Keep the ADD in the multi-line form shown so the shell applier's `^ALTER\s+TABLE ... ADD CONSTRAINT` regex matches the same way as for the old file.
- Update the model tag, `SetCaptchaMode`, and the existing constraint test list (`alita/db/constraints_test.go:TestCaptchaSettingsConstraint_ValidModes`, `validModes := []string{"math", "text"}`). The harness `AutoMigrate` lists already contain `CaptchaSettings` (`alita/modules/test_harness_test.go:344` and the db test mains), so no list change. Backup/export/import of `captcha_settings` carries the string through unchanged (`backup.go:626-637`); an imported `turnstile` on a bot without the web settings is handled by the join-time fallback.
- Add a PostgreSQL-only guarantee test in `./alita/db/migrations` (package already in `make test-postgres-integrity`'s package list, and `postgresSkip` already allows skips there): `TestCaptchaModeTurnstileConstraint` (insert `turnstile` accepted, `bogus` rejected, using the migration file executed through `cleanSupabaseSQL` + `splitSQLStatements` like `TestRaidDetectionCarryOver`); add its name to the `-run` regex. Do not run PostgreSQL locally.

### Pattern 10: Redis keys and scripts (PLAT-01, D-01, D-16, D-21)
All keys sit outside `alita:cache:` (survive `CLEAR_CACHE_ON_STARTUP`), new family `alita:captcha:` (the existing `alita:captcha:refresh:cooldown:*` already lives under it, `captcha.go:1693`).

| Key | Type / TTL | Purpose |
|-----|-----------|---------|
| `alita:captcha:ch:<token>` | hash `u` (user id), `a` (attempt id), `c` (chat id), `x` (attempt expiry, unix ms); TTL = time left to the attempt's `ExpiresAt` + 300 s | The challenge handle (D-01). Token = 16 bytes `crypto/rand`, `base64.RawURLEncoding` (22 characters of `[A-Za-z0-9_-]`). Hash fields, not JSON, so the Lua compare is on the exact decimal string (Lua numbers lose exactness above 14 digits). |
| `alita:captcha:done:<token>` | string `<user>:<result>[:<action>]`, TTL 10 min | Optional idempotency marker written after a decision: a later request from the same verified user gets the same answer with no side effect (covers a lost response); another user gets `not_for_you`. |
| `alita:captcha:rl:ip:<ip>` / `rl:u:<user>` / `rl:t:<token>:<user>` | integer, fixed window, `PEXPIRE` 60 s on first increment | Limiters (60 / 20 / 8 per window). Redis only, no in-process map, so nothing to evict. |

Scripts (run and passing on miniredis v2.39.0 [VERIFIED: scratchpad run; wrong user returned `[2]` and left the key, the right user returned `[1, ttl, fields]` and deleted it, a second consume returned `[0]`, re-arm returned 1 then 0 while the key existed]):
```lua
-- consume: KEYS[1]=challenge hash, ARGV[1]=verified user id. {0}=unknown, {2}=wrong user (nothing changed), {1,pttl,fields}=consumed
local u = redis.call('HGET', KEYS[1], 'u')
if not u then return {0} end
if u ~= ARGV[1] then return {2} end
local all = redis.call('HGETALL', KEYS[1])
local ttl = redis.call('PTTL', KEYS[1])
redis.call('DEL', KEYS[1])
return {1, ttl, all}

-- rearm: KEYS[1]=challenge hash, ARGV: 1 remaining ms, 2 user, 3 attempt, 4 chat, 5 expiry ms. Only if the key is absent and time remains.
if redis.call('EXISTS', KEYS[1]) == 1 then return 0 end
if tonumber(ARGV[1]) <= 0 then return 0 end
redis.call('HSET', KEYS[1], 'u', ARGV[2], 'a', ARGV[3], 'c', ARGV[4], 'x', ARGV[5])
redis.call('PEXPIRE', KEYS[1], ARGV[1])
return 1

-- window: KEYS[1]=counter, ARGV[1]=window ms. Returns the count after this call.
local n = redis.call('INCR', KEYS[1])
if n == 1 then redis.call('PEXPIRE', KEYS[1], ARGV[1]) end
return n
```
The wrong-user branch is the point of "compare-and-delete" in D-03: a plain `GETDEL` would let any member who opens the public link burn the target's challenge (Pitfall 2). A replica crash between consume and re-arm loses the challenge until the group's timeout (bounded, documented, Pitfall 9). A Redis flush loses all open challenges (members time out and are kicked; they can rejoin).

### Pattern 11: Shutdown, stats and docs touch points (D-17, PLAT-01)
- **Shutdown.** `httpServer.Stop()` is registered last in `main.go` (`:257` / `:277`), so LIFO runs it first: in-flight verify requests finish (the server's own 10 s `Shutdown`). Tracked post-pass tasks join `captchaLifecycleWG` and are awaited by `StopCaptchaLifecycle`, which runs later and before the DB close (`main.go:196-200`). No new `RegisterXxxDrain` and no extra shutdown time, so the worst-case 60 s budget in AGENTS.md is not made tighter. A task started after the lifecycle stopped runs inline (`startCaptchaLifecycleTask` returns false).
- **Stats.** A section like `raidAIStatsSection` [VERIFIED: alita/modules/devs.go:362-379] appended in `getStats` (`devs.go:407`): "Web captcha (this replica, since its last restart): issued, passed, failed, expired, invalid, retry, limited, errors". Counters are `atomic.Int64`: `issued` in `SendCaptcha`, `passed`/`failed`/`invalid`/`retry`/`limited` in the route, `expired` where `handleCaptchaTimeout` claims a `ts1:` attempt or the route answers `expired`, `errors` for secret errors, hostname/action mismatches and backend failures. Numbers only: no ids, tokens, `initData` or Cloudflare text. English text is fine (developer-only, like the raid section).
- **Docs generator.** `make check-docs` copies `docs/src/content/docs`, regenerates, and diffs [VERIFIED: Makefile `check-docs`]; hand-written new pages persist untouched. `docs/src/content/docs/commands/captcha/index.md` has no `MANUALLY MAINTAINED` marker, so it is regenerated from `locales/en.yml` (`captcha_help_msg`): run `make generate-docs` after editing the key. `api-reference/environment.md` carries the marker and is edited by hand.

### Anti-Patterns to Avoid
- **Letting the page decide anything** (D-04): no state is rendered from a local "success"; `passed` appears only from the server's JSON.
- **A plain `GETDEL` for the challenge token:** any member could burn another member's challenge (Pitfall 2).
- **Reading the user's current attempt by `(user, chat)` instead of by the attempt ID in the token record:** a re-join would let an old token act on the new attempt.
- **Putting `initData`, the Turnstile token, the challenge token, the secret or Cloudflare's body into any log, span attribute or error string.**
- **Using the request context for post-claim work, or the default 10 s write timeout for the verify handler** (Pitfall 5).
- **A per-IP map in process memory:** unbounded growth; use the Redis windows.
- **Adding a second list of "Turnstile attempts":** the reserved answer prefix on the existing row is the only marker; no schema change.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Constant-time comparison of the `initData` hash | `==` or `bytes.Equal` on hex strings | `hmac.Equal` on decoded bytes | Timing. |
| Random token / reserved answer | `math/rand`, timestamps, counters | `crypto/rand` (16 bytes), `base64.RawURLEncoding` | 128-bit unguessable (D-01); the existing `secureIntn` is for integers only. |
| Embedding JSON and values in HTML | string concatenation / `fmt.Sprintf` into HTML | `html/template` with `template.JS(json.Marshal(...))` | Verified safe against `</script>` and attribute breakout. |
| One-time token, re-arm, windows | Get-then-Delete in Go, `SETNX` plus separate reads | the three Lua scripts above | Atomic across replicas; the wrong-user branch must not mutate. |
| Pass / fail / timeout arbitration | A Redis lock, an in-memory set | `CompleteCaptchaAttemptAtomic` / `ReleaseCaptchaAttemptAtomic` (`RowsAffected`) | Already the project's exactly-once rule; survives restarts and replicas. |
| Kick that lets the member rejoin, ban, mute with 24 h unmute | New Telegram calls | `handleCaptchaTimeout` / `executeCaptchaFailureAction` | Includes the unban retry, the durable unmute row and the notice cleanup. |
| Unmute with durable retry | `RestrictChatMember` alone | `restoreCaptchaPermissions` (writes `captcha_muted_users` on failure) | The 5-minute worker finishes what a failed call left. |
| Trusted-proxy address parsing | Splitting `X-Forwarded-For` by hand and trusting the first entry | `net/netip` prefixes + rightmost-untrusted walk | Spoofing. |
| Body size, content type | Reading all then checking | `http.MaxBytesReader`, `mime.ParseMediaType` | Memory and 415/413 behaviour. |
| Secret redaction | Custom scrubbing | `logredact.RegisterSecret` | Applies to every log field (PLAT-02). |
| 7-language strings | A second translation mechanism | `i18n` translators + locale files | PLAT-03; the parity test replaces `make check-translations` for dynamic keys. |
| Turnstile server-side check | A Cloudflare SDK | ~60 lines over `net/http` | One endpoint; the outcome mapping is project policy. |

**Key insight:** every dangerous operation in this phase already has a hardened, tested implementation in the repo (claim, kick, unmute, durable retry, secrets). The new code is the thin trust boundary (verify `initData`, hold a one-time handle, ask Cloudflare) and must call the existing machinery instead of copying it, which is also what stops the callback path and the web path from drifting.

## Common Pitfalls

### Pitfall 1: Forged callbacks defeat a Turnstile attempt (HIGH)
**What goes wrong:** A Turnstile attempt is an ordinary `captcha_attempts` row. Callback data is not authenticated by Telegram (a modified client or a userbot can call `getBotCallbackAnswer` with any bytes), and the existing handlers only check that the presser equals the `u` field. `captchaRefreshCallback` then generates a math or text image captcha, sends it and rewrites the row's `answer` and `message_id` [VERIFIED: captcha.go:1737-1800, quote: `newAnswer, imageBytes, options, genErr = generateMathImageCaptcha()` ... `captcha.UpdateCaptchaAttemptOnRefreshByID(`], which converts the attempt into a captcha the attacker can solve. `captchaVerifyCallback` passes whenever `selectedAnswer == attempt.Answer` (`:1499`), so a guessable stored answer is a bypass.
**How to avoid:** (1) store `ts1:` + 128 random bits as the answer; (2) at the top of both callbacks, after loading the attempt, refuse any attempt whose answer has the `ts1:` prefix (answer with the existing `captcha_attempt_not_valid` text, change nothing); (3) the web pass uses the freshly read `attempt.Answer` internally, never a client value. Rolling-deploy caveat: an old replica has no guard until it is replaced; accepted, like the other rolling-deploy caveats in AGENTS.md.
**Warning signs / test:** `TestCaptchaWebAttemptRefusesCallbacks` sends hand-built `captcha_verify` and `captcha_refresh` data for a Turnstile attempt and asserts: row unchanged, nobody unmuted, no photo sent, refusal answered.

### Pitfall 2: The challenge token is public (HIGH)
**What goes wrong:** The token is in a URL button every group member can read. (a) If any `verify` call consumed it, a griefing member would kill the target's challenge. (b) If the per-token limiter is keyed by token alone, the griefer exhausts the victim's budget.
**How to avoid:** the consume script compares the verified user first and changes nothing for another user (`{2}`); limiter key is `rl:t:<token>:<verified user>`; wrong-user traffic is bounded by the per-IP and per-user (the attacker's own) windows. The barrier is the signed `user.id`, never the secrecy of the token.
**Warning signs:** a test where user B posts 100 verifies with A's token and A still passes afterwards.

### Pitfall 3: `start_param` may not be in the signed `initData`
**What goes wrong:** The telegram-apps docs say both "duplicated in the init data's start_param" (tip on the start-parameter page) and "start_param ... returned only for Mini Apps opened through the attachment menu" (parameter table) [CITED: `apps/docs/platform/start-parameter.md`, `init-data.md`]; a search summary of Telegram's own page says it appears in `initData`. If it is not signed, `initDataUnsafe.start_param` is empty and a page that only reads that goes straight to `invalid` in the real client.
**How to avoid:** the two-source read in Pattern 7 (URL query `tgWebAppStartParam` as the fallback) and the server rule "signed value wins when present". Because the token is a handle and not a credential, an unsigned `challenge` is safe: a wrong token still fails the `user.id` comparison and the `cdata` binding is computed from the same value the page used.
**Spike:** the first checklist item records which of the two the client delivers.

### Pitfall 4: `initData` freshness and clock handling
**What goes wrong:** `auth_date` is when the Mini App was opened, not when the member taps the widget. A member who opens the sheet and waits more than 5 minutes (possible with a 10-minute group limit) gets `invalid`. Skewed clocks reject fresh data.
**How to avoid:** accept up to 60 s in the future; 5 minutes is the locked rule (D-02); the `invalid` copy tells the member to open the link again, which gives fresh `initData`. Known-answer tests inject `now`.

### Pitfall 5: Work after the claim must not depend on the request
**What goes wrong:** The server `WriteTimeout` is 10 s [VERIFIED: server.go:383]. `siteverify` (5 s) plus Redis plus Telegram calls can exceed it; the reply is then empty (reproduced). A client that disconnects after the claim cancels `r.Context()`, which would abandon the unmute and leave a claimed attempt with a muted member.
**How to avoid:** `SetWriteDeadline` at the top of `Verify`; after `RowsAffected == 1` use `context.WithoutCancel(r.Context())` with explicit deadlines; run `finishCaptchaPass` as a tracked detached task; the claim already writes the durable unmute row (`createMutedUser` in the same transaction) as the backstop.

### Pitfall 6: Turnstile token replay, `cdata` and the hostname behind a proxy
**What goes wrong:** (a) Tokens are single use and valid 300 s; a replay returns `timeout-or-duplicate`. (b) `cdata` binds a token to a challenge only against *third parties*; the joining member controls their own page. (c) `siteverify`'s `hostname` is where the widget was served (the page's hostname), not the `Host` header the bot sees, so a reverse proxy does not change it; but if the Mini App URL registered in BotFather uses a different hostname than `CAPTCHA_WEB_BASE_URL`, every check mismatches.
**How to avoid:** compare `hostname` to the host of `CAPTCHA_WEB_BASE_URL`; classify hostname/action mismatch as a configuration `retry` with a loud Error log (never a kick); cdata mismatch as `Failed`. State in the guide: the BotFather URL host, the base URL host and the Turnstile widget's hostname list must agree (Cloudflare: a hostname also authorizes all its subdomains, no scheme/port/path) [CITED: cloudflare-docs `turnstile/additional-configuration/hostname-management/index.mdx`].

### Pitfall 7: Kicking people for the owner's or Cloudflare's errors
**What goes wrong:** Treating every `success:false` as a failed solve kicks members when the secret is wrong or Cloudflare has an outage.
**How to avoid:** the Pattern 5 table: only `invalid-input-response`, `timeout-or-duplicate` and a cdata mismatch fail the member; everything else is `retry`, re-arms the token, counts in `errors`, and logs secret errors at Error without the secret. The group's time limit still removes real bots.

### Pitfall 8: Logging and leaking inputs
**What goes wrong:** `initData` carries the user's name, id and language plus a hash; the Turnstile token and challenge token are credentials for one request; Go errors from `net/http` embed URLs; OTel span attributes (the webhook handler sets some) could capture them.
**How to avoid:** log only outcome codes and counters; never pass a request body, header, token or Cloudflare response to a logger or `fmt.Errorf("%w")`; rebuild transport errors from the error class; answers are fixed JSON with no detail; `GET /captcha/health` returns no config. `TestCaptchaWebNeverLogsInputs` runs every outcome with a capturing logrus hook (redaction hook off, to prove the code is clean) and a fake Cloudflare that echoes the secret in a 500 body.

### Pitfall 9: The consume/re-arm gap and Redis loss
**What goes wrong:** Between consume and re-arm the key is absent; a replica crash there loses the challenge. A Redis flush or outage loses every open challenge (and makes new joins fall back to math).
**How to avoid:** accepted and bounded by the group's time limit (the member is kicked and can rejoin); documented in the guide and `AGENTS.md`. The alternative (a lock released on retry) was rejected only because D-03/D-21 are locked.

### Pitfall 10: Client IP and proxy collapse
**What goes wrong:** Trusting `X-Forwarded-For` blindly lets one client invent unlimited buckets; ignoring it behind a proxy collapses everyone into one bucket (a DoS lever on the legitimate users).
**How to avoid:** `CAPTCHA_WEB_TRUSTED_PROXIES`, rightmost-untrusted walk, and no per-IP bucket when the peer is loopback/private and no proxies are configured (Pattern 6). Tests: spoofed header from an untrusted peer ignored; trusted peer with a chain resolves the right client.

### Pitfall 11: Slow, large or numerous requests; limiter memory
**What goes wrong:** Slowloris-style bodies, big bodies, thousands of buckets in process memory.
**How to avoid:** `ReadTimeout` 10 s already bounds slow bodies; `MaxBytesReader` 16 KiB; in-flight cap 64; all windows in Redis with a 60 s TTL; JSON `DisallowUnknownFields`. Per-request memory is bounded by the cap.

### Pitfall 12: The captcha routes share a port with `/metrics`, `/db_metrics`, `/debug/pprof`, `/webhook`
**What goes wrong:** An owner who points the public hostname at the whole bot port exposes `/metrics` and `/db_metrics` (open unless `METRICS_AUTH_TOKEN` is set, `server.go:172-196`: "METRICS_AUTH_TOKEN is not set — /metrics is unauthenticated"), `/health` (version, uptime), pprof if enabled, and the webhook path.
**How to avoid:** the guide's proxy examples forward only `/captcha` and `/captcha/`; startup Warn when the metrics token is empty; see Question for the Owner 1 for an in-code gate.

### Pitfall 13: Polling mode starts the HTTP server before modules are loaded
**What goes wrong:** `httpServer.Start()` runs before `postInit` in polling mode (`main.go:282` then `:306`); a verify request in that window finds no captcha lifecycle.
**How to avoid:** the readiness flag set by `MarkCaptchaWebReady()` (Pattern 6); `Verify` answers 503 `retry` until then. Webhook mode calls `postInit` first (`:262` then `:264`).

### Pitfall 14: Member and group state changes mid-challenge
Joins twice (old token then points at a vanished attempt: answer `expired`; the new attempt has a new token); leaves before passing (`leftMember` deletes the row and message, `greetings.go`; a later tap answers `expired`, and the "you were removed" body is slightly inaccurate for a member who left on their own: accepted); admin disables captcha (`disableCaptchaForChat` releases every attempt and restores users); approved user (`expireCaptchaAttempt` releases an approved member at expiry; a web pass for them simply passes); admin or approved member joining (join flow decides before `SendCaptcha`; muting an admin fails and rolls back as today); the bot lost the Restrict right (rights preflight -> `retry`; claim-then-unmute failure is durable through `captcha_muted_users`); challenge message edited or deleted by an admin (the timeout and cleanup delete tolerate a missing message through `DeleteMessageWithErrorHandling`; the member times out and is kicked).

### Pitfall 15: Migration: `ADD CONSTRAINT` is swallowed when it already exists
Covered in Pattern 9: `DROP CONSTRAINT IF EXISTS` first, then `ADD ... NOT VALID`. A migration that only adds would apply "successfully" and change nothing.

### Pitfall 16: `make check-translations` is blind to the page keys
It extracts only literal `GetString("...")` arguments [VERIFIED: scripts/check_translations/main.go `extractKeysFromFile`: `lit, ok := call.Args[0].(*ast.BasicLit)`], so 26 page keys read from a Go slice are never checked. Add a locale test like `lockdown_locale_test.go` (parity with `en.yml` in both directions, non-empty values, no placeholders or format verbs, `<` and `&` allowed only where the UI-SPEC says, maximum lengths from the UI-SPEC tables, the two kick bodies the only rejoin promise). The challenge-message and refusal keys are read with literal `GetString` calls, so the existing check covers those.

### Pitfall 17: Stale mode on other replicas, rolling deploys
`captcha_settings` is read through the cache with a 10 s local layer and is not in `skipLocal` [VERIFIED: alita/db/cache/local.go:55-71], so another replica can serve the old mode for up to `CACHE_LOCAL_TTL` seconds after `/captchamode`: accepted (same as the other modes). An old-build replica treats `turnstile` as text and lacks the callback guard until replaced.

### Pitfall 18: CSP details
No inline `style="..."` attributes and no `setAttribute('style', ...)` (a nonce-only `style-src` blocks both; CSSOM property assignment is allowed); no inline event handlers; the i18n block is `type="application/json"` with the nonce; the page must not use `eval` or `new Function`. Telegram's script injects a `<style>` for Web clients (Pattern 7), a known cosmetic violation. If a console shows CSP violations in the spike, add the narrowest entry and record it in `08-SPIKE.md`.

### Pitfall 19: Endless Try again on owner misconfiguration
Widget error codes for a wrong sitekey, an unauthorized domain or a disabled key are marked "Retry: No" by Cloudflare, but the approved UI maps every widget error to `retry`. The owner sees it as a loop. Mitigation without changing the UI: the guide's troubleshooting list names codes `110100/110110/110200/400020/400021/400070`; the page does not log them; the server cannot see them. Optional UI polish for the checker: map those codes to `unsupported`-style copy (not required).

### Pitfall 20: Dummy keys cannot be used end to end
With Cloudflare's dummy keys the verification returns `"action":"test"`, `"cdata":"test-data"` and hostname `localhost` regardless of what the widget sent, so the action/cdata/hostname checks always reject them. The owner's real-key smoke test is therefore the only end-to-end check (spike); the suite uses `httptest`.

### Pitfall 21: Clickjacking, CSRF, timing
Clickjacking: `frame-ancestors` lists only Telegram's web origins and the page does nothing without `initData` in the URL hash. CSRF: no cookie or ambient credential exists (`credentials: 'omit'`, the server never reads cookies); authentication is the signed body; `Content-Type: application/json` plus no CORS headers plus 405 on `OPTIONS` stops cross-site form posts; do **not** enforce `Origin` (WebView and iframe sandboxes can send `null`; no benefit without ambient credentials). Timing: `hmac.Equal`; the token is a handle, not compared as a secret.

### Pitfall 22: `crypto.subtle` is a secure-context API
The page computes `cData` with `crypto.subtle.digest`, which exists only on HTTPS origins (the Mini App must be HTTPS in production: [CITED: telegram-apps `creating-new-app.md`: "Production environment allows usage only valid HTTPS links"]). If it is missing, render the `unsupported` state; do not ship a hand-written SHA-256.

### Pitfall 23: The page's JavaScript is not covered by `make test`
No JS runner is part of the project. Keep the script small and pure-function based (language resolution, `cData` derivation, contrast math, answer-to-state mapping), cover it with a `node --test` file that loads the script into a `vm` context with a stubbed `window`/`document` (Node 22 built-ins only, nothing to install), run it by the executor, and list the rest in `08-SPIKE.md`. It is a Wave 0 gap, not a hidden `t.Skip`.

## Code Examples

### Consume-if-user (Go side of the script above)
```go
// Source: scripts verified on miniredis v2.39.0 (scratchpad); Go wrapper is the recommended shape.
type ConsumeResult int

const (
	ConsumeUnknown   ConsumeResult = iota // {0}: key missing (never issued, expired, already consumed)
	ConsumeWrongUser                      // {2}: another member; nothing changed
	ConsumeOK                             // {1, pttl, fields}: this call owns the challenge
)

func (s *Store) ConsumeIfUser(ctx context.Context, token string, userID int64) (ConsumeResult, Record, time.Duration, error) {
	res, err := consumeScript.Run(ctx, s.rdb, []string{keyChallenge(token)}, strconv.FormatInt(userID, 10)).Slice()
	if err != nil {
		return ConsumeUnknown, Record{}, 0, err // caller answers 503 retry; never a failed solve
	}
	// res[0] int64 status; for status 1: res[1] int64 PTTL (ms), res[2] flat []any of HGETALL pairs.
	// ...
}
```

### Mini App link and cdata
```go
// Source: D-06 (link) and D-19 (cdata) quoted in User Constraints; action value is the recommended constant.
func MiniAppLink(botUsername, shortName, token string) string {
	return "https://t.me/" + botUsername + "/" + shortName + "?startapp=" + token
}

func CData(challenge string) string {
	sum := sha256.Sum256([]byte("cd1:" + challenge))
	return hex.EncodeToString(sum[:]) // 64 characters, [0-9a-f]: inside Turnstile's cData rule (<=255, alphanumeric/_/-)
}

const turnstileAction = "captcha_join" // 12 characters, inside the <=32 rule
```

### `siteverify` call (shape only)
```go
// Source: request/response contract from cloudflare-docs server-side-validation.mdx.
body, _ := json.Marshal(map[string]string{"secret": c.secret, "response": turnstileToken})
req, _ := http.NewRequestWithContext(ctx, http.MethodPost, siteverifyURL, bytes.NewReader(body))
req.Header.Set("Content-Type", "application/json")
resp, err := c.http.Do(req) // http.Client{Timeout: 5 * time.Second, CheckRedirect: noRedirect}
// err -> Retry; never wrap or log err. Status >= 500 -> Retry. Else decode a LimitReader(64 KiB) into
// struct{ Success bool; Hostname, Action, CData string; ErrorCodes []string `json:"error-codes"` }
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Math/text captcha answered by callback buttons | Cloudflare Turnstile widget in a Mini App; server verifies signed `initData` + `siteverify` | This phase | Little friction; bots without a Telegram client session cannot even open the check. |
| Validate `initData` with the bot token only | Also possible: third-party Ed25519 `signature` with Telegram's public key (no bot token needed) | `signature` field appears in current `initData` | Deferred (CONTEXT); but `signature` is now part of the bot-token data-check string and must not be stripped (verified). |
| `web_app` inline buttons | Direct-link Mini Apps (`t.me/<bot>/<app>?startapp=`) | Bot API 6.x onward | `web_app` buttons are private-chat only, so groups must use the direct link. |
| Turnstile implicit rendering | Explicit rendering with `turnstile.render`, `reset`, callbacks | Current docs | Needed to bind `cData`/`action` per challenge and to reset on Try again. |

**Deprecated/outdated:** the UI-SPEC's lowercase `cdata` render option (the JS render parameter is `cData`); the telegram-apps "up to 64 characters" start parameter limit in older mirrors (current text says 512; the token is 22 characters).

## Assumptions Log

> Claims tagged `[ASSUMED]` or resting on unverified summaries. The planner should keep each behind the spike or an owner confirmation.

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | A direct-link Mini App opened from a URL button in a group receives the **tapper's** signed `initData` (`user.id` of the person who tapped). | Pattern 1 | The whole binding fails; spike item 1 proves it. Design is correct if true, and nothing else in this phase can compensate if false. |
| A2 | Whether `start_param` is inside the signed `initData` for direct links (docs contradict). | Pattern 7, Pitfall 3 | Handled by the two-source read and "signed wins when present"; worst case the binding is unsigned but still safe. |
| A3 | Telegram shows a first-launch "open this Mini App" confirmation for direct links. | Pattern 1 | Extra friction; copy in the challenge message and guide mentions it only if confirmed. |
| A4 | The Telegram Web origins for `frame-ancestors` are `web.telegram.org`, `webk.telegram.org`, `webz.telegram.org`. | Pattern 7 | Mini App blank in Telegram Web only; native apps unaffected. |
| A5 | `&mode=compact` is the correct link parameter and opens the compact sheet. | Pattern 1 | Cosmetic; not used in v1. |
| A6 | `Referrer-Policy: no-referrer` does not break the Turnstile widget's hostname detection. | Pattern 7 | Widget shows `110200`; relax to `strict-origin`. |
| A7 | BotFather short-name rules (`^[A-Za-z0-9_]{1,64}$` is a loose superset). | Pattern 8 | A valid name rejected at startup; loosen the regex. |
| A8 | Real Cloudflare `siteverify` returns HTTP 200 with `success:false` for failures and 400 only for malformed requests. | Pattern 5 | Classification relies on the JSON, not the status, so low impact. |
| A9 | Real Turnstile secret keys have no safe structural shape to scrub. | Pattern 8 | None: exact-value registration is the control. |
| A10 | Telegram's iOS, Android and Desktop WebViews run Turnstile (Cloudflare lists embedded browsers as "limited support"). | Pattern 7 | The feature does not work on a platform; spike item 2; D-09 fallback only covers unavailability, not a failing widget (see Open Question 2). |
| A11 | Telegram Web's iframe permits same-origin `fetch` from the page with a normal `Origin`. | Pitfall 21 | None, `Origin` is not enforced. |
| A12 | `GetChatMemberWithContext` with a 3 s context behaves like `FetchBotMember`'s 8 s call. | Pattern 6 | None beyond a tighter timeout. |

## Open Questions

1. **Does the real client put `start_param` into the signed `initData`?** Known: docs conflict; the shipped script parses only `tgWebAppData`. Unclear: actual behaviour. Recommendation: build the two-source read now; record the answer in `08-SPIKE.md` item 1.
2. **What happens to a member whose WebView cannot run the widget at all?** Known: D-09 covers *unavailable server setup*, not an unusable widget; such a member gets `retry` forever and is kicked at the limit (they can rejoin, same result). Unclear: how common. Recommendation: accept for v1; the spike measures it; an admin can switch the group to math.
3. **Claim won but unmute failed.** The approved UI has no state for it. Recommendation: log Warn, rely on the durable `captcha_muted_users` retry, answer `retry`; ask the UI checker whether to add a short "you passed, you will be unmuted shortly" copy later. Not a blocker.
4. **Should the bot-rights preflight be in v1?** Recommendation: yes (cheap, turns a bad state into a retry); the planner may defer it if time-boxed.

## Questions for the Owner

1. **Should the captcha routes be reachable only through a path allow-list in the proxy, or should the bot enforce it?**
   - Why it needs you: D-10 puts the routes on the same port as `/metrics`, `/db_metrics`, `/health`, pprof and `/webhook`; a proxy that forwards everything exposes them (metrics are open unless you set `METRICS_AUTH_TOKEN`).
   - Option A (recommended): document that the proxy forwards only `/captcha` and `/captcha/*`, and have the bot print one startup warning when the web captcha is enabled and `METRICS_AUTH_TOKEN` is empty. No code gate.
   - Option B: add a small middleware that answers 404 for any path outside `/captcha*` when the request `Host` equals the host in `CAPTCHA_WEB_BASE_URL` (internal scraping by another host name keeps working). Stronger, but breaks if your proxy rewrites `Host`.
   - Option C: a separate listener port for the captcha routes (deviates from D-10).
   - Recommendation: A for v1, B only if you ever put the bot behind a proxy you do not fully control.

## Existing Code to Extend (file, function, line)

| What | Where | Change |
|------|-------|--------|
| `SendCaptcha` (mode branch `1032-1062`, keyboard guard `1121-1129`, text `1131-1154`, send `1156-1177`, tail `1179-1192`) | `alita/modules/captcha.go:1000` | `mode` variable, Turnstile branch, token issue, URL-button message. |
| `captchaVerifyCallback` pass path `1499-1577`, failure path `1579-1635` | `captcha.go:1413` | Extract `claimCaptchaPass` / `finishCaptchaPass`; add the `ts1:` refusal after the attempt load (`1470-1488`). |
| `captchaRefreshCallback` attempt load `1703-1721` | `captcha.go:1638` | Add the `ts1:` refusal (Pitfall 1). |
| `handleCaptchaTimeout` `1259-1336`, `expireCaptchaAttempt` `1231-1257`, `executeCaptchaFailureAction` `1368-1389`, `unmuteCaptchaUser` `1391-1402`, `restoreCaptchaPermissions` `1404-1411` | `captcha.go` | Reused as is (counter hook for `expired` on a `ts1:` attempt). |
| Lifecycle: `StartCaptchaLifecycle` `138`, `StopCaptchaLifecycle` `174`, `startCaptchaLifecycleTask` `185`, `runOrphanedCaptchaRecovery` `244`, `scheduleCaptchaTimeout` `1203`, workers `2037-2066` | `captcha.go` | Unchanged; every Turnstile attempt is an ordinary row so the 5-minute cleanup tick, the timer and startup recovery already cover it. |
| `captchaModeCommand` `585-633`, `captchaCommand` (`/captcha` status line shows the raw mode, `486`) | `captcha.go` | Third mode, three ordered refusals, description key. |
| Settings/attempt repository: `GetCaptchaSettingsContext` `31`, `SetCaptchaMode` `73-92`, `createCaptchaAttemptPreMessage` `162-228`, `GetCaptchaAttemptByID` `297`, `CompleteCaptchaAttemptAtomic` `380`, `ReleaseCaptchaAttemptAtomic` `407`, `deleteCaptchaAttemptAtomic` `424` | `alita/db/captcha/repository.go` | `SetCaptchaMode` accepts `turnstile`; the rest reused. Defaults when no row: `Timeout: 2`, `FailureAction: "kick"`, `MaxAttempts: 3` (`:37-43`). |
| `CaptchaSettings` / `CaptchaAttempts` (`Answer string gorm:"column:answer;not null"`, `ExpiresAt` with `chk_captcha_expires_at`) | `alita/db/models/captcha.go:5-34` | CHECK tag only. |
| Join entry: `membershipDepsForJoin` -> `SendCaptcha(bot, &ctxCopy, user.Id, user.FirstName)` | `alita/modules/greetings.go:743-757` | None; `SendWelcomeMessage(bot, ctx, userID, firstName)` (`:620`) reads `ctx.EffectiveChat` and an optional message thread only, so a synthesized `ext.Context` works for the HTTP path. |
| HTTP server: `New`, `Start` timeouts `ReadTimeout`/`WriteTimeout` 10 s (`378-385`), `Stop` `419` (10 s `Shutdown`), no per-route hooks | `alita/utils/httpserver/server.go` | Add `Handle`; endpoint log list. `s.bot` is set only by `RegisterWebhook` (`:206`), so the captcha handler gets the bot by injection, not from `Server`. |
| `registerSecrets` `478-490`, `ValidateConfig` `212`, `setDefaults` `379`, `LoadConfig` struct literal `~290-346` | `alita/config/config.go` | New fields, validation, secret registration. |
| Startup/shutdown order | `main.go:105` (bot), `:160` (DB close), `:196-226` (drains), `:234-243` (routes), `:253-264` (webhook), `:282/:306` (polling), `postInit :434` | Route registration, readiness mark. |
| `/stats` | `alita/modules/devs.go:362-379`, `getStats :381`, `stats := devs.LoadAllStats() + raidAIStatsSection()` `:407` | Append a section. |
| Rate-limit precedent (Redis first, in-process fallback) | `alita/utils/ratelimit/backup_ratelimit.go`, `telegram_pacer.go` | Pattern only; the captcha windows are Lua counters in `captchaweb`. |
| i18n | `alita/i18n/manager.go:54` `GetTranslator`, `translator.go:17` `GetString`; locale files `locales/{en,es,fr,hi,id,pt,ru}.yml` | Build the 7 tables at startup. Existing keys to edit: `captcha_help_msg` (`en.yml:289`), `captcha_mode_specify` (`:976`), `captcha_mode_invalid` (`:977`), `captcha_invalid_mode_error` (`:1561`); related existing: `captcha_mode_set_formatted` (`:1734`) takes `(%s mode, %s description)`, `captcha_not_for_you` (`:1566`), `captcha_attempt_not_valid` (`:1568`). |
| Test harness | `alita/modules/test_harness_test.go:42-60` (`moduleBotClient`, `newModuleTestBot` with `Username: "AlitaTestBot"`, `Token: "999:test"`), `:344` (`&db.CaptchaSettings{}` in the migrate list), `alita/db/captcha/testmain_test.go`, `alita/db/constraints_test.go` | Reuse; no `AutoMigrate` list change (the model is already in every list). |

**New locale keys (UI-SPEC):** 26 `captcha_web_*` page keys (`page_title` ... `close_button`), `captcha_web_welcome`, `captcha_web_button`, `captcha_mode_turnstile_desc`, `captcha_web_refuse_not_configured`, `captcha_web_refuse_no_redis`, `captcha_web_refuse_no_rights`; four edited keys. All in 7 files. The translators must keep within the UI-SPEC "Max other" lengths; the locale test enforces them.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go toolchain | build, tests | yes | go1.26.0 linux/amd64 | - |
| miniredis (module cache) | Redis and Lua tests | yes | v2.39.0 | - |
| `redis-server` | optional manual runs | yes | 7.0.15 | miniredis |
| Node.js | optional `node --test` for the page script | yes (not a project dependency) | v22.22.0 | executor-run only; recorded in the summary |
| PostgreSQL | PostgreSQL-only tests | binaries may exist but MUST NOT be started | - | CI `make test-postgres-integrity` |
| `raw.githubusercontent.com`, `registry.npmjs.org` | reading docs sources and the `telegram-web-app.js` copy | yes | - | - |
| `core.telegram.org`, `telegram.org`, `docs.telegram-mini-apps.com`, `developers.cloudflare.com`, `challenges.cloudflare.com` | reading official pages, the live widget | no (proxy 403) | - | Cloudflare docs repo mirror, telegram-apps docs repo, SDK tarball; the real Telegram pages remain to be read when the host is allowed |
| Cloudflare keys, public hostname, BotFather Mini App, real Telegram clients | live checks | no, by owner decision | - | `httptest` fakes in the suite; `08-SPIKE.md` for the owner |

**Missing dependencies with no fallback:** none for planning or building.
**Missing dependencies with fallback:** everything live (faked in tests, checklisted for the owner).

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go `testing` + testify (assertions only); real fixtures: `internal/testdb.Run` / the module harness (SQLite), miniredis, `httptest` for Cloudflare, hand-written `gotgbot.BotClient` fakes (`moduleBotClient`). No mock libraries. |
| Config file | none (`-tags testtools`); harness `alita/modules/test_harness_test.go` |
| Quick run command | `go test -tags testtools -race -count=1 ./alita/utils/captchaweb ./alita/config ./alita/db/captcha` and `go test -tags testtools -race -count=1 -run '^(TestCaptchaWeb|TestSendCaptchaTurnstile|TestCaptchaMode|TestCaptchaVerifyCallback|TestHandleCaptchaTimeout)' ./alita/modules` |
| Full suite command | `make test` (then `make lint`, `make check-translations`, `make check-docs`, `CGO_ENABLED=0 go build ./...`) |

### Phase Requirements -> Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| CAPT-01 | `/captchamode turnstile` stored when web captcha is configured, Redis answers and the bot has Restrict; math and text still work | integration | `... -run '^TestCaptchaModeTurnstile$' ./alita/modules` | no, Wave 0 |
| CAPT-01 | Refusals in order (not configured, no Redis, no rights), mode unchanged, replies in the admin's language; non-admin refused by the gate | integration | `... -run '^TestCaptchaModeTurnstileRefusals$' ./alita/modules` | no, Wave 0 |
| CAPT-01 | Repository and model accept `turnstile`, reject `bogus` (SQLite CHECK built from the tag) | repo | `... -run '^(TestSetCaptchaModeAcceptsTurnstile)$' ./alita/db/captcha` and extended `TestCaptchaSettingsConstraint_ValidModes` | partly (extend) |
| CAPT-01 | PostgreSQL CHECK widened by the migration, old constraint replaced | PG only | `TestCaptchaModeTurnstileConstraint` in `./alita/db/migrations` (add to the `make test-postgres-integrity` `-run` list; never run locally) | no, Wave 0 |
| CAPT-02 | Join in a Turnstile group: member muted, attempt row with `ts1:` answer, pending flag invalidated, token in Redis with the right TTL, one message with exactly one URL button `https://t.me/AlitaTestBot/<short>?startapp=<token>` in the chat language | integration | `... -run '^TestSendCaptchaTurnstileMutesAndPostsMiniAppButton$' ./alita/modules` | no, Wave 0 |
| CAPT-02 / D-09 | Fallback to the group's math captcha when not configured, when Redis is down, when the bot has no username; no extra message | integration | `... -run '^TestSendCaptchaTurnstileFallsBackToMath$' ./alita/modules` | no, Wave 0 |
| CAPT-02 | Send failure and token-issue failure roll the attempt back and unmute | integration | `... -run '^TestSendCaptchaTurnstileRollsBackOnFailure$' ./alita/modules` | no, Wave 0 |
| CAPT-03 | Wrong user's verify returns `not_for_you`, changes nothing, does not consume; the right user then still passes | route | `... -run '^TestVerifyRouteWrongUserChangesNothing$' ./alita/utils/captchaweb` | no, Wave 0 |
| CAPT-03 | Reused / parallel token: 50 concurrent verifies, exactly one proceeds | route + miniredis | `... -run '^TestChallengeStoreConsumeOnce$' ./alita/utils/captchaweb` | no, Wave 0 |
| CAPT-03 | Forged `captcha_verify` / `captcha_refresh` callbacks cannot pass or replace a Turnstile attempt | integration | `... -run '^TestCaptchaWebAttemptRefusesCallbacks$' ./alita/modules` | no, Wave 0 |
| CAPT-03 | Old token after a re-join never touches the new attempt | integration | `... -run '^TestCaptchaWebRejoinInvalidatesOldChallenge$' ./alita/modules` | no, Wave 0 |
| CAPT-04 | `initData` known-answer vectors (both published), tampered field, missing/duplicate hash, stale (>5 min), future (>60 s), bad user JSON, bot user, `signature` kept in the check string | unit | `... -run '^(TestInitDataKnownAnswer|TestInitDataRejects)$' ./alita/utils/captchaweb` | no, Wave 0 |
| CAPT-04 | The page cannot report a pass: a body with no or an empty Turnstile token is `invalid`; a pass needs a fake-Cloudflare success with matching hostname, action and cdata | route | `... -run '^TestVerifyRouteNeverTrustsThePage$' ./alita/utils/captchaweb` | no, Wave 0 |
| CAPT-04 | `siteverify` outcome table (every row of Pattern 5) and request shape (JSON, only `secret` and `response`, no redirect follow, 64 KiB cap, 5 s timeout) | unit (`httptest`) | `... -run '^(TestSiteverifyOutcome|TestSiteverifyRequestShape)$' ./alita/utils/captchaweb` | no, Wave 0 |
| CAPT-04 | Retryable outcome re-arms the same token with the remaining TTL; failed solve, wrong user and unknown token never re-arm | route + miniredis | `... -run '^TestVerifyRouteRearmsOnlyOnRetry$' ./alita/utils/captchaweb` | no, Wave 0 |
| CAPT-04 | Limits: 413, 415, 405 (incl. `OPTIONS`), 429 + `Retry-After`, 503 over the in-flight cap and before readiness; no CORS headers; `Cache-Control: no-store` | route | `... -run '^TestVerifyRouteLimitsAndHeaders$' ./alita/utils/captchaweb` | no, Wave 0 |
| CAPT-04 | Client IP: spoofed header from an untrusted peer ignored; trusted chain resolved; no per-IP bucket for a private peer with no trusted list | unit | `... -run '^TestClientIP$' ./alita/utils/captchaweb` | no, Wave 0 |
| CAPT-05 | Web pass: unmute with the chat default permissions, held-message summary with count and types, challenge deleted, success note, greeting sent, rows gone, `captcha_muted_users` cleared | integration | `... -run '^TestCaptchaWebPassUnmutesSummarisesAndCleansUp$' ./alita/modules` | no, Wave 0 |
| CAPT-05 | Callback and web pass produce the same Telegram call sequence (shared function) and the existing callback tests stay green | integration | `... -run '^(TestCaptchaPassSharedByCallbackAndWeb|TestCaptchaVerifyCallback)' ./alita/modules` | partly (existing callback tests) |
| CAPT-05 | Unmute failure after the claim schedules the durable retry and does not report `passed` | integration | `... -run '^TestCaptchaWebUnmuteFailureSchedulesRetry$' ./alita/modules` | no, Wave 0 |
| CAPT-06 | Failed solve applies the group action once (kick = banned then unbanned so the member can rejoin; ban; mute 24 h), deletes the challenge, posts the notice, returns `failed` + action | integration | `... -run '^TestCaptchaWebFailedSolveAppliesAction$' ./alita/modules` | no, Wave 0 |
| CAPT-06 | Cloudflare outage / secret error / hostname mismatch never kicks | integration | `... -run '^TestCaptchaWebCloudflareErrorIsNotAFailure$' ./alita/modules` | no, Wave 0 |
| CAPT-07 | Timeout on one "replica" (second bot client, shared SQLite + miniredis) while the other serves verify: exactly one outcome | integration | `... -run '^TestCaptchaWebTimeoutAndVerifyOnDifferentReplicas$' ./alita/modules` | no, Wave 0 |
| CAPT-07 | Pass vs timeout race (N iterations, `-race`): exactly one of unmute or kick; pass after expiry applies the failure at once and answers `expired` + action | integration | `... -run '^(TestCaptchaWebPassVersusTimeoutRace|TestCaptchaWebPassAfterExpiryExpiresAtOnce)$' ./alita/modules` | no, Wave 0 |
| CAPT-07 | Restart: `runOrphanedCaptchaRecovery` resumes a Turnstile attempt like any other | integration | extend `TestRunOrphanedCaptchaRecoveryResumesValidAttempts` | partly |
| CAPT-08 | Embedded string table has all 7 languages and all 26 keys; English fallback; locale parity, lengths, no placeholders | unit | `go test -tags testtools -race -count=1 -run '^TestCaptchaWebLocaleKeys$' ./alita/i18n` and `TestCaptchaWebPageStrings` in captchaweb | no, Wave 0 |
| CAPT-08 | Page response: unique nonce per response, every `script`/`style` element carries the header's nonce, CSP value, no `X-Frame-Options`, i18n block valid JSON and safe against `</script>` in a value | route (HTML parse with `golang.org/x/net/html`, already an indirect dependency) | `... -run '^TestPageHeadersAndNonce$' ./alita/utils/captchaweb` | no, Wave 0 |
| CAPT-08 | Page script: language resolution (`in`->`id`, unknown->`en`), `cData` digest, answer-to-state mapping, contrast guard | `node --test` (executor-run, not in `make test`) | `node --test scripts/captcha-page/` | no, Wave 0 (see Pitfall 23) |
| PLAT-04 | Self-hosting page, `environment.md`, `sample.env`, `captcha_help_msg` docs regenerate cleanly | docs | `make check-docs` after `make generate-docs`; manual read-through of the guide against the nine steps in D-15 | manual-only for content (justification: prose) |
| PLAT-02 (Turnstile half) | Config: none or all four; partial -> error naming variables not values; https, host, no query; secret registered so `logredact.Scrub` removes it | unit | `go test -tags testtools -race -count=1 -run '^TestCaptchaWebConfig$' ./alita/config` | no, Wave 0 |
| PLAT-02 | The secret and all inputs never appear in logs on any path, redaction hook off, Cloudflare double echoing the secret | integration | `... -run '^TestCaptchaWebNeverLogsInputs$' ./alita/utils/captchaweb` | no, Wave 0 |
| D-17 | Counters increment on issue/pass/fail/expire/error and appear in the developer `/stats` text | integration | `... -run '^TestCaptchaWebStatsSection$' ./alita/modules` | no, Wave 0 |
| Shutdown | A finish task started before `StopCaptchaLifecycle` completes before it returns; after it, the task runs inline | unit | `... -run '^TestCaptchaWebFinishJoinsLifecycleDrain$' ./alita/modules` | no, Wave 0 |
| D-10 | Routes absent without settings, present with them; `/captcha/health` 200 with Redis, 503 without | route | `... -run '^(TestCaptchaWebRoutesRegistration|TestHealthRoute)$' ./alita/utils/captchaweb ./alita/utils/httpserver` | no, Wave 0 |

### Sampling Rate
- **Per task commit:** the quick run commands above for the packages touched.
- **Per wave merge:** `make test` plus `make lint`.
- **Phase gate:** `make test`, `make lint`, `make check-translations`, `make check-docs`, `CGO_ENABLED=0 go build ./...` green before `/gsd-verify-work`; `node --test` result recorded; PostgreSQL-only tests are exercised by CI (`make test-postgres-integrity`), not locally.

### Wave 0 Gaps
- [ ] `alita/utils/captchaweb/*_test.go` and a fake-Cloudflare `httptest` helper (switches: outcome, echo the secret, slow, 500, non-JSON, redirect).
- [ ] A modules-level harness helper: miniredis wired into `cache` (the raid tests already do it), a `moduleBotClient` per replica. The default client already answers `getChatMember` for the bot (user 999) as administrator with `can_restrict_members` (`alita/modules/test_harness_test.go:119-123`, quote: `"can_restrict_members":true`), so only the no-rights case needs a response override; `getChat`, `sendMessage`, `deleteMessage`, `restrictChatMember`, `banChatMember`, `unbanChatMember` responses must be present for the pass, failure and timeout tests.
- [ ] `alita/i18n` locale test for the `captcha_web_` keys (pattern: `lockdown_locale_test.go`).
- [ ] `scripts/captcha-page/` `node --test` file (executor-run).
- [ ] Add `TestCaptchaModeTurnstileConstraint` to the `-run` regex of `make test-postgres-integrity`.
- [ ] Known-answer vectors from the published sources stored as test constants (public sample tokens; `#nosec G101` if flagged).

### What only a live check can prove (cannot be automated; checklist in `08-SPIKE.md`)
Direct link posted by the bot in a group opens the Mini App for each client (Android, iOS, Desktop, Web K/A) and delivers the **tapper's** `initData`; whether `start_param` is signed; the `startapp` length; the first-launch dialog; that a second member's tap is rejected; that the Turnstile widget renders and returns a token inside each WebView; real `siteverify` hostname for the owner's domain; CSP violations in each client (including the `frame-ancestors` host list and the `style-src` effect of Telegram's injected style); whether `Referrer-Policy: no-referrer` is acceptable to the widget; `crypto.subtle` availability; compact mode; theme and safe-area rendering; real latency of the whole check against the 12 s client timeout; and that BotFather accepts the owner's short name and URL.

## Security Domain

### Applicable ASVS Categories (level 1, `security_enforcement` enabled, block on `high`)

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | yes | Signed `initData` is the authentication of the verifying user: HMAC-SHA256 with the bot-token-derived key, constant-time compare, `auth_date` window. |
| V3 Session Management | yes | The challenge token is a one-time, user-bound capability with a bounded lifetime; no cookies or ambient sessions exist. |
| V4 Access Control | yes | Only `user.id == challenge.user`; `/captchamode` keeps the live admin gate (`captchaAdminGate`) and the bot-rights check; the pass/fail/expire decision is the DB conditional delete. |
| V5 Input Validation | yes | Strict JSON (`DisallowUnknownFields`, size cap, field regexes), `initData` parsed once with duplicate-key rejection, `challenge` charset check, `html/template` for every value placed in HTML. |
| V6 Cryptography | yes | `crypto/rand` 128-bit token and reserved answer; HMAC-SHA256; `hmac.Equal`; no custom crypto. |
| V7 Error Handling and Logging | yes | Generic fixed JSON answers; no inputs, tokens or Cloudflare bodies in logs or errors; secret registered with `logredact`. |
| V8 Data Protection | yes | `Cache-Control: no-store`, `Referrer-Policy: no-referrer`; `initData` and tokens never persisted; Redis values hold ids only. |
| V9 Communications | yes | HTTPS to Cloudflare, no redirects followed; TLS terminated by the owner's proxy for members (documented). |
| V10 Malicious Code / third-party scripts | yes | CSP with nonce and two script hosts; `api.js` not proxied or cached (Cloudflare's rule). |
| V11 Business Logic | yes | One attempt per user per chat, one solve, exactly-once claim, per-IP/user/token windows. |
| V13 API | yes | JSON only, 405 on other methods incl. `OPTIONS`, no CORS, body cap, in-flight cap. |
| V14 Configuration | yes | Settings validated at startup; secret never logged; documented proxy exposure rules; sample.env without values. |

### Known Threat Patterns for this stack

| Pattern | STRIDE | Severity | Standard Mitigation |
|---------|--------|----------|---------------------|
| Forged `captcha_verify` / `captcha_refresh` callback converts or passes a Turnstile attempt | Spoofing, Elevation | **High** | Reserved random `ts1:` answer + explicit refusal in both callbacks (Pitfall 1). |
| Another member passes or burns the target's challenge using the public link | Spoofing, DoS | **High** | Signed `user.id` bound to the record; consume script mutates nothing for a wrong user; limiter keyed by (token, verified user). |
| A page or script reports a pass without Cloudflare | Tampering | **High** | Pass only after server-side `siteverify` with hostname, action and cdata; page answers are display only. |
| Turnstile secret or member data in logs/errors | Information disclosure | **High** | `RegisterSecret`; no inputs to loggers; fixed answers; redaction-off test with an echoing Cloudflare double. |
| Challenge token reuse / parallel use | Repudiation, Tampering | Medium | Atomic consume; DB conditional claim; done-marker for idempotent replies. |
| Replay of `initData` | Spoofing | Medium | 5-minute window; user-bound; signed `start_param` equality when present; a replay only helps the same user. |
| Replay or cross-challenge use of a Turnstile token | Tampering | Medium | Cloudflare single use + 300 s; `cdata` bound to the challenge; `action` and hostname checked. |
| Exposure of `/metrics`, `/db_metrics`, pprof, webhook through the public proxy | Information disclosure | Medium | Guide forwards only `/captcha*`; startup warning; Owner Question 1. |
| Spoofed forwarding headers to dodge limits | Spoofing, DoS | Medium | Trusted-proxy list, rightmost-untrusted walk, no header trust by default. |
| Flooding `/captcha/verify` (CPU, Redis, Cloudflare quota) | DoS | Medium | In-flight cap, three Redis windows, 16 KiB body, early `initData` rejection before any Cloudflare call. |
| Clickjacking / CSRF | Tampering | Low | `frame-ancestors` allow-list; no ambient credentials; JSON content type; 405 on `OPTIONS`. |
| Kicking members for owner or Cloudflare faults | DoS (self-inflicted) | Medium | Three-way outcome; only provable failures fail a member. |
| Timing side channel on the hash | Information disclosure | Low | `hmac.Equal`. |
| Half-applied pass after a crash | Integrity | Low | Claim writes the durable unmute row in the same transaction; cleanup tick and recovery cover every attempt. |
| Third-party script compromise (telegram.org / challenges.cloudflare.com) | Tampering | Low (accepted) | CSP pins hosts; no SRI is possible for self-updating scripts; Cloudflare forbids caching `api.js`. |

## API Coverage

Proposed content for the phase's `COVERAGE.md`. Surfaces enumerated from Cloudflare's documentation source (server-side-validation, widget-configurations, client-side-rendering, content-security-policy, supported-languages, testing, error-codes, hostname-management, mobile-implementation) and from the Telegram Mini Apps material listed in Sources; Telegram's official reference pages were unreachable, so the Telegram rows rest on mirrors.

### Cloudflare Turnstile: `siteverify`
| Capability | Decision | Reason |
|------------|----------|--------|
| `POST /turnstile/v0/siteverify` (JSON body) | INTEGRATE | The only trusted verification (CAPT-04). |
| Request `secret`, `response` | INTEGRATE | Required. |
| Request `remoteip` | OPT-OUT | Optional; the address behind a reverse proxy is unreliable and sending member IPs adds a disclosure for little value. |
| Request `idempotency_key` | OPT-OUT | The page's Try again yields a fresh token; a server-side retry of a possibly spent token has no use. |
| Response `success`, `error-codes` | INTEGRATE | Outcome table. |
| Response `hostname`, `action`, `cdata` | INTEGRATE | Binds the token to our page and to one challenge (D-03). |
| Response `challenge_ts` | OPT-OUT | Cloudflare already enforces the 300 s lifetime and single use. |
| Response `metadata.ephemeral_id` | OPT-OUT | Enterprise only. |
| Form-encoded requests | OPT-OUT | JSON is enough. |

### Cloudflare Turnstile: client widget
| Capability | Decision | Reason |
|------------|----------|--------|
| Explicit rendering (`api.js?render=explicit`, `turnstile.render`) | INTEGRATE | Per-challenge `cData`/`action`, reset on Try again. |
| Implicit rendering (`cf-turnstile` class) | OPT-OUT | Cannot bind per-challenge data cleanly. |
| `sitekey`, `action`, `cData`, `theme`, `size` (`flexible`/`compact`), `language`, `appearance`, `retry`, `refresh-expired`, `refresh-timeout` | INTEGRATE | Set by the UI contract. |
| `callback`, `error-callback`, `expired-callback`, `timeout-callback` | INTEGRATE | State machine (ready/verifying/retry). |
| `before-interactive-callback`, `after-interactive-callback` | OPT-OUT | No UI state needs them. |
| `unsupported-callback` | OPT-OUT | The loading timeout and the widget's own message cover an unsupported browser; revisit after the spike. |
| `turnstile.reset`, `turnstile.ready` / `onload` | INTEGRATE | Retry and safe render timing. |
| `turnstile.remove`, `getResponse`, `isExpired`, `execute`, `execution: 'execute'` | OPT-OUT | Not needed. |
| `response-field`, `response-field-name`, `tabindex`, `retry-interval`, `feedback-enabled`, `offlabel-*` | OPT-OUT | Defaults are acceptable; no form. |
| Widget modes (Managed / Non-Interactive / Invisible) | INTEGRATE Managed only | D-15; recommended by Cloudflare. |
| Test sitekeys and secrets | OPT-OUT in the suite; documentation only | Pitfall 20. |
| Pre-clearance, Turnstile analytics API, widget management API, ephemeral IDs, Challenge Platform products | OPT-OUT | Owner creates the widget in the dashboard; not needed. |

### Telegram Mini Apps and Bot API surface
| Capability | Decision | Reason |
|------------|----------|--------|
| Direct link `t.me/<bot>/<app>?startapp=<value>` | INTEGRATE | The only way to open a Mini App from a group message. |
| `mode=compact` / `mode=fullscreen` link parameter | OPT-OUT (v1) | Spike decides compact; fullscreen is not wanted. |
| BotFather `/newapp`, `/myapps` > Edit link | INTEGRATE (documentation) | Registration and URL change steps. |
| `InlineKeyboardButton.url` | INTEGRATE | Group-compatible launch. |
| `InlineKeyboardButton.web_app`, menu button, attachment menu | OPT-OUT | Private-chat only / not a group flow. |
| `initData` fields `user`, `auth_date`, `hash`, `signature`, `start_param` | INTEGRATE | Authentication and handle. |
| `initData` fields `chat_type`, `chat_instance`, `query_id`, `chat`, `receiver`, `can_send_after` | OPT-OUT | Not needed (log only if the spike wants it). |
| Bot-token HMAC validation | INTEGRATE | Locked (D-02). |
| Third-party Ed25519 `signature` validation | OPT-OUT | Deferred by CONTEXT. |
| `Telegram.WebApp`: `initData`, `initDataUnsafe` (`user.language_code`, `start_param`), `ready`, `close`, `colorScheme`, `themeParams` / `--tg-theme-*`, `themeChanged`, `isVersionAtLeast`, `disableVerticalSwipes`, `setHeaderColor`, `setBackgroundColor`, `HapticFeedback` | INTEGRATE | UI contract. |
| `expand`, `requestFullscreen`, MainButton, SecondaryButton, BackButton, SettingsButton, popups, `openLink`, `sendData`, `CloudStorage`, biometrics, location, sensors, `shareToStory`, `addToHomeScreen`, `switchInlineQuery`, `invokeCustomMethod` | OPT-OUT | Not part of a verification page. |
| Bot API `answerWebAppQuery`, `savePreparedInlineMessage`, `setChatMenuButton` | OPT-OUT | No result is sent from the Mini App to a chat; the bot acts on the server's decision. |

## Docs and AGENTS.md updates (same change set)

- **New** `docs/src/content/docs/self-hosting/captcha-web.md` (hand-written, add the `MANUALLY MAINTAINED` marker in its first 512 bytes to be explicit): public HTTPS hostname and a reverse proxy that forwards **only** `/captcha` and `/captcha/*` (examples for a plain proxy), BotFather `/newapp` (and `/myapps` > Edit link, URL `<base>/captcha`), Cloudflare widget in Managed mode with a hostname list that contains exactly the public host (hostnames are FQDNs, no scheme/port/path, subdomains included), the four settings (+ trusted proxies), `GET /captcha/health`, what is sent to Cloudflare (the Turnstile token and the secret, nothing about the member; the member's browser talks to Cloudflare itself) and to Telegram, the behaviour table (fallback to math, timeouts), and troubleshooting (widget codes `110200` etc., hostname mismatch, blank sheet in Telegram Web, endless Try again, `retry` vs `failed`). Say in one line that `tgWebAppStartParam` may appear in the proxy's access log and is a short-lived handle bound to one member.
- **Edit** `docs/src/content/docs/api-reference/environment.md` (frozen page) and `sample.env`: the four settings and `CAPTCHA_WEB_TRUSTED_PROXIES`; mark the secret as scrubbed from logs; no values.
- **Regenerate** `docs/src/content/docs/commands/captcha/index.md` with `make generate-docs` after editing `captcha_help_msg` in `locales/en.yml`.
- **`AGENTS.md`** (rules this phase adds or changes): Redis key families `alita:captcha:ch:*`, `alita:captcha:done:*`, `alita:captcha:rl:*` (outside the cache prefix, not cleared by `CLEAR_CACHE_ON_STARTUP`); the `ts1:` reserved answer and the refusal in both callbacks; web pass uses `claimCaptchaPass` / `finishCaptchaPass`, finish tasks join the captcha lifecycle drain; the verify route's readiness gate, write-deadline and detached-context rule; fallback to math when the web captcha is unavailable; `chk_captcha_mode` now includes `turnstile`; Turnstile secret registered; `Server.Handle`; no new handler group; the exposure rule for the shared port; counters in `/stats`.

## Sources

### Primary (HIGH confidence)
- Cloudflare docs source, `cloudflare/cloudflare-docs` (branch `production`, `src/content/docs/turnstile/...`), fetched through `raw.githubusercontent.com`: `get-started/server-side-validation.mdx` (endpoint, parameters, token limits, response fields, error codes), `get-started/client-side-rendering/widget-configurations.mdx` (render parameters, `cData`/`action` limits, callbacks, sizes, `language`), `get-started/client-side-rendering/index.mdx` (explicit rendering, `reset`, `ready`, `api.js` must not be proxied), `reference/content-security-policy.mdx`, `reference/supported-languages.mdx`, `troubleshooting/testing.mdx`, `troubleshooting/client-side-errors/error-codes.mdx`, `additional-configuration/hostname-management/index.mdx`, `get-started/mobile-implementation.mdx`, `cloudflare-challenges/reference/supported-browsers.mdx`, `get-started/index.mdx` (best practices).
- Repository files read this session: `AGENTS.md`, `.claude/CLAUDE.md`, `08-CONTEXT.md`, `08-UI-SPEC.md`, `REQUIREMENTS.md`, `STATE.md`, `07-RESEARCH.md`, `alita/modules/captcha.go`, `alita/db/captcha/repository.go`, `alita/db/models/captcha.go`, `migrations/20260412000000_add_missing_check_constraints.sql`, `migrations/20261029120000_add_raid_detection_ai.sql`, `alita/db/migrations/runner.go` (idempotency rewrite), `scripts/migrate_psql.sh`, `alita/utils/httpserver/server.go`, `alita/config/config.go`, `main.go`, `alita/modules/{greetings,membership,devs,raid_counters,shutdown_drains,lockdown_start}.go`, `alita/utils/chat_status/{access,owner}.go`, `alita/utils/cache/cache.go`, `alita/db/cache/local.go`, `alita/utils/logredact/logredact.go`, `alita/utils/ratelimit/backup_ratelimit.go`, `alita/i18n/{manager,translator}.go`, `scripts/check_translations/main.go`, `scripts/check_test_results/main.go`, `Makefile`, test harness files, gotgbot v2 source in the module cache (`gen_types.go`: `InlineKeyboardButton`).
- Executed in the scratchpad (outputs quoted above): the three Lua scripts on miniredis v2.39.0; `initData` validation against the two published vectors; `html/template` + `template.JS` embedding; `http.NewResponseController(w).SetWriteDeadline` against a 1 s `WriteTimeout`; `ServeMux` method-pattern responses.

### Secondary (MEDIUM confidence)
- `Telegram-Mini-Apps/telegram-apps` docs source via `raw.githubusercontent.com`: `apps/docs/platform/init-data.md` (algorithm, parameter table, `signature`, recommendation), `start-parameter.md` (characters, 512 limit, query-parameter delivery), `launch-parameters.md`, `creating-new-app.md` (`/newapp`, `/myapps`, HTTPS-only in production).
- `telegram-mini-apps/init-data-golang` README and `validate.go`, `sign.go`, `validate_third_party.go` (reference implementation and vectors).
- `@twa-dev/sdk` 8.0.2 tarball from `registry.npmjs.org`, `src/telegram-web-apps.js` (a copy of `telegram-web-app.js`; read only, never executed).
- WebSearch summaries of `core.telegram.org/bots/webapps` and `core.telegram.org/method/messages.requestAppWebView` (direct link format, `tgWebAppStartParam`, compact/fullscreen flags); the official pages themselves were unreachable.

### Tertiary (LOW confidence)
- WebSearch summaries of third-party BotFather guides (`/newapp` prompts, 640x360 picture); used for nothing beyond confirming the command name.

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH, no new packages; every seam read in the repo.
- Architecture: HIGH for the repo seams, exactly-once arbitration, scripts and server behaviours (read or executed); MEDIUM for the Telegram launch behaviour (mirrors and summaries, deliberately deferred to the spike).
- Cloudflare contract: HIGH (Cloudflare's own docs source); behaviour inside Telegram WebViews LOW until the spike.
- Pitfalls: HIGH for repo-derived ones (forged callbacks, write timeout, migration rewrite, translation check blind spot, readiness window); MEDIUM for platform ones (CSP in Telegram Web, Referrer-Policy).

**Research date:** 2026-10-29
**Valid until:** 7 days for the Telegram/Cloudflare client behaviour (fast-moving WebView and Mini App platform), 30 days for the repository seams (until the next phase edits `captcha.go`, `server.go` or `config.go`).
