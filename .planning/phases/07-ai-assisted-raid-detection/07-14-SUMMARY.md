---
phase: 07-ai-assisted-raid-detection
plan: 14
subsystem: raid-detection
tags: [shutdown, drain, goroutine, context-cancel, raid-ai, lockdown]

requires:
  - phase: 07-ai-assisted-raid-detection
    provides: "raid AI pool, raidAIRecordVerdict, settleAI (plans 07-01, 07-06, 07-07)"
provides:
  - "raidAIFireAsync / raidAIFires: an AI-triggered lockdown start on its own tracked, recovered goroutine"
  - "raidAIContext: the pool's cancellable context, every raid-only text call derives from it"
  - "StopRaidAI + RegisterRaidAIDrain: shutdown drain (cancel first, wait at most 1 s), registered in main.go"
  - "TestRaidAIPool: pool bounds, shedding, panic recovery, drain"
affects: [07-08 image job (must derive its call context from raidAIContext), shutdown budget]

plan_head_before: cd3f040e9ed4572bb17819c7da2d56255b4ffff9
plan_head_after: d2ea39c74320829a6f68c564c5f3476765432404

actuals:
  tokens: 14000
  tasks: 2
  commits: 4

tech-stack:
  added: []
  patterns:
    - "A lockdown start asked for by a verdict runs on a goroutine counted in a raidPostTracker and recovered, never on the worker"
    - "A drain cancels the pool context first, then waits a bounded second for trackers"

key-files:
  created:
    - alita/modules/raid_ai_pool_test.go
  modified:
    - alita/modules/raid_ai.go
    - alita/modules/raid_ai_count.go
    - alita/modules/shutdown_drains.go
    - main.go
    - alita/modules/raid_ai_burst_test.go
    - AGENTS.md

key-decisions:
  - "A check cut off by the drain is no verdict and is not a provider failure: it counts as shed and never touches the chat's breaker"
  - "StopRaidAI leaves raidAIDraining set (the process is exiting); tests restore the pool with restoreRaidAIPool"
  - "Queued jobs are dropped by the worker once the pool context is cancelled, counted as shed"

patterns-established:
  - "Test gate on one Bot API method (raidMethodGate / gateMethod) to hold a single Telegram call"
  - "A fake httptest handler must read the request body before r.Context() sees a client disconnect"

requirements-completed: [RAID-05, RAID-03]

coverage:
  - id: D1
    description: "A lockdown start asked for by an AI verdict runs off the raid AI worker and the /aispam worker"
    requirement: RAID-05
    verification:
      - kind: unit
        ref: "alita/modules/raid_ai_pool_test.go#TestRaidAIPool/a_lockdown_start_runs_off_the_worker and the_/aispam_worker_is_not_held_either"
        status: pass
    human_judgment: false
  - id: D2
    description: "The raid AI pool is bounded (4 workers, 128 queued), sheds when full and survives a panicking job"
    requirement: RAID-05
    verification:
      - kind: unit
        ref: "alita/modules/raid_ai_pool_test.go#TestRaidAIPool/bounded_and_shed and a_panicking_job_is_recovered"
        status: pass
    human_judgment: false
  - id: D3
    description: "StopRaidAI cancels in-flight AI checks, waits at most 1 s for jobs and starts, and is registered between the AI spam drain and the raid posts drain"
    requirement: RAID-03
    verification:
      - kind: unit
        ref: "alita/modules/raid_ai_pool_test.go#TestRaidAIPool/the_drain_cancels_and_returns and the_drain_waits_for_a_lockdown_start"
        status: pass
    human_judgment: false
  - id: D4
    description: "Real-process shutdown ordering and the 60 s budget with the new drain"
    verification: []
    human_judgment: true
    rationale: "Only a live SIGTERM of a running bot shows the LIFO order and the staff drain's remaining budget; no unit test starts the shutdown manager with every drain registered"

duration: 20min
completed: 2026-10-29
status: complete
---

# Phase 7 Plan 14: Raid AI pool lifecycle Summary

**An AI-triggered lockdown start now runs on its own tracked, panic-recovered goroutine, and a new `StopRaidAI` shutdown drain cancels every in-flight AI check and waits at most 1 s before the raid posts drain and the database close.**

## Performance

- **Duration:** about 20 min (2026-10-29T18:58Z to 19:18Z)
- **Tasks:** 2 of 2
- **Files:** 1 created, 6 modified

## Accomplishments

