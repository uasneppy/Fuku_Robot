# Phase 6: Automatic Raid Detection - Pattern Map

**Mapped:** 2026-10-18
**Files analyzed:** 20 (new or modified)
**Analogs found:** 17 / 20

All analog paths are git-tracked source files in `/home/user/Fuku_Robot` (none are gitignored mirrors).

## File Classification

| New/Modified File | Role | Data Flow | Closest Analog | Match Quality |
|-------------------|------|-----------|----------------|---------------|
| `migrations/20261018120000_add_chat_raid_detection.sql` | migration | batch (DDL + carry-over INSERT...SELECT) | `migrations/20260518113000_add_antiraid_settings.sql` | exact |
| `alita/db/models/raid_detection.go` | model | CRUD | `alita/db/models/antiraid.go` | exact |
| `alita/db/models/lockdown.go` (modify) | model | CRUD | same file, `LockdownTriggerManual` / `TriggerKind` | exact |
| `alita/db/raiddetection/repository.go` | repository | CRUD (cached read, column upsert) | `alita/db/antiraid/optimized.go` + `alita/db/antiraid/repository.go` | exact |
| `alita/db/raiddetection/repository_test.go`, `testmain_test.go` | test | CRUD | `alita/db/antiraid/repository_test.go`, `testmain_test.go` | exact |
| `alita/db/cache/local.go` (modify) | config | transform | same file, `skipLocal` | exact |
| `alita/db/lockdown/cooldown.go` (new) | repository | CRUD (read) | `alita/db/lockdown/repository.go` `GetActiveFresh` | role-match |
| `alita/modules/lockdown_start.go` (new, extracted) | service | request-response | `alita/modules/lockdown.go:287-421` | exact (refactor source) |
| `alita/modules/lockdown.go` (modify) | controller | request-response | itself | exact |
| `alita/modules/raid_counters.go` (new) | service | streaming / event-driven (Redis Lua) | `alita/modules/antiraid.go:71-76`, `alita/modules/lockdown_recent.go:58-89` | exact |
| `alita/modules/raid_trigger.go` (new) | service | event-driven | `alita/modules/lockdown.go` (start path) + `lockdown_worker.go` (wake) | role-match |
| `alita/modules/raid_join.go` (new hook) + `lockdown_guard.go` (modify) | middleware (watcher) | event-driven | `alita/modules/lockdown_guard.go:113-135` | exact |
| `alita/modules/raid_flood.go` (new watcher, group -4) | middleware (watcher) | event-driven | `alita/modules/antiflood.go:242-262`, `616` | exact |
| `alita/modules/autolockdown.go` (new command) | controller | request-response | `alita/modules/lockdown.go:287` + `:597-603` | exact |
| `alita/modules/antiraid.go` (shrink/retire) | module | request-response | itself (keep pointer commands; remove `onJoin` :448 and poller :89-122) | exact |
| `alita/modules/lockdown_alert*.go` (modify, flood burst D-17) | service | batch | `alita/modules/lockdown_alert.go` (`lockdownBurstSnapshot`, ~204-230) | exact |
| `alita/db/lockdown/burst.go` (modify, D-17 accounts) | repository | CRUD | same file | exact |
| `alita/db/lockdown/repository.go` (modify, trigger columns on Start) | repository | CRUD | itself `Start` :57-75 | exact |
| `alita/modules/*_test.go` (new: raid counters, trigger, flood, autolockdown) | test | request-response | `alita/modules/lockdown_guard_test.go`, `lockdown_fake_test.go`, `lockdown_recent_test.go` | exact |
| `alita/modules/testredis_test.go` (new, moved helper) | test | — | `alita/modules/antiraid_miniredis_test.go:22` (`withMiniredis`) | exact |
| `locales/*.yml` (7 files) | config | — | existing `lockdown_*` keys in `locales/en.yml` | exact |
| `AGENTS.md`, handler-groups docs | config | — | existing `-5 antiraid` / `-7 lockdown` lines | exact |

## Pattern Assignments

### `migrations/20261018120000_add_chat_raid_detection.sql` (migration, batch)

**Analog:** `migrations/20260518113000_add_antiraid_settings.sql`

