---
phase: 05-lockdown-alerts-and-response
plan: 11
subsystem: lockdown
tags: [lockdown, ban-run, report, log-channel, worker, i18n]
status: complete
requires:
  - phase: 05-lockdown-alerts-and-response
    provides: ban core and run repository (05-08), Ban button, confirm card and worker run (05-10), alert copies and closing (05-03)
provides:
  - lockdown.UnqueueBanRun and lockdown.ListBurstOfRunFresh
  - lockdownStopBanRun, a run that stops when its lockdown is no longer active
  - lockdownBanResultText, the full report (banned, declined requests, skipped per reason, failed by name)
  - lockdownBanLogPost, the one #LOCKDOWN_BAN record per run in the group's log channel
affects: [05-13]
tech-stack:
  added: []
  patterns:
    - "re-read the lockdown before every account of a batch, so a lift that starts mid-batch ends the run at the next account"
    - "show the running text on the card before the run row exists, so the only later writer of the card is the worker"
key-files:
  created:
    - alita/modules/lockdown_ban_run_test.go
    - .planning/phases/05-lockdown-alerts-and-response/deferred-items.md
  modified:
    - alita/db/lockdown/burst.go
    - alita/db/lockdown/burst_test.go
    - alita/modules/lockdown_response.go
    - alita/modules/lockdown_alert_callbacks.go
    - AGENTS.md
    - locales/en.yml
    - locales/es.yml
    - locales/fr.yml
    - locales/hi.yml
    - locales/id.yml
    - locales/pt.yml
    - locales/ru.yml
key-decisions:
  - "A run of a lockdown that is lifting, lifted or gone is stopped by the worker: queued accounts go back to unhandled (state '', run_id NULL), FinishBanRun(stopped) is the same conditional update as a normal end, and the winner edits the card and posts the log."
  - "The lockdown is read again before every account (one cheap read per paced Telegram call), not only once per pass, so a lift starting inside a batch of up to 50 accounts stops the run at the next account."
  - "Confirm edits the card to 'Banning N' (N from CountBanCandidatesFresh) before StartBanRun, instead of after it, which closes the 05-10 window where the worker's result could be overwritten by the running text. With nothing to count the card is left alone and StartBanRun alone decides busy or nothing left."
  - "A failed account is named with its escaped first name (or the ID), its ID in code tags and the stored escaped Telegram reason, up to 10, the rest counted with lockdown_lift_tally_more; a plain escaped name, not a mention, so a raider is not pinged by the report."
  - "The #LOCKDOWN_BAN post is sent for done and stopped runs that handled at least one account, by the replica whose FinishBanRun won, even when the run has no card."
requirements-completed: [LOCK-12]
duration: 75min
completed: 2026-10-11
commits: 4
plan_head_before: d2d4ceeff0ad765cbb83a785c92bcc911ed6cfa4
plan_head_after: e192b7d0cd6ee8e36171a69113287c74088420fa
actuals:
  tokens: 33000
  tasks: 2
  commits: 4
---

# Phase 5 Plan 11: Ban recent joiners through the whole raid Summary

**Staff can press Ban again as a raid goes on and each press handles only accounts no earlier press handled; a restart resumes a run on any replica, a lift stops it mid-batch and the card says so; the ban card keeps the Lift card's safety; the result card reports banned (declined requests apart), skipped per reason (admins, approved, already banned, let in by an admin) and failed accounts by name, ID and Telegram's reason; and the group's log channel gets exactly one `#LOCKDOWN_BAN` post per run naming who pressed Ban, with nothing written to the staff audit.**

Note: `commits: 4` counts the two RED test commits and the two implementation commits made before this SUMMARY; the SUMMARY commit follows, so `plan_head_after` is the last code commit.

## What was built

