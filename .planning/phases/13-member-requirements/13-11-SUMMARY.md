---
phase: 13-member-requirements
plan: 11
subsystem: modules
tags: [member-requirements, join, captcha-guard, kill-switch, welcome-held, fail-closed]
requires:
  - phase: 13-member-requirements
    provides: memberReqEnforce, memberReqResult, memberReqNeverChecked, memberReqRecordCtx (13-07, 13-08); the kill switch memberReqLive() and withMemberReqLive (13-09); memberreq.SetHoldWelcomeHeld, GetHoldFresh (13-01); the memberReqEnv test environment (13-07)
provides:
  - memberReqJoinThenWelcome, the one join entry of the chat_member join, the join service message and both captcha pass paths
  - memberReqCaptchaGuard, the first statement of unmuteCaptchaUser, behind the same switch and failing closed
  - memberReqHoldRead, the test seam of the one hold read both make; errMemberReqHoldUnreadable
  - test helpers mreqJoinEnv, mreqJoin, mreqServiceJoin, mreqSentTexts, mreqPressCaptcha, mreqCaptchaOn, mreqRestricts, mreqHoldReadCounter, mreqDueMutedRow
affects: [13-12, 13-13, 13-14, member-requirements]

actuals:
  tokens: 10300
  tasks: 2
  commits: 4

commits: 4
plan_head_before: a70dbd9a3be1aa5525ce6737cb7f263adbdac624
plan_head_after: 1f4b25490268866c7230947ff4e68db38076e771

tech-stack:
  added: []
  patterns:
    - "A guard behind the kill switch returns before any read, call or error, so a commit before the switch is on changes no real path"
    - "A fail-closed error built from IDs only (a sentinel plus the IDs), so no database text can match a permanent-failure substring"

key-files:
  created:
    - fuku/modules/memberreq_join.go
    - fuku/modules/memberreq_join_test.go
  modified:
    - fuku/modules/greetings.go
    - fuku/modules/captcha.go
    - fuku/modules/memberreq_env_test.go
    - AGENTS.md
    - .planning/REQUIREMENTS.md

key-decisions:
  - "A hold-read error in the join entry sends the plain welcome (the executor then also could not read the hold and would punish nobody); a welcome frees nobody, unlike an unmute, so the entry does not fail closed"
  - "The guard returns handled false with the error on a read failure, and the caller returns the error first; a punished Mute hold re-applies MutedPermissions with until_date the hold's marker and returns handled true with the re-mute's own error"
  - "isPermanentUnmuteError got an explicit errors.Is early return for errMemberReqHoldUnreadable, beside the guard's IDs-only text, so a later change of wording can never make the retry worker delete its row"

requirements-completed: [MREQ-02, MREQ-05, MREQ-09]

duration: 2h (including the family run, the 5-minute settings family and the flake loop)
completed: 2026-11-07
status: complete
---

# Phase 13 Plan 11: The join entry and the captcha guard Summary

**One entry, `memberReqJoinThenWelcome`, now stands where `SendWelcomeMessage` was on both welcome paths (the join after the dedupe claim, and the end of a captcha pass), and `unmuteCaptchaUser` starts with `memberReqCaptchaGuard`; both read `memberReqLive()` first, so in production, where the switch stays false until 13-13, a join gets exactly the plain welcome and every captcha unmute is exactly what it was.**

## Performance

- **Tasks:** 2 of 2 (task 1 tracer, task 2 TDD)
- **Commits:** 4 (RED and GREEN for each task), measured from `plan_head_before` with `git rev-list --count`.

## Accomplishments

