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
- ✓ Staff Group designation: only a group's live owner can set or remove Staff Group status; channels are refused — Phase 1
- ✓ Linking: a person who owns both groups links a group to one Staff Group (`/linkstaff` or the Add group picker), and unlinks it — Phase 1
- ✓ Links break automatically when the same person no longer owns both groups (ownership watchers plus an hourly live sweep; errors never unlink) — Phase 1
- ✓ `/staff` panel shows the help text and each linked group's live status, with Refresh and paging; the settings-menu button is Phase 9 — Phase 1
  <!-- Phase 1 caveat: 10 of 11 live-Telegram UAT checks were deferred without being run (see phases/01-staff-group-links/01-UAT.md Deferred Follow-Ups). -->

### Active

<!-- Current scope. Hypotheses until shipped and validated. Order reflects priority. -->

**Staff Group (first)**
- [ ] Any member of the Staff Group can run `/ban`, `/mute`, `/kick`, `/unban` and `/unmute`, including timed ban and mute, against an `@username` or a user ID.
- [ ] Every Staff Group action applies to all linked groups. In each group the bot first checks that the issuing member is an admin there with the needed right. It acts where they have it and skips the rest.
- [ ] After each action the issuer gets a per-group summary: done, skipped (not an admin there or missing the right), or failed (for example, the bot lacks rights).
- [ ] An optional reason appears in the summary and is posted to each affected group's log channel.

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
| Only a person who owns both groups can link them; one Staff Group per group; auto-unlink on ownership change | Strongest guarantee that a Staff Group can't gain control of a group it shouldn't | ✓ Shipped — Phase 1 |
| Lockdown = kick joiners + mute all except admins and approved users; admin-only manual lift | Stops a raid immediately and leaves the "safe again" call to a human | — Pending |
| Rules trigger lockdown; AI only assists | Fast and cheap enough to react to hundreds of joins; AI where judgement is needed | — Pending |
| Cloudflare Turnstile through a bot-hosted Mini App page | More private and less intrusive than reCAPTCHA; server-side verification | — Pending |
| `/staff` panel first, folded into the settings menu later | Ships the Staff Group without waiting for the settings menu | ✓ Panel shipped — Phase 1; menu fold-in Phase 9 |
| Order: Staff Group → raid protection → web captcha → settings menu | Owner's priority | — Pending |
| Every staff action needs a Confirm tap showing the resolved target | `@usernames` can be stale or recycled; owner prefers safety over speed | — Pending |
| Staff Group members aren't protected from staff actions; only each group's admins and owner, and the bot, are | Owner's call; staff are trusted, not immune | — Pending |
| Undo-everywhere button and a recent-actions list in `/staff` are in v1, backed by an audit record | Mistakes are reversible and accountable | — Pending |
| Old `/antiraid` is retired; its auto-threshold carries over to join-surge detection | One raid system, not two | — Pending |
| Auto-triggers are on by default with conservative thresholds | Protection from day one; admins tune or disable per group | — Pending |
| Images are classified by Gemini `gemini-3.5-flash-lite` (owner's key); text stays on TypeSafe | Owner's choice of provider; TypeSafe is text-only | — Pending |
| Turnstile captcha is one solve only; math and text modes stay as fallbacks | Owner's call; fallbacks cover Cloudflare or hosting outages | — Pending |
| `/settings` asks whether to open in the group or in private | GroupHelp-style choice | — Pending |
| Multiple replicas: pacing, counters, challenges and lockdown state are shared, not per process | That's how the bot is deployed | — Pending |
| Role exclusivity (a group can't be both a Staff Group and linked) is enforced by a PostgreSQL trigger with per-chat advisory locks, on top of the app checks | Closes the two-replica race the app checks alone can't; trigger rejection fails closed | ✓ Good — Phase 1 (concurrency test passes on PostgreSQL 16) |
| Only a definite owner mismatch removes a link; any Telegram error counts as unknown and changes nothing | A 429 or timeout must never mass-unlink groups | ✓ Good — Phase 1 |
| Exactly-once notices come from conditional writes (RowsAffected == 1 posts), not locks | Racing triggers and replicas can't double-post or contradict each other | ✓ Good — Phase 1 |
| Hourly staff sweeper across replicas behind a Redis `SETNX` lock; runs unguarded without Redis because every staff write is conditional | Catches ownership changes Telegram never announced (bot not admin, bot down) | ✓ Good — Phase 1 |
| Link refusals to strangers use one uniform text and never post into the named Staff Group | Prevents probing which chats are Staff Groups and spamming someone else's staff room | ✓ Good — Phase 1 |

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
*Last updated: 2026-10-05 after Phase 1*
