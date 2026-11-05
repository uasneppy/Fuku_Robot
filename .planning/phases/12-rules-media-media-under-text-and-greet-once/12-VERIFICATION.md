---
phase: 12-rules-media-media-under-text-and-greet-once
verified: 2026-11-05T15:23:21Z
status: human_needed
score: 10/10 must-haves verified (7/7 truths: 4/4 ROADMAP success criteria plus 3 supporting truths; 3/3 requirements)
covered_files:
  - ".planning/phases/12-rules-media-media-under-text-and-greet-once/12-01-PLAN.md"
  - ".planning/phases/12-rules-media-media-under-text-and-greet-once/12-01-SUMMARY.md"
  - ".planning/phases/12-rules-media-media-under-text-and-greet-once/12-02-PLAN.md"
  - ".planning/phases/12-rules-media-media-under-text-and-greet-once/12-02-SUMMARY.md"
  - ".planning/phases/12-rules-media-media-under-text-and-greet-once/12-03-PLAN.md"
  - ".planning/phases/12-rules-media-media-under-text-and-greet-once/12-03-SUMMARY.md"
  - ".planning/phases/12-rules-media-media-under-text-and-greet-once/12-04-PLAN.md"
  - ".planning/phases/12-rules-media-media-under-text-and-greet-once/12-04-SUMMARY.md"
  - ".planning/phases/12-rules-media-media-under-text-and-greet-once/12-05-PLAN.md"
  - ".planning/phases/12-rules-media-media-under-text-and-greet-once/12-05-SUMMARY.md"
  - ".planning/phases/12-rules-media-media-under-text-and-greet-once/12-06-PLAN.md"
  - ".planning/phases/12-rules-media-media-under-text-and-greet-once/12-06-SUMMARY.md"
  - ".planning/phases/12-rules-media-media-under-text-and-greet-once/12-07-PLAN.md"
  - ".planning/phases/12-rules-media-media-under-text-and-greet-once/12-07-SUMMARY.md"
  - ".planning/phases/12-rules-media-media-under-text-and-greet-once/12-08-PLAN.md"
  - ".planning/phases/12-rules-media-media-under-text-and-greet-once/12-08-SUMMARY.md"
  - ".planning/phases/12-rules-media-media-under-text-and-greet-once/12-09-PLAN.md"
  - ".planning/phases/12-rules-media-media-under-text-and-greet-once/12-09-SUMMARY.md"
  - ".planning/phases/12-rules-media-media-under-text-and-greet-once/12-10-PLAN.md"
  - ".planning/phases/12-rules-media-media-under-text-and-greet-once/12-10-SUMMARY.md"
  - ".planning/phases/12-rules-media-media-under-text-and-greet-once/12-11-PLAN.md"
  - ".planning/phases/12-rules-media-media-under-text-and-greet-once/12-11-SUMMARY.md"
  - ".planning/phases/12-rules-media-media-under-text-and-greet-once/12-12-PLAN.md"
  - ".planning/phases/12-rules-media-media-under-text-and-greet-once/12-12-SUMMARY.md"
  - ".planning/phases/12-rules-media-media-under-text-and-greet-once/12-13-PLAN.md"
  - ".planning/phases/12-rules-media-media-under-text-and-greet-once/12-13-SUMMARY.md"
  - ".planning/phases/12-rules-media-media-under-text-and-greet-once/12-14-PLAN.md"
  - ".planning/phases/12-rules-media-media-under-text-and-greet-once/12-14-SUMMARY.md"
  - ".planning/phases/12-rules-media-media-under-text-and-greet-once/12-15-PLAN.md"
  - ".planning/phases/12-rules-media-media-under-text-and-greet-once/12-16-PLAN.md"
  - ".planning/phases/12-rules-media-media-under-text-and-greet-once/12-16-SUMMARY.md"
  - "AGENTS.md"
  - "Makefile"
  - "README.md"
  - "docs/src/content/docs/api-reference/callbacks.md"
  - "docs/src/content/docs/api-reference/permissions.md"
  - "fuku/db/backup/backup.go"
  - "fuku/db/db.go"
  - "fuku/db/greetings/greeted.go"
  - "fuku/db/greetings/repository.go"
  - "fuku/db/lang/repository.go"
  - "fuku/db/models/greeted_member.go"
  - "fuku/db/models/greetings.go"
  - "fuku/db/models/rules.go"
  - "fuku/db/rules/repository.go"
  - "fuku/modules/greetings.go"
  - "fuku/modules/rules.go"
  - "fuku/modules/settings_edit.go"
  - "fuku/modules/settings_edit_actions.go"
  - "fuku/modules/settings_edit_check.go"
  - "fuku/modules/settings_edit_preview.go"
  - "fuku/modules/settings_input_door.go"
  - "fuku/modules/settings_menu.go"
  - "fuku/modules/settings_pages.go"
  - "fuku/utils/content/content.go"
  - "fuku/utils/formatting/formatting.go"
  - "fuku/utils/media/sender.go"
  - "locales/en.yml"
  - "locales/es.yml"
  - "locales/fr.yml"
  - "locales/hi.yml"
  - "locales/id.yml"
  - "locales/pt.yml"
  - "locales/ru.yml"
  - "locales/uk.yml"
  - "migrations/20261105120000_add_rules_media_and_greet_once.sql"
covered_digest: "v4:sha256:7abc207631c08c02f99810821d1bdcea92e53273856c2ef45af2d775a50c4f8c"
behavior_unverified: 0
overrides_applied: 0
decision_coverage:
  honored: 17
  total: 17
  not_honored: []