- **`lockdown.UnqueueBanRun(runID)`** (`alita/db/lockdown/burst.go`): queued rows of the run go back to unhandled (empty state, no run, no claim). Banned, skipped, failed and acting rows stay with the run. `FinishBanRun(stopped)` already did not need an empty queue.
- **Stop at the lift** (`lockdown_response.go`): `lockdownProcessBanRun` now stops a run whose lockdown is missing or not active (`lockdownStopBanRun`: `UnqueueBanRun`, then `FinishBanRun(stopped)`, and the winner edits the card with the stopped line and posts the log). The lockdown is read again before every account (`lockdown.GetFresh`), so a lift that starts inside a batch stops the run at the next account. A failed lockdown read makes the run wait.
- **Report** (`lockdownBanResultText`, `lockdownBanFailedLines`): `lockdown_ban_result`, `lockdown_ban_result_requests` when banned rows with reason `request` exist, one line per skip reason present, `lockdown_ban_failed_line` plus up to 10 accounts and `lockdown_lift_tally_more` for the rest, `lockdown_ban_stopped` when stopped. The group label and the failed list are spliced in after translation through tokens; the list is bounded by `lockdownTallyMaxRunes`.
- **Log post** (`lockdownBanLogPost`): `actionlog.Destination(chat, CategoryAdmin)`, header plus `#LOCKDOWN_BAN · Admin: <mention of the presser> · <lockdown_ban_log>` in the locked group's language, one paced send. Not sent for a run that handled no account, when there is no log channel or the category is off, or when the context is cancelled. A failed post is logged at warn and changes nothing. No staff audit row is written (no `StaffAction` outside comments in the file).
- **`lockdown.ListBurstOfRunFresh(runID, state, limit)`**: the failed accounts of a run in queue order.
- **Ban card** (`lockdown_alert_callbacks.go`): checked against the Lift card; Ask and Confirm both run `lockdownButtonAuthority`, Confirm checks asker, lazy expiry, lockdown still active, live authority and approvals again, Cancel is asker-only, and a service identity is refused before any lookup. No change was needed there; the tests pin it. Confirm now edits the card to the running text before `StartBanRun` (see Deviations).
- **Locale keys** in all 7 files: `lockdown_ban_stopped`, `lockdown_ban_result_requests`, `lockdown_ban_skip_admin`, `lockdown_ban_skip_approved`, `lockdown_ban_skip_banned`, `lockdown_ban_skip_let_in`, `lockdown_ban_failed_line`, `lockdown_ban_log`.
- **AGENTS.md**: the "Ban N recent joiners" bullet now states later presses, resume, stop at the lift, the report, the winner-only card edit and `#LOCKDOWN_BAN` post, the card ordering and the absence of audit and undo.

## Tests

- `TestLockdownBanRun`: "second press" (J1 and J2 banned; J3 joins; second Ban card says 1; Confirm bans only J3, no new ban or lookup for J1 and J2; second card banned 1), "restart" (a burst row left acting 3 minutes ago is released by the next cycle, banned, run done, card edited), "lifted mid-run" (/unlockdown before any cycle: no ban for good, run stopped with a finish time, the three rows unhandled with no run, card says stopped), "lift starts in the middle of a batch" (the lift is recorded through the approved-users seam after the worker read the lockdown; no account is banned and the run is stopped).
- `TestLockdownBanAuthority`: admin without can_restrict_members, admin demoted between Ask and Confirm, another admin's Confirm and Cancel (answered not-yours, card untouched, then the asker's own Cancel works), expired Confirm data, the three service identities on Ban and Confirm (no card, no lookup).
- `TestLockdownBanResult`: "full report" (P1 member, P2 admin, P3 kicked by someone else, J1 raider, E added by an admin, R declined request, F failing three times with a 400: banned 3, skipped 3, failed 1; requests line, admin, banned and let-in lines; F's escaped name, ID in code tags and escaped reason), "failed list is cut" (12 failing accounts: 10 listed, "2 more"), "log post" (exactly one `#LOCKDOWN_BAN` post with the label, the escaped presser and the mention; extra cycles post nothing; a second run posts a second), "no audit" (`staff_actions` row count unchanged), "category off" (no post, run still done, card edited), "a stopped run that handled nothing posts nothing".
- Repository: `TestUnqueueBanRun` (only queued rows return; banned, failed, acting and let_in rows stay; stopped finish works once even with an acting row; the returned accounts count as candidates again), `TestListBurstOfRunFresh`.
- Mutation checks: replacing the per-account lockdown re-read with a nil check makes "lift starts in the middle of a batch" fail (run done, every account banned); removing the "handled at least one account" guard makes "a stopped run that handled nothing posts nothing" fail.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] The "Banning N" edit could overwrite a finished run's result (the window noted by 05-10 and by the orchestrator)**
- **Found during:** Task 1 (this plan touches the result and card edit path)
- **Issue:** Confirm edited the card to the running text after `StartBanRun`; a worker tick in between could finish a run whose accounts all skip without a Telegram call and edit the result, which the running text then replaced.
- **Fix:** Confirm now counts the candidates (`CountBanCandidatesFresh`), edits the card to the running text, and only then calls `StartBanRun`. The worker cannot start before the run row exists, so the worker's result is always the last write. A refusal (busy, nothing left, start failed) overwrites the running text. With a count of 0 the card is not edited first, so a second admin's Confirm while a run is queued still reads "already running" (the first version of this change broke `TestLockdownBanCount/already running`; the count-0 case fixed it).
- **Files modified:** `alita/modules/lockdown_alert_callbacks.go`
- **Commit:** 201c19d

**2. [Rule 2 - Missing critical functionality] Per-account lockdown re-read, beyond the plan's "a run whose lockdown is not active is stopped"**
- **Issue:** the plan stops a run at the start of a pass, but a pass handles up to 50 accounts, each with a paced Telegram call that can wait seconds; a lift starting mid-pass would still get bans for good it will never take back, against the prohibition "MUST NOT keep banning after the lockdown left the active state".
- **Fix:** the lockdown is read before every account; a non-active lockdown ends the run at the next account. Covered by the mid-batch test and its mutation check.
- **Commit:** 201c19d

### Process notes

