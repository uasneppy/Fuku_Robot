---
phase: 08-turnstile-web-captcha
reviewed: 2026-10-30T02:35:00Z
depth: deep
files_reviewed: 38
files_reviewed_list:
  - alita/utils/captchaweb/challenge.go
  - alita/utils/captchaweb/initdata.go
  - alita/utils/captchaweb/siteverify.go
  - alita/utils/captchaweb/verify.go
  - alita/utils/captchaweb/limiter.go
  - alita/utils/captchaweb/clientip.go
  - alita/utils/captchaweb/page.go
  - alita/utils/captchaweb/page/captcha.html
  - alita/utils/captchaweb/stats.go
  - alita/utils/captchaweb/fixtures_testtools.go
  - alita/utils/captchaweb/clientip_test.go
  - alita/utils/captchaweb/initdata_test.go
  - alita/utils/captchaweb/leaf_test.go
  - alita/utils/captchaweb/limiter_test.go
  - alita/utils/captchaweb/page_test.go
  - alita/utils/captchaweb/siteverify_test.go
  - alita/utils/captchaweb/verify_limits_test.go
  - alita/utils/captchaweb/verify_outcomes_test.go
  - scripts/captcha-page/captcha-page.test.mjs
  - alita/modules/captcha_web.go
  - alita/modules/captcha.go
  - alita/modules/devs.go
  - alita/modules/captcha_web_test.go
  - alita/modules/captcha_web_outcomes_test.go
  - alita/modules/captcha_web_wiring_test.go
  - alita/modules/captcha_mode_turnstile_test.go
  - alita/utils/httpserver/server.go
  - main.go
  - alita/config/config.go
  - alita/config/captcha_web_test.go
  - alita/i18n/table.go
  - alita/i18n/captcha_web_locale_test.go
  - alita/db/captcha/repository.go
  - alita/db/models/captcha.go
  - migrations/20261030120000_allow_turnstile_captcha_mode.sql
  - alita/db/migrations/captcha_mode_test.go
  - Makefile
  - sample.env
findings:
  critical: 0
  warning: 7
  info: 6
  total: 13
status: issues_found
---

# Phase 8: Code Review Report

**Reviewed:** 2026-10-30T02:35:00Z
**Depth:** deep
**Files Reviewed:** 38
**Status:** issues_found

## Summary

The Turnstile web captcha was reviewed across the `captchaweb` package, the module-side
`Backend`, the callback guards, the HTTP wiring, the config, the migration and the tests.
`go test -tags testtools ./alita/utils/captchaweb`, the captcha-related module tests under
`-race`, and `node --test scripts/captcha-page/captcha-page.test.mjs` all pass in this checkout.
The migration test that needs PostgreSQL was not run (PostgreSQL was not started).

The security core is sound. I traced each of the high-severity invariants and found no way to break them:

1. **Wrong user.** `ConsumeIfUser` compares the signed `user.id` with the challenge's user inside one Lua
   script and changes nothing for another user. The per-token window is keyed by (token, verified user).
   A different `start_param` is refused.
2. **Page cannot pass.** A pass needs signed initData, then siteverify with matching hostname, action and
   `cdata`. The `cdata` is recomputed server-side from the token the user was bound to.
3. **Forged callbacks.** `captcha_verify` and `captcha_refresh` both refuse a `ts1:` answer before any
   comparison, count or write. `grep` of every reader of `captcha_attempts.answer` shows no unguarded path.
4. **Exactly once.** A pass, a failed solve and a timeout are all decided by the conditional delete of the
   attempt row.
5. **Secrets and redirects.** No log or error carries a secret, token or initData. Redirects are refused on
   every transport, including a caller-supplied one.
6. **Migration.** It drops both CHECKs and re-adds a `NOT VALID` one. I traced it through `cleanSupabaseSQL`
   and the Go splitter, and through the perl rewrite in `migrate_psql.sh`; it is correct for both appliers.

No blocker was found. The warnings are failure-path defects and one operator-error amplifier:

