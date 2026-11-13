---
phase: 12-rules-media-media-under-text-and-greet-once
reviewed: 2026-11-05T00:00:00Z
depth: deep
files_reviewed: 24
files_reviewed_list:
  - migrations/20261105120000_add_rules_media_and_greet_once.sql
  - fuku/db/db.go
  - fuku/db/models/greeted_member.go
  - fuku/db/models/greetings.go
  - fuku/db/models/rules.go
  - fuku/db/greetings/greeted.go
  - fuku/db/greetings/repository.go
  - fuku/db/rules/repository.go
  - fuku/db/lang/repository.go
  - fuku/db/backup/backup.go
  - fuku/utils/content/content.go
  - fuku/utils/media/sender.go
  - fuku/utils/formatting/formatting.go
  - fuku/modules/greetings.go
  - fuku/modules/rules.go
  - fuku/modules/settings_edit.go
  - fuku/modules/settings_edit_actions.go
  - fuku/modules/settings_edit_check.go
  - fuku/modules/settings_edit_preview.go
  - fuku/modules/settings_input_door.go
  - fuku/modules/settings_menu.go
  - fuku/modules/settings_pages.go
  - Makefile
  - locales/en.yml
findings:
  critical: 0
  warning: 2
  info: 3
  total: 5
status: issues_found
---

# Phase 12: Code Review Report

**Reviewed:** 2026-11-05
**Depth:** deep
**Files Reviewed:** 24 (plus the phase's tests as evidence, and all 8 locale files for the 17 new or renamed keys)
**Status:** issues_found

## Summary

The phase adds Rules media and buttons, the Media under text switch, the `db.ANIMATION` GIF type, Greet once with
the `greeted_members` record, the switch grid on the editor pages, step-1 `Validate` for controls, and the shared rules
builder and sender. I traced every path that ends in a write or a send.

**Verified by running code:**
- I ran the PostgreSQL integrity suite on a fresh throwaway PostgreSQL 16 cluster (migration chain first, then
  `make test-postgres-integrity`). All 17 packages pass, including the new `TestGreetedMemberClaimConcurrent` and
  `TestRulesMediaMigrationDefaults`. The migration is idempotent, runs as one file and leaves `NOT NULL DEFAULT` zero
  columns plus the two-column primary key.
- I ran the phase's module families on SQLite with `-race` (`TestGreetOnce*`, `TestRules*`, `TestSettingsEdit*`,
  `TestSettingsControl*`, `TestSettingsStaffPrivacy`, `TestSettingsGateAllDoors`, `TestSettingsAuthorityParity*`,
  `TestSettingsParity*`, `TestSettingsDrift*`, `TestGreetings*`): `ok` in 184 s.
- I ran a throwaway probe of the backup import on PostgreSQL. It confirmed WR-02, and I deleted it afterwards.

**Checked by reading, came out clean:**
- **Greet once claim.** The claim is one `INSERT ... ON CONFLICT DO NOTHING`. It sits after the Welcome-on check and
  before the build and the send, and it is reached only through `SendWelcomeMessage`: the chat_member join, the service
  message and both captcha passes. A captcha failure, a lockdown ban, Welcome off and every preview never reach it. A
  claim error fails closed. `welcomeDefinitelyNotSent` classifies `ErrNoPermission`, any 4xx and a 429 by code, and an
  unknown outcome keeps the claim. Neither Redis nor `claimRecentJoinProcessing` is involved.
