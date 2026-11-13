---
phase: 15-ai-spam-name-check
plan: 04
subsystem: member-requirements
tags: [member-requirements, ai-spam-name, claude, verdict-cache, singleflight, rule-table, tracer]

requires:
  - phase: 15-ai-spam-name-check
    provides: 15-01 (ai_action, memberreq.RuleAI), 15-02 (claude.ClassifyNames, namecanon), 15-03 (the ten AI locale keys)
provides:
  - the eighth memberReqRules row (ai, bit 128, Lookup memberReqAIRule, no Broken, next free bit 256)
  - memberReqLookupMode.Names, passed by memberReqLiveViolation from the live getChatMember record
  - memberReqAIConfigured (ENABLE_AISPAM and an Anthropic key, nothing else), memberReqAIBar 0.9, memberReqAIRule, memberReqAIJudge, memberReqAIParseProbability
  - the verdict cache fuku:memberreq:nv:<hash> (the probability, 24 h, a pass and a breach alike) and the flight family n:<hash>
  - the Anthropic stand-in toolkit (mreqAIState, mreqInstallAIFake, withAIConfigured, mreqAICleanup) and six tracer tests
  - every suite that counted seven rules counts eight
affects: [15-05, 15-06, 15-07, 15-08, 15-10, 15-11]

plan_head_before: b523d6294e5d873c9c7d9e0940e1282dff2f82f3
plan_head_after: 159513feb0e694ad0c7c730e46e6ff887a4b65b1
actuals:
  tokens: 18800
  tasks: 3
  commits: 5

tech-stack:
  added: []
  patterns:
    - "A lookup row that is not a Telegram question still answers one memberReqVerdict through memberReqFlightValue: key family n:, the function recovers inside, runs on memberReqContext()"
    - "The cache holds the probability, not the verdict, so a retuned bar applies to cached entries at once; the one parse of it rejects NaN and infinities"
    - "A read step (memberReqAIPrepare) and a decide step (memberReqAIRule) keep each function under the gocyclo limit"

key-files:
  created:
    - fuku/modules/memberreq_ai.go
    - fuku/modules/memberreq_ai_fake_test.go
    - fuku/modules/memberreq_ai_test.go
  modified:
    - fuku/modules/memberreq_rules.go
    - fuku/modules/memberreq_lookup.go
    - fuku/modules/memberreq_enforce.go
    - fuku/modules/memberreq_rules_test.go
    - fuku/modules/settings_mreq_test.go
    - fuku/modules/settings_mreq_card_test.go
    - fuku/modules/settings_mreq_lookup_test.go
    - fuku/modules/memberreq_lookup_e2e_test.go
    - fuku/modules/settings_staff_privacy_test.go
    - AGENTS.md

key-decisions:
  - "memberReqAIPrepare returns a small plan struct (canon, key, verdict, done) so the read step and the decide step are separate functions and neither passes gocyclo"
  - "No Redis and a Redis read error are both unknown (the cost guard: with the cache unreadable the provider is not asked); a miss and an unparsable stored value are a miss"
  - "mreqAICleanup keeps the plan's (t, chatIDs, userIDs) signature: it deletes every nv key before and after the test, the users rows of userIDs, and the chats' Redis keys only when the test ends"

requirements-completed: []

coverage:
  - id: D1
    description: "a joiner rated 0.9 or more is muted/warned/kicked/banned once with one notice naming the AI phrase; below 0.9 is welcomed; Off asks nobody"
    requirement: "MREQ-14, MREQ-01 (contribution only; completes none)"
    verification:
      - kind: integration
        ref: "go test -tags testtools -race -count=1 -run '^TestMemberReqAIEnforcePunishes$' ./fuku/modules (and -count=10)"
        status: pass
    human_judgment: false
  - id: D2
    description: "every answer that is not a probability punishes nobody and leaves no hold, notice, write or cached verdict, on the join path and as a job"
    requirement: "MREQ-10 (contribution only; completes none)"
    verification:
      - kind: integration
        ref: "TestMemberReqAIUnanswerable (16 shapes x join and job)"
        status: pass
    human_judgment: false
  - id: D3
    description: "AI not configured, exempt members and the lookup order"
    verification:
      - kind: integration
        ref: "TestMemberReqAINotConfigured, TestMemberReqAISparesExempt, TestMemberReqAILookupOrder"
        status: pass
    human_judgment: false
  - id: D4
    description: "the rule's request, cache and bar"
    verification:
      - kind: unit
        ref: "TestMemberReqAIRule"
        status: pass
    human_judgment: false
  - id: D5
    description: "the table, pass value, page grid, row budget and Main state count eight rules"
    verification:
      - kind: integration
        ref: "the whole TestMemberReq family and the settings family on SQLite"
        status: pass
    human_judgment: false

