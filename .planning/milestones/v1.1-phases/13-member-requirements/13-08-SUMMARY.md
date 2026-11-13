---
phase: 13-member-requirements
plan: 08
subsystem: modules
tags: [member-requirements, executor, failure-handling, notice-retry, unmute, lockdown]
requires:
  - phase: 13-member-requirements
    provides: memberReqEnforce, memberReqPaced, memberReqPostNotice, memberReqAnnounce, isMemberReqMute, memberReqEnv (13-07); BackoffHold, RevertHoldToPending, StaleClaim (13-01)
provides:
  - memberReqRefused, memberReqBackoff, memberReqSendFailed and memberReqLiveMarker (a refused punishment backs off, an unknown mute is checked by its marker, a reclaim adopts a live marker)
  - memberReqPostNotice that never drops a mute notice to pacing and retries any other failure once
  - memberReqUndeliveredMute (lift a mute whose notice could not be delivered, revert the hold to a pending backoff row)
  - memberReqUnmutePermissions and memberReqLiftMute, the one unmute code of the requirements
  - resolveUnmutePermissionsWithLockdown, with resolveUnmutePermissions as its wrapper
affects: [13-09, 13-12, 13-13, 13-14, member-requirements]

actuals:
  tokens: 14000
  tasks: 2
  commits: 7

commits: 7
plan_head_before: cdf258c0febd0c15e4678d8eec3317456c096872
plan_head_after: 43d6410d0a197914b5b56e1f60fd5c3a17ec1f5c

tech-stack:
  added: []
  patterns:
    - "A definite refusal (4xx other than 429 by code, or ratelimit.ErrRateLimited) backs the pending hold off for StaleClaim; an unknown mute answer is decided by one live getChatMember against the marker"
    - "A mute notice loops on ErrRateLimited with a short wait until its context or a 5 minute budget ends; no member is left muted without the button"
    - "Every requirement unmute takes its permissions only from resolveUnmutePermissionsWithLockdown"

key-files:
  created:
    - fuku/modules/memberreq_unmute.go
  modified:
    - fuku/modules/memberreq_enforce.go
    - fuku/modules/memberreq_notice.go
    - fuku/modules/chat_permissions.go
    - fuku/modules/memberreq_failure_test.go
    - AGENTS.md

key-decisions:
  - "A reclaimed pending mute hold whose old marker is still live on the member adopts it (the hold keeps that marker, the mute is not sent again) instead of muting twice"
  - "memberReqUndeliveredMute runs on context.WithoutCancel with a 30 s bound, and the revert uses the 5 s record context, so a shutdown never strands a mute without its button"
  - "memberReqPostNotice decides the pacer wait by the keyboard (a button means a mute notice); memberReqAnnounce decides the lift by hold.Action"

requirements-completed: []

duration: 35min (task 2 and wrap-up; task 1 ran in the earlier session)
completed: 2026-11-07
status: complete
---

# Phase 13 Plan 08: Failure handling, notice retry and the undeliverable-mute rule Summary

**A punishment Telegram refuses now backs off for two minutes instead of looping, an uncertain mute is decided by its marker, a mute notice is never dropped by pacing, and a mute whose notice still cannot be delivered is lifted through the lockdown-aware unmute so no member is left muted without the "I fixed it" button. Nothing here is reachable from a real group: `memberReqEnforce` still has no production caller and the kill switch arrives with 13-09 and 13-13.**

## Performance

- **Tasks:** 2 of 2 (task 1 tracer, task 2 TDD)
- **Commits:** 7 as measured from `plan_head_before`: five plan commits (below) plus the pause and resume handoff docs commits `8da2490` and `e1a5c61`, which are not plan work. The SUMMARY commit follows.
- No handler, no kill switch and no production caller was added; `grep` finds `memberReqEnforce(` only at its definition outside tests. No requirement is ticked.

## Accomplishments

