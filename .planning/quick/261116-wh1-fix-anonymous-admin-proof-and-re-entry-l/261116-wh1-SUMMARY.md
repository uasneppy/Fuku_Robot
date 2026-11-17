---
phase: 261116-wh1-fix-anonymous-admin-proof-and-re-entry-l
plan: 01
subsystem: anonymous-admins, locks, connections, notes/filters overwrite card
tags: [anonymous-admin, locks, pm-connection, overwrite-card, audit-Q3]
status: complete
requires: []
provides:
  - "JSON proof cache (chat_status.StoreAnonAdminCommand / LoadAnonAdminCommand)"
  - "proof tap: name parsed like gotgbot, unsupported-command alert, one answer on every path"
  - "chat_status.AnonProofPrompted and chat_status.CheckAnonymousAdmin"
  - "anonymous-aware /lock and /unlock with re-entry; lock watcher exemptions; anonchannel = anonymous channels only"
  - "connected-chat admin checks for PM-connected commands"
  - "overwritePressTarget: shared authority step of the /save and /filter overwrite card"
affects: [fuku/utils/chat_status, fuku/modules (bot_updates, bans, purges, locks, warns, greetings, notes, filters, overwrite), locales, AGENTS.md, generated lock-types docs]
tech-stack:
  added: []
  patterns: ["per-update marker in ctx.Data", "authority decided before a cached token is consumed"]
key-files:
  created:
    - fuku/utils/chat_status/anon_admin_cache_test.go
    - fuku/modules/anon_admin_proof_test.go
    - fuku/modules/locks_anon_test.go
    - fuku/modules/pm_connection_admin_test.go
    - fuku/modules/overwrite_pm_test.go
  modified:
    - fuku/utils/chat_status/chat_status.go
    - fuku/utils/chat_status/access.go
    - fuku/utils/chat_status/permission_responder.go
    - fuku/utils/chat_status/access_test.go
    - fuku/modules/bot_updates.go
    - fuku/modules/bot_updates_test.go
    - fuku/modules/anonymous_admin_router.go
    - fuku/modules/bans.go
    - fuku/modules/purges.go
    - fuku/modules/locks.go
    - fuku/modules/warns.go
    - fuku/modules/greetings.go
    - fuku/modules/notes.go
    - fuku/modules/filters.go
    - fuku/modules/overwrite.go
    - scripts/generate_docs/parsers.go
    - docs/src/content/docs/api-reference/lock-types.md
    - locales/{en,es,fr,hi,id,pt,ru,uk}.yml
    - AGENTS.md
decisions:
  - "Proof cache stores the message as JSON bytes; any decode failure, the old msgpack format and a decoder panic read as an expired button (key and 20 s TTL unchanged)."
  - "An unregistered command name after the proof tap gets one alert (bot_updates_anon_unsupported_command) and runs nothing; kick, dkick, purgefrom and purgeto are registered for re-entry."
  - "Item 14 shipped: a sent proof prompt marks the update in ctx.Data and PermissionResponder stays silent for it; a failed prompt marks nothing."
  - "Lock watchers: anonymous admins exempt from every lock (comments included); the linked channel's posts exempt from every lock but anonchannel and comments, which keep their own rules."
  - "Overwrite card in a private chat: target chat only from the card, presser must be its creator (non-zero UserID) and a live admin of the card's group, all before the token is consumed; a refusal answers without editing the card."
metrics:
  duration: "about 45 minutes"
  completed: 2026-11-17
actuals:
  tokens: 26000
  tasks: 3
  commits: 7
plan_head_before: ab7554855909b6a6b9f56969f1d6ca430e12e2d2
plan_head_after: 5fab71f
---

# Phase 261116-wh1 Plan 01: Anonymous-admin proof, re-entry, locks and PM connections Summary

The anonymous-admin proof cache now survives replies to forwards and pins (JSON instead of msgpack), the proof tap runs kick/dkick/purgefrom/purgeto, refuses every other unregistered command with one alert and always answers once, the proof prompt is the only reply, /lock and /unlock work for anonymous admins, the lock watchers spare anonymous admins and the linked channel, and PM-connected admin commands and the /save and /filter overwrite cards are judged in the connected group.

## Per audit item (section Q3, items 12 to 19)

| Item | Status | Commit | Tests added |
|------|--------|--------|-------------|
| 12 proof cache cannot decode a reply to a forward or pin | fixed | ef8a16e | `TestAnonAdminCommandRoundTrip`, `TestAnonAdminCommandOldFormatReadsAsExpired` (chat_status), `TestAnonAdminProofCachesInterfaceFields` (real dispatcher) |
| 13 prompting commands with no re-entry; name parsing; answer once | fixed | f4d1c84 | `TestAnonAdminReentryKickAndPurge`, `TestAnonAdminTapUnsupportedCommand`, `TestAnonAdminTapCommandNameForms`, `TestAnonAdminTapAnswersOnce` (all real dispatcher) |
| 14 double reply at the proof prompt | fixed | 4dd8c57 | `TestAnonProofPromptMarker` (chat_status), `TestAnonAdminProofSingleReply` (real dispatcher) |
| 15 /lock and /unlock refuse anonymous admins | fixed | 9fe2843 | `TestLockCommandsAnonymousAdmin` (real dispatcher, AnonAdmin on and off, non-admin tap, plain member, real admin) |
| 16 lock watchers delete anonymous admins' and the linked channel's posts | fixed | ae1dfb3 | `TestLockWatchersExemptAnonymousAdminAndLinkedChannel` (with member and anonymous-channel controls) |
| 17 /lock anonchannel also matched the linked channel | fixed | ae1dfb3 | `TestAnonChannelLockIgnoresLinkedChannel` |
| 18 PM-connected admin commands refuse a real admin | fixed (locks lines in 9fe2843, the rest in a76f173) | 9fe2843, a76f173 | `TestConnectedAdminCommandsInPrivateChat` (lock/unlock, setwarnmode, setwarnlimit, welcome off, notes, get noformat, plain-member and in-group controls) |
| 19 overwrite card can never succeed through a PM connection | fixed | 5fab71f | `TestOverwriteCardThroughPrivateConnection` (notes and filters: refusals keep the card, No consumes, foreign card in a group cancelled, Yes once), `TestOverwriteCardThroughPrivateConnectionEndToEnd` (real dispatcher, /save and /filter in PM, press in PM) |

