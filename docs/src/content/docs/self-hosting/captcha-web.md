---
title: Web captcha (Turnstile)
description: Put the Cloudflare Turnstile web captcha online - hostname, reverse proxy, BotFather Mini App, Turnstile widget, settings and troubleshooting.
---
<!-- MANUALLY MAINTAINED: do not regenerate -->

# Web captcha (Turnstile)

The web captcha is a third captcha mode next to `math` and `text`. A new member taps a button in the group, a Telegram Mini App opens a page served by your bot, and a Cloudflare Turnstile widget runs inside it. The bot's server decides the result: the page only shows what the server answered. Math and text captcha are unchanged and keep working without any of this setup.

The mode is off until you set four variables. This page walks through every step from a public hostname to a passing check.

## Before you start

You need:

- A **domain name** you control and a **reverse proxy with TLS** (Caddy, nginx or similar) in front of the bot. Telegram only opens Mini Apps over HTTPS.
- A **Cloudflare account** (the free plan is enough) to create a Turnstile widget.
- Access to **BotFather** with the bot's owner account.
- **Redis**. The challenge handles, the abuse limits and the decided answers live in Redis, so every replica shares them. Without Redis the mode is unavailable.

:::caution[One port, many endpoints]
The captcha routes are served on the bot's normal HTTP port (`HTTP_PORT`, default 8080). That same port also serves `/metrics`, `/db_metrics`, `/health`, `/debug/pprof/*` (when enabled) and the webhook. Your public proxy must forward **only** `/captcha` and `/captcha/*` to the bot. See step 1.
:::

## 1. A public HTTPS hostname and the proxy

Point a hostname (for example `captcha.example.com`) at your proxy and let the proxy obtain a certificate. The proxy forwards exactly two kinds of path to the bot's `HTTP_PORT`:

| Path | Method | What it is |
|------|--------|-----------|
| `/captcha` (exact) | GET | The Mini App page |
| `/captcha/verify` | POST | The page sends the Turnstile token here |
| `/captcha/health` | GET | Readiness check (step 5) |

Everything else must be answered by the proxy itself (a 404), never forwarded. The page calls `/captcha/verify` on its own host, so the page and the verify route must be reachable on the same hostname.

### Caddy

```caddy
captcha.example.com {
	@captcha path /captcha /captcha/*
	handle @captcha {
		reverse_proxy 127.0.0.1:8080
	}
	handle {
		respond 404
	}
}
```

Caddy terminates TLS, obtains the certificate and sets `X-Forwarded-For`. If the proxy connects to the bot from `127.0.0.1`, the matching setting is `CAPTCHA_WEB_TRUSTED_PROXIES=127.0.0.1,::1`.

### nginx

```nginx
server {
    listen 443 ssl http2;
    server_name captcha.example.com;

    ssl_certificate /etc/letsencrypt/live/captcha.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/captcha.example.com/privkey.pem;

    location = /captcha {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    location /captcha/ {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    location / {
        return 404;
    }
}
```

With this proxy on the same host the matching setting is `CAPTCHA_WEB_TRUSTED_PROXIES=127.0.0.1,::1`. If the proxy reaches the bot from another address (for example across a Docker network), list the address the **bot sees as the peer**, not the address of your users.

### Why only these paths

If you forward the whole port, `/metrics` and `/db_metrics` (open unless `METRICS_AUTH_TOKEN` is set), `/health` (version and uptime), the pprof endpoints and the webhook path become reachable from the internet. The bot enforces no path list itself. When the web captcha is on, the bot logs a warning at startup if `METRICS_AUTH_TOKEN` is empty, and another when `CAPTCHA_WEB_TRUSTED_PROXIES` is empty. Treat both warnings as a prompt to check your proxy.

If you also run the bot in webhook mode behind the same hostname, add the webhook path to the proxy as a separate, explicit rule. Do not widen the captcha rule.

## 2. Register the Mini App with BotFather

1. In BotFather send `/newapp` and choose your bot.
2. Give it a title and a short description. BotFather also asks for a picture; the app is only opened by direct link, so any picture works.
3. When it asks for the **Web App URL**, send `https://captcha.example.com/captcha` (the base URL plus `/captcha`).
4. When it asks for the **short name**, send a name made of letters, digits and underscores, for example `verify`. This is `TELEGRAM_MINI_APP_SHORT_NAME`.

To change the URL later, send `/myapps` to BotFather, pick the bot and the app, and choose **Edit link**.

The bot builds each member's button as `https://t.me/<bot username>/<short name>?startapp=<token>`. The token is a short random handle that identifies one member's challenge in one group.

## 3. Create the Cloudflare Turnstile widget

1. In the Cloudflare dashboard open **Turnstile** and add a widget.
2. Set **Widget Mode** to **Managed**.
3. In the **Hostname Management** list put exactly your public host, for example `captcha.example.com`. Use the bare host name: no `https://`, no port, no path. A listed host also authorizes all of its subdomains.
4. Save, then copy the **site key** (public) and the **secret key** (private).

