# Phase 2: Staff Actions Across Groups - Research

**Researched:** 2026-10-05
**Domain:** Telegram Bot API moderation fan-out (Go, gotgbot v2, GORM/PostgreSQL, Redis) inside an existing Alita-derived bot
**Confidence:** HIGH on codebase integration and on the `restrictChatMember` research flag; MEDIUM on Telegram rate-limit numbers (official FAQ page not directly fetchable from this sandbox)

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

**Command syntax**
- **D-01:** `/ban` and `/mute` take an optional duration. In `/ban @x 2d spamming`, the token right after the target is a duration if it matches the existing `m`/`h`/`d`/`w` form, and the action is then timed. Otherwise the action is permanent and the rest of the text is the reason. `/tban` and `/tmute` also work in the Staff Group and require a duration. The confirm card always shows the parsed duration, or "permanent", so a misparse is visible before Confirm.
- **D-02:** In a Staff Group, `/sban`, `/dban`, `/skick`, `/dkick`, `/smute` and `/dmute` are refused with a hint that names the staff commands. They never act locally in the Staff Group and never fan out.
- **D-03:** No reply-based targeting in a Staff Group. The target must be explicit: a numeric user ID, an `@username` the bot has seen, or a `text_mention` entity. A bare reply with no explicit target gets a hint asking for the `@username` or ID. Otherwise a reply would usually pick a fellow staff member, or the staffer who forwarded the spam.
- **D-04:** `@username` matching is case-insensitive. If the username matches more than one stored user, the bot refuses and lists every match with name, ID and when it last saw them, then asks for the numeric ID. Nothing is guessed. A username the bot has never seen is refused with a hint to use the numeric ID (ROADMAP success criterion 1).

**Confirm card**
- **D-05:** Carried forward from PROJECT.md and ROADMAP criterion 1: every staff action shows a confirm card first. It shows the resolved name and ID, the action, the duration, the reason and the number of linked groups. Nothing happens until the issuer taps Confirm, and only the issuer can confirm or cancel.
- **D-06:** Before Confirm the card shows only the linked-group count, for example "Applies to 6 linked groups". The issuer's rights are not pre-checked per group. Every check runs live after Confirm, and those results are the only ones that count.
- **D-07:** An unconfirmed card stays usable for **5 minutes**. After that its buttons answer "expired, run the command again", and the card is edited to "Expired" with the buttons removed.
- **D-08:** Cancel edits the card to "Cancelled by <name>" and removes the buttons. The card is never deleted, so the staff can see an action was considered and dropped.
- **D-09:** On Confirm the card is edited in place into the summary. The buttons are removed, the action and target header stays, and the per-group lines fill in below it. One action is one message in the Staff Group.

**Per-group rules**
- **D-10:** A staff `/ban` reaching a group the target isn't in, or has left, still bans there, so the target can't join it later. The line reads "banned (not in group)".
- **D-11:** `/mute` and `/kick` skip groups where the target isn't a member, with the line "skipped: not in group". A target whose status there is `kicked` (banned) counts as not in the group, so a kick can never lift an existing ban.
- **D-12:** A staff action never shortens an existing ban or mute. If the target is already banned or muted in a group and the new action would end sooner, the group is "skipped: already banned" or "skipped: already muted".
- **D-13:** A ban or mute is applied only when it ends later than the current one, where permanent counts as the latest. A permanent `/ban` over an existing 1-day ban upgrades it to permanent. The current end date comes from the same live `getChatMember` call the bot already makes for that group.
- **D-14:** Unban or unmute with nothing to do is "skipped: not banned" or "skipped: not muted". Unmute gives back the group's normal permissions, exactly as `/unmute` does there today. This was proposed as the default, and the owner accepted it by moving on.
- **D-15:** Carried forward from PROJECT.md and STAFF-07: in each group the bot never acts against that group's admins or owner, or against the bot itself. Such a group is skipped with the reason. Staff Group membership alone protects nobody.

**Progress summary**
- **D-16:** The summary is edited in batches, at most once every 2-3 seconds, while groups finish. A final edit follows when all groups are done. This keeps it clear of Telegram's per-chat edit limit, so the final result is never rate limited.
- **D-17:** Groups stay in a fixed order. Every linked group is listed from the start as "⏳ Group name". Its line then flips to "✅ Group name", "⏭ Group name: skipped: <reason>" or "❌ Group name: failed: <reason>". Lines never move. A tally line under the header shows the counts, for example "✅ 4 · ⏭ 1 · ❌ 1".
- **D-18:** If the full list won't fit in Telegram's 4096-character limit, the done lines collapse into one count line, for example "✅ 37 groups done". Skipped and failed groups are always listed by name with their reason, so no problem is ever hidden and no group is silently dropped (STAFF-08). With fewer than about 40 groups every line is shown.
- **D-19:** Failure lines use a plain, translated reason for known cases: "bot isn't an admin", "bot lacks ban rights", "rate limited, gave up after retries" and "group not found". An unexpected error reads "failed: Telegram error" followed by Telegram's short error text, so staff can tell errors apart and report them.

### Claude's Discretion
- What happens if the card is deleted, or can no longer be edited, mid-run? The final summary must still reach the issuer (STAFF-08), for example as a new message in the Staff Group. The planner picks the mechanism.
- What happens when two staff members act on the same target at once? The never-shorten and upgrade rules (D-12, D-13) make the outcomes converge. The planner decides whether anything more is needed.
- The exact hint wording for refused variants (D-02), bare replies (D-03), unknown usernames and ambiguous usernames (D-04). All of it goes in the 7 locale files.
- A length cap for the reason, and how a long reason is shortened on the card and summary header.
- Cross-action rules nobody asked about:
  - A ban on a muted user applies, because it is stricter.
  - A mute on a banned user is "skipped: already banned".
  - An unmute on a banned user is skipped, because mute and unmute must never lift a ban (ROADMAP research flag).
  The planner confirms these against research.
- Durations longer than Telegram's 366-day limit are treated as permanent, matching `extraction.TemporaryUntilDate`. The card shows the real result.
- The group order in the summary. The `/staff` panel's order (`ListLinksByStaffFresh`, `id ASC`) is the natural choice.

### Deferred Ideas (OUT OF SCOPE)
None: the discussion stayed within phase scope. Log-channel posting (STAFF-09), the audit record (STAFF-10) and "Undo everywhere" (STAFF-11) were already in Phase 3 and were not reopened. In Phase 2 the reason appears only on the card and in the summary.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| STAFF-01 | Any Staff Group member runs `/ban /mute /kick /unban /unmute` (timed ban/mute) against `@username` or numeric ID | Interceptor module at group 0 before Bans(70)/Mutes(80); pure arg parser; case-insensitive multi-match username lookup (Sections: Architecture, Patterns 1-3) |
| STAFF-02 | Optional reason on any staff action | Parser keeps rest-of-text as reason; length cap + HTML escaping; reason only on card and summary in this phase |
| STAFF-03 | Confirm card with name+ID, action, duration, reason, linked-group count; only issuer confirms/cancels | Redis card hash behind a 64-bit token, Lua compare-and-set state machine, issuer-bound callbacks `staff\|v1\|a=xc&t=<token>` (Pattern 2) |
| STAFF-04 | Confirmed action applied to every linked group, never the Staff Group | Snapshot `ListLinksByStaffFresh`; DB CHECK `group_chat_id <> staff_chat_id` plus role-exclusivity trigger already guarantee the Staff Group is never a link |
| STAFF-05 | Live per-group issuer admin + restrict check | Per-group `getChatMember(group, issuer)` accepting only `creator` or `administrator` with `can_restrict_members`; never `IsUserAdmin`/`CanUserRestrict` (Pattern 4) |
| STAFF-06 | Anonymous admin posts refused with "post as yourself" | Interceptor runs first and applies `helpers.IsAnonymousSender` / `RejectAnonymousSender` semantics, so the anon-proof button flow is never reached |
| STAFF-07 | Never act on admins/owner of a group or the bot | Target `getChatMember` status check before any write; creator/administrator -> skip; bot ID and Telegram service IDs -> skip |
| STAFF-08 | One summary message updated as groups finish; no group silently dropped | Pre-filled result slice, panic-safe workers, final sweep marks unset slots failed; batched edits; fallback new message; overflow continuation (Patterns 5-6) |
| STAFF-12 | Fan-out within rate limits, waits and retries on Telegram's say-so, failed not dropped | Redis slot-reservation pacer + shared `retry_after` block key + bounded retry wrapper (Pattern 3) |
| STAFF-13 | Existing commands unchanged outside Staff Groups | Interceptor returns `ext.ContinueGroups` with zero side effects when the chat is not a Staff Group; regression tests through a real dispatcher |
| PLAT-01 | Works with several replicas; shared pacing | All shared state (card, pacer, target lock) in Redis outside the `alita:cache:` prefix; Redis required for staff actions |
</phase_requirements>

## Summary

Phase 2 is almost entirely integration work on top of Phase 1. No new third-party package is needed, and no new migration is required (the card lives in Redis; the audit table is Phase 3). The new code is: a command interceptor ahead of the existing Bans/Mutes modules, a Redis-backed confirm card with an atomic state machine, a pure per-group decision function, a Redis pacer that shares one budget and one `retry_after` block across replicas, and a summary renderer/editor. The riskiest correctness item is the ROADMAP research flag, which is now resolved from the Bot API server source: `restrictChatMember` always sends a `restricted` status that **replaces** the previous status, so on a `kicked` target it silently converts the ban into a restriction, and on a `left` target it pre-restricts a non-member. `unbanChatMember` with `only_if_banned=false` is equally dangerous in the other direction (it turns `member`/`restricted` into `left`, i.e. kicks). The safe design is a status-first decision table (below) in which mute, kick and unmute are never sent to a `kicked` target, and unban always sends `only_if_banned=true`.

