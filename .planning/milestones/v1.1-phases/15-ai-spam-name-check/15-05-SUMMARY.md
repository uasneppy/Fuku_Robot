---
phase: 15-ai-spam-name-check
plan: 05
subsystem: member-requirements
tags: [member-requirements, ai-spam-name, baseline, new-member-marker, watcher, held-release]

requires:
  - phase: 15-ai-spam-name-check
    provides: 15-04 (the eighth rule row, memberReqAIRule tracer, verdict cache nv, Anthropic stand-in)
provides:
  - the rule's full order (verdict first, baseline-only first sighting, joiner / marker / rename judged), split into a read, a decide and a record step
  - fuku:memberreq:nm:<user> (baseline, SET NX / overwritten by a pass) and fuku:memberreq:nu:<chat>:<user> (marker), both 30 days via memberReqAIStateTTL
  - memberReqLookupMode.Joining (set by memberReqLiveViolation for the join entry's run)
  - memberReqAILive, memberReqAIMarkJoin (called by the join entry before its Check-on-join branch)
  - memberReqLookupRules (Photo, Channel, AI, any()) replacing the two-value memberReqLookupRulesOn; memberReqAIWatchQueue and the pure memberReqAIWatchSatisfied
  - the held release passes the sender's names
affects: [15-06, 15-07, 15-08, 15-10, 15-11]

plan_head_before: 3dac5a1887f5e55a9fcf040b0cd82b4777e93f61
plan_head_after: 4dcfa2882f019d278f9ba3ea30d3665c3a0e83e3
actuals:
  tokens: 19100
  tasks: 3
  commits: 4

tech-stack:
  added: []
  patterns:
    - "The rule's caller records its own answer (pass: nm = h and DEL nu in one TxPipeline; breach: SET nu; unknown: nothing); the flight stays a shared call-and-cache step"
    - "The watcher's AI part is a zero-value-safe struct (memberReqAIWatchRead) whose satisfied() is true when nothing was queued, so memberReqLookupCached gains only two branches"

key-files:
  created:
    - fuku/modules/memberreq_ai_baseline_test.go
    - fuku/modules/memberreq_ai_watch_test.go
  modified:
    - fuku/modules/memberreq_ai.go
    - fuku/modules/memberreq_lookup.go
    - fuku/modules/memberreq_enforce.go
    - fuku/modules/memberreq_watch.go
    - fuku/modules/memberreq_join.go
    - fuku/modules/memberreq_held.go
    - fuku/modules/memberreq_ai_fake_test.go
    - fuku/modules/memberreq_ai_test.go
    - fuku/modules/memberreq_cna_test.go
    - fuku/modules/memberreq_watch_lookup_test.go
    - AGENTS.md

key-decisions:
  - "memberReqAIMarkJoin keeps the plan's (ctx, chatID, userID) signature and is a plain marker write; the live test sits in the join entry as one branch (memberReqAILive), the entry's complexity stays far under 19"
  - "mreqAIAsk (the direct-call helper of the tracer tests) now sets Joining itself and a new mreqAIAskAs takes a user ID and the mode exactly as given for the baseline tests, so the edit of TestMemberReqAIRule is one helper and not twenty call sites"
  - "memberReqAIWatchQueue returns the zero memberReqAIWatchRead for names that are not judgeable; satisfied() is true for it, which removes a result and an && from memberReqLookupCached (19 before, 18 after)"

requirements-completed: []

coverage:
  - id: D1
    description: "an established member's first sighting records a baseline and makes no call; a baseline equal to the names writes nothing; no Redis and a Redis error are unknown; a member with a marker is never baselined"
    requirement: "MREQ-14 (contribution only; completes none)"
    verification:
      - kind: integration
        ref: "TestMemberReqAIBaseline"
        status: pass
    human_judgment: false
  - id: D2
    description: "a joiner is judged at join or at the first message, a rename is judged again, Check on join off still judges at the first message, a flagged member is never laundered by an expired baseline"
    requirement: "MREQ-14, MREQ-10 (contribution only; completes none)"
    verification:
      - kind: integration
        ref: "TestMemberReqAIRename, TestMemberReqAINewMember, TestMemberReqAICheckOnJoinOff, TestMemberReqAIMarkerSurvivesLongHold (TestMemberReqAINewMember and TestMemberReqAICheckOnJoinOff also ran -count=10)"
        status: pass
    human_judgment: false
  - id: D3
    description: "the watcher reads three extra keys, writes no verdict/baseline/marker, calls nobody, returns ext.ContinueGroups on every path; the pure decision is table-driven; the recorded gap is pinned"
    requirement: "MREQ-14, MREQ-10 (contribution only; completes none)"
    verification:
      - kind: integration
        ref: "TestMemberReqWatcherAI, TestMemberReqPassCacheWithAI"
        status: pass
    human_judgment: false
  - id: D4
    description: "switching the rule on in a group of 40 established members makes no provider call and every member gets a baseline"
    requirement: "MREQ-14 (contribution only)"
    verification:
      - kind: integration
        ref: "TestMemberReqWatcherAIBurst"
        status: pass
    human_judgment: false
  - id: D5
    description: "a Warn hold carrying bit 128 is released after a rename; cached breach and the marker keep it; exempt members and other holds make no request"
    requirement: "MREQ-10 (contribution only)"
    verification:
      - kind: integration
        ref: "TestMemberReqHeldReleaseAI"
        status: pass
    human_judgment: false

duration: 70min
completed: 2026-11-08
status: complete
---

# Phase 15 Plan 05: the baseline, the marker and the watcher's AI read Summary

**An established member's first sighting is now recorded with no provider call, a member the bot saw join is judged at join or at their first message (never baselined), a rename is judged again, the watcher decides from three cached GETs without writing or calling anyone, and a Warn hold carrying the AI bit is released once the member renames.**

## Accomplishments

- **The rule's order (`memberreq_ai.go`).** `memberReqAIPrepare` (no names: unknown; not configured and not judgeable: inert; no Redis or any read error: unknown) reads `nv`, `nm` and `nu` in one pipelined round (`memberReqAIReadState`). `memberReqAIDecide` is research S-03's order: a readable verdict decides first (the cached breach is a breach even for a baseline-equal member); not joining + no marker + equal baseline passes with no write; not joining + no marker + no baseline is `baselineOnly` (`SET NX`, pass, no call); everything else (a joiner, a marker, a different baseline) goes to the flight. `memberReqAIRule` records the answer through `memberReqAIRecord`: pass is `nm = h` plus `DEL nu` in one `TxPipeline`, breach is `SET nu`, unknown writes nothing. Records and the marker write carry no name, hash or probability in any log line.
- **Joining.** `memberReqLookupMode.Joining`, set in `memberReqLiveViolation` as `c.Source == memberReqFromJoin`, so the join entry judges at once and a rejoiner's old baseline never stands in for a new membership.
- **The marker at join.** `memberReqJoinThenWelcome` calls `memberReqAIMarkJoin` after the punished-mute branch and before `if !settings.CheckOnJoin` while `memberReqAILive(settings)` holds (action not Off and configured).
- **The watcher.** `memberReqLookupRules{Photo, Channel, AI}` with `any()` replaces `memberReqLookupRulesOn`. `memberReqLookupCached` takes the names; the AI part is `memberReqAIWatchQueue` (queues three GETs on the existing pipeline, or returns the zero value for names that are not judgeable) and `memberReqAIWatchSatisfied` (pure). `memberReqLookupCached` is gocyclo 18 (19 would have been the CI limit, 16 was the plan's figure for the old value; measured 19 on my first version, 18 after folding the judgeable flag into the struct).
- **Held release.** `memberReqHeldLookupsClear` passes `Names: &names` from `c.User`; the release itself is unchanged, and only a hold carrying bit 128 asks the AI rule.
- **Tests.** Nine new tests in two files plus the toolkit (`mreqAIMarkNew`, `mreqAIBaselineOf`, `mreqAIMarkerOf`, `mreqAIAskAs`, `mreqAICleanup` deleting baselines). Mutation checks (reverted, files restored): passing no names in the held release fails four `TestMemberReqHeldReleaseAI` subtests; dropping the join marker fails `TestMemberReqAINewMember` (unanswered join) and `TestMemberReqAICheckOnJoinOff`; making the watcher's verdict clause always true fails five "one job" subtests.
- **AGENTS.md.** The watcher bullet, two new Data bullets (`nm`, `nu`), the pass-cache bullet, the AI rule bullet (full order, `Joining`, meaning of "new" and its accepted limit, the recorded watcher gap, D-03 amended for holds carrying bit 128), the join, held-release and runner bullets.

## Task Commits

1. `409de70` test(15-05): a baseline for established members, a marker for new ones, and the watcher's AI read (does not compile on its own: it uses `Joining` and the new functions; the plan allows a red test commit)
2. `409edbb` feat(15-05): record established members without a call and judge new ones
3. `4dcfa28` docs(15-05): describe the baseline, the marker and the watcher's AI read

The measured `commits: 4` in the frontmatter is `git rev-list --count 3dac5a1..HEAD` at SUMMARY time and includes `ec47e8d docs(pause): phase 15 paused at plan 15-05 of 11`, a pause record committed by another agent while this plan ran; this plan's own commits are the three above.

## Verification (real results)

- Task 1: `go test -tags testtools -race -count=1 -run '^(TestMemberReqAIBaseline|TestMemberReqAIRename|TestMemberReqAINewMember|TestMemberReqAICheckOnJoinOff|TestMemberReqAIMarkerSurvivesLongHold)$' -v ./fuku/modules`: exit 0, every subtest PASS, no FAIL or SKIP. The six edited tracer tests (`TestMemberReqAIRule|EnforcePunishes|NotConfigured|SparesExempt|LookupOrder|Unanswerable`): exit 0 (17.5 s; Unanswerable 18.7 s). `-count=10` of `TestMemberReqAINewMember|TestMemberReqAICheckOnJoinOff`: exit 0 (ok, 32.3 s).
- Task 2: `-run '^(TestMemberReqWatcherAI|TestMemberReqWatcherAIBurst|TestMemberReqPassCacheWithAI|TestMemberReqHeldReleaseAI)$' -v`: exit 0, all PASS. The Phase 13/14 list (`TestMemberReqWatcherLookupRules|...|TestMemberReqRunnerSheds`): exit 0 (ok, 18.0 s). `test -z "$(gocyclo -over 19 memberreq_watch.go memberreq_held.go memberreq_join.go memberreq_enforce.go memberreq_ai.go memberreq_lookup.go)"`: exit 0; `memberReqLookupCached` measures 18.
- Task 3: `-run '^(TestMemberReqAI|TestMemberReqWatcher|TestMemberReqHeld|TestMemberReqJoin|TestMemberReqPassCache|TestMemberReqCaptchaGuard|TestMemberReqOnePunishmentAcrossReplicas|TestMemberReqLookupOnePunishmentAcrossReplicas|TestShutdownWaitsForMemberReqJobs|TestMemberReqEnforcementLive)'`: exit 0 (ok, 79.5 s). The plan's verification family `-run '^(TestMemberReqLookup|TestMemberReqFixed|TestMemberReqEnforce|TestMemberReqRunnerSheds|TestShutdown)'`: exit 0 (ok, 122.9 s).
- `go vet -tags testtools ./fuku/modules`: exit 0. `CGO_ENABLED=0 go build ./...`: exit 0. `gofmt -l` on the touched files: empty. `python3 .planning/phases/13-member-requirements/handoff/lint-local.py 8c0c827` read directly: exit 0 ("no dupl, gocyclo or godox findings"); its first run found staticcheck ST1018 (a literal U+200B in my baseline test), fixed before that file was first committed, then exit 0. `git diff --exit-code 8c0c827 -- go.mod go.sum`: exit 0.
- Acceptance greps: `func memberReqAIMarkJoin(` 1, `memberReqAIMarkJoin` in the join 1, `Joining` in memberreq_lookup.go 2, `fuku:memberreq:nu:` in memberreq_ai.go 1, `memberReqLookupRules` in watch 6, `memberReqAIBaselineKey` in watch 1, `Names: &names` in held 1, `func memberReqAIWatchSatisfied` 1, `func memberReqAIWatchQueue` 1, `func TestMemberReqWatcherAI` 2 (with the Burst), `fuku:memberreq:nm` in AGENTS.md 2, `fuku:memberreq:nu` 3, no `Skip(` or `Parallel()` in either new test file.
- Not run: `make lint` and gosec (cannot run here), `make test` (the closing plan 15-11), the settings test family (the plan does not ask for it and no settings code changed), PostgreSQL (nothing in this plan rests on it; the `fuku/modules` harness runs on SQLite).

## Tests that changed

`mreqAIAsk` (the direct-call helper) sets `Joining`, which edits `TestMemberReqAIRule`'s direct calls in one place; `TestMemberReqAIEnforcePunishes` subtest `job`, `TestMemberReqAIUnanswerable` job half and the three job subtests of `TestMemberReqAILookupOrder` record the member as new (`mreqAIMarkNew`) before the job runs. Every assertion they had is kept. `TestMemberReqAISparesExempt` needed no edit. The plan also named the job halves of `TestMemberReqAINotConfigured`: with the AI not configured the rule is inert whatever the baseline says, so those halves pass unedited and I did not add a marker there. The call sites of `memberReqLookupCached` in `memberreq_cna_test.go` (2) and `memberreq_watch_lookup_test.go` (4) gain the empty names argument.

## Deviations from Plan

1. **[Judgement] One helper edit instead of many call sites.** `mreqAIAsk` sets `Joining: true` itself (documented), and a new `mreqAIAskAs(env, userID, names, mode)` serves the baseline tests; the plan said direct calls "pass `Joining: true`".
2. **[Judgement] `mreqAICleanup` deletes the baselines of the user IDs it is given**, as the plan says; the harness's Redis is a per-test miniredis, so the deletion is hygiene only. Most tests pass `nil, nil` and rely on that.
3. **[Own mistake, fixed] ST1018** in `memberreq_ai_baseline_test.go` (a literal zero-width space) found by the lint stand-in; fixed with an escape before the plan-end run.
4. **Design note on the watcher bullet.** The pass key may still be written by the clean path of a satisfied member (the existing guarded predicate); `TestMemberReqWatcherAI`'s keyspace comparison excludes `fuku:memberreq:pass:*` for that reason and nothing else.

No unused identifier is left: `memberReqAIMarkJoin` is called by the join entry and by `mreqAIMarkNew`; `mreqAIBaselineOf`, `mreqAIMarkerOf`, `mreqAIAskAs`, `mreqAIOrdinary` and the helpers of the new test files are all read by tests of this plan.

## Requirements

`requirements-completed: []`. MREQ-01, MREQ-10 and MREQ-14 stay Pending and complete in plan 15-11. REQUIREMENTS.md is unchanged.

## Known Stubs

None. This plan still leaves for later plans: the names-bound gate, hourly budgets and breaker (15-06), the remedy sentence and "I fixed it" with the AI rule (15-07), the page status and Off-only form (15-08).

## Threat Flags

None new. T-15-17 (a joiner baselined unjudged: the join marker plus `TestMemberReqAINewMember`, `TestMemberReqAICheckOnJoinOff`), T-15-18 (an expired baseline laundering a flagged member: `TestMemberReqAIMarkerSurvivesLongHold`), T-15-19 (watcher writes or calls: `TestMemberReqWatcherAI` keyspace, request and command-count checks, `TestMemberReqWatcherAIBurst`) and T-15-20 (`nm` holds a 32-hex hash, `nu` the digit 1, no log line names either) are mitigated. T-15-SC: `go.mod` and `go.sum` unchanged. Recorded for the owner, not changed: the watcher gap (a baseline-equal member without a marker stays satisfied although another account's judgement of the same string cached a breach, the safe direction; pinned by a `TestMemberReqWatcherAI` subtest and written in AGENTS.md).

## Self-Check: PASSED

Both created test files and this SUMMARY exist, and the commits 409de70, 409edbb and 4dcfa28 are ancestors of HEAD.
