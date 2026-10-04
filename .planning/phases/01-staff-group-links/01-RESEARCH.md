# Phase 1: Staff Group Links - Research

**Researched:** 2026-10-04
**Domain:** Telegram group-management bot (Go 1.26, gotgbot v2 rc.36, GORM on PostgreSQL/SQLite, Redis). Cross-chat ownership links, ownership-change detection, group deep-link payloads, chat-ID migration, a live status panel.
**Confidence:** MEDIUM-HIGH. HIGH for codebase facts, gotgbot dispatcher semantics and Bot API server behaviour (all read from source this session). MEDIUM for Telegram client/server behaviour that no document states (flagged `[ASSUMED]` and listed in the Assumptions Log). One finding is LOW and has a built-in fallback: whether Telegram emits `chat_member` on an ownership transfer.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

#### Linking flow
- **D-01:** The main way to link is an **"Add group" button in `/staff`**. It opens Telegram's group picker through a `t.me/<bot>?startgroup=<payload>` link. When the owner picks a group, Telegram posts `/start@<bot> <payload>` there with the owner as sender. The bot then runs the live owner-of-both checks (D-03) and links the group. Where Telegram supports it, the link also asks for the bot's admin rights in the same step (restrict members, delete messages). The payload only says *which Staff Group*. It never grants authority, because the live checks do.
  - Research must confirm three things: the picker works when the bot is already in the chosen group; `/start@bot <payload>` arrives in the group with the picking user as `from`; and what the payload length and character limits are.
  - Today `/start` in a group (`alita/modules/help.go`, `start`) only replies with "ask me in PM", so the group `/start <payload>` path is new.
- **D-02:** There is also a typed fallback, **`/linkstaff`**, sent in the group being linked.
  - With no argument, it links to the Staff Group the issuer owns, when there is exactly one. Candidates are found by recorded owner, then verified live.
  - If the issuer owns several Staff Groups, `/linkstaff <staff group ID>` is required. `/staff` displays that ID.
- **D-03:** Both link paths run the same checks, all live and none from cache (SETUP-04):
  - the issuer is the live `creator` of both groups
  - the target is a supergroup
  - the target is not itself a Staff Group
  - the target is not already linked to any Staff Group
  - the chosen Staff Group really is a Staff Group

  Each refusal states its reason.
- **D-04:** **Link even if the bot lacks rights in the target group**, and warn. If the bot isn't an admin there, or can't restrict members, the link still goes through. The confirmation carries a warning, and `/staff` shows the group as broken. Phase 2 marks such groups as failed, with the reason.
- **D-05:** **Two ways to unlink:**
  - An Unlink button next to each group in `/staff`. It works only for the owner of both groups, is re-checked live on every press, and asks "Are you sure?" before acting.
  - `/unlinkstaff`, typed in the linked group.
- **D-06:** **Link and unlink notices go only to the Staff Group.** Nothing is posted in the linked group, and its members never see a notice.
- **D-07:** **Linking leaves no trace in the linked group.** The visible command message is deleted when the bot has delete rights. That covers the picker's `/start@bot …` and a typed `/linkstaff` or `/unlinkstaff`. The confirmation, or the refusal reason, is posted in the Staff Group. If the bot can't delete, the message stays.

#### Staff Group rules
- **D-08:** To make a group a Staff Group (SETUP-01):
  - The issuer must be the live `creator` of the group.
  - The chat must be a group or supergroup, not a channel or a private chat.
  - **The bot must already be an admin there.** This is what lets the bot receive `chat_member` updates and see ownership changes in the Staff Group.

  Each refusal states its reason.
- **D-09:** **One owner can have several Staff Groups.** — **Reversibility:** costly — The schema, the `/linkstaff` lookup that picks a Staff Group when there's no argument, and the panel all assume more than one Staff Group per owner. Narrowing to one later means a data cleanup and a uniqueness migration.
- **D-10:** **The two roles never overlap.** A Staff Group can't be linked to any Staff Group, and a linked group can't become a Staff Group until it's unlinked. Enforce this in the database where possible, not only in code. This also rules out a group linking to itself and chains of links. — **Reversibility:** one-way — Database constraints enforce it. Allowing nested Staff Groups later needs a migration and rethinking who has authority over whom.
- **D-11:** **The Staff Group may be a basic group or a supergroup. Linked groups must be supergroups.** Linking a basic group is refused with "upgrade to a supergroup first", because Telegram can't mute anyone in a basic group.
  - When the Staff Group upgrades from basic to supergroup (`migrate_to_chat_id` / `migrate_from_chat_id`), its Staff status and every link row move to the new chat ID in one transaction (SETUP-07).
  - Nothing in the codebase handles chat migration today; this is new.

#### When links break
- **D-12:** **A link belongs to the person who made it.** The link row stores that user's ID. The link is valid only while that same user is the live `creator` of *both* groups. If either group's creator differs, the link is removed. This holds even when both groups go to the same new owner: they have to relink. — **Reversibility:** one-way — The stored column and the validity rule define what a "link" is; every later phase's authority checks depend on it.
- **D-13:** **When a link is removed automatically, the Staff Group is told** which group was unlinked and why (for example, "its owner changed"). No private messages are sent, and nothing is posted in the linked group.
- **D-14:** **Losing the bot doesn't break a link.** If the bot is removed from a linked group, or loses admin or the restrict right there, the link stays. `/staff` shows it as broken ("bot not in group", "bot not admin", "bot can't restrict"), and the Staff Group gets **one** heads-up when that state changes, not one on every check. Adding the bot back fixes it with no relinking.
- **D-15:** **Ownership changes are caught in three ways:**
  1. Event-driven: `chat_member` and `my_chat_member` updates where the bot is an admin. The research suggests a dedicated handler group (e.g. `-3`), because the existing group-0 `NewChatMember` handler in `bot_updates.go` would shadow a group-0 watcher.
  2. A **background re-check about every hour** of every link, spread out so it doesn't burst API calls. This is what satisfies "the link disappears without anyone running a command" for groups where the bot isn't an admin.
  3. A live check whenever `/staff` opens or Refresh is pressed, and before every staff action from Phase 2.

#### `/staff` panel
- **D-16:** **`/staff` works only inside a Staff Group.** Anywhere else it is ignored, or at most answered with one short hint. It never reveals link details in a linked group.
- **D-17:** **Any member of the Staff Group can open `/staff`.** The Add group, Unlink and confirm buttons work only for the owner of both groups, re-checked live on every press. Refresh works for any member.
- **D-18:** **The status is live when `/staff` opens, and Refresh updates the same message in place.** Each group shows: linked; bot in group and bot is admin; bot can restrict members; owner still matches. The in-lockdown status is added in Phase 4. An owner mismatch found while building the panel removes that link and posts the notice from D-13.
- **D-19:** **Built for under 10 linked groups per Staff Group:** one message, no paging. It must still behave sensibly with more, by paging or truncating cleanly before Telegram's message size limit.
- **D-20:** The panel shows the Staff Group help text and the Staff Group's chat ID, which `/linkstaff <id>` needs when an owner has several Staff Groups (D-02).

### Claude's Discretion
- **Command names and aliases.** `/setstaff`, `/unsetstaff`, `/linkstaff`, `/unlinkstaff` and `/staff` are the working names. Keep them consistent with the repo's naming and add them to help and the generated docs.
- **Removing Staff status (SETUP-02):** whether it asks for an "Are you sure?" tap first, and the wording of the Staff Group notice. Removing the status unlinks every group.
- **The Staff Group changes owner.** Default: under D-12, all its links break, because the person who made them no longer owns it. The Staff status stays with the group, and the new owner can remove it or link their own groups. A notice goes to the Staff Group.
- **The bot is removed from the Staff Group, or loses admin there.** Default, matching D-14: keep the status and links, and fix it when the bot is re-added. The hourly re-check may clean up a Staff Group that Telegram says no longer exists.
- **Anonymous admins** running `/setstaff`, `/linkstaff` and the rest can't be proven to be the owner. Either refuse with "post as yourself", or reuse the existing anonymous-admin "prove it" button (`RegisterAnonymousAdminHandler`). The live creator check must never be skipped.
- **Where refusals go.** When `/linkstaff` is run by someone who doesn't own the Staff Group it names, the refusal must **not** be posted into that Staff Group, so nobody can spam a Staff Group this way. Use a short reply in the group that deletes itself, or a private message.
- **Picker payload format.** An opaque token or the Staff Group ID, with or without an expiry. It needs no secrecy, because the live checks decide.
- **The hourly re-check:** how it's paced, and making sure only one replica runs it at a time (for example, a Redis lock). Several replicas are the deployment target, and PLAT-01 applies here even though it's mapped to Phase 2.
- **Status icons and wording** in the panel, and the exact notice texts. All strings go in all 7 locale files (PLAT-03).
- **A linked group's chat ID changing.** It shouldn't happen, since linked groups are supergroups only, but re-keying it too is cheap and harmless.

### Deferred Ideas (OUT OF SCOPE)
- Showing `/staff` in a private chat (listing the Staff Groups you own) or in a linked group ("linked to Staff Group X"): offered, but the owner chose "Staff Group only". Could come back with the settings menu (Phase 9).
- Messaging the person who made a link privately when it breaks: offered; the owner chose a Staff Group notice only.
- Nested Staff Groups (head staff over moderator rooms): rejected for now (D-10).
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| SETUP-01 | Owner designates a group as Staff Group; channels / unusable groups refused with a reason | `/setstaff` flow; type check from the update's own `Chat.Type`; live owner + bot-admin check (Patterns 1, 2); `RequireGroup()` lets channels through, so the handler must refuse them itself (Pitfall 8) |
| SETUP-02 | Only the Staff Group owner can remove status; unlinks all | `/unsetstaff` with live creator check (never the recorded owner); one-transaction delete of the row and its links (Pattern 3) |
| SETUP-03 | Owner of both can link | Shared `linkGroup` flow behind the picker `/start@bot` route and `/linkstaff` (Pattern 5); group `/start` payload route (Q2) |
| SETUP-04 | Refused if not owner of both (live) or already linked | Tri-state live owner helper; `INSERT ... ON CONFLICT DO NOTHING` on `UNIQUE(group_chat_id)` and rows-affected as the race arbiter (Pattern 3) |
| SETUP-05 | Owner of both can unlink | `/unlinkstaff` and the panel Unlink button with confirm step; callback carries a link row ID, authority re-derived from the row (Pattern 9) |
| SETUP-06 | Link removed as soon as the same person no longer owns both; ownership changes watched | Q1 answer: `chat_owner_changed` / `chat_owner_left` service messages are the dedicated signal (new finding), plus `chat_member`, an hourly sweep and live checks (Pattern 4, 7) |
| SETUP-07 | Links and Staff status survive group to supergroup upgrade | `message.Migrate` filter, idempotent `RekeyChat` transaction, 400 `migrate_to_chat_id` error parameter as a second detector (Q3, Pattern 8) |
| SETUP-08 | `/staff` panel: help text + per-link status | Pure render function + live status builder with bounded concurrency; paging before 4096 chars (Pattern 6) |
| PLAT-03 | Every new string in all 7 locales | Actual locale files are `en es fr hi id pt ru`; `make check-translations` only sees literal `GetString("key")` calls (Pitfall 11); add a parity test |
</phase_requirements>

