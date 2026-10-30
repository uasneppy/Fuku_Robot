---
phase: 08-turnstile-web-captcha
fixed_at: 2026-10-30T03:00:00Z
review_path: .planning/phases/08-turnstile-web-captcha/08-REVIEW.md
iteration: 1
findings_in_scope: 13
fixed: 13
skipped: 0
status: all_fixed
---

# Phase 8: Code Review Fix Report

**Fixed at:** 2026-10-30
**Source review:** .planning/phases/08-turnstile-web-captcha/08-REVIEW.md
**Iteration:** 1

**Summary:**
- Findings in scope: 13 (0 critical, 7 warnings, 6 info)
- Fixed: 13 (IN-06 is documentation only, as decided)
- Skipped: 0

Work was done directly in the working tree on `claude/cool-hamilton-hvs9nl` (no worktree, as instructed), one commit per finding, nothing pushed. The commit order differs from the review order because some fixes build on others (WR-07, WR-05, WR-01, WR-02, WR-04, WR-03, WR-06, then the Info items). Every rule change updated the affected Turnstile bullets of `AGENTS.md` in the same commit.

Verification, run here: `gofmt -l` clean; `go vet -tags testtools` clean for `captchaweb`, `config`, `i18n`, `httpserver`, `modules`; `CGO_ENABLED=1 go test -tags testtools -race -count=1` passes for `./alita/utils/captchaweb`, `./alita/config`, `./alita/i18n`, `./alita/utils/httpserver` and `./alita/modules -run Captcha`; `node --test scripts/captcha-page/*.test.mjs` passes (24); `make check-translations`, `make check-docs` and `CGO_ENABLED=0 go build ./...` pass. `make lint` and the whole `make test` were not run (golangci-lint is built with go1.25; the orchestrator runs `make test`). PostgreSQL was never started; the SQLite harness and miniredis were used.

## Fixed Issues

### WR-07: `Backend.Fail` and `Expire` discard whether the failure action applied

**Commit:** 1437c9b
**Files modified:** `alita/utils/captchaweb/verify.go`, `alita/utils/captchaweb/challenge.go`, `alita/modules/captcha_web.go`, `alita/modules/captcha.go`, `AGENTS.md`, tests
**What changed:** `Backend.Fail` and `Backend.Expire` now return a `captchaweb.ActionResult` (`ActionLost`, `ActionApplied`, `ActionNotApplied`). `expireCaptchaAttemptApplied` carries `applied` out of the timer path (`expireCaptchaAttempt` keeps its signature). For `ActionNotApplied` the handler answers the neutral `invalid` with no action, `MarkDone` and `Done` accept `invalid` so the replay marker stores the same, and the handler logs `outcome=<result> applied=false` at Warn and counts an error.
**Test:** `TestVerifyRouteActionNotApplied` (captchaweb: failed solve and expiry paths, replay, warn log) and `TestCaptchaWebFailedActionNotAppliedIsNeutral` (modules, real harness: the ban call is refused, the page gets `invalid`, the member is unmuted, no removal notice, replay is `invalid`).
**Deviation:** the neutral answer is `invalid` ("This link is not valid ... Nothing was changed"). The existing page states are `passed`, `failed`, `expired`, `invalid`, `not_for_you`, `retry`; `failed` and `expired` default to a "removed" body for an empty action, `retry` would promise a second try the consumed challenge cannot give, and `passed` would claim a verification that did not happen. No locale key was added.

### WR-05: `Backend.Fail` ignores approved users and a disabled captcha

**Commit:** ac7bd7e
**Files modified:** `alita/modules/captcha.go`, `alita/modules/captcha_web.go`, `AGENTS.md`, `alita/modules/captcha_web_outcomes_test.go`
**What changed:** one shared decision, `settleCaptchaAttempt`, used by the timer, the cleanup tick, the late check (`expireCaptchaAttemptApplied`) and `Backend.Fail`: an approved member, or a group that turned the captcha off, is released and unmuted (message deleted, row released); everyone else gets `handleCaptchaTimeout`. The released case reports `Claimed` without `Applied`, so the page gets the neutral `invalid` and no removal is reported. `CountExpired` still counts only the `handleCaptchaTimeout` branch, as before.
**Test:** `TestCaptchaWebFailedSolveSparesApprovedAndDisabled` (modules, both cases: no kick or ban, member unmuted, challenge message deleted, no removal notice, row gone).

### WR-01: a lost claim answered "expired" while the attempt is still live

