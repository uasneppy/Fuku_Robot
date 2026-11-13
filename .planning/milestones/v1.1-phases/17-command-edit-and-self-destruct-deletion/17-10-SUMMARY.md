---
phase: 17-command-edit-and-self-destruct-deletion
plan: 10
subsystem: settings-menu
tags: [settings-menu, deletion, member-messages, safeguards, privacy, tests]

requires:
  - phase: 17-command-edit-and-self-destruct-deletion
    provides: the Member messages section and the Self-destruct card (17-09), the three producers (17-03 to 17-07)
provides:
  - proof that only a presser with can_change_info changes a member rule or confirms the card, and that a Staff Group never sees the section
  - proof that every member screen and the card carry the delete-right line at one read of the bot per draw
  - proof that the page and the command hook, the edit handler and the self-destruct watcher read one rule, and a stale screen is a snapshot
  - the privacy sweep of the section in 8 languages and in a private and a group menu of a non-creator administrator
  - one production fix: the page reads a hand-written member value its rule does not accept as Off
affects: [17-11, 17-12]

plan_head_before: cf159b46d5ac4f34588e5fcaa6ec081edc4d3c60
plan_head_after: 236a8023b1b6c78096efb25c7a6530536d2c9126

actuals:
  tokens: 9800    # chars/4 over the added lines of cf159b4..236a802 in fuku/ and AGENTS.md (39339 chars)
  tasks: 3
  commits: 5      # MEASURED: git rev-list --count cf159b4..HEAD before this SUMMARY commit

tech-stack:
  added: []
  patterns:
    - "a safeguard test file per section: authority, Staff Group, drift and stale tests in place of a parity test where no command exists"
    - "the page's reading of a rule goes through one function (settingsDelEffective) that agrees with the producers' reading"

key-files:
  created:
    - fuku/modules/settings_deletion_member_safety_test.go
  modified:
    - fuku/modules/settings_deletion.go
    - fuku/modules/settings_deletion_right_test.go
    - fuku/modules/settings_deletion_screens_test.go
    - fuku/modules/settings_staff_privacy_test.go
    - fuku/modules/settings_mreq_test.go
    - fuku/modules/settings_gate_test.go
    - AGENTS.md

key-decisions:
  - "A member row whose stored delay its rule does not accept reads as Off on the page (settingsDelEffective, used for the row state and the counts); Phase 16 rows keep their own reading, so every Phase 16 screen and golden digest is unchanged."
  - "The gate-door proof for dy is a new door of TestSettingsGateAllDoors (the test does not enumerate page actions): the administrator holds can_change_info, so only the owner gate decides."
  - "The drift test uses the Phase 17 producers' own environment (newCommandDeletionEnv plus LoadMemberDeletion, batcher drained, admin cache primed) and the page's owner from the same environment, so page presses and producer deliveries share one dispatcher, database and cache."

patterns-established:
  - "a screen drawn before another admin's write is a snapshot until the next press; a card writes the absolute value it named"

requirements-completed: []
status: complete
---

# Phase 17 Plan 10: Member messages page safeguards Summary

**The Member messages section is proven safe: only a presser with the change-info right changes a member rule or confirms the Self-destruct card, a Staff Group never sees it, every screen says when the bot cannot delete at one read per draw, the page reads what the three producers read, and no screen names a Staff Group in any of the 8 languages.**

## What changed

- `fuku/modules/settings_deletion_member_safety_test.go` (new): `TestSettingsAuthorityDeletionMember` (an administrator without `can_change_info` opens the hub, list and rows, but every choice of `mc`, `me`, `ms`, Off included, and the card's Yes is the `settings_no_change_info` alert with no edit, no write and no read of the bot; the right is judged again at the Yes; a non-owner administrator with the switch off is refused `settings_gate_owner_only`; the creator writes in a group and a private menu), `TestSettingsDeletionMemberStaffGroup` (Main has no Deletion cell; the member list, three row screens, seven changing presses and `dy` are the invalid-request alert with no edit, no write and no bot read in both menu kinds; an unreadable Staff Group row is the cannot-open edit with no write; no refusal names the Staff Group's title or ID), `TestSettingsDriftDeletionMember` and `TestSettingsDeletionMemberStale`.
- `fuku/modules/settings_deletion_right_test.go`: `settingsDelScreens` gains the member list, the three row screens and the Self-destruct card, so `TestSettingsDeletionRightLine` (every right-line outcome, one line or none, answered once before the one read, widths unchanged, a rule stored and a card confirmed while the line shows) and the two Phase 16 end-to-end tests that loop over the table cover them; `TestSettingsDeletionCost` adds the member list, rows, choices, the card's draw and its Yes (one read of the bot and one of the presser each).
- `fuku/modules/settings_staff_privacy_test.go`: an `unlisted values` data set in the 8-language pure sweep (hand-written member values and an unlisted Phase 16 value) and `settingsPrivacyPhase17Run` (a private and a group-menu run of an administrator who is not the creator: refused changes and Yes, permitted Commands, Edited messages and Self-destruct through the card, every right line, a failed write and Yes, no Staff Group title or ID, no member name).
- `fuku/modules/settings_mreq_test.go`: `tg` on `mc`, `me` and `ms` is refused in step 1. `fuku/modules/settings_gate_test.go`: a `deletion Yes` door for `dy` over the four owner worlds.
- `fuku/modules/settings_deletion.go`: the production fix below. `AGENTS.md`: the `settingsRightChangeInfo` Permissions bullet names the member rows and `dy`, the Deletion page bullet states the hand-written-value rule, and the Testing section lists the Phase 17 page families and toolkit.

