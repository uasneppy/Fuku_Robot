# Fuku Robot

## What This Is

Fuku Robot is a fork of Alita Robot, a Go Telegram group-management bot. It runs only for my own communities, and the goal is to make it work more like GroupHelp. The first step is a **Staff Group**: one Telegram group of trusted admins that manages all my other groups. Any of us can ban, mute or kick someone in every linked group with one command. After that come smarter raid protection, a web-based captcha, and a button settings menu.

## Core Value

My trusted staff can protect every one of my communities from one place. We act on a bad actor across all groups at once, and the bot never lets anyone act in a group where they aren't an admin.

## Requirements

### Validated

<!-- Inferred from the existing Alita Robot codebase (.planning/codebase/). Shipped and relied upon. -->

- ✓ Per-group moderation: ban, mute, kick, warn, including timed variants, plus purges — existing
- ✓ Admin tooling: admin list cache, approvals (approved users skip antiflood, locks, blacklists and captcha), disabling commands, anonymous-admin handling — existing
- ✓ Content controls: locks, blacklists, filters, notes, rules, greetings, pins, reactions — existing
- ✓ Antiflood with per-replica in-process counters — existing
- ✓ Antiraid "raid mode": temp-bans new joiners for a set time, auto-enables at N joins/min, expires on its own, Redis-only — existing
- ✓ Join captcha (math/text/image via base64Captcha), one attempt per user per chat, pending messages replayed on pass — existing
- ✓ AI spam detection through the TypeSafe API (`aispam`, inert without `TYPESAFE_API_KEY`) — existing
- ✓ Federations: `/fban` bans across a federation's chats where the user is known, with federation admins and a federation log chat — existing
- ✓ Per-group log channels and `/report` — existing
- ✓ Connections: manage one group's settings from PM — existing
- ✓ Backup import and export, 7 UI languages, PostgreSQL + Redis, webhook or polling, health and metrics endpoints — existing

### Active

<!-- Current scope. Hypotheses until shipped and validated. Order reflects priority. -->

**Staff Group (first)**
- [ ] A group's owner can designate it as a Staff Group, and only the owner can undo that. It must be a Telegram group, not a channel.
- [ ] A user who owns both a group and the Staff Group can link that group to it. Each group links to at most one Staff Group.
- [ ] A link breaks automatically as soon as the same person no longer owns both groups.
- [ ] Any member of the Staff Group can run `/ban`, `/mute`, `/kick`, `/unban` and `/unmute`, including timed ban and mute, against an `@username` or a user ID.
- [ ] Every Staff Group action applies to all linked groups. In each group the bot first checks that the issuing member is an admin there with the needed right. It acts where they have it and skips the rest.
- [ ] After each action the issuer gets a per-group summary: done, skipped (not an admin there or missing the right), or failed (for example, the bot lacks rights).
- [ ] An optional reason appears in the summary and is posted to each affected group's log channel.
- [ ] A `/staff` panel shows the help text and the linked groups with their status. It later becomes the "Staff Group" button in the settings menu.

**Raid protection (second)**
- [ ] A lockdown stops new members (anyone joining is kicked and listed in the alert) and mutes everyone except admins and approved users.
- [ ] A lockdown can be triggered by a join surge, a message or media flood (messages, photos, GIFs, stickers, mostly from new members), a burst of AI spam verdicts, or a manual `/lockdown`.
- [ ] Detection uses fast rule-based rates (joins, floods) to trigger lockdown. AI only assists by classifying borderline text and images.
- [ ] Each lockdown posts an alert to both the Staff Group and the attacked group's log channel. The alert has a "Lift lockdown" button and a "Ban N recent joiners" button for the accounts that joined in the burst.
- [ ] Only an admin of the locked group can lift it, wherever they press the button. A lockdown never lifts on its own.
- [ ] A lockdown applies only to the attacked group, not to every linked group.

**Web captcha (third)**
- [ ] New members verify through Cloudflare Turnstile on a Telegram Mini App page the bot hosts, with server-side token verification.
- [ ] A member who doesn't pass within the time limit is kicked and can rejoin to try again.

**Settings menu (fourth)**
- [ ] An inline-button settings menu configures each group, with "Staff Group" as one of its pages.

### Out of Scope

- Public, multi-tenant bot — it runs for my own communities only.
- A Telegram channel as the Staff Group — it has to be a group so members can talk and run commands.
- Merging with or changing federations — the Staff Group is a separate feature, and `/fban` stays as it is.
- Picking a subset of groups for one Staff Group action — every action hits all linked groups, by design.
- Linking one group to several Staff Groups — one Staff Group per group keeps authority unambiguous.
- Linking by group admins or by two different owners agreeing — only a person who owns both groups can link them.
- Targeting by replying to a forwarded message, or report buttons in the Staff Group — not chosen for v1; `@username` or ID is enough.
- Deleting the target's messages across groups — not needed for v1.
- Google reCAPTCHA — Turnstile was chosen as the more private, less intrusive option.
- Auto-lifting lockdowns — an admin must decide when it's safe.
- Sending every message to an AI model — too slow and costly; AI only assists rule-based detection.

