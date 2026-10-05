---
phase: 02-staff-actions-across-groups
plan: 05
subsystem: staff-moderation
tags: [telegram, gotgbot, redis, lua, confirm-card, expiry-timer, distributed-lock, i18n]

# Dependency graph
requires:
  - phase: 02-staff-actions-across-groups
    provides: "Redis confirm card with Lua compare-and-set (02-01), fan-out run (02-01..02-04), staffActionEnv test harness"
provides:
  - "Expiry timer: an unconfirmed card is edited to Expired with its buttons removed shortly after 5 minutes (scheduleStaffActionExpiry, expireStaffActionCard, staffActionExpirySlack)"
  - "Links signature: the card stores a sha256 signature of the sorted linked group IDs and Confirm aborts when the set changed, even at the same count (staffLinksSignature, card field LinksSig / hash field links_sig)"
  - "Per-target fan-out lock alita:staff:lock:target:<id> (SET NX, 30 min TTL, compare-and-delete release) so two staff actions on one person never run at once"
  - "/staff help documents the staff actions in all 7 locales; generated docs page regenerated; AGENTS.md records the new key and rules"
affects: [02-06, 02-07, phase-03-audit-undo]

# Actuals (#2632)
actuals:
  tokens: 15600
  tasks: 2
  commits: 4

# Commit measurement (#3968)
plan_head_before: e90a938e736f3f6435967444929a951e2264913f
plan_head_after: 1733458fae27b3911f6063da589fd547bf0c0c7e
commits: 4

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "time.AfterFunc expiry with RecoverFromPanic whose edit is gated by the same Redis compare-and-set a tap uses, so timer and late tap never both edit"
    - "Per-target SET NX lock keyed by card token, released by Lua compare-and-delete, with a deferred release that is handed over to the run goroutine once the run starts"
    - "Own-card detection on a busy lock: the lock value is the card token, so a repeat tap on the same card reads 'already handled' instead of 'target busy'"

key-files:
  created:
    - alita/modules/staff_action_lifecycle_test.go
  modified:
    - alita/modules/staff_action.go
    - alita/modules/staff_action_card.go
    - alita/modules/staff_action_run.go
    - locales/en.yml
    - locales/es.yml
    - locales/fr.yml
    - locales/hi.yml
    - locales/id.yml
    - locales/pt.yml
    - locales/ru.yml
    - docs/src/content/docs/commands/staff/index.md
    - AGENTS.md

key-decisions:
  - "The lock value is the card token; on a busy lock Confirm reads the holder, and a holder equal to the tapping card's own token answers 'already handled' (never 'target busy') without attempting the compare-and-set, so two same-moment taps on one card cannot hand the run to the loser and then lose the lock"
  - "Only a pending, unexpired card owned by the tapping issuer asks for the target lock; any other tap goes straight to the compare-and-set, which answers handled, expired or issuer-only as before"
  - "Lock release is one deferred call in staffActionConfirm for every path that ends before the run starts, and one deferred call in the run goroutine afterwards"
  - "The cross-replica and cancelled-card tests call expireStaffActionCard directly instead of waiting for a short timer, so they are deterministic; the real timer is covered by TestStaffActionCardExpiresByTimer"

patterns-established:
  - "Confirm-time guards run in this order: token, load, chat check, issuer-only and target lock (pending cards), compare-and-set, answer, Staff Group, issuer membership, links, signature, duration, summary edit, run"

requirements-completed: [STAFF-03, STAFF-04, PLAT-01]

