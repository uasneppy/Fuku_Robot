---
phase: 06-automatic-raid-detection
plan: 09
subsystem: lockdown
tags: [raid-detection, flood, lockdown-alert, burst, redis-lua, i18n]

requires:
  - phase: 06-automatic-raid-detection
    provides: the flood counter sets (:fa, :joined) and raidFire (06-04 to 06-07)
  - phase: 05-lockdown-alerts-and-response
    provides: lockdownBurstSnapshot, BeginAlert's burst rows, BurstSampleFresh and the Ban N recent joiners run (05-07, 05-10)
provides:
  - raidFloodersScript, raidSnapshotFlooders, readRaidFlooders and the raidFloodersLookup seam in raid_flood.go
  - raidFire copies the flooding accounts into alita:raid:{chat}:flooders when a flood trigger fires
  - lockdownMergeFlooders: a flood lockdown's burst rows hold the flooders (named from the users table) beside the tracker's joins
  - user.FindUsersByIDs, one uncached query for many users' names
  - lockdown_alert_counts_flood in all 7 locales; lockdownRenderSample takes the trigger kind
affects: [06-10, 06-13]

actuals:
  tokens: 7000
  tasks: 2
  commits: 3

plan_head_before: 3624d28a89d6ee2f7d697c360ebc1df5e312b66d
plan_head_after: 32fdf08ab070115ae1267a7998df82a481214dab

commits: 3

tech-stack:
  added: []
  patterns:
    - "A short-lived Redis snapshot taken at the instant a trigger fires is read once by the worker and saved to PostgreSQL with the alert, so nothing later needs Redis"
    - "Merged burst rows: the tracker's entry wins for name, username, link and bot flag, the earliest time wins, missing names come from one users query"

key-files:
  created:
    - alita/modules/raid_flood_burst_test.go
    - alita/db/user/find_by_ids_test.go
  modified:
    - alita/modules/raid_flood.go
    - alita/modules/raid_trigger.go
    - alita/modules/lockdown_alert.go
    - alita/db/user/repository.go
    - AGENTS.md
    - locales/en.yml
    - locales/es.yml
    - locales/fr.yml
    - locales/hi.yml
    - locales/id.yml
    - locales/pt.yml
    - locales/ru.yml

key-decisions:
  - "The flooders snapshot is taken in raidFire after the latch, the active check and the quiet period and before startLockdownRow, so a trigger that stays silent never overwrites it"
  - "The snapshot is saved as pre burst rows, so the CHECK on source needs no migration and Phase 5's ban run, link line and counts read it unchanged"
  - "A flooder the tracker also saw keeps the tracker's name, username, link and bot flag; the earlier join time wins"

patterns-established:
  - "lockdownRenderSample picks its counts key from the trigger kind; surge and manual alerts keep lockdown_alert_counts"

requirements-completed: [RAID-02]