## Context

- **Fork of Alita Robot.** The Go module path is still `github.com/divkix/Alita_Robot`. `AGENTS.md` at the repo root is the authoritative rulebook for this codebase, covering migrations, cache invalidation, handler groups, commands, locales and tests. Every phase must follow it.
- **The codebase map** is in `.planning/codebase/`. It was written by fast mapper agents; its `CONCERNS.md` items are leads to verify, not confirmed bugs.
- **Raid protection replaces the behaviour of the existing `antiraid`.** Today it temp-bans joiners, expires on its own, and relies on Redis alone. The new lockdown mutes, kicks joiners, never expires, and is lifted only by an admin. Existing `antiflood` counters live in-process per replica, and `aispam` already calls the TypeSafe API. Both are building blocks.
- **The existing captcha** uses `base64Captcha` and allows one attempt per user per chat. Group `-10` stores a pending user's messages for replay. The Turnstile captcha has to fit that flow or replace it cleanly.
- **Federations are the closest existing pattern.** `applyActiveFban` in `alita/modules/federations.go` already fans out bans across many chats. It is a model for Staff Group fan-out, not something to reuse directly.
- **`@username` lookup has a limit.** The Bot API can't resolve an arbitrary `@username`; the bot only knows users it has seen, through the users tracker in handler group `-1`. A raw user ID always works.
- **Anonymous admins can't be identified.** Someone posting as an anonymous admin in the Staff Group can't pass the per-group admin check, so their actions can't be authorised.
- **Hosting the captcha page.** The bot already runs an HTTP server for webhook, health and metrics, which can host the Mini App page. The owner will host whatever is most secure.

## Constraints

- **Tech stack:** Go 1.26.0, gotgbot v2, GORM on PostgreSQL (SQLite in tests), Redis. Stay on the existing stack.
- **Repo rules:** follow `AGENTS.md`:
  - Migrations are append-only, with timestamped names and one transaction per file.
  - Every write `cache.DeleteCache`s the keys it affects.
  - Commands go through `helpers.WrapCommand`.
  - Locale keys go in all 7 locale files.
  - Fire-and-forget goroutines recover from panics.
  - Tests use real fixtures (no mock libraries) and run through `make test`.
- **Security:**
  - The per-group admin check is never skipped.
  - Turnstile verification runs server-side.
  - New secrets, such as the Turnstile secret key, are registered with `logredact.RegisterSecret`.
- **Telegram limits:**
  - Fan-out across many groups has to respect Bot API rate limits and report partial failures, not drop them.
  - Callback data is capped at 64 bytes through `callbackcodec`.
- **Deployment:** Docker, with `AUTO_MIGRATE=true` in the deploy manifests.

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| Staff Group is separate from federations | Different model: a shared staff room, not a ban list; keeps `/fban` stable | — Pending |
| Any Staff Group member can act, gated by per-group admin rights | The Staff Group holds trusted people; the per-group check is the real safeguard | — Pending |
| Act where allowed, skip the rest, with a per-group summary | Partial success is more useful than all-or-nothing, as long as it's visible | — Pending |
| Every action hits all linked groups | That is the core value; per-group targeting adds complexity without demand | — Pending |
| Only a person who owns both groups can link them; one Staff Group per group; auto-unlink on ownership change | Strongest guarantee that a Staff Group can't gain control of a group it shouldn't | — Pending |
| Lockdown = kick joiners + mute all except admins and approved users; admin-only manual lift | Stops a raid immediately and leaves the "safe again" call to a human | — Pending |
| Rules trigger lockdown; AI only assists | Fast and cheap enough to react to hundreds of joins; AI where judgement is needed | — Pending |
| Cloudflare Turnstile through a bot-hosted Mini App page | More private and less intrusive than reCAPTCHA; server-side verification | — Pending |
| `/staff` panel first, folded into the settings menu later | Ships the Staff Group without waiting for the settings menu | — Pending |
| Order: Staff Group → raid protection → web captcha → settings menu | Owner's priority | — Pending |

## Evolution

This document evolves at phase transitions and milestone boundaries.

**After each phase transition** (via `/gsd-transition`):
1. Requirements invalidated? → Move to Out of Scope with reason
2. Requirements validated? → Move to Validated with phase reference
3. New requirements emerged? → Add to Active
4. Decisions to log? → Add to Key Decisions
5. "What This Is" still accurate? → Update if drifted

**After each milestone** (via `/gsd-complete-milestone`):
1. Full review of all sections
2. Core Value check — still the right priority?
3. Audit Out of Scope — reasons still valid?
4. Update Context with current state

---
*Last updated: 2026-10-04 after initialization*
