---
phase: 07-ai-assisted-raid-detection
plan: 07
subsystem: raid-detection
tags: [autolockdown, telegram, i18n, raid-ai, permissions]

requires:
  - phase: 07-ai-assisted-raid-detection
    provides: "raiddetection.SetAIEnabled/SetAIThreshold and the ai_enabled/ai_threshold columns (07-04), raidAIConfigured() (07-06), the AI burst test env (07-01)"
provides:
  - "/autolockdown ai N|on|off: the AI trigger changes with the grammar, range, live authority and anonymous-admin proof of joins and flood"
  - "the /autolockdown status lists the AI trigger and says when the bot has neither the TypeSafe nor the Gemini key"
  - "measured lockdown_help_msg headroom of all 7 locales for plan 07-12"
affects: [07-12 lockdown help text]

actuals:
  tokens: 21000
  tasks: 2
  commits: 4

plan_head_before: 623611b69f994f77d7ee6b76d446608ae9159635
plan_head_after: 5f76d6e844756d6c29af313fd290e83d621db7a3

tech-stack:
  added: []
  patterns:
    - "A new /autolockdown trigger word is one constant, one parse case and one write case; authority is inherited because requireAutoLockdownAuthority decides before the word is parsed"

key-files:
  created: []
  modified:
    - alita/modules/autolockdown.go
    - alita/modules/autolockdown_test.go
    - alita/i18n/lockdown_locale_test.go
    - AGENTS.md
    - locales/en.yml
    - locales/es.yml
    - locales/fr.yml
    - locales/hi.yml
    - locales/id.yml
    - locales/pt.yml
    - locales/ru.yml

key-decisions:
  - "The AI line follows the flood line; the not-set-up notice appears only when raidAIConfigured() is false (neither provider), as D-14 asks"
  - "No new refusal key for ai: it gets exactly the replies joins and flood get, because authority is judged before parsing"
  - "wantAutoAIRow sits beside wantAutoRow so existing callers keep their signature"

requirements-completed: [RAID-03]

coverage:
  - id: D1
    description: "/autolockdown lists the AI trigger (on or off with its threshold) and adds the not-set-up notice only when neither TypeSafe nor Gemini is configured"
    requirement: RAID-03
    verification:
      - kind: unit
        ref: "alita/modules/autolockdown_test.go#TestAutolockdownAI"
        status: pass
    human_judgment: false
  - id: D2
    description: "/autolockdown ai N|on|off changes only the AI trigger with the 3..500 range, live getChatMember authority, anonymous-admin proof, basic-group refusal, and a failed save writes nothing"
    requirement: RAID-03
    verification:
      - kind: unit
        ref: "alita/modules/autolockdown_test.go#TestAutolockdownAI"
        status: pass
      - kind: unit
        ref: "alita/modules/autolockdown_test.go#TestAutoLockdownArgs"
        status: pass
    human_judgment: false
  - id: D3
    description: "The next AI burst follows the new setting after the in-process cache is dropped (threshold 3 locks on 3 verdicts; off locks nothing and counts nothing)"
    requirement: RAID-03
    verification:
      - kind: integration
        ref: "alita/modules/autolockdown_test.go#TestAutolockdownAI/the_next_burst_follows_the_change_on_any_replica"
        status: pass
    human_judgment: false
  - id: D4
    description: "Wording quality of the three new and two changed locale keys in es, fr, hi, id, pt, ru"
    verification: []
    human_judgment: true
    rationale: "Translations were written by the executor and pass the key, placeholder and tag checks, but only a native reader can judge the wording"

duration: 14min
completed: 2026-10-29
status: complete
---

# Phase 7 Plan 07: /autolockdown ai Summary

**`/autolockdown` gains the `ai` word (`ai N|on|off`, 3..500) with the live-authority and anonymous-admin checks it already had for joins and flood, and its status lists the AI trigger and says when the bot has no AI keys.**

## Performance

- **Duration:** 14 min
- **Tasks:** 2 of 2
- **Files modified:** 11 (4 commits: 2 RED test commits, 2 feature commits)

## Accomplishments

- `autoTriggerAI` in `alita/modules/autolockdown.go`: a parse case and a write case (`raiddetection.SetAIThreshold` for a number, `raiddetection.SetAIEnabled` for on/off). The authority check (`requireAutoLockdownAuthority`), the anonymous-admin re-entry and the basic-group refusal are unchanged and run before the word is parsed, so `ai` is refused exactly like `joins` and `flood`.
- The status shows the AI line after the flood line through `autolockdownTriggerLine`, then `lockdown_auto_ai_unavailable` when `raidAIConfigured()` is false. A server with only one of the two providers shows no notice.
- Locale keys in all 7 files: new `lockdown_auto_ai_on`, `lockdown_auto_ai_off`, `lockdown_auto_ai_unavailable`; changed `lockdown_auto_hint` and `lockdown_auto_usage`, which now name `/autolockdown ai N|on|off`. Placeholders are unchanged (`{threshold}`).
- `TestAutolockdownAI` (view and change subtests), new `TestAutoLockdownArgs` rows and the `wantAutoAIRow` helper. The burst subtests run through the real `/aispam` path: `/autolockdown ai 3`, `dbcache.ResetLocalForTest()`, then 3 spam verdicts from 3 accounts lock with `TriggerCount` 3; with `ai off`, 6 verdicts lock nothing and write nothing to the verdict sets.
- AGENTS.md: the `/autolockdown` bullet states the `joins|flood|ai` grammar and the not-set-up notice. `/autoantiraid` stays the joins alias.