**Commit:** 66fe0a0
**Files modified:** `alita/utils/captchaweb/verify.go`, `alita/modules/captcha_web.go`, `AGENTS.md`, tests
**What changed:** `challengeRun.lostClaim` (used after `PassLost` and an unclaimed `Fail`) reads the attempt again with `Backend.Attempt`: still live gives `retry` with a re-arm, an unreadable attempt gives `retry` (503) with a re-arm, only a row confirmed gone or expired runs `expire()`. `Backend.Fail` re-reads the row after an unclaimed `settleCaptchaAttempt` and returns an error when it is still pending, so a database blip inside `handleCaptchaTimeout` re-arms instead of burning the challenge.
**Test:** `TestCaptchaWebLostClaimOfLiveAttemptIsRetry` (the message_id race: the ID is cleared, then written back by a hook on the `getChat` call that sits between the pass's read and its claim) and `TestCaptchaWebFailedSolveDatabaseBlipIsRetry` (a real GORM query callback fails the third read of `captcha_attempts`; 503 retry, row intact, nobody kicked, then the same challenge decides the failed solve once). Both were run against the old code and fail there (expired answers). The captchaweb table gained live, gone and unreadable cases for both paths.

### WR-02: a pass whose unmute failed ends on a dead "invalid" screen

**Commit:** 0b79c76
**Files modified:** `alita/utils/captchaweb/verify.go`, `AGENTS.md`, tests
**What changed:** the `PassUnmuteFailed` branch writes `Done=passed` before answering, so a repeat replays `passed` with no side effect and no Cloudflare call.
**Test:** `TestVerifyRoutePassWhoseUnmuteFailedIsRemembered` (captchaweb) and the extended `TestCaptchaWebUnmuteFailureSchedulesRetry` (modules: the second verify is `passed`, no Telegram call, one Cloudflare request).
**Deviation / choice recorded:** the first reply stays `retry`. The member is not unmuted until the retry worker has run, so "passed" would say something not yet true; the marker makes the second tap (the page's Try again) end on `passed`.

### WR-04: a site key / secret mismatch (or the Cloudflare test keys) mass-kicks joiners

**Commit:** e9558e6
**Files modified:** `alita/utils/captchaweb/breaker.go` (new), `alita/utils/captchaweb/verify.go`, `AGENTS.md`, `docs/src/content/docs/self-hosting/captcha-web.md`, `.planning/phases/08-turnstile-web-captcha/08-SPIKE.md`, tests
**What changed:** Redis counter `alita:captcha:streak` (`Store.RecordFailedSolve`: one Lua script, `INCR` plus `PEXPIRE` 10 minutes renewed on every failed solve; `Store.ClearFailedSolves`: `DEL` on every confirmed pass). The first `StreakLimit` (5) failed solves are enforced as D-28 says; from the sixth on, `suspectKeyMismatch` holds the `OutcomeFailed` verdict as `retry` (member stays muted, attempt and challenge intact, no marker, nobody removed; the attempt's own timeout still applies the group's action). The hold is logged at Error as `[CaptchaWeb] suspected key mismatch ...` by reason name only, at most once a minute per replica, and counted as an error. A Redis error fails open (the failure is enforced, a pass still goes through). Documented in `AGENTS.md`, in the deployment guide troubleshooting, and in the spike (check 5: confirm the pair belongs to one widget and is not a test key).
**Test:** `TestVerifyRouteKeyMismatchBreaker` (a single bad token never trips it; streak counting and the downgrade at the sixth with one Error log for several held solves; the notice again after a minute; a pass resets the streak; a quiet streak expires; Redis down fails open, via a go-redis hook) and `TestCaptchaWebKeyMismatchCannotMassKick` (modules, end to end: 5 kicked, later members stay muted with attempts intact, the owner fixes the keys, a member passes and the streak key is gone).
**Deviation:** all `OutcomeFailed` verdicts count (invalid token, duplicate and cdata mismatch), not only the first two reasons, because the breaker only needs "a failed solve with no pass in between".

### WR-03: a Redis error or parse failure after the Lua delete loses the challenge

**Commit:** 0dc344e
**Files modified:** `alita/utils/captchaweb/challenge.go`, `alita/utils/captchaweb/verify.go`, `AGENTS.md`, `alita/utils/captchaweb/verify_limits_test.go`
**What changed:** smallest correct design, a claim instead of an early delete. `consumeScript` adds a `k` field holding a random nonce and does not delete (a claimed key is "unknown" to every other caller, which keeps 50 parallel consumes to one). Go reads and parses the reply, then `finishScript` deletes the key only while it still carries the nonce. Any error after the script ran (a reply lost because the context ended or the connection dropped, a record that does not parse) runs `releaseScript` with the same nonce, so a non-nil error from `ConsumeIfUser` always means nothing was used up. `rearmScript` treats a key that still carries a claim (a delete that failed) as absent. The handler runs the claim on `context.WithoutCancel(r.Context())` with a 750 ms deadline; the release and the delete run on a context of their own.
**Test:** `TestChallengeStoreConsumeIsUndoneByAnError` (a hook that runs the script and then reports an error; an unparsable record; a left-over claim and a re-arm over it) and the existing `TestChallengeStoreConsumeOnce` (50 goroutines, exactly one `ConsumeOK`) still passes.
**Known limit (documented):** if Redis is unreachable for the release too, the claim stays until the challenge expires and the member times out like for any other Redis fault.

### WR-06: the in-flight slot is taken before the body is read

**Commit:** b0b9450
**Files modified:** `alita/utils/captchaweb/verify.go`, `AGENTS.md`, `alita/utils/captchaweb/verify_limits_test.go`
**What changed:** the slot is acquired after the address window, the body, the initData check and the two per-user windows, immediately before the claim. The rate-window calls stay before it.
**Test:** the `in flight` limits test now holds 64 slots with valid requests and asserts that, while every slot is held, an empty body is still 400 and forged initData still 200 invalid (no slot needed), and that a verified request beyond the cap is 503 retry with its challenge untouched and the backend not called. The old assertion "no Redis commands at all" was dropped because the windows are now spent first, by design.

### IN-01: dead method checks and undocumented 405 headers

**Commit:** 35bb9db
**Files modified:** `alita/utils/captchaweb/verify.go`, `alita/modules/captcha_web.go`, `alita/utils/httpserver/server.go`, tests, `AGENTS.md`
**What changed:** method-less patterns (`/captcha`, `/captcha/verify`, `/captcha/health`), so the handlers' own checks run. `Verify` answers 405 with `Allow: POST`, no-store, nosniff and the JSON body (and no longer counts the scanner's request as an `invalid` stat); `Health` gained a method check (405 with `Allow: GET, HEAD`); the page already had one (`http.Error` sets nosniff). `/captcha/` is still a 404.
**Test:** the routes subtest of `TestVerifyRouteLimitsAndHeaders` now runs a real `http.ServeMux` behind an `httptest.Server` with a real `Page`, and asserts status, `Allow`, `Cache-Control`, `X-Content-Type-Options`, the JSON content type and body and the absence of CORS headers for every wrong method on all three routes. `TestCaptchaWebRoutesRegistration` (httpserver) uses the `captchaweb.Route*` constants and asserts the mux leaves the method to the handler.

### IN-02: a shared address from `X-Forwarded-For` was returned as a bucket

**Commit:** 66248ed
**Files modified:** `alita/utils/captchaweb/clientip.go`, `alita/utils/captchaweb/clientip_test.go`, `AGENTS.md`
**What changed:** `return entry, !sharedAddress(entry)`.
**Test:** two new `TestClientIP` cases (a private and a loopback entry from the chain are no bucket).

### IN-03: two rules for empty trusted proxy entries

**Commit:** 25dd2d1
**Files modified:** `alita/config/config.go`, `alita/config/captcha_web_test.go`, `alita/utils/captchaweb/clientip_test.go`, `AGENTS.md`
**What changed:** `validCaptchaWebProxies` now skips empty entries, matching `ParseTrustedProxies`. "A proxy list alone is a startup error" is unchanged.
**Test:** the same table of 16 inputs is held in both packages: `TestValidCaptchaWebProxiesTable` and `TestParseTrustedProxiesTable` assert the same validity, and the latter the same prefix count.

### IN-04: the availability memo was three atomics

**Commit:** 09443cf
**Files modified:** `alita/modules/captcha_web.go`, `alita/modules/captcha_web_test.go`, `alita/modules/captcha_web_wiring_test.go`, `AGENTS.md`
**What changed:** one `atomic.Pointer[captchaWebProbeResult]` holding `{rt, at, ok}`, replaced as a unit; tests reset it with `captchaWebProbe.Store(nil)`.
**Test:** `TestCaptchaWebAvailabilityMemoIsOneUnit` (runtimes with a live and a dead Redis swapped under concurrent callers, `-race`; no caller gets the other runtime's answer).

### IN-05: the page script tests are outside CI, and the language lists could drift

**Commit:** 1af57ae
**Files modified:** `.github/workflows/ci.yml`, `alita/utils/captchaweb/page_test.go`, `AGENTS.md`
**What changed:** the `test` job of `ci.yml` now sets up Node 22 (`actions/setup-node@v7`, as `docs.yml` does) and runs `node --test scripts/captcha-page/*.test.mjs` before `make test`; `AGENTS.md` records it next to the page-test sentence.
**Test:** `TestPageLanguagesMatchScript` parses the `LANGS` array out of the embedded `captcha.html` and compares it with `PageLanguages`. The Node suite passes locally (24 tests).

### IN-06: stale initData and the CSP are unverified

**Commit:** ffeb82d
**Files modified:** `.planning/phases/08-turnstile-web-captcha/08-SPIKE.md`
**What changed:** no code change, as decided. The spike gained check 14 (keep the page open for more than 5 minutes, then solve, and record the answer and the copy, whether Try again appears and whether reopening the button recovers) and check 6 now asks for the console to be watched for CSP violations against a Managed widget on every inspectable client.

## Open

- **Stale initData copy (IN-06):** a solve that arrives more than 5 minutes after the Mini App was opened is answered `invalid`, and the page shows "This link is not valid ... It belongs to another member or was already used". That copy is wrong for this case and the widget's Try again cannot refresh `initData`; the challenge is not consumed by the stale answer, so reopening the button from the group recovers. No code change was made. Check 14 of the spike records the real behaviour; a distinct answer or copy is a product decision for after that run.
- **Live-only items** stay with the owner's spike (`08-SPIKE.md`): the CSP against a real managed widget (check 6) and stale signed data (check 14).

## Skipped Issues

None.

---

_Fixed: 2026-10-30_
_Fixer: Claude (gsd-code-fixer)_
_Iteration: 1_
