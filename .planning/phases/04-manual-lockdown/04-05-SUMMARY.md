---
phase: 04-manual-lockdown
plan: 05
subsystem: lockdown
tags: [telegram, lockdown, join-requests, greetings, antiraid, isolation, gotgbot, fail-closed]

requires:
  - phase: 04-manual-lockdown
    provides: "plans 04-03/04-04: group -7 join guard, decideLockdownJoin (request -> decline), joiner repository (RecordJoin, ReclaimJoin, ClaimJoiner, MoveJoiner, CancelPending), paced worker, lift unban and tally, lockdownFake and newLockdownEnv"
provides:
  - "Join-request guard (lockdownOnJoinRequest, group -7): records a request row and ends handling, so auto-approve and the approve card never see it; fails closed"
  - "Worker decline step (lockdownDeclineOne): paced declineChatJoinRequest, a gone request counts as declined, three attempts, rate limit costs none"
  - "Greetings Accept button refusal during a lockdown (D-25), Decline and Ban unchanged"
  - "Antiraid onJoin steps aside while a lockdown is active"
  - "Isolation test for two locked groups in both lift orders"
  - "lockdownActiveLookup test seam, test helpers joinRequest, pressJoinRequest, lastAnswer, addChat, sendIn, joinIn"
affects: [04-06 (stale acting/unbanning claims: a request row stuck in acting is also unreclaimed), 04-08, Phase 6 (retires antiraid)]

actuals:
  tokens: 21000
  tasks: 2
  commits: 4

tech-stack:
  added: []
  patterns:
    - "The request path fails closed (a failed read or write ends handling, the request stays pending); the member and service paths fail open because a joiner is still muted by the locked default"
    - "A request from someone with a finished row is reclaimed at once with a one second margin on the cut-off, so a row changed this very moment is still reclaimable; no dedupe window because Telegram delivers each request once"
    - "A request recorded just as the lift starts is withdrawn through lockdownPendingHeld and handed back to the group's own handling (ext.ContinueGroups)"

key-files:
  created:
    - alita/modules/lockdown_requests_test.go
    - alita/modules/lockdown_isolation_test.go
  modified:
    - alita/modules/lockdown_guard.go
    - alita/modules/lockdown_worker.go
    - alita/modules/greetings.go
    - alita/modules/antiraid.go
    - alita/modules/lockdown_fake_test.go
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
  - "A request that meets a row still pending or acting is left to the worker (no reclaim); only finished rows (lockdownReclaimStates) are reclaimed, any age"
  - "When the lift started between the lockdown read and the row write, the request is handed back to the group's own handling instead of being left unhandled (lockdownPendingHeld withdraws the row)"
  - "The Accept button reads the lockdown with lockdown.GetActiveFresh directly (as the plan says), not through the guard's test seam; an unconfirmed lockdown (locked_at NULL) also refuses Accept, while the guard needs a confirmed one, so the button is the more conservative of the two"
  - "Antiraid fails open on an unreadable lockdown (it carries on as before), because the lockdown guard already ran at group -7"

patterns-established:
  - "Per-chat test helpers (sendIn, joinIn, callsIn, writesTo) next to the single-chat env helpers, for tests that need a second group"

requirements-completed: [LOCK-01, LOCK-04]

duration: ~25 min
completed: 2026-10-07
status: complete

plan_head_before: 7305543337115967d3dc1e57a0ee0e40fe500f8b
plan_head_after: 685d6bcecc17d0d03b45ad8e278a3dda4a403270

