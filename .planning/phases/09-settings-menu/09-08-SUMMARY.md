---
phase: 09-settings-menu
plan: 08
subsystem: settings-menu
tags: [settings, captcha, turnstile, telegram, menu]
requires:
  - phase: 09-01
    provides: press pipeline, page registry, settingsRight predicates, Captcha time stepper
  - phase: 09-05
    provides: settings_captcha_* and settings_opt_* locale keys
  - phase: 09-06
    provides: applyCaptchaEnabled, applyCaptchaMode, applyCaptchaAction, captchaWebInstalled
provides:
  - Captcha page with on/off toggle, mode choice, time stepper and failure action choice
  - settingsCaptchaWeb and the full renderSettingsCaptcha(view, settings, web, notice)
affects: [settings menu Main page, Captcha page tests]
tech-stack:
  added: []
  patterns: [one apply function per setting shared by command and menu, redraw from the stored row]
key-files:
  created:
    - alita/modules/settings_captcha_test.go
  modified:
    - alita/modules/settings_captcha.go
    - alita/modules/settings_test.go
    - alita/modules/settings_menu_test.go
    - alita/modules/settings_private_test.go
key-decisions:
  - "Mode and action values only need the shape of a lower-case word; the apply function then refuses an unknown word with the command's own text"
  - "A stored Turnstile on an unavailable web captcha shows both the selected mark and the unavailable mark"
patterns-established:
  - "settingsCaptchaRedraw: every control press redraws from the apply result's read-back row, or a fresh read"
requirements-completed: [MENU-03, MENU-07, MENU-08]
status: complete
duration: 55min
completed: 2026-10-30
actuals:
  tokens: 36000
  tasks: 2
  commits: 3
commits: 3
plan_head_before: b3c964cbc69d121a2911ef0e7ac19cdd43864331
plan_head_after: 327721480bf2c4433b9c549691d433a0c1f273b4
---

# Phase 9 Plan 08: Captcha page Summary

**The Captcha page now switches the captcha on or off, picks its mode (with Turnstile's three ordered refusals) and its failure action, each through the apply function its command uses, next to the time limit stepper.**

## Performance

- **Tasks:** 2 of 2
- **Files:** 1 created, 4 modified (2 of them older tests, see Deviations)

## Accomplishments

- Toggle (`t=e`, right `settingsRightAdminBotAdmin`), mode (`t=m`, `settingsRightAdmin`) and action (`t=f`, `settingsRightAdmin`) controls registered on the Captcha page; time (`t=t`) is unchanged.
- The off press is answered empty before `applyCaptchaEnabled` runs its cleanup, so a slow cleanup cannot outlast the callback (D-31). A failed cleanup shows the command's `captcha_disable_failed` notice on a page that shows Off.
- Turnstile is labelled with a `🚫 ` prefix when `captchaWebInstalled` is false (no I/O at render). Pressing it runs `applyCaptchaMode`, so the three ordered refusals of `captchaWebModeRefusal` show as notices and nothing is stored.
- A stored `turnstile` mode while `captchaWebAvailable` is false adds `settings_captcha_fallback_note` under the mode line.
- Tests: `TestSettingsCaptchaPage`, `TestSettingsCaptchaToggle`, `TestSettingsCaptchaTurnstile`, `TestSettingsParityCaptcha`, `TestSettingsDriftCaptcha`, `TestSettingsAuthorityParityCaptcha`, `TestSettingsCaptchaCallbackBytes`, `TestSettingsCaptchaPageFitsEveryLocale`. All pass with `-race`. The private Turnstile choice is exactly 53 bytes, all callback data fits 64 bytes with nothing omitted, and the page is within 3800 UTF-16 units in all 7 locales with a 64-rune worst-case title.

## Task Commits

1. RED tests for the page, toggle and Turnstile: `b32de92` (test)
2. Controls, render and setters: `78ef8c8` (feat)
3. Parity, drift, authority parity, bytes and length tests: `3277214` (test)

## Deviations from Plan

**1. [Rule 3 - Blocking] Existing tests assumed the two-row Captcha page**
- **Found during:** Task 1 (running the plan's verify command) and Task 2 (broader `TestSettings` run)
- **Issue:** `settingsStepper` in `settings_test.go` required exactly two keyboard rows, `settings_private_test.go` indexed `keyboard[0]` as the stepper in two places, and `settings_menu_test.go` called the old three-argument `renderSettingsCaptcha`.
- **Fix:** The stepper is found by its control (`t=t`) instead of its position; the render call passes `settingsCaptchaWeb{}`. No assertion was weakened.
- **Files modified:** `alita/modules/settings_test.go`, `alita/modules/settings_private_test.go`, `alita/modules/settings_menu_test.go`
- **Commits:** `78ef8c8`, `3277214`

**2. [Rule 1 - Plan inconsistency] Strict value parsing versus the drift test**
- **Found during:** Task 1 design
- **Issue:** The plan says each setter answers the alert `common_callback_invalid_request` for any value outside the allowed set, but `TestSettingsDriftCaptcha` expects a hand-built `image` mode to show the command's `captcha_mode_invalid` notice (and `warn` to show `captcha_action_invalid`).
- **Fix:** The mode and action setters reject only a value that is not 1 to 16 lower-case ASCII letters (invalid-request alert); a well-shaped word the command would refuse goes to the apply function and shows its own notice. The toggle stays strictly `0` or `1`. Either way nothing is stored, and the menu refuses exactly what the command refuses.
- **Files modified:** `alita/modules/settings_captcha.go`

**3. Commit trailer**
- The plan text prints `Co-Authored-By: Claude Opus 5.5`; the commits carry the trailer lines the harness supplied (`Claude Sonnet 5.5`), as the executor notes require.

## Known Stubs

None.

## Threat Flags

None. No new endpoint, auth path, file access or schema. T-09-28, T-09-29 and T-09-30 are mitigated and tested (`TestSettingsAuthorityParityCaptcha`, `TestSettingsCaptchaTurnstile`, `TestSettingsDriftCaptcha`, `TestSettingsCaptchaToggle`).

## Verification Notes

- `CGO_ENABLED=0 go build ./...`, `go vet -tags testtools ./alita/modules/`, `gofmt -l alita/` (empty) and `make check-translations` pass. `make lint` could not run here (golangci-lint is built with go1.25 against module go1.26.0), so `gofmt` and `go vet` stood in for it.
- `go test -tags testtools -race -count=1 -run '^(TestSettings|TestCaptcha)' ./alita/modules` passes (51 s). The whole package and `make test` were not run (the orchestrator runs them between waves).
- The RED commit was made before the implementation commit, but the tests were not run against the base commit in isolation, so "red first" is by commit order only.

## Pending human verification (deferred to 09-SPIKE.md, not blocking)

1. In a real supergroup with the bot as administrator holding Restrict members, open `/settings`, Captcha, press the toggle On, Math, Text and Turnstile (with the web captcha configured) and Kick, Ban, Mute; run `/captcha` and confirm the same values.
2. With the web captcha unset, confirm the Turnstile button shows the `🚫` mark and pressing it shows the "not set up" notice and keeps the stored mode.
3. With a member pending a captcha, press Off and confirm the member is released and unmuted.

## Self-Check: PASSED

- `alita/modules/settings_captcha.go`, `alita/modules/settings_captcha_test.go`: present.
- Commits `b32de92`, `78ef8c8`, `3277214`: present on `worktree-agent-a12b8e37ee8c2f959`.
- Eight named tests report `--- PASS`.
