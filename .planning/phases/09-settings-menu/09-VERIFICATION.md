---
phase: 09-settings-menu
verified: 2026-10-30T13:30:00Z
status: passed
score: 5/5 roadmap success criteria verified by code and automated tests (live-client behaviour needs the owner)
behavior_unverified: 0
overrides_applied: 0
gaps: []
human_verification:
  - test: "Inline keyboard edit-in-place and Back/Close on Android, iOS, Desktop and Web (09-SPIKE.md checks 1, 4, 6, 7)"
    expected: "/settings shows the two-button choice; Open here turns that same message into Main; every page edits the one message; Back returns to Main; Close deletes it; forwarded menu and forum-topic group behave"
    why_human: "Suite fakes Telegram (hand-written BotClient); only real clients show edit-in-place, alert rendering and forum topics"
  - test: "Open in private deep link /start settings_<chat id> for an admin who never started the bot (check 2)"
    expected: "Telegram shows Start, then the private Main page for exactly that group; a non-admin gets settings_start_not_admin; a bot-less chat gets the same words"
    why_human: "URL button into /start and the Start-button flow exist only in real clients"
  - test: "Demoted admin and another member pressing the opener's menu; anonymous-admin proof flow (check 3, TestSettingsAnonymousAdmin covers the code path)"
    expected: "Other member gets 'not your menu'; a demoted opener gets 'not admin'; an anonymous admin gets the proof button and then opens the menu as the tapper"
    why_human: "Real demotion timing and Telegram's anonymous-admin message shape"
  - test: "Fast stepper tapping and 429 on editMessageText (check 8); Close on a private menu older than 48 hours (check 9)"
    expected: "Absolute-value steppers end at the last tapped value, a 429 is logged once and never undoes the write; an old private message edits to settings_closed when delete fails"
    why_human: "Real rate limits and the 48-hour delete rule"
  - test: "Staff Group page after a slow build, picker with 9+ groups, Link/Unlink from a group menu, Lift from group and private menu (checks 10, 11, 12)"
    expected: "Callbacks stay valid after the build; nothing in a group menu or the group names the Staff Group; the public lockdown_lifted notice is posted once"
    why_human: "Needs real groups, real getChatAdministrators latency and the live pacer"
  - test: "Button text truncation in hi and ru on a phone; native-speaker read of the 83 settings_* strings in es, fr, hi, id, pt, ru (checks 5, 14)"
    expected: "Labels readable, no cut-off that hides the state word; natural wording"
    why_human: "Rendering and translation quality"
  - test: "Captcha, Antiflood and Lockdown trigger pages against the commands in a real group (check 13)"
    expected: "A value changed in the menu equals /captchamode, /captchatime, /setflood, /autolockdown output and vice versa"
    why_human: "Cross-replica 10 s local-cache skew and the live bot-rights checks are only seen live"
---

# Phase 9: Settings Menu Verification Report

**Phase Goal:** As a group admin, I want to manage my group from one button menu, so that staff, lockdown, captcha and antiflood need no commands.
**Verified:** 2026-10-30 (HEAD e660367, branch claude/cool-hamilton-hvs9nl)
**Status:** passed (every automated truth holds; the deferred live-client checks are listed above and were accepted by the owner on 2026-10-30, see Human Verification Outcome)
**Re-verification:** No, initial verification

## Goal Achievement

