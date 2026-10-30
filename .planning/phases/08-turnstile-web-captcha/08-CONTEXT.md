# Phase 8: Turnstile Web Captcha - Context

**Gathered:** 2026-10-29
**Status:** Ready for planning
**Mode:** Smart discuss (autonomous); the owner accepted every recommended answer. The spike gate (real Android, iOS and Desktop clients, a public hostname, a BotFather Mini App and Turnstile keys) was deferred by the owner ("Plan and build now, spike later").

<domain>
## Phase Boundary

New members of a group in Turnstile mode verify through Cloudflare Turnstile on a page the bot hosts and Telegram opens as a Mini App. The bot's server alone decides a pass, from Telegram's signed user data and Cloudflare's verification. A pass unmutes the member and replays their messages exactly as the existing captcha does. A failed check or a timeout kicks the member, who can rejoin to try again. Math and text captcha stay available as fallbacks. The deployment steps are documented.

Requirements: CAPT-01, CAPT-02, CAPT-03, CAPT-04, CAPT-05, CAPT-06, CAPT-07, CAPT-08, PLAT-04. The Turnstile half of PLAT-02 (the secret never appears in logs) is a must-have of this phase.

Out of scope:
- The settings menu (Phase 9) and its Captcha page.
- Running the real-device spike. It needs the owner's hostname, Mini App and keys and is deferred; this phase ships the checklist the owner runs (D-18).
- A second solve or retries for Turnstile (one solve only, CAPT-06); math and text keep their own attempts.
- Image or audio captcha changes, other captcha providers, and the Staff Group.

</domain>

<decisions>
## Implementation Decisions

### Who can pass a challenge and who decides (CAPT-03, CAPT-04)
- **D-01:** A challenge is a random 128-bit token kept in Redis that points at the existing `captcha_attempts` row (attempt ID, chat, user, expiry). The row stays the authority and keeps its one attempt per user per chat. The Mini App link carries only the token in `startapp` (letters, digits, `_` and `-`), never user text or an ID.
- **D-02:** The server verifies Telegram's signed `initData` (HMAC-SHA256 with the key derived from the bot token, `auth_date` no older than 5 minutes) and requires `user.id` to equal the challenge's user. Another member can open the link, but their verification fails and changes nothing.
- **D-03:** The token is consumed by an atomic compare-and-delete when verification starts, so a reused, parallel or expired token fails. The Turnstile widget is bound to the challenge through its `cdata` field (a value derived from the token); the server requires Cloudflare's `siteverify` answer to echo it, together with the expected hostname and `action`, so a Turnstile token minted for another challenge is rejected.
- **D-04:** The page decides nothing. It posts `initData`, the Turnstile token and the challenge token to the server and shows the server's answer. A pass is recorded only after `siteverify` returns success with the expected hostname, `action` and `cdata`.

### The join flow, settings and failure handling (CAPT-01, CAPT-02, CAPT-05, CAPT-06, CAPT-07)
- **D-05:** `/captchamode turnstile` is a third mode beside `math` and `text`. An append-only migration widens the `captcha_mode` CHECK (`chk_captcha_mode`). The command refuses the mode with a clear reason unless the web captcha is fully configured (D-11). Math and text stay.
- **D-06:** A new member in a Turnstile group is muted and gets an attempt row as today, then a challenge message with one button that opens the Mini App through its direct link (`https://t.me/<bot>/<app>?startapp=<token>`). The message is edited or removed the way the existing captcha messages are.
- **D-07:** A pass runs the existing pass path: unmute, replay of pending messages, the greeting, and cleanup. That path becomes one shared function that the callback verify and the web verify both call, so the two cannot drift.
- **D-08:** A failed Turnstile check is "one solve only": the group's failure action runs at once (kick by default, so the member can rejoin). The group's time limit uses the existing timeout and cleanup machinery, so it works on any replica.
- **D-09:** If the web captcha is not available when a member joins a Turnstile group (not configured, or Redis unavailable), that member gets the group's math captcha instead, so nobody is left muted with no way in. A Cloudflare or network error during a check is not a failed solve: the page says "try again" and the timeout still applies.

### The captcha page and hosting (CAPT-04, CAPT-08, PLAT-02)
- **D-10:** The bot's existing HTTP server serves the page and its API on the same port: `GET /captcha` (the page), `POST /captcha/verify` and `GET /captcha/health`. They are registered only when the web settings are complete. The owner puts TLS and the public hostname in front with their own reverse proxy.
- **D-11:** Settings: `CAPTCHA_WEB_BASE_URL` (public https), `TELEGRAM_MINI_APP_SHORT_NAME`, `TURNSTILE_SITE_KEY` and `TURNSTILE_SECRET_KEY`. The secret is registered with `logredact.RegisterSecret` and is sent only to Cloudflare. The verify route has a body-size cap, per-IP and per-token rate limits, `Cache-Control: no-store`, no CORS, a 5 s `siteverify` timeout, and returns generic errors to the page.
- **D-12:** The page is one static HTML file with a small script, embedded in the binary (`embed.FS`), with no framework and no build step. It loads Telegram's `telegram-web-app.js` and Cloudflare's Turnstile script, and a strict Content-Security-Policy allows only those two hosts.
- **D-13:** The page language comes from `language_code` in the signed `initData`, mapped to one of the bot's 7 locales with English as the fallback, and the strings are injected by the server. New page strings are locale keys in all 7 files.
- **D-14:** Because this is a user-facing screen, a UI design contract (`08-UI-SPEC.md`) is written before planning: the states loading, ready, verifying, passed, failed, expired and try-again; light and dark; small WebView sizes.

