---
phase: 08-turnstile-web-captcha
plan: 02
subsystem: config
tags: [config, turnstile, captcha, logredact, env-docs]

requires: []
provides:
  - "Config fields CaptchaWebBaseURL, TelegramMiniAppShortName, TurnstileSiteKey, TurnstileSecretKey, CaptchaWebTrustedProxies"
  - "Config.CaptchaWebConfigured (true only when all four required values are set)"
  - "validateCaptchaWeb, called at the end of ValidateConfig (all-or-nothing, errors name variables only)"
  - "registerSecrets registers the Turnstile secret with logredact"
  - "sample.env block and environment reference sections for the five variables"
affects: [08-10 runtime wiring, 08-08 captchaweb trusted-proxy parsing]

actuals:
  tokens: 6000
  tasks: 2
  commits: 3
plan_head_before: c3cd9515ed551fedcfcea411661d6a30faf2c90f
plan_head_after: e9f62e559f99676feaf5ef6c46c7ac0ea016cb4a

tech-stack:
  added: []
  patterns:
    - "All-or-nothing optional feature configuration; validation errors name the variable and the rule, never the value"

key-files:
  created:
    - alita/config/captcha_web_test.go
  modified:
    - alita/config/config.go
    - sample.env
    - docs/src/content/docs/api-reference/environment.md

key-decisions:
  - "Validation uses the standard library only (net/url, net/netip, regexp, strings), so alita/config does not import anything that reaches alita/db"
  - "A proxy list set without the four required values is a startup error"
  - "The secret minimum length is the redactor's byte floor (6), checked with len()"
  - "A base URL containing ? or # anywhere is refused before parsing, so a bare trailing ? or # cannot slip through"

requirements-completed: [PLAT-04]

coverage:
  - id: D1
    description: "The four settings plus the optional proxy list load trimmed from the environment and validate all-or-nothing at startup, with errors that name variables only"
    requirement: PLAT-04
    verification:
      - kind: unit
        ref: "alita/config/captcha_web_test.go#TestCaptchaWebConfig"
        status: pass
    human_judgment: false
  - id: D2
    description: "The Turnstile secret is registered with the log redactor; the public site key is not"
    requirement: PLAT-04
    verification:
      - kind: unit
        ref: "alita/config/captcha_web_test.go#TestCaptchaWebConfig/the_secret_is_registered"
        status: pass
    human_judgment: false
  - id: D3
    description: "sample.env and the environment reference document the five variables, the all-or-nothing rule and the secret's handling, with no value"
    requirement: PLAT-04
    verification:
      - kind: other
        ref: "per-variable grep loop from the plan's Task 2 <verify>, plus make check-docs (no drift)"
        status: pass
    human_judgment: false

duration: 5min
completed: 2026-10-30
status: complete
---

# Phase 8 Plan 02: Web captcha owner settings Summary

**Five web captcha settings (base URL, Mini App short name, Turnstile site key and secret, optional trusted proxies) load trimmed, validate all-or-nothing with value-free errors at startup, and the Turnstile secret is registered with the log redactor.**

## Performance

- **Duration:** 5 min
- **Started:** 2026-10-30T00:22:00Z (approx.)
- **Completed:** 2026-10-30T00:27:00Z
- **Tasks:** 2
- **Files modified:** 4 (1 created)

## Accomplishments

- `alita/config/config.go`: five new `Config` fields, read with `os.Getenv` plus `strings.TrimSpace`; `CaptchaWebConfigured()`; `validateCaptchaWeb` called as the last step of `ValidateConfig`; `registerSecrets` passes `cfg.TurnstileSecretKey`.
- Validation rules: none of the four set is fine; some but not all is `web captcha settings are incomplete: missing <names>` in a fixed order; a proxy list alone is an error; base URL must be https with a host, no user info, query or fragment, and a path that is empty or `/`; short name `^[A-Za-z0-9_]{1,64}$`; secret at least 6 bytes; every proxy entry an IP address or CIDR prefix. Every error names the variable and the rule only.
- `TestCaptchaWebConfig` (new, 20 subtests): none set, all four (with and without `/`), all 14 proper subsets, proxies alone, nine bad base URLs, four bad short names, short secret (5 fails, 6 passes), bad and good proxy lists, loading from the environment with padded values, a partial environment stopping `LoadConfig`, and the secret's registration (site key left unscrubbed).
- `sample.env` and the frozen `environment.md` document all five variables; the new reference section sits after the AI spam section and the flat listing has the five lines in alphabetical order.

