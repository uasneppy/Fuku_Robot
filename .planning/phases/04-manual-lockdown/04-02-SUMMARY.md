---
phase: 04-manual-lockdown
plan: 02
subsystem: lockdown
tags: [telegram, lockdown, anonymous-admin, gotgbot, gorm, race-safety]

requires:
  - phase: 04-manual-lockdown
    provides: "plan 04-01: chat_lockdowns state machine (Start, GetActiveFresh, BeginLift, FinishLift), raw permission helpers, /lockdown, /unlockdown, requireLockdownAuthority, lockdownFake and newLockdownEnv"
provides:
  - "/lockdownstatus: live-checked, read-only report of whether the group is locked, since when (UTC), by whom, why, joiners removed so far, failed bans, declined requests and a hand-change warning"
  - "/unlockdown that restores first, records second, replaces a hand edit and says so, stays locked on a failed restore or record write, and lifts once when two admins press it together"
  - "chat_status.PromptAnonAdminProof: the proof prompt that ignores the chat's AnonAdmin mode, used by all three lockdown commands"
  - "lockdown.GetCurrentFresh and lockdown.TallyJoiners"
  - "14 locale keys in all 7 locale files, an extended lockdown_help_msg, the docs page and AGENTS.md rules"
affects: [04-03 join guard and worker (extends /unlockdown after the lift record, reads TallyJoiners), 04-05, 04-06, 04-08 staff panel marker]

actuals:
  tokens: 22400
  tasks: 3
  commits: 7

tech-stack:
  added: []
  patterns:
    - "Restore-then-record: the Telegram write comes first and the conditional database update (one winner) second, so a failure leaves the lockdown active"
    - "Test seam as a package variable (lockdownBeginLift) to force a record-write failure without a mock library"
    - "A barrier in the fake's setChatPermissions hook makes a two-admin race deterministic with no sleeps for ordering"
    - "Anonymous-capable checks and refusals read c.Chat and reply through c.Msg, never the update"

key-files:
  created:
    - alita/modules/lockdown_status.go
    - alita/modules/lockdown_status_test.go
    - alita/modules/lockdown_lift_test.go
    - alita/modules/lockdown_anon_test.go
  modified:
    - alita/db/lockdown/repository.go
    - alita/db/lockdown/repository_test.go
    - alita/modules/lockdown.go
    - alita/modules/lockdown_fake_test.go
    - alita/utils/chat_status/chat_status.go
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
  - "/lockdownstatus checks the viewer live (creator or any administrator) and is read-only; the hand-change comparison is skipped while the lock is unconfirmed, because the permissions may not have changed yet and a comparison would raise a false warning"
  - "The same unconfirmed-lock guard applies at the lift: manual_change is only judged for a confirmed lock"
  - "A failed getChat before the lift only skips the manual-change comparison; it never blocks the lift"
  - "Lockdown commands use their own requireLockdownGroup and lockdownRefuse (c.Chat and c.Msg) instead of helpers.RequireGroup and PermissionResponder, which find no chat once the anonymous-admin proof has cleared the callback query from the update"

patterns-established:
  - "requireLockdownAuthority answers an anonymous admin with PromptAnonAdminProof first, refuses that message, and the re-run as the tapper is checked live like anyone else"
  - "/unlockdown order: read current row, compare live permissions, restore, BeginLift (one winner), FinishLift, announce"

requirements-completed: [LOCK-05, LOCK-06, LOCK-07, LOCK-09]

plan_head_before: 741a316684fdebd783cca137cbdf42a454bc98f6
plan_head_after: 8c8306c25d4e11549024766010b817b0fab9f92d

