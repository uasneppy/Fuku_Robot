# Quick task 261117-w5l: lockdown review fixes (Wave 5) — summary

Branch `wave5-lockdown` from `6f2acd4`, not pushed. Each item has a test written first that failed on the old code
(most in `fuku/modules/lockdown_review_test.go`), then the fix. AGENTS.md is updated in the commit of each item.

## Fixed

| ID | Commit | What changed | Test |
|----|--------|--------------|------|
| L1 | `1edc45f` | `startLockdownRow` reads the row again when `ConfirmLocked` matches nothing. When an `/unlockdown` lifted the unconfirmed row while the lock call was on its way (lifting or lifted), it puts the stored permissions back and returns the new outcome `lockdownStartLiftedMeanwhile`. `/lockdown` answers `lockdown_already_lifted`; a raid trigger posts nothing (latch "lifted during the lock"). | `TestLockdownStartLiftedDuringLock` |
| L2 | `c0a7809` | New `lockdown.RouteUncertainBans`. At the lift (inline in `liftLockdownRow` before `FinishLift`, and in every worker cycle before `CancelPending`) it moves `ban_failed` rows and pending rows with an attempt counted (join requests excepted) to `banned`, with detail `models.JoinerDetailLiftCheck` and the attempts at zero. The live-checked unban then removes a marker ban. It cancels the row when the member carries no ban, so the tally does not count it as kept. An unban retry keeps the marker. | `TestLockdownLiftChecksBansWhoseAnswerFailed` |
| L3 | `b6e16a0` | A card's Confirm (`lc` `bc` `rc`) or Cancel (`x`) is honoured only on a card of the copy (`lockdownIsCardOf`: it replies to the copy's message and is not the copy). Anything else is answered "no longer valid", with no edit and no lookup. A stale press edits only a card of the copy. The test helper `pressLockdownButton` now gives the pressed message its `reply_to_message`, as Telegram does. | `TestLockdownAlertForgedCardPress` |
| L4 | `e69be71` | `lockdownUnbanOne` does the live read and the unban in one paced closure, so a slot wait or a 429 retry reads the member again. | `TestLockdownUnbanRereadsAfterWait` |
| L5 | `cf97541` | `decideLockdownJoin` bans a joiner whose performer is a bot account (`PerformerIsBot`, from the update's or the message's `from`), even a bot administrator. Such a join makes no performer lookup. The anonymous admin stays exempt. The raid join counter is unchanged. | `TestLockdownGuardBansWhenABotAdminAddsSomeone`, plus a case in `TestDecideLockdownJoin` |
| L6 | `b007020` | `lockdown.ListRemindableFresh(afterID, limit)`. The planner keeps an in-memory cursor (`lockdownReminderCursor`) and reads `lockdownReminderBatch` (50) after it. A short batch starts over from the lowest ID. | `TestLockdownRemindersReachEveryLockdown` |
| L7 | `a511e2a` | When the lift itself wins `FinishLift`, its caller posts the tally after its own announcement (`lockdownPostInlineTally`). The callers are `/unlockdown`, the alert's Lift Confirm and the settings Lift Yes. | `TestLockdownInlineLiftPostsTally` |
| L8 | `b6e05ce` | `lockdownStaleClaim` goes from 2 to 5 minutes. One paced call can take up to (3+1) × (60 s + 10 s) = 4 min 40 s. The pacer options are now `lockdownPacerOptions`. Existing tests that aged claims by 3 minutes now age them by `lockdownStaleClaim + 1 min`. | `TestLockdownStaleClaimOutlastsAPacedCall` |

## Not reproduced

None.

## Owner decisions

None from this wave. No `TestLegacy*Requests` golden covers lockdown code, so no pinned bytes were changed.

## User-visible behaviour changes

- `/lockdown` replies "This lockdown was already lifted." in one case: an `/unlockdown` lifted the row while the lock
  call was still running. The group's permissions are put back instead of staying locked.
- At a lift, raiders whose ban answer was a server error or a timeout are now unbanned when the ban did land.
  - Before: they kept the lockdown's 330-day ban.
  - The "unbanning N" count in the lift notice can include them.
- A forged Confirm or Cancel no longer edits the alert copy or another bot message. A press on a real card is unchanged.
- During a lockdown, a user added by another bot administrator is banned. Before, that user was let in.
- When a lift finishes at once (only kept bans), the group now gets the lift tally.
  - Before: nothing was posted.
  - The tally comes after the lifted notice.
- Reminders reach every due lockdown, even when more than 50 are active.
- A stopped worker's claims are released after 5 minutes instead of 2. Rows of a crashed worker resume about
  3 minutes later than before.

## Verification

- `go test -tags testtools -race -count=1 -run '^(TestLockdown|TestSettingsLockdown|TestRaid|TestDecideLockdown|TestStaffBanOnLockdown|TestStaffBanLockdown|TestStaffPanelShowsLockedGroup|TestAntiRaid)' ./fuku/modules`: ok (200 s).
- `go test -tags testtools -race -count=1 ./fuku/db/lockdown`: ok.
- `CGO_ENABLED=0 go build ./...`: ok.
- `make check-translations`: ok.
- `make lint`: 0 issues.
  - The installed golangci-lint 2.5.0 was built with go1.25 and refuses this go1.26 module.
  - Lint was run with golangci-lint v2.15.0, built locally into the scratchpad, through
    `make lint GOLANGCI_LINT_CMD=...`.

## Left unfinished

Not run: `make test`, the whole `fuku/modules` package and `make test-postgres-integrity`. That follows the wave's
instructions.

`RouteUncertainBans` is a plain conditional `UPDATE` and adds no PostgreSQL-only guarantee.
