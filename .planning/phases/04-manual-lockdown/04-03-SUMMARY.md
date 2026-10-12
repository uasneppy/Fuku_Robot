---
phase: 04-manual-lockdown
plan: 03
subsystem: lockdown
tags: [telegram, lockdown, join-guard, worker, pacer, gotgbot, gorm, race-safety]

requires:
  - phase: 04-manual-lockdown
    provides: "plans 04-01 and 04-02: chat_lockdowns state machine (Start, GetActiveFresh, BeginLift, FinishLift, GetCurrentFresh, TallyJoiners), /lockdown, /unlockdown restore-then-record, lockdownLiveMember, lockdownFake and newLockdownEnv"
provides:
  - "Join guard at handler group -7: records each chat_member joiner of a confirmed active lockdown and ends update handling, so no welcome, captcha challenge or captcha attempt follows"
  - "DB-driven lockdown worker (StartLockdownWorker / StopLockdownWorker) that bans recorded joiners through a fleet-wide pacer with a 330-day marker end date"
  - "Lift side: cancel pending joiners, unban only the lockdown's own bans (live kicked + ban_until within 2 s), keep every other status, post one tally naming anyone it could not unban"
  - "Joiner repository: JoinRecord, RecordJoin, ClaimJoiner, MoveJoiner, ListPendingFresh, DeletePendingJoin, CancelPending, ListJoinersInState, ListLiftingFresh"
  - "5 locale keys in all 7 locale files and AGENTS.md rules (group -7, alita:lockdown:* keys, worker drain, guard and lift rules)"
affects: [04-04 (join service message, rejoin and cross-path rules in recordLockdownJoin), 04-05 (join requests: decline verdict), 04-06 (stale acting/unbanning claims), 04-08, phase 5 alert sample]

actuals:
  tokens: 21300
  tasks: 2
  commits: 4

tech-stack:
  added: []
  patterns:
    - "Record then act: the guard only writes the joiner row and returns; the worker makes every Telegram write, so a raid cannot pin dispatcher goroutines behind a paced call"
    - "Own-ban marker: ban with until_date = row.ban_until (now + 330 days); the lift recognises its own ban by that end date alone (isLockdownBan, 2 s tolerance)"
    - "Row claims by conditional update (ClaimJoiner / MoveJoiner), exactly-once under several replicas, resumable from PostgreSQL after a restart"
    - "Package-variable test seams (lockdownRecordJoin) to force a record-write failure without a mock library"
    - "A pace slot refused as ErrRateLimited or a shutdown puts the row back without counting an attempt"

key-files:
  created:
    - alita/db/lockdown/joiners.go
    - alita/db/lockdown/joiners_test.go
    - alita/db/lockdown/joiners_lift_test.go
    - alita/modules/lockdown_guard.go
    - alita/modules/lockdown_marker.go
    - alita/modules/lockdown_worker.go
    - alita/modules/lockdown_guard_test.go
    - alita/modules/lockdown_guard_policy_test.go
    - alita/modules/lockdown_lift_joiners_test.go
  modified:
    - alita/db/lockdown/repository.go
    - alita/modules/lockdown.go
    - alita/modules/lockdown_fake_test.go
    - main.go
    - locales/en.yml
    - locales/es.yml
    - locales/fr.yml
    - locales/hi.yml
    - locales/id.yml
    - locales/pt.yml
    - locales/ru.yml
    - AGENTS.md

key-decisions:
  - "The guard never makes a Telegram write and never consults Redis; a failed lockdown read or joiner insert lets the joiner in, still muted by the locked defaults, because a ban without a record would never be lifted"
  - "decideLockdownJoin is a pure table like decideStaffAction: only a user added by a live creator or administrator (or an anonymous admin) is exempt; a self-join, a bot-performed join, a bot account and any failed performer lookup are banned"
  - "A joiner recorded just as the lift starts is withdrawn by the guard (it re-reads the lockdown after its insert) and the lift cancels any pending row it finds"
  - "runLockdownCycle reports progress, not 'row touched': a row put back for a retry or a rate limit is not progress, so the loop waits for the next tick instead of spinning"
  - "MoveJoiner from acting to banned resets attempts to 0, so a ban that needed retries does not eat into the lift's three unban tries"

