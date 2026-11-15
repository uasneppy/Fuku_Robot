# Phase 11: Message Editor - Research

**Researched:** 2026-11-04
**Domain:** Telegram bot (Go 1.26, gotgbot v2 rc.36, GORM, Redis): a private-chat message editor for the Welcome, Goodbye and Rules messages, with a Redis-backed wait for the owner's next message, a typed-reply door, previews through the group's send path, and on/off switches
**Confidence:** HIGH for everything grounded in the repo code and in scratch runs of this session; MEDIUM for the Telegram-side behaviours that could not be probed (core.telegram.org is blocked by the session network policy, see Assumptions Log)

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

#### Where the editor works (EDIT-01, EDIT-02)
- **D-01:** Set and Remove work in the private menu only (owner's choice, 2026-11-04). In a menu opened in the group those buttons show a short note with an "Open in private" link (the existing `/start settings_<chat id>` deep link, which carries the owner gate). Preview, Full preview and the on/off switches work in both places. No typed reply is ever awaited inside a group.
- **D-02:** The pages are new settings pages registered through `registerSettingsPage` in their own files (`Welcome`, `Goodbye`, `Rules`), so `settings_pages.go` is not edited. Every page press goes through the Phase 10 pipeline (`settingsPress.verify`, owner gate included).

#### The wait for the owner's next message (EDIT-02)
- **D-03:** After Set, the owner's next private message becomes that part. The wait is one slot per person: pressing Set on another part replaces the first wait and says so (owner's choice). Cancel, `/cancel` and a 10-minute expiry end it and leave the message unchanged.
- **D-04:** The wait lives in Redis (key family `fuku:settings:input:<user>`, 10-minute TTL, value names the group, page and part), so whichever replica receives the message sees it. Without Redis the Set buttons answer that the editor is not available right now; everything else still works. The reply is read by a private-chat message handler below group 12 (group number decided in research) that returns `ext.ContinueGroups` when the sender has no wait, so no other handler is affected.
- **D-05:** The typed reply is judged like a press: a live `getChatMember` of the sender in the group named by the wait, the Phase 10 owner gate (`settingsGate`), and only then the stored change. A wait whose owner lost access is ended with a short refusal. Service identities never start or answer a wait.
- **D-12:** A reply sent after the 10-minute wait has expired is not left silent (UI checker recommendation, adopted 2026-11-04; supersedes the UI-SPEC default A3 that let it pass through). Set also writes a short-lived marker (`fuku:settings:inputdone:<user>`, about 15 minutes) that records the last wait's group and part; when the typed-reply door finds a marker but no wait it answers with a plain reply that the wait ended, naming the part and saying to press Set again. Still no timer and no shutdown drain. A user with neither a wait nor a marker is passed through untouched, so ordinary private chat with the bot is unaffected.
- **D-13:** An album that is refused (for example sent where only text is accepted) gets one refusal, not one per item: the door remembers the media group id for about 30 seconds for refused albums as it does for accepted ones.

