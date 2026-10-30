# Phase 8: Turnstile real-device checklist

This checklist is for you, the owner. It tells you how to prove, on real Telegram clients and with your own Cloudflare keys, the things the automatic tests cannot prove, what each result means, and which constant to change when a check fails. There is no tool to run: you work through numbered checks by hand.

## Why this exists

The roadmap asked for a real-device spike before the Turnstile page is trusted. You chose to defer it ("Plan and build now, spike later", D-18), because it needs a public hostname, a BotFather Mini App, your Turnstile keys and real Telegram clients, and none of those exist during the build.

The test suite fakes Telegram and Cloudflare: Cloudflare's `siteverify` is an `httptest` server, Telegram is a hand-written bot client fake, and the page script runs in Node with a stubbed `window`. So the suite proves the server logic. It cannot prove how Telegram's WebViews, Telegram Web, a real Cloudflare widget and a real `siteverify` answer behave. This checklist does.

Until you have run it, the code uses its starting values (the CSP host list, `Referrer-Policy: no-referrer`, a plain `startapp` link, the widget options and a 12 s client timeout). The feature is built to work with them. This page is how you confirm or change them.

## Before you start

1. **Finish the deployment guide** (`docs/src/content/docs/self-hosting/captcha-web.md`, published as "Web captcha (Turnstile)"): a public HTTPS hostname, a reverse proxy that forwards only `/captcha` and `/captcha/*`, the BotFather Mini App, a Managed-mode Turnstile widget whose hostname list holds your host, and the four settings (`CAPTCHA_WEB_BASE_URL`, `TELEGRAM_MINI_APP_SHORT_NAME`, `TURNSTILE_SITE_KEY`, `TURNSTILE_SECRET_KEY`), plus `CAPTCHA_WEB_TRUSTED_PROXIES` if a proxy sits in front.
2. `GET https://<your host>/captcha/health` answers `200 {"status":"ok"}`.
3. **Two Telegram accounts**: the target (the person who joins) and another member of the same test group. Use one you can leave and re-join with.
4. **Four kinds of client**: Android, iOS, Telegram Desktop and a Telegram Web client (web.telegram.org, ideally both the K and A versions).
5. **A test group** (not one of your real communities) where the bot is an administrator with the Restrict members right, set to `/captchamode turnstile`. Leave `/captchatime` at its default or raise it to a few minutes so you are not racing the timer.
6. **Keep keys out of files.** Do not paste the secret key into this file, a ticket or a screenshot. This checklist only ever names variables.
7. Have the bot's log open. It logs outcomes and counters only, never a token or a key. A configuration fault shows as `[CaptchaWeb] siteverify configuration fault: <reason>`.

## What to run, in order

For every check: do the steps, compare with "Pass looks like", and if it does not match read "If not" and "Change". Tick the box when it passes or when you have recorded what you changed.

### 1. The button opens the Mini App for the person who tapped it (A1, A3)

- [ ] Android  [ ] iOS  [ ] Desktop  [ ] Web

Steps: with the target account, join the test group. The bot mutes you and posts a message with a "Verify with Turnstile" button. Tap it.

Pass looks like: the Mini App opens as a sheet and shows the page ("Checking..." then the widget). The first time you may see a Telegram dialog asking whether to open the app (A3); note whether it appears and whether it appears again on the second launch. Write down the length of the `startapp` value in the link: it must be 22 characters (16 random bytes in base64url) and open without being cut.

If not: if the button does nothing or Telegram says the app was not found, the short name or the BotFather URL is wrong (`/myapps` in BotFather). If the sheet opens but shows "Open this in Telegram" the page got no signed data (see check 8 and the guide's troubleshooting). If the link is cut, the token is too long for this client.

Change: BotFather's short name or URL first; then `TELEGRAM_MINI_APP_SHORT_NAME`. A link that is cut on a client is a code change in `MiniAppLink` and the token length in `alita/utils/captchaweb/challenge.go`.

### 2. Is `start_param` signed in the data Telegram sends? (A2)

- [ ] recorded

Steps: Telegram's documentation contradicts itself on whether `start_param` is inside the signed `initData`. The page reads two sources, `initDataUnsafe.start_param` first and the URL's `tgWebAppStartParam` as the fallback, and the server prefers the signed value when it is present. Open the page's remote debugging console if you can (Android: `chrome://inspect` with USB debugging; Desktop: the "Enable WebView inspecting" option in Telegram's experimental settings, if your version has it; Web: the browser console) and look at which of the two carried the value.

