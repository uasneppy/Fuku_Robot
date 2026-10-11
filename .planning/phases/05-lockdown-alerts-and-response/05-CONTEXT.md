# Phase 5: Lockdown Alerts and Response - Context

**Gathered:** 2026-10-11
**Status:** Ready for planning
**Mode:** Smart discuss (autonomous), all recommended answers accepted by the owner

<domain>
## Phase Boundary

Every confirmed lockdown posts one alert to the locked group's Staff Group (when it is linked) and to its log channel. The alert shows what triggered the lockdown, the counts behind it, a sample of the accounts in the burst, and the invite link the burst used when Telegram reported one. It carries three buttons: "Lift lockdown", "Ban N recent joiners" and "Revoke link". While the group stays locked, a shorter reminder is re-posted every hour until someone lifts it. Requirements LOCK-11, LOCK-12, LOCK-13 and LOCK-14. It also finishes LOCK-06 (lift from the alert's button) and LOCK-01 ("listed on the alert").

Out of scope:
- Automatic triggers, one lockdown and one alert when several triggers fire (LOCK-10), and retiring `/antiraid`. These are Phase 6.
- AI verdict counters. These are Phase 7.
- The settings menu. This is Phase 9.

</domain>

<decisions>
## Implementation Decisions

### The alert itself
- **D-01:** The alert goes to the linked Staff Group (if any) and to the group's log channel. Nothing extra is posted in the locked group, because the `/lockdown` reply is already there. A group with neither destination gets no alert.
- **D-02:** The burst of a manual `/lockdown` has two parts:
  - every joiner the lockdown recorded (`chat_lockdown_joiners`);
  - anyone who joined in the 10 minutes before the lockdown, read from antiraid's Redis join list (`alita:antiraid:joins:<chat>`). This part is best effort and is empty without Redis.

  Phase 6's automatic triggers will define their own burst.
- **D-03:** The alert lists up to 10 accounts, each with name, @username and ID, followed by "and N more". Bots are marked.
- **D-04:** The alert stays as posted. Fresh counts come in the reminders. At the lift, every copy of the alert and of the reminders is edited to "Lifted by X at T" and loses its buttons.

### The "Lift lockdown" button
- **D-05:** Only a live creator, or an administrator with `can_restrict_members`, of the locked group may press it. This is the same check as `/unlockdown` (`requireLockdownAuthority`): a live `getChatMember`, never the admin cache.
  - Being in the Staff Group grants nothing by itself.
  - A press from an anonymous admin or a channel identity is refused with "post as yourself".
- **D-06:** Tapping shows a confirm card ("Lift lockdown in {group}? Confirm / Cancel"). Only the person who tapped can confirm.
- **D-07:** If the lockdown is already lifted or lifting, the press is answered "Already lifted by X at T", nothing changes, and that alert is edited to its lifted state.
- **D-08:** The lift runs exactly the `/unlockdown` path:
  - It goes through `lockdownBeginLift`, which the Phase 4 rule says is the only entry to `BeginLift`.
  - It restores the snapshot first, then runs the paced unbans and posts one tally.
  - The tally is still posted in the locked group. The alert copies are edited to show "Lifted by X".
  - The recorded lifter is the presser.

### "Ban N recent joiners"
- **D-09:** The targets are the whole D-02 burst list, minus:
  - live creators and administrators of the group;
  - approved users;
  - the bot itself.

  Bots in the list are banned.
- **D-10:** The ban is permanent.
  - For a lockdown joiner, the row is marked as deliberately banned, so the lift keeps the ban (Phase 4 D-05).
  - Calls go through the lockdown pacer and use the same "never shorten or lift a deliberate ban" check as the lift: the live status decides each call, and a joiner already kicked by someone else's ban is left alone.
- **D-11:** Tapping shows a confirm card ("Ban N accounts in {group}?"). Only the presser can confirm.
  - The result reports banned, skipped (with reasons) and failed.
  - The group's log channel gets one `#LOCKDOWN_BAN` post.
  - These bans are not part of the staff audit record and cannot be undone.
- **D-12:** The button stays until the lift. A later press bans only accounts that no earlier press handled, and N counts only those. Only one ban run per lockdown can go at a time.

