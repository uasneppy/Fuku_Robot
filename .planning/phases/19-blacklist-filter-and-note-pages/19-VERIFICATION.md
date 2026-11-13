---
phase: 19-blacklist-filter-and-note-pages
verified: 2026-11-13T19:44:15Z
status: human_needed
score: "20/20 truths verified in code and in tests run by the verifier (4 ROADMAP success criteria plus 16 plan truths); the owner's 15 live checks and the owner confirmations in 19-SPIKE.md are deferred and block nothing in code"
covered_files:
  - ".planning/phases/19-blacklist-filter-and-note-pages/19-01-PLAN.md"
  - ".planning/phases/19-blacklist-filter-and-note-pages/19-01-SUMMARY.md"
  - ".planning/phases/19-blacklist-filter-and-note-pages/19-02-PLAN.md"
  - ".planning/phases/19-blacklist-filter-and-note-pages/19-02-SUMMARY.md"
  - ".planning/phases/19-blacklist-filter-and-note-pages/19-03-PLAN.md"
  - ".planning/phases/19-blacklist-filter-and-note-pages/19-03-SUMMARY.md"
  - ".planning/phases/19-blacklist-filter-and-note-pages/19-04-PLAN.md"
  - ".planning/phases/19-blacklist-filter-and-note-pages/19-04-SUMMARY.md"
  - ".planning/phases/19-blacklist-filter-and-note-pages/19-05-PLAN.md"
  - ".planning/phases/19-blacklist-filter-and-note-pages/19-05-SUMMARY.md"
  - ".planning/phases/19-blacklist-filter-and-note-pages/19-06-PLAN.md"
  - ".planning/phases/19-blacklist-filter-and-note-pages/19-06-SUMMARY.md"
  - ".planning/phases/19-blacklist-filter-and-note-pages/19-07-PLAN.md"
  - ".planning/phases/19-blacklist-filter-and-note-pages/19-07-SUMMARY.md"
  - ".planning/phases/19-blacklist-filter-and-note-pages/19-08-PLAN.md"
  - ".planning/phases/19-blacklist-filter-and-note-pages/19-08-SUMMARY.md"
  - ".planning/phases/19-blacklist-filter-and-note-pages/19-09-PLAN.md"
  - ".planning/phases/19-blacklist-filter-and-note-pages/19-09-SUMMARY.md"
  - ".planning/phases/19-blacklist-filter-and-note-pages/19-10-PLAN.md"
  - ".planning/phases/19-blacklist-filter-and-note-pages/19-10-SUMMARY.md"
  - ".planning/phases/19-blacklist-filter-and-note-pages/19-11-PLAN.md"
  - ".planning/phases/19-blacklist-filter-and-note-pages/19-11-SUMMARY.md"
  - ".planning/phases/19-blacklist-filter-and-note-pages/19-12-PLAN.md"
  - ".planning/phases/19-blacklist-filter-and-note-pages/19-12-SUMMARY.md"
  - "AGENTS.md"
  - "Makefile"
  - "README.md"
  - "docs/src/content/docs/api-reference/callbacks.md"
  - "docs/src/content/docs/api-reference/permissions.md"
  - "fuku/db/blacklists/entries_test.go"
  - "fuku/db/blacklists/repository.go"
  - "fuku/db/filters/entries_test.go"
  - "fuku/db/filters/repository.go"
  - "fuku/db/migrations/blacklist_action_check_test.go"
  - "fuku/db/notes/entries_test.go"
  - "fuku/db/notes/repository.go"
  - "fuku/i18n/settings_locale_test.go"
  - "fuku/modules/blacklists.go"
  - "fuku/modules/blacklists_apply_test.go"
  - "fuku/modules/blacklists_legacy_request_test.go"
  - "fuku/modules/filters.go"
  - "fuku/modules/filters_apply_test.go"
  - "fuku/modules/filters_legacy_request_test.go"
  - "fuku/modules/list_validation.go"
  - "fuku/modules/list_validation_test.go"
  - "fuku/modules/notes.go"
  - "fuku/modules/notes_apply_test.go"
  - "fuku/modules/notes_legacy_request_test.go"
  - "fuku/modules/settings.go"
  - "fuku/modules/settings_blacklist.go"
  - "fuku/modules/settings_blacklist_test.go"
  - "fuku/modules/settings_card.go"
  - "fuku/modules/settings_edit_hooks_test.go"
  - "fuku/modules/settings_filters.go"
  - "fuku/modules/settings_filters_test.go"
  - "fuku/modules/settings_gate_test.go"
  - "fuku/modules/settings_input.go"
  - "fuku/modules/settings_input_door.go"
  - "fuku/modules/settings_input_test.go"
  - "fuku/modules/settings_list.go"
  - "fuku/modules/settings_list_door_test.go"
  - "fuku/modules/settings_list_env_test.go"
  - "fuku/modules/settings_list_framework_test.go"
  - "fuku/modules/settings_list_input.go"
  - "fuku/modules/settings_list_locale_test.go"
  - "fuku/modules/settings_list_privacy_test.go"
  - "fuku/modules/settings_list_test.go"
  - "fuku/modules/settings_list_twostep_test.go"
  - "fuku/modules/settings_main_test.go"
  - "fuku/modules/settings_menu.go"
  - "fuku/modules/settings_menu_test.go"
  - "fuku/modules/settings_notes.go"
  - "fuku/modules/settings_notes_test.go"
  - "fuku/modules/settings_pages.go"
  - "fuku/modules/settings_phase18_test.go"
  - "fuku/modules/settings_phase19_e2e_test.go"
  - "fuku/modules/settings_phase19_test.go"
  - "fuku/modules/settings_staff_privacy_test.go"
  - "fuku/utils/content/content.go"
  - "fuku/utils/content/extract_characterization_test.go"
  - "fuku/utils/content/extract_typed_test.go"
  - "locales/en.yml"
  - "locales/es.yml"
  - "locales/fr.yml"
  - "locales/hi.yml"
  - "locales/id.yml"
  - "locales/pt.yml"
  - "locales/ru.yml"
  - "locales/uk.yml"
  - "migrations/20261113120000_drop_legacy_blacklist_action_check.sql"