coverage:
  - id: D1
    description: "An unconfirmed card expires visibly: edited to Expired with no buttons by the timer, never twice, and a late tap at or after expires_at gets the expired toast while a tap one second before still runs"
    requirement: "STAFF-03"
    verification:
      - kind: integration
        ref: "alita/modules/staff_action_lifecycle_test.go#TestStaffActionCardExpiresByTimer"
        status: pass
      - kind: integration
        ref: "alita/modules/staff_action_lifecycle_test.go#TestStaffActionCardExpiryBoundary"
        status: pass
      - kind: integration
        ref: "alita/modules/staff_action_lifecycle_test.go#TestStaffActionTimerSkipsCancelledCard"
        status: pass
    human_judgment: false
  - id: D2
    description: "Confirm starts exactly one fan-out however many taps or bot instances: 20 rounds of two simultaneous taps, and a card created on one replica confirmed on another sharing only Redis and the database"
    requirement: "PLAT-01"
    verification:
      - kind: integration
        ref: "alita/modules/staff_action_lifecycle_test.go#TestStaffActionConfirmExactlyOnce"
        status: pass
      - kind: integration
        ref: "alita/modules/staff_action_lifecycle_test.go#TestStaffActionConfirmAcrossReplicas"
        status: pass
    human_judgment: false
  - id: D3
    description: "A Confirm whose linked groups changed since the card was shown (added, or one swapped for another at the same count) edits the card to the links-changed abort and acts on nothing"
    requirement: "STAFF-04"
    verification:
      - kind: integration
        ref: "alita/modules/staff_action_lifecycle_test.go#TestStaffActionLinksChangedAborts"
        status: pass
    human_judgment: false
  - id: D4
    description: "Two staff actions on one target never run at once: a busy target answers an alert and leaves the card pending; the lock is compare-and-delete released after the run, after an abort and after an expired tap; the issuer's own double tap gets 'already handled'"
    requirement: "STAFF-03"
    verification:
      - kind: integration
        ref: "alita/modules/staff_action_lifecycle_test.go#TestStaffActionTargetLock"
        status: pass
      - kind: integration
        ref: "alita/modules/staff_action_lifecycle_test.go#TestStaffActionTargetLockReleased"
        status: pass
      - kind: integration
        ref: "alita/modules/staff_action_lifecycle_test.go#TestStaffActionTargetLockForeignRelease"
        status: pass
      - kind: integration
        ref: "alita/modules/staff_action_lifecycle_test.go#TestStaffActionOwnCardDoubleTap"
        status: pass
    human_judgment: false
  - id: D5
    description: "/staff help text documents the staff actions in all 7 locales, with a regenerated docs page and no drift"
    verification:
      - kind: other
        ref: "make generate-docs && make check-docs && make check-translations; go test -tags testtools -run '^TestStaffLocaleKeys$' ./alita/i18n"
        status: pass
    human_judgment: true
    rationale: "Parity, placeholders and docs drift are machine-checked, but whether the non-English help text reads naturally to native speakers is a judgment call"
  - id: D6
    description: "Behaviour on the real Telegram Bot API and against multiple real bot replicas (timer edit on a real message, concurrent taps through real webhook routing)"
    verification: []
    human_judgment: true
    rationale: "Everything is exercised through a hand-written fake BotClient and miniredis; no bot token or second replica exists in the sandbox"

# Metrics
duration: 11min
completed: 2026-10-05
status: complete
---

# Phase 2 Plan 05: Card lifecycle and replica safety Summary

**Staff action cards that expire visibly after 5 minutes, fire exactly once across replicas, abort on a changed linked-group set (sha256 signature of sorted IDs), and serialize on a per-target Redis lock with compare-and-delete release, plus `/staff` help in 7 locales**

## Performance

- **Duration:** 11 min
- **Started:** 2026-10-05T06:29:43Z
- **Completed:** 2026-10-05T06:40:59Z
- **Tasks:** 2
- **Files modified:** 13 (1 created, 12 modified)

## Accomplishments

- An ignored card is edited to "Expired" with its buttons removed about 5 minutes after it was sent. The timer and any late tap race through the same Redis compare-and-set, so only one of them edits, and a card that was confirmed or cancelled (on this replica or another) is never touched by the timer.
- Confirm already started exactly one run through the Lua compare-and-set; this plan proves it with 20 rounds of two simultaneous taps under `-race`, and with a card created by one bot instance and confirmed by a second one that shares only Redis and the database.
- Confirm compares a signature over the sorted linked group chat IDs with a fresh read and aborts with "The linked groups changed since this card was shown ({old} then, {new} now)" when the set differs, including a swap that keeps the count.
- While a run on a target is in flight, Confirm on another card for the same target answers the target-busy alert and leaves that card pending. The lock value is the card token and release is compare-and-delete, so a card whose lock expired never frees a newer card's lock. A repeat tap on the issuer's own card gets "already handled".
- `/staff` help now has a "Staff actions" section (commands, target forms, durations, the 5-minute Confirm, issuer-only buttons, the refused variants, the per-group checks, send as yourself) in en, es, fr, hi, id, pt and ru; the generated Staff commands page was regenerated and `make check-docs` reports no drift.

## Task Commits

1. **Task 1: expiry timer and exactly-once Confirm** - `27b5c46` (test, RED), `cb468ef` (feat, GREEN)
2. **Task 2: links-changed abort, per-target lock, help text, docs** - `f6e8679` (test, RED), `1733458` (feat, GREEN)

