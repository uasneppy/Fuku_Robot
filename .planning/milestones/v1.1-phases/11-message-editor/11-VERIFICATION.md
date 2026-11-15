---
phase: 11-message-editor
verified: 2026-11-04T21:40:00Z
status: human_needed
score: 13/13 must-haves verified (5/5 ROADMAP success criteria, 8/8 requirements)
covered_files:
  - ".planning/phases/11-message-editor/11-01-PLAN.md"
  - ".planning/phases/11-message-editor/11-01-SUMMARY.md"
  - ".planning/phases/11-message-editor/11-02-PLAN.md"
  - ".planning/phases/11-message-editor/11-02-SUMMARY.md"
  - ".planning/phases/11-message-editor/11-03-PLAN.md"
  - ".planning/phases/11-message-editor/11-03-SUMMARY.md"
  - ".planning/phases/11-message-editor/11-04-PLAN.md"
  - ".planning/phases/11-message-editor/11-04-SUMMARY.md"
  - ".planning/phases/11-message-editor/11-05-PLAN.md"
  - ".planning/phases/11-message-editor/11-05-SUMMARY.md"
  - ".planning/phases/11-message-editor/11-06-PLAN.md"
  - ".planning/phases/11-message-editor/11-06-SUMMARY.md"
  - ".planning/phases/11-message-editor/11-07-PLAN.md"
  - ".planning/phases/11-message-editor/11-07-SUMMARY.md"
  - ".planning/phases/11-message-editor/11-08-PLAN.md"
  - ".planning/phases/11-message-editor/11-08-SUMMARY.md"
  - ".planning/phases/11-message-editor/11-09-PLAN.md"
  - ".planning/phases/11-message-editor/11-09-SUMMARY.md"
  - ".planning/phases/11-message-editor/11-10-PLAN.md"
  - ".planning/phases/11-message-editor/11-10-SUMMARY.md"
  - ".planning/phases/11-message-editor/11-11-PLAN.md"
  - ".planning/phases/11-message-editor/11-11-SUMMARY.md"
  - ".planning/phases/11-message-editor/11-12-PLAN.md"
  - ".planning/phases/11-message-editor/11-12-SUMMARY.md"
  - ".planning/phases/11-message-editor/11-13-PLAN.md"
  - ".planning/phases/11-message-editor/11-13-SUMMARY.md"
  - ".planning/phases/11-message-editor/11-14-PLAN.md"
  - ".planning/phases/11-message-editor/11-14-SUMMARY.md"
  - "AGENTS.md"
  - "fuku/db/conn.go"
  - "fuku/db/greetings/repository.go"
  - "fuku/db/rules/repository.go"
  - "fuku/modules/greetings.go"
  - "fuku/modules/rules.go"
  - "fuku/modules/settings.go"
  - "fuku/modules/settings_edit.go"
  - "fuku/modules/settings_edit_actions.go"
  - "fuku/modules/settings_edit_check.go"
  - "fuku/modules/settings_edit_preview.go"
  - "fuku/modules/settings_input.go"
  - "fuku/modules/settings_input_door.go"
  - "fuku/modules/settings_menu.go"
  - "fuku/modules/settings_pages.go"
  - "fuku/utils/content/content.go"
  - "fuku/utils/formatting/formatting.go"
  - "locales/en.yml"
  - "locales/es.yml"
  - "locales/fr.yml"
  - "locales/hi.yml"
  - "locales/id.yml"
  - "locales/pt.yml"
  - "locales/ru.yml"
  - "locales/uk.yml"
