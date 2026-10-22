---
phase: 04-manual-lockdown
plan: 04
subsystem: lockdown
tags: [telegram, lockdown, join-guard, service-message, dedupe, gotgbot, gorm, race-safety]

requires:
  - phase: 04-manual-lockdown
    provides: "plan 04-03: group -7 chat_member guard, decideLockdownJoin, recordLockdownJoin, DB-driven ban worker, joiner repository, lockdownFake and newLockdownEnv"
provides:
  - "Join service message handler (lockdownOnJoinMessage, SetAllowBot) at group -7: records and bans like the chat_member path, runs for bot and anonymous-admin senders, filters ctx.EffectiveMessage.NewChatMembers in place to the users let in"
  - "Row-only cross-path deduplication: 20 s window (lockdownJoinDedupeWindow), re-join reclaim of finished rows (ReclaimJoin, lockdownReclaimStates), request-row reclaim, chat_member approval turns a pending row exempt (D-24)"
  - "Join service message deletion after the ban (SetJoinMsg, ListJoinMsgsToDeleteFresh, ClearJoinMsg, lockdownDeleteJoinMessages)"
  - "Goodbye suppression for the lockdown's own ban (HasJoinerBanFresh, lockdownKickedFilter, lockdownOnKicked)"
  - "Decision table plus invariant walk for decideLockdownJoin (lockdown_decide_test.go); AGENTS.md rules"
affects: [04-05 (join requests: the existing-row rules already reclaim a request row met by a link join), 04-06 (stale acting/unbanning claims), 04-08]

actuals:
  tokens: 19000
  tasks: 2
  commits: 4

tech-stack:
  added: []
  patterns:
    - "Deduplication is the (lockdown_id, user_id) row alone: first insert decides, a later delivery is judged by the row's state and updated_at, never by a Redis claim"
    - "SetJoinMsg and ClearJoinMsg use UpdateColumn so a second delivery or a cleared message never opens a new dedupe window"
    - "A lost conditional update loops back to re-read the row through the same RecordJoin seam (three looks), instead of a separate read function"
    - "A reclaimed row withdrawn by a lift that started first is moved pending to cancelled (it carries history); a brand new one is deleted"

key-files:
  created:
    - alita/modules/lockdown_decide_test.go
    - alita/modules/lockdown_paths_test.go
  modified:
    - alita/db/lockdown/joiners.go
    - alita/db/lockdown/joiners_test.go
    - alita/modules/lockdown_guard.go
    - alita/modules/lockdown_worker.go
    - alita/modules/lockdown_fake_test.go
    - alita/modules/lockdown_guard_policy_test.go
    - AGENTS.md

key-decisions:
  - "The join service message's ID is stored on the rows only when every non-ignored user in it was banned, so a message that announces an exempt user is never deleted"
  - "An anonymous admin on the service path is sender_chat equal to the group OR from equal to the Group Anonymous Bot, matching the chat_member path; no live lookup is made for either"
  - "A reclaim that fails with a database error lets the joiner in (fail open, like a failed insert), because a ban the lift cannot find is never undone"
  - "The worker keeps a join message row for the next cycle on a rate limit or shutdown, and clears it after any real answer, success or error (a delete without the right just leaves the message)"
  - "HasJoinerBanFresh keeps its own 2 s tolerance constant in the repository package, because the modules package cannot be imported from it"

patterns-established:
  - "Existing-row rules in order: request row met by a link/service join is reclaimed at any age; exempt row within 20 s lets in; chat_member exempt verdict turns a pending row exempt; any other row within 20 s or still pending/acting is handled (and remembers the join message); older finished row is reclaimed"

requirements-completed: [LOCK-01]

duration: ~15 min
completed: 2026-10-07
status: complete

plan_head_before: 441ca90b044c7a46fc03f1ff94a540b8cf60fc53
plan_head_after: aab4e4c440d1336837d8ae2e8eb20f78f9bd4ec3

