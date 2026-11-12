# Roadmap: Fuku Robot

## Milestones

- ✅ **v1.0 Staff Group, Raid Protection, Web Captcha & Settings Menu** - Phases 1-9 (shipped 2026-11-02, archived in `milestones/v1.0-ROADMAP.md`)
- 🚧 **v1.1 PM Settings Hub & Member Screening** - Phases 10-19 (in progress)

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

## v1.1 PM Settings Hub & Member Screening

**Milestone Goal:** Configure everything for a group from the bot's private chat with buttons, like GroupHelp, and screen members automatically on join and on every message.

## Overview

v1.1 moves every group setting into the bot's private chat behind buttons, lets each group's owner decide who may configure it, and adds member screening and message deletion. The phases follow the owner's priority, which is also the dependency order. The owner gate and the private-chat hub come first, because every later page rides on them (Phase 10). Next comes the private input state and the message editor. Welcome, Goodbye and Rules are written by sending messages in private (Phase 11), then rules gain media and buttons, media can sit under its text, and members can be greeted only once (Phase 12). Member requirements follow, cheapest first. Last name, username and name-script rules come with their punishments, one notice per violation and the "I fixed it" button (Phase 13). Profile-photo and channel-subscription lookups come next (Phase 14), and the AI spam-name check is last (Phase 15). Message deletion starts with a durable queue for service messages and the bot's own messages (Phase 16) and ends with members' commands, edits and self-destruct (Phase 17). The remaining pages close the milestone: simple settings and locks (Phase 18), then blacklists, filters and notes, which need the input state (Phase 19). Every phase ships a slice the owner can try in a real group.

`.planning/research/SUMMARY.md` lists 17 design points where the four research files disagree, plus live Telegram spikes and owner questions. Each phase's **Research flags** line names the ones its phase researcher settles. This roadmap decides none of them. It records only the decisions the owner has already made.

## Cross-Cutting Constraints

These apply to every phase. Planners carry them into each phase's must-haves.

- **AGENTS.md is the rulebook.** Migrations are append-only and timestamped (read the latest file in `migrations/` when the phase starts). Every write calls `cache.DeleteCache`, and a key that must be fresh on every replica is listed in `skipLocal`. Commands go through `helpers.WrapCommand` and callbacks through `callbackcodec` (64-byte cap). Fire-and-forget goroutines recover from panics. Tests use real fixtures, no mock libraries. `make generate-docs`, `make check-translations`, `make test` and `make lint` pass at the end of every phase. A new handler group, callback namespace, Redis key family, table (with its backup status), shutdown drain or rule change updates `AGENTS.md` in the same commit.
- **The per-group admin check is never skipped.** Every way into the menu (private list, in-group `/settings`, deep link, button press, typed input) re-checks the person live against the group it affects with `getChatMember`, never with the admin cache or Redis state. The owner gate from Phase 10 sits on top of that check and never replaces it.
- **State is shared across replicas (v1.0 PLAT-01).** Input waits, violation latches, greet-once records, the owner's admin-access switch and the deletion schedule behave the same whichever replica handles an update.
- **Secrets are registered with `logredact.RegisterSecret` (v1.0 PLAT-02).** The Anthropic key the name check uses already is. Any new secret is registered too, and no key or Telegram file URL reaches a log or an error.
- **PLAT-05 (mapped to Phase 10):** every later phase adds its new messages and button labels to all 8 locale files (en, es, fr, hi, id, pt, ru, uk).
- **PLAT-06 (mapped to Phase 14):** the AI name checks (Phase 15) and the deletion work (Phases 16 and 17) also share Telegram rate budgets across replicas, recover from crashes and drain within the existing shutdown budget.
- **New handler groups stay below 12.** Group 12 (the late raid flood counter) must stay after every other group, so any new handler group gets a lower number and is added to the handler list in `AGENTS.md`.
- **One setting, one code path (v1.0 MENU-08, and EDIT-11 and PAGE-10 in v1.1).** Every menu control calls its command's apply function, which holds the validation, the refusals, the write and its cache invalidation. Each new setting gets the parity, drift and authority-parity tests Phase 9 set up.
- **Unknown never punishes.** All four research files agree on this. A screening or lookup answer is pass, fail or unknown, and unknown does nothing. Every punishment and every unmute goes through an existing executor (the warn core, the ban, kick and mute paths, `resolveUnmutePermissions`), never a second copy.
- **Nothing names the Staff Group (v1.0 D-21, D-32).** No menu, refusal, notice or page shows a Staff Group's title or ID to anyone but the creator of the group being configured.

### Settled owner decisions

These were decided before research and are not reopened by any phase.

- Settings are owner-only by default. An owner-only switch lets the group's admins in. A regular admin without it does not see the group in `/settings` at all; there is no read-only view. In-group setting commands are unchanged (Phase 10).
- 8 locales (en, es, fr, hi, id, pt, ru, uk), and uk is the default language (Phase 10, PLAT-05).
- On join the captcha runs before the requirement check (Phase 13).
- A member who fails a requirement on join and is muted gets no welcome until "I fixed it" succeeds (Phase 13).
- Only a mute gets the "I fixed it" button. A kicked member rejoins and is checked again, a banned member waits for an admin, and a warn only warns (Phase 13).
- The AI spam-name check uses Claude Haiku through the owner's Anthropic key, on names only (Phase 15).
- Command deletion includes admins' commands (Phase 17).
- Edited-message deletion has a 1-minute default grace and exempts admins and approved users (Phase 17).
- Self-destruct exempts admins, approved users and pinned messages (Phase 17).