## Summary

Phase 1 is mostly composition of patterns the repo already has (repository + cache invalidation, `WrapCommand`, `callbackcodec`, ticker workers with a lifecycle context), plus three genuinely new capabilities: (1) ownership-change monitoring, (2) a group-side `/start@bot <payload>` route, and (3) chat-ID migration. The schema is two small tables. The risk is not volume; it is correctness of authority (every check live, fail closed, errors distinguished from mismatches) and of exactly-once side effects across replicas.

Three findings change the plan relative to the prior milestone research and CONTEXT.md. **First**, Telegram now has dedicated ownership service messages: `chat_owner_changed` (`new_owner`) and `chat_owner_left` (optional `new_owner`) exist in Bot API 10.3 and in the vendored gotgbot, with ready-made `message.ChatOwnerChanged` / `message.ChatOwnerLeft` filters. The earlier "there is no dedicated ownership update" claim is refuted. Whether `chat_member` also fires on a transfer is undocumented, so the design uses every signal plus the hourly sweep and live checks, and never depends on one. **Second**, the gotgbot dispatcher treats `ContinueGroups` as "try the next handler in the *same* group", not "jump to the next group". The ARCHITECTURE.md reasons for group `-3` (shadowing) and for the Phase 2 command-gate trick are therefore wrong in letter; group `-3` is still the right place, for a different reason (run before `-1`'s `botJoinedGroup`, which returns `EndGroups`). **Third**, `botJoinedGroup` makes the bot leave any basic group it is added to, so a basic-group Staff Group (D-11) is practically unreachable; the re-key code is still required (SETUP-07) and testable.

**Primary recommendation:** Build two tables (`staff_groups`, `staff_group_links`) with uncached authority reads, one shared `linkGroup` flow behind both the picker route and `/linkstaff`, one `recheckLink` function fed by four triggers (service message, `chat_member`, hourly Redis-locked sweep, panel open), and make every removal or health transition an `UPDATE/DELETE ... WHERE` whose rows-affected result decides who posts the single Staff Group notice.

## Telegram Platform Findings (answers to the D-01 / roadmap research questions)

### Q1. Does Telegram send `chat_member` / `my_chat_member` when ownership is transferred?

| Signal | What the evidence says | Confidence |
|--------|------------------------|------------|
| **`chat_owner_changed` service message** (dedicated, new) | Bot API 10.3 defines it. `Message.chat_owner_changed`: "Optional. Service message: chat owner has changed" with `ChatOwnerChanged.new_owner`: "The new owner of the chat" [VERIFIED: api.json from github.com/PaulSonOfLars/telegram-bot-api-spec, version "Bot API 10.3", release_date "August 24, 2026"]. Present in the vendored gotgbot: `ChatOwnerChanged *ChatOwnerChanged \`json:"chat_owner_changed,omitempty"\`` [VERIFIED: gotgbot gen_types.go:8077]. The Bot API server emits it from TDLib's `messageChatOwnerChanged`: `object("chat_owner_changed", JsonChatOwnerChanged(content, client_));` [VERIFIED: tdlib/telegram-bot-api Client.cpp:5333-5336]. TDLib maps MTProto `ChangeCreator` to it [VERIFIED: tdlib/td MessageContent.cpp:12068-12070]. It is a normal `message` update. "All bots, regardless of settings, will receive all service messages" [CITED: core.telegram.org/bots/faq, via search snippet], so no admin right is needed. | HIGH that it exists and is forwarded; MEDIUM that it fires for every transfer path (never tested live) |
| **`chat_owner_left` service message** | `ChatOwnerLeft.new_owner`: "Optional. The user who will become the new owner of the chat if the previous owner does not return to the chat" [VERIFIED: api.json]. Mapped from TDLib `NewCreatorPending` [VERIFIED: MessageContent.cpp:12063-12066]. Under D-12 the old owner is no longer `creator` at that moment, so this is also a removal trigger. | HIGH exists; MEDIUM semantics |
| **`chat_member`** | Official text: "A chat member's status was updated in a chat. The bot must be an administrator in the chat and must explicitly specify "chat_member" in the list of allowed_updates to receive these updates." [VERIFIED: api.json, `Update.chat_member`]. The Bot API server does **not** filter creator transitions: every TDLib `updateChatMember` becomes a `chat_member` or `my_chat_member` update (`auto update_type = is_my ? UpdateType::MyChatMember : UpdateType::ChatMember;`) [VERIFIED: Client.cpp:18702-18722], and TDLib passes every `updateChannelParticipant` through (`send_update_chat_member(...)`) [VERIFIED: DialogParticipantManager.cpp:1714-1768]. **Whether Telegram's servers emit `updateChannelParticipant` for the old and new creator on a transfer is not documented anywhere I could reach.** | LOW (`[ASSUMED]` A1) |
| **`my_chat_member`** | Only about the bot's own status ("The bot's chat member status was updated in a chat") [VERIFIED: api.json]. Irrelevant to ownership; relevant to D-14 (bot removed / demoted). | HIGH |
| **Staleness** | The Bot API server drops member updates older than a day: `auto left_time = update->date_ + 86400 - get_unix_time();` then `if (left_time > 0) {` [VERIFIED: Client.cpp:18704-18705]. A bot that was down more than 24 h never sees them, so a startup sweep is needed. | HIGH |
| **Config** | `AllowedUpdates` already lists `"message"`, `"callback_query"`, `"my_chat_member"`, `"chat_member"` [VERIFIED: alita/config/config.go:340-352]; the webhook path passes the same slice (`AllowedUpdates: config.AppConfig.AllowedUpdates` [VERIFIED: alita/utils/httpserver/server.go:219]). | HIGH |

TDLib's changelog says ownership transfer is supported for "supergroup and channel chats" [CITED: tdlib/td CHANGELOG.md, "Added support for transferring ownership of supergroup and channel chats"], which fits linked groups being supergroups only.

**Consequences for the plan.**
1. Register a watcher for `chat_owner_changed` / `chat_owner_left` service messages. It is the only ownership signal that does not need the bot to be admin and does not depend on an unverified Telegram behaviour. Treat the message as a **hint to re-verify live**, never as the authority (the new owner in the payload is used only to refresh `staff_groups.owner_user_id`).
2. Keep a `chat_member` watcher (old or new status `creator`) as an opportunistic extra.
3. The hourly sweep and live checks remain the guarantee (the roadmap's research flag, answered: if `chat_member` does not fire, the service messages, sweep and live checks still remove stale links).
4. Add a **manual end-of-phase UAT**: transfer ownership of a throwaway supergroup (bot admin) and log the raw updates. This resolves A1 and decides whether the `chat_member` watcher is worth keeping.
5. The prior PITFALLS claim "The Bot API has no dedicated 'ownership transferred' update" is REFUTED for the current Bot API.

### Q2. The `t.me/<bot>?startgroup=<payload>` picker

| Question | Answer | Evidence | Confidence |
|----------|--------|----------|------------|
| Works when the bot is already a member of the chosen group? | Yes. The client flow invites the bot "(if it is not yet a member)" and then sends `/start`: "Invites a bot to a chat (if it is not yet a member) and sends it the /start command; requires can_invite_users member right." | [VERIFIED: tdlib/td td_api.tl:12212-12213 (`sendBotStartMessage`)]. TDLib is the reference client library; the official apps' picker UI behaviour for already-present bots is not documented. | MEDIUM (`[ASSUMED]` A3: confirm in UAT) |
| Does `/start@<bot> <payload>` arrive in the group with the picker as `from`? | Yes in intent: the client sends it as the user. `sendBotStartMessage` "Returns the sent message" from the current user. A command addressed to this bot is always delivered, even in privacy mode ("commands explicitly meant for them (e.g., /command@this_bot)") [CITED: core.telegram.org/bots/faq via search snippet]. gotgbot matches `/start@Bot payload` as command `start` and rejects other bots' usernames [VERIFIED: ext/handlers/command.go: `if len(split) > 1 && split[1] != strings.ToLower(b.User.Username) { return false }`]. Open edge: a picker user who has "remain anonymous" on may produce an anonymous-admin message (`SenderChat` = the group). | TDLib doc for the sender; the anonymous edge is `[ASSUMED]` A4 | MEDIUM / LOW |
| Payload charset | `A-Za-z0-9_-` only. TDLib validates with `is_base64url_characters`: `static bool is_valid_start_parameter(Slice start_parameter) { return is_base64url_characters(start_parameter); }` [VERIFIED: tdlib/td LinkManager.cpp:56-58]. Negative chat IDs are fine since `-` is allowed. | HIGH |
| Payload length | At most 64 characters [CITED: core.telegram.org/bots/features, deep linking, via search snippet: "start payloads ... A-Za-z0-9_-, at most 64 characters"]. | MEDIUM |
| Can admin rights be requested in the same link? | Yes. Form: `/<bot_username>?startgroup=<parameter>&admin=change_info+delete_messages+restrict_members` and the parameterless `/<bot_username>?startgroup&admin=...` [VERIFIED: LinkManager.cpp:2873-2877]. Rights are `+`-separated names: `"change_info"`, `"post_messages"`, `"edit_messages"`, `"delete_messages"`, `"restrict_members"`, `"invite_users"`, `"pin_messages"`, `"manage_topics"`, `"promote_members"`, `"manage_video_chats"` ... [VERIFIED: LinkManager.cpp:418-440]. Use `admin=restrict_members+delete_messages` (D-01). | HIGH |
| Bot already admin + rights requested? | Rights are **combined**, never replaced: "If administrator rights are provided by the link, call getChatMember to receive the current bot rights in the chat and if the bot already is an administrator, check that the current user can edit its administrator rights, combine received rights with the requested administrator rights, show confirmation box to the user" [VERIFIED: td_api.tl:9402-9406 (`internalLinkTypeBotStartInGroup`)]. The user can untick rights in the confirm box, so D-04's "link even if rights are missing" is necessary. | MEDIUM (library doc; official clients assumed to match) |
| Who can use the picker for a public supergroup? | "bots can be added to a public supergroup only by administrators of the supergroup" [VERIFIED: td_api.tl:9403]. The owner qualifies. The start message is sent after the bot is added: "Then, if start_parameter isn't empty, call sendBotStartMessage with the given start parameter and the chosen chat" [VERIFIED: td_api.tl:9406-9407]. | MEDIUM |

`/start` in a group today: `start` has a non-private branch that only replies `help_pm_questions` [VERIFIED: alita/modules/help.go:373-381]: `text, _ := tr.GetString("help_pm_questions")`. `ctx.Args()` for `/start@bot stf_123` is `["/start@bot", "stf_123"]`, so `len(args) == 2` is the group-payload case.

### Q3. Migration service messages in gotgbot rc.36 and which arrives first

- Fields: `MigrateToChatId int64 \`json:"migrate_to_chat_id,omitempty"\`` and `MigrateFromChatId int64 \`json:"migrate_from_chat_id,omitempty"\`` on `gotgbot.Message` [VERIFIED: gotgbot gen_types.go:8093,8095].
- Filters exist, in `ext/handlers/filters/message`: `func Migrate(msg *gotgbot.Message) bool { return msg.MigrateFromChatId != 0 || msg.MigrateToChatId != 0 }`, plus `MigrateFrom` and `MigrateTo` [VERIFIED: filters/message/message.go:272-282]. Use `handlers.NewMessage(message.Migrate, h)`.
- Which chat each arrives in (from the Bot API server): `messageChatUpgradeTo` emits `object("migrate_to_chat_id", td::JsonLong(chat_id));` and `messageChatUpgradeFrom` emits `object("migrate_from_chat_id", ...)` [VERIFIED: Client.cpp:5033-5046]. By construction `migrate_to_chat_id` is a message **in the old basic group** (old ID = `msg.Chat.Id`, new ID = `MigrateToChatId`) and `migrate_from_chat_id` is a message **in the new supergroup** (new ID = `msg.Chat.Id`, old ID = `MigrateFromChatId`). The relative arrival order is not specified (`[ASSUMED]` A7). **Handle both messages with one idempotent `RekeyChat(old, new)`.**
- Second detector: any Bot API call on the dead basic group fails with `fail_query(400, "Bad Request: group chat was upgraded to a supergroup chat", ...)` carrying parameter `migrate_to_chat_id` [VERIFIED: Client.cpp:8821-8827]. In gotgbot this is `(*gotgbot.TelegramError).ResponseParams.MigrateToChatId`: `type ResponseParameters struct { MigrateToChatId int64 ... RetryAfter int64 ... }` [VERIFIED: gen_types.go:10679-10686; request.go:61-72 `ResponseParams *ResponseParameters`]. The sweep and panel builder should re-key when they see it. `helpers.IsExpectedTelegramError` already lists the substring "group chat was upgraded to a supergroup" [VERIFIED: alita/utils/helpers/telegram_helpers.go:80].
- Handler wiring: the message handler must call `.SetAllowBot(true)`. `Message.CheckUpdate` returns false when `ctx.EffectiveSender.IsBot() && !m.AllowBot` [VERIFIED: ext/handlers/message.go:61-63], and `Sender.IsBot` is `return s.Chat == nil && s.User != nil && s.User.IsBot` [VERIFIED: sender.go:137-139]. A service message whose `from` is a bot would otherwise be silently dropped.

## Corrections to prior research and CONTEXT.md (verified against current code)

| # | Prior claim | Reality | Impact |
|---|-------------|---------|--------|
| C1 | ARCHITECTURE.md Pattern 1 and D-15: a handler returning `ContinueGroups` "jumps to the next group"; the group-0 `NewChatMember` "would shadow" a group-0 watcher | `iterateOverHandlerGroups`: on `ContinueGroups` it does `// Continue handling current group.` then `continue` (next handler in the **same** group); on `nil` or any other handled return it hits `// Handler matched this update, move to next group by default.` then `break` [VERIFIED: gotgbot ext/dispatcher.go:294-296, 323-324]. `adminCacheAutoUpdate` returns `ext.ContinueGroups` [VERIFIED: alita/modules/bot_updates.go:105], so it would **not** shadow a later group-0 watcher. | Group `-3` is still recommended (D-15 allows "e.g. `-3`"), but the reason is ordering: `botJoinedGroup` at group `-1` returns `ext.EndGroups` when it leaves a chat [VERIFIED: bot_updates.go:64], which would stop later groups. For Phase 2, a staff `/ban` handler may "decline" with `ContinueGroups` and the real `/ban` still runs; the `CheckUpdate` gate is optional. |
| C2 | D-11 / SETUP-07: the Staff Group "may be a basic group" | `botJoinedGroup` runs on the bot's own join (`-1`, `!wasMember && isMember`): `if chat.Type == "group" || chat.Type == "channel" {` ... `_, err := b.LeaveChat(chat.Id, nil)` [VERIFIED: bot_updates.go:36,58; registration :233-242]. The bot leaves every basic group it is added to. | `/setstaff` can never succeed in a basic group, so SETUP-07 only triggers on a Staff Group that was a basic group before the bot's leave handler existed. Still implement and unit-test `RekeyChat` and both service messages; do not plan a live upgrade UAT for the Staff Group (see Open Question 1). |
| C3 | CONTEXT: `RequireUserOwner` "needs a variant that takes an explicit chat ID" | It already takes one: `func RequireUserOwner(b *gotgbot.Bot, ctx *ext.Context, chat *gotgbot.Chat, userId int64) bool` uses `extractChatFromContext(ctx, chat)`, which returns `chat` when non-nil [VERIFIED: chat_status/access.go:182-193; chat_status.go:81-84]. | The real gap is different: it returns a bare bool, so an API error and "not the owner" are indistinguishable. A sweep that unlinks on a transient error would be a bug. Add a tri-state helper (Pattern 2). |
| C4 | CLAUDE.md lists the locales as "en, id, pt, ro, tr, hi, es" | `locales/` holds `config.yml en.yml es.yml fr.yml hi.yml id.yml pt.yml ru.yml` [VERIFIED: `ls locales`]. The 7 languages are **en, es, fr, hi, id, pt, ru**; `config.yml` is the help alt-name pseudo-locale. | Add every key to those seven files. |
| C5 | CONTEXT/ARCHITECTURE: `chat.GetMember` for another user works for the bot generally | "The method is only guaranteed to work for other users if the bot is an administrator in the chat." [VERIFIED: api.json, `getChatMember`] | The hourly sweep must cover groups where the bot is not admin (D-15.2). Resolve the owner with `getChatAdministrators` (Pattern 2); `[ASSUMED]` A2 that it works for non-admin bots. |
| C6 | ARCHITECTURE Pattern 1 hot-path claim for Phase 2 | See C1 | Note for the Phase 2 researcher; no Phase 1 work. |

## Architectural Responsibility Map

The template tiers are web-centric; for this bot the tiers are the ones below.

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Authority: "who owns chat X right now" | Telegram Bot API (live `getChatAdministrators`) | `chat_status` helper | The only truth. Never the admin cache (30 min TTL) or the recorded owner. |
| Link / Staff Group persistence and invariants | Database (`staff_groups`, `staff_group_links`, constraints) | Repository package | UNIQUE and CHECK constraints are the race-free arbiter (D-10). |
| Command surface and refusals | Handler layer (`alita/modules/staff*.go`) via `helpers.WrapCommand` | Locale files | Pipeline answers failed checks; handlers answer domain refusals. |
| Ownership-change detection | Handler layer (group `-3` watchers) | Background worker (hourly sweep), panel open | Four triggers, one `recheckLink`. |
| Exactly-once notices and health transitions | Database (`DELETE/UPDATE ... WHERE`, rows affected) | Redis (sweep lock, optimisation only) | Correct with or without Redis; with several replicas. |
| `/staff` panel rendering | Pure render functions | Live status builder | Keeps Phase 9's menu a thin wrapper (ARCHITECTURE.md constraint on Phases 1-3). |
| Group `/start@bot` payload | Handler layer (`help.go` `start` + group deep-link registry) | Telegram client picker | Payload only names a Staff Group; authority comes from live checks. |

## Standard Stack

### Core (all already in `go.mod`; no new dependencies)
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `github.com/PaulSonOfLars/gotgbot/v2` | `v2.0.0-rc.36.0.20260919140833-240296efadb4` | Bot API, dispatcher, `message.ChatOwnerChanged/ChatOwnerLeft/Migrate` filters | Project stack [VERIFIED: go.mod:7] |
| `gorm.io/gorm` + `gorm.io/driver/postgres` / `sqlite` | v1.31.2 / v1.6.3 / v1.6.0 | Repository, transactions, `clause.OnConflict{DoNothing: true}` | Project stack [VERIFIED: go.mod] |
| `github.com/redis/go-redis/v9` | v9.22.0 | Sweep lock via `SetNX` | Existing precedent `client.SetNX(cache.Context, "alita:fed_export:"+fedID, "1", fedExportCooldown)` [VERIFIED: federations.go:1024] |
| `golang.org/x/sync` | v0.23.0 | `errgroup` for bounded concurrent live checks in the panel builder | Already a direct requirement (`golang.org/x/sync v0.23.0`, `singleflight` imported in `users.go` and `adminCache.go`) [VERIFIED: go.mod:26; grep] |
| `github.com/alicebob/miniredis/v2` | v2.39.0 | Real Redis for the sweep-lock test (`withMiniredis(t)`) | Existing helper [VERIFIED: alita/modules/antiraid_miniredis_test.go] |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| `getChatAdministrators` for owner resolution | `getChatMember(owner)` (existing `RequireUserOwner`) | `getChatMember` is "only guaranteed to work for other users if the bot is an administrator"; keep it only for the interactive issuer check where the repo pattern already exists |
| Postgres trigger for D-10 | App-level check inside the link transaction | Trigger closes the check-then-insert race; SQLite tests cannot run it (Postgres-gated test needed). See Open Question 4 |
| `time.AfterFunc` self-deleting reply | Durable delete queue | AfterFunc is lost on restart; a leftover short refusal is harmless. Existing precedent: `time.AfterFunc(30*time.Second, ...)` in captcha.go:1329 |

**Installation:** none.

**Version verification:** versions read from `go.mod` this session. `go test`, `go vet` and `go build` ran against them (see Environment Availability).

## Package Legitimacy Audit

No external packages are added in this phase, so the Package Legitimacy Gate does not apply. `golang.org/x/sync/errgroup` ships in the already-required `golang.org/x/sync v0.23.0` module. If a plan later adds a dependency, run `gsd_run query package-legitimacy check --ecosystem npm|pypi|crates ...` (Go modules are outside the seam's listed ecosystems; verify against `proxy.golang.org` and tag `[ASSUMED]` until confirmed).

**Packages removed due to [SLOP] verdict:** none
**Packages flagged as suspicious [SUS]:** none

## Architecture Patterns

### System Architecture Diagram

```
 Telegram updates ──────────────────────────────────────────────────────────────┐
   message (incl. service msgs), callback_query, my_chat_member, chat_member     │
                                    │                                            │
        ┌───────────────────────────▼──────────────────────────────┐             │
        │ gotgbot dispatcher (groups ascending; ContinueGroups =   │             │
        │ next handler in SAME group; nil = next group)            │             │
        └──┬─────────────┬──────────────┬─────────────────┬────────┘             │
           │ group -3    │ group -1/-2  │ group 0         │ callback `staff|`    │
           ▼             │ (existing:   ▼                 ▼                      │
  ┌─────────────────┐    │ botJoinedGrp │ /setstaff /unsetstaff /linkstaff       │
  │ ownership       │    │ may EndGroups│ /unlinkstaff /staff (WrapCommand)      │
  │ watcher:        │    │ for basic grp│ /start@bot stf_<id>  ──► help.go start │
  │ • chat_owner_*  │    │ )            │   (group branch)  ──► group deep-link  │
  │ • message.Migr. │    │              │                       registry         │
  │ • chat_member   │    │              │                                        │
  │   creator       │    │              │  Panel callbacks: Refresh, Unlink,     │
  │ • my_chat_member│    │              │  Confirm, Cancel, Page                 │
  └───────┬─────────┘    │              └───────────┬────────────────────────────┘
          │ hint only    │                          │
          ▼              │                          ▼
   ┌─────────────────────▼──────────────────────────────────────┐
   │ staff domain service (alita/modules/staff*.go)             │
   │  linkGroup(issuer, targetChat, staffID?)  → typed Result   │◄── hourly sweeper
   │  recheckLink(link) → ok | removed | health changed | skip  │    (Redis SetNX lock,
   │  rekey(old,new)                                            │     paced, recover)
   └────────┬───────────────────────────────┬───────────────────┘
            │ live reads                    │ writes (tx), rows-affected = winner
            ▼                               ▼
  chat_status.FetchOwnerID (uncached     db/staff repository ─► staff_groups
  getChatAdministrators), bot member      (cached gate reads +    staff_group_links
  status; errors ≠ mismatch               uncached authority      (UNIQUE/CHECK/trigger)
            │                             reads; DeleteCache
            ▼                             after commit)
  Telegram Bot API                               │
                                                 ▼
                              Staff Group notice (HTML-escaped titles,
                              Staff Group's language) — exactly once
```

### Recommended Project Structure
```
migrations/
└── 2026100x......_add_staff_groups_and_links.sql   # timestamp > 20261001120000
alita/
├── db/
│   ├── models/staff.go                 # StaffGroup, StaffGroupLink (+ TableName)
│   ├── staff/repository.go             # reads, tx writes, RekeyChat
│   ├── staff/testmain_test.go          # testdb.Run(m, &models.StaffGroup{}, &models.StaffGroupLink{})
│   └── cache/local.go                  # + skipLocal prefixes: staff_group:, staff_links:, staff_link_of:
├── modules/
│   ├── staff.go                        # module struct, Load, commands, callbacks, linkGroup
│   ├── staff_panel.go                  # pure render + live status builder
│   ├── staff_ownership.go              # group -3 watchers, recheckLink, sweeper Start/Stop
│   └── help.go                         # start: group branch -> group deep-link registry
├── utils/chat_status/owner.go          # FetchOwnerID, BotStatusIn (tri-state, uncached)
└── utils/helpers/command_pipeline.go   # + RejectAnonymousSender() CheckFunc
locales/{en,es,fr,hi,id,pt,ru}.yml      # staff_* keys; staff_help_msg
locales/config.yml                      # alt_names: Staff: [staff, setstaff, ...]
main.go                                 # postInit: StartStaffSweeper(b); shutdown handler StopStaffSweeper
AGENTS.md                               # group -3, alita:staff:* Redis key family, staff trap
```
Module registration: `RegisterLegacyModule("Staff", 236, LoadStaff)` (priority numbers 230/235/240 are taken by AntiRaid/Federations/Blacklists [VERIFIED: grep of `RegisterLegacyModule`]; no ordering dependency).

### Data model (migration + models; names must match `TableName()`)

Latest migration today: `20261001120000_text_filters_notes_without_file_id.sql` [VERIFIED: `ls migrations | tail`]. Use a larger timestamp. One transaction per file; no top-level `BEGIN/COMMIT`; no `CREATE INDEX CONCURRENTLY` (AGENTS.md). The runner supports dollar-quoted bodies (`splitSQLStatements` tracks `inDollarQuote`) and an existing migration already uses `DO $$` [VERIFIED: runner.go:233-310; 20260919120000_add_ai_spam_settings.sql]. Precedent for no FK on chat IDs: `federation_chats` has none [VERIFIED: 20260826000000_add_federations_and_log_channels.sql]. Recommend **no FK** (SQLite tests do not enforce FKs; behaviour must be identical in both engines).

| Table | Columns | Constraints |
|-------|---------|-------------|
| `staff_groups` | `id BIGSERIAL PK`, `chat_id BIGINT NOT NULL`, `owner_user_id BIGINT NOT NULL` (last verified owner, refreshed by rechecks), `title TEXT NOT NULL DEFAULT ''`, `created_at`, `updated_at` | `UNIQUE(chat_id)`; `INDEX(owner_user_id)` (D-09: many per owner, so **no** unique on owner) |
| `staff_group_links` | `id BIGSERIAL PK`, `group_chat_id BIGINT NOT NULL`, `staff_chat_id BIGINT NOT NULL`, `owner_user_id BIGINT NOT NULL` (the link maker, D-12), `group_title TEXT NOT NULL DEFAULT ''`, `health VARCHAR(16) NOT NULL DEFAULT 'ok'`, `created_at`, `updated_at` | `UNIQUE(group_chat_id)` (one Staff Group per group); `INDEX(staff_chat_id)`; `CHECK (group_chat_id <> staff_chat_id)`; `CHECK (health IN ('ok','bot_missing','bot_not_admin','bot_cannot_restrict'))` |
| D-10 exclusivity | PG-only: a `BEFORE INSERT OR UPDATE OF chat_id` trigger on `staff_groups` and `BEFORE INSERT OR UPDATE OF group_chat_id, staff_chat_id` on `staff_group_links`. Each takes `pg_advisory_xact_lock(chat_id)` first, then raises if the other table already holds that chat. | The advisory lock serialises per chat, and the check then runs in a fresh statement snapshot under READ COMMITTED, so concurrent `/setstaff` and link cannot both win `[ASSUMED]` A8. |

GORM model tags for the SQLite test schema (existing style `check:chk_name,expr`): `gorm:"column:group_chat_id;uniqueIndex;not null;check:chk_staff_link_distinct,group_chat_id <> staff_chat_id"` [VERIFIED style: alita/db/models/antiflood.go:8 `check:chk_antiflood_limit,flood_limit >= 0`]. Add the models to: `alita/modules/test_harness_test.go` AutoMigrate list, `alita/db/testmain_test.go`, and the new `alita/db/staff/testmain_test.go` (`testdb.Run(m, tables...)` prepends `User` and `Chat`) [VERIFIED: internal/testdb/database.go].

**Cache (AGENTS.md Data rules).** Keys via `cache.CacheKey(...)`: `staff_group:<chat>` (gate: is this a Staff Group; cache a zero-value sentinel for "no" as `GetChatFedContext` does), `staff_links:<staffChat>`, `staff_link_of:<groupChat>`. Authority reads (anything deciding ownership or an action) are **uncached** `...Fresh` functions. Every write calls `cache.DeleteCache` for all affected keys **after commit**, including both old and new IDs on a re-key. Add the three prefixes to `skipLocal` in `alita/db/cache/local.go` (today only `captchaPendingPrefix` is listed) so a second replica cannot serve a stale link for `CACHE_LOCAL_TTL` [VERIFIED: local.go `skipLocal`].

### Pattern 1: One link flow, typed results
**What:** `linkGroup(b, issuer, targetChat, staffID *int64) (Link, LinkError)` runs every D-03 check live. Both entry points (picker `/start@bot stf_<id>`, typed `/linkstaff [id]`) call it and only differ in how they render. Refusal reasons are an enum mapped to locale keys through a switch of **literal** `GetString("...")` calls (Pitfall 11): `not_owner_target`, `not_owner_staff`, `not_supergroup`, `target_is_staff_group`, `already_linked`, `not_a_staff_group`, `need_staff_id`, `no_staff_group`, `anonymous_sender`, `check_failed` (any API error: fail closed, "could not verify, try again").
**Order:** (1) reject anonymous / non-user sender; (2) `targetChat.Type == "supergroup"` from the update's own chat (live by construction); (3) live owner of target; (4) resolve staff: explicit id, else `ListStaffGroupsByOwner(issuer)` then verify each live; exactly one verified candidate or `need_staff_id`; (5) live owner of staff; (6) transaction: staff row exists, target not a Staff Group, `INSERT ... ON CONFLICT DO NOTHING` on `group_chat_id` with `RowsAffected == 0` meaning `already_linked`; (7) `DeleteCache` after commit; (8) best-effort bot-health snapshot into `health`, warning text if not ok (D-04); (9) post the confirmation to the Staff Group.
**Uniform refusal for non-owners:** when the issuer is not the live owner of the Staff Group they named, return one generic `not_owner_staff` that does not reveal whether that ID is a Staff Group (no oracle for probing IDs) and post it only in the issuing group as a self-deleting reply (Claude's Discretion item; Pitfall 4).

### Pattern 2: Tri-state live owner resolution
**What:** `chat_status.FetchOwnerID(b, chatID) (ownerID int64, err error)` calls `b.GetChatAdministratorsWithContext` (5-8 s context) and returns the user ID whose `GetStatus() == "creator"` (`ChatMemberOwner.GetStatus()` returns `ChatMemberStatusCreator` [VERIFIED: gen_types.go:2510-2511]). Callers distinguish three outcomes: **match**, **mismatch** (list fetched, creator absent or different), **unknown** (any error). Only *mismatch* may delete a link. *Unknown* leaves everything unchanged and is retried next cycle. `getChatAdministrators` bypasses `cache.LoadAdminCache` (30 min TTL), so it is the uncached call the repo lacks.
**Bot health:** `b.GetChatMember(chatID, b.Id, nil)` then `.MergeChatMember()`; statuses `administrator` with `CanRestrictMembers` give the four panel states. Map errors with the strings the Bot API server really emits: `Forbidden: bot was kicked from the supergroup chat`, `Forbidden: bot is not a member of the supergroup chat` [VERIFIED: Client.cpp:8846-8861], `Bad Request: chat not found` [VERIFIED: Client.cpp:7266] to `bot_missing`; anything else to *unknown*.

### Pattern 3: Rows-affected decides the winner (exactly-once, multi-replica)
**What:** every automatic removal or health change is one conditional statement: `DELETE FROM staff_group_links WHERE id = ? AND owner_user_id = ?` (winner is the caller with `RowsAffected == 1`), `UPDATE staff_group_links SET health = ? WHERE id = ? AND health <> ?`. Only the winner posts the Staff Group notice. This makes the event watcher, the sweep, the panel builder and a second replica race-safe without any Redis dependency (Redis only prevents duplicate API work). `/unsetstaff` is one transaction: delete links by `staff_chat_id`, then the `staff_groups` row; collect the removed links first for the notice.
**Never** `Updates(struct)` for the health or owner fields (zero values skipped): use `UpdateRecordWithZeroValues` or an explicit `map[string]any` (AGENTS.md Data).

### Pattern 4: Four triggers, one `recheckLink`
1. **Service messages** (group `-3`): `handlers.NewMessage(func(m *gotgbot.Message) bool { return message.ChatOwnerChanged(m) || message.ChatOwnerLeft(m) }, h).SetAllowBot(true)`. Look up the chat as a Staff Group and as a linked group; run `recheckStaffGroup(staffID)` / `recheckLink`. Return `ext.ContinueGroups`.
2. **`chat_member`** (group `-3`): `handlers.NewChatMember(creatorTransition, h)`, filter = old or new `MergeChatMember().Status == "creator"` and the chat is in the staff tables. Same recheck. `ContinueGroups`.
3. **`my_chat_member`** (group `-3`): bot status change in a chat that is a linked group, recheck health only (D-14 single heads-up through the `health <>` transition).
4. **Hourly sweep** and 5. **panel open / Refresh**: same function.
`recheckStaffGroup(staffID)`: resolve the Staff Group's owner once (`FetchOwnerID`), update `staff_groups.owner_user_id` if it changed (so `/linkstaff` candidates by recorded owner stay useful), and delete every link whose maker is no longer that owner. One combined notice listing all removed groups. `recheckLink(link)`: fetch group owner and (cached for the pass) staff owner; both must equal `link.owner_user_id` (D-12); any mismatch deletes; unknown skips.
Never trust the service-message payload as the verdict. Use it as a trigger and as the hint for the new recorded owner.

### Pattern 5: Group deep-link route for the picker
Add `RegisterGroupDeepLinkHandler(prefix string, handler DeepLinkHandler)` beside the existing registry in `deeplink_router.go`, and change the non-private branch of `start` in `help.go` to: if `len(args) == 2` and a registered group prefix matches, call it; otherwise keep the current `help_pm_questions` reply. The existing test `TestStartCommandRepliesInPrivateAndGroup` pins the old group behaviour for the no-payload case. The payload is `stf_` + decimal `-staffChatID` (e.g. `stf_1001234567890`), at most 20 characters, charset-safe. Parse strictly (`strconv.ParseInt`, reject anything else), no expiry needed (it grants no authority). The "Add group" button is a **URL button** `https://t.me/<bot>?startgroup=stf_<id>&admin=restrict_members+delete_messages`; URL buttons cannot be restricted per presser, so authority is enforced when `/start@bot` arrives (D-17 satisfied by the live checks).
Delete the command message best-effort first (`msg.Delete(b, nil)`), ignore the error (D-07), then run `linkGroup`.

### Pattern 6: Panel = pure render + live builder
`renderStaffPanel(tr, staff StaffGroup, rows []LinkStatus, page int) (text string, kb gotgbot.InlineKeyboardMarkup)` has no I/O, so Phase 9's menu page is a registration. The live builder (`buildLinkStatuses`) uses `errgroup.SetLimit(4)` and 2 calls per link (owner via administrators, bot via `getChatMember`) plus one staff-owner call per pass: `2N+1` calls, at most about 21 for `N < 10`. It also runs `recheckLink` semantics, so an owner mismatch found here deletes the link and posts the D-13 notice (D-18). Group titles come from the link row's `group_title` snapshot (refreshed when the live data differs), HTML-escaped with `html.EscapeString`. Page size 8 rows with Prev/Next buttons (D-19); keep the rendered text under about 3800 characters. Refresh uses `query.Message.EditText`; a "message is not modified" 400 is success (include an "Updated" time line so edits normally differ). Re-verify that the callback's message chat is a registered Staff Group and that `query.From` is a current Staff Group member (`getChatMember` status in member/administrator/creator, or restricted with `is_member`).
`/staff` outside a Staff Group: `return ext.EndGroups` with **no reply** (D-16: never reveals anything).

### Pattern 7: Hourly sweeper
Model on `StartAntiRaidExpiryPoller`: lifecycle `context.WithCancel`, a `sync.WaitGroup`, the goroutine starts with `defer error_handling.RecoverFromPanic("staffSweeper", "staff")`, `Stop` cancels then waits **without** holding the mutex [VERIFIED: antiraid.go:88-121]. Differences: it needs `*gotgbot.Bot`, so start it from `postInit` like `modules.StartCaptchaLifecycle(b)` [VERIFIED: main.go:415], and register a shutdown handler next to the others in `main.go` (registered after the DB-close handler, so LIFO runs it first; AGENTS.md Startup). Cycle: first run after a jittered 1-5 minutes (catches the 24 h update staleness), then hourly. Per cycle `SetNX("alita:staff:sweep:lock", instanceID, interval-1m)`; no Redis means run unguarded (idempotent by Pattern 3). Pace with `select { case <-ctx.Done(): return; case <-time.After(250 * time.Millisecond): }` between links. `alita:staff:*` is outside the cache prefix, so `AGENTS.md` must list it with `alita:antiraid:*` and `alita:anonAdmin:*`.

### Pattern 8: Chat migration
`RekeyChat(oldID, newID int64) (changed bool, err error)`: one transaction updating `staff_groups.chat_id`, `staff_group_links.staff_chat_id` and `group_chat_id` where equal to `oldID`; return `changed = false` and nil when nothing matched, so both service messages and the 400 detector are safe to deliver in any order and twice. After commit `DeleteCache` the three key families for **both** IDs. Handler: `handlers.NewMessage(message.Migrate, h).SetAllowBot(true)` at group `-3`, returning `ContinueGroups`; `migrate_to_chat_id` gives `(msg.Chat.Id, MigrateToChatId)`, `migrate_from_chat_id` gives `(MigrateFromChatId, msg.Chat.Id)`. Per-chat settings elsewhere (warns, filters, ...) are not migrated by this phase; that gap pre-exists.

### Pattern 9: Callbacks carry row IDs, authority comes from the row
Namespace `staff`, registered as `callbackquery.Prefix("staff|")` with the trailing pipe. No existing prefix is a prefix of `staff|`; existing registrations are `backup, about, anon_admin, antiraid, captcha_refresh, captcha_verify, change_language, configuration, connbtns, deleteMsg, filters_overwrite, formatting, helpq, join_request, notes.overwrite, reactions_help, report, restrict, rmAllApprovals, rmAllBlacklist, rmAllChatWarns, rmAllFilters, rmAllNotes, rmWarn, unbanall|, unpinallbtn, unrestrict` and `fedCallbackNamespace+"|"` [VERIFIED: grep of `callbackquery.Prefix(`]. Encode with `encodeCallbackData("staff", map[string]string{"a": "ul", "l": "<linkID>"})`: `staff|v1|a=ul&l=123456` is well under `MaxCallbackDataLen = 64` [VERIFIED: callbackcodec.go]. Actions: `rf` refresh, `ul` unlink-ask, `uc` unlink-confirm, `ux` cancel, `pg` page, `us`/`uy` unset-staff ask/confirm. The handler loads the link row, requires `query.Message` chat == `link.StaffChatID`, then live-checks `query.From` owns **both** groups. An unknown or deleted ID answers "expired" and re-renders; it never errors. Always `query.Answer`.

### Pattern 10: Anonymous senders
Recommended default: **refuse with "post as yourself"** for `/setstaff`, `/unsetstaff`, `/linkstaff`, `/unlinkstaff` and the picker route. Add `helpers.RejectAnonymousSender()` (checks `ctx.EffectiveSender.IsAnonymousAdmin()` and `User.Id` against `1087968824` / `777000`) as the first `RequiredChecks` entry; Phase 2 reuses it for STAFF-06. Rationale: `helpers.RequireUserOwner()` does not trigger the proof-button flow (only `hasUserPermission` / `IsUserAdmin` call `checkAnonAdmin` [VERIFIED: access.go:26, chat_status.go:63-79]), and `IsUserAdmin` returns true for `tgAdminList` IDs [VERIFIED: chat_status.go:166-190 `if slices.Contains(tgAdminList, userId) { return true }`], so reusing the proof flow would need extra plumbing for little benefit. Option B (reuse `RegisterAnonymousAdminHandler` + `anonPipelineHandler`) is feasible but is a larger change; it is the owner's call (Open Question 2).

### Anti-Patterns to Avoid
- **Trusting the recorded owner or the admin cache for any decision.** Recorded owner is only a lookup hint for `/linkstaff`.
- **Deleting a link on an API error.** Only a successful response with a different creator is a mismatch.
- **Posting refusals into a Staff Group the issuer does not own** (spam vector).
- **Dynamic locale keys** (`"staff_refuse_"+code`): the translation checker cannot see them.
- **Using `chat_member` as the only trigger.** Unverified delivery on transfers (A1); updates older than 24 h are dropped.
- **Embedding group titles unescaped in HTML notices.** Titles are attacker-controlled.
- **Adding staff tables to backup/export/import or `/reset`.** An imported file must never create or erase links (explicit module list in `alita/db/backup/types.go`; leave staff out).

### Suggested plan decomposition (planner may regroup)

| Wave | Content | Requirements |
|------|---------|--------------|
| 0 | Migration + models + `db/staff` repository + constraints tests; `staffBotClient` fake; test AutoMigrate lists; locale parity test skeleton; `skipLocal` prefixes | SETUP-04 (DB), D-10 |
| 1 | `chat_status.CheckOwner` / bot-status helpers; `helpers.RejectAnonymousSender`; `/setstaff`, `/unsetstaff` (+ confirm) | SETUP-01, SETUP-02 |
| 2 | `linkGroup` flow; `/linkstaff`, `/unlinkstaff`; group deep-link registry + `help.go` `start` branch; self-deleting refusals | SETUP-03, SETUP-04, SETUP-05 |
| 3 | Group `-3` watchers (service messages, `chat_member`, `my_chat_member`, migrate); `recheckLink` / `recheckStaffGroup`; `RekeyChat`; hourly sweeper + `main.go` start/stop | SETUP-06, SETUP-07 |
| 4 | Panel render + live builder + `staff|` callbacks (Refresh, Unlink, Confirm, Page, Add group URL button); `staff_help_msg`; all locale files; `locales/config.yml` alt_names; `make generate-docs`; `AGENTS.md` (group `-3`, `alita:staff:*`) | SETUP-08, PLAT-03 |
| End | Manual Telegram UAT (A1-A4) | SETUP-06, D-01 |

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Callback payloads | `strings.Split` / `fmt.Sprintf` | `callbackcodec.Encode/Decode` via `encodeCallbackData` / `decodeCallbackData` | 64-byte cap, versioning, url-encoding |
| Race-free "already linked" | check-then-insert in Go | `UNIQUE(group_chat_id)` + `clause.OnConflict{DoNothing: true}` + `RowsAffected` | Postgres arbitrates; existing repos use `clause.OnConflict` (`chats/repository.go:38`, `warns/repository.go:37`) |
| Exactly-once notice | Redis flag per notice | `DELETE/UPDATE ... WHERE` rows affected | Correct without Redis and across replicas |
| Owner resolution | cached `IsUserAdmin` / `LoadAdminCache` | new uncached `FetchOwnerID` (tri-state) | Cache is 30 min; bool helpers hide errors |
| Command registration | raw `dispatcher.AddHandler(handlers.NewCommand(...))` | `helpers.WrapCommand` with `xxxDesc` descriptors named for the docs parser | AGENTS.md; the docs generator regex needs `helpers.WrapCommand(dispatcher, xDesc, mod.handler)` [VERIFIED: scripts/generate_docs/parsers.go:279] |
| Per-chat language for background notices | new language lookup | `lang.GetLanguage(&ext.Context{EffectiveChat: &gotgbot.Chat{Id: staffChatID}})` | Precedent at `captcha.go:1321`; non-private branch reads only `chat.Id` [VERIFIED: lang/repository.go:54] |
| Background loops | bare `go func` | lifecycle context + WaitGroup + `error_handling.RecoverFromPanic` + shutdown handler | AGENTS.md Go rules |
| Redis lock | custom Lua | `rdb.SetNX(cache.Context, key, val, ttl)` | Existing precedent (federations export cooldown) |

**Key insight:** every cross-replica or cross-trigger guarantee in this phase is a database statement with a rows-affected check. Do not add coordination state that the database already provides.

## Common Pitfalls

### Pitfall 1: Unlinking on transient failure
**What goes wrong:** a 429, timeout or "chat not found" is treated as "not the owner" and a healthy link is deleted. **Why:** `RequireUserOwner` returns false for any error. **Avoid:** Pattern 2 tri-state; unit-test that a fake client returning an error leaves the link intact. **Warning sign:** `if !chat_status.RequireUserOwner(...) { delete }` anywhere in sweep or watcher code.

### Pitfall 2: Non-admin bot cannot read other users via `getChatMember`
**What goes wrong:** the hourly check silently never validates groups where the bot is not admin (the very case D-15.2 exists for). **Avoid:** resolve the owner with `getChatAdministrators`; if it also fails, report `unknown`, never `ok`. Verify in UAT (A2).

### Pitfall 3: Over-trusting the service message or `chat_member` payload
**What goes wrong:** a forged or reordered update changes authority. **Avoid:** payload only triggers a live recheck. Service messages cannot be forged by users, but `chat_member` for the *wrong chat* (not in the staff tables) must be ignored cheaply.

### Pitfall 4: Spam and probing through `/linkstaff <id>`
**What goes wrong:** anyone in any group can name someone else's Staff Group and cause visible output there, or learn which IDs are Staff Groups from different refusal texts. **Avoid:** post refusals only where the command was issued, self-deleting; give one generic message for "not a Staff Group" and "not yours".

### Pitfall 5: Command message not deleted, or deleted too late
**What goes wrong:** the picker's `/start@bot stf_<id>` stays visible in the linked group (D-07). **Avoid:** delete first, best-effort, ignore failure; never gate linking on delete success. Do not pre-check `CanBotDelete` (it uses the admin cache); just try.

### Pitfall 6: Confirmation cannot be delivered
**What goes wrong:** the bot was removed from the Staff Group after designation, so the notice fails and the owner sees nothing (D-06 forbids posting in the linked group). **Avoid:** log at warn; for typed commands, a short self-deleting reply in the issuing group is the fallback. Do not fail the link because the notice failed.

### Pitfall 7: `ContinueGroups` mental model
**What goes wrong:** copying ARCHITECTURE.md's reasoning leads to unnecessary gating code now and in Phase 2 (see C1). **Avoid:** cite dispatcher.go:280-326 in plans; keep watchers returning `ext.ContinueGroups`; keep commands returning `ext.EndGroups`.

### Pitfall 8: `RequireGroup()` accepts channels
**What goes wrong:** `RequireGroup` is `chat.Type != "private"` [VERIFIED: access.go:201 `return chat.Type != "private"`]; a command from a channel post or a forum topic passes it. **Avoid:** `/setstaff` must itself require `chat.Type` in `{"group","supergroup"}` and return the SETUP-01 channel refusal. Note `handlers.Command` ignores channel posts by default (`AllowChannel: false`), so the refusal matters mainly for typed commands forwarded oddly; still test it with a `Type: "channel"` context.

### Pitfall 9: Message handlers drop bot senders
**What goes wrong:** `chat_owner_*` and migrate service messages with a bot `from` never reach the handler (see Q3 wiring). **Avoid:** `.SetAllowBot(true)` on these watchers.

### Pitfall 10: Notices in the wrong language, or injected HTML
**What goes wrong:** background notices use the issuer's or English text; group titles break HTML or inject links. **Avoid:** translator from the **Staff Group's** language; `html.EscapeString` on every title and ID-adjacent string; cap title length (e.g. 64 runes).

### Pitfall 11: Locale completeness checks have blind spots
**What goes wrong:** `make check-translations` only inspects `X.GetString("literal")` calls with a string literal first argument [VERIFIED: scripts/check_translations/main.go:136-141 `lit, ok := call.Args[0].(*ast.BasicLit)`]. Keys built dynamically, or held in a map or switch value, pass the check and ship empty strings (`GetString` returns `""` plus an ignored error). **Avoid:** write each refusal key as a literal at a `GetString` call site, and add a parity test that reads `locales/*.yml` and asserts every `staff_*` key present in `en.yml` exists, non-empty, with identical `{placeholder}` sets in all seven files. In module tests the real locale manager is not initialised, so tests use `i18n.OverrideManagerForTest(yaml)` with marker strings, and a typo shows up as an empty message.

### Pitfall 12: Docs generator drift
**What goes wrong:** `make check-docs` fails when commands or help change without regeneration. **Avoid:** name descriptors `setStaffDesc`, `unsetStaffDesc`, `linkStaffDesc`, `unlinkStaffDesc`, `staffDesc`; register as `helpers.WrapCommand(dispatcher, setStaffDesc, staffModule.setStaff)` (no wrapper around the handler, or the regex misses it; `bans.go` wraps with `pipelineHandler(...)` and so is not picked up by that rule). Run `make generate-docs` and commit the output. Add `Staff: [...]` to `locales/config.yml` alt_names.

### Pitfall 13: Panel message too long
**What goes wrong:** one line per group with long localized text exceeds 4096 characters. **Avoid:** page size 8; compute length before send; unit-test with 40 fake links in every locale text set.

### Pitfall 14: `UpdateRecord` zero values
**What goes wrong:** writing `health='ok'` is fine, but anything writing `""`/`0`/`false` through `UpdateRecord` is silently skipped (AGENTS.md). **Avoid:** `UpdateRecordWithZeroValues` or conditional `UPDATE` through `db.DB.Model(...).Where(...).Update(...)` with rows-affected handling.

## Code Examples

### Group `-3` watcher registration (values verified)
```go
// Source: gotgbot ext/handlers/message.go (SetAllowBot), filters/message/message.go:210-216,272-282;
// dispatcher semantics: ext/dispatcher.go:294-296 (ContinueGroups = next handler in the same group).
dispatcher.AddHandlerToGroup(
    handlers.NewMessage(func(m *gotgbot.Message) bool {
        return message.ChatOwnerChanged(m) || message.ChatOwnerLeft(m) || message.Migrate(m)
    }, staffModule.onServiceMessage).SetAllowBot(true), -3)
dispatcher.AddHandlerToGroup(handlers.NewChatMember(creatorTransition, staffModule.onChatMember), -3)
// handlers return ext.ContinueGroups so groups -2, -1 and 0 still run.
```
Existing group registrations for context: `handlers.NewMyChatMember(chat_status.ExtractAdminUpdateStatusChange, adminCacheAutoUpdate), -2`, `botJoinedGroup ... -1, // process before all other handlers` [VERIFIED: bot_updates.go:230-242]. AGENTS.md group list to extend: `-10 captcha sweeper · -6 fed-ban · -5 antiraid · -2 admin-cache · -1 users tracker · 0 commands/help/greetings · 3 aispam · ...` [VERIFIED: AGENTS.md:29-31]; add `-3 staff ownership watcher`.

### Tri-state owner helper (shape)
```go
// ownerCheck: match / mismatch / unknown. Only mismatch may delete a link.
type OwnerResult int
const (OwnerMatch OwnerResult = iota; OwnerMismatch; OwnerUnknown)

func CheckOwner(b *gotgbot.Bot, chatID, wantUserID int64) (OwnerResult, int64) {
    ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
    defer cancel()
    admins, err := b.GetChatAdministratorsWithContext(ctx, chatID, nil)
    if err != nil {
        return OwnerUnknown, 0 // 429, timeout, kicked, chat not found: never a verdict
    }
    for _, a := range admins {
        if a.GetStatus() == gotgbot.ChatMemberStatusCreator {
            if a.GetUser().Id == wantUserID { return OwnerMatch, a.GetUser().Id }
            return OwnerMismatch, a.GetUser().Id
        }
    }
    return OwnerMismatch, 0 // list fetched, no creator: owner left (D-12)
}
```
`GetChatAdministratorsWithContext(ctx, chatId int64, opts)` [VERIFIED: gen_methods.go:2858]; `GetStatus()` / `GetUser()` on `ChatMember` [VERIFIED: gen_types.go:2309-2314 pattern for `ChatMemberAdministrator`; `ChatMemberOwner.GetStatus` returns `ChatMemberStatusCreator` at :2510-2511]. `[ASSUMED]` that `ChatMemberStatusCreator` is exported from `gen_consts.go` with the string `"creator"` (existing code compares the literal `"creator"`, access.go:192); the planner should compile-check.

### Link insert with race-free "already linked"
```go
// Source pattern: federations.go:120 db.DB.Transaction(func(tx *gorm.DB) error {...}),
// chats/repository.go:38 clause.OnConflict{ DoNothing: true }.
err := db.DB.Transaction(func(tx *gorm.DB) error {
    // 1) staff row must exist; 2) target must not be a Staff Group (portable check; PG trigger closes the race)
    res := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&link) // UNIQUE(group_chat_id)
    if res.Error != nil { return res.Error }
    if res.RowsAffected == 0 { return ErrAlreadyLinked }
    return nil
})
if err == nil { invalidateLinkKeys(link.GroupChatID, link.StaffChatID) } // DeleteCache after commit
```

### Group `/start` branch (the only edit to existing behaviour)
```go
// help.go start(), non-private branch (currently lines 373-381): add before the help_pm_questions reply
if len(args) == 2 {
    if handled, err := HandleGroupDeepLink(b, ctx, user, args[1]); handled {
        return err // staff handler returns ext.EndGroups
    }
}
// fall through to the existing `help_pm_questions` reply
```

### Hand-written Bot API fake with per-chat tables (Wave 0)
The existing `moduleBotClient` keys `getChatMember` by `user_id` only and returns a fixed administrators list [VERIFIED: test_harness_test.go: `if method == "getChatMember" && fmt.Sprint(params["user_id"]) == "777000"` returns `"creator"` for every chat; `getChatAdministrators` is a single canned response]. Phase 1 needs per-chat answers. Wrap it rather than editing it:
```go
type staffBotClient struct {
    *moduleBotClient
    creators map[int64]int64            // chatID -> creator userID (absent = no creator listed)
    botRole  map[int64]string           // chatID -> "administrator"/"restrict"/"left"/"kicked"
    failing  map[string]error           // "method:chatID" -> scripted TelegramError (429, 403, 400 migrate)
}
// override RequestWithContext: switch on method+chat_id, delegate everything else to the embedded client.
```
Assert behaviour (rows persisted, messages sent to the Staff Group and **not** to the linked group, calls not made), never literals or internals (AGENTS.md Testing).

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Ownership change only inferable from `chat_member` or polling | `chat_owner_changed` / `chat_owner_left` service messages | python-telegram-bot 22.7 (2026-03-16) added the types [CITED: docs.python-telegram-bot.org changelog, via search snippet]; present in Bot API 10.3 | Event-driven detection no longer needs the bot to be admin |
| Bot tracks a group upgrade only through `migrate_*` messages | Same, plus the 400 `migrate_to_chat_id` response parameter on dead chats | long-standing | A missed message is recoverable on the next API call |
| `startgroup` link only added the bot | Same link can request admin rights with `&admin=` | TDLib `LinkManager` | One tap adds the bot with the rights Phase 2 needs |

**Deprecated/outdated:** the milestone research statement "no dedicated ownership update exists" (see C-table).

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | Telegram emits `chat_member` for the old and new creator when ownership is transferred | Q1 | Low impact: service messages, sweep and live checks cover it. If false the `-3` `chat_member` watcher is dead code and may be dropped after UAT |
| A2 | `getChatAdministrators` works for a bot that is not an administrator and lists the creator (including an anonymous owner) | Pattern 2, Pitfall 2 | Links in groups where the bot is not admin cannot be re-verified until the bot is admin; they stay `unknown` (harmless in Phase 1, but stale until Phase 2 acts) |
| A3 | The official clients' group picker accepts a group where the bot is already a member and sends `/start@bot <payload>` there | Q2 | Add-group button fails for already-added groups; `/linkstaff` fallback covers it |
| A4 | A picker user with "remain anonymous" may arrive as an anonymous-admin message | Q2, Pattern 10 | Picker refused with "post as yourself"; `/linkstaff` also needs a non-anonymous post |
| A5 | `chat_owner_changed` / `chat_owner_left` reach a non-admin bot member as ordinary messages | Q1 | Detection falls back to the sweep for non-admin groups |
| A6 | Bot API version that introduced the ownership service messages is around 9.5 (March 2026) | State of the Art | None for planning |
| A7 | Arrival order of `migrate_to_chat_id` and `migrate_from_chat_id` is unspecified | Q3 | None: one idempotent `RekeyChat` handles both orders |
| A8 | A per-chat `pg_advisory_xact_lock` inside a BEFORE trigger makes the D-10 cross-table exclusivity race-free under READ COMMITTED | Data model | If wrong, a simultaneous `/setstaff` and link could both commit; mitigated by the in-transaction app check and the periodic sweep deleting a link whose group is also a Staff Group |
| A9 | "message is not modified" is a 400 error string that the existing code can match | Pattern 6 | Refresh button logs an error on unchanged content |
| A10 | The owner accepts a Postgres trigger for D-10 (versus app-level only) | Data model | Fall back to app-level check inside the link transaction |
| A11 | Anonymous owner default = refuse ("post as yourself") rather than reuse the proof button | Pattern 10 | Anonymous owners must post non-anonymously to set up links |
| A12 | The official clients combine, not replace, admin rights as TDLib's doc says | Q2 | Bot ends with fewer rights than expected; D-04 warning covers it |

## Open Questions

1. **D-11 basic-group Staff Group is unreachable.** `botJoinedGroup` leaves any basic group on join.
   - Known: `/setstaff` can never run in a basic group the bot has left; SETUP-07 re-key code is still required and unit-testable.
   - Unclear: whether the owner wants `/setstaff` to refuse basic groups with "upgrade to a supergroup first" (consistent, one line) or to keep silently accepting `group`.
   - Recommendation: accept `group` and `supergroup` in code (D-08 as written), but plan SETUP-07 verification as unit tests plus a handler-level test with synthetic `migrate_*` messages, not a live upgrade.
2. **Anonymous owners.** Refuse with "post as yourself" (recommended) or add the proof-button path? Decide before planning `RequiredChecks`.
3. **`chat_member` on transfer (A1).** Resolve in end-of-phase UAT with a throwaway supergroup; the plan must not block on it.
4. **D-10 trigger vs app-only.** Trigger adds a Postgres-only artefact that SQLite cannot exercise; the Postgres-gated test must be added to the `test-postgres-integrity` `-run` list in the Makefile. Confirm the owner accepts that.
5. **Recorded owner after a transfer.** Recommend updating `staff_groups.owner_user_id` from the live creator at every recheck; confirm that is intended (affects `/linkstaff` candidate lookup only).
6. **Self-deleting refusals** use `time.AfterFunc`; acceptable that a restart leaves a short refusal visible?

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go | everything | yes | go1.26.0 linux/amd64 | none |
| CGO + gcc | SQLite tests (`go-sqlite3`) | yes | `/usr/bin/gcc` | none |
| `go vet` | static check | yes | ran clean on `./alita/utils/helpers/ ./alita/modules/` | n/a |
| `make test` | AGENTS.md "only valid full test run" | **no (in this sandbox)** | fails: `go: no such tool "covdata"` for the test-less package `alita/utils/actionlog` under `-coverpkg`; `$(go env GOTOOLDIR)` holds only `asm cgo compile cover fix link preprofile vet`. All 59 packages with tests reported `ok`. | `go test -tags testtools -race -count=1 -timeout 10m ./...` (ran green in 48 s at baseline); run `make test` on a machine with a full toolchain |
| `make lint` | AGENTS.md | **no (in this sandbox)** | golangci-lint v2.5.0 built with go1.25.1 refuses the go 1.26.0 target | `go vet -tags testtools ./...`; run `make lint` in CI |
| `make check-translations` | PLAT-03 | yes | baseline: "All translations present" in all 7 locales | n/a |
| `make check-docs` | docs drift | yes | baseline: "No drift detected" | n/a |
| PostgreSQL server | trigger test, migration-chain test | no (`pg_isready`: no response; `psql` client installed; docker client installed, daemon not probed) | n/a | SQLite covers everything except the trigger; Postgres-gated tests run in CI (`ALITA_TEST_DATABASE=true` + `DATABASE_URL`) |
| Redis server | n/a | no | n/a | miniredis in tests (`withMiniredis(t)`) |
| goreleaser | `make build` | no | n/a | `CGO_ENABLED=0 go build ./...` |
| Telegram access / a throwaway supergroup | UAT for A1-A4 | not testable here | n/a | manual end-of-phase check (`human_verify_mode: end-of-phase`) |

**Missing dependencies with no fallback:** none for planning and implementation.
**Missing dependencies with fallback:** `make test` and `make lint` (use the direct commands above in this sandbox), PostgreSQL (CI).

## Validation Architecture

> `workflow.nyquist_validation` is `true` in `.planning/config.json` [VERIFIED: config.json], so this section applies.

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go `testing` + `testify` v1.12.1 (assertions only); real SQLite via `internal/testdb.Run` or a hand-rolled `TestMain`; miniredis for Redis; hand-written `gotgbot.BotClient` fakes. No mock libraries (AGENTS.md) |
| Config file | none; build tag `testtools` is mandatory (test-only helpers carry `//go:build testtools`, e.g. `i18n.OverrideManagerForTest`) |
| Quick run command | `go test -tags testtools -race -count=1 -run '^TestStaff' ./alita/db/staff ./alita/utils/chat_status ./alita/utils/helpers ./alita/modules` |
| Single package | `go test -tags testtools -race -count=1 ./alita/db/staff` (verified pattern: `go test -tags testtools -race -count=1 -run '^TestJoinFedRejectsNonOwner$\|^TestStartCommandRepliesInPrivateAndGroup$' ./alita/modules` returned `ok` in 18 s) |
| Full suite (dev/CI) | `make test` |
| Full suite (this sandbox) | `go test -tags testtools -race -count=1 -timeout 10m ./...` (green at baseline) |
| Phase gate extras | `make check-translations`, `make check-docs` (both green at baseline), `make lint` in CI |
| Postgres-only | `DATABASE_URL=... ALITA_TEST_DATABASE=true go test -tags testtools -race -count=1 -run '^TestStaffExclusivityTrigger' ./alita/db/staff`; migration chain `ALITA_TEST_MIGRATION_CHAIN=true DATABASE_URL=... go test -tags testtools -run '^TestRepositoryMigrationChain$' ./alita/db/migrations`; append new Postgres-gated test names to the `test-postgres-integrity` `-run` regex in the Makefile |
| Skip gate | `scripts/check_test_results` fails `make test` on any unexpected `t.Skip` except the listed Postgres skips; never add a plain `t.Skip` for missing Postgres in new tests without extending `postgresSkip` [VERIFIED: scripts/check_test_results/main.go] |

### Phase Requirements to Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| SETUP-01 | `/setstaff` by live creator, bot admin, supergroup: row persisted, cache keys deleted. Refusals each with its marker text: non-owner, channel `Type`, private, bot not admin, anonymous sender (zero Telegram write calls), group already linked (D-10), idempotent second call | module test with `staffBotClient` | `go test -tags testtools -race -count=1 -run '^TestSetStaff' ./alita/modules` | no, Wave 0 |
| SETUP-02 | `/unsetstaff` by live creator (not recorded owner) deletes row + links in one tx; admin or stranger refused; after an owner change the new live owner can remove it; confirm step | module test | `-run '^TestUnsetStaff' ./alita/modules` | no, Wave 0 |
| SETUP-03 | Picker route `/start@bot stf_<id>` and typed `/linkstaff`: link row has the maker's ID; confirmation posted only to the Staff Group; **no** send to the linked group; command message deleted when allowed; `stf_` payload with junk charset ignored | module test | `-run '^TestLinkStaff\|^TestStartGroupPayload' ./alita/modules` | no, Wave 0 |
| SETUP-04 | Refusal matrix: not owner of target, not owner of staff, target is a Staff Group, already linked (also under two concurrent calls: exactly one row), basic group, API error fails closed (no row), several Staff Groups needs id, non-owner naming someone else's Staff Group gets the uniform refusal and nothing is sent to that Staff Group | repo + module tests | `go test ... ./alita/db/staff -run '^TestCreateLink'` and `-run '^TestLinkStaffRefusals' ./alita/modules` | no, Wave 0 |
| SETUP-05 | `/unlinkstaff` and the Unlink button: only owner of both; confirm step; stale/deleted link ID answers "expired"; callback from the wrong chat refused | module test | `-run '^TestUnlinkStaff\|^TestStaffCallback' ./alita/modules` | no, Wave 0 |
| SETUP-06 | `chat_owner_changed` and `chat_owner_left` on a linked group and on the Staff Group remove the right links and post one notice; `chat_member` creator transition does the same; sweep removes mismatches; **API error leaves the link**; two concurrent rechecks post exactly one notice; sweep lock lets one of two runners proceed (miniredis); startup sweep runs; migrate-to and bot-removed keep the link (D-14) with one heads-up per health change | module + repo tests, miniredis | `-run '^TestStaffOwnership\|^TestStaffSweep' ./alita/modules` | no, Wave 0 |
| SETUP-07 | `RekeyChat` moves staff row + all links; idempotent; both orders of `migrate_to_chat_id` / `migrate_from_chat_id`; 400 `migrate_to_chat_id` param triggers re-key; cache keys of old and new IDs invalidated; a SQLite `CHECK` (`group <> staff`) still holds after re-key | repo + module tests | `go test ... ./alita/db/staff -run '^TestRekey'` and `-run '^TestStaffMigrate' ./alita/modules` | no, Wave 0 |
| SETUP-08 | Panel render (pure): each status combination, paging at 8, under 4096 chars with 40 links, escapes titles; `/staff` outside a Staff Group sends nothing; `/staff` shows help text and chat ID; Refresh edits in place; owner mismatch discovered while building removes the link and posts the notice; Refresh and Unlink authority re-checked per press | pure + module tests | `-run '^TestStaffPanel\|^TestStaffCommand' ./alita/modules` | no, Wave 0 |
| PLAT-03 | Every `staff_*` key present, non-empty, same placeholder set in `en es fr hi id pt ru`; no `GetString` with non-literal staff keys | parity test + checker | `go test -tags testtools -count=1 -run '^TestStaffLocaleKeys' ./alita/i18n` and `make check-translations` | no, Wave 0 |
| D-10 (DB) | CHECK and UNIQUE reject self-link and double link on SQLite; trigger rejects overlap under Postgres | repo test; Postgres-gated test | `-run '^TestStaffConstraints' ./alita/db/staff`; `-run '^TestStaffExclusivityTrigger'` with Postgres env | no, Wave 0 |
| Docs/help | commands appear in generated docs; no drift | generator | `make generate-docs && make check-docs` | n/a |

### Sampling Rate
- **Per task commit:** the package-level quick command for the touched package(s).
- **Per wave merge:** `go test -tags testtools -race -count=1 -timeout 10m ./...` plus `make check-translations`.
- **Phase gate:** `make test` on a full toolchain (or the sandbox equivalent), `make check-translations`, `make check-docs`, `make lint` in CI, then `/gsd-verify-work` including the manual Telegram UAT below.

### Wave 0 Gaps
- [ ] `alita/db/models/staff.go` and the migration (needed before any repository test compiles).
- [ ] `alita/db/staff/testmain_test.go` using `testdb.Run(m, &models.StaffGroup{}, &models.StaffGroupLink{})`.
- [ ] Add `&models.StaffGroup{}`, `&models.StaffGroupLink{}` to `alita/modules/test_harness_test.go` and `alita/db/testmain_test.go` AutoMigrate lists.
- [ ] `staffBotClient` fake (per-chat creators, bot roles, scripted `TelegramError` including `ResponseParams.MigrateToChatId` and 429), `//go:build testtools`.
- [ ] A staff-specific `i18n.OverrideManagerForTest` YAML with marker strings for every key.
- [ ] `TestStaffLocaleKeys` parity test (reads `locales/*.yml` with `yaml.v3`).
- [ ] Postgres-gated trigger test and its entry in the Makefile `test-postgres-integrity` regex.
- [ ] Framework install: none (testify, miniredis already present).

### Manual UAT (end-of-phase, real Telegram, cannot be automated)
1. Add-group picker with a group where the bot is already a member and already admin; confirm the bot's rights are not reduced and `/start@bot stf_...` arrives with the picker as sender (A3, A12).
2. Transfer ownership of a throwaway supergroup (bot admin), capture raw updates: did `chat_owner_changed` and/or `chat_member` arrive (A1, A5)?
3. Same transfer with the bot as a plain member (not admin): does `chat_owner_changed` still arrive, and does `getChatAdministrators` still list the creator (A2, A5)?
4. Owner with "remain anonymous" on runs the picker and `/setstaff` (A4).

## Security Domain

> `security_enforcement` is not set to `false` in config (absent = enabled); ASVS level 1 [VERIFIED: config.json `security_asvs_level: 1`, `security_block_on: "high"`].

### Applicable ASVS Categories
| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | no (identity is Telegram's) | n/a; reject non-user and anonymous senders explicitly |
| V3 Session Management | no | n/a |
| V4 Access Control | **yes** | live `getChatAdministrators` creator checks for both chats on every decision; callbacks re-authorize `query.From` and derive chat IDs from the DB row; no cache for authority reads |
| V5 Input Validation | yes | strict `strconv.ParseInt` for `/linkstaff <id>` and the `stf_` payload; callback decode through `callbackcodec`; reject non-negative chat IDs; `html.EscapeString` on titles |
| V6 Cryptography | no | no new secrets or crypto |
| V7 Error handling / logging | yes | never log payloads as secrets; no new secrets to `logredact.RegisterSecret`; log refusals at info, API errors at warn |
| V11 Business logic | yes | DB constraints + rows-affected arbitration against races; exactly-once notices |

### Known Threat Patterns for this stack
| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Forged `/start@bot stf_<id>` payload from a non-owner | Spoofing / Elevation | payload is untrusted; both owners verified live |
| Anonymous-admin or service-account sender passes owner/admin check (`1087968824`, `777000` pass `IsUserAdmin`) | Spoofing | `RejectAnonymousSender()` first; never call `IsUserAdmin` for these decisions |
| Stale link grants a former owner power | Elevation | D-12 live recheck on four triggers; unknown never counts as ok but never deletes either |
| Callback replay in another chat or after unlink | Tampering | callback carries a row ID; handler checks message chat == `link.StaffChatID` and live owner of both |
| Probing which chat IDs are Staff Groups | Information disclosure | uniform refusal text; refusals only in the issuing chat |
| Notice spam into a Staff Group by non-owners | DoS / Tampering | non-owner refusals never posted to the named Staff Group |
| HTML injection via group title in notices | Tampering | `html.EscapeString`, length cap |
| Check-then-insert race (two replicas, two commands) | Tampering | UNIQUE/CHECK constraints, `ON CONFLICT DO NOTHING`, PG trigger with advisory lock |
| Import/reset forging or erasing links | Tampering | staff tables are not part of backup/export/import/reset |
| Duplicate notices from several replicas | Repudiation noise | rows-affected winner posts |

## Project Constraints (from CLAUDE.md)

- Tech stack fixed: Go 1.26.0, gotgbot v2, GORM on PostgreSQL (SQLite in tests), Redis. No stack changes.
- Follow AGENTS.md: migrations append-only, timestamped, one transaction per file; every write `cache.DeleteCache`s affected keys; commands through `helpers.WrapCommand`; locale keys in all 7 locale files (actual set: `en es fr hi id pt ru`, see C4); fire-and-forget goroutines start with `defer error_handling.RecoverFromPanic(...)`; tests use real fixtures, no mock libraries, run through `make test`.
- Security: the per-group admin check is never skipped; new secrets use `logredact.RegisterSecret` (none in this phase).
- Telegram limits: callback data 64 bytes through `callbackcodec`; fan-out concerns apply to Phase 2, but the sweeper and panel builder here must still pace API calls.
- Deployment: Docker with `AUTO_MIGRATE=true`; never call `gorm.AutoMigrate` in production code.
- Value receivers on `moduleStruct`; handlers return `ext.EndGroups` / `ext.ContinueGroups`; never discard DB errors on state-changing paths; `errors.Wrap` style and `[Category][Function]` log prefixes.
- Work must start through a GSD command before file edits (GSD Workflow Enforcement).
- A new handler group or Redis key family updates `AGENTS.md` in the same commit (`-3`, `alita:staff:*`).
- Conventional Commits; user-visible changes use `feat:`.

## Sources

### Primary (HIGH confidence)
- Repository source read this session: `AGENTS.md`; `alita/modules/{bot_updates,help,deeplink_router,registry,anonymous_admin_router,federations,antiraid,callback_codec,test_harness_test,helpers_test}.go`; `alita/utils/chat_status/{access,chat_status}.go`; `alita/utils/helpers/command_pipeline.go`; `alita/utils/callbackcodec/callbackcodec.go`; `alita/db/{db,cache/local,cache/keys,cache/ttl,federations/repository,lang/repository,models/*}.go`; `alita/db/migrations/runner.go`; `internal/testdb/database.go`; `scripts/check_translations/main.go`, `scripts/check_test_results/main.go`, `scripts/generate_docs/parsers.go`; `main.go`; `alita/config/config.go`; `locales/`; `migrations/`; `Makefile`; `.planning/config.json`.
- gotgbot `v2.0.0-rc.36.0.20260919140833-240296efadb4` in the module cache: `ext/dispatcher.go` (`iterateOverHandlerGroups`), `ext/handlers/{command,message,chatmember}.go`, `ext/handlers/filters/message/message.go`, `gen_types.go`, `request.go`, `sender.go`, `gen_helpers.go`.
- Telegram Bot API server source (official implementation): `github.com/tdlib/telegram-bot-api` `telegram-bot-api/Client.cpp` (lines cited above).
- TDLib source: `github.com/tdlib/td` `td_api.tl`, `td/telegram/LinkManager.cpp`, `MessageContent.cpp`, `DialogParticipantManager.cpp`, `CHANGELOG.md`.
- Bot API 10.3 machine-readable spec mirror: `github.com/PaulSonOfLars/telegram-bot-api-spec` `api.json` (version "Bot API 10.3", August 24, 2026).
- Commands executed: `go test -tags testtools -race -count=1 ...` (targeted and full), `go vet`, `make check-translations`, `make check-docs`, `make test` (baseline failure analysed).

### Secondary (MEDIUM confidence)
- core.telegram.org/bots/faq and /bots/features, reached only through web-search snippets (the site is egress-blocked): service messages always delivered; privacy-mode delivery list; startgroup payload "up to 64 characters", `A-Za-z0-9_-`.
- docs.python-telegram-bot.org changelog snippet: `ChatOwnerChanged` / `ChatOwnerLeft` added in 22.7 (released 2026-03-16).
- `.planning/research/{ARCHITECTURE,PITFALLS,FEATURES,SUMMARY}.md`: reused; claims re-verified, with the corrections in the C-table.

### Tertiary (LOW confidence)
- Behaviour of official Telegram clients and servers not stated in any source (A1-A5, A12): to be confirmed in the manual UAT.

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH, no new dependencies; versions read from `go.mod`.
- Architecture: HIGH for how code attaches (read from source, tests run); MEDIUM for the Postgres trigger (A8, A10).
- Telegram behaviour: HIGH for API shapes and server mapping, MEDIUM for client picker semantics, LOW for `chat_member` on transfer.
- Pitfalls: HIGH, each tied to a read line or an executed command.

**Research date:** 2026-10-04
**Valid until:** about 30 days for code facts; the Bot API and gotgbot pin are fast-moving, so re-check `chat_owner_*` behaviour and the gotgbot version if planning slips past early November 2026.
