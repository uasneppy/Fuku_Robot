# Feature Research

**Domain:** Telegram group-management bot (GroupHelp-style): Staff Group multi-group moderation, raid lockdown, Mini App (Cloudflare Turnstile) captcha, inline-button settings menu
**Researched:** 2026-10-04
**Confidence:** MEDIUM overall. Staff Group and settings-menu behaviour in GroupHelp and Rose is mostly from training knowledge and search snippets, because the egress proxy blocked teletype.in, missrose.org, core.telegram.org and developers.cloudflare.com. Turnstile token rules and Mini App constraints were confirmed by search results. Per-area confidence is marked below.

**Scope note:** PROJECT.md decisions are treated as fixed. This file says how each area typically works, what users of such bots expect, and what PROJECT.md misses. Anything missing is in "Gaps in PROJECT.md" at the end.

**What already exists (from ARCHITECTURE.md and a code check):**
- `antiraid` is Redis-only, time-limited, with an auto-threshold. It will be replaced.
- Captcha has `captcha_mode` of `math` or `text`, a 1-10 minute timeout, a `kick`/`ban`/`mute` failure action, and `max_attempts`.
- `aispam`, `antiflood` (per-replica counters), federations (`applyActiveFban` is the fan-out model), log channels, `/connect` (PM management) and `callbackcodec` all exist.
- There is no settings menu and no `web_app` or Mini App code.

## Feature Landscape

### Area 1: Staff Group (multi-group moderation) — confidence MEDIUM

How it typically works. GroupHelp has a "Staff Group" concept: a group of trusted people linked to the managed groups. Rose and Combot have no equivalent; they use federations (a shared ban list), which is a different model and is correctly kept separate. Typical flow:
1. The owner designates the group.
2. Managed groups link to it.
3. A staff member runs a moderation command in the staff room.
4. The bot fans the action out and reports back.

#### Table Stakes (Users Expect These)

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| `/ban /mute /kick /unban /unmute` with timed variants, by `@username`, ID, or reply | Same verbs as per-group moderation; staff already know them | MEDIUM | Reuse the existing duration parser and target extraction. Telegram treats `until_date` under 30 s or over 366 days as permanent. `banChatMember` works on user IDs that are not in the chat, so staff bans work for people who never joined that group. This is different from `/fban`, which only touches chats where the user is "known". |
| Per-group admin + right check before acting (fixed by PROJECT.md) | The whole safety model | MEDIUM | Check the issuer live via `getChatMember` on each target group. Do not use the 10 s cached admin list as the authority for a destructive cross-group action. Needed right is `can_restrict_members`. Creator always passes. |
| Per-group result summary (done / skipped / failed + reason) | Partial success must be visible | MEDIUM | Post one message and edit it as groups complete, rather than N messages. Keep it under 4096 characters; truncate with a count. |
| Protect untouchable targets | Every moderation bot refuses these; the bot must not let staff ban admins, the owner, other staff members, itself, or other bots it manages | LOW | Skip per group with the reason "target is an admin there". Telegram also rejects banning a group owner or admin, so handle that error as a skip, not a failure. |
| Live Staff Group membership check at command time | Removing someone from the Staff Group must revoke their power immediately | LOW | Use `getChatMember` on the Staff Group, status `member`/`administrator`/`creator`. Do not cache. Reject `left`/`kicked`/`restricted` (unless restricted but still a member). |
| Anonymous-admin handling | Anonymous staff cannot be authorised per group | LOW | Reply once: "post as yourself". Do not guess. Already in PROJECT.md Context. |
| Bot-side failure classification | "Bot lacks rights" differs from "you lack rights" | LOW | Telegram errors map to failed (bot-side) vs skipped (user-side). The summary distinguishes them. |
| Rate-limit-safe fan-out | 20+ groups x many API calls hits 429 | MEDIUM | Sequential or small worker pool, honour `retry_after`, retry once. Report "failed: rate limited" rather than dropping. Telegram's global limit is roughly 30 requests per second. |
| Reason shown in the summary and posted to each affected group's log channel | Audit expectation | LOW | Existing `logchannels` repo. Post only to groups where the action was applied. |
| `/staff` panel showing linked groups and status | Owner needs to see what is linked and what is broken | LOW-MEDIUM | Status per group: linked, bot admin yes/no, `can_restrict_members` yes/no, owner still matches. |
| Link / unlink flow with owner-only gating (fixed by PROJECT.md) | Authority safety | MEDIUM | The link command runs in the group being linked, issued by a user who owns both. Check the creator of both groups via `getChatMember`. |
| Auto-unlink when ownership no longer matches | Fixed by PROJECT.md | MEDIUM | Event-driven (`chat_member` updates, which need `allowed_updates` to include `chat_member` and the bot to be admin) plus a lazy re-verify on each fan-out. The lazy check is what actually guarantees the rule. |
| Persistent audit record of staff actions | Without it you cannot answer "who banned X everywhere, when, why" and cannot build undo | MEDIUM | New table (migration + model + repo per AGENTS.md). Store issuer, target, action, reason, expiry, per-group outcomes. Not in PROJECT.md; needed anyway for the summary. |
| Handle chat migration (group becomes supergroup, chat ID changes) | A basic group upgrading to a supergroup gets a new ID and the old link goes dead silently | MEDIUM | Handle `migrate_to_chat_id`/`migrate_from_chat_id`: re-key the link rows. Applies to the Staff Group and to linked groups. Not in PROJECT.md. |

