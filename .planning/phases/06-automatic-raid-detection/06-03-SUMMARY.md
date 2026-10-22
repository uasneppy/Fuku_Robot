---
phase: 06-automatic-raid-detection
plan: 03
subsystem: lockdown
tags: [raid-detection, join-surge, service-message, redis, lua, gotgbot]

requires:
  - phase: 06-automatic-raid-detection
    provides: raidCountMemberJoin, raidJoinCounts, recordRaidJoins, raidFire, the alita:raid joined set
provides:
  - raidCountMessageJoins, the service-message path of the join surge, counted by the same rule as the chat_member path
  - lockdownMemberJoinInput and lockdownMessageJoinInputs, the one place the guard and the counter build a join's input
  - a performer lookup that a join or a join message makes at most once, shared by the counter and the guard
  - tests that pin the exemptions (RAID-07), the 60 s window, the Lua script and the no-Redis path
affects: [06-04, 06-05, 06-06]

actuals:
  tokens: 8200
  tasks: 2
  commits: 3

plan_head_before: bbacab3be9576b9b218945abcf56e1d78bae264e
plan_head_after: 8973ea92e89db39b0efc781f67e59905995e19ca

commits: 3

tech-stack:
  added: []
  patterns:
    - "Guard and counter share input builders so the performer rule cannot drift"
    - "A counter that made the live performer lookup hands the status to the guard, for a join message as for a chat_member update"

key-files:
  created:
    - alita/modules/raid_join_test.go
    - alita/modules/raid_counters_test.go
  modified:
    - alita/modules/lockdown_guard.go
    - alita/modules/raid_join.go
    - AGENTS.md

key-decisions:
  - "A service message's counted accounts go to Redis in one script call and the fire check uses the count that call returns"
  - "raidJoinsCountedFor and raidRecordAndFire are shared by the chat_member and service-message counters, so the approval read and the threshold check exist once"
  - "The request handler is untouched: a join request never reaches the counter"

patterns-established:
  - "A test that finds the code already right keeps the test commit alone and proves each case can fail by temporary mutation"

requirements-completed: [RAID-01, RAID-07]

duration: 13min
completed: 2026-10-18
status: complete
---

# Phase 6 Plan 03: Join surge through service messages and exemptions Summary

**A new_chat_members service message now counts toward the join surge by the same rule as the chat_member path (two deliveries of one join count once), and admin-added, anonymous-admin, approved, bot-itself and join-request joins are proven never to count.**

## Performance

- **Duration:** about 13 min
- **Started:** 2026-10-18T12:10:58Z
- **Completed:** 2026-10-18T12:23Z
- **Tasks:** 2
- **Files modified:** 5 (2 created)

## Accomplishments

- A service message that names 15 accounts, sent by a plain member or by the accounts themselves, locks the group with `join_surge` and a count of 15. Accounts that also arrive as `chat_member` updates are one member of the joined set, so 14 accounts delivered both ways hold 14, not 28.
- `lockdownOnJoinMessage` calls `raidCountMessageJoins` after the tracker write and before the lockdown read. When the counter looked the performer up (succeeded or failed) the guard reuses that answer, so a join message makes one `getChatMember`, never two (T-06-10).
- The guard's input construction moved into `lockdownMemberJoinInput` and `lockdownMessageJoinInputs`, used by both guard handlers and both counters. The guard's decisions are unchanged: all ten named guard suites pass.
- `TestRaidJoinExemptions` (14 subtests) shows admin-added (chat_member and service message), anonymous-admin-added (both paths), admin-added bots, approved users (both paths), the bot itself, join requests and an admin-approved request leave the counter alone, and that a member-added bot, a bot-approved request and a failed performer lookup do count (T-06-08).
- `TestRaidJoinBelowThreshold` pins the 60 s window end to end. `TestRaidJoinScript` pins dedupe, the window, the 24 h trim, the cap and the 25 h TTL on miniredis. `TestRaidInertWithoutRedis` shows 20 joins without Redis make no `getChat`, no `setChatPermissions`, no lockdown row, one info line, and `/lockdown` still locks.

## Task Commits

1. **Task 1: a service-message join counts, and a join delivered twice counts once**
   - `129580a` (test) `TestRaidJoinDoubleDelivery`, `TestRaidJoinServiceMessage`
   - `6b21209` (feat) `raidCountMessageJoins`, the shared builders, the guard hook, AGENTS.md
2. **Task 2: only real joins count; the window, the script and the no-Redis path are pinned**
   - `8973ea9` (test) `TestRaidJoinExemptions`, `TestRaidJoinBelowThreshold`, `TestRaidJoinScript`, `TestRaidInertWithoutRedis` (no implementation commit was needed, see below)

**Plan metadata:** the docs commit adding this file.

## TDD Gate Compliance

