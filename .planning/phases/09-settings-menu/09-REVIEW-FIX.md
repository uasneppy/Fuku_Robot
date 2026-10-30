---
phase: 09-settings-menu
fixed_at: 2026-10-30T13:00:00Z
review_path: .planning/phases/09-settings-menu/09-REVIEW.md
iteration: 1
findings_in_scope: 10
fixed: 10
skipped: 0
status: all_fixed
---

# Phase 9: Code Review Fix Report

**Fixed at:** 2026-10-30
**Source review:** `.planning/phases/09-settings-menu/09-REVIEW.md`
**Iteration:** 1

**Summary:**
- Findings in scope: 10 (WR-01..WR-05, IN-01..IN-05)
- Fixed: 10
- Skipped: 0

Every fix is one commit on `claude/cool-hamilton-hvs9nl`. Fixes that change a rule in
`AGENTS.md` update it in the same commit. Verification ran in the main checkout, not a
worktree, on SQLite and miniredis (no PostgreSQL).

## Fixed Issues

### WR-01: Lift Yes and Unlink Yes act on the current object, not the one the card showed

**Status:** fixed: requires human verification (logic change)
**Commit:** f057260
**Files modified:** `alita/modules/settings_card.go`, `alita/modules/settings_lockdown.go`,
`alita/modules/settings_staff.go`, `alita/modules/lockdown.go`, `AGENTS.md`, tests
**Applied fix (D-33):**
- The Lift card's Yes carries `r` (the `chat_lockdowns` row ID) and the Unlink card's Yes
  carries `l` (the `staff_group_links` row ID), beside `e`. `settingsCardObjectID` parses
  them strictly (positive 63-bit, at most 19 digits, digits only). A bad field is an invalid
  request answered before any lookup, like `e`. The `ly` and `uy` allow-lists carry the new
  field.
- New `applyLockdownLiftOf(ctx, b, chatID, actor, onlyID)`: `applyLockdownLift` calls it with
  0. It reads the chat's current lockdown once, and a row under another ID than the card's is
  `liftApplyLost` (`lockdown_already_lifted`) with nothing lifted. The comparison and the
  lift use the same read, so no lockdown can start between them.
- Unlink Yes compares the link read fresh with `l` before `runUnlinkGroup`; a different ID (or
  none) is `staff_unlink_not_linked` and nothing is removed.
- No new locale key. Worst-case callback tests cover the new fields (10-digit IDs; a
  17-digit ID is also asserted to fit 64 bytes).
- Tests: stale Lift card after another lockdown replaced the object, forged and
  out-of-range `r` and `l`, an `r` naming another lockdown, a stale Unlink card after a
  relink to another Staff Group.

### WR-02: A private menu shows the Staff Group title and chat ID to any admin of the linked group

**Status:** fixed: requires human verification (logic change)
**Commit:** 51859e1
**Files modified:** `alita/modules/settings_menu.go`, `alita/modules/settings_pages.go`,
`alita/modules/settings_staff.go`, `AGENTS.md`, tests
**Applied fix (D-32):** `settingsView.Creator` is set from the press's own live
`getChatMember` record (status creator, private menu only, no extra Telegram call).
`settingsStaffViewFor` stays the one deciding place: `Full` is the creator's private menu or
a Staff Group's own page, and it clears the title and ID for everyone else. The page does not
read the Staff Group row for a non-creator. Link and Unlink authority is unchanged.
`TestSettingsStaffPrivacy` gained a pure render of a non-creator private view in all 7
languages (plus the notices) and an end-to-end private run by a non-creator admin that
asserts no title, escaped title or chat ID appears in any text, button or callback data
while the creator still sees the full line. The AGENTS.md D-21 bullet now says what D-32
says.

### WR-03: One tap on Ban, Kick or the delete switch silently turns antiflood on at 5 messages

**Status:** fixed
**Commit:** 3362e7e
**Files modified:** `alita/modules/settings_antiflood.go`, `alita/i18n/settings_locale_test.go`,
`locales/*.yml` (7), `AGENTS.md`, tests
**Applied fix (D-34):** the repository and the commands are unchanged. In
`settingsFloodSetMode` and `settingsFloodSetDelete` the redraw adds the warning notice
`settings_antiflood_now_on` (new, menu only, all 7 locales, `{n}` is the stored limit) when
the limit read before the write was 0 and the read-back is not. A limit press never shows
it. Tests: fresh chat Ban press shows the notice and stores limit 5; fresh chat delete switch
likewise; a chat with an existing limit gets none; a limit press gets none; the page-fits
test renders the notice in all 7 languages. The AGENTS.md first-write trap bullet mentions
the notice.

### WR-04: The lift announcement blocks the page redraw while the Yes button is still live

**Status:** fixed
**Commit:** 7044ea8
**Files modified:** `alita/modules/settings_lockdown.go`,
`alita/modules/settings_lockdown_lift_test.go`, `AGENTS.md`
**Applied fix:** `settingsLockdownLiftYes` redraws the page first (`p.edit`) and returns nil;
only then does a private menu call `lockdownAnnounceLift` (public `lockdown_lifted` notice,
D-26). A menu opened in the group skips the public post, because its redrawn menu already says
it in that same chat, and only calls `wakeLockdownWorker()`. The callback is still answered
once, before the lift, through `settingsPress.answer`, and the lift still goes only through
`applyLockdownLift`/`applyLockdownLiftOf`. Tests: edit-before-announce order in the bot's
call log (private), a group menu posts no public notice but wakes the worker, a private menu
posts exactly one notice and wakes the worker.

### WR-05: The private press path never checks the chat type of `c`

