---
phase: 13-member-requirements
plan: 10
subsystem: modules
tags: [member-requirements, delete-messages, per-chat-cap, pause-latch, admin-safety]
requires:
  - phase: 13-member-requirements
    provides: memberReqEnforce and its step helpers, memberReqPaced, memberReqLiveMember, memberReqPostNotice (13-07, 13-08); the watcher, runner and kill switch (13-09); the Delete messages switch (13-05); mreq_notice_paused (13-04)
provides:
  - Delete messages in the executor (memberReqDeleteMessage, memberReqDeletes, memberReqSenderNonAdmin, memberReqDeleteHeld)
  - the per-chat punishment cap and pause latch (memberreq_limits.go: memberReqCapPerMinute, memberReqPause, memberReqCapKey, memberReqPausedKey, memberReqCapAllow, memberReqPauseChat, memberReqPaused)
  - test helpers mreqDeleteMessages, mreqDeleted, mreqAdminList, mreqNoAdminList, mreqPromote, mreqMemberLookups, mreqHoldMember, mreqPause, mreqResume, mreqCapKeys, mreqFixedClock
affects: [13-11, 13-12, 13-13, 13-14, member-requirements]

actuals:
  tokens: 11000
  tasks: 2
  commits: 4

commits: 4
plan_head_before: f3a4c55395bd11fb75fae94254009a1013bbff79
plan_head_after: 9413d6fcdb08f971115fca0331976643e76b3660

tech-stack:
  added: []
  patterns:
    - "A delete decision reads the cached admin list at the moment of the decision (cachedAdminStatus), never a value carried from the watcher"
    - "A brake that counts before the claim and never after the punishment, so it cannot drop a notice already due"
    - "The SET NX winner of a latch is the only poster of its notice, as the raid cannot-lock warning gate is"

key-files:
  created:
    - fuku/modules/memberreq_limits.go
    - fuku/modules/memberreq_delete_test.go
    - fuku/modules/memberreq_limits_test.go
  modified:
    - fuku/modules/memberreq_enforce.go
    - AGENTS.md
    - .planning/REQUIREMENTS.md

key-decisions:
  - "A message is deleted only when its sender is established as a non-admin: the job's live getChatMember on the punishing path, a known cached admin list that does not name them (or one live lookup when the list cannot say, outside a pause) on the held path, and the cached list alone during a pause"
  - "The delete runs after the punishment and the notice, whatever became of the punishment (took effect, refused, unknown, or a claim another replica won): the live lookup has already shown a non-admin who breaks a rule"
  - "The cap window uses raidNow().Unix()/60, the raid counters' clock seam, so a test can stop the minute and never straddles a window"
  - "A cancelled run punishes nobody: a Redis error on the cap with a cancelled context returns false instead of allowing"

requirements-completed: [MREQ-03]

duration: 40 min
completed: 2026-11-07
status: complete
---

# Phase 13 Plan 10: Delete messages and the per-chat punishment cap Summary

**With Delete messages on, a violator's message and every later message of a held member are deleted, only ever for a sender established as a non-admin, also while punishments are paused; and a mis-set rule in a busy group stops punishing after ten a minute, with one `mreq_notice_paused` and a 10-minute pause that Redis alone holds.**

## Performance

- **Tasks:** 2 of 2 (task 1 tracer, task 2 TDD)
- **Commits:** 4 (RED and GREEN for each task), measured from `plan_head_before` with `git rev-list --count`.

## Accomplishments