covered_digest: "v4:sha256:03534355b39d969ab361b1d1115f7e93dd23428947384674e0bfd2d76ffc6b4a"
behavior_unverified: 0
overrides_applied: 0
human_verification:
  - test: "Owner decision on accepted risk AR-04 (review WR-01, WR-03, IN-04), which only the orchestrator has accepted so far. Main case (WR-01): paste a welcome text longer than 4096 characters into the private chat after Set text."
    expected: "The Telegram client splits the paste into several messages, each at or under 4096. The bot saves whichever part claims the wait first and sends no reply for the others. The saved page shows the stored length. The owner either accepts this as the limit of what the bot can see (the bot never receives an over-limit text) or asks for a follow-up. The same item covers WR-03 (a claim that is never finished or released, for example after a crash, leaves the sender's non-command private messages unanswered for up to 10 minutes) and IN-04 (a typed reply that reaches a replica still on Phase 10 code gets no answer)."
    why_human: "Success criterion 2 and EDIT-03 say a text that is too long is refused and the wait stays open. The code does refuse every over-limit text it receives. A client-side split is a real-client behaviour the bot cannot detect, so whether this is acceptable is the owner's call, not a code fact."
  - test: "11-SPIKE.md check 1 (A3): set a GIF as the welcome media, tap Full preview, then let a test account join."
    expected: "The saved page says File or GIF. Full preview and the real welcome both show the GIF the same way, because both go through media.SendGreeting with the stored document type."
    why_human: "How a GIF stored as a document renders on real clients is client behaviour. The fakes only prove that the call and its parameters match the join path."
  - test: "11-SPIKE.md check 2 (A4): send an album of three photos for the media part, then an album whose first item is an unsupported kind."
    expected: "One saved page with the album note and no other reply. The refused album gets one refusal."
    why_human: "The order and timing of album item delivery across updates is a live Telegram fact (assumption A4). The tests simulate the parallel items against miniredis."
  - test: "11-SPIKE.md check 3 (A5): as an administrator who has never started the bot, tap Full preview in a group menu, then tap Start in private and press it again."
    expected: "First press: the alert settings_edit_preview_need_start, and nothing posted in the group. After Start: the preview arrives in private and the group gets only the toast."
    why_human: "That Telegram answers 403 for a send to a user who never started the bot is assumption A5 (settingsEditIsForbidden classifies by code)."
  - test: "11-SPIKE.md check 4 (A6): fast taps on Preview, Full preview and the Welcome switch."
    expected: "Every preview arrives, or a rate-limited one answers the generic alert and the next tap works. The switch ends in the state of the last tap. No spinner is left."
    why_human: "Telegram's per-chat send limits are live behaviour (A6)."
  - test: "11-SPIKE.md check 5 (A1, A2, A7, A8): a text near 4096 characters with emoji, then one just over; 8 and then 9 buttons in a row; 100 and then 101 buttons; a custom emoji; a very long button name and link. Run each through Full preview and a real join."
    expected: "The editor's limits match what Telegram accepts at the join. Full preview shows any refusal before a join would."
    why_human: "Telegram's exact length counting, button limits and button-name or link limits (A1, A2, A8) could not be fetched during research. The editor's constants are starting values."
  - test: "11-SPIKE.md check 6 (A9): type a formatted text in the normal input box, paste one from another chat, and send one from Telegram Web."
    expected: "Each is saved with its formatting. A private message with no wait gets no reply."
    why_human: "Whether a real client ever sends a rich_message for a typed text (which the door skips) is assumption A9."
  - test: "11-SPIKE.md check 7: on Android, iOS, Desktop and Web, tap a <code> placeholder, a button example and the /cancel in the prompt footer. Read the UTC end time. Open the three pages on Hindi, Russian and Ukrainian phones."
    expected: "Placeholders and examples copy on tap, /cancel sends on tap, and no editor button or Main grid name is cut off at phone width."
    why_human: "Tap-to-copy, tap-to-send and label truncation are client rendering. TestSettingsLabelLengths holds code-point ceilings only."
  - test: "11-SPIKE.md check 8: run two replicas on one Redis and one database. Press Set on one replica and reply until the other replica handles the reply. Press Cancel on one after Set on the other."
    expected: "The reply is saved exactly once whichever replica takes it, and Cancel on either replica ends the wait."
    why_human: "TestSettingsEditWaitAcrossReplicas proves this with two dispatchers on shared miniredis. Real deployment config (REDIS_DB on both replicas) can only be checked live."
  - test: "11-SPIKE.md check 9: press Set text, wait 11 minutes, send the text, then send \"hello\"."
    expected: "One settings_edit_wait_ended_note reply naming the text part, nothing saved, no reply to \"hello\". Cancel on the old prompt answers settings_edit_wait_expired."
    why_human: "Real Redis TTL behaviour over wall-clock time. The tests use miniredis FastForward."
  - test: "11-SPIKE.md check 10: a native-speaker read of the 83 new editor strings and the changed settings_help_msg in es, fr, hi, id, pt, ru and uk."
    expected: "The strings read naturally and keep the placeholders, the <code> tags and the button syntax."
    why_human: "make check-translations and the locale tests guard keys, tokens and sizes, not wording."
  - test: "11-SPIKE.md check 11 (CI-only): make lint with a golangci-lint built on Go 1.26, and the full make test in CI."
    expected: "No new lint findings and a green make test."
    why_human: "The sandbox's golangci-lint was built with Go 1.25 and refuses the Go 1.26.0 target, and the sandbox's Go lacks the covdata tool that the full make test needs. This verification ran go vet, gofmt and the focused test sets instead (all clean)."
  - test: "Owner review of the judgment-tier prohibitions in the plans' must_haves (see 'Prohibitions' in the report body), and of the two follow-ups in 11-SPIKE.md: the pre-existing /rules double escaping, and Goodbye being On by default for a group with no greetings row."
    expected: "The owner accepts the verifier's non-authoritative verdict (all nine prohibitions hold, each backed by a passing test) and decides whether each follow-up becomes its own change."
    why_human: "Judgment-tier prohibitions are never passed silently by an autonomous verifier. The follow-ups are product decisions the phase left open on purpose."
