# Project Research Summary

**Project:** Fuku Robot, milestone v1.1 (PM Settings Hub & Member Screening)
**Domain:** Telegram group-management bot (Go, gotgbot v2, GORM on PostgreSQL, Redis, several replicas)
**Researched:** 2026-11-02
**Confidence:** MEDIUM (HIGH on repo seams and the gotgbot surface; MEDIUM on Telegram runtime behaviour, which needs live spikes)

## Executive Summary

v1.1 moves every command-only group setting into the bot's private chat behind buttons, narrows who may configure a group to its owner by default, and adds three automated subsystems: member screening (join and per-message rules with punishments and an "I fixed it" button), an AI spam-name check, and durable message deletion. No new Go dependency is needed. Everything plugs into existing seams: the Phase 9 stateless settings menu (`settingsPress.verify`, `registerSettingsPage`, one apply function per setting), the lockdown worker pattern (database-driven claims, stale-claim release, a shutdown drain), the raid AI client and its breakers, and the Redis operational-key families.

The recommended order is the owner's own: the owner gate first (every new page rides on it), then the private-chat input state and the message editor, then member requirements, then deletion, then the remaining pages. All four research files agree on four cross-cutting rules. Authority is always a live `getChatMember` read, never the admin cache or Redis state. Every punishment and unmute reuses an existing executor (the warn core, `resolveUnmutePermissions`). Every new worker is durable in PostgreSQL and claims rows with conditional updates. Screening verdicts are three-valued, and only a provable failure punishes.

The main risks are mass punishment from a misconfigured or broken rule (a Telegram lookup error read as "fail"), two mute owners fighting over one restriction (captcha, staff, admin and requirement mutes), deleting things that must survive (alerts, staff cards, admin messages, messages from via-bots), and the 48-hour delete limit. The four files disagree on about seventeen design points, including handler group numbers, self-destruct storage, the name-AI pool, the delay cap and the group-list table. They are laid out side by side under **Decisions for phase research** below and must be settled by the phase researchers, not silently by the roadmapper.

## Key Findings

### Recommended Stack

No gotgbot bump. The pinned `v2.0.0-rc.36.0.20260919140833-240296efadb4` already exposes every field and method v1.1 needs (verified in the module source). Persistence is PostgreSQL through GORM with append-only migrations. Short-lived state (the "waiting for your message" input, pass caches, latches, pacers, verdict caches and budgets) lives in Redis with the repo's existing `GETDEL` and Lua compare-and-set patterns. Name script rules use the standard `unicode` tables. NFKC normalisation for cache keys uses `golang.org/x/text`, already a direct requirement. The name check extends the stdlib-only `fuku/utils/claude` client.

**Core technologies:**
- gotgbot rc.36 (pinned): every Telegram surface v1.1 needs, including `LinkedChatId`, `DeleteMessages`, `GetUserProfilePhotos`, `ShowCaptionAboveMedia`, `LinkPreviewOptions` and the full service-message field set.
- PostgreSQL via GORM: the deletion queue, settings rows, greet-once ledger and requirement holds. Durable across restarts and Redis flushes.
- Redis (go-redis, miniredis in tests): input-wait state, pass cache, episode latches, pacers, AI verdict cache and budgets. Outside the `fuku:cache:` prefix.
- `fuku/utils/claude` extended with `ClassifyName`: same forced single-tool request, names only, no sampling parameters.
- `ratelimit.TelegramPacer`: one new instance for deletions and one for requirement lookups, each with its own Redis key family.

### Expected Features

**Must have (table stakes):**
- PM `/settings` list of manageable groups, paged, with live verification on pick; an empty state that explains `/admincache`.
- Owner gate: owner-only by default, one owner-only toggle that lets admins in; a regular admin without the toggle does not see the group at all. In-group commands stay unchanged.
- Rules, Welcome and Goodbye editor: text (entity-preserving), one media, URL buttons, per-part and full preview, media-under-text toggle, cancel and expiry.
- Welcome "greet once" and "delete last welcome" (the latter already exists as `clean_old`).
- Member requirements: last name, username, profile photo, linked-channel subscription, Russian, Arabic and Chinese name blocks. Each with an action of off, warn, kick, ban or mute. Shared toggles "check on join" and "delete messages".
- Notice in the group once per episode, auto-deleted; "I fixed it" only for mutes, with a live re-check and ownership of the mute.
- Deletion: service messages per type (immediate or after N seconds), bot messages after N minutes (welcome, goodbye, rules, automatic and manual punishments), members' commands, edited messages, self-destruct.
- Remaining pages: warnings, reports, language, log channel and categories, anonymous-admin mode, locks, blacklists, filters and notes (browse, delete, add by typing).