- **Delete step (`memberreq_enforce.go`).** `memberReqDeleteMessage` is one `DeleteMessageWithContext` through `memberReqPaced`, only for a message case (`Source == memberReqFromMessage`, `MessageID > 0`) with Delete messages on (`memberReqDeletes`); a failure logs the message, user and chat IDs and the error code, never the content, and changes nothing. Three call sites: after the punishment on the punishing path (also after a refused or unknown punishment and after a claim another replica won), the held path, and the paused path.
- **Admin safety (`memberReqSenderNonAdmin`).** A known `cachedAdminStatus` answers true exactly when the sender is not in the list; unknown with `mayAsk` makes one paced live `getChatMember` and is true only for a member or a restricted member still in the group (`memberReqLiveEligible`; a creator, administrator, someone gone, a bot and a failed lookup are false); unknown without `mayAsk` is false with no call. The held path passes `mayAsk` as "the list cannot say and the chat is not paused" and reads `memberReqPaused` only then, so a known list costs no Redis read.
- **Cap and pause (`memberreq_limits.go`).** `memberReqCapAllow` counts the attempt in `fuku:memberreq:{<chat>}:cap:<unix minute>` (`INCR` plus `EXPIRE 120` in one pipeline under 250 ms) after the live lookup and rule re-check and before the claim. Over `memberReqCapPerMinute` (10) it refuses the punishment and sets `fuku:memberreq:{<chat>}:paused` with `SET NX EX` for `memberReqPause` (10 minutes); only the winner of the `SET NX` posts one `mreq_notice_paused` (`{n}` 10, `{minutes}` 10) in the group's language through `memberReqPostNotice`. A paused chat (`memberReqPaused`, read after the held path and before the live lookup) starts no punishment, hold or notice and makes no live lookup; it deletes only for a sender the cached list knows as a non-admin. No Redis and a Redis error allow the punishment.
- **Tests.** `TestMemberReqDeleteMessages` (warn plus three held messages, Delete messages off, a refused delete logged by IDs, a refused mute and an unknown-answer ban still deleting, clean member and join case never deleted, six held-path admin subtests: known list, invalidated list, promoted, promoted with the list reloaded, creator, failed lookup). `TestMemberReqDeleteDuringPause` (known list deletes with no `getChatMember`; unknown list deletes nothing and makes no call, for a new violator, a held member and a promoted one, who after the pause is checked by one lookup and still not deleted; Delete messages off; an approved member). `TestMemberReqCapPausesPunishments` (two punished, third pauses with the latch TTL `memberReqPause` and one notice with minutes 10 and n 2, fourth silent, both mute notices delivered; another chat unaffected; a new minute counts afresh; two replicas tipping the cap at once post one notice in all; the member over the cap still has the message deleted). `TestMemberReqCapWithoutRedis` (five violators each punished once and deleted, no counter, latch or notice).
- **AGENTS.md.** A Data bullet for the cap and pause keys, the executor bullet's Delete messages, pause and cap sentences with the accepted limit ("an unknown-status violator's message is not deleted during a pause"), the `StopMemberReqJobs` bullet's pause-latch sentence, the pacer bullet's mention of the delete, and the Testing families.

## Task Commits

1. Task 1 (tracer): `481793f` test RED, `635c5f2` feat GREEN
2. Task 2: `23386e8` test RED, `9413d6f` feat GREEN

## TDD Gate Compliance

| Gate | Commit | Evidence |
|------|--------|----------|
| RED task 1 | 481793f | `TestMemberReqDeleteMessages` failed on assertions (`deleted after the first message = [], want only 101` and the held-path subtests); no stand-in needed (no new symbol) |
| GREEN task 1 | 635c5f2 | `TestMemberReqDeleteMessages`, `TestMemberReqWatcherPunishesOnce`, `TestMemberReqPunishRefused` pass |
| RED task 2 | 23386e8 | Compile-only stand-in `memberreq_limits.go` (the two variables, the two key builders, a `memberReqCapAllow` that always allows and a `memberReqPaused` that is never paused). `TestMemberReqCapPausesPunishments` (all five subtests) and `TestMemberReqDeleteDuringPause` (the new-violator and unknown-list subtests) failed on assertions. `TestMemberReqCapWithoutRedis`, the known-list, Delete-messages-off and approved pause subtests already held at the stand-in: guards |
| GREEN task 2 | 9413d6f | all four tests pass |

`gsd_run check tdd-red-evidence` was not run (the plan is `type: execute`); the evidence is the real test output.

## Verification (real results)

- **Task 1 command** (`TestMemberReqDeleteMessages|TestMemberReqWatcherPunishesOnce|TestMemberReqPunishRefused`): `ok`. **Task 2 command** (the four new tests): `ok`. AGENTS.md greps: `fuku:memberreq:{<chat>}:paused` prints 2, `established as a non-admin` 1, `not deleted during a pause` 1, and the `StopMemberReqJobs` bullet awk-grep for `pause latch` exits 0. Code greps: one `func memberReqDeleteMessage(`, five mentions of `memberReqDeleteMessage(` in the executor (the definition and four call sites), one `func memberReqSenderNonAdmin(` whose body calls `cachedAdminStatus(`, `memberReqCapAllow(` present.
- **Family run at plan end** (background, output `scratchpad/family.out`): `go test -tags testtools -race -count=1 -run '^TestMemberReq' ./fuku/modules` gave `ok fuku/modules 36.784s`, `family exit=0`.
- **Flake loop:** the compiled race-enabled test binary ran `TestMemberReqCapPausesPunishments`, `TestMemberReqDeleteDuringPause`, `TestMemberReqCapWithoutRedis`, `TestMemberReqDeleteMessages` and `TestMemberReqOnePunishmentAcrossReplicas` with `-test.count=20`: exit 0, no `--- FAIL` line.
- `gofmt -l` on every file I touched: empty (the seven unformatted files in `fuku/modules` are unformatted at base). `go vet -tags testtools ./fuku/modules`: clean. `CGO_ENABLED=0 go build ./...`: ok. `make check-translations`: "All translations are present!" (no key added; `mreq_notice_paused` exists since 13-04). `git diff --exit-code 8c0c827 -- go.mod go.sum`: exit 0.
- **Lint stand-in:** `python3 .planning/phases/13-member-requirements/handoff/lint-local.py 8c0c827` read directly (`; echo exit=$?`): exit 1 once, with two staticcheck SA4000 findings in the test file the task 2 RED commit added (`!allow() || !allow() || allow()`, identical operands); I rewrote it to named results inside the task 2 GREEN commit, and it exited 0 after that and at the end of the plan ("no dupl, gocyclo or godox findings on lines added since 8c0c827"). It also exited 0 after the task 1 GREEN commit.
- **Settings family:** not run. I touched no shared test helper (`memberreq_env_test.go`, `settings_env_test.go` are unchanged; the new helpers live in my own test files).
- Not run: `make lint`, gosec (cannot run here; no new code assigns a string literal to a credential-like name or uses math/rand), `make test` (only 13-14), PostgreSQL (nothing here rests on a PostgreSQL-only guarantee; the `fuku/modules` harness runs on SQLite).

