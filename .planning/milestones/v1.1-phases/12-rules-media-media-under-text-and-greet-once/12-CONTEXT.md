# Phase 12: Rules Media, Media Under Text and Greet Once - Context

**Gathered:** 2026-11-05
**Status:** Ready for planning

<domain>
## Phase Boundary

The Welcome, Goodbye and Rules editor pages built in Phase 11 gain three things: Rules can carry one media item and URL buttons (EDIT-08), a "Media under text" switch puts a photo, video or GIF below its text in a welcome, goodbye or rules message (EDIT-07), and Welcome gets a "Greet once" switch so a member is welcomed only the first time they ever join, also when the join reaches several replicas (EDIT-09). Everything saved before this phase keeps looking and behaving exactly as it did until someone changes it. Out of scope: the link-preview toggle, a Test button (Full preview from Phase 11 already shows the real message), "Choose topic", member requirements and every later phase.

</domain>

<decisions>
## Implementation Decisions

### Rules media and buttons (EDIT-08)
- **D-01:** Rules gain one media item (the kinds a welcome already supports: photo, video, GIF, document, sticker, audio, voice) and URL buttons, stored in new columns of the `rules` table, one for the file ID, one for the media type and one for the buttons as JSON. The Rules editor page gets the `m` media and `b` buttons parts as two more rows of its parts table (`settingsEditPageDef.Parts`, Phase 11), so the renderer, the Set, Remove and Cancel actions, the typed-reply door and the previews are reused and not rebuilt. The check, the dry run and the stored form are the Welcome's own for the same input (Phase 11 D-06 to D-08).
- **D-02:** `/rules` shows the media and buttons in the group and as private rules, through the one shared send path (`media.SendGreeting` and the `buildRulesText` builder), so Full preview shows exactly what members see. `/setrules`, `/clearrules` and `/privaterules` keep their commands; media and buttons are set only from the editor (a rules message with media has no command syntax).
- **D-03:** Rules with media are one message: the media with the text as its caption and the buttons under it. The caption limit is the same 1024 characters as a welcome (owner's choice, 2026-11-05: refuse, like the welcome). The editor refuses a rules text over 1024 while media is set, and refuses media while the text is over 1024, and says why; removing the media keeps a long text (up to the existing 3800). Welcome, Goodbye and Rules all follow this one rule.
- **D-04:** Remove on the Rules media or the buttons clears just that part, as on the Welcome page (Phase 11 D-09). `/clearrules` clears all three parts together (text, media and buttons), so the three Remove presses of the Rules page store exactly what `/clearrules` stores, as `/resetwelcome` does for a welcome.
- **D-05:** Rules saved before this phase have no media and no buttons: the new columns read as zero, `/rules` sends the same text-only message as today, byte for byte (success criterion 4).

### Media under text (EDIT-07)
- **D-06:** "Media under text" is a switch on the Welcome, Goodbye and Rules pages, stored in a new boolean column for each message (`welcome_media_below`, `goodbye_media_below` on `greetings`, one on `rules`), default off, with no `default:` tag on the Go model (the GORM zero-value trap), so zero is today's behaviour. On means Telegram's `show_caption_above_media`: the caption sits above the media, so the media appears below its text. It applies to the message everywhere it is sent: the join or leave, `/rules`, private rules and Full preview.
- **D-07:** The switch only works for a photo, video or GIF (owner's choice, 2026-11-05: hide it otherwise). It is drawn on the page only while the stored media is one of those three, so it never promises something Telegram will not do for a document, audio, voice or sticker; a stored "on" is ignored (not sent, not shown) when the media is later replaced by another kind and comes back if the media is replaced by an eligible kind again.
- **D-08:** A GIF is stored today as a document (no animation type exists: `ExtractMedia` returns `db.DOCUMENT` for an animation) and sent with `sendDocument`, which cannot place a caption. To keep the roadmap promise for GIFs without touching anything already saved, a GIF saved from this phase on (by the editor, `/setwelcome`, `/setgoodbye`, the same `ExtractMedia`) is stored as a new animation type and sent with `sendAnimation`; GIFs saved earlier stay documents, show exactly as before, and get no switch until the owner sets the GIF again. The spike "caption placement" (roadmap research flag) confirms `show_caption_above_media` for photo, video and animation on a real client and the 1024 limit before the send.
- **D-09:** The 1024 caption limit and the 4096 text limit (Phase 11 D-07) are checked with the filled placeholders as before; turning the switch on or off changes no length, so it needs no new dry run.

### Greet once (EDIT-09)
- **D-10:** "Greet once" is a switch on the Welcome page only, stored in a new boolean column `welcome_once` on `greetings` (default off: greet every join, as today). Its apply function sits beside `/welcome` and needs the same right as `/welcome on|off`; it has no command of its own, so it carries a drift test and an authority test instead of a parity test.
- **D-11:** The record is a new table `greeted_members` (`chat_id`, `user_id`, `created_at`, primary key on both) that is read only while the switch is on. A member is welcomed when `INSERT ... ON CONFLICT DO NOTHING` affects one row, so two replicas that see one join never both greet. The claim is made just before the send, and when Telegram definitively refuses the send (owner's choice, 2026-11-05) the row is deleted again so the member's next join can still be welcomed; an unknown outcome (a timeout, a lost answer) keeps the row, because a double welcome is worse than a missing one. Rows are kept for good, which is the meaning of "ever".
- **D-12:** The claim is made only on the path where the member really is welcomed: after the captcha is passed, after any requirement later phases add, and never for a joiner the lockdown guard banned or the captcha failed. The greet-once record never uses `claimRecentJoinProcessing`, which fails closed on a Redis error, and works without Redis.
- **D-13:** `greeted_members` is never part of backup, export, import or reset (like the staff and lockdown tables), is read fresh (no cache), and is added to the AutoMigrate lists of the test harnesses. The new `greetings` and `rules` columns travel in backup JSON through their models, and an old export decodes to zeros, which is the old behaviour.

### Compatibility, storage and the editor pages
- **D-14:** One migration with a timestamp greater than every existing one (`20261104120000` is the latest), one transaction, no top-level BEGIN or COMMIT, `fuku/db/migrations/runner.go` and `scripts/migrate_psql.sh` agreeing, models with `TableName()`, the three new boolean columns and the rules columns `NOT NULL DEFAULT` the zero value so a restored row stays valid.
- **D-15:** The Welcome page gets the Media under text and Greet once switches, Goodbye gets Media under text, Rules gets the two new parts and Media under text. The pages still read through the side-effect-free loaders (`greetings.LoadSettingsContext`, `rules.LoadRulesContext`) and must not create a row. The page row limits (a private page at most 8 rows, a group-menu page 9) and the 3800 UTF-16 and 64-byte limits in all 8 languages still hold, so the UI contract decides how the extra switches share rows. Every new string goes into all 8 locale files.
- **D-16:** Preview and Full preview (Phase 11 D-10) show media under text and Rules media and buttons through the same builders, always to the presser's own private chat.
- **D-17:** Writes go through new column-targeted writers and apply functions beside the commands (one setting, one code path); each write deletes the cache keys it affects.

### Claude's Discretion
- The names of the new columns, locale keys and settings controls; the exact GIF type value and how an old reader treats an unknown type; the row layout of the extra switches within the page limits; the order of the migration statements.

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets
- The editor from Phase 11: the parts table `settingsEditPageDef.Parts` and `renderSettingsEditor`, the Set, Remove and Cancel actions, the wait store and the typed-reply door (`settings_input_door.go`, group 1), the pure check `settingsEditCheck` (media and buttons verbs) and the previews `settings_edit_preview.go`.
- One send path: `renderGreeting` (join, leave, previews) and `buildRulesText` (`/rules`), `media.SendGreeting` / `media.Send` (`fuku/utils/media/sender.go`: separate `SendPhoto`, `SendVideo`, `SendDocument` cases, no animation case yet).
- Writers and loaders: `greetings.Set*Only` one-part writers, `greetings.LoadSettingsContext`, `rules.LoadRulesContext`, `rules.SetChatRules`; shared helpers `content.ExtractMedia`, `content.ValidButtonURL`, `formatting.FillPlaceholders`.
- The conditional-insert pattern for exactly-once across replicas: `lockdown.BeginAlert`, `staff` conditional writes.

### Established Patterns
- Apply functions beside the commands (`applyWelcomeEnabled`, `applyRulesText`), returning a typed result read back from the database.
- Models carry no `default:` tag on booleans; migrations are append-only; tables outside backup are listed in AGENTS.md; harness AutoMigrate lists are updated with each model.
- Locale keys in all 8 files with a limits row in `fuku/i18n/settings_locale_test.go`.

### Integration Points
- `SendWelcomeMessage` (`greetings.go`) and the leave handler (goodbye) for media under text and greet once; the `/rules` and private-rules handlers in `rules.go`; `fuku/db/backup` (types and round-trip tests); `migrations/`, `fuku/db/migrations/runner.go`, `scripts/migrate_psql.sh`; the join paths that call `SendWelcomeMessage` (member update, captcha pass, join-request approval).

</code_context>

<specifics>
## Specific Ideas

- The owner's milestone answers: "Welcome once, Delete last welcome, Media under text" were the editor extras to build; "Delete last welcome" shipped in Phase 11.
- The three choices of this discussion (2026-11-05): give the greet-once record back on a definite send failure, refuse a rules text over 1024 once media is set, and hide the switch for media that cannot take it.

</specifics>

<deferred>
## Deferred Ideas

- The link-preview toggle (`welcome_preview` in the research) and a Test button: not in EDIT-07 to EDIT-09; Full preview already shows the real message.
- "Choose topic" for the welcome: ruled out of v1.1 by the owner.
- The `/rules` double escaping of `'`, `&` and `<` (pre-existing, kept for parity) and the Goodbye On by default for a group with no greetings row: left for the owner, listed in the Phase 11 spike follow-ups.
- Sending long rules text and media as two messages: refused for now in favour of one message (D-03).

</deferred>
