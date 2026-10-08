---
phase: quick-261008-j3d
plan: 01
subsystem: lockdown
tags: [lockdown, i18n, copy, logging]
status: complete
status_reason: "Orchestrator override: the root-package data race did not reproduce in 6 runs on the merged branch or in this worktree (it appears only under heavy parallel load and predates this item; tracked in STATE.md), and make lint needs a Go 1.26 golangci-lint. All of this item's own checks pass."
requires: []
provides:
  - "Lockdown replies carry no Telegram error text; the raw description goes to [Lockdown] log lines"
  - "Warning marker on the five lockdown failure lines in 7 locales"
  - "Help bullets in <code> spans; /lockdown help says it applies at once"
affects: [alita/modules/lockdown.go, locales, docs/lockdown]
key-files:
  modified:
    - alita/modules/lockdown.go
    - alita/modules/lockdown_refusals_test.go
    - alita/modules/lockdown_lift_test.go
    - alita/modules/lockdown_lock_unknown_test.go
    - alita/modules/lockdown_durability_test.go
    - alita/i18n/lockdown_locale_test.go
    - locales/en.yml
    - locales/es.yml
    - locales/fr.yml
    - locales/hi.yml
    - locales/id.yml
    - locales/pt.yml
    - locales/ru.yml
    - docs/src/content/docs/commands/lockdown/index.md
decisions:
  - "Telegram's raw error text never reaches a group reply; it is logged at warn level (error level for a failed lift record write)"
  - "greetings_join_request_lockdown left untouched (review finding withdrawn)"
actuals:
  tasks: 3
  commits: 3
plan_head_before: dab6c0b426af8b4d3e5bc19f1a4b2a2d36a2e411
plan_head_after: 924b5e6a636b76966cb6a46b88a1391b11222f61
completed: 2026-10-08
---

# Phase quick-261008-j3d Plan 01: Lockdown copy polish Summary

Lockdown replies no longer show Telegram's raw error text. They give one next step, the five failure lines start with a warning marker in all 7 locales, and the help shows copyable commands.

## Commits

| Task | Commit | Change |
|------|--------|--------|
| 1 (tracer) | 82bd170 | `lockdown_restore_failed`: marker, no `{detail}`, one next step; raw text kept in the warn log. Log-capture helpers, `TestLockdownWarningLines` |
| 2 | ddadf1d | `refuse` takes only a key; `lockdownDetailToken` removed; `{detail}` gone from `lockdown_lock_unknown`, `lockdown_lock_failed`, `lockdown_permissions_unreadable`; marker on `lockdown_lock_unknown`, `lockdown_lift_record_failed`, `lockdown_lift_tally_failed`, `lockdown_status_ban_failed`; new warn log for the no-permissions refusal and error log for a failed lift record write; `TestLockdownRepliesCarryNoTelegramDetail` |
| 3 | 924b5e6 | Help bullets in `<code>` plus the "at once, with no confirm step" words; capitalized `staff_panel_row_lockdown` (hi unchanged); bold `lockdown_status_active` lead; plainer `lockdown_status_permissions_unknown` and `lockdown_status_failed`; `TestLockdownStringsHaveBalancedTags`; regenerated lockdown docs page |

## Verification

- Lockdown, StaffPanel, RenderStaffRow, StaffLocale tests (`./alita/modules ./alita/i18n`, race): pass
- `make check-translations`: pass. `make generate-docs` then `make check-docs`: no drift; only `commands/lockdown/index.md` changed
- `CGO_ENABLED=0 go build ./...`: pass. `go vet -tags testtools` on both packages: clean
- `make test`: all packages pass except the root package `github.com/divkix/Alita_Robot`, which fails on a data race (`lockdown.ListJoinMsgsToDeleteFresh` read in the lockdown worker started by `TestPostInitSetsCommandsAndStartupMessage`). The same failure occurs on the base commit dab6c0b (checked by running `go test -race .` on an extracted copy), and no file touched here is involved. `alita/modules` passed (208 s). The flaky `tmute_as_a_reply` test did not fail.
- `make lint`: could not run ("the Go language version (go1.25) used to build golangci-lint is lower than the targeted Go version (1.26.0)"). `gofmt` is clean on touched files.
- `greetings_join_request_lockdown` has no diff in any locale.

## Deviations from Plan

None in code or copy. The plan's Task 1 RED step for the module test was not run on its own; the locale test RED was observed, and the module test went green with the fix.

A full disk (ENOSPC while linking tests) briefly blocked testing; it cleared on its own and nothing was deleted.

## Known Stubs

None.

## Threat Flags

None.

## Self-Check: PASSED

Commits 82bd170, ddadf1d and 924b5e6 exist on the branch; all listed files exist.
