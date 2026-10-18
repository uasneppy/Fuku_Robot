---
phase: 05-lockdown-alerts-and-response
verified: 2026-10-18T05:40:00Z
status: passed
score: 5/5 must-haves verified
covered_files:
  - .planning/phases/05-lockdown-alerts-and-response/05-01-PLAN.md
  - .planning/phases/05-lockdown-alerts-and-response/05-01-SUMMARY.md
  - .planning/phases/05-lockdown-alerts-and-response/05-02-PLAN.md
  - .planning/phases/05-lockdown-alerts-and-response/05-02-SUMMARY.md
  - .planning/phases/05-lockdown-alerts-and-response/05-03-PLAN.md
  - .planning/phases/05-lockdown-alerts-and-response/05-03-SUMMARY.md
  - .planning/phases/05-lockdown-alerts-and-response/05-04-PLAN.md
  - .planning/phases/05-lockdown-alerts-and-response/05-04-SUMMARY.md
  - .planning/phases/05-lockdown-alerts-and-response/05-05-PLAN.md
  - .planning/phases/05-lockdown-alerts-and-response/05-05-SUMMARY.md
  - .planning/phases/05-lockdown-alerts-and-response/05-06-PLAN.md
  - .planning/phases/05-lockdown-alerts-and-response/05-06-SUMMARY.md
  - .planning/phases/05-lockdown-alerts-and-response/05-07-PLAN.md
  - .planning/phases/05-lockdown-alerts-and-response/05-07-SUMMARY.md
  - .planning/phases/05-lockdown-alerts-and-response/05-08-PLAN.md
  - .planning/phases/05-lockdown-alerts-and-response/05-08-SUMMARY.md
  - .planning/phases/05-lockdown-alerts-and-response/05-09-PLAN.md
  - .planning/phases/05-lockdown-alerts-and-response/05-09-SUMMARY.md
  - .planning/phases/05-lockdown-alerts-and-response/05-10-PLAN.md
  - .planning/phases/05-lockdown-alerts-and-response/05-10-SUMMARY.md
  - .planning/phases/05-lockdown-alerts-and-response/05-11-PLAN.md
  - .planning/phases/05-lockdown-alerts-and-response/05-11-SUMMARY.md
  - .planning/phases/05-lockdown-alerts-and-response/05-12-PLAN.md
  - .planning/phases/05-lockdown-alerts-and-response/05-12-SUMMARY.md
  - .planning/phases/05-lockdown-alerts-and-response/05-13-PLAN.md
  - .planning/phases/05-lockdown-alerts-and-response/05-13-SUMMARY.md
  - .planning/phases/05-lockdown-alerts-and-response/05-14-PLAN.md
  - .planning/phases/05-lockdown-alerts-and-response/05-14-SUMMARY.md
  - alita/db/lockdown/alerts.go
  - alita/db/lockdown/burst.go
  - alita/db/lockdown/links.go
  - alita/modules/lockdown_alert.go
  - alita/modules/lockdown_alert_callbacks.go
  - alita/modules/lockdown_recent.go
  - alita/modules/lockdown_response.go
  - alita/modules/lockdown_worker.go
