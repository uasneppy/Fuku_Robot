---
phase: "6"
slug: "automatic-raid-detection"
status: verified
# threats_open = count of OPEN threats at or above workflow.security_block_on severity (the blocking gate)
threats_open: 0
asvs_level: 1
register_authored_at_plan_time: true
created: "2026-10-29"
---

# Phase 6 — Security

> Per-phase security contract: threat register, accepted risks, and audit trail.

---

## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| Joining accounts to the bot | Anyone can join a public group; their joins drive the join counter |
| Join updates to the counter | Telegram delivers a join twice; the performer field decides who is exempt |
| Group messages to the flood counter | Every member's message reaches the watcher; senders can post as the group or a channel, forward, or be Telegram service accounts |
| Counters to the ban path | A count may start a lockdown and nothing else; it must never ban, restrict or delete |
| Group members to `/autolockdown` and `/autoantiraid` | Anyone can type them; only a live creator or restricting admin may change a setting |
| Replicas to Redis and PostgreSQL | Several replicas count and may fire at the same moment; the database decides |
| Raid traffic to the Telegram API and to Redis | Every join and message of a raid reaches the hot path; a sustained raid must not amplify calls or memory |
| Group titles and account names to staff-facing HTML | Titles are set by group admins, names by the accounts themselves |
| Bot to Staff Group and log channel | Warnings must reach only the group's own destinations |
| Old replicas and old Redis state to the new code | A rolling deploy and leftover `alita:antiraid:*` keys meet the new bot |
| CI database to the carry-over test | The test runs against the CI PostgreSQL |
| Docs, help text and AGENTS.md to admins and later phases | Admins and Phase 7 rely on them to know what the bot will do |

---

## Threat Register

Built from the `<threat_model>` blocks of plans 06-01 to 06-13. The supply-chain row (T-06-SC) appears in every plan and is listed once. File references are to the production code as audited (`a5ee321`; the later `c180e93` changes only `06-VALIDATION.md`); see the audit trail for the test evidence.

