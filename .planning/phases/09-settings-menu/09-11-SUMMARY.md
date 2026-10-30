---
phase: 09-settings-menu
plan: 11
subsystem: telegram-ui
tags: [settings-menu, staff-group, privacy, callbacks, gotgbot]

requires:
  - phase: 09-settings-menu
    provides: the press pipeline and page registry (09-01), the staff row/link seams (09-02), the page strings (09-05)
provides:
  - "Staff Group page (page s) of the settings menu: link state and health, Link with a picker, Unlink with a second tap, read-only /staff panel inside a Staff Group"
  - "settings_card.go: the two-tap confirm card helper (settingsCardLifetime, settingsCardExpiry, settingsCardExpired, settingsCardRows) shared with plan 09-12's Lift"
  - "settingsStaffViewFor, the single enforcement point of D-21 (a group menu never names the Staff Group)"
affects: [09-12, 09-13, 09-14]

plan_head_before: b3c964cbc69d121a2911ef0e7ac19cdd43864331
plan_head_after: a63b15020ce6f6de8c0862a1ab3a152490b82d88

actuals:
  tokens: 60000
  tasks: 3
  commits: 6

tech-stack:
  added: []
  patterns:
    - "privacy by construction: the Staff Group's title and chat ID are copied into the view only for a private menu or a Staff Group's own page, so a group-menu renderer never receives them"
    - "read the database first, answer the press empty, then make the live Telegram checks"
    - "a pick carries a row ID that is re-matched against the presser's own rows, never a chat ID or a position"

key-files:
  created:
    - alita/modules/settings_staff.go
    - alita/modules/settings_card.go
    - alita/modules/settings_staff_test.go
    - alita/modules/settings_staff_privacy_test.go
  modified:
    - AGENTS.md

key-decisions:
  - "The page reads its database rows first and answers the press empty only afterwards (before the first Telegram call). A failed read therefore keeps the cannot-open alert as the press's one answer, which the plan's literal 'answer first' would have swallowed."
  - "The read-only panel's Refresh is {a: pg, v: <page>} (keeps the page); only the linked page's Refresh is {a: op, p: s}."
  - "renderSettingsStaffPicker strips titles itself when the view is not private, in addition to settingsStaffViewFor stripping them for the linked page, so no group-menu renderer can print a title."

requirements-completed: [MENU-03, MENU-04, MENU-05]

coverage:
  - id: D1
    description: "The Staff Group page shows the link and its health in the private variant (title and ID) and the group variant (hidden), with Refresh and Unlink; a group that is not linked offers Link; Main shows Linked / Not linked / N linked"
    requirement: "MENU-05"
    verification:
      - kind: unit
        ref: "alita/modules/settings_staff_test.go#TestSettingsStaffPageVariants"
        status: pass
    human_judgment: false
  - id: D2
    description: "Inside a Staff Group the page is the /staff panel read-only: 8 groups a page, Prev/Next with clamped pages, inside 3800 UTF-16 units, no Link, Unlink or staff| buttons"
    requirement: "MENU-05"
    verification:
      - kind: unit
        ref: "alita/modules/settings_staff_test.go#TestSettingsStaffReadOnly"
        status: pass
    human_judgment: false
  - id: D3
    description: "Link runs the /linkstaff pieces (none, check failed, one at once, picker), a pick carries the staff_groups row ID and is re-matched against the presser's rows then verified live, forged picks link nothing"
    requirement: "MENU-03"
    verification:
      - kind: unit
        ref: "alita/modules/settings_staff_test.go#TestSettingsStaffLink"
        status: pass
      - kind: unit
        ref: "alita/modules/settings_staff_test.go#TestSettingsStaffPick"
        status: pass
    human_judgment: false
  - id: D4
    description: "Unlink asks a second tap in place, expires after 5 minutes lazily, re-checks the creator of both groups at the ask and at Yes, and two Yes presses unlink once and post one notice"
    requirement: "MENU-04"
    verification:
      - kind: unit
        ref: "alita/modules/settings_staff_test.go#TestSettingsStaffUnlink"
        status: pass
      - kind: unit
        ref: "alita/modules/settings_staff_test.go#TestSettingsCardExpiry"
        status: pass
    human_judgment: false
  - id: D5
    description: "A menu opened in the group never names the Staff Group in any text, label, notice, alert or callback data, in 7 languages, on every reachable screen and in an end-to-end run"
    requirement: "MENU-05"
    verification:
      - kind: unit
        ref: "alita/modules/settings_staff_privacy_test.go#TestSettingsStaffPrivacy"
        status: pass
    human_judgment: false
  - id: D6
    description: "The menu refuses what /linkstaff and /unlinkstaff refuse; every Staff Group button fits 64 bytes in both variants"
    requirement: "MENU-03"
    verification:
      - kind: unit
        ref: "alita/modules/settings_staff_test.go#TestSettingsAuthorityParityStaff"
        status: pass
      - kind: unit
        ref: "alita/modules/settings_staff_test.go#TestSettingsStaffCallbackBytes"
        status: pass
    human_judgment: false
  - id: D7
    description: "How the page, the picker and the card read on real Telegram clients (label truncation in hi and ru, the picker with long titles, the live health build latency with many linked groups)"
    verification: []
    human_judgment: true
    rationale: "Client rendering and real Bot API latency cannot be asserted by a fake; covered by the deferred real-device checklist 09-SPIKE.md"

