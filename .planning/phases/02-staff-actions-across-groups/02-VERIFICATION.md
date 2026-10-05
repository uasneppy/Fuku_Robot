---
phase: 02-staff-actions-across-groups
verified: 2026-10-05T08:30:00Z
status: gaps_found
score: 4/5 must-haves verified
covered_files:
  - ".planning/phases/02-staff-actions-across-groups/02-01-PLAN.md"
  - ".planning/phases/02-staff-actions-across-groups/02-01-SUMMARY.md"
  - ".planning/phases/02-staff-actions-across-groups/02-02-PLAN.md"
  - ".planning/phases/02-staff-actions-across-groups/02-02-SUMMARY.md"
  - ".planning/phases/02-staff-actions-across-groups/02-03-PLAN.md"
  - ".planning/phases/02-staff-actions-across-groups/02-03-SUMMARY.md"
  - ".planning/phases/02-staff-actions-across-groups/02-04-PLAN.md"
  - ".planning/phases/02-staff-actions-across-groups/02-04-SUMMARY.md"
  - ".planning/phases/02-staff-actions-across-groups/02-05-PLAN.md"
  - ".planning/phases/02-staff-actions-across-groups/02-05-SUMMARY.md"
  - ".planning/phases/02-staff-actions-across-groups/02-06-PLAN.md"
  - ".planning/phases/02-staff-actions-across-groups/02-06-SUMMARY.md"
  - ".planning/phases/02-staff-actions-across-groups/02-07-PLAN.md"
  - ".planning/phases/02-staff-actions-across-groups/02-07-SUMMARY.md"
  - "alita/db/user/repository.go"
  - "alita/modules/staff.go"
  - "alita/modules/staff_action.go"
  - "alita/modules/staff_action_card.go"
  - "alita/modules/staff_action_decide.go"
  - "alita/modules/staff_action_parse.go"
  - "alita/modules/staff_action_run.go"
  - "alita/modules/staff_action_summary.go"
  - "alita/modules/staff_recheck.go"
  - "alita/utils/extraction/extraction.go"
  - "alita/utils/ratelimit/telegram_pacer.go"
  - "main.go"
covered_digest: "v2:sha256:1400edf2715e0735618d202bf358dac1b96d84eaf5ba9a69c89855b0c3a79103"
behavior_unverified: 0
overrides_applied: 0
gaps:
  - truth: "SC4 (STAFF-08, STAFF-12): the issuer's summary reaches a final state and the bot waits and retries when Telegram says to; no group is left unreported"
    status: partial
    reason: "WR-01 confirmed in code. deliverStaffActionFinal runs on a 15 s context (staffActionDeliverTimeout) but a single retry_after wait may be up to 60 s (staffActionRetryAfterCap). On a 429 above ~15 s sleepStaffRetry returns false at the deadline, editStaffActionFinal gives up, and the fallback sendStaffSummaryPart reuses the same expired context, so it makes one send attempt into the same flood and never retries. The card is then marked done while still showing the last progress snapshot (hourglass lines); bans are applied but the issuer never sees the outcome. Progress edits (every 2.5 s, about 24 per minute, not paced, 429 ignored) are what keep the Staff Group under flood pressure while the run is going. No test exercises a retry_after above 15 s (tests shorten the unit to 5 ms)."
    artifacts:
      - path: "alita/modules/staff_action_run.go"
        issue: "staffActionDeliverTimeout = 15s is shorter than staffActionRetryAfterCap = 60s; progress-edit 429s are ignored"
      - path: "alita/modules/staff_action_summary.go"
        issue: "editStaffActionFinal / sendStaffSummaryPart / deliverStaffActionFinal share one short context, so the fallback and continuation parts are starved after a long wait"
    missing:
      - "Delivery budget at least staffActionRetryAfterCap * attempts, or a fresh per-message context for the edit, the fallback send and each continuation part"
      - "Progress ticker backs off after a 429 (skip ticks until retry_after has passed)"
      - "A test with a retry_after longer than the delivery budget proving the summary still arrives"
  - truth: "SC4 (STAFF-12, PLAT-01): the fan-out waits only as long as the cap allows and never outlives the one-run-per-target lock"
    status: partial
    reason: "WR-02 confirmed in code. TelegramPacer.Do calls p.block(d) with the full retry_after before checking d > MaxWait. The caller that got the 429 fails at once, but every other paced call on every replica then reserves slot = now + PTTL(block) in reserveSlotScript, and wait() sleeps the whole delay with no MaxWait cap and no per-call timeout (staffActionCallTimeout starts only inside the closure, after the wait). A retry_after of thousands of seconds therefore stalls the remaining workers and every other staff run until restart, and a stalled run can outlive staffTargetLockTTL (30 min), after which a second Confirm on the same target can start, defeating the restrict-replaces-ban protection the lock exists for. Reaches only a retry_after above 60 s, which is rare."
    artifacts:
      - path: "alita/utils/ratelimit/telegram_pacer.go"
        issue: "wait() does not fail with ErrRateLimited when the reserved delay exceeds MaxWait; block() records an unbounded block"
      - path: "alita/modules/staff_action_card.go"
        issue: "target lock has no heartbeat, so it can expire while the run is alive"
    missing:
      - "wait() returns ErrRateLimited when the reserved delay exceeds MaxWait"
      - "Optionally clamp the shared block, and renew the target lock while a run is alive"
      - "A pacer test where a second caller meets a block above MaxWait"