| Threat ID | Category | Component | Severity | Disposition | Mitigation / evidence | Status |
|-----------|----------|-----------|----------|-------------|-----------------------|--------|
| T-06-01 | Tampering | `/lockdown` behaviour changed by the extraction | high | mitigate | `startLockdownRow` (`alita/modules/lockdown_start.go:81-175`) holds the extracted sequence in the old order (bot rights, getChat and permissions, `lockdown.Start`, lock call, `ConfirmLocked`, worker wake). The diff of `lockdown.go` shows each removed step reappearing there, and `/lockdown` maps every outcome to the reply it always sent (`lockdown.go:340-364`). `TestLockdownStartRow` and the unchanged Phase 4 and 5 lockdown suites pass | closed |
| T-06-02 | Tampering | two simultaneous starts, two lockdowns of one chat | high | mitigate | `lockdown.Start` is `INSERT ... ON CONFLICT DO NOTHING` (`alita/db/lockdown/repository.go:57-71`) behind the partial unique index `uk_chat_lockdowns_active` (`migrations/20261006120000_add_chat_lockdowns.sql:53`); the loser gets `lockdownStartAlreadyActive` (`lockdown_start.go:132-141`). `TestLockdownStartRow` (concurrent case) passes under `-race`. The PostgreSQL proof `TestStartLockdownOneActivePerChat` is in the CI list (`Makefile`, `test-postgres-integrity`) and was not run here | closed |
| T-06-03 | Tampering | alert trigger line carrying user text | low | mitigate | `lockdownAlertTriggerLine` (`lockdown_alert.go:543-567`) and `lockdownTriggerSentence` (`raid_trigger.go:331-346`) pass only `strconv.Itoa` integers and a time; the manual branch splices the starter's name after translation (`lockdown_alert.go:561-567`); the title keeps `lockdownGroupLabel` (`:439-444`) | closed |
| T-06-04 | Denial of Service | sock-puppet joins locking a victim group | medium | accept | See AR-06-02 | closed |
| T-06-05 | Tampering | two triggers or replicas starting two lockdowns | high | mitigate | As T-06-02. The `SET NX` latch only sheds load (`raid_trigger.go:221-236`); the decision is `startLockdownRow` → `lockdown.Start` (`raid_trigger.go:281`). `TestRaidJoinSurge`, `TestRaidTriggersSingleLockdown`, `TestRaidTriggersConcurrent` pass | closed |
| T-06-06 | Denial of Service | Redis growth from joins | low | mitigate | `raidJoinScript` (`raid_counters.go:83-94`): members are user IDs, 24 h trim (`ZREMRANGEBYSCORE`), cap `raidJoinedCap` 5000 (`:45`, `:91`) and `PEXPIRE` 25 h (`:36`, `:92`) on every write. `TestRaidJoinScript` passes | closed |
| T-06-07 | Elevation of Privilege | a count turning into a ban | high | mitigate | A grep of `raid_counters.go`, `raid_join.go`, `raid_flood.go`, `raid_trigger.go`, `autolockdown.go` and `antiraid.go` finds no ban, unban, restrict, delete, approve or decline call. `raidRecordAndFire` (`raid_join.go:172-188`) and `raidFloodCountUpdate` (`raid_flood.go:311-318`) end only in `raidFire` → `startLockdownRow`; the one Telegram write in these files is the notice and warning send `raidSend` (`raid_trigger.go:312-324`). Bans stay the worker's | closed |
| T-06-08 | Spoofing | a raider posing as admin-added to avoid counting | medium | mitigate | Exemption only from a live `lockdownLiveMember` of the performer (`raid_join.go:77-86`, `:116-124`; `lockdown.go:108-120`, 5 s) or the anonymous-admin identity (`lockdown_guard.go:95-105`, `:112-130`); a failed lookup leaves the status empty and the join counts (`raidJoinCounts`, `raid_join.go:43-56`). The guard reuses that status and treats an empty one as never exempt (`lockdown_guard.go:167`, `:239`). `TestRaidJoinExemptions` (14 subtests, incl. `performer lookup failed` and `approved request by the bot`) passes | closed |
| T-06-09 | Denial of Service | one join counted twice inflating the surge | medium | mitigate | The set member is the user ID and the score the latest join, `ZADD` without NX (`raid_counters.go:86`), so the chat_member update and the service message are one member. `TestRaidJoinDoubleDelivery` passes | closed |
| T-06-10 | Denial of Service | an extra live lookup per admin-added join | low | mitigate | `raidCountMemberJoin` and `raidCountMessageJoins` return `(status, looked)` (`raid_join.go:67-92`, `:102-135`) and both guards skip their own lookup when `looked` (`lockdown_guard.go:167`, `:239` and the branches after them). `TestRaidJoinServiceMessage` counts `getChatMember` calls | closed |
| T-06-11 | Spoofing | anonymous-admin or channel posts counted as members | medium | mitigate | `raidFloodSenderCounts` (`raid_flood.go:199-213`) refuses `sender.Chat`, `msg.SenderChat`, `IsAutomaticForward`, `userID <= 0`, the bot and `staffServiceUserIDs` (`staff_action_decide.go:513`: 777000, 1087968824, 136817688) before any Redis call. gotgbot's `Message.CheckUpdate` drops bot senders, and edits and channel posts are not allowed on the handler (`handlers/message.go:62`, `NewMessage` defaults). `TestRaidFloodExemptions` passes: service accounts, anonymous admin, Channel_Bot, automatic forward, `sender_chat` on a user, edited message, bot sender | closed |
| T-06-12 | Denial of Service | hot-path cost on every group message | medium | mitigate | `raidFloodCountUpdate` (`raid_flood.go:270-320`) orders chat type, sender, item (pure Go), Redis client, settings, approvals, cached admin list (`antiflood.go:122-166`: semaphore, 3 s bound), then one script call under `raidRedisTimeout` 250 ms (`raid_counters.go:38`, `raid_flood.go:225`). Every error returns `ContinueGroups` as no information. Sets are trimmed to 30 s, the two item sets capped at 2000 (`raid_flood.go:90`, `:113-116`), `PEXPIRE` 2 min (`:117`); the accounts set is bounded by the joined set's 5000 cap. See observation O-1 | closed |
| T-06-13 | Information Disclosure | message content leaking into Redis or logs | low | mitigate | The script arguments are the item key (`m:<message id>` or `g:<media group id>`) and the user ID only (`raid_flood.go:133-157`, `:227-235`); the only logs on the path are chat-ID debug lines (`:292`, `:308`); no text is stored or logged | closed |
| T-06-14 | Denial of Service | a busy legitimate chat locked by mistake | medium | mitigate | `raidFloodCounts` (`raid_flood.go:183-192`): items at least the threshold, new-member share at least 60%, at least 3 new accounts (constants `:61-63`); approved users and cached admins are skipped before counting (`:299-304`); tunable by `/autolockdown`. `TestRaidFloodRule` passes | closed |
| T-06-15 | Elevation of Privilege | a member or demoted admin turning detection off | high | mitigate | `autolockdownDesc` checks are `requireLockdownGroup()` and `requireAutoLockdownAuthority()` (`autolockdown.go:36-39`). With any word it is `requireLockdownAuthority(true)` (`:48-57`): a live `getChatMember` of the sender (`lockdown.go:150`), creator or administrator with `can_restrict_members` (`:156-163` via `staffIssuerSkipReason`, `staff_action_run.go:615-626`); there is no admin-cache or `AnonAdmin` setting in the path, a failed lookup refuses (`:151-154`), a message sent as another chat is refused (`:146-148`). An anonymous admin always gets the proof button first (`:138-143`); the re-entry is registered (`lockdown.go:577`) and re-runs the same checks on the rebuilt update with the tapper as sender (`moderation.go:349-360`, `bot_updates.go:214-222`). The check and the handler read `lockdownCommandArgs` of the same context (`autolockdown.go:52`, `:173`), so the view-or-change decision cannot differ between them. `TestAutoLockdownAuthority` (11 subtests: restrict-less admin, plain member, admin cache never authorizes, failed lookup, sender_chat, anonymous with and without the right) passes | closed |
| T-06-16 | Tampering | out-of-range or malformed thresholds | medium | mitigate | `parseAutoLockdownArgs` (`autolockdown.go:77-112`): exactly two words, `joins\|flood`, then `on`, `off` or a whole number; a parse error or a value outside 3..500 is `errAutoLockdownRange` before any write (`:102-106`). The repository re-checks (`raiddetection/repository.go:86-88`, `:118-121`, `:137-140`) and the table has CHECKs 3..500 (`migrations/20261018130000_add_chat_raid_detection.sql:33-34`). `TestAutoLockdownArgs`, `TestAutoLockdownBadInput`, `TestRaidDetectionSettings` pass (the CHECK half runs on PostgreSQL in CI) | closed |
| T-06-17 | Tampering | a stale setting served on another replica | medium | mitigate | `raidDetectionPrefix` is in `skipLocal` (`alita/db/cache/local.go:47`, `:55-71`); `upsertSettings` calls `cache.DeleteCache` for the chat's key after every write (`raiddetection/repository.go:105`) and returns before it on a failed write. `TestSkipLocalMatchesFreshnessCriticalKeys` and `TestAutoLockdown` pass | closed |
| T-06-18 | Tampering | two admins clobbering each other's trigger | low | mitigate | Four column-targeted `INSERT ... ON CONFLICT (chat_id) DO UPDATE` of named columns plus `updated_at` (`raiddetection/repository.go:78-83`, `:94-107`); the model has no `default:` tag, so a Go `false` is written (`models/raid_detection.go:9-21`). `TestRaidDetectionSettings` passes | closed |
| T-06-19 | Tampering | two lockdowns or two alerts for one raid | high | mitigate | As T-06-02 for the lockdown; the alert is decided by Phase 5's conditional `BeginAlert`, which Phase 6 does not touch (`alita/db/lockdown/alerts.go` has no Phase 6 diff). `TestRaidTriggersConcurrent` and `TestRaidTriggersSingleLockdown` pass. `TestStartLockdownOneActivePerChat` and `TestBeginAlert` are in the CI list | closed |
| T-06-20 | Denial of Service | Telegram calls amplified by a sustained raid | medium | mitigate | `raidFire` (`raid_trigger.go:221-308`): `SET NX` latch 60 s (`:228`), the active lockdown read before any Telegram call (`:242-250`), `raidOutcomeLatch` 10 s after a transient failure and 10 min for a group the bot cannot lock (`:170-192`). `TestRaidLatch` passes | closed |
| T-06-21 | Denial of Service | a group re-locked the moment an admin lifts it | low | mitigate | `lockdown.RecentlyLiftedFresh` (`alita/db/lockdown/cooldown.go:18-29`: any state, `lift_started_at > since`, uncached) called with `raidLiftQuiet` 10 min and real time (`raid_trigger.go:31`, `:255-265`); index `idx_chat_lockdowns_chat_lift` (`migrations/20261018120000_add_lockdown_trigger_columns.sql:26`). `TestRaidCooldownAfterLift` passes | closed |
| T-06-22 | Denial of Service | warning or notice spam during a sustained raid | medium | mitigate | The notice is posted only for `lockdownStartStarted` (`raid_trigger.go:302-304`). The warning is gated by `SET NX EX 86400` per group (`:434-444`) and the gate is given back with `DEL` only when nothing was delivered (`:479-481`, `:487-494`). Every send goes through `lockdownPaced` under 30 s (`:312-324`). Posts run in `raidPostAsync` goroutines with a counter and panic recovery (`:127-134`), started at most once per 60 s or 10 min latch. `TestRaidCannotLockWarnsOncePerDay`, `TestRaidAutoNotice`, `TestRaidPostsDoNotBlockTheTrigger` pass | closed |
| T-06-23 | Tampering | HTML injection through the group title | medium | mitigate | The title is capped at 128 runes and `html.EscapeString`-ed in `lockdownGroupLabel` (`lockdown_alert.go:439-444`; `raid_trigger.go:446-449`) and spliced in after translation by token (`raid_trigger.go:450-459`); the log-channel header escapes it in `actionlog.Destination` (`alita/utils/actionlog/actionlog.go:29-30`). The public notice carries no title and the trigger sentence only integers. The subtest `title is escaped` passes | closed |
| T-06-24 | Information Disclosure | a warning reaching a chat that is not the group's Staff Group or log channel | medium | mitigate | Destinations are `staff.GetLinkOfGroupFresh(chat.Id)` (uncached) and `actionlog.Destination(chat, CategoryAdmin)` (`raid_trigger.go:424-432`); with neither nothing is sent and the gate is untouched; sends go only to `link.StaffChatID` and `channelID` (`:463`, `:471`). Subtests `only the Staff Group` and `no destinations` pass | closed |
| T-06-25 | Information Disclosure | trigger details shown to non-admins | low | mitigate | `lockdownStatusDesc` keeps `requireLockdownGroup()` and `requireLockdownAuthority(false)` (live creator or administrator; `lockdown_status.go:20-23`); the trigger text comes from the row's integer columns (`:64-80`); reminders keep the Phase 5 destinations | closed |
| T-06-26 | Repudiation | an automatic lockdown read as an admin's action | low | mitigate | `lockdownStartedAutomatically` (`lockdown_status.go:107-112`) switches `/lockdownstatus` to `lockdown_status_active_auto` plus the trigger sentence (`:64-80`) and `/lockdown` to `lockdown_already_active_auto` (`lockdown.go:274-278`); the row records the bot as starter and the trigger columns (`raid_trigger.go:281-289`). `TestRaidLockdownStatus` passes | closed |
| T-06-27 | Tampering | HTML injection through flooders' names | medium | mitigate | `lockdownMergeFlooders` (`lockdown_alert.go:251-305`) caps names at 64 runes and usernames at 32 (`:293-294`); `lockdownRenderSampleAccount` escapes every user string at render (`:450-464`) and the sample is spliced in after translation (`:497`). The subtest `flooders listed` uses names with tags and passes | closed |
| T-06-28 | Elevation of Privilege | Ban N banning an admin, an approved user or an established member | high | mitigate | The flooder snapshot is built only from `:fa`, which receives only accounts that passed the approved-user and cached-admin checks and are in the 24 h joined set (`raid_flood.go:299-304`, `:105-109`, `:332-347`); established members are never in `:joined`. The accounts go to the unchanged Phase 5 ban run (`lockdown_response.go` has no Phase 6 diff), which decides each account live and reads approvals fresh. The subtests `ban the flooders` (leaves the established member alone) and `exempt senders and the bot are not saved` pass | closed |
| T-06-29 | Denial of Service | an oversized snapshot | low | mitigate | `raidFloodersMax` 500 and `raidFloodersTTL` 15 min (`raid_flood.go:81`, `:84`), one `user.FindUsersByIDs` query (`lockdown_alert.go:283`, `alita/db/user/repository.go`), the alert lists at most 10 (`lockdown_alert.go:44`) | closed |
| T-06-30 | Elevation of Privilege | `/autoantiraid` turning the join surge off | high | mitigate | `autoantiraidDesc` has the same two checks as `/autolockdown` (`antiraid.go:52-55`); `autoantiraid` prepends `joins` only inside the handler after the checks passed (`:73-79`), and the check treats any word as a change, so `off` and `N` need the live restrict right; alone it is a view for any live admin. The anonymous re-entry is registered (`antiraid.go:178`). `TestAntiRaidRetired` subtests `alias authority` and `alias anonymous admin` pass | closed |
| T-06-31 | Tampering | leftover raid-mode keys causing temp-bans | medium | mitigate | No join handler at group -5 and no poller: `LoadAntiRaid` registers only the pointer command, the alias and the old-button answer (`antiraid.go:164-172`); the one non-test reference to `alita:antiraid:` is the sweep pattern (`:103`); `alita/db/antiraid` and `StopAntiRaidExpiryPoller` are gone (`main.go` diff). The sweep removes the keys (`:116-140`). `TestAntiRaidRetiredNoBan` and `TestAntiRaidKeySweep` pass. The rolling-deploy window of an old replica is AR-06-05 | closed |
| T-06-32 | Information Disclosure | the pointer reply | low | accept | See AR-06-03 | closed |
| T-06-33 | Denial of Service | the sweep blocking startup | low | mitigate | `startRetiredAntiRaidSweep`: `sync.Once` (`antiraid.go:111`), its own goroutine with `RecoverFromPanic` (`:146-147`), 30 s context (`:107`, `:148`), `SCAN` with COUNT 100 and `UNLINK` of the batch (`:105`, `:124-129`); no Redis returns at once (`:117-120`); an error stops and returns the count so far. `TestAntiRaidSweepRunsOnceOnLoad` passes | closed |
| T-06-34 | Tampering | the carry-over test leaving rows in the CI database | low | mitigate | `TestRaidDetectionCarryOver` opens one transaction and always rolls it back in a defer (`alita/db/migrations/raid_carry_over_test.go:99-107`), uses random large negative chat IDs (`:20-27`) and skips without `ALITA_TEST_DATABASE=true` and `DATABASE_URL` | closed |
| T-06-35 | Information Disclosure | dev stats | low | accept | See AR-06-04 | closed |
| T-06-36 | Repudiation | docs that misstate which commands still act or what a backup restores | low | mitigate | Retired commands, the alias's live authority, the carry-over clamp and the backup gap are stated in `docs/src/content/docs/commands/antiraid/index.md:53-80`, `api-reference/commands.md:51`, `:351`, `api-reference/database-schema.md:703`, `:751` and `commands/backup/index.md:47-49`; no backup code touches `chat_raid_detection` (grep). See observation O-3 | closed |
| T-06-37 | Repudiation | help text that misstates the triggers or who may change them | low | mitigate | `lockdown_help_msg` (`locales/en.yml:2815-2839`) gives the defaults and windows (`:2831`), the exemptions, the live authority (`:2821-2823`), the 10 minute quiet period, the once-a-day warning and the backup gap; each figure matches the code (`raiddetection/repository.go:24-27`, `raid_flood.go:58-63`, `raid_trigger.go:31`, `:53`). `make check-translations` is clean (run here). See observation O-4 | closed |
| T-06-38 | Tampering | AGENTS.md rules that drift from the code and mislead Phase 7 | medium | mitigate | The Phase 6 rules were checked against the code: groups -4 and 12 (`raid_flood.go:48`, `:54`), caps 5000, 2000 and 500 (`raid_counters.go:45`, `raid_flood.go:90`, `:84`), TTLs 25 h, 2 min, 15 min and 24 h (`raid_counters.go:36`, `raid_flood.go:65`, `:81`, `raid_trigger.go:53`), the latch 60 s, 10 s and 10 min and the 10 min quiet period (`raid_trigger.go:27-41`, `raid_counters.go:40`), the sweep (`antiraid.go:105-107`), `skipLocal` (`local.go:47`), the 2 s post drain (`raid_trigger.go:65`, `shutdown_drains.go`, `main.go:221`) and the `/autolockdown` grammar and authority (`autolockdown.go`). No drift found in AGENTS.md | closed |
| T-06-SC | Tampering | npm/pip/cargo installs | low | accept | See AR-06-01 | closed |

