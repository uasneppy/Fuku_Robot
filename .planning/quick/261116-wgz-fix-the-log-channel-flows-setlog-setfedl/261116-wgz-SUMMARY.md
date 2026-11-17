---
phase: 261116-wgz-fix-the-log-channel-flows-setlog-setfedl
plan: 01
subsystem: log channels, federations
tags: [logchannels, federations, setlog, setfedlog, settings-menu, audit-261116-q1]
status: complete
requires: []
provides:
  - "/setlog two-step channel link through the real dispatcher"
  - "/setfedlog channel flow (owner's forward binds the channel)"
  - "typed /setfedlog in a group requires a group admin (item 2b)"
  - "four log categories (settings, admin, user, reports)"
affects: [fuku/modules/logchannels.go, fuku/modules/federations.go, fuku/db/logchannels, settings page o]
tech-stack:
  added: []
  patterns:
    - "shared one-hour channel-post marker helpers: storePendingMarker, peekPendingMarker, consumePendingMarker, channelForwardOrigin"
    - "raw helpers.NewBotCommand(...).SetAllowChannel(true) for sender-less channel posts"
key-files:
  created:
    - fuku/modules/logchannels_flow_test.go
    - fuku/modules/federations_setfedlog_flow_test.go
    - fuku/modules/logchannels_categories_test.go
    - fuku/db/backup/logchannels_old_export_test.go
  modified:
    - fuku/modules/logchannels.go
    - fuku/modules/federations.go
    - fuku/modules/settings_logchannel.go
    - fuku/db/logchannels/repository.go
    - fuku/modules/federations_command_test.go
    - fuku/modules/logchannels_legacy_request_test.go
    - fuku/modules/logchannels_apply_test.go
    - fuku/modules/settings_logchannel_test.go
    - fuku/modules/settings_logchannel_screens_test.go
    - fuku/modules/settings_logchannel_audit_test.go
    - fuku/modules/settings_phase18_test.go
    - fuku/db/logchannels/repository_test.go
    - fuku/i18n/settings_locale_test.go
    - locales/{en,es,fr,hi,id,pt,ru,uk}.yml
    - README.md
    - README.uk.md
    - docs/src/content/docs/commands/federations/index.md
    - docs/src/content/docs/commands/logchannels/index.md
    - AGENTS.md
decisions:
  - "The group-0 setlog command binds a forwarded marked post itself (shared bindSetLogForward with the group-11 watcher); the admin check runs before the marker is consumed"
  - "/setfedlog forward step is part of the command; a channel-origin forward never reaches the group branch and binds the forward origin's channel"
  - "Typed /setfedlog in a group requires chat_status.RequireUserAdmin of that group before fed ownership (item 2b)"
  - "Dead categories removed from accepted names only; cat_automated/cat_other columns and JSON keys kept, no migration"
metrics:
  duration: "about 20 minutes"
  completed: 2026-11-17
actuals:
  tokens: 31000
  tasks: 3
  commits: 3
plan_head_before: 832ad9307f16cd927af5232b2a9d3b18470ceda4
plan_head_after: dfbf97714a07c37c5050f5ef6ededd9893c97de3
---

# Quick task 261116-wgz: log channel flows (/setlog, /setfedlog, dead categories) Summary

`/setlog` and the `/setfedlog` channel flow now work end to end through the real dispatcher with a
single-use one-hour marker and an authority check before the marker is consumed. Typed `/setfedlog`
in a group needs a group admin. `/log`, `/nolog`, the settings page and every help text offer exactly
four log categories.

## Per audit item (section Q1 plus 2b)

