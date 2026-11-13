---
phase: 15-ai-spam-name-check
plan: 10
subsystem: member-requirements
tags: [member-requirements, ai-spam-name, shutdown, join-budget, log-sweep, request-minimality, independence]

requires:
  - phase: 15-ai-spam-name-check
    provides: 15-04 to 15-08 (the AI rule, its cost guards, the notice, the button, the page and its gate), 15-09 (the spike tool)
provides:
  - "TestShutdownEndsMemberReqAI: a job inside a hung provider call is ended by StopMemberReqJobs inside its wait, with no new drain"
  - "TestMemberReqAIJoinBudget: the join returns at its lookup budget while the flight finishes and caches"
  - "TestMemberReqAIRequestIsMinimal: the request is the three canonical names, the instruction and the fixed shape"
  - "TestMemberReqAIPacersAreIndependent: nothing the AI call shares can starve it or be starved by it"
  - "TestMemberReqLookupLogsCarryIDsOnly extended with the AI shapes on four paths, a transport error and a recovered panic; TestMemberReqLookupAlertsFitEveryLocale extended with the refused AI press"
  - "toolkit: mreqAIUnique, mreqAINameSentinel, mreqAIStaticForbidden, mreqAIForbiddenOf, mreqAISentinelMember and the four mreqAILog* path helpers"
  - "AGENTS.md: the shutdown proof in the member requirements drain bullet and the Phase 15 test-family paragraph"
affects: [15-11]

plan_head_before: 9c09f0432a8457ca05378ec61cf1ddb55f662ace
plan_head_after: b3638fa51c97c5ee827183e27317c0f869d03113
actuals:
  tokens: 21000
  tasks: 3
  commits: 4

tech-stack:
  added: []
  patterns:
    - "A sweep names members with a sentinel plus a counter and never the member's ID, so an ID and a name can each be looked for in a body or a log line"
    - "The log sweep reuses the unanswerable-shape tables of plan 15-05/15-06 (mreqAIShapes, mreqAIStateShapes) and runs each shape on the job, join, held-release and button paths"

key-files:
  created:
    - fuku/modules/memberreq_ai_sweeps_test.go
  modified:
    - fuku/modules/shutdown_drains_test.go
    - fuku/modules/memberreq_lookup_budget_test.go
    - fuku/modules/memberreq_ai_fake_test.go
    - AGENTS.md

key-decisions:
  - "No production file changed: every guarantee held on the code of plans 15-04 to 15-09 (no fix commit)"
  - "The log sweep cannot catch an Anthropic key leak on its own, because the global logredact hook (installed by fuku/config init, before a test hook) already redacts sk-ant-... in a line; the names and hashes have no such net and are what the sweep holds. Said in AGENTS.md"

requirements-completed: []

coverage:
  - id: D1
    description: "A job inside a hung provider call when StopMemberReqJobs runs is ended inside memberReqStopWait, leaves no hold, warn row, write, notice or cached verdict, is not a breaker failure, a later message starts no job, and a fresh flight of the same n: key and the gate slot are free"
    requirement: "PLAT-06 (cross-cutting, inherited, already ticked)"
    verification:
      - kind: unit
        ref: "TestShutdownEndsMemberReqAI (-race, count=10)"
        status: pass
    human_judgment: false
  - id: D2
    description: "The join returns at the join lookup budget with the member welcomed, unjudged and marked while the slower provider call finishes and caches; the first message judges from the cache with one request in all; a photo lookup that spends the budget leaves the AI row unasked; a hung job never delays the update"
    requirement: "PLAT-06"
    verification:
      - kind: unit
        ref: "TestMemberReqAIJoinBudget (three subtests, -race, count=10)"
        status: pass
    human_judgment: false
  - id: D3
    description: "No AI log line holds a name, names hash, verdict key, probability, instruction fragment or key under any failure shape on the job, join, held release and button paths, nor under a transport error or a recovered flight panic; each expected provider status or error type appears"
    requirement: "MREQ-14 (contribution only; completes none)"
    verification:
      - kind: unit
        ref: "TestMemberReqLookupLogsCarryIDsOnly (new subtests; verified by mutation: a log line that prints the first name fails it)"
        status: pass
    human_judgment: false
  - id: D4
    description: "The request body holds only the fixed field set, the forced tool, the instruction as system text and exactly the three canonical names; no title, message text, chat ID, user ID or key in the body; the key in x-api-key"
    requirement: "MREQ-14 (contribution only)"
    verification:
      - kind: unit
        ref: "TestMemberReqAIRequestIsMinimal"
        status: pass
    human_judgment: false
  - id: D5
    description: "A 30 s block on the lookup pacer does not touch an AI judgement, a burst writes no pacer key, a saturated gate delays no name-rule punishment or notice, and the refused AI press alert fits 200 code points in 8 languages and names no ID, Staff Group or member"
    requirement: "MREQ-10 (contribution only), PLAT-06"
    verification:
      - kind: unit
        ref: "TestMemberReqAIPacersAreIndependent, TestMemberReqLookupAlertsFitEveryLocale"
        status: pass
    human_judgment: false