- A challenge can be burnt and answered "expired" while the attempt is still live (WR-01).
- A member whose pass was recorded is shown a terminal "invalid" screen (WR-02).
- A misconfigured key pair kicks real joiners (WR-04).
- A few smaller correctness and hardening gaps follow (WR-03, WR-05, WR-06, WR-07).

## Warnings

### WR-01: A lost claim is answered "expired" and the challenge is burnt even when the attempt row is still live

**File:** `alita/utils/captchaweb/verify.go:575-576, 596-599`, `alita/modules/captcha_web.go:308-311, 382-383`, `alita/modules/captcha.go:1351-1373`

**Issue:** `pass()` maps `PassLost` to `c.expire()` and `fail()` maps `claimed == false` to `c.expire()`.
Both assume that "no claim" means "the row is gone". That is not what the Backend reports.

- `Backend.Pass` returns `PassLost, nil` whenever `CompleteCaptchaAttemptAtomic` matches no row. That
  statement also requires `message_id = ?` and `refresh_count = ?` to equal the values read a few
  milliseconds earlier. `SendCaptcha` writes `message_id` only after `SendMessage` returns
  (`UpdateCaptchaAttemptMessageID`). A tap in that window reads `message_id = 0`, the update lands, the claim
  matches nothing, and the member is told "Time ran out" for an attempt that is still live.
- `Backend.Fail` returns `claimed, nil`. `handleCaptchaTimeout` returns `(false, false)` on a transient
  `GetCaptchaAttemptByID` error and on a `ReleaseCaptchaAttemptAtomic` error. Both are database faults that
  changed nothing. The `Backend` contract in `verify.go:96-98` says such a fault must be an error so the
  challenge is re-armed, but the error is swallowed.

In both cases `expire()` then calls `Backend.Expire`, which deliberately leaves a live row to the timer.
The handler still writes `Done=expired` for 10 minutes and answers `expired` with the action. The challenge
is not re-armed, so the member cannot try again. The row survives and the timer later kicks them for real.
A one-off database blip or a sub-second race therefore costs a legitimate member their only attempt, and the
page tells them they were already removed.

**Fix:** Make "lost" distinguish "gone" from "changed". After `PassLost` or an unclaimed `Fail`, re-read with
`c.h.backend.Attempt`. If the attempt is still `Live`, call `c.retry(http.StatusOK)` (re-arm) instead of
`c.expire()`. Only expire when the row is confirmed gone or expired. In `Backend.Fail`, read the row again
after an unclaimed `handleCaptchaTimeout` and return an error when it still exists:

```go
claimed, _ := handleCaptchaTimeout(...)
if !claimed {
    if again, err := pendingTurnstileAttempt(rec); err != nil || again != nil {
        return false, errors.Join(err, errors.New("attempt still pending after an unclaimed fail"))
    }
}
return claimed, nil
```

### WR-02: After `PassUnmuteFailed` the member passed, but the page ends in a terminal "invalid" screen

**File:** `alita/utils/captchaweb/verify.go:577-583`, `alita/modules/captcha_web.go:312-316`

