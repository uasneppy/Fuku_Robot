# Phase 19: Blacklist, Filter and Note Pages - Research

**Researched:** 2026-11-13
**Domain:** Go Telegram bot (gotgbot v2.0.0-rc.36, GORM on PostgreSQL / SQLite tests, Redis): three paged list pages on the settings framework of Phases 9 to 18 (Blacklist, Filters, Notes), with delete by row ID, add by typed message through the Phase 11 typed-reply door (one step for a blacklist word, two steps for a filter or a note), and the blacklist action, each write going through one apply function shared with its command
**Confidence:** HIGH for every code fact (each discrete value below was read this session and is quoted with file and line); HIGH for the matcher and extraction behaviour (measured here with throwaway programs against the real packages); MEDIUM for the PostgreSQL CHECK situation of `none` (read from the migration chain, not run against a database); MEDIUM for label widths and the Main grid on a real phone (live checks).

Provenance tags: `[VERIFIED: file:line]` = file opened with Read (or printed) this session, value quoted verbatim beside the claim; `[VERIFIED: run]` = I ran it here (throwaway programs in the scratchpad, output pasted); `[CITED: file]` = project planning text; `[ASSUMED]` = reasoning or estimate, listed in the Assumptions Log.

<user_constraints>
## User Constraints (from CONTEXT.md)

The decisions are the owner's provisional defaults (CONTEXT "Provisional status": skip-discuss is on, none confirmed; each becomes an owner confirmation A19-n in `19-SPIKE.md`). Where the code disagrees with one, this file says so as a numbered correction C-nn with a recommended amendment and does not deviate silently.

### Locked Decisions (copied verbatim from 19-CONTEXT.md "Implementation Decisions (defaults, unconfirmed)")