| # | Truth (ROADMAP success criterion) | Status | Evidence |
|---|---|---|---|
| 1 | `/settings` asks group vs private; group menu only answers its opener; private menu manages the originating group | VERIFIED | `settings.go`: `settingsDesc` = `WrapCommand` with `requireLockdownGroup()` + `requireLockdownAuthority(false)` (live getChatMember), `settingsChoiceScreen` builds Open here (`a=oh`, `u`=opener) and Open in private (URL `?start=settings_<id>`). `settingsStartDeepLink` parses strictly, live-checks creator/admin, `getChat` group/supergroup only. `settingsPress.verify` step 3: `query.From.Id != p.opener` -> `settings_not_your_menu`; `c` accepted only on private message, `u` only on non-private. Tests PASS: `TestSettingsCommand`, `TestSettingsOwnership`, `TestSettingsStartDeepLink`, `TestSettingsPrivateRoundTrip`, `TestSettingsPrivateMenu`, `TestSettingsAnonymousAdmin` |
| 2 | One message edited in place with Back and Close on every page; every press re-checks live admin status | VERIFIED | `p.edit` -> `EditText` on the pressed message; `navRow(false)` ends every page, card, picker and the lockdown/staff variants (`settings_captcha.go:179`, `settings_antiflood.go:152`, `settings_lockdown.go:157/166/208`, `settings_staff.go:150/166/225/463`, `settings_card.go:82`); Main has Close only. `verify()` step 4 is a live `lockdownLiveMember` (`getChatMember`) of the presser in the group being configured on every press (admin cache never used); demoted -> `settings_not_admin`, lookup error -> `settings_check_failed`; a private menu whose bot/group is gone closes itself (D-11). Close = delete, fallback edit to `settings_closed`. Tests PASS: `TestSettingsPressPipeline`, `TestSettingsRights`, `TestSettingsNavigation` (Back, not-modified is success, 429 not retried), `TestSettingsCallbackBytes`, `TestSettingsKeyboardHelpers` |
| 3 | Staff Group page shows the /staff panel information; only the owner of both groups links/unlinks | VERIFIED | `settings_staff.go`: page reads the panel through `buildStaffPanelRows`/`renderSettingsStaffPanel` (read-only inside a Staff Group); Link runs `listVerifiedOwnStaffGroups` -> `resolveNamedStaffGroup` (live creator check) -> `linkToStaffGroup` + `announceStaffLink`, the code `/linkstaff` runs; Unlink Yes runs `runUnlinkGroup` (re-checks creator of both groups) and is bound to the link row `l` (D-33). D-21 masking: `settingsStaffViewFor` hands title/ID only to a private menu of the group's live creator (D-32) or a Staff Group's own page. Tests PASS: `TestSettingsStaffPageVariants`, `TestSettingsStaffReadOnly`, `TestSettingsStaffLink`, `TestSettingsStaffPick`, `TestSettingsStaffUnlink`, `TestSettingsAuthorityParityStaff`, `TestSettingsStaffPrivacy` (all 7 languages, group menu end to end), `TestSettingsCardExpiry` |
| 4 | Lockdown page shows state, working Lift, on/off + threshold per trigger; Captcha page sets mode and time limit; Antiflood page configures antiflood | VERIFIED | Lockdown (`settings_lockdown.go`): state lines, Lift ask/Yes (`settingsRightRestrict`, two-tap card, expiry, only through `applyLockdownLiftOf`, bound to lockdown row `r`, public notice via `lockdownAnnounceLift` from a private menu), three triggers each with toggle + stepper (-10,-1,+1,+10) writing through `applyAutoLockdown`. Captcha (`settings_captcha.go`): enabled, mode (math/text/turnstile), time 1..10, failure action via `applyCaptchaEnabled/Mode/Timeout/Action`. Antiflood (`settings_antiflood.go`): limit stepper incl. Off, action, delete switch via `applyFloodLimit/Mode/Delete`. Tests PASS: `TestSettingsLockdownPage`, `TestSettingsLockdownTriggers`, `TestSettingsLockdownLift*` (Outcomes, Twice, Expired, Authority, Private, BindsItsLockdown, RedrawsBeforeAnnouncing), `TestSettingsCaptchaPage/Toggle/Turnstile`, `TestSettingsAntifloodPage/Stepper/FreshChat/FirstWriteNotice/StoredLegacyAction`, `TestSettings*ReadFailure` |
| 5 | A setting changed in the menu is identical to the same change by command, and both show the same value | VERIFIED | One apply function per setting, called by both the command and the menu (`captcha.go:546/670/813/872`, `antiflood.go:475/488/496`, `autolockdown.go:281`, `lockdown.go:456/471`); repositories invalidate their cache key on every write (`captcha/repository.go` x5 `DeleteCache("captcha_settings")`, `antiflood/repository.go:40`, `raiddetection/repository.go:133`); DB errors are returned as typed outcomes, never dropped; pages read through the commands' getters with real errors (read failure = cannot-open edit). Steppers/toggles send absolute `v`. Tests PASS: `TestSettingsParityCaptcha`, `TestSettingsParityCaptchaTime`, `TestSettingsParityAntiflood`, `TestSettingsParityLockdownTriggers`, `TestSettingsParityLockdownLift`; drift `TestSettingsDriftCaptcha/CaptchaTime/Antiflood/LockdownTriggers`; authority `TestSettingsAuthorityParityCaptcha/Antiflood/LockdownTriggers/Staff` |

**Score:** 5/5 truths verified; 0 behavior-unverified in the automated sense.

### Required Artifacts / Key Links

| Artifact | Status | Details |
|---|---|---|
| `alita/modules/settings.go`, `settings_menu.go`, `settings_pages.go`, `settings_card.go` | VERIFIED | Registered in `init()` (priority 239), `RegisterAnonymousAdminHandler("settings")`, `RegisterDeepLinkHandler("settings_")`, callback `callbackquery.Prefix("settings|")` with no `SetAllowChannel`; `registerSettingsPage` panics on duplicate key/code |
| `settings_staff.go`, `settings_lockdown.go`, `settings_captcha.go`, `settings_antiflood.go` | VERIFIED | Each page self-registers from its own `init`; every control declares a `settingsRight`; unknown right fails closed |
| Locales | VERIFIED | 83 `settings_*` keys in each of en/es/fr/hi/id/pt/ru; `make check-translations` clean; `alita/i18n` settings limits tests PASS |
| Callback answering | VERIFIED | `answer()` is the only path (no `query.Answer` in settings files); deferred `press.answer("")` after `RecoverFromPanic` guarantees exactly one answer even on panic; tests count answers |

