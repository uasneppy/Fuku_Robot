---
phase: 05-lockdown-alerts-and-response
plan: 06
subsystem: lockdown
tags: [lockdown, alerts, log-channel, delivery, retries, stale-claims, callbacks]
status: complete
requires:
  - phase: 05-lockdown-alerts-and-response
    provides: alert copy rows and the poster (05-01), Lift buttons and confirm cards (05-02..05-04)
provides:
  - log channel copy of the lockdown alert (admin category on), under the usual log header
  - Lift button working from a channel message
  - ReleaseStaleAlertClaims and its call from every worker cycle
affects: [05-07, 05-13]
tech-stack:
  added: []
  patterns:
    - "one destination resolver (actionlog.Destination) for both the decision and the send, so the category switch and the header live in one place"
    - "stale alert claims released at the top of every cycle like stale joiner claims, no attempt counted"
key-files:
  created:
    - alita/modules/lockdown_alert_delivery_test.go
  modified:
    - alita/modules/lockdown_alert.go
    - alita/modules/lockdown.go
    - alita/modules/lockdown_worker.go
    - alita/db/lockdown/alerts.go
    - alita/db/lockdown/alerts_test.go
    - AGENTS.md
key-decisions:
  - "The log channel is resolved again just before the send; a channel that no longer takes the admin category gets nothing, and the copy goes through the usual three tries and is then ended as failed (the lookup cannot tell a turned-off category from a failed read)."
  - "The lockdown| callback handler is registered with SetAllowChannel(true); without it gotgbot drops every press on a channel message."
requirements-completed: [LOCK-11, LOCK-06]
duration: 40m
completed: 2026-10-11
commits: 4
plan_head_before: 584bea112ee90c27aac4aff015db5d56ba02b0c3
plan_head_after: a9a30eea5f0e9d369b3ccc96eafbb3fb4946a5eb
actuals:
  tokens: 9000
  tasks: 2
  commits: 4
---

# Phase 5 Plan 06: Log channel copy and resilient alert delivery Summary

**The lockdown alert now also lands in the locked group's log channel (only when its admin category is on, under the usual group header, in the group's language) with a working Lift button, and every copy is retried, resumed after a crash and posted once across replicas.**

Note: `commits: 4` counts the two RED test commits and the two implementation commits made before this SUMMARY; the SUMMARY commit follows, so `plan_head_after` is the last code commit.

## What was built

- **Log destination (`lockdown_alert.go`)**: `lockdownAlertDestinations` adds a `log` copy row (dest chat = the channel ID, chat = the locked group) when `actionlog.Destination(chat, logchannels.CategoryAdmin)` is ok. `lockdownPostAlertSlot` resolves the destination again right before sending, prefixes the header it returns to the text, and stores the prefixed text as the copy's `body`, so the later "Lifted by" edit starts from exactly what was sent. A channel that is gone or no longer takes the admin category gets nothing: the copy is released with an attempt and ended as failed after the third try. The log copy is translated with the locked group's language (`lockdownSlotTranslator` already did this).
- **Lift from the log copy (`lockdown.go`)**: the `lockdown|` callback handler gets `SetAllowChannel(true)`. gotgbot's `CallbackQuery.CheckUpdate` drops a press on a channel message otherwise, so the button on the log copy was dead until this fix. All authority checks are unchanged (live `getChatMember` of the locked group; the pressed chat must be the copy's own destination).
- **Delivery (`alerts.go`, `lockdown_worker.go`)**: `lockdown.ReleaseStaleAlertClaims(before)` runs two conditional updates in one transaction on disjoint rows (posting with `claimed_at` before the cut-off to pending; posted, open, close-claimed before the cut-off loses the claim), counts no attempt and returns the number released. `lockdownReleaseStale` calls it with the same 2-minute cut-off as the joiner claims and logs the count at warn; each of the two releases logs its own failure and the next cycle retries.
- **AGENTS.md**: the alert bullet now describes both destinations, the re-check before sending, and the channel callback; a new bullet describes resumption (stale claims, the accepted duplicate, three tries, rate limits costing no attempt, settle-then-alert in one cycle).

## Tests