- Task 1 RED (`129580a`): `TestRaidJoinDoubleDelivery` failed on "no active lockdown after the 15th account joined by service message" and `TestRaidJoinServiceMessage/member_invites` on "no active lockdown after a member added 15 accounts in one service message". Both are the target tests failing on the planned behaviour; the two "admin adds" regression subtests passed (they pin the single lookup that the chat_member path already had). No compile scaffolding was needed in any commit. GREEN (`6b21209`) made all of them pass. `gsd_run check tdd-red-evidence` was not run (the CLI is not available in this worktree); the failing assertions above are the RED evidence.
- Task 2: every case passed when first run, because plan 06-02's `raidJoinCounts`, the Redis script and the no-Redis notice already implement the rules and Task 1 routed the service-message path through them. This is an unexpected GREEN on the test commit, investigated rather than ignored: each case was shown able to fail by a temporary mutation, all reverted (`git checkout --` of the file; the production files match `6b21209`):
  - counting an approved user (approval read removed): `approved user` and `approved user by service message` fail;
  - treating an empty performer status as an administrator: `performer lookup failed` and `approved request by the bot` fail;
  - removing the 24 h trim line: `a join older than 24 hours is trimmed` fails;
  - removing the cap line: `the cap keeps the newest` fails;
  - removing the `PEXPIRE` line: `the key expires after 25 hours` fails;
  - widening the count window to 24 h: `a join older than the window ...` fails in both `TestRaidJoinScript` and `TestRaidJoinBelowThreshold`;
  - logging the no-Redis notice on every call: `TestRaidInertWithoutRedis` fails ("logged 20 times, want exactly once").
  There is therefore no `fix(06-03)` commit, as the plan allows.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Missing Critical] AGENTS.md updated in the Task 1 implementation commit**
- **Found during:** Task 1
- **Issue:** AGENTS.md says the join surge is counted by the `chat_member` guard through `raidCountMemberJoin` only, and that a join makes one performer lookup. Both statements were no longer complete. The project rule is to change AGENTS.md in the same commit as the change that invalidates it. The plan's `files_modified` did not list it.
- **Fix:** The join-surge bullet in the Subsystem traps section now names both guards, the shared builders and the one-lookup-per-join-message rule.
- **Files modified:** `AGENTS.md`
- **Committed in:** `6b21209`

---

**Total deviations:** 1 auto-fixed (1 missing critical, a documentation rule). **Impact on plan:** none on behaviour. Plan 06-04 runs in parallel and may also edit AGENTS.md; the edit here is confined to the one join-surge bullet, so a merge conflict, if any, is a one-bullet resolution.

## Issues Encountered

- `make lint` cannot run here (golangci-lint is built with go1.25 and the module needs go1.26). `gofmt -l` on the four plan files printed nothing and `go vet -tags testtools ./alita/modules` was clean in its place.
- No PostgreSQL server exists in this sandbox and none was started. Every test here runs on SQLite and miniredis.
- `raidKey` carries the chat ID as a hash tag and miniredis does not enforce slots, so cluster behaviour is not exercised; the script touches one key.

## Verification Run

| Check | Result |
|-------|--------|
| `go test -tags testtools -race -run '^TestRaidJoinDoubleDelivery$\|^TestRaidJoinServiceMessage$'` | pass (1 + 3 subtests) |
| `go test -tags testtools -race -run '^TestRaidJoinExemptions$\|^TestRaidJoinBelowThreshold$\|^TestRaidJoinScript$\|^TestRaidInertWithoutRedis$'` | pass (14 + 2 + 5 subtests, plus the Redis-less test) |
| The ten named guard suites (`TestLockdownGuard*`, `TestLockdownMixedServiceMessage`, `TestLockdownJoinAfterDeclinedRequest`, `TestLockdownRejoinAfterUnban`, `TestLockdownApprovalRace`, `TestLockdownAdminAddedJoinerExempt`, `TestLockdownSuppressesGoodbye`, `TestLockdownJoinRequestFailsClosed`, `TestLockdownAcceptButtonRefuses`, `TestDecideLockdownJoin*`) | pass, including `TestLockdownGuardDedupesTwoPaths` and `TestLockdownGuardServiceMessageVariants` |
| `go test -tags testtools -race -run 'Lockdown\|Unlockdown\|Raid' ./alita/modules` | pass (after the last commit) |
| `go test -tags testtools -race -run 'Join\|Greet\|Captcha\|Antiraid\|AntiRaid' ./alita/modules` | pass |
| `CGO_ENABLED=0 go build ./...`, `go vet -tags testtools ./alita/modules`, `gofmt -l` | clean |
| Task 1 acceptance (hook line 239 precedes the `lockdownActiveLookup` line 241 of `lockdownOnJoinMessage`; exactly 2 builder definitions) | met |
| Task 2 acceptance (`grep -c 't.Run('` of `raid_join_test.go` is 19, at least 13; gofmt; `git diff --exit-code go.mod go.sum`) | met |

The full `make test` was not run; the orchestrator runs it after the merge.

## Known Stubs

None.

## Threat Flags

None beyond the plan's register. T-06-08: exemption only from a live `getChatMember` or the anonymous-admin identity, a failed lookup counts (`performer lookup failed`). T-06-09: the set member is the user ID (`TestRaidJoinDoubleDelivery`, `the same user twice counts once`). T-06-10: one lookup per join or join message (`TestRaidJoinServiceMessage` counts `getChatMember` calls). T-06-SC: no installs; `go.mod` and `go.sum` unchanged.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

The flood trigger (06-04) and the settings commands (06-05) can build on `raidJoinsCountedFor`, `raidRecordAndFire` and the shared input builders. No blockers.

---
*Phase: 06-automatic-raid-detection*
*Completed: 2026-10-18*

## Self-Check: PASSED

Created files exist (`alita/modules/raid_join_test.go`, `alita/modules/raid_counters_test.go`); commits `129580a`, `6b21209` and `8973ea9` are in `git log`; `STATE.md` and `ROADMAP.md` are untouched.
