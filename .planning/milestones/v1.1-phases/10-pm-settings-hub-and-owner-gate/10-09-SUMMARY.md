---
phase: 10-pm-settings-hub-and-owner-gate
plan: 09
subsystem: settings
tags: [settings, private-list, pick, callbacks, telegram-errors, privacy, owner-gate]
requires:
  - phase: 10-pm-settings-hub-and-owner-gate
    provides: adminindex.ListForUserFresh (10-01), the roster feed (10-07), the sweep and the group repair path (10-08), settingsGate and isTelegramClientError (10-05), the doors slice of TestSettingsGateAllDoors (10-11)
provides:
  - The private group list (settings_hub.go) sent by /settings in the bot's private chat, sorted, labelled and paged
  - The shub|v1 callback namespace (lp paging, cx Close)
  - The pk core action of settings|v1 with the class rule at the member lookup and getChat
  - TestSettingsPickSameAnswer and the pick as the fifth door of TestSettingsGateAllDoors
  - AGENTS.md rules for the list, the namespace and the pick
affects: [10-10]

status: complete
actuals:
  tokens: 14000
  tasks: 2
  commits: 3

commits: 3
plan_head_before: 061cd09268a4021aec46c5bfcf5b9ae35cb1ec1c
plan_head_after: 324b966a290654112f25dd3743ee5e240d2f5d7d

tech-stack:
  added: []
  patterns:
    - "A private list is rebuilt from the presser's own user ID on every press, never from callback data, so a replayed or forwarded button shows the presser their own rows"
    - "An attacker-named chat in a callback is answered by Telegram error class (any 4xx except 429), never by wording, so one neutral alert hides whether the bot is in the chat"
    - "A test seam (settingsHubLookup) lets a paging test use lists of any size without seeding hundreds of roster rows"

key-files:
  created:
    - fuku/modules/settings_hub.go
    - fuku/modules/settings_hub_test.go
  modified:
    - fuku/modules/settings.go
    - fuku/modules/settings_gate.go
    - fuku/modules/settings_menu.go
    - fuku/modules/settings_pages.go
    - fuku/modules/settings_test.go
    - fuku/modules/settings_gate_test.go
    - AGENTS.md

key-decisions:
  - "A private chat passes requireSettingsAccess at once and settingsDesc.RequiredChecks is requireSettingsAccess() alone; in a group the same function still does the live admin check and then the gate, so the old requireLockdownGroup check had nothing left to do"
  - "Rows are sorted by the lower-cased collapsed title, and a group with no title is sorted by the chat ID text its row shows, so the order matches what the reader sees (the plan said title; an empty title would otherwise sort first as the empty string)"
  - "A list longer than 999 pages (7,992 groups) shows the last page as its limit, as the plan accepted"
  - "The tracer task (1) handled only cx; lp and the pick overrides arrived in task 2 after a RED test commit, so every commit's behaviour matches its tests"

requirements-completed: [MENU-09, MENU-10, MENU-11, MENU-13]