**Table + CHECK pattern** (whole file, lines 1-30):
```sql
CREATE TABLE IF NOT EXISTS antiraid_settings (
    id BIGSERIAL PRIMARY KEY,
    chat_id BIGINT NOT NULL,
    raid_time INT NOT NULL DEFAULT 21600,
    ...
    auto_antiraid_threshold INT NOT NULL DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(chat_id),
    CONSTRAINT chk_auto_antiraid_threshold_nonnegative CHECK (auto_antiraid_threshold >= 0)
);
```
Copy: `UNIQUE(chat_id)`, named `CONSTRAINT chk_...` checks. For D-18 use a range CHECK (`BETWEEN 3 AND 500`) on the new table.

**FK DO block** (lines 17-30): add the FK to `chats(chat_id)` only `IF EXISTS` the `chats` table, dropping any prior constraint first. No top-level `BEGIN`/`COMMIT`.

**Carry-over (D-03/D-18)**: `INSERT ... SELECT ... FROM antiraid_settings WHERE auto_antiraid_threshold > 0 ON CONFLICT (chat_id) DO NOTHING`, with the threshold clamped `GREATEST(auto_antiraid_threshold, 3)`. `trigger_kind` on `chat_lockdowns` needs no CHECK (per D-13). Add the three trigger columns (`trigger_count`, `trigger_window`, `trigger_percent`) with `ADD COLUMN IF NOT EXISTS`.

### `alita/db/models/raid_detection.go` (model, CRUD)

**Analog:** `alita/db/models/antiraid.go:5-17`

```go
type AntiRaidSettings struct {
	ID                    uint      `gorm:"primaryKey;autoIncrement" json:"-"`
	ChatID                int64     `gorm:"column:chat_id;uniqueIndex;not null" json:"chat_id,omitempty"`
	AutoAntiRaidThreshold int       `gorm:"column:auto_antiraid_threshold;default:0" json:"auto_antiraid_threshold,omitempty"`
	CreatedAt             time.Time `gorm:"column:created_at" json:"created_at,omitempty"`
	...
}

func (AntiRaidSettings) TableName() string {
	return "antiraid_settings"
}
```
Differ on purpose: **no `default:` tags on boolean flags** (RESEARCH Pattern 7; a Go `false` with `default:true` is dropped by GORM Create). Defaults live in Go (`defaultRaidDetection`).

### `alita/db/models/lockdown.go` (modify)

**Analog:** same file, lines 17-19 and 79.
```go
// LockdownTriggerManual is the trigger_kind of a lockdown an admin started with
const LockdownTriggerManual = "manual"
...
TriggerKind string `gorm:"column:trigger_kind;size:16;not null;default:'manual'" ...`
```
Add `LockdownTriggerJoinSurge = "join_surge"`, `LockdownTriggerFlood = "flood"`, and three trigger fields in the same style (`gorm:"column:trigger_count"` etc.). Add the same columns to the test-harness AutoMigrate lists (AGENTS.md step 4).

### `alita/db/raiddetection/repository.go` (repository, cached read + column upsert)

**Analog:** `alita/db/antiraid/optimized.go:15-56` and `alita/db/antiraid/repository.go:36-43`

Cached read, defaults on not-found:
```go
func getAntiRaidSettingsRawContext(ctx context.Context, chatID int64) (*models.AntiRaidSettings, error) {
	if db.DB == nil {
		return defaultAntiRaidSettings(chatID), errors.New("database not initialized")
	}
	var settings models.AntiRaidSettings
	err := db.DB.WithContext(ctx).Model(&models.AntiRaidSettings{}).
		Select("id, chat_id, ...").Where("chat_id = ?", chatID).First(&settings).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return defaultAntiRaidSettings(chatID), nil
	}
	...
}

func GetAntiRaidSettingsCachedContext(ctx context.Context, chatID int64) (*models.AntiRaidSettings, error) {
	cacheKey := cache.CacheKey("antiraid", chatID)
	cached, err := cache.GetFromCacheOrLoad(ctx, cacheKey, cache.CacheTTLAntiRaid, func(ctx context.Context) (*models.AntiRaidSettings, error) {
		return getAntiRaidSettingsRawContext(ctx, chatID)
	})
	...
}
```
Write pattern (`repository.go:36-43`): `upsertChatField` updates the row then `cache.DeleteCache(cache.CacheKey("antiraid", chatID))`. For Phase 6 use `cache.CacheKey("raid_detection", chatID)`, a one-column-per-trigger `clause.OnConflict{Columns: chat_id, DoUpdates: AssignmentColumns(...)}` upsert, and `DeleteCache` after commit. Never return nil on a failed write.

