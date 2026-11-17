---
phase: 261116-wh2-fix-raw-placeholder-strings-note-deep-li
plan: 01
subsystem: i18n, notes, misc, reports, disabling
tags: [audit-q4, locales, deep-links, reports, disabling]
status: complete
requires: []
provides:
  - "{name} placeholders for the backup and captcha strings that printed raw verbs"
  - "noteid_<chat>_<row id> private note links with a strict deep-link handler"
  - "User-Agent on every fetchJSON request"
  - "/reports in a private chat with and without a connection"
  - "/disable offers only names a handler checks; /blocklists honours its own name"
affects: [locales/*.yml, AGENTS.md]
tech-stack:
  added: []
  patterns:
    - "Locale strings filled with TranslationParams use {name} placeholders equal to the param names"
    - "Deep links name rows by ID, never by user text"
key-files:
  created:
    - fuku/i18n/raw_text_fixes_test.go
    - fuku/utils/formatting/help_locales_test.go
    - fuku/modules/backup_caption_test.go
    - fuku/modules/notes_deeplink_test.go
    - fuku/modules/reports_private_connection_test.go
    - fuku/modules/disable_noops_test.go
  modified:
    - locales/en.yml, es.yml, fr.yml, hi.yml, id.yml, pt.yml, ru.yml, uk.yml
    - fuku/modules/backup.go
    - fuku/db/notes/repository.go
    - fuku/db/notes/repository_test.go
    - fuku/modules/notes.go
    - fuku/modules/misc_extras.go
    - fuku/modules/misc_extras_test.go
    - fuku/modules/reports.go
    - fuku/modules/reports_legacy_request_test.go
    - fuku/modules/reports_command_test.go
    - fuku/modules/blacklists.go
    - fuku/modules/antiraid.go
    - AGENTS.md
decisions:
  - "Item 27: the connected private /reports passes the connected chat explicitly to RequireUserAdmin and RequireBotAdmin; with a nil chat those predicates judge update.Message.Chat (the private chat), which is why a connected admin was always refused"
  - "Item 28: /blocklists checks its own disabled name and then blacklists, so disabling blacklists still silences the alias"
  - "Item 25: a note list entry cached by the previous build (ID 0) falls back to the old note_<chat>_<name> link until notes_list is dropped"
metrics:
  duration: 28min
  completed: 2026-11-17
actuals:
  tokens: 35700
  tasks: 3
  commits: 9
plan_head_before: e5903b8774f857fe8b28b9b7a4c87606b09e7659
plan_head_after: 95d8ff420583e92b3cdf470c0a45f75e23f8b924
---

# Quick 261116-wh2: Audit Q4 strings, note links and small commands Summary

The nine defects of audit section Q4 (items 20 to 28) are fixed, each in its own `fix:` commit with a test that was seen failing first. Placeholders render in all 8 languages, private note links use row IDs, `/wiki` sends a User-Agent, `/reports` works from a private chat, and `/disable` only offers names a handler checks.

## Per audit item

| Item | Result | Commit | Tests added |
|------|--------|--------|-------------|
| 20 raw `%s`/`%d` in backup and captcha texts | fixed: the 8 keys use `{chat}`, `{modules}`, `{time}`, `{list}`, `{user}`, `{count}`, `{types}`, `{attempts}` in all 8 locales; the export caption HTML-escapes the chat title | 217c343 | `TestRawPlaceholderKeysRender` (fuku/i18n), `TestBuildExportCaptionRealLocales` (fuku/modules) |
| 21 missing `greetings_*_not_configured` keys | fixed: added to all 8 locales | fe2d13a | `TestGreetingsNotConfiguredKeysExist` |
| 22 `aispam_help_msg` mixes HTML and Markdown | fixed: heading is `<b>`, placeholder is `<code>&lt;on/off&gt;</code>` in all 8 locales | 8024c35 | `TestAISpamHelpRendersWithoutRawMarkdown` (fuku/utils/formatting) |
| 23 `/export` listed under user commands | fixed: the heading is an admin heading in all 8 locales | 577bacb | `TestBackupHelpExportIsAdminCommand` |
| 24 captcha mute "permanent" | fixed: says 24 hours in all 8 locales | b420d89 | `TestCaptchaMutedActionStatesTheDuration` |
| 25 private note deep links carry the raw name | fixed: `noteDeepLinkURL` builds `noteid_<chat>_<row id>` at all four sites; new `noteIDDeepLinkHandler` (strict parse before any Telegram call, membership check, `GetNoteEntryFresh(chat, id)`, shared `sendDeepLinkNote` with the admin-only check); old `note_` handler kept; names (and the chat title of the private `/notes` list) escaped in the HTML lists; `notes.GetNotesListEntriesContext` returns row IDs | ea4d1b2 | `TestNoteDeepLinkBuildersUseRowID`, `TestNoteIDDeepLinkHandler` (fuku/modules), `TestGetNotesListEntriesContext` (fuku/db/notes) |
| 26 `/wiki` always "not found" | fixed: `fetchJSON` sends `Xenobot-72/<BotVersion> (Telegram bot)` (or `Xenobot-72 (Telegram bot)` with no version) | f759b23 | `TestFetchJSONSendsUserAgent` |
| 27 `/reports` in a private chat | fixed: no connection = the user's own preference with no connection or admin check; a connection = the connected group's switch after `IsUserConnected(true, true)` and the admin and bot-admin checks on the connected chat | bc91d41 | `TestReportsPrivateConnection` (three cases through the real dispatcher) |
| 28 `/disable` accepts unchecked names | fixed: the retired raid pointer, `/export` and `/import` are no longer disableable; `/blocklists` checks `blocklists` then `blacklists` | 95d8ff4 | `TestDisableRefusesNamesThatHaveNoCheck`, `TestBlocklistsAliasHonoursItsOwnName` |

## Pinned expectations changed on purpose

- `TestLegacyReportsPrivateRequests` (`fuku/modules/reports_legacy_request_test.go`): its five cases now run with no stored connection (the connect, `getChat` response and disconnect setup were replaced by clearing the connection). Every golden constant is byte-for-byte unchanged and its last assertion still proves no group row is written. Stated in commit bc91d41.
- `TestReportsSettingsPrivateAndValidationBranches` (`fuku/modules/reports_command_test.go`): its connection setup was removed so its personal-preference assertions stay true.
- No other golden was touched. `TestLegacyReportsRequests`, `TestLegacyNoteRequests` and `TestLegacyBlacklistRequests` pass unchanged.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Connected private /reports judged the admin checks in the private chat**
- **Found during:** Task 3, item 27
- **Issue:** after taking the group branch for a connected private chat, `RequireUserAdmin(b, ctx, nil, ...)` and `RequireBotAdmin(b, ctx, nil)` resolve a nil chat to `update.Message.Chat` (the private chat), not `ctx.EffectiveChat`, so every connected admin was refused.
- **Fix:** pass the connected chat explicitly to both predicates (in a group chat it is the same chat).
- **Files modified:** fuku/modules/reports.go
- **Commit:** bc91d41

**2. [Rule 2 - Security] Chat title escaped in the private /notes list header**
- **Found during:** Task 2, item 25
- **Issue:** `notes_list_for_chat` spliced the raw chat title into the same HTML message whose note names the plan escapes.
- **Fix:** `html.EscapeString(chat.Title)` in that one `fmt.Sprintf`.
- **Commit:** ea4d1b2

**3. Test environment notes (no production change):** the plan's suggestion to reuse the existing private-list test pattern did not exercise the private `/notes` list (it has no stored connection, so `IsUserConnected` refuses), so `TestNoteDeepLinkBuildersUseRowID` connects a fresh user; the note handler test installs the real locales so its text comparisons are not empty-string matches; `TestFetchJSONSendsUserAgent` sets `config.AppConfig.BotVersion` (empty in tests) and restores it.

- `make generate-docs` produced no change (the generator does not show the Disableable flag), so no docs page was committed.

## Seen but not fixed (outside Q4, reported)

1. `greetings_welcome_status` / `greetings_goodbye_status` are filled with `%s` over booleans (`%!s(bool=true)`), from the plan.
2. `interpolateParams` runs the legacy verb pass over already substituted text, so user text holding `%d` can be formatted when a key mixes braces and common param names (cosmetic), from the plan.
3. The group notes list splices `<code>#%s</code>` with the raw note name (not one of the two lists the plan named).
4. `/reports block|unblock|showblocklist` still answer "group only" in a connected private chat.
5. `notes_click_for_private` (watcher and `/get`) splices the raw note name into a Markdown message.

## Known Stubs

None.

## Threat Flags

None: the new `noteid_` deep link is in the plan's threat model (T-261116-wh2-01/02) and mitigated as planned.

## Verification

- `CGO_ENABLED=0 go build ./...`, `go vet -tags testtools ./...`, `make check-translations`, `make check-docs`: clean.
- Targeted tests green: the four i18n tests, `./fuku/utils/formatting`, `TestGetNotesListEntriesContext`, and in `fuku/modules` every new test plus `TestBuildExportCaption`, `TestHandleDeepLink*`, `TestNotesWatcher*`, `TestGetNote*`, `TestLegacyNoteRequests`, `TestGitHubWikiUD*`, `TestLegacyReportsRequests`, `TestLegacyReportsPrivateRequests`, `TestReportsSettings*`, `TestDisable*`, `TestLegacyBlacklistRequests`, `TestCaptcha*`, `TestBackup*` and `-run Note`.
- Grep gates: `start=note_` appears once in notes.go (the ID-0 fallback); antiraid.go has no `Disableable: true`; backup.go registers neither `export` nor `import` as disableable. No migration, no go.mod/go.sum change.

## Self-Check: PASSED

- All six created test files and the modified files exist; commits 217c343, fe2d13a, 8024c35, 577bacb, b420d89, ea4d1b2, f759b23, bc91d41, 95d8ff4 are ancestors of HEAD.
