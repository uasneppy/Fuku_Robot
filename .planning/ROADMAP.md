# Roadmap: Fuku Robot

## Overview

Fuku Robot turns the Alita Robot fork into a GroupHelp-style bot for the owner's own communities, in the owner's priority order. First comes the Staff Group. Owners link their groups to one staff room (Phase 1). Staff then act on a bad actor in every linked group at once, behind a Confirm tap and live per-group admin checks (Phase 2), and every action becomes logged and reversible (Phase 3). Raid protection comes next: a durable lockdown that only an admin can lift (Phase 4), alerts with one-tap responses (Phase 5), rule-based auto-triggers that replace the old `/antiraid` (Phase 6), and AI that helps the rules without ever deciding alone (Phase 7). Then a Cloudflare Turnstile captcha served as a Telegram Mini App (Phase 8). Last, an inline settings menu brings it all together (Phase 9). Every phase ships an end-to-end slice the owner can try in real groups.

## Cross-Cutting Constraints

These apply to every phase. Planners carry them into each phase's must-haves.

- **AGENTS.md is the rulebook.** Migrations are append-only and timestamped. Every write calls `cache.DeleteCache`. Commands go through `helpers.WrapCommand` and callbacks through `callbackcodec` (64-byte cap). Tests use real fixtures, no mock libraries. `make generate-docs`, `make check-translations`, `make test` and `make lint` pass at the end of every phase. A new handler group, Redis key family or rule change updates `AGENTS.md` in the same commit.
- **The per-group admin check is never skipped.** Every action, button press and setting change re-checks the presser live against the group it affects.
- **PLAT-03 (mapped to Phase 1):** each phase adds its new messages and button labels to all 7 locale files.
- **PLAT-01 (mapped to Phase 2):** each later phase keeps its own state shared across replicas too. That covers lockdown state (Phase 4), detection counters (Phases 6 and 7) and captcha challenges (Phase 8).
- **PLAT-02 (mapped to Phase 7):** the Turnstile secret added in Phase 8 is registered with `logredact.RegisterSecret`, like the Gemini key.

## Phases

**Phase Numbering:**
- Integer phases (1, 2, 3): Planned milestone work
- Decimal phases (2.1, 2.2): Urgent insertions (marked with INSERTED)

Decimal phases appear between their surrounding integers in numeric order.

- [x] **Phase 1: Staff Group Links** - Owners designate a Staff Group and link the groups they own to it; `/staff` shows each link's health (completed 2026-10-05)
- [x] **Phase 2: Staff Actions Across Groups** - Staff ban, mute, kick, unban or unmute someone in every linked group at once, after a Confirm tap, with live per-group admin checks (completed 2026-10-05)
- [x] **Phase 3: Staff Audit and Undo** - Every staff action is posted to log channels, listed in `/staff`, and reversible with "Undo everywhere" (completed 2026-10-06)
- [x] **Phase 4: Manual Lockdown** - `/lockdown` removes joiners and mutes non-admins in one group; `/unlockdown` restores its permissions exactly (completed 2026-10-08)
- [x] **Phase 5: Lockdown Alerts and Response** - Each lockdown alerts the Staff Group and log channel with Lift, "Ban N recent joiners" and "Revoke link" buttons (completed 2026-10-18)
- [x] **Phase 6: Automatic Raid Detection** - Join surges and new-member floods lock a group automatically; the old `/antiraid` is retired (completed 2026-10-29)
- [ ] **Phase 7: AI-Assisted Raid Detection** - TypeSafe (text) and Gemini (images) spam verdicts feed a burst trigger, while rules still decide
- [ ] **Phase 8: Turnstile Web Captcha** - New members verify through Cloudflare Turnstile in a bot-hosted Telegram Mini App
- [ ] **Phase 9: Settings Menu** - `/settings` opens an inline-button menu with Staff Group, Lockdown, Captcha and Antiflood pages

## Phase Details

### Phase 1: Staff Group Links