**Should have (differentiators):**
- Per-group state badges in the PM list (lockdown, Staff Group linked, bot not admin). Never names the Staff Group.
- AI spam-name check through Haiku, with a verdict cache keyed by the name hash.
- Presets plus stepper for delays; grace window for edits; per-rule status line when a rule cannot work (no linked channel, bot not channel admin, no key).
- Reuse the message editor as a generic "message slot" for filters and notes.

**Defer (if the milestone runs long):** self-destruct (volume and pacing risk), per-part custom notice text, AI spam names (needs an accuracy spike), grace window for edits, list search.

**Anti-features (out of scope, owner decided):** "must add members", global silence, mass deletion, delete-all, a draft or publish step, a topic picker, lifting a ban or kick through "I fixed it", mass re-check of existing members, sending message text to an AI, banning on an AI verdict by default.

### Architecture Approach

The settings menu stays stateless and keyed on `c` (private) or `u` (group). The owner gate is extracted into one function, `settingsAuthorize`, called from `verify`, the deep link, the in-group `/settings` check and the input consumer, so the rule cannot drift between entry points. The PM hub reads a projection table of admins, never the Telegram API. Requirement and deletion work runs on watchers that always return `ext.ContinueGroups`, with punishments and deletions executed through existing code paths. Background work uses the lockdown worker shape on every replica.

**Major components:**
1. `settingsAuthorize` (extracted from `verify`): live member check plus owner gate. One answer type used by every door.
2. Access and admin-index repository: the owner flag and the group-list projection, written from the admin-cache load path.
3. PM hub and private input state: the hub list (paged, database-only), a Redis await hash with a claim nonce, and a private-chat consumer that re-authorises before every write.
4. Message editor: shared Rules, Welcome and Goodbye pages, column-targeted apply functions, one `renderGreeting` send path used by join, goodbye, rules, Test and preview.
5. Member requirements: a join screen (called inside `ProcessSingleJoin`), a message watcher, one punisher (`applyMemberReqPunishment`) that reuses the warn core, kick, ban and mute paths, and a "I fixed it" handler.
6. Deletion: a scheduled-deletions table, a DB-driven worker with a pacer and a drain registered after the DB close, schedule hooks at the send sites, a processor-level hook for command messages, and an edited-message handler.
7. Claude name client (`ClassifyName`) and a bounded job pool for names (the pool shape is an open decision).

### Critical Pitfalls

1. **Two mute owners fight over one restriction.** Captcha, `/mute`, warn-limit, antiflood, staff `/mute` and a requirement mute all write the same `MutedPermissions`. "I fixed it" must be able to tell its own mute from anyone else's. Avoid by binding each requirement mute to a hold record and a marker `until_date` (or a ledger row), checked against the live status before any unmute. See decision 12.
2. **Automatic unmutes lifting a lockdown or bypassing the choke point.** Every unmute goes through `resolveUnmutePermissions`, and an error means do nothing. Whether the button refuses outright during a lockdown is an open decision (decision 13).
3. **Mass punishment from a broken or mis-set rule.** A Telegram error must be "unknown", not "fail". Use three-valued verdicts, a per-chat breaker that degrades a rule after repeated unknowns, a per-chat punishment rate cap, a two-tap confirm card when a rule is switched on with a punishment, and the standard exemptions (admins, approved users, bots, service IDs, `sender_chat`, pending captcha).
4. **One violator, many punishments.** Use one episode latch per (chat, user, rule), a notice cap, and an atomic first-insert that decides who punishes, so ten messages on three replicas give one warn and one notice.
5. **Per-message lookups and the 48-hour delete limit.** Cheapest checks first, a Redis pass cache (positive results only), singleflight for profile and channel lookups, a paced pool, and a durable delete queue that drops rows older than 47 h measured from the message's own send time.

Further pitfalls to carry into planning: input-wait state that is per-replica or single-slot (Pitfall 10), formatting and UTF-16 entity handling (Pitfall 11), 1024 and 4096 limits checked after placeholder expansion (Pitfall 12), deleting before a handler has replied (Pitfall 13), deleting alerts, staff cards, pins or admin messages (Pitfall 15), and a missing `can_delete_messages` right that looks like success (Pitfall 17).

## Implications for Roadmap

Stages below use the owner's order, which is also the dependency order. The roadmapper assigns final phase numbers, since the shipped phases run 1 to 9. Each stage must update `AGENTS.md` in the same commit (handler groups, callback namespaces, Redis key families, new tables and their backup status, `skipLocal` entries, the shutdown drain, the `test-postgres-integrity` list, the settings test toolkit, and the 8-locale statement).