covered_digest: "v4:sha256:16ac75c45dec8be49bd16b5165dad7fc597009179e20b1d87290ba3ddb5258c0"
behavior_unverified: 0
overrides_applied: 0
gaps: []
human_verification:
  - test: "Run the owner's live checks 1 to 9 of 19-SPIKE.md in a real supergroup and a real private chat: the C-16 production CHECK, Main at 11 rows on a phone, a page of eight long entries, the two-step add in a real client, the effect of an added word, filter and note in the group, a group menu showing nothing, the six changed commands, the Blacklist action screen with mixed, Delete only and an inert stored value, and a native-speaker read of the 67 new strings"
    expected: "each check of 19-SPIKE.md passes as written there; a page of eight long entries and Main at 11 rows fit a phone without wrapping the buttons badly"
    why_human: "live Telegram clients, a real phone, real members and native-speaker wording cannot be observed from code or from a faked Telegram"
  - test: "Decide the provisional owner confirmations A19-1 to A19-11 and the flagged UI points A19-A1 to A19-A17 in 19-SPIKE.md (including A19-2 and A19-4, which change what /addblacklist, /filter and /save accept, and the fail-closed lookups of /filter and /save)"
    expected: "each confirmation is accepted or names the one commit that reverts it (the SPIKE lists them)"
    why_human: "owner decisions; the code implements the recommended defaults and the decisions are not in the code"
  - test: "Decide whether settings_help_msg should name the three list pages (A19-A17); it does not today"
    expected: "either the help text names the pages in all eight locales within the one-message limit, or the owner accepts the stale help"
    why_human: "a wording and product decision; not a success criterion of the phase"
  - test: "Run the full fuku/modules package in CI (or accept the executor's reported run) and watch the per-package time"
    expected: "the package stays under the 40 minute limit with margin; the executor measured 1897 s"
    why_human: "CI runner timing; the phase's own D-12 budget (about 40 s added) was exceeded (see warnings)"
---

# Phase 19: Blacklist, Filter and Note Pages Verification Report

**Phase Goal:** As a group owner, I want to browse, delete and add blacklisted words, filters and notes from the bot's private chat, so that I can manage long lists without commands.
**Verified:** 2026-11-13T19:44:15Z
**Status:** human_needed
**Re-verification:** No (first verification of Phase 19; no earlier VERIFICATION.md exists)

The phase is complete in code and in tests. I found no truth that fails and no must-have that is uncertain in code. The status is `human_needed` because the owner's live checks in `19-SPIKE.md` (15 numbered checks) and the provisional owner confirmations were deliberately deferred, as the closing plan records. I did not run any live check and do not claim one.

## Scope and method

