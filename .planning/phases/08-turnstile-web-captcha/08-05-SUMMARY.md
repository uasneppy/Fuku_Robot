---
phase: 08-turnstile-web-captcha
plan: 05
subsystem: captcha-web
tags: [captcha, turnstile, mini-app, csp, html, i18n, node-test]

requires:
  - phase: 08-01
    provides: "TurnstileAction, CDataPrefix, CData, ValidToken and the verify handler's Answer shape (passed, failed, expired, invalid, not_for_you, retry plus action)"
provides:
  - "captchaweb.Page, NewPage(PageOptions{SiteKey, Strings}), Page.ServeHTTP, CSPFor, PageKeys (26), PageLanguages (7)"
  - "alita/utils/captchaweb/page/captcha.html: the embedded Mini App page with the 9-state machine and window.CaptchaPage"
  - "scripts/captcha-page/captcha-page.test.mjs: node --test coverage of the page script (24 tests)"
affects: [08-07 builds the string tables from the locale files, 08-10 mounts Page at GET /captcha and records the page rules in AGENTS.md, 08-11 spike checklist]

actuals:
  tokens: 17000
  tasks: 3
  commits: 5
plan_head_before: f9560b78b4854f75cca428dbf59882850f236056
plan_head_after: 803a246fe9b290d7e360eea7fb71a7b68118ba30

tech-stack:
  added: []
  patterns:
    - "Per-response CSP nonce on every script and style element; CSPFor builds the one header value the test compares against"
    - "Strings travel in one json.Marshal block passed as template.JS; the main script holds no template action and reads server values from data-* attributes and the JSON block"
    - "Page script tested verbatim in a node:vm context with stub DOM, Telegram, Turnstile, fetch and fake timers"

key-files:
  created:
    - alita/utils/captchaweb/page.go
    - alita/utils/captchaweb/page/captcha.html
    - alita/utils/captchaweb/page_test.go
    - scripts/captcha-page/captcha-page.test.mjs
  modified: []

key-decisions:
  - "Language resolution takes the first non-empty code (Telegram's, else navigator's) and maps an unsupported one to en, as the UI contract words it, rather than falling through to the navigator language"
  - "stateForAnswer takes {status, body} or null; passed, failed and expired need HTTP 200, invalid and not_for_you need a non-429 status under 500, everything else is retry"
  - "The icon is chosen by a data-icon attribute on one container and CSS, not by hidden attributes on SVG elements"
  - "guardTheme (extra export) returns only the colours that change; a pair is judged again after a replacement, so it ends in at most three rounds"
  - "NewPage embeds only the 7 known languages and the 26 known keys, and requires every en value to be non-empty"
  - "noscript caption (UI-SPEC gives no text): 'This check needs JavaScript. Turn it on, then open the link in Telegram again.'"

requirements-completed: [CAPT-04, CAPT-08]

coverage:
  - id: D1
    description: "GET returns the embedded page with a fresh 16-byte nonce on every script and style, the exact CSP, no-store, no-referrer, nosniff, and no frame or CORS header; HEAD matches without a body; other methods get 405"
    requirement: CAPT-08
    verification:
      - kind: unit
        ref: "alita/utils/captchaweb/page_test.go#TestPageHeadersAndNonce"
        status: pass
    human_judgment: false
  - id: D2
    description: "The 7 string tables and the site key cannot break out of their script block or attribute"
    requirement: CAPT-08
    verification:
      - kind: unit
        ref: "alita/utils/captchaweb/page_test.go#TestPageEmbedsSafeStrings"
        status: pass
    human_judgment: false
  - id: D3
    description: "The markup follows the UI contract: data attributes, one status region, one widget group, one button, noscript, eight icons, exactly two external scripts, no inline style or handler, viewport and robots metas"
    requirement: CAPT-08
    verification:
      - kind: unit
        ref: "alita/utils/captchaweb/page_test.go#TestPageStructure"
        status: pass
      - kind: unit
        ref: "alita/utils/captchaweb/page_test.go#TestNewPageRefuses"
        status: pass
      - kind: unit
        ref: "alita/utils/captchaweb/page_test.go#TestPageNoScriptFallback"
        status: pass
    human_judgment: false
  - id: D4
    description: "Language, challenge, cData, Turnstile language, answer-to-state mapping, contrast guard, widget size and the one-POST latch behave as the UI contract says"
    requirement: CAPT-08
    verification:
      - kind: unit
        ref: "scripts/captcha-page/captcha-page.test.mjs (tests 1-9)"
        status: pass
    human_judgment: false
  - id: D5
    description: "The page decides nothing: a pass, failure, expiry or invalid state comes only from the server's JSON; transport errors, timeouts, 429, 5xx and unknown results are retry; the boot path renders the widget once and POSTs once per token, leaks no relayed value and calls no console method"
    requirement: CAPT-04
    verification:
      - kind: unit
        ref: "scripts/captcha-page/captcha-page.test.mjs#answers and boot tests (tests 6, 10-24)"
        status: pass
    human_judgment: false
  - id: D6
    description: "The page opens as one readable screen in light and dark themes inside a 320 px sheet on Android, iOS, Desktop and Web, and the widget renders inside each WebView"
    requirement: CAPT-08
    verification:
      - kind: manual
        ref: "08-SPIKE.md (plan 08-11), owner-run on real clients"
        status: pending
    human_judgment: true

