---
type: debug
id: 261118-p5l-race
status: resolved
---
# Data race in the root package tests

Symptom: `go test -tags testtools -race -count=1 .` reports a race between
`deletion.ReleaseStale` (the deletion worker's cycle, reading `db.DB`) and the cleanup of
`TestCloseDBConnectionsNilDBReturnsNil` (writing `db.DB`).

Root cause: `TestPostInitSetsCommandsAndStartupMessage` runs `postInit`, which starts five
background loops (captcha lifecycle, staff sweeper, admin index sweeper, lockdown worker,
deletion worker). Its cleanup stopped only three of them, so the deletion worker and the
admin index sweeper kept running into the next tests, reading `db.DB` while those tests
replaced it.

Production: not affected. `db.DB` is assigned once by `fuku/db`'s `init()` before `main()`
and is never replaced while the workers run; shutdown stops the workers before the
database close.

Fix: the test's cleanup also calls `StopDeletionWorker` and `StopAdminIndexSweeper`
(main's shutdown order) before restoring `db.DB`. Neither test is weakened or skipped.

Verified: the command passes 5 times in a row.