- `raidAIFireAsync(b, chat, trig)` in `raid_ai_count.go` starts `raidFire` on a goroutine counted in `raidAIFires` (a `raidPostTracker`, not a `sync.WaitGroup`) and starting with `defer error_handling.RecoverFromPanic("raidAIFire", "Lockdown")`. `raidAIRecordVerdict` calls it, so neither a raid AI worker nor an `/aispam` worker waits up to `raidFireTimeout` on Telegram.
- `raid_ai.go` gained `raidAIContext()` (a mutex-guarded cancellable context created with the workers), `raidAIStopWait` (1 s, a variable for tests) and `StopRaidAI() bool`. `raidAIProcessText` derives its TypeSafe call from the pool context. After cancellation a failed call is counted as shed (no verdict, breaker untouched) and a queued job is dropped by the worker as shed.
- `RegisterRaidAIDrain` (`shutdown_drains.go`) registers the drain with `raidAIStopWait + shutdownDrainGrace` and the log line "[Shutdown] Draining raid AI checks...". `main.go` calls it directly after `RegisterRaidPostsDrain` and before the inline `DrainAISpamChecks` handler (lines 221, 226, 230), so LIFO runs AI spam drain, then raid AI drain, then raid posts drain, then the database close.
- `TestRaidAIPool` pins six behaviours: start off the raid AI worker, start off the `/aispam` worker, bounded and shed (4 held jobs, 128 queued, the next one shed and counted), panic recovery, the drain cancelling a hung provider call (returned in about 0.3 ms, the server saw its request context cancelled, a later enqueue is shed), and the drain waiting on a held lockdown start. `settleAI` now also waits for `raidAIFires`.
- `AGENTS.md` updated in the same commits as the code: the "AI never decides alone" bullet (start on `raidAIFires`), the pool bullet (context, drain, `restoreRaidAIPool`), a new `StopRaidAI` bullet under Startup and shutdown, and the webhook-mode budget sentence before `StopStaffActions` (raid AI 2 s added; the staff drain can be cut up to 2 s short when every earlier drain runs to its limit).

## Task Commits

1. Task 1 RED: `1575625` test(07-14): show an AI lockdown start holds the worker that asked for it
2. Task 1 GREEN: `d336df7` feat(07-14): start AI-triggered lockdowns off the check workers
3. Task 2 tests: `2809f30` test(07-14): pin the raid AI pool's bounds and panic recovery
4. Task 2 GREEN: `d2ea39c` feat(07-14): drain the raid AI pool at shutdown

## Deviations from Plan

None that change behaviour. Two sequencing notes:

1. **RED commits compile.** The plan asks for a RED commit that shows the pool "has no drain". A test calling `StopRaidAI` cannot compile before it exists, so the RED commit for Task 1 uses only existing symbols (it genuinely failed: both subtests timed out waiting for the worker), the `raidAIFires` assertions and the `settleAI` change went into the Task 1 GREEN commit, and the bounded/shed and panic subtests (which characterise code that already existed and so pass on arrival) were committed on their own as `2809f30`. The two drain subtests and `restoreRaidAIPool` arrived with the Task 2 GREEN commit. Every commit builds and vets.
2. **[Rule 1 - Bug, test only]** The first version of "bounded and shed" blocked forever: its `started` channel send blocked once its buffer filled, wedging every worker (10 min test timeout). Fixed with a non-blocking send before the commit. The first "drain cancels" run failed because an `httptest` handler only sees a client disconnect after it has read the request body; the handler now drains the body first.

**Total deviations:** 0 behavioural, 2 test-only fixes.

## Verification

- `CGO_ENABLED=1 go test -tags testtools -race -count=1 -run '^TestRaidAIPool$' -v ./alita/modules`: all six subtests PASS, including `bounded_and_shed`.
- Task 1 focused set (`TestRaidAIPool|TestRaidAIBurstFires|TestRaidAIAlert|TestRaidAIBurstSettings|TestRaidAISpamSharesVerdict|TestRaidAICandidateText`): PASS.
- Plan-level `-run 'AISpam|Raid|Lockdown|Unlockdown|Shutdown' ./alita/modules` with `-race`: ok (145 s).
- `CGO_ENABLED=0 go build ./...`: ok. `go vet -tags testtools ./alita/modules .`: clean. `gofmt -l alita/modules main.go`: prints nothing. `git diff --exit-code go.mod go.sum`: unchanged.
- `main.go` changed: binary built to the scratchpad and `--version` run from `/tmp` printed `v2.23.14`.
- Acceptance greps: `func raidAIFireAsync(` 1, `raidAIFireAsync(` 2, `RecoverFromPanic` in `raid_ai_count.go` 1, `raidAIFires` in AGENTS.md 1+, `func StopRaidAI(` 1, `func raidAIContext(` 1, `func RegisterRaidAIDrain(` 1, `raidAIStopWait+shutdownDrainGrace` 1, `StopRaidAI` in AGENTS.md 3, main.go line order 221 < 226 < 230.
- `make lint` was not run: golangci-lint is built with Go 1.25 against the module's Go 1.26. `go vet` and `gofmt` stand in for it.
- The orchestrator runs the full `make test` after the wave.

## Pending human verification (not runnable here)

- Send SIGTERM to a running bot (webhook mode) with one AI check hung and confirm the log order "Draining AI spam checks", "Draining raid AI checks", "Draining raid detection posts", "Closing database connections", and that the whole shutdown stays inside 60 s.
- With a live TypeSafe key and Redis, raise a real AI burst while the Telegram API is slow and confirm the `/aispam` deletions of later messages are not delayed by the lockdown start.

## Known Stubs

None.

## Threat Flags

None. The plan's threats are mitigated as registered: T-07-24 (drain cancels first, waits 1 s, AGENTS.md records the worst case), T-07-46 (start on a tracked goroutine, two tests), T-07-47 (every worker and job under `RecoverFromPanic`, panic test).

## Self-Check: PASSED

- Files exist: `alita/modules/raid_ai_pool_test.go`, `alita/modules/raid_ai.go`, `alita/modules/raid_ai_count.go`, `alita/modules/shutdown_drains.go`, `main.go`.
- Commits `1575625`, `d336df7`, `2809f30`, `d2ea39c` exist on the branch.