- `TestLockdownAlertDestinations`: both, category off, log only, neither (alert stamped, no rows), linked later, and category turned off between the decision and the post (nothing sent, copy ended as failed after three cycles).
- `TestLockdownAlertLiftFromLogChannel`: a press on the log copy from a `channel` chat: a plain member is refused, a live admin gets the card as a reply in the channel, Confirm restores once and posts the lift notice in the group, and the next cycle edits both copies (body plus footer, no keyboard) and closes both rows.
- `TestLockdownAlertDelivery`: fails three times (other destination not held back, no fourth send), rate limited (pending, 0 attempts, posted next cycle), stale claim (released, one copy), live claim kept, stale close claim (released and closed), concurrent cycles (one message per destination, two rows).
- `TestLockdownAlertAfterSettle`: a settled lockdown is alerted in the same cycle; a settled-away one posts nothing.
- `TestReleaseStaleAlertClaims` (repository): stale send and stale close released, live ones kept, attempts and detail unchanged, count returned, second call releases nothing.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] The Lift button on a channel message was never delivered to the handler**
- **Found during:** Task 1 (`TestLockdownAlertLiftFromLogChannel`)
- **Issue:** `handlers.NewCallback` ignores callbacks from channel messages unless `SetAllowChannel(true)`; the plan did not mention it, so the log copy's button would have been dead in production.
- **Fix:** registered the `lockdown|` handler with `SetAllowChannel(true)`; AGENTS.md notes it.
- **Files modified:** `alita/modules/lockdown.go`, `AGENTS.md`
- **Commit:** 6a43326

**2. [Rule 2 - Missing critical functionality] Re-check of the log destination before sending**
- **Found during:** Task 1
- **Issue:** the destination is decided once, but the admin category can be turned off before the worker posts; the plan's prohibition (never post to a channel whose admin category is off) needs the check at the send too.
- **Fix:** `lockdownPostAlertSlot` calls `actionlog.Destination` again for a log copy and refuses when it is not ok or names another channel (release with an attempt, ended as failed on the third). Covered by the extra "category turned off before the post" subtest.
- **Commit:** 6a43326

### Test-shape notes

- Task 2 RED: only "stale claim", "stale close claim" and the repository test failed at RED; "fails three times", "rate limited", "live claim kept", "concurrent cycles" and `TestLockdownAlertAfterSettle` already passed because plans 05-01 to 05-04 built the claim, retry, rate-limit and cycle-order behaviour. They stay as regression tests. Step 4 of the plan (confirm the poster's classification) needed no code change.
- `ReleaseStaleAlertClaims` releases posting copies of any lockdown state; a posting copy of a lifted lockdown goes back to pending and is simply never listed (the pending list covers active lockdowns only).

## Verification

- `go test -tags testtools -race -count=1 -run '^TestLockdownAlertDestinations$|^TestLockdownAlertLiftFromLogChannel$' ./alita/modules`: pass.
- `go test -tags testtools -race -count=1 -run '^TestLockdownAlert|^TestStaffLog' ./alita/modules`: pass.
- `go test -tags testtools -race -count=1 -run '^TestLockdownAlertDelivery$|^TestLockdownAlertAfterSettle$' ./alita/modules`: pass; the concurrent-cycles subtest also passed 5 runs in a row.
- `go test -tags testtools -race -count=1 -run '^TestReleaseStaleAlertClaims$|^TestBeginAlert$|^TestAlertSlots$|^TestAlertClose$|^TestReleaseStaleClaims$' -v ./alita/db/lockdown`: pass.
- `go test -tags testtools -race -count=1 -run 'Lockdown|Unlockdown' ./alita/modules ./alita/db/lockdown`: pass (47 s).
- `CGO_ENABLED=0 go build ./...`, `go vet -tags testtools ./alita/modules ./alita/db/lockdown`: clean. `gofmt -l` on the touched Go files prints nothing. `go.mod` and `go.sum` unchanged.
- `golangci-lint` was not run: the installed build targets Go 1.25 and cannot lint Go 1.26 code (go vet and gofmt used instead).
- Not run: the whole `./alita/modules` package under `-race`, any PostgreSQL check, and a manual press on a real channel copy. Tests use SQLite, miniredis and a fake Telegram client.

## Threat surface

No surface beyond the plan's threat model. T-05-17 is covered by "category off" and "category turned off before the post"; T-05-18 by "fails three times" and "rate limited"; T-05-19 by "concurrent cycles", "stale claim" and "stale close claim". Enabling channel callbacks is limited to the `lockdown|` namespace, whose handler already ties a press to the copy's own destination chat and judges the presser by a live check of the locked group.

## Known Stubs

None.

## Self-Check: PASSED

`alita/modules/lockdown_alert_delivery_test.go` exists; commits d917e4f, 6a43326, 4ba16ee and a9a30ee are on the branch.
