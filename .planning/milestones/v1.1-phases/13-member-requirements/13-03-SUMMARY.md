---
phase: 13-member-requirements
plan: 03
subsystem: modules
tags: [member-requirements, warns, refactor, characterization, kick]
requires:
  - phase: 13-member-requirements
    provides: nothing from earlier plans is used here (wave 1, depends_on is empty)
provides:
  - applyWarn and warnApplyResult in fuku/modules/warns.go, the one warn-recording and limit-action path, sending no message
  - applyWarnLimitAction, the group's kick, permanent mute or ban (unknown mode applies nothing)
  - kickMemberContext in fuku/modules/bans.go, the member-removing unbanChatMember bounded by a context
affects: [13-07, 13-11, member-requirements]

actuals:
  tokens: 8000
  tasks: 2
  commits: 4

commits: 4
plan_head_before: a5d4ee1173df45fd3f48a8dcb433f016bb76c58a
plan_head_after: ccdb5b507599c7e0e3b711ca611f6dc17e37c550

tech-stack:
  added: []
  patterns:
    - "A refactor is pinned by a characterization run (32 named tests) green before the edit and again after it, with no existing test edited"
    - "A core that does the state change and returns a typed result; the command maps the result to its unchanged replies"

key-files:
  created:
    - fuku/modules/warns_apply_test.go
  modified:
    - fuku/modules/warns.go
    - fuku/modules/bans.go

key-decisions:
  - "warnApplyResult carries one field beyond the planned list, Recorded, so warnThisUser can tell the record error (the save-failed reply) from a reset error (returned raw, as before) without a sentinel error type"
  - "applyWarn's kick limit action uses kickMemberContext with the update context, the mute and ban keep the warn module's own unpaced chat.RestrictMember and chat.BanMember calls for parity"
  - "AGENTS.md is not edited: no rule is invalidated, and its line for applyWarn comes in 13-07 where the requirement path first uses it"

patterns-established:
  - "Test fixtures read the stored warn count from the warns row's NumWarns, never the row count (one row per member and chat)"

requirements-completed: []

coverage:
  - id: D1
    description: "The 32 existing warn and blacklist command tests are green before the extraction and green again after it, the same names, with warns_command_test.go and blacklists_command_test.go unedited"
    requirement: "MREQ-05"
    verification:
      - kind: unit
        ref: "scratchpad charrun.sh (the plan's -run list of 32 names): before 32 PASS 0 FAIL; after 32 PASS 0 FAIL; the PASS name sets are identical"
        status: pass
      - kind: other
        ref: "git diff --exit-code 8c0c827 -- fuku/modules/warns_command_test.go fuku/modules/blacklists_command_test.go (exit 0)"
        status: pass
    human_judgment: false
  - id: D2
    description: "applyWarn records one warn under warnPunishLock, runs afterRecord once inside the lock after the warn is stored, returns the count, limit, mode and reasons, and sends no message, restriction, ban or unban below the limit; a warn that cannot be recorded returns the error, runs no afterRecord and makes no Telegram call"
    requirement: "MREQ-05"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^TestApplyWarn$' ./fuku/modules (below the limit, afterRecord inside the lock, record error)"
        status: pass
    human_judgment: false
  - id: D3
    description: "At the limit applyWarn runs the group's action once (one unbanChatMember with only_if_banned false for kick, one restrictChatMember with every send right off and no until_date for mute, one banChatMember with no until_date for ban), resets the warns, reports LimitReached and LimitApplied, and sends no message; a refused action reports LimitErr, LimitApplied false and keeps the warns"
    requirement: "MREQ-05"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^TestApplyWarn$' ./fuku/modules (at the limit kick, mute, ban; limit action refused)"
        status: pass
    human_judgment: false
  - id: D4
    description: "kickMemberContext sends the member-removing unban for the chat and user and returns the Telegram error; a cancelled context makes no request; kickMember makes the same request as before"
    requirement: "MREQ-05"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^TestKickMemberContext$' ./fuku/modules"
        status: pass
    human_judgment: false

