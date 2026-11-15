---
phase: 15-ai-spam-name-check
plan: 08
subsystem: settings-menu
tags: [member-requirements, ai-spam-name, settings-page, off-only-form, press-gate, rule-table]

requires:
  - phase: 15-ai-spam-name-check
    provides: 15-03 (the ten AI locale keys), 15-04 (the eighth rule row, memberReqAIConfigured), 15-07 (RemedyKey, the button's AI step)
provides:
  - memberReqRule.Configured, UnconfiguredKey and ConfirmKeys, set on the AI row (no renderer or gate names the AI rule)
  - settingsMreqStatus.AINotConfigured, set at every return of settingsMreqReadStatus (a wrapper over settingsMreqReadChannelStatus)
  - settingsMreqRuleUnconfigured, settingsMreqUnconfiguredText and settingsMreqUnconfiguredLine; settingsMreqHereLines, settingsMreqRuleSlot and settingsMreqOffOnly read them
  - settingsMreqAIGate (called by settingsMreqChoose and settingsMreqYes) and settingsMreqRedrawRefused (shared with the channel gate)
  - the AI card's own consequence sentence (settings_mreq_confirm_ai_*) through ConfirmKeys
  - TestSettingsMreqAIStatus, TestSettingsMreqAIGate, withAINotConfigured
affects: [15-09, 15-10, 15-11]

plan_head_before: 582af9c92bfbbb1aade092b2efad6bf0b1b1958f
plan_head_after: 4e6ccf2353e51384ca7bec1d0853880f98bac078
actuals:
  tokens: 15000
  tasks: 3
  commits: 6

tech-stack:
  added: []
  patterns:
    - "A rule's process-level precondition is a table field (Configured func() bool) read by one helper, so the status, the slot, the Off-only test and the gate never test the rule's identity"
    - "A refused press repairs the stale screen through one shared redraw helper; the AI gate has no unknown branch because the fact is process-level"

key-files:
  created:
    - fuku/modules/settings_mreq_ai_test.go
  modified:
    - fuku/modules/memberreq_rules.go
    - fuku/modules/settings_mreq.go
    - fuku/modules/memberreq_ai_fake_test.go
    - fuku/modules/settings_mreq_test.go
    - fuku/modules/settings_mreq_card_test.go
    - fuku/modules/settings_mreq_lookup_test.go
    - fuku/modules/settings_staff_privacy_test.go
    - AGENTS.md

key-decisions:
  - "The status holds one boolean (AINotConfigured) for the one rule with a process-level precondition; settingsMreqReadStatus sets it from the table at every return and a comment says a later rule adds its own field and one line to settingsMreqRuleUnconfigured"
  - "settingsMreqAIGate reads rule.Configured() live (a press has no status) and redraws with settingsMreqStatus{AINotConfigured: true}, which is the same Off-only screen for the AI rule in any group type, so it never reads the group type"
  - "ConfirmKeys was added to the struct in the gate commit, where renderSettingsMreqCard first reads it, so no commit leaves an unused field"

requirements-completed: []

coverage:
  - id: D1
    description: "with the AI not configured the AI screen says so in words, offers [Off] alone (two rows, no choose or actions hint, the stored value visible and inert) and the page lists the line after the group-level line; configured it is the usual three rows; the six combinations in both menus, Redis down, and the status makes no call"
    requirement: "MREQ-10, MREQ-01 (contribution only; completes none)"
    verification:
      - kind: integration
        ref: "TestSettingsMreqAIStatus (24 combinations plus the Redis-down case, -count=3), TestSettingsMreqOpenCost"
        status: pass
    human_judgment: false
  - id: D2
    description: "a punishing AI press (st v 1 to 4, the card's Yes, a forged Yes) with the AI not configured is refused with the status line's words, nothing written, no card, the Off-only screen redrawn, one answer, no provider request; Off, the stored value, an expired Yes and a demoted presser are not refused by the gate; the gate judges the AI rule only; both stale-screen directions"
    requirement: "MREQ-10 (contribution only; completes none)"
    verification:
      - kind: integration
        ref: "TestSettingsMreqAIGate (-count=3)"
        status: pass
    human_judgment: false
  - id: D3
    description: "the AI card carries settings_mreq_confirm_ai_* and never the generic sentence; the table-driven tests cover the AI rows"
    requirement: "MREQ-01 (contribution only; completes none)"
    verification:
      - kind: integration
        ref: "TestSettingsMreqCard, TestSettingsMreqCallbackBytes, TestSettingsDriftMreq, TestSettingsAuthorityMreq, TestSettingsControlActionAllowList, TestApplyMemberReqAction, TestSettingsMreqForgedFields"
        status: pass
    human_judgment: false
  - id: D4
    description: "every screen fits 3800 units and 64-byte buttons in 8 languages under all eight statuses, nothing names the Staff Group, the channel or a member in a group menu and a private menu of a non-creator admin"
    requirement: "MREQ-10 (contribution only)"
    verification:
      - kind: integration
        ref: "TestSettingsMreqPageFitsEveryLocale, TestSettingsStaffPrivacy, and the whole settings family (366.7 s)"
        status: pass
    human_judgment: false

duration: about 70 min (not timed at start; estimated from the session)
completed: 2026-11-09
status: complete
---

# Phase 15 Plan 08: the AI line, the Off-only form and the press gate Summary

**The Requirements page now says in words when the AI name check cannot run (a code-side 🚫 line after the group-level line, and in the AI screen's one slot, where it wins over the basic-group line), the AI screen is `[Off]` alone while it cannot, a punishing AI press from a stale screen, a stale card or a forged button is refused by the pure `settingsMreqAIGate` with nothing written, and the AI card confirms with its own true sentence; every difference is a table field (`Configured`, `UnconfiguredKey`, `ConfirmKeys`), so no renderer or gate names the AI rule.**

## Accomplishments

- **Table and status (`memberreq_rules.go`, `settings_mreq.go`).** `memberReqRule` gained `Configured`, `UnconfiguredKey` and `ConfirmKeys`, set on the AI row (`memberReqAIConfigured`, `settings_mreq_here_ai`, the three `settings_mreq_confirm_ai_*`). `settingsMreqStatus.AINotConfigured` is set by the `settingsMreqReadStatus` wrapper at every return of `settingsMreqReadChannelStatus` (the `group` return and the page-scope return included), from the table, with no context, no call and no Redis. `settingsMreqHereLines` now builds the group-level lines first and appends each unconfigured rule's line while its action is not Off; `settingsMreqRuleSlot` returns that line first; `settingsMreqOffOnly` is true for an unconfigured rule.
- **Gate and card.** `settingsMreqAIGate(p, rule, stored, action)` runs where `settingsMreqChannelGate` runs (after the fresh same-value read in `settingsMreqChoose`, after the expiry check in `settingsMreqYes`), answers the alert as the press's one answer, writes nothing, draws no card and redraws the Off-only screen through `settingsMreqRedrawRefused` (which the channel gate now shares, so there is no dupl clone). `renderSettingsMreqCard` reads `rule.ConfirmKeys[action]`.
- **Tests.** `TestSettingsMreqAIStatus`: both menus, supergroup and basic group, configured and not, stored Off, Warn and Ban (24 subtests), the slot's one line, the Off-only rows, hints and check mark, the basic-group line still once, every other screen in a basic group, the pipeline's own request counts, and a Redis-down and switch-off case. `TestSettingsMreqAIGate`: 11 subtests per menu (the refusals with both ways of not being configured, the card's Yes and a forged Yes, Off and the stored value, the expired Yes, the demoted presser for `st` and `my`, the other rules, both stale-screen directions, the configured path in a supergroup and a basic group), each asserting one answer, no write, no provider request and the redraw. The table-driven tests gained the AI rows (see "Tests that changed").
- **AGENTS.md.** The page `m` bullet (the card sentence, the AI line paragraph, the gate paragraph), the rights bullet, and the Testing list.

## Task Commits

1. `d93be6b` test(15-08): the page says when the AI rule cannot work (red: 13 unconfigured cases failed)
2. `cc9cef7` feat(15-08): say on the page when the AI rule cannot work (the tracer; green)
3. `a740d7f` test(15-08): refuse a punishing AI press while the AI is not configured (red: the gate and the card keys)
4. `026d376` feat(15-08): refuse a punishing AI press while the AI is not configured and confirm with the AI's own sentence
5. `a1b2a05` test(15-08): the AI screens fit every language and name nobody
6. `4e6ccf2` docs(15-08): describe the AI line, the Off-only form and the press gate

`commits: 6` is `git rev-list --count 582af9c..HEAD` (the ledger file `gsd-plan-head-before-15-08` holds `582af9c9...`), before this SUMMARY's own commit.

## Verification (real results)

- Task 1 (`-run '^(TestSettingsMreqAIStatus|TestSettingsMreqRowBudget|TestSettingsMreqChannelStatus|TestSettingsMreqBasicGroup)$' -v ./fuku/modules`): all four PASS, no FAIL or SKIP. Acceptance: `grep -c AINotConfigured settings_mreq.go` 4, `grep -c 'Configured: memberReqAIConfigured' memberreq_rules.go` 1, no non-comment `memberreq.RuleAI` in `settings_mreq.go`, `git diff --exit-code 62ee0d1 -- fuku/modules/settings_pages.go` exit 0.
- Task 2 (`-run '^(TestSettingsMreqAIGate|TestSettingsMreqCard|TestSettingsMreqCallbackBytes|TestApplyMemberReqAction|TestSettingsDriftMreq|TestSettingsAuthorityMreq|TestSettingsControlActionAllowList|TestSettingsMreqRuleScreen)$' -v`): all eight PASS. `-run '^(TestSettingsMreqChannelRefusal|TestSettingsMreqForgedFields|TestSettingsMreqOpensFromMain|TestSettingsMreqSwitches)$'`: ok. `-count=3 -run '^(TestSettingsMreqOpenCost|TestSettingsMreqAIStatus|TestSettingsMreqAIGate)$'`: ok (83.3 s). Acceptance: `grep -c settingsMreqAIGate settings_mreq.go` 5, `grep -c ConfirmKeys settings_mreq.go` 2, `func TestSettingsMreqAIGate` 1.
- Task 3: the whole settings family `go test -tags testtools -race -count=1 -run '^(TestSettings|TestLoadSettings)' ./fuku/modules ./fuku/i18n`: `ok fuku/modules 366.666s`, `ok fuku/i18n 5.693s`, exit 0. `go vet -tags testtools ./fuku/modules`: exit 0. `python3 .planning/phases/13-member-requirements/handoff/lint-local.py 8c0c827`, exit read directly: 0 ("no dupl, gocyclo or godox findings on lines added since 8c0c827"). `test -z "$(~/.cache/fuku-lint-tools/gocyclo -over 19 fuku/modules/settings_mreq.go fuku/modules/memberreq_rules.go)"`: exit 0. `CGO_ENABLED=0 go build ./...`: exit 0. `git diff --exit-code 8c0c827 -- go.mod go.sum`: exit 0. `git diff --exit-code 62ee0d1 -- fuku/modules/settings_pages.go`: exit 0. `grep -c settingsMreqAIGate AGENTS.md` 2, `grep -c AINotConfigured AGENTS.md` 2.
- Plan-level: `-run '^(TestMemberReqRuleTable|TestMemberReqRuleKeysExist|TestMemberReqAI|TestMemberReqLookupAlertsFitEveryLocale)' ./fuku/modules`: ok (68.6 s), exit 0.
- Not run: `make lint` and gosec (cannot run here), `make test` (the closing plan 15-11), PostgreSQL (nothing here rests on it; the module harness runs on SQLite), `node --test scripts/captcha-page` (the page was not changed).

## Tests that changed

- `TestSettingsMreqRuleScreen`, `TestSettingsMreqCard` and the demotion subtest of `TestSettingsAuthorityMreq` now start with `withAIConfigured(t)`: they are the Phase 13 and 14 tests of the usual three-row screen, and the test process has no Anthropic key, so without it the AI screen is the Off-only form and a punishing press is refused (both are proved by the new tests). `TestSettingsMreqBasicGroup`'s "every rule screen" subtest does the same, so the AI screen there is still the basic-group one.
- `settingsMreqStatuses` gained "ai not configured" and "ai not configured in a basic group" rows and `settingsMreqOffOnlyScreen` learned the AI form; `TestSettingsMreqPageFitsEveryLocale`'s screen count is now computed from the table; `everyPunishment` in `TestSettingsMreqRowBudget`, `settingsMreqScreens` and the privacy test set `AIAction`.
- `settingsMreqWantCard` takes the consequence key per rule (the AI keys for `ai`) and fails when the AI card carries a generic sentence or another rule's card carries an AI one.
- `TestSettingsDriftMreq` seeds the AI rule (and a new subtest changes only the AI column), `TestSettingsControlActionAllowList` has an AI subtest, `TestApplyMemberReqAction` covers `RuleAI`, `TestSettingsMreqForgedFields` has nine AI cases refused in step 1, `TestSettingsMreqMainState` has an inert-value case, `TestSettingsMreqOpenCost` gained the AI cost cases, `TestSettingsStaffPrivacy` renders the two AI statuses, the AI alert and a group-menu and private-menu Phase 15 run.

## Deviations from Plan

1. **[Judgement] `TestSettingsMainPageFitsEveryLocale` is not edited.** It renders Main, whose Requirements line is `{n} on` with n up to 8 (the same width as 7); the AI rule changes nothing on Main, and the AI screens' fit is held by `TestSettingsMreqPageFitsEveryLocale` over the eight statuses. The plan's list names it, but there was no AI case to add.
2. **[Judgement] The "demoted presser" case of the gate test demotes the menu's owner to an administrator without `can_restrict_members`** instead of adding a second user, because the whole press harness of `settingsMreqHereEnv` presses as that owner.
3. **[Judgement] `ConfirmKeys` was added to the struct in Task 2, not Task 1,** where `renderSettingsMreqCard` first reads it, so the Task 1 commit has no unused field (the plan's own rule for unused identifiers).
4. **[Judgement] The first press of a private menu is made before the cost cases measure,** because it reads a few Redis keys of the press pipeline once; the cost test compares the AI screen with a name rule's screen after that warm-up.

No unused identifier is left: `Configured`, `UnconfiguredKey` and `ConfirmKeys` are read by the page code, and `withAINotConfigured` and every other new helper are called.

## Requirements

`requirements-completed: []`. MREQ-01, MREQ-10 and MREQ-14 stay Pending and complete in plan 15-11 after its end-to-end test. REQUIREMENTS.md is unchanged.

## Known Stubs

None. Left for later plans: the log and locale sweeps and the pacer independence test (15-10; it also adds the "AI refused press" entry to `TestMemberReqLookupAlertsFitEveryLocale`) and the end-to-end test (15-11).

## Threat Flags

None new. T-15-23 (a punishing value stored for an AI that cannot run): `TestSettingsMreqAIGate` including both stale-screen directions. T-15-24 (a page, card or alert naming the Staff Group, a channel or a member): `TestSettingsStaffPrivacy` (the pure renders over eight statuses, the AI alert, and the two Phase 15 runs). T-15-25 (a card that promises established members are judged): `TestSettingsMreqCard` fails on the generic sentence on the AI card.

## Self-Check: PASSED

`fuku/modules/settings_mreq_ai_test.go` and this SUMMARY exist, and the commits d93be6b, cc9cef7, a740d7f, 026d376, a1b2a05 and 4e6ccf2 are ancestors of HEAD.