### Stage 1: Hub and owner gate
**Rationale:** Nothing else can ship ungated, and the hub is the entry point for every later page. Pure extraction of `verify` steps plus one migration.
**Delivers:** Access table and admin-index projection (one migration), `settingsAuthorize` used by all four doors, `requireSettingsAccess` for `/settings`, the hub list (database-only read, paged, "All groups" row on Main), the Access page with the owner toggle, `settingsRightOwner`, the Main grid moved to two columns.
**Addresses:** PM list, owner gate, empty state, `/admincache` as the repair path.
**Avoids:** Owner flag served stale from another replica (Pitfall 6, `skipLocal` plus a two-instance test); anonymous admin mistaken for owner (Pitfall 7, gate after the proof swap); group existence leaking through refusal texts (Pitfall 8); empty hub for an owner who never spoke (Pitfall 9).
**Research flag:** Standard patterns. Decide the discovery source (decision 9) and the owner-flag table and cache policy (decision 10) first.

### Stage 2: Private input state and the message editor
**Rationale:** The input state is new infrastructure; the editor, the list pages and the log-channel set all use it. Rules gain media and buttons here, so the rules migration lives here too.
**Delivers:** Redis await store with a claim nonce, the private-only consumer with `/cancel`, an input registry registered by page, Rules/Welcome/Goodbye pages, `applyWelcomeX` and `applyRulesX` functions beside their commands (the commands keep their replies), greet once, delete last welcome, Test and full preview through the real send path, `validateGreeting` shared by editor and join.
**Addresses:** Editor table stakes, greet once, media under text, URL buttons, preview.
**Avoids:** Single-slot or process-local state (Pitfall 10), formatting injection and UTF-16 slicing (Pitfall 11), send-time limit failures (Pitfall 12), last-welcome and greet-once races (Pitfall 22).
**Research flag:** Needs a short spike. Entity-to-stored-form parity with `/setwelcome`; `show_caption_above_media` per media type; album handling (one media only, the rest ignored); the PM preview sending into the editor's chat.

### Stage 3: Member requirements
**Rationale:** Depends on Stage 1 for its page and on Stage 2 for the notice text. Cheap local rules first, then the paid lookups, then AI last.
**Delivers:** Rules table and per-kind holds, the join screen inside `ProcessSingleJoin`, the message watcher, the punisher (warn through the extracted `applyWarn`, kick with the goodbye suppression flag, ban, mute with the marker), the "I fixed it" handler, last-name, username and the three script rules; then photo and channel lookups with a pass cache; then the AI name job.
**Addresses:** All requirement rules, per-rule punishment, notice, "I fixed it", exemptions, the status line when a rule cannot run.
**Avoids:** Pitfalls 1 to 5, 17 to 21 and 23.
**Research flags:** Needs spikes before planning. `getUserProfilePhotos` for a hidden photo (zero versus error); `getChatMember` on a channel when the bot is not an administrator (exact error text; `chat_status.IsBotMissingError` treats "chat not found" as bot-missing, which is wrong for a channel lookup); the captcha interplay (decisions 12 and 14); the name pool (decision 8); a Haiku accuracy and cost spike through a `scripts/claude-spike`-style tool; a table-driven Unicode corpus (Ukrainian, Belarusian, Bulgarian, Russian, mixed script, zero-width).

### Stage 4: Message deletion
**Rationale:** Bot-message scheduling is needed by the requirement notice, so deletion can follow requirements, but the durable worker is the hard prerequisite. Self-destruct is last because it has the highest volume.
**Delivers:** Settings rows for delete rules, the `scheduled_deletions` table, the worker (tick plus wake channel, batch claim, `DeleteMessages` through a new pacer, stale-claim release, drain after DB close), service-message hygiene, bot-message schedule hooks at the send sites, member-command deletion, edited-message handler, then self-destruct.
**Addresses:** Service types with delay, bot messages after N minutes, members' commands, edited messages, self-destruct, the bot-right warning on the page.
**Avoids:** Deleting before the handler replied (Pitfall 13), deleting alerts, cards, pins and admin messages (Pitfall 15), storms and starvation (Pitfall 16), the invisible missing-right (Pitfall 17), `time.AfterFunc` timers (Pitfall 14).
**Research flags:** Needs research. `deleteMessages` partial-failure behaviour (an undeletable ID in a batch); the service-type policy table; the roughly fifteen manual-punishment reply sites in `bans.go`, `mute.go` and `warns.go`; the self-destruct storage decision (decision 6); the shutdown budget. Pitfall 30 warns that the 75 s stop grace period is nearly used by the existing drains, so the sum must be recomputed before this stage ships.

