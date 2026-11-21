---
phase: 21-pm-copies
plan: 03
subsystem: event-messages
status: complete
tags: [pm-copies, pacer, job-pool, rate-cap, guard]
requires: [22-02]
provides:
  - pmCopyOutcome (delivered, blocked, refused, busy, unknown) with String; pmCopyClassify; pmCopyErrCode
  - eventPMOffer.Custom and eventPMOffer.Fallback (built in offerReplyPM); pmCopySend
  - pmCopySkipTarget (offerReplyPM before the render, offerEventPM again)
  - pmCopyAutoCapPerMinute (10), pmCopyCapCounterTTL, pmCopyCapKey, pmCopyCapAllow, pmCopyNow, pmCopyRedisClient
  - TestEventPMFailures, TestEventPMLimits, TestEventPMPacerIndependent, TestEventPMLogsCarryIDsOnly, TestPMCopyCallers
  - toolkit: eventPMScript, eventPMRefuse, eventPMHold, withTestEventPM
affects: [22-04, 22-05, 22-06, 22-07, 22-09, 22, 23]
tech-stack:
  added: []
  patterns:
    - "classify a send by Telegram's code and sentinel errors only, log the class and the code, never the error's text"
    - "a fail-closed per-minute counter (INCR plus EXPIRE in one transaction) checked as a job's first step"
    - "a go/parser guard that lists the users of an engine with reasons and forbids a file-name prefix from naming it"
key-files:
  created:
    - fuku/modules/event_message_pm_failures_test.go
    - fuku/modules/pm_copy_callers_test.go
  modified:
    - fuku/modules/event_message_pm.go
    - fuku/modules/event_message.go
    - fuku/modules/event_message_pm_env_test.go
    - AGENTS.md
decisions:
  - "Log lines carry the class word and the Telegram code alone (pmCopyErrCode, 0 for none); memberReqErrText is no longer used by the private copy job, because for a non-Telegram error it prints err.Error()."
  - "pmCopySkipTarget runs twice: in offerReplyPM before composing (saves the render) and in offerEventPM (the engine's own guarantee for any caller)."
  - "The automatic flag travels on the offer as an unexported field set by offerEventPM from the spec, so the job never reads eventPMSpecs (a test may swap that table while jobs run)."
  - "pmCopyCapAllow returns a bool as the plan specified; the caller logs one Debug line that covers both 'over the cap' and 'the cap could not be counted'."
  - "Fallback is built only when Custom is true (a stored text, media or buttons), so a default copy never computes it twice."
metrics:
  duration: ~45 min
  completed: 2026-11-19
actuals:
  tokens: 10450
  tasks: 3
  commits: 3
plan_head_before: 89c979ed806f1350018db9be94a10ef83e0c004b
plan_head_after: d5c97ac4768cbd6f91decf5280ea6f8a9d820dc5
---

# Phase 22 Plan 03: Private copy failure rules and limits Summary

A private copy's send is now classified by Telegram's code (`pmCopyClassify`: 403 blocked, other 4xx and
`media.ErrNoPermission` refused, 429 and `ratelimit.ErrRateLimited` busy, anything else unknown), a refused custom copy is
replaced once by the event's default private text, an impossible target (non-positive, a service identity, the bot) is never
tried, automatic events are capped at 10 copies a group a minute on `fuku:pmcopy:{<chat>}:cap:<minute>` (fail closed), and
`TestPMCopyCallers` keeps every staff, lockdown and raid file away from the engine. The punishment and the group reply are
unchanged (`TestLegacyModerationRequests` untouched and green).

## What later plans use

