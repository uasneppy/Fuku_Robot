# Pitfalls Research

**Domain:** Cross-group moderation fan-out, raid lockdown, Mini App (Cloudflare Turnstile) captcha, and inline settings menus, added to an existing Go/gotgbot Telegram group-management bot (Fuku Robot, fork of Alita Robot)
**Researched:** 2026-10-04
**Confidence:** MEDIUM-HIGH. Codebase claims were read in source (HIGH). Telegram API behavior was checked against a Bot API 10.3 spec mirror (MEDIUM-HIGH, see Sources). Items tagged `[UNVERIFIED]` come from general knowledge of Telegram behavior and need a throwaway-group spike before the roadmap relies on them.

**Phase labels used below** (owner's order): **P1** Staff Group, **P2** Raid lockdown, **P3** Turnstile web captcha, **P4** Settings menu. "P1-foundation" means shared primitives that P1 must build and that P2-P4 reuse (strict authorizer, paced fan-out executor, short-token store).

**How to read CONCERNS.md leads:** where this file touched one, it says VERIFIED, REFUTED, or UNVERIFIED. Do not plan from the rest.

---

## Critical Pitfalls

### Pitfall 1: Using `IsUserAdmin` / the admin cache as the per-group gate for Staff Group actions

**What goes wrong:**
The per-group check ("issuer must be an admin here with the right") passes when it should not. Four concrete holes exist in the current code:
1. `chat_status.IsUserAdmin` returns `true` for any chat when the user ID is in `tgAdminList` = `{1087968824 (GroupAnonymousBot), 777000 (Telegram service account)}` (`chat_status.go:36,182`). A Staff Group message from an anonymous admin, or an auto-forwarded linked-channel post, carries one of those IDs as the sender. Fed into a loop over linked groups, it is "admin" in every group.
2. `IsUserAdmin` checks admin *status* only. It never checks `can_restrict_members`. A "pin-only" admin passes.
3. The admin list is cached for `AdminCacheTTL = 30 min` (`constants/time.go`). It is invalidated only by `chat_member` updates, which Telegram delivers only while the bot is an admin there and the update is not lost. A demoted admin keeps cross-group power for up to 30 minutes after a missed update.
4. `getUserMemberWithCache` falls back to live `getChatMember` only on a cache miss, so a stale hit never refreshes.

**Why it happens:**
`IsUserAdmin` is the convenient, familiar predicate and every existing command uses it. In a single-chat command the "who is the sender" question is already answered by the pipeline (`WrapCommand`); in a fan-out the same predicate is called with a user ID that came from a *different* chat context.

**How to avoid:**
- Build one dedicated authorizer (P1-foundation), e.g. `staffauth.CanAct(ctx, groupID, issuerID, right) (Decision, reason)`. It must:
  - reject non-positive IDs, `1087968824`, `777000`, and bots before any lookup;
  - call live `getChatMember(group, issuer)` per target group, never the admin cache, for every fan-out (one extra call per group; accept it);
  - require `status == "creator"` OR (`status == "administrator"` AND the specific right, `can_restrict_members` for ban/mute/kick/unban/unmute); `ChatMemberOwner` has no `can_*` flags, so handle `creator` explicitly (the existing `hasUserPermission` does, copy that);
  - fail closed on any API error, returning a distinct "check failed" decision (reported as failed, not skipped).
- Do not call `IsUserAdmin`, `RequireUserAdmin` or `CheckFunc`s inside the fan-out. They reply to the chat and have the bypass list.
- Table-driven test: issuer x group x right matrix (owner, admin with right, admin without right, ex-admin, member, anonymous bot ID, 777000, left user) with a hand-written `gotgbot.BotClient` fake. Assert on which Telegram calls were made (no ban call for any denied row).

**Warning signs:**
- Fan-out code imports `chat_status.IsUserAdmin` or `cache.GetAdminCacheUser`.
- No test row for ID `1087968824`.
- Authorization check count per action is lower than the linked-group count (cached).

**Phase to address:** P1 (first plan; everything else depends on it).

---

### Pitfall 2: Issuer identity in the Staff Group (anonymous admins, channel posts, `AnonAdmin` setting)

**What goes wrong:**
A Staff Group "member" is resolved from `msg.From`, which is wrong for several sender types:
- Anonymous admin posts have `From = GroupAnonymousBot` and `SenderChat = the group`. PROJECT.md already says these cannot be authorised. The trap is the existing `AnonAdmin` per-chat setting: `checkAnonAdmin` returns "is admin" for *every* anonymous sender when `admin.GetAdminSettings(chat.Id).AnonAdmin` is on (`chat_status.go:67`). If someone enables it in the Staff Group, `WrapCommand` lets the command through with the bot ID as issuer.
- Posts as a channel (`SenderChat` = a channel) in a group with a linked channel arrive with `From = 777000`.
- A cached "has a user row" or "was seen in Staff Group" is not membership; the issuer may have left or been kicked.

**Why it happens:**
The anonymous-admin router (`RegisterAnonymousAdminHandler` / `anonPipelineHandler`) exists so admin commands work for anonymous admins. Copying the "register for anonymous admins too" step from the AGENTS.md "Adding a module" checklist is the natural reflex for a new admin-style command.

**How to avoid:**
- Staff commands must NOT call `RegisterAnonymousAdminHandler`. Reply once with a locale string ("post as yourself, not anonymously") when `Sender.IsAnonymousAdmin()` or `SenderChat != nil`.
- Resolve issuer = `ctx.EffectiveSender.User` only, require `Id > 0`, then verify live that the issuer is a current member of the Staff Group: `status` in {creator, administrator, member}, or `restricted` with `is_member == true`. Handle `left`/`kicked` as not a member (mirror `IsUserInChatWithError`).
- Enforce at link time and at `/staff` that the Staff Group has the `AnonAdmin` setting off, or ignore that setting in staff handlers explicitly.
- Warn the owner in `/staff` if the Staff Group is public (has a username) or has an open invite link. Membership in the Staff Group is the first factor of authority, even though the per-group check is the real safeguard.

**Warning signs:**
- Staff handler registered via `RegisterAnonymousAdminHandler`.
- Issuer taken from `msg.GetSender()` without a `User`/`Id > 0` check.
- No test for an anonymous-admin Staff Group message that expects zero Telegram write calls.

**Phase to address:** P1.

---

### Pitfall 3: Link validity: ownership checks are TOCTOU, not event-driven, and chat IDs change

**What goes wrong:**
"A link breaks automatically as soon as the same person no longer owns both groups" is not achievable by events alone:
- The Bot API has no dedicated "ownership transferred" update. At best a `chat_member` update fires (`[UNVERIFIED]` whether it fires reliably for creator changes), and only when the bot is admin and the update is in `allowed_updates` (it is: `config.go:339`).
- A link created once and trusted forever lets the *former* owner keep cross-group power, or lets a Staff Group keep control of a group it was sold/transferred away from. This breaks the Core Value directly.
- Ownership at link time can change between the check and the DB write (TOCTOU).
- Basic groups upgrade to supergroups and then get a NEW chat ID (`migrate_to_chat_id` / `migrate_from_chat_id`). `grep MigrateToChatId` across `alita/` returns nothing: the codebase does not handle migration at all. A link keyed on the old ID silently dies (fails closed, but the owner sees a "linked" row that does nothing). If the *Staff Group* migrates, every link is orphaned.
- "Owner" must be `status == "creator"` from `getChatMember`/`getChatAdministrators`. An owner who is `is_anonymous` is still visible to the bot as creator via these calls; do not use "has all rights" as a proxy.

**How to avoid:**
- Store the link with `owner_user_id` and treat it as a *claim to re-verify*, never a fact. At every fan-out, per group, verify live that `owner_user_id` is `creator` in BOTH the Staff Group and that group (two `getChatMember` calls, which can be the same call as the issuer check when issuer == owner; otherwise add them). On mismatch: skip with reason `link_broken`, mark the row broken (write + `cache.DeleteCache`), and notify the Staff Group log.
- Also run a periodic reverification sweep (bounded, paced) and re-verify on `/staff` and on `chat_member` creator-change events, as faster detection, not as the only mechanism.
- Link and unlink writes: single transaction, re-check ownership inside the handler immediately before the write, enforce "one Staff Group per group" with a UNIQUE index on the linked chat ID (not just app logic).
- Handle `migrate_to_chat_id` (service message, in both chat IDs) to re-key link rows and Staff Group designation in one transaction. This is a new capability, not an extension of existing code.
- Linked group must be a supergroup for mute/unmute: the spec text for `restrictChatMember` says "in a supergroup". Classify basic groups as `unsupported_chat_type` in the summary, not `failed`.

**Warning signs:**
- Link table has no `owner_user_id`, or fan-out never reads it.
- Only a `chat_member` handler maintains link validity.
- No test where the owner is demoted between link time and action time.

**Phase to address:** P1 (schema + verification contract); migration handling can be its own plan inside P1.

---

### Pitfall 4: Target resolution and target protection (`@username`, wrong chat types, admins, banned/non-member users)

**What goes wrong (resolution):**
`extraction.GetUserId` is the existing resolver and has traps that matter more with cross-group blast radius:
- It looks up `users` by exact, case-sensitive `username = ?` into a single scalar, on a *non-unique* index (`idx_users_user_name`). Telegram usernames are case-insensitive and are reassigned. When user A drops `@foo` and user B takes it, A's row keeps `@foo` until A is next seen. Two rows match and `Scan` returns an arbitrary one: you may ban the *previous* owner of a username in every group.
- On a DB miss it falls back to `getChat("@username")` and returns `chat.Id` blindly. For a channel or supergroup username this is a negative ID (e.g. `-100...`). Feeding that to `banChatMember` bans a chat-as-user or errors; `IsUserAdmin` has special handling for these only in some paths.
- Names shorter than 5 characters return 0, which is correct per Telegram, but the error path says "not found" without distinguishing "never seen" from "invalid".
- `ExtractUserAndText` returns `-1` for "not found" and `0` for "no target" - easy to confuse in new code.

**What goes wrong (protection):**
- The bot acts with its own authority, so an admin with `can_restrict_members` can use the bot to ban another admin, the owner, or a fellow Staff member, which Telegram would not let that admin do directly (regular admins cannot restrict peers). Existing per-group commands guard this via `validateTarget`; a new fan-out path will not inherit it.
- `unbanChatMember` defaults to `only_if_banned = false`: "guarantees that after the call the user is not a member of the chat" (spec). Calling it for `/kick` on a user who is *banned* in some group lifts that ban. `kickMember` in `bans.go` does exactly this. `/kick` fan-out must check per-group membership first and only kick current members.
- `restrictChatMember` on a user who is banned/left `[UNVERIFIED]` converts the record to "restricted" and effectively lifts the ban in the groups where the user was banned. `/mute` fan-out must skip groups where the target's status is `kicked`.
- `/unban` must use `only_if_banned = true`; plain `/unmute` must not "restore" permissions to someone who is actually banned.

**How to avoid:**
- Resolve in this order: numeric ID, `text_mention` entity, then `@username` via DB. Reject (do not guess) when more than one row matches, and make the lookup case-insensitive with a deterministic tie-break by most recent `updated_at`. Show the resolved numeric ID in the summary header so the issuer can confirm.
- Never use the `getChat("@name")` fallback for staff actions. Require `chat.Type == "private"` and `Id > 0` if it is used at all.
- Reject as `target_protected` per group: the bot itself, any bot user, the group's owner, any current admin there, and any user who is a member of the Staff Group. Check live `getChatMember(group, target)` once and derive all of: membership, banned state, admin state.
- Per-action matrix (verify each cell in a spike): ban = ban where status not `kicked`; mute = restrict only where status in {member, restricted(is_member)}; kick = unban(only_if_banned=false) only where currently a member; unban = unban(only_if_banned=true); unmute = restore only where `restricted`.
- Timed variants: `until_date` less than 30 s or more than 366 days from now means FOREVER (spec, for both ban and restrict). Reuse `extraction.TemporaryUntilDate`, which already refuses short values; never pass a raw clamp.

**Warning signs:**
- Fan-out calls `extraction.GetUserId` unchanged, or any path that accepts negative IDs.
- Summary rows only have `done/skipped/failed`, with no `target_protected` or `not_a_member`.
- `kickMember` reused directly in the loop.

**Phase to address:** P1.

---

### Pitfall 5: Fan-out with no pacing, no `retry_after`, and no partial-failure accounting

**What goes wrong:**
`applyActiveFban` (the model P1 is told to follow) is a single goroutine looping `BanChatMember` sequentially, with no `retry_after` handling and errors logged at `Debug` only (`federations.go:756-767`). Copying it gives: silent drops on 429, a summary that says "done" for a group that returned an error, and a dispatcher stalled by an in-handler loop.

**CONCERNS.md lead check:** the lead says `federations.go:735` "spawns goroutine per federation member" and "applyActiveFban spawns 10K goroutines". REFUTED in shape: it is one goroutine and a sequential loop. The real risk is the opposite, no retry/backoff and invisible failures. The lead's remedy ("cap at 10 parallel") would be a regression for correctness if it drops errors again.

Telegram facts: any call can return 429 with `parameters.retry_after`; documented broadcast guidance is ~30 msgs/s overall, ~1 msg/s to one chat, 20 msgs/min to one group (these concern messages; ban/restrict have no published numeric limit, treat them with the same 429 contract). Per action the Staff flow makes about 3-5 calls per group (issuer check, target check, the action, optional log-channel post, optional getChat for unmute permissions). With 20 linked groups that is ~100 calls, plus the bot's normal traffic.

**How to avoid:**
- Build a fan-out executor (P1-foundation) with: bounded concurrency across groups (start at 4-5), strictly sequential calls within a group, a process-wide token bucket shared with other hot paths, and a typed result per group: `done | skipped(reason) | failed(reason, retryable)`.
- Detect 429 via `errors.As(err, &*gotgbot.TelegramError)` then `ResponseParams.RetryAfter` (field confirmed in the vendored gotgbot). Sleep that long (cap, e.g. 30 s), retry at most 2 times, then record `failed(rate_limited)`. A 429 on one group should also pause the global bucket, not only that group.
- Classify errors into stable reasons (`bot_not_admin`, `bot_lacks_right`, `target_is_admin`, `chat_not_found`/bot kicked, `unsupported_chat_type`, `rate_limited`, `unknown(code)`) so the summary is meaningful.
- Run the loop in a goroutine that starts with `defer error_handling.RecoverFromPanic(...)` and joins a WaitGroup drained at shutdown (AGENTS.md Go rules); acknowledge immediately ("working on N groups") then edit one message with the final summary.
- Summary message: one line per group is ~80-120 bytes; 4096-character limit means ~30+ groups overflow. Paginate or compress ("N ok, N skipped" plus the non-ok lines).
- Idempotency: the issuer may re-run after a partial failure. All actions must be safe to repeat (ban, mute already-muted, unban only_if_banned).
- Persist a staff action record (who, target, action, reason, per-group outcome) in Postgres: the in-chat summary is not an audit trail, and Telegram messages get deleted.

**Warning signs:**
- Per-group error logging at `Debug`.
- No test with a fake client returning 429 with `retry_after`.
- The command handler's own goroutine does the whole loop (blocks the dispatcher; `CONCERNS.md` notes there is no circuit breaker either, UNVERIFIED).

**Phase to address:** P1-foundation. Reused by P2 ("Ban N recent joiners") and by P3 if the Mini App endpoint does Telegram writes.

---

### Pitfall 6: New staff commands collide with the existing per-group `/ban`, `/mute`, `/kick`, `/unban`, `/unmute`

**What goes wrong:**
The requirement reuses the exact names of commands that already exist and are registered in group `0`. In the Staff Group, the existing handler would act on the Staff Group itself (or reject, since the sender is not admin there). gotgbot stops a group at the first matching handler, so whichever registers first wins, and `RegisterLegacyModule` priority decides that. Also: if both fire in separate groups, an issuer is banned from the Staff Group and the fan-out happens (double action), or neither runs.

**How to avoid:**
- Decide and document the contract: inside a Staff Group chat, the five commands mean fan-out and NEVER act locally; in a linked group they remain local (an admin who is in both can still use the local one). Implement by registering the staff handler with a lower priority number (earlier) that returns `ext.EndGroups` only when the chat is a registered Staff Group and `ext.ContinueGroups` otherwise.
- Do not rely on module load order accidents; add a test that loads all modules in registration order and asserts which handler receives `/ban` in a Staff Group vs a linked group.
- Use `helpers.WrapCommand` with `Disableable: true` only where it makes sense. Note that a chat can disable commands, which would make staff commands dead in a Staff Group if disabled; decide whether staff commands are exempt.
- Avoid a lookup per update: "is this chat a Staff Group" is on the hot path of every command in every chat. Read through `cache.GetFromCacheOrLoad`; remember the 10 s local layer (see Pitfall 10).

**Warning signs:**
- Two `NewCommand("ban", ...)` registrations with no ordering test.
- `/ban` in the Staff Group answers "you are not an admin".

**Phase to address:** P1.

---

### Pitfall 7: A permission-level lockdown cannot exempt approved users, and the bot's own "unmute" helpers will freeze the lockdown into users

**What goes wrong:**
Two linked defects around `setChatPermissions`:

(a) **The approved-user exemption.** Per the spec, `setChatPermissions` sets *default permissions for all members* and `ChatPermissions` "describes actions that a non-administrator user is allowed to take". Admins are unaffected; "approved users" is a purely bot-side concept (a DB table), so Telegram cannot exempt them. The spec does not define how a per-user restriction interacts with a locked default (`[UNVERIFIED]`: the working assumption is that restrictions only narrow, so a per-user "can send" cannot override a locked default). That makes "mute everyone except admins and approved users" impossible with `setChatPermissions` alone. The per-user alternative (restrict every member individually) is O(members) calls and infeasible in a raid.

(b) **Contamination via `resolveUnmutePermissions`.** `/unmute`, the captcha pass path (`unmuteCaptchaUser`, `captcha.go:1390`) and the unban-restore path (`bans.go:817`) all copy the chat's CURRENT default permissions from `getChat` into a per-user restriction (`chat_permissions.go:47`). During a permission-level lockdown those defaults are the locked set. A user unmuted or captcha-passed during lockdown therefore receives a *per-user* locked restriction that does NOT go away when the lockdown lifts and defaults are restored. After lift they are still mute, with no obvious cause.

**How to avoid:**
- Make an explicit design decision (needs a spike, then a PROJECT.md Key Decision) between:
  1. default-permission lock, and accept that approved users are muted too (document it; simplest and most robust);
  2. default-permission lock, plus a per-user `restrictChatMember(can_send_messages=true)` for each approved user, but ONLY IF the spike shows that a per-user grant overrides a locked default (needs one call per approved user, paced through the fan-out executor); or
  3. no permission lock; bot-side enforcement that deletes messages from non-exempt users (needs `can_delete_messages`, rate-limited, racy, visible flicker).
  Recommended: option 1, upgraded to option 2 only if the spike passes. Verify in a real throwaway supergroup before committing the roadmap to the "except approved users" wording.
- Fix (b) regardless: make `resolveUnmutePermissions` lockdown-aware. When a lockdown is active for the chat, resolve unmute permissions from the stored pre-lockdown snapshot (Pitfall 8), not from `getChat`. Add a test: unmute during lockdown, lift, assert the user's effective permissions equal the pre-lockdown defaults.
- Audit every caller of `resolveUnmutePermissions` (`mute.go:228`, `bans.go:817`, `captcha.go:1396`) when adding the lockdown.

**Warning signs:**
- The lockdown plan says "restore previous permissions" but nobody can say which function reads them.
- Users report "still muted after lockdown ended".
- No spike result on file for default-vs-per-user permission interaction.

**Phase to address:** P2 (spike is the first task; fix to `resolveUnmutePermissions` is in the same phase).

---

### Pitfall 8: Restoring prior permissions: lossy snapshots, re-entrancy, and admin edits during lockdown

**What goes wrong:**
- Snapshot taken from `ChatFullInfo.Permissions` into a Go `gotgbot.ChatPermissions` and replayed later. Newer fields (`can_react_to_messages`, `can_edit_tag`, `can_manage_topics`, which "default to" other fields when omitted) can be lost or flipped by struct round-tripping, because `bool` zero values are indistinguishable from "omitted". Existing `MutedPermissions` and `defaultUnmutePermissions` already hand-roll these structs.
- A second trigger while a lockdown is active (auto-trigger plus manual `/lockdown`, or two replicas) snapshots the *already locked* permissions as "prior", so lift restores a locked state.
- An admin manually changes group permissions during lockdown; lift overwrites their change with a stale snapshot.
- `use_independent_chat_permissions` left at default (false) makes `can_send_other_messages` and `can_add_web_page_previews` imply the base send permissions, so a restore can silently grant more than before.

**How to avoid:**
- Store the raw JSON of `ChatFullInfo.Permissions` from a fresh `getChat` and replay it verbatim with `use_independent_chat_permissions=true` (also on the lock call).
- Take the snapshot with an atomic "insert if no active lockdown for chat" (unique partial index on `(chat_id) WHERE lifted_at IS NULL`); a second trigger becomes a no-op that returns the existing lockdown.
- At lift, compare the current permissions to the permissions the bot set. If they differ, someone edited manually: restore only if the admin confirms in the alert (or skip and report). Always report the outcome.
- Verify each Telegram call result before announcing state (Pitfall 11).

**Warning signs:**
- Lockdown row has a `prior_permissions` column typed as individual booleans.
- No test for double trigger.

**Phase to address:** P2.

---

### Pitfall 9: Lockdown state that lives only in Redis, and an `antiraid` module being rewritten while its name is still taken

**What goes wrong:**
The existing `antiraid` stores `alita:antiraid:state:<chat>` in Redis only and "does nothing without Redis" (AGENTS.md). Its expiry poller won't even start without Redis (`antiraid.go:89`). A lockdown that "never lifts on its own" but whose state is in Redis will, after a Redis restart without persistence, an eviction under memory pressure, or a flush, silently end *in the bot's mind* while the group stays locked by Telegram permissions. Or the reverse: the bot forgets it holds kicks to apply. Also the operational keys sit outside `alita:cache:` so `CLEAR_CACHE_ON_STARTUP` doesn't touch them, which gives false assurance.

PROJECT.md says "Raid protection replaces the behaviour of the existing antiraid". Reusing the module name, command names (`/antiraid`, `/raidtime`, ...) or the callback namespace `antiraid` with different semantics (temp-ban vs permanent lockdown) risks old buttons still in chats firing new behavior, and users running the old command expecting expiry.

**How to avoid:**
- Postgres is the source of truth for lockdown state (migration per AGENTS.md rules: append-only, timestamped, single transaction). Redis only accelerates reads and holds the join-rate counters.
- Reads go through `cache.GetFromCacheOrLoad`. Because `GetFromCacheOrLoad` has a 10 s in-process layer and `DeleteCache` only evicts the local layer on the writing replica, add the lockdown key prefix to `skipLocal` (`alita/db/cache/local.go`), exactly as `captchaPendingPrefix` is today. Every write `DeleteCache`s it.
- A lockdown with Redis down must still enforce (kick-on-join uses the DB state); only the *rate detection* degrades. Surface "DEGRADED_DETECTION" in the alert and logs (CONCERNS.md recommends this; VERIFIED as a real gap: `StartAntiRaidExpiryPoller` skips without Redis).
- Choose a new callback namespace and new command names for the lockdown, and decide what happens to the old `/antiraid`: keep it, alias it, or remove it with a migration notice. Do not leave both silently active for the same chat.
- Make the new handler group number explicit and update the AGENTS.md group-number list in the same commit (the file requires it).

**Warning signs:**
- A lockdown struct with `ExpiresAt`, or a Redis `SET ... EX` anywhere in the new code.
- Handler for lockdown reuses `callbackquery.Prefix("antiraid")`.

**Phase to address:** P2.

---

### Pitfall 10: Kick-on-join races: three entry paths, a fail-closed dedupe, and kick loops

**What goes wrong:**
- **Three entry paths for one join:** `chat_member` update (needs bot admin), the `new_chat_members` service message, and `chat_join_request` (for groups with approval). The existing `antiraid.onJoin` reads only `msg.NewChatMembers` (`antiraid.go:465`). Service messages can be hidden or absent in large groups and for some join types (`[UNVERIFIED]` threshold). A lockdown that listens only to service messages lets raiders through silently in big groups.
- **Double counting:** `trackJoin` uses a ZSET member of `userID:unix:nanos` (`antiraid.go:138`), so it is unique per call and never dedupes. A join delivered as both a `chat_member` update and a service message counts twice, halving the effective threshold and creating false-positive lockdowns.
- **Fail-closed dedupe:** `claimRecentJoinProcessing` returns `false` ("someone else already processed this join") on any Redis error. If lockdown kick logic is guarded by it, a Redis blip means *no kick* at the worst time. Kick is idempotent; it does not need a claim.
- **Join-request groups:** `greetings.pendingJoins` auto-approves when `ShouldAutoApprove` is set. If it runs before the lockdown watcher, the user is admitted and then needs a kick. In lockdown, decline the request (`declineChatJoinRequest`, needs `can_invite_users`) rather than approve-then-kick.
- **Kick loops:** "kick" via `unbanChatMember(only_if_banned=false)` frees the user to rejoin immediately via the public link. A bot-driven raid re-joins in a loop; each cycle costs the bot 2+ API calls and trips 429 on the bot itself, degrading the whole fan-out and normal moderation.
- **Side effects on doomed joiners:** the captcha flow (`SendCaptcha`: image generation, `restrictChatMember`, a DB `captcha_attempts` insert, the `alita:cache:captcha_pending:<chat>` flag) and the welcome message must not run for a user being kicked, otherwise you create orphan attempts, pending flags (AGENTS.md: any insert into `captcha_attempts` must `DeleteCache` the flag) and spam in the group.
- **Legit joiners:** a user added by an admin, an approved user, or another bot added by an admin are all "new joiners". `ChatMemberUpdated.from` is the performer; `from != user` with an admin performer is an add.

**How to avoid:**
- One `lockdown.OnJoin(chat, user, source)` entry called from all three paths, deduping *counting* by `(chat, user)` within the window (use the user ID as the ZSET member so re-adds overwrite), but never gating the *kick* on that dedupe.
- Register the lockdown join watcher before greetings, captcha and join-request auto-approve (handler group number lower than `0`, return `ext.ContinueGroups`; a group-0 watcher returning `nil` silently kills later group-0 handlers).
- During lockdown: join request -> decline; direct join -> ban with a short `until_date` (>= 30 s, spec) or ban and record for batch unban at lift, rather than plain unban-kick. Decide explicitly and record the list of kicked IDs in the DB (needed for "Ban N recent joiners" and for unban-at-lift).
- Skip (do not kick) approved users, current admins, the bot, and joins performed by an admin. Document this exception list in the alert text.
- Log every skipped kick with a reason at `Info`, so false negatives are debuggable.

**Warning signs:**
- Lockdown implemented as another `NewChatMembers` filter.
- Join count in tests equals 2x expected when both update types are fed.
- `claimRecentJoinProcessing` imported by the lockdown code.

**Phase to address:** P2.

---

### Pitfall 11: Announcing a lockdown that did not take effect, and false-positive lockdowns that never auto-lift

**What goes wrong:**
- The alert says "LOCKDOWN ACTIVE" while `setChatPermissions` failed (bot not admin, no `can_restrict_members`, group is a basic group) or a kick partially failed. Staff trust the banner and relax.
- Because a lockdown "never lifts on its own", a false positive costs the whole community until a human acts, perhaps hours at 3 a.m. Typical false-positive sources: counting raw join events instead of distinct users; a legitimate surge after a promotion; bots added by an admin; counting `new_chat_members` + `chat_member` twice (Pitfall 10); a low absolute threshold in a small group (5 joins/min is a surge in a 20-member group); AI-verdict bursts where the AI service is flaky.
- AI burst trigger measures *shedding*: the AI spam queue is bounded and sheds checks under load (AGENTS.md/CONCERNS.md: shedding is accepted behavior). During a real raid the verdict rate drops exactly when you need it, and a "burst of AI spam verdicts" trigger can under-fire. The existing `TYPESAFE_API_KEY` dependency means the AI trigger is inert without it.
- Message/media flood detection built on `antiflood` reuses an in-process, per-replica counter that is per `(chat, user)`, while raid detection needs a per-chat aggregate across users. Multi-replica deployments will each see a fraction.
- "Mostly from new members" needs a join timestamp per member; the users tracker (group `-1`) does not store one.

**How to avoid:**
- State machine for a lockdown row: `applying -> active | partial | failed`. Announce only what the API confirmed; the alert lists per-step results (permissions locked yes/no, joins being kicked yes/no, bot rights present yes/no).
- Count distinct users, set an absolute floor and a relative threshold by member count, add hysteresis/cooldown after a lift (no re-trigger for N minutes without a manual command), and cap triggers per chat per hour.
- Ship rule-based detection in **observe-only (shadow) mode first**: log "would have locked" with counters for a week of real traffic, then enable enforcement per chat. This is the cheapest protection against a never-auto-lifting false positive.
- Store `joined_at` for new members (Redis ZSET or a DB column) so "mostly new members" is computable.
- Treat AI as a score contributor, not a standalone trigger. Count shed events as a signal ("queue saturated") rather than as silence.
- Make manual `/lockdown` require `can_restrict_members` (not just any admin), because the cost of griefing is high.

**Warning signs:**
- Alert text built before the API calls return.
- Thresholds are single global constants.
- No shadow-mode log line anywhere.

**Phase to address:** P2.

---

### Pitfall 12: Lockdown alert buttons: authorizing against the wrong chat, forgeable callback data, stale buttons

**What goes wrong:**
- The existing `antiraid.callbackHandler` derives `chatID := msg.GetChat().Id` from the message carrying the button and authorizes with `IsUserAdmin(chatID, query.From.Id)` (`antiraid.go:814-816`). In the new design the same alert is posted to the Staff Group and to the group's log channel. Copying the pattern authorizes against the *Staff Group* (or the log channel), not the locked group, so the wrong people can lift (any Staff Group admin) or the right admins cannot.
- Callback data is client-supplied. The Bot API says of `data` that "the message originated the query can contain no callback buttons with this data". Treat `data` as an untrusted request, never as an authorization statement.
- Stale buttons: an old alert's "Lift" pressed during a *new* lockdown of the same chat lifts the new one. "Ban N recent joiners" pressed twice or after state changed bans a different set.
- A `chat_id` in the payload of the 64-byte `callbackcodec` format can fit (`ns|v1|c=-1001234567890&a=lift` is ~35 bytes) but `lockdown id` + `chat id` + action + nonce can overflow, and the codec wrapper then returns `""` (a dead button; see Pitfall 17).
- The message may be inaccessible for old alerts; `query.Message` is `MaybeInaccessibleMessage`.
- Button tokens held in Redis with a TTL break "never lifts on its own": the Lift button dies after the TTL.

**How to avoid:**
- Payload carries only a short lockdown ID + action (`ns|v1|l=<id>&a=lift`). The handler loads the lockdown from Postgres, takes `chat_id` from that row, and then runs the **per-chat live check for `query.From.Id`** against that chat (admin for lift; admin with `can_restrict_members` for ban). Never read `chat_id` from the message or the payload.
- Bind the action to the lockdown row: lift only if `lockdown.id == payload.id AND lifted_at IS NULL`; ban-joiners only for the stored joiner IDs of that lockdown, excluding users approved/promoted since.
- Make each action idempotent and answer the callback in every branch (`AnswerCallbackQuery`), including denials.
- Provide `/lockdown off` as a command fallback so a dead button never traps a group.
- Banning joiners: `banChatMember` on supergroups always sets `revoke_messages` true (spec), so it deletes the user's history irreversibly; add a confirm step for N above a small number, and cap N.

**Warning signs:**
- The handler references `query.Message.GetChat()` for any authorization decision.
- Callback payload contains a user-facing or user-supplied string.
- No test where the presser is admin in the Staff Group but not in the locked group (must be denied).

**Phase to address:** P2.

---

### Pitfall 13: Mini App launch plumbing: the wrong button type, no `initData`, wrong binding

**What goes wrong:**
- The inline `web_app` button is **"Available only in private chats between a user and the bot"** (Bot API 10.3, `InlineKeyboardButton.web_app`). The current captcha message is posted in the group; a `web_app` button there will be rejected or not render. Teams discover this late.
- `initData` is only populated for launch modes that carry it (keyboard/inline button in PM, menu button, main Mini App, direct link). Opening the page by a plain URL button gives no `initData` `[UNVERIFIED for each mode]`, so validation has nothing to verify and developers are tempted to accept `user_id` from the query string.
- `chat_instance`, `chat_type` in `initData` are not a chat identity; do not derive the captcha's chat from them.
- `start_param` is limited (512 characters, `A-Z a-z 0-9 _ -`) `[UNVERIFIED]`; carrying a signed JWT or a long payload fails.
- Direct-link Mini Apps (`https://t.me/<bot>/<appname>?startapp=<param>`) require the app to be registered with BotFather first; this is a deployment prerequisite that tests will not catch.
- Bot API 10.x adds a native join-request Mini App flow (`ChatJoinRequest.query_id` with `sendChatJoinRequestWebApp` and `answerChatJoinRequestQuery`, which must be answered within 10 seconds, and applies only to "bots assigned to process join requests"). It is present in the vendored gotgbot, and it is a candidate for groups that use join approval. Treat as an opportunity to evaluate, not a requirement; its assignment mechanism is not in the spec I could read.

**How to avoid:**
- Group captcha message carries a **URL button** to a `t.me` direct link (or a PM deep link `t.me/<bot>?start=cap_<token>` that opens a PM with a `web_app` button). Pick one in a P3 spike on a real client matrix (Android, iOS, Desktop, Web).
- Bind the attempt server-side: the link carries only an opaque, single-use, high-entropy token (`start_param`) that maps in Postgres/Redis to `(attempt_id, user_id, chat_id, expires_at)`. After validating `initData`, require `initData.user.id == attempt.user_id`. Ignore any `user_id`/`chat_id` sent in the POST body.
- Token TTL = captcha timeout; consumed atomically on success; one outstanding token per `(user, chat)`.
- Document the BotFather steps in the deploy docs and add a startup/health check that warns if `BOT_USERNAME`/Mini App short name is unset.

**Warning signs:**
- Plan contains "inline web_app button in the group".
- The POST handler reads a `user_id` field.
- No device-matrix test.

**Phase to address:** P3 (spike first).

---

### Pitfall 14: `initData` validation mistakes

**What goes wrong:**
- Validation skipped or done in the browser only.
- Wrong algorithm: the key is `HMAC_SHA256(key="WebAppData", msg=bot_token)`, then `hash = HMAC_SHA256(key=that, msg=data_check_string)`; swapping the arguments is the classic bug. `data_check_string` is all received `key=value` pairs, **excluding `hash`**, sorted alphabetically, joined with `\n`, values URL-decoded once. Double decoding or sorting encoded values produces intermittent failures on names with unicode. (The newer Ed25519 `signature` is for third-party validation; for first-party HMAC verification keep the documented rule and use a known test vector. `[UNVERIFIED]` whether `signature` stays in the data-check string for HMAC, so test with live data.)
- Non-constant-time compare of the hash (use `hmac.Equal`).
- No freshness check. `auth_date` is the only anti-replay signal in `initData`. A leaked `initData` string is valid forever without it. Check `auth_date` against both a hard max age and `attempt.created_at` (it must not predate the attempt).
- Validating the bot token from a different bot (staging vs prod) -> everything fails with no useful error.
- `initData` logged at debug level: it contains the user ID, names, and the hash.

**How to avoid:**
- One small, unit-tested package `miniapp/initdata` with a published test vector, constant-time compare, max age (e.g. attempt timeout) and `auth_date` future-skew limit.
- Return one generic error to the client; log the reason at `Info` without the payload.
- Never echo `initData` into logs, error messages, or metrics labels.

**Warning signs:**
- `strings.Split` of `initData` on `&` without `url.ParseQuery`.
- `==` used to compare hashes.
- No `auth_date` constant.

**Phase to address:** P3.

---

### Pitfall 15: Turnstile `siteverify` mistakes: unbound tokens, test keys, and fail-open

**What goes wrong:**
Facts from Cloudflare's docs: tokens are valid 300 seconds, single-use (replay -> `timeout-or-duplicate`), max 2048 characters, and the response includes `success`, `hostname`, `action`, `cdata`, `challenge_ts`, `error-codes`. The docs' own sample shows the server must compare `action` and `hostname` itself.
- Treating HTTP 200 as success instead of `success == true`.
- Not binding the token to the attempt. A token solved by a human (or a solving farm) for session A can be submitted for attempt B within 300 s. Single-use stops reuse of the *same token*, not cross-attempt submission. Without `cdata`/`action` binding, a farm sells passes.
- Production running with Cloudflare's published always-pass test secret/sitekey (copied from docs into `.env`/compose). Every captcha passes.
- Unbounded `http.Client` (default has no timeout); a Cloudflare hiccup hangs the Mini App request and the goroutine.
- Failing **open** on a siteverify error (network, 5xx) lets raiders through during exactly the outage you'd least notice; failing **closed** by *kicking* the user punishes humans for our fault.
- Treating `timeout-or-duplicate` on a double-tap as a failure after the first request already passed.
- Passing `remoteip` as a hard requirement: Telegram WebViews, mobile NAT, and a reverse proxy's `X-Forwarded-For` make the IP unreliable (and spoofable if you trust the header from the open internet).
- `idempotency_key` misuse: it exists to safely **retry our own** failed siteverify call, not to accept duplicates from the client.

**How to avoid:**
- Render the widget with `data-action="tg-captcha"` and `data-cdata=<attempt token>` (cdata length limit `[UNVERIFIED]`, keep it short). Server accepts only if `success && action == expected && cdata == attempt.token && hostname == configured`.
- Startup validation: refuse to start with `captcha_mode=turnstile` if the secret/sitekey matches known Cloudflare test values, or if either is empty.
- HTTP client with explicit timeouts (e.g. 5 s total), 1 bounded retry using `idempotency_key`, no retry on `invalid-input-*`.
- On verification **error** (not `success=false`): do not mark the attempt failed; return a retryable error to the page and let the existing timeout be the only thing that kicks.
- On `timeout-or-duplicate`: look up the attempt; if already `passed` return success, otherwise instruct the page to reset the widget and retry.
- `remoteip`: send only when taken from a configured trusted-proxy header, never as a gate.

**Warning signs:**
- Siteverify call without `hostname`/`action`/`cdata` comparisons.
- `.env.sample` contains a working-looking Turnstile test key.
- `http.Post` or `http.DefaultClient` in the verifier.

**Phase to address:** P3.

---

### Pitfall 16: Completion races and side-effects from an HTTP handler (success vs timeout-kick vs leave)

**What goes wrong:**
The existing captcha is driven entirely by Telegram updates and a lifecycle task runner (`captcha.go:150+`, 10 s hardcoded shutdown wait in `main.go`). Turnstile adds a **second, out-of-band writer**: an HTTP handler that unmutes the user. Races:
- Success arrives at the same moment the timeout task kicks: the user is kicked after passing, or unmuted after being kicked.
- User leaves or is banned while solving. `restrictChatMember` on a banned or left user can change their state `[UNVERIFIED]` (it can convert a ban into a restriction record), so the "unmute" can *undo a ban* applied by an admin in the meantime.
- Double-submit (retry on flaky mobile network) unmutes twice and replays the pending messages twice (the group `-10` replay path).
- The unmute reads `resolveUnmutePermissions(getChat)`, which during a lockdown yields the locked set (Pitfall 7b).
- HTTP handler goroutines need `defer error_handling.RecoverFromPanic(...)`; Telegram writes after `db.Close` during shutdown (AGENTS.md: "Register new drains after DB-close so they run before it") panic or log noise.
- A raid hitting `/verify` with garbage: unauthenticated public endpoint doing DB lookups and outbound siteverify calls (amplification to Cloudflare, rate-limit exposure).

**How to avoid:**
- Reuse the existing atomic claim (`DeleteCaptchaAttemptByIDAtomic` pattern, `captcha_attempts` one-per-`(user, chat)`) as the single arbitration point: success and timeout both go through a compare-and-set on attempt state (`pending -> passed | failed`); only the winner performs side effects. Idempotent success returns 200 for duplicates.
- Before unmuting, `getChatMember`: act only if the user is still a member and `restricted`. If the user is `kicked`, `left`, or admin: do nothing and mark the attempt `stale`.
- Order: verify -> CAS the attempt -> unmute -> replay pending messages -> `DeleteCache` of `alita:cache:captcha_pending:<chat>` (AGENTS.md).
- Rate limit per IP and per token at the HTTP layer; cap request body (e.g. 8 KB); reject before any DB or outbound call when `Content-Type`/shape is wrong.
- Graceful shutdown: stop the HTTP listener first, then drain in-flight handlers via the WaitGroup, then DB close.

**Warning signs:**
- The HTTP handler calls `RestrictChatMember` directly.
- Attempt state transitions use two separate queries.
- No concurrency test that runs timeout and success simultaneously.

**Phase to address:** P3.

---

### Pitfall 17: Secrets and tokens leaking via logs, URLs, and Referer

**What goes wrong:**
- `logredact` scrubs only **exact** registered values (`RegisterSecret`, min length 6, `logredact.go:77`) and some header patterns. The Turnstile secret must be registered at config load next to the others (`config.go:486`). But derived or transient sensitive values are not "secrets": the HMAC key derived from the bot token, `initData` strings, Turnstile response tokens, attempt tokens in URLs. A `DEBUG=true` request dump leaks them.
- Putting the attempt token in the **query string** of the Mini App URL puts it into reverse-proxy access logs, browser history, and `Referer` headers sent to Cloudflare's challenge origin.
- The Turnstile **sitekey** is public by design; the **secret** must never be rendered in the page, returned in an API response, or committed to a sample env.
- `sample.env` / compose / `render.yaml` / `railway.toml` / `app.json` all carry env lists: a new env var added to one and forgotten in others, or a real value pasted into a manifest.

**How to avoid:**
- Register `TURNSTILE_SECRET_KEY` in `logredact.RegisterSecret` at config load, and add a test that logs a line containing it and asserts redaction.
- Page and API responses: `Referrer-Policy: no-referrer`, `Cache-Control: no-store`; pass the token in a fragment or POST body, not a path that is logged by default.
- Never log request bodies/URLs of `/verify`. Log attempt IDs, not tokens.
- Add the new env vars to every manifest and `sample.env` in one commit with empty values; `make check-docs` should cover them if documented.

**Warning signs:**
- `log.Debugf("%+v", r)` or `httputil.DumpRequest` anywhere on the Mini App path.
- Token in `r.URL.Query()`.

**Phase to address:** P3.

---

### Pitfall 18: Hosting the Mini App on the same HTTP server as webhook, metrics and pprof; CSP and framing

**What goes wrong:**
- The bot's single HTTP server mux (`httpserver/server.go`) already serves `/health`, `/webhook`, `/metrics`, `/db_metrics` (token-protected, but only if `METRICS_AUTH_TOKEN` is set - otherwise a warning and **unauthenticated**), and `/debug/pprof/` (registered via `http.DefaultServeMux` when enabled). Exposing this port publicly for the Mini App publishes all of it. Worker/polling deployments (`WorkingMode = "worker"`) may have no public ingress at all today, so P3 adds a real deployment requirement (public HTTPS, valid cert).
- CSP: Turnstile needs `script-src` and `frame-src` for `https://challenges.cloudflare.com` (Cloudflare docs). A strict CSP that omits `frame-src` yields a blank widget. Conversely, no CSP means any injected script reads `initData`.
- Telegram Web (browser) clients load Mini Apps inside an iframe; `X-Frame-Options: DENY` or a restrictive `frame-ancestors` breaks them `[UNVERIFIED which origins]`.
- The `/verify` endpoint is cross-origin-callable unless restricted.

**How to avoid:**
- Serve the Mini App from a separate listener/port (or at minimum a dedicated mux with an explicit allowlist of paths), and never mount pprof/metrics on it. Add a test that requests `/metrics`, `/debug/pprof/` and `/db_metrics` against the public listener and expects 404.
- Self-host the page's JS/CSS (no third-party CDN besides Telegram's `telegram-web-app.js` and Cloudflare's `api.js`); CSP with a per-response nonce, `script-src 'nonce-...' https://challenges.cloudflare.com https://telegram.org`, `frame-src https://challenges.cloudflare.com`, `connect-src 'self'`, `frame-ancestors` set to the Telegram web origins verified in the spike; `X-Content-Type-Options: nosniff`.
- CORS: same-origin only for `/verify`.
- Keep `captcha_mode=math|text` (existing) as a per-chat fallback if Turnstile is misconfigured or Cloudflare is down; ship Turnstile as an additional mode, not a replacement, until proven (PROJECT.md says "fit the flow or replace cleanly"; a mode switch is the lowest-risk interpretation).

**Warning signs:**
- `RegisterWebhook` and the Mini App routes share a mux that also has `RegisterPPROF`.
- `METRICS_AUTH_TOKEN` empty in the deploy manifest.
- Blank widget only in Telegram Web.

**Phase to address:** P3.

---

### Pitfall 19: 64-byte callback data and the silent `""` return

**What goes wrong:**
Telegram requires `callback_data` of 1-64 bytes (spec). The module wrapper `encodeCallbackData` returns `""` on overflow (AGENTS.md), shipping a dead button or a rejected message. The failure is data-dependent: chat IDs are up to 14 characters with the sign (`-1001234567890`), user IDs up to 10+ digits, and URL encoding of `|`/`&`/`=` expands payloads. Menu callbacks with `page`, `chat`, `setting`, `value`, `message owner` fields will overflow only for the longest IDs, i.e. in production on your biggest group and never in tests.

**How to avoid:**
- Budget: with `ns|v1|` taking ~6-8 bytes, keep payload fields <= ~50 bytes. Use 1-2 character field names and enum codes, never user text.
- Prefer deriving context rather than carrying it: a menu opened in a group takes `chat_id` from the message; a menu opened in PM through connections takes it from the stored connection (re-validated on each press). Carry only `page` + `action` + `value`.
- When state must be carried (e.g., pending Staff link confirmation), store it behind a short random token in Redis/Postgres with a TTL appropriate to the interaction, and handle token-expired by re-rendering the menu, not by erroring.
- Test: generate the full keyboard for a worst-case chat (`-1009999999999`) and worst-case user ID in every locale and assert every button's `callback_data` is 1..64 bytes and never `""`. Make `encodeCallbackData` overflow log at `Warn` with the namespace (CONCERNS.md recommendation, adopted).

**Warning signs:**
- Fields named `chat_id`, `user_id` both present in a single menu callback.
- Any button built without checking the returned string.

**Phase to address:** P4 (and P2 for alert buttons).

---

### Pitfall 20: Settings menu: permission checks only at render time, toggles that do not persist, and cache drift

**What goes wrong:**
- Menu messages in groups are visible to everyone; **anyone can press**. Checking "is admin" only when the menu is sent, or reading the sender of the original command, leaves every button open to any member. Permission must be re-verified on every press against the *target chat* (the group being configured, which in a PM connection is not the chat of the message).
- Different settings need different rights (e.g. lockdown thresholds need `can_restrict_members`; Staff Group link/unlink needs **owner**; captcha mode needs `can_change_info` or admin). A single "is admin" gate lets a low-rights admin reconfigure security.
- `UpdateRecord` skips zero values (AGENTS.md). A menu "turn off" handler that sets `false`/`0`/`""` through `UpdateRecord` silently does nothing, yet the menu re-renders the old state. Use `UpdateRecordWithZeroValues` for every boolean toggle.
- The menu reimplements a setting write instead of calling the same repository function the command uses, so the `cache.DeleteCache` keys diverge (AGENTS.md: every write must delete the keys it affects; reads go through `GetFromCacheOrLoad` and a 10 s local layer). Menu shows stale state on the next render.
- Toggles that send "toggle" rather than "set X=on" are not idempotent: two admins tapping, a double-tap, or a retried update flips twice.
- `*ForUpdate` predicates are memoised per update; calling them after the state change in the same update returns stale answers.

**How to avoid:**
- One `settings.Authorize(ctx, targetChat, userID, requiredRight)` used by every callback, backed by a **live** check for security-sensitive pages (staff, lockdown, captcha), and the admin cache only for low-risk cosmetic pages.
- A table `page -> required right` kept in one place and unit-tested.
- Buttons carry the desired value (`set=1`), the handler writes through the module's existing repository function, then **re-reads** (after `DeleteCache`) to render.
- Handle Telegram's `message is not modified` (400) as success; always `AnswerCallbackQuery`; edit with `editMessageText` rather than sending a new message per tap.
- Staff Group page: link/unlink verifies **ownership live** (Pitfall 3), requires the confirmation step, and shows the broken/ok status per linked group.

**Warning signs:**
- A menu handler that writes a field directly via GORM or `UpdateRecord`.
- No test where a non-admin presses a button on an admin's menu.
- `settings` page code does not import the existing repository packages.

**Phase to address:** P4.

---

### Pitfall 21: Locale and rendering failures in the menu (7 locales, empty strings, message size)

**What goes wrong:**
A missing i18n key returns `""` with an error callers ignore (AGENTS.md). In a keyboard, an empty button label makes Telegram reject the **entire** `sendMessage`/`editMessageText` (button text must be non-empty). Seven locale files must each contain every new key; a typo ships an empty label only in some languages. Longer translations make 2-per-row layouts wrap badly. The menu text can exceed 4096 characters with many linked groups on the Staff page.

**How to avoid:**
- Add all keys to all 7 files in the same commit as the code, run `make check-translations`, and copy key names from `locales/en.yml`.
- A test that renders every page in every locale and asserts no empty button text and total text < 4096.
- Paginate the Staff page (N groups per page) and use `/staff` text for long lists.

**Warning signs:**
- `text, _ := tr.GetString(...)` feeding a `Text:` field with no empty check.
- Locale files edited in different commits.

**Phase to address:** P4 (and every phase that adds locale keys).

---

## Moderate Pitfalls

### Pitfall A: Reason text and log-channel posting (P1)

**What goes wrong:** An optional free-text reason is posted into N groups' log channels. Unescaped HTML breaks `ParseMode: HTML` sends (400) or injects markup/links; very long reasons overflow; a missing or revoked log channel in one group makes the loop error out.
**Prevention:** `html.EscapeString` (as `notifyFedAction` does for the reason), cap length (~300 chars), treat log-channel delivery failure as `warn` in the summary and never as a failed moderation action. Log-channel send is a separate, paced call; do it after the action, not before.

### Pitfall B: Handler group placement (P1, P2)

**What goes wrong:** New watchers placed in a group that kills later handlers. A group-0 watcher returning `nil` silently disables later group-0 handlers; the group-number list in AGENTS.md is the contract and CONCERNS.md calls out brittleness.
**Prevention:** Pick the lockdown watcher's group number deliberately. It must run before greetings/captcha (group `0`) and join-request auto-approve, and it must not depend on the users tracker (`-1`) having run first. Return `ext.ContinueGroups`, update the AGENTS.md group-number list in the same commit, and add a registration-order test.

### Pitfall C: Docs, translations, and generated files (all phases)

**What goes wrong:** New commands without `make generate-docs`, new locale keys in fewer than 7 files, version strings edited by hand. CI fails late.
**Prevention:** Make the "Adding a module" checklist (AGENTS.md steps 1-8) the definition-of-done for every plan; check-docs/check-translations/lint/test in every phase's final verification.

### Pitfall D: Tests that assert implementation instead of behavior (all phases)

**What goes wrong:** AGENTS.md forbids literal/source-substring assertions and mock libraries, but fan-out and lockdown logic invite them. The 15+ modules without tests (`bans.go`, `mute.go`, `captcha.go`, `federations.go`) are the same code the new features touch.
**Prevention:** A reusable hand-written `gotgbot.BotClient` fake with a programmable per-chat member/permission table and per-call error injection (429 with `retry_after`, `CHAT_ADMIN_REQUIRED`, "can't remove chat owner"). Assert observable behavior: calls made / not made, rows persisted, cache invalidated. Build it once in P1-foundation.

### Pitfall E: Lockdown + captcha interplay ordering (P2, P3)

**What goes wrong:** During lockdown, new members are kicked, so captcha never runs for them; but members that joined seconds before the lockdown are in `captcha_attempts` pending with a muted state and a timeout task. Lift does not touch them (correct), but a lockdown's "mute everyone" via default permissions plus a captcha pass unmute can leave per-user state inconsistent (see Pitfall 7b).
**Prevention:** Decide and test: lockdown does not cancel pending captcha attempts; captcha pass during lockdown applies snapshot-derived permissions.

### Pitfall F: Raid detection thresholds in multi-replica or restarted bot (P2)

**What goes wrong:** Antiflood counters are per replica and lost on restart (verified in AGENTS.md/CONCERNS.md as a known characteristic). Raid-rate counters stored there would reset at restart. Redis counters are fine until Redis restarts.
**Prevention:** Put lockdown rate counters in Redis with DB-backed lockdown state; accept brief detection degradation and say so in the alert.

### Pitfall G: Staff Group designation UX edge cases (P1)

**What goes wrong:** A group designated as Staff Group while it is already linked as a member of another Staff Group; a Staff Group linking itself; a group linking to a Staff Group it is not owned by; a channel designated (PROJECT.md forbids). Cycles and self-links are easy to miss.
**Prevention:** Constraint set at the DB: a chat cannot be both a Staff Group and a linked group; unique link per chat; type check `group|supergroup` via live `getChat`.

---

## Minor Pitfalls

### Pitfall H: Rate-limit blind spot on admin commands (P1)
CONCERNS.md lists "no rate limiting on admin commands" (UNVERIFIED). Staff commands multiply the cost of each invocation by the group count; add a per-issuer cooldown (e.g. 1 action per 3 s) and a global in-flight cap.

### Pitfall I: Username privacy of `@name` in summaries (P1)
Posting resolved names/IDs into every group's log channel exposes the staff roster and issuer identity; use the issuer's name only in the Staff Group summary and a neutral label ("Staff") in member-facing groups if desired.

### Pitfall J: Message edit rate (P2)
Updating a lockdown alert for every kicked joiner (`editMessageText`) hits ~1 edit/second per chat. Batch edits (every 3-5 s), cap listed names, and show "+N more".

### Pitfall K: Webhook secret reuse (P3)
Do not reuse the webhook secret or metrics token as a Mini App session secret; use a separate high-entropy secret registered with `logredact`.

---

## Technical Debt Patterns

| Shortcut | Immediate Benefit | Long-term Cost | When Acceptable |
|----------|-------------------|----------------|-----------------|
| Reuse `IsUserAdmin` in fan-out | No new code | Cross-group authorization hole (Pitfall 1) | Never |
| Copy `applyActiveFban` loop | Fast to write | Silent 429 drops, no summary fidelity (Pitfall 5) | Never for staff; federations can be left alone |
| Lockdown state only in Redis | No migration | Silent loss of "never lifts" (Pitfall 9) | Never |
| Single global lockdown threshold | One constant | False positives on small groups (Pitfall 11) | Only in shadow mode |
| Toggle buttons ("flip") | Smaller callback data | Not idempotent, double-tap flips (Pitfall 20) | Never |
| Turnstile replaces old captcha | Less code to maintain | No fallback if Cloudflare/hosting down (Pitfall 18) | After a stable release with the mode switch |
| Carry chat_id in every callback | Simple handlers | Overflow on long IDs, chat spoof surface (Pitfalls 12, 19) | Only for short, token-less, read-only buttons |
| Hand-rolled `ChatPermissions` structs for restore | No JSON handling | Lossy restore (Pitfall 8) | Never; store raw JSON |
| Skip the throwaway-group spikes | Faster roadmap | Build on `[UNVERIFIED]` Telegram assumptions | Only for items not marked `[UNVERIFIED]` |

## Integration Gotchas

| Integration | Common Mistake | Correct Approach |
|-------------|----------------|------------------|
| Telegram `getChatMember` | Using cached admin list for cross-chat authz | Live call per group; handle `creator` explicitly; fail closed |
| Telegram `restrictChatMember` | Using it on banned/non-member targets and basic groups | Pre-check status and chat type; supergroups only |
| Telegram `unbanChatMember` | Default `only_if_banned=false` for `/unban` or `/kick` of non-members | `only_if_banned=true` for unban; kick only current members |
| Telegram `setChatPermissions` | Believing it can exempt approved users | Spike; accept approved users are muted too, or find an override |
| Telegram `banChatMember` | Raw `until_date` below 30 s (becomes permanent) | Use `TemporaryUntilDate`; note `revoke_messages` always true in supergroups |
| Telegram 429 | Treating as generic error | `retry_after` backoff + global bucket + typed result |
| Telegram callback data | Trusting `data` as authorization | Look up server-side state, re-authorize presser live |
| Telegram inline `web_app` button | Using it in a group message | URL button to direct link / PM deep link |
| Telegram Mini App `initData` | Trusting `unsafe` fields or client `user_id` | HMAC validate, `auth_date` age, bind to server token |
| Cloudflare Turnstile `siteverify` | Check HTTP status only; no action/hostname/cdata; test keys in prod | Compare all four, startup guard on test keys, bounded HTTP client |
| Redis | Using it as the source of truth for a never-expiring state | Postgres truth + Redis accel + `skipLocal` for freshness-critical keys |
| gotgbot handler groups | Watcher returns `nil` in group 0 | `ext.ContinueGroups`, explicit group numbers, registration-order test |

## Performance Traps

| Trap | Symptoms | Prevention | When It Breaks |
|------|----------|------------|----------------|
| Per-group live `getChatMember` x several calls in sequence | Staff command takes 10+ seconds | Concurrent across groups (bounded), sequential within; ack immediately, edit later | ~15-20+ linked groups |
| Summary one line per group | 4096-char limit overflow, message rejected | Paginate or compress non-ok lines | ~35-40 groups |
| Lockdown alert edited per kick | 429 on the alert chat | Batch every 3-5 s | Raid > 1 join/s |
| Kick loop on public invite link | Bot API 429 on the bot itself, normal moderation degrades | Ban with short `until_date` or batch unban; decline join requests | Raid with rejoin bots |
| Captcha image generation per join under raid | CPU spike (CONCERNS.md: unthrottled, VERIFIED as noted by mapper, not re-measured) | Skip captcha for users being kicked; bounded pool | ~100 joins/s |
| Public `/verify` endpoint unauthenticated | Amplified Cloudflare/DB load | Per-IP and per-token rate limits, body cap, early reject | Any bot-driven abuse |
| `ChatsContainingUser` JSON/array scan of `chats.users` for fed-ban | Slow for large chats | Not used by Staff (staff uses linked chat IDs, not "known users"); do not reuse for staff | Large user arrays |
| 10 s local cache on lockdown reads | Replica B lets joins through for up to 10 s | `skipLocal` prefix | Multi-replica |

## Security Mistakes

| Mistake | Risk | Prevention |
|---------|------|------------|
| `tgAdminList` bypass reached via fan-out (IDs 1087968824, 777000) | Any anonymous post = admin everywhere | Dedicated authorizer, explicit ID rejection (Pitfall 1) |
| Staff member bans group owner/admin via the bot | Bot becomes a privilege-escalation tool | `target_protected` per group (Pitfall 4) |
| Stale link after ownership change | Former owner retains cross-group power | Live owner check per action; break + notify (Pitfall 3) |
| Callback authorization from message chat | Wrong people lift lockdown (Pitfall 12) | Chat from DB row; live check |
| Forged callback data | Replay of old action buttons | ID-bound idempotent actions |
| Turnstile test keys or fail-open in prod | Captcha bypass | Startup guard; fail closed without kicking |
| Token not bound to attempt (`cdata`) | Solving-farm passes sold per attempt | `cdata` + `action` + `hostname` checks |
| `initData` without `auth_date` limit | Replay of stolen `initData` | Max age + attempt-time check |
| Secret and token logging | Credential leak via debug logs/proxies | `RegisterSecret`, no body logging, `no-referrer` |
| pprof/metrics on the public Mini App port | Info leak and DoS | Separate listener/allowlist mux |
| Open Staff Group (public or open invite) | Anyone joining gains the first authority factor | Warn/block at designation; live membership check |
| `revoke_messages` on "Ban N recent joiners" | Irreversible deletion on false positive | Confirm step, cap, stored joiner set |

## UX Pitfalls

| Pitfall | User Impact | Better Approach |
|---------|-------------|-----------------|
| Summary says "done" for partial results | Staff think a user is banned everywhere | Per-group typed result with reasons and a failed count in the header |
| Silent skip when issuer is not admin in a group | Staff don't know why a group was missed | "skipped: not an admin with can_restrict_members" per line |
| No "working..." ack | Staff re-run the command, double actions | Immediate ack, then edit |
| Lockdown banner before confirmation | False reassurance | State machine with confirmed steps |
| Dead Lift button (token expired) | Group locked until someone finds the command | DB-backed IDs and `/lockdown off` fallback |
| Mini App opens a blank page in Telegram Web | Humans can't pass, get kicked | CSP/frame spike, fallback captcha mode, timeout is the only kick |
| Menu in the group visible to everyone | Spam and accidental presses | Open in PM via connection, or auto-delete; deny non-admins with a toast |
| Captcha kicks a human for our verification error | Lost members | Verification errors are retryable and never count as failure |
| 7 locales, one missing key | Empty button, whole message fails for that language | Render-all-locales test |

## "Looks Done But Isn't" Checklist

- [ ] **Staff fan-out:** Often missing the per-group live check for anonymous IDs - verify a test for `1087968824` and `777000` yields zero write calls.
- [ ] **Staff fan-out:** Often missing 429 handling - verify a fake returning `retry_after` leads to a retry, then `rate_limited` in the summary.
- [ ] **Staff link:** Often missing re-verification at action time - verify demoting the owner between link and action marks the link broken.
- [ ] **Staff link:** Often missing chat migration - verify `migrate_to_chat_id` re-keys links.
- [ ] **Target safety:** Often missing banned-user handling - verify `/mute` and `/kick` do not lift an existing ban in any group.
- [ ] **Lockdown:** Often missing real enforcement confirmation - verify the alert reflects failed `setChatPermissions`.
- [ ] **Lockdown:** Often missing restore fidelity - verify prior permissions match byte-for-byte (raw JSON) after a double trigger and a mid-lockdown unmute.
- [ ] **Lockdown:** Often missing persistence - verify a Redis flush does not end an active lockdown.
- [ ] **Lockdown:** Often missing all three join paths - verify chat_member, service message and join request each kick/decline.
- [ ] **Lockdown buttons:** Often missing authorization against the locked group - verify a Staff Group admin who is not admin there is denied.
- [ ] **Turnstile:** Often missing `hostname`/`action`/`cdata` checks - verify a token minted for another action/attempt fails.
- [ ] **Turnstile:** Often missing test-key guard - verify startup refuses Cloudflare test keys in production mode.
- [ ] **Mini App:** Often missing public-port isolation - verify `/metrics` and `/debug/pprof/` return 404 on the Mini App listener.
- [ ] **Mini App:** Often missing timeout/success race handling - verify exactly one outcome when both fire.
- [ ] **Settings:** Often missing zero-value writes - verify turning a setting OFF persists and re-renders OFF.
- [ ] **Settings:** Often missing per-press authorization - verify a non-admin press is denied and an admin without the page's right is denied.
- [ ] **Callbacks:** Often missing the worst-case length - verify every button for chat `-1009999999999` is 1..64 bytes in every page.
- [ ] **All:** Locale keys in all 7 files, `make generate-docs`, `make check-translations`, `make lint`, `make test`.

## Recovery Strategies

| Pitfall | Recovery Cost | Recovery Steps |
|---------|---------------|----------------|
| Cross-group authorization hole shipped (1) | HIGH | Disable staff commands via feature flag/env, audit the staff action table for actions by anonymous/service IDs, un-ban affected users manually, add the test matrix, re-enable |
| Stale link let a former owner act (3) | HIGH | Revoke all links, force re-link by current owners, review the action audit trail |
| Wrong user banned by stale `@username` (4) | MEDIUM | Use the stored action record to `unban` in affected groups; add ID confirmation in summary |
| 429 storm / dropped actions (5) | MEDIUM | Re-run the action (idempotent) after backoff; the persisted action record lists which groups failed |
| Users still muted after lockdown (7b) | MEDIUM | One-off script: for users restricted during the lockdown window, restore snapshot permissions via the fan-out executor |
| Lockdown state lost (9) | MEDIUM | Query the DB lockdown table, re-apply kicks/permissions, announce; if Redis-only was shipped, migrate state and add reconciliation on startup |
| False-positive lockdown (11) | LOW | `/lockdown off` or button; raise thresholds; use the shadow-mode logs to retune |
| Turnstile bypass via test keys (15) | MEDIUM | Rotate keys, switch chats back to math/text captcha mode, review passes in the window |
| Secret logged (17) | HIGH | Rotate the secret, purge logs, add redaction test |
| Public port exposes pprof/metrics (18) | HIGH | Close the port, rotate `METRICS_AUTH_TOKEN`, redeploy with the isolated listener |
| Dead menu buttons (19) | LOW | Hotfix the encoding, add the worst-case test |

## Pitfall-to-Phase Mapping

| Pitfall | Prevention Phase | Verification |
|---------|------------------|--------------|
| 1. `IsUserAdmin` as gate | P1-foundation | Authz matrix test; zero write calls on denied rows |
| 2. Issuer identity / anonymous | P1 | Anonymous Staff Group message test |
| 3. Link validity / migration | P1 | Demote-owner and migrate-chat-id tests |
| 4. Target resolution and protection | P1 | Ambiguous username, banned target, admin target tests |
| 5. Fan-out pacing and accounting | P1-foundation | 429 injection test; summary fidelity test |
| 6. Command collision | P1 | Registration-order test in Staff vs linked group |
| 7. Approved-user exemption / unmute contamination | P2 (spike first) | Spike notes; unmute-during-lockdown-then-lift test |
| 8. Restore fidelity | P2 | Raw-JSON snapshot, double-trigger test |
| 9. Redis-only state / antiraid replacement | P2 | Redis flush test; `skipLocal` entry present |
| 10. Join races | P2 | Three-path join test; double-count test |
| 11. Announce vs reality, false positives | P2 | Failure injection; shadow-mode log |
| 12. Alert button authorization | P2 | Cross-chat press test; stale-button test |
| 13. Mini App launch plumbing | P3 (spike first) | Device matrix; token-binding test |
| 14. `initData` validation | P3 | Test vector; expired `auth_date` test |
| 15. `siteverify` mistakes | P3 | Mismatch tests; test-key startup guard |
| 16. Completion races | P3 | Concurrent success vs timeout test |
| 17. Secret leakage | P3 | Redaction test; header test |
| 18. Hosting / CSP / framing | P3 | Public listener 404 test; Telegram Web render check |
| 19. Callback 64-byte limit | P4 (also P2) | Worst-case length test across pages and locales |
| 20. Menu authorization and persistence | P4 | Non-admin press test; OFF toggle persistence test |
| 21. Locale / rendering | P4 | Render-all-locales test |
| Moderate A-G, minor H-K | the phase named in each | Per-item prevention above |

## Open Spikes (do before or at the start of the named phase)

| Question | Why it matters | Phase |
|----------|----------------|-------|
| Does a per-user "can send" restriction override a locked default permission set? | Decides whether "except approved users" is deliverable (Pitfall 7) | P2 |
| Does `restrictChatMember` on a banned/left user lift the ban or add a restriction record? | Safe design of mute/unmute fan-out and Turnstile unmute (Pitfalls 4, 16) | P1 |
| Are `chat_member` updates emitted for ownership transfer? | How fast links can break by event (Pitfall 3) | P1 |
| Service-message join visibility in large supergroups | Join path coverage (Pitfall 10) | P2 |
| Which Mini App launch modes provide `initData` from a group message | Pitfall 13 | P3 |
| Allowed `frame-ancestors` origins for Telegram clients; `cdata` length limit | Pitfalls 15, 18 | P3 |
| How a bot is "assigned" to join-request queries (`sendChatJoinRequestWebApp`) | Optional native captcha path (Pitfall 13) | P3 |

## Sources

- Codebase (read in this session, HIGH): `AGENTS.md`; `alita/utils/chat_status/chat_status.go` (`tgAdminList`, `IsUserAdmin`, `checkAnonAdmin`); `alita/utils/cache/adminCache.go` and `alita/utils/constants/time.go` (`AdminCacheTTL`); `alita/modules/federations.go:735,756` (`applyActiveFban`); `alita/modules/antiraid.go` (`trackJoin`, `onJoin`, `callbackHandler`, poller start); `alita/modules/greetings.go` (`claimRecentJoinProcessing`, `pendingJoins`); `alita/modules/captcha.go` (`SendCaptcha`, `unmuteCaptchaUser`); `alita/modules/chat_permissions.go`; `alita/modules/bans.go` (`kickMember`); `alita/utils/extraction/extraction.go` (`GetUserId`); `alita/db/user/repository.go` (`GetUserIdByUserName`); `alita/utils/callbackcodec/callbackcodec.go`; `alita/utils/helpers/command_pipeline.go`; `alita/utils/httpserver/server.go`; `alita/utils/logredact/logredact.go`; `alita/db/cache/local.go` (`skipLocal`); `alita/config/config.go` (`AllowedUpdates`, `RegisterSecret`); gotgbot module source (`TelegramError.ResponseParams.RetryAfter`).
- `.planning/codebase/CONCERNS.md` (leads only; `applyActiveFban` lead REFUTED in shape; Redis-SPOF-for-antiraid lead VERIFIED; callback-overflow lead VERIFIED via AGENTS.md; others UNVERIFIED).
- Telegram Bot API 10.3 spec mirror (JSON of the official docs, version "Bot API 10.3", released 2026-08-24): https://github.com/PaulSonOfLars/telegram-bot-api-spec - used for `banChatMember`, `unbanChatMember`, `restrictChatMember`, `setChatPermissions`, `ChatPermissions`, `ChatMemberOwner/Administrator/Restricted`, `InlineKeyboardButton.web_app` (private chats only), `CallbackQuery.data`, `ChatJoinRequest.query_id`, `sendChatJoinRequestWebApp`, `answerChatJoinRequestQuery`. MEDIUM-HIGH: community-maintained mirror of the official page; core.telegram.org itself was blocked by the research environment's egress proxy.
- Cloudflare Turnstile docs source (official repo, HIGH): https://github.com/cloudflare/cloudflare-docs/blob/production/src/content/docs/turnstile/get-started/server-side-validation.mdx and `.../turnstile/reference/content-security-policy.mdx` - token 300 s, single-use, `timeout-or-duplicate`, `idempotency_key`, action/hostname comparison, CSP `script-src` and `frame-src`.
- Telegram flood guidance (1 msg/s per chat, 20 msgs/min per group, ~30 msgs/s bulk; 429 `retry_after`) via secondary summaries of https://core.telegram.org/bots/faq (MEDIUM; page blocked); grammY flood docs https://grammy.dev/advanced/flood.
- `initData` HMAC algorithm and `auth_date` guidance via https://docs.telegram-mini-apps.com/platform/init-data and its Node validation docs (MEDIUM; matches Telegram's description, not fetched from core.telegram.org).
- Items marked `[UNVERIFIED]` rest on general knowledge of Telegram behavior and are listed in "Open Spikes".

---
*Pitfalls research for: Telegram staff-group fan-out, raid lockdown, Turnstile Mini App captcha, inline settings menus (Go / gotgbot v2)*
*Researched: 2026-10-04*