:::danger[Three hostnames must agree]
The host in the BotFather URL, the host in `CAPTCHA_WEB_BASE_URL` and the Turnstile hostname list must all be the same public host. If they drift, Cloudflare reports a different hostname than the bot expects and every check ends in "Try again". See Troubleshooting.
:::

:::caution[Cloudflare's dummy test keys never pass]
Cloudflare publishes dummy site and secret keys for testing. Their answers carry the action `test` and the cdata `test-data` whatever the page sent, so the bot's action, cdata and hostname checks always reject them. Use your real keys to test end to end.
:::

## 4. Set the four settings

Set these in the bot's environment and restart:

| Variable | What it holds |
|----------|---------------|
| `CAPTCHA_WEB_BASE_URL` | `https://captcha.example.com`, the public address of the bot's HTTP port as seen from the internet. |
| `TELEGRAM_MINI_APP_SHORT_NAME` | The short name from BotFather. |
| `TURNSTILE_SITE_KEY` | The widget's site key (public). |
| `TURNSTILE_SECRET_KEY` | The widget's secret key. |

An optional fifth setting, `CAPTCHA_WEB_TRUSTED_PROXIES`, lists the IPs or CIDRs of your reverse proxies (for example `127.0.0.1,::1`). The bot reads `X-Forwarded-For` only from a peer in this list. Without it every request seems to come from your proxy, so the per-address abuse limit is skipped; the per-member limits still apply. Setting it alone, without the four above, stops startup.

All four main settings are **all or nothing**. With none of them set the bot starts and the web captcha is off. With some but not all set the bot refuses to start and the error names each missing variable, never a value. Each variable is described in the [environment reference](/api-reference/environment/): [`CAPTCHA_WEB_BASE_URL`](/api-reference/environment/), [`TELEGRAM_MINI_APP_SHORT_NAME`](/api-reference/environment/), [`TURNSTILE_SITE_KEY`](/api-reference/environment/), [`TURNSTILE_SECRET_KEY`](/api-reference/environment/) and [`CAPTCHA_WEB_TRUSTED_PROXIES`](/api-reference/environment/).

The secret key is registered with the log redactor at startup, so its exact value is scrubbed from every log line. Keep it out of version control and out of screenshots anyway.

## 5. Check it

From anywhere on the internet:

```bash
curl -s https://captcha.example.com/captcha/health
```

| Answer | Meaning |
|--------|---------|
| `200 {"status":"ok"}` | The bot finished starting and Redis answers a `PING` within 250 ms. |
| `503 {"status":"unavailable"}` | The bot is still starting (in polling mode the HTTP server starts before the modules load), or Redis is down. |

The health answer never names a setting, an ID or a secret. Opening `https://captcha.example.com/captcha` in a normal browser shows, after a few seconds, a page titled "Open this in Telegram"; that is expected, because the page needs Telegram's signed data.

## 6. Turn it on in a group

An admin of the group runs:

```
/captchamode turnstile
```

The command keeps its usual admin check and then refuses, writing nothing, in this order:

| Refusal | What it needs |
|---------|---------------|
| "Turnstile captcha is not set up on this bot yet" | All four settings set, and the bot has a username. |
| "Turnstile captcha is unavailable right now because the bot's shared storage is down" | Redis answering a fresh `PING` within 250 ms. |
| "I need to be an admin with the Restrict members right" | The bot is the creator or an administrator with the Restrict members right in that group, checked live with Telegram. |

`/captchatime` (the time limit, default 2 minutes) and `/captchaaction` (`kick`, `ban` or `mute`, default `kick`) apply to this mode exactly as they do to math and text.

## What is sent where

- **To Cloudflare, from the bot's server:** the Turnstile token the member's browser produced and your secret key, to Cloudflare's `siteverify` endpoint. Nothing about the member (no name, ID, language or IP) is sent. The member's own browser talks to Cloudflare directly to load and run the widget.
- **To Telegram:** the usual mute, unmute, kick, ban and message calls, as with the other modes.
- **In your proxy's access log:** the link Telegram opens carries the challenge handle as `tgWebAppStartParam` in the URL. If your proxy logs full URLs it may record that value. It is a short-lived handle bound to one member and one group attempt, not a credential: another member who uses it is refused, and it stops working when the attempt ends.
- **Never written anywhere:** the signed Telegram data, the Turnstile token and Cloudflare's answers are not logged and not stored.

The member's browser loads two scripts, `telegram.org/js/telegram-web-app.js` and `challenges.cloudflare.com/turnstile/v0/api.js`. A content blocker that blocks either one leaves the page without Telegram's data or without the widget, and the member sees "Open this in Telegram" or Try again.

## How the mode behaves

- **One solve per member.** Each member has one attempt per group. A re-join replaces the attempt, and the old button then answers "expired" without touching the new attempt.
- **The server decides.** A pass needs three things together: signed Telegram data whose user is the member the challenge was issued to, a Cloudflare confirmation, and a Cloudflare answer that echoes your hostname, the action `captcha_join` and the challenge's own data. The page cannot pass anyone by itself.
- **`failed` or `retry`.** Only a token Cloudflare calls invalid or already used, or a token minted for another challenge, fails a member: the group's action runs once and the member sees a failure screen. Anything else is `retry` and kicks nobody: a Cloudflare outage, a timeout, a bad or unauthorized key, a hostname mismatch, or the bot missing the Restrict right. A run of 5 failed solves with no pass in between also turns further failures into `retry` (see Troubleshooting, "suspected key mismatch"). The member can press Try again, and your configuration faults are logged once at Error as `[CaptchaWeb] siteverify configuration fault: <reason>` (the reason name only, never a value).
- **Time limit.** The group's `/captchatime` still applies. A member who does not solve in time is handled with the group's `/captchaaction`, as in the other modes.
- **Math instead.** If the web captcha is unavailable at the moment a member joins (the settings are off, the bot has no username, or Redis does not answer in time), that member silently gets the math captcha. No extra message is posted. A Turnstile mode stored on a bot that cannot run it behaves the same way.
- **Several replicas** share Redis and the database. A challenge issued on one replica can pass on another, and a pass, a failure and a timeout are each decided exactly once. A Redis flush loses the open challenges; those members then time out as usual and can re-join.

## Troubleshooting

**The widget shows an error and "Try again" never ends.** Cloudflare's widget error codes `110100`, `110110`, `110200`, `400020`, `400021` and `400070` mean a wrong site key, a disabled key, or a hostname that is not authorized for the widget. The page cannot tell these apart from a flaky network, so the member just sees Try again. Check that `TURNSTILE_SITE_KEY` is the key of the widget you configured, and that the widget's hostname list holds exactly the public host (step 3).

**The log shows `siteverify configuration fault`.** The reason names the cause. A secret error means `TURNSTILE_SECRET_KEY` does not belong to the site key. A hostname or action mismatch means Cloudflare saw the page on a different host than `CAPTCHA_WEB_BASE_URL`: fix the three-way agreement of step 3. Members are not punished for these faults; they get `retry`.

**Members solve the widget but keep seeing Try again, and the log shows `suspected key mismatch`.** Cloudflare answers "invalid token" for a token that was minted by a different widget than the one your secret belongs to. That happens when `TURNSTILE_SITE_KEY` and `TURNSTILE_SECRET_KEY` were copied from two different widgets, or when Cloudflare's dummy test keys are mixed with real ones. The bot cannot tell that apart from a member who fails, so it protects your members: the first 5 failed solves in a row (with no successful pass between them) are handled as failures, and from the sixth on a failed solve is held as Try again. The member stays muted with their attempt intact, nobody is removed, and the group's time limit and `/captchaaction` still apply to anyone who really does not solve. The bot logs the reason once a minute at Error. Check that the site key and the secret are the two keys of one widget, that neither is a Cloudflare test key, then restart. The first successful pass ends the hold at once.

**The sheet is blank, but only in Telegram Web.** The page is allowed to be framed only by Telegram's web clients (`web.telegram.org`, `webk.telegram.org`, `webz.telegram.org`). If Telegram serves its web client from another host, the page stays blank there while the mobile and desktop apps work. The host list is the `frame-ancestors` part of the page's Content-Security-Policy header (`CSPFor` in `alita/utils/captchaweb/page.go`); open the browser console of the blank frame to read which host was refused, and add only that host.

**The page says "Open this in Telegram" and "Use the button in the group chat".** The page did not get Telegram's signed data. Open it from the button the bot posted, not by pasting the URL, and turn off content blockers.

**"This link is not valid".** The link belongs to another member or was already used, and nothing was changed. The signed Telegram data is also only accepted for 5 minutes after the app was opened, so a member who waited longer should open the button again from the group chat. Only the member the challenge was issued to can use it.

**`/captchamode turnstile` is refused.** Match the refusal text with the table in step 6.

**`GET /captcha/health` answers 503.** The bot is still starting, or Redis is down. Check Redis first.

**New members got a math captcha instead of Turnstile.** The web captcha was unavailable when they joined: check `/captcha/health`, the four settings and Redis. If the bot runs with the cache disabled (`DISABLE_CACHE=true`, so it has no Redis connection), the bot logs one warning at startup and does not mount the captcha routes at all: `/captcha/health` then answers 404, Turnstile groups get the math captcha and `/captchamode turnstile` is refused.

**Everything is slow or members see a time-out.** The page gives the whole check 12 seconds. Look at the latency between the bot and Cloudflare's `siteverify`, and at the proxy's timeouts for `POST /captcha/verify`.
