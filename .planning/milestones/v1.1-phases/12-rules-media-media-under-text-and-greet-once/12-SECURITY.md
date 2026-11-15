---
phase: "12"
slug: "rules-media-media-under-text-and-greet-once"
status: verified
# threats_open = count of OPEN threats at or above workflow.security_block_on severity (the blocking gate)
threats_open: 0
asvs_level: 1
register_authored_at_plan_time: true
created: "2026-11-05"
---

# Phase 12 — Security

> Per-phase security contract: threat register, accepted risks, and audit trail.
> Audited by `gsd-security-auditor` against the code at `4dae0c5` (all 16 plan slots executed, plan 12-15 dropped); every
> mitigation was read in the code and the security-relevant tests were run, a throwaway PostgreSQL 16 cluster included
> (migration chain, the claim tests, the backup round trip). The review-fix pass that followed (`4dae0c5..5ebf0ef`)
> moved lines in `fuku/modules/greetings.go`, `fuku/db/greetings/repository.go` and `fuku/db/backup/backup.go` and
> changed behaviour only where the New Threats section says so (NEW-01 and NEW-02 are fixed there). Evidence below is
> `file:line` at `4dae0c5` unless a path is given. Short names: `greet` = `fuku/modules/greetings.go`, `rulesm` =
> `fuku/modules/rules.go`, `edit`, `act`, `door`, `chk`, `prev`, `menu` = `fuku/modules/settings_edit.go`,
> `settings_edit_actions.go`, `settings_input_door.go`, `settings_edit_check.go`, `settings_edit_preview.go`,
> `settings_menu.go`; `sender` = `fuku/utils/media/sender.go`, `content` = `fuku/utils/content/content.go`, `greeted` =
> `fuku/db/greetings/greeted.go`, `grepo` = `fuku/db/greetings/repository.go`, `rrepo` = `fuku/db/rules/repository.go`,
> `bk` = `fuku/db/backup/backup.go`, `mig` = `migrations/20261105120000_add_rules_media_and_greet_once.sql`.

---

## Trust Boundaries

| # | Boundary | Data crossing |
|---|----------|---------------|
| 1 | Telegram user → `settings\|v1` callbacks (new controls `u` and `o`, Rules parts `m` and `b`, value `v`) | page, control, part, value, chat or user ID (not authenticated) |
| 2 | Owner's private message → stored rules (typed-reply door) | media file ID, button lines, album items |
| 3 | Joining user → welcome path (Greet once), across replicas | `(chat_id, user_id)` claim |
| 4 | Stored rules or greetings row → every member (`/rules`, deep link, join, leave) | file ID, type, caption, buttons, Media under text flag |
| 5 | Preview → the presser's own private chat | rendered message |
| 6 | Backup file → database (owner-only import) | `greetings` and `rules` rows, including the new columns |
| 7 | Deploy → PostgreSQL (`AUTO_MIGRATE=true`) | migration 20261105120000 |
| 8 | Group menu → every group member | page state words |
| 9 | Process logs | IDs, codes and errors only |

---

## Threat Register

53 entries: T-12-01 to T-12-52 plus T-12-SC, which every plan registers. Plan 12-15 was dropped (no `12-15-SUMMARY.md`,
no swap function), which decides T-12-48 and T-12-49.

