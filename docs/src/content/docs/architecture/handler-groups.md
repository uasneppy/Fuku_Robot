---
title: Handler Group Precedence
description: Which message watchers fire first and how propagation works in Alita Robot.
---

# Handler Group Precedence

gotgbot dispatches incoming updates to handlers in ascending group number order. Lower group numbers fire first. Within the same group, handlers fire in registration order. A handler returning `ext.EndGroups` stops all further group processing for that update. `ext.ContinueGroups` allows downstream groups to continue firing.

This page documents every registered message watcher with its exact handler group number, verified from source code.

## Precedence Table

| Group | Module | Handler | Filter | Return on Match | Notes |
|-------|--------|---------|--------|-----------------|-------|
| -10 | Captcha | `handlePendingCaptchaMessage` | nil (all messages) | `EndGroups` | Intercepts messages from users with pending captcha; stores and deletes message |
| -7 | Lockdown | `lockdownOnJoinMember`, `lockdownOnJoinMessage`, `lockdownOnKicked`, `lockdownOnJoinRequest` | ChatMember (joined or kicked), `new_chat_members` service message, join request | `EndGroups` when a confirmed lockdown handled the update, `ContinueGroups` otherwise | The lockdown join guard. Records each join in the 10-minute tracker and counts it for the join surge trigger first, then reads the lockdown fresh; during a lockdown it records the joiner and ends the update so no welcome or captcha follows |
| -4 | Lockdown | `raidFloodWatcher` | `message.All` | `ContinueGroups` (always) | The raid flood counter. Counts messages and media for the flood trigger, never deletes anything, and lets aispam, antiflood, locks and blacklists still see every message |
| -1 | BotUpdates | `botJoinedGroup` | MyChatMember (bot joined) | `EndGroups` | Early interceptor for bot group joins; leaves non-supergroups |
| -1 | Users | `logUsers` | `message.All` | `ContinueGroups` | Logs user activity; never blocks propagation |
| 0 | All modules | Commands, callbacks | Various | varies | Default group; standard command handlers |
| 3 | AISpam | `checkAISpam` | `message.All` | `ContinueGroups` (always) | Opt-in AI spam filter; enqueues a check and never blocks propagation |
| 4 | Antiflood | `checkFlood` | `message.All` | `EndGroups` on flood, `ContinueGroups` otherwise | Flood control; skips anon admins and media groups |
| 5 | Locks | `permHandler` | `message.All` | `EndGroups` on violation | Permission-based locks (can_send_messages, etc.) |
| 6 | Locks | `restHandler` | `message.All` | `EndGroups` on violation | Restriction-based locks (stickers, animations, etc.) |
| 7 | Blacklists | `blacklistWatcher` | non-command, non-media-group | `ContinueGroups` (always) | Checks words against blacklist; action taken but processing continues |
| 8 | Reports | (handler) | `message.All` | varies | Report tracking |
| 8 | Reactions | `checkReactions` | `message.All` | `EndGroups` | Checks message reactions against configured patterns; takes action (mute/ban) on violation |
| 9 | Filters | `filtersWatcher` | non-command, non-media-group | `ContinueGroups` (always) | Matches filter patterns; sends configured response but does not block |
| 10 | Pins | (handler) | `message.All` | `EndGroups` | Anti-channel-pin watcher |

## Propagation Behavior

:::caution[Critical propagation rules]
Understanding these rules is essential for debugging why a message did or did not trigger a particular watcher.

- **Captcha is first (group -10).** Any message from a user with a pending captcha is stored for moderator review and deleted. The message never reaches the raid flood counter, antiflood, blacklists, or filters.
- **The lockdown guard intercepts joins (group -7).** It counts every supergroup join for the join surge trigger, then, while a lockdown is active, records the joiner and ends the update, so welcome messages and captcha challenges never run for that join. With no lockdown it returns `ContinueGroups` and the join continues as usual. The old raid mode handler at group -5 is gone.
- **The raid flood counter never blocks (group -4).** It always returns `ContinueGroups` and deletes nothing. When a flood trigger fires it starts a lockdown, and the lockdown's muting stops the flood.
- **Blacklists and Filters use ContinueGroups.** Both take their configured action (warn, mute, ban for blacklists; send reply for filters) but do not block downstream watchers. A message matching a blacklist word still gets checked by filters.
:::

## Interaction Examples

### Scenario 1: User with pending captcha sends a message

1. **Captcha (group -10)** intercepts the message, stores it in the pending queue, and deletes it from the chat.
2. Returns `EndGroups`. Nothing else fires.
3. After successful verification, the stored messages are deleted and the bot sends a type/count summary; their contents are not replayed.

### Scenario 2: User sends a flood of messages

1. Captcha (group -10) passes through (no pending captcha).
2. The raid flood counter (group -4) counts the message towards the flood trigger and continues. If enough of a burst comes from newly joined members, it starts a lockdown, but it does not touch this message.
3. Users (group -1) logs activity, continues.
4. Commands (group 0) fire normally for earlier messages.
5. **Antiflood (group 4)** triggers after the flood threshold is reached, returns `EndGroups`.
6. Locks, blacklists, filters, and pins are all skipped for that message.

### Scenario 3: Message matches both a blacklist word and a filter

1. Captcha (group -10) passes through.
2. The raid flood counter (group -4) counts the message and continues.
3. Users (group -1) logs activity, continues.
4. Commands (group 0) — not a command, passes.
5. Antiflood (group 4) — not flooding, continues.
6. **Locks (groups 5-6)** check permissions. Message passes (no lock violation).
7. **Blacklists (group 7)** matches the word, takes configured action (e.g., warn user), returns `ContinueGroups`.
8. Reports (group 8) tracks the message.
9. **Filters (group 9)** matches the filter pattern, sends the configured response, returns `ContinueGroups`.
10. Pins (group 10) checks for anti-channel-pin conditions.

Both the blacklist action and the filter response are triggered for the same message.

## Related Pages

- [Request Flow](/architecture/request-flow) — Full update processing pipeline
- [Module Pattern](/architecture/module-pattern) — How modules register handlers
