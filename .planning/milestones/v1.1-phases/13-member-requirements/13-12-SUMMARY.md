---
phase: 13-member-requirements
plan: 12
subsystem: modules
tags: [member-requirements, rejoin, leave, hold-reset, mute-marker, kill-switch]
requires:
  - phase: 13-member-requirements
    provides: memberReqUnmutePermissions, memberReqLiftMute, resolveUnmutePermissionsWithLockdown (13-08); isMemberReqMute, memberReqLiveMember, memberReqRecordCtx (13-07); memberReqJoinThenWelcome, memberReqHoldRead, memberReqPunishedMute, memberReqCleanup with captcha, greetings and join-claim rows (13-11); memberreq.TakeHoldsBefore, RestoreHold, ClearHolds (13-01)
provides:
  - MembershipDeps.Reset, called by ProcessSingleJoin after a won join claim and before the join decision (nil-safe)
  - memberReqJoinReset, memberReqLiftOldMute, memberReqJoinDate, memberReqResetTimeout (memberreq_join.go)
  - the leave hook in leftMember (memberreq.ClearHolds right after clearRecentJoinProcessing)
  - the join date on membershipDepsForJoin and processSingleNewMember
  - test helpers mreqJoinAt, mreqJoinNow, mreqServiceJoinAt, mreqServiceJoinNow, mreqLeave, mreqRejoinable, mreqAgeHold, mreqMutedByJoin
affects: [13-13, 13-14, member-requirements]

actuals:
  tokens: 12000
  tasks: 2
  commits: 4

commits: 4
plan_head_before: 50a7b0a3c8cd32c7a22a830852afdb88e24c1d0c
plan_head_after: 83929a5ac2d1f19f571b7d85d8e01483ebd8d700

tech-stack:
  added: []
  patterns:
    - "A reset that takes holds with DELETE ... RETURNING first (one winner across replicas) and puts them back with their ID when its follow-up fails"
    - "A lift only against a live marker: the hold proves nothing about the member's current restriction, the live record does"

key-files:
  created:
    - fuku/modules/memberreq_rejoin_test.go
  modified:
    - fuku/modules/membership.go
    - fuku/modules/greetings.go
    - fuku/modules/greetings_command_test.go
    - fuku/modules/memberreq_join.go
    - fuku/modules/chat_permissions.go
    - AGENTS.md
    - .planning/REQUIREMENTS.md

key-decisions:
  - "The reset deletes first and lifts second (memberreq.TakeHoldsBefore, then the lift, RestoreHold on failure), as the plan says: one DELETE ... RETURNING decides the single acting replica. The accepted cost is that a crash between the delete and the lift can strand a mute with no hold; an admin's /unmute ends it"
  - "A punished mute hold whose live record no longer carries the marker (an admin's mute, or no restriction left) is deleted and nothing is lifted: the hold proves nothing about the member's current mute"
  - "A pending (never punished) mute hold is deleted without a lift, as the plan's prohibition allows lifting only a punished one"
  - "A join date of 0 or less is read as now (memberReqJoinDate), and a zero time.Time given to membershipDepsForJoin as well"

requirements-completed: [MREQ-04, MREQ-08]

duration: about 40 min of work (plus about 12 min of background family and settings runs)
completed: 2026-11-07
status: complete
---

# Phase 13 Plan 12: The leave hook and the rejoin reset Summary

