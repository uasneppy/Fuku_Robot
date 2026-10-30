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
- ✓ Antiraid "raid mode": temp-bans new joiners for a set time, auto-enables at N joins/min, expires on its own, Redis-only — existing (retired in Phase 6, replaced by automatic lockdown)
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
- ✓ `/lockdown` locks one group: its default permissions drop to all-off, everyone but its admins is muted (approved users too, because Telegram cannot exempt them), and anyone who joins or asks to join is banned or declined until the lift — Phase 4
- ✓ `/unlockdown` (a live owner or restrict-capable admin, anonymous admins after a proof tap) restores the exact pre-lockdown permissions, unbans only the lockdown's own bans, and posts one tally; `/lockdownstatus` shows since when, why and who — Phase 4
- ✓ A lockdown lives in PostgreSQL only: it survives restarts and Redis being cleared, never lifts on its own, touches only its own group, and shows as 🔒 on the linked group's `/staff` row — Phase 4
- ✓ Unmuting during a lockdown (`/unmute`, the Unmute button, a captcha pass, staff `/unmute`) uses the stored pre-lockdown permissions, so nobody stays muted after the lift — Phase 4
- ✓ Each lockdown posts one alert to the linked Staff Group and to the log channel (when its admin category is on): trigger, counts, up to 10 accounts from the burst including the ones the lockdown removed, and the invite link the burst used — Phase 5
- ✓ The alert's "Lift lockdown" works only for a live admin of the locked group with the restrict right, wherever it is pressed (Staff Group or log channel), through the same lift path as `/unlockdown`; a press on a lifted lockdown says so and changes nothing — Phase 5
- ✓ "Ban N recent joiners" bans the burst for good, skipping admins, approved users, the bot and joiners an admin let in, reports banned/skipped/failed and posts one `#LOCKDOWN_BAN`; the bans survive the lift — Phase 5
- ✓ "Revoke link" revokes the raid's invite link (bot-made links only) for a live admin with the invite right; hourly reminders repeat the alert until the lift — Phase 5
- ✓ A join surge starts a lockdown on its own: 15 different accounts joining within 60 s by default, counted across replicas in Redis; accounts an admin added or approved, approved users, join requests and the bot never count — Phase 6
- ✓ A message or media flood starts a lockdown on its own: 30 or more items (text, photo, GIF, sticker and other media) within 30 s, at least 60% of them from at least 3 accounts that joined in the last 24 h; admins, approved users, anonymous senders, service accounts and the bot never count, and a flood lockdown's alert lists the flooders — Phase 6
- ✓ An automatic lockdown is exactly a `/lockdown` (same snapshot, join guard, worker, alert, buttons, reminders and lift), tagged `join_surge` or `flood` with its numbers. Triggers firing together, on any replica, give one lockdown and one alert, and stay quiet for 10 minutes after a lift; a group the bot cannot lock gets one daily warning in its Staff Group and log channel — Phase 6
- ✓ `/autolockdown` shows both triggers to any live admin and turns each on or off or sets its threshold (3 to 500) for the owner or an admin with restrict rights, checked live; `/autoantiraid` is an alias for the join threshold — Phase 6
- ✓ The old `/antiraid` raid mode is retired: its commands point to `/lockdown` and `/autolockdown`, nobody is temp-banned any more, its leftover Redis keys are swept at startup, and each group's old auto-threshold carried over to the join-surge setting (clamped to 3..500) — Phase 6
- ✓ A burst of AI spam verdicts starts a lockdown on its own: 5 spam verdicts from at least 3 different accounts within 120 s by default, counted across replicas in Redis. `/aispam` delete verdicts feed it, and so do suspicious texts, photos, GIFs and stickers from members who joined in the last 24 h, checked off the update path (text by TypeSafe, images by Google Gemini when `GEMINI_API_KEY` is set). Ordinary messages and established members' messages are never sent to any AI — Phase 7
- ✓ AI never decides alone and never blocks: one verdict, one account or a duplicate delivery never locks; a slow, failing, rate-limited, unreadable or unconfigured provider means no verdict; the join-surge and flood triggers keep working with every provider down; `ENABLE_AISPAM=false` stops it all; hourly spend caps (20 per account, 150 per group, 500 in all) bound the Gemini cost — Phase 7
- ✓ `/autolockdown ai [N|on|off]` turns the AI trigger on or off and sets its threshold (3 to 500) with the same live authority and anonymous-admin proof as the other two triggers; the alert names "AI spam" and its count and lists the recent joiners behind it; the Anthropic API key and the Telegram file URL never reach a log or an error; a stdlib-only client and a spike tool for the owner's own accuracy, latency and cost run ship with it — Phase 7
- ✓ An admin can set a group's captcha mode to Turnstile (`/captchamode turnstile`, beside math and text). A new member is muted on joining and gets one message whose button opens the bot's captcha page as a Telegram Mini App in their own language (7 languages); when the web captcha is not set up, Redis does not answer or the bot has no username, the member gets the group's math captcha instead — Phase 8
- ✓ Only the member the challenge was issued to can pass it, and only the bot's server decides: Telegram's signed user data (HMAC, 5-minute window, user must match) plus Cloudflare's server-side verification of the token, bound to hostname, action and the challenge. A wrong member, a reused token, a forged button or a page reporting a pass by itself never unmutes anyone; a pass unmutes, replays held messages, greets and cleans up exactly like the button captcha — Phase 8
- ✓ A provably failed check or a timeout applies the group's failure action (kick by default, so the member can rejoin), exactly once whichever replica handles the join, the check or the timer; Cloudflare, network and setup faults retry and never kick, and a run of five failed solves with no pass is held back as a suspected key mismatch — Phase 8
- ✓ The owner can deploy the captcha page from a documented guide (public HTTPS hostname, BotFather Mini App, Turnstile keys, reverse-proxy rules), the Turnstile secret is registered with the log redactor and never logged, the verify route is rate-limited and hardened, and the developer `/stats` shows the web captcha's counters — Phase 8