**Goal:** As a community owner, I want to make one group my Staff Group and link my groups to it, so that staff have one control room.
**Mode:** mvp
**Depends on:** Nothing (first phase)
**Research flag:** During phase research, confirm whether Telegram sends `chat_member` updates when group ownership is transferred. If it doesn't, the live owner checks (on `/staff`, and from Phase 2 before every staff action) are the only thing that removes stale links.
**Requirements:** SETUP-01, SETUP-02, SETUP-03, SETUP-04, SETUP-05, SETUP-06, SETUP-07, SETUP-08, PLAT-03
**Success Criteria** (what must be TRUE):
  1. A group's owner can make that group the Staff Group. A channel, or a group that can't be used, is refused with a reason. Only the Staff Group's owner can remove the status, and removing it unlinks all its groups.
  2. A user who owns both a group and the Staff Group can link and unlink that group. Anyone who doesn't own both (checked live) is refused with a reason, and so is a group that's already linked to a Staff Group.
  3. When ownership of either group passes to someone else, the link disappears without anyone running a command.
  4. A link and the Staff Group status keep working after a group upgrades to a supergroup and gets a new chat ID.
  5. `/staff` in the Staff Group shows the help text and each linked group's status: linked, bot is admin, bot can restrict members, owner still matches. All its text exists in all 7 languages. The in-lockdown status is added when Phase 4 ships.

**Plans:** 10/10 plans complete

Plans:
**Wave 1**
- [x] 01-01-PLAN.md: Walking skeleton. `/setstaff` makes a Staff Group and `/staff` shows it (schema, repository, live owner check, test harness, locale parity test)

**Wave 2** *(blocked on Wave 1 completion)*
- [x] 01-02-PLAN.md: `/unsetstaff` with confirm, plus the full `/setstaff` refusal matrix (anonymous, channel, bot not admin, linked group)
- [x] 01-03-PLAN.md: Chat-migration re-key, so the Staff Group and its links survive a supergroup upgrade (StaffWatchers module, group -3)
- [x] 01-04-PLAN.md: D-10 role exclusivity in PostgreSQL. Decision checkpoint, then a trigger with an advisory lock and a CI-only test

**Wave 3** *(blocked on Wave 2 completion)*
- [x] 01-05-PLAN.md: Linking by `/linkstaff [id]` and the "Add group" picker, with live owner-of-both checks and quiet linked groups

**Wave 4** *(blocked on Wave 3 completion)*
- [x] 01-06-PLAN.md: Unlinking by `/unlinkstaff` and the Unlink button with confirm

**Wave 5** *(blocked on Wave 4 completion)*
- [x] 01-07-PLAN.md: Auto-unlink on ownership change (service messages and chat_member; one recheck core; exactly-once notices)

**Wave 6** *(blocked on Wave 5 completion)*
- [x] 01-08-PLAN.md: Bot-health tracking, with one heads-up per change, including recovery

**Wave 7** *(blocked on Wave 6 completion)*
- [x] 01-09-PLAN.md: Live `/staff` panel with per-group statuses, Refresh in place, paging and length cap
- [x] 01-10-PLAN.md: Hourly and startup sweeper behind a Redis lock, wired into startup and shutdown

### Phase 2: Staff Actions Across Groups

**Goal:** As a staff member, I want to ban, mute or kick someone in every linked group at once, so that one command protects them all.
**Mode:** mvp
**Depends on:** Phase 1
**Research flag:** During phase research, confirm what `restrictChatMember` does to a target who is banned from a group or not in it, so that mute and unmute fan-out can never lift a ban by accident.
**Requirements:** STAFF-01, STAFF-02, STAFF-03, STAFF-04, STAFF-05, STAFF-06, STAFF-07, STAFF-08, STAFF-12, STAFF-13, PLAT-01
**Success Criteria** (what must be TRUE):
  1. In the Staff Group, any member can run `/ban`, `/mute`, `/kick`, `/unban` or `/unmute` against an `@username` or numeric ID. Ban and mute can be timed, and any action can carry a reason. The bot first shows a confirmation: the resolved name and ID, the action, the duration, the reason and the number of linked groups. Nothing happens until the issuer taps Confirm, and nobody else can confirm or cancel. An `@username` the bot has never seen is refused with a hint to use the numeric ID.
  2. After Confirm, the action is applied in every linked group where the issuer is an admin with the right to restrict members at that moment. It is never applied in the Staff Group itself. A group is skipped, with the reason, if the issuer lacks that right there, the target is an admin or the owner there, or the target is the bot. A link whose owner no longer matches is removed and reported, not acted on.
  3. A post from an anonymous admin in the Staff Group gets the reply "post as yourself", and nothing else happens.
  4. The issuer gets one summary message that updates as groups finish. Every linked group is marked done, skipped (with the reason) or failed (with the reason, such as missing bot rights or rate limiting). The bot stays within Telegram's rate limits even with many groups and several bot replicas, and waits and retries when Telegram says to. No group is ever silently dropped.
  5. `/ban`, `/mute`, `/kick`, `/unban` and `/unmute` work exactly as before in every group that isn't a Staff Group.

