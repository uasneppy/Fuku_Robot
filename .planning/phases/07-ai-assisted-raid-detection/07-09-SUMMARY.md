---
phase: 07-ai-assisted-raid-detection
plan: 09
subsystem: raid-detection
tags: [gemini, budgets, circuit-breaker, redis, singleflight, cost-guard]

requires:
  - phase: 07-ai-assisted-raid-detection
    provides: "image path raidAIProcessImage (07-08), pool and RaidAIStats (07-06), HTTPError.KeyRejected and RetryDelay (07-02), raidAIContext (07-14)"
provides:
  - "Hourly Gemini budgets: 150 per group and 500 in all, a capped image is no verdict (RaidAIStats.Capped)"
  - "Verdict cache by file_unique_id (1 h), album gate, fire-latch skip (RaidAIStats.Cached), one verdict step shared by overlapping posts of one image"
  - "Per-group Gemini breaker (5 failures, 5 min) and a provider-wide pause (rejected key, 429 over 2 s)"
affects: [07-10 stats display of Capped and Cached, later live checks]

actuals:
  tokens: 12400
  tasks: 2
  commits: 4
plan_head_before: 8a2009123517fe9f48e67221e879d9441036979d
plan_head_after: f230ab444c25dc2916d94811d9e4b320b3936602
commits: 4

tech-stack:
  added: []
  patterns:
    - "Guard order cheapest and most decisive first: album gate, fire latch, verdict cache, breakers, budgets, then download and call"
    - "INCR plus EXPIRE in one MULTI for an hourly counter; a Redis error is no verdict, never an unbounded call"
    - "singleflight keyed by file_unique_id so concurrent posts of one image make one call"

key-files:
  created:
    - alita/modules/raid_ai_breaker.go
    - alita/modules/raid_ai_budget_test.go
  modified:
    - alita/modules/raid_ai_image.go
    - alita/modules/raid_ai.go
    - alita/modules/raid_ai_breaker_test.go
    - alita/modules/raid_ai_media_test.go
    - alita/modules/raid_ai_text_test.go
    - AGENTS.md

key-decisions:
  - "Overlapping posts of one image share one verdict step through a singleflight keyed by file_unique_id; without it the cache alone leaves a window where a flood of one sticker makes one call per worker"
  - "The followers of a shared step count as Cached and share its failure too (a blocked or failed image is not retried for each post)"
  - "A safety block records a success for the breaker: the provider answered, so a block is not an outage"
  - "A Redis error on any guard is counted as Shed (not Capped, not Failed): it is no verdict"
  - "The chat breaker ignores failures while the chat is already paused, so checks in flight when it tripped do not pause it again"

requirements-completed: [RAID-04, RAID-05]

duration: 50min
completed: 2026-10-29
status: complete
---

# Phase 07 Plan 09: Cost guards and breakers for the Gemini image checks Summary

**An image flood is bounded at 150 Gemini calls per group and 500 in all per hour, one image (or album) is judged once, and a failing or key-rejecting Gemini pauses the image checks quietly instead of being called per message.**

## Accomplishments

