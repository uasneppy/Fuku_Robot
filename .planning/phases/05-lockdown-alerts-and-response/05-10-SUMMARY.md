---
phase: 05-lockdown-alerts-and-response
plan: 10
subsystem: lockdown
tags: [lockdown, ban-run, alert-button, approvals, worker, i18n]
status: complete
requires:
  - phase: 05-lockdown-alerts-and-response
    provides: lockdownResponseBanOne, ban run repository, burst rows (05-08); alert keyboard, callback dispatcher and Lift card (05-09 and earlier)
provides:
  - approvals.ListApprovedIDsFresh, an uncached read of a chat's approved users that returns its error
  - the "Ban N recent joiners" button on the lockdown alert, its confirm card and the Confirm that starts a run
  - lockdownProcessBanRuns, the worker step that executes running ban runs, and lockdownBanRunResult, the card's banned/skipped/failed report
affects: [05-11, 05-13]
tech-stack:
  added: []
  patterns:
    - "a safety list read through a seam that returns its error, so a failed read refuses the ask, the confirm and a running run instead of degrading to an empty list"
    - "the card's chat and message are stored with the run in the same insert, so a worker that picks the run up at once already knows where to report"
key-files:
  created:
    - alita/modules/lockdown_ban_test.go
  modified:
    - alita/db/approvals/repository.go
    - alita/db/approvals/repository_test.go
    - alita/modules/lockdown_response.go
    - alita/modules/lockdown_alert.go
    - alita/modules/lockdown_alert_callbacks.go
    - alita/modules/lockdown_worker.go
    - alita/modules/lockdown_alert_lift_test.go
    - alita/modules/lockdown_fake_test.go
    - alita/modules/test_harness_test.go
    - AGENTS.md
    - locales/en.yml
    - locales/es.yml
    - locales/fr.yml
    - locales/hi.yml
    - locales/id.yml
    - locales/pt.yml
    - locales/ru.yml
key-decisions:
  - "lockdownAlertKeyboard takes the bot as a first argument (it needs the bot's ID to leave it out of N); it has no other caller."
  - "Confirm stores CardChatID/CardMsgID on the run before StartBanRun instead of calling SetBanRunCard afterwards, so a run picked up by the worker between the insert and a second write cannot finish with no card to report on."
  - "A run waits (returns no progress) while the approved users cannot be read; it is not stopped, and the next 2 s tick reads again."
  - "A run of a lockdown that is no longer active is left untouched (plan 05-11 stops and reports it)."
requirements-completed: [LOCK-12, LOCK-11]
duration: 70min
completed: 2026-10-11
commits: 3
plan_head_before: a62fa0c8fe75553831f3876d674b32e3ffd26f92
plan_head_after: edbc7b861dacbb627a6fbcea477ef468a5cca6f6
actuals:
  tokens: 32000
  tasks: 2
  commits: 3
---

# Phase 5 Plan 10: Ban N recent joiners button, card and worker run Summary

**A live admin of the locked group presses "Ban N recent joiners" on the alert, confirms a card only they can confirm, and the lockdown worker bans the burst for good (no end date) through the 05-08 per-account core, skipping admins, approved users, people an admin let in and the bot; the card is then edited with how many were banned, skipped and failed. Approved users are read fresh and a failed read bans nobody.**

Note: `commits: 3` counts the RED test commit, the implementation commit and the Task 2 test commit made before this SUMMARY; the SUMMARY commit follows, so `plan_head_after` is the last code commit.

## What was built