- **D-01 The lists are private-menu only.** A group menu is readable by every member, and blacklisted words, filter keywords and note names can be sensitive or abusive. In a group menu each page draws a short note and the Open in private row and never an entry, a count of entries or a word (the Phase 11 rule that no group menu creates a wait or shows content). The three cells stay on Main in both menus, so the grid stays one shape.
- **D-02 One setting, one code path, golden first.** Each command's write is extracted behind an apply function beside the command (add, remove, and for the blacklist the action) returning a typed result; the command maps the result to its unchanged reply, the page to a notice or a redraw. A golden of each command's requests and stored rows is committed ALONE before its extraction and never re-recorded (the Phase 18 rule, with the lesson of 85ca132: a golden that reads `config.AppConfig` pins the value it needs and passes bare and in CI's environment). Each list is read by a new non-creating, error-returning reader that shares the command's cache key and value type; a getter that hides read errors is never used by a page.
- **D-03 Rights are the commands'.** Each control declares the right its command asks for, judged on the press's live `getChatMember` record (never the cached admin list), and a typed reply is judged exactly like a press by the door. The research reads the real rights of each command (they differ: a blacklist command may ask for the bot as an administrator, `/filter` and `/save` may ask for change-info) and the plan maps each to a `settingsRight`, adding a composite right only if a command's order needs it.
- **D-04 The typed input is Phase 11's door, extended, not a second door.** The wait state (`fuku:settings:input:<user>`, ten minutes, claim, cancel, album gate, `/cancel`) stays; the new pages add parts or steps to it. The door's rule that the wait holds IDs and codes only, never message text, meets the two-step flows: the keyword typed first must be held until the reply arrives. Where it is held is the main design question (R-1).
- **D-05 Paging and identity.** Lists are paged with a cursor like the Locks pager or the staff history; at most 8 rows a page, so the keyboard and the 3800-unit text stay inside the limits in 8 languages. An entry is identified in callback data by its numeric row ID (every table has one), never by its text, so a button is at most 64 bytes through `callbackcodec`. An ID is checked against the pressed menu's chat before any delete (a forged ID of another chat deletes nothing), and an entry that is already gone is "already removed", not an error.
- **D-06 Deleting asks first.** A delete uses the shared two-tap card (`settingsCardRows`, expiry minute `e`, the object's ID bound to the card, the whole press pipeline again at Yes), because a stray tap in a long list is easy and a filter or note holds content that cannot be restored from the menu. A blacklist word is cheap to add again; the default is still the card for all three, for one rule.
- **D-07 Validation lives in the shared function.** What is refused when adding a blacklist word or a filter keyword (empty or blank, only wildcard or punctuation characters, anything the matcher turns into "matches every message") is decided once, in the apply function, so the command and the page refuse alike. If today's command accepts such input, the shared check tightens the command: that is an owner-visible behaviour change, recorded as an owner confirmation, and the golden pins today's behaviour for everything else. The research measures what the matchers really do (R-2).
- **D-08 The blacklist action.** The page shows the current action and sets it through the `/blacklistaction` apply function, with the command's allowed set (the model's CHECK) and its refusals. The action is stored per row today (a column on every word); the page reads and writes it as the command does.
- **D-09 No migration expected.** The tables exist and every row has a numeric ID. If the research finds a need (an index for a paged query), it follows the Migration rules (append-only, timestamped, one transaction, both appliers) and is named in the plan.
- **D-10 Staff Group.** The pages are not `Hidden` by default (as in Phase 18); the research confirms the commands work in a Staff Group and that nothing about a Staff Group is named by a screen (D-21 of Phase 18: no screen names the Staff Group).
- **D-11 Strings and limits.** All new strings in the 8 locale files with rows in the limits tables and label-length tests; a button label is plain text (the hub's rule: collapsed spaces, cut at a fixed number of code points, never HTML), user text in a message is escaped and capped; every callback at most 64 bytes in the worst case.
- **D-12 Tests stay light and honest.** Per page: golden first, parity, drift, authority parity (menu and typed reply), forged fields, callback bytes, fit in 8 languages (the worst entry, not a cross product), reads-create-nothing, privacy (no word, keyword or note text in a group menu), and an end-to-end test through the real dispatcher (the blacklisted word is acted on, the filter answers, the note is fetched, after a menu add). Whole-phase test time budget: about 40 seconds added to the `fuku/modules` run, no single test over about 10 seconds, every test green bare and in CI's environment (`cienv.sh`).
- **D-13 Order of plans.** Strings and the shared pieces first (the list and delete helpers, the two-step flow), then Blacklist end to end as the tracer (list, delete, add by one typed message, action), then Filters, then Notes, then the cross-page sweeps and the end-to-end test, then the closing plan (rulebook, README, docs, `19-SPIKE.md`, the phase gate, PAGE-07 to PAGE-09).
- **D-14 Requirements.** PAGE-07, PAGE-08 and PAGE-09 are completed only by the closing plan, after the end-to-end test and the phase gate pass, never by an earlier plan.

### Claude's Discretion

19-CONTEXT.md has no separate "Claude's Discretion" section. By the Phase 18 precedent the researcher and the UI-SPEC decide: page keys, `Order` values, icons, labels, locale key names, the exact list layout (numbered delete buttons or labelled ones), the page size within D-05's "at most 8", and whether the blacklist action sits on the list page or on its own screen.

### Deferred Ideas (OUT OF SCOPE) (copied verbatim from 19-CONTEXT.md "Phase Boundary")

Out of scope: search inside a long list (MENU2-04, deferred), editing an existing filter's or note's content from the menu, bulk removal (`/stopall`, `/clearall`, `/remallbl`) and the private-notes toggle (`/privnote`), which stay commands, linking anything, and changing what a command accepts except where a decision below says so.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| PAGE-07 | A Blacklist page lists the group's blacklisted words (paged), deletes one, adds one by typing it in PM, and sets the blacklist action. | S-01 (command facts), S-02 (validation), S-06 (paging and bytes), S-10 (right: new composite), S-11 (action semantics, C-02), S-08 (one-step add through the door), Plans 19-01 to 19-03 |
| PAGE-08 | A Filters page lists filters (paged), deletes one, and adds one by typing the keyword and then the reply in PM. | S-01, S-02, S-03 (two-step state), S-04 (extraction parity), S-10 (right: `settingsRightChangeInfo`), C-09 (existing keyword), Plans 19-04, 19-05 |
| PAGE-09 | A Notes page lists notes (paged), deletes one, and adds one by typing the name and then the content in PM. | S-01, S-03, S-04, S-10 (right: `settingsRightChangeInfoBotAdmin`), C-11 (note names), Plans 19-06, 19-07 |
| PAGE-10 (bind, already complete) | Every setting changed on these pages is stored, cached and refused exactly as by its command | S-01 apply designs, S-05 cache keys, Validation Architecture (parity, drift, authority parity) |
| PLAT-05 (bind) | New strings in 8 locales with limits tables | C-12 (key prefixes), S-07, Validation Architecture |
</phase_requirements>

## Project Constraints (from CLAUDE.md / AGENTS.md)

- Stay on Go 1.26.0, gotgbot v2, GORM, Redis; no new dependency; no mock library; tests use real fixtures (`internal/testdb.Run`, miniredis, hand-written `gotgbot.BotClient` fakes), always `-tags testtools`, `make test` is the only full run, no `t.Skip` to dodge a case. [CITED: AGENTS.md "Testing", "Commands"]
- Every write `cache.DeleteCache`s the keys it affects; a freshness-critical key goes in `skipLocal`; migrations append-only, timestamped, one transaction per file, both appliers agree; locale keys in all 8 files; fire-and-forget goroutines recover from panics; never discard a DB error on a state-changing path. [CITED: AGENTS.md "Data", "Migrations", "Go rules"]
- Callback data only through `callbackcodec` (`<ns>|v1|<url-encoded>`, 64-byte cap); `encodeCallbackData` returns `""` on overflow, which ships a dead button; user text never travels in callback data. [CITED: AGENTS.md "Callbacks"]
- One setting, one code path: one apply function per setting beside its command, returning a typed result; pages read through the getter their command reads, and a read error is the cannot-open edit; the user check is live (stricter than the commands' cached one) and the bot check uses the commands' own predicate. [CITED: AGENTS.md "Subsystem traps" "One setting, one code path", "Permissions"]
- The typed-reply door: no log line carries message text, a caption, a button name, a link or a file ID; the wait hash holds IDs and codes only. [CITED: AGENTS.md "Handlers" (settings typed-reply door), "Data" (`fuku:settings:input:<user>`)]
- A menu opened in the group never names the Staff Group, and its text is readable by every member of the group. [CITED: AGENTS.md "Subsystem traps"]
- Lint traps: gocyclo minimum 20, dupl threshold 100 (three near-identical page files will trip dupl: build one list engine, see Architecture), gosec G101 on identifiers containing token, secret or pass. [CITED: 18-RESEARCH.md "Project Constraints"; `golangci-lint` 2.5.0 here is built with go1.25.1, so `make lint` cannot run in this sandbox: VERIFIED: run, `golangci-lint --version`]
- Commits: Conventional Commits; user-visible changes need `feat:` or `fix:`. [CITED: AGENTS.md "Commits"]

## Summary

Phase 19 needs **no new package and no new dependency**. It adds one generic list-page engine plus three thin page registrations (Blacklist, Filters, Notes), seven apply functions extracted beside their commands (blacklist add, remove, action; filter add, remove; note add, remove), chat-scoped fresh readers and delete-by-ID functions in the three repositories, one new composite `settingsRight` for the blacklist commands, one optional Main hook so a private menu can show a count while a group menu shows none, and an extension of the Phase 11 typed-reply door: a list branch, a one-step part (blacklist words) and a two-step flow (keyword or name first, held in one new bounded field of the existing wait hash, then the reply or content). About 60 to 75 new locale keys. A migration is needed **only if** the PostgreSQL chain still carries the old blacklist CHECK without `none` (C-16, a live check that decides it).

The decisions hold with **sixteen corrections** (C-01 to C-16). The ones that change the design: (1) the blacklist action is stored on every word, every new word is stored with `warn` whatever the chat chose, `/blacklistaction` silently does nothing on an empty list while replying success, and the command's "current mode" is the action of whichever row the database returns first, so "reads and writes it as the command does" would make the page lie (C-02); (2) nothing in this engine matches literally every message (it is case-insensitive literal substring matching through Aho-Corasick, an empty pattern matches nothing, `*` and `.*` are literal), so the real footgun is a single common character, and the proposed shared refusal is "one code point that is not an emoji or other symbol" (C-03); (3) a delete Yes carrying a 19-digit row ID is 66 bytes and does not fit; it fits at 17 digits (64 bytes), the bound Lift and Unlink already use (C-04); (4) the notes list cache holds no row IDs and the filter keyword cache holds none either, so the pages read fresh, chat-scoped `(id, text)` rows instead of sharing the commands' keys (C-01); (5) `/filter` and `/save` of an existing keyword open an overwrite card that stores the whole content in Redis, which the menu cannot offer because editing content is out of scope, so the page refuses an existing keyword at the first step (C-09); (6) the door is wired to `settingsEditPageDef` in about ten places, so "extended, not a second door" is a real refactor of the door with a list branch, which must keep the editor path byte-identical (C-06).

**Primary recommendation:** hold the pending keyword or name in one new field `w` of the existing wait hash (written by one new atomic "advance" Lua script that also rotates the nonce), never in callback data and never in a log; build one list engine (`settings_list.go`) driven by a table per page; delete by `DELETE ... WHERE id = ? AND chat_id = ? RETURNING` with the command's own cache drops; draw entries as escaped, capped text lines with numbered delete buttons (no user text in labels); bound row IDs at 17 decimal digits; and order the plans Blacklist goldens and extraction, then the list engine with the Blacklist page as the tracer, then the door with the one-step add, then Filters (goldens, extraction, the two-step flow), Notes, the sweeps, the end-to-end test and the closing plan.

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| List page drawing, paging, delete card | Bot handler (settings callback, `fuku/modules/settings_list*.go`) | Database (fresh chat-scoped read) | Pure renderer over a fresh read; the menu keeps no state of its own |
| Delete by row ID | Repository (`fuku/db/{blacklists,filters,notes}`) | Cache (`DeleteCache` of the command's keys) | The conditional `DELETE ... WHERE id AND chat_id` is the authority; a forged ID deletes nothing |
| Add (validation, refusals, write, cache drop) | Apply function beside each command (`blacklists.go`, `filters.go`, `notes.go`) | Repository | One setting, one code path: the command and the page call the same function |
| Pending keyword between the two typed steps | Redis (the existing wait hash, new field `w`) | — | The wait is already the per-user, per-replica-shared, TTL-bound state of the door |
| Authority of a press or typed reply | Live `getChatMember` + owner gate (existing pipeline and door) | Commands' cached bot predicates for the bot half | Rule of the milestone: the per-group admin check is never skipped |
| "Works in the group at once" | Existing watchers (blacklist group 7, filters group 9, notes `#` and `/get` group 0) | Cache drop of the command keys | No new handler; the write drops the keys the watchers read |

## Standard Stack

### Core (all existing; no new dependency)

| Library | Version | Purpose | Why |
|---------|---------|---------|-----|
| `github.com/PaulSonOfLars/gotgbot/v2` | v2.0.0-rc.36 (pseudo-version `rc.36.0.20260919140833-240296efadb4` in the module cache) | `Message.OriginalMDV2()`, `Context.Args()` (= `strings.Fields(GetText())`) | the commands' own message conversion [VERIFIED: run, `go list -m`; gotgbot `ext/context.go:183-193`, `formatting.go:48-50`] |
| `gorm.io/gorm` | v1.31.2 | `clause.Returning{}` for delete by ID (precedent `memberreq.TakeHoldByID`) | [VERIFIED: fuku/db/memberreq/holds.go:139-141] |
| `github.com/redis/go-redis/v9` | v9.22.0 | wait hash scripts | existing |
| `github.com/cloudflare/ahocorasick` | v0.0.0-20240916140611-054963ec9396 | the blacklist and filter matcher (unchanged) | [CITED: CLAUDE.md stack list] |
| `fuku/utils/content` | in repo | `ExtractNoteAndFilter`, `TextLengthRefusal`, `ValidButtonURL` | the commands' extraction, reused for typed content (S-04) |
| `fuku/utils/callbackcodec` | in repo | `settings|v1|...` data, 64-byte cap | [VERIFIED: fuku/utils/callbackcodec/callbackcodec.go `MaxCallbackDataLen = 64`] |

**Installation:** none.

**Version verification:** no package is installed by this phase; nothing to verify against a registry.

## Package Legitimacy Audit

No external package is added. Packages removed (SLOP): none. Packages flagged (SUS): none. The planner needs no `checkpoint:human-verify` install task.

## Corrections to CONTEXT.md (the planner must carry these)

| # | Where | Problem (evidence) | Recommended amendment |
|---|-------|--------------------|-----------------------|
| C-01 | D-02 "shares the command's cache key and value type" | The commands' list caches do not carry row IDs for two of the three lists: the notes list caches `type cachedNoteInfo struct { Name string; AdminOnly bool }` [VERIFIED: fuku/db/notes/repository.go:76-79] under `cache.CacheKey("notes_list", chatID)` [VERIFIED: notes/repository.go:81-83]; the filter keyword list caches `[]string` of keywords [VERIFIED: fuku/db/filters/repository.go:34-49] under `filter_list`. Only the blacklist cache (`BlacklistSettingsSlice`, rows with `ID`) and `filters_optimized` carry IDs; the ID survives the msgpack round trip [VERIFIED: run, msgpack round trip printed `<nil> 42 x` and `7`]. Changing `cachedNoteInfo` would change a cached value type during a rolling deploy. Also every one of the commands' list getters hides its read error: `GetBlacklistSettingsContext` returns an empty slice on error (repository.go:82-84), `GetFiltersListContext` returns `[]string{}` (50-52), `GetNotesListContext` returns `nil` (106-108). | Read each list for the page with a **fresh, chat-scoped, error-returning query of `(id, text[, action / admin_only / msgtype])`** that creates nothing (`ListEntriesFresh` per repository), sort in Go, page in Go. It is what the commands will read once their key is dropped, it is fresher across replicas than a cached read (none of the three keys is in `skipLocal`), and it needs no cache-shape change. The drift test still proves "the page draws what the command stored". Record as A19-1. |
| C-02 | D-08 "reads and writes it as the command does" | (a) Every new word is stored with action `warn` whatever the chat chose: `Action: "warn",` in `AddBlacklist` [VERIFIED: fuku/db/blacklists/repository.go:15-20]. (b) `SetBlacklistAction` is `db.DB.Model(&models.BlacklistSettings{}).Where("chat_id = ?", chatId).Update("action", strings.ToLower(action))` [VERIFIED: repository.go:57]: it changes only the rows that exist, so on an empty list it stores nothing while `/blacklistaction ban` replies "Successfully Changed blacklist mode" [VERIFIED: blacklists.go:313-324]. (c) The command's "current mode" is `bss[0].Action` of the slice in database order, default `"warn"` [VERIFIED: fuku/db/models/blacklists.go:28-33]; the load has no `ORDER BY` (repository.go:75). (d) The help text says the default is `none` ("The Default mode for Blacklist is *none*, which will just delete the messages", locales/en.yml:265-267) while the code stores `warn`. So after "set Ban, then add a word" the list holds Ban and Warn rows, the watcher acts per row (`switch matched.Action`, blacklists.go:505), and the command's status shows whichever row comes first. | The page draws the action **only when every row agrees**, and otherwise a "mixed" line (count of words on another action; a choice sets every word). On an empty list it draws no action choices and a line saying the action is stored with each word. The apply function returns the rows affected (new `SetBlacklistActionContext(ctx, chat, action) (int64, error)`); the command maps "0 rows" to its unchanged success reply (the golden pins it), the page to the empty-list line. Owner question **A19-3 (recommended YES)**: a new word takes the chat's action when every existing row agrees (else `warn`), in `applyBlacklistAdd`, so the page's promise "sets the blacklist action" holds for words added afterwards. If confirmed it is an owner-visible change of `/addblacklist` and the golden must not contain a "chat had a non-warn action" case (a separate test pins the new rule). |
| C-03 | D-07 "matches every message" | Measured on the real matcher: patterns are lowercased (matcher.go:46-51), the text is lowercased (61), and `MatchThreadSafe` finds literal substrings (74). An empty pattern matches nothing; `*` and `.*` match only messages that contain those characters; `hell` matches `shell`; `hello world` does not match `hello  world` [VERIFIED: run, probe output below]. So no stored word can match literally every message. The real footgun is one code point: `a` matches `what` and `CAT`; one Cyrillic letter matches most Ukrainian text. Today `/addblacklist` refuses only words over 100 runes (`if len(runes) > 100 {`, blacklists.go:86) and `/filter` only keywords over 100 runes (`if len([]rune(filterWord)) > 100 {`, filters.go:125). Blacklist words cannot contain whitespace (they come from `ctx.Args()`, a `strings.Fields` split); a filter keyword can (the reply form joins the args with one space, content.go:93). | Shared refusal, in one helper beside each command, used by the command and the page: **refuse a normalised word or keyword that is exactly one code point, unless that code point is in Unicode category So (emoji and other symbols)**; also refuse an empty or blank one (only reachable from the page). Keep the 100-rune limit. New reply keys `blacklists_blacklist_word_too_broad` (listed like the too-long words, in the same pre-pass, as a separate reply) and `filters_keyword_too_broad`. This tightens both commands: owner confirmation **A19-2**. The goldens must **not** contain a single-code-point case (recording it would force a re-record when the rule lands); the rule has its own test. |
| C-04 | D-05 "a button is at most 64 bytes" with "a row ID up to 19 digits" | Computed with the real codec: the delete Yes `settings|v1|a=by&c=-1001234567890&e=29412345&r=9223372036854775807` is **66 bytes, over the cap**; with a 17-digit ID it is exactly 64 [VERIFIED: run, table in S-06]. A Yes cannot also carry a page number (64 bytes already with a 9-digit ID plus a 5-digit page). | Bound row IDs at **17 decimal digits** in the renderer (a row with a longer ID is drawn without its delete button and counted in `settingsScreen.Omitted`, which the bytes test holds at 0 for 17 digits), the same bound AGENTS.md states for Lift and Unlink ("which fits up to 17 digits"). The parser keeps `settingsCardNumber` (19 digits). Alternative if the owner insists on 19 digits: `r` in base36 (13 characters, Yes = 60 bytes) with a strict parser of its own. The Yes carries no page: after a delete the list redraws at the page where the removed entry's sort position now falls (S-06). |
| C-05 | D-03 rights | The three commands ask for three different things (S-10): the blacklist commands ask for the bot as an administrator twice, the user's restrict right and the bot's restrict right; `/filter` and `/stop` ask for change-info only; `/save` and `/clear` ask for the bot as an administrator plus change-info. No existing right covers the blacklist order. The door today judges the bot half only for `settingsRightAdminBotAdmin` (`if r.def.EditRight == settingsRightAdminBotAdmin && !r.botAdmin() {`, settings_input_door.go:269). | One new composite right `settingsRightRestrictBotRestrict` (appended after `settingsRightOwnerBotAdmin` so the zero value stays `settingsRightAdmin`); Filters use `settingsRightChangeInfo`, Notes `settingsRightChangeInfoBotAdmin` (both existing). Refactor the bot half into a function of `(b, group, right)` shared by the press (`settingsRightBotFirstRefusal`) and the door, with the door's live classification of a cached "false" (the `botAdmin()` pattern) generalised over a predicate on the bot's live record. |
| C-06 | D-04 "extended, not a second door" | The door is wired to the editor's page table: `settingsEditPageByKey(r.wait.Page)` in `run` (door.go:177), `r.def.EditRight` in `authorize` (265, 269), `def.CancelCode` in the refusal (418, 427), `r.def.Kind` in `apply` (442-445), `settingsEditLoad`/`renderSettingsEditor` in `saved` (509-515), `settingsInputPartWordKey` (141-149), and the wait's validity through `settingsInputValidPart` which only knows editor parts (settings_input.go:221-224), used by `settingsInputWaitFromFields` (240), `settingsInputSet` (294) and the marker parse (519). | Keep the editor path byte-identical and add one list branch: `settingsInputValidPart` becomes "an editor part or a list part"; `run` checks the list registry first and calls `runList`; `authorize` takes the right as a parameter; the refusal renderer takes the cancel code; the wait-ended note picks a list key ("press Add again"). Every Phase 11 door test must pass unchanged (name it in the plan's acceptance). |
| C-07 | D-01 and R-7 "three cells stay on Main" | `MainState func(b *gotgbot.Bot, tr *i18n.Translator, group *gotgbot.Chat) (state string, ok bool)` [VERIFIED: settings_pages.go:362] has no private flag, and Main draws every page's state in its body in both menus (`settingsMainEntries`, settings_pages.go:482). A count on Main would be shown in a group menu, which D-01 forbids. | Recommended: one optional field `PrivateOnly bool` on `settingsPage`; `settingsMainEntries` gains a `private bool` argument (3 call sites: settings.go:219, settings_menu.go:269, two tests) and for a `PrivateOnly` page in a group menu draws the fixed word `settings_state_private_only` **without calling `MainState`** (no read, no count); in a private menu `MainState` gives the count. Fallback with no framework edit: the fixed word in both menus. Record as A19-5. |
| C-08 | D-06 `settingsCardRows` | Its No is hard-wired to `map[string]string{"a": settingsActCancel, "p": noPage}` [VERIFIED: settings_card.go:77-78], which reopens page 1. In a long list a cancelled delete loses the owner's place. | Add a sibling helper in `settings_card.go` that takes the No fields (`settingsCardRowsTo(k, yes, no map[string]string)`), with No = the list's pager action carrying `r` (the entry's ID: "the page holding entry r"). Reusing `settingsCardRows` unchanged is acceptable if the planner wants zero helper edits (No goes to page 1). |
| C-09 | PAGE-08/09 "adds one", scope "editing an existing filter's or note's content" | `/filter` of an existing keyword does not refuse: it stores the whole new content in Redis behind a token (`setFilterOverwriteCache(token, overwriteFilter{... Text: text, FileID: fileid, Buttons: buttons ...})`, filters.go:146-156; key `fuku:%s_overwrite:%s`, TTL `5 * time.Minute`, overwrite.go:14, 45-47) and asks Yes/No; `/save` does the same (notes.go:111-177). Overwriting is editing content, which the CONTEXT puts out of scope. | The page **refuses an existing keyword or name at the first step** (the wait stays at step 1, so the owner can type another), and again at step 2 if another admin added it meanwhile (the wait then ends with the reason). The apply function returns an `exists` outcome; the command maps it to its unchanged overwrite flow. Record as A19-7. |
| C-10 | D-02 "the command maps the result to its unchanged reply" | Several replies are byte-sensitive quirks the goldens must pin and the page must not copy: user text spliced unescaped into HTML replies (`filters_added_success` with `<code>%s</code>`, filters.go:212-213; `filters_removed_success` with the original-case word, 268-269; `notes_save_success` three times, notes.go:192; `notes_removed_success`, 262-263; the already-blacklisted list, blacklists.go:128-131); `/blacklistaction` replies in Markdown (`formatting.Smarkdown()`, 331); `/save` and `/filter` without a reply collapse newlines and drop entities (measured: `/save rules line one\nline *two*` stores `"line one line <b>two</b>"`); with a reply and a two-word keyword the text becomes the keyword's second word (measured: `"/filter hello world"` replying to a text stores text `"world"`); `/save` sent as a photo caption panics (`slice bounds out of range [1:0]`, content.go:76 reads `msg.Text` only) and `/filter` hides the same panic behind its `recover` (filters.go:56-60) [VERIFIED: run]. | The goldens record each command exactly as it is (use words without `<`, `&` in the main cases; one case per command may pin the unescaped splice). The page escapes every user text, and its typed content uses the reply-form extraction without the two-word quirk (S-04). The caption panic is a pre-existing defect: listed under Open Questions, not fixed in this phase. |
| C-11 | PAGE-09 note names | `/save` accepts any name: no length cap, no character rule; without a reply the name is one word (`result.KeyWord = args[0]`, content.go:85), with a reply it may be several words joined by one space (93). A name with a space can never be fetched: `#name` takes the first space-separated token (`noteName := noteNameArgs[0]`, notes.go:688-690) and `/get` takes `args[0]` (812). `/clear` strips a leading `#` (`strings.TrimLeft(parts[1], "#")`, 230) while `/save #x` stores `#x`. | Recommended shared note-name rule (owner confirmation **A19-4**): refuse a name that contains whitespace, that starts with `#`, or that is over 100 runes (the filter keyword limit; it also bounds the `w` field of the wait). If the owner declines, the page accepts exactly what `/save`'s reply form accepts and the `w` bound becomes Telegram's own 4096-character text limit. |
| C-12 | D-11 strings | The prefix `settings_list_` is already taken by the private group list (`settings_list_intro`, `settings_list_hint`, `settings_list_empty`, locales/en.yml:3093-3095). | New keys use distinct prefixes: `settings_entries_` (shared list strings), `settings_bl_`, `settings_flt_`, `settings_notes_`; pager labels reuse `staff_panel_prev`/`staff_panel_next` (as Locks, the hub and the staff panel do: settings_locks.go:155-160). |
| C-13 | D-12 tests | Existing tests pin the page set and Main shape and must be updated, never loosened: `TestSettingsPhase18Codes` asserts `want := []string{"s", "l", "c", "f", "n", "k", "w", "g", "r", "m", "d", "t", "u", "o", "y", "a"}` (settings_phase18_test.go:166-167); `settingsMainShapeGroup`, `settingsMainShapePrivate` and `settingsMainOwnerRows = 10` (settings_main_test.go:36-38), also read by settings_deletion_test.go:119 and settings_deletion_screens_test.go:141. | Each plan that registers a page updates those three constants and the page set in the same commit (16, then 17, then 18 cells). |
| C-14 | Logs | The commands log user text: `"word": blWord` on add and remove failures (blacklists.go:116-120, 196-200), `[Notes] Failed to add note %s` (notes.go:181) and `Failed to remove note %s` (250). The door's rule is that no log line carries message text. | The extracted apply functions log chat IDs and the error only; this also changes the commands' log lines (logs are not part of the goldens). A logrus-hook test over every failing seam holds it (Validation Architecture). |
| C-15 | D-08 labels | The `none` action still deletes the message: the watcher deletes first (`_ = helpers.DeleteMessageWithErrorHandling(b, chat.Id, msg.MessageId)`, blacklists.go:503) and only then switches on the action, where `case "none": return ext.ContinueGroups` (586-587). Stored `tban`, `tmute` or `delete` (allowed by the CHECK, never written by the command) fall through the switch and behave the same (delete only). | Label the five choices for what they do: Warn, Mute, Kick, Ban, **Delete only** (value `none`). A stored `tban`/`tmute`/`delete` is drawn as stored with an inert line ("acts as Delete only"), marks no choice, and any choice replaces it (the Warnings page pattern of Phase 18). |
| C-16 | D-09 "no migration expected" | The migration chain adds `chk_blacklists_action CHECK (action IN ('warn', 'mute', 'ban', 'kick', 'tban', 'tmute', 'delete'))` (migrations/20250805204145_add_foreign_key_relations.sql:297-299, inside the applied `BEGIN; ... COMMIT;` block, lines 8 and 343) and never drops it; a later migration adds `chk_blacklist_action CHECK (action IN ('warn', 'mute', 'ban', 'kick', 'tban', 'tmute', 'delete', 'none')) NOT VALID` (migrations/20260412000000_add_missing_check_constraints.sql:67-70). A database built from the chain therefore has **both**, and `UPDATE blacklists SET action = 'none'` would violate the first. The GORM model only knows the second (models/blacklists.go:12), so SQLite tests cannot see this. | Live check (S-SPIKE 1) plus a PostgreSQL integrity test that sets each of the five command values on a chain-built database. If `none` fails, Phase 19 adds one append-only migration `ALTER TABLE blacklists DROP CONSTRAINT IF EXISTS chk_blacklists_action;` (both appliers), named in the Blacklist extraction plan; otherwise no migration. `/blacklistaction none` today replies `blacklists_set_bl_action_update_failed` on such a database (the write error is logged, blacklists.go:314-320). |

## Settled Findings

### S-01. Per-command facts (R-4; golden and apply-function material)

All commands of the domain are raw handlers built through `helpers.NewBotCommand` (no `WrapCommand`, no `Disableable` except the list views, no anonymous-admin re-entry). They work in a private chat connected to a group (`IsUserConnected`) and in a Staff Group like in any group (no Staff Group check anywhere in the three files) [VERIFIED: blacklists.go:593-614, filters.go:680-708, notes.go:932-964].

| Command (aliases) | Checks, in order | Reply keys | Write and cache keys dropped | Quirks |
|---|---|---|---|---|
| `/addblacklist` (`/blacklist`, `/addblocklist`, `/blocklist`) | `IsUserConnected(b, ctx, true, true)` (cached user admin and bot admin by `IsUserAdmin(b, chat.Id, b.Id)`, replying `connections_is_user_connected_bot_not_admin` / `_user_not_admin`); `RequireUser`; `IsUserAdmin` again (silent); `IsBotAdmin` (silent); `CanUserRestrict` → `chat_status_restrict_cmd_error`; `CanBotRestrict` → `chat_status_bot_restrict_group_error` [VERIFIED: blacklists.go:34-64; chat_status.go:396-509] | no args `blacklists_blacklist_give_bl_word`; over 100 runes `blacklists_blacklist_word_too_long` (separate reply, preview cut at 20 runes + `...`); then one reply of `blacklists_blacklist_already_blacklisted` list (unescaped) + `blacklists_blacklist_added_bl` list (escaped in `<code>`) + `common_settings_save_failed` when any write failed [VERIFIED: blacklists.go:66-145] | per word, lowercased: `blacklists.AddBlacklist(chat.Id, blWord)` = `db.CreateRecord` of `{Word: lower, Action: "warn", Reason: "Blacklisted word: '%s'"}`, then `cache.DeleteCache(cache.CacheKey("blacklist", chatId))` [VERIFIED: repository.go:14-30] | several words per call (`ctx.Args()[1:]`); duplicates checked against the cached `Triggers()` (stale up to `CACHE_LOCAL_TTL` on another replica; a duplicate that reaches PostgreSQL violates `uk_blacklists_chat_word` and is reported as a save failure); no context passed to the repository |
| `/rmblacklist` (`/rmblocklist`, `/unblacklist`) | same as add [VERIFIED: blacklists.go:153-180] | no args `blacklists_unblacklist_give_bl_word`; nothing removed `blacklists_unblacklist_no_removed_bl` (also when every delete failed); else `blacklists_unblacklist_removed_bl` with `', '`-joined words, no parse mode [VERIFIED: 182-220] | `RemoveBlacklist(chat.Id, word)` = `DELETE WHERE chat_id AND word`, drops `blacklist:<chat>` only when `RowsAffected > 0` [VERIFIED: repository.go:32-43] | a failed delete is logged with the word and skipped |
| `/blacklistaction` (`/blaction`) | `IsUserConnected(true, true)`; `RequireUser`; `CanUserRestrict` → `chat_status_restrict_cmd_error`; `CanBotRestrict` → `chat_status_bot_restrict_group_error` (no second `IsUserAdmin`, no `IsBotAdmin`) [VERIFIED: blacklists.go:284-305] | no args `blacklists_set_bl_action_current_mode` with `Action()`; one arg in `[]string{"mute", "kick", "warn", "ban", "none"}` (lowercased) → `blacklists_set_bl_action_changed_mode` or on error `blacklists_set_bl_action_update_failed`; anything else `blacklists_set_bl_action_choose_correct_option`; all replies `formatting.Smarkdown()` [VERIFIED: 307-331] | `SetBlacklistAction(chat.Id, action)` updates every row of the chat, drops `blacklist:<chat>` [VERIFIED: repository.go:56-65] | see C-02 (empty list, per-row action, first-row status), C-16 (`none` on PostgreSQL) |
| `/blacklists` (`/blocklists`), read only, Disableable | `CheckDisabledCmd`; `IsUserConnected(false, true)` [VERIFIED: 228-235] | `blacklists_ls_bl_list_bl` + sorted escaped list, or `blacklists_ls_bl_no_blacklisted` | none | sorts with `slices.Sort` (byte order) |
| `/filter` (`/addfilter`) | `recover`; `IsUserConnected(true, false)` (no bot check); `RequireUser`; `CanUserChangeInfo` → `chat_status_change_info_cmd_error`; **cap first**: `CountFilters` and `if filtersNum >= 150 {` → `filters_limit_exceeded`; then args: reply with ≤1 arg → `filters_keyword_required`, no reply with ≤2 args → `filters_invalid` [VERIFIED: filters.go:55-110] | extraction error → its `ErrorMsg` (`content_need_filter_content`, `content_text_too_long`, `content_caption_too_long`); keyword over 100 runes → `filters_keyword_too_long`; existing keyword → overwrite card `filters_overwrite_confirm` (or `filters_overwrite_token_failed` without Redis); write error → `common_settings_save_failed`; success `filters_added_success` with the keyword unescaped [VERIFIED: 112-219] | `AddFilter(chat, kw, text, fileid, buttons, type)` = insert with `ON CONFLICT (chat_id, keyword) DO NOTHING`, drops `filter_list:<chat>` and `filters_optimized:<chat>` only when a row was inserted [VERIFIED: filters/repository.go:69-96, 25-28] | `CountFilters` and `DoesFilterExists` hide their errors (return 0 and false) [VERIFIED: repository.go:142-148, 56-67]; a lost insert race replies success; keyword lowercased after extraction (123) |
| `/stop` (`/rmfilter`, `/removefilter`) | `IsUserConnected(true, false)`; `RequireUser`; `CanUserChangeInfo` [VERIFIED: 223-239] | no args `filters_remove_keyword_required`; not in the cached list → `filters_not_exists`; error → `common_settings_save_failed`; success `filters_removed_success` with the original-case word unescaped [VERIFIED: 241-275] | `RemoveFilter(chat, lower(kw))`, drops both filter keys when a row went [VERIFIED: repository.go:118-129] | keyword from `extraction.ExtractQuotes` of the joined args |
| `/save` (`/addnote`) | `IsUserConnected(true, true)`; `RequireUser`; `CanUserChangeInfo`; args: reply with ≤1 → `notes_keyword_required`, no reply with ≤2 → `notes_invalid` [VERIFIED: notes.go:50-90] | extraction error → its `ErrorMsg` (`content_need_note_content`, length texts); `{private}` and `{noprivate}` both → both cleared and `notes_private_conflict_warning` appended; existing name → overwrite card `notes_overwrite_confirm`; error → `notes_save_failed`; success `notes_save_success` (name three times, unescaped) [VERIFIED: 92-196] | `AddNote(...)` insert with `ON CONFLICT (chat_id, note_name) DO NOTHING`, drops `notes_list:<chat>` only when inserted [VERIFIED: notes/repository.go:131-162] | no count cap; no name rule (C-11); `DoesNoteExists` hides errors (118-129) |
| `/clear` (`/rmnote`) | `IsUserConnected(true, true)`; `RequireUser`; arg check; **then** `CanUserChangeInfo` [VERIFIED: notes.go:201-235] | `notes_remove_keyword_required`; not in the cached list → `notes_not_exists`; error → `error_generic`; success `notes_removed_success` (unescaped) [VERIFIED: 215-267] | `RemoveNote(chat, lower(name))`, drops `notes_list:<chat>` when a row went [VERIFIED: repository.go:190-200] | name = text after the first space with leading `#` stripped, then `ExtractQuotes` (after the existence check) |
| `#name` (group 0 message handler) and `/get` (Disableable) | `#`: no right; `/get`: `CheckDisabledCmd`, `IsUserConnected(false, false)` [VERIFIED: notes.go:671-697, 775-823] | not found → `/get` replies `notes_does_not_exist`, `#` passes on; admin-only to a member → `notes_admin_only`/`notes_admin_only_access`; private notes → `notes_click_for_private` with a deep link | reads `notes_list:<chat>` then a fresh `notes.GetNote` | name lowercased; first token only |

Out of scope commands (bulk removal, `/privnote`) are not touched; their callbacks keep their own namespaces (`rmAllBlacklist`, `rmAllFilters`, `rmAllNotes`, `filters_overwrite`, `notes.overwrite`).

Golden material, one file per module, committed alone before its extraction (Phase 18 pattern, `logchannels_legacy_request_test.go` as the model: fixed chat IDs from a reserved block, `withRealLocales(t)` so texts are real, a GORM callback to make a write fail [VERIFIED: fuku/modules/logchannels_legacy_request_test.go:1-60; warns_legacy_request_test.go:124]). None of these paths reads `config.AppConfig` (no hit in `fuku/db/lang`, `fuku/utils/content`, `fuku/utils/media`, `fuku/utils/chat_status`, `fuku/i18n`) [VERIFIED: run, grep], so the 85ca132 trap does not apply; the goldens still must pass bare and with `cienv.sh`. Cases: each check's refusal (bot not admin, user not admin, no restrict / no change-info, bot cannot restrict), no args, too long, already present, success (one and several words; one alias per handler), failed write, the overwrite card (Redis present and absent), the cap at 150, empty-list action, action current mode, each action value, and for notes the private-flag conflict. Exclude: single-code-point words and keywords (C-03), note names with spaces or `#` or over 100 runes (C-11), "chat had a non-warn action before adding" (C-02 A19-3).

### S-02. Matchers and validation (R-2)

What each matcher does [VERIFIED: fuku/utils/keyword_matcher/matcher.go:39-84, cache.go:26-59; run]:

- Blacklist words and filter keywords: one Aho-Corasick matcher per chat (`keyword_matcher.GetNamedCache("blacklists")` / `("filters")`), patterns lowercased with `strings.ToLower`, the match text lowercased, literal substring search, first hit wins; the per-chat matcher is rebuilt when the pattern list differs (`slices.Equal(matcher.patterns, patterns)`). The match text is the message text and caption, trimmed and deduplicated, plus URLs from entities (`buildModerationMatchText`, moderation_input.go:12-42). No word boundary, no regex, no wildcard, no Unicode case folding beyond `ToLower` (`ß` does not match `STRASSE`; `İ` lowercases to `i`).
- Note names: exact equality of the lowercased first token after `#` (`#` watcher) or of `args[0]` (`/get`) against the cached name list [VERIFIED: notes.go:688-693, 812-814].

Probe of the real matcher [VERIFIED: run]:

```
pats=[""] text="hello" -> "" false          pats=["a"] text="what" -> "a" true
pats=["a"] text="CAT" -> "a" true           pats=["*"] text="hello" -> "" false
pats=[".*"] text="hello" -> "" false        pats=["hell"] text="shell" -> "hell" true
pats=["hello world"] text="hello  world" -> "" false
pats=["привіт"] text="ПРИВІТ всім" -> "привіт" true   pats=["🙂"] text="ok 🙂" -> "🙂" true
pats=["ß"] text="STRASSE" -> "" false       pats=["İ"] text="istanbul" -> "İ" true
```

| Input | Blacklist today | Filter keyword today | Note name today | Proposed shared rule |
|---|---|---|---|---|
| empty / blank | impossible (Fields) | impossible from the command | impossible | refuse (page only can produce it): give-word / keyword-required key |
| one letter, digit, punctuation, space | accepted; matches most messages | accepted | accepted (harmless: exact match) | refuse for blacklist and filter (C-03); notes unaffected |
| one emoji or other symbol (So) | accepted | accepted | accepted | keep accepting (a legitimate target) |
| `*`, `.*`, regex-looking | accepted, literal | accepted, literal | accepted | keep (literal, matches only itself) |
| whitespace inside | impossible (split into words) | allowed with a reply (joined by one space) | allowed with a reply, unreachable | blacklist: the typed text is split like `ctx.Args()`; filter: `strings.Fields` joined by one space; note: refuse (C-11, A19-4) |
| over 100 runes | refused (`blacklists_blacklist_word_too_long`) | refused (`filters_keyword_too_long`) | accepted | keep; note names also capped at 100 (A19-4) |
| duplicate / case variant | lowercased; "already blacklisted" | lowercased; overwrite card | lowercased; overwrite card | page: "already on the list" refusal (C-09) |

Owner-visible change implied (A19-2): `/addblacklist a` and `/filter a <reply>` are refused with a new reply naming why ("would act on almost every message"); multi-word `/addblacklist a spam` adds `spam` and lists `a` as refused, exactly like the too-long words today.

### S-03. The two-step state (R-1)

The wait today: one hash per user `fuku:settings:input:<user>` with fields `c` (group), `p` (page), `t` (part), `m` (prompt message ID), `n` (nonce), `x` (expiry ms), `k` (claim) and `r<id>` (refusal replies), its own 10-minute `PEXPIRE`, a marker `fuku:settings:inputdone:<user>` of `c|p|t` (15 minutes), and the header comment "the hash holds IDs and codes only, never message text or a file ID" [VERIFIED: settings_input.go:24-52, 95-181]. A Set deletes the hash before writing (`redis.call('DEL', KEYS[1])`, line 97), `/cancel` and the Cancel button delete it (109-140), finish deletes it (169-172).

| Criterion | A. field `w` in the wait hash | B. short token to a separate key | C. keyword carried in the step-2 prompt, user replies to it |
|---|---|---|---|
| Atomic with the wait (Set replaces, `/cancel`, Cancel, finish, TTL) | yes: every existing script already deletes the whole hash; nothing to add | no: every script must also delete the side key (Set needs the old token from the old hash, Cancel and `/cancel` too), and the side key needs its own TTL kept in step | n/a (stateless) |
| Album gate | unchanged (`fuku:settings:inputalbum:*`) | unchanged | unchanged |
| A second Set replacing the first | the old `w` vanishes with the hash (line 97) | the old side key leaks until its TTL unless the Set script learns to delete it | an old prompt can still be replied to and saved (needs a nonce check anyway) |
| Restart / replica | in Redis, any replica | in Redis, any replica | in Telegram |
| No Redis | Add answers `settings_edit_unavailable` (the door already fails closed) | same | works without Redis, but the rest of the door needs Redis anyway |
| What travels in callback data | nothing (Pitfall 28 holds: no user text in callback data) | the token would not need to be in callback data either, so the token buys nothing | nothing |
| What is logged | nothing new (the door logs user, chat, page, part) | same | must parse the bot's own message text (localized, escaped): fragile |
| Failure modes | one extra field to validate on read | two keys to keep consistent; orphaned keys | the owner must use Telegram's reply; a plain message would be read as a new keyword; a forged reply-to must be checked to be the bot's own message |

**Recommendation: A.** Add `w` to the wait hash, present only on a second-step part (`y` filter reply, `c` note content), at most 100 runes after the shared validation (C-03, C-11; if A19-4 is declined, at most 4096 runes), never logged and never put in callback data. This amends the header rule of settings_input.go and the AGENTS.md "Data" bullet for one bounded field (record in AGENTS.md and as A19-6). Precedent: the commands already keep the whole filter or note content (text, file ID, buttons) in Redis for 5 minutes behind a token (overwrite.go:14-55).

Transition from step 1 to step 2 is one new Lua script `settingsInputAdvanceScript` (KEYS: wait, marker; ARGV: old nonce, new nonce, new part, `w`, new expiry ms, TTL ms, new marker value, marker TTL ms): it returns 0 unless `HGET n == old nonce` and no `k`; otherwise it `HSET`s `t`, `w`, `n`, `x`, drops the `r*` fields of step 1 (optional hygiene; their Cancel buttons carry the step-1 part and are refused as stale by the existing cancel script anyway), refreshes `PEXPIRE` (a fresh ten minutes for the content) and rewrites the marker. A second delivery of the same keyword message, a Cancel or `/cancel` racing it, or a Set that replaced the wait all see a stale nonce and do nothing. After the advance the door sends the step-2 prompt as a reply to the keyword (escaped keyword, the content rules, one Cancel button with `t` = the step-2 part) and records that message's ID as `m` through a small holder-of-nonce script (or as `r<id>` with the existing `settingsInputNoteRefusal`), so its Cancel ends the wait and the final save closes it. `settingsInputWaitFromFields` reads `w` and refuses a wait whose step-2 part has no valid `w` (and ignores `w` on other parts), so a malformed hash is "no wait" as today.

### S-04. What a typed reply may be (R-3)

`content.ExtractNoteAndFilter(msg, isFilter, language)` is the commands' extraction [VERIFIED: fuku/utils/content/content.go:49-116]:

- Inline form (`/filter kw text...`, no reply): keyword `args[0]`, text `strings.Join(args[1:], " ")` from `msg.Text` (entities lost, newlines collapsed), `MD2HTMLButtonsV2` on it, type `db.TEXT` (83-91).
- Reply form (`/filter kw` replying to M): keyword `strings.Join(args, " ")` (93); raw text from M (`OriginalCaptionMDV2` for a caption, else `OriginalMDV2`, so formatting and newlines survive) except the quirk that with two or more args and no caption on M the text comes from the command itself (297-301); buttons from the markdown, or from M's inline keyboard when it has one (95-100); type `db.TEXT` when M has text, else `extractMediaFromReply(M)` (102-106).
- Media kinds (`extractMediaFromReply`, 118-140): sticker, document, photo (largest size), audio, voice, video, animation (stored as `db.DOCUMENT`, Document checked first), video note; anything else (poll, location, contact, venue, dice, story) gives -1 and the "give it some content" reply. Types: `TEXT 1, STICKER 2, DOCUMENT 3, PHOTO 4, AUDIO 5, VOICE 6, VIDEO 7, VIDEO_NOTE 8` (`ANIMATION 9` is greetings only) [VERIFIED: fuku/db/db.go:33-44].
- `preFixes` (249-278): `TextLengthRefusal` (text over 4096 code points, caption over 1024, of the stored HTML; 204-219), default button name = the keyword, buttons with a non-http(s) link dropped silently (`ValidButtonURL`), text trimmed, no text and no file → -1. **No button count limit** (the editor's 100-button, 8-per-row check is Phase 11's, not the commands').
- Notes only: `NotesParser` sets the six flags from `{private}`, `{noprivate}`, `{admin}`, `{preview}`, `{protect}`, `{nonotif}` and leaves the directives in the stored text (111-113).

Measured [VERIFIED: run]:

```
inline multi-line        kw="rules" text="line one line <b>two</b>" type=1
reply 1-word kw          kw="hello" text="line one\nline two" type=1
reply 2-word kw (quirk)  kw="hello world" text="world" type=1
reply photo+caption      kw="pic" text="cap" type=4 file="big"
reply gif                kw="gif" text="" type=3 file="doc"
reply poll               kw="p" text="" type=-1
caption command          PANIC runtime error: slice bounds out of range [1:0]
buttons                  kw="b" text="hi" type=1 btns=1   (the javascript: link dropped)
note directives          kw="n" text="hello {admin} {private}" type=1
```

**Recommendation.** The page's second message is the content message M itself, so the stored row must equal what `/filter <kw>` (or `/save <name>`) **replying to M** stores. Refactor `content.go` without changing `ExtractNoteAndFilter`'s output: move the reply-branch body into `extractReplyContent(replyMsg, rawText, keyword, isFilter, language) ExtractResult` (called by the reply branch with the raw text `setRawText` computed, quirk preserved) and export `ExtractTypedNoteOrFilter(content *gotgbot.Message, keyword string, isFilter bool, language string) ExtractResult`, which uses the raw text the reply form uses for one argument (`OriginalCaptionMDV2` when the message has only a caption, else `OriginalMDV2`). A pure parity test asserts, for a table of messages M (text with entities and newlines, each media kind with and without caption, markdown buttons, a forwarded inline keyboard, an over-long text and caption, a poll) and single-word keywords K, that `ExtractTypedNoteOrFilter(M, K)` equals `ExtractNoteAndFilter(&Message{Text: "/filter " + K, ReplyToMessage: M})`. For a multi-word keyword the page stores what the reply form would store without the quirk (documented difference). The commands' replies for extraction errors (`content_need_*`, `content_*_too_long`) are the page's refusal texts (they are already built in the presser's language when `language` is the presser's).

The step-2 message goes through the door's existing candidate filter, so a content text that starts with `/` followed by a non-space is never taken (`settingsInputLooksLikeCommand`, door.go:44-56): a filter reply `/start` cannot be added from the page (the command can). Album: the first item is the content and the saved notice adds `settings_edit_album_cut` (existing). A rich message is never a candidate (door.go:61-65).

### S-05. Reads and freshness (R-5)

| List | Command key | TTL | In `skipLocal`? | Carries row ID? | Read error |
|---|---|---|---|---|---|
| blacklist | `blacklist:<chat>` (`BlacklistSettingsSlice`) | `CacheTTLBlacklist = DefaultCacheTTL` (30 min) | no | yes | hidden (empty slice) |
| filter keywords | `filter_list:<chat>` (`[]string`) | `CacheTTLFilterList` (30 min) | no | no | hidden (`[]string{}`) |
| filter bodies | `filters_optimized:<chat>` (`[]*ChatFilters`) | `15*time.Minute` | no | yes | **returned** (`GetChatFiltersCachedContext`) |
| notes | `notes_list:<chat>` (`[]cachedNoteInfo{Name, AdminOnly}`) | `CacheTTLNotesList` (30 min) | no | no | hidden (`nil`) |

[VERIFIED: fuku/db/cache/ttl.go:6-16; fuku/db/cache/local.go:58-84 (`skipLocal` lists captcha pending, staff group, staff link, raid detection, member requirements, delete rules only); filters/optimized.go:205-216; the repository lines in C-01]

Decisions: (1) the pages read fresh (C-01): `SELECT id, word, action FROM blacklists WHERE chat_id = ?`, `SELECT id, keyword, msgtype FROM filters WHERE chat_id = ?`, `SELECT id, note_name, admin_only, msg_type FROM notes WHERE chat_id = ?`, each through `db.DB.WithContext(ctx)`, returning the error, never creating a row; (2) sort in Go by the lowercased text in code point order, ties by ID (the hub's `settingsHubSort` rule), page at 8; (3) every read and every delete carries `chat_id = ?`, and the card's entry lookup is `WHERE id = ? AND chat_id = ?`, so a forged ID of another chat shows and deletes nothing. Cost: one indexed query per draw (the unique constraints `uk_*` lead with `chat_id`, and `idx_filters_chat_optimized ON filters(chat_id)` exists: migrations/20250806100000_critical_performance_indexes.sql:80-81). Filters are capped at 150; notes and blacklist words are uncapped, so a group with 10,000 words costs one query returning 10,000 short rows (a few hundred KB) per page draw [ASSUMED: estimate], acceptable for an owner action; if that ever matters the fallback is `ORDER BY id LIMIT 8 OFFSET n` plus a count. No migration and no index is needed.

Never call `notes.GetNotes` from a page: it creates a `notes_settings` row on read (`db.CreateRecordContext`, notes/repository.go:32-36). The privnote state is out of scope, so the Notes page does not read it.

Cross-replica freshness of the effect (R-9): none of the four command keys is in `skipLocal`, so another replica can serve the old list from its in-process layer for up to `CACHE_LOCAL_TTL` (10 s by default) after a page or a command write, exactly as today. Adding them to `skipLocal` would put a Redis round trip on every group text message for the blacklist and filter watchers (they read their key on every message) [ASSUMED: cost reasoning]. Do not add them.

### S-06. Callback design, paging, bytes, stale entries (R-6)

Recommended codes (page keys free today: `b e h i j p q v x z`; used: `s l c f n k w g r m d t u o y a` [VERIFIED: settings_*.go registrations, settings_phase18_test.go:167]; no clash with the existing action codes `ay bk no cl dl dr dy ls ly kg mo my op oh pk st tg lk ua uy pg ws wr wc wv wf gs gr gc gv gf rs rr rc rv rf` [VERIFIED: run, grep of settings_*.go]):

| Page (key) | Pager | Delete ask | Delete Yes | Add | Cancel the wait | Action |
|---|---|---|---|---|---|---|
| Blacklist (`b`) | `bg` (`v` page, or `r` entry) | `bx` (`r`) | `by` (`e`, `r`) | `ba` | `bc` (`t` = `w`) | control `a`, `st` only, `v` ∈ `warn mute kick ban none`; optional screen `bo` |
| Filters (`i`) | `ig` | `ix` | `iy` | `ia` | `ic` (`t` ∈ `k y`) | — |
| Notes (`e`) | `eg` | `ex` | `ey` | `ea` | `ec` (`t` ∈ `n c`) | — |

Every list action is valid only in a private menu: its `Validate` requires the `c` field (it receives the whole field map in step 1 [VERIFIED: settings_menu.go:316-320]), so a forged list press on a group menu is `common_callback_invalid_request` before any I/O and can never draw an entry.

Worst cases with the real codec, 14-character chat ID, 10-digit user ID [VERIFIED: run]:

```
open page b (private)    37 settings|v1|a=op&c=-1001234567890&p=b
open page b (group)      33 settings|v1|a=op&p=b&u=1234567890
pager v=99999            41 settings|v1|a=bg&c=-1001234567890&v=99999
pager to entry r17       53 settings|v1|a=bg&c=-1001234567890&r=12345678901234567
pager to entry r19       55 settings|v1|a=bg&c=-1001234567890&r=9223372036854775807
delete ask r17           53 settings|v1|a=bx&c=-1001234567890&r=12345678901234567
delete ask r19           55 settings|v1|a=bx&c=-1001234567890&r=9223372036854775807
delete ask r19 + v5      63 settings|v1|a=bx&c=-1001234567890&r=9223372036854775807&v=99999
delete Yes r17           64 settings|v1|a=by&c=-1001234567890&e=29412345&r=12345678901234567
delete Yes r19           OVERFLOW callback data exceeds max length: 66 > 64
delete Yes r9 + v5       64 settings|v1|a=by&c=-1001234567890&e=29412345&r=123456789&v=99999
card No core (p=b)       37 settings|v1|a=no&c=-1001234567890&p=b
add                      33 settings|v1|a=ba&c=-1001234567890
cancel wait t=w          37 settings|v1|a=bc&c=-1001234567890&t=w
action st none           48 settings|v1|a=st&c=-1001234567890&p=b&t=a&v=none
action screen open       33 settings|v1|a=bo&c=-1001234567890
back                     33 settings|v1|a=bk&c=-1001234567890
close                    33 settings|v1|a=cl&c=-1001234567890
unix minute now: 29909076   (8 digits until about the year 2160)
```

Cursor: the page number `v` (1 to 5 digits, clamped to the last page; parse with `settingsParseNumber(v, 5)`), or `r` (exactly one of `v` and `r`), "the page that holds entry r", page 1 when r is gone or belongs to another chat. A fixed page size of 8 and a per-entry display cap make an offset cursor unnecessary (the staff history needs an offset because its page shrinks to the 3800-unit cap; these pages never shrink). After a delete the Yes redraws the page where the removed entry's sort position now falls (computed from the `RETURNING` text), so the owner stays in place without `v` in the Yes.

Stale entries: the ask reads the entry fresh by `(id, chat_id)`; gone → redraw the page with `settings_entries_already_removed` and no card. The Yes deletes by `(id, chat_id)` with `RETURNING`; zero rows → the same notice (two admins deleting the same entry, a stale card, a forged ID all get one answer). An expired Yes → `settings_card_expired`, nothing deleted (existing `settingsCardExpired`).

Layout recommendation (UI-SPEC to confirm): body lines `N. <code>escaped text</code>` with the text stripped of bidi controls (`memberReqStripBidi`), cut at 32 code points plus `…` (the hub's `settingsHubLabelMax = 32`, `settingsHubEllipsis = "…"`, settings_hub.go:36, 42) and HTML-escaped after the cut; keyboard: numbered delete buttons `🗑 1` to `🗑 8` in two rows of four (no user text in any label), the pager row, `[➕ Add]` (or `[➕ Add | ⚙️ Action]` on Blacklist), then Back and Close: 5 rows (Filters, Notes) or 5 to 7 rows (Blacklist, depending on where the action sits), inside the 8-row private budget. Worst-case text: 8 entries of 32 `&` characters (5 UTF-16 units each when escaped) ≈ 8 × 180 = 1,440 units, plus header, group line (64-rune title escaped), intro, page line and a notice: under 2,500 of 3,800 [ASSUMED: estimate; the fit test computes it].

### S-07. Main and the group menu (R-7)

Main today: 15 grid cells, owner rows 10 [VERIFIED: settings_main_test.go:36-38: `"s|l / c|f / n|k / w|g / r|m / d|t / u|o / y / a / close"`, `settingsMainOwnerRows = 10`]. The Phase 18 UI-SPEC already sized Phase 19: "Phase 19 adds three more cells (18 cells, 9 grid rows, 11 / 10 rows), which is the reason the fallback exists" and kept the flat grid with a specified "More" hub as fallback (A18-A1, unconfirmed) [CITED: 18-UI-SPEC.md M34, A18-A1]; AGENTS.md: "The second hub the UI-SPEC kept as a fallback (A18-A1) is not built, because nothing forced it (D-23)" [VERIFIED: AGENTS.md:608].

**Recommendation: keep the flat grid.** Blacklist `Order` 46 (beside Locks 44, before Welcome 50), Filters 72 and Notes 74 (after Rules 70, before Member requirements 80). Main becomes `s|l / c|f / n|k / b|w / g|r / i|e / m|d / t|u / o|y / a / nav`: 9 full grid rows (the Anonymous admins cell is no longer alone), Access, navigation = 11 rows for the owner, 10 for an administrator, 21 buttons in a private menu (Groups + Close), well under Telegram's 100. Over the content-page budget by 3 (private, owner) as A18-A1 predicted; record as A19-8 together with the hub fallback. Icons (UI-SPEC to confirm): ⛔ Blacklist, 💬 Filters, 📝 Notes (each "icon name" cell within 20 code points in 8 languages is a label-length test row).

Main state line: with C-07's hook, a private menu shows `{n} words` / `{n} of 150 filters` / `{n} notes` (fresh count, failed read → unknown mark); a group menu shows the fixed word `settings_state_private_only` and reads nothing.

Group-menu page (D-01): the header, the group line, one line `settings_entries_group_note` ("This list is shown only in my private chat. Tap Open in private below."), the Open in private URL row (the existing deep link, left out when the bot has no username, `settingsEditOpenPrivateRow` pattern, settings_edit.go:517-529), Back and Close. No entry, count, action word or Add. Three rows.

### S-08. The door and the other waits (R-8)

- Add while a wait is open for anything else (a Welcome part, another list, another group): `settingsInputSet` returns the replaced wait and the prompt adds `settings_edit_replaced_note` (existing, `settingsEditWaitReplaced`, settings_edit_actions.go:115-119).
- Add again while in step 2 of the same list: the Set replaces the wait (new nonce, step 1, `w` gone): the replaced note applies because the part differs.
- Reply after ten minutes: the marker (`c|p|t` of the current step) gives one plain "wait ended" reply; it needs a list wording (new `settings_entries_wait_ended_note` "... Press Add on the page again.") and part words for `w`, `k`, `y`, `n`, `c`.
- A refused first step (blank, too broad, too long, exists, at the 150 cap, a media message instead of text) is a refusal reply with Cancel, the wait kept at step 1 and not extended (the Phase 11 refusal: door.go:413-434), so the owner can type another keyword; the refusal's Cancel is noted on the wait (`settingsInputNoteRefusal`).
- A refused second step (unsupported media, too long, no content) keeps the wait at step 2 (`w` kept). A keyword that became taken, or a cap reached, between the steps ends the wait with the reason and `settings_edit_wait_stopped`.
- Blacklist (one step): the typed text is split exactly like `ctx.Args()` (`strings.Fields` of the text or caption); pure pre-check against a fresh list; if nothing is addable (all blank, too long, too broad or already present) it is a refusal (wait kept); otherwise claim, `applyBlacklistAdd`, finish, send the list page as a new message with a notice of counts plus short refused previews (at most 5 too-long previews of 20 runes and 10 too-broad characters, escaped), and close the prompt. A failed write releases the claim and answers `common_settings_save_failed` (Phase 11 behaviour).
- Each typed message is judged like a press: live `getChatMember`, owner gate, the part's right (member part and bot part, C-05), live `getChat`, then the check, the claim or the advance, and the write [VERIFIED: door.go:176-292].
- `/cancel` deletes the whole hash, `w` included (settings_input.go:109-116). No Redis: Add answers the existing `settings_edit_unavailable` alert (its text says "The editor"; a list wording `settings_entries_add_unavailable` is better), while the list, the delete card and the action keep working (fresh database reads, stateless card).
- The door runs at handler group 1, after the group-0 commands; a step text that looks like a command passes on unanswered and the wait stays (Phase 11 accepted behaviour). A `#rules` typed in private reaches the `#` notes watcher at group 0 first, which finds no note of the private chat and passes on (notes.go:693-695), so it is safe as a typed name.

### S-09. The end-to-end proof (R-9)

"Works in the group at once" means, on the replica that took the press, at the very next group message:

- Blacklist: the page add calls `AddBlacklist`, which drops `blacklist:<chat>`; the group-7 watcher (`handlers.NewMessage(func(msg) bool { return msg.Text != "" || msg.Caption != "" }, ...)`, blacklists.go:611-613) reloads the triggers, the per-chat matcher is rebuilt because the pattern list changed, and a member's (not admin, not approved) message containing the word is deleted and the row's action applied (for a new word `warn` unless A19-3 lands), with the bot an administrator (`IsBotAdmin`, 493).
- Filters: the page add calls `AddFilter`, which drops `filter_list` and `filters_optimized`; the group-9 watcher matches and sends the reply through `media.SendFilter` (filters.go:594-675).
- Notes: the page add calls `AddNote`, which drops `notes_list`; `#name` (group 0, `strings.HasPrefix(msg.Text, "#")`, notes.go:953-960) and `/get name` send the note through `media.SendNote` (in a group with private notes off).
- After a page delete: the word is no longer acted on, the filter no longer answers, `/get name` answers `notes_does_not_exist`.

On another replica the same happens within `CACHE_LOCAL_TTL` (S-05). No cache other than the command keys is involved; the matcher cache is per process and rebuilds by comparison.

### S-10. Rights per control (D-03, C-05)

| Control | Right | Refusals (key, kind) |
|---|---|---|
| Open a page, pager, action screen, Cancel the wait | `settingsRightAdmin` | — (the live admin check every press makes; looser than `/blacklists` and `/filters`, which need the bot as an administrator: viewing writes nothing, the Phase 18 accepted looseness for `/anonadmin`'s status) |
| Blacklist add (press and typed reply), delete ask and Yes, action | new `settingsRightRestrictBotRestrict` | bot half first: `!IsUserAdmin(b, group, b.Id)` → `connections_is_user_connected_bot_not_admin` (notice); `!IsBotAdmin` → `chat_status_bot_not_admin` (notice; the command is silent here); member half: creator or administrator with `can_restrict_members` (`staffIssuerSkipReason`) else `settings_no_restrict` (alert); then `!CanBotRestrict` → `chat_status_bot_restrict_error` (notice, the button key the command's responder picks for a callback) |
| Filter add, delete ask and Yes | `settingsRightChangeInfo` | `settings_no_change_info` (alert) |
| Note add, delete ask and Yes | `settingsRightChangeInfoBotAdmin` | `connections_is_user_connected_bot_not_admin` (notice), then `settings_no_change_info` (alert) |

The delete ask declares the delete's right, so a presser without it never sees the card (the Member requirements card precedent); the Yes judges it again on a fresh member record. The owner gate's carve-out applies (a non-owner administrator with the switch off is refused at step 4b, stricter than the commands). `/blacklistaction` has no `IsBotAdmin` step; using the same composite for the action is stricter only when the two cached bot predicates disagree [ASSUMED: corner case], documented in the authority-parity test.

### S-11. Blacklist action semantics (D-08, C-02, C-15)

The page offers exactly the command's five values; the CHECK's other three (`tban`, `tmute`, `delete`) are drawn as stored with an inert line. Every choice goes through `applyBlacklistAction(ctx, chatID, action) blacklistActionResult{Outcome, ReplyKey, Rows int64, Err}`; outcomes `applied`, `noWords` (0 rows), `unknown` (not one of the five), `saveFailed`. A press of the action every row already has writes the same rows again and shows no notice.

### S-12. Staff Group (D-10)

The commands carry no Staff Group check and the StaffActions interceptors catch only `/ban`, `/mute` and their kin, so the three lists work in a Staff Group as in any group [VERIFIED: grep, no `staff` reference in blacklists.go, filters.go, notes.go]. The pages are not `Hidden`; no list screen draws anything about a Staff Group (they draw the group's own title and ID like every page; `TestSettingsStaffPrivacy` renders them in 8 languages).

### S-13. Framework edits the phase needs (named so the plans can carry them)

1. `settings_menu.go`: the composite right, its member part in `settingsRightRefusalFor`, its bot half shared with the door.
2. `settings_pages.go`: the optional `PrivateOnly` field and the `private` argument of `settingsMainEntries` (C-07; skip if the fallback is chosen).
3. `settings_card.go`: `settingsCardRowsTo` (C-08; optional).
4. `settings_input.go`: list parts in the validity check, the `w` field, the advance script, a prompt-ID script; `settings_input_door.go`: the list branch, `authorize(right)`, the generalised bot classification, the list refusal renderer, the list wait-ended note (C-06).
5. `fuku/utils/content/content.go`: `extractReplyContent` and `ExtractTypedNoteOrFilter` (S-04), output of `ExtractNoteAndFilter` unchanged.
6. Repositories: fresh list readers, entry reader by `(id, chat)`, delete by `(id, chat)` with `RETURNING` and the commands' cache drops, error-returning count and existence (filters, notes), `SetBlacklistActionContext` returning rows.

## Architecture Patterns

### System Architecture Diagram

```
owner taps a list page (private menu)                 owner types in the bot's private chat
        |                                                         |
        v                                                         v
settings| callback --> press pipeline (decode, service id,   group 1 door: candidate? wait? album gate
  opener, live getChatMember, owner gate, right)                  |
        |                                                         v
        +-- op / pager --> fresh (id,text) read, chat-scoped --> sort, page 8 --> render (escaped, capped,
        |                                                         numbered 🗑 buttons) --> edit message
        +-- delete ask --> read (id, chat) --> gone? "already removed" : two-tap card (r, e)
        +-- delete Yes --> expiry --> DELETE ... WHERE id AND chat RETURNING --> drop command keys --> redraw
        +-- action    --> applyBlacklistAction --> UPDATE rows --> drop blacklist:<chat> --> redraw
        +-- add       --> settingsInputSet (wait c,p,t=step1,m,n,x) --> prompt with Cancel
                                                                  |
                         live authorize (member, gate, right, getChat)
                                                                  |
              step 1 (keyword/name): normalise + shared validation + exists/cap (fresh)
                 refused -> refusal reply, wait kept         ok -> advance script (t=step2, w, new n, x)
                                                                  -> step-2 prompt (records m)
              step 2 (content): ExtractTypedNoteOrFilter -> refused: wait kept
                 ok -> claim -> applyFilterAdd/applyNoteAdd -> AddFilter/AddNote -> drop keys -> finish
                 blacklist (one step): split words -> pre-check -> claim -> applyBlacklistAdd -> finish
                                                                  |
                                                    saved list page sent as a new message
group message --> group 7 blacklist / group 9 filters / group 0 #note,/get --> read dropped keys --> act
```

### Recommended project structure (additions only)

```
fuku/modules/
├── settings_list.go            # the list engine: page table type, fresh read, sort, page, render, card, delete
├── settings_list_input.go      # list parts of the wait, step-1 checks, advance, door branch helpers
├── settings_blacklist.go       # page b table + action control (registers from init)
├── settings_filters.go         # page i table
├── settings_notes.go           # page e table
├── blacklists.go / filters.go / notes.go   # + apply functions and shared validation beside each command
fuku/db/{blacklists,filters,notes}/repository.go  # + ListEntriesFresh, EntryFresh, RemoveByID, ... (S-13)
fuku/utils/content/content.go   # + extractReplyContent, ExtractTypedNoteOrFilter
```

### Pattern: one list engine, three tables

**What:** a `settingsListDef` struct (page key, order, icon, title key, codes, right, reader, entry text and kind, delete apply, add parts, strings) and one set of functions that registers a page from it. **Why:** three hand-written page files would be near copies and trip `dupl` (threshold 100); a table keeps "a later list is one row" (the Deletion and Member requirements pattern).

### Anti-Patterns to Avoid

- Putting a word, keyword or name in callback data, a button label or a log line (Pitfall 28, the door's log rule).
- Deleting by text from the page (`RemoveFilter(chat, kw)`): a card drawn for one row could delete a re-added row of the same text (the D-33 rule "a card binds the object it was drawn for").
- Using `GetBlacklistSettingsContext`, `GetFiltersListContext`, `GetNotesListContext`, `CountFilters`, `DoesFilterExists`, `DoesNoteExists` or `notes.GetNotes` on a page path: each hides errors or creates rows.
- A second wait key or a second door (D-04).
- Showing an entry, a count or the action in a group menu (D-01).
- Re-recording a golden to make an extraction pass.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Converting a typed filter or note | a new parser | `content.ExtractNoteAndFilter`'s reply branch via `ExtractTypedNoteOrFilter` | parity with the command's stored row |
| Length refusals | own counters | `content.TextLengthRefusal` | the commands' words and counting |
| Button link checks | own URL rules | `content.ValidButtonURL` (inside `preFixes`) | same drop rule as the commands |
| Two-tap confirm | a new card | `settingsCardExpired`, `settingsCardObjectID`, `settingsCardRows`(`To`) | stateless, expiry, D-33 |
| Waiting for typed input | a new Redis state | the Phase 11 wait + one field + one script | one wait per user, existing cancel and expiry |
| Pacing / Telegram calls | anything | nothing new: the pages make no Telegram call beyond the press pipeline (and the door's live checks) | — |
| Bidi-safe display | own stripping | `memberReqStripBidi`, `staffDisplayTitle`, `html.EscapeString` | existing, tested |

## Runtime State Inventory

Not a rename or migration phase; omitted. The one new piece of runtime state is the `w` field of the existing `fuku:settings:input:<user>` hash (Redis only, ten-minute TTL, deleted by every end of a wait); it needs an AGENTS.md "Data" amendment, nothing else.

## Common Pitfalls

### Pitfall 1: user text where it must not be
**What goes wrong:** a word ends up in a button label, callback data, a log line or an unescaped HTML message (`<` breaks the edit; a right-to-left override reorders the screen). **How to avoid:** numbered buttons; escape after cutting; strip bidi controls; apply functions log IDs only (C-14). **Warning signs:** a test that searches the bot's requests and the log hook for a sentinel word finds it.

### Pitfall 2: a forged or stale row ID
**What goes wrong:** `r` from another chat or a deleted row. **How to avoid:** every read and delete carries `chat_id = ?`; zero rows is "already removed"; the same answer for gone and foreign (no oracle). **Warning signs:** a delete query without `chat_id`.

### Pitfall 3: huge lists and huge notices
**What goes wrong:** a blacklist add of 2,000 two-letter words produces a notice over 4,096 characters (the command's own reply already can, which Telegram refuses after the words are stored). **How to avoid:** counts plus capped previews in the page notice; the fit test uses the worst typed message. **Warning signs:** a notice built by joining every word.

### Pitfall 4: two admins on one list
**What goes wrong:** both add the same keyword (filters and notes: `ON CONFLICT DO NOTHING`, the second insert does nothing; blacklist on PostgreSQL: unique violation reported as a save failure), or both delete one entry. **How to avoid:** the apply result distinguishes `added` from `exists` (new `AddFilterContext` returning whether a row was inserted; the command maps `exists` from a lost race to its unchanged success reply, the page to "already on the list"); delete is conditional. The 150-filter cap can be exceeded by two simultaneous adds at 149, as with the command [ASSUMED: accepted].

### Pitfall 5: the door's single wait per user
**What goes wrong:** an owner with two private menus (two groups) presses Add in both; the second replaces the first and a keyword typed for the first lands in the second group. **How to avoid:** the replaced note on the prompt (existing), the step-2 prompt names the group and the keyword, the saved notice names the group. **Warning signs:** a prompt that does not show the group line.

### Pitfall 6: rolling deploys
**What goes wrong:** a list press on a replica on Phase 18 code is `common_callback_invalid_request`; a typed list reply on such a replica reads a wait whose page it does not know as "not readable" (logged), then takes and fails to parse the marker, so the message passes on unanswered while the wait stays live for new replicas [VERIFIED: settings_input.go:356-360, door.go:155-169 read this session]. **How to avoid:** accepted and documented like the Phase 11 caveat; `/cancel` works on old code (it deletes by key).

### Pitfall 7: the action that is not what it seems
**What goes wrong:** "Ban" set on the page, then a word added (page or command) acts with Warn; `none` labelled "Off" while it deletes. **How to avoid:** C-02 mixed line, A19-3, C-15 labels.

### Pitfall 8: dupl and gocyclo
**What goes wrong:** three page files or a long door `run` with list branches exceed the linters. **How to avoid:** the list engine and a separate `runList`.

## Code Examples

### Apply function shape (follows the Phase 18 apply functions, e.g. `applyLogCategory` described in AGENTS.md)

```go
// Sketch [ASSUMED names]: the command and the page call it; it holds the validation, the refusals,
// the write and the cache drop (inside the repository), and returns what each caller maps.
type blacklistAddOutcome int

const (
	blacklistAdded blacklistAddOutcome = iota // at least one word stored
	blacklistNothingAdded
	blacklistSaveFailed
)

type blacklistAddResult struct {
	Outcome  blacklistAddOutcome
	Added    []string // stored, lowercased
	Already  []string
	TooLong  []string // previews, as the command shows them
	TooBroad []string // A19-2
	Failed   int      // words whose write failed (logged by chat ID only)
}

// existing is the caller's read of the current words: the command passes its cached Triggers() as
// today (golden unchanged), the page its fresh read (a read error was refused before the call).
func applyBlacklistAdd(ctx context.Context, chatID int64, words, existing []string) blacklistAddResult
```

### Delete by ID (precedent: `memberreq.TakeHoldByID`, holds.go:133-149)

```go
// Sketch [ASSUMED name]: one conditional statement; zero rows is "already removed".
func RemoveBlacklistByID(ctx context.Context, chatID int64, id int64) (word string, removed bool, err error) {
	var rows []models.BlacklistSettings
	res := db.DB.WithContext(ctx).Clauses(clause.Returning{}).
		Where("id = ? AND chat_id = ?", id, chatID).Delete(&rows)
	if res.Error != nil {
		return "", false, res.Error
	}
	if len(rows) != 1 {
		return "", false, nil
	}
	cache.DeleteCache(cache.CacheKey("blacklist", chatID)) // the key RemoveBlacklist drops
	return rows[0].Word, true, nil
}
```

### Advance script (S-03)

```lua
-- KEYS[1] wait, KEYS[2] marker. ARGV: old nonce, new nonce, new part, pending, expiry ms,
-- ttl ms, marker value, marker ttl ms. Returns 1 when this caller advanced the wait.
if redis.call('HGET', KEYS[1], 'n') ~= ARGV[1] then return 0 end
if redis.call('HEXISTS', KEYS[1], 'k') == 1 then return 0 end
for _, f in ipairs(redis.call('HKEYS', KEYS[1])) do
  if string.sub(f, 1, 1) == 'r' then redis.call('HDEL', KEYS[1], f) end
end
redis.call('HSET', KEYS[1], 't', ARGV[3], 'w', ARGV[4], 'n', ARGV[2], 'x', ARGV[5])
redis.call('PEXPIRE', KEYS[1], ARGV[6])
redis.call('SET', KEYS[2], ARGV[7], 'PX', ARGV[8])
return 1
```

### Shared too-broad check (C-03)

```go
// Sketch [ASSUMED name]: true when a normalised word would act on almost every message.
func keywordTooBroad(word string) bool {
	if strings.TrimSpace(word) == "" {
		return true
	}
	if utf8.RuneCountInString(word) != 1 {
		return false
	}
	r, _ := utf8.DecodeRuneInString(word)
	return !unicode.Is(unicode.So, r) // one emoji or other symbol stays allowed
}
```

## State of the Art

Nothing external changes; inside the project the pattern is the Phase 18 one (goldens, apply functions, parity, drift, authority parity) plus the Phase 11 door. Deprecated or outdated: the help text's claim that the default blacklist mode is `none` (C-02 d).

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | A fresh read of up to ~10,000 short rows per page draw is acceptable | S-05 | slow page for a huge blacklist; fallback SQL paging by ID |
| A2 | Adding the four list keys to `skipLocal` would cost a Redis round trip per group message | S-05 | none if wrong (we do not add them) |
| A3 | Row IDs never exceed 17 digits in practice | C-04 | a row without a delete button (counted, logged) |
| A4 | The worst page text stays under ~2,500 UTF-16 units with a 32-code-point cap | S-06 | fit test fails; lower the cap |
| A5 | The two cached bot predicates rarely disagree, so one composite right for add, remove and action is acceptable | S-10 | a press refused where `/blacklistaction` would pass |
| A6 | Simultaneous adds may exceed the 150-filter cap, as with the command | Pitfall 4 | one or two filters over the cap |
| A7 | The sketches' names (`applyBlacklistAdd`, `RemoveBlacklistByID`, `keywordTooBroad`, `ExtractTypedNoteOrFilter`, `settingsRightRestrictBotRestrict`, codes `bg bx by ba bc`, keys `b i e`) | Code Examples, S-06 | planner may rename; no behaviour depends on them |
| A8 | A typed blacklist add should use the text or the caption (`GetText`), like `ctx.Args()` | S-08 | a photo caption ignored instead of used |

Owner confirmations for `19-SPIKE.md` (all provisional): A19-1 fresh list reads (C-01); A19-2 too-broad refusal for `/addblacklist` and `/filter` (C-03); A19-3 new words inherit a uniform action (C-02); A19-4 note name rule for `/save` (C-11); A19-5 Main state: count in private only via the hook (C-07); A19-6 the `w` field in the wait hash (S-03); A19-7 existing keyword or name refused from the page (C-09); A19-8 Main flat grid at 18 cells, 11/10 rows (S-07); A19-9 delete card for all three (D-06); A19-10 lists private-only (D-01); A19-11 "Delete only" label for `none` (C-15).

## Open Questions

1. **Does a chain-built PostgreSQL accept `none`?** What we know: both CHECKs are in the chain (C-16). Unclear: whether production has the old one. Recommendation: the PostgreSQL integrity test in Plan 19-01 decides; the migration is written only if it fails.
2. **Should the command quirks be fixed?** (unescaped HTML in replies and in the blacklist group notice `blacklists_bl_watcher_*` built with `formatting.Shtml()` and the raw word, blacklists.go:518-523; the `/save` caption panic; the two-word reply quirk). Recommendation: no, in this phase (scope and goldens); list them as follow-ups in `19-SPIKE.md`.
3. **Owner answers A19-2/3/4.** If declined, drop the matching task; the goldens are unaffected because they exclude those cases.
4. **Action on the list page or its own screen?** UI-SPEC decides; both fit the row budget (S-06).

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go | build, tests | ✓ | go1.26.0 | — |
| gcc (cgo for SQLite tests) | tests | ✓ | 13.3.0 | — |
| `cienv.sh` | CI-like test runs | ✓ | — | — |
| golangci-lint | `make lint` | ✗ (2.5.0 built with go1.25.1) | — | vet, gofmt, manual dupl/gocyclo read, `~/go/bin/gosec` (present) |
| PostgreSQL server | `make test-postgres-integrity`, C-16 | ✗ (`localhost:5432 - no response`) | psql client only | CI's PostgreSQL service |
| Redis | tests | miniredis | — | — |

Focused existing command tests pass here with `cienv.sh` (`TestAddListActionAndRemoveBlacklistCommands`, `TestAddListWatchAndRemoveTextFilter`, `TestAddGetListAndRemoveTextNote` and two validation tests: `ok ... 1.226s`) [VERIFIED: run]. Do not run the whole `./fuku/modules` package here (about 25 minutes).

## Validation Architecture

`workflow.nyquist_validation` is `false` in `.planning/config.json`; this section is included because the orchestrator asked for it.

### Test Framework

| Property | Value |
|----------|-------|
| Framework | Go `testing` + testify assertions, `-tags testtools -race`, SQLite through `internal/testdb`, miniredis, hand-written bot fakes |
| Config file | Makefile (`make test`, `-timeout 40m` per package) |
| Quick run command | `go test -tags testtools -race -count=1 -run '^(TestSettingsBlacklist\|TestSettingsFilters\|TestSettingsNotes\|TestSettingsList\|TestLegacyBlacklist\|TestLegacyFilter\|TestLegacyNote\|TestApplyBlacklist\|TestApplyFilter\|TestApplyNote\|TestSettingsMain\|TestSettingsPhase19)' ./fuku/modules ./fuku/i18n` |
| Full suite command | `make test` |

### Test families the phase needs

| Family | What it proves | Where | Est. time |
|---|---|---|---|
| `TestLegacyBlacklistRequests`, `TestLegacyFilterRequests`, `TestLegacyNoteRequests` | every request and stored row of the commands, recorded before extraction, never re-recorded (exclusions in S-01) | `*_legacy_request_test.go` | ~1 s each |
| `TestBlacklistActionValuesOnPostgres` (PG integrity list) | which of the five values the chain accepts (C-16) | `fuku/db/blacklists` or migrations | PG only |
| `TestKeywordTooBroad`, `TestNoteNameRule` | the shared validation, incl. the new command replies | modules | <0.1 s |
| `TestExtractTypedNoteOrFilter` | typed content = reply-form extraction for single-word keywords (S-04) | `fuku/utils/content` | <0.1 s |
| `TestApply*` | outcomes, rows, cache drops, logs by ID | modules | ~1 s |
| `TestSettingsParity{BlacklistAdd,BlacklistRemove,BlacklistAction,FilterAdd,FilterRemove,NoteAdd,NoteRemove}` | page and command leave the same rows and cache state (delete by ID vs by word) | modules | ~3 s |
| `TestSettingsDrift{Blacklist,Filters,Notes}` | the page draws what the command stored (incl. mixed action) | modules | ~1.5 s |
| `TestSettingsAuthorityParity{Blacklist,Filters,Notes}` | press and typed reply refuse what the command refuses, with the carve-out | modules | ~3 s |
| `TestSettingsListForged` | bad `r`, `v`, `e`, both `v` and `r`, a group-menu list press, another chat's ID | modules | ~1 s |
| `TestSettingsList*CallbackBytes` | every button ≤ 64 bytes at 14-char chat, 17-digit ID; 0 omitted | modules | <0.1 s |
| `TestSettingsList*PageFitsEveryLocale` | worst entry, worst notice, 8 languages, 3800 units, 20-code-point cells | modules | ~0.5 s |
| `TestSettingsListReadsCreateNothing` | opening pages and Main creates no row (no `notes_settings`, no `chats`) | modules | ~0.5 s |
| `TestSettingsListDoor*` (`OneStep`, `TwoStep`, `Refusals`, `Replaced`, `Expired`, `Cancel`, `Album`, `NoRedis`) | the door branch and the advance script; Phase 11 door tests unchanged | modules | ~4 s |
| `TestSettingsStaffPrivacy` (extended), `TestSettingsListGroupMenuShowsNothing` | no word, keyword, name or count in a group menu; no Staff Group | modules | +2 s |
| `TestSettingsGateAllDoors` (+ list Yes, list typed reply columns), `TestSettingsControlActionAllowList` (+ action control) | the owner gate decides the same at the new doors | modules | +1 s |
| `TestSettingsListLogsCarryIDsOnly` | log hook over failing seams: no sentinel word | modules | ~0.5 s |
| `TestSettingsPhase19EndToEnd` | menu add → word acted on, filter answers, `#note` and `/get` fetch; menu delete → no longer | modules | ~4 s |
| `TestSettingsMain*`, `TestSettingsPhase18Codes` (updated), label and limits tables | grid 18 cells, codes unique, strings in 8 languages | modules, `fuku/i18n` | ~1 s |

Estimated total ≈ 25 to 35 s against the 40 s budget; no single test over 5 s [ASSUMED: estimates from Phase 18 measurements: `TestSettingsPhase18EndToEnd` 3.95 s, `TestSettingsStaffPrivacy` 22.8 s total, +1 s for six pages; 18-1x SUMMARY]. CI measured the package at 1129 s to 1760 s of 2400 s [CITED: CONTEXT and 18-15 SUMMARY].

### Phase Requirements → Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| PAGE-07 | list, page, delete, add, action | integration (fake bot, SQLite, miniredis) | `go test -tags testtools -race -count=1 -run '^TestSettingsBlacklist' ./fuku/modules` | ❌ Wave 0 |
| PAGE-08 | list, page, delete, two-step add | integration | `-run '^TestSettingsFilters'` | ❌ |
| PAGE-09 | list, page, delete, two-step add | integration | `-run '^TestSettingsNotes'` | ❌ |
| criterion 4 | stored, cached, refused as the command; works at once | parity + e2e | `-run '^(TestSettingsParity\|TestSettingsPhase19EndToEnd)'` | ❌ |

### Sampling Rate
- **Per task commit:** the focused families of the plan plus the Phase 11 door tests when the door changes.
- **Per plan that registers a page or changes the door:** the whole `./fuku/modules` package once (the Phase 17/18 lesson), in the background.
- **Phase gate:** `make test`, `make check-translations`, `make check-docs`, lint stand-in, all green before `/gsd-verify-work`.

### Wave 0 Gaps
- [ ] The three golden files (each committed alone before its extraction).
- [ ] A list test toolkit `settings_list_env_test.go` on top of `settings_env_test.go` and `settings_edit_env_test.go` (seeding rows with fresh chat IDs from a reserved block, cleanups of the three tables and the four cache keys, a sentinel word).
- [ ] No framework install needed.

## Security Domain

`security_enforcement` is true (ASVS level 1).

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | no (Telegram identity) | — |
| V3 Session Management | partly | the wait: per user, nonce-checked, TTL, deleted by every end |
| V4 Access Control | yes | live `getChatMember`, owner gate, per-control `settingsRight`, chat-scoped reads and deletes |
| V5 Input Validation | yes | strict callback parsing in step 1 (`Validate`), shared word/keyword/name validation, `TextLengthRefusal`, `ValidButtonURL` |
| V6 Cryptography | no new | `crypto/rand` nonces (existing) |
| V7 Error handling and logging | yes | IDs-only logs (C-14) |
| V8 Data protection | yes | user text in Redis bounded and short-lived; never in callback data; group menus show no entry |

### Known Threat Patterns

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Forged `c` naming another group | Elevation | press pipeline: live admin check in that group, owner gate |
| Forged `r` of another chat's row | Tampering / Information disclosure | `WHERE id AND chat_id`; same answer for gone and foreign |
| Stale card Yes after a re-add | Tampering | delete by ID (D-33), expiry minute |
| Member reads lists in a group menu | Information disclosure | D-01: note only; list actions require `c` in step 1 |
| Abusive text breaking the menu (HTML, bidi) | Tampering | escape after cut, strip bidi, numbered labels |
| Over-broad blacklist word deleting every message | Denial of service | C-03 refusal (A19-2) |
| Keyword leaking through logs or Redis dumps | Information disclosure | no logs; `w` lives ten minutes at most, deleted on every end |
| Huge typed message exhausting the notice | Denial of service | capped notices, the 3800 fit test |

## Proposed Plan Decomposition (D-13 order, mvp: the tracer is the Blacklist page)

D-13 asks for "the two-step flow" among the shared pieces before the tracer; the two-step machinery is used only by Filters and Notes, so it is recommended to build it in the Filters plan (the tracer stays thin). Each plan 2 to 4 tasks.

| Plan | Content | Tasks | Size |
|---|---|---|---|
| 19-01 | Blacklist goldens committed alone → `applyBlacklistAdd/Remove/Action` + shared too-broad check (A19-2, its own commit after the golden) + repository additions (fresh reader, entry reader, `RemoveBlacklistByID`, `SetBlacklistActionContext`) + the PostgreSQL value test (and the C-16 migration only if it fails) | 3-4 | M |
| 19-02 | List engine + Blacklist page **tracer**: open, page, delete card, action (incl. mixed/empty/inert lines), group-menu note, composite right, Main hook (C-07), its strings; Main constants and codes test to 16 cells | 4 | L |
| 19-03 | Door extension with the one-step part: list parts in the wait, list branch, `authorize(right)`, generalised bot classification, Add/Cancel actions, wait-ended wording; Blacklist add by typed message; door tests (Phase 11 tests unchanged) | 3 | M |
| 19-04 | Filters goldens alone → `content` refactor with its pure parity test → `applyFilterAdd/Remove` (+ too-broad for `/filter`) + repository additions (`AddFilterContext` with inserted flag, count and existence with errors, fresh reader, `RemoveFilterByID`) | 3 | M |
| 19-05 | Two-step flow (advance script, `w`, prompt-ID script, step parts, step-1 checks) + Filters page (list, delete, two-step add, existing keyword refused, cap); Main 17 cells | 4 | L |
| 19-06 | Notes goldens alone → `applyNoteAdd/Remove` + note name rule (A19-4) + repository additions | 3 | M |
| 19-07 | Notes page on the two-step flow; Main 18 cells | 2-3 | M |
| 19-08 | Cross-page sweeps: privacy (8 languages, group menu), reads-create-nothing, bytes, fit, logs, gate doors, control allow-list, Staff Group, label lengths and limits tables | 3-4 | M |
| 19-09 | End-to-end through the real dispatcher (S-09) and a two-replica freshness check | 2 | S-M |
| 19-10 | Closing: AGENTS.md (pages, door `w` field, right, Main 18 cells, Data), README, `make generate-docs`, `19-SPIKE.md` (A19-1..11, live checks), phase gate, PAGE-07..09 ticked (D-14) | 3 | M |

## Live checks for `19-SPIKE.md` (owner steps)

1. On the production database: `SELECT conname, pg_get_constraintdef(oid) FROM pg_constraint WHERE conrelid = 'blacklists'::regclass;` and `/blacklistaction none` in a test group (C-16).
2. Main at 11 rows on a phone (owner, private menu) and the three new cells in uk, ru, hi.
3. A list page of 8 long entries (32 characters each, mixed scripts, an RTL word) on a phone: readability of the numbered buttons.
4. The two-step add in a real client: keyword, then a photo with a caption, then a text with formatting and buttons; compare with `/filter kw` as a reply.
5. A blacklist word added from the page acts in the group at once; a filter answers; `#note` and `/get` fetch.
6. Native read of the new strings in the 8 languages (especially the too-broad refusal and the "Delete only" label).
7. Owner answers A19-1 to A19-11.

## Sources

### Primary (HIGH confidence)
- Code read this session: `fuku/modules/blacklists.go`, `filters.go`, `notes.go`, `overwrite.go`, `callback_parse_overwrite.go`, `moderation_input.go`, `settings_input.go`, `settings_input_door.go`, `settings_menu.go`, `settings_pages.go`, `settings_card.go`, `settings_locks.go`, `settings_edit.go`, `settings_edit_actions.go`, `settings_edit_check.go` (head), `settings_hub.go` (label), `settings.go` (deep link URL), `memberreq_rules.go` (bidi strip); `fuku/db/{blacklists,filters,notes}/*.go`, `fuku/db/models/{blacklists,filters,notes}.go`, `fuku/db/cache/{ttl,local}.go`, `fuku/db/db.go`, `fuku/db/memberreq/holds.go`; `fuku/utils/content/content.go`, `fuku/utils/keyword_matcher/{matcher,cache}.go`, `fuku/utils/callbackcodec/callbackcodec.go`, `fuku/utils/chat_status/{access,chat_status}.go`; migrations `20250805200527`, `20250805204145`, `20250806093839`, `20250806100000`, `20260412000000`; gotgbot `ext/context.go`, `formatting.go`; `locales/en.yml`; tests `settings_main_test.go`, `settings_phase18_test.go`, `logchannels_legacy_request_test.go`.
- Runs: matcher probe, msgpack ID round trip, `ExtractNoteAndFilter` probe, callback byte table, focused command tests.

### Secondary
- `.planning/phases/19-blacklist-filter-and-note-pages/19-CONTEXT.md`, `.planning/ROADMAP.md`, `.planning/REQUIREMENTS.md`, `.planning/research/PITFALLS.md` (26, 28), `.planning/phases/18-remaining-settings-pages/18-RESEARCH.md`, `18-UI-SPEC.md` (M34, A18-A1), `18-SPIKE.md`, Phase 18 summaries (timings), `AGENTS.md`.

### Tertiary
- None (no web source was needed).

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH (nothing new).
- Architecture: HIGH for the code seams (each read); MEDIUM for the layout choices (UI-SPEC decides).
- Pitfalls: HIGH (each traced to code); MEDIUM for C-16 until the live check.

**Research date:** 2026-11-13
**Valid until:** 2026-12-13 (stable code base; re-check if Phase 18 follow-ups touch the door or the Main grid first)
