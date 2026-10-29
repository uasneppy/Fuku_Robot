---
phase: 06-automatic-raid-detection
fixed_at: 2026-10-29T15:20:00Z
review_path: .planning/phases/06-automatic-raid-detection/06-REVIEW.md
iteration: 1
findings_in_scope: 8
fixed: 8
skipped: 0
status: all_fixed
---

# Phase 6: Code Review Fix Report

**Fixed at:** 2026-10-29
**Source review:** `.planning/phases/06-automatic-raid-detection/06-REVIEW.md`
**Iteration:** 1

**Summary:**
- Findings in scope: 8 (3 warnings, 5 info)
- Fixed: 8
- Skipped: 0

Verification ran in the main checkout (no worktree, as instructed). `CGO_ENABLED=0 go build ./...` and
`go vet -tags testtools ./...` are clean. `CGO_ENABLED=1 go test -tags testtools -race -count=1 ./alita/modules ./alita/db/... .`
passes. `make check-translations` and `make check-docs` are clean. `make lint` and `make test` were not run (as instructed).

## Fixed Issues

### WR-01: Any slash-prefixed text is exempt from flood counting

**Files modified:** `alita/modules/raid_flood.go`, `alita/modules/lockdown.go`, `alita/modules/raid_flood_test.go`, `AGENTS.md`,
`.planning/phases/06-automatic-raid-detection/06-WR01-wip.patch` (deleted, not applied)
**Commit:** 9b150e7
**Applied fix:** Uses gotgbot's dispatch order, no reflect, no unsafe, no second list of command names. A handled command
returns `ext.EndGroups`, which ends every later group (confirmed in `ext/dispatcher.go` `iterateOverHandlerGroups`). The
group `-4` watcher now counts everything except a slash-prefixed text, and a new watcher in group `12` (unused by any
handler; `raidFloodLateGroup`) counts the slash-prefixed texts that reach it. Both call one function,
`raidFloodCountUpdate(b, ctx, late)`, so classification, exemptions, Lua script and `raidFire` are a single code path.
`raidFloodItemKey` now returns `(key, slash, ok)`. Regression tests added: 30 `"/x spam"` items lock; 30
`/lockdownstatus` items do not (and count nothing); `/lockdownstatus@AlitaTestBot` does not count; `/start@otherbot` and
`/////` count; 15 plain texts at `-4` plus 15 slash texts late add up to one total of 30; an admin's slash texts never
count late. AGENTS.md handler-group list and flood bullet state the rule and the late group.

Two existing flood tests had to change, because they encoded the old behaviour: the exemption case "commands" sent
`/cmd now`, which no handler takes and which now correctly counts, so it sends `/lockdownstatus` (a handled command);
and the `raidFloodItemKey` table moved the old "command" rows from "ignored" to a new "left to the late watcher" table.
All other lockdown tests pass unchanged.

**Audit of EndGroups on every path (reported as asked):**
- `WrapCommand` returns `ext.EndGroups` on a failed `BuildCommandContext` and on every failed `RequiredChecks`
  (including refusals); on success it returns whatever the handler returns.
- A static scan (go/ast) of all ~200 functions in `alita/modules` with a command signature found no direct `return nil`.
  Many return `err` after a failed Telegram call or DB write. `main.go`'s `dispatcherErrorHandler` maps every non-EndGroups
  error to `DispatcherActionNoop`, which moves on to the next group. So a handled command that fails with an error, and a
  handler that panics (recovered, returns nil), still reaches group 12 and its text is counted late. This only raises
  counts, and needs 30 such items from fresh accounts within 30 s to matter. It is documented in AGENTS.md.
- A slash text that an earlier group ends (aispam `3`, antiflood `4`, locks `5`/`6`, blacklists `7` deleting it) is also
  not counted late. Documented in AGENTS.md. The raw StaffActions interceptors return `ext.ContinueGroups` outside a Staff
  Group by design, so the real command handler then runs and ends the update.

### WR-02: The once-a-day "can't lock" warning gate is burned before anything is delivered

**Files modified:** `alita/modules/raid_trigger.go`, `alita/modules/raid_notice_test.go`, `AGENTS.md`
**Commit:** 0dbcb7e
**Applied fix:** `raidWarnCannotLock` resolves the Staff Group link and the log destination first and returns without
touching the gate when there is none. Then it takes `SET NX`, sends the copies and counts deliveries; with none
delivered it gives the gate back with `DEL` (`raidReleaseWarnGate`, best effort, logged on error). One delivered copy
keeps the gate. Tests: no destination leaves the gate free; a Staff Group linked afterwards is warned the same day; both
sends failing releases the gate and the next trigger warns both; one delivered copy keeps the gate (nothing more is sent).

### WR-03: The public notice and warning posts block the join guard and the message pipeline