### Behavioral Spot-Checks and Commands Run

| Check | Result |
|---|---|
| `CGO_ENABLED=0 go build ./...` | PASS |
| `go test -tags testtools -race -count=1 -run '^(TestSettings\|TestLoadSettings)' ./alita/modules ./alita/i18n` | PASS (modules 74.5 s, i18n 5.1 s), no `t.Skip` in settings tests |
| `make check-translations` | PASS, all 7 locales complete |
| `make check-docs` | PASS, no drift |
| Whole `make test` | not re-run (orchestrator: 64 packages ok) |
| PostgreSQL | not started; Phase 9 has no migration, index or CHECK, so no PostgreSQL-only guarantee applies |

### Requirements Coverage

| Requirement | Status | Evidence |
|---|---|---|
| MENU-01 choice in group vs private | SATISFIED | truth 1 |
| MENU-02 opener-only group menu; private menu manages its group | SATISFIED | truth 1 (`u`/`c` owner fields, step 3, private `getChat`) |
| MENU-03 live admin re-check each press | SATISFIED | truth 2 (`verify()` step 4, `TestSettingsPressPipeline`, `TestSettingsRights`) |
| MENU-04 edit in place, Back and Close | SATISFIED | truth 2 |
| MENU-05 Staff Group page, owner-only link/unlink | SATISFIED | truth 3 |
| MENU-06 Lockdown page: state, Lift, trigger switches/thresholds | SATISFIED | truth 4 |
| MENU-07 Captcha (mode, time) and Antiflood pages | SATISFIED | truth 4 |
| MENU-08 menu identical to command | SATISFIED | truth 5 |

No orphaned requirements.

### Anti-Patterns Found

None blocking. No TBD/FIXME/XXX in the settings files; no discarded DB errors on write paths; all 10 review findings (WR-01..05, IN-01..05) are fixed per 09-REVIEW-FIX.md and the fixes (D-32 creator-only Staff Group title in private menu, D-33 object-bound Lift/Unlink Yes, D-34) are present in code and covered by tests (`TestSettingsLockdownLiftBindsItsLockdown`, `TestSettingsStaffUnlink`, `TestSettingsStaffPrivacy`).

Accepted, documented limits (not gaps): `captcha_settings`/`antiflood` keys not in `skipLocal` so another replica can show an old value for up to `CACHE_LOCAL_TTL` (same as the commands); stale-cache write races inherent to the existing repositories.

### Human Verification Required

See the `human_verification` list in the frontmatter: the 14 checks of `09-SPIKE.md` collapse into seven live items (edit-in-place and Back/Close on all clients, the Open in private deep link, demoted/other-member and anonymous-admin flows, stepper/429/48-hour Close, Staff Group build and Link/Unlink/Lift in real groups, hi/ru truncation and translation read, and command-vs-menu parity in a real group).

### Gaps Summary

No gaps. Every MENU requirement and every ROADMAP success criterion has implementing code and an automated test that asserts observable behaviour. The phase cannot be marked fully `passed` only because inline-keyboard editing, the deep-link Start flow, Close deletion, 429 behaviour and anonymous-admin proof are Telegram-client behaviours the suite fakes.

---

_Verified: 2026-10-30_
_Verifier: Claude (gsd-verifier)_

## Human Verification Outcome

On 2026-10-30 the owner reviewed the seven `human_verification` items above (the 14 live checks of `09-SPIKE.md`) and chose to accept the phase and test them live later. The status moved from `human_needed` to `passed` on that basis. The checks were not run: they need a real bot, real groups, real Telegram clients on Android, iOS, Desktop and Web, and a PostgreSQL-free but multi-replica deployment. They stay open as deferred live checks, with exact steps in `09-SPIKE.md`:

- edit-in-place, Back and Close on every client, a forwarded menu and a forum-topic group;
- the Open in private button and `/start settings_<chat id>` for an admin who never started the bot;
- a demoted opener, another member pressing the opener's menu, and the anonymous-admin proof flow;
- fast stepper taps with a 429 on `editMessageText`, and Close on a private menu older than 48 hours;
- the Staff Group page after a slow build, the picker with 9 or more groups, Link and Unlink from a group menu, and a Lift from a group and from a private menu posting one public notice;
- button truncation in hi and ru on a phone, and a native-speaker read of the 83 `settings_*` strings in es, fr, hi, id, pt and ru;
- the Captcha, Antiflood and Lockdown trigger pages against the commands in a real group, including the 10 s local-cache skew between replicas.

