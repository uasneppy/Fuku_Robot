---
phase: 08-turnstile-web-captcha
verified: 2026-10-30T03:10:00Z
status: human_needed
score: 5/5 roadmap success criteria verified by code and automated tests (live-client behaviour deferred by the owner)
behavior_unverified: 0
overrides_applied: 0
gaps: []
human_verification:
  - test: "Direct Link Mini App delivers the tapper's initData on Android, iOS, Desktop and Web (08-SPIKE.md checks 1-3)"
    expected: "Tapping the group button opens the Mini App for the tapper, start_param/tgWebAppStartParam carries the 22-char token, and a second member's tap shows 'This link is not valid' without touching the target's challenge"
    why_human: "Needs a real bot, a public HTTPS host, BotFather and real Telegram clients; the suite fakes Telegram"
  - test: "Turnstile widget renders and returns a token inside each Telegram WebView, and the member is unmuted (08-SPIKE.md check 4)"
    expected: "Widget appears within seconds, solving shows Verifying then Passed, sheet closes, bot unmutes in the group"
    why_human: "Cloudflare lists embedded browsers as limited support; only real clients show it"
  - test: "Real siteverify echoes the owner's hostname (08-SPIKE.md check 5)"
    expected: "A real pass is accepted with Expect.Hostname = host of CAPTCHA_WEB_BASE_URL; no 'siteverify configuration fault: hostname'"
    why_human: "Needs the owner's Cloudflare keys and domain; tests use an httptest double"
  - test: "CSP against a live managed widget, and the frame-ancestors host list for Telegram Web (08-SPIKE.md check 6)"
    expected: "No CSP violations in the WebView console; the sheet is not blank in Telegram Web K/A/Z"
    why_human: "CSPFor was written from documentation; a live widget may need another source"
  - test: "BotFather accepts the owner's short name and Web App URL (guide step 2)"
    expected: "/newapp succeeds; https://t.me/<bot>/<short>?startapp=<token> opens the app"
    why_human: "Needs the owner's BotFather chat"
  - test: "Stale initData (older than 5 minutes) copy and re-open path"
    expected: "A member who waited more than 5 minutes sees 'This link is not valid' and can tap the group button again and pass"
    why_human: "Telegram client behaviour on re-opened sheets"
  - test: "Native-speaker read of the 26 page strings and the welcome/button/refusal texts in es, fr, hi, id, pt, ru"
    expected: "Natural wording within the UI-SPEC lengths"
    why_human: "Translation quality"
  - test: "TestCaptchaModeTurnstileConstraint on PostgreSQL (make test-postgres-integrity in CI)"
    expected: "chk_captcha_mode accepts turnstile, refuses bogus, one captcha_mode check remains"
    why_human: "PostgreSQL must not be started locally; the test skips here (SKIP observed) and is in the Makefile -run list"
---

# Phase 8: Turnstile Web Captcha Verification Report

**Phase Goal:** As a group admin, I want to verify new members with Turnstile in a Telegram Mini App, so that bots stay out with little friction.
**Verified:** 2026-10-30
**Status:** human_needed (every automated truth holds; the deferred live-device checks are listed above and are not gaps)
**Re-verification:** No, initial verification

## Goal Achievement

