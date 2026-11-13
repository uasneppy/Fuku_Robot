---
phase: 17-command-edit-and-self-destruct-deletion
plan: 05
subsystem: modules
tags: [member-deletion, edited-messages, tri-state-exemptions, handler-group, staff-group]

requires:
  - phase: 17-command-edit-and-self-destruct-deletion
    provides: plan 17-01 (member kinds, deletion.ValidateRuleDelay presets) and plan 17-04 (the delete-right latch for member kinds)
provides:
  - the MemberDeletion module (priority 243) with the raw edit handler at handler group -8 (memberDeleteGroup)
  - memberEditDue, memberEditGrace, memberEditPendingCaptcha and the shared tri-state exemption memberDeletionExempt / memberDeletionAdminStatus that plan 17-07's self-destruct watcher reuses
  - the toolkit newMemberDeletionEnv (sendEdit, lateEdit, storeMemberRule, primeAdminCache, coldAdminCache, member)
affects: [17-06, 17-07, 17-11, 17-12]

plan_head_before: 10b49a6
plan_head_after: b79be69b543b66b1305d536d923673fa66895303

actuals:
  tokens: 11500   # chars/4 over the added lines of my five commits (46055 chars), tests and AGENTS.md included
  tasks: 3
  commits: 5

tech-stack:
  added: []
  patterns:
    - "a raw ext.Handler with CheckUpdate on ctx.EditedMessage and a named result preset to ext.ContinueGroups"
    - "tri-state exemptions: approved, an admin, or a read that failed all keep the message"
    - "a sibling probe in the handler's own group proves ext.ContinueGroups (a nil result leaves the group)"

key-files:
  created:
    - fuku/modules/member_deletion.go
    - fuku/modules/member_deletion_env_test.go
    - fuku/modules/member_edited_test.go
  modified:
    - AGENTS.md

key-decisions:
  - "memberDeletionAdminTimeout is a variable (not a const) so the timeout case is a 20 ms test, not a 3 s one; it was its own refactor commit."
  - "The pending-captcha check reuses the member requirements' two captcha seams (memberReqCaptchaPendingLookup, memberReqCaptchaAttemptLookup) instead of two new variables."
  - "The pure time rule runs twice: once with grace 0 as the cheap pre-filter before the rule read, once with the stored grace after it."

requirements-completed: []

coverage:
  - id: D1
    description: "With member:edited stored as 60, a member's message edited 90 s after it was sent is deleted once at once (no queue row), one edited 20 s after is kept, a group with no rule keeps both, and an administrator's and an approved member's late edits are kept"
    requirement: "DEL-04 (contribution only)"
    verification:
      - kind: integration
        ref: "go test -tags testtools -race -count=1 -run '^TestMemberEditedDeletesLateEdit$' -v ./fuku/modules"
        status: pass
    human_judgment: false
  - id: D2
    description: "The grace arithmetic holds at its edges (missing, equal and earlier edit_date, a gap equal to the grace, one second over for every preset, no date, 47 h exactly and 47 h 1 min)"
    requirement: "DEL-04 (contribution only)"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^TestMemberEditedGrace$' -v ./fuku/modules"
        status: pass
    human_judgment: false
  - id: D3
    description: "An unknown never deletes: an approvals read error, a live 429, a 500, a transport error, a timeout, an empty or unrecognised answer, a full semaphore and a failed captcha read all keep the message; the cached list answers with no live read"
    requirement: "DEL-04 (contribution only)"
    verification:
      - kind: integration
        ref: "go test -tags testtools -race -count=1 -run '^(TestMemberDeletionExemptTriState|TestMemberEditedKeeps)$' -v ./fuku/modules"
        status: pass
    human_judgment: false
  - id: D4
    description: "Every exempt sender keeps the message with no deleteMessage, no getChatMember of the bot and no queue row: cached and live administrators, the creator, an approved user, a channel post, an anonymous admin, a bot, via_bot, an automatic forward, a service ID, a pending captcha, a private chat and a stored grace of 45 (Off); the control (a plain member behind a cold cache whose live read answers member) is deleted"
    requirement: "DEL-04 (contribution only)"
    verification:
      - kind: integration
        ref: "go test -tags testtools -race -count=1 -run '^TestMemberEditedKeeps$' -v ./fuku/modules"
        status: pass
    human_judgment: false
  - id: D5
    description: "The handler never ends an update: a sibling probe in group -8 and a later probe see every edit, deleted, kept or exempt, and also after a forced panic in the rule read; a new message never reaches it; a Staff Group (and a failing Staff Group read) is never touched; a bot that cannot delete costs one refused call and one live check, then none"
    requirement: "DEL-04, DEL-07 (contributions only)"
    verification:
      - kind: integration
        ref: "go test -tags testtools -race -count=1 -run '^(TestMemberEditedNeverEndsUpdate|TestMemberEditedStaffGroupNeverTouched|TestMemberEditedNeedsDeleteRight)$' -v ./fuku/modules"
        status: pass
    human_judgment: false