| Threat ID | Plan | Cat | Sev | Disposition | Mitigation (evidence, and the test that proves it) | Status |
|---|---|---|---|---|---|---|
| T-12-01 | 01 | T | med | mitigate | `rules_legacy_request_test.go` goldens pass at HEAD: `TestRulesLegacyRowSendsTodaysRequest`, `TestLegacyGreetingsRequestsUnchanged` | CLOSED |
| T-12-02 | 01 | R | low | mitigate | The golden commits (`5b8978c`, `26c5b19`) precede the first code change in each file; the golden file has no later commit | CLOSED |
| T-12-03 | 02 | T | high | mitigate | `mig`: seven `ADD COLUMN IF NOT EXISTS ... NOT NULL DEFAULT <zero>`, `CREATE TABLE IF NOT EXISTS greeted_members` with `PRIMARY KEY (chat_id, user_id)`; no BEGIN, COMMIT, ADD CONSTRAINT or CONCURRENTLY; sorts after `20261104120000`; `runner.go` and `migrate_psql.sh` untouched. `TestRulesMediaMigrationStatements`; `TestRepositoryMigrationChain` and `TestRulesMediaMigrationDefaults` run on PostgreSQL 16 | CLOSED |
| T-12-04 | 02 | I | med | mitigate | `fuku/db/backup/types.go:115` `AllExportableModules` has no greeted module; nothing in backup references `GreetedMember`. `TestGreetedMembersStayOutOfBackup` (SQLite; see NEW-04 for PostgreSQL) | CLOSED |
| T-12-05 | 02 | T | med | mitigate | `models/greetings.go` and `models/rules.go`: column tags only, no `default:` on any new boolean | CLOSED |
| T-12-06 | 02 | D | low | mitigate | `mig` NOT NULL DEFAULT zero; `models/types.go` turns a nil `ButtonArray` into `"[]"`. `TestOldGreetingsExportDecodesToZeros` (SQLite and PostgreSQL) | CLOSED |
| T-12-07 | 03 | T | high | mitigate | `greeted:26` `OnConflict{DoNothing}`, `greeted:31` requires `RowsAffected == 1`, behind the primary key. `TestGreetedMemberClaimConcurrent` passes on SQLite and PostgreSQL 16 and is in the `Makefile` integrity list with `./fuku/db/greetings` | CLOSED |
| T-12-08 | 03 | D | low | mitigate | `greeted:37-44`: releasing an absent row returns nil. `TestGreetedMemberClaim` | CLOSED |
| T-12-09 | 03 | I | low | mitigate | `fuku/db/lang/repository.go:60` cached read; not found gives `uk` and creates nothing. `TestGetGroupLanguage` | CLOSED |
| T-12-10 | 03 | T | med | mitigate | `grepo:511,521,532` go through `upsertGreetingSettings`, which deletes the cache key (`grepo:236`); `rrepo:121-160` write column maps (rules reads are uncached). Both `editor_repository_test.go` pass | CLOSED |
| T-12-11 | 04 | T | med | mitigate | `content:242` `extractGreetingMedia` sits only behind `ExtractWelcome` and `ExtractMedia`; `content:135` still stores a GIF as a document for notes, filters and pins; `pins.go` unchanged since `bd2f794`. `TestExtractNoteAndFilterGifStaysDocument`, `TestExtractMediaFromReply` | CLOSED |
| T-12-12 | 04 | T | med | mitigate | Only `sender:181-185` (`sendCaptioned`) sets `ShowCaptionAboveMedia`; `sender:359` ANDs the flag with `CaptionAboveEligible` (`:331`); `rulesm:140` does the same. `TestSendGreetingMessageCaptionAbove` | CLOSED |
| T-12-13 | 04 | D | low | accept | Rolling deploy: a replica on the old code sends a type 9 (GIF) welcome as text only and logs it; AGENTS.md states the caveat (AR-01) | ACCEPTED |
| T-12-14 | 04 | I | low | mitigate | `sender:170` logs the kind, `content.Name` and the chat ID; no file ID or caption | CLOSED |
| T-12-15 | 05 | D | low | mitigate | `TestSettingsLabelLengths` and the locale limits table (`./fuku/i18n`); the phone check is an owner step (12-SPIKE) | CLOSED |
| T-12-16 | 05 | I | med | mitigate | The locale diff adds only `{state}`, `{n}` and `{max}`; the token test and `TestSettingsStaffPrivacy` pass | CLOSED |
| T-12-17 | 06 | T | med | mitigate | `menu:262-268` runs `settingsControlFieldsValid` (`menu:301-311`) in step 1, before the live lookup (`menu:383`). `TestSettingsControlValidate` counts the fake's `getChatMember` and `getChat` calls; the auditor's probe confirmed `o` on g or r, `u` with `v=2` and an extra key are refused there | CLOSED |
| T-12-18 | 06 | E | med | mitigate | `menu:306` requires the control in the page's `Controls`; `edit:718-725` builds `Controls` from each page's `Switches`, so `o` exists only on Welcome | CLOSED |
| T-12-19 | 07 | T | med | mitigate | `rulesm:125-128`: with no media, `sendRulesText` uses the caller's own options. The goldens pass and were never re-recorded | CLOSED |
| T-12-20 | 07 | D | med | mitigate | `rulesm:149-153` retries once as text on a 4xx (not 429) for `/rules` and the deep link, logged at Warn with the chat ID only; the preview passes `degrade=false` (`prev:93`). `TestRulesDegradesToTextOnClientError` | CLOSED |
| T-12-21 | 07 | T | med | mitigate | The only write path for rules buttons is the editor: `door:462-475` → `applyRulesButtons`, behind `chk:329` `ValidButtonURL`; sent as URL buttons (`rulesm:112`). Caveat: the owner-only backup import (NEW-03) | CLOSED |
| T-12-22 | 07 | I | med | mitigate | `rulesm:548` `getChat` and `:557` `IsUserInChat` (a live `getChatMember`) run before the rules row is read | CLOSED |
| T-12-23 | 08 | T | med | mitigate | `greet` passes `MediaBelow` to `SendGreetingMessage`, which ANDs it (`sender:359`). `TestGreetingsMediaBelowEverywhere` | CLOSED |
| T-12-24 | 08 | T | med | mitigate | `greet:257-266` → `grepo:511,521` (cache key deleted at `:236`). `TestGreetingsMediaBelowApply` | CLOSED |
| T-12-25 | 09 | T | high | mitigate | The claim (primary-key insert) comes before the send. `TestGreetOnceConcurrentOneWelcome`: 16 concurrent joins give one welcome; the PostgreSQL claim test above | CLOSED |
| T-12-26 | 09 | D | med | mitigate | `greet` counts `ErrNoPermission`, a 4xx or a 429 (by code) as a definite refusal and gives the claim back only then. `TestGreetOnceDefiniteRefusalReleases`, `TestGreetOnceUnknownOutcomeKeeps` | CLOSED |
| T-12-27 | 09 | D | low | accept | Only a real welcome claims; rows are kept for good (D-11); AGENTS.md records it (AR-02) | ACCEPTED |
| T-12-28 | 09 | T | med | mitigate | The Welcome-on check precedes the claim; the only callers are the join (`greet:1044`) and `finishCaptchaPass` (`captcha.go:1742`); the lockdown guard at group -7 ends the update for a banned joiner; `prev` never calls `SendWelcomeMessage` or `ClaimGreeting`. `TestGreetOnceWelcomeOffNoClaim`, `TestGreetOnceCaptchaPathAndFailure`, `TestGreetOnceLockdownBanNoRecord`, `TestSettingsEditPreviewNeverClaims`. The caveats NEW-01 and NEW-02 are fixed (below) | CLOSED |
| T-12-29 | 09 | I | low | mitigate | `greet` and `greeted` log the chat (and user) ID and the DB error only | CLOSED |
| T-12-30 | 10 | E | high | mitigate | `edit:189-192`: `o` needs `settingsRightAdmin`; the press pipeline makes the live `getChatMember` (`menu:383`), the owner gate (`:414`), `getChat` (`:428`), then `checkRight` (`:563`). `TestSettingsAuthorityOnce` (the four owner worlds), `TestSettingsGateAllDoors` | CLOSED |
| T-12-31 | 10 | T | med | mitigate | `edit:704` `settingsEditSwitchValue` is every switch's `Validate` and runs in step 1. `TestSettingsControlValidate` | CLOSED |
| T-12-32 | 10 | I | med | mitigate | `edit:274` reads only `w.Once` through the side-effect-free loader; nothing outside `greeted.go` reads `greeted_members`. `TestSettingsDriftOnce`, `TestSettingsStaffPrivacy` | CLOSED |
| T-12-33 | 10 | D | low | mitigate | `edit:501-514` two-column grid. `TestSettingsEditSwitchGrid`, `TestSettingsEditCallbackBytes` (Omitted 0) | CLOSED |
| T-12-34 | 11 | E | med | mitigate | `edit:163-178`: `u` on Welcome and Goodbye needs `settingsRightChangeInfo`. `TestSettingsAuthorityMediaBelow` | CLOSED |
| T-12-35 | 11 | T | low | mitigate | `edit:653-657` and `:685-700` reload the page and refuse when the cell is no longer drawn. `TestSettingsEditMediaBelowStalePress`, `TestSettingsEditMediaBelowStoredOnIgnored` | CLOSED |
| T-12-36 | 11 | R | low | mitigate | `edit:660-671`: a failed save shows the save-failed notice over the freshly loaded state | CLOSED |
| T-12-37 | 12 | E | med | mitigate | Rules `u` needs `settingsRightAdminBotAdmin` (`edit:181-186`), as do `rs` and `rr` (`act:288-289`). `TestSettingsRulesMediaBelow`, `TestSettingsAuthorityParityEdit` | CLOSED |
| T-12-38 | 12 | T | med | mitigate | `act:227-244` removes each part through its own apply function; `applyRulesClear` (`rulesm:217`) calls `ClearRules` (`rrepo:154`). `TestSettingsEditRulesRemoveParity`, `TestRulesApplyParity` | CLOSED |
| T-12-39 | 12 | I | med | mitigate | The page shows state words, lengths and counts only. `TestSettingsStaffPrivacy` renders the Phase 12 states; the caption-echo check passes | CLOSED |
| T-12-40 | 12 | T | low | mitigate | `act:104-113` `settingsEditPrivateOnly` wraps Set, Remove and Cancel. `TestSettingsEditGroupMenuRefusal` covers `rs`, `rr` and `rc` with `t=m` and `t=b` | CLOSED |
| T-12-41 | 13 | T | med | mitigate | `chk:329` calls `ValidButtonURL` (`content:224`). `TestSettingsEditRulesTypedRefusals` (a `javascript:` link is refused) | CLOSED |
| T-12-42 | 13 | D | med | mitigate | `chk:226-231` refuses a text over 1024 while media is set; `chk:290-296` refuses media while the text is over 1024, with the longest language's heading counted. `TestSettingsRulesCaptionParity`. `/setrules` applies no 1024 rule, so the member-path degrade (`rulesm:149`) is the backstop (review IN-02, accepted) | CLOSED |
| T-12-43 | 13 | E | med | mitigate | `door:265-270` checks the right and then `botAdmin` (`door:301-323`) for the Rules `EditRight` on every part. `TestSettingsEditRulesTypedRefusals` | CLOSED |
| T-12-44 | 13 | I | low | mitigate | `door:462-475` adds no log line. `TestSettingsEditDoorLogsNoText` | CLOSED |
| T-12-45 | 14 | I | med | mitigate | `prev:51`, `:83` and `:111` all send to `p.query.From.Id`. `TestSettingsEditRulesPreviewParts` (group-menu case) | CLOSED |
| T-12-46 | 14 | T | med | mitigate | `prev` uses only `renderGreeting`, `SendGreetingMessage` and `sendRulesMessage`. `TestSettingsEditPreviewNeverClaims`, `TestSettingsEditPreviewWritesNothing` | CLOSED |
| T-12-47 | 14 | I | low | mitigate | `prev:92` calls `GetGroupLanguage`. `TestSettingsEditRulesPreviewGroupLanguage` | CLOSED |
| T-12-48 | 15 | T | low | accept | Plan 12-15 was dropped: no swap function, no `TestSwapCleanMsgIDConcurrent`; `greet` keeps the old read-modify-write of the last message ID. The Delete-last race predates this phase and its worst case is a cosmetic orphan welcome; AGENTS.md records it as the known "Delete-last race" (AR-03). Re-dispositioned from mitigate to accept | ACCEPTED |
| T-12-49 | 15 | D | low | accept | Not applicable: the swap transaction it describes was never built, so no lock wait exists | CLOSED (N/A) |
| T-12-50 | 16 | I | high | mitigate | `settings_staff_privacy_test.go` covers the Phase 12 states and a non-creator admin's private Phase 12 run end to end. `TestSettingsStaffPrivacy` | CLOSED |
| T-12-51 | 16 | D | low | mitigate | `TestSettingsEditScreensFitEveryLocale`, the page fit tests and `TestSettingsEditCallbackBytes` | CLOSED |
| T-12-52 | 16 | R | med | mitigate | `Makefile` integrity list holds `TestGreetedMemberClaimConcurrent` and `TestRulesMediaMigrationDefaults` plus `./fuku/db/greetings`; AGENTS.md is updated; `make check-docs` reports no drift | CLOSED |
| T-12-SC | 01-16 | T | high | mitigate | `go.mod` and `go.sum` unchanged against `bd2f794` and `72a692b` | CLOSED |