coverage:
  - id: D1
    description: "/lockdownstatus shows locked or not, since when (UTC), who, why, joiners removed, failed bans, declined requests, the unconfirmed note, a hand-change warning, or 'could not read the permissions', and a lifting lockdown's progress; it writes nothing and a plain member is refused"
    requirement: "LOCK-09"
    verification:
      - kind: integration
        ref: "alita/modules/lockdown_status_test.go#TestLockdownStatusCommand"
        status: pass
      - kind: unit
        ref: "alita/db/lockdown/repository_test.go#TestLockdownCurrentAndTally"
        status: pass
    human_judgment: false
  - id: D2
    description: "/unlockdown restores the exact snapshot (including {} and an all-false set), records manual_change only when the live permissions differ, says so, and restores before it records"
    requirement: "LOCK-07"
    verification:
      - kind: integration
        ref: "alita/modules/lockdown_lift_test.go#TestLockdownLiftManualChange"
        status: pass
    human_judgment: true
    rationale: "Exactness against a real getChat answer (research A1) still needs the live check carried over from plan 04-01"
  - id: D3
    description: "A failed restore replies with Telegram's reason and what to do, keeps the lockdown active with no lifter and unbans nobody; a failed record write keeps it active and a later /unlockdown lifts it"
    requirement: "LOCK-05"
    verification:
      - kind: integration
        ref: "alita/modules/lockdown_lift_test.go#TestLockdownLiftRestoreFailure"
        status: pass
      - kind: integration
        ref: "alita/modules/lockdown_lift_test.go#TestLockdownLiftRecordFailure"
        status: pass
    human_judgment: false
  - id: D4
    description: "Two /unlockdown commands at the same moment produce one 'Lockdown lifted' reply and one lift record; no lockdown, or a lift still unbanning, gets its own reply and no Telegram write"
    requirement: "LOCK-06"
    verification:
      - kind: integration
        ref: "alita/modules/lockdown_lift_test.go#TestUnlockdownLiftsOnce (SQLite, -race -count=10)"
        status: pass
      - kind: integration
        ref: "alita/modules/lockdown_lift_test.go#TestUnlockdownWithoutLockdown"
        status: pass
      - kind: integration
        ref: "BeginLift conditional update on PostgreSQL 16"
        status: unknown
    human_judgment: true
    rationale: "The one-winner guarantee is proven on SQLite; the PostgreSQL run was not possible in this agent"
  - id: D5
    description: "An anonymous admin's /lockdown, /unlockdown and /lockdownstatus always get the proof button, with AnonAdmin mode on or off; the tapper is checked live and is the one recorded; a stale admin-list entry or a missing restrict right is refused"
    requirement: "LOCK-06"
    verification:
      - kind: integration
        ref: "alita/modules/lockdown_anon_test.go#TestLockdownAnonymousAdminProof"
        status: pass
    human_judgment: true
    rationale: "The fake proves the flow; a real anonymous admin tapping the real proof button in a live supergroup is a UAT item"

duration: ~25min
completed: 2026-10-07
status: complete
---

# Phase 4 Plan 02: Status, safe lift and anonymous admins Summary

**/lockdownstatus reports a lockdown live and read-only, /unlockdown restores first and records second so it never reports a lift it did not achieve and lifts once under a race, and anonymous admins always prove who they are through a new `PromptAnonAdminProof` before any lockdown command.**

## Performance

- **Duration:** about 25 min (start time was not captured, so this is an estimate)
- **Started:** about 2026-10-07T01:18Z
- **Completed:** 2026-10-07T01:43Z
- **Tasks:** 3 (a tracer and two auto tasks, all tdd="true")
- **Files:** 19 changed (4 created, 15 modified), 1468 insertions, 15 deletions
- **Commits:** 7 before this SUMMARY (measured: `git rev-list --count 741a316..8c8306c`)

## Accomplishments