| Item | Status | Commit | Tests |
|------|--------|--------|-------|
| 1a `setlog` drops channel posts (no `SetAllowChannel`) | fixed | 956aa18 | `TestSetLogFlowThroughDispatcher` (memory store and miniredis) |
| 1b `requireUser` stops the sender-less channel post | fixed: the channel branch runs before any user lookup | 956aa18 | same |
| 1c group-0 `setlog` shadows the forward, watcher never ran | fixed: the command binds the forward itself through `bindSetLogForward`, shared with the group-11 watcher | 956aa18 | same (admin forward binds; member forward binds nothing and keeps the marker; unmarked forward and replay bind nothing; typed `/setlog` answers `logs_need_channel`; nil marshaler fails closed) |
| 2 `/setfedlog <fed id>` channel branch unreachable | fixed with the two-step proof: post stores `fuku:setfedlog:<channel>:<message>` (value fed ID, 1 h, single use); the fed owner's forward in private or in a group sets the log to the CHANNEL. Two locale keys in all 8 files; help text no longer lists `/setfedlog` under private use; README, docs, AGENTS | 8fbd458 | `TestSetFedLogFlowThroughDispatcher` (both stores), `TestSetFedLogLocaleKeys` |
| 2b typed `/setfedlog` in a group checked only fed ownership | fixed: `chat_status.RequireUserAdmin` of that group with the usual refusal (`chat_status_user_admin_cmd_error`) runs before the fed-owner check; the forward step (target is the channel) keeps only the fed-owner check | 8fbd458 | `TestSetFedLogFlowThroughDispatcher` subtest "typed setfedlog by an owner who is a plain member is refused" (nothing stored) and "typed setfedlog by an admin owner sets the group" |
| 3 dead categories `automated` and `other` | fixed: removed from `AllCategories`/`IsValidCategory`, the page table (4 cells, rows {2,2,2}), the two `settings_log_cat_` keys, `logs_categories` and `logchannels_help_msg` in 8 files, README, docs, AGENTS. Columns and JSON keys kept, no migration | dfbf977 | `TestLogCategoriesAcceptOnlyLiveNames` (dispatcher), `TestLogCategoryLocaleTexts`, `TestImportLogChannelsOldExportKeepsAllSixFlags`, extended `TestLogChannelSetCategoryAndUnset` / `TestLogChannelCategoryHelpers` |

Every new dispatcher test was run and recorded failing before the production change (RED), then passing.
A notable RED finding for item 2: at HEAD the owner's forward of `/setfedlog <id>` into a group set
THE GROUP as the federation log, not the channel.

## Pinned expectations changed on purpose

- `TestLegacyLogCategoryRequests`, case `several_off` only: `/nolog admin user other` now answers and
  audits `admin, user` and its flags are `100111` (the `other` column stays on). `logGoldenFlags`
  now reads the six stored columns straight from the row, so every other case is byte-identical.
- `TestImportFBansNeedsFileAndSetFedLogInChannel`: a direct channel `/setfedlog` call now expects
  `LogChatID` 0 (the post only records a marker).
- `TestFedSubscribeLogExportAndDelete`: its owner is listed as a group administrator in the fake,
  which typed `/setfedlog` in a group now requires (2b).
- `TestSettingsLogChannelAuditSurvivesFailedReadBack`: expects the six-column string `110111` from
  `logGoldenFlags` (it used to derive the string from `AllCategories`).
- Page shape tests (`settings_logchannel_test.go`, `settings_logchannel_screens_test.go`): rows
  `[2 2 2]`, four cells plus Back and Close; `settings_phase18_test.go`: Log page codes `s a u r`;
  `logchannels_apply_test.go`: baseline uses `reports` instead of `other`; `fuku/i18n/settings_locale_test.go`:
  two limits rows and two category names removed.

## Deviations from Plan

1. [Rule 1 - Bug] `settings_logchannel_audit_test.go` (not in the plan's file list) compared
   `logGoldenFlags` output with a string built from `AllCategories`; after the golden helper moved
   to six raw columns it needed the six-column literal. Commit dfbf977.
2. Help line wording: the plan's "/setfedlog typed in a group" line was written as
   `× /setfedlog: In a group, make that group the federation log (you must be an admin there).`
   because the docs generator took "/setfedlog in a group" as the command name.
3. AGENTS.md: the count of `settings_` keys was already stale before this task (452 written,
   466 in en.yml at HEAD); it is now written as the real count, 464, instead of "452 minus 2".
4. Locale strings for the two new keys were translated into each file's language (es, fr, hi, id,
   pt, ru included), per the brief's rule, although the neighbouring `feds_` strings in those six
   files are English. The help-text edits in those six `|` blocks stay English as the plan says.

## Threat Flags

None beyond the plan's threat model. T-wgz-05 (another group's admin forwarding a public channel's
marked `/setlog` post within the hour) is the accepted stated limit, recorded in AGENTS.md.

## Verification

- `CGO_ENABLED=0 go build ./...`, `go vet -tags testtools ./...`: clean.
- `make check-translations`, `make check-docs`: pass.
- Targeted: Task 1, 2 and 3 lists of the plan, all `Fed|Federation|Fban` tests, all
  `TestLegacy.*Requests` goldens, `./fuku/db/logchannels`, `./fuku/db/backup`, `./fuku/i18n`: pass.
- Grep gate (dead identifiers in fuku and locales): 0; AGENTS gates (`s a u m r o`, `65 states`,
  `{2,2,2,2}`): absent. `make test` was not run (per brief).

## Self-Check: PASSED

- Created files exist: logchannels_flow_test.go, federations_setfedlog_flow_test.go,
  logchannels_categories_test.go, logchannels_old_export_test.go.
- Commits 956aa18, 8fbd458, dfbf977 are ancestors of HEAD; none pushed.