| Kind | Name |
|------|------|
| Outcome | `pmCopyOutcome` (`pmCopyDelivered`, `pmCopyBlocked`, `pmCopyRefused`, `pmCopyBusy`, `pmCopyUnknown`), `String()`, `pmCopyClassify(err)`, `pmCopyErrCode(err)` |
| Send | `pmCopySend(ctx, o, msg) (pmCopyOutcome, int)`: one paced `media.SendGreetingMessage` to `o.UserID`, its class and code |
| Offer | `eventPMOffer.Custom`, `eventPMOffer.Fallback` (set by `offerReplyPM`), unexported `automatic` (set by `offerEventPM`) |
| Job | `runEventPM`: cap (automatic only), send, the fallback for a refused custom copy, one final log line. The final outcome is a local variable there: plan 22-06 (Details) and 22-07 (log line) hook "not delivered" (any final outcome but delivered of a job that sent) at the end of `runEventPM` |
| Targets | `pmCopySkipTarget(b, userID)` |
| Cap | `pmCopyAutoCapPerMinute`, `pmCopyCapCounterTTL` (120 s), `pmCopyCapKey`, `pmCopyCapAllow(ctx, chatID) bool`, `pmCopyNow`, `pmCopyRedisClient` |
| Toolkit | `eventPMTestClient.script` (`*eventPMScript`: `refusals`, `hold`, `memberName`), `eventPMScriptOf`, `eventPMRefuse(t, s, err)`, `eventPMHold(t, s) release`, `withTestEventPM(t, event, spec)`; in the failures file `eventPMTelegramError`, `eventPMBanCommand`, `eventPMOfferTo`, `eventPMAllCalls`, `eventPMSentTo`, `eventPMUsers`, `eventPMWantLine`, `eventPMWaitBeforeRestore` |

## Tasks

| Task | Name | Commit |
|------|------|--------|
| 1 | classify every outcome, fall back to the default once, never send to an impossible target (`TestEventPMFailures`, AGENTS.md) | d8edeef |
| 2 | the full pool, the automatic cap, the independent pacers and the log hygiene (three tests, AGENTS.md PM copies and Data bullets) | 488682b |
| 3 | the structural caller guard `TestPMCopyCallers` and AGENTS.md (the guard, the Phase 22 test families) | d5c97ac |

## Verification

- `go test -tags testtools -race -count=1 -run '^(TestEventPMFailures|TestEventPMEndToEnd|TestEventPMComposition|TestLegacyModerationRequests)$' -v ./fuku/modules`: PASS (task 1 gate; 8 `TestEventPMFailures` subtests, no SKIP).
- `go test -tags testtools -race -count=1 -run '^(TestEventPMLimits|TestEventPMPacerIndependent|TestEventPMLogsCarryIDsOnly|TestEventPMFailures)$' -v ./fuku/modules`: PASS (task 2 gate).
- `go test -tags testtools -race -count=1 -run '^(TestPMCopyCallers|TestCommandRepliesStayInsideTheHandler)$' -v ./fuku/modules`: PASS (task 3 gate).
- `go test -tags testtools -race -count=1 -run 'TestEventPM.*|TestPMCopy.*|TestEventMessage.*|TestLegacyModerationRequests|TestMessagesEndToEnd|TestCommandRepliesStayInsideTheHandler' -v ./fuku/modules`: ok (33 s), 20 top-level tests, no FAIL, SKIP or race.
- golangci-lint v2.13.1, `CGO_ENABLED=0`: `run --new-from-rev=9940d97 ./...` 0 issues; with `--build-tags=notesttools` 0 issues (one staticcheck QF1002, a tagged switch, fixed before the first commit).
- `CGO_ENABLED=0 go build ./...` ok; `gofmt -l` empty on every touched Go file; `make check-translations` all present; `make check-docs` no drift.
- Acceptance greps: `func pmCopyClassify(`, `func pmCopySkipTarget(`, `func pmCopyCapAllow(` once each in `event_message_pm.go`; `fuku:pmcopy:{` in `event_message_pm.go` and twice in AGENTS.md; `TestPMCopyCallers` twice in AGENTS.md; each of the three task-2 test functions once in the failures file.
- `git diff --exit-code` of `moderation_legacy_request_test.go` since its own commit, and `git diff --exit-code 9940d97 -- go.mod go.sum main.go`: no change.
- Mutations (all reverted) each failed the test that guards them: no fallback (`refused custom`); `pmCopySkipTarget` reduced to `userID < -100` (`impossible targets`); the cap allowing with no client (`fail closed without Redis`); 403 no longer blocked (the classification table, `never started`, `blocked`); and the plan's mutation proof, a temporary `offerEventPM(eventPMOffer{})` call appended to `staff_log.go`, failed `TestPMCopyCallers` with both the forbidden-prefix and the allow-list error (removed before any commit).

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug, test] `callsFor("")` matches no request**
- **Found during:** Task 2 (the first run of `TestEventPMLimits` counted 0 copies everywhere).
- **Issue:** `moduleBotClient.callsFor(method)` filters by an exact method, so `callsFor("")` is always empty; the first draft of `impossible targets` passed vacuously through it.
- **Fix:** `eventPMAllCalls(s)` reads every recorded request under the fake's lock; both the count helper and `impossible targets` use it (Task 1's commit already carries the fixed test). AGENTS.md's Phase 22 hygiene line says so.

