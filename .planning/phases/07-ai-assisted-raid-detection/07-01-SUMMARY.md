---
phase: 07-ai-assisted-raid-detection
plan: 01
subsystem: raid-detection
tags: [redis, lua, lockdown, aispam, typesafe, i18n, go]

requires:
  - phase: 06-smarter-raid-detection
    provides: raidFire, startLockdownRow, the :joined set, the flooders snapshot pattern, lockdown trigger columns
  - phase: 05-lockdown-alerts
    provides: lockdown alert, burst rows, Ban N recent joiners
provides:
  - "models.LockdownTriggerAIBurst (ai_burst) and models.LockdownTriggerKinds()"
  - "Redis AI verdict counter (alita:raid:{chat}:av and :aa) and the AI burst rule (raidAICounts)"
  - "raidAIRecordVerdict, fed by every /aispam delete verdict (D-03, D-19)"
  - "ai_burst on every trigger-kind surface (alert, status, reminder, notice, cannot-lock warning, /lockdown while locked)"
  - "AI spammer snapshot (alita:raid:{chat}:aispammers) feeding the alert sample and Ban N recent joiners"
affects: [07-02, 07-03, 07-04, 07-05, 07-06, 07-07, 07-14]

actuals:
  tokens: 23900
  tasks: 3
  commits: 7
plan_head_before: 53a48763edad1c48a0b307df9c12c3e14043bbec
plan_head_after: 64f57c30c85e30029505fded2db3dd9ecd276e5c

tech-stack:
  added: []
  patterns:
    - "Verdict counter as one Lua script: ZADD NX on the item, account written only for a newly added item"
    - "Trigger-kind surface table test over models.LockdownTriggerKinds()"
    - "One shared snapshot merge (lockdownMergeSnapshot) with thin per-trigger wrappers"

key-files:
  created:
    - alita/modules/raid_ai_count.go
    - alita/modules/raid_ai_burst_test.go
  modified:
    - alita/db/models/lockdown.go
    - alita/modules/aispam.go
    - alita/modules/raid_trigger.go
    - alita/modules/lockdown_alert.go
    - alita/modules/lockdown_status.go
    - locales/en.yml
    - locales/es.yml
    - locales/fr.yml
    - locales/hi.yml
    - locales/id.yml
    - locales/pt.yml
    - locales/ru.yml
    - AGENTS.md

key-decisions:
  - "A verdict only adds one item; only raidAICounts (threshold from at least 3 accounts in a fixed 120 s window, never below MinThreshold) calls raidFire"
  - "raidAIRecordVerdict calls raidFire synchronously in the /aispam worker for now; plan 07-14 moves it onto a tracked goroutine"
  - "The AI counts line does not say the listed accounts joined, because established members can be among them"
  - "Added models.LockdownTriggerKinds() so the surface table test fails for a kind it does not cover"
  - "lockdownMergeFlooders generalised into lockdownMergeSnapshot with thin wrappers instead of a copy"

requirements-completed: [RAID-03, RAID-05]

coverage:
  - id: D1
    description: "Five /aispam spam verdicts from three accounts within 120 s lock a supergroup through raidFire and startLockdownRow, recorded as ai_burst with count 5, window 120, percent 0, the bot as starter and an empty reason"
    requirement: RAID-03
    verification:
      - kind: integration
        ref: "alita/modules/raid_ai_burst_test.go#TestRaidAIBurstFires/five_verdicts_from_three_accounts_lock"
        status: pass
    human_judgment: false
  - id: D2
    description: "Four verdicts, five verdicts from two accounts, keep verdicts and a basic group never lock a group; /aispam deletes and calls TypeSafe exactly as before"
    requirement: RAID-05
    verification:
      - kind: integration
        ref: "alita/modules/raid_ai_burst_test.go#TestRaidAIBurstFires"
        status: pass
      - kind: unit
        ref: "alita/modules/raid_ai_burst_test.go#TestRaidAIVerdictScript"
        status: pass
    human_judgment: false
  - id: D3
    description: "Alert, /lockdownstatus, hourly reminder, public notice, cannot-lock warning and /lockdown during the lockdown all name the AI trigger with its numbers, in all 7 languages"
    requirement: RAID-03
    verification:
      - kind: integration
        ref: "alita/modules/raid_ai_burst_test.go#TestLockdownTriggerKindsAllSurfaces"
        status: pass
      - kind: integration
        ref: "alita/modules/raid_ai_burst_test.go#TestRaidAIAlert"
        status: pass
      - kind: unit
        ref: "alita/i18n TestLockdownLocaleKeys"
        status: pass
    human_judgment: false
  - id: D4
    description: "The alert sample lists the accounts behind the counted verdicts (an established member among them) and Ban N recent joiners bans them with the Phase 5 exclusions"
    requirement: RAID-03
    verification:
      - kind: integration
        ref: "alita/modules/raid_ai_burst_test.go#TestRaidAIAlert/spammers_listed"
        status: pass
      - kind: integration
        ref: "alita/modules/raid_ai_burst_test.go#TestRaidAIAlert/ban_the_spammers"
        status: pass
    human_judgment: false
  - id: D5
    description: "The translated wording of the four new locale keys reads naturally in es, fr, hi, id, pt and ru"
    verification: []
    human_judgment: true
    rationale: "Placeholders and key parity are tested, but whether the machine-written translations read naturally to native speakers is not something a test asserts"