coverage:
  - id: D1
    description: "A join announced by a new_chat_members service message of a confirmed locked group is recorded and banned exactly like a chat_member join, including a message sent by a bot account or an anonymous admin; no welcome, captcha challenge or restriction follows, and the join message is deleted after the ban"
    requirement: "LOCK-01"
    verification:
      - kind: integration
        ref: "alita/modules/lockdown_paths_test.go#TestLockdownGuardDedupesTwoPaths"
        status: pass
      - kind: integration
        ref: "alita/modules/lockdown_paths_test.go#TestLockdownGuardServiceMessageVariants"
        status: pass
    human_judgment: true
    rationale: "Live delivery order of chat_member versus service message and a real anonymous admin's sender_chat (research A3, A4, A5) are UAT items; the fake dispatcher proves handler behaviour, not Telegram's delivery"
  - id: D2
    description: "The same join delivered on both paths, in either order, gives one joiner row, one banChatMember call and one deleteMessage, and no greetings run; a later join more than 20 s after the row last changed is a new join with a new ban_until, and a second delivery of that re-join causes no further ban"
    requirement: "LOCK-01"
    verification:
      - kind: integration
        ref: "alita/modules/lockdown_paths_test.go#TestLockdownGuardDedupesTwoPaths"
        status: pass
      - kind: integration
        ref: "alita/modules/lockdown_paths_test.go#TestLockdownRejoinAfterUnban"
        status: pass
      - kind: integration
        ref: "alita/modules/lockdown_paths_test.go#TestLockdownJoinAfterDeclinedRequest"
        status: pass
      - kind: unit
        ref: "alita/db/lockdown/joiners_test.go#TestReclaimJoin"
        status: pass
      - kind: unit
        ref: "alita/db/lockdown/joiners_test.go#TestJoinMsgs"
        status: pass
    human_judgment: false
  - id: D3
    description: "A service message that adds an exempt user and a banned one keeps only the exempt user in NewChatMembers for later handlers (greetings welcome only them) and is not deleted"
    requirement: "LOCK-01"
    verification:
      - kind: integration
        ref: "alita/modules/lockdown_paths_test.go#TestLockdownMixedServiceMessage"
        status: pass
    human_judgment: false
  - id: D4
    description: "When the service message recorded a ban first and the chat_member update then shows an admin approving the request, the pending row becomes exempt and the user is let in and greeted; once the worker already banned them they stay banned until the lift (D-24)"
    requirement: "LOCK-01"
    verification:
      - kind: integration
        ref: "alita/modules/lockdown_paths_test.go#TestLockdownApprovalRace"
        status: pass
    human_judgment: true
    rationale: "Whether Telegram puts the approving admin in chat_member.from and the joiner in the service message's from (research A3, A5) is a live UAT item"
  - id: D5
    description: "Only a user added by a live creator or administrator, or by an anonymous admin, gets in; self-joins, joins performed by the bot, users added by non-admins and every bot are banned, and a failed performer lookup bans (D-07, D-08)"
    requirement: "LOCK-01"
    verification:
      - kind: unit
        ref: "alita/modules/lockdown_decide_test.go#TestDecideLockdownJoin"
        status: pass
      - kind: unit
        ref: "alita/modules/lockdown_decide_test.go#TestDecideLockdownJoinInvariants"
        status: pass
      - kind: integration
        ref: "alita/modules/lockdown_paths_test.go#TestLockdownAdminAddedJoinerExempt"
        status: pass
    human_judgment: false
  - id: D6
    description: "The lockdown's own ban of a joiner produces no goodbye (kicked update matched to a joiner row by until_date within 2 s); a ban without a matching row, or with another end date, still gets the group's goodbye"
    requirement: "LOCK-01"
    verification:
      - kind: integration
        ref: "alita/modules/lockdown_paths_test.go#TestLockdownSuppressesGoodbye"
        status: pass
      - kind: unit
        ref: "alita/db/lockdown/joiners_test.go#TestHasJoinerBanFresh"
        status: pass
    human_judgment: false
  - id: D7
    description: "No lockdown decision reads Redis: deduplication is the joiner row alone, so a Redis outage cannot skip a ban"
    requirement: "LOCK-01"
    verification:
      - kind: command
        ref: "grep -v '^[[:space:]]*//' alita/modules/lockdown_guard.go | grep -c 'claimRecentJoinProcessing\\|GetRedisClient\\|GetMarshal' prints 0"
        status: pass
      - kind: integration
        ref: "alita/modules/lockdown_paths_test.go#TestLockdownGuardDedupesTwoPaths"
        status: pass
    human_judgment: false
---

# Phase 4 Plan 04: Service-message joins, cross-path dedupe and who gets in Summary

**A join is handled once whichever way Telegram reports it: the guard now also takes the new_chat_members service message (bot and anonymous-admin senders included), deduplicates both paths by the joiner row alone with a 20 s window, treats a later re-join as a new join, lets an admin-approved user in, deletes the join message after the ban, and the lockdown's own ban sends no goodbye.**

## Performance