- **Sender, byte-identical for old data.** `sendCaptioned` sends exactly the old photo and video options. A false
  `ShowCaptionAboveMedia` adds no parameter (gotgbot's `addIfValueNotZero`). `SendGreeting` is unchanged.
  `CaptionAboveEligible` gates the flag in the sender, so a stored "on" with a document, sticker, audio or voice sends
  the old request.
- **Old GIFs.** Old document GIFs keep `sendDocument`. Notes, filters and pins keep `extractMediaFromReply`.
- **Rules builder and sender.** `/rules`, the deep link and Full preview share `buildRulesMessage` and
  `sendRulesMessage`. The text branch reproduces `msg.Reply`: `ReplyMessage.replyTo` only fills the message ID, and
  there is no thread ID either way. Click-to-read is unchanged. The member-path degrade fires only on
  `isTelegramClientError`, which unwraps `fukuerrors`. Full preview passes `degrade=false`.
- **`/clearrules` vs the three Removes.** `/clearrules` (`rules.ClearRules`) stores exactly what the three Rules
  Remove presses store: text `''`, file `''`, type `0`, `btns []`. Both keep `media_below`, `private` and `rules_btn`.
- **Editor.** The switch grid stays within the 8/9-row limit with all four Welcome cells. Step-1 `Validate` refuses a
  forged page, control or value before any I/O. The stale-press check reloads the page before writing `u`. Rights per
  cell match the UI-SPEC (A2). The typed-reply door routes the Rules media and buttons parts through their apply
  functions, behind the Rules bot-admin check and the album gate. Previews go only to `query.From.Id` and never claim
  or write.
- **Caption limit (D-03).** The 1024 rule holds both ways on Welcome, Goodbye and Rules, with the Rules heading
  measured in all 8 languages.
- **Locales.** All 17 new or renamed keys are in all 8 files with matching `{state}`, `{n}` and `{max}`. No code
  references the four removed keys.
- **Logging.** No new log line carries text, a caption, a button, a link or a file ID.

**Remaining problems:**
- Greet once fails open when the greetings read fails.
- The new `NOT NULL` greetings columns break an import whose payload omits a welcome or goodbye object.
- Three smaller notes (Info section).

## Warnings

### WR-01: Greet once fails open when the greetings read fails: an already-greeted member is welcomed again, with no claim made

**File:** `fuku/modules/greetings.go:888-899` (`GetGreetingSettings`, then `claimWelcome(..., greetPrefs.WelcomeSettings.Once)`); `fuku/db/greetings/repository.go:72-93` (`checkGreetingSettings` error branch)
**Issue:**
- `SendWelcomeMessage` reads the switch through `GetGreetingSettings`, an uncached `checkGreetingSettings`.
- On any read error other than not-found (a statement timeout, a dropped connection, pool exhaustion during a raid of
  joins), that function logs the error and returns hard-coded defaults: `ShouldWelcome: true` and `Once` left at Go
  `false`.
- `claimWelcome(chat, user, false)` then returns `send=true, claimed=false` without touching `greeted_members`, so
  the welcome goes out.
- The design's own rule (D-11, AGENTS.md "a claim database error sends nothing (fail closed) ... a double welcome is
  worse than a missing one") is kept for the claim but broken one step earlier, for the read that decides whether to
  claim at all.

**Failing scenario:**
1. A group has Greet once on, and member 42 was welcomed last week.
2. Member 42 leaves and rejoins during a short database hiccup that makes only the `greetings` `First()` fail.
3. The defaults say Welcome on and Greet once off, so member 42 is welcomed a second time.
4. Because no claim was attempted, nothing records it either.

The same fallback also welcomes in a group whose owner turned Welcome off. That part predates this phase, but Greet
once is the first setting whose purpose is to suppress a welcome.

**Fix:** Give the welcome path a read that returns its error, and fail closed on it. For example, use
`greetings.LoadSettingsContext` (it already returns errors and creates nothing), or add an error-returning variant of
`GetGreetingSettings`:
```go
greetPrefs, err := greetings.LoadSettingsContext(ctx, chat.Id)
if err != nil {
    log.Errorf("[Greetings][SendWelcomeMessage] read settings of chat %d: %v", chat.Id, err)
    return nil // a double welcome is worse than a missing one
}
```
If changing the welcome-off behaviour is out of scope, at least skip the send when the read errored and the stored
`Once` is unknown. Add a test that makes the greetings read fail with Greet once on and a `greeted_members` row
present, and asserts no `sendMessage`.

### WR-02: A greetings import whose payload omits `welcome_settings` or `goodbye_settings` now aborts the whole import on PostgreSQL

**File:** `fuku/db/backup/backup.go:694-706` (`importGreetings`) and `:209-238` (`replaceChatSetting`: `val, _ := field.ValueOf(...)` at 234); `fuku/db/models/greetings.go:18-21,33` (new fields with no `default:` tag); migration lines 27-29 (`NOT NULL DEFAULT FALSE`)
**Issue:**
- `replaceChatSetting` builds an insert map from every schema field. For a field inside a nil embedded pointer
  (`*WelcomeSettings` or `*GoodbyeSettings` absent from the payload), GORM's `ValueOf` returns `nil`, and the map
  inserts SQL `NULL`.
- Before this phase every `welcome_*` and `goodbye_*` column was nullable, so such a payload imported with NULLs.
- The three new columns are `NOT NULL` and the model has (correctly) no `default:` tag, so the explicit NULL is not
  replaced by the column default. The insert fails with `23502`.
- `ImportChatData` runs every module in one transaction, so the failure rolls back every module of the import, not
  just greetings.
- The codebase explicitly accepts partial payloads: there is a legacy `1.0` format with a
  `preserveLegacyOmissions` path, and `TestLegacyBackupPreservesFieldsThatVersionDidNotExport`.

**Failing scenario (verified on PostgreSQL 16 with a throwaway probe, since deleted):**
```
ImportModuleData(chat, "greetings", {"settings":{"welcome_settings":{"welcome_text":"hi","welcome_enabled":true}}})
-> ERROR: null value in column "goodbye_media_below" of relation "greetings" violates not-null constraint (SQLSTATE 23502)
ImportModuleData(chat, "greetings", {"settings":{}})
-> ERROR: null value in column "welcome_media_below" of relation "greetings" violates not-null constraint (SQLSTATE 23502)
```
SQLite does not catch this, because the AutoMigrate columns are nullable there. The backup tests that do run on
PostgreSQL in CI do not omit either object.

**Fix:** In `importGreetings`, fill a missing embedded struct before `replaceChatSetting`. Then a NULL never meets
the new `NOT NULL` columns, and the old columns keep their old values (an empty struct writes the same zero or empty
values the old NULLs read as):
```go
if data.Settings != nil {
    data.Settings.ChatID = chatID
    if data.Settings.WelcomeSettings == nil {
        data.Settings.WelcomeSettings = &models.WelcomeSettings{}
    }
    if data.Settings.GoodbyeSettings == nil {
        data.Settings.GoodbyeSettings = &models.GoodbyeSettings{}
    }
}
```
Alternatively, make `replaceChatSetting` drop nil values for columns that have a database default. Add an import test
with a payload missing `goodbye_settings` to the `make test-postgres-integrity` list.

## Info

### IN-01: A panic between the claim and the send keeps the Greet once row, so that member is never welcomed

**File:** `fuku/modules/greetings.go:882-886` (the recover), `:896-926`
**Issue:** The claim is taken before `renderGreeting` (placeholder filling, `FormattingReplacer`, a rules read and
keyboard building) and before the send.
- If anything in that span panics, the deferred `recover` in `SendWelcomeMessage` swallows the panic.
- `giveBackWelcome` is reached only on a send error, so a panic never reaches it.
- The row stays, although the welcome definitely was not sent. Every later join of that member is then silent for good.

This is low likelihood, but it is the one "definitely not sent" outcome that keeps the claim.
**Fix:** Release in the recover when a claim was won and no send was attempted. Track a `sendAttempted` flag set just
before `media.SendGreetingMessage`, and call `greetOnceStore.Release` from the deferred function when `claimed &&
!sendAttempted`.

### IN-02: `/setrules` still stores a text that cannot be the caption of the stored rules media, and replies "set successfully"

**File:** `fuku/modules/rules.go:410-417` (`setRules` → `applyRulesText`), `:179-185` (degrade)
**Issue:** D-03 is enforced only in the editor. `/setrules` with a long text while media is set does three things:
- It stores the text and answers `rules_set_successfully`.
- From then on every `/rules` and deep link takes Telegram's 400 ("caption is too long"), logs a Warn and silently
  sends the text without the media (the documented "Deliberate deviation from D-03").
- Full preview fails outright.

This is documented and accepted in 12-07. Still, the owner who ran the command is never told that the media will no
longer show.
**Fix (optional):** In `setRules`, when the stored row has media and `settingsEditRulesLongest(title,
normalizeRulesForHTML(html))` is over 1024, append a short note to the success reply. Do not refuse, to keep the
command's behaviour.

### IN-03: `TestGreetedMembersStayOutOfBackup` can only run on SQLite: the reset it relies on fails on PostgreSQL (pre-existing, out of scope)

**File:** `fuku/db/backup/greeted_backup_test.go:151`; root cause `fuku/db/backup/backup.go:1058-1065` (`clearWarns` inserts `WarnSettings{WarnLimit: 3}` with an empty `warn_mode`)
**Issue:**
- Run against PostgreSQL, `ClearChatData` fails with `chk_warns_mode` (`23514`), so this new test and the old
  `TestClearChatData_AllModules` and `TestClearModuleData_IndividualModules` fail.
- The phase's "greeted_members is never part of reset" guarantee is therefore proven only on SQLite.
- The root cause predates this phase. It also means a full reset on a PostgreSQL deployment fails at the warns
  module.

**Fix:** This is outside the phase. Set `WarnMode` to the default (`"mute"`) in `clearWarns`, then add
`TestGreetedMembersStayOutOfBackup` to `make test-postgres-integrity`.

---

_Reviewed: 2026-11-05_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: deep_
