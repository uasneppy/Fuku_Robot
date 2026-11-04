# Requirements: Fuku Robot

**Defined:** 2026-11-02 (milestone v1.1 PM Settings Hub & Member Screening)
**Core Value:** My trusted staff can protect every one of my communities from one place. We act on a bad actor across all groups at once, and the bot never lets anyone act in a group where they aren't an admin.

v1.0 requirements (SETUP, STAFF, LOCK, RAID, CAPT, MENU-01..08, PLAT-01..04) are archived in `milestones/v1.0-REQUIREMENTS.md`. IDs below continue that numbering where a category already exists.

## v1.1 Requirements

Requirements for this milestone. Each maps to one roadmap phase.

### PM settings hub and owner gate

- [x] **MENU-09**: `/settings` in the bot's private chat lists the groups the user may manage (paged), and picking one opens that group's main menu.
- [x] **MENU-10**: The list is built from the bot's own record of each group's owner and admins, refreshed whenever the bot loads a group's admin list or sees a promotion or demotion, and cleared when the bot leaves. Every pick is checked live. An empty or incomplete list tells the user to run `/admincache` or `/settings` in the missing group.
- [x] **MENU-11**: By default only a group's live owner can open and change its settings: in the PM list, through in-group `/settings`, through the `/start settings_<chat id>` deep link and on every button press.
- [x] **MENU-12**: The owner can switch "Admins can change settings" on or off from the menu. Only the live owner sees and can press it. While it is on, live administrators get the full menu as in v1.0.
- [x] **MENU-13**: While admin access is off, a regular admin does not see the group in their PM list, and in-group `/settings` and the deep link refuse them with a short reply that never names the Staff Group. In-group setting commands (`/setrules`, `/setwelcome`, `/captchamode` and the rest) keep working for admins exactly as before.
- [x] **MENU-14**: A change to admin access applies on every replica at once. An owner posting anonymously is identified by the proof tap, and the tapper's live status decides.
- [ ] **MENU-15**: The main menu shows every page of the group in a two-column grid with Close, in the opener's language, and every page has Back.

### Message editor (Rules, Welcome, Goodbye)

- [ ] **EDIT-01**: From the menu an admin opens Rules, Welcome or Goodbye and sees which parts are set (text, media, URL buttons) with a button to set or remove each part.
- [ ] **EDIT-02**: After pressing a part's button, the bot waits for the user's next private message and stores it as that part. A Cancel button or `/cancel` leaves the message unchanged, the wait ends by itself after 10 minutes, and it works whichever replica receives the message.
- [ ] **EDIT-03**: Text keeps its formatting and the existing placeholders (`{first}`, `{mention}` and the rest). Text that is too long to send (4096 characters, or 1024 as a caption) is refused with the reason, and the wait stays open.
- [ ] **EDIT-04**: Media accepts one item of a kind the welcome already supports (photo, video, GIF, document, sticker, audio, voice). From an album only one item is kept, and the user is told so.
- [ ] **EDIT-05**: URL buttons use the bot's existing button syntax. A bad syntax or URL is refused with the reason, and the wait stays open.
- [ ] **EDIT-06**: Each part has a Preview, and "Full preview" sends the whole message exactly as members will see it, through the same send path the group uses.
- [ ] **EDIT-07**: "Media under text" shows a photo, video or GIF below its text instead of above it.
- [ ] **EDIT-08**: Rules gain media and URL buttons, and `/rules` (in the group and as private rules) shows them.
- [ ] **EDIT-09**: Welcome has "Greet once": a member is greeted only the first time they ever join that group, also when the join reaches several replicas.
- [ ] **EDIT-10**: Welcome and Goodbye have their on/off switch and "Delete last welcome/goodbye" in the menu, stored exactly as `/welcome`, `/goodbye`, `/cleanwelcome` and `/cleangoodbye` store them.
- [ ] **EDIT-11**: A message saved in the editor is stored exactly as `/setwelcome`, `/setgoodbye` or `/setrules` would store it, and a message that Telegram would refuse to send is refused before saving.