duration: 55min
completed: 2026-11-12
status: complete
---

# Phase 17 Plan 05: The edited-message rule Summary

A member's message edited later than the group's `member:edited` grace is now deleted at once by a raw edit handler at group -8, and every exempt sender and every exemption that cannot be read keeps their message.

## What was built

- **Task 1 (tracer), `561a711` (red) and `784c43f` (green)**: `fuku/modules/member_deletion.go` holds the `MemberDeletion` module (priority 243, `LoadMemberDeletion`), `memberDeleteGroup = -8`, `memberEditedHandler` (`CheckUpdate` is `ctx.EditedMessage != nil` only, `Name` `member_edited`, `HandleUpdate` with a named result preset to `ext.ContinueGroups` and `defer error_handling.RecoverFromPanic`), the pure `memberEditDue`, `memberEditGrace` (only a stored preset counts, else Off), `memberDeletionExempt` (approvals through the seam `memberDeletionApprovedLookup`, default `approvals.IsUserApprovedContext`, an error is exempt, then `memberDeletionAdminStatus`, unknown is exempt), `memberDeletionAdminStatus` (the cached list when it knows, otherwise one live `getChatMember` through the seam `memberDeletionMemberLookup`, a semaphore `memberDeletionAdminSlots` of 8 that never waits and a 3 s timeout) and `memberEditPendingCaptcha` (an attempt or any read error is pending). The handler runs the order of the plan: chat type, sender shape (`raidFloodSenderCounts`, not a bot, no `via_bot`), pure time check, stored rule, exemptions, pending captcha, then `deletionAct` with kind `member:edited`, delay 0. It is registered in the same commit as every exemption. The red commit is a compile failure (the toolkit names `LoadMemberDeletion`).
- **Task 2, `2afb5c4` and `143ddb2`**: `memberDeletionAdminTimeout` became a variable (its own refactor commit) so the timeout case runs in 20 ms. `TestMemberEditedGrace` (13 cases), `TestMemberDeletionExemptTriState` (approvals read: 5 cases; admin status: 11 cases; cached list answers with no live read; timeout; full semaphore) and `TestMemberEditedKeeps` (17 keep rows plus the deleted control, each through the real dispatcher and asserting no `deleteMessage`, no new `getChatMember` of the bot, no queue row). All passed on the first run against the Task 1 code, so no `fix(17-05)` commit was needed.
- **Task 3, `b79be69`**: `TestMemberEditedNeverEndsUpdate` (a sibling probe in group -8 and a later probe; deleted, kept, exempt and panic cases; a new message never reaches the handler), `TestMemberEditedStaffGroupNeverTouched` (a Staff Group, and `deletionStaffLookup` failing) and `TestMemberEditedNeedsDeleteRight` (one refused call and one live check, then the latch; the admin list is cached before the bot is made a plain member). AGENTS.md: group `-8` in the group list, a Handlers bullet for the `MemberDeletion` module (order, tri-state exemptions, the seams, the sibling-group reasoning) and a Subsystem traps bullet "Member deletion, edited messages" with the stated limits.

## Verification results (real)