The most important structural finding for planning: the existing `/ban` etc. are registered by `Bans` (priority 70) and `Mutes` (priority 80) at handler group 0, and the Staff module is priority 236, so a Staff handler registered the normal way would never run first. Also the existing `restrictChecks` require the issuer to be an admin with restrict rights **in the chat the command was typed in**, which would wrongly refuse ordinary Staff Group members. The staff branch therefore has to be a separate pass-through handler set in a new module registered before priority 70, that returns `ext.ContinueGroups` untouched for every chat that is not a Staff Group. It cannot use `helpers.WrapCommand`, because `WrapCommand` calls `BuildCommandContext` first, which replies "cannot identify user" for sender-less messages and would double-reply in non-staff chats.

Telegram does not retry for us: gotgbot surfaces a 429 as `*gotgbot.TelegramError` with `ResponseParams.RetryAfter`, and the Bot API server emits exactly that on flood control. Pacing must therefore be explicit. The verified-on-miniredis design is a Lua "reserve the next slot" script plus a shared block key, giving a fleet-wide ceiling with no per-replica state.

**Primary recommendation:** Build the thinnest vertical tracer first (`/ban <numeric id>` in a Staff Group with one linked group -> Redis card -> Confirm -> live checks -> `banChatMember` -> edited summary), proving the interceptor ordering, card state machine and fan-out/summary seam. Then widen along the decision table, parser, pacer and renderer, each guarded by tests against an extended hand-written `staffBotClient`.

## Research Questions Answered (evidence)

### Q1. Research flag: what does `restrictChatMember` do to a banned or absent target?

**Finding (HIGH):** it replaces the target's status. A mute/unmute fan-out WOULD lift a ban by accident if it called `restrictChatMember` on a `kicked` target.

Evidence, from the Bot API server implementation (`tdlib/telegram-bot-api`, `telegram-bot-api/Client.cpp`, `master`, fetched 2026-10-05, `process_restrict_chat_member_query`, around lines 16522-16560) [VERIFIED: raw.githubusercontent.com/tdlib/telegram-bot-api/master/telegram-bot-api/Client.cpp]:

```cpp
get_chat_member(chat_id, user_id, std::move(query), [...](object_ptr<td_api::chatMember> &&chat_member, ...) {
  ...
  send_request(make_object<td_api::setChatMemberStatus>(
                   chat_id, make_object<td_api::messageSenderUser>(user_id),
                   make_object<td_api::chatMemberStatusRestricted>(
                       is_chat_member(chat_member->status_), until_date, std::move(permissions))),
               td::make_unique<TdOnOkQueryCallback>(std::move(query)));
```

and (around line 9044):

```cpp
bool Client::is_chat_member(const object_ptr<td_api::ChatMemberStatus> &status) {
  switch (status->get_id()) {
    case td_api::chatMemberStatusBanned::ID:
    case td_api::chatMemberStatusLeft::ID:
      return false;
    case td_api::chatMemberStatusRestricted::ID:
      return static_cast<const td_api::chatMemberStatusRestricted *>(status.get())->is_member_;
    default:
      // ignore Creator.is_member_
      return true;
  }
}
```

Reading: the server never branches on "banned"; it always issues `setChatMemberStatus(Restricted(is_member = <was a member>, until_date, permissions))`. A member has exactly one status, so for a `Banned` target the new status is `Restricted(is_member=false, ...)`: the ban record is gone and a restriction record exists instead. For a `Left` target it creates a restricted non-member (`is_member=false`), which matches the documented `ChatMemberRestricted.is_member` field: "True, if the user is a member of the chat at the moment of the request" [VERIFIED: gotgbot gen_types.go:2550-2551]. The server also rejects non-supergroups: `"Bad Request: method is available only in supergroups"` [VERIFIED: Client.cpp same function]. The independent web-search summary agreed (non-authoritative), and Phase 1 research had this flagged `[UNVERIFIED]` (`.planning/research/PITFALLS.md:109`).

Residual uncertainty: that tdlib's `setChatMemberStatus(Restricted)` clears a previous `Banned` is inferred from the status model (one status per member), not read from tdlib source. The design below is safe either way (it never sends the call to a `kicked` target), and a one-time live check with a throwaway group is cheap (see Open Questions).

Related server facts, same file [VERIFIED]:
- `unbanChatMember` with `only_if_banned=true`: fetches the member; if status is not Banned it returns `true` and does nothing; otherwise sets `Left`. With `only_if_banned=false` it sets `Left` unconditionally, i.e. it removes a current member (a kick) and also drops a restriction. Supergroups only: `"Bad Request: method is available only in supergroup and channel chats"`.
- `banChatMember` has no pre-check: it can ban any user ID (member, non-member, already banned) and simply overwrites `until_date`, which is why D-12/D-13 need the live status.
- Unknown users: `check_user_no_fail` proceeds even when the user is not known to the server, so a never-seen numeric ID reaches tdlib and may fail there with a Telegram error (exact text `[ASSUMED]`; classify as "Telegram error").
- `USER_ADMIN_INVALID` is mapped to `"Bad Request: user is an administrator of the chat"`; `MESSAGE_NOT_MODIFIED` to `"message is not modified: specified new message content and reply markup are exactly the same..."` [VERIFIED: Client.cpp fail_query_with_error].

**Safe order of checks and the decision table** (status comes from one live `getChatMember(group, target)`; `muted` means `restricted` with `can_send_messages=false`; `until=0` means permanent):

| Target status in group | ban | mute (timed or not) | kick | unban | unmute |
|---|---|---|---|---|---|
| `creator` / `administrator` | skip: target is an admin | skip | skip | skip | skip |
| `member` | `banChatMember` -> banned | `restrictChatMember(MutedPermissions)` -> muted | `unbanChatMember(only_if_banned=false)` -> kicked | skip: not banned | skip: not muted |
| `restricted`, `is_member=true`, not muted | ban (stricter) | apply mute | kick (clears the restriction, see Pitfall 9) | skip: not banned | skip: not muted |
| `restricted`, `is_member=true`, muted | ban (stricter) | apply only if new end is later (D-13), else skip: already muted | kick (clears mute, see Pitfall 9) | skip: not banned | `restrictChatMember(group defaults)` -> unmuted |
| `restricted`, `is_member=false` | ban -> "banned (not in group)" | skip: not in group | skip: not in group | skip: not banned | apply if muted, else skip: not muted |
| `left` | ban -> "banned (not in group)" | skip: not in group | skip: not in group | skip: not banned | skip: not muted |
| `kicked` | apply only if new end is later (D-13), else skip: already banned | **skip: already banned (NEVER restrict)** | **skip (NEVER unban(false))** | `unbanChatMember(only_if_banned=true)` -> unbanned | **skip: already banned (NEVER restrict)** |

Never-lift-a-ban guards, as invariants to test: (1) `restrictChatMember` is only ever sent when status is `member` or `restricted`; (2) `unbanChatMember(only_if_banned=false)` is only ever sent for the kick action to `member`/`restricted(is_member=true)`; (3) unban always uses `OnlyIfBanned: true`.

Open wording conflict for the planner: CONTEXT D-11 says a `kicked` target "counts as not in the group" (line "skipped: not in group") while the Discretion list says a mute on a banned user is "skipped: already banned". Recommend "skipped: already banned" for mute, kick and unmute on `kicked` targets (more informative, same outcome). See Open Questions.

### Q2. Rate limits, 429/`retry_after`, gotgbot surface, multi-replica pacing

