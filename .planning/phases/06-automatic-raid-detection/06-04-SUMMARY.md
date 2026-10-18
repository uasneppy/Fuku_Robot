---
phase: 06-automatic-raid-detection
plan: 04
subsystem: lockdown
tags: [raid-detection, message-flood, redis, lua, gotgbot, handler-group]

requires:
  - phase: 06-automatic-raid-detection
    provides: raidFire, raidTrigger, the alita:raid:{chat}:joined set, raidSettingsLookup, lockdownAlertTriggerLine flood branch
provides:
  - the group -4 flood watcher (raidFloodWatcher) registered in LoadLockdown
  - raidFloodItemKey (what counts as user content, albums once) and raidFloodCounts (the 60% and 3-account rule)
  - raidFloodScript, the Lua script behind alita:raid:{chat}:fi, :fn and :fa
affects: [06-05, 06-06, phase-07-ai-trigger]

actuals:
  tokens: 9500
  tasks: 2
  commits: 3

plan_head_before: bbacab3be9576b9b218945abcf56e1d78bae264e
plan_head_after: 220e867b0e6425913c66de9b2b57fd6a3a5a3849

commits: 3

tech-stack:
  added: []
  patterns:
    - "One Lua call adds an item to three hash-tagged sorted sets and returns their sizes; the Go side decides"
    - "A watcher at its own handler group that returns ext.ContinueGroups on every path"
    - "Exemptions checked cheapest first: chat type, sender shape, item, Redis, settings, approvals, cached admin list"

key-files:
  created:
    - alita/modules/raid_flood.go
    - alita/modules/raid_flood_test.go
  modified:
    - alita/modules/lockdown.go
    - AGENTS.md

key-decisions:
  - "A text that begins with / and a non-space character is a command even without a bot_command entity, because gotgbot's command handler runs it; counting it would also make the watcher call the admin list on every command in tests that send entity-less commands"
  - "The sender_chat skip stays an explicit check beside the service-ID list; the service IDs alone would hide a regression of it"
  - "The Lua script is the research's verified script unchanged; the fire decision stays in Go (raidFloodCounts)"

patterns-established:
  - "raidFloodSenderCounts judges a sender from the update alone, before any Redis or database read"

requirements-completed: [RAID-02, RAID-07]

duration: 50min
completed: 2026-10-18
status: complete
---

# Phase 6 Plan 04: Message flood trigger Summary

**30 items in 30 seconds, at least 60% of them from at least 3 accounts that joined in the last 24 hours, now lock the group through the same raidFire path as the join surge, with admins, approved users, service accounts, posts as the group or a channel, commands and edits never counted.**

## Performance

- **Duration:** about 50 min
- **Completed:** 2026-10-18
- **Tasks:** 2
- **Files modified:** 4 (2 created)

## Accomplishments

- A message watcher at the new handler group -4 (registered in `LoadLockdown`) adds each countable item to three Redis sorted sets in one Lua call: all items, items from members the joined set scores within 24 h, and those members' IDs. When the window holds the chat's threshold (default 30), at least 60% from new members and at least 3 such accounts, `raidFire` starts the lockdown with `trigger_kind` flood and the count, window (30) and percent on the row. The Staff Group alert names the flood with those numbers.
- The watcher returns `ext.ContinueGroups` on every path and deletes nothing, so aispam, antiflood, locks and blacklists still see every message and the lockdown's muting stops the flood.
- What counts is any user content (text that is not a command, photo, GIF, sticker, video, video note, voice, audio, document, poll, contact, location, venue, dice, story); an album counts once through the key `g:<media_group_id>`.
- Never counted: admins (cached list), approved users, the bot, Telegram service IDs, posts with `sender_chat` set (anonymous admin, channels, a linked channel's automatic forward), commands, service messages, edited messages, bot senders, and anything in a basic group.
- AGENTS.md carries the `-4` handler group, the `:fi`/`:fn`/`:fa` keys and the flood-trigger rules.

## Task Commits

1. **Task 1: a flood from recent joiners locks the group, and the alert names the flood**
   - `66a8189` (test) TestRaidFlood: 30 items from four members who joined 2 h ago lock; established members and members who joined 25 h ago do not
   - `5e1c39f` (feat) raid_flood.go, the -4 registration in LoadLockdown, AGENTS.md
2. **Task 2: what never counts and what does not trip the rule**
   - `220e867` (test) TestRaidFloodItemKey, TestRaidFloodExemptions, TestRaidFloodRule, TestRaidFloodScript (no implementation commit was needed, see below)

**Plan metadata:** the docs commit adding this file.

## TDD Gate Compliance

- Task 1 RED (`66a8189`) needed no compile scaffolding: the test file uses only symbols that already existed. `TestRaidFlood/recent_joiners_flood` failed on its own assertion, "no active lockdown after the 30th item in 30 seconds from recent joiners"; the other two subtests passed because nothing locks without a watcher, which is expected. GREEN (`5e1c39f`) made all three pass.
- Task 2: all four new tests passed when first run, because Task 1's code already implemented the item rules, the exemptions, the 60% and 3-account floors and the script. This is an unexpected GREEN on the test commit, investigated rather than ignored. Following the plan's allowance (keep the test commit alone when the Task 1 code passes every case, with a mutation check), each guard was shown able to fail; all mutations were temporary and reverted (`git diff` of `raid_flood.go` is clean against `5e1c39f`):
  - removing the `sender_chat` skip (both checks) failed only `sender_chat on an ordinary user`; the group anonymous bot, channel bot and linked channel cases still pass without it because their user IDs are in `staffServiceUserIDs`, which is why the extra synthetic case exists;
  - removing the `staffServiceUserIDs` check failed `service account 777000`;
  - disabling the cached admin check and the approval check failed `admin` and `approved user`;
  - disabling the album key, loosening the share to 50% and the account floor to 1 failed `56 percent does not lock`, `two accounts do not lock` and `an album counts once`.
- There is therefore no `fix(06-04)` commit.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] An entity-less "/command" must not count**
- **Found during:** Task 1 (verification, the wider Lockdown/Unlockdown/Raid/Antiflood/Blacklist/Lock run)
- **Issue:** the plan defines a command as a text whose first entity is a `bot_command` at offset 0. gotgbot's command handler also runs a text that begins with `/` with no entity, and the existing lockdown tests send commands that way (`env.send(env.admin, "/lockdown")`). The watcher counted them as text, so it called getChat and the admin list on every such command, which changed existing tests' Telegram call counts and consumed their scripted getChat and getChatMember failures (about ten subtests of `TestLockdownSettlesUnconfirmedLock`, `TestUnlockdownWithoutLockdown`, `TestLockdownCommandsRefuseNonAuthority` and `TestLockdownRefusals` failed).
- **Fix:** `raidFloodIsCommand` also treats a text starting with `/` followed by a non-space character as a command, mirroring the dispatcher. Real Telegram always sends the entity, so production behaviour is the plan's rule; the extra form only matters for entity-less text, which the dispatcher runs as a command anyway. No existing test or assertion was edited. AGENTS.md states the rule as implemented.
- **Files modified:** `alita/modules/raid_flood.go`, `AGENTS.md`
- **Verification:** `go test -tags testtools -race -count=1 -run 'Lockdown|Unlockdown|Raid|Antiflood|Blacklist|Lock' ./alita/modules` passes; `TestRaidFloodItemKey` pins `command with no entity` and `a lone slash` (counts).
- **Committed in:** `5e1c39f` (part of the Task 1 GREEN commit)