Pass looks like: either source works, and the member passes. Record which one your clients deliver: "signed", "URL only", or "differs per client".

If not: if both are empty on a client, the page shows "This link is not valid" immediately.

Change: nothing if either source works. If a client delivers neither, report the client and version; the read order is in the page script (`readChallenge` in `alita/utils/captchaweb/page/captcha.html`).

### 3. Another member's tap is rejected and changes nothing

- [ ] done

Steps: with the second account, open the target's button (same message in the group).

Pass looks like: the page shows "This link is not valid" ("It belongs to another member or was already used. Nothing was changed."). The target's challenge is still open: the target account can still open the button and pass in check 4.

If not: if the second account passes the target, stop and treat it as a bug: the server must refuse any `initData` whose user is not the challenge's member. If the target's own challenge is gone after the second account tapped, report it too.

Change: this is a server rule, not a constant. Do not run the feature on a real group until it passes.

### 4. The widget renders and returns a token in each WebView, and the check passes (A10)

- [ ] Android  [ ] iOS  [ ] Desktop  [ ] Web

Steps: with the target account, open the button, wait for the widget, and solve it.

Pass looks like: the widget appears within a few seconds, you solve it (often just a tick), the page shows "Verifying..." and then the passed screen, the sheet closes by itself after about two seconds, and the bot unmutes you in the group. Cloudflare lists embedded browsers as "limited support", so a WebView where the widget never loads is a real finding.

If not: if the widget never appears, the page shows Try again after about 10 seconds. A WebView that blocks the script or the iframe is the likely cause; look for CSP violations in the console (check 6). If the widget appears and shows an error code, see the guide's troubleshooting (codes 110100, 110110, 110200, 400020, 400021, 400070 mean the key or the hostname list).

Change: if a platform cannot run the widget at all, there is no constant to change; record the platform and version. If the console shows a blocked request, add only that source to the policy in `CSPFor` (`alita/utils/captchaweb/page.go`).

### 5. Real `siteverify` reports your hostname

- [ ] done

Steps: pass check 4 once with your real keys, then read the bot log.

Pass looks like: no `siteverify configuration fault` line and the member passed. The server only passes when Cloudflare's answer echoes your host (the host of `CAPTCHA_WEB_BASE_URL`), the action `captcha_join` and the challenge's own data.

Also confirm the key pair belongs to one widget: in the Cloudflare dashboard open the Turnstile widget, and check that `TURNSTILE_SITE_KEY` is that widget's site key and `TURNSTILE_SECRET_KEY` is that same widget's secret key (the dashboard shows both on one page), and that neither is a Cloudflare dummy test key. A pair from two widgets does not log a configuration fault: Cloudflare calls every token "invalid" and the member's solve counts as a failure. The breaker catches this after 5 failed solves in a row (the log then shows `suspected key mismatch` once a minute, and failed solves are held as Try again), but the first 5 members would already have been handled as failures, so check the pair before real members join. The Redis key `alita:captcha:streak` holds the current count of failed solves in a row; it should be absent or at most 1 after your tests.

If not: a `hostname mismatch` fault means the BotFather URL host, `CAPTCHA_WEB_BASE_URL`'s host and the Turnstile hostname list are not the same public host. An `invalid-input-secret` or `missing-input-secret` fault means the secret does not belong to the site key. An action mismatch means a stale cached page: reload and retry. Members are not punished for these faults; they get Try again.

Change: the three hostnames and the two keys. Never use Cloudflare's dummy test keys here: their answers carry the action `test` and the cdata `test-data`, so the checks always reject them.

### 6. Telegram Web: the page is not blank (A4), and the console is clean

- [ ] Web K  [ ] Web A  [ ] Desktop