### Observable Truths (ROADMAP success criteria)

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | Admin can set Turnstile mode, math/text remain; a Turnstile join mutes the member and posts a challenge whose button opens the Mini App in the member's language | VERIFIED | `alita/modules/captcha.go:603` accepts `math\|text\|turnstile`; `:612-619` `captchaWebModeRefusal` (`:658`) gates it (runtime, live PING, live bot Restrict right). `SendCaptcha` (`:1074-1100`) picks Turnstile or math fallback, creates `ts1:` attempt, `RestrictMember` (`:1198`), `issueChallenge` and one URL button `MiniAppLink` (`:1186-1196`). Migration `migrations/20261030120000_allow_turnstile_captcha_mode.sql` widens `chk_captcha_mode`. Language: page tables for 7 languages (`captchaweb/page.go` `PageLanguages`), 31 `captcha_web_*` lines in each of the 7 locale files. Tests PASS: `TestCaptchaWebTracerJoinVerifyUnmute`, `TestCaptchaModeTurnstile`, `TestCaptchaModeTurnstileRefusals`, `TestCaptchaModeListsTurnstileInEveryLanguage`, `TestSetCaptchaModeAcceptsTurnstile`, `TestCaptchaSettingsConstraint_ValidModes`, `TestCaptchaWebLocaleKeys`, `TestStringTable`, `TestPageStructure`, node page tests (24/24) |
| 2 | A pass unmutes, replays held messages, runs the greeting and cleans the challenge, as the existing captcha | VERIFIED | `captchaWebBackend.Pass` (`captcha_web.go:288-330`) calls the same `claimCaptchaPass` (`captcha.go:1559`, held messages read, `CompleteCaptchaAttemptAtomic`, unmute) and `finishCaptchaPass` (`:1583`) that the `captcha_verify` callback uses (`:1736/:1752`); finish runs on the captcha lifecycle (`startCaptchaLifecycleTask`). Tests PASS: `TestCaptchaPassSharedByCallbackAndWeb`, `TestCaptchaWebTracerJoinVerifyUnmute`, `TestCaptchaWebUnmuteFailureSchedulesRetry`, `TestCaptchaWebFinishJoinsLifecycleDrain` |
| 3 | Only the issued member can pass; another user's tap, a reused challenge, or a page-reported pass never unmutes; the server decides from signed Telegram data plus Cloudflare | VERIFIED | `Handler.Verify` (`captchaweb/verify.go:356-455`): `ValidateInitData` HMAC with bot token, constant-time, 5 min max age, no bot users (`initdata.go:51-115`); signed `start_param` must equal the posted challenge (`verify.go:399`); `Store.ConsumeIfUser` Lua compare-and-claim on signed user id (`challenge.go:246-303`) gives `not_for_you` without change; `Done` marker replays only to the same user (`verify.go:461-478`); `Client.Verify` requires Cloudflare success plus hostname, action `captcha_join`, and cdata `sha256("cd1:"+token)` (`siteverify.go:185-200`). The page only relays `init_data`, `turnstile_token`, `challenge` (`captcha.html:636`) and renders the server's answer. The `captcha_verify` and `captcha_refresh` callbacks refuse any `ts1:` attempt (`captcha.go:1705`, `:1898`). `liveTurnstileAttempt` matches attempt id, user, chat, `ts1:` and expiry. Tests PASS: `TestInitDataKnownAnswer`, `TestInitDataRejects`, `TestVerifyRouteWrongUserChangesNothing`, `TestChallengeStoreConsumeOnce` (50 parallel -> 1 winner), `TestVerifyRouteReplaysDecision`, `TestCaptchaWebAttemptRefusesCallbacks`, `TestCaptchaWebRejoinInvalidatesOldChallenge`, `TestSiteverifyOutcome`, `TestSiteverifyRequestShape` |
| 4 | A failed check or a timeout kicks the member (can rejoin) on any replica | VERIFIED | Provable failure only (`invalid-input-response`, `timeout-or-duplicate`, cdata mismatch) -> `Backend.Fail` -> `settleCaptchaAttempt`, the same decision the timer makes (`captcha_web.go:376-406`); a late check -> `Backend.Expire` -> `expireCaptchaAttemptApplied`; Cloudflare/network/config faults are `retry` and re-arm (`verify.go:632-639`). Timeout is DB-driven (`scheduleCaptchaTimeout`, cleanup tick `GetExpiredCaptchaAttempts`, restart recovery), the challenge is in Redis, so no replica affinity. The conditional delete of the attempt row decides exactly once. Tests PASS: `TestCaptchaWebFailedSolveAppliesAction`, `TestCaptchaWebCloudflareErrorIsNotAFailure`, `TestCaptchaWebPassAfterExpiryExpiresAtOnce`, `TestCaptchaWebTimeoutAndVerifyOnDifferentReplicas` (second replica, `-race`), `TestCaptchaWebPassVersusTimeoutRace`, `TestCaptchaWebBotWithoutRightsIsRetry`, `TestRunOrphanedCaptchaRecoveryResumesTurnstileAttempt`, `TestHandleCaptchaTimeout*` |
| 5 | Owner can deploy by documented steps; the Turnstile secret never appears in logs | VERIFIED (doc accuracy); live deploy deferred | `docs/src/content/docs/self-hosting/captcha-web.md` covers HTTPS host + proxy (Caddy/nginx forwarding only `/captcha`, `/captcha/*`), BotFather `/newapp` + `/myapps`, Turnstile widget (Managed, hostname list), the four variables + `CAPTCHA_WEB_TRUSTED_PROXIES`, health check, troubleshooting. Secret: `config.go:603-612` `registerSecrets` includes `cfg.TurnstileSecretKey`; `validateCaptchaWeb` rejects a secret shorter than the 6-char redaction minimum (`config.go:276`, `logredact.minSecretLen = 6`); the secret is used only as `Client.Secret` in the siteverify body (`main.go:461`, `siteverify.go:154`); no log call in `captchaweb` or `captcha_web.go` prints a token, initData, name or Cloudflare body (logs carry chat/user/attempt ids and reason names). Tests PASS: `TestCaptchaWebConfig` (includes `logredact.Scrub` of the secret), `TestCaptchaWebNeverLogsInputs`, `TestCaptchaWebStartupWarnings` |