**A detected leave now clears the member's hold, and the join that wins its dedupe claim settles every older hold through `MembershipDeps.Reset`: a punished requirement mute whose marker is still live on the member is lifted with the group's (or an active lockdown's stored) permissions and the hold restored when any step fails, so a kicked member is checked again at their rejoin, a banned member stays banned, a muted member who leaves and rejoins is never stranded, and one violation is punished once whether it is seen at the join or at a message.**

## Performance

- **Tasks:** 2 of 2 (task 1 tracer, task 2 TDD)
- **Commits:** 4 (RED and GREEN for each task), measured from `plan_head_before` with `git rev-list --count`. The SUMMARY commit follows.

## Accomplishments

- **Leave hook (task 1).** `leftMember` calls `memberreq.ClearHolds` right after `clearRecentJoinProcessing`, on `context.WithTimeout(tracing.UpdateContext(ctx), memberReqRecordTimeout)`. An error is logged at Error with the chat and user IDs and never stops the captcha cleanup or the goodbye.
- **Reset (task 2).** `ProcessSingleJoin` calls `deps.Reset(user)` after a won `Claim` (or a nil `Claim`) and before the approval check; a lost claim, the bot itself and a nil `Reset` skip it. `memberReqJoinReset(bot, chat, user, joinDate)` runs `memberreq.TakeHoldsBefore`; for each deleted punished mute hold it reads the live record (`memberReqLiveMember`) and lifts only when `isMemberReqMute(member, hold.MuteUntil)`, with permissions from `memberReqUnmutePermissions` (so `resolveUnmutePermissionsWithLockdown`) through `memberReqLiftMute`; on any failure it calls `memberreq.RestoreHold` with the hold as read (ID, notice ID, marker, punished time). Every log line carries IDs only. A hold created at or after the join date is never touched.
- **Join date.** `membershipDepsForJoin(bot, chat, threadID, joinDate)` and `processSingleNewMember(..., joinDate)`; `newMember` passes `memberReqJoinDate(ctx.ChatMember.Date)`, `cleanService` `memberReqJoinDate(msg.Date)`; a date of 0 or less is read as now.
- **Comment and docs.** The `resolveUnmutePermissions` comment and the AGENTS.md sentence list the rejoin reset; the AGENTS.md join bullet, the `chat_requirement_holds` Data bullet and the known limit (the goodbye after a requirement kick or ban is kept for parity with `/kick`) are updated in the task 2 commit.
- **Tests (`memberreq_rejoin_test.go`).**
  - `TestMemberReqKickBan`: a kick rule, leave update, rejoin gives a second unban call and a second notice and no welcome; a ban rule leaves the member kicked with zero `unbanChatMember` calls; a member who leaves has the warn hold cleared and a new warn starts a new episode; a `ClearHolds` failure (the update context cancelled) is logged by IDs and the goodbye is still sent.
  - `TestMemberReqRejoinResetsMuteHold`: the muted member's rejoin through the service message gives mute, lift (the group's permissions, no end date) and a new mute, a new hold and a second notice; a refused `restrictChatMember`, `getChat` or `getChatMember` restores the hold with its ID, notice ID and marker and the member stays muted; a hold created after the join date survives; an admin's mute (another end date) is never lifted; a warn, kick or ban hold is only deleted with no write call; during an active lockdown the lift uses the stored pre-lockdown permissions; a join dated 0 is read as now.
  - `TestMemberReqJoinResetOrder`: claim, reset, welcome on a won claim; claim alone on a lost one; nothing for the bot.
  - `TestMemberReqJoinAndMessageOnce`: join then message, message then join (warn: one warns row, one notice) and a join racing a message on a second replica (mute: one restrict call, one notice, one hold).

## Task Commits

1. Task 1 (tracer): `5e5c2cf` test RED, `7139e88` feat GREEN
2. Task 2: `e4d13b7` test RED, `83929a5` feat GREEN (also AGENTS.md)

## TDD Gate Compliance

| Gate | Commit | Evidence |
|------|--------|----------|
| RED task 1 | 5e5c2cf | No stand-in needed. All four `TestMemberReqKickBan` subtests failed on assertions (`hold after the kick's leave update = &{...}, want it cleared`, the same for the ban and the warn, and `no error line naming user ... and chat ...`) |
| GREEN task 1 | 7139e88 | all four pass, with `TestMemberJoinAndLeaveSendConfiguredGreetings` and `TestLeftMemberDeletesPendingCaptchaAttemptAndCleanGoodbye` unchanged |
| RED task 2 | e4d13b7 | Compile-only stand-ins: the `MembershipDeps.Reset` field (never called) and an empty `memberReqJoinReset`. Failing on assertions: the muted rejoiner (`restrictChatMember calls = 1, want the mute, its lift and the mute of the new check`), the admin-mute, warn/kick/ban and no-date subtests (`want it deleted`), the lockdown subtest and `TestMemberReqJoinResetOrder` (`claim,welcome, want claim,reset,welcome`). Passing at once as guards of behaviour that already held: the refused-lift subtests (nothing deletes a hold yet), the hold created after the join date, and all three `TestMemberReqJoinAndMessageOnce` subtests (the 13-07 latch) |
| GREEN task 2 | 83929a5 | all pass; the refused-lift subtests now pass because the reset deletes and then restores |