## Phases

**Phase Numbering:**
- Integer phases (10, 11, 12): Planned milestone work, continuing from v1.0's Phase 9
- Decimal phases (10.1, 10.2): Urgent insertions (marked with INSERTED)

Decimal phases appear between their surrounding integers in numeric order.

- [ ] **Phase 10: PM Settings Hub and Owner Gate** - `/settings` in private lists the user's groups, and only the owner configures a group unless they let admins in
- [ ] **Phase 11: Message Editor** - Welcome, Goodbye and Rules are written from the private chat by sending messages, with preview
- [ ] **Phase 12: Rules Media, Media Under Text and Greet Once** - Rules gain media and buttons, media can sit under its text, and members can be greeted only once
- [ ] **Phase 13: Member Requirements** - Last-name, username and name-script rules with per-rule punishments, one notice per violation and "I fixed it" for muted members
- [ ] **Phase 14: Profile Photo and Channel Rules** - Paced, fail-safe profile-photo and channel-subscription lookups
- [ ] **Phase 15: AI Spam-Name Check** - Claude Haiku judges members' names (names only), cached and budgeted
- [ ] **Phase 16: Scheduled Deletion of Service and Bot Messages** - A durable deletion queue removes service messages and the bot's own messages on a timer
- [ ] **Phase 17: Command, Edit and Self-Destruct Deletion** - Members' commands, late edits and, optionally, every message are deleted automatically
- [ ] **Phase 18: Remaining Settings Pages** - Warnings, switches, language, log channel, anonymous admins and locks on menu pages
- [ ] **Phase 19: Blacklist, Filter and Note Pages** - Browse, delete and add list entries from the private chat

**Coverage:** 53/53 v1.1 requirements mapped, each to exactly one phase (see the Traceability table in `REQUIREMENTS.md`).

## Phase Details

### Phase 10: PM Settings Hub and Owner Gate

**Goal:** As a group owner, I want to open my groups' settings from the bot's private chat and decide whether my admins may change them, so that I control who configures the bot.
**Mode:** mvp
**Depends on:** Phase 9 (the v1.0 settings menu, its press pipeline and pages)
**Requirements:** MENU-09, MENU-10, MENU-11, MENU-12, MENU-13, MENU-14, MENU-15, PLAT-05
**Owner decisions:** owner-only by default; an owner-only switch lets admins in; an admin without it does not see the group at all; in-group setting commands are unchanged; 8 locales with uk as the default.
**Research flags:** Mostly standard patterns (Stage 1 in SUMMARY.md). Settle decision 9 (the table behind the private group list and how it is fed: `chat_admin_index` or `chat_managers`, whether it may be seeded from `chat_users`, and whether a group whose bot is not an admin shows as unknown or is hidden), decision 10 (the admin-access table and its read policy: fresh read or cached with `skipLocal`, proven by a two-instance test) and the hub half of decision 16 (`settings|v1` or a new namespace for the list). Owner question from Gaps to Address: whether a group with no live creator lets admins in or denies everyone. Repo-side: `locales/uk.yml` exists but is shorter than `en.yml`, so run `make check-translations` first to see which keys uk lacks, and update the 7-locale wording in `AGENTS.md`. Pitfalls 6 to 9 (a stale owner switch on another replica, an anonymous admin taken for the owner, refusal texts that reveal a group exists, an empty list for an owner who never spoke).
**Success Criteria** (what must be TRUE):
  1. `/settings` in the bot's private chat lists the groups the user may manage, paged, and picking one opens that group's main menu after a live check. The list comes from the bot's own record of each group's owner and admins. That record is refreshed when the admin list loads or on a promotion or demotion, and cleared when the bot leaves. An empty or incomplete list tells the user to run `/admincache` or `/settings` in the missing group.
  2. By default only the group's live owner gets in, whether through the private list, in-group `/settings`, the `/start settings_<chat id>` deep link or a button press. A regular admin does not see the group in their list. In-group `/settings` and the deep link refuse them with a short reply that never names the Staff Group. `/setrules`, `/setwelcome`, `/captchamode` and the other in-group setting commands keep working for admins exactly as before.
  3. Only the live owner sees and can press "Admins can change settings". While it is on, live administrators get the full menu as in v1.0. A change takes effect on every replica at the next press. An owner posting anonymously is identified by the proof tap, and the tapper's live status decides.
  4. The main menu shows every page of the group in a two-column grid with Close, in the opener's language, and every page has Back.
  5. Every new message and button label exists in all 8 locales (en, es, fr, hi, id, pt, ru, uk), and `AGENTS.md` describes the 8-locale set.
**Plans:** 11/11 plans executed in 8 waves