*Status: open · closed · open — below high threshold (non-blocking)*
*Severity: critical > high > medium > low — only open threats at or above workflow.security_block_on count toward threats_open*
*Disposition: mitigate (implementation required) · accept (documented risk) · transfer (third-party)*

---

## Accepted Risks Log

| Risk ID | Threat Ref | Rationale | Accepted By | Date |
|---------|------------|-----------|-------------|------|
| AR-06-01 | T-06-SC | No packages were installed: `git diff 90f6f9d..HEAD -- go.mod go.sum` is empty (`90f6f9d` is the parent of the first Phase 6 code commit `e9dac9d`) | Plan threat models 06-01 to 06-13 | 2026-10-29 |
| AR-06-02 | T-06-04 | Automatic lockdown is a lever an attacker can pull. Fifteen fresh accounts joining within 60 s lock a victim group at the default; the flood variant is cheaper: at least 3 accounts that joined in the last 24 h sending 30 items in 30 s, at least 60% of them from those accounts. The group is then locked and joiners are removed until an admin lifts it, and a false lockdown costs one `/unlockdown`. Compensating controls are in the code: admin-added, approved, bot and request joins never count (`raid_join.go:43-56`); the 3-account and 60% floor (`raid_flood.go:183-192`); the 3..500 threshold range and the 15 and 30 defaults (`raiddetection/repository.go:24-31`); only a live creator or restricting admin can lift (`lockdown.go:64-67`); a 10 minute quiet period after a lift (`raid_trigger.go:255-265`); per-group tuning and off switches (`/autolockdown`) | Owner decision, `06-CONTEXT.md` (specifics, "a false lockdown costs an admin one `/unlockdown`") and `06-RESEARCH.md` threat table | 2026-10-18 |
| AR-06-03 | T-06-32 | `/antiraid`, `/raidtime` and `/raidactiontime` answer with static text that names no group data and change nothing, to anyone, like the old help (`antiraid.go:43-47`, `:59-67`) | 06-10 threat model | 2026-10-29 |
| AR-06-04 | T-06-35 | The developer `/stats` shows only three counts of `chat_raid_detection` (`raiddetection/stats.go:13-17`), and `getStats` returns unless the user is the owner or a developer (`alita/modules/devs.go:362-371`) | 06-11 threat model | 2026-10-29 |
| AR-06-05 | T-06-31 | During a rolling deploy an old replica can still temp-ban from the retired raid mode and write new `alita:antiraid:*` keys until it stops; the next start's sweep clears them (`antiraid.go:97-100`). The new code never reads those keys | 06-10 threat model | 2026-10-29 |