deferred: []
human_verification:
  - test: "Live run on a real bot with several linked groups (research flag A3, end-to-end run)"
    expected: "/ban @user 1d spam from the Staff Group shows the card; only the issuer's Confirm starts it; the card turns into a summary that fills in; groups where the issuer is not an admin with restrict rights, where the target is an admin, or where the bot lacks rights show skipped or failed with the reason; the Staff Group itself is never touched"
    why_human: "Every Telegram behaviour was proven only against a hand-written BotClient fake and miniredis"
  - test: "Flood control with many linked groups and two bot replicas"
    expected: "Staff Group edits and moderation calls stay inside Telegram limits, a real 429 is waited out, and the final summary still arrives"
    why_human: "Needs real Telegram rate limits; also the only way to see how often WR-01 is reached in practice"
  - test: "Anonymous admin posts /ban in the Staff Group"
    expected: "Reply asking to post as yourself, no card, no action"
    why_human: "Unit-tested with a fake; the real Telegram anonymous-admin sender shape (GroupAnonymousBot) was not seen live"
---

# Phase 2: Staff Actions Across Groups Verification Report

**Phase Goal:** As a staff member, I want to ban, mute or kick someone in every linked group at once, so that one command protects them all.
**Verified:** 2026-10-05T08:30:00Z
**Status:** gaps_found
**Re-verification:** No, initial verification

## Goal Achievement

The feature is built, wired and heavily tested. The safety-critical parts (per-group live authority, card issuer-only Confirm, decision table that never lifts a ban, no write to the Staff Group, non-staff behaviour untouched) hold up against the code, not just against the SUMMARYs. Two reviewer warnings (WR-01, WR-02) were checked against the code and both are real; they sit on success criterion 4 and the one-run-per-target guarantee, so the phase is reported as gaps_found rather than passed.

### Observable Truths (ROADMAP success criteria)

