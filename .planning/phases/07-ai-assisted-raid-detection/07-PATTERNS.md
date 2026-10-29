# Phase 7: AI-Assisted Raid Detection - Pattern Map

**Mapped:** 2026-10-29
**Files analyzed:** 36 (new or modified)
**Analogs found:** 33 / 36 (3 partial: the pool's image download, the album gate, the spike tool)

All analog paths below were verified as git-tracked source in `/home/user/Fuku_Robot` (`git ls-files` non-empty); none is a gitignored mirror. Line numbers are from the files as read on 2026-10-29. Module path is `github.com/divkix/Alita_Robot`.

## File Classification

| New/Modified File | Role | Data Flow | Closest Analog | Match Quality |
|-------------------|------|-----------|----------------|---------------|
| `alita/utils/gemini/{client,request,response,errors}.go` (new, stdlib only) | utility (REST client) | request-response | `alita/modules/aispam_jev.go` | exact (style), different package |
| `alita/utils/gemini/*_test.go` (new) | test | request-response | `alita/modules/aispam_jev_test.go` | exact |
| `alita/modules/raid_ai.go` (watcher, candidate rules, pool, drain, stats) | middleware (watcher) + service (pool) | event-driven + streaming (queue) | `raid_flood.go` `raidFloodCountUpdate` (watcher) + `aispam.go` 378-531 (pool/breaker/drain) | exact + exact |
| `alita/modules/raid_ai_media.go` (media select, token-safe download) | utility | file-I/O | `alita/modules/backup.go` `downloadBackupFile` (+ `backupDownloadHTTPClient`) | role-match |
| `alita/modules/raid_ai_count.go` (verdict script, candidate script, spammer snapshot, lookup seam) | service | event-driven (Redis Lua) | `raid_flood.go` 92-119, 322-405 | exact |
| `alita/modules/raid_ai_breaker.go` (per-chat + provider breaker) | service | request-response | `aispam.go` 465-531 | exact |
| `alita/modules/aispam.go` (modify: `aispamIsDelete`, report delete verdict) | module | event-driven | itself, lines 287-291, 313 | exact |
| `alita/modules/raid_trigger.go` (modify: sentence, notice, snapshot call in `raidFire`) | service | event-driven | itself, 267-277, 331-346, 359-373 | exact |
| `alita/modules/lockdown_alert.go` (modify: trigger line, counts key, burst snapshot, generalise merge) | service | batch | itself, 209-221, 251-305, 474-478, 543-568 | exact |
| `alita/modules/lockdown_status.go` (modify: `lockdownStartedAutomatically`) | service | request-response | itself, 107-112 | exact |
| `alita/db/models/lockdown.go` (modify: `LockdownTriggerAIBurst`) | model | CRUD | itself, constants near `LockdownTriggerFlood` | exact |
| `alita/modules/autolockdown.go` (modify: `ai` word) | controller | request-response | itself, 21-25, 85-92, 133-147, 161-168 | exact |
| `alita/modules/autolockdown_test.go`, `raid_*_test.go` (modify/new tests) | test | request-response | `autolockdown_test.go`, `raid_flood_test.go`, `raid_trigger_test.go`, `raid_status_test.go`, `raid_notice_test.go` | exact |
| `alita/db/raiddetection/repository.go` (modify: defaults, normalisation, 2 setters) | repository | CRUD (cached read, column upsert) | itself, 37-68, 78-145 | exact |
| `alita/db/raiddetection/stats.go` (modify: `aiOff`) + `alita/db/devs/repository.go` | repository | CRUD (read) | `stats.go` `LoadRaidDetectionStats`, `devs/repository.go:185` | exact |
| `alita/db/models/raid_detection.go` (modify: `AIEnabled`, `AIThreshold`) | model | CRUD | itself, lines 13-22 | exact |
| `migrations/2026102x..._add_raid_detection_ai.sql` (new) | migration | batch (DDL) | `migrations/20261018120000_add_lockdown_trigger_columns.sql` + `20261018130000_add_chat_raid_detection.sql` | exact |
| `alita/config/config.go` (modify: `GeminiAPIKey`, `GeminiModel`, `RegisterSecret`) | config | transform | `TypeSafeAPIKey` at config.go:195, 330, 486-493 | exact |
| `main.go` (modify: drain registration) + `alita/modules/shutdown_drains.go` (new `RegisterRaidAIDrain`) | config / lifecycle | event-driven | `RegisterRaidPostsDrain` (shutdown_drains.go:39-50, main.go ~221) | exact |
| `alita/modules/devs.go` (modify: AI stats section) | controller | request-response | `GetAISpamStats` (aispam.go:80-92) + `getStats` (devs.go:~388) | role-match (no consumer yet) |
| `locales/{en,es,fr,hi,id,pt,ru}.yml` (modify, 7 files) | config | transform | `lockdown_trigger_flood`, `lockdown_alert_trigger_flood`, `lockdown_alert_counts_flood`, `lockdown_auto_started_flood`, `lockdown_auto_flood_on` in `locales/en.yml` | exact |
| `docs/.../api-reference/{environment,database-schema,commands}.md`, `commands/aispam/index.md`, `sample.env`, generated `commands/lockdown/index.md` | docs/config | - | `TYPESAFE_API_KEY` entries (environment.md:59, 572; sample.env:107-112) | exact |
| `AGENTS.md` (modify) | docs | - | existing Handlers / Data / Startup bullets | exact |
| `alita/modules/test_harness_test.go` fakes + new `httptest` fakes | test | request-response | `moduleBotClient` (test_harness_test.go:42-152), `lockdownFake`, `withAISpamJevServer`, `TestDownloadBackupFileDownloadsGetFilePath` | exact |
| `scripts/gemini-spike/main.go` (+ pure-function tests) (new) | utility (CLI tool) | batch | `scripts/check_test_results/main.go` (stdlib-only `package main`) | partial |
| `07-SPIKE.md` (new, docs) | docs | - | none (RESEARCH Pattern 10 is the spec) | none |

## Pattern Assignments

### `alita/utils/gemini/` (utility, request-response)

**Analog:** `alita/modules/aispam_jev.go` (copy the shape, not the package: the new package must import only the stdlib, never `alita/config`, `alita/db` or `alita/modules`, which would drag in the pre-`main` Postgres `init()`).

**Constants and swappable endpoint/client** (aispam_jev.go:20-48). Copy the style; the Gemini endpoint is `<base>/v1beta/models/<url.PathEscape(model)>:generateContent`, key in `x-goog-api-key`, never in the URL:
```go
const (
	aispamJevTimeout = 8 * time.Second
	aispamJevRetryWait = 500 * time.Millisecond
	aispamJevMaxRetryWait = 2 * time.Second
	aispamJevMaxResponseBytes = 1 << 20
)

// Swappable so tests can point at an httptest server; nothing else sets them.
var (
	aispamJevEndpoint = "https://api.typesafe.ai/v1/systemone"
	aispamHTTPClient  = &http.Client{
		Timeout: aispamJevTimeout,
		Transport: &http.Transport{MaxIdleConns: 10, IdleConnTimeout: 90 * time.Second},
	}
)
```
In the leaf package prefer a `Client{Endpoint, APIKey, Model, HTTP}` struct (RESEARCH layout) so tests and the spike construct their own; keep exported package vars out.

**Typed HTTP error with retryability** (aispam_jev.go:157-170): `status` + `retryAfter`, `retryable()` = 429 or >=500. Gemini adds `retryDelay` from the error envelope (`"34s"`, parse with `strconv.ParseFloat(strings.TrimSuffix(s,"s"),64)`). `Error()` must print only the status, never the URL, key or body (Pitfall 5).

**Retry-once core** (aispam_jev.go:175-201), copy verbatim in shape:
```go
ctx, cancel := context.WithTimeout(ctx, aispamJevTimeout)
defer cancel()
verdict, err := aispamJevPost(ctx, apiKey, state)
if err == nil { return verdict, nil }
var httpErr *aispamHTTPError
if !errors.As(err, &httpErr) || !httpErr.retryable() { return nil, err }
wait := httpErr.retryAfter
if wait <= 0 { wait = aispamJevRetryWait }
if wait > aispamJevMaxRetryWait { return nil, err }
if !aispamSleepContext(ctx, wait) { return nil, err }
return aispamJevPost(ctx, apiKey, state)
```

**Request/post/body handling** (aispam_jev.go:203-242): `http.NewRequestWithContext`, header set, `defer` that drains with `io.LimitReader(resp.Body, max)` then closes, non-2xx returns the typed error, 2xx decodes through `io.LimitReader`. Differences to apply: `x-goog-api-key` header instead of `Authorization: Bearer`; wrap transport errors by rebuilding from `*url.Error.Err` (the request error text holds the URL); do not `%w`-wrap the build-request error.

**Strict verdict parse** (aispam_jev.go:244-265 is the shape; RESEARCH "Strict verdict parse" is the spec): reject NaN, Inf, `<0`, `>1`, missing probability, `promptFeedback.blockReason`, empty candidates, `finishReason != STOP`, all as a sentinel `ErrNoVerdict`.
```go
if math.IsNaN(probability) || math.IsInf(probability, 0) || probability < 0 || probability > 1 {
	return nil, fmt.Errorf("aispam: delete probability %v is outside [0,1]", probability)
}
```
Also copy `aispamParseRetryAfter` (267-285) and `aispamSleepContext` (287-298) into the leaf package (they are unexported in `modules`; do not export them across the boundary, the leaf package cannot import `modules`).

**Import-isolation test:** add a test in `alita/utils/gemini` asserting its imports exclude `alita/config` and `alita/db` (RESEARCH Pitfall 4).

---

### `alita/utils/gemini/*_test.go` (test, request-response)

**Analog:** `alita/modules/aispam_jev_test.go`

**httptest server helper** (lines 16-32): swap endpoint and `server.Client()`, restore in `t.Cleanup`. In the leaf package pass `server.URL` into the `Client` instead of mutating globals.
```go
func withAISpamJevServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	previousEndpoint := aispamJevEndpoint
	aispamJevEndpoint = server.URL
	...
```
**Table-driven fail-open cases** (lines 166-220: server error, 401 not retried, malformed JSON, missing probability, out-of-range probability, hung response). Reuse the case list and add Gemini ones: block reason, `finishReason` MAX_TOKENS/SAFETY, empty candidates, 429 with `retryDelay` over 2 s (no retry), 429 within 2 s (one retry, assert request count). Also assert the key never appears in any returned error string and the request URL has no `key=`. Build tag line 1: `//go:build testtools`. No `t.Skip` anywhere (the `make test` gate fails on skips).

---

### `alita/modules/raid_ai.go` (middleware watcher + pool, event-driven)

**Watcher analog:** `alita/modules/raid_flood.go` `raidFloodCountUpdate` (lines 270-320) and its two thin wrappers (247-259).

**Wrapper + recover pattern** (247-250):
```go
func (m moduleStruct) raidFloodWatcher(b *gotgbot.Bot, ctx *ext.Context) error {
	defer error_handling.RecoverFromPanic("raidFloodWatcher", "Lockdown")
	return raidFloodCountUpdate(b, ctx, false)
}
```
**Check order, cheapest first, every path `ext.ContinueGroups`** (270-304): chat type -> `raidFloodSenderCounts(b, ctx)` (reuse as is, 199-213: excludes `sender_chat`, automatic forwards, service IDs, bot, non-positive IDs) -> item classification (`raidFloodItemKey`, 133-158, reuse for the "image-capable media / text" decision) -> `raidRedis()` nil -> settings via `raidSettingsLookup` under `raidSettingsTimeout` -> approved (`chat_status.IsApprovedForUpdate`) -> admin (`antifloodModule.userIsFloodExempt`):
```go
settingsCtx, cancel := context.WithTimeout(context.Background(), raidSettingsTimeout)
settings, err := raidSettingsLookup(settingsCtx, chat.Id)
cancel()
if err != nil {
	log.Debugf("[RaidDetection] settings of chat %d could not be read, counting nothing: %v", chat.Id, err)
	return ext.ContinueGroups
}
if settings == nil || !settings.FloodEnabled { return ext.ContinueGroups }   // AI: !settings.AIEnabled
if chat_status.IsApprovedForUpdate(b, ctx, chat.Id, userID) { return ext.ContinueGroups }
if antifloodModule.userIsFloodExempt(b, chat.Id, userID) { return ext.ContinueGroups }
```
Registration: `LoadLockdown` (lockdown.go:550-560) adds the flood watchers with `dispatcher.AddHandlerToGroup(handlers.NewMessage(message.All, lockdownModule.raidFloodWatcher), raidFloodGroup)`. Register the AI watcher the same way at group 2 (unused today; document it in AGENTS.md Handlers). Returning `nil` instead of `ext.ContinueGroups` would kill later group-2 handlers.

**Pending-captcha and admin/approved carve-out:** reuse the body of `aispamCarvedOut` (aispam.go:234-244), including the "only query the attempt when the chat has one pending" shortcut:
```go
pending, err := captcha.HasPendingCaptchaAttemptsContext(context.Background(), chatID)
if err != nil || pending {
	if attempt, err := captcha.GetCaptchaAttempt(userID, chatID); err == nil && attempt != nil { return true }
}
return chat_status.IsUserAdminForUpdate(b, ctx, chatID, userID) || chat_status.IsApprovedForUpdate(b, ctx, chatID, userID)
```
**Link/forward tests:** `aispamLinkDomains(msg, text)` (aispam.go:~705) and `msg.ForwardOrigin != nil` (aispam.go:184). **"Will /aispam judge it" predicate** = the gates at aispam.go:155 and 166: `config.AppConfig.EnableAISpam && config.AppConfig.TypeSafeAPIKey != "" && aispam.IsAISpamEnabled(chat.Id) && !aispamBreakerPaused(chat.Id)`.

**Pool analog:** `alita/modules/aispam.go` 378-463. Copy the structure but give the raid pool its own vars, and replace `sync.WaitGroup` with `raidPostTracker` (raid_trigger.go:72-119; its doc explains why a bounded drain can't use a WaitGroup).
```go
aispamWorkersOnce.Do(func() {
	aispamQueue = make(chan aispamTask, aispamQueueCapacity)       // raid pool: cap 128, 4 workers
	for range aispamWorkerCount { go aispamWorker() }
})
func aispamWorker() {
	defer error_handling.RecoverFromPanic("aispamWorker", "aispam")
	for task := range aispamQueue {
		func() {
			defer aispamInFlight.Done()
			defer error_handling.RecoverFromPanic("processAISpam", "aispam")
			processAISpam(task)
		}()
	}
}
// enqueue: counter bumped BEFORE the draining flag is read
aispamInFlight.Add(1)
if aispamDraining.Load() { aispamInFlight.Done(); aispamShed.Add(1); return false }
select {
case aispamQueue <- task: return true
default: aispamInFlight.Done(); aispamShed.Add(1); log.WithFields(...).Debug("[AISpam] queue full, check shed"); return false
}
```
**Stats counters** (aispam.go:80-92): `atomic.Int64` for checked, spam (deleted), kept, shed, failed, plus the research's `blocked`; a `GetRaidAIStats()` mirror of `GetAISpamStats`.

**Fire off the worker:** `raidPostAsync(name, fn)` (raid_trigger.go:127-134) is the exact template for `raidAIFireAsync`: `start()`, goroutine, `defer finish()`, `defer error_handling.RecoverFromPanic(name, "Lockdown")`. Use a second `raidPostTracker` for fires and drain both.

**Drain:** `StopRaidAI()` copies `StopRaidPosts` (raid_trigger.go:141-149): set draining, `select` on `idleChan()` vs `time.After(raidAIStopWait)` (3 s), log a Warn on timeout, then cancel the pool context so in-flight HTTP aborts. Make the wait a `var` so tests shorten it (raid_trigger.go:65).

**Cost guards (Pitfall 10, D-20):** `INCR` budget keys with `raidKey(chat, "ab:"+hour)` and a global `alita:raid:aibudget:<yyyymmddhh>`, `EXPIRE` 2 h; verdict cache `alita:raid:aiv:<file_unique_id>`. Redis calls follow the `raidRedisTimeout` + `cache.Context` shape used throughout (raid_flood.go:225-226).

---

### `alita/modules/raid_ai_count.go` (service, Lua counters)

**Analog:** `alita/modules/raid_flood.go`.

**Counter script** (lines 92-119): `redis.NewScript`, `ZADD`, trim by `now - window`, per-key cap loop, `PEXPIRE`, returns sizes. The verdict script (RESEARCH Pattern 3) is this shape with `ZADD ... NX` on the items set and the account `ZADD` only when `added == 1`:
```lua
local now = tonumber(ARGV[1])
local cutoff = now - tonumber(ARGV[2])
redis.call('ZADD', KEYS[1], now, ARGV[4])
redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', cutoff)
...
local cap = tonumber(ARGV[6])
for i = 1, 2 do
  local n = redis.call('ZCARD', KEYS[i])
  if n > cap then redis.call('ZREMRANGEBYRANK', KEYS[i], 0, n - cap - 1) end
end
for i = 1, 3 do redis.call('PEXPIRE', KEYS[i], ARGV[7]) end
return {redis.call('ZCARD', KEYS[1]), ...}
```
**Go wrapper** (`recordRaidFloodItem`, 218-243): build keys with `raidKey(chatID, part)` (raid_counters.go:57-59, chat ID is the `{hash tag}`), `context.WithTimeout(cache.Context, raidRedisTimeout)`, `script.Run(ctx, client, keys, args...).Int64Slice()`, check `len(sizes)`, return `(items, accounts, err)`; the caller treats an error as "no information". Use `raidNow().UnixMilli()` so tests can move time.

**Pure fire rule** (183-192 `raidFloodCounts`): write `raidAICounts(items, accounts, threshold) bool` = `threshold >= raiddetection.MinThreshold && items >= threshold && accounts >= raidAIMinAccounts`; unit-test it like `TestRaidFloodRule`.

**Cap var for tests** (raid_flood.go:90): `var raidFloodCap = 2000` "variable only so a test can make the cap small". Do the same for `raidAICap` (must stay >= 500).

**Candidate script** (RESEARCH Pattern 4): reads `ZSCORE` on `raidKey(chat, raidPartJoined)` with `raidRecentWindow` (raid_counters.go:34). The in-script `ZSCORE` + recent-window test is lines 105-106 of the flood script.

**Spammer snapshot** (lines 332-372): copy `raidFloodersScript` + `raidSnapshotFlooders`:
```go
redis.call('DEL', KEYS[3])
local ids = redis.call('ZREVRANGE', KEYS[1], 0, tonumber(ARGV[3]) - 1)
for _, id in ipairs(ids) do
  local joinedAt = redis.call('ZSCORE', KEYS[2], id)
  if joinedAt and tonumber(joinedAt) > cutoff then redis.call('ZADD', KEYS[3], joinedAt, id) ... end
end
if kept > 0 then redis.call('PEXPIRE', KEYS[3], ARGV[4]) end
```
Difference (RESEARCH Pitfall 13): keep an established member who has no `:joined` score, scored by the verdict time (score from the accounts set). Reuse `raidFloodersMax` (500) and `raidFloodersTTL` (15 min), new key part `aispammers`.

**Snapshot read + test seam** (374-405): copy `readRaidFlooders` and `var raidFloodersLookup = readRaidFlooders` as `readRaidAISpammers` / `raidAISpammersLookup` (variable "only so a test can see whether it is called"). Its element type is `lockdownRecentJoin{UserID, At}`.

---

### `alita/modules/raid_ai_media.go` (utility, file-I/O)

**Analog (partial):** `alita/modules/backup.go` `downloadBackupFile`, with package vars `backupDownloadBaseURL` / `backupDownloadHTTPClient` (backup.go:54-55) and constant `maxBackupFileSize` (line ~58). It uses `getFile` then an HTTP GET against a swappable base URL. Do NOT copy its error handling: the Phase 7 download must rebuild errors from `url.Error.Err` so the Telegram token never appears (RESEARCH "Telegram download without leaking the token"; gotgbot only sanitises its own method calls). Differences: dedicated client with `CheckRedirect` returning `http.ErrUseLastResponse`, 10 s timeout, `io.LimitReader(resp.Body, 5<<20+1)`, MIME sniff with `http.DetectContentType`, allow only jpeg/png/webp, reject empty or `/`-prefixed `FilePath`.

**Media selection table** is RESEARCH Pattern 8 (largest photo size nearest 1024 px; static sticker as WebP; animated/video sticker, animation, video -> `Thumbnail`). The item key and album handling reuse `raidFloodItemKey` (raid_flood.go:154-157: `g:<media_group_id>` else `m:<message_id>`).

**Test analog:** `TestDownloadBackupFileDownloadsGetFilePath` (backup_test.go:214-245): `httptest.NewServer`, set the base URL var to `server.URL + "/file/bot"`, set `client.responses["getFile"] = json.RawMessage(`{"file_id":"..","file_path":"photos/a.jpg"}`)`, assert `client.callsFor("getFile")` length. `moduleBotClient.FileURL` (test_harness_test.go:150-152) returns `https://api.telegram.org/file/bot<token>/<path>`, so a Phase 7 test must either override `FileURL` on a wrapper client or make the download base URL a package var (preferred, as backup does). Add assertions that a failing server's error string contains neither `999:test` nor the server host path.

---

### `alita/modules/raid_ai_breaker.go` (service, request-response)

**Analog:** `alita/modules/aispam.go` 465-531, a per-chat breaker with a mutex-guarded map, bounded size, cooldown var, and a "trips exactly once" return:
```go
var aispamBreakers = struct {
	mu sync.Mutex
	m  map[int64]*aispamBreaker
}{m: make(map[int64]*aispamBreaker)}
...
if !ok {
	if len(aispamBreakers.m) >= aispamBreakerMaxChats { return false }
	breaker = &aispamBreaker{}
	aispamBreakers.m[chatID] = breaker
}
if !breaker.pausedTill.IsZero() && !now.Before(breaker.pausedTill) { /* reset */ }
breaker.failures++
if breaker.failures < aispamBreakerFailures || breaker.notified { return false }
breaker.pausedTill = now.Add(aispamBreakerCooldown)
```
Copy into a Gemini-only map (own cap, own cooldown var for tests). Post NO chat notice from the raid path (`aispamNotifyBreaker`, aispam.go:337-345, must not be called). Add the provider-wide pause (bad key, or 429 with `retryDelay` > 2 s) with one Warn per pause. TypeSafe raid-only jobs reuse `aispamBreakerPaused/RecordFailure/RecordSuccess` and ignore the "tripped" return.

---

### `alita/modules/aispam.go` (modify)

**Analog:** itself. Extract the bar selection at lines 287-291 so both paths share it:
```go
threshold := aispamThresholdNonEnglish
if verdict.Language == aispamLanguageEnglish { threshold = aispamThresholdEnglish }
deleting := verdict.DeleteProbability >= threshold
```
-> `aispamIsDelete(verdict) bool` (0.8 English, 0.9 otherwise). Report a delete verdict after line 313 (`aispamDeleted.Add(1)`) by calling the new `raidAIRecordVerdict(...)` (a Redis script plus an async fire), which needs a `chat gotgbot.Chat` field added to `aispamTask` (struct at 112-125). Keep `processAISpam` otherwise unchanged (D-03). Raid-only text jobs must NOT call `observeAISpamSender` (aispam.go:578-615 stores raw text) and must build a state without history (`task.state(nil, 1)`, see `state()` at 127).

**Existing tests to extend:** `alita/modules/aispam_test.go` (not read in detail; planner should grep for the `processAISpam` tests and add one asserting a delete verdict reaches the counter and a keep verdict does not, D-19).

---

### The seven `trigger_kind` switch sites (modify) and their tests

All currently fall through to "manual" for an unknown kind. Add `ai_burst` to each (RESEARCH Pattern 5 table, lines re-verified):

| Site | File:lines | Current code | Needed |
|------|-----------|--------------|--------|
| constant | `alita/db/models/lockdown.go` near `LockdownTriggerFlood = "flood"` (line ~29) | `LockdownTriggerManual`, `JoinSurge`, `Flood` | `LockdownTriggerAIBurst = "ai_burst"` (column `size:16`, no CHECK, so no migration) |
| `lockdownTriggerSentence` | `raid_trigger.go:331-346` | `switch kind { case JoinSurge: ... case Flood: ... } return ""` | `case AIBurst` -> `lockdownText(tr, "lockdown_trigger_ai_burst", i18n.TranslationParams{"count": strconv.Itoa(count), "window": strconv.Itoa(window)})` (covers `raidWarnCannotLock` at 457 too) |
| `raidPostNotice` | `raid_trigger.go:359-373` | `default: return` (no notice for an unknown kind) | `case AIBurst` -> `lockdown_auto_started_ai_burst` with `count`, `window` |
| `lockdownStartedAutomatically` | `lockdown_status.go:107-112` | `ld.TriggerKind == JoinSurge \|\| ld.TriggerKind == Flood` | add `\|\| == AIBurst` |
| `lockdownAlertTriggerLine` | `lockdown_alert.go:543-568` | falls to `lockdown_alert_trigger_manual` + `lockdownMention` | `case AIBurst` -> `lockdown_alert_trigger_ai_burst` with `count`, `window`, `time` |
| `lockdownRenderSample` counts key | `lockdown_alert.go:474-478` | `countsKey := "lockdown_alert_counts"; if kind == Flood { countsKey = "lockdown_alert_counts_flood" }` | add AI branch -> `lockdown_alert_counts_ai` (must not say "joined", Pitfall 13) |
| `lockdownBurstSnapshot` | `lockdown_alert.go:219-221` | `if ld.TriggerKind == models.LockdownTriggerFlood { joins = lockdownMergeFlooders(b, ld, joins) }` | add `else if AIBurst { joins = lockdownMergeAISpammers(...) }` |
| `raidFire` snapshot | `raid_trigger.go:271-277` | `if trig.Kind == models.LockdownTriggerFlood { if saved, snapErr := raidSnapshotFlooders(chat.Id); ... }` | add the AI branch calling `raidSnapshotAISpammers(chat.Id)` with the same debug logging, before `startLockdownRow` |

(That is eight rows counting the constant; the research's "seven switch sites" are rows 2-8.)

**Merge generalisation:** `lockdownMergeFlooders` (lockdown_alert.go:251-305) merges a snapshot with the tracker joins by user ID (tracker wins name/username/link/bot flag, earliest time wins, unnamed accounts named by one `user.FindUsersByIDs`, bot skipped, sorted by time then ID). RESEARCH suggests `lockdownMergeSnapshot(b, ld, joins, lookup)` plus two thin wrappers instead of a 55-line copy; the only line that differs is the lookup:
```go
flooders, err := raidFloodersLookup(ld.ChatID)
if err != nil {
	log.Warnf("[Lockdown] the flooding accounts of chat %d could not be read for the alert of lockdown %d: %v", ...)
	return joins
}
```

**Test analogs for these sites:**
- Status/reminder wording by kind: `raid_status_test.go` helpers `surgeLockdown()` / `floodLockdown()` (lines 16-37) then `TestRaidLockdownStatus` / `TestRaidLockdownReminder`. Add `aiBurstLockdown()` beside them (seed spammers instead of joins) and one table-driven test that renders every surface for each kind and fails if `ai_burst` produces the manual wording (Pitfall 2).
- Public notice: `raid_notice_test.go` `TestRaidAutoNotice` (line 24) and `wantOneNoticeIn()`.
- One lockdown from many triggers/replicas: `raid_trigger_test.go` `TestRaidTriggersSingleLockdown` (104), `TestRaidTriggersConcurrent` (240), `TestRaidCooldownAfterLift` (306), `TestRaidLatch` (359).
- Snapshot/alert sample: `raid_flood_burst_test.go` (the flooder snapshot to alert path), `TestRaidFloodScript` (raid_flood_test.go:562) for the script-level fixture with a fixed clock.

---

### `alita/modules/autolockdown.go` (controller, request-response)

**Analog:** itself. Exact edit points:
- Constants (22-25): add `autoTriggerAI = "ai"`.
- `parseAutoLockdownArgs` (85-92): add `case autoTriggerAI: change.Trigger = autoTriggerAI`. The 2-word grammar, `on`/`off`/number handling and 3..500 range check (93-110) are inherited unchanged.
- `writeAutoLockdown` (133-147): add `case autoTriggerAI:` -> `raiddetection.SetAIThreshold(chatID, change.Threshold)` if `SetThreshold` else `raiddetection.SetAIEnabled(chatID, change.Enable)`.
- `autolockdownStatusText` (161-168): add a line `autolockdownTriggerLine(tr, s.AIEnabled, s.AIThreshold, "lockdown_auto_ai_on", "lockdown_auto_ai_off")` and, when no provider is configured, `lockdown_auto_ai_unavailable`.
- Authority, anonymous-admin proof and `RegisterAnonymousAdminHandler` re-entry are decided before the word is parsed (`requireAutoLockdownAuthority`, 48-57), so nothing to add.
```go
switch strings.ToLower(args[0]) {
case autoTriggerJoins: change.Trigger = autoTriggerJoins
case autoTriggerFlood: change.Trigger = autoTriggerFlood
default: return autoLockdownChange{}, errAutoLockdownUsage
}
```
**Tests** (`autolockdown_test.go`): extend the `TestAutoLockdownArgs` table (157-200, entries like `{name: "flood on", args: []string{"flood","on"}, want: autoLockdownChange{Trigger: autoTriggerFlood, Enable: true}}`) with `ai` rows incl. range errors; add an end-to-end `ai 8` case modelled on `TestAutoLockdown` "joins 20 ..." (54-100: `env.send(env.admin, "/autolockdown ...")`, `env.wantReplyHas(staffMarker(...))`, `env.wantAutoRow(...)`, `dbcache.ResetLocalForTest()` to simulate another replica); update `wantAutoRow` (line 33) to also check the AI columns. Authority cases: `TestAutoLockdownAuthority` (221) already covers non-admin / no restrict / anonymous admin; add one `ai` change by an administrator without `can_restrict_members`.

---

### `alita/db/raiddetection/repository.go` (repository, cached read + column upsert)

**Analog:** itself. Edits:
- Constants (21-32): add `DefaultAIThreshold = 5` (reuse `MinThreshold`/`MaxThreshold`).
- `Defaults` (39-47): add `AIEnabled: true, AIThreshold: DefaultAIThreshold`.
- Column sets (78-83): add `aiEnabledColumns = []string{"ai_enabled","updated_at"}`, `aiThresholdColumns = []string{"ai_enabled","ai_threshold","updated_at"}`.
- Setters copy lines 128-145 exactly:
```go
func SetFloodThreshold(chatID int64, threshold int) error {
	if !thresholdInRange(threshold) { return ErrThresholdOutOfRange }
	row := Defaults(chatID)
	row.FloodEnabled = true
	row.FloodThreshold = threshold
	return upsertSettings(row, floodThresholdColumns)   // ends with cache.DeleteCache(cache.CacheKey(cacheModule, row.ChatID))
}
```
- **Stale-cache normalisation** (RESEARCH Pattern 7, the high-value trap): `GetSettings` (52-68) returns `cache.GetFromCacheOrLoad(...)` directly. Wrap it: take the result, and `if row.AIThreshold == 0 { row.AIEnabled = true; row.AIThreshold = DefaultAIThreshold }` on the value returned from the cache-or-load call, not only inside the loader (an entry cached by the pre-Phase-7 build decodes with zero AI fields). Note the loader at 58-66 returns a pointer; copy before mutating if the cache can hand back a shared pointer.
- `skipLocal` (`alita/db/cache/local.go`) already lists `raid_detection:*`; no change.

**Tests:** `alita/db/raiddetection/repository_test.go` / `testmain_test.go` (`TestRaidDetectionSettings` is the Postgres-integrity guarantee). Add: column-targeted upsert never clobbers the other triggers (AI setter after a flood setter), range error before any write, and `TestRaidDetectionStaleCacheDefaults` (write an old-shape value through the cache, read it back normalised). The model is already in the harness `AutoMigrate` lists (`alita/modules/test_harness_test.go`, `alita/db/devs/testmain_test.go`, `alita/db/raiddetection/testmain_test.go`); new columns come from the struct.

---

### `alita/db/models/raid_detection.go` (model)

**Analog:** itself, lines 13-22. Add after the flood fields, same tag style, still no `default:` tags on booleans:
```go
FloodEnabled   bool `gorm:"column:flood_enabled;not null" json:"flood_enabled"`
FloodThreshold int  `gorm:"column:flood_threshold;not null;check:chk_chat_raid_detection_flood,flood_threshold BETWEEN 3 AND 500" json:"flood_threshold"`
```
-> `AIEnabled bool \`gorm:"column:ai_enabled;not null" json:"ai_enabled"\`` and `AIThreshold int \`gorm:"column:ai_threshold;not null;check:chk_chat_raid_detection_ai,ai_threshold BETWEEN 3 AND 500" json:"ai_threshold"\``. Update the struct doc comment (lines 5-12).

---

### `migrations/2026102x..._add_raid_detection_ai.sql` (migration, DDL)

**Analog:** `migrations/20261018120000_add_lockdown_trigger_columns.sql` (header comment style, lines 1-24) and `20261018130000_add_chat_raid_detection.sql` (named `CONSTRAINT chk_chat_raid_detection_*`). Latest existing name is `20261018130000`, so pick a greater timestamp (e.g. `20261025120000`). Style to copy:
```sql
-- Every statement is idempotent; the Go runner applies each file once inside a
-- transaction and records its sha256 in schema_migrations.
ALTER TABLE chat_lockdowns ADD COLUMN IF NOT EXISTS trigger_count INTEGER NOT NULL DEFAULT 0;
```
Target (RESEARCH Pattern 7): `ALTER TABLE chat_raid_detection ADD COLUMN IF NOT EXISTS ai_enabled BOOLEAN NOT NULL DEFAULT TRUE;` and `ADD COLUMN IF NOT EXISTS ai_threshold INTEGER NOT NULL DEFAULT 5 CONSTRAINT chk_chat_raid_detection_ai CHECK (ai_threshold BETWEEN 3 AND 500);`. No top-level BEGIN/COMMIT, no `CONCURRENTLY`. `runner.go` and `scripts/migrate_psql.sh` are generic; the migration name appears only in `alita/db/migrations/raid_carry_over_test.go:17` (that test executes migration `20261018130000` with an explicit column list, so extra defaulted columns must not break it; the planner should confirm in CI via `make test-postgres-integrity`, which cannot be run locally here since PostgreSQL must not be started).

---

### `alita/db/raiddetection/stats.go` + `alita/db/devs/repository.go` (read) and `alita/modules/devs.go`

**Analog:** `LoadRaidDetectionStats` (stats.go:12-17):
```go
func LoadRaidDetectionStats() (configured, joinOff, floodOff int64) {
	configured = db.CountRows(&models.ChatRaidDetection{}, "")
	joinOff = db.CountRows(&models.ChatRaidDetection{}, "join_enabled = ?", false)
	floodOff = db.CountRows(&models.ChatRaidDetection{}, "flood_enabled = ?", false)
	return
}
```
Add `aiOff` with `"ai_enabled = ?", false`; the only caller is `alita/db/devs/repository.go:185` (`raidCustom, raidJoinOff, raidFloodOff := raiddetection.LoadRaidDetectionStats()`), plus `stats_test.go:12-32` (update the four-value destructuring and add an AI-off row to the fixture). `alita/db/devs` cannot import `alita/modules`, so the in-memory counters of the AI pool are appended in `alita/modules/devs.go` (`stats := devs.LoadAllStats()` at ~line 388) as an extra section built from `GetAISpamStats` / the new `GetRaidAIStats`. Note `GetAISpamStats` currently has no consumer anywhere, so there is no existing section to copy: format the new section in the same HTML style `LoadAllStats` uses and say the counters are per replica.

---

### `alita/config/config.go` and `logredact` registration

**Analog:** `TypeSafeAPIKey`.
- Struct field (config.go:192-195): comment + `EnableAISpam bool` / `TypeSafeAPIKey string`. Add `GeminiAPIKey string` and `GeminiModel string`.
- Loader (line 330): `TypeSafeAPIKey: os.Getenv("TYPESAFE_API_KEY"),` -> `GeminiAPIKey: os.Getenv("GEMINI_API_KEY")`, `GeminiModel` default `gemini-3.5-flash-lite`, validated against `^[A-Za-z0-9._-]{1,64}$` (fall back to default with a warning; Pitfall 6).
- Redaction (lines 486-493), add `cfg.GeminiAPIKey` to the list (empty values are ignored below 6 chars by `logredact`, `minSecretLen = 6`):
```go
logredact.RegisterSecret(
	cfg.BotToken, cfg.DatabaseURL, cfg.RedisPassword, cfg.WebhookSecret, cfg.MetricsAuthToken,
	cfg.TypeSafeAPIKey,
)
```
Optional backstop: one structural pattern for `AIza[0-9A-Za-z_-]{35}` in `logredact.structuralPatterns` (logredact.go, ~line 25); add a test beside the existing logredact tests that a Gemini key in an error string is scrubbed. `sample.env:105-117` and `docs/src/content/docs/api-reference/environment.md` (heading at line 59, sample list near 572; marked MANUALLY MAINTAINED) get matching `GEMINI_*` entries.

---

### `main.go` + `alita/modules/shutdown_drains.go` (drain registration)

**Analog:** `RegisterRaidPostsDrain` (shutdown_drains.go:39-50) and its call in main.go (~lines 217-221, comment "Also after the DB-close handler ...").
```go
func RegisterRaidPostsDrain(m *shutdown.Manager) {
	m.RegisterHandlerWithTimeout(func() error {
		log.Info("[Shutdown] Draining raid detection posts...")
		StopRaidPosts()
		return nil
	}, raidPostStopWait+shutdownDrainGrace)
}
```
Add `RegisterRaidAIDrain` with `raidAIStopWait+shutdownDrainGrace`, and call it in main.go immediately AFTER `modules.RegisterRaidPostsDrain(shutdownManager)` (LIFO: it then runs before the raid-posts drain, since a fire can start a post, and before the DB close, since a fire reads and writes PostgreSQL). Update the AGENTS.md "Startup and shutdown" bullets in the same commit. Test analog: `TestStopRaidPosts` (raid_post_async_test.go:183), which uses a gated client (`raidGatedClient`, lines 28-53) to hold a post in flight; copy that for an in-flight Gemini job.

---

### Locale keys (7 files) and docs

**Analog keys in `locales/en.yml`:** `lockdown_alert_trigger_flood` (2882), `lockdown_alert_counts_flood` (2902), `lockdown_auto_flood_on` (2941), `lockdown_auto_hint` (2943), `lockdown_auto_usage` (2945), `lockdown_auto_started_flood` (2951), `lockdown_trigger_flood` (2953). Copy their placeholder style `{count}`, `{window}`, `{time}`, `{threshold}` (numbers only, as strings). Example to follow:
```yaml
lockdown_trigger_flood: "Flood: {count} messages in {window} s, {percent}% from new members."
lockdown_alert_trigger_flood: "Flood: {count} messages in {window} s, {percent}% from new members. Locked automatically at {time}."
```
New: `lockdown_trigger_ai_burst`, `lockdown_alert_trigger_ai_burst`, `lockdown_alert_counts_ai`, `lockdown_auto_started_ai_burst`, `lockdown_auto_ai_on`, `lockdown_auto_ai_off`, `lockdown_auto_ai_unavailable`. Changed text (all 7): `lockdown_auto_hint`, `lockdown_auto_usage`, `lockdown_help_msg` (the `/autolockdown [joins|flood|ai]` line and paragraph; add the D-15 data note). Verify with `make check-translations`; copy key names from `en.yml` into Go (a typo ships an empty message). `locales/config.yml` is not a language.

**Docs:** `make generate-docs` regenerates `docs/src/content/docs/commands/lockdown/index.md` from `en.yml`; `make check-docs` fails on drift. Hand edits: `api-reference/environment.md`, `database-schema.md` (`chat_raid_detection` section), `commands.md`, `commands/aispam/index.md`, `sample.env`.

---

### Test harness fakes

- **Telegram bot fake:** `moduleBotClient` (test_harness_test.go:42-152). `RequestWithContext` records every call, returns `c.errors[method]` if set, else `c.responses[method]`, else `true`. For Phase 7 set `client.responses["getFile"] = json.RawMessage(`{"file_id":"x","file_unique_id":"u","file_path":"photos/a.jpg"}`)` (the backup tests do exactly this at backup_test.go:232-234) and assert with `client.callsFor("getFile")`. Note `callsFor` first calls `raidPosts.wait()` (line 157), so a new pool's reads should also wait on the AI trackers or the tests flake.
- **Lockdown env:** `lockdownFake` (lockdown_fake_test.go:36+, `setMemberRaw`, `lockdownAdminJSON`, `setCreator`) and `newLockdownEnv` / `newFloodEnv` (raid_flood_test.go:332-339: Staff Group linked, `raidCleanup`, `env.fake.setCreator(...)`). Use `newFloodEnv` for AI integration tests.
- **Item builders:** `floodMessage(t, from, kind)` (raid_flood_test.go:56-91) builds text/photo/gif/sticker/album messages with `floodTestFile`; add kinds `link` (text with URL and entity), `forward`, `repeat`, and photos with realistic `FileSize`. `e.dispatchMessage`, `e.sendItem`, `e.newMembers(n, age)` and `seedRecentJoin` (24-35) seed the `:joined` set directly.
- **Redis:** `withMiniredis(t)` (testredis_test.go:46) runs Lua; `raidSetClock` (raid_counters_test.go:19) moves `raidNow`. Fixed-clock script test template: `TestRaidFloodScript` (raid_flood_test.go:562-600).
- **Provider fakes:** `httptest` servers for Gemini and TypeSafe via the swap pattern in `withAISpamJevServer` (aispam_jev_test.go:16-32); Telegram file downloads via the backup test's base-URL swap. Make the Gemini client, TypeSafe decide function, download function and the verdict counter swappable package vars (D-16) with hand-written fakes; no mock libraries; no `t.Skip` (the `make test` gate in `scripts/check_test_results/main.go` fails on skips).
- **Must-have tests named by the research:** `TestRaidAINeverActs` (no Telegram write other than `getFile`), `TestRaidAIFailOpen`, a 10-photo album = one provider call, the stale-cache default test, redaction tests (key and file URL never in logs or errors).

---

### `scripts/gemini-spike/main.go` (utility CLI, batch)

**Analog (partial):** `scripts/check_test_results/main.go`: a stdlib-only `package main` inside the main module with a pure `checkResults(input io.Reader, output io.Writer) error` function and a `main_test.go` next to it (`scripts/check_test_results/main_test.go`). Copy that split: pure functions (CSV, stats, cost, bar sweep) unit-tested with fixed inputs; `main()` does flags and I/O. The other tracked tool, `scripts/validate_orphaned_data.go`, imports `github.com/divkix/Alita_Robot/alita/db` (line 10) and so is exactly what the spike must NOT copy. `scripts/check_translations/` has its own `go.mod`; do not do that here, the spike imports `alita/utils/gemini` from the main module.

Constraints that apply because `make test` runs `go test ... ./...` (Makefile:37): the spike package compiles and its tests run in CI, so its tests must be pure and network-free, and it must import only the stdlib and `alita/utils/gemini`. Without `GEMINI_API_KEY` it prints usage and exits non-zero having made no request (D-17).

---

## Shared Patterns

### Best-effort Redis call under 250 ms
**Source:** `alita/modules/raid_flood.go:225-226`, `raid_counters.go:38,57-59`
**Apply to:** every new Redis access (candidate script, verdict script, snapshot, budgets, album gate, verdict cache)
```go
ctx, cancel := context.WithTimeout(cache.Context, raidRedisTimeout)
defer cancel()
```
Keys via `raidKey(chatID, part)` (chat ID is the `{hash tag}`); time via `raidNow()`; no Redis -> `raidRedis()` returns nil and logs once, the AI trigger is inert and `/lockdown` still works.

### Recovered, counted goroutines
**Source:** `raid_trigger.go:127-134` (`raidPostAsync`), `aispam.go:398-408` (worker)
**Apply to:** pool workers, `raidAIFireAsync`, any helper goroutine. Every one starts with `defer error_handling.RecoverFromPanic(name, "Lockdown")` and is counted in a `raidPostTracker`, never a `sync.WaitGroup`.

### Fail open: no verdict on any error
**Source:** `aispam.go:259-282` (`processAISpam` failure path: counter, log with ids only, breaker, return), `aispam_jev.go:172-174`
**Apply to:** text jobs, image jobs, download, budget and breaker refusals. Log only IDs, never message text or image bytes; never log `file.URL(...)`.

### Settings read for a watcher
**Source:** `raid_flood.go:288-297` (`raidSettingsLookup` under `raidSettingsTimeout`; error -> count nothing, `settings == nil || !enabled` -> return)
**Apply to:** the AI watcher and `raidAIRecordVerdict` (read the threshold from the cached settings at report time).

### Exactly-once lockdown is the database
**Source:** `raid_trigger.go:203-308` (`raidFire`: latch, active check, recently-lifted check, snapshot, `startLockdownRow`, outcome latch, `raidPostAsync` notice)
**Apply to:** the AI trigger calls `raidFire(b, chat, raidTrigger{Kind: models.LockdownTriggerAIBurst, Count: items, Window: 120, Percent: 0})` and nothing else; never add a second start path or a Redis exactly-once.

### Caching writes
**Source:** `alita/db/raiddetection/repository.go:90-107` (`upsertSettings` ends with `cache.DeleteCache(cache.CacheKey(cacheModule, row.ChatID))`)
**Apply to:** `SetAIEnabled`, `SetAIThreshold`. Never use `UpdateRecord` for these (skips zero values).

### i18n
**Source:** `lockdownText(tr, key, i18n.TranslationParams{...})` used throughout `raid_trigger.go` and `lockdown_alert.go`; numbers as strings via `strconv`; user text (names, titles, links) spliced in after translation with `lockdownSplice`/tokens.
**Apply to:** every new alert, notice and status string.

### Tests
**Source:** `//go:build testtools` first line (e.g. aispam_jev_test.go:1), real fixtures (`internal/testdb.Run`, miniredis, hand-written fakes), run with `-tags testtools`, no `t.Skip`. Add the new guarantees to `make test-postgres-integrity` only if they rest on a partial unique index, CHECK or conditional update (the new `chk_chat_raid_detection_ai` CHECK belongs in the existing `TestRaidDetectionSettings` case); do NOT start PostgreSQL locally.

## No Analog Found

| File | Role | Data Flow | Reason |
|------|------|-----------|--------|
| `07-SPIKE.md` | docs | - | No existing spike document; use RESEARCH Pattern 10 as the spec. |
| Gemini provider-wide pause (inside `raid_ai_breaker.go`) | service | request-response | `/aispam` has only per-chat breakers; the provider-wide pause (bad key, long 429 `retryDelay`) is new. Use a mutex-guarded `pausedTill time.Time` in the style of `aispamBreaker`. |
| Album gate (`SET NX alita:raid:{<chat>}:ag:<media_group_id> EX 120`) | service | event-driven | Closest is `raidFire`'s latch (`client.SetNX(latchCtx, raidKey(...), "1", raidFireLatch)` at raid_trigger.go:228) and `raidWarnCannotLock`'s gate (435-444); copy the `SetNX` + Redis-error-means-skip shape. |

## Metadata

**Analog search scope:** `alita/modules/` (aispam*, raid_*, lockdown_alert/status, autolockdown, shutdown_drains, devs, backup, test harness and tests), `alita/db/raiddetection`, `alita/db/models`, `alita/config`, `alita/utils/logredact`, `migrations/`, `scripts/`, `locales/en.yml`, `main.go`, `Makefile`
**Files read in full or in targeted ranges:** about 30
**Pattern extraction date:** 2026-10-29
**Constraints honoured:** read-only; nothing started PostgreSQL; this file is the only write.
