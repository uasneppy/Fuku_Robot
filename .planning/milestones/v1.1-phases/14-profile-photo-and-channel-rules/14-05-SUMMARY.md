---
phase: 14-profile-photo-and-channel-rules
plan: 05
subsystem: member-requirements
tags: [rule-table, settings-page, lookup-rules, member-requirements]

requires:
  - phase: 14-profile-photo-and-channel-rules
    provides: plans 14-01 (columns, memberreq.RulePhoto and RuleChannel), 14-02 (locale keys), 14-03 and 14-04 (memberReqPhotoRule, memberReqChannelRule, memberReqVerdict, memberReqLookupMode)
provides:
  - the rule table with seven rows (ph bit 32, ch bit 64) carrying a Lookup function and no Broken
  - memberReqRule.Lookup and memberReqRule.HintKey; memberReqBroken skips a row with no Broken
  - memberReqLookupBroken, the fold of lookup verdicts into a mask (a bit only for an explicit breach)
  - the Requirements page with seven cells in six rows (2,2,2,1,2,2) and the photo rule screen's Warn-or-Mute hint
affects: [14-06, 14-07, 14-08, 14-09, 14-10]

plan_head_before: adf5def99548bcc9db5826671cb622cc33b2b451
plan_head_after: 382f9e8d8cd366863799e823d15f4b13e9627a37
actuals:
  tokens: 40000
  tasks: 3
  commits: 6

tech-stack:
  added: []
  patterns:
    - "One table drives the page, the controls, the notice phrases and the hold mask: adding two rows made the new rules reachable everywhere with no renderer change"
    - "A rule screen's hint is a table field (HintKey), not a test of the rule's identity"

key-files:
  created: []
  modified:
    - fuku/modules/memberreq_rules.go
    - fuku/modules/settings_mreq.go
    - fuku/modules/memberreq_rules_test.go
    - fuku/modules/memberreq_screens_test.go
    - fuku/modules/settings_mreq_test.go
    - fuku/modules/settings_mreq_card_test.go
    - fuku/modules/settings_staff_privacy_test.go
    - AGENTS.md

key-decisions:
  - "memberReqBroken judges the name rules only (it skips a nil Broken); the photo and channel bits can come only from memberReqLookupBroken, so between this plan and plan 14-06 nobody is punished for the lookup rules"
  - "The loops of TestSettingsMreqCard that press Kick, Ban and Mute skip the channel rule, with a comment naming plan 14-10 (its link check will precede the card); the channel rule is covered here for mo, step-1 validation, the authority refusals, Off and the rendering"
  - "TestMemberReqScreensFitEveryLocale measures the 'still broken' answer over the five name phrases (rows with a pure Broken) and every notice over all seven phrases with one named cap of 520"

requirements-completed: []

coverage:
  - id: D1
    description: "the table has seven rows in memberreq.Rules order, bits 1..64, every key present, exactly one of Broken and Lookup per row, ph and ch found by code"
    requirement: "MREQ-01 (contribution only)"
    verification:
      - kind: unit
        ref: "TestMemberReqRuleTable"
        status: pass
  - id: D2
    description: "memberReqBroken never reports a lookup bit and never calls a nil function; the strictest-punishment precedence holds over bits 32 and 64"
    requirement: "MREQ-01 (contribution only)"
    verification:
      - kind: unit
        ref: "TestMemberReqBrokenIgnoresLookupRules, TestMemberReqStrictest, TestMemberReqStrictestLookupBits"
        status: pass
  - id: D3
    description: "an owner opens the page (seven cells, six rows), opens the photo rule, picks Warn (stored at once) and Mute (card first), and sets the channel rule Off; the page, Main and the byte limits follow; no lookup request is made"
    requirement: "MREQ-11 and MREQ-12 (contributions only)"
    verification:
      - kind: unit
        ref: "TestSettingsMreqLookupRulesStore, TestSettingsMreqRowBudget, TestSettingsMreqCallbackBytes, TestSettingsMreqPageFitsEveryLocale"
        status: pass
  - id: D4
    description: "the photo rule's screen shows its hint right after the description; every key the table names translates in all 8 languages"
    requirement: "MREQ-11 (contribution only)"
    verification:
      - kind: unit
        ref: "TestSettingsMreqPhotoScreen, TestMemberReqRuleKeysExist, TestSettingsStaffPrivacy"
        status: pass
  - id: D5
    description: "memberReqLookupBroken folds verdicts in table order, skips Off and name rows, adds a bit only for a breach and the unknown flag only for an unknown"
    requirement: "MREQ-10 (contribution only)"
    verification:
      - kind: unit
        ref: "TestMemberReqLookupBroken"
        status: pass

