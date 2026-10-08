# Phase 1: Staff Group Links - Context

**Gathered:** 2026-10-04
**Status:** Ready for planning

<domain>
## Phase Boundary

A group's owner can make that group a Staff Group, link the supergroups they own to it, and unlink them. Each link records who made it. A link is removed automatically as soon as that person no longer owns both groups. A Staff Group keeps its status and links when it upgrades from a basic group to a supergroup and gets a new chat ID. Inside the Staff Group, `/staff` shows the help text and each linked group's live status, with buttons to add, unlink and refresh.

Not in this phase: staff moderation actions (`/ban`, `/mute` and the rest across groups) are Phase 2. The audit record and "recent actions" in `/staff` are Phase 3. The "in lockdown" status in `/staff` comes in Phase 4.

Requirements: SETUP-01 to SETUP-08, PLAT-03.

</domain>

<decisions>
## Implementation Decisions

### Linking flow
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

### Staff Group rules
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

### When links break
- **D-12:** **A link belongs to the person who made it.** The link row stores that user's ID. The link is valid only while that same user is the live `creator` of *both* groups. If either group's creator differs, the link is removed. This holds even when both groups go to the same new owner: they have to relink. — **Reversibility:** one-way — The stored column and the validity rule define what a "link" is; every later phase's authority checks depend on it.
- **D-13:** **When a link is removed automatically, the Staff Group is told** which group was unlinked and why (for example, "its owner changed"). No private messages are sent, and nothing is posted in the linked group.
- **D-14:** **Losing the bot doesn't break a link.** If the bot is removed from a linked group, or loses admin or the restrict right there, the link stays. `/staff` shows it as broken ("bot not in group", "bot not admin", "bot can't restrict"), and the Staff Group gets **one** heads-up when that state changes, not one on every check. Adding the bot back fixes it with no relinking.
- **D-15:** **Ownership changes are caught in three ways:**
  1. Event-driven: `chat_member` and `my_chat_member` updates where the bot is an admin. The research suggests a dedicated handler group (e.g. `-3`), because the existing group-0 `NewChatMember` handler in `bot_updates.go` would shadow a group-0 watcher.
  2. A **background re-check about every hour** of every link, spread out so it doesn't burst API calls. This is what satisfies "the link disappears without anyone running a command" for groups where the bot isn't an admin.
  3. A live check whenever `/staff` opens or Refresh is pressed, and before every staff action from Phase 2.

### `/staff` panel
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

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Scope and requirements
- `.planning/ROADMAP.md` §"Phase 1: Staff Group Links": goal, success criteria, and the research flag on whether `chat_member` updates arrive on ownership transfer
- `.planning/REQUIREMENTS.md` §"Staff Group setup and links": SETUP-01 to SETUP-08; §Platform: PLAT-03 (all 7 locales)
- `.planning/PROJECT.md` §Key Decisions, §Constraints, §Context: owner-of-both linking, one Staff Group per group, separate from federations, anonymous-admin limits

### Repo rules
- `AGENTS.md`: migrations (append-only, timestamped, one transaction per file), `cache.DeleteCache` on every write, `helpers.WrapCommand`, handler-group table, locale rules, panic-safe goroutines, real-fixture tests. A new handler group or Redis key family updates `AGENTS.md` in the same commit.

