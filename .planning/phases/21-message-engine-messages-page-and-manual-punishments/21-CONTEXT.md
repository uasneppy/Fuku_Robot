# Phase 21: Message Engine, Messages Page and Manual Punishments - Context

**Gathered:** 2026-11-18
**Status:** Ready for planning. The scope and the decisions marked "owner" were agreed with the owner in conversation on 2026-11-18. The decisions marked "default" are the recommended defaults; the owner asked for implementation to start without further questions, so they are provisional and reversible.

<domain>
## Phase Boundary

Phase 21 builds the foundation of milestone v1.2 and its first slice the owner can try:

- **Storage and renderer** (MSG-01 to MSG-04): one custom message per group, event and destination, and one renderer every send site calls, which falls back to today's default text.
- **Messages page** (MSG-05 to MSG-08): a private-only page in `/settings` that lists events by family and opens the v1.1 message editor for an event.
- **Manual events** (EVT-01): ban, timed ban, kick, mute, timed mute, unban, unmute.
- **Unchanged defaults** (MSG-11) and **deletion timers** (MSG-09).

Out of scope here: PM copies (Phase 22), every automatic and lockdown event (Phases 23 to 25), tuning (26 to 29), backup of custom messages and the Backup page (Phase 30). Nothing is set at Staff Group level and nothing is inherited between groups.

</domain>

<decisions>
## Implementation Decisions

### Scope rules (owner)
- **D-01 (owner):** Everything is set group by group, in that group's settings in the bot's private chat. No Staff Group defaults, no inheritance, no command that sets a custom message in this phase (the page is the only door).
- **D-02 (owner):** A custom text is one text. It replaces all 8 translations for that group and event. No row means today's default text in the group's language.
- **D-03 (owner):** A message never delays, undoes or skips a punishment. The punishment happens first; the message is best effort.