- `/lockdownstatus` (any live creator or administrator): not locked; or locked since a UTC time by a named person with the reason, the count of joiners removed so far, failed bans and declined requests when non-zero, and a warning when the group's permissions no longer equal the locked set. A lifting lockdown shows the lifter and "done of total". It makes no write call and leaves every `updated_at` unchanged (asserted in every subtest).
- `/unlockdown` is now: current row (no Telegram call when none or already lifting), live permission comparison, **restore**, then `BeginLift`, then `FinishLift`, then the reply. A hand edit is replaced and the reply says so; a failed restore or failed record write replies with what happened and what to do and leaves the lockdown active, unbanning nobody; the loser of a simultaneous lift is told it was already lifted.
- Anonymous admins: `requireLockdownAuthority` returns the proof prompt first (even with AnonAdmin mode on) and refuses that message; `lockdown`, `unlockdown` and `lockdownstatus` are registered for re-entry, so the tapper is checked live and recorded.
- `GetCurrentFresh` (active row, else newest lifting row) and `TallyJoiners` (count per state for one lockdown) in the repository.
- 14 locale keys in all 7 files, `lockdown_help_msg` extended twice, the generated docs page, and three AGENTS.md rules.

## Task Commits

1. **Task 1 (tracer): /lockdownstatus end to end**
   - RED `664bc6d` test(04-02): `TestLockdownStatusCommand` fails on all 13 subtests (no handler answers); `TestLockdownCurrentAndTally` fails on assertions against no-op `GetCurrentFresh`/`TallyJoiners` scaffolding that only lets it compile
   - GREEN `2a5a085` feat(04-02): repository functions, `lockdown_status.go`, 9 locale keys, docs
2. **Task 2: the lift**
   - RED `8da87c7` test(04-02): four of five tests fail on assertions; the `lockdownBeginLift` seam is added unused so they compile
   - GREEN `3ccc035` feat(04-02): restore-then-record `unlockdown`, 5 locale keys, AGENTS.md
3. **Task 3: anonymous admins**
   - RED `7be1e40` test(04-02): `TestLockdownAnonymousAdminProof` fails on all four subtests
   - GREEN `c0a5021` feat(04-02): `PromptAnonAdminProof`, the anonymous branch, registrations, help line, AGENTS.md
   - Fix `8c8306c` fix(04-02): see Deviations (the first GREEN broke an unrelated test)

**Tracer gate:** the tracer's `<verify>` has only automated commands, so under end-of-phase mode they were re-run and passed ("Tracer verified end-to-end - expanding"). No human check exists on this tracer.

## Files Created/Modified

See the frontmatter `key-files`. Behaviour lives in `alita/modules/lockdown.go` (`unlockdown`, `requireLockdownAuthority`, `lockdownRefuse`, `requireLockdownGroup`, the `lockdownBeginLift` seam, anonymous registrations), `alita/modules/lockdown_status.go`, `alita/db/lockdown/repository.go` and `alita/utils/chat_status/chat_status.go` (`PromptAnonAdminProof`).

## Decisions Made