- **Refusals (task 1, earlier session).** `memberReqRefused` (by code: `isTelegramClientError` or `ratelimit.ErrRateLimited`) makes `memberReqSendFailed` call `memberreq.BackoffHold` (refresh `created_at`) with no notice, so the member's next messages inside `memberreq.StaleClaim` are the held path with no Telegram call and one winner reclaims and retries after it. A failed warn is the same backoff. An unknown answer (5xx, timeout, dropped connection) of a mute is verified with one live `getChatMember` against the marker: present counts as applied, absent backs off, unreadable leaves the hold pending; a kick or ban with an unknown answer stays pending with no notice. A reclaim of a pending mute hold whose marker is live adopts it (`memberReqLiveMarker`).
- **Lockdown-aware unmute choke point (`chat_permissions.go`).** `resolveUnmutePermissionsWithLockdown(chatInfo) (permissions, lockdownActive, error)` carries the old body; `resolveUnmutePermissions` is its wrapper returning exactly what it returned before, so `/unmute`, the unrestrict button, the captcha pass, the orphan recovery and staff `/unmute` are unchanged.
- **One unmute code (`memberreq_unmute.go`).** `memberReqUnmutePermissions` (one paced `getChat`, then the resolver) and `memberReqLiftMute` (one paced `restrictChatMember`, no options, so no end date). First production caller is the undeliverable-notice rule; 13-12 (rejoin reset) and 13-13 ("I fixed it") reuse them.
- **Notice retry (`memberreq_notice.go`).** A notice with a keyboard (a mute) that the pacer gives up on (`ErrRateLimited`) waits `memberReqNoticeRateLimitWait` (2 s, a variable tests shorten) and is sent again until its context or `memberReqMuteNoticeBudget` (5 min) ends. Any other failure is retried once for every notice; a notice without a button is not retried after `ErrRateLimited`.
- **Undeliverable mute notice.** `memberReqAnnounce` calls `memberReqUndeliveredMute` for a mute whose notice still failed (and logs no ordinary not-delivered Warn for it): on `context.WithoutCancel` with a 30 s bound it resolves the permissions, lifts the mute, `memberreq.RevertHoldToPending` and logs ONE Warn with chat and user IDs only; a failed revert logs at Error. When the lift (or the `getChat` before it) is refused the mute stays, the hold stays punished and one Warn says the mute could not be lifted (a Telegram error is named by its code only).
- **AGENTS.md** (same commit): the 13-07 executor bullet now covers the refusal backoff, the marker check, marker adoption on reclaim, the never-dropped mute notice, the undeliverable-notice lift with its accepted limit and `RevertHoldToPending`, and names `memberreq_unmute.go`; the `resolveUnmutePermissions` bullet names `resolveUnmutePermissionsWithLockdown` and the requirements' lift.

## Task Commits

1. Handoff-time fix: `87f302c` fix(13-07) rename the notice name placeholder so gosec does not read it as a credential
2. Task 1 (tracer): `0ee9a87` test(13-08) RED (refused punishment, unknown mute answer), `5d8a485` feat(13-08) GREEN (back off a refused punishment and verify an unknown mute)
3. Task 2: `0ea4e39` test(13-08) RED (notice retry, undeliverable notice, resolver sibling), `43d6410` feat(13-08) GREEN (never leave a requirement mute without its button, also AGENTS.md)

## TDD Gate Compliance

| Gate | Commit | Evidence |
|------|--------|----------|
| RED task 1 | 0ee9a87 | Tests only, no compile-only stand-in: the tests built against the existing 13-07 code (every new name was a test helper) and failed on assertions |
| GREEN task 1 | 5d8a485 | `TestMemberReqPunishRefused`, `TestMemberReqPunishUnknown` pass |
| RED task 2 | 0ea4e39 | Compile-only stand-ins: `resolveUnmutePermissionsWithLockdown` returning `(resolveUnmutePermissions(...), false)` and the variable `memberReqNoticeRateLimitWait`. Failing on assertions: the active-lockdown resolver case (flag false), four retry subtests (no retry, no wait), five undeliverable subtests (no lift). Two subtests passed at once as guards of behaviour that already existed: "the member's next message inside the backoff adds nothing" (a punished hold is already Held) and "a notice without a button is not retried when the pacer gives up" |
| GREEN task 2 | 43d6410 | all three new tests pass, plus `TestResolveUnmutePermissionsLockdown`, `TestUnmuteDuringLockdownUsesSnapshot`, `TestUnmuteLockdownLookupFails` |

`gsd_run check tdd-red-evidence` was not run (the plan is `type: execute`); the evidence is the real test output.

## Verification (real results)

