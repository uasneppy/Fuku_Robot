---
phase: 13-member-requirements
plan: 13
subsystem: modules
tags: [member-requirements, callback, i-fixed-it, mute-marker, held-welcome, kill-switch, enforcement-on]
requires:
  - phase: 13-member-requirements
    provides: memberReqUnmutePermissions, memberReqLiftMute, resolveUnmutePermissionsWithLockdown (13-08); isMemberReqMute, memberReqLiveMember, memberReqBroken, memberReqRulesText, memberReqMention, memberReqPaced, memberReqFixedButton, memberReqRecordCtx (13-07, 13-02); memberreq.GetHoldByIDFresh, TakeHoldByID, RestoreHold (13-01); the module, the switch and withMemberReqLive (13-09); the join entry with welcome_held and the captcha guard (13-11); mreqReplica (13-09)
provides:
  - the mreq|v1 "I fixed it" callback (memberReqFixedCallback, memberReqFixedRun, memberReqParseFixed, memberReqFixedTimeout), registered by LoadMemberRequirements with callbackquery.Prefix("mreq|") and no channel permission
  - the held welcome sent once through SendWelcomeMessage after a join hold is freed
  - memberReqEnforcementLive true by default, stored by memberreq.go's init() (the one production writer)
  - test helpers mreqMuteByMessage, mreqPressFixed, mreqPressData, mreqFixedData, mreqAnswers, mreqOnlyAnswer, mreqFix, mreqSeedHold, mreqPressRefused, mreqWantHeld, mreqNoticeWith
affects: [13-14, member-requirements]

actuals:
  tokens: 14400
  tasks: 3
  commits: 6

commits: 6
plan_head_before: d7bb0b44a1aad6afea4e1f2c187195458077825d
plan_head_after: 6878d1bb493f2bf36f0c3a5e4550a70dfcff6528

tech-stack:
  added: []
  patterns:
    - "A press answered once through a recover plus a deferred single answer (settingsCallback shape), in the group's language"
    - "Take the row with a conditional DELETE ... RETURNING before the side effect, put it back with its ID when the side effect fails"
    - "The kill switch is stored in init() and read through one accessor; tests write it only through a helper that restores the previous value"

key-files:
  created:
    - fuku/modules/memberreq_fixed.go
    - fuku/modules/memberreq_fixed_test.go
  modified:
    - fuku/modules/memberreq.go
    - fuku/modules/memberreq_notice.go
    - fuku/modules/memberreq_env_test.go
    - fuku/modules/memberreq_watch_test.go
    - fuku/modules/chat_permissions.go
    - AGENTS.md
    - .planning/REQUIREMENTS.md

key-decisions:
  - "The press edits the message the button sits on, always. The plan's memberReqFixedEditTarget (the stored notice ID when it equals the pressed message, else the pressed message) can only ever return the pressed message ID, so no such function was written; the stored notice_msg_id is never read by the handler (UI-SPEC section 8 step 3 as revised)"
  - "memberReqFixedRun returns a result struct (text, alert, and the member whose held welcome is still to send) instead of (answer, alert), because the welcome is sent by the callback after the query is answered, not inside the 10 s run"
  - "The take of the hold and its restore run on memberReqRecordCtx (survives the press deadline), so a deadline that expires between the take and the unmute can still put the row back"
  - "Step 5 (live rules re-check) uses the cached memberReqSettingsLookup, whose key is already in skipLocal, so the current settings are read through the same path as the watcher"
  - "withMemberReqLive stays the only test writer; the default is stored in init(), never in LoadMemberRequirements"

requirements-completed: [MREQ-06, MREQ-07]

duration: about 85 min of work (plus about 12 min each for two whole-package runs and 5 min of flake loops)
completed: 2026-11-07
status: complete
---

# Phase 13 Plan 13: The "I fixed it" button and enforcement on Summary

**A member muted by a requirement rule can free themselves with the mute notice's button: only they can press it, the live marker proves the mute is the requirements' own, the rules are re-checked on their live names against the current settings, the permissions come only from `resolveUnmutePermissionsWithLockdown`, the hold is taken before the unmute and restored with the same ID when it fails, a join hold's held welcome is sent once, and in the same plan the enforcement switch becomes true by default.**

## Commits (the order is the safety property)

| # | Hash | Subject | Role |
|---|------|---------|------|
| 1 | 878f925 | test(13-13): add failing tests for the I fixed it button | Task 1 RED (failing on assertions) |
| 2 | a70ff8f | feat(13-13): let a muted member free themselves once they fix their profile | Task 1 GREEN: **registers the `mreq|` handler**; the switch is still false |
| 3 | d526fde | test(13-13): add failing tests for the button's refusals, lockdown and still-broken answers | Task 2 RED |
| 4 | 6d7763d | feat(13-13): I fixed it lifts only the requirements' own mute | Task 2 GREEN: live rules re-check and the lockdown variant; the switch is still false |
| 5 | 3ba4da4 | test(13-13): add failing tests for the held welcome, the button size and the enforcement default | Task 3 RED (`TestMemberReqEnforcementLive` fails: default still false) |
| 6 | 6878d1b | feat(13-13): send the held welcome once and turn member requirements on | Task 3 GREEN: **`memberReqEnforcementLive.Store(true)` in `init()`**, AGENTS.md |