human_verification:
  - test: "12-SPIKE.md check 1 (A1, backstop truth of 12-16): for a photo, a video and a GIF set now, turn Media under text on, tap Full preview, then let a test account join. Repeat on Android, iOS, Desktop and Web."
    expected: "On every client the caption (the welcome text) sits above the media, in the preview and in the real welcome. The sent message echoes show_caption_above_media true."
    why_human: "The tests prove the flag is sent exactly for sendPhoto, sendVideo and sendAnimation (TestSendGreetingMessageCaptionAbove, TestGreetingsMediaBelowEverywhere, TestRulesMediaBelowEverywhere, TestSettingsEditFullPreviewMediaBelow). Where a real client draws the caption is client rendering."
  - test: "12-SPIKE.md check 2 (A6): open the same group on an old Telegram client with Media under text on."
    expected: "The old client shows today's layout, with no error and no missing media."
    why_human: "Old-client rendering cannot be faked."
  - test: "12-SPIKE.md check 3 and the 'EDIT-07 limit' section (A7): set a GIF now and check it loops through sendAnimation. Then open a group whose welcome GIF was saved before Phase 12 (a document) and set that same GIF again."
    expected: "A new GIF reads GIF on the page and plays as an animation. An old GIF reads File or GIF, shows the file hint and no switch, and shows exactly as before until it is set again. After that the switch appears."
    why_human: "That a file ID taken from message.animation plays through sendAnimation, and how an old document GIF renders, is live Bot API behaviour. TestSetWelcomeGifIsAnimation and the 12-01 golden prove only the stored type and the request."
  - test: "12-SPIKE.md check 4 (A3): with a rules photo set, send a rules text of about 1030 characters through the editor. Then use /setrules so that heading plus text is exactly 1025 UTF-16 units, and run /rules and the private-rules link."
    expected: "The editor refuses with settings_edit_refuse_text_rules_long and the true number. The 1025-unit caption gets Telegram's 400 in the log, and the member path falls back to text and buttons."
    why_human: "Telegram's real caption counting (UTF-16 against 1024) could not be fetched during research. The fallback is proven only against the fake's 400 (TestRulesDegradesToTextOnClientError)."
  - test: "12-SPIKE.md check 5: run two bot replicas on one PostgreSQL. With Greet once on, a test account joins, leaves and joins again, once through each replica."
    expected: "Exactly one welcome in total. Turning Greet once off makes the next join welcome again. The page shows no count and no name."
    why_human: "TestGreetOnceConcurrentOneWelcome (16 concurrent joins give one welcome and one row) and TestGreetedMemberClaimConcurrent prove the claim on SQLite here and on PostgreSQL in CI run 106. Two live processes with a real deployment config can only be checked live."
  - test: "12-SPIKE.md check 6: set rules text, a photo and two URL buttons. Send /rules in the group, turn private rules on and open the link, then try a media-only row. Press the Rules Full preview with a phone language different from the group's."
    expected: "/rules shows one message: the photo, the text as caption and the buttons. With private rules on, the group gets the click-to-read button and the private message carries the media and buttons. A media-only row still sends. Full preview's heading is in the group's language and its header line in the presser's."
    why_human: "Real-client rendering of a captioned media message with an inline keyboard."
  - test: "12-SPIKE.md check 7 (backstop truths of 12-05, 12-10, 12-16): on Hindi, Russian and Ukrainian phones, open the Welcome page in private and look at the five switch cells. Then tap one switch five times quickly."
    expected: "No cell is cut off with an ellipsis, Hindi combining marks included. After the fast taps the page shows one consistent state, nothing is lost and no tap is retried."
    why_human: "TestSettingsLabelLengths holds a 20 rendered code-point ceiling. Truncation at phone width and Telegram's edit rate limits are client and live-API behaviour."
  - test: "12-SPIKE.md check 8: a native speaker of each of es, fr, hi, id, pt, ru and uk reads the Phase 12 keys on the real pages."
    expected: "Each string reads naturally. 'Greet once' and 'Media under text' are understood."
    why_human: "Translation quality is a human judgement."
  - test: "12-SPIKE.md check 9 and CI on head 2551fd9: run make lint with a golangci-lint built for Go 1.26, then make test and make test-postgres-integrity in CI."
    expected: "make lint is clean for the new lines and make test is green. The integrity step runs TestGreetedMemberClaimConcurrent, TestRulesMediaMigrationDefaults and TestImportGreetingsOmittedBlocks for real (not skipped) on PostgreSQL."
    why_human: "CI run 106 was green on 4dae0c5, before the review-fix pass. TestImportGreetingsOmittedBlocks (WR-02) joined the integrity list after that run, so it has passed on PostgreSQL only in the fixer's local throwaway cluster, not yet in CI. The sandbox's golangci-lint is built with Go 1.25.1 and refuses the Go 1.26.0 target. The verifier did not run make test as a whole because fuku/modules must not be run whole here. It ran the focused families instead (all green, see Behavioral Spot-Checks)."
  - test: "Owner review of the judgment-tier prohibitions of plans 12-01 to 12-16 (see 'Prohibitions' below), and of the follow-ups in 12-SPIKE.md: UI-SPEC A1 (Rules set by any part), A2 (the right of Media under text), A3 (resets leave the switches), A4 (icon-only switch cells), A12 (the Rules Full preview language), 429 as a definite refusal, the member-path text fallback that deviates from D-03, IN-02 (/setrules accepts a text too long for the stored rules media), NEW-03 (the backup import skips the editor's checks), the Delete-last race of the dropped plan 12-15, and a chat migration re-welcoming members once."
    expected: "The owner accepts the verifier's non-authoritative verdict (every prohibition holds, each backed by a passing test or a direct code read) and decides which follow-ups become their own changes."
    why_human: "An autonomous verifier never passes a judgment-tier prohibition silently. The follow-ups are product decisions the phase left open on purpose."
---

# Phase 12: Rules Media, Media Under Text and Greet Once Verification Report

**Phase Goal:** As a group owner, I want rules with media and buttons, media shown under its text, and members greeted only on their first join, so that my messages look and behave the way I set them.
**Mode:** mvp (the user story format is valid: "As a group owner, I want ..., so that ...")
**Verified:** 2026-11-05T15:23:21Z at HEAD 2551fd9
**Status:** human_needed
**Re-verification:** No, initial verification

All four ROADMAP success criteria and all three requirements (EDIT-07, EDIT-08, EDIT-09) are delivered and wired in the code. Every behaviour-dependent truth has a passing test that drives the real handlers through the hand-written Bot fake, miniredis and SQLite. The verifier ran each of these tests itself (see Behavioral Spot-Checks). The status is `human_needed` only because live checks remain. The owner chose to defer them (12-SPIKE.md), CI has not yet run on HEAD, and the owner still has to review the judgment-tier prohibitions and the follow-ups. There are no gaps.

## User Flow Coverage

User story: «As a group owner, I want rules with media and buttons, media shown under its text, and members greeted only on their first join, so that my messages look and behave the way I set them.»

