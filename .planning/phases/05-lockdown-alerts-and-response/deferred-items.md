# Phase 5 deferred items

## From plan 05-11

- **Intermittent test setup failure in the 05-08 response-ban tests** (`alita/modules/lockdown_response_test.go`:
  `TestLockdownBanRaces/unbanned_by_hand,_then_rejoins` at line 593 and `TestLockdownResponseBanOne` at line 172).
  Symptom: `seed burst row of <id>: UNIQUE constraint failed: chat_lockdown_burst.lockdown_id, chat_lockdown_burst.user_id`,
  about 1 run in 4 to 6 when the test is run alone. Cause: these tests `join` a user right after `lockAsAdmin` and run a
  cycle before `queuedBurst` writes that user's burst row. The alert's tracker snapshot (`lockdownBurstSnapshot`, window
  up to and including `locked_at`) sometimes counts the join as pre-lockdown, so `BeginAlert` already inserted a burst
  row for the user. The 05-10 tests avoid this with a 3 ms sleep after the lock. Not caused by 05-11 (the failing
  paths do not run a ban run or its card), so it was not changed here. Fix: sleep a few milliseconds in `lockAsAdmin`,
  or have `queuedBurst` update an existing row instead of creating one.

  **Resolved by the orchestrator after wave 9:** `queuedBurst` now reuses and resets a burst row the alert snapshot
  already saved for that user instead of inserting a duplicate. The same flake had also hit `TestLockdownBanSurvivesLift`
  in the wave 8 `make test` gate. `TestLockdownBanRaces`, `TestLockdownResponseBanOne` and
  `TestLockdownBanSurvivesLift` then passed 15 runs in a row under `-race`.