Every new test was seen failing before its production change (item 12, 13, 14 modules tests and item 15 to 17 tests by running them against the unfixed code; the item 18 and 19 tests by temporarily restoring the unfixed files with `git checkout -- <file>` and re-applying the fix; the two chat_status tests failed to compile before the new functions existed).

## Pinned expectations changed on purpose

- No `TestLegacy*Requests` golden changed or was re-recorded; `TestLegacyLockRequests`, `TestLegacyNoteRequests`, `TestLegacyFilterRequests`, `TestLegacyWarnSettingsRequests` and the rest pass unedited. `reports.go` is untouched (Q4 item 27).
- `fuku/modules/bot_updates_test.go`: three existing tests (`TestVerifyAnonymousAdminRestoresCachedMessageAndDeletesButton`, `TestVerifyAnonymousAdminRefusesChatMismatch`, the "cached command delete failure" subtest) now seed the proof cache through `chat_status.StoreAnonAdminCommand` instead of the old msgpack write, as the plan required (item 12).
- `fuku/utils/chat_status/access_test.go`: four direct calls of the unexported `checkAnonAdmin` pass a nil context for its new first parameter (item 14). Expectations unchanged.
- Generated docs: the `anonchannel` description drops "and linked channel posts" (two lines of `api-reference/lock-types.md`, from `scripts/generate_docs/parsers.go`).

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Correctness] Item 15 refusal path also covers a failed proof prompt**
- **Found during:** Task 2
- **Issue:** the plan said the anonymous path returns with no further reply after the prompt; if the prompt itself fails to send the admin would get nothing.
- **Fix:** `lockCommandAuthorized` always calls the `PermissionResponder` on refusal; item 14's marker keeps it silent when the prompt was sent, so a failed prompt still produces the refusal.
- **Files modified:** fuku/modules/locks.go
- **Commit:** 9fe2843

**2. [Rule 2 - Correctness] Stale overwrite press in a private chat answers without editing the card**
- **Found during:** Task 3
- **Issue:** editing the card message on a refusal would remove its buttons while the card stays in the cache for its holder.
- **Fix:** a stale private press (missing card, not the presser's, UserID 0, no group) answers the expired or cancelled text only; nothing is edited or consumed.
- **Files modified:** fuku/modules/notes.go, fuku/modules/filters.go
- **Commit:** 5fab71f

**3. [Rule 3 - Blocking] gofmt import grouping in overwrite.go**
- overwrite.go had a pre-existing misordered import block; touching the file required gofmt, so the block was regrouped (stdlib, third-party, internal). No behaviour change.

Other notes: the AGENTS.md lines for item 14 were added by amending its own commit (not pushed) so the rule change and the code change share one commit. Item 14's commit was kept: no golden or existing permission test changed.

## Known Stubs

None.

## Threat Flags

None beyond the plan's threat register: no new endpoint, auth path or schema; the new alert is a fixed locale string with no command text or ID.

## Verification

- `CGO_ENABLED=0 go build ./...` passes; `go vet -tags testtools ./...` passes.
- `make check-translations` passes (new key `bot_updates_anon_unsupported_command` in all 8 locales, each at most 160 code points, no markup or placeholder); `make check-docs` reports no drift.
- `go test -tags testtools -race -count=1 ./fuku/utils/chat_status ./fuku/utils/helpers ./fuku/i18n` passes.
- One `fuku/modules` run of the union of every test name the three tasks list plus neighbouring families (anonymous admin, locks, notes, filters, warns, greetings, purges, bans, connections, callbacks, command deletion, settings anonymous/gate/phase 18 e2e, `TestLegacy.*Requests`, `TestNoRawNewCommand`): 252 top-level tests, all `--- PASS`, including every new test name.
- `grep -c 'RequireUserAdmin(b, ctx, nil' fuku/modules/locks.go` is 0; `grep -c 'RequireUserAdmin(bot, ctx, nil' fuku/modules/greetings.go` is 0.

## Self-Check: PASSED

- Created files exist: fuku/utils/chat_status/anon_admin_cache_test.go, fuku/modules/anon_admin_proof_test.go, fuku/modules/locks_anon_test.go, fuku/modules/pm_connection_admin_test.go, fuku/modules/overwrite_pm_test.go.
- Commits exist on the branch: ef8a16e, f4d1c84, 4dd8c57, 9fe2843, ae1dfb3, a76f173, 5fab71f (7 commits since ab75548).