covered_digest: "v2:sha256:1bc11562b8cb006093f725315c0cfa29fb3419178f80fa4b03b0c3adfa994de5"
behavior_unverified: 0
overrides_applied: 0
human_verification:
  - test: "In a real supergroup linked to a Staff Group with a log channel (admin category on), run /lockdown, then press Lift lockdown on the Staff Group copy and on the log channel copy (separately, in two lockdowns), with a Staff Group member who is NOT an admin of the locked group and then with a real admin."
    expected: "The non-admin gets an alert toast and nothing changes. The admin gets a confirm card, Confirm restores the group's exact permissions, posts 'Lockdown lifted by X' in the locked group, and both alert copies are edited to 'Lifted by X at T' with no buttons."
    why_human: "Needs a live Telegram bot. Tests use hand-written gotgbot fakes, so real getChatMember/setChatPermissions behaviour and callbacks from a channel message are not exercised."
  - test: "Have several accounts (and one admin, one approved user) join a locked group, press Ban N recent joiners and Confirm."
    expected: "Accounts are banned for good (no end date, they cannot rejoin), the admin and the approved user are untouched, the card is edited to the banned/skipped/failed counts, and the log channel gets one #LOCKDOWN_BAN post. After /unlockdown the response bans stay."
    why_human: "Real banChatMember results, rate limits and the permanence of the ban need a live bot."
  - test: "Join a locked group through an invite link the bot created, then press Revoke link as an admin with can_invite_users, as an admin without it, and again after the revoke. Also join through a link another admin made."
    expected: "The alert shows the link; Revoke works only for the admin with can_invite_users; the alert copies get 'Link revoked by X' and lose the button; a foreign (cut short) link shows the 'revoke it in Telegram settings' line and no button. Telegram's real refusal texts map to the plain reasons."
    why_human: "Which link Telegram reports in chat_member updates and the real revokeChatInviteLink refusal descriptions cannot be produced by the fakes."
  - test: "Leave a lockdown active for at least two hours (or lower lockdownReminderEvery on a test deployment), then lift it."
    expected: "One reminder per destination appears each hour, the previous reminder loses its buttons, no reminders appear after the lift, and a restart or second replica never produces a duplicate reminder."
    why_human: "Hourly timing, multi-replica behaviour and the real clock are only simulated in tests."
  - test: "Run `make test-postgres-integrity` in CI (PostgreSQL)."
    expected: "TestBeginAlert, TestBanRunStart, TestUnqueueBanRun, TestBanRunReport, TestAlertClose, TestRecordLinkRevoked, TestAlertSync, TestAlertSyncKeepsAMarkSetDuringTheEdit and TestReminderSlots pass. They prove the exactly-once guarantees (one alert, one running ban run, one revoke, one reminder per destination) that SQLite cannot prove."
    why_human: "No PostgreSQL is available in this environment; the guarantees rest on a partial unique index and conditional updates that only PostgreSQL enforces the way production does."
---

# Phase 5: Lockdown Alerts and Response Verification Report

**Phase Goal:** As a group admin or staff member, I want to get a lockdown alert with one-tap fixes, so that we can see a raid and clean it up fast.
**Verified:** 2026-10-18
**Status:** human_needed
**Re-verification:** No, initial verification

## Goal Achievement

Every roadmap success criterion is implemented, wired into the worker and the dispatcher, and exercised by passing tests. I read the code instead of trusting the SUMMARYs. Code-level verification found no gaps. The remaining items need a live Telegram bot or PostgreSQL, so they are human verification, not gaps.

### Observable Truths

