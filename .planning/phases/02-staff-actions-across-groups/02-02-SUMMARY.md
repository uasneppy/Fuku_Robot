---
phase: 02-staff-actions-across-groups
plan: 02
subsystem: staff-moderation
tags: [telegram, gotgbot, fan-out, moderation, decision-table, i18n]

requires:
  - phase: 02-staff-actions-across-groups
    provides: "Plan 02-01 tracer: StaffActions interceptor, Redis confirm card, per-group live check chain, decideStaffAction (ban column), stateful BotClient fake"
provides:
  - "Pure decision table for ban, mute, kick, unban and unmute, decided from the target's live status"
  - "Staff /mute, /kick, /unban and /unmute from the Staff Group, end to end"
  - "Never-lift-a-ban proof (exhaustive invariants plus a stateful-fake end-to-end test)"
  - "staff_act_name_* and four skip locale keys in all 7 locales"
affects: [02-03, 02-04, 02-05, 02-06, 02-07, phase-03-audit-undo]

actuals:
  tokens: 15000
  tasks: 2
  commits: 4

plan_head_before: 91fc52b211eb0f687721dd2e569942ff5d62239d
plan_head_after: 0200f2c89105b1bd76e24e371844b1cea42c4e4e
commits: 4

tech-stack:
  added: []
  patterns:
    - "One decision function is the only place that picks the Telegram write; executeStaffCall switches only on its verdict"
    - "Status-first table: restrict only to member or restricted, member-removing unban only for kick on a current member, every unban only_if_banned=true"
    - "Exhaustive invariant test over kind x status x current end x new end as a guard that survives table edits"

key-files:
  created:
    - alita/modules/staff_action_decide_test.go
    - alita/modules/staff_action_actions_test.go
  modified:
    - alita/modules/staff_action_decide.go
    - alita/modules/staff_action.go
    - alita/modules/staff_action_run.go
    - alita/modules/staff_action_summary.go
    - alita/modules/staff_action_dispatch_test.go
    - locales/en.yml
    - locales/es.yml
    - locales/fr.yml
    - locales/hi.yml
    - locales/id.yml
    - locales/pt.yml
    - locales/ru.yml
    - AGENTS.md

key-decisions:
  - "A kicked target reads 'skipped: not in group' for mute, kick and unmute (D-11 locked wording), not the 'already banned' line research suggested; behaviour is identical (no call)"
  - "A staff /kick on a muted member applies and drops the mute, as per-group /kick does (assumption A6); one table row to reverse"
  - "/unmute on a restricted non-member who is muted applies the default permissions without making them a member"

patterns-established:
  - "Adding a staff action means adding a kind, a call, reasons and one decideStaffX helper; the executor and the summary stay mechanical"

requirements-completed: [STAFF-01, STAFF-05, STAFF-07, STAFF-13]

coverage:
  - id: D1
    description: "Staff /mute, /kick, /unban and /unmute use the same card, Confirm and per-group live checks as /ban and act only where the issuer is live creator or an administrator with can_restrict_members"
    requirement: "STAFF-01"
    verification:
      - kind: integration
        ref: "alita/modules/staff_action_actions_test.go#TestStaffActionFiveActions"
        status: pass
    human_judgment: false
  - id: D2
    description: "Mute restricts with MutedPermissions, kick is unbanChatMember only_if_banned=false, unban is only_if_banned=true, unmute restores the group's live default permissions and fails the group when getChat fails"
    requirement: "STAFF-05"
    verification:
      - kind: integration
        ref: "alita/modules/staff_action_actions_test.go#TestStaffActionFiveActions"
        status: pass
      - kind: integration
        ref: "alita/modules/staff_action_actions_test.go#TestStaffActionUnmuteNonMember"
        status: pass
    human_judgment: false
  - id: D3
    description: "No staff action lifts a ban, shortens a mute or acts on a creator or administrator: restrict and member-removing unban never reach a kicked target, equal end dates are not later"
    requirement: "STAFF-07"
    verification:
      - kind: unit
        ref: "alita/modules/staff_action_decide_test.go#TestStaffActionDecisionInvariants"
        status: pass
      - kind: unit
        ref: "alita/modules/staff_action_decide_test.go#TestStaffActionDecisionTable"
        status: pass
      - kind: integration
        ref: "alita/modules/staff_action_actions_test.go#TestStaffActionNeverLiftsBan"
        status: pass
      - kind: integration
        ref: "alita/modules/staff_action_actions_test.go#TestStaffActionMuteNeverShortens"
        status: pass
      - kind: integration
        ref: "alita/modules/staff_action_actions_test.go#TestStaffActionBanOverMute"
        status: pass
    human_judgment: false
  - id: D4
    description: "The commands outside Staff Groups make the same ordered Telegram calls as a dispatcher without StaffActions; anonymous senders of any of the five get only 'post as yourself'; the header shows a duration only for ban and mute"
    requirement: "STAFF-13"
    verification:
      - kind: integration
        ref: "alita/modules/staff_action_dispatch_test.go#TestStaffActionNonStaffUnchanged"
        status: pass
      - kind: integration
        ref: "alita/modules/staff_action_dispatch_test.go#TestStaffActionAnonymous"
        status: pass
      - kind: integration
        ref: "alita/modules/staff_action_actions_test.go#TestStaffActionHeaderDuration"
        status: pass
    human_judgment: false
  - id: D5
    description: "Research flag A3: that the real Telegram server never lifts a ban when a mute or unmute is skipped for a kicked target, and that a banned account still cannot rejoin"
    verification: []
    human_judgment: true
    rationale: "The fake models the server's status replacement as inferred from the Bot API server source; the final tdlib step is not observed. The design never sends restrict or unban(false) to a banned target, so it is safe either way, but the live check on a throwaway Staff Group is still outstanding"