#### Differentiators (Competitive Advantage)

| Feature | Value Proposition | Complexity | Notes |
|---------|-------------------|------------|-------|
| "Undo everywhere" button on the summary | One tap reverses a mistaken ban across all groups | LOW-MEDIUM | Needs the audit record. Callback carries a short token (64-byte cap), full state in Redis/DB. Gate to Staff Group members, re-check live. |
| Silent mode: delete the staff command message after acting | Keeps the staff room clean (GroupHelp offers silent variants) | LOW | Needs the bot to have delete rights in the Staff Group. |
| Staff Group as the alert sink (lockdown alerts, later reports) | Core value: "protect everything from one place" | LOW (given Phase 1) | Already planned for lockdown. Reports and AI-spam notices can reuse the same sink later. |
| Re-apply active timed actions to a newly linked group | A user banned yesterday is not banned in a group linked today | MEDIUM | Only meaningful if the audit table stores permanent/active bans. Makes it behave like a ban list, which blurs the line with federations. Recommend defer; decide at roadmap time. |
| Progress edit during long fan-out | Staff see it working instead of silence | LOW | Comes free with the single-message-edited summary. |
| `/staff` also shows recent staff actions | Accountability | LOW | Reads the audit table. |

#### Anti-Features (Commonly Requested, Often Problematic)

| Feature | Why Requested | Why Problematic | Alternative |
|---------|---------------|-----------------|-------------|
| Staff members can change settings, promote or demote in linked groups | "Staff group means full control" | Authority creep. The safety model rests on per-group admin rights. | Staff commands stay limited to restrict-type actions. Settings changes stay with each group's own admins (and the settings menu). |
| Staff Group auto-grants itself admin in linked groups | Convenience | Defeats the ownership guarantee | Bot never promotes anyone. |
| Confirmation dialog before every staff action | Safety | Slows the incident response this feature exists for | Undo button plus the per-group summary. |
| Merging with federations | Overlap in function | Out of Scope in PROJECT.md. Different trust model. | Leave `/fban` untouched. |
| Cross-group message deletion | Spam cleanup | Out of Scope for v1 | Revisit after the lockdown "ban recent joiners" action exists. |
| Per-action group selection | Flexibility | Out of Scope in PROJECT.md | All linked groups, always. |

### Area 2: Raid / anti-spam lockdown — confidence MEDIUM

How it typically works. Rose's antiraid is time-boxed: it bans every new join for a configurable period (default 6 hours) and can auto-enable at N joins per minute. GroupHelp and similar bots add a flood and spam layer on top. The decided design here differs deliberately: it kicks joiners, mutes non-admins, never auto-lifts, and has a manual-only lift. That is closer to Discord "lockdown" semantics.