## How the kill switch was checked (T-13-29, T-13-56)

`grep -rn memberReqEnforcementLive --include=*.go .` finds the declaration and `Load()` in `fuku/modules/memberreq.go` and, outside it, only `withMemberReqLive` in `memberreq_env_test.go`. This plan sets it nowhere: every new behaviour (delete, cap, pause) lives inside `memberReqEnforce` or the helpers it calls, and the only caller of `memberReqEnforce` is the watcher's job, started only behind `memberReqLive()`. `main.go` is not touched by this plan. After this plan a real group still cannot be punished or have a message deleted by any production path.

## Deviations from Plan

### Auto-fixed Issues

None needed for rules 1 to 3.

### Judgment calls

- **Delete after a lost claim.** The plan lists three delete points (held, paused, after a punishment that took effect or was refused or unknown). A claim another replica won (and a claim that failed on a database error) is a fourth outcome of the same punishing path: the job's live lookup has already shown a non-admin who breaks a rule, so the message is deleted there too, consistently with "every message of a violator" (D-14). The delete follows the punishment and the notice.
- **The cap's clock is `raidNow`.** The plan names "unix minute"; I used the existing raid clock seam (`raidNow().Unix()/60`) so a test can stop the minute (`mreqFixedClock`) and the two-replica and window tests never straddle a minute boundary. Production behaviour is `time.Now`.
- **A cancelled run is not "a Redis error".** When the cap's Redis call fails and the run's context is already cancelled, `memberReqCapAllow` returns false (the run punishes nobody) instead of allowing, because the claim that would follow cannot succeed on a cancelled context. A real Redis error with a live context still allows, as the plan says.
- **Doc comment of `memberReqEnforce`** renumbered 1 to 9 (gofmt rewrites an indented "3b." item into an awkward continuation).
- **Plan's lint path** under phase 11 does not exist; the Phase 13 stand-in was used.

No Rule 4 changes were needed.

## Requirement coverage

`requirements-completed: [MREQ-03]`. MREQ-03 ("With Delete messages on, a message from a member who breaks a rule is deleted") is complete: the checkbox and the traceability row `| MREQ-03 | Phase 13 | Complete |` in `.planning/REQUIREMENTS.md` are ticked by hand (`git diff` shows only those two lines). It is implemented and proven behind the kill switch, which 13-13 turns on. MREQ-04 stays Pending (held members' further messages now add nothing but their deletion; 13-12 completes it). `requirements.mark-complete` was not run.

## Known Stubs

None.

## Threat Flags

None beyond the plan's register. T-13-35 by `TestMemberReqCapPausesPunishments`; T-13-36 by `TestMemberReqDeleteDuringPause` (and the accepted unknown-status limit, documented in AGENTS.md); T-13-59 by `TestMemberReqDeleteMessages` (the six held-path subtests) and `TestMemberReqDeleteDuringPause` (unknown list, promoted member); T-13-37 by the two-replica subtest; T-13-38 by the refused-delete log assertion (IDs and code only); T-13-SC by the unchanged `go.mod` and `go.sum`.

## Notes for later plans

- STATE.md "Plan:" reads `11 of 14`; ROADMAP.md was not marked complete.
- A test that makes a held-path delete decision must set the admin cache it depends on (`mreqAdminList`, `mreqNoAdminList` or `mreqPromote`): the process-wide admin cache otherwise carries whatever an earlier handler loaded for the chat.
- The pause latch stands for `memberReqPause` (10 minutes) and is held in Redis only; 13-13's switch-on and 13-14's gate need no extra work for it.

## Self-Check: PASSED

- Files exist: `fuku/modules/memberreq_limits.go`, `fuku/modules/memberreq_delete_test.go`, `fuku/modules/memberreq_limits_test.go`.
- Commits `481793f`, `635c5f2`, `23386e8`, `9413d6f` are ancestors of HEAD.