### Stage 5: Remaining pages
**Rationale:** Mostly the established pattern (extract each apply function, then one page). Lists add need the input registry from Stage 2, so they go last.
**Delivers:** Warnings (extract `applyWarnLimit` and `applyWarnMode`), reports, language (the menu redraws in the new language), log channel (categories are toggles; setting the channel stays a command flow), anonymous-admin mode (owner right), locks (paged, about 8 per page), then blacklists, filters and notes (paged browse, delete by row ID, add by typing).
**Addresses:** Every remaining page the owner listed.
**Avoids:** A second write path in a page (Pitfall 26), 64-byte callback overflow (Pitfall 28), locale overflow in longer languages (Pitfall 29).
**Research flags:** Standard patterns, except the list add flows (a filter is two-step: keyword in the await hash, then the reply content) and filter or blacklist validation (a pattern that matches everything).

### Phase Ordering Rationale

- The gate must exist before any page, so Stage 1 is first. Every later page inherits it through `verify`.
- The input state and the editor share one registry, so Stage 2 comes before lists and the log-channel set.
- Requirements use the editor's notice text and the deletion hook for their notices, so they come before deletion, but they can ship without it (the notice simply is not auto-deleted until Stage 4).
- Self-destruct is the highest-volume and pacing-risk item, so it is last among the deletion work.
- The owner's priority matches this order, and the architecture file's build order agrees with it.

### Research Flags

Phases needing deeper research during planning:
- **Stage 2 (editor):** entity and HTML parity with the existing command; `show_caption_above_media` per type; albums.
- **Stage 3 (requirements):** profile-photo privacy; channel lookups for a non-admin bot; captcha interplay; name-pool shape; Haiku accuracy and cost; Unicode corpus.
- **Stage 4 (deletion):** batch delete semantics; service-type policy; manual reply sites; self-destruct storage; shutdown budget.

Phases with standard patterns (skip research-phase):
- **Stage 1 (hub and gate):** extraction of existing steps; the discovery and cache decisions are small and listed below.
- **Stage 5 (remaining pages):** one apply function per setting, one page per file, as Phase 9 established. Only the list add flows need a short design pass.

## Decisions for phase research

The four files disagree on the points below. Each lists the options and each file's reasoning. None is decided here. The phase researcher for the relevant stage settles it and records the choice in that phase's context and in `AGENTS.md`.

### 1. Handler group of the private input consumer
- **STACK:** group 1. Commands run first in group 0 and end the update, so `/commands` never become input. Group 1 is free today.
- **FEATURES:** a new private-only position "before any PM handler that could swallow a free message". No number chosen.
- **ARCHITECTURE:** group -1, after the users tracker, before group 0. Slash text is excluded by the filter, so commands still run. Chosen so that a note or filter typed as input is consumed before group 0 watchers see it.
- **PITFALLS:** group 1, a private-only filter, `ContinueGroups` without state, `EndGroups` only on consumption.

### 2. Handler group and position of the member-requirements watcher
- **STACK:** a free slot (-5, -8, -9 or 1). Named as `-5` only for the deletion janitor.
- **ARCHITECTURE:** group -1, after the users tracker and before commands. Reason: a violator's commands are not served either. It runs after `-4` and `-3`, so raid detection still counts everything.
- **PITFALLS:** group 1, after group-0 commands, before aispam (3), always `ContinueGroups`. Reason: no new watcher may swallow an update, and commands should not be held back by screening.
- **Trade-off:** -1 blocks a violator's commands (ARCHITECTURE). Group 1 lets them run (PITFALLS). Both keep the update flowing to aispam and the rest.

### 3. Group of the service-message and self-destruct watcher, and where join and leave deletion lives
- **STACK:** a janitor watcher at `-5`, before group 0, so it sees a command that group 0 will later end. It enqueues rows and never ends an update.
- **ARCHITECTURE:** `messageHygieneWatcher` at `-1`, after the memberReq watcher. It also handles join and leave deletion, and the group 0 `cleanService` keeps only its join processing.
- **PITFALLS:** do not add a handler. Extend `cleanService` (group 0) with a per-type policy and delete only after `ProcessJoins` and after any reply that targets the message.
- **FEATURES:** join and leave must keep sharing storage with `/cleanservice`, and `cleanService` also drives join processing, so the refactor must not skip that path.

