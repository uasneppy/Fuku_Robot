---
phase: 08-turnstile-web-captcha
plan: 09
subsystem: captcha
tags: [captcha, turnstile, captchamode, i18n, docs]
requires:
  - phase: 08-01
    provides: captchaWebCurrent runtime, captchaWebAvailable, Mini App join flow
  - phase: 08-04
    provides: captcha.SetCaptchaMode accepting turnstile
  - phase: 08-07
    provides: captcha_web_* locale keys and TestCaptchaWebLocaleKeys
  - phase: 08-08
    provides: the verify route and its safety work
provides:
  - "/captchamode turnstile with three ordered refusals (captchaWebModeRefusal)"
  - "Four new and four edited locale keys in all 7 languages"
  - "Captcha commands page and AGENTS.md rule for the Turnstile mode"
affects: [08-10, 08-11]
tech-stack:
  added: []
  patterns:
    - "Refuse-before-write: a mode that would only fall back is never stored"
key-files:
  created:
    - alita/modules/captcha_mode_turnstile_test.go
  modified:
    - alita/modules/captcha.go
    - locales/en.yml
    - locales/es.yml
    - locales/fr.yml
    - locales/hi.yml
    - locales/id.yml
    - locales/pt.yml
    - locales/ru.yml
    - docs/src/content/docs/commands/captcha/index.md
    - AGENTS.md
key-decisions:
  - "Refusal order is fixed: no runtime or bot username, then a fresh 250 ms Redis PING (not the 5 s memo), then a live FetchBotMember (creator, or administrator with can_restrict_members; unknown refuses)"
  - "Only Restrict members is named: SendCaptcha and the unmute path need that right; message deletion is best effort in every mode"
  - "The mode word is the literal \"turnstile\", as the rest of captcha.go uses; no new constant was added"
requirements-completed: [CAPT-01]
duration: 20 min
completed: 2026-10-30
status: complete
commits: 4
plan_head_before: 9dc5d056b34b62517743d79dda0476559c9f7990
plan_head_after: 007cd1ce30b61db9cce289de31663f21ddc18709
actuals:
  tokens: 14000
  tasks: 2
  commits: 4
coverage:
  - deliverable: "/captchamode turnstile stores the mode only when the bot can run it, else a plain refusal with nothing written"
    verification:
      - kind: test
        ref: "alita/modules/captcha_mode_turnstile_test.go#TestCaptchaModeTurnstile"
        status: pass
      - kind: test
        ref: "alita/modules/captcha_mode_turnstile_test.go#TestCaptchaModeTurnstileRefusals"
        status: pass
    human_judgment: false
  - deliverable: "Mode-listing texts name math, text and turnstile in all 7 locales and on the commands page"
    verification:
      - kind: test
        ref: "alita/modules/captcha_mode_turnstile_test.go#TestCaptchaModeListsTurnstileInEveryLanguage"
        status: pass
      - kind: command
        ref: "make check-translations && make check-docs"
        status: pass
    human_judgment: false
  - deliverable: "Quality of the six non-English translations of the new refusal and description texts"
    verification: []
    human_judgment: true
    rationale: "Wording quality in es, fr, hi, id, pt and ru is not asserted by any test; a native reader should skim the 4 new keys"
---

# Phase 8 Plan 09: /captchamode turnstile Summary