coverage:
  - id: D1
    description: "A join request that arrives during a confirmed lockdown is recorded and declined by the worker, never approved by auto-approve and with no approve card; a repeat request is declined again; a request Telegram reports gone counts as declined; requests pending at the lift are cancelled"
    requirement: "LOCK-01"
    verification:
      - kind: integration
        ref: "alita/modules/lockdown_requests_test.go#TestLockdownGuardDeclinesJoinRequest"
        status: pass
    human_judgment: true
    rationale: "Whether Telegram delivers a chat_join_request update to a bot with can_invite_users in a real supergroup, and lets a declined person request again at once (research A11), is a live UAT item"
  - id: D2
    description: "If the guard cannot read the lockdown, the request is left pending and never reaches auto-approve"
    requirement: "LOCK-01"
    verification:
      - kind: integration
        ref: "alita/modules/lockdown_requests_test.go#TestLockdownJoinRequestFailsClosed"
        status: pass
    human_judgment: false
  - id: D3
    description: "The bot's own Accept button on a join-request card answers with an alert and approves nothing while a lockdown is active; Decline still works; Accept works again after the lift"
    requirement: "LOCK-01"
    verification:
      - kind: integration
        ref: "alita/modules/lockdown_requests_test.go#TestLockdownAcceptButtonRefuses"
        status: pass
    human_judgment: true
    rationale: "The alert wording and how it reads in the Telegram client are a UAT item"
  - id: D4
    description: "Two groups locked at the same time are independent in either lift order: separate rows per group, the other group's permissions, row and joiners untouched, no call or post made to it by a lift"
    requirement: "LOCK-04"
    verification:
      - kind: integration
        ref: "alita/modules/lockdown_isolation_test.go#TestLockdownAffectsOnlyItsChat"
        status: pass
    human_judgment: false
  - id: D5
    description: "While a lockdown is active the antiraid join handler does nothing in that group (no temp-ban of an admin-added user, no join counting); it works as before after the lift"
    requirement: "LOCK-04"
    verification:
      - kind: integration
        ref: "alita/modules/lockdown_isolation_test.go#TestAntiRaidStepsAsideDuringLockdown"
        status: pass
    human_judgment: false
---

# Phase 4 Plan 05: Join requests and isolation between groups Summary

**Join requests during a lockdown are recorded and declined by the worker (never approved by auto-approve or the bot's own Accept button, failing closed), two locked groups are independent in either lift order, and the old /antiraid join handler steps aside while a lockdown is active.**

## Performance

- **Duration:** about 25 min
- **Tasks:** 2 (Task 1 tracer, Task 2 auto, both TDD)
- **Commits:** 4 (2 test, 2 feat); measured `git rev-list --count 7305543..HEAD` before this summary commit
- **Files:** 16 touched by the code commits (2 created, 14 modified)

## Accomplishments

- **Request guard** (`lockdownOnJoinRequest`, group -7): reads the lockdown through the new `lockdownActiveLookup` seam; a read error ends handling (request stays pending); no confirmed lockdown continues to greetings; otherwise it records a row with `join_path` request and ends the update, so `pendingJoins` at group 0 never sees it. `recordLockdownRequest` reclaims a finished row at once (any age), leaves a pending or acting row to the worker, and withdraws the row when the lift started first.
- **Decline step** (`lockdownDeclineOne`, called from `lockdownBanPending` for rows whose path is request): paced and timed `declineChatJoinRequest`; success or a gone request (`isJoinRequestGone`) moves the row to declined, a rate limit or shutdown puts it back for free, any other error costs an attempt and the third is `decline_failed`. Pending requests at a lift are already cancelled by `CancelPending`.
- **Accept button** (`joinRequestHandler`): after the admin and invite checks, `accept` reads the lockdown fresh; an active lockdown or a failed read answers an alert (`greetings_join_request_lockdown`, ShowAlert) and approves and edits nothing (D-25). Decline and Ban are unchanged.
- **Isolation** (LOCK-04): `TestLockdownAffectsOnlyItsChat` runs two supergroups in both lift orders; it passed with no production change, which shows every lockdown query was already keyed by `chat_id` or `lockdown_id` and the worker uses the row's own chat.
- **Antiraid**: `onJoin` returns `ext.ContinueGroups` before any work when `lockdown.GetActiveFresh` shows a confirmed lockdown; a failed read is logged and antiraid carries on. Works as before after the lift.
- Locale key `greetings_join_request_lockdown` in all 7 files, `lockdown_help_msg` extended in all 7, generated docs refreshed, AGENTS.md updated (guard bullet, request path, Accept button, isolation and antiraid step-aside).

## Task Commits

1. **Task 1 (tracer): join requests during a lockdown** - `cfa7aa5` (test, RED), `570801a` (feat, GREEN)
2. **Task 2: isolation between groups and antiraid step-aside** - `b149815` (test, RED), `685d6bc` (feat, GREEN)

**Plan metadata:** committed after this file (`docs(04-05): complete join requests and isolation plan`).

## TDD Gate Compliance

Both tasks have a `test(04-05)` commit before their `feat(04-05)` commit (`git log --oneline -4`). RED evidence (target tests failing on the planned behaviour, not on a build error):

- Task 1: `TestLockdownGuardDeclinesJoinRequest` ("approveChatJoinRequest calls for the raider = 1, want none during a lockdown"), `TestLockdownJoinRequestFailsClosed` ("approveChatJoinRequest calls = 1, want none"), `TestLockdownAcceptButtonRefuses` ("answer ... want the lockdown alert", "approveChatJoinRequest calls = 1", "editMessageText calls = 1"). The RED commit carries the compile-only `lockdownActiveLookup` variable (the guard did not use it yet).
- Task 2: `TestAntiRaidStepsAsideDuringLockdown` ("banChatMember calls for the admin-added user = 1, want none", "antiraid joins counted during the lockdown = 1, want 0").
- Already green at RED, expected: `TestLockdownAffectsOnlyItsChat` (both subtests) pins behaviour the earlier plans already implemented; the plan says "fix anything the isolation test exposes" and it exposed nothing. The `gsd_run check tdd-red-evidence` record was not produced (the plan is `type: execute`, not `type: tdd`).

Tracer gate: the tracer's automated `<verify>` passed end to end (logged "Tracer verified end-to-end, expanding"); the live-Telegram check is an end-of-phase UAT item below, not a stop.

## Decisions Made

See `key-decisions` above.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Missing Critical] Request recorded as the lift starts is handed back, not stranded**
- **Found during:** Task 1 (designing `recordLockdownRequest`)
- **Issue:** the plan's pseudo-code ends every request with ext.EndGroups. If the lift started between the lockdown read and the row write, the row is withdrawn by `lockdownPendingHeld` and no worker would ever decline it, leaving a request that is neither declined nor seen by greetings.
- **Fix:** `recordLockdownRequest` returns whether the lockdown holds the request; when the lift got there first the handler returns ext.ContinueGroups so the group's own join-request handling applies.
- **Files modified:** alita/modules/lockdown_guard.go
- **Committed in:** 570801a

