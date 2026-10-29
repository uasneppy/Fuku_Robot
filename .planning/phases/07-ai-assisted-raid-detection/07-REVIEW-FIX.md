---
phase: 07-ai-assisted-raid-detection
fixed_at: 2026-10-29T22:00:00Z
review_path: .planning/phases/07-ai-assisted-raid-detection/07-REVIEW.md
iteration: 1
findings_in_scope: 14
fixed: 14
skipped: 0
status: all_fixed
---

# Phase 7: Code Review Fix Report

**Fixed at:** 2026-10-29
**Source review:** `.planning/phases/07-ai-assisted-raid-detection/07-REVIEW.md`
**Iteration:** 1

**Summary:**
- Findings in scope: 14 (5 warnings, 9 info)
- Fixed: 14
- Skipped / deferred: 0

Verification ran in the main checkout on branch `claude/cool-hamilton-hvs9nl` (no worktree, as instructed).
PostgreSQL was never started; PostgreSQL-only tests run in CI. `CGO_ENABLED=0 go build ./...`,
`go vet -tags testtools ./...`, `gofmt -l`, `make check-translations` and `make check-docs` are clean.
`CGO_ENABLED=1 go test -tags testtools -race -count=1` passes for `alita/modules`, `alita/config`, `alita/utils/gemini`,
`alita/utils/logredact`, `alita/db/raiddetection`, `alita/i18n` and `scripts/gemini-spike`. `make test` and `make lint`
were not run (`golangci-lint` cannot lint Go 1.26 here; the orchestrator runs `make test`). No locale file was touched,
so `TestLockdownHelpFitsOneMessage` is unaffected.

## Fixed Issues

### WR-01: One new account can exhaust the per-group and the fleet-wide Gemini budget

**Files modified:** `alita/modules/raid_ai_image.go`, `alita/modules/raid_ai.go`, `alita/modules/raid_ai_budget_test.go`,
`AGENTS.md`, `docs/src/content/docs/architecture/caching.md`, `docs/src/content/docs/commands/aispam/index.md`
**Commit:** 15ad328
**Applied fix:** `raidAIReserveBudget(client, chatID, userID)` now takes one unit of a per-account budget first
(`alita:raid:{<chat>}:abu:<user>:<hour>`, `raidAIUserHourlyBudget` = 20, a variable only so a test can shrink it), with
the same atomic `INCR`+`EXPIRE` and `{chat}`-tagged key as the shared budgets. An account over its cap is counted in
`RaidAIStats.Capped` and spends neither the group's nor the fleet's unit. The verdict cache is read before any budget, so
a cached verdict still costs nothing. Tests: one account posting 12 distinct photos with a cap of 4 makes exactly 4 Gemini
calls, `Capped` grows by 8, the group and global counters stay at 4, and a second member's photo is still judged; a
sticker posted four times by an account with a cap of 1 makes one call and is never capped.

### WR-02: `ENABLE_AISPAM=false` does not stop the Gemini image checks

**Files modified:** `alita/modules/raid_ai.go`, `alita/modules/raid_ai_image.go`, `alita/config/config.go`,
`alita/modules/raid_ai_killswitch_test.go` (new), `sample.env`, `docs/src/content/docs/api-reference/environment.md`,
`docs/src/content/docs/commands/aispam/index.md`, `AGENTS.md`
**Commit:** 3e76ad6
**Applied fix:** `raidAIImageConfigured()` now requires `EnableAISpam` as well as a Gemini key, so `raidAIConfigured()`
is false with the switch off and `/autolockdown` shows `lockdown_auto_ai_unavailable`. The watcher enqueues nothing, and a
job already queued is dropped by `raidAIProcessJob` (text) and `raidAIProcessImage` (image) before any `getFile` or
provider call. `sample.env`, the environment and `/aispam` docs, the config comment and AGENTS.md now call it the global AI
kill switch for text and images. Tests (all fail on the old code): configured checks follow the switch; a new member's
photo is not fetched or sent; a queued image job and a queued text job are dropped; a link text is not sent to TypeSafe;
`/autolockdown` shows the not-set-up notice with both keys set.

### WR-03: Raid-only text checks send chat description, rules and sender profile