### Invite link and reminders
- **D-13:** The alert shows the invite link most of the burst used, with how many joined through it, plus "+N other links" when there are others. It is shown only when Telegram reported a link (the joiner rows' `invite_link`).
- **D-14:** "Revoke link" may be pressed only by a live creator, or an administrator with `can_invite_users`, of that group.
  - Tapping shows a confirm card, then calls `revokeChatInviteLink`.
  - The alert then says "Link revoked by X".
  - If Telegram refuses, the answer says why.
- **D-15:** A reminder is posted every 60 minutes while the group stays locked.
  - The database-driven worker sends it, with a conditional update so only one replica posts each reminder and a restart loses nothing.
  - Each reminder is a new message in the same destinations as the alert, and the previous reminder loses its buttons.
  - Reminders stop at the lift.
- **D-16:** A reminder says "Still locked since T", the reason, the number of joiners removed so far and the number still pending. It carries the same three buttons and no sample.

### Claude's Discretion
- The table and column layout for alert messages and reminder bookkeeping, following the append-only migration rules. The existing `chat_lockdowns.trigger_kind` and the joiner rows' `invite_link`, `first_name`, `username` and `is_bot` are already there for this.
- The callback namespace and the token scheme, within the 64-byte `callbackcodec` cap. Callback data carries only IDs or tokens.
- The exact wording of the alert, reminder, confirm and result texts in all 7 locales.

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets
- **Lockdown state** (`alita/db/lockdown`): fresh reads, conditional updates, and the joiner rows with names, `invite_link`, `join_path`, `performer_id`, `state` and `ban_until` (migration 20261006120000).
- **Lift and authority:** `lockdownBeginLift`, `requireLockdownAuthority` and the lift worker's tally (`alita/modules/lockdown*.go`).
- **Lockdown pacer:** `lockdownPaced` / `lockdownPacer`, fleet-wide, keys `alita:lockdown:pace:*`.
- **Staff Group lookups:** the Staff Group link for a group, and the `/staff` panel's 🔒 marker (`lockdown.ListActiveByChatsFresh`).
- **Log channel posts:** `sendStaffLogPost` in `staff_log.go`.
- **Confirm cards:** the Redis-backed card hash with a Lua compare-and-set, an issuer-only Confirm and expiry (`staff_action_card.go`).
- **Antiraid's recent-joins list:** a Redis sorted set per chat, `alita:antiraid:joins:<chat>` (`antiraid.go`).

### Established Patterns
- **Authority:** a live `getChatMember` for every press, never the admin cache.
- **Telegram calls:** paced and fleet-wide; a slot more than 60 s away fails at once.
- **Callback data:** goes through `callbackcodec` and carries only IDs or tokens.
- **Workers:** database-driven worker cycles on every replica, using conditional claims and `ReleaseStaleClaims`.
- **Failures:** each one is reported, never dropped, and never posted twice: only the replica whose conditional update won posts a notice.

### Integration Points
- **Posting the alert:** after a lockdown is confirmed (`locked_at` set), whether by `/lockdown` itself or by the worker settling an unconfirmed row.
- **Lift:** the alert's Lift button joins the `/unlockdown` path. The lift then edits every stored alert and reminder message.
- **Reminders:** the lockdown worker cycle sends them.
- **Phase 6:** its automatic triggers will call the same alert path with their own trigger kind and counts.

</code_context>

<specifics>
## Specific Ideas

- The alert's sample and the "Ban N recent joiners" target list both read the joiner rows Phase 4 records (Phase 4 D-04).
- A "Ban N recent joiners" ban must survive the lift (Phase 4 D-05), the same way a staff ban over a lockdown ban does.
- Anonymous-admin presses on the alert buttons are refused rather than proven, because the buttons sit in the Staff Group and the log channel, not in the group itself.

</specifics>

<deferred>
## Deferred Ideas

- A persistent PostgreSQL log of recent joins in every group, so the pre-lockdown part of the burst also works without Redis. It was considered for D-02 and declined for now, because it costs one write per join in every group. Phase 6 may revisit it with its join-burst detector.
- Undo for "Ban N recent joiners" through the staff audit. Declined for v1 (D-11).

</deferred>
