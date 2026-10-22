---
phase: "2"
slug: "staff-actions-across-groups"
# status lifecycle: draft (seeded by plan-phase) → validated (set by validate-phase §6)
# audit-milestone §5.5 distinguishes NOT-VALIDATED (draft) from PARTIAL (validated + nyquist_compliant: false) (#2117)
status: validated
nyquist_compliant: true
wave_0_complete: true
created: "2026-10-05"
---

# Phase 2 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test + testify assertions; real fixtures (SQLite via `internal/testdb.Run`, miniredis, hand-written `gotgbot.BotClient` fakes) |
| **Config file** | none; every test file starts with `//go:build testtools` |
| **Quick run command** | `go test -tags testtools -race -count=1 -run '^TestStaffAction' ./alita/modules` |
| **Full suite command** | `make test` (sandbox: `go test -tags testtools -race -count=1 ./alita/modules ./alita/utils/ratelimit ./alita/utils/extraction ./alita/db/user ./alita/i18n`) |
| **Estimated runtime** | `make test` ~180 s; `alita/modules` package ~120 s; quick run ~56 s (measured 2026-10-05 after wave 7) |

---

## Sampling Rate

- **After every task commit:** Run the quick run command for the touched package
- **After every plan wave:** Run the full suite command
- **Before `/gsd-verify-work`:** `make test`, `make lint`, `make check-translations`, `make check-docs` must be green
- **Max feedback latency:** 60 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 2-01-01 | 01 | 1 | STAFF-01, STAFF-02, STAFF-03, STAFF-04, STAFF-05, STAFF-07 | T-02-01, T-02-02, T-02-03 | Live getChatMember per group; only the issuer's Confirm runs; callback data carries only a token | unit + miniredis | `go test -tags testtools -race -count=1 -run '^TestStaffActionTracer$\|^TestStaffActionSkipsWhereIssuerNotAdmin$' ./alita/modules` | ✅ | ✅ green |
| 2-01-02 | 01 | 1 | STAFF-03, STAFF-05, STAFF-07, PLAT-01 | T-02-02, T-02-04, T-02-07 | Non-issuer taps refused; every skip/fail branch makes no write; Redis required | unit + miniredis | `go test -tags testtools -race -count=1 -run '^TestStaffAction(Cancel\|CardIssuerOnly\|CardTapAnswers\|ConfirmAborts\|PerGroupGates\|StaffChatNeverActedOn\|RedisRequired\|NoLinks\|ParseHints)$' ./alita/modules` | ✅ | ✅ green |
| 2-01-03 | 01 | 1 | STAFF-06, STAFF-13 | T-02-05, T-02-08 | Anonymous senders refused before any lookup; non-staff /ban identical to baseline | dispatcher | `go test -tags testtools -race -count=1 -run '^TestStaffAction(NonStaffUnchanged\|StaffGroupRoutesToCard\|Anonymous\|ModuleOrder\|StaleGatePassesThrough\|CommandNamesExact)$' ./alita/modules` | ✅ | ✅ green |
| 2-02-01 | 02 | 2 | STAFF-07 (research flag) | T-02-10 | restrictChatMember only to member/restricted; unban(false) only for kick; unban always only_if_banned | table-driven | `go test -tags testtools -race -count=1 -run '^TestStaffActionDecision\|^TestStaffActionEndsLater$' ./alita/modules` | ✅ | ✅ green |
| 2-02-02 | 02 | 2 | STAFF-01, STAFF-05, STAFF-13 | T-02-10, T-02-11, T-02-12 | Stateful fake proves no ban is lifted by mute/unmute/kick | unit + dispatcher | `go test -tags testtools -race -count=1 -run '^TestStaffAction(FiveActions\|NeverLiftsBan\|BanOverMute\|MuteNeverShortens\|KickClearsMute\|UnmuteNonMember\|HeaderDuration\|NonStaffUnchanged\|Anonymous)$' ./alita/modules` | ✅ | ✅ green |
| 2-03-01 | 03 | 3 | STAFF-01, STAFF-03 | T-02-13, T-02-14 | Strict lowercase duration grammar with overflow checks | unit | `go test -tags testtools -count=1 ./alita/utils/extraction` and `go test -tags testtools -race -count=1 -run '^TestStaffActionParse' ./alita/modules` | ✅ | ✅ green |
| 2-03-02 | 03 | 3 | STAFF-01, STAFF-02, STAFF-03, STAFF-13 | T-02-13 | until_date fixed once at Confirm; timed actions never shorten | unit + dispatcher | `go test -tags testtools -race -count=1 -run '^TestStaffAction(TimedBan\|TimedMute\|TimedNeverShortens\|OverLimitIsPermanent\|TbanTmuteNeedDuration\|DurationTokenRules\|NonStaffUnchanged\|Anonymous)$' ./alita/modules` | ✅ | ✅ green |
| 2-04-01 | 04 | 4 | STAFF-01 | T-02-15 | Lookup errors never read as "never seen" | repo (SQLite) | `go test -tags testtools -race -count=1 -run '^TestFindUsersByUsername' ./alita/db/user` | ✅ | ✅ green |
| 2-04-02 | 04 | 4 | STAFF-01, STAFF-03, STAFF-06, STAFF-13 | T-02-15, T-02-17, T-02-18 | Ambiguous/unknown usernames refused; variants never act in a Staff Group | unit + dispatcher | `go test -tags testtools -race -count=1 -run '^TestStaffAction(Parse\|Username\|TextMention\|BareReplyHint\|RefusedVariants\|NonStaffUnchanged\|Anonymous)' ./alita/modules` | ✅ | ✅ green |
| 2-05-01 | 05 | 5 | STAFF-03, PLAT-01 | T-02-19, T-02-23 | Exactly one fan-out per card across taps, timers and instances | miniredis + concurrency | `go test -tags testtools -race -count=1 -run '^TestStaffAction(CardExpiresByTimer\|CardExpiryBoundary\|ConfirmExactlyOnce\|ConfirmAcrossReplicas\|TimerSkipsCancelledCard)$' ./alita/modules` | ✅ | ✅ green |
| 2-05-02 | 05 | 5 | STAFF-03, STAFF-04, PLAT-01 | T-02-20, T-02-21, T-02-22 | Abort on changed links; one run per target | miniredis | `go test -tags testtools -race -count=1 -run '^TestStaffAction(LinksChangedAborts\|TargetLock\|TargetLockReleased\|TargetLockForeignRelease\|OwnCardDoubleTap)$' ./alita/modules` and `make generate-docs && make check-docs && make check-translations` | ✅ | ✅ green |
| 2-06-01 | 06 | 6 | STAFF-12, PLAT-01 | T-02-24, T-02-25, T-02-27 | Fleet-wide spacing, shared retry_after block, capped waits, local fallback | unit + miniredis | `go test -tags testtools -race -count=1 ./alita/utils/ratelimit` | ✅ | ✅ green |
| 2-06-02 | 06 | 6 | STAFF-08, STAFF-12, PLAT-01 | T-02-24, T-02-26 | Every fan-out call paced; D-19 reasons from a live probe; panics reported | unit | `go test -tags testtools -race -count=1 -run '^TestStaffAction(RetriesOn429\|RateLimitedFails\|FailureClassification\|LookupClassification\|RecheckPaced\|RecheckRateLimited\|WorkersBounded\|WorkerPanicReported)$' ./alita/modules` | ✅ | ✅ green |
| 2-07-01 | 07 | 7 | STAFF-08 | T-02-28 | Only done lines collapse; overflow goes to continuation; UTF-16 measured after escaping | unit (pure) | `go test -tags testtools -race -count=1 -run '^TestStaffActionSummary' ./alita/modules` | ✅ | ✅ green |
| 2-07-02 | 07 | 7 | STAFF-08, STAFF-12 | T-02-29, T-02-30, T-02-31 | Final summary always delivered (retry, fallback); shutdown drain before DB close | unit | `go test -tags testtools -race -count=1 -run '^TestStaffAction(ProgressBatched\|FinalEditRetriesOn429\|FinalFallsBackToNewMessage\|NotModifiedIsSuccess\|OverflowContinuation\|AllSkippedTally)$\|^TestStopStaffActionsFinalizes$' ./alita/modules` | ✅ | ✅ green |
| 2-08-01 | 08 | 8 | STAFF-08, STAFF-12 | T-02-32, T-02-33 | Final edit, fallback and each continuation get their own delivery budget, so a 60 s retry_after cannot lose the summary | unit + virtual clock | `go test -tags testtools -race -count=1 -run '^TestStaffAction(FinalEditLongRetryAfter\|FinalPartsOwnBudget\|FinalEditRetriesOn429\|FinalFallsBackToNewMessage\|NotModifiedIsSuccess\|OverflowContinuation)$\|^TestStopStaffActionsFinalizes$' ./alita/modules` | ✅ | ✅ green |
| 2-08-02 | 08 | 8 | STAFF-08, STAFF-12 | T-02-34, T-02-35 | Progress edits pause during a 429 hold; the final edit waits it out; retry_after clamped before it is multiplied | unit + virtual clock | `go test -tags testtools -race -count=1 -run '^TestStaffAction(ProgressBacksOffAfter429\|FinalEditWaitsForHold\|ProgressBatched)$\|^TestStaffRetryAfterClamp$' ./alita/modules` | ✅ | ✅ green |
| 2-09-01 | 09 | 8 | STAFF-12, PLAT-01 | T-02-27, T-02-36 | A slot beyond MaxWait fails the call at once as "rate limited", without calling Telegram or taking the slot | unit + miniredis | `go test -tags testtools -race -count=1 -run '^TestStaffActionBlockAboveMaxWaitFailsFast$' ./alita/modules` | ✅ | ✅ green |
| 2-09-02 | 09 | 8 | STAFF-12, PLAT-01 | T-02-37, T-02-38 | Redis-down fallback refuses the same way; retry_after overflow clamped; boundary pinned | unit + miniredis | `go test -tags testtools -race -count=3 -run '^TestTelegramPacer' ./alita/utils/ratelimit` | ✅ | ✅ green |
| 2-10-01 | 10 | 9 | PLAT-01, STAFF-12 | T-02-39, T-02-40 | The run's coordinator renews its own target lock by compare-and-set while the fan-out runs | miniredis | `go test -tags testtools -race -count=1 -run '^TestStaffActionTargetLockRenewedDuringRun$' ./alita/modules` | ✅ | ✅ green |
| 2-10-02 | 10 | 9 | PLAT-01, STAFF-12 | T-02-41, T-02-42 | A vanished lock is re-taken with the run's token; another card's lock is never touched; the lock is released after delivery | miniredis | `go test -tags testtools -race -count=1 -run '^TestStaffActionTargetLock\|^TestStaffActionOwnCardDoubleTap$\|^TestStaffActionConfirm' ./alita/modules` and `make test` | ✅ | ✅ green |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [x] Stateful `gotgbot.BotClient` fake for staff actions (per-`(chat,user)` status, `until_date`, `is_member`, `can_send_messages`, `can_restrict_members`; ban/restrict/unban as status transitions; scripted 429 sequences; `editMessageText` failure and "not modified" injection; call-order recording): task 2-01-01 (`staff_action_fake_test.go`), extended with delays, panics and in-flight counting in task 2-06-02
- [x] miniredis helper for staff-action tests (`withMiniredis` and `cache.SetRedisClientForTest` exist; `newStaffActionEnv` uses them): task 2-01-01
- [x] `extraction.ParseDurationToken` plus tests: task 2-03-01
- [x] `user.FindUsersByUsername` plus tests in `alita/db/user`: task 2-04-01
- [x] Real-dispatcher harness test loading `LoadBans`, `LoadMutes` and `LoadStaffActions` in registry order: task 2-01-03 (`staff_action_dispatch_test.go`)
- Framework install: none

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Mute fan-out never lifts a real ban (research flag; tdlib's final status-replacement step is inferred) | STAFF-04 | Requires a live Telegram group; fakes model the inferred server semantics only | In a throwaway group, ban a test account, run a staff `/mute` and `/unmute` from the Staff Group, and confirm the account is still banned |
| Behavior under real Telegram flood control across many groups and replicas | STAFF-12, PLAT-01 | Real rate-limit numbers are unpublished; tests script 429 responses and use a virtual clock | Link several groups, run a staff `/ban`, and confirm the summary finishes with every group marked and no group dropped (human-check in task 2-06-02). Then follow the human-check in task 2-10-02: trigger real 429s, confirm the card is not edited during `retry_after`, every card ends on a final summary, and a second Confirm on the same target answers "target busy" while the first run is going |
| End-to-end live run: card, Confirm, per-group skips, anonymous refusal, 5-minute expiry, unchanged per-group /ban | STAFF-01..08, STAFF-13 | Real clients, real admin rights and the edit cadence cannot be reproduced by the fake | Follow the human-check in task 2-07-02 (the research-flag check is the human-check in task 2-02-02) |

---

## Validation Sign-Off

- [x] All tasks have `<automated>` verify or Wave 0 dependencies
- [x] Sampling continuity: no 3 consecutive tasks without automated verify
- [x] Wave 0 covers all MISSING references
- [x] No watch-mode flags
- [x] Feedback latency < 60s (quick run measured at 59.5 s after wave 7 — at the limit; per-task -run patterns finish well under it)
- [x] `nyquist_compliant: true` set in frontmatter

**Approval:** validated 2026-10-05 (validate-phase, after all 7 plans merged); re-validated 2026-10-05 after gap-closure plans 02-08 to 02-10 merged

## Validation Audit 2026-10-05

Every per-task automated command was run against the merged phase head. Each exact-name pattern matched exactly the tests it names, and all passed under `-race`.

| Metric | Count |
|--------|-------|
| Gaps found | 0 |
| Resolved | 0 |
| Escalated | 0 |

Notes:
- `make lint` could not run in this sandbox: the installed golangci-lint was built with go1.25 and the module targets go1.26.0. Each plan ran `gofmt -l` and `go vet -tags testtools ./alita/...` instead. `make test`, `make check-translations` and `make check-docs` passed.
- Two intermittent tests were fixed at the root after wave 7:
  - `TestLogUsersPersistsSenderChatAndReplyUsers` had a data race between tests swapping the throttle maps and the sweep/async goroutines. It was reproduced 4 times in a 100 s loop and showed 0 times after the fix.
  - `TestTelegramPacerFleetSpacing` had a neighbour-gap assertion that broke on timer jitter. It now uses a jitter-proof lower bound. 50 `-race` runs pass, and a per-replica-only mutation fails it.
- Manual-only rows are unchanged. They need a live Telegram bot and groups.

## Validation Audit 2026-10-05 (gap closure)

Audited after plans 02-08, 02-09 and 02-10 merged (closing WR-01 and WR-02). Six per-task rows were added (2-08-01 to 2-10-02). Every new automated command was run against the merged head. Each named test ran and passed under `-race`: 6 new staff tests, `TestStaffRetryAfterClamp`, the 3 `TestStaffActionTargetLockRenewedDuringRun` subtests, and the 3 new `TestTelegramPacer*` tests at `-count=3`. The combined targeted run took 17 s.

| Metric | Count |
|--------|-------|
| Gaps found | 0 |
| Resolved | 0 |
| Escalated | 0 |

Notes:
- `make test` passed after each gap-closure wave (60 packages ok, 0 failures).
- The flood-control manual-only row now also points to the human-check in task 2-10-02.
