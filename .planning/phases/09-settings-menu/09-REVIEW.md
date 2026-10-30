---
phase: 09-settings-menu
reviewed: 2026-10-30T00:00:00Z
depth: deep
files_reviewed: 40
files_reviewed_list:
  - alita/modules/settings.go
  - alita/modules/settings_menu.go
  - alita/modules/settings_pages.go
  - alita/modules/settings_card.go
  - alita/modules/settings_captcha.go
  - alita/modules/settings_antiflood.go
  - alita/modules/settings_lockdown.go
  - alita/modules/settings_staff.go
  - alita/modules/captcha.go
  - alita/modules/antiflood.go
  - alita/modules/autolockdown.go
  - alita/modules/lockdown.go
  - alita/modules/lockdown_alert_callbacks.go
  - alita/modules/lockdown_status.go
  - alita/modules/staff_link.go
  - alita/modules/staff_panel.go
  - alita/utils/chat_status/owner.go
  - alita/i18n/test_helper.go
  - alita/modules/settings_env_test.go
  - alita/modules/settings_menu_test.go
  - alita/modules/settings_test.go
  - alita/modules/settings_captcha_test.go
  - alita/modules/settings_antiflood_test.go
  - alita/modules/settings_lockdown_test.go
  - alita/modules/settings_lockdown_lift_test.go
  - alita/modules/settings_staff_test.go
  - alita/modules/settings_staff_privacy_test.go
  - alita/modules/settings_private_test.go
  - alita/modules/settings_help_test.go
  - alita/modules/captcha_apply_test.go
  - alita/modules/antiflood_apply_test.go
  - alita/modules/lockdown_apply_test.go
  - alita/modules/staff_seams_test.go
  - alita/db/antiflood/repository_test.go
  - alita/i18n/settings_locale_test.go
  - alita/utils/chat_status/owner_test.go
  - locales/config.yml
  - locales/en.yml (and es, fr, hi, id, pt, ru: settings_* keys)
  - docs/src/content/docs/commands/settings/index.md
  - AGENTS.md
findings:
  critical: 0
  warning: 5
  info: 5
  total: 10
status: issues_found
---

# Phase 9: Code Review Report

**Reviewed:** 2026-10-30
**Depth:** deep
**Files Reviewed:** 40
**Status:** issues_found

## Summary

I reviewed the new settings menu, the five command-to-apply-function refactors and the shared seams, and traced calls across packages.

The press pipeline is sound as written:
- Structure and the `c`/`u` split are checked before any I/O.
- Service identities are refused before any lookup.
- The opener check runs before the live `getChatMember`.
- The per-control right is judged again on every press, so Yes of a card is covered.
- Every path I traced answers the callback exactly once.
- No code path of the menu discards a DB error.

The refactors are behaviour-preserving. I diffed the old command bodies against the apply functions and found no change in validation order, replies or stored rows. The cases I compared were `/setflood 00` and `-0`, `/delflood`, `/captchatime`, `/captchamode turnstile`, `/unlockdown` and `/linkstaff`.

Mechanical checks:
- `go build` and `go vet` are clean.
- `golangci-lint` reports nothing for the touched packages.
- `go test -tags testtools -run 'TestSettings|TestLoadSettings' ./alita/modules` passes, as does `./alita/i18n`.
- Placeholders and tags agree across all 7 locales for the 80 `settings_*` keys.
- Every locale key the new Go code uses exists.
- Every `settings_*_test.go` carries the `testtools` build tag, so `OverrideManagerLanguagesForTest` is not compiled into production.

I found no blocker. The five warnings are:
- the two-tap confirm cards act on whatever is current at the second tap;
- the private menu shows the Staff Group to any admin of the linked group;
- a one-tap antiflood side effect that silently turns antiflood on;
- the lift announcement runs before the page is redrawn;
- the private press path never checks the chat type of `c`.

## Warnings

### WR-01: Lift Yes and Unlink Yes act on the current object, not the one the card showed

**File:** `alita/modules/settings_lockdown.go:331-347`, `alita/modules/settings_staff.go:479-505`, `alita/modules/settings_card.go:45-64`
**Issue:** The Yes button carries only `a` and the owner field plus the expiry `e`. At Yes, `settingsLockdownLiftYes` calls `applyLockdownLift(chat)`, which lifts whatever lockdown is current for the chat. `settingsStaffUnlinkYes` re-reads `GetLinkOfGroupFresh(chat)` and unlinks whatever link is current. Nothing binds the second tap to the object the admin confirmed.

The card stays on screen for 5 to 6 minutes. This sequence is possible in that window:
1. Admin A asks to lift lockdown L1 and gets the card.
2. Admin B lifts L1.
3. A raid trigger starts L2.
4. A presses Yes, and L2 is lifted although A confirmed L1.

