# Phase 9: Settings Menu - Context

**Gathered:** 2026-10-30
**Status:** Ready for planning
**Mode:** Smart discuss (autonomous); the owner accepted every recommended answer in all four areas.

<domain>
## Phase Boundary

A group admin manages the group from one button menu, so Staff Group links, lockdown, captcha and antiflood need no commands. `/settings` in a group asks whether to open the menu in the group or in private. The menu is one message edited in place with Back and Close, every press re-checks live that the presser is still an admin of the group being configured, and a setting changed in the menu is the same setting as when changed by command.

Requirements: MENU-01, MENU-02, MENU-03, MENU-04, MENU-05, MENU-06, MENU-07, MENU-08.

There is no spike gate. The roadmap marks this phase "UI hint: yes", so a UI design contract (`09-UI-SPEC.md`) is produced before planning.

Out of scope:
- Menus for other modules (welcome, rules, warns, filters, notes and so on). Pages are registered in one table so they can be added later.
- Starting a lockdown from the menu (`/lockdown` stays a command; the Lockdown page shows state, Lift and the trigger settings only).
- Antiflood actions other than mute, ban and kick (`warn`, `tban`, `tmute`), which no command sets today (D-20).
- Any new setting. The menu exposes existing settings through the same code the commands use.
- Running live Telegram checks. They are deferred to a checklist (`09-SPIKE.md`) like Phases 7 and 8.

</domain>

<decisions>
## Implementation Decisions