| #   | Truth | Status | Evidence |
| --- | ----- | ------ | -------- |
| 1 | In the Staff Group any member can run /ban /mute /kick /unban /unmute against an @username or numeric ID; timed ban/mute; reason; card shows name, ID, action, duration, reason, linked-group count; only the issuer can Confirm or Cancel; unseen @username refused with a numeric-ID hint | VERIFIED | Interceptors at group 0, priority 65, before Bans (70) and Mutes (80): `staff_action.go:67-100`, registered via `RegisterLegacyModule("StaffActions", 65, ...)`. Card header built in `staff_action_summary.go` (`staffActionHeader`, `staffTargetDisplay`, `staffDurationLabel`); count in `staffActionCardText`. Issuer-only enforced inside the Lua compare-and-set (`transitionStaffCardScript`, `HGET issuer ~= ARGV[1]`), not just in Go. Unknown @username uses `staff_act_username_unknown` ("Use their numeric user ID instead"), users table only, no channels/live fallback. Callback routing in `staff.go:264-267`. Tests: TestStaffActionFiveActions, TestStaffActionCardIssuerOnly, TestStaffActionUsernameUnknown, TestStaffActionTimedBan, TestStaffActionHeaderDuration, all green under -race |
| 2 | After Confirm the action is applied in every linked group where the issuer is live an admin with restrict rights; never in the Staff Group; per-group skips with reason (issuer lacks right, target admin/owner, target is the bot); link with changed owner removed and reported | VERIFIED | `runStaffActionInGroup` (`staff_action_run.go:292-367`): staff-group guard, `recheckLink` (removes link on owner mismatch, line "link removed, the owner changed"), live `getChatMember(group, issuer)` accepting only creator or administrator with `can_restrict_members`, bot and service-ID short-circuit, live target lookup, then `decideStaffAction`. Target creator/admin always wins in the decision table (`staff_action_decide.go:173`). Restrict or member-removing unban never sent to banned or departed targets. Confirm re-verifies Staff Group, issuer membership and the linked-group signature (`staff_action_card.go:751-796`). Tests: TestStaffActionPerGroupGates, TestStaffActionStaffChatNeverActedOn, TestStaffActionNeverLiftsBan, TestStaffActionLinksChangedAborts, TestStaffActionDecisionInvariants |
| 3 | A post from an anonymous admin in the Staff Group gets "post as yourself" and nothing else happens | VERIFIED | `handleStaffAction` checks `helpers.IsAnonymousSender` right after the fresh Staff Group read and before parsing, Redis or any Telegram lookup, then replies `staff_post_as_yourself` and returns `ext.EndGroups` (`staff_action.go:141-150`). The refused variants (/sban etc.) go through the same check first. Test: TestStaffActionAnonymous. Real anonymous-admin payload not seen live (human item 3) |
| 4 | One summary message updates as groups finish; every group is done, skipped or failed with reason; stays within Telegram rate limits across many groups and replicas; waits and retries when Telegram says to; no group silently dropped | PARTIAL (gap) | Mostly present: per-group result slots, `sweepPending` turns any group that never reported (panic, shutdown) into a failed line, 3800-unit collapse never hides skipped or failed lines, continuation messages, `StopStaffActions` drain registered in `main.go:216-220`, Redis-backed fleet pacer (`alita:staff:pace:next` / `:block`) used for every fan-out call, bounded 4 workers, failure reasons from a live bot-rights probe. Tests: TestStaffActionSummaryCollapse, TestStaffActionWorkerPanicReported, TestStopStaffActionsFinalizes, TestTelegramPacerFleetSpacing, TestTelegramPacerSharedBlock. Defects: WR-01 (final summary can be lost on a 429 above ~15 s) and WR-02 (block above MaxWait stalls the whole fleet and can outlive the target lock). Both confirmed in code, see gaps |
| 5 | /ban /mute /kick /unban /unmute work exactly as before in every non-Staff Group | VERIFIED | Interceptor returns `ext.ContinueGroups` with no reply, write or Telegram call when `isStaffChatCached` is false (`staff_action.go:97-103`). TestStaffActionNonStaffUnchanged compares Bans/Mutes call sequences with and without the StaffActions module loaded; TestStaffActionModuleOrder, TestStaffActionStaleGatePassesThrough also green. Minor fail-open edge noted below |

**Score:** 4/5 truths verified (criterion 4 partial)

### Required Artifacts

