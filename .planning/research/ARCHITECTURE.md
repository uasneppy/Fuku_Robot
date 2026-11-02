# Architecture Research: v1.1 PM Settings Hub & Member Screening

**Domain:** Telegram group-management bot (Go, gotgbot v2, GORM/PostgreSQL, Redis, several replicas)
**Researched:** 2026-11-02
**Confidence:** HIGH for integration points (read from the code, `AGENTS.md` and gotgbot's source); MEDIUM for the PM group-list source and the name-AI pool sharing (design choices with a stated fallback); LOW only where marked "verify".

Scope: only what the six v1.1 feature groups add to or change in the existing architecture. `AGENTS.md` stays the rulebook; this file says where the new parts plug in.

---

## 1. Key findings (read before the details)

1. **The settings menu is stateless and keyed on `c` (private) or `u` (group).** The PM hub is one extra namespace (`settingsh|v1`) for the group list. A pick is the existing `settings|v1` `a=bk&c=<chat id>` button, so the pick reuses the whole `settingsPress.verify` pipeline with no new code.
2. **The owner gate is one function used by four doors.** Extract steps 4 to 4b of `verify` into `settingsAuthorize(b, userID, groupID)` (live `getChatMember`, then the stored `allow_admins` flag) and call it from `verify`, `settingsStartDeepLink`, the in-group `/settings` check and the PM input consumer. Four copies of the rule would drift.
3. **Edited messages reach no handler today.** `edited_message` is in `AllowedUpdates`, but every `handlers.NewMessage` and `NewCommand` in the repo leaves `AllowEdited` false, and `Message.CheckUpdate` only looks at `ctx.EditedMessage` when it is true. A small custom `handlers.Handler` that matches `ctx.EditedMessage != nil` is therefore collision-free.
4. **A handled command ends the update at group 0, so "delete commands" cannot be a watcher at group 1 or later, and cannot delete before the handler replies** (`msg.Reply` to a deleted message fails). The right hook is the dispatcher's `ext.Processor` seam, which `main.go` already wraps with `tracing.TracingProcessor`. A wrapper runs after all groups finished and deletes the command then.
5. **New handler groups: none below 12 are needed beyond `-1` (shared with the users tracker, registered after it) and `1` (edited messages).** Everything else is a function call inside an existing flow (`ProcessSingleJoin`, send sites, the Processor).
6. **The scheduled-deletion queue is the lockdown worker pattern**: a table, a batch claim by conditional update, a 1 s tick plus wake channel on every replica, a stale-claim release, a drain registered after DB-close. Use `deleteMessages` (up to 100 ids) per chat per call.
7. **The PM "awaiting input" state is a Redis hash with a TTL and a claim nonce**, not a DB row and not a draft. Every input is applied immediately through the same apply function the command uses. Redis down means the editor entry buttons say so; every other page keeps working.
8. **Requirement punishments must reuse the existing code, not copy it.** `warnThisUser` needs a ctx-free core; mute must use a marker `until_date` (the `ban_until` trick of the lockdown) so "I fixed it" can tell its own mute from an admin's; every unmute goes through `resolveUnmutePermissions`.

---

## 2. System overview (what changes)

```
Telegram updates (polling or webhook)
        |
        v
 tracing.TracingProcessor  --wrapped by-->  [NEW] postUpdateProcessor   (after all groups: delete the command message)
        |
        v
 Dispatcher handler groups (ascending)
  -10 captcha sweeper          -7 lockdown guard         -6 fed-ban       -4 raid flood counter
  -3 staff watchers            -2 admin-cache
  -1 users tracker ──► [NEW] memberReqWatcher ──► [NEW] messageHygieneWatcher ──► [NEW] settingsInputWatcher (PM only)
   0 commands / greetings (ProcessSingleJoin ──► [NEW] Screen step before captcha/welcome)
 [NEW] 1 editedMessageHandler (custom Handler, edited updates only)
   2 raid AI watcher   3 aispam   4 antiflood   5/6 locks   7 blacklists   8 reports/reactions
   9 filters   10 pins   11 log-channel   12 late raid flood counter (stays last)

 Callbacks:  settings|v1 (existing, + new pages/rights)   [NEW] settingsh|v1 (PM hub list)   [NEW] memreq|v1 ("I fixed it")

 Background (every replica, DB-driven, claim by conditional update):
   StartLockdownWorker (existing)      [NEW] StartDeletionWorker (scheduled_deletions)

 Redis (outside fuku:cache:): [NEW] fuku:settings:await:<user>   fuku:memberreq:*   fuku:delete:pace:*
 PostgreSQL: [NEW] chat_settings_access, chat_managers, chat_member_rules, member_requirement_holds,
             chat_delete_rules, scheduled_deletions, greeted_members; columns on greetings and rules
```

### Component boundaries

| Component | Responsibility | Talks to | New / Modified |
|-----------|---------------|----------|----------------|
| `settingsAuthorize` (`settings_menu.go`) | Live member check plus owner gate; one answer type used by every door | `lockdownLiveMember`, `access` repo | NEW (extracted from `verify`) |
| `fuku/db/access` | `chat_settings_access` (allow_admins), `chat_managers` (PM list projection) | PostgreSQL, `fuku/db/cache` | NEW |
| `settings_hub.go` | PM `/settings` list, paging, Refresh, `settingsh\|v1` callbacks | `access`, `chats` | NEW |
| `settings_access.go` | Access page: "let admins in" toggle (creator only) | `access` | NEW page |
| `settings_await.go` | Redis await store: Begin, Claim, Release, Clear, Cancel | Redis | NEW |
| `settings_input.go` | Group `-1` PM consumer, `/cancel`, per-kind input parsers and appliers | await store, `settingsAuthorize`, apply functions | NEW |
| `settings_editor.go` + `settings_welcome.go` | Shared Rules/Welcome/Goodbye editor pages | greetings and rules apply functions | NEW pages |
| `fuku/db/memberreq`, `memberreq*.go` | Rules, holds, join screen, message watcher, punisher, "I fixed it" | warns core, bans, `resolveUnmutePermissions`, claude client | NEW |
| `fuku/db/deletion`, `deletion*.go` | Rules, queue, worker, hygiene watcher, edited handler, Processor hook | Telegram `deleteMessages`, pacer | NEW |
| `membership.go` | Join pipeline | gets `Screen` dependency | MODIFIED |
| `greetings.go`, `rules.go` | Apply functions, greet-once, media/buttons/preview, schedule bot messages | repos, `media.Send*` | MODIFIED |
| `warns.go` | Ctx-free `applyWarn` core | `warns` repo | MODIFIED |
| `fuku/utils/claude` | `ClassifyName(ctx, name)` | Anthropic | MODIFIED (stdlib only rule holds) |
| `main.go` | Processor wrapper, `StartDeletionWorker`, drain registration | | MODIFIED |

---

## 3. PM hub and owner gate

### 3.1 Where each piece plugs in (exact files)

| File / symbol | Change |
|---------------|--------|
| `settings.go` `settingsDesc.RequiredChecks` | Replace `requireLockdownGroup(), requireLockdownAuthority(false)` with one `requireSettingsAccess()`. In a private chat it returns true (the hub decides per group). In a group it runs the live authority check and then the gate; a gated admin gets the reply `settings_owner_only`. |
| `lockdown.go` `requireLockdownAuthority` | Split its body into `lockdownAuthorize(c) (member, ok)` and keep the old function as a wrapper. The gate needs the member's `Status`, and a second `getChatMember` would double the live calls. The anonymous-admin proof prompt stays inside `lockdownAuthorize`, so `anonPipelineHandler(settingsDesc, ...)` re-runs the same check after the proof with the tapper as sender. |
| `settings.go` `settings()` | Branch on `chat.Type == "private"` to `settingsHub`. The group branch is unchanged. |
| `settings.go` `settingsStartDeepLink` | After the existing `member.Status` check, call the gate. An admin the gate refuses gets `settings_owner_only` (safe: that user just proved they are an admin of that group, so the reply tells a stranger nothing). Bot-missing and plain-member answers stay the identical masked `settings_start_not_admin`. |
| `settings_menu.go` `verify` step 4 | Replace the inline `getChatMember` + status check with `settingsAuthorize`. New step 4b: administrator and not `allow_admins` gives `settings_owner_only`. In a private menu this also edits the message to the no-keyboard note, so a dead menu does not stay on screen. A failed gate read fails closed with `settings_check_failed`. |
| `settings_menu.go` `settingsRight` | Add `settingsRightOwner` (status creator, alert `settings_owner_only`), `settingsRightChangeInfo` (creator or admin with `can_change_info`, mirrors the greetings/rules commands' `CanUserChangeInfo`), and later `settingsRightBotDelete` (bot `can_delete_messages`, notice). `settingsRightRefusal` reads them from `p.member`, so no extra Telegram call. |
| `settings_pages.go` `renderSettingsMain` | Draw two buttons per row. About 15 pages will not fit one per row, and the long "title · state" labels need shortening. One-time edit; later pages still never touch this file. |
| `fuku/db/cache/local.go` `skipLocal` | Add the `settings_access:` prefix. A creator turning admins off must reach every replica at once, as `raid_detection:` does. |

### 3.2 The gate rule

```
status creator                      -> allowed (always)
status administrator + allow_admins -> allowed
status administrator, no allow_admins -> refused: settings_owner_only (hidden from the PM list)
anything else                       -> settings_not_admin (unchanged)
```

- Storage: `chat_settings_access(chat_id BIGINT PRIMARY KEY, allow_admins BOOLEAN NOT NULL DEFAULT FALSE, updated_by BIGINT, updated_at)`. A missing row is "owner only". Read through `cache.GetFromCacheOrLoad` under `cache.CacheKey("settings_access", chat)`; the single write path `applyAllowAdmins` does a column-targeted upsert (a Go `false` must be written) and `cache.DeleteCache`. No `default:` tag on the Go model's bool, for the same reason as `ChatRaidDetection`. Never part of backup/export/import/reset. Not re-keyed on chat migration (falls back to owner-only, which is the safe side); say so in `AGENTS.md`.
- The toggle lives on a new Access page (key `a`, Order 5, so it sits first) with `settingsRightOwner`. In-group setting commands (`/captchamode` and the rest) are untouched, as the milestone requires.
- The gate never replaces the per-control right. Order stays: decode, service identity, opener, live member, **gate**, control right.
- Staff Group page, Lockdown page and the rest all ride on `verify`, so they are gated with no per-page code.

### 3.3 PM group list (source decision)

The Bot API cannot list a user's groups. Recommendation: **a derived projection table `chat_managers(chat_id, user_id, role, updated_at, PK(chat_id, user_id))`**, role `creator` or `admin`.

- Written from the one place that learns admin lists from Telegram: a package-level hook in `fuku/utils/cache/adminCache.go` (`AdminCacheLoadedHook func(chatID int64, ac AdminCache)`), called after a successful `loadAdminCacheFromTelegram` store, installed from a module `init`. The hook replaces that chat's rows in one transaction (delete rows for the chat, insert the current admins), from a recovered goroutine that joins the users-async-writes WaitGroup. Because every `IsUserAdmin` miss, `/admincache` and the `adminCacheAutoUpdate` reload go through `LoadAdminCache`, the table warms itself. Keeping the DB write out of `utils/cache` and behind a hook respects the import hierarchy.
- The list query is one SQL statement: `chat_managers` joined to `chats` for the stored title, joined to `chat_settings_access`, keeping `role = 'creator' OR allow_admins`, ordered by title, paged at 8. **No Telegram call per row.** The list is only a hint; picking a group runs the live `settingsAuthorize`, and a stale row is refused there.
- An admin of a group where the gate is closed never sees that group (hidden, not read-only). A user with no rows gets `settings_hub_empty` explaining that the bot learns admins as it sees them and that `/admincache` in the group fills the list.
- Fallback if the projection is rejected: scan active `chats` rows and read each Redis admin cache. This is O(all chats), misses cold caches and the fork may carry thousands of old rows, so it is the worse choice. A one-off warm-up for a user with zero rows is a phase decision (`chats.users @> [uid]` candidates, `getChatMember` on at most 20).
- Privacy (D-21/D-32): the hub shows only the user's own manageable groups' titles and never the Staff Group.

### 3.4 Hub callbacks

- Namespace `settingsh|v1`, registered with `callbackquery.Prefix("settingsh|")` (the `|` keeps it distinct from `settings|`). Fields: `a=ls`, `o=<offset>` (offset cursor like the staff history, clamped, refused outside the range). The press must be on a private-chat message and `query.From.Id` must equal the chat id; a service identity is refused first. It draws the list from the database only, so it works with Redis down.
- **Pick** buttons are `settings|v1|a=bk&c=<chat id>`. That lands on the existing pipeline: structure check, service identity, live member, gate, `getChat`, and `dispatch` redraws Main. Zero new pick code.
- The private Main page gets a leading "All groups" row (`settingsh` `a=ls`) above the nav row. Back from a page still goes to Main.
- `answer` exactly once per press, `defer RecoverFromPanic`, edit in place, "message is not modified" is success: the same discipline as `settingsCallback`.

---

## 4. Stateful PM input flow

### 4.1 State: Redis hash with TTL, claim nonce

Recommendation: **Redis, not a DB row.** It is ephemeral by definition, needs a TTL and an atomic claim (the staff card and captcha handle patterns already do both with Lua), and it avoids a table, a migration and a cleanup sweep. A DB row would work without Redis, but the cost (a migration, a PK read on every PM message, expiry housekeeping) buys nothing, because no draft is kept: each accepted input is written to its real table at once.

| Key | Type | TTL | Content |
|-----|------|-----|---------|
| `fuku:settings:await:<user id>` | hash | 10 min (`settingsAwaitTTL`) | `g` group chat id, `k` input kind (`wt` welcome text, `wm` welcome media, `wb` welcome buttons, `gt`/`gm`/`gb` goodbye, `rt`/`rm`/`rb` rules, `bl`/`fl`/`nt` list adds, ...), `p` page key, `m` the PM menu message id, `n` nonce, `s` step for two-step kinds, `x` scratch (a filter's keyword, kept in Redis, never in callback data) |

- **One state per user.** `Begin` overwrites. The old prompt message is not edited (no timer, no Redis expiry hook); its Cancel finds a different nonce and answers "no longer valid" and redraws the menu.
- **Begin** is a button press (`a=in`, fields `p`, `k`) that already passed `verify`: it writes the hash and edits the menu message into a prompt page (the prompt text, a Cancel button `a=ic&n=<nonce>` and no Back). The prompt page must declare its input kind so tests can enumerate them.
- **Cancel**: (a) the inline button, a Lua compare-and-delete on the nonce, then redraw the page; (b) `/cancel` in private, registered with `helpers.WrapCommand` (no existing `cancel` command), clearing the state and redrawing; (c) any other `settings|` press by that user for that menu (`bk`, `cl`, a page `op`) clears the state first, so a Close or Back never leaves an invisible pending input; (d) TTL expiry, lazy: input after expiry just falls through to the normal handlers.
- **Without Redis**: the `in` action answers `settings_input_unavailable` (alert, no edit). Toggles, steppers and the rest of every page are unaffected (the stateless-menu property is kept for everything that does not type).

### 4.2 Consumer handler

- `handlers.NewMessage(privateInputFilter, settingsInputWatcher)` in **group `-1`**, registered by a module whose priority is above the Users module's 100 so it sits after the users tracker. The tracker returns `ext.ContinueGroups`, so gotgbot keeps iterating the group (`iterateOverHandlerGroups` continues on `ContinueGroups`). Group `-1` is below `0`, so it runs before the `#note` watcher (group 0), `filtersWatcher` (9) and every command.
- Filter (cheap, no I/O): private chat, a real user sender, not a bot, **and** not slash-prefixed text (`raidFloodIsSlashText` already defines that; reuse it). Slash commands, `/start` included, pass through untouched, so `/start settings_<id>`, `/connect`, `/settings` and `/cancel` still work while a prompt is pending.
- Body: one Redis `GET` (a hash existence check). No state returns `ext.ContinueGroups` (one cheap read per PM message, PMs are rare). A state returns `ext.EndGroups` once consumed, so nothing else in that update runs.
- **Claim like `ConsumeIfUser`**: a Lua script sets a `claim` nonce field if absent and returns the state; a second message of an album finds a live claim and is ignored. After a good apply a second script deletes only while the claim matches. A validation failure (bad URL, empty text) **releases** the claim and keeps the state: the bot replies with the reason and the user can resend, with the original TTL. An unexpected error releases the claim and keeps the state. This mirrors the captcha handle (an error always means nothing was used up).
- **Re-authorize before applying**: `settingsAuthorize(b, user.Id, g)` runs again, because the user may have been demoted, or the owner may have closed the gate, since the prompt was drawn. A refusal clears the state and replies with the refusal text. Authority is therefore never a property of the stored state.
- Apply through a registry entry, the same registration style as pages: `settingsPage.Inputs map[string]settingsInputKind{Right, Parse, Apply}`, collected by `registerSettingsPage` (panics at init on a duplicate kind). `Apply` calls the setting's own apply function (`applyWelcomeText`, `applyRulesMedia`, `applyFilterAdd`, ...) that holds validation, refusal, write and `cache.DeleteCache`, so the typed path and the menu cannot drift (one setting, one code path).
- After a good apply: delete the user's input message (the Bot API lets a bot delete incoming messages in private chats; a failure is ignored) and edit the stored menu message `m` back to the page with a success notice. If the edit fails because the user deleted the menu, send the page as a new message.
- Input parsers need the message itself: `content.ExtractWelcome` does `strings.Fields(msg.Text)[1:]`, which assumes a leading `/command` and would panic on a plain PM message. Add `content.ExtractGreetingInput(msg)`-style helpers (text with entities to HTML, one media file id and type, button lines) rather than calling `ExtractWelcome`. **Verify** during the phase how `/setwelcome` turns entities into HTML so the PM path stores identical output.

### 4.3 Interaction with other PM handlers

| Handler | Group | Interaction |
|---------|-------|-------------|
| `start` + deep-link router | 0, command | Commands are never consumed. `settings_<id>` re-authorizes and renders Main; a pending prompt for another group stays until its TTL or `Begin` overwrites it. |
| Connections (`/connect`, `IsUserConnected`, `/setwelcome` via connection) | 0, command | Unaffected. The menu is keyed on `c`, not on the connection. Do not call `connections.ConnectId` from the hub. |
| Notes `#hashtag` watcher | 0 | Runs after `-1`, so a note named `#x` typed as input is consumed as input. Correct. |
| Filters watcher | 9 | Same. |
| Users tracker | -1 | Registered first, returns `ContinueGroups`, so the PM user is tracked before the consumer. |
| Anonymous-admin proof | callbacks | None; PMs have no anonymous senders. |

### 4.4 Group menus cannot type

A menu opened in the group has no PM to type in. Editor pages detect `!p.private` and show the editing buttons as a single URL button, `settingsPrivateURL`, "Edit in private", plus the toggles that need no typing. This keeps D-21/D-32 and avoids prompting in the group.

---

## 5. Message editor (Rules, Welcome, Goodbye)

**Pages.** `w`, `g`, `r` registered from `settings_welcome.go` and `settings_rules.go` through a shared `settings_editor.go` that renders: Text, Media, Buttons, Preview (link preview) toggle, "Media under text" toggle, and for Welcome On/Off, "Delete last welcome", "Greet once" and a Test button. Goodbye has On/Off, "Delete last goodbye". Rules has Text, Media, Buttons, the existing button label and private flag.

**Storage (one migration, `202611..._extend_greetings_rules_editor.sql`):**

- `greetings`: `welcome_preview`, `welcome_media_below`, `welcome_once`, `goodbye_preview`, `goodbye_media_below`, all `BOOLEAN NOT NULL DEFAULT FALSE`. Pick semantics so zero equals today's behavior (preview off, caption above, greet every join). That keeps old backup imports and the GORM zero-value trap harmless. Telegram's `show_caption_above_media` is the "text above media" switch (verify the exact option name in this gotgbot version).
- `rules`: `media_file_id TEXT`, `media_type INT`, `btns JSONB`, `preview`, `media_below`. Reuse `models.ButtonArray` and `db.TEXT`-style type constants.
- `greeted_members(chat_id BIGINT, user_id BIGINT, created_at, PK(chat_id, user_id))`. Greet-once is `INSERT ... ON CONFLICT DO NOTHING` and `RowsAffected == 1` means send. That is exactly-once across replicas by the same trick as `BeginAlert`. Never part of backup. Only consulted when `welcome_once` is on.
- Backup: the existing backup module reads `greetings` and `rules` through their models, so the new columns travel in JSON and an old export decodes to zero (legacy behavior). The new tables are excluded, as the staff and lockdown tables are.

**Code.**

- Column-targeted repository writes (`SetWelcomeText`, `SetWelcomeMedia`, `SetWelcomeButtons`, `SetWelcomePreview`...). Today's `greetings.SetWelcomeText(chat, text, file, buttons, type)` overwrites text, file and buttons together, which is wrong for one-field edits. Use explicit-map updates (`UpdateRecordWithZeroValues`) so a `false` is written.
- Re-use the toggle spec table in `greetings.go` (`setPref`, `showEnabled` ...), the pattern the greeting toggle commands already use, for the new bools. Add `applyWelcomeX` functions beside the commands; `/setwelcome` stays and also calls them.
- Sending: one `renderGreeting(chat, user, settings)` used by join, goodbye, Test (to the PM, with the presser as the member) and rules. Extend `media.Send`'s `Options` with `WebPreview` and caption-above. Rules sending (`sendRules`, the `rules_<chat>` deep link, the private-rules button) moves to the same renderer so media and buttons appear everywhere.
- Right: `settingsRightChangeInfo` (mirrors the commands).
- Bot messages: `SendWelcomeMessage`, goodbye and rules sends call `scheduleBotMessage` (section 7) after a successful send.

---

## 6. Member requirements and name blocks

### 6.1 Where the checks run

| Trigger | Hook | Notes |
|---------|------|-------|
| Join | New `Screen func(user gotgbot.User) (screened bool, err error)` in `MembershipDeps`, called in `ProcessSingleJoin` **after `Claim`** (so once per join, both delivery paths deduped) and **before the captcha decision**. Wire it in `membershipDepsForJoin`. | A screened join (punished) ends the pipeline: no captcha, no welcome (`JoinScreened` outcome). A pass continues unchanged. `DecideJoin` and its tests stay pure. |
| Every message | `memberReqWatcher` in **group `-1`** after the users tracker | Before commands, so a violator's commands are not served either; after `-4` raid flood counter and `-3`, so raid detection still counts everything; after `-10` (pending-captcha messages are held and ended) and `-7`/`-6` (lockdown/fed-ban already ended the update). Returns `ContinueGroups` except when it punished and deleted, then `EndGroups` (do not run aispam, filters or replies on a deleted message). |
| Name change | Same watcher: hash of the full name compared with a per-user Redis marker | Cheap local rules re-run on every message anyway; only the AI job is gated by the name hash. |
| "I fixed it" | `memreq\|v1` callback | Fresh re-check, see 6.4. |

Cheapest-first order inside the watcher, mirroring `raidFloodCountUpdate`: chat type, sender is a real user and not the bot and has no `sender_chat`, rule set loaded (cached, one read; no rule on means return), approved user (`IsApprovedForUpdate`), admin by the cached list, pending captcha, hold exists, per-user pass cache `fuku:memberreq:ok:<chat>:<user>:<rules version>` (TTL 5 min), then the rules. Free rules (last name, username, name scripts) read `msg.From` only. The photo rule (`getUserProfilePhotos`) and the channel rule (`getChatMember` of the linked channel) cost a Telegram call, so they run only on a pass-cache miss, and their results are cached.

### 6.2 Storage (migration `202611..._add_member_requirements.sql`)

- `chat_member_rules(chat_id, rule VARCHAR(16), punishment VARCHAR(8), on_join BOOLEAN, delete_msgs BOOLEAN, notify BOOLEAN, updated_at, PK(chat_id, rule))`, `CHECK punishment IN ('off','warn','kick','ban','mute')`. **One row per rule, absent row means off.** A new rule (`last_name`, `username`, `photo`, `channel`, `name_ai`, `name_ru`, `name_ar`, `name_zh`) needs no migration, and every toggle is a column-targeted upsert that cannot clobber another rule, so the `UpdateRecord`-skips-zero trap never applies. A fixed 8x4 column layout was rejected for the clobber and zero-value risks.
- `member_requirement_holds(chat_id, user_id, rule, mute_until BIGINT, notice_msg_id BIGINT, created_at, PK(chat_id, user_id))`. A hold is a record that **this system** muted this user. Insert-first with `ON CONFLICT DO NOTHING` decides who punishes: only the caller with `RowsAffected == 1` restricts and posts the notice, so ten messages from one violator, on three replicas, cause one mute and one notice.
- Rules are read through `cache.GetFromCacheOrLoad` (`cache.CacheKey("memberreq_rules", chat)`) with `cache.DeleteCache` on every write; the staleness of the 10 s local layer is acceptable (put it in `skipLocal` only if the owner wants instant off switches). Holds and the pass cache are read fresh. Neither table is part of backup/export/import/reset.
- Redis (outside `fuku:cache:`): `fuku:memberreq:ok:*` (pass cache), `fuku:memberreq:warn:<chat>:<user>:<rule>` (`SET NX EX 600`, the warn cooldown), `fuku:memberreq:namev:<sha8 of lowercased name>` (the AI verdict, global across chats), AI budget and breaker keys modelled on the raid image ones.

### 6.3 Punishments: reuse, do not copy

One executor, `applyMemberReqPunishment(ctx, b, chat, user, verdict, src, msg)`, where `src` is join, message or AI. It never runs for admins, approved users, the bot, or a pending-captcha member, and it needs the bot to be an administrator (otherwise log once, show the page warning, change nothing).

| Punishment | Reuse | Detail |
|------------|-------|--------|
| warn | Extract `applyWarn(b, chat, userID, reason) (result, err)` from `warnThisUser` (the `warns.WarnUser` call, the limit and mode switch, the reset), keeping `warnPunishLock`. `warnThisUser` becomes a thin reply wrapper. | Rate-limited by the `:warn:` latch so a nameless user does not collect a warn per message. At the limit the warn mode (kick/mute/ban) applies through the existing code. |
| kick | `kickMember(b, chatID, userID)` | The rejoiner is re-screened. `greetings.leftMember` would post a goodbye for the kick: set a short Redis flag `fuku:memberreq:kicked:<chat>:<user>` that `leftMember` honors. |
| ban | `chat.BanMember` | A ban waits for an admin; there is no button. |
| mute | Insert the hold first, then `chat.RestrictMember(b, userID, MutedPermissions, &gotgbot.RestrictChatMemberOpts{UntilDate: markerUntil})` | The marker is `now + 330 days` (below Telegram's 366-day "forever"), stored as `mute_until`. Same device as `lockdown`'s `ban_until`: the unmute side only acts when the live status is restricted **and** its `until_date` equals the hold's `mute_until` within 2 s, so an admin's own mute, or a lockdown's, is never lifted. A failed restrict deletes the hold again. |
| unmute | Always `resolveUnmutePermissions(chatInfo)` and refuse (alert, hold kept) when it returns an error | During a lockdown it returns the stored pre-lockdown permissions; raw `chatInfo.Permissions` would leave the member muted after the lift. The `/unmute` and captcha paths already do this. |

- "Delete their messages" deletes the offending message immediately and inline (they are in the update). The notice is posted with `ParseMode HTML`, the member mention escaped, with the "I fixed it" button only for mute (`memreq|v1|a=fx&c=<chat>&u=<user>`, about 42 bytes, via `callbackcodec`), then handed to `scheduleBotMessage(kindPunishAuto)` (section 7) so the notice can self-delete.
- Log: post to the group's log channel under the existing `automated` category through `actionlog.Destination`, naming the rule. No new category.
- `notify` off posts nothing but still punishes.

### 6.4 "I fixed it" button

Callback handler `memreq|v1`, `a=fx`. Order, each step refusing before the next: decode with `callbackcodec` and structure check; a service identity is refused; **`query.From.Id` must equal `u`** (only the target can tap; anyone else gets a presser-only alert); read the hold by `(c, u)` fresh, none means "already fixed or lifted"; the live `getChatMember` of the presser must be restricted with the hold's marker `until_date` (anything else: delete the hold and say nothing was changed); a **fresh re-check of the rules** (bypassing the pass cache; an unreadable check, such as a channel the bot cannot see, answers "try again" and keeps the mute); then `resolveUnmutePermissions` and `restrictChatMember`; the hold is removed by conditional delete only after Telegram confirmed. After a good unmute: delete the notice, and if the group uses a captcha and the member is not approved, hand the member to the normal join challenge instead of leaving them let in (recommendation; confirm with the owner, because the alternative lets a fixed member skip the captcha). Kick lets the member rejoin and be re-screened; ban gets no button (matches the PROJECT.md decision).

### 6.5 AI spam names (Claude Haiku)

- `fuku/utils/claude` gets `ClassifyName(ctx, name) (*Verdict, error)`: same forced single-tool request shape, no sampling parameters, the key only in `x-api-key`, name only in the prompt (never chat, user id or text). This is a stdlib-only change and keeps the leaf test.
- Run it **off the update path**. A join or a name change enqueues a job; the verdict later calls `applyMemberReqPunishment` with `src=AI`.
- Pool: recommend adding a third job kind (`name`) to the **existing raid AI pool** (`raid_ai.go`: 128 queue, 4 workers, `raidAIContext`, `StopRaidAI` drain, `ENABLE_AISPAM` kill switch, `ANTHROPIC_API_KEY` gate), with **its own breaker, hourly budget and verdict cache keys** (`fuku:memberreq:*`). That avoids a third drain registration and a second worker set, and the name call is tiny. Trade-off: a 100-image raid shares the 4 workers with name checks. If that is judged too coupled, give names their own small pool and drain in the same style. Flag for phase research.
- The check is off when the key is missing, `ENABLE_AISPAM=false`, the breaker is paused or the budget is gone: no verdict, never a block. Cached verdicts cost nothing. Name scripts (Russian letters ы, ъ, э, ё; Arabic; Han) are pure local rules and need no AI.

### 6.6 Interactions to keep correct

- Captcha: `claimCaptchaPass` unmutes through `resolveUnmutePermissions`; a member who then fails a requirement is caught by the next message check. Screening before the challenge means a violator never gets a challenge they cannot finish.
- Lockdown guard `-7` ends joins during a lockdown before screening; nothing to do.
- A staff `/unmute` or `/unban` on a held member does not clear the hold: the "I fixed it" step's live marker check and a hold-prune on `mute_until` passing keep it from lying. Optionally delete the hold when the member leaves (`leftMember`).
- Stored `rules` for the Staff Group chat: requirements do not apply to a Staff Group's own members unless an admin turns them on there; no special code.

---

## 7. Message deletion

### 7.1 Storage (migration `202611..._add_message_deletion.sql`)

- `chat_delete_rules(chat_id, kind VARCHAR(40), delay_secs INTEGER NOT NULL CHECK (delay_secs >= 0), updated_at, PK(chat_id, kind))`. Row absent = off. Kinds: `service:<type>` (new_chat_members, left_chat_member, new_chat_title, new_chat_photo, delete_chat_photo, pinned_message, video_chat_*, message_auto_delete_timer_changed, forum_topic_*, and so on), `bot:welcome`, `bot:goodbye`, `bot:rules`, `bot:punish_auto`, `bot:punish_manual`, `member:commands`, `member:edited`, `member:selfdestruct`. Kinds are not CHECKed (so a new service type needs no migration, the `trigger_kind` precedent); the Go side validates against a list. Delay unit is **seconds** everywhere; "N minutes" in the UI is `N*60`. Cap the stored delay at 24 h (`CHECK delay_secs <= 86400`), well below Telegram's 48 h delete limit.
- Read as one struct per chat through `cache.GetFromCacheOrLoad` (`cache.CacheKey("delete_rules", chat)`), invalidated by the single write path.
- Migration data copy: `INSERT INTO chat_delete_rules SELECT chat_id, 'service:new_chat_members', 0 ... FROM greetings WHERE clean_service_settings` and the same for `left_chat_member`. The legacy `/cleanservice` apply function writes both the new rows and the old `clean_service_settings` column in one transaction (the column stays for backup compatibility); `greetings.cleanService` keeps its join processing and drops its own delete in favor of the rules.
- `scheduled_deletions(id BIGSERIAL PK, chat_id, message_id, delete_at TIMESTAMPTZ, kind VARCHAR(40), state VARCHAR(8) DEFAULT 'pending' CHECK IN ('pending','acting'), claim_token VARCHAR(36), claimed_at, attempts INT DEFAULT 0, created_at, UNIQUE(chat_id, message_id))` plus a partial index on `delete_at WHERE state = 'pending'`. `UNIQUE(chat_id, message_id)` with `ON CONFLICT (chat_id, message_id) DO UPDATE SET delete_at = LEAST(...)` makes double scheduling (self-destruct plus a bot-message rule) harmless and keeps the earliest. Rows are removed when done. No `DeleteCache` (read fresh, never cached), not in backup. A row more than 47 h after its `created_at` is given up (Telegram refuses older deletes).

### 7.2 Scheduler API and who calls it

```go
// package deletion repo: Schedule(chatID, msgID int64, kind string, delay time.Duration) error  (ON CONFLICT keep earliest)
// package modules:
func scheduleBotMessage(chat int64, msg *gotgbot.Message, kind string) // reads chat_delete_rules; no rule means no row; delay 0 = delete inline in a recovered goroutine
```

| Rule kind | Hook (new vs modified) |
|-----------|------------------------|
| `service:*` | `messageHygieneWatcher` (new, group `-1`): a service message is recognized by a table of gotgbot `Message` fields; schedules, returns `ContinueGroups`. Delay 0 deletes inline (recovered goroutine). Join/leave service messages are handled here too, so the group `0` `cleanService` keeps only its join-processing role. |
| `member:selfdestruct` | Same watcher: real-user messages that are not exempt (approved, admin by the cached list, bot, `sender_chat`) get a row. Commands and everything else included, because the watcher is before group 0. |
| `member:commands` | The Processor wrapper (7.4), after all groups ran. Not a queue row. |
| `member:edited` | `editedMessageHandler` (7.5). |
| `bot:welcome`, `bot:goodbye`, `bot:rules` | After the successful send in `SendWelcomeMessage`, `leftMember`, `sendRules` (modified, one line each). |
| `bot:punish_auto` | The requirement notice, `warn` limit punishments from `applyWarn`, captcha failure notices if wanted. |
| `bot:punish_manual` | The final reply of `/ban /tban /kick /mute /tmute /warn` family. These are about 15 `msg.Reply` sites in `bans.go`, `mute.go` and `warns.go` (the `moderationCommand` helpers `banReplyWithButton`, `kickReply`, `muteReplyWithButton` plus the legacy handlers). A call-site change is the right shape because the categories are explicit; a `BotClient` decorator cannot know the category. |

Call sites must check the rule before inserting, so a chat with deletion off costs one cached read and no write.

### 7.3 Worker (copy of the lockdown worker shape)

- `StartDeletionWorker(b)` in `postInit` right after `StartLockdownWorker`; `RegisterDeletionDrain` in `main.go` after the DB-close handler, with `deletionWorkerStopWait` (5 s) plus `shutdownDrainGrace`. Every row resumes from the database, so a worker cut off loses nothing. It also joins the insert goroutines (`deletionInserts` counter) so a scheduled row in flight is written before the DB closes.
- Loop: tick **1 s** plus a `deletionWake` channel (a short delay scheduled while idle wakes it). Each cycle, with its own `RecoverFromPanic`: (1) release stale claims older than `deletionStaleClaim` (2 min) back to `pending` without counting an attempt; (2) list up to 200 due rows ordered by `delete_at`; (3) **batch claim** with one `UPDATE ... SET state='acting', claim_token=?, claimed_at=? WHERE id IN (...) AND state='pending'`, then `SELECT ... WHERE claim_token=?`. That is portable to the SQLite tests and keeps a multi-replica race to one winner per row, without `SKIP LOCKED`; (4) group by chat and call `deleteMessages` (at most 100 ids) through a new `deletionPacer` (`fuku:delete:pace:next` and `:block`, its own budget like the lockdown and staff pacers); (5) delete the rows of a call that succeeded (`deleteMessages` skips missing ids and still succeeds); (6) on 429/timeout/5xx return the rows to `pending` with `attempts+1` (three tries); on a permanent answer (bot not in chat, kicked, no `can_delete_messages`) delete the rows and set a 10-minute negative latch `fuku:delete:norights:<chat>` so a rightless group is not hammered; (7) prune rows past 47 h.
- A delete whose message is already gone is success. The worker never logs per message.
- Without Redis the pacer falls back to a local interval (existing behavior) and deletion still works.

### 7.4 Command deletion: the Processor seam

`main.go` builds the dispatcher with `Processor: tracing.TracingProcessor{}`. Wrap it in `deletionPostProcessor{inner: tracing.TracingProcessor{}}` (a new type in `fuku/modules`): `ProcessUpdate` calls `inner.ProcessUpdate`, then `afterUpdate(b, ctx)` with its own `defer RecoverFromPanic`. `afterUpdate` returns at once unless the update is a group message whose first entity is a `bot_command` at offset 0 addressed to this bot (or no `@bot`), the chat has `member:commands`, and the sender is not an anonymous admin. Then it deletes the message inline. Why this and not a queue row or a watcher:

- The handler (group 0) has already replied to the command; a delete earlier would break `msg.Reply`.
- It needs no second command list (the same principle as `raid_flood`), covers commands registered with raw `handlers.NewCommand` that bypass `WrapCommand`, and survives nothing it needs to survive: a crash between handling and delete leaves one command message, which is acceptable.
- **Skip anonymous admins**: the proof-tap flow re-runs the cached command and replies to it; deleting the message first would fail that reply.
- Also skip a command that the dispatcher left unhandled and addressed to another bot.
- Cost: one cached rules read per command message. Rules are loaded only when the message is a command, so non-command traffic pays nothing.

### 7.5 Edited messages

`edited_message` is already delivered, and no handler wants it. Implement `editedMessageHandler` as a small `handlers.Handler` (`CheckUpdate`: `ctx.EditedMessage != nil`, group or supergroup, real user sender, not a bot; `HandleUpdate`; `Name`) registered with `AddHandlerToGroup(..., 1)`. It checks `member:edited`, exempts approved and admins (cached list), then deletes inline (delay 0) or schedules. Do **not** use `NewMessage(...).SetAllowEdited(true)`: that would also match every new message and force an in-body `ctx.EditedMessage` check on the hot path. Group `1` is free today; edited updates match no other handler, so ordering against other groups is irrelevant. Note for `AGENTS.md`: this is the first handler to consume edits, so antiflood, locks, blacklists and aispam still do not see edits (an existing gap, out of scope).

---

## 8. Remaining pages

Each page is one `registerSettingsPage` call from its own `settings_<page>.go`, and each setting needs an `applyX` function **beside its command** first (the repo rule). Today only captcha, antiflood and auto-lockdown have them, so the extraction is the real work:

| Page | Apply functions to extract | Right |
|------|----------------------------|-------|
| Warnings | `applyWarnLimit`, `applyWarnMode` from `setWarnLimit`/`setWarnMode` | admin (verify what the commands require) |
| Reports | reports toggle | admin |
| Language | `applyChatLanguage` (the `/lang` handler) | admin |
| Log channel | category toggles; channel set/unset by typing in PM (an input kind) | admin |
| Anonymous-admin mode | `applyAnonAdmin` (`admin.SetAnonAdminMode`) | admin |
| Locks | `applyLock(type, on)` over the lock-types table | `settingsRightChangeInfo` or the lock command's check |
| Lists (blacklists, filters, notes) | browse and delete use existing repos; **add** is a PM input kind (a filter is two-step: keyword in `x`, then the reply content) | admin |

Each new setting adds the standing test set (parity, drift, authority-parity, worst-case callback bytes, page-fits-every-locale), a `settings_` limits-table row, and the Main-page state function. `TestSettingsStaffPrivacy` must still pass: no new page may name the Staff Group.

---

## 9. Migrations (append-only, timestamp above 20261101120000, one transaction each)

| Order | File (suggested) | Contents |
|-------|------------------|----------|
| 1 | `20261103120000_add_settings_access.sql` | `chat_settings_access`, `chat_managers` (+ index on `user_id`) |
| 2 | `20261103130000_extend_greetings_rules_editor.sql` | greetings columns, rules columns, `greeted_members` |
| 3 | `20261103140000_add_member_requirements.sql` | `chat_member_rules` (CHECK on punishment), `member_requirement_holds` |
| 4 | `20261103150000_add_message_deletion.sql` | `chat_delete_rules` (CHECK on delay), `scheduled_deletions` (UNIQUE, partial index), copy of `clean_service_settings` |

Same-file rules from `AGENTS.md`: every statement idempotent (`IF NOT EXISTS`), no top-level `BEGIN/COMMIT`, no `CREATE INDEX CONCURRENTLY`, `fuku/db/migrations/runner.go` and `scripts/migrate_psql.sh` agree; `duplicate_object` is swallowed on `ADD CONSTRAINT`, so add each CHECK in the `CREATE TABLE`, not later. Add every new model to the `AutoMigrate` lists of `fuku/modules/test_harness_test.go`, the new repo packages' `testmain_test.go`, and `fuku/db/testmain_test.go`. `make test-postgres-integrity` gets: `TestScheduleDeletion` (unique key keeps the earliest), `TestDeletionBatchClaim` (a second claimer gets nothing), `TestMemberHoldFirstWins` (the PK insert), `TestMemberRulePunishmentCheck`, `TestDeleteRuleDelayCheck`. Each rests on a unique index, a CHECK or a conditional update.

---

## 10. Data flow changes

**Open a group from PM:** `/settings` (private) -> `settingsHub` -> SELECT from `chat_managers` + access -> list message with pick buttons -> press `a=bk&c=<id>` -> `verify` (decode, service identity, `settingsAuthorize` = live member + gate, `getChat` type check) -> Main page edited in place.

**Edit a welcome text:** press `a=in&p=w&k=wt` -> `verify` + `settingsRightChangeInfo` -> `Begin` hash -> prompt page (Cancel) -> user types -> group `-1` consumer: GET, claim, `settingsAuthorize` again, parse, `applyWelcomeText`, `DeleteCache`, delete claim -> delete input message -> edit menu back with a notice.

**Join with requirements:** `chat_member` or service message -> group 0 -> `ProcessSingleJoin`: Claim -> Screen (rules cached, free rules, then cached photo/channel result, AI job enqueued) -> punished: executor (+hold, +notice, +schedule) and stop; else captcha or welcome (+`scheduleBotMessage`).

**Scheduled delete:** watcher or send site -> `deletion.Schedule` -> worker on any replica -> batch claim -> `deleteMessages` (paced) -> row removed.

---

## 11. Patterns to follow and anti-patterns

**Follow:** conditional-write-decides (holds, greet-once, claims), DB-driven worker with release-stale and drain (deletion), stateless callbacks that carry object IDs and re-authorize (hub, I-fixed-it), one apply function per setting, per-rule/per-type rows instead of wide tables, marker `until_date` to own a restriction, Lua compare-and-delete on a nonce (await).

**Avoid:**

- **`time.AfterFunc` for deletions or expiries.** Lost on restart and on the replica that dies; the queue exists for this.
- **Trusting the stored await state for authority.** It records intent only; re-run `settingsAuthorize` on consumption.
- **Putting the owner gate in the page code or a per-page check.** It must be in `verify`/`settingsAuthorize` so no page can forget it.
- **Using the cached admin list as the gate or as the "I fixed it" authority.** Live `getChatMember`, the repo's standing rule.
- **A watcher at group 1 or later for command deletion**, or deleting before the handler replied.
- **A watcher returning `nil`** in group `-1` or `0`: it silently kills the rest of that group. New watchers return `ContinueGroups` except on a deliberate consume.
- **Unmuting with `chatInfo.Permissions`.** Always `resolveUnmutePermissions`; refuse on error.
- **Punishing on an unknown check.** A photo or channel lookup that errors (privacy, bot not channel admin, 429) is "unknown": no punishment, no hold, a page warning.
- **Putting message text in callback data or Redis keys.** Tokens only; the keyword of a filter lives in the await hash.
- **A new group number for each feature.** Use `-1` (registered after the tracker) and `1`; update the `AGENTS.md` group table in the same commit.

## 12. Scalability

| Concern | Few groups (this bot) | Hundreds of groups | Notes |
|---------|-----------------------|--------------------|-------|
| Per-message watchers | one cached rules read each | same, plus Redis pass cache | keep early returns; no DB read per message when no rule is on |
| `scheduled_deletions` writes | direct insert | self-destruct on a busy group is one insert per message; batch the inserts in a bounded channel if needed | partial index on pending keeps the poll cheap |
| Delete throughput | `deleteMessages` per chat | paced fleet-wide; one 429 block shared | |
| Hub query | one indexed SQL | same | no per-row Telegram call |
| Name AI | negligible | verdict cache by name hash, hourly budgets | |

## 13. Suggested build order (dependencies)

1. **Hub and gate** (nothing depends on it, everything rides on it). Migration 1, `fuku/db/access`, admin-cache hook, `settingsAuthorize` extraction and the four doors, `requireSettingsAccess`, hub, Access page, Main grid layout, `settingsRightOwner`. Tests: gate parity across the four doors, hub list never shows a gated group, Redis down, old `/settings`-in-PM refusal tests updated.
2. **PM input state and the editor.** (a) await store, group `-1` consumer, `/cancel`, input registry, proven with one trivial kind; (b) migration 2, repos, apply functions, renderer, Rules/Welcome/Goodbye pages, greet-once, Test. Depends on 1 (authorize, pages). Unblocks Lists and log-channel set.
3. **Member requirements.** Migration 3; `applyWarn` extraction; local rules (last name, username, scripts) with join `Screen` and the message watcher; executor and holds; "I fixed it"; then photo and channel checks; then `ClassifyName` and the name AI job. Depends on 1 for its page; soft-depends on 4 only for notice deletion (one added line later).
4. **Message deletion.** Migration 4; repo, worker, drain, `StartDeletionWorker`; service-message and self-destruct watcher; Processor wrapper for commands; edited handler; bot-message call sites; wire requirement notices. Independent of 2 and 3 technically; after them to match the owner's priority.
5. **Remaining pages.** Extract the apply functions command by command, then the pages. Lists add (needs 2's input registry) last. The simple toggle pages (anonymous-admin, reports, language) can be pulled forward as early proof of page 1 if a quick win is wanted.

Phases most in need of deeper research: **3** (Telegram behavior of `getUserProfilePhotos` privacy, linked-channel lookups, the captcha interplay, the name-AI pool decision), **4** (`deleteMessages` limits and permission errors, service-message field table across gotgbot's `Message`, the exact set of manual-punishment reply sites), **2** (entity-to-HTML parity with `/setwelcome`, `show_caption_above_media` support in this gotgbot, album handling for media input). Phases 1 and 5 follow established repo patterns.

## 14. `AGENTS.md` updates this milestone owes (same commit rule)

Handler group table (`-1` additions, group `1`), callback namespaces (`settingsh|v1`, `memreq|v1`, the new settings action codes and the `in`/`ic` pair), the new Redis keys, the new tables and their "never cached / not in backup" statements, `skipLocal` entries, the Processor wrapper, the new shutdown drain, the new `make test-postgres-integrity` names, the owner-gate rule beside the Permissions section, and the settings test toolkit entries.

## Sources

- `/home/user/Fuku_Robot/AGENTS.md`, `.planning/PROJECT.md` (HIGH)
- `fuku/modules/settings.go`, `settings_menu.go`, `settings_pages.go`, `settings_card.go`, `greetings.go`, `rules.go`, `connections.go`, `deeplink_router.go`, `membership.go`, `warns.go`, `lockdown.go`, `lockdown_worker.go`, `shutdown_drains.go`, `chat_permissions.go`, `main.go`, `fuku/utils/cache/adminCache.go`, `fuku/db/cache/local.go`, `fuku/utils/helpers/command_pipeline.go` (HIGH, read in full or in the relevant parts)
- gotgbot v2.0.0-rc.36 source in the module cache: `ext/dispatcher.go` (`iterateOverHandlerGroups`, `Processor`), `ext/handlers/message.go` and `command.go` (`CheckUpdate`, `AllowEdited`) (HIGH)
- Bot API behavior of `deleteMessages`, deleting incoming private messages, `show_caption_above_media`, `getUserProfilePhotos` (MEDIUM: from knowledge of the API, not re-fetched in this session; verify in the owning phases)
