---
phase: 17-command-edit-and-self-destruct-deletion
plan: 03
subsystem: modules
tags: [command-deletion, processor-wrapper, gotgbot, tracer-slice, deletion-core]

requires:
  - phase: 17-command-edit-and-self-destruct-deletion
    provides: plan 17-01 (member kinds) and plan 17-02 (helpers.NewBotCommand, helpers.IsBotCommand)
provides:
  - modules.CommandDeletionProcessor, wired in main.go around tracing.TracingProcessor
  - botCommandName, a mirror of gotgbot's command matcher proven against the library's own CheckUpdate
  - the member:commands rule working end to end (stored 0 = on): a handled command is answered, then deleted through deletionAct with delay 0
affects: [17-04, 17-12]

plan_head_before: 5dfe53c26533632f6c155bc25e2500b272fe3d30
plan_head_after: 38ec5f654691c8d04e1f7cecbe68bcfdf9ef76b7

actuals:
  tokens: 7250    # chars/4 over the added lines of 5dfe53c..HEAD (29011 chars), tests and AGENTS.md included
  tasks: 3
  commits: 4      # MEASURED: git rev-list --count 5dfe53c..HEAD (before this SUMMARY commit)

tech-stack:
  added: []
  patterns:
    - "an ext.Processor wrapper outside tracing that decides after Inner returned, on a fresh context, with no defer around Inner"
    - "differential test against the library's own handler as the reference for a mirrored predicate"

key-files:
  created:
    - fuku/modules/command_deletion.go
    - fuku/modules/command_deletion_env_test.go
    - fuku/modules/command_deletion_test.go
  modified:
    - main.go
    - main_test.go
    - AGENTS.md

key-decisions:
  - "The post-step runs whether or not Inner returned an error: a handler error does not change that the bot handled (or refused) the command; refused and failed commands are plan 17-04's."
  - "A rich message whose extracted name has upper case is refused: the library compares it with a lower-case registered name without lower-casing, so no handler matches it."
  - "Only a stored value the kind accepts (ValidateRuleDelay: 0) counts as On; a hand-written 60 is Off (C-05)."

requirements-completed: []

coverage:
  - id: D1
    description: "With member:commands stored as 0, an admin's wrapped /ban is answered and then deleted through the real dispatcher: the reply request precedes the delete request in the fake's ordered call log, the reply stays, no queue row, under BaseProcessor and under tracing.TracingProcessor delivered polling-style"
    requirement: "DEL-03 (contribution only)"
    verification:
      - kind: integration
        ref: "go test -tags testtools -race -count=1 -run '^TestCommandDeletionAfterAnswer$' -v ./fuku/modules"
        status: pass
    human_judgment: false
  - id: D2
    description: "main.go installs CommandDeletionProcessor{Inner: tracing.TracingProcessor{}}"
    requirement: "DEL-03 (contribution only)"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^TestNewDispatcherWrapsCommandDeletion$' -v ."
        status: pass
    human_judgment: false
  - id: D3
    description: "Raw commands (bare, upper case, @thisbot, caption, no entities) are answered then deleted; another bot's command, an unregistered word, a mention-first entity, plain text, an edit, a private chat, a bot, a sender_chat, an anonymous admin, a service account, no rule row and a stored 60 keep the message with no queue row"
    requirement: "DEL-03 (contribution only)"
    verification:
      - kind: integration
        ref: "go test -tags testtools -race -count=1 -run '^(TestCommandDeletionRawCommand|TestCommandDeletionKeeps)$' -v ./fuku/modules"
        status: pass
    human_judgment: false
  - id: D4
    description: "botCommandName accepts a message for a name exactly when gotgbot's Command.CheckUpdate does (31 messages incl. caption, entity rules, @bot case, unicode, rich messages)"
    requirement: "DEL-03 (contribution only)"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^TestBotCommandNameMatchesGotgbot$' -v ./fuku/modules"
        status: pass
    human_judgment: false
  - id: D5
    description: "The delete survives the context TracingProcessor cancels on return (polling-style, nil data) for a wrapped and a raw command"
    requirement: "DEL-03 (contribution only)"
    verification:
      - kind: integration
        ref: "go test -tags testtools -race -count=1 -run '^TestCommandDeletionPollingContext$' -v ./fuku/modules"
        status: pass
    human_judgment: false

duration: 35min
completed: 2026-11-12
status: complete
---

# Phase 17 Plan 03: Command deletion tracer slice Summary

`modules.CommandDeletionProcessor` wraps `tracing.TracingProcessor` in `newConfiguredDispatcher` and, once every handler group has run, deletes a member's handled command through `deletionAct` (delay 0) when the chat stores `member:commands` as 0; the answer stays.

## What was built