Steps: open the button on a Web client. Open the browser console on the Mini App frame. Do the same on a Managed widget on every other client you can inspect (Android and Desktop remote debugging): Cloudflare's own CSP guidance names `challenges.cloudflare.com` for scripts and frames, but the exact list for a given widget mode could not be confirmed offline, so this is the first run of the policy against a live widget. Watch the console from before the widget loads until after the passed screen, and note every CSP violation by directive and blocked source, including the ones that appear only when the widget is solved or refreshed.

Pass looks like: the page shows normally and the console holds no Content-Security-Policy violation, apart from one cosmetic `style-src` warning if Telegram's own script injects a `<style>` element in Web clients (known, harmless).

If not: a blank sheet only in the Web client means the web client's host is not in `frame-ancestors`. The console names the refused ancestor. A violation naming a `script-src`, `frame-src`, `connect-src` or `img-src` source means the widget or Telegram's script needs one more source.

Change: in `CSPFor` (`alita/utils/captchaweb/page.go`) the `frame-ancestors` list holds `https://web.telegram.org`, `https://webk.telegram.org` and `https://webz.telegram.org`; add only the host the console names. For another blocked source add only that source to the one directive that blocked it (`script-src` holds `https://telegram.org` and `https://challenges.cloudflare.com`; `frame-src` holds `https://challenges.cloudflare.com`; `connect-src` is `'self'`). Do not loosen `default-src` and do not add `unsafe-inline`.

### 7. `Referrer-Policy: no-referrer` is accepted by the widget (A6)

- [ ] done

Steps: complete check 4 on at least one client and watch the widget for error 110200 (unauthorized domain) or an endless Try again even though the hostname list is right.

Pass looks like: the widget works with the policy as served.

If not: the widget may need the referrer to detect the host. If everything else (check 5) is right and the widget still reports an unauthorized domain, the policy is the next suspect.

Change: the `Referrer-Policy` header set in `Page.ServeHTTP` (`alita/utils/captchaweb/page.go`): relax `no-referrer` to `strict-origin`. Record that you did.

### 8. `crypto.subtle` is available (Pitfall 22)

- [ ] Android  [ ] iOS  [ ] Desktop  [ ] Web

Steps: the page needs `crypto.subtle` to compute its challenge data (`cData`). It exists only on HTTPS origins. Open the page on each client over your HTTPS host.

Pass looks like: the widget renders (the widget only renders once the data is computed).

If not: a client where `crypto.subtle` is missing shows "Open this in Telegram" with the unsupported body, even with signed data present. The page deliberately has no hand-written SHA-256.

Change: confirm the Mini App URL in BotFather is `https://` (a plain-HTTP or mixed-content URL removes `crypto.subtle`). If an HTTPS client still lacks it, record client and version.

### 9. The optional `&mode=compact` link parameter (A5)

- [ ] done

Steps: the bot's link has no `mode` parameter today, so Telegram opens its default sheet. Take a link from the group (`https://t.me/<bot>/<short name>?startapp=<token>`), add `&mode=compact` by hand, and open that on each client.

Pass looks like: the sheet opens smaller (compact) and the page is still readable; or the parameter is ignored harmlessly. Record which.

If not: if compact mode cuts the widget or the buttons, do not use it.

Change: nothing if you do not want compact mode. To make the bot send it, add the parameter in `MiniAppLink` (`alita/utils/captchaweb/challenge.go`); then the 22-character check in check 1 must be re-read with the longer link.

### 10. Themes, safe areas and the 320 px sheet

- [ ] light  [ ] dark  [ ] custom theme  [ ] notch or home bar

Steps: open the page in Telegram's light theme, dark theme and a custom theme (chat settings, change colors), on a phone with a notch or a home bar, and on the narrowest window you can make (about 320 px wide). Switch theme while the page is open.

Pass looks like: text, the hint text and the buttons stay readable (the page checks contrast and falls back to its own palette when a theme is too low-contrast), nothing hides under the notch, the home bar or Telegram's header, the page follows a theme change, a swipe during the solve does not close the sheet, and the Hindi and Russian strings wrap rather than get cut.

If not: record the client, the theme and a screenshot. Unreadable text points at the contrast fallback; content hidden under a bar points at the safe-area padding.

Change: the colour fallbacks and the safe-area padding are in the page's style block (`alita/utils/captchaweb/page/captcha.html`); the contrast threshold is `MIN_CONTRAST` and the widget size switch is `COMPACT_BELOW_PX` in its script.