- The hand-change comparison, in both `/lockdownstatus` and `/unlockdown`, is only made once Telegram confirmed the lock (`locked_at` set). Before that the live permissions may still be the pre-lockdown ones and a comparison would warn about a change nobody made. The plan did not say this; it follows the same "never claim what you did not check" rule.
- The total for a lifting lockdown is `unbanned + kept + unban_failed` done plus `banned + unbanning` still to do, exactly as the plan states. Joiners still `pending` or `acting` during a lift are not counted; plan 04-03's worker cancels pending rows.
- The race test holds both lifts at the restore call (a barrier in the fake's `setChatPermissions` hook, with a 10 s deadline and an assertion that both arrived) so each has read the lockdown as active before either records the lift. No sleep decides ordering.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] The anonymous-admin proof's re-run found no chat, so every check and refusal after it was silent**
- **Found during:** Task 3 GREEN (tests still failed after `PromptAnonAdminProof` and the registrations)
- **Issue:** `verifyAnonymousAdmin` sets `ctx.CallbackQuery = nil`, which clears the callback query of the embedded `*gotgbot.Update`. After that `chat_status.extractChatFromContext`, behind `helpers.RequireGroup` and `PermissionResponder`, finds no chat: `RequireGroup` returns false and every refusal reply is dropped, with no log. The plan assumed "the existing pipeline re-runs `requireLockdownAuthority` live for the tapper", which only works if the group check and the refusal work after the proof.
- **First attempt (reverted):** made `extractChatFromContext` fall back to `ctx.EffectiveChat`. It fixed the flow but broke `TestUnapproveAllCallbackCancelInvalidAndUnavailableMessage`, which relies on a callback with no message finding no chat (caught by `make test`, commit `c0a5021`).
- **Final fix:** `extractChatFromContext`, `helpers.RequireGroup` and `PermissionResponder` are untouched. The lockdown commands use their own `requireLockdownGroup` (reads `c.Chat`) and `lockdownRefuse` (translates with `c.Tr`, replies through `c.Msg`).
- **Files modified:** `alita/modules/lockdown.go`, `alita/modules/lockdown_status.go`, `alita/utils/chat_status/chat_status.go` and `access_test.go` (back to unchanged apart from `PromptAnonAdminProof`), `AGENTS.md`
- **Verification:** `TestLockdownAnonymousAdminProof` (4 subtests, including the refusal replies after the proof), `TestUnapproveAll...`, and a full `make test` pass
- **Committed in:** `8c8306c`

### Process notes

- **TDD:** in Task 2's RED commit `TestUnlockdownLiftsOnce` and four of six `TestLockdownLiftManualChange` subtests already passed. The one-winner guarantee comes from the `BeginLift` conditional update built in plan 04-01; the new test is a characterisation test that proves it under a forced overlap. The RED commits of Task 1 and Task 2 carry compile scaffolding (no-op repository functions, an unused `lockdownBeginLift` variable) that the GREEN commits replace, as in plan 04-01.
- **Acceptance greps:** the plan's literal `grep -n 'Name: "lockdownstatus"'` does not match because gofmt aligns the field (`Name:           "lockdownstatus"`); the equivalent `grep -nE 'Name:\s+"lockdownstatus"'` matches. All other acceptance criteria were run as written and pass.

**Total deviations:** 1 auto-fixed (Rule 3), 2 process notes. **Impact:** no scope creep; the fix is confined to lockdown code.

## Issues Encountered

- **A likely pre-existing bug outside this plan (not fixed, for the orchestrator):** the same silent failure after the anonymous-admin proof should affect every migrated command that lists `helpers.RequireGroup()` in its checks and re-enters through `anonPipelineHandler` (the `ban` family, `restrict`, `unrestrict`): `RequireGroup` returns false and sends nothing. I proved it only for the lockdown commands (debug output showed check 0, `RequireGroup`, returning false); I did not run a ban flow. Logged in `deferred-items.md`.
- **PostgreSQL not run.** `TestUnlockdownLiftsOnce` and `TestLockdownCurrentAndTally` ran on SQLite only. The `su postgres` cluster commands carried over from plan 04-01's summary were not attempted. `BeginLift` as one conditional UPDATE is the same statement 04-01 already exercises on SQLite; run it on PostgreSQL from the main checkout.
- **`make lint` cannot run here** (golangci-lint 2.5.0 was built with go1.25 and refuses the go1.26.0 module). `gofmt -l` is clean on every touched file (it lists `alita/modules/greetings_command_test.go`, which this plan does not touch), and `go vet -tags testtools` is clean on the touched packages.
- **`make test`:** the first run failed on `TestUnapproveAllCallbackCancelInvalidAndUnavailableMessage`, caused by my first Task 3 fix (see Deviations). After the fix `make test` exits 0 with no `--- FAIL`. The flaky `TestStaffActionNonStaffUnchanged/tmute_as_a_reply` did not fail.
- `gsd_run` is not available in this agent, so the broken-windows ledger (`.planning/WINDOWS.md`) was not appended. STATE.md, ROADMAP.md and REQUIREMENTS.md are the orchestrator's. Requirement IDs LOCK-05, LOCK-06, LOCK-07 and LOCK-09 are shared with later plans, so none should read complete from this plan alone.

## Verification Results

| Check | Result |
|-------|--------|
| `TestLockdownStatusCommand` (13 subtests), `TestLockdownCurrentAndTally` (6), `-race -count=1` | pass |
| `TestLockdownLiftManualChange` (6), `TestLockdownLiftRestoreFailure`, `TestUnlockdownWithoutLockdown` (3), `TestLockdownLiftRecordFailure` | pass |
| `TestUnlockdownLiftsOnce` `-race -count=10` | pass, no DATA RACE |
| `TestLockdownAnonymousAdminProof` (4 subtests) | pass |
| `go test -tags testtools -race -count=1 -run 'Lockdown\|Unlockdown' ./alita/modules ./alita/db/lockdown` and `./alita/utils/chat_status` | pass |
| `make test` (whole repository, after the final fix) | exit 0, no `--- FAIL`; PostgreSQL-only tests skipped as the result checker allows |
| `make check-translations`, `make check-docs` (after `make generate-docs`) | pass, no drift |
| `CGO_ENABLED=0 go build ./...`, `go vet -tags testtools` on touched packages | clean |
| `git diff --exit-code go.mod go.sum` | clean |
| Task acceptance greps (status, repository, seam, restore-before-record order, `unbans nobody`, `PromptAnonAdminProof`, registration counts 2 and 1, locale key counts equal to en in all 6 other files) | pass |
| PostgreSQL 16 run of the new repository test and the lift race | not run |
| `make lint` | cannot run in this environment |

## Known Stubs

None in code. One thing reads ahead of behaviour that plan 04-03 adds: `/lockdownstatus` shows "Joiners removed so far", failed bans and declined requests from `chat_lockdown_joiners`, but nothing writes joiner rows until the join guard of plan 04-03, so in production the counts are 0 until then (the tests seed rows directly). A lift that finds a `lifting` row with unfinished joiner rows says the lift is under way; the worker that finishes it is plan 04-03's.

## Threat Flags

None beyond the plan's register: T-04-06 (anonymous elevation, mitigated and tested), T-04-07 (concurrent lift, tested with an enforced overlap), T-04-08 (lift reported without a restore, tested) and T-04-09 (status to non-admins, tested). No new endpoint, auth path or schema.

## End-of-Phase UAT (needs a live Telegram supergroup)

1. With the group's AnonAdmin mode on, post `/lockdown test` as an anonymous admin: expect the proof button, and after the tap the lock notice naming the person who tapped.
2. `/lockdownstatus` as an anonymous admin and as a plain member: status after the tap; refusal for the member.
3. Lock, reopen sending permissions by hand, `/lockdownstatus` (warning), `/unlockdown` (reply says a manual change was replaced, Permissions screen back to the original).
4. Lock, remove the bot's "restrict members" right, `/unlockdown`: reply says the restore failed and what to do, group still locked. Restore the right, `/unlockdown` again: lifts.
5. Two admins send `/unlockdown` at the same moment: one "Lockdown lifted", one "already lifted".
6. Carried over from plan 04-01: capture the raw `getChat` permissions JSON as a fixture (research A1).

## Next Phase Readiness

- Ready: `GetCurrentFresh`, `TallyJoiners`, `lockdownBeginLift`, `lockdownRefuse`, `requireLockdownGroup`, the `seedLockdownJoiner` helper and the proof flow for plan 04-03 (join guard, worker, `lockdown_lifted_unbanning` after the lift record) and later plans.
- Needs action: run the PostgreSQL commands from the plan 04-01 summary plus `TestUnlockdownLiftsOnce` and `TestLockdownCurrentAndTally` against PostgreSQL on a machine that allows `su`; decide on the `deferred-items.md` item.

## Self-Check: PASSED

All four created Go files exist on disk and the seven commits (`664bc6d`, `2a5a085`, `8da87c7`, `3ccc035`, `7be1e40`, `c0a5021`, `8c8306c`) are present in `git log 741a316..HEAD`. The PostgreSQL, lint and live-Telegram items above remain open; this self-check covers only files and commits.

---
*Phase: 04-manual-lockdown*
*Completed: 2026-10-07*
