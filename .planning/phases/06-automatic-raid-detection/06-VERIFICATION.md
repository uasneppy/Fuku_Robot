---
phase: 06-automatic-raid-detection
verified: 2026-10-29T16:00:00Z
status: human_needed
score: 5/5 must-haves verified
covered_files:
  - alita/modules/raid_flood.go
  - alita/modules/raid_join.go
  - alita/modules/raid_counters.go
  - alita/modules/raid_trigger.go
  - alita/modules/antiraid.go
  - alita/modules/autolockdown.go
  - alita/modules/lockdown.go
  - alita/modules/lockdown_guard.go
  - alita/modules/lockdown_start.go
  - alita/db/raiddetection/repository.go
  - alita/db/models/raid_detection.go
  - migrations/20261018130000_add_chat_raid_detection.sql
  - .planning/phases/06-automatic-raid-detection/06-01-PLAN.md
  - .planning/phases/06-automatic-raid-detection/06-01-SUMMARY.md
  - .planning/phases/06-automatic-raid-detection/06-02-PLAN.md
  - .planning/phases/06-automatic-raid-detection/06-02-SUMMARY.md
  - .planning/phases/06-automatic-raid-detection/06-03-PLAN.md
  - .planning/phases/06-automatic-raid-detection/06-03-SUMMARY.md
  - .planning/phases/06-automatic-raid-detection/06-04-PLAN.md
  - .planning/phases/06-automatic-raid-detection/06-04-SUMMARY.md
  - .planning/phases/06-automatic-raid-detection/06-05-PLAN.md
  - .planning/phases/06-automatic-raid-detection/06-05-SUMMARY.md
  - .planning/phases/06-automatic-raid-detection/06-06-PLAN.md
  - .planning/phases/06-automatic-raid-detection/06-06-SUMMARY.md
  - .planning/phases/06-automatic-raid-detection/06-07-PLAN.md
  - .planning/phases/06-automatic-raid-detection/06-07-SUMMARY.md
  - .planning/phases/06-automatic-raid-detection/06-08-PLAN.md
  - .planning/phases/06-automatic-raid-detection/06-08-SUMMARY.md
  - .planning/phases/06-automatic-raid-detection/06-09-PLAN.md
  - .planning/phases/06-automatic-raid-detection/06-09-SUMMARY.md
  - .planning/phases/06-automatic-raid-detection/06-10-PLAN.md
  - .planning/phases/06-automatic-raid-detection/06-10-SUMMARY.md
  - .planning/phases/06-automatic-raid-detection/06-11-PLAN.md
  - .planning/phases/06-automatic-raid-detection/06-11-SUMMARY.md
  - .planning/phases/06-automatic-raid-detection/06-12-PLAN.md
  - .planning/phases/06-automatic-raid-detection/06-12-SUMMARY.md
  - .planning/phases/06-automatic-raid-detection/06-13-PLAN.md
  - .planning/phases/06-automatic-raid-detection/06-13-SUMMARY.md