patterns-established:
  - "Lift order per lockdown per cycle: CancelPending, claim banned rows in id order (getChatMember then only_if_banned unban), then FinishLift; the replica whose FinishLift wins posts the tally"
  - "Names and Telegram reasons in the tally are spliced in after translation through lockdownListToken, and the failure list is bounded by count (25) and by size (3000 runes)"

requirements-completed: [LOCK-01, LOCK-05, LOCK-07]

duration: ~50min (approximate; start time was not recorded)
completed: 2026-10-07
status: complete

plan_head_before: 6dde26603a1fe9ad9421952dd98f1226e3e5fe67
plan_head_after: 4e2a591663e96e085f5f59076e31c110cfca48cb

coverage:
  - id: D1
    description: "A user who joins a confirmed locked group through an invite link gets a pending joiner row (member path, invite link, raw first name and username, performer, ban_until about 330 days ahead) before any ban, and no welcome, captcha challenge, captcha attempt or restriction follows"
    requirement: "LOCK-01"
    verification:
      - kind: integration
        ref: "alita/modules/lockdown_guard_test.go#TestLockdownGuardBansChatMemberJoin"
        status: pass
    human_judgment: true
    rationale: "Live join delivery and the effect of ext.EndGroups on real chat_member updates (D-03) are UAT items; the fake dispatcher proves the handler order, not Telegram's delivery"
  - id: D2
    description: "The worker bans each pending joiner exactly once with until_date equal to the row's ban_until through the fleet-wide lockdown pacer; it starts and stops with the bot, Start twice and Stop twice are safe, and a reopened group still records and bans joiners without being re-locked"
    requirement: "LOCK-01"
    verification:
      - kind: integration
        ref: "alita/modules/lockdown_guard_test.go#TestLockdownWorkerStartStop"
        status: pass
      - kind: integration
        ref: "alita/modules/lockdown_guard_test.go#TestLockdownGuardBansAfterManualReopen"
        status: pass
      - kind: unit
        ref: "alita/db/lockdown/joiners_test.go#TestJoinerClaims"
        status: pass
    human_judgment: false
  - id: D3
    description: "The guard ignores unlocked chats and locks whose Telegram confirmation is missing, exempts a user a live admin added, bans a user a plain member added, lets a joiner in when the record cannot be written, and withdraws its own pending row when the lift started first"
    requirement: "LOCK-01"
    verification:
      - kind: integration
        ref: "alita/modules/lockdown_guard_test.go#TestLockdownGuardIgnoresUnlockedChats"
        status: pass
      - kind: unit
        ref: "alita/modules/lockdown_guard_policy_test.go#TestDecideLockdownJoin"
        status: pass
      - kind: integration
        ref: "alita/modules/lockdown_guard_policy_test.go#TestLockdownGuardFailsOpenWhenTheRecordCannotBeWritten"
        status: pass
      - kind: integration
        ref: "alita/modules/lockdown_guard_policy_test.go#TestLockdownGuardWithdrawsAJoinerWhenTheLiftStarted"
        status: pass
    human_judgment: false
  - id: D4
    description: "After /unlockdown the worker unbans, in recording order, only joiners whose live getChatMember shows kicked with the row's ban_until (2 s tolerance), with only_if_banned=true; a permanent ban, a /tban of another length and an already-unbanned member are kept and never touched; joiners still pending are cancelled and never banned; a lockdown with no joiners lifts with no unban call and no tally"
    requirement: "LOCK-07"
    verification:
      - kind: unit
        ref: "alita/modules/lockdown_lift_joiners_test.go#TestIsLockdownBan"
        status: pass
      - kind: integration
        ref: "alita/modules/lockdown_lift_joiners_test.go#TestLockdownLiftUnbansJoiners"
        status: pass
      - kind: integration
        ref: "alita/modules/lockdown_lift_joiners_test.go#TestLockdownLiftKeepsDeliberateBan"
        status: pass
      - kind: integration
        ref: "alita/modules/lockdown_lift_joiners_test.go#TestLockdownLiftCancelsPending"
        status: pass
      - kind: integration
        ref: "alita/modules/lockdown_lift_joiners_test.go#TestLockdownLiftNoJoiners"
        status: pass
    human_judgment: true
    rationale: "Research A2: getChatMember is assumed to report until_date as sent; the 2 s tolerance covers rounding, but a real answer must be saved as a fixture (UAT item below)"
  - id: D5
    description: "The lift posts one tally in the chat's language once every joiner is handled (unbanned, kept, and each unban failure by name and ID, in recording order, with an 'and N more' cut), posted once by the replica whose FinishLift won"
    requirement: "LOCK-01"
    verification:
      - kind: integration
        ref: "alita/modules/lockdown_lift_joiners_test.go#TestLockdownLiftUnbanFailureListed"
        status: pass
      - kind: integration
        ref: "alita/db/lockdown/joiners_lift_test.go#TestCancelPendingAndListJoinersInState"
        status: pass
    human_judgment: false
  - id: D6
    description: "All lockdown work is in PostgreSQL and resumes on any replica; the worker drain is registered after DB-close"
    requirement: "LOCK-05"
    verification:
      - kind: integration
        ref: "make test (full suite, exit 0)"
        status: pass
      - kind: integration
        ref: "BeginLift/ClaimJoiner/MoveJoiner conditional updates on PostgreSQL 16"
        status: unknown
    human_judgment: true
    rationale: "The conditional claims are proven on SQLite only; no PostgreSQL cluster was available to this agent"
