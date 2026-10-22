---
phase: 06-automatic-raid-detection
plan: 07
subsystem: lockdown
tags: [raid-detection, notice, warning, redis, i18n, pacer]

requires:
  - phase: 06-automatic-raid-detection
    provides: raidFire with its latch and outcome switch (06-02, 06-06), startLockdownRow outcomes and BotMember (06-01), the Redis raid keys (06-03)
provides:
  - one public notice in a group the bot just locked on its own (raidPostNotice, D-13)
  - a once-a-day warning to the Staff Group and the log channel when a raid cannot be locked (raidWarnCannotLock, D-14, D-19)
  - lockdownTriggerSentence, the time-less trigger sentence plan 06-08 reuses for /lockdownstatus
  - withMiniredisServer and lockdownEnv.redis for tests that need to fast-forward Redis time
affects: [06-08, 06-10, 06-13]

actuals:
  tokens: 10700
  tasks: 2
  commits: 4

plan_head_before: 96c4e8384fcdd939c43c564d4981258209ce0087
plan_head_after: 50de04f0c0e2b3b012315a91039ebc3d8b50541c

commits: 4

tech-stack:
  added: []
  patterns:
    - "Exactly-once public notice: only the raidFire call whose startLockdownRow returned Started posts it"
    - "Once-a-day cross-replica gate: SET NX EX 86400 on alita:raid:{chat}:warn, a lost gate or Redis error sends nothing"
    - "User text (group title) escaped and spliced after translation through a token; the trigger sentence is spliced the same way so its percent sign never meets the translator's printf pass"

key-files:
  created:
    - alita/modules/raid_notice_test.go
    - alita/modules/testredis_test.go
  modified:
    - alita/modules/raid_trigger.go
    - alita/modules/lockdown_fake_test.go
    - alita/i18n/lockdown_locale_test.go
    - AGENTS.md
    - locales/en.yml
    - locales/es.yml
    - locales/fr.yml
    - locales/hi.yml
    - locales/id.yml
    - locales/pt.yml
    - locales/ru.yml
  removed:
    - alita/modules/antiraid_miniredis_test.go

key-decisions:
  - "The notice is sent by raidFire itself, after the latch is set for the outcome, and only for lockdownStartStarted, so a lockdown an admin or another trigger already started is never announced twice"
  - "raidSend is the one paced send helper for both the notice and the warning copies; each copy gets its own raidPostTimeout so a slow Staff Group copy cannot starve the log copy"
  - "The trigger sentence is spliced in through raidTriggerToken after translation, like the group label, so a flood sentence containing a percent sign never reaches the translator's legacy printf pass"
  - "The warning gate (SetNX) is taken before the destinations are resolved, as the plan lists it; a group with no Staff Group and no log channel therefore uses up its day without sending anything"

patterns-established:
  - "A test-only Redis helper that returns the server (withMiniredisServer) so a test can FastForward Redis time"

requirements-completed: [RAID-01, RAID-02]

duration: 14min
completed: 2026-10-18
status: complete
---

# Phase 6 Plan 07: Notice and cannot-lock warning Summary

**An automatically locked group now reads once why it went quiet and how to lift it, and a raid the bot cannot lock is reported once a day to the Staff Group and the log channel instead of failing silently.**

## Performance

- **Duration:** 14 min
- **Started:** 2026-10-18T13:00:37Z
- **Completed:** 2026-10-18T13:14:00Z
- **Tasks:** 2
- **Files modified:** 13 (2 created, 1 removed)

## Accomplishments

