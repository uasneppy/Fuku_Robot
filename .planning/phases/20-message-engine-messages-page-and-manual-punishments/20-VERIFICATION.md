---
phase: 20-message-engine-messages-page-and-manual-punishments
verified: 2026-11-19T03:06:51Z
status: human_needed
score: "16/16 truths verified in code and by tests the verifier ran (5 ROADMAP success criteria, 11 requirements); 0 present-but-behavior-unverified; the live Telegram checks below are left to the owner"
behavior_unverified: 0
overrides_applied: 0
fingerprint_note: "covered_files and covered_digest are not written: `gsd-tools.cjs verification.fingerprint` fails in this checkout with MODULE_NOT_FOUND './vendor/re2js.cjs' (.claude/gsd-core/bin/lib/pattern.cjs), and the digest must never be hand-written. The same failure blocks the `verification.status` self-check; the status value above is one of passed | gaps_found | human_needed."
deferred:
  - truth: "MSG-01: a message can also be stored for the PM destination"
    addressed_in: "Phase 21"
    evidence: "Phase 21 PMSG-01: 'Each punishment event can also send a message to the punished person in private, with its own text, media and buttons'. eventmsg.Dests holds only `group`; the key (chat_id, event, dest) and ValidDest are ready for `pm` with no migration."
  - truth: "MSG-03: {admin} reads 'an admin' for an automatic action"
    addressed_in: "Phase 22"
    evidence: "Phase 22 adds the automatic events (warn, warn limit, flood, blacklist, forward blocklist). The engine already reads a nil actor as eventmsg_admin_anon (event_message.go:109)."
  - truth: "MSG-02: {action}, {count}, {limit} and {window} carry a value"
    addressed_in: "Phase 22"
    evidence: "These belong to the automatic events; for the seven manual events they render empty (event_message.go:91-103), which MSG-02 allows."
  - truth: "MSG-05: the automatic, lockdown and requirement families have rows"
    addressed_in: "Phases 22, 23, 24"
    evidence: "settingsMsgFamilies is table-driven; a family with no events is not drawn (settings_messages.go:214-222, 591-594)."
advisory:
  - finding: "TestChatEventMessagesMigrationDefaults (PostgreSQL only) skipped locally: no PostgreSQL server here"
    category: other
    reason: "It is in the Makefile's test-postgres-integrity -run list (Makefile:76) with ./fuku/db/eventmsg in its packages (Makefile:79), so CI runs it. The database-free half TestChatEventMessagesMigrationStatements passed here."
    evidence_status: "none provided"
  - finding: "eventmsg comments say EventTimedBan/EventTimedMute are also '/ban (or /mute) with a duration'"
    category: other
    reason: "The per-group /ban and /mute parse no duration (moderationBan and moderationMute have no ExtractTime), so a duration word stays part of the reason, as before Phase 20. The D-16 sentence is moot, not broken; a doc fix in fuku/db/eventmsg/events.go:21,27 would remove the confusion."
    evidence_status: "code read"
