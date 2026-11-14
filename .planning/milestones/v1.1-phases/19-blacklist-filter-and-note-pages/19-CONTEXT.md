# Phase 19: Blacklist, Filter and Note Pages - Context

**Gathered:** 2026-11-13
**Status:** Ready for research (skip-discuss is on: the ROADMAP goal and the four success criteria are the spec. Decisions D-01 to D-14 are recommended defaults, not owner answers; see "Provisional status")

<domain>
## Phase Boundary

Phase 19 puts the three long per-group lists on the settings menu, on the framework Phases 9 to 18 built (`registerSettingsPage`, the owner gate, the press pipeline, the shared two-tap card, the typed-reply door of Phase 11):

- **Blacklist page** (PAGE-07): the group's blacklisted words, paged; delete one; add one by typing it in the bot's private chat; set the blacklist action. Commands: `/blacklists`, `/addblacklist`, `/rmblacklist`, `/blacklistaction` (and their `blocklist` aliases).
- **Filters page** (PAGE-08): the filters, paged; delete one; add one by typing the keyword and then the reply (two typed messages). Commands: `/filter`, `/stop`, `/filters`.
- **Notes page** (PAGE-09): the notes, paged; delete one; add one by typing the name and then the content (two typed messages). Commands: `/save`, `/clear`, `/notes`, `/get`.
- **Parity** (success criterion 4): each add, delete and action change is stored, cached and refused exactly as its command does it, and works in the group at once: the blacklisted word is acted on, the filter answers, the note can be fetched.
- **PLAT-05 binds**: every new string exists in all 8 locales with limits tables and label-length tests.

Mode is `mvp`: build one thin vertical slice first (the Blacklist page, whose add is one typed message), then Filters and Notes on the two-step flow.

Out of scope: search inside a long list (MENU2-04, deferred), editing an existing filter's or note's content from the menu, bulk removal (`/stopall`, `/clearall`, `/remallbl`) and the private-notes toggle (`/privnote`), which stay commands, linking anything, and changing what a command accepts except where a decision below says so.

</domain>

<provisional_status>
## Provisional status

The owner was not asked these grey areas: skip-discuss is on and the standing instruction is to continue without further questions. Each decision is the default recommended from the code and from Phases 10 to 18, recorded as an unconfirmed owner confirmation (A19-n) in `19-SPIKE.md` when the phase closes. Changing one can reopen the matching plan.

</provisional_status>

<decisions>
## Implementation Decisions (defaults, unconfirmed)

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

</decisions>

<research_questions>
## For the researcher (the ROADMAP research flags, made concrete)

- **R-1 The two-step state.** A filter or note takes the keyword (or name) and then the reply. Options: the keyword in the wait hash as a field (bounded length, ten-minute TTL, never logged; this amends the door's "IDs and codes only" rule for one bounded field), a short token to a separate Redis key, or the keyword carried in a first prompt message the second message replies to. Weigh against Pitfall 28 (user text in Redis behind a short token), the album gate, `/cancel`, a second Set replacing the first, a restart, no Redis (the door already fails closed), and what the door logs.
- **R-2 What "matches everything" means here.** Read how blacklist words, filter keywords and note names are matched (substring, whole word, regex, Aho-Corasick), what the commands refuse today (golden material), and which inputs would match every message or nothing.
- **R-3 What a typed reply may be.** A typed filter reply or note content is text, or media with a caption, with the buttons syntax the commands support. The commands extract it from a message (`/filter kw` plus a reply, or inline text); name the extraction functions and decide whether the typed message goes through the same extraction so the stored row equals what the command stores for the same message (the Phase 11 rule for greetings and rules), including media types and the 64-button and length limits.
- **R-4 The commands' exact behaviour.** For every command in the domain: rights, checks, replies, multi-word and multi-line quirks (`/addblacklist` takes several words), duplicates (`/filter` of an existing keyword updates), aliases, what a failed save looks like, which cache keys each write drops. This is the golden and apply-function material.
- **R-5 Reads and freshness.** The cache keys the lists use (`GetFiltersListContext`, `GetNotesListContext`, `GetBlacklistSettingsContext`), whether they are in `skipLocal`, the cost of paging a long list (a thousand words), and whether a page can read a row's ID and text through a cheap query that creates nothing. Check that the paged read cannot show another chat's row.
- **R-6 Callback design.** The codes, fields and byte budgets for open, page, delete ask, delete Yes, add, action choice; the cursor type; how a stale list (an entry deleted since it was drawn) behaves.
- **R-7 Main and the group menu.** Main has 15 cells and 10 rows for the owner; three more pages need rows. Say whether the grid grows or a second hub is needed (the Phase 18 AGENTS.md records the fallback hub as unbuilt), and design the group-menu note of D-01.
- **R-8 The door and the other waits.** A user with an open wait for a Welcome part who presses Add on a list: replaced, with the Phase 11 note. A typed reply that arrives after the ten minutes. A refused first step (a keyword the validation refuses) must not strand the user.
- **R-9 The end-to-end proof.** What exactly "works in the group at once" means for each list (the blacklist watcher at group `7`, the filters handler at group `9`, `/get` and `#note`), and which caches must be dropped for that.

</research_questions>

<canonical_refs>
## Canonical References

- `.planning/ROADMAP.md` (Phase 19), `.planning/REQUIREMENTS.md` (PAGE-07 to PAGE-09), `.planning/research/PITFALLS.md` (Pitfalls 26 and 28), `.planning/research/FEATURES.md`.
- `AGENTS.md` (the rulebook: handlers, callbacks, the settings pages, the typed-reply door, Data, Testing) and the Phase 18 bullets for the apply-function pattern, the toggle helpers and the cross-page sweeps.
- Phase 11: `.planning/phases/11-message-editor/` (the door, the wait state, the media and buttons checks); Phase 18: `.planning/phases/18-remaining-settings-pages/` (CONTEXT, UI-SPEC, the handoff brief and guardrails, the plans as a model).
- Code: `fuku/modules/blacklists.go`, `filters.go`, `notes.go`, `fuku/db/blacklists`, `filters`, `notes`, `fuku/modules/settings_input*.go`, `settings_edit.go`, `settings_locks.go`, `settings_card.go`, `fuku/utils/extraction`.

</canonical_refs>