audit_acknowledged:
  milestone: v1.1
  at: 2026-11-13
  status: human_needed
---

# Phase 11: Message Editor Verification Report

**Phase Goal:** As a group owner, I want to write the welcome, goodbye and rules from the bot's private chat by sending them as messages, so that I can set them up without typing commands in the group.
**Mode:** mvp (the user story format is valid: "As a group owner, I want to ..., so that ...")
**Verified:** 2026-11-04T21:40:00Z
**Status:** human_needed
**Re-verification:** No, initial verification

All five ROADMAP success criteria and all eight requirement IDs are delivered in the code. Every behaviour-dependent truth has a passing test that drives the real handlers through the hand-written Bot fake, miniredis and SQLite. The status is `human_needed` only because live checks remain: the owner chose to defer them (11-SPIKE.md), the owner still has to decide on AR-04 and the prohibitions, and two items run only in CI. There are no gaps.

## User Flow Coverage

User story: «As a group owner, I want to write the welcome, goodbye and rules from the bot's private chat by sending them as messages, so that I can set them up without typing commands in the group.»

| Step | Expected | Evidence | Status |
|------|----------|----------|--------|
| Open the menu in private | `/settings` in private lists the owner's groups. A pick opens Main, which shows Welcome, Goodbye and Rules with their states | Pages registered from `settings_edit.go:508-527` (`registerSettingsPage`, orders 50/60/70). `settingsEditMainState` gives On/Off or Set/Not set (`:404-423`). `TestSettingsMainGrid`, `TestSettingsEditRegistry` | VERIFIED |
| Open Welcome, Goodbye or Rules | The page shows one state line per part (text, media, buttons; Rules has text only) and never echoes the content | `renderSettingsEditor` and `settingsEditBody` (`settings_edit.go:289-400`), `settingsEditStateWord` (`:257-282`). `TestSettingsEditPages`, `TestSettingsStaffPrivacy` | VERIFIED |
| Tap Set on a part | The menu turns into the waiting prompt with the limit, the UTC end time and a Cancel button. A 10-minute wait is written in Redis | `settingsEditSet` (`settings_edit_actions.go:124-160`) calls `settingsInputSet`. The Lua script uses HSET plus PEXPIRE 10 min plus a 15-min marker (`settings_input.go:51-55`). `TestSettingsEditSetWritesWait` | VERIFIED |
| Send the message in private | The next private message is judged like a press, checked, claimed and stored through the part's apply function | Door registered at group 1 (`settings_input.go:604`, `settingsInputGroup = 1` in `settings_input_door.go:25`). `run` (`:176-232`): authorize, `settingsEditCheck`, `settingsInputClaim`, `apply`. `TestSettingsEditTypedTextSaved`, `TestSettingsEditDoorStoresEveryPart` | VERIFIED |
| See the saved page and preview | The saved page arrives with a confirmation. Full preview sends the whole message to the owner's private chat through the group's send path | `saved` (`settings_input_door.go:488-500`). `settingsEditPreviewGreeting` uses `renderGreeting` and `media.SendGreeting` (`settings_edit_preview.go:74-95`). `TestSettingsEditFullPreview` | VERIFIED |
| Outcome: the group shows it, with no command typed in the group | The next join, leave or `/rules` shows the stored message, which equals what the commands store | `TestSettingsEditWholeMessageParity`: text, photo and buttons saved one by one in the editor give the same row as one `/setwelcome` reply. Rules equals `/setrules`. The join path reads that row through the unchanged `renderGreeting` and `media.SendGreeting` (`greetings.go:809-815`, `:884-889`) | VERIFIED |

## Goal Achievement

### Observable Truths