Related limits recorded in `AGENTS.md`, `06-CONTEXT.md` and the plans, not register threats: automatic detection is Redis only and inert without it, `/lockdown` still works; a command the bot handles never counts toward a flood (D-07), and a slash-prefixed text that an earlier group ends is not counted late, so a flood made only of those is not counted; the live admin fallback assumes an admin when saturated, which only lowers counts; a crash between send and record can post a duplicate Phase 5 alert copy (duplicate preferred over lost); the automatic notice and warning are best effort and one cut off at shutdown is lost; the `/autolockdown` settings are not part of backup, export, import or reset (D-20).

*Accepted risks do not resurface in future audit runs.*

---

## Observations

Not register threats, no unregistered flags (none of the 13 SUMMARY files raised a threat flag). None blocks the phase, and no production code was changed by this audit.

- **O-1, settings read bound.** The settings read on the per-message and per-join path is bounded at `raidSettingsTimeout` 2 s (`raid_join.go:16`), not 250 ms, and the key is in `skipLocal`, so each counted message makes one Redis read for it. The 250 ms bound in T-06-12 holds for the script. It matters only in a Redis brownout, when the group -4 watcher can hold a message for up to 2 s before aispam, antiflood, locks and blacklists run. Optional hardening: a shorter bound on the per-message read.
- **O-2, accounts set cap.** `:fa` has no explicit cap in the script (`raid_flood.go:113`, the loop covers the two item sets). It is bounded by the 30 s trim and by membership in the joined set (cap 5000).
- **O-3, docs drift.** `docs/src/content/docs/architecture/caching.md:30` still lists `CacheTTLAntiRaid`, which review fix IN-01 (`929c2d2`) removed from `alita/db/cache/ttl.go`. A stale doc row only; fix with the next docs change.
- **O-4, help wording.** `locales/en.yml:2823` says "Only the group owner, or an admin who can restrict members, can use these commands", which is stricter than the code for `/lockdownstatus` and for viewing `/autolockdown` (any live admin). The per-command lines above it are right, and the error is on the strict side.
- **O-5, stale flooder snapshot.** The flooder snapshot is best effort. If its script call fails at a second flood lockdown that starts within 15 minutes of the first flood's snapshot (a quick lift plus the 10 minute quiet period fits), the alert can list the earlier flood's accounts. The TTL bounds it, and the ban run still decides each account live.
- **O-6, public thresholds.** The help text and the public notice state the trigger numbers, which tells a raider how much stays under the line. That is the owner's choice (help text is explicit that the numbers are defaults).

