# Phase 10: PM Settings Hub and Owner Gate - Context

**Gathered:** 2026-11-03
**Status:** Ready for planning

<domain>
## Phase Boundary

A group's owner opens its settings from the bot's private chat and decides whether the group's admins may change them. `/settings` in the bot's private chat lists the groups the user may manage (today it refuses private chats), and picking one opens that group's main menu after a live check. By default only the group's live owner gets in, through every door: the private list, in-group `/settings`, the `/start settings_<chat id>` deep link and every button press. An owner-only switch, "Admins can change settings", lets live administrators in. In-group setting commands (`/setrules`, `/setwelcome`, `/captchamode` and the rest) are not gated and keep working for admins exactly as before.

Requirements: MENU-09, MENU-10, MENU-11, MENU-12, MENU-13, MENU-14, MENU-15, PLAT-05.

Out of scope for this phase (they belong to later phases):
- Any new settings page beyond the Access page (the message editor is Phase 11, member requirements Phase 13, deletion Phases 16 and 17, the remaining pages Phases 18 and 19).
- Gating the in-group setting commands. The owner decided only the menu is gated.
- Settings changes posted to the group's log channel (MENU2-01, deferred since v1.0).

This phase carries a UI hint: a UI design contract (`10-UI-SPEC.md`, via `/gsd-ui-phase 10`) is expected before planning, for the list page, the Access page and the two-column main menu.

</domain>

<decisions>
## Implementation Decisions

### The private group list (MENU-09)
- **D-01:** Each row of the list is one button showing the group's title only, like the owner's GroupHelp screenshot. No role marker and no attention marker. The title is escaped and spliced in after translation. A pick opens that group's main menu through the existing live-checked pipeline.
- **D-02:** A group where the bot is not an admin is hidden. The list holds only groups the bot has verified, because the bot learns a group's owner and admins from its admin list, which it loads only where it is an admin. The empty-list hint tells owners to make the bot an admin and run `/admincache`.
- **D-03:** `/settings` in the private chat always shows the list, even when the user has exactly one manageable group.
- **D-04:** The list is alphabetical by group title, eight groups per page, paged.

### How the list fills and stays fresh (MENU-10)
- **D-05:** Right after deploy the list is filled by a one-time startup backfill: the bot loads the admin list of every group it is in where it is an admin, paced to respect Telegram's limits and shared across replicas. An hourly sweep then rechecks, modelled on the Staff Group sweeper (`StartStaffSweeper`: first run shortly after start, a Redis `SETNX` lock across replicas, no lock needed to stay correct without Redis), so ownership changes Telegram never announced are caught without anyone acting.
- **D-06:** The record is also refreshed whenever the bot loads a group's admin list, on a promotion or demotion update, and cleared when the bot leaves the group (from MENU-10). It is only a hint: every pick is checked live, and a stale row is refused at the pick, never trusted.
- **D-07:** The repair path for a missing group is `/admincache` (or `/settings` in that group) and nothing else. There is no `/reload` alias and no Refresh button. The empty or incomplete list says so.

### The "Admins can change settings" switch (MENU-11, MENU-12, MENU-14)
- **D-08:** The switch lives on its own Access page, opened from an "Access" entry on the main menu. That entry and page are shown only to the live owner; an admin who is allowed in never sees them.
- **D-09:** Turning the switch ON asks for a second tap first (the shared two-tap card of `settings_card.go`: stateless, asker and expiry minute in the data, five minutes, Yes re-checks the owner live). Turning it OFF is a single tap, because it only narrows access.
- **D-10:** When the owner turns the switch OFF, an admin who already has a menu open is refused on their next press with a short alert, and the menu is left as it is. No extra machinery: every press already re-checks live, and the gate is part of that check.
- **D-11:** Nothing announces a flip anywhere (no group notice, no log-channel post). The Access page shows the current state.
- **D-12:** A change applies on every replica at the next press (roadmap criterion 3). How the stored value is read to guarantee that is phase research's call (see Open items).

