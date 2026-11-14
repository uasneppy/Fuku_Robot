---
phase: 17-command-edit-and-self-destruct-deletion
plan: 07
subsystem: modules
tags: [member-deletion, self-destruct, pinned-messages, busy-group, staff-group, handler-group]

requires:
  - phase: 17-command-edit-and-self-destruct-deletion
    provides: plan 17-05 (the MemberDeletion module, memberDeletionExempt) and plan 17-06 (the self-destruct batcher, the 1000-row worker)
provides:
  - memberSelfDestructWatch, a handlers.NewMessage(message.All, ...) watcher at group -8 (no setter) that only offers to the batcher
  - memberContentMessage, memberSelfDestructDelay, memberDeletePinned and the /pin hook in pins.go
  - the toolkit newSelfDestructEnv, sendMember, sendMemberIn, sendPinOf, selfDestructRows, withRealDeletionPacer, paceKeyState, registerReplyCommand, repliesTo
affects: [17-10, 17-11, 17-12]

plan_head_before: 25fe2a6e23fe4b66425ccedad7634af103d1b091
plan_head_after: 6866241af5bc94d377aa7a5fa1d81687b70a5532

actuals:
  tokens: 14000   # chars/4 over the added lines of 25fe2a6..6866241 (55802 chars), tests and AGENTS.md included
  tasks: 3
  commits: 5      # MEASURED: git rev-list --count 25fe2a6..HEAD before this SUMMARY commit

tech-stack:
  added: []
  patterns:
    - "a watcher that only offers to a bounded batcher, with a named result preset to ext.ContinueGroups"
    - "a real gorm query callback that fails the staff_groups read, so every Staff Group read of the process fails for real in one test"
    - "the production deletion pacer options over miniredis, with measurable call counts instead of sleeps"

key-files:
  created:
    - fuku/modules/member_selfdestruct_test.go
    - fuku/modules/member_selfdestruct_busy_test.go
  modified:
    - fuku/modules/member_deletion.go
    - fuku/modules/pins.go
    - fuku/modules/member_deletion_env_test.go
    - AGENTS.md

key-decisions:
  - "The pinned_message check runs right after the chat-type test (a group or supergroup), before the sender checks, and drops the row of the message the service message names, with the chat taken from the service message."
  - "The bot's own /pin drops the row through one memberDeletePinned call inside pins.go's `if pin {` branch (deviation note 3): the bot's pin service message has a bot sender that a watcher with no SetAllowBot never sees."
  - "The watcher has no captcha lookup: group -10 ends a pending member's update, but only while the captcha is enabled for the chat, which the test sets."

requirements-completed: []

coverage:
  - id: D1
    description: "With member:selfdestruct stored as 600, every kind of member message (text, photo, sticker, poll, forward, reply, via_bot, command, each item of a two-photo album) writes no row before the flush, one pending row each at its date plus 600 s after it, and the real worker deletes them at plus 601 s and not at plus 599 s"
    requirement: "DEL-05 (contribution only)"
    verification:
      - kind: integration
        ref: "go test -tags testtools -race -count=1 -run '^TestMemberSelfDestructQueuesMessages$' -v ./fuku/modules"
        status: pass
    human_judgment: false
  - id: D2
    description: "Nothing is queued for administrators (cached, creator, cold cache), approved users, a failed approvals read, a live admin read answering 429 or 500, channels, anonymous admins, automatic forwards, service IDs, bots, new_chat_members and chat_title service messages, a private chat, a member held by the captcha, and a stored delay of 45; the control (a plain member behind a cold cache) is queued"
    requirement: "DEL-05 (contribution only)"
    verification:
      - kind: integration
        ref: "go test -tags testtools -race -count=1 -run '^TestMemberSelfDestructKeeps$' -v ./fuku/modules"
        status: pass
    human_judgment: false
  - id: D3
    description: "A pinned_message service message drops exactly the pending self-destruct row of the message it names in its own chat (another chat's row, another kind, an acting row and another message are left; an unpin queues nothing), and the bot's own /pin drops it through the pins.go hook"
    requirement: "DEL-05 (contribution only)"
    verification:
      - kind: integration
        ref: "go test -tags testtools -race -count=1 -run '^TestMemberSelfDestructPinnedStays$' -v ./fuku/modules"
        status: pass
    human_judgment: false
  - id: D4
    description: "Switching Self-destruct Off drops the chat's pending rows and leaves another chat's rows, an acting row and other kinds; a delay change and a switch on drop nothing and keep the queued times; the watcher never ends an update (sibling and later probes, also after a forced panic); memberContentMessage is false for every service, creation and migration field"
    requirement: "DEL-05 (contribution only)"
    verification:
      - kind: integration
        ref: "go test -tags testtools -race -count=1 -run '^(TestMemberSelfDestructOffCancels|TestMemberSelfDestructNeverEndsUpdate|TestMemberContentMessageExcludesServiceFields)$' -v ./fuku/modules"
        status: pass
    human_judgment: false
  - id: D5
    description: "A busy group keeps pace: 3000 messages are offered with no write, the flush writes all 3000 with zero sheds, and the real worker and 100 ms pacer delete them with exactly 30 deleteMessages calls of 100 IDs (at most 10 per cycle) in at least 2.85 s and under the cycle guard; a command in another group is answered under a blocked pacer and touches no pacer key; a 429 leaves every row pending with no attempt while a command is still answered; with the ceiling at 50, 80 messages leave 50 rows, 30 counted sheds, and the 30 are never deleted"
    requirement: "DEL-05, PLAT-06 (contributions only)"
    verification:
      - kind: integration
        ref: "go test -tags testtools -race -count=3 -timeout 20m -run '^TestSelfDestructBusyGroup$' ./fuku/modules"
        status: pass
    human_judgment: false
  - id: D6
    description: "In a Staff Group (and when the Staff Group read fails) a member's command, a late edit and a plain message produce no deleteMessage, no deleteMessages and no queue row after a flush and a worker cycle, while the control group deletes the command and the edit and queues the message"
    requirement: "DEL-07 (contribution only)"
    verification:
      - kind: integration
        ref: "go test -tags testtools -race -count=1 -run '^TestMemberDeletionStaffGroupNeverTouched$' -v ./fuku/modules"
        status: pass
    human_judgment: false