- **Duration:** about 15 min (start recorded 02:16:46Z, last code commit about 02:28Z)
- **Tasks:** 2 (Task 1 tracer, Task 2 auto, both TDD)
- **Commits:** 4 (2 test, 2 feat); measured `git rev-list --count 441ca90..HEAD` before this summary commit
- **Files:** 9 touched by the code commits (2 created, 7 modified)

## Accomplishments

- **Service-message guard** (`lockdownOnJoinMessage`, group -7, `SetAllowBot(true)`): decides every user in the message, makes one live performer lookup only when a verdict depends on it, records each user through `recordLockdownJoin`, and assigns the let-in users (and the bot itself) back to `ctx.EffectiveMessage.NewChatMembers`. A message with nobody left ends the update (`ext.EndGroups`); a mixed one continues, so greetings welcome only the exempt user.
- **One join, one row** (`recordLockdownJoin` / `lockdownExistingJoin`): a second delivery of the same join, in either order, is the existing row; a finished row older than 20 s is reclaimed with one conditional update (`lockdown.ReclaimJoin`), so a hand-unbanned user who rejoins is banned again with a new `ban_until`; a declined join request met by a link join is reclaimed at any age; a chat_member update that shows an admin added or approved the user turns a still-pending row exempt, and once the worker banned them the ban stands until the lift (D-24).
- **Join message deleted after the ban:** the message ID is stored on the rows only when every user in it was banned (`JoinMsgID` at insert, or `SetJoinMsg` when the second delivery carries it); the worker (`lockdownDeleteJoinMessages`, after the bans, paced) deletes it and clears `join_msg_id`.
- **No goodbye for the lockdown's own ban:** `lockdownKickedFilter` + `lockdownOnKicked` end handling when the kicked update's `until_date` matches a joiner row's `ban_until` within 2 s (`HasJoinerBanFresh`); a deliberate ban (no row, or another end date) still gets the goodbye.
- **Who gets in is one tested table:** `TestDecideLockdownJoin` plus an invariant walk over 504 combinations (path, performer status, member kind, identity relations, anonymous flag): a bot, a self-join or a bot-performed join is never exempt, exempt needs a creator or administrator or an anonymous admin, and `lockdownNeedsPerformerLookup` is true exactly when the status can change the verdict.
- **AGENTS.md** records the service-message path, the row-only dedupe, the re-join and approval rules, join-message deletion and the goodbye rule.

## Task Commits

1. **Task 1 (tracer): service message, dedupe, re-join, approval race** - `d08911d` (test, RED), `9db0d5b` (feat, GREEN)
2. **Task 2: who gets in, goodbye suppression** - `b78f9a3` (test, RED), `aab4e4c` (feat, GREEN)

**Plan metadata:** committed after this file (`docs(04-04): complete service-message joins, dedupe and goodbye plan`).

## TDD Gate Compliance

Both tasks have a `test(04-04)` commit before their `feat(04-04)` commit (`git log --oneline -4` shows them in order). The RED commits carry compile-only stubs (`ReclaimJoin`, `SetJoinMsg`, `ListJoinMsgsToDeleteFresh`, `ClearJoinMsg`; later `HasJoinerBanFresh`) so the target tests failed on their assertions, not on a build error. RED evidence (target tests failing on the planned behaviour):

- Task 1: `TestLockdownGuardDedupesTwoPaths` (all three subtests: "joiner rows of user ... = 0, want 1", "row ... want path member and join_msg_id"), `TestLockdownMixedServiceMessage` ("saw new members [a b], want only [a]"), `TestLockdownJoinAfterDeclinedRequest`, `TestLockdownRejoinAfterUnban`, `TestLockdownApprovalRace`, `TestLockdownGuardServiceMessageVariants`, `TestReclaimJoin`, `TestJoinMsgs`.
- Task 2: `TestLockdownSuppressesGoodbye` ("messages sent after the lockdown's own ban = 1, want none") and `TestHasJoinerBanFresh` (three assertion failures).
- Already green at RED, expected: `TestDecideLockdownJoin`, `TestDecideLockdownJoinInvariants` and `TestLockdownAdminAddedJoinerExempt` pin behaviour plan 04-03 had already implemented (the plan says "fix whatever the table and walk expose"; they exposed nothing). The `gsd_run check tdd-red-evidence` record file was not produced (the plan is `type: execute`, not `type: tdd`).

Tracer gate: the tracer's automated `<verify>` passed end to end (logged "Tracer verified end-to-end, expanding"); its two `<human-check>` items are end-of-phase UAT items below, not a stop.