**Prohibitions in the plans:** all hold in the code: the goldens were committed first and never re-recorded (12-01); no
`default:` tag and `greeted_members` is not exportable (12-02); the claim is made in the database only, never through
Redis or a cache (12-03); only `sendCaptioned` reads the flag and pins are untouched (12-04); the text branch never goes
through the greeting sender (12-07); the claim comes after the Welcome-on check, never runs in a preview, and an unknown
outcome keeps it (12-09); Rules uses the shared predicates (12-13); 12-15 is droppable and was dropped.

---

## New Threats Found by the Audit (unregistered)

The plan summaries raise no unregistered threat flags. None of the items below reaches the `high` gate.

| ID | Severity | Finding and scenario | Disposition |
|---|---|---|---|
| NEW-01 | low | **Greet once was bypassed when the greetings read failed.** `SendWelcomeMessage` read its settings through `GetGreetingSettings`, which on a database read error returned a default with Welcome on and Greet once off (`grepo:72-90`), so a member greeted before could be welcomed again during a brownout, with no claim made. Same as review WR-01 | **Fixed** (`25cde9f`): `greetings.LoadGreetingSettings` returns the error; on any error the welcome path logs the chat ID and the error, sends nothing and claims nothing. `TestGreetOnceReadErrorFailsClosed` |
| NEW-02 | info | The claim was taken before `renderGreeting`; a panic in rendering (recovered) left the member claimed with no welcome. Same as review IN-01 | **Fixed** (`3ffb87c`): the recover gives the claim back when no send was attempted and keeps it after one. `TestGreetOncePanicBeforeSendReleasesClaim`, `TestGreetOncePanicAfterSendKeepsClaim` |
| NEW-03 | low | **The owner-only backup import bypasses the editor's checks for the new rules columns.** `importRules` (`bk:791-799`) writes `file_id`, `media_type`, `btns` and `media_below` as given, with no `ValidButtonURL` and no 1024 rule. A foreign file ID or an over-long caption makes `/rules` fall back to text; a button link Telegram rejects breaks `/rules` until the owner fixes it. Only the creator can import (`RequireUserOwner`), so no privilege is widened; Welcome buttons had the same exposure before this phase | **Accepted** (AR-04) |
| NEW-04 | info (predates Phase 12) | **A full reset fails on PostgreSQL.** `clearWarns` writes an empty `warn_mode`, which the legacy `chk_warns_mode` rejects, so `ClearChatData` over all modules rolls back there. Found because `TestGreetedMembersStayOutOfBackup` fails at its reset step on PostgreSQL. The guarantee that a reset leaves `greeted_members` alone still holds: no clear function touches it. Same as review IN-03 | **Deferred**: its own change, outside this phase |