**Plans:** 10/10 plans complete

Plans:
**Wave 1**
- [x] 02-01-PLAN.md: Tracer. `/ban <ID> [reason]` from the Staff Group, Redis confirm card (issuer-only Confirm and Cancel), live per-group checks, ban in every linked group, card edited into the summary; per-group `/ban` unchanged elsewhere; anonymous senders refused

**Wave 2** *(blocked on Wave 1 completion)*
- [x] 02-02-PLAN.md: `/mute`, `/kick`, `/unban` and `/unmute`, with a status-first decision table that never lifts or shortens a ban (research flag)

**Wave 3** *(blocked on Wave 2 completion)*
- [x] 02-03-PLAN.md: Timed bans and mutes (`/ban <ID> 2d`, `/tban`, `/tmute`); duration on the card, over 366 days treated as permanent, end date fixed at Confirm

**Wave 4** *(blocked on Wave 3 completion)*
- [x] 02-04-PLAN.md: Targets by `@username` (case-insensitive, never guessed) and text mention; hints for unknown or ambiguous names, bare replies and the refused `/sban`-style variants

**Wave 5** *(blocked on Wave 4 completion)*
- [x] 02-05-PLAN.md: Card lifecycle: 5-minute expiry, exactly-once Confirm across replicas, abort when linked groups changed, one staff action per target; `/staff` help text

**Wave 6** *(blocked on Wave 5 completion)*
- [x] 02-06-PLAN.md: Fleet-wide Redis pacer with shared `retry_after`, four paced workers, D-19 failure reasons

**Wave 7** *(blocked on Wave 6 completion)*
- [x] 02-07-PLAN.md: Live summary: tally, batched edits, collapse and continuation, final-edit retry and fallback, shutdown drain

**Wave 8** *(gap closure, blocked on Wave 7 completion)*
- [x] 02-08-PLAN.md: Gap 1 (WR-01). One delivery budget per summary message so a long `retry_after` cannot lose the final summary; progress edits back off after a 429
- [x] 02-09-PLAN.md: Gap 2, pacer half (WR-02). A paced call refuses a slot beyond `MaxWait` at once, without a Telegram request or a slot; `retry_after` overflow clamp

**Wave 9** *(gap closure, blocked on Wave 8 completion)*
- [x] 02-10-PLAN.md: Gap 2, lock half (WR-02). The run's coordinator renews its target lock by compare-and-set; AGENTS.md rules and the full phase gate

### Phase 3: Staff Audit and Undo

**Goal:** As a staff member, I want to log, list and undo every staff action, so that mistakes can be reversed and we stay accountable.
**Mode:** mvp
**Depends on:** Phase 2
**Requirements:** STAFF-09, STAFF-10, STAFF-11, SETUP-09
**Success Criteria** (what must be TRUE):
  1. Every group where a staff action was applied gets a log-channel post with the action, target, issuer and reason.
  2. Every staff action is recorded with the issuer, target, action, duration, reason, time and per-group outcomes. The record survives bot restarts.
  3. `/staff` lists recent staff actions with who, what, target, when, reason and the outcome in each group.
  4. "Undo everywhere" on a summary reverses the action in each group where the person pressing it is a Staff Group member and an admin with restrict rights. The other groups are skipped with the reason, and the result appears in the same done, skipped or failed summary. Someone outside the Staff Group can't undo anything.

**Plans:** 12/12 plans complete

Plans:
**Wave 1**
- [x] 03-01-PLAN.md: Tracer. Owner confirms the one-way prior-state columns (checkpoint), then every confirmed staff action is recorded with each group's prior state (written before the Telegram write) and outcome; survives restarts and chat migration

**Wave 2** *(blocked on Wave 1 completion)*
- [x] 03-02-PLAN.md: Undo decision table, test-first: decideStaffUndo restores the prior state, skips groups changed since, never re-applies an end date inside 120 s; invariants over every combination
- [x] 03-03-PLAN.md: Log-channel posts for applied actions (admin category, paced, never naming the Staff Group, never changing the result); the run engine takes a spec

**Wave 3** *(blocked on Wave 2 completion)*
- [x] 03-04-PLAN.md: "Recent actions" on /staff: paged list, newest first, 10 per page, offset cursor, members only