**Plan metadata:** committed with this SUMMARY (docs: complete plan)

## TDD Gate Compliance

Both tasks are `tdd="true"`; each has a `test(02-05)` commit before its `feat(02-05)` commit. RED commits carry no-op production stubs so the tests compile and the failure is on a planned assertion, not a build error.

- **Task 1 RED (`27b5c46`):** `go test -tags testtools -race -count=1 -run '^TestStaffAction(CardExpiresByTimer|CardExpiryBoundary|ConfirmExactlyOnce|ConfirmAcrossReplicas|TimerSkipsCancelledCard)$' ./alita/modules` exited 1 with one failure: `TestStaffActionCardExpiresByTimer`: "message 5001 was edited 0 times within 5s, want 1". The other four passed at once (unexpected GREEN) because the Redis compare-and-set from plan 02-01 already gave those guarantees; they are regression guards for the new timer and lock code. No `check tdd-red-evidence` run (the `gsd_run` resolver is not available here); the evidence is recorded above.
- **Task 1 GREEN (`cb468ef`):** the same command passed; `go test -tags testtools -race -count=1 -run '^TestStaff' ./alita/modules` passed (72 s).
- **Task 2 RED (`f6e8679`):** exit 1 with failures `TestStaffActionLinksChangedAborts` (both the added-group and swapped-group cases: the card showed a normal summary with ban lines instead of the abort text), `TestStaffActionTargetLock` ("answer = \"\" (alert false), want the target-busy alert"), `TestStaffActionTargetLockReleased/after_a_Confirm_aborted_by_changed_links` ("card state = done, want aborted") and `TestStaffActionTargetLockForeignRelease` ("target lock holder = \"\", want holder"). `TestStaffActionOwnCardDoubleTap` passed at once (the second tap on a running card already read "handled"); it guards the lock code against turning that into target-busy.
- **Task 2 GREEN (`1733458`):** all ten new tests passed; `-count=5` of the concurrency-sensitive ones also passed; full `go test -tags testtools -race -count=1 ./alita/modules ./alita/i18n` passed (117 s).
- **REFACTOR:** none.

## Files Created/Modified

- `alita/modules/staff_action_card.go` - expiry timer and expiry edit, `staffLinksSignature`, target lock helpers and release script, `takeStaffTargetLock`, links-changed abort and lock handling in `staffActionConfirm`, `LinksSig` card field
- `alita/modules/staff_action.go` - schedules the expiry right after the card is sent; stores the links signature on the card
- `alita/modules/staff_action_run.go` - the run goroutine releases the target lock in a deferred call
- `alita/modules/staff_action_lifecycle_test.go` - ten tests plus the two-replica and gated-ban test helpers
- `locales/*.yml` (7) - `staff_act_abort_links_changed`, `staff_act_target_busy`, extended `staff_help_msg`
- `docs/src/content/docs/commands/staff/index.md` - regenerated
- `AGENTS.md` - lock key, expiry and signature rules

## Decisions Made

See `key-decisions` above. The notable one: the busy-lock path distinguishes "held by this card" from "held by another card" by reading the lock value, because two simultaneous taps on one pending card both pass the pending check and the loser would otherwise be told "target busy" for a card that is actually running.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Same-card concurrent taps would report target-busy**
- **Found during:** Task 2 design, against the Task 1 `TestStaffActionConfirmExactlyOnce` contract ("exactly one tap of the pair gets the staff_act_card_handled toast")
- **Issue:** the plan's flow acquires the lock for any pending card and answers target-busy when it is held. Two simultaneous taps on one pending card would both load state pending; the loser would find the lock held by its own card and answer target-busy, contradicting the exactly-once test and the "own card gets already handled" truth. Letting the loser continue to the compare-and-set instead would let it win the claim if the first tap had not yet claimed, then have the first tap's failed claim release the lock under it.
- **Fix:** on a busy lock Confirm reads the holder; a holder equal to the tapping card's token answers "already handled" and stops, any other holder answers target-busy. The acquirer alone proceeds to the compare-and-set.
- **Files modified:** `alita/modules/staff_action_card.go`
- **Verification:** `TestStaffActionConfirmExactlyOnce` (20 rounds), `TestStaffActionOwnCardDoubleTap`, `TestStaffActionTargetLock`
- **Commit:** `1733458`