| Artifact | Expected | Status | Details |
| -------- | -------- | ------ | ------- |
| `alita/modules/staff_action.go` | command interceptors, target resolution | VERIFIED | substantive, registered, 14 commands incl. refused variants |
| `alita/modules/staff_action_card.go` | Redis card, Lua CAS, target lock, expiry, Confirm/Cancel | VERIFIED | wired from `staff.go` callback router |
| `alita/modules/staff_action_decide.go` | pure decision table | VERIFIED | single place choosing the Telegram write |
| `alita/modules/staff_action_run.go` | paced bounded fan-out, classification, shutdown drain | VERIFIED with WR-01 (delivery budget) |
| `alita/modules/staff_action_summary.go` | summary, collapse, final delivery | VERIFIED with WR-01 |
| `alita/modules/staff_action_parse.go` | argument grammar | VERIFIED | |
| `alita/utils/ratelimit/telegram_pacer.go` | fleet pacer | VERIFIED with WR-02 |
| `alita/db/user/repository.go` (`FindUsersByUsername`) | case-insensitive username lookup | VERIFIED | ordering has no tiebreaker (IN-02) |
| `main.go` shutdown hook | `StopStaffActions` before DB close | VERIFIED | `main.go:216-220` |
| `locales/*.yml` (7 files) | `staff_act_*` keys | VERIFIED | `make check-translations` clean |

### Key Link Verification

| From | To | Via | Status | Details |
| ---- | -- | --- | ------ | ------- |
| `/ban` etc. in Staff Group | `handleStaffAction` | `handlers.NewCommand` loop, priority 65 before Bans/Mutes | WIRED | registry sorted ascending in `LoadAllModules` |
| Card button `staff\|v1\|a=xc&t=` | `staffActionConfirm` | `staff.go:264-267` | WIRED | |
| Confirm | `startStaffActionRun` | after Lua CAS, target lock, link signature check | WIRED | |
| Run | every Telegram call | `staffPaced` -> `staffActionPacer.Do` | WIRED | card edits and final delivery are NOT paced (cause of WR-01) |
| `main.go` | `StopStaffActions` | `shutdownManager.RegisterHandler` | WIRED | |
| Run | `recheckLink` | per-link owner recheck with shared paced owner pass | WIRED | |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
| -------- | ------------- | ------ | ------------------ | ------ |
| card header | `TargetName` | `FindUsersByUsername` / `user.GetUserInfoById` | users table | FLOWING |
| summary lines | `progress.results` | per-group worker results from live `getChatMember` and write calls | yes | FLOWING |
| `until_date` | `newUntil` | computed once at Confirm from `DurationSec` | yes, same value to every group and retry | FLOWING |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
| -------- | ------- | ------ | ------ |
| Staff action tests (module) | `go test -tags testtools -race -count=1 -run 'Staff\|staff' ./alita/modules` | ok 96 s | PASS |
| Pacer, extraction, user lookup | `go test -tags testtools -race -count=1 ./alita/utils/ratelimit ./alita/utils/extraction ./alita/db/user` | ok | PASS |
| Staff DB package | `go test -tags testtools -race -count=1 ./alita/db/staff` | ok | PASS |
| Locale parity | `make check-translations` | all present | PASS |
| go vet | `go vet -tags testtools ./alita/modules ./alita/utils/ratelimit` | clean | PASS |
| gofmt | `gofmt -l alita main.go` | only `greetings_command_test.go`, last touched by commit a27bd8c, not this phase | PASS for phase files |

Lint was not run (sandbox golangci-lint built with go1.25 vs module go1.26.0). No test covers WR-01 or WR-02; the tests that pass use a 5 ms retry unit, so green tests do not refute the defects.

### Probe Execution

Step 7c: SKIPPED (no probes declared by the phase plans).

### Requirements Coverage

All 11 IDs appear in plan frontmatter and in REQUIREMENTS.md (traceability rows map each to Phase 2). No orphaned IDs: REQUIREMENTS.md maps no further Phase 2 IDs beyond these (STAFF-09 to STAFF-11 belong to later phases).

