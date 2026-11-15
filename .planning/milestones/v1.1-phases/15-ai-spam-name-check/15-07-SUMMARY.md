---
phase: 15-ai-spam-name-check
plan: 07
subsystem: member-requirements
tags: [member-requirements, ai-spam-name, notice, remedy, i-fixed-it, d-23]

requires:
  - phase: 15-ai-spam-name-check
    provides: 15-04 (the AI rule row, verdict cache, stand-in), 15-05 (baseline, marker, Names in the lookup mode), 15-06 (budgets, gate, breaker, sticky unknown, stale-names guard)
provides:
  - memberReqRule.RemedyKey (photo row mreq_remedy_photo, AI row mreq_remedy_ai) and memberReqNoticeRemedies, replacing memberReqNoticeHasRemedy, so no code names a rule's code for a remedy
  - the plain warn, kick and mute notices end with one remedy sentence per remedy-bearing broken rule, in rule order (photo, then AI)
  - "I fixed it" step 5 passes the live record's names, so the AI row judges them from the verdict cache (D-23)
  - TestMemberReqNoticeAIRemedy and TestMemberReqFixedAI
affects: [15-08, 15-10, 15-11]

plan_head_before: ec661d0
plan_head_after: cdd4051afe43aebb0d787eb1c52f794c101f13c0
actuals:
  tokens: 21000
  tasks: 3
  commits: 4

tech-stack:
  added: []
  patterns:
    - "A remedy sentence is a table field (RemedyKey) read by one function that loops over the table, so a later rule with a remedy is one field"
    - "A test that abandons a hung flight orders the flight's last reads before the swap cleanups by taking and giving back a gate slot (the race detector cannot see a sleep)"

key-files:
  created:
    - fuku/modules/memberreq_notice_ai_test.go
    - fuku/modules/memberreq_fixed_ai_test.go
  modified:
    - fuku/modules/memberreq_rules.go
    - fuku/modules/memberreq_notice.go
    - fuku/modules/memberreq_fixed.go
    - fuku/modules/memberreq_screens_test.go
    - fuku/modules/memberreq_notice_lookup_test.go
    - fuku/modules/memberreq_fixed_lookup_test.go
    - AGENTS.md

key-decisions:
  - "The AI row gets no cache bypass at the button (D-23); step 5 only passes Names, and memberReqFixedRun gained no branch (complexity stays 19)"
  - "The two Phase 13/14 expectations the AI tail changes (the all-rules notice suffix, the screens test's suffix check) were edited in the tracer feat commit instead of Task 3, so no commit is red"

requirements-completed: []

coverage:
  - id: D1
    description: "the plain warn, kick and mute notices end with one remedy per remedy-bearing broken rule in rule order (photo, then AI); ban and warn-limit notices carry none; every notice with eight phrases and both tails is at most 520 code points in all 8 languages; mreq_warn_reason at most 300; no AI string holds a model name, digit, percent or member name"
    requirement: "MREQ-14 (contribution only; completes none)"
    verification:
      - kind: integration
        ref: "go test -tags testtools -race -count=1 -run '^(TestMemberReqNoticeAIRemedy|TestMemberReqScreensFitEveryLocale|TestMemberReqNoticePhotoRemedy)'"
        status: pass
    human_judgment: false
  - id: D2
    description: "the button judges the AI rule with the live names from the cache: a renamed member is freed by one call, unchanged names read the cached verdict with no request (11 taps, 0 requests), a rename to another spam string costs one call for two taps"
    requirement: "MREQ-14 (contribution only)"
    verification:
      - kind: integration
        ref: "TestMemberReqFixedAI (-count=10: ok, 136.4 s)"
        status: pass
    human_judgment: false
  - id: D3
    description: "an AI answer that cannot be given (23 shapes: provider answers, a hang past the shortened budget, paused breaker, three budgets, a counter error, a full gate, a panic) is mreq_fixed_check_failed with no restrict call and the hold untouched; with the last-name rule definitely broken the answer names only that rule; AI not configured, approved and foreign-mute cases; the group's language; a cached pass; eight rules counted"
    requirement: "MREQ-10 (contribution only; completes none)"
    verification:
      - kind: integration
        ref: "TestMemberReqFixedAI, TestMemberReqFixedLookups, TestMemberReqStillBrokenText"
        status: pass
    human_judgment: false

duration: about 50 min (not timed at start; estimated from the session)
completed: 2026-11-09
status: complete
---

# Phase 15 Plan 07: the AI remedy tail and the button's AI step Summary

