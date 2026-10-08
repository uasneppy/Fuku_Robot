---
phase: quick-261008-j3b
plan: 01
subsystem: shutdown
tags: [shutdown, staff-actions, lockdown, docker, graceful-drain]
requires: []
provides:
  - shutdown.Manager.RegisterHandlerWithTimeout (per-handler allowance, bounded by the 60 s budget)
  - modules.RegisterStaffActionsDrain and modules.RegisterLockdownWorkerDrain
affects: [main.go, docker-compose.yml, AGENTS.md]
key-files:
  created:
    - alita/utils/shutdown/graceful_testtools.go
    - alita/modules/shutdown_drains.go
    - alita/modules/shutdown_drains_test.go
  modified:
    - alita/utils/shutdown/graceful.go
    - alita/utils/shutdown/graceful_test.go
    - main.go
    - docker-compose.yml
    - AGENTS.md
decisions:
  - "Drain allowance = the drain's own wait + shutdownDrainGrace (1 s), read at registration"
  - "Each handler context derives from the 60 s budget context, so a long per-handler timeout can never outlast the budget"
  - "Production container stop_grace_period 75 s"
metrics:
  tasks: 3
  commits: 3
  plan_head_before: a65c5733922e5944b55bf03967a4dd3ea3c42537
  plan_head_after: 6f7da319ca5484b4e3a8a122efca8dc99a3043b9
status: complete
---

# Quick 261008-j3b: Shutdown timing gap Summary

The shutdown manager now gives the staff-action and lockdown-worker drains their own wait plus a 1 s grace instead of a flat 10 s, so the DB-close handler no longer runs under a staff run that is still finalizing its record and summary.

## Commits

- `29b7838` fix: give the staff action drain its full wait before the database closes at shutdown
- `c8fc7b6` fix: tie the lockdown worker's shutdown allowance to its own wait
- `6f7da31` fix: let Docker wait out the 60 s graceful shutdown and document drain allowances

## What changed

- `graceful.go`: `defaultHandlerTimeout` (10 s), index-aligned `timeouts`, `RegisterHandlerWithTimeout`, and the loop extracted to `runHandlers(budget, defaultTimeout) int` (1 on global timeout, 0 otherwise). `shutdown()` keeps its once/panic-recovery/logging and calls `exitProcess(runHandlers(...))`. `handlers []func() error` is unchanged, so the existing tests pass untouched.
- `graceful_testtools.go` (`//go:build testtools`): `RunForTest`. Not in production builds.
- `shutdown_drains.go`: `shutdownDrainGrace`, `RegisterStaffActionsDrain` (`staffActionStopWait` + grace), `RegisterLockdownWorkerDrain` (`lockdownWorkerStopWait` + grace).
- `main.go`: both inline handlers replaced by the helpers in the same slots (sweeper < staff drain < lockdown drain < AI spam). LIFO order unchanged.
- `docker-compose.yml`: `stop_grace_period: 75s` on the `alita` service only.
- `AGENTS.md` "Startup and shutdown": per-handler timeout rule, both helpers, webhook/polling worst case, deploy grace requirement. Handlers/Permissions sections (j3a) untouched.

## Verification

- Red-first: `TestShutdownWaitsForStaffRunSummary` failed to compile before the implementation.
- Mutation checks (not committed): staff drain via plain `RegisterHandler` makes `TestShutdownWaitsForStaffRunSummary` fail (pending lines still on the card); lockdown drain with default allowance makes `TestShutdownWaitsForLockdownWorker` fail (1 joiner row still `unbanning`). Both restored.
- `go test -tags testtools -race ./alita/utils/shutdown/` passes (existing tests + 3 new guard tests: outlasts default, default still applies for RegisterHandler/0/negative, bounded by budget).
- `go test -tags testtools -race -count=5 -run '^(TestShutdownWaitsForStaffRunSummary|TestShutdownWaitsForLockdownWorker)$' ./alita/modules/` passes.
- `go test -tags testtools -race -count=1 .` (main package) passes.
- `go test -tags testtools -race -count=1 -timeout 15m ./alita/modules/` passes (192 s).
- `CGO_ENABLED=0 go build ./...` passes; `go vet -tags testtools` on modules, shutdown and main is clean.
- `make lint` could not run here (installed golangci-lint built with Go 1.25); used `go vet -tags testtools` and `gofmt -l` instead. `gofmt -l` flags only `alita/modules/greetings_command_test.go`, which is pre-existing and untouched by this item.

## Deviations from Plan

None - plan executed as written. Task 3's commit also carried only docker-compose.yml and AGENTS.md. The per-plan commit-ledger file was not created because the base commit is recorded above and measured with `git rev-list --count` (3).

## Known Stubs

None.

## Threat Flags

None.

## Open notes for the owner

- `DrainAISpamChecks` waits up to 30 s internally (`aispamDrainTimeout`) but keeps the 10 s default allowance. Raising it would push the webhook-mode worst case past 60 s, so it was left alone.
- Polling mode worst case: updater 10 s + HTTP 10 s + AI spam 10 s + lockdown 6 s can let the 60 s budget end the staff drain early (exit 1). Production runs webhooks.
- `render.yaml` (Render default 30 s shutdown delay), `app.json`/Procfile (Heroku fixed 30 s) and `railway.toml` were not changed. On those platforms the drain is cut at the platform's limit.
- The STATE.md [Cross-phase] shutdown concern can be closed by the batch coordinator.

## Self-Check: PASSED

Created files exist, commits 29b7838, c8fc7b6, 6f7da31 exist on branch `worktree-agent-a61e13b82af40a9bc`.