### 11. Whole-check latency against the 12 s client timeout

- [ ] done

Steps: pass check 4 several times on a phone with a mobile connection. Note how long the page shows "Verifying..." before it shows the passed screen. The server calls Cloudflare's `siteverify` (5 s limit), reads and writes Redis, then unmutes through Telegram.

Pass looks like: the verifying screen lasts a few seconds at most. The page gives the whole request 12 s (`VERIFY_TIMEOUT_MS`); past that it shows Try again even though the server may still have passed you.

If not: if a pass often takes more than about 6 s, find the slow part in the bot log timestamps (Cloudflare, Redis or Telegram). If a request times out on the page but the member was unmuted, record it: the server keeps the decided answer for 10 minutes, so pressing Try again and submitting again from the same member gets the same answer without a second Cloudflare call.

Change: `VERIFY_TIMEOUT_MS` in the page script (`alita/utils/captchaweb/page/captcha.html`); raise it only if latency is real and the proxy allows it. Also check the proxy's read timeout for `POST /captcha/verify`.

### 12. A failed solve, a time-out and a re-join behave as the guide says

- [ ] failed solve  [ ] time-out  [ ] re-join

Steps: (a) a failed solve is hard to cause by hand, because a normal solve returns a valid token; if you cannot cause one, mark it "not reproduced" and rely on the automatic tests for that branch; (b) open the button and wait out `/captchatime` without solving; (c) leave the group and join again, then open the button of the first attempt.

Pass looks like: (a) a provably invalid or already-used token gives the failure screen and the group's `/captchaaction` (kick, ban or mute) runs once; any other fault (network, Cloudflare error) gives Try again and kicks nobody; (b) the group's action runs at the time limit and the screen says "Time ran out"; (c) the old button answers "expired", the new button works, and the old button does not touch the new attempt.

If not: a kick on a Cloudflare or configuration error is a bug (only a provably invalid token may fail a member). A pass after the time limit is a bug.

Change: server rules, not constants. Report with the log lines.

### 13. BotFather accepts the short name (A7)

- [ ] done

Steps: the bot checks `TELEGRAM_MINI_APP_SHORT_NAME` at startup (1 to 64 letters, digits or underscores). Use the short name BotFather accepted.

Pass looks like: the bot starts, and the name BotFather accepted is also accepted by the bot.

If not: if BotFather accepts a name the bot refuses at startup (the error names the variable, never a value), the bot's rule is too strict.

Change: loosen the short-name pattern in `alita/config/config.go`, then rerun the config test.

### 14. Signed data older than 5 minutes (IN-06)

- [ ] Android  [ ] iOS  [ ] Desktop  [ ] Web

Steps: open the button, then leave the page open without solving for more than 5 minutes (stay inside `/captchatime`: raise it to 10 minutes for this check). Solve the widget after the 5 minutes, and read what the page shows and what the bot log says. Then press Try again if the page offers it, and finally close the sheet and open the button again from the group chat.

Pass looks like: the server accepts signed Telegram data only for 5 minutes after the app was opened (`InitDataMaxAge`), so the late solve is answered `invalid` and the page shows "This link is not valid" ("It belongs to another member or was already used. Nothing was changed."). That copy is wrong for this case, and the widget's Try again cannot refresh the signed data. The challenge is not consumed by a stale answer, so reopening the button from the group chat gets a fresh page and the member can still pass inside the time limit. Record exactly what the member sees, whether Try again appears, and whether reopening the button recovers.

If not: if the late solve passes, the age limit is not enforced (a bug). If reopening the button does not recover, the stale answer consumed the challenge (a bug: only a verified, well-formed request may use it up).