**Wave 4** *(blocked on Wave 3 completion)*
- [x] 03-05-PLAN.md: History detail view: per-group lines in the summary format, undo outcomes, unlinked and unfinished groups, wording for every undo reason

**Wave 5** *(blocked on Wave 4 completion)*
- [x] 03-06-PLAN.md: Tracer for "Undo everywhere": button on the final summary, confirm card replying to it, presser-only Confirm, DB claim, live per-group checks for the presser, undo summary, original marked "Undone by"

**Wave 6** *(blocked on Wave 5 completion)*
- [x] 03-07-PLAN.md: Undo lifecycle and safety: cancel, expiry, refusals, one undo per action across replicas, shared target lock, shutdown mid-undo, no time limit

**Wave 7** *(blocked on Wave 6 completion)*
- [x] 03-08-PLAN.md: Undo restores exactly the prior state end to end (independent permissions), skips changed, unapplied, unlinked and rightless groups; #STAFF_UNDO log posts

**Wave 8** *(blocked on Wave 7 completion)*
- [x] 03-09-PLAN.md: Undo from the history detail view, /staff help and docs, final AGENTS.md pass, full phase gate with PostgreSQL migration chain

**Wave 9** *(gap closure G-03-4, blocked on Wave 8 completion)*
- [x] 03-10-PLAN.md: Gap G-03-4, claim half (WR-01). An undo that reached no Telegram write gives its claim back (ReleaseUndo); the original summary is marked at the end of the run, "Undone by" or "changed nothing"; help text and docs

**Wave 10** *(gap closure G-03-4, blocked on Wave 9 completion)*
- [x] 03-11-PLAN.md: Gap G-03-4, shutdown half (WR-01). The undo Confirm joins the shutdown drain before it claims and refuses to claim once the shutdown began

**Wave 11** *(gap closure G-03-4, blocked on Wave 10 completion)*
- [x] 03-12-PLAN.md: Gap G-03-4, label half (WR-02). Recent actions, the detail view and the Undo answers say undone, running, interrupted or changed nothing from the stored undo outcomes; full phase gate

### Phase 4: Manual Lockdown

**Goal:** As a group admin, I want to lock my group down during a raid and lift it when safe, so that attackers are stopped and settings survive.
**Mode:** mvp
**Depends on:** Phase 1
**Spike gate:** Before planning, run two spikes in a throwaway supergroup. (1) Can a per-user restriction let approved users talk while the group's default permissions are locked? If not, the owner picks the LOCK-03 fallback before this phase is planned. (2) How do joins arrive: as `chat_member` updates, as join service messages (which may be missing in large groups), or as join requests? Does the join guard's `EndGroups` really stop greetings and captcha for a removed joiner? **Resolved 2026-10-06:** (1) can't: the per-user toggle is greyed out once the default is locked, so approved users are muted too (04-CONTEXT D-01, D-02). (2) is designed away: the join guard handles all three join paths, and live delivery is a UAT item (04-CONTEXT D-03).
**Requirements:** LOCK-01, LOCK-02, LOCK-03, LOCK-04, LOCK-05, LOCK-06, LOCK-07, LOCK-08, LOCK-09
**Success Criteria** (what must be TRUE):
  1. When an admin runs `/lockdown`, everyone except the group's admins is muted, and anyone who joins is removed at once. Removed users can rejoin after the lockdown is lifted. Other linked groups are unaffected.
  2. Approved users can still talk during a lockdown. If the spike shows Telegram can't allow this, the fallback the owner chose applies instead and is stated in the lockdown notice.
  3. An admin can see whether the group is locked, since when and why, and `/staff` shows the group as in lockdown. Running `/lockdown` again during a lockdown reports the existing one instead of starting another.
  4. A lockdown never lifts on its own. It survives bot restarts and a Redis flush, and every bot replica enforces it. Only an admin of the group can lift it with `/unlockdown`, and anyone else is refused.
  5. Lifting the lockdown restores the group's permissions exactly as they were before. Anyone unmuted during the lockdown, by `/unmute` or by passing the captcha, can still talk after it lifts.

**Plans:** 8/8 plans complete

Plans:
**Wave 1**
- [x] 04-01-PLAN.md: Tracer. `/lockdown` stores the exact getChat permissions, then locks; `/unlockdown` restores them byte for byte; live authority (owner or admin with restrict), refusals that record nothing, one active lockdown per chat on SQLite and PostgreSQL