coverage:
  - id: D1
    description: "/settings in the bot's private chat sends the user's groups as a new message (header, intro, hint, one full-width button per group, Close), built from the sender's ID with a database read and no Telegram call; one group is still a one-button list"
    requirement: MENU-09
    verification:
      - kind: unit
        ref: "fuku/modules/settings_hub_test.go#TestSettingsHubPick"
        status: pass
      - kind: unit
        ref: "fuku/modules/settings_hub_test.go#TestSettingsHubList"
        status: pass
    human_judgment: false
  - id: D2
    description: "Row labels are the plain title (collapsed, trimmed, cut at 32 code points plus an ellipsis, the chat ID when empty, never escaped); rows sort by lower-cased title then chat ID; 8 a page with Prev and Next only where a page exists; a page past the last is clamped and 0, non-numeric, 4-digit or extra-field values are invalid"
    requirement: MENU-09
    verification:
      - kind: unit
        ref: "fuku/modules/settings_hub_test.go#TestSettingsHubList"
        status: pass
      - kind: unit
        ref: "fuku/modules/settings_hub_test.go#TestSettingsHubPaging"
        status: pass
      - kind: unit
        ref: "fuku/modules/settings_hub_test.go#TestSettingsHubCallbacks"
        status: pass
    human_judgment: false
  - id: D3
    description: "A tap on a row (pk) runs the whole press pipeline and edits the list message in place into the group's Main page; the list is never deleted and never turned into the cannot-open edit"
    requirement: MENU-09
    verification:
      - kind: unit
        ref: "fuku/modules/settings_hub_test.go#TestSettingsHubPick"
        status: pass
    human_judgment: false
  - id: D4
    description: "The list comes from the bot's own roster record and an empty list (stranger, gated administrator, owner with no recorded group) is one byte-identical text with the hint naming /admincache and /settings; every pick is checked live"
    requirement: MENU-10
    verification:
      - kind: unit
        ref: "fuku/modules/settings_hub_test.go#TestSettingsEmptyListSameForAll"
        status: pass
      - kind: unit
        ref: "fuku/modules/settings_hub_test.go#TestSettingsPickSameAnswer"
        status: pass
      - kind: unit
        ref: "fuku/modules/settings_hub_test.go#TestSettingsHubPageFitsEveryLocale"
        status: pass
    human_judgment: false
  - id: D5
    description: "At a pick every non-admin status and every Telegram 4xx except 429 at getChatMember or getChat, and a non-group chat, answer settings_pick_unavailable with the list unedited and one answer; 429, 5xx, transport and timeout answer settings_check_failed; a gated administrator gets settings_gate_owner_only"
    requirement: MENU-11
    verification:
      - kind: unit
        ref: "fuku/modules/settings_hub_test.go#TestSettingsPickSameAnswer"
        status: pass
    human_judgment: false
  - id: D6
    description: "The pick decides exactly as the group /settings, the anonymous re-entry, the deep link and the press for the same member record and owner state"
    requirement: MENU-11
    verification:
      - kind: unit
        ref: "fuku/modules/settings_gate_test.go#TestSettingsGateAllDoors"
        status: pass
    human_judgment: false
  - id: D7
    description: "A gated administrator's group is not listed while the switch is off and an owner exists, and is listed when the switch is on or no owner is recorded; a user who owns X and is a gated administrator of Y sees X only"
    requirement: MENU-13
    verification:
      - kind: unit
        ref: "fuku/modules/settings_hub_test.go#TestSettingsListHidesGated"
        status: pass
    human_judgment: false
  - id: D8
    description: "shub presses are accepted only on a private message, refuse a service identity before any read, rebuild the list from the presser's own ID and answer exactly once; every button's data is at most 64 bytes with Omitted 0 (pick 33, paging 18, Close 12)"
    requirement: MENU-09
    verification:
      - kind: unit
        ref: "fuku/modules/settings_hub_test.go#TestSettingsHubCallbacks"
        status: pass
      - kind: unit
        ref: "fuku/modules/settings_hub_test.go#TestSettingsHubCallbackBytes"
        status: pass
    human_judgment: false
  - id: D9
    description: "Fast Prev or Next taps that hit an edit rate limit are not retried, and an unclosed list stays until used or closed (UI-SPEC backstops)"
    requirement: MENU-09
    human_judgment: true
    rationale: "Both are accepted backstop limits with no test: the edit path logs once and never retries (shared with every menu press), and no auto-close timer exists by design"
---

# Phase 10 Plan 09: Private group list and the pick Summary

**`/settings` in the bot's private chat now sends the groups the roster says the user may manage as a sorted, paged list of plain-title buttons (the `shub|v1` namespace), and a tap edits that same message into the group's Main page through the whole press pipeline, with every forged or non-admin pick answered by one neutral alert.**

## Performance

- **Duration:** about 55 min
- **Tasks:** 2 of 2 (the tracer, then a RED test commit and a GREEN code commit)
- **Files modified:** 9 (2 created: `settings_hub.go`, `settings_hub_test.go`)
- **Commits:** 3 (`557a9c1`, `3001383`, `324b966`)

## Accomplishments

