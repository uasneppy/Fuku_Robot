---
phase: 17-command-edit-and-self-destruct-deletion
plan: 11
subsystem: member-deletion
tags: [member-deletion, end-to-end, never-delete, log-sweep, reply-guard, tests]

requires:
  - phase: 17-command-edit-and-self-destruct-deletion
    provides: the command wrapper (17-03, 17-04), the edit handler (17-05), the batcher and worker (17-06), the self-destruct watcher (17-07), the Member messages page (17-09, 17-10)
provides:
  - TestMemberDeletionEndToEnd, the whole phase through the real dispatcher with only Telegram and the clock faked
  - TestMemberDeletionExcludedMessages, proof that the three member rules never touch the never-delete families (DEL-07), behaviour and structure
  - TestMemberDeletionLogsCarryIDsOnly, the log sweep over every failing seam of the member rules
  - the review of the reply guard's resolution limit (C-03), recorded in the guard's comment and AGENTS.md
affects: [17-12]

plan_head_before: dd2a28be376b5797a86cf6c30ddf90ea8aa1b073
plan_head_after: f4c85dfc485bd9caa52642917d78c70ee0f080ac

actuals:
  tokens: 9500    # chars/4 over the added lines of dd2a28b..f4c85df in fuku/ and AGENTS.md (37948 chars)
  tasks: 2
  commits: 2      # MEASURED: git rev-list --count dd2a28b..HEAD before this SUMMARY commit

tech-stack:
  added: []
  patterns:
    - "the end-to-end test runs on the drift environment of plan 17-10 (wrapper, edit handler, watcher, primed admin cache, drained batcher) with the page's owner, one env per subtest"
    - "a log sweep drives each failing seam in its own subtest, because mreqSwap restores at the end of the subtest, and checks the lines for sentinels, message IDs and claim tokens"

key-files:
  created:
    - fuku/modules/member_deletion_e2e_test.go
  modified:
    - fuku/modules/command_replies_guard_test.go
    - AGENTS.md

key-decisions:
  - "No production code changed: every end-to-end step passed against the code of plans 17-03 to 17-10, and each new test was proven to discriminate by a mutation (below)."
  - "The reply guard stays as it is and its allow-list stays empty: a permissive bare-name scan found three candidates and none is a real reply after a delete."
  - "The never-delete structural gate is a Go subtest (a file scan over the families) as well as the plan's grep, so the invariant is held by `make test` and not only by an acceptance command."

patterns-established:
  - "a throwaway permissive scan reviews a structural guard's resolution limit before the guard is trusted"

requirements-completed: []
status: complete
---

# Phase 17 Plan 11: End-to-end and sweeps Summary

**The whole phase is proven end to end through the real dispatcher: the owner sets Commands, Edited messages and Self-destruct from the page, the wrapper, the edit handler, the watcher, the batcher and the worker carry them out, every exemption and failed read keeps its message, the never-delete list is untouched, and no log line leaks message data.**

## What changed