**Score:** 5/5 truths verified in code and tests; 0 behavior-unverified in the automated sense. The live-client facts are routed to human verification per the owner's deferral.

### Requirement Coverage (REQUIREMENTS.md CAPT-01..CAPT-08, PLAT-04)

| Requirement | REQUIREMENTS.md text (short) | Status | Evidence |
|---|---|---|---|
| CAPT-01 | Admin sets Turnstile; math/text remain | SATISFIED | `captcha.go:603-619`, migration `20261030120000`, `captcha.SetCaptchaMode`; `TestCaptchaModeTurnstile`, `TestCaptchaModeTurnstileRefusals`, `TestSetCaptchaModeAcceptsTurnstile`, `TestCaptchaModeMigrationStatements`; PG test `TestCaptchaModeTurnstileConstraint` is in the Makefile `test-postgres-integrity` `-run` list (SKIP locally, runs in CI) |
| CAPT-02 | Muted on join, challenge message with Mini App button | SATISFIED | `captcha.go:1198`, `:1186-1196`; `captchaweb.MiniAppLink`; `TestCaptchaWebTracerJoinVerifyUnmute`, `TestSendCaptchaTurnstileFallsBackToMath`, `TestSendCaptchaTurnstileRollsBackOnFailure` |
| CAPT-03 | Only the issued member; no reuse | SATISFIED | signed-user compare in `ConsumeIfUser`, one-time claim, done marker; `TestVerifyRouteWrongUserChangesNothing`, `TestChallengeStoreConsumeOnce`, `TestCaptchaWebRejoinInvalidatesOldChallenge`, `TestCaptchaWebAttemptRefusesCallbacks` |
| CAPT-04 | Decided on the server by signed initData + Cloudflare; page never reports a pass | SATISFIED | `initdata.go`, `siteverify.go`, `verify.go`; page posts only relayed values (`captcha.html:636`); `TestInitDataKnownAnswer`, `TestInitDataRejects`, `TestSiteverifyOutcome`, `TestVerifyRouteRearmsOnlyOnRetry`, node page tests |
| CAPT-05 | Unmute, replay, greeting, cleanup as existing captcha | SATISFIED | shared `claimCaptchaPass` / `finishCaptchaPass`; `TestCaptchaPassSharedByCallbackAndWeb` |
| CAPT-06 | A failed check counts as failing; kick, can rejoin | SATISFIED | `Backend.Fail` via `settleCaptchaAttempt`; one solve per member (Redis claim + attempt row). The group's `/captchaaction` is applied (kick by default, ban or mute if the admin chose them), same as the timer. `TestCaptchaWebFailedSolveAppliesAction` |
| CAPT-07 | Timeout kicks, can rejoin | SATISFIED | `scheduleCaptchaTimeout`, `Backend.Expire`, cleanup tick, recovery; `TestCaptchaWebTimeoutAndVerifyOnDifferentReplicas`, `TestCaptchaWebPassAfterExpiryExpiresAtOnce`, `TestRunOrphanedCaptchaRecoveryResumesTurnstileAttempt` |
| CAPT-08 | Page in the user's language, 7 languages | SATISFIED (wording quality is human) | `PageLanguages` = en es fr hi id pt ru; 26 `PageKeys`; `NewPage` fails if an en key is missing; 31 `captcha_web_*` keys in each locale; `TestCaptchaWebLocaleKeys`, `TestStringTable`, `make check-translations` clean; node tests pin the language mapping (id, unknown -> en) |
| PLAT-04 | Deployment steps documented | SATISFIED | `captcha-web.md` steps 1-6 and troubleshooting; `sample.env:145-166`; `api-reference/environment.md:110-161`; `08-SPIKE.md` checklist; `make check-docs` reports no drift |