- `settings_hub.go` holds the list: `settingsHubLabel` (collapse, trim, cut at 32 code points plus "…", chat ID when empty, never escaped), `settingsHubSort` (lower-cased title in code point order, ties by chat ID), `renderSettingsHubList` (8 a page, a Page line and a paging row only with more than one page, the one neutral empty text with Close only), `settingsHubScreen` and `settingsHubCallback` (`lp` and `cx`, private messages only, service identities refused first, rebuilt from `query.From.Id`, answered once).
- `settings.go`: the private branch of `/settings` sends the list as a new message (`settingsPrivateList`); `settingsDesc.RequiredChecks` is `requireSettingsAccess()` alone, which passes a private chat. `LoadSettings` registers `callbackquery.Prefix("shub|")`.
- `settings_menu.go`: `settingsActPick` (`pk`, exactly `a` and `c`, private only, a core action no page can register) and the pick overrides in `verify`: every non-admin status and every client error (a 4xx other than 429, by class) at the member lookup or `getChat`, and a non-group chat, answer `settings_pick_unavailable`; 429, 5xx, transport and timeout answer `settings_check_failed`; the gate step is unchanged.
- Tests: `TestSettingsHubPick`, `TestSettingsHubList`, `TestSettingsHubPaging`, `TestSettingsHubEmpty`, `TestSettingsHubEmptyOnlyClose`, `TestSettingsEmptyListSameForAll`, `TestSettingsListHidesGated`, `TestSettingsHubCallbacks`, `TestSettingsPickSameAnswer`, `TestSettingsHubCallbackBytes`, `TestSettingsHubPageFitsEveryLocale`, and the pick as the fifth door of `TestSettingsGateAllDoors`.
- `AGENTS.md`: the Settings handler bullet, a new list bullet (the `shub|v1` namespace) and a pick bullet, and the gate bullet now names the pick as a door.

## Verification

