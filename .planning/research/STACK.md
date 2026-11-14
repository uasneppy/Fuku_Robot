# Technology Stack

**Project:** Fuku Robot, milestone v1.1 (PM Settings Hub and Member Screening)
**Researched:** 2026-11-02
**Scope:** only what the NEW v1.1 features need. Phases 1-9 (staff group, lockdown, raid detection, Turnstile, settings menu, Claude image client) are not re-researched.
**Overall confidence:** HIGH on the gotgbot surface and the repo seams (read from source); MEDIUM on Telegram runtime behaviour that the docs do not spell out (flagged per item, each needs a live spike).

## Headline

**No new Go dependency is needed.** Every v1.1 capability is already in `go.mod`:

- gotgbot `v2.0.0-rc.36.0.20260919140833-240296efadb4` (the pinned pseudo-version) already has every Telegram field and method listed below. I read them in the module source, not from memory. No gotgbot bump.
- Unicode script detection is the standard library `unicode` package.
- Scheduled deletion and the "greet once" ledger use PostgreSQL through GORM with append-only migrations.
- The PM "waiting for your next message" state reuses the repo's existing Redis `GETDEL` pattern (`fuku/modules/overwrite.go`).
- The Claude Haiku name check is a small extension of the stdlib-only `fuku/utils/claude` client.

What changes is new tables (migrations), new handler/worker code, one new handler group number, and a handful of new Redis key families.

## Recommended Stack

### Core framework (unchanged)

| Technology | Version | Purpose | Why |
|------------|---------|---------|-----|
| Go | 1.26.0 | everything | repo constraint |
| gotgbot v2 | rc.36 (`...20260919140833-240296efadb4`) | Telegram API | has all v1.1 fields/methods, verified below |
| GORM + PostgreSQL (SQLite in tests) | v1.31.2 / driver v1.6.3 | queues, settings, ledgers | durable, cross-replica, same test fixtures as Phase 4-6 |
| go-redis | v9.22.0 | wait state, verdict caches, latches, pacers | already the operational-state store |
| `fuku/utils/callbackcodec` | in repo | all new callbacks | 64-byte cap, `<ns>|v1|...` |
| `fuku/utils/ratelimit.TelegramPacer` | in repo | pace deletes and requirement checks | fleet-wide, Redis-backed, exists |

### Telegram Bot API surface (verified against rc.36 source)

All rows: confidence HIGH that gotgbot exposes it (read from `gen_types.go` / `gen_methods.go` in the module cache). Runtime-behaviour caveats are in the Notes column and in PITFALLS.

