# Phase 13: Member Requirements - Research

**Researched:** 2026-11-05
**Domain:** Telegram bot (Go 1.26, gotgbot v2 rc.36, GORM, Redis, PostgreSQL): a per-group screening engine (name and username rules) with one punishment per episode across replicas, a join hook shared by the captcha and no-captcha paths, a self-service unmute owned by a marker end date, and a new settings page
**Confidence:** HIGH for everything grounded in repo code, the gotgbot module source and the Unicode probe run in this session; MEDIUM for Telegram-side behaviour that needs a live client (core.telegram.org does not resolve from this session: `getaddrinfo ENOTFOUND core.telegram.org`; see the Assumptions Log)

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

#### Join, welcome and the "I fixed it" button (owner decisions from the roadmap)
- **D-01:** The captcha runs before the requirement check on join. The join check sits on the one shared pass path (`finishCaptchaPass` after `claimCaptchaPass`) and, in a group with no captcha, on the join itself before the welcome; both callers reach one shared entry point so the two paths cannot drift. The lockdown guard (group -7) still runs first, and a captcha-failed or lockdown-banned joiner never reaches the check. A member is never given a captcha challenge because of a requirement, and a member who passed the captcha is not sent through it again by "I fixed it".
- **D-02:** A member muted on join gets no welcome until "I fixed it" succeeds; the held welcome is then sent through `SendWelcomeMessage`, so Greet once (Phase 12) claims the member only when the welcome really goes out. A kicked or banned member gets no welcome. A warned member is welcomed as usual after the warn. A member muted later by the message check was already welcomed, so "I fixed it" sends no welcome for that hold.
- **D-03:** Only a mute gets the "I fixed it" button. A kick lets the member rejoin and be checked again, a ban waits for an admin, a warn only warns. The button is a callback in its own namespace through `callbackcodec`, at most 64 bytes, carrying numbers only, usable only by the held member.

#### Several rules at once and the confirm tap (owner choices, 2026-11-05)
- **D-04:** A member who breaks several rules at once is one episode: the strictest punishment among the broken rules is applied once (ban over kick over mute over warn) and the one notice names every broken rule. Fixing one of several rules leaves the member held for the others: "I fixed it" re-checks every rule live and unmutes only when none is still broken.
- **D-05:** On the Member requirements page, choosing kick, ban or mute for a rule draws the shared two-tap Yes/No card (`settings_card.go`, lazy five-minute expiry, Yes through the whole press pipeline) and writes nothing until Yes; off and warn change at once. The card says the rule acts on every member at their next message.

#### Storage and the one-punishment guarantee
- **D-06:** One wide row per chat, `chat_requirements`, with one small-integer action column per rule (0 off, 1 warn, 2 kick, 3 ban, 4 mute) under a `CHECK` 0 to 4, plus `check_on_join` and `delete_messages` booleans. The model has no `default:` tag on its booleans (the GORM zero-value trap), writes are column-targeted upserts that write a Go false, every write deletes the cache keys it affects, and a chat with no row means everything off. Phases 14 and 15 add their own rule columns by their own migrations. The row is read through `cache.GetFromCacheOrLoad` because the watcher reads it on every group message; whether its key goes into `skipLocal` is settled in research.
- **D-07:** One hold row per `(chat, user)` is both the episode latch and the mute record: the rules broken, the punishment applied, the mute's marker `until_date`, whether a welcome is held, the notice message ID. The row is inserted first with `INSERT ... ON CONFLICT DO NOTHING` and the one caller with `RowsAffected == 1` punishes and posts the notice, so ten messages on three replicas give one punishment and one notice. A message check that finds the member clean deletes a non-mute hold so a later break is a new episode; a rejoin or a leave deletes the hold. The hold is read fresh from the database, never cached, never part of backup, export, import or reset, and works without Redis.
- **D-08:** The requirement mute is owned by a marker end date, as the lockdown's ban is: `restrictChatMember` with `until_date` of now plus 330 days (a finite date Telegram keeps). "I fixed it" unmutes only when a live `getChatMember` shows the member restricted with an `until_date` matching the hold's marker within 2 seconds; a mute that an admin, a staff action, the captcha, antiflood or a lockdown placed replaces that end date, so the button refuses and changes nothing. No other mute path is edited. A captcha pass never unmutes a member who has an active hold.
- **D-09:** "I fixed it" during an active lockdown unmutes through `resolveUnmutePermissions`, which returns the stored pre-lockdown permissions, as MREQ-06 words it ("respecting an active lockdown"); an error there means nothing is changed and the member is told. The hold is deleted by a conditional delete before the unmute, so two taps on two replicas act once.