status: complete
---

# Phase 13 Plan 03: The warn core extraction Summary

**`applyWarn` now holds the warn module's whole record-and-limit path (lock, store, `afterRecord` hook, kick or permanent mute or ban at the limit, reset only after the action took effect) and sends nothing; `warnThisUser` maps its typed result to the same replies as before, and `kickMemberContext` bounds the member-removing unban by a context.**

## Performance

- **Tasks:** 2 of 2 (task 1 tracer, task 2 TDD)
- **Commits:** 4 (two RED `test(13-03)`, one `refactor(13-03)`, one `feat(13-03)`)
- No locale key, migration, handler, callback or kill-switch change. `memberReqEnforcementLive` is untouched (it does not exist yet). The only new caller of `applyWarn` besides `warnThisUser` is its own tests; plan 13-07 is its production user.

## Accomplishments

- `warnApplyResult{Count, Limit, Mode, LimitReached, LimitApplied, LimitErr, Reasons, Recorded}` and `applyWarn(ctx, b, chat, userID, reason, afterRecord)`. `LimitApplied` is true only after the action and the reset both succeeded. A refused action is `LimitErr` with the warn still counted and nothing reset; a reset failure is the function's error (with `Recorded` true); an unknown mode logs the same warning and applies nothing.
- `applyWarnLimitAction` carries the three limit actions and their log lines (`[warn] warnlimit: kick|mute|ban (...)`, `[Warns] Unknown warn mode`) unchanged.
- `warnThisUser` keeps lines for the admin refusal, the live `GetMember`, the translator and buttons, the save-failed reply, the `/dwarn` reply deletion and `/swarn` command deletion (now the `afterRecord` closure, still inside the lock and after the warn is stored), the three limit texts with the reasons list, the warning text with Remove warn and Rules buttons, and every error return (`LimitErr` returned where the action's error was).
- `kickMemberContext` (`UnbanChatMemberWithContext`, `OnlyIfBanned: false`); `kickMember` calls it with `context.Background()`. `applyWarn`'s kick uses it with the update context.

## Characterization gate (real results)

The plan's 32 named tests (every top-level test of `warns_command_test.go` and `blacklists_command_test.go`), run with `-tags testtools -race -count=1 -v ./fuku/modules`:

- Before any edit of `warns.go` (HEAD a5d4ee1): 32 PASS, 0 FAIL, `ok`.
- After the extraction (task 1, commit 146bd31): 32 PASS, 0 FAIL.
- At plan end (HEAD ccdb5b5, with the new tests in the same binary): 32 PASS, 0 FAIL; the sorted PASS name lists before and after are identical.
- `git diff --exit-code 8c0c827 -- fuku/modules/warns_command_test.go fuku/modules/blacklists_command_test.go`: exit 0 (no existing test edited).

## TDD Gate Compliance

| Gate | Commit | Evidence |
|------|--------|----------|
| RED task 1 | a5619de `test(13-03): add failing tests for recording a warn through applyWarn` | the package's tests do not compile without `applyWarn` (build error, not an assertion failure; the symbols did not exist at HEAD, as the plan's baseline says) |
| GREEN task 1 | 146bd31 `refactor(13-03): record a warn and its limit action through applyWarn` | the three TestApplyWarn subtests and the 32 characterization tests pass |
| RED task 2 | 8f4e0e6 `test(13-03): add failing tests for the warn limit action and kickMemberContext` | does not compile without `kickMemberContext`; after adding it, the first GREEN attempt failed on assertions of my own tests (see deviations 1 and 2), proving they ran |
| GREEN task 2 | ccdb5b5 `feat(13-03): report the warn limit action and bound a kick by a context` | `TestApplyWarn` (7 subtests) and `TestKickMemberContext` (4 subtests) pass |

## Verification (real results)