- `raidPostNotice` (D-13): the `raidFire` call that gets `lockdownStartStarted` posts one message in the locked group, in the group's language and through `lockdownPaced`: the trigger line with its numbers, `lockdown_note_approved_muted`, and the no-delete and no-invite notes `/lockdown` adds when `res.BotMember` lacks those rights. A failed send is logged at warn and changes nothing.
- `raidWarnCannotLock` (D-14, D-19): for `NotSupergroup`, `BotCannotRestrict` and `LockFailed` no lockdown is started and one warning naming the group and the trigger goes to the linked Staff Group (Staff Group language) and the log channel (admin category, group language, usual header first). One `SET NX EX 86400` on `alita:raid:{<chat>}:warn` gates both copies across replicas; a Redis error or a lost `SetNX` sends nothing. Bot rights unreadable, permissions unreadable, state failed and lock unknown only log.
- `lockdownTriggerSentence` renders the time-less trigger sentence; the warning uses it now and plan 06-08 reuses it.
- 7 locale keys in all 7 files; the three warning keys start with the warning marker and are in `lockdownWarningKeys`.
- `withMiniredis` moved to `testredis_test.go` beside `withMiniredisServer`, and `lockdownEnv.redis` keeps the server so the 24-hour dedupe is tested by fast-forwarding Redis time. `antiraid_miniredis_test.go` is gone, which also unblocks deleting the antiraid tests in 06-10.
- AGENTS.md documents the notice, the warning, the `:warn` key and which outcomes stay silent.

## Task Commits

1. **Task 1: automatic-lockdown notice**
   - RED `83071ba` (test): `TestRaidAutoNotice` fails with "0 messages reached the locked group, want exactly 1" in four subtests
   - GREEN `192bccd` (feat): `raidPostNotice`, `lockdownTriggerSentence`, 4 locale keys x 7 files
2. **Task 2: cannot-lock warning**
   - RED `e2068b1` (test): `TestRaidCannotLockWarnsOncePerDay` fails with "0 messages reached the Staff Group, want 1" in six subtests, and `TestLockdownWarningLines` fails for the three new keys in all 7 locales
   - GREEN `50de04f` (feat): `raidWarnCannotLock`, `raidSend`, 3 locale keys x 7 files, AGENTS.md

**Plan metadata:** the commit that adds this SUMMARY (docs).

## TDD Gate Compliance

RED and GREEN commits exist for both tasks (`test(06-07)` before `feat(06-07)`); no REFACTOR commit was needed. `raidSend` was introduced in the Task 2 GREEN commit and `raidPostNotice` switched to it there, a small in-commit cleanup.

RED evidence is recorded by hand because `gsd_run check tdd-red-evidence` is not installed in this worktree. In both RED runs the target test failed on an assertion for the planned behaviour (no message reached the chat), not on a compile error or an empty run.

Compile scaffolding in the Task 2 RED commit: the move of `withMiniredis` into `testredis_test.go`, the `lockdownEnv.redis` field and the `lockdownWarningKeys` extension. The new test needs `env.redis`, which Go will not compile without; these are test helpers, not behaviour. Several negative subtests (admin lockdown gets no notice, notice fails, transient failures, no destinations) pass in RED by design, because they assert that nothing is sent.

## Files Created/Modified

- `alita/modules/raid_trigger.go` - `raidPostNotice`, `raidWarnCannotLock`, `raidSend`, `lockdownTriggerSentence`, `raidPartWarn`, `raidWarnEvery`, `raidPostTimeout`, `raidTriggerToken`; the outcome switch at the end of `raidFire`
- `alita/modules/raid_notice_test.go` - `TestRaidAutoNotice`, `TestRaidCannotLockWarnsOncePerDay`
- `alita/modules/testredis_test.go` - `withMiniredisServer`, `withMiniredis` (moved)
- `alita/modules/lockdown_fake_test.go` - `lockdownEnv.redis`
- `alita/i18n/lockdown_locale_test.go` - three keys added to `lockdownWarningKeys`
- `locales/*.yml` - `lockdown_auto_started_join_surge`, `lockdown_auto_started_flood`, `lockdown_trigger_join_surge`, `lockdown_trigger_flood`, `lockdown_auto_cant_lock_rights`, `lockdown_auto_cant_lock_basic`, `lockdown_auto_cant_lock_refused`
- `AGENTS.md` - one bullet under Data

## Decisions Made

