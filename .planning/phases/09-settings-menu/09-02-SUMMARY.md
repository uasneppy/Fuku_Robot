---
phase: 09-settings-menu
plan: 02
subsystem: staff
tags: [staff-group, refactor, seams, settings-menu]
requires: []
provides:
  - renderStaffRowHead and renderStaffRowStatus (the row split, status never names the group)
  - staffPanelView.limit (panel text length cap, zero means 3800)
  - listVerifiedOwnStaffGroups, linkToStaffGroup, announceStaffLink (shared link flow pieces)
affects: [09-11]
tech-stack:
  added: []
  patterns: [behaviour-preserving seam extraction, live-verified candidate list that fails closed on unknown]
key-files:
  created:
    - alita/modules/staff_seams_test.go
  modified:
    - alita/modules/staff_panel.go
    - alita/modules/staff_link.go
key-decisions:
  - "listVerifiedOwnStaffGroups returns []models.StaffGroup by value; resolveOwnStaffGroup takes the address of the single match, so its refusal order is unchanged."
  - "The empty-candidates case is not split from the no-match case: both gave staffRefusalNoStaffGroup, so merging them keeps the refusal text identical."
requirements-completed: [MENU-05]
duration: ~35min
completed: 2026-10-30
status: complete
commits: 4
plan_head_before: 5df191f8683da0eca1c46c354afc11ad23532f77
plan_head_after: 75af581f1ac98fd0731306e8750c9e0b9f9a22dd
actuals:
  tokens: 6500
  tasks: 2
  commits: 4
---

# Phase 9 Plan 02: Staff row and link seams Summary

The Staff Group row renderer and the link flow are now reusable pieces the settings menu (plan 09-11) can call, while `/staff`, `/linkstaff`, the group picker and `/unlinkstaff` behave exactly as before.

## What was built

**Task 1 (panel):** `renderStaffRow` is now `renderStaffRowHead` (escaped title, chat ID in code) plus a newline plus `renderStaffRowStatus` (status icons, reason line, lockdown line with the UTC time only). The status piece never contains the linked group's title or chat ID, so a group's own menu can print it under its own header. `staffPanelView` has a new `limit` field in UTF-16 units; zero means `staffPanelMaxUTF16` (3800), and `staffPanelTextFor` fits against it, so the settings page can fit its header and footer in the same 3800 units.

**Task 2 (link flow):** `listVerifiedOwnStaffGroups` holds the live-verification loop (recorded owner is only a candidate, `CheckOwner` decides, unknown is counted and left out). `resolveOwnStaffGroup` calls it with the same refusals in the same order. `linkToStaffGroup` holds the target checks, the bot health read and the insert; `runLinkGroup` is now the anonymous check, `resolveLinkStaffGroup`, then `linkToStaffGroup`. `announceStaffLink` holds the Staff Group notice (`staff_link_done` plus the health warning); `deliverLinkOutcome` keeps its refusal branch and nil-link log and calls it. None of the new pieces sends anything to the linked group.

## Commits

| Commit | Message |
| ------ | ------- |
| bed1a57 | test(09-02): pin the panel row split and length limit |
| 5146a06 | refactor(09-02): split the panel row into head and status, add a text limit |
| 889e336 | test(09-02): pin the link flow pieces |
| 75af581 | refactor(09-02): split the link flow into listVerifiedOwnStaffGroups, linkToStaffGroup and announceStaffLink |

`commits: 4` is measured from the ledger base to HEAD before this SUMMARY's own commit.

## Verification

- `CGO_ENABLED=0 go build ./...`: ok.
- Task 1 command: all 12 named tests ran and passed (10 pre-existing unedited, plus `TestStaffRowSplitMatchesRow` and `TestStaffPanelTextLimit`).
- Task 2 command: all 20 named tests ran and passed (17 pre-existing unedited, plus `TestListVerifiedOwnStaffGroups`, `TestLinkToStaffGroup`, `TestAnnounceStaffLink`).
- Wider check: `go test -run 'Staff|LinkStaff|UnlinkStaff' ./alita/modules` passed (148 s).
- `go vet -tags testtools ./alita/modules/` exits 0; `gofmt -l alita/` is empty.
- `make lint` was not run: golangci-lint is built with go1.25 and the module is go1.26.0 in this sandbox, so `gofmt` and `go vet` stood in for it. The whole `make test` was left to the orchestrator. No locale key was added, so `make check-translations` was not needed.

## Deviations from Plan

**1. [Rule 3 - Blocking] The plan's `<automated>` commands start with `cd /home/user/Fuku_Robot`.** That is the main checkout, not this worktree, so run as written they would test the wrong tree (worktree-path-safety step 0c). I did not run them as written. I ran the identical `go test` commands from the worktree root instead, and the results above are from the worktree's code. This is a plan defect for the orchestrator to fix in `09-02-PLAN.md` if the plan is reused; I did not edit the plan.

**2. [Process] Tests split across two RED commits.** I wrote all five tests at once, then committed the Task 1 tests first and the Task 2 tests second so each task has its own RED commit as the plan prescribes. The RED commits do not compile for `./alita/modules` under `-tags testtools` until the matching implementation commit lands, which is the intent of a RED commit.

**3. [Commit trailer]** Commits end with the attribution trailer lines the harness supplied, not the ones printed in the plan text, as the orchestrator instructed.

No other deviations; no threat flags (no new endpoint, auth path, file access or schema).

## Known Stubs

None.

## Threat model

- T-09-12 (elevation of privilege): mitigated. Every candidate goes through `CheckOwner`, an unknown answer fails closed in `resolveOwnStaffGroup`, `checkLinkTarget` still checks the target's live creator, and the 17 pinning link tests plus the new `TestListVerifiedOwnStaffGroups` and `TestLinkToStaffGroup` pass.
- T-09-13 (information disclosure): mitigated. `TestStaffRowSplitMatchesRow` proves the status piece never holds the raw or escaped title or the chat ID, for every health, owner and lockdown state.

## Pending human verification

None. This plan changes no behaviour a user can see; the live checks for the settings page belong to 09-SPIKE.md.

## Self-Check: PASSED

- alita/modules/staff_seams_test.go, staff_panel.go and staff_link.go exist and hold `renderStaffRowStatus`, `listVerifiedOwnStaffGroups`, `linkToStaffGroup`, `announceStaffLink`.
- Commits bed1a57, 5146a06, 889e336 and 75af581 are in `git log`.