No orphaned requirements: REQUIREMENTS.md maps CAPT-01..08 and PLAT-04 to Phase 8 and all are claimed by the plans. Note: the REQUIREMENTS.md checkboxes and traceability table still read "Pending" and VALIDATION.md frontmatter is still `status: draft`, `nyquist_compliant: false`; these are bookkeeping the orchestrator should update on phase close, not code gaps.

### Key Links

| From | To | Via | Status |
|---|---|---|---|
| `main.go` `registerCaptchaWeb` (`:439-476`, called `:241`) | `modules.ConfigureCaptchaWeb` + `Server.Handle` for `/captcha`, `/captcha/verify`, `/captcha/health` | `CaptchaWebRoutes()` | WIRED (`httpserver/server.go` `Handle`, `endpointList`); `TestCaptchaWebRoutesRegistration`, `TestCaptchaWebWiring` |
| `postInit` (`main.go:524`) | `MarkCaptchaWebReady` | readiness gate, verify and health answer 503 until then (`verify.go:369`, `:238`) | WIRED; `TestVerifyRouteReadinessGate`, `TestHealthRoute` |
| `SendCaptcha` | `captchaWebRuntime.issueChallenge` -> `Store.Issue` | `captcha.go:1188` | WIRED |
| `Handler.Verify` | `captchaWebBackend` (Attempt, Pass, Fail, Expire, BotCanRestrict) | `Options.Backend` (`captcha_web.go:136`) | WIRED |
| `captchaWebBackend.Pass` | `claimCaptchaPass` + `finishCaptchaPass` | `captcha_web.go:312-324` | WIRED |
| `config.registerSecrets` | `logredact.RegisterSecret(TurnstileSecretKey)` | `config.go:611` | WIRED |
| `getStats` | `captchaWebStatsSection` | `TestCaptchaWebStatsSection` | WIRED |
| `ts1:` guard in every reader of `captcha_attempts.answer` that compares or reissues it | `IsReservedAnswer` | callbacks `captcha.go:1705` (verify) and `:1898` (refresh); web side `captcha_web.go:265`, `:363`; counter `captcha.go:1329`. Remaining `attempt.Answer` uses (`:1570`, `:1764`, `:1983`, `db/captcha/repository.go`) only pass the stored value into compare-and-set claims | WIRED, complete |

### AGENTS.md vs code (spot checks)

| AGENTS.md claim | Code | Result |
|---|---|---|
| `ts1:` + 32 hex answer | `NewReservedAnswer` 16 bytes hex (`challenge.go:74`) | match |
| Challenge key `alita:captcha:ch:<token>`, 22-char token, grace 5 min, fields u/a/c/x | `challenge.go:45,64,39,138-143` | match |
| Claim nonce `k`, delete only with the nonce, release on error, 750 ms deadline | `consumeScript`/`finishScript`/`releaseScript`, `verify.go:424` (3 x 250 ms) | match |
| Rate windows 60 s; 60 / 20 / 8; IPv6 by /64; Redis error -> 503 | `limiter.go:13-21`, `verify.go:286-317` | match |
| Streak key `alita:captcha:streak`, limit 5, TTL 10 min, cleared on pass, fails open | `breaker.go`, `verify.go:624,661-676` | match |
| Done marker `alita:captcha:done:<token>`, `SET NX`, 10 min, replay only to same user | `challenge.go:49-51,166-216`, `verify.go:461-478` | match |
| In-flight cap 64 taken after body and initData checks | `verify.go:38,413-419` | match |
| Routes mounted without a method; handlers answer 405 with Allow/no-store/nosniff | `verify.go:46-50,361-365`, `page.go ServeHTTP` | match |
| Routes absent when settings unset or Redis missing | `CaptchaWebRoutes()` nil without runtime; `main.go:446-451` | match |
| Restrict-right gate, order not-configured, no-redis, no-rights | `captchaWebModeRefusal` (`captcha.go:658-682`) | match |

### Documentation vs code (captcha-web.md)