### Member requirements and name blocks

- [ ] **MREQ-01**: A Member requirements page lets the owner (or admins, when allowed) give each rule its own punishment: off, warn, kick, ban or mute. The rules are: must have a last name, must have a username, must have a profile photo, must be subscribed to the linked channel, no AI-flagged spam name, no Russian name, no Arabic name and no Chinese name. All rules are off by default.
- [ ] **MREQ-02**: With "Check on join" on, a new member is checked right after passing the captcha (or on joining, when the group has no captcha). Every member is checked when they send a message.
- [ ] **MREQ-03**: With "Delete messages" on, a message from a member who breaks a rule is deleted.
- [ ] **MREQ-04**: A violation is punished once: further messages from the same member do not add warns, notices or punishments until the member fixes it or rejoins. This holds across replicas.
- [ ] **MREQ-05**: Each violation posts one notice in the group naming the member and the rule broken. A warn counts toward the group's warn limit like any other warn.
- [ ] **MREQ-06**: A member muted by a rule gets an "I fixed it" button that only they can use. The bot re-checks them live: if the problem is fixed, they are unmuted (respecting an active lockdown) and get the welcome that was held back; if not, they are told what is still missing.
- [ ] **MREQ-07**: "I fixed it" only ever lifts a mute that a requirement rule placed. It never lifts a mute from an admin, a staff action, the captcha, antiflood or a lockdown.
- [ ] **MREQ-08**: A kicked member can rejoin and is checked again. A banned member stays banned until an admin unbans them.
- [ ] **MREQ-09**: Admins, approved users, bots, Telegram service accounts and messages sent as a channel or by an anonymous admin are never checked.
- [ ] **MREQ-10**: A check that cannot be answered (a Telegram error, a lookup that times out, the bot not being an admin of the linked channel) never punishes anyone. The page shows when a rule cannot work in that group (no linked channel, the bot not admin there, AI not configured).
- [ ] **MREQ-11**: The profile-photo rule treats a photo the bot cannot see as missing, and the page says so.
- [ ] **MREQ-12**: The channel rule is offered only in a group linked to a channel, and checks that the member is subscribed to that channel.
- [ ] **MREQ-13**: The Russian, Arabic and Chinese name rules check the member's first and last name: Russian-only letters (ы, ъ, э, ё), Arabic script and Chinese characters. The page says these are letter-based and can also catch related languages. Invisible or whitespace-only characters do not count as a last name.
- [ ] **MREQ-14**: The AI spam-name rule sends only a member's names (first name, last name, username) to Claude Haiku, on join and when the names change, never message text. An established member's names are first recorded without a call. Verdicts are cached and hourly budgets cap the cost. `ENABLE_AISPAM=false` or a missing Anthropic key turns the rule off. No verdict means no punishment.
- [ ] **MREQ-15**: Requirement notices are deleted automatically when the group's "automatic punishments" deletion timer is set.

### Message deletion

- [ ] **DEL-01**: A Service messages page sets each Telegram service-message type (member joined, member left, new photo, new title, pinned message, forum topics, boosts, video chats, auto-delete timer changes and the other types Telegram sends) to keep, delete immediately, or delete after N seconds. Joined and left share storage with the existing `/cleanservice` setting.
- [ ] **DEL-02**: A Bot messages page sets each kind of the bot's own message (welcome, goodbye, rules, automatic punishments, manual punishment replies to `/ban`, `/mute`, `/kick`, `/warn` and the like) to keep or delete after N minutes.
- [ ] **DEL-03**: With "Delete commands" on, every member's command in the group (admins' included) is deleted after the bot has answered it.
- [ ] **DEL-04**: With "Delete edited messages" on, a member's message edited after a grace period (1 minute by default, adjustable on the page) is deleted. Admins and approved users are exempt.
- [ ] **DEL-05**: With "Self-destruct" on, every member message is deleted N minutes after it was sent. Admins, approved users and pinned messages are exempt.
- [ ] **DEL-06**: Scheduled deletions survive restarts and run once across replicas. Delays are capped below Telegram's 48-hour limit, and a deletion too old for Telegram is dropped quietly.
- [ ] **DEL-07**: Deletion never touches Staff Group summaries and cards, lockdown alerts, captcha challenges, log-channel posts or anything in a Staff Group.
- [ ] **DEL-08**: When the bot lacks the right to delete messages, every deletion page says so instead of failing silently.