| Check | Result |
|-------|--------|
| `go test -tags testtools -race -count=1 -run '^TestSettingsHubPick$' ./fuku/modules` (tracer) | ok |
| `go test -tags testtools -race -count=1 -run '^(TestSettings\|TestLoadSettings)' ./fuku/modules ./fuku/i18n` (after the tracer) | ok modules 98.8 s, ok i18n 4.8 s |
| RED run of the task 2 tests before the code | `TestSettingsHubPaging` (no translator in the first draft, fixed in the test), `TestSettingsHubCallbacks` (3 lp subtests) and `TestSettingsPickSameAnswer` failed as expected; committed as `test(10-09)` |
| the ten named task 2 tests plus `TestSettingsGateAllDoors` (after GREEN) | ok 28.1 s |
| `go test -tags testtools -race -count=1 -run '^(TestSettings\|TestLoadSettings\|TestAdminIndex)' ./fuku/modules ./fuku/i18n` (final) | ok modules 137.1 s, ok i18n 4.7 s |
| `CGO_ENABLED=0 go build ./...` | ok |
| `git diff --exit-code 6ef6ad9 -- go.mod go.sum` | clean |
| `make check-docs` | No drift detected |
| `make check-translations` | All translations are present |
| `grep -c 'callbackquery.Prefix("shub|")' settings.go` | 1 (baseline 0) |
| `grep -c settingsActPick` pages / menu | 1 / 4 |
| `grep -c 'RequiredChecks: []helpers.CheckFunc{requireSettingsAccess()}' settings.go` | 1 (baseline 0) |
| `grep -c refreshChatAdminIndex settings.go` | 1 (plan 10-08's repair call kept) |
| `grep -c '"settings_pick_unavailable"' settings_menu.go` | 4 (baseline 0) |
| `grep -c 'query.From.Id' settings_hub.go` | 3 |
| AGENTS.md `shub\|v1` / `settings_pick_unavailable` | 1 / 1 (baseline 0) |

`golangci-lint` could not run here (the installed binary was built with Go 1.25 and the module targets 1.26.0); `go vet -tags testtools ./fuku/modules` is clean.

## Task Commits

1. **Task 1 (tracer): /settings in private lists the owner's groups and a tap opens that group's Main page** - `557a9c1` (feat)
2. **Task 2 RED: tests for the list's order, labels, paging, visibility and the same-answer rule** - `3001383` (test)
3. **Task 2 GREEN: paging, the neutral pick answers and the rulebook** - `324b966` (feat)

The tracer feedback gate: the tracer's two `<verify>` commands were re-run end to end before task 2 started and passed.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Rulebook sync] AGENTS.md updated with the GREEN code commit, not the tracer**
- **Found during:** Task 1
- **Issue:** AGENTS.md should change in the same commit as a rule it invalidates, but the plan places all AGENTS edits in task 2. The tracer made the private `/settings` answer change (the Handlers bullet's `requireLockdownGroup()` claim) for one commit only.
- **Fix:** The Handlers, Callbacks and gate bullets were all updated together in the `324b966` commit that completes the feature; the tracer commit sits between two commits that both describe the final behaviour.
- **Files modified:** `AGENTS.md`
- **Commit:** `324b966`

**2. [Plan-directed test change] The private `/settings` test expects the list**
- The case "a private chat is refused" in `settings_test.go` expected `chat_status_group_only_error`; it is now "a private chat gets the group list instead of the choice message" and expects the empty list for a user with no rows, as the plan says. No other earlier press or private-menu test changed; the whole settings set passed unchanged.

**3. [Judgment] Rows with no title sort by the chat ID text**
- The plan sorts by the lower-cased title. A group with an empty title shows its chat ID as the label, and sorting the empty string first would put those rows ahead of every named group for no visible reason, so the sort key is the text the row shows. Named groups, the only case the plan's examples cover, sort exactly as specified.

**Total deviations:** 1 auto-fixed (Rule 2), 1 plan-directed test change, 1 judgment call. **Impact:** none on scope.

## Requirements

`requirements-completed: [MENU-09, MENU-10, MENU-11, MENU-13]`, each judged against its full text in `.planning/REQUIREMENTS.md`:

- **MENU-09** (list, paged, a pick opens the main menu): delivered by this plan.
- **MENU-10** (list from the bot's record, refreshed by the feed, sweep and repair paths, cleared when the bot leaves, every pick checked live, an empty or incomplete list names `/admincache` and `/settings`): the feed is 10-07, the sweep and `/settings` repair 10-08, and the list, the hint (on the populated and the empty list) and the live pick are this plan. All of it is true now.
- **MENU-11** (owner-only by default in the PM list, in-group `/settings`, the deep link and every press): the four doors were delivered by 10-05, 10-06 and 10-11; the PM list (hidden group, gated pick) completes it, and `TestSettingsGateAllDoors` proves all five doors agree.
- **MENU-13** (a gated admin does not see the group in their list, in-group `/settings` and the deep link refuse with a reply that never names the Staff Group, in-group setting commands unchanged): the list part is this plan; the refusals and the unchanged commands were delivered and tested earlier (`TestSettingsAuthorityParityOwnerGate`).

MENU-15 (the Main page grid and Back) and PLAT-05 stay **Pending**: they belong to plan 10-10. MENU-12 and MENU-14 were already complete.

## Known Stubs

None.

## Threat Flags

None. T-10-26 to T-10-31 are mitigated as planned (the class rule at the pick, the list rebuilt from `query.From.Id`, one empty text, the full pipeline at the pick, private-only `shub`, byte-checked buttons). No new endpoint, schema or secret was added.

## Next Phase Readiness

Ready for 10-10 (the last plan of the phase): the private Main page's Groups button back to the list, the two-column grid, the phase gate and the SPIKE file. The `shub` namespace and `renderSettingsHubList` are the target of that button; no rolling-deploy caveat was added beyond the Phase 9 ones.

## Self-Check: PASSED

- `fuku/modules/settings_hub.go`, `fuku/modules/settings_hub_test.go`, and the edited files exist; the tracer, RED and GREEN commits (`557a9c1`, `3001383`, `324b966`) are ancestors of HEAD and `git rev-list --count 061cd09..HEAD` was 3 at the end of task 2.
- No file was deleted by any commit.