- `fuku/modules/member_deletion_e2e_test.go` (new), three tests:
  - `TestMemberDeletionEndToEnd`, nine subtests, each on its own environment:
    1. **the owner sets the three rules and each one acts**: the creator presses Main, Deletion, Member messages, Commands On, Edited messages 1 min, Self-destruct 10 min and the card's Yes; the three rules are stored and the hub reads `💬 Member messages: On 3`. A `/rules` is answered then deleted (reply index before delete index), a 20 s edit is kept and a 90 s one deleted, a message is queued at its date plus 600 s, kept by the worker at +599 s and deleted once at +601 s.
    2. **commands**: an admin's `/ban` and a member's `/rules` are answered then deleted; `/rules@otherbot` and `/nosuchcommand17` stay; the bot's replies stay; no queue row.
    3. **an anonymous admin's command**: deleted after the proof tap and after the ban (the 17-04 fixture); with AnonAdmin on the command runs at its first update and stays (D-34).
    4. **edits**: 20 s, exactly the grace, an admin's late edit, an approved member's late edit and a late edit of a 48 hour old message are kept; a 90 s edit is deleted once, no row.
    5. **self-destruct**: row at date plus 600 s; an admin's and an approved member's messages are never offered (flush wrote exactly 2); a message named by a pinned_message service message loses its row and stays while the other is deleted at +601 s; Off cancels the waiting row and a later message writes nothing.
    6. **unknown never deletes (D-33)**: a failed approvals read, and a cold admin cache whose live getChatMember answers 429, each keep a late edit and queue nothing.
    7. **the bot cannot delete**: with the bot a plain member every member screen of `settingsDelScreens` shows the 🚫 line once and the three rules can still be stored; Telegram then refuses the first delete: one refused call and one live check of the bot, the latch is `service`, and a second command, a late edit and a self-destruct pass cost no further call and no further check, and the latched chat's member rows are dropped.
    8. **Staff Group**: with all three rules stored, a command, a late edit and a message produce no row and no delete request, also after a worker pass at +601 s; Main has no Deletion cell and the nine forged presses (the Phase 16 set plus the member codes) are the invalid-request alert, answered once.
    9. **restart and replicas**: with the batcher's period an hour, three messages sit in its buffer; `StopDeletionWorker` writes them with its final flush; after a fresh root two workers (the env's bot and a replica's) over one database delete each message exactly once.
  - `TestMemberDeletionExcludedMessages`: with the three member rules at their shortest presets (`memberShortestRules`: 0, 30, 60), a Staff Group card and summary, a lockdown alert copy, a captcha challenge and a log-channel post sent through their real senders leave no row and no delete request, also after a worker cycle a day and a half later (the fixtures of `TestDeletionExcludedMessagesNeverScheduled`, member kinds only); the bot's own reply, a command-looking text and a late edit delivered as updates from the bot are ignored; and a file scan asserts no non-test file of `staff*`, `lockdown*`, `captcha*`, `raid*`, `logchannels.go` or `actionlog` names `memberDeleteOffer`, `ScheduleBatch`, `scheduleBotMessage`, `deletionSchedule`, `deletionAct` or `deletion.Schedule` (and that at least 10 files matched, so the glob cannot go blind).
  - `TestMemberDeletionLogsCarryIDsOnly`: a debug-level hook over eleven failing seams driven with a text, a first name and a username that carry sentinels: the approvals read, the live administrator read behind a cold cache (429), a saturated administrator semaphore, the two captcha reads, the Staff Group read (the core), a full batcher buffer, an unreadable rule at the flush, an unreadable queued count, a failed batch write, the worker's rule screen over three due self-destruct rows, and a failing pin drop (a gorm delete callback on `scheduled_deletions`). No line holds a sentinel, a claim token or a message ID (`deletionWantNoMessageIDs` over every ID delivered or seeded); every Warn of the member deletion code names a chat (the batch write's own line names a count of chats); all twelve expected Warn texts appeared, so each failing seam was driven; three offers to a one-slot buffer log exactly one line for the chat.
- `fuku/modules/command_replies_guard_test.go`: the comment of the allow-list records the review below. No behaviour change.
- `AGENTS.md`: the Command deletion bullet states the guard's limit and the review result.

## Reply guard review (C-03, D-23)

Plan 17-04's guard resolves calls by plain function name and by the caller's own receiver only. To see what that limit hides, a throwaway test (not committed) parsed every non-test file of `fuku/modules` and followed **every** selector call by bare method name, to depth 4, across all receivers. It found three goroutine sites that reach a `Reply` or `ReplyParameters`, and none replies to a member's command after a delete:

- `raid_ai_count.go: raidAIFireAsync` and `staff_action_run.go: startJoinedStaffRun` reach `start`: `raidPostTracker.start` in `raid_trigger.go` has the same bare name as `help.go`'s `start`, a handler that replies (the collision plan 17-04 found with its first, bare-name version). `raidFire` posts notices with `SendMessage` and never replies.
- `startJoinedStaffRun` is the Staff Group run's coordinator; the staff command it answers is a Staff Group message, and `deletionAct` refuses every Staff Group (D-17), so the command wrapper never deletes it.

The strict guard's allow-list stays empty, the guard is unchanged, and the finding is recorded in the guard's comment and in AGENTS.md. A reply reached through an object's method with a different name would still be invisible to the strict scan; the permissive scan is the review tool for the next change to a goroutine in the package.

## Mutation checks (each reverted; the tree matches the commits)