See `key-decisions`. One is worth a second look: the `SetNX` gate is taken before the Staff Group link and the log channel are resolved (the order the plan lists). A group with neither destination spends its day's gate without sending anything, so a Staff Group linked later the same day waits until the next day for a warning. The gate could be moved after the lookups to avoid that; it was left as planned because the latch already limits attempts to one per 10 minutes.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] `deliveryEnv` cannot be used for the warning test**
- **Found during:** Task 2 (writing the RED test)
- **Issue:** The plan names `deliveryEnv` as the fixture ("a Staff Group and a log channel"), but it also types `/lockdown raid`, which would start a lockdown before the trigger runs.
- **Fix:** A small `warnEnv` helper in `raid_notice_test.go` links the Staff Group and sets the log channel without starting anything; it uses the same `linkLockdownStaffGroup`, `setStaffLogChannel` and `wantAlertHeader` pieces.
- **Files modified:** `alita/modules/raid_notice_test.go`
- **Committed in:** `e2068b1`

### Additions beyond the plan's behaviour list

- Test subtests for a flood trigger warning, a Staff Group with no log channel, an escaped group title (T-06-23), a missing invite right, and an admin's `/lockdown` followed by a surge (no automatic notice, D-13). They add coverage for the threat register and for "only the trigger that started the lockdown posts it".
- A short AGENTS.md bullet. No existing rule was invalidated, but the new Redis key and posting behaviour are rules a later reader needs.

---

**Total deviations:** 1 auto-fixed (Rule 3 - fixture)
**Impact on plan:** none on behaviour; the production code is as planned.

## Issues Encountered

- `make lint` and a golangci-lint run cannot lint Go 1.26 here; `gofmt -l alita/modules alita/i18n` (clean) and `go vet -tags testtools ./alita/modules ./alita/i18n` (clean) stand in. `greetings_command_test.go` was not touched.
- No PostgreSQL server exists here and none was started; all tests ran on SQLite plus miniredis.
- The sandbox rejected compound shell commands in several calls, so file edits that were first attempted as a Python script were redone with the Edit tool and locale fragments were appended with a plain `cat >>`.

## Verification

- `CGO_ENABLED=1 go test -tags testtools -race -count=1 -run '^TestRaidCannotLockWarnsOncePerDay$|^TestRaidAutoNotice$' -v ./alita/modules`: PASS, every subtest
- `CGO_ENABLED=1 go test -tags testtools -race -count=1 ./alita/modules`: ok (264 s), so every test using `withMiniredis` still compiles and passes
- `CGO_ENABLED=1 go test -tags testtools -count=1 -run '^TestLockdownWarningLines$|^TestLockdownLocaleKeys$|^TestLockdownStringsHaveBalancedTags$' ./alita/i18n`: ok
- `make check-translations`: all translations present; `make check-docs`: no drift
- `CGO_ENABLED=0 go build ./...`: ok; `git diff --exit-code go.mod go.sum`: clean
- Acceptance greps: `lockdown_auto_started_join_surge:` and `lockdown_trigger_flood:` appear once in each of the 7 locale files; `func lockdownTriggerSentence(`, `func raidPostNotice(`, `func raidWarnCannotLock(` match; `antiraid_miniredis_test.go` is gone and `func withMiniredis(` is in `testredis_test.go`; `grep -c 'lockdown_auto_cant_lock_' alita/i18n/lockdown_locale_test.go` prints 3

## Known Stubs

None.

## Threat Flags

None. The new Redis key `alita:raid:{<chat>}:warn` is under the already documented `alita:raid:*` prefix. T-06-22 (spam) is covered by the SET NX gate, the Started-only notice and `lockdownPaced`; T-06-23 (title injection) by escaping and post-translation splicing (the `title is escaped` subtest); T-06-24 (wrong destination) by the uncached staff link and `actionlog.Destination`.

## Next Phase Readiness

- Plan 06-08 can call `lockdownTriggerSentence(tr, kind, count, window, percent)` for `/lockdownstatus`.
- Plan 06-10 can delete the antiraid tests without losing `withMiniredis`.
- Nothing blocks the next wave.

## Self-Check: PASSED

- `alita/modules/raid_notice_test.go`, `alita/modules/testredis_test.go`: present
- Commits `83071ba`, `192bccd`, `e2068b1`, `50de04f`: present in `git log`
