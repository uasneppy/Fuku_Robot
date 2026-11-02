# API Coverage — Cloudflare Turnstile and the Telegram Mini App surface

> Full coverage by default. Opt-outs are explicit, reasoned decisions.
> Surface enumerated from 08-RESEARCH.md "## API Coverage": Cloudflare's documentation source
> (server-side validation, widget configurations, client-side rendering, content security policy,
> supported languages, testing, client-side error codes, hostname management, mobile implementation)
> and the Telegram Mini Apps material listed in the research's sources. Telegram's official pages
> were unreachable during research, so the Telegram rows rest on mirrors and are re-checked by the
> owner's live checklist (08-SPIKE.md, plan 08-11). Every INTEGRATE row names the plan task that
> builds it. No live call to Cloudflare or Telegram is made in this phase; tests use `httptest`.

| capability | decision | reason |
|---|---|---|
| Cloudflare `POST /turnstile/v0/siteverify` with a JSON body | INTEGRATE | The only trusted verification (CAPT-04); `captchaweb.Client.Verify` built in plan 08-01 Task 1, 5 s timeout and no redirects |
| siteverify request field `secret` | INTEGRATE | Required; the secret travels only in this body, registered with logredact in plan 08-02 Task 1 |
| siteverify request field `response` (the widget token, max 2048) | INTEGRATE | Required; plan 08-01 Task 1, length-checked by the verify handler |
| siteverify request field `remoteip` | OPT-OUT | Optional in Cloudflare's contract; the address seen behind the owner's reverse proxy is unreliable and sending member IPs adds a disclosure for no gain |
| siteverify request field `idempotency_key` | OPT-OUT | The page's Try again produces a fresh widget token, so a server-side retry of a possibly spent token has no use |
| siteverify form-encoded requests | OPT-OUT | JSON is accepted and is the one format the client sends |
| siteverify response `success` and `error-codes` | INTEGRATE | The D-28 outcome table: Pass in plan 08-01 Task 1, the full Failed versus Retry classification in plan 08-06 Task 1 |
| siteverify response `hostname`, `action`, `cdata` | INTEGRATE | Bind a token to the owner's page and to one challenge (D-03, D-19); checked in plan 08-01 Task 1, mismatches classified in plan 08-06 Task 1 |
| siteverify response `challenge_ts` | OPT-OUT | Cloudflare already enforces the 300 s lifetime and single use, reported as timeout-or-duplicate |
| siteverify response `metadata.ephemeral_id` | OPT-OUT | Enterprise-only field; not available to the owner's plan |
| Turnstile explicit rendering (`api.js?render=explicit`, `turnstile.render`) | INTEGRATE | Per-challenge `cData` and `action` and a controlled reset; plan 08-05 Task 1 |
| Turnstile implicit rendering (the `cf-turnstile` class) | OPT-OUT | Cannot bind per-challenge data cleanly before the challenge is read from the launch parameters |
| Widget options `sitekey`, `action`, `cData`, `theme`, `size`, `language` | INTEGRATE | Set by the UI contract with the D-27 correction (`cData` camelCase); plan 08-05 Tasks 1 and 3 |
| Widget options `appearance`, `retry`, `refresh-expired`, `refresh-timeout` | INTEGRATE | Set to always and never so the page, not the widget, decides when to reset (UI contract); plan 08-05 Tasks 1 and 3 |
| Widget callbacks `callback`, `error-callback`, `expired-callback` | INTEGRATE | The ready, verifying and retry states; plan 08-05 Tasks 1 and 3 |
| Widget callback `timeout-callback` | INTEGRATE | Moves the page to the retry state like the other widget problems; plan 08-05 Tasks 1 and 3 |
| Widget callbacks `before-interactive-callback`, `after-interactive-callback` | OPT-OUT | No UI state of the approved contract needs them |
| Widget `unsupported-callback` | OPT-OUT | The 8 s and 10 s loading timers and the widget's own message cover an unsupported browser; revisited after the owner's spike |
| `turnstile.reset` and the `onload` ready hook (`captchaTurnstileReady`) | INTEGRATE | Try again and safe render timing; plan 08-05 Tasks 1 and 3 |
| `turnstile.remove`, `getResponse`, `isExpired`, `execute` | OPT-OUT | The page renders once per load and submits from the callback; none of these is needed |
| Turnstile execution mode `execute` | OPT-OUT | The widget runs on render (appearance always); a deferred execute step adds nothing for a single check |
| Widget options `response-field`, `tabindex`, `retry-interval`, `offlabel-*` | OPT-OUT | There is no form; Cloudflare's defaults are acceptable for the single widget |
| Widget options `response-field-name`, `feedback-enabled` | OPT-OUT | There is no form field to name, and Cloudflare's feedback default is acceptable |
| Widget mode Managed | INTEGRATE | The mode the guide tells the owner to create (D-15); documented in plan 08-11 Task 1 |
| Widget modes Non-Interactive and Invisible | OPT-OUT | Managed is Cloudflare's recommendation and the mode the UI contract was designed for |
| Turnstile supported languages (en, es, fr, hi, id, pt, ru) | INTEGRATE | All 7 bot locales are in Cloudflare's table; any other code maps to auto; plan 08-05 Task 2 |
| Turnstile content-security-policy guidance (nonce on api.js, frame-src) | INTEGRATE | The page's per-response nonce CSP (D-23); plan 08-05 Task 1 |
| Turnstile client-side error codes (110100, 110200, 400020 and the rest) | INTEGRATE | Documented for the owner's troubleshooting in plan 08-11 Task 1; the page maps every widget error to retry as the UI contract says |
| Turnstile hostname management (FQDN list, subdomains included) | INTEGRATE | The guide's widget step and the hostname check; plan 08-11 Task 1 and plan 08-06 Task 1 |
| Turnstile test sitekeys and secrets | OPT-OUT | Their answers carry action test and cdata test-data, so they can never pass a real check (research Pitfall 20); the suite uses `httptest` and the guide mentions them for smoke tests only |
| Pre-clearance, analytics and widget management APIs | OPT-OUT | The owner creates the widget in the dashboard; nothing in the bot needs these |
| Other Challenge Platform products and ephemeral IDs | OPT-OUT | Not needed by a single Managed widget; ephemeral IDs are Enterprise-only |
| Telegram direct Mini App link `t.me/<bot>/<app>?startapp=<value>` | INTEGRATE | The only way to open a Mini App from a group message (D-06); `captchaweb.MiniAppLink` in plan 08-01 Task 1 |
| Direct link parameter `mode=compact` | OPT-OUT | Cosmetic and unconfirmed (research A5); the owner tries it in spike check 9 (plan 08-11 Task 2) |
| Direct link parameter `mode=fullscreen` | OPT-OUT | A full-screen sheet is not wanted for a one-tap check |
| BotFather `/newapp` and `/myapps` (Edit link) | INTEGRATE | Registration and URL change steps of the deployment guide; plan 08-11 Task 1 |
| Bot API `InlineKeyboardButton.url` | INTEGRATE | The group-compatible launch button; plan 08-01 Task 1 |
| Bot API `InlineKeyboardButton.web_app`, the menu button, the attachment menu | OPT-OUT | `web_app` buttons work only in private chats with the bot; the menu button and attachment menu are not a group join flow |
| `initData` fields `user`, `auth_date`, `hash`, `signature`, `start_param` | INTEGRATE | Authentication (D-02) and the challenge handle (D-27); plan 08-01 Task 1, hardened in plan 08-03 Task 1 |
| `initData` fields `chat_type`, `chat_instance`, `query_id`, `chat` | OPT-OUT | Not needed to decide a pass; the signature still covers them because every field but hash is in the data-check string |
| `initData` field `receiver` | OPT-OUT | Only set for attachment-menu launches; not needed to decide a pass and still covered by the signature |
| `initData` field `can_send_after` | OPT-OUT | Not needed to decide a pass; still covered by the signature with every other field |
| Bot-token HMAC validation of `initData` | INTEGRATE | Locked decision D-02; plan 08-01 Task 1 and the published known-answer vector in plan 08-03 Task 1 |
| Third-party Ed25519 `signature` validation | OPT-OUT | Deferred by CONTEXT.md (Deferred Ideas); the bot holds the token, so the HMAC path is sufficient |
| Launch parameter `tgWebAppStartParam` in the page URL | INTEGRATE | Second source of the challenge when `start_param` is not signed (D-27, research Pitfall 3); plan 08-05 Task 1 |
| `Telegram.WebApp` `initData`, `initDataUnsafe`, `ready`, `close` | INTEGRATE | The page relays `initData`, reads the language and `start_param` from `initDataUnsafe`, and closes after a pass (UI contract); plan 08-05 Tasks 1 and 3 |
| `Telegram.WebApp` `colorScheme`, `themeParams`, `themeChanged` | INTEGRATE | Light, dark and custom themes with the runtime contrast guard (UI contract); plan 08-05 Tasks 1 and 2 |
| `Telegram.WebApp` `isVersionAtLeast`, `disableVerticalSwipes` | INTEGRATE | Keeps a swipe from closing the sheet during the solve on clients that support it; plan 08-05 Task 1 |
| `Telegram.WebApp` `setHeaderColor`, `setBackgroundColor`, `HapticFeedback` | INTEGRATE | Header and background match the theme; optional success and error haptics; plan 08-05 Task 1 |
| `Telegram.WebApp` `expand`, `requestFullscreen`, Main/Back buttons | OPT-OUT | Not part of a verification page; the UI contract uses its own single in-page button |
| `Telegram.WebApp` SecondaryButton and SettingsButton | OPT-OUT | The UI contract uses its own single in-page button |
| `Telegram.WebApp` popups, `openLink`, `sendData`, `CloudStorage` | OPT-OUT | Not part of a verification page; the page needs no storage, no popup and sends nothing to a chat |
| `Telegram.WebApp` biometrics, location, sensors, `shareToStory` | OPT-OUT | Device features a verification page has no use for |
| `Telegram.WebApp` `addToHomeScreen` | OPT-OUT | A verification page is opened once per join and is not an app to keep |
| `Telegram.WebApp` `switchInlineQuery`, `invokeCustomMethod` | OPT-OUT | Inline mode and custom methods are not used by the captcha |
| Bot API `answerWebAppQuery`, `savePreparedInlineMessage` | OPT-OUT | No result is sent from the Mini App to a chat; the bot acts on the server's decision through its existing captcha calls |
| Bot API `setChatMenuButton` | OPT-OUT | The menu button is a private-chat entry point, not part of the group join flow |