human_verification:
  - test: "In a real supergroup, set a custom Ban message from the private /settings menu (bold text with {mention}, {reason}, {admin}, one photo, one URL button), then run /ban @member spam & co"
    expected: "The ban happens first; the group gets one photo reply to the command, caption filled and escaped, the Unban (admins only) button on the first row and the URL button under it; the message disappears on the group's Bot messages > Manual punishments timer"
    why_human: "The fake client proves the request bytes; only a live client shows how Telegram renders the caption, the keyboard and the deletion"
  - test: "Store a media item, then make Telegram refuse it on a live account (for example a text over 1024 characters filled with a long reason, or a file ID from another bot)"
    expected: "The member is punished, the custom message is refused and the default text is posted once in the group's language"
    why_human: "The refusal classification rests on Telegram's real 4xx answers; tests script a 400"
  - test: "As an anonymous admin in a group with AnonAdmin off, send /ban (custom Ban text containing {admin}) and tap the proof button as a named admin; repeat with AnonAdmin on"
    expected: "Both times {admin} reads 'адмін' / 'an admin' in the group's language and no group message names the tapper"
    why_human: "The proof tap and Telegram's sender_chat on a live anonymous admin cannot be fully reproduced by the fake"
  - test: "Turn Silent on for Kick, run /kick and /dkick; turn it off and run /sban, /skick, /smute"
    expected: "The members are removed or muted; nothing is posted for the silent Kick; the s-variants stay silent whatever is stored"
    why_human: "Live confirmation of the end-to-end behaviour in a real group"
  - test: "On a phone, open Messages from the private menu: hub, Admin actions list, one event screen with every part set, Preview of each part and Full preview; then press the Messages cell in a group menu"
    expected: "Screens are readable and fit (event screen at most 7 rows; Main is now 13 rows for the owner, 12 for an admin); previews arrive only in your private chat, with no Unban button; the group menu shows only the note and an Open in private button that works"
    why_human: "Layout and phone readability are visual; tests hold bytes and lengths only"
  - test: "Set the group's timezone (for example Asia/Tokyo), give Timed mute a text with {duration} and {until}, run /tmute @member 30m"
    expected: "{duration} reads '30 minutes' and {until} the end time in the group's zone with its label, e.g. '19.11.2026 15:04 (UTC+9)'"
    why_human: "Live confirmation of the time formatting a member sees"
---

# Phase 20: Message Engine, Messages Page and Manual Punishments Verification Report

**Phase Goal:** As the owner, I can set what the bot says in a group when an admin bans, kicks, mutes, unbans or unmutes someone, from that group's settings in the bot's private chat. A group that sets nothing sees no change.
**Verified:** 2026-11-19T03:06:51Z
**Status:** human_needed
**Re-verification:** No, initial verification

Every truth was checked against the code and against tests the verifier ran itself. No gap was found. The status is `human_needed` only because of the live Telegram checks listed at the end.

## Tests run by the verifier