---

# Phase 4 Plan 03: Join guard, ban worker and lift unban Summary

**A group-minus-7 join guard that records joiners of a locked group before anything else can happen to them, a pacer-driven DB worker that bans them with a 330-day marker end date, and a lift that unbans only that marker (never a deliberate ban) and posts one tally.**

## Performance

- **Duration:** about 50 min (approximate; start time was not recorded)
- **Completed:** 2026-10-07
- **Tasks:** 2 (Task 1 tracer, Task 2 auto, both TDD)
- **Commits:** 4 (2 test, 2 feat); measured `git rev-list --count 6dde266..HEAD`
- **Files modified:** 21 (9 created, 12 modified)

## Accomplishments

- Anyone who joins a confirmed locked group through a `chat_member` update is recorded (`pending`, `member` path, invite link, raw name and username, performer, `ban_until` = now + 330 days) before any ban, and update handling ends: no welcome, no captcha challenge, no captcha attempt, no restriction. The control in `TestLockdownGuardBansChatMemberJoin` proves a join before the lockdown does get a welcome or challenge.
- The worker (`StartLockdownWorker`, started in `postInit`; `StopLockdownWorker` registered after the DB-close handler so LIFO stops it first) bans each pending joiner once, through the `alita:lockdown:pace:*` pacer, with `until_date` = the row's `ban_until`. It claims rows with conditional updates, so any replica can run it and a restart only resumes.
- After `/unlockdown` restored the permissions the worker cancels pending joiners, then per banned row asks `getChatMember` and unbans (`only_if_banned=true`) only when the live member is kicked with that row's `ban_until` within 2 s. Permanent bans, `/tban` of another length and members already unbanned by hand are marked `kept` and never touched.
- One tally message in the chat's language, posted once by the replica whose `FinishLift` won: unbanned count, kept count, and each joiner it could not unban by name and ID in recording order, cut at 25 lines (and a 3000-rune bound) with "and N more".
- `/unlockdown` now says "Unbanning N removed joiners now" when banned joiners exist and wakes the worker; a lockdown with no joiners still lifts by `/unlockdown` alone with no tally.

## Task Commits

1. **Task 1 (tracer): join, record, ban** - `2d7957c` (test, RED), `4fc6bec` (feat, GREEN)
2. **Task 2: lift unbans own bans, keeps deliberate ones, tallies** - `150dbe7` (test, RED), `4e2a591` (feat, GREEN)

**Plan metadata:** committed after this file (`docs(04-03): complete join guard, ban worker and lift unban plan`).

## TDD Gate Compliance

Both tasks have a `test(04-03)` commit before their `feat(04-03)` commit. The RED commits carry compile-only stubs (the joiner repository functions, `StartLockdownWorker`, `runLockdownCycle`, `isLockdownBan`) so the target tests failed on their assertions, not on a build error. RED evidence, target tests failing on the planned behaviour: `TestLockdownGuardBansChatMemberJoin` ("joiner rows of user ... = 0, want 1"), `TestLockdownWorkerStartStop`, `TestLockdownGuardBansAfterManualReopen`, `TestRecordJoinOncePerUser`, `TestJoinerClaims`, `TestIsLockdownBan`, `TestLockdownLiftUnbansJoiners`, `TestLockdownLiftKeepsDeliberateBan`, `TestLockdownLiftCancelsPending`, `TestLockdownLiftUnbanFailureListed`. Two tests were already green at RED because they guard against a regression and the stubs did nothing (`TestLockdownGuardIgnoresUnlockedChats`, `TestLockdownLiftNoJoiners`); that is expected for "nothing happens" tests. The `gsd_run check tdd-red-evidence` record file was not produced (the plan is `type: execute`, not `type: tdd`).