- Documented guidance: avoid more than ~1 message/second in one chat (short bursts allowed, then 429), no more than 20 messages/minute in a group, and ~30 messages/second for bulk broadcast without paid broadcasts [CITED: core.telegram.org/bots/faq, via WebSearch summary 2026-10-05; the page itself is blocked by the sandbox egress proxy]. These numbers concern sending messages; there is no published number for `banChatMember`/`restrictChatMember`/`getChatMember`, but any API request may answer 429, so the contract (honor `retry_after`) applies to all of them [CITED: same summary].
- Server side: a 429 from Telegram becomes HTTP 429 with `parameters.retry_after` (`query->set_retry_after_error(retry_after_time)` in `fail_query_with_error`; a 429 with an unparsable message becomes a 500) [VERIFIED: Client.cpp lines ~74-80 of the error mapper].
- gotgbot: `ResponseParameters.RetryAfter int64` with comment "In case of exceeding flood control, the number of seconds left to wait before the request can be repeated" [VERIFIED: gotgbot gen_types.go:10683-10684]. A non-OK response becomes `*gotgbot.TelegramError{Method, Params, Code, Description, ResponseParams}` [VERIFIED: gotgbot request.go:61-72, 161-170]. **gotgbot does not retry**; the repo has no `RetryAfter` handling outside an unrelated AI-spam HTTP client (`grep -rn "RetryAfter"` over `alita`, `main.go`). Detect with `errors.As(err, &tgErr)`, `tgErr.Code == 429`, `tgErr.ResponseParams != nil && tgErr.ResponseParams.RetryAfter > 0`.
- The bot's HTTP client has a 30 s per-request timeout (`constants.LongTimeout = 30 * time.Second`, `main.go` `http.Client{Timeout: constants.LongTimeout}`), and `CheckOwner`/`FetchBotMember` apply an 8 s `liveCheckTimeout` [VERIFIED: owner.go:39]. A retry loop must carry its own `context` deadline.
- Multi-replica pacing (PLAT-01): per-replica limiters multiply the budget by the replica count and cannot share a `retry_after`. Use Redis: (a) a slot-reservation Lua script that hands each caller a wait time so that fleet-wide calls are at least `interval` apart, and (b) a shared block key set from every observed `retry_after`, which every replica's next reservation respects. Probed against miniredis 2.39 with `TIME` and `PTTL` inside the script (see Code Examples): four back-to-back reservations returned waits 0, 100, 199, 299 ms, and an active 5 s block made the next reservation return 5000 ms [VERIFIED: probe run 2026-10-05 in this session].
- Cost model for sizing: per linked group about 4 Bot API calls (`getChatAdministrators` via `recheckLink`, `getChatMember` issuer, `getChatMember` target, the action), plus one `getChat` only for unmute, plus one `FetchBotMember` only when a call fails. At 10 calls/s fleet-wide, 30 groups take about 12 s. Recommended starting values (planner may tune): interval 100 ms, 4 concurrent groups, up to 3 retries per call, single wait capped at 60 s (a longer `retry_after` fails the group as "rate limited, gave up after retries" and still sets the shared block).

### Q3. Edit-message limits for the single updating summary

Telegram publishes no number for `editMessageText`; the closest official figure is the per-chat ~1 message/second guidance above, and edits are widely reported to be throttled the same way [ASSUMED]. D-16's "at most one edit every 2-3 seconds" sits comfortably inside it. Design points:
- One coordinator goroutine owns the card's edits (a single writer), so throttling is local, not Redis.
- Treat Telegram's `message is not modified` as success; the repo already has `isMessageNotModified` (`staff_panel.go:466`).
- On a 429 for an edit, sleep `retry_after` and retry up to 3 times; the final edit must never be dropped. If the card cannot be edited (deleted, "message to edit not found", repeated failure), send the final summary as a new message in the Staff Group (`sendStaffNotice` already posts there). This resolves the first Discretion item.

### Q4. How the existing commands are registered, and where the Staff branch goes

[VERIFIED by Read]
- `bans.go:1108-1125`: `helpers.WrapCommand(dispatcher, banDesc, pipelineHandler(bansModule.ban))` and siblings (`sban tban dban unban kick dkick skick`), `bans.go:1129`: `RegisterLegacyModule("Bans", 70, LoadBans)`.
- `mute.go:294-298` and `mute.go:302`: `mute smute tmute dmute unmute`, `RegisterLegacyModule("Mutes", 80, LoadMutes)`.
- `staff.go:376`: `RegisterLegacyModule("Staff", 236, LoadStaff)`; `staff_watchers.go:192`: `RegisterLegacyModule("StaffWatchers", 237, LoadStaffWatchers)` at handler group -3.
- `registry.go:35-45`: modules load by ascending priority; `LoadAllModules` uses `slices.SortStableFunc`.
- `restrictChecks` (`bans.go:1057-1066`) = `CheckDisabled, RequireGroup, RequireUserAdmin, RequireBotAdmin, CanUserRestrict, CanBotRestrict`, all evaluated against the chat of the command. In a Staff Group a plain member fails `RequireUserAdmin`, and the bot may or may not be admin there. So the existing pipeline cannot serve staff commands.
- gotgbot dispatch: handlers within a group run in registration order; `ext.ContinueGroups` "Continue handling current group" (next handler in the same group); `ext.EndGroups` stops everything; returning `nil` ends only the current group [VERIFIED: gotgbot ext/dispatcher.go `iterateOverHandlerGroups`, handler_mapping.go `add` appends]. `handlers.NewCommand` ignores bots, edits and channel posts by default and matches `/cmd@bot` only for this bot [VERIFIED: ext/handlers/command.go].
- Anonymous admins: `bot_updates.go:207` re-enters through `HandleAnonymousAdmin` -> `anonPipelineHandler`, which bypasses group-0 interceptors. That path is only reached after the proof button, which `checkAnonAdmin` offers only from `RequireUserAdmin`. If the interceptor refuses anonymous senders first, that flow never starts in a Staff Group.

**Recommendation:** a new module `StaffActions`, `RegisterLegacyModule("StaffActions", 65, LoadStaffActions)` (no existing module uses 65; the grep of all `RegisterLegacyModule` calls shows priorities 12, 20, 30, 40, 50, 55, 60, 70, 80, ...), registering raw `handlers.NewCommand(name, h)` at group 0 for 13 names: staff actions `ban tban mute tmute kick unban unmute`, refused variants `sban dban skick dkick smute dmute`. Each handler:
1. Returns `ext.ContinueGroups` immediately, with no reply and no Telegram call, unless the chat is a group/supergroup and cached `staff.GetStaffGroup(chat.Id) != nil` (cached gate, keys already in `skipLocal`).
2. Inside a Staff Group: confirm with `staff.GetStaffGroupFresh`, refuse anonymous senders with `staff_post_as_yourself` (use `helpers.IsAnonymousSender`; the reply path is identical to `RejectAnonymousSender`), then run the staff flow and return `ext.EndGroups`.

Why not `WrapCommand`: `WrapCommand` runs `BuildCommandContext` before anything else, and that function replies `common_cannot_identify_user` when the sender is nil (`command_pipeline.go:23-30`). A pass-through `WrapCommand` registered ahead of Bans would add a second reply in every non-staff chat for those updates, violating STAFF-13. The docs generator only discovers `WrapCommand` registrations, which is fine here because these are not new commands. Add an AGENTS.md note for this exception.

### Q5. Callback data (64 bytes, `callbackcodec`) for issuer-bound Confirm/Cancel

[VERIFIED by Read, `callbackcodec.go`] Format `<ns>|v1|<url-encoded query>`, `MaxCallbackDataLen = 64`, `Encode` returns `ErrDataTooLong` on overflow and `encodeCallbackData` turns that into `""` (a dead button). Existing namespace `staff` (`staffCallbackNamespace = "staff"`) and actions `uy un ul uc ux rf pg` (`staff.go:45-53`); new actions `xc` (confirm) and `xn` (cancel) do not collide. Encoded size: `staff|v1|` (9) + `a=xc&t=` (7) + 16 hex chars = 32 bytes, well under 64. Never put the target, reason, duration or issuer ID in callback data: they live in the Redis card. The issuer binding is not in the callback data either; it is the stored `issuer` field compared with `query.From.Id` inside the atomic claim. `callback_data` is attacker-visible (any member can see and replay it), so the token only locates the card; authorization is the issuer comparison plus a chat check (`query.Message.GetChat().Id == card.staff_chat_id`, the same shape as `loadUnlinkTarget`, `staff_unlink.go:285-315`).

### Q6. `@username` resolution: what the bot can and cannot resolve

[VERIFIED by Read] `extraction.GetUserId` (`extraction.go:193-221`) resolves in this order: the users table (`GetUserIdByUserName`, exact-case `username = ?`, one row, `repository.go:118-129`), then the **channels** table, then a **live Telegram `getChat("@username")`**. All three are wrong for staff: it is case-sensitive, returns one arbitrary row on duplicates, can return a channel ID, and can contact Telegram. D-04 needs a new users-only lookup returning every match with name, ID and last activity.

What the bot can resolve is exactly what the users tracker has stored: senders, repliers' targets and forwarded-from users (`users.go` `logUsers`, which writes through `user.UpdateUser`; `models.User` has `UserId`, `UserName`, `Name`, `LastActivity`). A user the bot never saw, a user with hidden forward privacy, or a username that has since changed hands cannot be resolved correctly. `getChat` by `@username` is for public groups and channels, not private users, so there is no Telegram fallback to offer. A `text_mention` entity carries a user ID directly (no lookup), and a numeric ID needs none. The card's "resolved name and ID" display is the defence against a stale or reused username.