**Files modified:** `alita/modules/raid_ai.go`, `alita/modules/aispam_jev.go`, `alita/modules/raid_ai_request_test.go` (new),
`sample.env`, `docs/src/content/docs/commands/aispam/index.md`, `AGENTS.md`
**Commit:** 307020d
**Applied fix:** The raid-only call builds its own `raidAIState` (`raidAIStateOf`): chat title and language, the sender's
first-seen days and a `messages_last_hour` of 1, and the text with `has_link`, link domains and `is_forward`. It has no field
for the description, rules, warn count or recent messages, so none can be sent, and the live `getChat` for the description
no longer happens on this path. `/aispam`'s `aispamState` request is byte-for-byte unchanged (`aispamJevDecide` and
`aispamJevRequest.State` now take `any`). Test `TestRaidAITextRequestIsMinimal` reads the body the TypeSafe httptest server
receives for a chat that has rules and a sender with a warning, and checks the exact key set and that no sentinel from the
rules or the warning leaks. The `/lockdown` help text only says "suspicious text ... is checked by TypeSafe", so it never
claimed more and was left alone (no locale change). `sample.env`, the `/aispam` docs and AGENTS.md say exactly what is sent.

### WR-04: The AI-burst snapshot puts established members into "Ban N recent joiners"

**Files modified:** `alita/modules/raid_ai_count.go`, `alita/modules/lockdown_alert.go`, `alita/modules/raid_trigger.go`,
`alita/modules/raid_ai_burst_test.go`, `AGENTS.md`, `docs/src/content/docs/architecture/caching.md`
**Commit:** af8db15
**Applied fix:** `raidAISpammersScript` now saves an account only when the `:joined` set scores it within 24 h, scored by that
join time; an account with no recent score is never saved. Established accounts still count toward the threshold and the
3-account floor (the verdict script is untouched). AGENTS.md records "the AI burst snapshot lists recent joiners only".
Tests: new case "an established spammer counts but is never listed or banned" (one established and two recent spammers):
the lockdown still starts, the snapshot holds the two recent joiners, the alert lists them and not the established one, there
are 2 burst rows, the Ban button says 2, and after Confirm the recent joiners are banned and the established one is not. The
shared fixture `aiSpammers()` now makes three recent joiners; the old three-with-one-established fixture moved to
`aiSpammersWithEstablished()` for the new case. The "spammers listed" case no longer expects a verdict-time row.

### WR-05: The test harness waits for AI jobs and posts but not for the lockdown starts in between

**Files modified:** `alita/modules/test_harness_test.go`, `alita/modules/staff_action_fake_test.go`,
`alita/modules/lockdown_alert_test.go`, `alita/modules/raid_ai_integration_test.go`
**Commit:** f347dd0
**Applied fix:** `callsFor`, `sentTo`, `sentChats` and `callsSince` now wait `raidAIJobs`, then `raidAIFires`, then
`raidPosts`, the order `settleAI` uses. No other helper waited on the trackers.

### IN-01: `raidAIGeminiPause.reason` is written and never read

**Files modified:** `alita/modules/raid_ai_breaker.go`
**Commit:** 7fa479a
**Applied fix:** The write-only field is removed; the warning keeps logging its local reason. No behaviour change, so no new test.

### IN-02: An invalid `GEMINI_MODEL` is logged verbatim before secrets are registered

**Files modified:** `alita/config/config.go`, `alita/config/config_pure_test.go`
**Commit:** 4499365
**Applied fix:** The warning now says only that the value was rejected and how many characters it had. Test: a value shaped
like a pasted key falls back to the default, a warning names `GEMINI_MODEL`, and no log line carries the value.

### IN-03: The Gemini per-chat breaker silently stops tracking new chats past 10000

**Files modified:** `alita/modules/raid_ai_breaker.go`, `alita/modules/raid_ai_breaker_test.go`, `AGENTS.md`
**Commit:** 9907fbb
**Applied fix:** A full map first evicts idle breakers (no failures, not paused); if every tracked chat is failing or paused,
the new chat stays untracked and one warning per process says so. Tests: a full map of idle breakers makes room and the new
chat gets paused after five failures; a full map of failing chats logs exactly one warning.

### IN-04: Album gate and cross-chat `singleflight` edge cases