| Command | Result |
| ------- | ------ |
| `go test -tags testtools -race -count=1 -run '^(TestLegacyModerationRequests\|TestEventMessage.*\|TestRenderEventText\|TestMessagesEndToEnd\|TestSettingsMessages.*\|TestSettingsAuthorityMessages.*\|TestSettingsDriftMessages.*)$' ./fuku/modules` | ok, 30 top-level tests PASS (37 s) |
| `go test -tags testtools -race -count=1 -run '^(TestSettingsMainGrid\|TestSettingsFwdBlockMainShape\|TestSettingsStaffPrivacy\|TestBotManualPunishmentReplies\|TestBotManualRepliesNotCovered\|TestSettingsPrivateOnlyPage)$' ./fuku/modules` | ok (40 s) |
| `go test -tags testtools -race ./fuku/db/eventmsg ./fuku/db/migrations ./fuku/db/backup -run 'TestEventMessages\|TestChatEventMessagesMigration\|TestEventMessagesStayOutOfBackup'` | ok; `TestChatEventMessagesMigrationDefaults` SKIP (PostgreSQL only, in CI's integrity list) |
| `go test -tags testtools ./fuku/db/cache -run TestSkipLocalMatchesFreshnessCriticalKeys` | ok (`event_messages` in skipLocal, `event_messagesx` not) |
| `go test -tags testtools ./fuku/i18n` | ok |
| `make check-translations` | all keys present in 8 locales |
| `CGO_ENABLED=0 go build ./...` | ok |

The whole `fuku/modules` package was not re-run (plan 20-11 ran it, 35 min, ok).

## Goal Achievement

### ROADMAP success criteria

| # | Truth | Status | Evidence (code) | Proving test |
| - | ----- | ------ | --------------- | ------------ |
| SC1 | In the private menu the owner opens Messages and sees the manual family with each event's state; a group menu shows only the "open in private" note | ✓ VERIFIED | `settings_messages.go:195-206` (page `h`, `PrivateOnly`, `PrivateNoteKey: settings_msg_group_note`, `Hidden` in a Staff Group); `settingsMsgStateOf` 310-321 (Default / Custom / Silent, Silent wins); hub 586-605; family list 609-624; Main line 358-367 | `TestSettingsMessagesOpens`, `TestSettingsMessagesMainState`, `TestSettingsMessagesGroupMenu`, `TestSettingsMessagesStaffGroup` (settings_messages_test.go:69,155,190,236); e2e `owner sets a ban message`, `group menu`, `staff group` (messages_e2e_test.go:355) |
| SC2 | Each of the seven events opens the editor: text, one media and URL buttons set by private messages, the editor's limits and refusals, Preview of each part and a Full preview through the real send path | ✓ VERIFIED | one editor definition per event, `registerSettingsEditDef(settingsMsgEditDef(...))` settings_messages.go:207-211, 246-258; actions resolved by `k` with the private field required 162-170, 263-272; typed reply judged in `settings_input_door.go:229-280` (same `settingsEditCheck`, which has no event-kind exception, so 4096 / 1024 / http(s) / keyboard limits apply, with `ReservedButtons` for the Unban/Unmute row); Write hook 502-517; Full preview built by `composeEventMessage`, the send path's own composer, and sent with `media.SendGreetingMessage` to the presser only (541-570) | `TestSettingsMessagesSet`, `TestSettingsMessagesRemove`, `TestSettingsMessagesTyped` (stored text, photo with caption ignored, button line, refusals: button line in text, `javascript:` link, 100 buttons with the reserved row, slash text, media while the text is over 1024, album), `TestSettingsMessagesPreview`; e2e `previews` (Full preview to the owner only, no Unban row, nothing in the group) |
| SC3 | After a custom text is set, the next `/ban`, `/tban`, `/kick`, `/mute`, `/tmute`, `/unban`, `/unmute` posts it with `{first}`, `{mention}`, `{reason}`, `{admin}`, `{duration}`, `{until}` filled; empty placeholders render nothing; user values escaped; `{admin}` is "an admin" for an anonymous admin; silent and delete-first variants as before | ✓ VERIFIED | all seven sites call `replyEvent`: ban/dban bans.go:180-195, tban 117-138, kick/dkick 62-77, unban 453-465, mute/dmute mute.go:62-77, tmute 125-142, unmute 263-287; no other send of these defaults exists (grep of the seven locale keys finds only the builders). Placeholders: `eventPlaceholderValues` event_message.go:91-103 (reason, duration escaped; until via `formatChatTime` in the chat's zone; others empty); names via `formatting.FillPlaceholders`, which HTML-escapes first/last/full name, username and title (formatting.go:261-270); sentinel pass `renderEventText` 153-163 so a value is never filled twice and `{count}` never triggers the member count. `{admin}`: `eventAdminName` 108-114, `actorIsAnonymous` 138-146, mark set only in `verifyAnonymousAdmin` after the live check (bot_updates.go:278). `/sban`, `/skick`, `/smute` have `reply: nil` or only delete (bans.go:351, 976; mute.go:173-176); `/dban`, `/dkick`, `/dmute` use the ban, kick and mute events (D-14) | `TestEventMessageSites`, `TestEventMessageSilentVariants`, `TestEventMessageAfterAnonymousProof` (event_message_sites_test.go:140,247,202); `TestRenderEventText`, `TestEventMessageCompose`, `TestEventMessageAnonymousAdmin` (event_message_test.go:218,275,420); e2e `owner sets a ban message` (caption `Ann &amp; &lt;Bo&gt; (spam &amp; co, by Ad&lt;b&gt;min)`), `timed` (Asia/Tokyo `{until}`), `anonymous` |
| SC4 | A group with nothing stored sends exactly today's reply in the group's language; a refused custom message falls back to the default after the punishment; a custom message is deleted on the same timer | ✓ VERIFIED | `replyEvent` event_message.go:232-265: read error, no row and an empty row all call `replySuccess` (moderation.go:52-58, the pre-Phase-20 call); the refusal (`eventMessageRefused`: 4xx other than 429 or `ErrNoPermission`, 213-215) sends the default once; a custom send is queued with `scheduleBotMessage(sent, deletion.KindBotManual)` (263). `replyEvent` runs in `cmd.reply`, after `cmd.execute` applied the punishment (moderation.go:295-306) | `TestLegacyModerationRequests` (moderation_legacy_request_test.go:166-212: 14 replies and 3 silent commands, request bytes and the 600 s `bot:punish_manual` row); `TestEventMessageFallback` (event_message_test.go:343: the ban precedes the send, a refused photo sends the default once, a 429 or 5xx makes no second send); e2e `refused`, `remove`, `owner sets a ban message` (`wantQueued`) |
| SC5 | Each event has a Silent switch; Silent acts and posts nothing; viewing needs the live admin check and the owner gate; every change needs `can_change_info`, judged live on every press and every typed message | ✓ VERIFIED | Silent action `hq` with `Right: settingsRightChangeInfo` settings_messages.go:190-192, run 419-439 through `applyEventSilent` (event_message_apply.go:65-68); `replyEvent` returns nil for a silent row (event_message.go:241-244), so nothing is posted or scheduled while the action log still runs; Set/Remove need `EditRight: settingsRightChangeInfo` (155); the typed reply calls `authorize(def.EditRight)` (settings_input_door.go:236, 285-349: live `getChatMember`, owner gate, right, live `getChat`); viewing actions `hl`/`he` need `settingsRightAdmin` (185-188) | `TestSettingsMessagesSilent`, `TestSettingsDriftMessagesSilent`, `TestSettingsAuthorityMessagesSilent` (no `can_change_info`, demoted between presses, plain member, gated admin, live creator), `TestSettingsAuthorityMessagesEdit`, `TestSettingsDriftMessagesEdit`; e2e `silent`, `rights` (demoted before the typed message: refused, row unchanged), `two replicas` |

### Requirements coverage

| Requirement | Plans | Description (short) | Status | Evidence |
| ----------- | ----- | ------------------- | ------ | -------- |
| MSG-01 | 20-02, 20-04 | One stored message per event and destination: text, one media, URL buttons; nothing stored sends today's reply | ✓ SATISFIED (group); PM deferred to Phase 21 | migration `20261119120000_add_chat_event_messages.sql` (PK `(chat_id, event, dest)`, NOT NULL zero defaults, no CHECK on event/dest); `fuku/db/eventmsg/events.go` (`ValidEvent`, `ValidDest`), `repository.go` (cached `Get`, fresh `GetFresh`, four column-targeted upserts, each validates before writing and calls `cache.DeleteCache(CacheKey(chatID))`); `TestEventMessages`, `TestEventMessagesUpsert`, `TestLegacyModerationRequests` |
| MSG-02 | 20-04, 20-05 | Welcome placeholders plus the event ones; empty placeholders render nothing; user values escaped | ✓ SATISFIED | event_message.go:80-103, 153-163; formatting.go:261-270; `TestRenderEventText`, e2e caption and `timed` |
| MSG-03 | 20-04, 20-05 | `{admin}` "an admin" for an anonymous admin (and automatic actions); never the Staff Group | ✓ SATISFIED (manual scope); automatic actions arrive in Phase 22 | event_message.go:108-146; bot_updates.go:278; the renderer only knows the target, the group and the actor; the page is `Hidden` in a Staff Group and Staff Group `/ban` etc. are taken by the StaffActions interceptors before Bans and Mutes; `TestEventMessageAnonymousAdmin`, `TestEventMessageAfterAnonymousProof`, `TestSettingsStaffPrivacy` (Phase 20 screens in 8 languages, `settingsPrivacyPhase20Screens`) |
| MSG-04 | 20-04 | A refused custom message is replaced by the default; the punishment is never delayed, undone or skipped | ✓ SATISFIED | moderation.go:295-306 (execute before reply); event_message.go:255-262; `TestEventMessageFallback`, e2e `refused` (method order: ban, refused photo, default) |
| MSG-05 | 20-06 | Private-only Messages page with families and states; group menu shows the note | ✓ SATISFIED (manual family; others in Phases 22-24) | settings_messages.go:127-132, 195-206, 586-624; `TestSettingsMessagesOpens`, `TestSettingsMessagesGroupMenu`, `TestSettingsPrivateOnlyPage` |
| MSG-06 | 20-03, 20-08, 20-09 | Editor: Set / Remove of text, media, buttons by private messages with the editor's limits; Preview per part; Full preview through the real send path | ✓ SATISFIED | settings_messages.go:153-170, 246-258, 482-570; settings_input_door.go:229-280; `TestSettingsMessagesSet`, `...Remove`, `...Typed`, `...Preview`, e2e `previews` |
| MSG-07 | 20-07 | Silent switch per event, default off; Silent acts and posts nothing | ✓ SATISFIED | settings_messages.go:190-192, 419-439, 654-661; event_message.go:241-244; `TestSettingsMessagesSilent`, e2e `silent`, `no redis`, `two replicas` |
| MSG-08 | 20-06, 20-07, 20-08 | Viewing: live admin check and owner gate; every change: `can_change_info` live on every press and typed message | ✓ SATISFIED | settings_messages.go:155, 183-193; settings_input_door.go:236, 285-349; `TestSettingsAuthorityMessagesSilent`, `TestSettingsAuthorityMessagesEdit`, e2e `rights` |
| MSG-09 | 20-04, 20-05 | Bot messages deletion timers apply to custom messages as to the default | ✓ SATISFIED | event_message.go:263 (`KindBotManual`, the default's kind, moderation.go:57); a silent row schedules nothing; e2e `wantQueued` after a custom ban, a custom tmute and the default after a refusal |
| MSG-11 | 20-01 | Every existing reply byte-identical with nothing stored, proved by goldens recorded before the send sites changed | ✓ SATISFIED | `git log --follow fuku/modules/moderation_legacy_request_test.go` shows one commit, `31e7e18` (2026-11-18 23:41, the test file alone, 212 lines), before the first send-site change `3b0cfcb` (20-04); never edited since; it passes on HEAD |
| EVT-01 | 20-04, 20-05 | Seven manual events; silent and delete-first variants keep their behaviour | ✓ SATISFIED | eventmsg.Events (events.go:42-50); the seven sites above; `TestEventMessageSites`, `TestEventMessageSilentVariants`, golden cases `sban`, `skick`, `smute`, `dban`, `dkick`, `dmute` |

No requirement mapped to Phase 20 in REQUIREMENTS.md is missing from the plans (orphan check: MSG-01..09, MSG-11, EVT-01 all claimed; MSG-10 belongs to Phase 29).

### Specific adversarial checks

| Check | Result |
| ----- | ------ |
| A send site of the seven events that bypasses `replyEvent` | None. The only other `replySuccess` calls are the channel `/ban` and `/unban` (bans.go:251, 467), which act on a channel and are not events (D-14). `/kickme`, `/banme`, the restrict cards and the Unban/Unmute buttons are out of scope by D-16. |
| A custom message delaying, undoing or skipping the punishment | Not possible: `replyEvent` is reached only through `cmd.reply`, after `cmd.execute` returned nil (moderation.go:295-306). The read is bounded by 2 s and only delays the reply. A non-refusal send error (429, 5xx) is returned as the default's error always was, which skips the action log exactly as before Phase 20. |
| An unescaped user value in a rendered text | None found. Reason and duration are `html.EscapeString`-ed (event_message.go:96-98), `{admin}` too (110-113), names and title by `FillPlaceholders`; the sentinel pass prevents a value from being filled again; `{until}` is a formatted time. |
| A write without `can_change_info` judged live | None. Silent, Set and Remove declare `settingsRightChangeInfo`, judged by the press pipeline on a fresh `getChatMember`; the typed reply runs `authorize(def.EditRight)` on every message. The four `applyEvent*` functions are the only callers of the repository writers (grep). |
| A write that does not drop `event_messages:<chat>` | None. Each of `SetText`, `SetMedia`, `SetButtons`, `SetSilent` calls `cache.DeleteCache(CacheKey(chatID))` after a successful upsert (repository.go:111-161); the prefix is in `skipLocal` (fuku/db/cache/local.go:54, 97). |
| The Staff Group named anywhere | Not found. The page is `Hidden` for a Staff Group (settings_messages.go:371-374); no Phase 20 locale string mentions it; `TestSettingsStaffPrivacy` renders every Messages screen in 8 languages for a group view and a non-creator private view. |
| `TestLegacyModerationRequests` edited after its commit | No: one commit, `31e7e18`, which touched only that file. |

### Required artifacts

| Artifact | Expected | Status |
| -------- | -------- | ------ |
| `migrations/20261119120000_add_chat_event_messages.sql` | table, inline PK, zero defaults, idempotent | ✓ VERIFIED |
| `fuku/db/eventmsg/events.go`, `repository.go` | Go-side event table, cached and fresh reads, four upserts with cache drop | ✓ VERIFIED, wired (event_message.go:33, settings_messages.go:175, event_message_apply.go:19-24) |
| `fuku/db/models/event_message.go` | model, `TableName()` | ✓ VERIFIED |
| `fuku/modules/event_message.go` | engine: read, compose, send, fallback, schedule | ✓ VERIFIED, called by seven sites |
| `fuku/modules/event_message_apply.go` | one apply function per part | ✓ VERIFIED, called by the page hooks |
| `fuku/modules/settings_messages.go` | the page, families, events, editor definitions, Silent, previews | ✓ VERIFIED, registered in `init` |
| `AGENTS.md` | table, cache key, page, engine, tests | ✓ present (lines 1501, 2621, 3258 and the Testing bullets) |

### Data-flow trace

| Artifact | Data | Source | Real data | Status |
| -------- | ---- | ------ | --------- | ------ |
| `replyEvent` | stored row | `eventmsg.Get` -> `GetFresh` (`SELECT ... WHERE chat_id = ?`) through the cache | yes | ✓ FLOWING |
| Messages hub / event screen | states and parts | `eventmsg.GetFresh` | yes | ✓ FLOWING |
| `{until}` | end time | `t.untilDate` set by `extractTemporalTarget` (moderation.go:471) before reply, shown in `chatClockOf(chat)` | yes | ✓ FLOWING (e2e `timed`) |

### Anti-patterns

`TBD`, `FIXME`, `XXX`, `TODO`, `HACK`, `PLACEHOLDER` in the Phase 20 production files (`event_message.go`, `event_message_apply.go`, `settings_messages.go`, `fuku/db/eventmsg/*.go`, the migration): none. No `//nolint:unused` placeholders left (20-11 summary, confirmed by the build and the run tests).

| File | Line | Pattern | Severity | Impact |
| ---- | ---- | ------- | -------- | ------ |
| `fuku/db/eventmsg/events.go` | 21, 27 | comment says `/ban` or `/mute` "with a duration" maps to the timed event, but those per-group commands take no duration | ℹ️ Info | documentation only |

### Behavioral spot-checks

| Behavior | Command | Result | Status |
| -------- | ------- | ------ | ------ |
| Default replies byte-identical | `-run '^TestLegacyModerationRequests$'` | PASS (17 subtests) | ✓ PASS |
| Custom message end to end through the real dispatcher | `-run '^TestMessagesEndToEnd$'` | PASS (12 subtests: set, previews, timed, refused, silent, remove, anonymous, rights, group menu, staff group, no redis, two replicas) | ✓ PASS |
| Repository upserts and validation | `./fuku/db/eventmsg` | PASS | ✓ PASS |
| Kept out of backups | `TestEventMessagesStayOutOfBackup` | PASS | ✓ PASS |

### Probe execution

Not applicable: no probe scripts are declared by the phase and `scripts/*/tests/probe-*.sh` does not exist for it.

### Human verification required

See `human_verification` in the frontmatter: a live custom Ban with photo and button, a live refusal fallback, the anonymous admin `{admin}` on a live account (AnonAdmin off and on), live Silent and s-variants, the phone layout of the page and of the 13-row Main, and the timezone display of `{until}`.

### Gaps summary

No gaps. Every success criterion and requirement of Phase 20 has production code and a passing test that exercises it. The PM destination, automatic actions and the other families are deferred by the roadmap to Phases 21 to 24. Two non-blocking notes: the PostgreSQL-only migration test is left to CI's integrity run, and the eventmsg comments about "/ban with a duration" describe a path the per-group commands do not have.

---

_Verified: 2026-11-19T03:06:51Z_
_Verifier: gsd-verifier_
