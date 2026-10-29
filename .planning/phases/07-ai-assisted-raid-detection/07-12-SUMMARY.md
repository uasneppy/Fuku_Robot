---
phase: 07-ai-assisted-raid-detection
plan: 12
subsystem: docs-i18n
tags: [i18n, lockdown, help-text, agents-md, ai-spam, gemini, typesafe]

requires:
  - phase: 07-ai-assisted-raid-detection
    provides: "plans 07-01 to 07-11 and 07-14: the AI trigger, its counters, pool, image path, settings and stats"
provides:
  - "lockdown_help_msg in all 7 locales describes the AI trigger and /autolockdown [joins|flood|ai]"
  - "regenerated /lockdown docs page"
  - "AGENTS.md checked line by line against the shipped Phase 7 code"
affects: [phase-09-settings-menu, any later change to lockdown_help_msg]

actuals:
  tokens: 21000
  tasks: 2
  commits: 2
plan_head_before: a8d43bc99c8a2220647757fd66a3f35fd9de6402
plan_head_after: da0f5dc87d980fdbf4dfd6be9dc9264cd2b49672

tech-stack:
  added: []
  patterns: ["help text length held by TestLockdownHelpFitsOneMessage (UTF-16 units, total at most 4076)"]

key-files:
  created: []
  modified:
    - locales/en.yml
    - locales/es.yml
    - locales/fr.yml
    - locales/hi.yml
    - locales/id.yml
    - locales/pt.yml
    - locales/ru.yml
    - docs/src/content/docs/commands/lockdown/index.md
    - AGENTS.md

key-decisions:
  - "English AI paragraph is the plan's 273-character wording, placed after the automatic-trigger paragraph"
  - "fr, hi and ru also needed the five named shortenings (the plan expected only es, pt, id and maybe fr)"
  - "es, pt and id needed further rewording beyond the five named groups, never dropping a fact"

requirements-completed: [RAID-03, RAID-04, RAID-05, PLAT-02]

coverage:
  - id: D1
    description: "The /lockdown help in all 7 languages names the AI trigger, what goes to TypeSafe and Google Gemini, the 5-verdicts-from-3-accounts-in-120-s rule, that one verdict never locks and AI never deletes or bans, and /autolockdown ai"
    requirement: "RAID-05"
    verification:
      - kind: unit
        ref: "alita/i18n/lockdown_locale_test.go#TestLockdownHelpFitsOneMessage"
        status: pass
      - kind: unit
        ref: "alita/i18n/lockdown_locale_test.go#TestLockdownLocaleKeys"
        status: pass
      - kind: unit
        ref: "alita/i18n/lockdown_locale_test.go#TestLockdownStringsHaveBalancedTags"
        status: pass
    human_judgment: true
    rationale: "Six translations need a native-speaker read; the facts were kept but the wording was compressed to fit one Telegram message"
  - id: D2
    description: "The generated /lockdown docs page matches the help text"
    requirement: "PLAT-02"
    verification:
      - kind: other
        ref: "make check-docs"
        status: pass
    human_judgment: false
  - id: D3
    description: "AGENTS.md states every Phase 7 rule once and matches the shipped code"
    verification:
      - kind: other
        ref: "grep -cF of the 13 acceptance identifiers in AGENTS.md; CGO_ENABLED=0 go build ./...; go vet -tags testtools ./..."
        status: pass
    human_judgment: false

duration: 45min
completed: 2026-10-29
status: complete
---

# Phase 7 Plan 12: AI trigger help text and the final AGENTS.md pass Summary

**The `/lockdown` help now tells admins in all 7 languages what the AI trigger sends to TypeSafe and Google Gemini, the 5-verdicts-from-3-accounts-in-120-s rule and that AI never locks on one verdict or deletes and bans; AGENTS.md was re-checked against the Phase 7 code and the missing stats, defaults and testing rules were added.**

## Performance

- **Duration:** about 45 min
- **Tasks:** 2 of 2
- **Files modified:** 9

## Accomplishments

- `lockdown_help_msg` (en, es, fr, hi, id, pt, ru): the `/autolockdown` bullet is `[joins|flood|ai] [N|on|off]` and a new "AI trigger" paragraph follows the automatic-trigger paragraph. English is 273 characters.
- To fit one message, existing sentences were reworded (no behaviour, exception, number or command dropped): the authority sentences of the Lift and Ban paragraphs now say "same rights as /unlockdown", the two opening sentences of the button paragraph became one, the defaults sentence, the 330-day sentence pair and the Revoke paragraph's last sentence were shortened. The reminder sentence and a few "restrict members" repeats were shortened further in es, pt, id.
- `make generate-docs` regenerated `docs/src/content/docs/commands/lockdown/index.md`; `make check-docs` reports no drift.
- AGENTS.md: the rest of Phase 7 (handler group 2, `StopRaidAI`, every `alita:raid:*` AI key, settings columns, `ai_burst`, the leaf Gemini package, the token-safe download, budgets, breakers, "AI never decides alone") was verified against the source and was already correct. Added: the developer `/stats` AI lines (`AI spam off`, `raidAIStatsSection`), the exact condition of the not-set-up notice, the `GEMINI_API_KEY` and `GEMINI_MODEL` defaults, the pointer to the help text and its length test, and the testing rule that no test calls a real AI provider.

