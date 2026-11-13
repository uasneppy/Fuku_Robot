---
phase: 11-message-editor
plan: 05
subsystem: modules
tags: [greetings, rules, apply-functions, message-editor, tdd, refactor]
requires:
  - phase: 11-message-editor
    provides: plan 11-03 LoadSettingsContext, LoadRulesContext and the six one-part writers
provides:
  - applyWelcomeEnabled, applyGoodbyeEnabled, applyCleanWelcome, applyCleanGoodbye (one write path of the switches)
  - applyGreetingText, Media, Buttons, RemoveText, RemoveMedia, RemoveButtons and Reset (one write path of the parts)
  - applyRulesText and applyRulesClear (one write path of /setrules and /clearrules)
  - renderGreeting and buildRulesText (one builder of what a member sees)
affects: [11-08, 11-09, 11-11, 11-12, 11-13, settings-message-editor]

actuals:
  tokens: 14000
  tasks: 3
  commits: 5

commits: 5
plan_head_before: b1e5d9e819a98fb96e3ef49e01b092943bcd0f78
plan_head_after: cb200c96a7a424d885b87e32d95c3d0f842fa69c

tech-stack:
  added: []
  patterns:
    - "Typed apply result (outcome, stored row read back, error) beside the command, with a package-level write seam (greetingWrites, rulesWrites) a test can make fail"
    - "A message a member sees is built by one pure function (renderGreeting, buildRulesText); the senders keep their bookkeeping outside it"

key-files:
  created:
    - fuku/modules/greetings_apply_test.go
    - fuku/modules/rules_apply_test.go
  modified:
    - fuku/modules/greetings.go
    - fuku/modules/rules.go

key-decisions:
  - "applyGreetingRemoveButtons writes a nil list, not an empty one: that is exactly what the reset's four-column write stores, so three Removes and one reset leave an identical row (TestGreetingRemovesEqualReset)."
  - "The read-back after a write runs on context.WithoutCancel of the caller's context with its own 5 s bound, so a cancelled command context never turns a stored write into a missing read-back."
  - "rulesApplyResult reuses greetingApplyOutcome (greetingApplied, greetingSaveFailed) instead of a second outcome type, as the plan's type table says."

requirements-completed: []

duration: 25min
completed: 2026-11-04
status: complete

coverage:
  - id: D1
    description: "/welcome, /goodbye, /cleanwelcome and /cleangoodbye store the same row as the matching apply function and reply as before; a failed write is greetingSaveFailed and the command answers common_settings_save_failed"
    requirement: EDIT-10
    verification:
      - kind: unit
        ref: "fuku/modules/greetings_apply_test.go#TestGreetingApplyParity"
        status: pass
      - kind: unit
        ref: "fuku/modules/greetings_apply_test.go#TestGreetingApplyWriteFailure"
        status: pass
    human_judgment: false
  - id: D2
    description: "Part writes change only their own columns; three Removes leave the row one reset leaves; /setrules and /clearrules equal applyRulesText and applyRulesClear"
    requirement: EDIT-11
    verification:
      - kind: unit
        ref: "fuku/modules/greetings_apply_test.go#TestGreetingApplyPartWrites"
        status: pass
      - kind: unit
        ref: "fuku/modules/greetings_apply_test.go#TestGreetingRemovesEqualReset"
        status: pass
      - kind: unit
        ref: "fuku/modules/rules_apply_test.go#TestRulesApplyParity"
        status: pass
    human_judgment: false
  - id: D3
    description: "renderGreeting and buildRulesText build exactly what the join path and /rules send"
    requirement: EDIT-06
    verification:
      - kind: unit
        ref: "fuku/modules/greetings_apply_test.go#TestRenderGreetingMatchesJoin"
        status: pass
      - kind: unit
        ref: "fuku/modules/rules_apply_test.go#TestBuildRulesTextMatchesSendRules"
        status: pass
    human_judgment: false
---

# Phase 11 Plan 05: Apply functions and shared builders Summary

**Every welcome, goodbye and rules write now has one apply function (`applyWelcomeEnabled` ... `applyRulesClear`) that the commands call and the menu will call, and the message a member sees is built by `renderGreeting` and `buildRulesText`, so the menu can never store or show something the commands would not.**

## Performance