duration: 45min
completed: 2026-10-30
status: complete
---

# Phase 8 Plan 05: The Turnstile captcha page Summary

**The bot now has the Mini App page: one embedded HTML file served with a per-response nonce and a strict CSP, a nine-state script that relays Telegram's initData, the Turnstile token and the challenge to `/captcha/verify` and shows only what the server answers, with the seven string tables embedded safely and a 24-test node suite over the script.**

## Performance

- **Duration:** about 45 min
- **Tasks:** 3
- **Files created:** 4 (no existing file modified)

## Accomplishments

- `page.go`: `NewPage` refuses an empty site key and an `en` table missing (or holding an empty value for) any of the 26 `PageKeys`; it parses the template once, marshals the tables once with `encoding/json` (so `<`, `>`, `&`, U+2028 and U+2029 are escaped) and hands them over as `template.JS`. `ServeHTTP` answers GET and HEAD, draws a 16-byte `crypto/rand` nonce per response, sets the CSP from `CSPFor`, `Cache-Control: no-store`, `Referrer-Policy: no-referrer`, `X-Content-Type-Options: nosniff`, and never `X-Frame-Options` or a CORS header. It logs nothing.
- `captcha.html`: markup, colour tokens with the light and dark fallbacks, the eight inline icons, the 48 px full-width button, safe-area padding, reduced-motion handling, the icon hidden under 420 px of height, the English first-paint block and the noscript caption, all as the UI contract specifies. The nonce script (`id="captcha-main"`, no template action inside) holds the boot sequence, the 8 s Telegram wait, the 10 s Turnstile wait, cData through `crypto.subtle` (unsupported without it), the explicit widget render with the planned options, the single latch-guarded POST with a 12 s `AbortController`, the answer table, Try again (widget reset, or reload when never rendered), the 2000 ms auto-close, haptics, the contrast guard at boot and on `themeChanged`, and `window.CaptchaPage`.
- `page_test.go`: five Go tests (headers and nonce, safe strings, structure, refusals, no-script fallback).
- `captcha-page.test.mjs`: 24 tests that load the script verbatim into `node:vm`.

## Task Commits

1. **Task 1 RED:** `f2f7cc0` test(08-05): show there is no captcha page (fails to compile: `Page`, `NewPage`, `PageKeys` did not exist)
2. **Task 1 GREEN:** `a852bc5` feat(08-05): serve the Turnstile captcha page
3. **Task 2:** `257ad38` test(08-05): pin the captcha page script's mappings
4. **Task 3 fix:** `410d47f` fix(08-05): wait the full 8 s for the Telegram object
5. **Task 3 tests:** `803a246` test(08-05): prove the captcha page leaks nothing and degrades clearly

## Verification

- `CGO_ENABLED=1 go test -tags testtools -race -count=1 -v ./alita/utils/captchaweb`: `--- PASS` for `TestPageHeadersAndNonce`, `TestPageEmbedsSafeStrings`, `TestPageStructure`, `TestNewPageRefuses`, `TestPageNoScriptFallback`; package `ok`.
- `node --test scripts/captcha-page/*.test.mjs` (Node v22.22.0): `# tests 24`, `# pass 24`, `# fail 0`; five consecutive runs were identical.
- `CGO_ENABLED=0 go build ./...`: ok. `go vet -tags testtools ./alita/utils/captchaweb`: clean. `gofmt -l alita/utils/captchaweb`: nothing. `git diff --exit-code go.mod go.sum`: no diff.
- `make lint` was not run: golangci-lint is built with go1.25 and cannot lint the go1.26.0 module in this sandbox; `go vet` and `gofmt -l` stand in.
- Acceptance greps: `//go:embed page/captcha.html` 1; the `frame-ancestors` string 1; `api.js?render=explicit` 1; `telegram-web-app.js` 1; `credentials: 'omit'` 1; `.innerHTML =` 0; `cData`, `tgWebAppStartParam`, `AbortController`, `CaptchaPage` all at least 1; `from 'node:test'` 1; `func TestPageNoScriptFallback(` 1.
- I did not run `make test` or the `./alita/modules` package; the plan changes no shared code.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] `node --test scripts/captcha-page/` runs nothing on Node 22**
- **Found during:** Task 2 first run
- **Issue:** Node 22.22 treats the argument as a module path (a glob, not a directory): `Cannot find module '.../scripts/captcha-page'`, exit 1. The plan's `<verify>` command and verification section use the bare directory form.
- **Fix:** the working form is `node --test scripts/captcha-page/*.test.mjs`; the test file header says so. The verifier and plans 08-10 and 08-11 (which record the node test command) should use the glob form.
- **Files modified:** scripts/captcha-page/captcha-page.test.mjs (header comment)

