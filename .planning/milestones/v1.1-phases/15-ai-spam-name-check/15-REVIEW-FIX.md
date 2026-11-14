---
phase: 15-ai-spam-name-check
review: 15-REVIEW.md
status: all_fixed
findings_in_scope: 5
fixed: 5
skipped: 3
fixed_at: 2026-11-09
---

# Phase 15: Review Fix Report

Fix pass for WR-01, WR-02, WR-03, WR-04 and IN-03 of `15-REVIEW.md`, run sequentially on branch
`claude/ecstatic-goldberg-qsss05` in that order. Every finding has a RED `test(15)` commit and a GREEN `fix(15)` commit.
Deferred by the orchestrator (untouched): IN-01, IN-02, IN-04. No requirement was ticked or unticked, `phase.complete` was not
run, `REQUIREMENTS.md` was not edited, `go.mod` and `go.sum` are unchanged since 8c0c827.

## Commits

| Finding | RED (test) | GREEN (fix) |
|---------|------------|-------------|
| WR-01 | 89883df | 122e732 |
| WR-02 | 2c57ca9 | 0b1ac70 |
| WR-03 | 75894d0 | 1079516 |
| WR-04 | e3adc29 | 03c10a3 |
| IN-03 | 7930063 | 97496fa |

Each RED commit failed its new assertions for the stated reason on the then-current code (a hold released with 0 provider
requests, a baseline TTL of 720h where 4320h was wanted, a Warn that was deferred, 6 job starts and 6 live lookups, a CSV cell
with no leading quote), never on a compile error. No test is skipped and `t.Skip` is not used.

## WR-01: a bit-128 hold released, a mute freed, by a baseline-only pass

**What changed.** `memberReqLookupMode` has a new field `IgnoreBaseline` (`Joining` keeps meaning only "the join entry's run").
`memberReqAIPrepare` passes `Joining || IgnoreBaseline` to `memberReqAIDecide`, whose parameter is now `skipBaseline`.
`memberReqHeldLookupsClear` and `memberReqFixedJudge` set it, so the AI row answers a held member and a button press from the
verdict cache or one real call, never from a baseline step, even when Redis lost `nv`, `nm` and `nu`. The baseline-only pass is
unchanged for the watcher's and the job's first sight of an established member who has no hold.

**Files.** `memberreq_lookup.go`, `memberreq_ai.go`, `memberreq_held.go`, `memberreq_fixed.go`, `AGENTS.md`.

**Tests.** `TestMemberReqAIHeldRedisLoss` and `TestMemberReqAIFixedRedisLoss` (`memberreq_ai_redisloss_test.go`): keys flushed,
unchanged spam name, provider answering 0.97: the Warn hold stays (outcome Held, 1 request, marker set, no baseline written) and
the mute stays ("still broken" alert, 1 request, hold and live marker intact); the same member with a changed ordinary name is
released / freed with exactly one request.

## WR-02: the 30 day baseline lapses for every member

**What changed.** A separate `memberReqAIBaselineTTL` (default `memberReqAIBaselineTTLDefault`, 180 days) is used by the two
baseline writes (`SET NX` at first sight, and the definite pass transaction). The marker keeps `memberReqAIStateTTL` (30 days).
The equal-baseline pass is still a no-write step. AGENTS.md (the `nm` Data bullet and the AI rule bullet) and `15-SPIKE.md`
(accepted limits) now say that a member dormant for over 180 days is first-sighted again and baselined with no call (accepted,
the same D-02 limit).

**Files.** `memberreq_ai.go`, `AGENTS.md`, `15-SPIKE.md`.

**Tests.** `TestMemberReqAIBaselineTTL` (`memberreq_ai_baselinettl_test.go`, literal day counts, not the package variables): a
first sighting and a definite pass write `nm` for about 180 days, a breach keeps `nu` at about 30, an equal baseline is still not
rewritten. Existing test edited where its expectation legitimately changed: the helper `mreqAIWantBaseline`
(`memberreq_ai_baseline_test.go`) now expects `memberReqAIBaselineTTL`, not `memberReqAIStateTTL`; the "not rewritten" assertions
of `TestMemberReqAIBaseline` are untouched and pass.