Note: `models.User.UserName` has a plain btree index (`idx_users_user_name`), so `LOWER(username) = LOWER(?)` scans. That is acceptable for this deployment size; an optional functional index migration (`CREATE INDEX ... ON users (lower(username))`, timestamp greater than `20261004130000`) can be added but is not required.

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Command intake and Staff Group gating | Bot process (dispatcher handlers) | PostgreSQL (staff tables, cached gate) | Ordering at group 0 decides whether staff or per-group logic runs |
| Target resolution (`@username`, ID, `text_mention`) | PostgreSQL (`users`) | Bot process (parser) | Only stored users can be resolved; Telegram cannot resolve private usernames |
| Confirm card state, issuer binding, expiry | Redis (operational hash, atomic Lua) | Bot process (callback handler) | Callback and command can land on different replicas |
| Per-group authority (issuer rights, target protection, owner match) | Telegram Bot API (live `getChatMember`/`getChatAdministrators`) | Bot process | Cached admin lists are never an authority |
| Per-group decision (done/skip/fail) | Bot process (pure function) | — | Pure and table-testable; the research-flag safety lives here |
| Pacing, shared `retry_after` | Redis (slot reservation, block key) | Bot process (retry wrapper) | One budget across replicas (PLAT-01) |
| Summary rendering and batched edits | Bot process (single coordinator) | Telegram Bot API (`editMessageText`) | Single writer keeps edit rate local |
| Link removal on owner mismatch | PostgreSQL (conditional DELETE) | Telegram (live `CheckOwner`) | Reuse `recheckLink`; only `OwnerMismatch` removes |

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `github.com/PaulSonOfLars/gotgbot/v2` | `v2.0.0-rc.36.0.20260919140833-240296efadb4` | Bot API client, dispatcher, `TelegramError` | Already the project stack [VERIFIED: go.mod:7] |
| `github.com/redis/go-redis/v9` | v9.22.0 | Card hash, pacer script, target lock | Already used with `redis.NewScript` in `antiraid.go:51-75` [VERIFIED: go.mod:17] |
| `golang.org/x/sync` (`errgroup`) | v0.23.0 | Bounded per-group worker pool | Already used in `staff_panel.go` `buildStaffPanelRows` with `SetLimit(4)` [VERIFIED: go.mod:26] |
| `gorm.io/gorm` | v1.31.2 | Case-insensitive username query | Existing repository pattern |
| `crypto/rand` (stdlib) | Go 1.26.0 | 64-bit card token | Unguessable tokens; no dependency |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `github.com/alicebob/miniredis/v2` | v2.39.0 | Tests for card, pacer, lock (Lua + `TIME`/`PTTL` work) | Every Redis-touching test [VERIFIED: go.mod:8, probe] |
| `github.com/stretchr/testify` | v1.12.1 | Assertions only | Tests (no mock libraries) |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| Slot-reservation Lua pacer | `golang.org/x/time/rate` per replica | Per-replica budgets multiply by replica count and cannot share `retry_after`; contradicts PLAT-01 |
| Redis hash + Lua CAS card | `GETDEL` of a JSON blob | Cannot distinguish expired / cancelled / running on a repeat tap, so a double-tap or late tap cannot be answered precisely and could clobber the summary |
| New interceptor module | Branching inside `Bans`/`Mutes` descriptors | Entangles staff logic with per-group commands, risks STAFF-13 |

**Installation:** none. `go.mod` is unchanged.

**Version verification:** versions above were read from `go.mod` this session; no package is added, so no registry query was needed.

## Package Legitimacy Audit

No external packages are installed by this phase (every library above is already in `go.mod` and in use). `gsd_run query package-legitimacy check` was therefore not run.

| Package | Registry | Age | Downloads | Source Repo | Verdict | Disposition |
|---------|----------|-----|-----------|-------------|---------|-------------|
| (none new) | — | — | — | — | — | — |

**Packages removed due to [SLOP] verdict:** none
**Packages flagged as suspicious [SUS]:** none

## Architecture Patterns

### System Architecture Diagram

```
Telegram update (Staff Group message "/ban @x 2d spam")
        |
        v
 dispatcher group 0, module order by priority
        |
        v
 StaffActions interceptor (priority 65, raw handlers.NewCommand)
   - chat not a Staff Group (cached gate) ------------------> ext.ContinueGroups ---> Bans(70)/Mutes(80) run exactly as today
   - Staff Group:
       anonymous sender? ---> reply "post as yourself" ---> EndGroups
       sban/dban/skick/dkick/smute/dmute? ---> refusal hint ---> EndGroups
       parse args (target, duration, reason) --- error ---> hint reply ---> EndGroups
       resolve target (ID / text_mention / users-table username, multi-match refusal)
       list links (fresh); zero links ---> hint
       create card in Redis (state=pending, TTL, token) ; send Confirm/Cancel card
        |
        v   (tap: callback staff|v1|a=xc&t=<token>, any replica)
 staffCallback (group 0)
   - staff group still valid? issuer still a member? query.From == card.issuer? chat matches?
   - Lua CAS pending -> running (exactly once) | cancelled | expired
   - answer callback; edit card -> header + all lines "pending"
        |
        v
 fan-out coordinator goroutine (joined to WaitGroup, panic-recovered)
   snapshot links (id ASC) -> errgroup SetLimit(4)
        per group (sequential inside a group, every Telegram call via pacer.Do):
           recheckLink (owner match; mismatch -> remove + report)
           getChatMember(group, issuer): creator | administrator && can_restrict_members
           target is bot / service ID -> skip (no call)
           getChatMember(group, target) -> decideStaffAction(action, state, newUntil)
           execute (ban | restrict | unban(only_if_banned) | kick) [unmute: getChat -> defaults]
           on 400/403 -> FetchBotMember probe -> classify (bot missing / not admin / lacks rights / group not found / Telegram error)
           result -> results channel
        Redis pacer: slot reservation + shared retry_after block (all replicas)
   coordinator: every 2.5 s if dirty -> edit summary ; final edit when all done ; fallback new message
        |
        v
 summary in the Staff Group: header, tally, one line per group (done / skipped / failed)
```

### Recommended Project Structure
```
alita/modules/
├── staff_action.go            # module registration (priority 65), interceptors, gate, anon/refusal paths
├── staff_action_parse.go      # pure: args -> target ref, duration, reason; caps; hints as typed errors
├── staff_action_card.go       # Redis card hash, token, Lua CAS state machine, expiry timer
├── staff_action_decide.go     # pure: decideStaffAction(action, targetState, newUntil) -> verdict
├── staff_action_run.go        # coordinator, per-group worker, error classification, shutdown drain
├── staff_action_summary.go    # pure renderer + batching editor + fallback delivery
├── staff_action_*_test.go     # //go:build testtools
alita/utils/ratelimit/
└── telegram_pacer.go          # Redis slot reservation + block key + Do(retry) wrapper (+ local fallback)
alita/db/user/repository.go    # + FindUsersByUsername (case-insensitive, all matches, last activity)
alita/utils/extraction/        # + exported pure ParseDurationToken (no replies)
locales/*.yml                  # staff_act_* keys, all 7 files
```

### Pattern 1: Pass-through interceptor, group 0, priority below 70
**What:** raw `handlers.NewCommand` handlers that touch nothing unless the chat is a Staff Group.
**When to use:** the 13 command names above.
**Example:**
```go
// Source: derived from gotgbot ext/dispatcher.go iterateOverHandlerGroups and alita/modules/registry.go
func staffEntry(act staffActionKind) func(*gotgbot.Bot, *ext.Context) error {
	return func(b *gotgbot.Bot, ctx *ext.Context) error {
		chat := ctx.EffectiveChat
		if chat == nil || (chat.Type != "group" && chat.Type != "supergroup") || staff.GetStaffGroup(chat.Id) == nil {
			return ext.ContinueGroups // zero side effects: STAFF-13
		}
		defer error_handling.RecoverFromPanic("staffEntry", "StaffActions")
		return staffModule2.run(b, ctx, act) // always ends with ext.EndGroups inside a Staff Group
	}
}
```

### Pattern 2: Redis card with an atomic state machine
**What:** hash `alita:staff:act:<token>` with fields `state` (`pending|running|done|cancelled|expired`), `issuer`, `staff_chat`, `action`, `target`, `target_name` (display only), `duration_s` (0 = permanent), `reason`, `group_count`, `expires_at`. A Lua script transitions `state` only from `pending`, and checks issuer and expiry inside the script, so two taps, or two replicas, can never both start a fan-out. Return codes distinguish: ok, wrong state (already handled), not issuer, expired.
**Why a hash and not `GETDEL`:** after Confirm the key must still say "running", and a late tap must be answered "already handled" without editing the message (an edit would overwrite the live summary).
**TTL:** `pending` cards live a little longer than `expires_at` (for example 5 min plus 1 min) so a late tap can still answer "expired"; terminal states shrink the TTL (for example 1 h). Keys sit outside `alita:cache:` so `CLEAR_CACHE_ON_STARTUP` keeps pending cards across a restart (AGENTS.md data rule).
**Expiry edit (D-07):** lazily on any tap after `expires_at`, plus a best-effort `time.AfterFunc(5m)` timer on the creating replica that does the same CAS `pending -> expired` and then edits the card (precedent: `replySelfDeleting`, `staff_notify.go:32-46`, with `RecoverFromPanic`). The CAS guarantees one edit across replicas; a restart that loses the timer is covered by the lazy path.
**Unique token:** `crypto/rand` 8 bytes, hex (16 chars). Treat a token collision on `HSETNX`-style create as retry.

### Pattern 3: Redis pacer plus bounded retry wrapper
**What:** one wrapper through which every fan-out Telegram call goes (including `getChatMember` and `getChatAdministrators` inside `recheckLink`, which needs a small seam, e.g. a context-carried pacer or a `pace func` parameter like `recheckStaffGroup` already has).
**Behavior:** `Reserve` (Lua) returns the ms to wait; sleep (context-aware); call; on 429 with `RetryAfter`, set the shared block key to `max(existing, retry_after)`, sleep `retry_after` (+ small jitter), retry up to N times, then return a typed rate-limit error. If Redis errors mid-run, fall back to a conservative local interval and keep going (log a warning) rather than failing groups. The cards need Redis anyway, so this fallback only covers a mid-run outage.
**Idempotence:** compute `until_date` once at Confirm time and reuse it for every group and every retry; ban, restrict and unban are idempotent for identical parameters.