| Step | Expected | Evidence | Status |
|------|----------|----------|--------|
| Open Rules in the private editor | The Rules page shows the parts text, media and buttons, each with [Set] or [Set \| Remove] | Rules parts table: `settings_edit.go:218-219` (`settingsEditRulesTextPart, settingsEditMediaPart, settingsEditButtonsPart`). The page reads the whole row through `rules.LoadRulesSettingsContext` (`settings_edit.go:227`, `fuku/db/rules/repository.go:49`), which creates nothing. Tests: `TestSettingsEditRulesParts`, `TestSettingsEditRulesMainState` | VERIFIED |
| Set Rules media and buttons by sending a message | The next private message is checked by the Welcome's predicates and stored by the Rules apply functions | `settingsInputReply.applyRules` (`settings_input_door.go:462-475`) calls `applyRulesMedia` and `applyRulesButtons` (`rules.go:199,204`). The 1024 caption rule applies both ways (`settings_edit_check.go:230,290-296`). Tests: `TestSettingsEditRulesTypedParity`, `TestSettingsEditRulesTypedRefusals`, `TestSettingsRulesCaptionParity` | VERIFIED |
| Turn Media under text on | The switch is drawn only for a photo, a video or an animation, and writes its one column | Switch cells `settings_edit.go:163-188`, `Drawn: settingsEditMediaBelowEligible` (`:394-396`) over `media.CaptionAboveEligible` (`sender.go:331`). Apply functions `greetings.go:257,263`, `rules.go:210`. Tests: `TestSettingsEditMediaBelowSwitch`, `TestSettingsEditMediaBelowEligibility`, `TestSettingsEditMediaBelowStalePress`, `TestSettingsRulesMediaBelow` | VERIFIED |
| Turn Greet once on | The Welcome page has a Greet once switch that writes `welcome_once` | `settingsEditOnceSwitch` (`settings_edit.go:189-195`), on Welcome only (`:203`), apply `applyWelcomeOnce` (`greetings.go:270`) and `SetWelcomeOnce` (`fuku/db/greetings/repository.go:548`). Tests: `TestSettingsEditOnceSwitch`, `TestSettingsDriftOnce`, `TestSettingsAuthorityOnce` | VERIFIED |
| Check with Full preview | Full preview shows the message exactly as members get it, Media under text and Rules media and buttons included, and claims nobody | `settingsEditPreviewGreeting` uses `media.SendGreetingMessage` with `CaptionAbove: st.MediaBelow` (`settings_edit_preview.go:116`). `settingsEditPreviewRules` uses `buildRulesMessage` and `sendRulesMessage` in the group's language (`:92-93`). Tests: `TestSettingsEditFullPreviewMediaBelow`, `TestSettingsEditRulesPreviewGroupLanguage`, `TestSettingsEditRulesPreviewParts`, `TestSettingsEditPreviewNeverClaims` | VERIFIED |
| Outcome: messages look and behave as set | `/rules` (group and private) shows media and buttons. Media sits under the text at join, leave, `/rules` and private rules. A member is welcomed once ever. Old messages are unchanged | Truths 1 to 4 below | VERIFIED (live client rendering deferred, see Human Verification) |

## Goal Achievement

### Observable Truths