- `go test -tags testtools -race -count=1 -run '^(TestApplyWarn|TestKickMemberContext)$' -v ./fuku/modules`: PASS for all 11 subtests; `ok`.
- `CGO_ENABLED=0 go build ./...`: ok. `go vet -tags testtools ./fuku/modules`: clean. `gofmt -l` on `warns.go`, `bans.go`, `warns_apply_test.go`: empty (it lists seven older files of the package that are not mine).
- `git diff --exit-code 8c0c827 -- go.mod go.sum`: exit 0.
- Lint stand-in `lint-local.py 8c0c827`: exit 0, run before the last code commit and again after it (`no dupl, gocyclo or godox findings`; staticcheck and errcheck clean). `make lint` itself cannot run here (golangci-lint built with Go 1.25).
- `make generate-docs` left no diff; `make check-docs`: "No drift detected"; `make check-translations`: "All translations are present".
- Not run: `make test` and the full `fuku/modules` package (only plan 13-14 runs them); the settings test family (the plan does not ask for it).
- Lint note for the next plan: `applyWarn` and `kickMemberContext` are both called from production code (`warnThisUser`, `kickMember`), and the new test helpers are all used, so the `unused` check is quiet. The non-nil `afterRecord` path is called from `warnThisUser` and from `TestApplyWarn`; the nil path from `TestApplyWarn`.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug in my own test] The stored warn count was counted by rows**
- **Found during:** Task 2 first GREEN run
- **Issue:** `storedWarns` counted `warns_users` rows, but there is one row per member and chat holding `NumWarns`; the refused-action subtest saw 1 where 2 were stored. The below-limit subtest had passed only by coincidence.
- **Fix:** `storedWarns` reads the row's `NumWarns` (0 when none).
- **Files modified:** `fuku/modules/warns_apply_test.go`
- **Commit:** ccdb5b5

**2. [Rule 1 - Bug in my own test] Two fake-client assumptions**
- **Found during:** Task 2 first GREEN run
- **Issue:** gotgbot leaves a false `only_if_banned` off the wire (Telegram's default is the same false), so asserting the key is present failed; and the shared fake client ignores the request context, so a cancelled context made a recorded request.
- **Fix:** `assertOnlyIfBannedOff` accepts an absent or false key; `ctxAwareBotClient` wraps the fake and refuses a request whose context is done, as gotgbot's HTTP client does.
- **Files modified:** `fuku/modules/warns_apply_test.go`
- **Commit:** ccdb5b5

### Judgment calls

- **`Recorded` field (beyond the planned struct).** The plan says a reset error is returned as the function's error, and `warnThisUser` must still reply `common_settings_save_failed` only for the record error and return the reset error raw, as today. `Recorded` tells them apart. No other field or signature differs from the plan.
- **Lock scope.** The old `warnThisUser` held `warnPunishLock` until it returned, which included the final reply send; `applyWarn` releases it when it returns, before the reply is sent. The lock only has to cover record, limit action and reset (its stated purpose), so for one warn the Telegram requests, their order, the replies and the stored rows are identical; only two concurrent warns of the same member could now send their replies in either order.
- **Planner-assumption note.** The plan said `applyWarn` returns the `WarnUser` error and `warnThisUser` keeps the same save-failed reply; this is what the code does.

## Known Stubs

None. `applyWarn` has its production caller (`warnThisUser`); the requirement path's use arrives in 13-07.

## Threat Flags

None. No new endpoint, auth path, file access or schema; T-13-08 (the extraction changing a command) is mitigated by the characterization run and `afterRecord` inside the lock, T-13-09 by `LimitApplied` only after action and reset, T-13-SC by the unchanged go.mod and go.sum.

## Requirement coverage

MREQ-05 is contributed to and left Pending (`requirements-completed: []`); plan 13-11 completes it. No behaviour reaches a real group differently from before.

## Self-Check: PASSED

- `fuku/modules/warns_apply_test.go`, `fuku/modules/warns.go` (`func applyWarn(` once) and `fuku/modules/bans.go` (`func kickMemberContext(` once) exist.
- Commits a5619de, 146bd31, 8f4e0e6 and ccdb5b5 are ancestors of HEAD.
