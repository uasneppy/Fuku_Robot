---
phase: 06-automatic-raid-detection
reviewed: 2026-10-18T15:00:00Z
depth: standard
files_reviewed: 59
findings:
  critical: 0
  warning: 3
  info: 5
  total: 8
status: issues_found
---

# Phase 6: Code Review Report

**Reviewed:** 2026-10-18
**Depth:** standard
**Files Reviewed:** 59. The list is in `/tmp/.../scratchpad/p6review.txt`: code, locales and migrations changed since `f3433a18^`. Docs pages are excluded.
**Status:** issues_found

## Summary

The review covered all Phase 6 production code against `f3433a18^`:
- the two migrations, the model and repository, and the Lua counters;
- the flood and join triggers, `raidFire` and `startLockdownRow`;
- `/autolockdown` and `/autoantiraid`;
- the antiraid retirement;
- the alert changes and the locales.

Test files were checked only for reliability. The seams are restored with `t.Cleanup`. `make check-translations` and `make check-docs` are clean, and all 26 new or changed locale keys match `en.yml` in placeholders and HTML.

There are no blockers. The areas checked hold up:
- **Exactly one lockdown and one alert.**
  - `lockdown.Start` uses `ON CONFLICT DO NOTHING` behind `uk_chat_lockdowns_active`, and a loser makes no lock call.
  - `BeginAlert` is unchanged.
  - The fire latch is a `SET NX` that only sheds load. Both triggers share one latch key.
  - The snapshot is read before `Start`.
- **Lua scripts.** All keys share the `{chat}` hash tag. The user ID is the member, so join dedupe is correct. The 24 h trim, caps and TTLs are correct, and the millisecond values stay within Lua's number-to-string precision.
- **Flood rule.**
  - The maths matches D-06.
  - Edits, channel posts and bot senders never reach the handler.
  - `sender_chat`, automatic forwards and the service IDs are rejected before Redis is used.
  - An album counts once.
- **`startLockdownRow`.** `/lockdown` maps every outcome to the reply it always sent.
- **`/autolockdown` and `/autoantiraid`.**
  - Authority is a live check, with anonymous-admin re-entry.
  - Writes are column-targeted upserts with no GORM `default:` on the booleans, each followed by `DeleteCache`.
  - The settings key is in `skipLocal`.
- **Migrations.** They are append-only plain DDL. The carry-over is idempotent through `ON CONFLICT DO NOTHING`.
- **Retirement.** No temp-ban path remains. The sweep only touches `alita:antiraid:*`, using SCAN and UNLINK in batches, in a goroutine with panic recovery and a 30 s bound. The shutdown handler was removed cleanly.

## Warnings

### WR-01: Any slash-prefixed text is exempt from flood counting, so a raider can evade the flood trigger

**File:** `alita/modules/raid_flood.go:129` (`raidFloodItemKey`), `:145-157` (`raidFloodIsCommand`)

**Issue:** A text is treated as a command, and so not counted, in either of two cases:
- its first entity is a `bot_command` at offset 0;
- it starts with `/` followed by a non-space character.

Telegram turns any `/word` into a command entity. A flood of `"/x buy crypto ..."` or `"/////"` from 30 fresh accounts is never counted, so the trigger never fires. D-07's intent is that the bot's own commands do not count. That does not cover every slash-prefixed text.

**Fix:** Exempt only commands the bot actually handles: a registered command name, either bare or addressed `@<this bot>`. A slash text that the bot would not dispatch, including `@otherbot` commands, counts as an item.

### WR-02: The once-a-day "can't lock" warning gate is used up before anything is delivered and is never given back

**File:** `alita/modules/raid_trigger.go:323-365` (`raidWarnCannotLock`)

**Issue:** `SET NX alita:raid:{chat}:warn EX 86400` is taken before the destinations are resolved and before any send. Two cases burn the gate:
- a group with neither destination;
- a group where both posts fail.

The warning is then lost for 23 hours, and it is the only signal that a raid went unanswered.

**Fix:** Take the gate only when at least one destination exists. If no copy was delivered, give it back with `DEL`. When at least one copy was delivered, keep it set, so that the two copies still count as one warning a day.

### WR-03: The public notice and warning posts block the join guard and the message pipeline for up to 30 s per post

**File:** `alita/modules/raid_trigger.go:207-212`, called from `raid_join.go:215`, `:267` and `raid_flood.go:276`

**Issue:** `raidPostNotice` and `raidWarnCannotLock` run inside the synchronous `raidFire`. Each post runs under `raidPostTimeout` (30 s) and waits on `lockdownPaced`. This has two effects:
- The joiner who tips the threshold is not recorded by the guard until the send returns. On the warning path, which makes two sends, that can be about 60 s.
- On the flood path, the update that wins the latch holds back aispam, antiflood, locks, blacklists and filters for the same time.

**Fix:** Send the notice and the warning from a goroutine with panic recovery and a bounded context. The lock itself stays synchronous.

## Info

### IN-01: Dead production code left in `alita/db/antiraid`

**File:** `alita/db/antiraid/repository.go:47-91`, `optimized.go:41-45`

**Issue:** After the retirement, the setters, `LoadAntiRaidStats` and the cached getters are called only from tests.

**Fix:** Remove what nothing uses. Keep only what backup needs.

### IN-02: The settings cache module name is a literal in three places

**File:** `alita/db/raiddetection/repository.go:105`, `alita/db/cache/local.go:47`

**Fix:** Use `cacheModule` in `upsertSettings`. Add a test that `skipLocal` is true for `cache.CacheKey("raid_detection", 1)`.

### IN-03: `lockdown_help_msg` states the default thresholds as if they applied to every group

**File:** `locales/*.yml` (`lockdown_help_msg`)

**Fix:** Say "by default 15 … / 30 …, see `/autolockdown` for this group's numbers" in all 7 locales.

### IN-04: The settings and Redis preamble is duplicated in the two join counters

**File:** `alita/modules/raid_join.go:188-201`, `:230-243`

**Fix:** Extract one helper, for example `raidJoinSettings(chat)`, and call it from both counters.

### IN-05: `/autolockdown` reads settings with an unbounded `context.Background()`

**File:** `alita/modules/autolockdown.go:210`, `:230`

**Fix:** Use `context.WithTimeout(..., raidSettingsTimeout)`.

---

_Reviewed: 2026-10-18_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
