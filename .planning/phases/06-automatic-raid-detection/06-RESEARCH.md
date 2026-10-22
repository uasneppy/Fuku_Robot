# Phase 6: Automatic Raid Detection - Research

**Researched:** 2026-10-18
**Domain:** Go Telegram bot, Redis sliding-window counters (Lua), shared lockdown start path, retiring a legacy module
**Confidence:** HIGH for everything that is in-repo code (opened this session); MEDIUM for Telegram Bot API behaviour (core.telegram.org was blocked by the session's egress policy, so Bot API facts rest on gotgbot's generated field docs and on behaviour the repo already encodes)

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

#### Join-surge trigger (RAID-01)
- **D-01:** The default threshold is 15 joins within 60 seconds. This is conservative, and uses the same per-minute unit as the old `/autoantiraid N`.
- **D-02:** Only real joins count, each user once even when Telegram delivers the same join as both a `chat_member` update and a service message.
  - Users an admin added or approved never count (RAID-07, and the same performer rule as the Phase 4 guard). Neither do approved users or the bot itself.
  - Join requests do not count, because they don't let anyone in.
- **D-03:** Carry-over (LOCK-15): a group's non-zero `antiraid_settings.auto_antiraid_threshold` becomes its join-surge threshold. A group where it was 0 (off) gets the new default, on at 15, because the owner chose triggers on by default.
- **D-04:** Counting is shared across replicas through Redis, read from the existing 10-minute join tracker (`alita:lockdown:recent:<chat>`, Phase 5) or a key beside it.
  - Without Redis the trigger is inert and logs once; manual `/lockdown` still works.
  - A count is never a ban decision. It only starts a lockdown.

#### Message and media flood trigger (RAID-02)
- **D-05:** "Recently joined" means joined this group in the last 24 hours. Each group's joiners and their join times are kept in Redis for 24 hours. The group -7 join handlers write it best effort, like the 10-minute tracker.
- **D-06:** The default threshold is 30 counted items (messages, photos, GIFs or stickers) within 30 seconds. At least 60% of them must come from recently joined members, from at least 3 different recently joined accounts. A busy conversation among established members never trips it.
- **D-07:** These never count (RAID-07): admins, approved users, the bot itself, Telegram service accounts, and posts sent as the linked channel or as the group (anonymous admins). Commands and edited messages don't count either. The cached admin list is acceptable here, because counting grants no authority and a live `getChatMember` per message is too costly.
- **D-08:** The trigger does not delete the flood. The lockdown mutes everyone, which stops it.

#### Settings and admin control (RAID-06)
- **D-09:** One new command, `/autolockdown`.
  - With no arguments it shows both triggers, whether each is on, and its threshold.
  - `/autolockdown joins <N|off|on>` and `/autolockdown flood <N|off|on>` change them.
  - The Phase 9 settings menu adds buttons for the same settings later.
- **D-10:** Changing a setting needs the live group owner, or an admin who can restrict members, checked live: the same `requireLockdownAuthority` as `/lockdown`.
  - Anonymous admins confirm with the proof button (`RegisterAnonymousAdminHandler`).
  - Any admin may view the settings.
- **D-11:** Settings live in a new table, `chat_raid_detection`, with explicit on/off flags and thresholds per trigger.
  - A missing row means the defaults.
  - The append-only migration copies the old non-zero `auto_antiraid_threshold` into it (D-03).
  - Reads go through `cache.GetFromCacheOrLoad`, and writes `DeleteCache` the key.
- **D-12:** A change takes effect on the next message or join, on every replica. The settings key is listed in `skipLocal`, so the 10-second in-process cache never serves a stale setting.

#### Automatic lockdown behaviour and retiring /antiraid (LOCK-10, LOCK-15)
- **D-13:** An automatic lockdown is exactly a `/lockdown`: the same snapshot, lock, join guard, worker, alert, buttons, reminders and lift.
  - It is recorded with the bot as starter and with `trigger_kind` `join_surge` or `flood`. The column has no CHECK, so no migration is needed for the value.
  - The reason names the trigger and its count, and the Phase 5 alert shows them.
  - A short public notice in the locked group says it was locked automatically and that an admin can `/unlockdown`.
- **D-14:** When the bot can't lock (not an admin with the restrict right, or not a supergroup), no lockdown is started. A warning ("a raid was detected but I can't lock this group") goes to the group's Staff Group or log channel at most once per day per group, deduplicated across replicas.
- **D-15:** Exactly one lockdown and one alert come from the existing `uk_chat_lockdowns_active` partial unique index plus `lockdown.Start`'s `ON CONFLICT DO NOTHING`, and from Phase 5's conditional `BeginAlert`.
  - A trigger that fires while a lockdown is active, or loses the race, does nothing.
  - After an admin lifts a lockdown, automatic triggers stay quiet in that group for 10 minutes, so leftover activity does not lock it again at once.
- **D-16:** Retiring `/antiraid`:
  - `/antiraid`, `/raidtime` and `/raidactiontime` reply with a pointer to `/lockdown` and `/autolockdown`.
  - `/autoantiraid N|off` keeps working as an alias that sets the join-surge threshold or turns it off.
  - The old raid-mode join handler (`onJoin`, group -5, which temp-bans) and its Redis expiry poller are removed, so nobody is temp-banned any more.
  - A raid mode still active in Redis at deploy is ignored and its keys cleared.
  - The `antiraid_settings` table stays (migrations are append-only); its timing columns are no longer read.

### Claude's Discretion
- Handler group placement of the flood counter. It must see messages before antiflood (group 4) ends them, and return `ext.ContinueGroups`.
- The exact Redis key layout and atomic counting (a Lua script or a pipelined ZADD/ZCOUNT) of both counters.
- How the trigger's lock call shares code with `/lockdown`: one shared start-lockdown function used by both.
- The wording of the public notice, the warning and the `/autolockdown` replies, in all 7 locales.
- Whether the old Redis raid keys are cleared by a startup sweep or lazily.

### Deferred Ideas (OUT OF SCOPE)
- A persistent PostgreSQL join log so the triggers work without Redis. Declined in Phase 5 for its per-join cost and declined again here (D-04).
- Deleting the flood messages when the flood trigger fires (D-08).
- Per-trigger windows that admins can configure, not just thresholds. The thresholds are tunable and the windows stay fixed for v1.

