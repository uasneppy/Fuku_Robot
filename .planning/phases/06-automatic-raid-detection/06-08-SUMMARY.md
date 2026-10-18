---
phase: 06-automatic-raid-detection
plan: 08
subsystem: lockdown
tags: [raid-detection, lockdown-status, reminder, i18n]

requires:
  - phase: 06-automatic-raid-detection
    provides: lockdownTriggerSentence and the trigger columns on chat_lockdowns (06-02, 06-07), join surge and flood triggers (06-04, 06-05, 06-06)
provides:
  - lockdownStartedAutomatically, the one predicate for "the bot locked this group on a trigger"
  - /lockdownstatus, the already-active reply of /lockdown and the hourly reminder name the trigger and its numbers for an automatic lockdown (D-13, D-21)
  - locale keys lockdown_status_active_auto and lockdown_already_active_auto in all 7 languages
affects: [06-10, 06-13]

actuals:
  tokens: 6600
  tasks: 2
  commits: 4

plan_head_before: a1ef85d0c26424af870d4f3924b50f01d4ffad9c
plan_head_after: 2626d3a32321682e91788d2957ec10aa87c6e5da

commits: 4

tech-stack:
  added: []
  patterns:
    - "Automatic lockdowns are told from the trigger columns (lockdownStartedAutomatically), never from reason or from the starter being the bot"
    - "The trigger sentence is rendered per reader from trigger_kind/count/window/percent, so each destination reads it in its own language"

key-files:
  created:
    - alita/modules/raid_status_test.go
  modified:
    - alita/modules/lockdown.go
    - alita/modules/lockdown_status.go
    - alita/modules/lockdown_alert.go
    - AGENTS.md
    - locales/en.yml
    - locales/es.yml
    - locales/fr.yml
    - locales/hi.yml
    - locales/id.yml
    - locales/pt.yml
    - locales/ru.yml

key-decisions:
  - "No wrapper around lockdownTriggerSentence: the three call sites (status, already-active, reminder) each pass the row's four trigger columns directly, which keeps the plan's grep acceptance criteria literally true"
  - "An automatic lockdown's status and already-active reply leave the reason line out entirely; only the trigger sentence describes why it was locked"
  - "The reminder puts the trigger sentence where the reason line was, so its layout (title, reason-or-trigger, counts, link) is unchanged for manual lockdowns"

patterns-established:
  - "Admin-facing text branches on lockdownStartedAutomatically(ld); every other trigger_kind, manual included, keeps today's text"

requirements-completed: [RAID-01, RAID-02]

duration: 17min
completed: 2026-10-18
status: complete
---

# Phase 6 Plan 08: Status, already-active and reminder name the automatic trigger Summary

**An admin who asks about, retries or is reminded of an automatically locked group now reads "locked automatically" plus the trigger (join count and window, or flood count, window and percent) in their own language, instead of "started by" the bot with an empty reason.**

## Performance

- **Duration:** 17 min
- **Tasks:** 2
- **Files modified:** 12 (1 created)

## Accomplishments

- `lockdownStartedAutomatically(ld)` (in `lockdown_status.go`) is true for `trigger_kind` `join_surge` or `flood` and false for everything else, nil included.
- `/lockdownstatus` of an automatic lockdown opens with `lockdown_status_active_auto` (since) and the trigger sentence; the reason line and the "started by" name are gone. The removed, ban-failed, declined, unconfirmed and permissions lines are untouched.
- `/lockdown` during an automatic lockdown answers `lockdown_already_active_auto` (since) plus the trigger sentence and makes no `setChatPermissions` call (asserted).
- The hourly reminder shows the trigger sentence where a manual lockdown shows its reason.
- Two locale keys in all 7 files with identical placeholders (`{since}`); `make check-translations` is clean.
- AGENTS.md: a new bullet describes the automatic wording of the status and already-active reply, and the Phase 5 reminder bullet now says the trigger sentence replaces the reason for an automatic lockdown.

## Task Commits

1. **Task 1 RED:** `9ee0722` test(06-08): show the status of an automatic lockdown says the bot started it
2. **Task 1 GREEN:** `48f57d0` feat(06-08): name the automatic trigger in the lockdown status
3. **Task 2 RED:** `fd01135` test(06-08): show an automatic lockdown's reminder gives no reason
4. **Task 2 GREEN:** `2626d3a` feat(06-08): name the automatic trigger in lockdown reminders

## TDD Gate Compliance

Both tasks have a `test(06-08)` commit before their `feat(06-08)` commit. Each RED run failed on the planned assertions only (the reply used `@@lockdown-status-active@@`, `@@lockdown-already-active@@` or lacked the trigger marker; the manual subtests already passed because they pin unchanged behavior). The RED commits compile without scaffolding; no REFACTOR commit was needed.

## Tests

- `TestRaidLockdownStatus`: surge status, flood status, already active, manual unchanged.
- `TestRaidLockdownReminder`: surge reminder, flood reminder, manual reminder. They age `alert_started_at` by 61 minutes and run one worker cycle, reading the reminder the Staff Group received.
- Test helpers `surgeLockdown` and `floodLockdown` build a real automatic lockdown through the join and message handlers (Redis via miniredis, hand-written BotClient fake), so the assertions cover the whole path from trigger to admin text.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Missing critical] AGENTS.md updated in the same commits**
- **Found during:** Task 1 and Task 2
- **Issue:** AGENTS.md says "A reminder shows the reason ..." and documents no automatic wording for the status and already-active reply; the repo rule is to update it in the commit that changes the rule.
- **Fix:** One new bullet (Task 1 commit) and a clause in the reminder bullet (Task 2 commit).
- **Files modified:** AGENTS.md
- **Commit:** 48f57d0, 2626d3a

**Total deviations:** 1 auto-fixed (1 missing critical). **Impact:** documentation only; no behavior change beyond the plan.

## Verification

- `CGO_ENABLED=1 go test -tags testtools -race -count=1 -run 'Lockdown|Unlockdown|Raid' ./alita/modules`: ok (116 s)
- `TestLockdownLocaleKeys`, `TestLockdownStringsHaveBalancedTags` (`./alita/i18n`): pass
- `make check-translations`: all translations present; `make check-docs`: no drift
- `CGO_ENABLED=0 go build ./...`, `go vet -tags testtools ./alita/modules`: clean
- `gofmt -l` on the four touched Go files: nothing; `go.mod` and `go.sum` unchanged
- `make lint` could not run here (golangci-lint cannot lint Go 1.26); `gofmt` and `go vet` stand in. The full `make test` was left to the orchestrator.

## Known Stubs

None.

## Threat Flags

None. `/lockdownstatus` keeps `requireLockdownAuthority(false)` and the reminder its Phase 5 destinations; the new text holds only numbers (T-06-25, T-06-26 mitigated and covered by `TestRaidLockdownStatus`).

## Self-Check: PASSED

- `alita/modules/raid_status_test.go` exists; commits `9ee0722`, `48f57d0`, `fd01135`, `2626d3a` exist on the branch.
- Acceptance criteria re-run: `func lockdownStartedAutomatically(` in `lockdown_status.go`; `lockdownTriggerSentence(` count 1 in each of `lockdown.go`, `lockdown_status.go`, `lockdown_alert.go`; `lockdown_status_active_auto:` once in each of the 7 locale files; `gofmt -l` prints nothing; `git diff --exit-code go.mod go.sum` exits 0.