| #   | Truth (ROADMAP success criterion) | Status | Evidence |
| --- | --------------------------------- | ------ | -------- |
| 1 | The owner opens Welcome, Goodbye or Rules and sees which parts are set, with a button to set or remove each. After a press, the next private message becomes that part. Cancel or `/cancel` leaves the message unchanged, the wait ends by itself after 10 minutes, and it works on any replica | VERIFIED | **Pages:** the parts table and three definitions (`settings_edit.go:82-133`). Part rows show `[Set]`, or `[Set \| Remove]` while the part is set (`:311-325`). Set, Remove and Cancel are registered with `Validate` on `t` and the commands' rights (`settings_edit_actions.go:267-276`). **Wait:** Redis-only hash `fuku:settings:input:<user>` with its own PEXPIRE (`settings_input.go:36-45,51-55`). No process map or AfterFunc exists (grep is empty). **Door:** `LoadSettings` calls `LoadSettingsInput` (`settings.go:238`), which registers `/cancel` through `WrapCommand` and the door at group 1 (`settings_input.go:600-605`). The door returns `ContinueGroups` for anyone with no wait and no marker before any send (`settings_input_door.go:83-114`). **Cancel:** the button runs `settingsEditCancel` and the Cancel script, which compares group, page, part and prompt ID (`settings_edit_actions.go:166-191`). `/cancel` runs `settingsCancel` and `settingsInputCancelAny` (`settings_input.go:577-595`). A non-private `/cancel` is passed on (`:546-564`). **Expiry and replicas:** the marker answers once with GETDEL (`settings_input_door.go:155-173`). Tests pass: `TestSettingsEditSetWritesWait`, `TestSettingsEditCancelButton`, `TestSettingsEditCancelCommand*`, `TestSettingsEditWaitExpiry` (miniredis FastForward past `settingsInputTTL`), `TestSettingsEditMarkerReplyOnce`, `TestSettingsEditWaitAcrossReplicas` (a second dispatcher saves once and the first sends nothing), `TestSettingsEditDoorPassThrough`, `TestSettingsCancelInGroupPassesOn`. |
| 2 | Text keeps formatting and placeholders. A too-long text (4096, or 1024 as a caption), bad button syntax or a bad URL is refused with the reason and the wait stays open. One media item of a supported kind is accepted. From an album one item is kept and the owner is told | VERIFIED | **Conversion:** `tgmd2html.MD2HTMLButtonsV2(msg.OriginalMDV2())` for greetings and `MD2HTMLV2` for rules, the commands' own conversion (`settings_edit_check.go:129-169,218-239`). **Limits:** the commands' own `content.TextLengthRefusal` (`content.go:204`, also called by `preFixes` at `:239`), then a dry run with the owner, a worst-case member and a minimal member through `formatting.FillPlaceholders`/`VisibleLength` (`settings_edit_check.go:198-214`), at 4096 or 1024 when media is stored (`:145-160`). **Buttons:** the commands' parser, `content.ValidButtonURL` (`content.go:224`, also used by `preFixes` at `:252`), and 100 buttons in all or 8 in a row, counting `{rules}` (`settings_edit_check.go:282-334`). **Media:** `content.ExtractMedia`, video note refused, caption ignored with a note (`:262-278`). **Wait stays open:** a refusal is sent before the claim and only adds an `r<id>` field (HSET, no PEXPIRE: `settings_input.go:148-153`). **Album:** `albumFirst` uses SET NX 30 s, and the saved notice adds `settings_edit_album_cut` (`settings_input_door.go:127-137,474-483`). Tests pass: `TestSettingsEditCheckTextParity`, `TestSettingsEditTextLimits`, `TestSettingsEditFilledLength`, `TestSettingsEditButtonRefusalsDoor`, `TestSettingsEditMediaKindsDoor`, `TestSettingsEditRefusalKeepsWait`, `TestSettingsEditAlbumWritesOnce`, `TestSettingsEditAlbumNotice`, `TestSettingsEditRefusedAlbumOneRefusal`. Concern (not a gap): a client-split over-limit paste, recorded as WR-01/AR-04. See the human items. |
| 3 | Each part has a Preview, and Full preview sends the whole message as members see it through the group's send path. A message Telegram would refuse is refused before it is saved | VERIFIED | Preview rows come after the part rows, with Full preview last (`settings_edit.go:337-354`). The actions `wv/wf`, `gv/gf` and `rv/rf` need `settingsRightAdmin` (`settings_edit_actions.go:273-274`). Full preview uses `renderGreeting` (the join's builder, `greetings.go:316`) and `media.SendGreeting` with the stored type, file and keyboard. It is sent only to `p.query.From.Id`, filled with the presser and the group title (`settings_edit_preview.go:45-95`). Rules Full preview uses `buildRulesText`, the `/rules` builder (`:61-68`). A 403 answers need_start, other errors the generic alert, and nothing is retried (`:101-113`). The pre-save refusal is the dry run in truth 2, which always runs before `settingsInputClaim` (`settings_input_door.go:194-208`). Tests pass: `TestSettingsEditFullPreview`, `TestSettingsEditPreview`, `TestSettingsEditRulesPreview`, `TestSettingsEditPreviewWritesNothing`, `TestSettingsEditPreviewErrors`, `TestRenderGreetingMatchesJoin`, `TestBuildRulesTextMatchesSendRules`. |
| 4 | A message saved in the editor is stored exactly as `/setwelcome`, `/setgoodbye` or `/setrules` would store it, so the next join, leave or `/rules` shows it, and those commands still work | VERIFIED | Editor writes go through the apply functions (`greetings.go:258-301`, `rules.go:56-66`). They write through column-targeted repository writers that share `upsertGreetingSettings` and its `cache.DeleteCache(greetings:<chat>)` (`fuku/db/greetings/repository.go:218-237,258-348`). `/setrules` calls `applyRulesText` (`rules.go:251`) and `/clearrules` calls `applyRulesClear` (`:90`). `/resetwelcome` and `/resetgoodbye` call `applyGreetingReset` (`greetings.go:533`). `/setwelcome` keeps its four-column `SetWelcomeText` write (`:497`), and parity with the editor's one-part writes is proven by test. Tests pass: `TestSettingsEditWholeMessageParity` (text, photo and buttons each saved through the door equal one `/setwelcome` reply to the photo, for Welcome and Goodbye; Rules equals `/setrules`), `TestSettingsEditTextParity`, `TestSettingsEditMediaParity`, `TestSettingsEditButtonsParity`, `TestSettingsEditRemoveParity`, `TestGreetingRemovesEqualReset`, `TestRulesApplyParity`, `TestMemberJoinAndLeaveSendConfiguredGreetings`, `TestGreetingCommandsPropagateGotgbotRequestErrors`. |
| 5 | The Welcome and Goodbye pages switch each message on or off and turn Delete last on or off, stored exactly as `/welcome`, `/goodbye`, `/cleanwelcome` and `/cleangoodbye` store them | VERIFIED | Control `e` uses `settingsRightAdmin` and control `d` uses `settingsRightChangeInfo` (`settings_edit.go:519-524`). Each sends the absolute `v` "0" or "1", anything else is invalid (`:463-495`), and calls `applyWelcomeEnabled`, `applyGoodbyeEnabled`, `applyCleanWelcome` or `applyCleanGoodbye` (`:443-457`). The commands call the same functions: `/welcome` and `/goodbye on\|off` (`greetings.go:431,433`), `/cleanwelcome` (`:637`) and `/cleangoodbye` (`:661`). Tests pass: `TestSettingsParityGreetingSwitches`, `TestSettingsDriftGreetings`, `TestSettingsAuthorityParityGreetings`, `TestSettingsEditWelcomeSwitch`, `TestSettingsEditSwitchFailure`, `TestGreetingApplyParity`, `TestGreetingApplyIsAbsolute`. |