`gsd_run check tdd-red-evidence` was not run (the plan is `type: execute`); the evidence is the real test output.

## How the kill switch was checked (T-13-29, T-13-56)

- `grep -rn memberReqEnforcementLive --include=*.go .` outside tests finds only the declaration, its comment and `memberReqLive()`'s `Load()` in `fuku/modules/memberreq.go`; in tests only `withMemberReqLive` in `memberreq_env_test.go` (`Load`, `Store`, previous value restored in `t.Cleanup`). **This plan writes it nowhere**, `main.go` is untouched, and the new tests set it only through `newMemberReqEnv`'s `withMemberReqLive`.
- **Why the leave hook and the reset are safe without the switch.** `ClearHolds` and `TakeHoldsBefore` only DELETE rows of `chat_requirement_holds`, and the reset's only Telegram write is the lift of a requirement mute proven by the hold's marker on the live record. While the switch is off nothing creates a hold (the watcher, the join entry and the captcha guard are all inert), so both find no row: a leave costs one statement and a join costs one statement, no Telegram call, no restrict, kick, ban, warn or delete. Neither ever bans or unbans.

## Verification (real results)

- **Task 1 command** (`TestMemberReqKickBan|TestMemberJoinAndLeaveSendConfiguredGreetings|TestLeftMemberDeletesPendingCaptchaAttemptAndCleanGoodbye`): `ok`. **Task 2 command** (the new tests plus `TestDecideJoin`, the `TestProcessSingleJoin*` family, `TestProcessJoinsMultiMember`, `TestMemberJoinAndLeaveSendConfiguredGreetings`): `ok`. Acceptance: `grep -c 'memberreq.ClearHolds(' fuku/modules/greetings.go` prints 1, `grep -q 'deps.Reset'` exits 0, `grep -c 'func memberReqJoinReset(' fuku/modules/memberreq_join.go` prints 1, `grep -c 'MembershipDeps.Reset' AGENTS.md` prints 1.
- **Family run at plan end** (the race-enabled test binary built from HEAD, `-test.run '^(TestMemberReq|TestGreetOnce|TestGreeting|TestMemberJoin|TestLeftMember|TestCaptcha|TestLockdown|TestProcessSingleJoin|TestProcessJoins|TestDecideJoin|TestLegacyGreetings|TestUnmute|TestResolveUnmutePermissions)'`, output in the scratchpad): `PASS`, `family exit=0`, no `--- FAIL`.
- **Settings family, run once** because the join and leave paths changed: `go test -tags testtools -race -count=1 -run '^(TestSettings|TestLoadSettings)' ./fuku/modules ./fuku/i18n` gave `ok fuku/modules 298.409s`, `ok fuku/i18n 5.619s`.
- **Flake loop:** the compiled race-enabled binary ran `TestMemberReqKickBan`, `TestMemberReqRejoinResetsMuteHold`, `TestMemberReqJoinAndMessageOnce` and `TestMemberReqJoinResetOrder` with `-test.count=20`: `PASS`, `flake exit=0`, no `--- FAIL`.
- `gofmt -l` on every Go file I touched: empty (`error_logging.go`, `formatting.go`, `help.go`, `overwrite.go`, `blacklists_command_test.go`, `callback_codec.go`, `deeplink_router.go` are unformatted at base and not mine). `go vet -tags testtools ./fuku/modules`: clean. `CGO_ENABLED=0 go build ./...`: ok. `make check-translations`: "All translations are present!" (no key added). `make check-docs`: "No drift detected". `git diff --exit-code 8c0c827 -- go.mod go.sum`: exit 0.
- **Lint stand-in** `python3 .planning/phases/13-member-requirements/handoff/lint-local.py 8c0c827`, exit code read directly: exit 0 after the task 1 GREEN commit (the first run failed on `mreqServiceJoinNow` unused, so I moved that helper into task 2's RED commit, where tests use it), after the task 2 GREEN commit (after the fix below) and again at the end ("no dupl, gocyclo or godox findings on lines added since 8c0c827"). The plan's path under phase 11 does not exist.
- Not run: `make lint` and gosec (cannot run here; no new code assigns a string literal to a credential-like name or uses math/rand), `make test` (only 13-14), PostgreSQL (nothing here rests on a PostgreSQL-only guarantee; the `fuku/modules` harness runs on SQLite).

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - blocking] errcheck on three touched test lines**
- **Found during:** Task 2 lint stand-in
- **Issue:** The mechanical argument edit of the `processSingleNewMember` call sites made three bare calls whose error was already ignored count as "added" lines, and errcheck flagged them (lines 510, 519, 555 of `greetings_command_test.go`).
- **Fix:** `_ = processSingleNewMember(...)` on those three lines. No expectation changed.
- **Files modified:** `fuku/modules/greetings_command_test.go`
- **Commit:** 83929a5