## lockdown_help_msg headroom

Measured with `TestLockdownHelpFitsOneMessage -v` (header plus `lockdown_help_msg`, UTF-16 units, limit 4096). `lockdown_help_msg` is not changed by this plan. The values equal the coordinator's earlier measurement, so there is no difference to report.

| Locale | Used | Headroom |
|--------|------|----------|
| en | 3542 | 554 |
| es | 4052 | 44 |
| fr | 3987 | 109 |
| hi | 3878 | 218 |
| id | 4036 | 60 |
| pt | 4065 | 31 |
| ru | 3955 | 141 |

Plan 07-12 has only 31 units in pt and 44 in es to describe the AI trigger in `lockdown_help_msg`.

## Task Commits

1. Task 1 RED: `1241c24` test(07-07): show /autolockdown does not list the AI trigger
2. Task 1 GREEN: `19dc2e0` feat(07-07): show the AI trigger in /autolockdown and say when AI is not set up
3. Task 2 RED: `193abb1` test(07-07): show /autolockdown refuses the ai word
4. Task 2 GREEN: `5f76d6e` feat(07-07): let /autolockdown ai turn the AI trigger on or off and set its threshold

## Deviations from Plan

None - plan executed exactly as written.

Notes, not deviations:
- The Task 2 RED commit fails by compile error (`undefined: autoTriggerAI`) rather than by assertion, because the tests name the new constant.
- The "hint names the ai word" subtest only proves the hint is rendered. That the text names `/autolockdown ai` is shown by the acceptance grep (`grep -c '/autolockdown ai' locales/en.yml` prints 2), not by a test that reads source text, per the repo's testing rule.

## Verification

- `go test -tags testtools -race -count=1 -run '^(TestAutolockdownAI|TestAutoLockdown|TestAutoLockdownArgs|TestAutoLockdownAuthority|TestAutoLockdownBadInput|TestAutoLockdownBasicGroup|TestAutoLockdownSaveFails)$' ./alita/modules`: pass (18 `TestAutolockdownAI` subtests).
- `go test -tags testtools -race -count=1 -run 'AutoLockdown|Autolockdown|AntiRaid|Raid' ./alita/modules`: pass.
- `go test -tags testtools -count=1 -run '^(TestLockdownLocaleKeys|TestLockdownStringsHaveBalancedTags|TestLockdownHelpFitsOneMessage)$' -v ./alita/i18n`: pass, 7 headroom lines.
- `make check-translations` and `make check-docs`: clean.
- `CGO_ENABLED=0 go build ./...`, `go vet -tags testtools ./alita/modules ./alita/i18n`, `gofmt -l` on the touched files: clean (`alita/modules/greetings_command_test.go` is not gofmt-clean on the base commit and was left alone).
- `make lint` could not run here: golangci-lint is built with Go 1.25 and the module is Go 1.26. `go vet` and `gofmt` stood in for it. The orchestrator runs the full `make test` after the wave.

## Threat Model

T-07-25 (a non-admin or demoted admin switching AI off before a raid) is mitigated by the inherited live `getChatMember` check; covered by the "without can_restrict_members", "plain member" and anonymous-admin subtests. T-07-26 (bad threshold stored) by the 3..500 check before any write; covered by the range and bad-words subtests. T-07-27 (admins believing AI runs without keys) by `lockdown_auto_ai_unavailable`; covered by the "no AI keys" subtest.

## Threat Flags

None - no new endpoint, auth path, file access or schema change.

## Known Stubs

None.

## Pending human verification

- Read the new and changed strings in es, fr, hi, id, pt and ru for wording (executor translations).
- In a real supergroup: `/autolockdown` shows the AI line; on a bot without `TYPESAFE_API_KEY` and `GEMINI_API_KEY` it shows the not-set-up notice; `/autolockdown ai 8` and `/autolockdown ai off` save as an admin with the restrict right and are refused for one without it.

## Next Phase Readiness

Ready for the rest of wave 4. Plan 07-12 starts from the headroom table above.

## Self-Check: PASSED

- Modified files exist: `alita/modules/autolockdown.go`, `alita/modules/autolockdown_test.go`, `alita/i18n/lockdown_locale_test.go`, `AGENTS.md`, `locales/{en,es,fr,hi,id,pt,ru}.yml`.
- Commits `1241c24`, `19dc2e0`, `193abb1`, `5f76d6e` exist; `git rev-list --count` from the base is 4.
- All acceptance criteria of both tasks pass (`autoTriggerAI` 5 matches, `lockdown_auto_ai_unavailable` and `raidAIConfigured()` 1 each, 1 per locale, `/autolockdown ai` 2 in en.yml, `joins|flood|ai` in AGENTS.md).