- **The join entry (`memberreq_join.go`).** `memberReqLive()` false, a chat that is not a supergroup, a bot, the bot itself and a Telegram service account (`memberReqNeverChecked`), an unreadable setting and a group with every rule off all send the plain welcome. With a rule on the hold is read fresh through `memberReqHoldRead`: a punished requirement mute (the D-08 race) records `welcome_held` and sends no welcome whatever Check on join says. With Check on join off the member is welcomed with no further read. Otherwise `memberReqEnforce` runs synchronously under `memberReqJoinTimeout` (15 s, `memberReqFromJoin`): Removed and Muted send nothing (the claim already stored `welcome_held` for a mute), Held with a punished Mute hold records `welcome_held` and sends nothing, and everything else (warned, none, not applied, held by another hold) is welcomed after the notice the executor posted.
- **Both welcome paths.** The Welcome closure of `membershipDepsForJoin` (greetings.go) and the last line of `finishCaptchaPass` (captcha.go, the button and the web pass) call it; `captcha.go` no longer calls `SendWelcomeMessage`. A captcha-failed joiner and a lockdown-banned joiner never reach it.
- **The captcha guard.** `memberReqCaptchaGuard` is the first statement of `unmuteCaptchaUser`. Switch off: it returns `false, nil` before any read or call. Switch on: a hold-read error returns `errMemberReqHoldUnreadable` wrapped with the chat and user IDs only (no database text), so the member stays muted and each caller's own retry applies; `isPermanentUnmuteError` answers false for it (naturally, and explicitly through `errors.Is`). A punished Mute hold makes it re-apply `MutedPermissions` with `UntilDate: hold.MuteUntil` through `memberReqPaced` and return handled, so a captcha pass never frees a held member and the marker stays valid for "I fixed it". No other mute path is edited.
- **AGENTS.md.** The "Member requirements, the join" bullet (entry, switch, Check on join, held welcome, the D-08 race read, the guard, its switch, its fail-closed read, the accepted cost of a member kept muted by a passing database fault), the Greet once bullet's callers list, and the executor bullet's callers.

## Task Commits

1. Task 1 (tracer): `6ec222c` test RED, `f79427a` feat GREEN
2. Task 2: `db0c3b6` test RED, `1f4b254` feat GREEN

## TDD Gate Compliance

| Gate | Commit | Evidence |
|------|--------|----------|
| RED task 1 | 6ec222c | No stand-in needed (the tests drive joins through the dispatcher). Four `TestMemberReqJoinNoCaptcha` subtests failed on assertions (`welcomes = 1, want none for a requirement-muted joiner`, `notice at -1, welcome at 0`, the kick and the ban); the clean, Check-on-join-off and every-rule-off subtests and `TestMemberReqJoinOffUntilLive` passed against the old code, as guards of behaviour that already held |
| GREEN task 1 | f79427a | all pass, with `TestMemberJoinAndLeaveSendConfiguredGreetings` and `TestGreetOnceFirstJoinOnly` unchanged |
| RED task 2 | db0c3b6 | No stand-in needed. Failing on assertions: both `TestMemberReqJoinAfterCaptcha` pass subtests (`mute notice at -1`, a welcome sent), `TestMemberReqCaptchaGuard` re-apply and fail-closed subtests. `TestMemberReqCaptchaGuardOffUntilLive`, the no-hold and warn-hold subtests, the captcha-failed and lockdown-banned subtests and `TestMemberReqJoinExemptions` already held: guards |
| GREEN task 2 | 1f4b254 | all pass, with `TestCaptchaVerifyCallbackCorrectAnswerUnmutesAndCleansAttempt`, `TestCaptchaVerifyCallbackFinalWrongAnswerAppliesFailureAction` and `TestGreetOnceCaptchaPathAndFailure` unchanged |

`gsd_run check tdd-red-evidence` was not run (the plan is `type: execute`); the evidence is the real test output.

## How the kill switch was checked (T-13-29, T-13-56, T-13-58)

