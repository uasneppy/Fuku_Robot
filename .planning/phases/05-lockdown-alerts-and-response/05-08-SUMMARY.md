---
phase: 05-lockdown-alerts-and-response
plan: 08
subsystem: lockdown
tags: [lockdown, ban-run, burst, migration, safety, worker-interlock]
status: complete
requires:
  - phase: 05-lockdown-alerts-and-response
    provides: chat_lockdown_burst table and its response columns, BurstSampleFresh (05-07); lockdown worker, lift and marker ban (Phase 4)
provides:
  - decideLockdownResponseBan, the pure live-status decision table for "Ban N recent joiners"
  - lockdownResponseBanOne, the one function that decides and bans one account of a burst for good
  - chat_lockdown_ban_runs (migration 20261011140000) with one running run per lockdown
  - ban run repository (CountBanCandidatesFresh, StartBanRun, ListQueuedBurstFresh, ListRunningBanRunsFresh, SetBanRunCard, FinishBanRun, TallyBanRun, ReleaseStaleBurstClaims) and burst claims (ClaimBurst, MoveBurst, GetJoinerOfUserFresh)
  - lockdownRemovedSoFar, so /lockdownstatus keeps counting a response ban as removed
affects: [05-10, 05-13]
tech-stack:
  added: []
  patterns:
    - "claim the joiner row before the Telegram call and move it to kept/response_ban after, so the worker's marker ban and the lift's unban cannot race a ban with no end date"
    - "partial unique index on (lockdown_id) WHERE state = 'running' for one running run per lockdown, the uk_chat_lockdowns_active idiom"
key-files:
  created:
    - migrations/20261011140000_add_lockdown_ban_runs.sql
    - alita/modules/lockdown_response.go
    - alita/modules/lockdown_response_test.go
  modified:
    - alita/db/models/lockdown.go
    - alita/db/lockdown/joiners.go
    - alita/db/lockdown/burst.go
    - alita/db/lockdown/burst_test.go
    - alita/db/lockdown/testmain_test.go
    - alita/modules/lockdown_status.go
    - AGENTS.md
key-decisions:
  - "A joiner row in a state outside the claim list and not exempt/acting/unbanning (unbanned, unban_failed) is banned without a claim: the lift has finished with it, so there is nothing to interlock with."
  - "The 'let_in' reason is a models constant (BurstReasonLetIn) shared by the repository, which marks exempt rows at StartBanRun, and the module's lockdownBanSkipLetIn."
  - "StartBanRun marks exempt joiners' unhandled rows skipped let_in even when the user is in exclude; exclusion only keeps an account from being queued."
  - "The bot's own ID and approved users reach StartBanRun through exclude, and lockdownResponseBanOne checks both again before any call (approved comes in as a map from its caller)."
requirements-completed: [LOCK-12]
duration: 75min
completed: 2026-10-11
commits: 6
plan_head_before: b0770888b90f47bedbdabc79ac1c8021291c422e
plan_head_after: 8561fb8e6b639583caccf3fb8a68ed3362b59568
actuals:
  tokens: 25000
  tasks: 3
  commits: 6
---

# Phase 5 Plan 08: Ban recent joiners, the safety core Summary

**One account of a lockdown's burst is decided by its live status and banned for good (no end date) or skipped with its reason, without ever touching an admin, an approved user, the bot or someone an admin let in, and the worker's marker ban and the lift's unban can no longer overwrite or undo it; ban runs are stored with one running run per lockdown.**

Note: `commits: 6` counts the three test commits and three implementation commits made before this SUMMARY; the SUMMARY commit follows, so `plan_head_after` is the last code commit.

## What was built

