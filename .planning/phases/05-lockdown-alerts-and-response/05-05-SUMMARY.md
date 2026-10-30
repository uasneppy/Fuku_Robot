---
phase: 05-lockdown-alerts-and-response
plan: 05
subsystem: lockdown
tags: [redis, join-tracker, lockdown-guard, best-effort]
requires:
  - phase: 05-lockdown-alerts-and-response
    provides: lockdown join guard (group -7) from phase 4, alert plumbing from 05-01..05-03
provides:
  - "alita:lockdown:recent:<chat> 10-minute join tracker written by both group -7 join handlers"
  - "lockdownRecentLookup snapshot seam (one entry per user, link merged across the two join paths)"
affects: [05-07, phase-06]
tech-stack:
  added: []
  patterns: ["best-effort single-pipeline Redis write under a 250 ms timeout on a hot path"]
key-files:
  created:
    - alita/modules/lockdown_recent.go
    - alita/modules/lockdown_recent_test.go
  modified:
    - alita/modules/lockdown_guard.go
    - AGENTS.md
key-decisions:
  - "ZADD NX so an identical member delivered by both join paths keeps the earliest score; differing members (link vs no link) are merged by user ID at read time"
  - "Tracker write sits before the lockdown read in both handlers and returns nothing, so no guard verdict, return value or ban depends on Redis"
patterns-established:
  - "Join tracker entry builders (lockdownMemberJoins, lockdownMessageJoins) keep the guard's call site a single trackLockdownRecentJoins(...) line"
requirements-completed: [LOCK-11]
status: complete
actuals:
  tokens: 14000
  tasks: 2
  commits: 3
plan_head_before: 589d484fca1c028b44fc729820cada51f917cc7c
plan_head_after: 9bc609be209e3870400f990e18e125b36e35b488
duration: 25min
completed: 2026-10-11
---

# Phase 5 Plan 05: Recent-joins tracker Summary

**A best-effort, Redis-only 10-minute join tracker per supergroup, written by both lockdown join handlers and readable as a de-duplicated snapshot, with the guard's behaviour unchanged with Redis present, failing or absent.**

## What was built

- `alita/modules/lockdown_recent.go`: `alita:lockdown:recent:<chat>` is a sorted set scored by join time in unix milliseconds. Each member is compact JSON (`i`, `n`, `u`, `b`, `l`). `trackLockdownRecentJoins` does one pipeline under a 250 ms timeout: `ZADD NX`, trim older than 10 minutes, keep the newest 500, `EXPIRE` 700 s. It does nothing without Redis, and an error is logged at debug and dropped. Names are capped at 64 runes, usernames at 32 and links at 128. `lockdownRecentLookup` (= `readLockdownRecentJoins`) returns each user once for a window `(from, to]`: the earliest join time wins and a non-empty link, name or username fills in. Junk members are skipped, and it returns `(nil, nil)` without Redis.
- `alita/modules/lockdown_guard.go`: `lockdownOnJoinMember` and `lockdownOnJoinMessage` call `trackLockdownRecentJoins` right after the supergroup check and before the lockdown read. The bot's own join is skipped, and join requests are not tracked. The chat_member path carries the invite link and the service-message path does not.
- `AGENTS.md`: the operational Redis key list documents `alita:lockdown:recent:<chat>`, and the lockdown-guard trap bullet notes that its write is best effort and ignored.

## Tests

- `TestLockdownRecentTracked` (chat_member with link, service message with a bot account, the bot itself, window) and `TestLockdownRecentJoins` (dedupe, earliest time and link fill-in, trim, cap in single and batch writes, TTL, capped text, junk members, no Redis, failed Redis). The last two prove a joiner of a locked group is still recorded, banned by the worker and unaffected.
- Mutation check: removing the trim, rank-cap and expire calls makes the trim, cap, batch-cap and ttl cases fail.
- Passing: `go test -tags testtools -race -count=1 -run 'Lockdown|Greetings|Antiraid' ./alita/modules`, `CGO_ENABLED=0 go build ./...`, `go vet -tags testtools ./alita/modules`, `gofmt -l` clean on touched files, `go.mod`/`go.sum` unchanged. Acceptance greps pass (`trackLockdownRecentJoins(` x2 in the guard, no `claimRecentJoinProcessing|IsUserAdmin|BanChatMember` in guard code).
- `golangci-lint` was not run: the installed build targets Go 1.25 and cannot lint this Go 1.26 code (go vet and gofmt used instead).

## Deviations from Plan

### Auto-fixed Issues

None. One process note: the plan expected a RED-then-fix commit pair for Task 2. The implementation written in Task 1 (ZADD NX, trim, cap, expire, timeout, error swallowing) already satisfied every Task 2 case, so `TestLockdownRecentJoins` was committed as a single `test(05-05)` commit and there was no `fix(05-05)` commit. The tests were shown to fail when the trim, cap and expire calls are removed.

The Task 1 RED commit fails to compile (the symbols it tests do not exist yet), which is the failing state for that commit.

## Known Stubs

None.

## Threat Flags

None. The only new surface is the Redis key in the planned threat model (T-05-14 to T-05-16 mitigated as specified).

## Commits

- c99ef95 test(05-05): show the joins before a lockdown are forgotten
- fd7aa6b feat(05-05): remember each group's joins of the last ten minutes
- 9bc609b test(05-05): pin the join tracker's dedupe, trim, cap, TTL and no-Redis behaviour

## Self-Check: PASSED

- alita/modules/lockdown_recent.go, alita/modules/lockdown_recent_test.go, AGENTS.md, alita/modules/lockdown_guard.go: present and committed.
- Commits c99ef95, fd7aa6b, 9bc609b exist on the worktree branch.