## Help length, final (UTF-16 units of help header plus lockdown_help_msg, limit 4096)

| Locale | Before | After | Headroom after |
|--------|--------|-------|----------------|
| en | 3542 | 3820 | 276 |
| es | 4052 | 4065 | 31 |
| fr | 3987 | 4064 | 32 |
| hi | 3878 | 3968 | 128 |
| id | 4036 | 4062 | 34 |
| pt | 4065 | 4070 | 26 |
| ru | 3955 | 4006 | 90 |

Every locale is at 4076 units or fewer (headroom 20 or more).

## Task Commits

1. **Task 1: AI trigger in the lockdown help, 7 locales, regenerated docs** - `3f3c6a8` (docs)
2. **Task 2: AGENTS.md pass** - `da0f5dc` (docs)

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] fr, hi and ru also exceeded the budget**
- **Found during:** Task 1, edit group 1
- **Issue:** with the translated AI paragraph, fr was 238 units, hi 105 and ru 170 over 4076; the plan expected only es, pt, id (and maybe fr) to need shortening.
- **Fix:** applied the same five named shortenings (rewording only) to fr, hi and ru.
- **Files modified:** locales/fr.yml, locales/hi.yml, locales/ru.yml
- **Commit:** 3f3c6a8

**2. [Rule 3 - Blocking] es, pt, id still over after the five named shortenings and a terser AI paragraph**
- **Found during:** Task 1, edit group 2
- **Issue:** after the five groups and the terser AI paragraph the totals were still 63 (es), 55 (pt) and 51 (id) units over.
- **Fix:** more rewording of existing sentences with the same facts: "that permission" instead of repeating "restrict members" in the two places each language repeats it, reminders "until the lift", "excludes them" in id, "never count" order in es. No fact, number or command dropped. pt has no "both triggers are on by default" sentence, so its group 3 had nothing to shorten.
- **Files modified:** locales/es.yml, locales/pt.yml, locales/id.yml
- **Commit:** 3f3c6a8

**Total deviations:** 2 auto-fixed (both Rule 3). **Impact:** wording only; the help is slightly terser in six locales. Translation of the AI paragraph in es, pt and id is terser than the English where it needed to be (the 280 cap applies to English only); all facts are kept: who is checked, 5 verdicts from 3 accounts in 120 s, one verdict never locks, AI never deletes or bans, and "if set up".

## Verification

- `TestLockdownHelpFitsOneMessage`, `TestLockdownLocaleKeys`, `TestLockdownStringsHaveBalancedTags`, `TestLockdownWarningLines`: pass
- `make check-translations`: all translations present; `make check-docs`: no drift
- `CGO_ENABLED=0 go build ./...` and `go vet -tags testtools ./...`: clean; `gofmt -l alita scripts main.go`: nothing; `go.mod` and `go.sum` unchanged
- `make lint` could not run here (golangci-lint is built with Go 1.25, the module is Go 1.26); `gofmt` and `go vet` stand in for it. The full `make test` is left to the orchestrator after the wave.
- No PostgreSQL was started.

## Known Stubs

None.

## Threat Flags

None. Help text and documentation only.

## Pending human verification

- A native-speaker read of the six translations (es, fr, hi, id, pt, ru): the AI paragraph and the reworded sentences, especially pt (26 units of headroom) and hi/ru where the wording was compressed.
- A real `/lockdown help` in pt (and the other locales) in Telegram to confirm the help shows as one message with the tags rendered (live check cannot run here).
- Owner decision, unchanged from earlier plans: the live Gemini spike (`go run ./scripts/gemini-spike`, needs `GEMINI_API_KEY`).

## Next Phase Readiness

Phase 7 words and rules are closed. Phase 9's settings menu can build on the AGENTS.md rules; any new text added to `lockdown_help_msg` must keep every locale at 4076 units or fewer (pt has 26 units of headroom).

## Self-Check: PASSED

- locales/en.yml, es.yml, fr.yml, hi.yml, id.yml, pt.yml, ru.yml, docs/src/content/docs/commands/lockdown/index.md and AGENTS.md exist and are committed.
- Commits 3f3c6a8 and da0f5dc exist in the branch.
