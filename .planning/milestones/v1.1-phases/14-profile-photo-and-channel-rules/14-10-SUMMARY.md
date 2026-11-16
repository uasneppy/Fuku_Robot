---
phase: 14-profile-photo-and-channel-rules
plan: 10
subsystem: member-requirements
tags: [settings-menu, channel-rule, live-link-check, forged-callback, requirements-page]

requires:
  - phase: 14-profile-photo-and-channel-rules
    provides: 14-04 (memberReqLinkedChannel with the Fresh mode and the lookup pacer), 14-05 (the seven-row rule table and the ph and ch controls), 14-09 (settingsMreqReadStatus, the Off-only channel screen, the draw helpers)
provides:
  - settingsMreqChannelGate, the live link check of a punishing channel press, called by settingsMreqChoose and settingsMreqYes
  - TestSettingsMreqChannelRefusal and TestSettingsMreqForgedFields
  - the Phase 13 table-driven settings tests covering the channel rule's punishing values
affects: [14-11, 14-12, 15]

plan_head_before: 6e3624f04ac3440913555c16627062ac073989d0
plan_head_after: 6169fc2442c351e64c6553586ae5098fbb2a429a
actuals:
  tokens: 30000
  tasks: 3
  commits: 7

tech-stack:
  added: []
  patterns:
    - "A gate whose refusal alert is the press's one answer runs before any empty answer and edits the screen itself, returning false so the caller edits nothing more"
    - "A test case that cannot run in one menu kind is not registered there (a case-table field and a continue), never t.Skip"

key-files:
  created: []
  modified:
    - fuku/modules/settings_mreq.go
    - fuku/modules/settings_mreq_lookup_test.go
    - fuku/modules/settings_mreq_test.go
    - fuku/modules/settings_mreq_card_test.go
    - AGENTS.md

key-decisions:
  - "A punishing value is Warn, Kick, Ban and Mute (v 1 to 4), as the plan, UI-SPEC section 5 and research S-09 say; only Off is exempt. The orchestrator's hand-off text said Warn never checks; the written contract was followed (see Deviations)"
  - "The refusal redraw builds the Off-only screen from the status the gate just established (no link, or basic) instead of reading the caches again; a Yes, which has no fresh row, reads the stored row only on the refusal path"
  - "An unreadable link fails closed with mreq_fixed_check_failed and edits nothing (UI-SPEC P14-A4, owner confirmation still open)"

requirements-completed: [MREQ-12]

coverage:
  - id: D1
    description: "a punishing channel press (st v 1..4 and the card's Yes) is refused with the nolink alert and the Off-only redraw when the group has no linked channel or is a basic group, writes nothing and draws no card; an unreadable link answers mreq_fixed_check_failed and edits nothing; a linked channel proceeds whatever the bot's standing"
    requirement: "MREQ-12"
    verification:
      - kind: unit
        ref: "TestSettingsMreqChannelRefusal"
        status: pass
    human_judgment: false
  - id: D2
    description: "Off, a press of the stored value and an expired Yes make no link check; a demoted presser is refused before the check; every path answers once"
    requirement: "MREQ-12"
    verification:
      - kind: unit
        ref: "TestSettingsMreqChannelRefusal"
        status: pass
    human_judgment: false
  - id: D3
    description: "malformed mo, st, tg and my fields of the new rules are refused in step 1 with no getChatMember, and a forged well-formed channel press for a group with no linked channel writes nothing"
    requirement: "MREQ-12"
    verification:
      - kind: unit
        ref: "TestSettingsMreqForgedFields"
        status: pass
    human_judgment: false
  - id: D4
    description: "the Phase 13 table-driven tests cover the channel rule's non-Off values and the photo and channel controls, none re-recorded"
    verification:
      - kind: unit
        ref: "TestSettingsMreqCard, TestSettingsControlActionAllowList, TestSettingsDriftMreq, TestSettingsAuthorityMreq, TestSettingsMreqRuleScreen"
        status: pass
    human_judgment: false

duration: 150min
completed: 2026-11-08
status: complete
---

# Phase 14 Plan 10: The live link check of a punishing channel press Summary