duration: 21min
completed: 2026-10-29
status: complete
---

# Phase 7 Plan 01: AI burst tracer Summary

**A burst of 5 /aispam spam verdicts from 3 accounts in 120 s locks a supergroup through the shared raidFire path as trigger kind ai_burst, and every lockdown surface names the AI trigger with its numbers and lists the accounts behind it.**

## Performance

- **Duration:** 21 min
- **Started:** 2026-10-29T17:36:03Z
- **Completed:** 2026-10-29T17:56:58Z
- **Tasks:** 3
- **Files modified:** 15 (2 created, 13 modified)

## Accomplishments

- `alita/modules/raid_ai_count.go`: a Redis verdict counter (`alita:raid:{chat}:av` and `:aa`, one Lua script, `ZADD NX`, fixed 120 s window, 3-account floor, cap, 5 minute TTL) and the rule `raidAICounts`. `processAISpam` reports each delete verdict through `raidAIRecordVerdict`; a keep verdict never counts, and the `/aispam` deletion and its single TypeSafe call per message are unchanged.
- `ai_burst` is a new trigger kind (no migration: the column is 16 characters with no CHECK). `raidFire`, the latch, the active check and the quiet period are unchanged, so exactly one lockdown and one alert still come from `uk_chat_lockdowns_active`, `lockdown.Start` and `BeginAlert`.
- Every site that switches on the kind learned `ai_burst`: `lockdownTriggerSentence`, `raidPostNotice`, `lockdownStartedAutomatically`, `lockdownAlertTriggerLine`, `lockdownRenderSample`. `TestLockdownTriggerKindsAllSurfaces` renders each kind in `models.LockdownTriggerKinds()` on all of them.
- The accounts behind the counted verdicts are snapshotted (`alita:raid:{chat}:aispammers`) by `raidFire` before the start and merged into the alert's burst rows, so the sample lists them (an established member included, timed by its verdict) and Ban N recent joiners can ban them.
- Four locale keys in all 7 files; AGENTS.md rules for the keys, "AI never decides alone", the surfaces and the rolling-deploy caveat.

## Task Commits

1. **Task 1: five /aispam verdicts from three accounts lock the group, and the alert names the AI trigger**
   - `e92a3d3` test (RED)
   - `f7e8ad4` feat
2. **Task 2: every surface reads an AI lockdown as automatic and names its trigger**
   - `55a8a1a` refactor (`models.LockdownTriggerKinds()`, so the table test can enumerate kinds)
   - `1c5d7bd` test (RED)
   - `f93c147` feat
3. **Task 3: the alert lists the accounts behind the verdicts, and Ban N recent joiners can ban them**
   - `c9be95f` test (RED)
   - `64f57c3` feat

**Plan metadata:** the docs commit that adds this file.

## Files Created/Modified

- `alita/modules/raid_ai_count.go` - verdict counter script, `raidAICounts`, `raidAIItemKey`, `raidAIRecordVerdict`, spammer snapshot script and `raidAISpammersLookup` seam
- `alita/modules/raid_ai_burst_test.go` - `TestRaidAIVerdictScript`, `TestRaidAIBurstFires`, `TestLockdownTriggerKindsAllSurfaces`, `TestRaidAIAlert`; helpers `newAIBurstEnv`, `enableAI`, `spamFrom`, `textsFrom`, `settleAI`, `aiBurstLockdown`, `aiRequests`, `staffChatOf`, `aiSpammers`, `aiRaidFrom`
- `alita/modules/aispam.go` - `aispamTask.chat`/`itemKey`, `aispamBar`, `aispamIsDelete`, the delete-verdict report
- `alita/modules/raid_trigger.go` - `ai_burst` in `lockdownTriggerSentence`, `raidPostNotice` and `raidFire`'s snapshot step
- `alita/modules/lockdown_alert.go` - `ai_burst` in the trigger line, counts line and burst snapshot; `lockdownMergeSnapshot` with two wrappers
- `alita/modules/lockdown_status.go` - `lockdownStartedAutomatically` knows `ai_burst`
- `alita/db/models/lockdown.go` - `LockdownTriggerAIBurst`, `LockdownTriggerKinds()`
- `locales/*.yml` - `lockdown_trigger_ai_burst`, `lockdown_alert_trigger_ai_burst`, `lockdown_alert_counts_ai`, `lockdown_auto_started_ai_burst`
- `AGENTS.md` - the rules above