`git log` order confirms the handler (a70ff8f) precedes the store (6878d1b); at a70ff8f and 6d7763d `memberreq.go` has no `Store`. The only commit that turns enforcement on is 6878d1b.

## What was built

- **`fuku/modules/memberreq_fixed.go`**: `memberReqFixedCallback` (recover plus a deferred single answer, strict decode through `decodeCallbackData` and `memberReqParseFixed`, the presser check before any read, the run under `memberReqFixedTimeout` 10 s) and `memberReqFixedRun` with the UI contract's steps in order: hold read fresh and matched by ID, chat and member alone; one live `getChatMember` and `isMemberReqMute`; the live names against `memberReqSettingsLookup` (`memberReqBroken`: a rule now Off is not checked, a member who fixed only some rules gets `mreq_fixed_still_broken` for the rest and stays held); `getChat` and `resolveUnmutePermissionsWithLockdown` (via `memberReqUnmutePermissions`) before the hold is touched; the conditional `TakeHoldByID`; `memberReqLiftMute`; `RestoreHold` with the same row on failure; the edit of the pressed notice with the mention spliced after translation and no keyboard; the toast or, during a lockdown, the alert and the lockdown notice. A join hold (`welcome_held`) then sends the welcome once through `SendWelcomeMessage`, after the query is answered.
- **`memberreq.go`**: registers `handlers.NewCallback(callbackquery.Prefix("mreq|"), ...)` (no channel permission, and no such word in the file), and stores the switch default in `init()`.
- **`chat_permissions.go`**: the `resolveUnmutePermissions` comment lists "I fixed it".
- **`AGENTS.md`**: the Handlers switch sentence (true by default, one production write, read only through `memberReqLive()`, a hold-read error in the captcha guard fails closed), the executor and join bullets, a new `mreq|v1` Callbacks bullet (fields, the ordered steps, the answers, the never-lifts rule, the held welcome, the language), a Permissions bullet, and the `resolveUnmutePermissions` callers sentence.
- **Tests** (`memberreq_fixed_test.go`, helpers in `memberreq_env_test.go`): `TestMemberReqFixedUnmutes` (with a uk subtest), `TestMemberReqFixedBeforeNoticeStored` (notice ID 0 and another message), `TestMemberReqFixedRefusals` (malformed data table of 13 cases, no message and a private chat, another member and an admin with no read, a hold of another chat, of another member, taken and unknown, getChatMember 400/429/502/timeout, still broken, two rules with one fixed, a rule since Off, getChat failure and lookup error, uk answers), `TestMemberReqFixedNeverLifts` (admin /mute, /tmute, staff, captcha, antiflood, warn-limit, marker plus and minus 3 s, an admin's unmute, a member who left, each pressed twice), `TestMemberReqFixedDuringLockdown`, `TestMemberReqFixedRestoreOnFailure`, `TestMemberReqFixedTwoReplicas`, `TestMemberReqFixedHeldWelcome` (join hold with Greet once: no `greeted_members` row before the press, one welcome and one row after, a second press stale; message hold sends none), `TestMemberReqFixedCallbackBytes` (33 and 52 bytes), `TestMemberReqEnforcementLive`.

## TDD gate compliance

RED commits 878f925, d526fde and 3ba4da4 each fail on assertions (no compile-only stand-in): the press goes unanswered without a handler (task 1); the still-broken and lockdown cases fail in task 2 (the first handler commit deliberately lacks step 5 and the lockdown variant); `TestMemberReqEnforcementLive` fails on the default in task 3. Honest notes: in RED 2 the never-lifts, restore and two-replica tests already passed because the first handler commit already had the marker check, the take and the restore (the plan's tracer needed them for safety); in RED 3 the held-welcome and button-size tests already passed because the held welcome was written in the first handler commit, and the commit message says so.

## Verification (real results)

- Task 1: `go test -tags testtools -race -count=1 -run '^(TestMemberReqFixedUnmutes|TestMemberReqFixedBeforeNoticeStored)$' -v ./fuku/modules`: PASS.
- Task 2 and Task 3 verify patterns: PASS (all `TestMemberReqFixed*` and `TestMemberReqEnforcementLive`, `TestGreetOnceFirstJoinOnly`).
- **Whole package**, background, output to a scratchpad file: `go test -tags testtools -race -count=1 -timeout 30m ./fuku/modules`. The first run (before the last commit) failed on exactly one test, `TestMemberReqWatcherOffUntilLive` (see Deviations); after that fix the whole package run again: `ok github.com/uasneppy/fuku_robot/fuku/modules 705.734s`, exit 0. This run contains the settings family and the plan's family patterns, so they were not run separately.
- `go test -tags testtools -race -count=1 ./fuku/i18n ./fuku/db/...`: 37 packages `ok`, exit 0.
- Flake loops, `-count=20 -race`: `TestMemberReqFixedTwoReplicas|RestoreOnFailure|DuringLockdown|HeldWelcome|Unmutes|BeforeNoticeStored|CallbackBytes|TestMemberReqEnforcementLive` (70 s) and `TestMemberReqFixedRefusals|NeverLifts` (132 s): both ok.
- `python3 .planning/phases/13-member-requirements/handoff/lint-local.py 8c0c827`: exit 0 read directly (no dupl, gocyclo or godox findings), after every GREEN commit and after the last code commit. The plan's own lint path under phase 11 does not exist; the phase 13 stand-in was used.
- `gofmt -l` on every touched file: clean (the three files unformatted at base are not mine). `go vet -tags testtools ./fuku/modules`: clean. `CGO_ENABLED=0 go build ./...`: ok. `make check-translations`: all present. `make check-docs`: no drift. `git diff --exit-code 8c0c827 -- go.mod go.sum`: exit 0.
- Acceptance greps: `callbackquery.Prefix("mreq|")` appears once in `memberreq.go`; `SetAllowChannel` 0 in `memberreq.go`; `TakeHoldByID(` once and `RestoreHold(` once and `isMemberReqMute(` present in `memberreq_fixed.go`; no `MutedPermissions` in its code lines; `awk` over `init()` finds `memberReqEnforcementLive.Store(true)`; `I fixed it` in `chat_permissions.go`; `mreq|v1` in `AGENTS.md` 4 times.
- **Writer grep** (`grep -rn 'memberReqEnforcementLive\.\(Store\|Swap\|CompareAndSwap\)' --include=*.go .`): exactly one production writer, `fuku/modules/memberreq.go:59` in `init()`. The only other writes are in `withMemberReqLive` (`memberreq_env_test.go:80` and its restore at `:81`). The one reader is `memberReqLive()`.
- Not run: the PostgreSQL integrity target (this plan adds no migration, no `CHECK` and no partial index), `make lint` and gosec (cannot run here, per the brief), and `make test` (only plan 13-14 runs it).

## Deviations from Plan

### Existing test changed (the one the brief asked me to name)

**1. [Rule 1 - expectation that the new default invalidates] `TestMemberReqWatcherOffUntilLive` (`fuku/modules/memberreq_watch_test.go`)**
- **Found during:** the first whole-package run, after the switch was stored in `init()`.
- **Issue:** its first statement asserted "memberReqLive() is true outside any test helper, want the production default false", the exact property this plan changes. It used `t.Fatal`, so its two subtests did not run either.
- **Fix:** only that assertion now expects true, with a comment naming this plan. The subtests, which turn the switch off through `withMemberReqLive(t, false)`, are unchanged and pass. The test's name still says "OffUntilLive" and was not renamed.
- **Commit:** 6878d1b.

No other existing test changed outcome or text: the whole package, including every join, message and captcha-unmute test, passes with enforcement and the captcha guard on.

### Plan notes that differ from what was written

- `memberReqFixedEditTarget` was not written (it could only return the pressed message ID); `memberReqFixedRun` returns a struct instead of `(answer, alert)` so the held welcome can follow the answer. Both are in Key decisions.
- `mreqMuteByMessage(user)` takes only the member (the caller stores the rules and loads the modules) and returns the notice; `mreqPressFixed(from, notice)` is as planned.
- In `TestMemberReqFixedUnmutes` the "group's live defaults" are the lockdown fake's own `getChat` permissions (`lockdownSnapshotPermissions`), not `fake.chatPerms`, which the lockdown fake ignores.

## Auth gates

None.

## Known Stubs

None.

## Threat Flags

None beyond the plan's own register: the `mreq|` callback is the planned T-13-46 to T-13-51 surface (mitigated and tested), and no new endpoint, file access or schema change was added.

## Requirements

`requirements-completed: [MREQ-06, MREQ-07]`. Ticked by hand in `.planning/REQUIREMENTS.md` (the checkbox and the traceability row of each); `git diff` shows exactly those four lines changed. Phase 13 is not marked Complete: plan 13-14 and the verification chain remain.

## Self-Check: PASSED

- Created files exist: `fuku/modules/memberreq_fixed.go`, `fuku/modules/memberreq_fixed_test.go`.
- Commits exist in HEAD's history: 878f925, a70ff8f, d526fde, 6d7763d, 3ba4da4, 6878d1b.