**Wave 2** *(blocked on Wave 1 completion)*
- [x] 04-02-PLAN.md: `/lockdownstatus` (since, who, why, removed count, hand-edit warning); the lift replaces hand edits, stays locked when the restore fails, lifts once; anonymous admins always prove who they are

**Wave 3** *(blocked on Wave 2 completion)*
- [x] 04-03-PLAN.md: Joiners recorded by the group -7 guard and banned until the lift by a paced, DB-driven worker (no welcome or captcha); the lift unbans only the lockdown's own bans and posts one tally

**Wave 4** *(blocked on Wave 3 completion)*
- [x] 04-04-PLAN.md: Every join path handled once (service messages, cross-path dedupe, re-joins, mixed messages, D-24 approvals); only admin-added users get in, bots never; no goodbye for removed joiners

**Wave 5** *(blocked on Wave 4 completion)*
- [x] 04-05-PLAN.md: Join requests declined during a lockdown and the Accept button refuses; a lockdown stays inside its own group; `/antiraid` steps aside while one is active

**Wave 6** *(blocked on Wave 5 completion)*
- [x] 04-06-PLAN.md: Restart mid-lift resumes, stale claims released, unconfirmed locks settled, Redis flush or outage changes nothing, a lockdown never lifts on its own

**Wave 7** *(blocked on Wave 6 completion)*
- [x] 04-07-PLAN.md: Unmuting during a lockdown (`/unmute`, button, captcha pass, staff `/unmute`) gives the pre-lockdown permissions, so the user talks after the lift

**Wave 8** *(blocked on Wave 7 completion)*
- [x] 04-08-PLAN.md: `/staff` marks locked groups (SETUP-08); a staff ban on a lockdown joiner survives the lift; lockdown locale parity test, final AGENTS.md pass and full phase gate

### Phase 5: Lockdown Alerts and Response

**Goal:** As a group admin or staff member, I want to get a lockdown alert with one-tap fixes, so that we can see a raid and clean it up fast.
**Mode:** mvp
**Depends on:** Phase 2, Phase 4
**Requirements:** LOCK-11, LOCK-12, LOCK-13, LOCK-14
**Success Criteria** (what must be TRUE):
  1. Each lockdown posts one alert to the group's Staff Group (if it has one) and to its log channel. The alert shows the trigger, the counts behind it, and a sample of the accounts that joined during the burst, including those removed during the lockdown.
  2. "Lift lockdown" works only for an admin of the locked group, whether it's pressed in the Staff Group or in the log channel. Anyone else is refused and nothing changes. If the lockdown has already been lifted, pressing a button on its alert says so and does nothing.
  3. An admin of the locked group can press "Ban N recent joiners" to ban the accounts that joined during the burst, except admins and approved users. They are told how many were banned, skipped and failed.
  4. When Telegram reports which invite link the burst used, the alert shows it. Only an admin of that group can revoke it with "Revoke link".
  5. While the group stays locked, the alert is re-posted periodically as a reminder. The reminders stop once the lockdown is lifted.

**Plans:** 14/14 plans complete

Plans:
**Wave 1**
- [x] 05-01-PLAN.md: Tracer. A lockdown of a linked group posts one alert in its Staff Group (exactly-once gate, worker-posted copy, stored text)

**Wave 2** *(blocked on Wave 1 completion)*
- [x] 05-02-PLAN.md: The alert's Lift button: live authority, asker-only confirm card, the shared lift function with /unlockdown, the public lift notice

**Wave 3** *(blocked on Wave 2 completion)*
- [x] 05-03-PLAN.md: Every copy closes at the lift ("Lifted by X at T", no buttons); stale buttons do nothing; repository tests for the gate and claims

**Wave 4** *(blocked on Wave 3 completion)*
- [x] 05-04-PLAN.md: Lift button safety: service identities, forged chats, no restrict right, demotion; card expiry; 64-byte callbacks; works without Redis
- [x] 05-05-PLAN.md: The 10-minute Redis join tracker (D-17) written by both group -7 join paths, best effort

**Wave 5** *(blocked on Wave 4 completion)*
- [x] 05-06-PLAN.md: The alert in the log channel (admin category) with a working Lift; delivery retries, rate limits, stale claims, concurrent replicas, the settle path

**Wave 6** *(blocked on Wave 5 completion)*
- [x] 05-07-PLAN.md: The burst on the alert: the tracker snapshot saved with the lockdown, up to 10 accounts with name, @username and ID, bots marked, the trigger's counts