**A member held for a spam-like name now reads what to do in the group notice (the photo remedy, then `mreq_remedy_ai`, one sentence per remedy-bearing broken rule from a `RemedyKey` table field), and "I fixed it" judges the AI rule with the member's current names from the verdict cache: a renamed member is freed by one call, unchanged names get the same cached verdict on every tap, and an AI that cannot answer changes nothing.**

## Accomplishments

- **Notice (`memberreq_notice.go`, `memberreq_rules.go`).** `memberReqRule.RemedyKey` is set on the photo and AI rows. `memberReqNoticeRemedies(key, broken)` (replacing `memberReqNoticeHasRemedy`, whose literal `"ph"` is gone) returns the remedy keys of the broken rules in table order for `mreq_notice_warn`, `mreq_notice_kick` and `mreq_notice_mute` only, and `memberReqNoticeText` appends each after one space, after translation and before the mention splice. The ban and the three warn-limit notices carry none.
- **Button (`memberreq_fixed.go`).** `memberReqFixedJudge` builds `liveNames` from the live record and passes `Names: &liveNames` in its lookup mode; the AI row already ignores `Fresh` and `Classify`, so the press reads the verdict cache (D-23), makes one call only for names with no verdict through the breaker, gate and account budget inside the 5 s, and maps every unknown to `mreq_fixed_check_failed` through the existing Phase 14 step 5. No other step changed; `memberReqFixedRun` is still at complexity 19.
- **Tests.** `TestMemberReqNoticeAIRemedy` (8 subtests) and `TestMemberReqFixedAI` (frees a renamed member with one call; unchanged names with a cached breach: 11 taps, 0 requests, nothing changed; a rename to another spam string: one call for two taps; 23 unanswerable shapes with nothing else broken; the last-name rule definitely broken beats an unknown AI answer under three shapes; approved freed at step 4b with no request; AI not configured is inert and the member is freed; a foreign mute is refused with no request; the answer in the group's language (uk); a cached pass with a photo-held member; all eight rules answer with the count 8, then 7 after a last name, then 6 with Chinese switched Off).
- **AGENTS.md.** The executor bullet's notice paragraph (table-driven tails, the 520 cap proved by the rendered check, the 636-at-ceilings and French-mute-at-506 facts, `mreq_warn_reason` at most 300), step 5 of the `mreq|v1` bullet (the AI row from the cache, D-23, the no-bypass reason, up to eight counted) and the Permissions bullet (rename or approval).

## Task Commits

1. `5b01f23` test(15-07): the AI remedy and the button for a renamed member (red: no AI tail yet, the press did not pass the names)
2. `d05a457` feat(15-07): the AI remedy sentence and the button's AI step
3. `f000dfb` test(15-07): the notice with both tails and every answer of the button for the AI rule
4. `cdd4051` docs(15-07): describe the AI remedy tail and the button's AI step

`commits: 4` is `git rev-list --count ec661d0..HEAD` (base `ec661d0`, the HEAD before this plan's first commit; no ledger file was written for this plan, so the base is the recorded HEAD).

## Verification (real results)

- Task 1: `-run '^(TestMemberReqNoticeAIRemedy|TestMemberReqFixedAI)$'`: red at `5b01f23` (the notice had no AI tail, the press made no AI call), ok after `d05a457`. `-run '^(TestMemberReqNoticePhotoRemedy|TestMemberReqFixedLookups|TestMemberReqFixedUnmutes)$'`: ok.
- Task 2: `-run '^(TestMemberReqNoticeAIRemedy|TestMemberReqFixedAI|TestMemberReqStillBrokenText)$' -v`: ok, every subtest PASS, no FAIL or SKIP. `-count=10 -run '^TestMemberReqFixedAI$'`: exit 0 (ok, 136.4 s); eight more runs of the compiled test binary: all exit 0. Most Task 2 tests passed at once, because Task 1's change and Phase 14's step 5 already map an unknown to check-failed and a breach to still-broken; each is kept as a new entry into those paths. The one real failure found was a data race in my own test (see Deviations).
- Task 3: `-run '^(TestMemberReqScreensFitEveryLocale|TestMemberReqNoticePhotoRemedy|TestMemberReqFixed|TestMemberReqNotice|TestMemberReqStillBrokenText|TestMemberReqWarnRecovery)'`: ok (55.7 s), exit 0. `-run '^(TestMemberReqFixedNeverLiftsWithLookupRules|TestMemberReqFixedApproved|TestMemberReqFixedRefusals|TestMemberReqLookupDoesNotBlockNameBreach|TestMemberReqLookupOrder|TestMemberReqLookupPunishes|TestMemberReqAI)'`: ok (81.8 s), exit 0.
- `CGO_ENABLED=0 go build ./...`: exit 0. `go vet -tags testtools ./fuku/modules`: exit 0. `gofmt -l` on every file I touched: empty (the package has other unformatted files that are not mine). `python3 .planning/phases/13-member-requirements/handoff/lint-local.py 8c0c827` read directly: exit 0 ("no dupl, gocyclo or godox findings"). `gocyclo -over 19` on the three production files: empty (`memberReqFixedRun` 19, unchanged). `git diff --exit-code 8c0c827 -- go.mod go.sum`: exit 0. No `Skip(` or `Parallel()` in the two new test files.
- Not run: `make lint` and gosec (cannot run here), `make test` (the closing plan 15-11), the settings test family (the plan does not ask for it and no settings code changed), PostgreSQL (nothing here rests on it; the module harness runs on SQLite).

## Tests that changed

- `TestMemberReqScreensFitEveryLocale` (`memberreq_screens_test.go`): its notices carry all eight phrases and both tails; "ends with the photo remedy" became "ends with the photo remedy then the AI remedy" (M20) plus a no-remedy check for the other notices; the cap comment and header say eight rules; "Five, six and seven rules" became "Five to eight rules". The still-broken render already looped over `len(memberReqRules)`.
- `TestMemberReqNoticePhotoRemedy` (`memberreq_notice_lookup_test.go`): the all-rules subtest is renamed to "all eight rules" and expects the photo remedy directly before the AI remedy at the end; the no-photo-remedy codes lists gain `{"ai"}` and `{"ln","ch","ai"}`.
- `TestMemberReqStillBrokenText` (same file): the real-locale count loop runs to 8 (the first loop already ran to `len(memberReqRules)`).
- The "five or more broken rules" case of `TestMemberReqFixedLookups` (`memberreq_fixed_lookup_test.go`): runs to eight rules (Chinese and the AI rule on, the AI verdict a cached breach), counts 8, then 7 after a last name, then 6 with Chinese switched Off, so "a rule that is Off is not counted" is still asserted.

## Deviations from Plan

1. **[Judgement] The two test expectations the AI tail changes were edited in the tracer feat commit (`d05a457`), not in Task 3,** so no commit leaves a Phase 13/14 test red. The plan's Task 1 verify says those tests are "unedited at this point"; they cannot pass unedited once the AI tail exists. The remaining named edits (renames, comments, loops, the fixed-lookup count case) are in Task 3 as planned.
2. **[Rule 1 - own test bug] A data race in my hung-flight cases.** An abandoned flight reads `memberReqLookupFlightTimeout` while a cleanup restores it; the Phase 14 pattern (`time.Sleep(700ms)`) gives no happens-before edge and the race detector reported it in 2 of 6 runs. Fixed with `mreqAIAwaitHungFlight` (waits for the stand-in to be idle, then takes and gives back a gate slot, which orders the flight's last reads before the restores). 8 of 8 compiled-binary runs and `-count=10` pass. The Phase 14 hang tests keep their sleep (out of scope, not changed).
3. **[Judgement] The "unknown with the last-name rule broken" case runs under three shapes** (a 500, a request that outlives the budget, a paused group breaker) rather than all 23, to keep the run time down; every shape is covered for "nothing else broken".

No unused identifier is left: `RemedyKey` and `memberReqNoticeRemedies` are read by the builder, and every test helper is called.

## Requirements

`requirements-completed: []`. MREQ-01, MREQ-10 and MREQ-14 stay Pending and complete in plan 15-11. REQUIREMENTS.md is unchanged.

## Known Stubs

None. Left for later plans: the page status and the Off-only form (15-08), the log and locale sweeps (15-10) and the end-to-end test (15-11).

## Threat Flags

None new. T-15-21 (the button as a re-roll): `TestMemberReqFixedAI` asserts 0 requests for 11 taps on unchanged names and exactly 1 for a rename. T-15-22 (a notice or alert naming the name, a probability or the model, or too long): `TestMemberReqNoticeAIRemedy`, `TestMemberReqScreensFitEveryLocale`. T-15-SC: `go.mod` and `go.sum` unchanged.

## Self-Check: PASSED

The two created test files and this SUMMARY exist, and the commits 5b01f23, d05a457, f000dfb and cdd4051 are ancestors of HEAD.
