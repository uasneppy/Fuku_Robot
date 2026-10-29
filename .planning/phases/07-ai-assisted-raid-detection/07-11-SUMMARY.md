---
phase: 07-ai-assisted-raid-detection
plan: 11
subsystem: docs
tags: [docs, architecture, raid-detection, ai, gemini, typesafe, redis]

requires:
  - phase: 07-ai-assisted-raid-detection
    provides: the raid AI watcher, pool, counters, settings columns and ai_burst kind (plans 07-01 to 07-09, 07-14)
provides:
  - architecture pages showing handler group 2, the raid AI pool, every new alita:raid:* AI key and the new code locations
  - reference pages showing ai_enabled / ai_threshold, the ai_burst trigger kind, /autolockdown ai
  - aispam page telling admins what goes to TypeSafe and Google Gemini and that AI never locks alone
affects: [07-12 generated lockdown help text, phase verification]

actuals:
  tokens: 5200
  tasks: 2
  commits: 2
plan_head_before: cb5b0e497854f85512cb807a01c17e74ab451db3
plan_head_after: a51d83292b328793800c0533ca33d23bcecc7333

tech-stack:
  added: []
  patterns: ["Docs state only what the shipped code and AGENTS.md say; names and numbers copied from source"]

key-files:
  created: []
  modified:
    - docs/src/content/docs/architecture/handler-groups.md
    - docs/src/content/docs/architecture/request-flow.mdx
    - docs/src/content/docs/architecture/caching.md
    - docs/src/content/docs/architecture/project-structure.mdx
    - docs/src/content/docs/self-hosting/database.md
    - docs/src/content/docs/api-reference/database-schema.md
    - docs/src/content/docs/api-reference/commands.md
    - docs/src/content/docs/commands/aispam/index.md

key-decisions:
  - "The aispam page says the raid text check sends the /aispam data minus the sender's recent messages, because raidAIProcessText builds the state with no history"
  - "Generated pages (commands/antiraid, lockdown) were left alone: they come from the help text in plan 07-12"

requirements-completed: [RAID-03, RAID-04, PLAT-02]

coverage:
  - id: D1
    description: "Architecture pages show handler group 2 (raidAIWatcher), the AI-never-decides-alone note, the group 2 walkthrough steps, and group 2 in the request-flow list"
    requirement: RAID-03
    verification:
      - kind: other
        ref: "make check-docs; grep -c raidAIWatcher architecture/handler-groups.md"
        status: pass
    human_judgment: false
  - id: D2
    description: "caching.md lists av, aa, aispammers, ar, ag, ab, aibudget and aiv keys with lifetimes; project-structure.mdx lists raid_ai*.go, alita/utils/gemini and scripts/gemini-spike"
    requirement: PLAT-02
    verification:
      - kind: other
        ref: "grep acceptance criteria of task 1; make check-docs; CGO_ENABLED=0 go build ./..."
        status: pass
    human_judgment: false
  - id: D3
    description: "Reference pages show ai_enabled/ai_threshold with defaults and CHECK, the ai_burst kind, /autolockdown ai, the D-19 note and the D-15/D-21 data notes"
    requirement: RAID-04
    verification:
      - kind: other
        ref: "grep acceptance criteria of task 2; make check-docs; make check-translations"
        status: pass
    human_judgment: false
  - id: D4
    description: "Accuracy and plain-language quality of what the aispam page says is sent to Google and TypeSafe"
    verification: []
    human_judgment: true
    rationale: "Wording read by group admins deciding whether to keep the trigger; automation can only check the facts against code, not whether the explanation is clear"

duration: 14min
completed: 2026-10-29
status: complete
---

# Phase 7 Plan 11: Documentation of the AI-assisted raid detection Summary

**Hand-maintained docs now cover handler group 2, the raid AI pool and its Redis keys, the `ai_enabled`/`ai_threshold` columns, the `ai_burst` kind, `/autolockdown ai`, and plainly state what goes to Google Gemini and TypeSafe.**

## Performance

- **Duration:** about 14 min
- **Tasks:** 2
- **Files modified:** 8 (docs only, no Go code, no AGENTS.md)

## Accomplishments

- Task 1 (`62b69f6`): `handler-groups.md` gets the group 2 row for `raidAIWatcher`, a note that the watcher never blocks and a note that AI never decides alone (at least N verdicts from at least 3 accounts within 120 s, N defaulting to 5), and group 2 steps in two walkthroughs. `request-flow.mdx` lists group 2 before aispam. `caching.md` gains rows for `:av`/`:aa`, `:aispammers`, `:ar:{userId}`, `:ag:{mediaGroupId}`, `:ab:{hour}` with `alita:raid:aibudget:{hour}`, and `alita:raid:aiv:{fileUniqueId}`, each with its lifetime (checked against `raid_ai*.go`). `project-structure.mdx` lists the five `raid_ai*.go` files, `alita/utils/gemini/` and `scripts/gemini-spike/`.
- Task 2 (`a51d832`): `database-schema.md` documents `ai_enabled` (TRUE) and `ai_threshold` (5, `chk_chat_raid_detection_ai` 3..500), migration `20261029120000`, the pre-migration cached value reading as on at 5, and the `ai_burst` kind (count = verdict items, window 120, percent 0, no migration needed). `database.md` and `commands.md` name the AI trigger and `ai`. `aispam/index.md` has a new "AI Spam And Raid Protection" section (delete verdicts feed the trigger, never locks alone, raid text checks via TypeSafe, images via Gemini, nothing sent without keys, paid-tier Gemini key recommended, `/autolockdown ai off`) and `GEMINI_API_KEY` in the operator table.

## Task Commits

1. Task 1: `62b69f6` docs(07-11): describe the AI candidate watcher, the raid AI pool and its keys
2. Task 2: `a51d832` docs(07-11): document the AI trigger settings and what the AI checks send

## Deviations from Plan

None - plan executed exactly as written.

Notes: the `commands/antiraid` and `commands/lockdown` pages are generated from help text (plan 07-12) and were not touched. Handler group 12 (late flood watcher) is still missing from the handler-groups table; it predates this plan and is out of scope.

## Verification

- `make check-docs`: no drift. `make check-translations`: all present. `CGO_ENABLED=0 go build ./...`: exit 0.
- Task 1 acceptance: `raidAIWatcher` 1, `alita:raid:aibudget` 1, `aiv` 1, `gemini-spike` 1, `raid_ai.go` 1 (all pass).
- Task 2 acceptance: `ai_threshold` 2, `ai_burst` 3, `Gemini` 4, `TypeSafe` 3, MANUALLY MAINTAINED marker present in aispam, database-schema and commands pages (all pass).

## Pending human verification

- A person should read the new aispam section as an admin would and confirm it is clear (coverage item D4).
- Live checks (real Gemini/TypeSafe calls, a real lockdown from the AI trigger) cannot run in this environment and are not part of this docs-only plan.

## Known Stubs

None.

## Threat Flags

None.

## Self-Check: PASSED

- All eight modified pages exist and carry the described text; commits `62b69f6` and `a51d832` exist in the log.