#### Table Stakes (Users Expect These)

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| Manual `/lockdown` and `/unlockdown` (or `/lockdown off`) plus status | Staff need a panic button and a plain-text path besides the button | LOW | Admin of that group only (rights check, not Staff Group). Keep the button path from PROJECT.md. |
| Auto-trigger on join surge (N joins in T seconds, configurable) | The classic raid signal; Rose already offers it, so the existing `AutoAntiRaidThreshold` setting must carry over | MEDIUM | Sliding window in Redis (sorted set per chat). Minimum sample size and a per-group enable switch to avoid false positives on legitimate spikes (an event or channel promotion). |
| Auto-trigger on message or media flood from new members | Raids that already joined (or slipped in) flood text, photos, GIFs, stickers | MEDIUM-HIGH | Needs chat-level aggregate counters, not antiflood's per-user, per-replica ones. Use Redis. Needs a "new member" notion: join time per user per chat (see Gaps). |
| Auto-trigger on burst of AI spam verdicts | Already decided | MEDIUM | Count `aispam` positive verdicts per chat per window. Rules fire, AI assists (decided). |
| Lockdown action: kick (not permanent ban) every joiner | Decided. Kick means ban then unban so honest users can come back | MEDIUM | Process through a paced queue; hundreds of joins per minute is plausible. Delete the join service message too. |
| Lockdown action: mute everyone except admins and approved users | Decided | MEDIUM-HIGH | The reliable mechanism is a chat-level default permissions flip (`setChatPermissions`, one call), which admins ignore. Approved users then need per-user permission overrides, or they are muted too. Snapshot the previous default permissions and restore them on lift. A restore that guesses defaults is a classic bug. |
| Durable lockdown state | A lockdown that never expires must survive a restart | MEDIUM | DB row (migration) as the source of truth, Redis as accelerator. The old antiraid was Redis-only; reusing that would lose lockdown on a Redis flush. |
| Alert with trigger reason, counts, window, and a sample list of joiners | Staff must know why it fired and who | MEDIUM | Telegram messages cap at 4096 characters: cap the sample and show "+N more". Full list as a document if needed. |
| Alert to Staff Group and the attacked group's log channel (decided) | Core value | LOW | One alert per lockdown, then edit it with updated counters (throttled), rather than re-alerting every join. |
| "Lift lockdown" button, admin of the locked group only, from anywhere (decided) | Decided | MEDIUM | The callback must verify admin rights in the locked chat, not the chat where the button was pressed. Callback data carries a short token; the chat ID lives in the DB or Redis (64-byte cap). Re-check on every press. |
| "Ban N recent joiners" button (decided) | The standard post-raid cleanup | MEDIUM | Join log per chat (Redis sorted set or DB) with retention at least as long as the longest window. Exclude admins and approved users and anyone who joined before the burst window. Admin-gated like lift. Show the result summary and offer undo (unban list). |
| Exempt admins and approved users | Repo rule: approved users skip antiflood, locks, blacklists, captcha | LOW | Applies to detection counters as well as the mute. |
| One active lockdown per chat, idempotent trigger | Concurrent triggers (surge plus flood) must not double-alert or double-snapshot permissions | LOW-MEDIUM | Compare-and-set on the state row. |
| Migrate or replace legacy `/antiraid` settings | Existing users of `raidtime`, `raidactiontime`, auto threshold | LOW-MEDIUM | Keep the auto-threshold meaning, retire the temp-ban timing, and reply to old commands with a pointer. |

#### Differentiators (Competitive Advantage)

| Feature | Value Proposition | Complexity | Notes |
|---------|-------------------|------------|-------|
| Alert-only (dry-run) mode for auto triggers | Lets the owner tune thresholds without taking a false-positive lockdown | LOW | One extra setting. Recommended: it is the cheapest false-positive control. |
| Invite-link attribution in the alert | `ChatMemberUpdated` carries `invite_link`; showing "N of the burst joined through link X" and offering "revoke link" is very actionable | MEDIUM | Needs the bot to see chat-member updates. Strong differentiator; optional first-release add. |
| Periodic "lockdown still active" reminder | Because nothing auto-lifts, a forgotten lockdown silently mutes a community | LOW | Edit/re-post the alert at intervals (for example 1 h, then daily). It respects "never auto-lifts". |
| Lockdown banner in `/staff` panel | One place to see which groups are locked | LOW | Depends on Phase 1 panel. |
| Account-signal scoring (no username, no photo, very new ID, same-name pattern) feeding rules | Raiders share traits; this narrows "ban N recent joiners" to likely bots | MEDIUM | Keep as rule signals, not a black box. Telegram user IDs increase over time, so a high ID is a usable "new account" heuristic. Defer past first release. |
| External reputation lookup on join (for example the CAS database) | Cheap extra signal | LOW-MEDIUM | External dependency and privacy trade-off. Defer. |
| Cross-group correlation: a raid on group A raises sensitivity on linked group B | Natural fit with the Staff Group | MEDIUM | Out of Scope today ("applies only to the attacked group"). Mention only. |

#### Anti-Features

| Feature | Why Requested | Why Problematic | Alternative |
|---------|---------------|-----------------|-------------|
| Auto-lift after a timer | Convenience | Out of Scope: an admin must decide | Periodic reminder. |
| Permanent-banning every joiner during lockdown | Strong stop | Honest joiners lose access forever, and lifting needs mass unban | Kick, as decided. |
| Send every message to AI for raid detection | "Smarter" | Out of Scope: slow and costly | Rules first, AI on borderline items only. |
| Lockdown across all linked groups at once | Seems protective | Out of Scope; one attacked group should not silence the rest | Alert the Staff Group and let staff lock others by hand. |
| Lockdown that mutes admins or approved users | Simplicity | Locks out the people who must fix it | Always exempt. |
| Public "raid mode" announcement spam in the group | Transparency | Tells raiders it worked and adds noise | Alert staff and log channel only. |