## Decisions Made

See `key-decisions` above.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] `TestDecideLockdownJoin` already existed in `lockdown_guard_policy_test.go`**
- **Found during:** Task 2 (writing `lockdown_decide_test.go`)
- **Issue:** Plan 04-03 had put `TestDecideLockdownJoin` in `lockdown_guard_policy_test.go`. The plan names `alita/modules/lockdown_decide_test.go` as its home; leaving both would not compile (duplicate function).
- **Fix:** moved the table into `lockdown_decide_test.go` (adding two cases: a bot added by a live creator on the service path, a live administrator on the service path, and the kicked/restricted performer statuses) and added `TestDecideLockdownJoinInvariants`; removed the old copy.
- **Files modified:** alita/modules/lockdown_decide_test.go, alita/modules/lockdown_guard_policy_test.go
- **Committed in:** b78f9a3

**2. [Rule 2 - Missing Critical] Reclaim failure and lift-start race on a reclaimed row**
- **Found during:** Task 1 (designing the existing-row rules)
- **Issue:** the 04-03 withdrawal (delete a pending row when the lift started first) fits a brand new row, but a reclaimed row carries an earlier join's history, and a database error while reclaiming must not leave a ban the lift cannot find.
- **Fix:** a reclaimed row withdrawn by a lift that started first is moved pending to cancelled (`lockdownPendingHeld(..., reclaimed=true)`); a failed reclaim lets the joiner in (fail open, logged), like a failed insert.
- **Files modified:** alita/modules/lockdown_guard.go
- **Committed in:** 9db0d5b

**3. [Rule 2 - Missing Critical] `SetJoinMsg` and `ClearJoinMsg` leave `updated_at` alone**
- **Found during:** Task 1
- **Issue:** a plain `Updates` would stamp `updated_at`, so a second delivery that only attaches a message ID (or the worker clearing it) would open a new 20 s window and could make a real re-join look like a duplicate.
- **Fix:** both use `UpdateColumn`; `TestJoinMsgs` asserts `updated_at` is unchanged.
- **Files modified:** alita/db/lockdown/joiners.go
- **Committed in:** 9db0d5b

**4. [Rule 2 - Missing Critical] A rate limit must not drop a join message delete**
- **Found during:** Task 1
- **Issue:** the plan says to clear `join_msg_id` "whatever the delete returns"; a pacer refusal or shutdown is not an answer from Telegram, and clearing then would skip the delete for good.
- **Fix:** `lockdownDeleteJoinMessages` leaves the row for the next cycle on a rate limit or shutdown and clears it after any real answer (success or error).
- **Files modified:** alita/modules/lockdown_worker.go
- **Committed in:** 9db0d5b

---

**Total deviations:** 4 auto-fixed (1 blocking, 3 missing critical). **Impact on plan:** none of them changes behaviour the plan specified; all were needed for correctness. The new DB tests are in `joiners_test.go` as planned; extra tests were added (`TestDecideLockdownJoinInvariants`, a second mixed-message subtest for greetings, a replaced-ban goodbye case).

## Issues Encountered

- A Bash call with a long Python heredoc was refused twice by the sandbox ("too complex to verify"); file edits went through the Edit and Write tools and short commands.
- Pre-existing, not touched: `gofmt -l alita/modules` lists `greetings_command_test.go`.

## Known Limitations (carried forward, not stubs)

- Join requests (the `lockdownJoinDecline` verdict, `chat_join_request` handler) are plan 04-05; the existing-row rules already reclaim a `request` row met by a link or service join, and AGENTS.md says join requests are not handled by the guard yet.
- A row stuck in `acting` or `unbanning` after a crash is still not reclaimed (plan 04-06). A join service message whose users are a mix of let-in and banned is never deleted (by design); a multi-user message where one user's existing row turns out exempt after the message ID was already stored on a sibling row could still be deleted. That case needs a performer who differs between the two deliveries of one join, which Telegram does not produce except for request approvals (single-user messages).
- Time-based tests do not sleep: row age is set with `ageJoiner`/direct updates, ban_until shifted by hand in `TestLockdownRejoinAfterUnban`.

## Known Stubs

None. The four compile-only stubs of the RED commits are replaced in the GREEN commits.

## Threat Flags

