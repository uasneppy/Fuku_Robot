# Phase 6: Automatic Raid Detection - Context

**Gathered:** 2026-10-18
**Status:** Ready for planning
**Mode:** Smart discuss (autonomous); the owner accepted every recommended answer

<domain>
## Phase Boundary

Raids are detected and locked down automatically, so a group is protected when no admin is online. Two rule-based triggers start the same lockdown as `/lockdown`:
- a join surge;
- a burst of messages, photos, GIFs or stickers, mostly from recently joined members.

Both are on by default in every group with conservative thresholds, and admins can turn each off or tune it. Several triggers firing together give exactly one lockdown and one alert, across replicas. The old `/antiraid` raid mode is retired: its commands point to `/lockdown`, its auto-threshold carries over to join-surge detection, and nobody is temp-banned any more.

Requirements: RAID-01, RAID-02, RAID-06, RAID-07, LOCK-10, LOCK-15.

Out of scope:
- AI spam verdict counters and Gemini image classification (RAID-03/04/05, and RAID-06's AI toggle), which are Phase 7.
- The settings menu, which is Phase 9.
- Deleting flood messages, which is out of scope for v1 (PROJECT.md).

</domain>

<decisions>
## Implementation Decisions

### Join-surge trigger (RAID-01)
- **D-01:** The default threshold is 15 joins within 60 seconds. This is conservative, and uses the same per-minute unit as the old `/autoantiraid N`.
- **D-02:** Only real joins count, each user once even when Telegram delivers the same join as both a `chat_member` update and a service message.
  - Users an admin added or approved never count (RAID-07, and the same performer rule as the Phase 4 guard). Neither do approved users or the bot itself.
  - Join requests do not count, because they don't let anyone in.
- **D-03:** Carry-over (LOCK-15): a group's non-zero `antiraid_settings.auto_antiraid_threshold` becomes its join-surge threshold. A group where it was 0 (off) gets the new default, on at 15, because the owner chose triggers on by default.
- **D-04:** Counting is shared across replicas through Redis, read from the existing 10-minute join tracker (`alita:lockdown:recent:<chat>`, Phase 5) or a key beside it.
  - Without Redis the trigger is inert and logs once; manual `/lockdown` still works.
  - A count is never a ban decision. It only starts a lockdown.

### Message and media flood trigger (RAID-02)
- **D-05:** "Recently joined" means joined this group in the last 24 hours. Each group's joiners and their join times are kept in Redis for 24 hours. The group -7 join handlers write it best effort, like the 10-minute tracker.
- **D-06:** The default threshold is 30 counted items (messages, photos, GIFs or stickers) within 30 seconds. At least 60% of them must come from recently joined members, from at least 3 different recently joined accounts. A busy conversation among established members never trips it.
- **D-07:** These never count (RAID-07): admins, approved users, the bot itself, Telegram service accounts, and posts sent as the linked channel or as the group (anonymous admins). Commands and edited messages don't count either. The cached admin list is acceptable here, because counting grants no authority and a live `getChatMember` per message is too costly.
- **D-08:** The trigger does not delete the flood. The lockdown mutes everyone, which stops it.

### Settings and admin control (RAID-06)
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

### Automatic lockdown behaviour and retiring /antiraid (LOCK-10, LOCK-15)
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

### Amendments after research (owner, 2026-10-18)
- **D-17 (refines D-13, Pitfall 10):** A flood lockdown saves the flooding accounts as its alert burst, so the alert's sample lists them and "Ban N recent joiners" can ban them. These are the recently joined accounts whose items counted toward the trigger, with the usual Phase 5 exclusions. Without this, the burst would hold only the 10-minute join tracker, and flooders who joined hours earlier would be missing.
- **D-18 (refines D-03):** In the migration, a carried-over old threshold below 3 is raised to 3, so no group locks on its first join or two. New input to `/autolockdown` and `/autoantiraid` must be between 3 and 500, and the table has a CHECK for the stored range.
- **D-19 (refines D-14):** The "raid detected but I can't lock" warning goes to both the Staff Group and the log channel when both exist, like the alert. One daily limit per group covers both.
- **D-20 (scope):** Backups (export, import, reset) do not carry the new `chat_raid_detection` settings in this phase, and the docs say so. The legacy backup module keeps reading `antiraid_settings` unchanged. The developer `/stats` lines are repointed to the new table.
- **D-21 (Claude's call on research recommendations):**
  - The trigger numbers (count, window, percent) are stored as columns on `chat_lockdowns`, not inside `reason`, so each destination's alert can render them in its own language.
  - Counting runs in supergroups only. `/autolockdown` in a basic group answers with the existing basic-group text.
  - `/autolockdown joins N` or `flood N` sets the threshold and also turns that trigger on.

- **D-22 (refines D-03/D-18, planner's call, reported to the owner 2026-10-18):** The carry-over clamps the old threshold into the stored range in both directions: `LEAST(GREATEST(auto_antiraid_threshold, 3), 500)`. A threshold above 500 becomes 500, because the table's CHECK (3 to 500) would otherwise reject the row and abort startup. The capped group is more sensitive than before, which is accepted as safer than failing to start.

### Claude's Discretion
- Handler group placement of the flood counter. It must see messages before antiflood (group 4) ends them, and return `ext.ContinueGroups`.
- The exact Redis key layout and atomic counting (a Lua script or a pipelined ZADD/ZCOUNT) of both counters.
- How the trigger's lock call shares code with `/lockdown`: one shared start-lockdown function used by both.
- The wording of the public notice, the warning and the `/autolockdown` replies, in all 7 locales.
- Whether the old Redis raid keys are cleared by a startup sweep or lazily.

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets
- **Lockdown machinery.**
  - `alita/modules/lockdown.go`: the `lockdown` command does the snapshot (`getChat`), the lock (`setChatPermissions`), `lockdown.Start` and the settle path.
  - `alita/modules/lockdown_guard.go`: the group -7 join handlers.
  - `alita/modules/lockdown_worker.go`: the worker.
  - `alita/modules/lockdown_alert*.go`: the alert, which already shows `trigger_kind` and counts.
- **One active lockdown per chat:** `uk_chat_lockdowns_active` plus `lockdown.Start` (`alita/db/lockdown/repository.go`) already give LOCK-10 across replicas.
- **The 10-minute join tracker:** `alita/modules/lockdown_recent.go`, `alita:lockdown:recent:<chat>`, a sorted set scored by join time and deduplicated by user ID at read.
- **Old antiraid:** `alita/modules/antiraid.go` (962 lines) holds the commands, `onJoin` at group -5, the expiry poller and the Redis keys `alita:antiraid:state` and `alita:antiraid:joins`. `alita/db/antiraid` holds the settings repository, and `models.AntiRaidSettings` has `AutoAntiRaidThreshold` (joins per minute, 0 = off).
- **Authority:** `requireLockdownAuthority` (live `getChatMember`), `chat_status.IsUserAdmin` (cached), `approvals.IsUserApproved` (cached).
- **Log destination:** `actionlog.Destination` and `staff.GetLinkOfGroupFresh`.

### Established Patterns
- Watchers return `ext.ContinueGroups`; commands use `helpers.WrapCommand`; anonymous admin re-entry uses `RegisterAnonymousAdminHandler`.
- Operational Redis keys sit outside `alita:cache:`. Best-effort Redis writes on the join path stay under 250 ms and their results are ignored.
- Exactly-once side effects come from conditional database writes; only the winner posts.
- Migrations are append-only and timestamped; every new model goes into the test-harness AutoMigrate lists.

### Integration Points
- The group -7 join handlers record a join (feeding the surge counter and the 24-hour joiner set).
- A new message watcher counts flood items.
- A shared "start lockdown" function is called from `/lockdown` and from the triggers.
- The `antiraid` module is replaced: old commands point to the new ones, `/autoantiraid` becomes an alias, and `onJoin` and the poller are removed.
- Phase 7 adds an AI-verdict trigger on the same counters and settings table.

</code_context>

<specifics>
## Specific Ideas

- The alert's trigger line should read like "Join surge: 23 joins in 60 s" or "Flood: 41 messages in 30 s, 87% from new members".
- Default thresholds are deliberately conservative (owner decision: on by default, conservative). A false lockdown costs an admin one `/unlockdown`; a missed raid costs much more.
- Antiraid's own join handler already stands aside during a lockdown (Phase 4). This phase removes it entirely.

</specifics>

<deferred>
## Deferred Ideas

- A persistent PostgreSQL join log so the triggers work without Redis. Declined in Phase 5 for its per-join cost and declined again here (D-04).
- Deleting the flood messages when the flood trigger fires (D-08).
- Per-trigger windows that admins can configure, not just thresholds. The thresholds are tunable and the windows stay fixed for v1.

</deferred>