## Task Commits

1. **Task 1 RED:** `9416f43` test(08-02): show the web captcha settings are not loaded or checked (fails to compile: the symbols did not exist yet)
2. **Task 1 GREEN:** `d0cdf1c` feat(08-02): load and validate the web captcha settings
3. **Task 2:** `e9f62e5` docs(08-02): document the web captcha settings

## Verification

- `CGO_ENABLED=1 go test -tags testtools -race -count=1 -run '^TestCaptchaWebConfig$' -v ./alita/config`: `--- PASS: TestCaptchaWebConfig` with all subtests passing.
- `CGO_ENABLED=1 go test -tags testtools -race -count=1 ./alita/config`: ok.
- `CGO_ENABLED=0 go build ./...`: ok. `go vet -tags testtools ./alita/config`: clean. `gofmt -l alita/config`: nothing.
- `make lint` was not run: golangci-lint is built with go1.25 and cannot lint the go1.26.0 module in this sandbox; `go vet` and `gofmt -l` stand in.
- `make check-docs`: no drift. (`make check-translations` not run: no locale keys were touched.)
- Task 1 acceptance greps: each `os.Getenv("<VAR>")` count is 1 for all five variables; `cfg.TurnstileSecretKey` count 3; `func (c *Config) CaptchaWebConfigured() bool` count 1; `captchaweb` import count 0; `go.mod` and `go.sum` unchanged.
- Task 2 acceptance: per-variable loop prints ALL_OK; MANUALLY MAINTAINED marker count 1; `^#TURNSTILE_SECRET_KEY=$` count 1; `### \`TURNSTILE_SECRET_KEY\`` count 1; scrubbed/redact count 6.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Commit trailer model name**
- **Found during:** Task 1 commit
- **Issue:** The plan text names `Co-Authored-By: Claude Opus 5.5`; the harness attribution for this session is `Claude Sonnet 5.5`, and the orchestrator notes direct using the harness trailer lines.
- **Fix:** Commits carry the harness's two trailer lines. No model identifier appears in code, comments or docs.

**2. [Rule 3 - Blocking] Plan-commit ledger not written**
- **Found during:** before the first commit
- **Issue:** The sandbox refused the compound `git rev-parse --git-dir` command that writes the per-plan ledger file.
- **Fix:** `plan_head_before` is the known worktree base `c3cd9515ed551fedcfcea411661d6a30faf2c90f`, and `commits: 3` was measured with `git rev-list --count <base>..HEAD` before this SUMMARY commit.

**Total deviations:** 2 (both process-level, no code impact). **Impact:** none on the delivered behavior.

Beyond the plan's list, the test file also covers an empty-query base URL (`https://host?`) and a 64-character short name boundary; the implementation refuses any `?` or `#` in the raw URL before parsing so a bare trailing `?` cannot pass.

## Authentication Gates

None.

## Known Stubs

None. The settings take effect only when plan 08-10 wires the routes and the runtime; until then a complete set changes nothing but the secret's registration (as the plan states).

## Threat Flags

None. No new endpoint, auth path or file access; the threat register items T-08-07 to T-08-10 are mitigated and covered by `TestCaptchaWebConfig` ("the secret is registered", "short secret", "partial", "bad base URL", "proxies without the four").

## Pending human verification

None for this plan (no live host, bot or Cloudflare keys needed).

## Issues Encountered

None.

## Next Phase Readiness

Plan 08-08 can parse `Config.CaptchaWebTrustedProxies` with `net/netip` (the string is already validated); plan 08-10 can gate route registration on `AppConfig.CaptchaWebConfigured()` and record the configuration rule in AGENTS.md as planned.

## Self-Check: PASSED

- `alita/config/captcha_web_test.go`: FOUND
- Commits `9416f43`, `d0cdf1c`, `e9f62e5`: FOUND