**Files modified:** `alita/modules/raid_trigger.go`, `alita/modules/shutdown_drains.go`, `main.go`,
`alita/modules/raid_post_async_test.go` (new), `alita/modules/shutdown_drains_test.go`, `alita/modules/lockdown_fake_test.go`,
`alita/modules/lockdown_alert_test.go`, `alita/modules/staff_action_fake_test.go`, `alita/modules/test_harness_test.go`, `AGENTS.md`
**Commit:** b05412b
**Applied fix:** The lock stays synchronous. `raidPostNotice` and `raidWarnCannotLock` now run through `raidPostAsync`, a
goroutine that starts with `defer error_handling.RecoverFromPanic(...)` and is counted in `raidPosts`; each send keeps
its `raidPostTimeout` (30 s) bound. `raidPosts` is a small counter, not a `sync.WaitGroup`: `go test -race` flagged
a WaitGroup that is added to while a timed-out drain waiter is abandoned. Shutdown: `StopRaidPosts` waits at most
`raidPostStopWait` (2 s), registered after DB-close by `RegisterRaidPostsDrain` in `main.go`; posts still running are cut
off with the process (best effort, stated in AGENTS.md, and the webhook-mode budget line was updated to include the 3 s).
Tests are deterministic with no sleeps: the fake's read helpers (`sentTo`, `sentChats`, `callsFor`) and `newLockdownEnv`'s
cleanup wait on `raidPosts`. New tests use a gated bot client to show that the 15th join is recorded as a joiner row
and the 30th flood message returns while the notice send is blocked, that the warning is also asynchronous, that
`StopRaidPosts` times out while a send is held and reports done after, that a panicking post is recovered and counted
done, and that a real `shutdown.Manager` run delivers the notice before the DB-close probe.

### IN-01: Dead production code left in `alita/db/antiraid`

**Files modified:** `alita/db/antiraid/*` (package deleted), `alita/db/backup/backup_test.go`, `alita/db/cache/ttl.go`, `AGENTS.md`
**Commit:** 929c2d2
**Applied fix:** No production code called anything in the package (grep confirmed; the backup module reads and resets
`antiraid_settings` through `models.AntiRaidSettings` directly), so the whole package and its tests were removed,
together with the unused `CacheTTLAntiRaid` constant. The one remaining caller was `backup_test.go`, which now writes
its row with `db.DB.Create` and reads it back directly. The table and model stay. AGENTS.md no longer says the package stays.

### IN-02: The settings cache module name is a literal in three places

**Files modified:** `alita/db/raiddetection/repository.go`, `alita/db/cache/local_test.go`
**Commit:** 2a8b610
**Applied fix:** `upsertSettings` uses `cacheModule`. Added `TestSkipLocalRaidDetectionSettingsKey` (`skipLocal` is true for
`CacheKey("raid_detection", 1)` and a real chat ID). The `skipLocal` prefix in `cache/local.go` stays a literal, because
the cache package cannot import `raiddetection`.

### IN-03: `lockdown_help_msg` states the default thresholds as if they applied to every group

**Files modified:** `locales/{en,es,fr,hi,id,pt,ru}.yml`, `docs/src/content/docs/commands/lockdown/index.md` (regenerated)
**Commit:** de1d417
**Applied fix:** All 7 locales now say the numbers are the defaults, that `/autolockdown` shows this group's numbers, and
that commands the bot handles do not count toward a flood. Placeholders and HTML match across locales (6 `<code>` pairs
each). `make generate-docs`, `make check-docs` and `make check-translations` are clean. Help text is sent as one message,
so lengths were checked against Telegram's 4096: es, pt and fr were trimmed elsewhere in the same paragraph so es and pt
stay under it (about 4052 and 4065 with the header).

### IN-04: The settings and Redis preamble is duplicated in the two join counters

**Files modified:** `alita/modules/raid_join.go`
**Commit:** 40a383c
**Applied fix:** Extracted `raidJoinSettings(chat)` (supergroup, Redis, bounded settings read, any trigger on) and call it
from both counters; nil checks and their order are unchanged. Existing join-surge tests cover it.

### IN-05: `/autolockdown` reads settings with an unbounded `context.Background()`

**Files modified:** `alita/modules/autolockdown.go`, `alita/modules/raid_join.go` (comment), `alita/modules/autolockdown_test.go`
**Commit:** 40a383c (same commit as IN-04, as a closely related pair)
**Applied fix:** Both reads go through `autolockdownReadSettings`, which uses `context.WithTimeout(..., raidSettingsTimeout)`
and the existing `raidSettingsLookup` seam. A test checks that the view and the readback after a save each get a
deadline within `raidSettingsTimeout` and that a failed read still gets its reply.

## Skipped Issues

None. All 8 in-scope findings were fixed.

## Notes for the orchestrator

- Pre-existing, not changed by the fixes: the French `lockdown_help_msg` is about 4298 characters with its header before
  this work (4347 after IN-03, which trimmed what it could), over Telegram's 4096 limit for a single help message. It
  needs a separate trim or a split.
- `make test` and `make lint` were not run, as instructed. The wider package runs above did pass.

---

_Fixed: 2026-10-29_
_Fixer: Claude (gsd-code-fixer)_
_Iteration: 1_
