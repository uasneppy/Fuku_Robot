---
phase: 17-command-edit-and-self-destruct-deletion
plan: 04
subsystem: modules
tags: [command-deletion, anonymous-admin, delete-right-latch, reply-guard, staff-group]

requires:
  - phase: 17-command-edit-and-self-destruct-deletion
    provides: plan 17-01 (member kinds, deletion.NeedsDeleteRight) and plan 17-03 (CommandDeletionProcessor, botCommandName)
provides:
  - a proof that an anonymous admin's command is deleted only after the proof tap re-entered and ran it (order proof delete, ban, reply, command delete), and stays with no tap or with the AnonAdmin setting on (D-34 limit)
  - tests for every sender and outcome of command deletion (member, approved, creator; refused, disabled, basic group, failed, /cancel; held, repeated; Staff Group and failing Staff Group read)
  - TestCommandRepliesStayInsideTheHandler, a go/parser guard that no goroutine in fuku/modules replies (recorded allow-list: empty)
  - the delete-right latch generalised from service kinds to every kind that needs the right (service and member), at all five sites
affects: [17-05, 17-06, 17-07, 17-11, 17-12]

plan_head_before: deabec9551c8f0abea3900dfcae21bdd4d6ea0f2
plan_head_after: 10f4b7dfb5d5fede251745791a7c8eab6dfe50d6

actuals:
  tokens: 9950    # chars/4 over the added lines of my four commits (39813 chars), tests and AGENTS.md included
  tasks: 3
  commits: 4      # my four commits; rev-list deabec9..HEAD counts 5 because commit 4a4d72b (fix(staff), another agent's, staff_action.go only) sits between them

tech-stack:
  added: []
  patterns:
    - "a structural go/parser guard with a recorded allow-list and a scanner self-test"
    - "one NeedsDeleteRight predicate at every latch site instead of per-class tests"

key-files:
  created:
    - fuku/modules/command_deletion_edge_test.go
    - fuku/modules/command_replies_guard_test.go
    - fuku/modules/deletion_member_right_test.go
  modified:
    - fuku/modules/deletion_schedule.go
    - fuku/modules/deletion_worker.go
    - fuku/modules/deletion_right.go
    - AGENTS.md

key-decisions:
  - "The wrapper from plan 17-03 already satisfied the anonymous-admin behaviour, so Task 1 is a test-only commit; mutation proofs show the tests bite."
  - "The reply guard follows plain same-package calls and calls on the caller's own receiver only (no type information); the allow-list is empty because no goroutine of the package replies."
  - "The latch keeps its stored value service and its keys; only its meaning widened to every kind that needs the delete right."

requirements-completed: []

coverage:
  - id: D1
    description: "An anonymous admin's /ban is not deleted at the first update; after the tap the ordered call log reads deleteMessage(proof), banChatMember, sendMessage(reply), deleteMessage(command) strictly in that order; with no tap, after the cache expired, and with AnonAdmin on the command stays (D-34 limit); the cached Group Anonymous Bot is not read as the sender"
    requirement: "DEL-03 (contribution only)"
    verification:
      - kind: integration
        ref: "go test -tags testtools -race -count=1 -run '^(TestCommandDeletionAnonymousAdmin|TestAnonymousAdminBanAfterProof)$' -v ./fuku/modules"
        status: pass
    human_judgment: false
  - id: D2
    description: "Members, approved users and the creator are deleted after the answer; service accounts, bots and channel senders are not; refused, disabled, basic-group, failed and /cancel commands are deleted after their answer; a held member's command and a command a handler already deleted cost one quiet refused call with no message ID logged; a Staff Group (and a failing Staff Group read) is never touched"
    requirement: "DEL-03, DEL-07 (contributions only)"
    verification:
      - kind: integration
        ref: "go test -tags testtools -race -count=1 -run '^(TestCommandDeletionSenders|TestCommandDeletionRefusedAndFailed|TestCommandDeletionHeldAndRepeated|TestCommandDeletionStaffGroupNeverTouched)$' -v ./fuku/modules"
        status: pass
    human_judgment: false
  - id: D3
    description: "No goroutine in fuku/modules replies (recorded allow-list is empty); a new replying goroutine fails the guard naming file and function; the scanner finds literal, ReplyParameters, named-callee and own-receiver cases and ignores send-only goroutines and a same-named method of another type"
    requirement: "DEL-03 (contribution only)"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^(TestCommandRepliesStayInsideTheHandler|TestRepliesGuardFindsAReplyingGoroutine)$' -v ./fuku/modules"
        status: pass
    human_judgment: false
  - id: D4
    description: "With a bot that cannot delete, a member:commands or member:edited delete makes one refused call and one live check, then no call; due member:selfdestruct rows decide the latch once and are dropped with no further call, later rows too; pending member rows go with the decision; bot kinds still delete and never take the check; a bot with the right sets no latch and takes the check once per guard"
    requirement: "DEL-04, DEL-05, PLAT-06 (contributions only)"
    verification:
      - kind: integration
        ref: "go test -tags testtools -race -count=1 -run '^TestMemberKindsNeedDeleteRight$' -v ./fuku/modules"
        status: pass
    human_judgment: false