| #   | Truth | Status | Evidence |
| --- | ----- | ------ | -------- |
| 1 | **SC1 / EDIT-08:** The owner adds one media item and URL buttons to Rules from the editor, and `/rules` shows them in the group and as private rules | VERIFIED | **Storage:** migration `20261105120000` adds `rules.file_id TEXT NOT NULL DEFAULT ''`, `media_type BIGINT NOT NULL DEFAULT 0`, `btns JSONB NOT NULL DEFAULT '[]'` and `media_below` (lines 31-34). Model fields with no `default:` tag (`fuku/db/models/rules.go`). Column-targeted writers `SetRulesMediaOnly`, `SetRulesButtonsOnly`, `SetRulesMediaBelow` and `ClearRules` (`fuku/db/rules/repository.go:134-160`, through `updateRulesColumns` with `UpdateRecordWithZeroValues`). **One builder, one sender:** `buildRulesMessage` (`rules.go:100`) and `sendRulesMessage` (`:125`) are called by `/rules` (`:372`), by the deep link (`:568`) and by Full preview (`settings_edit_preview.go:92-93`, with `degrade` false). With media the message is one media message with the text as caption and the buttons under it. Without media it is the caller's own `SendMessage` with the keyboard added (`rules.go:160-171`), so it never goes through the greeting sender. **`/clearrules`:** `clearRules` → `applyRulesClear` (`rules.go:249`) → `rules.ClearRules` empties text, media and buttons together and leaves `media_below`, `private` and `rules_btn` alone. The three Remove presses go through `settingsEditApplyRemoveRules` (`settings_edit_actions.go:227-244`). Tests pass: `TestRulesMediaAndButtonsInGroup`, `TestPrivateRulesMediaAndButtons`, `TestRulesMediaOnlyAndButtonsOnly`, `TestRulesClickToReadHasNoMedia`, `TestRulesDegradesToTextOnClientError`, `TestSettingsEditRulesRemoveParity` (subtest "three Removes equal /clearrules"), `TestSettingsEditRulesTypedParity`, `TestRulesApplyParity`. |
| 2 | **SC2 / EDIT-07:** With Media under text on, a photo, video or GIF in a welcome, goodbye or rules message appears below its text, in Full preview and in the group | VERIFIED (live rendering deferred) | **Flag:** `media.Content.CaptionAbove` becomes `ShowCaptionAboveMedia` only in `sendCaptioned` for `SendPhoto`, `SendVideo` and `SendAnimation` (`sender.go:167-187`). No other sender reads it, and a false value adds no parameter (gotgbot `omitempty`). **One predicate:** `CaptionAboveEligible` (`sender.go:331-337`). `SendGreetingMessage` ANDs the flag with it (`:359`), `sendRulesMessage` does the same (`rules.go:141`), and the page draws the switch through the same predicate (`settings_edit.go:394-396`). **Everywhere:** the join (`greetings.go:948`), the leave (`:1032`), `/welcome` and `/goodbye` with and without `noformat` (`:501,515`), `/rules` and the deep link (`rules.go:141`), and Full preview (`settings_edit_preview.go:116`, and the Rules form at `:92-93`). **GIFs:** `extractGreetingMedia` tests the animation first and stores `db.ANIMATION` (9) (`content.go:242-247`), behind `ExtractWelcome` (`:174`) and `ExtractMedia` (`:235`). Notes, filters and pins keep `extractMediaFromReply`. Tests pass: `TestSendGreetingMessageCaptionAbove`, `TestCaptionAboveEligible`, `TestGreetingsMediaBelowEverywhere`, `TestRulesMediaBelowEverywhere`, `TestSettingsEditFullPreviewMediaBelow`, `TestSettingsEditMediaBelowStoredOnIgnored`, `TestSetWelcomeGifIsAnimation`, `TestExtractGreetingMediaAnimation`, `TestExtractNoteAndFilterGifStaysDocument`. **Stated limit, judged below:** a GIF stored before Phase 12 stays a document and gets no switch until it is set again. |
| 3 | **SC3 / EDIT-09:** With Greet once on, a member is welcomed only the first time they ever join. Leaving and rejoining brings no second welcome, also across replicas | VERIFIED (two live replicas deferred) | **Record:** `greeted_members` with `PRIMARY KEY (chat_id, user_id)` (migration lines 36-41). `ClaimGreeting` is one `INSERT ... ON CONFLICT DO NOTHING` and wins only on `RowsAffected == 1` (`fuku/db/greetings/greeted.go:24-32`). It is never cached, never Redis and never `claimRecentJoinProcessing` (grep: the only callers of `greetOnceStore` are in `greetings.go:277-330`). **Claim point:** inside `SendWelcomeMessage`, after the Welcome-on check and before the render and send (`greetings.go:924-929`). The only callers are the join's `membershipDepsForJoin.Welcome` (`:1075`) and the captcha pass `finishCaptchaPass` (`captcha.go:1742`). A captcha challenge, a captcha failure and a lockdown guard ban never reach it. **Give back:** only on a definite refusal, `welcomeDefinitelyNotSent` (`:289-296`: `media.ErrNoPermission`, a 4xx other than 429 by `isTelegramClientError`, or a 429). An unknown outcome keeps the claim (`:316-324`). A panic before the send releases it (`:899-908`). **Fail closed:** a claim error sends nothing (`:301-313`), and a greetings read error sends and claims nothing (`:911-917`, WR-01). Tests pass: `TestGreetOnceFirstJoinOnly`, `TestGreetOnceConcurrentOneWelcome` (16 concurrent joins give 1 welcome and 1 row), `TestGreetOnceCaptchaPathAndFailure`, `TestGreetOnceLockdownBanNoRecord`, `TestGreetOnceWithoutRedis`, `TestGreetOnceWelcomeOffNoClaim`, `TestGreetOnceOffNoRecord`, `TestGreetOnceDefiniteRefusalReleases` (400, 403, 429, restricted chat), `TestGreetOnceUnknownOutcomeKeeps`, `TestGreetOnceClaimErrorFailsClosed`, `TestGreetOnceReadErrorFailsClosed`, `TestGreetOncePanicBeforeSendReleasesClaim`, `TestGreetOncePanicAfterSendKeepsClaim`, `TestGreetedMemberClaim`, `TestGreetedMemberClaimConcurrent`, `TestSettingsEditPreviewNeverClaims`. |
| 4 | **SC4:** Rules, welcomes and goodbyes saved before this phase look exactly as they did until someone changes them | VERIFIED | **Goldens:** `TestRulesLegacyRowSendsTodaysRequest` and `TestLegacyGreetingsRequestsUnchanged` (`fuku/modules/rules_legacy_request_test.go`) compare the JSON of the exact request parameters with literal goldens. They were committed in 5b8978c and 26c5b19 (11:01 and 11:03), before the first change to `rules.go`, `sender.go` or `greetings.go` (222029d, 11:32). Only those two commits ever touched the file, so the goldens were never re-recorded. Both pass at HEAD. They cover `/rules` with private rules off, on, as a reply and with no rules, the deep link with rules set and not set, a text welcome, a photo welcome, a video goodbye, `/welcome` and a document GIF, and they assert no legacy request carries `show_caption_above_media`. **Zero defaults:** every new column is `NOT NULL DEFAULT` its zero value (migration lines 27-34). No new boolean has a `default:` tag (`models/greetings.go` MediaBelow and Once, `models/rules.go` MediaBelow). **Old exports:** `TestOldGreetingsExportDecodesToZeros` strips the new keys from an export and imports it over a chat that had them all set: every new column ends at zero. `importGreetings` fills an omitted block before the write (`backup.go:701-709`, WR-02, `TestImportGreetingsOmittedBlocks`). **Old GIFs:** type 3 still sends `sendDocument` (golden) and reads File or GIF with no switch (`settings_edit.go:330,433-435`). |
| 5 | **Compatibility of storage (D-11, D-13, D-14):** one append-only migration in one transaction. `greeted_members` is never in backup, export, import or reset, and is in the harness AutoMigrate lists | VERIFIED | `20261105120000` is later than `20261104120000` and has no BEGIN, COMMIT, CONCURRENTLY or ADD CONSTRAINT (`TestRulesMediaMigrationStatements` passes). Neither `runner.go` nor `migrate_psql.sh` changed. `greeted_members` appears in no production file outside `greeted.go` and `greeted_member.go` (grep). `TestGreetedMembersStayOutOfBackup` passes: an export holds only backup modules, a `greeted_members` module is refused, an import writes no row and a reset keeps both rows. `GreetedMember` is in `fuku/modules/test_harness_test.go:377`, `fuku/db/backup/testmain_test.go:47` and `fuku/db/greetings/testmain_test.go:12`. `TestRulesMediaMigrationDefaults` is PostgreSQL only and skips here. It is in the integrity list and ran in CI run 106. |
| 6 | **Forged controls are refused before any I/O (12-06):** a `tg` or `st` press for a page or control that does not exist, or a switch value other than `0` or `1`, is an invalid request before any `getChatMember` | VERIFIED | `settingsControlFieldsValid` (`settings_menu.go:301-311`) runs in `settingsCheckFields`, step 1 (`:267`). `settingsControl.Validate` (`settings_pages.go:385`). Every editor switch declares `settingsEditSwitchValue` (`settings_edit.go:704-706,723`). Greet once on Goodbye or Rules is not a registered control, so it is refused. `TestSettingsControlValidate` counts the fake's lookups. |
| 7 | **Editor limits (D-15):** a private page has at most 8 rows and a group-menu page at most 9. Every button is at most 64 bytes, every screen at most 3800 UTF-16 units in all 8 languages. No screen names the Staff Group or shows anything from `greeted_members`. Every new string is in all 8 locales | VERIFIED | The switch grid is two columns (`settings_edit.go:501-512`). Row assertions: `TestSettingsEditSwitchGrid` (exactly 8 private and 9 group with every part set, `settings_edit_switch_test.go:185-208`) and `TestSettingsEditPages` (at most 8 and 9 for all three pages, `settings_edit_test.go:352-356`). Bytes: `TestSettingsEditCallbackBytes`, `TestSettingsEditRulesCallbackParts`. Lengths: `TestSettingsEditScreensFitEveryLocale`, `TestSettingsMainPageFitsEveryLocale`. Privacy: `TestSettingsStaffPrivacy` covers the Phase 12 screens in 8 languages, a group menu and a non-creator private run. Labels: `TestSettingsLabelLengths` (the 5 switch cells at most 20 rendered code points), `TestSettingsEditLocaleTokens`. The 13 new keys and the 4 renamed `settings_edit_line_*` keys exist in all 8 files. Each file has 199 `settings_` keys. `make check-translations` passes. Every `settings_`, `rules_`, `common_` and `chat_status_` key named in the Phase 12 production files exists in `en.yml` (script check). |