#### The rules (MREQ-13)
- **D-10:** The name rules judge the first and last name, never the username. Russian: the letters ы, ъ, э, ё (either case) in either name. Arabic: any letter of the Arabic script. Chinese: any Han ideograph. The page says these are letter-based and can also catch related languages (Belarusian uses ы and ё, Bulgarian uses ъ; Persian and Urdu use the Arabic script; Japanese kanji are Han). Ukrainian uses none of ы, ъ, э, ё, so a Ukrainian name must never be caught by the Russian rule, and the corpus asserts it (corrected 2026-11-05 from the UI contract's finding, A11). A last name made only of invisible or whitespace-like characters (zero-width, joiners, the Braille blank, the Hangul filler and similar) counts as no last name. "Must have a username" goes by the `username` field. A table-driven corpus pins the Unicode behaviour (Ukrainian, Belarusian, Bulgarian, Russian, mixed script, zero-width).
- **D-11:** Never checked: admins, approved users, bots, Telegram service accounts (`staffServiceUserIDs`), messages with `sender_chat` set (a channel or an anonymous admin), the bot itself, and a member with a pending captcha. A check that cannot be answered (an unreadable setting or admin state) never punishes anyone.

#### The watcher, punishments and notices
- **D-12:** One message watcher in its own handler group before the commands (group 0), after the raid flood counter (-4) so raid detection still counts everything, settled to an exact free slot in research. It returns `ext.ContinueGroups` on every path and never ends an update, so the commands, aispam, antiflood, locks and blacklists still see the message. Checks run cheapest first: chat type, sender exemptions, the cached settings (every rule off ends it), a positive-only Redis pass cache keyed by a hash of the names, the pure name rules, then the hold insert and the punishment. The rules of this phase need no Telegram call.
- **D-13:** Punishments reuse the existing paths: a warn goes through the warn module's own path and counts toward the group's warn limit and its warn action; a kick is the member-removing unban and a ban a `banChatMember`, as the commands send them; a mute is `restrictChatMember` with the D-08 marker. Each violation posts one notice in the group naming the member (escaped, never trusted) and the rules broken (MREQ-05). The group message rate limit during a raid is a research flag: notices are paced or capped per chat, and a per-chat cap on punishments per minute is considered, so a mis-set rule cannot mass-punish without a visible stop.
- **D-14:** With "Delete messages" on, every message of a member who breaks a rule is deleted, including further messages of a member who is already held (they add no warns, notices or punishments). A failed delete is only logged. A kicked member can rejoin and is checked again; a banned member stays banned until an admin unbans them (MREQ-08).
- **D-15:** The Redis key family is `fuku:memberreq:*`, outside the `fuku:cache:` prefix so `CLEAR_CACHE_ON_STARTUP` keeps it. Nothing the requirements need for correctness lives only in Redis.

#### The page and compatibility
- **D-16:** The Member requirements page is registered from its own file through `registerSettingsPage` and joins the Main grid by its `Order`, with its state line on Main. It reads through a side-effect-free loader that creates no row. The right of a punishment control is settled in the UI contract (the proposal is the creator or an administrator with `can_restrict_members`, as the lockdown triggers). The page row limits (a private page at most 8 rows, a group-menu page 9), the 3800 UTF-16 and 64-byte limits in all 8 languages and the rule that no screen names the Staff Group still hold; every new string goes into all 8 locale files.
- **D-17:** One migration with a timestamp greater than `20261105120000`, one transaction, no top-level BEGIN or COMMIT, `fuku/db/migrations/runner.go` and `scripts/migrate_psql.sh` agreeing, models with `TableName()`, the new tables added to the AutoMigrate lists of the test harnesses and listed in AGENTS.md as outside backup, export, import and reset. Each setting has one apply function beside where its command would be (there is no command: the page's apply functions carry drift and authority tests instead of a parity test).
- **D-18:** The tests cover: the Unicode corpus; the one-punishment guarantee across three replicas (`otherReplica`) and ten messages; the strictest-wins rule and the single notice; the confirm card for kick, ban and mute; "I fixed it" refusing to lift an admin, staff, captcha, antiflood or lockdown mute and unmuting through `resolveUnmutePermissions` during a lockdown; the held welcome and Greet once; the exemptions; the page's size, row and callback limits in 8 languages; and test hygiene (every test deletes the users, cache, hold and settings rows it writes, with fresh chat IDs, because CI runs all of `fuku/modules` in one PostgreSQL database).

### Claude's Discretion
- The table and column names within D-06 and D-07, the exact handler group, the callback namespace name, the locale keys and the page's key letter, `Order` and layout within the row limits, the pass-cache lifetime, the notice pacing and the punishment cap numbers, the order of the migration statements.

### Deferred Ideas (OUT OF SCOPE)
- The profile-photo rule, the channel-subscription rule and the page's lines that say a rule cannot work in a group: Phase 14. The AI spam-name rule: Phase 15. Automatic deletion of requirement notices: Phase 16.
- A command to set the rules (the page is the only way); grace periods for existing members when a rule is switched on (the requirement says every member is checked at their next message, and the confirm card is the safeguard).
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| MREQ-01 | A page gives each rule its own punishment (off, warn, kick, ban, mute); all off by default. This phase: last name, username, Russian, Arabic, Chinese | S-06 (storage, no-row = all off), S-10 (page, controls, card, rights, allow-list), S-11 (tests that change) |
| MREQ-02 | "Check on join" checks a new member right after the captcha (or on joining with no captcha); every member is checked at their message | S-01 (group -2 watcher), S-02 (hot path), S-03 (shared join entry on both paths) |
| MREQ-03 | "Delete messages" deletes a violator's messages | S-02 (job step 17), S-05 (held path deletes), S-07 (paced deletes) |
| MREQ-04 | Punished once; later messages add nothing until fixed or rejoined; across replicas | S-05 (insert-first hold, clean predicate, stale reclaim, leave and rejoin), S-02 (pass cache) |
| MREQ-05 | One notice naming the member and the rule; a warn counts toward the warn limit | S-04 (`applyWarn` core, notice variants), S-07 (notice pacing, cap) |
| MREQ-06 | "I fixed it" only for the member; live re-check; unmute respecting a lockdown; held welcome; else told what is missing | S-09 (ten steps), S-03 (welcome held flag), S-04 (unmute permissions) |
| MREQ-07 | "I fixed it" never lifts an admin, staff, captcha, antiflood or lockdown mute | S-04 (marker), S-09 step 4 and the until_date table, S-03 (captcha choke-point guard) |
| MREQ-08 | A kicked member can rejoin and is checked again; a banned member stays banned | S-04 (kick = member-removing unban, ban = permanent `banChatMember`), S-05 (leave deletes the hold) |
| MREQ-09 | Admins, approved users, bots, service accounts, `sender_chat` messages never checked | S-02 (exemption order, tri-state checks), S-01 (gotgbot drops bots) |
| MREQ-13 | Russian ы ъ э ё, Arabic script, Han; letter-based; invisible last name counts as none | S-08 (classifier, NFC, invisible set, probe output) |
</phase_requirements>

## Project Constraints (from CLAUDE.md / AGENTS.md)

- Stay on Go 1.26.0, gotgbot v2, GORM (PostgreSQL in production, SQLite in tests), Redis. No new dependency: `golang.org/x/text` is already a direct requirement (`go.mod:27`).
- Migrations append-only, timestamped above every existing file (latest `20261105120000_add_rules_media_and_greet_once.sql`), one transaction, no top-level `BEGIN`/`COMMIT`; `runner.go` and `scripts/migrate_psql.sh` must agree (a plain new file needs no applier change, as Phase 12's commit `4553cb1` shows).
- Every write `cache.DeleteCache`s the keys it affects; freshness-critical keys go in `skipLocal` (`fuku/db/cache/local.go:55-71`).
- Commands go through `helpers.WrapCommand` (this phase adds no command). Watchers return `ext.ContinueGroups`; a group-0 `nil` kills later group-0 handlers.
- Locale keys in all 8 files (en, es, fr, hi, id, pt, ru, uk); every `settings_` key has one row in `fuku/i18n/settings_locale_test.go`.
- Fire-and-forget goroutines start with `defer error_handling.RecoverFromPanic(...)`.
- Tests: `-tags testtools`, real fixtures (no mock libraries), `make test`; PostgreSQL-only guarantees join `make test-postgres-integrity` (`Makefile:39-44`, anchored `-run` list plus package list).
- Security: never skip the per-group admin check; callback data is never authority; logs carry no message text, names or file IDs (AGENTS "Go rules", the editor's no-content logging rule).
- Telegram limits: fan-out respects rate limits and reports partial failures; callback data at most 64 bytes through `callbackcodec`.
- AGENTS.md is updated in the same commit as any change that invalidates a rule in it.
- GSD workflow: all edits through `/gsd-execute-phase`.

## Summary

The engine is small in code but sits on five existing subsystems (handler groups, the join path, the captcha unmute, the warn module and the settings menu), and every one of them has a sharp edge this research found in the source. The watcher goes into handler group **-2**: no number between -4 and 0 is free (-3 staff watchers, -2 admin cache, -1 users tracker), but group -2 holds only a `my_chat_member` handler (`bot_updates.go:273-275`), so for every message update the watcher is alone in its group, runs before the commands and after the raid flood counter, and cannot be skipped by a neighbour. gotgbot's dispatcher (`ext/dispatcher.go:280-327`) treats `nil` and `ContinueGroups` identically for a handler alone in its group, and `EndGroups` ends everything, so the watcher returns `ContinueGroups` on every path.

The hot path makes no Telegram call and no database write for a clean member: chat type and sender shape (pure), the cached settings (one Redis GET once the key joins `skipLocal`; most chats stop here with every rule off), the positive-only pass cache (one Redis GET), and only on a miss the pure rules plus one conditional `DELETE` of a stale hold. The pass cache is worth keeping, but for a different reason than D-12 states: the rules cost microseconds; what the cache saves is the PostgreSQL `DELETE` the clean path must issue (D-07) on every message. A violation hands the member to a bounded async job, which settles the exemptions that need a lookup (approved, pending captcha, a live `getChatMember` that is also the authoritative admin check and the source of the current names), inserts the hold first, punishes once through a new fleet-wide pacer, posts one notice and deletes the message. A per-chat cap (10 punishments a minute, then a 10-minute pause with one `mreq_notice_paused`) keeps a mis-set rule visible and keeps the group under Telegram's 20-messages-a-minute limit.

Four findings change the plan relative to the CONTEXT and UI contract. (1) `ctxTr` in a group callback is the **group's** language, not the presser's (`lang/repository.go:31-54`), so the UI contract's "presser's language" rule for "I fixed it" answers cannot be met with `ctxTr`; recommend the group language. (2) A muted member's leave and rejoin are **invisible** to the greetings `chat_member` handlers, because `ExtractJoinLeftStatusChange` returns `false, false` when old and new status are both `restricted` (`chat_status.go:584-586`); a mute hold must therefore survive a leave and be resolved at the next join, where the marker mute is lifted if it is still live. (3) The captcha's own unmute has six call sites, all through `unmuteCaptchaUser` (`captcha.go:1630-1641`); D-08's "a captcha pass never unmutes a held member" belongs there, and the guard re-applies the hold's own marker instead of skipping. (4) `IsUserAdmin` and `IsUserApproved` both turn a failed read into "not admin" / "not approved" (`chat_status.go:217-226`, `approvals/repository.go:56-58`), which would punish on an unknown state; the job needs tri-state variants.

**Primary recommendation:** build the engine as one shared executor (`memberReqEnforce`) called synchronously by the join entry and through a bounded async runner by the group -2 watcher, with the hold row as the only latch, the marker `until_date` as the only proof of ownership, and every Telegram call of the engine behind a dedicated `fuku:memberreq:pace:*` pacer.

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Rule settings (per chat) | Database (`chat_requirements`) | Cache (`member_requirements` key, `skipLocal`) | Durable, read on every group message, must change on every replica at once |
| Episode latch and mute record | Database (`chat_requirement_holds`, insert-first) | none | Must work without Redis and across replicas (D-07, D-15) |
| Pass cache, cap window, pause latch, pacer | Redis (`fuku:memberreq:*`) | in-process fallback (pacer only) | Load shedding and speed only; nothing for correctness (D-15) |
| Name rules | Bot process (pure Go) | none | No Telegram call; `unicode` tables plus NFC |
| Exemptions needing state | Bot process via cache/DB/Telegram | none | Admin: cached list, then a live `getChatMember`; approvals and captcha tri-state |
| Punishment, notice, delete | Telegram Bot API (paced) | Database (warn rows) | Reuse warn core, kick, ban, mute with marker |
| Join check | Greetings join path and captcha finish | Bot process | One shared entry, before the welcome |
| "I fixed it" | Callback handler (`mreq|v1`) | Database (hold), Telegram (live checks) | Presser-only, conditional delete, `resolveUnmutePermissions` |
| Page | Settings menu (`settings|v1`, page `m`) | Database | Existing press pipeline, card, rights |

## Standard Stack

### Core (all existing, no new dependency)
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `github.com/PaulSonOfLars/gotgbot/v2` | v2.0.0-rc.36.0.20260919140833-240296efadb4 [VERIFIED: go.mod] | handlers, `RestrictChatMember`, `BanChatMember`, `UnbanChatMember`, `GetChatMember` | The bot's client |
| `gorm.io/gorm` | v1.31.2 [VERIFIED: CLAUDE.md stack] | models, `clause.OnConflict{DoNothing: true}`, column-targeted upserts | Every repository |
| `github.com/redis/go-redis/v9` | v9.22.0 | pass cache, cap, pause, pacer | Existing client |
| `golang.org/x/text/unicode/norm` | v0.42.0 [VERIFIED: go.mod:27, module cache] | NFC before the Russian rule (decomposed ё) | Already a direct requirement; used by `scripts/generate_docs` |
| `unicode` (stdlib) | Unicode 15.0.0 in Go 1.26 [VERIFIED: probe printed `unicode.Version 15.0.0`] | `unicode.Arabic`, `unicode.Han`, categories, `Other_Default_Ignorable_Code_Point` | Standard script tables |
| `fuku/utils/ratelimit` | in repo | `NewTelegramPacer` with own keys | The fleet-wide pacer every fan-out uses |

### Supporting
| Library | Purpose | When to Use |
|---------|---------|-------------|
| `github.com/alicebob/miniredis/v2` | Redis in tests | every module test |
| hand-written `lockdownFake` / `staffActionFake` | Bot API fake with stateful restrict/ban/unban | punishments, "I fixed it", replicas |
| `internal/testdb` | SQLite with `_busy_timeout=10000&_journal_mode=WAL` (`internal/testdb/database.go:31`) | repository tests; concurrent inserts serialize instead of failing |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| NFC | NFKC | NFKC also folds compatibility forms; every script rule already catches the compatibility forms (Arabic presentation forms, Kangxi radicals, CJK compatibility ideographs are in the script tables, probe), so NFKC adds risk and no catch |
| Redis pass cache | in-process expirable LRU | zero network per hit, but each replica pays its own miss; D-12 chose Redis and the hot path already pays Redis per message for raid flood settings |
| async runner for messages | synchronous punishment in the watcher | a paced call can wait up to 60 s (`telegram_pacer.go:20`) and would hold one of 200 dispatcher goroutines per violator during a raid |

**Installation:** none.

## Package Legitimacy Audit

No external package is installed by this phase. `golang.org/x/text` is already pinned in `go.mod` and `go.sum` (`golang.org/x/text v0.42.0 h1:JbOZXgfeCPU9gacVtYliJqOhD+zhrEqK4LfdpmlUZqI=`), so the legitimacy seam has nothing to check.

| Package | Registry | Verdict | Disposition |
|---------|----------|---------|-------------|
| (none new) | - | - | - |

**Packages removed due to [SLOP] verdict:** none. **Packages flagged as suspicious [SUS]:** none.

## Settled Findings (what the planner builds on)

### S-01. The watcher's handler group: -2

**Groups in use** [VERIFIED: grep of every `AddHandlerToGroup` and `handlerGroup:` in `fuku/modules`, files read]:

| Group | Owner | Source |
|-------|-------|--------|
| -10 | captcha pending-message holder | `captcha.go:2373` `handlers.NewMessage(nil, captchaModule.handlePendingCaptchaMessage), -10` |
| -7 | lockdown join guard (four handlers) | `lockdown.go:34` `handlerGroup: -7`, `lockdown_guard.go:603-617` |
| -6 | fed-ban | `federations.go:35` `fedHandlerGroup      = -6` |
| -4 | raid flood counter | `raid_flood.go:48` `const raidFloodGroup = -4` |
| -3 | staff watchers (migrate, ownership, chat_member, my_chat_member) | `staff_watchers.go:24` `handlerGroup: -3`, `:171-188` |
| -2 | admin cache auto-update, **my_chat_member only** | `bot_updates.go:273-275` `handlers.NewMyChatMember(chat_status.ExtractAdminUpdateStatusChange, adminCacheAutoUpdate), -2` |
| -1 | users tracker (`message.All`) and `botJoinedGroup` (my_chat_member) | `users.go:57` `handlerGroup: -1`, `users.go:208`, `bot_updates.go:276-284` |
| 0 | commands, greetings, help, settings callbacks | `dispatcher.AddHandler` |
| 1 | settings typed-reply door | `settings_input_door.go:25` `const settingsInputGroup = 1` |
| 2 | raid AI candidate watcher | `raid_ai.go:42` `raidAIGroup = 2` |
| 3 | aispam | `aispam.go:41` `aispamHandlerGroup = 3` |
| 4 to 11 | antiflood 4, locks 5/6, blacklists 7, reports and reactions 8, filters 9, pins 10, log channel 11 | module structs |
| 12 | late raid flood counter | `raid_flood.go:54` `const raidFloodLateGroup = 12` |

Free numbers: -9, -8, -5 and anything below -10; **none between -4 and 0**.

**Dispatcher semantics** [VERIFIED: `ext/dispatcher.go:280-327` in the module cache]: for each group, handlers are tried in registration order; a handler whose `CheckUpdate` is false is skipped; `errors.Is(err, ContinueGroups)` continues **within the current group** (`// Continue handling current group.`); `errors.Is(err, EndGroups)` returns at once (`// Stop all group handling.`); `nil` falls to the trailing `break`, i.e. moves to the next group (`// Handler matched this update, move to next group by default.`); any other error goes to `d.Error`, and the bot's `dispatcherErrorHandler` (`main.go:414`) decides. So for a handler that is the only match in its group, `nil` and `ContinueGroups` both reach the next group; only a second matching handler in the same group tells them apart. `TestRegisteredModulesRespectGotgbotDefaultGroupSemantics` (`registry_test.go:124-153`) pins this.

**Neighbours:** the users tracker's `logUsers` has exactly one return, `return ext.ContinueGroups` (`users.go:204`). The admin-cache handler at -2 is a `NewMyChatMember` handler, so it never matches a message update. The staff watchers at -3 match only migrate and ownership service messages and member updates and always return `ext.ContinueGroups` (`staff_watchers.go:126-127`).

**Recommendation: group -2, as the only message handler of that group** (`memberReqWatchGroup = -2`). It runs after -4 (raid flood still counts first), after -3, before -1 and 0, so a violator's command is checked and still served (the watcher never ends the update). No earlier handler in its group can match a message, so nothing can skip it, and its own `ContinueGroups` cannot be mistaken for `nil`. Rejected: -1 (shares the group with the users tracker; correct today, but a future `nil` there would silently disable screening), -5 (free, but before the raid flood counter, contrary to D-12), 1 (after the commands: a slash command that ends the update would never be screened).

**Handler filter** [VERIFIED: `ext/handlers/message.go:21-27, 62-99`]: `handlers.NewMessage(filter, response)` defaults to `AllowEdited: false` and `AllowChannel: false`, and `CheckUpdate` drops a sender whose `IsBot()` is true unless `AllowBot` (`if ctx.EffectiveSender != nil && ctx.EffectiveSender.IsBot() && !m.AllowBot { return false }`). `Sender.IsBot()` is `s.Chat == nil && s.User != nil && s.User.IsBot` (`sender.go:137-139`), so a `sender_chat` post is **not** dropped by gotgbot and the watcher must reject it itself. Use `handlers.NewMessage(func(m *gotgbot.Message) bool { return m.Chat.Type == "supergroup" }, memberReqModule.memberReqWatcher)` with no `SetAllow*`: new messages only, no edits, channel posts, business or guest messages, no bot senders, private chats never match.

**AGENTS.md to re-check:** the Handlers group list ("`-2` admin-cache") becomes "`-2` admin-cache (my_chat_member) and the member requirements watcher (messages only; the only message handler of the group, `ext.ContinueGroups` on every path)"; the module list sentence about watchers; and a line that no future message handler may be added to group -2 ahead of it.

### S-02. The hot path, exemptions and the pass cache

**Watcher order (synchronous, on the update goroutine):**

1. Filter: supergroup (S-01). Basic groups are out of scope this phase (`RestrictChatMember` is "to restrict a user in a supergroup" and `UnbanChatMember` "in a supergroup or channel" [VERIFIED: gotgbot `gen_methods.go:4643, 8304`], so mute and kick cannot work there; Phase 14's "cannot work here" line covers it).
2. User content: reuse `raidFloodItemKey(msg)` and take its `ok` (`raid_flood.go:134-163`): it excludes `NewChatMembers`, `LeftChatMember`, `PinnedMessage`, title/photo changes, group creation and migration, and anything with neither text nor a listed media kind; slash texts are content (`slash` true), so commands are screened.
3. Sender shape: reuse `raidFloodSenderCounts(b, ctx)` (`raid_flood.go:199-213`): rejects `sender == nil`, `sender.Chat != nil`, `msg.SenderChat != nil`, `msg.IsAutomaticForward`, non-positive IDs, the bot itself and `staffServiceUserIDs` (`staff_action_decide.go:513` `var staffServiceUserIDs = map[int64]bool{777000: true, 1087968824: true, 136817688: true}`). Bots never get here (gotgbot drops them).
4. Settings: `memberreq.GetSettings(ctx, chat)` through the cache (S-06). Error: return (never punish). No rule on: return. Most chats end here.
5. Pass cache GET (below). Hit: return.
6. Pure rules on `msg.From` (S-08) with only the rules whose action is not Off. Clean: run the clean predicate (S-05) with the message date, then SET the pass value, return.
7. Violation: `cachedAdminStatus(chat, user)` (`antiflood.go:106-121`, reads only the cached admin list, returns `known, isAdmin`); known and admin: return. Then hand the case to the async runner (S-07) and return. Shed (runner full): return; the next message retries.

**Job order (async, recovering, counted):**

8. Approved: a new error-returning reader, `approvals.IsUserApprovedContext(ctx, chat, user) (bool, error)`, through the same cache key `cache.CacheKey("approvals", chatID)` (`approvals/repository.go:46`), so no new invalidation duty. Error: return. Today's `IsUserApproved` hides the loader error (`GetApprovedUsersContext` returns `nil` on error, `repository.go:56-58`), which would read as "not approved".
9. Pending captcha, tri-state: `captcha.HasPendingCaptchaAttemptsContext(ctx, chat)` then `captcha.GetCaptchaAttempt(user, chat)`; any error: return; an attempt: return. `aispamCarvedOut` (`aispam.go:238-248`) cannot be reused as is: when `GetCaptchaAttempt` errors it falls through to "not carved out".
10. Hold read fresh: a hold exists and is not a stale pending one: held path (delete the message when Delete messages is on, nothing else), return.
11. Pause latch exists (S-07): return.
12. Live `getChatMember(chat, user)` through the pacer, bounded: error: return; creator or administrator: return; `left` or `kicked`, or `restricted` with `is_member` false: return; re-run the rules on the live record's names (rename race): clean: return. This one call is the authoritative admin check (the cached list can be 30 minutes stale and `IsUserAdmin` returns false on a failed `getChat`, `chat_status.go:217-226`) and gives the current names (needed anyway by the join check and "I fixed it").
13. Cap window INCR (S-07); over the cap: start the pause, return.
14. Claim the hold (insert-first) or reclaim a stale pending one (S-05). Lost: held path, return.
15. Punish (S-04). Refused or unknown: per the failure table (S-05). Applied: mark punished.
16. Notice (S-07), store its message ID on the hold.
17. Delete the triggering message when Delete messages is on (also when the punishment was refused: the violation is established, deletion is independent).

**Cost per group message** [ASSUMED: latencies are typical LAN figures, not measured here]:

| Case | Work | Network round trips |
|------|------|---------------------|
| chat with no rule on (most chats) | steps 1-4 | 1 Redis GET (settings, `skipLocal`) |
| member already known clean | steps 1-5 | 2 Redis GETs |
| first message after a rename or settings change, clean | steps 1-6 | 2 Redis GETs, 1 PostgreSQL DELETE by unique key, 1 Redis SET; rules are microseconds (probe) |
| held member (warned, or unmuted by an admin) | 1-10 in job | settings + pass + approvals/captcha (cached) + 1 PostgreSQL SELECT, + 1 deleteMessage when on |
| new violation | 1-17 | the above + 1 `getChatMember` + punishment (1 call, a warn at the limit adds its limit action) + 1 `sendMessage` + optional `deleteMessage` |

**Pass cache verdict: keep it, narrowed.** Without it, step 6's clean predicate is a PostgreSQL statement on every message of every member in every chat with a rule on (D-07 requires the clean check to end a non-mute hold); with it, that statement runs once per member per name change, settings change or TTL. The rules themselves are not worth caching. Shape: key `fuku:memberreq:pass:<chat>:<user>` (per user, so a key per active member, never per name), value the first 16 hex characters of `SHA-256(rule signature | first | last | username)` where the rule signature is the five action values in rule order (so turning a rule on or changing a punishment invalidates every entry without a delete), `SET ... EX 1800` (30 minutes, Claude's discretion), written only after a clean verdict **and** a successful clean predicate. A hit compares the stored value with the current hash; anything else is a miss. A Redis error is a miss. Without Redis the clean predicate simply runs every message (correct, slower). Positive-only means a violation is never cached, so a fix is seen at once.

### S-03. The join hook: one shared entry, a reset at the join, a guard in the captcha unmute

**The two paths today** [VERIFIED: files read]:
- No captcha (and approved members in a captcha group, and `ErrChallengeDisabled`): `newMember` (`greetings.go:969-986`, `chat_member`) and `cleanService` (`greetings.go:1092-1138`, `new_chat_members` service message) both call `ProcessSingleJoin` (`membership.go:71-102`), which takes `claimRecentJoinProcessing` (`greetings.go:87-124`, `fuku:recentJoinProcessing:<chat>:<user>`, `var recentJoinProcessTTL = 5 * time.Second`), so of the two deliveries of one join exactly one continues, then calls `deps.Welcome(user)`, the closure in `membershipDepsForJoin` (`greetings.go:1051-1078`) that calls `SendWelcomeMessage(bot, &ctxCopy, user.Id, user.FirstName)`. The closure receives the full `gotgbot.User` from the update.
- Captcha: `claimCaptchaPass` (`captcha.go:1673-1693`) claims the attempt and unmutes through `restoreCaptchaPermissions`; `finishCaptchaPass` (`captcha.go:1697-1745`) posts the summary, deletes the challenge, posts the success note and ends with `SendWelcomeMessage(bot, in.Ctx, in.UserID, in.FirstName)` (`:1742`). Both pass paths reach it: the button (`captcha.go:1842-1850`, `FirstName: user.FirstName` from `query.From`) and the web pass (`captcha_web.go:303-325`, `FirstName: user.FirstName` from `captchaweb.InitData`, which carries only `UserID`, `FirstName`, `LanguageCode`, `StartParam`, `AuthDate`, `initdata.go:31-37`).
- Join requests: an approval (auto or the Accept button) makes the member join, which arrives as the same `chat_member` update and service message, so there is no third path.
- Lockdown guard (-7) and fed-ban (-6) end the update for joiners they handle, so group 0 never runs for them (D-01 holds without code).

**Recommendation: one entry, `memberReqJoinThenWelcome(bot, ctx, user, src)`**, called from (a) the `Welcome` closure of `membershipDepsForJoin` instead of `SendWelcomeMessage`, and (b) the last line of `finishCaptchaPass` instead of `SendWelcomeMessage`. It: reads the settings; when Check on join is off or no rule is on, sends the welcome and returns; otherwise makes one live `getChatMember` (the web pass has no last name or username, and the live record is the authoritative admin check for both paths), runs `memberReqEnforce` synchronously with `src=join` under a 15 s context, and then sends the welcome through `SendWelcomeMessage` **only** when the outcome is not "removed" (kick or ban applied) and not "held" (mute applied). A warn sends its notice first, then the welcome (D-02). A refused or unanswerable check sends the welcome (never punish, never withhold). The live call is made only when a rule is on and Check on join is on, so joins pay nothing otherwise.

**The reset at the join (D-07 "a rejoin deletes the hold"), with a correction.** Add a `Reset func(user gotgbot.User)` dep to `MembershipDeps`, called by `ProcessSingleJoin` after a won `Claim` and before `DecideJoin`, filled by `membershipDepsForJoin` with the join's own date (`ctx.ChatMember.Date`, or `msg.Date` for the service message). It deletes the holds of `(chat, user)` **created before the join** (`DELETE ... WHERE created_at < joinDate RETURNING *`), so a hold the message watcher created for this very membership a moment earlier survives. When the deleted hold was a punished mute and the member's live status is still `restricted` with that marker, the reset lifts the mute through the same unmute core "I fixed it" uses (S-09 steps 6, 7, 9). This is required because:
- A requirement-muted member who leaves and rejoins is **invisible to the `chat_member` join and leave handlers**: `ExtractJoinLeftStatusChange` returns `false, false` whenever the old and new status strings are equal (`chat_status.go:584-586` `if oldMemberStatus == newMemberStatus { return false, false }`), and a restricted member leaving or rejoining stays `restricted` with only `is_member` flipping [ASSUMED A3, A4]. Only the `new_chat_members` service message (`cleanService`) sees the rejoin.
- Telegram keeps a supergroup restriction across a leave [ASSUMED A3], so deleting the hold without lifting the mute would leave the member muted for 330 days with no button.

**Leave (D-07 "a leave deletes the hold"):** call `memberreq.ClearHolds(chat, user)` in `leftMember` (`greetings.go:988-1049`) right after `clearRecentJoinProcessing` (`:997`). It runs for every leave the existing filter detects (`wasMember && !isMember`): a kick (member or restricted to `left`), a ban (`kicked`), a member leaving. Deleting a mute hold there is safe, because a detected leave means the status changed away from `restricted`, i.e. the restriction is gone. A muted member's own leave is not detected (above) and is handled by the join reset. `lockdownOnKicked` (`lockdown_guard.go:565-568` and below) ends the kicked update of a lockdown ban before `leftMember`; that member was never screened, so nothing is lost.

**The captcha guard (D-08 "a captcha pass never unmutes a held member").** Every captcha unmute goes through `unmuteCaptchaUser` (`captcha.go:1630-1641`): the pass (`:1692` via `restoreCaptchaPermissions`), the five-minute retry worker (`:2303`), the restart recovery release (`:301`), the captcha-off cleanup (`:613`), `rollbackCaptchaAttempt` (`:1402`) and the failure-action restore (`:1549`). Put the guard there, once: read the hold fresh; on a read error return the error (every caller already treats an unmute error as "retry later": the pass writes the `captcha_muted_users` row due now, `restoreCaptchaPermissions` `:1643-1650`); when a punished mute hold exists, call `restrictChatMember` with `MutedPermissions` and `until_date` = the hold's own marker instead of unmuting, and return nil. Re-applying the same absolute marker keeps "I fixed it" valid (its step 4 compares against that marker). `finishCaptchaPass` then reaches the join entry, which finds the mute hold, sets `welcome_held` on it (one conditional update by ID) and sends no welcome. How a hold can exist at pass time: the reset deleted older holds, so only a hold created between the join and the attempt row (a joiner who posts before `SendCaptcha` inserts the attempt, so the -10 holder and the pending-captcha exemption did not see one) reaches the guard. Rare, but it is exactly the D-08 case.

**Kicked, banned, held welcome and bookkeeping:** a member kicked or banned by the join check gets no welcome and no greet-once claim (the claim is inside `SendWelcomeMessage`, `greetings.go:924-929`); the kick or ban then fires `leftMember`, which clears the join dedupe key, the captcha rows and the scheduled captcha unmute, and posts the goodbye when goodbye is on (parity with `/kick`, the captcha kick and the warn-limit kick; see Open Question 9). The clean-welcome `last_msg_id` is untouched because no welcome was sent.

**When the bot cannot restrict:** the punishment is refused with a 4xx, nothing is recorded as punished, the hold stays pending as a backoff record (S-05), the welcome is sent, no notice is posted.

### S-04. Punishments

**Warn: extract a context-free core from `warnThisUser`** (`warns.go:95-250`). Today the function, in order: replies `warns_admin_warning_error` to an admin (`:108-112`); live `chat.GetMember` (`:114-120`); takes `warnPunishLock(chat.Id, userId)` (`:121-123`, 32 in-process shards); reads `warns.GetWarnSettingContext` (`:124`); `warns.WarnUser(userId, chat.Id, reason)` (`:125`, which also creates the chats and users rows through `EnsureChatInDb` and `EnsureUserInDb`, `fuku/db/warns/repository.go:92-97`); deletes for `dwarn`/`swarn` (`:135-144`); at the limit runs `kickMember` (`:150`), `chat.RestrictMember(b, userId, MutedPermissions, nil)` (`:159-162`) or `chat.BanMember(b, userId, nil)` (`:171`), returning the error **before** resetting when the action fails (`:153-156, 165-168, 174-177`), then `warns.ResetUserWarns` (`:184`); builds the reply (limit texts with the reasons list, or `warns_user_warning` plus the Remove warn and Rules buttons) and sends it as a reply (`:231-243`). Reply sites to keep out of the requirement path: `:110`, `:128`, `:231`.

Smallest refactor that keeps every warn command byte-identical: move lines 121-187 into

```go
// proposed, in warns.go
type warnApplyResult struct {
	Count, Limit int
	Mode         string   // the group's warn mode read under the lock
	LimitReached bool
	LimitApplied bool     // the limit action took effect and the warns were reset
	LimitErr     error    // the limit action failed; the warn stays counted, nothing reset
	Reasons      []string
}

// applyWarn records one warn and runs the limit action under warnPunishLock. afterRecord
// runs inside the lock right after the warn is stored (dwarn/swarn deletion), so
// warnThisUser keeps its exact order. It sends nothing.
func applyWarn(ctx context.Context, b *gotgbot.Bot, chat *gotgbot.Chat, userID int64,
	reason string, afterRecord func()) (warnApplyResult, error)
```

`warnThisUser` keeps lines 95-120 and 188-249 and maps the result to the same replies (a `WarnUser` error is the `common_settings_save_failed` reply; `LimitErr` is `return err` as today; an unknown mode keeps the reasons-only text). The requirement path calls `applyWarn(ctx, b, chat, userID, mreq_warn_reason, nil)` and posts its own notice: below the limit `mreq_notice_warn` with `Count` and `Limit`; at the limit with `LimitApplied` the `mreq_notice_warn_limit_<Mode>` variant; at the limit with `LimitErr` the plain `mreq_notice_warn` (never a removal it did not make). The warn-limit mute is the warn module's own permanent mute (until 0) and gets no button (UI contract). Regression gate: the existing warn and blacklist tests unchanged (`TestWarn*`, `TestSilentWarn*`, `TestDeleteWarn*`, `TestRemoveWarn*`, `TestRmWarn*`, `TestResetAllWarns*`, `TestLoadWarns*` in `warns_command_test.go`; `TestBlacklist*`, because `blacklists.go:572` calls `warnThisUser`).

**Kick:** `kickMember` is `b.UnbanChatMember(chatID, userID, &gotgbot.UnbanChatMemberOpts{OnlyIfBanned: false})` (`bans.go:27-30`); the Bot API defines this as removing a current member ("if the user is a member of the chat they will also be removed", gotgbot `gen_methods.go:8304`). Add `kickMemberContext(ctx, ...)` beside it (the same request with `UnbanChatMemberWithContext`) so the pacer's context bounds it, and let `kickMember` call it.

**Ban:** `BanChatMemberWithContext(ctx, chat, user, nil)` with empty options, as `/ban` and the warn limit send it (`warns.go:171`): no end date (permanent), no `revoke_messages`.

**Mute:** `RestrictChatMemberWithContext(ctx, chat, user, MutedPermissions, &gotgbot.RestrictChatMemberOpts{UntilDate: marker})` with `MutedPermissions` (`chat_permissions.go:14-29`, every `CanSend*` false, `CanManageTopics: helpers.Ptr(false)`) and no `UseIndependentChatPermissions` (as every other mute). Marker: reuse the lockdown span, `const lockdownBanSpan = 330 * 24 * time.Hour` (`lockdown_marker.go:35`), computed once and stored on the hold **before** the call. Telegram treats a restriction of more than 366 days or less than 30 seconds as forever ("If user is restricted for more than 366 days or less than 30 seconds from the current time, they are considered to be restricted forever.", gotgbot `gen_methods.go:4635`), so 330 days stays finite. Ownership test, mirroring `isLockdownBan` (`lockdown_marker.go:20-29`, `const lockdownBanUntilTolerance int64 = 2`): `m.Status == gotgbot.ChatMemberStatusRestricted && m.IsMember && hold.MuteUntil != 0 && abs(m.UntilDate - hold.MuteUntil) <= 2`. `MergedChatMember` carries `UntilDate` and `IsMember` (gotgbot `gen_types.go:2091-2093`; `ChatMemberRestricted.UntilDate` `:2585`).

**What every other mute path writes as `until_date`** (so the marker can never be confused with them) [VERIFIED: files read]:

| Path | Call | until_date |
|------|------|------------|
| `/mute`, `/smute`, `/dmute` | `c.Chat.RestrictMember(c.Bot, t.userID, MutedPermissions, nil)` (`mute.go:133, 148, 185`) | 0 (forever) |
| `/tmute` | `&gotgbot.RestrictChatMemberOpts{UntilDate: _time}` (`mute.go:96-97`) | the chosen duration |
| staff mute and tmute | `RestrictChatMemberWithContext(..., MutedPermissions, &gotgbot.RestrictChatMemberOpts{UntilDate: newUntil})` (`staff_action_run.go:657-658`) | 0 or the duration |
| captcha challenge and re-mute | `RestrictMember(bot, userID, MutedPermissions, nil)` (`captcha.go:1291, 1622, 2332`) | 0 |
| antiflood | `chat.RestrictMember(b, userId, MutedPermissions, nil)` (`antiflood.go:360`) | 0 |
| warn limit | `chat.RestrictMember(b, userId, MutedPermissions, nil)` (`warns.go:159-162`) | 0 |
| blacklists | `b.RestrictChatMember(chat.Id, user.Id(), MutedPermissions, nil)` (`blacklists.go:510`) | 0 |
| lockdown | no per-member restriction; the chat's default permissions; its bans are `kicked` with `ban_until` (`lockdown_worker.go:420`) | n/a |

Only a `/tmute 330d` or a staff mute of 330 days sent within 2 s of the requirement mute could carry an equal end date; accepted (durations are whole minutes and the coincidence needs the same second).

**Refusals:** classify by code with `isTelegramClientError` (`settings_gate.go:256-262`: a `*gotgbot.TelegramError` with code 400-499 other than 429). A 4xx means Telegram did nothing ("not enough rights", "user is an administrator of the chat", "method is available only for supergroups" and similar wordings [ASSUMED: wordings; the classifier never reads them]). A pacer `ErrRateLimited` (`telegram_pacer.go`) also did nothing. A 5xx, a timeout or a dropped connection is unknown. Nothing is recorded as punished unless Telegram answered success; the treatment of each case is in S-05.

### S-05. The hold: schema, order, failures

**Table `chat_requirement_holds`** (name per discretion):

| Column | Type | Meaning |
|--------|------|---------|
| `id` | BIGSERIAL PK | the `h` of the "I fixed it" button |
| `chat_id`, `user_id` | BIGINT NOT NULL, UNIQUE `(chat_id, user_id)` as `uk_chat_requirement_holds_chat_user` | one episode per member |
| `rules` | INTEGER NOT NULL, `CHECK (rules > 0)` | bitmask of the broken rules (1 last name, 2 username, 4 Russian, 8 Arabic, 16 Chinese; Phases 14 and 15 take the next bits) |
| `action` | SMALLINT NOT NULL, `CHECK (action BETWEEN 1 AND 4)` | punishment applied (1 warn, 2 kick, 3 ban, 4 mute) |
| `mute_until` | BIGINT NOT NULL DEFAULT 0 | the marker, unix seconds; 0 unless a mute |
| `welcome_held` | BOOLEAN NOT NULL DEFAULT FALSE | a join mute held the welcome |
| `notice_msg_id` | BIGINT NOT NULL DEFAULT 0 | the notice, set after the send |
| `created_at` | TIMESTAMPTZ NOT NULL DEFAULT NOW() | claim time (written from Go, as `ClaimGreeting` does) |
| `punished_at` | TIMESTAMPTZ NULL | set only after Telegram (or the warn write) confirmed |

**Repository `fuku/db/memberreq` (fresh reads, no cache, no `DeleteCache`):**
- `ClaimHold(ctx, hold) (won bool, id int64, err error)`: `db.DB.Clauses(clause.OnConflict{Columns: chat_id,user_id, DoNothing: true}).Create(row)`, `won = RowsAffected == 1` (the `ClaimGreeting` pattern, `fuku/db/greetings/greeted.go:24-32`).
- `ReclaimStaleHold(ctx, chat, user, staleBefore, fresh) (won bool, id int64, err error)`: one conditional `UPDATE ... SET created_at = now, rules, action, mute_until, welcome_held, notice_msg_id = 0 WHERE chat_id = ? AND user_id = ? AND punished_at IS NULL AND created_at < ?`, `won = RowsAffected == 1`. The stale age is 2 minutes, `memberReqStaleClaim`, mirroring `lockdownStaleClaim`.
- `MarkHoldPunished(ctx, id)`, `SetHoldNotice(ctx, id, msgID)`, `SetHoldWelcomeHeld(ctx, id)`.
- `TakeHoldByID(ctx, id, chat, user) (*Hold, bool, error)`: `DELETE ... WHERE id = ? AND chat_id = ? AND user_id = ? RETURNING *` ("I fixed it" step 8; one of two taps wins). SQLite 3.35+ supports `RETURNING` (gorm sqlite uses it); if the planner prefers, read then delete with `WHERE id = ?` and check `RowsAffected`.
- `RestoreHold(ctx, hold)`: `INSERT` with the explicit `id` and every column as read, `ON CONFLICT DO NOTHING` (no target, so either the PK or the unique key conflicts quietly). PostgreSQL accepts an explicit value in a BIGSERIAL column; the sequence is untouched and the old ID was never reused.
- `ClearHoldOnClean(ctx, chat, user, msgDate)`: the clean predicate below.
- `ClearHolds(ctx, chat, user)` (leave) and `TakeHoldsBefore(ctx, chat, user, joinDate)` (join reset).
- `GetHoldFresh(ctx, chat, user)`, `GetHoldByIDFresh(ctx, id)`.

**The clean predicate (resolves the UI contract against D-07).** D-07 says a clean message deletes a non-mute hold; the UI contract (section 8, the paragraph on a member who is no longer restricted) says the next clean message deletes the mute hold an admin's `/unmute` left behind. Both hold if the clean statement is:

```sql
DELETE FROM chat_requirement_holds
WHERE chat_id = ? AND user_id = ? AND created_at < ?msgDateMinus2s
  AND (action <> 4
       OR (punished_at IS NOT NULL AND punished_at < ?msgDateMinus2s)
       OR (punished_at IS NULL AND created_at < ?staleBefore))
```

A member muted by our marker cannot send a message, so a clean message dated more than 2 s after the mute was confirmed proves someone lifted it; a message older than the hold (out-of-order processing across 200 dispatcher goroutines, a rename between two quick messages) never deletes it. The 2 s margin covers the Telegram-to-server clock difference [ASSUMED A12].

**Order of a won episode and what each failure leaves:**

| Step | Failure | Leaves behind |
|------|---------|---------------|
| claim (insert-first or stale reclaim) | DB error | nothing; next message retries |
| cap INCR (before the claim) | over the cap | nothing; pause latch (S-07) |
| punishment | 4xx or `ErrRateLimited` (definitely not applied) | the pending hold with `created_at` refreshed to now: a backoff record, reclaimable after 2 minutes, so a bot without rights makes one Telegram call per violator per 2 minutes, not one per message; no notice; a join gets its welcome. Exception: "user is an administrator" class is not detectable by code, so it backs off like the rest; the live check of step 12 normally exempts admins before |
| punishment | 5xx, timeout, connection (unknown) | mute: verify with one live `getChatMember`; marker present means applied (continue), absent means not (backoff), unreadable leaves the pending hold for the 2-minute reclaim, whose next attempt re-checks the marker first. Kick or ban: keep the pending hold; if it applied, the leave update clears it; if not, the reclaim retries (both calls are idempotent). Warn: a `WarnUser` error is a rolled-back transaction (`db.DB.Transaction`, repository.go:99), so backoff |
| mark punished | DB error | logged; the hold stays pending and the 2-minute reclaim would re-punish; for a mute it re-checks the marker first (no second mute, but a second notice is possible); for a warn a second warn is possible. Accepted: it needs a DB failure right after a DB success |
| notice | Telegram refusal or timeout | one retry; then the punishment and the hold stay, one Warn line with the chat and user IDs only (UI contract accepted limit). Pacing never drops a mute notice (S-07) |
| store notice ID | DB error | one retry, then a Warn; the button answers `mreq_fixed_stale`; an admin can `/unmute` |
| delete message | any | logged only (D-14) |

**PostgreSQL-only guarantees and `make test-postgres-integrity`:** add `TestRequirementHoldClaimConcurrent` (N goroutines claim one `(chat, user)`, exactly one wins, one row; the reclaim of a stale pending hold has one winner) and `TestChatRequirementsSettings` (the five `CHECK`s reject 5 and -1 with SQLSTATE 23514, column-targeted upserts of two rules at once never clobber, a Go 0 and a Go false are written) to the anchored `-run` list, and `./fuku/db/memberreq` to the package list (`Makefile:41-44`). Optionally `TestMemberRequirementsMigration` in `./fuku/db/migrations` (statements run twice without error, constraint names exist), as Phase 12's `TestRulesMediaMigrationDefaults` did.

### S-06. Storage

**`chat_requirements`** (one row per chat, D-06): `id BIGSERIAL PK`, `chat_id BIGINT NOT NULL` with `CONSTRAINT uk_chat_requirements_chat UNIQUE (chat_id)`, five `SMALLINT NOT NULL DEFAULT 0` columns (`last_name_action`, `username_action`, `russian_action`, `arabic_action`, `chinese_action`) each with its own named `CHECK (... BETWEEN 0 AND 4)` (`chk_chat_requirements_last_name` and so on), `check_on_join BOOLEAN NOT NULL DEFAULT FALSE`, `delete_messages BOOLEAN NOT NULL DEFAULT FALSE`, `created_at`/`updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()`. Inline constraints in `CREATE TABLE IF NOT EXISTS`, the style of `20261104120000_add_settings_hub_tables.sql` and `20261018130000_add_chat_raid_detection.sql` (both read): the Go runner rewrites a top-level `ALTER TABLE ... ADD CONSTRAINT` into a `duplicate_object`-swallowing block (`runner.go:540-577`), so inline is simpler and identical in both appliers.

**Model** (`fuku/db/models/member_requirements.go`): no `default:` tag on any field; the actions as `int` with `type:smallint;not null;check:chk_chat_requirements_last_name,last_name_action BETWEEN 0 AND 4` (GORM builds the CHECK on SQLite too, AGENTS Testing; the raid model does the same, `models/raid_detection.go:17`); `TableName()` returns `"chat_requirements"`; the hold model likewise with `TableName()` `"chat_requirement_holds"`.

**Reads:** `memberreq.GetSettings(ctx, chatID)` through `cache.GetFromCacheOrLoad(ctx, cache.CacheKey("member_requirements", chatID), cache.DefaultCacheTTL, loader)` where the loader returns a zero row (all off) with the chat ID on `gorm.ErrRecordNotFound` and the wrapped error otherwise (the `raiddetection.loadSettings` pattern, `raiddetection/repository.go:83-99`); it creates no row. `memberreq.GetSettingsFresh(ctx, chatID)` is the same query without the cache, for the card's same-value check (UI contract section 2) and the apply read-back.

**Writes:** `SetRuleAction(ctx, chatID, rule, action)`, `SetCheckOnJoin`, `SetDeleteMessages`, each one `INSERT ... ON CONFLICT (chat_id) DO UPDATE` of its own column plus `updated_at` (`clause.OnConflict{Columns: chat_id, DoUpdates: clause.AssignmentColumns(cols)}`, `raiddetection/repository.go:125-140`), then `cache.DeleteCache(key)`. A value outside 0..4 returns an error before any write. A first write inserts zeros for the other columns, which is "off".

**`skipLocal`: yes.** Add `memberRequirementsPrefix = CacheKey("member_requirements") + ":"` to `local.go:44-48` and to the `skipLocal` return (`:67-70`) with the reason "a rule switched off must stop punishing on every replica at the next message, not after CACHE_LOCAL_TTL". Precedent: raid detection settings are in the list for the same reason and are read per message in supergroups (`raid_flood.go:257-262`). Cost: one Redis GET per supergroup message (S-02). With `skipLocal` the cached read is fresh enough for the page; the card's same-value comparison still uses `GetSettingsFresh` because the UI contract fixed it.

**Rolling deploy:** the key is new, so no old cached value exists. Phases 14 and 15 add columns: an older cached value decodes them as zero (off), the safe direction.

**AutoMigrate lists to extend** (AGENTS "Adding a module" step 4): `fuku/modules/test_harness_test.go` (list at `:329-378`), a new `fuku/db/memberreq/testmain_test.go` (`testdb.Run(m, &models.ChatRequirements{}, &models.ChatRequirementHold{})`, the `menuaccess` pattern), and `fuku/db/backup/testmain_test.go` (`:41-48`) for the backup test. Extend `settingsCleanup` (`settings_env_test.go:192-218`) with both tables and the cache key, so every settings test also clears them.

**Backup exclusion:** `AllExportableModules` (`fuku/db/backup/types.go:115-135`) is an explicit list, so the tables are out by construction. Prove it as Phase 12 did with `TestGreetedMembersStayOutOfBackup` (`fuku/db/backup/greeted_backup_test.go:112-152`: export holds only backup modules, exporting the table name is an error, an import creates no row, a reset leaves the rows): add `TestMemberRequirementsStayOutOfBackup` with both tables. Requirement warns are ordinary warns and do travel in backup (correct: they are warns).

**Migration file:** `migrations/20261106120000_add_member_requirements.sql` (greater than `20261105120000`), the two `CREATE TABLE IF NOT EXISTS` statements and nothing else, a header comment in the Phase 12 style naming both tables as outside backup, export, import and reset.

### S-07. Notices, rate, pacing, the cap and shutdown

**Telegram's group limit:** "In a group, bots are not be able to send more than 20 messages per minute." [CITED: core.telegram.org/bots/faq, via a search result snippet; the page itself does not resolve here, A1]. Existing pacers are fleet-wide, not per group: `telegram_pacer.go:17-21` defaults (`defaultPacerInterval = 100 * time.Millisecond`, `defaultPacerMaxRetries = 3`, `defaultPacerMaxWait = 60 * time.Second`), one Redis slot key and one shared block key per pacer; `lockdownPacer` (`fuku:lockdown:pace:next`, `fuku:lockdown:pace:block`, `lockdown_worker.go:32-38`) and `staffActionPacer` (`staff_action_run.go:173`). A 429 from one group blocks the whole pacer for `retry_after`.

**Recommendations:**
- A new pacer `memberReqPacer` with `NextKey: "fuku:memberreq:pace:next"`, `BlockKey: "fuku:memberreq:pace:block"`, `Interval: 100 * time.Millisecond`, `MaxRetries: 3`, `MaxWait: 60 * time.Second`, through `memberReqPaced(ctx, call)`. Every Telegram call of the engine uses it (the live `getChatMember`, the punishment, the notice, the delete, the "I fixed it" calls). The warn limit's own kick/mute/ban inside `applyWarn` stays unpaced, as the warn module sends it today.
- **Keep the per-chat cap** (A15 resolved: keep): a fixed 60 s window, `INCR fuku:memberreq:{<chat>}:cap:<unix minute>` plus `EXPIRE 120` in one pipeline, counted before the claim. More than `memberReqCapPerMinute = 10` starts a pause: `SET fuku:memberreq:{<chat>}:paused 1 NX EX 600` (`memberReqPause = 10 * time.Minute`); the one caller whose `SET NX` succeeded posts `mreq_notice_paused` with `{n}` 10 and `{minutes}` 10 (best effort, paced). While the latch exists no new episode starts; existing held members' messages are still deleted when Delete messages is on. Without Redis there is no cap (accepted, like the raid counters). Numbers: 10 punishments a minute stays under half the group limit, leaving room for welcomes, goodbyes and the bot's other replies; a raid that trips it is the lockdown's job (join surge, flood), not screening's. Both numbers are package variables only so tests can shrink them. This adds the conditional 25th `mreq_` key.
- **A mute notice is never dropped by pacing:** when `memberReqPaced` returns `ErrRateLimited` for a mute notice (no request was made), wait out the block and try again, bounded by the job context; the one Telegram-refusal retry of the UI contract is separate.
- **Async runner:** the watcher's job runs on `memberReqRunAsync(job)`, the `raidPostAsync` shape (`raid_trigger.go:127-134`: a counter started before the goroutine, `defer t.finish()`, `defer error_handling.RecoverFromPanic(name, module)`), with a cap of 32 jobs in flight (`memberReqMaxInFlight`); over the cap the job is dropped and the next message retries. Tests wait with the tracker's `wait()`, as raid tests use `raidPosts.wait()`.
- **Shutdown:** register `RegisterMemberReqDrain` (wait 2 s, plus `shutdownDrainGrace`) in `main.go` right before `modules.RegisterStaffActionsDrain(shutdownManager)` (`main.go:225`), so LIFO runs it after the staff drain (it never takes from that drain's 31 s) and before the database close, as `StopAdminIndexSweeper` is placed (`main.go:209-221`). It cancels the runner context first. Everything resumes from the database: a cut-off claim is a pending hold the 2-minute reclaim retries; a lost notice is the accepted limit above. The join path and "I fixed it" run on update goroutines, which the dispatcher and HTTP stop already drain.

### S-08. The rules

**Classifier (proved by a throwaway probe in this session, Go 1.26.0, Unicode 15.0.0, deleted afterwards):**

```go
// verified by the probe
var russianOnly = map[rune]bool{'ы': true, 'ъ': true, 'э': true, 'ё': true, 'Ы': true, 'Ъ': true, 'Э': true, 'Ё': true}

func hasRussian(s string) bool { // NFC first: е + U+0308 becomes ё
	for _, r := range norm.NFC.String(s) { if russianOnly[r] { return true } }
	return false
}
func hasArabic(s string) bool { // a letter of the Arabic script
	for _, r := range s { if unicode.Is(unicode.Arabic, r) && unicode.IsLetter(r) { return true } }
	return false
}
func hasHan(s string) bool { // any Han-script code point
	for _, r := range s { if unicode.Is(unicode.Han, r) { return true } }
	return false
}
func visible(r rune) bool {
	return unicode.In(r, unicode.L, unicode.N, unicode.P, unicode.S) &&
		!unicode.Is(unicode.Other_Default_Ignorable_Code_Point, r) && r != 0x2800
}
func blank(s string) bool { for _, r := range s { if visible(r) { return false } }; return true }
```

Each rule judges first and last name separately or joined (same result); "Must have a last name" is `blank(lastName)`; "Must have a username" is `user.Username == ""`.

**Invisible and whitespace-like characters** (probe output; "Trim" = `strings.TrimSpace(s) == ""`):

| Code point | Category | `IsSpace` | Trim | Other_DICP | `blank` |
|-----------|----------|-----------|------|------------|---------|
| U+0020, U+00A0, U+2000-200A, U+202F, U+205F, U+3000 | Zs | true | true | false | true |
| U+2028, U+2029 | Zl, Zp | true | true | false | true |
| U+00AD, U+061C, U+180E, U+200B-200F, U+202A-202E, U+2060-2064, U+2066-2069, U+206F, U+FEFF, U+1D173, U+E0020 | Cf | false | **false** | false | true |
| U+115F, U+1160, U+3164, U+FFA0 (Hangul fillers) | **Lo** | false | **false** | true | true |
| U+2800 (Braille blank) | **So** | false | **false** | false | true (listed explicitly) |
| U+034F, U+17B4, U+17B5 | Mn | false | false | true | true |
| U+0300, U+0308, U+FE0F, U+E0100 (marks, variation selectors) | Mn | false | false | false | true |
| U+FFFC (object replacement) | So | false | false | false | **false** (renders as a box) |

So `strings.TrimSpace` and `unicode.IsSpace` miss every Cf character, the Hangul fillers and the Braille blank; a category test alone misses the fillers (Lo) and the Braille blank (So). The rule above catches all the D-10 cases. Not detected: characters added after Unicode 15.0 (for example CJK Extension I at U+2EBF0, probe `han=false`; Arabic letters added in 16.0 such as U+10EC2, probe `arabic=false`) until Go updates its tables [VERIFIED: probe]. Tatweel U+0640 is Common script, so a name of tatweels is not Arabic (and is visible).

**NFC, not NFKC:** NFC was needed for the decomposed ё (`"Пе\u0308тр"`: raw `false`, NFC `true`). NFKC adds nothing: Arabic presentation forms (`ﻣﺤﻤﺪ`, `ﷲ`), Kangxi radicals (`⼀`), CJK compatibility ideographs (`丽`), `々` and `〇` are already in the script tables (probe `ar=true`/`han=true` without normalisation); NFKC maps U+3164 and U+FFA0 to U+1160, which is still invisible.

**Corpus with expected verdicts (probe output; the planner's table test pins exactly these):**

| Group | Names | ru | ar | han |
|-------|-------|----|----|-----|
| Ukrainian (must pass) | Олександр Ковальчук, Тарас Шевченко, Ірина Їжакевич, Євген Ґалаґан, Наталія Бойко, Андрій Мельник, Віталій Кличко, Юлія Тимошенко, Марʼяна Зозуля (U+02BC), Мар'яна Під'ячий, Олена Пчілка, Ґудзь Єлизавета, Їжак, Леся Українка, Богдан Хмельницький | false | false | false |
| Russian with the letters | Эдуард, Пётр Быков, Семён, Артём Рыбаков, Объедков, ЁЖИК, Фёдор Достоевский, decomposed Пётр and Ёлка | true | false | false |
| Russian without them (letter-based limit) | Иван Петров, Мария Иванова, Александр Смирнов | false | false | false |
| Belarusian | Аляксандр Лукашэнка, Рыгор Барадулін: true; Святлана Ціханоўская: false | mixed | false | false |
| Bulgarian | Ангел Бъчваров, Ъгъл: true; Цветан Цветков: false | mixed | false | false |
| Other Cyrillic | Новак Ђоковић, Нұрсұлтан Назарбаев, Сүхбаатар | false | false | false |
| Arabic script | محمد, Ahmed محمد, عمران خان (Urdu), محمد رضا شجریان (Persian), ﷲ, ﻣﺤﻤﺪ | false | true | false |
| Not Arabic letters | ٣٤٥ (Arabic-Indic digits), U+064B alone (a mark) | false | false | false |
| Han | 王伟, 陳大文, 田中 太郎, 佐々木, 々, 〇, ⼀, 丽, 𠀀 | false | false | true |
| Not Han | さくら, サクラ, 김민준 | false | false | false |
| Latin and mixed | Alex, Alex Иванов, José Müller, Zoë: all false; Alex Пётр, Ёzhik: ru true | mixed | false | false |
| Zero-width inside | `Пе\u200bтр` false; `Э\u200b` ru true; `\u200b王` han true | - | - | - |

A Latin `ë` (U+00EB) in place of `ё` evades the Russian rule; accepted (the rule is letter-based, and the page says so).

**Username:** `User.Username` is "Optional. User's or bot's username" (gotgbot `gen_types.go:15497`); a user's extra usernames are exposed only as `ChatFullInfo.ActiveUsernames` (`gen_types.go:1746-1747`), which costs a `getChat` per user. Go by `User.Username` (D-10); whether it is filled for a user with only collectible usernames is A6.

**Strictest:** severity ban 4 > kick 3 > mute 2 > warn 1 (the stored values 1..4 are not in severity order, so use a severity table, never the raw number). `{rules}` lists every broken rule whose action is not Off, in rule order.

### S-09. "I fixed it"

**Registration:** `dispatcher.AddHandler(handlers.NewCallback(callbackquery.Prefix("mreq|"), memberReqModule.fixedCallback))` with no `SetAllowChannel`, like `settings|` and `shub|` (`settings.go:235-237`); `lockdown|` sets `SetAllowChannel(true)` only because its buttons sit on channel copies (`lockdown.go:667`). Encode with `encodeCallbackData("mreq", map[string]string{"a": "f", "h": id, "u": user})` and decode with `decodeCallbackData(query.Data, "mreq")` (`callback_codec.go:15-45`). No existing namespace starts with `mreq` [VERIFIED: grep].

**Steps** (UI contract order; data sources found in code):
1. Decode strictly (`a=f`, `u` and `h` positive, at most 19 digits, nothing else; `settingsCardNumber`, `settings_card.go:27-41`, is the shape); message present, chat a supergroup.
2. `query.From.Id == u`, else `mreq_fixed_not_yours`, no read. A service identity can never equal `u`.
3. `GetHoldByIDFresh(h)`: chat, user, `action = 4`, `punished_at` set, `notice_msg_id` = the pressed message; else `mreq_fixed_stale`.
4. Live `getChatMember(chat, u)` through the pacer: any error `mreq_fixed_check_failed`; restricted with `is_member` false (the member left) `mreq_fixed_stale`; not the marker (S-04 test) `mreq_fixed_not_ours`. `getChatMember` "is only guaranteed to work for other users if the bot is an administrator in the chat" (gotgbot `gen_methods.go:2952`), so a bot that lost its rights answers check-failed.
5. Re-check the rules on that member record's `User` (first, last, username) against `GetSettings` (an unreadable setting is check-failed; a rule now Off is not checked).
6. `getChat` of the group; failure is check-failed.
7. `resolveUnmutePermissions(chatInfo)` (`chat_permissions.go:66-84`: during an active lockdown the stored pre-lockdown permissions, else the live defaults, else `defaultUnmutePermissions()`; an error is returned, and no caller may restrict on it). Error: check-failed.
8. `TakeHoldByID` (conditional): lost: `mreq_fixed_stale`.
9. `RestrictChatMember(chat, u, perms, nil)` (the `unmuteCaptchaUser` call shape, `captcha.go:1639`). Failure: `RestoreHold` with the same row, check-failed.
10. Success: toast `mreq_fixed_done` (or the lockdown alert `mreq_fixed_done_lockdown` when step 7 found an active lockdown), edit the notice in the group's language with no keyboard, then `SendWelcomeMessage(bot, ctx, u, firstName)` only when the hold had `welcome_held`.

Answer exactly once on every path (a deferred empty answer after the recover, as `settingsCallback` does). Bound the whole flow (steps 4, 6, 9 each with a short timeout, about 10 s in all) so the answer is sent while the query is still answerable [ASSUMED A8].

**Language (correction to the UI contract):** `ctxTr(ctx)` is `i18n.MustNewTranslator(lang.GetLanguage(ctx))` (`i18n_helpers.go:24-26`), and `lang.GetLanguage` returns the **group** language for any non-private `EffectiveChat` (`lang/repository.go:42-53`); there is no exported per-user getter (`getUserLanguage` is unexported, `:72`), and a user with no stored language reads `"uk"` (`:87-95`). A callback on a group notice therefore gets the group's language from `ctxTr`, and the captcha buttons already answer that way (`captcha.go` uses `lang.GetLanguage(ctx)` in its callbacks). Recommend: every "I fixed it" answer and the notice edit in the group's language; drop the UI contract's two-language test row (Open Question 6).

**Never lifts another mute:** step 4 compares against the marker, and every other path writes 0 or a user-chosen duration (S-04 table); a member an admin unmuted is not restricted (or restricted with another date) and gets `mreq_fixed_not_ours` with no write. A captcha re-mute replaces the marker with 0 (and the D-08 guard re-applies the marker, S-03), antiflood and warn-limit mutes write 0, staff mutes their own date, and a lockdown never restricts a member.

### S-10. The settings page

**Registry contract** [VERIFIED: `settings_pages.go:350-400, 422-440`]: `settingsPage{Key, Order, Icon, TitleKey, MainState, Open, Controls, Actions, OwnerOnly}`; `settingsControl{Right, Set, Validate func(value string) bool}`; `settingsPageAction{Fields, Right, Validate func(fields map[string]string) bool, Run}`; `registerSettingsPage` panics on a duplicate page key, a duplicate page action code or a core code (`settingsCoreActions`: `oh op bk cl tg st no pk`). Page key `m`, `Order` 80 and the codes `mo`, `my` are free (existing keys `s l c f w g r a`, orders 10..90 without 80, page codes `lk ua uy pg ls ly ay ws wr wc gs gr gc rs rr rc wv wf gv gf rv rf` [VERIFIED: grep]).

**The optional action allow-list.** `settingsCheckFields` (`settings_menu.go:237-295`) handles `tg`/`st` by allowing and requiring `p`, `t`, `v` and then calling `settingsControlFieldsValid(fields)` (`:265-269`), which checks the page, the control and `control.Validate(fields["v"])` (`:301-311`); it never sees the action. Add `Actions []string` to `settingsControl` (nil keeps today's behaviour: both `tg` and `st`) and change the function to `settingsControlFieldsValid(action string, fields map[string]string)` with `len(control.Actions) == 0 || slices.Contains(control.Actions, action)` before `Validate`; the one call site passes `action`. This edits `settings_pages.go` (the struct) and `settings_menu.go` once; AGENTS' "a new page never edits `settings_pages.go`" stays true for registering pages, and the bullet needs one clause about the allow-list.

**Controls and actions:**

| Code | Kind | Right | Validate | Behaviour |
|------|------|-------|----------|-----------|
| `ln un ru ar zh` | controls | `settingsRightRestrict` | `Actions: []string{"st"}`, `v` one digit 0..4 | 0/1 write through `applyMemberReqAction`, redraw the rule screen; 2/3/4 draw the card; same value (by `GetSettingsFresh`) writes nothing and redraws |
| `j d` | controls | `settingsRightRestrict` | `v` `0` or `1` | `applyMemberReqJoin` / `applyMemberReqDelete`, redraw the page |
| `mo` | page action | `settingsRightAdmin` | exactly `t`, a rule code | draw the rule screen |
| `my` | page action | `settingsRightRestrict` | exactly `t`, `v`, `e`; `t` a rule code; `v` in 2,3,4; `e` via `settingsCardNumber` | `settingsCardExpired` (`settings_card.go:46-53`), then the apply, redraw the rule screen |

`settingsRightRestrict` is judged by `settingsRightRefusalFor` on the press's own member record through `staffIssuerSkipReason` (`settings_menu.go:81-96`): `settings_no_restrict` for an administrator without the right, `settings_not_admin` otherwise, both alerts, nothing edited. The card is `settingsCardRows(k, map[string]string{"a": "my", "t": rule, "v": v}, "m")` (`settings_card.go:66-84`, adds `e` and the navigation row). The rule screen's Back is a button with `{"a": "op", "p": "m"}` and Close (`k.button`, `settings_pages.go:195`), not `navRow` (UI amendment M5). The choice rows reuse `settingsChoiceRow` (`settings_pages.go:314-332`).

**Main grid:** `Order` 80 puts the page between Rules (70) and Access (90): `s|l / c|f / w|g / r|m`, then the owner-only Access row. Grid label `🛂 Requirements`; `TestSettingsLabelLengths` holds a grid button to 20 code points (`gridMax = 20`, `settings_locale_test.go:403`), so the name is at most 18 (UI table: 14 EN, 18 other).

**Locale tests to extend:** `settings_locale_test.go` limits table (`TestSettingsLocaleKeys`, `:290`) must cover exactly the `settings_` keys: 199 today [VERIFIED: `grep -cE '^settings_' locales/en.yml` = 199], 235 after; `TestSettingsLabelLengths` (`:401`) gains the grid page `{"🛂", "settings_page_mreq"}`, the seven cells rendered with their icons, and `settings_state_off_plain`, `settings_opt_warn` in the choice-row list (`:426-427` today lists `settings_opt_kick`, `_ban`, `_mute` but not warn or off); a new `fuku/i18n/memberreq_locale_test.go` with `TestMemberReqLocaleKeys` for the 25 `mreq_` keys (0 today) beside `TestCaptchaWebLocaleKeys` (`captcha_web_locale_test.go:89`); `TestSettingsHelpFitsOneMessage` (`:576`) for the changed `settings_help_msg`.

### S-11. Existing tests that change on purpose, or break if forgotten

| Test | File | Why |
|------|------|-----|
| the owner's group Main shape `"s|l / c|f / w|g / r / a / close"` | `settings_edit_test.go:990` | becomes `"s|l / c|f / w|g / r|m / a / close"` |
| hand-built `mainEntries` | `settings_staff_privacy_test.go:113-123` | add the `m` entry (check 12) |
| every page ends with Back and Close (group and private) | `settings_menu_test.go:470-512` | iterates `settingsPagesInOrder()`: page `m` must open in the env, so `ChatRequirements` must be in the harness AutoMigrate list in the same plan as the page |
| Main grid end to end, worst-case Main, page counts | `settings_main_test.go:215-260`, `settings_edit_screens_test.go:108-118`, `settings_test.go:167-175` | dynamic; they call the new `MainState` (needs the table) |
| `TestSettingsLocaleKeys`, `TestSettingsLabelLengths`, `TestSettingsHelpFitsOneMessage` | `fuku/i18n/settings_locale_test.go` | new rows, new labels, new help text |
| warn command tests | `warns_command_test.go` | must pass unchanged after the `applyWarn` extraction |
| join and captcha tests (`greet_once_test.go`, `captcha_*_test.go`, `lockdown_paths_test.go`) | `fuku/modules` | must pass unchanged: with no `chat_requirements` row the join entry is one cached read and `SendWelcomeMessage` |

### S-12. AGENTS.md sentences that become false or incomplete

Handlers (the group list: -2), Callbacks (new `mreq|v1` namespace; page `m` of `settings|v1` with its codes, fields and the allow-list), Permissions (rights of page `m`; "I fixed it" presser-only, no admin override), Data (`chat_requirements`, `chat_requirement_holds`, every `fuku:memberreq:*` key, `skipLocal` entry, outside backup), Startup and shutdown (the new drain and its place), Subsystem traps (the marker, the captcha choke-point guard, the join reset and the invisible muted leave, the `applyWarn` core, the pass cache meaning), Testing (AutoMigrate lists, the PostgreSQL list, the fake's name fields, cleanup rule). Also: `resolveUnmutePermissions`' comment lists its callers ("/unmute, the unrestrict button, a captcha pass and staff /unmute", `chat_permissions.go:56-59`); add "I fixed it" and the join reset there and in the AGENTS bullet.

## Architecture Patterns

### System Architecture Diagram

```
 group message ─► -10 captcha holder (pending member: EndGroups) ─► -7/-6 join guards ─► -4 raid flood
                                                                                           │
                                                                                           ▼
                        ┌────────────── -2 memberReqWatcher (ContinueGroups always) ───────────────┐
                        │ supergroup? content? sender shape? ─no─► return                          │
                        │ settings (cache, skipLocal) ── none on / error ─► return                 │
                        │ pass cache hit ─► return                                                 │
                        │ rules(msg.From) ── clean ─► clean predicate DELETE, SET pass ─► return   │
                        │ cached admin? ─► return       else ─► memberReqRunAsync(job) (cap 32)    │
                        └───────────────────────────────────────────────────────────────────────────┘
                                                                                           │ (update goes on to -1, 0 commands, 1..12)
                                                                                           ▼
   join (chat_member / service msg, group 0) ─► ProcessSingleJoin: Claim ─► Reset(holds before join, lift live marker)
        │                                          ├─ captcha ─► challenge ... pass ─► unmuteCaptchaUser (guard: re-own marker) ─► finishCaptchaPass ─┐
        │                                          └─ no captcha / approved ─► Welcome dep ───────────────────────────────────────────────────────┤
        │                                                                                                                                          ▼
        │                                                                   memberReqJoinThenWelcome: settings, live getChatMember, enforce(sync) ─► welcome unless held/removed
        ▼
   memberReqEnforce(case): approved? pending captcha? hold? pause? live getChatMember (admin, names)
        ─► cap INCR ─► ClaimHold / ReclaimStale ─► punish (warn core | kick | ban | mute+marker) via memberReqPacer
        ─► MarkHoldPunished ─► notice (group language) ─► SetHoldNotice ─► delete message (if on)

   [✅ I fixed it] (mreq|v1, group 0) ─► presser==u ─► hold by id ─► live marker ─► rules ─► getChat ─► resolveUnmutePermissions
        ─► TakeHoldByID ─► restrict(perms) (fail: RestoreHold) ─► toast/alert, edit notice, held welcome via SendWelcomeMessage

   leave (chat_member, group 0 leftMember) ─► ClearHolds          settings menu page m ─► apply functions ─► upsert + DeleteCache
```

### Recommended Project Structure
```
migrations/20261106120000_add_member_requirements.sql
fuku/db/models/member_requirements.go        # ChatRequirements, ChatRequirementHold
fuku/db/memberreq/                           # settings.go, holds.go, testmain_test.go, *_test.go
fuku/db/approvals/repository.go              # + IsUserApprovedContext (error-returning)
fuku/db/cache/local.go                       # + member_requirements prefix in skipLocal
fuku/modules/memberreq.go                    # module registration (priority 241), handler wiring, drain
fuku/modules/memberreq_rules.go              # rule table, classifier, strictest, pass hash, notice name
fuku/modules/memberreq_enforce.go            # executor, pacer, cap/pause, runner, notices
fuku/modules/memberreq_watch.go              # group -2 watcher, pass cache, clean path
fuku/modules/memberreq_join.go               # join entry, reset, leave hook, captcha guard helper
fuku/modules/memberreq_fixed.go              # "I fixed it"
fuku/modules/settings_mreq.go                # page m
fuku/modules/warns.go                        # applyWarn extraction
locales/*.yml                                # 36 settings_ + 25 mreq_ keys, help text
fuku/i18n/memberreq_locale_test.go           # TestMemberReqLocaleKeys
```

### Pattern 1: Insert-first latch with a stale reclaim
The hold is claimed before any Telegram call (`ClaimGreeting` shape); the winner acts; a pending claim older than 2 minutes is reclaimed by one conditional update (the lockdown stale-claim shape). Refusals refresh `created_at` so the reclaim is also the backoff.

### Pattern 2: Ownership by end date
The bot recognises its own restriction by an end date nobody else writes (`isLockdownBan`, extended to `restricted`). Never by "is the member restricted".

### Pattern 3: One choke point per side effect
Every captcha unmute through `unmuteCaptchaUser`; every unmute's permissions through `resolveUnmutePermissions`; every requirement Telegram call through `memberReqPaced`; every warn through `applyWarn`.

### Anti-Patterns to Avoid
- **Returning `nil` from the watcher:** harmless today in group -2 but fatal if a second message handler is ever added there; always `ext.ContinueGroups`.
- **Using `IsUserAdmin` / `IsUserApproved` / `aispamCarvedOut` as is:** each turns an unknown into "not exempt".
- **Deleting a mute hold on every clean message or every leave:** orphans a live mute (S-03, S-05).
- **Using `ctxTr` and calling it the presser's language** in a group callback.
- **Building a second warn path** that copies `warnThisUser`'s limit logic.
- **Logging names or notice text:** IDs only.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Unicode normalisation | custom ё composition | `norm.NFC` (`golang.org/x/text`) | canonical composition has many cases |
| Script detection | code-point ranges | `unicode.Arabic`, `unicode.Han` | tables cover the extensions and compatibility blocks (probe) |
| Invisible characters | a hand list of Cf code points | categories + `unicode.Other_Default_Ignorable_Code_Point` + U+2800 | the derived property is the Unicode definition of "renders nothing" |
| Fleet-wide pacing | per-replica limiter | `ratelimit.NewTelegramPacer` with own keys | AGENTS: shared budgets never per replica |
| Exactly-once claim | Redis `SET NX` latch | PostgreSQL unique key + `ON CONFLICT DO NOTHING` | D-07: works without Redis, survives a flush |
| Unmute permissions | a hand-built struct or `chat.Permissions` | `resolveUnmutePermissions` | lockdown D-23 |
| Warn counting and limit | a copy of the limit switch | `applyWarn` extracted from `warnThisUser` | one code path, D-13 |
| Two-tap card | a new card | `settingsCardRows`, `settingsCardExpired` | stateless, tested |
| Callback encoding | `strings.Split` | `callbackcodec` via `encodeCallbackData`/`decodeCallbackData` | 64-byte cap, URL encoding |

**Key insight:** every hard part (exactly once, ownership of a mute, lockdown-safe unmute, pacing) already has a proven pattern in this repo; the phase's risk is in wiring them to the join path and the captcha without creating a second owner.

## Runtime State Inventory

Not a rename or migration phase: omitted.

## Common Pitfalls

### Pitfall 1: Two mute owners (milestone Pitfall 1)
**What goes wrong:** "I fixed it" lifts an admin, staff, captcha or antiflood mute. **Avoid:** the marker test only (S-04 table, S-09 step 4); the captcha guard re-applies the marker instead of skipping (S-03). **Warning sign:** a test where `/mute` then the button unmutes.

### Pitfall 2: Lockdown bypass (milestone Pitfall 2)
**What goes wrong:** the button grants pre-lockdown rights during a lockdown. **Avoid:** `resolveUnmutePermissions` only, error means no change (D-09). Whether a per-user restriction can exceed the chat's locked defaults is A5; the existing D-23 design (and the UI's `mreq_notice_fixed_lockdown` text) assumes it cannot. **Warning sign:** any `RestrictChatMember` in the phase whose permissions are not from `resolveUnmutePermissions` (other than `MutedPermissions`).

### Pitfall 3: Mass punishment (milestone Pitfall 3)
**What goes wrong:** a mute rule on a large group mutes everyone at their next message; an error read as "fail". **Avoid:** the confirm card (D-05); tri-state everywhere (S-02 steps 4, 8, 9, 12); the per-chat cap and pause notice (S-07); a 4xx backoff so a rights problem never loops. **Warning sign:** a fake returning 400/403/429/5xx for `getChatMember` produces any punishment.

### Pitfall 4: Warn and notice per message (milestone Pitfall 4)
**What goes wrong:** every message adds a warn; join and first message both punish. **Avoid:** the hold is the only latch, shared by the join check and the message check (S-05); the join check runs after `claimRecentJoinProcessing` (one of the two deliveries). **Warning sign:** ten messages, more than one warn.

### Pitfall 5: The invisible muted leave (new)
**What goes wrong:** a requirement-muted member leaves and rejoins; no handler deletes or lifts anything; on a no-captcha rejoin the member stays muted 330 days with no button. **Why:** `chat_status.go:584-586`. **Avoid:** the join reset (S-03) through the service-message path. **Warning sign:** a test that sends a restricted→restricted `chat_member` update and expects `leftMember`.

### Pitfall 6: Rename and out-of-order messages
**What goes wrong:** a clean older message processed after a newer violating one deletes the fresh hold, giving a second warn. **Avoid:** the clean predicate's `created_at < msgDate - 2s` (S-05); the live re-check before the claim (S-02 step 12).

### Pitfall 7: Captcha interplay (milestone Pitfall 23)
**What goes wrong:** the check runs for a pending-captcha member, or the pass unmutes a held member, or a lockdown joiner is screened. **Avoid:** the -10 holder ends pending members' updates; the tri-state captcha check in the job; the guard in `unmuteCaptchaUser`; the join check is at group 0 behind -7 and -6.

### Pitfall 8: Goodbye after a requirement kick or ban
**What goes wrong:** with goodbye on (the database default), every requirement kick or ban posts a goodbye, doubling the bot's messages in a raid. **Avoid (or accept):** parity with `/kick`, the captcha kick and the warn limit; the cap bounds it; see Open Question 9.

### Pitfall 9: Test hygiene
**What goes wrong:** a test leaves a users row (WarnUser creates one through `EnsureUserInDb`; the captcha path stores one from the fake's answer) and a later test in the same PostgreSQL database reads it; Phase 12's commit `7030575` fixed exactly this for user 4242. **Avoid:** unique member IDs per test (never small fixed numbers), a cleanup that deletes the users rows and `cache.CacheKey("user", id)` of every member it used, the hold and settings rows of its chats, the cache key, `dbcache.ResetLocalForTest()`, and the Redis keys of its chats (miniredis is per env, but `otherReplica` shares it).

### Pitfall 10: Callback answer and language
**What goes wrong:** answering twice or not at all; answering in the "presser's language" through `ctxTr` (it is the group's). **Avoid:** one deferred answer; group language (Open Question 6).

### Pitfall 11: The bot's own and others' messages
Bots are dropped by gotgbot; the bot itself, `sender_chat` posts, linked-channel forwards and service IDs by `raidFloodSenderCounts`; private chats never match the filter; Staff Group members are screened only if that group has rules on (no special code); `/start` in private never reaches the watcher.

### Pitfall 12: Rolling deploy
An old replica has no watcher (fewer punishments, never wrong), no `mreq|` handler (a press there gets no answer and the spinner times out; the member taps again), no page `m` (a press there is refused as an unknown page), no captcha guard (it can unmute a held member on the rare D-08 race) and no join reset (a muted rejoiner on it stays muted until the button is pressed on a new replica). All are accepted, like the earlier rolling-deploy caveats; the migration runs before any replica serves.

## Code Examples

### The watcher's shape
```go
// proposed; reuses raid_flood.go:134-163 and :199-213, antiflood.go:106-121
const memberReqWatchGroup = -2

func (m moduleStruct) memberReqWatcher(b *gotgbot.Bot, ctx *ext.Context) error {
	defer error_handling.RecoverFromPanic("memberReqWatcher", "MemberRequirements")
	msg, chat := ctx.EffectiveMessage, ctx.EffectiveChat
	if _, _, ok := raidFloodItemKey(msg); !ok {
		return ext.ContinueGroups
	}
	userID, ok := raidFloodSenderCounts(b, ctx)
	if !ok {
		return ext.ContinueGroups
	}
	settings, err := memberReqSettingsLookup(tracing.UpdateContext(ctx), chat.Id)
	if err != nil || !settings.AnyRuleOn() {
		return ext.ContinueGroups
	}
	names := memberReqNamesOf(msg.From)
	if memberReqPassHit(chat.Id, userID, settings, names) {
		return ext.ContinueGroups
	}
	broken := memberReqBroken(settings, names)
	if broken == 0 {
		memberReqClean(chat.Id, userID, time.Unix(msg.Date, 0), settings, names)
		return ext.ContinueGroups
	}
	if known, isAdmin := cachedAdminStatus(chat.Id, userID); known && isAdmin {
		return ext.ContinueGroups
	}
	memberReqRunAsync(memberReqCase{Chat: *chat, User: *msg.From, Settings: *settings, Source: memberReqFromMessage,
		MessageID: msg.MessageId, MessageDate: msg.Date})
	return ext.ContinueGroups
}

func LoadMemberRequirements(dispatcher *ext.Dispatcher) {
	dispatcher.AddHandlerToGroup(handlers.NewMessage(func(m *gotgbot.Message) bool {
		return m.Chat.Type == "supergroup"
	}, memberReqModule.memberReqWatcher), memberReqWatchGroup)
	dispatcher.AddHandler(handlers.NewCallback(callbackquery.Prefix("mreq|"), memberReqModule.fixedCallback))
}

func init() { RegisterLegacyModule("MemberRequirements", 241, LoadMemberRequirements) }
```

### Hold claim (verified pattern, `greeted.go:24-32`)
```go
row := &models.ChatRequirementHold{ChatID: c, UserID: u, Rules: r, Action: a, MuteUntil: until,
	WelcomeHeld: held, CreatedAt: time.Now()}
res := db.DB.WithContext(ctx).Clauses(clause.OnConflict{
	Columns: []clause.Column{{Name: "chat_id"}, {Name: "user_id"}}, DoNothing: true,
}).Create(row)
won := res.Error == nil && res.RowsAffected == 1 // row.ID is the button's h
```

### Marker ownership (extends `lockdown_marker.go:20-29`)
```go
func isMemberReqMute(m gotgbot.MergedChatMember, muteUntil int64) bool {
	if m.Status != gotgbot.ChatMemberStatusRestricted || !m.IsMember || muteUntil == 0 || m.UntilDate == 0 {
		return false
	}
	diff := m.UntilDate - muteUntil
	if diff < 0 {
		diff = -diff
	}
	return diff <= lockdownBanUntilTolerance
}
```

### The settings allow-list (edit of `settings_menu.go:301-311`)
```go
func settingsControlFieldsValid(action string, fields map[string]string) bool {
	pg, ok := settingsPageByKey(fields["p"])
	if !ok {
		return false
	}
	control, ok := pg.Controls[fields["t"]]
	if !ok {
		return false
	}
	if len(control.Actions) > 0 && !slices.Contains(control.Actions, action) {
		return false
	}
	return control.Validate == nil || control.Validate(fields["v"])
}
```

### Scratch reproduction (deleted afterwards)
The Unicode probe lived in the session scratchpad (`mreqprobe/main.go` and `extra_test.go`, its own `go.mod` requiring `golang.org/x/text v0.42.0`, run with `GOFLAGS=-mod=mod GOPROXY=off go run .`); its full output is the tables of S-08. It was removed after this file was written; nothing was created in the repository besides this file.

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| PITFALLS' ledger cleared by every mute path | marker end date, no other mute path edited | D-08 | only `unmuteCaptchaUser` gets a guard |
| PITFALLS' Redis episode latch | PostgreSQL insert-first hold | D-07 | works without Redis |
| ARCHITECTURE's watcher at -1 ending the update after a punishment | -2, `ContinueGroups` always | D-12 and this research | commands, aispam and the rest always see the message |
| ARCHITECTURE's one row per rule | one wide row | D-06 | column-targeted upserts |

**Deprecated/outdated:** none in this phase.

## Assumptions Log

| # | Claim | Section | Risk if Wrong | Live check |
|---|-------|---------|---------------|-----------|
| A1 | A bot can send at most 20 messages a minute to one group (search snippet of the official FAQ; page unreachable) | S-07 | the cap of 10 a minute is too high or too low | send 25 notices to a test group in a minute and record the first 429 and its `retry_after` |
| A2 | `getChatMember` returns a restricted member's `until_date` within 2 s of the value sent, for a 330-day restriction | S-04, S-09 | the button never recognises its own mute | mute a test account for 330 days, read `getChatMember`, compare |
| A3 | A supergroup keeps a member's restriction when they leave, reporting `restricted` with `is_member` false | S-03, Pitfall 5 | if dropped, the join-reset lift is a harmless no-op | mute, leave, read `getChatMember`, rejoin, read again |
| A4 | That leave and the rejoin arrive as `chat_member` updates whose old and new status are both `restricted` (so `leftMember` and `newMember` do not run), and the rejoin still produces a `new_chat_members` service message | S-03 | if the rejoin produces no service message in some groups, the reset never runs for a muted rejoiner | same test, log the updates |
| A5 | A per-user restriction cannot grant more than the chat's default permissions, so an unmute with pre-lockdown permissions during a lockdown keeps the member silent until the lift | Pitfall 2, S-09 | if wrong, "I fixed it" lets a member talk in a locked group (D-09 then needs a refusal during lockdown instead) | lock a test group, unmute a member with all rights, try to send |
| A6 | `User.username` is set for a user whose only usernames are collectible | S-08 | such a user is wrongly caught by "must have a username" | needs a Fragment username account; otherwise accept |
| A7 | `getChatMember` returns the user's current names | S-09 | "I fixed it" sees an old name | rename a test account, read `getChatMember` at once |
| A8 | A callback query must be answered within roughly 10 to 15 s | S-09 | a slow path answers into an expired query (the member sees a spinner) | delay an answer in a test bot and see when `answerCallbackQuery` fails |
| A9 | Restrict refusal wordings ("not enough rights", "user is an administrator of the chat") | S-04 | none: the classifier uses codes | none needed |
| A10 | Latency figures in the cost table | S-02 | only the cost narrative | none needed |
| A11 | A Telegram user can post in a group before `SendCaptcha` inserts the attempt (the D-08 race) | S-03 | if impossible, the captcha guard never fires (still correct) | none needed |
| A12 | Telegram's message dates and the server clock agree within 2 s | S-05 | the clean predicate keeps or deletes a hold one message late | none needed (NTP) |

## Open Questions (each with a recommendation)

1. **Handler group.** Recommendation: **-2**, the only message handler of the group (S-01). Update AGENTS.
2. **Pass cache.** Recommendation: **keep**, Redis, per `(chat, user)`, value = hash of the rule signature and the three names, 30 minutes, written only after a successful clean predicate; it exists to skip the clean `DELETE`, not the rules (S-02).
3. **`skipLocal`.** Recommendation: **yes**, as raid detection (S-06).
4. **Per-chat cap (A15).** Recommendation: **keep**: 10 punishments a minute per chat, then a 10-minute pause with one `mreq_notice_paused` (`{n}` 10, `{minutes}` 10); the key is added (25 `mreq_` keys). No cap without Redis.
5. **Mute hold on leave and rejoin.** Recommendation: a detected leave deletes any hold; a muted member's leave is not detected, so the join reset deletes holds older than the join and lifts a still-live marker mute through the unmute core before the join check (S-03). This realises D-07's "a rejoin or a leave deletes the hold" without orphaning a mute.
6. **Language of "I fixed it" answers.** Recommendation: the **group's** language for every answer and the notice edit (`ctxTr` on the group callback, the captcha precedent); correct UI contract section 8's Language row and drop check 10's "two languages different" sub-test, replacing it with "answers and edit are in the group's language". The alternative (the presser's stored language through a new exported getter) answers Ukrainian to every member who never set one.
7. **Clean message and a mute hold.** Recommendation: the clean predicate of S-05 (delete a mute hold only when the message is more than 2 s newer than the confirmed mute, or the hold is a stale pending one), which satisfies both D-07 and the UI contract.
8. **Basic groups.** Recommendation: **supergroups only** this phase (mute and kick are supergroup methods, gotgbot `gen_methods.go:4643, 8304`); Phase 14's "cannot work here" line names it.
9. **Goodbye after a requirement kick or ban.** Recommendation: **unchanged** (parity with `/kick`, the captcha kick and the warn limit); record as a known limit; a later phase may add a suppression flag.
10. **Join check names.** Recommendation: one live `getChatMember` per join, only when Check on join is on and a rule is on; no change to `captchaweb.InitData`.
11. **Refused punishment.** Recommendation: keep the pending hold as a 2-minute backoff (no Telegram call per message), welcome a joiner, post nothing; the stale reclaim retries.
12. **Async message path.** Recommendation: bounded runner of 32, drop when full; the join path stays synchronous under a 15 s context.
13. **Deletion during a pause.** Recommendation: only members already held keep having their messages deleted; new violators are left alone during the pause.
14. **Warn limit pacing.** Recommendation: the limit action inside `applyWarn` stays the warn module's own unpaced call (parity); only the requirement's own calls use `memberReqPacer`.
15. **Admin exemption source.** Recommendation: the cached list when it knows the member (`cachedAdminStatus`), otherwise the live `getChatMember` of the job, which also provides the names; never `IsUserAdmin` on this path.
16. **Log channel.** Recommendation: none this phase (UI A17).

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go | build, tests | ✓ | go1.26.0 linux/amd64 | - |
| gcc (cgo for SQLite tests) | `make test` | ✓ | 13.3.0 | - |
| PostgreSQL server | `make test-postgres-integrity` | ✗ (client 16.15 present, `pg_isready`: no response) | - | CI runs it; locally the PostgreSQL-only tests skip |
| redis-server | optional | ✓ | - | miniredis in tests |
| golangci-lint | `make lint` | ✓ | 2.5.0 | - |
| docker | `make build` | ✓ | - | `CGO_ENABLED=0 go build ./...` |
| core.telegram.org | Bot API docs | ✗ (DNS) | - | gotgbot's generated doc comments; live checks A1-A8 |

**Missing dependencies with no fallback:** none blocking. **With fallback:** PostgreSQL (CI), Telegram docs (gotgbot comments plus live checks).

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go `testing` with `-tags testtools`, testify assertions, miniredis, hand-written Bot API fakes |
| Config file | none (Makefile targets) |
| Quick run command | `go test -tags testtools -race -count=1 -run '^(TestMemberReq|TestSettingsMreq|TestSettingsAuthorityMreq|TestSettingsDriftMreq)' ./fuku/modules ./fuku/i18n ./fuku/db/memberreq` |
| Full suite command | `make test` (and `make test-postgres-integrity` in CI) |

### Phase Requirements to Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| MREQ-13 | corpus: Ukrainian pass, Russian/Belarusian/Bulgarian letters, Arabic, Han, invisible last names, NFC | unit (pure) | `go test -tags testtools -run '^TestMemberReqRules' ./fuku/modules` | ❌ Wave 0 (`memberreq_rules_test.go`) |
| MREQ-01 | page, rule screen, card, switches, defaults, row budget, callback bytes, 8 locales | integration | `-run '^TestSettingsMreq' ./fuku/modules` | ❌ |
| MREQ-01 | rights and drift | integration | `-run '^(TestSettingsAuthorityMreq|TestSettingsDriftMreq)'` | ❌ |
| MREQ-01 | storage, CHECK, upserts | unit + PG | `-run '^TestChatRequirementsSettings' ./fuku/db/memberreq` | ❌ |
| MREQ-02 | join check on both captcha pass paths and the no-captcha path; message check | integration | `-run '^TestMemberReqJoin' ./fuku/modules` | ❌ |
| MREQ-03 | delete messages, held members too, failed delete only logged | integration | `-run '^TestMemberReqDelete' ./fuku/modules` | ❌ |
| MREQ-04 | ten messages, three replicas, one punishment, one notice; clean ends a warn hold; rejoin and leave | integration | `-run '^TestMemberReqOnePunishment' ./fuku/modules` | ❌ |
| MREQ-04 | concurrent claim | PG | `-run '^TestRequirementHoldClaimConcurrent' ./fuku/db/memberreq` | ❌ |
| MREQ-05 | notice keys, strictest, rule list, warn count and limit variants, mention escaping and isolates, 400 code points | integration + locale | `-run '^(TestMemberReqNotice|TestMemberReqStrictest)' ./fuku/modules`; `-run '^TestMemberReqLocaleKeys' ./fuku/i18n` | ❌ |
| MREQ-05 | warn core byte-identical | regression | `-run '^(TestWarn|TestSilentWarn|TestDeleteWarn|TestRemoveWarn|TestRmWarn|TestResetAllWarns|TestLoadWarns|TestBlacklist)' ./fuku/modules` | ✅ |
| MREQ-06 | each "I fixed it" step, restore with the same ID, held welcome once, Greet once | integration | `-run '^TestMemberReqFixed' ./fuku/modules` | ❌ |
| MREQ-07 | never lifts admin, staff, captcha, antiflood, warn-limit, lockdown mutes; captcha guard | integration | `-run '^(TestMemberReqFixedNeverLifts|TestMemberReqCaptchaGuard)' ./fuku/modules` | ❌ |
| MREQ-08 | kick rejoin re-checked; ban stays | integration | `-run '^TestMemberReqKickBan' ./fuku/modules` | ❌ |
| MREQ-09 | every exemption, unknown states never punish, update types, `ContinueGroups` on every path, commands still run | integration | `-run '^TestMemberReqWatcher' ./fuku/modules` | ❌ |
| D-17 | tables out of backup | unit | `-run '^TestMemberRequirementsStayOutOfBackup' ./fuku/db/backup` | ❌ |

### Sampling Rate
- **Per task commit:** the quick run command for the touched package.
- **Per wave merge:** `go test -tags testtools -race -count=1 ./fuku/modules ./fuku/i18n ./fuku/db/...`.
- **Phase gate:** `make test`, `make lint`, `make check-translations`, `make check-docs` green before `/gsd-verify-work`; CI's `make test-postgres-integrity` green.

### Wave 0 Gaps
- [ ] `fuku/modules/staff_action_fake_test.go`: the fake's user JSON is fixed (`{"id":%d,"is_bot":false,"first_name":"User%d"}`, `:248`); add a per-user profile (first name, last name, username) the fake reports in `getChatMember`, kept apart from the member records that `restrictChatMember` replaces (`:346-372`). Zero value keeps today's answer.
- [ ] `fuku/modules/memberreq_env_test.go`: a `memberReqEnv` on `newSettingsEnv` (or `newLockdownEnv` with Greetings, Captcha and Warns loaded) with `sendAs(user, text)`, `joinAs(user)`, `pressFixed(user, notice)`, `holdOf(user)`, `noticesTo(chat)`, `jobs.wait()`, and a `memberReqCleanup(t, chats, users)` that deletes both tables' rows, the users rows and `user` cache keys of its member IDs, the settings cache key and calls `dbcache.ResetLocalForTest()`.
- [ ] Unique member IDs: a helper on the env (for example from `uniqueLinkOwnerID()`), never fixed small IDs (Phase 12 commit `7030575`).
- [ ] `fuku/db/memberreq/testmain_test.go`; backup and harness AutoMigrate lists; `settingsCleanup` extension.
- [ ] Makefile: two test names and `./fuku/db/memberreq` in `test-postgres-integrity`.

## Security Domain

### Applicable ASVS Categories (level 1)
| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | no | Telegram identity only |
| V3 Session Management | no | stateless callbacks |
| V4 Access Control | yes | page rights by live `getChatMember` (press pipeline, owner gate, `settingsRightRestrict`); "I fixed it" only `query.From.Id == u` plus the hold's own chat, user and notice ID; callback data never authority |
| V5 Input Validation | yes | strict callback field sets and digit parsers before any I/O; member names are untrusted: `formatting.MentionHtml` escapes, spliced after translation, capped at 64 runes, bidi-isolated |
| V6 Cryptography | no | SHA-256 for the pass hash is a fingerprint, not a secret |
| V7 Error Handling and Logging | yes | logs carry chat and user IDs only, never names, rule text or notice text |
| V11 Business Logic | yes | one punishment per episode (unique key), the per-chat cap, the confirm card, tri-state checks |

### Known Threat Patterns for this stack
| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Forged or replayed "I fixed it" (another `u` or `h`) | Spoofing/Tampering | presser equals `u`; hold read fresh and matched on chat, user, notice ID; conditional delete |
| A member freeing themselves from someone else's mute | Elevation | marker test; captcha guard |
| Lockdown bypass through the button | Elevation | `resolveUnmutePermissions`, A5 live check |
| HTML or bidi injection through names | Tampering | escaping, isolates, no name in templates |
| Evading a rule with zero-width or decomposed letters | Tampering | per-rune scan after NFC (probe) |
| Mass punishment from a mis-set rule or a broken dependency | DoS | card, cap and pause, tri-state, backoff |
| Notice flood and shared-pacer starvation | DoS | per-chat cap below the group limit; own pacer keys |
| Hot-path load from a spamming violator | DoS | held path costs one SELECT; bounded runner |

## Recommended decomposition into plans

| Plan | Wave | Depends on | Scope | TDD | AGENTS.md | PG integrity |
|------|------|-----------|-------|-----|-----------|--------------|
| 13-01 Storage | 1 | - | migration, models, `fuku/db/memberreq` (settings and holds, every operation of S-05/S-06), `IsUserApprovedContext`, `skipLocal`, AutoMigrate lists, backup test, `settingsCleanup` | yes | Data, Testing | `TestChatRequirementsSettings`, `TestRequirementHoldClaimConcurrent`, package path |
| 13-02 Rules engine (pure) | 1 | - | rule table, classifier, strictest, pass hash, notice name and mention | yes (corpus first) | - | - |
| 13-03 Warn core | 1 | - | `applyWarn` extraction, `kickMemberContext` | characterization (existing tests first, then new `TestApplyWarn*`) | Subsystem traps (one line) | - |
| 13-04 Locale keys | 1 | - | 36 `settings_` + 25 `mreq_` keys in 8 locales, help text, limits tables, `TestMemberReqLocaleKeys`, label lengths | yes (tests first) | - | - |
| 13-05 Enforcement core | 2 | 01, 02, 03, 04 | `memberReqEnforce`, pacer, cap and pause, async runner, notices, delete, drain and its `main.go` place, fake profile fields (Wave 0) | yes | Data (keys), Startup and shutdown | - |
| 13-06 Settings page | 2 | 01, 04 | `settings_mreq.go`, `settingsControl.Actions`, apply functions, Main grid, page tests, privacy test, changed Main shape | yes | Callbacks (page `m`), Permissions | - |
| 13-07 Message watcher | 3 | 05 | group -2 watcher, pass cache, clean path, exemption tests | yes | Handlers | - |
| 13-08 Join hook | 3 | 05 | join entry, `Reset` dep, `finishCaptchaPass` and `Welcome` dep wiring, `unmuteCaptchaUser` guard, `leftMember` hook, held welcome flag | yes | Subsystem traps (join, captcha) | - |
| 13-09 "I fixed it" | 3 | 05, 08 | `mreq|v1` handler, ten steps, restore, held welcome, never-lifts tests, lockdown test | yes | Callbacks (`mreq|v1`), Permissions | - |
| 13-10 Gate | 4 | all | three-replica ten-message test, final AGENTS pass, `resolveUnmutePermissions` comment, `make generate-docs`, `make check-docs`, `make check-translations`, `make lint`, `make test`; owner live checks A1-A8 on a checklist | - | final | confirm list |

## Sources

### Primary (HIGH confidence)
- Repo source read this session: `fuku/modules/{greetings,membership,captcha,captcha_web,warns,bans,mute,chat_permissions,lockdown_marker,lockdown_guard,lockdown_worker,raid_flood,raid_trigger,antiflood,aispam,users,bot_updates,staff_watchers,staff_action_decide,staff_action_run,settings_pages,settings_menu,settings_card,settings_access,settings_gate,i18n_helpers,callback_codec}.go`, `fuku/utils/chat_status/{chat_status,update_memo}.go`, `fuku/utils/ratelimit/telegram_pacer.go`, `fuku/db/{raiddetection,approvals,greetings,warns,lang,cache}/*.go`, `fuku/db/backup/{types,greeted_backup_test}.go`, `migrations/2026110*.sql`, `Makefile`, `main.go`, `internal/testdb/database.go`, the settings and staff test fakes.
- gotgbot module source `v2.0.0-rc.36.0.20260919140833-240296efadb4`: `ext/dispatcher.go`, `ext/handlers/message.go`, `sender.go`, `gen_methods.go`, `gen_types.go`.
- Unicode probe run in this session (Go 1.26.0, Unicode 15.0.0, `golang.org/x/text` v0.42.0).

### Secondary (MEDIUM confidence)
- [Telegram Bots FAQ](https://core.telegram.org/bots/faq) (and [its .md copy](https://core.telegram.org/bots/faq.md)), seen through a search result snippet: the 20-messages-a-minute group limit.

### Tertiary (LOW confidence)
- The milestone research (`.planning/research/PITFALLS.md`, `ARCHITECTURE.md`, `SUMMARY.md`) for alternatives the locked decisions replaced.

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH (no new dependency, versions from `go.mod`).
- Architecture: HIGH for the repo wiring (every call site read); MEDIUM where it rests on A3 to A5.
- Pitfalls: HIGH (each traced to a line).
- Unicode rules: HIGH (probe output).

**Research date:** 2026-11-05
**Valid until:** 2026-12-05 (repo moves fast; re-check the handler group list and `settings_` key count before planning if another phase lands first)