duration: 31min
completed: 2026-11-08
status: complete
---

# Phase 15 Plan 04: the AI rule tracer Summary

**A joiner whose canonical names the model rates at 0.9 or more is now muted, warned, kicked or banned once with one notice by the real join entry, through one Claude call and one cached probability; every other outcome (a good name, a name that cannot be judged, no key, no Redis, any provider failure shape, a cancelled root) leaves the member alone. The suites count eight rules.**

## Accomplishments

- **The row.** `memberReqRules` gets the eighth row `{ai, 128, memberreq.RuleAI, ..., Lookup: memberReqAIRule, HintKey: settings_mreq_hint_ai}`. The comments say the lookup rules are three and the next free bit is 256. `memberReqBroken` still skips lookup rows; `memberReqLookupMasks` asks the AI row after photo and channel.
- **The names reach the rule.** `memberReqLookupMode.Names *memberReqNames` (nil makes the AI rule unknown). `memberReqLiveViolation` builds `names := memberReqNamesOf(member.User)` once from the live `getChatMember` record, judges the name rules with it and passes `Names: &names`; the executor never mentions the rule.
- **`memberreq_ai.go`.** `memberReqAIConfigured` is exactly `EnableAISpam && AnthropicAPIKey != ""` (own function, comment says why it never includes Redis or the raid switch). `memberReqAIRule` = `memberReqAIPrepare` (no names: unknown; not configured: inert; not judgeable: inert; no Redis or a read error: unknown; cached probability: answer) + one flight `n:<verdict key>` that re-reads the cache, calls `ClassifyNames` with the canonical strings and stores the probability for 24 h (`memberReqAIVerdictTTL`, a variable) before answering pass or breach by `memberReqAIBar` (0.9). `memberReqAIParseProbability` is the single parse of a stored value and rejects NaN, infinities and values outside 0..1 (`math.IsNaN`/`math.IsInf` before the range test). Failures: a cancelled flight logs nothing, a refusal stop is one Info line by IDs, anything else one Warn by IDs, HTTP status, error type and a scrubbed text. `mode.Fresh` and `mode.Classify` are ignored. No goroutine pool, queue or drain was added.
- **Tests (the tracer).** `memberreq_ai_fake_test.go`: the Anthropic stand-in (success, refusal, any stop and content, HTTP statuses with retry-after, null/truncated bodies, a dropped connection, a hang that ends with the request context, recorded decoded requests and key header). Six tests in `memberreq_ai_test.go`: `TestMemberReqAIRule` (bar table, caching, TTL, canonical equivalence, another model, request shape and key only in the header, Fresh/Classify do not bypass the cache, planted garbage values are a miss, 12 concurrent callers make one request, no Redis and a failing Redis), `TestMemberReqAIEnforcePunishes` (join Mute/Warn/Kick/Ban with the second delivery, below the bar, Off, and the job path), `TestMemberReqAINotConfigured` (the four combinations equal `raidAIImageConfigured`; three unconfigured states times the raid switch on and off, inert not unknown because a weaker name Warn still fires under an AI Ban), `TestMemberReqAISparesExempt`, `TestMemberReqAILookupOrder`, `TestMemberReqAIUnanswerable` (16 shapes on both paths, then a second sighting with a working answer punishes).
- **Eight rules in the suites.** All `[7]int` fixtures are `[8]int`; the shared helpers carry the AI rule; the grid is four full rows of two cells, the row budget widths are `{2,2,2,2,2,2}`.
- **AGENTS.md.** Shutdown bullet, page `m` bullet (eight cells, widths 2/2/2/2, n up to 8, control list), Data (`nv` key, the flight family and the ignored `Fresh`), the rule-table bullet, a new "Member requirements, the AI spam-name rule" bullet, the `/autolockdown` kill-switch sentence and "Defaults that surprise".

## Task Commits