- Task 2 tests: `go test -tags testtools -race -count=1 -run '^(TestResolveUnmutePermissionsWithLockdown|TestMemberReqMuteNoticeRetry|TestMemberReqMuteNoticeUndeliverable|TestResolveUnmutePermissionsLockdown|TestUnmuteDuringLockdownUsesSnapshot|TestUnmuteLockdownLookupFails)$' -v ./fuku/modules` gave `ok`, every subtest PASS.
- **Family run at plan end** (one background run, output in the scratchpad): `go test -tags testtools -race -count=1 -run '^(TestMemberReq|TestUnmute|TestResolveUnmutePermissions)' ./fuku/modules` gave `ok fuku/modules 30.159s`, `done exit=0`.
- **Existing tests that reach `resolveUnmutePermissions`**, run explicitly, all PASS (`ok fuku/modules 4.518s`): `TestCaptchaVerifyCallbackCorrectAnswerUnmutesAndCleansAttempt`, `TestCaptchaVerifyCallbackUnmuteFailureSchedulesRetryUnmute`, `TestCaptchaWebUnmuteFailureSchedulesRetry`, `TestCaptchaWebTracerJoinVerifyUnmute`, `TestUnrestrictCallbacksApplyUnmuteAndInvalidUser`, `TestStaffActionUnmuteNonMember`, `TestRunOrphanedCaptchaRecoveryPersistsFailedUnmuteWithoutFailingStartup`, `TestResolveUnmutePermissionsLockdown`, `TestUnmuteDuringLockdownUsesSnapshot`. No existing test changed outcome and none was edited.
- `gofmt -l` on every Go file I touched: empty (the unformatted files `blacklists_command_test.go`, `callback_codec.go`, `deeplink_router.go` and a few others in `fuku/modules` are unformatted at base and not mine). `go vet -tags testtools ./fuku/modules`: clean. `CGO_ENABLED=0 go build ./...`: ok. `make check-translations`: "All translations are present!" (no locale key added; only existing `mreq_*` keys are used). `git diff --exit-code 8c0c827 -- go.mod go.sum`: exit 0.
- **Lint stand-in** `python3 .planning/phases/13-member-requirements/handoff/lint-local.py 8c0c827`, run directly after the GREEN commit: exit 0, "no dupl, gocyclo or godox findings on lines added since 8c0c827". `make lint` and gosec cannot run here; the new code assigns no string literal to a credential-like name.
- The settings family was NOT run: no shared test helper those tests use was touched (the new helpers `mreqShortenNoticeWait`, `mreqRefusals`, `mreqWarnLines`, `mreqRestrictPermissions`, `mreqStartLockdown`, `mreqUndeliverable` live in `memberreq_failure_test.go`, used only by this plan's tests; `chat_permissions.go` keeps `resolveUnmutePermissions` byte-compatible and the unmute tests above passed).
- Not run: `make lint`, `make test` (only 13-14), PostgreSQL (nothing here rests on it; the `fuku/modules` harness runs on SQLite).

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - blocking, CI gosec of the 13-07 push] G101 on the notice name placeholder**
- **Found during:** the pause handoff: CI's gosec job flagged `const memberReqNameToken = "<<mreq-name>>"` (G101, a false positive on the identifier name) on the pushed head `cdf258c`.
- **Fix:** renamed to `memberReqNamePlaceholder` (two files, no behaviour change), in its own commit.
- **Files modified:** `fuku/modules/memberreq_notice.go`, `fuku/modules/memberreq_screens_test.go`
- **Commit:** `87f302c`

### Judgment calls

- **Task 1's RED commit had no compile-only stand-in.** The 13-07 code already contained every production name the tests used, so the commit holds the test file alone and fails on assertions.
- **Marker adoption (task 1 design choice).** The plan said a reclaim "checks the live marker before muting again". Implemented as adoption: a reclaimed pending mute hold whose stored marker is still live on the member (`memberReqLiveMarker`) keeps that marker (`memberReqTakeHold`'s `keepUntil`), is marked punished and announces, instead of sending the mute a second time, so the marker on the member and on the hold stay equal.
- **Seams.** Task 1 added no production seam (its helpers `mreqShortenStale`, `mreqLostAnswerClient`/`loseAnswers`, `mreqRateLimited` and `mreqPunishmentCases` are test code; `memberreq.StaleClaim` was already a variable). Task 2 added one production variable, `memberReqNoticeRateLimitWait`, so a test can shorten the wait between sends of a rate-limited mute notice.
- **Outcome of an undeliverable mute.** `memberReqEnforce` still returns `memberReqMuted` for the episode whose mute was applied and then lifted: the outcome says what the executor did first, and the lift is visible in the hold (pending again) and the Warn line. 13-09 reads the outcome for counters only.
- **Plan's lint path.** `.planning/phases/11-message-editor/handoff/lint-local.py` does not exist; the Phase 13 stand-in path was used.

No Rule 2 or Rule 4 changes were needed.

## Accepted limit

When Telegram refuses the lift of an undeliverable mute as well (two refusals in a row), the member stays muted with no button and the hold stays punished; one Warn says the mute could not be lifted. The rejoin reset (13-12) or an admin's `/unmute` followed by a clean message (13-09) still ends it. Recorded in AGENTS.md.

## Requirement coverage

Contributes to MREQ-04 (refused punishment backs off), MREQ-05 (notice retried, never dropped by pacing), MREQ-06 (the unmute code "I fixed it" reuses; a muted member always has a button) and MREQ-07 (the unknown-answer check by the marker). **Completes none**: `requirements-completed: []`; MREQ-04, MREQ-05, MREQ-06 and MREQ-07 stay Pending in REQUIREMENTS.md and `requirements.mark-complete` was not run.

## First callers (what a later plan wires)

- `memberReqUnmutePermissions`, `memberReqLiftMute`: first production caller is `memberReqUndeliveredMute` (here); the rejoin reset of **13-12** and the "I fixed it" handler of **13-13** reuse them.
- `memberReqEnforce` and everything it reaches: first production caller is the message watcher of **13-09** behind the kill switch `memberReqEnforcementLive` (false until **13-13**).

## Known Stubs

None.

## Self-Check: PASSED

- Files: `fuku/modules/memberreq_unmute.go` exists; `13-08-SUMMARY.md` written.
- Commits `87f302c`, `0ee9a87`, `5d8a485`, `0ea4e39`, `43d6410` are ancestors of HEAD.