#### What is accepted and how it is checked (EDIT-03, EDIT-04, EDIT-05, EDIT-11)
- **D-06:** Text keeps its formatting and placeholders (`{first}`, `{mention}` and the rest) and is stored exactly as `/setwelcome`, `/setgoodbye` and `/setrules` store it (entities converted the same way, the same stored form), so a saved message shows at the next real join, leave or `/rules`.
- **D-07:** Before saving, the bot does a dry run (owner's choice): it builds the message with the placeholders filled using the owner's own name and a worst-case-length name, and refuses it with the reason, leaving the wait open, if Telegram would reject it: text over 4096 characters (1024 as a caption), a bad button syntax or URL, or a media item the welcome cannot send. The refusal names the cause in the owner's language.
- **D-08:** Media accepts exactly one item of a kind the welcome already supports (photo, video, GIF, document, sticker, audio, voice). From an album one item is kept and the owner is told so. URL buttons use the bot's existing button syntax, no new syntax.

#### Remove and Preview (EDIT-01, EDIT-06)
- **D-09:** Remove on the Welcome or Goodbye text goes back to the default text, like `/resetwelcome` and `/resetgoodbye`. Remove on the Rules text clears it, like `/clearrules`. Remove on a media item or on the buttons simply removes it (owner's choice: same as the commands).
- **D-10:** Preview shows one part. Full preview sends the whole message exactly as members will see it, through the same send path the group uses, filled with the owner's own name and the group's title. It is sent only to the owner in the private chat, never into the group.

#### Switches on Welcome and Goodbye (EDIT-10)
- **D-11:** Welcome and Goodbye pages carry their on/off switch and "Delete last welcome/goodbye". Presses use one apply function per setting beside the existing command (`/welcome`, `/goodbye`, `/cleanwelcome`, `/cleangoodbye`), so the menu and the command store the same row and invalidate the same cache, as the project rule "one setting, one code path" requires. They send absolute target values, so a double tap is harmless.

### Claude's Discretion
- The handler group number of the private input consumer (below 12), the exact Redis key names, the wording of every prompt and refusal, the page layout and icons (settled in the UI contract), how Telegram entities are converted for storage (settled by a short spike against the `/setwelcome` code path first), and how an album is cut to one item.

### Deferred Ideas (OUT OF SCOPE)
- Typing a number into a stepper through the same input state (the state is reusable; no stepper uses it here).
- Editing a message from a menu opened in the group (the owner chose private only).
- Posting editor changes to the log channel (MENU2-01).

Also out of scope for this phase (CONTEXT `<domain>`): media and URL buttons for Rules, `/rules` showing them, "Media under text" and "Greet once" (Phase 12; the Rules page offers the text part only and is built so Phase 12 adds the other parts without reworking it); the member requirement rules, deletion pages and the remaining settings pages (Phases 13 to 19).
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| EDIT-01 | From the menu an admin opens Rules, Welcome or Goodbye and sees which parts are set, with a button to set or remove each part | S-08 (pages from one parts table, side-effect-free reads), S-06 (rights), Pitfalls 2, 9, 10 |
| EDIT-02 | After pressing a part's button, the bot waits for the next private message and stores it; Cancel, `/cancel`, 10-minute expiry; any replica | S-02 (group 1), S-03 (Redis scripts), S-10 (door algorithm), Validation rows EDIT-02 |
| EDIT-03 | Text keeps formatting and placeholders; too long (4096 / 1024) refused, wait stays open | S-01 (stored form), S-04 (validation, dry run), Pitfalls 4, 12 |
| EDIT-04 | Media accepts one item of a supported kind; album keeps one and says so | S-01 (media storage), S-09 (album), Assumptions A3, A4 |
| EDIT-05 | URL buttons use the existing syntax; bad syntax or URL refused, wait stays open | S-01 (button parse), S-04 (URL rules) |
| EDIT-06 | Preview per part; Full preview through the same send path | S-07 (send path, preview builder), Pitfall 11 |
| EDIT-10 | Welcome/Goodbye on/off and Delete last stored exactly as the commands store them | S-05 (apply functions, cache keys), S-06 (rights) |
| EDIT-11 | A saved message is stored exactly as `/setwelcome`, `/setgoodbye`, `/setrules` stores it; a message Telegram would refuse is refused before saving | S-01, S-04, Validation parity tests |
</phase_requirements>

## Project Constraints (from CLAUDE.md / AGENTS.md)

Directives the plans must keep (copied in substance from `.claude/CLAUDE.md` and `AGENTS.md`, treated as locked):

- Stack stays Go 1.26.0, gotgbot v2, GORM (SQLite in tests), Redis; no new dependency.
- Migrations are append-only; this phase needs none (no schema change, see S-05).
- Every write must `cache.DeleteCache` the keys it affects (S-05 finds that greetings and rules reads are uncached, so the invalidation is a documented no-op for reads, not an omission).
- New commands go through `helpers.WrapCommand` (the `/cancel` command, S-02). Locale keys go in all locale files: the repo has **8** language files (`en es fr hi id pt ru uk`) plus `locales/config.yml`; `AGENTS.md` still says "7" in two places and should be corrected in the same commit.
- Fire-and-forget goroutines start with `defer error_handling.RecoverFromPanic(...)`. The door and every press handler recover from panics.
- Tests use real fixtures (miniredis, hand-written `gotgbot.BotClient` fakes), no mock libraries, always `-tags testtools`, run through `make test`; assert observable behaviour (reply sent, row persisted, gate enforced), never literals or test-double internals.
- Callback data only through `callbackcodec` (64-byte cap); never `strings.Split` it. A settings page is one `registerSettingsPage` call from its own file's `init`.
- Handler rules: watchers return `ext.ContinueGroups`, commands `ext.EndGroups`; a new handler group is documented in `AGENTS.md` in the same commit; group 12 must stay last.
- Security: the per-group admin check is never skipped; the Staff Group is never named in any settings screen in any language (`TestSettingsStaffPrivacy`, D-21 and D-32).
- Never log message text, button names or URLs; log IDs, part and outcome only.
- Update `AGENTS.md` in the same commit as any change that invalidates a rule (new handler group, new Redis key families, new callback codes, new right, new command).

## Summary

The editor is mostly assembly of parts that already exist, but the spike turned up **seven facts that differ from what the UI contract and CONTEXT assume**, and the plans must be built on them: (1) the greetings and rules **reads create rows** (`checkGreetingSettings`, `checkRulesSetting`) and the **database default makes Goodbye ON** for any chat that has a row, so the UI contract's "Goodbye defaults to Off" is wrong for real groups (S-05, Pitfalls 1-2); (2) `/setrules` plus `/rules` **double-escapes** a rules text that has no formatting tags and contains `'`, `&`, `<`, `>` or a double quote (verified by a scratch run, S-01, Pitfall 3); (3) the settings pipeline's `settingsCheckFields` **does not require** a page action's extra fields, so "a missing or unknown `t` is refused before any I/O" needs a small additive hook (S-08); (4) the dispatcher's `ext.ContinueGroups` means "continue the **current group**", and gotgbot's command handler reads `msg.GetText()`, which **includes a caption**, so a media caption `/welcome` is a command (S-02); (5) `formatting.FormattingReplacer` makes a **Telegram call for `{count}`** and a **database read (that can create a row) for `{rules}`**, so the dry run needs a pure fill function (S-04); (6) a media album's items arrive as parallel updates, so the "kept item" is the first to win a `SET NX`, and siblings must be silenced **before** the wait is claimed or the D-12 marker would make each one answer "the wait ended" (S-09); (7) the existing `settingsEnv` toolkit has no way to send entities, media or albums in private, and its `otherReplica()` only loads the modules `LoadSettings` registers, so the door must be registered inside `LoadSettings` to get the two-replica test for free (S-11).

The stored form is settled: a private message becomes stored text by exactly the commands' code path, `tgmd2html.MD2HTMLButtonsV2(msg.OriginalMDV2())` for Welcome and Goodbye and `tgmd2html.MD2HTMLV2(msg.OriginalMDV2())` for Rules; media by `extractMediaFromReply` (a GIF is stored as `db.DOCUMENT`); buttons by `MD2HTMLButtonsV2` plus the existing URL filter, which the editor turns into refusals. The handler group of the typed-reply door is **1** (after group-0 commands, after the group `-1` users tracker, before every group that could end a private update). The wait is one Redis hash per user claimed with the repo's own claim/finish/release Lua pattern (`challenge.go`), verified on miniredis in this session, so a refusal never consumes the wait and an album or a double delivery writes once.

**Primary recommendation:** Build one table-driven editor (`settings_edit*.go`) over three small changes to shared code: column-targeted greetings writes plus side-effect-free reads in `fuku/db/greetings` and `fuku/db/rules`, a pure `FillPlaceholders` and exported length/URL/media helpers in `formatting` and `content` (parity by construction, the commands call the same helpers), and a Redis wait store with a group-1 door registered from `LoadSettings`; then prove parity with command-versus-editor row comparisons, two-instance miniredis tests and the TestSettingsGateAllDoors table.

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Editor pages (state lines, Set/Remove/Preview/switch buttons) | Bot handlers (`fuku/modules`, settings pipeline) | Database repos (`fuku/db/greetings`, `fuku/db/rules`) | Pages are settings pages drawn from a part table; state comes from side-effect-free repo reads |
| Wait for the next message (state, expiry, claim) | Redis (operational keys outside `fuku:cache:`) | Bot handlers (door) | Must survive replicas and restarts; TTL is the expiry; claim is a Lua compare-and-set |
| Typed-reply door (authority, validation, write) | Bot handlers (group 1) | Telegram API (live `getChatMember`, `getChat`) | Authority is the live record plus `settingsGate`, never cached state |
| Stored form (entities to HTML, media type, buttons) | Library code (`content`, `gotg_md2html`, gotgbot `OriginalMDV2`) | Database repos | One conversion path shared with the commands |
| Dry run / validation (length, URL, media kind) | Pure Go (`formatting`, `content`) | Telegram API only at Full preview | Local, sends nothing; Telegram remains the backstop |
| Preview and Full preview | Bot handlers (builder over `media.SendGreeting`) | Telegram API (send to the presser's private chat) | Same send function as the group; target chat is an argument |
| On/off and Delete-last switches | Bot handlers (apply functions beside commands) | Database repos (`SetWelcomeToggle`, `SetCleanWelcomeSetting` ...) | One setting, one code path |
| Rights per control | Bot handlers (`settingsRightRefusal`) | Telegram API (the press's own live member record) | Menu refuses what the matching command refuses |

## Standard Stack

### Core (all existing, no new dependency)
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `github.com/PaulSonOfLars/gotgbot/v2` | `v2.0.0-rc.36.0.20260919140833-240296efadb4` [VERIFIED: go.mod] | Updates, `Message.OriginalMDV2`, `GetText`, `MediaGroupId`, send/edit methods | The repo's Telegram client |
| `github.com/PaulSonOfLars/gotg_md2html` | `v0.0.0-20260314092343-61634cbfb443` [VERIFIED: go.mod] | `MD2HTMLButtonsV2`, `MD2HTMLV2`, `StripHTMLV2` | The converter `/setwelcome`, `/setgoodbye`, `/setrules` already use |
| `github.com/redis/go-redis/v9` | v9.22.0 [CITED: CLAUDE.md Technology Stack] | Wait store: `redis.NewScript`, pipeline, `GetDel`, `SetNX` | Already used by staff cards, captchaweb, raid counters |
| `github.com/alicebob/miniredis/v2` | v2.39.0 [CITED: CLAUDE.md] | Tests: Lua scripts, `FastForward` for TTL | Repo standard; `HGETALL`, `GETDEL`, `PTTL`, `EVAL` supported [VERIFIED: miniredis cmd_*.go registrations] |
| `gorm.io/gorm` | v1.31.2 [CITED: CLAUDE.md] | Column-targeted writes through `upsertGreetingSettings` | Existing repository pattern |

### Supporting
| Library | Purpose | When to Use |
|---------|---------|-------------|
| `fuku/utils/callbackcodec` via `encodeCallbackData` / `decodeCallbackData` | All new button data | Every settings button |
| `fuku/i18n` (`ctxTr`, `settingsText`) | Strings in the sender's language | Every message |
| `fuku/utils/error_handling.RecoverFromPanic` | Door and handlers | Top of every new handler |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| Redis hash + Lua claim | `GETDEL` of a string | `GETDEL` consumes before the checks; a refusal would then need a put-back and loses the remaining TTL; the hash claim is the repo's proven pattern |
| Door at group `-1` | Group 1 | `-1` runs before commands and must itself exclude commands; group 1 gets "a command always wins" for free (S-02) |
| `sendAnimation` for GIFs | Keep `sendDocument` (parity) | Changing it also changes the join path; out of scope unless the owner reports a bug (A3) |

**Installation:** none. **Version verification:** versions are the pinned ones in `go.mod`; nothing is installed.

## Package Legitimacy Audit

No external package is installed or added by this phase. `go.mod` and `go.sum` are untouched. The `package-legitimacy` seam was not run because there is nothing to check.

| Package | Registry | Age | Downloads | Source Repo | Verdict | Disposition |
|---------|----------|-----|-----------|-------------|---------|-------------|
| (none) | - | - | - | - | - | - |

**Packages removed due to [SLOP] verdict:** none
**Packages flagged as suspicious [SUS]:** none

## Settled Spike Findings (the roadmap's "short spike first")

Evidence tags: file:line with the value quoted verbatim, or "scratch run" for experiments executed this session (scratch test files were deleted again; `git status` is clean; reproductions are in Code Examples).

### S-01. Does a private message store in the same form as the commands? Yes, through the same two calls

**Welcome and Goodbye text.** `ExtractWelcome` stores `result.Text, _buttons = tgmd2html.MD2HTMLButtonsV2(rawText)` for a text command (`fuku/utils/content/content.go:160-163`) and, for a replied-to text message with no extra args, the same call over `replyMsg.OriginalMDV2()` (`content.go:164-172`, `setRawText` `content.go:255-265`: `*rawText = replyMsg.OriginalMDV2()`). A message typed in private is exactly that "replied-to text" case without the reply, so the editor's text part is `html, btns := tgmd2html.MD2HTMLButtonsV2(msg.OriginalMDV2())` [VERIFIED: content.go:160-172, 255-265]. A caption is `msg.OriginalCaptionMDV2()` (`content.go:244-248`), but the editor ignores captions (UI-SPEC A5).

**Rules text.** `rules.SetChatRules(chat.Id, tgmd2html.MD2HTMLV2(text))` where `text = msg.ReplyToMessage.OriginalMDV2()` or the part after the first space of `msg.OriginalMDV2()` (`fuku/modules/rules.go:190-206`). The editor stores `tgmd2html.MD2HTMLV2(msg.OriginalMDV2())` [VERIFIED: rules.go:190-206].

**Entities and UTF-16.** `Message.OriginalMDV2()` is `getOrigMsgMDV2(utf16.Encode([]rune(m.Text)), m.Entities)` and slices every entity in UTF-16 units (`gotgbot/formatting.go:48-50`, `149-164`, `utf16.Decode(utf16Data[...])`), so emoji do not shift markup [VERIFIED: gotgbot formatting.go]. Scratch run: text `"Hi 😀 bold link"` with a bold entity at offset 6 length 4 and a text_link entity at offset 11 length 4 gave `MD="Hi 😀 *bold* [link](https://e.com)"` and `HTML="Hi 😀 <b>bold</b> <a href=\"https://e.com\">link</a>"` (the emoji is two UTF-16 units and the offsets landed correctly) [VERIFIED: scratch run].

**Plain markdown characters are NOT escaped by `OriginalMDV2`** (`getOrigMsgMDV2` returns the raw text for the unformatted parts, `formatting.go:150-152, 162`). So a typed `*x*` with no bold entity is stored as `<b>x</b>`. That is exactly what the commands do today and is part of "stored exactly as the commands store it" (Pitfall 12).

**Text containing `buttonurl://` lines.** `MD2HTMLButtonsV2` removes `[Name](buttonurl://...)` from the text and returns it as buttons (`md2htmlV2.go:301-342`). Scratch run: `"*bold* and _it_ [x](buttonurl://https://e.com) tail"` returned `html="<b>bold</b> and <i>it</i>  tail"` and `btns=[{Name:x Type:url Content:https://e.com ...}]`. So the editor detects "text contains button lines" as `len(btns) > 0` and refuses it (UI-SPEC `settings_edit_refuse_text_buttons`), with no regex of its own [VERIFIED: scratch run + md2htmlV2.go:311-342].

**Buttons part.** `[Rules](buttonurl://https://example.com)\n[Chat](buttonurl://https://t.me/x:same)` returned `html=""` and two buttons, the second `SameLine:true` with content `https://t.me/x` (the `:same` suffix is stripped, `md2htmlV2.go:329-332`) [VERIFIED: scratch run]. `ButtonV2.Style` (`buttonurl#primary:...`) is parsed but `ConvertButtonV2ToDbButton` keeps only `Name`, `Url`, `SameLine` (`content.go:269-279`), so styles are silently dropped, as they are today.

**What `preFixes` does that the editor must mirror or turn into a refusal** (`content.go:200-239`):
```go
if *dataType == db.TEXT && textRuneCount > 4096 {            // content_text_too_long
} else if *dataType != db.TEXT && textRuneCount > 1024 {     // content_caption_too_long
} else { ... buttonUrlFixer ... *text = strings.Trim(*text, "\n\t\r ") ... }
```
`buttonUrlFixer` keeps a button only when `err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""` (`content.go:220-229`) and an empty button name becomes the default label (`content.go:214-218`, `"Greeting"` passed by `ExtractWelcome`, `content.go:178`). The count is `utf8.RuneCountInString` of the **stored HTML** (`content.go:203`).

**Media storage.** `extractMediaFromReply` (`content.go:118-140`) returns, in this order, Sticker (`db.STICKER`), Document (`db.DOCUMENT`), Photo (`replyMsg.Photo[len(replyMsg.Photo)-1].FileId`, `db.PHOTO`), Audio, Voice, Video, then `replyMsg.Animation.FileId, db.DOCUMENT` and VideoNote. **A GIF is stored as `db.DOCUMENT`** (UI-SPEC A1 confirmed from the code). Telegram also sets `document` on an animation message ("For backward compatibility, when this field is set, the document field will also be set", `gotgbot/gen_types.go:8026`), and `Document` is checked first, so an animation stores `Document.FileId`; either way the type is DOCUMENT. Constants: `TEXT int = 1`, `STICKER int = 2`, `DOCUMENT int = 3`, `PHOTO int = 4`, `AUDIO int = 5`, `VOICE int = 6`, `VIDEO int = 7`, `VIDEO_NOTE int = 8` (`fuku/db/db.go:33-42`). Default texts: `DefaultWelcome = "Привіт, {first}, як справи?"`, `DefaultGoodbye = "Шкода, що ти йдеш, {first}"` (`db.go:44-47`).

**Stored columns** (`fuku/db/greetings/repository.go:183-198`): `SetWelcomeText` writes `welcome_text`, `welcome_btns` (`models.ButtonArray(buttons)`), `welcome_type`, `welcome_file_id` in one `upsertGreetingSettings` call; the Goodbye twin writes the `goodbye_*` columns (`repository.go:215-230`). Toggles: `welcome_enabled`, `goodbye_enabled`, `welcome_clean_old`, `goodbye_clean_old` (`repository.go:200-212, 232-244, 274-286, 302-314`). Rules: column `rules` (`fuku/db/rules/repository.go:42-52`). Buttons are `models.Button{Name, Url, SameLine}` (`fuku/db/models/types.go:20-24`) stored as a jsonb array whose `Value()` writes `"[]"` when empty (`types.go:42-47`).

**Decision:** the editor calls the same converter functions; the three commands and the editor share exported helpers added to `content` (S-04) so parity is by construction, and a parity test per part compares the stored rows.

### S-02. Handler group of the typed-reply door: **group 1**

How the dispatcher walks groups (`gotgbot/ext/dispatcher.go:280-328`): groups in numeric order; in a group the handlers in registration order; a handler that **matches** and returns `ext.ContinueGroups` makes the loop `continue` **inside the same group** (the next handler of that group is tried, then the next group); `ext.EndGroups` stops everything; **any other return (nil or an error) breaks out of the current group and moves to the next group**. `handlers.NewMessage` skips bot senders unless `SetAllowBot` and skips edited messages, channel posts and business messages unless enabled (`handlers/message.go` `CheckUpdate`). `handlers.NewCommand` decides "is a command" on `msg.GetText()` and `msg.GetEntities()` (`handlers/command.go` `checkMessage`: `text := msg.GetText()`), and `Message.GetText()` returns `m.Caption` when non-empty else `m.Text` (`gotgbot/custom_helpers.go:25-30`). So **a media message whose caption starts with `/welcome` is a command**.

What sits in private chat today (from the registrations read this session): `-10` captcha sweeper, `handlePendingCaptchaMessage` returns `ext.ContinueGroups` for any chat that is not a group or supergroup (`captcha.go:2144-2146`); `-6` fed-ban returns `ext.ContinueGroups` when `chat_status.RequirePrivate` holds (`federations.go:1184-1186`); `-1` users tracker ends with `return ext.ContinueGroups` (`users.go:146-203`); `0` commands (each ends its update with `ext.EndGroups`), the `#` notes watcher (private chat id as `chat.Id` finds no notes, so `ext.ContinueGroups`, `notes.go:671-695`) and the greetings service-message cleaner (service messages only); `2` raid AI (supergroups only), `3` aispam, `4` antiflood, `5/6` locks, `7` blacklists, `8` reports and reactions, `9` filters, `10` pins, `11` log channel, `12` late raid counter. Nothing registers group 1 [VERIFIED: grep of `AddHandlerToGroup` and `handlerGroup:` across `fuku/modules`]. PITFALLS.md (research) also names group 1 as free and recommends it (Pitfall 10).

**Decision: `settingsInputGroup = 1`** for the typed-reply door, `handlers.NewMessage(filter, door)` with `filter = m.Chat.Type == "private"`, no `SetAllowEdited`, no `SetAllowBot` (gotgbot already skips bot senders). Why 1 and not `-1`:
- Group-0 commands win automatically: a command typed while a wait is open runs and ends its update with `ext.EndGroups` (D-03 "a command is never taken as content"), so the door never sees it. The door still checks "looks like a command" for unregistered slash texts and for captions, using `msg.GetText()` and `msg.GetEntities()` (a copy of `raidFloodIsSlashText`, `raid_flood.go:165-177`, which reads only `Text`).
- The users tracker at `-1` runs first and returns `ext.ContinueGroups`, so user tracking is never starved; the door is reached.
- The private `/settings` list and `/start settings_<id>` are commands (group 0), the settings list buttons are callbacks (`shub|`, `settings|`), the captcha handlers return `ContinueGroups` for private chats, the anonymous-admin proof is a callback. None of them is a plain private message, so none competes with the door.
- Every later group is group-chat machinery; the door ending an accepted or refused update with `ext.EndGroups` keeps them from seeing the owner's welcome text (in particular the `reports` regex and `filters` are group-only in effect).

**Return values.** No wait and no marker, or not a candidate: `ext.ContinueGroups`, no reply, no write, no Telegram call, at most the one Redis round trip. Consumed (accepted, refused with a reply, ended with a reply, silenced album sibling): `ext.EndGroups`. A recovered panic returns `ext.EndGroups` after logging.

**Where it is registered.** In `LoadSettings` (`fuku/modules/settings.go:230-238`) by one added line `LoadSettingsInput(dispatcher)`, so every environment that loads the settings module, `settingsEnv.otherReplica()` included, has the door (S-11). `AGENTS.md` Handlers lists group `1` in the same commit.

**`/cancel`.** There is no existing `/cancel` command (grep of `"cancel"` shows only filter callback actions). Register it with `helpers.WrapCommand` (repo rule), not `Disableable`, with one `RequiredChecks` entry that returns `false` silently outside a private chat so a group `/cancel` makes no reply, write or Telegram call (the pipeline then returns `ext.EndGroups`; no other handler uses `/cancel`). Inside private chat: delete the wait and the marker atomically; reply `settings_edit_cancelled` when a wait existed, `settings_edit_cancel_nothing` otherwise. The docs generator parses `helpers.WrapCommand` registrations, so run `make generate-docs` and `make check-docs` at the end.

### S-03. Redis keys, TTLs and claim atomicity (all scripts verified on miniredis)

Pattern source: the Turnstile challenge store `consumeScript` / `finishScript` / `releaseScript` (`fuku/utils/captchaweb/challenge.go:238-311`): claim by adding a field, delete only with that claim's nonce after the work succeeded, release the claim with the same nonce when the work failed. `staff_action_card.go:273-311` is the other model (create-if-free and compare-and-transition scripts). Both use `redis.NewScript`, `cache.GetRedisClient()` and `cache.ContextWithTimeout()` (5 s, `cache.go:32-34`); `GetRedisClient()` returns a `*redis.Client` (single node, `cache.go:188-192`), so two-key scripts are valid.

| Key | Type | TTL | Content |
|-----|------|-----|---------|
| `fuku:settings:input:<user>` | hash | **10 min** (`PEXPIRE 600000` at Set; the key's own expiry is the wait's expiry, no timer, no drain) | `c` group chat id, `p` page key (`w g r`), `t` part (`x m b`), `m` prompt message id, `n` nonce (16 hex), `x` expiry unix ms (for the prompt's "until" time); plus `k` claim nonce while a claim is held. Never message text, never `file_id` |
| `fuku:settings:inputdone:<user>` | string `c|p|t` | **15 min** | Written by Set. Present with no wait means "the wait expired" (S-03 invariant below) |
| `fuku:settings:inputalbum:<user>:<media_group_id>` | string | **30 s** (`SET NX EX 30`) | Album memory; `media_group_id` must match `^[0-9]{1,20}$` or the item is treated as not part of an album |

All three sit in the existing `fuku:settings:*` family, outside the `fuku:cache:` prefix, so `CLEAR_CACHE_ON_STARTUP` keeps them. Document them in `AGENTS.md` Data.

**Scripts** (exact text in Code Examples; each run on miniredis in a scratch test this session):
- **setScript** (KEYS wait, marker): returns the old hash fields (`HGETALL`), `DEL`s, `HSET`s the new fields, `PEXPIRE`s, writes the marker with `PX 900000`. The caller compares the old `c`, `p`, `t` to the new ones: different means "replaced" (UI-SPEC `settings_edit_replaced_note`); the same part just refreshes the TTL. Scratch run: first set returned `[]`, the second returned `[c -1001 m 7 n n1 p w t x x 999]` and the key had a 10 m TTL [VERIFIED: scratch run].
- **claimScript**: succeeds only when `HGET n` equals the expected nonce and no `k` exists; sets `k`; returns `PTTL`. Scratch run: stale nonce `0`, correct nonce `600000`, a second claim `0`, a claim after `FastForward(11m)` `0` [VERIFIED: scratch run].
- **finishScript** (claim holder only): deletes the wait and the marker. **releaseScript** (claim holder only): `HDEL k` so the wait is back with its remaining TTL. Scratch run: finish deleted the key; release returned 1 [VERIFIED: scratch run].
- **cancelScript** (Cancel button and `/cancel`): returns 2 while a claim is held (a write is in flight: answer stale), 0 when the key is gone, 1 after deleting when `c`, `p`, `t` equal the button's, 3 when they differ (a stale prompt: the live wait is left untouched). Scratch run returned 2, 3, 1, 0 in those cases [VERIFIED: scratch run].
- **Album**: one `SetNX` per album item; the first caller proceeds, the rest are consumed silently. Scratch run: `true` then `false` [VERIFIED: scratch run].
- **Marker read**: `GetDel` of the marker so exactly one reply is sent per marker even when two deliveries race. Scratch run: first `"-1002|g|m"`, second `redis.Nil` [VERIFIED: scratch run].
- **Door read**: one pipeline of `HGETALL wait` and `GET marker` (one round trip). Do not use `MGET` across a hash and a string.

**Invariant that makes D-12 correct:** the marker means "the wait expired". So every deliberate end deletes **both** keys in the same script: save (finish), Cancel button, `/cancel`, access lost, gate refusal, right lost, replaced Set (the new Set rewrites both). Only the wait's TTL ever leaves a marker behind. Without this, an owner who saved successfully would get "the wait ended" for the next ordinary private message within 15 minutes.

**Without Redis:** `cache.GetRedisClient() == nil` (or a script error) makes Set answer the alert `settings_edit_unavailable` and changes nothing; the door returns `ext.ContinueGroups` at once; no process map is ever used (Pitfall 10 of the v1.1 research, `backup.go` pending maps are the counter-example).

### S-04. Validation a Telegram send would apply, and the helpers to reuse

| Rule | Source | Editor behaviour |
|------|--------|------------------|
| Text 1-4096 characters "after entities parsing" | `gotgbot/gen_methods.go:5870` ("Text of the message to be sent, 1-4096 characters after entities parsing") [CITED: gotgbot-embedded Bot API docs] | Dry run on the **visible** text of the filled message (below) |
| Caption 0-1024 characters after parsing | `gen_methods.go:5286` (document caption), `4817` (animation) [CITED] | Same, 1024 when the stored type is not TEXT |
| Whether Telegram counts UTF-16 units or code points | not stated in the embedded docs | Count UTF-16 units (never less than code points): conservative [ASSUMED] (A1) |
| Command parity count: code points of the stored HTML | `content.go:203-212` | Refuse with the commands' own text: `content_text_too_long` / `content_caption_too_long`, `fmt.Sprintf(template, n)` exactly as `preFixes` does (`content.go:207-208, 211-212`) |
| Button URL | `content.go:220-229` | Refuse (not drop) any button whose URL is not http/https with a host; name the button's 1-based number only |
| Buttons per row / per message | 8 / 100 [ASSUMED] (A2; PITFALLS Pitfall 12 says 100 [TG]) | Count rows with the same grouping `keyboard.BuildKeyboard` uses (`SameLine` appends to the last row, `keyboard.go` `BuildKeyboard`) plus one for a `{rules}` button; refuse over the limits |
| Link preview | `media.sendText` sets `LinkPreviewOptions{IsDisabled: !opts.WebPreview}`, greetings pass `WebPreview: false` (`sender.go:107-112, 329-346`) | Nothing to validate; no URL in a button is ever fetched by the bot |
| Media kinds the greeting can send | `media.Send` switch: TEXT, STICKER, DOCUMENT, PHOTO, AUDIO, VOICE, VIDEO, VIDEO_NOTE (`sender.go:82-102`) | Editor accepts photo, video, animation, document, sticker, audio, voice; **video note is refused** (UI-SPEC), poll/contact/location/dice/etc. refused |
| Sticker and video note carry no caption | `sendSticker`, `sendVideoNote` send no text (`sender.go:121-135, 227-241`) | Page note `settings_edit_note_no_caption`; the 1024 stored-count rule still applies for command parity |
| Empty message | a TEXT message whose filled text is empty fails at every join | Dry run also fills a minimal user (first name `A`, no last name, no username) and refuses a TEXT-type message whose visible filled text is empty |

**The dry run needs a pure fill function.** `FormattingReplacer(b, chat, user, oldMsg, buttons)` (`formatting.go:218-320`) calls `cachedMemberCount(b, chat)` for `{count}` (`formatting.go:267-270`, a `chat.GetMemberCount` Telegram call, cached 60 s) and `rules.GetChatRulesInfo(chat.Id)` for `{rules}` (`formatting.go:291`, which can create the chat and rules rows, S-05). Neither belongs in a validation step. Refactor behaviour-preservingly: extract from `FormattingReplacer` a pure `FillPlaceholders(oldMsg string, user *gotgbot.User, chatTitle, memberCount string) string` that builds the same `values` array and calls `replaceFormatPlaceholders` and also strips `{rules}` / `{rules:same}` / `{rules:up}` (`rulesBtnRegex`, `formatting.go:28`); `FormattingReplacer` keeps its signature and calls it. Existing tests keep covering `FormattingReplacer`; add `TestFillPlaceholders*`. Single-pass substitution means a name containing `{first}` is never expanded twice (`formatting.go:185-187` comment, `188-216`), and every substituted value is `html.EscapeString`-ed (`formatting.go:272-281`), so a joiner named `<a href=...>` is not live markup.

**Visible length.** `tgmd2html.StripHTMLV2(html)` (`gotg_md2html/stripV2.go`) drops tags and `html.UnescapeString`s the rest, which is the visible text of the converter's output (the converter HTML-escapes its whole input first, so the only `<`/`>` left are its own tags, `md2htmlV2.go:109-117`). Length in UTF-16 units is `len(utf16.Encode([]rune(visible)))`. Worst-case fill: first and last name 64 characters, `{fullname}` 129, username `@` plus 32, `{mention}` the same as `{username}` (`formatting.go:259-262, 272-281` show `{username}` and `{mention}` hold the same value), `{count}` nine digits, `{id}` 13 digits, `{chatname}` the group's real title. Three fills are checked: the owner's own name, the worst case and the minimal user. Emoji-only 64-character names (two units each) can still exceed the budget at join time; accepted (A11).

**Rules dry run.** Heading plus text as `sendRules` builds it: `fmt.Sprintf(temp, html.EscapeString(chat.Title)) + "\n\n" + normalizedRules` with `rules_for_chat_header` (`rules.go:136-139`); take the longest heading over the enabled locales because the group language can change, and also check the deep-link template `rules_for_chat` (`rules.go:372-376`). `{first}` and the other placeholders are **not** filled in rules (`/rules` calls neither `FormattingReplacer` nor anything similar, `rules.go:105-174`), so the prompt says so (UI-SPEC).

**Exported helpers to add (behaviour-preserving, called by the commands too):**
1. `content.ExtractMedia(msg *gotgbot.Message) (fileID string, dataType int)` over `extractMediaFromReply`.
2. `content.TextLengthRefusal(text string, dataType int, language string) string` holding the two checks of `preFixes` (`content.go:205-212`); `preFixes` calls it.
3. `content.ValidButtonURL(raw string) bool` holding `buttonUrlFixer`'s predicate (`content.go:223-224`); `buttonUrlFixer` calls it.
4. `formatting.FillPlaceholders` (above) and `formatting.VisibleLength(html string) int`.

### S-05. Reads, writes, cache keys

**Reads create rows and hide errors.** `checkGreetingSettings` creates the greetings row when the chat exists (`greetings/repository.go:46-69`, `db.CreateRecord(greetingSrc)`), returns in-memory defaults without persisting when the chat does not exist (`repository.go:21-44`), and on any other read error logs and returns defaults (`repository.go:71-93`). `checkRulesSetting` calls `chats.EnsureChatInDb` and `db.CreateRecord` on a missing row (`rules/repository.go:15-35`). `GetGreetingSettings` and `GetChatRulesInfo` drop the error (`repository.go:124-126`; `rules/repository.go:37-40`). The UI contract's rule "a getter that hides read errors is never used by a page" therefore needs **new side-effect-free, error-returning reads**:
- `greetings.LoadSettingsContext(ctx, chatID) (*models.GreetingSettings, error)`: `First` by `chat_id`; `gorm.ErrRecordNotFound` returns the runtime defaults **without creating anything** (below); a nil embedded `WelcomeSettings`/`GoodbyeSettings` or an empty text takes the same defaults `checkGreetingSettings` applies (`repository.go:95-119`).
- `rules.LoadRulesContext(ctx, chatID) (string, error)`: `First`; not found returns `""` and creates nothing.

**Goodbye is ON by default, not Off (verified conflict with the UI contract).** Migration `20250805200527`: `"goodbye_enabled" boolean default true` (`migrations/20250805200527_initial_migration.sql:171`), and the model tag `ShouldGoodbye bool gorm:"column:enabled;default:true"` (`fuku/db/models/greetings.go:15-23`, `GoodbyeSettings`). GORM's `Create` turns a Go `false` back into the SQL default. Scratch run on SQLite with the real model: for an existing chat with no row, `GetGreetingSettings(chatID).GoodbyeSettings.ShouldGoodbye` returned **true**, the first read **created a row** (1 row) with `welcome=true goodbye=true`, and after deleting that row `SetWelcomeText` on the fresh chat also stored `goodbye=true` with the default goodbye text [VERIFIED: scratch run; `fuku/db/greetings/repository_test.go:42-44` documents the same: "expected default ShouldGoodbye=true (DB column default)"]. Only a chat unknown to the `chats` table gets the in-memory `Off`, and a group in the settings hub is always known. **Decision:** the page and the Main status line show what the runtime will do: no row means Welcome On, Goodbye **On**, Delete-last Off (`checkGreetingSettings` for an existing chat). The UI-SPEC's Goodbye mock "Goodbye: ❌ Off" and "Defaults ... Goodbye Off" (Interaction Contract, Switches) must be corrected to On for a no-row chat; a test pins it (`TestSettingsEditDefaultsMatchRuntime`: the page state for a no-row chat equals what `GetGreetingSettings` then returns). Changing the model default is a separate change (migration plus every command's behaviour) and is not part of this phase.

**Writes.** `upsertGreetingSettings(chatID, updates map[string]any)` does `EnsureChatInDb`, `FirstOrCreate` with the default attrs, `Updates(updates)` and **`cache.DeleteCache(cache.CacheKey("greetings", chatID))`** (`repository.go:160-180`), so any new column-targeted writer that goes through it inherits the invalidation. New repository writers (each a one-line `updates` map, same function): `SetWelcomeTextOnly` (`welcome_text`), `SetWelcomeMediaOnly` (`welcome_file_id`, `welcome_type`), `SetWelcomeButtonsOnly` (`welcome_btns`), and the three Goodbye twins; toggles already exist (`SetWelcomeToggle`, `SetGoodbyeToggle`, `SetCleanWelcomeSetting`, `SetCleanGoodbyeSetting`). `SetWelcomeText` (all four columns) cannot serve a one-part write and stays for the commands. Rules: `rules.SetChatRules(chatId, rules)` writes `rules` through `UpdateRecordWithZeroValues` (`rules/repository.go:42-52`) and returns `gorm.ErrRecordNotFound` when no row matched (`db.go` `updateRecordInternal`), after `checkRulesSetting` created the row.

**Cache keys.** `greetings:<chat>` is the only key written, and **nothing reads it**: `GetGreetingSettings`, `GetWelcomeButtons` and `GetGoodbyeButtons` all call `checkGreetingSettings`, a direct `db.GetRecord` (`repository.go:17-19, 124-142`; `grep CacheKey("greetings"` finds only the writer, `backup.go:705, 1019` and `repository.go:178`). Rules has **no cache key at all** (`grep` finds no `CacheKey("rules"`), so `SetChatRules` has nothing to invalidate. Consequence: no `skipLocal` entry is needed (`fuku/db/cache/local.go:55-71` lists only captcha pending, staff group, staff link and raid detection), another replica never serves a stale value, and the AGENTS rule "every write must `cache.DeleteCache` the keys it affects" is met by the existing call. Keep reads uncached; if a later phase caches them it must add `skipLocal` entries and `DeleteCache` on every write.

**Race note (accepted).** Column-targeted writes cannot clobber the other parts, but "text vs media type vs caption limit" is checked against a read that another admin can change a moment later. No lock is added; the worst case is a caption over 1024 that Telegram refuses at join time, and Full preview shows it.

### S-06. Rights per control (command predicates versus `settingsRight*`)

| Command | Predicate read | Source |
|---------|----------------|--------|
| `/setwelcome`, `/setgoodbye`, `/resetwelcome`, `/resetgoodbye` | `chat_status.IsUserConnected(bot, ctx, true, false)` (admin) then `chat_status.CanUserChangeInfo(bot, ctx, chat, user.Id)` | `greetings.go:303-317, 348-363, 407-420` |
| `/cleanwelcome`, `/cleangoodbye` (`greetingToggle`) | `IsUserConnected(bot, ctx, true, cfg.connectBotAdmin)` (false for these two) then `CanUserChangeInfo` | `greetings.go:536-553`, `466-504` |
| `/welcome on|off`, `/goodbye on|off` (`displayGreeting`) | `IsUserConnected(bot, ctx, true, false)`; the toggle branch `RequireUserAdmin(bot, ctx, nil, user.Id)` only | `greetings.go:145-148, 236-240` |
| `/setrules`, `/clearrules` | `IsUserConnected(bot, ctx, true, true)` (admin and bot admin); **no** change-info check | `rules.go:30-36, 176-182` |
| change-info semantics | `requiredField(&userMember) || userMember.Status == "creator"` with `requiredField = m.CanChangeInfo` | `fuku/utils/chat_status/access.go:35, 38-42` |

So the UI-SPEC's rights table is confirmed. `settingsRightChangeInfo` is a new constant in `settings_menu.go` beside `settingsRightRestrict`, judged on the press's own live member record with no extra call, exactly like `staffIssuerSkipReason` (`staff_action_run.go:615-628`): creator passes; administrator passes when `p.member.CanChangeInfo`; anything else is the alert `settings_no_change_info` (a creator or administrator is already established at step 4, so a non-admin never reaches it). Switches use `settingsRightAdmin` (the live admin check every press makes), Rules Set/Remove use the existing `settingsRightAdminBotAdmin` (`chat_status.IsBotAdmin(p.b, nil, p.group)`, `settings_menu.go:78-82`, notice `chat_status_bot_not_admin`). The menu's check is live and the commands' is cached (`getUserMemberWithCache`), so the menu is stricter, as for every other right (AGENTS Permissions). Authority-parity tests per row (Validation).

The typed-reply door judges the same rights from **its own** live `getChatMember` record (not `p.member`): factor a pure `settingsRightRefusalFor(member gotgbot.MergedChatMember, right)` out of the `settingsRightRefusal` switch for the member-only rights, and keep the bot-admin check beside it, so press and door cannot drift.

### S-07. Preview and Full preview through the group's send path

- `media.SendGreeting(b *gotgbot.Bot, chatID int64, text, fileID string, msgType int, keyboard *gotgbot.InlineKeyboardMarkup, threadID int64)` takes the **target chat as an argument** and wraps `media.Send` with `ParseMode HTML`, `WebPreview false`, `AllowWithoutReply true` (`sender.go:329-346`). The group join, leave and `/welcome` all call it (`greetings.go:652, 727, 220/229`). So a preview is `SendGreeting(bot, presser.Id, ...)`.
- `SendWelcomeMessage` cannot be reused: it reads settings and the chat from `ctx.EffectiveChat`, builds the user from `userID, firstName` only, and mutates bookkeeping (`DeleteMessageWithErrorHandling` of the last welcome and `SetCleanWelcomeMsgId`, `greetings.go:619-665`); the leave path does the same for goodbye (`greetings.go:686-741`). **Decision:** extract one builder `renderGreeting(bot, group *gotgbot.Chat, user *gotgbot.User, text string, buttons []db.Button) (text string, kb *gotgbot.InlineKeyboardMarkup)` = `FormattingReplacer` + `keyboard.BuildKeyboard` (the three lines `greetings.go:226-228`, `642-646`, `721-722`), and make the join, the leave, `/welcome` and the preview call it. The preview then sends with `SendGreeting` and never touches `LastMsgId`. Refactoring the three callers is behaviour-preserving; `TestMemberJoinAndLeaveSendConfiguredGreetings` and the dedupe tests cover them.
- Preview user: the presser (`&p.query.From`, full name and username), chat: `p.group` (title for `{chatname}`; it carries `Id` for `chat.GetMemberCount`). Preview text sends `SendGreeting(bot, presser.Id, filledText, "", db.TEXT, &gotgbot.InlineKeyboardMarkup{}, 0)`; Preview media sends the stored media with empty caption and empty keyboard; Preview buttons sends `settings_edit_preview_buttons_body` with the keyboard; Full preview sends header then `SendGreeting` with the stored type, caption and keyboard; for Rules the text is the same composition as `sendRules` (extract `buildRulesText(tr, title, rawRules) string` from `rules.go:129-142`, used by both).
- **What a bot can send to a user.** In a private menu the user is by definition talking to the bot. In a group menu the presser may never have started the bot; Telegram answers 403 ("Forbidden: bot can't initiate conversation with a user" / "bot was blocked by the user") [ASSUMED wording, A5]; `helpers.IsExpectedTelegramError` already lists `"bot was blocked by the user"` (`telegram_helpers.go:71`). Classify by class, not wording, like `isTelegramClientError` (`settings_gate.go:256-262`): `errors.As(err, &tgErr)` with `Code == 403` gives `settings_edit_preview_need_start`; any other error gives `settings_edit_preview_failed` and the cause goes to the log only. `media.Send` wraps errors with `errors.Wrapf` whose `WrappedError` has `Unwrap` (`fuku/utils/errors/errors.go:21`), so `errors.As` works. `media.Send` only marks a chat restricted for group-permission wordings (`helpers.IsPermissionError`, `telegram_helpers.go:31-37`), never for a private 403, and returns `ErrNoPermission` in that case (treat it as `preview_failed`).
- **429.** `media.Send` does not retry; a 429 is a `*gotgbot.TelegramError` with `Code 429` and `ResponseParameters.RetryAfter` (`gen_types.go:10684`). A preview is two sends (header, content) at human speed; Telegram asks for at most about one message per second per chat [ASSUMED, A6]. Do not add a pacer or retry (UI-SPEC: not retried, nothing was written); a failed second send answers `settings_edit_preview_failed` and is logged with the status only.
- A preview never writes, never posts into the group, never calls `SendWelcomeMessage`; tests assert the greetings row and the group's calls are unchanged.

### S-08. Pages, parts table, apply functions, the pipeline hook

- Three `registerSettingsPage` calls (keys `w g r`, orders 50, 60, 70) from one shared file plus thin per-page files (D-02). Registration panics at init on a duplicate page key or action code (`settings_pages.go:400-420`); action codes `ws wr wv wf wc`, `gs gr gv gf gc`, `rs rr rv rf rc` clash with none of `oh op bk cl tg st no pk` (`settingsCoreActions`, `settings_pages.go:395-398`) or the existing page codes (`ls ly ua uy lk pg ay`).
- A part table drives one renderer: `editorPart{Code, StateLine, Accepts, Apply, Remove, PreviewKind}`; Welcome and Goodbye register `x m b`, Rules `x`; Phase 12 adds rows (UI-SPEC "Parts table"). The renderer is a **pure** `renderSettingsEditor(v settingsView, st editorState, notice string) settingsScreen` so the door can draw the saved page without a `settingsPress` (the pages' `Open(p, notice)` is a thin wrapper that loads the state then calls it); `settingsPress.view()` needs `p.query.From`, which a door has no callback for.
- **Missing pipeline hook (new finding).** `settingsCheckFields` allows a page action's `Fields` but **requires only** `a` and the owner (`settings_menu.go:237-245`: the `default:` branch adds keys to `allowed`, never to `required`; `settings_pages.go:374-381` says "they are allowed, not required, and Run parses them strictly"). The UI-SPEC says a missing or unknown `t` is refused "before any I/O". Add one optional field `Validate func(fields map[string]string) bool` to `settingsPageAction` and one line in the `default:` branch (`if entry.action.Validate != nil && !entry.action.Validate(fields) { return false }`), with a unit test; the editor's `Validate` checks `t` exists and is one of the page's parts. This is a deliberate, minimal, additive edit of two shared files (`settings_menu.go`, `settings_pages.go`); D-02's "settings_pages.go is not edited" is about registering pages. Fallback if the planner refuses the edit: check `t` first thing in `Run`, which is after the pipeline's step-4 `getChatMember`.
- **Apply functions (one setting, one code path), beside the commands** (`greetings.go`, `rules.go`), each with validation, the repository write and a typed result like `captchaApplyResult` (`captcha.go:797-807`, `applyCaptchaTimeout` `813-831`) / `accessApplyResult` (`settings_access.go:56-83`): `applyWelcomeEnabled`, `applyGoodbyeEnabled`, `applyCleanWelcome`, `applyCleanGoodbye` (the commands' `SetWelcomeToggle` ... calls move into them; the commands keep their replies and `common_settings_save_failed` text), and part writers `applyGreetingText/Media/Buttons(isWelcome, ...)`, `applyRulesText`, plus the Remove variants. The commands call the same apply functions where the semantics are identical (toggles, clean, reset, `/setrules`, `/clearrules`). Absolute values (`v`), so a double tap writes the same value.
- **Reads for pages:** `greetings.LoadSettingsContext` and `rules.LoadRulesContext` (S-05) behind package-level seams (`var settingsEditRead = ...`) so a test can make the read fail (as `settingsFloodRead` does, `settings_antiflood.go:42`); a read error is the cannot-open edit (`settings_menu.go:456-462`). `MainState` makes database reads only and returns `ok=false` on error (`settings_pages.go:350-353`); it must **not** use `GetGreetingSettings` (row creation on a read).
- **Existing generic tests now see 3 more pages.** Tests that loop `settingsPagesInOrder()` (`settings_main_test.go:218-229, 315`; `settings_menu_test.go:478, 501`; `settings_test.go:167`) and `TestSettingsMainGrid`/`TestSettingsMainPageFitsEveryLocale` must keep passing with real page fixtures; the harness `AutoMigrate` already holds `RulesSettings` and `GreetingSettings` (`test_harness_test.go:337, 343`).

### S-09. Albums: how one item is kept

Telegram delivers each album item as its own message update sharing `media_group_id` (`gotgbot/gen_types.go:8008-8009`, "The unique identifier inside this chat of a media message group this message belongs to") [CITED]; the dispatcher runs updates on up to `DISPATCHER_MAX_ROUTINES` (default 200) goroutines (AGENTS Architectural Constraints), so items are processed in parallel and in any order [ASSUMED order, A4]. **Decision: the kept item is whichever wins the album `SET NX`**, not necessarily the lowest message id; tests assert exactly one write and one "album" notice, never which item (UI-SPEC A8 settled as "first to win").

Door order for a message with a valid `media_group_id` **and a live wait** (after the cheap checks and the wait/marker read): `SET NX fuku:settings:inputalbum:<user>:<id> EX 30`; lost means a sibling is (or was) handled: return `ext.EndGroups` silently. Won: continue (this item is the leader). The album key is set **before** the live checks and the claim, so a refused leader's siblings are silenced too (D-13) and the siblings of an accepted album cannot start a second write. The expired-wait case needs no album key: the marker is read with `GETDEL`, so exactly one sibling gets the value and sends the one "wait ended" reply, and the others find neither wait nor marker and pass through silently (`ext.ContinueGroups`, nothing said). A user with neither wait nor marker never reaches the album key (no write for ordinary private traffic).

### S-10. The typed-reply door, step by step

`LoadSettingsInput` registers the group-1 handler and `/cancel`. The handler, with `defer error_handling.RecoverFromPanic("settingsInput", "Settings")`:

0. **Candidate** (no I/O): private chat, `msg.From != nil`, `msg.From.Id == msg.Chat.Id`, not `staffServiceUserIDs[msg.From.Id]` (`staff_action_decide.go:513`), `msg.RichMessage == nil`, not slash-prefixed by `msg.GetText()`/`GetEntities()`. Otherwise `ext.ContinueGroups`.
1. **Read** wait and marker in one pipeline. Redis error or no client: `ext.ContinueGroups`, nothing said. Malformed wait (missing field, `p`/`t` not a known pair, non-numeric `c`): treat as no wait, log once.
2. **No wait, marker present:** `GetDel` the marker; if it returned a value, one plain reply `wait ended, press Set again` naming the part (D-12; new keys `settings_edit_wait_ended_note` and the three part words, see Open Questions Q9), then `ext.EndGroups`; if it returned nothing (a sibling or a duplicate took it), `ext.ContinueGroups` with nothing said. Neither wait nor marker: `ext.ContinueGroups`.
3. **Album gate** for a message with a valid `media_group_id` (S-09), only with a live wait.
4. **Live member** `lockdownLiveMember(b, c, sender)` (`lockdown.go:114-126`, 5 s timeout): status other than creator/administrator, or `isTelegramClientError(err)`: `settings_edit_refuse_access_lost`, wait ended (cancelScript-style delete of both keys). Other errors (429, 5xx, transport, timeout): `settings_check_failed`, wait stays.
5. **Owner gate** `settingsGate(b, c, *msg.From, member)` (`settings_gate.go:89-116`): `settingsGateOwnerOnly` gives `settings_gate_owner_only`, wait ended; undecided gives `settings_check_failed`, wait stays.
6. **Right of the part** from the member record (change-info for Welcome/Goodbye, bot-admin for Rules): `settings_no_change_info` or `chat_status_bot_not_admin`, wait ended.
7. **`getChat(c)`** (needed for the title and the group/supergroup check): client error or not a group/supergroup: `settings_edit_refuse_access_lost`, ended; other errors `settings_check_failed`, stays.
8. **Pure validation** (S-04) of the input for the part; a refusal replies with the reason, a blank line and `settings_edit_still_waiting`, keyboard `[Cancel]`; the wait is untouched and its TTL not extended. The entity conversion is wrapped in a function with `recover` that returns an error, because `getOrigMsgMDV2` slices by entity offsets and a malformed update must be a `need_text` refusal, not a swallowed panic.
9. **Claim** (claimScript with `n` from the read). Lost (0): say nothing, `ext.EndGroups`.
10. **Apply** (the part's apply function). Failure: `releaseScript`, reply `common_settings_save_failed` plus `settings_edit_still_waiting`. Success: `finishScript` (deletes wait and marker), read the stored state fresh, send the saved page as a **new** message (page's header and group line name the target, Pitfall 10 of the v1.1 research) with the ✅ notice and the caption/album lines, then best-effort edit of the old prompt (`m`) to `settings_edit_prompt_closed` with no keyboard. If the post-write read fails, send the plain `settings_edit_saved_*` text.

Telegram calls on a successful typed save: `getChatMember`, `getChat`, plus `getChatAdministrators` only for a gated non-creator with the switch off, then `sendMessage` and `editMessageText`. The editing of the old prompt is synchronous (no goroutine) so tests need no waiting and no extra recover. No log line carries text, button names, URLs or file ids.

### S-11. How to test it (existing toolkit and its gaps)

- `settingsEnv` (`settings_env_test.go`) gives a real dispatcher, miniredis (`env.redis`, a `*miniredis.Miniredis` from `withMiniredisServer`, `testredis_test.go:22-43`), the marker locale, `openMenu`, `press`, `pressPrivate`, `edits`, `lastEdit`, `answerCount`, `memberLookupsOf`, `settingsButtonByMarker`, `settingsButtonFields`, `newOwner`, `setAdminsAllowed`, `otherReplica()`. `sendPrivate(from, text)` builds only `Message{Text}` (`settings_env_test.go:251-255`): **no entities, no media, no album, no caption**.
- `otherReplica()` builds a second fake, bot and dispatcher over the same database and miniredis and loads `LoadLockdown, LoadSettings, LoadCaptcha, LoadAntiflood, LoadStaff, LoadHelp, LoadBotUpdates` only (`settings_env_test.go:103-134`). Registering the door inside `LoadSettings` (S-02) makes the two-replica test work with no toolkit edit.
- Fakes: `staffBotClient` records every call and answers `getChatMember`/`getChatAdministrators` per chat; `lockdownFake.setMemberRaw(chat, user, rawJSON)` plus a `lockdownAdminJSON`-style helper gives an administrator record with `can_change_info` true or false (`lockdown_fake_test.go:68-83`; the shipped helper hardcodes `"can_change_info":false`, so add `settingsEditAdminJSON(userID, canChangeInfo)`); `setFailure(method, chatID, err)` scripts Telegram errors (`staff_helpers_test.go:100-104`). `moduleBotClient` answers only `sendMessage`, `sendDocument`, `sendPhoto` with a Message JSON (`test_harness_test.go:63-88`); **every other send method returns `true`**, which does not decode into `*Message`, so the editor tests add `fake.responses["sendSticker"|"sendVideo"|"sendAudio"|"sendVoice"|"sendAnimation"]` (the map is a plain field).
- Redis TTL: `env.redis.FastForward(10*time.Minute + time.Second)` makes the wait expire; `env.redis.Exists(key)`, `env.redis.HGet(key, "t")`, `env.redis.TTL(key)` assert state. Lua runs under miniredis (verified).
- Command parity: call the real commands (`greetingsModule.setWelcome(bot, ctx)` through `newGreetingMessageContext`, `rulesModule.setRules`) on chat A with a message whose text is `"/setwelcome " + text` and whose entity offsets are shifted by the UTF-16 length of `"/setwelcome "`, and the editor on chat B with the same text unshifted; compare the stored `welcome_text/_type/_file_id/_btns`. A `shiftEntities` helper in the new test file does the shift.
- Gate table: add one door "typed reply" to `TestSettingsGateAllDoors` (`settings_gate_test.go:998-1004`) with a driver that opens the menu, presses Set while the world allows it, applies the world, sends the typed reply and classifies the outcome (`settings_gate_owner_only` marker: `owner_only`; `settings_check_failed`: `check_failed`; a saved page or `settings_edit_saved_` marker: `in`); `settingsGateClassify` (`settings_gate_test.go:846-862`) already knows the first two markers.
- Locale tests: `fuku/i18n/settings_locale_test.go` requires a limits row for **every** `settings_` key and nothing else, the same placeholder list in every language, no printf verb, only `<b>`/`<code>` tags, alert keys `<= 200` without markup (`settings_locale_test.go:179-268`); the new alert keys must be appended to `settingsAlertKeys` (`:143-154`). `GetString` with no params does **not** interpolate at all and with params leaves unknown `{tokens}` untouched (`i18n/translator.go:38-44, 82-88`), which is what keeps `{first}` literal in the prompts.
- Baseline is green: `TestSettingsGateAllDoors`, `TestSettingsCallbackBytes`, `TestSettingsHelpFitsOneMessage` passed this session in about 6 s.

## Architecture Patterns

### System Architecture Diagram

```
 OWNER (private chat)                         GROUP MENU (any admin the gate lets in)
   |  taps Welcome page > [Set text]            |  taps [Set text]
   v                                            v
 settings| callback ---> settingsPress.verify (decode, service id, opener, live getChatMember,
   |                      owner gate 4b, getChat) ---> checkRight (ChangeInfo | BotAdmin | Admin)
   |                                                    |
   |   private?  no --> redraw page + settings_edit_group_note + Open-in-private row
   |             yes
   v
 Redis: setScript  fuku:settings:input:<user> (hash, 10 min)  + fuku:settings:inputdone:<user> (15 min)
   |  (no Redis -> alert settings_edit_unavailable)
   v
 edit message into the PROMPT (Cancel only)                       Preview / Full preview (both menus)
                                                                     |
 OWNER sends a private message ---> dispatcher groups: -10 -6 -1 (ContinueGroups)   build via renderGreeting
   |                                   0 commands (/cancel, /settings, /start) END  --> media.SendGreeting(b, presser.Id,...)
   v                                                                                   (never SendWelcomeMessage,
 group 1: settingsInputDoor                                                              never LastMsgId)
   0 candidate? --no--> ContinueGroups
   1 pipeline read wait+marker --none--> ContinueGroups ; marker only --> GETDEL, "wait ended", End
   3 album SETNX --lost--> End (silent)
   4 live getChatMember --not admin / 4xx--> access_lost (wait ended)    --429/5xx--> check_failed (stays)
   5 settingsGate --owner only--> gate_owner_only (ended) --undecided--> check_failed (stays)
   6 right (ChangeInfo / BotAdmin) --> refusal (ended)
   7 getChat (title, group type)
   8 pure validation + dry run --refuse--> reason + still_waiting + [Cancel]   (wait untouched)
   9 claimScript --lost--> End (silent)
  10 apply (column-targeted write, DeleteCache) --fail--> releaseScript + save_failed
        success --> finishScript (wait+marker gone) --> NEW message: page + notice, old prompt -> "closed"
```

### Recommended Project Structure
```
fuku/db/greetings/repository.go   # + LoadSettingsContext, Set{Welcome,Goodbye}{Text,Media,Buttons}Only
fuku/db/rules/repository.go       # + LoadRulesContext
fuku/utils/content/content.go     # + ExtractMedia, TextLengthRefusal, ValidButtonURL (commands call them)
fuku/utils/formatting/formatting.go  # + FillPlaceholders, VisibleLength (FormattingReplacer calls FillPlaceholders)
fuku/modules/greetings.go         # apply* functions; renderGreeting builder used by join/leave/welcome/preview
fuku/modules/rules.go             # applyRulesText/Clear; buildRulesText shared with preview
fuku/modules/settings_edit.go     # part table, pure renderer, state read, register helper
fuku/modules/settings_edit_{welcome,goodbye,rules}.go  # thin registerSettingsPage calls (or one file)
fuku/modules/settings_edit_check.go   # pure validation + dry run
fuku/modules/settings_edit_preview.go # preview actions
fuku/modules/settings_input.go    # wait store (scripts), door, /cancel, LoadSettingsInput
fuku/modules/settings_menu.go     # + settingsRightChangeInfo, Validate hook line
locales/*.yml (8)                 # + 79 keys, settings_help_msg changed
```

### Pattern 1: Claim, work, finish or release (the wait)
**What:** claim the wait by nonce only after every check passed; finish deletes wait and marker; release gives it back. **When:** the typed save. Source: `captchaweb/challenge.go:238-311`.
```go
// Source: fuku/utils/captchaweb/challenge.go:246-271 (same shape); scripts verified on miniredis
var settingsInputClaimScript = redis.NewScript(`
if redis.call('HGET', KEYS[1], 'n') ~= ARGV[1] then return 0 end
if redis.call('HEXISTS', KEYS[1], 'k') == 1 then return 0 end
redis.call('HSET', KEYS[1], 'k', ARGV[2])
return redis.call('PTTL', KEYS[1])
`)
```

### Pattern 2: One setting, one code path (apply functions)
Source: `applyCaptchaTimeout` (`captcha.go:809-831`), `applyAdminsAllowed` (`settings_access.go:68-83`). Each apply function validates, writes, returns a typed result with the stored value read back; the command and the page both map it.

### Pattern 3: Pure renderer plus thin `Open`
The renderer takes a `settingsView` and a loaded state and returns a `settingsScreen`; `Open` loads and calls it (like `renderSettingsAntiflood`, `settings_antiflood.go:121-154`); the door draws the saved page by building its own `settingsView` (Private true, `OwnerKey "c"`, the group id, title from `getChat`, `Owner` from the member status).

### Anti-Patterns to Avoid
- **Reading with `GetGreetingSettings` / `GetChatRulesInfo` in a page or `MainState`:** creates rows and hides errors (S-05).
- **Claiming before validating:** a refusal would consume the wait.
- **Answering "the wait ended" before the album gate:** every sibling of an album would answer.
- **A second list of command names** to decide "is a command": use the dispatcher's own order (group 1) plus the slash predicate.
- **Echoing stored text or button names in any page or refusal:** a group menu is readable by every member of the group; show states, lengths and button numbers only.
- **Calling `SendWelcomeMessage` for a preview:** it deletes the previous welcome and rewrites `LastMsgId`.
- **Keeping the wait in a process map or behind `time.AfterFunc`.**

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Entities to stored text | Own UTF-16 slicing/escaping | `Message.OriginalMDV2()` + `tgmd2html.MD2HTMLButtonsV2` / `MD2HTMLV2` | Same as the commands; UTF-16 handled in gotgbot |
| Visible length of HTML | Own tag stripper | `tgmd2html.StripHTMLV2` + `utf16.Encode` | The converter's HTML is the only input; unescape included |
| Button parse and URL filter | A regex for `buttonurl://` | `MD2HTMLButtonsV2` and `content.ValidButtonURL` | `:same`, `#style`, `\` escapes already handled |
| Wait atomicity | Read-then-delete in Go | Lua claim/finish/release | Double delivery and albums run on parallel goroutines and replicas |
| Pacing of previews | A limiter | Nothing (human-paced; not retried) | UI-SPEC; `staffPaced` is for fan-outs |
| Right checks | New admin-cache lookups | The press's live `MergedChatMember` and `settingsGate` | Cached admin state never authorizes in settings |
| Callback data | `strings.Split` / `fmt.Sprintf` | `encodeCallbackData` / `decodeCallbackData` | 64-byte cap and `Omitted` accounting |
| Placeholder substitution | `strings.NewReplacer` (re-scan risk) | `replaceFormatPlaceholders` via `FillPlaceholders` | Single pass, escaped values |
| Sender "is a service identity" | A new ID list | `staffServiceUserIDs` | One definition |

**Key insight:** every hard part (conversion, send, authority, atomicity, locale limits) already has one canonical implementation; the work is wiring plus the seven corrections above. A parallel implementation anywhere would make the editor store or send something the commands do not, which is exactly what EDIT-11 forbids.

## Common Pitfalls

### Pitfall 1: Assuming Goodbye is Off by default
**What goes wrong:** the page says Off while the next leave sends a goodbye. **Why:** DB default `true` and GORM Create (S-05, scratch run). **Avoid:** the page state for a no-row chat equals `On`; `TestSettingsEditDefaultsMatchRuntime`. **Warning sign:** a page test fixture asserting `settings_state` Off for Goodbye on a fresh chat.

### Pitfall 2: Reads that create rows
**What goes wrong:** opening the Main page or a page writes `greetings`/`rules` rows (and `EnsureChatInDb`). **Avoid:** `LoadSettingsContext` / `LoadRulesContext`; a test counts rows before and after opening the pages.

### Pitfall 3: Rules double escaping (pre-existing)
**What goes wrong:** `/setrules` stores `tgmd2html.MD2HTMLV2(text)`; for a text with no formatting tags the stored value is already HTML-escaped (`Don&#39;t spam &amp; &lt;be&gt; nice`), and `/rules` re-runs `normalizeRulesForHTML` (`rules_format.go:12-21`: no tag found, so `MD2HTMLV2` again), displaying `Don&#39;t` literally. Scratch run: `STORED="Don&#39;t spam &amp; &lt;be&gt; nice"`, `NORM="Don&amp;#39;t spam &amp;amp; &amp;lt;be&amp;gt; nice"` [VERIFIED: scratch run]. Texts with a bold or link entity are unaffected (a tag is present). **Decision:** keep parity (store exactly as `/setrules`; the preview shows what members see, bug included) and record the defect as a follow-up for the owner; do not change `normalizeRulesForHTML` in this phase (it changes `/rules` for every group). The Rules dry run counts the displayed (double-escaped) visible length, which is what `/rules` sends.

### Pitfall 4: Validating with `FormattingReplacer`
`{count}` triggers a Telegram call and `{rules}` a database read that can create a row. Use `FillPlaceholders` (S-04).

### Pitfall 5: `ext.ContinueGroups` is not "go to the next group"
It continues the **current group**; returning nil or an error moves to the next group (`dispatcher.go:294-324`). The door returns `ContinueGroups` for "not mine" (nothing else in group 1) and `EndGroups` when it consumed.

### Pitfall 6: Captions are commands too
`handlers.NewCommand` uses `msg.GetText()` (caption first). A media reply captioned `/welcome` runs the command at group 0 and never reaches the door; a caption that merely starts with `/` and has no handler reaches the door and must be treated as a command attempt (no content, wait stays, nothing said).

### Pitfall 7: The marker outliving the wait
Delete both keys on every deliberate end (S-03 invariant); test: save, then an ordinary private message within 15 minutes gets no reply.

### Pitfall 8: Album siblings answering
With a live wait: album `SET NX` before the live checks and the claim (S-09). With an expired wait: the marker is read with `GETDEL` so exactly one sibling replies. A test with three concurrent items expects one reply total in both worlds.

### Pitfall 9: `t` not required by the pipeline
Add `Validate` (S-08) or check `t` first in `Run`; test `ws` without `t`, with `t=z`, with `t=m` on Rules, with a non-ASCII value, all `common_callback_invalid_request`.

### Pitfall 10: Existing page loops and the Main grid
Three new real pages join `settingsPagesInOrder()`; update `TestSettingsMainGrid` expectations (`[Welcome | Goodbye]`, `[Rules]` alone, `[Access]` last) and the Main status block, and the label-length test list (`settings_locale_test.go:303-307`, add the three `icon + name` pairs).

### Pitfall 11: Typed `*x*` becomes bold
`OriginalMDV2` does not escape plain markdown (S-01). It is parity with the commands; do not "fix" it in the editor, and the prompt's "Bold, italic, links and other formatting are kept" stays true.

### Pitfall 12: Text with only placeholders
`{last}` alone is empty for a member with no last name and every join then fails (Telegram rejects empty text). The minimal-user fill refuses it.

### Pitfall 13: A first Set on a fresh chat turns Goodbye on
`upsertGreetingSettings` creates the row with the DB defaults (S-05). Same as `/setwelcome` today; the saved page shows the stored Goodbye state so the owner sees it.

### Pitfall 14: Test fakes return `true` for unfaked send methods
A sticker, video, audio, voice or animation preview fails to decode in the fake unless its response is added (S-11).

### Pitfall 15: Stale numbers in docs
`AGENTS.md` says 7 locales; the repo has 8. `make check-docs` will see the new `/cancel`; run `make generate-docs` then `make check-docs`.

## Code Examples

### Stored form of each part (reuse, do not re-implement)
```go
// Source: fuku/utils/content/content.go:160-172, rules.go:206 (the commands' own calls)
raw := msg.OriginalMDV2()                          // text message typed in private
html, btns := tgmd2html.MD2HTMLButtonsV2(raw)      // Welcome / Goodbye text part
if len(btns) > 0 { /* refuse: settings_edit_refuse_text_buttons */ }
if msg := content.TextLengthRefusal(html, dataType, lang); msg != "" { /* refuse with it */ }
rules := tgmd2html.MD2HTMLV2(raw)                  // Rules text part, as /setrules
fileID, kind := content.ExtractMedia(msg)          // GIF -> db.DOCUMENT; VIDEO_NOTE -> refuse
```

### Wait scripts (verified on miniredis)
```go
// KEYS[1] wait, KEYS[2] marker. ARGV: ttlMs, c, p, t, m, n, x, markerValue, markerTtlMs
var settingsInputSetScript = redis.NewScript(`
local old = redis.call('HGETALL', KEYS[1])
redis.call('DEL', KEYS[1])
redis.call('HSET', KEYS[1], 'c', ARGV[2], 'p', ARGV[3], 't', ARGV[4], 'm', ARGV[5], 'n', ARGV[6], 'x', ARGV[7])
redis.call('PEXPIRE', KEYS[1], ARGV[1])
redis.call('SET', KEYS[2], ARGV[8], 'PX', ARGV[9])
return old
`)
// cancel: ARGV c, p, t. 2 = a claim is held, 0 = no wait, 1 = deleted (wait and marker), 3 = other part
var settingsInputCancelScript = redis.NewScript(`
if redis.call('HEXISTS', KEYS[1], 'k') == 1 then return 2 end
local f = redis.call('HMGET', KEYS[1], 'c', 'p', 't')
if not f[1] then return 0 end
if f[1] == ARGV[1] and f[2] == ARGV[2] and f[3] == ARGV[3] then
  redis.call('DEL', KEYS[1]); redis.call('DEL', KEYS[2]); return 1
end
return 3
`)
// finish: delete wait and marker only for the holder of claim ARGV[1]; release: HDEL k only for the holder
var settingsInputFinishScript = redis.NewScript(`
if redis.call('HGET', KEYS[1], 'k') == ARGV[1] then redis.call('DEL', KEYS[2]); return redis.call('DEL', KEYS[1]) end
return 0
`)
```
(The set script above adds the marker write to the verified one, a trivial `SET ... PX`; the claim, release and cancel bodies are as run. Re-run them in the first test.)

### Door pipeline read (one round trip)
```go
// Source: go-redis pipeline; miniredis supports it
pipe := client.Pipeline()
waitCmd := pipe.HGetAll(ctx, waitKey)
markerCmd := pipe.Get(ctx, markerKey)
_, err := pipe.Exec(ctx)             // redis.Nil from Get is expected, any other error = no wait
```

### Dry-run skeleton
```go
// pure; no Telegram, no database
func editorDryRun(users []*gotgbot.User, groupTitle, html string, dataType int, buttons []db.Button, rulesBtn bool) (refusal key, n, max int) {
    limit := 4096; if dataType != db.TEXT { limit = 1024 }
    for _, u := range users { // owner, worst case, minimal
        filled := formatting.FillPlaceholders(html, u, groupTitle, "999999999")
        n := formatting.VisibleLength(filled)   // StripHTMLV2 then UTF-16 units
        if n > limit { return "settings_edit_refuse_too_long_filled", n, limit }
        if dataType == db.TEXT && n == 0 { return "settings_edit_refuse_need_text", 0, limit }
    }
    // buttons: rows by BuildKeyboard grouping (+1 for {rules}); <= 8 per row, <= 100 total
}
```

### Spike reproductions (scratch tests run this session, deleted afterwards)
```go
// entity/stored-form spike (package modules, -tags testtools)
m := gotgbot.Message{Text: "Hi 😀 bold link", Entities: []gotgbot.MessageEntity{
    {Type: "bold", Offset: 6, Length: 4}, {Type: "text_link", Offset: 11, Length: 4, Url: "https://e.com"}}}
tgmd2html.MD2HTMLV2(m.OriginalMDV2())   // Hi 😀 <b>bold</b> <a href="https://e.com">link</a>
// rules double escape
s := tgmd2html.MD2HTMLV2("Don't spam & <be> nice"); normalizeRulesForHTML(s) // Don&amp;#39;t ...
// goodbye default (package greetings): chats.EnsureChatInDb; GetGreetingSettings(id).GoodbyeSettings.ShouldGoodbye == true; 1 row created
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Per-replica pending-input maps (`backup.go` pending maps) | Redis hash with nonce claim | this phase | Works on any replica, atomic |
| Command-only welcome editing | Private message editor + commands, one stored form | this phase | Two doors, one code path |
| Silent drop of a bad button URL (`buttonUrlFixer`) | Editor refuses with the button number | this phase | The commands keep dropping; the editor is stricter (EDIT-05) |

**Deprecated/outdated:** none introduced. Note gotgbot's new `RichMessage` type exists (`gen_types.go:8024-8025`); a typed private message is expected to populate `Text` (A9), and the door treats a message with `RichMessage != nil` as not a candidate.

## Runtime State Inventory

Not a rename/refactor phase. New persistent state only: three Redis key families (`fuku:settings:input:*`, `fuku:settings:inputdone:*`, `fuku:settings:inputalbum:*`), all TTL-bound and outside the cache prefix; no table, migration, model, secret or env var. Stored data: no migration; the greetings and rules rows keep their shape; nothing stored is keyed to a renamed identifier. Build artifacts: none.

## Assumptions Log

| # | Claim | Section | Confidence | Verified how | Risk if Wrong |
|---|-------|---------|------------|--------------|---------------|
| A1 | Telegram's 4096/1024 limits count UTF-16 units (at least as strict as code points) | S-04 | MEDIUM | Embedded Bot API text says only "characters after entities parsing"; chose the stricter count | If Telegram counts code points, the editor refuses a text that would have sent (only for astral-plane characters near the limit) |
| A2 | At most 8 buttons per row and 100 per message | S-04 | MEDIUM | v1.1 PITFALLS Pitfall 12 `[TG]`, not re-probed | A wrong limit either refuses a valid keyboard or lets a bad one through to Full preview |
| A3 | A GIF stored as `DOCUMENT` and resent with `sendDocument` still renders as a GIF | S-01 | LOW | Not probed; parity with the commands; Full preview shows exactly what members see | If it renders as a file, the owner sees it in Full preview; a `sendAnimation` change would touch the join path (out of scope) |
| A4 | Album items arrive as separate updates, processed in parallel and in any order | S-09 | MEDIUM | gotgbot doc for `media_group_id`; dispatcher parallelism from AGENTS | If items were sequential the album gate is just redundant |
| A5 | A bot's send to a user who never started it or blocked it fails with 403 | S-07 | MEDIUM | `helpers.IsExpectedTelegramError` lists "bot was blocked by the user" (`telegram_helpers.go:71`); classify by `Code == 403` | A different code would give `preview_failed` instead of `preview_need_start` |
| A6 | About 1 message per second per chat is safe; two back-to-back preview sends are tolerated | S-07 | MEDIUM | Telegram folklore, not probed | A 429 on the second send gives `preview_failed`; the owner presses again |
| A7 | Custom-emoji entities in a bot's text are shown only with Fragment usernames, else fallback emoji | Pitfalls | LOW | v1.1 PITFALLS Pitfall 11 `[TG]` | Cosmetic; Full preview shows it |
| A8 | Telegram has no extra documented limit on button text or URL length beyond what Full preview catches | S-04 | LOW | Not probed | A rare join-time refusal; the owner sees it in Full preview |
| A9 | A normally typed private message populates `Text` (not `RichMessage`) | Deprecated | MEDIUM | gotgbot type exists; no repo code handles `RichMessage` for plain text | A rich message would be refused as not a candidate (no wait consumed) |
| A10 | `document.file_id` of an animation message equals its `animation.file_id` | S-01 | LOW | Irrelevant to behaviour: `Document` is read first | none |
| A11 | 64-character emoji-only names can still exceed the budget at join time | S-04 | HIGH (arithmetic) | Two UTF-16 units per astral character | Accepted tolerance; Full preview shows the owner's own name only |
| A12 | The Redis client is single node (multi-key scripts valid) | S-03 | HIGH | `cache.go:188-192` returns `*redis.Client` | A cluster would need hash tags and renamed keys |

**Needs user confirmation before it becomes locked:** A3 (GIF rendering; the owner's device checklist already holds it) and the **Goodbye default On** correction (Open Question Q1, a verified fact that changes a UI-SPEC mock). Everything else is conservative or has Full preview as its check.

## Open Questions (RESOLVED)

1. **What state does the Goodbye page show for a chat with no greetings row?**
   - What we know: UI-SPEC says Off; code and a scratch run say the first read or write persists `goodbye_enabled = true` (S-05).
   - **RESOLVED:** show what the runtime does: Goodbye **On**, Welcome On, Delete-last Off; correct the UI-SPEC mock and the "Defaults" sentence in the same plan that writes the page; `TestSettingsEditDefaultsMatchRuntime` pins it. No model or migration change here.
2. **Which handler group for the door?**
   - **RESOLVED:** group 1, registered from `LoadSettings` (S-02); `AGENTS.md` Handlers gains it.
3. **How is `/cancel` registered?**
   - **RESOLVED:** `helpers.WrapCommand` with a silent private-only check, not Disableable (S-02).
4. **Key names and TTLs?**
   - **RESOLVED:** `fuku:settings:input:<user>` 10 min (hash), `fuku:settings:inputdone:<user>` 15 min, `fuku:settings:inputalbum:<user>:<media_group_id>` 30 s; claim/finish/release/cancel Lua (S-03).
5. **UI-SPEC A1 (GIF)?**
   - **RESOLVED:** stored as `db.DOCUMENT` exactly as the commands store it (`content.go:134-135`); label "File or GIF"; no `sendAnimation`; Full preview and the owner's device checklist are the rendering check (A3).
6. **UI-SPEC A6 (HTML-count strictness)?**
   - **RESOLVED:** both checks stay: the commands' stored-HTML code-point count (parity, refusal text `content_*`) and the dry run's visible UTF-16 count over three fills; the HTML count is never looser than the visible count, and placeholders are the reason the dry run is needed (S-04).
7. **UI-SPEC A8 (album item)?**
   - **RESOLVED:** the item that wins the album `SET NX`; siblings silent for 30 s; tests assert one write, not which item (S-09).
8. **UI-SPEC A9 (deep link landing)?**
   - **RESOLVED:** confirmed: `settingsStartDeepLink` renders `renderSettingsMain` and sends it (`settings.go:208-225`); the payload grammar is `settings_<chat id>` only and is parsed strictly (`parseStaffChatIDArg`); the deep link is not edited, the owner taps the page again.
9. **What text does the D-12 "wait ended" reply use?**
   - What we know: D-12 adds a plain reply that names the part; the UI-SPEC's 79 keys have no key for it, and none of its keys is a bare part word (`settings_edit_line_text` carries `{state}`).
   - **RESOLVED:** add **four** `settings_` keys to the contract (79 + 4 = 83, each with a limits row, a value in all 8 locales and no alert status because it is a plain reply): `settings_edit_wait_ended_note` ("The wait for the {part} ended and nothing changed. Press Set on the page again.", Max EN 120 / other 168) and the part words `settings_edit_part_text` ("text", 14 / 20), `settings_edit_part_media` ("media", 14 / 20), `settings_edit_part_buttons` ("buttons", 14 / 20). The marker value `c|p|t` selects the word. The UI-SPEC copy table gets these four rows in the same plan that adds the locales; the plan that writes them also bumps the "79" in its own checks.
10. **How is a missing `t` refused before I/O?**
    - **RESOLVED:** the `Validate` hook (S-08); fallback to a first-line check in `Run`.
11. **Where does a preview's target chat come from in a group menu?**
    - **RESOLVED:** `query.From.Id` (a private chat id equals the user id); 403 gives `settings_edit_preview_need_start`.
12. **Should the pre-existing rules double escaping be fixed here?**
    - **RESOLVED:** no (S-01 parity, Pitfall 3); reported as a follow-up.
13. **Does anything need `skipLocal` or a new `DeleteCache`?**
    - **RESOLVED:** no; greetings and rules reads are uncached and the existing `greetings:<chat>` invalidation stays (S-05).

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go | build, tests | yes | go1.26.0 linux/amd64 | none needed |
| gcc / CGO | `go-sqlite3` in tests | yes | `/usr/bin/gcc` | none needed |
| golangci-lint | `make lint` | yes | `/usr/local/bin/golangci-lint` | none needed |
| Node 22 | captcha page script test (untouched) | yes | `/opt/node22/bin/node` | not needed |
| redis-server / pg_isready | live Redis / PostgreSQL | binaries present, servers not running | - | miniredis and SQLite in tests (repo standard); PostgreSQL only in CI and this phase has no PostgreSQL guarantee |
| core.telegram.org | Bot API docs | **blocked by the session network policy** | - | gotgbot's generated doc comments (used); A1-A8 stay unprobed |

**Missing dependencies with no fallback:** none for planning or execution.
**Missing dependencies with fallback:** Telegram docs (add the host to the session network settings to settle A1, A2, A5, A6, A8).

## Validation Architecture

Nyquist validation is enabled (`workflow.nyquist_validation: true` in `.planning/config.json`).

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go `testing`, build tag `testtools`, miniredis, hand-written `gotgbot.BotClient` fakes, no mock libraries |
| Config file | none; `Makefile` target `test` |
| Quick run command | `go test -tags testtools -race -count=1 -run '^(TestSettings|TestLoadSettings)' ./fuku/modules ./fuku/i18n` (AGENTS), plus `go test -tags testtools -race -count=1 ./fuku/db/greetings ./fuku/db/rules ./fuku/utils/content ./fuku/utils/formatting` |
| Full suite command | `make test` (never a bare `go test ./...`), then `make lint`, `make check-translations`, `make generate-docs` then `make check-docs` |

### Phase Requirements to Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| EDIT-01 | Three pages register, Main grid `[Welcome | Goodbye] [Rules] [Access]`, status lines, state lines per part, Set/Remove drawn per state, Remove only while set, Preview buttons only for set parts, Open-in-private row only in a group menu, at most 8 rows | integration | `go test -tags testtools -race -count=1 -run '^(TestSettingsEditPages|TestSettingsMainGrid|TestSettingsEditRegistry)$' ./fuku/modules` | ❌ Wave 0 `settings_edit_test.go`; `settings_main_test.go` updated |
| EDIT-01 | Pages and Main read without creating rows; a failed read is the cannot-open edit; no-row defaults equal the runtime (Goodbye On) | integration + db | `-run '^(TestSettingsEditReadsCreateNothing|TestSettingsEditReadFailure|TestSettingsEditDefaultsMatchRuntime)$' ./fuku/modules`; `-run '^(TestLoadGreetingSettingsCreatesNothing|TestLoadRulesCreatesNothing)$' ./fuku/db/greetings ./fuku/db/rules` | ❌ Wave 0 |
| EDIT-01 | Group menu: Set and Remove write nothing, show `settings_edit_group_note` and Open in private; no wait key created | integration | `-run '^TestSettingsEditGroupMenuRefusal$' ./fuku/modules` | ❌ Wave 0 |
| EDIT-01 | Forged `t`: missing, unknown, `m` on Rules, non-ASCII, extra key refused before any `getChatMember` | unit/integration | `-run '^(TestSettingsEditCallbackFields|TestSettingsPageActionValidate)$' ./fuku/modules` | ❌ Wave 0 |
| EDIT-02 | Set writes the wait (10 min TTL, fields c/p/t/m/n, marker 15 min), edits into the prompt; Set on another part replaces and says so; same part refreshes without the note | integration | `-run '^(TestSettingsEditSetWritesWait|TestSettingsEditReplaceWait)$' ./fuku/modules` | ❌ Wave 0 |
| EDIT-02 | Set on replica A, reply on replica B (second handler instance, shared miniredis) | integration | `-run '^TestSettingsEditWaitAcrossReplicas$' ./fuku/modules` | ❌ Wave 0 (`otherReplica`) |
| EDIT-02 | Cancel button, refusal-message Cancel, `/cancel` (private, group ignored, none), stale Cancel leaves the live wait, expiry via `FastForward`, marker reply once, no marker after save/cancel | integration | `-run '^(TestSettingsEditCancelButton|TestSettingsEditCancelCommand|TestSettingsEditStaleCancel|TestSettingsEditWaitExpiry|TestSettingsEditMarkerReplyOnce)$' ./fuku/modules` | ❌ Wave 0 |
| EDIT-02 | Door passes through: no wait and no marker, service identity, edited message, bot, command, caption command, no Redis; one Redis read, zero writes, zero Telegram calls | integration | `-run '^TestSettingsEditDoorPassThrough$' ./fuku/modules` | ❌ Wave 0 |
| EDIT-02 | Door authority: same decision as a press for the same member record in the four owner worlds plus unknown (sixth door in `TestSettingsGateAllDoors`); lost access ends the wait; 429/5xx stays | integration | `-run '^(TestSettingsGateAllDoors|TestSettingsEditDoorAuthority)$' ./fuku/modules` | existing, one door added; ❌ Wave 0 |
| EDIT-02 | Double delivery and a 3-item album write once; refused album gets one refusal; expired wait plus album gets one reply | integration | `-run '^(TestSettingsEditDoubleDelivery|TestSettingsEditAlbumWritesOnce|TestSettingsEditRefusedAlbumOneRefusal)$' ./fuku/modules` | ❌ Wave 0 |
| EDIT-02 | Refusal never consumes the wait (TTL not extended); failed write releases the claim | integration | `-run '^(TestSettingsEditRefusalKeepsWait|TestSettingsEditWriteFailureReleases)$' ./fuku/modules` | ❌ Wave 0 |
| EDIT-03 | Text keeps entities (bold, link, spoiler, emoji, RTL, overlapping) and placeholders; stored HTML equals the command's | integration | `-run '^TestSettingsEditTextParity$' ./fuku/modules` | ❌ Wave 0 |
| EDIT-03 | Limits: stored form over 4096 (`content_text_too_long`), over 1024 with media (`content_caption_too_long`), filled-in worst case over (`settings_edit_refuse_too_long_filled`), minimal-user empty, text with button lines, media over a long text | integration + unit | `-run '^(TestSettingsEditTextLimits|TestSettingsEditFilledLength|TestSettingsEditTextButtonLines|TestSettingsEditMediaTextLong)$' ./fuku/modules`; `-run '^(TestFillPlaceholders|TestVisibleLength)' ./fuku/utils/formatting`; `-run '^TestTextLengthRefusal$' ./fuku/utils/content` | ❌ Wave 0 |
| EDIT-04 | Each accepted media kind stores the commands' `file_id` and type (GIF as DOCUMENT); refused kinds (video note, contact, poll, text); caption ignored with notice; album keeps one and says so; sticker note | integration | `-run '^(TestSettingsEditMediaParity|TestSettingsEditMediaKinds|TestSettingsEditCaptionIgnored|TestSettingsEditAlbumNotice)$' ./fuku/modules` | ❌ Wave 0 |
| EDIT-05 | Buttons: valid replace the set, `:same`, default label, empty name; refuse non-http, no host, none readable, over 100 / 8 per row, count includes `{rules}`; stored equals the command's | integration | `-run '^(TestSettingsEditButtonsParity|TestSettingsEditButtonRefusals)$' ./fuku/modules`; `-run '^TestValidButtonURL$' ./fuku/utils/content` | ❌ Wave 0 |
| EDIT-06 | Preview text/media/buttons and Full preview go only to the presser's private chat with header, filled with the presser's name and the group title; write nothing (row, `LastMsgId` unchanged, no delete); group menu toast; need-start (403) and failed alerts; Rules full preview equals `/rules` composition | integration | `-run '^(TestSettingsEditPreview|TestSettingsEditFullPreview|TestSettingsEditPreviewWritesNothing|TestSettingsEditPreviewErrors|TestSettingsEditRulesPreview)$' ./fuku/modules` | ❌ Wave 0 |
| EDIT-10 | Switches store the same row and the same cache key as the commands; absolute values; failure notice keeps old state; rights per control | integration | `-run '^(TestSettingsParityGreetingSwitches|TestSettingsDriftGreetings|TestSettingsAuthorityParityGreetings)$' ./fuku/modules` | ❌ Wave 0 |
| EDIT-11 | Per part the stored row equals what one `/setwelcome`, `/setgoodbye`, `/setrules` stores (text/media/buttons/Rules) and Remove equals the reset/clear commands; commands still pass their existing tests | integration | `-run '^(TestSettingsEditTextParity|TestSettingsEditMediaParity|TestSettingsEditButtonsParity|TestSettingsEditRulesParity|TestSettingsEditRemoveParity)$' ./fuku/modules` + existing `TestSetAndResetGreetingTextCommands`, `TestSetRulesStoresCommandTextAndReplies` | ❌ Wave 0; existing |
| EDIT-11 | Rights: Welcome/Goodbye Set/Remove/Delete-last need change-info (alert), Rules need bot admin (notice), switch/Preview/Cancel admin only | integration | `-run '^TestSettingsAuthorityParityEdit$' ./fuku/modules` | ❌ Wave 0 |
| UI / locale | 83 keys (the UI-SPEC's 79 plus the four of Q9) in 8 locales, ceilings, alerts <= 160 and no markup, tokens `{first}` ... `:same` `buttonurl://` byte-identical in the prompts, help text fits one message | unit | `-run '^(TestSettingsLocaleKeys|TestSettingsLabelLengths|TestSettingsEditLocaleTokens|TestSettingsHelpFitsOneMessage)$' ./fuku/i18n` | existing + new test |
| UI / size | Every page, prompt, refusal and saved message <= 3800 UTF-16 units in 8 locales with a 64-rune multibyte title; every callback of every screen <= 64 bytes with `Omitted` 0 | integration | `-run '^(TestSettingsEditPagesFitEveryLocale|TestSettingsEditCallbackBytes)$' ./fuku/modules` | ❌ Wave 0 |
| Privacy | No new screen names the Staff Group, in a group menu and in a non-creator private menu, 8 languages | integration | `-run '^TestSettingsStaffPrivacy$' ./fuku/modules` | existing, extended |

### Sampling Rate
- **Per task commit:** the focused `-run` of the task's tests (under 30 s each), plus `go test -tags testtools -race -count=1 ./fuku/utils/content ./fuku/utils/formatting` when those packages change.
- **Per wave merge:** `go test -tags testtools -race -count=1 ./fuku/modules ./fuku/i18n ./fuku/db/greetings ./fuku/db/rules ./fuku/utils/...` plus `make lint`.
- **Phase gate:** `make test`, `make lint`, `make check-translations`, `make generate-docs` then `make check-docs`; manual device checklist (GIF rendering, tap-to-copy `<code>` examples, `/cancel` tap, label truncation in hi/ru/uk). Nothing here joins `make test-postgres-integrity` (no migration, no index, no CHECK, no conditional update on PostgreSQL).

### Wave 0 Gaps
- [ ] `fuku/modules/settings_edit_env_test.go`: `newSettingsEditEnv` (wraps `newSettingsEnv`, seeds a greetings/rules cleanup), `sendPrivateMessage(from, *gotgbot.Message)`, `sendPrivateAlbum(from, items)` (same `MediaGroupId`, concurrent), `shiftEntities(text, prefix)` (UTF-16 shift for command parity), `settingsEditAdminJSON(userID, canChangeInfo)`, fake responses for `sendSticker/sendVideo/sendAudio/sendVoice/sendAnimation`, wait readers over `env.redis`, `editCleanup(chatIDs...)` deleting `GreetingSettings` and `RulesSettings` rows
- [ ] `fuku/modules/settings_input_test.go`, `settings_edit_test.go`, `settings_edit_check_test.go`, `settings_edit_preview_test.go`
- [ ] `fuku/db/greetings` and `fuku/db/rules`: tests for the new loaders and column writers (SQLite, `testdb.Run`)
- [ ] `fuku/utils/content`, `fuku/utils/formatting`: tests for the new exported helpers (and keep the existing ones green)
- [ ] `settings_gate_test.go`: one more door and driver in `TestSettingsGateAllDoors`
- [ ] `fuku/i18n/settings_locale_test.go`: limits rows for every new `settings_` key (83) and the `settings_help_msg` ceiling check, new alert keys in `settingsAlertKeys`, label rows for the three grid names, new `TestSettingsEditLocaleTokens`
- [ ] Update `settings_main_test.go` expectations (grid, status lines)
- [ ] `AGENTS.md`: Handlers (group 1, the door, `/cancel`), Callbacks (the three pages' codes, `settingsRightChangeInfo`), Permissions, Data (three key families), the locale count; `make generate-docs`

## Security Domain

### Applicable ASVS Categories (level 1)

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | no (Telegram identifies the user) | n/a |
| V3 Session Management | yes, the wait is the only server-side state | per-user key from `msg.From.Id`, nonce claim, 10-minute TTL, no timer |
| V4 Access Control | **yes** | live `getChatMember` + `settingsGate` + part right at the Set press and again at the typed reply; fail closed; service identities never start or answer a wait |
| V5 Input Validation | **yes** | strict `Validate` of `t`, `settingsCheckFields`, door candidate checks, `media_group_id` `^[0-9]{1,20}$`, button URL http/https+host, length and button limits, recover around entity conversion |
| V6 Cryptography | no (the 16-hex nonce is a uniqueness token, `crypto/rand` as `newStaffActionToken`, `staff_action_card.go:322-329`) | `crypto/rand`, never `math/rand` |
| V7 Error handling / logging | yes | no text, names, URLs, file ids in logs; Telegram wording never reaches the user; refusals by class |
| V8 Data protection | yes | Redis holds only ids and codes; stored text is never echoed in pages |

### Known Threat Patterns for this stack

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| A typed message used by someone who is no longer an admin (wait outlives access) | Elevation of privilege | Door repeats live `getChatMember`, the gate and the right before the write; refusal ends the wait (S-10 steps 4-6) |
| A wait hijacked across users | Spoofing | Key is `fuku:settings:input:<msg.From.Id>`, the sender must equal the private chat id; a user can only ever consume their own wait |
| A wait aimed at another group | Tampering | `c` in the value comes from the verified press (`p.group.Id`), never from the typed message; forged callbacks `c` go through `settingsPress.verify` (live admin check, gate, getChat group type) |
| Forged `ws`/`wr`/`wv`/`wc` callbacks (wrong `t`, wrong page, missing owner field, a group menu `u` replay) | Tampering | `settingsCheckFields` + `Validate` before I/O; group-menu opener check; Set refuses in a group menu; `t` must belong to the page |
| Double delivery / album races writing twice | Tampering | claim by nonce (atomic), album `SET NX`, finish only by the claim holder |
| Stale Cancel deleting a newer wait | Tampering | cancelScript compares `c`, `p`, `t` and refuses while a claim is held |
| Redis key exposure | Information disclosure | keys hold ids and codes only; no text, no `file_id`; family outside the cache prefix |
| HTML injection through stored text | Tampering | Stored text is owner-authored formatting by design; joiner-supplied values are `html.EscapeString`-ed in one pass (`formatting.go:272-281`, `188-216`); a page shows only states and lengths, never the stored HTML |
| HTML or markup through button names or errors | Tampering | Refusals name the button's number only; button labels are never echoed; user text is spliced after translation |
| SSRF-like button URLs (`http://169.254.x.x`, `file:`, `javascript:`) | Tampering / Information disclosure | Only http/https with a host are accepted; the bot never fetches a button URL; link previews are disabled for greetings (`sender.go:109-111`); members' clients open the link (accepted: owner-authored, as with the commands) |
| A refusal revealing whether the bot is in a chat | Information disclosure | every typed-reply refusal follows a wait that only a proven admin could start; `settings_edit_refuse_access_lost` names no group; 4xx all give one text |
| Oversized or malformed entities panicking the converter | Denial of service | recover around the conversion, treat as a refusal; handler-level `RecoverFromPanic` |
| Door as a load amplifier | Denial of service | cheap checks first; one Redis round trip per private message; Telegram calls only for users with a wait; album writes only with a wait or marker |
| Callback data overflow shipping a dead button | Denial of service (UX) | `encodeCallbackData` returns `""`, `Omitted` counted, worst-case test (largest new button 45 bytes) |
| Logs carrying message text | Information disclosure | log ids, part, outcome only; a test asserts no log line contains the sample text (use a logrus test hook) |

## Sources

### Primary (HIGH confidence)
- Repository files read this session: `fuku/modules/greetings.go`, `rules.go`, `rules_format.go`, `settings.go`, `settings_menu.go`, `settings_pages.go`, `settings_gate.go`, `settings_antiflood.go`, `settings_access.go`, `staff_action_card.go`, `raid_flood.go`, `lockdown.go` (`lockdownLiveMember`), `captcha.go` (apply result, pending handler), `federations.go`, `notes.go`, `users.go`, `staff_watchers.go`, `i18n_helpers.go`; `fuku/utils/content/content.go`, `formatting/formatting.go`, `media/sender.go`, `keyboard/keyboard.go`, `helpers/command_pipeline.go`, `helpers/telegram_helpers.go`, `chat_status/access.go`, `chat_status/chat_status.go` (`IsUserConnected`), `captchaweb/challenge.go`, `cache/cache.go`, `cache/restrictedCache.go`; `fuku/db/db.go`, `db/greetings/repository.go`, `db/rules/repository.go`, `db/models/{greetings,rules,types}.go`, `db/cache/local.go`, `fuku/i18n/translator.go`, `fuku/i18n/settings_locale_test.go`; test toolkit `settings_env_test.go`, `test_harness_test.go`, `staff_helpers_test.go`, `staff_action_fake_test.go`, `lockdown_fake_test.go`, `testredis_test.go`, `settings_gate_test.go`, `greetings_command_test.go`; `migrations/20250805200527_initial_migration.sql`; `go.mod`.
- gotgbot source at the pinned version: `formatting.go`, `ext/dispatcher.go`, `ext/handlers/{message,command}.go`, `custom_helpers.go`, `gen_types.go`, `gen_methods.go` (embedded Bot API docs).
- `gotg_md2html` source: `md2htmlV2.go`, `stripV2.go`. miniredis command registrations.
- Scratch runs this session (files deleted, `git status` clean): entity/stored-form and button parse; rules double escape; greetings default (Goodbye ON, row created by a read); Lua wait scripts, `GETDEL`, `SetNX`, `FastForward` on miniredis; baseline `TestSettingsGateAllDoors` etc. green.
- `.planning/phases/11-message-editor/11-CONTEXT.md`, `11-UI-SPEC.md`, `.planning/REQUIREMENTS.md`, `.planning/ROADMAP.md` (Phase 11), `.planning/research/PITFALLS.md` (Pitfalls 10-12), `10-RESEARCH.md` (format and conventions), `AGENTS.md`, `.claude/CLAUDE.md`.

### Secondary (MEDIUM confidence)
- v1.1 `PITFALLS.md` statements tagged `[TG]` (100 buttons, UTF-16 counting folklore, custom-emoji limits), not re-probed.

### Tertiary (LOW confidence)
- Telegram-side behaviours A1-A3, A5-A8 [ASSUMED]; `core.telegram.org` unreachable from this session.

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH, nothing new, versions from `go.mod`
- Architecture: HIGH, read from code and exercised by scratch runs (stored form, scripts, defaults)
- Pitfalls: HIGH for the repo-derived ones (1-10, 13-15), MEDIUM for Telegram-side ones (A1-A9)

**Research date:** 2026-11-04
**Valid until:** 2026-12-04 (the code moves with every v1.1 phase; before planning re-check `settingsCoreActions`, the Main renderer, the highest migration and `settings_env_test.go` if another phase merged first)