## WR-03: a Redis outage deferred weaker punishments

**What changed.** No Redis client, a failed state read (`memberReqAIPrepare`), a failed cache re-read inside the flight
(`memberReqAIJudge`) and a budget counter fault or no client at the reservation (`memberReqAIReserveUnit`) now answer
`memberReqAIStickyUnknown`. The prepare step runs before the flight, so its sticky bit travels in a new `memberReqAIPlan.sticky`
field and `memberReqAIRule` calls `memberReqNoteSticky` for it, which reaches `touch.sticky` through the same bit stamping D-22
built. The verdict stays unknown, so the held release and "I fixed it" still refuse. A 5xx, a 400, a timeout and a refusal are
still plain unknowns and still defer.

**Files.** `memberreq_ai.go`, `AGENTS.md`.

**Tests.** `TestMemberReqAIRedisFaultsAreSticky` (`memberreq_ai_redisdown_test.go`): last-name Warn plus AI Ban, blank last name,
with no Redis client, a failed state read and a budget counter fault: the Warn is applied with the last name bit only and no
provider request; the control with Redis up and a 500 still defers; the lookup fold stays unknown for each cause; a failed
in-flight cache re-read is a sticky unknown. The existing `TestMemberReqAIUnknownDefers` is unchanged and passes.

## WR-04: a paused breaker cost a job per marker holder