### Other deviations

**2. [Test] The 429 uses `retry_after` 4000, not 30**
- The test pacer (`withFastPMCopyPacer`) has a 5 ms retry unit, so 30 would be a 150 ms wait and the pacer would retry; 4000 units is 20 s, over the pacer's 15 s MaxWait, which is the "not retried" case the plan describes.

**3. [Logging] No `memberReqErrText` in the job any more**
- It prints `err.Error()` for a non-Telegram error, which is not "the code only". The job logs `pmCopyOutcome.String()` and `pmCopyErrCode` (0 for none).

**4. [Scope] Fail-closed Redis error via a second miniredis**
- "A miniredis that answers errors" is a separate `miniredis.RunT` with `SetError`, installed only through `pmCopyRedisClient`, so the pacer keeps the site's healthy miniredis; the manual copy of the same state is offered directly rather than through `/ban` (the cached event read would also hit the broken store).

**5. [Commits] Task 1 and Task 2 were written together and split for the commits**
- The production file and the toolkit were written in one pass; Task 1's commit was built from a reduced copy (no cap, no hold, no `withTestEventPM`) that passed its own gate and lint before it was committed, then the full files were restored for Task 2. The task-3 commit carries the plan's subject `feat(22-03): keep private copies best effort, paced and capped`.

## Not verified

- The whole `fuku/modules` package was not run (targeted runs only, as instructed); `make test-shard-check` was not run (the new top-level tests are found by `go test -list` like any other).
- `make lint` itself was not run; the CI-equivalent `golangci-lint run --new-from-rev=9940d97` was, with and without `notesttools`.
- No PostgreSQL run: nothing of this plan rests on a constraint (the cap is Redis only).
- Live Telegram: the exact code of "bot can't initiate conversation" and "bot was blocked by the user" (403 in the Bot API documentation and in practice) is assumed, not observed in this run.

## Notes for plan 22-04 (the page)

- Nothing of this plan touches the page: the switch is still the pm row's `silent` (D-08), and turning a copy on is `eventmsg.SetSilent(chat, event, pm, false, by)`, exactly as `storeEventPM` does.
- The page's previews (D-27) must go through `composeEventPM` and send only to the presser; they must not call `offerEventPM` (that would put a preview into the job pool, the cap and, later, the Details and log paths). `TestPMCopyCallers` allows `offerEventPM` only from `event_message.go`, so a settings file that calls it fails the guard by design.
- The guard's forbidden prefixes match the base name (`staff`, `lockdown`, `raid`), so the Messages page files (`settings_messages*.go`) may read `eventPMSpecFor` and `eventPMSpecs` freely.
- For plans 22-06 and 22-07: hook the Details record and the log line where `runEventPM` logs a final outcome other than delivered; a shed copy (`offerEventPM`'s pool) and a capped one (the cap's early return) must stay silent there (D-18). `eventPMOffer` will need the group message's ID and keyboard for the Details edit; `replyEventGroup` already returns the posted message.

## Known Stubs

None.

## Threat Flags

None: no surface beyond the plan's threat model. T-22-10 (`TestPMCopyCallers`, `TestEventPMLimits` cap and fail closed), T-22-11 (`TestEventPMPacerIndependent`, `TestEventPMLimits` full pool), T-22-12 (`TestEventPMFailures`: one request on a 403, a 429 and an unknown error), T-22-13 (`TestEventPMLogsCarryIDsOnly`) and T-22-14 (`TestEventPMFailures` "impossible targets") are mitigated and tested.

## Self-Check: PASSED

- Files: the 2 created files exist; the 4 modified files carry their edits.
- Commits: d8edeef, 488682b and d5c97ac are ancestors of HEAD.