**Score:** 7/7 truths verified (0 present but behavior-unverified). Requirements: 3/3 satisfied. Total 10/10. Decision gate 17/17 honored, and dropped plan 12-15 confirmed independent (below).

### EDIT-07 and the old-GIF limit: does it satisfy the requirement?

EDIT-07 says "Media under text" shows a photo, video or GIF below its text. The owner's decisions define how far that goes:

- D-07: the switch only works for a photo, a video or a GIF, and is hidden otherwise.
- D-08: a GIF saved from this phase on is stored as an animation and sent with `sendAnimation`. GIFs saved earlier stay documents, show exactly as before, and get no switch until the owner sets the GIF again.

The code does exactly that. The verifier also read the limit in AGENTS.md (line 1701, plus the media-types bullet at 1572-1585), in 12-14-SUMMARY.md (line 113) and in 12-SPIKE.md (lines 6-8, with a live check), and the sentence is the same in all three.

**Verdict:** EDIT-07 is satisfied as D-07 and D-08 define it. Every GIF the owner sets from now on (by the editor, `/setwelcome` or `/setgoodbye`) gets the switch and is sent below its text. A GIF saved before the phase has no caption-placement method behind it (`sendDocument` cannot take `show_caption_above_media`). Converting it silently would break SC4 ("look exactly as they did until someone changes them"). Setting it again is the "change" SC4 allows. The limit therefore follows from the owner's decisions plus SC4. It is not a missing piece of code. The owner should still confirm it live (human item 3).

### Dropped plan 12-15

`12-15-SUMMARY.md` does not exist and nothing depends on 12-15. Only 12-15's own frontmatter names `depends_on: ["12-03", "12-09"]`. 12-16's `depends_on` lists 12-01 to 12-14 only. The only "12-15" in 12-05-SUMMARY.md is the threat ID T-12-15. No `SwapClean*` exists in `fuku/` (grep empty). The Makefile integrity list leaves out `TestSwapCleanMsgIDConcurrent`. AGENTS.md:439 records "Known Delete-last race (plan 12-15 not shipped)". So 12-16 followed the absent branch, as its truth requires. `verify.artifacts` on 12-15-PLAN.md reports its two artifacts missing, which is the expected result for a dropped plan and not a gap.

### Required Artifacts

| Artifact | Expected | Status | Details |
| -------- | -------- | ------ | ------- |
| `migrations/20261105120000_add_rules_media_and_greet_once.sql` | 7 columns and `greeted_members` | VERIFIED | Read in full. Idempotent, no transaction control |
| `fuku/db/models/greeted_member.go`, `greetings.go`, `rules.go` | Models, `HasContent`, no `default:` tags on the new booleans | VERIFIED | `TestHasContent` passes |
| `fuku/db/greetings/greeted.go` | `ClaimGreeting`, `ReleaseGreeting` | VERIFIED | Wired from `greetOnceStore` (`greetings.go:277-283`) |
| `fuku/db/greetings/repository.go` | `SetWelcomeMediaBelow`, `SetGoodbyeMediaBelow`, `SetWelcomeOnce`, `LoadGreetingSettings` | VERIFIED | Writers go through `upsertGreetingSettings`, which deletes `greetings:<chat>` |
| `fuku/db/rules/repository.go` | `LoadRulesSettingsContext` and 4 writers | VERIFIED | Rules reads are uncached (`GetChatRulesInfo` is a plain `GetRecord`), so no cache key applies |
| `fuku/db/lang/repository.go` | `GetGroupLanguage` | VERIFIED | Used by the Rules Full preview (`settings_edit_preview.go:92`) |
| `fuku/utils/media/sender.go` | ANIMATION case, `CaptionAbove`, `CaptionAboveEligible`, `SendGreetingMessage` | VERIFIED | `SendGreeting` is kept as the flag-off wrapper |
| `fuku/utils/content/content.go` | `extractGreetingMedia` | VERIFIED | Greeting paths only |
| `fuku/modules/rules.go` | `buildRulesMessage`, `sendRulesMessage`, Rules apply functions | VERIFIED | Read in full (diff) |
| `fuku/modules/greetings.go` | Media under text and Greet once apply functions, claim and give-back | VERIFIED | Read in full (diff) |
| `fuku/modules/settings_edit*.go`, `settings_input_door.go`, `settings_menu.go`, `settings_pages.go` | Switch table, controls `u` and `o`, Rules parts, the caption rule, previews | VERIFIED | Read in full (`settings_edit.go:130-729`) and by diff |
| Tests named in the plans' artifacts | `rules_legacy_request_test.go`, `greet_once_test.go`, `greetings_media_below_test.go`, `greetings_gif_test.go`, `rules_media_send_test.go`, `settings_control_validate_test.go`, `settings_edit_switch_test.go`, `settings_edit_rules_test.go`, `settings_input_rules_test.go`, `greeted_test.go`, `rules_media_migration_test.go`, `greeted_backup_test.go` | VERIFIED | `verify.artifacts` passes for every plan except the dropped 12-15 |

### Key Link Verification

The `verify.key-links` helper could not parse most `from` fields, because they name a symbol after the path. The verifier grepped every pattern by hand. All 25 links of plans 12-01 to 12-14 and 12-16 are present.