**2. [Rule 3 - blocking] go build cache filled the disk mid-task**
- **Found during:** Task 1 (a test binary link failed with "no space left on device", after the task 1 commit had been made)
- **Fix:** `go clean -cache` (a regenerable cache), then re-ran the task 1 command: `ok`. No repository file was affected; the committed content was verified by that run.

### Judgment calls

- **Six call sites, not seven.** `greetings_command_test.go` has six `processSingleNewMember` call sites (lines 510 to 629), not seven; each gained `time.Now()` with no change of expectation (and three gained `_ =`, above).
- **`mreqServiceJoinNow` and `mreqJoinNow` are thin wrappers** of `mreqServiceJoinAt` and `mreqJoinAt`, which the "hold created after the join date" and "join with no date" tests call with an explicit date.
- **The aged hold.** `memberreq.ClaimHold` stamps `created_at` itself, and the join date has one second of resolution, so a test that wants the reset to find a hold ages it with `mreqAgeHold` (an UPDATE of `created_at`). In production a leave and a rejoin are seconds apart; a hold created within the same second as a join dated the same second is treated as belonging to the new membership (accepted).
- **Concurrent subtest uses a Mute rule.** As 13-09 recorded, a concurrent warn on SQLite can fail the warn module's read-then-write transaction with "database is locked", so the join-racing-a-message case is a mute (fully concurrent), and the warn cases run join-then-message and message-then-join in sequence.
- **Lockdown case is a direct call.** During an active lockdown the group `-7` join guard ends a rejoin's service message before greetings runs (it records the joiner for a ban), so the stored-permissions lift is proven through a direct `memberReqJoinReset` call.
- **Plan's lint path** under phase 11 does not exist; the Phase 13 stand-in was used.

No Rule 2 or Rule 4 changes were needed.

## Requirement coverage

`requirements-completed: [MREQ-04, MREQ-08]`. MREQ-04 (a violation is punished once, until the member fixes it or rejoins, across replicas) is complete in code: 13-07's latch and 13-09's clean path plus this plan's leave hook and rejoin reset, with the join-and-message test. MREQ-08 (a kicked member rejoins and is checked again, a banned member stays banned) is complete: `TestMemberReqKickBan`. Live behaviour waits for the switch 13-13 turns on. I ticked the checkbox and traceability row of each by hand in `.planning/REQUIREMENTS.md` (`git diff` shows exactly those four lines); `requirements.mark-complete` was not run. MREQ-06 and MREQ-07 stay Pending for 13-13.

## Known Stubs

None.

## Threat Flags

None beyond the plan's register. T-13-43 by the marker check, the lockdown-permission test and the admin-mute test; T-13-44 by `TestMemberReqRejoinResetsMuteHold` (lift and restore); T-13-45 by the hold-after-join-date test and `TestMemberReqJoinAndMessageOnce`; T-13-SC by the unchanged `go.mod` and `go.sum`. Live checks A3 and A4 (Telegram keeping a restriction across a leave, and the rejoin producing a service message) remain the owner's, per the plan: if a restriction is dropped at the leave, the lift is a harmless no-op because the marker is then absent.

## Notes for later plans

- 13-13 sends the held welcome (the hold's `welcome_held` is recorded by the entry and the claim) and is the only plan that sets `memberReqEnforcementLive`. A hold restored by a failed rejoin lift is a punished mute hold again, which the join entry reads as "welcome held".
- STATE.md "Plan:" reads `13 of 14`; ROADMAP.md was not marked complete.

## Self-Check: PASSED

- Files exist: `fuku/modules/memberreq_rejoin_test.go`, `13-12-SUMMARY.md`.
- Commits `5e5c2cf`, `7139e88`, `e4d13b7`, `83929a5` are ancestors of HEAD.