| Need | gotgbot rc.36 symbol | Notes |
|------|---------------------|-------|
| Linked channel of a group | `Bot.GetChat(chatId, nil)` returns `*ChatFullInfo`; field `LinkedChatId int64` | "discussion group identifier for a channel and vice versa". Only via `getChat` on the group; the `Chat` in an update does not carry it. Cache it (10 min, key via `cache.CacheKey("linked_chat", chat)`), invalidate on chat migration. 0 means no linked channel. |
| Subscription check | `Bot.GetChatMember(channelId, userId, nil)` returns `ChatMember` (interface; use `.MergeChatMember()` or `GetStatus()`) | Doc: "only guaranteed to work for other users if the bot is an administrator in the chat". So the bot must be an admin of the **channel**. Subscribed = creator, administrator, member, or restricted with `IsMember` true (`ChatMemberRestricted.IsMember` exists). left/kicked = not subscribed. Any lookup error (bot not in channel, "chat not found") = no information, fail open, and surface "bot is not an admin of the linked channel" in the menu (check `GetChatMember(channel, bot.Id)` once). MEDIUM on the exact error text for a non-admin bot. |
| Has profile photo | `Bot.GetUserProfilePhotos(userId, &GetUserProfilePhotosOpts{Limit: 1})` returns `*UserProfilePhotos{TotalCount int64, Photos [][]PhotoSize}` | `TotalCount == 0` means "no photo OR hidden from the bot by the user's privacy". Telegram does not document the privacy case; community reports say a hidden photo returns empty. LOW-MEDIUM: run one live spike with a test account that hides its photo. Treat it as a known false-positive class, say so in the menu text, and let "I fixed it" re-check. |
| Last name / username present | `gotgbot.User.LastName`, `.Username` (from the update's `From`) | Free, no API call, so these two rules can run on every message at zero cost. |
| Batch delete | `Bot.DeleteMessages(chatId, []int64, nil)` | 1-100 ids per call; doc: "If some of the specified messages can't be found, they are skipped". Same limits as `deleteMessage`: 48 h, no topic/supergroup-creation service messages, bot needs admin / `can_delete_messages`. Behaviour of a batch that contains an undeletable (too old) id is not documented: LOW. Plan: on a batch error that is not 429/rights, fall back to per-id `DeleteMessage` to isolate the bad row. |
| Single delete | `Bot.DeleteMessage(chatId, messageId, nil)` | Doc also says: bots can delete incoming messages in private chats, so the PM editor can delete the user's input message to keep the PM tidy. |
| Link preview below text | `LinkPreviewOptions{IsDisabled, Url, PreferSmallMedia, PreferLargeMedia, ShowAboveText}` on `SendMessageOpts` and `EditMessageTextOpts` | `ShowAboveText:false` (the zero value) puts the preview below the text. **`Url` must be a URL**, so this only works for media the owner gives as a link, never for a `file_id`. |
| Caption above/below media | `ShowCaptionAboveMedia bool` on `SendPhotoOpts`, `SendVideoOpts`, `SendAnimationOpts`, `EditMessageCaptionOpts`, `CopyMessageOpts` | The native way to render "text above, media below" for an uploaded photo/video/GIF. Caption limit is 1024 characters after entity parsing (text messages: 4096). |
| Copy a message | `Bot.CopyMessage(chatId, fromChatId, messageId, *CopyMessageOpts)` | Exists, but needs the source message to still exist. Not recommended for storage (see below). |
| Keep formatting from a PM message | `Message.OriginalHTML()`, `OriginalTextHTML()`, `OriginalCaptionHTML()` (`formatting.go`) | Converts the user's text plus entities back to HTML. Store HTML, send with `ParseMode: "HTML"`. |
| Edited messages | `Update.EditedMessage *Message`; `handlers.NewMessage(filter, h).SetAllowEdited(true)` | `edited_message` is already in `config.AllowedUpdates`. Handlers ignore edits unless `SetAllowEdited(true)`. Doc warning (verbatim in source): the update "may at times be triggered by changes to message fields that are either unavailable or not actively used by your bot". |
| Join/leave detection | `Update.ChatMember` (`ChatMemberUpdated`, with `InviteLink`, `ViaJoinRequest`, `ViaChatFolderInviteLink`), plus `Message.NewChatMembers` | Both already handled by the lockdown guard (group -7); dedupe through the existing join-claim helpers. |

### Service message fields in rc.36 (all present on `gotgbot.Message`)

One table-driven `serviceMessageKind(msg) (key string, ok bool)` should map these to settings keys. Never test with `msg.Text == ""`; test the typed field.

| Menu type | Field on `Message` | Type |
|-----------|-------------------|------|
| User joined | `NewChatMembers` | `[]User` |
| User left | `LeftChatMember` | `*User` |
| New group photo | `NewChatPhoto` | `[]PhotoSize` |
| Group photo removed | `DeleteChatPhoto` | `bool` |
| New title | `NewChatTitle` | `string` |
| Pinned message | `PinnedMessage` | `MaybeInaccessibleMessage` |
| Auto-delete timer changed | `MessageAutoDeleteTimerChanged` | pointer |
| Forum topic created / edited / closed / reopened | `ForumTopicCreated`, `ForumTopicEdited`, `ForumTopicClosed`, `ForumTopicReopened` | pointers. **Telegram cannot delete the "topic created" service message** (doc: supergroup/channel/forum-topic creation messages can't be deleted); do not offer it. |
| General topic hidden / unhidden | `GeneralForumTopicHidden`, `GeneralForumTopicUnhidden` | pointers |
| Boost added | `BoostAdded` | `*ChatBoostAdded` (the separate `chat_boost` update is not needed and is not in `AllowedUpdates`) |
| Video chat | `VideoChatScheduled`, `VideoChatStarted`, `VideoChatEnded`, `VideoChatParticipantsInvited` | pointers (invites = `VideoChatParticipantsInvited`) |
| Chat background | `ChatBackgroundSet` | pointer |
| Gifts | `Gift`, `UniqueGift`, `GiftUpgradeSent` | pointers |
| Giveaways | `GiveawayCreated`, `Giveaway`, `GiveawayWinners`, `GiveawayCompleted` | pointers (`Giveaway` is user content, not strictly service; offer under "giveaways") |
| Owner changes | `ChatOwnerLeft`, `ChatOwnerChanged` | new in this Bot API, pointers |
| Group/supergroup created, migration | `GroupChatCreated`, `SupergroupChatCreated`, `MigrateToChatId`, `MigrateFromChatId` | **never delete** (creation can't be deleted; migration is handled by `staff.RekeyChat`) |
| Payments / shared / misc | `SuccessfulPayment`, `RefundedPayment`, `UsersShared`, `ChatShared`, `WriteAccessAllowed`, `ProximityAlertTriggered`, `Checklist*` | offer one "other service messages" bucket |

Existing `greetings.clean_service_settings` covers join/leave only; v1.1 replaces it with the per-type table below and migrates the old flag into the join and leave rows.

### Database (new tables, one migration each, timestamped after `20261101120000`)

Follow AGENTS.md: append-only, one transaction per file, models with `TableName()`, add to every `AutoMigrate` harness list, reads via `cache.GetFromCacheOrLoad`, every write `cache.DeleteCache`. Design rule for every settings column: **the Go zero value must be the safe default** (off / 0 / owner-only), because GORM's `Create` turns a Go `false` back into a SQL default (the existing `chat_raid_detection` note). Use column-targeted `INSERT ... ON CONFLICT (chat_id) DO UPDATE` upserts like `raiddetection` so two admins never clobber each other.

| Table | Purpose | Notes |
|-------|---------|-------|
| `scheduled_deletions` | durable delete queue | `id`, `chat_id`, `message_id`, `delete_at`, `sent_at` (the message's own date, for the 48 h rule), `kind` (short string: `service`, `own_welcome`, `own_punish`, `command`, `edited`, `selfdestruct`...), `claimed_at`, `attempts`. `UNIQUE (chat_id, message_id)` with `ON CONFLICT DO UPDATE SET delete_at = LEAST(...)` so the earliest rule wins. Index `(delete_at)`. Rows are **deleted** on completion (no history), so the table stays small. Never cached, never in backup/export/import/reset. |
| `chat_cleanup_settings` | per-chat delete rules | one row per chat; seconds columns per service type (0 = off, so zero is safe) plus own-message minutes per kind, commands, edited, self-destruct minutes. Cached read via `GetFromCacheOrLoad`. |
| `chat_requirements` | member requirements | action columns as small ints `0 off, 1 warn, 2 kick, 3 ban, 4 mute` per rule (last name, username, photo, channel, AI name, Russian, Arabic, Chinese), plus `check_on_join`, `delete_messages`. Zero = off. |
| `chat_menu_access` | owner gate | `chat_id` PK, `admins_allowed bool` (zero = owner only). Read **fresh** (uncached) in the press pipeline: it is an authority input, and AGENTS.md already keeps authority reads off the in-process cache. |
| `chat_admin_index` | reverse index for the PM group list | `(chat_id, user_id, status)` with `status` creator/administrator, PK `(chat_id, user_id)`, index on `user_id`. The Bot API cannot list a user's groups and `fuku/utils/cache/adminCache.go` is keyed by chat only, so there is no reverse lookup today. Fill it from `loadAdminCacheFromTelegram` (one replace-per-chat transaction, in a recover-protected goroutine), from `chat_member` promotions/demotions, and clear it on `my_chat_member` when the bot leaves. The PM list reads this table only; every row is **live-verified** (`getChatMember`) when drawn or picked, and a stale row is pruned, so the index is a hint and never authority. Owner-only default means only `status = 'creator'` rows are listed. |
| `chat_welcomed_users` | "greet once" ledger | PK `(chat_id, user_id)`, `first_welcomed_at`. `INSERT ... ON CONFLICT DO NOTHING`; `RowsAffected == 1` means first time, so it is atomic across replicas and doubles as the dedupe for the two join deliveries. Postgres, not Redis: "ever" must survive a flush. |
| `greetings`, `rules` (columns added) | message editor | add media kind, `file_unique_id`, `media_below_text`, buttons (rules gets the same `ButtonArray` welcome already has), greet-once flag. Existing columns `file_id`, `btns`, `last_msg_id`, `clean_old` stay; "delete last welcome" is the existing `clean_old` + `last_msg_id`. |

### Scheduled deletion: PostgreSQL queue plus a DB-driven worker (recommended over Redis ZSET)

**Use a PostgreSQL table drained by a worker that copies the lockdown worker pattern** (`StartLockdownWorker`: `context` + cancel, `RecoverFromPanic`, `done` channel, bounded stop wait, wake channel).

Why PostgreSQL:

1. The requirement says deletions must survive restarts and work across replicas. A queue row in PG survives a Redis flush, `CLEAR_CACHE_ON_STARTUP` (which spares `fuku:*` operational keys, but not an operator `FLUSHDB` or a failover without persistence), and works without Redis at all, like the lockdown worker does.
2. The repo already has the proven shape: rows claimed with one conditional update, a stale-claim release, `RowsAffected == 1` as the winner test, and PG-integrity tests. SQLite test parity needs no `FOR UPDATE SKIP LOCKED`.
3. A ZSET would need a Lua pop-and-claim, a lost-claim recovery path and a second source of truth for "which messages are pending". More code for a weaker guarantee.

Claim shape that works on both PostgreSQL and SQLite and is safe across replicas (put the claim predicate in the **outer** `WHERE` so the loser re-evaluates it after the row lock):

```sql
UPDATE scheduled_deletions SET claimed_at = :now, attempts = attempts + 1
WHERE id IN (SELECT id FROM scheduled_deletions
             WHERE delete_at <= :now AND (claimed_at IS NULL OR claimed_at < :stale)
             ORDER BY delete_at LIMIT :n)
  AND delete_at <= :now AND (claimed_at IS NULL OR claimed_at < :stale)
RETURNING id, chat_id, message_id, sent_at;
```

(GORM: `Clauses(clause.Returning{})`; works on PG and SQLite 3.35+, which go-sqlite3 in `gorm.io/driver/sqlite` v1.6.0 ships.)

Worker rules:

- **One path for everything**, including "immediately": enqueue with `delete_at = now` and wake the worker through an in-process channel. Never call `DeleteMessage` inline on the update path. That gives batching (a join burst becomes one `deleteMessages`), pacing and 429 handling in one place, and avoids deleting a command message before the command's own reply has been sent (a reply to a deleted message fails). Schedule command deletions with a small minimum delay (about 2 s).
- Per cycle, group claimed rows by chat, send up to 100 ids per `DeleteMessages` call through a **new `ratelimit.NewTelegramPacer`** instance (`fuku:del:pace:next` / `fuku:del:pace:block`, own budget like `lockdownPacer` and `staffPaced`).
- Drop, without any Telegram call, rows whose `sent_at` is older than 47 h 50 m (48 h limit, small safety margin) and count them. After downtime this is the normal case for stale rows, so it must not loop. Cap every UI delay below 47 h (self-destruct minutes max 2820).
- Outcomes: success or "message to delete not found" / "can't be deleted" / bot kicked / chat gone / bot lacks the delete right means **delete the row** (no retry, log at debug, count). 429 or shutdown releases the claim with no attempt. Other errors: up to 3 attempts, then drop.
- Stale claim release (`claimed_at` older than 2 min) at the start of each cycle, like `lockdownStaleClaim`.
- Shutdown: register a `StopDeletionWorker` drain **after** DB-close with `RegisterHandlerWithTimeout` (5 s wait plus `shutdownDrainGrace`), exactly like `StopLockdownWorker`; every row resumes from the database, so a cut-off cycle loses nothing. Update the AGENTS.md shutdown paragraph in the same commit.
- Enqueue is one `INSERT ... ON CONFLICT` and is a hot path only for the self-destruct feature (one row per member message in chats that turned it on). Gate it on the cached `chat_cleanup_settings` read so chats without the feature cost nothing, and do not batch the writer until a measurement says so. A failed enqueue is logged and the message stays (fail safe).

Why not a library: River, asynq, machinery and similar bring their own schema/Redis layout and a new dependency tree, against the "append-only migrations, stay on the stack" rules.

### PM "waiting for your next message" state: reuse the existing Redis consume pattern

Use the repo's own mechanism (`fuku/modules/overwrite.go`: `setOverwriteCache`, `consumeOverwriteCache` with `rdb.GetDel`, `cache.GetMarshal()`), generalised with a new kind. It already survives replicas (Redis), expires on its own (5 min TTL there; use 10 min for an editor), sits outside the `fuku:cache:` prefix, and falls back to a mutex-guarded in-process path when no Redis client exists (tests, dev).

- Key `fuku:pmwait:<user id>`: one pending input per user (a new menu press replaces the old one). Value: chat ID being configured, the field (`rules_text`, `welcome_media`, `welcome_buttons`, `filter_add`, ...), the menu message ID to edit afterwards, and a version byte.
- Consume with atomic `GETDEL`, so when a user sends two messages quickly only one replica gets the state. On a validation failure (bad URL line, text over the limit), **write the state back** with the remaining TTL (the same shape as `captchaweb.Store.Rearm`) and reply with the reason. Cancel is a button (`a=xw`) that deletes the key, and `/cancel` or any other command clears it.
- Authority is checked **when the input is applied**, not only when the wait was armed: the consumer re-runs the live creator/admin check (and the owner gate) for the stored chat before writing anything, because Redis state is not proof of rights.
- The input handler is a watcher in the **private chat only**, in a free handler group (suggested group `1`: it comes after the group-0 command handlers, so `/commands` never become input, and group 1 is unused today). It returns `ext.ContinueGroups` when there is no pending state, and `ext.EndGroups` only when it consumed one. Verify no group-0 handler already catches every private message and returns `nil` (AGENTS.md: that silently kills later group-0 handlers). Add the new group number to the AGENTS.md handler list.
- Without Redis the editor says it is temporarily unavailable; the rest of the menu (stateless) keeps working.
- **Do not use gotgbot's `ext/handlers/conversation`**: its default storage is in-memory (per replica), it keys by chat/user in its own way, and its handler groups would not line up with the repo's group numbering. Writing the state in Redis directly is smaller and testable with miniredis.

The editor stores media as **`file_id` plus `file_unique_id` plus media kind** taken from the PM message (`Message.Photo[last].FileId`, `Video.FileId`, `Animation.FileId`, `Document`, `Sticker`), which is what the existing `greetings.file_id` and `type` columns already do. Do not store a source message to `copyMessage`: it breaks when the owner deletes the PM message, and it cannot carry the group's own buttons and settings. Albums arrive as separate updates (one per item, same `media_group_id`): take the first, ignore the rest, say "one media only".

"Media under text" has two honest implementations, pick by media source:

| Media source | Render | Limit |
|--------------|--------|-------|
| Uploaded photo / video / GIF (a `file_id`) | `sendPhoto` / `sendVideo` / `sendAnimation` with `Caption` = the text and `ShowCaptionAboveMedia: true` | caption 1024 chars; the editor must refuse longer text for this mode |
| An `https` URL given by the owner | `sendMessage` with `LinkPreviewOptions{Url: url, ShowAboveText: false, PreferLargeMedia: true}` | URL must be reachable by Telegram |

A `file_id` cannot be a link preview (no URL). Do not add media hosting or a public media route to the bot's HTTP server to fake it: a `getFile` URL contains the bot token (AGENTS.md forbids exposing it) and a new public route widens the attack surface the Phase 8 hardening just closed. If the owner insists on "link preview below text" for uploaded files, that is a separate decision (external hosting), not part of this milestone.

URL buttons: parse owner input with `net/url` (scheme `https`, `http` or `tg`), at most 8 per row and 100 per message (Telegram limits), and render with `gotgbot.InlineKeyboardButton{Text, Url}`. The existing `ButtonArray` / `db.Button` model already stores them for welcome. The preview in PM is the **real send function** of the greeting module called with the PM chat as target, so preview and group output cannot drift (the same "one code path" rule as the Phase 9 apply functions).

### Unicode script detection (standard library only)

`unicode.Cyrillic`, `unicode.Arabic`, `unicode.Han`, `unicode.Hiragana`, `unicode.Katakana`, `unicode.Hangul` are all in the standard library. I compiled and ran a probe on Go 1.26.0 to confirm the cases below.

| Rule | Implementation | Verified behaviour |
|------|---------------|--------------------|
| Arabic name | any rune with `unicode.Is(unicode.Arabic, r)` | matches basic Arabic and the Presentation Forms (`ﻣﺤﻤﺪ`), so Persian/Urdu/Pashto also match. Accepted: the menu says "Arabic script". |
| Chinese name | any `unicode.Han` rune **and** no Hiragana/Katakana/Hangul rune | `李雷` matches; `田中さん` does not (Japanese kana present). A Japanese name written only in kanji (`田中`) is indistinguishable from Chinese: accepted false positive, documented. |
| Russian name | any of `ы ъ э ё` (case-folded via `unicode.ToLower`), **and** the Cyrillic check | `Иван ёж` matches; `Олег`, `Олексій їжак`, `ҐЄ` do not. This is the owner's rule. Known false positives: Belarusian (ы ё э), Bulgarian (ъ), Kazakh and other Cyrillic languages. Ukrainian never uses these four letters, which suits a Ukrainian community. Optionally exempt a name that also holds an Ukrainian-only letter (`і ї є ґ`): a Russian name will not, so this only removes mixed-script oddities. |
| Which strings to test | `first_name` + `last_name` only (each at most 64 chars) | usernames are ASCII by Telegram rules, so script rules do not apply to them |
| Normalisation (for the AI cache key only, not for script rules) | `golang.org/x/text/unicode/norm` NFKC then `strings.ToLower` and whitespace collapse | `golang.org/x/text v0.42.0` is already a direct requirement in `go.mod` (used by `scripts/generate_docs`); using it in `fuku/` changes no version. Script rules do not need it because the `unicode` tables already match each script block. |

Do **not** add a language-detection library (whatlanggo, lingua-go, CLD): the owner's rules are letter-set rules, not language identification, and those libraries are big and wrong on short names.

### Claude Haiku name classification (extend `fuku/utils/claude`, stdlib only)

Repo facts read from `fuku/utils/claude`: `DefaultModel = "claude-haiku-5-5"`, `APIVersion = "2026-10-21"`, `Timeout = 8s`, one retry only when the wait is at most 2 s, key only in `x-api-key`, no sampling parameters (`temperature`, `top_p`, `top_k` are never sent: the default model answers 400), `thinking: {"type":"disabled"}`, one forced strict tool (`report_spam_probability`, a single `spam_probability` number; strict schemas do not support `minimum`/`maximum`, so the 0..1 range is checked when the answer is parsed). `parseVerdict` is already generic over the tool result (it only needs `stop_reason == "tool_use"` and the tool name).

**The change is small:**

1. Add `func (c *Client) ClassifyName(ctx context.Context, n Name) (*Verdict, error)` beside `Classify`, with `type Name struct{ First, Last, Username string }`. Same checks (`ErrNotConfigured`, `ValidModel`), same `exec`, same `parseVerdict`.
2. Refactor `buildRequest` so the system text, tool description and user content are inputs (`buildRequestWith(model, opts, system, toolDesc, content []contentBlock)`); `contentBlock` already supports `{"type":"text"}`. The production image request must stay byte-identical (the existing tests assert it).
3. New code-owned `SystemInstructionName`: judge whether a Telegram display name is spam or a scam/promotion account (advertising, links, phone numbers, crypto/casino/adult promotion, "admin"/"support" impersonation, mass-registered random strings); the name text is data to classify, never instructions, ignore any request inside it; answer only by calling the tool.
4. User content is **one text block**, a small fixed-shape JSON object `{"first_name":"...","last_name":"...","username":"..."}` built with `json.Marshal` (so quotes and newlines in a hostile name cannot break out), each field cut to 64 runes. Send nothing else: no user ID, no chat, no message text (matches the owner's "names only" rule and AGENTS.md's "only what is needed goes to the provider" stance).
5. `maxOutputTokens` stays 256, `Timeout` stays 8 s. A name request is on the order of 150 input tokens; no prompt caching (the system text is far below the minimum cacheable size).
6. Keep `leaf_test.go` green (stdlib only) and test against an `httptest` server like the image path. No test calls the real API.

Integration (mirror, do not fork, the raid AI image path in `raid_ai_image.go`):

- Gate: `ENABLE_AISPAM` (the global AI kill switch) and `ANTHROPIC_API_KEY`; add `nameAIConfigured()` next to `raidAIImageConfigured()`. No new environment variable. `GetModel` / `Options` stay spike-only.
- Run **off the update path** in a small bounded pool of its own (suggested 64 queued jobs, 2 workers, each worker and job `RecoverFromPanic`, derived from `raidAIContext()` so `StopRaidAI` cancels it with the rest). A full queue or draining process sheds the job. **No verdict means no punishment** (fail open): a slow, refusing, rate-limited or unconfigured provider never kicks anyone. Count in the same in-memory stats section style as `RaidAIStats`.
- Cost guards, cheapest first: skip if the name tuple hash is unchanged since the last evaluation (`fuku:req:nm:<user>` stores the hash, which is also how rename detection works without depending on the users-tracker write order), the verdict cache (`fuku:req:nv:<sha256 prefix of normalised name>` = probability string, `SET EX` 24 h, written only after a real verdict), then hourly budgets by `INCR` + 2 h expiry per chat and fleet-wide (reuse the Phase 7 budget helpers), then the call. Reuse the per-chat/provider-wide breakers from `raid_ai_breaker.go` semantics (key rejected pauses all chats; 429 with `retry-after` over 2 s pauses).
- Bar: a name verdict of at least `nameAIBar` (suggest 0.9, a code constant, higher than the image bar 0.85 because a false positive here punishes a real person) triggers the chat's configured punishment for the "AI spam name" rule. Never log or store the name text beyond the hashed key; the verdict cache holds only a number.
- Name sent to Anthropic also means a privacy note in the menu help text (like `lockdown_help_msg` does for TypeSafe/Anthropic) in all 7 locales.

### Member requirements engine (what the stack needs, not the full design)

- **Cheap rules first, no API call:** last name, username, the three script rules and the name hash all read the update's own `User`.
- **Telegram-calling rules** (photo, channel subscription) go through a **bounded concurrency gate plus `singleflight`** (`golang.org/x/sync` is already a dependency; `raidAIImageFlight` uses it) and a short pass cache `fuku:req:ok:<chat>:<user>` (about 10 min, positive results only). Each call has a 2-3 s timeout and an error is "no information" (fail open), like the raid counters. This is what keeps "check on every message" from becoming one `getUserProfilePhotos` per message.
- **One notice per violation burst:** a `SET NX EX 60` latch `fuku:req:latch:<chat>:<user>` so 20 messages from a violator do not produce 20 notices or 20 punishments across replicas (same design as `fuku:raid:{chat}:fire`).
- **Punishments reuse the existing code paths** (warn through `warns`, kick/ban/mute through the bans/mutes functions); no staff code is involved. Unmute for "I fixed it" goes through `resolveUnmutePermissions` (the choke point, so it also works during a lockdown).
- **"I fixed it" button:** callback `req|v1` through `callbackcodec`, fields `a=fx`, `u=<user>`; the handler requires `query.From.Id == u` (callback data is not authenticated), re-runs **all** rules live (bypassing the pass cache), and unmutes only when every rule passes. Needs its own `callbackquery.Prefix("req|")` registration.
- **Exemptions** (must not be screened): admins, approved users, the bot, Telegram service IDs (`staffServiceUserIDs`), and anything with `sender_chat` (anonymous admins, channels, the linked channel's automatic forward). Reuse `raidFloodSenderCounts` and `IsApprovedForUpdate` rather than a second list.
- **Handler placement:** a new watcher; the join check hooks the existing join paths, the on-message check is a watcher that always returns `ext.ContinueGroups`. Pick numbers from the free slots in AGENTS.md (the list has no `-5`, `-8`, `-9`, `1`) and update the list in the same commit.

### Message deletion handlers (what the stack needs)

- **Janitor watcher** (suggested group `-5`, free since the antiraid handler was retired): always `ext.ContinueGroups`; it only enqueues rows. It runs before group 0 so it sees a command that a group-0 handler will later end, and it enqueues service messages (via `serviceMessageKind`), member commands (slash text addressed to this bot or unaddressed, using the `bot_command` entity at offset 0 like `raidFloodIsSlashText`), and self-destruct rows. It runs after the lockdown join guard (-7), which stays first.
- **Edited-message deletion:** a second handler on `handlers.NewMessage(...).SetAllowEdited(true)` that returns immediately unless `ctx.EditedMessage != nil`, skips admins/approved/bots/`sender_chat`, and enqueues a delete with `kind=edited`. Because Telegram may send `edited_message` for changes the bot cannot see (doc text above), require `EditDate` to differ from `Date` by more than a small grace (suggest 5 s) and make the setting say "edited messages" not "text edits". LOW confidence on false triggers: needs a live check with a link preview and a reaction.
- **Bot's own messages after N minutes:** the send sites (welcome, goodbye, rules reply, automatic punishment notice, manual `/ban` `/mute` `/kick` `/warn` replies) each call one helper `scheduleOwnDeletion(chat, msg, kind)` right after a successful send; the helper reads the cached `chat_cleanup_settings` and enqueues. The message ID comes from the `*Message` the send returned. A sent message that is already older than 48 h at delete time is dropped by the worker.
- The bot needs `can_delete_messages`; show the existing bot-admin notice (`chat_status_bot_not_admin`) on the page, as `/lock` pages do.
- `setChatMessageAutoDeleteTime` does not exist in the Bot API; `ChatFullInfo.MessageAutoDeleteTime` is read-only. Telegram's own auto-delete timer cannot be set by the bot, so "self-destruct" must be implemented by the queue above.

### Settings hub (PM) additions

| Need | Choice | Why |
|------|--------|-----|
| PM `/settings` list | new `chat_admin_index` + live verify per drawn row (at most 8 per page, paced, the `staffPanelBuildTimeout` pattern) | Bot API cannot list a user's groups; the admin cache has no reverse index |
| Owner gate | `chat_menu_access.admins_allowed`; creator check = live `getChatMember` status `creator` (not `chat_status.CheckOwner`'s full `getChatAdministrators` unless the list is needed) | extends the Phase 9 `settingsPress.verify` step 4 with one fresh PK read; in-group `/settings` and the `settings_<chat>` deep link run the same function so the three entry points cannot drift |
| Pages | one `registerSettingsPage` call per page from its own file's `init` (warnings, reports, language, log channel, anon admin, locks, lists, requirements, deletion, editors) | the Phase 9 contract; no edits to `settings_pages.go` |
| Callback namespaces | reuse `settings|v1`; the editor/requirements/button handlers that need their own prefix use `req|v1`; the PM editor's picks carry row IDs or tokens, never text | 64-byte cap; AGENTS.md rule |
| Lists (blacklists/filters/notes) | offset-cursor paging (`o=`) like the staff history list; delete by row ID (`r=`), never by word; "add by typing" is a `fuku:pmwait` state | same hazards the history list already solved |
| Parity | every new setting gets one `apply*` function shared by command and menu, plus the `TestSettingsParity*`, `TestSettingsDrift*`, `TestSettingsAuthorityParity*`, worst-case callback-byte and 7-locale length tests | existing Phase 9 test contract |

New locale keys go in **all** locale files (`en es fr hi id pt ru uk`; the repo now ships `uk.yml` too, and AGENTS.md still says 7: confirm the current set with `make check-translations` before writing keys).

## Alternatives Considered

| Category | Recommended | Alternative | Why Not |
|----------|-------------|-------------|---------|
| Delete queue | PostgreSQL table + DB-driven worker | Redis ZSET + Lua pop | Loses the queue on a Redis flush/failover; needs a second recovery path; no DB-integrity test parity. Redis stays right for short-lived state, not for "must survive restarts". |
| Delete queue | home-grown table | River / asynq / machinery | New dependency tree and their own schemas; against the append-only migration and "stay on the stack" rules; the lockdown worker already proves the pattern. |
| Delete call | `DeleteMessages` (batch, up to 100) behind a pacer | `DeleteMessage` per row | Per-row calls multiply the request count in a raid or a busy self-destruct chat. Keep single delete only as the fallback that isolates a bad id. |
| PM wait state | Redis `GETDEL` (existing `overwrite.go` pattern) | gotgbot `conversation` handler | In-memory by default (per replica), own key strategy, group numbering clash. |
| PM wait state | Redis | PostgreSQL row per user | A DB read on every private message; no automatic expiry. Redis is already required for the other flows; accept "editor unavailable without Redis". |
| Media storage | `file_id` + `file_unique_id` + kind | `copyMessage` from the PM message | Breaks when the PM message is deleted; cannot attach the group's buttons; copy has the same 48 h/permission pitfalls. |
| "Media under text" | `ShowCaptionAboveMedia` (uploads) / `LinkPreviewOptions{ShowAboveText:false}` (URLs) | host the file and use a link preview | Needs public media hosting or exposing a bot-token `getFile` URL. |
| Name script rules | `unicode` range tables | language-detection library | Heavy, inaccurate on 2-word names, and the rules are letter-set rules anyway. |
| Name AI | extend `fuku/utils/claude` | Anthropic Go SDK | The repo keeps a stdlib-only, log-safe client with redaction rules and a spike tool; an SDK would break `leaf_test.go` and the key/URL hygiene guarantees. |
| Name AI | Haiku through the existing client | TypeSafe (`aispam`) | TypeSafe is a text-moderation call tuned to messages with chat context; the owner asked for Haiku and the key/kill switch/breakers already exist. |
| Channel check | `getChatMember` with a 10 min positive cache | `chat_member` updates on the channel | The bot only gets channel `chat_member` updates as a channel admin and only for changes while it is running; a live check is simpler and authoritative. |

## Installation

```bash
# No new modules. Core and supporting libraries are already in go.mod:
#   github.com/PaulSonOfLars/gotgbot/v2 v2.0.0-rc.36.0.20260919140833-240296efadb4
#   golang.org/x/text v0.42.0   (already a direct requirement; add unicode/norm imports only)
#   golang.org/x/sync v0.23.0   (singleflight)
CGO_ENABLED=0 go build ./...                   # compile check
make test && make lint && make check-translations && make check-docs
```

Non-code changes this milestone must land together (AGENTS.md says to update it in the same commit): new handler group numbers, the `StopDeletionWorker` drain in the shutdown paragraph, the new Redis key families (`fuku:pmwait:*`, `fuku:req:*`, `fuku:del:pace:*`), the new tables under "Data" (read fresh / never in backup/export/import/reset), the new `req|v1` namespace, and the `make test-postgres-integrity` list for the deletion claim (`TestScheduledDeletionClaim`: two claimers, one winner) and any new `CHECK` (the action columns `0..4`).

## What NOT to add

- No gotgbot bump, no Anthropic SDK, no queue library, no language-detection library, no cron library (a ticker loop like the lockdown worker is enough), no Telegraph/media hosting, no new public HTTP route, no web UI, no second cache layer.
- Do not use `gorm.AutoMigrate` outside test harnesses; do not edit any applied migration.
- Do not put a `default:` GORM tag on any new boolean (the Create zero-value trap).
- Do not call `getChatAdministrators` per row to draw the PM list (2N+1 calls is already the cap for the staff panel); one `getChatMember` per drawn row.

## Sources

Verified directly (HIGH):

- gotgbot module source, `v2.0.0-rc.36.0.20260919140833-240296efadb4`, in the Go module cache: `gen_types.go` (`Message`, `Update`, `ChatFullInfo.LinkedChatId`, `LinkPreviewOptions`, `UserProfilePhotos`, `ChatMemberRestricted.IsMember`, `MessageAutoDeleteTimerChanged`, `ChatMemberUpdated`), `gen_methods.go` (`DeleteMessage`, `DeleteMessages`, `GetUserProfilePhotos`, `GetChatMember`, `CopyMessage`, `SendPhoto/Video/Animation`, `EditMessageCaption`, `SendMessageOpts`, `EditMessageTextOpts`), `formatting.go` (`OriginalHTML` and caption variants), `ext/handlers/message.go` (`SetAllowEdited`), `ext/handlers/conversation` (in-memory storage). The method and field doc comments are generated from the Bot API spec and carry the 48 h rule, the 1-100 id batch, the "skipped if not found" rule, and the `edited_message` caveat.
- Repo: `fuku/utils/claude/{client,request,response}.go`, `fuku/modules/overwrite.go`, `fuku/modules/lockdown_worker.go`, `fuku/utils/ratelimit/telegram_pacer.go`, `fuku/utils/cache/adminCache.go`, `fuku/utils/chat_status/owner.go`, `fuku/db/models/{greetings,rules,admin,user}.go`, `fuku/config/config.go` (`AllowedUpdates`), `AGENTS.md`.
- Go 1.26.0 probe program (run locally) for the `unicode` script cases in the table above.

Not verifiable in this session (core.telegram.org is not reachable from the sandbox; web search returned only secondary sources):

- Profile photo privacy returning an empty list: community reports only (Habr Q&A, a Chinese Q&A page). LOW-MEDIUM.
- `getChatMember` on a channel requires the bot to be a channel admin: gotgbot's generated doc text says it is "only guaranteed to work for other users if the bot is an administrator in the chat"; exact failure text for a non-admin bot unconfirmed. MEDIUM.
- `deleteMessages` behaviour for a batch holding an undeletable (too old) id; spurious `edited_message` deliveries. LOW. Both need a live spike in phase research (owner step, like the Claude spike).
