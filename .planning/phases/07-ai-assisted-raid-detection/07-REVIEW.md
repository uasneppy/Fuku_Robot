---
phase: 07-ai-assisted-raid-detection
reviewed: 2026-10-29T18:00:00Z
depth: deep
files_reviewed: 61
files_reviewed_list:
  - AGENTS.md
  - alita/config/config.go
  - alita/config/config_pure_test.go
  - alita/db/devs/repository.go
  - alita/db/devs/repository_test.go
  - alita/db/models/lockdown.go
  - alita/db/models/raid_detection.go
  - alita/db/raiddetection/repository.go
  - alita/db/raiddetection/repository_test.go
  - alita/db/raiddetection/stats.go
  - alita/db/raiddetection/stats_test.go
  - alita/i18n/lockdown_locale_test.go
  - alita/modules/aispam.go
  - alita/modules/autolockdown.go
  - alita/modules/autolockdown_test.go
  - alita/modules/devs.go
  - alita/modules/lockdown.go
  - alita/modules/lockdown_alert.go
  - alita/modules/lockdown_alert_test.go
  - alita/modules/lockdown_status.go
  - alita/modules/raid_ai.go
  - alita/modules/raid_ai_breaker.go
  - alita/modules/raid_ai_breaker_test.go
  - alita/modules/raid_ai_budget_test.go
  - alita/modules/raid_ai_burst_test.go
  - alita/modules/raid_ai_count.go
  - alita/modules/raid_ai_image.go
  - alita/modules/raid_ai_integration_test.go
  - alita/modules/raid_ai_media.go
  - alita/modules/raid_ai_media_test.go
  - alita/modules/raid_ai_pool_test.go
  - alita/modules/raid_ai_stats_test.go
  - alita/modules/raid_ai_text_test.go
  - alita/modules/raid_join_surge_test.go
  - alita/modules/raid_trigger.go
  - alita/modules/shutdown_drains.go
  - alita/modules/staff_action_fake_test.go
  - alita/modules/test_harness_test.go
  - alita/utils/gemini/client.go
  - alita/utils/gemini/errors.go
  - alita/utils/gemini/gemini_test.go
  - alita/utils/gemini/leaf_test.go
  - alita/utils/gemini/request.go
  - alita/utils/gemini/response.go
  - alita/utils/logredact/logredact.go
  - alita/utils/logredact/logredact_test.go
  - locales/en.yml
  - locales/es.yml
  - locales/fr.yml
  - locales/hi.yml
  - locales/id.yml
  - locales/pt.yml
  - locales/ru.yml
  - main.go
  - migrations/20261029120000_add_raid_detection_ai.sql
  - sample.env
  - scripts/gemini-spike/main.go
  - scripts/gemini-spike/main_test.go
  - scripts/gemini-spike/probe.go
  - scripts/gemini-spike/report.go
  - scripts/gemini-spike/report_test.go
findings:
  critical: 0
  warning: 5
  info: 9
  total: 14
status: issues_found
---

# Phase 7: Code Review Report

**Reviewed:** 2026-10-29
**Depth:** deep
**Files Reviewed:** 61 (the config list; the diff base is `53a48763`)
**Status:** issues_found

## Summary

The review covered the Gemini leaf client, the raid AI watcher, pool, breakers, budgets and verdict counter, the `ai_burst` trigger plumbing, the settings columns and migration, `/autolockdown ai`, the shutdown drain, the locales and the spike tool. Cross-file calls were traced from the group 2 watcher and from `processAISpam` through `raidAIRecordVerdict` and `raidFire` to the alert snapshot.