duration: about 50 min (not timed at start; estimated from the session)
completed: 2026-11-09
status: complete
---

# Phase 15 Plan 10: the cross-cutting sweeps of the AI rule Summary

**The shutdown, join-budget, request-minimality, log and independence guarantees of the AI spam-name rule are each proved by a test, with no production change: a hung provider call is ended by the existing drain, a join never waits past its 4 s lookup budget, no name or hash reaches a log, the request is exactly the three names and the instruction, and nothing the call shares can starve it.**

## Accomplishments

- **Shutdown (`TestShutdownEndsMemberReqAI`, `shutdown_drains_test.go`).** A twin of `TestShutdownEndsMemberReqLookups` for a job held inside the stand-in's hung request: `StopMemberReqJobs` returns true inside `memberReqStopWait`; the job made no restrict, ban, kick or send call and left no hold, warn row or verdict key; the breaker holds no failure for the group (a shutdown is not a provider fault); a late job is refused and the stand-in saw one request; a fresh flight of the same `n:<verdict key>` is not joined and a gate of size 1 serves a new acquirer, both asked again until the unwinding goroutine has finished. No drain, pool or queue exists for the AI call, so the AGENTS.md 65 s worst case is unchanged (`git diff 62ee0d1 -- main.go fuku/utils/shutdown` is empty).
- **Join budget (`TestMemberReqAIJoinBudget`).** With the join budget at 200 ms and the provider answering after 1.5 s the join returns before the answer (no verdict key yet), the member is welcomed, nothing is punished or cached and the marker is set; the flight then caches its verdict by itself and the member's first message judges from the cache (one request in all) and mutes. A photo lookup held past the budget leaves the AI row unasked (zero requests, welcomed, marked). A message on a hung call returns from the dispatcher at once (the job runs on its own goroutine) and a released hang is unknown and punishes nobody. The assertions check the order of events, not tight times.
- **Request minimality (`TestMemberReqAIRequestIsMinimal`).** A message job in a group with a sentinel title, by a member with sentinel names and a sentinel message: the body holds none of the title, message, chat ID (with or without the minus), user ID, default group title or key; its top-level fields are exactly `model max_tokens system thinking tools tool_choice messages`; `max_tokens` 128, thinking disabled, the forced tool, one tool, the system text equal to `claude.NamesInstruction`, one user message whose JSON has exactly the three canonical strings; the key is in `x-api-key` only.
- **Log sweep (`TestMemberReqLookupLogsCarryIDsOnly`, extended in place; the existing subtests and `mreqLogForbidden` are unedited).** Every shape of `mreqAIShapes` (provider answers: 400, 401, 403, two 429s, 500, refusal, end_turn, missing tool call, probability out of range both ways, null and truncated bodies, dropped connection, a request that outlives the budget, the shutdown root cancelled) and `mreqAIStateShapes` (paused group, paused provider, three exhausted budgets, a Redis budget error, a full gate, a recovered panic in the call) runs on four paths (job, join, held release, button after a rename), a transport error runs through `memberReqAIHTTP` with the failing transport, and a recovered panic is one `memberReqFlight` line naming no chat or user ID. The forbidden list is the Phase 14 list plus the sentinel and ordinary names, the key, the instruction's opening words, the probabilities the stand-in answers, the three key families and, per member, the names hash and verdict key. `mustSee` demands `status=401/429/500`, the three error types, the refusal line, the generic failure line, the pause line, "is used up" and "could not be counted", so a shape that logged nothing fails.
- **Independence (`TestMemberReqAIPacersAreIndependent`).** A 30 s block on the lookup pacer's keys leaves an AI judgement (executor, Mute, 0.97) alone: one call, muted, nothing written to `lkp:pace:next`, the block untouched. Ten distinct strings write no key under `fuku:memberreq:pace:` or `fuku:memberreq:lkp:pace:`. With a one-slot gate held by one call and a second caller waiting, a last-name Mute and its notice for another member land well inside the 1.5 s gate wait, and the waiting caller is unknown with no extra request.
- **Alert size.** The "refused AI press" entry (`settingsMreqUnconfiguredText` for the AI row: `settings_mreq_here_ai` with the translated `settings_mreq_rule_ai`, no icon) joined `TestMemberReqLookupAlertsFitEveryLocale`: at most 200 code points in all 8 languages, no chat or channel ID, no Staff Group, no member, and it names the rule.
- **AGENTS.md.** The member requirements drain bullet now names `TestShutdownEndsMemberReqAI` and what it proves; the Testing section has the "Phase 15 (the AI spam-name rule) test families" paragraph: the toolkit, hygiene and every family by name (the 15-04 to 15-09 families verified to exist, the plan's draft list extended with `TestMemberReqAINotConfigured`, `TestMemberReqAISparesExempt` and `TestMemberReqAILookupOrder`), and `TestMemberReqAIEndToEnd` named as 15-11's.

## Task Commits

1. `5fcb177` test(15-10): the AI call ends at shutdown and never parks a join
2. `ad579b5` test(15-10): no name, hash or key in any AI log line, and only the names in the request
3. `8f10940` test(15-10): nothing the AI call shares can starve it, and the refused press fits 200
4. `b3638fa` docs(15-10): list the Phase 15 test families and the shutdown proof

`commits: 4` is `git rev-list --count 9c09f04..HEAD` before this SUMMARY's own commit (`plan_head_before` above).

## Verification (real results)

- `go test -tags testtools -race -count=1 -run '^(TestShutdownEndsMemberReqAI|TestMemberReqAIJoinBudget)$' -v ./fuku/modules`: PASS (3 subtests and the shutdown test), no FAIL or SKIP. `-count=10` of the same: `ok` (36 s), exit 0.
- `go test ... -run '^(TestMemberReqLookupLogsCarryIDsOnly|TestMemberReqAIRequestIsMinimal)$' -v`: PASS, exit 0 (about 58 s).
- Task 3's full focused run, `-run '^(TestMemberReqAI|TestMemberReqLookupAlertsFitEveryLocale|TestMemberReqLookupLogsCarryIDsOnly|TestMemberReqPacersAreIndependent|TestShutdownEndsMemberReq|TestShutdownWaitsForMemberReqJobs)' ./fuku/modules`: `ok` 140 s, exit 0. The plan's verification line `-run '^(TestMemberReqAI|TestShutdown|TestMemberReqLookup)'` (the `TestShutdown` and `TestMemberReqLookup` parts): `ok` 101 s, exit 0.
- `go vet -tags testtools ./fuku/modules`: exit 0. `gofmt -l` on the files I touched: nothing (the seven other files `gofmt -l ./fuku/modules` lists were unformatted before this plan and are not mine). `CGO_ENABLED=0 go build ./...`: exit 0.
- `python3 .planning/phases/13-member-requirements/handoff/lint-local.py 8c0c827`, exit read directly: 0 ("no dupl, gocyclo or godox findings on lines added since 8c0c827"), after the last code commit and again with the AGENTS.md change in the tree.
- Mutation checks that the sweeps are not vacuous (the production file was put back each time; `git status` shows `memberreq_ai.go` unchanged): a `log.Debugf("judging %s", names.First)` before the provider call fails the shapes subtest (job, button paths) and the panic subtest with `a log line carries "ZZAISentinel"`, `"Promo"` and `"Maria"`. A mutation that appended the API key to the failure line did NOT fail the sweep, because the `logredact` hook already redacts it (see decisions); that is recorded in AGENTS.md rather than hidden.
- Invariants: `git diff --exit-code 62ee0d1 -- main.go fuku/utils/shutdown` exit 0; `git diff --exit-code 8c0c827 -- go.mod go.sum` exit 0; `grep -c 'func TestShutdownEndsMemberReqAI' shutdown_drains_test.go` 1, `grep -c 'func TestMemberReqAIJoinBudget'` 1, `grep -c 'func TestMemberReqAIRequestIsMinimal'` 1, `grep -c memberReqAICall memberreq_lookup_budget_test.go` 1, `grep -c 'Skip(\|Parallel()' memberreq_ai_sweeps_test.go` 0, `grep -c TestShutdownEndsMemberReqAI AGENTS.md` 2, `grep -c mreqAIUnique AGENTS.md` 1.
- Not run: `make lint` and gosec (cannot run in this sandbox; the local stand-in covered `dupl`, `gocyclo`, `godox`, `staticcheck` and `errcheck` on added lines), `make test` and the whole `fuku/modules` package (15-11 runs it), PostgreSQL (nothing here rests on it; the module harness runs on SQLite), the settings family `-run '^(TestSettings|TestLoadSettings)'` (no settings code or test changed; only the shared alert test file did, which was run).

## Deviations from Plan

1. **[Judgement] No fix commit and no production change.** Every guarantee held on the first run of its test. Each timing-sensitive test was run `-count=10`.
2. **[Judgement] The plan's "AI forbidden list" is `mreqAIStaticForbidden` plus the per-member `mreqAIForbiddenOf`**, and the plan's "mreqAIUnique() (a counter)" is a string counter used with `mreqAINameSentinel`. The toolkit also gained `mreqAISentinelMember` and, in `memberreq_ai_sweeps_test.go`, the four path helpers `mreqAILogJob`, `mreqAILogJoin`, `mreqAILogHeld` and `mreqAILogButton` (each used by the extended log test, so none is unused).
3. **[Judgement] The log sweep reuses `mreqAIShapeEnv`**, which also mints a discarded `mreqAIMember`; the extra member is harmless and `mreqAIStaticForbidden` includes its name family (`Promo`, `Casino`, `Maria`) so nothing it could leak is missed.
4. **[Judgement] The button path applies the shape after the mute** (a member is muted with the default 0.97, renamed to names with no verdict, then the shape is installed and "I fixed it" pressed), and the shutdown shape stops the runner only after the mute.
5. **[Judgement] The tasks' commit boundaries were kept by hand.** The alert entry and the independence test were written before Task 2's commit and held back from it (the files were put back after the commit), so each commit has the content its subject names.
6. **[Judgement] The key-leak limit.** The plan expected the sweep to catch the API key; the global `logredact` hook redacts it first, so the sweep cannot. The key stays held by that hook plus `memberReqAILogFailure`'s own scrub, and AGENTS.md says so.
7. **[Judgement] The "timeout past the lookup budget" shape logs nothing by design** (a cancelled flight logs nothing: `memberReqAILogFailure` returns when the context ended); the sweep still runs it and checks the lines that exist. The AI independence test uses the executor for the lookup-block case and the rule called directly for the burst and gate cases.

No unused identifier is left (the lint stand-in's `unused` check is clean).

## Known Stubs

None.

## Requirements

`requirements-completed: []`. This plan contributes to MREQ-14, MREQ-10 and PLAT-06; PLAT-06 stays ticked (the AI part of it is proved here), MREQ-01, MREQ-10 and MREQ-14 stay Pending until plan 15-11. REQUIREMENTS.md is unchanged.

## Threat Flags

None. No endpoint, auth path, file access or schema; this plan adds tests and one AGENTS.md paragraph. T-15-28 (a name or hash in a log) is held by the sweep (mutation-checked for names), T-15-29 by `TestShutdownEndsMemberReqAI`, T-15-30 by `TestMemberReqAIJoinBudget`, and T-15-SC by the unchanged `go.mod` and `go.sum`.

## Self-Check: PASSED

Created file `fuku/modules/memberreq_ai_sweeps_test.go` exists, and commits `5fcb177`, `ad579b5`, `8f10940` and `b3638fa` are ancestors of HEAD.