### Area 3: Mini App captcha with Cloudflare Turnstile — confidence MEDIUM-HIGH for constraints, MEDIUM for UX norms

How it typically works. Classic Telegram bots (Shieldy, GroupHelp, Rose) use in-chat challenges: a button press, a math question, or an image. A Mini App captcha moves the challenge to a web page opened from the group.

Hard constraints (confirmed by search results):
- Inline `web_app` buttons are available only in private chats between a user and the bot. They are not usable in groups.
- In a group, the only route is a URL button pointing to a direct Mini App link (`https://t.me/<bot>/<appshortname>?startapp=<param>`). The Mini App has to be registered once with BotFather (`/newapp`), which is a manual deployment step.
- `initData` is signed with an HMAC derived from the bot token and carries `auth_date`; the server must validate the signature and reject stale `auth_date`.
- Turnstile tokens expire after 300 seconds and are single-use; a replay returns `timeout-or-duplicate`. Server-side siteverify (POST `https://challenges.cloudflare.com/turnstile/v0/siteverify`) is mandatory.

#### Table Stakes (Users Expect These)

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| Restrict on join, unrestrict on pass | Standard captcha contract; the existing flow already mutes pending users | LOW (exists) | Webapp becomes a new `captcha_mode` value (requires a migration changing the `chk_captcha_mode` check constraint, and a new migration file, never an edit). |
| Challenge message with a URL button (not `web_app`) in the group | Only option that works in groups | LOW-MEDIUM | `startapp` param is an opaque short nonce, not user or chat data. Param limits are small. |
| Challenge bound to one `(user, chat)` and one nonce | Other members must not be able to verify for the target, and a nonce must not be replayable | MEDIUM | Nonce row in Redis/DB with TTL equal to the captcha timeout. The server checks `initData.user.id` equals the challenge's user. Mark the nonce used atomically on success. |
| Server-side validation of both `initData` and the Turnstile token | Mandatory; client-side is spoofable | MEDIUM | Check HMAC signature, `auth_date` freshness, user ID match, then siteverify. Verify `success`, the hostname, and bind the Turnstile `cdata`/action to the nonce so a token solved elsewhere cannot be reused. |
| Timeout then kick, rejoin allowed (decided) | Matches existing sweeper | LOW (exists) | Existing `timeout` (1-10 min) and `failure_action` settings continue to apply. |
| Delete challenge message and join service message on result | Keeps the chat clean | LOW | Already done by the math/text flow. |
| Pending-message replay and greeting after pass | Existing behaviour in group `-10` and greetings | MEDIUM | The HTTP handler must hand off into the same "verified" code path rather than duplicating it. Unify on a single function. |
| Retry inside the page without consuming the attempt | Turnstile can fail transiently (network, widget error) | LOW-MEDIUM | The existing "one attempt per user per chat" rule must be read as one challenge window, not one widget solve. Decide explicitly. |
| Rate limit and size limit on the verify endpoint | A public endpoint on the bot's HTTP server | LOW-MEDIUM | Per-IP and per-nonce limits. Unknown nonce returns the same response as an invalid one. |
| HTTPS public hostname with correct CSP and Turnstile domain allow-list | Mini Apps require HTTPS; Turnstile checks the hostname | LOW (ops) | Polling-mode deployments may not expose a public port today. This is a deployment prerequisite; flag it. |
| Register the Turnstile secret with `logredact.RegisterSecret` | Repo rule | LOW | Also never log the token or `initData`. |
| Keep math/text captcha as selectable fallback | The Mini App cannot be a single point of failure (Cloudflare outage, misconfigured domain, bot hosting down) | LOW | The PROJECT.md wording ("fit that flow or replace it cleanly") should resolve to "add a mode, keep the old ones". |

#### Differentiators (Competitive Advantage)