duration: 21min
completed: 2026-10-05
status: complete
---

# Phase 2 Plan 02: Staff mute, kick, unban and unmute Summary

**One status-first decision table (`decideStaffAction`) now drives all five staff actions, so `/mute`, `/kick`, `/unban` and `/unmute` fan out from the Staff Group with the same card and live checks as `/ban`, and no action can lift a ban or touch an administrator.**

## Performance

- **Duration:** 21 min
- **Tasks:** 2
- **Files modified:** 15 (2 created, 13 modified)

## Accomplishments

- `decideStaffAction` is split into `decideStaffBan`, `decideStaffMute`, `decideStaffKick`, `decideStaffUnban` and `decideStaffUnmute`, with the creator-or-administrator check first for every kind. A mute applies to a member or a restricted member, and skips a muted one unless the new mute ends later (equal is not later, permanent counts as latest). A kick removes only a current member. An unban acts only on a kicked target. An unmute acts only on a muted restricted target, member or not.
- `executeStaffCall` switches only on the verdict's call: `restrictChatMember` with `MutedPermissions`, `unbanChatMember` with `only_if_banned=false` for kick and `true` for unban, and for unmute a live `getChat` followed by `restrictChatMember` with `resolveUnmutePermissions(info)`. A `getChat` failure fails the group, as per-group `/unmute` does. An unknown call returns an error instead of silently doing nothing.
- The four commands are four rows in `staffActionCommands`; the interceptor, gate and card are unchanged. The header shows the duration segment only for ban and mute (`staffActionHasDuration`), each action has its own icon and localized name, and four new skip reasons read "not in group", "already muted", "not banned" and "not muted".
- `TestStaffActionNeverLiftsBan` proves with the stateful fake that a mute, an unmute and a kick aimed at a target banned in one group send zero `restrictChatMember` and zero `unbanChatMember` calls there while the group where the target is a member changes as expected. `TestStaffActionDecisionInvariants` enumerates 5 kinds x 18 live states (creator, administrator, member, left and kicked at two end dates each, plus every restricted combination) x 4 new end dates and asserts the six invariants from the plan.
- AGENTS.md carries the never-lift-a-ban rule next to the Staff actions authority bullet.

## Task Commits

1. **Task 1: full decision table, test-first** - `22c5c57` (test, RED), `b6e3e6e` (feat, GREEN)
2. **Task 2: four commands end to end** - `7957a85` (test, RED), `0200f2c` (feat, GREEN)

**Plan metadata:** committed with this SUMMARY (docs: complete plan)

## TDD Gate Compliance

Both tasks are `tdd="true"` and both have a RED commit before the GREEN commit.

- **Task 1 RED (`22c5c57`):** `go test -tags testtools -race -count=1 -run '^TestStaffActionDecision|^TestStaffActionEndsLater$' ./alita/modules` exited 1. `TestStaffActionDecisionTable` failed on 29 sub-tests (every mute, kick, unban and unmute row; the rows for ban and for creator/admin already passed), each with the assertion `decideStaffAction(...) = {Call:0 Reason:fail_lookup}, want ...`. `TestStaffActionDecisionInvariants` and `TestStaffActionEndsLater` passed, which is expected: the invariants hold vacuously while the new kinds return no call, and `staffEndsLater` did not change. The new kind, call and reason constants had to be declared in the RED commit so the test compiles; `decideStaffAction` did not use them yet. GREEN is `b6e3e6e`: the same command and `TestStaffActionTracer` pass.
- **Task 2 RED (`7957a85`):** the target set `TestStaffAction(FiveActions|NeverLiftsBan|BanOverMute|MuteNeverShortens|KickClearsMute|UnmuteNonMember|HeaderDuration|NonStaffUnchanged|Anonymous)` exited 1. Failures were all on the planned behaviour: "no confirm card was sent to the Staff Group" for every mute, kick, unban and unmute scenario (the commands were not registered), "messages to the Staff Group = [], want exactly one with staff_post_as_yourself" for the anonymous cases of the four new commands, and "staff commands = 1, want ... five". `TestStaffActionBanOverMute` and `TestStaffActionNonStaffUnchanged` passed at once: ban was already wired, and the non-staff comparison is a regression guard that must hold both before and after. GREEN is `0200f2c`, after which the whole set passes.
- **Mutation check (reverted, not committed):** making `decideStaffMute` return a mute for a kicked target failed `TestStaffActionDecisionInvariants` ("restrict sent to a target who is neither member nor restricted") and `TestStaffActionNeverLiftsBan` ("line for Group A = ✅ Group A, want skipped").
- **REFACTOR:** none.
- `gsd_run check tdd-red-evidence` was not run (the `gsd_run` resolver is not available in this sandbox); the RED evidence above records command, exit code, failing tests and assertion text instead.