duration: 55min
completed: 2026-11-08
status: complete
---

# Phase 14 Plan 05: The rule table, the seven-cell page and the folded lookup mask Summary

**The photo and channel rules are real rows of the rule table: an owner sees seven cells in six rows and stores their punishments, memberReqBroken still judges only names, and memberReqLookupBroken is ready to add a lookup bit only through an explicit breach.**

## Accomplishments

- `fuku/modules/memberreq_rules.go`: the `Lookup` field and the rows `ph` (bit 32, `memberReqPhotoRule`) and `ch` (bit 64, `memberReqChannelRule`) with the keys of the UI contract and no `Broken`; `memberReqBroken` skips a row with no `Broken`; the `HintKey` field (`ln`: `settings_mreq_hint_blank`, `ph`: `settings_mreq_hint_photo`); `memberReqLookupBroken(ctx, b, settings, chatID, userID, mode) (mask, unknown)`; comments updated (next free bit 128, seven actions in `memberReqPassValue`).
- `fuku/modules/settings_mreq.go`: the rule screen appends `rule.HintKey` right after the description instead of testing for the last-name rule (the last-name screen is byte-identical to Phase 13). The grid, controls, body and Main derive from the table and needed no other edit; `settings_pages.go` is untouched.
- AGENTS.md: the page `m` bullet (seven cells `ln un ru ar zh ph ch`, six rows, `{n} on` up to 7, the hint field, ph and ch controls), a new Subsystem traps bullet "Member requirements, the rule table", and the privacy-test counts.

## Task Commits

1. `c95d2c7` test(14-05): the Requirements page and the rule table for seven rules (RED: the edited and new tests do not compile until `Lookup` exists)
2. `0d449b8` feat(14-05): put the photo and channel rules in the rule table (GREEN of Task 1)
3. `6151406` test(14-05): the photo rule screen carries its hint and every table key exists (RED)
4. `a4fe532` feat(14-05): a rule may carry a hint, and the photo rule recommends Warn or Mute (GREEN of Task 2, plus the final form of `TestSettingsMreqPhotoScreen`)
5. `fc0ec83` test(14-05): memberReqLookupBroken folds lookup verdicts (RED)
6. `382f9e8` feat(14-05): fold lookup verdicts into the broken mask (GREEN of Task 3 and AGENTS.md)

## Phase 13 tests edited to the seven-rule truth (none re-recorded)

`TestMemberReqRuleTable` (seven codes, bits, exactly one of Broken and Lookup), `requirementsOf` (up to seven actions), `TestMemberReqStrictest` (the full mask is the 31 name bits, the subset loop compares `memberReqBroken` with the subset's name bits and calls `memberReqStrictest` with the whole subset), `TestMemberReqPassValue` (seven actions), `TestMemberReqScreensFitEveryLocale` (cap 520 named `memberReqNoticeCap`, "still broken all" over the five name phrases), `settingsMreqSeed`/`settingsMreqPageMarkers`/`settingsMreqWantPage`/`settingsMreqWantGrid`/`settingsMreqStoredActions` and every call site (`[5]int` count now 0 in both files), `settingsMreqRuleKeys`/`CellKeys`/`RuleCodes`/`DescKeys`, `settingsMreqWantSwitchRow` (switches at index 4, navigation at 5), `TestSettingsMreqMainState` (1..7 rules on), `TestSettingsDriftMreq`, `TestSettingsMreqPageFitsEveryLocale` (6 + 21 + 21 screens), `TestSettingsMreqCallbackBytes` and `TestSettingsMreqRowBudget` (six rows, widths 2,2,2,1,2,2), `settingsMreqWantRuleScreen` (hint by table row), `TestSettingsMreqCard` (skips the channel rule's strict presses, see below), `TestSettingsStaffPrivacy` (seven actions, 21 cards, ph and ch presses). The unchanged byte expectations (cell 34/38, choice 42/46, Yes 49/53, No 33/37) hold for ph and ch.

## Verification (real results)