---

**Total deviations:** 1 auto-fixed (1 bug). **Impact on plan:** none on behaviour for real traffic; it keeps the plan's "commands never count" rule true for every command the dispatcher recognises.

## Issues Encountered

- `make lint` cannot run here (golangci-lint cannot lint Go 1.26); `gofmt -l` on the touched files and `go vet -tags testtools ./alita/modules` stand in and are clean. `alita/modules/greetings_command_test.go` is not gofmt-clean on the base commit; it was left alone.
- No PostgreSQL server was available or started; every test used SQLite and miniredis. The full `make test` was not run (the orchestrator owns it).
- `docs/src/content/docs/architecture/handler-groups.md` still lists the old group table (it has no `-7` or `-4` rows and still shows antiraid at `-5`); `make check-docs` reports no drift, and the page is part of the antiraid retirement later in the phase, so it was not touched here.

## Verification

- `TestRaidFlood`, `TestRaidFloodItemKey`, `TestRaidFloodExemptions`, `TestRaidFloodRule`, `TestRaidFloodScript`: pass with `-tags testtools -race`.
- `-run 'Lockdown|Unlockdown|Raid|Antiflood|Blacklist|Lock' ./alita/modules`: pass (about 91 s).
- `CGO_ENABLED=0 go build ./...`, `go vet -tags testtools ./alita/modules`, `make check-docs`: clean. `git diff --exit-code go.mod go.sum`: clean.
- Acceptance criteria: `raidFloodGroup = -4` and the `lockdownModule.raidFloodWatcher), raidFloodGroup)` registration match; `return ext.ContinueGroups` appears 10 times and `ext.EndGroups` 0 times outside comments in `raid_flood.go`; the AGENTS.md group list entry matches once; `raidFloodItemKey` and `raidFloodCounts` exist.

## Known Stubs

None.

## Threat Flags

None beyond the plan's threat model: T-06-11 (sender_chat skip, pinned by `sender_chat on an ordinary user`), T-06-12 (cheapest-first order, one script call under 250 ms, caps and TTLs), T-06-13 (only item keys and user IDs reach Redis; no text is logged or stored) and T-06-14 (60% and 3-account floors, admins and approved users excluded) are implemented as written.

## Next Phase Readiness

- Plan 06-05 can tune `flood_threshold` per chat: the watcher already reads `settings.FloodEnabled` and `settings.FloodThreshold` through `raidSettingsLookup` on every counted message.
- A flood lockdown's alert shows the flood line and counts, but its burst snapshot still comes from the 10-minute join tracker, so "Ban N recent joiners" has nothing to ban for a flood of accounts that joined hours earlier (research Pitfall 10, descoped by this plan; `fa` is not kept for it).
- Plan 06-03 (service-message join writes to the joined set) runs in parallel; the two plans share no file except `AGENTS.md`, where this plan adds new bullets rather than editing existing ones.

## Self-Check: PASSED

- `alita/modules/raid_flood.go`, `alita/modules/raid_flood_test.go`: found.
- Commits `66a8189`, `5e1c39f`, `220e867`: found in `git log`.
- `git rev-list --count bbacab3..HEAD` was 3 before this file.

---
*Phase: 06-automatic-raid-detection*
*Completed: 2026-10-18*