Change: nothing in code from this check alone. It is an open product question whether the stale case should get its own answer or copy (see the fix report's "open" note); record the evidence here. `InitDataMaxAge` is in `alita/utils/captchaweb/initdata.go`.

## What to read in the output

There is no tool output, only three places to look:

- **The client**: the sheet and the group (screens, unmute, kick).
- **The bot log**: outcomes and counters. `[CaptchaWeb] siteverify configuration fault: <reason>` names the cause of a configuration problem.
- **The developer `/stats`**: a "Web captcha (this replica, since its last restart)" section with eight numbers (issued, passed, failed, expired, invalid, retry, limited, errors) for the replica that answers. Compare them with what you did: for example `invalid` rises after check 3 and `errors` rises with a configuration fault.

## Decisions

| Question | Keep the current value if | Change it if |
|---|---|---|
| `frame-ancestors` host list | the page works in every Web client | a Web client is blank and the console names another host |
| `Referrer-Policy: no-referrer` | the widget works | the widget reports an unauthorized domain with correct keys and hostname list (relax to `strict-origin`) |
| Plain `startapp` link | all clients open it | you prefer the compact sheet and check 9 passed |
| 12 s client timeout | passes finish well under 12 s | real latency is above it and the proxy allows longer |
| Widget options (Managed, `appearance: always`, retry never) | check 4 passes everywhere | a client needs a different appearance; this is a code change in `renderWidget` |
| `start_param` read order | either source works on every client | both are empty on a client you care about |

## Which constants to change afterwards

| What | Where |
|---|---|
| `frame-ancestors`, `connect-src` and the other CSP sources | `CSPFor` in `alita/utils/captchaweb/page.go` |
| `Referrer-Policy` header | `Page.ServeHTTP` in `alita/utils/captchaweb/page.go` |
| Widget options (`appearance`, `retry`, `refresh-*`, `size`, `theme`) | `renderWidget` in `alita/utils/captchaweb/page/captcha.html` |
| Compact-sheet link parameter (`mode=compact`) | `MiniAppLink` in `alita/utils/captchaweb/challenge.go` |
| Client timeout (12 s) | `VERIFY_TIMEOUT_MS` in `alita/utils/captchaweb/page/captcha.html` |
| Contrast and compact widget thresholds | `MIN_CONTRAST` and `COMPACT_BELOW_PX` in `alita/utils/captchaweb/page/captcha.html` |
| `initData` age limit (5 minutes) | `InitDataMaxAge` in `alita/utils/captchaweb/initdata.go` |
| Short-name pattern | `alita/config/config.go` |
| Hostnames, keys, proxy list | the four settings, `CAPTCHA_WEB_TRUSTED_PROXIES`, the proxy, BotFather and the Cloudflare dashboard (no code change) |

After any code change run `make test` and `make lint`, and when the page changes run `node --test scripts/captcha-page/*.test.mjs`.

## Checks only a live run can do

These are the same behaviours `08-VALIDATION.md` lists as manual-only, with the check that covers each:

1. The direct link opens the Mini App for the tapper on Android, iOS, Desktop and Web, delivers that user's signed data, and another member's tap is refused: checks 1 and 3.
2. Whether `start_param` is signed, the `startapp` length and the first-launch dialog: checks 1 and 2.
3. The widget renders and returns a token in each WebView, and real `siteverify` reports your hostname: checks 4 and 5.
4. The Content-Security-Policy and the `frame-ancestors` list in every client, and `Referrer-Policy`: checks 6 and 7.
5. `crypto.subtle`, compact mode, themes and safe areas: checks 8, 9 and 10.
6. The latency of the whole check against the 12 s client timeout: check 11.
7. Failed solve, time-out and re-join with real keys: check 12.
8. BotFather accepts your short name and URL: check 13.
9. `/captchamode turnstile` in a group where the bot is an administrator with Restrict members replies with the confirmation, and with the right removed refuses and keeps the mode (plan 08-09). Run it once next to check 1.
10. The `captcha_web_*` strings in es, fr, hi, id, pt and ru read naturally, if you can read them, or ask a native speaker (plan 08-07). Check 10 shows them on the page.
11. CI only, not yours: `TestCaptchaModeTurnstileConstraint` runs against PostgreSQL in `make test-postgres-integrity` (plan 08-04).
12. Added by the code review: signed data older than 5 minutes and what the member sees (check 14), and the CSP against a Managed widget on every inspectable client (check 6).

## What the checklist never does

- It makes no automated call to Telegram or Cloudflare. You do each step by hand.
- No key or secret goes into this file or any other file. Variable names only.
- It changes nothing in a live group without you: use a test group, and restore `/captchamode` in any group you borrowed.
- It does not touch your database. The one database check (the `turnstile` mode constraint) belongs to CI.