| Requirement | Source Plan | Description | Status | Evidence |
| ----------- | ----------- | ----------- | ------ | -------- |
| STAFF-01 | 01, 02, 03, 04 | five commands, timed variants, @username or ID | SATISFIED | SC1 evidence |
| STAFF-02 | 01, 03 | optional reason | SATISFIED | `staffActionHeader`, parse tests |
| STAFF-03 | 01, 03, 04, 05 | confirmation card, issuer-only | SATISFIED | Lua CAS, expiry, exactly-once tests |
| STAFF-04 | 01, 05 | every linked group, never the Staff Group | SATISFIED | staff-group guard, links signature abort |
| STAFF-05 | 01, 02 | live per-group issuer authority | SATISFIED | `fetchLiveMember` + `staffIssuerSkipReason` |
| STAFF-06 | 01, 04 | anonymous admin refused | SATISFIED | SC3 evidence |
| STAFF-07 | 01, 02 | never act on admin/owner/bot targets | SATISFIED | decision table, bot and service short-circuit |
| STAFF-08 | 01, 06, 07 | one summary, per-group status, none dropped | PARTIAL | groups never dropped from the summary; final delivery can fail under flood (WR-01) |
| STAFF-12 | 06, 07 | rate limits, wait and retry, report as failed | PARTIAL | fan-out pacing and retry verified; delivery budget (WR-01) and over-cap block (WR-02) defects |
| STAFF-13 | 01, 02, 03, 04 | per-group commands unchanged outside Staff Groups | SATISFIED | TestStaffActionNonStaffUnchanged |
| PLAT-01 | 01, 05, 06 | multi-replica safe | SATISFIED for card CAS, target lock and pacer; WR-02 weakens the lock guarantee in the over-cap case. Whole-requirement scope (detection counters, captcha, lockdown) belongs to later phases |

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
| ---- | ---- | ------- | -------- | ------ |
| `staff_action_run.go` | 56, 245 | 15 s delivery budget below the 60 s retry cap | Blocker for SC4 (WR-01) | final summary can be lost |
| `telegram_pacer.go` | 128-167 | block recorded before MaxWait check; `wait` not capped | Warning (WR-02) | fleet-wide stall, lock can expire |
| `staff_action.go` | 97-103 (`isStaffChatCached`) | a failed cached Staff Group read returns nil, so the update falls through to the per-group command | Info | fail-open only for an admin typing in a Staff Group during a DB or cache fault |
| `staff_action_card.go` | 518-526 | missing-hash tap rewrites the message with no issuer check | Info (IN-06) | narrow edge |
| `staff_action_run.go` | 362-365 | "interrupted by restart" shown for a write that may have landed | Info (IN-04) | misleading line; re-run is idempotent |
| various | | IN-01, IN-02, IN-03, IN-05 from 02-REVIEW.md | Info | quality items, none affect the goal |

No TBD, FIXME or XXX markers found in the phase's production files.

### Human Verification Required

See the `human_verification` frontmatter: a live end-to-end run, real flood control across many groups and two replicas, and a real anonymous-admin post. These cannot be proven with the fake client and are also the only way to learn how often WR-01 is reached.

### Gaps Summary

Criteria 1, 2, 3 and 5 are achieved and backed by passing behavioral tests plus a read of the code. Criterion 4 is mostly achieved but has two confirmed robustness defects:

1. WR-01: the final summary delivery budget (15 s) is shorter than the retry_after it tries to honor (up to 60 s), and the fallback send shares the same dead context. Under sustained flood control the groups are acted on but the issuer's card stays on the last progress snapshot. The 2.5 s unpaced progress edits make the flood more likely. Small fix: fresh per-message contexts sized to the cap, and back off progress edits on a 429.
2. WR-02: a retry_after above MaxWait blocks the whole fleet for the full duration, and a stalled run can outlive the 30 minute target lock. Small fix: make `wait` fail with `ErrRateLimited` when the reserved delay exceeds MaxWait.

Both are narrow in likelihood (WR-01 needs a 429 above 15 s on the Staff Group chat; WR-02 needs a retry_after above 60 s) but both break a sentence of criterion 4 or a stated guarantee, and neither has a test. The 02-REVIEW-DISPOSITION.md still lists them as `open`. Close them with `/gsd-plan-phase --gaps`, or accept them with an override if the maintainer judges the risk acceptable for a private bot.

---

_Verified: 2026-10-05T08:30:00Z_
_Verifier: Claude (gsd-verifier)_