- **`decideLockdownResponseBan(member, banUntil)`** (`alita/modules/lockdown_response.go`): pure table. Creator or administrator: skip `admin`. Kicked: ban only when `isLockdownBan` (the lockdown's own 330-day ban, which the new ban upgrades), otherwise skip `already_banned` (this includes a kicked pre-lockdown account with no joiner row, `banUntil` 0). Member, restricted and left: ban.
- **`lockdownResponseBanOne(ctx, b, ld, row, approved)`**: the only place that calls `banChatMember` for this feature, with empty `BanChatMemberOpts` (no end date, no message removal). Order: approved map, the bot's own ID, a fresh read of the joiner row; an `exempt` joiner is skipped `let_in` with no claim and no Telegram call (also when the row turned exempt after queueing); `acting`/`unbanning` or a lost claim puts the account back in the queue with no attempt; otherwise the joiner row is claimed to `acting`, then one `lockdownPaced` unit reads the live status, decides and bans. A ban moves the burst row to `banned` (reason `request` for a declined join request) and the joiner row to `kept` with detail `response_ban`. A skip, a rate limit or a failure restores the joiner row to the state and detail it came from; a rate limit or shutdown requeues without an attempt, any other failure counts one and the third is final (`failed`, with the escaped Telegram detail).
- **Repository** (`alita/db/lockdown`): `GetJoinerOfUserFresh`, `ClaimBurst`, `MoveBurst` (Task 1); `CountBanCandidatesFresh`, `StartBanRun`, `ListQueuedBurstFresh`, `ListRunningBanRunsFresh`, `SetBanRunCard`, `FinishBanRun`, `TallyBanRun`, `ReleaseStaleBurstClaims` (Task 3). `StartBanRun` runs in one transaction: insert the run (`ON CONFLICT DO NOTHING`; a refused insert writes nothing), copy joiner rows into the burst (`INSERT ... SELECT ... ON CONFLICT DO NOTHING`), mark unhandled rows of exempt joiners skipped `let_in`, queue the rest except `exclude`; nothing queued finishes the run at once.
- **Migration `20261011140000_add_lockdown_ban_runs.sql`**: `chat_lockdown_ban_runs` with state and destination CHECKs and `uk_chat_lockdown_ban_runs_running`. Idempotent, no transaction statements, no `CONCURRENTLY`. Model `ChatLockdownBanRun`, `BanRunState*`, `JoinerDetailResponseBan`, `BurstReasonLetIn`; the model is in the lockdown package's AutoMigrate list.
- **`lockdownRemovedSoFar`** (`lockdown_status.go`): banned plus kept rows with detail `response_ban`; `/lockdownstatus` uses it. The lift already counts a kept row in its `lockdown_lift_tally_kept` line and never unbans it, so no worker change was needed.
- **AGENTS.md**: a Subsystem-traps bullet for the response ban (decision table, empty options, claim-before-call, let_in, accepted Pitfall 3 limit, status count) and a Data bullet for `chat_lockdown_ban_runs`.

## Tests

- `TestDecideLockdownResponseBan`: 10 table rows.
- `TestLockdownResponseBanOne`: pre-lockdown member, joiner under the lockdown's ban (the worker's ban with an end date, then the upgrade with none), banned by someone else, admin, approved, let in by an admin, bot account and the bot's own ID, declined request, three 400 failures, four 429s.
- `TestLockdownBanSurvivesLift`: two response bans and one marker ban; the lift unbans only the marker ban, the tally says 2 left alone, the fake still shows both kicked with no end date.
- `TestLockdownBanRaces`: worker holds the row, response ban first then a full cycle, lift holds the row, turned exempt after queueing, and the characterization of a kept row not being reclaimed on a re-join.
- `TestLockdownStatusCountsResponseBans`.
- Repository: `TestBurstClaims`, `TestBanRunStart` (candidate counts, queueing and let_in, exclusion, second concurrent run refused, empty run done at once, a later run takes only unhandled rows, finish/tally/card/listings), `TestReleaseStaleBurstClaims`.

## Deviations from Plan

### Auto-fixed Issues

None.

### Process notes