duration: 70min
completed: 2026-10-30
status: complete
---

# Phase 9 Plan 11: Staff Group page Summary

**The settings menu links and unlinks a group through the /linkstaff and /unlinkstaff code, shows the link's live health, shows the read-only /staff panel inside a Staff Group, and a menu opened in the group is proven never to name the Staff Group.**

## Accomplishments

- `settings_staff.go` (page `s`, order 10): `settingsStaffMainState` (Linked / Not linked / `{count} linked`, database only), `settingsStaffViewFor` (the D-21 enforcement point), `renderSettingsStaffLinked`, `renderSettingsStaffNotLinked`, `renderSettingsStaffPanel` (read-only `/staff` panel, 8 groups a page, fitted into what is left of 3800 UTF-16 units after the settings header, group line, notice and the `settings_staff_readonly` line), `renderSettingsStaffPicker`, `renderSettingsStaffUnlinkCard`, and the actions `lk`, `ua`, `uy`, `pg`.
- Link (`lk`): `listVerifiedOwnStaffGroups` decides (none, check failed, exactly one links at once through `linkToStaffGroup` and `announceStaffLink`, otherwise a picker of at most 8). A pick (`lk` with `s`) takes the staff_groups row ID, lists the presser's own Staff Groups again, takes the row whose ID equals `s`, then `resolveNamedStaffGroup` verifies the live creator. Every refusal is the command's own text through `staffLinkRefusalText` as a notice.
- Unlink (`ua`, `uy`): the ask runs `checkUnlinkAuthority`, the card is the shared two-tap helper, Yes (with `e`, five minutes ahead, lazy expiry) runs `runUnlinkGroup`, whose conditional delete lets one of two Yes presses win and post the one `staff_notice_unlinked_by` to the Staff Group only.
- `settings_card.go`: `settingsCardLifetime`, `settingsCardExpiry`, `settingsCardExpired`, `settingsCardRows`, ready for plan 09-12's Lift.
- The page reads the database, answers the press empty, then makes its live checks (`buildStaffPanelRows`, at most 2N+1 calls, 45 s bound). A link the owner check removed draws as "Not linked" with no notice.
- AGENTS.md: the Staff Group page's actions, the `s` row-ID rule and the card under Subsystem traps/Callbacks, and the D-21 rule.

## Task Commits

1. Task 1 RED: `f159cb0` test(09-11): show there is no Staff Group page
2. Task 1 GREEN: `2b90d38` feat(09-11): add the Staff Group page to the settings menu
3. Task 2 RED: `72f5abd` test(09-11): show Link and Unlink are missing from the Staff Group page
4. Task 2 GREEN: `a3789a7` feat(09-11): link and unlink from the Staff Group page
5. Task 3 tests: `37ab548` test(09-11): prove the group menu never names the Staff Group
6. Task 3 docs: `a63b150` docs(09-11): record the Staff Group page rules in AGENTS.md

## Verification

- `CGO_ENABLED=0 go build ./...`: ok.
- `go test -tags testtools -race -count=1 -v` on `./alita/modules` for `TestSettingsStaffPageVariants`, `TestSettingsStaffReadOnly`, `TestSettingsCardExpiry`, `TestSettingsStaffLink`, `TestSettingsStaffPick`, `TestSettingsStaffUnlink`, `TestSettingsStaffPrivacy`, `TestSettingsAuthorityParityStaff`, `TestSettingsStaffCallbackBytes` plus the pinning tests `TestLinkStaffTracer`, `TestUnlinkStaffTracer`, `TestStaffUnlinkButtonConfirmTwiceRemovesOnce` and every `TestSettings*` test of plans 09-01 and 09-07: all PASS (each named test printed its `--- PASS` line).
- Mutation checks: answering after the build made the answer-order test fail; making `settingsStaffViewFor` always Full made `TestSettingsStaffPrivacy` fail in every language. Both were restored.
- `go vet -tags testtools ./alita/modules/` exits 0 and `gofmt -l alita/` prints nothing. `make lint` was not run: golangci-lint in the sandbox is built with go1.25 and cannot lint this go1.26.0 module, so `gofmt` and `go vet` stood in. `make check-translations` and `make check-docs`: clean. The whole `make test` was left to the orchestrator.
- Acceptance greps: the four render functions, `renderStaffRowStatus(`, `buildStaffPanelRows(`, `staffPanelTextFor(`, the six command functions (`listVerifiedOwnStaffGroups(`, `linkToStaffGroup(`, `announceStaffLink(`, `resolveNamedStaffGroup(`, `checkUnlinkAuthority(`, `runUnlinkGroup(`) in `settings_staff.go`; the three card symbols in `settings_card.go`; `settingsStaffViewFor` and `staff_groups row ID` in AGENTS.md.