Routes (`/captcha` GET, `/captcha/verify` POST, `/captcha/health`) match `verify.go:46-50`. The five env names match `config.go:463-467`, `sample.env` and `environment.md`. The "some but not all stops startup" and "trusted proxies alone stops startup" claims match `validateCaptchaWeb` (`config.go:243-282`). The three refusal texts match `locales/en.yml:1040-1042`. "Open this in Telegram" and "This link is not valid" match `captcha_web_unsupported_title` and `captcha_web_invalid_title`. The 12 s page timeout matches `VERIFY_TIMEOUT_MS = 12000` (`captcha.html:242`). The 5-minute initData age matches `InitDataMaxAge`. The streak description (5 enforced, hold from the sixth, logged once a minute) matches `verify.go:661-689`. The `frame-ancestors` host list matches `CSPFor`. The no-Redis behaviour (`/captcha/health` 404, math fallback, command refused) matches `main.go` and `captchaWebModeRefusal`. No mismatch found.

### Behavioural Spot-Checks (run by this verifier)

| Check | Command | Result |
|---|---|---|
| Focused Go packages | `CGO_ENABLED=1 go test -tags testtools -race -count=1 ./alita/utils/captchaweb ./alita/config ./alita/i18n ./alita/utils/httpserver ./alita/db/captcha ./alita/db/migrations` | all `ok` |
| Module captcha tests | `... -run 'Captcha' ./alita/modules` | `ok` (18.8 s) |
| Page script | `node --test scripts/captcha-page/*.test.mjs` | 24 pass, 0 fail |
| VALIDATION.md test names (21 module tests, captchaweb, i18n, httpserver, migrations) re-run with `-v` | explicit anchored `-run` | every one `--- PASS`; only `TestCaptchaModeTurnstileConstraint` is `SKIP` (PostgreSQL only, in the CI `-run` list in the Makefile) |
| `-run` patterns match something | grep of `^func <Name>` for all 49 named tests | every name exists once; the two prefix patterns (`TestCaptchaVerifyCallback`, `TestHandleCaptchaTimeout`) match 5 and 2 existing tests |
| `TestCaptchaSettingsConstraint_ValidModes` (`./alita/db`), `TestSetCaptchaModeAcceptsTurnstile` | `-v` | PASS |
| `make check-translations` | | all 7 locales complete |
| `make check-docs` | | no drift |
| `CGO_ENABLED=0 go build ./...`, `go vet -tags testtools` on captchaweb/modules/config, `gofmt -l` | | clean |
| CI runs the page tests | `.github/workflows/ci.yml:344` | present |

### Anti-Patterns

| Scan | Result |
|---|---|
| `TBD`/`FIXME`/`XXX` in lines added since c3cd951 | none |
| `TODO`/`HACK`/`PLACEHOLDER`/"not yet implemented" in added non-test Go | none |
| Fire-and-forget goroutines in `captcha_web.go` / `captchaweb` | none added (the finish uses the lifecycle task with `RecoverFromPanic`) |
| Stubs / hollow data flow | none: handlers query Redis and the DB, Cloudflare is a real HTTP client, the page posts to the real route |
| Secret in logs | no log call includes secret, token, initData, name or Cloudflare body; redaction registered |
| 08-REVIEW findings | 13 findings (WR-01..07, IN-01..06), 13 fix commits after 39059ab (1437c9b ... ffeb82d) present in history; the behaviours they describe (lost-claim re-read, done marker for failed unmute, claim-then-delete, streak breaker, in-flight slot order, method-less mounts, /64 bucketing, shared approved/captcha-off decision, `applied` flag) are all visible in the code read above |

### Notes (non-blocking)

1. REQUIREMENTS.md and VALIDATION.md bookkeeping remain "Pending"/"draft" (see above).
2. CAPT-06's "kicked" is the group's `/captchaaction` (default kick); an admin who chose `mute` gets the mute, identical to the timer and to the existing captcha. This follows the existing setting and was the intended design (docs say so).
3. PostgreSQL constraint behaviour is proven only on CI; the migration order (drop both checks, then re-add `NOT VALID`) is covered locally by `TestCaptchaModeMigrationStatements`.

## Gaps Summary

No gaps. All five roadmap success criteria and all nine requirements are implemented, wired and covered by passing tests. Remaining uncertainty is limited to behaviour that needs real Telegram clients, a real Cloudflare widget and the owner's BotFather, which the owner deferred ("plan and build now, spike later") and which are listed under `human_verification` and in `08-SPIKE.md`.

---

_Verified: 2026-10-30_
_Verifier: Claude (gsd-verifier)_
