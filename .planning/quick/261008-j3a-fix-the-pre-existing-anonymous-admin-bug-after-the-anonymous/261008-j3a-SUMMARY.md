---
phase: quick-261008-j3a
plan: 01
subsystem: anonymous-admin
tags: [anon-admin, bans, chat_status, bugfix]
requires: []
provides:
  - anonymous-admin proof tap re-enters the command with a message update and its own chat
affects: [alita/modules/bot_updates.go, AGENTS.md]
key-files:
  created: [alita/modules/anon_admin_reentry_test.go]
  modified:
    - alita/modules/bot_updates.go
    - alita/modules/bot_updates_test.go
    - AGENTS.md
    - .planning/phases/04-manual-lockdown/deferred-items.md
status: complete
commits: 4
plan_head_before: dab6c0b426af8b4d3e5bc19f1a4b2a2d36a2e411
plan_head_after: 686f9604be97975575cd91e7301ffabad023bbcd
actuals:
  tasks: 2
  commits: 4
---

# Quick 261008-j3a: Anonymous-admin re-entry fix

`verifyAnonymousAdmin` now rebuilds the update as the cached command's message update with `EffectiveChat` set to that command's chat, so `/ban` and every other anonymous-capable command behind `RequireGroup` runs after the proof tap. It also refuses a tap whose callback `c`, proof-button chat and cached-command chat are not one chat.

## Commits

- f107a62 test(anon-admin): reproduce silent anonymous /ban after the proof tap (RED, failed as expected)
- 182176f fix(anon-admin): run re-entered commands in their own chat after the proof tap
- d26905b test(anon-admin): cover proof taps whose chats do not match (RED, both cases failed as expected)
- 686f960 fix(anon-admin): refuse a proof tap whose chats do not match

## Verification

- Targeted anon tests (TestAnonymousAdminBanAfterProof x3, TestVerifyAnonymousAdmin*, TestLockdownAnonymousAdminProof, TestUnapproveAllCallbackCancelInvalidAndUnavailableMessage): pass.
- `CGO_ENABLED=0 go build ./...`: OK.
- `make test`: passed (exit 0), first run, no flaky failure.
- `make lint`: could not run in this environment (golangci-lint was built with go1.25, config targets 1.26.0). `go vet -tags testtools ./alita/modules` is clean.
- Scope: changed files are only the plan's files_modified. No lockdown*.go, locales, chat_status or helpers source.

## Deviations from Plan

None in code. Environment note: the disk filled up mid-task (link failed with "no space left on device"); I deleted Go build-cache files older than 3 hours under /root/.cache/go-build to continue.

## Follow-up candidate (not fixed here)

The ban family sends the proof prompt followed by a `chat_status_restrict_cmd_error` reply before the tap, because the anonymous sender fails `CanUserRestrict`, which emits the proof and then a refusal. Pre-existing and out of scope.

## Known Stubs

None.

## Self-Check: PASSED