covered_digest: "v2:sha256:dc83ccc1f428c2a18a8e9bfd264f2c8878fb5d29883337ab397e4b0195c597bc"
behavior_unverified: 0
overrides_applied: 0
human_verification:
  - test: "Join a real surge into a test supergroup"
    expected: "Add 15+ fresh accounts (or whatever threshold /autolockdown shows) within 60 s with Redis running. The group locks on its own, one public notice appears in the group, and the Staff Group and log channel alert names the join surge and its count. Accounts an admin adds or approves do not count toward it."
    why_human: "Needs a live Telegram bot, a supergroup and many real accounts. The tests drive a real dispatcher with a fake Bot API client and miniredis, not Telegram."
  - test: "Flood a real group from recently joined accounts"
    expected: "With 3+ accounts that joined in the last 24 h, send 30+ texts, photos, GIFs or stickers within 30 s. The group locks and the alert shows 'Flood: N messages in 30 s, P% from new members'. The same volume from established members does not lock. Admins and approved users sending the same volume never count."
    why_human: "Needs real accounts with real join times and real Telegram update delivery."
  - test: "Two or more replicas race the same raid"
    expected: "With two bot replicas on one PostgreSQL and one Redis, a join surge and a flood together give exactly one chat_lockdowns active row and one alert per destination."
    why_human: "Needs two live replicas. The single-row guarantee rests on uk_chat_lockdowns_active, lockdown.Start's ON CONFLICT DO NOTHING and BeginAlert, which run against PostgreSQL in CI (make test-postgres-integrity), not here."
  - test: "/autolockdown in a real group"
    expected: "Alone it shows both triggers with thresholds for any live creator or admin. 'joins 20', 'flood off', 'flood on' by a creator or an admin with can_restrict_members save, and the next burst follows the new setting on every replica. An admin without can_restrict_members is refused for a change. An anonymous admin gets the proof button first. A number outside 3..500 is refused."
    why_human: "Needs a real group, real admin roles and anonymous-admin mode."
  - test: "Retired /antiraid in a real group"
    expected: "/antiraid, /raidtime and /raidactiontime each reply with the pointer to /lockdown and /autolockdown and change nothing. An old antiraid| button answers the same text. /autoantiraid N sets the join-surge threshold. Nobody is temp-banned during a join surge. Any alita:antiraid:* key left in Redis is gone after a restart."
    why_human: "Needs a real group and Redis; the no-ban property is also proved by TestAntiRaidRetiredNoBan against a fake client."
  - test: "Migration carry-over on PostgreSQL 16"
    expected: "Run TestRaidDetectionCarryOver and TestRaidDetectionSettings in CI (make test-postgres-integrity). A group with auto_antiraid_threshold = 40 gets join_threshold 40, one with 1 gets 3, one with 9000 gets 500, one with 0 gets no row and so the defaults. The CHECK 3..500 rejects out-of-range writes."
    why_human: "No PostgreSQL here. TestRaidDetectionCarryOver SKIPs on SQLite, which cannot run the clamp or the CHECK."
  - test: "Native-speaker check of the six non-English translations"
    expected: "es, fr, hi, id, pt and ru read naturally for the new lockdown_auto_*, lockdown_trigger_*, lockdown_antiraid_retired keys and the updated lockdown_help_msg. pt's help message is 4096 characters counting its HTML tags, so it is near Telegram's 4096 limit; check that /lockdown help still sends in pt."
    why_human: "Translation quality and a real send cannot be judged by a script. Placeholders and HTML are checked by make check-translations."
---

# Phase 6: Automatic Raid Detection Verification Report

**Phase Goal:** As a group admin, I want to have raids detected and locked down automatically, so that my group is protected when no admin is online.
**Verified:** 2026-10-29
**Status:** human_needed
**Re-verification:** No, initial verification

All five roadmap success criteria hold in the code. Nothing is FAILED. The status is `human_needed` only because live Telegram, two replicas and PostgreSQL are outside this environment.

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | A burst of joins in a short window locks the group automatically; the alert names the join surge and its count | VERIFIED | `raidCountMemberJoin` and `raidCountMessageJoins` (`raid_join.go`) are called from both group `-7` join guards (`lockdown_guard.go:167, 239`) before the lockdown read. `recordRaidJoins` runs the `raidJoinScript` Lua (ZADD by user ID, 24 h trim, cap, ZCOUNT over 60 s). `raidRecordAndFire` calls `raidFire` when `count >= JoinThreshold`. `raidFire` goes through the same `startLockdownRow` as `/lockdown`, with the bot as starter and `trigger_kind` `join_surge` plus count and window columns. `lockdownTriggerSentence`, the alert trigger line and `raidPostNotice` render the kind and count. `TestRaidTriggersSingleLockdown` and the join-surge tests pass. |
| 2 | A burst of messages, photos, GIFs or stickers, mostly from recently joined members, locks the group; a busy conversation among established members does not; admins and approved users never count | VERIFIED | `raidFloodCountUpdate` counts text, photo, GIF (animation before document), sticker and other media. `raidFloodScript` adds an item to `fi`, and to `fn` and `fa` only when `ZSCORE joined` is within 24 h. `raidFloodCounts` needs items >= threshold, new*100 >= 60*items and accounts >= 3. Exemptions: `sender_chat`, automatic forwards, service IDs, the bot, `IsApprovedForUpdate`, `userIsFloodExempt`. `TestRaidFlood`, `TestRaidFloodExemptions`, `TestRaidFloodRule` and `TestRaidFloodBurst` pass. |
| 3 | Both triggers are on by default with conservative thresholds; an admin can turn each off or change its threshold; the next burst follows the new setting | VERIFIED | `raiddetection.Defaults` is on/15 and on/30; a missing row reads as the defaults; the migration column defaults match. `/autolockdown` parses `joins\|flood` then `on\|off\|N`, with 3..500 enforced in the parser, the repository and a DB `CHECK`. Authority is `requireAutoLockdownAuthority` (live `getChatMember`, view for any admin, change needs `can_restrict_members`), with anonymous re-entry registered. Writes are column-targeted upserts followed by `cache.DeleteCache`; the key is in `skipLocal` (`alita/db/cache/local.go:47`). Every counter reads settings per update through `raidSettingsLookup`. `TestRaidDetectionSettings` passes on SQLite; the `CHECK` half is a CI PostgreSQL test. |
| 4 | Several triggers at once give exactly one lockdown and one alert, also across replicas | VERIFIED (in-process); cross-replica routed to human/CI | Both triggers share one path: `raidFire` with the `SET NX` latch, an active-lockdown read, a recent-lift quiet period, then `startLockdownRow`, which relies on `lockdown.Start` `ON CONFLICT DO NOTHING` behind `uk_chat_lockdowns_active`; the alert is decided by the unchanged conditional `BeginAlert`. The notice is posted only for `lockdownStartStarted`. `TestRaidTriggersConcurrent` (parallel join and flood triggers) and `TestRaidTriggersSingleLockdown` pass. The PostgreSQL index guarantees (`TestStartLockdownOneActivePerChat`, `TestBeginAlert`) are in the CI list. |
| 5 | `/antiraid` and the old timing commands point to `/lockdown`; the old auto-threshold becomes the join-surge threshold; the old raid mode no longer temp-bans | VERIFIED | `antiraid.go` is 180 lines: it registers `/antiraid`, `/raidtime`, `/raidactiontime` as one pointer reply, `/autoantiraid` as an alias that prepends `joins` to `autolockdownApply`, an old-button answer, and the key sweep. There is no join handler at group `-5`, no ban call and no poller. A grep for `antiraid` in non-test Go code outside `antiraid.go` finds only the backup module's direct table access, the model and a comment. `alita/db/antiraid` is deleted. Migration `20261018130000` copies `LEAST(GREATEST(auto_antiraid_threshold,3),500)` for every row with a non-zero value. `TestAntiRaidRetired`, `TestAntiRaidRetiredNoBan` and `TestAntiRaidSweepRunsOnceOnLoad` pass. |

