---
quick_id: 261105-fq1
description: Cut fuku/modules test wall time by 30 percent (measure, then trim fixtures)
status: complete
commit: 9cb059f
---

# Quick task 261105-fq1 summary

## Result

`go test -tags testtools -race -count=1 ./fuku/modules` on 4 CPUs, package `Elapsed` from `-json`:

| Run | Package time | Tests and subtests |
|---|---|---|
| Before (branch head bdde073) | 560.7 s | 3513 pass, 0 fail, 0 skip |
| After the locale managers are built once | 99.0 s | 3513 pass, 0 fail, 0 skip |
| After that and the hung-response fix | **86.3 s** (84.6 percent faster) | 3513 pass, 0 fail, 0 skip |
| Same code under the exact `make test` flags (`-coverpkg` all of `fuku/...`) | 92.4 s, 74.2 percent coverage, gate exit 0 | |

The before and after test sets are identical (same 3513 names, same outcomes). The settings-menu suites went from
89.4 s to 9.0 s. The wall clock of the first run (12 min) included a cold module download and `-race` cgo
build, so the `Elapsed` of the package line is the number to compare.

## What was slow

No single test dominated (the slowest was 17 s). Fixture timing under `-race` showed `withStaffLocale` at
312 ms and `withRealLocales` at 1327 ms per call; miniredis, pacer, fake bot, dispatcher and every `Load*` are
1 to 5 ms. `newLockdownEnv` (295 call sites) and `newSettingsEnv` (154) ran the locale parse in every test and
subtest, which was about 460 s of the 561 s.

## What changed (commit 9cb059f)

- `fuku/i18n/test_helper.go`: `NewManagerForTest` (parse locales into a manager without installing it) and
  `InstallManagerForTest` (install it, return the restore); `OverrideManagerLanguagesForTest` is now built on them.
  Unit tests in `fuku/i18n/test_helper_test.go`.
- `fuku/modules/staff_helpers_test.go`, `settings_env_test.go`: `withStaffLocale` and `withRealLocales` build
  their manager once behind a `sync.Once` and install the shared one per test. A shared manager is safe because
  locale maps, indexes and translators are never written after they are built.
- `fuku/modules/aispam_jev_test.go`: the hung handler drains the request body first, because `net/http` only watches
  for the client leaving once the body has been read. 10.0 s to 0.2 s, same 200 ms deadline and 3 s bound.
- `AGENTS.md`: the new rules (build locale managers once, the body-drain trap, how to measure).

## Not done, on purpose

- No `t.Parallel()`: the package shares one SQLite file (`db.DB`), the cache marshaler, `config.AppConfig`,
  the locale manager singleton and the Redis client, so the tests are not independent. It was not needed.
- No package or build-tag split: the package is now 86 to 92 s, far under the 6-minute line, and CI keeps running everything.
- The Makefile `-timeout` and the CI `timeout-minutes` were not touched; the stop-gap commit named in the task
  (ff87d62) is not on this branch, and the margin is now wide.
- `TestRaidAIFailOpen` (10.5 s) is real work (breaker-tripping loops, 30-message floods under `-race`); cutting it
  would weaken what it proves.

## Verification and caveats

- `go test -tags testtools -race -count=1 -json ./fuku/modules` before and after, diffed by test name.
- `make test` could not finish in this sandbox for a reason that reproduces with these changes stashed: the sandbox
  Go toolchain has no `covdata`, so `go test -coverpkg` fails on `fuku/utils/actionlog` (no test files)
  with `go: no such tool "covdata"`. Every other package in that run was `ok`. The same command for `./fuku/modules
  ./fuku/i18n` piped through `scripts/check_test_results` exits 0.
- `golangci-lint` in the sandbox is built with Go 1.25 and refuses the 1.26 module, so `make lint` did not run here.
  `go vet -tags testtools`, `gofmt` on the touched files and `make check-docs` are clean.
- A shuffled `-count=2` run of the settings, staff, captcha and lockdown-alert tests passes.
- This branch has no `settings_edit_env_test.go` and 10 settings test files; the Phase 11 head named in the task may
  have fixtures this branch lacks. Any new fixture that parses a whole locale file per test should build it once the same way.