**Score:** 5/5 truths verified (0 present but behavior-unverified). Requirements: 8/8 satisfied. Total 13/13.

### Required Artifacts

| Artifact | Expected | Status | Details |
| -------- | -------- | ------ | ------- |
| `fuku/modules/settings_edit.go` | Parts table, three pages, pure renderer, MainState, switches, init registration | VERIFIED | Read in full. Wired through `registerSettingsPage` in `init` |
| `fuku/modules/settings_edit_actions.go` | Set, Cancel, Remove, prompt, Validate | VERIFIED | Read in full. Group menu wrapped by `settingsEditPrivateOnly` (D-01) |
| `fuku/modules/settings_edit_check.go` | Pure check and dry run | VERIFIED | Read in full. No DB read, no send. Converter panic recovered |
| `fuku/modules/settings_edit_preview.go` | Preview and Full preview through the group's send path | VERIFIED | Read in full. Target is always `query.From.Id` |
| `fuku/modules/settings_input.go` | Wait store (7 Lua scripts), `/cancel`, `LoadSettingsInput` | VERIFIED | Keys `fuku:settings:input:`, `inputdone:` and `inputalbum:`. AGENTS.md says "seven Lua scripts", which matches the 7 `redis.NewScript` |
| `fuku/modules/settings_input_door.go` | Typed-reply door at group 1 | VERIFIED | Read in full. Live member check, gate, right, bot check, `getChat`, then check, claim, apply, finish or release |
| `fuku/modules/greetings.go`, `rules.go` | Apply functions, `renderGreeting`, `buildRulesText` shared with the commands | VERIFIED | Commands call them (see truths 4 and 5) |
| `fuku/db/greetings/repository.go`, `fuku/db/rules/repository.go` | Uncached reads that create nothing, and one-part writers | VERIFIED | `LoadSettingsContext`, `LoadRulesContext`, `Set*Only` through `upsertGreetingSettings`. `TestLoadGreetingSettingsCreatesNothing`, `TestGreetingColumnWritersKeepOtherParts` |
| `fuku/utils/content/content.go`, `fuku/utils/formatting/formatting.go` | Shared rules: `TextLengthRefusal`, `ValidButtonURL`, `ExtractMedia`, `FillPlaceholders`, `VisibleLength` | VERIFIED | `preFixes` calls the first two and `FormattingReplacer` calls `FillPlaceholders` (`formatting.go:309`) |
| `locales/*.yml` | 83 editor keys in all 8 | VERIFIED | 83 matching keys in each file. All 86 `settings_*` keys the editor code names exist in all 8 |
| Docs | AGENTS.md, commands.md, callbacks.md, permissions.md, handler-groups.md, README | VERIFIED | AGENTS.md has group `1` in the handler list (line 97), the door (146-180), the keys (572-581) and the previews (1557). commands.md lists `/cancel`, callbacks.md has the `w/g/r` codes table, permissions.md has the editor rights, handler-groups.md has the group 1 row, README has the Greetings line |