The same applies to Unlink after an unlink and re-link to another Staff Group. The authority checks all rerun, so this is not a privilege gap. It does mean a destructive confirmation can hit an object the confirmer never saw. The alert callbacks bind the card to the lockdown row with `r`, and the old Unlink button carried `l`. The menu's cards are looser than both.
**Fix:** Carry the identity in the card.
- In `renderSettingsLockdownLiftCard`, add `"r": strconv.FormatUint(uint64(row.ID), 10)` to the Yes fields (`Fields: []string{"e","r"}` on `ly`).
- In `settingsLockdownLiftYes`, parse `r` strictly, read the current row, and answer `lockdown_alert_expired` or the stale text when `row.ID != r`.
- Do the same for Unlink with the `staff_group_links` row ID (`l`), compared to `link.ID` before `runUnlinkGroup`.

### WR-02: A private menu shows the Staff Group title and chat ID to any admin of the linked group

**File:** `alita/modules/settings_staff.go:99-119` (`settingsStaffViewFor`), `settings_staff.go:252-254` (`settingsStaffRead`), `alita/modules/settings_menu.go:287-325`
**Issue:** `Full` is set for any private menu, and the private press only proves that the presser is a live creator or administrator of the linked group. A moderator admin of group G who is not in the Staff Group, and not its creator, can open `/start settings_<G>` and read the Staff Group's title and chat ID. `data.linkStaff` is read for every private press, and `settings_staff_linked_to` prints both.

The Phase 1 invariant in AGENTS.md is that nothing about a link is shown to the linked group's members. D-21 relaxes this for the private menu on the assumption that only staff read it. That assumption does not hold, because the private audience is "any admin of G". The group-menu masked line ("Linked to a Staff Group.") already tells every member that the group is linked. That follows D-21 but is a weaker posture than D-06 and D-07.
**Fix:** Gate `Full` on proven membership.
- Show the title and ID only when the presser passes the live `chat_status.CheckOwner` for `link.StaffChatID`, or is a member of the Staff Group.
- Otherwise use `settings_staff_linked_hidden`, as the group menu does.
- Alternatively, record in CONTEXT that D-06 is weakened for the admins of a linked group.
- Extend `TestSettingsStaffPrivacy` with a non-staff admin's private press.

### WR-03: One tap on Ban, Kick or the delete switch silently turns antiflood on at 5 messages

**File:** `alita/modules/settings_antiflood.go:315-345`, `alita/modules/antiflood.go` (`applyFloodMode`, `applyFloodDelete`), `alita/db/antiflood/repository.go:33-103`
**Issue:** On a chat with no `antiflood_settings` row, the cached default is limit 0 (off). The menu's action buttons (ban or kick) and the delete switch call `FirstOrCreate`, which inserts a row with the model default `flood_limit: 5`. A Ban press on a chat that shows "Antiflood is off" therefore makes the bot start punishing anyone who sends 5 messages in the flood window. The page redraws from a read-back, so a careful admin could notice, but the screen shows no notice about it.

AGENTS.md records this as accepted (D-27) because the commands behave the same. A command is typed deliberately and replies "mode set to ban". A menu button a user taps to browse options is much easier to hit by accident.
**Fix:** Preferably fix the repository.
- `upsertChatField` should insert `flood_limit: 0` when it creates the row, so mode and delete never change the limit.
- Update `TestSetFloodModeOnFreshChatStoredLimit`, `TestSetFloodMsgDelOnFreshChatStoredLimit` and the AGENTS.md trap bullet.

If the repository must stay unchanged:
- In `settingsFloodSetMode` and `settingsFloodSetDelete`, compare `before.Limit` with `result.Stored.Limit`.
- When the stored limit changed from 0, show a warning notice (new locale key, all 7 files) such as "Antiflood is now on at {n} messages".

### WR-04: The lift announcement blocks the page redraw while the Yes button is still live

**File:** `alita/modules/settings_lockdown.go:340-347`, `alita/modules/lockdown_alert_callbacks.go:713-729` (`lockdownPostLiftNotice`)
**Issue:** `settingsLockdownLiftYes` calls `lockdownAnnounceLift` before it returns the screen, and the caller edits the message only after the return. `lockdownAnnounceLift` posts through `lockdownPaced`, which can wait up to `MaxWait` (60 s) for a slot and has a 30 s send timeout. The lockdown is already lifted, yet the card, with its live Yes button, is still shown for that whole time. A second Yes then lands on `liftApplyInProgress` or not-active, and the admin sees a stale card for up to a minute. The alert's own Lift Confirm edits the card first and posts afterwards.

In a group menu the group also gets two messages for one lift: the public `lockdown_lifted` notice and the redrawn page with the same text as a notice.
**Fix:** Edit first, then announce. For example, have `settingsLockdownLiftYes` call `p.edit(screen)` itself, return nil, and then call `lockdownAnnounceLift`. Alternatively, run the announce after `dispatch` returns. For a non-private menu, consider skipping the public notice, since the menu message already says it in the same chat. D-26 requires the notice only from the private menu.