### 4. Edited-message handler mechanism and grace window
- **STACK:** `handlers.NewMessage(...).SetAllowEdited(true)`, deleting when `EditDate` exceeds `Date` by more than 5 s.
- **ARCHITECTURE:** a custom `handlers.Handler` that matches only `ctx.EditedMessage != nil`, at group 1. Reason: `SetAllowEdited` also matches every new message and forces an edit check onto the hot path.
- **PITFALLS:** group 1 as well. Skip `via_bot`, `sender_chat`, messages older than 47 h, and offer a grace window rather than deleting at the first edit.
- **FEATURES:** a grace window "a few minutes" by default, plus an age cutoff, because a link preview loading can also produce an edit.
- **Open:** the grace length (5 seconds versus minutes), and whether an unchanged-text edit counts (no reliable way to tell without storing text).

### 5. Mechanism for deleting members' commands
- **STACK:** the janitor enqueues the command message with a small delay of about 2 s, so the reply goes first. One delete path for everything.
- **ARCHITECTURE:** a wrapper around the dispatcher's `Processor` (wrapping `tracing.TracingProcessor`) deletes the command inline after every group has finished. It covers commands registered with raw `NewCommand`, needs no second command list, and skips anonymous admins, whose proof re-entry replies to the cached command.
- **PITFALLS:** do it at the end of the `WrapCommand` pipeline, after the handler returned, only for commands addressed to this bot, and set `AllowSendingWithoutReply` on replies that may target a deleted message. Also covers the anonymous-admin re-entry.
- **FEATURES:** delete after the handler returns, never at group -10, and exempt nothing else.

### 6. Storage for self-destruct
- **STACK:** PostgreSQL rows, the same queue as bot messages. Do not batch inserts until a measurement says so.
- **FEATURES:** PostgreSQL rows, batched inserts, a retention rule that deletes a row once handled. Calls it the highest-volume write in the milestone.
- **ARCHITECTURE:** PostgreSQL, with a bounded insert channel if measurement needs it. The scheduler's inserts join the same drain.
- **PITFALLS:** a Redis sorted set (`fuku:del:{chat}`), one pipelined `ZADD` per message, best effort, documented as lost on a Redis flush, with a per-chat minimum delay and `deleteMessages` batches.
- **Trade-off:** PostgreSQL survives a flush but costs a write per message. Redis is cheap but a flush silently leaves messages undeleted. The owner must accept the failure mode whichever is chosen.

### 7. Maximum delay and the age rule
- **STACK:** UI cap for self-destruct at 2820 minutes (47 h). The worker drops rows whose `sent_at` is older than 47 h 50 m, with no Telegram call.
- **FEATURES:** service messages at most 24 h; bot messages and self-destruct at 47 h, shown in the UI.
- **ARCHITECTURE:** a stored `CHECK delay_secs <= 86400` (24 h) for every kind. Rows more than 47 h after `created_at` are given up.
- **PITFALLS:** a 24 h maximum in the UI, and items older than 47 h expired.
- **Agreement to record:** the age must be measured from the message's own send time, not from when the row was written (`sent_at` in STACK). Only that matches Telegram's rule, and it matters after downtime when rows are stale. Whether the cap is 24 h or 47 h is open.

### 8. Pool for the name AI check
- **STACK:** a small separate pool (64 queued, 2 workers), derived from `raidAIContext()` so `StopRaidAI` cancels it, with its own key family (`fuku:req:nm:*`, `fuku:req:nv:*`) and budget helpers reused from Phase 7.
- **ARCHITECTURE:** a third job kind in the existing raid AI pool (128 queued, 4 workers), with its own breaker, hourly budget and verdict cache keys (`fuku:memberreq:*`). Saves a drain registration and a worker set. Flags that a 100-image raid would share workers with name checks, a coupling to judge in research.
- **PITFALLS:** separate small queue or a weighted share; never share the 4-worker queue, because a name burst would shed image checks. Its own budget family (`fuku:names:ai:*`).
- **Recommendation for research to test:** two of three files prefer a separate pool. Confirm with a load note.

### 9. Table for the PM group list and how it is fed
- **STACK:** `chat_admin_index (chat_id, user_id, status)`, PK `(chat_id, user_id)`, index on `user_id`. Filled from `loadAdminCacheFromTelegram` in a recovered goroutine, by `chat_member` promotions and demotions, and cleared on `my_chat_member` when the bot leaves. Only `creator` rows are listed by default.
- **ARCHITECTURE:** `chat_managers (chat_id, user_id, role, updated_at)`. Written through a package hook (`AdminCacheLoadedHook`) called after a successful `LoadAdminCache` store, so `utils/cache` never imports the database layer. The list is one SQL statement joining `chats` and the access table.
- **PITFALLS:** no new projection. Candidate chats come from `chat_users` (the users tracker), seeded when the admin cache loads and when the bot is added, with cache-only reads. Warns that a bot which is not an administrator cannot prove ownership, so that group must be shown as unknown rather than hidden.
- **FEATURES:** a persisted user-to-group admin index, with a `/reload` alias for `/admincache`, pruned by `my_chat_member`. Consistent with STACK and ARCHITECTURE.
- **Open:** the table name (`chat_admin_index` or `chat_managers`), and whether the list may be seeded from `chat_users` when the index is empty (PITFALLS).