### Pattern 4: Per-group authority order (fail closed)
1. Snapshot links once at Confirm (`ListLinksByStaffFresh`, `id ASC`). Re-check `staff.GetStaffGroupFresh`, issuer membership in the Staff Group (`chat_status.IsUserInChatWithError`, as `staffPanelRebuild` does) and that the link count still equals the card's `group_count`; if it changed, abort the card with "linked groups changed, run the command again".
2. `recheckLink(b, link, pass)` with one shared `staffOwnerPass`: `staffRecheckRemoved`/`staffRecheckGone` -> line "skipped: link removed (owner changed)", `staffRecheckUnknown` -> failed "could not verify the group owner". Only `OwnerMismatch` removes anything (Phase 1 rule).
3. Issuer: `getChatMember(group, issuer)`; accept only `creator`, or `administrator` with `CanRestrictMembers`. Anything else -> "skipped: you're not an admin there / lack the restrict right". Lookup error -> failed. Never `chat_status.IsUserAdmin` (returns true for `tgAdminList` IDs and reads the admin cache), never `CanUserRestrict` (reads `ctx.EffectiveChat`).
4. Target short-circuits with no Telegram call: target is the bot (`b.Id`) or one of the Telegram service IDs (777000, 1087968824) -> skipped.
5. Target `getChatMember` -> `decideStaffAction`.
6. Execute through the pacer. On any 400/403, probe `chat_status.FetchBotMember` once and classify: `BotMemberMissing` -> failed "group not found / bot not in group"; status not `administrator` -> "bot isn't an admin"; administrator without `CanRestrictMembers` -> "bot lacks ban rights"; otherwise "Telegram error: <description>". This avoids depending on exact error strings (`helpers.IsExpectedTelegramError` already shows the fragile substrings in use, `telegram_helpers.go:61-104`).

### Pattern 5: Panic-safe result accounting (no silent drops)
Pre-fill `results[i] = pending` for every linked group. Each worker is `group.Go(func() error { defer error_handling.RecoverFromPanic(...); ... })` exactly like `buildStaffPanelRows`. After `group.Wait()`, every slot still `pending` becomes `failed: internal error`, and the final edit lists it. On shutdown, cancel the context, let workers finish or abandon, and mark remaining slots `failed: interrupted by restart`. Re-running is safe because the decision table never shortens or lifts anything.

### Pattern 6: Summary rendering
Pure function over `(header, results)` returning HTML, never slicing HTML strings. Measure in UTF-16 units with the same cap as the panel (`staffPanelMaxUTF16 = 3800`, `staff_panel.go:40`) and reuse `staffDisplayTitle` for titles. Order: all lines; if too long, collapse done lines into one count line (D-18); if skipped plus failed lines alone still exceed the cap, send the overflow as continuation messages so no group is hidden (STAFF-08 outranks the "one message" ideal in that pathological case). User-controlled text (reason, names, titles) must be spliced in after translation using placeholder tokens, because the translator runs a printf-style pass (`staff.go:55-59` `staffGroupsToken`, `staff_link.go:34` `staffGroupTitleToken`).

### Anti-Patterns to Avoid
- **`helpers.WrapCommand` for the interceptor:** duplicates the nil-user reply in non-staff chats (see Q4).
- **`chat_status.IsUserAdmin` / `CanUserRestrict` / admin cache as the per-group gate:** not cross-group valid (CONTEXT Pitfall 1).
- **Calling `extraction.GetUserId` or `ExtractUserAndText` for staff targets:** channel and Telegram fallbacks, exact-case, UTF-16 slicing bugs (`msg.Text[ent.Offset+ent.Length:]` indexes bytes with UTF-16 units).
- **Unconditional `UnbanChatMember` (as `kickMember` does) or `RestrictMember` on a target of unknown status.**
- **User text in callback data**, or an unchecked `encodeCallbackData` result (a `""` result ships a dead button).
- **Looping `BanChatMember` with errors logged at Debug (the `applyActiveFban` model):** silent drops on 429.
- **Calling `actionlog.*` from the fan-out:** log-channel posts are STAFF-09, Phase 3.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Owner/link validity before acting | A new ownership check | `recheckLink` + `staffOwnerPass` (`staff_recheck.go`) | Already conditional-write, exactly-once notices, tri-state |
| Bot-rights probe | A new `getChatMember(bot)` parser | `chat_status.FetchBotMember` (`owner.go:71`) | Already tri-state, 8 s timeout, treats left/kicked as missing |
| Callback encode/decode | String concatenation or `strings.Split` | `encodeCallbackData` / `decodeCallbackData` (`callback_codec.go`) | 64-byte cap, versioning, AGENTS.md rule |
| Member-in-chat test | Status string switch | `chat_status.IsUserInChatWithError` | Handles `restricted` + `is_member` correctly |
| Posting into the Staff Group | Raw `SendMessage` + options | `sendStaffNotice`, `formatting.Shtml()` | Link previews off, reply-without-reply allowed |
| Message-not-modified | String matching in new code | `isMessageNotModified` (`staff_panel.go:466`) | Already case-insensitive |
| Title escaping/truncation | New HTML escape | `staffDisplayTitle` (64 runes + `html.EscapeString`) | Prevents HTML injection from group titles |
| UTF-16 entity slicing | `text[off:off+len]` | `extractEntityText` (`moderation_input.go:89`) | Entity offsets are UTF-16 (AGENTS.md) |
| Duration grammar | A second `m/h/d/w` parser | Export a pure `ParseDurationToken` from `extraction.parseTemporaryDuration` | One grammar; today it is unexported and replies on error |
| Redis scripts | Multi-step GET/SET races | `redis.NewScript` (`antiraid.go:51-75`) | Atomic across replicas; works in miniredis |
| Anonymous sender detection | New heuristics | `helpers.IsAnonymousSender` | Covers anon admin, channel identity, linked channel, 777000 |
| Bounded concurrency | Ad-hoc goroutine fan-out | `errgroup.Group` with `SetLimit` | Pattern already in `buildStaffPanelRows` |

**Key insight:** every hard part (authority, notices, callbacks, escaping, scripts) already has a tested Phase 1 or antiraid helper. The genuinely new logic is the decision table, the pacer, the card state machine and the renderer, and all four are pure or Redis-only and can be tested without Telegram.

## Runtime State Inventory

Not a rename/refactor/migration phase: omitted. (No stored data is renamed; the only persisted state is new Redis keys with TTLs.)

## Common Pitfalls

### Pitfall 1: Mute/unmute lifts a ban (the research flag)
**What goes wrong:** `restrictChatMember` on a `kicked` or `left` target replaces the status with a restriction (Q1).
**Why:** the server always sends `Restricted(is_member = was-a-member, ...)`.
**How to avoid:** status-first decision table; invariants (1)-(3) as tests with a stateful fake client that models the replacement semantics.
**Warning signs:** a test double that returns `true` for any `restrictChatMember` without modelling status transitions cannot catch this bug; the fake must change the target's stored status.

### Pitfall 2: Staff handler never runs, or runs for everyone
**What goes wrong:** registering staff commands in `LoadStaff` (236) puts them after Bans/Mutes, so `/ban` in a Staff Group hits `restrictChecks` instead; registering a handler that returns `nil` or `EndGroups` for non-staff chats kills the per-group command.
**How to avoid:** priority 65 module, `ext.ContinueGroups` for non-staff chats, and a real-dispatcher test that loads Bans, Mutes and StaffActions and sends the same `/ban` update to a normal group and a Staff Group.

### Pitfall 3: Non-staff behavior drift (STAFF-13)
**What goes wrong:** any reply, DB write or Telegram call in the pass-through path, or a changed `RequiredChecks`, alters behavior for every group.
**How to avoid:** the pass-through reads only the cached staff gate; no edits to `banDesc`/`muteDesc`; keep `RegisterAnonymousAdminHandler` entries untouched; regression tests for all five commands in a non-staff chat asserting identical Telegram calls.

### Pitfall 4: Confirm double-fire and non-issuer taps
**What goes wrong:** two taps or two replicas start two fan-outs; another staff member confirms.
**How to avoid:** Lua CAS with issuer check inside the script; answer the callback exactly once; a repeat tap answers "already handled" and never edits.

### Pitfall 5: A group left as "pending" forever
**What goes wrong:** a recovered worker panic, a crash, or a restart leaves a line at the pending marker, which is a silent drop.
**How to avoid:** Pattern 5 (post-wait sweep, shutdown finalization); register the drain after the DB-close handler (AGENTS.md: shutdown is LIFO, new drains registered after DB-close run before it), as `StopStaffSweeper` does in `main.go`. Crash-level loss is a known limit (Open Questions).

### Pitfall 6: 429 mistaken for a generic failure
**What goes wrong:** the group is reported failed on the first flood-control reply, or retries hammer Telegram.
**How to avoid:** the pacer, shared block key, bounded retries, `retry_after` cap; test with a fake that returns 429 `retry_after=2` N times then OK, and one that always returns 429.

### Pitfall 7: Translator printf pass and HTML injection
**What goes wrong:** a reason like `100%d` or a group title containing `<` corrupts the message or breaks Telegram's parser.
**How to avoid:** placeholder tokens spliced after translation, `html.EscapeString` for every user-controlled value, never build HTML by slicing.

### Pitfall 8: Entity offsets and text-mention parsing
**What goes wrong:** names with non-ASCII characters shift `text_mention` offsets; the reason is cut at the wrong byte.
**How to avoid:** own parser built on `extractEntityText` and UTF-16-aware offsets; match against `Entities` and `CaptionEntities` (command text via `msg.GetText()`, which prefers the caption).