**2. [Rule 2 - Missing Critical] One second margin on the reclaim cut-off**
- **Found during:** Task 1
- **Issue:** the plan says "notAfter = now (any age)". `ReclaimJoin` matches `updated_at < notAfter`, and a row the worker finished a microsecond ago could miss that strict comparison.
- **Fix:** notAfter is `time.Now().Add(time.Second)`; the update is still conditional on the finished states, so the margin only widens "any age".
- **Files modified:** alita/modules/lockdown_guard.go
- **Committed in:** 570801a

---

**Total deviations:** 2 auto-fixed (2 missing critical). **Impact on plan:** neither changes behaviour the plan specified; both make the request path correct at its edges.

## Issues Encountered

- A Bash command that mentioned both `git` and other commands in one compound line was refused by the sandbox twice ("too complex to verify"); commands were split into plain separate calls.
- Pre-existing, not touched: `gofmt -l alita/modules` lists `greetings_command_test.go` (noted in 04-04).

## Known Limitations (carried forward, not stubs)

- A request row stuck in `acting` after a crash is not reclaimed (the stale `acting` claim recovery is plan 04-06, and it covers request rows because they use the same claim).
- No per-user suppression of a raid that loops join requests: each request costs one paced decline (T-04-19, accepted; research A11 and Open Question 6, revisited in Phase 6).
- The Accept button also refuses while a lockdown is active but not yet confirmed (`locked_at` NULL), where the guard lets the request through to greetings. The window is the single lock call; the button is the more conservative of the two.
- The Accept button's lockdown read does not go through the guard's test seam (the plan names `lockdown.GetActiveFresh` there), so its fail-closed branch (a failed read treated as locked) has no test of its own.

## Known Stubs