- Before any download or Gemini call an image job passes the album gate (`alita:raid:{<chat>}:ag:<media_group_id>`, SET NX EX 120), the fire-latch skip (EXISTS on the chat's fire key), the verdict cache (`alita:raid:aiv:<file_unique_id>`, 1 h, verdicts only), the breakers, and the hourly budgets (`alita:raid:{<chat>}:ab:<yyyymmddhh>` and `alita:raid:aibudget:<yyyymmddhh>`, INCR with a 2 h expiry).
- Each refusal is no verdict and is counted: `RaidAIStats.Capped` (budget) and `RaidAIStats.Cached` (cache hit, album drop, latch skip); a paused check is `Shed`. A cached verdict is judged against the same bar and recorded like a fresh one but is not `Checked`.
- `raid_ai_breaker.go`: five Gemini failures pause one group's image checks for 5 minutes; a rejected key pauses every group for 5 minutes; a 429 with a retry delay over 2 s pauses every group for that delay (at most 5 minutes). One warning per pause, no notice anywhere. Download and getFile failures never count.
- AGENTS.md documents the key families, the guard order, the counters and the breakers in the same commits as the code.

## Task Commits

| Task | Commit | What |
|------|--------|------|
| 1 RED | 9eb5664 | `test(07-09)`: TestRaidAIBudget (fails on the old code), plus the names it needs to compile |
| 1 GREEN | 8e57936 | `feat(07-09)`: budgets, verdict cache, album gate, latch skip, shared verdict step, AGENTS.md |
| 2 RED | 5fa59f2 | `test(07-09)`: Gemini half of TestRaidAIBreaker (pause cases fail on the stub) |
| 2 GREEN | f230ab4 | `feat(07-09)`: per-group breaker, provider-wide pause, wiring, AGENTS.md |

## Verification

- `CGO_ENABLED=1 go test -tags testtools -race -count=1 -run '^(TestRaidAIBreaker|TestRaidAIBudget|TestRaidAIMedia|TestRaidAISecretsNeverLogged)$' ./alita/modules`: pass.
- `CGO_ENABLED=1 go test -tags testtools -race -count=1 -run 'AISpam|Raid|Lockdown' ./alita/modules`: pass (about 154 s).
- `CGO_ENABLED=0 go build ./...`, `go vet -tags testtools ./alita/modules ./alita/utils/gemini`, `gofmt -l alita`: clean. `git diff --exit-code go.mod go.sum`: unchanged (`golang.org/x/sync/singleflight` was already a dependency).
- `make lint` was NOT run: golangci-lint is built with Go 1.25 against the module's Go 1.26. `go vet` and `gofmt` stand in for it.
- Acceptance greps: `raidAIChatHourlyBudget` 2, `raidAIGlobalHourlyBudget` 2, `alita:raid:aiv:` 1, `alita:raid:aibudget:` 1 in `raid_ai_image.go`; both key families in AGENTS.md; `func raidAIGeminiPaused(` 1, `KeyRejected()` 1, `raidAIGeminiPaused(` in `raid_ai_image.go` 1; AGENTS.md matches provider-wide/pauses all chats 3 times.
- The full `make test` and the PostgreSQL-only tests were not run here (the orchestrator runs `make test` after the wave; PostgreSQL is never started in this environment). Nothing in this plan touches PostgreSQL.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Missing critical functionality] Overlapping posts of one image share one verdict step**
- **Found during:** Task 1 design
- **Issue:** The verdict cache alone leaves a window: four workers can pick up three posts of the same sticker before the first verdict is stored, so a flood of one image would still make one Gemini call per worker. That breaks the must-have "the same image is judged once an hour" under exactly the threat T-07-33 names.
- **Fix:** `raidAIImageFlight`, a `singleflight.Group` keyed by `file_unique_id`, around the verdict step (cache, breaker, budget, download, call). Followers reuse the answer and count as Cached.
- **Files modified:** alita/modules/raid_ai_image.go, AGENTS.md, test `the same image at the same moment is judged once`
- **Commit:** 8e57936

**2. [Rule 1 - Bug in tests] Two 07-08 media tests posted the same image twice**
- **Found during:** Task 1 verify (`a deprecated model warns once`, `the raid image path never acts`)
- **Issue:** Both sent `photoSizes()` (one `file_unique_id`) twice and expected two Gemini requests. That is the case the new cache exists for.
- **Fix:** they send two different images (`photoNamed`). The assertions are unchanged.
- **Files modified:** alita/modules/raid_ai_media_test.go
- **Commit:** 8e57936

**3. [Rule 3 - Blocking] Breaker state leaked between tests**
- **Found during:** Task 2 verify: `TestRaidAISecretsNeverLogged` failed when run after the breaker tests (a rejected-key pause outlived its test).
- **Fix:** `newRaidAIImageEnv` calls `resetRaidAIGeminiBreakers(t)`, which clears the breakers and the provider pause now and at cleanup.
- **Files modified:** alita/modules/raid_ai_media_test.go
- **Commit:** f230ab4

**4. [Process] RED commits carry compile scaffolding**
- The plan asks for a test-only RED commit. A test that names new package variables does not compile without them and would break every test in the package, so each RED commit also declares the names (budget variables, `Capped`/`Cached`, the breaker types with no-op functions). The tests fail on behavior, not on a compile error.

**Total deviations:** 3 auto-fixed (1 missing critical, 1 test bug, 1 blocking) plus 1 process note. **Impact:** none on scope; the singleflight is about 15 lines.

## Decisions and caveats

- A follower of a shared verdict step shares the leader's failure, block, cap or pause, even when the follower is in another chat whose own budget or breaker would have allowed a call. This keeps a failing image from being retried per post. Cross-replica overlap is not covered (the Redis cache covers only a finished verdict).
- A group's first album photo that is capped, paused or fails leaves the album unjudged: the gate was already taken, so the other photos are dropped. Accepted; an album is "judged once".
- The budget is reserved before the download, so a failed download or call still spends its unit (the conservative side stated by the plan).
- `Shed` now also counts a Redis error on a guard, in addition to its earlier meaning (full queue, draining, paused TypeSafe chat).

## Known Stubs

None.

## Threat Flags

None. The new Redis keys are server-side counters with TTLs; no new endpoint, auth path or trust boundary.

## Human verification still pending (cannot run here)

- A live run with a real `GEMINI_API_KEY`: post a few images from a fresh account and confirm `Capped`/`Cached` move and the pause warning reads sensibly when the key is revoked.
- PostgreSQL-only tests were not run; none of this plan's tests depend on PostgreSQL.

## Self-Check: PASSED

- Created files exist: raid_ai_breaker.go, raid_ai_budget_test.go.
- Commits 9eb5664, 8e57936, 5fa59f2, f230ab4 exist on the branch.
- All task acceptance criteria and the plan-level verification re-run and passing (see Verification).