### Research (already done for this milestone)
- `.planning/research/ARCHITECTURE.md` §"Flow B: Link, unlink, and auto-unlink": the three-layer ownership detection and the group `-3` watcher. §"Data Model": `staff_groups` and `staff_group_links` (UNIQUE(group_chat_id), CHECK that a group isn't linked to itself), uncached reads for authority checks, `skipLocal` prefixes. §"Handler Group Plan".
- `.planning/research/PITFALLS.md` §"Pitfall 1" (the cached admin list isn't an authority), §"Pitfall 2" (anonymous admins and issuer identity), §"Pitfall 3" (ownership is checked at one moment and can change before it's used; chat migration), §"Pitfall G" (edge cases when designating a Staff Group), §"Pitfall 19" (the 64-byte callback cap)
- `.planning/research/FEATURES.md` §"Area 1: Staff Group": table stakes, auto-unlink, chat-migration re-keying
- `.planning/research/SUMMARY.md` §"Phase 1: Staff Group". Note: it bundles staff actions and the audit table into "Phase 1". The roadmap has since split those into Phases 2 and 3, and the roadmap is authoritative.

### Codebase maps
- `.planning/codebase/ARCHITECTURE.md`, `.planning/codebase/CONVENTIONS.md`, `.planning/codebase/TESTING.md`

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `chat_status.RequireUserOwner` (`alita/utils/chat_status/access.go`): a live `getChatMember` check for `creator`, with no cache. It's the base for every owner-of-both check. It needs a variant that takes an explicit chat ID, so one call can check the *other* group.
- `federations.joinFed` / `leaveFed` (`alita/modules/federations.go`): the nearest linking pattern. They run in the group being joined, with an owner check, and give a typed reply for each refusal.
- `deeplink_router.go`: `RegisterDeepLinkHandler` / `HandleDeepLink`. Today it only handles private-chat `/start <arg>`. The group `/start@bot <payload>` that the picker sends needs a new route (`help.go` `start` currently answers "ask me in PM" in groups).
- `alita/utils/callbackcodec`: namespaced callback data under 64 bytes, for the panel's Add group, Unlink, Confirm and Refresh buttons.
- The anonymous-admin flow (`bot_updates.go`, `RegisterAnonymousAdminHandler`, `anon_admin` callback), if anonymous owners are supported.
- Background task patterns: `startCaptchaWorkers` (`captcha.go`) and the ticker loops in `antiraid.go` and `aispam.go`. They use a lifecycle context, `error_handling.RecoverFromPanic`, and a shutdown hook. The hourly re-check should follow them.

### Established Patterns
- Repository pattern: `alita/db/<domain>/`. Reads use `cache.GetFromCacheOrLoad`, and writes call `cache.DeleteCache` after commit. The research says authority reads (links used to decide ownership or actions) bypass the cache.
- Commands are registered with `helpers.WrapCommand(dispatcher, CommandDescriptor{...}, handler)`. Failed checks are answered by the pipeline, not the handler.
- Handler groups: a `chat_member` watcher must not sit in group 0, because `bot_updates.go` already has a group-0 `NewChatMember(ExtractAdminUpdateStatusChange, …)` that would shadow it. `AllowedUpdates` already includes `chat_member` and `my_chat_member` (`alita/config/config.go`).
- Migrations: SQL files in `migrations/`. The latest is `20261001120000_*`, so new files need a later timestamp. Also add new models to the test AutoMigrate lists in `testmain_test.go`.

### Integration Points
- New module `alita/modules/staff.go` (or similar), registered via `RegisterLegacyModule`.
- New repo `alita/db/staff/` with models and migrations for Staff Groups and links.
- `/start` in groups (`help.go`) is routed to the staff picker payload handler.
- Chat migration: a new handler for service messages that carry `migrate_to_chat_id` / `migrate_from_chat_id`.
- Locale files: all 7, for every new string (`make check-translations`).
- `AGENTS.md` handler-group table, if a new group such as `-3` is added.

</code_context>

<specifics>
## Specific Ideas

- **GroupHelp-style linking:** "Add group" opens Telegram's own group picker, so nobody copies chat IDs. `/linkstaff` is the typed fallback.
- **Keep linked groups quiet:** members of a linked group never see that it's linked. Command messages are deleted, and every notice and refusal for the owner goes to the Staff Group.
- **Scale:** under 10 groups per Staff Group, so a single-message panel with live checks on every open is fine.

</specifics>

<deferred>
## Deferred Ideas

- Showing `/staff` in a private chat (listing the Staff Groups you own) or in a linked group ("linked to Staff Group X"): offered, but the owner chose "Staff Group only". Could come back with the settings menu (Phase 9).
- Messaging the person who made a link privately when it breaks: offered; the owner chose a Staff Group notice only.
- Nested Staff Groups (head staff over moderator rooms): rejected for now (D-10).

</deferred>

---

*Phase: 01-staff-group-links*
*Context gathered: 2026-10-04*