### Deployment guide, the deferred spike and visibility (PLAT-01, PLAT-04)
- **D-15:** A self-hosting page documents the steps: a public HTTPS hostname and reverse proxy, BotFather `/newapp` Mini App registration, a Cloudflare Turnstile widget (managed mode, hostname allow-list), the four settings, a health check, what the bot sends to Cloudflare, and a troubleshooting list.
- **D-16:** Every replica can serve the page and verify, because challenge tokens and counters live in Redis and the attempt in PostgreSQL: a challenge issued by one replica can pass on another. Without Redis the Turnstile mode is unavailable and groups fall back to math (D-09).
- **D-17:** Counters for issued, passed, failed, expired and errors appear in the developer `/stats` for this replica, following the existing pattern. Tokens, `initData` and Turnstile responses are never logged.
- **D-18 (spike deferred, owner's choice):** The real-device spike gate is not run in this phase. `08-SPIKE.md` is a checklist for the owner to run on real Android, iOS and Desktop clients: (1) a direct-link button posted in a group opens the page with signed `initData` for the user who tapped, another user is rejected, and the `startapp` value fits; (2) the Turnstile widget renders and returns a token inside Telegram's WebViews, and `siteverify` reports the owner's hostname. Each step says what a result means and what to change.

### Amendments after the UI design contract (orchestrator's calls, 2026-10-29)
- **D-19 (refines D-03):** `GET /captcha` is served before the challenge is known (Telegram passes `startapp` only in the URL fragment and in the signed `initData`), so the page computes the Turnstile `cdata` itself as `hex(SHA-256("cd1:" + start_param))`. The server recomputes it from the verified `initData` and requires `siteverify` to echo it, which still binds a Turnstile token to one challenge. No extra route is added.
- **D-20 (refines D-13):** The server cannot know the user's language before `initData` is verified, so it embeds all 7 locale string tables as a JSON block and the page picks one from `initDataUnsafe.user.language_code`, for display only, with English as the fallback. Nothing security-relevant depends on the language.
- **D-21 (refines D-03 and D-09):** A retryable outcome (Cloudflare or network error, the bot momentarily unable to reach Telegram) gives the challenge back: the server re-arms the same token with its remaining lifetime so the Try again button can succeed. A wrong user, a used or unknown token, or a failed solve never re-arms it.
- **D-22 (refines D-08):** A `failed` or `expired` answer carries the group's failure `action` (kick, ban or mute) so the page can say plainly whether the member can rejoin; the UI contract has separate copy for each.
- **D-23 (refines D-12):** The CSP uses a per-response nonce for the inline script and style, and `frame-ancestors` for the Telegram web hosts; no `X-Frame-Options` is sent, because it would break Telegram's web clients. The exact host list is confirmed in the real-device spike (`08-SPIKE.md`).
- **D-24:** The UI contract is `08-UI-SPEC.md` (hand-written page, `--tg-theme-*` variables with fallbacks, 9 states, 26 `captcha_web_*` page keys, plus the challenge message keys and three `/captchamode turnstile` refusal keys). Four existing keys are edited in all 7 locales (`captcha_mode_specify`, `captcha_mode_invalid`, `captcha_help_msg`, `captcha_invalid_mode_error`). Cloudflare's widget option names and language codes in it are from memory and must be confirmed by the executor or research; an unconfirmed language code maps to `'auto'`.

### Amendments after research (orchestrator's calls, 2026-10-29)
- **D-25 (security, from research Pitfall 1):** A Turnstile attempt row stores a random reserved answer (`ts1:` plus 128 random bits). Both `captchaVerifyCallback` and `captchaRefreshCallback` refuse an attempt whose answer has that prefix, so a hand-built math/text callback can never pass or refresh a Turnstile attempt.
- **D-26 (security):** The challenge token is public (it is in the button URL), so the consume step first compares the verified `initData` user with the challenge's user and changes nothing for another user: a plain compare-and-delete would let any member burn the target's challenge. The per-token rate window is keyed by (token, user).
- **D-27:** `start_param` may not be inside the signed `initData`, so the page also reads `tgWebAppStartParam` from its URL; the server derives the token from whichever the page sends and binds `cData` to it (D-19). The widget's JS option is `cData` (camelCase), not `cdata`; the UI contract is corrected accordingly.
- **D-28:** The retry classification follows the research: only `invalid-input-response`, `timeout-or-duplicate` and a `cdata` mismatch count as a failed solve and run the failure action. Transport errors, 5xx, `internal-error`, `bad-request`, secret errors and hostname or action mismatches answer `retry` and re-arm the token (D-21), so members are never kicked for the owner's mistakes.
- **D-29:** The migration drops `chk_captcha_mode` with `DROP CONSTRAINT IF EXISTS` first and re-adds it `NOT VALID` with `turnstile` included, because both migration appliers swallow `duplicate_object` on `ADD CONSTRAINT`. The code lives in a new package `alita/utils/captchaweb` (a `Backend` interface, no `alita/db` import) with the implementation in `alita/modules/captcha_web.go`; no new Go dependency.
- **D-30:** In polling mode the HTTP server starts before `postInit`, so the verify route is gated on a readiness flag and answers `retry` until the bot is ready. `make check-translations` cannot see the dynamic `captcha_web_*` page keys, so a locale-parity test covers them.
- **D-31 (owner question defaulted to the researcher's recommendation):** The captcha routes share the HTTP port with `/metrics`, `/db_metrics`, `/health`, pprof and `/webhook`. The bot does not enforce a path allow-list; the deployment guide tells the owner to forward only `/captcha` and `/captcha/*` through the proxy, and the bot prints one startup warning when the web captcha is enabled and `METRICS_AUTH_TOKEN` is empty. A middleware or a separate port can follow if the proxy is ever not fully controlled.

### Claude's Discretion
- Redis key names and TTLs for challenge tokens, the rate limiters and counters, and how the per-IP limit reads the client address behind the owner's proxy (trusted forwarding header settings).
- The exact `cdata` derivation, the Turnstile `action` string, and the verify route's JSON shape and status codes.
- How the migration widens the CHECK (drop and re-add inside one transaction) and how model, repository and tests change with it.
- Wording of the challenge message, button label, page strings, the refusal reasons of `/captchamode turnstile`, and all 7 locales.
- How the shared pass path and the timeout path are factored out of `captcha.go`, and which tests move with them.
- Where the page and API code live (a new package under `alita/utils/` or inside `httpserver`), keeping `alita/db` init-order rules.

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets
- **The existing captcha:** `alita/modules/captcha.go` (2099 lines): `SendCaptcha` (mute, attempt row, message), `captchaVerifyCallback` (the pass and fail path), `handleCaptchaTimeout`, `expireCaptchaAttempt`, `executeCaptchaFailureAction`, `unmuteCaptchaUser`, pending-message replay, `StartCaptchaLifecycle` workers, and `/captchamode`, `/captchatime`, `/captchaaction`, `/captchamaxattempts`.
- **Data:** `alita/db/captcha/repository.go` (settings, attempts, stored messages, the `captcha_pending` flag that must be invalidated after an attempt insert) and `alita/db/models/captcha.go` (`CaptchaSettings.CaptchaMode` with `chk_captcha_mode IN ('math','text')`, `CaptchaAttempts` unique on (user, chat), `ExpiresAt`).
- **HTTP server:** `alita/utils/httpserver/server.go` (a mux with `/health`, `/metrics`, `/db_metrics`, pprof and the webhook path; webhook and polling modes; shutdown manager).
- **Phase 7 patterns:** swappable endpoints and `httptest` fakes for outside services, config plus `registerSecrets`, `logredact`, the developer `/stats` counters, Redis scripts with `{chat}` tags, `RegisterXxxDrain` shutdown drains.
- **Locales:** 7 locale files, `make check-translations`, and the docs generator.

### Established Patterns
- Operational Redis keys sit outside `alita:cache:`; best-effort Redis work stays under a short timeout; exactly-once effects come from conditional database writes, and Redis only sheds load or holds short-lived tokens.
- Captcha allows one attempt per (user, chat); group `-10` stores the pending user's messages for replay; any code that inserts into `captcha_attempts` must `DeleteCache` the pending flag after commit.
- Migrations are append-only and timestamped; model changes go into every test harness `AutoMigrate` list.

### Integration Points
- `SendCaptcha` branches on the mode to send the challenge message; `captchaModeCommand` and the repository accept `turnstile`.
- A shared "pass the captcha" function is extracted from `captchaVerifyCallback` and called by the web verify route; the web server needs the running bot to call Telegram (unmute, delete the challenge message, replay).
- `main.go` registers the routes when the settings are complete and a drain for in-flight verify requests; config gains the four settings and the secret registration.

</code_context>

<specifics>
## Specific Ideas

- Button label like "Verify with Turnstile" in the member's language; the challenge message names the time limit.
- The owner hosts whatever is most secure; the page must work behind a plain reverse proxy that terminates TLS.
- Owner decisions carried in: Cloudflare Turnstile through a bot-hosted Mini App, server-side verification, one solve only, math and text as fallbacks.

</specifics>

<deferred>
## Deferred Ideas

- Running the real-device spike (direct-link launch from a group, Turnstile in Telegram WebViews) with the owner's hostname, Mini App and keys; `08-SPIKE.md` is the way to do it.
- Retries or a second solve for Turnstile, other captcha providers, and the Phase 9 Captcha settings page.
- The Ed25519 third-party `initData` signature path, and serving the page from a separate static host.

</deferred>