### Storage (default)
- **D-04:** One new table (suggested name `chat_event_messages`) with one row per `(chat_id, event, dest)`: text, `file_id`, `media_type`, buttons (jsonb, as the greetings and rules rows store them), a media-under-text flag only if the editor reuse needs it, and the Silent switch. `dest` is `group` now; Phase 22 adds `pm` rows without changing the key. Event and dest are validated in Go (a table of known events), never by a CHECK, so later phases add events with no migration. No foreign key. Timestamped migration, one transaction, both appliers agree.
- **D-05:** Read through `cache.GetFromCacheOrLoad` as one struct per chat (all of a chat's rows under one key), every write drops the key, and the key prefix goes in `skipLocal`: a change, and above all a Remove or a Silent switch, must act at the very next command on every replica. A failed read is an error to the caller, and the send site then uses the default text (never a guess, never a skipped punishment). The page reads fresh and creates nothing.
- **D-06:** The model carries no `default:` tag on its booleans. Writes are column-targeted upserts so a Go `false` and an empty string are written.
- **D-07:** The table is not part of export, import or reset in this phase (Phase 30 adds it). A test proves it stays out, like the other v1.1 tables.

### Renderer and sending (default)
- **D-08:** One function builds an event message from (chat, event, dest, the target user, the event's values). With no row it returns exactly what the send site sends today, byte for byte. With a row it fills the Welcome placeholders through the existing `formatting.FormattingReplacer` for the punished person and the group, then the event placeholders `{reason}`, `{admin}`, `{duration}`, `{until}`, `{action}`, `{count}`, `{limit}`, `{window}`.
- **D-09:** A placeholder with no value for the event renders as nothing, never raw. `{reason}` with no reason is empty. `{duration}` and `{until}` are empty for a permanent punishment. `{until}` is formatted with `formatChatTime` in the group's zone (`chatClock`).
- **D-10:** Every user-supplied value (names, the reason) is HTML-escaped before it is spliced. The owner's own text keeps its formatting exactly as the Welcome editor stores it.
- **D-11:** With a custom text the default's separate "Reason:" line is not appended. The owner places `{reason}` where they want it. The editor's hint says so.
- **D-12:** `{admin}` is the acting admin's escaped name, and "an admin" (a new translated key, group language) for an anonymous admin. The re-entered command after the proof tap carries the tapper as sender, so the anonymous mark has to be carried from the original update to the send site. It never names the Staff Group.
- **D-13:** The custom message is sent as today's reply is (a reply to the command, same reply parameters), through the greeting send path for media and buttons. If Telegram definitely refuses it (a 4xx other than 429, classified by `isTelegramClientError`), the default text is sent once instead and one Warn line is logged with the chat ID and event only. A 429, a 5xx or a transport error behaves as today's send does. No custom text, button or file ID is ever logged.
- **D-14:** Silent means nothing is posted in the group for that event. The silent command variants (`/sban` and the like) and the delete-first variants keep their current behaviour and are not events of their own: `/dban` uses the ban event, and a silent variant stays silent whatever is stored.
- **D-15:** Whatever is sent, default or custom, is queued for deletion by the same `scheduleBotMessage(sent, deletion.KindBotManual...)` call as today (MSG-09).

### Events (default)
- **D-16:** Seven event keys: `ban`, `tban`, `kick`, `mute`, `tmute`, `unban`, `unmute`. `/ban` with a duration is `tban`; `/mute` with a duration is `tmute`. All success replies go through `moderationCtx.replySuccess` today (five sites in `bans.go`, three in `mute.go`); that is the hook point. Validation and error replies, `/kickme`, `/banme`, the restrict cards and Staff Group actions are untouched.
- **D-17:** Characterization tests of every one of those success replies (request bytes, reply parameters, keyboard, and the deletion row) are recorded and committed alone BEFORE any send site changes, and never re-recorded (MSG-11).

### Messages page (default)
- **D-18:** A new page registered from its own file through `registerSettingsPage`, `PrivateOnly` (a group menu shows the framework's "open in private" note and runs no page code). Pick a free page key and `Order` so it sits near Welcome, Goodbye and Rules; state the Main grid's new shape and update the three shape constants.
- **D-19:** The page is table-driven by family (manual, automatic, lockdown, requirements) and by event, in the style of the Deletion page's section and row tables. Only the manual family has rows now; a family with no rows is not drawn. Each event line shows one state word: Default, Custom or Silent. Main shows a count of customised events or "Default".
- **D-20:** An event screen reuses the v1.1 message editor: parts text, media and buttons with Set and Remove, Preview of each part and a Full preview through the real renderer with sample values, plus the Silent switch. How an event and a part fit the wait hash (`p` one page key, `t` one part) and the 64-byte callbacks is the planner's main design question; `settingsTypedPages` and the list engine's part table are the existing hooks. No new handler group.
- **D-21:** The editor's limits apply: 4096 characters, 1024 with media, http or https buttons only, at most the keyboard limits. The check measures the text with the longest plausible placeholder values is NOT attempted; the D-13 fallback covers a text that only overflows once filled.
- **D-22:** Rights: opening and viewing need `settingsRightAdmin` (the live check and the owner gate, as every page). Set, Remove and the Silent switch need `settingsRightChangeInfo`, judged live on every press and on every typed message.
- **D-23:** There is no command to compare with, so each control carries a drift test and an authority test in place of a parity test, plus the worst-case callback test (64 bytes) and the length test in all 8 languages. The typed reply joins `TestSettingsGateAllDoors` if it adds a door.

### Repo rules that bind
- **D-24:** New locale keys in all 8 files, with rows in the limits tables. Ukrainian and English follow `locale-style.md`. HTML only.
- **D-25:** AGENTS.md is updated in the same commits that add a rule (the table, the cache key, the page, its codes and rights, the renderer). `make generate-docs` if a help text changes.
- **D-26:** `fuku/modules` is near its CI time limit. Keep each new test light: worst cases, not cross products. Fresh chat and user IDs, cleanup in `t.Cleanup`, no `t.Skip`, no mock libraries.

### Claude's Discretion
Table and function names, the page key and codes, the exact screens and strings, how the editor is parameterised for events, and whether the anonymous mark travels in `ctx.Data` or another way.

</decisions>

<canonical_refs>
## Canonical References

- `AGENTS.md`: the settings Callbacks bullets (pages, the editor, the list engine, `PrivateOnly`, rights), "One setting, one code path", "Scheduling the bot's own messages", Data and Migrations rules, Testing.
- `fuku/modules/moderation.go` (`replySuccess`), `bans.go`, `mute.go`: the send sites.
- `fuku/modules/settings_edit*.go`, `settings_input*.go`, `settings_deletion.go`, `settings_list.go`: the editor, the door and the table-driven page patterns.
- `fuku/modules/greetings.go` (`renderGreeting`), `fuku/utils/formatting`, `fuku/utils/media`: placeholders and the send path.
- `fuku/modules/deletion_bot.go`: `scheduleBotMessage`.
- `.planning/ROADMAP.md` Phase 21 planning notes and success criteria; `.planning/REQUIREMENTS.md`.

</canonical_refs>

<deferred>
## Deferred Ideas

PM copies, automatic and lockdown events, tuning, backup: later phases of this milestone. A custom message per language (MSG-F1): future.

</deferred>

---
*Phase: 20-message-engine-messages-page-and-manual-punishments*
*Context gathered: 2026-11-18*