### `alita/db/cache/local.go` (modify)

**Analog:** same file, lines 44-63.
```go
captchaPendingPrefix = CacheKey("captcha_pending") + ":"
staffGroupPrefix     = CacheKey("staff_group") + ":"
...
return strings.HasPrefix(key, captchaPendingPrefix) ||
	strings.HasPrefix(key, staffGroupPrefix) ||
```
Add `raidDetectionPrefix = CacheKey("raid_detection") + ":"` and a `strings.HasPrefix` line (D-12). Extend `local_test.go` prefix list with `"raid_detection"` and a negative `"raid_detectionx"`.

### `alita/db/lockdown/cooldown.go` (new, read)

**Analog:** `alita/db/lockdown/repository.go:76` `GetActiveFresh` (fresh, uncached read; `now()` helper at :36). Add `RecentlyLiftedFresh(chatID, since)` as a `SELECT ... WHERE chat_id = ? AND state <> active AND lift_started_at > ?` read, never cached (AGENTS.md: lockdown tables are read only through fresh queries).

### `alita/db/lockdown/repository.go` Start (modify for trigger columns)

**Analog:** same file, lines 57-75. The row is inserted as-is with `clause.OnConflict{DoNothing: true}`; trigger columns just become fields on the struct, so no change to the `ON CONFLICT` logic. Winner is `RowsAffected == 1`.
```go
result := db.DB.Clauses(clause.OnConflict{DoNothing: true}).Create(row)
if result.Error != nil {
	if isUniqueViolation(result.Error) { return false, nil }
	...
}
return result.RowsAffected == 1, nil
```

### `alita/modules/lockdown_start.go` (new; extracted from `lockdown.go`)

**Analog / source:** `alita/modules/lockdown.go:287-421` (the `/lockdown` body). Extract from the bot-rights check (:332-341) through `ConfirmLocked` and `wakeLockdownWorker()` (:393-400). Return an outcome enum; `/lockdown` maps outcomes back to the same replies it sends today.

Concrete excerpts to move (verbatim logic):
```go
botMember, botResult, botErr := chat_status.FetchBotMember(b, chat.Id)
if botResult == chat_status.BotMemberUnknown { ... refuse("lockdown_bot_check_failed") }
if botResult == chat_status.BotMemberMissing ||
	botMember.Status != gotgbot.ChatMemberStatusAdministrator || !botMember.CanRestrictMembers {
	return refuse("lockdown_bot_cannot_restrict")
}
```
```go
row := &models.ChatLockdown{
	ChatID: chat.Id, TriggerKind: models.LockdownTriggerManual, Reason: ..., StartedBy: actor.Id,
	StartedByName: name, ChatTitle: lockdownCapRunes(chat.Title, lockdownTitleMaxRunes),
	PrePermissions: string(chatInfo.Permissions), LockedPermissions: lockdownLockedPermissions,
}
started, err := lockdown.Start(row)
```
Lock refusal branch to copy as-is (:377-391): `if !lockdownDefinitiveRefusal(err) { ... "lockdown_lock_unknown" }`, else `DeleteUnconfirmed(row.ID)` then `"lockdown_lock_failed"`. Stay synchronous (no goroutine), per RESEARCH Pattern 6. `/lockdown` keeps the supergroup reply (:306), `GetActiveFresh` + settle (:310-330) and the success reply (:402-420).

### `alita/modules/lockdown.go` (modify; command + registration)