| Feature | Value Proposition | Complexity | Notes |
|---------|-------------------|------------|-------|
| Invisible / managed Turnstile mode so most real users see no puzzle | The privacy and low-friction reason Turnstile was chosen | LOW | A widget setting. |
| Mini App localised via Telegram `language_code` (7 locales) | Existing 7-locale promise | MEDIUM | The page needs its own small string table; the repo's `check-translations` does not cover it. |
| Theme-aware UI, auto-close after success | Feels native | LOW | `Telegram.WebApp.themeParams`, `close()`. |
| Join-request mode: verify before admission | User never enters the group, so there is nothing to clean up on failure | MEDIUM-HIGH | Uses `chat_join_request`. Needs a different state machine. Defer. |
| Verify-time metrics (pass rate, median time, timeouts) | Tune timeouts, detect Turnstile outages | LOW | Existing Prometheus endpoint. |

#### Anti-Features

| Feature | Why Requested | Why Problematic | Alternative |
|---------|---------------|-----------------|-------------|
| Google reCAPTCHA | Familiar | Out of Scope; privacy | Turnstile. |
| `web_app` inline button in the group | Looks native | Telegram does not allow it in groups | URL button to the direct Mini App link. |
| Trusting client-reported success (page POSTs "passed") | Simple | Trivially forged | Server-side siteverify plus `initData` validation only. |
| Fingerprinting, IP storage, or passing `remoteip` as a tracking tool | Stronger bot detection | Privacy cost, contradicts the reason for choosing Turnstile | Do not store IPs; use IP only for transient rate limiting. |
| Re-verifying existing members | Completeness | Hostile, no value | Applies to new joiners only; approved users skip (repo rule). |
| Self-hosting `telegram-web-app.js` | Offline control | Telegram expects its hosted script; self-hosted copies go stale | Load from Telegram, keep CSP narrow. |
| Full web admin dashboard on the same server | "While we have a web server" | Attack surface and scope creep | Settings menu stays in Telegram. |

### Area 4: Inline-button settings menu — confidence MEDIUM

How it typically works. GroupHelp's `/settings` opens a menu of buttons. Pages cover language, welcome, rules, captcha, antiflood, warns, locks, blocks, and so on. Navigation edits one message in place with a back button. Toggles show a check or cross. Numeric values use stepper buttons or presets. In GroupHelp, setup happens mostly in the bot's private chat; Rose's help and `/connect` use the same private-chat model. This codebase already has `/connect` for PM management, so a PM menu bound to a connected chat is the natural fit.

#### Table Stakes (Users Expect These)

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| `/settings` in a group, opening the menu for that group | The entry point everyone looks for | MEDIUM | Either edit inline in the group (only invoker may press) or send a deep-link button (`?start=settings_<token>`) to the bot's PM. Recommended: PM menu, because group chat stays clean and it fits the existing connections model. |
| Admin check on open and on every press | Buttons can be pressed by anyone who sees the message or after an admin is demoted | LOW-MEDIUM | The check is `getChatMember` on the target chat at press time. Callback data carries a token or chat reference, never a trusted role claim. |
| Edit in place, Back / Close navigation | Standard UX | MEDIUM | Swallow Telegram's "message is not modified" 400 error, which happens when a press re-renders identical content. |
| Toggle state visible on the button | Users must see current state | LOW | Re-render from the DB each press so two admins editing at once see current data. |
| Steppers or presets for numeric and duration values | Free-text input is awkward | LOW-MEDIUM | Use presets (for example captcha timeout 1/2/5/10). Respect the existing DB check-constraint ranges. |
| Callback data via `callbackcodec`, 64-byte cap | Repo rule; an overflow ships a dead button | LOW | Keep payloads to `page` + `action` + short chat token. Anything larger sits behind a Redis token. |
| Writes go through the repository layer with cache invalidation | Repo rule | LOW | Menu must call the same repo functions as the commands, so settings made by command and by menu agree. Never write directly. |
| Pages for features this milestone touches: Staff Group, Lockdown/raid, Captcha, Antiflood | Menu must cover what is new | MEDIUM | Staff Group page is owner-only for link and unlink; admins can view status. |
| Localised labels in all 7 locale files | Repo rule | MEDIUM | Large key count; run `make check-translations`. |
| Group picker when opened in PM with several manageable groups | Admins of many groups need to choose | MEDIUM | Reuse `/connect`. A full "all groups I admin" list needs a reverse lookup from admin caches; consider listing only connected or recently active groups. |

#### Differentiators (Competitive Advantage)

| Feature | Value Proposition | Complexity | Notes |
|---------|-------------------|------------|-------|
| Settings changes posted to the group's log channel | Audit trail; GroupHelp-style logging | LOW | Existing log channel plumbing. |
| Staff Group page doubles as the `/staff` panel (decided) | Single place to see links and status | LOW (given Phase 1) | `/staff` remains as a shortcut. |
| Lockdown page with current state and Lift button | One place to react | LOW (given Phase 2) | Reuses the lift callback. |
| Free-text edit flows (welcome text, rules) with a Redis "awaiting reply" state | Full GroupHelp parity | MEDIUM-HIGH | Needs a pending-input state machine with TTL and cancel. Defer. |
| Reset-to-defaults per page with confirmation | Safety net | LOW-MEDIUM | Defer unless requested. |