## Files Created/Modified

- `alita/modules/staff_action_decide.go` - four new kinds, calls and reasons, `staffDo`/`staffSkip`, the per-kind helpers
- `alita/modules/staff_action_decide_test.go` - table, exhaustive invariants, `staffEndsLater` cases
- `alita/modules/staff_action.go` - four rows in `staffActionCommands`
- `alita/modules/staff_action_run.go` - four new Telegram calls in `executeStaffCall`
- `alita/modules/staff_action_summary.go` - icons, names, `staffActionHasDuration`, new reason texts
- `alita/modules/staff_action_actions_test.go` - end-to-end tests on the stateful fake
- `alita/modules/staff_action_dispatch_test.go` - non-staff and anonymous suites widened to all five commands
- `locales/*.yml` (7) - 8 keys each
- `AGENTS.md` - never-lift-a-ban rule

## Decisions Made

- D-11 locked wording wins for a kicked target (see key-decisions); flagged for the owner at verification.
- A staff `/kick` on a muted member applies (assumption A6); flagged for the owner at verification.
- The new translations for es, fr, pt, id, ru and hi are my own and have not had a native-speaker review.

## Deviations from Plan

None - plan executed exactly as written.

The plan lists `staff_action_actions_test.go` and the decision test as the only new files; `staff_action_dispatch_test.go` also gained "mute as a reply" to the non-staff comparison, a small addition inside the planned extension.

## Deferred Issues

- `alita/modules/greetings_command_test.go` is not gofmt-clean on the base commit. Pre-existing and unrelated, left alone.
- `make lint` cannot run in this sandbox (installed golangci-lint is built with go1.25, module targets 1.26.0). `gofmt -l` on all changed Go files (clean), `go vet -tags testtools ./alita/...`, `CGO_ENABLED=0 go build ./...`, `make check-translations` and `make check-docs` all pass.
- Human check (research flag A3), outstanding: in a throwaway supergroup linked to a test Staff Group, ban a test account with the per-group `/ban`, then from the Staff Group run `/mute <id>` and Confirm, then `/unmute <id>` and Confirm, then try to rejoin. Expected: both summaries show the group as "skipped: not in group" and the account is still banned.

## Issues Encountered

None.

## Known Stubs

None.

## Threat Flags

None. No new network endpoint, auth path or schema; T-02-10 is mitigated by `TestStaffActionDecisionInvariants` and `TestStaffActionNeverLiftsBan`, and T-02-11 and T-02-12 are accepted as planned.

## User Setup Required

None - no external service configuration required.

## Verification Run

- `go test -tags testtools -race -count=1 ./alita/modules ./alita/i18n` - pass
- `go test -tags testtools -count=1 -run '^TestStaffLocaleKeys$' ./alita/i18n` - pass
- `CGO_ENABLED=0 go build ./...` - pass
- `go vet -tags testtools ./alita/...` - pass
- `make check-translations`, `make check-docs` - pass
- Acceptance greps: `OnlyIfBanned: true` and `resolveUnmutePermissions(` in `staff_action_run.go`, `Name: "unmute"` in `staff_action.go`, `only_if_banned` in `AGENTS.md`, `staffKindUnmute` and `func decideStaffUnmute` in `staff_action_decide.go` - all match
- go.mod and go.sum untouched

## Next Phase Readiness

- Plans 02-03 onward can add durations and further kinds by extending the same table; `newUntil` is still always 0 (permanent) at the Confirm call site, and the header's duration segment already keys off the action kind.
- Live-Telegram verification of the whole flow, including the A3 check above, is still outstanding.

---
*Phase: 02-staff-actions-across-groups*
*Completed: 2026-10-05*

## Self-Check: PASSED

- All 2 created and 13 modified files are present in `git diff --stat 91fc52b..HEAD` (15 files).
- Task commits `22c5c57`, `b6e3e6e`, `7957a85`, `0200f2c` are present in `git log`.
- All task acceptance criteria and the plan-level verification commands were re-run and pass (see Verification Run).