---

## Security Audit Trail

| Audit Date | Threats Total | Closed | Open | Run By |
|------------|---------------|--------|------|--------|
| 2026-10-29 | 39 | 39 | 0 | gsd-security-auditor (L1 evidence, with boundary-placement checks on the high-severity threats) |

## Security Audit 2026-10-29
| Metric | Count |
|--------|-------|
| Threats found | 39 |
| Closed | 39 |
| Open | 0 |

Evidence at L1 depth, plus placement checks on the high-severity threats (authority, exactly-once, count-to-ban):
- 35 `mitigate` threats were checked in code with file and line evidence; 4 `accept` threats (T-06-04, T-06-32, T-06-35, T-06-SC) have entries above. Severity split: 8 high, 15 medium, 16 low. Block threshold `high`; no open threat of any severity.
- The named mitigation tests run and pass under `-race -count=1 -tags testtools` on SQLite and miniredis: 38 top-level tests in `./alita/modules`, `./alita/db/raiddetection` and `./alita/db/cache`, 0 failed, 0 skipped. `make check-translations` is clean. PostgreSQL was never started and `DATABASE_URL` was unset.
- Not run here: `TestRaidDetectionCarryOver` (PostgreSQL only, SKIPs on SQLite) and the PostgreSQL half of `TestRaidDetectionSettings`, plus `TestStartLockdownOneActivePerChat` and `TestBeginAlert`. They are in the `make test-postgres-integrity` list (`./alita/db/lockdown ./alita/db/raiddetection ./alita/db/migrations`) and run in CI. T-06-02, T-06-05, T-06-16 and T-06-19 rest on them for the database half; the code half is verified above.
- Migrations: `20261018120000` and `20261018130000` are new files sorted after every existing one; no earlier migration file was modified or deleted since `e9dac9d~1`; neither has a top-level `BEGIN` or `COMMIT`, nor `CONCURRENTLY`; every statement is idempotent; `CHECK` 3..500 and the `LEAST(GREATEST(x, 3), 500)` clamp are in `20261018130000`; the runner's splitter skips `--` comments (`alita/db/migrations/runner.go:233-268`), so the semicolons in the comments are safe. `runner.go` and `scripts/migrate_psql.sh` are unchanged, so both appliers still agree.
- Redis key and script injection: every key comes from `raidKey` (`raid_counters.go:57-59`) with an `int64` chat ID and a constant part; script arguments are `int64` values formatted with `strconv`, the item key (`m:<int>` or `g:<media group id>`) and numbers, passed as `ARGV` and never joined into script text. The sweep pattern `alita:antiraid:*` is a constant.
- Live checks the owner deferred in `06-VERIFICATION.md` (real Telegram surge and flood, two replicas, native-speaker review) are not security evidence and were not run.

---

## Sign-Off

- [x] All threats have a disposition (mitigate / accept / transfer)
- [x] Accepted risks documented in Accepted Risks Log
- [x] `threats_open: 0` confirmed
- [x] `status: verified` set in frontmatter

**Approval:** verified 2026-10-29