### Refusals and the group with no owner (MENU-11, MENU-13)
- **D-13:** An admin who is not the owner and types `/settings` in a group while the switch is off gets a plain public refusal: only the group's owner can open settings. The reply does not mention the switch or the Access page, and never names the Staff Group.
- **D-14:** When a user's private list is empty (including an admin whose groups are all gated), it shows one neutral text for everyone: no groups to show; only group owners can change settings unless an owner lets admins in; if a group you own is missing, make sure the bot is an admin there and send `/admincache` in it. The text never differs by what the bot knows about the person, so it reveals which groups exist to no one.
- **D-15:** A group with no live owner (the creator's account is gone) lets its live-checked administrators in, as in v1.0, so the group can still be configured. Every press is still a live `getChatMember` check.

### Carried forward from Phase 9 and v1.1 (not reopened)
- **D-16:** The owner gate sits on top of the live admin check and never replaces it. Every door re-checks the person with `getChatMember` against the group it affects, never the admin cache or Redis state (Phase 9 D-04, roadmap constraint).
- **D-17:** An owner posting anonymously is identified by the proof tap, and the tapper's live status decides (existing `PromptAnonAdminProof` and `RegisterAnonymousAdminHandler` re-entry).
- **D-18:** A private menu speaks the opener's language, a group menu the group's language (Phase 9 D-10). Nothing shows the Staff Group's title or ID to anyone but the live creator of the group being configured (Phase 9 D-21, D-32).
- **D-19:** One setting, one code path: the Access page's write goes through one apply function that holds validation, the refusals, the write and its `cache.DeleteCache`, with the parity, drift and authority tests Phase 9 set up. Because the menu is now intentionally stricter than the commands (owner-only), the authority-parity tests get a documented carve-out; the commands are not tightened.
- **D-20:** The main menu is a two-column grid with Close, in the opener's language, and every page has Back (roadmap criterion 4). Every new message and button label exists in all 8 locales (en, es, fr, hi, id, pt, ru, uk), and `AGENTS.md` is updated to say 8 (PLAT-05).
  - *Clarification recorded 2026-11-04 during the autonomous run (orchestrator, not a new owner choice; flagged for the owner's review):* "every page" means every page of a group's menu, i.e. every page reached from that group's main menu, which is how the roadmap sentence reads ("shows every page of the group ... and every page has Back") and how Phase 9 D-06 worded it ("every sub-page has Back and Close"). The private group list is the top level of the hub, not a page of any group, and has no parent to go back to, exactly as the main menu had none in v1.0, so its navigation is Close only. In a private menu the main page gains a Groups button that returns to the list.

### Claude's Discretion
- Exact button labels, emoji, page-control layout, and where the Access entry sits in the two-column grid (settled in `10-UI-SPEC.md`). Whether Main in a private menu gets a "Groups" button back to the list.
- Names of the new table(s), columns, repository functions and the callback namespace of the list pages (research decisions 9, 10 and the hub half of 16 in `research/SUMMARY.md`). The extraction of the shared authorization step out of `settingsPress.verify` (research proposes `settingsAuthorize`) and the in-group check (`requireSettingsAccess`).
- The pacing, lock and first-run timing of the startup backfill and the hourly sweep, and their shutdown drain.
- The exact words of the deep-link refusal for a gated admin and of the in-menu refusal alerts, as long as no refusal names the Staff Group or reveals whether the bot is in a given chat to someone who is not an admin of it.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Phase scope and requirements
- `.planning/ROADMAP.md` § Phase 10 — goal, five success criteria, research flags, the settled owner decisions and cross-cutting constraints
- `.planning/REQUIREMENTS.md` — MENU-09..15 and PLAT-05 (v1.1); MENU-01..08 in `.planning/milestones/v1.0-REQUIREMENTS.md`
- `.planning/PROJECT.md` — Current Milestone v1.1, key decisions, v1.1 Telegram limits (the Bot API cannot list a user's groups)

### Research for this phase
- `.planning/research/SUMMARY.md` — Stage 1, "Decisions for phase research" 9, 10 and 16 (list table and feed, owner-flag table and read policy, callback namespace), Pitfalls 6 to 9, Gaps to Address
- `.planning/research/ARCHITECTURE.md` — the hub and gate integration points (`settingsAuthorize`, `requireSettingsAccess`, the group-list projection fed from the admin-cache load path)
- `.planning/research/PITFALLS.md` — pitfalls 6 to 9 (stale owner switch on another replica, anonymous admin taken for the owner, refusal texts that reveal a group exists, empty list for an owner who never spoke)
- `.planning/research/STACK.md` — the admin-index and access-table sketches (decision inputs, not a locked choice)
- `.planning/research/FEATURES.md` — PM hub and owner gate table stakes and anti-features

### Phase 9 (the menu this phase extends)
- `.planning/milestones/v1.0-phases/09-settings-menu/09-CONTEXT.md` — D-01..D-34, especially D-02 (c and u fields), D-04 (live check every press), D-09 and D-33 (two-tap cards bind their object), D-21 and D-32 (never name the Staff Group), D-23 (reuse command texts), D-29 (deep link router)
- `.planning/milestones/v1.0-phases/09-settings-menu/09-SECURITY.md` — threat model of the press pipeline (open notes U-1 to U-4)
- `.planning/milestones/v1.0-phases/09-settings-menu/09-UI-SPEC.md` — the visual contract the Access page and the grid extend

### Rulebook
- `AGENTS.md` — Handlers (Settings module, `settings_` deep link), Callbacks (`settings|v1`, the press pipeline), Permissions (`settingsRight*`), Data (`skipLocal`, `DeleteCache`, operational Redis keys), Migrations, Testing (settings toolkit and parity tests). It must be updated in the same commit as any rule this phase changes (locale count, new table's backup status, the sweeper and its drain).

### Owner's request
- The owner's document "Fuku Bot New T" (not in the repo): the PM group list screenshot and the main-menu screenshot are the layout reference, and the owner's wording is "a toggle that lets me as an owner of a group decide whether I want regular admins to use /settings to configure anything. As a default, only the owner of a group can change bot's settings."

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `fuku/modules/settings.go`: `settingsDesc` (its `RequiredChecks` are `requireLockdownGroup()` and `requireLockdownAuthority(false)`, so `/settings` refuses private chats today), `settingsChoiceScreen`, `settingsStartDeepLink` (strict payload parsing, live `getChatMember`, one `getChat`), `LoadSettings`, `RegisterAnonymousAdminHandler("settings", ...)`, `RegisterDeepLinkHandler("settings_", ...)`.
- `fuku/modules/settings_menu.go`: `settingsPress.verify` (steps 1 to 5: decode, service identity, opener, live `getChatMember`, then the control's right), the `settingsRight*` constants, the core action codes.
- `fuku/modules/settings_pages.go`: `registerSettingsPage` (panics at init on a duplicate), `settingsMainEntries`, `renderSettingsMain`.
- `fuku/modules/settings_card.go`: the shared two-tap confirm card (asker, expiry minute, `settingsCardExpired`).
- `fuku/modules/lockdown.go`: `lockdownLiveMember`, `requireLockdownGroup`, `requireLockdownAuthority`.
- `fuku/utils/cache/adminCache.go`: `LoadAdminCache` and `loadAdminCacheFromTelegram` (stores a negative entry when the bot is not an admin, so the feed only exists where the bot is an admin); `fuku/modules/admin.go` `adminCache` (reloads it; needs an admin user and an admin bot) and `clearAdminCache`.
- `fuku/modules/staff_sweeper.go`: the hourly sweeper pattern (first run 1 to 5 minutes after start, `SETNX` lock, drain).
- `chat_status.IsBotMissingError`, `PromptAnonAdminProof`, the `settings_*` locale keys and `fuku/i18n/settings_locale_test.go` (a limits table that must cover exactly the `settings_` keys).

### Established Patterns
- Stateless menu: no Redis key, so a press works on any replica. Phase 10 adds the first state the menu reads (the owner switch and the admin record), so freshness across replicas needs an explicit policy (fresh read, or cached and listed in `skipLocal` with a two-instance test).
- Every press is answered exactly once, only through `settingsPress.answer`; edit in place; "message is not modified" is success.
- Callback data only through `callbackcodec`, 64 bytes; a worst-case test fails on any overflow.
- Every write calls `cache.DeleteCache`; new tables read fresh or through `cache.GetFromCacheOrLoad`; tables that must not travel are kept out of backup, export, import and reset.
- A page is one `registerSettingsPage` call from its own file's `init` plus its strings, and never edits `settings_pages.go`.

### Integration Points
- New: a migration, model and repository for the owner switch and the admin record; a hook where the admin list is loaded; the startup backfill and hourly sweep with their drain; a new Access page file; the private `/settings` entry and the list pages.
- Changed: `settingsDesc` checks (the in-group door), `settingsStartDeepLink` (the gate), `settingsPress.verify` (the gate, shared with the other doors), `renderSettingsMain` (two-column grid, Access entry).
- Handler groups: this phase adds none (the callback is matched by prefix). A new group would need a number below 12 and an `AGENTS.md` entry.
- Phase 9 recorded "no new infrastructure" (D-31). That no longer holds here: this phase adds a migration and a background sweep.

</code_context>

<specifics>
## Specific Ideas

- The PM list should look like GroupHelp's: one plain button per group, the group's name only (the owner's "Налаштування групи" screenshot).
- "Only the owner by default" is a security stance, not a preference: when anything is unclear, the gate refuses.
- The empty list must never help someone discover which groups exist, so its text is the same for everyone.

</specifics>

<deferred>
## Deferred Ideas

Raised or offered during discussion and not chosen. None belongs in this phase.

- Role and attention markers on list rows (owned versus allowed-admin, bot not admin, lockdown active).
- Showing a group where the bot is not an admin as "unavailable" instead of hiding it.
- Opening the menu directly when the user has exactly one group.
- A `/reload` alias of `/admincache` and a Refresh button on the PM list.
- Ordering the list by last opened or newest group.
- Announcing a switch flip (a public group notice, or a log-channel post; settings changes in the log channel are MENU2-01).
- Closing an open admin menu when the switch is turned off, instead of refusing in place.
- A tailored empty-list text for admins whose groups are gated.
- Treating only admins with every admin right as the stand-in for a missing owner.

</deferred>

**Open items for phase research** (settle in `10-RESEARCH.md`, then record in this phase's plans and `AGENTS.md`):
- Decision 9: the table behind the list, its name, how it is fed, and whether it may also be seeded from `chat_users`.
- Decision 10: the owner-switch table and its read policy (fresh read or `skipLocal` with a two-instance test).
- The hub half of decision 16: whether the list pages reuse `settings|v1` or add a namespace.
- How a missing creator is recognised live without an extra Telegram call on every press (D-15).
- Whether `/admincache` already refreshes the new record on every path (it calls `LoadAdminCache`), and what the backfill does for a group whose admin list cannot be read.
- Run `make check-translations` first: `locales/uk.yml` is shorter than `en.yml` (2,437 lines against 3,095).

---

*Phase: 10-PM Settings Hub and Owner Gate*
*Context gathered: 2026-11-03*