- Task 1 verify command (the 11 named tests): ok. Task 2 verify (`TestSettingsMreqPhotoScreen|TestMemberReqRuleKeysExist|TestSettingsMreqRuleScreen|TestSettingsMreqCard|TestSettingsStaffPrivacy|TestSettingsControlActionAllowList`): ok. Task 3 verify (`TestMemberReqLookupBroken|TestMemberReqRuleTable|TestMemberReqBrokenIgnoresLookupRules`): ok.
- Settings family `go test -tags testtools -race -count=1 -run '^(TestSettings|TestLoadSettings)' ./fuku/modules ./fuku/i18n`: `ok fuku/modules 412.258s`, `ok fuku/i18n 7.132s`.
- Phase 13 regression `go test -tags testtools -race -count=1 -run '^(TestMemberReq|TestSettingsMreq|TestSettingsDriftMreq|TestSettingsAuthorityMreq|TestSettingsControlActionAllowList|TestSettingsStaffPrivacy|TestSettingsMainPageFitsEveryLocale|TestShutdownWaitsForMemberReqJobs|TestApplyWarn|TestApplyMemberReqAction)' ./fuku/modules`: `ok 158.638s` (SQLite).
- `CGO_ENABLED=0 go build ./...` ok; `go vet -tags testtools ./fuku/modules` clean; `gofmt -l` on the touched Go files: nothing; `git diff --exit-code 8c0c827 -- go.mod go.sum`: clean.
- Lint stand-in `python3 .planning/phases/13-member-requirements/handoff/lint-local.py 8c0c827`, exit code read directly: 0 ("no dupl, gocyclo or godox findings on lines added since 8c0c827").
- Acceptance greps: `Code: "ph"` 1, `Code: "ch"` 1, `[5]int` in the two settings test files 0, `want at most 400` 0, `HintKey` in `memberreq_rules.go` 4 (at least 3), `rule.HintKey` in `settings_mreq.go` 2, `func TestMemberReqRuleKeysExist` 1, `func memberReqLookupBroken(` 1, `ln un ru ar zh ph ch` in AGENTS.md 4.
- Not run: `make lint` and gosec (cannot run in this sandbox), `make test` (plan 14-12's gate), PostgreSQL (nothing here rests on it; the harness does not run on PostgreSQL here).

## Deviations from Plan

**1. [Judgment] Task 1's tests were committed before `HintKey` existed, and the hint assertions moved to Task 2.** The plan names the table rows in Task 1 and `HintKey` in Task 2, so Task 1's `settingsMreqHintKeys` held only the blank-name hint and Task 2 added the photo hint and the `HintKey` assertion in `TestMemberReqRuleTable`. Each green commit passes its own task's tests.

**2. [Scope, tests] `TestSettingsMreqCard` skips the channel rule's Kick, Ban and Mute presses**, as the plan's assumptions allow, with a comment naming plan 14-10. The channel rule is still covered by `mo`, the forged-field tests, the authority tests (every code at every value is refused without the right), Off at once and the card rendering in the size and privacy tests.

**3. [Test detail] `TestSettingsMreqPhotoScreen` counts the delta of calls around the `mo` press** (no `getUserProfilePhotos`, `getChat`, `getChatAdministrators` or `getChatMemberCount`, exactly one `getChatMember` of the presser), because opening the menu itself makes a `getChatAdministrators` call for the owner gate.

## Known Stubs

None as placeholder data. **Staged symbol with no production caller yet:** `memberReqLookupBroken` (plan 14-06 folds it into the executor); it is called by `TestMemberReqLookupBroken`, and the lint stand-in's unused check passes. `memberReqRule.Lookup` is read only by `memberReqLookupBroken` until then.

## Threat Flags

None. No new endpoint, auth path or schema; the page only gained two table rows whose callback codes are two letters and are validated against the table in step 1 (T-14-17 to T-14-20 are covered by `TestMemberReqBrokenIgnoresLookupRules`, `TestSettingsControlActionAllowList`, `TestSettingsMreqRowBudget`, `TestSettingsMreqCallbackBytes`, and the acceptance greps above).

## Known limits and new concerns

- Between this plan and plan 14-06 the page offers the photo and channel rules while the executor does not enforce them: a group that sets one is screened by nothing for it until 14-06. No release happens between plans.
- `TestMemberReqLookupBroken` swaps `memberReqRules` for the length of a test; a goroutine of an earlier test that still reads the table could race with it. The full settings family and the Phase 13 regression both ran clean under `-race`, but the swap is a small standing hazard for later tests that start jobs and do not wait for them.
- The channel rule's strict presses (Kick, Ban, Mute) are written today without a link check, as in Phase 13's shape; plan 14-10 adds the check and joins them to the table-driven loops.

## Self-Check: PASSED

The edited files exist and the six commits (`c95d2c7`, `0d449b8`, `6151406`, `a4fe532`, `fc0ec83`, `382f9e8`) are ancestors of HEAD; `git rev-list --count adf5def..HEAD` printed 6 before this summary.