Plans:
**Wave 1**
- [x] 10-01-PLAN.md — the switch and roster tables and their repositories, concurrent replaces serialized per chat (wave 1)
- [x] 10-02-PLAN.md — the 19 new strings and the new /help text in all 8 locales, the sizing tests and the regenerated docs (wave 1)
- [x] 10-03-PLAN.md — the locale-set guard test and the 8-locale wording of the project documents (wave 1)

**Wave 2** *(blocked on Wave 1 completion)*
- [x] 10-04-PLAN.md — the settings test harness, the PostgreSQL integrity list and the AGENTS.md rules for the new tables and the 8 locales (wave 2)

**Wave 3** *(blocked on Wave 2 completion)*
- [x] 10-05-PLAN.md — the owner gate on in-group /settings (with the anonymous re-entry) and every press (wave 3)

**Wave 4** *(blocked on Wave 3 completion)*
- [x] 10-06-PLAN.md — the owner-only Access page: two taps on, one tap off, every replica at the next press (wave 4)
- [x] 10-07-PLAN.md — the roster feed from admin-list loads, promotions, demotions and the bot leaving (wave 4)

**Wave 5** *(blocked on Wave 4 completion)*
- [x] 10-11-PLAN.md — the owner gate and the 4xx class rule on the deep link, every door deciding the same, and the permissions reference (wave 5)

**Wave 6** *(blocked on Wave 5 completion)*
- [x] 10-08-PLAN.md — the startup backfill, the hourly sweep and the group /settings repair path (wave 6)

**Wave 7** *(blocked on Wave 6 completion)*
- [x] 10-09-PLAN.md — the private group list and the pick (wave 7)

**Wave 8** *(blocked on Wave 7 completion)*
- [x] 10-10-PLAN.md — the two-column Main grid with the status block and the Groups button, the owner's live checks and the phase gate (wave 8)

**UI hint**: yes

### Phase 11: Message Editor

**Goal:** As a group owner, I want to write the welcome, goodbye and rules from the bot's private chat by sending them as messages, so that I can set them up without typing commands in the group.
**Mode:** mvp
**Depends on:** Phase 10 (every editor page and every typed reply goes through the owner gate)
**Requirements:** EDIT-01, EDIT-02, EDIT-03, EDIT-04, EDIT-05, EDIT-06, EDIT-10, EDIT-11
**Research flags:** Short spike first (Stage 2 in SUMMARY.md): whether a message sent in private stores in the same form as `/setwelcome` (entities to stored text), how an album is cut to one item, and how a preview is sent into the private chat through the group's send path. Settle decision 1 (handler group of the private input consumer: -1 or 1, below 12 either way), the input and cancel half of decision 16, and the pending-input key name from the naming list. Owner question from Gaps to Address: stepper buttons as "type a number" through the input state. Pitfalls 10, 11 and 12 (a wait state that is per replica or single-slot, formatting and UTF-16 entity offsets, length limits checked after placeholders are filled in).
**Success Criteria** (what must be TRUE):
  1. From the menu the owner opens Welcome, Goodbye or Rules and sees which parts are set (text, media, URL buttons; Rules gains media and buttons in Phase 12), with a button to set or remove each. After pressing one, the owner's next private message becomes that part. Cancel or `/cancel` leaves the message unchanged, the wait ends by itself after 10 minutes, and it works whichever replica receives the message.
  2. Text keeps its formatting and the placeholders (`{first}`, `{mention}` and the rest). Text too long to send (4096 characters, or 1024 as a caption), a bad button syntax or a bad URL is refused with the reason, and the wait stays open. One media item of a kind the welcome supports (photo, video, GIF, document, sticker, audio, voice) is accepted. From an album only one item is kept, and the owner is told.
  3. Each part has a Preview, and Full preview sends the whole message exactly as members will see it, through the same send path the group uses. A message Telegram would refuse is refused before it is saved.
  4. A message saved in the editor is stored exactly as `/setwelcome`, `/setgoodbye` or `/setrules` would store it, so the next real join, leave or `/rules` in the group shows it, and those commands still work as before.
  5. The Welcome and Goodbye pages switch each message on or off and turn "Delete last welcome/goodbye" on or off, stored exactly as `/welcome`, `/goodbye`, `/cleanwelcome` and `/cleangoodbye` store them.
**Plans:** 14/14 plans executed in 7 waves

Plans:
**Wave 1**
- [x] 11-01-PLAN.md — the first 57 editor strings and the new settings /help in all 8 locales, with their guards (wave 1)
- [x] 11-02-PLAN.md — the commands' length, link, media and placeholder rules as shared helpers (wave 1)
- [x] 11-03-PLAN.md — side-effect-free greetings and rules reads and one-part greeting writes (wave 1)

**Wave 2** *(blocked on Wave 1 completion)*
- [x] 11-04-PLAN.md — the remaining 26 editor strings (refusals, previews, saved, wait ended) in all 8 locales (wave 2)
- [x] 11-05-PLAN.md — one apply function per greeting and rules setting and the shared message builders, used by the commands (wave 2)
- [x] 11-06-PLAN.md — the page-action Validate hook and the change-info right in the settings pipeline (wave 2)
- [x] 11-07-PLAN.md — the Redis wait store and /cancel (wave 2)

