# Phase 2: Staff Actions Across Groups - Context

**Gathered:** 2026-10-05
**Status:** Ready for planning

<domain>
## Phase Boundary

Any member of a Staff Group can run `/ban`, `/mute`, `/kick`, `/unban` and `/unmute` there against a target given as an `@username` or a numeric ID. Ban and mute can be timed, and any action can carry a reason. The bot shows a confirm card. After the issuer taps Confirm, it applies the action in every linked group where the issuer is, at that moment, an admin with the right to restrict members. It never acts in the Staff Group itself. One summary message shows each group as done, skipped (with the reason) or failed (with the reason). Fan-out stays within Telegram's rate limits across several replicas. The same commands keep working as before in every group that isn't a Staff Group.

Requirements: STAFF-01..08, STAFF-12, STAFF-13, PLAT-01.

Not in this phase:
- Posting to each group's log channel (STAFF-09).
- The audit record (STAFF-10).
- "Undo everywhere" (STAFF-11).

All three are Phase 3. In Phase 2 the reason appears only on the card and in the summary.

</domain>

<decisions>
## Implementation Decisions

### Command syntax
- **D-01:** `/ban` and `/mute` take an optional duration. In `/ban @x 2d spamming`, the token right after the target is a duration if it matches the existing `m`/`h`/`d`/`w` form, and the action is then timed. Otherwise the action is permanent and the rest of the text is the reason. `/tban` and `/tmute` also work in the Staff Group and require a duration. The confirm card always shows the parsed duration, or "permanent", so a misparse is visible before Confirm.
- **D-02:** In a Staff Group, `/sban`, `/dban`, `/skick`, `/dkick`, `/smute` and `/dmute` are refused with a hint that names the staff commands. They never act locally in the Staff Group and never fan out.
- **D-03:** No reply-based targeting in a Staff Group. The target must be explicit: a numeric user ID, an `@username` the bot has seen, or a `text_mention` entity. A bare reply with no explicit target gets a hint asking for the `@username` or ID. Otherwise a reply would usually pick a fellow staff member, or the staffer who forwarded the spam.
- **D-04:** `@username` matching is case-insensitive. If the username matches more than one stored user, the bot refuses and lists every match with name, ID and when it last saw them, then asks for the numeric ID. Nothing is guessed. A username the bot has never seen is refused with a hint to use the numeric ID (ROADMAP success criterion 1).

### Confirm card
- **D-05:** Carried forward from PROJECT.md and ROADMAP criterion 1: every staff action shows a confirm card first. It shows the resolved name and ID, the action, the duration, the reason and the number of linked groups. Nothing happens until the issuer taps Confirm, and only the issuer can confirm or cancel.
- **D-06:** Before Confirm the card shows only the linked-group count, for example "Applies to 6 linked groups". The issuer's rights are not pre-checked per group. Every check runs live after Confirm, and those results are the only ones that count.
- **D-07:** An unconfirmed card stays usable for **5 minutes**. After that its buttons answer "expired, run the command again", and the card is edited to "Expired" with the buttons removed.
- **D-08:** Cancel edits the card to "Cancelled by <name>" and removes the buttons. The card is never deleted, so the staff can see an action was considered and dropped.
- **D-09:** On Confirm the card is edited in place into the summary. The buttons are removed, the action and target header stays, and the per-group lines fill in below it. One action is one message in the Staff Group.

### Per-group rules
- **D-10:** A staff `/ban` reaching a group the target isn't in, or has left, still bans there, so the target can't join it later. The line reads "banned (not in group)".
- **D-11:** `/mute` and `/kick` skip groups where the target isn't a member, with the line "skipped: not in group". A target whose status there is `kicked` (banned) counts as not in the group, so a kick can never lift an existing ban.
- **D-12:** A staff action never shortens an existing ban or mute. If the target is already banned or muted in a group and the new action would end sooner, the group is "skipped: already banned" or "skipped: already muted".
- **D-13:** A ban or mute is applied only when it ends later than the current one, where permanent counts as the latest. A permanent `/ban` over an existing 1-day ban upgrades it to permanent. The current end date comes from the same live `getChatMember` call the bot already makes for that group.
- **D-14:** Unban or unmute with nothing to do is "skipped: not banned" or "skipped: not muted". Unmute gives back the group's normal permissions, exactly as `/unmute` does there today. This was proposed as the default, and the owner accepted it by moving on.
- **D-15:** Carried forward from PROJECT.md and STAFF-07: in each group the bot never acts against that group's admins or owner, or against the bot itself. Such a group is skipped with the reason. Staff Group membership alone protects nobody.

### Progress summary
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

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Project scope and requirements
- `.planning/ROADMAP.md` § "Phase 2: Staff Actions Across Groups": goal, requirements, 5 success criteria, and the research flag on `restrictChatMember` against banned or absent users.
- `.planning/REQUIREMENTS.md`: STAFF-01..08, STAFF-12, STAFF-13 and PLAT-01 (full text). STAFF-09..11 are Phase 3.
- `.planning/PROJECT.md`: the Key Decisions table, including:
  - a Confirm tap is required;
  - staff are not immune;
  - every action hits all linked groups;
  - multiple replicas share pacing state.
- `.planning/STATE.md` § Decisions: the Phase 1 rule that a pre-action owner recheck reuses `staff.*Fresh` + `chat_status.CheckOwner`, and that only `OwnerMismatch` removes a link.

### Repo rules
- `AGENTS.md`: handler groups, `WrapCommand`, `callbackcodec` with its 64-byte cap and `""` on overflow, `cache.DeleteCache` on writes, the `alita:staff:*` operational keys, 7 locales, the Staff links trap bullets, real-fixture tests only.