**Wave 7** *(blocked on Wave 6 completion)*
- [x] 05-08-PLAN.md: The ban policy core: per-account decision and ban with no end date, admins, approved users, people an admin let in (D-22) and the bot skipped, safe beside the worker and the lift; ban run storage
- [x] 05-09-PLAN.md: The invite link the burst used on the alert, with a pointer when the bot cannot revoke it

**Wave 8** *(blocked on Wave 7 completion)*
- [x] 05-10-PLAN.md: "Ban N recent joiners": button, asker-only card, DB-driven run, result; approved users read fresh

**Wave 9** *(blocked on Wave 8 completion)*
- [x] 05-11-PLAN.md: Ban again during a raid (new accounts only, resume, stop at the lift); full result with skip reasons and declined requests; one #LOCKDOWN_BAN log post

**Wave 10** *(blocked on Wave 9 completion)*
- [x] 05-12-PLAN.md: "Revoke link" for bot-created links by admins with the invite right; every copy says who revoked it; plain-word refusals

**Wave 11** *(blocked on Wave 10 completion)*
- [x] 05-13-PLAN.md: Hourly reminders with fresh counts and live buttons; the previous reminder loses its buttons; none after the lift, no backlog after downtime

**Wave 12** *(blocked on Wave 11 completion)*
- [x] 05-14-PLAN.md: Help text and docs in all 7 languages, PostgreSQL integrity list, final AGENTS.md pass, full phase gate

### Phase 6: Automatic Raid Detection

**Goal:** As a group admin, I want to have raids detected and locked down automatically, so that my group is protected when no admin is online.
**Mode:** mvp
**Depends on:** Phase 5
**Requirements:** RAID-01, RAID-02, RAID-06, RAID-07, LOCK-10, LOCK-15
**Success Criteria** (what must be TRUE):
  1. A burst of joins in a short window locks the group automatically. The alert names the join surge and its count.
  2. A burst of messages, photos, GIFs or stickers, mostly from recently joined members, locks the group automatically. A busy conversation among established members does not. Admins and approved users never count toward either trigger.
  3. Both triggers are on by default in every group, with conservative thresholds. An admin can turn each one off or change its threshold, and the next burst follows the new setting.
  4. When several triggers fire at once, the group gets exactly one lockdown and one alert. This holds even when different bot replicas handle the joins and messages.
  5. `/antiraid` and its old timing commands now point admins to `/lockdown`. A group's old auto-threshold becomes its join-surge threshold, and the old raid mode no longer temp-bans anyone.

**Plans:** 13/13 plans complete

Plans:
**Wave 1**
- [x] 06-01-PLAN.md: One start path shared by /lockdown and the triggers (`startLockdownRow`, replies unchanged), the trigger columns on `chat_lockdowns`, and the alert line that names the trigger

**Wave 2** *(blocked on Wave 1 completion)*
- [x] 06-02-PLAN.md: Tracer. 15 joins in 60 s lock a group automatically and the alert names the join surge with its count; settings table with the threshold carry-over (clamped to 3..500, D-22)

**Wave 3** *(blocked on Wave 2 completion)*
- [x] 06-03-PLAN.md: Every join path counted once; admin-added, approved, the bot and join requests never count; window, script and no-Redis pinned; the guard suites still pass
- [x] 06-04-PLAN.md: The flood trigger at handler group -4: 30 items in 30 s, 60% from members who joined in the last day, at least 3 accounts; exemptions and item kinds

**Wave 4** *(blocked on Wave 3 completion)*
- [x] 06-05-PLAN.md: `/autolockdown` shows and changes each trigger (live owner or restricting admin), and the next burst follows it on every replica
- [x] 06-06-PLAN.md: One lockdown and one alert however many triggers and replicas fire; 10 quiet minutes after a lift; the fire latch by outcome

**Wave 5** *(blocked on Wave 4 completion)*
- [x] 06-07-PLAN.md: The group is told it was locked automatically; a raid the bot cannot lock is reported once a day to the Staff Group and the log channel

**Wave 6** *(blocked on Wave 5 completion)*
- [x] 06-08-PLAN.md: /lockdownstatus, /lockdown and reminders name the automatic trigger

**Wave 7** *(blocked on Wave 6 completion)*
- [x] 06-09-PLAN.md: A flood alert lists the flooding accounts and "Ban N recent joiners" can ban them (D-17)