**Wave 3** *(blocked on Wave 2 completion)*
- [x] 11-08-PLAN.md — the Welcome, Goodbye and Rules pages with their switches and Delete last (wave 3)

**Wave 4** *(blocked on Wave 3 completion)*
- [x] 11-09-PLAN.md — Set, Cancel and Remove on every part, and the group-menu refusal (wave 4)
- [x] 11-10-PLAN.md — the pure check and dry run of a typed part (wave 4)

**Wave 5** *(blocked on Wave 4 completion)*
- [x] 11-11-PLAN.md — the typed-reply door at handler group 1, text parts end to end, the sixth gate door (wave 5)
- [x] 11-12-PLAN.md — Preview and Full preview through the group's send path (wave 5)

**Wave 6** *(blocked on Wave 5 completion)*
- [x] 11-13-PLAN.md — media, albums and buttons through the door, and whole-message parity with the commands (wave 6)

**Wave 7** *(blocked on Wave 6 completion)*
- [x] 11-14-PLAN.md — privacy and size checks of every editor screen, the rulebook and docs, the owner's live checks and the phase gate (wave 7)

**UI hint**: yes

### Phase 12: Rules Media, Media Under Text and Greet Once

**Goal:** As a group owner, I want rules with media and buttons, media shown under its text, and members greeted only on their first join, so that my messages look and behave the way I set them.
**Mode:** mvp
**Depends on:** Phase 11 (the editor, its input state and the shared send path)
**Requirements:** EDIT-07, EDIT-08, EDIT-09
**Research flags:** Live spike "Caption placement" from Gaps to Address: `show_caption_above_media` for photo, video and animation, and a caption over 1024 characters refused before the send. Settle decision 15 (the greet-once record and its timing: claim before the send, so a failed send suppresses the welcome, or after it, so two replicas may both greet; and the table name). Repo-side backup item: the new greetings and rules columns travel in backup JSON, and the greet-once record stays out of backup, export, import and reset. Pitfall 22 (last-welcome and greet-once races).
**Success Criteria** (what must be TRUE):
  1. The owner adds one media item and URL buttons to Rules from the editor, and `/rules` shows them both in the group and as private rules.
  2. With "Media under text" on, a photo, video or GIF in a welcome, goodbye or rules message appears below its text, both in Full preview and in the group.
  3. With "Greet once" on, a member is welcomed only the first time they ever join the group. Leaving and rejoining brings no second welcome, also when the join reaches several replicas.
  4. Rules, welcomes and goodbyes saved before this phase look exactly as they did until someone changes them.
**Plans:** 15/16 plans executed in 11 waves (12-15 is droppable)

Plans:
**Wave 1**
- [x] 12-01-PLAN.md — characterization: pin the requests old rules, welcomes, goodbyes and document GIFs send, before any refactor (wave 1)
- [x] 12-02-PLAN.md — the migration, the model fields, HasContent, greeted_members and the backup proofs (wave 1)
- [x] 12-05-PLAN.md — the 13 new editor strings and the new Rules hint in all 8 locales, with the five-cell label check (wave 1)

**Wave 2** *(blocked on Wave 1 completion)*
- [x] 12-03-PLAN.md — the greet-once claim, the column-targeted writers, the whole-row rules read and the group language by chat ID (wave 2)
- [x] 12-04-PLAN.md — GIF greetings as animations, sendAnimation and the caption-above flag through one predicate (wave 2)

**Wave 3** *(blocked on Wave 2 completion)*
- [x] 12-06-PLAN.md — forged settings controls and switch values refused before any lookup (wave 3)
- [x] 12-07-PLAN.md — /rules and private rules show media and buttons through one builder, old rows unchanged (wave 3)
- [x] 12-08-PLAN.md — the join, the leave and the greeting displays honour Media under text (wave 3)

**Wave 4** *(blocked on Wave 3 completion)*
- [x] 12-09-PLAN.md — Greet once: one welcome per member ever, across replicas, given back only on a definite refusal (wave 4)

**Wave 5** *(blocked on Wave 4 completion)*
- [x] 12-10-PLAN.md — the switch grid and the Welcome page's Greet once switch (completes EDIT-09) (wave 5)

**Wave 6** *(blocked on Wave 5 completion)*
- [x] 12-11-PLAN.md — Media under text on the Welcome and Goodbye pages (wave 6)

**Wave 7** *(blocked on Wave 6 completion)*
- [x] 12-12-PLAN.md — the Rules page's media and buttons parts, /clearrules over three parts, Rules Media under text (wave 7)

**Wave 8** *(blocked on Wave 7 completion)*
- [x] 12-13-PLAN.md — typed Rules media and buttons and the one-message caption rule (wave 8)

**Wave 9** *(blocked on Wave 8 completion)*
- [x] 12-14-PLAN.md — Full preview with Media under text and the Rules previews in the group's language (completes EDIT-07, EDIT-08) (wave 9)