## Decisions Made

See `key-decisions` above. Additions beyond the plan's own recorded decisions: progress-based cycle result, attempts reset on acting to banned, and the `lockdownRecordJoin` seam.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Attempt counter carried from the ban phase into the unban phase**
- **Found during:** Task 1 (designing MoveJoiner)
- **Issue:** `attempts` is one column. A joiner whose ban needed two retries would have only one unban try left at the lift before being declared `unban_failed`.
- **Fix:** `MoveJoiner` from `acting` to `banned` (only a ban that took effect makes that move) sets `attempts = 0`. Covered in `TestJoinerClaims` ("a ban that took effect starts the unban with no attempts").
- **Files modified:** alita/db/lockdown/joiners.go, alita/db/lockdown/joiners_test.go
- **Committed in:** 2d7957c, 4fc6bec

**2. [Rule 2 - Missing Critical] Tally could exceed Telegram's 4096-character limit**
- **Found during:** Task 2
- **Issue:** 25 failure lines of up to 64-rune names plus 120-rune escaped reasons can pass 4096 characters, so the tally itself would fail to send, losing the list of people who stayed banned.
- **Fix:** `lockdownFailureLines` also stops at `lockdownTallyMaxRunes` (3000) and counts what it left out in the "and N more" line.
- **Files modified:** alita/modules/lockdown_worker.go
- **Committed in:** 4e2a591