**2. [Rule 1 - Bug] Unsupported state came at 7.75 s, not 8 s**
- **Found during:** Task 3 (the boot test advances fake timers to 7999 ms and expects loading)
- **Issue:** the poll counted the wait before it ran, so the 32nd poll at 7750 ms already crossed the 8000 ms line.
- **Fix:** the elapsed time is checked before it is advanced.
- **Files modified:** alita/utils/captchaweb/page/captcha.html
- **Commit:** `410d47f`

**3. [Rule 3 - Blocking] Commit trailer model name**
- **Issue:** the plan text names `Co-Authored-By: Claude Opus 5.5`; the harness attribution for this session is `Claude Sonnet 5.5` and the orchestrator notes direct using the harness trailer lines.
- **Fix:** commits carry the harness's two trailer lines; no model identifier appears in code, comments or docs.

### Honest process notes

- Task 2's RED step did not go red. The whole script was written during Task 1 (it has to exist for the HTML to be complete), so the node suite passed on its first run apart from one wrong expectation in my own test (a light-scheme pair whose white label already equals the fallback is not listed as an override). I corrected the test and kept the lighter commit sequence rather than inventing a failing step. The only code fix the node suite found was the 8 s bug above.
- The boot test waits 30 ms of real time per flush because `webcrypto.subtle.digest` finishes on a worker thread; with only `setImmediate` turns it failed intermittently, with the delay it passed five runs in a row.

**Total deviations:** 3 (one real bug, two process-level). **Impact:** none on the delivered behavior beyond the fixed bug.

## Authentication Gates

None.

## Known Stubs

None. `NewPage` needs the string tables (plan 08-07 builds them from the 7 locale files) and the route (plan 08-10 mounts `GET /captcha`); until then the page is built and tested but not reachable, as planned.

## Threat Flags

None. The page is the planned surface; threat register items T-08-18 (breakout: `TestPageEmbedsSafeStrings`, `TestPageHeadersAndNonce`), T-08-19 (frame-ancestors lists only Telegram web origins, `credentials: 'omit'`), T-08-21 (leak test: no console call, no relayed value in any text or attribute) and T-08-22 (a pass only from a 200 `passed` answer) are mitigated and covered. T-08-20 (compromised third-party script) stays accepted.

## Pending human verification

Needs a real bot, a public HTTPS hostname and Cloudflare keys, so it goes to the owner's spike checklist (`08-SPIKE.md`, plan 08-11) and was not faked:

1. Open the Mini App from a Turnstile group's button on Android, iOS, Telegram Desktop and a Web client; confirm the page appears as one screen, the widget renders inside each WebView, and the page is readable at about 320 px wide in light and dark themes (including a custom Telegram theme).
2. In the browser console of the Web client, confirm there is no CSP violation; if Turnstile's iframe needs an extra `style-src` or `connect-src` entry, add the narrowest one and record it in `08-SPIKE.md`.
3. Confirm `frame-ancestors` allows Telegram Web (`web.telegram.org`, `webk.telegram.org`, `webz.telegram.org`) and `Referrer-Policy: no-referrer` does not break the widget.
4. Confirm the page opens in the compact sheet (optionally with `&mode=compact` on the direct link) and that a swipe during the solve does not close it.
5. Confirm the language shown follows the Telegram account language for all seven locales (hi and ru wrap rather than truncate).
6. Accepted limit: Cloudflare's own iframe rendering (focus, contrast, language) is outside the page's control.

## Issues Encountered

None beyond the deviations above.

## Next Phase Readiness

Plan 08-07 can build `PageOptions.Strings` from the locale files (keys are the full `captcha_web_*` names in `PageKeys`; `en` must be complete). Plan 08-10 mounts `Page` at `GET /captcha`, records the CSP and the node test (glob form) in AGENTS.md, and must pass the public Turnstile site key to `NewPage`.

## Self-Check: PASSED

- `alita/utils/captchaweb/page.go`, `alita/utils/captchaweb/page/captcha.html`, `alita/utils/captchaweb/page_test.go`, `scripts/captcha-page/captcha-page.test.mjs`: FOUND
- Commits `f2f7cc0`, `a852bc5`, `257ad38`, `410d47f`, `803a246`: FOUND