**Wave 10** *(blocked on Wave 9 completion)*
- [ ] 12-15-PLAN.md — droppable: the atomic swap of the last welcome and goodbye message ID (wave 10). **Dropped** by the orchestrator: its worst case is a cosmetic orphan welcome from a race that predates the phase, no requirement needs it, and 12-16 records the known race in AGENTS.md (accepted risk AR-03 in 12-SECURITY.md)

**Wave 11** *(blocked on Wave 10 completion, or Wave 9 when 12-15 is dropped)*
- [x] 12-16-PLAN.md — privacy and size checks of every Phase 12 screen, the PostgreSQL list, the rulebook and docs, the owner's live checks and the phase gate (wave 11)

**UI hint**: yes

### Phase 13: Member Requirements

**Goal:** As a group owner, I want members with no last name or username, or with names in a script I block, warned, removed or muted automatically, and a muted member able to free themselves once they fix it, so that the group screens people when no admin is online.
**Mode:** mvp
**Depends on:** Phase 10 (the page and its gate), Phase 12 (the welcome, greet once included, that a held member gets once let in)
**Requirements:** MREQ-01, MREQ-02, MREQ-03, MREQ-04, MREQ-05, MREQ-06, MREQ-07, MREQ-08, MREQ-09, MREQ-13
**Owner decisions:** the captcha runs before the requirement check on join; a member muted on join gets no welcome until "I fixed it" succeeds; only a mute gets the "I fixed it" button (kick lets the member rejoin and be checked again, a ban waits for an admin, a warn only warns).
**Research flags:** Settle decision 2 (handler group of the message watcher: -1 holds back a violator's commands, 1 lets them run), decision 11 (the requirement row shape: one wide row per chat or one row per rule), decision 12 (who owns a requirement mute: a marker `until_date` or a ledger cleared by every other mute path, and what a captcha pass does to a held member), decision 13 ("I fixed it" during an active lockdown: unmute through `resolveUnmutePermissions` or refuse until the lift), what is left of decision 14 now that the owner put the captcha first and holds the welcome, the "I fixed it" half of decision 16, and the requirement Redis key family name. From Gaps to Address: the per-group message rate limit for notices during a raid (live check), and the owner question on whether "I fixed it" in a captcha group sends the member through the challenge again (check whether the captcha-first decision already answers it). Build a table-driven Unicode corpus for the name rules (Ukrainian, Belarusian, Bulgarian, Russian, mixed script, zero-width). The requirement holds stay out of backup. Pitfalls 1 to 4 and 23.
**Success Criteria** (what must be TRUE):
  1. A Member requirements page gives each rule its own punishment (off, warn, kick, ban or mute), with "Check on join" and "Delete messages" switches, and everything is off by default. This phase's rules work: must have a last name, must have a username, no Russian name, no Arabic name, no Chinese name. The page says the name rules go by letters (Russian-only ы, ъ, э, ё; Arabic script; Chinese characters) and can also catch related languages, and an invisible or whitespace-only last name does not count as one.
  2. With "Check on join" on, a new member is checked right after passing the captcha, or on joining when the group has no captcha, and every member is checked when they send a message. Admins, approved users, bots, Telegram service accounts and messages sent as a channel or by an anonymous admin are never checked.
  3. A violation is punished once and posts one notice in the group naming the member and the rule broken. Further messages from that member add no warns, notices or punishments until they fix it or rejoin, across replicas. A warn counts toward the group's warn limit, and "Delete messages" removes the violator's messages. A kicked member can rejoin and is checked again, and a banned member stays banned until an admin unbans them.
  4. A member muted by a rule gets an "I fixed it" button only they can use. The bot re-checks them live. If the problem is fixed, they are unmuted (respecting an active lockdown) and get the welcome that was held back. If not, they are told what is still missing. The button never lifts a mute placed by an admin, a staff action, the captcha, antiflood or a lockdown.
**Plans:** 14/14 plans executed
- [x] 13-01-PLAN.md
- [x] 13-02-PLAN.md
- [x] 13-03-PLAN.md
- [x] 13-04-PLAN.md
- [x] 13-05-PLAN.md
- [x] 13-06-PLAN.md
- [x] 13-07-PLAN.md
- [x] 13-08-PLAN.md
- [x] 13-09-PLAN.md
- [x] 13-10-PLAN.md
- [x] 13-11-PLAN.md
- [x] 13-12-PLAN.md
- [x] 13-13-PLAN.md
- [x] 13-14-PLAN.md
**UI hint**: yes

### Phase 14: Profile Photo and Channel Rules

**Goal:** As a group owner, I want to require a profile photo and, in a channel's comment group, a subscription to that channel, so that throwaway accounts and outsiders are screened too, without anyone punished because a lookup failed.
**Mode:** mvp
**Depends on:** Phase 13 (the rules engine, its punisher and its page)
**Requirements:** MREQ-10, MREQ-11, MREQ-12, PLAT-06
**Research flags:** Two live spikes from Gaps to Address. "Profile photo privacy": does `getUserProfilePhotos` return zero or an error for a user who hides their photo? "Channel subscription": what exact error does `getChatMember(channel, user)` give when the bot is not the channel's admin? `chat_status.IsBotMissingError` reads "chat not found" as bot-missing, which is wrong for a channel lookup. Settle the lookup pacer's Redis key family, a pass cache for positive results only, singleflight per lookup, and the shutdown budget for any new drain (Pitfall 30, recomputed against 60 s and `stop_grace_period: 75s`). Pitfalls 5 and 17 to 21.
**Success Criteria** (what must be TRUE):
  1. The profile-photo rule punishes a member who has no photo the bot can see. A photo hidden from the bot counts as missing, and the page says so.
  2. The channel rule is offered only in a group linked to a channel, and it punishes a member who is not subscribed to that channel.
  3. A check that cannot be answered (a Telegram error, a lookup that times out, the bot not being an admin of the linked channel) never punishes anyone. The page shows when a rule cannot work in that group (no linked channel, the bot not admin there). The "AI not configured" line arrives with the AI rule in Phase 15.
  4. During a burst of joins or messages, photo and channel lookups share one Telegram rate budget across replicas and never hold up the rest of the bot. A lookup cut off by a crash or a shutdown punishes nobody and leaves nothing stuck, and lookups in flight end within the existing shutdown budget.
**Plans:** 12/12 plans executed
- [x] 14-01-PLAN.md — migration, model fields and repository rules for the photo and channel columns, the PostgreSQL guarantee (wave 1)
- [x] 14-02-PLAN.md — the 16 new and 7 reworded locale keys in all 8 files and the test constants they move (wave 2)
- [x] 14-03-PLAN.md — the lookup layer (verdict type, lookup pacer, singleflight with panic recovery, positive-only caches) and the photo lookup (wave 3)
- [x] 14-04-PLAN.md — the linked channel, subscription and bot-status lookups, the classifier and the D-22 breach gate (wave 4)
- [x] 14-05-PLAN.md — the two rule-table rows, the mask, the page's seven cells and the tests that counted to five (wave 5)
- [x] 14-06-PLAN.md — the executor and join integration as step 6b, the phase budgets and the unanswerable-shape matrix (wave 6)
- [x] 14-07-PLAN.md — the watcher's cached-positive read, the job hand-off and the held release (wave 7)
- [x] 14-08-PLAN.md — the notice remedy tail and "I fixed it" with approvals, fresh lookups and the count-aware answer (wave 8)
- [x] 14-09-PLAN.md — the page status block, the Off-only channel screen and the bounded status reader (wave 9)
- [x] 14-10-PLAN.md — the live link check that refuses a punishing channel value without a linked channel (wave 10)
- [x] 14-11-PLAN.md — budget independence, logs, alert size, privacy and backup sweep (wave 11)
- [x] 14-12-PLAN.md — the end-to-end slice, rulebook and docs, the owner's live checks and the phase gate (wave 12)
**UI hint**: yes

### Phase 15: AI Spam-Name Check

**Goal:** As a group owner, I want accounts whose names look like spam flagged by AI, so that advertising accounts are screened when they join, with no message text leaving the server.
**Mode:** mvp
**Depends on:** Phase 14 (the requirement page's "cannot work" status lines and the shared lookup budgets)
**Requirements:** MREQ-14 (completes), MREQ-10 and MREQ-01 (completed here: the "AI not configured" line and the eighth rule)
**Owner decisions:** the check uses Claude Haiku through the owner's Anthropic key (the key the raid image checks use), on names only.
**Research flags:** Spike before planning, as Stage 3 in SUMMARY.md asks: Claude Haiku's accuracy, latency and cost on a labelled set of real names, run with the owner's key through a `scripts/claude-spike`-style tool. Settle decision 8 (a separate small pool or a third job kind in the raid AI pool; two of the three files prefer a separate pool, to be confirmed with a load note), decision 17 (an established member's first sight only records a baseline; this is the proposed default to confirm) and the requirement key family name. Owner questions from Gaps to Address: the verdict bar (0.85 or 0.9) and the AI rule's default punishment (warn or mute recommended, never ban by default).
**Success Criteria** (what must be TRUE):
  1. With the AI spam-name rule on, a member whose names the AI judges as spam gets the rule's punishment when they join or change their names. Only the first name, last name and username are sent, never message text.
  2. An established member's names are first recorded without a call, so switching the rule on in a large group causes no burst of calls. Names already judged are answered from the cache, and hourly budgets cap the cost.
  3. With `ENABLE_AISPAM=false` or no Anthropic key the rule is off, and the page says AI is not configured. A slow, failing, rate-limited or over-budget AI gives no verdict and punishes nobody.
**Plans:** 11/11 plans executed
- [x] 15-01-PLAN.md — migration, model field and repository rule for the AI column, the PostgreSQL guarantee (wave 1)
- [x] 15-02-PLAN.md — the names request in the Claude client and the canonical form of names in a leaf package (wave 2)
- [x] 15-03-PLAN.md — the ten new locale keys in all 8 files and the limit tests they move (wave 3)
- [x] 15-04-PLAN.md — the eighth rule-table row, the AI rule on the join path with its verdict cache, and the tests that counted seven rules (wave 4)
- [x] 15-05-PLAN.md — the baseline for established members, the new-member marker, the watcher's cached read and the held release (wave 5)
- [x] 15-06-PLAN.md — hourly budgets, the four-call gate, the name check's own breaker, sticky unknowns and the stale-names guard (wave 6)
- [x] 15-07-PLAN.md — the AI remedy tail of the notice and "I fixed it" with the AI rule (wave 7)
- [x] 15-08-PLAN.md — the page's AI status line, the Off-only form, the press gate and the AI confirm card (wave 8)
- [x] 15-09-PLAN.md — the names mode of the spike tool and the owner's live checklist (wave 9)
- [x] 15-10-PLAN.md — shutdown, join budget, log, request and independence proofs (wave 10)
- [x] 15-11-PLAN.md — the end-to-end slice, rulebook and docs, the phase gate and the requirement ticks (wave 11)
**UI hint**: yes

### Phase 16: Scheduled Deletion of Service and Bot Messages

**Goal:** As a group owner, I want Telegram's service messages and the bot's own messages removed on a schedule I set, so that the chat stays readable.
**Mode:** mvp
**Depends on:** Phase 13 (requirement notices are one of the automatic-punishment messages this phase schedules)
**Requirements:** DEL-01, DEL-02, DEL-06, DEL-07, DEL-08, MREQ-15
**Research flags:** Live spike "Batch delete" from Gaps to Address: what `deleteMessages` does when one ID in the batch is too old or cannot be deleted, so a per-ID fallback can isolate it. Settle decision 3 (where service-message deletion runs; the group-0 `cleanService` also drives join processing, and joined and left must keep sharing `/cleanservice` storage), decision 7 (the maximum delay, 24 h or 47 h; all files agree the age is measured from the message's own send time), the deletion half of decision 11 (row shape) and the naming list (pacer keys, delete kinds, the claim shape). Also work out the service-type policy table and the roughly fifteen manual-punishment reply sites in `bans.go`, `mute.go` and `warns.go`, recompute the shutdown budget before the new drain ships (Pitfall 30), and keep the deletion queue out of backup. Two owner questions are already closed: the Staff Group exemption (answered by DEL-07) and "Own commands" (deferred as DEL2-01). Pitfalls 13 to 17.
**Success Criteria** (what must be TRUE):
  1. A Service messages page sets each Telegram service-message type to keep, delete immediately or delete after N seconds. The types include member joined, member left, new photo, new title, pinned message, forum topics, boosts, video chats, auto-delete timer changes and the rest. Joined and left share storage with `/cleanservice`.
  2. A Bot messages page sets each kind of the bot's own message to keep or delete after N minutes: welcome, goodbye, rules, automatic punishments (requirement notices included) and replies to manual `/ban`, `/mute`, `/kick`, `/warn` and the like.
  3. Scheduled deletions survive a restart and run once across replicas. Delays are capped below Telegram's 48-hour limit, and a message too old to delete is dropped quietly.
  4. Deletion never touches Staff Group summaries and cards, lockdown alerts, captcha challenges, log-channel posts or anything in a Staff Group.
  5. When the bot lacks the right to delete messages, every deletion page says so instead of failing silently.
**Plans:** 7/9 plans executed
- [x] 16-01-PLAN.md — the data layer: migration, models, rules and queue repository, the cache key, backup exclusion and the PostgreSQL guarantees (wave 1)
- [x] 16-02-PLAN.md — the kind tables and the service-type table, the effective rule, and the apply functions including `/cleanservice` (wave 2)
- [x] 16-03-PLAN.md — the tracer: the pacer, the inline delete, the service watcher at group -5, the `cleanService` rewiring and the Staff Group refusal (wave 3)
- [x] 16-04-PLAN.md — the queue worker, the delete-right reader, the scoped latch and `StopDeletionWorker` with its shutdown arithmetic (wave 4)
- [x] 16-05-PLAN.md — `scheduleBotMessage` and the welcome, goodbye, rules and manual-punishment hooks (wave 5)
- [x] 16-06-PLAN.md — the requirement notices (the mute notice at the 24 h cap), the "I fixed it" reschedule and the antiflood, blacklist and aispam notices (wave 6)
- [x] 16-07-PLAN.md — the 57 locale keys in all 8 files and the limits tests they move (wave 7)
- [ ] 16-08-PLAN.md — the Deletion page: hub, lists, row screens, the delete-right line and the `Hidden` field (wave 8)
- [ ] 16-09-PLAN.md — the end-to-end test, the exclusion and privacy sweeps, the rulebook and docs, `16-SPIKE.md`, the phase gate and the requirement ticks (wave 9)
**UI hint**: yes

### Phase 17: Command, Edit and Self-Destruct Deletion

**Goal:** As a group owner, I want members' commands, late edits and, when I choose, every message removed automatically, so that the group stays clean and an edited message cannot slip spam in later.
**Mode:** mvp
**Depends on:** Phase 16 (the deletion queue, its worker and the deletion pages)
**Requirements:** DEL-03, DEL-04, DEL-05
**Owner decisions:** command deletion includes admins' commands; edited-message deletion has a 1-minute default grace and exempts admins and approved users; self-destruct exempts admins, approved users and pinned messages.
**Research flags:** Settle decision 4 (how edited messages are caught; the grace length is settled at 1 minute by default, and whether an edit that changes nothing counts is open), decision 5 (how a command is deleted after the bot has answered it, covering commands from raw `NewCommand` handlers and the anonymous-admin re-entry), decision 6 (self-destruct storage: PostgreSQL rows or a Redis set; the owner must accept the failure mode of the choice) and decision 7 (the delay cap). Live spike "Edited-message noise" from Gaps to Address: how often a link preview or caption touch arrives as an edit. Pitfalls 13, 15 and 16.
**Success Criteria** (what must be TRUE):
  1. With "Delete commands" on, every member's command in the group, admins' included, is deleted after the bot has answered it, and the answer still arrives.
  2. With "Delete edited messages" on, a member's message edited after the grace period (1 minute by default, adjustable on the page) is deleted. Edits by admins and approved users are kept.
  3. With "Self-destruct" on, every member message is deleted N minutes after it was sent. Messages from admins and approved users, and pinned messages, stay.
  4. In a busy group with self-destruct on, deletions keep pace without hitting Telegram's limits or slowing the bot's replies, and nothing on the never-delete list (Staff Group items, lockdown alerts, captcha challenges, log-channel posts) is touched.
**Plans:** TBD
**UI hint**: yes

### Phase 18: Remaining Settings Pages

**Goal:** As a group owner, I want warnings, reports, language, the log channel, anonymous-admin mode and locks on menu pages, so that I can configure them without commands.
**Mode:** mvp
**Depends on:** Phase 10 (the owner gate and the main menu grid)
**Requirements:** PAGE-01, PAGE-02, PAGE-03, PAGE-04, PAGE-05, PAGE-06, PAGE-10
**Research flags:** Standard patterns (Stage 5 in SUMMARY.md): one apply function per setting beside its command and one page per file, as Phase 9 set up. Check which apply functions must be extracted first (`applyWarnLimit`, `applyWarnMode` and the rest), which right the Anonymous admins page needs (PAGE-05 says the same as `/anonadmin`; the research suggests the owner), locks paging within the 64-byte callback cap (Pitfall 28), and labels that still fit in the longer languages (Pitfall 29).
**Success Criteria** (what must be TRUE):
  1. A Warnings page sets the warn limit and the action at the limit. A switches page turns reports, the AI spam filter (`/aispam`) and auto-approve of join requests on or off.
  2. A Language page sets the group's language, and the menu redraws in the new language.
  3. A Log channel page shows the group's log channel and turns each log category on or off; linking a new channel stays a command. An Anonymous admins page sets the anonymous-admin mode, with the same rights `/anonadmin` needs.
  4. A Locks page turns each lock type on or off, paged.
  5. Every setting changed on these pages is stored, cached and refused exactly as by its command, and the page and the command always show the same value.
**Plans:** TBD
**UI hint**: yes

### Phase 19: Blacklist, Filter and Note Pages

**Goal:** As a group owner, I want to browse, delete and add blacklisted words, filters and notes from the bot's private chat, so that I can manage long lists without commands.
**Mode:** mvp
**Depends on:** Phase 11 (the private input state the add flows type into)
**Requirements:** PAGE-07, PAGE-08, PAGE-09
**Research flags:** Design the list add flows (a filter and a note take two typed messages: the keyword or name first, kept in the wait state, then the reply or content), validation that refuses a blacklist word or filter that would match everything, and delete by row ID within the 64-byte cap. Pitfalls 26 and 28. Search inside long lists stays deferred (MENU2-04).
**Success Criteria** (what must be TRUE):
  1. A Blacklist page lists the group's blacklisted words (paged), deletes one, adds one typed in the private chat, and sets the blacklist action.
  2. A Filters page lists the filters (paged), deletes one, and adds one by typing the keyword and then the reply.
  3. A Notes page lists the notes (paged), deletes one, and adds one by typing the name and then the content.
  4. Each add, delete and action change is stored, cached and refused exactly as the matching command does it, and works in the group at once: the blacklisted word is acted on, the filter answers and the note can be fetched.
**Plans:** TBD
**UI hint**: yes

## Progress

**Execution Order:**
Phases run in numeric order, 10 → 19, which is the owner's priority. Phase 18 needs only Phase 10 and Phase 19 needs only Phase 11, but each waits its turn unless the owner says otherwise.

| Phase | Plans Complete | Status | Completed |
|-------|----------------|--------|-----------|
| 10. PM Settings Hub and Owner Gate | 11/11 | In Progress | - |
| 11. Message Editor | 14/14 | In Progress | - |
| 12. Rules Media, Media Under Text and Greet Once | 15/16 | In Progress | - |
| 13. Member Requirements | 14/14 | In Progress | - |
| 14. Profile Photo and Channel Rules | 12/12 | In Progress | - |
| 15. AI Spam-Name Check | 11/11 | In Progress | - |
| 16. Scheduled Deletion of Service and Bot Messages | 7/9 | In Progress | - |
| 17. Command, Edit and Self-Destruct Deletion | 0/TBD | Not started | - |
| 18. Remaining Settings Pages | 0/TBD | Not started | - |
| 19. Blacklist, Filter and Note Pages | 0/TBD | Not started | - |
