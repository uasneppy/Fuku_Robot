# Phase 16: Scheduled Deletion of Service and Bot Messages - Research

**Researched:** 2026-11-10
**Domain:** Go Telegram bot (gotgbot v2.0.0-rc.36, GORM on PostgreSQL / SQLite tests, Redis): a durable deletion queue, a service-message watcher, opt-in hooks on the bot's own sends, and one settings page
**Confidence:** HIGH for the code facts (every discrete value below was read this session and is quoted), MEDIUM for Telegram error strings and the batch-delete edge cases (the pinned docs are silent; the code is written safe under every outcome)

Provenance tags: `[VERIFIED: file:line]` = file opened this session and the value quoted verbatim; `[VERIFIED: gotgbot docs]` = read in the pinned module `github.com/PaulSonOfLars/gotgbot/v2@v2.0.0-rc.36.0.20260919140833-240296efadb4`; `[VERIFIED: run]` = I ran it here (SQLite via the repo's driver, a throwaway PostgreSQL 16 that I stopped and deleted afterwards); `[ASSUMED]` = training knowledge or reasoning, listed in the Assumptions Log.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

**Rules storage and service-message types (accepted, area 1)**
- **D-01:** One table holds every deletion rule: `chat_delete_rules(chat_id, kind, delay_secs)`, primary key `(chat_id, kind)`, a `CHECK` that `delay_secs` is 0 to 86400, a row absent meaning Keep or Off. Kinds are strings validated in Go (`service:<type>` and `bot:<kind>` now, Phase 17 adds `member:*`), not a database `CHECK`, so a new service type needs no migration. One new migration (timestamp above 20261109120000, every statement idempotent, no top-level `BEGIN`/`COMMIT`, `scripts/migrate_psql.sh` and `runner.go` agreeing) creates this table and the queue of D-05. A chat's rules are read as one struct through `cache.GetFromCacheOrLoad` (`cache.CacheKey("delete_rules", chat)`), the key is listed in `skipLocal` (the service watcher and every send site read it, and a rule switched off must stop acting on every replica at the next message), and every write calls `cache.DeleteCache`. The table is never part of backup, export, import or reset.
- **D-02:** Member joined and member left keep the legacy `greetings.clean_service_settings` flag as their on/off, so `/cleanservice` is unchanged (on means delete at once) and nothing diverges after a backup import. The rule rows of `service:new_chat_members` and `service:left_chat_member` hold only the delay and count only while the flag is true: effective rule = Keep when the flag is false, otherwise the row's delay, or 0 (now) when there is no row. The page writes through one apply function beside `/cleanservice`: Keep sets the flag false, Now sets it true and removes the row, After N sets it true and writes the row, in one transaction followed by `cache.DeleteCache` of both keys. The rule table is outside backup while the flag travels in the greetings backup, so an import can never leave a stale row in charge of a group whose flag is off.
- **D-03:** The page offers about twelve grouped service-type rows, from one Go table (kind, label key, predicate over the fields of `gotgbot.Message`) so a later type is one row: Member joined, Member left, Chat photo (new and removed), Chat title, Pinned message, Video chats (scheduled, started, ended, participants invited), Forum topics (created, edited, closed, reopened, general topic hidden or shown), Boosts, Auto-delete timer changes, Chat background, Giveaways and gifts, and Other (every remaining deletable service field of the pinned gotgbot version, for example users or chat shared, write access allowed, web app data, proximity alert, checklist tasks, suggested posts, payments). The group-created, supergroup-created, channel-created and migrate-to and migrate-from messages are never offered and never deleted: Telegram refuses to delete creation messages, and the Staff Group re-key reads the migrate messages. The exact grouping, count and labels are the planner's, under the 20-code-point cell and 64-byte callback limits.
- **D-04:** One cap, 24 hours (86400 seconds), for every rule and kind, so an outage of up to a day still leaves the message deletable under Telegram's 48-hour limit. The page offers presets and nothing else: service types Keep, Now, 10 s, 1 min, 10 min, 1 h, 6 h, 24 h; bot kinds Off, 1, 10 and 60 minutes, 6 h and 24 h. The age is measured from the message's own send time (D-08).

**Deletion queue and worker (accepted, area 2)**
- **D-05:** Scheduled deletions live in PostgreSQL: `scheduled_deletions(id, chat_id, message_id, delete_at, kind, state pending|acting, claim_token, claimed_at, attempts, created_at)`, unique on `(chat_id, message_id)` with `ON CONFLICT ... DO UPDATE SET delete_at = LEAST(...)` so scheduling one message twice keeps the earliest time, and a partial index on `delete_at` where the state is pending. It is read only through fresh queries (never cached, so no `DeleteCache` applies) and is never part of backup, export, import or reset (a test like `TestMemberRequirementsStayOutOfBackup` proves it).
- **D-06:** `StartDeletionWorker` starts in `postInit` right after `StartLockdownWorker` and runs on every replica: a 1 s tick plus a wake channel, and every cycle under its own `RecoverFromPanic`. A cycle releases claims older than 2 minutes back to pending without counting an attempt, lists up to 200 due rows by `delete_at`, claims them with one conditional `UPDATE ... WHERE id IN (...) AND state = 'pending'` and a read by claim token (portable to the SQLite tests and no `SKIP LOCKED`), groups them by chat and sends `deleteMessages` with at most 100 IDs per call. When a batch call fails, it falls back to per-ID `deleteMessage` so the one undeletable ID is isolated (the live spike "Batch delete" is an owner check, deferred, and the code is safe under every outcome). A call that succeeds removes its rows.
- **D-07:** A 429, a timeout, a 5xx or a shutdown puts the rows back to pending (three tries; a 429 and a shutdown cost no attempt). A message that is gone, cannot be deleted or is too old removes its row quietly with no per-message log line. A permanent refusal for the chat (the bot not in it, kicked, or no delete right) removes the call's rows and sets a 10-minute negative latch `fuku:delete:norights:<chat>`; while it stands, scheduling writes nothing for that chat and the worker makes no call, so a group that cannot be deleted in is not hammered, and an owner who fixes the right waits at most 10 minutes. A row more than 47 hours past its message's own date is dropped quietly (DEL-06).
- **D-08:** The deletion time is the message's own send time plus the delay (`delete_at = message date + delay`), so a service message delivered late or a delay that began before a restart is never stretched. A delay of 0 deletes inline in a goroutine that starts with `defer error_handling.RecoverFromPanic`, through the same pacer, and writes no row; a crash can leave that one message, which is accepted.
- **D-09:** The worker and the inline deletes go through their own fleet-wide pacer, `deletionPaced` over `ratelimit.NewTelegramPacer` with keys `fuku:delete:pace:next` and `fuku:delete:pace:block` (Interval 100 ms, 3 retries, 60 s, the member requirements' shape), separate from every other pacer so a deletion burst never delays a punishment notice, a lockdown ban or a staff fan-out. Without Redis it falls back to one replica's interval and deletion still works.
- **D-10:** No new drain. `StopDeletionWorker` is a plain shutdown handler (`RegisterHandler`, the 10 s default) registered right after the admin index sweeper's stop, so LIFO runs it after `StopStaffActions` and before the database close. It cancels the loop and waits for it, and returns at once because every Telegram call of a cycle derives from the loop's context, so the worst case of 65 s in webhook mode is unchanged. Every row resumes from the database. The plan re-states the shutdown budget in `AGENTS.md` and proves it with a `TestShutdownEndsDeletionWorker` (a cycle inside a hung call is ended by the cancel, its rows go back to pending, a restart resumes them).

**Bot messages, kinds and hooks (accepted, area 3)**
- **D-11:** The Bot messages page offers exactly five kinds, the DEL-02 list: `bot:welcome`, `bot:goodbye`, `bot:rules`, `bot:punish_auto` and `bot:punish_manual`. Automatic punishments are the member requirement notices (the Phase 13 to 15 notices and the cap's pause notice, MREQ-15) and the notices of the antiflood, blacklist and aispam actions, plus the warn replies those modules produce. Captcha messages, lockdown notices and alerts, staff messages, log posts and raid posts are never a kind.
- **D-12:** Scheduling is explicit and opt-in: one helper, `scheduleBotMessage(chat, msg, kind)`, reads the chat's cached rules (a chat with the kind off costs one cached read and no write) and is called at named sites only: the welcome send in `SendWelcomeMessage`, the goodbye in `leftMember`, the group-sent rules message (the `/rules` reply and the click-to-read message), the requirement notice sends, the antiflood, blacklist and aispam notice sends, and the manual-punishment reply closures (through `moderationCtx`, whose reply functions today discard the sent message). Nothing hooks `helpers.SendMessageWithErrorHandling` or `media.Send`, which would capture captcha challenges, log posts, the aispam dump post and the settings previews. Previews (`/welcome`, the settings editor's), the private rules message and anything in a private chat are never scheduled. A chat that is not a group or supergroup is never scheduled. `cleanwelcome` and `cleangoodbye` (delete the previous greeting) stay as they are and may apply together with the timer, because the queue keeps the earliest time.
- **D-13:** A manual-punishment kind covers only the success reply of `/ban`, `/tban`, `/kick`, `/mute`, `/tmute`, `/unban`, `/unmute` and `/warn` and their d-variants. Validation and error replies, warn lists, `/resetwarns`, `/rmwarn`, the `/restrict` and `/unrestrict` question cards and the `/kickme` and `/banme` commands are not covered. `warnThisUser` is shared by the manual `/warn` and the blacklist's automatic warn, so it takes a source and each caller schedules its own kind.
- **D-14:** A requirement mute notice carries the member's "I fixed it" button, which is their only self-service way out. It is therefore scheduled at the 24-hour cap whatever the automatic-punishments timer says. When the member taps the button the notice is edited to its fixed text and that message then follows the normal timer: the queue keeps the earliest time, so scheduling it again at the shorter delay simply moves it up. A notice without a button follows the timer directly. The page's hint for the automatic punishments kind says that a mute notice stays up to a day so the member can still tap the button.

**Pages, rights and exclusions (accepted, area 4)**
- **D-15:** One Deletion page (a new page key, registered from its own file through `registerSettingsPage`, so `settings_pages.go` is not edited) is one Main cell with the state `{n} on` (the number of rules that are not Keep or Off) and the unknown mark when the read failed. It opens a hub screen listing Service messages and Bot messages, to which Phase 17 adds Commands, Edited messages and Self-destruct as further rows. Each sub-screen is reached through page actions with a `t` field (the Member requirements rule-screen shape), draws the rows of its table with the stored state, and opens a per-row screen with the preset choices. Its `Order` sits between Member requirements and Access, and its key letter and name are the planner's. A page read that fails is the cannot-open edit, as on every page.
- **D-16:** Preset buttons send the absolute number of seconds as `v` (at most 5 digits), so a double tap is harmless and every callback stays within 64 bytes (a worst-case test per page, as everywhere). There is no stepper and no typed input; a custom number is not offered in this phase.
- **D-17:** Opening the page and its screens needs `settingsRightAdmin`; changing a rule needs `settingsRightChangeInfo`, the right `/cleanservice`, `/cleanwelcome` and `/cleangoodbye` already ask for, so the menu refuses what the commands refuse (an authority-parity test). A rule can be set while the bot cannot delete: every deletion screen then shows a plain line that the bot cannot delete messages here, from one live `getChatMember` of the bot per draw through the deletion pacer under a short timeout (DEL-08). A bot that is an administrator with the delete right draws no line, and a check that cannot be answered (a 429, a timeout, a shed pacer, an unreadable answer) draws the page's unknown mark, never an error text, a refusal or the cannot-open edit.
- **D-18:** DEL-07 is enforced in three layers: the call sites are opt-in and none of them is in staff, lockdown, captcha or log code; `deletion.Schedule` (the one writer of queue rows) and the service-message watcher refuse a Staff Group chat; and the page is not drawn inside a Staff Group (the Staff page already is the read-only panel there). A test lists every kind, asserts none can name a staff, lockdown, captcha or log message, and asserts a Staff Group chat gets no row and no delete.
- **D-19:** Member joined and member left are handled where they are today, in the group-0 `cleanService`, after join processing, because the welcome and the captcha challenge reply to the join message and a deletion before them would break that reply; the handler drops its own unconditional delete and applies the rule of D-02 (now, or a queue row after N seconds). Every other service type goes through one new watcher that never ends an update (it returns `ext.ContinueGroups` on every path, its result preset so a recovered panic cannot return `nil`), is registered with the bot-sender flag set because a bot or an anonymous admin can cause a service message, and applies the rule of D-01 (now, or a queue row). The lockdown guard still ends and deletes banned joiners' messages itself, unchanged. Which handler group the watcher takes is the planner's, under the rule of AGENTS.md (never a second message handler in group -2).

### Claude's Discretion
- The exact grouping, count and labels of the service types (D-03), the page key letter, the `Order` value and the hub's wording (D-15), the migration timestamp and column types (D-01, D-05), the worker's batch sizes and timings (D-06) and the pacer's interval (D-09) within the numbers above, the locale key names, and how the service-message watcher is registered (D-19).
- Whether the hook in each reply closure returns the sent message through a changed `replyFn` or schedules inside the closure (D-12).
- The structure of the plans (the tracer slice first: a Now-deleted pinned message through the real watcher and rule, then the queue and worker, then the pages and the send sites).

### Deferred Ideas (OUT OF SCOPE)
- Members' commands, edited messages and self-destruct, and their `member:*` rule kinds: Phase 17 (DEL-03 to DEL-05).
- A typed custom delay and a stepper: they need the input state and are not part of the owner's list.
- A `/stats` section for the deletion queue (queued, deleted, dropped, capped), like the AI and captcha counters: optional, not built here.
- Mass deletion, delete-all and global silence: out of scope for v1.1 (PROJECT.md).
- The warn-list, error and card replies as bot kinds, and the Other service types as separate rows: the grouping can be refined after the owner's live check.
- The live checks (the batch delete spike, the caption of the delete-right line, phone-width cells in hi, ru and uk, two replicas, a restart with queued rows, a native-speaker read of the new strings) go to a `16-SPIKE.md` and are deferred under the owner's standing choice.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| DEL-01 | Service messages page: each Telegram service-message type is keep, delete now, or delete after N seconds; joined and left share storage with `/cleanservice` | S-02 (field classification and the 12-row table), S-03 (cleanService, watcher, group, D-02 transaction), S-06 (page) |
| DEL-02 | Bot messages page: each kind of the bot's own message is off or deleted after N minutes (welcome, goodbye, rules, automatic punishments, manual punishment replies) | S-04 (every send site with file:line, the minimal seams) |
| MREQ-15 | Requirement notices are deleted when the automatic-punishments timer is set | S-04 (memberReqPostNotice, mute notice at the cap, "I fixed it" reschedule) |
| DEL-06 | Scheduled deletions survive restarts and run once across replicas; delays capped below 48 h; a too-old deletion is dropped quietly | S-05 (migration, claim, worker, classification, `sent_at` correction C-01) |
| DEL-07 | Deletion never touches Staff Group items, lockdown alerts, captcha challenges, log posts | S-03, S-05 (three layers, where each check runs), S-06 (Hidden) |
| DEL-08 | Every deletion page says so when the bot lacks the right to delete | S-01 (right classification), S-06 (right line reader) |
| PLAT-05 / PLAT-06 (bind) | 57 strings in 8 locales; background work shares budgets across replicas, recovers, ends within the shutdown budget | S-05 (pacer, shutdown arithmetic), S-06 (locale tests), Validation Architecture |
</phase_requirements>

## Project Constraints (from CLAUDE.md / AGENTS.md)

- Stay on Go 1.26.0, gotgbot v2, GORM, Redis; no new dependency, no mock library; tests use real fixtures (`internal/testdb.Run`, miniredis, hand-written `gotgbot.BotClient` fakes) and always `-tags testtools`; `make test` is the only full run; no `t.Skip` to dodge a case (CI skip gate). [VERIFIED: AGENTS.md "Testing", "Commands"]
- Migrations are append-only, timestamp above every existing one, one transaction per file, no top-level `BEGIN`/`COMMIT`, no `CREATE INDEX CONCURRENTLY`; `fuku/db/migrations/runner.go` and `scripts/migrate_psql.sh` must agree. Never `gorm.AutoMigrate` in production code. [VERIFIED: AGENTS.md "Migrations"]
- Every write `cache.DeleteCache`s the keys it affects; keys read on every message belong in `skipLocal` (`fuku/db/cache/local.go`). `UpdateRecord` skips zero values: write `0` and `false` with column-targeted upserts. [VERIFIED: AGENTS.md "Data"]
- Group numbers are execution order; a watcher returns `ext.ContinueGroups`; a group-0 watcher returning `nil` kills every later group-0 handler; never add a second message handler in group `-2`; group 12 stays last. Value receivers on `moduleStruct`. [VERIFIED: AGENTS.md "Handlers"]
- Every fire-and-forget goroutine starts with `defer error_handling.RecoverFromPanic(...)`; register shutdown drains after DB-close; update AGENTS.md in the same commit as any rule it invalidates; all strings in 8 locales; callback data through `callbackcodec` (64 bytes), never `strings.Split`. [VERIFIED: AGENTS.md "Go rules", "Callbacks", "Startup and shutdown"]
- The per-group live admin check and the owner gate are never skipped (settings doors); the page reuses the press pipeline unchanged. [VERIFIED: AGENTS.md "Permissions"]

## Summary

Phase 16 is mostly assembly of patterns the repo already has: a DB-driven worker with conditional-update claims (the lockdown worker), a fleet-wide `ratelimit.NewTelegramPacer`, a cached per-chat settings struct with a `skipLocal` key (member requirements), a registry-driven settings page with page actions carrying `t` (the Member requirements rule screens), and opt-in hooks on named send sites. No new package, no new Redis family beyond `fuku:delete:pace:*` and `fuku:delete:norights:*`. The two tables and the SQL for them were exercised against a real PostgreSQL 16 and the repo's SQLite driver this session: the migration applies twice without error, the `LEAST` upsert needs the target table name (an unqualified `delete_at` is "ambiguous" on PostgreSQL), SQLite needs `MIN(table.col, excluded.col)` instead, the partial index is used by the due query, and the `UPDATE ... WHERE id IN (subselect) AND state='pending'` claim plus a read by token works on both.

The decisions hold, with nine concrete corrections the planner must carry (section "Corrections", C-01 to C-09). The ones that change code or text: the queue needs the message's own send time (`sent_at`) or D-07's "47 hours past its message's own date" cannot be computed (C-01); Telegram refuses to delete forum-topic creation service messages and `giveaway` is the giveaway post, not a service message, so two UI-SPEC rows over-promise (C-02, C-03); the "no delete right" latch must not stop `bot:*` deletion because a bot that is a plain member can still delete its own messages (C-04); a bot-sent join/leave service message never reaches `cleanService` (no `SetAllowBot`), so the watcher must cover joined/left for bot senders (C-06); and the existing `cleanService` tests assert `deleteMessage` synchronously, which D-08's inline goroutine breaks unless the tests wait on a tracker (C-08).

**Primary recommendation:** build in this order: data (migration, models, repository, `skipLocal`, harness lists) and the pure kind table in parallel, then the tracer (a pinned-message notice deleted Now through the real dispatcher and a seeded rule), then the worker and the `After N` path, then the bot-message hooks, then the page, then locales, then the closing sweeps and AGENTS.md. Register the service watcher at handler group `-5` (free, after the guards at -10/-7/-6, before the counters at -4), delete joined/left in `cleanService` after join processing, and give `StopDeletionWorker` its own 3 s wait so its contribution to the shutdown arithmetic is 3 s, not the 10 s default.

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Rule storage and reads | Database / Storage (`chat_delete_rules`) | Cache layer (`delete_rules:<chat>`, `skipLocal`) | Read on every service message and every opted-in send, so it is the cached-loader pattern; writes drop the key |
| Durable queue | Database / Storage (`scheduled_deletions`) | Redis (pacer keys, latch) | State must survive restarts and be claimed once across replicas: conditional updates in PostgreSQL |
| Service-message detection | API / Backend (new watcher, `cleanService`) | Dispatcher groups | Update-driven; must never end an update |
| Bot-message scheduling | API / Backend (named call sites + `scheduleBotMessage`) | Queue repository | Opt-in by design: only the sites that own a message know its kind |
| Deletion execution | API / Backend (worker, inline goroutines) | Telegram Bot API through `deletionPaced` | Fleet-wide pacing and per-chat batching |
| Settings page | API / Backend (settings framework, `settings_deletion.go`) | Telegram client (inline keyboard) | Menu in the bot, no web surface |
| Delete-right line | API / Backend (one live `getChatMember` per draw) | Telegram | A Telegram fact, never cached |
| DEL-07 exclusion | API / Backend (call-site discipline, `Schedule`, watcher, page `Hidden`) | Database (Staff Group read) | Three layers by D-18 |

## Standard Stack

### Core (all existing; no new dependency)

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `github.com/PaulSonOfLars/gotgbot/v2` | v2.0.0-rc.36 (`...20260919140833-240296efadb4`) | `DeleteMessage`, `DeleteMessages`, `Message` service fields, handlers | The bot's client; `DeleteMessages` exists and is unused so far [VERIFIED: gen_methods.go:1451] |
| `gorm.io/gorm` + postgres / sqlite drivers | v1.31.2 / v1.6.3 / v1.6.0 | models, `clause.OnConflict`, conditional updates | Repository pattern of every `fuku/db/<domain>` |
| `github.com/redis/go-redis/v9`, `eko/gocache` | v9.22.0, v4.4.0 | pacer keys, norights latch, the cached rules read | existing |
| `fuku/utils/ratelimit` (`NewTelegramPacer`) | in repo | fleet-wide pacing | member requirements / lockdown / staff all use it |
| `golang.org/x/sync` | v0.23.0 | not needed beyond what exists (a plain buffered channel is enough for the inline cap) | existing |

### Supporting / Alternatives

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| PostgreSQL queue (D-05, locked) | Redis sorted set | Rejected by D-05: must survive a Redis flush and be one source of truth |
| `SKIP LOCKED` claims | conditional update + token (D-06, locked) | Portable to the SQLite tests; verified to give one winner by the `state='pending'` re-check |

**Installation:** none. **Version verification:** no package is added, so `npm view`/`pip index` do not apply; `go list -m -f '{{.Dir}}' github.com/PaulSonOfLars/gotgbot/v2` gave the module directory read for this research.

## Package Legitimacy Audit

No external package is added by this phase. Packages removed (SLOP): none. Packages flagged (SUS): none. The planner needs no `checkpoint:human-verify` install task.

## Settled Findings (what the planner builds on)

### S-01. Telegram semantics (question 1)

**The Bot API text (pinned docs)** [VERIFIED: gotgbot docs, `gen_methods.go:1354-1367`], quoted verbatim:

```
// Use this method to delete a message, including service messages, with the following limitations:
//   - A message can only be deleted if it was sent less than 48 hours ago.
//   - Service messages about a supergroup, channel, or forum topic creation can't be deleted.
//   - A dice message in a private chat can only be deleted if it was sent more than 24 hours ago.
//   - Bots can delete outgoing messages in private chats, groups, and supergroups.
//   - Bots can delete incoming messages in private chats.
//   - Bots granted can_post_messages permissions can delete outgoing messages in channels.
//   - If the bot is an administrator of a group, it can delete any message there.
//   - If the bot has can_delete_messages administrator right in a supergroup or a channel, it can delete any message there.
```

`DeleteMessages` [VERIFIED: gen_methods.go:1447-1449]: `Use this method to delete multiple messages simultaneously. If some of the specified messages can't be found, they are skipped. Returns True on success.` with `messageIds`: `A JSON-serialized list of 1-100 identifiers of messages to delete. See deleteMessage for limitations on which messages can be deleted.` The gotgbot call sends `v["message_ids"] = messageIds` as a `[]int64` in the params map (`gen_methods.go:1459`), so a test fake reads `params["message_ids"].([]int64)`.

What follows for the code:

- **Who may delete what.** The bot's own sends (every `bot:*` kind) are "outgoing messages in ... groups, and supergroups": a bot that is a plain member can delete them. Incoming service messages need administrator status: in a basic group any administrator, in a supergroup `can_delete_messages`. So the delete-right line is exact for `service:*` kinds and conservative for `bot:*` kinds (A16-A6 resolved: the softened `settings_del_no_right` text "some of these settings may not work" is the right one; no per-section rule is needed).
- **Creation messages.** supergroup / channel / forum-topic creation service messages "can't be deleted" (group creation is not named but D-03 already excludes it). This makes `ForumTopicCreated` undeletable: see C-02.
- **What is only knowable by a live check** (the owner's "Batch delete" spike, deferred): (a) whether `deleteMessages` fails the whole call or silently skips an ID that exists but cannot be deleted (too old, a creation message, no right); (b) the exact `description` strings per refusal; (c) whether a basic-group administrator's `getChatMember` carries `can_delete_messages: true`. The code assumes nothing: a batch that returns success removes its rows; a batch that fails falls back to per-ID `deleteMessage` and each ID is classified on its own; a classifier that does not recognise a description treats a 4xx other than 429 as "drop this row quietly" and any other failure as "retry" (S-05).
- **Error descriptions the repo already classifies** (the only strings with production history) [VERIFIED: `fuku/utils/helpers/telegram_helpers.go:17-18`]: `strings.Contains(errStr, "message to delete not found") || strings.Contains(errStr, "message can't be deleted")` (gone / cannot be deleted, treated as success), and [VERIFIED: `fuku/modules/captcha.go:208-217`] the permanent list `"message to delete not found"`, `"message can't be deleted"`, `"bot was kicked"`, `"chat not found"`, `"group chat was deactivated"`, `"bot is not a member"`, `"CHAT_NOT_FOUND"`, `"PEER_ID_INVALID"`. [VERIFIED: `fuku/utils/chat_status/owner.go:46`] `botMissingMarkers = []string{"bot was kicked", "bot is not a member", "chat not found"}` and `cache.IsBotGoneError` is "a Telegram answer with Bot API error code 400 or 403" [VERIFIED: `fuku/utils/cache/adminCache.go:83-90`]. Strings I did not see in the repo and therefore tag `[ASSUMED]` (A1): the description of a supergroup upgrade (`"group chat was upgraded to a supergroup chat"`, which also carries `parameters.migrate_to_chat_id`, readable through `TelegramError.ResponseParams.MigrateToChatId`) and any "not enough rights" text for a missing delete right.
- **Recommended classifier** (`deletionClassify(err) -> done | gone | refused | retryFree | retry`), matching on lower-cased description fragments and on codes, never on a single string:
  - `retryFree`: `errors.Is(err, ratelimit.ErrRateLimited)` or `ctx.Err() != nil` (the pacer already waited out `retry_after`; a shutdown is not the message's fault). No attempt is counted (D-07).
  - `gone` (the bot cannot act in the chat at all; latch every kind): `chat_status.IsBotMissingError(err)`, descriptions containing `"group chat was deactivated"`, `"chat_not_found"`, `"peer_id_invalid"`, `"upgraded to a supergroup"`, or `ResponseParams.MigrateToChatId != 0`.
  - `refused` (this ID only, drop quietly, no log line, no latch): descriptions containing `"message to delete not found"` or `"message can't be deleted"`, `"message_id_invalid"`, or any other 4xx that is not 429 and not `gone`.
  - `retry` (counts an attempt; third failure drops the row): 5xx, timeout, transport error, anything else.

**The live right check (DEL-08)** [VERIFIED: `fuku/utils/chat_status/access.go:138-151`]: `CanBotDelete` is `botMember, ok := getUserMemberWithCache(b, chat, b.Id, "canBotDelete")` then `return botMember.CanDeleteMessages`, a cached answer, so the page must not use it (D-17 says live and paced). `MergedChatMember` carries `CanDeleteMessages` only for administrators: [VERIFIED: gen_types.go:2319-2326] `ChatMemberAdministrator.MergeChatMember` copies `CanDeleteMessages: v.CanDeleteMessages`, while [VERIFIED: gen_types.go:2520-2526] `ChatMemberOwner.MergeChatMember` sets only `Status: "creator"`, `User`, `IsAnonymous`, `CustomTitle`. `chat_status.FetchBotMember` [VERIFIED: owner.go:73-97] is live and tri-state but unpaced and uses `liveCheckTimeout`, so the page reader is its own function (like `memberReqBotChannelStatus`): `deletionPaced(ctx, ...)` around `b.GetChatMemberWithContext(callCtx, group.Id, b.Id, nil)`, classified by the shape of `memberReqClassifyBot` [VERIFIED: `fuku/modules/memberreq_lookup.go:504-523`]: with an error, `!errors.Is(err, ratelimit.ErrRateLimited) && cache.IsBotGoneError(err)` is "cannot", everything else unknown; with no error, creator is "can", administrator is "can" when `CanDeleteMessages` or when the chat is a basic group (`p.group.Type == "group"`, the docs say an administrator of a group can delete any message; a basic-group administrator's flags may all read false, A2), member / restricted / left / kicked are "cannot", anything else unknown. The Telegram 400/403 reading as "cannot" is the repo's own precedent and errs toward the 🚫 line, never toward an error text.

### S-02. Service-message policy table (question 2, D-03)

**Every service field of the pinned `gotgbot.Message`** [VERIFIED: `gen_types.go` Message struct, fields 125-239 of the struct], with my classification. "Content" fields (Invoice, Game, Poll, Story, Checklist, Contact, Dice, Venue, Location, `Giveaway`) are real user posts, not service messages.

| Group | `Message` fields (exact) | Deletable? |
|-------|--------------------------|-----------|
| Never offered, never deleted | `GroupChatCreated`, `SupergroupChatCreated`, `ChannelChatCreated`, `MigrateToChatId`, `MigrateFromChatId` (D-03), and **`ForumTopicCreated`** (docs: forum topic creation can't be deleted; C-02) | no |
| `sj` joined | `NewChatMembers != nil` | via `cleanService` (and the watcher for bot senders, C-06) |
| `sl` left | `LeftChatMember != nil` | same |
| `sp` photo | `NewChatPhoto != nil`, `DeleteChatPhoto` | yes |
| `sh` title | `NewChatTitle != ""` | yes |
| `sn` pinned | `PinnedMessage != nil` | yes (the interface field is non-nil for a pin notice) |
| `sv` video chats | `VideoChatScheduled`, `VideoChatStarted`, `VideoChatEnded`, `VideoChatParticipantsInvited` | yes |
| `sf` forum topics | `ForumTopicEdited`, `ForumTopicClosed`, `ForumTopicReopened`, `GeneralForumTopicHidden`, `GeneralForumTopicUnhidden` (not `ForumTopicCreated`) | yes |
| `sb` boosts | `BoostAdded` | yes |
| `sa` timer | `MessageAutoDeleteTimerChanged` | yes |
| `sk` background | `ChatBackgroundSet` | yes |
| `sg` giveaways, gifts | `GiveawayCreated`, `GiveawayWinners`, `GiveawayCompleted`, `Gift`, `UniqueGift`, `GiftUpgradeSent` (**not** the `Giveaway` field: it is the giveaway post itself, C-03) | yes |
| `so` other | `UsersShared`, `ChatShared`, `WriteAccessAllowed`, `WebAppData`, `ProximityAlertTriggered`, `ChecklistTasksDone`, `ChecklistTasksAdded`, `SuggestedPostApproved`, `SuggestedPostApprovalFailed`, `SuggestedPostDeclined`, `SuggestedPostPaid`, `SuggestedPostRefunded`, `SuccessfulPayment`, `RefundedPayment`, `PassportData`, `ConnectedWebsite`, `ChatOwnerLeft`, `ChatOwnerChanged`, `CommunityChatAdded`, `CommunityChatJoined`, `CommunityChatRemoved`, `DirectMessagePriceChanged`, `PaidMessagePriceChanged`, `ManagedBotCreated`, `PollOptionAdded`, `PollOptionDeleted` | yes |

Rows and codes are exactly the UI-SPEC's twelve (`sj sl sp sh sn sv sf sb sa sk sg so`); only the predicates of `sf` and `sg` change. One Go table `[]deletionServiceKind{Code, Kind, NameKey, DescKey, HintKey, Match func(*gotgbot.Message) bool}`; a function `deletionServiceKindOf(msg) (kind string, ok bool)` returns the first match, `ok=false` for a message that is content or a never-deleted type.

**One source of truth with `raid_flood.go`.** `raidFloodItemKey` [VERIFIED: raid_flood.go:136-141] lists `msg.NewChatMembers != nil || msg.LeftChatMember != nil || msg.PinnedMessage != nil || msg.NewChatTitle != "" || msg.NewChatPhoto != nil || msg.DeleteChatPhoto || msg.GroupChatCreated || msg.SupergroupChatCreated || msg.MigrateToChatId != 0 || msg.MigrateFromChatId != 0` only as an early exit; anything with no media and no text is rejected right after (`if !media && !text { return "", false, false }`), so it needs no more service fields and **should not be edited**. Do not share the predicate; instead add `TestServiceKindsCoverMessageFields`: reflect over `gotgbot.Message` and require every field to sit in exactly one of three explicit lists in the test (service-deletable, never-deleted, content), so a gotgbot upgrade that adds a service field fails the build instead of silently leaving it undeleted.

**How the dispatcher treats service messages** [VERIFIED: gotgbot `ext/handlers/message.go` `CheckUpdate`]:

```go
if ctx.EffectiveSender != nil && ctx.EffectiveSender.IsBot() && !m.AllowBot {
    return false
}
if ctx.Message != nil {
    return m.Filter == nil || m.Filter(ctx.Message)
}
```

`Sender.IsBot()` is `s.Chat == nil && s.User != nil && s.User.IsBot` [VERIFIED: gotgbot `sender.go:137-139`]. So: a service message caused by a bot (From is a bot, no `sender_chat`) is dropped unless `SetAllowBot(true)`; an anonymous admin (`sender_chat` set) is not a bot; a service message with no `From` has `User == nil`, so `IsBot` is false and it passes. The watcher is therefore registered `handlers.NewMessage(deletionServiceFilter, h).SetAllowBot(true)`, exactly as the lockdown guard and the migrate watcher do [VERIFIED: staff_watchers.go:172 `handlers.NewMessage(message.Migrate, staffWatchersModule.onMigrateMessage).SetAllowBot(true)`].

**Dispatcher semantics that decide the group** [VERIFIED: gotgbot `ext/dispatcher.go:280-328`]: `ContinueGroups` continues the same group "as if the handler did not match", `EndGroups` ends everything, any other return (`nil`) matched, so the loop `break`s to the next group. A watcher that returned `nil` would hide every later handler of its group.

### S-03. D-19 in the code (question 3)

**`cleanService` today** [VERIFIED: greetings.go:1109-1155] (quotes verbatim): it takes `msg := ctx.EffectiveMessage`, `user := chat_status.RequireUser(bot, ctx)`, ends with `ext.EndGroups` when `user.Id == bot.Id`; for `msg.NewChatMembers != nil` it reads captcha settings and runs `ProcessJoins(chat.Id, bot.Id, msg.NewChatMembers, captchaEnabled, deps)` with `membershipDepsForJoin(bot, &chatCopyBase, threadID, memberReqJoinDate(msg.Date))`; then:

```go
greetPrefs := greetings.GetGreetingSettings(chat.Id)

if greetPrefs.ShouldCleanService {
    _, err := msg.Delete(bot, nil)
    if err != nil {
        log.Error(err)
        return err
    }
}
return ext.EndGroups
```

Registered at greetings.go:1469-1476 as `handlers.NewMessage(func(msg *gotgbot.Message) bool { return msg.LeftChatMember != nil || msg.NewChatMembers != nil }, greetingsModule.cleanService)` through `dispatcher.AddHandler` (group 0) with **no `SetAllowBot`**.

Facts that fix the design:
1. The deletion must stay after `ProcessJoins` (it already is). The welcome does not reply to the join message: `SendWelcomeMessage` builds `media.GreetingMessage{..., ThreadID: threadID}` and `media.SendGreetingMessage` sends with `ReplyMsgID: 0` [VERIFIED: sender.go:353-370], so only the captcha challenge (and anything reading `msg`) depends on the message still existing; the order stays as is and D-19 holds.
2. `GetGreetingSettings` is uncached and can write: `checkGreetingSettings` -> `LoadGreetingSettings`, which on a missing row for a known chat builds defaults (and logs) [VERIFIED: greetings/repository.go:23-146, 198-200]. The read of the flag moves to `greetings.LoadSettingsContext(ctx, chatID)` [VERIFIED: repository.go:149-175: `db.DB.WithContext(ctx).Where("chat_id = ?", chatID).First(settings)`, "creates nothing"]; an error means "do not delete" (fail closed, one log line).
3. **Gap (C-06):** because `cleanService` has no `SetAllowBot`, the join/leave service message of a user added or removed *by a bot* is never delivered to it (today `/cleanservice` already misses those). The chat_member updates still drive the welcome/goodbye, so nothing in `cleanService` is needed for them. The new watcher must therefore handle `sj`/`sl` too, but **only when `ctx.EffectiveSender.IsBot()`** (the case `cleanService` cannot see), applying the same D-02 effective rule. No double handling is possible: for a non-bot sender the watcher skips `sj`/`sl`, for a bot sender `cleanService` never runs.
4. The lockdown guard (group -7) filters `msg.NewChatMembers` in place and ends the update when nobody is left [VERIFIED: AGENTS.md "lockdown guard"], and `lockdownDeleteJoinMessages` deletes banned joiners' join messages from its own table [VERIFIED: lockdown_worker.go:452-483]. Neither update reaches group 0 or the watcher (EndGroups at -7), so there is no overlap with D-19, and queue rows are never written for those messages.

**Where the new rule applies in `cleanService`:** after the join loop, replace the last block with: skip when the sender is the bot (already returned above); `flag, err := LoadSettingsContext`; `delay, ok := effective(flag, rules["service:new_chat_members"|"service:left_chat_member"])`; `ok` false = keep; delay 0 = `deletionDeleteNow(chat, msg.MessageId)` (inline goroutine); delay > 0 = `deletion.Schedule(...)`; always `return ext.EndGroups` (the old code returned the delete error to the dispatcher; the new code never errors).

**The D-02 apply function (transaction).** The Keep/Now/After writes touch two stores. A new exported greetings helper does the row guarantee on a transaction handle: `greetings.SetCleanServiceTx(tx *gorm.DB, chatID int64, on bool)` = `chats.EnsureChatInDb` outside the tx (as `upsertGreetingSettings` does, repository.go:235-240), then in the tx `FirstOrCreate` with `defaultGreetingSettingsAttrs(chatID)` and a **column-targeted** `Updates(map[string]any{"clean_service_settings": on, "updated_at": now})` (the model has `default:false` on that column [VERIFIED: models/greetings.go:39], so a `Create` of Go `false` would be rewritten; the map update is what `SetShouldCleanService` already does). `deletion.ApplyJoinLeftRule(ctx, chatID, kind, state)` wraps one `db.DB.Transaction`: flag update, then rule `DELETE` (Keep, Now) or upsert (After N); after commit `cache.DeleteCache(cache.CacheKey("greetings", chatID))` [VERIFIED: repository.go:252] and `cache.DeleteCache(cache.CacheKey("delete_rules", chatID))`. The command `/cleanservice` keeps calling `greetings.SetShouldCleanService` (flag only): **on a command `off` then `on`, a stale delay row survives and is honoured again**; that is D-02's stated behaviour (the row only counts while the flag is true) but note the surprise in the page's Currently line (the effective state is drawn, so it shows the stale delay after `/cleanservice on`). If the owner dislikes it, the command's apply function can also `DELETE` the two rows in the same transaction (one more call site); flagged as Open Question 1.

**Handler group for the watcher.** Used groups [VERIFIED: AGENTS.md "Handlers" and code]: -10 captcha, -7 lockdown guard, -6 fed-ban, -4 raid flood (`raidFloodGroup = -4`, raid_flood.go:48), -3 staff watchers (`handlerGroup: -3`), -2 member requirements (`memberReqWatchGroup = -2`, memberreq_watch.go:35, exclusive), -1 users tracker (`handlerGroup: -1`), 0 commands/greetings, 1 settings door, 2 raid AI (`raidAIGroup = 2`), 3-11 modules, 12 late flood. **Free: -9, -8, -5 and 13+ (13+ is forbidden: group 12 must stay last).** Recommendation: **group -5** (`serviceDeleteGroup = -5`): after the guards that may legitimately end an update (-10 captcha pending, -7 lockdown joins, -6 fed-ban) and before the flood counter (-4), so the watcher also runs for messages later modules end; -5 hosts no handler since the old raid mode was retired [VERIFIED: AGENTS.md "There is no join handler at group -5"], and AGENTS.md must be edited to list it. Group -3 (as a fifth staff watcher) would also work technically (every -3 watcher returns `ContinueGroups`) but mixes a deletion feature into "staff watchers".

**The watcher's shape (never `nil`):**

```go
// Source: pattern of onMigrateMessage (staff_watchers.go:33-56) and memberReqWatcher (AGENTS.md)
func (m moduleStruct) serviceDeleteWatcher(b *gotgbot.Bot, ctx *ext.Context) (result error) {
	result = ext.ContinueGroups          // preset: a recovered panic cannot return nil
	defer error_handling.RecoverFromPanic("serviceDeleteWatcher", "ServiceDeletion")
	... cheap checks first: chat type group/supergroup, kind of the message, rules read, staff gate ...
	return ext.ContinueGroups
}
```

Order, cheapest first: `msg != nil` and `msg.Chat.Type` is `group` or `supergroup`; `deletionServiceKindOf(msg)` (pure); for `sj`/`sl` only when `ctx.EffectiveSender.IsBot()`; the cached rules read (`deletion.GetRules`, key in `skipLocal`; an error ends it, so an unreadable setting deletes nothing); the effective delay for the kind (for `sj`/`sl` also the greetings flag via `LoadSettingsContext`); only then the Staff Group refusal (a fresh `staff.GetStaffGroupFresh`, fail closed on error: it is reached only when a deletion would happen, so the extra read is rare); then delay 0 -> inline delete, delay > 0 -> `deletion.Schedule`. The module registers `RegisterLegacyModule("ServiceDeletion", 242, LoadServiceDeletion)` (priorities used: ...241 MemberRequirements, 250 Reactions [VERIFIED by grep]; 242 is free) so the watcher group does not depend on the registration order.

### S-04. Reply and notice sites (question 4, D-12 / D-13)

Every site below was opened this session. "Seam" is the smallest change that returns the sent `*gotgbot.Message` or schedules inside.

**Manual punishment replies (`bot:punish_manual`).** The seam: `moderationCtx` [VERIFIED: moderation.go:21-29] is `Bot, Chat, Msg, User, Ctx, Tr, Module`; `replyFn` is `func(c *moderationCtx, t *target) error` (moderation.go:182) and every success closure does `_, err = c.Msg.Reply(...)`, discarding the message. **Smallest change that does not alter behaviour: a method `func (c *moderationCtx) replySuccess(text string, opts *gotgbot.SendMessageOpts) error`** that calls `c.Msg.Reply(c.Bot, text, opts)` and, on success, `scheduleBotMessage(sent, kindManual)`; replace `_, err = c.Msg.Reply(` with `err = c.replySuccess(` at exactly the success sites below (no `replyFn` signature change, no change to `run`).

| Command(s) | Success reply site | Notes |
|------------|--------------------|-------|
| `/kick`, `/dkick` | `kickReply`, bans.go:64 `_, err = c.Msg.Reply(c.Bot,` | shared by moderationKick (bans.go:251) and moderationDkick (bans.go:94) |
| `/tban` | bans.go:129 `_, err = c.Msg.Reply(c.Bot, baseStr, formatting.Shtml())` | |
| `/ban`, `/dban` | `banReplyWithButton`, bans.go:155 `_, err = c.Msg.Reply(c.Bot, text,` | shared by moderationBan (reply at bans.go:229) and moderationDban (bans.go:349) |
| `/ban` of a channel sender | bans.go:231 `_, err := c.Msg.Reply(c.Bot, text, sendMsgOptns)` | the other branch of the same closure |
| `/unban` | bans.go:420 `_, err := c.Msg.Reply(c.Bot, text, formatting.Shtml())` | |
| `/mute`, `/dmute` | `muteReplyWithButton`, mute.go:35 | moderationMute (mute.go:136), moderationDmute (mute.go:188) |
| `/tmute` | mute.go:115 `_, err = c.Msg.Reply(c.Bot, baseStr, formatting.Shtml())` | |
| `/unmute` | mute.go:254 `_, err = c.Msg.Reply(c.Bot, text, formatting.Shtml())` | |
| `/warn`, `/dwarn`, `/swarn` | `warnThisUser`, warns.go:290 `_, err = b.SendMessage(chat.Id, reply, &gotgbot.SendMessageOpts{...ReplyMarkup: &keyboard,...})` | the sent message is discarded today (`_, err =`); both the plain warn and the warn-limit text go through this one send |
| **Not covered (D-13)** | validation replies (bans.go:189, 275, 331, 366, 381; mute.go:65, 75, 170, 201, 213; warns.go:197, 226, 329, 337), `/kickme` ok reply bans.go:289, `/banme` bans.go:948, `/restrict` question card bans.go:526, `/unrestrict` card bans.go:731, `/sban`, `/skick`, `/smute` (no reply: `reply: nil` bans.go:315 / 905; `smute`'s reply only deletes the command, mute.go:172) | |

`warnThisUser(b, ctx, userId, reason, warnType string)` [VERIFIED: warns.go:182] already takes `warnType` ("warn", "dwarn", "swarn") but not a source. Its two callers: warns.go:355 `return m.warnThisUser(b, ctx, warnusr, reason, warnType)` and blacklists.go:572 `err = warnsModule.warnThisUser(b, ctx, user.Id(), fmt.Sprintf(reason, i), "warn")`; the only test call is warns_command_test.go:458. Add a final parameter `source warnSource` (`warnSourceManual`, `warnSourceAuto`); the one scheduling call after the send at warns.go:290 picks `bot:punish_manual` or `bot:punish_auto` from it. (A struct options parameter is not needed for two values.)

**Automatic punishment notices (`bot:punish_auto`).**

| Site | File:line | Seam / note |
|------|-----------|-------------|
| Requirement notices (all five punishments, with or without button) | `memberReqPostNotice`, memberreq_notice.go:173-208; its send is `sent, sendErr = b.SendMessageWithContext(callCtx, chatID, text, opts)` and it returns `sent.MessageId` | Call `scheduleBotMessage(sent, kind)` where `err == nil` (line ~196): it has `sent` (id, `Date`, `Chat`), `withButton := len(keyboard.InlineKeyboard) > 0`, so one site covers both callers (memberreq_enforce.go:560 `memberReqAnnounce`, memberreq_limits.go:102 the pause notice). The scheduling uses a context that survives the run's cancellation (the record contexts of the member requirements, `memberReqRecordCtx`) |
| Requirement mute notice with the button (D-14) | same site; `withButton` is true only for a mute (`memberReqNoticeKeyboard(tr, hold.Action, hold.ID, user.Id)`) | schedule at the 24 h cap (86400 s) when the kind is on (see C-07 for "Off") |
| The "I fixed it" edit | `memberReqFixedEdit(ctx, b, tr, chat.Id, query.Message.GetMessageId(), live.User, noticeKey)`, memberreq_fixed.go:213 (function at :293) | after a successful edit call `deletion.Schedule` for that message at the timer's delay; `ON CONFLICT ... LEAST` moves the 24 h row up. The callback's `query.Message` is `MaybeInaccessibleMessage` (`GetDate()` 0 when inaccessible), so the base time is `now`, see C-07 |
| Antiflood action notice | antiflood.go:402 `if _, err := helpers.SendMessageWithErrorHandling(b, chatId, ..., &gotgbot.SendMessageOpts{...MessageThreadId: msg.MessageThreadId})` | capture the returned message (it returns `(*gotgbot.Message, error)`; `nil, nil` for a restricted chat, so guard nil) |
| Blacklist notices | blacklists.go:516 (mute), :537 (ban), :557 (kick): `_, err = b.SendMessage(chat.Id, ..., formatting.Shtml())` | capture the message at each; the `warn` action goes through `warnThisUser(..., "warn")` at :572 with `warnSourceAuto` |
| AI spam | aispam.go:356-364 `aispamNotifyBreaker`: `helpers.SendMessageWithErrorHandling(b, chatID, text, formatting.Shtml())` | **the only group-side message aispam sends**: a delete verdict is silent (`aispamMirrorDeletion` posts to `config.AppConfig.MessageDump`, an owner log chat, never a kind, aispam.go:366-391). D-12 names "aispam notice sends"; the breaker notice is that site. Decision for the planner: schedule it (cost: three lines) |

Raid/lockdown posts, captcha messages, staff messages and log posts: no hook, by construction (D-12).

**Welcome, goodbye, rules.**

| Kind | Site | Notes |
|------|------|-------|
| `bot:welcome` | `SendWelcomeMessage`, greetings.go:945 `sent, err := media.SendGreetingMessage(bot, chat.Id, media.GreetingMessage{...})` | schedule right after the `err` check (before the `CleanWelcome` block at :960), only `chat.Type` group/supergroup |
| `bot:goodbye` | `leftMember`, greetings.go:1039 `sent, err := media.SendGreetingMessage(...)` | same; `cleanwelcome`/`cleangoodbye` bookkeeping untouched (the queue keeps the earlier of the two times) |
| `bot:rules`, click-to-read message | rules.go:357 `if _, err := msg.Reply(bot, text, opts); err != nil` (the group message with the "read rules" URL button, when `row.Private`) | schedule the reply; the chat is a group by the `RequireGroup` guard just above |
| `bot:rules`, the rules message | rules.go:372 `if err := sendRulesMessage(bot, msg.Chat.Id, rm, opts, true)` | `sendRulesMessage` returns only `error` today (rules.go:125) and is shared with the private deep link (rules.go:568, to the member's private chat) and the settings preview (settings_edit_preview.go:93, to the presser's private chat). Change it to return `(*gotgbot.Message, error)` (three callers; `sendRulesText` and `media.Send` both already return the message and it is discarded: rules.go:137 `_, err := media.Send(...)`, :168 `_, err := b.SendMessage(...)`) and schedule only at the `/rules` call; `scheduleBotMessage` independently refuses a non-group chat, so the deep link and previews can never be scheduled even by mistake. `/rules` also works from a private chat connected to a group: `msg.Chat` is then private and nothing is scheduled |

**`scheduleBotMessage(sent *gotgbot.Message, kind string)`** (signature note: the sent message carries its own chat, id and date; D-12's `(chat, msg, kind)` shape is not binding): returns at once for `sent == nil`, a chat that is not `group`/`supergroup`, or `kind` off; reads the cached rules (`deletion.GetRules`); a read error schedules nothing; checks the negative latch and the Staff Group (fresh read) only when a row would be written; computes `delete_at = sentAt + delay` with `sentAt = clamp(msg.Date)` (below); calls `deletion.Schedule`; **never returns an error to the caller** (a failure is one Warn with chat and message IDs) so no send site changes behaviour. The write is synchronous with a 3 s context (a few milliseconds; a goroutine would need a drain and a panic guard for no gain).

**Time arithmetic.** Message dates are unix seconds in `Message.Date int64`. `sentAt = time.Unix(date, 0).UTC()`; if `date <= 0` or after `now` use `now` (clock skew must never stretch a message past its delay); store UTC at second precision (SQLite compares times as text, so mixed offsets or fractional digits would mis-order them; the lockdown repository already passes `time.Time` cutoffs this way). The shared test fake answers `sendMessage` with `"date":1` [VERIFIED: staff_action_fake_test.go: `{"message_id":%d,"date":1,"chat":%s}`], which would put every test row at 1970 and trip the 47 h drop: see Wave 0 Gaps (a `sendDate` seam on the fake, default 1 so nothing else changes) and C-09.

### S-05. Queue, worker, pacer, shutdown (question 5)

**Migration (verified this session).** File `migrations/20261110120000_add_scheduled_deletion.sql` (above `20261109120000`). Both appliers handle it unchanged: `runner.go` rewrites `CREATE TABLE` / `CREATE [UNIQUE] INDEX` to `IF NOT EXISTS` and wraps top-level `ADD CONSTRAINT`; `migrate_psql.sh` does the same with perl [VERIFIED: runner.go:472-520, migrate_psql.sh:67-75]; inline constraints in `CREATE TABLE` and a `WHERE` on an index are untouched by those rewrites (precedent: `uk_chat_lockdowns_active ... WHERE state = 'active'`, migrations/20261006120000:53). I applied the file twice on PostgreSQL 16 with `ON_ERROR_STOP` (only "already exists, skipping" notices):

```sql
CREATE TABLE IF NOT EXISTS chat_delete_rules (
    chat_id BIGINT NOT NULL,
    kind VARCHAR(40) NOT NULL,
    delay_secs INTEGER NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    PRIMARY KEY (chat_id, kind),
    CONSTRAINT chk_chat_delete_rules_delay CHECK (delay_secs BETWEEN 0 AND 86400)
);

CREATE TABLE IF NOT EXISTS scheduled_deletions (
    id BIGSERIAL PRIMARY KEY,
    chat_id BIGINT NOT NULL,
    message_id BIGINT NOT NULL,
    kind VARCHAR(40) NOT NULL,
    sent_at TIMESTAMP WITH TIME ZONE NOT NULL,
    delete_at TIMESTAMP WITH TIME ZONE NOT NULL,
    state VARCHAR(8) NOT NULL DEFAULT 'pending',
    claim_token VARCHAR(32) NOT NULL DEFAULT '',
    claimed_at TIMESTAMP WITH TIME ZONE NULL,
    attempts SMALLINT NOT NULL DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    CONSTRAINT uk_scheduled_deletions_message UNIQUE (chat_id, message_id),
    CONSTRAINT chk_scheduled_deletions_state CHECK (state IN ('pending', 'acting')),
    CONSTRAINT chk_scheduled_deletions_attempts CHECK (attempts >= 0)
);

CREATE INDEX IF NOT EXISTS idx_scheduled_deletions_due ON scheduled_deletions (delete_at) WHERE state = 'pending';
CREATE INDEX IF NOT EXISTS idx_scheduled_deletions_acting ON scheduled_deletions (claimed_at) WHERE state = 'acting';
```

Verified outcomes [VERIFIED: run]: the CHECK refuses `delay_secs = 86401` (`violates check constraint "chk_chat_delete_rules_delay"`); the due query `WHERE state='pending' AND delete_at <= now() ORDER BY delete_at LIMIT 200` plans an `Index Scan using idx_scheduled_deletions_due`; three inserts of one `(chat_id, message_id)` at +1 h, +1 min, +2 h leave `delete_at` at +1 min. `sent_at` is the addition of C-01 (the other columns are D-05's, plus `id` BIGSERIAL; `kind` is `VARCHAR(40)`, the longest kind `service:auto_delete_timer` is 25 characters).

**Models** (`fuku/db/models/delete_rule.go`, `scheduled_deletion.go`; each with `TableName()` returning the migration's name, no `default:` tag on any value that can be a Go zero, so a `0` delay is written). The composite-key precedent is `GreetedMember` (`gorm:"column:chat_id;primaryKey;autoIncrement:false"`). I ran the equivalent tags through `AutoMigrate` on the repo's SQLite driver [VERIFIED: run]: `uniqueIndex:uk_sd,priority:1/2`, `index:idx_sd_due,where:state = 'pending'` (renders `CREATE INDEX ... WHERE state = 'pending'`), `check:chk_sd_state,state IN ('pending','acting')` all work.

**Repository `fuku/db/deletion/`:**
- Rules: `GetRules(ctx, chatID) (*Rules, error)` through `cache.GetFromCacheOrLoad(ctx, cache.CacheKey("delete_rules", chatID), cache.DefaultCacheTTL, load)` and `GetRulesFresh`, exactly the shape of `memberreq.GetSettings` [VERIFIED: memberreq/settings.go:100-120]; `Rules{ChatID int64; Delays map[string]int}` (msgpack handles the map; a chat with no rows is an empty map and creates nothing); `SetRule(ctx, chatID, kind string, delay int) error` (validates the kind against the Go registry and `0 <= delay <= 86400` before any write, then one `clause.OnConflict{Columns: chat_id, kind; DoUpdates: clause.AssignmentColumns([]string{"delay_secs","updated_at"})}`, a Go `0` is written) and `RemoveRule(ctx, chatID, kind)`; **every write ends with `cache.DeleteCache(cache.CacheKey("delete_rules", chatID))`**. `skipLocal`: add `deleteRulesPrefix = CacheKey("delete_rules") + ":"` beside `memberRequirementsPrefix` and `|| strings.HasPrefix(key, deleteRulesPrefix)` in `fuku/db/cache/local.go`, plus the two rows (`"delete_rules"` positive, `"delete_rulesx"` negative) in `local_test.go:286-292`.
- Queue: `Schedule(ctx, e Entry) (bool, error)` (refuses a Staff Group by a fresh `staff.GetStaffGroupFresh`, fail closed; refuses a latched chat; one upsert; returns whether a row was written). **The portable upsert** (the one place the dialects differ) [VERIFIED: run on both]:
  ```go
  func leastExpr(tx *gorm.DB) clause.Expr { // tx.Name() is "postgres" or "sqlite" (adminindex/repository.go:90 uses tx.Name() == "postgres")
      if tx.Name() == "postgres" { return gorm.Expr("LEAST(scheduled_deletions.delete_at, EXCLUDED.delete_at)") }
      return gorm.Expr("MIN(scheduled_deletions.delete_at, excluded.delete_at)")
  }
  db.DB.Clauses(clause.OnConflict{
      Columns:   []clause.Column{{Name: "chat_id"}, {Name: "message_id"}},
      DoUpdates: clause.Set{{Column: clause.Column{Name: "delete_at"}, Value: leastExpr(db.DB)}},
  }).Create(&row)
  ```
  The table qualifier is required on PostgreSQL: `SET delete_at = LEAST(delete_at, EXCLUDED.delete_at)` fails with `ERROR: column reference "delete_at" is ambiguous` [VERIFIED: run]. `sent_at`, `kind`, `created_at` of an existing row are left alone (same message, same date).
- `ClaimDue(ctx, now, limit, token) ([]Row, error)`: one `UPDATE scheduled_deletions SET state='acting', claim_token=?, claimed_at=? WHERE id IN (SELECT id FROM scheduled_deletions WHERE state='pending' AND delete_at <= ? ORDER BY delete_at LIMIT ?) AND state='pending'`, then `SELECT ... WHERE claim_token = ? AND state = 'acting'` [VERIFIED: run: PostgreSQL returned `tok1`; SQLite `claimed 1`]. Two replicas racing: under READ COMMITTED the loser's `UPDATE` waits on the row lock and re-checks the outer `state='pending'`, so each row has one winner, and the token read returns exactly the rows a caller won (A3: reasoned from PostgreSQL's re-evaluation rule; the concurrency test in the integrity list proves it on a real server). Token: 16 `crypto/rand` bytes as 32 hex characters (the settings-input nonce pattern).
- `Complete(ctx, token, ids)` = `DELETE WHERE claim_token = ? AND id IN ?` (a row a stale-release handed to another replica is not removed by the first replica's late completion); `Release(ctx, token, ids, countAttempt)` = state `pending`, token `''`, `claimed_at` NULL, `attempts = attempts + 1` only when counted (the third counted failure deletes instead); `ReleaseStale(ctx, cutoff)` = the same reset for `state='acting' AND claimed_at < ?`, no attempt; `DropOld(ctx, ids)`; `DropChatPending(ctx, chatID, kindPrefix)` for the latch.

**Worker `fuku/modules/deletion_worker.go`, a copy of the lockdown worker's frame** [VERIFIED: lockdown_worker.go:213-302]: `StartDeletionWorker(b)` (mutex, `context.WithCancel`, goroutine with `defer close(done)` and `defer error_handling.RecoverFromPanic`), `StopDeletionWorker()` (cancel, wait for `done` at most `deletionWorkerStopWait`, a `Warn` when it runs out), `deletionWorkerLoop` (one pass at start, then a pass per tick or wake; a pass is cycles back to back until one makes no progress), `runDeletionCycle` with `defer error_handling.RecoverFromPanic("deletionCycle", "Deletion")`; `deletionWake` channel and `wakeDeletionWorker()` (buffered 1, never blocks; called after `Schedule` of a row due now). Numbers: tick 1 s (D-06), batch 200, chunk 100, stale claim 2 minutes, `deletionMaxAttempts = 3`. All timings are package variables for tests.

One cycle: `ReleaseStale(now-2m)`; `ClaimDue(now, 200, token)`; **drop** rows whose `sent_at < now - 47h` quietly (`Complete`); group by chat (sorted for determinism); per chat: negative latch check (`deletionLatched`, below) and a fresh Staff Group read (fail closed: on error release the chat's rows free; on a Staff Group, drop its rows); per ≤100-ID chunk `deletionPaced(ctx, func(callCtx){ bot.DeleteMessagesWithContext(callCtx, chat, ids, nil) })` under a per-call 10 s timeout; success completes the chunk; failure with `retryFree` releases **all** remaining claimed rows free and ends the cycle (the same pacer limits every row, so the rest would only be refused, the lockdown worker's rule at lockdown_worker.go:469-472); `gone` latches the chat (value `gone`) and drops its pending rows; any other failure falls back to per-ID `DeleteMessageWithContext` through the pacer, each ID classified by `deletionClassify` (done/refused complete the row, `retry` releases it counted, `retryFree` as above). **Cycle guard:** a cycle that is still running at `claimed_at + 90 s` releases what it has not finished (free) before returning, so the 2-minute stale release never hands a row to a second replica while the first is mid-flight (a double delete is harmless, the second gets "not found", but this keeps the claim honest). No per-message log line; one Debug line per chat batch with counts only, a Warn once per stop that timed out, never an ID list, text or token.

**Latch (D-07, with C-04).** `fuku:delete:norights:<chat>` is `SET EX 600` with a value that names the scope: `gone` (blocks every kind: the bot is not in the chat, was kicked, chat deleted or upgraded) or `service` (blocks only `service:*`). `gone` comes from the `gone` classifier. `service` is set only by a **live check**, not by a description: when a `service:*` ID is refused with "can't be deleted", at most once per chat per cycle one `getChatMember` of the bot through the pacer decides (the S-01 classifier says "cannot" -> set `service`, drop the chat's pending `service:*` rows; "can" or unknown -> just drop the row). `bot:*` rows are never stopped by a missing right, because a plain-member bot can delete its own messages. `Schedule` and `scheduleBotMessage` consult the latch with one `EXISTS` (no Redis, or a Redis error, means "not latched": the latch only saves calls, correctness never depends on it; `fuku:delete:*` is outside `fuku:cache:`, so `CLEAR_CACHE_ON_STARTUP` keeps it, AGENTS.md "Data").

**Pacer.** `var deletionPacer = ratelimit.NewTelegramPacer(ratelimit.TelegramPacerOptions{NextKey: "fuku:delete:pace:next", BlockKey: "fuku:delete:pace:block", Interval: 100 * time.Millisecond, MaxRetries: 3, MaxWait: 60 * time.Second})` and `deletionPaced(ctx, call)`, the member requirements' shape [VERIFIED: memberreq_enforce.go:29-73, which also wraps each call in a 10 s `context.WithTimeout`]. `Do` repeats the same closure after a 429's `retry_after`, returns `ErrRateLimited` after `MaxRetries` or when a slot or block is more than `MaxWait` away with no Telegram request [VERIFIED: telegram_pacer.go:140-162]. Fleet throughput is bounded by the interval: at most 10 calls/s, so up to 1000 deletions/s with 100-ID chunks, which is the headroom Phase 17 needs.

**Inline delay-0 deletes (D-08) and a cap.** `deletionDeleteNow(chat, messageID)`: `deletionInline.start()` (a counter, the `raidPostTracker` idea: `startBelow` check-and-increment, so a late call after the drain is refused) then `go func(){ defer deletionInline.done(); defer error_handling.RecoverFromPanic("deletionInline", "Deletion"); ... deletionPaced(ctx, DeleteMessage) ... }()` with `ctx` derived from the worker's root context (`deletionContext()`, cancelled by `StopDeletionWorker`, like `memberReqContext`) plus a 15 s timeout. **When the inline counter is at its cap (suggest 64) the call writes a queue row due now instead** (`delete_at = now`, wake): a join flood then degrades to the durable path instead of parking hundreds of goroutines on the pacer. Tests wait with `deletionInline.wait()`.

**Shutdown placement and arithmetic.** Handlers are registered in `main.go`; LIFO runs the last registered first. Registration order [VERIFIED: main.go:161-251]: DB close, users async writes, monitoring, db monitoring, tracer, captcha lifecycle, staff sweeper stop (:204), **admin index sweeper stop (:216-220)**, member requirements drain (:226), staff actions drain (:231), lockdown worker drain (:236), raid posts drain (:240), raid AI drain (:245), AI spam drain (:247), and later (webhook mode) the HTTP stop. Insert the new handler **right after the admin index sweeper's stop (after line 220) and before `modules.RegisterMemberReqDrain`**, so LIFO is: ... staff actions -> member requirements -> **deletion worker** -> admin index sweeper -> staff sweeper -> captcha lifecycle -> tracer -> ... -> DB close. `StartDeletionWorker(b)` goes in `postInit` right after `modules.StartLockdownWorker(b)` (main.go:503).

Arithmetic [VERIFIED: AGENTS.md "Startup and shutdown" for the drain figures; graceful.go:30-31,86-123 for the manager]: the manager runs each handler under `min(its allowance, remaining global budget)`; `shutdownTimeout = 60 * time.Second`, `defaultHandlerTimeout = 10 * time.Second`. Webhook-mode worst case up to and including the member requirements drain = HTTP stop 10 + AI spam 10 + raid AI 2 + raid posts 3 + lockdown 6 + staff 31 + member requirements 3 = **65 s**, already above 60, so when every earlier drain runs to its limit the manager exits with code 1 before reaching any later handler (the staff drain is cut 2 s short). The deletion stop follows it in LIFO. D-10's "unchanged" is true in the sense that the 60 s global budget caps everything and the exit stays inside `stop_grace_period: 75s`, but the **sum** of allowances including the new handler is 65 + its wait. A plain `RegisterHandler` handler allows 10 s; to keep the stated arithmetic small give `StopDeletionWorker` its own `deletionWorkerStopWait = 3 * time.Second` (an internal wait shorter than the 10 s default, so no `RegisterHandlerWithTimeout` is needed): contribution 3 s, **68 s worst case, 60 s enforced, exit 1, every row resumes from the database**. Typical case (nothing near its limit): the cancel ends the loop at once because every Telegram call of a cycle and every inline delete derives from the cancelled root context, and the one `Release` write in flight is bounded by its own 5 s database context. Polling mode: the updater's extra 10 s moves the cut earlier, as AGENTS.md already says for the other drains. AGENTS.md "Startup and shutdown" gets one bullet with these numbers and the sentence that a deletion cut off loses nothing.

**Test seams the worker needs:** `deletionNow` (a `var func() time.Time`, mirror of `raidNow`); `deletionPacer` (a variable so `withFastDeletionPacer(t)` can install a 1 ms interval and a 5 ms `RetryAfterUnit` exactly like `withFastLockdownPacer`, lockdown_fake_test.go:325-339); timings (`deletionWorkerTick`, `deletionStaleClaim`, `deletionBatch`, `deletionChunk`, `deletionInlineMax`, `deletionWorkerStopWait`) as variables restored through `mreqSwap`; the bot client is the shared `lockdownFake` / `staffActionFake` (it needs `deleteMessages` and `deleteMessage` handlers: see Wave 0).

### S-06. Settings page (question 6)

**M24 `Hidden`.** `settingsPage` [VERIFIED: settings_pages.go:350-373] gains `Hidden func(group *gotgbot.Chat) bool` (nil = today's behaviour). Two edits only: in `settingsMainEntries` (settings_pages.go:479-493, the single place Main entries are built; `settings.go:219` and `settings_menu.go:232` both call it) add `if pg.Hidden != nil && pg.Hidden(group) { continue }` before the `MainState` call; in `openPage` (settings_menu.go:508-522) add, after the `OwnerOnly` guard, `if pg.Hidden != nil && pg.Hidden(p.group) { p.answer(settingsText(p.tr, "common_callback_invalid_request"), true); return }`. `openPage` is also reached from `checkRight` (settings_menu.go:549-551) and `dispatch` for `op`/`no`, so every route is covered. Page actions (`dl`, `dr`) and controls (`st`) bypass `openPage` (`runPageAction`/`runControl` call `Run`/`Set` directly, settings_menu.go:557-589), which is why the page also starts each of them with its own Staff Group gate (S-06 "Gate"). The page's `Hidden` is a fresh `staff.GetStaffGroupFresh(group.Id)` (`(nil, nil)` for no row, an error otherwise [VERIFIED: staff/repository.go:204-215]): true only for a definite row; its `MainState` makes the same fresh read and returns `ok=false` on error (UI-CHECK F1). Cost: one indexed read per Main draw for `Hidden` and one for `MainState`.

**Registration and actions.** One `init()` in `settings_deletion.go`: `registerSettingsPage(settingsPage{Key: "d", Order: 85, Icon: "🧹", TitleKey: "settings_page_del", MainState, Open, Hidden, Controls, Actions})`; the registry panics at init on a duplicate page key or action code [VERIFIED: settings_pages.go:426-443]. `Actions: {"dl": {Fields: []string{"t"}, Right: settingsRightAdmin, Validate: sectionCodeOnly, Run}, "dr": {... rowCodeOnly ...}}`. `settingsCheckFields` default branch adds `entry.action.Fields` to the allow-list, requires only `a` and the owner key, and calls `Validate(fields)` [VERIFIED: settings_menu.go:271-283], so `Validate` must itself require `t` (and nothing else) and check it against the section or row table. **Controls: one control per row code** in `Controls map[string]settingsControl` (`t` is the key; `settingsControlFieldsValid` looks up `pg.Controls[fields["t"]]`, requires the action in `Actions`, and calls `Validate(fields["v"])` [VERIFIED: settings_menu.go:303-316]); each is `{Right: settingsRightChangeInfo, Actions: []string{"st"}, Validate: presetValidator(row.Presets), Set: closure over the row}`; the 17 closures are built in a loop from the two tables, so no code branches on a row's identity. `presetValidator` accepts `"n"` or the canonical decimal of a preset (no sign, space, non-ASCII digit or leading zero; `0` is the one zero), which `settingsParseNumber(v, 5)` [settings_pages.go:119] plus a round-trip `strconv.Itoa` comparison gives. Right: `settingsRightChangeInfo` [VERIFIED: settings_menu.go:73-80, 105-114] needs the creator, or an administrator with `CanChangeInfo`, judged on the press's own member record with no extra call, and refuses with the alert `settings_no_change_info`.

**Draws and the right line.** Every draw follows the Member requirements pattern [VERIFIED: settings_mreq.go:253-275]: read the database first (a failed read is the cannot-open edit), `p.answer("", false)`, then the status read, then the edit; `settingsDelReadRight(p)` runs `deletionPaced` + `GetChatMemberWithContext` under `settingsDelHereTimeout = 3 * time.Second` (a variable, like `settingsMreqHereTimeout`, settings_mreq.go:196) and classifies per S-01. The line is `🚫 ` or `❔ ` plus the key, the icon a code-side prefix. A choice press: pipeline calls + one `getChatMember` for the redraw.

**Gate (D-18, defence in depth).** One helper `settingsDelStaffGate(p) (blocked bool)` at the top of `dl`, `dr` and every control's `Set`, after the pipeline: fresh `GetStaffGroupFresh(p.group.Id)`; a row means `p.answer("common_callback_invalid_request", true)` and nothing edited or written; an error means `p.cannotOpen()` and nothing written; no text names a Staff Group.

**Apply functions.** One per behaviour, beside the code that owns the setting: `applyDeleteRule(ctx, chatID, kind, delay)` for the ten non-joined/left service rows and the five bot rows (validate against the registry, `deletion.SetRule` / `RemoveRule`, the cache key dropped by the repository, the rules read back through the cached loader); `applyCleanServiceRule(ctx, chatID, kind, state)` for joined and left (S-03). A press of the stored state writes nothing (judged on the cached loader; the key is in `skipLocal`).

**What opening costs / what must change when a ninth Main cell appears.** Tests that hard-code the real Main grid [VERIFIED by grep]: `fuku/modules/settings_edit_test.go:990` and `fuku/modules/settings_mreq_test.go:231` and `:267` assert `"s|l / c|f / w|g / r|m / a / close"` and `"s|l / c|f / w|g / r|m / a / groups|close"`; with page `d` at Order 85 they become `"s|l / c|f / w|g / r|m / d / a / close"` and `"... / d / a / groups|close"`. `TestSettingsMainGrid` builds its entries by hand (`renderSettingsMain(view, []settingsMainEntry{...})`, settings_main_test.go:74-90), so it does not break (the UI-SPEC's "must change" is optional there; add a nine-cell subtest). The registry-driven tests adapt on their own: `settings_main_test.go:208-271` (end to end, computes the shape from `settingsPagesInOrder()`), `settings_menu_test.go:470-505` (opens every page by `op` and requires the last row `back|close`, and Back to Main: **the hub's `Open` runs in these tests, so the new tables must be in the harness AutoMigrate list first**), `settings_test.go:160-185` (non-owner Main, dynamic rows), `settings_edit_screens_test.go:100-120` (Main with every page). `settingsMainWorstState` (settings_main_test.go:~240) needs `settingsText(tr, "settings_state_del_on", TranslationParams{"n": 99})` in its candidates so `TestSettingsMainPageFitsEveryLocale` holds the page to the worst state. `settings_staff_privacy_test.go:180` builds an explicit Main entry list (add the Deletion entry and a Staff-Group case with none). `fuku/i18n/settings_locale_test.go`: `settingsLocaleLimits` (+57 rows; the comment at :301 says "255 rows in all"), `TestSettingsLabelLengths` pages list (:511-522 gains `{"🧹", "settings_page_del"}`), the `options` list (:529-534 gains the new choice labels) and a new group for the 2 + 12 + 5 names with each of `➖ 🗑 ⏳`, `memberReqQuotingKeys` (gains `settings_del_hint_punish_auto`); `TestSettingsHelpFitsOneMessage` is unaffected (help text unchanged, A16-A11).

### S-07. Backup, export, import, reset (question 7)

The exportable set is the explicit list `AllExportableModules()` [VERIFIED: backup/types.go:115-137]: admin, antiflood, antiraid, approvals, blacklists, captcha, connections, disabling, filters, greetings, locks, notes, pins, reactions, reports, rules, warns, federations, log channels. A table in no module stays out by construction. Add `TestDeletionTablesStayOutOfBackup` in `fuku/db/backup/` copying `TestMemberRequirementsStayOutOfBackup` [VERIFIED: memberreq_backup_test.go:19-84]: seed rules and a queue row for the source chat, `ExportChatData` (assert `len(exported.Data) == len(AllExportableModules())`, `ExportChatData(..., []string{"chat_delete_rules"})` and `"scheduled_deletions"` error, the JSON contains neither table name nor `delay_secs`), `ImportChatData` into a second chat creates neither row, `ClearChatData` leaves both. The new models join that package's `testmain_test.go` AutoMigrate list (backup/testmain_test.go:41-80). **The `greetings` flag:** `GreetingSettings.ShouldCleanService` has `json:"clean_service_settings"` [VERIFIED: models/greetings.go:39] and `clearGreetings` rebuilds the row from defaults with the flag false (backup.go:1014-1029), so an import carries the flag and a reset turns it off; the D-02 rule (a row counts only while the flag is true) means an import or reset can never leave a stale row in charge. No column is added to greetings, so `exportGreetingsData` is unchanged.

### S-08. Risks and pitfalls specific to this phase (question 8): see "Common Pitfalls".

### S-09. Test strategy (question 9): see "Validation Architecture" and "Proposed plan split".

## Corrections to CONTEXT.md and the UI-SPEC (the planner must carry these)

| # | Where | Problem | Change |
|---|-------|---------|--------|
| C-01 | D-05 / D-07 | D-07 drops "a row more than 47 hours past its message's own date", but D-05's columns hold no message date (`delete_at` = date + a delay the row does not record, `created_at` = when the row was written, which lags a message delivered late). The rule cannot be computed exactly. | Add `sent_at TIMESTAMPTZ NOT NULL` (the message's own send time) to `scheduled_deletions`; `Schedule` takes `sentAt` and computes `delete_at = sentAt + delay` itself (also what D-08 says); the worker drops `sent_at < now - 47h`. The age check stays advisory: Telegram's own refusal ("can't be deleted") drops the row anyway. Phase 17's self-destruct reuses `sent_at` |
| C-02 | UI-SPEC row `sf`, `settings_del_desc_forum`, D-03 | The docs say forum topic creation service messages "can't be deleted"; `ForumTopicCreated` is in the row's predicate and its description says "created". It over-promises and wastes a call per topic. | Drop `ForumTopicCreated` from `sf` (keep it in the never-deleted list), and drop the word "created" from `settings_del_desc_forum` in all 8 locales (key name and limits unchanged) |
| C-03 | UI-SPEC row `sg` ("`Giveaway*`") | The `Giveaway` field is the giveaway post an admin made (content), not a service message; `Giveaway*` would delete it. | `sg` = `GiveawayCreated`, `GiveawayWinners`, `GiveawayCompleted`, `Gift`, `UniqueGift`, `GiftUpgradeSent` |
| C-04 | D-07 latch | A "no delete right" refusal cannot be told from "message can't be deleted" by text (the repo's own handling treats both as one string), and a plain-member bot can still delete its own messages, so a text-driven latch would stop `bot:*` deletion in groups where it works. | Latch value carries a scope: `gone` blocks everything, `service` blocks only `service:*` and is set only after a live `getChatMember` shows the bot cannot delete (S-05). A missing right never stops `bot:*` rows |
| C-05 | D-10 | "worst case of 65 s unchanged" is true only because the 60 s global budget caps; the sum with the new handler is 65 s + its allowance (10 s by default). | Give `StopDeletionWorker` an internal wait of 3 s; state 68 s worst case / 60 s enforced in AGENTS.md; keep `RegisterHandler` |
| C-06 | D-19 | `cleanService` has no `SetAllowBot`, so the join/leave service message of a user added or removed by a bot never reaches it (today `/cleanservice` misses it too). D-19 gives the watcher "every other" type. | The watcher also handles `sj`/`sl` when `ctx.EffectiveSender.IsBot()` (S-03); `cleanService` stays unchanged otherwise |
| C-07 | D-14 | Two points D-14 leaves open: (1) kind Off + a mute notice with a button; (2) the base time of the reschedule after "I fixed it": `message date + delay` can already be in the past (a 10-minute timer, a tap after 20 minutes), so the confirmation would vanish within a second. | (1) Off means Off: no row; any non-Off delay means the mute notice is scheduled at 86400 s. (2) The "I fixed it" reschedule uses `now` as its base (`sent_at = now` for that call, since the message was just edited), so the member reads the confirmation for the timer's full length; the row keeps `LEAST` semantics. Two lines in `scheduleBotMessageAt(base time)` |
| C-08 | D-08 vs existing tests | `TestCleanServiceProcessesJoinAndDeletesServiceMessage` and `TestCleanServiceProcessesMultipleNewMembersWithoutCaptcha` (greetings_command_test.go:442-490) assert `client.callsFor("deleteMessage")` has length 1 right after `cleanService` returns; an inline goroutine makes that racy. | Both tests call `deletionInline.wait()` before the assertion (a behavioural test, not a Phase 12 characterization test, so editing it is allowed); the new tracker is the seam |
| C-09 | tests | The shared `staffActionFake` answers every `sendMessage` with `"date":1`; the worker's 47 h rule and `delete_at = date + delay` would put every hooked message in 1970. | Wave 0: a `sendDate func() int64` field on `staffActionFake` (default returns 1, so every existing test is unchanged) set to `deletionNow().Unix()` by the deletion environment |

Not wrong, but note for the owner's spike file: A16-A6's wording is satisfied by the softened `settings_del_no_right` text (S-01); the UI-SPEC's claim that `TestSettingsMainGrid` must change is optional (hand-built entries).

## Architecture Patterns

### System Architecture Diagram

```
 Telegram update (service message) ──► dispatcher
        │
        ├─ group -10 captcha / -7 lockdown guard / -6 fed-ban   (may end the update: nothing below sees it)
        ├─ group -5  serviceDeleteWatcher  (AllowBot) ── kind = deletionServiceKindOf(msg)
        │              │ rules = deletion.GetRules (cached, skipLocal)  ── no rule ─► ContinueGroups
        │              │ staff gate (fresh)  ── Staff Group ─► ContinueGroups
        │              ├─ delay 0  ─► deletionDeleteNow ─► goroutine ─► deletionPaced ─► deleteMessage
        │              └─ delay N  ─► deletion.Schedule ─► scheduled_deletions (LEAST upsert) ─► wake
        └─ group 0   cleanService (joined/left): ProcessJoins ─► effective rule (flag AND row) ─► same two paths

 Bot send sites (welcome, goodbye, /rules, requirement notices, antiflood, blacklist, aispam breaker, manual replies)
        └─► scheduleBotMessage(sent, kind) ── off / private / Staff Group / latched ─► nothing
                                            └─► deletion.Schedule(sentAt + delay)

 DeletionWorker (every replica, tick 1 s + wake)
   ReleaseStale(2 min) ─► ClaimDue(200, token) ─► drop sent_at < now-47h ─► per chat: latch? staff? ─►
   deleteMessages(≤100 ids, deletionPaced) ──ok──► Complete(token)
        │ fail ─► per-ID deleteMessage ─► classify: done/refused ─► Complete │ retry ─► Release(+1) │ retryFree ─► Release(free), end cycle
        └ gone ─► latch fuku:delete:norights:<chat> (10 min), drop chat rows

 Settings: Main cell "d" ─► hub ─► dl(t=s|b) list ─► dr(t=row) screen ─► st(t=row, v) ─► apply fn ─► rules / flag+row tx ─► DeleteCache
           every draw: p.answer("") ─► 1 live getChatMember(bot) via deletionPaced ─► right line
```

### Recommended Project Structure

```
migrations/20261110120000_add_scheduled_deletion.sql
fuku/db/models/delete_rule.go, scheduled_deletion.go
fuku/db/deletion/            rules.go (cached read, SetRule, RemoveRule, ApplyJoinLeftRule), queue.go (Schedule, ClaimDue, Complete, Release, ReleaseStale, DropChatPending), kinds.go (the Go kind registry shared with modules), testmain_test.go
fuku/db/greetings/           + SetCleanServiceTx
fuku/db/cache/local.go       + delete_rules in skipLocal
fuku/modules/deletion_kinds.go     service table (Match predicates), bot kinds, presets, validators
fuku/modules/deletion_service.go   watcher (group -5), cleanService helper, deletionDeleteNow, inline tracker
fuku/modules/deletion_worker.go    pacer, loop, cycle, classify, latch, Start/Stop, Register in main.go
fuku/modules/deletion_bot.go       scheduleBotMessage, scheduleBotMessageAt
fuku/modules/settings_deletion.go  page, tables, renderers, dl/dr, controls, right reader, staff gate
fuku/modules/deletion_*_test.go, settings_deletion_*_test.go
```

### Pattern 1: Opt-in scheduling helper that never fails its caller
**What:** `scheduleBotMessage` reads cached rules, applies every refusal, writes one row, logs and swallows errors. **When:** every named send site. Never hook `helpers.SendMessageWithErrorHandling` or `media.Send`.

### Pattern 2: Claim by conditional update, release by token
**What:** `ClaimDue` (one `UPDATE ... WHERE id IN (subselect) AND state='pending'` + token read), `Complete`/`Release` guarded by the token. **Why:** portable to SQLite, one winner per row on PostgreSQL, a late completion of a stale claim removes nothing.

### Pattern 3: Table-driven page (no identity branches)
**What:** the section table and the row table carry `Code, Kind, NameKey, DescKey, HintKey, Presets`; renderers, validators and the Main count read them. A Phase 17 section is a table row.

### Anti-Patterns to Avoid
- Returning `nil` from the watcher (kills every later group-5-and-later handler of its group): preset the named result to `ext.ContinueGroups`.
- Qualifying nothing in the PostgreSQL upsert (`ambiguous` error, verified).
- Reading the greetings flag with `GetGreetingSettings` (writes a row, hides errors).
- Using `CanBotDelete` or the admin cache for the right line (cached; D-17 says live).
- Counting an attempt on a 429 or a shutdown, or latching on a text match (C-04).
- A per-message log line for a dropped row (D-07), or logging ids/tokens in the worker.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Fleet-wide Telegram pacing and 429 handling | a limiter | `ratelimit.NewTelegramPacer` (`deletionPacer`) | slot + shared block across replicas, shed at `MaxWait` |
| Worker frame (start/stop/wake/recover) | a new scheduler | copy `lockdown_worker.go` | tested shape, shutdown semantics already argued in AGENTS.md |
| Per-chat cached read | a map with a mutex | `cache.GetFromCacheOrLoad` + `skipLocal` | cross-replica freshness, generation guards |
| Callback encoding | `strings.Split`, `fmt.Sprintf` | `encodeCallbackData` / `settingsKeyboard.button` | 64-byte cap handling |
| Bot-right classification | new status switch | the shape of `memberReqClassifyBot` | tri-state, code-based, tested |
| Press pipeline, rights, notices | custom checks | `settingsPress.verify`, `settingsRightChangeInfo`, `p.answer` | owner gate and live admin check on every press |
| Time-ordered claims | `SKIP LOCKED` | conditional update + token | SQLite tests, D-06 |

**Key insight:** every piece except the table and the kind registry already exists in this repo; the phase's risk is in the seams (existing tests, fake dates, the two dialects), not in new machinery.

## Common Pitfalls

### Pitfall 1: A watcher that returns `nil`
**What goes wrong:** every later handler in the watcher's group never runs. **Why:** the dispatcher `break`s the group on a matched handler that returns no error [VERIFIED: dispatcher.go:323-324]. **Avoid:** named result preset to `ext.ContinueGroups`, recovered panic keeps it. **Detect:** `TestServiceWatcherNeverEndsUpdate` with an injected panic and a second handler in the group.

### Pitfall 2: Stale claims and double deletes across replicas
**What goes wrong:** a cycle slower than the 2-minute stale release gets its rows re-claimed. **Why:** pacer waits up to 60 s per call, three 429 retries, 100 ms per chunk. **Avoid:** the 90 s cycle guard (release unfinished rows free), `Complete` guarded by the token, and the fact that a second delete is "not found" and dropped quietly (harmless). **Detect:** `TestDeletionWorkerOneDeleteAcrossReplicas`, `TestDeletionWorkerStaleClaimNotDoubleProcessed`.

### Pitfall 3: A 429 storm
**What goes wrong:** every cycle claims 200 rows and releases them. **Avoid:** `retryFree` ends the cycle and releases free; the pacer's shared block makes the next cycle's first call fail at once with no Telegram request (2 cheap queries per second per replica). **Detect:** `TestDeletionWorkerRateLimitedFree` (no attempts counted, no Telegram request while blocked).

### Pitfall 4: `msg.Date` zero, future, 1970
**Avoid:** clamp (S-04); store UTC seconds. Fakes answer `date:1` (C-09); a Phase 12-style characterization of "date 0" input belongs in `TestScheduleBotMessageClampsDate`.

### Pitfall 5: Rows for chats the bot left
**What goes wrong:** pending rows for a kicked/deleted chat are claimed forever. **Avoid:** the `gone` classifier drops the chat's pending rows and latches 10 minutes; later schedules for that chat write nothing while latched (and after the latch lapses a stale row simply fails once more and re-latches).

### Pitfall 6: The welcome replies to a join message that was deleted
**Not an issue:** the welcome sends with no reply target (`ReplyMsgID: 0`), only the captcha challenge and the `chat_member` path matter, and the join-message deletion happens after `ProcessJoins` in `cleanService`. A deletion of the join service message before a captcha reply would break the challenge's reply target, which is exactly D-19's reason; the watcher never handles `sj`/`sl` for a non-bot sender.

### Pitfall 7: Lockdown worker deletes join messages itself
**No overlap:** the guard ends banned joiners' updates at -7 (EndGroups), so neither `cleanService` nor the watcher sees them, and the lockdown worker's deletion uses its own table and pacer.

### Pitfall 8: `cleanwelcome` / `cleangoodbye` interplay
**Safe:** they delete the previous greeting by `LastMsgId` right after the new one is sent (greetings.go:960-965, 1052-1057); the queue row of the new message is independent, and if the old message also has a row its delete is "not found" and dropped quietly. `LEAST` keeps the earlier time when both want the same message.

### Pitfall 9: Anonymous admins and bots as senders
**Covered:** `IsBot()` is false for an anonymous admin (`Chat != nil`), so no `SetAllowBot` is needed for them but is for bot senders (S-02). Manual-punishment hooks work for anonymous-admin re-entry because the re-entered command runs the same `moderationCommand.run` with the swapped message (AGENTS.md "Anonymous admins bypass WrapCommand"); no special case.

### Pitfall 10: The latch versus a bot promoted later
**Avoid:** the latch lasts 10 minutes by design (an owner who fixes the right waits at most that); `service`-scoped only after a live check, so a bot that is promoted just after a refusal loses at most 10 minutes of `service:*` deletion and `bot:*` is never latched by rights. The page's right line is live every draw, so the owner sees the fix at once.

### Pitfall 11: Supergroup migration
**Decision:** do not re-key rules or rows. AGENTS.md says other per-chat settings are not migrated on a chat ID change [VERIFIED: AGENTS.md "Staff Group chat migration is re-keyed by `staff.RekeyChat` ...; other per-chat settings are not migrated"], so the new supergroup starts with Keep/Off like every other setting; pending rows of the old chat fail with a `gone` answer (upgrade, `MigrateToChatId`) and are dropped. One Info line in AGENTS.md.

### Pitfall 12: Forum topics
Deletion is by chat and message ID and ignores threads; `ForumTopicCreated` is the one undeletable service type (C-02).

### Pitfall 13: Huge queues (Phase 17 preview)
One row per member message in a busy group: the partial index covers only pending rows, rows are **deleted on completion** (no soft state, so the table stays at "messages awaiting their time"), `ListDue` is `ORDER BY delete_at LIMIT 200` through the partial index (plan verified), loops back to back while progress is made (throughput is pacer-bound, 1000 deletions/s fleet-wide at 100 per call), `ReleaseStale` uses `idx_scheduled_deletions_acting`. Dead-row growth is only latched/old rows, dropped each cycle.

### Pitfall 14: Test hygiene
Fresh chat IDs from `uniqueModuleChatID()`; `t.Cleanup` deletes rule rows, queue rows, greetings rows and the cache keys (`delete_rules:<chat>`, `greetings:<chat>`) and the `fuku:delete:*` keys; swap every package variable through `mreqSwap`; a test that calls `StopDeletionWorker` restores the worker (`restoreDeletionWorker`, like `restoreMemberReqRunner`); no `t.Parallel`, no `t.Skip`.

## Code Examples

### The claim (verified on PostgreSQL 16 and SQLite)
```go
// ClaimDue: one conditional update, then a read by token (D-06). Source: run in this research.
res := db.DB.WithContext(ctx).Exec(
    `UPDATE scheduled_deletions SET state = 'acting', claim_token = ?, claimed_at = ?
       WHERE id IN (SELECT id FROM scheduled_deletions
                    WHERE state = 'pending' AND delete_at <= ? ORDER BY delete_at LIMIT ?)
         AND state = 'pending'`, token, now, now, limit)
// then: db.DB.Where("claim_token = ? AND state = 'acting'", token).Find(&rows)
```

### Settings controls from the tables
```go
// Source: pattern of settingsControl (settings_pages.go:376-390) and settingsControlFieldsValid (settings_menu.go:303-316)
controls := map[string]settingsControl{}
for _, row := range allDeletionRows() { // service rows then bot rows, one table
	row := row
	controls[row.Code] = settingsControl{
		Right: settingsRightChangeInfo, Actions: []string{settingsActSet},
		Validate: presetValidator(row.Presets),
		Set:      func(p *settingsPress, v string) *settingsScreen { return settingsDelChoose(p, row, v) },
	}
}
```

### Inline delay-0 delete with the cap
```go
func deletionDeleteNow(chatID, messageID int64) {
	if !deletionInline.startBelow(deletionInlineMax) { // full or draining: take the durable path
		_, _ = deletion.Schedule(context.WithoutCancel(...), deletion.Entry{ChatID: chatID, MessageID: messageID, DeleteAt: deletionNow()})
		wakeDeletionWorker()
		return
	}
	go func() {
		defer deletionInline.done()
		defer error_handling.RecoverFromPanic("deletionInline", "Deletion")
		ctx, cancel := context.WithTimeout(deletionContext(), 15*time.Second) // cancelled by StopDeletionWorker
		defer cancel()
		_ = deletionPaced(ctx, func(callCtx context.Context) error { _, err := bot.DeleteMessageWithContext(callCtx, chatID, messageID, nil); return err })
	}()
}
```

## State of the Art

| Old Approach | Current Approach | Impact |
|--------------|------------------|--------|
| `/cleanservice` deletes joined/left at once, nothing else | one rule table + watcher, joined/left keep the flag | D-02 keeps the command and backup unchanged |
| `deleteMessage` per message (repo, everywhere) | `deleteMessages` up to 100 ids | first use of the batch method; fallback per ID until the spike proves the edge cases |

**Deprecated:** none.

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | Telegram's description for a supergroup upgrade contains "upgraded to a supergroup" and a "not enough rights" text may exist for a missing delete right; the classifier matches them but never depends on them | S-01 | Low: unmatched strings fall to "4xx other than 429 drops the row" or "retry" (the third failure drops), so only a few extra calls |
| A2 | A basic-group administrator's `getChatMember` may report `can_delete_messages` false although the docs say an administrator of a group can delete any message | S-01 | A false 🚫 line in a basic group; fixed by classifying administrator by chat type (supergroup needs the flag) |
| A3 | Under READ COMMITTED the losing `UPDATE ... WHERE id IN (...) AND state='pending'` re-checks the outer predicate and skips the row, so each row has one winner | S-05 | Two replicas could both act on a row (a double delete, harmless); `TestClaimBatchOneWinner` on PostgreSQL proves it |
| A4 | `deleteMessages` either skips or fails on an ID that exists but cannot be deleted; the pinned docs only say "can't be found ... skipped" | S-01 | The owner's batch spike settles it; the code is safe under both |
| A5 | Telegram's service messages by a bot sender carry `From` = that bot with no `sender_chat` (so `IsBot()` is true) | S-02 | If wrong, `SetAllowBot` is merely unnecessary for them |
| A6 | `staffActionFake`'s `date:1` is the only date source the new hooked tests meet; other fakes (`moduleBotClient`) return their own dates | S-04, C-09 | A test that asserts a row would see 1970 and fail loudly (no silent pass) |
| A7 | Telegram error text for a 429 on `deleteMessages` carries `retry_after` like other methods, so `ratelimit.RetryAfterSeconds` applies | S-05 | The pacer treats a 429 without it as a plain failure and the row is retried (counted); low |

## Open Questions

1. **A stale delay row after `/cleanservice off` then `on`.**
   - Known: D-02 says the row counts only while the flag is true, so `off` then `on` re-activates an old `After 1 h` instead of Delete now.
   - Unclear: whether the owner expects the command to reset the delay.
   - Recommendation: keep D-02 literally (no change to `/cleanservice`); the page shows the effective state, so the owner sees it. If wanted, the command's apply function deletes the two rows in the same transaction (one site).
2. **aispam notice (S-04).** aispam sends no punishment notice; only the breaker notice exists. Recommendation: schedule it as `bot:punish_auto` (D-12 names aispam), a three-line change; drop it if the owner thinks a health notice is not a punishment.

No product-level blocking question for the owner.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go toolchain | build, tests | yes | go1.26.0 linux/amd64 | none needed |
| CGO + SQLite driver | repository and module tests | yes | the repo's `gorm.io/driver/sqlite` ran an AutoMigrate and upsert here | none needed |
| PostgreSQL server | integrity tests (CI), migration check | not running here; binaries present (`/usr/lib/postgresql/16`) | 16 (I started a throwaway instance, ran the migration twice and the upsert/claim SQL, then stopped it and deleted `/var/tmp/pgprobe16`) | CI runs `make test-postgres-integrity` |
| Redis | pacer, latch | `redis-server` present; tests use miniredis v2.39.0 | n/a | tests use miniredis |
| `make`, `golangci-lint` | gates | per AGENTS.md | n/a | n/a |
| Live Telegram | the owner's batch-delete spike | no | n/a | deferred to `16-SPIKE.md` |

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go `testing` with `-tags testtools -race`; hand-written `gotgbot.BotClient` fakes; miniredis; SQLite via `internal/testdb.Run` (PostgreSQL for the integrity list); testify only in `fuku/db` tests |
| Config file | none (build tag); `Makefile` `test`, `test-postgres-integrity` |
| Quick run, repository | `go test -tags testtools -race -count=1 ./fuku/db/deletion ./fuku/db/cache ./fuku/db/backup ./fuku/db/migrations` |
| Quick run, service and worker | `go test -tags testtools -race -count=1 -run '^(TestService\|TestDeletion\|TestScheduleBot\|TestShutdownEndsDeletion\|TestCleanService)' ./fuku/modules` |
| Quick run, settings page | `go test -tags testtools -race -count=1 -run '^(TestSettings\|TestLoadSettings)' ./fuku/modules ./fuku/i18n` |
| Full suite | `make test` (`fuku/modules` 9-15 min) |
| Phase gate | `make test`, `make lint`, `make check-translations`, `make check-docs`, `make test-postgres-integrity` (CI) before `/gsd-verify-work` |

### Phase Requirements to Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| DEL-01 | table covers every `gotgbot.Message` field exactly once (service / never / content) | unit (reflect) | `-run '^TestServiceKindsCoverMessageFields$'` | Wave 0 |
| DEL-01 | each of the 12 rows: predicate matches only its fields; creation, migrate and `ForumTopicCreated` and `Giveaway` never match | unit, table | `-run '^TestServiceKindTable$'` | Wave 0 |
| DEL-01 | tracer: a pinned-message notice is deleted Now through the real dispatcher and a seeded rule; no rule keeps it | integration | `-run '^TestServiceWatcherDeletesNow$'` | Wave 0 |
| DEL-01 | After N writes one row at `date + N`; a second schedule keeps the earlier; delay 0 writes no row | integration | `-run '^(TestServiceWatcherSchedulesAfterN\|TestServiceWatcherZeroWritesNoRow)$'` | Wave 0 |
| DEL-01 | watcher: bot sender, anonymous admin, no sender all seen; always `ContinueGroups`, also on a panic; a later handler of the group still runs | integration | `-run '^(TestServiceWatcherSenders\|TestServiceWatcherNeverEndsUpdate)$'` | Wave 0 |
| DEL-01 | joined/left: effective rule matrix (flag off + stale row = Keep; flag on, no row = Now; flag on + row = delay); delete after join processing; bot-sender join handled by the watcher only | integration | `-run '^(TestCleanServiceEffectiveRule\|TestCleanServiceDeletesAfterJoin\|TestServiceWatcherBotJoin)$'` | exists (edit: C-08) / Wave 0 |
| DEL-01 | apply function: one transaction (flag + row), both cache keys dropped, Keep/Now/After; parity with `/cleanservice` | repo + settings | `-run '^(TestApplyCleanServiceRule\|TestSettingsParityCleanService)$'` | Wave 0 |
| DEL-02 | `scheduleBotMessage`: kind off writes nothing, private/channel/Staff Group/latched refuse, date clamp, never errors the caller | integration | `-run '^(TestScheduleBotMessage\|TestScheduleBotMessageClampsDate)$'` | Wave 0 |
| DEL-02 | each site schedules its kind: welcome, goodbye, `/rules` group message and click-to-read; private rules and previews never | integration | `-run '^(TestBotWelcomeScheduled\|TestBotGoodbyeScheduled\|TestBotRulesScheduled\|TestBotRulesPrivateNeverScheduled)$'` | Wave 0 |
| DEL-02 | manual replies: table over ban, tban, dban, kick, dkick, mute, tmute, dmute, unban, unmute, warn, dwarn, swarn schedule `bot:punish_manual`; errors, kickme, banme, lists, cards do not | integration, table | `-run '^(TestBotManualPunishmentReplies\|TestBotManualRepliesNotCovered)$'` | Wave 0 |
| DEL-02 | automatic notices: antiflood, blacklist mute/ban/kick/warn (source auto), aispam breaker schedule `bot:punish_auto`; `/warn` is manual | integration | `-run '^(TestBotAutoPunishmentNotices\|TestWarnThisUserSource)$'` | Wave 0 |
| MREQ-15 | requirement notices (warn, kick, ban, mute, pause) scheduled at the timer; a mute notice at 86400; Off writes nothing | integration | `-run '^(TestMemberReqNoticeScheduled\|TestMemberReqMuteNoticeAtCap)$'` | Wave 0 |
| MREQ-15 | "I fixed it" reschedules the notice at the timer from now and moves the 24 h row up | integration | `-run '^TestMemberReqFixedRescheduleMovesUp$'` | Wave 0 |
| DEL-06 | `Schedule` keeps the earliest time on both dialects; Go zero delay; unique (chat, message) | repo | `go test ./fuku/db/deletion -run '^(TestScheduleKeepsEarliest\|TestScheduleRules)$'` | Wave 0 |
| DEL-06 | one claim winner among concurrent callers; token-guarded complete/release; stale release | repo, concurrency (PostgreSQL + SQLite) | `go test ./fuku/db/deletion -run '^(TestClaimBatchOneWinner\|TestReleaseStaleClaims\|TestCompleteIsTokenGuarded)$'` | Wave 0 |
| DEL-06 | migration statements run twice, CHECK 0..86400, partial index exists, defaults | unit + PostgreSQL | `go test ./fuku/db/migrations -run '^(TestScheduledDeletionMigrationStatements\|TestScheduledDeletionMigrationDefaults)$'` | Wave 0 |
| DEL-06 | worker: batch of 100 via `deleteMessages`; batch failure falls back per ID; gone/can't-be-deleted drop quietly with no log line; 5xx counts, third failure drops; 429 and shutdown free | integration | `-run '^(TestDeletionWorkerBatches\|TestDeletionWorkerFallsBackPerID\|TestDeletionWorkerClassifiesErrors\|TestDeletionWorkerRateLimitedFree)$'` | Wave 0 |
| DEL-06 | rows older than 47 h from `sent_at` dropped; delay never above 24 h; restart resumes; two replicas delete once | integration | `-run '^(TestDeletionWorkerDropsOldRows\|TestDeletionDelayCapped\|TestDeletionWorkerResumesAfterRestart\|TestDeletionWorkerOneDeleteAcrossReplicas)$'` | Wave 0 |
| DEL-06/PLAT-06 | cancel ends a cycle inside a hung call, rows back to pending, a restart resumes; inline goroutines end; stop is bounded by 3 s | integration | `-run '^TestShutdownEndsDeletionWorker$'` | Wave 0 (`shutdown_drains_test.go`) |
| PLAT-06 | deletion pacer independent of the other pacers; a block on one leaves the others alone | integration | `-run '^TestDeletionPacerIsIndependent$'` | Wave 0 |
| DEL-07 | no kind can name a staff, lockdown, captcha or log message (kind list audited; call sites not in those files) | unit | `-run '^TestDeletionKindsNeverStaffLockdownCaptchaLog$'` | Wave 0 |
| DEL-07 | `Schedule`, the watcher and the worker refuse a Staff Group (no row, no call); a chat that became a Staff Group after rows were written | integration | `-run '^(TestScheduleRefusesStaffGroup\|TestServiceWatcherSkipsStaffGroup\|TestDeletionWorkerSkipsStaffGroup)$'` | Wave 0 |
| DEL-07 | page: Main has no Deletion cell in a Staff Group, `op`, `dl`, `dr`, `st` answered `common_callback_invalid_request`, no `getChatMember`; failed read is cannot-open | integration | `-run '^(TestSettingsDeletionStaffGroup\|TestSettingsDeletionHidden)$'` | Wave 0 |
| DEL-08 | right line: creator/admin with the right draws none; admin without, member, restricted, left, kicked, 400/403 draw 🚫 once; 429, 5xx, transport, timeout, shed, null draw ❔ once, never an error; basic-group admin; rule still settable | integration | `-run '^TestSettingsDeletionRightLine$'` | Wave 0 |
| DEL-01/02 | page: row budget, widths, callback bytes (max 50), validation (every forged shape refused before I/O), states and words, writes, rights, drift, parity, costs | integration | `-run '^(TestSettingsDeletion)'` | Wave 0 |
| PLAT-05 | 57 keys in 8 files, ceilings, placeholders, quoting, labels at 20 / 13 / 15 code points | i18n | `go test ./fuku/i18n -run '^(TestSettingsLocaleKeys\|TestSettingsLabelLengths\|TestSettingsReusedKeyPlaceholders\|TestMemberReqLocaleKeys)$'` | exists (extend) |
| DEL-06/07 | backup/export/import/reset leave both tables alone; `delete_rules` is in `skipLocal` | repo | `go test ./fuku/db/backup -run '^TestDeletionTablesStayOutOfBackup$'`; `./fuku/db/cache -run '^TestSkipLocal'` | Wave 0 / exists (extend) |
| all | end to end through the real dispatcher: owner sets a rule from the page, service notice and bot notices are scheduled and deleted by the worker, a Staff Group is untouched | integration | `-run '^TestDeletionEndToEnd$'` | Wave 0 |

### Sampling Rate
- **Per task commit:** the quick command of the plan's own family, `-race -count=1`.
- **Per wave merge:** `go test -tags testtools -race -count=1 -run '^(TestService|TestDeletion|TestScheduleBot|TestBot|TestCleanService|TestSettings|TestLoadSettings|TestMemberReq|TestShutdown)' ./fuku/modules ./fuku/db/... ./fuku/i18n`.
- **Phase gate:** `make test` through CI's `go test -json` into `scripts/check_test_results`, `make lint`, `make check-translations`, `make check-docs`, `make test-postgres-integrity`.
- **Flake discipline:** run each concurrency test (`TestClaimBatchOneWinner`, `TestDeletionWorkerOneDeleteAcrossReplicas`, the inline-cap test) once with `-race -count=20`.

**`make test-postgres-integrity`:** add to the `-run` regex (`$$`-anchored list) `TestScheduleKeepsEarliest` (the `LEAST` upsert), `TestClaimBatchOneWinner` (conditional update winner), `TestScheduledDeletionMigrationDefaults` (CHECK, partial index, idempotence), and add `./fuku/db/deletion` to the package list. The reasoning, per AGENTS.md: the guarantee rests on a unique index, a CHECK and a conditional update.

### Wave 0 Gaps
- [ ] `fuku/db/deletion/testmain_test.go` (`internal/testdb.Run` with `&models.DeleteRule{}`, `&models.ScheduledDeletion{}`, `&models.StaffGroup{}`, `&models.StaffGroupLink{}`, `&models.GreetingSettings{}`, `&models.Chat{}` for the Staff and greetings refusals).
- [ ] AutoMigrate lists gain both models: `fuku/modules/test_harness_test.go:329-381` (before the page lands), `fuku/db/backup/testmain_test.go:41-80`.
- [ ] `fuku/modules/staff_action_fake_test.go`: a `sendDate func() int64` on `staffActionFake` (default 1), used by the deletion environment; `deleteMessages`/`deleteMessage` recorded (the base `staffBotClient` already records any method; the batch test reads `params["message_ids"].([]int64)`).
- [ ] `fuku/modules/deletion_env_test.go`: `deletionEnv` on `lockdownFake` (a supergroup, the bot as administrator with the right, miniredis, the real dispatcher with the new module and Greetings loaded), `sendService(from, field)` that builds updates with arbitrary service fields (the `lockdownEnv.serviceJoin` helper, lockdown_fake_test.go:476, is the template; `PinnedMessage` is the interface `gotgbot.MaybeInaccessibleMessage`, satisfied by a `gotgbot.Message` value), `withFastDeletionPacer`, `deletionCleanup` (rows, cache keys, `fuku:delete:*` keys), `restoreDeletionWorker`, `deletionInline.wait()`.
- [ ] `fuku/modules/settings_deletion_env_test.go` on `newSettingsEnv` per the UI-SPEC (hygiene: `uniqueModuleChatID()`, rows and keys deleted in `t.Cleanup`).
- [ ] Framework install: none.

## Security Domain

### Applicable ASVS Categories (level 1)

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication / V3 Session | no | none new: Telegram authenticates senders; callback data is never authority |
| V4 Access Control | yes | the press pipeline unchanged (live `getChatMember`, owner gate 4b); `settingsRightChangeInfo` for every write; the Staff Group gate before every write; the worker never acts on a Staff Group |
| V5 Input Validation | yes | `t` from the table, `v` from a per-row allow-list (canonical digits), kinds validated in Go before any write, `delay_secs` CHECK |
| V6 Cryptography | no | the claim token is a `crypto/rand` value, not a secret |
| V7 Logging | yes | chat and message counts and error classes only; never message text, tokens, or id lists |
| V11 Business Logic | yes | opt-in sites only (no catch-all hook), DEL-07 three layers, 24 h cap, 47 h drop, negative latch |
| V13 API and rate limiting | yes | fleet-wide pacer, `retryFree` ends a cycle, inline cap, 100-ID chunks |

### Known Threat Patterns

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| A forged callback names a row, value or section | Tampering | strict `Validate` in step 1, before any I/O |
| A scheduled deletion reaching a Staff Group, lockdown alert, captcha or log post | Tampering / Information disclosure | no hook in those code paths, `Schedule` and the watcher and the worker refuse a Staff Group, page hidden there (D-18) |
| Join flood spawning unbounded delete goroutines | Denial of service | inline cap with a durable fallback, paced, drained by the root context |
| A hammered group where the bot cannot delete | Denial of service | scoped latch (`gone`, `service`), pending rows dropped at latch time |
| Stale-claim double delete | Repudiation / DoS | token-guarded completion, 90 s cycle guard; a repeat delete is "not found" |
| Rules read failing open | Tampering | an unreadable rule deletes nothing |
| A deleted message the owner wanted | Tampering | one tap resets the rule; hints state timing; mute notice held 24 h for the member's button |

## Proposed plan split (small plans, tracer-first)

| Plan | Wave | Content (about 3 tasks each) | Depends on | Owns (files) |
|------|------|------------------------------|-----------|--------------|
| 16-01 data | 1 | (1) migration + models + migration tests (statements, PostgreSQL defaults); (2) `fuku/db/deletion` rules and queue repository with tests (earliest, claim winner, token guards, stale release, `SetCleanServiceTx`, `ApplyJoinLeftRule`); (3) `skipLocal` + cache test row, `TestDeletionTablesStayOutOfBackup`, AutoMigrate lists, Makefile integrity list | none | `migrations/20261110120000_*.sql`, `fuku/db/models/delete_rule.go`, `scheduled_deletion.go`, `fuku/db/deletion/`, `fuku/db/greetings/repository.go`, `fuku/db/cache/local.go`, `local_test.go`, `fuku/db/backup/*_test.go`, `fuku/db/migrations/*_test.go`, `fuku/modules/test_harness_test.go`, `Makefile` |
| 16-02 kinds | 1 | (1) `deletion_kinds.go` service table + bot kinds + presets + validators; (2) `TestServiceKindTable`, `TestServiceKindsCoverMessageFields`, `TestDeletionKindsNeverStaffLockdownCaptchaLog`; (3) the effective-rule pure function and its matrix test | none | `fuku/modules/deletion_kinds.go`, `deletion_kinds_test.go` |
| 16-03 tracer | 2 | (1) module, pacer, `deletionDeleteNow`, inline tracker, `deletionSchedule` (rule read, Staff gate, latch check) and the watcher at group -5; (2) `cleanService` rewiring (flag via `LoadSettingsContext`, effective rule, bot-sender case) and C-08 test edits; (3) `deletion_env_test.go`, `staffActionFake.sendDate`, `TestServiceWatcherDeletesNow`, sender and never-ends tests | 16-01, 16-02 | `fuku/modules/deletion_service.go`, `deletion_pacer.go`, `greetings.go` (cleanService only), `greetings_command_test.go`, `staff_action_fake_test.go`, env test files |
| 16-04 worker | 3 | (1) `deletion_worker.go` loop, cycle, claim, chunking, fallback, classifier, latch; (2) `Start`/`Stop`, `main.go` (`postInit`, shutdown handler after line 220), AGENTS shutdown numbers staged; (3) worker tests incl. `TestShutdownEndsDeletionWorker`, rate-limit, old rows, two replicas, Staff Group; integrity-list additions | 16-03 | `fuku/modules/deletion_worker.go`, `main.go`, `shutdown_drains_test.go`, worker tests, `Makefile` |
| 16-05 bot hooks A | 3 | (1) `scheduleBotMessage` / `scheduleBotMessageAt`; (2) welcome, goodbye, `sendRulesMessage` returns the message, `/rules` and click-to-read hooks; (3) `moderationCtx.replySuccess` at the manual sites, `warnThisUser` source parameter and its manual call; tests | 16-03 | `fuku/modules/deletion_bot.go`, `greetings.go` (two sites), `rules.go`, `moderation.go`, `bans.go`, `mute.go`, `warns.go`, tests |
| 16-06 bot hooks B | 4 | (1) `memberReqPostNotice` hook + mute notice at the cap + Off rule; (2) "I fixed it" reschedule from `now`; (3) antiflood, blacklist (incl. warn source auto) and aispam breaker hooks; tests | 16-05 (needs `warnThisUser`'s source and the helper) | `memberreq_notice.go`, `memberreq_fixed.go`, `antiflood.go`, `blacklists.go`, `aispam.go`, tests |
| 16-07 page | 3 | (1) `Hidden` field (settings_pages.go, settings_menu.go) + the page skeleton, tables, state function, Main state; (2) hub, lists, row screens, `dl`/`dr`, controls, apply functions, Staff gate; (3) right-line reader and `TestSettingsDeletion*` families, Main-shape test edits (S-06 list) using the marker locale | 16-01, 16-02, 16-03 (joined/left apply) | `settings_pages.go`, `settings_menu.go`, `settings_deletion.go`, `deletion_apply.go`, `settings_*_test.go` |
| 16-08 locales | 5 | (1) 57 keys in 8 files with the C-02 wording; (2) `settingsLocaleLimits` rows, `TestSettingsLabelLengths` lists, `memberReqQuotingKeys`; (3) page-fit and privacy sweeps in all languages | 16-07 | `locales/*.yml`, `fuku/i18n/*_test.go` |
| 16-09 closing | 6 | (1) `TestDeletionEndToEnd`, `TestSettingsStaffPrivacy` additions; (2) AGENTS.md (Handlers group -5, Data tables and keys, Callbacks page `d`, Permissions, shutdown arithmetic, Testing list), docs, `make generate-docs`; (3) `16-SPIKE.md` and all gates | all | `AGENTS.md`, `docs/`, test files, `16-SPIKE.md` |

Order: 16-01 and 16-02 in parallel; 16-03 is the tracer (a pinned notice, a seeded rule, the real dispatcher, one Telegram delete); then 16-04, 16-05 and 16-07 in parallel (the page can use the marker locale as Phase 14 and 15 did); 16-06 after 16-05; locales after the page; closing last. `use_worktrees` is false, so `locales/*.yml`, `AGENTS.md`, `main.go` and `greetings.go` are single-owner per wave (greetings.go is touched by 16-03 in Wave 2 and 16-05 in Wave 3, never in the same wave).

## Sources

### Primary (HIGH confidence)
- gotgbot v2.0.0-rc.36 module docs and code read in the module cache: `gen_methods.go` (`DeleteMessage` 1354-1371, `DeleteMessages` 1445-1473), `gen_types.go` (`Message` struct, `MergeChatMember`), `ext/handlers/message.go`, `ext/dispatcher.go:280-328`, `sender.go:137-161`, `ext/handlers/filters/message/message.go`.
- Repo files opened this session with line citations above: `fuku/modules/greetings.go`, `moderation.go`, `bans.go`, `mute.go`, `warns.go`, `blacklists.go`, `antiflood.go`, `aispam.go`, `rules.go`, `memberreq_notice.go`, `memberreq_enforce.go`, `memberreq_limits.go`, `memberreq_fixed.go`, `memberreq_lookup.go`, `lockdown_worker.go`, `staff_watchers.go`, `raid_flood.go`, `settings_pages.go`, `settings_menu.go`, `settings_mreq.go`, `shutdown_drains.go`, `main.go`; `fuku/db/greetings/repository.go`, `fuku/db/memberreq/settings.go`, `fuku/db/cache/local.go`, `fuku/db/staff/repository.go`, `fuku/db/backup/types.go`, `backup.go`, `memberreq_backup_test.go`, `fuku/db/migrations/runner.go`, `scripts/migrate_psql.sh`, `migrations/20261006120000*.sql`, `20261106120000*.sql`, `20261109120000*.sql`; `fuku/utils/ratelimit/telegram_pacer.go`, `fuku/utils/shutdown/graceful.go`, `fuku/utils/chat_status/access.go`, `owner.go`, `fuku/utils/cache/adminCache.go`, `fuku/utils/helpers/telegram_helpers.go`, `fuku/utils/media/sender.go`; tests `lockdown_fake_test.go`, `staff_action_fake_test.go`, `settings_main_test.go`, `settings_menu_test.go`, `greetings_command_test.go`, `fuku/i18n/settings_locale_test.go`; `AGENTS.md`, `16-CONTEXT.md`, `16-UI-SPEC.md`, `16-UI-CHECK.md`, `15-RESEARCH.md`.
- Executed here: the migration SQL twice, the `LEAST` upsert, the unqualified-column failure, the CHECK, the claim `UPDATE ... RETURNING` and the due-query plan on PostgreSQL 16 (throwaway instance, removed); the `MIN` upsert, partial-index AutoMigrate, check tag and subselect claim on the repo's SQLite driver.

### Secondary (MEDIUM confidence)
- A web search for the deleteMessage error strings returned only the Bot API limitations already read in the pinned docs; no source for exact descriptions (hence A1).

### Tertiary (LOW confidence)
- None used as authority.

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH, nothing new.
- Architecture and seams: HIGH, every site and group read this session; the SQL executed.
- Telegram edge behaviour (batch with undeletable IDs, exact strings, basic-group flags): MEDIUM, covered by safe code and the owner's spike.
- Pitfalls: HIGH for the code-derived ones, MEDIUM for 429-storm cost figures.

**Research date:** 2026-11-10
**Valid until:** 2026-12-10 (repo moves with each phase; re-check handler groups, `main.go` registration order and the Main grid tests if another phase lands first)

## RESEARCH COMPLETE

**Phase:** 16 - Scheduled Deletion of Service and Bot Messages
**Confidence:** HIGH (code facts and SQL executed), MEDIUM (Telegram batch-delete edge cases, exact error strings)

### Key Findings
- Everything reuses existing patterns (lockdown worker frame, member requirements pacer and cached settings, registry-driven page). The migration, the `LEAST`/`MIN` upsert and the claim were run on PostgreSQL 16 and SQLite; the PostgreSQL upsert needs the table-qualified column.
- Handler group `-5` is free and right for the service watcher; `cleanService` (group 0) keeps joined/left and must read the flag through `LoadSettingsContext`; bot-sent join/leave messages never reach it (no `SetAllowBot`), so the watcher covers them.
- Shutdown: insert `StopDeletionWorker` after `main.go:220`, give it an internal 3 s wait; worst case 68 s of allowances against the enforced 60 s budget (exit 1, rows resume).
- The manual-reply seam is a `moderationCtx.replySuccess` method at 11 named success sites; `warnThisUser` takes a source; `sendRulesMessage` must return the message; `memberReqPostNotice` is the one site for every requirement notice.
- Tests that break or need care: three hard-coded Main shapes, two `cleanService` tests that assert a synchronous delete, and the shared fake's `date:1`.

### File Created
`/home/user/Fuku_Robot/.planning/phases/16-scheduled-deletion-of-service-and-bot-messages/16-RESEARCH.md`

### Open Questions for the owner (product-level only, non-blocking)
1. After `/cleanservice off` then `on`, should an old "After 1 h" delay come back (D-02 as written) or reset to Delete now?
2. Should the aispam breaker notice count as an automatic punishment message (it is the only aispam group message)?

### Proposed plan decomposition
16-01 data, 16-02 kinds (wave 1, parallel); 16-03 tracer (wave 2: pinned notice deleted Now through the real watcher and a seeded rule); 16-04 worker, 16-05 bot hooks A, 16-07 page (wave 3, parallel); 16-06 bot hooks B (wave 4); 16-08 locales (wave 5); 16-09 closing end-to-end, AGENTS.md, docs, spike file (wave 6).

### Where CONTEXT.md or the UI-SPEC must change
- **D-05/D-07:** add a `sent_at` column; the 47 h rule cannot be computed without the message's date (C-01).
- **UI-SPEC `sf`:** remove `ForumTopicCreated` (Telegram: forum topic creation can't be deleted) and the word "created" from `settings_del_desc_forum` (C-02).
- **UI-SPEC `sg`:** `Giveaway*` would delete the giveaway post; use `GiveawayCreated`, `GiveawayWinners`, `GiveawayCompleted`, `Gift`, `UniqueGift`, `GiftUpgradeSent` (C-03).
- **D-07 latch:** scope it (`gone` for all kinds, `service` only after a live right check); a missing right must never stop `bot:*` deletion (C-04).
- **D-10:** the 65 s figure holds only because of the global 60 s cap; state 68 s / 60 s with a 3 s internal wait (C-05).
- **D-19:** the watcher also handles joined/left for bot senders (C-06).
- **D-14:** Off means no row for the mute notice, and the "I fixed it" reschedule uses `now` as its base (C-07).
- **Tests:** wait on the inline tracker in the two `cleanService` tests, and add a `sendDate` seam to the shared fake (C-08, C-09).