**Score:** 5/5 truths verified, 0 present but behavior-unverified. Live and PostgreSQL behavior is listed under human verification, not counted as a gap.

### Specific Checks Requested

**Slash-prefixed texts that no command handles (late group-12 watcher): COUNTED.**
- `raidFloodItemKey` marks a text as `slash` when its first entity is a `bot_command` at offset 0, or it starts with `/` and a non-space character. The group `-4` watcher counts only `slash == false`; the group-12 watcher (`LoadLockdown`, `raid_flood.go:54`) counts only `slash == true`. Both call `raidFloodCountUpdate`, so classification, exemptions, script and trigger are one path.
- I read gotgbot's `iterateOverHandlerGroups`: `ext.EndGroups` stops every later group, a handler returning `nil` moves to the next group. A handled command therefore never reaches group 12, and `/word` with no handler, `/////` and `/start@otherbot` do.
- No other handler is registered at group 12 or above.
- An AST scan of every `(b, ctx) error` function in `alita/modules` found no direct `return nil`. An erroring command handler still reaches group 12 (the dispatcher's error handler maps errors to a no-op), which only raises counts and is documented in AGENTS.md.
- `TestRaidFloodSlashTexts` drives a real `ext.Dispatcher` through `ProcessUpdate`. 30 unhandled slash texts lock; 40 handled commands (also `@thisbot`) count nothing; `@otherbot` counts; admins' slash texts never count.
- Known limit (documented in AGENTS.md): a slash text that an earlier group ends (aispam, antiflood, locks, blacklists deleting it) is not counted late.

**Can the old raid mode still temp-ban anyone: NO.**
- No handler at group `-5`, no expiry poller, no `BanChatMember` or `alita:antiraid:state` read or write in `alita/modules/antiraid.go`. The only `alita:antiraid:*` reference in non-test code is the sweep's `SCAN`/`UNLINK` pattern.
- The `.claude/worktrees/` copies of the old file are executor leftovers outside the build.
- Rolling-deploy caveat (documented): an old replica can still temp-ban until it stops.

### Required Artifacts

| Artifact | Status | Details |
|----------|--------|---------|
| `alita/modules/raid_counters.go`, `raid_join.go`, `raid_flood.go`, `raid_trigger.go` | VERIFIED | Substantive, wired from the join guards and `LoadLockdown`. |
| `alita/modules/autolockdown.go` | VERIFIED | `WrapCommand` plus `RegisterAnonymousAdminHandler`. |
| `alita/modules/antiraid.go` | VERIFIED | Retired as described. |
| `alita/modules/lockdown_start.go` | VERIFIED | The one start path, used by `/lockdown` and `raidFire`. |
| `alita/db/raiddetection/repository.go`, `alita/db/models/raid_detection.go` | VERIFIED | Cached reads, upserts with `DeleteCache`, no GORM `default:` tag on the booleans. |
| `migrations/20261018130000_add_chat_raid_detection.sql` | VERIFIED | Append-only, `CHECK` 3..500, clamped carry-over, idempotent. |
| `RegisterRaidPostsDrain` in `main.go:221` | VERIFIED | Registered after DB-close. |
| Locale keys | VERIFIED | `make check-translations` clean; sampled keys present in 7 of 7 files. |

### Key Link Verification

| From | To | Status |
|------|----|--------|
| Group `-7` join guards | `raidCountMemberJoin` / `raidCountMessageJoins` | WIRED (`lockdown_guard.go:167, 239`) |
| `LoadLockdown` | flood watchers at `-4` and `12` | WIRED (`lockdown.go:557, 560`) |
| `raidRecordAndFire` / `raidFloodCountUpdate` | `raidFire` | WIRED |
| `raidFire` | `startLockdownRow` | WIRED, same function `/lockdown` calls |
| `/autolockdown` writes | `raiddetection.Set*` then `cache.DeleteCache` | WIRED |
| Settings reads | `skipLocal` prefix `raid_detection:` | WIRED |
| `LoadAntiRaid` | pointer commands, alias and sweep | WIRED |

### Data-Flow Trace (Level 4)

Join and flood counts come from Redis Lua scripts keyed `alita:raid:{chat}:*`, settings from `chat_raid_detection` via the cache, and the trigger numbers are stored in `chat_lockdowns` columns and rendered by the alert. No static or hardcoded source found. FLOWING.

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| Raid, autolockdown, antiraid and flood tests | `go test -tags testtools -race -count=1 -run 'Raid\|AutoLockdown\|Autolockdown\|AntiRaid\|Antiraid\|Flood\|JoinSurge\|Retired' ./alita/modules` | ok, 40 top-level PASS, 0 FAIL | PASS |
| Settings, migrations, cache and devs packages | `go test ... ./alita/db/raiddetection ./alita/db/migrations ./alita/db/cache ./alita/db/devs` | ok for all four | PASS |
| Carry-over clamp | `TestRaidDetectionCarryOver` | SKIP on SQLite | needs PostgreSQL (human) |
| Translations and docs | `make check-translations`, `make check-docs` | clean, no drift | PASS |

### Probe Execution

No probes declared for this phase. SKIPPED.

### Requirements Coverage

| Requirement | Status | Evidence |
|-------------|--------|----------|
| RAID-01 | SATISFIED | Truth 1 |
| RAID-02 | SATISFIED | Truth 2 |
| RAID-06 | SATISFIED | Truth 3 |
| RAID-07 | SATISFIED | `raidJoinCounts` (admin or anonymous-admin performer, approved, bot, requests) and the flood exemptions in Truth 2 |
| LOCK-10 | SATISFIED in code; PostgreSQL index proof in CI | Truth 4 |
| LOCK-15 | SATISFIED in code; carry-over proof in CI | Truth 5 |

No orphaned requirements: all six IDs are claimed by plans 06-01 to 06-13.

### Anti-Patterns Found

None. No TBD, FIXME, XXX, TODO or HACK in the Phase 6 production files. The 8 review findings (3 warnings, 5 info) are marked fixed in `06-REVIEW-FIX.md`, and I confirmed WR-01 (late watcher), WR-02 (warning gate released when nothing was delivered) and WR-03 (`raidPostAsync` with panic recovery and a drain) in the code.

### Gaps Summary

No gaps. Remaining items are live-system checks (see `human_verification`). Notes for the owner:
- The French `lockdown_help_msg` is now 4021 characters raw; the last commit trimmed it under the limit. pt is exactly 4096 raw including HTML tags, which Telegram does not count, so it should send; confirm with a real `/lockdown help` in pt.
- The cross-replica "one lockdown, one alert" guarantee and the clamped carry-over rest on PostgreSQL tests that did not run here.

---

_Verified: 2026-10-29_
_Verifier: Claude (gsd-verifier)_