| From | To | Via | Status | Details |
| ---- | -- | --- | ------ | ------- |
| `/rules`, deep link | `sendRulesMessage` | one sender | WIRED | `rules.go:372,568` |
| `sendRulesMessage` (media) | `media.Send` with `CaptionAbove` | `CaptionAboveEligible` | WIRED | `rules.go:141` |
| `{rules}` button gate | `HasContent` | `!rulesDb.HasContent()` | WIRED | `formatting.go:319` |
| join, leave, `/welcome` | `SendGreetingMessage` | `CaptionAbove: MediaBelow` | WIRED | `greetings.go:501,515,948,1032` |
| `applyWelcomeMediaBelow` | `greetings.SetWelcomeMediaBelow` | `greetingWrites.WelcomeMediaBelow` | WIRED | `greetings.go:258` |
| `SendWelcomeMessage` | `ClaimGreeting`, `ReleaseGreeting` | `greetOnceStore` | WIRED | `greetings.go:305,320,328` |
| `finishCaptchaPass`, `membershipDepsForJoin` | `SendWelcomeMessage` | the one claim holder | WIRED | `captcha.go:1742`, `greetings.go:1075` |
| control `o` | `applyWelcomeOnce` | switch table | WIRED | `settings_edit.go:194` |
| control `u` | `applyWelcomeMediaBelow`, `applyGoodbyeMediaBelow`, `applyRulesMediaBelow` | switch table | WIRED | `settings_edit.go:169,177,187` |
| page, stale press | `media.CaptionAboveEligible` | `settingsEditMediaBelowEligible` | WIRED | `settings_edit.go:395` |
| controls | `settingsControl.Validate` | `settingsEditSwitchValue` | WIRED | `settings_edit.go:723` |
| `settingsCheckFields` | `settingsControlFieldsValid` | step 1 | WIRED | `settings_menu.go:267` |
| Rules Remove | `applyRulesText`, `applyRulesMedia`, `applyRulesButtons` | part switch | WIRED | `settings_edit_actions.go:227-244` |
| `/clearrules` | `rules.ClearRules` | `rulesWrites.ClearRules` | WIRED | `rules.go:217-218,249` |
| typed Rules reply | `applyRulesMedia`, `applyRulesButtons` | `applyRules` | WIRED | `settings_input_door.go:462-475` |
| Rules text and media checks | `settingsEditRulesLongest` | one measure | WIRED | `settings_edit_check.go:226,293` |
| Rules Full preview | `buildRulesMessage`, `lang.GetGroupLanguage` | group form, `degrade` false | WIRED | `settings_edit_preview.go:92-93` |
| Greeting Full preview | `SendGreetingMessage` | `CaptionAbove: st.MediaBelow` | WIRED | `settings_edit_preview.go:116` |
| `GreetingMessage` extraction | `extractGreetingMedia` | `ExtractWelcome`, `ExtractMedia` | WIRED | `content.go:174,236` |
| `Send` | `sendCaptioned` (`sendAnimation`) | `case db.ANIMATION` | WIRED | `sender.go:103` |
| `ClaimGreeting` | primary key | `OnConflict{DoNothing: true}`, `RowsAffected == 1` | WIRED | `greeted.go:26,31` |
| `GetGroupLanguage` | `getGroupLanguageContext` | cached `chat_lang` read | WIRED | `lang/repository.go:60-62` |
| model tags | migration columns | `column:media_below`, `json:"media_below"` | WIRED | `models/rules.go`, `models/greetings.go` |
| label test | locale switch keys | rendered check | WIRED | `settings_locale_test.go` (12 matches) |
| privacy test | Phase 12 screens | `settings_edit_below_unavailable`, `settingsEditControlOnce` | WIRED | `verify.key-links` passed (12-16) |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
| -------- | ------------- | ------ | ------------------ | ------ |
| Rules page and Main line | `settingsEditState` (text, media, buttons, MediaBelow) | `rules.LoadRulesSettingsContext` (fresh DB read, creates nothing) | Yes | FLOWING |
| Welcome and Goodbye switches | `st.MediaBelow`, `st.Once` | `greetings.LoadSettingsContext` and the apply read-back | Yes | FLOWING |
| `/rules`, deep link | `rulesMessage` | `rules.GetChatRulesInfo` row → `buildRulesMessage` | Yes | FLOWING |
| Join, leave | `CaptionAbove` | the stored `welcome_media_below` and `goodbye_media_below` (`greetPrefs`) | Yes | FLOWING |
| Greet once | claim | `greeted_members` conditional insert | Yes | FLOWING |

### Behavioral Spot-Checks

All were run by the verifier at HEAD 2551fd9 in this sandbox.

| Behavior | Command | Result | Status |
| -------- | ------- | ------ | ------ |
| Compile | `CGO_ENABLED=0 go build ./...` | exit 0 | PASS |
| Vet | `go vet -tags testtools ./fuku/modules ./fuku/db/greetings ./fuku/db/rules ./fuku/db/lang ./fuku/db/backup ./fuku/db/models ./fuku/db/migrations ./fuku/utils/content ./fuku/utils/media ./fuku/utils/formatting ./fuku/i18n` | exit 0 | PASS |
| Format | `gofmt -l` on every Go file the phase changed | none | PASS |
| Translations | `make check-translations` | "All translations are present!" (exit 0) | PASS |
| Generated docs | `make generate-docs` then `make check-docs` | "No drift detected" (exit 0), tree still clean | PASS |
| Dependencies untouched | `git diff --exit-code 72a692b -- go.mod go.sum` | no diff | PASS |
| Repositories and helpers | `go test -tags testtools -race -count=1 ./fuku/db/greetings ./fuku/db/rules ./fuku/db/lang ./fuku/db/backup ./fuku/utils/content ./fuku/utils/media ./fuku/utils/formatting` | all 7 ok | PASS |
| Named db and helper tests, verbose | `-run '^(TestOldGreetingsExportDecodesToZeros\|TestGreetedMembersStayOutOfBackup\|TestImportGreetingsOmittedBlocks\|TestGreetedMemberClaim\|TestGreetedMemberClaimConcurrent\|TestLoadGreetingSettings\|TestRulesMediaMigration\|TestHasContent\|TestGetGroupLanguage\|TestSendGreetingMessageCaptionAbove\|TestExtract\|TestCaptionAbove\|TestAllModulesRoundTripEveryMeaningfulField)'` over 8 packages | every one PASS. `TestRulesMediaMigrationDefaults` SKIP (PostgreSQL only, by design; its database-free half `TestRulesMediaMigrationStatements` PASS) | PASS |
| Goldens, Greet once, Media under text, Rules sends, joins, captcha, lockdown | `go test -tags testtools -race -count=1 -v -run '^(TestRulesLegacyRowSendsTodaysRequest\|TestLegacyGreetingsRequestsUnchanged\|TestGreetOnce\|TestSetWelcomeGifIsAnimation\|TestGreetingsMediaBelow\|TestRulesApply\|TestBuildRulesText\|TestRulesMedia\|TestPrivateRulesMediaAndButtons\|TestRulesClickToReadHasNoMedia\|TestRulesDegradesToTextOnClientError\|TestRenderGreetingMatchesJoin\|TestMemberJoinAndLeaveSendConfiguredGreetings\|TestGreeting\|TestCaptcha\|TestLockdown\|TestProcessSingleNewMember\|TestLeftMember)' ./fuku/modules` | ok (88.5 s), 176 PASS, 0 FAIL, 0 SKIP | PASS |
| Settings family (editor, switches, Rules parts, previews, privacy, limits, gate) | `go test -tags testtools -race -count=1 -run '^(TestSettings\|TestLoadSettings)' ./fuku/modules ./fuku/i18n` | `fuku/modules` ok (268.4 s), `fuku/i18n` ok (5.1 s) | PASS |
| Decision coverage | `gsd-tools query check.decision-coverage-verify` | 17/17 honored | PASS |