| # | Truth (roadmap SC) | Status | Evidence |
|---|--------------------|--------|----------|
| 1 | One alert per lockdown to the Staff Group (if linked) and the log channel, showing trigger, counts and a sample of the burst, including removed accounts | VERIFIED | `lockdownPlanAlerts` -> `lockdown.BeginAlert` (one conditional update on `alert_started_at IS NULL`, creates one copy row per destination and the pre-lockdown burst rows in the same transaction). `lockdownAlertDestinations` gives a Staff Group slot from a fresh link and a log slot via `actionlog.Destination(..., CategoryAdmin)`; the locked group is never a destination. `lockdownRenderAlert` renders group, trigger, reason, counts (`BurstSampleFresh`: pre rows plus lockdown joiner rows in any state, so removed ones are listed) and up to 10 accounts, with names escaped and spliced in after the translator. The worker runs plan then post each cycle. Tests passed: `TestLockdownAlertDestinations`, `TestLockdownAlertSample`, `TestLockdownAlertPostsOnce`, `TestLockdownAlertDelivery`, `TestLockdownAlertGate`, `TestBeginAlert` (SQLite). |
| 2 | Lift works only for a live admin of the locked group, from the Staff Group or the log channel; others refused with no change; a lifted lockdown says so and does nothing | VERIFIED | `lockdownAlertCallback` is registered with `callbackquery.Prefix("lockdown|")` and `SetAllowChannel(true)`. It refuses service identities first, requires the copy row to belong to the pressed chat, and takes the locked chat from the lockdown row. `lockdownButtonAuthority` uses a live `getChatMember` (creator or admin with `can_restrict_members`), so neither the admin cache nor Staff Group membership authorizes. Ask and Confirm both re-check. Confirm runs `liftLockdownRow`, the same function as `/unlockdown` (restore first, then conditional `BeginLift`). A non-active lockdown goes to `lockdownStalePress` ("Already lifted by X at T", closes that copy, no action). Tests passed: `TestLockdownAlertLiftAuthority` (staff non-admin, no restrict right, lookup failure, service IDs, forged chat, creator, demoted before Confirm), `TestLockdownAlertStaleButtons`, `TestLockdownAlertLiftFromLogChannel`, `TestLockdownAlertLift`, `TestLockdownAlertLiftOutcomes`, `TestLockdownAlertClosedAtLift`. |
| 3 | Admin presses "Ban N recent joiners"; admins and approved users excluded; told banned/skipped/failed | VERIFIED | Ask and Confirm check live admin authority. Approved users come from `approvals.ListApprovedIDsFresh` (a failed read refuses the Ask, starts no run, and makes a running run wait). `StartBanRun` inserts the run under the `uk_chat_lockdown_ban_runs_running` partial unique index and queues unhandled burst rows (bot and approved users excluded; exempt/let-in rows marked skipped). `lockdownResponseBanOne` is the single ban path: skips approved, bot, exempt, claims the joiner row, then in one paced unit reads the live status, applies `decideLockdownResponseBan` (live creator or admin skipped; a kicked account under someone else's ban is left alone), re-reads the lockdown for activeness, and calls `banChatMember` with empty options (no end date). `lockdownReportBanRuns` edits the card with banned/skipped (per reason)/failed counts and posts one `#LOCKDOWN_BAN` to the log channel. Tests passed: `TestLockdownBanRecentJoiners`, `TestLockdownBanAuthority`, `TestLockdownBanResult`, `TestLockdownBanRun`, `TestLockdownBanRaces`, `TestLockdownResponseBanOne`, `TestDecideLockdownResponseBan`, `TestLockdownBanSurvivesLift`, `TestLockdownBanReportResumes`, `TestLockdownBanCount`. |
| 4 | Alert shows the invite link when Telegram reports it; only an admin of that group can revoke it | VERIFIED | `lockdown.TopInviteLinksFresh` feeds `lockdownRenderLinkLine` (link in code tags, HTML-escaped and spliced after the translator, "+N other links", and a foreign-link line for a link cut short). The Revoke button shows only while `lockdownLinkRevocable(slot.Link)` and `LinkRevokedAt == nil`. `lockdownRevokeAsk` and `lockdownRevokeConfirm` use `lockdownInviteAuthority` (live `getChatMember`, creator or admin with `can_invite_users`); the link revoked is always `slot.Link` from the pressed copy, never callback data. The revoke is recorded (`RecordLinkRevoked`, one winner, marks live copies `edit_pending`) only after Telegram confirmed it. Tests passed: `TestLockdownAlertInviteLink`, `TestLockdownRevokeLink`, `TestLockdownRevokeLinkRefusals`, `TestAlertSync` and `TestRecordLinkRevoked` (the last two in `alita/db/lockdown`, SQLite run). |
| 5 | While locked, the alert is re-posted periodically as a reminder; reminders stop at the lift | VERIFIED | `lockdownPlanReminders` (`ListRemindableFresh`: active, confirmed, alert decided) plans only the current `seq = floor((now - alert_started_at) / 1h)` through `PlanReminderSlots` (`ON CONFLICT DO NOTHING` on the slot key, and only while the lockdown is active). `lockdownPostAlertSlot` refuses to post for a non-active lockdown and supersedes a stale `seq`. A posted reminder takes the buttons off older reminders (`SupersedeReminders`). Pending copies are listed only for active lockdowns; the lift's close step edits every copy to "Lifted by". Tests passed: `TestLockdownReminders` (first, second, no catch-up, concurrent, stops at the lift, stale pending), `TestLockdownReminderContent`, `TestLockdownReminderDestinations`. |

**Score:** 5/5 truths verified (0 present-but-behavior-unverified; live-Telegram and PostgreSQL items are listed under Human Verification).

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `alita/modules/lockdown_alert.go` (987 lines) | alert planning, posting, reminders, sync, close | VERIFIED | Substantive and wired into `runLockdownCycle` (plan, reminders, post, sync, close). |
| `alita/modules/lockdown_alert_callbacks.go` (739 lines) | `lockdown|` button handlers | VERIFIED | Registered in `LoadLockdown` with `SetAllowChannel(true)`. |
| `alita/modules/lockdown_response.go` (701 lines) | ban policy, run processing, result report | VERIFIED | Run processing and report steps are in `runLockdownCycle`. |
| `alita/modules/lockdown_recent.go` | best-effort 10-minute join tracker | VERIFIED | Called by both group `-7` join handlers (`lockdown_guard.go` lines 123 and 194). |
| `alita/db/lockdown/alerts.go`, `burst.go`, `links.go` | conditional-update repositories | VERIFIED | Fresh reads, conditional updates, errors wrapped. No `DeleteCache` is needed because the rows are never cached (matches AGENTS.md). |
| `migrations/20261011120000..20261011160000_*.sql` (5 files) | alerts, burst, ban runs, link revoke, report columns | VERIFIED | Timestamps greater than all earlier files, no `BEGIN`/`COMMIT`/`CONCURRENTLY`, unique/partial unique indexes present, runner globs `*.sql`. |
| `alita/db/models/lockdown.go` models | `ChatLockdownAlert`, `ChatLockdownBurst`, `ChatLockdownBanRun` | VERIFIED | In the AutoMigrate lists of `alita/modules/test_harness_test.go` and `alita/db/lockdown/testmain_test.go`. |
| Locale keys (7 files) | `lockdown_alert_*`, `lockdown_ban_*`, `lockdown_reminder_*`, `lockdown_revoke_*` | VERIFIED | 56 matching keys in each of en/es/fr/hi/id/pt/ru; `make check-translations` passes. |

### Key Link Verification

| From | To | Via | Status |
|------|----|-----|--------|
| `runLockdownCycle` | `lockdownPlanAlerts` / `lockdownPlanReminders` / `lockdownPostAlerts` / `lockdownSyncAlertCopies` / `lockdownCloseAlerts` | direct calls in `lockdown_worker.go` lines 291-300 | WIRED |
| `runLockdownCycle` | `lockdownProcessBanRuns` / `lockdownReportBanRuns` | direct calls | WIRED |
| `LoadLockdown` | `lockdownAlertCallback` | `handlers.NewCallback(Prefix("lockdown|")).SetAllowChannel(true)` | WIRED |
| `lockdownLiftConfirm` | `liftLockdownRow` -> `lockdownBeginLift` | same function as `/unlockdown` | WIRED |
| join guard (`-7`) | `trackLockdownRecentJoins` | both join paths | WIRED |
| `lockdownPlanAlerts` | `lockdown.BeginAlert` | exactly-once conditional update | WIRED |
| `lockdownBanConfirm` | `lockdown.StartBanRun` -> worker `lockdownProcessBanRun` -> `lockdownResponseBanOne` | run row plus queued burst rows | WIRED |

### Data-Flow Trace (Level 4)

| Artifact | Data | Source | Real data | Status |
|----------|------|--------|-----------|--------|
| alert sample and counts | `accounts`, `counts` | `BurstSampleFresh` (burst and joiner tables) | Yes | FLOWING |
| alert invite link | `links` | `TopInviteLinksFresh` over joiner `invite_link` | Yes | FLOWING |
| Ban N label and card | `count` | `CountBanCandidatesFresh` | Yes | FLOWING |
| ban result card | `tally` | `TallyBanRun` | Yes | FLOWING |
| reminder counts | `tally` | `TallyJoiners` / `lockdownRemovedSoFar` | Yes | FLOWING |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| Alert, lift, ban, revoke, reminder, close tests (about 35 top-level tests) | `go test -tags testtools -race -count=1 -run '^(...)$' ./alita/modules` (two runs) | `ok` | PASS |
| Conditional-update repositories | `go test -tags testtools -race -count=1 ./alita/db/lockdown ./alita/db/approvals` | `ok` | PASS |
| Callback data fits 64 bytes | `go test -tags testtools ./alita/utils/callbackcodec` and `TestLockdownCallbackData` | `ok` | PASS |
| Production build | `CGO_ENABLED=0 go build ./...` | success | PASS |
| `go vet -tags testtools ./alita/modules ./alita/db/lockdown` | | no output | PASS |
| `make check-translations`, `make check-docs` | | all present, no drift | PASS |

The full `make test` was not re-run, as instructed (it passed 61/61 on HEAD per the orchestrator). Step 7c: no probe scripts are declared by this phase.

### Requirements Coverage

| Requirement | Description | Status | Evidence |
|-------------|-------------|--------|----------|
| LOCK-11 | One alert to Staff Group and log channel with trigger, counts, sample, Lift and Ban buttons | SATISFIED | Truth 1 and keyboard `lockdownAlertKeyboard`. |
| LOCK-12 | Admin presses Ban N recent joiners; admins and approved excluded; result reported | SATISFIED | Truth 3. |
| LOCK-13 | Alert shows invite link; Revoke link admin-only | SATISFIED | Truth 4. |
| LOCK-14 | Periodic reminder until lifted | SATISFIED | Truth 5. |
| LOCK-06 (finish) | Lift from the alert's button, wherever posted | SATISFIED | Truth 2. |
| LOCK-01 (finish) | Removed joiners listed on the alert | SATISFIED | Joiner rows in any state are listed by `BurstSampleFresh`. |

No orphaned requirements: REQUIREMENTS.md maps only LOCK-11 to LOCK-14 to Phase 5.

### Anti-Patterns Found

None blocking. No `TODO`/`FIXME`/`XXX`/`TBD` markers in the phase's non-test files or migrations. No stubs, empty handlers or hardcoded empty data on the rendered paths. The 12 review findings (3 warnings, 9 info) were each confirmed fixed in the code: WR-01 (resumable ban report, `lockdownReportBanRuns`), WR-02 (re-read of the lockdown inside the paced unit, `lockdownResponseBanOne`), WR-03 (tests listed in `test-postgres-integrity`), IN-01 (staff link re-checked before send), IN-02/IN-07 (revoke refusal mapping and lost-race text), IN-05 (dead code removed).

Observations (not gaps):
- By design (D-04, D-20) the alert text is not refreshed. Accounts that join after the alert posts appear only as counts in reminders and in the Ban N targets, and a log channel whose `admin` category is off gets no copy. This matches the owner's decisions and LOCK-01/LOCK-11 as scoped there.
- `.planning/REQUIREMENTS.md` still shows LOCK-11..LOCK-14 unchecked (lines 69-76) and "Pending" in the traceability table (lines 204-207). The orchestrator should tick them and update ROADMAP progress once the human items are accepted.
- `deferred-items.md` records one test flake from plan 05-11; its note says the orchestrator resolved it (`queuedBurst` reuses an existing burst row) and the affected tests passed here.

### Human Verification Required

See the `human_verification` frontmatter. Five items: real Lift from the Staff Group and the log channel; a real Ban run; real invite-link reporting and revoke refusals; the hourly reminder across a real clock, restart and replicas; and the PostgreSQL integrity run in CI.

### Gaps Summary

No gaps. All five roadmap success criteria are implemented, wired and covered by passing tests against real SQLite fixtures and hand-written Telegram fakes. The status is `human_needed` only because the live-Telegram and PostgreSQL behaviours listed above cannot be proven here.

---

_Verified: 2026-10-18_
_Verifier: Claude (gsd-verifier)_

## Human Verification Outcome

2026-10-18: the owner accepted the human verification items ("All good, continue") and Phase 5 is marked passed.