- ✓ `/settings` in a group asks the admin to open the menu in the group or in private (a deep link `/start settings_<chat id>`). A menu opened in the group answers only its opener; a private menu manages the group it was opened from. One message is edited in place with Back and Close on every page, and every press re-checks live that the presser is still a creator or administrator of that group (anonymous admins re-enter through the proof tap) — Phase 9
- ✓ The menu has a Staff Group page (the `/staff` panel read-only inside a Staff Group; Link and Unlink for the owner of both groups, with a two-tap card bound to the object it showed), a Lockdown page (state, a Lift card, an on/off switch and a stepper for each of the join, flood and AI triggers), a Captcha page (switch, mode, time limit, failure action) and an Antiflood page (limit, action, delete switch) — Phase 9
- ✓ A setting changed in the menu is the same code path as the command: one apply function per setting holds the validation, the refusals, the write and its cache invalidation, and parity, drift and authority-parity tests prove the stored row, the cache and the refusals match. A menu opened in the group, or in private by an admin who is not the group's creator, never names the Staff Group — Phase 9

### Active

<!-- Current scope. Hypotheses until shipped and validated. Order reflects priority. -->

All nine phases of the first milestone are shipped. No active requirements remain; follow-ups and deferred live checks are in STATE.md.

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
| Lockdown = kick joiners + mute all except admins and approved users; admin-only manual lift | Stops a raid immediately and leaves the "safe again" call to a human | ⚠️ Revised — Phase 4: approved users are muted too (Telegram has no per-user exception to a locked default, spike 1, D-01/D-02); joiners are banned until the lift instead of kicked; the manual lift shipped |
| Rules trigger lockdown; AI only assists | Fast and cheap enough to react to hundreds of joins; AI where judgement is needed | ✓ Shipped — Phases 6 and 7 (join surge, flood, AI burst; AI only adds to a count) |
| Cloudflare Turnstile through a bot-hosted Mini App page | More private and less intrusive than reCAPTCHA; server-side verification | ✓ Good — Phase 8 (live Telegram and Cloudflare checks deferred, `08-SPIKE.md`) |
| `/staff` panel first, folded into the settings menu later | Ships the Staff Group without waiting for the settings menu | ✓ Good — panel Phase 1; menu page Phase 9 (read-only panel inside a Staff Group, Link and Unlink elsewhere) |
| Order: Staff Group → raid protection → web captcha → settings menu | Owner's priority | ✓ Good — Phases 1-9 shipped in this order |
| Every staff action needs a Confirm tap showing the resolved target | `@usernames` can be stale or recycled; owner prefers safety over speed | ✓ Shipped — Phase 2 (5-minute card, issuer-only Confirm) |
| Staff Group members aren't protected from staff actions; only each group's admins and owner, and the bot, are | Owner's call; staff are trusted, not immune | ✓ Shipped — Phase 2 |
| Undo-everywhere button and a recent-actions list in `/staff` are in v1, backed by an audit record | Mistakes are reversible and accountable | ✓ Shipped — Phase 3 (live UAT passed) |
| Old `/antiraid` is retired; its auto-threshold carries over to join-surge detection | One raid system, not two | ✓ Shipped — Phase 6 (carry-over clamped to 3..500, D-18/D-22) |
| Auto-triggers are on by default with conservative thresholds | Protection from day one; admins tune or disable per group | ✓ Shipped — Phase 6 (15 joins in 60 s; 30 items in 30 s); live tuning in real groups still to be checked |
| Images are classified by Claude Haiku 5.5 (`claude-haiku-5-5`) through the Anthropic API (owner's key); text stays on TypeSafe | Owner's choice of provider; TypeSafe is text-only. Replaced Gemini `gemini-3.5-flash-lite` in quick task 261030-rig | ✓ Shipped — Phase 7 (REST client, no new dependency), provider switched in quick 261030-rig; the request shape is checked only against test doubles, so accuracy, latency and cost are not yet measured and the owner runs `scripts/claude-spike` with a real key |
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
| The lockdown snapshot is the raw `getChat` permissions JSON, replayed verbatim with `use_independent_chat_permissions` | The typed struct's `omitempty` bools lose rights that were explicitly off; the lift must restore exactly | ✓ Good — Phase 4 (UAT test 3) |
| Lockdown state lives only in PostgreSQL; the join guard records and a DB-driven worker on every replica makes every Telegram write, paced fleet-wide | Survives restarts and Redis loss, and a raid never pins a handler | ✓ Good — Phase 4 (UAT test 11) |
| A lockdown's own ban is marked by a 330-day `until_date`, and the lift unbans only joiners whose live ban carries it | A deliberate `/ban` or staff ban placed during the lockdown must survive the lift | ✓ Good — Phase 4 (UAT tests 6 and 10); bans run out after 330 days, an accepted limit |
| A request approved from Telegram's own list during a lockdown may still be banned first (D-24); the greetings Accept button refuses during a lockdown (D-25) | Accepted residual race; `/unlockdown` unbans them | ✓ Good — Phase 4 (UAT tests 5 and 8) |
| Alert buttons use stateless `lockdown|v1` callbacks (copy row ID, asker and expiry minute in the data) and judge every press by a live `getChatMember` of the locked chat, never Staff Group membership | Works without Redis, from the log channel too, and a forged or old button can reach nothing | ✓ Good — Phase 5 |
| Every alert, reminder, close, ban-run and ban-result delivery is a database row claimed with a conditional update and resumed by the worker on any replica | A restart, a 429 or a dead replica never loses or doubles a notice (duplicate accepted over lost after a crash) | ✓ Good — Phase 5 (code review WR-01 made the ban result resumable too) |
| "Ban N recent joiners" bans are permanent, outside the staff audit and not undoable; only bot-made invite links get a Revoke button, one revoke per lockdown | Owner's call (D-10, D-11, D-18); a cut-short link cannot be revoked by the bot | ✓ Good — Phase 5 (live UAT accepted by the owner) |
| Automatic triggers share one start path with `/lockdown` (`startLockdownRow`); exactly one lockdown and one alert come from `uk_chat_lockdowns_active`, `lockdown.Start` and `lockdown.BeginAlert`, and the Redis fire latch only sheds load | Counters run on every replica and Redis can lose data, so the database decides, never Redis | ✓ Good — Phase 6 (cross-replica proof is a PostgreSQL test run in CI) |
| Detection counters are Redis-only (Lua scripts keyed by `{chat}`); without Redis the automatic triggers do nothing and `/lockdown` still works | Counting is best effort and must never gate a ban or block a handler; a PostgreSQL join log was declined for its per-join cost | ✓ Good — Phase 6 |
| The flood counter is two watchers: group `-4` counts everything but slash-prefixed texts, group `12` counts the slash texts no command took | A handled command ends the update, so the dispatcher itself says which commands the bot handles, with no second command list | ✓ Good — Phase 6 (review WR-01, no reflection) |
| Notices and the "cannot lock" warning are sent from recovered, drained goroutines; the lock itself stays synchronous | A slow pacer slot must not hold the join guard or the message pipeline | ✓ Good — Phase 6 (review WR-03, `StopRaidPosts` drain) |
| A flood lockdown saves the flooders as its alert burst; the settings table carries no backup/export/import | Flooders who joined hours before the lock must be on the alert and in "Ban N recent joiners"; owner kept the new settings out of backups for now | ✓ Good — Phase 6 (D-17, D-20) |
| The Gemini spike gate is deferred; the phase ships a spike kit (`scripts/gemini-spike/`, `07-SPIKE.md`) instead and `/gsd-ai-integration-phase` is not run | The spike needs the owner's key; the client is plain REST behind constants (0.85 bar, model id, thumbnails) the owner tunes from real results | ✓ Good — Phase 7 (owner's choice, "plan now, spike later") |
| The raid AI path has its own bounded pool, breakers, budgets and verdict cache, apart from `/aispam`'s queue | A 100-image raid must not shed real `/aispam` moderation, and an image flood must not burn the owner's Gemini quota | ✓ Good — Phase 7 (D-18, D-20; review WR-01 added the per-account cap) |
| The AI burst lists recent joiners only; established members' `/aispam` verdicts still count toward the threshold | "Ban N recent joiners" must never offer an established member | ✓ Good — Phase 7 (review WR-04) |
| `ENABLE_AISPAM=false` is the one global switch for all AI checks, text and images; no key means no outbound call | The owner must be able to stop every AI call with one setting | ✓ Good — Phase 7 (review WR-02) |
| A Turnstile attempt is an ordinary `captcha_attempts` row with a reserved `ts1:` answer, and the Redis token is only a handle | One attempt per member per group, the timer, the cleanup tick and restart recovery cover it unchanged; the pass path is shared with the button so the two cannot drift | ✓ Good — Phase 8 (D-01, D-25; both button callbacks refuse a `ts1:` attempt) |
| Pass, failed solve and timeout are decided only by the conditional delete of the attempt row; Redis holds handles, rate windows and replay markers, never the decision | Exactly once on any replica, as for lockdown alerts | ✓ Good — Phase 8 (D-16; review WR-01/WR-02/WR-03 hardened the lost-claim, replay and consume paths) |
| Only a provable failed solve kicks; Cloudflare, network and setup faults retry, and a streak of five failed solves with no pass is held back as a suspected key mismatch | A bad site key and secret pair must not mass-kick real joiners | ✓ Good — Phase 8 (D-28; review WR-04 added the breaker) |
| The live Telegram and Cloudflare checks are deferred; the phase ships a checklist (`08-SPIKE.md`) and the self-hosting guide | The checks need the owner's hostname, BotFather Mini App and Turnstile keys | ✓ Good — Phase 8 (owner's choice, "plan and build now, spike later") |
| The verify route shares the bot's HTTP port; the guide and two startup warnings tell the owner to forward only `/captcha*` through the public proxy | A path allow-list in the bot would be a second place to get wrong | ✓ Good — Phase 8 (D-31) |
| The menu is stateless and calls the commands' own apply functions (`applyCaptcha*`, `applyFlood*`, `applyAutoLockdown`, `applyLockdownLift`) | One setting, one code path: the menu can never drift from the command, and a press works on any replica with Redis down | ✓ Good — Phase 9 (D-12, D-13; parity, drift and authority-parity tests per setting) |
| Every menu press re-checks the presser with a live `getChatMember` of the configured group, and each control declares the right it needs | The cached admin list is never the authority; a demoted admin is refused on the next tap | ✓ Good — Phase 9 (D-04, D-05; `settingsRight*`) |
| Two-tap cards (Lift, Unlink) carry the object they were drawn for and an expiry, with no timer and no Redis key | A stale Yes can never lift a newer lockdown or unlink a newer link | ✓ Good — Phase 9 (D-33, from review WR-01) |
| A group menu, and a private menu of an admin who is not the group's creator, never show the Staff Group's title or ID | Phase 1's rule that nothing about a link reaches the linked group's members, since a private menu is shown to every live admin | ✓ Good — Phase 9 (D-21 refined by D-32, from review WR-02) |
| The antiflood first-write side effect (a first action or delete press stores limit 5 and turns antiflood on) is kept in the repository and announced in the menu | Fixing the repository would change `/setfloodmode` and `/delflood` too | ⚠️ Revisit — Phase 9 (D-27, D-34); the repository fix is a separate follow-up |

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
*Last updated: 2026-10-30 after Phase 9 (all phases of the first milestone shipped)*