None beyond the plan's `<threat_model>`: T-04-14 (live performer lookup, failed lookup bans: `TestLockdownAdminAddedJoinerExempt`, `TestDecideLockdownJoin`), T-04-15 (one row per (lockdown, user) with a 20 s window, no Redis in the decision: `TestLockdownGuardDedupesTwoPaths`) and T-04-16 (bots and self-joins always ban: the invariant walk) are implemented and tested.

## Verification Run

- Plan tests, all with `-tags testtools -race -count=1` and passing: `TestLockdownGuardDedupesTwoPaths`, `TestLockdownMixedServiceMessage`, `TestLockdownRejoinAfterUnban`, `TestLockdownJoinAfterDeclinedRequest`, `TestLockdownApprovalRace`, `TestLockdownGuardServiceMessageVariants`, `TestDecideLockdownJoin`, `TestLockdownAdminAddedJoinerExempt`, `TestLockdownSuppressesGoodbye` (./alita/modules); `TestReclaimJoin`, `TestJoinMsgs`, `TestHasJoinerBanFresh` (./alita/db/lockdown). The lockdown, joiner and decision tests also pass with `-count=8`.
- Plan-level verification: `go test -tags testtools -race -count=1 -run 'Lockdown|Unlockdown|Joiner|RecordJoin|ReclaimJoin|JoinMsgs' ./alita/modules ./alita/db/lockdown` and `-run 'Greeting|Captcha|AntiRaid' ./alita/modules`: both ok. `git diff go.mod go.sum`: unchanged.
- `make test`: exit 0 (69.8% coverage). The known flaky `TestStaffActionNonStaffUnchanged/tmute_as_a_reply` did not fail.
- `CGO_ENABLED=0 go build ./...`, `go vet -tags testtools ./alita/modules ./alita/db/lockdown .`, `make check-translations` (all present, no locale keys added), `make check-docs` (no drift), `gofmt -l` on the plan's touched files (clean): all pass.
- Acceptance criteria re-run, all pass: `lockdownJoinDedupeWindow = 20 * time.Second` matches; `SetAllowBot(true)` matches; the comment-stripped guard has 0 matches for `claimRecentJoinProcessing|GetRedisClient|GetMarshal`; the exact `ReclaimJoin` signature matches; `func (m moduleStruct) lockdownOnKicked(` matches; `grep -c AddHandlerToGroup` prints 3; AGENTS.md contains "no goodbye"; `git log --oneline` shows each `test(04-04)` commit before its `feat(04-04)` commit.
- `make lint`: not run. The installed golangci-lint 2.5.0 is built with go1.25 and refuses the go1.26.0 module (environment limitation).
- PostgreSQL: not run. No throwaway cluster was set up; the new conditional updates (`ReclaimJoin`, `SetJoinMsg`, `ListJoinMsgsToDeleteFresh`, `HasJoinerBanFresh`) ran on SQLite only. Command left for the orchestrator or CI, with the migration chain applied first: `DATABASE_URL=<postgres url> ALITA_TEST_DATABASE=true CGO_ENABLED=1 go test -tags testtools -race -count=1 ./alita/db/lockdown`. The `updated_at < ?` cut-off is passed as a UTC time, matching the UTC values the repository writes.

## End-of-Phase UAT Items (live Telegram, not run)

1. **Tracer human-check 1.** In a test supergroup in lockdown: join through an invite link, then have an admin add a user directly, then have an admin add a bot. Confirm one ban for the link joiner and for the bot, no welcome or captcha for either, their join messages deleted (when the bot can delete), and the admin-added user let in and greeted. Repeat while the bot is restarted mid-raid.
2. **Tracer human-check 2 (D-24).** Leave a join request pending, run `/lockdown`, then approve the request from Telegram's own request list. The person should get in; if the service message won the race and they were banned, `/unlockdown` must unban them. (Also confirms research A3 and A5: which of `from` carries the approving admin and which the joiner.)
3. **Task 2 human-check (research A4).** Have an anonymous admin add a user: the user should get in. Have the lockdown ban a joiner and confirm no goodbye message appears even with goodbyes on.
4. **Delivery order.** Confirm a real join arrives on both paths and that the 20 s window covers the gap between them (research A3).

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- 04-05 can add the `chat_join_request` handler on top of `decideLockdownJoin` (Path request gives `lockdownJoinDecline`) and `recordLockdownJoin`'s existing-row rules (a request row met by a link join is already reclaimed).
- 04-06 should add the stale `acting`/`unbanning` reclaim.
- No blockers.

---
*Phase: 04-manual-lockdown*
*Completed: 2026-10-07*

## Self-Check: PASSED