duration: 70min
completed: 2026-11-12
status: complete
---

# Phase 17 Plan 07: The self-destruct watcher Summary

With `member:selfdestruct` on, every message of a real member is offered to the batcher at group -8 and deleted at its own date plus the delay, while administrators, approved members, unreadable exemptions, service messages, held members, Staff Groups and pinned messages stay.

## What was built

- **Task 1 (tracer), `1454262` (red) and `e4cce42` (green)**: `memberSelfDestructWatch` in `fuku/modules/member_deletion.go` (a value-receiver method of `moduleStruct`, a named result preset to `ext.ContinueGroups`, `defer error_handling.RecoverFromPanic`), registered in `LoadMemberDeletion` with `handlers.NewMessage(message.All, ...)` at `memberDeleteGroup` and no setter, in the same commit as every exemption, the pin drop and the `/pin` hook. Order: group or supergroup; a `pinned_message` service message drops the named message's pending row (`memberDeletePinned`, the chat from the service message) and returns; sender shape through `raidFloodSenderCounts` and not a bot (a `via_bot` message counts); `memberContentMessage` (false for everything `deletionServiceKindOf` recognises and for the creation and migration fields); `memberSelfDestructDelay` (only the six presets count, else Off, an unreadable rule queues nothing); `memberDeletionExempt` (D-33, unknown is exempt); `memberDeleteOffer` with `SentAt` the message's own date. It never calls `deletionAct` or `deletionSchedule` (the file's code lines still name `deletionAct(` once, the edit handler's). `pins.go` gains one call, `memberDeletePinned(chat.Id, prevMessage.MessageId)`, inside the successful-pin branch. The red commit is a runtime failure (the flush wrote 0 rows), not a compile failure.
- **Task 2, `3846c4e`**: `TestMemberSelfDestructKeeps` (17 keep rows plus the control, including live reads answering 429 and 500, a failed approvals read and a held captcha member), `TestMemberSelfDestructPinnedStays` (exact drop; the bot's own `/pin` through the real `pins.go` handler), `TestMemberSelfDestructOffCancels`, `TestMemberSelfDestructNeverEndsUpdate` (sibling probe in group -8 plus a later probe, and a forced panic in the rule read), `TestMemberContentMessageExcludesServiceFields`. All passed against the Task 1 code; no `fix(17-07)` commit was needed.
- **Task 3, `eb61c23` (tests) and `6866241` (AGENTS.md)**: `TestSelfDestructBusyGroup` (burst, replies under a blocked pacer, a 429, the ceiling) over the real worker and the production 100 ms pacer, and `TestMemberDeletionStaffGroupNeverTouched` (a Staff Group, a failing read through both the `deletionStaffLookup` seam and a real failing gorm query callback on `staff_groups`, and an ordinary-group control). AGENTS.md: group -8 names both handlers, the `MemberDeletion` Handlers bullet describes the watcher, and a Subsystem traps bullet "Member deletion, self-destruct" states what counts, the exemptions, the pin drop and `/pin` hook, Off and delay changes, the accepted limits and the busy-group guarantees. `make generate-docs` wrote no page and `make check-docs` says "No drift detected".

## Verification results (real)

- Task 1: `go test -tags testtools -race -count=1 -run '^TestMemberSelfDestructQueuesMessages$' -v ./fuku/modules` all three subtests PASS (red before the watcher: `memberDeleteFlush() = 0, want 10 written`).
- Task 2 command `-run '^(TestMemberSelfDestruct.*|TestMemberContentMessageExcludesServiceFields|TestServiceKindsCoverMessageFields|TestPin.*)$' -v`: every test and subtest PASS, no SKIP (the pending-captcha row first failed because group -10 holds only while the captcha is enabled; the fixture now enables it; the pinned test's first version expected another chat's row to survive a pin in that same chat, a test mistake fixed in the test).
- Task 3: `-run '^(TestSelfDestructBusyGroup|TestMemberDeletionStaffGroupNeverTouched)$' -v` PASS; `-count=3 -run '^TestSelfDestructBusyGroup$'` printed `ok ... 21.365s`; `make generate-docs` then `make check-docs`: "No drift detected".
- Plan verification run `-run '^(TestMemberSelfDestruct|TestSelfDestructBusyGroup|TestMemberDeletion|TestMemberEdited|TestCommandDeletion|TestDeletionEndToEnd|TestPin)' ./fuku/modules`: `ok ... 52.173s`, exit 0. Also `TestLoadAllModulesUsesStablePriorityOrder`, `TestRegisteredModulesInstallGotgbotHandlerGroups`, `TestRegisterLegacyModule*`, `TestCommandRepliesStayInsideTheHandler`, `TestShutdownEndsMemberDeletionBatcher` and `TestDeletionWorker*`: `ok ... 18.563s`.
- Mutation proofs (each reverted; the files match their commits): bypassing `memberDeletionExempt` failed the administrator, approved and failed-read rows of `TestMemberSelfDestructKeeps`; turning "unknown is exempt" into "only an admin is exempt" failed the 429 and 500 rows; a `nil` watcher result failed both subtests of `TestMemberSelfDestructNeverEndsUpdate`; deleting the `/pin` hook failed `TestMemberSelfDestructPinnedStays/the_bot's_own_/pin_drops_the_row`.
- Acceptance greps: `memberSelfDestructWatch` in member_deletion.go 4; `memberDeletePinned` in pins.go 1; the two helper funcs 2; code lines naming `deletionAct(`, `deletionSchedule(` or `deletion.Schedule(` 1 (the edit handler's, limit 1); `429\|500` in member_selfdestruct_test.go 6; code lines naming `SetAllow` in member_deletion.go 0; `memberSelfDestructWatch\|self-destruct watcher` in AGENTS.md at least 1.
- `CGO_ENABLED=0 go build ./...` ok; `go vet -tags testtools ./fuku/modules` ok; `gofmt -l` on the five Go files this plan created or edited prints nothing; `python3 .planning/phases/13-member-requirements/handoff/lint-local.py 8c0c827` exit 0 (read directly): "no dupl, gocyclo or godox findings on lines added since 8c0c827"; `git diff --exit-code 8c0c827 -- go.mod go.sum` exit 0; no `reflect.Ptr` or `ioutil.` in the new files.
- Not run: `make lint` (cannot run here), `make test` and the whole `fuku/modules` package (not this plan's task), the settings test family (no settings code touched), gosec and CI's govet `inline` analyzer (cannot run here), PostgreSQL (no new SQL).

## Deviations from Plan

1. **The pinned check follows the chat-type test** instead of preceding it (the plan lists the pin first, "in a group or supergroup"); the two orders are equivalent and one test is cheaper.
2. **Failing Staff Group read through a real query callback.** The plan's `deletionStaffLookup` swap only reaches `deletionAct`; `deletion.ScheduleBatch` reads through an unexported seam of its own package. `TestMemberDeletionStaffGroupNeverTouched` therefore swaps the seam and also registers a gorm query callback that fails every `staff_groups` read, so all three rules meet a real failing read. No production hook was added.
3. **The pending-captcha test enables the captcha** (`captcha.SetCaptchaEnabled`), because group -10 lets every message through while it is disabled, even with a pending attempt row; AGENTS.md says "while the captcha is enabled".
4. **"Unpin queues nothing" is asserted as "no pin event requeues anything"** (a flush after the pins writes 0): Telegram sends no service message for an unpin, so there is nothing to simulate beyond that.
5. **The `ForwardOrigin` row of `TestMemberContentMessageExcludesServiceFields` is weak** (the reflection builder leaves an interface field nothing satisfies at its zero value); the forward is covered for real by the tracer's forward message.
6. Red commit is a runtime failure rather than a compile failure; no `fix(17-07)` commit was needed.

## Known limits (stated in AGENTS.md)

A pin that lands while the message is in the batcher's buffer (about 300 ms) or after the worker claimed its row still deletes it; a pin by another bot is never seen (a live check for `17-SPIKE.md`); exemptions are judged at send time; a command deleted by command deletion leaves a self-destruct row whose later delete is refused quietly.

## Requirements

`requirements-completed: []`. DEL-05, DEL-07 and PLAT-06 are contributed to; plan 17-12 completes DEL-05 after the end-to-end test with the page. `requirements.mark-complete` was not called.

## Known Stubs

None. `memberDeleteOffer`, the 17-06 function with tests as its only caller, now has its production caller.

## Threat Flags

None. No new endpoint, Redis key, schema or drain; the watcher writes nothing itself and the pin drop is one `DropPendingMessage` matching chat, message, kind and pending state (T-17-30); T-17-28 to T-17-32 are mitigated as the plan's register says.

## Self-Check: PASSED

- Created files `fuku/modules/member_selfdestruct_test.go` and `fuku/modules/member_selfdestruct_busy_test.go`: FOUND.
- Commits 1454262, e4cce42, 3846c4e, eb61c23, 6866241: all reachable from HEAD.