#### Anti-Features

| Feature | Why Requested | Why Problematic | Alternative |
|---------|---------------|-----------------|-------------|
| Porting every command into menu pages (filters, notes, blacklist editing) | "Everything in the menu" | Large content-editing UX with high key count and low value | Keep list-style content as commands. |
| Menu actions that bypass command-level checks | Shortcut | Two inconsistent permission models | Share the permission predicates and repo calls with commands. |
| Menu state stored only in the callback data | Statelessness | 64-byte cap and spoofable | Short token plus server-side lookup. |
| Settings menu usable by non-admins in read-only mode | Transparency | Leaks config and adds a second render path | Admins only. |
| Auto-closing and deleting the menu on timers | Tidiness | Races with users mid-edit | Explicit Close button. |

## Feature Dependencies

```
Staff link (owner-of-both check, migration, repo)
    └──requires──> Live ownership/membership verification helper (getChatMember, uncached)
    └──requires──> Chat-migration re-keying (group -> supergroup)

Staff actions (/ban /mute /kick /unban /unmute)
    └──requires──> Staff link
    └──requires──> Target resolution (ID / known @username / reply)
    └──requires──> Paced fan-out worker (429 handling)
    └──enhances──> Audit table  ──enables──> Undo button, /staff recent actions

/staff panel ──requires──> Staff link, per-group status probe
Settings menu "Staff Group" page ──requires──> /staff panel logic

Lockdown state (durable DB row + Redis accelerator)
    └──requires──> Snapshot/restore of chat default permissions
    └──requires──> Join log (per chat, timestamps)  ──enables──> "Ban N recent joiners"
    └──requires──> Chat-level aggregate counters (Redis) for flood/media triggers
Lockdown triggers (join surge, flood, AI burst, manual) ──require──> Lockdown state
Lockdown alert ──requires──> Staff link (Staff Group sink) + log channel
Lift / Ban-joiners buttons ──require──> callbackcodec token store + live admin check on the locked chat
Lockdown replaces legacy antiraid ──conflicts──> old temp-ban timing (retire raidtime / raidactiontime)

Mini App captcha
    └──requires──> New captcha_mode value (migration)
    └──requires──> Public HTTPS route on bot HTTP server + BotFather /newapp registration + Turnstile site/secret keys
    └──requires──> Challenge nonce store (bound to user+chat, TTL)
    └──requires──> Shared "verified" code path with existing captcha (replay, greeting, cleanup)
Lockdown ──interacts──> Captcha: joiners are kicked before a captcha starts; running captchas are left alone

Settings menu ──requires──> callbackcodec, live admin re-check, repo layer
Settings menu pages for Lockdown / Captcha / Staff ──require──> those features exist (hence the decided order)
```

### Dependency Notes

- **Audit table sits under Staff actions, not in PROJECT.md:** the summary, undo button and "recent actions" all read from it. Add it in Phase 1 even if only the summary uses it at first.
- **Join log is shared by two features:** "Ban N recent joiners" and "new-member flood" detection both need it. Build it once in Phase 2, not per trigger.
- **Durable lockdown state conflicts with reusing legacy antiraid storage:** the legacy path is Redis-only with expiry; the new one is never expiring and DB-backed.
- **Captcha and lockdown order matters:** group `-10` (captcha sweeper) runs before antiraid at `-5`. The lockdown kick must not trigger the captcha timeout failure action twice, or leave a pending captcha row that later replays messages.
- **Settings menu last (decided):** it only wraps repos and permission checks that already exist. The one reason to pull part of it forward is a `/staff` panel built with menu primitives, so build the `/staff` panel with `callbackcodec` and in-place editing from the start.

## MVP Definition

### Launch With (v1) — maps to PROJECT.md order

Staff Group (Phase 1):
- [ ] Designate, link, unlink and auto-unlink with live owner checks. Reason: the safety model.
- [ ] `/ban /mute /kick /unban /unmute` (timed) fan-out with live per-group admin-right check. Reason: core value.
- [ ] Single edited summary message (done / skipped / failed). Reason: decided.
- [ ] Protected targets, anonymous-admin refusal, 429-safe pacing, chat-migration re-keying. Reason: table stakes missing from PROJECT.md.
- [ ] Audit table written on every action. Reason: cheap now, painful to retrofit.
- [ ] Reason posted to log channels; `/staff` panel with status.