### 10. Owner-gate table and read policy
- **STACK:** `chat_menu_access (chat_id PK, admins_allowed bool)`, zero means owner only. Read fresh and uncached, because it is an authority input and AGENTS.md keeps authority reads off the in-process cache.
- **ARCHITECTURE:** `chat_settings_access (chat_id PK, allow_admins bool, updated_by, updated_at)`. Read through `GetFromCacheOrLoad` with a `settings_access:` prefix in `skipLocal`, one column-targeted upsert, `DeleteCache` on write.
- **PITFALLS:** either works, provided the value is fresh or listed in `skipLocal` with `DeleteCache`. Requires a two-instance test that flips the toggle on one replica and presses on another.
- **Recommendation to test:** STACK's fresh read follows the repo's AGENTS.md rule for authority reads. ARCHITECTURE's cache adds up to 10 s of staleness per replica.

### 11. Row shape for requirement and deletion settings
- **Requirements, STACK:** one wide row per chat (`chat_requirements`) with one small-integer action column per rule (0 off, 1 warn, 2 kick, 3 ban, 4 mute), plus `check_on_join` and `delete_messages`. A `CHECK` on 0 to 4.
- **Requirements, ARCHITECTURE:** one row per (chat, rule) in `chat_member_rules`, with `punishment` and a `CHECK`. Absent row means off. Rejects a fixed 8-column layout for clobber and zero-value risk, and a new rule needs no migration.
- **Deletion, STACK:** one row per chat in `chat_cleanup_settings`, with seconds columns per service type and per-kind minutes.
- **Deletion, ARCHITECTURE:** one row per (chat, kind) in `chat_delete_rules`, with `delay_secs` and a `CHECK`. Kinds are not `CHECK`ed, so a new service type needs no migration.
- **Trade-off:** wide rows are one read and one cache entry. Narrow rows add a rule without touching the others. Both avoid clobbering if writes are column-targeted.

### 12. Ownership of a requirement mute, and the captcha pass
- **ARCHITECTURE:** an insert-first `member_requirement_holds` row. Muting uses a marker `until_date` (now plus 330 days, as the lockdown does). "I fixed it" unmutes only when the live `until_date` matches the hold within 2 s. No other mute path is touched. The captcha pass unmutes through `resolveUnmutePermissions` and relies on the next message check to catch a held member.
- **PITFALLS:** a DB ledger (`chat_id, user_id, rule, muted_at`), cleared by every other mute path (`/mute`, `/tmute`, warn limit, staff mute, blacklist, antiflood, `/unmute`, `/ban`, kick, leaving) through one `ClearRequirementMute` call. The captcha pass must check the ledger and keep the member muted.
- **FEATURES:** a bot-owned record is required, and a captcha pass must not silently unmute a member under a requirement mute.
- **Trade-off:** the marker approach avoids editing every mute module, but a staff or admin mute replaces the `until_date` and makes the hold unmatched, so "I fixed it" refuses, which is the safe side. The ledger approach needs changes in about ten places. Captcha behaviour differs between ARCHITECTURE and FEATURES and must be settled.

### 13. "I fixed it" during an active lockdown
- **ARCHITECTURE and FEATURES:** unmute through `resolveUnmutePermissions`, which returns the stored pre-lockdown permissions during a lockdown, so the member is free after the lift.
- **PITFALLS (Pitfall 2):** refuse outright while a lockdown is active ("an admin will let you in after the lift"), because self-service unmute is a weaker authority than an admin's `/unmute`.

### 14. Join screening relative to the captcha
- **ARCHITECTURE:** a `Screen` step in `ProcessSingleJoin`, after `Claim` and before the captcha decision. A violator never gets a challenge they cannot finish. A screened join ends the pipeline.
- **FEATURES:** run the join check after the captcha pass, on the shared pass path (`finishCaptchaPass`), so the two mutes never collide.
- **PITFALLS (Pitfall 23):** after the pass, or inside the same claim, and skip the welcome when the member is removed.
- **Open:** whether a failed join with action "mute" holds the welcome until the member passes (FEATURES suggests yes, a question for the owner).