coverage:
  - id: D1
    description: "A flood lockdown's alert lists the recently joined accounts that flooded, with escaped name, @username and ID, even when they joined hours before the lock; an established member is not listed"
    requirement: RAID-02
    verification:
      - kind: integration
        ref: "alita/modules/raid_flood_burst_test.go#TestRaidFloodBurst/flooders_listed"
        status: pass
    human_judgment: false
  - id: D2
    description: "Ban N recent joiners on a flood alert counts and bans the flooding accounts for good, leaving the established member and the bot alone"
    requirement: RAID-02
    verification:
      - kind: integration
        ref: "alita/modules/raid_flood_burst_test.go#TestRaidFloodBurst/ban_the_flooders"
        status: pass
    human_judgment: false
  - id: D3
    description: "A flood alert's counts line is lockdown_alert_counts_flood; a join-surge alert keeps lockdown_alert_counts and never reads the flooders snapshot"
    requirement: RAID-02
    verification:
      - kind: integration
        ref: "alita/modules/raid_flood_burst_test.go#TestRaidFloodBurst/surge_alert_unchanged"
        status: pass
    human_judgment: false
  - id: D4
    description: "The flooders are snapshotted when the trigger fires and saved to PostgreSQL with the alert; a lost snapshot still posts the alert with the tracker's joins, unknown names list by ID, and approved users, admins and the bot are never saved"
    requirement: RAID-02
    verification:
      - kind: integration
        ref: "alita/modules/raid_flood_burst_test.go#TestRaidFloodBurst/snapshot_gone"
        status: pass
      - kind: integration
        ref: "alita/modules/raid_flood_burst_test.go#TestRaidFloodBurst/names_unknown"
        status: pass
      - kind: integration
        ref: "alita/modules/raid_flood_burst_test.go#TestRaidFloodBurst/exempt_senders_and_the_bot_are_not_saved"
        status: pass
    human_judgment: false
  - id: D5
    description: "user.FindUsersByIDs reads many users in one uncached query, ignores unknown IDs, returns an empty map for no input and returns database errors"
    verification:
      - kind: unit
        ref: "alita/db/user/find_by_ids_test.go#TestFindUsersByIDs"
        status: pass
    human_judgment: false
  - id: D6
    description: "lockdown_alert_counts_flood reads naturally in es, fr, hi, id, pt and ru"
    verification:
      - kind: unit
        ref: "alita/i18n/lockdown_locale_test.go#TestLockdownLocaleKeys (parity, placeholders)"
        status: pass
    human_judgment: true
    rationale: "Tests prove the key exists with the same placeholders in every locale; only a native reader can judge the wording of the six translations"

duration: 14min
completed: 2026-10-18
status: complete
---

# Phase 6 Plan 09: Flood alert names the flooders Summary

**When the flood trigger fires, the accounts that flooded are snapshotted in Redis with their join times, saved to PostgreSQL as the lockdown's burst rows when the alert is decided, and listed on the alert and banned by "Ban N recent joiners" even when they joined hours earlier.**

## Performance

- **Duration:** 14 min
- **Started:** 2026-10-18T13:35:00Z
- **Completed:** 2026-10-18T13:49:00Z
- **Tasks:** 2
- **Files modified:** 14 (2 created)

## Accomplishments
- `raidFire`, for a flood trigger, runs `raidFloodersScript` once after the latch, active check and quiet period and before `startLockdownRow`: the newest 500 accounts of `:fa` whose `:joined` score is within 24 h are stored, scored by join time, with a 15-minute TTL. A failed snapshot is logged at debug and the lockdown still starts.
- `lockdownBurstSnapshot` merges the snapshot (read through the `raidFloodersLookup` seam, only for a flood lockdown) with the tracker's joins by user ID, names the rest with one `user.FindUsersByIDs` query, skips the bot, and `lockdown.BeginAlert`'s winner saves them as `pre` burst rows. The Phase 5 sample, link line and ban run are unchanged and see them.
- A flood alert's counts line is `lockdown_alert_counts_flood`; surge and manual alerts keep `lockdown_alert_counts`.
- `TestRaidFloodBurst` covers six cases; `TestFindUsersByIDs` covers three.

## Task Commits

1. **Task 1: a flood alert lists the flooding accounts** (TDD)
   - RED: `a73c510` (test) - the alert showed `lockdown_alert_counts 0 0` and no flooder, 0 burst rows; `TestFindUsersByIDs` failed on the scaffold
   - GREEN: `cba89cc` (feat)