Raid protection (Phase 2):
- [ ] Durable lockdown state with permission snapshot and restore.
- [ ] Kick joiners; mute non-admin, non-approved members.
- [ ] Triggers: manual, join surge, flood/media, AI burst; legacy threshold carried over.
- [ ] Alert to Staff Group and log channel with Lift and "Ban N recent joiners".
- [ ] Manual `/lockdown`, `/unlockdown`, status.

Web captcha (Phase 3):
- [ ] New `webapp` captcha mode alongside math/text.
- [ ] Nonce-bound challenge, `initData` validation, Turnstile siteverify, shared verified path.
- [ ] Timeout then kick, rejoin allowed.

Settings menu (Phase 4):
- [ ] `/settings` with admin re-check on each press.
- [ ] Pages: Staff Group, Lockdown, Captcha, Antiflood.

### Add After Validation (v1.x)

- [ ] Undo-everywhere button on staff summaries, once the audit table has real data.
- [ ] Alert-only (dry-run) mode and periodic lockdown reminder, after the first real raid shows what needs tuning.
- [ ] Invite-link attribution with "revoke link" in the alert.
- [ ] Settings-change logging to log channel.
- [ ] Silent staff commands (delete the command message).

### Future Consideration (v2+)

- [ ] Join-request (pre-admission) captcha.
- [ ] Free-text settings editing flows.
- [ ] Account-signal scoring and external reputation lookups.
- [ ] Re-applying staff bans to newly linked groups (revisit alongside whether this blurs into federations).
- [ ] Staff-driven message deletion across groups.

## Feature Prioritization Matrix

| Feature | User Value | Implementation Cost | Priority |
|---------|------------|---------------------|----------|
| Staff link/unlink with live ownership check | HIGH | MEDIUM | P1 |
| Staff fan-out ban/mute/kick (+ un-) with per-group rights check | HIGH | MEDIUM | P1 |
| Per-group summary message | HIGH | MEDIUM | P1 |
| 429-safe paced fan-out | HIGH | MEDIUM | P1 |
| Protected targets and anonymous-admin refusal | HIGH | LOW | P1 |
| Audit table | MEDIUM | MEDIUM | P1 |
| Chat-migration re-keying | MEDIUM | MEDIUM | P1 |
| `/staff` panel | MEDIUM | LOW | P1 |
| Durable lockdown state + permission snapshot/restore | HIGH | MEDIUM-HIGH | P1 |
| Join surge trigger + manual lockdown | HIGH | MEDIUM | P1 |
| Flood/media trigger from new members | HIGH | MEDIUM-HIGH | P1 |
| AI-verdict burst trigger | MEDIUM | MEDIUM | P1 |
| Alert + Lift + Ban N recent joiners | HIGH | MEDIUM | P1 |
| Turnstile Mini App captcha (full server-side validation) | HIGH | HIGH | P1 |
| Math/text fallback retained | MEDIUM | LOW | P1 |
| Settings menu with live admin re-check | MEDIUM | MEDIUM | P1 |
| Undo-everywhere button | MEDIUM | LOW-MEDIUM | P2 |
| Alert-only mode | MEDIUM | LOW | P2 |
| Lockdown reminder | MEDIUM | LOW | P2 |
| Invite-link attribution | MEDIUM | MEDIUM | P2 |
| Settings-change log posts | LOW | LOW | P2 |
| Silent staff commands | LOW | LOW | P3 |
| Join-request captcha | MEDIUM | HIGH | P3 |
| Account-signal scoring | MEDIUM | MEDIUM | P3 |
| Free-text settings editing | LOW | HIGH | P3 |

**Priority key:**
- P1: Must have for the milestone
- P2: Should have, add when possible
- P3: Nice to have, future consideration

## Competitor Feature Analysis

Competitor claims are from training knowledge and search snippets, not fetched documentation (see Sources). Treat as MEDIUM at best.