### Remaining settings pages

- [ ] **PAGE-01**: A Warnings page sets the warn limit and the action at the limit, stored exactly as the warn commands store them.
- [ ] **PAGE-02**: Simple switches get a page: reports on/off, the AI spam filter (`/aispam`) on/off, and auto-approve join requests on/off.
- [ ] **PAGE-03**: A Language page sets the group's language, and the menu redraws in the new language.
- [ ] **PAGE-04**: A Log channel page shows the group's log channel and turns each log category on or off. Linking a new channel stays a command.
- [ ] **PAGE-05**: An Anonymous admins page sets the anonymous-admin mode with the same rights `/anonadmin` needs.
- [ ] **PAGE-06**: A Locks page turns each lock type on or off, paged.
- [ ] **PAGE-07**: A Blacklist page lists the group's blacklisted words (paged), deletes one, adds one by typing it in PM, and sets the blacklist action.
- [ ] **PAGE-08**: A Filters page lists filters (paged), deletes one, and adds one by typing the keyword and then the reply in PM.
- [ ] **PAGE-09**: A Notes page lists notes (paged), deletes one, and adds one by typing the name and then the content in PM.
- [ ] **PAGE-10**: Every setting changed on these pages is stored, cached and refused exactly as by its command, as v1.0 required for the first menu pages (MENU-08).

### Platform

- [ ] **PLAT-05**: Every new message and button label exists in all 8 locales (en, es, fr, hi, id, pt, ru, uk), and `AGENTS.md` reflects the 8-locale set.
- [ ] **PLAT-06**: New background work (deletion queue, name checks, requirement lookups) shares Telegram rate budgets across replicas, recovers from crashes, and is drained within the existing shutdown budget.

## Future Requirements

Deferred. Tracked but not in this roadmap.

### Settings menu

- **MENU2-01**: Settings changes posted to the group's log channel (carried over from v1.0)
- **MENU2-03**: Pages for approved users, disabled commands, pins and federations
- **MENU2-04**: Search inside long blacklist, filter and note lists

### Editor

- **EDIT2-01**: Forum topic picker for the welcome message
- **EDIT2-02**: Custom notice text per requirement rule

### Deletion

- **DEL2-01**: Deleting the bot's replies to members' commands ("own commands")
- **DEL2-02**: Global silence, mass deletion and delete-all tools

### Carried over from v1.0

- **STAFF2-01**: Silent mode, deleting the staff command message after acting
- **STAFF2-02**: Re-applying active staff bans to a newly linked group
- **RAID2-01**: Alert-only mode for automatic triggers, for tuning thresholds without locking
- **RAID2-02**: Suspicious-account scoring to sharpen "Ban recent joiners"
- **RAID2-03**: External reputation lookup on join (e.g. CAS)
- **CAPT2-01**: Captcha metrics on the metrics endpoint
- **CAPT2-02**: Join-request captcha that verifies people before they're admitted

## Out of Scope