### Key Link Verification

| From | To | Via | Status | Details |
| ---- | -- | --- | ------ | ------- |
| `LoadSettings` | `LoadSettingsInput` | direct call | WIRED | `settings.go:238` |
| `LoadSettingsInput` | `settingsInput` door | `AddHandlerToGroup(handlers.NewMessage(settingsInputIsPrivate, ...), settingsInputGroup)` | WIRED | `settings_input.go:604` |
| `settingsEditSet` | `settingsInputSet` | wait with group, page, part, prompt ID, nonce | WIRED | `settings_edit_actions.go:141` |
| door | `settingsGate`, `settingsRightRefusalFor` | the live member record | WIRED | `settings_input_door.go:255,265` |
| door | claim, finish, release | claim only after check, finish on success, release on failed write | WIRED | `settings_input_door.go:208,220,226` |
| door | `settingsInputAlbumFirst` | album gate before the live checks | WIRED | `settings_input_door.go:131` |
| `settingsCheckFields` | `settingsPageAction.Validate` | step 1, before I/O | WIRED | `settings_menu.go:277` |
| `settingsRightRefusal` | `settingsRightRefusalFor` | shared pure judgement | WIRED | `settings_menu.go:120` |
| switches and Remove | commands' apply functions | `applyWelcomeEnabled` etc., `applyGreetingRemove*`, `applyRulesClear` | WIRED | `settings_edit.go:443-457`, `settings_edit_actions.go:196-226` |
| previews | `renderGreeting`, `media.SendGreeting`, `buildRulesText` | the join and `/rules` builders | WIRED | `settings_edit_preview.go:62,79-91` |
| one-part writes | `greetings:<chat>` cache | `upsertGreetingSettings` then `cache.DeleteCache` | WIRED | `fuku/db/greetings/repository.go:236` |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
| -------- | ------------- | ------ | ------------------ | ------ |
| Editor pages and Main lines | `settingsEditState` | `greetings.LoadSettingsContext`, `rules.LoadRulesContext` (fresh DB reads, no create) | Yes | FLOWING |
| Typed reply | `verdict.Text/FileID/Buttons` | the owner's private message, converted by the commands' converters | Yes, persisted through the apply functions | FLOWING |
| Full preview | rendered text and keyboard | stored row, then `renderGreeting`, then `media.SendGreeting` | Yes | FLOWING |
| Wait | `settingsInputWait` | Redis hash written by Set, read by the door on any replica | Yes | FLOWING |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
| -------- | ------- | ------ | ------ |
| Compile | `CGO_ENABLED=0 go build ./...` | ok | PASS |
| Vet | `go vet -tags testtools ./fuku/modules` | ok | PASS |
| Translations | `make check-translations` | All translations present (exit 0) | PASS |
| Generated docs | `make check-docs` | No drift (exit 0), tree still clean | PASS |
| Dependencies untouched | `git diff --exit-code 72a692b -- go.mod go.sum` | no diff | PASS |
| Format | `gofmt -l` on every Go file the phase changed | none | PASS |
| Editor, input, gate, privacy, group `/cancel` | `go test -tags testtools -race -count=1 -run '^(TestSettingsEdit\|TestSettingsInput\|TestSettingsGateAllDoors\|TestSettingsStaffPrivacy\|TestSettingsCancelInGroupPassesOn)' ./fuku/modules` | ok (122.8 s) | PASS |
| Parity, drift, authority, rights, apply, builders, Main, join and leave | `-run '^(TestSettingsParityGreetingSwitches\|TestSettingsDriftGreetings\|TestSettingsAuthorityParity\|TestSettingsPageActionValidate\|TestSettingsRight\|TestGreeting\|TestRenderGreetingMatchesJoin\|TestRulesApply\|TestBuildRulesText\|TestMemberJoinAndLeaveSendConfiguredGreetings\|TestSettingsMain\|TestLoadSettings\|TestSettingsCommandsUngated)' ./fuku/modules` (33 tests, listed with `-list`) | ok | PASS |
| Locale guards, shared helpers, repositories | `go test -tags testtools -race -count=1 ./fuku/i18n ./fuku/utils/content ./fuku/utils/formatting ./fuku/db/greetings ./fuku/db/rules` | all ok | PASS |

Each invariant-style truth has a test that drives the real handler and checks the observable result. The truths covered this way: a refusal neither consumes nor extends the wait, a double delivery writes once, a reply on another replica is saved, an expired wait answers once, a group menu never creates a wait, and a preview writes nothing. None is left resting on presence alone.

### Probe Execution

