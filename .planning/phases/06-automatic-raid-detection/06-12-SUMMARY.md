---
phase: 06-automatic-raid-detection
plan: 12
subsystem: docs
tags: [docs, raid-detection, lockdown, antiraid-retirement]

requires:
  - phase: 06-automatic-raid-detection
    provides: plans 06-01 to 06-09 behaviour (triggers, settings table, handler groups, keys) and the 06-10 retirement design
provides:
  - AntiRaid page describing the retired raid mode, the automatic triggers with defaults, /autolockdown, the /autoantiraid alias and the carry-over
  - reference pages (commands, callbacks, database schema, environment) matching the new behaviour
  - backup page stating that /autolockdown settings are not carried (D-20)
  - architecture and self-hosting pages showing handler groups -7 and -4, the alita:raid:* keys and the new files
affects: [06-13 docs regeneration]

actuals:
  tokens: 9500
  tasks: 2
  commits: 2
plan_head_before: d1ab55c1beee7a89271f564a6bb0167ec3f5d7bb
commits: 2
plan_head_after: bde7fd8767cadaa33121a276757cc1b34b1646c3

tech-stack:
  added: []
  patterns: []

key-files:
  created: []
  modified:
    - docs/src/content/docs/commands/antiraid/index.md
    - docs/src/content/docs/api-reference/commands.md
    - docs/src/content/docs/api-reference/callbacks.md
    - docs/src/content/docs/api-reference/database-schema.md
    - docs/src/content/docs/api-reference/environment.md
    - docs/src/content/docs/commands/backup/index.md
    - docs/src/content/docs/commands/index.mdx
    - docs/src/content/docs/architecture/handler-groups.md
    - docs/src/content/docs/architecture/caching.md
    - docs/src/content/docs/architecture/request-flow.mdx
    - docs/src/content/docs/architecture/project-structure.mdx
    - docs/src/content/docs/self-hosting/database.md

key-decisions:
  - "Dropped the Disableable column from the hand-written AntiRaid page rather than assert disableability of the pointer commands that plan 06-10 builds"
  - "Marked /raidtime and /raidactiontime Disableable in commands.md because plan 06-10 registers them as aliases of the /antiraid descriptor"

requirements-completed: [LOCK-15, RAID-06]

duration: 15min
completed: 2026-10-18
status: complete
---

# Phase 6 Plan 12: Raid detection docs Summary

**Twelve docs pages now describe the retired raid mode, the join surge and flood triggers with their defaults and tuning, the new settings table and Redis keys, and the new handler groups -7 and -4.**

## Performance

- **Duration:** about 15 min
- **Tasks:** 2
- **Files modified:** 12 (no code, config, locale or generated page touched)

## Accomplishments

- Task 1 (commit 7ef517e): the six frozen pages, edited by hand with the MANUALLY MAINTAINED marker kept.
  - `commands/antiraid/index.md` rewritten: what replaced the raid mode, defaults (15 joins in 60 s; 30 items in 30 s, 60% from members who joined in the last 24 h, at least 3 accounts), what never counts, the notice, alert, 10-minute quiet period, once-a-day cannot-lock warning, Redis-only counting, `/autolockdown` grammar and live authority, the retired `/antiraid`, `/raidtime`, `/raidactiontime`, the `/autoantiraid` alias, and the carry-over (clamped 3..500, off became on at 15).
  - `api-reference/commands.md` (retired and alias rows plus a new `/autolockdown` row in both tables), `callbacks.md` (retired button answered with the pointer), `database-schema.md` (new `chat_raid_detection` section, `chat_lockdowns` trigger columns, `antiraid_settings` marked legacy), `environment.md` (`CLEAR_CACHE_ON_STARTUP` names the kept operational keys).
  - `commands/backup/index.md`: backups do not carry `/autolockdown` settings, the AntiRaid module's old `antiraid_settings` no longer controls anything (D-20).
- Task 2 (commit bde7fd8): the hand-written pages.
  - `handler-groups.md`: antiraid row removed, rows for -7 and -4 added, prose and both worked examples updated.
  - `caching.md`: `raid_detection` settings key and the `alita:raid:*` keys replace the antiraid keys; `skipLocal` and `CLEAR_CACHE_ON_STARTUP` prose updated.
  - `request-flow.mdx`, `project-structure.mdx` (new file list and `raiddetection` repository), `self-hosting/database.md` (`chat_raid_detection` row, `antiraid_settings` legacy), `commands/index.mdx` (AntiRaid card says retired and points to Lockdown).

## Verification

- Marker check over the six frozen pages: no "marker lost" line.
- `make check-docs`: "No drift detected" after each task.
- Acceptance greps: `autolockdown` in the AntiRaid page 7, `chat_raid_detection` in database-schema 3, `autolockdown` in backup 2, `| -4 ` 1, `| -7 ` 1, `chat_raid_detection` in self-hosting/database 1, `alita:raid:` in caching 7.
- `git diff --exit-code go.mod go.sum`: clean.

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

- Plan 06-10 runs in parallel and is not in this worktree, so the pointer-command text and the Disableable flags follow its plan, not its code. Plan 06-13 (or the verifier) should confirm the Disableable cells of `/raidtime` and `/raidactiontime` in `api-reference/commands.md` and the "4 commands" count on the AntiRaid card once 06-10 is merged.
- `commands.md` totals ("159 commands") were already stale (no `/lockdown` rows) and were left alone.

## Known Stubs

None.

## Threat Flags

None. Docs only; T-06-36 mitigated by stating every retired command, the alias authority and the backup gap.

## Next Phase Readiness

Ready for 06-13, which regenerates the pages built from `locales/en.yml`.

## Self-Check: PASSED

- All twelve files exist and are modified in commits 7ef517e and bde7fd8.
- Both commits are present in the branch log.