Review finding WR-02 (a partial greetings import aborting on PostgreSQL through the new `NOT NULL` columns) is a
correctness regression, not a threat; it is fixed in `230f82c` and listed in `12-REVIEW-DISPOSITION.md`.

---

## Accepted Risks Log

| Risk ID | Threat Ref | Rationale |
|---------|------------|-----------|
| AR-01 | T-12-13 | During a rolling deploy a replica on the old code sends a type 9 (GIF) welcome as text only and logs it (AGENTS.md) |
| AR-02 | T-12-27 | Only a real welcome claims, the primary key blocks duplicates, and rows are kept for good (D-11, AGENTS.md); a chat migration to a supergroup welcomes members once more, and members who joined while Greet once was off are welcomed once when it is turned on |
| AR-03 | T-12-48, T-12-49 | The Delete-last race predates this phase and is cosmetic; plan 12-15 was dropped and the race is recorded in AGENTS.md |
| AR-04 | NEW-03 | The owner-only backup import writes the new rules columns without the editor's checks; the member-path text fallback is the backstop and no privilege is widened |

---

## Security Audit Trail

| Audit Date | Threats Total | Closed | Open (blocking) | Open (non-blocking) | Run By |
|------------|---------------|--------|-----------------|---------------------|--------|
| 2026-11-05 | 53 | 50 closed (T-12-49 as N/A), 3 accepted (T-12-13, T-12-27, T-12-48) | 0 | 0 | gsd-security-auditor |

Tests run, all passing with no skips except where noted: 63 named security tests in `./fuku/modules` and then the whole
`^(TestSettings|TestLoadSettings|TestGreet|TestRules|TestLegacyGreetings)` family (275 s); `./fuku/i18n`,
`./fuku/db/greetings`, `./fuku/db/rules`, `./fuku/db/lang`, `./fuku/db/backup`, `./fuku/db/models`,
`./fuku/utils/media`, `./fuku/utils/content`, `./fuku/utils/formatting`; on PostgreSQL 16 `TestRepositoryMigrationChain`,
`TestRulesMediaMigrationDefaults`, `TestGreetedMemberClaim`, `TestGreetedMemberClaimConcurrent`,
`TestAllModulesRoundTripEveryMeaningfulField`, `TestImportChatDataRollsBackEarlierModules` and
`TestOldGreetingsExportDecodesToZeros` (`TestGreetedMembersStayOutOfBackup` fails at its reset step there for the
pre-existing reason in NEW-04); `make check-docs` with no drift; a forged-field probe, deleted afterwards. CI on `4dae0c5`
ran the same PostgreSQL integrity target green.

**threats_open:** 0