Step 7c: SKIPPED. The phase declares no probe scripts.

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
| ----------- | ----------- | ----------- | ------ | -------- |
| EDIT-01 | 11-01/03/06/08/09 | Pages show which parts are set, with Set or Remove per part | SATISFIED | Truth 1 |
| EDIT-02 | 11-01/07/09/11/14 | Next private message becomes the part. Cancel, `/cancel`, 10-minute expiry, any replica | SATISFIED | Truth 1 |
| EDIT-03 | 11-02/04/10/11 | Formatting and placeholders kept. Over-limit text refused, wait stays | SATISFIED | Truth 2 (with the WR-01 concern for owner review) |
| EDIT-04 | 11-10/13 | One media item of a supported kind. Album cut to one, owner told | SATISFIED | Truth 2 |
| EDIT-05 | 11-02/10/13 | Existing button syntax. Bad syntax or URL refused, wait stays | SATISFIED | Truth 2 |
| EDIT-06 | 11-04/05/12/14 | Preview per part. Full preview through the group's send path | SATISFIED | Truth 3 |
| EDIT-10 | 11-05/08 | On/off and Delete last stored as the commands store them | SATISFIED | Truth 5 |
| EDIT-11 | 11-03/05/06/10/13 | Stored exactly as the commands store. Refused before saving if Telegram would refuse | SATISFIED | Truths 3 and 4 |

All eight IDs appear in the plans' `requirements` frontmatter. They are ticked `[x]` and marked Complete in REQUIREMENTS.md (lines 24-34, 143-153), which matches the code. EDIT-07, EDIT-08 and EDIT-09 map to Phase 12 and stay Pending. No orphaned requirement maps to Phase 11. ROADMAP shows 14/14 plans. Its Phase 11 checkbox and progress row ("In Progress") wait for this verification. STATE.md frontmatter (`percent: 0`, `last_activity_desc: Phase 11 execution started`) is stale metadata for the orchestrator to update (Info).

### Decisions D-01..D-13 against the code

| Decision | Holds | Evidence |
| -------- | ----- | -------- |
| D-01 private-only Set and Remove, Open in private in a group menu, previews and switches in both | Yes | `settingsEditPrivateOnly` (`settings_edit_actions.go:104-113`), Open in private row (`settings_edit.go:374-386`). Previews and switches are not wrapped. `TestSettingsEditGroupMenuRefusal` |
| D-02 own files, `registerSettingsPage`, full press pipeline | Yes | `settings_edit.go:508-527`. `settings_pages.go` changed only for the `Validate` hook (plan 11-06), with no page registered there |
| D-03 one slot per person, replace says so | Yes | Set script (`settings_input.go:51-55`), `settingsEditWaitReplaced` (`settings_edit_actions.go:117-119`) |
| D-04 Redis wait, group below 12 returning ContinueGroups, Set unavailable without Redis | Yes | Group 1, `errSettingsInputUnavailable` gives `settings_edit_unavailable` |
| D-05 reply judged like a press, lost access ends the wait, service identities excluded | Yes | `authorize` (`settings_input_door.go:238-292`), `settingsInputCandidate` (`:61-65`), sixth door of `TestSettingsGateAllDoors` |
| D-06 same stored form | Yes | Truth 4 |
| D-07 dry run with owner and worst-case names | Yes | `settingsEditFilledLongest` |
| D-08 one media item, album cut, existing button syntax | Yes | Truth 2 |
| D-09 Remove equals reset or clear | Yes | `TestGreetingRemovesEqualReset`, `TestSettingsEditRemoveParity` |
| D-10 previews only to the presser's private chat, owner's name and group title | Yes | `settings_edit_preview.go`. A `{rules}` preview creating an empty rules row as a join does is documented and accepted (AR-02) |
| D-11 one apply function per switch, absolute values | Yes | Truth 5 |
| D-12 marker turns a late reply into one plain reply | Yes | `waitEnded` with GETDEL, `TestSettingsEditMarkerReplyOnce` |
| D-13 a refused album gets one refusal | Yes | `TestSettingsEditRefusedAlbumOneRefusal` |

### Prohibitions (judgment tier, non-authoritative verdict, flagged for owner review)

Every plan prohibition is `verification: judgment`. The verifier's verdict below is non-authoritative and is listed among the human items. None is passed silently.