- `grep -rn memberReqEnforcementLive --include=*.go .` finds the declaration and `Load()` in `fuku/modules/memberreq.go` and, outside it, only `withMemberReqLive` in `memberreq_env_test.go` (`Load`, then `Store` with the previous value restored in `t.Cleanup`). **This plan sets it nowhere**: no production line calls `Store`, and `main.go` is not touched.
- `memberReqLive()` is read in three production places: the watcher (13-09), the join entry and the captcha guard, each as its first statement.
- `TestMemberReqJoinOffUntilLive`: with the switch off, Check on join on and two rules on Mute, a joiner without a last name or username gets the plain welcome, no `getChatMember` about them, no hold, no notice and no restrict, ban or unban call.
- `TestMemberReqCaptchaGuardOffUntilLive`: with the switch off, a punished requirement mute hold for the member and `memberReqHoldRead` replaced by a function that counts its calls and always fails, `unmuteCaptchaUser`, `restoreCaptchaPermissions` and one run of `unmuteExpiredCaptchaUsers` over a due `captcha_muted_users` row each unmute the member as at HEAD (one `getChat` and one `restrictChatMember` with the group's permissions each, the row deleted) and the counter stays 0. `captchaApplyBrokenDB` was not used.
- After this plan a real group still cannot be punished, and no real captcha unmute changes, by any production path.

## Verification (real results)

- **Task 1 command** (`TestMemberReqJoinNoCaptcha|TestMemberReqJoinOffUntilLive|TestMemberJoinAndLeaveSendConfiguredGreetings|TestGreetOnceFirstJoinOnly`): `ok`. **Task 2 command** (the four new tests plus the three existing captcha and greet-once tests): `ok`. Acceptance greps: `greetings.go` has 1 `memberReqJoinThenWelcome(`; the entry's body reads `memberReqLive()`; `captcha.go` has 0 `SendWelcomeMessage(`, 1 `memberReqJoinThenWelcome(` and 1 `memberReqCaptchaGuard(`; inside the guard's body `memberReqLive()` comes before `memberReqHoldRead`; the AGENTS.md join bullet contains `memberReqLive()` and `fails closed`; `grep -c memberReqJoinThenWelcome AGENTS.md` prints 2.
- **Family run at plan end** (background, output in the scratchpad): `go test -tags testtools -race -count=1 -run '^(TestMemberReq|TestGreetOnce|TestGreeting|TestMemberJoin|TestLeftMember|TestCaptcha|TestLockdown|TestLegacyGreetings|TestUnmute|TestResolveUnmutePermissions)' ./fuku/modules` gave `ok fuku/modules 136.927s`, `family exit=0`.
- **Settings family, run once** because the join and captcha paths changed: `go test -tags testtools -race -count=1 -run '^(TestSettings|TestLoadSettings)' ./fuku/modules ./fuku/i18n` gave `ok fuku/modules 312.703s`, `ok fuku/i18n 5.783s`, `settings exit=0`.
- **Flake loop:** the compiled race-enabled test binary ran the six new tests (`TestMemberReqJoinNoCaptcha`, `TestMemberReqJoinOffUntilLive`, `TestMemberReqJoinAfterCaptcha`, `TestMemberReqCaptchaGuard`, `TestMemberReqCaptchaGuardOffUntilLive`, `TestMemberReqJoinExemptions`) with `-test.count=20`: `PASS`, `flake exit=0`, no `--- FAIL` line.
- `gofmt -l` on every file I touched: empty. `go vet -tags testtools ./fuku/modules`: clean. `CGO_ENABLED=0 go build ./...`: ok. `make check-translations`: "All translations are present!" (no key added). `make check-docs`: "No drift detected". `git diff --exit-code 8c0c827 -- go.mod go.sum`: exit 0.
- **Lint stand-in:** `python3 .planning/phases/13-member-requirements/handoff/lint-local.py 8c0c827`, exit code read directly (`; echo exit=$?`): exit 0 after the task 1 GREEN commit, after the task 2 GREEN commit and again at the end ("no dupl, gocyclo or godox findings on lines added since 8c0c827"). The plan's path under phase 11 does not exist.
- Not run: `make lint` and gosec (cannot run here; no new code assigns a string literal to a credential-like name or uses math/rand), `make test` (only 13-14), PostgreSQL (nothing here rests on a PostgreSQL-only guarantee; the `fuku/modules` harness runs on SQLite).

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Missing critical functionality] `isPermanentUnmuteError` answers false for the unreadable-hold error explicitly**
- **Found during:** Task 2
- **Issue:** The plan only requires that the guard's error is not read as permanent. The guard's text carries IDs only, so no substring can match today, but the classifier is substring-based and a later wording change could silently make the retry worker delete its row and leave a member muted with no retry.
- **Fix:** `errors.Is(err, errMemberReqHoldUnreadable)` returns false at the top of `isPermanentUnmuteError` (captcha.go). `TestMemberReqCaptchaGuard` makes the underlying database error say "user not found" and asserts the returned error neither contains that text nor reads as permanent.
- **Commit:** 1f4b254