None.

## Threat Flags

None beyond the plan's `<threat_model>`: T-04-17 (request approved during a lockdown: `TestLockdownGuardDeclinesJoinRequest`, `TestLockdownAcceptButtonRefuses`, `TestLockdownJoinRequestFailsClosed`) and T-04-18 (a lockdown acting in another group: `TestLockdownAffectsOnlyItsChat`, both lift orders) are mitigated and tested; T-04-19 and T-04-SC are accepted (no package installs, `go.mod` and `go.sum` unchanged).

## Verification Run

- Named plan tests, all with `-tags testtools -race -count=1` and passing: `TestLockdownGuardDeclinesJoinRequest`, `TestLockdownJoinRequestFailsClosed`, `TestLockdownAcceptButtonRefuses`, `TestLockdownAffectsOnlyItsChat`, `TestAntiRaidStepsAsideDuringLockdown`.
- Plan-level: `go test -tags testtools -race -count=1 -run 'AntiRaid|Antiraid|Lockdown|Unlockdown|Greeting|JoinRequest|Captcha' ./alita/modules ./alita/db/lockdown` ok; `-run 'Greeting|JoinRequest|PendingJoins' ./alita/modules` ok.
- `make test`: exit 0 (69.8% coverage, no FAIL lines in the log). The known flaky `TestStaffActionNonStaffUnchanged/tmute_as_a_reply` did not fail.
- `CGO_ENABLED=0 go build ./...`, `go vet -tags testtools ./alita/modules ./alita/db/lockdown .`, `make check-translations` (all present), `make generate-docs` then `make check-docs` (no drift), `gofmt -l` on the touched Go files (clean), `git diff --exit-code go.mod go.sum` (unchanged): all pass.
- Acceptance criteria re-run, all pass: the exact `NewChatJoinRequest(chatjoinrequest.All, lockdownModule.lockdownOnJoinRequest)` and `var lockdownActiveLookup = lockdown.GetActiveFresh` lines match; `greetings_join_request_lockdown` is in greetings.go and once in each of es, fr, hi, id, pt, ru; AGENTS.md contains "Accept button" and "returns at once while a lockdown is active"; `lockdown.GetActiveFresh(chat.Id)` is in antiraid.go; `git log` shows each `test(04-05)` before its `feat(04-05)`.
- `make lint`: not run. The installed golangci-lint 2.5.0 is built with go1.25 and refuses the go1.26.0 module (environment limitation).
- PostgreSQL: not run. No throwaway cluster was set up; the request path uses the existing conditional updates (`RecordJoin`, `ReclaimJoin`, `ClaimJoiner`, `MoveJoiner`, `CancelPending`) with no new SQL, all exercised on SQLite only. Command left for the orchestrator or CI, with the migration chain applied first: `DATABASE_URL=<postgres url> ALITA_TEST_DATABASE=true CGO_ENABLED=1 go test -tags testtools -race -count=1 ./alita/db/lockdown`.

## End-of-Phase UAT Items (live Telegram, not run)

1. **Join request during a lockdown.** In a test supergroup with join requests required and auto-approve on: run `/lockdown`, then request to join from a second account. The request should be declined within a few seconds with no approve card and no welcome; request again and see it declined again; run `/unlockdown` and confirm a new request is handled normally. (Confirms research A11: a declined person can request again at once.)
2. **Accept button.** With auto-approve off, let a request post its approve card, run `/lockdown`, then tap Accept as an admin: expect the alert text and no approval; tap Decline: the request is declined. After `/unlockdown` a new card's Accept approves.
3. **Antiraid coexistence.** With `/antiraid` on and a lockdown active, add a user as an admin: the user gets in and is not temp-banned. After the lift, raid mode bans a self-join as before.
4. **Two groups.** Lock two real groups, lift one, and confirm the other keeps its restricted permissions and its removed joiners.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- 04-06 should add the stale `acting`/`unbanning` claim reclaim; join-request rows share the `acting` claim, so the same recovery covers them.
- No blockers.

---
*Phase: 04-manual-lockdown*
*Completed: 2026-10-07*

## Self-Check: PASSED