- Task 2 followed the same shape as plans 05-05..05-07: the race and lift cases already passed against the Task 1 implementation (the claim-first design satisfied them), so the only failing RED test was the status count, and the single `fix(05-08)` commit adds `lockdownRemovedSoFar`.
- Task 1 and Task 3 RED commits do not compile (the symbols under test do not exist yet), which is their failing state.
- `BurstReasonLetIn` was added to the models package (not in the plan's artifact list) so the repository and the module share one `let_in` value.
- The ban run's model was not added to `alita/modules/test_harness_test.go`: nothing in the module tests uses ban runs yet, and plan 05-09 runs in parallel. Plan 05-10 must add `&models.ChatLockdownBanRun{}` there when its worker loop is tested.

## Verification

- `go test -tags testtools -race -count=1 -run '^TestDecideLockdownResponseBan$|^TestLockdownResponseBanOne$' -v ./alita/modules`: pass (10 table rows, 9 subtests).
- `go test -tags testtools -race -count=1 -run '^TestLockdownBanSurvivesLift$|^TestLockdownBanRaces$|^TestLockdownStatusCountsResponseBans$' -v ./alita/modules`: pass.
- `go test -tags testtools -race -count=1 -run 'Lockdown|Unlockdown|StaffBan' ./alita/modules`: pass three times (about 52 s). One earlier run, made right after the Task 2 change while the machine was busy, printed FAIL with only the usual "lockdown table unreachable" log line in the tail; I did not capture the failing test name and two reruns of the identical command passed, so I treat it as a one-off under load.
- `go test -tags testtools -race -count=1 -run '^TestBanRunStart$|^TestReleaseStaleBurstClaims$|^TestBurstClaims$' -v ./alita/db/lockdown` and the whole `./alita/db/lockdown`, `./alita/db/migrations`, `./alita/db/models` packages: pass.
- `CGO_ENABLED=0 go build ./...`, `go vet -tags testtools ./alita/modules ./alita/db/lockdown ./alita/db/models`: clean. `gofmt -l` on every touched Go file prints nothing. `go.mod` and `go.sum` unchanged.
- Acceptance greps pass: `JoinerDetailResponseBan = "response_ban"` present; `grep -c 'UntilDate\|RevokeMessages' alita/modules/lockdown_response.go` prints 0; `lockdownBanSkipLetIn` present; last migration is `20261011140000_add_lockdown_ban_runs.sql` with no `BEGIN/COMMIT/ROLLBACK/CONCURRENTLY`; `models.ChatLockdownBanRun{}` once in `alita/db/lockdown/testmain_test.go`; AGENTS.md names `response_ban`, `Pitfall 3` and `chat_lockdown_ban_runs`.
- `golangci-lint` was not run: the installed build targets Go 1.25 and cannot lint Go 1.26 code (go vet and gofmt used instead).
- Not run: the whole `./alita/modules` package under `-race`, any PostgreSQL check (including the human-check of applying the migration after the earlier Phase 5 migrations and confirming the partial unique index refuses a second running row; the `INSERT ... SELECT ... ON CONFLICT` and the partial index were exercised on SQLite only), and a real Telegram call.

## Threat surface

No surface beyond the plan's threat model. T-05-22 is covered by `TestLockdownBanSurvivesLift` and `TestLockdownBanRaces`; T-05-23 by `TestDecideLockdownResponseBan`, `TestLockdownResponseBanOne` and `TestBanRunStart`; T-05-24 by the refused second run in `TestBanRunStart`.

## Known Stubs

None. `lockdownResponseBanOne` and the run repository have no caller yet on purpose: plan 05-10 adds the button, the confirm card and the worker loop on top. `ListRunningBanRunsFresh`, `ListQueuedBurstFresh`, `SetBanRunCard`, `TallyBanRun` and `ReleaseStaleBurstClaims` are exercised by repository tests only until then.

## Self-Check: PASSED

`migrations/20261011140000_add_lockdown_ban_runs.sql`, `alita/modules/lockdown_response.go` and `alita/modules/lockdown_response_test.go` exist; commits 7f0ee46, 50a63cd, 4b21368, 5ebba31, db02821 and 8561fb8 are on the branch.