duration: 70min
completed: 2026-11-12
status: complete
---

# Phase 17 Plan 04: Command deletion edges and the delete-right latch Summary

Command deletion now has proofs for every way a command arrives (anonymous admin after the proof tap, refused, failed, held, Staff Group), a structural guard that no goroutine replies after its handler, and a delete-right latch that covers service and member kinds at every site.

## What was built

- **Task 1 (tracer), `3902da6`**: `TestCommandDeletionAnonymousAdmin` on the existing anonymous lockdown environment with the Processor wrapper and the commands rule stored as 0. Subtests: "after the tap" (no delete at the first update; call log order proof delete < banChatMember < reply sendMessage < command delete, one command delete), "AnonAdmin on" (D-34: the ban and reply run at the first update, the command stays, no row), "no tap" (also after the 20 s cache entry expired), "the cached sender is not read" (the cached `From` is the Group Anonymous Bot, the delete still follows the tap), "the first update is skipped". The test passes against plan 17-03's `commandDeletionAfter` unchanged, so there is no production change and no separate red commit.
- **Task 2, `f7f34f0`**: `TestCommandDeletionSenders` (6 rows), `TestCommandDeletionRefusedAndFailed` (check refusal of a member's `/ban`, disabled `/banme`, `/lockdown` in a basic group, a handler error, `/cancel`), `TestCommandDeletionHeldAndRepeated` (pending captcha, and `/del` deleting its own command, each with the second delete answered "message to delete not found", no row, `deletionWantNoMessageIDs` on the log), `TestCommandDeletionStaffGroupNeverTouched` (a Staff Group, and `deletionStaffLookup` failing). `TestCommandRepliesStayInsideTheHandler` plus its scanner self-test.
- **Task 3, `01730e8` (red) and `10f4b7d` (green)**: `deletion.NeedsDeleteRight` replaces `IsServiceKind` in `deletionLatched`, `deletionInlineDecideLatch`, and twice in `maybeLatchService`; `deletionDecideServiceLatch` (deviation note 2) drops the chat's pending rows once per prefix (`deletionServicePrefix`, new `deletionMemberPrefix`). `TestMemberKindsNeedDeleteRight`: commands, edits, self-destruct rows via the worker, pending member rows dropped with the decision, bot kinds untouched, a bot with the right. AGENTS.md: the `norights` and `chk` Data bullets, the deletion core and worker bullets, and the Command deletion bullet's limits (including the sentence "AnonAdmin setting is on").

## Verification results (real)

- `go test -tags testtools -race -count=1 -run '^(TestCommandDeletionAnonymousAdmin|TestAnonymousAdminBanAfterProof)$' -v ./fuku/modules`: both PASS (5 + 3 subtests).
- `go test ... -run '^(TestCommandDeletionSenders|TestCommandDeletionRefusedAndFailed|TestCommandDeletionHeldAndRepeated|TestCommandDeletionStaffGroupNeverTouched)$' -v ./fuku/modules`: all PASS (6 + 5 + 2 + 2 subtests).
- `go test ... -run '^(TestCommandRepliesStayInsideTheHandler|TestRepliesGuardFindsAReplyingGoroutine)$' -v ./fuku/modules`: both PASS.
- Red: `TestMemberKindsNeedDeleteRight` failed on 6 subtests before the change (e.g. "latch after the first refused commands = "", want "service""); after it, PASS.
- Wider run `-run '^(TestCommandDeletion|TestMemberKindsNeedDeleteRight|TestCommandRepliesStayInsideTheHandler|TestRepliesGuard|TestDeletion|TestServiceWatcher|TestAnonymousAdminBanAfterProof|TestBotCommandNameMatchesGotgbot)' ./fuku/modules`: `ok ... 52.723s` (the Phase 16 latch, worker, inline and service watcher tests are unchanged and green; the one JSON "Recovered from panic" log line in that run is the injected panic of `TestServiceWatcherNeverEndsUpdate`).
- Mutation proofs (each reverted): replacing the wrapper's sender test with `sender == nil` and a constant ID failed 4 anonymous subtests ("the command was deleted at the first update, before the proof"); making `deletionIsStaffChat` return false failed both Staff Group subtests ("deletes of message 101 = 1, want none").
- Acceptance greps: code lines with `IsServiceKind` in `deletion_schedule.go` and `deletion_worker.go` = 0; `NeedsDeleteRight` in both files (3 each, 2 files non-zero); `AnonAdmin setting is on` in AGENTS.md = 1; `deletionMemberPrefix` in `deletion_right.go` = 3; the three named test funcs of the edge file and the guard func present.
- `CGO_ENABLED=0 go build ./...` ok; `go vet -tags testtools ./fuku/modules` ok; `gofmt -l` on the seven files this plan created or edited prints nothing.
- `python3 .planning/phases/13-member-requirements/handoff/lint-local.py 8c0c827`: first run exit 1 with one QF finding (De Morgan's law in the edge test), fixed; rerun exit 0 ("no dupl, gocyclo or godox findings on lines added since 8c0c827"), exit code read directly.
- `make check-docs`: "No drift detected". `git diff --exit-code 8c0c827 -- go.mod go.sum` exit 0. No `reflect.Ptr` or `ioutil.` in the new files.
- Not run: `make lint` (cannot run here), `make test` and the whole `fuku/modules` package (not this plan's task), the settings test family (this plan edits no settings code), the CI-only govet `inline` analyzer, `node --test` (no page change).

## Deviations from Plan

- **No separate red commit for Task 1** (the plan allowed this: the test passes unchanged against 17-03's code; mutation proofs show it bites). Task 2's tests also passed on first run against the existing code (tests and guard in one commit, no production change), so its commit is `test(17-04)`, not `feat`.
- **Extra test `TestCommandDeletionHeldAndRepeated`** (not in the plan's list) for the held-member and handler-already-deleted cases, kept apart so the plan's named tests keep their meaning. The "handler already deleted" case uses `/del` (the purge family's single-message form) as the representative.
- **The reply guard resolves calls by plain name and by the caller's own receiver only.** A first version followed any selector by bare method name and reported two false positives (a method named `start` colliding with `help.go`'s `start`); the final scan reports none, so the recorded allow-list is empty. Reaching a reply through another object's method is outside what it sees; plan 17-11 reviews that limit.
- **A foreign commit** `4a4d72b fix(staff): ...` (staff_action.go only) landed on the branch during my run; it is not mine and I did not touch it, so `rev-list deabec9..HEAD` reads 5 while my plan made 4 commits.

## Requirements

DEL-03, DEL-04, DEL-05, DEL-07 and PLAT-06 are contributed to, none completed: `requirements-completed: []`, `requirements.mark-complete` was not run (plan 17-12 does).

## Known Stubs

None.

## Threat Flags

None. T-17-13 to T-17-17 mitigated as the plan states: post-run `ctx.Update.Message` and `ctx.EffectiveSender` with the first update skipped; `deletionAct` refuses a Staff Group and fails closed; `NeedsDeleteRight` at every latch site plus the member-prefix drop; the reply guard; no message ID in the log of a refused delete (asserted).

## Self-Check: PASSED

- FOUND: fuku/modules/command_deletion_edge_test.go, fuku/modules/command_replies_guard_test.go, fuku/modules/deletion_member_right_test.go
- FOUND commits: 3902da6, f7f34f0, 01730e8, 10f4b7d (all ancestors of HEAD)