**Files modified:** `alita/modules/raid_ai_image.go`, `alita/modules/raid_ai_budget_test.go`, `AGENTS.md`
**Commit:** 2c36980
**Applied fix:** Both edges closed, the fix was small. The singleflight is keyed by chat ID and `file_unique_id`, so another
chat no longer inherits a leader's refusal (the verdict cache still serves it afterwards). The album gate stays where it is
(taking it after the budget would let every photo of an album spend a unit before the gate drops it), but it is given back
with `DEL` (`raidAIAlbumRelease`) when the photo that took it is refused before any Gemini call (paused breaker, budget
refusal, Redis error, fire latch), so the rest of the album can still be judged; a photo that reached Gemini keeps the gate.
Tests (both fail on the old code): an album whose first photo met a pause is judged once the pause ends; a failing album
costs one call only; a chat B photo overlapping a failing chat A call for the same sticker gets its own verdict.

### IN-05: Real-time sleeps in two tests

**Files modified:** `alita/modules/raid_ai_budget_test.go`, `alita/modules/raid_ai_breaker_test.go`,
`alita/modules/aispam_test.go`, `alita/modules/aispam.go`
**Commit:** 3cccfed
**Applied fix:** The overlap test waits (polling `waitUntil`) for the leader to be inside the gated Gemini handler and for all
three jobs to have asked Redis for the fire latch (counted with miniredis's pre-hook), instead of sleeping 200 ms; the gate
is also opened on cleanup so a failed wait cannot hang. The `/aispam` breaker (`aispamBreakerPaused`,
`aispamBreakerRecordFailure`) now reads `raidNow`, the clock the Gemini breaker uses (`time.Now` in production), so the raid
breaker test and the `/aispam` cooldown test move a clock instead of sleeping. Both tests still assert the same outcomes.

### IN-06: The config test leaves secrets in the global redaction registry

**Files modified:** `alita/utils/logredact/reset_testtools.go` (new, `testtools` tag), `alita/config/config_pure_test.go`,
`alita/config/redact_reset_testtools_test.go` (new), `alita/config/redact_reset_default_test.go` (new, `!testtools` no-op)
**Commit:** a72516d
**Applied fix:** `logredact.ResetForTest()` (testtools only) empties the exact-value registry. The config tests call it through
`forgetRegisteredSecrets` (from `setRequiredConfigEnv`, which LoadConfig-based tests use, and from the registration test); a
no-op twin keeps the package compiling without the tag. Test `TestRegisteredSecretsDoNotOutliveTheTest` proves a secret a
subtest registers is gone after it ends.

### IN-07: Rolling-deploy cache caveat for `ai off`

**Files modified:** `AGENTS.md`
**Commit:** 3bc555c
**Applied fix:** Documented next to the other rolling-deploy caveats, in the `chat_raid_detection` bullet: an old-build replica
can re-cache a row without the AI fields, new replicas read AI as on until the 30-minute entry expires or the next write.
Documentation only; the cache key was not versioned.

### IN-08: Spike tool: `-endpoint` unvalidated and an all-failed run exits 0

**Files modified:** `scripts/gemini-spike/main.go`, `scripts/gemini-spike/main_test.go`, `AGENTS.md`
**Commit:** 5ce2deb
**Applied fix:** `-endpoint` must be https, or http for `localhost`, `127.0.0.1` or `::1`, with no credentials in the URL,
checked before anything is sent. A run in which every request failed now returns `exitFailure`. Tests: six bad endpoints are
refused with no request; https and the three loopback hosts are accepted. The existing "a provider failure is counted" case
encoded exit 0 for an all-failed run and now expects exit 1 (its other assertions are unchanged).

### IN-09: Two literals duplicate named constants

**Files modified:** `alita/utils/gemini/response.go`, `alita/modules/raid_ai_image.go`
**Commit:** be28b1a
**Applied fix:** `safeToken` uses `maxTokenRunes`, `raidAIScrub` uses `logredact.Placeholder`. No behaviour change; the existing
Gemini and secrets-never-logged tests cover both.

## Skipped Issues

None. All 14 in-scope findings were fixed.

## Decisions beyond the owner's instructions

- WR-03: the owner's list did not include the reply flag, so the raid request omits `is_reply` as well (the `isReply` field of
  `raidAIJob` is gone). `messages_last_hour` stays at the constant 1 the raid path already sent.
- WR-04: the alert's counts line (`lockdown_alert_counts_ai`) keeps its wording (it does not say the accounts joined and is still
  true); no locale was changed.
- IN-04: the album gate was released on a transient refusal rather than moved after the budget, for the cost reason above.
- IN-06: the reset is a testtools-only helper with a no-op twin in the config tests, not a public API.

---

_Fixed: 2026-10-29_
_Fixer: Claude (gsd-code-fixer)_
_Iteration: 1_