- **`approvals.ListApprovedIDsFresh(chatID)`** (`alita/db/approvals/repository.go`): a direct `db.DB` pluck of the chat's approved user IDs into a set, error wrapped and returned, never touching the cache (so no `DeleteCache` applies).
- **Alert keyboard** (`lockdown_alert.go`): `lockdownAlertKeyboard(b, tr, slot, ld)` now builds the Lift row and a second row with the Ban button (`a=bn&r=<copy>`). The label carries N from `CountBanCandidatesFresh` (approved users and the bot excluded, exempt joiners excluded by the query) or the generic `lockdown_alert_btn_ban_any` when N is 0 or the approvals/count cannot be read. A button that does not encode is left out and logged.
- **Ask and Confirm** (`lockdown_alert_callbacks.go`): `lockdownBanAsk` requires `lockdownButtonAuthority` (live, restrict right), reads the approved users fresh (error: `lockdown_ban_check_failed`, no card), counts (0: `lockdown_ban_nothing`) and posts the card as a reply to the pressed message with Confirm `a=bc&r&u&e` and the shared Cancel. `lockdownBanConfirm` checks asker, expiry, lockdown still active, authority again and the approvals again, then `lockdown.StartBanRun` with the presser, the copy's `Dest` as `card_dest`, the pressed card's chat and message, and approved users plus the bot as `exclude`. Refused start: `lockdown_ban_busy`; queued 0: `lockdown_ban_nothing`; error: `lockdown_ban_start_failed`; otherwise the card is edited to `lockdown_ban_running` with the queued count and the worker is woken.
- **Worker step** (`lockdown_response.go`, `lockdown_worker.go`): `runLockdownCycle` runs `lockdownProcessBanRuns` after `lockdownBanPending`. Per running run of an active lockdown: approved users fresh (error: log at warn, skip the run this cycle), `ListQueuedBurstFresh` in batches of `lockdownWorkerBatch`, `ClaimBurst` then `lockdownResponseBanOne`, then `FinishBanRun(done)`; the winner calls `lockdownBanRunResult`, one paced edit of the card with banned, skipped and failed in the card's translator (staff card: Staff Group language; log card: the locked group's). `lockdownReleaseStale` also calls `ReleaseStaleBurstClaims` with the same cut-off.
- **Locale keys** in all 7 files: the plan's eight (`lockdown_alert_btn_ban`, `lockdown_alert_btn_ban_any`, `lockdown_ban_ask`, `lockdown_ban_running`, `lockdown_ban_result`, `lockdown_ban_nothing`, `lockdown_ban_busy`, `lockdown_ban_check_failed`) plus `lockdown_ban_start_failed`. The ask text says only that the accounts are banned for good and can't rejoin and that I can't undo it.
- **AGENTS.md**: the `lockdown|v1` Callbacks bullet gains `bn`/`bc`; a new Subsystem-traps bullet states the fresh approvals read and the fail-closed rule, the worker-executed run, the winner of `FinishBanRun` editing the card, no audit/undo, and what the texts claim.
- **Test harness**: `&models.ChatLockdownBanRun{}` in the module AutoMigrate list (as 05-08 asked) and `lockdownCleanup` deletes ban-run rows.

## Tests

- `TestLockdownBanRecentJoiners` ("burst": a 3-row pre-lockdown burst with a member, an approved user and an admin plus a user, a bot account and an admin-added joiner after the lock; Ban label with 4, a non-admin refused with no card and no run, card with 4 as a reply to the alert, another member's Confirm refused, Confirm starts one running run, the cycles ban P1, J1 and J2 with no `until_date`, never touch the approved user, the admin or the let-in joiner (no `getChatMember` for the let-in one), burst and joiner row states, card edited to banned 3, skipped 2, failed 0; "request": a declined join request is banned with reason `request`).
- `TestLockdownBanCount` ("excluded from N", "approved read fails at Ask", "approved read fails at Confirm", "approved read fails during the run" (no `getChatMember`/`banChatMember`, rows stay queued, run stays running, and the next cycle after the read recovers bans them), "nothing left", "already running").
- `TestListApprovedIDsFresh`: one chat only, empty map for a chat with none, sees a row written straight through `db.DB`.
- Mutation check: removing the early return on a failed approvals read in `lockdownProcessBanRun` makes "approved read fails during the run" fail (accounts banned, run done).

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] The existing `TestLockdownAlertLift` pinned a one-button alert keyboard**
- **Found during:** Task 1 (the plan requires a second row)
- **Fix:** the assertion now expects the Lift row and a second row with the Ban button.
- **Files modified:** `alita/modules/lockdown_alert_lift_test.go`
- **Commit:** 093cb4c

**2. [Rule 2 - Missing critical functionality] Extra locale key `lockdown_ban_start_failed`**
- **Issue:** a `StartBanRun` error had no honest text: `lockdown_ban_check_failed` says the approved users could not be read, and `lockdown_alert_*` texts talk about rights or the lift.
- **Fix:** one extra key in all 7 files; the card says nothing was banned and to try again.
- **Commit:** 093cb4c