## Production fix found by a test (fix commit e4ee321)

`TestSettingsDriftDeletionMember` (hand-written values) proved a drift: a `member:edited` or `member:selfdestruct` row holding 45 s, or a `member:commands` row holding 60, is Off for the command hook, the edit handler and the self-destruct watcher, but the page drew it as `⏳ After 45 s` and counted it in the hub and Main lines. `SetRule` and the page refuse such values, so only a hand-written row can hold one. `settingsDelEffective` (wrapping `deletionEffectiveDelay`) now reads a member row its rule does not accept as Off for the row state and the counts; the service and bot rows keep their reading, so `TestSettingsDeletionPhase16ScreensUnchanged` passes untouched.

## Earlier-plan test edited (legitimate, named)

- `settings_deletion_screens_test.go` `TestSettingsDeletionStates` (plan 17-09) walked every stored delay on every row and expected the stored delay to be drawn as on for the member rows too. Its expectation for a member row whose stored delay the rule does not accept is now Off (`wantOn`), the reading the fix above makes true. No Phase 16 test and no golden digest was changed.

## Verification (real output)

- Task gates: `go test -tags testtools -race -count=1 -run '^(TestSettingsAuthorityDeletionMember|TestSettingsDeletionMemberStaffGroup|TestSettingsAuthorityDeletion|TestSettingsDeletionStaffGroup|TestSettingsDeletionHidden)$' -v ./fuku/modules`: all PASS. Task 2 gate (`TestSettingsDeletionRightLine|Cost|TestSettingsDriftDeletionMember|TestSettingsDeletionMemberStale|TestSettingsDriftDeletion|TestSettingsDeletionStaleState`): all six PASS. Task 3 gate (`TestSettingsStaffPrivacy|TestSettingsControlActionAllowList|TestSettingsGateAllDoors`): all three PASS (the privacy test ran its 8 language subtests and both Phase 17 runs).
- Mutation check: setting the `dy` action's right to `settingsRightAdmin` made `TestSettingsAuthorityDeletionMember` fail on the card's Yes (answered "", edited, wrote 1 set); restored.
- The whole settings family once at the end: `ok github.com/uasneppy/fuku_robot/fuku/modules 519.874s`, `ok github.com/uasneppy/fuku_robot/fuku/i18n 7.367s`.
- The Deletion-adjacent families: `go test -tags testtools -race -count=1 -timeout 30m -run '^(TestDeletion|TestMember|TestCommand|TestBot|TestScheduleBot|TestShutdown)' ./fuku/modules`: `ok github.com/uasneppy/fuku_robot/fuku/modules 568.323s`.
- `CGO_ENABLED=0 go build ./...`: ok. `go vet -tags testtools ./fuku/modules`: exit 0, no output. `gofmt -l` over the seven Go files this plan created or edited: no output.
- `python3 .planning/phases/13-member-requirements/handoff/lint-local.py 8c0c827`: the first run exit 1 on one staticcheck QF finding in my edit of `settings_deletion_screens_test.go` (De Morgan); fixed in `236a802`; second run exit 0, `no dupl, gocyclo or godox findings on lines added since 8c0c827` (exit code read directly, written to a file). After the fix commit only `TestSettingsDeletionStates` was re-run (ok, 1.628s); the two whole-family runs above ran just before that one-line expectation rewrite.
- `git diff --exit-code 8c0c827 -- go.mod go.sum`: exit 0.

## Deviations from Plan

**1. [Rule 1 - Bug] The page drew a hand-written member value as a live rule** (found by Task 2's drift test)
- **Found during:** Task 2
- **Fix:** `settingsDelEffective` in `settings_deletion.go`, with its own `fix(17-10)` commit before the test commit, as the plan prescribes; AGENTS.md updated in that commit.
- **Commit:** e4ee321

**2. Plan 17-09's `TestSettingsDeletionStates` edited** (named above, a consequence of the fix).

**3. `TestSettingsGateAllDoors` does not enumerate page actions**, so the "extended where it enumerates press actions" clause had nothing to extend; a new `deletion Yes` door was added instead, which proves the same thing for `dy`.

**4. The Staff Group three-kind check of C17-15** (a Staff Group chat gets no row and no delete from any of the three rules) is not re-tested here: plan 17-07 already carries it for all three producers (DEL-07); this plan proves the page layer of the exclusion.

## Not run

- `make test`, `make lint`, `make check-docs`, `make check-translations` (not asked for; `make lint` cannot run in this sandbox; no locale or docs-generator input changed).
- `gosec` (cannot run locally; the new code has no discarded error, no `math/rand` and no string literal assigned to a secret-like name).
- Live Telegram checks stay with plan 17-12's `17-SPIKE.md`.

## Known Stubs

None.

## Requirements

This plan contributes to DEL-03, DEL-04, DEL-05, DEL-07 and DEL-08 (rights, Staff Group exclusion, the delete-right line, page-to-producer drift) and completes none; `requirements.mark-complete` was not called (plan 17-12 closes them), `requirements-completed: []`.

## Self-Check: PASSED

- Files: `fuku/modules/settings_deletion_member_safety_test.go` FOUND.
- Commits: `51f77c2`, `e4ee321`, `68ca86a`, `a0f1706`, `236a802` all ancestors of HEAD.