- **Duration:** ~25 min
- **Tasks:** 3
- **Files modified:** 4 (2 production, 2 new test files)

## Accomplishments

- `greetingApplyResult{Outcome, Settings, Err}` and the `greetingWrites` seam (a struct of the greetings repository writes, a variable only so a test can make a write fail). Each apply makes one write, logs a failure with the chat ID and returns `greetingSaveFailed` with the error, or reads the row back with `greetings.LoadSettingsContext` (a failed read-back is logged and returns `greetingApplied` with nil Settings).
- Switch applies (`applyWelcomeEnabled`, `applyGoodbyeEnabled`, `applyCleanWelcome`, `applyCleanGoodbye`). `displayGreeting`'s on/off branches collapsed into one branch that calls them, and the `setPref` of the two clean configs wraps them; every reply is word for word the same.
- Part applies (`applyGreetingText`, `Media`, `Buttons`, the three `Remove*` and `applyGreetingReset`, the last being the four-column write `resetGreeting` makes, which now calls it).
- `rulesApplyResult`, `rulesWrites`, `applyRulesText` and `applyRulesClear` in `rules.go`; `setRules` and `clearRules` call them.
- `renderGreeting` (FormattingReplacer then BuildKeyboard) now serves `SendWelcomeMessage`, `leftMember` and `/welcome`'s formatted display; `buildRulesText` serves `sendRules`. The clean-old deletion and `SetClean*MsgId` stay in the senders, outside the builders (T-11-15).

## Task Commits

1. **Task 1 (tracer): switch applies** - `1fcbfee` (refactor)
2. **Task 2 (TDD): part applies, removes, reset, rules**
   - RED - `416d36f` (test)
   - GREEN - `de9ebc7` (refactor)
3. **Task 3 (TDD): shared builders**
   - RED - `03a70c2` (test)
   - GREEN - `cb200c9` (refactor)

The GREEN commits are `refactor(...)`, not `feat(...)`, on purpose: this plan changes no user-visible behaviour (the commands reply and store exactly as before), so the changelog should not list them.

## TDD Gate Compliance

- **Task 2 RED:** `TestGreetingApplyPartWrites`, `TestGreetingRemovesEqualReset`, `TestGreetingApplyResetEqualsCommand`, `TestRulesApplyParity`, `TestRulesApplyWriteFailure`, with no-op stub applies so the package compiles. They failed on their assertions (stub writes left the rows as they were). `gsd_run check tdd-red-evidence` on the run (target `TestGreetingRemovesEqualReset`) returned `RED_EVIDENCE_OK` (`target_test_failed`). Semantic assessment: the target ran and failed on the planned "three Removes equal one reset" assertion, not on a build or fixture fault.
- **Task 2 GREEN:** the real applies; all five pass.
- **Task 3 RED:** `TestRenderGreetingMatchesJoin` and `TestBuildRulesTextMatchesSendRules` with stub builders returning empty values; both failed on their assertions (stub text `""` against the formatted text; builder text against what `/rules` sends). Semantic assessment: build succeeded, the planned equality assertions failed.
- **Task 3 GREEN:** the real builders; both pass.
- Commit order: `test(11-05)` `416d36f` precedes its GREEN `de9ebc7`; `test(11-05)` `03a70c2` precedes its GREEN `cb200c9`. The GREEN commits are typed `refactor`, not `feat`, per the note above, so a grep for `^feat\(11-05\)` finds none; the gate is satisfied by the test-then-implementation order.
- Task 1 is a `type="tracer"` task: its tests were written with its code and run before the commit. Tracer gate: the task's `<verify>` command passed end to end before expansion continued.

## Files Created/Modified

- `fuku/modules/greetings.go` - result types, `greetingWrites`, the eleven apply functions, `renderGreeting`, and the commands calling them
- `fuku/modules/rules.go` - `rulesApplyResult`, `rulesWrites`, `applyRulesText`, `applyRulesClear`, `buildRulesText`, and `setRules`, `clearRules`, `sendRules` calling them
- `fuku/modules/greetings_apply_test.go` - parity, absolute-value, write-failure, part-write, Remove-equals-reset, reset and render tests
- `fuku/modules/rules_apply_test.go` - rules parity, write-failure and builder tests

## Decisions Made

