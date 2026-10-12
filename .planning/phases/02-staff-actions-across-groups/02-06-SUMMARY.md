---
phase: 02-staff-actions-across-groups
plan: 06
subsystem: staff-moderation
status: complete
tags: [telegram, rate-limit, redis, lua, pacer, errgroup, fan-out, i18n]

# Dependency graph
requires:
  - phase: 02-staff-actions-across-groups
    provides: "runStaffActionFanOut / runStaffActionInGroup / executeStaffCall (02-01..02-03), decideStaffAction table, card lifecycle and per-target lock (02-05), staffActionEnv harness and the stateful staffActionFake"
provides:
  - "ratelimit.TelegramPacer: Redis slot reservation (alita:staff:pace:next), shared retry_after block (alita:staff:pace:block), bounded retry wrapper Do, ErrRateLimited, RetryAfterSeconds, local fallback"
  - "Every staff fan-out Telegram call goes through staffPaced: issuer and target getChatMember, getChat, the write call, the bot-rights probe and the owner recheck's getChatAdministrators"
  - "Four groups at once (errgroup SetLimit(4)); a panicking worker's group is reported 'failed: internal error', never dropped"
  - "D-19 failure reasons from a live, paced bot-rights probe: bot isn't an admin, bot lacks ban rights, group not found, rate limited, internal error"
  - "staffOwnerPass.call seam and newPacedStaffOwnerPass, leaving the panel, sweeper and watchers untouched"
affects: [02-07, phase-03-audit-undo]

# Actuals (#2632)
actuals:
  tokens: 31000
  tasks: 2
  commits: 4

# Commit measurement (#3968)
plan_head_before: e843633950f3fdd79823fbb70d19a659f5f08aaf
plan_head_after: fd5d57be6b18e04342a9d942b696b47571726318
commits: 4

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Lua slot-reservation pacer: each caller reserves the next free slot (server TIME, GET next, PTTL block) and sleeps the returned milliseconds, so the fleet as a whole stays one interval apart"
    - "Shared retry_after block extended by Lua compare on PTTL; every replica's next reservation honours it"
    - "Pre-filled pending results slice plus a post-Wait sweep, so a recovered worker panic still yields a failed line"
    - "A closure-style call seam (staffOwnerPass.call) that only the fan-out sets, so other callers keep calling CheckOwner directly"

key-files:
  created:
    - alita/utils/ratelimit/telegram_pacer.go
    - alita/utils/ratelimit/telegram_pacer_test.go
    - alita/modules/staff_action_scale_test.go
  modified:
    - alita/modules/staff_recheck.go
    - alita/modules/staff_action_run.go
    - alita/modules/staff_action_decide.go
    - alita/modules/staff_action_summary.go
    - alita/modules/staff_action_fake_test.go
    - locales/en.yml
    - locales/es.yml
    - locales/fr.yml
    - locales/hi.yml
    - locales/id.yml
    - locales/pt.yml
    - locales/ru.yml
    - AGENTS.md

key-decisions:
  - "The pacer stays a leaf utility: it imports the utils cache package for the Redis client and nothing from alita/modules; the staff module owns the keys and the tuning through the staffActionPacer variable"
  - "A 429 is detected only through errors.As on *gotgbot.TelegramError (Code 429 with ResponseParams.RetryAfter > 0); a retry_after equal to MaxWait is waited, one above it fails the group at once but still extends the shared block"
  - "The write-failure probe and the lookup-failure probe run only for a Telegram 400 or 403; a 5xx, a timeout or a cancelled context is never turned into 'bot isn't an admin'"
  - "A bot whose live status is creator after a 400/403 is not blamed: the failure is shown as Telegram's own text"
  - "Summary edits are not paced through the shared budget (RESEARCH Q3): one coordinator per card owns them (plan 02-07)"

patterns-established:
  - "Test pacer: withFastStaffPacer installs a 1 ms interval, 5 ms retry_after unit pacer as staffActionPacer for every staffActionEnv test, restored by t.Cleanup"
  - "Fake helpers setDelay, setPanic, setBotRole, maxConcurrent: delays and panics happen outside every lock of the fake"

requirements-completed: [STAFF-08, STAFF-12, PLAT-01]

duration: 55min
completed: 2026-10-05
---

# Phase 2 Plan 06: Paced, Bounded Staff Fan-Out Summary

