---
phase: 14-profile-photo-and-channel-rules
plan: 09
subsystem: member-requirements
tags: [settings-menu, requirements-page, status-block, lookup-pacer, channel-rule, basic-group]

requires:
  - phase: 14-profile-photo-and-channel-rules
    provides: plans 14-04 to 14-08 (the lookup layer with its classifier, pacer and Fresh mode, the seven-row rule table, the executor and watcher lookup paths, the reworded locale keys including the settings_mreq_here_* lines and settings_mreq_hint_lookup)
provides:
  - the cannot-work block of the Requirements page (a list of lines, basic-group line while a rule is on, else the channel rule's no-link, not-administrator or could-not-check line)
  - the status slot of every rule screen and the Off-only channel screen (two rows, hints omitted, stored value visible and inert)
  - settingsMreqReadStatus, the one bounded paced status reader, and settingsMreqDrawPage / settingsMreqDrawRule, the one draw order (database, answer empty, status, edit) for all four draw sites
  - Fresh and non-Fresh lookups in separate flights (memberReqFlightKey)
affects: [14-10, 14-11, 14-12, 15]

plan_head_before: 6ef90e5b0269b78f08f082307e211746ea72fc31
plan_head_after: a5a894d94702cf1460aa008fc0e402e6369249f6
actuals:
  tokens: 20000
  tasks: 3
  commits: 7

tech-stack:
  added: []
  patterns:
    - "A page that shows a live status reads the database first, answers the press empty, then reads the status under one bound, so a slow Telegram never holds the spinner and an unreadable status is a mark, never an error"
    - "A status value whose zero value is 'nothing known' plus thin wrapper renderers with the zero status keep every older direct caller drawing the old shape"

key-files:
  created:
    - fuku/modules/settings_mreq_lookup_test.go
  modified:
    - fuku/modules/settings_mreq.go
    - fuku/modules/memberreq_lookup.go
    - fuku/modules/memberreq_lookup_test.go
    - fuku/modules/settings_mreq_test.go
    - AGENTS.md

key-decisions:
  - "The reader takes a scope (basic only, page, channel screen) instead of the plan's channelScreen bool, because a bool cannot tell the photo screen (no channel read even when the channel rule is on) from the page (channel read only while the rule is on)"
  - "Both draw helpers answer the press themselves (idempotent p.answer), so the dispatcher's answer-after is a no-op and the order database, answer, status, edit holds on every path including the checkRight notice redraw"
  - "The channel screen reads fresh on its opening only; redraws read the caches that the opening refreshed, which keeps every draw at two calls or fewer (plan assumption, research C4)"

requirements-completed: []

coverage:
  - id: D1
    description: "the page says in words when the channel rule cannot work (bot not an administrator, no linked channel, could not check) or the group is a basic group, in a block between the switch lines and the hints that is omitted with its blank line when empty, plus the lookup hint after the blank-name hint"
    requirement: "MREQ-10"
    verification:
      - kind: unit
        ref: "TestSettingsMreqChannelStatus, TestSettingsMreqBasicGroup"
        status: pass
    human_judgment: false
  - id: D2
    description: "every rule screen has a status slot, and the channel screen is [Off] alone (two rows, no choice hints, stored value visible) while the group has no linked channel or is a basic group; unreadable answers draw the unknown mark with three rows and never an error"
    requirement: "MREQ-12"
    verification:
      - kind: unit
        ref: "TestSettingsMreqChannelStatusScreen"
        status: pass
    human_judgment: false
  - id: D3
    description: "opening or redrawing a screen costs at most two paced calls (none for Main, the photo screen, an Off channel rule, a basic group), bounded by 3 s, never through the Phase 13 pacer, the press answered exactly once and empty before the status read; a redraw keeps the status line"
    requirement: "MREQ-10"
    verification:
      - kind: unit
        ref: "TestSettingsMreqOpenCost, TestSettingsMreqRedrawKeepsStatus"
        status: pass
    human_judgment: false
  - id: D4
    description: "the page and every status screen fit 6 rows (3 or 2 on a rule screen), 64-byte buttons and 3800 UTF-16 units in all 8 languages"
    verification:
      - kind: unit
        ref: "TestSettingsMreqRowBudget, TestSettingsMreqCallbackBytes, TestSettingsMreqPageFitsEveryLocale"
        status: pass
    human_judgment: false
  - id: D5
    description: "a Fresh lookup never joins a non-Fresh flight and the other way round, while callers of one mode still make one request"
    verification:
      - kind: unit
        ref: "TestMemberReqLookupFlightsKeepModesApart"
        status: pass
    human_judgment: false

duration: 95min
completed: 2026-11-08
status: complete
---

# Phase 14 Plan 09: The Requirements page says when a rule cannot work Summary

**The Requirements page and every rule screen now say in words, after one bounded paced read, when the channel rule cannot work (no linked channel, bot not an administrator, could not check) or the group is a basic group, the channel screen offers Off alone while it cannot be linked, and a Fresh lookup no longer joins a non-Fresh flight.**

## Performance

- **Duration:** about 95 minutes of work, plus a 430 s settings family run
- **Tasks:** 3 (plus the approved flight-key pair)
- **Files modified:** 6 (1 created)

## Accomplishments

- `settingsMreqStatus` (a Basic flag and a channel state whose zero value is unread) and the renderers `renderSettingsMreqPageStatus` and `renderSettingsMreqRuleScreenStatus`; `renderSettingsMreqPage` and `renderSettingsMreqRuleScreen` stay as wrappers with the zero status.
- The cannot-work block (`settingsMreqHereLines`, a `[]string` so Phase 15 appends `settings_mreq_here_ai` with no renderer change), the lookup hint after the blank-name hint, and the rule screens' status slot.
- `settingsMreqReadStatus(p, row, scope, fresh)`: a basic group returns from `p.group.Type` with no call; the photo and name screens and a page whose channel rule is Off make no call; otherwise one 3 s context (`settingsMreqHereTimeout`) over `memberReqLinkedChannel` and, for a linked channel only, `memberReqBotChannelStatus`, both through the lookup pacer. Every unreadable answer is unknown and draws the page's unknown mark.
- `settingsMreqDrawPage` and `settingsMreqDrawRule`: the one draw order (database read by the caller, `p.answer("", false)`, status, edit) used by page open, switch redraw, rule open (channel screen fresh) and rule redraw (caches).
- The Off-only channel screen (`settingsMreqOffOnly`): two rows, the choose and actions hints omitted, a stored Ban shown and inert.
- AGENTS.md: the page `m` bullet describes the block, the slot, the Off-only screen, the reader and its costs; the lookup flights bullet describes the `f:` keys.

## Task Commits

1. Approved addition, red: `af87a3a` test(14-09): a fresh lookup never joins a non-fresh flight (4 of 5 subtests red)
2. Approved addition, green: `cae3969` fix(14-09): keep fresh and non-fresh lookups in separate flights
3. Task 1 (tracer), red: `d77fa87` test(14-09): the page says when the channel rule cannot work
4. Task 1, green: `08febb1` feat(14-09): show on the page when the channel rule cannot work
5. Task 2, red: `bb51a53` test(14-09): rule screen status slots, the Off-only channel screen and the call counts
6. Task 2, green: `ad19a1f` feat(14-09): the rule screens say when a rule cannot work and the channel screen offers Off alone
7. Task 3: `a5a894d` docs(14-09): describe the status block of the Requirements page

Red subtests: Task 1's `TestSettingsMreqChannelStatus` (not-administrator block, no status request) was red at `d77fa87`; the administrator-bot and Off-rule subtests passed already, as designed. Task 2 at `bb51a53`: every `TestSettingsMreqChannelStatusScreen` case except the administrator bot (plus the two private skips), three `TestSettingsMreqOpenCost` cases (the fresh channel screen) and the rule-screen halves of `TestSettingsMreqBasicGroup` and `TestSettingsMreqRedrawKeepsStatus` were red; the page-only tests were already green. Task 3 had no red phase: its extended measures (`TestSettingsMreqRowBudget`, `TestSettingsMreqCallbackBytes`, `TestSettingsMreqPageFitsEveryLocale`) held on the first run, because Task 2's renderers already satisfied them.

## Verification (real results)

- `go test -tags testtools -race -count=1 -run '^TestSettingsMreqChannelStatus$' ./fuku/modules`: ok
- Task 2 verify list (`TestSettingsMreqChannelStatus|...|TestSettingsMreqCard`) plus `TestSettingsMreqChannelStatusScreen`, `TestSettingsMreqBasicGroup`, `TestSettingsMreqOpenCost`, `TestSettingsMreqRedrawKeepsStatus`, `TestSettingsDriftMreq`, `TestSettingsAuthorityMreq`, `TestSettingsControlActionAllowList`, `TestApplyMemberReqAction`: ok (28 s)
- Task 3 verify list (`RowBudget|CallbackBytes|PageFitsEveryLocale|TestSettingsMainPageFitsEveryLocale|ChannelStatus`): ok
- The flight tests and the neighbouring lookup, fixed-button and shutdown families (`TestMemberReqLookup*`, `TestMemberReqPhotoRule`, `TestMemberReqLinkedChannel`, `TestMemberReqChannel*`, `TestMemberReqBotStatus`, `TestShutdownWaitsForMemberReqJobs`, `TestMemberReqFixed*`): ok (134 s)
- Settings family `go test -tags testtools -race -count=1 -run '^(TestSettings|TestLoadSettings)' ./fuku/modules ./fuku/i18n`: ok, modules 430.075 s, i18n 7.156 s (includes `TestSettingsStaffPrivacy`, run on the unchanged privacy sweep)
- `CGO_ENABLED=0 go build ./...`: ok; `go vet -tags testtools ./fuku/modules`: clean; `gofmt -l` on the touched files: clean
- Lint stand-in `lint-local.py 8c0c827`, exit read directly: 0 (twice: after Task 2 and after Task 3)
- `git diff --exit-code 8c0c827 -- go.mod go.sum`: exit 0
- Not run: `make lint`, `make test` (sandbox limits, and 14-12 is the gate plan); `make check-translations` and `make generate-docs` (no locale or doc-source change in this plan).

## Deviations from Plan

### Approved by the orchestrator

**1. [Outside the file list] Fresh and non-Fresh lookups use separate flights**
- **Found during:** plan 14-08's report (named one-line fix)
- **Issue:** the flights were keyed `p:<user>`, `s:<channel>:<user>`, `l:<chat>` and `b:<channel>` without the lookup mode, so a Fresh caller could join a running non-Fresh flight that answered from a stale positive cache read.
- **Fix:** `memberReqFlightKey` prefixes `f:` for a Fresh lookup at the four flight sites; non-Fresh keys are unchanged (so `shutdown_drains_test.go`'s `p:` key still collides on purpose). `TestMemberReqLookupFlightsKeepModesApart` (in `memberreq_lookup_test.go`, a third file outside the list) proves a Fresh photo, bot-status and linked-channel caller makes its own request during a held non-Fresh flight and ignores a stale pass, a non-Fresh caller does not join a Fresh flight, and 100 Fresh callers still make one request (100 non-Fresh callers are `TestMemberReqLookupSingleflight`, unchanged and green). The AGENTS.md flights bullet is updated in the same commit.
- **Files modified:** `fuku/modules/memberreq_lookup.go`, `fuku/modules/memberreq_lookup_test.go`, `AGENTS.md`
- **Commits:** `af87a3a`, `cae3969`

### Auto-fixed

**2. [Rule 1 - expectation changed by design] Three Phase 13 tests link a channel**
- **Found during:** Task 2 (full mreq page run)
- **Issue:** `TestSettingsMreqLookupRulesStore`, `TestSettingsAuthorityMreq` and `TestSettingsMreqRuleScreen` open the channel rule's screen in a group with no linked channel and expected the three-row shape; by D-05 and M7 that screen is now `[Off]` alone.
- **Fix:** each calls the new `settingsMreqLinkChannel` (a linked channel whose bot is an administrator) after building its environment; no assertion was weakened or re-recorded.
- **Files modified:** `fuku/modules/settings_mreq_test.go`, `fuku/modules/settings_mreq_lookup_test.go`
- **Commit:** `ad19a1f`

**3. [Plan signature] `settingsMreqReadStatus` takes a scope, not `channelScreen bool`**
- A bool cannot separate the photo or name screen (no channel read even when the channel rule is on) from the page (channel read only while it is on). The reader's parameters are `(p, row, scope, fresh)`. The acceptance grep `settingsMreqReadStatus(` prints 3 in `settings_mreq.go` (the definition and the two draw helpers' calls), not 5, because the four draw sites share two helpers; the four sites are still each covered by a named test.

## Requirements

- **MREQ-10: contributed, NOT completed (orchestrator decision).** The requirement text ends with "AI not configured", which is Phase 15's `settings_mreq_here_ai` line, appended to the cannot-work block this plan builds. Left Pending in REQUIREMENTS.md (checkbox and traceability cell); `requirements-completed: []`. Phase 15 completes MREQ-10 with its AI line.
- MREQ-12: contributed (the Off-only channel screen); 14-10 completes it. Not ticked.

## Known Stubs

None. No placeholder text, empty data flow or TODO was added.

## Threat Flags

None. No new endpoint, auth path, file access or schema: the page makes at most two paced calls with chat IDs taken from `getChat`, never from callback data. T-14-35 to T-14-38 are covered by `TestSettingsMreqChannelStatus` (no channel ID on the page), `TestSettingsMreqOpenCost`, `TestSettingsMreqChannelStatusScreen` (a 429, shed, 5xx, null and timeout are never "not an administrator") and the answer-once checks.

## New concerns (named, not fixed)

- The channel screen's redraw after a press reads the caches (plan assumption), while UI-SPEC section 2 says every draw of the channel screen reads fresh. The press's own live check (14-10) is what repairs a stale screen; a redraw can show a up to 10 minute old "no linked channel" if the link changed between the opening and the press. Within budget and failing safe (inert), but it differs from the UI-SPEC wording.
- A private-menu draw makes one more `getChat` than a group draw (the press pipeline's own, not reused): accepted in the plan, now stated in AGENTS.md.
- `TestSettingsStaffPrivacy` (14-11's) still renders only the zero-status screens through `renderSettingsMreqPage` and `renderSettingsMreqRuleScreen`; it passes, but the new status screens are covered for privacy only by `TestSettingsMreqChannelStatus` (channel ID absent) and the size sweep's Staff Group marker check inside `TestSettingsMreqPageFitsEveryLocale`, until 14-11 extends it.

## Self-Check: PASSED

- Created file present: `fuku/modules/settings_mreq_lookup_test.go`; this SUMMARY.
- Commits `af87a3a`, `cae3969`, `d77fa87`, `08febb1`, `bb51a53`, `ad19a1f`, `a5a894d` are ancestors of HEAD.
- `git rev-list --count 6ef90e5..HEAD` was 7 at the time of writing.
