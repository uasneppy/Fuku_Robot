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
- ✓ Staff actions: any Staff Group member runs `/ban`, `/mute`, `/kick`, `/unban` and `/unmute` (timed ban and mute, optional reason) against an `@username`, a numeric ID or a mention, behind a Confirm card only the issuer can tap — Phase 2
- ✓ Every staff action fans out to all linked groups with a live per-group check of the issuer's restrict right; it acts where allowed, skips the rest, never touches the Staff Group, and never lifts a ban by accident — Phase 2
- ✓ One live summary per action: every group is marked done, skipped or failed with its reason, and none is dropped, under Telegram rate limits across replicas — Phase 2
- ✓ Each applied staff action and its reason are posted to the admin log channel of every group where it was applied, naming the issuer and target and "via Staff Group" but never the Staff Group itself — Phase 3
- ✓ Every staff action is recorded (issuer, target, action, duration, reason, each group's prior state and outcome), and `/staff` has a Recent actions list with a per-action detail view — Phase 3
- ✓ The summary has an "Undo everywhere" button: any Staff Group member can undo ban, mute, unban or unmute after a Confirm tap, with a live per-group restrict check for the presser; undo restores each group's prior state and skips a group whose status changed since — Phase 3

### Active

<!-- Current scope. Hypotheses until shipped and validated. Order reflects priority. -->

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
| Any Staff Group member can act, gated by per-group admin rights | The Staff Group holds trusted people; the per-group check is the real safeguard | ✓ Shipped — Phase 2 |
| Act where allowed, skip the rest, with a per-group summary | Partial success is more useful than all-or-nothing, as long as it's visible | ✓ Shipped — Phase 2 |
| Every action hits all linked groups | That is the core value; per-group targeting adds complexity without demand | ✓ Shipped — Phase 2 |
| Only a person who owns both groups can link them; one Staff Group per group; auto-unlink on ownership change | Strongest guarantee that a Staff Group can't gain control of a group it shouldn't | ✓ Shipped — Phase 1 |
| Lockdown = kick joiners + mute all except admins and approved users; admin-only manual lift | Stops a raid immediately and leaves the "safe again" call to a human | — Pending |
| Rules trigger lockdown; AI only assists | Fast and cheap enough to react to hundreds of joins; AI where judgement is needed | — Pending |
| Cloudflare Turnstile through a bot-hosted Mini App page | More private and less intrusive than reCAPTCHA; server-side verification | — Pending |
| `/staff` panel first, folded into the settings menu later | Ships the Staff Group without waiting for the settings menu | ✓ Panel shipped — Phase 1; menu fold-in Phase 9 |
| Order: Staff Group → raid protection → web captcha → settings menu | Owner's priority | — Pending |
| Every staff action needs a Confirm tap showing the resolved target | `@usernames` can be stale or recycled; owner prefers safety over speed | ✓ Shipped — Phase 2 (5-minute card, issuer-only Confirm) |
| Staff Group members aren't protected from staff actions; only each group's admins and owner, and the bot, are | Owner's call; staff are trusted, not immune | ✓ Shipped — Phase 2 |
| Undo-everywhere button and a recent-actions list in `/staff` are in v1, backed by an audit record | Mistakes are reversible and accountable | ✓ Shipped — Phase 3 (live UAT passed) |
| Old `/antiraid` is retired; its auto-threshold carries over to join-surge detection | One raid system, not two | — Pending |
| Auto-triggers are on by default with conservative thresholds | Protection from day one; admins tune or disable per group | — Pending |
| Images are classified by Gemini `gemini-3.5-flash-lite` (owner's key); text stays on TypeSafe | Owner's choice of provider; TypeSafe is text-only | — Pending |
| Turnstile captcha is one solve only; math and text modes stay as fallbacks | Owner's call; fallbacks cover Cloudflare or hosting outages | — Pending |
| `/settings` asks whether to open in the group or in private | GroupHelp-style choice | — Pending |
| Multiple replicas: pacing, counters, challenges and lockdown state are shared, not per process | That's how the bot is deployed | ✓ Pacing and the per-target lock are shared through Redis — Phase 2; counters, challenges and lockdown state come later |
| Role exclusivity (a group can't be both a Staff Group and linked) is enforced by a PostgreSQL trigger with per-chat advisory locks, on top of the app checks | Closes the two-replica race the app checks alone can't; trigger rejection fails closed | ✓ Good — Phase 1 (concurrency test passes on PostgreSQL 16) |
| Only a definite owner mismatch removes a link; any Telegram error counts as unknown and changes nothing | A 429 or timeout must never mass-unlink groups | ✓ Good — Phase 1 |
| Exactly-once notices come from conditional writes (RowsAffected == 1 posts), not locks | Racing triggers and replicas can't double-post or contradict each other | ✓ Good — Phase 1 |
| Hourly staff sweeper across replicas behind a Redis `SETNX` lock; runs unguarded without Redis because every staff write is conditional | Catches ownership changes Telegram never announced (bot not admin, bot down) | ✓ Good — Phase 1 |
| Link refusals to strangers use one uniform text and never post into the named Staff Group | Prevents probing which chats are Staff Groups and spamming someone else's staff room | ✓ Good — Phase 1 |
| Staff commands are raw group-0 interceptors ahead of Bans and Mutes, a documented exception to `WrapCommand` | `BuildCommandContext` replies to sender-less updates; outside a Staff Group the interceptors pass through with no reply, write or Telegram call, so per-group commands are unchanged | ✓ Good — Phase 2 |
| One decision table picks each group's Telegram call from the target's live status | `restrictChatMember` and `unbanChatMember` replace a status server-side, so a mute, kick or unmute could otherwise lift a ban | ✓ Good — Phase 2 (live UAT passed) |
| Staff fan-out goes through a fleet-wide Redis pacer; a call whose slot is more than 60 s away fails at once as "rate limited" | One flood must not freeze every staff run on every replica, and a refused group is reported, never dropped | ✓ Good — Phase 2 (edge case WR-03 at `retry_after` = 60 left open) |
| One staff run per target, held by a Redis lock the run renews every 10 minutes | A second Confirm on the same person must not race a slow first run | ✓ Good — Phase 2 |
| Each summary message has its own delivery budget, and progress edits pause during a 429 | The final summary must still arrive when Telegram makes the bot wait | ✓ Good — Phase 2 |
| The staff audit record lives in PostgreSQL, is read fresh (never cached) and is kept forever, outside backup, export, import and reset | An audit trail must not be stale, pruned or rewritten by an import | ✓ Good — Phase 3 |
| A record is created at Confirm and each group's prior state is written before its Telegram write; a failed write fails closed | Undo can only restore what was recorded, so nothing acts unrecorded | ✓ Good — Phase 3 |
| Undo restores each group's recorded prior state and skips a group whose live status no longer matches what the action left behind | Undo must never lift a ban or mute an admin put there later | ✓ Good — Phase 3 (unmute undo uses the broad "left behind" check, WR-04, owner accepted) |
| One undo per action through a conditional database claim, given back when no group reached its Telegram write; any attempted write keeps it | A crash or refusal before any write must not burn the only undo, and a possibly applied write must never allow a second one | ✓ Good — Phase 3 (UAT tests 4 and 7) |
| Undo state shown everywhere comes from the stored per-group undo outcomes (running, interrupted, undone, changed nothing), never from the claim alone | The history must say what actually happened; unconfirmed outcomes read "changed nothing" | ✓ Good — Phase 3 (UAT tests 6 and 8) |
| Staff log posts use the existing `admin` log category | No new settings or migration; a group that turns admin logs off gets no staff posts | ✓ Good — Phase 3 |

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
*Last updated: 2026-10-06 after Phase 3*