| Plan | Prohibition | Verifier verdict | Evidence |
| ---- | ----------- | ---------------- | -------- |
| 11-01 | No new string carries a placeholder that could name the Staff Group, a group, a user or a stored message | holds | `TestSettingsEditLocaleTokens`, `TestSettingsStaffPrivacy` |
| 11-03, 11-08 | A page or Main read never creates a greetings, rules or chats row | holds | `TestSettingsEditReadsCreateNothing`, `TestLoadGreetingSettingsCreatesNothing`. The `{rules}` preview exception is accepted as AR-02 |
| 11-07 | The wait is never in a process map, behind `time.AfterFunc`, or holding text or a file ID | holds | grep finds no `AfterFunc` or map in the editor files. The hash fields are IDs and codes only |
| 11-08 | No page shows the stored text, a button name, a link or the Staff Group | holds | state words only (`settings_edit.go:257-282`), `TestSettingsStaffPrivacy` |
| 11-09 | A group menu never creates a wait or removes a part | holds | `settingsEditPrivateOnly`, `TestSettingsEditGroupMenuRefusal` |
| 11-11 | The door never answers, writes or calls Telegram for someone with no wait and no marker. A refusal never consumes or extends the wait | holds | `settings_input_door.go:96-98` (return before any send), note-refusal script has no PEXPIRE, `TestSettingsEditDoorPassThrough`, `TestSettingsEditRefusalKeepsWait` |
| 11-12 | A preview never posts into the group, deletes a message or changes `last_msg_id` | holds | target `query.From.Id`, `TestSettingsEditPreviewWritesNothing` |
| 11-13 | No album produces more than one write or reply | holds | `TestSettingsEditAlbumWritesOnce`, `TestSettingsEditRefusedAlbumOneRefusal`, `TestSettingsEditExpiredAlbumOneReply` |
| 11-14 | No editor screen names the Staff Group except to the creator in a private menu | holds | `TestSettingsStaffPrivacy` (prompt, refusals, saved page, previews, 8 languages) |

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
| ---- | ---- | ------- | -------- | ------ |
| (phase source files) | - | No `TBD`, `FIXME`, `XXX`, `TODO`, `HACK` or placeholder stub | - | none |
| `.planning/phases/11-message-editor/11-SPIKE.md` | follow-ups | Says an anonymous admin's `/cancel` in a group is answered "cannot identify user". Since NEW-01, `settingsCancelPassOn` takes every non-private `/cancel` that gotgbot's command filter matches, and returns `nil` with no reply before the pipeline runs, so that note is probably stale | Info | Documentation only. The behaviour is quieter than the note says |
| `.planning/STATE.md` | 18, 11 | `percent: 0`, "execution started" after all plans ran | Info | Orchestrator bookkeeping |

### Prior Audits (used as evidence, spot-checked, not redone)

- Code review: 0 critical. WR-02, WR-04, WR-05, IN-01, IN-02 and IN-03 are fixed, and each fix is visible in the code (`botAdmin` at `settings_input_door.go:301-321`, the `{rules}` count at `settings_edit_check.go:162-167`, the Cancel Redis-error branch at `settings_edit_actions.go:176-181`, the prompt-ID compare in the Cancel script, `settings_edit_wait_stopped`). WR-01, WR-03 and NEW-04 are deferred and accepted as AR-03 and AR-04 by the orchestrator only. They are listed for the owner.
- Security: status verified, 0 open threats. NEW-01 (group `/cancel` bypassing moderation) is fixed and pinned by `TestSettingsCancelInGroupPassesOn`. NEW-03 (`ParameterizedQueries: true`, `fuku/db/conn.go:56-59`) is in place.
- Validation: `validated`, `nyquist_compliant: true`.

### Human Verification Required

See the frontmatter list. In short: the owner's decision on AR-04 (a client-split over-limit paste saves one part, plus WR-03 and IN-04); the live checks 1 to 10 of 11-SPIKE.md (GIF rendering A3, album delivery A4, the 403 for an admin who never started the bot A5, rate limits A6, Telegram's length and button limits A1/A2/A7/A8, rich_message A9, tap-to-copy and label truncation in hi/ru/uk, two live replicas, a real 10-minute expiry, a native-speaker read); the CI-only items of check 11 (`make lint` on Go 1.26 and the full `make test`); and the owner's review of the judgment-tier prohibitions and of the two follow-ups (`/rules` double escaping, Goodbye On by default).

### Gaps Summary

No gaps. Every ROADMAP success criterion and every requirement is delivered and wired in the code:

- The pages are registered and reachable from Main.
- The typed-reply door sits at group 1 and is registered by `LoadSettingsInput`, which `LoadSettings` calls.
- The Redis wait, the Cancel button and `/cancel` are connected.
- Set, Remove, Preview and Full preview declare the commands' rights.
- The switches and Remove go through the same apply functions as the commands.
- The previews go through the join's builder and send function.
- The locale keys exist in all 8 files, and AGENTS.md and the manual docs describe what exists.

The one point that touches a criterion's wording is WR-01: Telegram clients split a long paste, so the bot never sees the over-limit text and cannot refuse it. That is a real-client limit the owner must accept or turn into a follow-up. It is not a missing piece of code.

---

_Verified: 2026-11-04T21:40:00Z_
_Verifier: Claude (gsd-verifier)_