**`/captchamode turnstile` becomes the third captcha mode, stored only after three ordered live checks (web captcha runtime, a fresh Redis PING, the bot's own Restrict members right), with every mode-listing text corrected in 7 languages and on the docs page.**

## Performance

- **Duration:** about 20 min (2026-10-30)
- **Tasks:** 2 of 2
- **Files:** 11 (1 created, 10 modified)

## Accomplishments

- `captchaModeCommand` accepts `turnstile`. After `captchaAdminGate` (unchanged, still first) it calls the new `captchaWebModeRefusal(bot, chat)`, which returns the refusal locale key or `""`: `captcha_web_refuse_not_configured` (no runtime installed, no bot username), `captcha_web_refuse_no_redis` (no PING within `captchaweb.RedisTimeout`, 250 ms, fresh, not the 5 s memo), `captcha_web_refuse_no_rights` (live `chat_status.FetchBotMember`: missing, unknown, or not creator/administrator with `can_restrict_members`). A refusal replies in the admin's language and writes nothing. `captcha_mode_turnstile_desc` fills the confirmation.
- 4 new keys in all 7 locale files (`captcha_mode_turnstile_desc`, three `captcha_web_refuse_*`) and 4 edited keys (`captcha_mode_specify`, `captcha_mode_invalid`, `captcha_help_msg`, `captcha_invalid_mode_error`), mode words untranslated.
- `docs/src/content/docs/commands/captcha/index.md` (hand-edited, marker kept) lists the Turnstile type, the `<math/text/turnstile>` line and links `/self-hosting/captcha-web/` (the page plan 08-11 writes).
- `AGENTS.md` Permissions records the command's rule and refusal order.

## Task Commits

1. RED `test(08-09)`: `677c913` (TestCaptchaModeTurnstile, TestCaptchaModeTurnstileRefusals)
2. Task 1 `feat(08-09)`: `e156323` (command, helper, 4 keys x 7 locales, AGENTS.md)
3. RED `test(08-09)`: `0dffe6f` (TestCaptchaModeListsTurnstileInEveryLanguage)
4. Task 2 `docs(08-09)`: `007cd1c` (4 edited keys x 7 locales, commands page)

## Test coverage

- `TestCaptchaModeTurnstile`: admin 777000 gets `captcha_mode_set_formatted` with `turnstile` and the description marker, the stored mode is `turnstile`, a following join gets a Mini App token, and math and text still store.
- `TestCaptchaModeTurnstileRefusals` (subtests): not configured, no Redis (the runtime's own miniredis closed), bot plain member, bot administrator without Restrict members, rights unknown (getChatMember error), ordering (not configured wins over no rights), non-admin (ext.EndGroups, none of the refusal texts). Each shows exactly one reply and the stored mode still `math`.
- `TestCaptchaModeListsTurnstileInEveryLanguage`: the four keys name the modes in all 7 files; the no-argument and bogus-mode replies use the right keys.
- Run: `go test -tags testtools -race -run 'Captcha|Help|Locale|Translat' ./alita/modules ./alita/i18n` pass; `make check-translations`, `make check-docs`, `CGO_ENABLED=0 go build ./...`, `go vet -tags testtools ./alita/modules`, `gofmt -l alita/modules` all clean. The full `./alita/modules` package and `make test` were not run (the orchestrator runs them between waves); `make lint` cannot run in this sandbox (golangci-lint built with go1.25 against module go1.26.0), so `gofmt` and `go vet` stood in.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Plan symbol] No `captchaweb.ModeTurnstile` constant**
- **Found during:** Task 1
- **Issue:** I first wrote `captchaweb.ModeTurnstile`; the package has no such constant (`captcha.go` and the repository use the literal).
- **Fix:** used the literal `"turnstile"` like the rest of `captcha.go`.
- **Files modified:** alita/modules/captcha.go
- **Commit:** e156323

**2. [Test shape] Bot rights simulated with a wrapper client**
- **Issue:** `moduleBotClient` hardcodes bot ID 999 as an administrator and its `errors` map is per method, so a "plain member" or "unknown" bot could not be expressed without also breaking the admin gate.
- **Fix:** the test file holds a small `turnstileBotClient` (embeds the shared fake, overrides only the bot's own `getChatMember`), used with a bot of ID 555. No shared harness file was touched.
- **Commit:** 677c913

**3. [Attribution] Commit trailer**
- The plan text names one model line in its trailer; the harness's attribution instruction for this run gives `Co-Authored-By: Claude Sonnet 5.5`, which the orchestrator told me to use. Used the harness lines.

**Total deviations:** 3 minor, no behaviour change from the plan.

## Authentication Gates

None.

## Known Stubs

None.

## Threat Flags

None. T-08-38 (non-admin changing the mode) is held by the unchanged `captchaAdminGate` plus the "not an admin" subtest; T-08-39 by the three refusals; T-08-40 by plain owner-facing texts that name no setting, variable, secret or URL.

## Pending human verification

- A native reader should skim the new `captcha_mode_turnstile_desc` and `captcha_web_refuse_*` texts in es, fr, hi, id, pt and ru (translations were written by me; no test judges wording).
- Live check (deferred to 08-SPIKE.md by the owner): on a real bot with the web captcha configured, `/captchamode turnstile` in a group where the bot is an administrator with Restrict members replies with the confirmation; with the right removed it replies with the rights refusal and the mode stays.

## Next Phase Readiness

The command is wired. It depends on `captchaWebCurrent` being installed at startup, which is plan 08-10's wiring; until then every `/captchamode turnstile` correctly answers "not set up". The docs link `/self-hosting/captcha-web/` resolves once plan 08-11 writes that page.

## Self-Check: PASSED

- alita/modules/captcha_mode_turnstile_test.go: FOUND
- Commits 677c913, e156323, 0dffe6f, 007cd1c: FOUND
- Acceptance criteria of both tasks re-run and passing.