Checks run here (PostgreSQL was not started):
- `go test -tags testtools -race -count=1` passed for `alita/modules` (whole package, 308 s), `alita/utils/gemini`, `alita/utils/logredact`, `alita/db/raiddetection`, `alita/db/devs`, `alita/db/models`, `alita/config`, `alita/i18n` and `scripts/gemini-spike`.
- `go vet -tags testtools` is clean on the touched packages, and `CGO_ENABLED=0 go build ./...` succeeds.
- `golangci-lint` could not run (the installed binary was built with Go 1.25, the module targets 1.26).
- Migration `20261029120000` and the `chk_chat_raid_detection_ai` CHECK were not executed against PostgreSQL. I read the SQL and the grammar is valid; CI's migration-chain step and `TestRaidDetectionSettings` have to confirm it.

There are no blockers. I tried to build a path where one verdict, one account, or a duplicated delivery locks a group, and none exists:
- **Fire rule.** `raidAICounts` needs `items >= threshold`, `accounts >= 3` and `threshold >= 3`.
- **Counter script.** The verdict script dedupes the message with `ZADD NX` and touches the account only when the item was newly added. Every key of every script carries the `{chat}` tag, and I checked the ARGV order of all three scripts against the Go callers.
- **No Telegram write.** The AI path makes no Telegram write except `getFile` and the designed `raidFire`.
- **Goroutines and drain.** Every goroutine starts with a recover, the drain order is correct (AI spam, raid AI, raid posts, lockdown worker, staff actions, DB close), and `raidAIFires` is awaited after `raidAIJobs`, so the ordering cannot miss a fire that a job started.
- **Settings.** The writes are column-targeted and call `DeleteCache`. The stale-cache normalisation only mutates a freshly decoded value, because a shared loader result always has `ai_threshold >= 3`.
- **`ai_burst` surfaces.** All seven `trigger_kind` sites handle it, and the grep shows no other switch.
- **Locales.** All 7 files match `en.yml` in keys and placeholders, and the help text stays under 4096 UTF-16 units (pt is the tightest at 4025 before the header).
- **Secrets.**
  - The Gemini key travels only in `x-goog-api-key`. Errors are rebuilt from `ue.Err`, `HTTPError.Error()` never prints the server message, and redirects are not followed.
  - The Telegram file URL is never wrapped or logged. The download error is rebuilt, status errors carry the code alone, and the token is scrubbed as a last step.
  - `GEMINI_MODEL` is validated with `ValidModel` and `url.PathEscape`d.
- **Spike tool.** It sends nothing without a key or `-fake-key -probe`, never prints the key, and `go list -deps` shows it imports only `alita/utils/gemini`.

The warnings below are cost, consent, kill-switch, blast-radius and test-reliability problems, not correctness holes in the fire rule.

## Warnings

### WR-01: One new account can exhaust the per-group and the fleet-wide Gemini budget, blinding the AI trigger everywhere

**File:** `alita/modules/raid_ai_image.go:278-301` (`raidAIReserveBudget`), `alita/modules/raid_ai.go:279-321` (`raidAIWatchImage`)

**Issue:** The hourly budgets (150 per group, 500 in all) are keyed by chat and by hour only. Any account that joined in the last 24 h is a candidate for every photo, GIF and sticker it posts, and nothing limits one account. The cache saves a repeat of the same file, but each different photo reserves one unit before the download.

Scenario: one fresh account posts 150 distinct photos. The pool runs about two checks a second, so the chat budget is gone in roughly 75 s. After that every new member's image in that group is `capped` for the rest of the UTC hour. An attacker with a presence in four groups burns the 500 global units, and then no group's image check runs. This is cost-bounded, but it is a cheap way to turn off the AI image trigger just before a real raid. The join and flood triggers still work, so it is a degradation, not an unlock.

The same exhaustion happens by accident in a busy group with a steady stream of new members posting ordinary photos.

**Fix:** Put a small per-account cap in front of the shared budgets, and reserve the shared units only for accounts under it:

```go
// raidAIUserHourlyBudget bounds one account's Gemini calls per hour in one group.
var raidAIUserHourlyBudget int64 = 10

func raidAIReserveBudget(client *redis.Client, chatID, userID int64) bool {
	stamp := raidAIHourStamp(raidNow())
	budgets := []struct {
		key   string
		limit int64
	}{
		{raidKey(chatID, raidPartAIBudget+"u:"+strconv.FormatInt(userID, 10)+":"+stamp), raidAIUserHourlyBudget},
		{raidKey(chatID, raidPartAIBudget+":"+stamp), raidAIChatHourlyBudget},
		{raidAIGlobalBudgetPrefix + stamp, raidAIGlobalHourlyBudget},
	}
	// ...unchanged loop; a user over their own cap spends neither shared unit.
}
```

Pass `job.userID` from `raidAIImageVerdict`, and add a test where one member posts N+1 distinct photos and a second member's photo is still judged.

### WR-02: `ENABLE_AISPAM=false` does not stop the Gemini image checks, and there is no runtime kill switch for them

**File:** `alita/modules/raid_ai.go:73-76` (`raidAIImageConfigured`), `sample.env:114-117`

**Issue:** `raidAITextConfigured()` honours `EnableAISpam`, but `raidAIImageConfigured()` is just `GeminiAPIKey != ""`. `sample.env` still describes `ENABLE_AISPAM` as the "Global kill switch for the AI spam filter ... Set to false to stop all AI spam checks without deploying". With a Gemini key set, setting it to false stops TypeSafe calls and leaves every member image going to Google.

The only ways to stop images are to unset `GEMINI_API_KEY` (a redeploy) or to run `/autolockdown ai off` in every group. For a feature that sends member photos to a third party, an owner who flips the documented switch in an incident would reasonably believe images stopped.

**Fix:** Make the existing switch cover both providers, or add an explicit one and document it:

```go
func raidAIImageConfigured() bool {
	return config.AppConfig.EnableAISpam && config.AppConfig.GeminiAPIKey != ""
}
```