See `key-decisions` above. Planner assumptions kept: the rules double escaping (Pitfall 3) is not fixed (`buildRulesText` runs `normalizeRulesForHTML` as `sendRules` did); `/setwelcome` and `/setgoodbye` still call the four-column writes directly (their parts are compared in plans 11-11 and 11-13). No AGENTS.md edit: no rule changed.

## Deviations from Plan

None - plan executed exactly as written.

**Total deviations:** 0.

## Verification (real results)

- Task 1: `go test -tags testtools -race -count=1 -run '^(TestGreetingApplyParity|TestGreetingApplyWriteFailure|TestWelcomeAndGoodbyeTogglesPersistForNewChat|TestGreetingCleanupCommandsPersistForNewChat|TestGreetingToggleRejectsNonAdminUsers)$' ./fuku/modules`: PASS (plus `TestGreetingApplyIsAbsolute`).
- Task 2: `... -run '^(TestGreetingRemovesEqualReset|TestRulesApplyParity|TestSetAndResetGreetingTextCommands|TestSetRulesStoresCommandTextAndReplies|TestSetRulesWithoutTextDoesNotOverwriteExistingRules|TestClearRulesRemovesStoredRules)$' ./fuku/modules`: ok (with the other new tests).
- Task 3: `... -run '^(TestRenderGreetingMatchesJoin|TestBuildRulesTextMatchesSendRules|TestMemberJoinAndLeaveSendConfiguredGreetings|TestWelcomeDisplaySendsStatusAndGreeting|TestSendRulesUsesPrivateButtonWhenEnabled|TestLeftMemberDeletesPendingCaptchaAttemptAndCleanGoodbye)$' ./fuku/modules`: ok. The plan's wide prefix run (`TestGreeting|TestWelcome|TestSetGreeting|TestMemberJoin|TestCleanService|TestProcessSingleNewMember|TestNewMember|TestLeftMember|TestPendingJoin|TestJoinRequest|TestAutoApprove|TestSetRules|TestSendRules|TestRules|TestPrivateRules|TestClearRules|TestRenderGreeting|TestBuildRulesText|TestSetAndReset`): ok. The orchestrator's extra run `-run 'Goodbye|Filter|Note|Rules|Welcome|Greeting' ./fuku/modules`: ok.
- Acceptance greps: `func applyWelcomeEnabled/applyGoodbyeEnabled/applyCleanWelcome/applyCleanGoodbye` = 4; the seven part applies = 7; `applyRulesText`/`applyRulesClear` = 2; `renderGreeting(` = 4; `buildRulesText(` = 2; `formatting.FormattingReplacer` in greetings.go = 1 (inside `renderGreeting`); `greetings.SetWelcomeToggle|SetGoodbyeToggle` only on the two seam defaults; the only direct `rules.SetChatRules` call is the seam default (the other two grep hits are `SetChatRulesButton`).
- `CGO_ENABLED=0 go build ./...`: ok. `git diff --exit-code 72a692b -- go.mod go.sum`: exit 0. `gofmt -l` on touched files: clean. `go vet -tags testtools ./fuku/modules`: clean.
- Not run: `make lint` (installed golangci-lint is built with Go 1.25 and refuses Go 1.26.0), `make test` (not asked by this plan), the whole `fuku/modules` package (slow; only the plan's and the orchestrator's filtered runs), PostgreSQL (nothing here needs it).

## Requirements

`requirements-completed: []`. The plan contributes to EDIT-10, EDIT-11 and EDIT-06 but completes none (EDIT-10 is completed by 11-08, EDIT-11 by 11-13, EDIT-06 by 11-12), so all three stay Pending in REQUIREMENTS.md.

## Issues Encountered

None.

## Known Stubs

None. (The RED-phase stubs were replaced in the GREEN commits.)

## Threat Flags

None. No new endpoint, auth path or schema; T-11-13, T-11-14 and T-11-15 are mitigated by the tests named above and by keeping the clean-old bookkeeping outside the builders.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

Plans 11-08 (switches), 11-09 (Remove), 11-11 and 11-13 (typed reply) and 11-12 (previews) call these functions and builders instead of writing or building a greeting themselves.

---
*Phase: 11-message-editor*
*Completed: 2026-11-04*

## Self-Check: PASSED