### Judgment calls

- **A hold-read error in the join entry welcomes.** The plan lists "everything else sends the welcome". When the hold cannot be read, `memberReqEnforce` would not read it either and would punish nobody; a welcome frees nobody (unlike an unmute), so the entry logs the error by IDs and welcomes, and only the captcha guard fails closed.
- **A punished Mute hold, not a pending one, holds the welcome.** `memberReqPunishedMute` requires `PunishedAt != nil`. A pending mute whose punishment Telegram refused left the member unmuted, so they are welcomed (outcome NotApplied).
- **`memberReqCleanup` was edited** (`memberreq_env_test.go`, a shared file) as the lead's brief asks: it now also deletes the chats' `captcha_muted_users`, `greetings` and `greeted_members` rows, the `greetings` and `captcha_settings` cache keys and the `fuku:recentJoinProcessing:<chat>:<user>` claims (Redis and the in-process map, `memberReqDeleteJoinClaims`). Every new test mints members with `uniqueLinkOwnerID()` and chats with `uniqueModuleChatID()`; the web subtest builds its own `newCaptchaWebEnv` with a fresh member ID, not the fixed 42.
- **Worker test scope.** `unmuteExpiredCaptchaUsers` processes every due row in the database, so a leftover row of another test would make a harmless extra call to the fake; the assertions count calls about this test's member and chat only.
- **Plan's lint path** under phase 11 does not exist; the Phase 13 stand-in was used.

No Rule 4 changes were needed.

## Requirement coverage

`requirements-completed: [MREQ-02, MREQ-05, MREQ-09]`. All three are complete in code: MREQ-02 (a new member is checked after the captcha pass on both paths, or on joining with no captcha, and at every message by 13-09), MREQ-05 (every violation path, message and join, posts its one notice, and a warn counts toward the warn limit), MREQ-09 (admins, approved users, bots, service accounts, channel and anonymous-admin senders are never checked, at a message and at a join). Live behaviour waits for the switch that 13-13 turns on. The checkbox and traceability row of each are ticked by hand in `.planning/REQUIREMENTS.md` (`git diff` shows exactly those six lines). MREQ-06 and MREQ-07 are contributions only (the held welcome is recorded, the captcha guard keeps the marker): they stay Pending for 13-13; MREQ-04 stays Pending for 13-12. `requirements.mark-complete` was not run.

## Known Stubs

None.

## Threat Flags

None beyond the plan's register. T-13-39 by `TestMemberReqCaptchaGuard` (re-apply, fail closed, never permanent, row kept and the next run unmutes); T-13-58 by `TestMemberReqCaptchaGuardOffUntilLive` and the accepted, documented cost; T-13-40 by `TestMemberReqJoinNoCaptcha`; T-13-41 by `TestMemberReqJoinAfterCaptcha` and `TestMemberReqJoinOffUntilLive`; T-13-42 by `TestMemberReqJoinExemptions`; T-13-SC by the unchanged `go.mod` and `go.sum`.

## Notes for later plans

- 13-12 settles holds at the leave and the rejoin; 13-13 sends the held welcome (the hold's `welcome_held` is already recorded by the entry and by the claim) and is the only plan that sets `memberReqEnforcementLive`.
- STATE.md "Plan:" reads `12 of 14`; ROADMAP.md was not marked complete.
- A test that drives a captcha pass and a requirement mute on the same member must use `mreqJoinEnv` (greetings loaded, a recognisable welcome text, Check on join set) and `mreqRestricts` for the member's restrict history: the captcha mute, the captcha unmute and the requirement mute are three calls in that order.

## Self-Check: PASSED

- Files exist: `fuku/modules/memberreq_join.go`, `fuku/modules/memberreq_join_test.go`.
- Commits `6ec222c`, `f79427a`, `db0c3b6`, `1f4b254` are ancestors of HEAD.