### 15. Greet-once record and timing
- **STACK:** `chat_welcomed_users`, `INSERT ... ON CONFLICT DO NOTHING` at join; `RowsAffected == 1` means first time, which also dedupes the two join deliveries.
- **ARCHITECTURE:** `greeted_members` with the same insert, consulted only when greet-once is on, and recommends exactly-once through the conditional insert.
- **PITFALLS:** the claim runs after a successful send and only on the path where the member passes. A send error should roll the record back, so a failed send does not suppress the welcome forever.
- **Trade-off:** claim before send gives exactly-once but a failed send suppresses the welcome; claim after send can double-greet across replicas for a short window. Table name differs across files.

### 16. Callback namespaces
- **Hub list:** STACK reuses `settings|v1` (the list picks are existing `a=bk&c=` buttons). ARCHITECTURE adds `settingsh|v1` for the list itself, with `a=ls` and an offset cursor. Picks in both cases use the existing `settings|v1` pipeline.
- **"I fixed it":** STACK names it `req|v1` with `a=fx`. ARCHITECTURE names it `memreq|v1` with `a=fx`. PITFALLS uses `a=fx&u=&r=`.
- **Input and cancel:** ARCHITECTURE uses `a=in` and `a=ic` in `settings|v1`. STACK uses `a=xw` on its own state.

### 17. Name checks for established members
- **STACK:** check when the name hash changes, through a `fuku:req:nm:<user>` hash that also detects renames.
- **FEATURES:** check on join and on name change, with a verdict cached by the name hash.
- **PITFALLS:** for an established member, the first sight only records a baseline and never calls the model. Otherwise enabling the rule in a large group is a burst of calls.
- **Agreement to record:** the baseline rule from PITFALLS is compatible with the other two, so it is the proposed default for research to confirm.

### Naming conflicts to settle in the same pass
These do not change behaviour but must be consistent by the time code is written.
- Pending-input key: `fuku:pmwait:<user>` (STACK), `fuku:settings:await:<user>` (ARCHITECTURE), `fuku:editor:wait:{user}` (PITFALLS).
- Requirement Redis families: `fuku:req:*` (STACK), `fuku:memberreq:*` (ARCHITECTURE), `fuku:names:ai:*` (PITFALLS).
- Deletion pacer: `fuku:del:pace:*` (STACK) versus `fuku:delete:pace:*` (ARCHITECTURE).
- Delete kinds: `service`, `own_welcome`, `own_punish`, `command`, `edited`, `selfdestruct` (STACK) versus `service:<type>`, `bot:welcome`, `bot:punish_auto`, `member:commands`, `member:selfdestruct` and so on (ARCHITECTURE).
- Welcome ledger: `chat_welcomed_users` (STACK), `greeted_members` (ARCHITECTURE), `greeted` (PITFALLS).
- Scheduled claim: a single `UPDATE ... RETURNING` (STACK), or a batch claim with a `state` column and `claim_token` (ARCHITECTURE). Both are portable to SQLite. The batch variant is easier to reason about with several replicas.
- Stage 3 callback: see decision 16.
- Migration timestamps: the repo's latest migration must be read from `migrations/` at the start of each stage. STACK says after `20261101120000`; PITFALLS and AGENTS mention `20261030120000`.

## Confidence Assessment

| Area | Confidence | Notes |
|------|------------|-------|
| Stack | HIGH on the API surface, MEDIUM on runtime | Symbols and fields read from the gotgbot rc.36 source. Runtime behaviour (batch delete partial failure, profile-photo privacy, channel lookups by a non-admin bot) is unverified. |
| Features | HIGH on owner-visible UX, MEDIUM on GroupHelp semantics | The 11 owner screenshots are authoritative for layout. The meaning of the stepper glyph buttons and of "Own commands" is inferred. Edit-deletion false triggers are LOW. |
| Architecture | HIGH on integration seams, MEDIUM on two design choices | Read from the repo and the gotgbot dispatcher. The group-list source and the name-pool sharing are design choices with stated fallbacks. |
| Pitfalls | MEDIUM-HIGH on repo facts, MEDIUM on Telegram | Repo facts read from code. Telegram behaviour tagged [TG] came from prior knowledge; `core.telegram.org` was unreachable. |

**Overall confidence:** MEDIUM. The design is sound and reuses known patterns, but several Telegram behaviours decide design points and must be confirmed by live spikes before the relevant stage is planned.

### Gaps to Address

Live Telegram checks (owner-run, like the Phase 8 and Phase 9 checklists):
- **Profile photo privacy:** what `getUserProfilePhotos` returns for a user who hides the photo (zero or an error). Use a test account. Stage 3.
- **Channel subscription:** the exact error from `getChatMember(channel, user)` when the bot is not an administrator of the channel, and whether "chat not found" must be treated as "unknown", not "bot missing". Stage 3.
- **Batch delete:** the behaviour of `deleteMessages` when one ID in the batch is too old or undeletable. Plan a per-ID fallback that isolates the bad row. Stage 4.
- **Edited-message noise:** how often `edited_message` arrives for a link preview or caption touch without an author edit. Stage 4.
- **Caption placement:** `show_caption_above_media` for photo, video and animation, and that uploaded media with captions over 1024 characters is refused before send. Stage 2.
- **Rate limits:** the per-group message limit (about 20 per minute, from [TG]) for notices during a raid. Stage 3.