## Deviations from Plan

**1. [Rule 1 - Plan defect] "Answer the press empty first" would swallow the cannot-open alert**
- **Found during:** Task 1
- **Issue:** AGENTS.md says a page that cannot read its data answers `settings_cannot_open_alert` and edits to `settings_cannot_open`. Answering empty before the database read would make the alert a no-op (`settingsPress.answer` answers once).
- **Fix:** `settingsStaffBuild` reads the database first (no Telegram call), then answers empty, then makes the live checks. D-31's point, that the answer precedes the live checks, holds and is tested (`answer at call < first getChatAdministrators/bot getChatMember`).
- **Files:** `alita/modules/settings_staff.go`. **Commit:** `2b90d38`.

**2. [Rule 1 - Plan test cannot hold] "Yes is refused when ownership moved and the link stays"**
- **Found during:** Task 2
- **Issue:** after the refusal the page is redrawn with the live owner recheck (`buildStaffPanelRows`), and if the Staff Group's creator really changed, `recheckLink` correctly removes the link of a maker who no longer owns it (and tells the Staff Group), exactly as `/staff` does. So "the link stays" cannot be asserted for a mismatch.
- **Fix:** the test asserts the refusal text (`staff_unlink_refuse_not_owner`) and that Yes itself posted no `staff_notice_unlinked_by`; a separate test shows that an owner check that cannot be answered at Yes (error, not mismatch) refuses with `staff_check_failed` and the link stays.
- **Files:** `alita/modules/settings_staff_test.go`. **Commit:** `a3789a7` (test in `72f5abd`/`a3789a7`).

**3. [Process] Panel Refresh button**: the read-only panel's Refresh is `{a: pg, v: <page>}` so it keeps the page, instead of `{a: op, p: s}` (which would return to page 1). The linked page's Refresh is `{a: op, p: s}` as planned.

**4. [Process] Task 1 and Task 3 GREEN steps**: Task 1's tests passed on the first run after the implementation; the privacy and parity tests of Task 3 were green when written because the renderers were already built from the enforcement point. They are not weakened: the mutation check above shows they fail on a leak. No `settings_staff.go` change was needed in Task 3.

**5. [Process] Plan-commit ledger file not written** (the harness refuses the compound git-dir write, as in plans 09-01/09-02): `commits` and the two head SHAs above were measured directly with `git rev-list --count b3c964c..HEAD` (6, taken before this SUMMARY's own commit).

**Total deviations:** 5 (2 plan defects fixed, 3 process notes). **Impact:** none on the delivered behaviour.

## Known Stubs

None.

## Threat Flags

None beyond the plan's threat model (T-09-38 to T-09-43 are covered by the tests named above). No new endpoint, schema or secret.

## Accepted limits (recorded)

- A forged `lk` or `ua` press inside a Staff Group runs the command's own refusals (a Staff Group cannot be linked; it has no link to unlink), so the read-only panel's missing buttons are not the only guard.
- The page's live build repeats on every paging press (the same cost as `/staff`'s Prev/Next).

## Pending human verification (deferred to 09-SPIKE.md, not blocking)

On a real bot with a Staff Group and a supergroup you created: send `/settings`, tap Open here, open Staff Group, tap Link (one Staff Group links at once and the Staff Group gets the "Group linked" notice; with two Staff Groups the picker shows positions in the group and titles in private), tap Unlink and Yes (the Staff Group gets the unlink notice, the group gets nothing), and open the menu inside the Staff Group (read-only panel, paging with more than 8 groups). Check that labels are not cut in hi and ru, and that the health build with many linked groups stays within the callback's patience.

## Self-Check: PASSED

- Created files exist: `alita/modules/settings_staff.go`, `settings_card.go`, `settings_staff_test.go`, `settings_staff_privacy_test.go` (checked on disk).
- Commits present: `f159cb0`, `2b90d38`, `72f5abd`, `a3789a7`, `37ab548`, `a63b150` (`git log` on this branch).
- All three tasks' `<verify>` commands and acceptance criteria re-run and green (see Verification).
