---
phase: 18-remaining-settings-pages
plan: 14
subsystem: settings-menu
tags: [telegram, settings-page, end-to-end, dispatcher, behaviour-parity]
status: complete

requires:
  - phase: 18-remaining-settings-pages
    provides: "plans 18-01 to 18-13 (the six pages Warnings n, Switches t, Language u, Log channel o, Anonymous admins y and Locks k, their apply functions, the commands behind them and the cross-page proofs)"
provides:
  - "TestSettingsPhase18EndToEnd (settings_phase18_e2e_test.go): eight subtests that drive each page's real buttons through the real dispatcher, with only Telegram faked, and assert what the bot then does in the group: warnings, locks, ai spam, anonymous admins, language, log channel, reports, auto approve"
  - "The phase18E2E toolkit: newPhase18E2E (the Deletion settings environment plus the module a subtest needs, cleanup of members, chats and rows), member, openPage, buttonFor, tap, post, report, callsAbout, warnCount, deleted, repliesWith, logPostsWith"
  - "AGENTS.md: the Testing bullet names the test, its subtests, its toolkit and the one fixture caveat (the bot's own record in the anonymous-admin subtest)"
affects: [18-15]

plan_head_before: ab168a4
plan_head_after: c90199532ec0ed20723514a72f594786ecb683de

actuals:
  tokens: 6300   # chars/4 over the added non-planning lines of ab168a4..c901995 (25278 chars), the test file and AGENTS.md
  tasks: 3
  commits: 3   # MEASURED: git rev-list --count ab168a4..HEAD (before the SUMMARY, state and a one-word AGENTS.md follow-up)

key-files:
  created:
    - fuku/modules/settings_phase18_e2e_test.go
  modified:
    - AGENTS.md

key-decisions:
  - "No production code changed: every behaviour held at HEAD, so there is no fix(18-14) commit."
  - "Every press is a real button of the drawn page (found by its callback fields on the page's keyboard and pressed with its own data), not a hand-built callback, so a page that drew the wrong cell or the wrong target value fails the test. The one hand-held press is the AI subtest's stale On button, kept from before the filter became unavailable on purpose."
  - "The log channel subtest uses /report as the real log producer (actionlog.Reports) and the page's own #SETTINGS audit as the second one, so the category gate is seen on a live bot action, not only on the audit post."
  - "The language subtest installs the real locale files on top of the marker locale (withRealLocales), because it must read the real Spanish words; this is the one subtest over a second (1.4 s, parsing eight locale files)."

patterns-established:
  - "buttonFor(keyboard, key, value, ...) finds a page's control by its decoded settings fields and refuses an odd argument list, so a missing key cannot silently match the first control of the page (it did once while writing the warnings subtest: a Kick press that was really the first mode button)."

requirements-completed: []

coverage:
  - id: E1
    description: "A warn limit and action set from the Warnings page are what the warn module applies when a member reaches the limit: Ban, Mute and Kick each applied once with the warns cleared, nothing applied below the limit, and a hand-written tban applies nothing and keeps the warns"
    requirement: "PAGE-01, PAGE-10 (contribution only)"
    verification:
      - kind: integration
        ref: "go test -tags testtools -race -count=1 -run '^TestSettingsPhase18EndToEnd$/^warnings$' ./fuku/modules"
        status: pass
    human_judgment: false
  - id: E2
    description: "A lock set from page 1 deletes a non-exempt member's link, an administrator's and an approved member's stay, /locks lists url as the page shows it, and unlocking from the page stops the deletion"
    requirement: "PAGE-06, PAGE-10 (contribution only)"
    verification:
      - kind: integration
        ref: "go test -tags testtools -race -count=1 -run '^TestSettingsPhase18EndToEnd$/^locks$' ./fuku/modules"
        status: pass
    human_judgment: false
  - id: E3
    description: "With the AI spam filter unavailable (provider value empty, global switch off) /aispam on and a stale On press refuse the same way and store no row; available, the page's On is what IsAISpamEnabled reads"
    requirement: "PAGE-02, PAGE-10 (contribution only)"
    verification:
      - kind: integration
        ref: "go test -tags testtools -race -count=1 -run '^TestSettingsPhase18EndToEnd$/^ai_spam$' ./fuku/modules"
        status: pass
    human_judgment: false
  - id: E4
    description: "With anonymous-admin mode off an anonymous admin's /warn gets the proof prompt and does not run, a non-owner administrator's press changes nothing and is answered with the owner-only alert, and after the creator turns the mode on from the page the same command runs with no prompt"
    requirement: "PAGE-05, PAGE-10 (contribution only)"
    verification:
      - kind: integration
        ref: "go test -tags testtools -race -count=1 -run '^TestSettingsPhase18EndToEnd$/^anonymous_admins$' ./fuku/modules"
        status: pass
    human_judgment: false
  - id: E5
    description: "The language chosen on the Language page is the one the bot's next group reply is in and the owner's own language is untouched; /report and /reports follow the Reports switch; a join request is approved once with Auto-approve on and not with it off; a report is logged and a page change audited only while their log categories are on"
    requirement: "PAGE-03, PAGE-04, PAGE-10 (contribution only)"
    verification:
      - kind: integration
        ref: "go test -tags testtools -race -count=1 -run '^TestSettingsPhase18EndToEnd$/^(language|log_channel|reports|auto_approve)$' ./fuku/modules"
        status: pass
    human_judgment: false