- `memberDeletionExempt`'s final test changed from `!= memberDeletionNotAdmin` to `== memberDeletionIsAdmin` (unknown stops being exempt): `TestMemberDeletionEndToEnd/unknown_never_deletes_(D-33)/a_cold_admin_cache_whose_live_read_answers_429` failed ("a late edit was deleted 1 times behind a cold cache and a 429, want it kept").
- `memberDeletePinned`'s Warn changed to include the message ID: `TestMemberDeletionLogsCarryIDsOnly` failed ("log line at warning names message 117").

## Verification (real output)

- Task 1 gate: `go test -tags testtools -race -count=1 -timeout 20m -run '^(TestMemberDeletionEndToEnd|TestDeletionEndToEnd)$' -v ./fuku/modules`: `ok ... 7.761s`, no FAIL or SKIP line. Count 10: `go test -tags testtools -race -count=10 -timeout 30m -run '^TestMemberDeletionEndToEnd$' ./fuku/modules`: `ok ... 39.727s`.
- Task 2 gate: `-run '^(TestMemberDeletionExcludedMessages|TestMemberDeletionLogsCarryIDsOnly|TestCommandRepliesStayInsideTheHandler|TestRepliesGuardFindsAReplyingGoroutine|TestDeletionExcludedMessagesNeverScheduled)$' -v`: all five PASS, no SKIP, `ok ... 4.014s`.
- Acceptance greps: `func TestMemberDeletionEndToEnd` 1; the two Task 2 test names 2; the never-delete producer grep prints 0; the skip gate `awk` over `git diff -U0 42c48c4 -- fuku` exits 0.
- The whole package the way CI runs it: `go test -tags testtools -race -count=1 -timeout 30m ./fuku/modules`: `ok  	github.com/uasneppy/fuku_robot/fuku/modules	1521.494s`, exit 0 (run after both task commits, before this SUMMARY).
- `go vet -tags testtools ./fuku/modules`: exit 0. `gofmt -l` over the two Go files this plan created or edited: no output. `python3 .planning/phases/13-member-requirements/handoff/lint-local.py 8c0c827`: exit 0, `no dupl, gocyclo or godox findings on lines added since 8c0c827` (exit code read directly, written to a file). `git diff --exit-code 8c0c827 -- go.mod go.sum`: exit 0.

## Deviations from Plan

**1. "Each rule costs one refused call and one live check, then no call"** is tested as the latch behaves (plan 17-04): the first inline delete of any member rule costs one refused call and one live check of the bot, and the latch (`service` scope) then stops every member rule (commands, edits, the worker's rows) with no further call or check. The three rules do not each pay their own pair: they share the chat's latch. The subtest asserts the shared behaviour.

**2. The "excluded messages" fixtures do not load the MemberDeletion module on the lockdown or staff dispatchers.** A message an admin sent in a locked group is a real member message and would legitimately be queued; the bot's own posts never arrive as updates. Instead of loading the module there, a separate subtest delivers the bot's own messages as updates to the member environment (all three rules on) and proves they are ignored, and the Staff Group case of the end-to-end test proves a member's message in a Staff Group writes nothing.

**3. The grep gate of the plan is also a Go subtest** (a file scan with a floor on the number of files), so `make test` holds the structure.

No production code changed and no earlier plan's test was edited.

## Known Stubs

None.

## Requirements

Contributes to DEL-03, DEL-04, DEL-05, DEL-07, DEL-08 and PLAT-06; completes none (`requirements-completed: []`). `requirements.mark-complete` was not called: plan 17-12 ticks them after the phase gate.

## Not run

- `make test` and `make lint` (the sandbox cannot run `make lint`; `make test` was replaced by the CI-style whole-package run above plus the lint stand-in). gosec and the CI-only `inline` analyzer cannot run here; the new file has no string literal assigned to a token/secret-named identifier, no `math/rand`, no discarded error (the one `_ = tx.AddError(...)` in the failing gorm callback is the gorm idiom for a callback that records an error on the statement) and no `reflect.Ptr`.
- Nothing here runs on PostgreSQL: the `fuku/modules` harness runs on SQLite.

## Self-Check: PASSED

- `fuku/modules/member_deletion_e2e_test.go` exists; `command_replies_guard_test.go` and `AGENTS.md` carry the review.
- Commits `05e5bf5` and `f4c85df` are ancestors of HEAD.
