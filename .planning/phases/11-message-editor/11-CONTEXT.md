# Phase 11: Message Editor - Context

**Gathered:** 2026-11-04
**Status:** Ready for planning

<domain>
## Phase Boundary

A group's owner writes the Welcome, Goodbye and Rules messages from the bot's private chat by sending them as messages, instead of typing `/setwelcome`, `/setgoodbye` or `/setrules` in the group. Each of the three gets a settings page that shows which parts are set (text, media, URL buttons), a Set and a Remove button per part, a Preview and a Full preview. After Set, the owner's next private message becomes that part. Welcome and Goodbye also get their on/off switch and "Delete last welcome/goodbye" on the same pages.

Requirements: EDIT-01, EDIT-02, EDIT-03, EDIT-04, EDIT-05, EDIT-06, EDIT-10, EDIT-11.

Out of scope for this phase (they belong to later phases):
- Media and URL buttons for Rules, `/rules` showing them, "Media under text" and "Greet once" (Phase 12; this phase's Rules page offers the text part only and the page is built so Phase 12 adds the other parts without reworking it).
- The member requirement rules, deletion pages and the remaining settings pages (Phases 13 to 19).
- Settings changes posted to the group's log channel (MENU2-01, deferred since v1.0).
- A "type a number" input for stepper buttons: the pending-input state is built so it could be reused, but no stepper uses it in this phase.

This phase carries a UI hint: a UI design contract (`11-UI-SPEC.md`, via `/gsd-ui-phase 11`) is expected before planning, for the three pages, the waiting prompt, the refusals and the previews.

</domain>

<decisions>
## Implementation Decisions

### Where the editor works (EDIT-01, EDIT-02)
- **D-01:** Set and Remove work in the private menu only (owner's choice, 2026-11-04). In a menu opened in the group those buttons show a short note with an "Open in private" link (the existing `/start settings_<chat id>` deep link, which carries the owner gate). Preview, Full preview and the on/off switches work in both places. No typed reply is ever awaited inside a group.
- **D-02:** The pages are new settings pages registered through `registerSettingsPage` in their own files (`Welcome`, `Goodbye`, `Rules`), so `settings_pages.go` is not edited. Every page press goes through the Phase 10 pipeline (`settingsPress.verify`, owner gate included).

### The wait for the owner's next message (EDIT-02)
- **D-03:** After Set, the owner's next private message becomes that part. The wait is one slot per person: pressing Set on another part replaces the first wait and says so (owner's choice). Cancel, `/cancel` and a 10-minute expiry end it and leave the message unchanged.
- **D-04:** The wait lives in Redis (key family `fuku:settings:input:<user>`, 10-minute TTL, value names the group, page and part), so whichever replica receives the message sees it. Without Redis the Set buttons answer that the editor is not available right now; everything else still works. The reply is read by a private-chat message handler below group 12 (group number decided in research) that returns `ext.ContinueGroups` when the sender has no wait, so no other handler is affected.
- **D-05:** The typed reply is judged like a press: a live `getChatMember` of the sender in the group named by the wait, the Phase 10 owner gate (`settingsGate`), and only then the stored change. A wait whose owner lost access is ended with a short refusal. Service identities never start or answer a wait.

- **D-12:** A reply sent after the 10-minute wait has expired is not left silent (UI checker recommendation, adopted 2026-11-04; supersedes the UI-SPEC default A3 that let it pass through). Set also writes a short-lived marker (`fuku:settings:inputdone:<user>`, about 15 minutes) that records the last wait's group and part; when the typed-reply door finds a marker but no wait it answers with a plain reply that the wait ended, naming the part and saying to press Set again. Still no timer and no shutdown drain. A user with neither a wait nor a marker is passed through untouched, so ordinary private chat with the bot is unaffected.
- **D-13:** An album that is refused (for example sent where only text is accepted) gets one refusal, not one per item: the door remembers the media group id for about 30 seconds for refused albums as it does for accepted ones.

### What is accepted and how it is checked (EDIT-03, EDIT-04, EDIT-05, EDIT-11)
- **D-06:** Text keeps its formatting and placeholders (`{first}`, `{mention}` and the rest) and is stored exactly as `/setwelcome`, `/setgoodbye` and `/setrules` store it (entities converted the same way, the same stored form), so a saved message shows at the next real join, leave or `/rules`.
- **D-07:** Before saving, the bot does a dry run (owner's choice): it builds the message with the placeholders filled using the owner's own name and a worst-case-length name, and refuses it with the reason, leaving the wait open, if Telegram would reject it: text over 4096 characters (1024 as a caption), a bad button syntax or URL, or a media item the welcome cannot send. The refusal names the cause in the owner's language.
- **D-08:** Media accepts exactly one item of a kind the welcome already supports (photo, video, GIF, document, sticker, audio, voice). From an album one item is kept and the owner is told so. URL buttons use the bot's existing button syntax, no new syntax.

### Remove and Preview (EDIT-01, EDIT-06)
- **D-09:** Remove on the Welcome or Goodbye text goes back to the default text, like `/resetwelcome` and `/resetgoodbye`. Remove on the Rules text clears it, like `/clearrules`. Remove on a media item or on the buttons simply removes it (owner's choice: same as the commands).
- **D-10:** Preview shows one part. Full preview sends the whole message exactly as members will see it, through the same send path the group uses, filled with the owner's own name and the group's title. It is sent only to the owner in the private chat, never into the group.

### Switches on Welcome and Goodbye (EDIT-10)
- **D-11:** Welcome and Goodbye pages carry their on/off switch and "Delete last welcome/goodbye". Presses use one apply function per setting beside the existing command (`/welcome`, `/goodbye`, `/cleanwelcome`, `/cleangoodbye`), so the menu and the command store the same row and invalidate the same cache, as the project rule "one setting, one code path" requires. They send absolute target values, so a double tap is harmless.

### Claude's Discretion
- The handler group number of the private input consumer (below 12), the exact Redis key names, the wording of every prompt and refusal, the page layout and icons (settled in the UI contract), how Telegram entities are converted for storage (settled by a short spike against the `/setwelcome` code path first), and how an album is cut to one item.

</decisions>

<canonical_refs>
## Canonical References

- `.planning/ROADMAP.md` Phase 11 success criteria; `.planning/REQUIREMENTS.md` EDIT-01 to EDIT-06, EDIT-10, EDIT-11.
- `.planning/research/SUMMARY.md` and `PITFALLS.md` (pitfalls 10 to 12: a wait that is per replica or single-slot, UTF-16 entity offsets, length limits after placeholders are filled).
- `.planning/phases/10-pm-settings-hub-and-owner-gate/10-CONTEXT.md`, `10-UI-SPEC.md` and the Phase 10 summaries: the owner gate, the press pipeline, the `settingsRight*` rights, `registerSettingsPage`, the two-tap card and the callback rules every new page follows.
- `AGENTS.md` sections Handlers, Callbacks, Permissions, Data and the settings rules.
</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `fuku/modules/greetings.go`: `welcome`, `setWelcome`, `resetWelcome`, `goodbye`, `setGoodbye`, `resetGoodbye`, `cleanWelcome`, `cleanGoodbye` (registered through `helpers.WrapCommand`); `fuku/db/greetings` repository.
- `fuku/modules/rules.go`: `setRules`, `clearRules`, `sendRules`, `privaterules`; `fuku/db/rules` repository.
- The settings menu: `settings_menu.go` (`settingsPress.verify`, rights), `settings_pages.go` (`registerSettingsPage`), `settings_card.go` (two-tap cards), `settings_gate.go` (`settingsGate`), `settings_hub.go`, `fuku/utils/callbackcodec`.
- Existing apply-function pattern: `applyCaptchaTimeout` in `captcha.go` and the antiflood `applyFlood*` functions.

### Established Patterns
- One apply function per setting beside its command; pages read through the same getter as the command; a read error is the cannot-open edit.
- Callback data at most 64 bytes with a worst-case test; every page has a length test in all 8 locales; a group menu never names the Staff Group.
- Handlers in groups below 12; watchers return `ext.ContinueGroups`; fire-and-forget goroutines recover from panics.

### Integration Points
- New pages join Main through `registerSettingsPage` (the Main grid already draws every registered page).
- The private-chat input handler must coexist with the private `/settings` list, the `/start` deep links and `/cancel`.

</code_context>

<specifics>
## Specific Ideas

- The owner's milestone answers still apply: the editor mirrors GroupHelp's idea of a message editor (text, one media, URL buttons, preview); "Media under text" and "Greet once" come in Phase 12.
- A short spike comes first (research flag): does a message sent in private store in the same form as `/setwelcome`, how an album is cut to one item, and how a preview goes to the private chat through the group's send path.

</specifics>

<deferred>
## Deferred Ideas

- Typing a number into a stepper through the same input state (the state is reusable; no stepper uses it here).
- Editing a message from a menu opened in the group (the owner chose private only).
- Posting editor changes to the log channel (MENU2-01).

</deferred>