| Feature | Rose / Miss Rose | GroupHelp | Shieldy / Group Butler / Combot | Our Approach |
|---------|------------------|-----------|---------------------------------|--------------|
| Multi-group moderation | Federations (shared ban list; users must be known) | Staff Group concept; a staff command list exists in its docs (for example `multiban`, max 20) | Combot offers cross-chat anti-spam database; others are single-group | Staff Group fan-out with live per-group rights check; federations untouched |
| Raid response | `/antiraid`: temp-ban new joins for a set time (default 6 h), auto at N joins per minute | Flood, join and spam layers; settings via menu | Mostly captcha-only; spam bots lean on global ban lists | Lockdown: kick joiners and mute non-admins, never auto-lifts, admin-only lift, alerts to staff and log channel |
| Captcha | Button / math / text modes | Captcha with several modes | Shieldy: button, digits, image; Combot has a CAS global check | Cloudflare Turnstile via bot-hosted Mini App, server-side verified, old modes kept as fallback |
| Settings UI | Command-driven; help menu buttons | `/settings` button menu, mostly in PM | Mixed; Shieldy uses commands | PM menu with in-place edits, live admin re-check, callbackcodec |
| Alerts to a staff channel | Log channels | Staff/log groups | Log channels | Alert to Staff Group and log channel with action buttons |

## Gaps in PROJECT.md

Items users of these bots treat as table stakes that PROJECT.md does not mention, in rough order of risk:

1. **Chat migration (group to supergroup ID change).** Links, lockdown state and nonces keyed by chat ID go stale. Add re-keying to Phase 1 and test it for the Staff Group too.
2. **Chat-level permission snapshot and restore for lockdown.** "Mute everyone" via a default-permissions flip must save the previous permissions and restore exactly those on lift. Approved users need per-user overrides. Specify in Phase 2.
3. **Durable lockdown state.** The legacy antiraid is Redis-only and expires. A never-expiring lockdown needs a DB row. Specify in Phase 2.
4. **Join log and "new member" definition.** "Mostly from new members" and "Ban N recent joiners" both need per-chat join timestamps. Confirm whether the users tracker (group `-1`) records joins per chat; if not, add it in Phase 2 before the flood trigger.
5. **Chat-level, multi-replica flood counters.** Existing antiflood is per-user and per-replica; lockdown flood detection needs aggregate Redis counters.
6. **Staff audit record.** Needed for the summary, undo, and accountability.
7. **Protected targets** (admins, owner, other staff, bot) and 429-aware pacing in staff fan-out.
8. **Bot-removed or bot-demoted handling** for linked groups, surfaced in `/staff` instead of silently failing every action.
9. **Command alternatives to every button** (`/unlockdown`, `/lockdown status`) so a stale or lost alert never traps a group.
10. **Mini App deployment prerequisites.** BotFather `/newapp` registration, public HTTPS hostname, Turnstile domain allow-list and keys. Polling-mode deployments may need a new public port. Capture in Phase 3 setup and docs.
11. **Captcha retry semantics.** "One attempt per user per chat" needs an explicit rule for in-page Turnstile retries.
12. **`allowed_updates` must include `chat_member`** for event-driven ownership change and invite-link attribution. The bot must be admin to receive those updates.
13. **Legacy `/antiraid` migration.** Define what happens to existing `raidtime`, `raidactiontime`, and auto-threshold settings.

## Sources

- GroupHelp features overview and command list (staff, multiban): search results via MetricGram and Teletype command list; the teletype.in pages themselves were blocked, so detail is MEDIUM-LOW.
  - https://www.metricgram.com/alternatives/grouphelp
  - https://teletype.in/@grouphelp/security
  - https://teletype.in/@emiliandev/Liste-der-Group-Help-Befehle-10-14
- Rose antiraid behaviour (default 6 h temp-ban, auto-enable on join rate): search snippets of https://missrose.org/docs/anti-spam/antiraid/ (page blocked; MEDIUM).
- Telegram Mini App initData validation (HMAC with "WebAppData" key, `auth_date` freshness): https://docs.telegram-mini-apps.com/platform/init-data (search snippet, HIGH-MEDIUM).
- `web_app` inline buttons are private-chat only; `?startapp=` direct links: python-telegram-bot and aiogram `InlineKeyboardButton` docs plus Telegram bug tracker snippets (HIGH-MEDIUM). Primary page https://core.telegram.org/bots/webapps was blocked.
- Cloudflare Turnstile server-side validation (300 s validity, single use, `timeout-or-duplicate`, siteverify endpoint): https://developers.cloudflare.com/turnstile/get-started/server-side-validation/ (search snippet, HIGH-MEDIUM). The `idempotency_key` parameter was not confirmed; do not rely on it.
- Codebase facts: `/home/user/Fuku_Robot/.planning/PROJECT.md`, `/home/user/Fuku_Robot/.planning/codebase/ARCHITECTURE.md`, `alita/db/models/captcha.go`, `alita/modules/antiraid.go`, `alita/modules/connections.go`.

---
*Feature research for: Telegram group-management bot (Staff Group, lockdown, Mini App captcha, settings menu)*
*Researched: 2026-10-04*