**3. [Rule 1 - Race] Card stored with the run, not by `SetBanRunCard` afterwards**
- **Issue:** the plan calls `SetBanRunCard` after `StartBanRun`; a worker tick between the two would finish a short run with no card to report on, and the result would be lost.
- **Fix:** Confirm sets `CardChatID`/`CardMsgID` on the run struct before `StartBanRun`, which inserts them with the row. `SetBanRunCard` stays in the repository, unused by the module.
- **Commit:** 093cb4c

**4. `lockdownAlertKeyboard` signature gained the bot** (`b *gotgbot.Bot` first), needed to leave the bot's ID out of N; it had no other caller.

### Process notes

- Task 2's cases all passed against the Task 1 implementation (it already used one exclusion list everywhere and fails closed), so there was no separate `fix(05-10)` commit; the Task 2 commit is test-only. The mutation check above shows the new cases are not vacuous.
- The Task 1 RED commit does not compile (the symbols under test do not exist yet), which is its failing state, as in plans 05-05 to 05-09.
- Plan wording "already running": the second admin's Ask is only possible while unhandled accounts remain, so the test has both admins ask first, the first confirm, then the second confirm (busy).

## Verification

- `go test -tags testtools -race -count=1 -run '^TestLockdownBanRecentJoiners$|^TestLockdownBanCount$' -v ./alita/modules`: pass (2 + 6 subtests).
- `go test -tags testtools -race -count=1 -run '^TestListApprovedIDsFresh$' -v ./alita/db/approvals`: pass; whole `./alita/db/approvals` and `./alita/db/lockdown`: pass.
- `go test -tags testtools -race -count=1 -run 'Lockdown|Unlockdown' ./alita/modules ./alita/db/lockdown`: pass (57 s).
- Full `go test -tags testtools -race -count=1 ./alita/modules`: pass (201 s). The intermittent FAIL seen once by the 05-08 executor did not reproduce in any of five runs of the lockdown suite here.
- `go test -tags testtools -count=1 ./alita/i18n` (including `TestLockdownLocaleKeys`), `make check-translations`, `make check-docs`, `CGO_ENABLED=0 go build ./...`, `go vet -tags testtools ./alita/modules ./alita/db/approvals`: clean. `gofmt -l` on every touched Go file prints nothing (the pre-existing `greetings_command_test.go` formatting is untouched and out of scope). `git diff --exit-code go.mod go.sum` exits 0.
- Acceptance greps pass: `responded := lockdownProcessBanRuns(ctx, b)`, `lockdown.ReleaseStaleBurstClaims(`, `models.ChatLockdownBanRun{}` once in the harness, `lockdown.StartBanRun(` in the callbacks, the `banned for good and can't rejoin` wording in `lockdown_ban_ask`, `ListApprovedIDsFresh` in AGENTS.md.
- `golangci-lint` was not run: the installed build targets Go 1.25 and cannot lint Go 1.26 code (go vet and gofmt used instead).
- Not run: any PostgreSQL check (SQLite only; the `NOT IN ?` exclusion list is bound as one parameter per approved user, fine for realistic approval lists but not exercised against a group with tens of thousands of approvals), and a real Telegram call.

## Threat surface

No surface beyond the plan's threat model. T-05-27 is covered by the non-admin case and the asker-only Confirm in `TestLockdownBanRecentJoiners`; T-05-28 by the three "approved read fails" cases; T-05-29 by the pacer (`lockdownPaced` inside `lockdownResponseBanOne`), batches of `lockdownWorkerBatch` and the "already running" case.

## Known Stubs

None.

## Known limits

- The Ban button's N is computed when the alert copy is posted and is not refreshed; the card shows the live N at Ask.
- A run that finishes between `StartBanRun` and Confirm's "Banning N…" edit could have its result overwritten by the running text; the worker only wakes on the 2 s tick or on that wake, and every account needs at least one paced call, so this needs a run of accounts that all skip without a Telegram call.

## Self-Check: PASSED

`alita/modules/lockdown_ban_test.go` exists; commits 672c6f4, 093cb4c and edbc7b8 are on the branch.