**Analog:** itself.
- Command body guards: `chat == nil || msg == nil || actor == nil` → `ext.EndGroups` (:291-293).
- `reply := func(text string) error { replyLockdown(b, msg, text); return ext.EndGroups }` (:295-298).
- Authority wrapper: `requireLockdownAuthority(needRestrict bool) helpers.CheckFunc` (:130); `requireLockdownGroup()` (:92); `lockdownCommandArgs(ctx)` (:195).
- Registration (:585-603):
```go
func LoadLockdown(dispatcher *ext.Dispatcher) { ... }
func init() {
	RegisterAnonymousAdminHandler("lockdown", anonPipelineHandler(lockdownDesc, lockdownModule.lockdown))
	RegisterAnonymousAdminHandler("unlockdown", anonPipelineHandler(unlockdownDesc, lockdownModule.unlockdown))
	RegisterAnonymousAdminHandler("lockdownstatus", anonPipelineHandler(lockdownStatusDesc, lockdownModule.lockdownStatus))
}
```
Add `autolockdown` and `autoantiraid` here the same way.

### `alita/modules/raid_counters.go` (new; Redis Lua counters)

**Analog:** `alita/modules/antiraid.go:71-76` (Lua join tracker) and `alita/modules/lockdown_recent.go:58-89` (best-effort pipelined write).

Lua scripts in package scope with `redis.NewScript`:
```go
trackJoinScript = redis.NewScript(`
	redis.call('ZADD', KEYS[1], ARGV[1], ARGV[2])
	redis.call('EXPIRE', KEYS[1], ARGV[3])
	redis.call('ZREMRANGEBYSCORE', KEYS[1], 0, ARGV[4])
	return redis.call('ZCARD', KEYS[1])
`)
```
Best-effort write shape from `trackLockdownRecentJoins`:
```go
if len(joins) == 0 || !cache.IsRedisAvailable() { return }
client := cache.GetRedisClient()
if client == nil { return }
ctx, cancel := context.WithTimeout(cache.Context, lockdownRecentTimeout)  // 250 ms
defer cancel()
```
Key family: `lockdownRecentKeyPrefix = "alita:lockdown:recent:"` (`lockdown_recent.go:21`). Use `alita:raid:{<chat>}:...` (new family, outside `alita:cache:`). Member = user ID, score = join ms (`ZADD` without NX, per RESEARCH Pattern 4). Time passed in from Go (`raidNow` var for tests).

### `alita/modules/lockdown_recent.go` (reference; unchanged)

Keep as-is. Copy constants style (`:21-34`) and the struct with `json:"-"` score (`:38-45`). Do NOT count its `ZCARD` (double counts per delivery).

### `alita/modules/lockdown_guard.go` (modify; join hook)

**Analog:** same file, `lockdownOnJoinMember` :113-135.
```go
func (m moduleStruct) lockdownOnJoinMember(b *gotgbot.Bot, ctx *ext.Context) error {
	defer error_handling.RecoverFromPanic("lockdownOnJoinMember", "Lockdown")
	chat := ctx.EffectiveChat
	update := ctx.ChatMember
	if chat == nil || update == nil || chat.Type != "supergroup" {
		return ext.ContinueGroups
	}
	// Best effort and first: the tracker never decides or delays what the guard does.
	trackLockdownRecentJoins(chat.Id, lockdownMemberJoins(b, update))
	active, err := lockdownActiveLookup(chat.Id)
	...
```
Insert the raid join hook right after `trackLockdownRecentJoins` (before `lockdownActiveLookup`). Count a join only when `decideLockdownJoin(input) == lockdownJoinBan` (`lockdown_guard.go:60-79`), then drop approved users. Same for `lockdownOnJoinMessage` (:184). Do not touch `lockdownOnJoinRequest` (:294). Extract the input builder so the guard and counter share it.

### `alita/modules/raid_flood.go` (new watcher, group -4)

**Analog:** `alita/modules/antiflood.go:242-262` (watcher head) and `:607-616` (registration).
```go
func (m *moduleStruct) checkFlood(b *gotgbot.Bot, ctx *ext.Context) error {
	...
	return ext.ContinueGroups
}
// registration
dispatcher.AddHandlerToGroup(handlers.NewMessage(message.All, antifloodModule.checkFlood), antifloodModule.handlerGroup)
```
Use `handlers.NewMessage(message.All, ...)` at `raidFloodGroup = -4`, registered inside `LoadLockdown` (one module). Exempt via `antifloodModule.userIsFloodExempt` (`antiflood.go:122-127`) and `IsApprovedForUpdate`. Antiflood skips albums (`MediaGroupId != ""`), so count the album as `g:<media_group_id>`. Every path returns `ext.ContinueGroups`. Add `defer error_handling.RecoverFromPanic(...)` as the first line.