- `fuku/modules/command_deletion.go`: `CommandDeletionProcessor{Inner}` (value receiver; `Inner.ProcessUpdate` first, no defer around it, then `commandDeletionAfter`); `commandDeletionAfter` (own `RecoverFromPanic`; order: message present, negative chat ID, group or supergroup, text or caption starts with `/` or a rich message, `botCommandName`, `helpers.IsBotCommand`, sender from `ctx.EffectiveSender` only (user, no `Chat`, not a bot, `msg.SenderChat` nil, not `staffServiceUserIDs`, not the bot), the rule through `deletionRulesLookup` on a fresh `deletionRecordTimeout` context accepted by `ValidateRuleDelay`, then `deletionAct`); `botCommandName` mirroring gotgbot's `checkMessage` for the `/` trigger (rich branch included).
- `main.go`: one line, `Processor: modules.CommandDeletionProcessor{Inner: tracing.TracingProcessor{}}`; `main.go` was not reformatted.
- Tests: toolkit `command_deletion_env_test.go` (`useCommandDeletionProcessor`, `newCommandDeletionEnv` on `newDeletionE2E`, rule stored through `applyDeleteRule(..., deletionSet(0))`, ordered call-log helpers over the fake), `TestCommandDeletionAfterAnswer` (both processors), `TestCommandDeletionRawCommand` (6 forms), `TestCommandDeletionKeeps` (11 rows), `TestCommandDeletionPollingContext`, `TestBotCommandNameMatchesGotgbot` (31 messages x 8 names), and `TestNewDispatcherWrapsCommandDeletion` in `main_test.go`.
- AGENTS.md: a Handlers bullet "Command deletion (`command_deletion.go`, Phase 17, DEL-03)".

## Verification results (real)

- `go test -tags testtools -race -count=1 -run '^(TestCommandDeletionAfterAnswer|TestCommandDeletionRawCommand|TestCommandDeletionKeeps|TestBotCommandNameMatchesGotgbot)$' -v ./fuku/modules`: all four PASS (subtests: 2 + 6 + 11 + 31; 55 PASS lines for the family incl. the polling test), no SKIP.
- `go test ... -run '^(TestNewDispatcherWrapsCommandDeletion|TestNewDispatcherHandlesExpectedAndWrappedErrors)$' -v .`: both PASS.
- `go test ... -run '^(TestCommandDeletionPollingContext|TestCommandDeletionAfterAnswer)$' -v ./fuku/modules`: PASS.
- Phase 16 slice untouched: `-run '^(TestDeletionEndToEnd|TestAnonymousAdminBanAfterProof|TestNoRawNewCommand)$' ./fuku/modules ./fuku/utils/helpers`: both `ok`.
- Mutation proofs (each reverted afterwards): deleting before `Inner` made `TestCommandDeletionAfterAnswer` fail with "the reply request is at 6 and the delete at 0" under both processors; removing the sender check failed 4 keep rows (bot, sender_chat, anonymous admin, service account); accepting any stored value failed the "stored 60" row; reading the rule through `tracing.UpdateContext` failed `TestCommandDeletionPollingContext` (0 deletes, so the fresh context is load-bearing).
- Acceptance greps: `grep -c 'CommandDeletionProcessor{Inner: tracing.TracingProcessor{}}' main.go` = 1; code lines naming `tracing.UpdateContext` in `command_deletion.go` = 0; `deletionAct(` count = 1; `IsBotCommand` count = 2; the three named test funcs = 3; `CommandDeletionProcessor` in AGENTS.md >= 1.
- `CGO_ENABLED=0 go build ./...` ok; `go vet -tags testtools . ./fuku/modules` ok; `gofmt -l` on the three new files and `main_test.go` prints nothing (`main.go` left out as the plan says).
- `make generate-docs` changed nothing; `make check-docs`: "No drift detected".
- `python3 .planning/phases/13-member-requirements/handoff/lint-local.py 8c0c827`: first run exit 1 with 3 staticcheck QF "could remove embedded field" findings, fixed in the last commit; rerun exit 0 ("no dupl, gocyclo or godox findings on lines added since 8c0c827"), exit code read directly.
- `git diff --exit-code 8c0c827 -- go.mod go.sum` exit 0. No `reflect.Ptr` or `ioutil.` in the new files.
- Not run: `make lint` (cannot run here, Go 1.25 golangci-lint), `make test` and the whole `fuku/modules` package (not this plan's task), the settings test family (this plan edits no settings code), the CI-only govet `inline` analyzer, `node --test` (no page change).

## Deviations from Plan

- **No separate red commits.** The plan asks for tests committed red before the production file. I wrote the production file first and committed per task with the tests alongside: `2a41172` feat (tracer: wrapper, wiring, tracer test), `84c7411` test (Task 2's tests; the production file already satisfied them, no mirror correction was needed), `011c029` docs (polling test and AGENTS.md), `38ec5f6` chore (lint stand-in QF fixes). The tests were checked to bite by the mutations above instead.
- **[Rule 3 - lint] Embedded-selector cleanups** (`ctx.Update.Message` to `ctx.Message`, `b.User.Username` to `b.Username`): behavior identical, found by the stand-in's quickfix step.
- The tracer test also runs with `ext.BaseProcessor` as `Inner` (plan: tracing only for the polling test); the ordering is proved under both.

## Requirements

DEL-03 is contributed to and stays Pending: `requirements-completed: []`. `requirements.mark-complete` was not run; plan 17-04 covers the anonymous-admin re-entry, refused and failed commands, the Staff Group and the delete-right latch, and plan 17-12 completes the requirement.

## Known Stubs

None.

## Threat Flags

None. T-17-09 to T-17-12 mitigated as the plan states (matcher mirror plus registry, `EffectiveSender` checks, `deletionAct` plus fresh bounded context, delete after Inner); the bare-name race with another bot (A17-1) stays accepted and goes to the phase spike file.

## Self-Check: PASSED

- FOUND: fuku/modules/command_deletion.go, fuku/modules/command_deletion_env_test.go, fuku/modules/command_deletion_test.go
- FOUND commits: 2a41172, 84c7411, 011c029, 38ec5f6 (all ancestors of HEAD)