Not run here: `make lint` (the sandbox golangci-lint 2.5.0 is built with go1.25.1 and refuses the Go 1.26.0 target), `make test` as a whole (fuku/modules must not be run whole here), and `make test-postgres-integrity` (no PostgreSQL server is running, and the verifier starts no service). These are human item 9.

### Probe Execution

Step 7c: SKIPPED. The phase declares no probe scripts.

### Test Quality Audit

| Test File | Linked Req | Active | Skipped | Circular | Assertion Level | Verdict |
|-----------|-----------|--------|---------|----------|-----------------|---------|
| `rules_legacy_request_test.go` | SC4 | 2 | 0 | no: goldens recorded on the pre-phase code and committed before any change | Value (exact request JSON) | VALID |
| `greet_once_test.go` | EDIT-09 | 13 | 0 | no | Behavioral (sends counted, rows counted) | VALID |
| `rules_media_send_test.go` | EDIT-08, EDIT-07 | 6 | 0 | no | Value and behavioral | VALID |
| `greetings_media_below_test.go`, `greetings_gif_test.go` | EDIT-07 | 3 | 0 | no | Value (request parameters, stored type) | VALID |
| `settings_edit_switch_test.go`, `settings_edit_rules_test.go`, `settings_input_rules_test.go`, `settings_edit_preview_test.go` | EDIT-07/08/09 | all | 0 | no | Behavioral (press, stored row, redraw, sends) | VALID |
| `greeted_test.go` | EDIT-09 | 2 | 0 | no | Behavioral (concurrent claims) | VALID |
| `rules_media_migration_test.go` | SC4 | 2 | 1 (PostgreSQL only, by design) | no | Value | VALID (PG half in CI) |
| `greeted_backup_test.go`, `greetings_omitted_test.go` | D-13, SC4 | 3 | 0 | no | Value | VALID |

**Disabled tests on requirements:** 0. The one skip is PostgreSQL-only by design, it is in the CI integrity list, and its database-free half runs. **Circular patterns detected:** 0. **Insufficient assertions:** 0.

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
| ----------- | ----------- | ----------- | ------ | -------- |
| EDIT-07 | 12-02/04/05/06/08/10/11/12/14/16 | "Media under text" shows a photo, video or GIF below its text | SATISFIED (as D-07 and D-08 define it; live rendering deferred) | Truth 2 and the EDIT-07 judgement above |
| EDIT-08 | 12-01/02/03/05/07/12/13/14/16 | Rules gain media and URL buttons, and `/rules` (group and private) shows them | SATISFIED | Truth 1 |
| EDIT-09 | 12-02/03/09/10/14/16 | Greet once: welcomed only the first time ever, also across replicas | SATISFIED (two live replicas deferred) | Truth 3 |

REQUIREMENTS.md marks all three `[x]` and Complete (lines 30-32, 149-151), which matches the code. No orphaned requirement maps to Phase 12. ROADMAP shows 15/16 plans with 12-15 droppable. Its Phase 12 checkbox (`[ ]`, line 74) waits for this verification (Info, orchestrator bookkeeping).

### Decisions D-01..D-17 against the code

| Decision | Holds | Evidence |
| -------- | ----- | -------- |
| D-01 Rules media and buttons as parts of the parts table, the Welcome's check | Yes | `settings_edit.go:219`, `settings_edit_check.go:270-300` (shared predicates), `TestSettingsEditRulesTypedParity` |
| D-02 `/rules`, private rules and Full preview through one builder and sender; media only from the editor | Yes | Truth 1. `/setrules` unchanged apart from going through `applyRulesText` |
| D-03 one message, 1024 caption, refused both ways in the editor | Yes, with the documented member-path fallback | `settings_edit_check.go:228-231,290-296`, `TestSettingsRulesCaptionParity`. The fallback (`rules.go:149-154`) is a deliberate, recorded deviation (12-07, AGENTS.md, 12-SPIKE.md) |
| D-04 Remove clears one part, `/clearrules` all three | Yes | `TestSettingsEditRulesRemoveParity` |
| D-05 old rules rows byte for byte | Yes | Golden `TestRulesLegacyRowSendsTodaysRequest` |
| D-06 three new booleans, no `default:` tag, `show_caption_above_media` everywhere | Yes | Truth 2 |
| D-07 photo, video or GIF only; stored on kept but ignored | Yes | `TestSettingsEditMediaBelowStoredOnIgnored`, `TestSettingsEditMediaBelowStalePress` |
| D-08 new GIFs as animation, old ones stay documents | Yes | `TestSetWelcomeGifIsAnimation`, golden document GIF |
| D-09 no new dry run for the switch | Yes | The switch press calls only the apply function (`settings_edit.go:643-677`) |
| D-10 Greet once on Welcome only, the `/welcome` right, drift and authority tests | Yes | `settingsRightAdmin` (`:192`), `TestSettingsDriftOnce`, `TestSettingsAuthorityOnce` |
| D-11 `greeted_members`, conditional insert, given back only on a definite refusal | Yes | Truth 3 |
| D-12 claimed only where really welcomed, never Redis | Yes | Truth 3, `TestGreetOnceWithoutRedis` |
| D-13 out of backup, fresh reads, harness lists, columns in backup JSON | Yes | Truth 5, `TestOldGreetingsExportDecodesToZeros` |
| D-14 one migration, zero defaults, appliers unchanged | Yes | Truth 5 |
| D-15 pages read side-effect free, row and size limits | Yes | Truth 7 |
| D-16 previews show media under text and Rules media, only to the presser | Yes | `TestSettingsEditFullPreviewMediaBelow`, `TestSettingsEditRulesPreviewParts`, target `p.query.From.Id` |
| D-17 column-targeted writers beside the commands, cache invalidated | Yes | `upsertGreetingSettings` (deletes `greetings:<chat>`). Rules reads are uncached |

Decision coverage gate: 17/17 honored ("All trackable CONTEXT.md decisions are honored by shipped artifacts").

### Prohibitions (judgment tier, non-authoritative verdict, flagged for owner review)

Every plan prohibition is `verification: judgment`. The verdicts below are the verifier's, non-authoritative, and listed among the human items.