**Status:** fixed
**Commit:** 7c69874
**Files modified:** `alita/modules/settings_menu.go`, `alita/modules/settings_private_test.go`,
`AGENTS.md`
**Applied fix:** after `getChat(c)`, a result that is not `group` or `supergroup` is
`p.cannotOpen()` (`settings_cannot_open` plus its alert), like the deep link. New cases in
`TestSettingsPrivateMenu` use a channel and a private chat as `c`: one answer, the
cannot-open edit, no captcha row written. The test fails without the fix.

### IN-01: The documented test command skips one of the new tests, and one test asserts an English literal

**Status:** fixed
**Commit:** 2924f0f
**Files modified:** `AGENTS.md`, `alita/modules/settings_help_test.go`
**Applied fix:** the documented command is `-run '^(TestSettings|TestLoadSettings)'`, and
`TestLoadSettingsRegistersHelp` asserts the command names `/settings` and `/setfloodmode`
only.

### IN-02: `captchaWebModeRefusal` loads `captchaWebCurrent` twice

**Status:** fixed
**Commit:** dd3f922
**Files modified:** `alita/modules/captcha.go`
**Applied fix:** new `captchaWebRuntimeInstalled(rt, bot)`; `captchaWebInstalled` is a thin
wrapper over it, and `captchaWebModeRefusal` loads the runtime once and uses that value for
the installed check and the Redis `PING`. Refusal order and behaviour are unchanged; the
existing captcha mode tests pass unchanged.

### IN-03: The deep link answers differently for "bot not in chat" and "not an admin"

**Status:** fixed
**Commit:** 2e51158
**Files modified:** `alita/modules/settings.go`, `alita/modules/settings_private_test.go`,
`AGENTS.md`
**Applied fix:** a bot-missing or chat-gone error at the `getChatMember` step is answered
`settings_start_not_admin`. `settings_cannot_open` stays for the later steps (gone at
`getChat`, or a chat type that is not a group). No cooldown. The deep-link test and the
AGENTS.md deep-link bullet were adjusted.

### IN-04: The Link picker lists at most 8 Staff Groups, and one-match-plus-unknown becomes a picker

**Status:** fixed
**Commit:** 4f5f7f3
**Files modified:** `alita/modules/staff_link.go`, `alita/modules/settings_staff.go`,
`alita/i18n/settings_locale_test.go`, `locales/*.yml` (7), `AGENTS.md`, tests
**Applied fix:** `/linkstaff`'s decision is now one shared function, `decideOwnStaffGroup`;
Link without `s` uses it, so one match beside a group that could not be verified is
`staff_check_failed` instead of a one-button picker (several matches still make the picker,
where the command asks for an ID). A picker cut at 8 adds `settings_staff_pick_more` (new,
menu only, all 7 locales, `{n}` is the number missing). Tests for both, including a parity
check against the command.

### IN-05: Several refusal texts reused from the commands read oddly inside a menu, and a panic leaves the spinner running

**Status:** fixed
**Commit:** dc7d91b
**Files modified:** `alita/modules/settings_menu.go`, `alita/modules/settings_staff.go`,
`alita/i18n/settings_locale_test.go`, `locales/*.yml` (7), `AGENTS.md`, tests
**Applied fix (D-34):** new `settings_staff_no_group` (all 7 locales, no "send this command
again") is used for the no-Staff-Group refusal in the Link flow through
`settingsStaffLinkRefusalText`; other refusals keep the command's words. `settingsCallback`
builds the press before verifying it and defers an empty `answer` right after the recover
defer, so a recovered panic anywhere in the checks or a page still answers once (a normal
press has already answered, so it is a no-op). A test registers a panicking page action,
presses it, and asserts one answer, no edit, and that the menu works afterwards; it fails
without the deferred answer.

## Skipped Issues

None.

## Deviations and notes

- WR-01 (unlink wording): a stale Unlink card whose group is now linked to another Staff
  Group is answered with the existing `staff_unlink_not_linked` text ("This group is not
  linked to a Staff Group."), as instructed. The redrawn page below it shows the new link, so
  the line reads slightly off in that one case. A "this confirmation no longer applies"
  wording would be more exact if you want a follow-up.
- WR-01 (callback size): a Yes now carries a row ID. It fits 64 bytes up to 17 digits in
  both menu variants; an ID beyond that would be counted in `settingsScreen.Omitted` and the
  Yes left out (never a dead button), which a ~4 billion row ID cannot reach in practice.
- WR-04: the group-menu branch still wakes the lockdown worker; only the public post is
  skipped (D-26 as amended).

## Verification

All run in the main checkout `/home/user/Fuku_Robot` (not an isolated worktree), SQLite and
miniredis only, no PostgreSQL started.

| Command | Result |
|---|---|
| `go test -tags testtools -race -count=1 -v -run '^(TestSettings\|TestLoadSettings)' ./alita/modules ./alita/i18n` | pass |
| `go test -tags testtools -race -count=1 -run 'Captcha\|Antiflood\|Flood\|LinkStaff\|Unlink\|StaffLink\|StaffSeams\|Lockdown(Apply\|Alert\|Lift\|Announce\|Status)\|ApplyLockdown\|ApplyAutoLockdown' ./alita/modules` | pass |
| `CGO_ENABLED=0 go build ./...` | pass |
| `go vet -tags testtools ./alita/...` | clean |
| `gofmt -l alita` | empty |
| `make check-translations` | all present |
| `make check-docs` | no drift |

`make test` and `make lint` were not run (the orchestrator does that).

---

_Fixed: 2026-10-30_
_Fixer: Claude (gsd-code-fixer)_
_Iteration: 1_