1. `e3becf7` test(15-04): the AI rule through the real join entry (red: does not compile until the rule exists)
2. `d8b2641` feat(15-04): judge a member's names through Claude, cached by the names
3. `8cb9feb` test(15-04): count eight rules where the suites counted seven
4. `09eb240` test(15-04): check the planted verdicts and drop an unused stand-in helper (lint stand-in findings)
5. `159513f` docs(15-04): describe the eighth rule, its verdict cache and its shutdown behaviour

## Verification (real results)

- `go test -tags testtools -race -count=1 -run '^(TestMemberReqAIRule|TestMemberReqAIEnforcePunishes|TestMemberReqAINotConfigured|TestMemberReqAISparesExempt|TestMemberReqAILookupOrder|TestMemberReqAIUnanswerable)$' -v ./fuku/modules`: exit 0, 88 PASS lines, no FAIL or SKIP (first run, before the lint-fix commit; the whole family below re-ran them after it).
- `go test -tags testtools -race -count=10 -run '^TestMemberReqAIEnforcePunishes$' ./fuku/modules`: exit 0 (ok, 34.5 s).
- Mutation checks (reverted, file restored byte-identical): dropping the NaN/Inf guard fails the planted-value subtest (`planted "NaN": requests = 0, want 1`); changing `>=` to `>` fails the bar table at 0.9.
- Task 2 list `-run '^(TestMemberReqRuleTable|TestMemberReqPassValue|TestMemberReqRuleKeysExist|TestMemberReqStrictest|TestMemberReqBrokenIgnoresLookupRules|TestMemberReqStrictestLookupBits|TestMemberReqLookupBroken|TestSettingsMreqRowBudget|TestSettingsMreqCallbackBytes|TestSettingsMreqLookupRulesStore|TestSettingsMreqOpensFromMain|TestSettingsMreqMainState|TestSettingsMreqBasicGroup)$'`: exit 0, each of the 13 names printed one PASS. `go vet -tags testtools ./fuku/modules`: exit 0.
- `go test -tags testtools -race -count=1 -run '^(TestMemberReq)' ./fuku/modules`: exit 0 (ok, 280.6 s) after the final code state.
- `go test -tags testtools -race -count=1 -run '^(TestSettings|TestLoadSettings)' ./fuku/modules ./fuku/i18n`: exit 0 (modules ok 429.3 s, i18n ok 6.9 s).
- Guarantees by name (`TestMemberReqOnePunishmentAcrossReplicas`, `TestMemberReqLookupOnePunishmentAcrossReplicas`, `TestMemberReqJoinLookups`, `TestMemberReqLookupDoesNotBlockNameBreach`, `TestShutdownEndsMemberReqLookups`, `TestMemberReqRunnerSheds`, `TestMemberReqEnforcementLive`): exit 0, all seven PASS.
- `CGO_ENABLED=0 go build ./...`: exit 0. `python3 .planning/phases/13-member-requirements/handoff/lint-local.py 8c0c827` read directly: exit 0 ("no dupl, gocyclo or godox findings"); its first run reported staticcheck U1000 (an unused `queue` helper of the stand-in), ST1018 (a literal U+200B) and four errcheck findings on `miniredis.Set` in my tests, all fixed in `09eb240`, then exit 0. `test -z "$(~/.cache/fuku-lint-tools/gocyclo -over 19 memberreq_ai.go memberreq_rules.go memberreq_lookup.go memberreq_enforce.go)"`: exit 0. `git diff --exit-code 8c0c827 -- go.mod go.sum`: exit 0.
- Acceptance greps: `func memberReqAIRule(` 1, `func memberReqAIConfigured(` 1, `Code: "ai"` 1, `Names *memberReqNames` 1, `math.IsNaN` 1, `ClassifyNames` 1, `memberReqContext()` in memberreq_lookup.go 2, no `Skip(`/`Parallel()` in the two new files, `"ai"` in memberreq_rules_test.go 1, `settings_btn_mreq_ai` 1, `settings_mreq_rule_ai` 1, `grep -c '2, 2, 2, 1'` over the two settings test files 0 and 0, `[7]int` 0 in all five files, `fuku:memberreq:nv` in AGENTS.md 1, `memberReqAIConfigured` 1, `ai_action` 2.
- Not run: `make lint` and gosec (cannot run here), `make test` (the closing plan does), PostgreSQL (nothing in this plan rests on it; the `fuku/modules` harness runs on SQLite).

## Tests that changed