### Research (written before the roadmap)
- `.planning/research/PITFALLS.md`:
  - Pitfall 1: `IsUserAdmin` and the admin cache are not a valid per-group gate.
  - Pitfall 2: issuer identity and anonymous admins.
  - Pitfall 3: link validity and TOCTOU.
  - Pitfall 4: target resolution and target protection.
  - Pitfall 5: fan-out pacing, `retry_after` and partial failures.
  - Pitfall 6: command collisions with per-group `/ban` and the rest.
  - Pitfall 19: 64-byte callback data.
  - Pitfalls A, H, I and J: reason text, rate-limit blind spot, username privacy, edit rate.
- `.planning/research/ARCHITECTURE.md`: the staff-action patterns and fan-out design.
- `.planning/research/FEATURES.md`: Staff Group feature expectations, as GroupHelp does them.

### Phase 1 artifacts
- `.planning/phases/01-staff-group-links/01-VERIFICATION.md` and `01-SECURITY.md`: what Phase 1 guarantees about links, and the open low flag TF-01-06.

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `alita/db/staff/repository.go`:
  - `GetStaffGroupFresh` and `ListLinksByStaffFresh` (ordered `id ASC`) are the authority reads for "is this a Staff Group" and for the fan-out list.
  - `GetLinkOfGroup` is cached and is a gate only.
  - `DeleteLinkIfOwner` and the conditional-write helpers remove a link whose owner no longer matches.
- `alita/modules/staff_recheck.go` (`recheckLink`, `applyLinkHealth`) and `alita/utils/chat_status/owner.go` (`CheckOwner` tri-state, `FetchBotMember`): the per-group owner recheck. ROADMAP criterion 2 says a mismatched link is removed and reported, not acted on.
- `alita/modules/staff.go`: the `staff` callbackcodec namespace and its action constants (`uy`, `un`, `ul`, `uc`, `ux`, `rf`, `pg`). New confirm, cancel and expiry actions go under it. Card state (target, action, duration, reason) belongs in Redis behind a short token, not in callback data.
- `alita/modules/staff_notify.go`: posting into the Staff Group.
- `alita/utils/extraction/extraction.go`:
  - `parseTemporaryDuration` and `ExtractTime` hold the `m`/`h`/`d`/`w` duration grammar.
  - `TemporaryUntilDate` applies the 30 s to 366 d window.
  - `GetUserId` resolves `@username`.
- `alita/db/user/repository.go` `GetUserIdByUserName`: exact-case `username = ?`, and returns one row. D-04 needs a case-insensitive lookup that returns every match, with last-seen data.
- `alita/modules/mute.go` `resolveUnmutePermissions`: the group's normal permissions that unmute restores (D-14).
- `alita/modules/federations.go` `applyActiveFban`: the closest existing fan-out. Use it as a model, not for reuse, because it has no pacing and no per-group accounting.

### Established Patterns
- Commands go through `helpers.WrapCommand` with `CommandDescriptor`. `bans.go` (lines 1073-1083) and `mute.go` (lines 275-279) declare the existing descriptors. Staff behaviour has to branch inside these commands, or run ahead of them, without changing per-group behaviour anywhere else (STAFF-13).
- `alita/modules/moderation.go` (`moderationCommand.run`, `extractFromArgs`, `extractFromReply`, `validateTarget`) is the shared per-group moderation pipeline that staff fan-out runs alongside.
- Every Staff Group authority decision uses fresh reads and conditional writes. Exactly-once notices come from RowsAffected == 1, as in Phase 1.

### Integration Points and Traps
- **Per-group issuer check must be live.**
  - `chat_status.IsUserAdmin` returns true for `tgAdminList` IDs (1087968824 GroupAnonymousBot, 777000) and reads the admin cache.
  - `CanUserRestrict` goes through `hasUserPermission`, which reads `ctx.EffectiveChat` and the anonymous-admin path.
  - Neither is a valid cross-group gate (Pitfall 1). Each linked group needs its own live `getChatMember(group, issuer)` that accepts only `creator`, or `administrator` with `can_restrict_members`.
- **Target status** comes from the same live `getChatMember(group, target)` call:
  - `kicked` with `until_date`;
  - `restricted` with `can_send_messages=false` and `until_date`;
  - `left` and `member`;
  - `administrator` and `creator`, which are protected.
  D-10 to D-14 are decided from it.
- **Anonymous senders** in the Staff Group get "post as yourself" (STAFF-06). `helpers.RejectAnonymousSender()` already does this for `/setstaff`.
- **Pacing:** `alita/utils/ratelimit` holds only the backup limiter. Shared fan-out pacing and `retry_after` handling are new. PLAT-01 requires them to live in Redis so that several replicas share one budget.
- **Callbacks:** use `callbackcodec` only. Never put user text in callback data.

</code_context>

<specifics>
## Specific Ideas

- Summary layout, as the owner picked it:
  ```
  🔨 Ban · Name (123456789) · 2 days · spamming
  ✅ 4 · ⏭ 1 · ❌ 1
  ✅ Group A
  ⏭ Group B: skipped: you're not an admin there
  ❌ Group C: failed: bot lacks ban rights
  ⏳ Group D
  ```
- The model is GroupHelp: one command in the staff room protects every community.

</specifics>

<deferred>
## Deferred Ideas

None: the discussion stayed within phase scope. Log-channel posting, the audit record and "Undo everywhere" were already in Phase 3 and were not reopened.

</deferred>

---

*Phase: 02-staff-actions-across-groups*
*Context gathered: 2026-10-05*