**What changed.** `memberReqAIOnlyPaused` (`memberreq_watch.go`): after `memberReqLookupCached` says "not satisfied", and before
`memberReqMissExempt`, the watcher checks, cheapest first, whether the AI rule is live for the group and
`memberReqAIPaused(chatID)` (in memory, provider-wide or the group's own) holds. Only then does it make one more Redis-only read,
`memberReqLookupCachedWith(..., ignoreAI=true)` (the old read body with the AI keys left out; `memberReqLookupCached` is a
wrapper over it with `false`). When every other lookup rule is satisfied, the AI part is the only open reason: the watcher starts
no job and runs no clean path and returns `ext.ContinueGroups` having done nothing. This is a third outcome, deliberately not
"satisfied", because satisfied would run `ClearHoldOnClean` and delete the hold of a member held for an AI breach during the
pause. Any other open reason (a name rule, the photo or channel rule) still hands a job as before. The watcher writes nothing and
calls nobody. The marker stays, so the first message after the pause is judged.

**Files.** `memberreq_watch.go`, `AGENTS.md`.

**Tests.** `TestMemberReqWatcherAIPaused` (`memberreq_ai_pausedwatch_test.go`): six clean messages of a marker holder under a 401
provider-wide pause start no job and write nothing, the same six cost no live lookup (the reviewer's reproduction was 6), a
group's own pause counts, a member with a bit-128 hold who posts during the pause keeps the hold and no pass key is written (a
guard against the "satisfied" shortcut; it passed before the fix too, because the job path also kept the hold), after the pause
ends the next message makes one provider request and records the baseline, and a photo rule that is open still hands a job while
a member whose only open rule is the paused AI rule hands none.

## IN-03: spreadsheet formulas in the spike's names CSV

**What changed.** `csvSafe` in `scripts/claude-spike/names.go` prefixes a single quote to a first name, last name or username
cell that starts with `=`, `+`, `-`, `@`, a tab or a carriage return; every other cell is returned as it was. The label column is
not touched (the reader only accepts `spam` or `ham`), and the input file's parsing is unchanged. AGENTS.md states it.

**Files.** `scripts/claude-spike/names.go`, `AGENTS.md`.

**Tests.** `TestNamesCSVNeutralisesFormulas` (`scripts/claude-spike/names_test.go`, parses the output with `encoding/csv`): the six
hostile starts in all three columns, and an ordinary row (`a=b`, `x-y@z`) exactly as written.

## Deferred (not fixed, by decision)

- **IN-01** (verdict key and baseline hash ignore the case of the first and last name, while the text sent to the model keeps it):
  accepted. A consistency note that needs the model to score two case variants of one string on opposite sides of 0.9; fixing it
  would bump `mrn1` and `mrv1` and invalidate every baseline and cached verdict.
- **IN-02** (a refusal is never cached, so it costs a call per message up to the 5 per account per hour cap): accepted. Bounded by
  the account budget, waste and a short-lived deferral, not a bypass.
- **IN-04** (one flight serves every chat, so the leader's breaker, budget or cap decides a follower's answer): accepted. It is
  transient (the next message re-judges) and needs the same canonical string in two chats at the same moment.

## Verification

- `gofmt -l` on every `.go` file changed since 8c0c827: clean except `main.go`, which is also unformatted at 8c0c827 and was not
  touched by this pass. `CGO_ENABLED=0 go build ./...`: ok. `go vet -tags testtools ./fuku/modules ./scripts/claude-spike`: ok.
  `git diff --exit-code 8c0c827 -- go.mod go.sum`: exit 0.
- Lint stand-in `python3 .planning/phases/13-member-requirements/handoff/lint-local.py 8c0c827`, exit code read directly: 0
  ("no dupl, gocyclo or godox findings on lines added since 8c0c827"), after the last code commit.
- gosec 2.29.0 (`$(go env GOPATH)/bin/gosec -quiet -no-fail -fmt json ./...`) after the last code commit: of the 38 non-test `.go`
  files changed since 8c0c827 the only one with an issue is the baseline `fuku/modules/warns.go:36 G115`. None of the files this pass
  touched has an issue.
- CI's own gate: `go test -tags testtools -json -race -count=1 -timeout 30m -run '^(TestSettings|TestLoadSettings|TestMemberReq|TestMemberRequirement|TestShutdown|TestChatRequirementsSettings|TestApplyMemberReqAction|TestApplyWarn)' ./fuku/modules ./fuku/db/memberreq ./fuku/i18n`
  exited 0 (`fuku/modules` ok in 730 s), followed by the full tests of `./scripts/claude-spike ./fuku/utils/claude ./fuku/utils/namecanon`
  (exit 0), into one JSON file: 3118 test events passed, 0 failed, 0 skipped, and `go run ./scripts/check_test_results` on it
  exited 0. The `covdata` fallback was not needed.
- The new tests with `-race -count=10` (`TestMemberReqAIHeldRedisLoss`, `TestMemberReqAIFixedRedisLoss`, `TestMemberReqAIBaselineTTL`,
  `TestMemberReqAIRedisFaultsAreSticky`, `TestMemberReqWatcherAIPaused`): ok, exit 0; `TestNamesCSVNeutralisesFormulas` with
  `-count=20`: ok.
- Not run: `make test` and `make lint` (cannot run here, see the brief), `make check-translations` and `make check-docs` (no locale
  or generated-doc source was touched), a PostgreSQL proof (no migration or SQL changed; the `fuku/modules` harness runs on SQLite).

## Concerns noted, not fixed

- **NEW-01.** A member who is held for a name rule (a Warn hold that lacks bit 128) while the AI rule's answer was a sticky
  unknown (Redis down, paused breaker, used-up budget) is not judged for the AI rule while the hold lives, because a held member's
  messages make no lookup unless the hold carries bit 128. This is the partial episode D-22 already accepts for the sticky states;
  WR-03 widens the set of causes to Redis faults. It clears with the hold (a clean message, a rename, a rejoin).
- **NEW-02.** WR-01 costs one provider call per held bit-128 member after a Redis loss (bounded by the three budgets, and cached
  afterwards), and a held member who renamed during a breaker pause stays held until the pause ends (the watcher starts no job for
  them, WR-04), at most 5 minutes per pause, repeating while a key stays rejected.
- **NEW-03.** The WR-04 check makes one extra Redis-only read per unsatisfied message while a pause is on and the group's AI rule is
  live, instead of a runner job and a paced live lookup; an unpaused group pays nothing extra (the in-memory test comes first).
