---
phase: 07-ai-assisted-raid-detection
plan: 03
subsystem: ai-raid-detection
status: complete
tags: [gemini, config, logredact, secrets, docs]

requires:
  - phase: 07-ai-assisted-raid-detection
    provides: "07-02 gemini client: DefaultModel and ValidModel"
provides:
  - "Config.GeminiAPIKey and Config.GeminiModel, loaded from GEMINI_API_KEY and GEMINI_MODEL"
  - "setDefaults: empty model becomes gemini.DefaultModel, a name failing gemini.ValidModel falls back with one warning"
  - "registerSecrets(cfg): one helper registering every secret, the Gemini key included"
  - "logredact structural pattern AIza[0-9A-Za-z_-]{35}"
  - "GEMINI_API_KEY and GEMINI_MODEL documented in sample.env and the environment docs"
affects: [07-04, 07-08 image path, 07-09 provider pause]

requirements-completed: [PLAT-02, RAID-04]

actuals:
  tokens: 9000
  tasks: 2
  commits: 4

plan_head_before: 64b384ed107868efc0042c2a1f8ae75120812bb6
plan_head_after: 3af42d6
commits: 4

tech-stack:
  added: []
  patterns:
    - "Secrets registered through one registerSecrets helper called from the config init()"
    - "Exact-value registration is the primary control, a structural AIza pattern is the backstop"

key-files:
  created: []
  modified:
    - alita/config/config.go
    - alita/config/config_pure_test.go
    - alita/utils/logredact/logredact.go
    - alita/utils/logredact/logredact_test.go
    - sample.env
    - docs/src/content/docs/api-reference/environment.md

key-decisions:
  - "registerSecrets keeps every previously registered secret and adds cfg.GeminiAPIKey; init() calls it where the inline call was"
  - "A GEMINI_MODEL outside the allow-list falls back to the default instead of failing startup, with one log.Warnf quoting the value"
  - "No GEMINI_MAX_PER_HOUR style variables: budgets stay code constants (D-20)"

duration: 15 min
completed: 2026-10-29
---

# Phase 7 Plan 03: Gemini config and key redaction Summary

The bot now reads `GEMINI_API_KEY` and `GEMINI_MODEL`, registers the key as a secret, scrubs any Google-key-shaped string from logs, and documents what goes to Google when the key is set.

## Accomplishments

- `Config.GeminiAPIKey` and `Config.GeminiModel` load beside `TYPESAFE_API_KEY`. An unset key leaves the image path inert and does not fail `LoadConfig`.
- `GEMINI_MODEL` defaults to `gemini-3.5-flash-lite`. A name that fails `gemini.ValidModel` (for example `../v1/files`, a 65-character name, `gemini?key=x`) is replaced by the default with a warning, so it never reaches a request URL (T-07-12).
- `registerSecrets(cfg)` passes BotToken, DatabaseURL, RedisPassword, WebhookSecret, MetricsAuthToken, TypeSafeAPIKey and GeminiAPIKey to `logredact.RegisterSecret` (PLAT-02, T-07-11).
- `logredact` has a structural pattern `AIza[0-9A-Za-z_-]{35}` that scrubs an unregistered key in a URL query, a JSON body or prose, and leaves shorter strings and the plain word "AIza" alone.
- `sample.env` and `environment.md` (marker kept) describe both variables: default, inert without the key, what is sent to Google, the paid-tier recommendation, the model allow-list and fallback (T-07-13, D-21).

## Tasks

| Task | Name | Commits |
|------|------|---------|
| 1 | Gemini key and model load, default, validate, register | 9ef88d9 (test, RED), 1aa7785 (feat) |
| 2 | AIza structural scrub and docs | 8014f2c (test, RED), 3af42d6 (feat) |

Both tasks followed RED then GREEN. RED for Task 1 was a compile failure (the fields did not exist); RED for Task 2 was three failing subtests showing the key leaked.

## Verification

- `CGO_ENABLED=1 go test -tags testtools -race -count=1 -run '^(TestGeminiConfig|TestLoadConfig|TestSetDefaults)$' -v ./alita/config`: PASS, including `--- PASS: TestGeminiConfig`.
- `CGO_ENABLED=1 go test -tags testtools -race -count=1 -run '^(TestScrubGoogleAPIKeyShape|TestScrubStructuralPatterns|TestScrubDoesNotCorruptOrdinaryProse)$' -v ./alita/utils/logredact`: PASS.
- `CGO_ENABLED=1 go test -tags testtools -race -count=1 ./alita/config ./alita/utils/logredact`: PASS.
- `CGO_ENABLED=0 go build ./...`: clean. `go vet -tags testtools ./alita/config ./alita/utils/logredact`: clean. `gofmt -l alita/config alita/utils/logredact`: nothing.
- `make check-docs`: no drift. `git diff --exit-code go.mod go.sum`: unchanged.
- `make lint` was not run: golangci-lint is built with Go 1.25 and cannot lint the Go 1.26 module here, so `go vet -tags testtools` and `gofmt -l` stand in for it.
- Acceptance greps: `os.Getenv("GEMINI_API_KEY")` 1, `os.Getenv("GEMINI_MODEL")` 1, `func registerSecrets(` 1, `gemini.ValidModel` 1, `AIza` in logredact.go present, `GEMINI_API_KEY` in environment.md 3, MANUALLY MAINTAINED marker kept, "paid" present.

## Deviations from Plan

None - plan executed exactly as written. One small addition inside the planned test: the "the key is registered" subtest also registers a value that the `AIza` pattern cannot match, so exact-value registration stays proven once Task 2's pattern would scrub the plan's example key on its own.

## Known Stubs

None.

## Threat Flags

None. No new endpoints, auth paths or schema; the plan's threat register (T-07-11, T-07-12, T-07-13) is covered as described above.

## Pending human verification

- Live: start the bot with a real `GEMINI_API_KEY` and confirm the key never appears in the startup log, a failed-request log or the `/metrics` output (cannot be done here).
- Live: set `GEMINI_MODEL=../x` and confirm the warning is logged once and the default model is used.

## Self-Check: PASSED

- Modified files exist on disk; commits 9ef88d9, 1aa7785, 8014f2c and 3af42d6 are in `git log`; `git rev-list --count 64b384e..HEAD` is 4 before this summary commit.