Sender-shape guard (Pitfall 2): `ctx.EffectiveSender == nil`, `ctx.EffectiveSender.Chat != nil`, `msg.IsAutomaticForward`, `staffServiceUserIDs[userID]` (`staff_action_decide.go:513`), `userID == b.Id`.

### `alita/modules/antiflood.go` (reference; read only)

Exemption helpers to reuse:
```go
func cachedAdminStatus(chatId, userId int64) (known bool, isAdmin bool) { ... }   // :105-120
func (a *antifloodStruct) userIsFloodExempt(b *gotgbot.Bot, chatId, userId int64) bool {  // :122-127
	if known, isAdmin := cachedAdminStatus(chatId, userId); known {
		return isAdmin
	}
	return a.adminCheckWithTimeout(b, chatId, userId)
}
```

### `alita/modules/raid_trigger.go` (new; `raidFire`)

**Analog:** `alita/modules/lockdown.go` start path (via `startLockdownRow`) and `wakeLockdownWorker()` (:400). Flow (RESEARCH Pattern 9): `SET NX EX 60` latch → `RecentlyLiftedFresh(chat, now-10min)` → `startLockdownRow` → on started: `lockdownPaced` public notice + `wakeLockdownWorker()`; on can't-lock: warn once/day via `SET NX EX 86400` and `staff.GetLinkOfGroupFresh` + `actionlog.Destination(chat, logchannels.CategoryAdmin)` (as `lockdownAlertDestinations` does). Run synchronously in the handler, not in a goroutine (no drain needed).

### `alita/modules/autolockdown.go` (new command)

**Analog:** `alita/modules/lockdown.go:287` (command shape) and `:597-603` (registration). Authority by argument count:
```go
func requireAutoLockdownAuthority() helpers.CheckFunc {
	view, change := requireLockdownAuthority(false), requireLockdownAuthority(true)
	return func(c *helpers.CommandContext) bool {
		if len(lockdownCommandArgs(c.Ctx)) == 0 { return view(c) }
		return change(c)
	}
}
```
Descriptor and registration as `lockdownDesc`; `RegisterAnonymousAdminHandler("autolockdown", anonPipelineHandler(...))`. Also register `autoantiraid` as an alias (D-16). Replies in the old module's style; validation range 3..500 for joins and flood thresholds (D-18), `lockdownReplyLockdown` for output.

### `alita/modules/antiraid.go` (retire / shrink)

**Analog / source:** itself. Remove:
- `onJoin` (`:448`) and its registration in `LoadAntiRaid` (`:943-950`, `handlerGroup -5`).
- Expiry poller `StartAntiRaidExpiryPoller`/`StopAntiRaidExpiryPoller` (`:89-122`) and its `main.go:196-200` shutdown line.
- Lua scripts `deleteRaidStateScript`, `replaceRaidStateScript`, `disableRaidScript` (`:52-70`) and the state helpers `:124-257`.

Keep and repoint:
- `autoAntiRaid` (`:740`) becomes the alias; writes the join-surge threshold via `raiddetection`.
- `antiraid`, `raidTime`, `raidActionTime` (`:529`, `:652`, `:656`) reply the pointer text.

Stale-key sweep (once, Redis present): `SCAN MATCH alita:antiraid:* COUNT 100` + `UNLINK`, inside a goroutine that starts `defer error_handling.RecoverFromPanic(...)`.

Keep `withMiniredis` before deleting `antiraid_miniredis_test.go` (move to `alita/modules/testredis_test.go`, build tag `testtools`).

### `alita/modules/lockdown_alert.go` / `alita/db/lockdown/burst.go` (modify, D-17)