### Pitfall 9: Kick clears a mute; kick vs D-12 wording
**What goes wrong:** `unbanChatMember(only_if_banned=false)` sets `Left`, which removes the member and also drops their restriction record, so a kicked, previously muted user can rejoin unmuted. D-12 says an action "never shortens an existing ban or mute".
**Recommendation:** treat D-12 as covering ban/mute actions only (a kick has no end date), keep kick consistent with the existing per-group `/kick`, and surface the choice to the owner (Assumptions A6, Open Question 1).

### Pitfall 10: Over-limit durations
**What goes wrong:** CONTEXT says durations beyond 366 days are "treated as permanent", but today `parseTemporaryDuration` returns `errTimeLimitExceeded` and replies "time limit exceeded" (`extraction.go:366-373`, `TemporaryUntilDate` returns `(0,false)` outside 30 s..366 d, constants `minTemporaryDurationSeconds int64 = 30`, `maxTemporaryDurationSeconds int64 = 366 * 24 * 60 * 60`). The staff parser must convert over-limit to permanent itself, and the card must say "permanent". Never pass a raw clamp to Telegram (it treats <30 s or >366 d as forever anyway, which would hide a misparse).

### Pitfall 11: Race between target lookup and action
**What goes wrong:** between `getChatMember(target)` and `restrictChatMember`, another actor (a local admin, another staff action) bans the target; the restrict then replaces the ban.
**How to avoid / mitigate:** a per-target Redis lock `alita:staff:lock:target:<id>` held for the fan-out (SET NX with a safety TTL, compare-and-delete on release) refuses a second concurrent staff action on the same target ("another staff action on this target is running"). This also settles the second Discretion item without relying only on convergence. A local admin racing within milliseconds remains a tiny accepted window (document it).

### Pitfall 12: Basic groups and non-supergroups
`restrictChatMember` and `unbanChatMember` fail with `method is available only in supergroups` (server messages above). Phase 1 refuses linking basic groups (`staff_link_refuse_not_supergroup`), but a link can outlive a downgrade or odd state; classify these errors, do not retry.