**Issue:** When the claim wins but the unmute fails, the attempt row is already deleted. The Backend deletes
the challenge message and leaves a durable `captcha_muted_users` retry, so the member will be unmuted. The
handler answers `retry` without re-arming and without writing a `Done` marker. The page shows "Something went
wrong ... try again". When the member taps Try again and solves a fresh widget token, `ConsumeIfUser` finds
nothing, `Done` finds nothing, and the answer is `invalid`. That is terminal on the page ("This link is not
valid ... Nothing was changed", with a Close button). A member who passed ends up on a dead end, and the
challenge message is gone too, so they have no signal that the worker will unmute them. `AGENTS.md` documents
that the token is not re-armed, but nothing documents that the second tap yields "invalid".

**Fix:** The pass is decided, so remember it. In the `default` branch of `pass()` call
`c.decided(ResultPassed, "")` before answering. A repeat then replays `passed`, with no side effect. Consider
answering `passed` directly, because the claim is irreversible and the retry is durable. The first reply can
stay `retry` if the intent is to reflect the failed unmute.

### WR-03: A Redis error or parse failure after the Lua delete loses the challenge without a re-arm

**File:** `alita/utils/captchaweb/challenge.go:255-286`, `alita/utils/captchaweb/verify.go:390-396`

**Issue:** `consumeScript` deletes the key atomically in Redis. `ConsumeIfUser` then reads the reply under a
250 ms context derived from `r.Context()`. Two failure modes leave the key deleted while the caller gets an
error and holds no record to re-arm:

- The context expires, or the client disconnects, after Redis executed the script but before the reply is
  read. `Run` returns an error, `Verify` answers 503 `retry`, and the challenge is gone.
- `parseConsumed` rejects the stored hash after `DEL` ran, so the code returns `ErrInvalidChallenge`.

The package comment promises "re-arm only where nothing was claimed", but here something was consumed. The
member's next tap is `invalid`, and the attempt times out as a kick. Both are rare, but the first one is a
natural result of the 250 ms bound under load.

**Fix:** Return the pre-delete record to the caller on a parse failure, or avoid deleting before the reply is
safe. One option is to `RENAME` the key to a short-lived `...:inflight:<token>` key inside the script, and
`DEL` it after the reply is parsed. If the reply is lost, a second call can recover from the inflight key.
Also run the consume on `context.WithoutCancel(r.Context())` with its own timeout, so a client disconnect does
not abandon it mid-flight.

### WR-04: A sitekey/secret mismatch (or the Cloudflare test keys) makes every honest solve a "failed" outcome and kicks the joiner

**File:** `alita/utils/captchaweb/siteverify.go:203-247`, `alita/utils/captchaweb/verify.go:545-549`

**Issue:** D-28 treats `invalid-input-response` as proof that the member's token is bad. In practice it is
also what Cloudflare returns when the token was minted by a widget whose secret is not the one configured. Two
examples are a site key and secret copied from different widgets, and Cloudflare's dummy test site key used
with a real secret (or the reverse). `ConfigError()` lists only `secret`, `hostname` and `action`, so this
very common setup mistake is classified as the member's fault. Every joiner who solves the check is then
`failed` and the group's `/captchaaction` runs (kick by default). `BotCanRestrict` is checked first, so the bot
really does remove them. The first symptom is a stream of wrongful kicks right after the owner enables the
mode. I could not confirm Cloudflare's exact behaviour offline. The `invalid-input-response` behaviour for
mismatched keys is widely reported, and the classification is a decision (D-28) that was accepted without a
guard.

**Fix:** Add a cheap circuit breaker so a bad configuration cannot mass-kick:

- Keep a Redis counter `alita:captcha:cf:fail` over a rolling 10 minutes. When there are N (say 5)
  consecutive `OutcomeFailed` with reason `invalid_token` or `duplicate` and zero `OutcomePass`, treat further
  failures as `OutcomeRetry` with `ReasonSecret`. That makes `ConfigError()` log the fault.
- Alternatively, run one canary `Verify` at startup with an obviously fake token. A secret that Cloudflare
  rejects returns `invalid-input-secret`, which surfaces a bad secret. It would not detect a mismatched site
  key, so also document that mismatch under the troubleshooting list.

### WR-05: `Backend.Fail` ignores approved users and a disabled captcha, which the timer path honours

**File:** `alita/modules/captcha_web.go:370-384`, `alita/modules/captcha.go:1319-1347`

**Issue:** `expireCaptchaAttempt` (the timer and cleanup path) first checks `chat_status.IsApproved` and
`settings.Enabled`. For an approved user, or a group that disabled the captcha, it deletes the message and
releases the attempt instead of punishing. `Backend.Fail` calls `handleCaptchaTimeout` directly and skips both
checks. `AGENTS.md` says "Approved users skip ... captcha". An admin who approves a member who is mid-challenge
does not stop a failed solve from kicking them, while the same member timing out would be spared. The page
then answers `failed` with "You were removed from the group".

**Fix:** Share one decision with the timer. In `Fail`, call `expireCaptchaAttempt(b.rt.bot, attempt)` for the
approved or disabled cases. Better, add a helper `captchaSkipPunishment(bot, attempt)` and use it in both.
Return `claimed=true` with an `Action` of `""` for the released case, so the page does not claim a removal.

### WR-06: The in-flight slot is taken before the per-address window and before the unauthenticated body is read

**File:** `alita/utils/captchaweb/verify.go:346-361`, `alita/utils/httpserver/server.go:407`

**Issue:** Each request takes one of 64 `inflight` slots, then the per-address window is counted, and only then
is the body read. The server's 10 s `ReadTimeout` is the only bound on a slow upload. An unauthenticated
client that sends headers and then drips a body holds a slot for up to 10 s. Roughly 7 addresses (60 requests
a minute each) keep all 64 slots held, after which every real member's verify gets 503 `retry`. When the
per-address bucket is skipped (the documented default: a private or loopback peer with
`CAPTCHA_WEB_TRUSTED_PROXIES` empty), a single client can do it. Buffering proxies such as nginx absorb this
for the documented deployment, but the bot itself does not rely on one.

**Fix:** Acquire the slot only for the expensive part. Read and decode the body first (bounded by
`ReadTimeout` and `MaxBytesReader`, and it holds no slot), validate the initData, then take the slot before
`ConsumeIfUser` and the Telegram or Cloudflare calls. Keep the rate-window calls before the slot so abuse is
counted whether or not the slot is free.

### WR-07: `Backend.Fail` and `Expire` discard whether the failure action actually applied, so the page reports a removal that did not happen

**File:** `alita/modules/captcha_web.go:382`, `alita/modules/captcha.go:1396-1405, 1336-1347`

**Issue:** `handleCaptchaTimeout` returns `(claimed, applied)`. When the action fails (Telegram error, missing
right), it unmutes the member and returns `(true, false)`. `Fail` keeps only `claimed`, and `expireCaptchaAttempt`
discards `applied` as well. The handler then stores `Done=failed:kick` and the page says "You were removed from
the group", although the member was only unmuted and is still in the group. The replay marker repeats that
for 10 minutes. The `BotCanRestrict` pre-check makes this uncommon but not impossible (a race with an admin
change, the target is an admin, or a Telegram error).

**Fix:** Extend the `Backend` result so the handler can say it. For example have `Fail` return an
`applied bool` too. When it is false, answer `retry`-free copy that says the group's action could not be
applied, or at least store the `Done` marker with an empty action so the page shows neutral text. Log
`applied=false` at warn level (counts only).

## Info

### IN-01: The handlers' own method checks are dead code, and the documented 405 headers are not what the mux sends

**File:** `alita/utils/captchaweb/verify.go:335-339`, `alita/utils/captchaweb/page.go:143-148`, `AGENTS.md` (verify route paragraph)

**Issue:** The routes are mounted as `"POST /captcha/verify"` and `"GET /captcha"`, so Go's mux answers a wrong
method itself with a plain-text 405. That response has no `Cache-Control: no-store`, no `X-Content-Type-Options`
and no JSON body. The `if r.Method != ...` branches in both handlers run only when a test calls the handler
directly. `AGENTS.md` states that "every answer, the 405 and 429 ones included, carries `Cache-Control: no-store`
... and a JSON body", which is untrue for the mounted route. The unit tests exercise the handler directly, so
they cannot catch the gap.

**Fix:** Either mount the handlers with method-less patterns (`/captcha/verify`) so the handler's checks apply,
or delete the dead branches and correct the AGENTS.md sentence. Add a test through a real `ServeMux` and
`httptest.Server` that asserts the 405 headers.

### IN-02: `ClientIP` returns a shared private address as a bucket when it comes from `X-Forwarded-For`

**File:** `alita/utils/captchaweb/clientip.go:99-108`

**Issue:** For an untrusted `RemoteAddr`, `sharedAddress` makes the result `bucket=false`. For the entry taken
from `X-Forwarded-For` the same check is skipped (`return entry, true`). If that entry is a private, loopback
or link-local address (for example an inner proxy that was not put in the trusted list), all members share one
60 per minute bucket and real users start to get 429. The documented advice ("list your proxies") covers it,
but the result silently becomes a self-inflicted limit.

**Fix:** `return entry, !sharedAddress(entry)`.

### IN-03: `CAPTCHA_WEB_TRUSTED_PROXIES` is validated differently in `config` and in `captchaweb`, and setting it alone aborts startup

**File:** `alita/config/config.go:263, 306-327`, `alita/utils/captchaweb/clientip.go:14-37`

**Issue:** `validCaptchaWebProxies` rejects an empty entry (`"10.0.0.1,,10.0.0.2"` or a trailing comma).
`ParseTrustedProxies` documents "empty entries are skipped". Two parsers for one setting can drift, and the
`config` one cannot share code with `captchaweb` (the package is intentionally a leaf). Separately,
`validateCaptchaWeb` makes a stray `CAPTCHA_WEB_TRUSTED_PROXIES` fatal when the other four are unset, so a
deployment that turns the web captcha off by clearing the four keys but leaves the proxy list crashes at boot.

**Fix:** Pick one rule (skip empties, matching `ParseTrustedProxies`) and test both against one table of
inputs. Consider logging a warning instead of failing when the proxy list is set but the feature is off.

### IN-04: The availability memo is three independent atomics

**File:** `alita/modules/captcha_web.go:76-86, 195-212`

**Issue:** `captchaWebProbeRT`, `captchaWebProbeAt` and `captchaWebProbeOK` are loaded and stored separately.
Concurrent callers can observe a new `RT` with an old `At` or `OK`, so one call can serve the previous
runtime's answer for up to 5 s. In production there is one runtime for the process life, so the effect is
limited to tests that swap runtimes, but the memo pattern is not race-free as written. All callers also
probe in parallel when the memo expires.

**Fix:** Keep one `atomic.Pointer[probeResult]` holding `{rt, at, ok}` and replace it as a unit.

### IN-05: The page's JavaScript logic is tested only outside `make test`, and the language list is duplicated in three places

**File:** `scripts/captcha-page/captcha-page.test.mjs`, `alita/utils/captchaweb/page.go:52`, `alita/utils/captchaweb/page/captcha.html:236`

**Issue:** The 24 Node tests cover the answer-to-state mapping, `cdata`, the submit latch and the contrast
guard. `make test` and CI do not run them (`AGENTS.md`: "run by the executor"), so a regression in
`stateForAnswer` or `cdataFor` would reach production unnoticed. The `7 locales` list lives in
`PageLanguages` (Go), `LANGS` (JS) and the locale files. A test asserts the locale keys but not that `LANGS`
equals `PageLanguages`.

**Fix:** Add a CI step that runs `node --test scripts/captcha-page/*.test.mjs` when Node is available, or
embed a Go test that execs it and skips with the standard gate. Add a Go test that parses the `LANGS` array out
of `captcha.html` and compares it with `PageLanguages`.

### IN-06: Stale initData and the CSP are unverified edge cases that the deferred spike has to cover

**File:** `alita/utils/captchaweb/initdata.go:17, 93`, `alita/utils/captchaweb/page.go:127-137`, `alita/utils/captchaweb/page/captcha.html`

**Issue:**

- `InitDataMaxAge` is 5 minutes from `auth_date`, which is the launch time of the Mini App. A member who
  leaves the page open for longer, then solves, gets `invalid` ("belongs to another member or was already
  used"). That text is wrong for this case, and the widget's `Try again` cannot refresh `initData`. The
  challenge is not consumed at that point, so reopening the link recovers, but nothing tells the member to.
- `connect-src` is `'self'` only, and `style-src` allows nonce'd styles only. Cloudflare's own CSP guidance
  names `challenges.cloudflare.com` for the script and frame sources, and the exact list for a given widget
  mode cannot be confirmed offline. D-18 deferred the real-device spike, so the CSP has not been exercised
  against a live widget.

**Fix:** Map a stale `auth_date` to a distinct answer (or reuse `retry` with copy that says to reopen the
link), and make sure the spike checklist (`08-SPIKE.md`) includes "keep the page open for more than 5 minutes"
and "watch the console for CSP violations on a managed widget".

---

_Reviewed: 2026-10-30T02:35:00Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: deep_