### Entry, ownership and authority (MENU-01, MENU-02, MENU-03)
- **D-01:** `/settings` is a group command through `helpers.WrapCommand` (admin check, group only, `Disableable` not set) that replies with two buttons: "Open here" and "Open in private". The command sender must pass a live `getChatMember` admin check (creator or administrator), never the admin cache. Anonymous admins get the usual proof first and re-enter through `RegisterAnonymousAdminHandler`.
- **D-02:** A menu opened in the group carries the opener's user ID in every button (`u`). It responds only to that user; anyone else gets a short alert and nothing changes. A menu opened in private carries the group's chat ID in every button (`c`) and manages only that group. In private there is no opener field: the presser is the user in the private chat.
- **D-03:** "Open in private" is a URL button, a Telegram deep link `https://t.me/<bot>?start=settings_<chat id>`, so it works even when the admin never started the bot. The `/start` handler gets one payload branch that parses `settings_<id>` strictly (digits with an optional leading minus, the same bounds as other chat IDs), runs the live admin check for that user and group, and sends the main page to the private chat. A refused check answers with a plain "you are not an admin of that group" reply and sends no menu.
- **D-04:** Every button press re-checks live with `getChatMember` that the presser is still the creator or an administrator of the group being configured (the callback's group in private, the chat the message sits in for a group menu). A failed lookup refuses, like the lockdown buttons. A demoted admin's press is refused and edits nothing.
- **D-05:** Changing a setting needs the same right the matching command needs, checked live per setting, so the menu can never do more than the command: lockdown Lift and the lockdown trigger switches and thresholds need the creator or an administrator with `can_restrict_members` (`lockdownButtonAuthority` / `requireAutoLockdownAuthority`'s rule); Staff Group link and unlink need the live owner of both groups (`chat_status.CheckOwner`, the `/linkstaff` and `/unlinkstaff` rules); captcha and antiflood changes need the admin right those commands check. A refusal shows the command's own refusal text (or an alert) and writes nothing.

### Menu shape (MENU-04)
- **D-06:** One message, edited in place. The main page has buttons Staff Group, Lockdown, Captcha, Antiflood and Close. Every sub-page has Back and Close. Close deletes the menu message; when it cannot (private chat after 48 h, or a missing right) it edits the message to a short "Menu closed" note with no buttons.
- **D-07:** Callback data uses a new `settings|v1` namespace through `alita/utils/callbackcodec` only (64-byte cap, never `strings.Split`). Fields: page, action, and the group ID (private menu) or the opener's ID (group menu), plus a value where a step needs one. Chat IDs are up to 14 characters with the sign, so a field set stays under the cap; an encode overflow is a bug caught by a test, never a dead button.
- **D-08:** No free-text input and no Redis state. Numbers move with stepper buttons, choices with buttons, and confirmation with a second tap. The menu is stateless, so it works without Redis and on any replica.
- **D-09:** Dangerous actions (Lift lockdown, Unlink Staff Group) ask for a second tap in place ("Lift now? Yes / No") before they run. The Yes press re-checks authority live again and carries an expiry minute like the lockdown cards, so a stale confirm is refused.
- **D-10:** Language: a menu opened in the group speaks the group's language; a menu opened in private speaks the admin's language. All strings are new `settings_*` keys in all 7 locale files, and every user-controlled string (group titles, names) is escaped and spliced in after the translator.
- **D-11:** A press on a menu that cannot render (the group no longer exists, the bot was removed, a lookup failed) answers an alert and edits to a short "cannot open" note; it never leaves a dead button. Every callback is answered exactly once.

### Pages and controls (MENU-05, MENU-06, MENU-07)
- **D-12:** Staff Group page. In a group it shows the link state and health (the same information as the `/staff` row for this group: linked or not, the Staff Group, the bot's health, a lockdown marker) from the fresh `staff.*Fresh` reads. Link and Unlink buttons call the same functions as `/linkstaff` and `/unlinkstaff` (live owner of both groups only; Unlink asks a second tap). Opened inside a Staff Group it shows the `/staff` panel information read-only (the linked groups and their status) and offers no link or unlink there.
- **D-13:** Lockdown page. It shows the current state (since when, who, the reason or the trigger sentence, confirmed or lifting) from a fresh `chat_lockdowns` read, a Lift button that takes the same path as `/unlockdown` (`liftLockdownRow`, a second tap first), and for each of join surge, flood and AI spam an on/off switch and a threshold stepper from 3 to 500, written by the `raiddetection.Set*` functions with the `autolockdown` grammar's rules (a threshold also turns its trigger on). It does not start a lockdown. A basic group shows a "needs a supergroup" note.
- **D-14:** Captcha page. It shows and sets: on or off, the mode (math, text, turnstile; turnstile with the same three ordered refusals as `/captchamode`), the time limit (1 to 10 minutes, a stepper), and the failure action (kick, ban, mute). Each change goes through the same apply code as `/captcha`, `/captchamode`, `/captchatime` and `/captchaaction`.
- **D-15:** Antiflood page. It shows and sets: the limit (a stepper; 0 means off as it does for `/setflood`), the action (mute, ban, kick, warn; `tban` and `tmute` are shown but stay command-only because they need a duration), and the delete-flood-messages switch. Each change goes through the same apply code as `/setflood`, `/setfloodmode` and `/delflood`.
- **D-16:** Pages are rendered by pure functions from a settings snapshot (like `renderStaffPanel`), built by one loader per page from fresh or cached reads. A registry table lists the pages (key, title key, render, handle), so adding a page later is one table entry plus its strings.

### One setting, one code path (MENU-08)
- **D-17:** Each setting the menu can change has one apply function that both the command handler and the menu call. It holds validation, the setting-specific refusals (for example Turnstile's three), the repository write and the `cache.DeleteCache` of the affected keys, and returns a typed result the command renders as its reply and the menu renders as a page or alert. The existing command handlers are refactored to call it; their observable replies do not change.
- **D-18:** Pages read through the same cached getters the commands read, so the command's answer and the menu's value are always the same. Keys that a replica could serve stale from the in-process layer are already in `skipLocal` where freshness matters; the plan checks each setting the menu edits.
- **D-19:** Tests prove parity per setting: change by command and read the menu's value; change by menu and read what the command shows; compare the stored rows; and a drift test that fails when a command and the menu disagree about validation (for example a threshold of 2, a time limit of 11, an unknown mode). Authority parity is tested too: a press that the command would refuse is refused by the menu.

### Amendments after the UI contract (orchestrator's calls, 2026-10-30)
- **D-20 (refines D-15, A-1):** `/setfloodmode` accepts only `ban`, `kick` and `mute`; nothing in the code writes `warn`, `tban` or `tmute`, and no antiflood command takes a duration. The Antiflood page therefore offers mute, ban and kick. A stored `warn`, `tban` or `tmute` (an old or imported row) is shown read-only with a note and is never claimed to be settable by a command. The limit control is Off, then 3 to 100, because that is what `/setflood` accepts.
- **D-21 (privacy, A-2):** Phase 1's rule is that nothing about a link is shown to the linked group's members. A menu opened in the group is a message every member can read, so there the Staff Group page shows only a masked line (linked or not, and the bot's health, never the Staff Group's name or ID). The private menu and a menu opened inside a Staff Group show the full information. This keeps D-12's information for the admin who asked for it in private.
- **D-22 (Link picker, A-3):** an owner can have several Staff Groups and D-08 allows no free text, so Link opens a stateless picker with one button per Staff Group the presser owns, each verified live at the press, then calls the same function as `/linkstaff`. A presser who owns no Staff Group gets the command's own refusal.
- **D-23 (reused strings, A-4):** where a command already has the refusal or state text, the menu reuses that existing key, in all 7 languages, instead of duplicating it as a `settings_*` key, so "the menu refuses with the command's words" holds. New `settings_*` keys cover only what is new (page titles, labels, menu-specific alerts).
- **D-24 (value button, A-5):** a stepper's value button redraws the page, which also re-runs the live admin check. A double tap is harmless because a stepper button sends the absolute target value.

### Amendments after research (orchestrator's calls, 2026-10-30)
Where `09-UI-SPEC.md` and these amendments differ, the amendments win; `09-RESEARCH.md` "Spec Deltas" (12 rows) is the list the plans must apply.
- **D-25 (authority is what the code does, refines D-05):** antiflood changes (`/setflood`, `/setfloodmode`, `/delflood`) need a live admin and the bot as an admin (`IsUserConnected(..., true, true)`, refusal `connections_is_user_connected_bot_not_admin`); `/captcha` on/off needs the user and the bot as admins; `/captchamode`, `/captchatime` and `/captchaaction` need the user only. The menu applies exactly these, and an authority-parity test proves it refuses what each command refuses and allows what it allows.
- **D-26 (menu Lift posts the public notice):** a Lift from the menu does what the lockdown alert's Lift does: it runs `liftLockdownRow`, posts the public `lockdown_lifted` notice into the locked group through `lockdownPaced` (`lockdownPostLiftNotice`) and wakes the worker, in both menu variants.
- **D-27 (antiflood first-write side effect kept):** `SetFloodMode` and `SetFloodMsgDel` on a chat with no row probably also store the default limit 5, which turns antiflood on. The menu keeps the commands' behaviour (D-17), and its apply functions return a database read-back (the uncached row) so the page shows the stored value, never an assumption. A characterization test settles what the commands really do first. A repository fix is a separate follow-up that would change the commands too (recorded in `STATE.md`).
- **D-28 (cross-replica skew accepted):** `captcha_settings` and `antiflood` keys are not in `skipLocal`, so another replica can show the old value for up to `CACHE_LOCAL_TTL` (10 s), exactly as the commands can. Adding them would put a Redis round trip on every group message, so it is not done. The replica that took the press is always consistent.
- **D-29 (seams and registration):** the Staff page splits `renderStaffRow` and gives `staffPanelTextFor` a `limit` so the menu can reuse the panel text without its Add group, Recent and Unlink buttons or the linked group's title and ID line; the link flow splits `resolveOwnStaffGroup` into `listVerifiedOwnStaffGroups`, `linkToStaffGroup` and `announceStaffLink` (the commands call the same pieces). The private entry registers through `RegisterDeepLinkHandler("settings_", ...)` (no edit to `help.go`'s `start`). `/settings` uses the `requireLockdownGroup()` and `requireLockdownAuthority(false)` checks (a live creator or administrator), `RegisterAnonymousAdminHandler`, `RegisterLegacyModule("Settings", ...)`, `settings_help_msg` and `alt_names.Settings`. The new key `settings_lockdown_reason` is dropped in favour of the existing `lockdown_reason_line` (D-23).
- **D-30 (structural checks and error mapping):** callback structure is checked before any I/O: `c` is accepted only on a private message and `u` only on a non-private one, so a forwarded menu cannot act in another chat; a service identity (`staffServiceUserIDs`) is refused first; then the live check against the chat the message really sits in (group menu) or the verified `c` (private). `chat_status.IsBotMissingError` is exported (one line over the existing classifier) so a private menu whose group is gone or whose bot was removed gets the D-11 cannot-open edit, while any other lookup error is `settings_check_failed`. Pages read antiflood through `GetAntifloodSettingsCachedContext` (the same cache entry the command reads, with a real error), never `GetFloodContext`, which hides read errors. One answer helper sends alerts (`answerInvalidCallback` is a toast).
- **D-31 (no new infrastructure):** no migration, model, Redis key, shutdown drain, handler group or package is added (the harness `AutoMigrate` lists already hold every model the pages read). The menu answers the callback empty first and then works, so a slow Staff page build or lift cannot outlast the callback query.

### Claude's Discretion
- The exact `settings|v1` field names and abbreviations, and how a stepper encodes its direction and size (for example a step of 1, 5 or 10 for thresholds).
- Whether the main page and sub-pages share one render pipeline with a header and footer, the exact layout, emoji and wording (settled in `09-UI-SPEC.md`).
- Where the apply functions live (a new `alita/modules/settings_apply.go` or beside each command) and how the typed results are named.
- How the `/start` payload branch is wired into the existing help module's `start` handler without changing its other behaviour.

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets
- **Staff Group:** `alita/modules/staff.go` (`setStaff`), `staff_link.go` (`linkStaff`, unlink handlers), `staff_panel.go` (`renderStaffPanel`, `buildStaffPanelRows`, `buildStaffPanel`, `staffPanelPage`, `staffPanelRefresh`), `alita/db/staff` (`*Fresh` reads), `chat_status.CheckOwner`.
- **Lockdown:** `lockdown.go` (`requireLockdownAuthority`, `liftLockdownRow`, `/unlockdown`), `lockdown_alert_callbacks.go` (`lockdownButtonAuthority`, `lockdownInviteAuthority`, the two-tap confirm card with asker and expiry in the data), `autolockdown.go` (`autolockdownApply`, `requireAutoLockdownAuthority`), `alita/db/raiddetection` (`GetSettings`, the six `Set*` upserts), `alita/db/lockdown` (fresh state reads).
- **Captcha:** `captcha.go` (`captchaCommand`, `captchaModeCommand` with `captchaWebModeRefusal`, the time and action commands, `captchaAdminGate`), `alita/db/captcha` (`GetCaptchaSettings`, `SetCaptchaMode` and the other setters).
- **Antiflood:** `antiflood.go` (`setFlood`, `setFloodMode`, `setFloodDeleter`, `flood`), `alita/db/antiflood` (`GetFlood`, `SetFlood`, `SetFloodMode`, delete setter).
- **Callbacks and permissions:** `alita/utils/callbackcodec` (`Encode`, `Decode`, `Field`), `callbackquery.Prefix`, the anonymous-admin re-entry (`RegisterAnonymousAdminHandler`), `chat_status` live predicates (`FetchBotMember`, `CheckOwner`), `helpers.WrapCommand`.
- **Start/help:** `help.go` registers `start` (`DefaultHelpRegistry().start`); the payload branch for `settings_<id>` is added there.

### Established Patterns
- Stateless two-tap cards: the asker and an expiry minute travel in the callback data, only the asker may confirm, expiry is lazy.
- Pure render functions over a snapshot; loaders do the reads; handlers answer the callback exactly once.
- Live authority at the ask and again at confirm; a failed lookup refuses.
- Every write calls `cache.DeleteCache` on the keys it affects; new strings go in all 7 locale files; tests use real fixtures (SQLite harness, miniredis, hand-written `BotClient` fakes).
- The docs generator reads `locales/en.yml`; a new command's help text must fit Telegram's 4096-character limit in every locale.

### Integration Points
- New `alita/modules/settings.go` (command, callback handler, pages) registered with `RegisterLegacyModule`; `/settings` added to the help registry and the generated docs; a `settings_apply` layer shared with `captcha.go`, `antiflood.go`, `autolockdown.go`, `staff_link.go` and the lockdown commands.
- `help.go` `start` handler: the `settings_<id>` payload branch.
- `AGENTS.md`: the settings menu callback namespace, the authority rule per setting, the apply-function rule (a command and the menu never diverge), and the `/start` payload rule.

</code_context>

<specifics>
## Specific Ideas

- The menu should feel like GroupHelp's: one tidy message, big clear buttons, a current value shown on every control, Back and Close always in the same place.
- A setting's current value is visible on the page and on its button (for example "Time limit: 2 min  −  +").
- "The menu can never do more than the command" is the guiding rule: where a command refuses, the menu refuses with the same words.

</specifics>

<deferred>
## Deferred Ideas

- Menu pages for other modules (welcome, rules, warns, filters, notes, locks, approvals).
- Starting a lockdown, or running a staff action, from the menu.
- `tban` and `tmute` antiflood actions with a duration picker.
- A menu auto-close timer for group menus left open.
- Per-admin "last menu" shortcuts and a connected-group picker in private.

</deferred>