- `go test -tags testtools -race -count=1 -run '^(TestMemberEdited.*|TestMemberDeletionExemptTriState)$' -v ./fuku/modules`: all seven tests PASS (`ok ... 17.577s`).
- Plan wider run `-run '^(TestMemberEdited|TestMemberDeletionExempt|TestCommandDeletion|TestMemberKindsNeedDeleteRight|TestServiceWatcher)' ./fuku/modules`: `ok ... 40.084s`.
- `TestLoadAllModulesUsesStablePriorityOrder`, `TestRegisterLegacyModuleIgnoresDuplicateNames`, `TestRegisteredModulesInstallGotgbotHandlerGroups`: PASS.
- Mutation proofs (each reverted; `git status` clean of them afterwards): bypassing `memberDeletionExempt` failed the administrator and approved rows of `TestMemberEditedDeletesLateEdit`; turning "unknown is exempt" into "only an admin is exempt" failed the 429 and 500 rows of `TestMemberEditedKeeps`, and a captcha read error read as "not pending" failed its row; setting the handler's result to `nil` failed `TestMemberEditedNeverEndsUpdate` (the sibling probe in group -8 is skipped). A first version of that test used one later-group probe only and did not fail under `nil` (gotgbot's `nil` still moves to the next group), which is why the sibling probe was added.
- Acceptance greps: `RegisterLegacyModule("MemberDeletion", 243` = 1; the four helper funcs = 4; `IsUserApprovedContext` = 1; code lines naming `SetAllowEdited`, `IsApprovedForUpdate` or `userIsFloodExempt` = 0; `MemberDeletion` in AGENTS.md = 5, `memberDeleteGroup` = 1; the three Task 3 test funcs = 3.
- `CGO_ENABLED=0 go build ./...` ok; `go vet -tags testtools ./fuku/modules` ok; `gofmt -l` on the three Go files this plan created prints nothing.
- `python3 .planning/phases/13-member-requirements/handoff/lint-local.py 8c0c827`: exit 0 ("no dupl, gocyclo or godox findings on lines added since 8c0c827"), exit code read directly.
- `make generate-docs` then `make check-docs`: "No drift detected"; the generator wrote or changed no page.
- `git diff --exit-code 8c0c827 -- go.mod go.sum` exit 0. No `reflect.Ptr` or `ioutil.` in the new files.
- Not run: `make lint` (cannot run here), `make test` and the whole `fuku/modules` package (not this plan's task), the settings test family (this plan edits no settings code), the CI-only govet `inline` analyzer, gosec.

## Deviations from Plan

- **Task 1's pure time check is called twice** (grace 0 before the rule read, the stored grace after it), because the plan lists the time check before the rule read but `memberEditDue` takes the grace; the first call keeps "no I/O before step 4".
- **A separate refactor commit** (`2afb5c4`) for making the admin timeout a variable; the plan says production fixes go in their own commit, and this is a test seam rather than a fix.
- **`TestMemberEditedNeverEndsUpdate` uses two probes** (sibling group -8 and a later group), not the plan's one later-group probe, so it can tell `ext.ContinueGroups` from `nil` (see the mutation proofs).
- **Task 3 is one commit** (tests and AGENTS.md, subject `test(17-05)`), as the plan says AGENTS.md goes in the same commit as the tests; the plan's suggested `docs(17-05)` subject was not used because the commit holds tests too.
- **`17-SPIKE.md` is cited, not created**: AGENTS.md and the plan's threat register point to a live check in `17-SPIKE.md`, which no plan has written yet (a later plan of the phase owns it).

## Known Stubs

None. `memberDeletionExempt` is used by the edit handler now; plan 17-07 is a second caller, not the only one.

## Requirements

DEL-04 and DEL-07 receive contributions only (a late edit deleted, admins and approved users exempt, a Staff Group edit never deleted). Neither is completed: `requirements-completed: []`, `requirements.mark-complete` was not run. Plan 17-12 completes them after the end-to-end test.

## Threat Flags

None. The only new surface is a handler reading `edited_message` updates, covered by the plan's threat register (T-17-18 to T-17-22).

## Self-Check: PASSED

Files found: `fuku/modules/member_deletion.go`, `fuku/modules/member_deletion_env_test.go`, `fuku/modules/member_edited_test.go`. Commits found as ancestors of HEAD: 561a711, 784c43f, 2afb5c4, 143ddb2, b79be69. `git rev-list --count 10b49a6..HEAD` = 5.