- Phase range: the 2dff096 to 966ba6a work after Phase 18 closed (`d93ec1d` is the Phase 18 closing commit). The first Phase 19 commit is `32e158b` (context) and the first implementation commit is `a8de41a` (the blacklist golden). `99f796e` and `7a18e10` are Phase 18 gap-closure commits that sit between the Phase 18 close and the Phase 19 work; they are not Phase 19 scope and I did not re-verify them.
- Changed files: 66 non-planning files since `d93ec1d` (listed in `covered_files`), of which 22 are production Go files, one migration, eight locale files, the rulebook and docs, and 35 test files.
- Truths: the four ROADMAP success criteria, and the `must_haves.truths` of plans 19-01 to 19-12 (each plan's truth lines were read from its frontmatter).
- Every truth below was checked against production code (file:line) and against a test that exercises the behaviour. Where a test was run by me, it is marked "ran". The whole `./fuku/modules` package was not run by me (per the brief); the executor's reported whole-package result is cited as reported evidence only.

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| T1 | SC1: the Blacklist page lists the chat's words paged, deletes one, adds one typed in private, and sets the blacklist action | VERIFIED | Page table `settingsBlacklistDef` at `fuku/modules/settings_blacklist.go:483`, registered through `registerSettingsList`; list and paging `renderSettingsList` (`settings_list.go:282`, clamp and Prev/Next only where a page exists); delete ask and Yes `settingsListRunAsk` (`:470`) and `settingsListRunYes` (`:491`) calling `applyBlacklistRemoveByID`; add `settingsBlacklistTyped` (`settings_blacklist.go:437`) through `applyBlacklistAdd`; action `settingsBlacklistRunSet` (`:239`) through `applyBlacklistAction`. E2E subtest "blacklist" (`settings_phase19_e2e_test.go:158`) ran: a page-added word is stored as `spam` on warn, the next member message is deleted and warns once, admin and approved messages stay, Delete only deletes without a warn, a page delete stops the next message. Ran green |
| T2 | SC2: the Filters page lists filters paged, deletes one, and adds one by typing the keyword and then the reply | VERIFIED | `settingsFiltersDef` (`settings_filters.go:162`) with `TwoStep` engine hooks; step 1 `listStepOne` (`settings_list_input.go:366`) and step 2 `listStepTwo` (`:442`); the reply is stored through `applyFilterAdd` (`settings_filters.go` `settingsFiltersApply`, `filters.go:193`). E2E subtest "filters" ran: the page-stored filter answers `what are the Prices?` once with the stored reply and its Shop button, and stops answering after the page deletes it. Ran green |
| T3 | SC3: the Notes page lists notes paged, deletes one, and adds one by typing the name and then the content | VERIFIED | `settingsNotesDef` (`settings_notes.go:139`); name check `settingsNotesCheck` runs `checkNoteName` (`notes.go:131`); content through `applyNoteAdd` (`notes.go:152`); admin-only tag `AdminTagKey`. E2E subtest "notes" ran: `#rules` and `/get rules` fetch the page-added note, and after the page delete `/get rules` answers `notes_does_not_exist` and `#rules` sends nothing. Ran green |
| T4 | SC4: each add, delete and action change is stored, cached and refused exactly as the matching command does it, and works in the group at once | VERIFIED | One apply function per write, called by the command and by the page (see Key Links). Cache drops: `AddBlacklistContext` drops `blacklist:<chat>` (`fuku/db/blacklists/repository.go`), `RemoveBlacklistByID` drops it only when a row went, `SetBlacklistActionContext` drops it after the update; filters drop `filter_list` and `filters_optimized` (`invalidateFilterCaches`, `fuku/db/filters/repository.go:25`); notes drop `notes_list` (`invalidateNotesCache`, `fuku/db/notes/repository.go:85`). Parity tests `TestSettingsParityBlacklistAdd`, `...Remove`, `...Action`, `TestSettingsParityFilterAdd`, `...Remove`, `TestSettingsParityNoteAdd`, `...Remove` are in the 141-name run and passed. "Works in the group at once" is proved through the real dispatcher by the E2E subtests (T18). Refusals are shared: `checkFilterKeyword` (`filters.go:161`) and `checkNoteName` (`notes.go:131`) are the commands' own checks and the page's `Check` hooks |
| T5 | (19-01) A characterization golden of the four blacklist commands is committed alone before any production change and passes unchanged afterwards | VERIFIED | `a8de41a` adds only `fuku/modules/blacklists_legacy_request_test.go` (294 lines, one commit, never edited since: `git log --follow` shows one commit); the first production change is `f90a3ce` after it. The same holds for the filter golden `c223cb2` (before `deef07b`) and the note golden `1f159fc` (before `6d3f326`). The `ExtractNoteAndFilter` characterization `e696952` precedes the `ca22c2e` refactor. Ran green: `TestLegacyBlacklistRequests`, `TestLegacyFilterRequests`, `TestLegacyNoteRequests` |
| T6 | (19-01) The four blacklist apply functions hold the refusals, the write and its cache drop, return typed results, and log the chat ID and error only; the blacklist readers are fresh, chat-scoped, error-returning and create nothing | VERIFIED | `applyBlacklistAdd` (`blacklists.go:186`), `applyBlacklistRemove` (`:229`), `applyBlacklistRemoveByID` (`:252`), `applyBlacklistAction` (`:271`) with `blacklistAddResult` and the outcome types; logs use `chatId` and `error` fields only. `ListBlacklistEntriesFresh` (`fuku/db/blacklists/repository.go:119`), `GetBlacklistEntryFresh`, `CountBlacklistFresh`, `RemoveBlacklistByID` (`:155`, `WHERE id = ? AND chat_id = ?`, returns the row word). Reads: `TestSettingsBlacklistReadsCreateNothing` and `TestSettingsListReadsCreateNothing` ran green |
| T7 | (19-01, 19-05, 19-08) The too-broad refusal (A19-2) and the inherited blacklist action (A19-3) each land in their own commit after the golden; the filter and note fail-closed lookups and the note name rule (A19-4) are own commits too | VERIFIED | Commits in order: `12ee6a4` (A19-2 `/addblacklist`), `3025b0f` (A19-3), `f0dd7f7` (A19-2 `/filter`), `4f794f1` (fail-closed `/filter`), `2eeac8f` (A19-4 `/save`), `ed7988a` (fail-closed `/save`). Code: `keywordTooBroad` (`list_validation.go:18`), `blacklistSharedAction` (`blacklists.go:165`), `noteNameInvalid` (`list_validation.go:33`), `checkFilterKeyword` returns `filterSaveFailed` on a lookup error (`filters.go:161`), `checkNoteName` likewise (`notes.go:131`). Ran green: `TestApplyBlacklist*`, `TestApplyFilter*`, `TestApplyNote*` (in the 141-name run). The refusal is a single code point outside Unicode So or a blank word; the matchers are substring matchers (Aho-Corasick over lower-cased text, `blacklists.go:688`, `filters.go:810`), so a one-character word is the only kind that matches almost every message. Narrower than D-07's wording, recorded as A19-2 (see warnings) |
| T8 | (19-01) On a chain-built database `UPDATE blacklists SET action = v` accepts warn, mute, kick, ban and none and refuses any other value with SQLSTATE 23514; the legacy check is dropped by the append-only migration and the test is in the PostgreSQL integrity list | VERIFIED | Migration `migrations/20261113120000_drop_legacy_blacklist_action_check.sql` (one `ALTER TABLE ... DROP CONSTRAINT IF EXISTS chk_blacklists_action`, no transaction keywords; the legacy constraint was added by `migrations/20250805204145_add_foreign_key_relations.sql`). Timestamp is later than every existing file. `Makefile` integrity list includes `TestBlacklistActionValuesOnPostgres`. Ran on a throwaway PostgreSQL 16 cluster (see Behavioral Spot-Checks): `TestRepositoryMigrationChain` applied all 63 migrations, `TestBlacklistActionValuesOnPostgres` PASS, and `pg_constraint` on `blacklists` lists only `chk_blacklist_action` |
| T9 | (19-02) `PrivateOnly` (group menu shows only the note, no MainState or Open call); `settingsCardRowsTo`; the ninth right `settingsRightRestrictBotRestrict` with its bot halves; Main still 15 cells; 387 keys then | VERIFIED | `PrivateOnly` field and the `private` argument of `settingsMainEntries` (`settings_pages.go`, switch case `pg.PrivateOnly && !private`); `openPage` refuses a group menu before any page code (`settings_menu.go:595`); `settingsCardRowsTo` (`settings_card.go`, used by `settingsCardRows`); `settingsRightRestrictBotRestrict` (`settings_menu.go:93`), `settingsRightBotFirst` (`:162`), `settingsRightBotAfter` (`:189`). The bot-half refactor keeps the eight older rights' order; `TestSettingsRights`, `TestSettingsRightRefusalForParity`, `TestSettingsRightBotHalves`, `TestSettingsGateAllDoors` ran green |
| T10 | (19-03) The Blacklist page: eight a page in lower-cased code point order with ties by row ID, cut at 32 code points, bidi stripped, escaped; delete ask and Yes bound to the row; the action screen shows Action X only when uniform, Mixed with counts otherwise; Main 16 cells | VERIFIED | Ordering `settingsListCompare` (`settings_list.go:185`, `strings.Compare` on lower-cased text, ties by ID), page cut `settingsListPageSize = 8`, line `settingsListLine` and `settingsListLabel` (cut then `html.EscapeString`); action screen `renderSettingsBlacklistAction` (`settings_blacklist.go:196`), `settingsBlacklistUniformAction` (`settings_blacklist.go:99`), counts `settingsBlacklistActionBody`. Main shape `s|l / c|f / n|k / b|i / w|g / r|e / m|d / t|u / o|y / a / groups|close` (`settings_main_test.go:36`). `TestSettingsListBlacklistCallbackBytes`, `TestSettingsListBlacklistPageFitsEveryLocale`, `TestSettingsBlacklistDelete`, `TestSettingsMainPhase19` ran green |
| T11 | (19-04) Typed add through `applyBlacklistAdd`, judged like a press (live member record, owner gate, right, live chat); refusals keep the wait; the door's list branch; the bounded `w` and `q` fields and the atomic advance and prompt scripts | VERIFIED | The door's list branch: `settingsInputReply.run` checks `settingsListByPage` first (`settings_input_door.go:181-190`) and hands to `runList` (`settings_list_input.go:271`); authority `authorize(def.Right)` (`settings_input_door.go:244`) does `lockdownLiveMember`, the owner gate `settingsGate`, `settingsRightBotFirst`, `settingsRightRefusalFor`, `settingsRightBotAfter(..., true)`, live `getChat`. Wait store: fields `w` and `q` (`settings_input.go`, `Pending`/`StepMsgID`, `settingsInputPendingValid`), scripts `settingsInputAdvanceScript` (`:122`) and `settingsInputSetPromptScript`, `settingsInputAdvance` (`:389`). Refusal with no addable word keeps the wait (`settingsBlacklistTyped`). Tests `TestSettingsListDoorRefusals`, `TestSettingsListDoorAuthority`, `TestSettingsListDoorNoRedis`, `TestSettingsParityBlacklistAdd`, `TestSettingsListDoorAlbum` ran green. The Phase 11 editor door tests (`TestSettingsEditTyped*`) were not in the verifier's 141-name set and were not run by the verifier; `TestSettingsInput*` (9 subtests) ran as part of the list door names. The editor door's full run is reported only by the executor's whole-package result |
| T12 | (19-05) The filter golden, `applyFilterAdd` and `applyFilterRemove`, the filter readers, and `content.ExtractTypedNoteOrFilter` equal to `ExtractNoteAndFilter` for `/filter K` on the parity table | VERIFIED | `applyFilterAdd` (`filters.go:193`) through `checkFilterKeyword` (`:161`); `filterWrites` seam (`:60`); the command `addFilter` (`:320`) maps outcomes to its replies and keeps its own cap pre-check (`CountFilters`). `ExtractTypedNoteOrFilter` (`fuku/utils/content/content.go`) and `extractReplyContent` share one reply-form path with `ExtractNoteAndFilter`; `extract_characterization_test.go` (committed alone in `e696952`) and `extract_typed_test.go` ran green. `ListFilterEntriesFresh` (`fuku/db/filters/repository.go:165`) and `RemoveFilterByID` (`:192`) are chat-scoped |
| T13 | (19-06) The two-step engine: step 1 refuses blank, non-text and refused keywords, advances atomically with `settingsInputAdvance`, sends the step-2 prompt as a reply and records it; step 2 ignores a message whose ID is not greater than `q`, extracts the content, re-runs Check, claims, applies, finishes; an Add gate creates no wait; the test-only page proves it | VERIFIED | `listStepOne` (`settings_list_input.go:366`), `listStepTwo` (`:442`, `if r.msg.MessageId <= r.wait.StepMsgID { return }` at its head), `AddGate` consulted in `settingsListRunAdd` (`:135`), `settingsInputSetPrompt` after the send. Tests `TestSettingsListTwoStepFlow`, `TestSettingsListTwoStepBranches` (`settings_list_twostep_test.go`: every step-1 refusal including U+3000 alone, a `Check` error, the gate, a redelivered keyword, a taken and a failed `Apply`, an album, expiry, `/cancel`) and `TestSettingsListDoorTwoStep*` ran green |
| T14 | (19-07) The Filters page: listed, paged and deleted like Blacklist through the same engine; `settingsRightChangeInfo`; Main reads n/150 in private and In private in a group; the cap refuses an Add with no wait | VERIFIED | `settingsFiltersDef` (`settings_filters.go:162`): `Right: settingsRightChangeInfo`, `CountMax: filtersCapMax`, `AddGate: settingsFiltersAddGate` (`:141`, a fresh count, redraws page 1 with `settings_flt_refuse_cap`, no wait), `PrivateOnly` through `settingsListPage`. Keyword refusals are `checkFilterKeyword`'s (`settingsFiltersCheck`, `settings_filters.go:78`). `TestSettingsFiltersDelete`, `TestSettingsParityFilterRemove`, `TestSettingsListFiltersCallbackBytes`, `TestSettingsListFiltersPageFitsEveryLocale` ran green |
| T15 | (19-08) The notes golden, `applyNoteAdd` and `applyNoteRemove`, the note readers, and the fail-closed `/save` lookup; `/save` refuses a name with whitespace, a leading #, or over 100 code points (`notes_name_invalid`); a stored note is fetched by `#name` and `/get` at once and not after its delete | VERIFIED | `applyNoteAdd` (`notes.go:152`), `checkNoteName` (`:131`), `applyNoteRemoveByID`(`:201`), `resolveNotePrivateConflict` shared by `/save` and the page. Readers `ListNoteEntriesFresh` (`fuku/db/notes/repository.go:235`) read no `notes_settings` row. `TestLegacyNoteRequests` ran green; the watcher answers in the E2E "notes" subtest. The name rule is `noteNameInvalid` (`list_validation.go:33`) |
| T16 | (19-09, 19-10) The Notes page: admin-only tag, name then content, `settingsRightChangeInfoBotAdmin`, 452 `settings_` keys in each locale, Main at 18 cells (11 owner rows, 10 administrator rows); the cross-page sweeps: counts in private, `In private` in a group, codes unique and private-only, no screen or log names a word, the owner gate decides the same at the list doors | VERIFIED | `settingsNotesDef` (`settings_notes.go:139`, `Right: settingsRightChangeInfoBotAdmin`, `AdminTagKey`, `Order: 72`). Locale counts: 452 `settings_` keys in each of en, es, fr, hi, id, pt, ru, uk, with identical key sets (verified by grep). Main shape `settings_main_test.go:36-38` (18 cells, `settingsMainOwnerRows = 11`). Gate: `TestSettingsGateAllDoors` has the door "list Yes and list typed reply" (`settings_gate_test.go:1160`) and ran green. Privacy: `TestSettingsListGroupMenuShowsNothing` and `TestSettingsListLogsCarryIDsOnly` ran green; `settings_staff_privacy_test.go` renders the three pages in a group menu and in a private menu of an administrator who is not the creator |
| T17 | (19-10) The codes: Blacklist `bp bx by bo bs bn bc`, Filters `ig ix iy ia ic`, Notes `eg ex ey ea ec`, pages `b i e` registered once and `PrivateOnly`, no `Hidden`; the part table `(b,w) (i,k) (i,y) (e,n) (e,c)` | VERIFIED | Codes in `settings_blacklist.go:489-506`, `settings_filters.go:168-189`, `settings_notes.go:145-167` and the actions at `settings_blacklist.go:279-291`; `settingsListPage` sets `PrivateOnly: true` (`settings_list.go:578-584`); no `Hidden` in any of the three files (grep empty). Part table `settingsListParts` (`settings_list_input.go:38`). `TestSettingsPhase19Codes` ran green |
| T18 | (19-11) Through the real dispatcher with only Telegram faked: a page-added word is acted on by the group-7 watcher; a page-added filter answers and stops after its delete; a page-added note is fetched and stops after its delete; the pages work in a Staff Group; with no Redis the list, card and action work and Add answers the alert; a wait created on one replica is completed on another | VERIFIED | `TestSettingsPhase19EndToEnd` (`settings_phase19_e2e_test.go:157`) with six subtests "blacklist", "filters", "notes", "staff group", "no redis", "two replicas". Ran green with `-race` (2.4 s). The Telegram side is the fake of the settings environment, so the group behaviour here is the code path and not a live client (that is human check 5) |
| T19 | (19-12) The rulebook, README and docs describe the pages; `19-SPIKE.md` lists the owner's live checks and the owner confirmations with their revert commits; PAGE-07 to PAGE-09 ticked only by the closing plan | VERIFIED | `AGENTS.md`, `README.md`, `callbacks.md`, `permissions.md` contain the list pages (grep; `make check-docs` reports no drift). `19-SPIKE.md` exists (27,848 bytes, 15 numbered checks, A19-1 to A19-11, A19-A1 to A19-A17). `.planning/REQUIREMENTS.md` lines 73-75 ticked and rows 183-185 read Complete; PAGE-10 and PLAT-05 remain ticked |
| T20 | Cross-cutting: no migration beyond the one, no model change, no `main.go` change, no shutdown drain, no handler group, no new Redis key family, no `go.mod` change, no skipped test beyond the two PostgreSQL guards, no debt marker in the changed production code | VERIFIED | `git diff --stat d93ec1d..966ba6a -- main.go fuku/db/models .github/workflows/ci.yml fuku/config` is empty; the only migration is the one above; `Makefile` change is the integrity list only; added production lines contain no `go func`, `AddHandlerToGroup`, `RegisterHandler` or new `fuku:` family (the only `redis.NewScript` additions are the two scripts in the existing `settings_input.go` wait family); `git diff --exit-code 8c0c827 -- go.mod go.sum` exits 0; `t.Skip` added only in `fuku/db/migrations/blacklist_action_check_test.go` (two PostgreSQL-only guards, the allowed pattern); anti-pattern grep (`TBD|FIXME|XXX|TODO|HACK|PLACEHOLDER`) over the changed production files finds none |

**Score:** 20/20 truths verified in code and in tests. Behaviour that depends on the owner's live Telegram clients is listed under Human Verification; it is not counted as unverified here because the behaviour is exercised by the tests above.

### Requirements Coverage

| Requirement | Source Plan(s) | Description | Status | Evidence |
|-------------|----------------|-------------|--------|----------|
| PAGE-07 | 19-01, 19-02, 19-03, 19-04, 19-11, 19-12 | Blacklist page: words paged, one deleted, one added typed in private, action set | SATISFIED | T1, T4, T6, T8, T10, T11, T18 |
| PAGE-08 | 19-05, 19-06, 19-07, 19-11, 19-12 | Filters page: paged, one deleted, one added by keyword then reply | SATISFIED | T2, T12, T13, T14, T18 |
| PAGE-09 | 19-08, 19-09, 19-11, 19-12 | Notes page: paged, one deleted, one added by name then content | SATISFIED | T3, T15, T16, T18 |
| PAGE-10 | 19-10, 19-11 (bound) | Every setting stored, cached and refused as its command does | SATISFIED for this phase's writes | T4, T7, parity tests |
| PLAT-05 | 19-02, 19-03, 19-07, 19-09 | New strings in all 8 locales with limits rows | SATISFIED | T16 (452 keys in each of 8 files, identical key sets); `make check-translations` passes; `TestSettingsListKeysExist`, `TestSettingsLabelLengths` and the list locale tests ran green |

No orphaned requirement: `.planning/REQUIREMENTS.md` maps PAGE-07, PAGE-08 and PAGE-09 to Phase 19 and all three are in a plan's `requirements` field.

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|-----|--------|---------|
| `/addblacklist` (`blacklists.go:331`) and Blacklist page Add (`settings_blacklist.go:463`) | `applyBlacklistAdd` (`blacklists.go:186`) | direct call | WIRED | both call sites read in code |
| Blacklist page delete Yes (`settings_blacklist.go:298`) | `applyBlacklistRemoveByID` (`blacklists.go:252`) | direct call | WIRED | one conditional DELETE by (id, chat) |
| Blacklist page action set (`settings_blacklist.go:243`) and `/blacklistaction` (`blacklists.go:530`) | `applyBlacklistAction` (`blacklists.go:271`) | direct call | WIRED | |
| `/rmblacklist` (`blacklists.go:422`) | `applyBlacklistRemove` (`:229`) | direct call | WIRED | command-only write by design: the page deletes by row ID |
| `/filter` (`filters.go:383`) and Filters page add (`settings_filters.go` via `settingsFiltersApply`, the engine's `Apply`) | `applyFilterAdd` (`filters.go:193`) | direct call | WIRED | the page reaches it through `settingsFiltersApply` and the list engine's `Apply` hook (`settings_list_input.go` `listStepTwo`, `ts.Apply`) |
| `/stop` (`filters.go:467`) | `applyFilterRemove` (`:226`) | direct call | WIRED | command-only write by design |
| Filters page delete (`settings_filters.go:128`) | `applyFilterRemoveByID` (`filters.go:212`) | direct call | WIRED | |
| `/save` (`notes.go:343`) and Notes page add (`settingsNotesApply`, engine `Apply`) | `applyNoteAdd` (`notes.go:152`) | direct call | WIRED | |
| `/clear` (`notes.go:452`) | `applyNoteRemove` (`:170`) | direct call | WIRED | command-only write by design |
| Notes page delete (`settings_notes.go:128`) | `applyNoteRemoveByID` (`notes.go:201`) | direct call | WIRED | |
| Typed reply of a list wait | `runList` (`settings_list_input.go:271`) | `settingsInputReply.run` (`settings_input_door.go:183`) | WIRED | list branch precedes the editor path |
| Typed reply authority | `authorize` (`settings_input_door.go:244`) | `runList` | WIRED | live member, owner gate, bot and member rights, live chat |
| Two-step step 1 to step 2 | `settingsInputAdvance` (`settings_input.go:389`) and `settingsInputSetPrompt` | `listStepOne` (`settings_list_input.go:366`) | WIRED | nonce-guarded scripts, no claim held |
| Page `Check` hooks | `checkFilterKeyword` (`filters.go:161`) and `checkNoteName` (`notes.go:131`) | `settingsFiltersCheck`, `settingsNotesCheck` | WIRED | the command and the page refuse alike |
| Page readers | `ListBlacklistEntriesFresh`, `ListFilterEntriesFresh`, `ListNoteEntriesFresh` and the Count and Get readers | `settingsListReader` and friends (`settings_list.go`) | WIRED | fresh, chat-scoped, error-returning, create nothing |
| Framework: `PrivateOnly` | `openPage` (`settings_menu.go:595`) and `settingsMainEntries` (`settings_pages.go`) | the list pages set `PrivateOnly: true` in `settingsListPage` (`settings_list.go:584`) | WIRED | group menu shows only the note |
| Card No returns to the ask's page | `settingsCardRowsTo` (`settings_card.go`) | `renderSettingsListCard` (`settings_list.go`) | WIRED | M40 |

### Data-Flow Trace (Level 4)

| Artifact | Data variable | Source | Produces real data | Status |
|----------|---------------|--------|--------------------|--------|
| Blacklist list and action screen | entries, action line | `ListBlacklistEntriesFresh` (`fuku/db/blacklists/repository.go:119`) | yes (a query per draw) | FLOWING |
| Filters list and Main count | entries, `n/150` | `ListFilterEntriesFresh`, `CountFiltersContext` | yes | FLOWING |
| Notes list and admin tag | entries, `AdminOnly` | `ListNoteEntriesFresh` | yes | FLOWING |
| Typed add result | stored row | `applyBlacklistAdd`, `applyFilterAdd`, `applyNoteAdd` | yes (repository write, read back by the next draw) | FLOWING |

### Behavioral Spot-Checks (run by the verifier)

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| Focused Phase 19 set (8 tests: E2E, goldens, Legacy, codes, Main, privacy, log carry) | `source cienv.sh && go test -tags testtools -race -count=1 -v -run '^(TestSettingsPhase19EndToEnd\|TestLegacyNoteRequests\|TestLegacyFilterRequests\|TestLegacyBlacklistRequests\|TestSettingsPhase19Codes\|TestSettingsMainPhase19\|TestSettingsListGroupMenuShowsNothing\|TestSettingsListLogsCarryIDsOnly)$' ./fuku/modules` | exit 0, 8 of 8 PASS, `ok ... 9.385s` | PASS |
| All 141 test functions defined in the Phase 19 test files, across the seven affected packages (`./fuku/modules ./fuku/i18n ./fuku/db/blacklists ./fuku/db/filters ./fuku/db/notes ./fuku/db/migrations ./fuku/utils/content`), `-race`, CI environment | `go test -tags testtools -race -count=1 -v -run '<141 names>'` | exit 0; 892 PASS lines including subtests, 0 FAIL; top-level: 140 PASS, 1 SKIP (`TestBlacklistActionValuesOnPostgres`, the PostgreSQL guard, because this run did not set `FUKU_TEST_DATABASE`); `fuku/modules` 133.6 s | PASS |
| CI skip gate over the same 141 names in `go test -json` form | `go test -json ... \| go run ./scripts/check_test_results` | exit 0 (140 pass, 1 allowed PostgreSQL-only skip) | PASS |
| PostgreSQL migration chain on a throwaway PostgreSQL 16 cluster (empty database) | `FUKU_TEST_MIGRATION_CHAIN=true DATABASE_URL=... go test -run '^TestRepositoryMigrationChain$' ./fuku/db/migrations` | PASS; 63 migrations applied including `20261113120000_drop_legacy_blacklist_action_check.sql`; indexes verified 15, missing 0 | PASS |
| Phase 19 PostgreSQL proof on the same cluster | `FUKU_TEST_DATABASE=true DATABASE_URL=... go test -v -race -run '^(TestBlacklistActionValuesOnPostgres\|TestBlacklistActionMigrationStatements\|TestRaidDetectionCarryOver\|TestRulesMediaMigrationDefaults\|TestScheduledDeletion...\|TestMemberRequirement...\|TestCaptchaModeTurnstileConstraint)$' ./fuku/db/migrations` | all PASS (no skip); `pg_constraint` on `blacklists`: only `chk_blacklist_action` (eight values, NOT VALID) | PASS |
| Locale files | `make check-translations` | "All translations are present!" | PASS |
| Generated docs | `make check-docs` | "No drift detected" | PASS |
| Go vet on the changed packages | `go vet -tags testtools ./fuku/modules ./fuku/db/blacklists ./fuku/db/filters ./fuku/db/notes ./fuku/utils/content` | exit 0 | PASS |
| Formatting of changed Go files | `gofmt -l` over the changed `.go` files | prints nothing | PASS |
| Module files | `git diff --exit-code 8c0c827 -- go.mod go.sum` | exit 0 | PASS |
| Local lint fallback | `python3 .planning/phases/13-member-requirements/handoff/lint-local.py d93ec1d` | "no dupl, gocyclo or godox findings on lines added since d93ec1d", exit 0 | PASS (fallback) |
| golangci-lint | `golangci-lint run --new-from-rev=d93ec1d ...` | refused: "the Go language version (go1.25) used to build golangci-lint is lower than the targeted Go version (1.26.0)" | NOT RUN (tool limit, see warnings) |
| Whole `./fuku/modules` package | not run by the verifier (about 33 minutes; the brief said not to) | executor's `19-12-SUMMARY.md` reports `ok 1897.185 s`, 1841 top-level PASS, 0 FAIL, 0 SKIP | REPORTED ONLY |
| Locale key counts | grep of `^settings_` keys per file | 452 in each of the eight files, identical key sets | PASS |
| List action codes and orders | grep of `PagerCode`, `AskCode`, `YesCode`, `AddCode`, `CancelCode`, `Order` | 7 Blacklist, 5 Filters, 5 Notes codes; Orders 46, 47, 72 | PASS |

Commands the verifier ran against a throwaway database: a PostgreSQL 16 cluster was started under `/var/lib/postgresql/p19verify` on 127.0.0.1:5439, used for the two PostgreSQL runs above, stopped, and its data directory deleted. Nothing was written to the repository by that run.

## Golden and Commit Order (anti-cheating checks on the characterization pins)

| Golden | Created in | Later edits | Production change that followed |
|--------|------------|-------------|----------------------------------|
| `fuku/modules/blacklists_legacy_request_test.go` | `a8de41a` (294 insertions, test file only) | none | `f90a3ce` refactor, then `9616775`, `12ee6a4`, `3025b0f`, `76408de` |
| `fuku/modules/filters_legacy_request_test.go` | `c223cb2` (323 insertions, test file only) | none | `deef07b` refactor, then `f0dd7f7`, `4f794f1`, `9b24753` |
| `fuku/modules/notes_legacy_request_test.go` | `1f159fc` (322 insertions, test file only) | none | `6d3f326` refactor, then `2eeac8f`, `ed7988a`, `82ac0a5` |
| `fuku/utils/content/extract_characterization_test.go` | `e696952` (172 insertions, test file only) | none | `ca22c2e` extraction |

## Key Link Notes (things the plans asked for that I checked specifically)

- The apply functions named in the brief all exist: `applyBlacklistAdd`, `applyBlacklistRemove`, `applyBlacklistRemoveByID`, `applyBlacklistAction`, `applyFilterAdd`, `applyFilterRemove`, `applyFilterRemoveByID`, `applyNoteAdd`, `applyNoteRemove`, `applyNoteRemoveByID`. Three of them (`applyBlacklistRemove`, `applyFilterRemove`, `applyNoteRemove`) are called by their command only, by design: the page deletes by row ID, so the page's writes are the three `...RemoveByID` functions plus the add and action functions. Each page write is called by both its command and the page where the command has the same write (the add functions and `applyBlacklistAction`).
- The typed-reply door's list branch exists (`settings_input_door.go:183`).
- No list page registers `Hidden` (grep over the three page files is empty).

## Warnings (not gaps; need an owner decision or a note)

These are recorded for the owner. None of them is a must-have that fails.

1. **Test time exceeds the phase's own budget.** D-12 (`19-CONTEXT.md`) asked for about 40 s added to the `fuku/modules` run. The closing plan measured about 62 s added and the whole package at 1897 s (the per-package limit is 2400 s, the CI job limit 45 min). My run of the 141 names took 133.6 s under `-race`. This is within limits but close to them, and it is a deviation from the phase's own budget. Decision for the owner: accept, or ask for a trimming pass.
2. **Lint could not run.** The installed `golangci-lint` is built with Go 1.25 and refuses the Go 1.26.0 target, so `make lint` is not run here. The documented fallback (`lint-local.py`, `go vet`, `gofmt`) is clean for the changed lines. `gosec` and `govulncheck` were not run.
3. **The too-broad rule is narrower than D-07's wording (A19-2).** It refuses a blank word and a single code point outside Unicode So. D-07 mentioned "only wildcard or punctuation characters". For the substring matchers in use, only a single character matches almost every message, so the narrow rule is the one that matches the matchers, and it is recorded as an owner confirmation in `19-SPIKE.md`. Multi-character punctuation words such as `..` are accepted by design. Decision for the owner: accept A19-2 as recorded.
4. **Two owner-visible command refusals beyond the too-broad rule.** `/filter` (`4f794f1`) and `/save` (`ed7988a`) now refuse to add when their existence or count lookup fails (before this phase they added the entry). `/save` also refuses names with whitespace or a leading `#` (`2eeac8f`, A19-4). Each is in its own commit with its golden unchanged except for the intended refusals. These change what commands do and are listed in `19-SPIKE.md` as owner confirmations.
5. **Help text is stale for the three list pages.** `settings_help_msg` does not name the Blacklist, Filters or Notes pages (`19-SPIKE.md` A19-A17). Not a success criterion. Owner decision.
6. **Fake Telegram in the end-to-end proof.** T18 proves the code path through the real dispatcher with the settings environment's fake Telegram. Live group behaviour (a real supergroup, a real admin, a real client) is check 5 of `19-SPIKE.md` and is human.
7. **Phase 18 gap closure sits in the range.** `99f796e` (and `7a18e10`) are between Phase 18's close and Phase 19's work and changed `language.go` and `settings_logchannel.go`. They are not Phase 19 scope and were not re-verified here; the Phase 18 verification records them as its gap closure.

## Anti-Pattern Scan

| File group | Pattern | Result |
|------------|---------|--------|
| Changed production Go files | debt markers `TBD`, `FIXME`, `XXX`, `TODO`, `HACK`, `PLACEHOLDER`, `coming soon`, `not implemented` | none found |
| Changed production Go files | `return nil`/empty literals | only legitimate empty results on read or error paths (e.g. `fuku/db/filters/repository.go:41,51` and `fuku/db/notes/repository.go:54` return empty slices with the error) |
| List engine and page files | logging of a word, keyword, name, content or `Pending` value | none; `log` lines carry chat ID, user ID, page key and the error only (`TestSettingsListLogsCarryIDsOnly` ran green) |
| List engine and page files | `Hidden` on a list page | none (grep empty) |
| Test files added in the phase | `t.Skip` | two, both PostgreSQL-only guards in `fuku/db/migrations/blacklist_action_check_test.go` (the allowed pattern) |
| Migration | transaction keywords, `CREATE INDEX CONCURRENTLY`, `gorm.AutoMigrate` | none |

Severity of the scan: no BLOCKER, no WARNING from the scan itself.

## Human Verification Required

The items below are the owner's, from `19-SPIKE.md`. They are not satisfied by any test here.

1. **Live checks 1 to 9 of `19-SPIKE.md`** (C-16 on production, Main at 11 rows on a phone, a page of eight long entries, the two-step add in a real client, the effect of an added word, filter and note in a real group, the group menu showing nothing, the six changed commands in a real group, the Blacklist action screen with mixed and inert values, and the native read of the 67 new strings).
2. **Owner confirmations A19-1 to A19-11 and the flagged UI points A19-A1 to A19-A17**, especially A19-2, A19-3, A19-4, and the fail-closed lookups of `/filter` and `/save`.
3. **The help text** (`settings_help_msg`) and whether it should name the three pages (A19-A17).
4. **Check 15 of the SPIKE** (CI lint and the full run), which is for CI.

## Gaps Summary

No gap. Every success criterion and every plan truth is verified in code and in tests run by the verifier or in the 141-name run. The status is `human_needed` only because of the deferred live checks and owner decisions listed above.

---

_Verified: 2026-11-13T19:44:15Z_
_Verifier: Claude (gsd-verifier)_

## Post-verification change (orchestrator)

After this report was written, the security audit of Phase 19 (`19-SECURITY.md`, verdict SECURED, 59 of 59 threats closed) raised one low, non-blocking finding, SEC-19-01 (a very long Blacklist or Notes list was sorted with two `strings.ToLower` calls per comparison). The orchestrator fixed it in `899ab69` (`fuku/modules/settings_list.go`: `settingsListSort` computes each entry's key once; `settings_list_test.go` gains a test holding the order equal to `settingsListCompare`'s). The order every page draws is unchanged, so no truth above is affected; the focused list, page and end-to-end tests ran `ok` bare and under the CI environment, the lint stand-in exits 0, and `covered_digest` above was recomputed after the change with `gsd_run query verification.fingerprint` over the same 90 files. The whole `fuku/modules` package was not re-run for this change (CI runs it on the pushed head).
