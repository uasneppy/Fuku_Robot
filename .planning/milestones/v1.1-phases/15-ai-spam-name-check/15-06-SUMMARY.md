---
phase: 15-ai-spam-name-check
plan: 06
subsystem: member-requirements
tags: [member-requirements, ai-spam-name, budgets, gate, breaker, sticky-unknown, stale-names]

requires:
  - phase: 15-ai-spam-name-check
    provides: 15-04 (the AI rule tracer, verdict cache nv, the stand-in), 15-05 (baseline nm, marker nu, Joining, the watcher's AI read, the held release with names)
provides:
  - the three hourly budgets (5 per account fleet-wide, 100 per group, 400 in all) in fuku:memberreq:aib:*, reserved through memberReqAIReserve over the stateless raidAIBudgetTake
  - the 4-slot gate (semaphore.NewWeighted(4), a 3 s bounded wait, released by defer) taken before any budget unit
  - memberReqAIBreaker, the name check's own type-based breaker (per-group failures and pause, provider-wide pause, bounded map)
  - memberReqAIAnswer{Verdict, Sticky} as the flight value; the sticky unknown plumbing (memberReqLookupBitKey, memberReqNoteSticky, memberReqLookupTouch.sticky) and the executor's clearing of sticky bits before memberReqUnknownDefers (D-22)
  - the stale-names guard (memberReqRule.NamesBound, memberReqNamesBoundMask, memberReqAINamesHash, the judged hash passed to memberReqRereadAfterLookups, D-29)
  - the seams memberReqAITake and memberReqAICall, in-memory counters memberReqAIStatsSnapshot
affects: [15-07, 15-08, 15-10, 15-11]

plan_head_before: eca8e16e4d5bbb7f6cde4f3990ab85085f21f614
plan_head_after: 1c9486442e7e18c01b23480aba4c78f5e10ab41a
actuals:
  tokens: 20900
  tasks: 3
  commits: 7

tech-stack:
  added: []
  patterns:
    - "A flight value that carries a state flag beside the verdict (memberReqAIAnswer{Verdict, Sticky}); the caller notes the flag into the run's touch through a context key stamped by memberReqLookupMasks, so the executor never names a rule"
    - "The guards are three small functions (memberReqAIAcquire: breaker then gate; memberReqAIReserveUnit: budgets; memberReqAIAfterFailure: settle) so memberReqAIJudge stays under the gocyclo limit and the gate release is a plain defer in the flight body"
    - "A breaker as a type with small methods (isPaused, admit, pruneIdle, countFailure, pauseProvider, clear) and a tiny outcome struct, logged by the package-level wrappers outside the lock"

key-files:
  created:
    - fuku/modules/memberreq_ai_budget.go
    - fuku/modules/memberreq_ai_breaker.go
    - fuku/modules/memberreq_ai_guards_test.go
  modified:
    - fuku/modules/memberreq_ai.go
    - fuku/modules/memberreq_lookup.go
    - fuku/modules/memberreq_rules.go
    - fuku/modules/memberreq_enforce.go
    - fuku/modules/memberreq_ai_fake_test.go
    - fuku/modules/memberreq_ai_test.go
    - AGENTS.md

key-decisions:
  - "The breaker check sits in memberReqAIAcquire together with the gate (breaker first), so the flight body reads: cache, acquire (breaker, gate), reserve unit (budgets), call, settle; `defer memberReqAIGate.Release(1)` is a literal defer in the flight body, after a successful acquire"
  - "withAIConfigured installs a fresh breaker for every AI test (the breaker is process-wide state, and a 401 shape pauses every group for 5 minutes), so no test can pause the ones after it; the instance is swapped with mreqSwap"
  - "memberReqAINamesHash returns the empty string for names that are not judgeable, so a change from judgeable to not judgeable (and back) drops the names-bound bit, while a zero-width character does not"

requirements-completed: []

coverage:
  - id: D1
    description: "the provider is never called without a reserved unit: account, then group, then fleet, a refusal stops the chain, cached verdicts spend none, a counter error is unknown, new counters each UTC hour, exact keys with a 2 hour expiry, image keys untouched"
    requirement: "MREQ-14 (contribution only; completes none)"
    verification:
      - kind: integration
        ref: "go test -tags testtools -race -count=1 -run '^TestMemberReqAIBudget$' ./fuku/modules"
        status: pass
    human_judgment: false
  - id: D2
    description: "at most four (here two or one) calls at once; no slot within the wait is a sticky unknown spending nothing; a cancel returns at once; a panicking call is recovered and releases the slot; 100 callers of one string make one call"
    requirement: "MREQ-14 (contribution only)"
    verification:
      - kind: integration
        ref: "TestMemberReqAIGate, TestMemberReqAIFlight (also -count=20)"
        status: pass
    human_judgment: false
  - id: D3
    description: "the name check's own breaker: five failures pause a group, a rejected key or a long 429 pauses every group, answers and cancels, the bound, one warning per pause, no sharing with the image breaker"
    requirement: "MREQ-10 (contribution only; completes none)"
    verification:
      - kind: integration
        ref: "TestMemberReqAIBreaker"
        status: pass
    human_judgment: false
  - id: D4
    description: "a sticky unknown never defers a weaker punishment (both directions, 35 combinations); the verdict stays unknown for the lookup fold and the held release; eight more unanswerable shapes leave the member alone and recover"
    requirement: "MREQ-10 (contribution only)"
    verification:
      - kind: integration
        ref: "TestMemberReqAIUnknownDefers, TestMemberReqAIUnanswerable (mutation check: removing the sticky clear fails 9 subtests)"
        status: pass
    human_judgment: false
  - id: D5
    description: "a member who renamed during the call is not punished for the old string; the verdict cache stores a decimal string with a 24 h expiry, treats bad values (NaN, +Inf, Inf, -Inf included) as a miss in the rule's and the watcher's read, never stores an unknown"
    requirement: "MREQ-14 (contribution only)"
    verification:
      - kind: integration
        ref: "TestMemberReqAINamesChangedDuringLookup, TestMemberReqAIVerdictCache"
        status: pass
    human_judgment: false

duration: about 1h (not timed at start; estimated from the session)
completed: 2026-11-09
status: complete
---

# Phase 15 Plan 06: cost guards, breaker, sticky unknown and stale-names guard Summary

**The AI name check now costs a bounded amount and fails quietly: every provider call reserves an account, group and fleet hourly unit (5, 100, 400) after taking one of four per-replica gate slots, a failing provider is backed off by the name check's own breaker, an outage or an empty hour can no longer keep a weaker name punishment from standing, and a member who renames during the call is never punished for the old string.**

## Accomplishments

- **Budgets (`memberreq_ai_budget.go`).** `memberReqAIReserve` takes the account's unit (`fuku:memberreq:aib:u:<user>:<yyyymmddhh>`, fleet-wide), then the group's (`aib:c:<chat>:`), then the fleet's (`aib:g:`), each through the seam `memberReqAITake` (default `raidAIBudgetTake`: `INCR` plus a 2 hour expiry in one transaction) on the UTC hour of `raidNow`. A refusal or an error stops the chain and is a three-way result (reserved, capped, fault), so the caller can tell a state from a fault. The three limits are package variables only for tests. Counters (`memberReqAIStatsSnapshot`: checked, passed, breached, cached, capped, shed, failed, blocked) are in memory.
- **Gate and flight (`memberreq_ai.go`).** `memberReqAIGate` is `semaphore.NewWeighted(4)` with a 3 s wait. The flight's value is `memberReqAIAnswer{Verdict, Sticky}`. `memberReqAIJudge`: cache re-read, `memberReqAIAcquire` (breaker, then gate), `defer memberReqAIGate.Release(1)`, `memberReqAIReserveUnit`, the call through the seam `memberReqAICall`, `memberReqAIAfterFailure` (a cancel records nothing, a refusal stop is a success, anything else a failure), the cache write. A paused breaker, a gate that could not be had on a live context and a used-up budget are a sticky unknown; a counter fault, no Redis and a cancel are a plain unknown. No pool, queue, goroutine set or drain was added.
- **Breaker (`memberreq_ai_breaker.go`).** `memberReqAIBreaker` with its own state: five failures pause a group for 5 minutes; 401, 402, 403 (`KeyRejected`) pause every group for 5 minutes; a 429 with a retry delay over 2 s pauses every group for that delay, at most 5 minutes; a pause that is running is not extended or logged again; a bounded map of 10,000 groups with idle pruning and one warning per breaker when full; each pause is one warning naming status and error type only.
- **Sticky unknown (D-22).** `memberReqLookupMasks` stamps the row's bit into the context of its `Lookup`; `memberReqNoteSticky` ORs it into `memberReqLookupTouch.sticky`; `memberReqLiveViolation` clears `touch.sticky` from the unknown mask before `memberReqUnknownDefers`. The verdict itself stays unknown for the held release and "I fixed it" (no touch, no-op).
- **Stale names (D-29).** `memberReqRule.NamesBound` (AI row only), `memberReqNamesBoundMask()`, `memberReqAINamesHash(names)` (the canonical hash, empty when not judgeable); `memberReqLiveViolation` computes the judged hash before the lookups and `memberReqRereadAfterLookups` drops the names-bound bits when the re-read record's hash differs.
- **AGENTS.md.** A Data bullet for the three `aib` key families; the AI rule bullet gains the guards paragraph (order, gate, breaker, sticky, stale names, counters); the executor bullet's WR-02 sentence gains the sticky exception and its WR-03 sentence the names-changed rule.

## Task Commits

1. `4ed5679` test(15-06): budgets, gate and one call for 100 callers (red: names symbols the next commit adds)
2. `deba4e3` feat(15-06): hourly budgets and a four-call gate for the name check
3. `3515a01` test(15-06): a breaker for the name check and a sticky unknown (red: same reason)
4. `4494248` feat(15-06): pause the name check when the provider fails and never defer on a sticky unknown
5. `ca5823f` test(15-06): never punish a changed name, and the cache keeps to its rules (`TestMemberReqAINamesChangedDuringLookup` red at this commit; `TestMemberReqAIVerdictCache` already passed, because plan 15-04's single parse function `memberReqAIParseProbability` already holds the NaN and infinity guard)
6. `30960d4` fix(15-06): never punish a member for a name they changed during the AI call
7. `1c94864` docs(15-06): describe the budgets, gate, breaker, sticky unknown and stale-names guard

`commits: 7` is `git rev-list --count eca8e16..HEAD` at SUMMARY time (the base is the persisted ledger; the first test commit was made before the ledger file was written, so it holds the parent of that commit, which is the same value).

## Verification (real results)

- Task 1: `go test -tags testtools -race -count=1 -run '^(TestMemberReqAIBudget|TestMemberReqAIGate|TestMemberReqAIFlight)$' -v ./fuku/modules`: exit 0, every subtest PASS, no FAIL or SKIP. `-count=20 -run '^(TestMemberReqAIGate|TestMemberReqAIFlight)$'`: ok (63.3 s). Greps: `raidAIBudgetTake` in budget file 2, `fuku:memberreq:aib:` 2, `semaphore.NewWeighted` 1, `defer memberReqAIGate.Release` 1. `git diff --exit-code 8c0c827 -- fuku/modules/raid_ai_image.go fuku/modules/raid_ai_breaker.go`: exit 0.
- Task 2: `-run '^(TestMemberReqAIBreaker|TestMemberReqAIUnknownDefers|TestMemberReqAIUnanswerable)$'`: ok (36.0 s). `-run '^(TestRaidAIBreaker|TestRaidAIImageBreakerCap|TestMemberReqLookupDoesNotBlockNameBreach|TestMemberReqLookupUnanswerable|TestMemberReqLookupOrder)$'`: ok (27.8 s), unedited. Mutation check (reverted, file restored): commenting out the `unknownMask &^= int(touch.sticky.Load())` line makes the 9 sticky rows of `TestMemberReqAIUnknownDefers` with a stricter AI rule fail (three rows times the paused breaker, used-up budget and full gate; I read the first seven failures of the run) (the paused breaker, used-up budget and full gate rows with a stricter AI rule). Greps: `type memberReqAIBreaker struct` 1, `sticky atomic.Int64` 1, `memberReqLookupBitKey` in rules 1. `git diff --exit-code 8c0c827 -- fuku/modules/raid_ai_breaker.go fuku/modules/raid_ai_breaker_test.go`: exit 0.
- Task 3: `-run '^(TestMemberReqAINamesChangedDuringLookup|TestMemberReqAIVerdictCache)$'`: ok. `-run '^(TestMemberReqAI|TestMemberReqLookup|TestMemberReqFixed|TestMemberReqHeldRelease|TestMemberReqEnforce|TestMemberReqJoin)'`: exit 0 (ok, 182.4 s). The plan's verification list `-run '^(TestRaidAIBreaker|TestRaidAIImageBreakerCap|TestMemberReqOnePunishmentAcrossReplicas|TestMemberReqLookupOnePunishmentAcrossReplicas|TestMemberReqLookupDoesNotBlockNameBreach|TestMemberReqLookupCapAndPause|TestShutdownEndsMemberReqLookups)$'`: ok (8.2 s). A wider `-run '^(TestMemberReq|TestShutdown|TestApplyWarn)'`: exit 0 (ok, 296.4 s). Greps: `NamesBound` in rules 7 (at least 2), `fuku:memberreq:aib` in AGENTS.md 1 line (the Data bullet), `memberReqNamesBoundMask` in enforce 1.
- `CGO_ENABLED=0 go build ./...`: exit 0. `go vet -tags testtools ./fuku/modules`: exit 0. `gofmt -l` on the touched files: empty. `python3 .planning/phases/13-member-requirements/handoff/lint-local.py 8c0c827` read directly: exit 0 ("no dupl, gocyclo or godox findings"); its first run flagged `memberReqNamesBoundMask` as unused (U1000) after Task 2, so the function and its table field use moved to Task 3's commit, where the executor calls it. `test -z "$(gocyclo -over 19 memberreq_ai.go memberreq_ai_budget.go memberreq_ai_breaker.go memberreq_lookup.go memberreq_rules.go memberreq_enforce.go)"`: exit 0. `git diff --exit-code 8c0c827 -- go.mod go.sum`: exit 0.
- Not run: `make lint` and gosec (cannot run here), `make test` (the closing plan 15-11), the settings test family (this plan changes no settings code; the rule table gained one field only), PostgreSQL (nothing here rests on it; the harness runs on SQLite), "I fixed it" with a sticky unknown driven end to end (it reads `memberReqLookupBroken` with no touch, which `TestMemberReqAIUnknownDefers` proves returns unknown for a used-up budget; the button's own tests are unchanged and pass).

## Tests that changed

`TestMemberReqAIUnanswerable` (plan 15-04): `mreqAIShapeEnv` runs a `setup` for the new state shapes, and the second sighting installs a fresh breaker and runs the shape's `cure` first, because a 401, 403 or long-429 shape now pauses every group for 5 minutes and "a following request after the cause ends works" needs that cause ended. Every assertion it had is kept; eight state shapes are appended. `withAIConfigured` (plan 15-04's helper) installs a fresh breaker. No other test file was edited.

## Deviations from Plan

1. **[Plan-required] D-26's "a copy of the image breaker" became a type-based breaker with its own state.** `memberReqAIBreaker` has small methods (`isPaused`, `admit`, `pruneIdle`, `countFailure`, `pauseProvider`, `clear`) instead of package-level functions over a global struct, so `dupl` finds no clone of `raid_ai_breaker.go`. The generic-helper fallback was NOT needed: the lint stand-in reported no dupl finding on the first run, so `raid_ai_breaker.go` and `raid_ai_breaker_test.go` are untouched (the existing `TestRaidAIBreaker` and `TestRaidAIImageBreakerCap` pass unedited).
2. **[Judgement] The breaker check is inside `memberReqAIAcquire` (beside the gate)** rather than a separate step; the order is unchanged (breaker, gate, budget).
3. **[Judgement] `memberReqNamesBoundMask` and the `NamesBound` use moved from Task 2 to Task 3's commit** (the plan lists them under Task 3's files) after the lint stand-in flagged the function unused at the Task 2 commit.
4. **[Judgement] Test-commit order.** The Task 3 test commit holds only the test file and was red for `TestMemberReqAINamesChangedDuringLookup` at that commit; the production fix is the next commit.
5. **Own mistakes, fixed before commit:** a row of the rule table was edited with the wrong indentation in a script, so `NamesBound: true` was first absent and the stale-names tests stayed red until the sed fix; `mreqAIUsers` empties the verdict cache (through `mreqAICleanup`), so the test that needs a cached verdict mints its second user before the first judgement.

No unused identifier is left: the seams `memberReqAITake` and `memberReqAICall` are swapped by tests of this plan, `memberReqAIStatsSnapshot` is read by `TestMemberReqAIBudget` and `TestMemberReqAIBreaker`, and every test helper is read by a test.

## Carry-over note for the orchestrator's final list

D-26's "a copy of the image breaker" was built as a type-based breaker with its own state (`memberReqAIBreaker`) to avoid a `dupl` clone of `raid_ai_breaker.go`; the generic-helper fallback was not needed and no existing breaker file changed.

## Requirements

`requirements-completed: []`. MREQ-10 and MREQ-14 (and MREQ-01) stay Pending and complete in plan 15-11. REQUIREMENTS.md is unchanged.

## Known Stubs

None. Left for later plans: the remedy sentence and "I fixed it" with the AI rule (15-07), the page status and Off-only form (15-08), the log and locale sweeps (15-10) and the end-to-end test (15-11). The counters have no `/stats` section (D-32 marks it optional).

## Threat Flags

None new. T-15-13 (cost by renaming or mass joining: `TestMemberReqAIBudget`, `TestMemberReqAIGate`, `TestMemberReqAIFlight`), T-15-14 (an outage or exhausted hour suspending the weaker punishments: `TestMemberReqAIUnknownDefers`, both directions, mutation-checked), T-15-15 (a member punished for a name already changed: `TestMemberReqAINamesChangedDuringLookup`) and T-15-16 (a panic, a cancel or a leaked slot: `TestMemberReqAIGate`) are mitigated. T-15-SC: `go.mod` and `go.sum` unchanged. Recorded for the owner: an attacker who spends the fleet's 400 hourly units makes every group's AI check a sticky unknown for the rest of the hour (a denial of the AI rule only; the other rules and weaker punishments are unaffected by design), which is the accepted cost of an abuse limit.

## Self-Check: PASSED

The three created files and this SUMMARY exist, and the commits 4ed5679, deba4e3, 3515a01, 4494248, ca5823f, 30960d4 and 1c94864 are ancestors of HEAD.