2. **Task 2: Ban N recent joiners bans the flooders; edges** - `32fdf08` (test only; Task 1's code passed every case, mutation-checked as the plan allows)

**Plan metadata:** the commit that adds this file (docs: complete plan)

## TDD Gate Compliance

- Task 1: RED (`a73c510`) precedes GREEN (`cba89cc`). The RED commit carries compile scaffolding: `user.FindUsersByIDs` returning an empty map, so `TestFindUsersByIDs` builds and fails on its assertions. The RED failures were assertion failures on the planned behavior (verified from the test output); `gsd_run check tdd-red-evidence` was not run.
- Task 2: no failing-first commit exists, because Task 1's implementation already satisfied every Task 2 case. The plan names this outcome. Mutation checks were run instead and failed as expected: skipping `lockdownMergeFlooders` fails `flooders listed`, `ban the flooders`, `names unknown` and `exempt senders and the bot are not saved`; merging for every trigger kind fails `surge alert unchanged`. Both mutations were reverted before the commit.

## Files Created/Modified
- `alita/modules/raid_flood.go` - `raidPartFlooders`, `raidFloodersTTL` (15 min), `raidFloodersMax` (500), `raidFloodersScript`, `raidSnapshotFlooders`, `readRaidFlooders`, `raidFloodersLookup`
- `alita/modules/raid_trigger.go` - the snapshot call in `raidFire` for a flood trigger
- `alita/modules/lockdown_alert.go` - flood branch of `lockdownBurstSnapshot`, `lockdownMergeFlooders`, `lockdownRenderSample(..., kind)`
- `alita/db/user/repository.go` - `FindUsersByIDs`
- `alita/modules/raid_flood_burst_test.go`, `alita/db/user/find_by_ids_test.go` - tests
- `locales/*.yml` (7) - `lockdown_alert_counts_flood`
- `AGENTS.md` - the `alita:raid:{<chat>}:flooders` rule, in the same commit as the code

## Decisions Made
- Followed the plan's recorded decisions. One detail the plan left open: the merged list is sorted by join time then user ID, so the burst rows are inserted in a stable order; names and usernames the users table returns are capped with `lockdownNameMaxRunes` and `lockdownRecentUsernameMaxRunes`.
- The number of burst rows can reach 1000 (500 from the tracker, 500 flooders); the alert still lists 10.

## Deviations from Plan

None - plan executed exactly as written. The Task 2 test commit is test-only, which the plan allows.

## Issues Encountered

- `make lint` cannot run here (golangci-lint cannot lint Go 1.26). `gofmt -l alita/modules alita/db/user` prints nothing and `go vet -tags testtools ./alita/modules ./alita/db/user ./alita/db/lockdown ./alita/i18n` is clean.
- No PostgreSQL server is available and none was started; all tests ran on SQLite and miniredis. This plan adds no new conditional-update guarantee that needs `make test-postgres-integrity`.
- `make test` was not run (the orchestrator runs it). `make check-docs` was not run; no command or generated-docs source changed.

## Verification run

- `go test -tags testtools -race -count=1 -run '^TestRaidFloodBurst$|^TestLockdownBanRecentJoiners$|^TestLockdownAlertSample$' ./alita/modules` - pass
- `go test -tags testtools -race -count=1 -run 'Lockdown|Unlockdown|Raid' ./alita/modules` - pass
- `go test -tags testtools -race -count=1 ./alita/db/user ./alita/db/lockdown` - pass
- `go test -tags testtools -count=1 -run '^TestLockdownLocaleKeys$' ./alita/i18n` and `make check-translations` - pass
- `CGO_ENABLED=0 go build ./...` - pass; `git diff --exit-code go.mod go.sum` - unchanged

## Threat Flags

None. T-06-27 (HTML in names): names, usernames and IDs go through the Phase 5 sample renderer, escaped and spliced after the translator; `flooders listed` uses names with tags. T-06-28 (Ban N hitting an admin, approved or established member): only accounts that posted counted items enter the snapshot, and `exempt senders and the bot are not saved` plus `ban the flooders` cover it. T-06-29 (oversized snapshot): 500 accounts, 15-minute TTL, one names query.

## Known Stubs

None.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- Ready for 06-10. No blockers.

---
*Phase: 06-automatic-raid-detection*
*Completed: 2026-10-18*

## Self-Check: PASSED

- Created files found: `alita/modules/raid_flood_burst_test.go`, `alita/db/user/find_by_ids_test.go`.
- Commits found: `a73c510`, `cba89cc`, `32fdf08`.
- Acceptance criteria re-run: `raidSnapshotFlooders(` in `raid_trigger.go`, `raidFloodersLookup(` in `lockdown_alert.go`, `func FindUsersByIDs(`, `raidFloodersMax = 500`, the key present once in each of the 7 locale files, `go.mod` and `go.sum` unchanged.
