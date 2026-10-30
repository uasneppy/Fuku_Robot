---
phase: 08-turnstile-web-captcha
plan: 11
subsystem: docs
tags: [turnstile, telegram-mini-app, self-hosting, reverse-proxy, docs, spike]

requires:
  - phase: 08-turnstile-web-captcha
    provides: routes, settings, health answer, page CSP, siteverify classification, /captchamode refusals (plans 08-02, 08-05, 08-06, 08-08, 08-09)
provides:
  - self-hosting guide for the web captcha (hostname, proxy, BotFather, Turnstile widget, settings, health, behaviour, troubleshooting)
  - database-schema page lists turnstile in the captcha_mode CHECK
  - 08-SPIKE.md, the owner's 13-check real-device checklist
affects: [08-10 reconcile at merge, phase 8 verification]

actuals:
  tokens: 14000
  tasks: 2
  commits: 2
plan_head_before: 056826f0aa681a8666fb8b61a769cddac3b201e8
plan_head_after: 0e4532659786d6ac0751c8cf1a69a33b3193c56b

tech-stack:
  added: []
  patterns:
    - "Frozen hand-written docs page with the MANUALLY MAINTAINED marker after front matter"

key-files:
  created:
    - docs/src/content/docs/self-hosting/captcha-web.md
    - .planning/phases/08-turnstile-web-captcha/08-SPIKE.md
  modified:
    - docs/src/content/docs/api-reference/database-schema.md

key-decisions:
  - "Caddy example uses a named path matcher with handle blocks and a 404 fallback; nginx uses location = /captcha, location /captcha/ and a 404 catch-all, so neither can forward the metrics, pprof or webhook paths"
  - "The troubleshooting entry for a blank Telegram Web sheet points at CSPFor in page.go instead of the phase-planning spike file, because the guide is a public docs page"
  - "Check 12 admits a failed solve cannot be caused by hand and tells the owner to mark it not reproduced"

patterns-established:
  - "Spike checklist: each numbered check has Steps, Pass looks like, If not, Change"

requirements-completed: [PLAT-04]

coverage:
  - id: D1
    description: "Self-hosting page covering the D-15 steps, the shared-port rule (D-31), what leaves the server and troubleshooting"
    requirement: PLAT-04
    verification:
      - kind: other
        ref: "make generate-docs && make check-docs (no drift) plus the grep loop of Task 1"
        status: pass
    human_judgment: true
    rationale: "Prose accuracy against a real deployment can only be judged by the owner following it (VALIDATION manual-only row 6)"
  - id: D2
    description: "database-schema.md captcha_mode CHECK lists turnstile"
    requirement: PLAT-04
    verification:
      - kind: other
        ref: "grep -c \"captcha_mode IN ('math','text','turnstile')\" database-schema.md prints 1"
        status: pass
    human_judgment: false
  - id: D3
    description: "08-SPIKE.md real-device checklist (D-18)"
    requirement: PLAT-04
    verification:
      - kind: other
        ref: "grep check of Task 2 (all 13 required terms), 8 sections, 26 mentions of change"
        status: pass
    human_judgment: true
    rationale: "The checklist itself is run by the owner on real Telegram clients and with real Cloudflare keys"

duration: 25min
completed: 2026-10-30
status: complete
---

# Phase 8 Plan 11: Web captcha deployment guide and real-device checklist Summary

**One self-hosting page takes the owner from a public hostname to a passing /captcha/health, and 08-SPIKE.md gives a 13-check real-device checklist, each check with its pass signal and the constant to change.**

## Performance

- **Duration:** about 25 min
- **Completed:** 2026-10-30
- **Tasks:** 2
- **Files modified:** 3 (two created, one edited)

## Accomplishments