**Wave 8** *(blocked on Wave 7 completion)*
- [x] 06-10-PLAN.md: The old raid mode is retired: pointer commands, the `/autoantiraid` alias, no temp-bans, startup key sweep
- [x] 06-11-PLAN.md: Developer stats on the new table and the PostgreSQL carry-over proof
- [x] 06-12-PLAN.md: The docs pages describe the automatic triggers and the retired raid mode

**Wave 9** *(blocked on Wave 8 completion)*
- [x] 06-13-PLAN.md: Help text in all 7 languages, PostgreSQL integrity list, final AGENTS.md pass, full phase gate

### Phase 7: AI-Assisted Raid Detection

**Goal:** As a group admin, I want to have AI flag borderline spam text and images, so that subtler raids still trigger a lockdown.
**Mode:** mvp
**Depends on:** Phase 6
**Spike gate:** Before planning, test `gemini-3.5-flash-lite` on sample spam photos, GIFs and stickers: accuracy, latency, cost, and how animated media is sampled. Then run `/gsd-ai-integration-phase` for the AI-SPEC. The research STACK recommended Claude Haiku through `anthropic-sdk-go`, but the owner chose Gemini, so this phase's research must target the Gemini Go client instead.
**Requirements:** RAID-03, RAID-04, RAID-05, PLAT-02
**Success Criteria** (what must be TRUE):
  1. Borderline text is checked with the existing TypeSafe AI check. Borderline photos, GIFs and stickers are checked with Gemini (`gemini-3.5-flash-lite`, using the owner's key). Ordinary messages are not sent to any AI.
  2. A burst of AI spam verdicts in a short window locks the group, and the alert names the AI trigger and its count. Like the other triggers, it's on by default, and an admin can turn it off or change its threshold.
  3. AI never decides a lockdown alone. A single verdict doesn't lock a group. If an AI provider is slow, down or not configured, the rule-based triggers keep working and nothing waits on AI.
  4. The Gemini API key never appears in logs, including errors and debug output.

**Plans:** 13 plans

Plans:
**Wave 1**
- [ ] 07-01-PLAN.md: Tracer. Five /aispam spam verdicts from three accounts lock the group as `ai_burst` through `raidFire`; every trigger-kind surface names the AI trigger; the alert lists the accounts behind the verdicts
- [ ] 07-02-PLAN.md: The stdlib-only Gemini client (`alita/utils/gemini`): verified request, strict verdict, key-free errors, one retry within 2 s, leaf-package test

**Wave 2** *(blocked on Wave 1 completion)*
- [ ] 07-03-PLAN.md: `GEMINI_API_KEY` and `GEMINI_MODEL` loaded, validated and registered with `logredact`; Google-key shape scrubbed; env docs with the paid-tier note
- [ ] 07-04-PLAN.md: `ai_enabled` / `ai_threshold` on `chat_raid_detection` (append-only migration, CHECK 3..500), stale-cache defaults, and the counter follows each group's setting
- [ ] 07-05-PLAN.md: The owner's Gemini spike kit (`scripts/gemini-spike/` with `-probe` and `-models-get`) and `07-SPIKE.md`

**Wave 3** *(blocked on Wave 2 completion)*
- [ ] 07-06-PLAN.md: The group 2 candidate watcher and the bounded raid AI pool: new members' suspicious text checked once by TypeSafe, shared verdict with /aispam, drain after `RegisterRaidPostsDrain`

**Wave 4** *(blocked on Wave 3 completion)*
- [ ] 07-07-PLAN.md: `/autolockdown ai N|on|off` with the same live checks, the AI status line and the "not set up" notice

**Wave 5** *(blocked on Wave 4 completion)*
- [ ] 07-08-PLAN.md: New members' photos, GIFs, stickers and videos judged by Gemini; the right frame per kind; token-safe download; secrets never logged

**Wave 6** *(blocked on Wave 5 completion)*
- [ ] 07-09-PLAN.md: Hourly Gemini budgets, verdict cache, album gate, fire-latch skip, per-chat breaker and provider-wide pause

**Wave 7** *(blocked on Wave 6 completion)*
- [ ] 07-10-PLAN.md: Developer `/stats`: AI-off groups and this replica's AI check counters
- [ ] 07-11-PLAN.md: Architecture and reference docs for the AI trigger, its keys and what is sent where

**Wave 8** *(blocked on Wave 7 completion)*
- [ ] 07-12-PLAN.md: The /lockdown help in all 7 languages, the generated page, the final AGENTS.md pass

**Wave 9** *(blocked on Wave 8 completion)*
- [ ] 07-13-PLAN.md: Final integration: an image burst locks end to end, the AI path never acts, join and flood still fire with every provider down; full phase gate

### Phase 8: Turnstile Web Captcha

**Goal:** As a group admin, I want to verify new members with Turnstile in a Telegram Mini App, so that bots stay out with little friction.
**Mode:** mvp
**Depends on:** Phase 4
**Spike gate:** Before planning, test on real Android, iOS and Desktop clients. (1) A Direct Link Mini App button posted in a group opens the page with signed `initData` for the user who tapped it. A user who isn't the target is rejected, and the `startapp` parameter is long enough for the challenge token. (2) The Turnstile widget renders and returns a token inside Telegram's WebViews, and siteverify reports the Mini App's hostname. For the spike, the owner provides a public HTTPS hostname, a BotFather Mini App and Turnstile keys.
**Requirements:** CAPT-01, CAPT-02, CAPT-03, CAPT-04, CAPT-05, CAPT-06, CAPT-07, CAPT-08, PLAT-04
**Success Criteria** (what must be TRUE):
  1. An admin can set a group's captcha mode to Turnstile, and math and text stay available. In a Turnstile group, a new member is muted on joining and gets a challenge message. Its button opens the bot's captcha page as a Mini App in the member's language.
  2. A member who passes is unmuted, their held messages are replayed, the greeting runs and the challenge is cleaned up, as with the existing captcha.
  3. Only the member the challenge was issued to can pass it. Another user's tap, a reused challenge, or a page reporting a pass by itself never unmutes anyone. The bot's server decides a pass using only Telegram's signed user data and Cloudflare's verification.
  4. A member who fails the Turnstile check, or doesn't pass within the group's time limit, is kicked and can rejoin to try again. This works no matter which bot replica handles the join or the check.
  5. The owner can deploy the captcha page by following the documented steps: a public HTTPS hostname, BotFather Mini App registration, and Turnstile keys. The Turnstile secret never appears in logs.

**Plans:** TBD
**UI hint**: yes

### Phase 9: Settings Menu

**Goal:** As a group admin, I want to manage my group from one button menu, so that staff, lockdown, captcha and antiflood need no commands.
**Mode:** mvp
**Depends on:** Phase 3, Phase 7, Phase 8
**Requirements:** MENU-01, MENU-02, MENU-03, MENU-04, MENU-05, MENU-06, MENU-07, MENU-08
**Success Criteria** (what must be TRUE):
  1. `/settings` in a group asks the admin whether to open the menu in the group or in private. A menu opened in the group responds only to the admin who opened it. A menu opened in private manages the group it was opened from.
  2. The menu edits one message in place, with Back and Close on every page. Every press re-checks live that the presser is still an admin of that group, so a press by a demoted admin is refused.
  3. The Staff Group page shows the `/staff` panel information. Only the owner of both groups can link or unlink.
  4. The Lockdown page shows the current state, a working Lift button, and an on/off switch and threshold for each automatic trigger. The Captcha page sets the mode and time limit, and the Antiflood page configures antiflood.
  5. A setting changed in the menu is identical to the same setting changed by command, and both always show the same current value.

**Plans:** TBD
**UI hint**: yes

## Progress

**Execution Order:**
Phases run in numeric order, 1 → 9, which is the owner's priority. Phase 4 needs only Phase 1, and Phase 8 needs only Phase 4, but each still waits its turn unless the owner says otherwise.

| Phase | Plans Complete | Status | Completed |
|-------|----------------|--------|-----------|
| 1. Staff Group Links | 10/10 | Complete    | 2026-10-05 |
| 2. Staff Actions Across Groups | 10/10 | Complete    | 2026-10-05 |
| 3. Staff Audit and Undo | 12/12 | Complete    | 2026-10-06 |
| 4. Manual Lockdown | 8/8 | Complete    | 2026-10-08 |
| 5. Lockdown Alerts and Response | 14/14 | Complete    | 2026-10-18 |
| 6. Automatic Raid Detection | 13/13 | Complete    | 2026-10-29 |
| 7. AI-Assisted Raid Detection | 0/13 | Planned | - |
| 8. Turnstile Web Captcha | 0/TBD | Not started | - |
| 9. Settings Menu | 0/TBD | Not started | - |