- Task 1: only "lifted mid-run" failed on the RED run; "second press", "restart" and every `TestLockdownBanAuthority` case already passed against the 05-10 code (it already shares the Lift card's asker, expiry, Cancel and service-identity checks and re-checks authority at Confirm). They are kept as the pinned tests the plan asks for. The RED repository test does not compile (`UnqueueBanRun` does not exist yet), which is its failing state, as in plans 05-05 to 05-10.
- Task 2: the RED run failed on the missing report lines and the missing log post; the "no audit", "category off" and "stopped run posts nothing" cases passed from the start because nothing wrote an audit row or a log post yet (the mutation check above shows the last is not vacuous).
- The test for the full report uses ascending user IDs in join order, because the joiner rows are copied into the burst by an `INSERT ... SELECT` without an `ORDER BY`, so the scripted refusal of `banChatMember` (first in, first out per chat) must meet the accounts in a known order whichever index SQLite scans.
- An un-stopped "stopped after some accounts" case (a lift that arrives after at least one account was handled, so a stopped run posts the log) is not tested: it needs a hook between two accounts that the fake does not have. The code path is the same `lockdownBanRunResult` that the done case tests.

## Verification

- `go test -tags testtools -race -count=1 -run '^TestLockdownBanRun$|^TestLockdownBanAuthority$' -v ./alita/modules`: pass (4 + 5 subtests).
- `go test -tags testtools -race -count=1 -run '^TestLockdownBanResult$' -v ./alita/modules`: pass (6 subtests).
- `go test -tags testtools -race -count=1 -run '^TestUnqueueBanRun$|^TestBanRunStart$|^TestBurstClaims$' -v ./alita/db/lockdown` and the whole `./alita/db/lockdown`: pass.
- `go test -tags testtools -race -count=1 -run 'Lockdown|Unlockdown|StaffLog' ./alita/modules ./alita/db/lockdown`: pass (57 s) on the first run of the final code in the Task 1 tree; on the Task 2 tree one run failed in `TestLockdownResponseBanOne` (see Deferred Issues) and every other run passed.
- Full `go test -tags testtools -race -count=1 ./alita/modules`: pass (204 s) on the final code.
- `go test -tags testtools -count=1 -run '^TestLockdownLocaleKeys$' ./alita/i18n`, `make check-translations`, `make check-docs`, `CGO_ENABLED=0 go build ./...`, `go vet -tags testtools ./alita/modules ./alita/db/lockdown`: clean. `gofmt -l` on every touched Go file prints nothing (the pre-existing `greetings_command_test.go` formatting is untouched and out of scope). `git diff --exit-code go.mod go.sum` exits 0.
- Acceptance greps pass: `func UnqueueBanRun(`, `lockdown.UnqueueBanRun(`, `"#LOCKDOWN_BAN`, 0 non-comment `StaffAction` lines in `lockdown_response.go`, `#LOCKDOWN_BAN` in AGENTS.md, `lockdown_ban_stopped` and `lockdown_ban_skip_let_in` once in each of the 7 locale files.
- `golangci-lint` was not run: the installed build targets Go 1.25 and cannot lint Go 1.26 code (go vet and gofmt used instead).
- Not run: any PostgreSQL check (SQLite only) and a real Telegram call.

## Deferred Issues

- Intermittent setup failure in two 05-08 tests (`TestLockdownBanRaces/unbanned_by_hand,_then_rejoins` at `lockdown_response_test.go:593`, `TestLockdownResponseBanOne` at `:172`): `UNIQUE constraint failed: chat_lockdown_burst.lockdown_id, chat_lockdown_burst.user_id`, about 1 run in 4 to 6 when run alone. A join made right after `lockAsAdmin` can fall inside the alert snapshot's window (up to and including `locked_at`), so the burst row the test then seeds already exists. Not caused by this plan (neither path runs a ban run or its card); recorded with the fix options in `deferred-items.md`. This is very likely the intermittent FAIL the 05-08 executor saw once.

## Threat surface

No surface beyond the plan's threat model. T-05-30 is covered by "lifted mid-run" and "lift starts in the middle of a batch"; T-05-31 by the log-post and full-report cases (the presser is stored on the run, named in the post and shown on the card); T-05-32 by `TestLockdownBanAuthority`.

## Known Stubs

None.

## Known limits

- The "Banning N" text can show for a moment on a second admin's card before "already running" replaces it, when new joiners arrived after the first run was queued; a failed second edit would leave it, which only misstates that this card started a run while the other run is handling the same accounts.
- A stopped run's acting row (a call in flight on another replica) is moved by that call; if its replica died, the stale release returns it to the queue under the stopped run, where nothing picks it up after the lift. It stays in the lockdown's burst and is unhandled for no later run (the lockdown is over).
- The log post of a run is not retried if the replica that won `FinishBanRun` dies before sending it (the accounts' states and the card are stored first).

## Self-Check: PASSED

`alita/modules/lockdown_ban_run_test.go` and `deferred-items.md` exist; commits 7e435cf, 201c19d, 90a5265 and e192b7d are on the branch.