### Pitfall 13: Recheck and probe call volume
`recheckLink` makes one `getChatAdministrators` per group (the Staff Group's answer is shared by `staffOwnerPass`); the 8 s `liveCheckTimeout` applies per call, so one very slow group can take tens of seconds. Keep workers bounded and per-group work off the dispatcher goroutine.

### Pitfall 14: Test fake too weak
`staffBotClient` (`staff_helpers_test.go`) answers `getChatMember` from a static per-chat string and its restricted/kicked JSON has no `until_date` or `can_send_messages`; failures are static per `method:chat`. Phase 2 needs a stateful fake (see Validation Architecture, Wave 0).

## Code Examples

### Slot-reservation pacer script (probed on miniredis 2.39)
```go
// Source: probe run in this research session against miniredis v2.39.0 (waits 0, 100, 199, 299 ms; 5000 ms under an active block)
var reserveSlotScript = redis.NewScript(`
local t = redis.call('TIME')
local now = tonumber(t[1]) * 1000 + math.floor(tonumber(t[2]) / 1000)
local nxt = tonumber(redis.call('GET', KEYS[1]) or '0')
local blk = redis.call('PTTL', KEYS[2])
local slot = math.max(now, nxt)
if blk > 0 then slot = math.max(slot, now + blk) end
redis.call('SET', KEYS[1], slot + tonumber(ARGV[1]), 'PX', ARGV[2])
return slot - now
`)
// KEYS[1] "alita:staff:pace:next", KEYS[2] "alita:staff:pace:block"; ARGV[1] interval ms, ARGV[2] key TTL ms.
// Setting the block from a 429: SET block 1 PX <retry_after*1000> only when it would extend the current PTTL.
```

### Retry wrapper shape
```go
// Source: gotgbot request.go TelegramError + ResponseParameters.RetryAfter; own design
func retryAfter(err error) (time.Duration, bool) {
	var tgErr *gotgbot.TelegramError
	if errors.As(err, &tgErr) && tgErr.Code == 429 && tgErr.ResponseParams != nil && tgErr.ResponseParams.RetryAfter > 0 {
		return time.Duration(tgErr.ResponseParams.RetryAfter) * time.Second, true
	}
	return 0, false
}

func (p *TelegramPacer) Do(ctx context.Context, call func(context.Context) error) error {
	for attempt := 0; ; attempt++ {
		if err := p.wait(ctx); err != nil { // reserve slot, honour shared block, bounded by maxWait
			return err
		}
		err := call(ctx)
		wait, limited := retryAfter(err)
		if !limited {
			return err
		}
		p.block(ctx, wait) // extend the shared block for every replica
		if attempt >= p.maxRetries {
			return fmt.Errorf("%w: %v", ErrRateLimited, err)
		}
	}
}
```

### Decision function skeleton (pure)
```go
// Source: decision table in Q1; status values from gotgbot gen_consts.go:222-227
// ChatMemberStatusCreator = "creator", ChatMemberStatusAdministrator = "administrator", ChatMemberStatusMember = "member",
// ChatMemberStatusRestricted = "restricted", ChatMemberStatusLeft = "left", ChatMemberStatusKicked = "kicked"
type targetState struct {
	Status   string // gotgbot.ChatMemberStatus*
	IsMember bool   // restricted only
	Muted    bool   // restricted && !CanSendMessages
	Until    int64  // until_date, 0 = permanent
}

func endsLater(newUntil, curUntil int64) bool { // 0 means permanent
	switch {
	case curUntil == 0:
		return false
	case newUntil == 0:
		return true
	default:
		return newUntil > curUntil
	}
}
// decideStaffAction(action, state, newUntil) returns (verdict{Apply|Skip|Fail}, reasonCode, apiCall).
// restrictChatMember is only reachable when Status is member or restricted.
```

### Issuer-bound confirm button
```go
// Source: alita/modules/staff.go (staffCallbackNamespace = "staff"; encodeCallbackData in callback_codec.go)
const (
	staffActActionConfirm = "xc" // new; existing: uy un ul uc ux rf pg
	staffActActionCancel  = "xn"
)
confirm := encodeCallbackData(staffCallbackNamespace, map[string]string{"a": staffActActionConfirm, "t": token})
// "staff|v1|a=xc&t=<16 hex>" = 32 bytes; an empty result means overflow and must refuse to send the card.
```

### Username lookup (new repository function)
```go
// Source: pattern of alita/db/user/repository.go GetUserIdByUserName (Read this session); own design
func FindUsersByUsername(username string, limit int) ([]models.User, error) {
	var rows []models.User
	err := db.DB.Where("LOWER(username) = LOWER(?)", username).
		Order("last_activity DESC").Limit(limit).Find(&rows).Error
	return rows, err // caller: 0 rows -> "never seen, use the numeric ID"; >1 rows -> list name, ID, last_activity, ask for the ID
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| `restrictChatMember` with full permissions as "unmute" | Treat it as a status replacement; guard by status | n/a (server behavior) | Never send to `kicked`/`left` |
| `kickChatMember` | `banChatMember` (server keeps `kickchatmember` as an alias, Client.cpp:374-375) | Bot API 5.3 era | Use `BanChatMember` |
| Per-replica token bucket | Redis shared reservation + block key | Roadmap decision (STATE.md) | Matches PLAT-01 |

**Deprecated/outdated:** reading admin status from the 30-minute admin cache for cross-group authority (rejected in Phase 1 research).

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | Edits count against the ~1 message/second per chat guidance; no official number exists for `editMessageText` | Q3 | Low: 2-3 s batching plus 429 retry covers it |
| A2 | Staff actions should refuse (fail closed) when Redis is unavailable, rather than degrade to in-process state | Pattern 2 | Medium: owner may prefer a degraded mode; without Redis cards cannot be shared across replicas |
| A3 | `setChatMemberStatus(Restricted)` fully clears a prior `Banned` record (inferred from the one-status-per-member model; the server source proves only what is sent) | Q1 | Low: design never sends restrict to `kicked`; confirm once with a throwaway group |
| A4 | Unknown-user and exact error strings from tdlib (for example for a never-seen numeric ID) | Q1, Pattern 4 | Low: classification uses a live bot probe plus "Telegram error: <text>", not string matching |
| A5 | Pacer defaults: 100 ms interval, 4 workers, 3 retries, 60 s max single wait | Q2 | Low: tunable constants; too aggressive shows as 429 retries, too slow lengthens runs |
| A6 | A staff kick over a muted target clears the mute (accepted, consistent with per-group `/kick`) | Pitfall 9 | Medium: may conflict with the owner's reading of D-12 |
| A7 | A numeric ID unknown to the users table is accepted, with the card showing the ID and "name unknown" | Q6 | Low: staff may expect refusal; only `@username` refusal is decided (D-04) |
| A8 | Abort the card at Confirm if the linked-group count changed since the card was shown | Pattern 4 | Low: strictness only; alternative is to act on the new list |
| A9 | A per-target Redis lock for the duration of a fan-out is worth adding | Pitfall 11 | Low: small code; skipping it leaves a ms-wide race |
| A10 | Telegram flood numbers (1/s per chat, 20/min per group, ~30/s bulk) | Q2 | Low: sourced from a summary of the official FAQ, not the page itself |
| A11 | Omitting `reply_markup` on `editMessageText` removes the inline keyboard (the repo's `editStaffCallbackMessage` comment relies on it) | Pattern 2, 6 | Low: verify in tests with the fake client's recorded edit |

## Open Questions

1. **Kick on a muted target vs D-12**
   - Known: `unbanChatMember(only_if_banned=false)` sets `Left`, dropping the restriction; D-12 says actions never shorten a mute.
   - Unclear: whether the owner means that to include kick.
   - Recommendation: apply the kick (matches `/kick` today), document it on the card help text, list it in the discuss-phase confirmation.

2. **Wording for mute/kick/unmute on a `kicked` target**
   - Known: D-11 says "skipped: not in group"; the Discretion list says "skipped: already banned".
   - Recommendation: "skipped: already banned" for all three; planner confirms.

3. **Redis absent**
   - Known: antiraid does nothing without Redis; PLAT-01 requires shared state.
   - Recommendation: refuse the command with a translated message when `cache.GetRedisClient()` is nil.

4. **Crash mid-run**
   - Known: a process crash leaves unfinished lines as the pending marker; graceful shutdown can finalize them.
   - Recommendation: accept for Phase 2; re-running is idempotent. Phase 3's DB audit record could back a resume later.

5. **Over-limit durations**
   - Known: CONTEXT says treat as permanent; existing parser refuses.
   - Recommendation: staff-only conversion to permanent, shown on the card as "permanent (longer than 366 days)".

6. **Live confirmation of the research flag**
   - Recommendation: add one deferred UAT item (throwaway supergroup: ban a test account, send `restrictChatMember` via a scratch script, read `getChatMember`) so A3 becomes observed. Not gating, because the design never relies on it.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go toolchain | build and tests | yes | go1.26.0 | none needed |
| gcc (cgo for go-sqlite3 tests) | `-tags testtools` tests | yes | /usr/bin/gcc | none needed |
| miniredis | Redis tests (card, pacer, lock) | yes (module) | v2.39.0 | none needed |
| redis-server | optional local run | binary present, not running | 7.0.15 | miniredis in tests |
| PostgreSQL | only Postgres-tagged integrity tests | not running (psql present) | — | SQLite via `internal/testdb.Run`; Phase 2 has no migration |
| Docker | `make build` only | present | — | `CGO_ENABLED=0 go build ./...` |
| Outbound web to core.telegram.org | doc lookups | blocked by egress proxy | — | gotgbot embedded spec comments plus tdlib Bot API server source (reachable) |

**Missing dependencies with no fallback:** none.
**Missing dependencies with fallback:** live Telegram (no bot token in the sandbox): all Telegram behavior is exercised through the hand-written fake `BotClient`; live UAT stays deferred as in Phase 1.

A single existing test ran in the sandbox: `CGO_ENABLED=1 go test -tags testtools -count=1 -run TestStaffSweepTracer ./alita/modules` returned `ok` in 13.5 s wall (first compile). `01-SKELETON.md` records that `make test` and `make lint` fail in the sandbox for toolchain reasons, so use targeted `go test -tags testtools -race -count=1 -run ...` here.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go `testing` + `testify` assertions; real fixtures (SQLite via `internal/testdb.Run`, miniredis, hand-written `gotgbot.BotClient` fakes) |
| Config file | none; every test file starts with `//go:build testtools` |
| Quick run command | `go test -tags testtools -race -count=1 -run '^TestStaffAction' ./alita/modules` |
| Full suite command | `make test` (CI/local with toolchain); sandbox: per-package `go test -tags testtools -race -count=1 ./alita/modules ./alita/utils/ratelimit ./alita/utils/extraction ./alita/db/user ./alita/i18n` |

### Phase Requirements -> Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| STAFF-01 | Each of the 7 commands in a Staff Group reaches the staff flow; numeric ID, `@username`, `text_mention` | unit + dispatcher | `go test -tags testtools -race -count=1 -run 'TestStaffActionParse\|TestStaffActionDispatch' ./alita/modules` | Wave 0 |
| STAFF-01 | Timed grammar (`m h d w`), over-limit -> permanent, `/tban`/`/tmute` require duration | unit | `go test -tags testtools -count=1 ./alita/utils/extraction` | Wave 0 |
| STAFF-01 | Unknown `@username` refused; multi-match lists all; case-insensitive | repo + unit | `go test -tags testtools -count=1 -run TestFindUsersByUsername ./alita/db/user` | Wave 0 |
| STAFF-02 | Reason kept, capped, HTML-escaped on card and summary | unit | `-run TestStaffActionReason` | Wave 0 |
| STAFF-03 | Card shows name, ID, action, duration, reason, count; nothing applied before Confirm; non-issuer tap refused; expiry; cancel | unit + miniredis | `-run 'TestStaffActionCard'` | Wave 0 |
| STAFF-04 | Never acts on the Staff Group; one call set per linked group | unit | `-run TestStaffActionFanOut` | Wave 0 |
| STAFF-05 | Issuer not admin / lacks restrict -> skipped, no write call recorded | unit | `-run TestStaffActionIssuerRights` | Wave 0 |
| STAFF-06 | Anonymous admin in a Staff Group: reply "post as yourself", no card, no anon-proof keyboard, no Telegram write | dispatcher | `-run TestStaffActionAnonymous` | Wave 0 |
| STAFF-07 | Target admin/creator/bot/service ID skipped with reason | unit | `-run TestStaffActionTargetProtection` | Wave 0 |
| STAFF-08 | Every group ends done/skipped/failed; panic-injected worker still reported; edit failure falls back to new message; collapse and overflow | unit | `-run 'TestStaffActionSummary\|TestStaffActionNoSilentDrop'` | Wave 0 |
| STAFF-12 | 429 with `retry_after` retried then succeeds; permanent 429 -> "rate limited, gave up after retries"; shared block honoured by a second pacer instance | unit + miniredis | `go test -tags testtools -race -count=1 ./alita/utils/ratelimit` | Wave 0 |
| STAFF-13 | The five commands behave identically in a non-staff group (same Telegram calls) with Bans, Mutes and StaffActions all loaded | dispatcher | `-run TestStaffActionNonStaffUnchanged` | Wave 0 |
| PLAT-01 | Two pacer instances on one miniredis keep fleet spacing; card CAS exactly-once across two handlers | miniredis | `-run 'TestStaffActionPacerFleet\|TestStaffActionConfirmOnce'` | Wave 0 |
| Research flag | Decision table: restrict never sent to `kicked`/`left`; `unban(false)` only for kick on members; unban always `only_if_banned=true`; stateful fake proves no ban is lifted | table-driven | `-run TestStaffActionDecisionTable` | Wave 0 |
| PLAT-03 | New `staff_act_*` keys exist in all 7 locales with equal placeholders | unit | `go test -tags testtools -count=1 -run TestStaffLocaleKeys ./alita/i18n` | exists (covers `staff_` prefix automatically) |

### Sampling Rate
- **Per task commit:** the quick run command for the touched package.
- **Per wave merge:** all five packages listed under the sandbox full-suite command.
- **Phase gate:** `make test`, `make lint`, `make check-translations`, `make check-docs` (and `make generate-docs` if `staff_help_msg` changes) green before `/gsd-verify-work`.

### Wave 0 Gaps
- [ ] Extend or add a stateful fake (new file, e.g. `alita/modules/staff_action_fake_test.go`): per-`(chat,user)` member records with `status`, `until_date`, `is_member`, `can_send_messages`, administrator `can_restrict_members`; apply `banChatMember`/`restrictChatMember`/`unbanChatMember` as status transitions that mirror the server semantics in Q1 (replacement, `only_if_banned`); per-call scripted error sequences (429 N times then OK; always 429); `editMessageText` failure and `not modified` injection; `getChat` returning group default permissions; call-order recording.
- [ ] miniredis helper for staff-action tests (`cache.SetRedisClientForTest` exists).
- [ ] `extraction.ParseDurationToken` plus tests.
- [ ] `user.FindUsersByUsername` plus tests in `alita/db/user`.
- [ ] A real-dispatcher harness test loading `LoadBans`, `LoadMutes` and `LoadStaffActions` in registry order.
- [ ] Framework install: none.

## Security Domain

`security_enforcement` is enabled (absent in `.planning/config.json`'s override means enabled; value `true`, ASVS level 1, block on high).

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | yes (identity = Telegram sender) | Real user ID from `EffectiveSender.User`; refuse anonymous/channel/service senders |
| V3 Session Management | yes (the card is a short session) | Unguessable `crypto/rand` token, 5-minute expiry, single-use atomic state, bound to issuer and chat |
| V4 Access Control | yes (core value) | Live per-group issuer check (`creator` or `administrator`+`can_restrict_members`); live target protection; no cached admin lists; issuer-only Confirm/Cancel |
| V5 Input Validation | yes | Strict parser, ID range checks, username regex `^[A-Za-z0-9_]{5,32}$`, reason length cap, HTML escape on output, callback data only carries a token |
| V6 Cryptography | yes (token generation only) | `crypto/rand`, no hand-rolled primitives |
| V7 Error Handling and Logging | yes | Log issuer/target/action/outcome counts at Info (audit record is Phase 3); never log tokens as secrets are not involved; no `ext.EndGroups` logging as error |
| V11 Business Logic | yes | Atomic CAS, never-shorten rule, per-target lock, TOCTOU re-checks per group |

### Known Threat Patterns for this stack

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Another staff member (or any chat member) taps Confirm/Cancel | Spoofing / EoP | `query.From.Id == card.issuer` checked inside the Lua CAS; chat equals `card.staff_chat` |
| Replayed or forged callback data | Spoofing / Tampering | 64-bit random token, server-side state, single-use transition, no data in callback |
| Acting in a group where the issuer is not an admin (the core invariant) | EoP | Live `getChatMember(group, issuer)` per group; link owner recheck; fail closed on lookup errors |
| Acting on a group admin, owner or the bot | EoP / Tampering | Live target status check; skip with reason; bot ID and 777000/1087968824 short-circuit |
| Username reuse or look-alike leading to the wrong target | Spoofing | Card shows resolved name and ID; multi-match refusal; unseen username refused |
| HTML/printf injection via reason, names, titles | Tampering | `html.EscapeString`, token splice after translation, never slice HTML |
| Confirm race / double-fire | Tampering | Atomic state machine; per-target lock |
| Flood and cost amplification by spamming commands | DoS | Shared pacer, worker cap, optional per-issuer cooldown (`SET NX PX`), one in-flight run per target |
| Stale Staff Group authority at Confirm time | EoP | Re-run `GetStaffGroupFresh` and issuer membership at Confirm |
| Anonymous admin bypassing per-group checks | Spoofing | Refuse before any lookup (`staff_post_as_yourself`) |

## Project Constraints (from CLAUDE.md)

Extracted from `./.claude/CLAUDE.md` and the imported `AGENTS.md` (binding):
- Stay on Go 1.26.0, gotgbot v2, GORM/PostgreSQL (SQLite in tests), Redis; no new stack. Production builds `CGO_ENABLED=0`, tests need `CGO_ENABLED=1`.
- Commands go through `helpers.WrapCommand`; **this phase needs a documented exception** for the pass-through interceptor (justified in Q4) and AGENTS.md must be updated in the same commit.
- Handler groups: staff watchers sit at `-3`; group-0 order is set by module priority; watchers return `ext.ContinueGroups`, commands `ext.EndGroups`; a group-0 handler returning `nil` silently kills later group-0 handlers.
- Callbacks only through `alita/utils/callbackcodec` (64-byte cap); `encodeCallbackData` returns `""` on overflow (dead button); put user text in Redis behind a short token.
- Permissions: `chat_status` predicates return bools and never reply; `IsUserAdmin` returns false for channel and non-positive IDs; never pass a chat ID as a user ID; `*ForUpdate` predicates are memoised per update, so watchers only.
- Data: every write `cache.DeleteCache`s the keys it affects; operational Redis keys (`alita:staff:*`) sit outside the `alita:cache:` prefix; `UpdateRecord` skips zero values. Phase 2 adds no DB write, so no cache invalidation is needed beyond what `recheckLink`'s removal already does.
- Migrations append-only, timestamped, one transaction per file; none required here.
- i18n: every key in all 7 locale files; use literal `tr.GetString("staff_act_...")` calls so `make check-translations` sees them; copy key names from `locales/en.yml`; a missing key returns `""`, so guard empty button labels (an empty label makes Telegram reject the whole message).
- Go rules: never discard a DB error on a state-changing path; every fire-and-forget goroutine starts with `defer error_handling.RecoverFromPanic(...)`; async writers join a drained WaitGroup; shutdown drains register after DB-close; register new secrets with `logredact.RegisterSecret` (none introduced here).
- Subsystem traps: Staff links use `staff.*Fresh` plus live `chat_status.CheckOwner`; only `OwnerMismatch` removes a link; exactly-once notices come from `RowsAffected == 1`; the staff sweeper and Staff tables are untouched.
- Testing: always `-tags testtools`; real fixtures, no mock libraries; assert observable behavior (reply sent, call made or not made, row persisted, gate enforced), never literals or test-double internals; `make test` is the only valid full run.
- Commits: Conventional Commits; user-visible changes need `feat:`/`fix:`.
- GSD: file-changing work starts through a GSD command.

## Recommended MVP Slicing (mode: mvp, tracer-first)

**Thinnest end-to-end slice (the tracer):** a Staff Group member sends `/ban 123456789` (numeric ID only, no duration, no reason) with exactly one linked group; the bot shows the confirm card; the issuer taps Confirm; the bot rechecks the link, checks the issuer live, checks the target live, calls `banChatMember`, and edits the card into a summary with one done line.

Layers it crosses, each in its simplest form:
1. **Routing:** new `StaffActions` module at priority 65; the `ban` interceptor only; `ContinueGroups` pass-through proven by a test where a non-staff group still gets the old reply.
2. **Gate and identity:** cached `GetStaffGroup`, anonymous refusal.
3. **Parser:** numeric ID only.
4. **Card:** Redis hash, token, Lua CAS `pending -> running`, issuer check; callback actions `xc`/`xn` added to `staffCallback`.
5. **Engine:** snapshot links, `recheckLink`, issuer `getChatMember`, target `getChatMember`, the ban branch of the decision table, one call, a trivial synchronous (single worker) path, no pacer yet.
6. **Summary:** header plus one line, one edit.
7. **i18n:** the minimal `staff_act_*` keys in 7 locales.
8. **Tests:** the stateful fake with `getChatMember`, `banChatMember`, `editMessageText`.

Then widen in dependency order (each independently valuable and testable): (a) full decision table and all five actions with the research-flag invariants; (b) parser breadth (username multi-match, `text_mention`, durations, reasons, refused variants, bare-reply hint); (c) pacer, 429 retry, worker pool; (d) summary tally, ordering, collapse, batching, fallback delivery, overflow; (e) card lifecycle (cancel, 5-minute expiry with timer, double tap, count-changed abort) and per-target lock; (f) STAFF-13 and anonymous regression suites; (g) AGENTS.md update, `make generate-docs`/`check-docs`, help text.

## Sources

### Primary (HIGH confidence)
- Bot API server implementation, `tdlib/telegram-bot-api` `telegram-bot-api/Client.cpp` on `master`, fetched 2026-10-05 via raw.githubusercontent.com: `process_restrict_chat_member_query`, `is_chat_member`, `process_unban_chat_member_query`, `process_ban_chat_member_query`, `fail_query_with_error` (429 -> `retry_after`, `USER_ADMIN_INVALID`, `MESSAGE_NOT_MODIFIED` mappings), `check_user_no_fail`.
- gotgbot v2 source in the module cache (`@v2.0.0-rc.36.0.20260919140833-240296efadb4`): `request.go` (`TelegramError`), `gen_types.go` (`ResponseParameters`, `ChatMemberBanned/Restricted/Left`, `MergedChatMember`), `gen_methods.go` (method descriptions with the official text), `gen_consts.go` (status constants), `ext/dispatcher.go` and `ext/handler_mapping.go` (group iteration), `ext/handlers/command.go`.
- Repository files read this session: `AGENTS.md`, `.claude/CLAUDE.md`, `02-CONTEXT.md`, `REQUIREMENTS.md`, `STATE.md`, `alita/modules/bans.go`, `mute.go`, `moderation.go`, `moderation_input.go`, `staff.go`, `staff_watchers.go`, `staff_recheck.go`, `staff_notify.go`, `staff_sweeper.go`, `staff_panel.go`, `staff_unlink.go`, `bot_updates.go`, `anonymous_admin_router.go`, `registry.go`, `alita/utils/helpers/command_pipeline.go`, `telegram_helpers.go`, `alita/utils/chat_status/owner.go`, `chat_status.go`, `alita/utils/extraction/extraction.go`, `alita/utils/callbackcodec/callbackcodec.go`, `alita/db/user/repository.go`, `alita/db/staff/repository.go`, `alita/modules/staff_helpers_test.go`, `alita/i18n/staff_locale_test.go`, `main.go`.
- Probe: slot-reservation Lua script run against miniredis v2.39.0 in this session.

### Secondary (MEDIUM confidence)
- Telegram Bot FAQ flood guidance (1 msg/s per chat, 20/min per group, ~30/s bulk, 429 on any request), quoted from a WebSearch summary of https://core.telegram.org/bots/faq (page blocked by the sandbox proxy), consistent with `.planning/research/PITFALLS.md` lines 135 and 750.
- `.planning/research/PITFALLS.md`, `ARCHITECTURE.md`: pre-roadmap research; used for the recorded `[UNVERIFIED]` flag now resolved.

### Tertiary (LOW confidence)
- Web-search summary that `restrictChatMember` on a banned user converts them to restricted (non-authoritative; superseded by the server source above).
- Edit-rate behavior of `editMessageText` (A1).

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH, no new packages; all versions read from `go.mod`.
- Architecture: HIGH, interceptor ordering, `WrapCommand` constraint and dispatcher semantics read from source in this session.
- Research-flag semantics: HIGH on what the server sends; MEDIUM-HIGH on the tdlib last inch (A3), mitigated by design.
- Rate limits: MEDIUM, numbers via secondary retrieval of the official FAQ; contract (429 + `retry_after`) verified in server and gotgbot source.
- Pitfalls: HIGH for code-derived ones, MEDIUM for race and edit-rate notes.

**Research date:** 2026-10-05
**Valid until:** 2026-11-04 (30 days; the Bot API and gotgbot pins move slowly, re-check `restrictChatMember` text if Bot API 10.x notes change it)
