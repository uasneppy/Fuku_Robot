# Phase 12: Rules Media, Media Under Text and Greet Once - Research

**Researched:** 2026-11-05
**Domain:** Telegram bot (Go 1.26, gotgbot v2 rc.36, GORM, Redis, PostgreSQL): extending the Phase 11 message editor with Rules media and buttons, a caption-above-media switch, and a persistent exactly-once "greet once" record
**Confidence:** HIGH for everything grounded in repo code and in scratch runs of this session; MEDIUM for the Telegram-side behaviours that need a live client (core.telegram.org is blocked by the session network policy, see Environment Availability and the Assumptions Log)

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

#### Rules media and buttons (EDIT-08)
- **D-01:** Rules gain one media item (the kinds a welcome already supports: photo, video, GIF, document, sticker, audio, voice) and URL buttons, stored in new columns of the `rules` table, one for the file ID, one for the media type and one for the buttons as JSON. The Rules editor page gets the `m` media and `b` buttons parts as two more rows of its parts table (`settingsEditPageDef.Parts`, Phase 11), so the renderer, the Set, Remove and Cancel actions, the typed-reply door and the previews are reused and not rebuilt. The check, the dry run and the stored form are the Welcome's own for the same input (Phase 11 D-06 to D-08).
- **D-02:** `/rules` shows the media and buttons in the group and as private rules, through the one shared send path (`media.SendGreeting` and the `buildRulesText` builder), so Full preview shows exactly what members see. `/setrules`, `/clearrules` and `/privaterules` keep their commands; media and buttons are set only from the editor (a rules message with media has no command syntax).
- **D-03:** Rules with media are one message: the media with the text as its caption and the buttons under it. The caption limit is the same 1024 characters as a welcome (owner's choice, 2026-11-05: refuse, like the welcome). The editor refuses a rules text over 1024 while media is set, and refuses media while the text is over 1024, and says why; removing the media keeps a long text (up to the existing 3800). Welcome, Goodbye and Rules all follow this one rule.
- **D-04:** Remove on the Rules media or the buttons clears just that part, as on the Welcome page (Phase 11 D-09). `/clearrules` clears all three parts together (text, media and buttons), so the three Remove presses of the Rules page store exactly what `/clearrules` stores, as `/resetwelcome` does for a welcome.
- **D-05:** Rules saved before this phase have no media and no buttons: the new columns read as zero, `/rules` sends the same text-only message as today, byte for byte (success criterion 4).

#### Media under text (EDIT-07)
- **D-06:** "Media under text" is a switch on the Welcome, Goodbye and Rules pages, stored in a new boolean column for each message (`welcome_media_below`, `goodbye_media_below` on `greetings`, one on `rules`), default off, with no `default:` tag on the Go model (the GORM zero-value trap), so zero is today's behaviour. On means Telegram's `show_caption_above_media`: the caption sits above the media, so the media appears below its text. It applies to the message everywhere it is sent: the join or leave, `/rules`, private rules and Full preview.
- **D-07:** The switch only works for a photo, video or GIF (owner's choice, 2026-11-05: hide it otherwise). It is drawn on the page only while the stored media is one of those three, so it never promises something Telegram will not do for a document, audio, voice or sticker; a stored "on" is ignored (not sent, not shown) when the media is later replaced by another kind and comes back if the media is replaced by an eligible kind again.
- **D-08:** A GIF is stored today as a document (no animation type exists: `ExtractMedia` returns `db.DOCUMENT` for an animation) and sent with `sendDocument`, which cannot place a caption. To keep the roadmap promise for GIFs without touching anything already saved, a GIF saved from this phase on (by the editor, `/setwelcome`, `/setgoodbye`, the same `ExtractMedia`) is stored as a new animation type and sent with `sendAnimation`; GIFs saved earlier stay documents, show exactly as before, and get no switch until the owner sets the GIF again. The spike "caption placement" (roadmap research flag) confirms `show_caption_above_media` for photo, video and animation on a real client and the 1024 limit before the send.
- **D-09:** The 1024 caption limit and the 4096 text limit (Phase 11 D-07) are checked with the filled placeholders as before; turning the switch on or off changes no length, so it needs no new dry run.

#### Greet once (EDIT-09)
- **D-10:** "Greet once" is a switch on the Welcome page only, stored in a new boolean column `welcome_once` on `greetings` (default off: greet every join, as today). Its apply function sits beside `/welcome` and needs the same right as `/welcome on|off`; it has no command of its own, so it carries a drift test and an authority test instead of a parity test.
- **D-11:** The record is a new table `greeted_members` (`chat_id`, `user_id`, `created_at`, primary key on both) that is read only while the switch is on. A member is welcomed when `INSERT ... ON CONFLICT DO NOTHING` affects one row, so two replicas that see one join never both greet. The claim is made just before the send, and when Telegram definitively refuses the send (owner's choice, 2026-11-05) the row is deleted again so the member's next join can still be welcomed; an unknown outcome (a timeout, a lost answer) keeps the row, because a double welcome is worse than a missing one. Rows are kept for good, which is the meaning of "ever".
- **D-12:** The claim is made only on the path where the member really is welcomed: after the captcha is passed, after any requirement later phases add, and never for a joiner the lockdown guard banned or the captcha failed. The greet-once record never uses `claimRecentJoinProcessing`, which fails closed on a Redis error, and works without Redis.
- **D-13:** `greeted_members` is never part of backup, export, import or reset (like the staff and lockdown tables), is read fresh (no cache), and is added to the AutoMigrate lists of the test harnesses. The new `greetings` and `rules` columns travel in backup JSON through their models, and an old export decodes to zeros, which is the old behaviour.

#### Compatibility, storage and the editor pages
- **D-14:** One migration with a timestamp greater than every existing one (`20261104120000` is the latest), one transaction, no top-level BEGIN or COMMIT, `fuku/db/migrations/runner.go` and `scripts/migrate_psql.sh` agreeing, models with `TableName()`, the three new boolean columns and the rules columns `NOT NULL DEFAULT` the zero value so a restored row stays valid.
- **D-15:** The Welcome page gets the Media under text and Greet once switches, Goodbye gets Media under text, Rules gets the two new parts and Media under text. The pages still read through the side-effect-free loaders (`greetings.LoadSettingsContext`, `rules.LoadRulesContext`) and must not create a row. The page row limits (a private page at most 8 rows, a group-menu page 9) and the 3800 UTF-16 and 64-byte limits in all 8 languages still hold, so the UI contract decides how the extra switches share rows. Every new string goes into all 8 locale files.
- **D-16:** Preview and Full preview (Phase 11 D-10) show media under text and Rules media and buttons through the same builders, always to the presser's own private chat.
- **D-17:** Writes go through new column-targeted writers and apply functions beside the commands (one setting, one code path); each write deletes the cache keys it affects.

### Claude's Discretion
- The names of the new columns, locale keys and settings controls; the exact GIF type value and how an old reader treats an unknown type; the row layout of the extra switches within the page limits; the order of the migration statements.

### Deferred Ideas (OUT OF SCOPE)
- The link-preview toggle (`welcome_preview` in the research) and a Test button: not in EDIT-07 to EDIT-09; Full preview already shows the real message.
- "Choose topic" for the welcome: ruled out of v1.1 by the owner.
- The `/rules` double escaping of `'`, `&` and `<` (pre-existing, kept for parity) and the Goodbye On by default for a group with no greetings row: left for the owner, listed in the Phase 11 spike follow-ups.
- Sending long rules text and media as two messages: refused for now in favour of one message (D-03).
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| EDIT-07 | "Media under text" shows a photo, video or GIF below its text instead of above it. | S-01 (the new animation type, `ShowCaptionAboveMedia` on exactly three gotgbot opts structs, one shared eligibility predicate, one new struct-based send function), S-05 (switch grid, control `u`, stale press), S-06 (tests that change), Pitfalls 1, 2, 7 |
| EDIT-08 | Rules gain media and URL buttons, and `/rules` (in the group and as private rules) shows them. | S-03 (one builder with two heading forms, legacy text-only branch kept literal, deep link, caption rule shared with the Welcome, language accessor), S-04 (rules columns, writers, backup), S-05 (Rules parts, door `apply`), Pitfalls 3, 4, 5, 6 |
| EDIT-09 | Welcome has "Greet once": a member is greeted only the first time they ever join that group, also when the join reaches several replicas. | S-02 (the one choke point `SendWelcomeMessage`, claim placement, refusal classifier, table, retention, Pitfall 22 decision), S-04 (migration, PostgreSQL guarantee), Pitfalls 8, 9, 10 |
</phase_requirements>

## Project Constraints (from CLAUDE.md / AGENTS.md)

Directives the plans must keep (substance of `.claude/CLAUDE.md` and `AGENTS.md`, treated as locked):

- Stack stays Go 1.26.0, gotgbot v2, GORM (SQLite in tests), Redis, PostgreSQL; **no new dependency**.
- Migrations are append-only, timestamped, one transaction per file, no top-level `BEGIN`/`COMMIT`, no `CREATE INDEX CONCURRENTLY`; both appliers swallow `duplicate_object` on a bare `ADD CONSTRAINT`, so constraints go inline in `CREATE TABLE`. Never `gorm.AutoMigrate` in production code.
- Every write `cache.DeleteCache`s the keys it affects. Greetings and rules reads are **uncached** (S-02), so the existing `greetings:<chat>` delete is a documented no-op for reads; `greeted_members` has no cache at all.
- Commands go through `helpers.WrapCommand`; this phase adds **no command and no handler group**. Locale keys go in all 8 files (`en es fr hi id pt ru uk`; `locales/config.yml` is not a language).
- Fire-and-forget goroutines start with `defer error_handling.RecoverFromPanic(...)`; this phase adds none (the claim and release run inline in the join path).
- Tests use real fixtures (miniredis, hand-written `gotgbot.BotClient` fakes), no mock libraries, always `-tags testtools`, full run only through `make test`; assert observable behaviour (message sent, row persisted, gate enforced), never literals, source substrings or test-double internals.
- Callback data only through `callbackcodec` (64-byte cap); never `strings.Split` it.
- The per-group live admin check is never skipped; the Staff Group is never named on any settings screen (`TestSettingsStaffPrivacy`).
- Never log message text, captions, button names, links or file IDs; log IDs, part and outcome only.
- Update `AGENTS.md` in the same commit as any change that invalidates a rule (list in S-07).

## Summary

The phase is mostly assembly on top of Phase 11, but reading the code found **thirteen facts that differ from what the CONTEXT, the UI-SPEC or the milestone research assume**. The plans must be built on these (each is detailed in the finding named in brackets):

1. `extractMediaFromReply` is shared by `/setwelcome`, `/setgoodbye`, **`/save` and `/filter`** (`ExtractNoteAndFilter`) and `pins.go` has its own copy. Changing it to return an animation would change notes and filters too. Recommendation: split it, so only the greeting paths (`ExtractWelcome` and the exported `ExtractMedia` the editor calls) return the new type [S-01].
2. `/rules` today sends through `msg.Reply` with **link previews on**, while `media.SendGreeting` always sends text with previews **off**. Routing `/rules` through `SendGreeting` would change every old rules message. The text-only branch of the shared Rules send must stay the literal legacy call; the media branch uses `media.Send` [S-03].
3. The private-rules deep link uses a **different heading key** (`rules_for_chat`) from `/rules` (`rules_for_chat_header`); both must keep their wording for old rows. One builder, two heading forms [S-03].
4. The existing generic locale test (`TestSettingsLocaleKeys`) counts a template **as written**, so the five switch-button keys need limit rows of 26 (name 18 + `{state}` 7 + space), and the binding 20-code-point ceiling is a new rendered check in `TestSettingsLabelLengths`. The UI-SPEC's "row counts `{state}` as 1" cannot be implemented literally in the current test [S-05].
5. The core `tg` action does **not** refuse an unknown `t` or a bad `v` before I/O (only page actions have `Validate`); the UI-SPEC check 3 needs one small additive hook in `settingsCheckFields` plus an optional `settingsControl.Validate` [S-05].
6. The UI-SPEC contradicts itself on the Rules refusal key for a text over 4096 with no media (table row 3 says "unchanged", check 8 says Rules never uses `settings_edit_refuse_too_long_filled`). Resolved: keep the old key for the no-media case; the new key's English says "the limit with media" and is only correct with media [S-05, Open Question 11].
7. The `{rules}` button of a welcome is gated on the rules **text** (`rulesDb.Rules == ""` at `formatting.go:319`), so a media-only Rules (UI-SPEC A1: set means text, media or buttons) would lose its `{rules}` button. One shared `HasContent` test on the model fixes it [S-03].
8. `settingsEditButtonRows(buttons, in.State.Text)` counts a `{rules}` token in the stored text as an extra button; for a Rules page the stored text has no placeholder meaning, so the Rules path must pass `""` [S-05].
9. Greetings reads are **uncached** (`db.GetRecord`, no cache key has a reader), so Pitfall 22's premise "read from `GetGreetingSettings` (cached: 10 s local layer plus Redis)" is wrong; the clean-old race is a plain read-modify-write [S-02].
10. `rules.LoadRulesContext` returns only the text; the editor needs the whole row. Add `LoadRulesSettingsContext` and keep the old function as a wrapper to avoid churn in about ten call sites and tests [S-03].
11. `SendGreeting` has a fixed 7-argument signature and nine call sites; add a struct-based `SendGreetingMessage` and keep `SendGreeting` as a wrapper [S-01].
12. `make test-postgres-integrity` does not list `./fuku/db/greetings` at all; the greet-once concurrency test and the migration-defaults test must be added to its `-run` list and package list [S-04].
13. A rules message with media and a caption over 1024 has no command-path guard (`/setrules` stores any text, and a backup import can bring both), so `/rules` would fail silently at send. Recommended small hardening: degrade once to the text-only message on a definite 4xx, in the member-facing path only [S-03].

The roadmap's "caption placement" spike splits cleanly: option names, omit-when-false serialisation, which opts structs carry the field, and the one-message caption arithmetic are settled by code reading and compile probes (HIGH); real rendering, old-client behaviour, the `sendAnimation` file_id round trip and the live 400 for an over-long caption need a live client and become backstops for the owner's `12-SPIKE.md`.

**Primary recommendation:** add an `ANIMATION = 9` media type and one struct-based send function whose `CaptionAbove` flag is honoured only by `sendPhoto`, `sendVideo` and `sendAnimation`; put the greet-once claim (`INSERT ... ON CONFLICT DO NOTHING`, give-back on a definite refusal) inside `SendWelcomeMessage`, the single place every real welcome passes; build Rules as one message builder with two heading forms and a literal legacy text-only branch; extend the editor tables (parts, a two-column switch grid with controls `u` and `o`) and prove it with a characterization test written against today's `/rules` before any refactor.

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Media type constant and "caption above" eligibility | Library (`fuku/db` constant, `fuku/utils/media`) | Bot handlers | One pure predicate on the type, read by the sender, the page, the stale-press check; no module re-implements the list |
| Sending (animation, caption above, buttons) | `fuku/utils/media` | Telegram API | One send function; the flag is structurally ignored by methods that have no such field |
| Greet-once claim and release | Database (`greeted_members`, PK + conditional insert) | Bot handlers (`SendWelcomeMessage`) | Must hold across replicas and survive Redis loss; the database is the only shared durable store the join path already trusts |
| Rules message assembly (heading, caption, keyboard, media, language) | Bot handlers (`rules.go`) | `fuku/db/lang` (group language by chat ID), `fuku/db/rules` | Same builder for `/rules`, private rules and Full preview so they cannot drift |
| Rules and greetings storage | Database repos (`fuku/db/rules`, `fuku/db/greetings`) | Backup module | Column-targeted writers; new columns travel in backup JSON through the models |
| Editor pages, switch grid, controls `u`/`o`, parts | Bot handlers (settings pipeline) | i18n | Table-driven: a part is a table row, a switch is a cell |
| Caption/length checks for Rules media and text | Pure Go (`settings_edit_check.go`) | Telegram (backstop at send) | Shared predicates with the Welcome, only the measure differs |
| Groups' language for the Rules Full preview | `fuku/db/lang` cached read | — | The accessor by chat ID is the same cached read `/rules` reaches |

## Standard Stack

### Core (all existing, no new dependency)
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `github.com/PaulSonOfLars/gotgbot/v2` | `v2.0.0-rc.36.0.20260919140833-240296efadb4` [VERIFIED: go.mod:7] | `SendAnimation`, `ShowCaptionAboveMedia`, `Message.Animation`, `TelegramError.Code` | The repo's Telegram client |
| `gorm.io/gorm` + `gorm.io/driver/sqlite` / `postgres` | v1.31.2 / v1.6.0 / v1.6.3 [CITED: CLAUDE.md Technology Stack] | `clause.OnConflict{DoNothing: true}` conditional insert, column-targeted `Updates(map)`, `clause.Locking` (the sqlite dialector no-ops it, S-02) | Existing repository pattern (`lockdown.RecordJoin`, `Start`) |
| `github.com/alicebob/miniredis/v2` | v2.39.0 [CITED: CLAUDE.md] | Redis fixtures for the editor wait tests (greet-once itself needs no Redis) | Repo standard |

### Supporting
| Library | Purpose | When to Use |
|---------|---------|-------------|
| `fuku/utils/callbackcodec` via `encodeCallbackData` | New buttons (`t=u`, `t=o`, Rules `t=m`/`t=b`) | Every settings button |
| `fuku/i18n` (`settingsText`, `ctxTr`) | All strings | Every message |
| `fuku/utils/errors` `Wrapf` | Wraps with `Unwrap`, so `errors.As(err, &*gotgbot.TelegramError)` still works through `media.Send`'s wrapper [VERIFIED: errors.go:25-27 `func (e *WrappedError) Unwrap() error`] | The refusal classifier |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| New `ANIMATION` type | Keep `DOCUMENT` and add `media_below` only for photo/video | Fails D-08: a GIF could never get the switch |
| Claim inside `SendWelcomeMessage` | Claim in `ProcessSingleJoin`'s `Welcome` dep | `finishCaptchaPass` calls `SendWelcomeMessage` directly (`captcha.go:1742`), so a claim in the dep would miss the captcha-pass path |
| `INSERT ... ON CONFLICT DO NOTHING` | Redis `SETNX` | Redis can be flushed or absent; D-11/D-12 demand a durable, Redis-free record |
| Changing `extractMediaFromReply` | Split into a greeting variant | Changing the shared function would alter `/save` and `/filter` GIFs for no benefit (S-01) |

**Installation:** none. **Version verification:** versions are the pinned ones in `go.mod`; nothing is installed.

## Package Legitimacy Audit

No external package is installed or added; `go.mod` and `go.sum` stay untouched. The `package-legitimacy` seam was not run because there is nothing to check.

| Package | Registry | Age | Downloads | Source Repo | Verdict | Disposition |
|---------|----------|-----|-----------|-------------|---------|-------------|
| (none) | - | - | - | - | - | - |

**Packages removed due to [SLOP] verdict:** none
**Packages flagged as suspicious [SUS]:** none

## Settled Findings (what the planner builds on)

Evidence tags: `file:line` with the value quoted verbatim, or "scratch run" for experiments executed this session (scratch files were deleted again; `git status` was clean after each).

### S-01. Media types and the send path

**Constants** [VERIFIED: fuku/db/db.go:34-41]: `TEXT       int = 1`, `STICKER    int = 2`, `DOCUMENT   int = 3`, `PHOTO      int = 4`, `AUDIO      int = 5`, `VOICE      int = 6`, `VIDEO      int = 7`, `VIDEO_NOTE int = 8`. No `CHECK` constrains the stored type: the columns are `"welcome_type" bigint` / `"goodbye_type" bigint` (initial migration, lines 167 and 174) and `"msg_type" bigint` / `"msgtype" bigint` for notes and filters [VERIFIED: migrations/20250805200527_initial_migration.sql:149,167,174,198]. **Recommended new value: `ANIMATION int = 9`** (next free integer, nothing in the database or code refuses it).

**Sender** [VERIFIED: fuku/utils/media/sender.go:82-102]: `Send` switches on `content.MsgType` with `case db.TEXT, 0:` (so a stored type of 0 sends as text, which is what makes "no media type" = 0 safe for Rules), one case per type, and `default: log.Warnf("[Media] Unknown message type %d, falling back to text", content.MsgType)` then `sendText`. There is no animation case; a replica on older code that meets type 9 sends the text only, without the media (accepted rolling-deploy caveat, like the others in AGENTS.md). Every per-type function is the same shape (`sendPhoto` etc., lines 155-225): empty `FileID` falls back to text, `Caption: content.Text`, `ParseMode`, `ReplyMarkup: opts.Keyboard`. `sendSticker` and `sendVideoNote` send no caption. `SendGreeting` (line 329) is `func SendGreeting(b *gotgbot.Bot, chatID int64, text, fileID string, msgType int, keyboard *gotgbot.InlineKeyboardMarkup, threadID int64)` with `WebPreview: false`.

**Where a GIF becomes a document** [VERIFIED: fuku/utils/content/content.go:124-125,134-135]: `} else if replyMsg.Document != nil {` / `return replyMsg.Document.FileId, db.DOCUMENT` comes before `} else if replyMsg.Animation != nil {` / `return replyMsg.Animation.FileId, db.DOCUMENT`. gotgbot documents why `Document` hits first: `Animation *Animation` is commented "For backward compatibility, when this field is set, the document field will also be set." [VERIFIED: gotgbot gen_types.go:8026]. So **the new rule must test `Animation` before `Document`**; otherwise every GIF keeps matching the document branch.

**Who calls the media extraction** [VERIFIED: grep of `extractMediaFromReply|ExtractMedia` over `fuku`]: `extractMediaFromReply` is called by `ExtractNoteAndFilter` (content.go:105, `/save` and `/filter`) and `ExtractWelcome` (content.go:174); the exported `ExtractMedia` (content.go:234-236, `return extractMediaFromReply(msg)`) is called only by `settingsEditCheckMedia` (settings_edit_check.go:266). `pins.go:595-620` has its own copy (`} else if msg.ReplyToMessage.Animation != nil {` ... `dataType = db.DOCUMENT`). `/setwelcome` and `/setgoodbye` reach it through `ExtractWelcome` (content.go:142-181).

**Decision (S-01a): split the function.** Keep `extractMediaFromReply` exactly as is (notes, filters, pins stay `DOCUMENT`: they have no Media-under-text switch and a new type there would only widen the blast radius into backup/import and the `message_type_*` captcha strings). Add `extractGreetingMedia(msg)` = the same chain but with `Animation` tested before `Document` and returning `db.ANIMATION`, and make **both** `ExtractMedia` and `ExtractWelcome` call it. Result: a GIF set through `/setwelcome` by reply, `/setgoodbye` by reply and the editor is an animation through one function; a GIF saved as a note, filter or pin stays a document. A GIF forwarded as a *file* (document only, no animation) stays `DOCUMENT` and gets no switch, as D-07/D-08 say.

**Other consumers of the media type and what they do with 9** [VERIFIED: grep of `db\.(DOCUMENT|PHOTO|...)` over `fuku`, non-test]: `settings_edit.go:219-236` (`settingsEditMediaKey`) returns `""` for an unknown type, and `settingsEditPartIsSet` (line 248: `st.FileID != "" && settingsEditMediaKey(st.MediaType) != ""`) would then show an animation as "Not set": **add `case db.ANIMATION: return "settings_edit_media_animation"`**. `settings_edit_check.go:267` (`dataType == -1 || dataType == db.VIDEO_NOTE`) needs nothing; `content.TextLengthRefusal` treats every non-TEXT type as a 1024 caption [VERIFIED: content.go:211 `case dataType != db.TEXT && textRuneCount > 1024:`], so animation is covered. `captcha.go:101-113` (`messageTypeToString`) and `captcha.go:2188-2213` classify *incoming* held messages with `Document` first and never produce 9; `pins.go` never produces 9. Backup writes only defaults (`backup.go:1010,1015` `db.TEXT`) and round-trips the stored integer untouched. So the complete list of code that must learn type 9 is: the sender, `settingsEditMediaKey`, the extraction, and the new eligibility predicate.

**gotgbot option names** (pinned version, struct bodies read this session). `ShowCaptionAboveMedia bool` exists with the doc comment "Pass True if the caption must be shown above the message media" on **`SendAnimationOpts`** [VERIFIED: gen_methods.go:4824], **`SendPhotoOpts`** [VERIFIED: gen_methods.go:6067] and **`SendVideoOpts`** [VERIFIED: gen_methods.go:6584]. It is serialised with `addIfValueNotZero(v, "show_caption_above_media", opts.ShowCaptionAboveMedia, opts.ShowCaptionAboveMedia == false)` [VERIFIED: gen_methods.go:4872, 6111, 6636; `addIfValueNotZero` skips a zero value, custom_helpers.go:169-175], so **a false flag adds no parameter**: the request of every message saved before this phase is byte-identical. `SendDocumentOpts`, `SendAudioOpts`, `SendVoiceOpts`, `SendStickerOpts`, `SendVideoNoteOpts` and `SendMessageOpts` have **no** such field [VERIFIED: struct bodies counted with awk, 0 occurrences each; compile probe of this session: `vet: fuku/utils/media/zz_probe_test.go:5:34: unknown field ShowCaptionAboveMedia in struct literal of type gotgbot.SendDocumentOpts`, probe file deleted]. The incoming `Message` carries `ShowCaptionAboveMedia bool` too ("True, if the caption must be shown above the message media", gen_types.go:8053), so a live spike can assert the echo of the sent message. The Bot API added the parameter to `sendPhoto`, `sendVideo` and `sendAnimation` in Bot API 7.4 (May 2024) [CITED: hackage.haskell.org/package/telegram-bot-api-7.4 changelog, third-party mirror of the official changelog; core.telegram.org itself is unreachable]. The caption limit is "0-1024 characters after entities parsing" for `SendPhotoOpts.Caption` and `SendAnimationOpts.Caption` [VERIFIED: gen_methods.go doc comments, Caption fields above lines 4824 and 6067].

**Design of the send change (S-01b).**
- `db.ANIMATION int = 9` beside the others, and a pure `media.CaptionAboveEligible(msgType int) bool` = `msgType == db.PHOTO || db.VIDEO || db.ANIMATION`. The page (`settingsEditMediaBelowEligible(st)` = file ID set and the predicate), the stale-press check and the sender all call this one function (UI-SPEC "Eligibility").
- `media.Content` gains `CaptionAbove bool`; `sendPhoto`, `sendVideo` and the new `sendAnimation` set `ShowCaptionAboveMedia: content.CaptionAbove`; the other senders never read it, so a stored "on" with a document, audio, voice, sticker or video note is ignored by construction (D-07) and nothing needs an if at the call sites.
- New `media.GreetingMessage{Text, FileID, MsgType, CaptionAbove, Keyboard, ThreadID}` and `media.SendGreetingMessage(b, chatID, msg)`; `SendGreeting` stays as a wrapper with `CaptionAbove` false. Sites that must pass the flag: `greetings.go:411` and `:404` (`/welcome` and `/goodbye` display, flag from the settings read), `:815` (join), `:889` (leave), and the four preview sites in `settings_edit_preview.go:80-91` (only Full preview passes it; Preview media alone has no caption and passes false). `sender_test.go:368` keeps compiling untouched.
- `sendAnimation`: `b.SendAnimation(opts.ChatID, gotgbot.InputFileByID(content.FileID), &gotgbot.SendAnimationOpts{ReplyParameters, ParseMode, ReplyMarkup, Caption, ShowCaptionAboveMedia, ProtectContent, DisableNotification, MessageThreadId})` with the empty-file-ID fallback to text, as its siblings. `resolveSendResult(msg, err, chatID, "animation", sentAt)`.

**What the roadmap spike can and cannot settle by reading code.**

| Spike item | Settled by code reading | Needs a live client (owner's `12-SPIKE.md`) |
|------------|-------------------------|---------------------------------------------|
| Option name and which methods have it | yes: struct fields above, compile probe | no |
| False flag leaves old requests unchanged | yes: `addIfValueNotZero` | no |
| Documents, audio, voice, stickers, video notes cannot place a caption | yes: no field on their opts structs | no |
| Caption is the text plus the buttons stay under it | partly (the send passes `ReplyMarkup` and `Caption` to one call) | confirm the layout on Android, iOS, Desktop |
| Caption really renders above the media for photo, video, animation; echo `show_caption_above_media: true` on the sent message | no | yes |
| An old client ignores the flag (caption below, today's layout) | no | yes (A6) |
| `sendAnimation` with the `Animation.FileId` captured from an animation message works and renders as a looping GIF | no | yes (A7) |
| `document.file_id` equals `animation.file_id` for one GIF message | no (Phase 11 A10, LOW) | optional; irrelevant to behaviour because the new rule stores `Animation.FileId` |
| A 1025-unit caption gets a 400 from Telegram; the editor's refusal is exact at 1024 UTF-16 units | no (the limit text is gotgbot's doc comment; counting unit is Phase 11 A1) | yes (A3) |
| `show_caption_above_media` is accepted when the caption is empty | no | yes (affects only a rules/welcome with media and no text) |

### S-02. Greet once

**Every call path of `SendWelcomeMessage`** [VERIFIED: grep `SendWelcomeMessage` over `fuku`, non-test]: exactly two non-test callers. (1) `membershipDepsForJoin`'s `Welcome` closure (`greetings.go:924-930`: `return SendWelcomeMessage(bot, &ctxCopy, user.Id, user.FirstName)`), which `ProcessSingleJoin` calls only after `DecideJoin` returned the welcome branch or after `ErrChallengeDisabled` fell through (`membership.go:86-101`). That closure serves both the member update (`newMember`, `greetings.go:830-847` via `processSingleNewMember`) and the service message (`cleanService`, `greetings.go:946-992` via `ProcessJoins`). (2) `finishCaptchaPass` (`captcha.go:1742`: `if err := SendWelcomeMessage(bot, in.Ctx, in.UserID, in.FirstName); err != nil {`), run after `claimCaptchaPass` succeeded and unmuted (`captcha.go:1850-1866`, and the web verify at `captcha_web.go:312-324`). The join-request Accept button only calls `ApproveChatJoinRequest` (greetings.go joinRequestHandler, `b.ApproveChatJoinRequest(chat.Id, joinUser.Id, nil)`); the welcome then comes from the `chat_member` update through path (1). A joiner the lockdown guard bans never reaches either path: the group `-7` guard ends the update before any welcome, captcha challenge or attempt [CITED: AGENTS.md, Subsystem traps, "records the joiner row first and only then ends the update with `ext.EndGroups`, so no welcome, captcha challenge or captcha attempt follows"]; a joiner whose captcha failed never calls `finishCaptchaPass`.

**Decision (S-02a): the claim sits inside `SendWelcomeMessage`**, the single function both paths call, **after** the `ShouldWelcome` check and **before** `renderGreeting`/the send (`greetings.go:801-815`). Order: read settings (`greetings.GetGreetingSettings`, an uncached DB read, `greetings/repository.go:18-20` `db.GetRecord`) -> if `WelcomeSettings == nil` or `!ShouldWelcome`: return (no claim) -> if `WelcomeSettings.Once`: claim; a lost claim returns `nil` with no send and no clean-old step; a claim error is logged at Error and also returns without sending (fail closed: D-11, "a double welcome is worse than a missing one") -> render and send -> on error, if the claim was ours and the error is a definite refusal, release. Because the claim is read fresh from `greetPrefs` on every join, a switch flipped on one replica is seen by the next join on another.

**Behaviours the contract asks about.**
- Welcome switch off (`ShouldWelcome` false): no claim, no row. The member is not welcomed, so they are not recorded; if the owner later turns Welcome on, earlier joiners are welcomed once (same limit as the hint `settings_edit_hint_once`).
- `Once` off: the table is not read and not written; behaviour is today's.
- Bots that join: `ProcessSingleJoin` skips only the bot itself (`membership.go:72-74` `if user.Id == botID`), so another bot is welcomed today and would be claimed like any member. `SendWelcomeMessage` receives only an ID and a first name, so do not special-case bots.
- `clean_old` (delete last welcome): runs only after a successful send, so a lost claim or a refused send deletes nothing.
- A re-join after leaving: `leftMember` calls `clearRecentJoinProcessing` (a Redis/in-process dedupe key) but never touches `greeted_members`, so the second join is not welcomed.
- Chat migration (basic group to supergroup changes the chat ID): rows under the old ID are orphaned and members are welcomed again once. AGENTS.md already says "other per-chat settings are not migrated"; record it as accepted. Do not add a re-key in this phase.
- A service message that adds several members: each `(chat, user)` claims separately.

**Definite refusal versus unknown outcome.** The send error from `media.Send` is `errors.Wrapf(err, "failed to send %s to chat %d", ...)` or, for permission errors, the plain sentinel `ErrNoPermission` [VERIFIED: sender.go:28-40 `if helpers.IsPermissionError(errStr) {` ... `return zero, ErrNoPermission`; sender.go:42 `var ErrNoPermission = fmt.Errorf("bot lacks permission to send messages")`], and `Send` returns it too, without any request, when `cache.IsChatRestricted(opts.ChatID)` holds (sender.go:64-67). The code classifier already in the repo is `isTelegramClientError` [VERIFIED: settings_gate.go:256-262: `return tgErr.Code >= 400 && tgErr.Code <= 499 && tgErr.Code != 429`], which unwraps with `errors.As`. **Recommended wrapper** (new, in `greetings.go`): a send is "definitely not delivered" when `errors.Is(err, media.ErrNoPermission)` (nothing was sent or Telegram said no rights), or `isTelegramClientError(err)`, or the error is a `*gotgbot.TelegramError` with `Code == 429` (Telegram answered "too many requests" without processing). Everything else (a `url.Error`, a context deadline, a transport error, a 5xx, a nil/unreadable answer) is unknown and **keeps** the row. Never classify by wording (AGENTS.md "Callbacks"/`isTelegramClientError` rule). The 429 inclusion is the one deliberate extension of the reused classifier; if the owner prefers literal reuse, a 429 in a join flood then costs that member's welcome for good (see Open Question 5).

**Table design.** `greeted_members(chat_id BIGINT NOT NULL, user_id BIGINT NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), PRIMARY KEY (chat_id, user_id))`. No foreign key (staff, lockdown and hub-table precedent, and the claim must work for a chat the `chats` table does not know yet). Model `GreetedMember` with `TableName() "greeted_members"`, `ChatID int64 gorm:"column:chat_id;primaryKey;autoIncrement:false"`, `UserID` likewise, `CreatedAt`. Repository functions in `fuku/db/greetings`: `ClaimGreeting(chatID, userID) (claimed bool, err error)` = `db.DB.Clauses(clause.OnConflict{DoNothing: true}).Create(row)` with `RowsAffected == 1`, and `ReleaseGreeting(chatID, userID) error` = `Delete` by both keys (releasing an absent row is not an error). **Scratch run, this session** (SQLite, throwaway model deleted): 8 goroutines inserting the same `(chat_id, user_id)` through that exact call produced `winners=1` with the explicit `autoIncrement:false` tag and also without it, so the tag is belt and braces, not a requirement. The PostgreSQL behaviour (a concurrent insert of the same key waits for the other transaction, then `DO NOTHING` reports 0 rows) is documented PostgreSQL behaviour [CITED: postgresql.org/docs/current/sql-insert.html, "INSERT into tables that lack unique indexes will not be blocked ... tables with unique indexes might block"] and is proven in CI by a PostgreSQL-only test (S-04). Two replicas that lost/won: if the winner's send is then definitely refused it deletes the row, and the loser has already returned without a welcome, so that one join is unwelcomed while the member's next join is welcomed. That is the stated trade (D-11).

**Growth and retention.** One tiny row per welcomed `(chat, member)` while the switch is on, kept for good (D-11). Estimate: roughly 100 bytes per row including the primary-key index (tuple header plus 8+8+8 bytes plus index entry), so one million rows is on the order of 100 MB [ASSUMED]. The write rate equals the rate of real welcomes, which Telegram's send pacing already bounds; a raid cannot out-run it because lockdown-banned and captcha-failed joiners never reach the claim. No cleanup job; a privacy-driven purge is a future `DELETE ... WHERE user_id = ?` (the PK's leading column is `chat_id`; add a `user_id` index only if that purge is ever built).

**Pitfall 22 and the `last_msg_id` swap (decision).** Verified fact: the read is not cached: `checkGreetingSettings` calls `db.GetRecord` (`db.go:106-125`, `DB.WithContext(ctx).Where(where).First(model)`) and `upsertGreetingSettings` ends with `cache.DeleteCache(cache.CacheKey("greetings", chatID))` (`greetings/repository.go:236`) while no code reads that key through the cache (`grep CacheKey("greetings"` finds only the delete). The race is real but plain: `SendWelcomeMessage` reads `LastMsgId` at line 794, sends, deletes the old ID (line 821), then `SetCleanWelcomeMsgId` (line 822); two joins close together on two replicas read the same old ID and one welcome is never deleted (an orphan message, no data loss). **Recommendation: include it as one small, last, droppable plan.** It is not required by EDIT-07..09 and its worst case is cosmetic, but Phase 12 already rewrites exactly these two send blocks, the roadmap flags Pitfall 22, and the fix is about 40 lines. Smallest safe change: `greetings.SwapCleanWelcomeMsgID(chatID, newID int64) (old int64, err error)` and the goodbye twin, each one transaction: `Clauses(clause.Locking{Strength: "UPDATE"})` select of `welcome_last_msg_id`, then a column-targeted update to `newID`; the sender then deletes `old`. The sqlite dialector ignores the locking clause [VERIFIED: gorm.io/driver/sqlite sqlite.go:125-131, `"FOR": func(c clause.Clause, builder clause.Builder) { if _, ok := c.Expression.(clause.Locking); ok { // SQLite3 does not support row-level locking. return }`], so the same code runs in tests; PostgreSQL serialises the two swaps on the row. A single `UPDATE ... RETURNING <old>` as the research text suggests is not valid PostgreSQL (RETURNING yields new values), which is why the transaction form is recommended. If the plan budget is tight this is the one task to drop: AGENTS.md then records the known race in the Welcome bullet.

### S-03. Rules media and buttons

**What exists** [VERIFIED: fuku/modules/rules.go]: `buildRulesText(tr, title, rawRules)` (lines 33-41) returns `rules_no_rules_set` when `normalizeRulesForHTML(rawRules) == ""`, else `fmt.Sprintf(header, html.EscapeString(title)) + "\n\n" + normalized` with `header` from `rules_for_chat_header`. `sendRules` (156-218) loads `rules.GetChatRulesInfo`, builds `Text`, and when `chat_status.RequireGroup(bot, ctx, nil) && rules.Private` replaces it with `rules_click_for_rules` plus one URL button `t.me/%s?start=rules_%d` (the click-to-read message, A8: no media, no buttons there). It then sends with `msg.Reply(bot, Text, &gotgbot.SendMessageOpts{ReplyMarkup: rulesKb, ReplyParameters: &gotgbot.ReplyParameters{MessageId: replyMsgId, AllowSendingWithoutReply: true}, ParseMode: formatting.HTML})` and **no `LinkPreviewOptions`** (previews on). `rulesDeepLinkHandler` (369-429) checks `GetChat`, `IsUserInChat`, reads `rules.GetChatRulesInfo`, returns `rules_not_set` when the normalized text is empty, else `tr.GetString("rules_for_chat", i18n.TranslationParams{"first": html.EscapeString(chatinfo.Title), "second": normalizedRules})` sent with `msg.Reply(b, text, formatting.Shtml())`, in the opener's language. The two headings are different texts: `rules_for_chat: "Rules for <b>%s</b>:\n\n%s"` and `rules_for_chat_header: "The rules for <b>%s</b> are:"` [VERIFIED: locales/en.yml:1067, 1271]. `settingsEditRulesHeadings` (settings_edit_check.go:244-257) already measures **both** forms in all eight languages, so the dry run already covers the private form.

**Decision (S-03a): one builder, two heading forms, a literal legacy branch.**
- `buildRulesMessage(tr, title, row *models.RulesSettings, form)` returns `{Text, FileID, MsgType, Below, Keyboard, Empty}`. `Empty` is `!row.HasContent()` (S-03b). Text is: empty -> the form's no-rules line (`rules_no_rules_set` for the group form, `rules_not_set` for the private form); else the heading (group form: `fmt.Sprintf(header, EscapeString(title))`; private form: the `rules_for_chat` call above with `second` empty) plus `"\n\n" + normalizeRulesForHTML(row.Rules)` when that is non-empty, and **the bare heading with trailing newlines trimmed** when the text is empty but media or buttons exist (UI-SPEC A1). The group form is what `/rules` (private rules off, or a connected private chat) and Full preview send; the private form is the deep link. `buildRulesText` stays unchanged for the text-only case and is called by the builder, so its existing callers and tests keep passing.
- `Keyboard` is `keyboard.BuildKeyboard(row.Buttons)` wrapped as `InlineKeyboardMarkup` (same helper the welcome uses, `fuku/utils/keyboard`).
- **One sender**, `sendRulesMessage(...)`, used by `/rules`, the deep link and Full preview: with `FileID == ""` it makes **exactly the legacy call** (`SendMessage` with `ParseMode HTML`, the keyboard only when there are buttons, `ReplyParameters` as the caller passes them, no link-preview option); with media it calls `media.Send` with `Content{Text, FileID, MsgType, CaptionAbove: Below && eligible}` and `Options{ChatID, ReplyMsgID, AllowWithoutReply, Keyboard}`. This is what keeps success criterion 4 true: `SendGreeting` would switch previews off and drop reply parameters (`sender.go:329-346` `ReplyMsgID: 0`, `WebPreview: false`). Prove it with a **characterization test written first** that records the fake client's `sendMessage` parameters for `/rules` and for the deep link on an old row against today's code, then must stay green after the refactor (`TestRulesLegacyRowSendsTodaysRequest`).
- The deep link keeps `AllowSendingWithoutReply` false and `msg.Reply` semantics (reply to the `/start` message); `/rules` keeps `AllowSendingWithoutReply: true`. The reply target is a parameter of the sender, not a property of the builder.

**S-03b. "Rules is set" (UI-SPEC A1).** Add `func (r RulesSettings) HasContent() bool` on the model: `strings.TrimSpace(Rules) != "" || FileID != "" || len(Buttons) > 0` (use the same trimmed test `normalizeRulesForHTML` applies so a whitespace-only text is not content). Use it in `/rules`, the deep link, the Rules page `settingsEditMainState`, Full preview and **the `{rules}` button gate** (`formatting.go:318-321`: `rulesDb := rules.GetChatRulesInfo(chat.Id)` / `if rulesDb.Rules == "" {` / `return res, btns`), otherwise a media-only Rules would show no `{rules}` button. `LoadRulesStats` (`rules/repository.go:94`: `Where("rules != ?", "")`) counts text only; leave it (a developer stat, and a jsonb comparison is not portable between PostgreSQL and the SQLite test database).

**Storage helpers** (`fuku/db/rules`, all via `checkRulesSetting` then `db.UpdateRecordWithZeroValues(&models.RulesSettings{}, models.RulesSettings{ChatId: chatId}, map[string]any{...})`, exactly like `SetChatRules` at `repository.go:60-69`, so a zero value is written): `SetRulesMediaOnly(chatID, fileID, mediaType)` writes `file_id`, `media_type`; `SetRulesButtonsOnly(chatID, buttons)` writes `btns` as `models.ButtonArray(buttons)`; `SetRulesMediaBelow(chatID, on)` writes `media_below`; `ClearRules(chatID)` writes `rules ""`, `file_id ""`, `media_type 0`, `btns []` (the `/clearrules` write, D-04). Rules reads are uncached (no `CacheKey("rules"` in the repo), so there is no key to delete; say so in each doc comment. New loader `LoadRulesSettingsContext(ctx, chatID) (*models.RulesSettings, error)` creating nothing and returning a zero row for "no row"; `LoadRulesContext` stays as `return row.Rules` wrapper (about ten call sites and tests use it: `rules.go:74`, `settings_edit.go:139`, `settings_edit_actions_test.go:818-819`, `fuku/db/rules/*_test.go`).

**Apply functions** (beside the commands, `rules.go`): `rulesWrites` gains `SetRulesMediaOnly`, `SetRulesButtonsOnly`, `SetRulesMediaBelow`, `ClearRules` (the test seam; the existing test at `settings_edit_actions_test.go:730-733` overrides `rulesWrites.SetChatRules` and keeps working because Remove text still goes through it). Semantics to preserve D-04: **Remove text** = `SetChatRules(id, "")` (text only; today it calls `applyRulesClear`, which must now mean all three), **Remove media** and **Remove buttons** clear only their part, **`/clearrules`** (`clearRules`, `rules.go:79-104`) calls the three-part `ClearRules`. `rulesApplyResult` keeps its `Rules string` field (used by `rules_apply_test.go:36`) and gains `Row *models.RulesSettings` for the page redraw.

**Rules language for Full preview (UI-SPEC M3).** `/rules` translates with `i18n.MustNewTranslator(lang.GetLanguage(ctx))` (rules.go:185); in a group `GetLanguage` takes `getGroupLanguageContext(tracing.UpdateContext(ctx), chat.Id)` [VERIFIED: lang/repository.go:53 `return getGroupLanguageContext(tracing.UpdateContext(ctx), chat.Id)`], which is `getCachedLanguage(ctx, cache.CacheKey("chat_lang", groupID), &models.Chat{}, "chat_id", groupID)` (line 61) with `"uk"` as the fallback on a missing row, an empty value and any error (lines 80-97). In a private menu `GetLanguage` would return the presser's language (line 42 `if chat.Type == "private" {`), so add an exported accessor `lang.GetGroupLanguage(ctx context.Context, groupID int64) string` that is exactly `getGroupLanguageContext`. Same cached read, same `"uk"` fallback, **creates no row** (a SELECT through `cache.GetFromCacheOrLoad`). Build Full preview with `i18n.MustNewTranslator(lang.GetGroupLanguage(ctx, group.Id))` and keep `p.tr` for `settings_edit_preview_header_rules`. Update the stale comment above `settingsEditPreviewRules` and the AGENTS.md sentence in the same commit (S-07).

**The caption rule, one predicate.** Today `settingsEditCheck` has `case in.Def.Kind == settingsEditRules: // Rules has a text part only.` [VERIFIED: settings_edit_check.go:116-117], which falls through to `settings_edit_refuse_need_text` for media and buttons. Remove that arm so Rules media and buttons reach `settingsEditCheckMedia` and `settingsEditCheckButtons`. Changes inside the shared functions:
- `settingsEditCheckRulesText`: after the conversion, `limit := settingsEditTextLimit` or `settingsEditCaptionLimit` when `settingsEditPartIsSet(in.State, settingsEditPartMedia)` (the Welcome test, any stored kind including sticker and video note). With media and `longest > 1024`: `settingsEditOverLimit("settings_edit_refuse_text_rules_long", longest, 1024)` (new key). Without media and `longest > 4096`: keep `settingsEditTooLong` (old key; see Open Question 11). `longest` is already the max over the eight languages and both heading forms.
- `settingsEditCheckMedia`: for `Def.Kind == settingsEditRules` measure `max over settingsEditRulesHeadings(title) of formatting.VisibleLength(heading + normalizeRulesForHTML(stored))` against 1024 (when the stored text is empty the caption is the heading alone; measuring `heading + ""` over-counts by two units, which is conservative), refuse with `settings_edit_refuse_media_rules_long` (new key), and skip the placeholder fill (`settingsEditFilledLongest` fills `{first}` etc., which rules never do). A video note stays refused as typed media (`dataType == -1 || dataType == db.VIDEO_NOTE` -> `settings_edit_refuse_need_media`), as on Welcome.
- `settingsEditCheckButtons`: line 307 is `total, widest := settingsEditButtonRows(buttons, in.State.Text)`; for Rules pass `""` so a `{rules}` string inside the rules text is not counted as a button (Pitfall 6).
- `settingsEditPromptMax` (`settings_edit_actions.go:56-64`) has `case def.Kind == settingsEditRules: return settingsEditRulesPromptMax` first; reorder so a stored media (the Welcome's `settingsEditPartIsSet(st, media)`) gives 1024 for Rules too, and 3800 otherwise.
- The door's `apply` (`settings_input_door.go:442-444`) is `if r.def.Kind == settingsEditRules { return applyRulesText(...) }` regardless of part; replace by a part switch (`applyRulesText`, `applyRulesMedia`, `applyRulesButtons`). Its other Rules specifics (`def.EditRight == settingsRightAdminBotAdmin && !r.botAdmin()`, line 269) are already generic for all parts.

**Runtime hardening (recommended, small).** A rules row can carry media and a caption over 1024 without going through the editor: `/setrules` stores any text (no check of the stored media), and a backup import restores both. `/rules` would then fail at send with a 400 and say nothing. In the member-facing senders only (`/rules`, deep link; **not** Full preview, so the owner still sees the failure as `settings_edit_preview_failed`), on a definite 4xx from the media send retry once as the text-only message (with the buttons) and log at Warn by chat ID. A 4xx means Telegram did not send, so there is no duplicate risk. This covers the caption limit, a `file_id` that belongs to another bot (backup import, Pitfall 12) and entity parse errors with one rule.

### S-04. Storage

**Migration.** Name `migrations/20261105120000_add_rules_media_and_greet_once.sql` (greater than the latest, `20261104120000_add_settings_hub_tables.sql`; the runner reads `*.sql` generically, `runner.go:153-158`, so no applier change is needed: nothing registers or lists migrations, and `scripts/migrate_psql.sh` runs the same files; keep both appliers untouched and say so in the plan). One transaction by the runner; no `BEGIN`/`COMMIT`; every statement idempotent; constraints inline:

```sql
ALTER TABLE greetings ADD COLUMN IF NOT EXISTS welcome_media_below BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE greetings ADD COLUMN IF NOT EXISTS goodbye_media_below BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE greetings ADD COLUMN IF NOT EXISTS welcome_once BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE rules ADD COLUMN IF NOT EXISTS file_id TEXT NOT NULL DEFAULT '';
ALTER TABLE rules ADD COLUMN IF NOT EXISTS media_type BIGINT NOT NULL DEFAULT 0;
ALTER TABLE rules ADD COLUMN IF NOT EXISTS btns JSONB NOT NULL DEFAULT '[]'::jsonb;
ALTER TABLE rules ADD COLUMN IF NOT EXISTS media_below BOOLEAN NOT NULL DEFAULT FALSE;
CREATE TABLE IF NOT EXISTS greeted_members (
    chat_id BIGINT NOT NULL,
    user_id BIGINT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    PRIMARY KEY (chat_id, user_id)
);
```

This follows the repo's own precedent for added columns [VERIFIED: migrations/20261029120000_add_raid_detection_ai.sql:24 `ALTER TABLE chat_raid_detection ADD COLUMN IF NOT EXISTS ai_enabled BOOLEAN NOT NULL DEFAULT TRUE;`] and for tables [VERIFIED: migrations/20261104120000_add_settings_hub_tables.sql `CREATE TABLE IF NOT EXISTS chat_settings_access (` ... `CONSTRAINT uk_chat_settings_access_chat UNIQUE (chat_id)`]. The existing `greetings` columns are nullable (`"welcome_type" bigint`, `"welcome_btns" jsonb`, initial migration lines 167-168), the new ones are `NOT NULL` with the zero default as D-14 requires, so every existing row reads as zero and a restored row stays valid. Do **not** use `ADD CONSTRAINT` (duplicate_object is swallowed). Media type 0 on `rules` means "none" and sends as text (S-01).

**Models** (no `default:` tag on any boolean, the GORM trap noted in `chat_settings_access.go`'s comment): in `models/greetings.go` add to `WelcomeSettings` `MediaBelow bool gorm:"column:media_below" json:"media_below"` and `Once bool gorm:"column:once" json:"once"`, and to `GoodbyeSettings` `MediaBelow` likewise; the embedded prefixes `welcome_` / `goodbye_` make the columns `welcome_media_below`, `welcome_once`, `goodbye_media_below` [VERIFIED: models/greetings.go `embedded;embeddedPrefix:welcome_` and `WelcomeSettings *WelcomeSettings gorm:"embedded;embeddedPrefix:welcome_"`]. In `models/rules.go` add `FileID string gorm:"column:file_id"`, `MediaType int gorm:"column:media_type"`, `Buttons ButtonArray gorm:"column:btns;type:jsonb"`, `MediaBelow bool gorm:"column:media_below"` with json tags `file_id,omitempty`, `media_type,omitempty`, `btns,omitempty`, `media_below`. `ButtonArray.Value()` writes `"[]"` for an empty array [VERIFIED: models/types.go `func (ba ButtonArray) Value() (driver.Value, error) { if len(ba) == 0 { return "[]", nil }`] and `Scan(nil)` gives an empty array, so a nil slice never violates `NOT NULL`. New `models/greeted_member.go` with `TableName() "greeted_members"`.

**Writers (`fuku/db/greetings`)**, all through `upsertGreetingSettings` (which already ends in `cache.DeleteCache(cache.CacheKey("greetings", chatID))`, line 236) so a Go `false` is written by `Updates(map)`: `SetWelcomeMediaBelow(chatID, on)` -> `welcome_media_below`, `SetGoodbyeMediaBelow` -> `goodbye_media_below`, `SetWelcomeOnce` -> `welcome_once`. Apply functions beside `/welcome`: `applyWelcomeMediaBelow`, `applyGoodbyeMediaBelow`, `applyWelcomeOnce` through `finishGreetingApply` and new entries in `greetingWrites` (the same test seam). Remove media / `/resetwelcome` / `/resetgoodbye` do **not** touch the three switches (UI-SPEC A3); `applyGreetingReset` writes four columns only (greetings.go:305-310).

**Harness AutoMigrate lists.** `GreetingSettings` and `RulesSettings` are already in every list that uses them (`fuku/modules/test_harness_test.go:337,343`, `fuku/db/testmain_test.go:45,59`, `fuku/db/devs/testmain_test.go:54,61`, `fuku/db/user`, `fuku/db/warns`, `fuku/db/rules/repository_test.go:48,62`), and AutoMigrate adds the new columns by itself, so **column changes need no list edit**. The new model `GreetedMember` must be added to: `fuku/modules/test_harness_test.go` (join tests), `fuku/db/greetings/testmain_test.go` (`testdb.Run(m, &models.GreetingSettings{}, &models.GreetedMember{})`), and `fuku/db/backup/testmain_test.go` (so a test can prove backup, import and reset leave it alone). `editCleanup` (`settings_edit_env_test.go:33-40`) should also delete `greeted_members` rows of its chats. `internal/testdb.Run` takes the list as arguments (`database.AutoMigrate(tables...)`), so no change there.

**Backup (D-13)** [VERIFIED: fuku/db/backup/types.go:181,196 `type GreetingsBackup = SettingBackup[models.GreetingSettings]`, `type RulesBackup = SettingBackup[models.RulesSettings]`]. Export is `findChatSetting[models.GreetingSettings]` and import is `replaceChatSetting`, which JSON-round-trips the struct and builds a column map from `stmt.Schema.Fields` (`backup.go:209-238`), so the new columns travel as soon as the models carry JSON tags; an old export has no such keys and decodes to zeros, which is the old behaviour. `clearGreetings` and `clearRules` (reset) build structs with zero new fields, so a reset clears the switches and the rules media (a reset is "everything default", consistent with `RulesSettings{ChatId: chatID}`). `AllExportableModules` is an explicit list of module names (`types.go:119-139`), so `greeted_members` is out of backup, export, import and reset by construction; the test proves it. Importing a backup *replaces* a chat's `welcome_once`/`media_below` with the export's values; accepted (replace semantics). `TestAllModulesRoundTripEveryMeaningfulField` (`roundtrip_test.go:19`) creates `models.GreetingSettings` and `models.RulesSettings` literals (lines 81 and 121) and must gain the new fields; it is already in the PostgreSQL integrity list.

**PostgreSQL-only guarantees (`make test-postgres-integrity`).** Add (names are the plan's contract): `TestGreetedMemberClaimConcurrent` (`./fuku/db/greetings`: N goroutines, one winner, release then claim again, a different chat is independent, the PK refuses a second row); `TestRulesMediaMigrationDefaults` (`./fuku/db/migrations`, PostgreSQL only, rolled-back transaction in the style of `TestCaptchaModeTurnstileConstraint`: after the chain, an existing row reads `welcome_once`/`media_below` false, rules `file_id ''`, `media_type 0`, `btns '[]'`, and the migration's statements can run twice); and, only if the swap plan is kept, `TestSwapCleanMsgIDConcurrent` (`./fuku/db/greetings`). The database-free half is `TestRulesMediaMigrationStatements` (in `make test`; asserts the statement count, that every statement is idempotent, no `ADD CONSTRAINT`, no transaction control, and that the file name sorts last). The Makefile `-run` list is anchored with `$`, so list the real function names, and `./fuku/db/greetings` must be added to the package list (it is absent today) [VERIFIED: Makefile `test-postgres-integrity` package list `./fuku/db/backup ./fuku/db/captcha ... ./fuku/db/menuaccess`].

### S-05. Editor reuse

**Parts table.** `settingsEditPages[2]` (Rules, `settings_edit.go:127-132`) has `Parts: []settingsEditPart{settingsEditRulesTextPart}`; make it `{settingsEditRulesTextPart, settingsEditMediaPart, settingsEditButtonsPart}` (both entries already exist, lines 95-106). The renderer's `settingsEditBody`, `settingsEditPartRows` and `settingsEditPreviewRows` loop over `def.Parts`, so Rules gets its lines, Set/Remove rows and Preview buttons with no renderer change; the sticker/video-note note (`settingsEditBody`, line 298) already applies to any page.

**State.** `settingsEditState` (lines 153-163) gains `MediaBelow bool`, `Once bool`; `settingsEditStateOf` copies `w.MediaBelow`, `w.Once` (Welcome) and `g.MediaBelow` (Goodbye); `settingsEditLoad`'s Rules arm (lines 200-206, today `return settingsEditState{Kind: def.Kind, Text: text}, nil`) uses `LoadRulesSettingsContext` and fills `Text, MediaType, FileID, Buttons, MediaBelow`. `settingsEditRulesRead` (a test seam, line 139) changes type with it.

**Switch grid.** Replace `settingsEditSwitchRows` (lines 358-369, two full-width rows) by a cell list built from the definition and state, two cells per row, filled left to right: Welcome `e, d, u?, o`; Goodbye `e, d, u?`; Rules `u?` (`u?` = drawn only while `settingsEditMediaBelowEligible(st)`). Add `settingsStateIcon(on) string` (`"✅"`/`"❌"`) beside `settingsStateWord` (settings_pages.go:101-106, `"✅ " + ...`); the button label is `icon + " " + name`, the body line is `settingsText(key, {state: settingsStateWord})`. `settingsToggleButton` (settings_pages.go:327-338) returns a one-button row; build the buttons the same way but collect them into pairs. `settingsEditPageDef` gains line keys for the two renamed keys. **Control codes:** `u` Media under text on `w`, `g`, `r`; `o` Greet once on `w` only; the part codes `x m b` and the control codes `e d u o` never meet (parts are `t` of the part actions, controls are `t` of `tg`). `init()` (lines 508-527) registers `Controls` for non-Rules pages only; make it `Controls` per page: Welcome `{e d u o}`, Goodbye `{e d u}`, Rules `{u}`. `TestSettingsEditRegistry` currently asserts `(w.key != "r") != (len(pg.Controls) == 2)` (settings_edit_test.go:866) and must change to 4/3/1. Rights: `e` `settingsRightAdmin`, `d` `settingsRightChangeInfo` (existing), `u` the page's `EditRight` (A2), `o` `settingsRightAdmin` (D-10).

**Apply path.** `settingsEditApplySwitch` (lines 443-457) returns `greetingApplyResult`, which has no Rules shape. Generalise it to return the outcome and a `*settingsEditState` (the pattern `settingsEditApplyRemove` already uses, settings_edit_actions.go:196-226) so Rules `u` fits. Each switch handler first reloads the page (`settingsEditLoad`): for `u`, if the stored media is not eligible it **writes nothing** and redraws with `settingsNoticeWarn(settingsText(tr, "settings_edit_below_unavailable"))` (UI-SPEC A5); otherwise it calls `applyWelcomeMediaBelow` / `applyGoodbyeMediaBelow` / `applyRulesMediaBelow` and redraws from the read-back, no notice. The check-then-write gap is harmless: a stored "on" with an ineligible media is ignored by the sender.

**Core `tg` validation hook.** `settingsCheckFields` for `settingsActToggle, settingsActSet` requires `p`, `t`, `v` but never checks that `t` is a control of page `p` or that `v` is valid [VERIFIED: settings_menu.go:262-266 `case settingsActToggle, settingsActSet: for _, key := range []string{"p", "t", "v"} {`], so `t=o` on Goodbye reaches `runControl` after the live `getChatMember` and answers `common_callback_invalid_request` there (`runControl`, settings_menu.go:533). Page actions have `Validate`; controls do not. The UI-SPEC check 3 ("each `common_callback_invalid_request` before any I/O") needs two small additions: in `settingsCheckFields`, when the page is registered and `Controls[fields["t"]]` is absent, return false; and an optional `settingsControl.Validate func(v string) bool` (nil = unchanged) set to "0 or 1" on the editor switches. No existing test depends on the later refusal (grep of `"a", "tg"` in tests found only valid controls).

**Main page.** `settingsEditMainState` (lines 404-423): Rules reads `Text`/media/buttons through the loader and answers `settings_state_set` when `HasContent()` (A1); Welcome and Goodbye stay `On`/`Off`.

**Locales (13 new, 4 renamed, 1 changed).** Counts verified this session: `grep -cE '^settings_edit_' locales/en.yml` = 62, `^settings_btn_edit_` = 15, `^settings_` = 186 [VERIFIED: grep], matching the UI-SPEC; 8 locale files plus `config.yml`. After: 74 + 16 = 90 editor keys and 199 `settings_` keys. The four renames (`settings_btn_edit_welcome_toggle` -> `settings_edit_line_welcome`, `settings_btn_edit_goodbye_toggle` -> `settings_edit_line_goodbye`, `settings_btn_edit_clean_welcome` -> `settings_edit_line_clean_welcome`, `settings_btn_edit_clean_goodbye` -> `settings_edit_line_clean_goodbye`) touch `settings_edit.go` (2 lines), `settings_edit_test.go` (10), `settings_edit_actions_test.go` (2), `settings_locale_test.go` (10) and 4 lines in each of 8 locale files [VERIFIED: grep counts]; the text is byte-identical so no retranslation. **Limits-table gotcha:** `TestSettingsLocaleKeys` checks `utf8.RuneCountInString(text)` of the template **as written** against `{max, other}` [VERIFIED: settings_locale_test.go:312 `if runes := utf8.RuneCountInString(text); runes > ceiling {`], so a row for `{state} Media under text` (24 characters as written in English, up to 26 in other languages with an 18-character name) cannot carry the UI-SPEC's 20. Give the five switch keys a row of `{26, 26}` in the table and put the binding rule where it can be rendered: extend `TestSettingsLabelLengths` (settings_locale_test.go:384-) with a check that replaces `{state}` by the single icon (`✅`, then `❌`, **no word and no extra space**: unlike the existing `editorSwitches` loop at lines 454-463 that substitutes `icon + word`) and fails above 20 in all 8 locales, and delete the old four-key `editorSwitches` list (lines 420-425, 454-463). The placeholder test must expect `{state}` in keys 1 to 7, `{n}` and `{max}` in the two new Rules refusals, and `<code>/rules</code>` in `settings_edit_hint_rules` (extend `settingsEditLocaleTokens` at `TestSettingsEditLocaleTokens`, line 629-648). `settings_edit_below_unavailable` is a notice (a `⚠️` line on the redrawn page), not an alert, so it stays out of `settingsAlertKeys`.

### S-06. Tests that encode behaviour this phase changes on purpose

The plans must edit these explicitly (not discover them as red tests):

| File:line | Today asserts | Change |
|-----------|---------------|--------|
| `fuku/utils/content/editor_helpers_test.go:142-176` `TestExtractMedia` | animation (with or without a document) -> `db.DOCUMENT`; loop asserts `ExtractMedia == extractMediaFromReply` for every case | animation cases -> `db.ANIMATION` with `Animation.FileId`; the equality loop must exclude animation (the legacy function stays) and a new case pins that `/save`'s extraction still gives `DOCUMENT` |
| `fuku/utils/content/content_test.go:160-255` `TestExtractMediaFromReply` | animation -> `DOCUMENT` | unchanged (legacy function kept); add a greeting-extraction twin |
| `fuku/modules/settings_input_media_test.go:237-239` | `if kind == "animation" && editorType != db.DOCUMENT {` ... "a GIF is stored as type %d, want the document type" | flip to `db.ANIMATION`; the surrounding parity assertion (editor stores what `/setwelcome` by reply stores) stays and now proves both are animations |
| `fuku/modules/settings_edit_check_test.go:361,380-385` | "a GIF is stored as a document" | flip to animation |
| `fuku/modules/settings_edit_test.go:268-294` "Rules has the text line only and no switch" | Rules page has no media/buttons lines and no switch buttons | replace by "Rules has three parts and one switch cell while eligible" |
| `fuku/modules/settings_edit_test.go:383-463` "the mock states draw exactly the contract's rows" | rows end `toggle, clean` full width; Rules rows `set_text ... full_preview` only | rewrite rows to the grid (`[Welcome \| Delete last]`, `[Media under text \| Greet once]`); Rules mocks gain media/buttons rows; `settingsEditLabelName(def.ToggleLabelKey)` uses the renamed keys |
| `fuku/modules/settings_edit_test.go:847-896` `TestSettingsEditRegistry` | `len(pg.Controls) == 2` for w, g; none for r | 4, 3, 1 controls |
| `fuku/modules/settings_edit_test.go:898-946` `settingsEditCallbackStates` / `TestSettingsEditCallbackBytes` | states use `db.VIDEO_NOTE`/`PHOTO`/`STICKER` | add `MediaBelow`/`Once`/ANIMATION states; Rules "all set" now draws 3 Previews (`previews != len(def.Parts)` is generic) |
| `fuku/modules/settings_edit_test.go:972-1030` `TestSettingsEditPagesFitEveryLocale` | pages fit 3800 units | add the new hints and switch lines to the worst case |
| `fuku/modules/settings_edit_actions_test.go:468-483` `TestSettingsEditPromptMax` | Rules `3800` even with `Text: "t"` | add Rules with media -> 1024; no media -> 3800 |
| `fuku/modules/settings_edit_actions_test.go:548-585` `TestSettingsEditCallbackFields` | `"rs with t=m"` is refused as invalid | now valid on Rules; the invalid example becomes `rs with t=z` (UI-SPEC) |
| `fuku/modules/settings_edit_actions_test.go:760-830` `TestSettingsEditRemoveParity` (rules half, ~806-820) | Remove text equals `/clearrules` for text only | three Removes together equal `/clearrules`' stored row (all three parts) |
| `fuku/modules/settings_edit_preview_test.go:285-361` `TestSettingsEditRulesPreview` | Full preview equals `buildRulesText(tr "en", ...)`; `/rules` equals it | Full preview and `/rules` equal the new builder's message; add the group-language case (presser `en`, group `uk` or another) and keep the header in the presser's language |
| `fuku/i18n/settings_locale_test.go:144-223,405-463` | four `settings_btn_edit_*_toggle/clean_*` rows and the four-key full-width loop | rows renamed; new 13 rows; the five-cell rendered check |
| `fuku/db/backup/roundtrip_test.go:19-...` | greetings and rules literals without the new fields | add the fields so the round trip proves they travel |
| `AGENTS.md` (see S-07) | the sentences listed below | rewritten in the same commit as the code |

Nothing else asserts the old Rules layout: `TestSettingsMain*` checks the Rules state line only through `settingsMainStateLine` and stays valid; `rules_apply_test.go` and `rules_command_test.go` assert the text path and keep passing.

### S-07. AGENTS.md sentences that become false

Rewrite in the same commits as the code (AGENTS.md rule): line 178 ("a GIF as a document" in the typed-reply media part), lines 416-418 ("Rules has `x` only", "a GIF is stored as a document and reads File or GIF"), lines 1566-1567 (the Rules Full preview heading "built in the presser's language ... not the group's as `/rules` has it", superseded by UI-SPEC M3). Add: the two new `tg` controls and the Rules part actions under Callbacks (`settings|v1`); the new columns and the `greeted_members` table (outside backup, export, import and reset; read fresh; the claim/release rule and the accepted limits) under Data; the new ANIMATION type, the shared Rules builder and the greet-once claim under Subsystem traps; the PostgreSQL list and the harness lists under Testing; the two new refusal keys and `lang.GetGroupLanguage`.

## Architecture Patterns

### System Architecture Diagram

```
 Owner (private chat, editor)                                        Members
   |  press Set / switch (settings|v1 tg t=u|o, rs/rr/rv/rf t=x|m|b)    |
   v                                                                    |
 settingsPress.verify (live getChatMember, owner gate 4b, right)        |
   |            \                                                       |
   |             +--> settingsCheckFields: unknown t / bad v refused before I/O (NEW hook)
   v                                                                    |
 apply functions beside commands ---> column-targeted writers --------> PostgreSQL
 (applyWelcomeOnce, applyXMediaBelow,   greetings: welcome_once,          greetings / rules rows
  applyRulesMedia, applyRulesButtons)   *_media_below; rules: file_id,    (+ greeted_members)
                                        media_type, btns, media_below
   |
   v  typed reply (door, group 1) -> settingsEditCheck (shared predicates) -> apply -> saved page

 Join (chat_member / service msg)      captcha pass (button or web)         /rules, deep link, Full preview
   |  ProcessSingleJoin                  |  finishCaptchaPass                   |
   +-----------> SendWelcomeMessage <----+                                    v
                   | ShouldWelcome? -no-> return                        buildRulesMessage(form)
                   | Once? -yes-> ClaimGreeting (INSERT .. DO NOTHING)        |
                   |   lost/error -> return (no send)                         v
                   v                                                   sendRulesMessage
              renderGreeting                                      FileID=="" -> legacy SendMessage
                   v                                              media     -> media.Send(CaptionAbove)
        media.SendGreetingMessage ---------------------------------------> Telegram
                   | err: definite refusal & claimed -> ReleaseGreeting
                   v
        CleanWelcome bookkeeping (only after a send)
```

### Recommended Project Structure
```
migrations/20261105120000_add_rules_media_and_greet_once.sql
fuku/db/db.go                       # ANIMATION constant
fuku/db/models/{greetings,rules,greeted_member}.go
fuku/db/greetings/{repository.go, greeted.go}   # media-below/once writers, Claim/ReleaseGreeting, (swap)
fuku/db/rules/repository.go         # LoadRulesSettingsContext, SetRulesMediaOnly/ButtonsOnly/MediaBelow, ClearRules
fuku/db/lang/repository.go          # GetGroupLanguage(ctx, chatID)
fuku/utils/content/content.go       # extractGreetingMedia; ExtractMedia + ExtractWelcome use it
fuku/utils/media/sender.go          # ANIMATION case, Content.CaptionAbove, CaptionAboveEligible, SendGreetingMessage
fuku/modules/rules.go               # buildRulesMessage, sendRulesMessage, apply functions, HasContent use
fuku/modules/greetings.go           # claim in SendWelcomeMessage, classifier, apply functions, flag at 4 send sites
fuku/modules/settings_edit*.go      # parts, grid, controls, checks, preview, door apply, Main state
fuku/utils/formatting/formatting.go # {rules} gate via HasContent
locales/*.yml, fuku/i18n/settings_locale_test.go
```

### Pattern 1: Claim, send, give back on a definite refusal
**What:** a conditional insert is the claim; the send follows; only a definite 4xx/permission refusal deletes the row.
**When:** `SendWelcomeMessage` with `Once` on.
**Example:**
```go
// Source: this research (S-02), pattern of lockdown.RecordJoin (fuku/db/lockdown/joiners.go:65-71)
claimed := false
if greetPrefs.WelcomeSettings.Once {
	ok, err := greetings.ClaimGreeting(chat.Id, userID)
	if err != nil {
		log.Errorf("[Greetings][SendWelcomeMessage] greet-once claim failed for chat %d: %v", chat.Id, err)
		return nil // fail closed: a missing welcome beats a double one
	}
	if !ok {
		return nil // already greeted, or another replica won
	}
	claimed = true
}
sent, err := media.SendGreetingMessage(bot, chat.Id, msg)
if err != nil {
	if claimed && welcomeDefinitelyNotSent(err) {
		if rerr := greetings.ReleaseGreeting(chat.Id, userID); rerr != nil {
			log.Warnf("[Greetings] giving the greet-once record of chat %d back failed: %v", chat.Id, rerr)
		}
	}
	return err
}
```

### Pattern 2: One setting, one code path (unchanged)
Every new switch has an apply function beside its command/page home that holds the write and the read-back (`finishGreetingApply`, `finishRulesApply`); the menu and any future command call it. Greet once and Media under text have no command, so the proof is a drift test and an authority test (D-10, UI-SPEC check 6, 7).

### Pattern 3: Pure renderer plus table-driven blocks
`renderSettingsEditor` stays pure; the switch block is a cell list; a later part or switch is a table row.

### Anti-Patterns to Avoid
- **Changing `extractMediaFromReply`** for everyone: it also stores GIFs for `/save` and `/filter`.
- **Routing `/rules` through `SendGreeting`:** it turns link previews off and drops the reply parameters, so an old rules row would no longer send today's request.
- **Claiming before the `ShouldWelcome` check or in `ProcessSingleJoin`:** a disabled welcome would record members, and the captcha-pass path would be missed.
- **Reading `greeted_members` from the page, a preview or a stats line:** previews must never claim; pages never show it.
- **Classifying a send error by wording:** use codes and the `ErrNoPermission` sentinel.
- **Copying the Welcome's media/caption list into Rules:** share the predicates by calling them.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Exactly-once across replicas | A Redis lock, an in-process map, a read-then-insert | `INSERT ... ON CONFLICT DO NOTHING` + `RowsAffected` on a PK table | Durable, Redis-free, proven by the scratch run and a PostgreSQL CI test |
| Refusal classification | String matching on Telegram descriptions | `isTelegramClientError` + `errors.Is(err, media.ErrNoPermission)` (+ 429 by code) | Wording changes; codes do not |
| Caption-above support list | A list in the page, the sender and the check | `media.CaptionAboveEligible(type)` | One predicate (UI-SPEC "Eligibility") |
| Rules message text | A second builder for Full preview or the deep link | `buildRulesMessage(form)` | Preview, `/rules` and private rules cannot drift (D-02) |
| Group language lookup | A new query or `lang.GetLanguage(ctx)` in a private menu | `lang.GetGroupLanguage(ctx, chatID)` over the cached read | Same fallback to `uk` as `/rules`; creates no row |
| Button limits, URL validity, length counts | New regexes | `content.ValidButtonURL`, `settingsEditButtonRows`, `formatting.VisibleLength`, `content.TextLengthRefusal` | Phase 11 parity helpers |
| Last-welcome swap | `UPDATE ... RETURNING <old>` | a short transaction with `clause.Locking` | `RETURNING` cannot return the old value in PostgreSQL |

**Key insight:** every hard part of this phase already has a proven helper in the repo (conditional insert, wait store, apply functions, shared checks). The risk is not missing machinery but drifting from legacy behaviour (previews, headings, GIF storage in other modules), which is why the plan leads with characterization tests.

## Runtime State Inventory

Not a rename/refactor phase, but it adds persistent state and changes how stored values are read:

| Category | Items Found | Action Required |
|----------|-------------|------------------|
| Stored data | `greetings` gains 3 boolean columns, `rules` gains 4 columns, new table `greeted_members`; every existing row reads as zero/default through `NOT NULL DEFAULT`; GIFs saved earlier stay `DOCUMENT` (3) and are never rewritten | one migration (code edit + schema), **no data migration** |
| Live service config | None: nothing lives in an external UI; the bot has no external service holding these values | none (verified: settings are database rows only) |
| OS-registered state | None | none |
| Secrets/env vars | None added; no new setting | none |
| Build artifacts | None | none |
| Redis | No new key family: greet-once is database-only and works without Redis; the editor wait keys of Phase 11 are unchanged | none |

Rolling deploy (accepted, like the other caveats in AGENTS.md): a replica still on the previous code ignores the new columns (welcomes every join, no caption placement), sends type 9 as text only (`Unknown message type 9, falling back to text`), and its `/rules` ignores media and buttons until replaced.

## Common Pitfalls

### Pitfall 1: A stored "on" with an ineligible media must stay invisible and unsent
**What goes wrong:** a document or sticker with `media_below` true is drawn as on, or `sendDocument` is given a flag.
**Why:** the flag is a separate column from the media (D-07).
**How to avoid:** draw and send only through `CaptionAboveEligible`; the sender's non-photo/video/animation functions never read `CaptionAbove`. **Warning signs:** a test with a sticker + `MediaBelow` true that draws the line or the button.

### Pitfall 2: GIF branches in the wrong order
`Document` is set on every animation message, so testing it first returns `DOCUMENT` forever (content.go:124-125 before 134-135). Test `Animation` first in the greeting extraction; keep the legacy order for notes/filters/pins. Warning sign: the parity test shows `db.DOCUMENT` for a GIF.

### Pitfall 3: Today's `/rules` request changes by accident
`SendGreeting` disables link previews and sends no reply parameters; `/rules` today previews links and replies with `AllowSendingWithoutReply: true`. **Avoid:** the legacy literal branch for `FileID == ""` and a characterization test recorded before the refactor. Warning sign: the recorded `sendMessage` params of an old row differ.

### Pitfall 4: The two Rules headings
The deep link wording (`rules_for_chat`) and `/rules` wording (`rules_for_chat_header`) both ship to members today. One builder with a `form` argument; the dry run already measures both; Full preview uses the group form.

### Pitfall 5: Sticker and video note rules lose their text
Telegram sends them without caption; the page shows `settings_edit_note_no_caption` (now on Rules too). The 1024 rule still applies to them (shared predicate), by decision. Do not add a Rules exemption.

### Pitfall 6: `{rules}` inside a Rules text counts as a button
`settingsEditButtonRows(buttons, storedText)` adds a button for a `{rules}` token. Pass `""` for Rules or a rules text containing `{rules}` could refuse a valid keyboard at 100 buttons.

### Pitfall 7: `settings_edit_refuse_too_long_filled` wording on Rules
Its English talks about names filled in; Rules has none. With media Rules uses the new key. For a no-media text over 4096 the existing key stays (Open Question 11), so the UI-SPEC check 8 sentence "never carries the wording" must be scoped to the with-media case in the test.

### Pitfall 8: Claim before the right place, or never given back
A claim before `ShouldWelcome` records members that were not welcomed; a claim without the give-back on a 4xx (wrong `file_id`, bot lost rights) suppresses welcomes for good; a give-back on an unknown outcome (timeout) lets a double welcome through. Classify by code and sentinel only; keep on unknown.

### Pitfall 9: A preview or the page touching `greeted_members`
Full preview of a Welcome with Greet once on must never claim a member (UI-SPEC A9, check 9). The preview path never calls `SendWelcomeMessage`; assert that no row exists after each preview.

### Pitfall 10: Using the join dedupe for the record
`claimRecentJoinProcessing` fails closed on a Redis error (greetings.go:98-103 comment `// Genuine Redis error — fail closed`) and expires in seconds. The greet-once record must not depend on it (D-12); test with no Redis installed.

### Pitfall 11: Limits-table and label arithmetic
The generic locale test counts templates as written (26 for the five switch keys), the real 20-code-point check is the new rendered one; `hi` combining marks look wider than their code-point count (phone check on the owner's list).

### Pitfall 12: Page rows
Welcome worst case is 8 rows private (3 part + 2 preview + 2 switch + nav) and 9 in a group; the grid only fits if the switches are two to a row. Build the worst case in a test.

## Code Examples

### Sender additions (skeleton)
```go
// Source: this research (S-01b); shape of fuku/utils/media/sender.go:155-171 sendPhoto
func sendAnimation(b *gotgbot.Bot, content Content, opts Options, parseMode string, replyParams *gotgbot.ReplyParameters) (*gotgbot.Message, error) {
	if content.FileID == "" {
		log.Warnf("[Media] Empty FileID for ANIMATION '%s' in chat %d, falling back to text", content.Name, opts.ChatID)
		return sendText(b, content, opts, parseMode, replyParams)
	}
	sentAt := time.Now()
	msg, err := b.SendAnimation(opts.ChatID, gotgbot.InputFileByID(content.FileID), &gotgbot.SendAnimationOpts{
		ReplyParameters:       replyParams,
		ParseMode:             parseMode,
		ReplyMarkup:           opts.Keyboard,
		Caption:               content.Text,
		ShowCaptionAboveMedia: content.CaptionAbove,
		ProtectContent:        opts.IsProtected,
		DisableNotification:   opts.NoNotif,
		MessageThreadId:       opts.ThreadID,
	})
	return resolveSendResult(msg, err, opts.ChatID, "animation", sentAt)
}
```

### Greeting media extraction (Animation before Document)
```go
// Source: this research (S-01a); chain of fuku/utils/content/content.go:118-140
func extractGreetingMedia(m *gotgbot.Message) (fileID string, dataType int) {
	if m != nil && m.Animation != nil {
		return m.Animation.FileId, db.ANIMATION
	}
	return extractMediaFromReply(m) // sticker, document, photo, audio, voice, video, video note
}
```
(The sticker-before-animation order of the legacy chain is kept by calling it second only when there is no animation; an animation message never has a sticker.)

### Refusal classifier
```go
// Source: this research (S-02); classifier reused from fuku/modules/settings_gate.go:256-262
func welcomeDefinitelyNotSent(err error) bool {
	if errors.Is(err, media.ErrNoPermission) || isTelegramClientError(err) {
		return true
	}
	var tgErr *gotgbot.TelegramError
	return errors.As(err, &tgErr) && tgErr != nil && tgErr.Code == 429
}
```

### Eligibility predicate
```go
// Source: this research (S-01b); constants fuku/db/db.go:34-41 plus ANIMATION
func CaptionAboveEligible(msgType int) bool {
	return msgType == db.PHOTO || msgType == db.VIDEO || msgType == db.ANIMATION
}
```

### Scratch reproductions (deleted afterwards, `git status` clean)
1. Composite-key conditional insert on SQLite: eight goroutines, same key, `Clauses(clause.OnConflict{DoNothing: true}).Create(...)`, `winners=1` both with and without `autoIncrement:false`.
2. Compile probe: `gotgbot.SendDocumentOpts{ShowCaptionAboveMedia: true}` -> `unknown field ShowCaptionAboveMedia in struct literal of type gotgbot.SendDocumentOpts`.
3. Baseline: `go test -tags testtools -count=1 -run '^(TestExtractMedia|TestExtractMediaFromReply)$' ./fuku/utils/content` -> `ok`.

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| GIF stored as `DOCUMENT`, sent with `sendDocument` (cannot place a caption) | `ANIMATION` (9), `sendAnimation` with `show_caption_above_media` | this phase (D-08) for GIFs saved from now on | old GIFs unchanged, no switch until re-set |
| Rules are text only, `/rules` sends text | one message: media with caption, buttons under it | this phase | `/rules`, deep link and Full preview share one builder |
| Join dedupe by Redis `SETNX` (15 s, fails closed) | durable per-member record (`greeted_members`) while Greet once is on | this phase | works across replicas and without Redis |
| Caption always below media | `show_caption_above_media` (Bot API 7.4, May 2024) for photo, video, animation | Bot API 7.4 [CITED: telegram-bot-api-7.4 changelog] | old clients ignore it and show today's layout (A6) |

**Deprecated/outdated:** Phase 11's sentence "a sticker, or text over a sticker, is allowed" in `11-UI-SPEC.md` (the code holds a stored sticker's text to 1024 like every kind); AGENTS.md's "GIF stored as a document" for new GIFs; Pitfall 22's "cached" premise (reads are uncached).

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | `show_caption_above_media` renders the caption above photo, video and animation on current Android, iOS, Desktop and Web | S-01 | the switch would do nothing visible; Full preview shows it to the owner first; owner's `12-SPIKE.md` check |
| A2 | `document.file_id` of an animation message equals its `animation.file_id` | S-01 | none for behaviour: the new rule stores `Animation.FileId`; old documents are never re-sent as animations |
| A3 | A caption is limited to 1024 UTF-16 units after entity parsing and Telegram answers 400 above it (counting unit is Phase 11 A1) | S-03 | the editor's refusal could be a unit off for astral characters near the limit |
| A4 | A Telegram 429 means the request was not processed (so a give-back is safe) | S-02 | a delivered welcome could be sent again at the member's next join; only on a 429 with a delivered message, which Telegram does not do |
| A5 | ~100 bytes per `greeted_members` row including its PK index | S-02 | storage estimate only |
| A6 | An older client ignores `show_caption_above_media` and shows the caption below | UI-SPEC | cosmetic: today's layout |
| A7 | `sendAnimation` with a `file_id` captured from an animation message works and loops as a GIF | S-01 | GIFs set after this phase fail to send; Full preview shows it to the owner; spike |
| A8 | PostgreSQL `INSERT ... ON CONFLICT DO NOTHING` of a key a concurrent in-flight transaction is inserting waits and then reports 0 rows | S-02 | two replicas could both welcome; proven by `TestGreetedMemberClaimConcurrent` in CI, not by this session |
| A9 | An incoming rich or plain private message populates `Text` (Phase 11 A9) | S-05 | unchanged from Phase 11 |
| A10 | UI-SPEC A1 (Rules set = text, media or buttons), A2 (right of the Media under text switch = the page's `EditRight`), A3 (Remove/reset leave the two switches), A4 (icon-only switch labels, amendment M1) and A12 (Rules Full preview in the group's language even when private rules is on) are the owner's to confirm at plan review | UI-SPEC | the UI-SPEC already lists them; each is one constant or one test |

**Needs user confirmation before it becomes locked:** A10 only (carried over from the approved UI-SPEC, not new). A1, A3, A7 are covered by the owner's live checklist.

## Open Questions (RESOLVED)

1. **What value is the animation type, and how does an old reader treat it?**
   - **RESOLVED:** `db.ANIMATION int = 9`; an old replica sends it as text only through `default:` (sender.go:99-101), documented as a rolling-deploy caveat; no `CHECK` exists on the columns.
2. **Do notes, filters and pins also store GIFs as animations?**
   - **RESOLVED:** no. `extractMediaFromReply` is untouched; `ExtractMedia` and `ExtractWelcome` use `extractGreetingMedia`. A test pins that `/save` and `/filter` still store `DOCUMENT`.
3. **Does a GIF set through `/setwelcome` by reply and through the editor both become an animation?**
   - **RESOLVED:** yes, one function (`extractGreetingMedia`) behind both; `TestSettingsEditMediaParity` keeps comparing editor and command and now expects `db.ANIMATION`.
4. **Where exactly does the greet-once claim sit?**
   - **RESOLVED:** inside `SendWelcomeMessage`, after the `ShouldWelcome` check and before the render/send (S-02a): the one function the `chat_member` path, the service-message path and `finishCaptchaPass` all call.
5. **How is a definite Telegram refusal told from an unknown outcome?**
   - **RESOLVED:** `welcomeDefinitelyNotSent`: `errors.Is(err, media.ErrNoPermission)`, `isTelegramClientError(err)` (4xx except 429), or a `TelegramError` with code 429. Unknown (transport, timeout, 5xx, unreadable answer) keeps the row. If the owner wants literal reuse of `isTelegramClientError`, drop the 429 clause; the only cost is a lost welcome for a member whose first send hit a flood limit.
6. **What if the claim itself fails (database error)?**
   - **RESOLVED:** fail closed: log at Error, send nothing (a missing welcome beats a double one, D-11).
7. **Does the atomic `last_msg_id` swap belong in this phase?**
   - **RESOLVED:** yes, as one small last plan that can be dropped without affecting EDIT-07..09 (S-02): transaction with `clause.Locking`, same code on SQLite; PostgreSQL concurrency test `TestSwapCleanMsgIDConcurrent` joins the integrity list. If dropped, AGENTS.md records the known race.
8. **Is a media-only or buttons-only Rules "set"?**
   - **RESOLVED:** yes (UI-SPEC A1) through `RulesSettings.HasContent()`, used by `/rules`, the deep link, Full preview, the Main line and the `{rules}` button gate.
9. **How does `/rules` stay byte-identical for old rows?**
   - **RESOLVED:** the shared sender keeps the legacy `SendMessage` call for `FileID == ""` (previews on, caller's reply parameters, keyboard only with buttons) and uses `media.Send` only with media; a characterization test recorded against today's code gates the refactor (S-03a).
10. **Which keys does the check use for the Rules over-limit refusals, and what is the no-media 4096 case?**
    - **RESOLVED:** with media: `settings_edit_refuse_text_rules_long` (text) and `settings_edit_refuse_media_rules_long` (media). Without media and over 4096: the existing `settings_edit_refuse_too_long_filled` stays (UI-SPEC table row 3 "unchanged"; the new key says "the limit with media", wrong here). The UI-SPEC check 8 phrase "A Rules refusal never carries the wording of `settings_edit_refuse_too_long_filled`" is tested for the with-media refusals only.
11. **How does the `tg` control get its "refused before any I/O" behaviour?**
    - **RESOLVED:** a control-existence check in `settingsCheckFields` plus an optional `settingsControl.Validate`, set to "0 or 1" on the editor switches (S-05). The same hook makes `t=o` on Goodbye or Rules and `t=e|d` on Rules refuse early.
12. **How are the five switch keys limited in `settings_locale_test.go`?**
    - **RESOLVED:** table rows `{26, 26}` (template as written); the binding rendered 20 is a new check in `TestSettingsLabelLengths` replacing `{state}` by one icon (S-05). The four renamed line keys keep their old limits (20/28, 20/28, 28/40, 28/40).
13. **What if a rules row has media and a caption over 1024 (via `/setrules` after media, or a backup import)?**
    - **RESOLVED:** no new command refusal (no new locale key); the member-facing senders retry once as text-only on a definite 4xx (S-03 "Runtime hardening"); Full preview does not degrade, so the owner sees the failure alert.
14. **Does `/welcome noformat` and `/goodbye` display pass the flag?**
    - **RESOLVED:** yes, both display paths (greetings.go:404 and :411) pass the stored flag so the owner sees the real layout; `noformat` is raw markup but the placement is harmless.
15. **What does a Rules `media_type` of 0 mean?**
    - **RESOLVED:** no media; `media.Send` already sends type 0 as text (`case db.TEXT, 0:`), the page treats `FileID == ""` or an unnamed type as not set, and Remove writes `file_id ''` and `media_type 0`.
16. **Does Greet once touch bots?**
    - **RESOLVED:** no special case; `ProcessSingleJoin` welcomes other bots today and the claim follows the welcome.
17. **Does the greet-once record survive a chat migration (new chat ID)?**
    - **RESOLVED:** no; rows under the old ID are orphaned and members are welcomed again once. Accepted and documented; no re-key.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go | build, tests | yes | go1.26.0 linux/amd64 | none needed |
| gcc / CGO | `go-sqlite3` in tests | yes | `/usr/bin/gcc` | none needed |
| golangci-lint | `make lint` | yes | 2.5.0 (built with go1.25.1) | CI runs the real gate; run locally with a Go 1.26 build as in Phase 11 |
| Node 22 | captcha page script test (untouched) | yes | v22.23.4 | not needed |
| redis-server / psql / pg_isready | live Redis / PostgreSQL | binaries present, no servers configured (`DATABASE_URL` and `FUKU_TEST_DATABASE` unset in this session) | - | miniredis and SQLite (repo standard); PostgreSQL guarantees run only in CI (`make test-postgres-integrity`) |
| core.telegram.org | Bot API docs | **blocked by the session network policy** (`getaddrinfo ENOTFOUND core.telegram.org`) | - | gotgbot's generated doc comments (used) and a third-party changelog mirror; Telegram-side claims stay `[ASSUMED]` until the host is allowed |

**Missing dependencies with no fallback:** none for planning or execution.
**Missing dependencies with fallback:** Telegram docs and live clients (owner's `12-SPIKE.md`); PostgreSQL (CI).

## Validation Architecture

Nyquist validation is enabled (`workflow.nyquist_validation: true` in `.planning/config.json`).

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go `testing`, build tag `testtools`, miniredis, hand-written `gotgbot.BotClient` fakes, SQLite via `internal/testdb.Run`, no mock libraries |
| Config file | none; `Makefile` target `test` (15 min per package) |
| Quick run command | the focused `-run` names of the map below (seconds each); the AGENTS family `go test -tags testtools -race -count=1 -run '^(TestSettings|TestLoadSettings)' ./fuku/modules ./fuku/i18n` takes about 3.6 minutes, so it is a per-wave command; plus `go test -tags testtools -race -count=1 ./fuku/db/greetings ./fuku/db/rules ./fuku/db/lang ./fuku/db/backup ./fuku/utils/content ./fuku/utils/media ./fuku/utils/formatting ./fuku/db/migrations` |
| Full suite command | `make test` (never a bare `go test ./...`), then `make lint`, `make check-translations`, `make generate-docs` then `make check-docs`; CI: `make test-postgres-integrity` |

### Phase Requirements to Test Map
Test function names are the plan's contract (new unless marked existing).

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| EDIT-07 | `sendPhoto`, `sendVideo`, `sendAnimation` carry `show_caption_above_media` true only when the flag is set; documents, audio, voice, sticker, video note never; flag off adds no parameter | unit | `-run '^(TestSendGreetingMessageCaptionAbove|TestCaptionAboveEligible|TestSendAnimationUsesSendAnimation)$' ./fuku/utils/media` | ❌ Wave 0 |
| EDIT-07 | A GIF through the editor and through `/setwelcome` by reply is `db.ANIMATION`; notes/filters/pins still `DOCUMENT`; a legacy document GIF stays document | unit + integration | `-run '^(TestExtractMedia|TestExtractGreetingMediaAnimation|TestExtractNoteAndFilterGifStaysDocument)$' ./fuku/utils/content`; `-run '^(TestSettingsEditMediaParity|TestSettingsEditMediaKinds)$' ./fuku/modules` | existing, updated + ❌ |
| EDIT-07 | Switch drawn exactly for photo, video, animation; not for document, audio, voice, sticker, video note, no media, legacy GIF; stored "on" ignored and not sent; returns on an eligible replacement; stale press writes nothing and shows `settings_edit_below_unavailable` | integration | `-run '^(TestSettingsEditMediaBelowEligibility|TestSettingsEditMediaBelowStalePress|TestSettingsEditMediaBelowStoredOnIgnored)$' ./fuku/modules` | ❌ Wave 0 |
| EDIT-07 | The join, leave, `/welcome`, `/goodbye`, `/rules`, private rules and Full preview all send with the flag from the stored column | integration | `-run '^(TestGreetingsMediaBelowEverywhere|TestRulesMediaBelowEverywhere)$' ./fuku/modules` | ❌ Wave 0 |
| EDIT-07 | Three switch writes are column-targeted, write a Go `false`, keep other columns, call `cache.DeleteCache`; drift and authority (Welcome/Goodbye `can_change_info` alert, Rules bot-admin notice) | integration + db | `-run '^(TestSettingsParityMediaBelow|TestSettingsDriftMediaBelow|TestSettingsAuthorityMediaBelow)$' ./fuku/modules`; `-run '^(TestSetMediaBelowColumnTargeted)$' ./fuku/db/greetings ./fuku/db/rules` | ❌ Wave 0 |
| EDIT-08 | Legacy rows: `/rules` and the deep link send the same request as today (recorded before the refactor) | integration (characterization) | `-run '^TestRulesLegacyRowSendsTodaysRequest$' ./fuku/modules` | ❌ Wave 0 (write first, green on old code) |
| EDIT-08 | `/rules` in the group, private rules (deep link) and a connected private chat show media with caption and buttons; media-only and buttons-only rules; click-to-read group message carries none | integration | `-run '^(TestRulesMediaAndButtonsInGroup|TestPrivateRulesMediaAndButtons|TestRulesMediaOnlyAndButtonsOnly|TestRulesClickToReadHasNoMedia)$' ./fuku/modules` | ❌ Wave 0 |
| EDIT-08 | Rules page: three parts, Set/Remove/Cancel/Preview for `m` and `b`, `t=z` invalid, right = bot admin (notice), group menu refusal, Remove stores `/clearrules`' row, `/clearrules` clears all three | integration | `-run '^(TestSettingsEditRulesParts|TestSettingsEditRulesRemoveParity|TestSettingsEditCallbackFields|TestSettingsAuthorityParityEdit|TestSettingsEditGroupMenuRefusal)$' ./fuku/modules` | existing, updated + ❌ |
| EDIT-08 | Typed Rules media and buttons store what the Welcome path stores for the same input; caption rule parity table (text at 1000/1024/1025 less heading, every stored kind) between Welcome and Rules; prompt 1024 with media else 3800; video note refused as typed media | integration + unit | `-run '^(TestSettingsEditRulesTypedParity|TestSettingsRulesCaptionParity|TestSettingsEditPromptMax|TestSettingsEditCheckRules)$' ./fuku/modules` | existing, updated + ❌ |
| EDIT-08 | Full preview equals `/rules` for the same row (heading in the group's language, header in the presser's), group language change seen next preview, unreadable language falls back to `uk`, language read creates no row | integration | `-run '^(TestSettingsEditRulesPreview|TestSettingsEditRulesPreviewGroupLanguage)$' ./fuku/modules`; `-run '^TestGetGroupLanguage$' ./fuku/db/lang` | existing, updated + ❌ |
| EDIT-08 | Runtime degrade: a definite 4xx on a media rules send retries once as text-only in `/rules` and the deep link, not in Full preview | integration | `-run '^TestRulesDegradesToTextOnClientError$' ./fuku/modules` | ❌ Wave 0 |
| EDIT-08 | Rules loader creates no row; writers column-targeted; `ClearRules` stores zeros; `HasContent`; `{rules}` button for media-only rules | db + unit | `-run '^(TestLoadRulesSettingsCreatesNothing|TestRulesMediaWriters|TestClearRulesStoresZeroes|TestHasContent)$' ./fuku/db/rules ./fuku/db/models`; `-run '^TestFormattingRulesButtonMediaOnly$' ./fuku/utils/formatting` | ❌ Wave 0 |
| EDIT-09 | First join welcomed, second not; welcome off -> no row; `Once` off -> no row and today's behaviour; captcha-failed and lockdown-banned joiners leave no row; the captcha-pass path claims; works without Redis | integration | `-run '^(TestGreetOnceFirstJoinOnly|TestGreetOnceWelcomeOffNoClaim|TestGreetOnceOffNoRecord|TestGreetOnceCaptchaPathAndFailure|TestGreetOnceLockdownBanNoRecord|TestGreetOnceWithoutRedis)$' ./fuku/modules` | ❌ Wave 0 |
| EDIT-09 | Concurrent `SendWelcomeMessage` for one `(chat, user)` -> exactly one welcome; definite refusal (403, 400, `ErrNoPermission`, 429) gives the row back; unknown outcome (timeout, 5xx) keeps it; claim error sends nothing | integration | `-run '^(TestGreetOnceConcurrentOneWelcome|TestGreetOnceDefiniteRefusalReleases|TestGreetOnceUnknownOutcomeKeeps|TestGreetOnceClaimErrorFailsClosed)$' ./fuku/modules` | ❌ Wave 0 |
| EDIT-09 | `ClaimGreeting`/`ReleaseGreeting` repository behaviour; PostgreSQL concurrency | db (+ PostgreSQL only) | `-run '^TestGreetedMemberClaim$' ./fuku/db/greetings`; CI `TestGreetedMemberClaimConcurrent` | ❌ Wave 0 |
| EDIT-09 | Switch `o`: Welcome only, `settingsRightAdmin`, absolute value, drift and authority, the four owner worlds through the press and the typed-reply door; preview and Full preview never claim; the page never reads `greeted_members` | integration | `-run '^(TestSettingsEditOnceSwitch|TestSettingsDriftOnce|TestSettingsAuthorityOnce|TestSettingsGateAllDoors|TestSettingsEditPreviewNeverClaims)$' ./fuku/modules` | existing (`TestSettingsGateAllDoors`) + ❌ |
| EDIT-07/08/09 | Migration: statement shape (db-free) and defaults on PostgreSQL; backup round trip carries the new columns, an old export decodes to zeros, `greeted_members` untouched by export/import/reset | unit + PostgreSQL + db | `-run '^TestRulesMediaMigrationStatements$' ./fuku/db/migrations`; CI `TestRulesMediaMigrationDefaults`; `-run '^(TestAllModulesRoundTripEveryMeaningfulField|TestOldGreetingsExportDecodesToZeros|TestGreetedMembersStayOutOfBackup)$' ./fuku/db/backup` | existing, updated + ❌ |
| UI | Row budget (8 private / 9 group) at worst case for Welcome, Goodbye, Rules and the grid fill; callback bytes of every new button at worst-case IDs (largest 45) with `Omitted` 0; `tg` unknown `t`, `t=o` on g/r, `t=e|d` on r, `v=2`, missing `t`, extra key refused before any `getChatMember` | integration | `-run '^(TestSettingsEditSwitchGrid|TestSettingsEditCallbackBytes|TestSettingsControlValidate)$' ./fuku/modules` | existing, updated + ❌ |
| UI / locale | 13 new + 4 renamed rows, exactly 199 `settings_` keys, five switch labels <= 20 rendered with `✅` and `❌` in 8 locales, tokens, hints, Rules hint keeps `<code>/rules</code>`; pages fit 3800 units in 8 locales with a 64-rune title; privacy (no Staff Group on any new screen) | unit + integration | `-run '^(TestSettingsLocaleKeys|TestSettingsLabelLengths|TestSettingsEditLocaleTokens|TestSettingsHelpFitsOneMessage)$' ./fuku/i18n`; `-run '^(TestSettingsEditPagesFitEveryLocale|TestSettingsEditScreensFitEveryLocale|TestSettingsStaffPrivacy)$' ./fuku/modules`; `make check-translations` | existing, updated |
| Success 4 | Rules, welcomes and goodbyes saved before the phase look exactly as before: legacy `/rules` request, flag-off requests carry no `show_caption_above_media`, an old export imports, an old GIF document sends with `sendDocument` | integration | the characterization test above plus `-run '^TestLegacyGreetingsRequestsUnchanged$' ./fuku/modules` | ❌ Wave 0 |

### Sampling Rate
- **Per task commit:** the focused `-run` of the task's tests (under 30 s each), plus the package-level run of any `fuku/db/*` or `fuku/utils/*` package the task touched.
- **Per wave merge:** `go test -tags testtools -race -count=1 ./fuku/modules ./fuku/i18n ./fuku/db/greetings ./fuku/db/rules ./fuku/db/lang ./fuku/db/backup ./fuku/db/migrations ./fuku/utils/...` plus `make lint`.
- **Phase gate:** `make test`, `make lint`, `make check-translations`, `make generate-docs` then `make check-docs`, CI `make test-postgres-integrity`, and the owner's live checklist `12-SPIKE.md` (deferred like Phases 7 to 11 unless the owner says otherwise). Baseline: before this phase the settings family is green (modules 188.6 s, i18n 5.2 s, see Metadata).

### Wave 0 Gaps
- [ ] **Characterization first:** `fuku/modules/rules_legacy_request_test.go` records the fake's `sendMessage` params for `/rules` (private rules off and on) and the deep link for an old row, and a legacy greeting/goodbye request with the flag off; commit it green against untouched code before any refactor.
- [ ] `fuku/utils/media/sender_test.go`: caption-above and animation tests; `fuku/utils/content`: greeting extraction tests.
- [ ] `fuku/db/greetings/greeted_test.go` and `testmain_test.go` (add `&models.GreetedMember{}`); writers' tests; `fuku/db/rules` loader/writer tests; `fuku/db/lang` `TestGetGroupLanguage`.
- [ ] `fuku/db/backup`: `testmain_test.go` gains `GreetedMember`; round-trip fields; `TestOldGreetingsExportDecodesToZeros`, `TestGreetedMembersStayOutOfBackup`.
- [ ] `fuku/db/migrations`: `rules_media_migration_test.go` (statements, and the PostgreSQL defaults test).
- [ ] `fuku/modules/test_harness_test.go`: `&models.GreetedMember{}`; `editCleanup` deletes `greeted_members`; a join-driving helper reusing the `TestMemberJoinAndLeaveSendConfiguredGreetings` pattern; fake answers for `sendAnimation` already exist (`settingsEditFakeSends`, `settings_edit_preview_test.go:27-37`).
- [ ] `fuku/i18n/settings_locale_test.go`: 13 rows, 4 renames, five-cell rendered check, new token rows.
- [ ] Makefile: add the new PostgreSQL test names and `./fuku/db/greetings` to `test-postgres-integrity`.
- [ ] `AGENTS.md` (S-07), `make generate-docs`.
- [ ] Owner's `12-SPIKE.md`: the live items of the S-01 table.

## Security Domain

### Applicable ASVS Categories (level 1)

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | no (Telegram identifies the user) | n/a |
| V3 Session Management | limited (the Phase 11 wait is unchanged) | per-user key, nonce claim, TTL |
| V4 Access Control | **yes** | every press and typed reply keeps the live `getChatMember` + `settingsGate` + part right; Rules `m`/`b` need the bot-admin right like `x`; `o` needs the admin check like `/welcome on\|off`; `u` the page's `EditRight`; strict control/value validation before I/O |
| V5 Input Validation | **yes** | `content.ValidButtonURL` (http/https + host) for Rules buttons; caption/text length via the shared predicates; `tg` `t`/`v` hook; media only from the kinds `ExtractMedia` accepts |
| V6 Cryptography | no | n/a |
| V7 Error handling / logging | yes | log chat/user IDs and outcome only; never text, captions, button names, links or file IDs; Telegram wording never reaches the user |
| V8 Data protection | yes | `greeted_members` holds IDs and a timestamp only, is never echoed, exported or shown; kept for good by decision (D-11) |

### Known Threat Patterns for this stack

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Forged `t=o` on Goodbye/Rules or `t=m` on a non-Rules page | Tampering | control-existence hook before I/O; strict part `Validate` |
| Forged `c`/`u` aiming a switch at another group | Elevation of privilege | unchanged pipeline: live `getChatMember` of the presser in that group, owner gate, right |
| A join flood inflating `greeted_members` | Denial of service | only a real welcome claims (not captcha-failed, not lockdown-banned); the write rate equals the welcome rate; PK blocks duplicates |
| Double welcome across replicas | Tampering (integrity) | durable conditional insert, not Redis; fail closed on a claim error |
| Permanent suppression of a welcome by a refused send | Denial of service (UX) | give-back on a definite refusal by code |
| Stored "on" widening an ineligible send | Tampering | eligibility predicate, structural in the sender |
| Over-long caption making `/rules` fail for every member | Denial of service (UX) | editor refusals plus the member-path degrade to text-only on a definite 4xx |
| Button URLs (`javascript:`, `file:`, scheme-less) in Rules | Tampering | `ValidButtonURL` refuses them with the button's number; the bot never fetches the link |
| A refusal revealing whether the bot is in a chat | Information disclosure | unchanged: Rules refusals follow a wait only a proven admin started; no refusal names a group, a title or the Staff Group |
| Leaking the Staff Group on new screens | Information disclosure | `TestSettingsStaffPrivacy` renders every new screen in 8 languages |
| Importing another bot's backup (foreign `file_id`) | Tampering | member-path degrade; Full preview shows the failure to the owner |

## Sources

### Primary (HIGH confidence)
- Repository files read this session: `fuku/db/db.go`, `fuku/db/models/{greetings,rules,types,chat_settings_access}.go`, `fuku/db/greetings/repository.go`, `fuku/db/rules/repository.go`, `fuku/db/lang/repository.go`, `fuku/db/backup/{backup,types}.go`, `fuku/db/backup/roundtrip_test.go`, `fuku/db/lockdown/{joiners,repository}.go`, `fuku/utils/media/sender.go`, `fuku/utils/content/content.go`, `fuku/utils/formatting/formatting.go`, `fuku/utils/keyboard/keyboard.go`, `fuku/utils/errors/errors.go`, `fuku/utils/helpers/telegram_helpers.go`, `fuku/modules/{greetings,membership,rules,rules_format,captcha,pins,settings_edit,settings_edit_actions,settings_edit_check,settings_edit_preview,settings_input_door,settings_menu,settings_pages,settings_gate}.go`, tests `settings_edit_test.go`, `settings_edit_actions_test.go`, `settings_edit_preview_test.go`, `settings_edit_check_test.go`, `settings_input_media_test.go`, `settings_edit_env_test.go`, `test_harness_test.go`, `fuku/utils/content/{content,editor_helpers}_test.go`, `fuku/i18n/settings_locale_test.go`, `fuku/db/migrations/{settings_hub_migration,captcha_mode}_test.go`, `Makefile`, `migrations/20250805200527_initial_migration.sql`, `20261029120000_add_raid_detection_ai.sql`, `20261030120000_allow_turnstile_captcha_mode.sql`, `20261104120000_add_settings_hub_tables.sql`, `locales/en.yml`, `go.mod`, `AGENTS.md`.
- gotgbot source at the pinned version (module cache): `gen_methods.go` (opts structs and serialisation), `gen_types.go` (`Message`, `Animation`), `custom_helpers.go` (`addIfValueNotZero`), `request.go` (`TelegramError`), `gen_helpers.go` (`ReplyMessage`); `gorm.io/driver/sqlite` `sqlite.go` (locking clause).
- Scratch runs this session (files deleted, `git status` clean): conditional insert winners, compile probe for the opts structs, baseline `content` tests.
- `.planning/phases/12-.../12-CONTEXT.md`, `12-UI-SPEC.md`, `.planning/REQUIREMENTS.md`, `.planning/ROADMAP.md` (Phase 12), `.planning/research/{PITFALLS,SUMMARY}.md`, `11-RESEARCH.md`.

### Secondary (MEDIUM confidence)
- Bot API 7.4 changelog via the Haskell `telegram-bot-api-7.4` package changelog (third-party mirror; `show_caption_above_media` added to `sendPhoto`, `sendVideo`, `sendAnimation`, May 28 2024).
- PostgreSQL documentation of `INSERT` (`postgresql.org/docs/current/sql-insert.html`) on unique-index blocking and `ON CONFLICT DO NOTHING` (search extract; the full concurrency section was not fetched).

### Tertiary (LOW confidence)
- Telegram-side behaviours A1, A3, A4, A6, A7 [ASSUMED]; `core.telegram.org` unreachable from this session.

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH, nothing new, versions from `go.mod`
- Architecture: HIGH, read from code and exercised by scratch runs (conditional insert, option names)
- Pitfalls: HIGH for the repo-derived ones (1-12 except Telegram rendering), MEDIUM for the Telegram-side ones (A1, A3, A6, A7)

**Baseline test run (before any change, this session):** `go test -tags testtools -race -count=1 -run '^(TestSettings|TestLoadSettings)' ./fuku/modules ./fuku/i18n` -> `ok  .../fuku/modules  188.637s` and `ok  .../fuku/i18n  5.229s` (about 3.6 minutes wall with `-race`, so the settings quick run is not a per-task command; use the focused `-run` names instead).

**Research date:** 2026-11-05
**Valid until:** 2026-12-05 (the code moves with every v1.1 phase; before planning re-check the highest migration, `settingsCoreActions`, `settingsEditPages` and the harness AutoMigrate lists if another phase merged first)