| Plan | Prohibition | Verifier verdict | Evidence |
| ---- | ----------- | ---------------- | -------- |
| 12-01 | Characterization committed before any change, goldens never re-recorded | holds | git log: 5b8978c and 26c5b19 precede 222029d. Only those two commits touch the file |
| 12-02 | No `default:` tag on a new boolean. `greeted_members` never in export, import or reset | holds | Model diff, `TestGreetedMembersStayOutOfBackup` |
| 12-03 | Greet-once record never Redis, `claimRecentJoinProcessing` or cached | holds | `greeted.go` uses `db.DB` only, `TestGreetOnceWithoutRedis` |
| 12-04 | No caption flag on other kinds; note, filter and pin GIFs unchanged | holds | `sendCaptioned` only, `TestExtractNoteAndFilterGifStaysDocument` |
| 12-05 | No new string carries a title, a name, stored text, a link or the Staff Group | holds | `TestSettingsEditLocaleTokens`, `TestSettingsStaffPrivacy` |
| 12-06 | A forged control or value costs no lookup or write | holds | `TestSettingsControlValidate` |
| 12-07 | The text-only branch never goes through `SendGreeting`, goldens never re-recorded | holds | `sendRulesText` (`rules.go:160-171`), goldens green |
| 12-08 | No old greeting request changes; a stored on never reaches an ineligible send | holds | Goldens, `SendGreetingMessage` ANDs with the predicate |
| 12-09 | No claim before the Welcome-on check, in a preview, for a captcha-failed or lockdown-banned joiner, or through Redis; an unknown outcome never gives it back | holds | Truth 3 tests |
| 12-10 | Welcome page and Main never read `greeted_members`; Greet once never on Goodbye or Rules | holds | grep (no reader), switch tables, `TestSettingsDriftOnce` |
| 12-11 | Never draws Media under text for an ineligible kind; a stale press never writes | holds | `TestSettingsEditMediaBelowEligibility`, `TestSettingsEditMediaBelowStalePress` |
| 12-12 | No Rules screen echoes text, a button, a link or a file ID; no Remove or `/clearrules` touches Media under text | holds | State words only (`settings_edit.go:364-389`), `ClearRules` columns |
| 12-13 | No Rules-only media list; a Rules refusal with media never uses the filled-text wording | holds | `settingsEditMediaCaptionRefusal` shares `content.ExtractMedia` and uses its own key |
| 12-14 | A preview never sends to the group, never calls `SendWelcomeMessage` or `ClaimGreeting`, never writes | holds | `TestSettingsEditPreviewNeverClaims`, target `query.From.Id` |
| 12-15 | Droppable, nothing depends on it | holds | See "Dropped plan 12-15" |
| 12-16 | No Phase 12 screen names the Staff Group or shows `greeted_members` data | holds | `TestSettingsStaffPrivacy` |

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
| ---- | ---- | ------- | -------- | ------ |
| (phase diff, non-planning) | - | No `TODO`, `FIXME`, `XXX`, `TBD`, `HACK` or placeholder | - | none |
| (phase diff, production) | - | The only discarded values are `tr.GetString` results (the existing translator pattern). No DB error is discarded on a state-changing path | - | none |
| `fuku/db/rules/repository.go` | 63 | `LoadRulesContext` has no production caller any more (the page reads `LoadRulesSettingsContext`). Plan 12-03 kept it on purpose as a wrapper, and tests cover it | Info | Dead-ish API, harmless |
| `fuku/modules/greetings.go` | 911-917 | WR-01 fix: any greetings read error now sends no welcome, also in a group with Greet once off. Before the phase the defaults were sent | Info | Error path only. Intentional, documented in AGENTS.md:967 |
| `fuku/utils/formatting/formatting.go` | 319 | The `{rules}` button gate is now `HasContent` (trimmed). A rules text that is only whitespace no longer adds the button | Info | Practically unreachable (`/setrules` stores converted text; Telegram trims messages). Recorded in 12-SPIKE.md follow-ups |
| `.planning/.../12-SPIKE.md` | follow-ups | "A panic inside `renderGreeting` after the greet-once claim: the claim is kept" is stale: IN-01 (3ffb87c) now releases it, as AGENTS.md:972-974 says | Info | Documentation only |
| `AGENTS.md` | 1701 | The EDIT-07 limit sentence sits inside the editor's rolling-deploy caveats paragraph, between two unrelated sentences | Info | Readability only |

### Prior Audits (used as evidence, spot-checked, not redone)

- **Code review:** 0 critical. WR-01, WR-02 and IN-01 are fixed. The verifier saw each fix in the code: `LoadGreetingSettings` plus the early return at `greetings.go:911-917`, `importGreetings` at `backup.go:701-709`, and `releaseUnsentWelcome` at `greetings.go:899-908`. Each fix's test passes here: `TestGreetOnceReadErrorFailsClosed`, `TestImportGreetingsOmittedBlocks` (SQLite) and `TestGreetOncePanicBeforeSendReleasesClaim`. IN-02, IN-03 and NEW-03 are deferred and listed for the owner.
- **Security:** status verified, `threats_open: 0`.
- **Validation:** `validated`, `nyquist_compliant: true`.

### Human Verification Required

See the frontmatter list. In short:

- 12-SPIKE.md live checks 1 to 8: caption placement on four clients, an old client, `sendAnimation` and the old-GIF limit, Telegram's 1025-unit caption and the member fallback, Greet once on two live replicas, `/rules` with media and buttons on real clients, hi/ru/uk cell truncation and fast taps, and a native-speaker read.
- CI on HEAD (check 9): `make lint` with a Go 1.26 golangci-lint, `make test`, and `make test-postgres-integrity` with `TestImportGreetingsOmittedBlocks` running on PostgreSQL in CI for the first time.
- The owner's review of the judgment-tier prohibitions and of the follow-ups: A1, A2, A3, A4, A12, the 429 rule, the D-03 member fallback, IN-02, NEW-03, the Delete-last race, and chat migration.

### Gaps Summary

No gaps. Every ROADMAP success criterion, every requirement, every decision D-01 to D-17 and every plan must-have is delivered and wired in the code:

- Rules media and buttons are stored in their own columns. `/rules`, private rules and Full preview build them through one builder and one sender. `/clearrules` and the three Removes store the same row.
- Media under text is one predicate shared by the page, the stale press and every sender. It is sent at the join, the leave, `/welcome`, `/rules`, private rules and Full preview.
- Greet once is a database conditional insert at the one welcome function. It is given back only on a definite refusal, and it is out of backup.
- Old rows send byte-identical requests (literal goldens recorded before any change).
- The editor stays within 8 and 9 rows, 64 bytes, 3800 UTF-16 units and 8 locales, and names no Staff Group.

The only point that touches a requirement's wording is the old-GIF limit of EDIT-07. It follows from the owner's D-07 and D-08 together with SC4, so it is a stated limit, not a gap. It still needs a live confirmation (human item 3). The rest of the open work is the live checks the owner deferred (as in Phases 10 and 11) and the first CI run on HEAD.

---

_Verified: 2026-11-05T15:23:21Z_
_Verifier: Claude (gsd-verifier)_