(Also out of scope per the Phase Boundary: AI spam verdict counters and Gemini image classification (RAID-03/04/05 and RAID-06's AI toggle, Phase 7), and the settings menu (Phase 9).)
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| RAID-01 | A burst of joins within a short window automatically triggers a lockdown. | Join counter (Redis ZSET `joined`, Lua), hook in both group -7 join handlers, shared `startLockdownRow`, `trigger_kind=join_surge` |
| RAID-02 | A burst of messages, photos, GIFs or stickers, mostly from recently joined members, automatically triggers a lockdown. | Flood watcher at group -4, Lua flood script (items / new-member items / new-member accounts), 24h joined set, ratio + min-accounts rule, `trigger_kind=flood` |
| RAID-06 | Automatic triggers on by default in every group with conservative thresholds; admin can turn each off or tune it. | `chat_raid_detection` table + repository + `skipLocal`, `/autolockdown`, live `requireLockdownAuthority` |
| RAID-07 | Admins and approved users never count toward detection. | Join side: `decideLockdownJoin` performer rule + approvals; message side: `userIsFloodExempt` + `IsApprovedForUpdate` + sender-shape exclusions |
| LOCK-10 | At most one active lockdown per group; several triggers firing at once create a single lockdown and a single alert. | `uk_chat_lockdowns_active` + `lockdown.Start` ON CONFLICT DO NOTHING + conditional `BeginAlert` (already exist), fire latch to cap Telegram/DB work, tests for two triggers and two "replicas" |
| LOCK-15 | Old `/antiraid` retired: commands point to `/lockdown`, auto-threshold carries over to join-surge detection, temp-ban timings dropped. | Data migration (INSERT...SELECT), pointer commands, `/autoantiraid` alias, removal of `onJoin` (-5) + poller + stale-key sweep |
</phase_requirements>

## Project Constraints (from CLAUDE.md / AGENTS.md)

Treated as locked, same authority as CONTEXT.md decisions.

- Go 1.26.0, gotgbot v2, GORM (PostgreSQL; SQLite in tests), Redis. No new dependencies are needed (see Package Legitimacy Audit).
- Migrations: append-only, timestamp greater than every existing file (latest is `20261011160000_add_lockdown_ban_run_report.sql`), one transaction per file, no top-level `BEGIN`/`COMMIT`, no `CREATE INDEX CONCURRENTLY`. Never call `gorm.AutoMigrate` in production code. If the applier changes, change `alita/db/migrations/runner.go` and `scripts/migrate_psql.sh` together (this phase adds a file only, so neither changes).
- Every write `cache.DeleteCache`s the keys it affects. Reads via `cache.GetFromCacheOrLoad` with `cache.CacheKey`. Two packages are named `cache`: loader and generation guards are in `alita/db/cache`, not `alita/utils/cache`. Freshness-critical keys go in `skipLocal` (`alita/db/cache/local.go`).
- Commands via `helpers.WrapCommand` (+ `RegisterAnonymousAdminHandler` for admin commands). Watchers return `ext.ContinueGroups`; a group-0 watcher returning `nil` kills later group-0 handlers. Value receivers on `moduleStruct`. Do not add replies for failed checks; the pipeline sends them.
- Every fire-and-forget goroutine starts with `defer error_handling.RecoverFromPanic(...)`. Never discard a DB error on a state-changing path.
- Locale keys in all 7 locale files (`en es fr hi id pt ru`); a typo'd key ships an empty message. Copy names from `locales/en.yml`. `make check-translations` only sees literal `GetString` keys.
- Operational Redis keys sit outside `alita:cache:`; a new Redis key family or handler group updates `AGENTS.md` in the same commit.
- Tests: `-tags testtools`, real fixtures (`internal/testdb.Run`, miniredis, hand-written `gotgbot.BotClient` fakes), no mock libraries, assert observable behaviour. `make test` is the only valid full run (not to be run by this research). Postgres-only guarantees go in the `make test-postgres-integrity` `-run` list.
- Telegram limits: respect rate limits and report partial failures; callback data capped at 64 bytes through `callbackcodec` (this phase adds no buttons).
- Security: the per-group admin check is never skipped; new secrets would be registered with `logredact.RegisterSecret` (this phase adds none).
- Conventional Commits; `docs:`/`test:`/`chore:` are dropped from the changelog, so user-visible work is `feat:` or `fix:`.

## Summary

Phase 6 is mostly wiring, because Phases 4 and 5 already built everything that happens after a lock. The new work is four things: (1) a pair of Redis sliding-window counters that live beside the Phase 5 join tracker, (2) a flood watcher at a new handler group, (3) a shared start-lockdown function extracted from `/lockdown` so a trigger can run the same snapshot/lock/confirm sequence with no command context, and (4) retiring the 962-line `antiraid.go` down to pointer commands, an alias and a stale-key sweep.

The decisive design facts, all verified in the code this session: the existing tracker `alita:lockdown:recent:<chat>` stores JSON members that differ per delivery (name, link), so its `ZCARD` double counts and it cannot be the surge counter; a new user-ID-keyed ZSET gives dedupe of the two join deliveries for free and doubles as the 24-hour "recently joined" set. gotgbot's default `handlers.NewMessage` already excludes edited messages, channel posts and `IsBot()` senders, but `Sender.IsBot()` is false whenever `sender_chat` is set, so anonymous-admin and linked-channel posts must be excluded explicitly. Group `-4` is unused, runs after the join guard (-7) and fed-ban (-6) and before aispam (3), antiflood (4) and locks (5/6), so it is the right group. Exactly-once lockdown and alert already come from `uk_chat_lockdowns_active` + `lockdown.Start` + `BeginAlert`; the trigger layer only needs a short Redis latch so a sustained flood does not hammer Telegram and the database on every message.

Three things the CONTEXT does not spell out and the planner must decide or confirm (see Open Questions): the alert must render the trigger line localized per destination, so the count/window/percent should be stored as columns on `chat_lockdowns` rather than baked into the `reason` string; a flood lockdown's "Ban N recent joiners" burst would be empty (flooders joined hours ago, outside the 10-minute tracker) unless the flooding accounts are saved for the alert; and `withMiniredis`, which a dozen test files use, lives in `antiraid_miniredis_test.go`, so deleting the antiraid tests must first move that helper.

**Primary recommendation:** Extract `startLockdownRow` from `lockdown.go` first (behaviour-preserving, existing `/lockdown` tests as the safety net), then build the counters and triggers on top of it; add a `chat_raid_detection` table plus three trigger columns on `chat_lockdowns` in one migration; put the flood watcher at group `-4`; retire antiraid last.

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Join counting + 24h recent-joiner set | Redis (shared counters, Lua) | Bot handler (group -7 hook) | Counters must be shared across replicas (PLAT-01); handlers only feed them |
| Message/media flood counting | Redis (Lua) | Bot watcher (group -4) | Same reason; the ratio must be computed atomically over three sets |
| Trigger decision (threshold, ratio, accounts) | Bot handler (Go) | Redis (returns counts) | Settings are PostgreSQL-backed; the script returns counts, Go compares with the current threshold |
| Exactly-one lockdown / one alert | PostgreSQL (partial unique index, conditional updates) | Redis fire latch (load shedding only) | Authority state is DB-only (Phase 4 rule); the latch is never a correctness mechanism |
| Per-group settings | PostgreSQL `chat_raid_detection` | Redis cache via `GetFromCacheOrLoad` (`skipLocal`) | D-11/D-12 |
| Lock call, snapshot, confirm | Bot (shared `startLockdownRow`) | Lockdown worker (settle, alert, reminders, lift) | Same sequence as `/lockdown`; worker already resumes any crash from the DB |
| Alert / status trigger line | Lockdown worker + locale files | `chat_lockdowns` trigger columns | Rendered per destination language, so the data must be structured |
| "Can't lock" warning dedupe | Redis `SET NX EX 86400` | Staff Group / log channel posts | Best-effort once-a-day; Redis loss can only repeat a warning |
| Post-lift cooldown | PostgreSQL (`chat_lockdowns.lift_started_at`) | — | Survives Redis loss and restarts; no new lift-path code |
| Retired raid-mode key cleanup | Bot startup sweep (Redis SCAN/UNLINK) | — | One bounded pass; nothing reads those keys any more |

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `github.com/redis/go-redis/v9` | v9.22.0 (go.mod) | `redis.NewScript` Lua counters, `SetNX` latches | Already the Redis client; `redis.NewScript` already used by `antiraid.go`, `aispam.go`, `staff_action_card.go`, `ratelimit/telegram_pacer.go` |
| `gorm.io/gorm` | v1.31.2 (go.mod) | `chat_raid_detection` model + column-targeted upsert | Existing ORM |
| `github.com/PaulSonOfLars/gotgbot/v2` | v2.0.0-rc.36 (go.mod; module cache dir `...rc.36.0.20260919140833-240296efadb4`) | handlers, `ext.Context`, `Sender` | Existing |
| `github.com/alicebob/miniredis/v2` | v2.39.0 (go.mod) | Redis for tests (supports the Lua used here) | Existing; verified below |

No new packages. [VERIFIED: go.mod]

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `alita/utils/callbackcodec` | in repo | callback encode/decode | Not needed this phase (no buttons; Phase 9 adds them) |
| `alita/utils/error_handling` | in repo | `RecoverFromPanic` | Top of every new handler |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| One Lua script per counter | Pipelined `ZADD`+`ZREMRANGEBYSCORE`+`ZCARD` | A pipeline is not atomic across replicas; the flood ratio needs three consistent counts and the 24h `ZSCORE` branch. Lua is already the repo's pattern for multi-step Redis state. |
| New user-ID-keyed ZSET for joins | Reuse `alita:lockdown:recent:<chat>` | Its members are JSON that differ per delivery (name, link), so `ZCARD` counts one join twice. Dedupe is done at read time in `readLockdownRecentJoins`, not by Redis. Keep it as is for the alert snapshot. |
| Redis cooldown key after a lift | DB read of `lift_started_at` | A Redis key is lost on flush and needs lift-path code; the DB row already records the lift. |

**Installation:** none.

**Version verification:** `go.mod` versions above were taken from the project instructions and `go.mod` (`miniredis v2.39.0` confirmed by `grep -n miniredis go.mod`). Lua behaviour was verified by running the scripts below against the repo's own miniredis (see Code Examples).

## Package Legitimacy Audit

No external packages are installed or added by this phase. Every dependency named above is already in `go.mod`. Audit table not applicable.

**Packages removed due to [SLOP] verdict:** none
**Packages flagged as suspicious [SUS]:** none

## Architecture Patterns

### System Architecture Diagram

```
 Telegram update
      |
      v
 dispatcher (groups ascending; ContinueGroups = keep scanning this group, then next group)
      |
      +--[-10 captcha sweeper]---------------------------------------------+
      |                                                                    |
      +--[-7 lockdown guard]  chat_member join / new_chat_members msg      |
      |      1. trackLockdownRecentJoins  (10-min tracker, unchanged)      |
      |      2. NEW raidOnJoins: decide who counts (decideLockdownJoin     |
      |         performer rule + approvals) -> Lua JOIN script             |
      |            -> returns distinct joins in last 60 s                  |
      |         count >= join_threshold && join_enabled  --------+         |
      |      3. active-lockdown lookup / record joiner (unchanged)|        |
      |                                                            |        |
      +--[-6 fed-ban] [-5 (antiraid onJoin REMOVED)]               |        |
      |                                                            |        |
      +--[-4 NEW raidFloodWatcher]  message (not edited/bot/cmd)   |        |
      |      filters: sender shape, service ids, supergroup,       |        |
      |               Redis up, flood_enabled (settings GET)       |        |
      |      exempt: cached admin, approved                        |        |
      |      Lua FLOOD script -> {items, newItems, newAccounts}    |        |
      |      items>=T && new*100>=60*items && accounts>=3 ---------+        |
      |                                                            v        |
      |                                          raidFire(kind, count, ...) |
      |                         1 latch SET NX EX 60 (alita:raid:{chat}:fire)
      |                         2 cooldown: lift_started_at within 10 min?  |
      |                         3 startLockdownRow  (shared with /lockdown) |
      |                              live bot rights -> getChat -> Start    |
      |                              (INSERT, partial unique index)         |
      |                              -> setChatPermissions -> ConfirmLocked |
      |                         4a started: public notice in the group,     |
      |                            wake worker                              |
      |                         4b already active / lost race: silent       |
      |                         4c bot cannot lock: warn Staff/log once/day |
      |                            (SET NX EX 86400)                        |
      +--[3 aispam][4 antiflood][5/6 locks][7..11 ...]  (unchanged)         |
                                                                            |
 Lockdown worker (every replica, DB driven, unchanged) <--------------------+
   BeginAlert (conditional) -> alert copies -> Staff Group + log channel
   alert trigger line now localized from trigger_kind + trigger_count/_window/_percent
```

### Recommended Project Structure
```
migrations/
└── 20261018120000_add_chat_raid_detection.sql   # table, trigger columns, index, carry-over INSERT
alita/db/models/
├── raid_detection.go            # ChatRaidDetection + TableName "chat_raid_detection"
└── lockdown.go                  # + LockdownTriggerJoinSurge/Flood consts, 3 trigger columns on ChatLockdown
alita/db/raiddetection/          # NEW domain package (settings repository)
├── repository.go                # Get (cached, defaults on not-found), SetJoin, SetFlood (column-targeted upsert)
├── repository_test.go
└── testmain_test.go
alita/db/lockdown/
└── cooldown.go                  # RecentlyLiftedFresh(chatID, since) (+ test)
alita/db/cache/local.go          # skipLocal += raid_detection prefix
alita/modules/
├── lockdown_start.go            # NEW startLockdownRow (extracted from lockdown.go)
├── raid_counters.go             # Lua scripts, key builders, recordJoins / recordFloodItem
├── raid_trigger.go              # raidFire: latch, cooldown, start, notice, warn
├── raid_join.go                 # hook called from lockdown_guard.go (+ input builders)
├── raid_flood.go                # group -4 watcher + item classification
├── autolockdown.go              # /autolockdown + /autoantiraid alias
└── antiraid.go                  # SHRUNK: pointer commands, stale button answer, key sweep
```

### Pattern 1: Handler-group semantics (what ContinueGroups means)
**What:** `ContinueGroups` keeps scanning the *same* group; any matching handler that returns `nil` ends that group and moves to the next; `EndGroups` stops everything. [VERIFIED: gotgbot `ext/dispatcher.go:280-330`]
```go
// iterateOverHandlerGroups (verbatim, trimmed)
if errors.Is(err, ContinueGroups) {
	// Continue handling current group.
	continue
} else if errors.Is(err, EndGroups) {
	// Stop all group handling.
	return nil
}
...
// Handler matched this update, move to next group by default.
break
```
**When to use:** the flood watcher at `-4` returns `ext.ContinueGroups` on every path, including after it fired a lockdown.

### Pattern 2: Handler-group map and where `-4` sits
`AGENTS.md` Handlers: "`-10` captcha sweeper · `-7` lockdown join guard · `-6` fed-ban · `-5` antiraid · `-3` staff watchers ... `-2` admin-cache · `-1` users tracker · `0` commands/help/greetings · `3` aispam · `4` antiflood · `5`/`6` locks · `7` blacklists ...". A grep of `handlerGroup:`/`AddHandlerToGroup` across `alita/modules` found no `-4` (fed is `fedHandlerGroup = -6`, staff watchers `handlerGroup: -3`, antiraid `handlerGroup: -5`). [VERIFIED: grep + AGENTS.md]

**Recommendation: group `-4`.** It runs after the join guard, fed-ban and (now removed) antiraid, and before aispam (3), antiflood (4), locks (5/6) and blacklists (7), so a message that those later end (delete, mute, ban) was already counted. Antiflood itself returns `ext.ContinueGroups` on most paths and skips `MediaGroupId != ""` messages entirely (so antiflood does not see albums; the new counter must). [VERIFIED: alita/modules/antiflood.go:242-262]. Register it inside `LoadLockdown` (one module, one help page), with `dispatcher.AddHandlerToGroup(handlers.NewMessage(message.All, ...), raidFloodGroup)` where `raidFloodGroup = -4`. Update `AGENTS.md` (remove `-5 antiraid`, add `-4 raid flood counter`) and `docs/src/content/docs/architecture/handler-groups.md`.

### Pattern 3: Join-side counting (reuse the Phase 4 performer rule)
Both group `-7` join handlers already call `trackLockdownRecentJoins(...)` first, "best effort and first". Add one more best-effort call right after it, before the active-lockdown lookup, so the join that tips the threshold is judged by the lockdown it just started:

- chat_member path (`lockdownOnJoinMember`): build the same `lockdownJoinInput` the guard builds (`Path: models.JoinPathMember`, `PerformerID: update.From.Id`, `MemberID`, `MemberIsBot`, `BotID: b.Id`, `PerformerAnonymousAdmin: update.From.Id == lockdownGroupAnonymousBot`). If `lockdownNeedsPerformerLookup(input)`, one live `lockdownLiveMember` for the performer; a failed lookup leaves status empty, which `decideLockdownJoin` never treats as exempt. **A user counts toward the surge iff `decideLockdownJoin(input) == lockdownJoinBan`** (that verdict is "everything but a user a live creator/administrator or anonymous admin added", and a self-join, bot-performed join, bot account or unknown performer all land there). `lockdownJoinExempt` and `lockdownJoinIgnore` (the bot itself) do not count. Then drop approved users (`approvals.IsUserApproved`, cached, cheap).
- service message path (`lockdownOnJoinMessage`): same, per member, with `PerformerID = msg.From.Id` and `anonymous := (msg.SenderChat != nil && msg.SenderChat.Id == chat.Id) || performerID == lockdownGroupAnonymousBot`. One performer lookup shared by the whole message.

Extract the input builders from the two handlers into small helpers so the guard and the counter cannot drift. A ViaJoinRequest chat_member update approved by an admin has that admin as `From` and is therefore exempt (admin approved); approved by the bot it counts. Join requests themselves (`chat_join_request`) are never fed to the counter (D-02): do not touch `lockdownOnJoinRequest`.

`decideLockdownJoin` (verbatim from `lockdown_guard.go:60-79`):
```go
switch {
case in.MemberID == in.BotID:
	return lockdownJoinIgnore
case in.Path == models.JoinPathRequest:
	return lockdownJoinDecline
case in.MemberIsBot:
	return lockdownJoinBan
case in.PerformerID == in.MemberID:
	return lockdownJoinBan
case in.PerformerID == in.BotID:
	return lockdownJoinBan
case in.PerformerAnonymousAdmin:
	return lockdownJoinExempt
case in.PerformerStatus == gotgbot.ChatMemberStatusCreator ||
	in.PerformerStatus == gotgbot.ChatMemberStatusAdministrator:
	return lockdownJoinExempt
}
return lockdownJoinBan
```
The users who count are recorded in the Redis `joined` set (so "recently joined" for the flood ratio means "joined and was not admin-added/approved"). Admin-added users are deliberately not "new members" for the ratio: this only lowers the ratio, i.e. fewer false positives.

### Pattern 4: Redis key layout (new family `alita:raid:*`)
Operational keys, outside `alita:cache:`, so `CLEAR_CACHE_ON_STARTUP` does not clear them. Use a Redis hash tag on the chat ID so the multi-key flood script is cluster-safe (a single node ignores it). Chat IDs are numeric (negative for groups), so a key can never be injected. [ASSUMED: hash tags are harmless here; the repo has no cluster use today.]

| Key | Type | Member -> score | Window / cap / TTL |
|-----|------|-----------------|--------------------|
| `alita:raid:{<chat>}:joined` | ZSET | user ID -> join time (unix ms, latest join) | 24 h window, cap 5000 (trim oldest), PEXPIRE 25 h |
| `alita:raid:{<chat>}:fi` | ZSET | item key (`m:<message_id>` or `g:<media_group_id>`) -> ms | 30 s window, cap 2000, PEXPIRE 2 min |
| `alita:raid:{<chat>}:fn` | ZSET | same item key, only items from recent joiners -> ms | same |
| `alita:raid:{<chat>}:fa` | ZSET | user ID of a recent joiner who posted -> last post ms | same |
| `alita:raid:{<chat>}:fire` | string | latch value `1` | `SET NX EX 60` |
| `alita:raid:{<chat>}:warn` | string | warning dedupe | `SET NX EX 86400` |

Existing precedent for the family and TTL style: `lockdownRecentKeyPrefix = "alita:lockdown:recent:"`, window 10 min, cap 500, TTL 700 s, timeout 250 ms. [VERIFIED: alita/modules/lockdown_recent.go:21-29]. Keep the same 250 ms bound (`lockdownRecentTimeout`) on the join hot path and use a similar bound on the message path; errors are logged at debug and treated as "no information", never as a trigger and never as a ban.

`ZADD` without `NX` is intended for `joined`: the member is the user ID, so the two deliveries of one join (chat_member + service message) collapse into one member by construction, and a re-join refreshes recency. The surge count is `ZCOUNT joined (now-60s +inf`, i.e. distinct accounts that joined in the last 60 s. Do not use `ZCARD` of the tracker and do not use `ZADD NX` here (NX would keep an old score and make a rejoining user look 23 hours old).

Why not reuse `alita:lockdown:recent:<chat>`: `trackLockdownRecentJoins` stores `json.Marshal(lockdownRecentJoin{UserID, Name, Username, Bot, Link})` as the member and uses `ZAddNX`, so the same user delivered twice with and without an invite link is two members; `readLockdownRecentJoins` merges them "by user ID at read time". [VERIFIED: lockdown_recent.go:36-45, 82, 92, 128-157]

### Pattern 5: Lua scripts (atomic across replicas)
Time is passed in from Go (`time.Now().UnixMilli()`), the repo's own pattern (`trackJoinScript` takes `now` as an argument). Clock skew between replicas is NTP-small against 30-60 s windows. [ASSUMED]. Make the clock injectable (`var raidNow = time.Now`) so tests are deterministic.

Both scripts below were executed against the repo's miniredis v2.39.0 on this machine (via a temporary `go run` inside the module, removed afterwards) and returned the expected counts: a user delivered twice counted once; a join older than 60 s was excluded from the window count; user IDs 1, 2 (in `joined`) counted as recent, 100 did not; the same album key sent twice counted once; after the 30 s window the sets drained to `[1 0 0]`; `SET NX` latch returned `true` then `false`; TTL was set (2 m). `ZSCORE` of a missing member arrives in Lua as `false` and the `if joinedAt and ...` guard handles it. [VERIFIED: ran against miniredis v2.39.0]

### Pattern 6: One shared start function (extract from `/lockdown`)
`lockdown.go:287-421` currently interleaves replies with the sequence. Extract everything from "bot rights" to "ConfirmLocked + wake" into `startLockdownRow` that returns an outcome; `/lockdown` maps outcomes to exactly the replies it sends today, the trigger maps them to log lines, warn and notice. The `/lockdown`-only parts stay in the handler: the supergroup reply, the `GetActiveFresh` + `settleUnconfirmedLockdown` pre-step, the actor and reason, the success reply with notes.

Outcomes needed (each corresponds to an existing branch, quoted from `lockdown.go`):

| Outcome | Existing branch | `/lockdown` reply key today | Trigger action |
|---------|-----------------|-----------------------------|----------------|
| `startNotSupergroup` | `chat.Type != "supergroup"` | `lockdown_basic_group` | warn (D-14) |
| `startBotCheckFailed` | `botResult == chat_status.BotMemberUnknown` | `lockdown_bot_check_failed` | log, short latch, no warn |
| `startBotCannotRestrict` | `BotMemberMissing \|\| Status != administrator \|\| !CanRestrictMembers` | `lockdown_bot_cannot_restrict` | warn once/day (D-14) |
| `startPermissionsUnreadable` | `fetchLockdownChat` error or `!lockdownHasPermissions` | `lockdown_permissions_unreadable` | log + warn-eligible? (see Pitfall 9) |
| `startAlreadyActive` | `!started` from `lockdown.Start` | `lockdownAlreadyActiveText` | silent |
| `startStateFailed` | `lockdown.Start` error | `lockdown_state_failed` | log, short latch |
| `startLockFailed` | `lockdownDefinitiveRefusal(err)` then `DeleteUnconfirmed` | `lockdown_lock_failed` | log, treat like cannot-lock (warn) |
| `startLockUnknown` | non-definitive lock error, row kept | `lockdown_lock_unknown` | log; worker settles the row |
| `startStarted` | `ConfirmLocked` ok (or its retry failed: logged, still started) | success text | notice in the group |

The row the trigger writes (fields verified in `lockdown.go:355-364` and `models/lockdown.go`): `ChatID`, `TriggerKind` (new constants), `Reason`, `StartedBy: b.Id`, `StartedByName` (the bot's name, capped with `lockdownCapRunes(..., lockdownNameMaxRunes)`), `ChatTitle: lockdownCapRunes(chat.Title, lockdownTitleMaxRunes)`, `PrePermissions: string(chatInfo.Permissions)`, `LockedPermissions: lockdownLockedPermissions`, plus the three new trigger columns.

Do the trigger start **synchronously in the handler** (guarded by `defer error_handling.RecoverFromPanic`), not in a goroutine. It needs no new goroutine, no WaitGroup and no shutdown drain: a process killed mid-sequence leaves an unconfirmed active row that `lockdownSettleUnconfirmed` settles after `lockdownUnconfirmedGrace`, which is exactly the Phase 4 design. The cost is one ~1-3 s handler for the single winner of the latch.

### Pattern 7: Settings repository (column-targeted upsert, no GORM zero-value trap)
Model without `default:` tags on the booleans (a Go `false` on a `default:true` column is omitted by GORM Create and the DB default silently turns it back on; this is the CLAUDE.md "Updating Record Without Checking Zero Values" anti-pattern). Defaults live in Go (`defaultRaidDetection(chatID)`: both on, 15 and 30) and in the SQL `DEFAULT`s. Writes are one statement per trigger using `clause.OnConflict{Columns: chat_id, DoUpdates: AssignmentColumns(...)}` over only that trigger's columns, so two admins changing `joins` and `flood` at once cannot clobber each other (a read-modify-write of the whole row would). `DeleteCache(cache.CacheKey("raid_detection", chatID))` after the write. Read via `GetFromCacheOrLoad(ctx, key, cache.CacheTTLRaidDetection, loader)` where the loader returns the defaults on `gorm.ErrRecordNotFound` (same shape as `antiraid.getAntiRaidSettingsRawContext`).

`skipLocal` addition, following the verbatim pattern in `alita/db/cache/local.go` (`staffGroupPrefix = CacheKey("staff_group") + ":"`, `return strings.HasPrefix(key, captchaPendingPrefix) || ...`):
```go
raidDetectionPrefix = CacheKey("raid_detection") + ":"
// ... in skipLocal:
    strings.HasPrefix(key, raidDetectionPrefix)
```
and extend `alita/db/cache/local_test.go:286` (the prefix list) with `"raid_detection"` and `"raid_detectionx"` in the negative list.

Cost note: D-12 means every counted message does one Redis GET for the settings (local layer skipped) plus one script call. Order the cheap in-memory filters first (see Pitfall 3).

### Pattern 8: `/autolockdown` authority (view vs change in one command)
`requireLockdownAuthority(needRestrict bool)` is a `helpers.CheckFunc` that does the live `getChatMember`, prompts `PromptAnonAdminProof` for anonymous admins, and replies its own refusals. One command needs "any live admin" to view and "owner or can_restrict_members" to change, so wrap by argument count in the descriptor:
```go
func requireAutoLockdownAuthority() helpers.CheckFunc {
	view, change := requireLockdownAuthority(false), requireLockdownAuthority(true)
	return func(c *helpers.CommandContext) bool {
		if len(lockdownCommandArgs(c.Ctx)) == 0 {
			return view(c)
		}
		return change(c)
	}
}
```
(`lockdownCommandArgs` is the existing helper returning the words after the command.) Descriptor: `Name: "autolockdown"`, `RequiredChecks: {requireLockdownGroup(), requireAutoLockdownAuthority()}`, not Disableable (like `lockdownDesc`). Register `RegisterAnonymousAdminHandler("autolockdown", anonPipelineHandler(autolockdownDesc, autolockdownModule.autolockdown))` in `init()` next to the three existing registrations. Do the same for the alias `autoantiraid`. The anonymous re-entry swaps in the tapper as sender and the same live check then runs for them (AGENTS.md Handlers). No callback buttons in this phase, so no `callbackcodec` use.

Argument grammar (recommended): `joins|flood` then `on|off|<N>`. `N` sets the threshold and turns the trigger on (an admin typing a number wants it active) [ASSUMED]. `off` keeps the stored threshold. Bounds: joins 3..500, flood 10..1000 [ASSUMED], with a `lockdown_auto_*` usage/range reply on invalid input. The windows (60 s / 30 s), the 60 % and the 3-accounts floor are code constants, not settings (windows are deferred by CONTEXT).

### Pattern 9: Trigger evaluation and the latch
```text
raidFire(b, chat, kind, count, window, percent):
  if !latch(SET NX EX 60 alita:raid:{chat}:fire)            -> return   // another handler/replica is on it
  recently lifted (lift_started_at within 10 min)?          -> return   // D-15 (leave latch)
  r := startLockdownRow(kind, count, window, percent)
  switch r.Outcome:
    started:        send public notice (lockdownPaced), wake worker
    alreadyActive:  nothing
    cannotLock / notSupergroup / lockFailed:  warn once/day, extend latch to ~10 min
    transient (botCheckFailed, stateFailed, readFailed):  PEXPIRE latch 10 s
```
The latch is load shedding only. Correctness is `uk_chat_lockdowns_active` + `lockdown.Start`: its comment says "exactly one caller ever sees true for a chat, on any replica", and `Start` inserts *before* the lock call, so a loser never makes a `setChatPermissions` call. [VERIFIED: alita/db/lockdown/repository.go:48-66]. Exactly one alert: `BeginAlert`'s conditional `UPDATE ... alert_started_at IS NULL` [VERIFIED: alita/db/lockdown/alerts.go:48-50], run by the worker only. A trigger that loses `Start` because another trigger just won sees `startAlreadyActive` and does nothing. If the winner's lock is definitively refused it deletes its unconfirmed row; the loser's latch expires in 60 s and the still-high counters re-fire it, so a raid is not lost.

### Anti-Patterns to Avoid
- **Deciding on `ZCARD` of `alita:lockdown:recent:*`:** double counts; see Pattern 4.
- **Per-replica in-memory counters:** antiflood's are per replica (AGENTS.md); PLAT-01 requires shared detection counters.
- **Using `IsBot()` alone to skip anonymous admins:** `Sender.IsBot()` returns `s.Chat == nil && s.User != nil && s.User.IsBot`, so it is false when `sender_chat` is set. [VERIFIED: gotgbot `sender.go:137`]. Check `Sender.Chat != nil` explicitly.
- **Writing the trigger start in its own goroutine:** would need a drain and recover wiring for no gain.
- **A fresh `getChatMember` per message to exempt admins:** D-07 explicitly allows the cached list; reuse `antifloodModule.userIsFloodExempt`, which is cached-first with a bounded, semaphore-limited live fallback that assumes admin when the semaphore is full "to prevent false positives". [VERIFIED: antiflood.go:105-153]
- **Baking a localized sentence into `chat_lockdowns.reason`:** the alert is rendered per destination in that chat's language (`lockdownSlotTranslator`); a stored English string would break PLAT-03 for non-English Staff Groups.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Exactly-one lockdown | A Redis lock around Start | `lockdown.Start` + `uk_chat_lockdowns_active` | Already proven; the latch is only a load limiter |
| Exactly-one alert | A "fired" flag | `lockdown.BeginAlert` via the worker | Already conditional |
| Live authority for `/autolockdown` | A new admin check | `requireLockdownAuthority(true/false)` | Live `getChatMember`, anon proof, localized refusals already there |
| Performer / admin-added rule on joins | A second join classifier | `decideLockdownJoin` + `lockdownNeedsPerformerLookup` + `lockdownLiveMember` | One rule, shared with the guard |
| Admin exemption on messages | A new admin cache | `antifloodModule.userIsFloodExempt` (+ `IsApprovedForUpdate`) | Cached-first, bounded |
| Fleet-wide Telegram pacing for the notice/warn posts | A per-replica limiter | `lockdownPaced` (`lockdownPacer`) | AGENTS.md: per-replica limiters must not be used for shared budgets |
| Log/Staff destination resolution for the warning | New lookups | `staff.GetLinkOfGroupFresh` + `actionlog.Destination(chat, logchannels.CategoryAdmin)` (as `lockdownAlertDestinations` does) | Same checks as every log post |
| Per-chat language for notice/warning | `i18n` by hand | `staffChatTranslator(chatID)` | Returns the translator for a chat's own language without a command context |
| Parsing `/autolockdown` N | Custom parser | `strconv.Atoi` + range check + `lockdownCommandArgs` | Trivial; keep bounds as constants |
| Stale-key sweep | Per-key lookups | One `SCAN alita:antiraid:*` + `UNLINK` | Bounded, idempotent |

**Key insight:** every hard problem here (exactly-once, authority, pacing, crash recovery, alerts) was solved in Phases 4-5. The risk in this phase is *integration*: extracting the start sequence without changing `/lockdown`, and getting the Redis counters and exemptions right.

## Runtime State Inventory

This phase retires a module and migrates a setting, so the inventory is required.

| Category | Items Found | Action Required |
|----------|-------------|------------------|
| Stored data | `antiraid_settings` rows (`raid_time`, `raid_action_time`, `auto_antiraid_threshold`, table created by `20260518113000_add_antiraid_settings.sql`, FK to `chats` ON DELETE CASCADE when `chats` exists). Backup/export/import/reset still reads and writes `models.AntiRaidSettings` (`alita/db/backup/backup.go:23,338,466,566,882,958`, `types.go:96,119,203`). Dev `/stats` reads `antiraid.LoadAntiRaidStats()` (`alita/db/devs/repository.go:185,289-291`). | **Data migration** (one INSERT...SELECT, D-03) for the non-zero thresholds. **Code edit**: after the migration `/autoantiraid` writes only `chat_raid_detection`, so `auto_antiraid_threshold` drifts and the backup "antiraid" module and dev stats become misleading. Recommend: leave the backup module and table alone (append-only, out of scope), switch the dev-stats lines to the new table or drop them, and document that restoring an old backup does not change detection settings (Open Question 5). |
| Live service config | None. No external UI/DB-held config references the string `antiraid`; Telegram config is only `allowed_updates` (already includes `chat_member` for the Phase 4 guard). Verified by grep of non-test, non-docs source: only the files listed in this research mention antiraid. | None |
| OS-registered state | None. The expiry poller is an in-process goroutine, not an OS task. | None |
| Secrets / env vars | None reference antiraid by name (`docs/.../environment.md:455` mentions "Active anti-raid state" only in prose about `CLEAR_CACHE_ON_STARTUP`). No new secrets this phase, so no `logredact.RegisterSecret`. | Reword the prose line; no code |
| Build artifacts / installed packages | None. No package rename. `coverage.out` in the repo root mentions antiraid; it is a generated artifact. | None |
| **Redis (live keys)** | `alita:antiraid:state:<chat>` (raid mode; TTL up to 366 days via `maxAntiRaidDuration`) and `alita:antiraid:joins:<chat>` (60 s) [VERIFIED: antiraid.go:37-39,124-130]. They sit outside `alita:cache:` so `CLEAR_CACHE_ON_STARTUP` never clears them (`alita/utils/cache/startup_test.go:16,37` uses `alita:antiraid:state:-1001` as the preserved example). | **Startup sweep** (D-16, recommended over lazy): in the shrunken `LoadAntiRaid`, when Redis is available, one recover-protected goroutine does `SCAN MATCH alita:antiraid:* COUNT 100` + `UNLINK`, once per process. Nothing reads these keys after this phase, so a late or missed sweep only wastes memory. Rolling deploy: an old replica could briefly recreate a state key; the next restart's sweep removes it. |

**Nothing found in:** Live service config, OS-registered state, Secrets/env, Build artifacts (verified by the greps above).

## Common Pitfalls

### Pitfall 1: `withMiniredis` lives in a file this phase deletes
**What goes wrong:** `withMiniredis` is defined in `alita/modules/antiraid_miniredis_test.go:22`. It is called from `aispam_test.go:333`, `staff_*_test.go`, `lockdown_fake_test.go:337`, `filters_command_test.go` and more. Deleting the antiraid test files breaks the whole package's test build.
**How to avoid:** first task of the retire step: move `withMiniredis` (and its imports) to a new `alita/modules/testredis_test.go` (build tag `testtools`), then delete/replace `antiraid_test.go` (1034 lines) and `antiraid_miniredis_test.go`. Also fix `callback_nil_handlers_test.go:16,49` (`antiRaid := &antiRaidStruct{}` and the `antiRaidCallbackHandler` row) and delete `TestAntiRaidStepsAsideDuringLockdown` in `lockdown_isolation_test.go:240` (it references `LoadAntiRaid`, `antiRaidModule.enableRaid/disableRaid`, `joinsKey`, `antiraid.SetAutoAntiRaidThreshold`).
**Warning signs:** `undefined: withMiniredis` at compile time.

### Pitfall 2: Anonymous admins and channels slip past `IsBot()`
**What goes wrong:** the default `handlers.NewMessage` skips senders whose `IsBot()` is true and skips edited and channel messages, which is why edits never reach the watcher. But for an anonymous admin `sender_chat` is the group and `from` is the Group Anonymous Bot, so `Sender.IsBot()` is false; a linked channel's automatic forward has `from` 777000 (not a bot) and `sender_chat` the channel. [VERIFIED: gotgbot `ext/handlers/message.go:CheckUpdate`, `sender.go`]
**How to avoid:** in the watcher return early when `ctx.EffectiveSender == nil`, `ctx.EffectiveSender.Chat != nil` (covers anonymous admin, anonymous channel, linked channel), `msg.IsAutomaticForward`, or `staffServiceUserIDs[userID]` (`map[int64]bool{777000: true, 1087968824: true, 136817688: true}`, `staff_action_decide.go:513`), or `userID == b.Id`. This mirrors `checkFlood`'s own `user.IsAnonymousAdmin() || user.IsLinkedChannel()` guard.

### Pitfall 3: Hot-path cost on every group message
**What goes wrong:** with D-12 the settings key skips the local cache, so each counted message costs a Redis GET, an admin lookup and a script call. A cache miss in `IsUserAdmin` can call `getChat` + load the admin list.
**How to avoid:** order checks cheapest first: (1) `cache.IsRedisAvailable()` and `chat.Type == "supergroup"`; (2) sender shape and item classification (pure Go); (3) settings (`flood_enabled` false returns here); (4) approved users (`IsApprovedForUpdate`, local-cached); (5) `userIsFloodExempt` (cached-first); (6) the script. Guard the Redis calls with a short timeout (~250 ms, like `lockdownRecentTimeout`) and ignore errors.

### Pitfall 4: What counts as an item
**What goes wrong:** counting service messages, commands, album parts or deleted-by-locks messages skews the ratio.
**How to avoid:** use an allow-list function `raidFloodItemKey(msg) (key string, ok bool)`:
- text without a leading `bot_command` entity (`message.Command` semantics: first entity type `bot_command` at offset 0), photo, animation (GIF; Telegram also sets `document` on an animation, so test animation first), sticker, plus video, video note, voice, audio, document, poll, contact, location, venue, dice [ASSUMED: CONTEXT says "messages, photos, GIFs or stickers"; counting other user-authored content makes sense for "messages" but confirm].
- Never count: `NewChatMembers`, `LeftChatMember`, pinned/title/photo/migrate service messages, `ViaBot`-only inline results are fine to count.
- Album parts share a `media_group_id`: use the item key `g:<media_group_id>` so a 10-photo album is one item (otherwise three new members each posting one album reach 30 items instantly). Antiflood skips albums for the same reason.
- Forwards count (spam is often forwarded).
- Edited messages and channel posts never reach the handler (defaults).
- Forum topics: same chat ID, nothing special.

### Pitfall 5: False positives of the "60 % from new members" rule
**What goes wrong:** a legitimately busy event right after a join wave, a brand-new group where most members joined today, or an announcement that makes newcomers chat all at once can satisfy 30 items / 30 s with ≥ 60 % from ≥ 3 accounts who joined < 24 h ago.
**Why it is still the right default:** D-06 is locked and the owner accepted that a false lockdown costs one `/unlockdown`. Mitigations that stay inside the decisions: admin-added and approved users are excluded from "recent joiners", the 3-account floor stops one account from tripping it, per-group tuning via `/autolockdown flood N|off`, and the post-lift cooldown (D-15). Mention in help text that approving trusted newcomers keeps them out of detection. A possible later refinement, not recommended now: also require the group to be larger than the number of recent joiners.
**Warning signs:** repeated `flood` lockdowns in a small or new group.

### Pitfall 6: Default-on large public groups and join surges
15 joins per 60 s is routine for a big public group; with D-01/D-03 it is on by default there. Not a bug, but the planner should make `/autolockdown` output show the threshold and how to change it, and the pointer replies should name `/autolockdown joins N`.

### Pitfall 7: Carried-over thresholds of 1 or 2
`/autoantiraid 1` was legal, so D-03 would copy `1` and every single join would lock the group (the old behaviour only temp-banned later joiners). Copy verbatim as D-03 says, make the new column CHECK `BETWEEN 1 AND 1000`, validate 3..500 for new input, and flag it in Open Question 2 for the owner (a `GREATEST(..., 3)` clamp in the migration is the alternative).

### Pitfall 8: GORM zero values on the settings model
See Pattern 7. A model with `default:true` booleans plus `Create` silently ignores `false`. Test: set `joins off`, read back through a *cold* cache (call `cache.ResetLocalForTest()` / `DeleteCache`), and assert it is off; then `joins 20` and assert enabled + 20.

### Pitfall 9: Permissions unreadable / lock refused are "can't lock" too
D-14 lists "not an admin with the restrict right, or not a supergroup". `startPermissionsUnreadable` and `startLockFailed` (definitive 4xx refusal) are the same class from the admin's point of view ("I can't lock this group"). Recommend the warning for `cannot restrict`, `not supergroup` and `lock failed`; only log for `permissions unreadable`, `bot check failed` and `state failed` (transient). Note the flood watcher and the join hook only run in supergroups (the guard's tracker early-returns for other types), so `startNotSupergroup` is defensive; see Open Question 3.

### Pitfall 10: The flood lockdown's burst is empty
**What goes wrong:** `lockdownBurstSnapshot` saves only joins in `(locked_at - 10 min, locked_at]` from the tracker as `pre` burst rows [VERIFIED: lockdown_alert.go:204-230]. For a flood the offenders joined up to 24 h ago, so the alert shows `Joined in the 10 minutes before the lockdown: 0` and the "Ban N recent joiners" button has nothing to ban (`lockdown_ban_nothing`), which defeats the point of LOCK-11/12 for the flood trigger.
**How to avoid (recommended, descopable):** when the flood fires, keep the `fa` offenders set for 15 minutes (do not delete it on fire) and extend `lockdownBurstSnapshot` for `TriggerKind == flood` to also read `fa`, resolve names best effort from the users table, and add them as `BurstSourcePre` rows with `JoinedAt` from the `joined` score; add a flood-specific counts line key instead of "Joined in the 10 minutes before". If descoped, the alert still names the flood and its numbers (success criterion) and the Ban button degrades to its generic label. Raise as Open Question 1.

### Pitfall 11: Latch lifetime vs cooldown
If the latch is deleted on every outcome, a sustained flood re-runs `getChatMember` + DB reads on every message past the threshold. Keep it for 60 s; extend it on "can't lock"; shorten it to ~10 s only on transient failures.

### Pitfall 12: Antiraid removal touches more than the module
Files and places that mention the module (grep this session): `main.go:196-200` (shutdown handler "Stopping anti-raid expiry poller..."), `alita/modules/antiraid.go`, `antiraid_test.go`, `antiraid_miniredis_test.go`, `callback_nil_handlers_test.go`, `lockdown_isolation_test.go`, `lockdown_guard.go` and `lockdown.go` (comments naming antiraid -5), `staff_sweeper.go:117` (comment only), `test_harness_test.go:350` and `alita/db/testmain_test.go:68` (keep `AntiRaidSettings` in AutoMigrate; the table stays), `locales/*.yml` (`antiraid_*` keys, `antiraid_help_msg`), `locales/config.yml:9` (`AntiRaid: [antiraid, raid]`), `docs/...` (see Documentation list), `AGENTS.md`. `helpers.AddCmdToDisableable("antiraid")` is dropped. `parseDuration`, `formatDuration`, `banRaidMember` and the Redis helpers die with the module (nothing else uses `parseDuration`/`formatDuration`/`maxAntiRaidDuration`: grep found no other non-test use).

### Pitfall 13: ZSET score precision and cap trimming
Millisecond epochs (~1.8e12) are exact in a double. Capping `joined` at 5000 drops the *oldest* joiners, so under an extreme raid some real recent joiners stop counting as recent; this is bounded memory over precision and acceptable.

### Pitfall 14: A pending `edit` in a rolling deploy
During a rolling deploy an old replica can still run `onJoin` and temp-ban. Document it in the plan's deployment note; no code can prevent it.

## Code Examples

### Lua: join script (verified against miniredis v2.39.0)
```lua
-- KEYS[1] = alita:raid:{chat}:joined
-- ARGV: 1=now_ms 2=surge_window_ms 3=recent_ms(24h) 4=cap 5=ttl_ms 6..N=user ids
local now = tonumber(ARGV[1])
for i = 6, #ARGV do
  redis.call('ZADD', KEYS[1], now, ARGV[i])
end
redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', now - tonumber(ARGV[3]))
local size = redis.call('ZCARD', KEYS[1])
local cap = tonumber(ARGV[4])
if size > cap then redis.call('ZREMRANGEBYRANK', KEYS[1], 0, size - cap - 1) end
redis.call('PEXPIRE', KEYS[1], ARGV[5])
return redis.call('ZCOUNT', KEYS[1], '(' .. (now - tonumber(ARGV[2])), '+inf')
```
Go side: `joinScript.Run(ctx, client, []string{raidJoinedKey(chatID)}, now, window, recent, cap, ttl, uid1, uid2...).Int()`. (The harness run passed three extra placeholder arguments at positions 6-8, which only added a stray member `0`; the real call passes just the user IDs.)

### Lua: flood script (verified against miniredis v2.39.0)
```lua
-- KEYS: 1=items 2=new_items 3=new_accounts 4=joined
-- ARGV: 1=now_ms 2=window_ms 3=recent_ms 4=item 5=uid 6=cap 7=ttl_ms
local now = tonumber(ARGV[1])
local cutoff = now - tonumber(ARGV[2])
redis.call('ZADD', KEYS[1], now, ARGV[4])
redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', cutoff)
local joinedAt = redis.call('ZSCORE', KEYS[4], ARGV[5])
if joinedAt and tonumber(joinedAt) > now - tonumber(ARGV[3]) then
  redis.call('ZADD', KEYS[2], now, ARGV[4])
  redis.call('ZADD', KEYS[3], now, ARGV[5])
end
redis.call('ZREMRANGEBYSCORE', KEYS[2], '-inf', cutoff)
redis.call('ZREMRANGEBYSCORE', KEYS[3], '-inf', cutoff)
local cap = tonumber(ARGV[6])
for i = 1, 2 do
  local n = redis.call('ZCARD', KEYS[i])
  if n > cap then redis.call('ZREMRANGEBYRANK', KEYS[i], 0, n - cap - 1) end
end
for i = 1, 3 do redis.call('PEXPIRE', KEYS[i], ARGV[7]) end
return {redis.call('ZCARD', KEYS[1]), redis.call('ZCARD', KEYS[2]), redis.call('ZCARD', KEYS[3])}
```
Go decision (constants `raidFloodNewPercent = 60`, `raidFloodMinAccounts = 3`):
```go
items, newItems, accounts := res[0], res[1], res[2]
fire := items >= threshold && newItems*100 >= raidFloodNewPercent*items && accounts >= raidFloodMinAccounts
percent := int(newItems * 100 / items) // shown on the alert
```

### Shared start function skeleton (identifiers in the "reused" column are quoted from lockdown.go as read this session)
```go
// NEW: lockdown_start.go
type lockdownStartParams struct {
	Chat        *gotgbot.Chat
	TriggerKind string // models.LockdownTriggerManual / NEW JoinSurge / Flood
	Reason      string
	StarterID   int64
	StarterName string
	Count, Window, Percent int // trigger columns, 0 for manual
}

// Reused verbatim from lockdown.go:355-364 for the row:
//   ChatID, TriggerKind, Reason, StartedBy, StartedByName, ChatTitle: lockdownCapRunes(chat.Title, lockdownTitleMaxRunes),
//   PrePermissions: string(chatInfo.Permissions), LockedPermissions: lockdownLockedPermissions
// Reused calls: chat_status.FetchBotMember, fetchLockdownChat, lockdownHasPermissions, lockdown.Start,
//   setLockdownPermissions, lockdownDefinitiveRefusal, lockdown.DeleteUnconfirmed, lockdown.ConfirmLocked,
//   wakeLockdownWorker
func startLockdownRow(ctx context.Context, b *gotgbot.Bot, p lockdownStartParams) lockdownStartResult
```
`/lockdown` keeps its `GetActiveFresh` + `settleUnconfirmedLockdown` pre-step and keeps `botMember.CanDeleteMessages/CanInviteUsers` notes (return the `MergedChatMember` in the result). Existing tests that pin `/lockdown` behaviour and must stay green unchanged: `lockdown_test.go`, `lockdown_refusals_test.go`, `lockdown_lock_unknown_test.go`, `lockdown_durability_test.go`, `lockdown_anon_test.go`.

### Settings upsert (column-targeted)
```go
row := defaultRaidDetection(chatID)       // Go defaults for the other trigger's columns on first insert
row.JoinEnabled, row.JoinThreshold = enabled, threshold
err := db.DB.Clauses(clause.OnConflict{
	Columns:   []clause.Column{{Name: "chat_id"}},
	DoUpdates: clause.AssignmentColumns([]string{"join_enabled", "join_threshold", "updated_at"}),
}).Create(&row).Error
if err == nil { cache.DeleteCache(cache.CacheKey("raid_detection", chatID)) }
```
(`alita/db/cache` `cache.CacheKey`/`cache.DeleteCache` as used in `alita/db/antiraid/repository.go:43`.)

### Migration (one file, timestamp > 20261011160000)
```sql
-- chat_raid_detection: per-chat automatic raid detection settings ...
CREATE TABLE IF NOT EXISTS chat_raid_detection (
    id BIGSERIAL PRIMARY KEY,
    chat_id BIGINT NOT NULL,
    join_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    join_threshold INTEGER NOT NULL DEFAULT 15,
    flood_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    flood_threshold INTEGER NOT NULL DEFAULT 30,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    CONSTRAINT uk_chat_raid_detection_chat UNIQUE (chat_id),
    CONSTRAINT chk_chat_raid_detection_join CHECK (join_threshold BETWEEN 1 AND 1000),
    CONSTRAINT chk_chat_raid_detection_flood CHECK (flood_threshold BETWEEN 1 AND 1000)
);
ALTER TABLE chat_lockdowns ADD COLUMN IF NOT EXISTS trigger_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE chat_lockdowns ADD COLUMN IF NOT EXISTS trigger_window INTEGER NOT NULL DEFAULT 0;
ALTER TABLE chat_lockdowns ADD COLUMN IF NOT EXISTS trigger_percent INTEGER NOT NULL DEFAULT 0;
CREATE INDEX IF NOT EXISTS idx_chat_lockdowns_chat_lift ON chat_lockdowns (chat_id, lift_started_at);
INSERT INTO chat_raid_detection (chat_id, join_enabled, join_threshold, flood_enabled, flood_threshold)
SELECT chat_id, TRUE, auto_antiraid_threshold, TRUE, 30
FROM antiraid_settings
WHERE auto_antiraid_threshold > 0
ON CONFLICT (chat_id) DO NOTHING;
```
Notes: no FK on `chat_id` (the staff/lockdown precedent: "There is deliberately no foreign key on chat IDs"). `trigger_kind VARCHAR(16)` already fits `join_surge` (10) and `flood` (5): the column is declared `trigger_kind VARCHAR(16) NOT NULL DEFAULT 'manual',` [VERIFIED: migrations/20261006120000_add_chat_lockdowns.sql:34]. No existing migration uses `INSERT` or `ON CONFLICT` (grep found none), but the runner executes each statement with `tx.Exec` after `cleanSupabaseSQL`/`splitSQLStatements`, which only rewrite `CREATE TABLE/INDEX/TYPE` and `ALTER TABLE ... ADD CONSTRAINT`, and an `UPDATE` data statement precedent exists in `20261011160000_add_lockdown_ban_run_report.sql`; the new migration must still be proven by the PostgreSQL migration-chain test in CI (`ALITA_TEST_MIGRATION_CHAIN=true`, `repository_chain_test.go`). [VERIFIED: runner.go:392-470]. SQL comments with semicolons are fine (existing migrations have them). The old threshold column is unchanged.

### Shutdown change
`main.go:196-200` currently registers `modules.StopAntiRaidExpiryPoller()`. Remove that handler. No new shutdown drain is needed because the trigger runs synchronously inside the update handler and everything after the lock is the already-registered `RegisterLockdownWorkerDrain`. [VERIFIED: main.go:196-222]

### Trigger line localization (new keys use the `lockdown_` prefix so `TestLockdownLocaleKeys` covers all 7 locales for free)
`alita/i18n/lockdown_locale_test.go` checks every key starting with `lockdown_` for parity, non-empty values and identical placeholders in all 7 locales. [VERIFIED: alita/i18n/lockdown_locale_test.go:10-85]. Recommended new keys (names are suggestions): `lockdown_alert_trigger_join_surge` ("Join surge: {count} joins in {window} s. Locked automatically at {time}."), `lockdown_alert_trigger_flood` ("Flood: {count} messages in {window} s, {percent}% from new members. Locked automatically at {time}."), `lockdown_auto_notice_join_surge`, `lockdown_auto_notice_flood` (public notice, names the count, says locked automatically and that an admin can `/unlockdown`), `lockdown_auto_cant_lock_warning`, `lockdown_auto_status*`, `lockdown_auto_joins_set`, `lockdown_auto_flood_set`, `lockdown_auto_usage`, `lockdown_auto_range`, `lockdown_auto_save_failed`, `lockdown_antiraid_retired`. The group title and any name must be spliced in after translation with the existing tokens (`lockdownGroupToken`, `lockdownNameToken`). `lockdownRenderAlert` switches on `ld.TriggerKind` for its trigger line (today it always uses `lockdown_alert_trigger_manual`, `lockdown_alert.go:472-478`); the status text and `lockdownAlreadyActiveText` should show the same trigger line for non-manual kinds instead of "started by <bot>".

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Raid mode: per-chat Redis state, expiry poller, temp-bans every joiner (`antiraid.go`) | Detect, then run the Phase 4/5 lockdown; no bans except the lockdown's own marker bans | This phase | Joiners are removed by the guard/worker and unbanned at the lift; nobody is temp-banned |
| Counting joins in a 60 s ZSET with a per-join unique member (`userID:now:nanos`) | Member = user ID, so duplicates collapse | This phase | Double delivery counted once; also the 24 h recent set |
| Antiflood per-replica counters | Raid counters in Redis | This phase | PLAT-01 |

**Deprecated/outdated:** `/antiraid`, `/raidtime`, `/raidactiontime` (pointer replies only), `antiraid_settings.raid_time/raid_action_time` (unread), `antiraid_auto_triggered` and the other `antiraid_*` strings (unused; removing them from all 7 locales is optional hygiene since `check-translations` only checks for missing keys).

## Documentation and Test Files Touched by Retiring antiraid

- Docs (several are frozen with `<!-- MANUALLY MAINTAINED: do not regenerate -->` and must be edited by hand): `docs/src/content/docs/commands/antiraid/index.md` (frozen), `docs/src/content/docs/api-reference/commands.md` (frozen; AntiRaid rows at ~44-51, 345-350, 425-426), `api-reference/callbacks.md` (~44, 105-110), `architecture/handler-groups.md` (row for -5 at line 17, prose at 36-37, 52, 61), `architecture/caching.md` (~30, 59-60, 120), `architecture/request-flow.mdx` (~242, 291), `architecture/project-structure.mdx` (~34, 58), `api-reference/environment.md:455`, `self-hosting/database.md:266`, `api-reference/database-schema.md:701-719` (and add the new table), `commands/backup/index.md:39`, `commands/index.mdx:32`. The Lockdown page is generated: add `/autolockdown` to `lockdown_help_msg` in all 7 locales, add `autolockdown` to `Lockdown:` in `locales/config.yml:34`, rewrite `antiraid_help_msg`, then run `make generate-docs` and `make check-docs`.
- `AGENTS.md`: Handlers list (remove `-5 antiraid`, add `-4`), "Operational Redis keys" line (add `alita:raid:*`), the Data bullets (new `chat_raid_detection`, the `alita:raid:*` keys, the trigger columns, "never part of backup/export/import/reset"), the "Antiraid is Redis-only..." trap and the "Antiraid's join handler (`onJoin`, group `-5`) returns at once while a confirmed lockdown ... (until Phase 6 retires it)" bullet, and the new Subsystem traps (latch, cooldown, warn dedupe, fixed windows, flood item rules).
- Test files: replace `antiraid_test.go`/`antiraid_miniredis_test.go` (keep `withMiniredis` first), edit `callback_nil_handlers_test.go`, delete `TestAntiRaidStepsAsideDuringLockdown`, add `models.ChatRaidDetection` to the AutoMigrate lists in `alita/modules/test_harness_test.go` (list at ~315-360), `alita/db/testmain_test.go` (~44-85) and the new `alita/db/raiddetection/testmain_test.go`. `ChatLockdown`'s new columns need no list change (already migrated by model).

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | Telegram Bot API facts not visible in gotgbot (e.g. `chat_member` `from` is the user for invite-link joins; animations also carry `document`) are as remembered; core.telegram.org was unreachable (403 from egress policy) | Pattern 3, Pitfall 4 | A mis-classified join/GIF; mitigated by tests with explicit fixtures |
| A2 | Redis hash tags (`{chat}`) in key names are harmless and make the flood script cluster-safe | Pattern 4 | None on a single node; if the repo later adds a cluster, other keys are not tagged |
| A3 | Passing `now` from Go (not Redis `TIME`) is fine; replica clock skew is negligible against 30-60 s windows | Pattern 5 | A skewed replica shifts counts by its skew |
| A4 | `/autolockdown joins N` also turns the trigger on; `off` keeps the threshold | Pattern 8 | Owner might prefer N to leave an off trigger off |
| A5 | Threshold bounds joins 3..500, flood 10..1000 for new input; DB CHECK 1..1000 | Pattern 8, Pitfall 7 | Owner wants other bounds |
| A6 | Counted item kinds beyond text/photo/GIF/sticker (video, voice, document, ...) | Pitfall 4 | Narrower or wider detection than the owner expects |
| A7 | A new `alita:raid:*` family is acceptable (vs. nesting under `alita:lockdown:`) | Pattern 4 | Rename before coding; AGENTS.md says to document the family either way |
| A8 | Synchronous trigger start inside the handler is acceptable latency (~1-3 s, one winner per 60 s per chat) | Pattern 6 | If not, move to a tracked goroutine with a drain |
| A9 | `GREATEST` clamp is not applied to carried thresholds (verbatim copy per D-03) | Pitfall 7 | Groups with threshold 1-2 lock on the first join |
| A10 | Cooldown read error means "skip this attempt and retry soon" rather than "start anyway" | Pattern 9 | A transient DB error delays one lockdown by ~10 s |

## Open Questions

1. **Flood lockdown burst / Ban button (Pitfall 10).**
   - What we know: the alert's burst comes only from the 10-minute join tracker; flooders joined up to 24 h earlier.
   - What's unclear: whether the owner wants the flooding accounts listed and bannable from a flood alert.
   - Recommendation: include it (keep `fa` for 15 min, extend `lockdownBurstSnapshot` for `flood`, add a flood counts line). If descoped, state that "Ban N recent joiners" is generic/empty for flood lockdowns.
2. **Carried-over thresholds of 1 or 2.**
   - Recommendation: verbatim copy (D-03) + CHECK 1..1000, and ask the owner whether to clamp to 3 in the migration.
3. **Basic (non-super) groups.** The guard's tracker and the new hooks run in supergroups only, so a basic group never reaches D-14's "not a supergroup" warning.
   - Recommendation: supergroups only for counting; keep `startNotSupergroup` defensive; `/autolockdown` in a basic group answers with the existing `lockdown_basic_group` text.
4. **Where to store trigger numbers.** Recommend three INTEGER columns on `chat_lockdowns` (above); the alternative is a machine-readable string in `reason`, which cannot be localized per destination.
5. **Legacy backup module and dev stats.** `BackupModuleAntiraid` and `LoadAntiRaidStats` keep reading `antiraid_settings`. Recommend: leave the backup module (append-only table, no scope), repoint or drop the dev-stats lines, and say in docs that backups do not carry `/autolockdown` settings. Adding `chat_raid_detection` to backup/import/reset is a separate decision.
6. **Warning destination.** CONTEXT says "Staff Group or log channel". Recommendation: post to both when both exist (like the alert), through `lockdownPaced`, one `SET NX` per group per day gating both.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go | build, tests | yes | go1.26.0 linux/amd64 (default toolchain) | — |
| gcc / cgo | tests (go-sqlite3) | yes | gcc 13.3.0, `CGO_ENABLED=1` | — |
| miniredis | Redis-backed tests | yes (in go.mod) | v2.39.0 | — |
| Redis server | none required (tests use miniredis) | binary present, not needed | — | miniredis |
| PostgreSQL | `make test-postgres-integrity`, migration-chain test | `psql` binary present; **no server must be started in this environment** | — | Run in CI; SQLite for everything else |
| core.telegram.org | Bot API docs | no (egress 403) | — | gotgbot generated docs + existing repo behaviour |

**Missing dependencies with no fallback:** none for planning.
**Missing dependencies with fallback:** PostgreSQL (CI only), Telegram docs (see Assumptions A1).

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go `testing` (+ `testify` assertions only), SQLite via `internal/testdb.Run`, miniredis, hand-written `gotgbot.BotClient` fakes (`lockdownFake`) |
| Config file | none; build tag `testtools`; harness `alita/modules/test_harness_test.go`, `alita/modules/lockdown_fake_test.go` (`newLockdownEnv`) |
| Quick run command | `CGO_ENABLED=1 go test -tags testtools -race -count=1 -run '^TestRaid|^TestAutoLockdown|^TestLockdown' ./alita/modules ./alita/db/raiddetection ./alita/db/lockdown` |
| Full suite command | `make test` (the only valid full run; not run by research) |

### Phase Requirements -> Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| RAID-01 | N distinct self-joins within 60 s lock the group; row has `trigger_kind=join_surge`, `trigger_count>=N`, `StartedBy=bot`; alert trigger line names the surge and count | integration (dispatcher + fake bot + miniredis) | `go test -tags testtools -race -count=1 -run '^TestRaidJoinSurge' ./alita/modules` | no, Wave 0 |
| RAID-01 | Same join delivered as chat_member and service message counts once | integration | `-run '^TestRaidJoinDoubleDelivery$'` | no |
| RAID-01 | Below threshold / outside 60 s window does not lock | integration | `-run '^TestRaidJoinBelowThreshold$'` | no |
| RAID-01 | Lua join script: dedupe, window, 24 h trim, cap (miniredis) | unit | `-run '^TestRaidJoinScript$'` | no |
| RAID-02 | 35 items from 4 recent joiners within 30 s lock the group (`flood`, percent shown); 40 items from established members do not; fewer than 3 accounts do not; ratio just under 60 % does not | integration | `-run '^TestRaidFlood'` | no |
| RAID-02 | Item classification: text/photo/GIF/sticker count; commands, edited, service messages, album parts (once per `media_group_id`) as specified | unit | `-run '^TestRaidFloodItemKey$'` | no |
| RAID-02 | Lua flood script: counts, new-member subset, accounts, window drain | unit | `-run '^TestRaidFloodScript$'` | no |
| RAID-07 | Admin-added joiner, approved joiner, the bot itself, a join request never count toward the surge | integration | `-run '^TestRaidJoinExemptions$'` | no |
| RAID-07 | Admin, approved user, bot, service IDs (777000, 1087968824, 136817688), anonymous-admin post (`sender_chat` = group), linked-channel auto-forward never count toward flood | integration | `-run '^TestRaidFloodExemptions$'` | no |
| RAID-06 | Both triggers on by default for a chat with no row; `/autolockdown` shows state; `joins 20`, `joins off`, `flood on` change it; the next burst follows the new setting on a second "replica" (clear the local layer) | integration + repo | `-run '^TestAutoLockdown'` ; `go test -tags testtools -race -count=1 ./alita/db/raiddetection` | no |
| RAID-06 | Setting `false` persists (GORM zero-value trap); joins write does not clobber flood columns; `DeleteCache` invalidates | repo | `-run '^TestRaidDetectionSettings'` ./alita/db/raiddetection | no |
| RAID-06 | `skipLocal` covers the raid_detection key and not `raid_detectionx` | unit | `go test -tags testtools -race -count=1 -run '^TestSkipLocalMatchesFreshnessCriticalKeys$' ./alita/db/cache` (extend `local_test.go:286`) | extend |
| RAID-06 (D-10) | View needs any live admin; change needs creator or `can_restrict_members`; anonymous admin gets the proof prompt and re-enters; non-admin refused; failed lookup refuses | integration | `-run '^TestAutoLockdownAuthority$'` | no |
| LOCK-10 | Join surge and flood fire together: exactly one `chat_lockdowns` active row, one `setChatPermissions` call, one alert slot set after the worker cycle | integration | `-run '^TestRaidTriggersSingleLockdown$'` | no |
| LOCK-10 | Two goroutines ("replicas") evaluate the same burst concurrently: one lockdown | integration (`-race`) | `-run '^TestRaidTriggersConcurrent$'` | no |
| LOCK-10 | Existing guarantee `uk_chat_lockdowns_active` + `Start` + `BeginAlert` (no new conditional update added) | PG integrity | already in `make test-postgres-integrity`: `TestStartLockdownOneActivePerChat`, `TestBeginAlert` | yes |
| D-14 | Bot not admin / no restrict right: no lockdown, one warning to Staff Group and log channel; second trigger the same day posts nothing (miniredis `FastForward` 24 h then posts again) | integration | `-run '^TestRaidCannotLockWarnsOncePerDay$'` | no |
| D-15 | After `/unlockdown`, a burst within 10 minutes starts nothing; after aging `lift_started_at` by 11 minutes it does | integration | `-run '^TestRaidCooldownAfterLift$'` | no |
| D-13 | Public notice posted once in the locked group by the winner only; alert/status trigger lines per kind in all locales | integration + i18n | `-run '^TestRaidAutoNotice$'` ; `go test -tags testtools -race -count=1 -run '^TestLockdownLocaleKeys$' ./alita/i18n` | no / yes |
| D-04 | Without Redis: counters inert, one log line, `/lockdown` still works | integration | `-run '^TestRaidInertWithoutRedis$'` | no |
| LOCK-15 | `/antiraid`, `/raidtime`, `/raidactiontime` reply with the pointer text; `/autoantiraid 20` sets joins=20 and enabled, `/autoantiraid off` disables | integration | `-run '^TestAntiRaidRetired'` | no |
| LOCK-15 | A join while the old `alita:antiraid:state:<chat>` key exists bans nobody; the startup sweep deletes `alita:antiraid:*` keys | integration | `-run '^TestAntiRaidRetiredNoBan$'`, `-run '^TestAntiRaidKeySweep$'` | no |
| LOCK-15 (D-03) | Migration copies non-zero thresholds only, leaves zero/missing rows to defaults, is idempotent | PG chain / PG integrity | `ALITA_TEST_MIGRATION_CHAIN=true` migration chain + a `TestRaidDetectionCarryOver` added to the `make test-postgres-integrity` `-run` list; CI only (no PostgreSQL here) | no |
| Docs / i18n | Generated docs in sync; no missing literal keys | gate | `make generate-docs && make check-docs`; `make check-translations` | n/a |

### Sampling Rate
- **Per task commit:** the quick run command above (the relevant `-run` subset).
- **Per wave merge:** `CGO_ENABLED=1 go test -tags testtools -race -count=1 ./alita/modules ./alita/db/...`
- **Phase gate:** `make test`, `make lint`, `make check-translations`, `make check-docs` green, plus the CI migration chain and `make test-postgres-integrity` (add `TestRaidDetectionCarryOver` to the Makefile `-run` list, which is `$`-anchored, and `./alita/db/raiddetection` to its package list if the test lives there).

### Wave 0 Gaps
- [ ] `alita/modules/testredis_test.go` - receives `withMiniredis` before `antiraid_miniredis_test.go` is deleted
- [ ] `alita/modules/raid_*_test.go`, `autolockdown_test.go`, `antiraid_retired_test.go` - cover the rows above; add helpers on `lockdownEnv` to send photo/sticker/GIF/album messages with a given `From` and to seed the 24 h joined set at a chosen age
- [ ] `alita/db/raiddetection/{repository_test.go,testmain_test.go}` - `testdb.Run(m, &models.ChatRaidDetection{})`
- [ ] `alita/db/lockdown` test for `RecentlyLiftedFresh` and for the new trigger columns round-tripping through `Start`
- [ ] Add `&models.ChatRaidDetection{}` to the AutoMigrate lists of `alita/modules/test_harness_test.go` and `alita/db/testmain_test.go`
- [ ] Clock seam `raidNow` and a settings lookup seam (like `lockdownActiveLookup`) so tests control time and failures
- [ ] Framework install: none

## Security Domain

`security_enforcement` is enabled (`.planning/config.json`, `security_asvs_level: 1`).

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | no | Telegram identity only |
| V3 Session Management | no | Anonymous-admin proof button is existing machinery |
| V4 Access Control | yes | Setting changes need live creator / `can_restrict_members` (`requireLockdownAuthority(true)`), viewing a live admin; never the admin cache; a failed lookup refuses; trigger-started lockdowns are the bot's own action, and lifting stays admin-only via the Phase 5 buttons |
| V5 Input Validation | yes | `/autolockdown` args parsed with `strconv.Atoi`, bounded; chat and user IDs are int64 and are the only parts of Redis keys; group titles/names HTML-escaped and spliced after translation |
| V6 Cryptography | no | No new secrets or crypto |
| V7 Logging | yes | No tokens or message text in new logs; log chat and user IDs only |
| V11 Business logic | yes | Fixed windows, caps on Redis structures, latch and day-dedupe bound Telegram calls and posts per chat |

### Known Threat Patterns for this stack

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Attacker joins N sock-puppets or floods to lock a victim group (lockdown as DoS) | DoS | Inherent to the feature (owner accepted: a false lockdown costs one `/unlockdown`). Mitigated by approvals and admin exemptions, 3-account floor, tunable thresholds, 10-minute post-lift cooldown, and a lockdown that only an admin can lift |
| Spoofed or replayed settings change | Elevation | Live `getChatMember` at the command, anonymous proof re-entry, no callback data this phase |
| Unbounded Redis growth from a message flood | DoS | ZSET caps (5000 / 2000), 30 s and 24 h trims on every write, PEXPIRE on every key |
| Telegram API amplification (every message past the threshold triggering getChat / getChatMember) | DoS | Fire latch (60 s), "can't lock" backoff, `lockdownPaced` for the notice and warning, one synchronous winner |
| Cross-group effect | Tampering | Every Redis key and every DB query is keyed by the one chat; a lockdown never touches another group (Phase 4 rule) |
| Content injection via names into notices | Tampering | Names and titles escaped (`html.EscapeString` / `lockdownMention`) and spliced after the translator, as the alert does |
| Unauthenticated information leak from the pointer commands | Info disclosure | Pointer replies are static text |

## Sources

### Primary (HIGH confidence, files opened this session)
- `.planning/phases/06-automatic-raid-detection/06-CONTEXT.md`, `.planning/REQUIREMENTS.md`, `.planning/STATE.md`, `.planning/ROADMAP.md` (Phase 6 section), `.planning/config.json`
- `AGENTS.md`, `.claude/CLAUDE.md` (via the session context)
- `alita/modules/lockdown.go` (whole), `lockdown_guard.go` (whole), `lockdown_recent.go` (whole), `lockdown_perms.go`, `lockdown_status.go`, `lockdown_alert.go` (1-200, 330-660), `antiraid.go` (whole), `antiflood.go` (40-300), `users.go` (150-215)
- `alita/db/lockdown/repository.go` (1-520), `burst.go` (1-140), `alerts.go` (AlertPlan, BeginAlert), `alita/db/antiraid/repository.go`, `optimized.go`, `alita/db/models/lockdown.go`, `antiraid.go`
- `alita/db/cache/local.go`, `ttl.go`, `loader.go` (1-115); `alita/utils/cache` (`IsRedisAvailable`); `alita/utils/chat_status` (`IsUserAdmin`, `FetchBotMember`, `update_memo.go`); `alita/utils/actionlog` (`Destination`); `alita/utils/helpers/command_pipeline.go`
- `main.go:120-320`; `migrations/20260518113000_add_antiraid_settings.sql`, `20261006120000_add_chat_lockdowns.sql`, `20261011160000_add_lockdown_ban_run_report.sql`; `alita/db/migrations/runner.go` (392-560), `repository_chain_test.go`
- gotgbot v2 source in the module cache: `ext/dispatcher.go`, `ext/handlers/message.go`, `ext/handlers/filters/message/message.go`, `sender.go`, `gen_types.go` (`ChatMemberUpdated`, `Message` fields)
- Tests read for patterns: `lockdown_fake_test.go`, `lockdown_test.go`, `lockdown_recent_test.go`, `lockdown_isolation_test.go`, `antiraid_miniredis_test.go`, `callback_nil_handlers_test.go`, `alita/i18n/lockdown_locale_test.go`, `alita/modules/test_harness_test.go`, `alita/db/testmain_test.go`, `alita/db/lockdown/testmain_test.go`
- Runtime check: both Lua scripts executed against the repo's miniredis v2.39.0 (temporary program inside the module, removed afterwards; `git status` clean)

### Secondary (MEDIUM confidence)
- gotgbot's generated field documentation of the Bot API (`ChatMemberUpdated.from` "Performer of the action, which resulted in the change"; `Message.sender_chat`, `is_automatic_forward`, `media_group_id`)

### Tertiary (LOW confidence)
- Bot API behaviour recalled from training (see A1); `https://core.telegram.org/bots/api` could not be fetched (egress proxy returned 403, not retried per policy)

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - no new dependencies; all in go.mod and already used.
- Architecture: HIGH - built on code read in full this session; Lua scripts executed against miniredis.
- Pitfalls: HIGH for repo-specific ones (test helper location, `IsBot()` shape, GORM zero values, empty flood burst); MEDIUM for Telegram-behaviour ones.

**Research date:** 2026-10-18
**Valid until:** 2026-11-17 (30 days; the codebase is the moving part, re-read `lockdown.go` and `lockdown_guard.go` if Phase 5 follow-ups land first)
