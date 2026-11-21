# Roadmap: Xenobot-72

## Milestones

- ✅ **v1.0 Staff Group, Raid Protection, Web Captcha & Settings Menu** - Phases 1-9 (shipped 2026-11-02, archived in `milestones/v1.0-ROADMAP.md`)
- ✅ **v1.1 PM Settings Hub & Member Screening** - Phases 10-19 (shipped 2026-11-13, archived in `milestones/v1.1-ROADMAP.md`; the owner's live Telegram checks are deferred, see `STATE.md` Deferred Items)
- 🚧 **v1.2 Custom Messages and Tuning** - Phases 21-30 (in progress)

## Phases

<details>
<summary>✅ v1.0 Staff Group, Raid Protection, Web Captcha & Settings Menu (Phases 1-9) - SHIPPED 2026-11-02</summary>

Full phase details, success criteria and plan lists: [milestones/v1.0-ROADMAP.md](milestones/v1.0-ROADMAP.md). Requirements: [milestones/v1.0-REQUIREMENTS.md](milestones/v1.0-REQUIREMENTS.md).

- [x] **Phase 1: Staff Group Links** - 10/10 plans (completed 2026-10-05)
- [x] **Phase 2: Staff Actions Across Groups** - 10/10 plans (completed 2026-10-05)
- [x] **Phase 3: Staff Audit and Undo** - 12/12 plans (completed 2026-10-06)
- [x] **Phase 4: Manual Lockdown** - 8/8 plans (completed 2026-10-08)
- [x] **Phase 5: Lockdown Alerts and Response** - 14/14 plans (completed 2026-10-18)
- [x] **Phase 6: Automatic Raid Detection** - 13/13 plans (completed 2026-10-29)
- [x] **Phase 7: AI-Assisted Raid Detection** - 14/14 plans (completed 2026-10-29)
- [x] **Phase 8: Turnstile Web Captcha** - 11/11 plans (completed 2026-10-30)
- [x] **Phase 9: Settings Menu** - 14/14 plans (completed 2026-10-30)

</details>

<details>
<summary>✅ v1.1 PM Settings Hub & Member Screening (Phases 10-19) - SHIPPED 2026-11-13</summary>

Full phase details, success criteria and plan lists: [milestones/v1.1-ROADMAP.md](milestones/v1.1-ROADMAP.md). Requirements: [milestones/v1.1-REQUIREMENTS.md](milestones/v1.1-REQUIREMENTS.md). Audit: [milestones/v1.1-MILESTONE-AUDIT.md](milestones/v1.1-MILESTONE-AUDIT.md) (`tech_debt`, no blocker). Phase folders, with each phase's `VERIFICATION`, `SECURITY` and `SPIKE` files: `milestones/v1.1-phases/`.

Every phase is code-verified and security-audited with no open threat; each verification reads `human_needed` because the owner's live Telegram checks were deferred on purpose (the dates below are the verification dates).

- [x] **Phase 10: PM Settings Hub and Owner Gate** - 11/11 plans (verified 2026-11-04)
- [x] **Phase 11: Message Editor** - 14/14 plans (verified 2026-11-04)
- [x] **Phase 12: Rules Media, Media Under Text and Greet Once** - 16/16 plans (verified 2026-11-05)
- [x] **Phase 13: Member Requirements** - 14/14 plans (verified 2026-11-08)
- [x] **Phase 14: Profile Photo and Channel Rules** - 12/12 plans (verified 2026-11-08)
- [x] **Phase 15: AI Spam-Name Check** - 11/11 plans (verified 2026-11-10)
- [x] **Phase 16: Scheduled Deletion of Service and Bot Messages** - 9/9 plans (verified 2026-11-12)
- [x] **Phase 17: Command, Edit and Self-Destruct Deletion** - 12/12 plans (verified 2026-11-12)
- [x] **Phase 18: Remaining Settings Pages** - 15/15 plans (verified 2026-11-13)
- [x] **Phase 19: Blacklist, Filter and Note Pages** - 12/12 plans (verified 2026-11-13)

</details>

### 🚧 v1.2 Custom Messages and Tuning (In Progress)

**Milestone Goal:** The owner decides, group by group in that group's settings in the bot's private chat, what the bot says when it punishes someone or locks the group, whether the person also gets it privately, and how the punishments themselves behave. Backups of a group's settings are reachable from the same private chat.

**Phase Numbering:**
- Integer phases (20, 21, 22): Planned milestone work, continuing from v1.1's Phase 19
- Decimal phases (20.1, 20.2): Urgent insertions (marked with INSERTED)

Decimal phases appear between their surrounding integers in numeric order.

- [ ] **Phase 21: Message Engine, Messages Page and Manual Punishments** - Storage, one renderer with default fallback, goldens of today's replies, a private Messages page on the editor, and custom messages for ban, kick, mute, unban and unmute
- [ ] **Phase 22: PM Copies** - An optional private copy to the punished person on its own pacing, with a Details button fallback and a log line
- [ ] **Phase 23: Warn, Flood, Blacklist and Forward Blocklist Messages** - Custom messages for warn, warn limit and the three automatic actions
- [ ] **Phase 24: Captcha, AI Spam and Member Requirement Notices** - Custom messages for a failed captcha and requirement notices, and a new rate-limited AI spam notice
- [ ] **Phase 25: Lockdown Messages** - Custom messages for a manual lockdown, each automatic trigger and the lift, with every alert and raid-scale path unchanged
- [ ] **Phase 26: Timed Punishments, Escalation and Warn Expiry** - Working timed bans and mutes for warn limit, blacklist and flood, an escalation ladder, and expiring warns
- [ ] **Phase 27: Manual Command Defaults, Reason Shortcuts and Appeal Button** - Default durations, a reason-required switch, saved reason words and an appeal link on ban messages
- [ ] **Phase 28: Advanced Thresholds** - Per-group detection and screening limits that are code constants today, inside safe bounds
- [ ] **Phase 29: Plain Tone and Copy Settings** - Neutral default texts per group and a confirmed copy of one group's messages and tuning to another
- [ ] **Phase 30: Backup Page and Custom Messages in Backups** - Export, import and reset from the private chat, and custom messages inside `/export` and `/import`

**Coverage:** 40/40 v1.2 requirements mapped, each to exactly one phase (see the Traceability table in `REQUIREMENTS.md`).

## Overview

v1.2 lets the owner change what the bot says and how its punishments behave, group by group, from the bot's private chat. The order follows the owner's priority, which is also the dependency order. The foundation comes first (Phase 21). It holds storage, one renderer that falls back to today's text, goldens of today's replies recorded before any send site changes, the Messages page on the v1.1 editor, and the manual ban, kick and mute events as the first slice the owner can try. PM copies follow (Phase 22), because they need the engine and the manual events and they set the pacing and failure rules every later event reuses. Then the automatic events come in two groups. Phase 23 takes the punishments that already share helpers (warn, warn limit, flood, blacklist, forward blocklist). Phase 24 takes the three that each have their own rules (captcha, member requirement notices, and the new AI spam notice). Lockdown messages come next (Phase 25), with a hard guard that alerts, raid-scale removals and the Staff Group stay untouched. Tuning is split in three coherent phases: how automatic punishments work (Phase 26), how manual commands work (Phase 27) and the global limits (Phase 28). Plain tone and copy settings (Phase 29) need every event and every tuning value to exist, so they come after. The Backup page and custom messages in `/export` and `/import` close the milestone (Phase 30). Backups sit last for two reasons. Export must produce the same file as `/export`, so the messages table and its event keys have to be stable first. And the page is a front for the existing `/export`, `/import` and `/reset` code, so it can wait until the rest is done.

I put the Backup page and MSG-10 in one phase because both are about one file format: the page's export has to carry the custom messages, and its import has to report on them. Every phase that adds screens ships a slice the owner can try in a real group.

## Cross-Cutting Constraints

These apply to every phase. Planners carry them into each phase's must-haves.

- **AGENTS.md is the rulebook.** Migrations are append-only and timestamped (the latest file today is `20261118120000_add_chat_timezones.sql`; read `migrations/` when the phase starts). Every write calls `cache.DeleteCache`. A key that is read on every punishment or message, and must act on every replica at once, is listed in `skipLocal`. Callbacks go through `callbackcodec` (64-byte cap). Fire-and-forget goroutines recover from panics. Tests use real fixtures, no mock libraries. `make generate-docs`, `make check-translations`, `make test` and `make lint` pass at the end of every phase. A new table, Redis key family, handler group, callback namespace or rule change updates `AGENTS.md` in the same commit.
- **Everything is per group, in the bot's private chat.** No Staff Group defaults and no inheritance. The only thing that moves values between groups is the explicit copy in Phase 29. No page, message, placeholder, PM copy or log line names the Staff Group or another group (v1.0 D-21, D-32).
- **One renderer for every send site.** Events are table rows, registered from their own file and validated in Go (like `deletion.ValidKind` and the settings pages), so a later phase adds rows and strings and does not edit the engine, `settings_pages.go` or the migration. A custom message is one text for all 8 languages. No row means the default translation in the group's language.
- **The punishment comes first.** A custom message, a PM copy, a Details button, a log line or a notice never delays, undoes or skips a ban, mute, kick, warn, lockdown or lift. A custom message Telegram refuses is replaced by the default text (MSG-04). Unknown never punishes, as in v1.1.
- **MSG-11 is a rule for every phase.** The first plan of each phase records characterization tests of every send site that phase will change, and commits them alone before any production change. They are never re-recorded or weakened (the v1.1 golden rule). The existing `*_legacy_request_test.go` goldens pin settings commands, not the punishment replies and notices of this milestone. `bans.go` and `mute.go` have none today.
- **Deletion timers stay as they are (MSG-09).** A custom message goes through the same `scheduleBotMessage` kind as the default it replaces (`bot:punish_manual`, `bot:punish_auto` and the rest). A surface that is never scheduled today (Staff Group, lockdown alerts, captcha challenges, log posts) stays unscheduled.
- **Telegram limits and shutdown.** Anything that sends in bulk (PM copies, AI notices) is paced fleet-wide in Redis on its own keys, like the deletion, member requirement and lockdown pacers. A new goroutine recovers from panics. A new shutdown drain must recompute the sum in `AGENTS.md` (the worst case is already 68 s against the 60 s budget), so prefer a bounded pool with no new drain, as the self-destruct batcher did.
- **Settings rules from v1.1 hold.** Every page is `PrivateOnly`. Viewing needs the live admin check and the owner gate. Every change needs its right (`can_change_info` for messages), judged live on every press and every typed message. Each setting has one apply function that its command also calls, with parity, drift and authority-parity tests. Every page has a worst-case callback test (64 bytes) and a length test in 8 languages. `fuku/modules` is near its CI time limit, so keep each new test light.
- **Texts.** New keys go in all 8 locale files. uk and en follow the style guide in `docs/src/content/docs/contributing/locale-style.md` (`TestLocaleStyle` fails otherwise). Every reply is HTML, and every value a user typed is escaped. Keys read through table fields are held by a table test, because `make check-translations` cannot see them.

### Settled owner decisions

These were decided before this roadmap and are not reopened by any phase.

- Everything is configured per group in that group's settings in the bot's private chat. No Staff Group defaults, no inheritance (2026-11-18).
- A custom message is one text that replaces all 8 translations for that group.
- PM copies are best effort, paced on their own budget and never sent for raid-scale removals.
- Only a confirmed lift by an admin ends a lockdown. Nothing here adds an automatic lift.
- The Backup page reuses the commands' own code paths and never posts the backup file in a group.

## Phase Details

### Phase 21: Message Engine, Messages Page and Manual Punishments
**Goal**: As the owner, I can set what the bot says in a group when an admin bans, kicks, mutes, unbans or unmutes someone, from that group's settings in the bot's private chat. A group that sets nothing sees no change.
**Depends on**: Phase 19 (the settings framework, the message editor, the typed-reply door and the list engine)
**Requirements**: MSG-01, MSG-02, MSG-03, MSG-04, MSG-05, MSG-06, MSG-07, MSG-08, MSG-09, MSG-11, EVT-01
**Planning notes**:
- Plan 1 is the goldens. The success replies of `/ban`, `/tban`, `/kick`, `/mute`, `/tmute`, `/unban` and `/unmute` all go through `moderationCtx.replySuccess` (five sites in `bans.go`, three in `mute.go`). Record them, and their `bot:punish_manual` deletion kind, and commit them alone.
- Storage is one table keyed by group, event and destination (group now, PM in Phase 22), with the text, media, buttons and Silent switch. The event key is checked in Go, not by a CHECK, so later phases need no migration. Decide whether the read key goes in `skipLocal`: a change must act at the next command on every replica.
- The main design question is how an event, a destination and a part fit the editor's wait hash (`p` is one page key, `t` one part) and the 64-byte callbacks. The Deletion page's section and row tables are the pattern for a page with many rows, and `settingsTypedPages` is the door's hook for a page with its own parts.
- `{admin}` must read "an admin" for an anonymous admin even after the proof tap. The re-entered command carries the tapper as sender, so the engine needs the anonymous mark from the original update.
- The page is table-driven by family (manual, automatic, lockdown, requirements). Only the manual family has rows in this phase.
**Success Criteria** (what must be TRUE):
  1. In the private `/settings` menu of a group, the owner opens Messages and sees the manual family with each event's state (default, custom or silent). In a group menu the page shows only the "open in private" note.
  2. The owner opens an event (ban, timed ban, kick, mute, timed mute, unban or unmute) and sets text, one media and URL buttons by sending messages in private. The editor's limits and refusals apply. Each part has a Preview, and a Full preview goes through the real send path.
  3. After a custom text is set, the next `/ban`, `/tban`, `/kick`, `/mute`, `/tmute`, `/unban` or `/unmute` in that group posts it, with `{first}`, `{mention}`, `{reason}`, `{admin}`, `{duration}` and `{until}` filled in. A placeholder with no value for that event shows nothing or a neutral word, never raw. Names and reasons typed by users are escaped. `{admin}` reads "an admin" for an anonymous admin. The silent and delete-first variants behave as before.
  4. A group with nothing stored sends exactly today's reply for every command above, in the group's language. If Telegram refuses a custom message (too long, bad file, bad markup), the default text goes out instead and the punishment has already happened. A custom message is deleted on the same Bot messages timer as the default.
  5. Each event has a Silent switch. Silent means the bot acts and posts nothing in the group. Viewing the page needs the live admin check and the owner gate. Every change needs `can_change_info`, judged live on every press and every typed message.
**Plans**: 11 plans
Plans:
- [ ] 21-01-PLAN.md — goldens of every manual punishment reply, committed alone (wave 1)
- [ ] 21-02-PLAN.md — chat_event_messages storage, eventmsg repository, skipLocal, backup exclusion (wave 1)
- [ ] 21-03-PLAN.md — message editor hooks for page-defined messages (wave 1)
- [ ] 21-04-PLAN.md — tracer: event message engine and the ban event end to end (wave 2)
- [ ] 21-05-PLAN.md — tban, kick, unban, mute, tmute, unmute sites and the anonymous mark (wave 3)
- [ ] 21-06-PLAN.md — private Messages page: families, events, states, Main (wave 4)
- [ ] 21-07-PLAN.md — Silent switch (wave 5)
- [ ] 21-08-PLAN.md — editor on event screens: Set, Remove, typed reply (wave 6)
- [ ] 21-09-PLAN.md — previews through the send path's composer (wave 7)
- [ ] 21-10-PLAN.md — page safeguards: bytes, lengths, forged presses, privacy (wave 8)
- [ ] 21-11-PLAN.md — end-to-end proof and phase gates (wave 9)
**UI hint**: yes

### Phase 22: PM Copies
**Goal**: As the owner, I can have the bot also tell a punished person in private, and it never delays or changes the punishment.
**Depends on**: Phase 21
**Requirements**: PMSG-01, PMSG-02, PMSG-03, PMSG-04, PMSG-05, PMSG-06, PMSG-07
**Planning notes**:
- Telegram only lets a bot message a person who started it, so a refusal (403) is the normal "not delivered" answer. It is not an error.
- The PM sender needs its own Redis pacing keys and a per-group per-minute cap counter, both outside the cache prefix. It runs after the punishment and the group message. Use a bounded pool with no new drain, or recompute the shutdown budget.
- The Details button needs a short-lived record of the message that only the punished person can read, for example a token in Redis behind a `/start details_<token>` deep link (`RegisterDeepLinkHandler`). Opening the link starts the bot, so it works for a person who never did. The handler must check that the person who opened it is the punished one. Callback and deep link data stay under 64 bytes.
- Never PM for lockdown joiner bans, "Ban N recent joiners" or a Staff Group action. Those paths must not reach the PM sender. A guard test lists every caller.
- Each event screen gains a PM copy (text, media, buttons and its on/off switch). The two per-group options, the Details button and the log line, sit on the Messages page.
- Goldens first for any send site this phase touches that Phase 21 did not.
**Success Criteria** (what must be TRUE):
  1. Each punishment event on the Messages page has a PM copy with its own text, media, buttons and on/off switch, off by default. With it on, a punished person who started the bot receives it in private after the punishment and the group message.
  2. If the person never started the bot or blocked it, nothing else changes: the punishment stands and the group message appears as usual. A burst of PM copies never delays a ban, a mute or a notice, because PM copies use their own fleet-wide pacing budget.
  3. With the Details option on and the PM not delivered, the group message carries a Details button. The punished person taps it, the bot's private chat opens and shows them their message. Anyone else who taps it gets no content.
  4. With the log option on, a PM copy that could not be delivered leaves one line in the group's log channel.
  5. No PM copy is sent for a lockdown's joiner bans, for "Ban N recent joiners" or for a Staff Group action. A per-group cap bounds PM copies per minute for automatic events. A PM copy never names the Staff Group or another group, and names the acting admin only through `{admin}`.
**Plans**: TBD
**UI hint**: yes

### Phase 23: Warn, Flood, Blacklist and Forward Blocklist Messages
**Goal**: As the owner, I can customize what the bot says when it warns someone, when a member reaches the warn limit, and when flood, a blacklisted word or a blocked forward gets punished, in the group and by PM.
**Depends on**: Phase 22
**Requirements**: EVT-02, EVT-03
**Planning notes**:
- Send sites: `warnThisUser` (one warn send, with a manual or automatic source, and the blacklist's warn action goes through it), the warn-limit action, `checkFlood`, the blacklist watcher's notices and `forwardBlockNotice` (in `forwardblock_notice.go`, not a `staff*` file, because `TestMemberDeletionExcludedMessages` forbids a deletion producer in a staff file).
- Goldens first. The goldens that exist pin the warn settings commands, not these notices.
- The warn message keeps its Remove warn button, added by the bot after any custom buttons. A custom message cannot remove or replace it.
- The forward blocklist reason and notice name neither the source nor the Staff Group. No placeholder may add them.
- This phase creates the Automatic family on the Messages page. Phase 24 adds more events to it.
**Success Criteria** (what must be TRUE):
  1. The Messages page has an Automatic family with warn, warn limit, flood action, blacklist action and forward blocklist action. Each opens the editor with a group message, a PM copy and a Silent switch, as in Phases 21 and 22.
  2. A custom warn message is posted on `/warn` and on the blacklist's warn action, keeps its Remove warn button and fills `{count}` and `{limit}`. When a member reaches the limit, the custom warn-limit message is posted with `{action}` naming what was done.
  3. A flood, blacklist or forward blocklist punishment posts its custom message with `{action}` and `{reason}`. The forward blocklist message names neither the blocked source nor the Staff Group.
  4. A group with nothing stored gets today's text for all five, byte for byte. A refused custom message falls back to the default. The message is deleted on the same timer as the default, and the punishment is never delayed.
  5. PM copies for these events follow Phase 22's rules, including the per-group cap. A flood of offenders produces at most the capped number of PM copies a minute, and every punishment still happens.
**Plans**: TBD
**UI hint**: yes

### Phase 24: Captcha, AI Spam and Member Requirement Notices
**Goal**: As the owner, I can customize the message for a failed captcha and the notices for member requirement breaches, and I can turn on a rate-limited notice for AI spam deletions.
**Depends on**: Phase 23 (the Automatic family on the Messages page) and Phase 22 (PM copies)
**Requirements**: EVT-04, EVT-05, EVT-09
**Planning notes**:
- Captcha failure goes through `handleCaptchaTimeout`, shared by the timer, the cleanup tick, the late check and the web `Fail` and `Expire`. One message must serve the timeout and the failed web check. The challenge message itself is not an event.
- Member requirement notices go through `memberReqPostNotice`. The bot appends the list of broken rules, the remedy sentences and the `mreq|v1` "I fixed it" button after the custom text. The 520-code-point cap of the notices is a rendered check over the real translations, so a custom text plus the appended parts must still fit Telegram's limits. This phase creates the Requirements family.
- The AI notice is new behavior in `/aispam`'s delete branch. It is off by default per group. It needs Redis counters (one per person over a few minutes, one per group per minute) on their own keys outside the cache prefix, and it sheds quietly when over the cap. `aispamNotifyBreaker` stays separate. The deletion itself does not change.
- Goldens first for the captcha failure notice and the four requirement notices.
**Success Criteria** (what must be TRUE):
  1. A member who fails the captcha by timeout or by a failed web check gets the custom message in the group and, if on, by PM. The captcha challenge message is unchanged.
  2. A member requirement notice (warn, kick, ban or mute) takes a custom text. The list of broken rules still follows it, and a mute still carries the "I fixed it" button, which works as before.
  3. The AI spam notice is off by default, and a deletion then posts nothing, as today. With it on, a deletion posts a notice, at most one per person per few minutes and a capped number per group per minute. A wave of spam posts at most the cap, and every spam message is still deleted.
  4. A group with nothing stored gets today's captcha and requirement notices byte for byte. A refused custom message falls back to the default, and the deletion timers apply as before.
**Plans**: TBD
**UI hint**: yes

### Phase 25: Lockdown Messages
**Goal**: As the owner, I can customize what the bot posts in a group when a lockdown starts (by hand or by one of the three triggers) and when it is lifted. The technical notes, the alerts and every raid-scale path stay as they are.
**Depends on**: Phase 24 (it extends the Messages page; the engine comes from Phase 21, and lockdown messages send no PM copies)
**Requirements**: EVT-06, EVT-07, EVT-08, EVT-10
**Planning notes**:
- Send sites: the `/lockdown` reply, `raidPostNotice` for join surge, flood and AI burst (numbers come from `lockdownTriggerSentence`), and `lockdownAnnounceLift`. The automatic notice and the cannot-lock warning go through `raidPostAsync` and the `StopRaidPosts` drain. The custom message must use that same path, so no new drain is added.
- The technical notes (`lockdown_note_approved_muted`, the no-delete and no-invite notes) are appended by the bot after the custom text.
- MSG-02 lists `{count}` and `{window}` only. EVT-07 also says "the trigger's other numbers" (the flood percent). Decide in discussion whether `{percent}` joins the placeholder set.
- A lift from a settings menu opened in the group only wakes the worker today, because the redrawn menu already says it in that chat. EVT-08 says "whoever lifted it". Confirm with the owner whether that one path stays as it is.
- EVT-10 is proved here by a guard test over every caller of the engine. It fails if a Staff Group card, summary or log post, a lockdown alert or reminder, the "Ban N recent joiners" run or report, a `#LOCKDOWN_BAN` post, a log channel post or a captcha challenge ever goes through the renderer or the PM sender. It holds from Phase 21, and this phase completes the event list it checks.
- Goldens first for the manual start reply, each automatic notice and the lift notice.
**Success Criteria** (what must be TRUE):
  1. A manual `/lockdown` posts the custom message, and the technical notes that apply follow it.
  2. An automatic lockdown posts the message of its trigger (join surge, flood or AI burst) with `{count}`, `{window}` and the trigger's other numbers, and the same technical notes follow.
  3. Lifting a lockdown posts the custom lifted message, whether the lift came from `/unlockdown`, the alert's Lift button or the settings menu.
  4. Staff Group actions, lockdown alerts and reminders, raid-scale removals (joiner bans and "Ban N recent joiners") with their reports, log channel posts and captcha challenges are exactly as before. They are not on the Messages page, never go through the renderer and never get a PM copy.
  5. A group with nothing stored gets today's lockdown texts byte for byte. A refused custom message falls back to the default, and the lock, the snapshot, the worker and the alert are never delayed by it.
**Plans**: TBD
**UI hint**: yes

### Phase 26: Timed Punishments, Escalation and Warn Expiry
**Goal**: As the owner, I can make automatic punishments timed, escalate repeat offenders step by step, and let old warns expire.
**Depends on**: Phase 23 (the automatic events and their `{duration}` and `{until}` text)
**Requirements**: TUNE-01, TUNE-02, TUNE-03
**Planning notes**:
- `tban` and `tmute` pass the database checks of the warn mode and the blacklist action but nothing applies them at the limit. Antiflood accepts them in its check but never writes them (D-20). This phase makes them work for warn limit, blacklist and flood, with a duration. The commands (`/setwarnmode`, `/blacklistaction`, `/setfloodmode`) and the pages go through the same apply function. Their goldens (`TestLegacyWarnSettingsRequests`, `TestLegacyBlacklistRequests`) are not re-recorded, and new arguments get new tests.
- Reuse the duration grammar (`extraction.ParseDurationToken`) and Telegram's 30-second rule for `until_date`: compute the end date once, and never send one that is too close.
- Ladder questions for the discussion: where offence counts live (PostgreSQL or Redis with a TTL, shared across replicas), which automatic sources count (flood, blacklist, forward blocklist and warn limit are the candidates; member requirement and captcha punishments have their own rules), and when a count resets.
- Warn expiry: `warns_users` holds one row per person per group with a count and an array of reasons, and no time per warn. Expiry needs a per-warn time, so it needs a migration and a safe reading of old rows. `/warns`, `/rmwarn`, `/resetwarns`, the backup export of warns and the limit check must agree on what counts.
**Success Criteria** (what must be TRUE):
  1. The owner picks a timed ban or a timed mute, with a duration, as the action for warn limit, blacklist or flood, from the page and by command. When it fires, the member is restricted until the end date, the message shows `{duration}` and `{until}`, and the restriction ends at that date.
  2. With the ladder on, a member's first, second and later automatic offences in that group get the steps the owner set, in order, whichever replica sees them. With the ladder off (the default), punishments are exactly what each setting says.
  3. With warn expiry set to N days, a warn older than N days no longer counts toward the limit, and `/warns` shows what counts. With it off (the default), nothing changes.
  4. A group that sets none of this behaves exactly as today, and every argument the three commands accept today answers as before.
**Plans**: TBD
**UI hint**: yes

### Phase 27: Manual Command Defaults, Reason Shortcuts and Appeal Button
**Goal**: As the owner, I can set how my admins' manual punishments behave by default, save short words for common reasons, and give banned people a way to appeal.
**Depends on**: Phase 22 (ban messages and their PM copies carry the appeal button)
**Requirements**: TUNE-04, TUNE-05, TUNE-06
**Planning notes**:
- This is for the per-group commands only. Staff Group commands keep their own durations and reasons (EVT-10, and no Staff Group defaults).
- Questions for the discussion: which commands the reason-required switch covers (ban, mute, kick, warn), and whether the silent and delete-first variants get the default duration.
- Reason shortcuts are a list (word, then full text) with a two-step add, like Filters on the v1.1 list engine. Expansion happens where the reason is parsed. The expanded reason shows in the group message, the PM copy, the warn reason and the log channel. Stored text is escaped when it is shown.
- The appeal link uses the editor's link check (http or https). The button is added after any custom buttons and counts toward Telegram's button limits.
- Goldens first for the manual command paths this phase changes (the duration parse and the missing-reason refusal).
**Success Criteria** (what must be TRUE):
  1. With a default duration set, `/mute` and `/ban` given without a duration apply it. An explicit duration is unchanged. With no default, they behave as today.
  2. With "reason required" on, a punishment with no reason is refused with a short reply and nothing happens. With a reason, it runs. It is off by default.
  3. The owner adds a reason shortcut (a word and its full text) on a page, browses the list and deletes an entry behind a two-tap card. `/ban @user spam` then shows the full text as the reason in the group message, the PM copy, the warn reason and the log post. A word that is not a shortcut is used as typed.
  4. With an appeal link set, a ban message (default or custom) and its PM copy carry the button. With none set, they carry none. The link passes the editor's link check.
**Plans**: TBD
**UI hint**: yes

### Phase 28: Advanced Thresholds
**Goal**: As the owner, I can tune the detection and screening limits that are fixed in code today, per group, inside safe bounds.
**Depends on**: Phase 27
**Requirements**: TUNE-07
**Planning notes**:
- The values and their constants today (from `AGENTS.md`): join surge window 60 s, flood window 30 s with the 60% new-member share, the 3-account floor and the 24 h newness age, AI burst window 120 s, `raidAIImageBar` 0.85, `memberReqAIBar` 0.9, the member requirement cap of 10 a minute and its 10-minute pause, `lockdownReminderEvery` of 1 hour, and a captcha timeout above today's 10-minute cap.
- Hot paths: the flood watcher reads its settings on every message and the Redis Lua scripts fix their windows, caps and expiries. A window change must keep the script arguments, caps and TTLs consistent, and it must add no database round trip per message. Each value needs Go-side bounds and a default equal to today's constant, with a test that fails if the two drift.
- A change must act on every replica at the next event, so decide `skipLocal` for the new key.
- Goldens first for the behaviors whose constants move.
**Success Criteria** (what must be TRUE):
  1. A private Advanced page lists the areas (raid windows, new-member share and age, AI spam bars, member requirement cap and pause, lockdown reminder interval, captcha timeout). Each value shows its current setting, its default and its allowed range.
  2. A value can only be set inside its bounds. An out-of-range, stale or forged press changes nothing. Reset to default puts today's constant back, for one value or one area.
  3. A changed value acts at the next relevant event on every replica. For example, a new flood window changes what counts as a flood, a new reminder interval changes when the next lockdown reminder posts, and a captcha timeout above 10 minutes applies to the next joiner.
  4. A group that changes nothing behaves exactly as today.
**Plans**: TBD
**UI hint**: yes

### Phase 29: Plain Tone and Copy Settings
**Goal**: As the owner, I can make the bot's default texts neutral, and I can copy one group's messages and tuning to another group I own.
**Depends on**: Phase 28 (every event and every tuning value exists before the tone covers or the copy moves them)
**Requirements**: TUNE-08, TUNE-09
**Planning notes**:
- Plain tone: each event of this milestone needs a neutral default text, group and PM, in all 8 languages. It applies only where no custom message is set. A test renders every event in both tones and all 8 languages and holds the length limits and placeholder equality. The MSG-11 goldens are for tone off.
- Copy settings: the source and the target must both be groups where the person may change settings, checked live at the ask and again at Yes (`getChatMember` and the owner gate). The two-tap card is bound to the pair and has an expiry. It names exactly what will be overwritten (custom messages, Silent and PM switches, each tuning area). Whether an empty source entry clears the target's custom message is a design question for the discussion.
- A refusal must not reveal whether the bot is in the other group, as the private pick does.
- Copy writes message rows through one function that applies the editor's link and length checks. Phase 30's import reuses it.
- Copy never moves Staff Group links, approvals or anything outside messages and tuning.
**Success Criteria** (what must be TRUE):
  1. The Messages page has a plain-tone switch, off by default. With it on, the bot uses neutral default texts for this milestone's events in the group's language (all 8 languages). Custom messages are untouched.
  2. From a group's settings, the owner picks another group and sees a card that names exactly what will be overwritten. After Yes, the target group has the same custom messages and tuning values.
  3. The person must be allowed to change settings in both groups, checked live. Otherwise the copy is refused, and the refusal does not reveal anything about the other group.
  4. Copying happens once. Nothing links the groups afterwards, the source is unchanged, and a stale or replayed card changes nothing.
**Plans**: TBD
**UI hint**: yes

### Phase 30: Backup Page and Custom Messages in Backups
**Goal**: As the owner, I can export, import and reset a group's settings from the bot's private chat, and the backup carries the group's custom messages.
**Depends on**: Phase 29 (the messages table, its event keys and the message write function are stable)
**Requirements**: MSG-10, BKP-01, BKP-02, BKP-03
**Planning notes**:
- The commands already exist: `/export` (live admin and bot admin, the file goes to the admin's private chat, `AcquireExport`), `/import` (live owner, reply to the file, a confirm card, `AcquireImport`) and `/reset` (live owner, a confirm card, `AcquireReset`). The page calls one apply function per action, and the commands call the same ones and keep their replies. Parity tests prove the page and the command leave the same result. The rights fit the existing composites (`settingsRightAdminBotAdmin` for export, `settingsRightOwnerBotAdmin` for import and reset).
- The command's pending import and reset tokens live in one replica's memory (`pendingImports`, `pendingResets`). The page's cards must work on any replica. Decide where the parsed file waits between the file and the Yes (Redis with a size cap, or a stored `file_id` that is downloaded and parsed again at Yes). A document in private needs a new part of the typed-reply door.
- Custom messages become a backup module: `AllExportableModules`, export, import and reset. Import applies the editor's link and length checks, counts what it leaves out in `ImportReport.Skipped`, skips and counts event keys this build does not know, and imports an old export without the module as before (the `TestOldGreetingsExportDecodesToZeros` pattern). If messages are a module, `/reset` of all modules clears them. That changes `/reset` for everyone, so confirm it with the owner.
- Tuning values are not in the requirements. Whether they join the backup is the owner's call, and the default is that they stay out, like the staff tables.
- The file is never posted in a group. The page is `PrivateOnly`.
**Success Criteria** (what must be TRUE):
  1. The Backup page opens only in the bot's private chat (a group menu shows the "open in private" note), for a person who passes the live admin check and the owner gate.
  2. The owner or an allowed admin picks all modules or some and presses Export. The bot sends the backup file to them in the private chat, never in the group. It is the same file `/export` makes, custom messages included, under the same rate limit and the same admin right.
  3. The person presses Import and sends the file in the private chat. The bot shows what will be imported, and a two-tap card confirms. It runs the same import as `/import`, with the same checks, the same report of what was left out and the same rate limit, and only for the group's live owner. Custom messages are restored with the editor's checks, and an old export without them imports as before.
  4. The owner resets all or chosen modules to their defaults behind a two-tap card, as `/reset` does, with its rate limit. Anyone who is not the live owner is refused and nothing changes.
  5. A stale, expired or forged card or press changes nothing, and the page and the commands leave the same stored result.
**Plans**: TBD
**UI hint**: yes

## Progress

**Execution Order:**
Phases execute in numeric order: 20 → 21 → 22 → 23 → 24 → 25 → 26 → 27 → 28 → 29

| Phase | Milestone | Plans Complete | Status | Completed |
|-------|-----------|----------------|--------|-----------|
| 21. Message Engine, Messages Page and Manual Punishments | v1.2 | 0/TBD | Not started | - |
| 22. PM Copies | v1.2 | 0/TBD | Not started | - |
| 23. Warn, Flood, Blacklist and Forward Blocklist Messages | v1.2 | 0/TBD | Not started | - |
| 24. Captcha, AI Spam and Member Requirement Notices | v1.2 | 0/TBD | Not started | - |
| 25. Lockdown Messages | v1.2 | 0/TBD | Not started | - |
| 26. Timed Punishments, Escalation and Warn Expiry | v1.2 | 0/TBD | Not started | - |
| 27. Manual Command Defaults, Reason Shortcuts and Appeal Button | v1.2 | 0/TBD | Not started | - |
| 28. Advanced Thresholds | v1.2 | 0/TBD | Not started | - |
| 29. Plain Tone and Copy Settings | v1.2 | 0/TBD | Not started | - |
| 30. Backup Page and Custom Messages in Backups | v1.2 | 0/TBD | Not started | - |