### WR-05: The private press path never checks the chat type of `c`

**File:** `alita/modules/settings_menu.go:311-324` (compare `alita/modules/settings.go:141-143`)
**Issue:** The deep link refuses anything but a group or supergroup. The press pipeline accepts whatever `getChat(c)` returns. AGENTS.md says callback data is not authenticated by Telegram. A user with a raw MTProto client can send an arbitrary `settings|v1|c=<id>` payload on a message of their own private chat, and the only gates are that `c` is negative and the presser is a live admin of it.

A channel the bot is in and the presser administers passes. The Captcha and Antiflood pages then draw and write `captcha_settings` or `antiflood_settings` rows for a channel ID. `IsBotAdmin` and `IsUserAdmin` are also evaluated against a channel there. The impact is low, because the presser needs admin rights there. It breaks the "private press equals deep-link rules" symmetry and writes junk rows.
**Fix:** After `full.ToChat()` in `settingsVerifyPress`, add:
```go
if group.Type != "group" && group.Type != "supergroup" {
    p.cannotOpen()
    return nil
}
```
Add a case to `TestSettingsOwnership` using a fake `getChat` that returns a channel.

## Info

### IN-01: The documented test command skips one of the new tests, and one test asserts an English literal

**File:** `AGENTS.md` (Testing bullet "Run the settings tests with ... `-run '^TestSettings'`"), `alita/modules/settings_help_test.go:49`
**Issue:** `TestLoadSettingsRegistersHelp` does not match `^TestSettings`. The same bullet cites it as a guard, but the recommended invocation never runs it. `make test` does run it. The test also asserts the literal English help text ("Manage this group from one button menu."), which AGENTS.md says not to do (assert observable behaviour, not literals). Editing the help copy would break it for no behavioural reason.
**Fix:** Use `-run '^(TestSettings|TestLoadSettings)'` in AGENTS.md. In the test, assert on the `/settings` and `/setfloodmode` command names only.

### IN-02: `captchaWebModeRefusal` loads `captchaWebCurrent` twice

**File:** `alita/modules/captcha.go:704-708`
**Issue:** `rt := captchaWebCurrent.Load()` runs first, then `captchaWebInstalled(bot)` loads it again, and the later `rt.cfg.Redis.Ping` uses the first copy. The runtime is only installed at startup, so the window is theoretical. The `|| rt == nil` guard makes it safe, but the two loads can in principle disagree.
**Fix:** Give `captchaWebInstalled` a helper that takes `rt`, or return `rt` from one call, so the checks and the use share one load.

### IN-03: The deep link answers differently for "bot not in chat" and "not an admin"

**File:** `alita/modules/settings.go:118-142`
**Issue:** Any user can send `/start settings_-100…` with an arbitrary negative ID. The answer is `settings_cannot_open` when the bot is missing or the chat is gone, and `settings_start_not_admin` otherwise. That is a small oracle for "is the bot in chat X". Each probe also costs one to two Bot API calls with no per-user limit. Chat IDs are large, so this is hard to exploit. It is worth noting because the other refusals were designed not to leak.
**Fix:** Optionally return `settings_start_not_admin` for the bot-missing case too, and apply a small per-user cooldown on the deep link.

### IN-04: The Link picker lists at most 8 Staff Groups, and one-match-plus-unknown becomes a picker

**File:** `alita/modules/settings_staff.go:328`, `settings_staff.go:343-351`, `settings_staff.go:415-422`
**Issue:** There are two points of drift from `/linkstaff`:
- A presser who owns more than 8 verified Staff Groups cannot reach the 9th from the menu. The picker truncates silently. The command can link any of them by explicit ID.
- With one match and `unknown > 0`, the command fails closed with `staff_check_failed`. The menu shows a one-button picker.

Neither grants more authority than the command, because the pick re-verifies live. Both contradict "the menu does exactly what the command does".
**Fix:** Show a "N more, use /linkstaff <id>" line when the list is truncated. For the matched == 1 and unknown > 0 case, show the same refusal as the command.

### IN-05: Several refusal texts reused from the commands read oddly inside a menu, and a panic leaves the spinner running

**File:** `alita/modules/settings_staff.go:344-347`, `alita/modules/settings_menu.go:329-344`
**Issue:**
- `staff_link_no_staff_group` ends with "…then send this command again", which is wrong in a button flow.
- `settingsCallback` defers `RecoverFromPanic`, which swallows a panic, but the query is then never answered. The client shows a spinner until Telegram times it out.

**Fix:** Add a menu-specific variant of the no-Staff-Group line (all 7 locales, with the length table row). In `settingsCallback`, answer the query in a deferred function that checks `p.answered`, so a recovered panic still answers once.

---

_Reviewed: 2026-10-30_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: deep_