| Feature | Reason |
|---------|--------|
| Public, multi-tenant bot | Runs for the owner's own communities only |
| A web admin dashboard | Settings stay in Telegram; smaller attack surface |
| Read-only menu for regular admins without admin access | Owner's call: the group is hidden from them instead |
| Restricting in-group setting commands to the owner | Owner's call: only the menu is gated; commands stay open to admins |
| Lifting a ban or kick through "I fixed it" | A banned member cannot see the group's button; a kicked one rejoins and is re-checked |
| "Must add members" requirement | Not requested |
| Sending message text to an AI | Too slow and costly; the AI name check sends names only |
| Re-checking every existing member when a rule is switched on | Members are checked when they next send a message, which spreads the load |
| A draft-and-publish step in the editor | Each part is saved when it is sent; preview covers the review |
| Deleting admins' edited messages or self-destructing their messages | Owner's call: only commands are deleted for admins |
| Google reCAPTCHA, retrying inside the captcha page | Unchanged from v1.0 |
| Auto-lifting lockdowns | An admin decides when it's safe |

## Traceability

Which phases cover which requirements. Each v1.1 requirement maps to exactly one phase. Filled in by the roadmap.

| Requirement | Phase | Status |
|-------------|-------|--------|
| MENU-09 | Phase 10 | Complete |
| MENU-10 | Phase 10 | Complete |
| MENU-11 | Phase 10 | Complete |
| MENU-12 | Phase 10 | Complete |
| MENU-13 | Phase 10 | Complete |
| MENU-14 | Phase 10 | Complete |
| MENU-15 | Phase 10 | Pending |
| EDIT-01 | Phase 11 | Pending |
| EDIT-02 | Phase 11 | Pending |
| EDIT-03 | Phase 11 | Pending |
| EDIT-04 | Phase 11 | Pending |
| EDIT-05 | Phase 11 | Pending |
| EDIT-06 | Phase 11 | Pending |
| EDIT-07 | Phase 12 | Pending |
| EDIT-08 | Phase 12 | Pending |
| EDIT-09 | Phase 12 | Pending |
| EDIT-10 | Phase 11 | Pending |
| EDIT-11 | Phase 11 | Pending |
| MREQ-01 | Phase 13 | Pending |
| MREQ-02 | Phase 13 | Pending |
| MREQ-03 | Phase 13 | Pending |
| MREQ-04 | Phase 13 | Pending |
| MREQ-05 | Phase 13 | Pending |
| MREQ-06 | Phase 13 | Pending |
| MREQ-07 | Phase 13 | Pending |
| MREQ-08 | Phase 13 | Pending |
| MREQ-09 | Phase 13 | Pending |
| MREQ-10 | Phase 14 | Pending |
| MREQ-11 | Phase 14 | Pending |
| MREQ-12 | Phase 14 | Pending |
| MREQ-13 | Phase 13 | Pending |
| MREQ-14 | Phase 15 | Pending |
| MREQ-15 | Phase 16 | Pending |
| DEL-01 | Phase 16 | Pending |
| DEL-02 | Phase 16 | Pending |
| DEL-03 | Phase 17 | Pending |
| DEL-04 | Phase 17 | Pending |
| DEL-05 | Phase 17 | Pending |
| DEL-06 | Phase 16 | Pending |
| DEL-07 | Phase 16 | Pending |
| DEL-08 | Phase 16 | Pending |
| PAGE-01 | Phase 18 | Pending |
| PAGE-02 | Phase 18 | Pending |
| PAGE-03 | Phase 18 | Pending |
| PAGE-04 | Phase 18 | Pending |
| PAGE-05 | Phase 18 | Pending |
| PAGE-06 | Phase 18 | Pending |
| PAGE-07 | Phase 19 | Pending |
| PAGE-08 | Phase 19 | Pending |
| PAGE-09 | Phase 19 | Pending |
| PAGE-10 | Phase 18 | Pending |
| PLAT-05 | Phase 10 | Pending |
| PLAT-06 | Phase 14 | Pending |

PLAT-05 and PLAT-06 are cross-cutting: they are mapped to the first phase where they matter and bind every later phase (see Cross-Cutting Constraints in `ROADMAP.md`).

**Coverage:**
- v1.1 requirements: 53 total
- Mapped to phases: 53
- Unmapped: 0 ✓

---
*Requirements defined: 2026-11-02*
*Last updated: 2026-11-02 after the v1.1 roadmap (Phases 10-19) was created*