**One Redis-backed pacer shares a call budget and a retry_after block across replicas; the staff fan-out runs four groups at a time through it, waits out every 429, and explains each failure from a live probe of the bot's rights.**

## Performance

- **Duration:** about 55 min
- **Tasks:** 2 of 2 (both TDD: RED commit, then GREEN commit)
- **Files created:** 3, **modified:** 13

## Accomplishments

- `ratelimit.TelegramPacer` (leaf package, no import of the modules package). Do waits for a Redis slot, runs the call, and on a 429 extends the shared block and re-invokes the same closure, so parameters such as `until_date` are identical on every attempt. 3 retries, a 60 s cap per wait, 100 ms fleet-wide interval by default.
- Redis failure mid-run falls back to a local interval with one warn per minute, and then stays local for 5 s so an outage does not add a dial timeout to every call.
- Staff fan-out: every Telegram call of a run is paced (both `getChatMember` lookups, `getChat` and the restrict for unmute as two paced calls, the write call, the probe, and the owner recheck through `newPacedStaffOwnerPass`). `recheckLink` is unchanged; the seam is a field on `staffOwnerPass` that only the fan-out sets.
- Four workers (`errgroup.SetLimit(staffActionWorkers)`), each with `defer error_handling.RecoverFromPanic("staffActionWorker", "StaffActions")` writing only its own slot; after `Wait` every slot still pending becomes `failed: internal error`.
- D-19 classification by live probe (`chat_status.FetchBotMember`, paced), never by error text. A recheck that is still rate limited reads "rate limited, gave up after retries" and never removes a link.
- Five locale keys in all 7 locale files; AGENTS.md Data bullet for `alita:staff:pace:next` and `alita:staff:pace:block`.

## Task Commits

1. **Task 1 RED: failing pacer tests** - `1921365` (test)
2. **Task 1 GREEN: fleet-wide pacer** - `7de61c3` (feat)
3. **Task 2 RED: failing fan-out tests and fake helpers** - `4a080d5` (test)
4. **Task 2 GREEN: paced, bounded fan-out with failure reasons** - `fd5d57b` (feat)

No REFACTOR commit was needed.

## TDD Gate Compliance

RED then GREEN for both tasks, in order.

- Task 1 RED: a skeleton `telegram_pacer.go` (types, `Do` returning "not implemented", `RetryAfterSeconds` returning false) so the tests compile and fail on their own assertions. All 9 target tests failed on the planned behaviour (for example "Do() error = telegram pacer: not implemented, want nil", "RetryAfterSeconds(429 retry_after=3) = (0, false), want (3, true)"). No build errors, no zero-test discovery.
- Task 2 RED: no production stubs were needed. All 8 target tests failed on assertions (for example "line for Group A = ... fail-telegram ..., want failed with staff_act_fail_rate_limited", "most requests in flight at once = 1, want between 2 and 4", "line for Group A = ⏳ Group A, want a done line" for the panic case).
- `gsd_run check tdd-red-evidence` could not be run: the gsd-tools CLI fails inside this worktree (missing `vendor/re2js.cjs`). The RED evidence above was verified by hand from the test output.

## Verification run

- `go test -tags testtools -race -count=1 ./alita/modules ./alita/utils/ratelimit ./alita/i18n`: ok (modules about 112 s whole package, `-run '^TestStaff'` about 72 s)
- The 8 new fan-out tests under `-race -count=3`: ok. The pacer tests under `-race -count=5`: ok.
- Existing Phase 1 panel, sweeper, watcher and ownership tests pass untouched (they call `newStaffOwnerPass()`, whose behaviour is unchanged).
- `make check-translations`: all translations present. `TestStaffLocaleKeys`: ok. `make check-docs`: no drift.
- `CGO_ENABLED=0 go build ./...`: ok. `go vet -tags testtools ./alita/...`: clean. `gofmt -l` is clean for every file touched. `git diff go.mod go.sum`: empty.
- `make lint` was not run: golangci-lint in this sandbox is built with go1.25 against module go1.26.0, so `gofmt -l` and `go vet` stand in for it.
- The `<human-check>` (five or more real test groups, optionally a second replica) is not done and remains for the owner.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Slot key TTL computed in the script, not passed as a constant**
- **Found during:** Task 1
- **Issue:** the RESEARCH script writes the next-slot key with a fixed `PX` TTL. With a long shared block (up to 60 s) or a deep queue, the slot recorded for later callers is further ahead than any fixed TTL, so the key could expire and let later callers jump the queue.
- **Fix:** the script sets `PX slot - now + interval + slack` (slack 1000 ms passed as ARGV[2]). The probed behaviour (waits 0, interval, 2 x interval, block honoured) is unchanged.
- **Files modified:** alita/utils/ratelimit/telegram_pacer.go
- **Commit:** 7de61c3