**A punishing value of the channel rule (Warn, Kick, Ban, Mute, by `st` or the card's Yes) now passes one fresh, paced, 3 s `getChat` of the group first: no linked channel or a basic group refuses with the status line's alert and redraws the Off-only screen, an unreadable link refuses with "could not check" and edits nothing, so a stale screen, a stale card or a forged button can no longer set a punishment for a group that cannot be linked. This completes MREQ-12.**

## Accomplishments

- `settingsMreqChannelGate(p, rule, stored, action)` in `settings_mreq.go`: a no-op for every other rule and for Off; a basic group is read from `p.group.Type` (no lookup); otherwise `memberReqLinkedChannel` with `Fresh: true` under `settingsMreqHereTimeout`, before any answer, because its alert is the press's one answer. No link: alert `settings_mreq_here_nolink` (with `{rule}`), nothing written, no card, the Off-only screen drawn through `renderSettingsMreqRuleScreenStatus` with the status the gate established. Unknown (error, timeout, shed, null): alert `mreq_fixed_check_failed`, no edit. Linked: proceed, the bot's own administrator status is never consulted.
- Two call sites: `settingsMreqChoose` after the fresh same-value read and before the card or the write, and `settingsMreqYes` after the expiry check and before `settingsMreqApplyRule`. A refusal returns the nil screen.
- `TestSettingsMreqChannelRefusal`: both menu kinds; Warn, Kick, Ban and Mute with no link; a linked channel with an administrator and with a plain-member bot (card then Yes writes); the card's Yes after the link was lost or the pacer sheds; unreadable links (500, null, timeout in the group menu, shed in both); Off with no link and with a shed pacer; the stored value; an expired Yes after the link was lost; a presser demoted before the Yes (refused with `settings_no_restrict`, zero `getChat` of the group); a basic group in both menus (no lookup of its own).
- `TestSettingsMreqForgedFields`: 21 malformed field sets refused in step 1 with no `getChatMember`, no edit and no write, plus four well-formed forged channel presses for a group with no linked channel that write nothing.
- The Phase 13 tests: `TestSettingsMreqCard` no longer skips the channel rule (linked-channel fixture); `TestSettingsControlActionAllowList` gains tg-on-every-rule-refused and st-on-ph-and-ch-accepted subtests; `TestSettingsDriftMreq` gains a subtest that the ph and ch presses change only their own columns and drop the cached key. `TestSettingsAuthorityMreq` and `TestSettingsMreqRuleScreen` already looped over all seven rules (14-05 and 14-09) and stay unchanged.
- AGENTS.md: the page `m` bullet describes the check and its order, the Permissions bullet says the new rules keep Phase 13's rights with the link check after the right, the Testing section says a test never skips to dodge a case.

## Task Commits

1. `ef9f053` test(14-10): a punishing channel press is refused when the group cannot be linked (tracer, red: 6 subtests across both menus)
2. `b950943` feat(14-10): refuse a punishing channel press when the group has no linked channel (green)
3. `d30e295` test(14-10): the card path and the paths that never reach the link check (red: the two "card's Yes checks the link again" subtests in each menu)
4. `d5dcb78` feat(14-10): check the link again at the card's Yes (green)
5. `3bf3505` test(14-10): the Phase 13 settings tests cover both new rules in full (no red: the code already held)
6. `1134dd7` test(14-10): register no case for the private menu instead of skipping it (CI run 42468815252)
7. `6169fc2` docs(14-10): describe the link check of a punishing channel press

## Verification (real results)

- Task 1 and 2 verifies (`TestSettingsMreqChannelRefusal`, `TestSettingsMreqCard`): ok.
- Task 3 verify list (`TestSettingsMreqForgedFields|ChannelRefusal|Card|AuthorityMreq|DriftMreq|ControlActionAllowList|RuleScreen`), run with `-v`: all seven `--- PASS`.
- First settings family run (`^(TestSettings|TestLoadSettings)` over `fuku/modules fuku/i18n`, 447 s): one failure, `TestSettingsMreqRedrawKeepsStatus` (14-09's test demanded that the press be answered before its last lookup request; a gated Ban press now looks up first by design). Fixed in the test (see Deviations); the mreq set then passed (77 s).
- CI-style proof: one `go test -tags testtools -json -race -count=1 -timeout 30m` run of the whole settings family plus the Phase 14 families (`TestMemberReqLookup|PhotoRule|LinkedChannel|Channel|BotStatus|WatcherLookup|Held|Fixed|JoinLookups`, `TestShutdownEndsMemberReqLookups`, `TestChatRequirementsSettings`, `TestMemberRequirementLookupMigration`, `TestApplyMemberReqAction`) over `fuku/modules`, `fuku/db/memberreq`, `fuku/db/migrations` and `fuku/i18n`: test exit 0 (modules 546 s), zero `fail` events, and `go run ./scripts/check_test_results` on that JSON exits 0. The one skip is `TestMemberRequirementLookupMigrationDefaults` (PostgreSQL only, no database here), which the gate allows.
- CI skip failure fixed first (run 42468815252): `TestSettingsMreqChannelStatusScreen`'s two private-menu cases are no longer skipped but not registered (`groupMenuOnly`); the sweep of every Phase 14 test file found no other `t.Skip` except the allowed migration test.
- `CGO_ENABLED=0 go build ./...`: ok; `go vet -tags testtools ./fuku/modules`: clean; `gofmt -l` on the touched files: clean.
- Lint stand-in `lint-local.py 8c0c827`, exit read directly: 1 once (QF1008 embedded selector in my test, fixed), then 0.
- `git diff --exit-code 8c0c827 -- go.mod go.sum`: exit 0.
- Not run: `make lint`, `make test` (sandbox limits; 14-12 is the gate plan), `make check-translations` and `make generate-docs` (no locale or doc-source change).

## Deviations from Plan

**1. [Orchestrator text vs contract] Warn is a gated value.** The hand-off message said Warn never makes the check; the plan (must_haves "v 1 to 4", Task 1's tracer is a Warn press), UI-SPEC section 5 ("a refused Warn on this screen") and research S-09 all gate Warn, and only Off is exempt. The written contract was followed; no admin is stranded because Off always works. If the owner wants Warn exempt, it is a one-condition change in `settingsMreqChannelGate` and a test update.

**2. [Plan helper] `mreqLinkChannel(group)` was not added.** `settingsMreqLinkChannel` (14-09) and `mreqChannel` already give a linked channel with an administrator bot; a second helper would only duplicate them.

**3. [Rule 1 - expectation changed by design] `TestSettingsMreqRedrawKeepsStatus` (14-09).** Its press helper required every press to be answered before the last lookup request. A gated punishing channel press makes its link check before any answer (Pitfall 10: the refusal alert is the one answer) and a card draw has no later status read. The helper now exempts a gated `st ch` press with v of at least 1 from that one assertion; everything else it checks (single answer, status kept on redraw) is unchanged.

**4. [CI fix, coordinator] Skip removal** in `TestSettingsMreqChannelStatusScreen` (14-09's test, my file list), commit `1134dd7`; AGENTS.md Testing section gained the no-skip rule.

**5. [Plan step skipped] No red phase for Task 3's tests**, because the code already held; stated at commit `3bf3505`.

## Requirements

- **MREQ-12: COMPLETED.** The channel lookup and gate (14-04), the enforcement through the job, join and watcher (14-06, 14-07), the Off-only screen (14-09) and this plan's refused punishing press make the whole text true. Ticked in REQUIREMENTS.md (checkbox and traceability row).
- MREQ-10 stays Pending (Phase 15's AI line); MREQ-01 stays Pending.

## Known Stubs

None.

## Threat Flags

None. No new endpoint, auth path or schema. T-14-39 to T-14-41 are covered by `TestSettingsMreqChannelRefusal` (forged and stale presses, single answer, no edit on unknown), `TestSettingsMreqForgedFields` and the rights tests over all seven rules.

## New concerns (named, not fixed)

- `TestSettingsStaffPrivacy` (14-11's) presses the channel rule only with Off in a group with no linked channel; its comment still says "plan 14-10 adds the channel's strict presses". A strict channel press there would now be refused as no-link, so 14-11 should run the strict presses with a linked-channel fixture (and add the refusal screens to the privacy sweep: the nolink alert and the Off-only redraw name no channel).
- A refused channel press in a private menu costs two lookups in the pipeline (the press's own `getChat` plus the gate's fresh one); accepted, same note as the status read.
- `TestSettingsMreqChannelRefusal` takes about 18 s because of the many per-case environments (each opens a fresh settings environment); not a problem alone, but 14-12's `make test` budget for `fuku/modules` grows by that.

## Self-Check: PASSED

- Modified files present; this SUMMARY present.
- Commits `ef9f053`, `b950943`, `d30e295`, `d5dcb78`, `3bf3505`, `1134dd7`, `6169fc2` are ancestors of HEAD; `git rev-list --count 6e3624f..HEAD` was 7 at the time of writing.