- `docs/src/content/docs/self-hosting/captcha-web.md` (frozen page, marker in the first 512 bytes): before you start; 1 hostname and the Caddy and nginx proxies that forward only `/captcha` and `/captcha/*`, with the reason (metrics, pprof, webhook share the port) and the two startup warnings; 2 BotFather `/newapp` and `/myapps`; 3 Turnstile Managed widget with the hostname list and the three-hostnames-must-agree rule, plus the dummy-keys warning; 4 the four settings and `CAPTCHA_WEB_TRUSTED_PROXIES`, all-or-nothing, linked to the environment reference, secret scrubbed from logs; 5 `GET /captcha/health` (200 ok, 503 unavailable); 6 `/captchamode turnstile` and its three ordered refusals; what is sent to Cloudflare and Telegram and the `tgWebAppStartParam` access-log note; behaviour (math fallback, time limit, one solve, failed versus retry, replicas); troubleshooting including widget codes 110100, 110110, 110200, 400020, 400021, 400070.
- `database-schema.md`: the `captcha_mode` row now reads `captcha_mode IN ('math','text','turnstile')`.
- `08-SPIKE.md` following the 07-SPIKE.md skeleton: Why this exists, Before you start, 13 numbered checks (direct link per client, `start_param` signed or not, wrong-user tap, widget per WebView, siteverify hostname, `frame-ancestors` and CSP, Referrer-Policy, `crypto.subtle`, `mode=compact`, themes and safe areas, latency against the 12 s client timeout, failed solve / time-out / re-join, BotFather short name), What to read, Decisions, Which constants to change, Checks only a live run can do, What the checklist never does.

## Task Commits

1. **Task 1: web captcha deployment guide and schema row** - `10b8a41` (docs)
2. **Task 2: Turnstile real-device checklist** - `0e45326` (docs)

## Files Created/Modified

- `docs/src/content/docs/self-hosting/captcha-web.md` - the deployment guide
- `docs/src/content/docs/api-reference/database-schema.md` - the `captcha_mode` CHECK now lists `turnstile`
- `.planning/phases/08-turnstile-web-captcha/08-SPIKE.md` - the owner's live checklist

## Decisions Made

- Proxy examples use explicit allow rules with a 404 fallback so a forgotten path is closed by default.
- The guide's blank-sheet entry names `CSPFor` (`alita/utils/captchaweb/page.go`) rather than the planning file, since the guide is public documentation.
- Check 12 tells the owner a failed solve cannot be provoked by hand and to mark it not reproduced instead of faking it.

## Deviations from Plan

None - plan executed exactly as written. Two small notes:

- The plan's acceptance count `grep -c '/api-reference/environment/'` counts matching lines, and the guide has one line holding all five links, so it prints 1 (the criterion is "1 or more").
- The docs site was not built with a local tool (the site build tool was not run); `make generate-docs` and `make check-docs` were run instead and report no drift.

## Verification

- `make generate-docs` and `make check-docs`: "No drift detected".
- Task 1 grep loop: every required term present; marker within the first 512 bytes; schema row count 1; `only.*/captcha` present.
- Task 2 grep loop: all required terms present; 8 `## ` sections; 26 lines mention "change".
- `CGO_ENABLED=0 go build ./...`: succeeds.
- Link and name check: the routes (`GET /captcha`, `POST /captcha/verify`, `GET /captcha/health`), the five variables, `METRICS_AUTH_TOKEN`, `/captchatime` (default 2), `/captchaaction` (kick/ban/mute), the three refusal texts, the link shape `https://t.me/<bot>/<short>?startapp=<token>`, `captcha_join`, the CSP hosts, `Referrer-Policy: no-referrer`, `VERIFY_TIMEOUT_MS` 12000, `InitDataMaxAge` 5 minutes and the short-name pattern were each read in the code or the locale file.
- No live call to Telegram, Cloudflare or any external host was made; PostgreSQL was not started; `make test` was not run (documentation only).

## Issues Encountered

None.

## Known Stubs

None.

## Cross-plan note for the orchestrator

The two startup warnings and the readiness behaviour are documented from `08-10-PLAN.md` (plan 08-10 runs in parallel and was not in this worktree): the guide says the bot warns at startup when `METRICS_AUTH_TOKEN` is empty or `CAPTCHA_WEB_TRUSTED_PROXIES` is empty, and that health answers 503 until the bot has finished starting. If plan 08-10 changes either detail, reconcile the "Why only these paths" and step 5 sections at merge.

## Threat Flags

None. Documentation only; the guide and checklist carry variable names only, never values (T-08-48), and both proxy examples forward only `/captcha` and `/captcha/*` (T-08-46).

## Next Phase Readiness

Phase 8 documentation is complete. The owner can run `08-SPIKE.md` once a hostname, a Mini App and Turnstile keys exist; its results decide whether the CSP host list, the Referrer-Policy, the link builder or the client timeout change.

## Self-Check: PASSED

- FOUND: docs/src/content/docs/self-hosting/captcha-web.md
- FOUND: .planning/phases/08-turnstile-web-captcha/08-SPIKE.md
- FOUND: commits 10b8a41 and 0e45326

---
*Phase: 08-turnstile-web-captcha*
*Completed: 2026-10-30*