`TestMemberReqRuleTable` (eight codes with `ai` last, bit 128, hint key on the AI row), `TestMemberReqPassValue` (eight actions, the loop to 8), `requirementsOf` (eight actions, `AIAction`), `TestSettingsMreqRowBudget` and the basic-group page test (widths `{2,2,2,2,2,2}`), `TestSettingsMreqLookupRulesStore` (eight cells), the Main-state loop (n to 8, eighth action), `TestSettingsMreqPageFitsEveryLocale` (`2*3*6 + 8*3*6 + 24` screens) and the byte-budget comment. Helpers: `settingsMreqRuleKeys`, `settingsMreqRuleCodes`, `settingsMreqCellKeys`, `settingsMreqActionsOf`, `settingsMreqSeed`, `settingsMreqPageMarkers`, `settingsMreqWantPage` and `settingsMreqWantGrid` (`[8]int`, four rows of two). Not in the plan's list but required by the new row (the card test panicked at index 7 without them): `settingsMreqDescKeys` gains `settings_mreq_desc_ai` and `settingsMreqHintKeys` gains `settings_mreq_hint_ai`. Comments counting seven were updated in `settings_staff_privacy_test.go`, `memberreq_lookup_e2e_test.go` and `settings_mreq_card_test.go`. No assertion was weakened or re-recorded.

Fixtures that still say seven, each owned by a later plan: `memberreq_notice_lookup_test.go:88` (subtest name "all seven rules") and `memberreq_screens_test.go` lines 23, 40, 88 and 157 (comments about seven rule phrases and the count answer): plan 15-07. `fuku/i18n/settings_locale_test.go:243` is a historical Phase 13 comment about seven cell rows, not a fixture; left as it is.

## Deviations from Plan

**1. [Rule 3 - blocking, within scope] Two more shared helper lists.** `settingsMreqDescKeys` and `settingsMreqHintKeys` in `settings_mreq_test.go` also had seven entries; the card test panicked at index 7 until they gained the AI keys. The plan names the other helpers but not these two.

**2. [Judgement] The red commit does not compile**, as the plan says it may: `e3becf7` holds the stand-in and the six tests before any production code. Task 1 was committed as test commit, then feature commit; the plan's expected green run is after `d8b2641`.

**3. [Own mistake, fixed] Lint stand-in findings in my tests** (see Verification): an unused `queue` helper, a literal U+200B and unchecked `miniredis.Set` errors. Fixed in `09eb240` before the SUMMARY; the plan-end stand-in run is clean.

**4. Design note.** The held case of `TestMemberReqAISparesExempt` runs on the job path only: on the join path the rejoin reset deletes holds created before the join date before the entry runs, so a seeded hold cannot reach it. Its hold carries the last-name bit only, never bit 128.

No unused identifier is left: `mreqAIDecoded` fields, `received`, `mreqAICanon`, `mreqAIRestartRoot` and the stand-in steps are all read by tests of this plan. `memberReqAIEndpoint`/`memberReqAIHTTP` are production seams used by the stand-in; `Names` is read by `memberReqAIPrepare`.

## Requirements

`requirements-completed: []`. MREQ-01, MREQ-10 and MREQ-14 stay Pending and complete in plan 15-11. REQUIREMENTS.md is unchanged.

## Known Stubs

None. By design (the plan's own words) this is the minimal rule: no baseline or new-member marker (15-05), no names-bound gate, budgets or breaker (15-06), no remedy sentence (15-07), no "AI not configured" status or Off-only form on the page (15-08), no held-release AI call for a hold with bit 128 (the held release passes no `Names`, so the rule is unknown there and the hold stays). The rule is not safe to enable for a real group on its own; the phase ships as one release. In this commit an established member's message job also judges the member, and the watcher does not yet know the AI rule, so a clean-named member is judged only at join (or when a name rule hands them to a job).

## Threat Flags

None new. T-15-10 (no breach on any non-probability), T-15-11 (only the three canonical strings go out, the key only in the header, the cache holds a hash and a probability) and T-15-12 (exempt members are never asked: the executor's exemptions, hold read, pause and live eligibility check come first) are mitigated and tested; T-15-SC: `go.mod` and `go.sum` unchanged.

## Self-Check: PASSED

All three created files exist and the commits e3becf7, d8b2641, 8cb9feb, 09eb240 and 159513f are ancestors of HEAD (checked with `git merge-base --is-ancestor`).