**2. [Rule 2 - Missing critical] Lock released on every pre-run exit via one deferred call, and not attempted for cards that cannot win**
- **Found during:** Task 2
- **Issue:** the plan says "every confirm-time abort releases it"; with about ten abort returns in `staffActionConfirm` a per-return call is easy to miss. A lock taken for an already-expired card would also make a busy target mask the "expired" answer.
- **Fix:** one `defer` releases the lock unless the run started (the run goroutine then owns the release); the lock is only requested for a card that is pending and before its `expires_at`.
- **Files modified:** `alita/modules/staff_action_card.go`
- **Verification:** `TestStaffActionTargetLockReleased` (completed run, links-changed abort, expired late tap)
- **Commit:** `1733458`

**3. [Plan wording] Help lines split one command per line**
- **Found during:** Task 2 docs generation
- **Issue:** the plan's help text joined `/mute ... and /tmute ...` and `/unban user and /unmute user` on one line; the docs generator takes everything before the first colon as the command, so the generated page showed a garbled code span.
- **Fix:** one command per line (`/mute`, `/tmute`, `/unban`, `/unmute`) in all 7 locales. Every item the plan listed is still documented.
- **Files modified:** `locales/*.yml`, `docs/src/content/docs/commands/staff/index.md`
- **Commit:** `1733458`

**4. [Test design] Cross-replica and cancelled-card tests call `expireStaffActionCard` directly**
- **Issue:** the plan describes replica A's timer firing later with a short lifetime. A short lifetime makes the test race the confirm on replica B. The tests instead create the card with the default lifetime and invoke the timer's function directly after the other path finished; the real scheduled timer is covered by `TestStaffActionCardExpiresByTimer`.
- **Files modified:** `alita/modules/staff_action_lifecycle_test.go`
- **Commit:** `f6e8679` (test file first appears in `27b5c46`)

**Total deviations:** 2 auto-fixed (1 bug, 1 missing-critical), 2 plan-wording or test-design adjustments. **Impact:** none on scope; all plan truths hold.

## Authentication Gates

None.

## Known Stubs

None. The no-op `staffLinksSignature`, `acquireStaffTargetLock` and `releaseStaffTargetLock` stubs that the RED commits carried were replaced in `1733458`.

## Threat Flags

None. The plan's threat register (T-02-19 to T-02-23) is covered by the tests above; no new network endpoint, auth path or schema change was added. The only new surface is the Redis key family `alita:staff:lock:target:<id>`, which the plan already names.

## Verification Notes

- `make lint` cannot run in this sandbox (golangci-lint here is built with go1.25 against module go1.26.0), so `gofmt -l alita` and `go vet -tags testtools ./alita/...` were run instead. `gofmt` lists only `alita/modules/greetings_command_test.go`, which is already not gofmt-clean on the base commit and was left alone; `go vet` is clean.
- `CGO_ENABLED=0 go build ./...`, `make generate-docs`, `make check-docs` (no drift), `make check-translations` (all present) and `TestStaffLocaleKeys` pass.
- Plan acceptance greps all match: `scheduleStaffActionExpiry` and its `RecoverFromPanic("staffActionExpiry", "StaffActions")` in `staff_action_card.go`, the call in `staff_action.go`, `"alita:staff:lock:target:"` in `staff_action_card.go`, `releaseStaffTargetLock(` in `staff_action_run.go`, `tban` in the docs page, the lock key in `AGENTS.md`.
- A restart loses the in-process expiry timer; the lazy check on any later tap and the hash TTL cover it (accepted in the plan).

## Next Phase Readiness

Ready for 02-06 (shared pacer, continuation messages, shutdown drain). Notes for it: `staffActionRunsWG` still joins the run goroutines, and the target lock is released inside that goroutine's deferred chain, so a shutdown drain that waits on the group also lets the locks go; a drain that cancels `staffActionsCtx` early still releases them because the release is deferred.

## Self-Check: PASSED

- Created and modified files exist on disk: `alita/modules/staff_action_lifecycle_test.go`, `alita/modules/staff_action_card.go`, `alita/modules/staff_action.go`, `alita/modules/staff_action_run.go`, `docs/src/content/docs/commands/staff/index.md`.
- Commits exist: `27b5c46`, `cb468ef`, `f6e8679`, `1733458`; `git rev-list --count e90a938..HEAD` is 4 before this SUMMARY.
- All task acceptance criteria and the plan-level verification commands re-ran green after the final code commit.