**3. [Rule 1 - Bug] English text of `lockdown_lift_tally_kept` said "stay banned"**
- **Found during:** Task 2
- **Issue:** The plan's wording, "{count} stay banned because someone banned them on purpose.", is false for a member that was already unbanned by hand (`kept` also covers that), which the plan's own test counts as kept.
- **Fix:** Wording is "{count} were left as they are because someone had already unbanned them or banned them on purpose." in all 7 locales.
- **Files modified:** locales/*.yml
- **Committed in:** 4e2a591

**4. [Rule 2 - Missing Critical] Extra tests and a test seam**
- Added `TestDecideLockdownJoin` (pure table), the admin-added exempt join, the plain-member-adds ban, the fail-open record write (the plan's LOCK-01 prohibition is `verification: test`), the lift-started withdrawal, `joiners_lift_test.go`, and a `lockdownRecordJoin` package variable as a seam so a record-write failure can be forced without a mock library. `AGENTS.md`'s staff-style "never lift by accident" rule is mirrored in the new guard bullet.
- **Committed in:** 4fc6bec, 4e2a591

---

**Total deviations:** 4 auto-fixed (2 bug, 2 missing critical). **Impact on plan:** all necessary for correctness or for proving a stated prohibition; no scope creep, no new files outside the plan's area beyond the extra test files.

## Issues Encountered

- A cycle's return value is "made progress" (a row reached a final state or was cancelled), not "touched a row". A row put back for a retry or a rate limit must not make the worker loop spin, so those return false and wait for the next tick.
- The Bash tool refused some heredoc and multi-line commands; file edits went through the Edit and Write tools.

## Known Limitations (carried to later plans, not stubs)

- `recordLockdownJoin` handles an existing row simply: an exempt row lets the joiner in, anything else counts as handled. A user unbanned by hand during a lockdown who rejoins is therefore swallowed (no welcome) while not banned until the reclaim and dedupe-window rules of plan 04-04 land. The `chat_member` path is the only path covered; join service messages (04-04) and join requests (04-05, the `lockdownJoinDecline` verdict) are not.
- A row stuck in `acting` or `unbanning` after a crash is not reclaimed here; plan 04-06 adds stale-claim recovery. Until then a lift with such a row stays `lifting`.
- A tally whose send is cut off by a shutdown (or fails) is logged at warn and not retried; the lift is already recorded as finished by then.
- `until_date` echo (research A2) is assumed; see the UAT item.

## Known Stubs

None.

## Threat Flags

None beyond the plan's `<threat_model>`: T-04-10 (guard makes no Telegram write, worker bans under the pacer), T-04-11 (record before ban, fail-open on a failed insert), T-04-12 (`isLockdownBan` plus `TestLockdownLiftKeepsDeliberateBan`) and T-04-13 (one fleet-wide Redis pacer, rate-limited rows retried without an attempt) are all implemented and tested.

## Verification Run

- Plan tests: `TestLockdownGuardBansChatMemberJoin`, `TestLockdownGuardBansAfterManualReopen`, `TestLockdownGuardIgnoresUnlockedChats`, `TestLockdownWorkerStartStop`, `TestRecordJoinOncePerUser`, `TestJoinerClaims`, `TestIsLockdownBan`, `TestLockdownLiftUnbansJoiners`, `TestLockdownLiftKeepsDeliberateBan`, `TestLockdownLiftNoJoiners`, `TestLockdownLiftCancelsPending`, `TestLockdownLiftUnbanFailureListed`: all pass with `-tags testtools -race -count=1`; the guard, lift and worker tests also pass with `-count=8`.
- `go test -tags testtools -race -count=1 ./alita/db/... ./alita/modules/... ./alita/utils/...`: all ok.
- `make test`: exit 0 (69.7% coverage). The known flaky `TestStaffActionNonStaffUnchanged/tmute_as_a_reply` did not fail.
- `CGO_ENABLED=0 go build ./...`, `go vet -tags testtools ./alita/modules ./alita/db/lockdown .`, `make check-translations` (all 7 present), `make check-docs` (no drift), `gofmt -l` on the touched files (clean), `git diff go.mod go.sum` (unchanged): all pass.
- All `<acceptance_criteria>` of both tasks were re-run and pass (greps for `handlerGroup: -7`, `loadLockdownGuard(dispatcher)`, no `claimRecentJoinProcessing`/`IsUserAdmin`/`BanChatMember` outside comments in the guard, `alita:lockdown:pace:block`, `StartLockdownWorker`/`StopLockdownWorker` wiring with the stop after `return closeDBConnections()`, the AGENTS.md rules, `OnlyIfBanned: true`, the unban only inside `lockdownUnbanOne` which calls `isLockdownBan`, equal `^lockdown_lift` counts of 9 in every locale, `lockdownBanUntilTolerance int64 = 2`, test commit before feat commit).
- `make lint`: not run, the installed golangci-lint 2.5.0 is built with go1.25 and refuses the go1.26.0 module (environment limitation, as noted in the run rules).
- PostgreSQL: not run. No throwaway cluster was set up; all repository and conditional-update tests ran on SQLite. Command left for the orchestrator or CI: `DATABASE_URL=<postgres url> ALITA_TEST_DATABASE=true CGO_ENABLED=1 go test -tags testtools -race -count=1 ./alita/db/lockdown` (with the migration chain applied first).

## End-of-Phase UAT Items (live Telegram, not run)

1. **Research A2 / tracer human-check.** In a test supergroup in lockdown, let a test account join through an invite link, then read `getChatMember` for it and compare `until_date` with the stored `ban_until`; save the real answer as a test fixture. Then `/unlockdown` and confirm the account can rejoin. If Telegram does not echo the value within 2 s, widen `lockdownBanUntilTolerance`.
2. **D-03 live delivery.** Confirm a real join delivers a `chat_member` update to group -7 and that `ext.EndGroups` suppresses the welcome and the captcha challenge in the live dispatcher (the join service message path is plan 04-04).
3. **Pacing under a burst.** With several accounts joining at once, confirm bans land through the 100 ms pacer without 429 storms.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- 04-04 can extend `recordLockdownJoin` (existing-row, reclaim and dedupe rules), add the service-message guard and the kicked filter on top of `decideLockdownJoin`, `lockdownNeedsPerformerLookup` and `MoveJoiner`.
- 04-05 reuses `decideLockdownJoin` (Path request gives `lockdownJoinDecline`) and the worker's claim/move/pacer helpers.
- 04-06 should add the stale `acting`/`unbanning` reclaim to `lockdownBanPending` / `lockdownProcessLift`.
- No blockers.

---
*Phase: 04-manual-lockdown*
*Completed: 2026-10-07*

## Self-Check: PASSED
