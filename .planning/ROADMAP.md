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
- [ ] **Phase 2: Staff Actions Across Groups** - Staff ban, mute, kick, unban or unmute someone in every linked group at once, after a Confirm tap, with live per-group admin checks
- [ ] **Phase 3: Staff Audit and Undo** - Every staff action is posted to log channels, listed in `/staff`, and reversible with "Undo everywhere"
- [ ] **Phase 4: Manual Lockdown** - `/lockdown` removes joiners and mutes non-admins in one group; `/unlockdown` restores its permissions exactly
- [ ] **Phase 5: Lockdown Alerts and Response** - Each lockdown alerts the Staff Group and log channel with Lift, "Ban N recent joiners" and "Revoke link" buttons
- [ ] **Phase 6: Automatic Raid Detection** - Join surges and new-member floods lock a group automatically; the old `/antiraid` is retired
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

**Plans:** TBD

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

**Plans:** TBD

### Phase 4: Manual Lockdown

**Goal:** As a group admin, I want to lock my group down during a raid and lift it when safe, so that attackers are stopped and settings survive.
**Mode:** mvp
**Depends on:** Phase 1
**Spike gate:** Before planning, run two spikes in a throwaway supergroup. (1) Can a per-user restriction let approved users talk while the group's default permissions are locked? If not, the owner picks the LOCK-03 fallback before this phase is planned. (2) How do joins arrive: as `chat_member` updates, as join service messages (which may be missing in large groups), or as join requests? Does the join guard's `EndGroups` really stop greetings and captcha for a removed joiner?
**Requirements:** LOCK-01, LOCK-02, LOCK-03, LOCK-04, LOCK-05, LOCK-06, LOCK-07, LOCK-08, LOCK-09
**Success Criteria** (what must be TRUE):
  1. When an admin runs `/lockdown`, everyone except the group's admins is muted, and anyone who joins is removed at once. Removed users can rejoin after the lockdown is lifted. Other linked groups are unaffected.
  2. Approved users can still talk during a lockdown. If the spike shows Telegram can't allow this, the fallback the owner chose applies instead and is stated in the lockdown notice.
  3. An admin can see whether the group is locked, since when and why, and `/staff` shows the group as in lockdown. Running `/lockdown` again during a lockdown reports the existing one instead of starting another.
  4. A lockdown never lifts on its own. It survives bot restarts and a Redis flush, and every bot replica enforces it. Only an admin of the group can lift it with `/unlockdown`, and anyone else is refused.
  5. Lifting the lockdown restores the group's permissions exactly as they were before. Anyone unmuted during the lockdown, by `/unmute` or by passing the captcha, can still talk after it lifts.

**Plans:** TBD

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

**Plans:** TBD

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

**Plans:** TBD

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

**Plans:** TBD

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
| 2. Staff Actions Across Groups | 0/TBD | Not started | - |
| 3. Staff Audit and Undo | 0/TBD | Not started | - |
| 4. Manual Lockdown | 0/TBD | Not started | - |
| 5. Lockdown Alerts and Response | 0/TBD | Not started | - |
| 6. Automatic Raid Detection | 0/TBD | Not started | - |
| 7. AI-Assisted Raid Detection | 0/TBD | Not started | - |
| 8. Turnstile Web Captcha | 0/TBD | Not started | - |
| 9. Settings Menu | 0/TBD | Not started | - |