---

# Phase 18 Plan 14: The end-to-end proof that each page's setting is what the bot acts on Summary

**Each of the six new pages is proved, through the real dispatcher with only Telegram faked, to change what the bot does in the group and not only a stored row: warn limit and action, locks, the AI spam filter, anonymous-admin mode, the group language, log categories, reports and auto-approve. Test-only plus AGENTS.md; no production file changed.**

## What changed

- **Task 1, tracer (commit f1d22d4).** `settings_phase18_e2e_test.go` with `TestSettingsPhase18EndToEnd`, the `phase18E2E` toolkit and the `warnings` subtest. The owner opens the menu and the Warnings page, presses the stepper's real -1 button (3 to 2) and the Ban cell, and an administrator's `/warn` on a member twice through the dispatcher makes the second warn reach the limit: one `banChatMember`, no restrict or unban, the warns cleared and nothing applied after the first warn. Pressing Mute and then Kick on the same page does the same for one `restrictChatMember` with no rights and one member-removing `unbanChatMember`. A `tban` written straight on the row (then the cache dropped) applies no ban, restrict or unban and leaves both warns stored.
- **Task 2 (commit 78bdc1c).** `locks`: the owner presses the Links cell (`ur`) on page 1; a member's link message is deleted once (one `deleteMessage` of that message), a member's plain text, an administrator's link and an approved member's link are not; `/locks` answers `url = true`; the cell's own On-to-Off target unlocks and the next link stays, `/locks` answers `url = false`. `ai spam`: the page is opened while the filter is configured and its On button is kept; then, for each cause (empty provider value, global switch off), `/aispam on` answers `aispamRefusalKey()` and the stale On press answers the `settings_sw_ai_unavailable` alert, and no `ai_spam_settings` row exists; configured again, a fresh page's On makes `aispam.IsAISpamEnabled` true. `anonymous admins`: with the mode off an anonymous `/warn` (sender chat the group) gets exactly one proof prompt and stores no warn; a non-owner administrator's press on the page's toggle leaves the mode off and is answered `chat_status_owner_button_error`; the creator's press turns it on and the same command stores one warn with no second prompt.
- **Task 3 (commit c901995).** `language`: `/rules` in the group answers the default language's `rules_no_rules_set`; the owner presses the `es` cell; the next `/rules` answers the Spanish text, the chats row stores `es` and the owner's own `users.language` is not `es`. `log channel`: with a linked channel, a report posts `#REPORT`; the page turns the reports category off (one `#SETTINGS` audit post, no more `#REPORT` posts, the group still sees the report reply); with the settings category off a further change is not audited; back on, the next change is. `reports`: `/report` follows the Reports switch (replies by default, nothing after the page's Off, a reply again after On) and `/reports` shows the matching status. `auto approve`: a join request is not approved with the switch off, is approved once after the page's On and is not approved after Off. AGENTS.md: the Testing bullet names the test, its subtests, its toolkit and the fixture caveat below.

## Verification (real results)

- `CGO_ENABLED=0 go build ./...` ok; `go vet -tags testtools ./fuku/modules` ok; `gofmt -l fuku/modules/settings_phase18_e2e_test.go` empty; `git diff --exit-code 8c0c827 -- go.mod go.sum` exit 0; `make check-docs` "No drift detected"; `make check-translations` "All translations are present!"; no `t.Skip`, `reflect.Ptr` or `ioutil` in the file.
- `python3 .planning/phases/13-member-requirements/handoff/lint-local.py 8c0c827` exit 0 (read directly): "no dupl, gocyclo or godox findings on lines added since 8c0c827".
- Focused `^(TestSettingsPhase18EndToEnd|TestLegacy|TestSettingsMain|TestSettingsPhase18)` passes in the CI environment (22.1 s) and bare (21.5 s). `^TestSettingsPhase18EndToEnd$` passes with `-count=3` (13.6 s for the three runs: no state leaks between runs).
- Whole `./fuku/modules` package under CI's environment (`BOT_TOKEN`, `OWNER_ID`, `REDIS_ADDRESS`, `DATABASE_URL`, `DEBUG=false`, `-race -count=1 -timeout 40m -v`, log at `scratchpad/modules-18-14.log`): `ok  github.com/uasneppy/fuku_robot/fuku/modules  1676.716s`; count of `--- FAIL`, `--- SKIP` and `^FAIL` lines: 0. `./fuku/i18n`: `ok  github.com/uasneppy/fuku_robot/fuku/i18n  22.674s`.
- Time the plan adds to the whole-package run: `TestSettingsPhase18EndToEnd` took 3.95 s inside it (warnings 0.42, locks 0.37, ai_spam 0.35, anonymous_admins 0.38, language 1.40, log_channel 0.35, reports 0.34, auto_approve 0.34 seconds), so about 4 s, inside the 25 s budget and no subtest over 5 s. The language subtest is the slowest because it installs the eight real locale files.

## Deviations from Plan

None in scope: the plan was executed as written, test-only plus AGENTS.md, no `fix(18-14)` commit.

Notes that are not deviations:

- The plan's `language` behaviour names "a command that answers with a fixed text, such as `/rules` with no rules"; that is what the subtest uses (`LoadRules`, `rules_no_rules_set`, compared with the real `uk` and `es` locale text).
- The `log channel` subtest drives `/report` (the Reports category's real producer) next to the `#SETTINGS` audit, a stronger form than the plan's `/log user` example; `/log` and `/nolog` are covered by `TestSettingsParityLogCategory` and `TestSettingsDriftLogChannel`.
- Fixture caveat, recorded in AGENTS.md: a mode-on anonymous sender skips the cached admin list (`checkAnonAdmin` answers first), so `CanBotRestrict` falls back to the bot's own live `getChatMember` record; the Deletion environment's bot record cannot restrict, so the `anonymous admins` subtest sets a bot record that can (`lockdownBotAdminJSON(true, true, false)`). This is the fixture, not a behaviour change.
- Pre-existing behaviour seen, left as it is (outside this plan's scope): with the mode off, an anonymous admin's `/warn` gets the proof prompt and, right after it, `chat_status_restrict_cmd_error` from `standardModGates`' `CanUserRestrict` step, as every moderation command does for an anonymous sender. The test asserts the prompt and that the command did not run, not the number of replies.

## Known Stubs

None.

## Threat Flags

None. The test file adds no endpoint, auth path or schema; T-18-47 (a page that stores the right row while the bot acts on another value) is mitigated by this test for every page, T-18-48 (anonymous-admin mode turned on by someone the command refuses) by the `anonymous admins` subtest (a non-owner's press changes nothing and is answered with the owner-only alert; the creator's press lifts the proof), and T-18-SC by the unchanged `go.mod` and `go.sum`.

## Requirements

`requirements-completed: []`. The plan contributes to PAGE-01 to PAGE-06 and PAGE-10 (behaviour parity seen from the group) and completes none; plan 18-15 completes them after its own gate.

## Self-Check: PASSED

- `fuku/modules/settings_phase18_e2e_test.go` exists and defines `TestSettingsPhase18EndToEnd` with the subtests `warnings`, `locks`, `ai spam`, `anonymous admins`, `language`, `log channel`, `reports` and `auto approve`.
- Commits f1d22d4, 78bdc1c and c901995 are on the branch (`git merge-base --is-ancestor` of each against HEAD).
- `grep -c 'TestSettingsPhase18EndToEnd' AGENTS.md` is at least 1.