**2. [Rule 2 - Missing critical functionality] 5 s local backoff after a Redis failure**
- **Found during:** Task 1 (the Redis-down test took 1.2 s)
- **Issue:** with Redis down, go-redis retries dialling for about 0.4 s per command, so every paced call (and every block write) paid that delay even though the run was meant to continue locally.
- **Fix:** after a Redis failure the pacer stays on its local interval for 5 s before trying Redis again; the warn is still logged at most once a minute.
- **Files modified:** alita/utils/ratelimit/telegram_pacer.go
- **Commit:** 7de61c3

**3. [Rule 2 - Missing critical functionality] A panic inside a paced owner check cannot read as "owner matches"**
- **Found during:** Task 2
- **Issue:** `staffOwnerOutcome`'s zero value is `OwnerMatch`. If a worker panicked inside the memoised `CheckOwner` (a `sync.Once` is then spent), every other worker sharing the pass would have read an owner match with no error.
- **Fix:** on the paced path only, the entry is set to OwnerUnknown with an error before `run` executes. The unpaced path (`call == nil`) is exactly the old code.
- **Files modified:** alita/modules/staff_recheck.go
- **Commit:** fd5d57b

### Interpretation notes (not deviations)

- The plan says the classification probe runs "on a 400 or 403" for write failures and "otherwise the same probe" for lookup failures. I probed only on 400/403 for both, so a 5xx, timeout or cancelled context stays "could not check members" or "Telegram error" and is never relabelled as a rights problem.
- The fake's `setDelay` and `setPanic` are keyed by (method, chat) like `script`, so the delay in `TestStaffActionWorkersBounded` applies to both the issuer and the target `getChatMember` of a group; it only lengthens the overlap that the 2 to 4 bound is measured over.
- The 429 and pacer tests rely on miniredis TTLs being frozen without `FastForward`: a block set in a test stays active for the rest of that test's Redis, which only ever adds a 5 ms wait. Assertions are written so they hold for real expiry too. No test uses a real production-length sleep.

### Tooling limits in this worktree

- `gsd_run` (the gsd-tools CLI) fails here, so the RED-evidence verifier, the plan commit ledger and the cwd-drift sentinel files were not written. Commit counts were measured with `git rev-list --count` from the base commit, and `.planning` files are committed with plain `git`. STATE.md and ROADMAP.md are not touched (the orchestrator owns them).

## Known Stubs

None. The pacer skeleton in the RED commit was replaced in the GREEN commit; nothing hardcoded flows to the UI.

## Threat Flags

None. The two new Redis keys (`alita:staff:pace:next`, `alita:staff:pace:block`) are operational keys outside the `alita:cache:` prefix and carry no user data. Mitigations for T-02-24 to T-02-27 are covered by TestTelegramPacerFleetSpacing, TestStaffActionWorkersBounded, TestTelegramPacerSharedBlock, TestStaffActionFailureClassification and TestTelegramPacerRetryAfterCap.

## Issues Encountered

None beyond the deviations above.

## Next Phase Readiness

Plan 02-07 (batched summary edits) can build on `staffPaced` being intentionally unused for edits: its coordinator owns edit pacing and handles its own 429s. The final summary is still delivered by `deliverStaffActionSummary` from the run goroutine.

## Self-Check: PASSED

- Files present: `alita/utils/ratelimit/telegram_pacer.go`, `alita/utils/ratelimit/telegram_pacer_test.go`, `alita/modules/staff_action_scale_test.go`
- Commits present: 1921365, 7de61c3, 4a080d5, fd5d57b
- Acceptance greps: `staffActionPacer.Do(` in staff_action_run.go, `func newPacedStaffOwnerPass` in staff_recheck.go, `SetLimit(staffActionWorkers)` and `RecoverFromPanic("staffActionWorker", "StaffActions")` in staff_action_run.go, `alita:staff:pace:block` in AGENTS.md, `func (p *TelegramPacer) Do(ctx context.Context, call func(context.Context) error) error` and `var ErrRateLimited` in the pacer, and no `Alita_Robot/alita/modules` import in the pacer.
