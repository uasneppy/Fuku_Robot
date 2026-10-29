---
phase: 07-ai-assisted-raid-detection
plan: 10
subsystem: developer-stats
tags: [stats, raid-detection, ai-spam, observability]

requires:
  - phase: 07-ai-assisted-raid-detection
    provides: "ai_enabled column (07-04), GetRaidAIStats and RaidAIStats counters (07-06, 07-08, 07-09)"
provides:
  - "LoadRaidDetectionStats returns aiOff, the chats with ai_enabled false"
  - "AI spam off line under Raid detection in the developer /stats"
  - "raidAIStatsSection: this replica's raid AI and /aispam counters appended to /stats"
affects: [07-11 docs, developer /stats]

actuals:
  tokens: 3546
  tasks: 2
  commits: 4

plan_head_before: cb5b0e497854f85512cb807a01c17e74ab451db3
plan_head_after: 3e36a9199d07d313bc5ca33b48db8a46bb64dd92

tech-stack:
  added: []
  patterns:
    - "In-memory counters are appended in alita/modules because alita/db/devs cannot import alita/modules"

key-files:
  created:
    - alita/modules/raid_ai_stats_test.go
  modified:
    - alita/db/raiddetection/stats.go
    - alita/db/raiddetection/stats_test.go
    - alita/db/devs/repository.go
    - alita/db/devs/repository_test.go
    - alita/modules/devs.go

key-decisions:
  - "LoadRaidDetectionStats returns (configured, joinOff, floodOff, aiOff); the new line is '<b>AI spam off:</b> N chats' after Flood off, not a locale key (the /stats text is developer-only and not localized)"
  - "raidAIStatsSection prints numbers only, under a heading that says 'this replica, since its last restart', so per-process counters are not read as fleet totals"

requirements-completed: [RAID-04, RAID-03]

coverage:
  - id: D1
    description: "The developer /stats counts the groups that turned the AI trigger off beside the join surge and flood counts"
    requirement: RAID-03
    verification:
      - kind: unit
        ref: "alita/db/raiddetection/stats_test.go#TestLoadRaidDetectionStats"
        status: pass
      - kind: unit
        ref: "alita/db/devs/repository_test.go#TestLoadAllStats_IncludesModuleCounts"
        status: pass
    human_judgment: false
  - id: D2
    description: "The developer /stats shows this replica's raid AI counters (checked, spam, kept, shed, failed, blocked, capped, cached) and /aispam's (checked, deleted, kept, shed, failed) under a per-replica heading, with no message text, file ID or key"
    requirement: RAID-04
    verification:
      - kind: unit
        ref: "alita/modules/raid_ai_stats_test.go#TestRaidAIStats"
        status: pass
      - kind: unit
        ref: "alita/modules/raid_ai_stats_test.go#TestRaidAIStatsDevsOnly"
        status: pass
    human_judgment: false

duration: 6min
completed: 2026-10-29
status: complete
---

# Phase 7 Plan 10: Developer /stats for the AI checks Summary

**The developer `/stats` now counts the groups that switched the AI trigger off and shows this replica's raid AI and `/aispam` counters under a "this replica" heading, as numbers only.**

## Performance

- **Duration:** about 6 min
- **Completed:** 2026-10-29T20:15Z
- **Tasks:** 2
- **Files modified:** 6 (1 created)

## Accomplishments

- `raiddetection.LoadRaidDetectionStats` returns a fourth count, `aiOff` (`ai_enabled = false`), and `devs.LoadAllStats` prints "AI spam off: N chats" right after "Flood off".
- `raidAIStatsSection` in `alita/modules/devs.go` prints one raid-path line (checked, spam, kept, shed, failed, blocked, capped, cached) and one `/aispam` line (checked, deleted, kept, shed, failed). `getStats` sends `devs.LoadAllStats() + raidAIStatsSection()`. `GetAISpamStats` has its first consumer.
- `TestRaidAIStats` drives the real dispatcher in one group of new members: a spam text, a kept text, a text whose TypeSafe call returns 500, an image judged spam, the same image from another member (verdict cache), a blocked image and an image over the lowered hourly budget, then one `/aispam` delete verdict. The owner's `/stats` edit then carries "checked 3, spam 3, kept 1, shed 0, failed 1, blocked 1, capped 1, cached 1" and "checked 1, deleted 1, kept 0, shed 0, failed 0", and none of the message text, file IDs or keys. `TestRaidAIStatsDevsOnly` shows a non-developer's `/stats` sends and edits nothing.

## Task Commits

1. Task 1 RED: `83315e4` test(07-10): show /stats does not count the AI trigger
2. Task 1 GREEN: `f605c41` feat(07-10): count groups with the AI trigger off in developer stats
3. Task 2 RED: `741b108` test(07-10): show /stats has no AI check counters
4. Task 2 GREEN: `3e36a91` feat(07-10): show the AI check counters in developer stats

## Verification

- `go test -tags testtools -race -count=1 ./alita/db/raiddetection ./alita/db/devs`: ok.
- `go test -tags testtools -race -count=1 -run 'Dev|Raid|AISpam' ./alita/modules`: ok (73 s).
- `TestRaidAIStats` and `TestDevStatsPropagatesSendAndEditErrors` pass with `-v`.
- `CGO_ENABLED=0 go build ./...` and `go vet -tags testtools ./alita/...`: clean.
- `gofmt -l` on the touched files: clean. `make lint` was not run (golangci-lint is built with Go 1.25, the module is Go 1.26), so `go vet` and `gofmt` stand in for it, as the plan allows.
- Acceptance greps: `"ai_enabled = ?", false` 1, `AI spam off` 1 in repository.go, `func raidAIStatsSection(` 1, `GetRaidAIStats()` 1, `GetAISpamStats()` 1, `this replica` 1.
- No string was added to a locale file, so `make check-translations` was not needed. No PostgreSQL was started; the count is a plain row count and ran on SQLite.

## Deviations from Plan

None - plan executed exactly as written. Two small choices inside the plan's freedom: the heading reads "AI checks (this replica, since its last restart)" and the test also checks the `/aispam` line by enabling `/aispam` for the env's chat after the raid scenario, so the raid counters are unchanged by it.

## Known Stubs

None.

## Threat Flags

None. The section prints counters only (T-07-37), the heading says "this replica" (T-07-38), and the owner/developer check in `getStats` is unchanged.

## Pending human verification

- Run `/stats` as the owner on the live bot and read the new "AI checks (this replica...)" block and the "AI spam off" line once real traffic has passed through; this needs the live Telegram, TypeSafe and Gemini services and cannot run here.

## Next Phase Readiness

Ready for the wave's remaining plans. 07-11 (docs) can describe the new `/stats` lines; this plan touched no docs.

## Self-Check: PASSED

- Created file exists: `alita/modules/raid_ai_stats_test.go`.
- The four task commits (`83315e4`, `f605c41`, `741b108`, `3e36a91`) are on the branch.