**Analog:** `lockdown_alert.go` `lockdownBurstSnapshot` (~204-230) saves only tracker joins as `pre` rows. For `TriggerKind == flood`, also read the `fa` offender set (`alita:raid:{chat}:fa`) and insert them via the same `BeginAlert` transaction with `ON CONFLICT DO NOTHING` (`burst.go`). Keep the same `chat_lockdown_burst` row shape.

### Tests

**Analogs:**
- `alita/modules/lockdown_guard_test.go` and `lockdown_fake_test.go`: hand-written `gotgbot.BotClient` fakes, no mock libraries.
- `alita/modules/lockdown_recent_test.go`: Redis via `withMiniredis` (`antiraid_miniredis_test.go:22`, to be moved).
- `alita/db/antiraid/repository_test.go:15` `skipIfNoDb` + `testmain_test.go`: real SQLite fixtures via `internal/testdb.Run`.

Add to `alita/db/testmain_test.go`, `alita/modules/test_harness_test.go` and `alita/db/lockdown/testmain_test.go`: `ChatRaidDetection` and the new `ChatLockdown` columns in AutoMigrate lists (AGENTS.md "Adding a module" step 4). Postgres-only guarantees (one active lockdown across two triggers, one alert) go in the `make test-postgres-integrity` `-run` list.

### Locales

**Analog:** existing `lockdown_*` keys in `locales/en.yml` (e.g. `lockdown_started`, `lockdown_basic_group`). Add trigger, notice, warn and `/autolockdown` keys to all 7 files (`en es fr hi id pt ru`) and check with `make check-translations`.

## Shared Patterns

### Handler groups and return values
**Source:** `alita/modules/lockdown_guard.go:113-135` and `alita/modules/antiflood.go:242-262`
**Apply to:** `raid_flood.go`, `raid_join.go` hook, `lockdown_guard.go`
Watchers return `ext.ContinueGroups`; commands return `ext.EndGroups`. A group `0` watcher returning `nil` kills later group-0 handlers. Every new goroutine starts with `defer error_handling.RecoverFromPanic(...)`.

### Best-effort Redis on the hot path
**Source:** `alita/modules/lockdown_recent.go:53-89`
**Apply to:** `raid_counters.go`, the join hook
`cache.IsRedisAvailable()` check first, `context.WithTimeout(cache.Context, 250ms)`, errors logged at debug and ignored. Never a ban or start decision on Redis alone.

### Live authority
**Source:** `alita/modules/lockdown.go:92` (`requireLockdownGroup`), `:130` (`requireLockdownAuthority`), `:107` (`lockdownLiveMember`)
**Apply to:** `autolockdown.go` (change path). Trigger path uses no authority check (counts only).

### Cached settings reads and writes
**Source:** `alita/db/antiraid/optimized.go:41-56`, `alita/db/antiraid/repository.go:36-43`
**Apply to:** `alita/db/raiddetection/`. Read via `cache.GetFromCacheOrLoad`; every write calls `cache.DeleteCache(cache.CacheKey("raid_detection", chatID))`; `skipLocal` for the prefix (`alita/db/cache/local.go:44-63`).

### Exactly-once lockdown
**Source:** `alita/db/lockdown/repository.go:57-75` (`Start`), `alita/modules/lockdown.go:365-375`
**Apply to:** `raid_trigger.go`. Latch is only load shedding; `Start` returns `false` for the loser and the trigger does nothing.

## No Analog Found

| File | Role | Data Flow | Reason |
|------|------|-----------|--------|
| `alita/modules/raid_flood.go` item classification (`raidFloodItemKey`) | utility | transform | No existing message-count classifier; build from gotgbot `message` fields and `antiflood.go` checks. Follow RESEARCH Pitfall 4. |
| Flood ratio Lua script (`items`, `newItems`, `newAccounts`) | service | streaming | No existing 3-set ratio script; use RESEARCH Pattern 5 (verified against miniredis). |

## Metadata

**Analog search scope:** `alita/modules/`, `alita/db/`, `alita/db/lockdown/`, `alita/db/antiraid/`, `alita/db/models/`, `migrations/`, `locales/`
**Files scanned:** 14 read or grepped directly; 758-line RESEARCH.md read through line 467 (remaining sections not reviewed for analogs)
**Pattern extraction date:** 2026-10-18