Update the `sample.env` comment (and `raidAIConfigured`'s doc) to say "stops all AI checks, text and images".

### WR-03: Raid-only text checks send chat description, rules and sender profile for chats that never opted in to `/aispam`, and the disclosure and `sample.env` say otherwise

**File:** `alita/modules/raid_ai.go:501-518` (`raidAIProcessText` building `task.state(nil, 1)`), `alita/modules/aispam.go:129-151`, `sample.env:105-107`, `locales/en.yml` (`lockdown_help_msg`, AI paragraph)

**Issue:** `aispamTask.state` includes the chat title, the chat description (a live `getChat`), the chat rules, the sender's days since first seen and warn count, besides the text. `/aispam` discloses all of that in `aispam_data_notice`, and a chat has to switch it on first.

The raid text path now sends the same state to TypeSafe for any supergroup where a new member posts a link, a forward or a repeated text. The trigger is on by default and no admin opted in. The only disclosure is the help text, which says "suspicious text from members who joined in the last 24 hours is checked by TypeSafe". Nothing says the group's description and rules go along with it.

`sample.env` still says "The filter is off in every chat until an admin runs /aispam on", which is no longer true of the TypeSafe call.

D-15 asks only for the text-and-images sentence, so this is a gap in what the decision covers, not a deviation from it.

**Fix:** Preferred: send a reduced state for raid-only jobs, since the raid verdict needs only the text, link domains and forward/reply flags:

```go
state := task.state(nil, 1)
state.Chat.Description, state.Chat.Rules = "", ""
```

Otherwise extend the `lockdown_help_msg` sentence in all 7 locales ("with the group's title, description and rules") and correct the `sample.env` header comment.

### WR-04: The AI-burst snapshot puts established members into "Ban N recent joiners"

**File:** `alita/modules/raid_ai_count.go:210-228` (`raidAISpammersScript`), `alita/modules/lockdown_alert.go:294-307` (`lockdownMergeSnapshot`), `alita/modules/lockdown_alert.go:227-243`

**Issue:** The `/aispam` delete verdicts of established members count (D-19). The snapshot script keeps accounts with no recent `:joined` score and scores them by their verdict time. `lockdownBurstSnapshot` then saves them as `pre` burst rows with a `JoinedAt` that is really a verdict time.

The "Ban N recent joiners" run queues unhandled burst rows, excluding only approved users, admins, the bot and joiners recorded exempt. So a long-standing member who tripped one TypeSafe false positive, plus four others from two more accounts, lands on the ban list. The button is labelled for recent joiners and the card says the accounts are "banned for good".

An admin still presses Confirm and the sample lists the accounts, so this is a risk, not an automatic ban. But the wording promises recent joiners, and "AI only counts" ends up shaping a permanent-ban candidate list from a single model verdict. Pitfall 13 fixed only the counts line.

**Fix:** Save only accounts with a recent join score as `pre` rows (drop the verdict-time-only ones, or save them with a distinct source the ban run skips):

```lua
local joinedAt = redis.call('ZSCORE', KEYS[2], id)
if joinedAt and tonumber(joinedAt) > cutoff then
  redis.call('ZADD', KEYS[3], joinedAt, id)
  kept = kept + 1
end
```

If the owner wants established members in the sample, list them in the text but exclude them from `CountBanCandidatesFresh` and the queue.

### WR-05: The test harness waits for AI jobs and posts but not for the lockdown starts in between

**File:** `alita/modules/test_harness_test.go:158-159`, `alita/modules/staff_action_fake_test.go:195-196`, `alita/modules/lockdown_alert_test.go:61-62`, `alita/modules/raid_ai_integration_test.go:67-68`

**Issue:** Each read helper (`callsFor`, `sentTo`, `sentChats`, `callsSince`) now calls `raidAIJobs.wait()` and then `raidPosts.wait()`. A verdict hands the start to `raidAIFireAsync`, which is counted in `raidAIFires`, a third tracker.

Window: the job finishes (`raidAIJobs` idle), the fire goroutine has not reached `raidPostAsync` yet, and `raidPosts` is idle too. The helper then returns early. A positive assertion on the notice can flake, and a negative one ("no sendMessage", "no setChatPermissions") can pass vacuously.

The AI tests mostly call `settleAI()` first, which waits on all three in the right order, so the suite passes today. The helpers are still wrong for any new test that reads calls straight after sending, and they were changed in this phase to be safe against exactly this.

**Fix:** Wait on the fires between the jobs and the posts in all four helpers:

```go
raidAIJobs.wait()
raidAIFires.wait()
raidPosts.wait()
```

## Info

### IN-01: `raidAIGeminiPause.reason` is written and never read

**File:** `alita/modules/raid_ai_breaker.go:55-59`, `:153`

**Issue:** The `reason` field of the provider-wide pause is assigned under the mutex but nothing reads it (the log line uses the local `reason`). Dead state.

**Fix:** Drop the field, or surface it in `/stats` if it was meant for the owner.

### IN-02: An invalid `GEMINI_MODEL` is logged verbatim before secrets are registered

**File:** `alita/config/config.go:464-468`, `:521`

**Issue:** `setDefaults` runs inside `LoadConfig`, before `registerSecrets(cfg)`. The warning prints the rejected value with `%q`. If two variables are swapped by mistake (a key pasted into `GEMINI_MODEL`), the key is logged unredacted, though the `AIza...` shape pattern would catch a Google key if the hook is already installed.

**Fix:** Log only that the value was rejected and its length, not the value.

### IN-03: The Gemini per-chat breaker silently stops tracking new chats past 10000

**File:** `alita/modules/raid_ai_breaker.go:94-97`

**Issue:** Once the map holds `raidAIGeminiBreakerMaxChats` entries, `raidAIGeminiRecordFailure` returns for an untracked chat, so that chat can never be paused and every one of its images keeps calling a failing provider (within its budget). Entries are never pruned. It matches the `/aispam` breaker's behaviour and is unlikely for a private bot, but the failure is silent.

**Fix:** Evict breakers that are neither paused nor failing when the map is full, or log once when the cap is hit.

### IN-04: Album gate and cross-chat `singleflight` edge cases

**File:** `alita/modules/raid_ai_image.go:148`, `:153-169`, `:192-209`

**Issue:** Two fail-open edges:
- The album gate is taken before the fire-latch check, the cache, the breaker and the budget. If the first job is then refused for a transient reason (a paused chat, a Redis error on the budget), the rest of the album is dropped for 120 s even though they could have been judged.
- `raidAIImageFlight` is keyed by `file_unique_id` only, across chats. A follower from chat B inherits the leader's `ok=false` when the leader's chat was capped or paused, though chat B has budget.

Neither can cause a lock; both only lower the count.

**Fix:** Take the album gate after the budget is reserved (just before the download), and key the flight with the chat ID, or let a follower retry its own verdict when the leader's result is not `ok`.

### IN-05: Real-time sleeps in two tests

**File:** `alita/modules/raid_ai_budget_test.go:168`, `alita/modules/raid_ai_breaker_test.go:103`

**Issue:** The budget test sleeps 200 ms to let three posts overlap. If a slow runner is late, the third job hits the verdict cache and the test passes without having proved the overlap. The TypeSafe breaker test waits out a 50 ms cooldown in real time because that breaker has no clock seam, unlike the Gemini one.

**Fix:** Wait for the leader to be inside the gated handler (a request counter reaching 1) instead of sleeping, and give `aispamBreaker` a clock the way `raidNow` serves the Gemini breaker.

### IN-06: The config test leaves secrets in the global redaction registry

**File:** `alita/config/config_pure_test.go` (`TestGeminiConfig`, "the key is registered")

**Issue:** `registerSecrets` registers three values in the process-wide `logredact` registry and the test never removes them. `logredact`'s own tests use a `reset()`; `config` cannot reach it. Later tests in the package are scrubbed by those values.

**Fix:** Expose a test-only reset (a `//go:build testtools` helper in `logredact`) and call it from `t.Cleanup`.

### IN-07: In a rolling deploy an old replica's cached settings override an admin's `ai off`

**File:** `alita/db/raiddetection/repository.go:65-75`

**Issue:** The normalisation treats `AIThreshold == 0` as "not populated" and sets `AIEnabled = true`. The settings cache is shared in Redis, so an old-build replica that reloads a row after an admin ran `/autolockdown ai off` writes an entry without the AI fields, and new replicas read it as AI on for up to the 30-minute TTL. AGENTS.md accepts rolling-deploy caveats of this class, and a single-replica deploy is unaffected.

**Fix:** Note it next to the other rolling-deploy caveats in AGENTS.md, or version the cache key (`raid_detection_v2`) in the release that adds the columns.

### IN-08: Spike tool: `-endpoint` is unvalidated and an all-failed run exits 0

**File:** `scripts/gemini-spike/main.go:132`, `:431-434`

**Issue:** `-endpoint` accepts any URL, including `http://`, and the real key is sent there in a header. The owner types the flag, so this is unlikely to bite. Separately, when every request fails the tool prints "Every request failed..." but returns `exitOK`, so a wrapper script cannot tell.

**Fix:** Require `https://` unless the host is loopback, and return `exitFailure` when `failed == len(results)`.

### IN-09: Two literals duplicate named constants

**File:** `alita/utils/gemini/response.go:162` (`truncateRunes(s, 40)` against `maxTokenRunes` in `errors.go`), `alita/modules/raid_ai_image.go:105` (`"[REDACTED]"` against `gemini`'s `redacted` and `logredact.Placeholder`)

**Issue:** The same value is spelled in two places, so a change to one leaves the other behind.

**Fix:** Use `maxTokenRunes` in `safeToken`, and `logredact.Placeholder` in `raidAIScrub`.

---

_Reviewed: 2026-10-29_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: deep_