Owner questions (not blockers for research, but they change the design):
- A group with no live creator: admins allowed (FEATURES suggests yes, still live-checked) or owner-only deny (ARCHITECTURE implies deny). Stage 1.
- Should the Staff Group be exempt from deletion kinds (summaries and cards)? FEATURES and PITFALLS say yes. Stage 4.
- A member who fails a requirement on join with action mute: hold the welcome until they pass? FEATURES suggests yes. Stage 3.
- After "I fixed it" in a captcha-enabled group, send the member through the join challenge again, or let them in? ARCHITECTURE recommends the challenge. Stage 3.
- "Own commands" in the scheduled-deletion screenshot: the bot's replies to commands? Not in the owner's list, so the default is to leave it out. Stage 4.
- Stepper glyph buttons in the GroupHelp screenshots: the proposal is "type a number" through the input state. Stage 2.
- Name-AI bar (0.85 or 0.9) and the default punishment for the AI rule (warn or mute recommended, never ban by default). Stage 3.

Repo-side items to settle when the stage starts:
- Locale set: the owner has decided on 8 locales (en, es, fr, hi, id, pt, ru, uk). `AGENTS.md` and `PROJECT.md` still say 7 and must be updated in the same commit as the first new key. Confirm with `make check-translations`.
- Shutdown budget: the new deletion drain (5 s plus 1 s grace) and the already-registered drains must still fit the 60 s budget and the `stop_grace_period: 75s` manifest value. Recompute before Stage 4.
- Requirement mute and captcha pass interplay (decision 12) must be decided before Stage 3 planning.
- Backup: the access table, admin index, requirement holds, deletion queue and greet ledger must stay out of backup, export, import and reset, as the staff and lockdown tables do. Greetings and rules gain columns that travel in backup JSON.

## Sources

### Primary (HIGH confidence)
- `/home/user/Fuku_Robot/AGENTS.md`, `/home/user/Fuku_Robot/.planning/PROJECT.md`: repo rules, the owner's v1.1 decisions, and the current milestone scope.
- `/home/user/Fuku_Robot/.planning/research/STACK.md`, `FEATURES.md`, `ARCHITECTURE.md`, `PITFALLS.md`: the four research files this summary synthesises.
- gotgbot v2.0.0-rc.36 source in the module cache: `gen_types.go` (service-message fields, `LinkedChatId`, `LinkPreviewOptions`, `UserProfilePhotos`, `ChatMemberRestricted.IsMember`), `gen_methods.go` (`DeleteMessage`, `DeleteMessages`, `GetUserProfilePhotos`, `GetChatMember`, `ShowCaptionAboveMedia`), `ext/dispatcher.go` (handler groups, `Processor`), `ext/handlers/message.go` (`SetAllowEdited`).
- Repo code cited by the research files: `fuku/utils/claude`, `fuku/modules/overwrite.go`, `lockdown_worker.go`, `raid_ai.go`, `raid_ai_image.go`, `fuku/utils/cache/adminCache.go`, `fuku/modules/greetings.go`, `membership.go`, `settings_menu.go`, `settings_pages.go`, `chat_permissions.go`, `fuku/utils/ratelimit/telegram_pacer.go`.
- Owner's GroupHelp screenshots 1 to 11 (UX reference for visible layout), as summarised in FEATURES.md.

### Secondary (MEDIUM confidence)
- Bot API behaviour recalled from prior knowledge (48-hour delete rule, `deleteMessages` batch of 100, profile-photo privacy, channel `getChatMember` requiring admin rights, caption limits after entity parsing), tagged [TG] in PITFALLS and flagged in STACK. Needs confirmation against the live docs.
- Search summaries of Rose and Shieldy documentation (`/cleanservice`, `/cleancommand`, `/cleanwelcome`, `1inch/shieldy` issue 52). Unofficial and partially unreachable.

### Tertiary (LOW confidence)
- GroupHelp, Combot and Group Butler behaviour beyond the owner's screenshots: no usable sources found. No claim in this summary depends on them.
- Edit-deletion false triggers, profile-photo privacy return values and the channel-admin error text: listed as gaps for live spikes.

---
*Research completed: 2026-11-02*
*Ready for roadmap: yes, once the Decisions for phase research are assigned to their stages*