## Decisions Made

- `raidAIRecordVerdict` calls `raidFire` synchronously in the `/aispam` worker, as the plan decided; plan 07-14 moves it onto a tracked goroutine with its own drain.
- `models.LockdownTriggerKinds()` is a function, not an exported variable, so no mutable global is added and the surface table has something to enumerate.
- The test helper `newAIBurstEnv(t) *lockdownEnv` keeps the plan's signature; the request counter of the TypeSafe fake lives in a package variable (`aiSpamRequests`) read through `env.aiRequests()`, because `lockdownEnv` is defined in another test file.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Added `models.LockdownTriggerKinds()`**
- **Found during:** Task 2
- **Issue:** The plan asks for a table test that "fails for a kind added later without these cases", but Go cannot enumerate constants, so the table could not detect a new kind.
- **Fix:** A small function in `alita/db/models/lockdown.go` (a file the plan already lists) lists every kind; the test fails for any kind in it without a case. Committed separately as `55a8a1a`.
- **Files modified:** alita/db/models/lockdown.go
- **Verification:** `TestLockdownTriggerKindsAllSurfaces` passes; removing a case would fail it.
- **Committed in:** 55a8a1a

**2. [Test shape] RED for Task 3 is a build failure**
- **Found during:** Task 3
- **Issue:** The RED tests reference the `raidAISpammersLookup` seam, which does not exist before the implementation, so the RED commit `c9be95f` fails to compile instead of failing on behaviour. (Task 1 and 2 REDs failed on behaviour.) A mutation check after the implementation (the AI branch of `raidFire`'s snapshot switched off) failed the three snapshot cases, so they do test the behaviour.
- **Committed in:** c9be95f

---

**Total deviations:** 1 auto-fixed (Rule 3), 1 note about RED shape.
**Impact on plan:** None on scope. No file outside the plan's list was touched except nothing: every changed file was already listed.

## Issues Encountered

None. `make lint` cannot run here (golangci-lint is built with Go 1.25 against a Go 1.26 module), so `go vet -tags testtools ./alita/modules ./alita/db/models` and `gofmt -l alita` (both clean) stand in for it.

## Verification Run

- `CGO_ENABLED=1 go test -tags testtools -race -count=1 -run 'AISpam|Raid|Lockdown|Unlockdown' ./alita/modules` - ok (about 137 s)
- `go test -tags testtools ./alita/i18n ./alita/db/models` - ok (includes `TestLockdownLocaleKeys`)
- `CGO_ENABLED=0 go build ./...`, `go vet -tags testtools ./alita/modules ./alita/db/models`, `gofmt -l alita` - clean
- `make check-translations` - all translations present; `make check-docs` - no drift
- `git diff --exit-code go.mod go.sum` - clean (no dependency change)
- The full `make test` was not run here; the orchestrator runs it after the wave. No PostgreSQL was started and this plan adds no PostgreSQL-only guarantee.

## Pending Human Verification

Nothing in this plan could be checked against live services here:

1. **Live TypeSafe and Telegram burst.** In a test supergroup where the bot is an admin with `can_restrict_members` and `ENABLE_AISPAM=true` with a real `TYPESAFE_API_KEY`: run `/aispam on`, then have three non-admin accounts post spam-like texts (for example an offer with a `t.me` link) until 5 are deleted within 2 minutes. Expect the group to lock, the public notice "AI spam ... 5 messages were judged spam within 120 seconds" in the group, and an alert in the linked Staff Group with the AI trigger line and the three accounts listed. Then `/unlockdown` and confirm the lift.
2. **Translations.** A native reader of es, fr, hi, id, pt and ru should read the four new `lockdown_*_ai*` keys (`locales/*.yml`) once for naturalness.

## Next Phase Readiness

- Plans 07-02 onward can build on `ai_burst`, the `:av`/`:aa` layout and the test helpers (`newAIBurstEnv`, `settleAI`, `aiBurstLockdown`). Until plan 07-04 adds the per-chat setting, the rule uses the D-05 default of 5 (`raidAIDefaultThreshold`), on in every group.
- Plan 07-14 moves the `raidFire` call onto a tracked goroutine; `settleAI` is the place to extend for that drain.

---
*Phase: 07-ai-assisted-raid-detection*
*Completed: 2026-10-29*

## Self-Check: PASSED

Created files exist (`alita/modules/raid_ai_count.go`, `alita/modules/raid_ai_burst_test.go`); commits `e92a3d3`, `f7e8ad4`, `55a8a1a`, `1c5d7bd`, `f93c147`, `c9be95f`, `64f57c3` are in `git log`; the acceptance greps of the three tasks pass.
